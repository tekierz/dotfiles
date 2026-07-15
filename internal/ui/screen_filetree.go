package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tekierz/dotfiles/internal/operation"
)

// fileTreeScreen is the migrated ScreenHandler for the installation summary
// (file tree) screen shown before installation starts.
//
// State stays on App: it reads the deep-dive config and install cache through
// s.App(). Pressing enter navigates to the migrated progress screen, which
// triggers the install from its own Init() (the sudo check + startInstallation
// live in progressScreen.Init, so the start does not depend on the ordering of a
// separate installStartMsg relative to the NavigateMsg in a tea.Batch).
type fileTreeScreen struct {
	BaseScreen
}

// NewFileTreeScreen creates a new file tree screen handler.
func NewFileTreeScreen(ctx *ScreenContext) *fileTreeScreen {
	s := &fileTreeScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *fileTreeScreen) ID() Screen { return ScreenFileTree }

// Init triggers the async install-cache load on entry (idempotent).
func (s *fileTreeScreen) Init() tea.Cmd {
	if a := s.App(); a != nil {
		// Manage may arrive with a one-tool plan that has already been reviewed
		// against the current typed installation snapshot. Preserve that exact
		// identity through navigation instead of rebuilding it on screen entry.
		if a.pendingInstallPlan != nil && a.installPlanError == nil {
			return nil
		}
		a.refreshPendingInstallPlan()
		return a.startInstallCacheLoad()
	}
	return nil
}

// Update handles keyboard input for the file tree screen. (Mouse is a no-op,
// matching the legacy handleSummaryMouse for ScreenFileTree.)
func (s *fileTreeScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "ctrl+c", "q":
			return s, tea.Quit
		case "up", "k":
			if a.installPlanScroll > 0 {
				a.installPlanScroll--
			}
		case "down", "j":
			if a.installPlanScroll < int(^uint(0)>>1) {
				a.installPlanScroll++
			}
		case "pgup":
			a.installPlanScroll = maxInt(0, a.installPlanScroll-maxInt(1, s.Height()/2))
		case "pgdown":
			a.installPlanScroll += maxInt(1, s.Height()/2)
		case "home":
			a.installPlanScroll = 0
		case "end":
			a.installPlanScroll = int(^uint(0) >> 1)
		case "enter":
			if a.pendingInstallPlan == nil && a.installPlanError == nil {
				a.refreshPendingInstallPlan()
			}
			if a.installPlanError != nil || a.pendingInstallPlan == nil || a.pendingInstallPlan.hasBlocked() {
				return s, nil
			}
			// Begin installation: navigate to the migrated progress screen. The
			// install is triggered by progressScreen.Init() (which does the sudo
			// check itself), so we deliberately do NOT emit a separate
			// installStartMsg here — its ordering relative to the NavigateMsg in a
			// tea.Batch is not guaranteed, and the message would have no migrated
			// handler until Progress is active.
			return s, NavigateTo(ScreenProgress)
		case "esc":
			a.invalidatePendingInstallPlan()
			a.installReviewTools = nil
			return s, NavigateTo(ScreenNavPicker)
		}
	}
	return s, nil
}

