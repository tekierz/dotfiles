package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/config"
)

// ==========================
// Users Screen (Dual Pane)
// ==========================
//
// Design:
// - Left pane: List of user profiles with active indicator
// - Right pane: User settings (theme, nav, keyboard)
// - Actions: Create, Switch, Delete, Edit

const (
	usersPaneList     = 0
	usersPaneSettings = 1
)

// userItem represents a user in the left pane
type userItem struct {
	name     string
	theme    string
	navStyle string
	keyboard string
	isActive bool
}

// userFieldKind describes field types for editing
type userFieldKind int

const (
	userFieldOption userFieldKind = iota
	userFieldText
)

// userField represents an editable field in the right pane
type userField struct {
	key         string
	label       string
	description string
	kind        userFieldKind
	value       string
	options     []string
}

// userLoadedMsg is sent when user list is loaded
type userLoadedMsg struct {
	users      []userItem
	activeUser string
	err        error
}

// userSavedMsg is sent after saving a user profile
type userSavedMsg struct {
	name string
	err  error
}

// userDeletedMsg is sent after deleting a user profile
type userDeletedMsg struct {
	name string
	err  error
}

// userSwitchedMsg is sent after switching to a user
type userSwitchedMsg struct {
	name string
	err  error
}

// loadUsersCmd loads all user profiles
func loadUsersCmd() tea.Cmd {
	return func() tea.Msg {
		names, err := config.ListUserProfiles()
		if err != nil {
			return userLoadedMsg{err: err}
		}

		cfg, _ := config.LoadGlobalConfig()
		activeUser := ""
		if cfg != nil {
			activeUser = cfg.ActiveUser
		}

		var users []userItem
		for _, name := range names {
			profile, err := config.LoadUserProfile(name)
			if err != nil {
				continue
			}
			users = append(users, userItem{
				name:     profile.Name,
				theme:    profile.Theme,
				navStyle: profile.NavStyle,
				keyboard: profile.KeyboardStyle,
				isActive: profile.Name == activeUser,
			})
		}

		return userLoadedMsg{users: users, activeUser: activeUser}
	}
}

// saveUserCmd saves a user profile
func saveUserCmd(name, theme, nav, keyboard string) tea.Cmd {
	return func() tea.Msg {
		profile := &config.UserProfile{
			Name:          name,
			Theme:         theme,
			NavStyle:      nav,
			KeyboardStyle: keyboard,
		}

		// Load existing profile to preserve timestamps
		existing, err := config.LoadUserProfile(name)
		if err == nil {
			profile.CreatedAt = existing.CreatedAt
		}

		if err := config.SaveUserProfile(profile); err != nil {
			return userSavedMsg{name: name, err: err}
		}
		return userSavedMsg{name: name}
	}
}

// deleteUserCmd deletes a user profile
func deleteUserCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if err := config.DeleteUserProfile(name); err != nil {
			return userDeletedMsg{name: name, err: err}
		}
		return userDeletedMsg{name: name}
	}
}

// switchUserCmd switches to a user profile
func switchUserCmd(name string) tea.Cmd {
	return func() tea.Msg {
		profile, err := config.LoadUserProfile(name)
		if err != nil {
			return userSwitchedMsg{name: name, err: err}
		}
		if err := config.ApplyUserProfile(profile); err != nil {
			return userSwitchedMsg{name: name, err: err}
		}
		return userSwitchedMsg{name: name}
	}
}

