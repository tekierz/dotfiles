package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/health"
)

func adjustManageNumber(value, dir, step, minValue, maxValue int) int {
	if step <= 0 {
		step = 1
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	if dir > 0 {
		if value > maxValue-step {
			return maxValue
		}
		return value + step
	}
	if value < minValue+step {
		return minValue
	}
	return value - step
}

// manageScreen is the migrated ScreenHandler for the live dual-pane Manage
// screen (the management-tab "Manage" entry).
//
// State stays on App: the selected tool/pane (manageIndex, managePane,
// configFieldIndex), the install-status cache (manageInstalled /
// manageInstalledReady / installCacheLoading), the scroll offsets
// (manageToolsScroll, manageFieldsScroll), the inline-edit fields (manageEditing,
// manageEditValue, manageEditCursor, manageEditField, manageEditFieldKey), the
// status line (manageStatus) and editable state are all read/written through
// s.App(). The dual-pane layout, item list, field list and every renderManage*
// helper remain methods on *App and are reused unchanged.
//
// On-enter load: Init() kicks the shared install-status cache load via
// a.startInstallCacheLoad() (idempotent; guarded against double-loading). The
// main menu and tab navigation also kick this load before navigating (shared
// startTabTargetLoad), so the cache loads exactly once however the screen is
// entered. The cache result (installCacheDoneMsg) is applied globally in
// App.Update before delegation, so it is intentionally NOT handled here.
//
// Installs are routed through the reviewed plan and shared installation worker;
// Manage has no separate direct package-install message path.
type manageScreen struct {
	BaseScreen
}

// NewManageScreen creates a new Manage dual-pane screen handler.
func NewManageScreen(ctx *ScreenContext) *manageScreen {
	s := &manageScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *manageScreen) ID() Screen { return ScreenManage }

// Init kicks the shared install-status cache load on entry (idempotent).
func (s *manageScreen) Init() tea.Cmd {
	a := s.App()
	if a == nil {
		return nil
	}
	return a.startInstallCacheLoad()
}

// navigateTab routes a management-tab switch through the ScreenManager and kicks
// the destination's on-enter load (shared with the legacy tab navigation).
// Migrated destinations enter managed mode; legacy destinations (Users) fall
// back to legacy mode harmlessly.
func (s *manageScreen) navigateTab(target Screen) tea.Cmd {
	a := s.App()
	return tea.Batch(NavigateTo(target), startTabTargetLoad(a, target))
}

// Update handles keyboard, mouse, and the Manage async result messages.
func (s *manageScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	theme, nav, animations := a.theme, a.navStyle, a.animationsEnabled
	defer func() {
		if a.theme != theme || a.navStyle != nav || a.animationsEnabled != animations {
			a.syncSharedSettings()
			a.invalidateSettingsReviews()
		}
	}()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			// Tear down any in-flight install worker + subprocess before quitting.
			a.teardownStream()
			return s, tea.Quit
		}
		// 'q' quits from the Manage screen except while the inline string editor
		// is active (so typing 'q' into a field doesn't quit). This mirrors the
		// legacy global quit guard: !(screen == ScreenManage && manageEditing).
		if msg.String() == "q" && !a.manageEditing {
			a.teardownStream()
			return s, tea.Quit
		}
		return s, s.handleKey(msg)
	case tea.MouseMsg:
		return s, s.handleMouse(msg)
	}
	return s, nil
}

