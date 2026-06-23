package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// animationScreen is the migrated ScreenHandler for the intro animation (the
// matrix-rain "DOTFILES" splash shown before the wizard/main menu).
//
// State stays on App: the frame counter (animFrame), the window dimensions
// (width/height), the post-intro destination (postIntroScreen) and the
// animationDone flag are read/written through s.App() so the rest of the legacy
// wizard and the post-intro transition keep seeing the same values.
//
// On-enter work: Init() returns tickAnimation(), the command that previously
// lived in App.Init's `if a.screen == ScreenAnimation` branch. Driving it from
// the handler's Init keeps it from double-firing (App.Init no longer issues it).
//
// Async-in-handler: because the ScreenManager delegates every non-navigation
// message to this handler while it is active, the intro tick advance is handled
// here (not in App.Update's tickMsg case): each tickMsg advances animFrame and,
// at introAnimationFrames, transitions via a.postIntroTransition() (which routes
// through NavigateTo). animationDoneMsg is handled here too. The "no window size
// yet, don't advance" guard is preserved so the intro does not fast-forward on
// terminals that deliver WindowSizeMsg late.
type animationScreen struct {
	BaseScreen
}

// NewAnimationScreen creates a new intro animation screen handler.
func NewAnimationScreen(ctx *ScreenContext) *animationScreen {
	s := &animationScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *animationScreen) ID() Screen { return ScreenAnimation }

// Init kicks the intro frame tick on entry. This was previously issued from
// App.Init; moving it here keeps it from double-firing.
func (s *animationScreen) Init() tea.Cmd {
	return tickAnimation()
}

// Update handles the intro tick advance, the animation-done signal, and any key
// (which skips the intro).
func (s *animationScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == keyCtrlC {
			return s, tea.Quit
		}
		// Any key skips the animation. postIntroTransition routes through the
		// ScreenManager (NavigateTo) so migrated post-intro screens enter managed
		// mode; the manager will replace this handler as a result.
		return s, a.postIntroTransition()

	case tickMsg:
		// If we don't have a window size yet, don't advance frames. This prevents
		// the intro from "fast-forwarding" on terminals that deliver WindowSizeMsg
		// a little late.
		if a.width == 0 || a.height == 0 {
			return s, tickAnimation()
		}
		a.animFrame++
		// Animation runs for a short burst and then transitions to the wizard.
		if a.animFrame >= introAnimationFrames {
			return s, a.postIntroTransition()
		}
		return s, tickAnimation()

	case animationDoneMsg:
		return s, a.postIntroTransition()
	}
	return s, nil
}

// View renders the intro animation. It ports renderAnimation, reading the live
// App state (animFrame, width, height). The width/height args are accepted for
// interface conformance; the matrix layout reads a.width/a.height directly and
// falls back to loadingMessage until a WindowSizeMsg has been seen.
func (s *animationScreen) View(width, height int) string {
	a := s.App()
	if a.width == 0 || a.height == 0 {
		return loadingMessage
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
	drops := animationComputeDrops(contentW, rainH, a.animFrame)
	lines = append(lines, animationRenderRain(drops, contentW, rainH, a.animFrame)...)

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

	return PlaceWithBackground(a.width, a.height, card)
}

// animationHash32 is a tiny deterministic mixer (no RNG state, stable across
// frames) used to derive per-column/per-cell pseudo-random values.
func animationHash32(v uint32) uint32 {
	v ^= v >> 16
	v *= 0x7feb352d
	v ^= v >> 15
	v *= 0x846ca68b
	v ^= v >> 16
	return v
}

// animationDrop holds the precomputed parameters for one matrix-rain column.
type animationDrop struct {
	head   int
	length int
	color  int // 0 = green, 1 = cyan spark
}

// animationComputeDrops precomputes the per-column drop parameters for the
// matrix rain at the given frame.
func animationComputeDrops(contentW, rainH, animFrame int) []animationDrop {
	drops := make([]animationDrop, contentW)
	for x := 0; x < contentW; x++ {
		//nolint:gosec // G115: terminal-bounded small positive int, no overflow
		h := animationHash32(uint32(x*1337 + 42))
		speed := 1 + int(h%3) // 1..3
		length := 6 + int((h>>8)%10)
		gap := 8 + int((h>>16)%10)
		cycle := rainH + length + gap
		//nolint:gosec // G115: terminal-bounded small positive int, no overflow
		head := (animFrame*speed + int(h%uint32(cycle))) % cycle
		head -= length // allow entering from above

		color := 0
		if (h>>24)%11 == 0 {
			color = 1
		}

		drops[x] = animationDrop{head: head, length: length, color: color}
	}
	return drops
}

// animationRenderRain renders the matrix-rain lines for the given drops/frame.
func animationRenderRain(drops []animationDrop, contentW, rainH, animFrame int) []string {
	chars := []rune("01ABCDEFGHIJKLMNOPQRSTUVWXYZ@#$%&*")
	headStyle := lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
	midStyle := lipgloss.NewStyle().Foreground(ColorGreen)
	tailStyle := lipgloss.NewStyle().Foreground(ColorGreen).Faint(true)
	sparkStyle := lipgloss.NewStyle().Foreground(ColorNeonBlue).Bold(true)

	rows := make([]string, 0, rainH)
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
			//nolint:gosec // G115: terminal-bounded small positive int, no overflow
			sv := animationHash32(uint32(x*31 + y*97 + ((animFrame - dist) * 7)))
			ch := chars[int(sv)%len(chars)]

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
		rows = append(rows, line.String())
	}
	return rows
}
