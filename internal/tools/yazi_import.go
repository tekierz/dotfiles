package tools

// Read-only Yazi TOML observation pinned to Yazi v26.5.6. This tranche does
// not connect the importer to any writer: it only resolves and observes the
// three independent global configuration files.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
	"github.com/tekierz/dotfiles/internal/safefile"
)

const (
	YaziCompatibilityVersion = "26.5.6"
	YaziFileMain             = "yazi.toml"
	YaziFileKeymap           = "keymap.toml"
	YaziFileTheme            = "theme.toml"
)

const (
	YaziFieldKeymap      = "mgr.keymap"
	YaziFieldShowHidden  = "mgr.show_hidden"
	YaziFieldPreviewMode = "preview.mode"
	YaziFieldSortBy      = "mgr.sort_by"
	YaziFieldSortReverse = "mgr.sort_reverse"
	YaziFieldLineMode    = "mgr.linemode"
	YaziFieldScrollOff   = "mgr.scrolloff"

	maxYaziTOMLBytes       = 1 << 20
	maxYaziTOMLLines       = 20_000
	maxYaziTOMLDepth       = 64
	maxYaziTOMLNodes       = 10_000
	maxYaziTOMLTables      = 4_096
	maxYaziTOMLArrayItems  = 10_000
	maxYaziTOMLStringBytes = 64 << 10
)

type YaziFileKind string

const (
	YaziFileKindMain   YaziFileKind = "main"
	YaziFileKindKeymap YaziFileKind = "keymap"
	YaziFileKindTheme  YaziFileKind = "theme"
)

type YaziConfigOrigin string

const (
	YaziConfigOriginOverride YaziConfigOrigin = "yazi-config-home"
	YaziConfigOriginXDG      YaziConfigOrigin = "xdg-config-home"
	YaziConfigOriginDefault  YaziConfigOrigin = "home-default"
)

type YaziFileOwnership string

const (
	YaziOwnershipMissing         YaziFileOwnership = "missing"
	YaziOwnershipExactCurrent    YaziFileOwnership = "exact-current"
	YaziOwnershipExactHistorical YaziFileOwnership = "exact-historical"
	YaziOwnershipNative          YaziFileOwnership = "native"
	YaziOwnershipMalformed       YaziFileOwnership = "malformed"
)

// YaziFileObservation keeps ownership and errors independent for each source.
// A malformed keymap must not discard valid observations from yazi.toml.
type YaziFileObservation struct {
	Kind           YaziFileKind
	Path           string
	Exists         bool
	Ownership      YaziFileOwnership
	External       bool
	ReadOnlyReason string
	Error          string
}

// YaziConfigImport is a read-only snapshot of Yazi's three global files.
type YaziConfigImport struct {
	Config YaziConfig
	Fields map[string]ConfigFieldProvenance
	Paths  YaziConfigPaths
	Main   YaziFileObservation
	Keymap YaziFileObservation
	Theme  YaziFileObservation
}

// YaziConfigPaths is the immutable three-file global source set used by Yazi.
type YaziConfigPaths struct {
	Origin YaziConfigOrigin
	Dir    string
	Main   string
	Keymap string
	Theme  string
}

// ResolveYaziConfigPaths mirrors Yazi v26.5.6 global config discovery.
// YAZI_CONFIG_HOME is the config directory itself, not an XDG-style parent.
func ResolveYaziConfigPaths() (YaziConfigPaths, error) {
	dir := os.Getenv("YAZI_CONFIG_HOME")
	origin := YaziConfigOriginOverride
	if dir != "" {
		if yaziPathHasUnsafeCharacters(dir) {
			return YaziConfigPaths{}, fmt.Errorf("YAZI_CONFIG_HOME contains forbidden control characters")
		}
		if !filepath.IsAbs(dir) {
			return YaziConfigPaths{}, fmt.Errorf("YAZI_CONFIG_HOME must be an absolute path")
		}
		dir = filepath.Clean(dir)
	} else if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		if yaziPathHasUnsafeCharacters(xdg) {
			return YaziConfigPaths{}, fmt.Errorf("XDG_CONFIG_HOME contains forbidden control characters")
		}
		if !filepath.IsAbs(xdg) {
			return YaziConfigPaths{}, fmt.Errorf("XDG_CONFIG_HOME must be an absolute path for Yazi config")
		}
		dir = filepath.Join(filepath.Clean(xdg), "yazi")
		origin = YaziConfigOriginXDG
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return YaziConfigPaths{}, fmt.Errorf("determine HOME for Yazi config: %w", err)
		}
		if home == "" {
			return YaziConfigPaths{}, fmt.Errorf("determine HOME for Yazi config: HOME is empty")
		}
		if !filepath.IsAbs(home) {
			return YaziConfigPaths{}, fmt.Errorf("HOME must be an absolute path for Yazi config")
		}
		if yaziPathHasUnsafeCharacters(home) {
			return YaziConfigPaths{}, fmt.Errorf("HOME contains forbidden control characters")
		}
		dir = filepath.Join(home, ".config", "yazi")
		origin = YaziConfigOriginDefault
	}
	return YaziConfigPaths{
		Origin: origin,
		Dir:    dir,
		Main:   filepath.Join(dir, YaziFileMain),
		Keymap: filepath.Join(dir, YaziFileKeymap),
		Theme:  filepath.Join(dir, YaziFileTheme),
	}, nil
}

func yaziPathHasUnsafeCharacters(path string) bool {
	for _, r := range path {
		if unicode.IsControl(r) || isBidiControl(r) {
			return true
		}
	}
	return false
}

// ImportYaziConfig observes each source independently. Resolver failures are
// returned; file-specific read or parse failures remain attached to that file
// so valid siblings are still available to the dashboard.
func ImportYaziConfig() (YaziConfigImport, error) {
	paths, err := ResolveYaziConfigPaths()
	if err != nil {
		return YaziConfigImport{}, err
	}
	result := YaziConfigImport{
		Config: defaultImportedYaziConfig(),
		Fields: map[string]ConfigFieldProvenance{},
		Paths:  paths,
	}
	result.Main, result.Config, result.Fields = observeYaziMain(paths.Main, result.Config, result.Fields)
	result.Keymap, result.Config, result.Fields = observeYaziKeymap(paths.Keymap, result.Config, result.Fields)
	result.Theme = observeYaziTheme(paths.Theme)
	return result, nil
}

