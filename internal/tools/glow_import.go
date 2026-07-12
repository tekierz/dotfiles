package tools

// This file intentionally implements only the flat YAML subset accepted by
// Glow's shipped v2.1.2 configuration template. It is not a general YAML
// parser. Compatibility is pinned to Glow v2.1.2 (f570874), Viper v1.21.0 and
// go-app-paths v0.2.2. Glow's v2.1.2 release note mentions
// CHARM_CONFIG_HOME, but the executable actually reads GLOW_CONFIG_HOME.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	GlowFieldStyle            = "style"
	GlowFieldMouse            = "mouse"
	GlowFieldPager            = "pager"
	GlowFieldWidth            = "width"
	GlowFieldAll              = "all"
	GlowFieldShowLineNumbers  = "showLineNumbers"
	GlowFieldPreserveNewLines = "preserveNewLines"
	glowManagedStart          = "# >>> dotfiles managed glow settings >>>"
	glowManagedEnd            = "# <<< dotfiles managed glow settings <<<"
)

var glowExtensions = []string{"json", "toml", "yaml", "yml", "properties", "props", "prop", "hcl", "tfvars", "dotenv", "env", "ini", ""}

type GlowConfigImport struct {
	Config   GlowConfig
	Fields   map[string]ConfigFieldProvenance
	Sources  []ConfigImportSource
	Managed  bool
	Warnings []string
}

type glowCandidate struct {
	path    string
	mutable bool
}

func glowConfigCandidates() ([]glowCandidate, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, fmt.Errorf("determine HOME for Glow config: %w", err)
	}
	var dirs []string
	for _, env := range []string{"GLOW_CONFIG_HOME", "XDG_CONFIG_HOME"} {
		if value := os.Getenv(env); value != "" {
			if !filepath.IsAbs(value) {
				return nil, fmt.Errorf("%s must be absolute for Glow config", env)
			}
			if env == "XDG_CONFIG_HOME" {
				value = filepath.Join(value, "glow")
			}
			dirs = append(dirs, filepath.Clean(value))
		}
	}
	if runtime.GOOS == "darwin" {
		dirs = append(dirs, filepath.Join(home, "Library", "Preferences", "glow"))
	} else {
		if os.Getenv("XDG_CONFIG_HOME") == "" {
			dirs = append(dirs, filepath.Join(home, ".config", "glow"))
		}
		for _, root := range filepath.SplitList(os.Getenv("XDG_CONFIG_DIRS")) {
			if root != "" {
				if !filepath.IsAbs(root) {
					return nil, fmt.Errorf("XDG_CONFIG_DIRS entries must be absolute for Glow config")
				}
				dirs = append(dirs, filepath.Join(root, "glow"))
			}
		}
		dirs = append(dirs, "/etc/xdg/glow", "/etc/glow")
	}
	seen := map[string]bool{}
	var result []glowCandidate
	for _, dir := range dirs {
		dir = filepath.Clean(dir)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		mutable := dir == home || strings.HasPrefix(dir, home+string(filepath.Separator))
		for _, ext := range glowExtensions {
			name := "glow"
			if ext != "" {
				name += "." + ext
			}
			result = append(result, glowCandidate{filepath.Join(dir, name), mutable && (ext == "yaml" || ext == "yml")})
		}
	}
	return result, nil
}

// GlowConfigMutationPath mirrors Viper's first-existing-file lookup. Only YAML
// files under HOME are writable; an active unsupported or system config blocks.
func GlowConfigMutationPath() (string, error) {
	if env := glowSettingOverride(); env != "" {
		return "", fmt.Errorf("%s overrides Glow config; unset it before management", env)
	}
	candidates, err := glowConfigCandidates()
	if err != nil {
		return "", err
	}
	for _, candidate := range candidates {
		if info, statErr := os.Stat(candidate.path); statErr == nil && !info.IsDir() {
			if !candidate.mutable {
				return "", fmt.Errorf("active Glow config %s is unsupported or outside HOME", candidate.path)
			}
			return candidate.path, nil
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return "", statErr
		}
	}
	// Glow chooses glow.yml in its highest-precedence directory when no config
	// exists. Do not silently fall through to a lower user directory.
	for _, candidate := range candidates {
		if strings.HasSuffix(candidate.path, ".yml") {
			if !candidate.mutable {
				return "", fmt.Errorf("default Glow config %s is outside HOME", candidate.path)
			}
			return candidate.path, nil
		}
	}
	return "", fmt.Errorf("no writable Glow user config directory")
}