// handleKey ports the legacy handleManageKey, returning a tea.Cmd and routing
// navigation through the ScreenManager (NavigateTo) instead of poking a.screen.
func (s *manageScreen) handleKey(msg tea.KeyMsg) tea.Cmd {
	a := s.App()
	key := msg.String()
	lazyGitBlockReason := ""
	itemsAtInput := a.manageItems()
	if len(itemsAtInput) > 0 && itemsAtInput[clampInt(a.manageIndex, 0, len(itemsAtInput)-1)].id == "lazygit" {
		lazyGitBlockReason = lazyGitManageUIBlockReason(a)
	}
	if a.manageEditing && lazyGitBlockReason != "" {
		a.manageCancelEditing()
		a.manageStatus = "LazyGit settings are read-only: " + lazyGitBlockReason
		return nil
	}

	// Inline string editor captures keys first so typing doesn't trigger global
	// bindings.
	if a.manageEditing {
		switch key {
		case "esc":
			a.manageCancelEditing()
			return nil

		case "enter":
			if a.manageCommitEditing() {
				a.manageStatus = "Updated ✓"
			}
			return nil

		case "left", "h":
			if a.manageEditCursor > 0 {
				a.manageEditCursor--
			}
			return nil

		case "right", "l":
			if a.manageEditCursor < utf8.RuneCountInString(a.manageEditValue) {
				a.manageEditCursor++
			}
			return nil

		case "home":
			a.manageEditCursor = 0
			return nil

		case "end":
			a.manageEditCursor = utf8.RuneCountInString(a.manageEditValue)
			return nil

		case "backspace":
			r := []rune(a.manageEditValue)
			cur := clampInt(a.manageEditCursor, 0, len(r))
			if cur > 0 {
				r = append(r[:cur-1], r[cur:]...)
				a.manageEditCursor = cur - 1
				a.manageEditValue = string(r)
			}
			return nil

		case "delete":
			r := []rune(a.manageEditValue)
			cur := clampInt(a.manageEditCursor, 0, len(r))
			if cur < len(r) {
				r = append(r[:cur], r[cur+1:]...)
				a.manageEditValue = string(r)
			}
			return nil

		default:
			// Insert typed runes (ignore non-rune keys and alt-modified keys).
			// Note: Bubble Tea represents Ctrl combinations as KeyType values (not
			// KeyRunes).
			if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 && !msg.Alt {
				r := []rune(a.manageEditValue)
				cur := clampInt(a.manageEditCursor, 0, len(r))
				insert := msg.Runes

				out := make([]rune, 0, len(r)+len(insert))
				out = append(out, r[:cur]...)
				out = append(out, insert...)
				out = append(out, r[cur:]...)

				a.manageEditValue = string(out)
				a.manageEditCursor = cur + len(insert)
			}
			return nil
		}
	}

	// Non-editing manage UI.
	items := a.manageItems()
	if len(items) == 0 {
		if key == "esc" {
			// ScreenMainMenu is migrated; route through the ScreenManager.
			return NavigateTo(ScreenMainMenu)
		}
		return nil
	}

	layout := a.manageLayout()
	a.manageEnsureToolsVisible(layout, len(items))
	fields := a.manageFieldsFor(items[a.manageIndex].id)
	a.manageEnsureFieldsVisible(layout, len(fields))

	// Helpers.
	currentField := func() (manageField, bool) {
		if len(fields) == 0 {
			return manageField{}, false
		}
		idx := clampInt(a.configFieldIndex, 0, len(fields)-1)
		return fields[idx], true
	}

	adjustField := func(dir int) {
		f, ok := currentField()
		if !ok {
			return
		}
		if a.manageFieldMutationBlocked(f) {
			return
		}
		switch f.kind {
		case manageFieldOption:
			if f.str != nil && len(f.options) > 0 {
				if f.unknownReadOnly && !oneOf(*f.str, f.options...) {
					a.manageStatus = "Custom native LazyGit values are read-only"
					return
				}
				*f.str = cycleStringOption(f.options, *f.str, dir > 0)
				if f.key == "theme" {
					a.syncThemeIndex()
				}
			}
		case manageFieldNumber:
			if f.n != nil {
				step := f.step
				if step == 0 {
					step = 1
				}
				*f.n = adjustManageNumber(*f.n, dir, step, f.min, f.max)
			}
		case manageFieldText, manageFieldToggle:
			// Text fields use edit mode; toggles have no ordered adjustment.
		}
	}

	toggleField := func() {
		f, ok := currentField()
		if !ok {
			return
		}
		if a.manageFieldMutationBlocked(f) {
			return
		}
		if f.kind == manageFieldToggle && f.b != nil {
			*f.b = !*f.b
		}
	}

	startEditingField := func() {
		f, ok := currentField()
		if !ok {
			return
		}
		a.manageStartEditing(f)
	}

	// Handle tab navigation first (1-5 keys). A number key for the already-active
	// tab is a no-op.
	if target, ok := tabNavigationTarget(key); ok {
		if target == s.ID() {
			return nil
		}
		return s.navigateTab(target)
	}

	switch key {
	// Global navigation.
	case "esc":
		a.manageStatus = ""
		a.manageCancelEditing()
		a.managePane = managePaneTools
		// ScreenMainMenu is migrated; route through the ScreenManager.
		return NavigateTo(ScreenMainMenu)

	case "tab":
		if a.managePane == managePaneTools {
			a.managePane = managePaneSettings
		} else {
			a.managePane = managePaneTools
		}
		return nil

	// Save (persist to config).
	case "s", "ctrl+s":
		a.manageStatus = ""
		return a.prepareManageSave()

	case "i", "I":
		// Install the selected tool/app from either pane. Both panes consume the
		// same accepted installation snapshot and build the same reviewed,
		// single-tool plan; rendering focus must not change install authority.
		item := items[a.manageIndex]
		if item.id == "global" {
			a.manageStatus = "Select a tool/app to install"
			return nil
		}
		if a.installationSnapshotLoading || a.installCacheLoading {
			a.manageStatus = "Installation status loading"
			return nil
		}
		if a.installationSnapshotError != "" {
			a.manageStatus = installationSnapshotUnavailable
			return nil
		}
		if a.installationSnapshotStale {
			a.manageStatus = "Installation status stale"
			return nil
		}
		if !a.installationSnapshotReady {
			a.manageStatus = "Installation status unknown"
			return nil
		}
		presence, installability := item.installationTruth()
		if presence == health.PresencePresent {
			a.manageStatus = "Already installed"
			return nil
		}
		if item.unavailableReason != "" {
			a.manageStatus = item.unavailableReason
			return nil
		}
		if presence == health.PresenceUnknown {
			a.manageStatus = "Installation status unknown"
			return nil
		}
		if installability == health.InstallabilityUnsupported {
			a.manageStatus = "Installation unavailable on " + a.installationSnapshot.Platform()
			return nil
		}
		if installability != health.InstallabilitySupported {
			a.manageStatus = "Installation availability unknown"
			return nil
		}
		action := item.installationAction()
		if action != "install" && action != "repair" {
			a.manageStatus = "Installation status unknown"
			return nil
		}

		plan, err := buildInstallPlanForTools(a, defaultToolInstallRuntime(), time.Now(), []string{item.id})
		if err != nil || plan == nil || plan.hasBlocked() {
			a.pendingInstallPlan = nil
			if err == nil {
				err = fmt.Errorf("%s", installationSnapshotUnavailable)
			}
			a.installPlanError = err
			a.manageStatus = installationSnapshotUnavailable
			return nil
		}
		a.pendingInstallPlan = plan
		a.installReviewTools = []string{item.id}
		a.installPlanError = nil
		a.installPlanScroll = 0
		a.manageStatus = strings.ToUpper(action[:1]) + action[1:] + " requested"
		return NavigateTo(ScreenFileTree)

	case "?":
		// Jump to hotkeys/cheatsheet for the selected tool.
		item := items[a.manageIndex]
		a.hotkeyFilter = ""
		if item.id != "global" {
			a.hotkeyFilter = item.id
		}
		a.hotkeyCategory = 0
		a.hotkeyCursor = 0
		a.hotkeyCatScroll = 0
		a.hotkeyItemScroll = 0
		a.hotkeysPane = 0
		a.hotkeysReturn = ScreenManage
		// ScreenHotkeys is migrated; route through the ScreenManager.
		return NavigateTo(ScreenHotkeys)
	}

	// Pane-specific navigation.
	if a.managePane == managePaneTools {
		switch key {
		case "up", "k":
			if a.manageIndex > 0 {
				a.manageIndex--
				a.configFieldIndex = 0
				a.manageFieldsScroll = 0
			}
			a.manageEnsureToolsVisible(layout, len(items))
			return nil

		case "down", "j":
			if a.manageIndex < len(items)-1 {
				a.manageIndex++
				a.configFieldIndex = 0
				a.manageFieldsScroll = 0
			}
			a.manageEnsureToolsVisible(layout, len(items))
			return nil

		case "right", "l", "enter":
			a.managePane = managePaneSettings
			return nil
		}

		return nil
	}

	// Settings pane.
	if lazyGitBlockReason != "" && oneOf(key, "left", "right", "h", "l", " ", "enter") {
		a.manageStatus = "LazyGit settings are read-only: " + lazyGitBlockReason
		return nil
	}
	switch key {
	case "up", "k":
		if a.configFieldIndex > 0 {
			a.configFieldIndex--
		}
		a.manageEnsureFieldsVisible(layout, len(fields))
		return nil

	case "down", "j":
		if a.configFieldIndex < len(fields)-1 {
			a.configFieldIndex++
		}
		a.manageEnsureFieldsVisible(layout, len(fields))
		return nil

	case "left", "h":
		adjustField(-1)
		return nil

	case "right", "l":
		adjustField(1)
		return nil

	case " ":
		// Space toggles booleans. For options/numbers, it acts as "forward".
		if f, ok := currentField(); ok {
			switch f.kind {
			case manageFieldToggle:
				wasEnabled := a.animationsEnabled
				toggleField()
				if f.key == "animations" && a.animationsEnabled && !wasEnabled {
					// Restart the UI tick when enabling animations.
					return tickUI()
				}
			case manageFieldOption:
				adjustField(1)
			case manageFieldNumber:
				adjustField(1)
			case manageFieldText:
				// Space is inserted only while the text editor is active.
			}
		}
		return nil

	case "enter":
		// Enter toggles boolean fields, or starts editing for text fields.
		if f, ok := currentField(); ok {
			switch f.kind {
			case manageFieldToggle:
				wasEnabled := a.animationsEnabled
				toggleField()
				if f.key == "animations" && a.animationsEnabled && !wasEnabled {
					return tickUI()
				}
			case manageFieldText, manageFieldNumber:
				startEditingField()
			case manageFieldOption:
				adjustField(1)
			}
		}
		return nil
	}

	return nil
}

