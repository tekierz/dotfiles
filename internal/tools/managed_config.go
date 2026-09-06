package tools

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tekierz/dotfiles/internal/safefile"
)

// preflightWholeFileConfig validates ownership for every file in a multi-file
// tool update before the first mutation. Individual writers still revalidate
// under their per-target lock; this preflight prevents a known later ownership
// refusal from leaving earlier files partially updated.
func preflightWholeFileConfig(path string, allowLegacyYaziTheme bool) error {
	existing, exists, err := readNativeConfig(path)
	if err != nil {
		return err
	}
	if !exists || hasGeneratedConfigHeader(existing) {
		return nil
	}
	if !allowLegacyYaziTheme || !hasLegacyGeneratedYaziThemeHeader(existing) {
		return fmt.Errorf("%w: %s", ErrUnmanagedConfig, path)
	}
	return nil
}

// managedLineSpan identifies a complete logical line, including its trailing
// newline when one is present. Managed markers are recognized only when they
// occupy the entire line; marker text in a user comment or value is not an
// ownership grant.
type managedLineSpan struct {
	start int
	end   int
}

func wrapManagedConfigSection(startMarker, endMarker, content string) []byte {
	return []byte(startMarker + "\n" + strings.TrimRight(content, "\n") + "\n" + endMarker + "\n")
}

func singleLineConfigText(value string) string {
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || isBidiControl(r) {
			return ' '
		}
		return r
	}, value))
	const maxBytes = 256
	if len(value) <= maxBytes {
		return value
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return strings.TrimSpace(value[:cut])
}

func isBidiControl(r rune) bool {
	return r == '\u061c' || r == '\u200e' || r == '\u200f' ||
		(r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069')
}

// mergeManagedConfigSection replaces one unambiguous managed section or
// appends a new one without changing any user-owned byte. ownershipAdded is
// true only when the caller is taking ownership for the first time and should
// preserve a migration backup before committing the result.
func mergeManagedConfigSection(existing, managed []byte, startMarker, endMarker, configName string) (content []byte, ownershipAdded bool, err error) {
	current := string(existing)
	starts := exactManagedMarkerLines(current, startMarker)
	ends := exactManagedMarkerLines(current, endMarker)

	switch {
	case len(starts) == 1 && len(ends) == 1 && starts[0].start < ends[0].start:
		merged := append([]byte(nil), existing[:starts[0].start]...)
		merged = append(merged, managed...)
		merged = append(merged, existing[ends[0].end:]...)
		return merged, false, nil
	case len(starts) != 0 || len(ends) != 0:
		return nil, false, fmt.Errorf("refusing to update %s with ambiguous or incomplete dotfiles managed sections (%d start, %d end)", configName, len(starts), len(ends))
	}

	if strings.TrimSpace(current) == "" {
		return append([]byte(nil), managed...), true, nil
	}

	separator := []byte("\n\n")
	if bytes.HasSuffix(existing, []byte("\n\n")) {
		separator = nil
	} else if bytes.HasSuffix(existing, []byte("\n")) {
		separator = []byte("\n")
	}
	merged := append([]byte(nil), existing...)
	merged = append(merged, separator...)
	merged = append(merged, managed...)
	return merged, true, nil
}

func exactManagedMarkerLines(content, marker string) []managedLineSpan {
	var spans []managedLineSpan
	for start := 0; start < len(content); {
		relativeEnd := strings.IndexByte(content[start:], '\n')
		end := len(content)
		contentEnd := end
		if relativeEnd >= 0 {
			contentEnd = start + relativeEnd
			end = contentEnd + 1
		}
		logicalEnd := contentEnd
		if logicalEnd > start && content[logicalEnd-1] == '\r' {
			logicalEnd--
		}
		if content[start:logicalEnd] == marker {
			spans = append(spans, managedLineSpan{start: start, end: end})
		}
		start = end
	}
	return spans
}

func errorReportsCommittedMutation(err error) bool {
	var committed interface{ Committed() bool }
	return errors.As(err, &committed) && committed.Committed()
}

// rollbackManagedArtifact removes a file created by the failed adoption or
// restores the exact previous bytes. It refuses cleanup if another actor has
// changed the artifact since our write.
func rollbackManagedArtifact(root, rel string, applied, previous []byte, previousExisted bool) error {
	current, revision, err := readToolConfig(root, rel)
	if err != nil {
		return err
	}
	if !revision.Exists() {
		if previousExisted {
			return fmt.Errorf("cannot restore %s: replacement disappeared", rel)
		}
		return nil
	}
	if !bytes.Equal(current, applied) {
		return fmt.Errorf("cannot roll back %s: artifact changed after failed adoption", rel)
	}
	if previousExisted {
		return replaceToolConfigAtRevision(root, rel, revision, previous)
	}
	return removeManagedArtifactAtRevision(root, rel, revision)
}

func removeManagedArtifactAtRevision(root, rel string, revision safefile.Revision) error {
	if err := safefile.RemoveWithinRevision(root, rel, revision); err != nil {
		return fmt.Errorf("remove orphaned adoption artifact %s: %w", rel, err)
	}
	return nil
}

func rollbackManagedArtifactIfApplied(root, rel string, applied, previous []byte, previousExisted bool) error {
	current, revision, err := readToolConfig(root, rel)
	if err != nil {
		return err
	}
	if !revision.Exists() && !previousExisted {
		return nil
	}
	if revision.Exists() && previousExisted && bytes.Equal(current, previous) {
		return nil
	}
	return rollbackManagedArtifact(root, rel, applied, previous, previousExisted)
}
