package tools

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

const (
	GhosttyFieldFontSize          = "font_size"
	GhosttyFieldFontFamily        = "font_family"
	GhosttyFieldOpacity           = "opacity"
	GhosttyFieldBlurRadius        = "blur_radius"
	GhosttyFieldTabBindings       = "tab_bindings"
	GhosttyFieldScrollbackLines   = "scrollback_lines"
	GhosttyFieldCursorStyle       = "cursor_style"
	GhosttyFieldWindowDecorations = "window_decorations"
	GhosttyFieldConfirmClose      = "confirm_close"
)

// GhosttyConfigImport is a non-mutating projection of recognized effective
// values from Ghostty's native file. Fields records presence and provenance;
// unrecognized and repeatable settings remain solely in the preserved source.
type GhosttyConfigImport struct {
	Config   GhosttyConfig
	Fields   map[string]ConfigFieldProvenance
	Sources  []ConfigImportSource
	Managed  bool
	Warnings []string
}

type ghosttyConfigAssignment struct {
	key        string
	value      string
	provenance ConfigFieldProvenance
}

// ImportGhosttyConfig observes ~/.config/ghostty/config through the same
// descriptor-anchored no-follow read path used by the writer. It never invokes
// Ghostty, creates config parents, or mutates ownership state.
func ImportGhosttyConfig() (GhosttyConfigImport, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return GhosttyConfigImport{}, fmt.Errorf("failed to get home directory: %w", err)
	}
	var sources []ghosttyImportSource
	for _, path := range ghosttyConfigCandidates(home) {
		content, exists, err := readNativeConfig(path)
		if err != nil {
			return GhosttyConfigImport{}, err
		}
		sources = append(sources, ghosttyImportSource{path: path, content: content, exists: exists})
	}
	return parseGhosttyConfigSources(sources)
}

type ghosttyImportSource struct {
	path    string
	content []byte
	exists  bool
}

func parseGhosttyConfigImport(path string, content []byte, exists bool) (GhosttyConfigImport, error) {
	return parseGhosttyConfigSources([]ghosttyImportSource{{path: path, content: content, exists: exists}})
}

func parseGhosttyConfigSources(sources []ghosttyImportSource) (GhosttyConfigImport, error) {
	result := GhosttyConfigImport{
		Fields: make(map[string]ConfigFieldProvenance),
	}
	var assignments []ghosttyConfigAssignment
	for _, source := range sources {
		observed := ConfigImportSource{Path: source.path, Exists: source.exists, Active: source.exists}
		if !source.exists {
			result.Sources = append(result.Sources, observed)
			continue
		}

		starts := exactManagedMarkerLines(string(source.content), ghosttyManagedStart)
		ends := exactManagedMarkerLines(string(source.content), ghosttyManagedEnd)
		switch {
		case len(starts) == 0 && len(ends) == 0:
			parsed, warnings := parseGhosttyAssignments(source.content, source.path, ConfigValueNative, 1)
			if hasGhosttyConfigFileDirective(parsed) {
				return GhosttyConfigImport{}, fmt.Errorf("refusing to infer effective Ghostty values from %s because it loads config-file sources", source.path)
			}
			assignments = append(assignments, parsed...)
			result.Warnings = append(result.Warnings, warnings...)
		case len(starts) == 1 && len(ends) == 1 && starts[0].start < ends[0].start:
			result.Managed = true
			observed.Managed = true
			before, warnings := parseGhosttyAssignments(source.content[:starts[0].start], source.path, ConfigValueNative, 1)
			if hasGhosttyConfigFileDirective(before) {
				return GhosttyConfigImport{}, fmt.Errorf("refusing to infer effective Ghostty values from %s because it loads config-file sources", source.path)
			}
			assignments = append(assignments, before...)
			result.Warnings = append(result.Warnings, warnings...)
			managedLine := bytes.Count(source.content[:starts[0].end], []byte("\n")) + 1
			managed, warnings := parseGhosttyAssignments(source.content[starts[0].end:ends[0].start], source.path, ConfigValueManaged, managedLine)
			if hasGhosttyConfigFileDirective(managed) {
				return GhosttyConfigImport{}, fmt.Errorf("managed Ghostty section in %s must not load config-file sources", source.path)
			}
			assignments = append(assignments, managed...)
			result.Warnings = append(result.Warnings, warnings...)
			afterLine := bytes.Count(source.content[:ends[0].end], []byte("\n")) + 1
			after, warnings := parseGhosttyAssignments(source.content[ends[0].end:], source.path, ConfigValueNative, afterLine)
			if hasGhosttyConfigFileDirective(after) {
				return GhosttyConfigImport{}, fmt.Errorf("refusing to infer effective Ghostty values from %s because it loads config-file sources", source.path)
			}
			assignments = append(assignments, after...)
			result.Warnings = append(result.Warnings, warnings...)
		default:
			return GhosttyConfigImport{}, fmt.Errorf("refusing to import Ghostty config %s with ambiguous or incomplete dotfiles managed sections (%d start, %d end)", source.path, len(starts), len(ends))
		}
		result.Sources = append(result.Sources, observed)
	}

	applyGhosttyImport(&result, assignments)
	return result, nil
}

