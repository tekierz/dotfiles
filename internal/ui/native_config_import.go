package ui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/tools"
)

// NativeManageConfigState exposes the observed native source chain and
// per-field provenance to the UI model. Applied is true only on first adoption,
// when no manage.json exists; explicit saved product preferences always win.
type NativeManageConfigState struct {
	Git             tools.GitConfigImport
	Ghostty         tools.GhosttyConfigImport
	GitError        string
	GhosttyError    string
	Applied         bool
	PreferenceError string
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
	exists bool
	fields map[string]bool
	err    error
}

func inspectManagePreferencePresence() managePreferencePresence {
	configDir := config.ConfigDir()
	if configDir == "" {
		return managePreferencePresence{exists: true, err: config.ErrNoConfigDir}
	}
	path := filepath.Join(configDir, "tools", "manage.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return managePreferencePresence{fields: map[string]bool{}}
	}
	if err != nil {
		return managePreferencePresence{exists: true, fields: map[string]bool{}, err: err}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return managePreferencePresence{exists: true, fields: map[string]bool{}, err: err}
	}
	fields := make(map[string]bool, len(raw))
	for key := range raw {
		fields[key] = true
	}
	if !fields["NativeImportSchemaVersion"] {
		// One-time adoption migration for prototype-era full-struct saves. Those
		// files had no way to distinguish deliberate choices from copied defaults.
		for key := range fields {
			if strings.HasPrefix(key, "Git") || strings.HasPrefix(key, "Ghostty") || strings.HasPrefix(key, "Ghossty") {
				delete(fields, key)
			}
		}
	}
	return managePreferencePresence{exists: true, fields: fields}
}

func observeNativeManageConfig(target *ManageConfig, preferences managePreferencePresence) NativeManageConfigState {
	state := NativeManageConfigState{}
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
	state.Applied = preferences.err == nil && target != nil && *target != before
	if preferences.err != nil {
		state.PreferenceError = preferences.err.Error()
	}
	return state
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