func defaultImportedYaziConfig() YaziConfig {
	return YaziConfig{Keymap: "vim", ShowHidden: false, PreviewMode: "auto", SortBy: "alphabetical", SortReverse: false, LineMode: "none", ScrollOff: 5}
}

func observeYaziMain(path string, cfg YaziConfig, fields map[string]ConfigFieldProvenance) (YaziFileObservation, YaziConfig, map[string]ConfigFieldProvenance) {
	observation, content := readYaziObservation(YaziFileKindMain, path)
	if !observation.Exists || observation.Error != "" {
		return observation, cfg, fields
	}
	parsed, provenance, err := parseYaziMain(path, content, cfg)
	if err != nil {
		return malformedYaziObservation(observation, err), cfg, fields
	}
	if legacy, legacyProvenance, ok := inspectExactHistoricalYaziMain(path, content, cfg); ok {
		parsed = legacy
		provenance = legacyProvenance
		observation.Ownership = YaziOwnershipExactHistorical
	} else {
		observation.Ownership = classifyYaziMain(content, parsed)
	}
	if observation.Ownership == YaziOwnershipExactHistorical && parsed.PreviewMode == "custom" {
		parsed.PreviewMode = "never"
	}
	finalizeYaziObservation(&observation)
	for key, value := range provenance {
		value.Scope = yaziProvenanceScope(observation.Ownership)
		fields[key] = value
	}
	return observation, parsed, fields
}

func observeYaziKeymap(path string, cfg YaziConfig, fields map[string]ConfigFieldProvenance) (YaziFileObservation, YaziConfig, map[string]ConfigFieldProvenance) {
	observation, content := readYaziObservation(YaziFileKindKeymap, path)
	if !observation.Exists || observation.Error != "" {
		return observation, cfg, fields
	}
	keymap, provenance, err := parseYaziKeymap(path, content, cfg.Keymap)
	if err != nil {
		return malformedYaziObservation(observation, err), cfg, fields
	}
	candidate := cfg
	candidate.Keymap = keymap
	if legacyKeymap, legacyProvenance, ok := inspectExactHistoricalYaziKeymap(path, content, cfg.Keymap); ok {
		candidate.Keymap = legacyKeymap
		provenance = legacyProvenance
		observation.Ownership = YaziOwnershipExactHistorical
	} else {
		observation.Ownership = classifyYaziKeymap(content, candidate)
	}
	if observation.Ownership == YaziOwnershipExactCurrent {
		if style, ok := currentYaziKeymapStyle(content); ok {
			candidate.Keymap = style
			if style == "vim" && provenance.Key == "" {
				provenance = ConfigFieldProvenance{Path: path, Line: 3, Key: "header.keymap-style"}
			}
		}
	}
	finalizeYaziObservation(&observation)
	if provenance.Key != "" {
		provenance.Scope = yaziProvenanceScope(observation.Ownership)
		fields[YaziFieldKeymap] = provenance
	}
	return observation, candidate, fields
}

func observeYaziTheme(path string) YaziFileObservation {
	observation, content := readYaziObservation(YaziFileKindTheme, path)
	if !observation.Exists || observation.Error != "" {
		return observation
	}
	doc, err := parseYaziTOML(path, content)
	if err != nil {
		return malformedYaziObservation(observation, err)
	}
	if err := validateYaziTableTypes(path, doc, "flavor", "mgr", "manager", "tabs", "indicator", "mode", "status", "input", "pick", "select", "tasks", "which", "help", "filetype"); err != nil {
		return malformedYaziObservation(observation, err)
	}
	observation.Ownership = classifyYaziTheme(content)
	finalizeYaziObservation(&observation)
	if observation.Ownership == YaziOwnershipNative {
		if _, flavored := doc["flavor"]; flavored {
			observation.ReadOnlyReason = "native Yazi flavor selection is read-only in this release"
		} else {
			observation.ReadOnlyReason = "custom native Yazi theme is read-only in this release"
		}
	}
	return observation
}

func readYaziObservation(kind YaziFileKind, path string) (YaziFileObservation, []byte) {
	observation := YaziFileObservation{Kind: kind, Path: path, Ownership: YaziOwnershipMissing, External: yaziPathExternal(path)}
	if !observation.External {
		home, err := os.UserHomeDir()
		if err != nil {
			observation.Ownership = YaziOwnershipMalformed
			observation.Error = fmt.Sprintf("determine HOME for Yazi %s: %v", kind, err)
			finalizeYaziObservation(&observation)
			return observation, nil
		}
		if err := rejectYaziAncestorSymlinks(home, path); err != nil {
			observation.Ownership = YaziOwnershipMalformed
			observation.Error = fmt.Sprintf("inspect Yazi %s path: %v", kind, err)
			finalizeYaziObservation(&observation)
			return observation, nil
		}
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		finalizeYaziObservation(&observation)
		return observation, nil
	}
	observation.Exists = true
	if err != nil {
		observation.Ownership = YaziOwnershipMalformed
		observation.Error = fmt.Sprintf("inspect Yazi %s: %v", kind, err)
		finalizeYaziObservation(&observation)
		return observation, nil
	}
	if !info.Mode().IsRegular() {
		observation.Ownership = YaziOwnershipMalformed
		observation.Error = fmt.Sprintf("Yazi %s source is not a readable regular file", kind)
		finalizeYaziObservation(&observation)
		return observation, nil
	}
	if info.Size() > maxYaziTOMLBytes {
		observation.Ownership = YaziOwnershipMalformed
		observation.Error = fmt.Sprintf("Yazi %s exceeds %d-byte safety limit", kind, maxYaziTOMLBytes)
		finalizeYaziObservation(&observation)
		return observation, nil
	}
	if observation.External {
		root, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil {
			observation.Ownership = YaziOwnershipMalformed
			observation.Error = fmt.Sprintf("resolve external Yazi %s directory: %v", kind, err)
			finalizeYaziObservation(&observation)
			return observation, nil
		}
		content, revision, err := safefile.ReadWithinLimit(root, filepath.Base(path), maxYaziTOMLBytes)
		if err != nil || !revision.Exists() {
			observation.Ownership = YaziOwnershipMalformed
			observation.Error = fmt.Sprintf("read external Yazi %s safely: %v", kind, err)
			finalizeYaziObservation(&observation)
			return observation, nil
		}
		if revision.LinkCount() != 1 {
			observation.Ownership = YaziOwnershipMalformed
			observation.Error = fmt.Sprintf("external Yazi %s source has %d hardlinks", kind, revision.LinkCount())
			finalizeYaziObservation(&observation)
			return observation, nil
		}
		return observation, content
	}
	home, err := os.UserHomeDir()
	if err != nil {
		observation.Ownership = YaziOwnershipMalformed
		observation.Error = fmt.Sprintf("determine HOME for Yazi %s: %v", kind, err)
		finalizeYaziObservation(&observation)
		return observation, nil
	}
	rel, err := filepath.Rel(home, path)
	if err != nil {
		observation.Ownership = YaziOwnershipMalformed
		observation.Error = fmt.Sprintf("resolve Yazi %s below HOME: %v", kind, err)
		finalizeYaziObservation(&observation)
		return observation, nil
	}
	content, revision, err := safefile.ReadWithinLimit(home, rel, maxYaziTOMLBytes)
	observation.Exists = revision.Exists() || observation.Exists
	if err != nil {
		observation.Ownership = YaziOwnershipMalformed
		observation.Error = fmt.Sprintf("read Yazi %s: %v", kind, err)
	} else if revision.Exists() && revision.LinkCount() != 1 {
		observation.Ownership = YaziOwnershipMalformed
		observation.Error = fmt.Sprintf("Yazi %s source has %d hardlinks", kind, revision.LinkCount())
	} else if len(content) > maxYaziTOMLBytes {
		observation.Ownership = YaziOwnershipMalformed
		observation.Error = fmt.Sprintf("Yazi %s exceeds %d-byte safety limit", kind, maxYaziTOMLBytes)
	}
	finalizeYaziObservation(&observation)
	return observation, content
}

