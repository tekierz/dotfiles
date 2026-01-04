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

	// Compute a stable "card" size that fits on the screen.
	// We keep the content area fixed-size throughout the animation to avoid
	// center-jitter as elements appear.
	outerW := min(a.width-2, 90)
	outerH := min(a.height-2, 22)
	if outerW < 20 {
		outerW = maxInt(0, a.width-2)
	}
	if outerH < 10 {
		outerH = maxInt(0, a.height-2)
	}

	// Border(2) + horizontal padding(4) = 6 columns of overhead.
	// Border(2) + vertical padding(2)   = 4 rows of overhead.
	contentW := maxInt(10, outerW-6)
	contentH := maxInt(6, outerH-4)

	// Progress (0..1), clamped.
	progress := float64(a.animFrame) / float64(introAnimationFrames)
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	// Layout inside the card:
	// - rainH lines of matrix noise
	// - 1 blank line
	// - 1 logo/banner line
	// - 1 progress line
	// - 1 hint line
	const reservedLines = 4
	rainH := maxInt(1, contentH-reservedLines)

	// Build animation frame line-by-line for consistent widths.
	lines := make([]string, 0, contentH)

	// Rain (matrix-style), but explicitly vertical and readable (not "glitch noise").
	chars := []rune("01ABCDEFGHIJKLMNOPQRSTUVWXYZ@#$%&*")
	headStyle := lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
	midStyle := lipgloss.NewStyle().Foreground(ColorGreen)
	tailStyle := lipgloss.NewStyle().Foreground(ColorGreen).Faint(true)
	sparkStyle := lipgloss.NewStyle().Foreground(ColorNeonBlue).Bold(true)

	hash32 := func(v uint32) uint32 {
		// Tiny deterministic mixer (no RNG state, stable across frames).
		v ^= v >> 16
		v *= 0x7feb352d
		v ^= v >> 15
		v *= 0x846ca68b
		v ^= v >> 16
		return v
	}

	// Precompute per-column drop parameters.
	type drop struct {
		head   int
		length int
		color  int // 0 = green, 1 = cyan spark
	}
	drops := make([]drop, contentW)
	for x := 0; x < contentW; x++ {
		h := hash32(uint32(x*1337 + 42))
		speed := 1 + int(h%3) // 1..3
		length := 6 + int((h>>8)%10)
		gap := 8 + int((h>>16)%10)
		cycle := rainH + length + gap
		head := (a.animFrame*speed + int(h%uint32(cycle))) % cycle
		head -= length // allow entering from above

		color := 0
		if (h>>24)%11 == 0 {
			color = 1
		}

		drops[x] = drop{head: head, length: length, color: color}
	}

	for y := 0; y < rainH; y++ {
		var line strings.Builder
		for x := 0; x < contentW; x++ {
			d := drops[x]
			if y > d.head || d.head < 0 {
				line.WriteByte(' ')
				continue
			}

			dist := d.head - y // 0 at head, increases upward
			if dist < 0 || dist >= d.length {
				line.WriteByte(' ')
				continue
			}

			// Pick a stable-ish character for this cell.
			s := hash32(uint32(x*31 + y*97 + ((a.animFrame - dist) * 7)))
			ch := chars[int(s)%len(chars)]

			// Choose style by distance down the trail.
			style := tailStyle
			if dist == 0 {
				if d.color == 1 {
					style = sparkStyle
				} else {
					style = headStyle
				}
			} else if dist < d.length/3 {
				style = midStyle
			}

			line.WriteString(style.Render(string(ch)))
		}
		lines = append(lines, line.String())
	}

	// Blank spacer line (kept always to avoid layout jitter).
	lines = append(lines, "")

	// Banner line: reveal "DOTFILES" smoothly, but keep constant width.
	bannerText := "DOTFILES"
	reveal := int(progress * float64(len([]rune(bannerText))+4))
	if reveal < 0 {
		reveal = 0
	}
	if reveal > len([]rune(bannerText)) {
		reveal = len([]rune(bannerText))
	}

	var banner strings.Builder
	runes := []rune(bannerText)
	for i, r := range runes {
		if i <= reveal {
			banner.WriteRune(r)
		} else {
			banner.WriteRune(' ')
		}
	}

	bannerLine := lipgloss.Place(
		contentW, 1,
		lipgloss.Center, lipgloss.Center,
		GradientText("░▒▓█ ", GradientCyber)+
			lipgloss.NewStyle().Bold(true).Render(GradientText(banner.String(), GradientCyber))+
			GradientText(" █▓▒░", []lipgloss.Color{"#bf00ff", "#0044ff", "#006eff", "#0099ff", "#00c3ff", "#00e1ff", "#00ffff"}),
	)
	lines = append(lines, bannerLine)

	// Progress line (animated).
	barW := min(40, maxInt(10, contentW-18))
	bar := ProgressBarAnimated(progress, barW, a.animFrame)
	pct := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(fmt.Sprintf("%3d%%", int(progress*100)))
	progressLine := lipgloss.Place(
		contentW, 1,
		lipgloss.Center, lipgloss.Center,
		AnimatedSpinnerDots(a.animFrame)+" "+bar+" "+pct,
	)
	lines = append(lines, progressLine)

	// Hint line.
	hint := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("[Press any key to skip]")
	hintLine := lipgloss.Place(contentW, 1, lipgloss.Center, lipgloss.Center, hint)
	lines = append(lines, hintLine)

	// Ensure we always render exactly contentH lines (stability).
	for len(lines) < contentH {
		lines = append(lines, "")
	}
	if len(lines) > contentH {
		lines = lines[:contentH]
	}

	content := strings.Join(lines, "\n")

	borderColor := GradientCyber[(a.animFrame/2)%len(GradientCyber)]
	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		card,
	)
}