// handleMouse ports the legacy handleManageMouse, preserving the pixel-precise
// hit-testing exactly. Navigation routes through the ScreenManager (NavigateTo).
func (s *manageScreen) handleMouse(msg tea.MouseMsg) tea.Cmd {
	a := s.App()
	m := tea.MouseEvent(msg)

	// When editing, keep interaction keyboard-driven to avoid confusing focus
	// shifts and accidental toggles.
	if a.manageEditing {
		return nil
	}

	// Nothing to do if we don't have a valid layout yet.
	if a.width <= 0 || a.height <= 0 {
		return nil
	}

	// Handle tab bar clicks (Y=0 is the tab bar line). Ignore a click on the
	// already-active tab (this screen). Migrated destinations enter managed mode;
	// legacy destinations (Users) fall back to legacy mode harmlessly. Both go
	// through navigateTab (NavigateTo + on-enter load).
	if m.Y == 0 && m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
		target := a.detectTabClick(m.X)
		if a.compactManageSinglePaneActive() {
			target = detectCompactManageTabClick(m.X)
		}
		if screen := target; screen != 0 && screen != s.ID() {
			return s.navigateTab(screen)
		}
	}

	layout := a.manageLayout()
	items := a.manageItems()

	// Bounds guard: the wheel-scroll fields branch indexes items[a.manageIndex],
	// so an empty list (or a stale index) would panic. Match the click path,
	// which guards len(items) before indexing.
	if len(items) == 0 {
		return nil
	}
	a.manageIndex = clampInt(a.manageIndex, 0, len(items)-1)

	// Wheel scroll: choose pane based on mouse X.
	if m.IsWheel() {
		delta := 0
		switch m.Button { //nolint:exhaustive // Only vertical wheel actions are meaningful here.
		case tea.MouseButtonWheelUp:
			delta = -1
		case tea.MouseButtonWheelDown:
			delta = 1
		default:
			return nil
		}

		if m.X < layout.rightX { // left side (tools)
			oldScroll := a.manageToolsScroll
			newScroll := clampInt(oldScroll+delta, 0, layout.maxToolsScroll(len(items)))
			a.manageToolsScroll = newScroll
			if a.manageIndex == oldScroll {
				a.manageIndex = newScroll
			} else if a.manageIndex < newScroll {
				a.manageIndex = newScroll
			} else if a.manageIndex >= newScroll+layout.leftListH {
				a.manageIndex = newScroll + layout.leftListH - 1
			}
		} else { // right side (fields)
			fields := a.manageFieldsFor(items[a.manageIndex].id)
			oldScroll := a.manageFieldsScroll
			newScroll := clampInt(oldScroll+delta, 0, layout.maxFieldsScroll(len(fields)))
			a.manageFieldsScroll = newScroll
			if a.configFieldIndex == oldScroll {
				a.configFieldIndex = newScroll
			} else if a.configFieldIndex < newScroll {
				a.configFieldIndex = newScroll
			} else if a.configFieldIndex >= newScroll+layout.rightListH {
				a.configFieldIndex = newScroll + layout.rightListH - 1
			}
		}
		return nil
	}

	// Only respond to left click presses for now.
	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return nil
	}

	// Click in left pane list area: select tool.
	if layout.inLeftList(m.X, m.Y) {
		relY := m.Y - layout.leftListY
		idx := a.manageToolsScroll + relY
		if idx >= 0 && idx < len(items) {
			a.managePane = managePaneTools
			if idx != a.manageIndex {
				a.manageIndex = idx
				a.configFieldIndex = 0
				a.manageFieldsScroll = 0
				a.manageEditing = false
				a.manageEditField = nil
				a.manageEditValue = ""
				a.manageStatus = ""
			}
			a.manageEnsureToolsVisible(layout, len(items))
		}
		return nil
	}

	// Click in right pane fields area: focus + edit/toggle/adjust.
	if layout.inRightList(m.X, m.Y) {
		items := a.manageItems()
		if len(items) == 0 {
			return nil
		}

		fields := a.manageFieldsFor(items[a.manageIndex].id)
		if len(fields) == 0 {
			return nil
		}

		relY := m.Y - layout.rightListY
		fieldIdx := a.manageFieldsScroll + relY
		if fieldIdx < 0 || fieldIdx >= len(fields) {
			return nil
		}

		a.managePane = managePaneSettings
		a.configFieldIndex = fieldIdx
		a.manageEnsureFieldsVisible(layout, len(fields))
		if items[a.manageIndex].id == "lazygit" {
			if reason := lazyGitManageUIBlockReason(a); reason != "" {
				a.manageStatus = "LazyGit settings are read-only: " + reason
				return nil
			}
		}

		f := fields[fieldIdx]
		if a.manageFieldMutationBlocked(f) {
			return nil
		}
		switch f.kind {
		case manageFieldToggle:
			if f.b != nil {
				wasEnabled := a.animationsEnabled
				*f.b = !*f.b
				if f.key == "animations" && a.animationsEnabled && !wasEnabled {
					// Restart UI tick if animations were turned back on via mouse.
					return tickUI()
				}
			}
		case manageFieldOption:
			// Click on left half cycles backward, right half cycles forward.
			forward := m.X >= (layout.rightX + layout.rightW/2)
			if f.str != nil && len(f.options) > 0 {
				if f.unknownReadOnly && !oneOf(*f.str, f.options...) {
					a.manageStatus = "Custom native LazyGit values are read-only"
					return nil
				}
				*f.str = cycleStringOption(f.options, *f.str, forward)
				if f.key == "theme" {
					a.syncThemeIndex()
				}
			}
		case manageFieldNumber:
			// Click on left half decrements, right half increments.
			dir := -1
			if m.X >= (layout.rightX + layout.rightW/2) {
				dir = 1
			}
			if f.n != nil {
				step := f.step
				if step == 0 {
					step = 1
				}
				*f.n = adjustManageNumber(*f.n, dir, step, f.min, f.max)
			}
		case manageFieldText:
			// Single click just focuses. Enter starts editing (keyboard) for now.
		}

		return nil
	}

	return nil
}

