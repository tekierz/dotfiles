package tools

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

// ErrReviewedInstallRequired prevents recipe-backed tools from bypassing the
// accepted plan. Their installation must run through the immutable recipe
// executor, which binds the provider, arguments, detector, and risk disclosure.
var ErrReviewedInstallRequired = errors.New("reviewed install recipe required")

func recipeBackedInstallError(toolID string) error {
	return fmt.Errorf("%s: %w", toolID, ErrReviewedInstallRequired)
}

func binaryAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func npmCLIInstallRecipe(
	tool Tool,
	environment InstallEnvironment,
	args []string,
	binary string,
	authentication string,
	risk string,
) (operation.InstallRecipe, error) {
	packages := slices.Clone(PackagesForPlatform(tool.Packages(), environment.Platform))
	if len(packages) == 0 {
		return operation.InstallRecipe{}, fmt.Errorf("%s has no npm prerequisite recipe for %s", tool.ID(), environment.Platform)
	}
	if environment.Manager == "" {
		return operation.InstallRecipe{}, fmt.Errorf("installing %s requires a package manager for Node.js", tool.Name())
	}
	return operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        tool.ID(),
		Platform:      string(environment.Platform),
		Manager:       environment.Manager,
		Steps: []operation.InstallStep{
			{Kind: operation.InstallStepPackageManager, Provider: environment.Manager, Packages: packages},
			{Kind: operation.InstallStepNPMGlobal, Provider: "npm", Args: slices.Clone(args)},
		},
		Detector:       operation.InstallDetector{Kind: operation.InstallDetectorBinary, Values: []string{binary}},
		Authentication: authentication,
		Risk:           risk,
	}, nil
}

func nodePrerequisitePackages() map[pkg.Platform][]string {
	return map[pkg.Platform][]string{
		pkg.PlatformMacOS: {"node"},
	}
}
