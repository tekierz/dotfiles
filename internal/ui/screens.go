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

	var themeList strings.Builder
	for i, t := range themes {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if i == a.themeIndex {
			prefix = "▶ "
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(t.color)).Bold(true)
		}
		themeList.WriteString(style.Render(fmt.Sprintf("%s%-20s %s", prefix, t.name, t.desc)))
		themeList.WriteString("\n")
	}

	// Preview box with color swatches
	selectedTheme := themes[a.themeIndex]
	previewColor := lipgloss.Color(selectedTheme.color)

	preview := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(previewColor).
		Width(32).
		Padding(1).
		Render(lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.NewStyle().Foreground(previewColor).Bold(true).Render(selectedTheme.name),
			"",
			lipgloss.NewStyle().Foreground(previewColor).Render("████████████████████"),
			"",
			lipgloss.NewStyle().Foreground(ColorText).Render("  normal text"),
			lipgloss.NewStyle().Foreground(previewColor).Render("  highlighted text"),
			lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  muted text"),
			"",
			lipgloss.NewStyle().Foreground(ColorGreen).Render("  ✓ success"),
			lipgloss.NewStyle().Foreground(ColorRed).Render("  ✗ error"),
		))

	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		themeList.String(),
		"  ",
		preview,
	)

	help := HelpStyle.Render("[↑↓/jk] Navigate    [ENTER] Select    [ESC] Back")

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
	if a.installComplete {
		title = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).Render("✓ Installation Complete!")
	}

	steps := []struct {
		name string
	}{
		{"Initializing backup"},
		{"Installing packages"},
		{"Configuring zsh"},
		{"Configuring tmux"},
		{"Configuring ghostty"},
		{"Configuring yazi"},
		{"Configuring git"},
		{"Setting up utilities"},
		{"Setting up neovim"},
		{"Finalizing"},
	}

	var stepList strings.Builder
	for i, s := range steps {
		var status string
		var style lipgloss.Style

		if i < a.installStep {
			status = "✓"
			style = lipgloss.NewStyle().Foreground(ColorGreen)
		} else if i == a.installStep && a.installRunning {
			status = "▶"
			style = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
		} else {
			status = "○"
			style = lipgloss.NewStyle().Foreground(ColorTextMuted)
		}
		stepList.WriteString(style.Render(fmt.Sprintf("  %s %s\n", status, s.name)))
	}

	// Calculate progress
	progressPercent := float64(a.installStep) / float64(len(steps))
	if a.installComplete {
		progressPercent = 1.0
	}
	progress := ProgressBar(progressPercent, 40)

	// Output panel - show real output
	var outputLines string
	if len(a.installOutput) > 0 {
		// Show last 6 lines
		start := 0
		if len(a.installOutput) > 6 {
			start = len(a.installOutput) - 6
		}
		outputLines = strings.Join(a.installOutput[start:], "\n")
	} else if a.installRunning {
		outputLines = lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Starting installation...")
	} else if !a.installComplete {
		outputLines = lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Press ENTER to start")
	}

	output := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Width(60).
		Height(8).
		Padding(0, 1).
		Render(outputLines)

	var help string
	if a.installComplete {
		help = HelpStyle.Render("[ENTER] Continue")
	} else if a.installRunning {
		help = HelpStyle.Render("Installation in progress...")
	} else {
		help = HelpStyle.Render("[ENTER] Start    [ESC] Back")
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		stepList.String(),
		"",
		progress,
		"",
		output,
		"",
		help,
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
