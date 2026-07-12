package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// manageConfigToolIDs is the set of tool IDs whose Manage field tables this
// guardrail covers (everything manageFieldsFor returns fields for except the
// "global" pseudo-tool and claude-code, whose MCP toggles apply through a separate
// gated path). It is the iteration source for the completeness checks.
var manageConfigToolIDs = []string{
	"ghostty", "tmux", "zsh", "neovim", "git", "yazi",
	"fzf", "lazygit", "lazydocker", "btop", "glow",
}

// appForFields builds a minimal App sufficient for manageFieldsFor to enumerate a
// tool's editable fields.
func appForFields() *App {
	return &App{
		manageConfig:      NewManageConfig(),
		theme:             "catppuccin-mocha",
		navStyle:          "emacs",
		animationsEnabled: true,
	}
}

// manageFieldRoundTrip describes how to prove one applied Manage field reaches its
// generated config file: mutate sets the ManageConfig field away from its default,
// file is the generated path (relative to HOME), and want is a substring the
// mutated value must produce in that file.
//
// want is the positive-token assertion (a specific string the mutated value must
// produce). The DIFFERENTIAL backstop (TestManageAppliedFieldsRoundTrip) does not
// rely on want: it regenerates the file with the BASELINE value and again with the
// MUTATED value and requires the two outputs to DIFFER, so a field whose change
// produces no on-disk difference fails even if a static header still matches. A
// few fields legitimately have no single positive token (e.g. a boolean whose
// "off" state OMITS a line); those set want to the static header and rely on the
// differential check — that is sound because the differential check still proves
// the line was removed.
type manageFieldRoundTrip struct {
	mutate func(mc *ManageConfig)
	file   string // path under HOME
	want   string
}

