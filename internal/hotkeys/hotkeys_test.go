package hotkeys

import (
	"testing"
)

// zshEmacsName is the display name of the emacs/Mac-style Zsh hotkey category.
const zshEmacsName = "Zsh (emacs/Mac-style)"

// findCategory returns the category with the given ID, or nil.
func findCategory(cats []Category, id string) *Category {
	for i := range cats {
		if cats[i].ID == id {
			return &cats[i]
		}
	}
	return nil
}

// keysFor returns the Keys of the item with the given description in a category.
func keysFor(t *testing.T, cats []Category, catID, desc string) string {
	t.Helper()
	c := findCategory(cats, catID)
	if c == nil {
		t.Fatalf("category %q not found", catID)
	}
	for _, it := range c.Items {
		if it.Description == desc {
			return it.Keys
		}
	}
	t.Fatalf("item %q not found in category %q", desc, catID)
	return ""
}

func TestNormalizeNavStyle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want NavStyle
	}{
		{"vim", NavVim},
		{"VIM", NavVim},
		{"Vim", NavVim},
		{"emacs", NavEmacs},
		{"EMACS", NavEmacs},
		{"", NavEmacs},
		{"junk", NavEmacs},
		{"vi", NavEmacs}, // only exact "vim" (case-insensitive) maps to vim
		{"vimmer", NavEmacs},
		{" vim", NavEmacs}, // no trimming; whitespace makes it non-matching
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := normalizeNavStyle(tc.in); got != tc.want {
				t.Fatalf("normalizeNavStyle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCategoriesStableSetAndCount(t *testing.T) {
	t.Parallel()

	// The category set must be identical regardless of nav style (only the
	// bindings inside flip; categories don't appear/disappear).
	vim := Categories("vim")
	emacs := Categories("emacs")

	if len(vim) != len(emacs) {
		t.Fatalf("category count differs by nav style: vim=%d emacs=%d", len(vim), len(emacs))
	}

	wantIDs := []string{
		"tmux", "zsh", "yazi", "fzf", "ghostty", "neovim", "lazygit", "eza",
		"zoxide", "dotfiles", "git", "delta", "lazydocker", "glow", "bat",
		"btop", "ripgrep", "fd", "claude", "tailscale", "sunshine", "moonlight",
	}
	if len(vim) != len(wantIDs) {
		t.Fatalf("category count = %d, want %d", len(vim), len(wantIDs))
	}
	for i, id := range wantIDs {
		if vim[i].ID != id {
			t.Fatalf("category[%d].ID = %q, want %q", i, vim[i].ID, id)
		}
	}

	// IDs must be unique.
	seen := map[string]bool{}
	for _, c := range vim {
		if seen[c.ID] {
			t.Fatalf("duplicate category ID %q", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestCategoriesNavStyleBindingsFlip(t *testing.T) {
	t.Parallel()

	vim := Categories("vim")
	emacs := Categories("emacs")

	// Each entry: category, item description, expected vim Keys, expected emacs Keys.
	cases := []struct {
		cat       string
		desc      string
		vimKeys   string
		emacsKeys string
	}{
		{"tmux", "Navigate panes", "Alt-h/j/k/l", "Alt-Arrow"},
		{"yazi", "Navigate", "h/j/k/l", "Arrow keys"},
		{"yazi", "Toggle hidden files", ".", "Ctrl-h"},
		{"neovim", "Navigate", "h/j/k/l", "Arrow keys"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.cat+"/"+tc.desc, func(t *testing.T) {
			t.Parallel()
			gotVim := keysFor(t, vim, tc.cat, tc.desc)
			gotEmacs := keysFor(t, emacs, tc.cat, tc.desc)
			if gotVim != tc.vimKeys {
				t.Fatalf("vim %s/%s Keys = %q, want %q", tc.cat, tc.desc, gotVim, tc.vimKeys)
			}
			if gotEmacs != tc.emacsKeys {
				t.Fatalf("emacs %s/%s Keys = %q, want %q", tc.cat, tc.desc, gotEmacs, tc.emacsKeys)
			}
			if gotVim == gotEmacs {
				t.Fatalf("%s/%s did not flip between styles (both %q)", tc.cat, tc.desc, gotVim)
			}
		})
	}
}

func TestZshTitleFlipsByNavStyle(t *testing.T) {
	t.Parallel()

	vimZsh := findCategory(Categories("vim"), "zsh")
	emacsZsh := findCategory(Categories("emacs"), "zsh")
	if vimZsh == nil || emacsZsh == nil {
		t.Fatalf("zsh category missing")
	}
	if vimZsh.Name != "Zsh (vim mode)" {
		t.Fatalf("vim zsh Name = %q, want %q", vimZsh.Name, "Zsh (vim mode)")
	}
	if emacsZsh.Name != zshEmacsName {
		t.Fatalf("emacs zsh Name = %q, want %q", emacsZsh.Name, zshEmacsName)
	}
}

// TestUnknownNavStyleBehavesAsEmacs ensures the documented fallback: any value
// other than "vim" yields emacs bindings.
func TestUnknownNavStyleBehavesAsEmacs(t *testing.T) {
	t.Parallel()

	for _, ns := range []string{"", "EMACS", "garbage", "vi", "zsh"} {
		cats := Categories(ns)
		if got := keysFor(t, cats, "tmux", "Navigate panes"); got != "Alt-Arrow" {
			t.Fatalf("nav %q: tmux nav = %q, want emacs default %q", ns, got, "Alt-Arrow")
		}
		zsh := findCategory(cats, "zsh")
		if zsh.Name != zshEmacsName {
			t.Fatalf("nav %q: zsh title = %q, want emacs default", ns, zsh.Name)
		}
	}
}

// TestCategoriesWellFormed asserts every category has a non-empty ID and Name
// and at least one item, and every item has non-empty Keys and Description.
func TestCategoriesWellFormed(t *testing.T) {
	t.Parallel()

	for _, ns := range []string{"vim", "emacs"} {
		ns := ns
		t.Run(ns, func(t *testing.T) {
			t.Parallel()
			cats := Categories(ns)
			if len(cats) == 0 {
				t.Fatalf("no categories returned for nav %q", ns)
			}
			for ci, c := range cats {
				if c.ID == "" {
					t.Fatalf("category[%d] has empty ID", ci)
				}
				if c.Name == "" {
					t.Fatalf("category %q has empty Name", c.ID)
				}
				if len(c.Items) == 0 {
					t.Fatalf("category %q has no items", c.ID)
				}
				for ii, it := range c.Items {
					if it.Keys == "" {
						t.Fatalf("category %q item[%d] has empty Keys", c.ID, ii)
					}
					if it.Description == "" {
						t.Fatalf("category %q item[%d] (%q) has empty Description", c.ID, ii, it.Keys)
					}
				}
			}
		})
	}
}
