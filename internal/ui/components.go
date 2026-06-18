package ui

// CalculateMaxLogScroll returns the maximum scroll value for a log panel.
func CalculateMaxLogScroll(totalLines, visibleHeight int) int {
	if totalLines <= visibleHeight {
		return 0
	}
	return totalLines - visibleHeight
}
