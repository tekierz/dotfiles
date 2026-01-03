package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/ui"
)

func main() {
	// Check for skip-intro flag
	skipIntro := false
	for _, arg := range os.Args[1:] {
		if arg == "--skip-intro" {
			skipIntro = true
		}
	}

	// Initialize the app
	app := ui.NewApp(skipIntro)

	// Create and run the Bubble Tea program
	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running installer: %v\n", err)
		os.Exit(1)
	}
}
