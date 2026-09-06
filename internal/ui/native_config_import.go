package ui

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/tools"
)

// NativeManageConfigState exposes the observed native source chain and
// per-field provenance to the UI model. Applied is true only on first adoption,
// when no manage.json exists; explicit saved product preferences always win.
type NativeManageConfigState struct {
	Git                  tools.GitConfigImport
	Ghostty              tools.GhosttyConfigImport
	Tmux                 tools.TmuxConfigImport
	Btop                 tools.BtopConfigImport
	Glow                 tools.GlowConfigImport
	LazyGit              tools.LazyGitConfigImport
	Yazi                 tools.YaziConfigImport
	GitError             string
	GhosttyError         string
	TmuxError            string
	BtopError            string
	GlowError            string
	LazyGitError         string
	YaziError            string
	BtopThemeExplicit    bool
	BtopThemeUnsupported bool
	Applied              bool
	PreferenceError      string
}

func yaziUIFieldBlockReason(a *App, fieldKey string) string {
	if a == nil {
		return ""
	}
	if a.nativeConfigState.PreferenceError != "" {
		return "saved management preferences could not be read safely: " + a.nativeConfigState.PreferenceError
	}
	if a.nativeConfigState.YaziError != "" {
		return "native Yazi configuration could not be imported safely: " + a.nativeConfigState.YaziError
	}
	switch fieldKey {
	case "keymap":
		return a.nativeConfigState.Yazi.Keymap.ReadOnlyReason
	case "hidden", "preview_mode", "sort_by", "sort_rev", "linemode", "scrolloff":
		return a.nativeConfigState.Yazi.Main.ReadOnlyReason
	default:
		return ""
	}
}

// lazyGitUIBlockReason is the single presentation and interaction policy for
// LazyGit whole-source read-only state.
func lazyGitUIBlockReason(a *App) string {
	if a == nil {
		return ""
	}
	if a.nativeConfigState.PreferenceError != "" {
		return "saved management preferences could not be read safely: " + a.nativeConfigState.PreferenceError
	}
	if a.nativeConfigState.LazyGitError != "" {
		return "native LazyGit configuration could not be imported safely: " + a.nativeConfigState.LazyGitError
	}
	return a.nativeConfigState.LazyGit.ReadOnlyReason
}

func lazyGitStandaloneUIBlockReason(a *App) string {
	if reason := lazyGitUIBlockReason(a); reason != "" {
		return reason
	}
	if a == nil || a.deepDiveConfig == nil {
		return ""
	}
	if err := tools.ValidateLazyGitConfig(lazygitConfigFrom(*a.deepDiveConfig), a.theme); err != nil {
		return err.Error()
	}
	return ""
}

func lazyGitManageUIBlockReason(a *App) string {
	if reason := lazyGitUIBlockReason(a); reason != "" {
		return reason
	}
	if a == nil || a.manageConfig == nil {
		return ""
	}
	if err := tools.ValidateLazyGitConfig(lazygitConfigFrom(manageConfigToDeepDive(a.manageConfig)), a.theme); err != nil {
		return err.Error()
	}
	return ""
}

