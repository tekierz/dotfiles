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

// Layout offsets shared between renderUsersDualPane and handleUsersMouse so
// mouse hit detection stays in sync with what is actually drawn.
//   - usersTabBarRows: rows consumed by the tab bar (RenderTabBar emits 1 line).
//   - usersHeaderRows: rows each pane draws (a header line + a divider line)
//     before the first selectable list/field row.
const (
	usersTabBarRows = 1
	usersHeaderRows = 2
)

// userItem represents a user in the left pane.
type userItem struct {
	name     string
	theme    string
	navStyle string
	keyboard string
	isActive bool
}

// userFieldKind describes field types for editing.
type userFieldKind int

const (
	userFieldOption userFieldKind = iota
	userFieldText
)

// userField represents an editable field in the right pane.
type userField struct {
	key         string
	label       string
	description string
	kind        userFieldKind
	value       string
	options     []string
}

// userLoadedMsg is sent when user list is loaded.
type userLoadedMsg struct {
	users      []userItem
	activeUser string
	err        error
}

// userSavedMsg is sent after saving a user profile.
type userSavedMsg struct {
	name string
	err  error
}

// userDeletedMsg is sent after deleting a user profile.
type userDeletedMsg struct {
	name string
	err  error
}

// userSwitchedMsg is sent after switching to a user.
type userSwitchedMsg struct {
	name string
	err  error
}

// loadUsersCmd loads all user profiles.
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

// saveUserCmd saves a user profile.
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

// deleteUserCmd deletes a user profile.
func deleteUserCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if err := config.DeleteUserProfile(name); err != nil {
			return userDeletedMsg{name: name, err: err}
		}

		// If the deleted profile was the active user, clear ActiveUser so the
		// hotkeys/favorites system (keyed by global config's ActiveUser) is not
		// left pointing at a now-nonexistent profile, orphaning its data.
		if cfg, err := config.LoadGlobalConfig(); err == nil && cfg != nil && cfg.ActiveUser == name {
			// Best effort: a failure here leaves the profile deleted but the
			// active marker stale; surface nothing further since the deletion
			// itself succeeded.
			_ = config.ClearActiveUser()
		}

		return userDeletedMsg{name: name}
	}
}

// switchUserCmd switches to a user profile.
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

// usersScreen is the migrated ScreenHandler for the Users dual-pane screen (the
// management-tab "Users" entry: profile list + per-profile settings).
//
// State stays on App: the cached profile list (usersItems), the selection
// (usersIndex / usersPane / usersFieldIndex), the load flag (usersLoaded), the
// transient input modes (usersCreating / usersNewName / usersDeleting) and the
// status line (usersStatus) are read/written through s.App(). The render helpers
// (renderUsersListPane, renderUsersSettingsPane, renderUsersStatusBar) and the
// field helpers (getUserFields, cycleUserFieldOption) remain methods on *App and
// are reused unchanged.
//
// On-enter load: Init() loads the profile list via loadUsersCmd when it has not
// already been loaded, mirroring the legacy on-enter trigger. The shared
// startTabTargetLoad (used by tab navigation in the Manage/Users handlers) also
// kicks this load before navigating, and both guard on usersLoaded, so the list
// loads exactly once however the screen is entered.
//
// Async-in-handler: because the ScreenManager delegates every non-navigation
// message to this handler while it is active, the Users async results are
// handled here (not in App.Update): userLoadedMsg, userSavedMsg, userDeletedMsg,
// userSwitchedMsg. The save/delete/switch results re-issue loadUsersCmd to
// refresh the list. Deleting the active user clears ActiveUser inside
// deleteUserCmd (the Phase B fix), preserved here.
type usersScreen struct {
	BaseScreen
}

// NewUsersScreen creates a new Users dual-pane screen handler.
func NewUsersScreen(ctx *ScreenContext) *usersScreen {
	s := &usersScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *usersScreen) ID() Screen { return ScreenUsers }

// Init loads the profile list on entry (idempotent against usersLoaded).
func (s *usersScreen) Init() tea.Cmd {
	a := s.App()
	if a == nil {
		return nil
	}
	if !a.usersLoaded {
		a.usersLoaded = true
		return loadUsersCmd()
	}
	return nil
}

