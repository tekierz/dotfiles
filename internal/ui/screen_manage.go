package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// manageScreen is the migrated ScreenHandler for the live dual-pane Manage
// screen (the management-tab "Manage" entry).
//
// State stays on App: the selected tool/pane (manageIndex, managePane,
// configFieldIndex), the install-status cache (manageInstalled /
// manageInstalledReady / installCacheLoading), the scroll offsets
// (manageToolsScroll, manageFieldsScroll), the inline-edit fields (manageEditing,
// manageEditValue, manageEditCursor, manageEditField, manageEditFieldKey), the
// status line (manageStatus), the install flags (manageInstalling,
// manageInstallID) and the streaming-install log buffer (installLogs /
// installLogScroll / installLogAutoScroll) are all read/written through
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
// Async handling: manageSavedMsg is handled here while this screen is active.
// The streaming/terminal install messages (manageInstallDoneMsg,
// manageSudoRequiredMsg, manageStartInstallMsg, manageInstallWithLogsMsg) are
// instead handled GLOBALLY in App.Update before delegation (see streaming.go),
// so the finalize + cache-refresh chain survives navigation away from this
// screen (the install worker + package-manager subprocess outlive the screen):
//   - manageSudoRequiredMsg -> tea.Exec(sudo prompt) -> manageStartInstallMsg
//   - manageStartInstallMsg -> register cancelable ctx/streamCancel, then
//     a.streamingInstallToolCmd(ctx, toolID) (FIX 3: so teardownStream cancels it)
//   - manageInstallWithLogsMsg success -> InvalidateCache + manageInstalledReady
//     =false + re-issue a.startInstallCacheLoad() so the install-status cache
//     refreshes (Phase B + C10 fix).
//
// Streaming model / no data race: a.streamingInstallToolCmd runs the package
// install inside a tea.Cmd closure, collects all output into a local slice, and
// returns a single terminal manageInstallWithLogsMsg carrying the logs. No
// goroutine touches shared App state, so the manage install is the safe
// collect-then-message pattern (no per-line stream to re-arm, no race).
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
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == keyCtrlC {
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

	// --- Async results (delegated here while this screen is active) ---
	case manageSavedMsg:
		a.handleManageSavedMsg(msg)
		return s, nil

		// The streaming/terminal install messages (manageInstallDoneMsg,
		// manageSudoRequiredMsg, manageStartInstallMsg, manageInstallWithLogsMsg)
		// are handled GLOBALLY in App.Update before delegation so the
		// finalize/cache-refresh chain survives navigation; they never reach here.
	}
	return s, nil
}

// handleKey ports the legacy handleManageKey, returning a tea.Cmd and routing
// navigation through the ScreenManager (NavigateTo) instead of poking a.screen.
func (s *manageScreen) handleKey(msg tea.KeyMsg) tea.Cmd {
	a := s.App()
	key := msg.String()

	// Inline string editor captures keys first so typing doesn't trigger global
	// bindings.
	if a.manageEditing {
		return s.manageHandleEditKey(msg, key)
	}

	// Non-editing manage UI.
	items := a.manageItems()
	if len(items) == 0 {
		if key == keyEsc {
			// ScreenMainMenu is migrated; route through the ScreenManager.
			return NavigateTo(ScreenMainMenu)
		}
		return nil
	}

	layout := a.manageLayout()
	a.manageEnsureToolsVisible(layout, len(items))
	fields := a.manageFieldsFor(items[a.manageIndex].id)
	a.manageEnsureFieldsVisible(layout, len(fields))

	// Block navigating away while a tool install is streaming: the terminal
	// manageInstallWithLogsMsg is only handled by this active screen, so leaving
	// would drop it, strand manageInstalling=true, and orphan the install
	// subprocess. (The 'i' install trigger is already guarded.)
	if a.manageInstalling {
		if key == keyEsc {
			a.manageStatus = "Install in progress…"
			return nil
		}
		if _, ok := tabNavigationTarget(key); ok {
			a.manageStatus = "Install in progress…"
			return nil
		}
	}

	// Handle tab navigation first (1-5 keys). A number key for the already-active
	// tab is a no-op.
	if target, ok := tabNavigationTarget(key); ok {
		if target == s.ID() {
			return nil
		}
		return s.navigateTab(target)
	}

	// Global actions (navigation, save, install, hotkeys, log scrolling). If one
	// matched, return its command; otherwise fall through to pane navigation.
	if cmd, ok := s.manageHandleGlobalKey(key, layout, items); ok {
		return cmd
	}

	// Pane-specific navigation.
	if a.managePane == managePaneTools {
		return s.manageHandleToolsPaneKey(key, layout, items)
	}

	// Settings pane.
	return s.manageHandleSettingsPaneKey(key, layout, fields)
}

