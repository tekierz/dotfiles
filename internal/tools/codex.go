package tools

import (
	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

// CodexTool represents the OpenAI Codex CLI.
type CodexTool struct {
	BaseTool
}

func NewCodexTool() *CodexTool {
	return &CodexTool{BaseTool: BaseTool{
		id:             "codex",
		name:           "Codex",
		description:    "OpenAI coding agent for the terminal",
		icon:           "󰚩",
		category:       CategoryUtility,
		packages:       nodePrerequisitePackages(),
		uiGroup:        UIGroupCLITools,
		defaultEnabled: false,
	}}
}

func (t *CodexTool) IsInstalled() bool { return directInstallationDetected(t) }
func (t *CodexTool) IsInstalledOutsidePackageManager(DirectInstallationObservation) bool {
	return binaryAvailable("codex")
}

func (t *CodexTool) InstallationDirectAlternatives(DirectInstallationObservation) []health.DirectAlternative {
	return recipeBinaryHealth("codex")
}

func (t *CodexTool) Install(pkg.PackageManager) error {
	return recipeBackedInstallError(t.ID())
}

func (t *CodexTool) PackageMetadataIsAuthoritative() bool { return false }

func (t *CodexTool) InstallRecipe(environment InstallEnvironment) (operation.InstallRecipe, error) {
	return npmCLIInstallRecipe(
		t,
		environment,
		[]string{"install", "-g", "@openai/codex@0.144.1"},
		"codex",
		"ChatGPT sign-in or an OpenAI API key",
		"downloads and executes npm package lifecycle code",
	)
}