func rejectYaziAncestorSymlinks(home, path string) error {
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path is not contained by HOME")
	}
	parts := strings.Split(rel, string(filepath.Separator))
	current := home
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("ancestor %q is a symlink", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("ancestor %q is not a directory", current)
		}
	}
	return nil
}

func yaziPathExternal(path string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !filepath.IsAbs(home) || yaziPathHasUnsafeCharacters(home) {
		return true
	}
	return !pathWithinHome(home, path)
}

func malformedYaziObservation(observation YaziFileObservation, err error) YaziFileObservation {
	observation.Ownership = YaziOwnershipMalformed
	observation.Error = err.Error()
	finalizeYaziObservation(&observation)
	return observation
}

func finalizeYaziObservation(observation *YaziFileObservation) {
	if observation.External {
		observation.ReadOnlyReason = "active Yazi config is outside HOME or HOME containment cannot be verified"
		return
	}
	switch observation.Ownership {
	case YaziOwnershipMissing, YaziOwnershipExactCurrent:
		observation.ReadOnlyReason = ""
	case YaziOwnershipExactHistorical:
		observation.ReadOnlyReason = "exact historical dotfiles Yazi config is read-only until explicitly migrated"
	case YaziOwnershipNative:
		observation.ReadOnlyReason = "arbitrary native Yazi TOML is read-only in this release"
	case YaziOwnershipMalformed:
		observation.ReadOnlyReason = "malformed or unreadable Yazi TOML is read-only"
	}
}

func yaziProvenanceScope(ownership YaziFileOwnership) ConfigValueScope {
	if ownership == YaziOwnershipExactCurrent || ownership == YaziOwnershipExactHistorical {
		return ConfigValueManaged
	}
	return ConfigValueNative
}

