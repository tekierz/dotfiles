package tools

import (
	"fmt"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

// T3CodeTool is the T3 Code desktop agent frontend. It is a GUI app, not a
// terminal command, and installation is bound to an explicit Homebrew cask.
type T3CodeTool struct{ BaseTool }

func NewT3CodeTool() *T3CodeTool {
	return &T3CodeTool{BaseTool: BaseTool{
		id: "t3-code", name: "T3 Code", description: "Desktop frontend for coding agents",
		icon: "󰚩", category: CategoryApp, packages: map[pkg.Platform][]string{},
		uiGroup: UIGroupMacApps, defaultEnabled: false, platformFilter: pkg.PlatformMacOS,
	}}
}

func (t *T3CodeTool) IsInstalled() bool { return hasMacOSApp("T3 Code") }

func (t *T3CodeTool) Install(pkg.PackageManager) error { return recipeBackedInstallError(t.ID()) }

func (t *T3CodeTool) InstallerAvailable(platform pkg.Platform) bool {
	return platform == pkg.PlatformMacOS
}

func (t *T3CodeTool) InstallRecipe(environment InstallEnvironment) (operation.InstallRecipe, error) {
	if environment.Platform != pkg.PlatformMacOS || environment.Manager != "brew" {
		return operation.InstallRecipe{}, fmt.Errorf("installing T3 Code requires Homebrew on macOS")
	}
	return operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        t.ID(), Platform: string(environment.Platform), Manager: environment.Manager,
		Steps:          []operation.InstallStep{{Kind: operation.InstallStepHomebrewCask, Provider: "brew", Casks: []string{"t3-code"}}},
		Detector:       operation.InstallDetector{Kind: operation.InstallDetectorAppBundle, Values: []string{"T3 Code.app"}},
		Authentication: "uses existing coding-agent authentication; T3 Code stores no dashboard-managed credentials",
		Risk:           "installs the current t3-code Homebrew cask; artifact content is not pinned",
	}, nil
}