// navigateTab routes a management-tab switch through the ScreenManager and kicks
// the destination's on-enter load (shared with the legacy tab navigation).
func (s *usersScreen) navigateTab(target Screen) tea.Cmd {
	a := s.App()
	return tea.Batch(NavigateTo(target), startTabTargetLoad(a, target))
}

// Update handles keyboard, mouse, and the Users async result messages.
func (s *usersScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return s, tea.Quit
		}
		// 'q' quits except while typing a new user name (so it doesn't quit
		// mid-entry). This mirrors the legacy global quit guard, which only
		// special-cased ScreenManage's inline editor; here the name-entry mode is
		// the equivalent text-capture state for Users.
		if msg.String() == "q" && !a.usersCreating {
			return s, tea.Quit
		}
		return s, s.handleKey(msg)

	case tea.MouseMsg:
		return s, s.handleMouse(msg)

	// --- Async results (delegated here while this screen is active) ---
	case userLoadedMsg:
		if msg.err != nil {
			// Reset the guard so re-entering the screen retries the load
			// instead of permanently stranding an empty list.
			a.usersLoaded = false
			a.usersStatus = fmt.Sprintf("Load failed: %v", msg.err)
		} else {
			a.usersItems = msg.users
			a.usersStatus = ""
		}
		return s, nil

	case userSavedMsg:
		if msg.err != nil {
			a.usersStatus = fmt.Sprintf("Save failed: %v", msg.err)
		} else {
			a.usersStatus = fmt.Sprintf("Saved %s ✓", msg.name)
			// Reload user list.
			return s, loadUsersCmd()
		}
		return s, nil

	case userDeletedMsg:
		if msg.err != nil {
			a.usersStatus = fmt.Sprintf("Delete failed: %v", msg.err)
		} else {
			a.usersStatus = fmt.Sprintf("Deleted %s", msg.name)
			// Reload user list and adjust index.
			if a.usersIndex > 0 {
				a.usersIndex--
			}
			return s, loadUsersCmd()
		}
		return s, nil

	case userSwitchedMsg:
		if msg.err != nil {
			a.usersStatus = fmt.Sprintf("Switch failed: %v", msg.err)
		} else {
			a.usersStatus = fmt.Sprintf("Switched to %s ✓", msg.name)
			// Reload user list to update active indicator.
			return s, loadUsersCmd()
		}
		return s, nil
	}
	return s, nil
}

