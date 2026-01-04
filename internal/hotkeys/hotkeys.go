package hotkeys

import "strings"

// Item is a single hotkey/cheatsheet entry.
type Item struct {
	Keys        string
	Description string
}

// Category groups hotkeys/cheatsheet items by tool or domain.
type Category struct {
	ID    string
	Name  string
	Icon  string
	Items []Item
}

// NavStyle is the navigation style used for bindings where applicable.
// Supported: "vim", "emacs". Unknown values will fall back to "emacs".
type NavStyle string

const (
	NavVim   NavStyle = "vim"
	NavEmacs NavStyle = "emacs"
)

func normalizeNavStyle(s string) NavStyle {
	if strings.EqualFold(s, string(NavVim)) {
		return NavVim
	}
	return NavEmacs
}

// Categories returns the hotkey categories for the provided navigation style.
//
// This is the single source of truth used by:
// - the Hotkeys TUI screen
// - contextual hotkey/docs in the manager
func Categories(navStyle string) []Category {
	nav := normalizeNavStyle(navStyle)

	// NOTE: Keep this list user-centric and concise. If something belongs in
	// "docs", we can add more later, but the default view should be scannable.

	tmuxNav := "Alt-h/j/k/l"
	zshTitle := "Zsh (vim mode)"
	yaziNav := "h/j/k/l"
	yaziHidden := "."
	nvimNav := "h/j/k/l"
	if nav == NavEmacs {
		tmuxNav = "Alt-Arrow"
		zshTitle = "Zsh (emacs/Mac-style)"
		yaziNav = "Arrow keys"
		yaziHidden = "Ctrl-h"
		nvimNav = "Arrow keys"
	}

	return []Category{
		{
			ID:   "tmux",
			Name: "Tmux",
			Icon: "",
			Items: []Item{
				{"Prefix + |", "Split pane vertically"},
				{"Prefix + -", "Split pane horizontally"},
				{tmuxNav, "Navigate panes"},
				{"Prefix + H/J/K/L", "Resize panes"},
				{"Prefix + c", "New window"},
				{"Prefix + n/p", "Next/previous window"},
				{"Prefix + [", "Copy mode"},
				{"Prefix + r", "Reload config"},
				{"Prefix + d", "Detach session"},
			},
		},
		{
			ID:   "zsh",
			Name: zshTitle,
			Icon: "",
			Items: []Item{
				{"Ctrl-r", "Search command history"},
				{"Ctrl-t", "Fuzzy find files (fzf)"},
				{"Alt-c", "Fuzzy cd to directory"},
				{"Ctrl-g", "Fuzzy find git files"},
				{"Tab", "Autocomplete"},
				{"Ctrl-w", "Delete word backwards"},
			},
		},
		{
			ID:   "yazi",
			Name: "Yazi",
			Icon: "󰉋",
			Items: []Item{
				{yaziNav, "Navigate"},
				{yaziHidden, "Toggle hidden files"},
				{"/", "Search"},
				{"Space", "Toggle selection"},
				{"y", "Yank (copy)"},
				{"x", "Cut"},
				{"p", "Paste"},
				{"a", "Create file/dir"},
				{"r", "Rename"},
				{"q", "Quit"},
			},
		},
		{
			ID:   "fzf",
			Name: "fzf",
			Icon: "󰍉",
			Items: []Item{
				{"Ctrl-r", "Fuzzy search history"},
				{"Ctrl-t", "Fuzzy find file"},
				{"Alt-c", "Fuzzy cd into dir"},
				{"**<Tab>", "Path completion"},
			},
		},
		{
			ID:   "ghostty",
			Name: "Ghostty",
			Icon: "󰆍",
			Items: []Item{
				{"Super-c/v", "Copy/Paste (super)"},
				{"Super-{/}", "Prev/next tab"},
				{"Super-1/2/3…", "Switch to tab N"},
				{"Ctrl-Shift-,", "Reload config"},
				{"Ctrl-Shift-n", "New window"},
			},
		},
		{
			ID:   "neovim",
			Name: "Neovim",
			Icon: "",
			Items: []Item{
				{"i", "Insert mode"},
				{"Esc", "Normal mode"},
				{nvimNav, "Navigate"},
				{":w", "Save"},
				{":q", "Quit"},
				{":wq", "Save and quit"},
				{"dd", "Delete line"},
				{"yy", "Yank line"},
				{"p", "Paste"},
				{"/", "Search"},
				{"n/N", "Next/prev match"},
			},
		},
		{
			ID:   "lazygit",
			Name: "LazyGit",
			Icon: "󰊢",
			Items: []Item{
				{"Space", "Stage/unstage file"},
				{"c", "Commit"},
				{"P", "Push"},
				{"p", "Pull"},
				{"b", "Branches menu"},
				{"m", "Merge"},
				{"r", "Rebase"},
				{"/", "Search"},
				{"?", "Help"},
				{"q", "Quit"},
			},
		},
		{
			ID:   "eza",
			Name: "eza",
			Icon: "󰙅",
			Items: []Item{
				{"ls", "List with icons"},
				{"la", "List all + git"},
				{"ll", "Long format + git"},
				{"lt", "Tree view"},
			},
		},
		{
			ID:   "zoxide",
			Name: "zoxide",
			Icon: "󰄛",
			Items: []Item{
				{"cd <query>", "Jump to frecent dir"},
				{"cd -", "Previous dir"},
				{"zi", "Interactive selection"},
			},
		},
		{
			ID:   "dotfiles",
			Name: "Dotfiles",
			Icon: "󰒓",
			Items: []Item{
				{"dotfiles install", "Launch installer wizard"},
				{"dotfiles manage", "Open dual-pane manager"},
				{"dotfiles theme", "Change theme"},
				{"dotfiles hotkeys", "Hotkey reference TUI"},
				{"dotfiles update", "Package update UI"},
			},
		},
	}
}
