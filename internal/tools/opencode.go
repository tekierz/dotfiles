package tools

import (
	"fmt"
	"slices"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

// OpenCodeTool represents the OpenCode terminal coding agent.
type OpenCodeTool struct {
	BaseTool
}

func NewOpenCodeTool() *OpenCodeTool {
	return &OpenCodeTool{BaseTool: BaseTool{
		id:          "opencode",
		name:        "OpenCode",
		description: "Open-source coding agent for the terminal",
		icon:        "󰚩",
		category:    CategoryUtility,
		packages: map[pkg.Platform][]string{
			pkg.PlatformMacOS: {"anomalyco/tap/opencode"},
			pkg.PlatformArch:  {"opencode"},
		},
		uiGroup:        UIGroupCLITools,
		defaultEnabled: false,
	}}
}

func (t *OpenCodeTool) IsInstalled() bool { return directInstallationDetected(t) }
func (t *OpenCodeTool) IsInstalledOutsidePackageManager(DirectInstallationObservation) bool {
	return binaryAvailable("opencode")
}

func (t *OpenCodeTool) Install(pkg.PackageManager) error {
	return recipeBackedInstallError(t.ID())
}

func (t *OpenCodeTool) PackageMetadataIsAuthoritative() bool { return false }

func (t *OpenCodeTool) InstallRecipe(environment InstallEnvironment) (operation.InstallRecipe, error) {
	packages := slices.Clone(PackagesForPlatform(t.Packages(), environment.Platform))
	if len(packages) == 0 {
		return operation.InstallRecipe{}, fmt.Errorf("OpenCode has no reviewed package recipe for %s", environment.Platform)
	}
	if environment.Manager == "" {
		return operation.InstallRecipe{}, fmt.Errorf("installing OpenCode requires a package manager")
	}
	if environment.Platform == pkg.PlatformMacOS && environment.Manager != "brew" {
		return operation.InstallRecipe{}, fmt.Errorf("installing OpenCode on macOS requires Homebrew")
	}
	if environment.Platform == pkg.PlatformArch && environment.Manager != "pacman" && environment.Manager != "paru" {
		return operation.InstallRecipe{}, fmt.Errorf("installing OpenCode on Arch requires pacman or paru")
	}
	return operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        t.ID(),
		Platform:      string(environment.Platform),
		Manager:       environment.Manager,
		Steps: []operation.InstallStep{{
			Kind: operation.InstallStepPackageManager, Provider: environment.Manager, Packages: packages,
		}},
		Detector:       operation.InstallDetector{Kind: operation.InstallDetectorBinary, Values: []string{"opencode"}},
		Authentication: "provider login or API key; local providers may require neither",
		Risk:           "resolves the current OpenCode release from the reviewed package-manager source; artifact content is not pinned",
	}, nil
}