// NativeConfigState returns a defensive copy suitable for status/provenance UI.
func (a *App) NativeConfigState() NativeManageConfigState {
	state := a.nativeConfigState
	state.Git.Fields = cloneConfigProvenance(state.Git.Fields)
	state.Git.Sources = append([]tools.ConfigImportSource(nil), state.Git.Sources...)
	state.Git.Warnings = append([]string(nil), state.Git.Warnings...)
	state.Git.Config.Aliases = append([]string(nil), state.Git.Config.Aliases...)
	state.Ghostty.Fields = cloneConfigProvenance(state.Ghostty.Fields)
	state.Ghostty.Sources = append([]tools.ConfigImportSource(nil), state.Ghostty.Sources...)
	state.Ghostty.Warnings = append([]string(nil), state.Ghostty.Warnings...)
	state.Tmux.Fields = cloneConfigProvenance(state.Tmux.Fields)
	state.Tmux.Sources = append([]tools.ConfigImportSource(nil), state.Tmux.Sources...)
	state.Tmux.Warnings = append([]string(nil), state.Tmux.Warnings...)
	state.Btop.Fields = cloneConfigProvenance(state.Btop.Fields)
	state.Btop.Sources = append([]tools.ConfigImportSource(nil), state.Btop.Sources...)
	state.Btop.Warnings = append([]string(nil), state.Btop.Warnings...)
	state.Glow.Fields = cloneConfigProvenance(state.Glow.Fields)
	state.Glow.Sources = append([]tools.ConfigImportSource(nil), state.Glow.Sources...)
	state.Glow.Warnings = append([]string(nil), state.Glow.Warnings...)
	state.LazyGit.Fields = cloneConfigProvenance(state.LazyGit.Fields)
	state.LazyGit.Sources = append([]tools.ConfigImportSource(nil), state.LazyGit.Sources...)
	state.LazyGit.Warnings = append([]string(nil), state.LazyGit.Warnings...)
	state.Yazi.Fields = cloneConfigProvenance(state.Yazi.Fields)
	return state
}

