package ui

import (
	"strings"

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
	case "left", "right", "h", "l":
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
	var content strings.Builder
	fieldIdx := 0

	deltaFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Delta Diff View", deltaFocused))
	content.WriteString(renderToggleLabeled(cfg.GitDeltaSideBySide, "Side-by-side", "Unified", deltaFocused))
	content.WriteString("\n\n")
	fieldIdx++

	branchFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Default Branch", branchFocused))
	content.WriteString(renderOptionSelector(
		[]string{"main", "master", "develop"},
		[]string{"main", "master", "develop"},
		cfg.GitDefaultBranch,
		branchFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	rebaseFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("Pull with Rebase", cfg.GitPullRebase, rebaseFocused))
	content.WriteString("\n")
	fieldIdx++

	signFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("GPG Sign Commits", cfg.GitSignCommits, signFocused))
	content.WriteString("\n\n")
	fieldIdx++

	credFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Credential Helper", credFocused))
	content.WriteString(renderOptionSelector(
		[]string{"cache", "store", "osxkeychain", "none"},
		[]string{"Cache (temp)", "Store (file)", "macOS Keychain", "None"},
		cfg.GitCredentialHelper,
		credFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	content.WriteString(sectionHeaderStyle.Render("Included Aliases"))
	content.WriteString("\n")
	aliases := []string{
		"git st → status",
		"git co → checkout",
		"git br → branch",
		"git ci → commit",
		"git lg → log --graph",
	}
	for _, alias := range aliases {
		content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  " + alias + "\n"))
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(55)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ select • space toggle • esc back")

	return PlaceWithBackground(
		width, height,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
