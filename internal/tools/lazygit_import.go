package tools

// Read-only LazyGit YAML observation pinned to LazyGit v0.62.1 (f2788e4).
// The parser uses yaml.v3 only to inspect an AST; arbitrary native YAML is
// never marshaled or rewritten by dotfiles.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const (
	LazyGitFieldSidePanelWidth = "gui.sidePanelWidth"
	LazyGitFieldMouseEvents    = "gui.mouseEvents"
	LazyGitFieldColorPreset    = "gui.theme"
	LazyGitFieldPagerPreset    = "git.pagers"
)

type LazyGitConfigImport struct {
	Config                LazyGitConfig
	Fields                map[string]ConfigFieldProvenance
	Sources               []ConfigImportSource
	Managed               bool
	ReadOnlyReason        string
	RepoOverridesPossible bool
	Warnings              []string
}

type lazyGitObservedSource struct {
	path    string
	content []byte
	exists  bool
	active  bool
}

type lazyGitObservation struct {
	target   string
	sources  []lazyGitObservedSource
	readOnly string
}

func LazyGitConfigMutationPath() (string, error) {
	observed, err := observeLazyGitSources()
	if err != nil {
		return "", err
	}
	if observed.readOnly != "" {
		return "", errors.New(observed.readOnly)
	}
	return observed.target, nil
}

func ImportLazyGitConfig() (LazyGitConfigImport, error) {
	observed, err := observeLazyGitSources()
	if err != nil {
		return LazyGitConfigImport{}, err
	}
	result := LazyGitConfigImport{
		Config: LazyGitConfig{SidePanelWidth: "0.3333", MouseEvents: true, ColorPreset: "standard", PagerPreset: "builtin"},
		Fields: map[string]ConfigFieldProvenance{}, ReadOnlyReason: observed.readOnly,
		RepoOverridesPossible: true,
	}
	for _, source := range observed.sources {
		result.Sources = append(result.Sources, ConfigImportSource{Path: source.path, Exists: source.exists, Active: source.active})
		if !source.exists || !source.active {
			continue
		}
		parsed, parseErr := parseLazyGitYAML(source.path, source.content)
		if parseErr != nil {
			return LazyGitConfigImport{}, parseErr
		}
		for id, provenance := range parsed.Fields {
			result.Fields[id] = provenance
		}
		if _, ok := parsed.Fields[LazyGitFieldSidePanelWidth]; ok {
			result.Config.SidePanelWidth = parsed.Config.SidePanelWidth
		}
		if _, ok := parsed.Fields[LazyGitFieldMouseEvents]; ok {
			result.Config.MouseEvents = parsed.Config.MouseEvents
		}
		if _, ok := parsed.Fields[LazyGitFieldColorPreset]; ok || parsed.Config.ColorPreset == "custom" {
			result.Config.ColorPreset = parsed.Config.ColorPreset
		}
		if _, ok := parsed.Fields[LazyGitFieldPagerPreset]; ok || parsed.Config.PagerPreset == "custom" {
			result.Config.PagerPreset = parsed.Config.PagerPreset
		}
	}
	// Report only the effective merged values. Earlier LG_CONFIG_FILE sources
	// may contain custom shapes that a later source replaces completely.
	if result.Config.ColorPreset == "custom" {
		result.ReadOnlyReason = appendLazyGitReason(result.ReadOnlyReason, "custom LazyGit colors are read-only in this release")
	}
	if result.Config.PagerPreset == "custom" {
		result.ReadOnlyReason = appendLazyGitReason(result.ReadOnlyReason, "custom LazyGit pagers are read-only in this release")
	}
	activeIndex := -1
	for i, source := range observed.sources {
		if source.active && source.exists {
			if activeIndex != -1 {
				activeIndex = -2
				break
			}
			activeIndex = i
		}
	}
	if activeIndex >= 0 && observed.readOnly == "" {
		content := observed.sources[activeIndex].content
		result.Managed = IsManagedLazyGitConfigContent(content) || isLegacyManagedLazyGitConfigContent(content)
		result.Sources[activeIndex].Managed = result.Managed
		if result.Managed {
			markLazyGitManagedProvenance(result.Fields)
		}
		if !result.Managed && result.ReadOnlyReason == "" {
			result.ReadOnlyReason = "arbitrary native LazyGit YAML is read-only; only a missing or exact dotfiles-managed global file can be written"
		}
	}
	return result, nil
}

