package tools

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func TestDescribeInstallAdaptsPackageToolToAcceptedEnvironment(t *testing.T) {
	tool := NewZshTool()
	recipe, err := DescribeInstall(tool, InstallEnvironment{Platform: pkg.PlatformPi, Manager: "apt"})
	if err != nil {
		t.Fatal(err)
	}
	if recipe.ToolID != "zsh" || recipe.Platform != "pi" || recipe.Manager != "apt" || len(recipe.Steps) != 1 {
		t.Fatalf("recipe identity = %#v", recipe)
	}
	step := recipe.Steps[0]
	if step.Kind != operation.InstallStepPackageManager || step.Provider != "apt" || len(step.Packages) == 0 {
		t.Fatalf("package recipe = %#v", step)
	}
	step.Packages[0] = "mutated"
	again, err := DescribeInstall(tool, InstallEnvironment{Platform: pkg.PlatformPi, Manager: "apt"})
	if err != nil || again.Steps[0].Packages[0] == "mutated" {
		t.Fatalf("tool package metadata leaked through recipe: %#v err=%v", again, err)
	}
}

func TestClaudeRecipeDeclaresExactNPMExecution(t *testing.T) {
	recipe, err := DescribeInstall(NewClaudeCodeTool(), InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recipe.Steps) != 2 || len(recipe.Detector.Values) != 1 || recipe.Detector.Values[0] != "claude" {
		t.Fatalf("Claude recipe = %#v", recipe)
	}
	npm := recipe.Steps[1]
	want := []string{"install", "-g", "@anthropic-ai/claude-code"}
	if npm.Kind != operation.InstallStepNPMGlobal || npm.Provider != "npm" || len(npm.Args) != len(want) {
		t.Fatalf("Claude npm step = %#v", npm)
	}
	for index := range want {
		if npm.Args[index] != want[index] {
			t.Fatalf("Claude npm args = %v, want %v", npm.Args, want)
		}
	}
}

func TestDescribeInstallRequiresExplicitManager(t *testing.T) {
	if _, err := DescribeInstall(NewZshTool(), InstallEnvironment{Platform: pkg.PlatformMacOS}); err == nil {
		t.Fatal("package-backed recipe accepted an empty manager")
	}
	if _, err := DescribeInstall(NewClaudeCodeTool(), InstallEnvironment{Platform: pkg.PlatformMacOS}); err == nil {
		t.Fatal("Claude recipe accepted an empty prerequisite manager")
	}
}
