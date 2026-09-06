package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/tekierz/dotfiles/internal/operation"
)

type manageSaveConfirmScreen struct{ BaseScreen }

func NewManageSaveConfirmScreen(ctx *ScreenContext) *manageSaveConfirmScreen {
	s := &manageSaveConfirmScreen{}
	s.SetContext(ctx)
	return s
}

func (s *manageSaveConfirmScreen) ID() Screen    { return ScreenManageSaveConfirm }
func (s *manageSaveConfirmScreen) Init() tea.Cmd { return nil }

func (s *manageSaveConfirmScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	if done, ok := msg.(manageSaveDoneMsg); ok {
		result := done.standaloneConfigExecutionResult
		a.manageSaveRunning = false
		a.manageSaveDone = result.err == nil || result.applied
		a.manageSaveManual = result.manualRecovery
		a.manageSaveWarning = result.warning
		if result.applied && a.pendingManageSavePlan != nil {
			accepted := a.pendingManageSavePlan
			a.manageConfigBaseline = accepted.snapshot
			a.manageConfigBaselineTheme = accepted.theme
			a.theme = accepted.theme
			a.navStyle = accepted.navStyle
			a.animationsEnabled = accepted.animationsEnabled
			if a.screenMgr != nil {
				a.screenMgr.Context().Theme = a.theme
				a.screenMgr.Context().NavStyle = a.navStyle
				a.screenMgr.Context().AnimationsEnabled = a.animationsEnabled
			}
		}
		switch {
		case result.err != nil && result.applied:
			a.manageStatus = "Saved and preserved; operational cleanup failed: " + result.err.Error()
		case result.err != nil:
			a.manageStatus = "Save failed: " + result.err.Error()
		case result.warning != "":
			a.manageStatus = "Saved with warning: " + result.warning
		default:
			a.manageStatus = "Saved ✓"
		}
		return s, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	if a.manageSaveRunning {
		// The accepted transaction owns rollback and result reporting until its
		// command completes. Do not let navigation or process quit abandon it.
		return s, nil
	}
	switch key.String() {
	case "q", "ctrl+c":
		return s, tea.Quit
	case "esc":
		if !a.manageSaveRunning {
			return s, NavigateTo(ScreenManage)
		}
	case "up", "k":
		if a.manageSaveScroll > 0 {
			a.manageSaveScroll--
		}
	case "down", "j":
		a.manageSaveScroll++
	case "home":
		a.manageSaveScroll = 0
	case "end":
		a.manageSaveScroll = 1 << 20
	case "enter":
		if a.manageSaveDone {
			return s, NavigateTo(ScreenManage)
		}
		if a.pendingManageSavePlan != nil && a.pendingManageSavePlan.plan != nil && !a.pendingManageSavePlan.plan.hasBlocked() && !managePlanHasApplicableChanges(a.pendingManageSavePlan.plan) {
			a.manageStatus = "No changes"
			return s, NavigateTo(ScreenManage)
		}
		if !a.manageSaveRunning && a.manageSavePlanErr == nil && a.pendingManageSavePlan != nil && a.pendingManageSavePlan.plan != nil && !a.pendingManageSavePlan.plan.hasBlocked() {
			a.manageSaveRunning = true
			a.manageStatus = "Saving reviewed changes..."
			return s, a.executeManageSavePlanCmd()
		}
	}
	return s, nil
}

func (s *manageSaveConfirmScreen) View(width, height int) string {
	a := s.App()
	if width <= 80 || height <= 24 {
		return renderCompactManageSaveConfirm(a, width, height)
	}
	lines := []string{lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("Review Manage Save"), ""}
	if a.pendingManageSavePlan == nil || a.pendingManageSavePlan.plan == nil {
		lines = append(lines, configSaveErrorStyle.Render("Blocked: "+errorText(a.manageSavePlanErr)))
	} else {
		plan := a.pendingManageSavePlan.plan
		hash := plan.hash()
		if len(hash) > 16 {
			hash = hash[:16]
		}
		lineWidth := max(24, width-10)
		lines = append(lines, "Plan: "+hash, "", "Reviewed actions:")
		for _, action := range plan.actions() {
			if action.ID == "state:parents" {
				continue
			}
			label := "  " + action.ID
			if action.Disposition != operation.DispositionApply {
				label += "  [" + string(action.Disposition) + "]"
			}
			lines = append(lines, strings.Split(ansi.Wrap(label, lineWidth, " /-_"), "\n")...)
			meta := fmt.Sprintf("    ownership: %s • rollback: %s", action.Ownership, action.Reversibility)
			lines = append(lines, strings.Split(ansi.Wrap(meta, lineWidth, " /-_"), "\n")...)
			for _, target := range orderedTargetOwnership(action) {
				detail := fmt.Sprintf("      %s: %s", target.target, target.ownership)
				lines = append(lines, strings.Split(ansi.Wrap(detail, lineWidth, " /-_"), "\n")...)
			}
			if action.Reason != "" {
				lines = append(lines, strings.Split(ansi.Wrap("    "+action.Reason, lineWidth, " /-_"), "\n")...)
			}
		}
		if targets := plan.backupTargets(); len(targets) > 0 {
			lines = append(lines, "", "Exact mandatory-backup targets:")
			for _, target := range targets {
				state := "create"
				for _, action := range plan.actions() {
					for _, observation := range action.Observations {
						if observation.Source == target && observation.Exists {
							state = "update"
						}
					}
				}
				lines = append(lines, strings.Split(ansi.Wrap(fmt.Sprintf("  %s  %s", state, target), lineWidth, " /-_"), "\n")...)
			}
		}
		if plan.hasBlocked() {
			lines = append(lines, "", configSaveErrorStyle.Render("Confirmation disabled until blocked actions are resolved."))
		} else if len(plan.backupTargets()) == 0 {
			lines = append(lines, "", "No changes to save.")
		}
	}
	if a.manageSaveManual {
		lines = append(lines, "", configSaveErrorStyle.Render("Manual recovery required; rollback could not prove every write."))
	}
	if a.manageSaveWarning != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(ColorYellow).Render("Warning: "+a.manageSaveWarning))
	}
	if a.manageStatus != "" {
		lines = append(lines, "", a.manageStatus)
	}

	viewportHeight := max(4, height-8)
	maxScroll := max(0, len(lines)-viewportHeight)
	if a.manageSaveScroll > maxScroll {
		a.manageSaveScroll = maxScroll
	}
	visible := lines[a.manageSaveScroll:min(len(lines), a.manageSaveScroll+viewportHeight)]
	help := manageSaveHelp(a)
	content := strings.Join(append(visible, "", HelpStyle.Render(help)), "\n")
	boxWidth := min(82, max(34, width-4))
	return PlaceWithBackground(width, height, ContainerStyle.Width(boxWidth).Render(content))
}