// manageAppliedRoundTrips is the round-trip proof table: every Manage field
// classified as APPLIED (i.e. NOT in manageNotAppliedFields) must have an entry
// here, keyed "toolID/fieldKey". Running the entry through the real scoped Manage
// legacy direct-generator mapping path (manageConfigToDeepDive -> changedManageTools)
// and finding `want` in `file` proves the field is wired end-to-end:
// struct -> manageConfigToDeepDive -> toolDeepDiveFields (so the scoped diff
// detects it) -> generator (so the file actually changes). The guardrail test
// fails if any applied field lacks an entry here OR if its entry does not
// round-trip.
var manageAppliedRoundTrips = map[string]manageFieldRoundTrip{
	// Ghostty
	"ghostty/font_family":   {func(mc *ManageConfig) { mc.GhosttyFontFamily = "Fira Code" }, ".config/ghostty/config.ghostty", "Fira Code"},
	"ghostty/font_size":     {func(mc *ManageConfig) { mc.GhosttyFontSize = 21 }, ".config/ghostty/config.ghostty", "font-size = 21"},
	"ghostty/opacity":       {func(mc *ManageConfig) { mc.GhosttyOpacity = 80 }, ".config/ghostty/config.ghostty", "background-opacity = 0.80"},
	"ghostty/blur":          {func(mc *ManageConfig) { mc.GhosttyBlurRadius = 12 }, ".config/ghostty/config.ghostty", "background-blur = 12"},
	"ghostty/cursor":        {func(mc *ManageConfig) { mc.GhosstyCursorStyle = "bar" }, ".config/ghostty/config.ghostty", "cursor-style = bar"},
	"ghostty/scrollback":    {func(mc *ManageConfig) { mc.GhosttyScrollbackLines = 12345 }, ".config/ghostty/config.ghostty", "scrollback-limit = 12345"},
	"ghostty/decor":         {func(mc *ManageConfig) { mc.GhosttyWindowDecorations = false }, ".config/ghostty/config.ghostty", "window-decoration = false"},
	"ghostty/confirm_close": {func(mc *ManageConfig) { mc.GhosttyConfirmClose = false }, ".config/ghostty/config.ghostty", "confirm-close-surface = false"},
	"ghostty/tab_bindings":  {func(mc *ManageConfig) { mc.GhosttyTabBindings = "ctrl" }, ".config/ghostty/config.ghostty", "keybind = ctrl+t=new_tab"},

	// Tmux
	"tmux/prefix":            {func(mc *ManageConfig) { mc.TmuxPrefix = "C-b" }, ".tmux.conf", "set -g prefix C-b"},
	"tmux/split_binds":       {func(mc *ManageConfig) { mc.TmuxSplitBinds = "pipes" }, ".tmux.conf", "bind | split-window -h"},
	"tmux/base":              {func(mc *ManageConfig) { mc.TmuxBaseIndex = 0 }, ".tmux.conf", "set -g base-index 0"},
	"tmux/mouse":             {func(mc *ManageConfig) { mc.TmuxMouseMode = false }, ".tmux.conf", "set -g mouse off"},
	"tmux/status_pos":        {func(mc *ManageConfig) { mc.TmuxStatusPosition = "top" }, ".tmux.conf", "set -g status-position top"},
	"tmux/pane_border":       {func(mc *ManageConfig) { mc.TmuxPaneBorderStyle = "double" }, ".tmux.conf", "pane-border-lines double"},
	"tmux/history":           {func(mc *ManageConfig) { mc.TmuxHistoryLimit = 12345 }, ".tmux.conf", "set -g history-limit 12345"},
	"tmux/escape":            {func(mc *ManageConfig) { mc.TmuxEscapeTime = 25 }, ".tmux.conf", "set -sg escape-time 25"},
	"tmux/resize":            {func(mc *ManageConfig) { mc.TmuxAggressiveResize = false }, ".tmux.conf", "aggressive-resize off"},
	"tmux/tpm_enabled":       {func(mc *ManageConfig) { mc.TmuxTPMEnabled = false }, ".tmux.conf", "Generated by dotfiles"},
	"tmux/plugin_sensible":   {func(mc *ManageConfig) { mc.TmuxPluginSensible = false }, ".tmux.conf", "Generated by dotfiles"},
	"tmux/plugin_resurrect":  {func(mc *ManageConfig) { mc.TmuxPluginResurrect = false }, ".tmux.conf", "Generated by dotfiles"},
	"tmux/plugin_continuum":  {func(mc *ManageConfig) { mc.TmuxPluginContinuum = true }, ".tmux.conf", "tmux-continuum"},
	"tmux/plugin_yank":       {func(mc *ManageConfig) { mc.TmuxPluginYank = false }, ".tmux.conf", "Generated by dotfiles"},
	"tmux/continuum_save":    {func(mc *ManageConfig) { mc.TmuxPluginContinuum = true; mc.TmuxContinuumSaveMin = 45 }, ".tmux.conf", "@continuum-save-interval '45'"},
	"tmux/continuum_restore": {func(mc *ManageConfig) { mc.TmuxPluginContinuum = true; mc.TmuxContinuumRestore = false }, ".tmux.conf", "@continuum-restore 'off'"},

	// Zsh
	"zsh/hist_size": {func(mc *ManageConfig) { mc.ZshHistorySize = 12345 }, ".zshrc", "HISTSIZE=12345"},
	"zsh/hist_dups": {func(mc *ManageConfig) { mc.ZshHistoryIgnoreDups = false }, ".zshrc", "Generated by dotfiles"},
	"zsh/autocd":    {func(mc *ManageConfig) { mc.ZshAutoCD = false }, ".zshrc", "Generated by dotfiles"},
	"zsh/correct":   {func(mc *ManageConfig) { mc.ZshCorrection = false }, ".zshrc", "Generated by dotfiles"},
	"zsh/menu":      {func(mc *ManageConfig) { mc.ZshCompletionMenu = false }, ".zshrc", "Generated by dotfiles"},
	"zsh/syntax":    {func(mc *ManageConfig) { mc.ZshSyntaxHighlight = false }, ".zshrc", "Generated by dotfiles"},
	"zsh/autosug":   {func(mc *ManageConfig) { mc.ZshAutosuggestions = false }, ".zshrc", "Generated by dotfiles"},

	// Neovim (requires an existing ~/.config/nvim; the test seeds it).
	// "numbers" is now the SINGLE control for both line-number opts: it drives
	// vim.opt.number (off only when "none") AND vim.opt.relativenumber (on only
	// when "relative"). See TestNeovimNumbersDrivesBothOpts for the per-value pair.
	"neovim/numbers": {func(mc *ManageConfig) { mc.NeovimLineNumbers = "none" }, ".config/nvim/lua/custom/options.lua", "vim.opt.number = false"},
	"neovim/tab":     {func(mc *ManageConfig) { mc.NeovimTabWidth = 8 }, ".config/nvim/lua/custom/options.lua", "vim.opt.tabstop = 8"},
	"neovim/expand":  {func(mc *ManageConfig) { mc.NeovimExpandTab = false }, ".config/nvim/lua/custom/options.lua", "vim.opt.expandtab = false"},
	"neovim/wrap":    {func(mc *ManageConfig) { mc.NeovimWrap = true }, ".config/nvim/lua/custom/options.lua", "vim.opt.wrap = true"},
	"neovim/cursor":  {func(mc *ManageConfig) { mc.NeovimCursorLine = false }, ".config/nvim/lua/custom/options.lua", "vim.opt.cursorline = false"},
	"neovim/clip":    {func(mc *ManageConfig) { mc.NeovimClipboard = "unnamed" }, ".config/nvim/lua/custom/options.lua", "vim.opt.clipboard = \"unnamed\""},
	"neovim/undo":    {func(mc *ManageConfig) { mc.NeovimUndoFile = false }, ".config/nvim/lua/custom/options.lua", "vim.opt.undofile = false"},

	// Git
	"git/branch":       {func(mc *ManageConfig) { mc.GitDefaultBranch = "develop" }, ".config/dotfiles/git/config", "defaultBranch = develop"},
	"git/setup_remote": {func(mc *ManageConfig) { mc.GitAutoSetupRemote = false }, ".config/dotfiles/git/config", "autoSetupRemote = false"},
	"git/rebase":       {func(mc *ManageConfig) { mc.GitPullRebase = false }, ".config/dotfiles/git/config", "rebase = false"},
	"git/diff":         {func(mc *ManageConfig) { mc.GitDiffTool = "difftastic" }, ".config/dotfiles/git/config", "external = difft"},
	"git/merge":        {func(mc *ManageConfig) { mc.GitMergeTool = "meld" }, ".config/dotfiles/git/config", "tool = meld"},
	"git/creds":        {func(mc *ManageConfig) { mc.GitCredentialHelper = "store" }, ".config/dotfiles/git/config", "helper = store"},
	"git/sign":         {func(mc *ManageConfig) { mc.GitSignCommits = true }, ".config/dotfiles/git/config", "gpgsign = true"},
	"git/delta_side":   {func(mc *ManageConfig) { mc.GitDeltaSideBySide = false }, ".config/dotfiles/git/config", "side-by-side = false"},
	"git/alias_st":     {func(mc *ManageConfig) { mc.GitAliasStatus = false }, ".config/dotfiles/git/config", "Generated by dotfiles"},
	"git/alias_co":     {func(mc *ManageConfig) { mc.GitAliasCheckout = false }, ".config/dotfiles/git/config", "Generated by dotfiles"},
	"git/alias_br":     {func(mc *ManageConfig) { mc.GitAliasBranch = false }, ".config/dotfiles/git/config", "Generated by dotfiles"},
	"git/alias_ci":     {func(mc *ManageConfig) { mc.GitAliasCommit = false }, ".config/dotfiles/git/config", "Generated by dotfiles"},
	"git/alias_lg":     {func(mc *ManageConfig) { mc.GitAliasLogGraph = false }, ".config/dotfiles/git/config", "Generated by dotfiles"},

	// Yazi
	"yazi/keymap":       {func(mc *ManageConfig) { mc.YaziKeymap = "emacs" }, ".config/yazi/keymap.toml", `on = "<C-p>"`},
	"yazi/hidden":       {func(mc *ManageConfig) { mc.YaziShowHidden = true }, ".config/yazi/yazi.toml", "show_hidden = true"},
	"yazi/preview_mode": {func(mc *ManageConfig) { mc.YaziPreviewMode = "never" }, ".config/yazi/yazi.toml", "previewers = []"},
	"yazi/sort_by":      {func(mc *ManageConfig) { mc.YaziSortBy = "modified" }, ".config/yazi/yazi.toml", "sort_by = \"mtime\""},
	"yazi/sort_rev":     {func(mc *ManageConfig) { mc.YaziSortReverse = true }, ".config/yazi/yazi.toml", "sort_reverse = true"},
	"yazi/linemode":     {func(mc *ManageConfig) { mc.YaziLineMode = "permissions" }, ".config/yazi/yazi.toml", "linemode = \"permissions\""},
	"yazi/scrolloff":    {func(mc *ManageConfig) { mc.YaziScrollOff = 9 }, ".config/yazi/yazi.toml", "scrolloff = 9"},

	// FZF
	"fzf/opts":           {func(mc *ManageConfig) { mc.FzfDefaultOpts = "--cycle" }, ".config/fzf/fzf.zsh", "--cycle"},
	"fzf/height":         {func(mc *ManageConfig) { mc.FzfHeight = 55 }, ".config/fzf/fzf.zsh", "--height=55%"},
	"fzf/layout":         {func(mc *ManageConfig) { mc.FzfLayout = "reverse-list" }, ".config/fzf/fzf.zsh", "--layout=reverse-list"},
	"fzf/border":         {func(mc *ManageConfig) { mc.FzfBorderStyle = "sharp" }, ".config/fzf/fzf.zsh", "--border=sharp"},
	"fzf/preview":        {func(mc *ManageConfig) { mc.FzfPreview = false }, ".config/fzf/fzf.zsh", "Generated by dotfiles"},
	"fzf/preview_window": {func(mc *ManageConfig) { mc.FzfPreviewWindow = "up:50%" }, ".config/fzf/fzf.zsh", "--preview-window=up:50%"},

	// LazyGit
	"lazygit/side":      {func(mc *ManageConfig) { mc.LazyGitSideBySide = false }, ".config/lazygit/config.yml", "Generated by dotfiles"},
	"lazygit/paging":    {func(mc *ManageConfig) { mc.LazyGitPaging = "never" }, ".config/lazygit/config.yml", "pager: cat"},
	"lazygit/mouse":     {func(mc *ManageConfig) { mc.LazyGitMouseMode = false }, ".config/lazygit/config.yml", "mouseEvents: false"},
	"lazygit/gui_theme": {func(mc *ManageConfig) { mc.LazyGitGuiTheme = "light" }, ".config/lazygit/config.yml", "selectedLineBgColor"},

	// Btop
	"btop/theme": {func(mc *ManageConfig) { mc.BtopTheme = "nord" }, ".config/btop/btop.conf", "color_theme = \"nord\""},
	"btop/rate":  {func(mc *ManageConfig) { mc.BtopUpdateMs = 750 }, ".config/btop/btop.conf", "update_ms = 750"},
	"btop/temp":  {func(mc *ManageConfig) { mc.BtopShowTemp = false }, ".config/btop/btop.conf", "show_coretemp = false"},
	"btop/scale": {func(mc *ManageConfig) { mc.BtopTempScale = "fahrenheit" }, ".config/btop/btop.conf", "temp_scale = \"fahrenheit\""},
	"btop/graph": {func(mc *ManageConfig) { mc.BtopGraphSymbol = "block" }, ".config/btop/btop.conf", "graph_symbol = \"block\""},
	"btop/boxes": {func(mc *ManageConfig) { mc.BtopShownBoxes = "cpu mem" }, ".config/btop/btop.conf", "shown_boxes = \"cpu mem\""},

	// Glow
	"glow/style":             {func(mc *ManageConfig) { mc.GlowStyle = "light" }, glowTestRelPath(), "style: \"light\""},
	"glow/pager":             {func(mc *ManageConfig) { mc.GlowPager = "auto" }, glowTestRelPath(), "pager: true"},
	"glow/width":             {func(mc *ManageConfig) { mc.GlowWidth = 123 }, glowTestRelPath(), "width: 123"},
	"glow/mouse":             {func(mc *ManageConfig) { mc.GlowMouse = true }, glowTestRelPath(), "mouse: true"},
	"glow/all":               {func(mc *ManageConfig) { mc.GlowAll = true }, glowTestRelPath(), "all: true"},
	"glow/line_numbers":      {func(mc *ManageConfig) { mc.GlowShowLineNumbers = true }, glowTestRelPath(), "showLineNumbers: true"},
	"glow/preserve_newlines": {func(mc *ManageConfig) { mc.GlowPreserveNewLines = true }, glowTestRelPath(), "preserveNewLines: true"},
}