// handleUsersKey handles keyboard input on the Users screen
func (a *App) handleUsersKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Handle new user name input
	if a.usersCreating {
		switch key {
		case "esc":
			a.usersCreating = false
			a.usersNewName = ""
			return a, nil
		case "enter":
			if a.usersNewName != "" {
				if err := config.ValidateUsername(a.usersNewName); err != nil {
					a.usersStatus = fmt.Sprintf("Invalid: %v", err)
					return a, nil
				}
				// Create with defaults
				a.usersCreating = false
				name := a.usersNewName
				a.usersNewName = ""
				return a, saveUserCmd(name, "catppuccin-mocha", "emacs", "linux")
			}
			return a, nil
		case "backspace":
			if len(a.usersNewName) > 0 {
				a.usersNewName = a.usersNewName[:len(a.usersNewName)-1]
			}
			return a, nil
		default:
			// Add character to name (only alphanumeric, underscore, hyphen)
			if len(key) == 1 && len(a.usersNewName) < 32 {
				c := key[0]
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
					(c >= '0' && c <= '9' && len(a.usersNewName) > 0) ||
					(c == '_' || c == '-') && len(a.usersNewName) > 0 {
					a.usersNewName += key
				}
			}
			return a, nil
		}
	}

	// Handle delete confirmation
	if a.usersDeleting {
		switch key {
		case "y", "Y":
			a.usersDeleting = false
			if a.usersIndex < len(a.usersItems) {
				name := a.usersItems[a.usersIndex].name
				return a, deleteUserCmd(name)
			}
			return a, nil
		case "n", "N", "esc":
			a.usersDeleting = false
			return a, nil
		}
		return a, nil
	}

	// Tab navigation (number keys for tabs)
	if handled, cmd := a.handleTabNavigationWithCmd(key); handled {
		return a, cmd
	}

	switch key {
	case "tab", "shift+tab":
		// Toggle pane
		if a.usersPane == usersPaneList {
			a.usersPane = usersPaneSettings
		} else {
			a.usersPane = usersPaneList
		}
		return a, nil

	case "up", "k":
		if a.usersPane == usersPaneList {
			if a.usersIndex > 0 {
				a.usersIndex--
			}
		} else {
			if a.usersFieldIndex > 0 {
				a.usersFieldIndex--
			}
		}
		return a, nil

	case "down", "j":
		if a.usersPane == usersPaneList {
			if a.usersIndex < len(a.usersItems)-1 {
				a.usersIndex++
			}
		} else {
			fields := a.getUserFields()
			if a.usersFieldIndex < len(fields)-1 {
				a.usersFieldIndex++
			}
		}
		return a, nil

	case "left", "h":
		if a.usersPane == usersPaneSettings {
			fields := a.getUserFields()
			if a.usersFieldIndex < len(fields) {
				f := fields[a.usersFieldIndex]
				if f.kind == userFieldOption {
					a.cycleUserFieldOption(f, -1)
					return a, nil
				}
			}
		}
		return a, nil

	case "right", "l":
		if a.usersPane == usersPaneSettings {
			fields := a.getUserFields()
			if a.usersFieldIndex < len(fields) {
				f := fields[a.usersFieldIndex]
				if f.kind == userFieldOption {
					a.cycleUserFieldOption(f, 1)
					return a, nil
				}
			}
		}
		return a, nil

	case "enter":
		if a.usersPane == usersPaneList {
			// Switch to selected user
			if a.usersIndex < len(a.usersItems) {
				name := a.usersItems[a.usersIndex].name
				return a, switchUserCmd(name)
			}
		} else {
			// Cycle option field
			fields := a.getUserFields()
			if a.usersFieldIndex < len(fields) {
				f := fields[a.usersFieldIndex]
				if f.kind == userFieldOption {
					a.cycleUserFieldOption(f, 1)
				}
			}
		}
		return a, nil

	case "n", "a":
		// New user
		a.usersCreating = true
		a.usersNewName = ""
		return a, nil

	case "d", "x":
		// Delete user (with confirmation)
		if len(a.usersItems) > 0 && a.usersIndex < len(a.usersItems) {
			a.usersDeleting = true
		}
		return a, nil

	case "s":
		// Save current user's settings
		if len(a.usersItems) > 0 && a.usersIndex < len(a.usersItems) {
			item := a.usersItems[a.usersIndex]
			return a, saveUserCmd(item.name, item.theme, item.navStyle, item.keyboard)
		}
		return a, nil

	case "r":
		// Refresh user list
		return a, loadUsersCmd()

	case "q", "esc":
		a.screen = ScreenMainMenu
		return a, nil
	}

	return a, nil
}