// renderWelcome renders the welcome/main menu screen
func (a *App) renderWelcome() string {
	// ASCII Logo with gradient
	// Keep the logo from overflowing on smaller terminals.
	logoMaxW := maxInt(20, a.width-6)
	logo := lipgloss.NewStyle().MaxWidth(logoMaxW).Render(ASCIILogo())

	// Decorative border
	borderW := min(60, maxInt(24, a.width-8))
	topBorder := CyberBorder(borderW)

	// Status indicators with neon styling
	statusContent := fmt.Sprintf(
		"  %s %s  %s %s  %s %s",
		StatusDot("success"),
		GradientText("SYSTEM READY", GradientCyber),
		StatusDot("success"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("13 THEMES"),
		StatusDot("success"),
		lipgloss.NewStyle().Foreground(ColorMagenta).Render("ALL TOOLS"),
	)

	// Tools list with icons
	tools := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		"zsh • tmux • neovim • yazi • ghostty • fzf • zoxide • bat • delta")

	// Description with gradient accent
	desc := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(ColorText).Render("Your terminal environment, configured in minutes."),
		lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true).Render("Cross-platform · Fully reversible · Open source"),
	)

	// Buttons with better styling
	var quickSetup, deepDive string

	if !a.deepDive {
		quickSetup = lipgloss.NewStyle().
			Padding(1, 3).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorCyan).
			Foreground(ColorCyan).
			Bold(true).
			Render("▶ QUICK SETUP\n  Recommended")
		deepDive = lipgloss.NewStyle().
			Padding(1, 3).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Foreground(ColorTextMuted).
			Render("  DEEP DIVE\n  Customize all")
	} else {
		quickSetup = lipgloss.NewStyle().
			Padding(1, 3).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Foreground(ColorTextMuted).
			Render("  QUICK SETUP\n  Recommended")
		deepDive = lipgloss.NewStyle().
			Padding(1, 3).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorMagenta).
			Foreground(ColorMagenta).
			Bold(true).
			Render("▶ DEEP DIVE\n  Customize all")
	}

	// Stack buttons vertically on narrower terminals.
	var buttons string
	if a.width < 78 {
		buttons = lipgloss.JoinVertical(lipgloss.Center, quickSetup, "", deepDive)
	} else {
		buttons = lipgloss.JoinHorizontal(lipgloss.Top, quickSetup, "  ", deepDive)
	}

	// Help text with gradient
	help := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		"←→ select • enter continue • q quit")

	// Bottom border
	bottomBorder := CyberBorder(borderW)

	// Compose the screen
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		topBorder,
		logo,
		truncateVisible(statusContent, borderW),
		truncateVisible(tools, borderW),
		"",
		desc,
		"",
		buttons,
		"",
		help,
		bottomBorder,
	)

	// Center on screen with styled container
	container := lipgloss.NewStyle().
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		container,
	)
}

// renderThemePicker renders the theme selection screen
func (a *App) renderThemePicker() string {
	title := TitleStyle.Render("Select Theme")

	// Layout adapts based on terminal width:
	// - wide: list + preview side-by-side
	// - narrow: list only (preview below would overflow easily)
	showPreview := a.width >= 86

	previewOuterW := min(36, maxInt(24, a.width/3))
	if !showPreview {
		previewOuterW = 0
	}

	listW := maxInt(24, a.width-10)
	if showPreview {
		listW = maxInt(24, a.width-previewOuterW-12)
	}

	var themeList strings.Builder
	for i, t := range themes {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if i == a.themeIndex {
			prefix = "▶ "
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(t.color)).Bold(true)
		}
		line := style.Render(fmt.Sprintf("%s%-20s %s", prefix, t.name, t.desc))
		themeList.WriteString(truncateVisible(line, listW))
		themeList.WriteByte('\n')
	}

	content := themeList.String()

	if showPreview {
		// Preview box with color swatches
		selectedTheme := themes[a.themeIndex]
		previewColor := lipgloss.Color(selectedTheme.color)

		preview := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(previewColor).
			Width(maxInt(1, previewOuterW-2)). // border adds 2
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

		content = lipgloss.JoinHorizontal(lipgloss.Top, content, "  ", preview)
	}

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
	if a.width < 78 {
		content = lipgloss.JoinVertical(lipgloss.Center, emacsBox, "", vimBox)
	}

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

	// Prevent the tree from overflowing narrow terminals.
	treeMaxW := maxInt(20, a.width-6)
	tree = lipgloss.NewStyle().MaxWidth(treeMaxW).Render(tree)

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
	progressW := min(50, maxInt(20, a.width-30))
	progress := ProgressBar(progressPercent, progressW)

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
		// Keep the output panel responsive so it doesn't overflow smaller terminals.
		// Note: Width/Height apply before borders in lipgloss, so subtract 2 to
		// target an approximate outer size.
		Width(maxInt(20, min(72, a.width-10)-2)).
		Height(clampInt(a.height/4, 6, 10)).
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
	summary = lipgloss.NewStyle().MaxWidth(maxInt(20, a.width-6)).Render(summary)

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
		MaxWidth(maxInt(20, a.width-10)).
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