// View renders the Manage dual-pane screen. It ports renderManageDualPane,
// reading the live App state (the layout uses a.width/a.height, kept in sync with
// the manager's ctx on WindowSizeMsg) and reusing the shared renderManage*
// helpers on *App. The width/height args are accepted for interface conformance
// and used as a fallback when the App dimensions are not yet set.
func (s *manageScreen) View(width, height int) string {
	a := s.App()
	if a.width == 0 || a.height == 0 {
		// Fall back to the manager-provided dimensions if the App hasn't seen a
		// WindowSizeMsg yet; otherwise the layout cannot be computed.
		if width <= 0 || height <= 0 {
			return "Loading..."
		}
		a.width, a.height = width, height
	}

	// Show loading state if the install-status cache is being populated.
	if a.installCacheLoading || a.installationSnapshotLoading {
		spinner := AnimatedSpinnerDots(a.uiFrame)
		loadingStyle := lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true)
		loadingText := loadingStyle.Render(fmt.Sprintf("%s Loading installation status...", spinner))

		return PlaceWithBackground(
			a.width, a.height,
			loadingText,
		)
	}

	layout := a.manageLayout()
	items := a.manageItems()

	// Clamp selection safely (important if config/tools list changes).
	if len(items) > 0 {
		a.manageIndex = clampInt(a.manageIndex, 0, len(items)-1)
	} else {
		a.manageIndex = 0
	}

	// Keep scrolls sane.
	a.manageEnsureToolsVisible(layout, len(items))
	fields := []manageField(nil)
	if len(items) > 0 {
		fields = a.manageFieldsFor(items[a.manageIndex].id)
	}
	a.manageEnsureFieldsVisible(layout, len(fields))
	if a.compactManageSinglePaneActive() {
		if a.managePane == managePaneTools {
			return a.renderCompactManageTools(layout, items)
		}
		if len(items) > 0 && items[a.manageIndex].id == "yazi" {
			return a.renderCompactManageYazi(layout, fields)
		}
	}

	header := a.renderManageHeader(layout.w)
	footer := a.renderManageFooter(layout.w, items, fields)

	left := a.renderManageToolsPanel(layout, items)
	right := a.renderManageSettingsPanel(layout, items, fields)

	// Style the gap between panels (no explicit background to respect terminal
	// transparency).
	gapStyle := lipgloss.NewStyle().
		Height(layout.bodyH)
	gap := gapStyle.Render(strings.Repeat(" ", layout.gap))

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)
	view := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)

	// No explicit background to respect terminal transparency.
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, view)
}
