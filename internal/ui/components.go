package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderLogPanel renders a scrollable log panel with the given logs
// height is the number of visible lines, scroll is the offset from bottom (0 = show newest)
func RenderLogPanel(logs []string, width, height, scroll int, title string, active bool) string {
	if width <= 2 || height <= 2 {
		return ""
	}

	// Panel styling
	borderColor := ColorBorder
	if active {
		borderColor = ColorCyan
	}

	// Calculate inner dimensions (accounting for border + padding)
	innerWidth := width - 4   // 1 border + 1 padding each side
	innerHeight := height - 4 // 1 border + 1 padding + 1 title line each side

	if innerHeight < 1 {
		innerHeight = 1
	}
	if innerWidth < 10 {
		innerWidth = 10
	}

	// Calculate visible log range
	totalLines := len(logs)
	if totalLines == 0 {
		// Empty state
		emptyMsg := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("Waiting for output...")
		content := lipgloss.Place(innerWidth, innerHeight, lipgloss.Center, lipgloss.Center, emptyMsg)
		return renderLogPanelFrame(content, width, height, title, borderColor)
	}

	// Calculate which lines to show (scroll from bottom)
	// scroll=0 means show the newest lines at the bottom
	endIdx := totalLines - scroll
	if endIdx < 0 {
		endIdx = 0
	}
	if endIdx > totalLines {
		endIdx = totalLines
	}

	startIdx := endIdx - innerHeight
	if startIdx < 0 {
		startIdx = 0
	}

	// Extract visible lines
	var visibleLines []string
	for i := startIdx; i < endIdx; i++ {
		line := logs[i]
		// Truncate long lines to fit width
		if lipgloss.Width(line) > innerWidth {
			line = truncateVisible(line, innerWidth)
		}
		visibleLines = append(visibleLines, line)
	}

	// Pad to fill height
	for len(visibleLines) < innerHeight {
		visibleLines = append([]string{""}, visibleLines...)
	}

	content := strings.Join(visibleLines, "\n")
	return renderLogPanelFrame(content, width, height, title, borderColor)
}

// renderLogPanelFrame renders the frame around log content
func renderLogPanelFrame(content string, width, height int, title string, borderColor lipgloss.Color) string {
	// Title styling
	titleStyle := lipgloss.NewStyle().
		Foreground(ColorNeonPink).
		Bold(true)

	// Build the panel
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(width - 2).
		Height(height - 2)

	// Combine title and content
	titleLine := titleStyle.Render(title)
	innerContent := lipgloss.JoinVertical(lipgloss.Left, titleLine, content)

	return panel.Render(innerContent)
}

// LogPanelScrollInfo returns scroll indicator string for log panel
func LogPanelScrollInfo(logs []string, visibleHeight, scroll int) string {
	totalLines := len(logs)
	if totalLines <= visibleHeight {
		return ""
	}

	// Calculate position
	maxScroll := totalLines - visibleHeight
	if maxScroll < 0 {
		maxScroll = 0
	}

	// scroll=0 is at bottom, maxScroll is at top
	if scroll >= maxScroll {
		return "▲ TOP"
	} else if scroll == 0 {
		return "▼ BOTTOM (auto-scroll)"
	}

	// Calculate percentage
	pct := 100 - (scroll * 100 / maxScroll)
	return lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		strings.Repeat("▲", 1) + " " + string(rune('0'+pct/10)) + string(rune('0'+pct%10)) + "%",
	)
}

// CalculateMaxLogScroll returns the maximum scroll value for a log panel
func CalculateMaxLogScroll(totalLines, visibleHeight int) int {
	if totalLines <= visibleHeight {
		return 0
	}
	return totalLines - visibleHeight
}