func InspectLazyGitConfigContent(path string, content []byte, exists bool) (LazyGitConfigImport, error) {
	result := LazyGitConfigImport{Config: LazyGitConfig{SidePanelWidth: "0.3333", MouseEvents: true, ColorPreset: "standard", PagerPreset: "builtin"}, Fields: map[string]ConfigFieldProvenance{}, Sources: []ConfigImportSource{{Path: path, Exists: exists, Active: true}}, RepoOverridesPossible: true}
	if !exists {
		return result, nil
	}
	parsed, err := parseLazyGitYAML(path, content)
	if err != nil {
		return LazyGitConfigImport{}, err
	}
	parsed.Managed = IsManagedLazyGitConfigContent(content) || isLegacyManagedLazyGitConfigContent(content)
	parsed.RepoOverridesPossible = true
	parsed.Sources = result.Sources
	parsed.Sources[0].Managed = parsed.Managed
	if parsed.Managed {
		markLazyGitManagedProvenance(parsed.Fields)
	}
	if !parsed.Managed {
		parsed.ReadOnlyReason = "arbitrary native LazyGit YAML is read-only; only a missing or exact dotfiles-managed global file can be written"
	}
	if parsed.Config.ColorPreset == "custom" {
		parsed.ReadOnlyReason = appendLazyGitReason(parsed.ReadOnlyReason, "custom LazyGit colors are read-only in this release")
	}
	if parsed.Config.PagerPreset == "custom" {
		parsed.ReadOnlyReason = appendLazyGitReason(parsed.ReadOnlyReason, "custom LazyGit pagers are read-only in this release")
	}
	return parsed, nil
}

func observeLazyGitSources() (lazyGitObservation, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return lazyGitObservation{}, fmt.Errorf("determine HOME for LazyGit config: %w", err)
	}
	if chain := os.Getenv("LG_CONFIG_FILE"); chain != "" {
		parts := strings.Split(chain, ",")
		seen := map[string]bool{}
		result := lazyGitObservation{readOnly: "LG_CONFIG_FILE custom source chains are observed read-only in this release"}
		for _, raw := range parts {
			if raw == "" || strings.TrimSpace(raw) != raw || !filepath.IsAbs(raw) {
				return lazyGitObservation{}, fmt.Errorf("LG_CONFIG_FILE entries must be nonempty absolute paths")
			}
			path := filepath.Clean(raw)
			if seen[path] {
				return lazyGitObservation{}, fmt.Errorf("LG_CONFIG_FILE contains duplicate path %s", path)
			}
			seen[path] = true
			content, exists, readErr := readNativeConfig(path)
			if readErr != nil {
				return lazyGitObservation{}, fmt.Errorf("read LG_CONFIG_FILE source %s: %w", path, readErr)
			}
			if !exists {
				return lazyGitObservation{}, fmt.Errorf("LG_CONFIG_FILE source does not exist: %s", path)
			}
			result.sources = append(result.sources, lazyGitObservedSource{path: path, content: content, exists: true, active: true})
		}
		return result, nil
	}

	modernDir, legacyDir, err := lazyGitConfigDirs(home)
	if err != nil {
		return lazyGitObservation{}, err
	}
	modern := filepath.Join(modernDir, "config.yml")
	legacy := ""
	if legacyDir != "" {
		legacy = filepath.Join(legacyDir, "config.yml")
	}
	modernContent, modernExists, err := readNativeConfig(modern)
	if err != nil {
		return lazyGitObservation{}, err
	}
	var legacyContent []byte
	var legacyExists bool
	if legacy != "" {
		legacyContent, legacyExists, err = readNativeConfig(legacy)
		if err != nil {
			return lazyGitObservation{}, err
		}
	}
	target := modern
	active := modern
	if legacy != "" && !modernExists && legacyExists {
		target, active = legacy, legacy
	}
	result := lazyGitObservation{target: target}
	result.sources = append(result.sources, lazyGitObservedSource{path: modern, content: modernContent, exists: modernExists, active: active == modern})
	if legacy != "" && legacy != modern && legacyExists {
		result.sources = append(result.sources, lazyGitObservedSource{path: legacy, content: legacyContent, exists: true, active: active == legacy})
	}
	if active == legacy && legacy != "" && legacy != modern {
		result.readOnly = "active LazyGit config uses the legacy jesseduffield fallback and is read-only in this release; dashboard writes require the modern lazygit config directory"
	} else if !pathWithinHome(home, target) {
		result.readOnly = "active LazyGit global config is outside HOME and cannot receive a verified rollback point"
	}
	return result, nil
}

func lazyGitConfigDirs(home string) (modern, legacy string, err error) {
	return lazyGitConfigDirsForOS(home, runtime.GOOS)
}