// TestManageFieldsAllClassified is the completeness half of the guardrail: every
// field key the Manage UI presents as editable-and-saving must be classified as
// either applied (has a manageAppliedRoundTrips entry) or explicitly not-applied
// (in manageNotAppliedFields). This is what makes "no control may report success
// while doing nothing" unable to silently regress: add a new editable field and
// this test fails until it is wired or marked.
func TestManageFieldsAllClassified(t *testing.T) {
	app := appForFields()

	for _, toolID := range manageConfigToolIDs {
		fields := app.manageFieldsFor(toolID)
		if len(fields) == 0 {
			t.Errorf("tool %q presents no fields (manageConfigToolIDs out of sync?)", toolID)
			continue
		}
		for _, f := range fields {
			key := toolID + "/" + f.key
			applied := manageFieldIsApplied(toolID, f.key)
			_, hasRoundTrip := manageAppliedRoundTrips[key]

			if applied {
				if !hasRoundTrip {
					t.Errorf("field %q is classified APPLIED but has no round-trip proof in manageAppliedRoundTrips; either wire it through a generator (and add the round-trip) or list it in manageNotAppliedFields", key)
				}
			} else {
				if hasRoundTrip {
					t.Errorf("field %q is in manageNotAppliedFields yet also has a round-trip entry; remove one", key)
				}
			}
		}
	}
}

