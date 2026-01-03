package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderAnimation renders the intro animation screen
func (a *App) renderAnimation() string {
	if a.width == 0 || a.height == 0 {
		return "Loading..."
	}

	// Use fallback animation
	anim := NewFallbackAnimation(a.width, a.height)
	for i := 0; i < a.animFrame; i++ {
		anim.NextFrame()
	}

	frame := anim.NextFrame()

	// Style the frame with cyberpunk colors
	styled := lipgloss.NewStyle().
		Foreground(ColorCyan).
		Render(frame)

	// Add skip hint at the bottom
	hint := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("\n\n  [Press any key to skip]")

	return styled + hint
}

// renderWelcome renders the welcome/main menu screen
func (a *App) renderWelcome() string {
	// Logo with glitch effect
	logo := GlitchText("D O T F I L E S")

	// Status box
	statusBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1).
		Render(fmt.Sprintf(
			"  %s SYSTEM: %s\n  %s THEMES: %s\n  %s TOOLS:  %s",
			lipgloss.NewStyle().Foreground(ColorCyan).Render(">"),
			StatusReadyStyle.Render("Ready"),
			lipgloss.NewStyle().Foreground(ColorCyan).Render(">"),
			lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("13 loaded"),
			lipgloss.NewStyle().Foreground(ColorCyan).Render(">"),
			lipgloss.NewStyle().Foreground(ColorTextMuted).Render("zsh • tmux • nvim • yazi • ghostty"),
		))

	// Description
	desc := lipgloss.NewStyle().
		Foreground(ColorText).
		Render("Your terminal environment, configured in minutes.\nCross-platform. Fully reversible. Open source.")

	// Buttons
	quickSetupStyle := ButtonStyle
	deepDiveStyle := ButtonStyle

	if !a.deepDive {
		quickSetupStyle = ButtonActiveStyle
	} else {
		deepDiveStyle = ButtonActiveStyle
	}

	quickSetup := quickSetupStyle.Render(fmt.Sprintf(" %s QUICK SETUP\n   Recommended ", func() string {
		if !a.deepDive {
			return "▶"
		}
		return " "
	}()))

	deepDive := deepDiveStyle.Render(fmt.Sprintf(" %s DEEP DIVE\n   Customize all ", func() string {
		if a.deepDive {
			return "▶"
		}
		return " "
	}()))

	buttons := lipgloss.JoinHorizontal(lipgloss.Top, quickSetup, "  ", deepDive)

	// Help text
	help := HelpStyle.Render("[←→] Select    [ENTER] Continue    [Q] Quit")

	// Compose the screen
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		"",
		logo,
		"",
		statusBox,
		"",
		desc,
		"",
		buttons,
		"",
		help,
	)

	// Center on screen
	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(content),
	)
}

// renderThemePicker renders the theme selection screen
func (a *App) renderThemePicker() string {
	title := TitleStyle.Render("Select Theme")

	themes := []struct {
		name  string
		desc  string
		color lipgloss.Color
	}{
		{"catppuccin-mocha", "Dark, warm pastels", lipgloss.Color("#89b4fa")},
		{"catppuccin-latte", "Light, warm pastels", lipgloss.Color("#1e66f5")},
		{"dracula", "Dark with vibrant purples", lipgloss.Color("#bd93f9")},
		{"gruvbox-dark", "Retro warm browns", lipgloss.Color("#83a598")},
		{"gruvbox-light", "Warm paper-like tones", lipgloss.Color("#076678")},
		{"nord", "Arctic cool blues", lipgloss.Color("#88c0d0")},
		{"tokyo-night", "Rich purples and blues", lipgloss.Color("#7aa2f7")},
		{"solarized-dark", "Low contrast dark", lipgloss.Color("#268bd2")},
		{"solarized-light", "Low contrast light", lipgloss.Color("#268bd2")},
		{"monokai", "Classic vibrant", lipgloss.Color("#66d9ef")},
		{"rose-pine", "Soft muted pinks", lipgloss.Color("#c4a7e7")},
	}

	var themeList strings.Builder
	for _, t := range themes {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if t.name == a.theme {
			prefix = "▶ "
			style = lipgloss.NewStyle().Foreground(t.color).Bold(true)
		}
		themeList.WriteString(style.Render(fmt.Sprintf("%s%-20s %s", prefix, t.name, t.desc)))
		themeList.WriteString("\n")
	}

	// Preview box (placeholder)
	preview := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Width(30).
		Height(8).
		Padding(1).
		Render(fmt.Sprintf("Preview: %s\n\n(Live preview coming soon)", a.theme))

	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		themeList.String(),
		"  ",
		preview,
	)

	help := HelpStyle.Render("[↑↓] Navigate    [ENTER] Select    [ESC] Back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			"",
			content,
			"",
			help,
		)),
	)
}

// renderNavPicker renders the navigation style selection screen
func (a *App) renderNavPicker() string {
	title := TitleStyle.Render("Select Navigation Style")

	emacsStyle := ButtonStyle
	vimStyle := ButtonStyle

	if a.navStyle == "emacs" {
		emacsStyle = ButtonActiveStyle
	} else {
		vimStyle = ButtonActiveStyle
	}

	emacsBox := emacsStyle.Render(lipgloss.JoinVertical(
		lipgloss.Left,
		" EMACS / MAC STYLE ",
		"",
		" Arrow keys for navigation",
		" Ctrl-A/E for line start/end",
		" Ctrl-W to delete word",
		"",
		" Recommended for beginners",
	))

	vimBox := vimStyle.Render(lipgloss.JoinVertical(
		lipgloss.Left,
		" VIM STYLE ",
		"",
		" hjkl for navigation",
		" Modal editing (Esc/i)",
		" Efficient for experts",
		"",
		" Power user choice",
	))

	content := lipgloss.JoinHorizontal(lipgloss.Top, emacsBox, "  ", vimBox)

	help := HelpStyle.Render("[←→] Select    [ENTER] Continue    [ESC] Back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Center,
			title,
			"",
			content,
			"",
			help,
		)),
	)
}

