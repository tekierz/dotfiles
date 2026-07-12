package tools

import (
	"fmt"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
)

const hermesInstallUnavailableReason = "verified artifact and architecture support pending"

// HermesAgentTool is a discovery-only representation of the Hermes agent.
// Installation remains unavailable until artifact and architecture authority is
// represented by the reviewed installation-plan contract.
type HermesAgentTool struct{ BaseTool }

func NewHermesAgentTool() *HermesAgentTool {
	return &HermesAgentTool{BaseTool: BaseTool{
		id:             "hermes",
		name:           "Hermes Agent",
		description:    "Autonomous AI agent runtime by Nous Research (discovery only)",
		icon:           "󰚩",
		category:       CategoryUtility,
		packages:       map[pkg.Platform][]string{},
		uiGroup:        UIGroupCLITools,
		defaultEnabled: false,
	}}
}

func (t *HermesAgentTool) IsInstalled() bool { return false }

func (t *HermesAgentTool) InstallationDirectAlternatives(DirectInstallationObservation) []health.DirectAlternative {
	return []health.DirectAlternative{{
		Kind:              health.DirectSourceBinary,
		Identifiers:       []string{"hermes"},
		State:             health.ComponentUnknown,
		DiagnosticCode:    health.DiagnosticProbeFailed,
		DiagnosticSummary: "verified hermes provenance unavailable",
	}}
}

func (t *HermesAgentTool) PackageMetadataIsAuthoritative() bool { return false }

func (t *HermesAgentTool) InstallationUnavailableReason() string {
	return hermesInstallUnavailableReason
}

func (t *HermesAgentTool) installUnavailableError() error {
	return fmt.Errorf("%s: %s", t.ID(), t.InstallationUnavailableReason())
}

func (t *HermesAgentTool) Install(pkg.PackageManager) error {
	return t.installUnavailableError()
}

func (t *HermesAgentTool) InstallForPlatform(pkg.PackageManager, pkg.Platform) error {
	return t.installUnavailableError()
}