func ImportGlowConfig() (GlowConfigImport, error) {
	path, err := GlowConfigMutationPath()
	if err != nil {
		return GlowConfigImport{}, err
	}
	content, exists, err := readNativeConfig(path)
	if err != nil {
		return GlowConfigImport{}, err
	}
	result, err := parseGlowConfigImport(path, content, exists)
	if err != nil {
		return GlowConfigImport{}, err
	}
	candidates, candidateErr := glowConfigCandidates()
	if candidateErr != nil {
		return GlowConfigImport{}, candidateErr
	}
	result.Sources = nil
	for _, candidate := range candidates {
		info, statErr := os.Stat(candidate.path)
		candidateExists := statErr == nil && !info.IsDir()
		if statErr != nil && !os.IsNotExist(statErr) {
			return GlowConfigImport{}, statErr
		}
		if candidateExists || candidate.path == path {
			result.Sources = append(result.Sources, ConfigImportSource{Path: candidate.path, Exists: candidateExists, Active: candidate.path == path, Managed: candidate.path == path && result.Managed})
		}
	}
	return result, nil
}

func glowSettingOverride() string {
	// GLOW_ENABLE_GLAMOUR selects the renderer path but does not replace any of
	// the seven persisted v2.1.2 config keys. GLAMOUR_STYLE does replace style.
	for _, env := range []string{"GLOW_STYLE", "GLAMOUR_STYLE", "GLOW_MOUSE", "GLOW_PAGER", "GLOW_WIDTH", "GLOW_ALL", "GLOW_SHOWLINENUMBERS", "GLOW_PRESERVENEWLINES"} {
		if os.Getenv(env) != "" {
			return env
		}
	}
	return ""
}

func InspectGlowConfigContent(path string, content []byte, exists bool) (GlowConfigImport, error) {
	return parseGlowConfigImport(path, content, exists)
}

type glowLine struct {
	start, end, logicalEnd, number int
	key, value                     string
	modeled, managed               bool
}

func parseGlowConfigImport(path string, content []byte, exists bool) (GlowConfigImport, error) {
	result := GlowConfigImport{Config: GlowConfig{Style: "auto", Pager: "never", Width: 0, All: true}, Fields: map[string]ConfigFieldProvenance{}, Sources: []ConfigImportSource{{Path: path, Exists: exists, Active: true}}}
	if !exists {
		result.Config = GlowConfig{Style: "auto", Pager: "never", Width: 80}
		return result, nil
	}
	lines, managed, err := parseGlowYAMLLines(path, content)
	if err != nil {
		return GlowConfigImport{}, err
	}
	result.Managed = managed
	result.Sources[0].Managed = managed
	seen := map[string]bool{}
	for _, line := range lines {
		if !line.modeled {
			continue
		}
		id := glowFieldID(line.key)
		if seen[id] {
			return GlowConfigImport{}, fmt.Errorf("%s:%d: duplicate Glow key %s", path, line.number, line.key)
		}
		seen[id] = true
		value, err := parseGlowValue(id, line.value)
		if err != nil {
			return GlowConfigImport{}, fmt.Errorf("%s:%d: %w", path, line.number, err)
		}
		scope := ConfigValueNative
		if line.managed {
			scope = ConfigValueManaged
		}
		result.Fields[id] = ConfigFieldProvenance{Path: path, Line: line.number, Key: line.key, Scope: scope}
		switch id {
		case GlowFieldStyle:
			result.Config.Style = value.(string)
		case GlowFieldMouse:
			result.Config.Mouse = value.(bool)
		case GlowFieldPager:
			if value.(bool) {
				result.Config.Pager = "auto"
			} else {
				result.Config.Pager = "never"
			}
		case GlowFieldWidth:
			result.Config.Width = value.(int)
		case GlowFieldAll:
			result.Config.All = value.(bool)
		case GlowFieldShowLineNumbers:
			result.Config.ShowLineNumbers = value.(bool)
		case GlowFieldPreserveNewLines:
			result.Config.PreserveNewLines = value.(bool)
		}
	}
	return result, nil
}

