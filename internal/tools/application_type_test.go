package tools

import (
	"regexp"
	"testing"
)

type applicationTypeOverrideTool struct {
	Tool
	typeValue ApplicationType
}

func (tool applicationTypeOverrideTool) ApplicationType() ApplicationType {
	return tool.typeValue
}

func TestApplicationTypeUsesOptionalMetadataAndTotalFallback(t *testing.T) {
	lazyDocker, ok := NewRegistry().Get("lazydocker")
	if !ok {
		t.Fatal("lazydocker fixture missing")
	}
	tests := []struct {
		name string
		tool Tool
		want ApplicationType
	}{
		{name: "optional provider", tool: applicationTypeOverrideTool{Tool: NewFzfTool(), typeValue: ApplicationTypeService}, want: ApplicationTypeService},
		{name: "nil", tool: nil, want: ApplicationTypeUnknown},
		{name: "shell fallback", tool: NewZshTool(), want: ApplicationTypeSystem},
		{name: "terminal fallback", tool: NewGhosttyTool(), want: ApplicationTypeTerminal},
		{name: "editor fallback", tool: NewNeovimTool(), want: ApplicationTypeEditor},
		{name: "file fallback", tool: NewYaziTool(), want: ApplicationTypeFileManager},
		{name: "git fallback", tool: NewLazyGitTool(), want: ApplicationTypeDeveloperTool},
		{name: "container fallback", tool: lazyDocker, want: ApplicationTypeDeveloperTool},
		{name: "utility fallback", tool: NewFzfTool(), want: ApplicationTypeUtility},
		{name: "app fallback", tool: NewIINATool(), want: ApplicationTypeApp},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ApplicationTypeOf(test.tool); got != test.want {
				t.Fatalf("ApplicationTypeOf()=%q, want %q", got, test.want)
			}
		})
	}
}

func TestApplicationTypeExplicitProductClassification(t *testing.T) {
	tests := []struct {
		want  ApplicationType
		tools []Tool
	}{
		{want: ApplicationTypeAIAgent, tools: []Tool{NewClaudeCodeTool(), NewCodexTool(), NewPiTool(), NewOpenCodeTool(), NewT3CodeTool()}},
		{want: ApplicationTypeTerminal, tools: []Tool{NewGhosttyTool(), NewTmuxTool()}},
		{want: ApplicationTypeEditor, tools: []Tool{NewNeovimTool(), NewCursorTool()}},
		{want: ApplicationTypeService, tools: []Tool{NewTailscaleTool(), NewSunshineTool()}},
		{want: ApplicationTypeSystem, tools: []Tool{NewZshTool(), NewBtopTool()}},
		{want: ApplicationTypeUtility, tools: []Tool{NewFzfTool(), NewGlowTool()}},
		{want: ApplicationTypeApp, tools: []Tool{NewIINATool(), NewZenBrowserTool(), NewMoonlightTool()}},
	}
	for _, test := range tests {
		for _, tool := range test.tools {
			t.Run(tool.ID(), func(t *testing.T) {
				if got := ApplicationTypeOf(tool); got != test.want {
					t.Fatalf("ApplicationTypeOf(%s)=%q, want %q", tool.ID(), got, test.want)
				}
				if got := ApplicationTypeToken(ApplicationTypeOf(tool)); got == "" {
					t.Fatalf("ApplicationTypeToken(%q) is empty", ApplicationTypeOf(tool))
				}
			})
		}
	}
}

func TestApplicationTypeCanonicalOrderIsTotalAndUnknownLast(t *testing.T) {
	ordered := []ApplicationType{
		ApplicationTypeSystem,
		ApplicationTypeTerminal,
		ApplicationTypeEditor,
		ApplicationTypeFileManager,
		ApplicationTypeDeveloperTool,
		ApplicationTypeAIAgent,
		ApplicationTypeService,
		ApplicationTypeUtility,
		ApplicationTypeApp,
		ApplicationTypeUnknown,
	}
	previous := -1
	for _, applicationType := range ordered {
		order := ApplicationTypeOrder(applicationType)
		if order <= previous {
			t.Fatalf("order for %q=%d, must follow %d", applicationType, order, previous)
		}
		previous = order
	}
	if got := ApplicationTypeOrder(ApplicationType("future-type")); got != ApplicationTypeOrder(ApplicationTypeUnknown) {
		t.Fatalf("unknown value order=%d, want unknown-last=%d", got, ApplicationTypeOrder(ApplicationTypeUnknown))
	}
}

func TestApplicationTypeTokensAreExactUniqueBoundedASCII(t *testing.T) {
	wants := map[ApplicationType]string{
		ApplicationTypeSystem:        "SYS",
		ApplicationTypeTerminal:      "TERM",
		ApplicationTypeEditor:        "EDIT",
		ApplicationTypeFileManager:   "FILE",
		ApplicationTypeDeveloperTool: "DEV",
		ApplicationTypeAIAgent:       "AI",
		ApplicationTypeService:       "SVC",
		ApplicationTypeUtility:       "UTIL",
		ApplicationTypeApp:           "APP",
		ApplicationTypeUnknown:       "OTHER",
	}
	seen := make(map[string]ApplicationType, len(wants))
	for applicationType, want := range wants {
		got := ApplicationTypeToken(applicationType)
		if got != want {
			t.Errorf("ApplicationTypeToken(%q)=%q, want %q", applicationType, got, want)
		}
		if len(got) == 0 || len(got) > 5 || !regexp.MustCompile(`^[A-Z]+$`).MatchString(got) {
			t.Errorf("token %q is not bounded uppercase ASCII", got)
		}
		if prior, duplicate := seen[got]; duplicate {
			t.Errorf("token %q is shared by %q and %q", got, prior, applicationType)
		}
		seen[got] = applicationType
	}
	if got := ApplicationTypeToken(ApplicationType("future-type")); got != "OTHER" {
		t.Errorf("future type token=%q, want OTHER", got)
	}
}
