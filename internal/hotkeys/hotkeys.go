package hotkeys

import (
	"strings"
	"unicode"
)

// Item is a single hotkey/cheatsheet entry.
//
// ID is a stable, navStyle-independent identifier for this item within its
// category, formatted as "<categoryID>.<slug>". It is used as the persistence
// key for favorites so that favorites survive navigation-style changes (the Keys
// field changes between emacs/vim, the ID never does).
type Item struct {
	ID          string // stable, navStyle-independent identifier: "<catID>.<slug>"
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

// slugify converts a description into a lowercase, hyphen-separated slug
// suitable for use as the suffix of a stable item ID.
// It lowercases all runes, replaces runs of non-alphanumeric characters with a
// single hyphen, and trims leading/trailing hyphens.
func slugify(s string) string {
	var b strings.Builder
	inSep := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if inSep && b.Len() > 0 {
				b.WriteByte('-')
			}
			inSep = false
			b.WriteRune(r)
		} else {
			inSep = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// makeItem constructs an Item with a stable ID derived from the category ID and
// the item's description. The Keys field may vary with navStyle; the ID never does.
func makeItem(catID, keys, description string) Item {
	return Item{
		ID:          catID + "." + slugify(description),
		Keys:        keys,
		Description: description,
	}
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
		nvimNav = "Arrow keys"
	}

	mk := makeItem // local alias for brevity

	return []Category{
		{
			ID:   "tmux",
			Name: "Tmux",
			Icon: "",
			Items: []Item{
				// Pane management
				mk("tmux", "Prefix + |", "Split pane vertically"),
				mk("tmux", "Prefix + -", "Split pane horizontally"),
				mk("tmux", tmuxNav, "Navigate panes"),
				mk("tmux", "Prefix + H/J/K/L", "Resize panes"),
				mk("tmux", "Prefix + z", "Toggle pane zoom"),
				mk("tmux", "Prefix + x", "Close current pane"),
				mk("tmux", "Prefix + !", "Convert pane to window"),
				mk("tmux", "Prefix + q", "Show pane numbers"),
				mk("tmux", "Prefix + {/}", "Swap pane left/right"),
				// Window management
				mk("tmux", "Prefix + c", "New window"),
				mk("tmux", "Prefix + n/p", "Next/previous window"),
				mk("tmux", "Prefix + 0-9", "Switch to window N"),
				mk("tmux", "Prefix + w", "List windows"),
				mk("tmux", "Prefix + ,", "Rename window"),
				mk("tmux", "Prefix + &", "Close window"),
				mk("tmux", "Prefix + l", "Last active window"),
				// Session management
				mk("tmux", "Prefix + s", "List sessions"),
				mk("tmux", "Prefix + $", "Rename session"),
				mk("tmux", "Prefix + d", "Detach session"),
				mk("tmux", "Prefix + (", "Previous session"),
				mk("tmux", "Prefix + )", "Next session"),
				// Copy mode & misc
				mk("tmux", "Prefix + [", "Copy mode"),
				mk("tmux", "Prefix + ]", "Paste buffer"),
				mk("tmux", "Prefix + r", "Reload config"),
				mk("tmux", "Prefix + ?", "List keybindings"),
			},
		},
		{
			ID:   "zsh",
			Name: zshTitle,
			Icon: "",
			Items: []Item{
				mk("zsh", "Ctrl-r", "Search command history"),
				mk("zsh", "Ctrl-t", "Fuzzy find files (fzf)"),
				mk("zsh", "Alt-c", "Fuzzy cd to directory"),
				mk("zsh", "Tab", "Autocomplete"),
				mk("zsh", "Ctrl-w", "Delete word backwards"),
			},
		},
		{
			ID:   "yazi",
			Name: "Yazi",
			Icon: "󰉋",
			Items: []Item{
				mk("yazi", yaziNav, "Navigate"),
				mk("yazi", yaziHidden, "Toggle hidden files"),
				mk("yazi", "/", "Search"),
				mk("yazi", "Space", "Toggle selection"),
				mk("yazi", "y", "Yank (copy)"),
				mk("yazi", "x", "Cut"),
				mk("yazi", "p", "Paste"),
				mk("yazi", "a", "Create file/dir"),
				mk("yazi", "r", "Rename"),
				mk("yazi", "q", "Quit"),
			},
		},
		{
			ID:   "fzf",
			Name: "fzf",
			Icon: "󰍉",
			Items: []Item{
				mk("fzf", "Ctrl-r", "Fuzzy search history"),
				mk("fzf", "Ctrl-t", "Fuzzy find file"),
				mk("fzf", "Alt-c", "Fuzzy cd into dir"),
				mk("fzf", "**<Tab>", "Path completion"),
			},
		},
		{
			ID:   "ghostty",
			Name: "Ghostty",
			Icon: "󰆍",
			Items: []Item{
				mk("ghostty", "Super-c/v", "Copy/Paste (super)"),
				mk("ghostty", "Super-{/}", "Prev/next tab"),
				mk("ghostty", "Super-1/2/3…", "Switch to tab N"),
				mk("ghostty", "Ctrl-Shift-,", "Reload config"),
				mk("ghostty", "Ctrl-Shift-n", "New window"),
			},
		},
		{
			ID:   "neovim",
			Name: "Neovim",
			Icon: "",
			Items: []Item{
				mk("neovim", "i", "Insert mode"),
				mk("neovim", "Esc", "Normal mode"),
				mk("neovim", nvimNav, "Navigate"),
				mk("neovim", ":w", "Save"),
				mk("neovim", ":q", "Quit"),
				mk("neovim", ":wq", "Save and quit"),
				mk("neovim", "dd", "Delete line"),
				mk("neovim", "yy", "Yank line"),
				mk("neovim", "p", "Paste"),
				mk("neovim", "/", "Search"),
				mk("neovim", "n/N", "Next/prev match"),
			},
		},
		{
			ID:   "lazygit",
			Name: "LazyGit",
			Icon: "󰊢",
			Items: []Item{
				mk("lazygit", "Space", "Stage/unstage file"),
				mk("lazygit", "c", "Commit"),
				mk("lazygit", "P", "Push"),
				mk("lazygit", "p", "Pull"),
				mk("lazygit", "b", "Branches menu"),
				mk("lazygit", "m", "Merge"),
				mk("lazygit", "r", "Rebase"),
				mk("lazygit", "/", "Search"),
				mk("lazygit", "?", "Help"),
				mk("lazygit", "q", "Quit"),
			},
		},
		{
			ID:   "eza",
			Name: "eza",
			Icon: "󰙅",
			Items: []Item{
				mk("eza", "ls", "List with icons"),
				mk("eza", "la", "List all + git"),
				mk("eza", "ll", "Long format + git"),
				mk("eza", "lt", "Tree view"),
			},
		},
		{
			ID:   "zoxide",
			Name: "zoxide",
			Icon: "󰄛",
			Items: []Item{
				mk("zoxide", "cd <query>", "Jump to frecent dir"),
				mk("zoxide", "cd -", "Previous dir"),
				mk("zoxide", "zi", "Interactive selection"),
			},
		},
		{
			ID:   "dotfiles",
			Name: "Dotfiles",
			Icon: "󰒓",
			Items: []Item{
				mk("dotfiles", "dotfiles install", "Launch installer wizard"),
				mk("dotfiles", "dotfiles manage", "Open dual-pane manager"),
				mk("dotfiles", "dotfiles theme", "Change theme"),
				mk("dotfiles", "dotfiles hotkeys", "Hotkey reference TUI"),
				mk("dotfiles", "dotfiles update", "Package update UI"),
			},
		},
		{
			ID:   "git",
			Name: "Git",
			Icon: "",
			Items: []Item{
				mk("git", "git status", "Show working tree status"),
				mk("git", "git add .", "Stage all changes"),
				mk("git", "git commit -m", "Commit with message"),
				mk("git", "git push", "Push to remote"),
				mk("git", "git pull", "Pull from remote"),
				mk("git", "git log --oneline", "Compact commit history"),
				mk("git", "git diff", "Show unstaged changes"),
				mk("git", "git branch", "List branches"),
				mk("git", "git checkout -b", "Create and switch branch"),
				mk("git", "git stash", "Stash changes"),
			},
		},
		{
			ID:   "delta",
			Name: "Delta",
			Icon: "",
			Items: []Item{
				mk("delta", "git diff", "Diff with delta styling"),
				mk("delta", "git show", "Show commit with delta"),
				mk("delta", "git log -p", "Log with patches"),
				mk("delta", "delta --help", "Show delta options"),
			},
		},
		{
			ID:   "lazydocker",
			Name: "LazyDocker",
			Icon: "",
			Items: []Item{
				mk("lazydocker", "d", "Remove container"),
				mk("lazydocker", "s", "Stop container"),
				mk("lazydocker", "r", "Restart container"),
				mk("lazydocker", "a", "Attach to container"),
				mk("lazydocker", "l", "View logs"),
				mk("lazydocker", "[/]", "Prev/next panel"),
				mk("lazydocker", "enter", "Focus panel"),
				mk("lazydocker", "?", "Help"),
				mk("lazydocker", "q", "Quit"),
			},
		},
		{
			ID:   "glow",
			Name: "Glow",
			Icon: "󰈙",
			Items: []Item{
				mk("glow", "glow README.md", "Render markdown file"),
				mk("glow", "glow -p", "Use pager"),
				mk("glow", "glow -s dark", "Dark style"),
				mk("glow", "j/k", "Scroll up/down"),
				mk("glow", "q", "Quit"),
			},
		},
		{
			ID:   "bat",
			Name: "bat",
			Icon: "󰭟",
			Items: []Item{
				mk("bat", "bat file.txt", "View file with syntax highlighting"),
				mk("bat", "bat -A", "Show non-printable characters"),
				mk("bat", "bat -n", "Show line numbers only"),
				mk("bat", "bat --diff", "Show git diff"),
				mk("bat", "bat -l json", "Force language"),
			},
		},
		{
			ID:   "btop",
			Name: "btop",
			Icon: "󰄨",
			Items: []Item{
				mk("btop", "h", "Toggle help"),
				mk("btop", "Esc", "Close menu/go back"),
				mk("btop", "m", "Toggle memory graph"),
				mk("btop", "n", "Toggle network graph"),
				mk("btop", "p", "Toggle process view"),
				mk("btop", "f", "Filter processes"),
				mk("btop", "k", "Kill process"),
				mk("btop", "q", "Quit"),
			},
		},
		{
			ID:   "ripgrep",
			Name: "ripgrep",
			Icon: "󰈞",
			Items: []Item{
				mk("ripgrep", "rg pattern", "Search for pattern"),
				mk("ripgrep", "rg -i pattern", "Case insensitive"),
				mk("ripgrep", "rg -w word", "Match whole word"),
				mk("ripgrep", "rg -t py pattern", "Search Python files"),
				mk("ripgrep", "rg -g '*.js'", "Glob filter"),
				mk("ripgrep", "rg -C 3", "Show 3 lines context"),
				mk("ripgrep", "rg -l pattern", "List matching files only"),
			},
		},
		{
			ID:   "fd",
			Name: "fd",
			Icon: "󰱼",
			Items: []Item{
				mk("fd", "fd pattern", "Find files matching pattern"),
				mk("fd", "fd -e js", "Find by extension"),
				mk("fd", "fd -t d", "Find directories only"),
				mk("fd", "fd -H", "Include hidden files"),
				mk("fd", "fd -x cmd", "Execute command on results"),
			},
		},
		{
			ID:   "claude",
			Name: "Claude Code",
			Icon: "󰚩",
			Items: []Item{
				mk("claude", "claude", "Start Claude Code"),
				mk("claude", "/help", "Show help"),
				mk("claude", "/clear", "Clear conversation"),
				mk("claude", "/compact", "Summarize context"),
				mk("claude", "Ctrl-C", "Cancel current operation"),
				mk("claude", "Esc Esc", "Exit Claude Code"),
			},
		},
		{
			ID:   "tailscale",
			Name: "Tailscale",
			Icon: "󰖂",
			Items: []Item{
				mk("tailscale", "tailscale status", "Show connection status"),
				mk("tailscale", "tailscale up", "Connect to tailnet"),
				mk("tailscale", "tailscale down", "Disconnect from tailnet"),
				mk("tailscale", "tailscale ip", "Show Tailscale IP addresses"),
				mk("tailscale", "tailscale ssh <host>", "SSH to peer node"),
				mk("tailscale", "tailscale ping <host>", "Ping peer node"),
				mk("tailscale", "tailscale netcheck", "Network diagnostic"),
			},
		},
		{
			ID:   "sunshine",
			Name: "Sunshine",
			Icon: "☀",
			Items: []Item{
				mk("sunshine", "sunshine", "Start streaming server"),
				mk("sunshine", "sunshine --help", "Show command options"),
				mk("sunshine", "localhost:47990", "Web UI (browser)"),
			},
		},
		{
			ID:   "moonlight",
			Name: "Moonlight",
			Icon: "🌙",
			Items: []Item{
				mk("moonlight", "moonlight", "Launch client"),
				mk("moonlight", "moonlight pair <host>", "Pair with host"),
				mk("moonlight", "moonlight stream <host>", "Stream from host"),
				mk("moonlight", "moonlight list <host>", "List available apps"),
			},
		},
	}
}