// TestManageNotAppliedKeysAreReal is the reverse-direction guard: every key listed
// in manageNotAppliedFields must correspond to a field the UI actually presents, so
// the not-applied list can never contain stale keys that mark nothing.
func TestManageNotAppliedKeysAreReal(t *testing.T) {
	app := appForFields()
	for toolID, keys := range manageNotAppliedFields {
		presented := map[string]bool{}
		for _, f := range app.manageFieldsFor(toolID) {
			presented[f.key] = true
		}
		for key := range keys {
			if !presented[key] {
				t.Errorf("manageNotAppliedFields[%q][%q] does not match any presented field", toolID, key)
			}
		}
	}
}

// seedNeovimIfNeeded creates the ~/.config/nvim dir the neovim pure writer
// requires (it only overlays an EXISTING config). Shared by the differential
// generations so both baseline and mutated writes find the seeded preset.
func seedNeovimIfNeeded(t *testing.T, key, home string) {
	t.Helper()
	if !strings.HasPrefix(key, "neovim/") {
		return
	}
	nvimDir := filepath.Join(home, ".config", "nvim")
	if err := os.MkdirAll(nvimDir, 0o755); err != nil {
		t.Fatalf("seed nvim dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nvimDir, "init.lua"), []byte("-- seed\n"), 0o644); err != nil {
		t.Fatalf("seed init.lua: %v", err)
	}
}

// generateToolFileFor writes the given tool's config for the given ManageConfig
// into a fresh temp HOME and returns the generated file content. It uses the real
// scoped generator path (manageConfigToDeepDive -> applyOneToolConfig) so it
// captures exactly what a save would write — but writes UNCONDITIONALLY (no diff
// gate) so it can capture the baseline output too. Returns "" if the tool wrote no
// file (e.g. lazygit when deselected — not the case for the tools in this table).
func generateToolFileFor(t *testing.T, key, toolID string, mc *ManageConfig, relFile, theme string) string {
	t.Helper()
	home := withTempHome(t)
	seedNeovimIfNeeded(t, key, home)
	if errs := applyOneToolConfig(toolID, manageConfigToDeepDive(mc), theme); len(errs) > 0 {
		t.Fatalf("applyOneToolConfig(%q) errors: %v", toolID, errs)
	}
	data, err := os.ReadFile(filepath.Join(home, relFile))
	if err != nil {
		t.Fatalf("read %s: %v", relFile, err)
	}
	return string(data)
}

// TestManageAppliedFieldsRoundTrip is the apply half of the guardrail, now
// DIFFERENTIAL so a no-op field can no longer pass. For every applied field it:
//
//  1. proves the scoped save DETECTS the change (changedManageTools is non-empty),
//     closing the "added to a generator but not toolDeepDiveFields" under-apply
//     trap; and
//  2. proves the change actually ALTERS the generated file: it regenerates the
//     tool's config with the BASELINE value and again with the MUTATED value (both
//     via the real generator path) and requires the two outputs to DIFFER. A field
//     that changes nothing on disk FAILS here even though a static header still
//     matches — this is what catches the git/diff and lazygit/gui_theme lies.
//
// It also keeps the positive-token assertion (rt.want) against the mutated output,
// so where a specific token is expected it is still verified.
func TestManageAppliedFieldsRoundTrip(t *testing.T) {
	const theme = "catppuccin-mocha"
	for key, rt := range manageAppliedRoundTrips {
		key, rt := key, rt
		t.Run(key, func(t *testing.T) {
			toolID := key[:strings.IndexByte(key, '/')]

			baseline := NewManageConfig()
			current := NewManageConfig()
			rt.mutate(current)

			// (1) The scoped diff must detect this field's change.
			changed := changedManageTools(baseline, current, theme, theme)
			if len(changed) == 0 {
				t.Fatalf("mutating %q produced no detected change; field is not wired into toolDeepDiveFields (under-apply trap)", key)
			}

			// (2) Differential backstop: baseline vs mutated generated output must
			// differ. This is the no-op detector — a field that writes the same file
			// regardless of its value fails here.
			baseOut := generateToolFileFor(t, key, toolID, baseline, rt.file, theme)
			mutOut := generateToolFileFor(t, key, toolID, current, rt.file, theme)
			if baseOut == mutOut {
				t.Fatalf("field %q is a no-op: mutating it did not change %s (generator ignores the field)", key, rt.file)
			}

			// Positive-token assertion against the mutated output.
			if !strings.Contains(mutOut, rt.want) {
				t.Errorf("field %q did not round-trip: %q not found in %s:\n%s", key, rt.want, rt.file, mutOut)
			}
		})
	}
}