func cloneConfigProvenance(source map[string]tools.ConfigFieldProvenance) map[string]tools.ConfigFieldProvenance {
	if source == nil {
		return nil
	}
	cloned := make(map[string]tools.ConfigFieldProvenance, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

type managePreferencePresence struct {
	exists              bool
	fields              map[string]bool
	schema              int
	legacyLazyGitTheme  string
	legacyLazyGitPaging string
	err                 error
}

func inspectManagePreferencePresence() managePreferencePresence {
	if config.ConfigDir() == "" {
		return managePreferencePresence{exists: true, err: config.ErrNoConfigDir}
	}
	raw, exists, err := config.LoadToolConfigWithPresence("manage", func() *map[string]json.RawMessage {
		fields := make(map[string]json.RawMessage)
		return &fields
	})
	if err != nil {
		return managePreferencePresence{exists: true, fields: map[string]bool{}, err: err}
	}
	if !exists {
		return managePreferencePresence{fields: map[string]bool{}}
	}
	fields := make(map[string]bool, len(*raw))
	for key := range *raw {
		fields[key] = true
	}
	var schemaVersion int
	if encoded, ok := (*raw)["NativeImportSchemaVersion"]; ok {
		if err := json.Unmarshal(encoded, &schemaVersion); err != nil {
			return managePreferencePresence{exists: true, fields: fields, err: errors.New("native import schema version must be an integer")}
		}
		if schemaVersion < 1 || schemaVersion > currentNativeImportSchemaVersion {
			return managePreferencePresence{exists: true, fields: fields, err: errors.New("unsupported native import schema version")}
		}
	}
	var legacyTheme, legacyPaging string
	if encoded, ok := (*raw)["LazyGitTheme"]; ok {
		_ = json.Unmarshal(encoded, &legacyTheme)
	}
	if encoded, ok := (*raw)["LazyGitPaging"]; ok {
		_ = json.Unmarshal(encoded, &legacyPaging)
	}
	switch schemaVersion {
	case 0:
		// One-time adoption migration for prototype-era full-struct saves. Those
		// files had no way to distinguish deliberate choices from copied defaults.
		for key := range fields {
			if strings.HasPrefix(key, "Git") || strings.HasPrefix(key, "Ghostty") || strings.HasPrefix(key, "Ghossty") || strings.HasPrefix(key, "Tmux") || strings.HasPrefix(key, "Btop") || strings.HasPrefix(key, "Glow") || strings.HasPrefix(key, "LazyGit") || strings.HasPrefix(key, "Yazi") {
				delete(fields, key)
			}
		}
	case 1:
		// Schema v1 introduced explicit Git/Ghostty presence. Tmux import did not
		// exist, so serialized Tmux defaults from that version are not evidence of
		// user intent and may be hydrated once from the active native source.
		for key := range fields {
			if strings.HasPrefix(key, "Tmux") {
				delete(fields, key)
			}
		}
		fallthrough
	case 2:
		// Native btop import was introduced in schema v3. Older serialized
		// defaults are not proof of an explicit btop preference.
		for key := range fields {
			if strings.HasPrefix(key, "Btop") {
				delete(fields, key)
			}
		}
		fallthrough
	case 3:
		// Native Glow import is schema v4. Earlier files serialized prototype
		// defaults and pager labels that do not prove explicit user intent.
		for key := range fields {
			if strings.HasPrefix(key, "Glow") {
				delete(fields, key)
			}
		}
		fallthrough
	case 4:
		// LazyGit native hydration and honest preset names are schema v5.
		for key := range fields {
			if strings.HasPrefix(key, "LazyGit") {
				delete(fields, key)
			}
		}
		fallthrough
	case 5:
		// Native Yazi observation is schema v6. Earlier files serialized
		// dashboard defaults rather than explicit per-field intent.
		for key := range fields {
			if strings.HasPrefix(key, "Yazi") {
				delete(fields, key)
			}
		}
	}
	return managePreferencePresence{
		exists: true, fields: fields, schema: schemaVersion,
		legacyLazyGitTheme: legacyTheme, legacyLazyGitPaging: legacyPaging,
	}
}

func observeNativeManageConfig(target *ManageConfig, preferences managePreferencePresence, theme string) NativeManageConfigState {
	state := NativeManageConfigState{}
	state.BtopThemeExplicit = preferences.fields["BtopTheme"]
	var before ManageConfig
	if target != nil {
		before = *target
	}
	gitImport, err := tools.ImportGitConfig()
	switch {
	case err != nil:
		state.GitError = err.Error()
	case len(gitImport.Warnings) != 0:
		state.Git = gitImport
		state.GitError = "refusing ambiguous Git import: " + strings.Join(gitImport.Warnings, "; ")
	default:
		state.Git = gitImport
		if preferences.err == nil {
			overlayImportedGitConfig(target, gitImport, preferences.fields)
		}
	}
	ghosttyImport, err := tools.ImportGhosttyConfig()
	switch {
	case err != nil:
		state.GhosttyError = err.Error()
	case len(ghosttyImport.Warnings) != 0:
		state.Ghostty = ghosttyImport
		state.GhosttyError = "refusing ambiguous Ghostty import: " + strings.Join(ghosttyImport.Warnings, "; ")
	default:
		state.Ghostty = ghosttyImport
		if preferences.err == nil {
			overlayImportedGhosttyConfig(target, ghosttyImport, preferences.fields)
		}
	}
	tmuxImport, err := tools.ImportTmuxConfig()
	switch {
	case err != nil:
		state.TmuxError = err.Error()
	case len(tmuxImport.Warnings) != 0:
		state.Tmux = tmuxImport
		state.TmuxError = "refusing ambiguous tmux import: " + strings.Join(tmuxImport.Warnings, "; ")
	default:
		state.Tmux = tmuxImport
		if preferences.err == nil {
			overlayImportedTmuxConfig(target, tmuxImport, preferences.fields)
		}
	}
	btopImport, err := tools.ImportBtopConfig()
	switch {
	case err != nil:
		state.BtopError = err.Error()
	case len(btopImport.Warnings) != 0:
		state.Btop = btopImport
		state.BtopError = "refusing ambiguous btop import: " + strings.Join(btopImport.Warnings, "; ")
	case btopImportedThemeNeedsReplacement(btopImport, theme) && !preferences.fields["BtopTheme"]:
		state.Btop = btopImport
		state.BtopThemeUnsupported = true
		state.BtopError = "native btop color_theme cannot be represented by the dashboard without changing it"
	default:
		state.Btop = btopImport
		if preferences.err == nil {
			overlayImportedBtopConfig(target, btopImport, preferences.fields, theme)
		}
	}
	glowImport, err := tools.ImportGlowConfig()
	switch {
	case err != nil:
		state.GlowError = err.Error()
	case len(glowImport.Warnings) != 0:
		state.Glow = glowImport
		state.GlowError = "refusing ambiguous Glow import: " + strings.Join(glowImport.Warnings, "; ")
	default:
		state.Glow = glowImport
		if preferences.err == nil {
			overlayImportedGlowConfig(target, glowImport, preferences.fields, preferences.schema)
		}
	}
	lazyGitImport, err := tools.ImportLazyGitConfig()
	switch {
	case err != nil:
		state.LazyGitError = err.Error()
	case len(lazyGitImport.Warnings) != 0:
		state.LazyGit = lazyGitImport
		state.LazyGitError = "refusing ambiguous LazyGit import: " + strings.Join(lazyGitImport.Warnings, "; ")
	default:
		state.LazyGit = lazyGitImport
		if preferences.err == nil {
			explicit := preferences.fields
			if lazyGitImport.ReadOnlyReason != "" {
				explicit = map[string]bool{}
			}
			overlayImportedLazyGitConfig(target, lazyGitImport, explicit, preferences)
		}
	}
	yaziImport, err := tools.ImportYaziConfig()
	if err != nil {
		state.YaziError = err.Error()
	} else {
		state.Yazi = yaziImport
		if preferences.err == nil {
			overlayImportedYaziConfig(target, yaziImport, preferences.fields)
		}
	}
	state.Applied = preferences.err == nil && target != nil && *target != before
	if preferences.err == nil && target != nil {
		target.NativeImportSchemaVersion = currentNativeImportSchemaVersion
	}
	if preferences.err != nil {
		state.PreferenceError = preferences.err.Error()
	}
	return state
}

func overlayImportedYaziConfig(target *ManageConfig, imported tools.YaziConfigImport, explicit map[string]bool) {
	if target == nil {
		return
	}
	applyMain := imported.Main.Exists && imported.Main.Ownership != tools.YaziOwnershipMissing && imported.Main.Ownership != tools.YaziOwnershipMalformed
	if applyMain {
		force := imported.Main.Ownership == tools.YaziOwnershipNative || imported.Main.Ownership == tools.YaziOwnershipExactHistorical
		for key, apply := range map[string]func(){
			"YaziShowHidden":  func() { target.YaziShowHidden = imported.Config.ShowHidden },
			"YaziPreviewMode": func() { target.YaziPreviewMode = imported.Config.PreviewMode },
			"YaziSortBy":      func() { target.YaziSortBy = imported.Config.SortBy },
			"YaziSortReverse": func() { target.YaziSortReverse = imported.Config.SortReverse },
			"YaziLineMode":    func() { target.YaziLineMode = imported.Config.LineMode },
			"YaziScrollOff":   func() { target.YaziScrollOff = imported.Config.ScrollOff },
		} {
			if force || !explicit[key] {
				apply()
			}
		}
	}
	applyKeymap := imported.Keymap.Exists && imported.Keymap.Ownership != tools.YaziOwnershipMissing && imported.Keymap.Ownership != tools.YaziOwnershipMalformed
	if applyKeymap {
		force := imported.Keymap.Ownership == tools.YaziOwnershipNative || imported.Keymap.Ownership == tools.YaziOwnershipExactHistorical
		if force || !explicit["YaziKeymap"] {
			target.YaziKeymap = imported.Config.Keymap
		}
	}
}

func overlayImportedLazyGitConfig(target *ManageConfig, imported tools.LazyGitConfigImport, explicit map[string]bool, preferences managePreferencePresence) {
	if target == nil {
		return
	}
	if preferences.schema < 5 {
		switch preferences.legacyLazyGitTheme {
		case "auto", "dark":
			target.LazyGitColorPreset = "standard"
		case "light":
			target.LazyGitColorPreset = "light-high-contrast"
		}
		switch preferences.legacyLazyGitPaging {
		case "never":
			target.LazyGitPagerPreset = "builtin"
		case "delta":
			target.LazyGitPagerPreset = "delta"
		case "diff-so-fancy":
			target.LazyGitPagerPreset = "custom"
		}
	}
	hasNative := false
	for _, source := range imported.Sources {
		hasNative = hasNative || (source.Active && source.Exists)
	}
	if !hasNative {
		return
	}
	for key, apply := range map[string]func(){
		"LazyGitSidePanelWidth": func() { target.LazyGitSidePanelWidth = imported.Config.SidePanelWidth },
		"LazyGitMouseEvents":    func() { target.LazyGitMouseEvents = imported.Config.MouseEvents },
		"LazyGitColorPreset":    func() { target.LazyGitColorPreset = imported.Config.ColorPreset },
		"LazyGitPagerPreset":    func() { target.LazyGitPagerPreset = imported.Config.PagerPreset },
	} {
		if !explicit[key] {
			apply()
		}
	}
}

func overlayImportedGlowConfig(target *ManageConfig, imported tools.GlowConfigImport, explicit map[string]bool, schema int) {
	if target == nil {
		return
	}
	hasNative := false
	for _, source := range imported.Sources {
		hasNative = hasNative || (source.Active && source.Exists)
	}
	if !hasNative {
		if schema < 4 && !explicit["GlowPager"] {
			switch target.GlowPager {
			case "none", "never":
				target.GlowPager = "never"
			case "auto", "less", "more", "":
				target.GlowPager = "auto"
			}
		}
		return
	}
	for key, apply := range map[string]func(){
		"GlowStyle":            func() { target.GlowStyle = imported.Config.Style },
		"GlowPager":            func() { target.GlowPager = imported.Config.Pager },
		"GlowWidth":            func() { target.GlowWidth = imported.Config.Width },
		"GlowMouse":            func() { target.GlowMouse = imported.Config.Mouse },
		"GlowAll":              func() { target.GlowAll = imported.Config.All },
		"GlowShowLineNumbers":  func() { target.GlowShowLineNumbers = imported.Config.ShowLineNumbers },
		"GlowPreserveNewLines": func() { target.GlowPreserveNewLines = imported.Config.PreserveNewLines },
	} {
		if !explicit[key] {
			apply()
		}
	}
}

func btopImportedThemeNeedsReplacement(imported tools.BtopConfigImport, theme string) bool {
	if _, ok := imported.Fields[tools.BtopFieldTheme]; !ok {
		for _, source := range imported.Sources {
			if source.Active && source.Exists && !source.Managed {
				return true
			}
		}
		return false
	}
	value := imported.Config.Theme
	return value != theme && !oneOf(value, "dracula", "gruvbox", "nord", "tokyo-night")
}

func overlayImportedBtopConfig(target *ManageConfig, imported tools.BtopConfigImport, explicit map[string]bool, theme string) {
	if target == nil {
		return
	}
	if _, ok := imported.Fields[tools.BtopFieldTheme]; ok && !explicit["BtopTheme"] {
		if imported.Config.Theme == theme {
			target.BtopTheme = "auto"
		} else if oneOf(imported.Config.Theme, "dracula", "gruvbox", "nord", "tokyo-night") {
			target.BtopTheme = imported.Config.Theme
		}
	}
	if _, ok := imported.Fields[tools.BtopFieldUpdateMs]; ok && !explicit["BtopUpdateMs"] {
		target.BtopUpdateMs = imported.Config.UpdateMs
	}
	if _, ok := imported.Fields[tools.BtopFieldShowTemp]; ok && !explicit["BtopShowTemp"] {
		target.BtopShowTemp = imported.Config.ShowTemp
	}
	if _, ok := imported.Fields[tools.BtopFieldGraphType]; ok && !explicit["BtopGraphSymbol"] {
		target.BtopGraphSymbol = imported.Config.GraphType
	}
	if _, ok := imported.Fields[tools.BtopFieldTempScale]; ok && !explicit["BtopTempScale"] {
		target.BtopTempScale = imported.Config.TempScale
	}
	if _, ok := imported.Fields[tools.BtopFieldShownBoxes]; ok && !explicit["BtopShownBoxes"] {
		target.BtopShownBoxes = imported.Config.ShownBoxes
	}
}

func overlayImportedTmuxConfig(target *ManageConfig, imported tools.TmuxConfigImport, explicit map[string]bool) {
	if target == nil {
		return
	}
	hasNativeSource := false
	for _, source := range imported.Sources {
		hasNativeSource = hasNativeSource || (source.Active && source.Exists && !source.Managed)
	}
	// A native source with no plugin declarations must not inherit the product's
	// compiled TPM defaults. Explicit saved preferences continue to win.
	if hasNativeSource {
		for key, destination := range map[string]*bool{
			"TmuxTPMEnabled":      &target.TmuxTPMEnabled,
			"TmuxPluginSensible":  &target.TmuxPluginSensible,
			"TmuxPluginResurrect": &target.TmuxPluginResurrect,
			"TmuxPluginContinuum": &target.TmuxPluginContinuum,
			"TmuxPluginYank":      &target.TmuxPluginYank,
		} {
			if !explicit[key] {
				*destination = false
			}
		}
	}
	if _, ok := imported.Fields[tools.TmuxFieldPrefix]; ok && !explicit["TmuxPrefix"] {
		target.TmuxPrefix = map[string]string{"ctrl-a": "C-a", "ctrl-b": "C-b", "ctrl-space": "C-Space"}[imported.Config.Prefix]
	}
	if _, ok := imported.Fields[tools.TmuxFieldSplitBinds]; ok && !explicit["TmuxSplitBinds"] {
		target.TmuxSplitBinds = imported.Config.SplitBinds
	}
	if _, ok := imported.Fields[tools.TmuxFieldStatusPosition]; ok && !explicit["TmuxStatusPosition"] {
		target.TmuxStatusPosition = imported.Config.StatusBar
	}
	if _, ok := imported.Fields[tools.TmuxFieldMouse]; ok && !explicit["TmuxMouseMode"] {
		target.TmuxMouseMode = imported.Config.MouseMode
	}
	if _, ok := imported.Fields[tools.TmuxFieldBaseIndex]; ok && !explicit["TmuxBaseIndex"] {
		target.TmuxBaseIndex = imported.Config.BaseIndex
	}
	if _, ok := imported.Fields[tools.TmuxFieldPaneBorder]; ok && !explicit["TmuxPaneBorderStyle"] {
		target.TmuxPaneBorderStyle = imported.Config.PaneBorderStyle
	}
	if _, ok := imported.Fields[tools.TmuxFieldHistoryLimit]; ok && !explicit["TmuxHistoryLimit"] {
		target.TmuxHistoryLimit = imported.Config.HistoryLimit
	}
	if _, ok := imported.Fields[tools.TmuxFieldEscapeTime]; ok && !explicit["TmuxEscapeTime"] {
		target.TmuxEscapeTime = imported.Config.EscapeTime
	}
	if _, ok := imported.Fields[tools.TmuxFieldAggressiveResize]; ok && !explicit["TmuxAggressiveResize"] {
		target.TmuxAggressiveResize = imported.Config.AggressiveResize
	}
	for field, setting := range map[string]struct {
		key         string
		destination *bool
		value       bool
	}{
		tools.TmuxFieldTPMEnabled:      {"TmuxTPMEnabled", &target.TmuxTPMEnabled, imported.Config.TPMEnabled},
		tools.TmuxFieldPluginSensible:  {"TmuxPluginSensible", &target.TmuxPluginSensible, imported.Config.PluginSensible},
		tools.TmuxFieldPluginResurrect: {"TmuxPluginResurrect", &target.TmuxPluginResurrect, imported.Config.PluginResurrect},
		tools.TmuxFieldPluginContinuum: {"TmuxPluginContinuum", &target.TmuxPluginContinuum, imported.Config.PluginContinuum},
		tools.TmuxFieldPluginYank:      {"TmuxPluginYank", &target.TmuxPluginYank, imported.Config.PluginYank},
	} {
		if _, ok := imported.Fields[field]; ok && !explicit[setting.key] {
			*setting.destination = setting.value
		}
	}
	if _, ok := imported.Fields[tools.TmuxFieldContinuumSaveMin]; ok && !explicit["TmuxContinuumSaveMin"] {
		target.TmuxContinuumSaveMin = imported.Config.ContinuumSaveMin
	}
	if _, ok := imported.Fields[tools.TmuxFieldContinuumRestore]; ok && !explicit["TmuxContinuumRestore"] {
		target.TmuxContinuumRestore = imported.Config.ContinuumRestore
	}
}

func overlayImportedGitConfig(target *ManageConfig, imported tools.GitConfigImport, explicit map[string]bool) {
	if target == nil {
		return
	}
	if _, ok := imported.Fields[tools.GitFieldDefaultBranch]; ok && !explicit["GitDefaultBranch"] && oneOf(imported.Config.DefaultBranch, "main", "master", "develop") {
		target.GitDefaultBranch = imported.Config.DefaultBranch
	}
	if _, ok := imported.Fields[tools.GitFieldAutoSetupRemote]; ok && !explicit["GitAutoSetupRemote"] {
		target.GitAutoSetupRemote = imported.Config.AutoSetupRemote
	}
	if _, ok := imported.Fields[tools.GitFieldPullRebase]; ok && !explicit["GitPullRebase"] {
		target.GitPullRebase = imported.Config.PullRebase
	}
	if _, ok := imported.Fields[tools.GitFieldDiffTool]; ok && !explicit["GitDiffTool"] && oneOf(imported.Config.DiffTool, "delta", "difftastic", "vimdiff", "nvimdiff") {
		target.GitDiffTool = imported.Config.DiffTool
	}
	if _, ok := imported.Fields[tools.GitFieldMergeTool]; ok && !explicit["GitMergeTool"] && oneOf(imported.Config.MergeTool, "vimdiff", "nvimdiff", "meld") {
		target.GitMergeTool = imported.Config.MergeTool
	}
	if _, ok := imported.Fields[tools.GitFieldCredentialHelper]; ok && !explicit["GitCredentialHelper"] && oneOf(imported.Config.CredentialHelper, "store", "cache", "osxkeychain", "none") {
		target.GitCredentialHelper = imported.Config.CredentialHelper
	}
	if _, ok := imported.Fields[tools.GitFieldSignCommits]; ok && !explicit["GitSignCommits"] {
		target.GitSignCommits = imported.Config.SignCommits
	}
	if _, ok := imported.Fields[tools.GitFieldDeltaSideBySide]; ok && !explicit["GitDeltaSideBySide"] {
		target.GitDeltaSideBySide = imported.Config.DeltaSideBySide
	}
	// On an adopted native source, absence means dotfiles must not introduce a
	// default alias that could override a custom or intentionally absent alias.
	hasNativeSource := false
	for _, source := range imported.Sources {
		hasNativeSource = hasNativeSource || (source.Active && !source.Managed)
	}
	if hasNativeSource {
		if !explicit["GitAliasStatus"] {
			target.GitAliasStatus = false
		}
		if !explicit["GitAliasCheckout"] {
			target.GitAliasCheckout = false
		}
		if !explicit["GitAliasBranch"] {
			target.GitAliasBranch = false
		}
		if !explicit["GitAliasCommit"] {
			target.GitAliasCommit = false
		}
		if !explicit["GitAliasLogGraph"] {
			target.GitAliasLogGraph = false
		}
	}
	for field, destination := range map[string]*bool{
		tools.GitFieldAliasSt: &target.GitAliasStatus,
		tools.GitFieldAliasCo: &target.GitAliasCheckout,
		tools.GitFieldAliasBr: &target.GitAliasBranch,
		tools.GitFieldAliasCi: &target.GitAliasCommit,
		tools.GitFieldAliasLg: &target.GitAliasLogGraph,
	} {
		manageKey := map[string]string{tools.GitFieldAliasSt: "GitAliasStatus", tools.GitFieldAliasCo: "GitAliasCheckout", tools.GitFieldAliasBr: "GitAliasBranch", tools.GitFieldAliasCi: "GitAliasCommit", tools.GitFieldAliasLg: "GitAliasLogGraph"}[field]
		if _, ok := imported.Fields[field]; ok && !explicit[manageKey] {
			*destination = true
		}
	}
}

func overlayImportedGhosttyConfig(target *ManageConfig, imported tools.GhosttyConfigImport, explicit map[string]bool) {
	if target == nil {
		return
	}
	if _, ok := imported.Fields[tools.GhosttyFieldFontFamily]; ok && !explicit["GhosttyFontFamily"] && imported.Config.FontFamily != "" {
		target.GhosttyFontFamily = imported.Config.FontFamily
	}
	if _, ok := imported.Fields[tools.GhosttyFieldFontSize]; ok && !explicit["GhosttyFontSize"] && imported.Config.FontSize >= 8 && imported.Config.FontSize <= 32 {
		target.GhosttyFontSize = imported.Config.FontSize
	}
	if _, ok := imported.Fields[tools.GhosttyFieldOpacity]; ok && !explicit["GhosttyOpacity"] && imported.Config.Opacity >= 0 && imported.Config.Opacity <= 100 {
		target.GhosttyOpacity = imported.Config.Opacity
	}
	if _, ok := imported.Fields[tools.GhosttyFieldBlurRadius]; ok && !explicit["GhosttyBlurRadius"] && imported.Config.BlurRadius >= 0 && imported.Config.BlurRadius <= 100 {
		target.GhosttyBlurRadius = imported.Config.BlurRadius
	}
	if _, ok := imported.Fields[tools.GhosttyFieldCursorStyle]; ok && !explicit["GhosstyCursorStyle"] && oneOf(imported.Config.CursorStyle, "block", "bar", "underline") {
		target.GhosstyCursorStyle = imported.Config.CursorStyle
	}
	if _, ok := imported.Fields[tools.GhosttyFieldScrollbackLines]; ok && !explicit["GhosttyScrollbackLines"] && imported.Config.ScrollbackLines >= 1_000_000 && imported.Config.ScrollbackLines <= 100_000_000 {
		target.GhosttyScrollbackLines = imported.Config.ScrollbackLines
	}
	if _, ok := imported.Fields[tools.GhosttyFieldWindowDecorations]; ok && !explicit["GhosttyWindowDecorations"] {
		target.GhosttyWindowDecorations = imported.Config.WindowDecorations
	}
	if _, ok := imported.Fields[tools.GhosttyFieldConfirmClose]; ok && !explicit["GhosttyConfirmClose"] {
		target.GhosttyConfirmClose = imported.Config.ConfirmClose
	}
	if _, ok := imported.Fields[tools.GhosttyFieldTabBindings]; ok && !explicit["GhosttyTabBindings"] {
		target.GhosttyTabBindings = imported.Config.TabBindings
	}
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}
