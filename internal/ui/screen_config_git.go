package ui

import (
	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configGitScreen is the migrated ScreenHandler for the Git config screen.
// Navigation + back are inherited from configFieldNav. The legacy handler used
// left/right for option cycling (fields 1, 4) and space for toggles (fields 0,
// 2, 3); the shared adjust callback dispatches on the raw key to preserve that.
//
// Fields: 0=delta, 1=branch, 2=rebase, 3=sign, 4=credential.
type configGitScreen struct {
	configFieldNav
}

// NewConfigGitScreen creates a new Git config screen handler.
func NewConfigGitScreen(ctx *ScreenContext) *configGitScreen {
	s := &configGitScreen{}
	s.id = ScreenConfigGit
	s.maxField = func(*App) int { return 4 }
	s.adjust = gitAdjust
	s.SetContext(ctx)
	return s
}

func gitAdjust(a *App, key string, fwd bool) {
	cfg := a.deepDiveConfig
	switch key {
	case keyLeft, keyRight, "h", "l":
		switch a.configFieldIndex {
		case 1: // Default branch
			opts := []string{"main", "master", "develop"}
			cfg.GitDefaultBranch = cycleOption(opts, cfg.GitDefaultBranch, fwd)
		case 4: // Credential helper
			opts := []string{"cache", "store", "osxkeychain", "none"}
			cfg.GitCredentialHelper = cycleOption(opts, cfg.GitCredentialHelper, fwd)
		}
	case " ":
		switch a.configFieldIndex {
		case 0: // Delta side-by-side
			cfg.GitDeltaSideBySide = !cfg.GitDeltaSideBySide
		case 2: // Pull rebase
			cfg.GitPullRebase = !cfg.GitPullRebase
		case 3: // Sign commits
			cfg.GitSignCommits = !cfg.GitSignCommits
		}
	}
}

// Update delegates to the shared field-navigation handler.
func (s *configGitScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the Git configuration screen.
func (s *configGitScreen) View(width, height int) string {
	a := s.App()
	title := renderConfigTitle("", "Git", "Version control settings")

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(55))
	fieldIdx := 0

	rec.field(fieldIdx)
	deltaFocused := a.configFieldIndex == fieldIdx
	rec.write(renderFieldLabel("Delta Diff View", deltaFocused))
	rec.write(renderToggleLabeled(cfg.GitDeltaSideBySide, "Side-by-side", "Unified", deltaFocused))
	rec.write("\n\n")
	fieldIdx++

	rec.field(fieldIdx)
	branchFocused := a.configFieldIndex == fieldIdx
	rec.write(renderFieldLabel("Default Branch", branchFocused))
	rec.write(renderOptionSelector(
		[]string{"main", "master", "develop"},
		[]string{"main", "master", "develop"},
		cfg.GitDefaultBranch,
		branchFocused,
	))
	rec.write("\n\n")
	fieldIdx++

	rec.field(fieldIdx)
	rebaseFocused := a.configFieldIndex == fieldIdx
	rec.write(renderCheckbox("Pull with Rebase", cfg.GitPullRebase, rebaseFocused))
	rec.write("\n")
	fieldIdx++

	rec.field(fieldIdx)
	signFocused := a.configFieldIndex == fieldIdx
	rec.write(renderCheckbox("GPG Sign Commits", cfg.GitSignCommits, signFocused))
	rec.write("\n\n")
	fieldIdx++

	rec.field(fieldIdx)
	credFocused := a.configFieldIndex == fieldIdx
	rec.write(renderFieldLabel("Credential Helper", credFocused))
	rec.write(renderOptionSelector(
		[]string{"cache", "store", "osxkeychain", "none"},
		[]string{"Cache (temp)", "Store (file)", "macOS Keychain", "None"},
		cfg.GitCredentialHelper,
		credFocused,
	))
	rec.write("\n\n")

	// Trailing reference section (non-field content): excluded from hit extents.
	rec.write(sectionHeaderStyle.Render("Included Aliases"))
	rec.write("\n")
	aliases := []string{
		"git st → status",
		"git co → checkout",
		"git br → branch",
		"git ci → commit",
		"git lg → log --graph",
	}
	for _, alias := range aliases {
		rec.write(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  " + alias + "\n"))
	}

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := s.footer()
	a.configFieldLayout = rec.finalize(width, height, title, box, help)

	return PlaceWithBackground(
		width, height,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
