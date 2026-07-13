package installplan

import (
	"reflect"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestReviewedPrivateRecipesMapToTruthfulPublicVocabularyAndOrderedSteps(t *testing.T) {
	tests := []struct {
		name     string
		tool     tools.Tool
		wantAuth string
		wantRisk string
	}{
		{name: "ordinary package", tool: tools.NewGitTool(), wantAuth: "none", wantRisk: "package_manager_install"},
		{name: "Claude", tool: tools.NewClaudeCodeTool(), wantAuth: "interactive_provider_login", wantRisk: "npm_lifecycle_code"},
		{name: "Codex", tool: tools.NewCodexTool(), wantAuth: "chatgpt_or_openai_api_key", wantRisk: "npm_lifecycle_code"},
		{name: "Pi", tool: tools.NewPiTool(), wantAuth: "provider_login_or_api_key", wantRisk: "npm_scripts_disabled_runtime_code"},
		{name: "OpenCode", tool: tools.NewOpenCodeTool(), wantAuth: "provider_login_or_api_key", wantRisk: "package_manager_current_release_unpinned"},
		{name: "T3 Code", tool: tools.NewT3CodeTool(), wantAuth: "existing_app_auth", wantRisk: "homebrew_cask_unpinned"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recipe, err := tools.DescribeInstall(test.tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
			if err != nil {
				t.Fatal(err)
			}
			snapshot := mustSnapshot(t, 37, mustObservation(t, test.tool.ID(), health.PackageMissing, recipe))
			counts := &dependencyCounts{}
			deps := countingDependencies(counts, recipe)
			result, err := Build(Request{Intent: mustIntent(t, test.tool.ID()), Snapshot: snapshot, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 37}}, deps)
			if err != nil {
				t.Fatal(err)
			}
			projected := result.Public().Actions()[0].Install
			if projected == nil || projected.Authentication != test.wantAuth || projected.Risk != test.wantRisk {
				t.Fatalf("public auth/risk=%+v, want %q/%q from private %q/%q", projected, test.wantAuth, test.wantRisk, recipe.Authentication, recipe.Risk)
			}
			if projected.RecipeDigest != mustRecipeDigest(t, recipe) || projected.Platform != recipe.Platform || projected.Manager != recipe.Manager || len(projected.Steps) != len(recipe.Steps) {
				t.Fatalf("public recipe identity/order=%+v, private=%+v", projected, recipe)
			}
			for index, privateStep := range recipe.Steps {
				publicStep := projected.Steps[index]
				if publicStep.Kind != publicStepKind(privateStep.Kind) || publicStep.Provider != privateStep.Provider ||
					!reflect.DeepEqual(publicStep.Packages, nonnil(privateStep.Packages)) || !reflect.DeepEqual(publicStep.Casks, nonnil(privateStep.Casks)) || !reflect.DeepEqual(publicStep.Arguments, nonnil(privateStep.Args)) {
					t.Fatalf("step %d public=%+v private=%+v", index, publicStep, privateStep)
				}
			}
		})
	}
}

func publicStepKind(kind operation.InstallStepKind) string {
	switch kind {
	case operation.InstallStepPackageManager:
		return "package_manager"
	case operation.InstallStepNPMGlobal:
		return "npm_global"
	case operation.InstallStepHomebrewCask:
		return "homebrew_cask"
	default:
		return "unknown"
	}
}

func nonnil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