func parseGlowYAMLLines(path string, content []byte) ([]glowLine, bool, error) {
	if !utf8.Valid(content) {
		return nil, false, fmt.Errorf("%s: invalid UTF-8 in Glow YAML", path)
	}
	for _, b := range content {
		if b == 0 || b < 0x09 || (b > 0x0d && b < 0x20) {
			return nil, false, fmt.Errorf("%s: forbidden control byte in Glow YAML", path)
		}
	}
	if bytes.HasPrefix(content, []byte{0xef, 0xbb, 0xbf}) {
		return nil, false, fmt.Errorf("%s: YAML BOM is unsupported", path)
	}
	starts := exactManagedMarkerLines(string(content), glowManagedStart)
	ends := exactManagedMarkerLines(string(content), glowManagedEnd)
	if len(starts) != len(ends) || len(starts) > 1 || (len(starts) == 1 && starts[0].start >= ends[0].start) {
		return nil, false, fmt.Errorf("%s: broken Glow managed markers", path)
	}
	if len(ends) == 1 && len(bytes.TrimSpace(content[ends[0].end:])) != 0 {
		return nil, false, fmt.Errorf("%s: Glow managed block must be trailing", path)
	}
	var lines []glowLine
	inManaged := false
	seenKeys := map[string]bool{}
	for start, n := 0, 1; start < len(content); n++ {
		rel := bytes.IndexByte(content[start:], '\n')
		end := len(content)
		logical := end
		if rel >= 0 {
			logical = start + rel
			end = logical + 1
		}
		if logical > start && content[logical-1] == '\r' {
			logical--
		}
		raw := string(content[start:logical])
		trimmed := strings.TrimSpace(raw)
		if raw == glowManagedStart {
			inManaged = true
			start = end
			continue
		}
		if raw == glowManagedEnd {
			inManaged = false
			start = end
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			start = end
			continue
		}
		if strings.ContainsRune(raw, '\t') || raw[0] == ' ' || raw[0] == '\t' || strings.HasPrefix(trimmed, "%") || trimmed == "---" || trimmed == "..." || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return nil, false, fmt.Errorf("%s:%d: complex YAML is unsupported", path, n)
		}
		colon := strings.IndexByte(raw, ':')
		if colon <= 0 {
			return nil, false, fmt.Errorf("%s:%d: expected flat YAML assignment", path, n)
		}
		if colon+1 < len(raw) && raw[colon+1] != ' ' {
			return nil, false, fmt.Errorf("%s:%d: YAML mapping colon must be followed by whitespace", path, n)
		}
		key := strings.TrimSpace(raw[:colon])
		if err := validateGlowPlainYAMLToken(key, true); err != nil {
			return nil, false, fmt.Errorf("%s:%d: quoted or complex YAML keys are unsupported", path, n)
		}
		lowerKey := strings.ToLower(key)
		if seenKeys[lowerKey] {
			return nil, false, fmt.Errorf("%s:%d: duplicate top-level YAML key %s", path, n, key)
		}
		seenKeys[lowerKey] = true
		if key == "<<" || strings.ContainsAny(key, "&*!|>") {
			return nil, false, fmt.Errorf("%s:%d: complex YAML feature is unsupported", path, n)
		}
		value, err := stripGlowYAMLComment(strings.TrimSpace(raw[colon+1:]))
		if err != nil {
			return nil, false, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		if value == "" {
			return nil, false, fmt.Errorf("%s:%d: nested or empty YAML values are unsupported", path, n)
		}
		if err := validateGlowFlatScalar(value); err != nil {
			return nil, false, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		if unquotedGlowYAMLHasSpecial(value) {
			return nil, false, fmt.Errorf("%s:%d: complex YAML feature is unsupported", path, n)
		}
		lines = append(lines, glowLine{start, end, logical, n, key, value, glowFieldID(key) != "", inManaged})
		start = end
	}
	return lines, len(starts) == 1, nil
}

func validateGlowFlatScalar(value string) error {
	if value == "" {
		return fmt.Errorf("empty YAML scalar is unsupported")
	}
	if value[0] == '"' {
		var decoded string
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			return fmt.Errorf("invalid double-quoted YAML scalar")
		}
		return nil
	}
	if value[0] == '\'' {
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return fmt.Errorf("invalid single-quoted YAML scalar")
		}
		inner := value[1 : len(value)-1]
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\'' {
				if i+1 >= len(inner) || inner[i+1] != '\'' {
					return fmt.Errorf("invalid single-quoted YAML scalar")
				}
				i++
			}
		}
		return nil
	}
	return validateGlowPlainYAMLToken(value, false)
}

