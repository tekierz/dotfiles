package ui

// Repeated string literals used across the ui package, extracted into named
// constants so the values are defined once (and to satisfy goconst). The values
// are unchanged from the literals they replace, so runtime behavior is identical.

// Bubble Tea key strings (the values returned by tea.KeyMsg.String()).
const (
	keyEnter     = "enter"
	keyEsc       = "esc"
	keyDown      = "down"
	keyRight     = "right"
	keyLeft      = "left"
	keyTab       = "tab"
	keyCtrlC     = "ctrl+c"
	keyBackspace = "backspace"
	keyDelete    = "delete"
)

// Navigation styles / editor keymap modes (also used as yazi keymap values).
const (
	navEmacs = "emacs"
	navVim   = "vim"
)

// Tool / config identifiers reused across save-scope and field logic.
const (
	toolGhostty = "ghostty"
	toolTmux    = "tmux"
	toolNeovim  = "neovim"
	toolLazygit = "lazygit"
	toolBtop    = "btop"
)

// Manage-screen field keys and the synthetic "global" settings item id.
const (
	manageItemGlobal = "global"
	manageFieldTheme = "theme"
	manageFieldAnims = "animations"
)

// Miscellaneous repeated values.
const (
	defaultTheme   = "catppuccin-mocha"
	platformLinux  = "linux"
	statusPending  = "pending"
	optionDefault  = "default"
	optionNever    = "never"
	tmuxStatusTop  = "top"
	loadingMessage = "Loading..."
)