// handleKey ports the legacy handleUsersKey, returning a tea.Cmd and routing
// navigation through the ScreenManager (NavigateTo) instead of poking a.screen.
func (s *usersScreen) handleKey(msg tea.KeyMsg) tea.Cmd {
	a := s.App()
	key := msg.String()

	// Handle new user name input.
	if a.usersCreating {
		switch key {
		case "esc":
			a.usersCreating = false
			a.usersNewName = ""
			return nil
		case "enter":
			if a.usersNewName != "" {
				if err := config.ValidateUsername(a.usersNewName); err != nil {
					a.usersStatus = fmt.Sprintf("Invalid: %v", err)
					return nil
				}
				// Create with defaults.
				a.usersCreating = false
				name := a.usersNewName
				a.usersNewName = ""
				return saveUserCmd(name, "catppuccin-mocha", "emacs", "linux")
			}
			return nil
		case "backspace":
			if len(a.usersNewName) > 0 {
				a.usersNewName = a.usersNewName[:len(a.usersNewName)-1]
			}
			return nil
		default:
			// Add character to name (only alphanumeric, underscore, hyphen).
			if len(key) == 1 && len(a.usersNewName) < 32 {
				c := key[0]
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
					(c >= '0' && c <= '9' && len(a.usersNewName) > 0) ||
					(c == '_' || c == '-') && len(a.usersNewName) > 0 {
					a.usersNewName += key
				}
			}
			return nil
		}
	}

	// Handle delete confirmation.
	if a.usersDeleting {
		switch key {
		case "y", "Y":
			a.usersDeleting = false
			if a.usersIndex < len(a.usersItems) {
				name := a.usersItems[a.usersIndex].name
				return deleteUserCmd(name)
			}
			return nil
		case "n", "N", "esc":
			a.usersDeleting = false
			return nil
		}
		return nil
	}

	// Tab navigation (number keys for tabs). A number key for the already-active
	// tab is a no-op.
	if target, ok := tabNavigationTarget(key); ok {
		if target == s.ID() {
			return nil
		}
		return s.navigateTab(target)
	}

	switch key {
	case "tab", "shift+tab":
		// Toggle pane.
		if a.usersPane == usersPaneList {
			a.usersPane = usersPaneSettings
		} else {
			a.usersPane = usersPaneList
		}
		return nil

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
		return nil

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
		return nil

	case "left", "h":
		if a.usersPane == usersPaneSettings {
			fields := a.getUserFields()
			if a.usersFieldIndex < len(fields) {
				f := fields[a.usersFieldIndex]
				if f.kind == userFieldOption {
					a.cycleUserFieldOption(f, -1)
					return nil
				}
			}
		}
		return nil

	case "right", "l":
		if a.usersPane == usersPaneSettings {
			fields := a.getUserFields()
			if a.usersFieldIndex < len(fields) {
				f := fields[a.usersFieldIndex]
				if f.kind == userFieldOption {
					a.cycleUserFieldOption(f, 1)
					return nil
				}
			}
		}
		return nil

	case "enter":
		if a.usersPane == usersPaneList {
			// Switch to selected user.
			if a.usersIndex < len(a.usersItems) {
				name := a.usersItems[a.usersIndex].name
				return switchUserCmd(name)
			}
		} else {
			// Cycle option field.
			fields := a.getUserFields()
			if a.usersFieldIndex < len(fields) {
				f := fields[a.usersFieldIndex]
				if f.kind == userFieldOption {
					a.cycleUserFieldOption(f, 1)
				}
			}
		}
		return nil

	case "n", "a":
		// New user.
		a.usersCreating = true
		a.usersNewName = ""
		return nil

	case "d", "x":
		// Delete user (with confirmation).
		if len(a.usersItems) > 0 && a.usersIndex < len(a.usersItems) {
			a.usersDeleting = true
		}
		return nil

	case "s":
		// Save current user's settings.
		if len(a.usersItems) > 0 && a.usersIndex < len(a.usersItems) {
			item := a.usersItems[a.usersIndex]
			return saveUserCmd(item.name, item.theme, item.navStyle, item.keyboard)
		}
		return nil

	case "r":
		// Refresh user list.
		return loadUsersCmd()

	case "esc":
		// ScreenMainMenu is migrated; route through the ScreenManager. ('q' is
		// handled as quit in Update, matching the legacy global quit.)
		return NavigateTo(ScreenMainMenu)
	}

	return nil
}

// getUserFields returns the editable fields for the current user.
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

// cycleUserFieldOption cycles through options for a field.
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

