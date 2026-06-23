package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// bin/dotfiles-setup deliberately keeps two self-contained copies of a few
// definitions: one in the outer setup script and one inside the embedded
// `dotfiles` CLI heredoc (which must stand alone — see embedded_cli_restore_test).
// Historically these copies drifted: the embedded load_theme_colors was missing
// themes its own SUPPORTED_THEMES advertised (C23), and the restore guard was
// never pasted into the heredoc (C22). We cannot physically de-duplicate them
// without breaking the self-contained-heredoc invariant, so instead we PIN them:
// these tests fail the moment one copy is edited without the other, which is the
// practical guarantee that "the two copies cannot drift".

// readSetupSource returns the full text of bin/dotfiles-setup plus the byte
// offset where the embedded CLI heredoc begins, so callers can distinguish the
// outer script region from the embedded region.
func readSetupSource(t *testing.T) (src string, heredocStart int) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	data, err := os.ReadFile(filepath.Join(repoRoot, "bin", "dotfiles-setup"))
	if err != nil {
		t.Fatalf("read bin/dotfiles-setup: %v", err)
	}
	src = string(data)
	heredocStart = strings.Index(src, "cat > ~/.local/bin/dotfiles << 'DOTFILES_CLI_EOF'")
	if heredocStart < 0 {
		t.Fatal("embedded CLI heredoc start marker not found")
	}
	return src, heredocStart
}

var supportedThemesRe = regexp.MustCompile(`(?m)^SUPPORTED_THEMES="([^"]*)"`)

// extractFuncBody returns the text of the column-0 shell function `name` within
// region (from `name() {` through the first line that is exactly `}`).
func extractFuncBody(t *testing.T, region, name string) string {
	t.Helper()
	start := strings.Index(region, name+"() {")
	if start < 0 {
		t.Fatalf("function %s not found in region", name)
	}
	rest := region[start:]
	end := strings.Index(rest, "\n}")
	if end < 0 {
		t.Fatalf("end of function %s not found", name)
	}
	return rest[:end+2]
}

var themeCaseRe = regexp.MustCompile(`(?m)^\s+([a-z][a-z0-9-]+)\)\s*$`)

func themeLabels(funcBody string) []string {
	matches := themeCaseRe.FindAllStringSubmatch(funcBody, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

// TestEmbeddedSupportedThemesMatchesOuter pins the two SUPPORTED_THEMES strings.
func TestEmbeddedSupportedThemesMatchesOuter(t *testing.T) {
	src, heredocStart := readSetupSource(t)
	outer := supportedThemesRe.FindStringSubmatch(src[:heredocStart])
	embedded := supportedThemesRe.FindStringSubmatch(src[heredocStart:])
	if outer == nil {
		t.Fatal("outer SUPPORTED_THEMES not found")
	}
	if embedded == nil {
		t.Fatal("embedded SUPPORTED_THEMES not found")
	}
	if outer[1] != embedded[1] {
		t.Fatalf("SUPPORTED_THEMES drifted between the outer script and the embedded CLI:\nouter:    %q\nembedded: %q", outer[1], embedded[1])
	}
}

// TestEmbeddedThemeColorsCoverSupportedThemes pins load_theme_colors: both copies
// must define a case for exactly the advertised themes (catches C23, where the
// embedded copy lacked themes SUPPORTED_THEMES promised).
func TestEmbeddedThemeColorsCoverSupportedThemes(t *testing.T) {
	src, heredocStart := readSetupSource(t)
	outerRegion, embeddedRegion := src[:heredocStart], src[heredocStart:]

	outerLabels := themeLabels(extractFuncBody(t, outerRegion, "load_theme_colors"))
	embeddedLabels := themeLabels(extractFuncBody(t, embeddedRegion, "load_theme_colors"))
	if strings.Join(outerLabels, ",") != strings.Join(embeddedLabels, ",") {
		t.Fatalf("load_theme_colors theme cases drifted:\nouter:    %v\nembedded: %v", outerLabels, embeddedLabels)
	}

	themes := strings.Fields(supportedThemesRe.FindStringSubmatch(outerRegion)[1])
	sort.Strings(themes)
	if strings.Join(themes, ",") != strings.Join(outerLabels, ",") {
		t.Fatalf("SUPPORTED_THEMES and load_theme_colors disagree:\nadvertised: %v\nhandled:    %v", themes, outerLabels)
	}
}

// TestEmbeddedRestoreGuardMatchesOuter pins the security-critical restore guard:
// the embedded copy must be byte-identical to the outer one (catches any drift in
// the path-traversal checks, the C22 class).
func TestEmbeddedRestoreGuardMatchesOuter(t *testing.T) {
	src, heredocStart := readSetupSource(t)
	outer := extractFuncBody(t, src[:heredocStart], "is_safe_restore_path")
	embedded := extractFuncBody(t, src[heredocStart:], "is_safe_restore_path")
	if outer != embedded {
		t.Fatalf("is_safe_restore_path drifted between the outer script and the embedded CLI; the two copies must stay identical")
	}
}
