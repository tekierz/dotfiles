package tools

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	BtopFieldTheme      = "theme"
	BtopFieldUpdateMs   = "update_ms"
	BtopFieldShowTemp   = "show_temp"
	BtopFieldGraphType  = "graph_type"
	BtopFieldTempScale  = "temp_scale"
	BtopFieldShownBoxes = "shown_boxes"

	btopManagedStart = "# >>> dotfiles managed btop settings >>>"
	btopManagedEnd   = "# <<< dotfiles managed btop settings <<<"
)

type BtopConfigImport struct {
	Config   BtopConfig
	Fields   map[string]ConfigFieldProvenance
	Sources  []ConfigImportSource
	Managed  bool
	Warnings []string
}

type btopAssignment struct {
	key        string
	value      any
	provenance ConfigFieldProvenance
}

// BtopConfigMutationPath returns the one config path selected by btop's XDG
// lookup. A malformed relative XDG_CONFIG_HOME is rejected instead of silently
// writing an inactive ~/.config file.
func BtopConfigMutationPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return "", fmt.Errorf("XDG_CONFIG_HOME must be absolute for btop config")
		}
		return filepath.Join(filepath.Clean(xdg), "btop", "btop.conf"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine HOME for btop config: %w", err)
	}
	root := filepath.Join(home, ".config")
	return filepath.Join(root, "btop", "btop.conf"), nil
}

// BtopThemeMutationPath returns the exact generated theme sidecar referenced by
// the managed btop settings section.
func BtopThemeMutationPath(cfg BtopConfig, themeName string) (string, error) {
	configPath, err := BtopConfigMutationPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), "themes", BtopThemeArtifactName(cfg, themeName)+".theme"), nil
}

func ImportBtopConfig() (BtopConfigImport, error) {
	path, err := BtopConfigMutationPath()
	if err != nil {
		return BtopConfigImport{}, err
	}
	content, exists, err := readNativeConfig(path)
	if err != nil {
		return BtopConfigImport{}, err
	}
	return parseBtopConfigImport(path, content, exists)
}

// InspectBtopConfigContent parses bytes already captured by a reviewed plan so
// syntax validation and revision authority describe the same observation.
func InspectBtopConfigContent(path string, content []byte, exists bool) (BtopConfigImport, error) {
	return parseBtopConfigImport(path, content, exists)
}

func parseBtopConfigImport(path string, content []byte, exists bool) (BtopConfigImport, error) {
	result := BtopConfigImport{Fields: make(map[string]ConfigFieldProvenance)}
	result.Sources = []ConfigImportSource{{Path: path, Exists: exists, Active: true}}
	if !exists {
		return result, nil
	}

	legacyManaged := hasGeneratedConfigHeader(content)
	starts := exactManagedMarkerLines(string(content), btopManagedStart)
	ends := exactManagedMarkerLines(string(content), btopManagedEnd)
	if len(starts) != len(ends) || len(starts) > 1 || (len(starts) == 1 && starts[0].start >= ends[0].start) {
		return BtopConfigImport{}, fmt.Errorf("refusing to import btop config %s with ambiguous or incomplete dotfiles managed sections (%d start, %d end)", path, len(starts), len(ends))
	}
	if len(ends) == 1 && len(bytes.TrimSpace(content[ends[0].end:])) != 0 {
		return BtopConfigImport{}, fmt.Errorf("refusing to import btop config %s whose dotfiles managed section is not trailing", path)
	}
	result.Managed = legacyManaged || len(starts) == 1
	result.Sources[0].Managed = result.Managed

	assignments := make([]btopAssignment, 0)
	managed := legacyManaged
	for index, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		switch line {
		case btopManagedStart:
			managed = true
			continue
		case btopManagedEnd:
			managed = legacyManaged
			continue
		}
		assignment, recognized, warning := parseBtopAssignment(path, index+1, line, managed)
		if warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
		if recognized && warning == "" {
			assignments = append(assignments, assignment)
		}
	}
	applyBtopImport(&result, assignments)
	return result, nil
}

func parseBtopAssignment(path string, lineNumber int, raw string, managed bool) (btopAssignment, bool, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return btopAssignment{}, false, ""
	}
	equal := strings.IndexByte(trimmed, '=')
	if equal < 1 {
		if fields := strings.Fields(trimmed); len(fields) != 0 {
			if _, modeled := btopFieldIDForKey(fields[0]); modeled {
				return btopAssignment{}, true, fmt.Sprintf("%s:%d: malformed effective btop assignment for %s", path, lineNumber, fields[0])
			}
		}
		return btopAssignment{}, false, ""
	}
	key := strings.TrimSpace(trimmed[:equal])
	fieldID, recognized := btopFieldIDForKey(key)
	if !recognized {
		return btopAssignment{}, false, ""
	}
	valueText := strings.TrimSpace(trimmed[equal+1:])
	value, ok := parseBtopLiteral(valueText)
	if !ok {
		return btopAssignment{}, true, fmt.Sprintf("%s:%d: malformed effective btop assignment for %s", path, lineNumber, key)
	}
	scope := ConfigValueNative
	if managed {
		scope = ConfigValueManaged
	}
	return btopAssignment{key: fieldID, value: value, provenance: ConfigFieldProvenance{Path: path, Line: lineNumber, Key: key, Scope: scope}}, true, ""
}