func manageSaveHelp(a *App) string {
	planAvailable := a.pendingManageSavePlan != nil && a.pendingManageSavePlan.plan != nil
	switch {
	case a.manageSaveRunning:
		return "saving reviewed changes • input is paused"
	case a.manageSaveDone:
		return "↑↓ scroll • enter/esc return • q close"
	case planAvailable && a.pendingManageSavePlan.plan.hasBlocked():
		return "↑↓ scroll • blocked • esc edit • q cancel"
	case planAvailable && !managePlanHasApplicableChanges(a.pendingManageSavePlan.plan):
		return "no changes • enter/q close • esc edit"
	default:
		return "↑↓ scroll • enter confirm • esc edit • q cancel"
	}
}

func renderCompactManageSaveConfirm(a *App, width, height int) string {
	lines := []string{"Review Manage Save"}
	if a.manageStatus != "" {
		lines = append(lines, a.manageStatus)
	}
	if a.pendingManageSavePlan == nil || a.pendingManageSavePlan.plan == nil {
		lines = append(lines, "Blocked: "+errorText(a.manageSavePlanErr))
	} else {
		plan := a.pendingManageSavePlan.plan
		hash := plan.hash()
		if len(hash) > 16 {
			hash = hash[:16]
		}
		lines = append(lines, "Plan: "+hash)
		for _, action := range plan.actions() {
			if action.ID == "state:parents" {
				continue
			}
			label := action.ID
			if action.Disposition != operation.DispositionApply {
				label += " [" + string(action.Disposition) + "]"
			}
			lines = append(lines, label)
			if action.Reason != "" {
				lines = append(lines, action.Reason)
			}
			if action.Target != "" {
				lines = append(lines, "Target: "+action.Target)
			}
		}
		if plan.hasBlocked() {
			lines = append(lines, "Confirmation disabled until blocked actions are resolved.")
		} else if !managePlanHasApplicableChanges(plan) {
			lines = append(lines, "No changes to save.")
		}
	}
	if a.manageSaveManual {
		lines = append(lines, "Manual recovery required")
	}
	if a.manageSaveWarning != "" {
		lines = append(lines, "Warning: "+a.manageSaveWarning)
	}
	wrapped := wrapCompactConfirmationSource(width, lines)
	viewportHeight := max(0, height-1)
	maxScroll := max(0, len(wrapped)-viewportHeight)
	start := clampInt(a.manageSaveScroll, 0, maxScroll)
	if !a.manageSaveRunning {
		a.manageSaveScroll = start
	}
	end := min(len(wrapped), start+viewportHeight)
	return renderCompactConfirmationRows(width, height, wrapped[start:end], manageSaveHelp(a))
}

func managePlanHasApplicableChanges(plan *installPlan) bool {
	if plan == nil {
		return false
	}
	for _, action := range plan.actions() {
		if action.Disposition == operation.DispositionApply {
			return true
		}
	}
	return false
}
