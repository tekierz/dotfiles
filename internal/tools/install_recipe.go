package tools

import (
	"fmt"
	"slices"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

// InstallEnvironment is the host identity accepted during planning. Recipe
// providers must not rediscover either value while describing an install.
type InstallEnvironment struct {
	Platform pkg.Platform
	Manager  string
}

// InstallRecipeProvider is optional so legacy Tool implementations remain
// source-compatible. Recipe-backed tools are executed from the accepted recipe
// and never through their Tool.Install method.
type InstallRecipeProvider interface {
	InstallRecipe(InstallEnvironment) (operation.InstallRecipe, error)
}

// DescribeInstall returns a non-secret, execution-complete recipe. Ordinary
// package-backed tools are adapted to the same contract automatically.
func DescribeInstall(tool Tool, environment InstallEnvironment) (operation.InstallRecipe, error) {
	if provider, ok := tool.(InstallRecipeProvider); ok {
		recipe, err := provider.InstallRecipe(environment)
		if err != nil {
			return operation.InstallRecipe{}, err
		}
		return operation.CloneInstallRecipe(recipe), nil
	}
	packages := slices.Clone(PackagesForPlatform(tool.Packages(), environment.Platform))
	if len(packages) == 0 {
		return operation.InstallRecipe{}, fmt.Errorf("%s has no package recipe for %s", tool.ID(), environment.Platform)
	}
	if environment.Manager == "" {
		return operation.InstallRecipe{}, fmt.Errorf("%s requires a package manager", tool.ID())
	}
	return operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        tool.ID(),
		Platform:      string(environment.Platform),
		Manager:       environment.Manager,
		Steps: []operation.InstallStep{{
			Kind:     operation.InstallStepPackageManager,
			Provider: environment.Manager,
			Packages: packages,
		}},
		Detector: operation.InstallDetector{Kind: operation.InstallDetectorPackageReceipt, Values: slices.Clone(packages)},
		Risk:     "installs packages from the configured system package manager",
	}, nil
}
