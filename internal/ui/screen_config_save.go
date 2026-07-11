package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/operation"
)

type configSaveConfirmScreen struct{ BaseScreen }

var configSaveErrorStyle = lipgloss.NewStyle().Foreground(ColorRed).Bold(true)

func NewConfigSaveConfirmScreen(ctx *ScreenContext) *configSaveConfirmScreen {
	s := &configSaveConfirmScreen{}
	s.SetContext(ctx)
	return s
}

func (s *configSaveConfirmScreen) ID() Screen    { return ScreenConfigSaveConfirm }
func (s *configSaveConfirmScreen) Init() tea.Cmd { return nil }

func (s *configSaveConfirmScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	if done, ok := msg.(standaloneConfigSaveDoneMsg); ok {
		a.standaloneConfigRunning = false
		a.standaloneConfigDone = done.err == nil || done.applied
		a.standaloneConfigManualRecovery = done.manualRecovery
		a.standaloneConfigWarning = done.warning
		if done.err != nil {
			a.standaloneConfigErr = done.err
			if done.applied {
				a.standaloneConfigStatus = "Configuration applied and preserved; operational cleanup failed"
			} else {
				a.standaloneConfigStatus = "Save failed"
			}
		} else if done.warning != "" {
			a.standaloneConfigStatus = "Configuration saved with a backup-retention warning"
		} else {
			a.standaloneConfigStatus = "Configuration saved successfully"
		}
		return s, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch key.String() {
	case "q", "ctrl+c":
		if !a.standaloneConfigRunning {
			return s, tea.Quit
		}
	case "esc":
		if !a.standaloneConfigRunning && !a.standaloneConfigDone {
			a.standaloneConfigStatus = ""
			a.standaloneConfigErr = nil
			return s, NavigateTo(a.startScreen)
		}
	case "enter":
		if a.standaloneConfigDone {
			return s, tea.Quit
		}
		if !a.standaloneConfigRunning && a.standaloneConfigPlan != nil && !a.standaloneConfigPlan.hasBlocked() && len(a.standaloneConfigPlan.configTools) == 0 {
			return s, tea.Quit
		}
		if !a.standaloneConfigRunning && a.standaloneConfigErr == nil && a.standaloneConfigPlan != nil && !a.standaloneConfigPlan.hasBlocked() && len(a.standaloneConfigPlan.configTools) == 1 {
			a.standaloneConfigRunning = true
			a.standaloneConfigStatus = "Saving reviewed configuration..."
			return s, a.executeStandaloneConfigPlanCmd()
		}
	}
	return s, nil
}

func (s *configSaveConfirmScreen) View(width, height int) string {
	a := s.App()
	var body []string
	body = append(body, lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("Review Configuration Save"), "")
	if a.standaloneConfigPlan == nil {
		body = append(body, configSaveErrorStyle.Render("Blocked: "+errorText(a.standaloneConfigErr)))
	} else {
		plan := a.standaloneConfigPlan
		toolID := "unknown"
		for _, action := range plan.actions() {
			if action.Kind == operation.KindWriteConfig {
				toolID = action.ToolID
				break
			}
		}
		body = append(body,
			fmt.Sprintf("Tool: %s", toolID),
			fmt.Sprintf("Plan: %s", plan.hash()),
			"",
			"Exact rollback-covered targets:",
		)
		for _, target := range plan.backupTargets() {
			state := "create"
			for _, action := range plan.actions() {
				for _, observation := range action.Observations {
					if filepath.Clean(observation.Source) == filepath.Clean(target) && observation.Exists {
						state = "update"
					}
				}
			}
			body = append(body, fmt.Sprintf("  %s  %s", state, target))
		}
		for _, action := range plan.actions() {
			if action.Kind != operation.KindWriteConfig {
				continue
			}
			body = append(body, "", fmt.Sprintf("Ownership: %s", action.Ownership))
			if action.Disposition == operation.DispositionApply && len(plan.backupTargets()) != 0 {
				body = append(body, "Backup: mandatory before mutation")
			}
			if action.Disposition == operation.DispositionBlocked || action.Disposition == operation.DispositionSkip {
				body = append(body, configSaveErrorStyle.Render(fmt.Sprintf("%s: %s", action.Disposition, action.Reason)))
			}
		}
	}
	if a.standaloneConfigErr != nil {
		body = append(body, "", configSaveErrorStyle.Render(errorText(a.standaloneConfigErr)))
	}
	if a.standaloneConfigManualRecovery {
		body = append(body, configSaveErrorStyle.Render("Manual recovery required; automatic rollback could not prove every write."))
	}
	if a.standaloneConfigWarning != "" {
		body = append(body, lipgloss.NewStyle().Foreground(ColorYellow).Render("Warning: "+a.standaloneConfigWarning))
	}
	if a.standaloneConfigStatus != "" {
		body = append(body, "", a.standaloneConfigStatus)
	}
	help := "enter confirm • esc edit • q cancel"
	if a.standaloneConfigDone {
		help = "enter/q close"
	} else if a.standaloneConfigPlan != nil && !a.standaloneConfigPlan.hasBlocked() && len(a.standaloneConfigPlan.configTools) == 0 {
		help = "no changes • enter/q close • esc edit"
	}
	body = append(body, "", HelpStyle.Render(help))
	content := strings.Join(body, "\n")
	return PlaceWithBackground(width, height, ContainerStyle.Width(min(76, max(40, width-6))).Render(content))
}

func errorText(err error) string {
	if err == nil {
		return "configuration plan is unavailable"
	}
	return err.Error()
}