func validateGlowPlainYAMLToken(value string, key bool) error {
	if value == "" {
		return fmt.Errorf("empty YAML token is unsupported")
	}
	if strings.ContainsRune("-?:%,[]{}@`", rune(value[0])) {
		return fmt.Errorf("reserved YAML indicator is unsupported")
	}
	if key {
		for _, r := range value {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
				return fmt.Errorf("complex YAML key is unsupported")
			}
		}
		return nil
	}
	lower := strings.ToLower(value)
	if lower == "null" || value == "~" {
		return fmt.Errorf("non-scalar YAML value is unsupported")
	}
	for i := 0; i < len(value); i++ {
		if value[i] == ':' && (i+1 == len(value) || value[i+1] == ' ') {
			return fmt.Errorf("nested YAML mapping syntax is unsupported")
		}
	}
	return nil
}

func unquotedGlowYAMLHasSpecial(value string) bool {
	if value == "" || value[0] == '"' || value[0] == '\'' {
		return false
	}
	return strings.ContainsAny(value, "&*!|>")
}

func stripGlowYAMLComment(value string) (string, error) {
	quote := byte(0)
	escaped := false
	for i := 0; i < len(value); i++ {
		c := value[i]
		if escaped {
			escaped = false
			continue
		}
		if quote == '"' && c == '\\' {
			escaped = true
			continue
		}
		switch c {
		case '\'', '"':
			switch quote {
			case 0:
				quote = c
			case c:
				quote = 0
			}
			continue
		}
		if c == '#' && quote == 0 && (i == 0 || value[i-1] == ' ') {
			return strings.TrimSpace(value[:i]), nil
		}
	}
	if quote != 0 {
		return "", fmt.Errorf("unterminated YAML quote")
	}
	return strings.TrimSpace(value), nil
}
func glowFieldID(key string) string {
	switch strings.ToLower(key) {
	case "style":
		return GlowFieldStyle
	case "mouse":
		return GlowFieldMouse
	case "pager":
		return GlowFieldPager
	case "width":
		return GlowFieldWidth
	case "all":
		return GlowFieldAll
	case "showlinenumbers":
		return GlowFieldShowLineNumbers
	case "preservenewlines":
		return GlowFieldPreserveNewLines
	}
	return ""
}
func parseGlowValue(id, value string) (any, error) {
	quoted := len(value) > 0 && (value[0] == '"' || value[0] == '\'')
	if len(value) >= 2 && value[0] == '"' {
		var s string
		if err := json.Unmarshal([]byte(value), &s); err != nil {
			return nil, fmt.Errorf("invalid JSON quoted scalar")
		}
		value = s
	} else if len(value) > 0 && value[0] == '\'' {
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return nil, fmt.Errorf("invalid single-quoted scalar")
		}
		inner := value[1 : len(value)-1]
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\'' {
				if i+1 >= len(inner) || inner[i+1] != '\'' {
					return nil, fmt.Errorf("invalid single-quoted scalar")
				}
				i++
			}
		}
		value = strings.ReplaceAll(inner, "''", "'")
	}
	switch id {
	case GlowFieldStyle:
		if !quoted && (strings.EqualFold(value, "null") || value == "~" || strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{") || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "?")) {
			return nil, fmt.Errorf("non-scalar Glow style is unsupported")
		}
		if err := validateGlowStyleSyntax(value); err != nil {
			return nil, err
		}
		return value, nil
	case GlowFieldWidth:
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid Glow width")
		}
		return n, nil
	default:
		switch strings.ToLower(value) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return nil, fmt.Errorf("invalid Glow boolean for %s", id)
	}
}