// getUserFields returns the editable fields for the current user
func (a *App) getUserFields() []userField {
	if len(a.usersItems) == 0 || a.usersIndex >= len(a.usersItems) {
		return nil
	}

	item := a.usersItems[a.usersIndex]
	return []userField{
		{
			key:         "theme",
			label:       "Theme",
			description: "Color theme for all tools",
			kind:        userFieldOption,
			value:       item.theme,
			options:     config.AvailableThemes,
		},
		{
			key:         "nav",
			label:       "Navigation",
			description: "Keyboard navigation style",
			kind:        userFieldOption,
			value:       item.navStyle,
			options:     []string{"emacs", "vim"},
		},
		{
			key:         "keyboard",
			label:       "Keyboard",
			description: "Desktop keyboard style",
			kind:        userFieldOption,
			value:       item.keyboard,
			options:     []string{"linux", "macos"},
		},
	}
}

// cycleUserFieldOption cycles through options for a field
func (a *App) cycleUserFieldOption(f userField, delta int) {
	if len(a.usersItems) == 0 || a.usersIndex >= len(a.usersItems) {
		return
	}

	item := &a.usersItems[a.usersIndex]
	current := f.value

	// Find current index in options
	idx := 0
	for i, opt := range f.options {
		if opt == current {
			idx = i
			break
		}
	}

	// Cycle
	idx += delta
	if idx < 0 {
		idx = len(f.options) - 1
	} else if idx >= len(f.options) {
		idx = 0
	}

	newValue := f.options[idx]

	// Update the item
	switch f.key {
	case "theme":
		item.theme = newValue
	case "nav":
		item.navStyle = newValue
	case "keyboard":
		item.keyboard = newValue
	}

	a.usersStatus = "Modified (press 's' to save)"
}

// handleUsersMouse handles mouse input on the Users screen
func (a *App) handleUsersMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Handle tab bar clicks
	if msg.Y <= 2 {
		return a.handleTabBarMouse(msg)
	}

	// Handle list clicks
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		// Check if click is in the left pane (user list)
		leftPaneWidth := a.width / 3
		if msg.X < leftPaneWidth {
			a.usersPane = usersPaneList
			// Calculate which user was clicked (accounting for header)
			userIdx := msg.Y - 5 // Adjust for tab bar + header
			if userIdx >= 0 && userIdx < len(a.usersItems) {
				a.usersIndex = userIdx
			}
		} else {
			a.usersPane = usersPaneSettings
			// Calculate which field was clicked
			fieldIdx := msg.Y - 5
			fields := a.getUserFields()
			if fieldIdx >= 0 && fieldIdx < len(fields) {
				a.usersFieldIndex = fieldIdx
			}
		}
	}

	return a, nil
}

// renderUsersDualPane renders the Users management screen
func (a *App) renderUsersDualPane() string {
	// Load users if not already loaded
	if !a.usersLoaded {
		a.usersLoaded = true
		// Trigger async load - will be handled via message
	}

	// Tab bar at top
	tabBar := RenderTabBar(ScreenUsers, a.width)

	// Calculate pane dimensions
	leftWidth := a.width / 3
	rightWidth := a.width - leftWidth - 3 // -3 for separator
	contentHeight := a.height - 4         // Tab bar + status line

	// Left pane: user list
	leftPane := a.renderUsersListPane(leftWidth, contentHeight)

	// Right pane: user settings
	rightPane := a.renderUsersSettingsPane(rightWidth, contentHeight)

	// Separator
	sep := lipgloss.NewStyle().
		Foreground(ColorBorder).
		Render(strings.Repeat("│\n", contentHeight))

	// Join panes
	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, sep, rightPane)

	// Status bar
	statusBar := a.renderUsersStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, tabBar, content, statusBar)
}

