package ui

import (
	"strings"
)

// manageGeneratorToolOrder is the stable set of tool IDs whose config files the
// Manage save can write, in the deterministic order used for applying changes and
// reporting errors. It mirrors the install worker's all-tools order (minus
// claude-code, which is gated and handled separately) so the scoped Manage save
// and the install path agree on which tools exist.
var manageGeneratorToolOrder = []string{
	"ghostty", "tmux", "zsh", "neovim", "git", "yazi", "fzf", "lazygit", "btop", "glow",
}

// toolDeepDiveFields returns the slice of DeepDiveConfig values that the named
// tool's generator actually consumes (see toolConfigGenerators). Diffing these
// per tool — rather than the raw ManageConfig fields — guarantees the change
// detection can never drift from what the generators read: if a generator stops
// using a field, that field stops affecting the diff automatically.
//
// The values are returned as a comparable []any so two snapshots can be compared
// element-by-element without reflection over the whole DeepDiveConfig.
func toolDeepDiveFields(toolID string, cfg DeepDiveConfig) []any {
	switch toolID {
	case "ghostty":
		return []any{
			cfg.GhosttyFontSize, cfg.GhosttyFontFamily, cfg.GhosttyOpacity,
			cfg.GhosttyBlurRadius, cfg.GhosttyTabBindings, cfg.GhosttyScrollbackLines,
			cfg.GhosttyCursorStyle, cfg.GhosttyWindowDecorations, cfg.GhosttyConfirmClose,
		}
	case "tmux":
		return []any{
			cfg.TmuxPrefix, cfg.TmuxSplitBinds, cfg.TmuxStatusBar, cfg.TmuxMouseMode,
			cfg.TmuxBaseIndex, cfg.TmuxPaneBorderStyle, cfg.TmuxHistoryLimit,
			cfg.TmuxEscapeTime, cfg.TmuxAggressiveResize,
			cfg.TmuxTPMEnabled, cfg.TmuxPluginSensible, cfg.TmuxPluginResurrect,
			cfg.TmuxPluginContinuum, cfg.TmuxPluginYank, cfg.TmuxContinuumSaveMin,
			cfg.TmuxContinuumRestore,
		}
	case "zsh":
		return []any{
			cfg.ZshPromptStyle, cfg.ZshHistorySize, cfg.ZshAutoCD,
			cfg.ZshSyntaxHighlight, cfg.ZshAutosuggestions,
			cfg.ZshHistoryIgnoreDups, cfg.ZshCorrection, cfg.ZshCompletionMenu,
		}
	case "neovim":
		return []any{
			cfg.NeovimConfig, cfg.NeovimTabWidth, cfg.NeovimWrap,
			cfg.NeovimCursorLine, cfg.NeovimClipboard,
			cfg.NeovimLineNumbers, cfg.NeovimExpandTab,
			cfg.NeovimUndoFile,
		}
	case "git":
		return []any{
			cfg.GitDeltaSideBySide, cfg.GitDefaultBranch, cfg.GitPullRebase,
			cfg.GitSignCommits, cfg.GitCredentialHelper,
			cfg.GitAutoSetupRemote, cfg.GitMergeTool, cfg.GitDiffTool,
			strings.Join(cfg.GitAliases, "\x00"),
		}
	case "yazi":
		return []any{
			cfg.YaziKeymap, cfg.YaziShowHidden, cfg.YaziPreviewMode, cfg.YaziSortBy, cfg.YaziSortReverse,
			cfg.YaziLineMode, cfg.YaziScrollOff,
		}
	case "fzf":
		return []any{
			cfg.FzfPreview, cfg.FzfHeight, cfg.FzfLayout,
			cfg.FzfDefaultOpts, cfg.FzfBorderStyle, cfg.FzfPreviewWindow,
		}
	case "lazygit":
		return []any{
			cfg.LazyGitSideBySide, cfg.LazyGitMouseMode, cfg.LazyGitTheme,
			cfg.LazyGitPaging,
		}
	case "btop":
		return []any{
			cfg.BtopTheme, cfg.BtopUpdateMs, cfg.BtopShowTemp, cfg.BtopGraphType,
			cfg.BtopTempScale, cfg.BtopShownBoxes,
		}
	case "glow":
		return []any{cfg.GlowPager, cfg.GlowStyle, cfg.GlowWidth, cfg.GlowMouse, cfg.GlowAll, cfg.GlowShowLineNumbers, cfg.GlowPreserveNewLines}
	}
	return nil
}

// claudeCodeChanged reports whether the Claude Code MCP selection differs between
// two DeepDiveConfigs. claude-code is not in toolConfigGenerators (its apply is
// gated on having any server configured), so its change detection is separate.
func claudeCodeChanged(base, cur DeepDiveConfig) bool {
	if len(base.ClaudeCodeMCPs) != len(cur.ClaudeCodeMCPs) {
		return true
	}
	for k, v := range cur.ClaudeCodeMCPs {
		if base.ClaudeCodeMCPs[k] != v {
			return true
		}
	}
	return false
}

func sliceEqual(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// changedManageTools returns the IDs of the tools whose generated config differs
// between the baseline (last loaded/saved) and current ManageConfig — i.e. the
// ONLY tools whose config files a Manage save should rewrite. This is the
// data-loss fix (P1-A2): a single tool's edit must never rewrite another tool's
// config file from manage.json defaults.
//
// Theme is deliberately not part of this diff. A global theme selection is
// persisted as desired state, but it must not synthesize or rewrite every tool's
// config from Manage defaults. Theme-dependent artifacts are applied through the
// reviewed install plan, where selection, ownership, backup scope, and rollback
// are explicit. Only tools whose own modeled fields changed are returned here.
//
// The result is ordered per manageGeneratorToolOrder (claude-code last) so apply
// and error reporting are deterministic.
func changedManageTools(baseline, current *ManageConfig, _, _ string) []string {
	base := manageConfigToDeepDive(baseline)
	cur := manageConfigToDeepDive(current)

	var changed []string
	for _, id := range manageGeneratorToolOrder {
		if !sliceEqual(toolDeepDiveFields(id, base), toolDeepDiveFields(id, cur)) {
			changed = append(changed, id)
		}
	}
	if claudeCodeChanged(base, cur) {
		changed = append(changed, "claude-code")
	}
	return changed
}