// View renders the installation summary: packages to install, already installed
// tools, and the files that will be modified.
func (s *fileTreeScreen) View(width, height int) string {
	a := s.App()
	if a.installCacheLoading {
		return installStatusLoadingView(a, width, height)
	}

	title := TitleStyle.Render("Reviewed Installation Plan")
	newStyle := lipgloss.NewStyle().Foreground(ColorGreen)
	modStyle := lipgloss.NewStyle().Foreground(ColorYellow)
	blockedStyle := lipgloss.NewStyle().Foreground(ColorRed)
	mutedStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	textStyle := lipgloss.NewStyle().Foreground(ColorText)
	pkgStyle := lipgloss.NewStyle().Foreground(ColorCyan)

	var lines []string
	plan := a.pendingInstallPlan
	switch {
	case a.installPlanError != nil:
		lines = append(lines, blockedStyle.Render("  Plan could not be built:"), "  "+a.installPlanError.Error())
	case plan == nil:
		lines = append(lines, mutedStyle.Render("  Waiting for host observations..."))
	default:
		hash := plan.hash()
		if len(hash) > 12 {
			hash = hash[:12]
		}
		lines = append(lines, mutedStyle.Render("  Plan: "+hash))
		if phase, phased := plan.phase(); phased {
			switch phase.Kind() {
			case operation.InstallPhasePrerequisite:
				lines = append(lines,
					pkgStyle.Render("  Phase 1 — Node/npm prerequisites"),
					modStyle.Render("  This phase stops after prerequisites. A fresh review is required for npm."),
					mutedStyle.Render("  Remaining: "+strings.Join(phase.RemainingTools(), ", ")),
				)
			case operation.InstallPhaseNPM:
				lines = append(lines,
					pkgStyle.Render("  Phase 2 — npm installation"),
					modStyle.Render("  Fresh host and npm authority reviewed. This phase completes the tool install."),
				)
			}
		}
		for _, action := range plan.actions() {
			var style lipgloss.Style
			marker := "●"
			switch action.Disposition {
			case operation.DispositionBlocked:
				style = blockedStyle
				marker = "✗"
			case operation.DispositionSkip:
				style = mutedStyle
				marker = "↷"
			case operation.DispositionApply:
				switch {
				case action.Kind == operation.KindInstallTool:
					style = pkgStyle
				case action.Observation.Exists:
					style = modStyle
				default:
					style = newStyle
				}
			}
			line := fmt.Sprintf("  %s %-14s %s", marker, action.ToolID, action.Description)
			if action.ToolID == "" {
				line = fmt.Sprintf("  %s %-14s %s", marker, "state", action.Description)
			}
			lines = append(lines, style.Render(line))
			if len(action.TargetOwnership) != 0 {
				lines = append(lines, mutedStyle.Render("      ownership: "+string(action.Ownership)))
				for _, target := range orderedTargetOwnership(action) {
					lines = append(lines, mutedStyle.Render(fmt.Sprintf("        %s: %s", target.target, target.ownership)))
				}
			}
			if action.InstallRecipe != nil {
				recipe := action.InstallRecipe
				lines = append(lines, mutedStyle.Render(fmt.Sprintf("      %s on %s · detector %s:%s", recipe.Manager, recipe.Platform, recipe.Detector.Kind, strings.Join(recipe.Detector.Values, ", "))))
				for _, step := range recipe.Steps {
					detail := strings.Join(step.Packages, " ")
					if len(step.Casks) > 0 {
						detail = strings.Join(step.Casks, " ")
					}
					if len(step.Args) > 0 {
						detail = step.Provider + " " + strings.Join(step.Args, " ")
					}
					lines = append(lines, pkgStyle.Render(fmt.Sprintf("        → %s: %s", step.Kind, detail)))
				}
				if recipe.Authentication != "" {
					lines = append(lines, mutedStyle.Render("        Auth: "+recipe.Authentication))
				}
				lines = append(lines, modStyle.Render("        Risk: "+recipe.Risk))
			}
			if action.RemoteArtifact != nil {
				artifact := action.RemoteArtifact.Review()
				lines = append(lines,
					pkgStyle.Render(fmt.Sprintf("      artifact: %s · %s", artifact.Source, artifact.URL)),
					mutedStyle.Render(fmt.Sprintf("        version %s · ref %s", artifact.Version, artifact.ImmutableRef)),
					mutedStyle.Render(fmt.Sprintf("        verify %s:%s · destination %s", artifact.Verification, artifact.Digest, artifact.Destination)),
					modStyle.Render("        Risk: "+string(artifact.Risk)),
				)
			}
			if action.Reason != "" {
				lines = append(lines, blockedStyle.Render("      "+action.Reason))
			}
		}
		if targets := plan.backupTargets(); len(targets) > 0 {
			lines = append(lines, "", textStyle.Render(fmt.Sprintf("  Verified rollback scope: %d target(s)", len(targets))))
		}
	}

	maxLines := maxInt(6, height-12)
	if len(lines) > maxLines {
		// Reserve one row for the scroll status instead of overwriting the last
		// action. The prior viewport made the final plan line unreachable even
		// at maximum scroll, undermining the "review all" promise.
		contentLines := maxInt(1, maxLines-1)
		maxScroll := len(lines) - contentLines
		a.installPlanScroll = clampInt(a.installPlanScroll, 0, maxScroll)
		start := a.installPlanScroll
		end := min(len(lines), start+contentLines)
		visible := append([]string(nil), lines[start:end]...)
		visible = append(visible, mutedStyle.Render(fmt.Sprintf("  Plan lines %d–%d of %d  •  ↑↓/PgUp/PgDn/Home/End scroll", start+1, end, len(lines))))
		lines = visible
	} else {
		a.installPlanScroll = 0
	}

	tree := strings.Join(lines, "\n")

	legend := mutedStyle.Render(
		fmt.Sprintf("  %s New    %s Modified    %s Package    %s Blocked",
			newStyle.Render("●"),
			modStyle.Render("●"),
			pkgStyle.Render("●"),
			blockedStyle.Render("✗"),
		))

	helpText := "[↑↓] Review All    [ENTER] Apply This Exact Plan    [ESC] Back"
	if a.installPlanError != nil || (plan != nil && plan.hasBlocked()) {
		helpText = "Resolve blocked ownership items before applying    [ESC] Back"
	}
	help := HelpStyle.Render(helpText)

	// Prevent the tree from overflowing narrow terminals.
	treeMaxW := maxInt(20, width-6)
	tree = lipgloss.NewStyle().MaxWidth(treeMaxW).Render(tree)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			tree,
			legend,
			"",
			help,
		)),
	)
}