// renderFileTree renders the files that will be modified
func (a *App) renderFileTree() string {
	title := TitleStyle.Render("Files to be Modified")

	// File tree with color coding
	tree := lipgloss.NewStyle().Foreground(ColorText).Render(`
  ~/.config/
  ├── `) + lipgloss.NewStyle().Foreground(ColorGreen).Render("dotfiles/") + lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" (new)") + `
  │   ├── settings
  │   └── backups/
  ├── ` + lipgloss.NewStyle().Foreground(ColorGreen).Render("ghostty/") + `
  │   ├── config
  │   └── themes/dotfiles-theme
  ├── ` + lipgloss.NewStyle().Foreground(ColorGreen).Render("yazi/") + `
  │   ├── yazi.toml
  │   ├── keymap.toml
  │   └── theme.toml
  ├── ` + lipgloss.NewStyle().Foreground(ColorGreen).Render("bat/") + `
  │   └── config
  └── ` + lipgloss.NewStyle().Foreground(ColorGreen).Render("nvim/") + lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" (Kickstart.nvim)") + `

  ~/
  ├── ` + lipgloss.NewStyle().Foreground(ColorYellow).Render(".zshrc") + lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" (backed up)") + `
  ├── ` + lipgloss.NewStyle().Foreground(ColorYellow).Render(".tmux.conf") + lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" (backed up)") + `
  ├── ` + lipgloss.NewStyle().Foreground(ColorYellow).Render(".gitconfig") + lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" (backed up)") + `
  └── .local/bin/
      ├── ` + lipgloss.NewStyle().Foreground(ColorGreen).Render("hk") + `
      ├── ` + lipgloss.NewStyle().Foreground(ColorGreen).Render("caff") + `
      └── ` + lipgloss.NewStyle().Foreground(ColorGreen).Render("dotfiles") + `
`

	legend := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		fmt.Sprintf("  %s New    %s Modified (backed up)",
			lipgloss.NewStyle().Foreground(ColorGreen).Render("●"),
			lipgloss.NewStyle().Foreground(ColorYellow).Render("●"),
		))

	help := HelpStyle.Render("[ENTER] Start Installation    [ESC] Back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			tree,
			legend,
			"",
			help,
		)),
	)
}

// renderProgress renders the installation progress screen
func (a *App) renderProgress() string {
	title := TitleStyle.Render("Installing...")

	steps := []struct {
		name   string
		status string
	}{
		{"Initializing backup", "✓"},
		{"Installing packages", "▶"},
		{"Configuring zsh", "○"},
		{"Configuring tmux", "○"},
		{"Configuring ghostty", "○"},
		{"Configuring yazi", "○"},
		{"Configuring git", "○"},
		{"Setting up utilities", "○"},
		{"Setting up neovim", "○"},
		{"Finalizing", "○"},
	}

	var stepList strings.Builder
	for _, s := range steps {
		var style lipgloss.Style
		switch s.status {
		case "✓":
			style = lipgloss.NewStyle().Foreground(ColorGreen)
		case "▶":
			style = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
		default:
			style = lipgloss.NewStyle().Foreground(ColorTextMuted)
		}
		stepList.WriteString(style.Render(fmt.Sprintf("  %s %s\n", s.status, s.name)))
	}

	progress := ProgressBar(0.2, 40)

	// Output panel (placeholder)
	output := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Width(50).
		Height(6).
		Padding(0, 1).
		Render("$ brew install zsh zsh-syntax-highlighting...\n" +
			lipgloss.NewStyle().Foreground(ColorGreen).Render("==> Downloading zsh-5.9.tar.xz"))

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		stepList.String(),
		"",
		progress,
		"",
		output,
	)

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(content),
	)
}

// renderSummary renders the post-installation summary
func (a *App) renderSummary() string {
	title := lipgloss.NewStyle().
		Foreground(ColorGreen).
		Bold(true).
		Render("✓ Installation Complete!")

	summary := lipgloss.NewStyle().Foreground(ColorText).Render(fmt.Sprintf(`
  Theme:      %s
  Navigation: %s
  Backup:     ~/.config/dotfiles/backups/

  Next steps:

  1. %s or restart terminal
  2. %s to start tmux
  3. %s to finish plugin installation
  4. %s to customize prompt
  5. %s to see hotkey reference
`,
		lipgloss.NewStyle().Foreground(ColorCyan).Render(a.theme),
		lipgloss.NewStyle().Foreground(ColorCyan).Render(a.navStyle),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("source ~/.zshrc"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("tmux"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("nvim"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("p10k configure"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("hk"),
	))

	help := HelpStyle.Render("[ENTER] Exit")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Center,
			title,
			summary,
			help,
		)),
	)
}

// renderError renders the error recovery screen
func (a *App) renderError() string {
	title := lipgloss.NewStyle().
		Foreground(ColorRed).
		Bold(true).
		Render("✗ Error Occurred")

	errMsg := "Unknown error"
	if a.lastError != nil {
		errMsg = a.lastError.Error()
	}

	errorBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorRed).
		Padding(1).
		Render(errMsg)

	options := lipgloss.JoinHorizontal(
		lipgloss.Top,
		ButtonStyle.Render(" [R] Retry "),
		"  ",
		ButtonStyle.Render(" [S] Skip "),
		"  ",
		ButtonStyle.Render(" [Q] Quit "),
	)

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Center,
			title,
			"",
			errorBox,
			"",
			options,
		)),
	)
}