// manageHandleGlobalKey handles the screen-wide action keys that apply
// regardless of which pane is focused. The returned bool reports whether the
// key matched a global action; when false the caller falls through to
// pane-specific navigation exactly as before. It is the extracted body of
// handleKey's global-action switch.
func (s *manageScreen) manageHandleGlobalKey(key string, layout manageLayout, items []manageItem) (tea.Cmd, bool) {
	a := s.App()
	switch key {
	// Global navigation.
	case keyEsc:
		a.manageStatus = ""
		a.manageCancelEditing()
		a.managePane = managePaneTools
		// ScreenMainMenu is migrated; route through the ScreenManager.
		return NavigateTo(ScreenMainMenu), true

	case keyTab:
		if a.managePane == managePaneTools {
			a.managePane = managePaneSettings
		} else {
			a.managePane = managePaneTools
		}
		return nil, true

	// Save (persist to config).
	case "s", "ctrl+s":
		a.manageStatus = "Saving…"
		return a.saveManageConfigCmd(), true

	case "i":
		return s.manageHandleInstallKey(items), true

	case "?":
		// Jump to hotkeys/cheatsheet for the selected tool.
		item := items[a.manageIndex]
		a.hotkeyFilter = ""
		if item.id != manageItemGlobal {
			a.hotkeyFilter = item.id
		}
		a.hotkeyCategory = 0
		a.hotkeyCursor = 0
		a.hotkeyCatScroll = 0
		a.hotkeyItemScroll = 0
		a.hotkeysPane = 0
		a.hotkeysReturn = ScreenManage
		// ScreenHotkeys is migrated; route through the ScreenManager.
		return NavigateTo(ScreenHotkeys), true

	case "c", "C":
		// Clear install logs (only when not installing).
		if !a.manageInstalling && len(a.installLogs) > 0 {
			a.clearInstallLogs()
			a.manageStatus = "Logs cleared"
		}
		return nil, true

	case "pgup", "ctrl+u":
		// Scroll logs up (when viewing logs).
		if len(a.installLogs) > 0 {
			a.installLogScroll += 10
			maxScroll := CalculateMaxLogScroll(len(a.installLogs), layout.bodyH-6)
			if a.installLogScroll > maxScroll {
				a.installLogScroll = maxScroll
			}
			a.installLogAutoScroll = false
		}
		return nil, true

	case "pgdown", "ctrl+d":
		// Scroll logs down (when viewing logs).
		if len(a.installLogs) > 0 {
			a.installLogScroll -= 10
			if a.installLogScroll < 0 {
				a.installLogScroll = 0
			}
		}
		return nil, true
	}

	return nil, false
}