func parseYaziMain(path string, content []byte, defaults YaziConfig) (YaziConfig, map[string]ConfigFieldProvenance, error) {
	doc, err := parseYaziTOML(path, content)
	if err != nil {
		return defaults, nil, err
	}
	if err := validateYaziTableTypes(path, doc, "mgr", "manager", "preview", "plugin"); err != nil {
		return defaults, nil, err
	}
	lines, err := yaziTOMLKeyLines(content)
	if err != nil {
		return defaults, nil, fmt.Errorf("%s: %w", path, err)
	}
	cfg := defaults
	fields := map[string]ConfigFieldProvenance{}
	mgrName := "mgr"
	mgr, _ := doc[mgrName].(map[string]any)
	if value, ok := mgr["show_hidden"]; ok {
		parsed, ok := value.(bool)
		if !ok {
			return defaults, nil, fmt.Errorf("%s: mgr.show_hidden must be a boolean", path)
		}
		cfg.ShowHidden = parsed
		fields[YaziFieldShowHidden] = yaziProvenance(path, lines, mgrName+".show_hidden")
	}
	if value, ok := mgr["sort_by"]; ok {
		parsed, ok := value.(string)
		if !ok {
			return defaults, nil, fmt.Errorf("%s: mgr.sort_by must be a string", path)
		}
		cfg.SortBy = map[string]string{"mtime": "modified"}[parsed]
		if cfg.SortBy == "" {
			cfg.SortBy = parsed
		}
		fields[YaziFieldSortBy] = yaziProvenance(path, lines, mgrName+".sort_by")
	}
	if value, ok := mgr["sort_reverse"]; ok {
		parsed, ok := value.(bool)
		if !ok {
			return defaults, nil, fmt.Errorf("%s: mgr.sort_reverse must be a boolean", path)
		}
		cfg.SortReverse = parsed
		fields[YaziFieldSortReverse] = yaziProvenance(path, lines, mgrName+".sort_reverse")
	}
	if value, ok := mgr["linemode"]; ok {
		parsed, ok := value.(string)
		if !ok {
			return defaults, nil, fmt.Errorf("%s: mgr.linemode must be a string", path)
		}
		cfg.LineMode = parsed
		fields[YaziFieldLineMode] = yaziProvenance(path, lines, mgrName+".linemode")
	}
	if value, ok := mgr["scrolloff"]; ok {
		parsed, ok := value.(int64)
		if !ok || parsed < 0 || parsed > 100_000 {
			return defaults, nil, fmt.Errorf("%s: mgr.scrolloff must be an integer from 0 to 100000", path)
		}
		cfg.ScrollOff = int(parsed)
		fields[YaziFieldScrollOff] = yaziProvenance(path, lines, mgrName+".scrolloff")
	}
	preview, _ := doc["preview"].(map[string]any)
	plugin, _ := doc["plugin"].(map[string]any)
	for _, key := range []string{"previewers", "preloaders", "prepend_previewers", "append_previewers", "prepend_preloaders", "append_preloaders"} {
		if value, exists := plugin[key]; exists {
			if _, ok := value.([]any); !ok {
				return defaults, nil, fmt.Errorf("%s: plugin.%s must be an array", path, key)
			}
		}
	}
	previewers, hasPreviewers, err := emptyYaziArray(plugin["previewers"])
	if err != nil {
		return defaults, nil, fmt.Errorf("%s: plugin.previewers must be an array", path)
	}
	preloaders, hasPreloaders, err := emptyYaziArray(plugin["preloaders"])
	if err != nil {
		return defaults, nil, fmt.Errorf("%s: plugin.preloaders must be an array", path)
	}
	_, hasPrependPreviewers := plugin["prepend_previewers"]
	_, hasAppendPreviewers := plugin["append_previewers"]
	_, hasPrependPreloaders := plugin["prepend_preloaders"]
	_, hasAppendPreloaders := plugin["append_preloaders"]
	hasMixingOverrides := hasPrependPreviewers || hasAppendPreviewers || hasPrependPreloaders || hasAppendPreloaders
	hasPluginOverrides := hasPreviewers || hasPreloaders || hasMixingOverrides
	switch {
	case hasPreviewers && previewers && hasPreloaders && preloaders && !hasMixingOverrides:
		cfg.PreviewMode = "never"
		fields[YaziFieldPreviewMode] = yaziProvenance(path, lines, "plugin.previewers")
	case hasPluginOverrides:
		cfg.PreviewMode = "custom"
		fields[YaziFieldPreviewMode] = yaziProvenance(path, lines, firstYaziPreviewOverrideKey(plugin, lines))
	case preview["image_delay"] == int64(0):
		cfg.PreviewMode = "always"
		fields[YaziFieldPreviewMode] = yaziProvenance(path, lines, "preview.image_delay")
	case preview["image_delay"] != nil:
		value, ok := preview["image_delay"].(int64)
		if !ok || value < 0 {
			return defaults, nil, fmt.Errorf("%s: preview.image_delay must be a non-negative integer", path)
		}
		cfg.PreviewMode = "auto"
		fields[YaziFieldPreviewMode] = yaziProvenance(path, lines, "preview.image_delay")
	}
	return cfg, fields, nil
}

func firstYaziPreviewOverrideKey(plugin map[string]any, lines map[string]int) string {
	bestKey, bestLine := "plugin.previewers", int(^uint(0)>>1)
	for _, name := range []string{"previewers", "preloaders", "prepend_previewers", "append_previewers", "prepend_preloaders", "append_preloaders"} {
		if _, exists := plugin[name]; !exists {
			continue
		}
		key := "plugin." + name
		line := lines[key]
		if line > 0 && line < bestLine {
			bestKey, bestLine = key, line
		}
	}
	return bestKey
}

func emptyYaziArray(value any) (empty, present bool, err error) {
	if value == nil {
		return false, false, nil
	}
	items, ok := value.([]any)
	if !ok {
		return false, true, errors.New("not an array")
	}
	return len(items) == 0, true, nil
}

func parseYaziKeymap(path string, content []byte, defaultKeymap string) (string, ConfigFieldProvenance, error) {
	doc, err := parseYaziTOML(path, content)
	if err != nil {
		return "", ConfigFieldProvenance{}, err
	}
	if err := validateYaziTableTypes(path, doc, "mgr", "manager"); err != nil {
		return "", ConfigFieldProvenance{}, err
	}
	lines, err := yaziTOMLKeyLines(content)
	if err != nil {
		return "", ConfigFieldProvenance{}, fmt.Errorf("%s: %w", path, err)
	}
	mgrName := "mgr"
	mgr, _ := doc[mgrName].(map[string]any)
	seenVim, seenEmacs := false, false
	var provenanceKey string
	for _, key := range []string{"keymap", "prepend_keymap", "append_keymap"} {
		raw, exists := mgr[key]
		if !exists {
			continue
		}
		if provenanceKey == "" {
			provenanceKey = mgrName + "." + key
		}
		entries, ok := raw.([]any)
		if !ok {
			return "", ConfigFieldProvenance{}, fmt.Errorf("%s: %s.%s must be an array", path, mgrName, key)
		}
		for index, entry := range entries {
			binding, ok := entry.(map[string]any)
			if !ok {
				return "", ConfigFieldProvenance{}, fmt.Errorf("%s: %s.%s[%d] must be a table", path, mgrName, key, index)
			}
			onValues, err := validateYaziBinding(binding, path, mgrName+"."+key, index)
			if err != nil {
				return "", ConfigFieldProvenance{}, err
			}
			for _, on := range onValues {
				seenVim = seenVim || on == "j" || on == "k"
				seenEmacs = seenEmacs || on == "<C-n>" || on == "<C-p>"
			}
		}
	}
	keymap := "custom"
	if seenVim && !seenEmacs {
		keymap = "vim"
	} else if seenEmacs && !seenVim {
		keymap = "emacs"
	}
	if provenanceKey == "" {
		return defaultKeymap, ConfigFieldProvenance{}, nil
	}
	return keymap, yaziProvenance(path, lines, provenanceKey), nil
}

func inspectExactHistoricalYaziMain(path string, content []byte, defaults YaziConfig) (YaziConfig, map[string]ConfigFieldProvenance, bool) {
	const legacyHeader = "[manager]\n"
	if !bytes.Contains(content, []byte(legacyHeader)) {
		return defaults, nil, false
	}
	modern := bytes.Replace(content, []byte(legacyHeader), []byte("[mgr]\n"), 1)
	parsed, provenance, err := parseYaziMain(path, modern, defaults)
	if err != nil {
		return defaults, nil, false
	}
	for theme := range yaziThemePalettes {
		for _, candidate := range yaziHistoricalPreviewCandidates(parsed) {
			if bytes.Equal(content, []byte(generateHistoricalYaziMain(candidate, theme))) {
				for fieldID, field := range provenance {
					field.Key = strings.Replace(field.Key, "mgr.", "manager.", 1)
					provenance[fieldID] = field
				}
				return candidate, provenance, true
			}
		}
	}
	return defaults, nil, false
}

