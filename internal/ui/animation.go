package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DurFrame represents a single frame in a .dur animation
type DurFrame struct {
	Delay   float64    `json:"delay"`
	Content [][]string `json:"content"` // 2D array of characters
	Colors  [][]int    `json:"colors"`  // 2D array of color indices
}

// DurAnimation represents a complete .dur animation file
type DurAnimation struct {
	Version    string     `json:"version"`
	Width      int        `json:"width"`
	Height     int        `json:"height"`
	FrameRate  float64    `json:"framerate"`
	Frames     []DurFrame `json:"frames"`
	ColorTable []string   `json:"colortable"`
}

// DetectDurdraw checks if durdraw is available on the system
func DetectDurdraw() bool {
	_, err := exec.LookPath("durdraw")
	return err == nil
}

// PlayDurAnimation plays a .dur animation file using durdraw
func PlayDurAnimation(path string) error {
	if !DetectDurdraw() {
		return fmt.Errorf("durdraw not found")
	}

	cmd := exec.Command("durdraw", "-p", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// DetectDurfetch checks if durfetch is available
func DetectDurfetch() bool {
	_, err := exec.LookPath("durfetch")
	return err == nil
}

// PlayDurfetchAnimation plays a built-in durfetch animation
// Available animations: linux-fire, cm-eye, bsd, linux-tux, unixbox
func PlayDurfetchAnimation(animName string) error {
	if !DetectDurfetch() {
		return fmt.Errorf("durfetch not found")
	}

	cmd := exec.Command("durfetch", "-l", animName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// BuiltInAnimations returns the list of available durfetch animations
func BuiltInAnimations() []string {
	return []string{
		"linux-fire",
		"cm-eye",
		"bsd",
		"linux-tux",
		"unixbox",
	}
}

// LoadDurAnimation loads a .dur animation file
func LoadDurAnimation(path string) (*DurAnimation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var anim DurAnimation
	if err := json.Unmarshal(data, &anim); err != nil {
		return nil, err
	}

	return &anim, nil
}

// GetAnimationPath returns the path to the intro animation
func GetAnimationPath() string {
	// Check multiple locations
	paths := []string{
		"assets/intro.dur",
		filepath.Join(os.Getenv("HOME"), ".config/dotfiles/intro.dur"),
		"/usr/local/share/dotfiles/intro.dur",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// FallbackAnimation is a pure-Go ASCII animation for when durdraw isn't available
type FallbackAnimation struct {
	frames     []string
	frameIndex int
	width      int
	height     int
}

// NewFallbackAnimation creates a fallback ASCII animation
func NewFallbackAnimation(width, height int) *FallbackAnimation {
	return &FallbackAnimation{
		frames: generateFallbackFrames(width, height),
		width:  width,
		height: height,
	}
}

// NextFrame returns the next frame of the animation
func (a *FallbackAnimation) NextFrame() string {
	if len(a.frames) == 0 {
		return ""
	}
	frame := a.frames[a.frameIndex]
	a.frameIndex = (a.frameIndex + 1) % len(a.frames)
	return frame
}

// Done returns true if the animation has completed one cycle
func (a *FallbackAnimation) Done() bool {
	return a.frameIndex == 0 && len(a.frames) > 0
}

func generateFallbackFrames(width, height int) []string {
	frames := make([]string, 30)

	// Generate a matrix-style rain effect
	for f := 0; f < 30; f++ {
		var sb strings.Builder

		// Calculate how much of the animation is complete
		progress := float64(f) / 30.0

		for y := 0; y < min(height-4, 20); y++ {
			for x := 0; x < min(width-4, 60); x++ {
				// Matrix rain effect
				if (x+y+f)%7 == 0 {
					chars := "01アイウエオカキクケコサシスセソ"
					sb.WriteRune([]rune(chars)[(x*y+f)%len([]rune(chars))])
				} else if (x+y+f)%13 == 0 {
					sb.WriteString("░")
				} else {
					sb.WriteString(" ")
				}
			}
			sb.WriteString("\n")
		}

		// Add the logo when animation is > 50% complete
		if progress > 0.5 {
			logo := generateLogo(progress)
			sb.WriteString("\n")
			sb.WriteString(logo)
		}

		frames[f] = sb.String()
	}

	return frames
}

func generateLogo(progress float64) string {
	// Typing effect for the logo text
	fullText := "░▒▓█ D O T F I L E S █▓▒░"
	visibleChars := int(float64(len(fullText)) * (progress - 0.5) * 2)
	if visibleChars > len(fullText) {
		visibleChars = len(fullText)
	}
	if visibleChars < 0 {
		visibleChars = 0
	}

	return fullText[:visibleChars]
}

// RainDrop represents a single drop in the matrix rain
type RainDrop struct {
	X     int
	Y     int
	Speed int
	Char  rune
}

// MatrixRain generates an animated matrix rain effect
type MatrixRain struct {
	drops  []RainDrop
	width  int
	height int
	chars  []rune
}

// NewMatrixRain creates a new matrix rain animation
func NewMatrixRain(width, height int) *MatrixRain {
	chars := []rune("01アイウエオカキクケコサシスセソタチツテトナニヌネノハヒフヘホマミムメモヤユヨラリルレロワヲン")

	drops := make([]RainDrop, width/2)
	for i := range drops {
		drops[i] = RainDrop{
			X:     i * 2,
			Y:     -i % height,
			Speed: 1 + (i % 3),
			Char:  chars[i%len(chars)],
		}
	}

	return &MatrixRain{
		drops:  drops,
		width:  width,
		height: height,
		chars:  chars,
	}
}

// Frame returns the current frame and advances the animation
func (m *MatrixRain) Frame() string {
	grid := make([][]rune, m.height)
	for i := range grid {
		grid[i] = make([]rune, m.width)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	// Place drops
	for i := range m.drops {
		d := &m.drops[i]
		if d.Y >= 0 && d.Y < m.height && d.X >= 0 && d.X < m.width {
			grid[d.Y][d.X] = d.Char
		}
		// Update position
		d.Y += d.Speed
		if d.Y >= m.height {
			d.Y = 0
			d.Char = m.chars[time.Now().UnixNano()%int64(len(m.chars))]
		}
	}

	// Convert to string
	var sb strings.Builder
	for _, row := range grid {
		sb.WriteString(string(row))
		sb.WriteString("\n")
	}

	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