// manageHandleInstallKey starts the install flow for the selected tool/app
// (settings pane only). It is the extracted body of handleKey's "i" case.
func (s *manageScreen) manageHandleInstallKey(items []manageItem) tea.Cmd {
	a := s.App()
	// Install selected tool/app (settings pane only).
	if a.managePane != managePaneSettings {
		return nil
	}
	item := items[a.manageIndex]
	if item.id == manageItemGlobal {
		a.manageStatus = "Select a tool/app to install"
		return nil
	}
	if a.manageInstalling {
		return nil
	}
	if item.installed {
		a.manageStatus = "Already installed"
		return nil
	}

	// Clear logs and start install flow (will check sudo first).
	a.clearInstallLogs()
	a.manageStatus = ""
	a.manageInstalling = true
	a.manageInstallID = item.id
	return a.checkSudoAndInstallCmd(item.id)
}

// manageHandleEditKey handles a key press while the inline string editor is
// active. It is the extracted body of handleKey's a.manageEditing branch.
func (s *manageScreen) manageHandleEditKey(msg tea.KeyMsg, key string) tea.Cmd {
	a := s.App()
	switch key {
	case keyEsc:
		a.manageCancelEditing()
		return nil

	case keyEnter:
		a.manageCommitEditing()
		a.manageStatus = "Updated ✓"
		return nil

	case keyLeft, "h":
		if a.manageEditCursor > 0 {
			a.manageEditCursor--
		}
		return nil

	case keyRight, "l":
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

	case keyBackspace:
		r := []rune(a.manageEditValue)
		cur := clampInt(a.manageEditCursor, 0, len(r))
		if cur > 0 {
			r = append(r[:cur-1], r[cur:]...)
			a.manageEditCursor = cur - 1
			a.manageEditValue = string(r)
		}
		return nil

	case keyDelete:
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

// manageCurrentField returns the currently selected settings field (clamped to
// the fields slice) and whether one exists.
func (s *manageScreen) manageCurrentField(fields []manageField) (manageField, bool) {
	if len(fields) == 0 {
		return manageField{}, false
	}
	a := s.App()
	idx := clampInt(a.configFieldIndex, 0, len(fields)-1)
	return fields[idx], true
}

// manageAdjustField cycles option fields or steps number fields in the given
// direction (text/toggle fields are unaffected by left/right cycling).
func (s *manageScreen) manageAdjustField(fields []manageField, dir int) {
	a := s.App()
	f, ok := s.manageCurrentField(fields)
	if !ok {
		return
	}
	switch f.kind {
	case manageFieldOption:
		if f.str != nil && len(f.options) > 0 {
			*f.str = cycleStringOption(f.options, *f.str, dir > 0)
			switch f.key {
			case manageFieldTheme:
				a.syncThemeIndex()
			case "nav":
				a.syncScreenContext()
			}
		}
	case manageFieldNumber:
		if f.n != nil {
			step := f.step
			if step == 0 {
				step = 1
			}
			*f.n = clampInt(*f.n+(dir*step), f.min, f.max)
		}
	case manageFieldText, manageFieldToggle:
		// Text/toggle fields are not adjusted by left/right cycling.
	}
}

// manageToggleField flips the current toggle field, if any.
func (s *manageScreen) manageToggleField(fields []manageField) {
	a := s.App()
	f, ok := s.manageCurrentField(fields)
	if !ok {
		return
	}
	if f.kind == manageFieldToggle && f.b != nil {
		*f.b = !*f.b
		if f.key == manageFieldAnims {
			a.syncScreenContext()
		}
	}
}

// manageStartEditingField begins inline editing of the current field.
func (s *manageScreen) manageStartEditingField(fields []manageField) {
	a := s.App()
	f, ok := s.manageCurrentField(fields)
	if !ok {
		return
	}
	a.manageStartEditing(f)
}

// manageHandleToolsPaneKey handles navigation keys while the tools (left) pane
// is focused. It is the extracted body of handleKey's managePaneTools branch.
func (s *manageScreen) manageHandleToolsPaneKey(key string, layout manageLayout, items []manageItem) tea.Cmd {
	a := s.App()
	switch key {
	case "up", "k":
		if a.manageIndex > 0 {
			a.manageIndex--
			a.configFieldIndex = 0
			a.manageFieldsScroll = 0
		}
		a.manageEnsureToolsVisible(layout, len(items))
		return nil

	case keyDown, "j":
		if a.manageIndex < len(items)-1 {
			a.manageIndex++
			a.configFieldIndex = 0
			a.manageFieldsScroll = 0
		}
		a.manageEnsureToolsVisible(layout, len(items))
		return nil

	case keyRight, "l", keyEnter:
		a.managePane = managePaneSettings
		return nil
	}

	return nil
}

// manageHandleSettingsPaneKey handles navigation/edit keys while the settings
// (right) pane is focused. It is the extracted body of handleKey's settings
// pane switch.
func (s *manageScreen) manageHandleSettingsPaneKey(key string, layout manageLayout, fields []manageField) tea.Cmd {
	a := s.App()
	switch key {
	case "up", "k":
		if a.configFieldIndex > 0 {
			a.configFieldIndex--
		}
		a.manageEnsureFieldsVisible(layout, len(fields))
		return nil

	case keyDown, "j":
		if a.configFieldIndex < len(fields)-1 {
			a.configFieldIndex++
		}
		a.manageEnsureFieldsVisible(layout, len(fields))
		return nil

	case keyLeft, "h":
		s.manageAdjustField(fields, -1)
		return nil

	case keyRight, "l":
		s.manageAdjustField(fields, 1)
		return nil

	case " ":
		return s.manageActivateFieldSpace(fields)

	case keyEnter:
		return s.manageActivateFieldEnter(fields)
	}

	return nil
}

// manageActivateFieldSpace handles the space key on the current settings field:
// space toggles booleans and acts as "forward" for options/numbers. It is the
// extracted body of manageHandleSettingsPaneKey's " " case.
func (s *manageScreen) manageActivateFieldSpace(fields []manageField) tea.Cmd {
	a := s.App()
	if f, ok := s.manageCurrentField(fields); ok {
		switch f.kind {
		case manageFieldToggle:
			wasEnabled := a.animationsEnabled
			s.manageToggleField(fields)
			if f.key == manageFieldAnims && a.animationsEnabled && !wasEnabled {
				// Restart the UI tick when enabling animations.
				return tickUI()
			}
		case manageFieldOption:
			s.manageAdjustField(fields, 1)
		case manageFieldNumber:
			s.manageAdjustField(fields, 1)
		case manageFieldText:
			// Space does not start editing a text field; only enter does.
		}
	}
	return nil
}

// manageActivateFieldEnter handles the enter key on the current settings field:
// enter toggles booleans or starts editing for text fields. It is the extracted
// body of manageHandleSettingsPaneKey's keyEnter case.
func (s *manageScreen) manageActivateFieldEnter(fields []manageField) tea.Cmd {
	a := s.App()
	if f, ok := s.manageCurrentField(fields); ok {
		switch f.kind {
		case manageFieldToggle:
			wasEnabled := a.animationsEnabled
			s.manageToggleField(fields)
			if f.key == manageFieldAnims && a.animationsEnabled && !wasEnabled {
				return tickUI()
			}
		case manageFieldText:
			s.manageStartEditingField(fields)
		case manageFieldOption:
			s.manageAdjustField(fields, 1)
		case manageFieldNumber:
			// No modal editor for numbers yet; treat as increment.
			s.manageAdjustField(fields, 1)
		}
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
	// through navigateTab (NavigateTo + on-enter load). Blocked while installing
	// so the streaming install message can't be dropped by a screen switch.
	if !a.manageInstalling && m.Y == 0 && m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
		if screen := a.detectTabClick(m.X); screen != 0 && screen != s.ID() {
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
		return s.manageHandleMouseWheel(m, layout, items)
	}

	// Only respond to left click presses for now.
	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return nil
	}

	// Click in left pane list area: select tool.
	if layout.inLeftList(m.X, m.Y) {
		return s.manageHandleLeftListClick(m, layout, items)
	}

	// Click in right pane fields area: focus + edit/toggle/adjust.
	if layout.inRightList(m.X, m.Y) {
		return s.manageHandleRightListClick(m, layout)
	}

	return nil
}

// manageHandleMouseWheel applies a wheel-scroll event to whichever pane the
// pointer is over. It is the extracted body of handleMouse's m.IsWheel() branch.
func (s *manageScreen) manageHandleMouseWheel(m tea.MouseEvent, layout manageLayout, items []manageItem) tea.Cmd {
	a := s.App()
	delta := 0
	switch m.Button {
	case tea.MouseButtonWheelUp:
		delta = -1
	case tea.MouseButtonWheelDown:
		delta = 1
	case tea.MouseButtonNone, tea.MouseButtonLeft, tea.MouseButtonMiddle,
		tea.MouseButtonRight, tea.MouseButtonWheelLeft, tea.MouseButtonWheelRight,
		tea.MouseButtonBackward, tea.MouseButtonForward, tea.MouseButton10, tea.MouseButton11:
		return nil
	}

	if m.X < layout.rightX { // left side (tools)
		a.manageToolsScroll = clampInt(a.manageToolsScroll+delta, 0, layout.maxToolsScroll(len(items)))
	} else { // right side (fields)
		fields := a.manageFieldsFor(items[a.manageIndex].id)
		a.manageFieldsScroll = clampInt(a.manageFieldsScroll+delta, 0, layout.maxFieldsScroll(len(fields)))
	}
	return nil
}

// manageHandleLeftListClick selects the clicked tool in the left pane. It is the
// extracted body of handleMouse's layout.inLeftList branch.
func (s *manageScreen) manageHandleLeftListClick(m tea.MouseEvent, layout manageLayout, items []manageItem) tea.Cmd {
	a := s.App()
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

// manageHandleRightListClick focuses and edits/toggles/adjusts the clicked
// settings field. It is the extracted body of handleMouse's layout.inRightList
// branch.
func (s *manageScreen) manageHandleRightListClick(m tea.MouseEvent, layout manageLayout) tea.Cmd {
	a := s.App()
	// While the install-log view occupies the right pane, the settings fields
	// are not rendered (renderManageSettingsPanel swaps to the log panel when
	// installing or logs exist). Ignore field hit-testing in that state so a
	// click in the log region does not mutate hidden settings fields.
	if a.manageInstalling || len(a.installLogs) > 0 {
		return nil
	}

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

	return s.manageApplyFieldClick(fields[fieldIdx], m, layout)
}

// manageApplyFieldClick toggles/cycles/steps the clicked settings field based on
// the click X position (left half decrements/backward, right half
// increments/forward). It is the extracted body of manageHandleRightListClick's
// switch over the field kind.
func (s *manageScreen) manageApplyFieldClick(f manageField, m tea.MouseEvent, layout manageLayout) tea.Cmd {
	a := s.App()
	switch f.kind {
	case manageFieldToggle:
		if f.b != nil {
			wasEnabled := a.animationsEnabled
			*f.b = !*f.b
			if f.key == manageFieldAnims && a.animationsEnabled && !wasEnabled {
				// Restart UI tick if animations were turned back on via mouse.
				return tickUI()
			}
		}
	case manageFieldOption:
		// Click on left half cycles backward, right half cycles forward.
		forward := m.X >= (layout.rightX + layout.rightW/2)
		if f.str != nil && len(f.options) > 0 {
			*f.str = cycleStringOption(f.options, *f.str, forward)
			if f.key == manageFieldTheme {
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
			*f.n = clampInt(*f.n+dir*step, f.min, f.max)
		}
	case manageFieldText:
		// Single click just focuses. Enter starts editing (keyboard) for now.
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
			return loadingMessage
		}
		a.width, a.height = width, height
	}

	// Show loading state if the install-status cache is being populated.
	if a.installCacheLoading {
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
