package tools

import (
	"fmt"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
)

const cursorAgentInstallUnavailableReason = "verified artifact support pending"

// CursorAgentTool represents the terminal Cursor Agent CLI. This discovery-only
// integration deliberately exposes no installer until a versioned artifact,
// architecture, checksum, extraction, and ownership contract is reviewed.
type CursorAgentTool struct{ BaseTool }

func NewCursorAgentTool() *CursorAgentTool {
	return &CursorAgentTool{BaseTool: BaseTool{
		id:             "cursor-agent",
		name:           "Cursor Agent",
		description:    "Cursor's coding agent for the terminal",
		icon:           "󰚩",
		category:       CategoryUtility,
		packages:       map[pkg.Platform][]string{},
		uiGroup:        UIGroupCLITools,
		defaultEnabled: false,
	}}
}

func (t *CursorAgentTool) IsInstalled() bool { return false }

// IsInstalledOutsidePackageManager remains fail-closed. An arbitrary PATH
// executable or symlink is not evidence that this dashboard owns or reviewed
// the installed artifact.
func (t *CursorAgentTool) IsInstalledOutsidePackageManager(DirectInstallationObservation) bool {
	return false
}

func (t *CursorAgentTool) InstallationDirectAlternatives(DirectInstallationObservation) []health.DirectAlternative {
	return []health.DirectAlternative{{
		Kind:              health.DirectSourceBinary,
		Identifiers:       []string{"cursor-agent"},
		State:             health.ComponentUnknown,
		DiagnosticCode:    health.DiagnosticProbeFailed,
		DiagnosticSummary: "verified cursor-agent provenance unavailable",
	}}
}

func (t *CursorAgentTool) PackageMetadataIsAuthoritative() bool { return false }

func (t *CursorAgentTool) InstallationUnavailableReason() string {
	return cursorAgentInstallUnavailableReason
}

func (t *CursorAgentTool) Install(pkg.PackageManager) error {
	return fmt.Errorf("%s: %s", t.ID(), t.InstallationUnavailableReason())
}
