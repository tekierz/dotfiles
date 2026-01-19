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
			Icon: "",
			Items: []Item{
				// Pane management
				{"Prefix + |", "Split pane vertically"},
				{"Prefix + -", "Split pane horizontally"},
				{tmuxNav, "Navigate panes"},
				{"Prefix + H/J/K/L", "Resize panes"},
				{"Prefix + z", "Toggle pane zoom"},
				{"Prefix + x", "Close current pane"},
				{"Prefix + !", "Convert pane to window"},
				{"Prefix + q", "Show pane numbers"},
				{"Prefix + {/}", "Swap pane left/right"},
				// Window management
				{"Prefix + c", "New window"},
				{"Prefix + n/p", "Next/previous window"},
				{"Prefix + 0-9", "Switch to window N"},
				{"Prefix + w", "List windows"},
				{"Prefix + ,", "Rename window"},
				{"Prefix + &", "Close window"},
				{"Prefix + l", "Last active window"},
				// Session management
				{"Prefix + s", "List sessions"},
				{"Prefix + $", "Rename session"},
				{"Prefix + d", "Detach session"},
				{"Prefix + (", "Previous session"},
				{"Prefix + )", "Next session"},
				// Copy mode & misc
				{"Prefix + [", "Copy mode"},
				{"Prefix + ]", "Paste buffer"},
				{"Prefix + r", "Reload config"},
				{"Prefix + ?", "List keybindings"},
			},
		},
		{
			ID:   "zsh",
			Name: zshTitle,
			Icon: "",
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
			Icon: "",
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
		{
			ID:   "git",
			Name: "Git",
			Icon: "",
			Items: []Item{
				{"git status", "Show working tree status"},
				{"git add .", "Stage all changes"},
				{"git commit -m", "Commit with message"},
				{"git push", "Push to remote"},
				{"git pull", "Pull from remote"},
				{"git log --oneline", "Compact commit history"},
				{"git diff", "Show unstaged changes"},
				{"git branch", "List branches"},
				{"git checkout -b", "Create and switch branch"},
				{"git stash", "Stash changes"},
			},
		},
		{
			ID:   "delta",
			Name: "Delta",
			Icon: "",
			Items: []Item{
				{"git diff", "Diff with delta styling"},
				{"git show", "Show commit with delta"},
				{"git log -p", "Log with patches"},
				{"delta --help", "Show delta options"},
			},
		},
		{
			ID:   "lazydocker",
			Name: "LazyDocker",
			Icon: "",
			Items: []Item{
				{"d", "Remove container"},
				{"s", "Stop container"},
				{"r", "Restart container"},
				{"a", "Attach to container"},
				{"l", "View logs"},
				{"[/]", "Prev/next panel"},
				{"enter", "Focus panel"},
				{"?", "Help"},
				{"q", "Quit"},
			},
		},
		{
			ID:   "glow",
			Name: "Glow",
			Icon: "󰈙",
			Items: []Item{
				{"glow README.md", "Render markdown file"},
				{"glow -p", "Use pager"},
				{"glow -s dark", "Dark style"},
				{"j/k", "Scroll up/down"},
				{"q", "Quit"},
			},
		},
		{
			ID:   "bat",
			Name: "bat",
			Icon: "󰭟",
			Items: []Item{
				{"bat file.txt", "View file with syntax highlighting"},
				{"bat -A", "Show non-printable characters"},
				{"bat -n", "Show line numbers only"},
				{"bat --diff", "Show git diff"},
				{"bat -l json", "Force language"},
			},
		},
		{
			ID:   "btop",
			Name: "btop",
			Icon: "󰄨",
			Items: []Item{
				{"h", "Toggle help"},
				{"Esc", "Close menu/go back"},
				{"m", "Toggle memory graph"},
				{"n", "Toggle network graph"},
				{"p", "Toggle process view"},
				{"f", "Filter processes"},
				{"k", "Kill process"},
				{"q", "Quit"},
			},
		},
		{
			ID:   "ripgrep",
			Name: "ripgrep",
			Icon: "󰈞",
			Items: []Item{
				{"rg pattern", "Search for pattern"},
				{"rg -i pattern", "Case insensitive"},
				{"rg -w word", "Match whole word"},
				{"rg -t py pattern", "Search Python files"},
				{"rg -g '*.js'", "Glob filter"},
				{"rg -C 3", "Show 3 lines context"},
				{"rg -l pattern", "List matching files only"},
			},
		},
		{
			ID:   "fd",
			Name: "fd",
			Icon: "󰱼",
			Items: []Item{
				{"fd pattern", "Find files matching pattern"},
				{"fd -e js", "Find by extension"},
				{"fd -t d", "Find directories only"},
				{"fd -H", "Include hidden files"},
				{"fd -x cmd", "Execute command on results"},
			},
		},
		{
			ID:   "claude",
			Name: "Claude Code",
			Icon: "󰚩",
			Items: []Item{
				{"claude", "Start Claude Code"},
				{"/help", "Show help"},
				{"/clear", "Clear conversation"},
				{"/compact", "Summarize context"},
				{"Ctrl-C", "Cancel current operation"},
				{"Esc Esc", "Exit Claude Code"},
			},
		},
		{
			ID:   "tailscale",
			Name: "Tailscale",
			Icon: "󰖂",
			Items: []Item{
				{"tailscale status", "Show connection status"},
				{"tailscale up", "Connect to tailnet"},
				{"tailscale down", "Disconnect from tailnet"},
				{"tailscale ip", "Show Tailscale IP addresses"},
				{"tailscale ssh <host>", "SSH to peer node"},
				{"tailscale ping <host>", "Ping peer node"},
				{"tailscale netcheck", "Network diagnostic"},
			},
		},
		{
			ID:   "sunshine",
			Name: "Sunshine",
			Icon: "☀",
			Items: []Item{
				{"sunshine", "Start streaming server"},
				{"sunshine --help", "Show command options"},
				{"localhost:47990", "Web UI (browser)"},
			},
		},
		{
			ID:   "moonlight",
			Name: "Moonlight",
			Icon: "🌙",
			Items: []Item{
				{"moonlight", "Launch client"},
				{"moonlight pair <host>", "Pair with host"},
				{"moonlight stream <host>", "Stream from host"},
				{"moonlight list <host>", "List available apps"},
			},
		},
	}
}