// handleMouse ports the legacy handleUsersMouse. Tab-bar clicks route through
// the ScreenManager (NavigateTo via navigateTab); list/field clicks update the
// selection. The hit detection matches what View draws.
func (s *usersScreen) handleMouse(msg tea.MouseMsg) tea.Cmd {
	a := s.App()
	m := tea.MouseEvent(msg)

	// Tab bar is a single line at Y=0 (see RenderTabBar in styles.go). Ignore a
	// click on the already-active tab (this screen).
	if m.Y == 0 {
		if m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
			if screen, _ := a.detectTabClick(m.X); screen != 0 && screen != s.ID() {
				return s.navigateTab(screen)
			}
		}
		return nil
	}

	// Row of the first selectable item within either pane. The layout is:
	// tab bar (Y=0), then each pane renders a header line, a divider line,
	// and only then the first list/field row. This matches renderUsersListPane
	// and renderUsersSettingsPane, which both write header + divider before
	// the first row.
	const firstRowY = usersTabBarRows + usersHeaderRows

	// Handle list clicks.
	if m.Button == tea.MouseButtonLeft && m.Action == tea.MouseActionPress {
		// Capture the pane active during the LAST render before any reassignment.
		// The description-line compensation below must reference this value, not
		// the post-click pane (which would always be usersPaneSettings after the
		// assignment on line 600, making the guard a tautology).
		prevPane := a.usersPane

		// Check if click is in the left pane (user list).
		leftPaneWidth := a.width / 3
		if m.X < leftPaneWidth {
			a.usersPane = usersPaneList
			// Calculate which user was clicked (accounting for header).
			userIdx := m.Y - firstRowY
			if userIdx >= 0 && userIdx < len(a.usersItems) {
				a.usersIndex = userIdx
			}
		} else {
			a.usersPane = usersPaneSettings
			// Calculate which field was clicked. renderUsersSettingsPane draws an
			// EXTRA description line immediately after the currently-SELECTED field
			// (when it has a non-empty description), but ONLY when the settings pane
			// was active during the last render (prevPane == usersPaneSettings). On a
			// first click that switches INTO the settings pane (prevPane==usersPaneList)
			// no description line was drawn, so the shift must NOT apply (C19).
			fields := a.getUserFields()
			fieldIdx := m.Y - firstRowY
			sel := a.usersFieldIndex
			if prevPane == usersPaneSettings && sel >= 0 && sel < len(fields) && fields[sel].description != "" {
				descRow := sel + 1 // the inserted description line's relative row
				switch {
				case fieldIdx == descRow:
					// Click landed on the selected field's description line: keep the
					// selection on that field rather than selecting the next one.
					fieldIdx = sel
				case fieldIdx > descRow:
					// Below the inserted line: shift up by one to undo the offset.
					fieldIdx--
				}
			}
			if fieldIdx >= 0 && fieldIdx < len(fields) {
				a.usersFieldIndex = fieldIdx
			}
		}
	}

	return nil
}

// minUsersWidth/minUsersHeight are the smallest terminal dimensions at which
// the dual-pane layout can meaningfully render. Below these the screen falls
// back to a "too small" stub. The thresholds mirror the clamps used by other
// screens (Manage, Update, Hotkeys) that also have multi-pane layouts.
const (
	minUsersWidth  = 10
	minUsersHeight = 5
)

// View renders the Users management screen. It ports renderUsersDualPane,
// reading the live App state (the layout uses a.width/a.height, kept in sync
// with the manager's ctx on WindowSizeMsg). The width/height args are accepted
// for interface conformance and used as a fallback when the App dimensions are
// not yet set.
func (s *usersScreen) View(width, height int) string {
	a := s.App()
	if a.width == 0 || a.height == 0 {
		if width <= 0 || height <= 0 {
			return "Loading..."
		}
		a.width, a.height = width, height
	}

	// Guard against tiny positive dimensions that would produce negative derived
	// widths/heights (and panic in strings.Repeat). Other screens use the same
	// "too small" fall-through; mirror that idiom here.
	if a.width < minUsersWidth || a.height < minUsersHeight {
		return "Loading..."
	}

	// Tab bar at top.
	tabBar := RenderTabBar(ScreenUsers, a.width)

	// Calculate pane dimensions. Clamp all derived values to ≥0 so that any
	// future resize race cannot produce a negative strings.Repeat count.
	leftWidth := maxInt(0, a.width/3)
	rightWidth := maxInt(0, a.width-leftWidth-3) // -3 for separator
	contentHeight := maxInt(0, a.height-4)       // Tab bar + status line

	// Left pane: user list
	leftPane := a.renderUsersListPane(leftWidth, contentHeight)

	// Right pane: user settings
	rightPane := a.renderUsersSettingsPane(rightWidth, contentHeight)

	// Separator — guard against the (now-clamped) zero case.
	sep := ""
	if contentHeight > 0 {
		sep = lipgloss.NewStyle().
			Foreground(ColorBorder).
			Render(strings.Repeat("│\n", contentHeight))
	}

	// Join panes
	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, sep, rightPane)

	// Status bar
	statusBar := a.renderUsersStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, tabBar, content, statusBar)
}

// renderUsersListPane renders the left pane with user list.
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

// renderUsersSettingsPane renders the right pane with user settings.
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

// renderUsersStatusBar renders the status bar at the bottom.
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
