package tools

import (
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

// PiTool represents the Pi coding agent CLI.
type PiTool struct {
	BaseTool
}

func NewPiTool() *PiTool {
	return &PiTool{BaseTool: BaseTool{
		id:             "pi",
		name:           "Pi",
		description:    "Minimal, extensible coding agent",
		icon:           "π",
		category:       CategoryUtility,
		packages:       nodePrerequisitePackages(),
		uiGroup:        UIGroupCLITools,
		defaultEnabled: false,
	}}
}

func (t *PiTool) IsInstalled() bool { return binaryAvailable("pi") }

func (t *PiTool) Install(pkg.PackageManager) error {
	return recipeBackedInstallError(t.ID())
}

func (t *PiTool) PackageMetadataIsAuthoritative() bool { return false }

func (t *PiTool) InstallRecipe(environment InstallEnvironment) (operation.InstallRecipe, error) {
	return npmCLIInstallRecipe(
		t,
		environment,
		[]string{"install", "-g", "--ignore-scripts", "@earendil-works/pi-coding-agent@0.80.3"},
		"pi",
		"provider login or API key; local providers may require neither",
		"installs an npm package with lifecycle scripts disabled; package code runs when Pi is launched",
	)
}