func inspectExactHistoricalYaziKeymap(path string, content []byte, defaultKeymap string) (string, ConfigFieldProvenance, bool) {
	const legacyHeader = "[manager]\n"
	if !bytes.Contains(content, []byte(legacyHeader)) {
		return defaultKeymap, ConfigFieldProvenance{}, false
	}
	modern := bytes.Replace(content, []byte(legacyHeader), []byte("[mgr]\n"), 1)
	keymap, provenance, err := parseYaziKeymap(path, modern, defaultKeymap)
	if err != nil || (keymap != "vim" && keymap != "emacs") {
		return defaultKeymap, ConfigFieldProvenance{}, false
	}
	cfg := YaziConfig{Keymap: keymap}
	if !bytes.Equal(content, []byte(generateHistoricalYaziKeymap(cfg))) {
		return defaultKeymap, ConfigFieldProvenance{}, false
	}
	provenance.Key = strings.Replace(provenance.Key, "mgr.", "manager.", 1)
	return keymap, provenance, true
}

func validateYaziBinding(binding map[string]any, path, key string, index int) ([]string, error) {
	var onValues []string
	if raw, exists := binding["on"]; exists {
		switch typed := raw.(type) {
		case string:
			onValues = []string{typed}
		case []any:
			for _, value := range typed {
				item, ok := value.(string)
				if !ok {
					return nil, fmt.Errorf("%s: %s[%d].on array values must be strings", path, key, index)
				}
				onValues = append(onValues, item)
			}
		default:
			return nil, fmt.Errorf("%s: %s[%d].on must be a string or array of strings", path, key, index)
		}
	}
	if raw, exists := binding["run"]; exists {
		if err := validateYaziStringOrStringArray(raw); err != nil {
			return nil, fmt.Errorf("%s: %s[%d].run must be a string or array of strings", path, key, index)
		}
	}
	if raw, exists := binding["desc"]; exists {
		if _, ok := raw.(string); !ok {
			return nil, fmt.Errorf("%s: %s[%d].desc must be a string", path, key, index)
		}
	}
	return onValues, nil
}

func validateYaziStringOrStringArray(value any) error {
	switch typed := value.(type) {
	case string:
		return nil
	case []any:
		for _, item := range typed {
			if _, ok := item.(string); !ok {
				return errors.New("array contains a non-string value")
			}
		}
		return nil
	default:
		return errors.New("value is neither string nor array")
	}
}