func parseGhosttyAssignments(content []byte, path string, scope ConfigValueScope, startLine int) ([]ghosttyConfigAssignment, []string) {
	lines := strings.Split(string(content), "\n")
	assignments := make([]ghosttyConfigAssignment, 0)
	var warnings []string
	for index, raw := range lines {
		lineNumber := startLine + index
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		equal := strings.IndexByte(line, '=')
		if equal < 1 {
			warnings = append(warnings, fmt.Sprintf("%s:%d: ignored malformed Ghostty assignment", path, lineNumber))
			continue
		}
		key := strings.TrimSpace(line[:equal])
		if key == "" || strings.ContainsAny(key, " \t") {
			warnings = append(warnings, fmt.Sprintf("%s:%d: ignored malformed Ghostty key", path, lineNumber))
			continue
		}
		// Ghostty keys are case-sensitive and all documented keys are lowercase.
		// A differently-cased key is ineffective and must never hydrate the UI.
		if key != strings.ToLower(key) {
			continue
		}
		value, ok := parseGhosttyValue(strings.TrimSpace(line[equal+1:]))
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s:%d: ignored malformed quoted Ghostty value", path, lineNumber))
			continue
		}
		assignments = append(assignments, ghosttyConfigAssignment{
			key:   key,
			value: value,
			provenance: ConfigFieldProvenance{
				Path: path, Line: lineNumber, Key: key, Scope: scope,
			},
		})
	}
	return assignments, warnings
}

func parseGhosttyValue(value string) (string, bool) {
	if value == "" || value[0] != '"' {
		return value, true
	}
	unquoted, err := strconv.Unquote(value)
	if err != nil {
		return "", false
	}
	return unquoted, true
}

func hasGhosttyConfigFileDirective(assignments []ghosttyConfigAssignment) bool {
	for _, assignment := range assignments {
		if assignment.key == "config-file" {
			return true
		}
	}
	return false
}

func applyGhosttyImport(result *GhosttyConfigImport, assignments []ghosttyConfigAssignment) {
	effective := make(map[string]ghosttyConfigAssignment)
	var tabBinding ghosttyConfigAssignment
	var primaryFont ghosttyConfigAssignment
	fontListActive := false
	for _, assignment := range assignments {
		switch assignment.key {
		case "font-family":
			if assignment.value == "" {
				fontListActive = false
				primaryFont = ghosttyConfigAssignment{}
			} else if !fontListActive {
				primaryFont = assignment
				fontListActive = true
			}
		case "background-blur", "background-blur-radius":
			effective["background-blur"] = assignment
		case "keybind":
			if _, ok := ghosttyTabBinding(assignment.value); ok {
				tabBinding = assignment
			}
		default:
			effective[assignment.key] = assignment
		}
	}

	if assignment := primaryFont; assignment.key != "" {
		value := strings.TrimSpace(assignment.value)
		if value != "" && value == singleLineConfigText(value) {
			result.Config.FontFamily = value
			result.Fields[GhosttyFieldFontFamily] = assignment.provenance
		}
	}
	if assignment, ok := effective["font-size"]; ok {
		if value, err := strconv.ParseFloat(assignment.value, 64); err == nil && value > 0 && value <= 512 && math.Trunc(value) == value {
			result.Config.FontSize = int(value)
			result.Fields[GhosttyFieldFontSize] = assignment.provenance
		}
	}
	if assignment, ok := effective["background-opacity"]; ok {
		if value, err := strconv.ParseFloat(assignment.value, 64); err == nil && value >= 0 && value <= 1 {
			result.Config.Opacity = int(math.Round(value * 100))
			result.Fields[GhosttyFieldOpacity] = assignment.provenance
		}
	}
	if assignment, ok := effective["background-blur"]; ok {
		if value, err := strconv.Atoi(assignment.value); err == nil && value >= 0 && value <= 100 {
			result.Config.BlurRadius = value
			result.Fields[GhosttyFieldBlurRadius] = assignment.provenance
		}
	}
	if assignment, ok := effective["scrollback-limit"]; ok {
		if value, err := strconv.Atoi(assignment.value); err == nil && value >= 0 {
			result.Config.ScrollbackLines = value
			result.Fields[GhosttyFieldScrollbackLines] = assignment.provenance
		}
	}
	if assignment, ok := effective["cursor-style"]; ok {
		value := strings.ToLower(assignment.value)
		if value == "block" || value == "bar" || value == "underline" {
			result.Config.CursorStyle = value
			result.Fields[GhosttyFieldCursorStyle] = assignment.provenance
		}
	}
	if assignment, ok := effective["window-decoration"]; ok && assignment.value != "" {
		if value, valid := parseConfigBool(assignment.value); valid {
			result.Config.WindowDecorations = value
			result.Fields[GhosttyFieldWindowDecorations] = assignment.provenance
		}
	}
	if assignment, ok := effective["confirm-close-surface"]; ok && assignment.value != "" {
		if value, valid := parseConfigBool(assignment.value); valid {
			result.Config.ConfirmClose = value
			result.Fields[GhosttyFieldConfirmClose] = assignment.provenance
		}
	}
	if tabBinding.key != "" {
		if value, ok := ghosttyTabBinding(tabBinding.value); ok {
			result.Config.TabBindings = value
			result.Fields[GhosttyFieldTabBindings] = tabBinding.provenance
		}
	}
}

func ghosttyTabBinding(value string) (string, bool) {
	normalized := strings.ToLower(strings.ReplaceAll(value, " ", ""))
	switch normalized {
	case "super+t=new_tab":
		return "super", true
	case "ctrl+t=new_tab":
		return "ctrl", true
	case "ctrl+shift+t=new_tab":
		return "ctrl-shift", true
	default:
		return "", false
	}
}
