package planpublic

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestPublicPlanHomebrewCaskStepRequiresExactEnvironmentAndShape(t *testing.T) {
	valid := vocabularyInstall("t3-code", "macos", "brew", "existing_app_auth", "homebrew_cask_unpinned", []InstallStepSpec{{
		Kind: "homebrew_cask", Provider: "brew", Packages: []string{}, Casks: []string{"t3-code"}, Arguments: []string{},
	}}, DetectorSpec{Kind: "app_bundle", Values: []string{"T3 Code.app"}})
	if _, err := NewDocument(vocabularyDocument("t3-code", valid)); err != nil {
		t.Fatalf("valid Homebrew cask projection rejected: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*InstallSpec)
	}{
		{name: "non macOS", mutate: func(install *InstallSpec) { install.Platform = "debian" }},
		{name: "non brew manager", mutate: func(install *InstallSpec) { install.Manager = "apt" }},
		{name: "non brew provider", mutate: func(install *InstallSpec) { install.Steps[0].Provider = "custom" }},
		{name: "empty casks", mutate: func(install *InstallSpec) { install.Steps[0].Casks = []string{} }},
		{name: "nil casks", mutate: func(install *InstallSpec) { install.Steps[0].Casks = nil }},
		{name: "packages present", mutate: func(install *InstallSpec) { install.Steps[0].Packages = []string{"t3-code"} }},
		{name: "arguments present", mutate: func(install *InstallSpec) { install.Steps[0].Arguments = []string{"--force"} }},
		{name: "empty cask token", mutate: func(install *InstallSpec) { install.Steps[0].Casks = []string{""} }},
		{name: "unsafe cask token", mutate: func(install *InstallSpec) { install.Steps[0].Casks = []string{"../t3-code"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			install := cloneInstall(valid)
			test.mutate(install)
			if _, err := NewDocument(vocabularyDocument("t3-code", install)); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("invalid Homebrew cask shape accepted: %+v err=%v", install, err)
			}
		})
	}
}

func TestPublicPlanPackageManagerStepRequiresExactManagerAndShape(t *testing.T) {
	valid := vocabularyInstall("codex", "macos", "brew", "none", "package_manager_install", []InstallStepSpec{{
		Kind: "package_manager", Provider: "brew", Packages: []string{"codex"}, Casks: []string{}, Arguments: []string{},
	}}, DetectorSpec{Kind: "package_receipt", Values: []string{"codex"}})
	if _, err := NewDocument(vocabularyDocument("codex", valid)); err != nil {
		t.Fatalf("valid package-manager step rejected: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*InstallSpec)
	}{
		{name: "provider differs from manager", mutate: func(install *InstallSpec) { install.Steps[0].Provider = "apt" }},
		{name: "empty packages", mutate: func(install *InstallSpec) { install.Steps[0].Packages = []string{} }},
		{name: "nil packages", mutate: func(install *InstallSpec) { install.Steps[0].Packages = nil }},
		{name: "casks present", mutate: func(install *InstallSpec) { install.Steps[0].Casks = []string{"codex"} }},
		{name: "arguments present", mutate: func(install *InstallSpec) { install.Steps[0].Arguments = []string{"--force"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			install := cloneInstall(valid)
			test.mutate(install)
			if _, err := NewDocument(vocabularyDocument("codex", install)); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("invalid package-manager shape accepted: %+v err=%v", install.Steps[0], err)
			}
		})
	}
}

func TestPublicPlanNPMGlobalStepRequiresExactReviewedArgumentGrammar(t *testing.T) {
	validSteps := npmSteps("node", "@openai/codex@0.144.1", false)
	valid := vocabularyInstall("codex", "macos", "brew", "chatgpt_or_openai_api_key", "npm_lifecycle_code", validSteps, DetectorSpec{Kind: "binary", Values: []string{"codex"}})
	if _, err := NewDocument(vocabularyDocument("codex", valid)); err != nil {
		t.Fatalf("valid npm-global step rejected: %v", err)
	}
	withIgnoreScripts := cloneInstall(valid)
	withIgnoreScripts.Steps[1].Arguments = []string{"install", "-g", "--ignore-scripts", "@openai/codex@0.144.1"}
	if _, err := NewDocument(vocabularyDocument("codex", withIgnoreScripts)); err != nil {
		t.Fatalf("reviewed --ignore-scripts grammar rejected: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*InstallStepSpec)
	}{
		{name: "wrong provider", mutate: func(step *InstallStepSpec) { step.Provider = "node" }},
		{name: "packages present", mutate: func(step *InstallStepSpec) { step.Packages = []string{"@openai/codex@0.144.1"} }},
		{name: "casks present", mutate: func(step *InstallStepSpec) { step.Casks = []string{"codex"} }},
		{name: "empty arguments", mutate: func(step *InstallStepSpec) { step.Arguments = []string{} }},
		{name: "nil arguments", mutate: func(step *InstallStepSpec) { step.Arguments = nil }},
		{name: "reordered prefix", mutate: func(step *InstallStepSpec) { step.Arguments = []string{"-g", "install", "@openai/codex@0.144.1"} }},
		{name: "package before global", mutate: func(step *InstallStepSpec) { step.Arguments = []string{"install", "@openai/codex@0.144.1", "-g"} }},
		{name: "extra flag", mutate: func(step *InstallStepSpec) {
			step.Arguments = []string{"install", "-g", "--force", "@openai/codex@0.144.1"}
		}},
		{name: "duplicate package", mutate: func(step *InstallStepSpec) {
			step.Arguments = []string{"install", "-g", "@openai/codex@0.144.1", "@openai/codex@0.144.1"}
		}},
		{name: "ignore scripts reordered", mutate: func(step *InstallStepSpec) {
			step.Arguments = []string{"install", "--ignore-scripts", "-g", "@openai/codex@0.144.1"}
		}},
		{name: "missing final package", mutate: func(step *InstallStepSpec) { step.Arguments = []string{"install", "-g"} }},
		{name: "unknown command", mutate: func(step *InstallStepSpec) { step.Arguments = []string{"update", "-g", "@openai/codex@0.144.1"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			install := cloneInstall(valid)
			test.mutate(&install.Steps[1])
			if _, err := NewDocument(vocabularyDocument("codex", install)); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("invalid npm-global grammar accepted: %+v err=%v", install.Steps[1], err)
			}
		})
	}
}

func TestPublicPlanAuthenticationVocabularyIsClosed(t *testing.T) {
	accepted := []string{
		"none",
		"interactive_provider_login",
		"chatgpt_or_openai_api_key",
		"provider_login_or_api_key",
		"existing_app_auth",
	}
	for _, authentication := range accepted {
		t.Run(authentication, func(t *testing.T) {
			install := vocabularyInstall("codex", "macos", "brew", authentication, "package_manager_install", []InstallStepSpec{{
				Kind: "package_manager", Provider: "brew", Packages: []string{"codex"}, Casks: []string{}, Arguments: []string{},
			}}, DetectorSpec{Kind: "package_receipt", Values: []string{"codex"}})
			document, err := NewDocument(vocabularyDocument("codex", install))
			if err != nil {
				t.Fatalf("accepted authentication rejected: %v", err)
			}
			if got := document.Actions()[0].Install.Authentication; got != authentication {
				t.Fatalf("authentication %q projected as %q", authentication, got)
			}
		})
	}
	for _, authentication := range []string{"", "oauth", "api_key", "interactive", "unknown"} {
		install := vocabularyInstall("codex", "macos", "brew", authentication, "package_manager_install", []InstallStepSpec{{
			Kind: "package_manager", Provider: "brew", Packages: []string{"codex"}, Casks: []string{}, Arguments: []string{},
		}}, DetectorSpec{Kind: "package_receipt", Values: []string{"codex"}})
		if _, err := NewDocument(vocabularyDocument("codex", install)); !errors.Is(err, ErrInvalidDocument) {
			t.Fatalf("unknown authentication %q accepted: %v", authentication, err)
		}
	}
}

func TestPublicPlanRiskVocabularyIsClosed(t *testing.T) {
	accepted := []string{
		"package_manager_install",
		"npm_lifecycle_code",
		"npm_scripts_disabled_runtime_code",
		"package_manager_current_release_unpinned",
		"homebrew_cask_unpinned",
	}
	for _, risk := range accepted {
		t.Run(risk, func(t *testing.T) {
			install := vocabularyInstall("codex", "macos", "brew", "none", risk, []InstallStepSpec{{
				Kind: "package_manager", Provider: "brew", Packages: []string{"codex"}, Casks: []string{}, Arguments: []string{},
			}}, DetectorSpec{Kind: "package_receipt", Values: []string{"codex"}})
			document, err := NewDocument(vocabularyDocument("codex", install))
			if err != nil {
				t.Fatalf("accepted risk rejected: %v", err)
			}
			if got := document.Actions()[0].Install.Risk; got != risk {
				t.Fatalf("risk %q projected as %q", risk, got)
			}
		})
	}
	for _, risk := range []string{"", "safe", "curl_pipe_shell", "unknown"} {
		install := vocabularyInstall("codex", "macos", "brew", "none", risk, []InstallStepSpec{{
			Kind: "package_manager", Provider: "brew", Packages: []string{"codex"}, Casks: []string{}, Arguments: []string{},
		}}, DetectorSpec{Kind: "package_receipt", Values: []string{"codex"}})
		if _, err := NewDocument(vocabularyDocument("codex", install)); !errors.Is(err, ErrInvalidDocument) {
			t.Fatalf("unknown risk %q accepted: %v", risk, err)
		}
	}
}

func TestReviewedIntegrationRecipeVocabularyAndStepOrder(t *testing.T) {
	tests := []struct {
		name          string
		toolID        string
		install       *InstallSpec
		wantStepKinds []string
		wantAuth      string
		wantRisk      string
	}{
		{name: "Claude", toolID: "claude-code", wantStepKinds: []string{"package_manager", "npm_global"}, wantAuth: "interactive_provider_login", wantRisk: "npm_lifecycle_code",
			install: vocabularyInstall("claude-code", "macos", "brew", "interactive_provider_login", "npm_lifecycle_code", npmSteps("node", "@anthropic-ai/claude-code@1.0.0", false), DetectorSpec{Kind: "binary", Values: []string{"claude"}})},
		{name: "Codex", toolID: "codex", wantStepKinds: []string{"package_manager", "npm_global"}, wantAuth: "chatgpt_or_openai_api_key", wantRisk: "npm_lifecycle_code",
			install: vocabularyInstall("codex", "macos", "brew", "chatgpt_or_openai_api_key", "npm_lifecycle_code", npmSteps("node", "@openai/codex@0.144.1", false), DetectorSpec{Kind: "binary", Values: []string{"codex"}})},
		{name: "Pi", toolID: "pi", wantStepKinds: []string{"package_manager", "npm_global"}, wantAuth: "provider_login_or_api_key", wantRisk: "npm_scripts_disabled_runtime_code",
			install: vocabularyInstall("pi", "macos", "brew", "provider_login_or_api_key", "npm_scripts_disabled_runtime_code", npmSteps("node", "@earendil-works/pi-coding-agent@0.80.3", true), DetectorSpec{Kind: "binary", Values: []string{"pi"}})},
		{name: "OpenCode", toolID: "opencode", wantStepKinds: []string{"package_manager"}, wantAuth: "provider_login_or_api_key", wantRisk: "package_manager_current_release_unpinned",
			install: vocabularyInstall("opencode", "macos", "brew", "provider_login_or_api_key", "package_manager_current_release_unpinned", []InstallStepSpec{{Kind: "package_manager", Provider: "brew", Packages: []string{"anomalyco/tap/opencode"}, Casks: []string{}, Arguments: []string{}}}, DetectorSpec{Kind: "binary", Values: []string{"opencode"}})},
		{name: "T3 Code", toolID: "t3-code", wantStepKinds: []string{"homebrew_cask"}, wantAuth: "existing_app_auth", wantRisk: "homebrew_cask_unpinned",
			install: vocabularyInstall("t3-code", "macos", "brew", "existing_app_auth", "homebrew_cask_unpinned", []InstallStepSpec{{Kind: "homebrew_cask", Provider: "brew", Packages: []string{}, Casks: []string{"t3-code"}, Arguments: []string{}}}, DetectorSpec{Kind: "app_bundle", Values: []string{"T3 Code.app"}})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, err := NewDocument(vocabularyDocument(test.toolID, test.install))
			if err != nil {
				t.Fatalf("reviewed integration projection rejected: %v", err)
			}
			projected := document.Actions()[0].Install
			var gotKinds []string
			for _, step := range projected.Steps {
				gotKinds = append(gotKinds, step.Kind)
			}
			if !reflect.DeepEqual(gotKinds, test.wantStepKinds) || projected.Authentication != test.wantAuth || projected.Risk != test.wantRisk {
				t.Fatalf("projected recipe kinds/auth/risk=%v/%q/%q", gotKinds, projected.Authentication, projected.Risk)
			}
		})
	}
}

func vocabularyDocument(toolID string, install *InstallSpec) DocumentSpec {
	intent := testExplicitIntent([]string{toolID})
	return DocumentSpec{
		Status: StatusReady, PlanHash: strings.Repeat("1", 64), Platform: "macos", Manager: "brew", Intent: intent,
		Snapshot:     &Snapshot{SchemaVersion: 1, Generation: 1, PublicDigest: strings.Repeat("a", 64)},
		Capabilities: Capabilities{Installation: "planned", Config: "not_planned", Service: "not_collected", Auth: "not_collected", Apply: "not_available"},
		Summary:      Summary{Apply: 1},
		Actions:      []ActionSpec{{ActionID: "install:" + toolID, Kind: "install_tool", ToolID: toolID, Description: "install " + toolID, Disposition: "apply", Ownership: "package_manager", Reversibility: "external", Install: install}},
	}
}

func vocabularyInstall(toolID, platform, manager, authentication, risk string, steps []InstallStepSpec, detector DetectorSpec) *InstallSpec {
	return &InstallSpec{
		SchemaVersion: 1, Platform: platform, Manager: manager, Steps: steps, Detector: detector,
		Authentication: authentication, Risk: risk, RecipeDigest: recipeVocabularyDigest(toolID),
	}
}

func recipeVocabularyDigest(toolID string) string {
	sum := strings.Repeat("a", 64)
	if toolID == "pi" {
		sum = strings.Repeat("b", 64)
	}
	return sum
}

func npmSteps(nodePackage, npmPackage string, ignoreScripts bool) []InstallStepSpec {
	arguments := []string{"install", "-g"}
	if ignoreScripts {
		arguments = append(arguments, "--ignore-scripts")
	}
	arguments = append(arguments, npmPackage)
	return []InstallStepSpec{
		{Kind: "package_manager", Provider: "brew", Packages: []string{nodePackage}, Casks: []string{}, Arguments: []string{}},
		{Kind: "npm_global", Provider: "npm", Packages: []string{}, Casks: []string{}, Arguments: arguments},
	}
}
