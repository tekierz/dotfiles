package tools

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func TestAICLIRecipesDeclareExactReviewedExecution(t *testing.T) {
	tests := []struct {
		name         string
		tool         Tool
		environment  InstallEnvironment
		wantPackages []string
		wantNPMArgs  []string
		wantBinary   string
	}{
		{
			name: "Codex macOS", tool: NewCodexTool(),
			environment:  InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"},
			wantPackages: []string{"node"},
			wantNPMArgs:  []string{"install", "-g", "@openai/codex@0.144.1"},
			wantBinary:   "codex",
		},
		{
			name: "Pi macOS", tool: NewPiTool(),
			environment:  InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"},
			wantPackages: []string{"node"},
			wantNPMArgs:  []string{"install", "-g", "--ignore-scripts", "@earendil-works/pi-coding-agent@0.80.3"},
			wantBinary:   "pi",
		},
		{
			name: "OpenCode macOS", tool: NewOpenCodeTool(),
			environment:  InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"},
			wantPackages: []string{"anomalyco/tap/opencode"},
			wantBinary:   "opencode",
		},
		{
			name: "OpenCode Arch", tool: NewOpenCodeTool(),
			environment:  InstallEnvironment{Platform: pkg.PlatformArch, Manager: "paru"},
			wantPackages: []string{"opencode"},
			wantBinary:   "opencode",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recipe, err := DescribeInstall(test.tool, test.environment)
			if err != nil {
				t.Fatal(err)
			}
			if recipe.ToolID != test.tool.ID() || recipe.Platform != string(test.environment.Platform) || recipe.Manager != test.environment.Manager {
				t.Fatalf("recipe identity = %#v", recipe)
			}
			if recipe.Detector.Kind != operation.InstallDetectorBinary || !reflect.DeepEqual(recipe.Detector.Values, []string{test.wantBinary}) {
				t.Fatalf("detector = %#v", recipe.Detector)
			}
			if recipe.Authentication == "" || recipe.Risk == "" {
				t.Fatalf("recipe omitted auth or risk disclosure: %#v", recipe)
			}
			if len(recipe.Steps) == 0 || recipe.Steps[0].Kind != operation.InstallStepPackageManager || !reflect.DeepEqual(recipe.Steps[0].Packages, test.wantPackages) {
				t.Fatalf("package step = %#v, want %v", recipe.Steps, test.wantPackages)
			}
			if test.wantNPMArgs == nil {
				if len(recipe.Steps) != 1 {
					t.Fatalf("package-only recipe has %d steps", len(recipe.Steps))
				}
			} else if len(recipe.Steps) != 2 || recipe.Steps[1].Kind != operation.InstallStepNPMGlobal || !reflect.DeepEqual(recipe.Steps[1].Args, test.wantNPMArgs) {
				t.Fatalf("npm step = %#v, want %v", recipe.Steps, test.wantNPMArgs)
			}

			// DescribeInstall must return a defensive copy of all execution input.
			recipe.Steps[0].Packages[0] = "mutated"
			again, err := DescribeInstall(test.tool, test.environment)
			if err != nil || again.Steps[0].Packages[0] == "mutated" {
				t.Fatalf("recipe metadata leaked across calls: %#v err=%v", again, err)
			}
		})
	}
}

func TestRecipeBackedAIToolsRejectDirectInstall(t *testing.T) {
	mgr := pkg.NewMockPackageManager()
	for _, tool := range []Tool{NewCodexTool(), NewPiTool(), NewOpenCodeTool()} {
		if err := tool.Install(mgr); !errors.Is(err, ErrReviewedInstallRequired) {
			t.Errorf("%s direct install error = %v", tool.ID(), err)
		}
		if len(mgr.InstallCalls) != 0 {
			t.Fatalf("%s direct install mutated package manager", tool.ID())
		}
		if tool.DefaultEnabled() {
			t.Errorf("%s must remain opt-in", tool.ID())
		}
		if authoritative, ok := tool.(interface{ PackageMetadataIsAuthoritative() bool }); !ok || authoritative.PackageMetadataIsAuthoritative() {
			t.Errorf("%s prerequisite package metadata must not report product installation", tool.ID())
		}
	}
}

func TestAICLIRecipesRejectUnsupportedOrAmbiguousEnvironments(t *testing.T) {
	for _, test := range []struct {
		tool Tool
		env  InstallEnvironment
	}{
		{NewCodexTool(), InstallEnvironment{Platform: pkg.PlatformUnknown, Manager: "brew"}},
		{NewCodexTool(), InstallEnvironment{Platform: pkg.PlatformDebian, Manager: "apt"}},
		{NewPiTool(), InstallEnvironment{Platform: pkg.PlatformMacOS}},
		{NewPiTool(), InstallEnvironment{Platform: pkg.PlatformPi, Manager: "apt"}},
		{NewOpenCodeTool(), InstallEnvironment{Platform: pkg.PlatformDebian, Manager: "apt"}},
		{NewOpenCodeTool(), InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "apt"}},
		{NewOpenCodeTool(), InstallEnvironment{Platform: pkg.PlatformArch, Manager: "brew"}},
	} {
		if _, err := DescribeInstall(test.tool, test.env); err == nil {
			t.Errorf("%s accepted unsupported environment %#v", test.tool.ID(), test.env)
		}
	}
}

func TestAICommandToolsAreRegistered(t *testing.T) {
	registry := NewRegistry()
	for _, id := range []string{"codex", "pi", "opencode"} {
		tool, ok := registry.Get(id)
		if !ok {
			t.Errorf("%s missing from registry", id)
			continue
		}
		if tool.UIGroup() != UIGroupCLITools || tool.DefaultEnabled() {
			t.Errorf("%s UI metadata = group %q default %t", id, tool.UIGroup(), tool.DefaultEnabled())
		}
	}
}

func TestT3CodeDeclaresExactHomebrewCaskRecipe(t *testing.T) {
	tool := NewT3CodeTool()
	recipe, err := DescribeInstall(tool, InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recipe.Steps) != 1 || recipe.Steps[0].Kind != operation.InstallStepHomebrewCask || !reflect.DeepEqual(recipe.Steps[0].Casks, []string{"t3-code"}) {
		t.Fatalf("T3 cask recipe = %#v", recipe)
	}
	if recipe.Detector.Kind != operation.InstallDetectorAppBundle || !reflect.DeepEqual(recipe.Detector.Values, []string{"T3 Code.app"}) {
		t.Fatalf("T3 detector = %#v", recipe.Detector)
	}
	if tool.DefaultEnabled() || tool.HasConfig() {
		t.Fatal("T3 must remain opt-in and install-only")
	}
	if err := tool.Install(pkg.NewMockPackageManager()); !errors.Is(err, ErrReviewedInstallRequired) {
		t.Fatalf("T3 direct install error = %v", err)
	}
	for _, environment := range []InstallEnvironment{{Platform: pkg.PlatformDebian, Manager: "apt"}, {Platform: pkg.PlatformMacOS, Manager: "apt"}} {
		if _, err := DescribeInstall(tool, environment); err == nil {
			t.Fatalf("T3 accepted unsupported environment %#v", environment)
		}
	}
}