func generateGlowManagedSection(cfg GlowConfig) []byte {
	cfg = normalizeGlowConfig(cfg)
	pager := cfg.Pager != "never" && cfg.Pager != "none"
	style, _ := json.Marshal(cfg.Style)
	body := fmt.Sprintf("style: %s\nmouse: %t\npager: %t\nwidth: %d\nall: %t\nshowLineNumbers: %t\npreserveNewLines: %t\n", style, cfg.Mouse, pager, cfg.Width, cfg.All, cfg.ShowLineNumbers, cfg.PreserveNewLines)
	return wrapManagedConfigSection(glowManagedStart, glowManagedEnd, body)
}

func mergeGlowManagedSection(existing []byte, cfg GlowConfig) ([]byte, bool, error) {
	legacy := hasExactLegacyGlowHeader(existing)
	if legacy {
		existing = stripLegacyGlowProductLines(existing)
	}
	lines, managed, err := parseGlowYAMLLines("glow.yml", existing)
	if err != nil && len(bytes.TrimSpace(existing)) != 0 {
		return nil, false, err
	}
	section := generateGlowManagedSection(cfg)
	if managed {
		for _, line := range lines {
			if line.modeled && !line.managed {
				return nil, false, fmt.Errorf("refusing modeled Glow key %s outside managed block", line.key)
			}
		}
		return mergeManagedConfigSection(existing, section, glowManagedStart, glowManagedEnd, "glow.yml")
	}
	remove := map[int]bool{}
	for _, line := range lines {
		if line.modeled {
			remove[line.start] = true
		}
	}
	var kept []byte
	for _, line := range lines {
		_ = line
	}
	for start := 0; start < len(existing); {
		rel := bytes.IndexByte(existing[start:], '\n')
		end := len(existing)
		if rel >= 0 {
			end = start + rel + 1
		}
		if !remove[start] {
			kept = append(kept, existing[start:end]...)
		}
		start = end
	}
	return mergeManagedConfigSection(kept, section, glowManagedStart, glowManagedEnd, "glow.yml")
}

func hasExactLegacyGlowHeader(content []byte) bool {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	return len(lines) >= 2 && lines[0] == "# Generated by dotfiles TUI" && strings.HasPrefix(lines[1], "# Theme: ") && strings.TrimSpace(strings.TrimPrefix(lines[1], "# Theme: ")) != ""
}
func stripLegacyGlowProductLines(content []byte) []byte {
	type legacyLine struct {
		start, end int
		text       string
	}
	var lines []legacyLine
	for start := 0; start < len(content); {
		rel := bytes.IndexByte(content[start:], '\n')
		end := len(content)
		logical := end
		if rel >= 0 {
			logical = start + rel
			end = logical + 1
		}
		text := string(content[start:logical])
		text = strings.TrimSuffix(text, "\r")
		lines = append(lines, legacyLine{start, end, text})
		start = end
	}
	remove := map[int]bool{0: true, 1: true}
	if len(lines) > 2 && lines[2].text == "" {
		remove[2] = true
	}
	// Only the contiguous stanza emitted between pager/width and mouse by the
	// legacy generator is product-owned. Identical later user additions remain.
	for i := 3; i+2 < len(lines); i++ {
		if lines[i].text != "# Local mode (no cloud)" || lines[i+1].text != "local: true" {
			continue
		}
		prev := i - 1
		for prev >= 0 && lines[prev].text == "" {
			prev--
		}
		next := i + 2
		for next < len(lines) && lines[next].text == "" {
			next++
		}
		if prev >= 0 && (strings.HasPrefix(lines[prev].text, "pager: ") || strings.HasPrefix(lines[prev].text, "width: ")) && next < len(lines) && strings.HasPrefix(lines[next].text, "mouse: ") {
			remove[i] = true
			remove[i+1] = true
			break
		}
	}
	var out []byte
	for i, line := range lines {
		if !remove[i] {
			out = append(out, content[line.start:line.end]...)
		}
	}
	return out
}