// renderUsersListPane renders the left pane with user list
func (a *App) renderUsersListPane(width, height int) string {
	var b strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorCyan).
		Width(width).
		Padding(0, 1)

	if a.usersCreating {
		b.WriteString(headerStyle.Render("New User: " + a.usersNewName + "█"))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Padding(0, 1).
			Render("Enter name, Esc to cancel"))
		b.WriteString("\n\n")
	} else if a.usersDeleting && a.usersIndex < len(a.usersItems) {
		b.WriteString(headerStyle.Render("Delete " + a.usersItems[a.usersIndex].name + "?"))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(ColorYellow).
			Padding(0, 1).
			Render("Press Y to confirm, N to cancel"))
		b.WriteString("\n\n")
	} else {
		b.WriteString(headerStyle.Render("User Profiles"))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(ColorBorder).
			Render(strings.Repeat("─", width)))
		b.WriteString("\n")
	}

	if len(a.usersItems) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Padding(1, 1)
		b.WriteString(emptyStyle.Render("No user profiles."))
		b.WriteString("\n")
		b.WriteString(emptyStyle.Render("Press 'n' to create one."))
	} else {
		for i, item := range a.usersItems {
			isSelected := i == a.usersIndex && a.usersPane == usersPaneList

			// Active indicator
			marker := "○"
			markerColor := ColorTextMuted
			if item.isActive {
				marker = "●"
				markerColor = ColorGreen
			}

			markerStyle := lipgloss.NewStyle().Foreground(markerColor)

			// Name style
			nameStyle := lipgloss.NewStyle().Padding(0, 1)
			if isSelected {
				nameStyle = nameStyle.
					Bold(true).
					Background(ColorSurface).
					Foreground(ColorCyan)
			} else {
				nameStyle = nameStyle.Foreground(ColorText)
			}

			line := fmt.Sprintf("%s %s", markerStyle.Render(marker), nameStyle.Render(item.name))
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	// Pad to height
	lines := strings.Count(b.String(), "\n")
	for i := lines; i < height-1; i++ {
		b.WriteString("\n")
	}

	return lipgloss.NewStyle().Width(width).Render(b.String())
}

// renderUsersSettingsPane renders the right pane with user settings
func (a *App) renderUsersSettingsPane(width, height int) string {
	var b strings.Builder

	if len(a.usersItems) == 0 || a.usersIndex >= len(a.usersItems) {
		emptyStyle := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Padding(1, 1)
		b.WriteString(emptyStyle.Render("Select a user to view settings"))
		return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
	}

	item := a.usersItems[a.usersIndex]

	// Header with user name
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorMagenta).
		Width(width).
		Padding(0, 1)

	b.WriteString(headerStyle.Render(fmt.Sprintf("Settings: %s", item.name)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(ColorBorder).
		Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	// Render fields
	fields := a.getUserFields()
	for i, f := range fields {
		isSelected := i == a.usersFieldIndex && a.usersPane == usersPaneSettings

		// Label
		labelStyle := lipgloss.NewStyle().
			Width(12).
			Padding(0, 1)
		if isSelected {
			labelStyle = labelStyle.Bold(true).Foreground(ColorCyan)
		} else {
			labelStyle = labelStyle.Foreground(ColorText)
		}

		// Value
		valueStyle := lipgloss.NewStyle().Padding(0, 1)
		if isSelected {
			valueStyle = valueStyle.
				Background(ColorSurface).
				Foreground(ColorMagenta).
				Bold(true)
		} else {
			valueStyle = valueStyle.Foreground(ColorTextMuted)
		}

		// For option fields, show arrows
		valueStr := f.value
		if f.kind == userFieldOption && isSelected {
			valueStr = fmt.Sprintf("◀ %s ▶", valueStr)
		}

		line := labelStyle.Render(f.label+":") + valueStyle.Render(valueStr)
		b.WriteString(line)
		b.WriteString("\n")

		// Description
		if isSelected && f.description != "" {
			descStyle := lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true).
				Padding(0, 1)
			b.WriteString(descStyle.Render("  " + f.description))
			b.WriteString("\n")
		}
	}

	// Help text
	b.WriteString("\n")
	helpStyle := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Padding(0, 1)
	b.WriteString(helpStyle.Render("←/→ change • Enter switch user • s save"))

	// Pad to height
	lines := strings.Count(b.String(), "\n")
	for i := lines; i < height-1; i++ {
		b.WriteString("\n")
	}

	return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
}

// renderUsersStatusBar renders the status bar at the bottom
func (a *App) renderUsersStatusBar() string {
	statusStyle := lipgloss.NewStyle().
		Width(a.width).
		Padding(0, 1).
		Foreground(ColorTextMuted)

	helpText := "n:new  d:delete  s:save  r:refresh  Tab:switch pane  q:back"
	if a.usersStatus != "" {
		helpText = a.usersStatus + " │ " + helpText
	}

	return statusStyle.Render(helpText)
}