func lazyGitConfigDirsForOS(home, goos string) (modern, legacy string, err error) {
	if value := os.Getenv("CONFIG_DIR"); value != "" {
		if !filepath.IsAbs(value) {
			return "", "", fmt.Errorf("CONFIG_DIR must be absolute for LazyGit config")
		}
		modern = filepath.Clean(value)
		// CONFIG_DIR is the exact upstream directory override. The historical
		// jesseduffield fallback applies only to platform/XDG discovery.
		return modern, "", nil
	}
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		if !filepath.IsAbs(value) {
			return "", "", fmt.Errorf("XDG_CONFIG_HOME must be absolute for LazyGit config")
		}
		root := filepath.Clean(value)
		return filepath.Join(root, "lazygit"), filepath.Join(root, "jesseduffield", "lazygit"), nil
	}
	if goos == "darwin" {
		root := filepath.Join(home, "Library", "Application Support")
		return filepath.Join(root, "lazygit"), filepath.Join(root, "jesseduffield", "lazygit"), nil
	}
	root := filepath.Join(home, ".config")
	return filepath.Join(root, "lazygit"), filepath.Join(root, "jesseduffield", "lazygit"), nil
}

func pathWithinHome(home, path string) bool {
	rel, err := filepath.Rel(home, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func parseLazyGitYAML(path string, content []byte) (LazyGitConfigImport, error) {
	result := LazyGitConfigImport{Config: LazyGitConfig{SidePanelWidth: "0.3333", MouseEvents: true, ColorPreset: "standard", PagerPreset: "builtin"}, Fields: map[string]ConfigFieldProvenance{}}
	if !utf8.Valid(content) || bytes.HasPrefix(content, []byte{0xef, 0xbb, 0xbf}) {
		return result, fmt.Errorf("%s: invalid UTF-8 or BOM in LazyGit YAML", path)
	}
	for _, b := range content {
		if b < 0x20 && b != '\n' && b != '\r' && b != '\t' {
			return result, fmt.Errorf("%s: forbidden control byte in LazyGit YAML", path)
		}
	}
	for _, r := range string(content) {
		if (unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t') || isBidiControl(r) {
			return result, fmt.Errorf("%s: forbidden control character in LazyGit YAML", path)
		}
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return result, nil
		}
		return result, fmt.Errorf("parse LazyGit YAML %s: %w", path, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return result, fmt.Errorf("%s: multiple YAML documents are unsupported", path)
		}
		return result, fmt.Errorf("parse LazyGit YAML %s: %w", path, err)
	}
	if len(document.Content) == 0 {
		return result, nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return result, fmt.Errorf("%s: LazyGit YAML root must be a mapping", path)
	}
	if err := validateLazyGitNode(root); err != nil {
		return result, fmt.Errorf("%s: %w", path, err)
	}
	gui := yamlMapValue(root, "gui")
	if gui != nil {
		if gui.Kind != yaml.MappingNode {
			return result, fmt.Errorf("%s:%d: gui must be a mapping", path, gui.Line)
		}
		if node := yamlMapValue(gui, "sidePanelWidth"); node != nil {
			if node.Kind != yaml.ScalarNode || (node.Tag != "!!float" && node.Tag != "!!int" && node.Tag != "!!str") || validateLazyGitFraction(node.Value) != nil {
				return result, fmt.Errorf("%s:%d: gui.sidePanelWidth must be a finite decimal from 0 through 1", path, node.Line)
			}
			result.Config.SidePanelWidth = node.Value
			if strings.HasPrefix(result.Config.SidePanelWidth, ".") {
				result.Config.SidePanelWidth = "0" + result.Config.SidePanelWidth
			}
			result.Fields[LazyGitFieldSidePanelWidth] = lazyGitProvenance(path, node, "gui.sidePanelWidth")
		}
		if node := yamlMapValue(gui, "mouseEvents"); node != nil {
			if node.Kind != yaml.ScalarNode || node.Tag != "!!bool" || (node.Value != "true" && node.Value != "false") {
				return result, fmt.Errorf("%s:%d: gui.mouseEvents must be a boolean", path, node.Line)
			}
			result.Config.MouseEvents = node.Value == "true"
			result.Fields[LazyGitFieldMouseEvents] = lazyGitProvenance(path, node, "gui.mouseEvents")
		}
		if node := yamlMapValue(gui, "theme"); node != nil {
			preset := classifyLazyGitTheme(node)
			result.Config.ColorPreset = preset
			result.Fields[LazyGitFieldColorPreset] = lazyGitProvenance(path, node, "gui.theme")
		}
	}
	git := yamlMapValue(root, "git")
	if git != nil {
		if git.Kind != yaml.MappingNode {
			return result, fmt.Errorf("%s:%d: git must be a mapping", path, git.Line)
		}
		if node := yamlMapValue(git, "pagers"); node != nil {
			result.Config.PagerPreset = classifyLazyGitPagers(node)
			result.Fields[LazyGitFieldPagerPreset] = lazyGitProvenance(path, node, "git.pagers")
		} else if paging := yamlMapValue(git, "paging"); paging != nil {
			result.Config.PagerPreset = classifyLegacyLazyGitPaging(paging)
			result.Fields[LazyGitFieldPagerPreset] = lazyGitProvenance(path, paging, "git.paging")
		}
	}
	return result, nil
}

func appendLazyGitReason(existing, addition string) string {
	if existing == "" {
		return addition
	}
	if strings.Contains(existing, addition) {
		return existing
	}
	return existing + "; " + addition
}

func markLazyGitManagedProvenance(fields map[string]ConfigFieldProvenance) {
	for id, provenance := range fields {
		provenance.Scope = ConfigValueManaged
		fields[id] = provenance
	}
}

func validateLazyGitNode(node *yaml.Node) error {
	if node.Anchor != "" || node.Alias != nil || node.Kind == yaml.AliasNode || node.Tag == "!!merge" {
		return fmt.Errorf("anchors, aliases, and merge keys are unsupported in LazyGit YAML")
	}
	if node.Tag != "" && !strings.HasPrefix(node.Tag, "!!") {
		return fmt.Errorf("custom YAML tags are unsupported")
	}
	if node.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return fmt.Errorf("mapping keys must be strings")
			}
			if seen[key.Value] {
				return fmt.Errorf("duplicate YAML mapping key %q at line %d", key.Value, key.Line)
			}
			seen[key.Value] = true
		}
	}
	for _, child := range node.Content {
		if err := validateLazyGitNode(child); err != nil {
			return err
		}
	}
	return nil
}

func yamlMapValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func lazyGitProvenance(path string, node *yaml.Node, key string) ConfigFieldProvenance {
	return ConfigFieldProvenance{Path: path, Line: node.Line, Key: key, Scope: ConfigValueNative}
}

func classifyLazyGitTheme(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.MappingNode {
		return "custom"
	}
	wantStandard := map[string][]string{"activeBorderColor": {"green", "bold"}, "inactiveBorderColor": {"default"}, "selectedLineBgColor": {"blue"}, "defaultFgColor": {"default"}}
	wantLight := map[string][]string{"activeBorderColor": {"blue", "bold"}, "inactiveBorderColor": {"default"}, "selectedLineBgColor": {"reverse"}, "defaultFgColor": {"black"}}
	if yamlStringArraysEqual(node, wantStandard) {
		return "standard"
	}
	if yamlStringArraysEqual(node, wantLight) {
		return "light-high-contrast"
	}
	return "custom"
}

func yamlStringArraysEqual(node *yaml.Node, want map[string][]string) bool {
	if len(node.Content)/2 != len(want) {
		return false
	}
	for key, values := range want {
		child := yamlMapValue(node, key)
		if child == nil || child.Kind != yaml.SequenceNode || len(child.Content) != len(values) {
			return false
		}
		for i, value := range values {
			if child.Content[i].Kind != yaml.ScalarNode || child.Content[i].Value != value {
				return false
			}
		}
	}
	return true
}

func classifyLazyGitPagers(node *yaml.Node) string {
	if node.Kind != yaml.SequenceNode {
		return "custom"
	}
	if len(node.Content) == 0 {
		return "builtin"
	}
	if len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
		return "custom"
	}
	entry := node.Content[0]
	if len(entry.Content) != 4 {
		return "custom"
	}
	color, pager := yamlMapValue(entry, "colorArg"), yamlMapValue(entry, "pager")
	if color != nil && pager != nil && color.Kind == yaml.ScalarNode && color.Value == "always" && pager.Kind == yaml.ScalarNode && pager.Value == "delta --dark --paging=never" {
		return "delta"
	}
	return "custom"
}

func classifyLegacyLazyGitPaging(node *yaml.Node) string {
	if node.Kind != yaml.MappingNode {
		return "custom"
	}
	pager := yamlMapValue(node, "pager")
	color := yamlMapValue(node, "colorArg")
	if pager == nil || pager.Kind != yaml.ScalarNode {
		return "custom"
	}
	if pager.Value == "cat" {
		return "builtin"
	}
	if pager.Value == "delta --dark --paging=never" && color != nil && color.Value == "always" {
		return "delta"
	}
	return "custom"
}
