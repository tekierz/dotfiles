package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