func parseYaziTOML(path string, content []byte) (map[string]any, error) {
	if len(content) > maxYaziTOMLBytes {
		return nil, fmt.Errorf("%s: TOML exceeds %d-byte safety limit", path, maxYaziTOMLBytes)
	}
	if !utf8.Valid(content) || bytes.HasPrefix(content, []byte{0xef, 0xbb, 0xbf}) {
		return nil, fmt.Errorf("%s: invalid UTF-8 or BOM in Yazi TOML", path)
	}
	if bytes.Count(content, []byte{'\n'})+1 > maxYaziTOMLLines {
		return nil, fmt.Errorf("%s: TOML exceeds %d-line safety limit", path, maxYaziTOMLLines)
	}
	for _, r := range string(content) {
		if (unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t') || isBidiControl(r) {
			return nil, fmt.Errorf("%s: forbidden control character in Yazi TOML", path)
		}
	}
	if err := preflightYaziTOMLShape(path, content); err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := toml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("%s: invalid or ambiguous Yazi TOML: %w", path, err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	if err := validateYaziTOMLResources(path, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// preflightYaziTOMLShape uses the streaming parser to reject structurally
// excessive input before the general TOML decoder constructs a document.
func preflightYaziTOMLShape(path string, content []byte) error {
	var parser unstable.Parser
	parser.Reset(content)
	nodes, tables := 0, 0
	for parser.NextExpression() {
		expression := parser.Expression()
		nodes++
		if nodes > maxYaziTOMLNodes {
			return fmt.Errorf("%s: TOML exceeds %d-node safety limit", path, maxYaziTOMLNodes)
		}
		if expression.Kind == unstable.Table || expression.Kind == unstable.ArrayTable {
			tables++
			if tables > maxYaziTOMLTables {
				return fmt.Errorf("%s: TOML exceeds %d-table safety limit", path, maxYaziTOMLTables)
			}
		}
		if len(yaziNodeKey(expression))+1 > maxYaziTOMLDepth {
			return fmt.Errorf("%s: TOML exceeds %d-level nesting limit", path, maxYaziTOMLDepth)
		}
		if expression.Kind == unstable.KeyValue {
			if err := preflightYaziTOMLValue(path, expression.Value(), 1, &nodes, &tables); err != nil {
				return err
			}
		}
	}
	if err := parser.Error(); err != nil {
		return fmt.Errorf("%s: invalid Yazi TOML preflight: %w", path, err)
	}
	return nil
}

func preflightYaziTOMLValue(path string, node *unstable.Node, depth int, nodes, tables *int) error {
	if depth > maxYaziTOMLDepth {
		return fmt.Errorf("%s: TOML exceeds %d-level nesting limit", path, maxYaziTOMLDepth)
	}
	*nodes++
	if *nodes > maxYaziTOMLNodes {
		return fmt.Errorf("%s: TOML exceeds %d-node safety limit", path, maxYaziTOMLNodes)
	}
	switch node.Kind {
	case unstable.String:
		if len(node.Data) > maxYaziTOMLStringBytes {
			return fmt.Errorf("%s: TOML string exceeds %d-byte safety limit", path, maxYaziTOMLStringBytes)
		}
	case unstable.Array:
		children := node.Children()
		items := 0
		for children.Next() {
			items++
			if items > maxYaziTOMLArrayItems {
				return fmt.Errorf("%s: TOML array exceeds %d-item safety limit", path, maxYaziTOMLArrayItems)
			}
			if err := preflightYaziTOMLValue(path, children.Node(), depth+1, nodes, tables); err != nil {
				return err
			}
		}
	case unstable.InlineTable:
		*tables++
		if *tables > maxYaziTOMLTables {
			return fmt.Errorf("%s: TOML exceeds %d-table safety limit", path, maxYaziTOMLTables)
		}
		children := node.Children()
		entries := 0
		for children.Next() {
			entries++
			if entries > maxYaziTOMLArrayItems {
				return fmt.Errorf("%s: TOML inline table exceeds %d-entry safety limit", path, maxYaziTOMLArrayItems)
			}
			entry := children.Node()
			if entry.Kind != unstable.KeyValue {
				return fmt.Errorf("%s: malformed inline TOML table", path)
			}
			if depth+len(yaziNodeKey(entry))+1 > maxYaziTOMLDepth {
				return fmt.Errorf("%s: TOML exceeds %d-level nesting limit", path, maxYaziTOMLDepth)
			}
			if err := preflightYaziTOMLValue(path, entry.Value(), depth+1, nodes, tables); err != nil {
				return err
			}
		}
	case unstable.Invalid, unstable.Comment, unstable.Key, unstable.Table, unstable.ArrayTable, unstable.KeyValue,
		unstable.Bool, unstable.Float, unstable.Integer, unstable.LocalDate, unstable.LocalTime, unstable.LocalDateTime, unstable.DateTime:
		// Scalar kinds other than String have no nested resource shape.
	}
	return nil
}

func validateYaziTableTypes(path string, doc map[string]any, names ...string) error {
	for _, name := range names {
		value, exists := doc[name]
		if !exists {
			continue
		}
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("%s: %s must be a table", path, name)
		}
	}
	return nil
}

type yaziTOMLResourceCount struct {
	nodes  int
	tables int
}

func validateYaziTOMLResources(path string, doc map[string]any) error {
	count := yaziTOMLResourceCount{tables: 1}
	if err := walkYaziTOMLResources(doc, 1, &count); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func walkYaziTOMLResources(value any, depth int, count *yaziTOMLResourceCount) error {
	if depth > maxYaziTOMLDepth {
		return fmt.Errorf("TOML exceeds %d-level nesting limit", maxYaziTOMLDepth)
	}
	count.nodes++
	if count.nodes > maxYaziTOMLNodes {
		return fmt.Errorf("TOML exceeds %d-node safety limit", maxYaziTOMLNodes)
	}
	switch typed := value.(type) {
	case map[string]any:
		if depth > 1 {
			count.tables++
			if count.tables > maxYaziTOMLTables {
				return fmt.Errorf("TOML exceeds %d-table safety limit", maxYaziTOMLTables)
			}
		}
		for key, child := range typed {
			if len(key) > maxYaziTOMLStringBytes {
				return fmt.Errorf("TOML key exceeds %d-byte safety limit", maxYaziTOMLStringBytes)
			}
			if err := walkYaziTOMLResources(child, depth+1, count); err != nil {
				return err
			}
		}
	case []any:
		if len(typed) > maxYaziTOMLArrayItems {
			return fmt.Errorf("TOML array exceeds %d-item safety limit", maxYaziTOMLArrayItems)
		}
		for _, child := range typed {
			if err := walkYaziTOMLResources(child, depth+1, count); err != nil {
				return err
			}
		}
	case string:
		if len(typed) > maxYaziTOMLStringBytes {
			return fmt.Errorf("TOML string exceeds %d-byte safety limit", maxYaziTOMLStringBytes)
		}
	}
	return nil
}

func yaziTOMLKeyLines(content []byte) (map[string]int, error) {
	result := map[string]int{}
	var parser unstable.Parser
	parser.Reset(content)
	var table []string
	for parser.NextExpression() {
		expression := parser.Expression()
		switch expression.Kind {
		case unstable.Table, unstable.ArrayTable:
			table = yaziNodeKey(expression)
		case unstable.KeyValue:
			key := append(append([]string(nil), table...), yaziNodeKey(expression)...)
			result[strings.Join(key, ".")] = parser.Shape(expression.Raw).Start.Line
		case unstable.Invalid, unstable.Comment, unstable.Key, unstable.Array, unstable.InlineTable, unstable.String,
			unstable.Bool, unstable.Float, unstable.Integer, unstable.LocalDate, unstable.LocalTime, unstable.LocalDateTime, unstable.DateTime:
			// Comments and other expression kinds do not contribute key lines.
		}
	}
	if err := parser.Error(); err != nil {
		return nil, err
	}
	return result, nil
}

func yaziNodeKey(node *unstable.Node) []string {
	var result []string
	iterator := node.Key()
	for iterator.Next() {
		result = append(result, string(iterator.Node().Data))
	}
	return result
}

func yaziProvenance(path string, lines map[string]int, key string) ConfigFieldProvenance {
	return ConfigFieldProvenance{Path: path, Line: lines[key], Key: key}
}

func classifyYaziMain(content []byte, cfg YaziConfig) YaziFileOwnership {
	for theme := range yaziThemePalettes {
		if bytes.Equal(content, []byte(generateCurrentYaziMain(cfg, theme))) {
			return YaziOwnershipExactCurrent
		}
		for _, candidate := range yaziHistoricalPreviewCandidates(cfg) {
			if bytes.Equal(content, []byte(GenerateYaziConfig(candidate, theme))) || bytes.Equal(content, []byte(generateHistoricalYaziMain(candidate, theme))) {
				return YaziOwnershipExactHistorical
			}
		}
	}
	return YaziOwnershipNative
}

func generateCurrentYaziMain(cfg YaziConfig, theme string) string {
	var content strings.Builder
	content.WriteString("# Generated by dotfiles TUI\n")
	content.WriteString("# Compatibility: Yazi 26.5.6\n\n[mgr]\n")
	fmt.Fprintf(&content, "sort_by = %q\n", yaziSortBy(cfg.SortBy))
	fmt.Fprintf(&content, "sort_reverse = %t\n", cfg.SortReverse)
	fmt.Fprintf(&content, "linemode = %q\n", yaziLineMode(cfg.LineMode))
	fmt.Fprintf(&content, "scrolloff = %d\n", cfg.ScrollOff)
	fmt.Fprintf(&content, "show_hidden = %t\n\n[preview]\n", cfg.ShowHidden)
	fmt.Fprintf(&content, "image_delay = %d\n", yaziPreviewImageDelay(cfg.PreviewMode))
	if yaziPreviewMode(cfg.PreviewMode) == "never" {
		content.WriteString("\n[plugin]\npreviewers = []\npreloaders = []\n")
	}
	return content.String()
}

func yaziHistoricalPreviewCandidates(cfg YaziConfig) []YaziConfig {
	candidates := []YaziConfig{cfg}
	if cfg.PreviewMode == "custom" {
		legacyNever := cfg
		legacyNever.PreviewMode = "never"
		candidates = append(candidates, legacyNever)
	}
	return candidates
}

func classifyYaziKeymap(content []byte, cfg YaziConfig) YaziFileOwnership {
	if _, ok := currentYaziKeymapStyle(content); ok {
		return YaziOwnershipExactCurrent
	}
	if (cfg.Keymap == "vim" || cfg.Keymap == "emacs") && (bytes.Equal(content, []byte(GenerateYaziKeymap(cfg, ""))) || bytes.Equal(content, []byte(generateHistoricalYaziKeymap(cfg)))) {
		return YaziOwnershipExactHistorical
	}
	return YaziOwnershipNative
}

func currentYaziKeymapStyle(content []byte) (string, bool) {
	for _, style := range []string{"vim", "emacs"} {
		if bytes.Equal(content, []byte(generateCurrentYaziKeymap(style))) {
			return style, true
		}
	}
	return "", false
}

func generateCurrentYaziKeymap(style string) string {
	header := "# Generated by dotfiles TUI\n# Compatibility: Yazi 26.5.6\n# Keymap style: " + style + "\n"
	if style == "vim" {
		return header
	}
	return header + "\n[mgr]\nprepend_keymap = [\n" +
		"  { on = \"<C-p>\", run = \"arrow -1\", desc = \"Move up\" },\n" +
		"  { on = \"<C-n>\", run = \"arrow 1\", desc = \"Move down\" },\n" +
		"  { on = \"<C-b>\", run = \"leave\", desc = \"Go to parent\" },\n" +
		"  { on = \"<C-f>\", run = \"enter\", desc = \"Enter directory\" },\n" +
		"  { on = \"<Space>\", run = \"toggle\", desc = \"Toggle selection\" },\n" +
		"]\n"
}

func classifyYaziTheme(content []byte) YaziFileOwnership {
	for theme := range yaziThemePalettes {
		current := generateCurrentYaziTheme(theme)
		if bytes.Equal(content, []byte(current)) {
			return YaziOwnershipExactCurrent
		}
		if bytes.Equal(content, []byte(GenerateYaziTheme(theme))) || bytes.Equal(content, []byte(generateHistoricalYaziTheme(theme))) {
			return YaziOwnershipExactHistorical
		}
	}
	return YaziOwnershipNative
}

func generateCurrentYaziTheme(theme string) string {
	old := GenerateYaziTheme(theme)
	const header = "# Generated by dotfiles TUI\n"
	if !strings.HasPrefix(old, header) {
		return ""
	}
	return header + "# Compatibility: Yazi 26.5.6\n" + strings.TrimPrefix(old, header)
}

// These frozen generators reproduce the exact pre-bb6de48 product bytes. They
// are recognition-only seams and are deliberately not connected to writers.
func generateHistoricalYaziMain(cfg YaziConfig, theme string) string {
	var sb strings.Builder
	sb.WriteString("# Generated by dotfiles TUI\n")
	sb.WriteString(fmt.Sprintf("# Theme: %s\n\n", theme))
	sb.WriteString("[manager]\nratio = [1, 4, 3]\n")
	sb.WriteString(fmt.Sprintf("sort_by = %q\n", yaziSortBy(cfg.SortBy)))
	sb.WriteString("sort_sensitive = false\n")
	sb.WriteString(fmt.Sprintf("sort_reverse = %t\n", cfg.SortReverse))
	sb.WriteString("sort_dir_first = true\n")
	sb.WriteString(fmt.Sprintf("linemode = %q\n", cfg.LineMode))
	sb.WriteString(fmt.Sprintf("scrolloff = %d\n", cfg.ScrollOff))
	sb.WriteString(fmt.Sprintf("show_hidden = %t\n", cfg.ShowHidden))
	sb.WriteString("show_symlink = true\n\n[preview]\n")
	sb.WriteString(fmt.Sprintf("# Preview mode: %s\n", yaziPreviewMode(cfg.PreviewMode)))
	sb.WriteString("# yazi has no single preview toggle; image_delay controls eager image rendering,\n")
	sb.WriteString("# and PreviewMode=never disables previewer plugins in the [plugin] section below.\n")
	sb.WriteString(fmt.Sprintf("image_delay = %d\n", yaziPreviewImageDelay(cfg.PreviewMode)))
	sb.WriteString("tab_size = 2\nmax_width = 600\nmax_height = 900\ncache_dir = \"\"\nimage_filter = \"triangle\"\nimage_quality = 75\nueberzug_scale = 1\nueberzug_offset = [0, 0, 0, 0]\n\n")
	sb.WriteString("[opener]\nedit = [\n  { run = '\"$EDITOR\" \"$@\"', block = true, for = \"unix\" },\n]\nopen = [\n  { run = 'xdg-open \"$@\"', desc = \"Open\", for = \"linux\" },\n  { run = 'open \"$@\"', desc = \"Open\", for = \"macos\" },\n]\n\n[log]\nenabled = false\n")
	if yaziPreviewMode(cfg.PreviewMode) == "never" {
		sb.WriteString("\n[plugin]\n# PreviewMode=never disables yazi's previewer plugins with a schema-valid override.\npreviewers = []\n")
	}
	return sb.String()
}

func generateHistoricalYaziKeymap(cfg YaziConfig) string {
	var sb strings.Builder
	sb.WriteString("# Generated by dotfiles TUI\n")
	sb.WriteString(fmt.Sprintf("# Keymap style: %s\n\n[manager]\nkeymap = [\n", cfg.Keymap))
	if cfg.Keymap == "vim" {
		sb.WriteString("  # Vim-style navigation\n  { on = \"k\", run = \"arrow -1\", desc = \"Move up\" },\n  { on = \"j\", run = \"arrow 1\", desc = \"Move down\" },\n  { on = \"h\", run = \"leave\", desc = \"Go to parent\" },\n  { on = \"l\", run = \"enter\", desc = \"Enter directory\" },\n  { on = \"g\", run = \"arrow -99999999\", desc = \"Go to top\" },\n  { on = \"G\", run = \"arrow 99999999\", desc = \"Go to bottom\" },\n")
	} else {
		sb.WriteString("  # Emacs-style navigation\n  { on = \"<C-p>\", run = \"arrow -1\", desc = \"Move up\" },\n  { on = \"<C-n>\", run = \"arrow 1\", desc = \"Move down\" },\n  { on = \"<C-b>\", run = \"leave\", desc = \"Go to parent\" },\n  { on = \"<C-f>\", run = \"enter\", desc = \"Enter directory\" },\n  { on = \"<M-<>\", run = \"arrow -99999999\", desc = \"Go to top\" },\n  { on = \"<M->>\", run = \"arrow 99999999\", desc = \"Go to bottom\" },\n")
	}
	sb.WriteString("\n  # Common operations\n  { on = \"<Enter>\", run = \"open\", desc = \"Open file\" },\n  { on = \"<Esc>\", run = \"escape\", desc = \"Cancel\" },\n  { on = \"q\", run = \"quit\", desc = \"Quit\" },\n  { on = \"Q\", run = \"quit --no-cwd-file\", desc = \"Quit without changing cwd\" },\n  { on = \".\", run = \"hidden toggle\", desc = \"Toggle hidden files\" },\n  { on = \"/\", run = \"find --smart\", desc = \"Find\" },\n  { on = \"y\", run = \"yank\", desc = \"Yank (copy)\" },\n  { on = \"x\", run = \"yank --cut\", desc = \"Cut\" },\n  { on = \"p\", run = \"paste\", desc = \"Paste\" },\n  { on = \"d\", run = \"remove\", desc = \"Delete\" },\n  { on = \"a\", run = \"create\", desc = \"Create file/directory\" },\n  { on = \"r\", run = \"rename\", desc = \"Rename\" },\n  { on = \"<Space>\", run = \"select --state=none\", desc = \"Toggle selection\" },\n  { on = \"~\", run = \"cd ~\", desc = \"Go to home\" },\n]\n")
	return sb.String()
}

func generateHistoricalYaziTheme(theme string) string {
	p := yaziPaletteForTheme(theme)
	return fmt.Sprintf(`# Theme: %s (generated by dotfiles)

[manager]
cwd = { fg = "%s" }
hovered = { fg = "%s", bg = "%s" }
preview_hovered = { underline = true }

find_keyword = { fg = "%s", bold = true, italic = true, underline = true }
find_position = { fg = "%s", bg = "reset", bold = true }

marker_copied = { fg = "%s", bg = "%s" }
marker_cut = { fg = "%s", bg = "%s" }
marker_marked = { fg = "%s", bg = "%s" }
marker_selected = { fg = "%s", bg = "%s" }

tab_active = { fg = "%s", bg = "%s" }
tab_inactive = { fg = "%s", bg = "%s" }

border_symbol = "│"
border_style = { fg = "%s" }

[status]
separator_open = ""
separator_close = ""
separator_style = { fg = "%s", bg = "%s" }

mode_normal = { fg = "%s", bg = "%s", bold = true }
mode_select = { fg = "%s", bg = "%s", bold = true }

progress_label = { fg = "%s", bold = true }
progress_normal = { fg = "%s", bg = "%s" }
progress_error = { fg = "%s", bg = "%s" }

permissions_t = { fg = "%s" }
permissions_r = { fg = "%s" }
permissions_w = { fg = "%s" }
permissions_x = { fg = "%s" }
permissions_s = { fg = "%s" }

[input]
border = { fg = "%s" }
title = {}
value = {}
selected = { reversed = true }

[select]
border = { fg = "%s" }
active = { fg = "%s", bold = true }
inactive = {}

[tasks]
border = { fg = "%s" }
title = {}
hovered = { fg = "%s", underline = true }

[which]
cols = 3
mask = { bg = "%s" }
cand = { fg = "%s" }
rest = { fg = "%s" }
desc = { fg = "%s" }
separator = "  "
separator_style = { fg = "%s" }

[help]
on = { fg = "%s" }
run = { fg = "%s" }
desc = {}
hovered = { reversed = true, bold = true }
footer = { fg = "%s", bg = "%s" }

[filetype]
rules = [
  { mime = "image/*", fg = "%s" },
  { mime = "video/*", fg = "%s" },
  { mime = "audio/*", fg = "%s" },
  { mime = "application/zip", fg = "%s" },
  { mime = "application/gzip", fg = "%s" },
  { mime = "application/x-tar", fg = "%s" },
  { mime = "application/pdf", fg = "%s" },
  { url = "*", fg = "%s" },
  { url = "*/", fg = "%s" },
]
`, theme,
		p.Cyan,
		p.BG, p.Accent,
		p.Yellow,
		p.Accent3,
		p.Green, p.Green,
		p.Accent3, p.Accent3,
		p.Accent, p.Accent,
		p.Yellow, p.Yellow,
		p.BG, p.FG,
		p.FG, p.Overlay,
		p.Muted,
		p.Overlay, p.Overlay,
		p.BG, p.Accent,
		p.BG, p.Accent2,
		p.FG,
		p.Accent, p.Overlay,
		p.Accent3, p.Overlay,
		p.Accent,
		p.Yellow,
		p.Accent3,
		p.Green,
		p.Muted,
		p.Accent,
		p.Accent,
		p.Accent3,
		p.Accent,
		p.Accent3,
		p.Surface,
		p.Cyan,
		p.Muted,
		p.Accent3,
		p.Overlay,
		p.Cyan,
		p.Accent3,
		p.Overlay, p.FG,
		p.Yellow,
		p.Accent3,
		p.Accent3,
		p.Magenta,
		p.Magenta,
		p.Magenta,
		p.Accent3,
		p.FG,
		p.Accent,
	)
}