func btopFieldIDForKey(key string) (string, bool) {
	switch key {
	case "color_theme":
		return BtopFieldTheme, true
	case "update_ms":
		return BtopFieldUpdateMs, true
	case "show_coretemp":
		return BtopFieldShowTemp, true
	case "graph_symbol":
		return BtopFieldGraphType, true
	case "temp_scale":
		return BtopFieldTempScale, true
	case "shown_boxes":
		return BtopFieldShownBoxes, true
	default:
		return "", false
	}
}

func parseBtopLiteral(value string) (any, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	if value[0] == '"' {
		closing := strings.IndexByte(value[1:], '"')
		if closing < 0 {
			return nil, false
		}
		closing++
		return value[1:closing], true
	}
	token := value
	fields := strings.Fields(token)
	if len(fields) == 0 {
		return nil, false
	}
	switch fields[0] {
	case "true", "True":
		return true, true
	case "false", "False":
		return false, true
	}
	digitsOnly := fields[0] != ""
	for index := 0; index < len(fields[0]); index++ {
		digitsOnly = digitsOnly && fields[0][index] >= '0' && fields[0][index] <= '9'
	}
	if digitsOnly {
		if n, err := strconv.Atoi(fields[0]); err == nil {
			return n, true
		}
	}
	return fields[0], true
}

func applyBtopImport(result *BtopConfigImport, assignments []btopAssignment) {
	effective := make(map[string]btopAssignment)
	for _, assignment := range assignments {
		effective[assignment.key] = assignment // btop uses last valid assignment.
	}
	for field, assignment := range effective {
		valid := true
		switch field {
		case BtopFieldTheme:
			value, ok := assignment.value.(string)
			valid = ok && btopThemeRepresentable(value)
			if valid {
				result.Config.Theme = value
			}
		case BtopFieldUpdateMs:
			value, ok := assignment.value.(int)
			valid = ok && value >= 250 && value <= 10000
			if valid {
				result.Config.UpdateMs = value
			}
		case BtopFieldShowTemp:
			value, ok := assignment.value.(bool)
			valid = ok
			if valid {
				result.Config.ShowTemp = value
			}
		case BtopFieldGraphType:
			value, ok := assignment.value.(string)
			valid = ok && (value == "braille" || value == "block" || value == "tty")
			if valid {
				result.Config.GraphType = value
			}
		case BtopFieldTempScale:
			value, ok := assignment.value.(string)
			valid = ok && (value == "celsius" || value == "fahrenheit")
			if valid {
				result.Config.TempScale = value
			}
		case BtopFieldShownBoxes:
			value, ok := assignment.value.(string)
			candidate := BtopConfig{ShownBoxes: value}
			valid = ok && strings.TrimSpace(value) != "" && ValidateBtopConfig(candidate, "default") == nil
			if valid {
				result.Config.ShownBoxes = value
			}
		}
		if !valid {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s:%d: effective btop value for %s cannot be represented safely", assignment.provenance.Path, assignment.provenance.Line, assignment.provenance.Key))
			continue
		}
		result.Fields[field] = assignment.provenance
	}
}

func btopThemeRepresentable(value string) bool {
	return validateConfigToken("native btop theme", value) == nil
}

func generateBtopManagedSection(cfg BtopConfig, themeName string) []byte {
	cfg = normalizeBtopConfig(cfg)
	body := fmt.Sprintf("color_theme = %q\nupdate_ms = %d\ngraph_symbol = %q\nshown_boxes = %q\nshow_coretemp = %t\ntemp_scale = %q\n",
		btopColorTheme(cfg, themeName), cfg.UpdateMs, cfg.GraphType, cfg.ShownBoxes, cfg.ShowTemp, cfg.TempScale)
	return wrapManagedConfigSection(btopManagedStart, btopManagedEnd, body)
}

func mergeBtopManagedSection(existing []byte, cfg BtopConfig, themeName string) ([]byte, bool, error) {
	return mergeManagedConfigSection(existing, generateBtopManagedSection(cfg, themeName), btopManagedStart, btopManagedEnd, "btop.conf")
}
