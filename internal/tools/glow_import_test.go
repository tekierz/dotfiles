package tools

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestGlowV212SevenSettingsRoundTrip(t *testing.T) {
	cfg := GlowConfig{Style: "auto", Pager: "auto", Width: 0, Mouse: true, All: true, ShowLineNumbers: true, PreserveNewLines: true}
	content := generateGlowManagedSection(cfg)
	for _, want := range []string{`style: "auto"`, "mouse: true", "pager: true", "width: 0", "all: true", "showLineNumbers: true", "preserveNewLines: true"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("generated config missing %q:\n%s", want, content)
		}
	}
	got, err := InspectGlowConfigContent("glow.yml", content, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Config != cfg {
		t.Fatalf("round trip = %#v, want %#v", got.Config, cfg)
	}
}

func TestGlowStyleRejectsControlsAndUsesJSONCompatibleQuoting(t *testing.T) {
	for _, style := range []string{"bad\x1bpath", "bad\tpath", "bad\x7fpath"} {
		cfg := GlowConfig{Style: style, Pager: "never", Width: 80}
		if err := ValidateGlowConfig(cfg, "nord"); err == nil {
			t.Errorf("accepted control style %q", style)
		}
	}
	stylePath := `/tmp/a\b"c.json`
	cfg := GlowConfig{Style: stylePath, Pager: "never", Width: 80}
	content := generateGlowManagedSection(cfg)
	got, err := InspectGlowConfigContent("glow.yml", content, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.Style != cfg.Style {
		t.Fatalf("style roundtrip=%q want %q", got.Config.Style, cfg.Style)
	}
}

func TestGlowPartialNativeUsesV212RuntimeDefaultsAndProvenance(t *testing.T) {
	got, err := InspectGlowConfigContent("glow.yml", []byte("mouse: true\n"), true)
	if err != nil {
		t.Fatal(err)
	}
	want := GlowConfig{Style: "auto", Pager: "never", Width: 0, Mouse: true, All: true}
	if got.Config != want {
		t.Fatalf("config = %#v, want %#v", got.Config, want)
	}
	if got.Fields[GlowFieldMouse].Line != 1 || got.Fields[GlowFieldMouse].Scope != ConfigValueNative {
		t.Fatalf("provenance = %#v", got.Fields)
	}
}

func TestGlowManagedMergePreservesUnknownBytes(t *testing.T) {
	native := []byte("# mine\r\ncustom: 'yes'\r\nstyle: light")
	merged, added, err := mergeGlowManagedSection(native, GlowConfig{Style: "auto", Pager: "never", Width: 80})
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatal("first adoption not reported")
	}
	if !bytesContains(merged, []byte("# mine\r\ncustom: 'yes'\r\n")) {
		t.Fatalf("unknown bytes changed:\n%q", merged)
	}
	second, added, err := mergeGlowManagedSection(merged, GlowConfig{Style: "dark", Pager: "never", Width: 80})
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Fatal("replacement reported adoption")
	}
	if strings.Count(string(second), glowManagedStart) != 1 || !strings.Contains(string(second), `style: "dark"`) {
		t.Fatalf("bad replacement:\n%s", second)
	}
}

func TestTrackedGlowRMWReturnsExactRevisionAndPreservesNativeBytes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GLOW_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	path := filepath.Join(home, ".config", "glow", "glow.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("custom: keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := WriteGlowConfigTracked(GlowConfig{Style: "auto", Pager: "never", Width: 80}, "nord")
	if err != nil {
		t.Fatal(err)
	}
	assertExactMutationEvidence(t, path, first)
	second, err := WriteGlowConfigTracked(GlowConfig{Style: "dark", Pager: "never", Width: 80}, "nord")
	if err != nil {
		t.Fatal(err)
	}
	assertExactMutationEvidence(t, path, second)
	if first.Revision == second.Revision {
		t.Fatal("RMW revision did not change")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), "custom: keep\n") {
		t.Fatalf("native bytes lost:\n%s", content)
	}
}

func TestReviewedGlowWriterRechecksDiscoveryAfterLock(t *testing.T) {
	tests := []struct {
		name   string
		inject func(*testing.T, string)
	}{
		{"higher yaml", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "glow.yaml"), []byte("style: light\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unsupported json", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "glow.json"), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"setting override", func(t *testing.T, _ string) { t.Setenv("GLOW_STYLE", "light") }},
		{"Glamour style override", func(t *testing.T, _ string) { t.Setenv("GLAMOUR_STYLE", "light") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := filepath.Join(home, "glow")
			t.Setenv("GLOW_CONFIG_HOME", dir)
			t.Setenv("XDG_CONFIG_HOME", "")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "glow.yml")
			original := []byte("custom: keep\n")
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			rel, err := filepath.Rel(home, path)
			if err != nil {
				t.Fatal(err)
			}
			_, accepted, parents, err := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
			if err != nil {
				t.Fatal(err)
			}
			locker := operation.Locker(func(string, string) (func() error, error) { tc.inject(t, dir); return func() error { return nil }, nil })
			if _, err := WriteGlowConfigAtAuthorityTracked(GlowConfig{Style: "dark", Pager: "never", Width: 80}, "nord", accepted, parents, locker); err == nil {
				t.Fatal("writer accepted changed discovery")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(original) {
				t.Fatalf("inactive target mutated: %q", got)
			}
		})
	}
}

func TestGlowParserRejectsAmbiguousYAML(t *testing.T) {
	for _, content := range []string{"style: dark\nSTYLE: light\n", "custom: one\nCUSTOM: two\n", `"style": dark` + "\n", "'mouse': true\n", "? key: value\n", "foo # comment: value\n", ",foo: value\n", "]foo: value\n", "foo:bar\n", "custom: \"x\" junk\n", "custom: 'x' junk\n", "custom: [a]\n", "custom: {x: y}\n", "custom: null\n", "custom: ~\n", "custom: - item\n", "custom: ? key\n", "custom: @bad\n", "custom: `bad\n", "custom: one: two\n", " style: dark\n", "style: &x dark\n", "style: |\n dark\n", "style: [dark]\n", "style: {x: y}\n", "style: null\n", "style: ~\n", "style: 'dark' junk\n", "---\nstyle: dark\n", glowManagedStart + "\nstyle: dark\n"} {
		if _, err := InspectGlowConfigContent("glow.yml", []byte(content), true); err == nil {
			t.Errorf("accepted %q", content)
		}
	}
}

func TestGlowParserRejectsInvalidUTF8AndForbiddenControls(t *testing.T) {
	for _, content := range [][]byte{{0xff, '\n'}, {'s', 't', 'y', 'l', 'e', ':', ' ', 0, 'x', '\n'}, {'s', 't', 'y', 'l', 'e', ':', ' ', 0x1f, 'x', '\n'}} {
		if _, err := InspectGlowConfigContent("glow.yml", content, true); err == nil {
			t.Errorf("accepted bytes %x", content)
		}
	}
}

func TestGlowQuotedStyleMayContainYAMLSpecialCharacters(t *testing.T) {
	stylePath := filepath.Join(t.TempDir(), "a!b&c*.json")
	if err := os.WriteFile(stylePath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := InspectGlowConfigContent("glow.yml", []byte("style: "+strconv.Quote(stylePath)+" # safe comment\n"), true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.Style != stylePath {
		t.Fatalf("style=%q", got.Config.Style)
	}
}

func TestGlowV212StyleValidation(t *testing.T) {
	for _, style := range []string{"auto", "ascii", "dark", "dracula", "tokyo-night", "light", "notty", "pink"} {
		if err := ValidateGlowConfig(GlowConfig{Style: style, Pager: "never", Width: 9001}, "nord"); err != nil {
			t.Errorf("builtin %s: %v", style, err)
		}
	}
	for _, style := range []string{"bogus", "/missing/custom.json", " dark "} {
		if err := ValidateGlowConfig(GlowConfig{Style: style, Pager: "never"}, "nord"); err == nil {
			t.Errorf("writable custom/invalid style %q accepted", style)
		}
	}
	custom := "/missing/custom.json"
	got, err := InspectGlowConfigContent("glow.yml", []byte("style: "+strconv.Quote(custom)+"\n"), true)
	if err != nil || got.Config.Style != custom {
		t.Fatalf("read-only native custom=%+v err=%v", got, err)
	}
}

func TestGlowManagedBlockRejectsModeledKeyOutside(t *testing.T) {
	existing := []byte("mouse: true\n\n" + glowManagedStart + "\nstyle: \"auto\"\n" + glowManagedEnd + "\n")
	if _, _, err := mergeGlowManagedSection(existing, GlowConfig{Style: "auto", Pager: "never"}); err == nil {
		t.Fatal("modeled key outside block accepted")
	}
}

func TestGlowLegacyProductOutputMigrationRemovesObsoleteLocal(t *testing.T) {
	legacy := []byte("# Generated by dotfiles TUI\n# Theme: nord\n\nstyle: \"dark\"\npager: true\n\n# Local mode (no cloud)\nlocal: true\nmouse: false\ncustom: keep\n# Local mode (no cloud)\nlocal: true\n")
	merged, added, err := mergeGlowManagedSection(legacy, GlowConfig{Style: "auto", Pager: "never", Width: 80})
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatal("legacy migration not adoption")
	}
	text := string(merged)
	if strings.Count(text, "local: true") != 1 || strings.Contains(text, "Generated by dotfiles") {
		t.Fatalf("product stanza not removed exactly once:\n%s", text)
	}
	if !strings.Contains(text, "custom: keep\n") {
		t.Fatalf("user addition lost:\n%s", text)
	}
}

func TestGlowPathPrecedenceAndUnsupportedActiveFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	xdg := filepath.Join(home, "xdg")
	override := filepath.Join(home, "override")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("GLOW_CONFIG_HOME", override)
	if got, err := GlowConfigMutationPath(); err != nil || got != filepath.Join(override, "glow.yml") {
		t.Fatalf("default path = %q, %v", got, err)
	}
	if err := os.MkdirAll(override, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(override, "glow.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := GlowConfigMutationPath(); err == nil {
		t.Fatal("unsupported higher-priority config did not block")
	}
	t.Setenv("GLOW_CONFIG_HOME", "relative")
	if _, err := GlowConfigMutationPath(); err == nil {
		t.Fatal("relative root did not block")
	}
}

func TestGlowViperExtensionOrderPrefersYAMLOverYML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "glow-home")
	t.Setenv("GLOW_CONFIG_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"glow.yml", "glow.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("style: auto\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := GlowConfigMutationPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "glow.yaml") {
		t.Fatalf("path=%q", got)
	}
}

func TestGlowPathObservationDoesNotCreateDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "missing")
	t.Setenv("GLOW_CONFIG_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	got, err := GlowConfigMutationPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "glow.yml") {
		t.Fatalf("path=%q", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("path observation created directory or unexpected stat: %v", err)
	}
}

func TestGlowSettingEnvironmentOverrideBlocksButConfigHomeDoesNot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GLOW_CONFIG_HOME", filepath.Join(home, "glow"))
	t.Setenv("XDG_CONFIG_HOME", "")
	if _, err := GlowConfigMutationPath(); err != nil {
		t.Fatalf("GLOW_CONFIG_HOME treated as setting override: %v", err)
	}
	for _, env := range []string{"GLOW_STYLE", "GLAMOUR_STYLE"} {
		t.Setenv("GLOW_STYLE", "")
		t.Setenv("GLAMOUR_STYLE", "")
		t.Setenv(env, "dark")
		if _, err := GlowConfigMutationPath(); err == nil || !strings.Contains(err.Error(), env) {
			t.Fatalf("%s override error=%v", env, err)
		}
	}
}

func TestGlowNoFileDoesNotFallThroughFromExternalHighestDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GLOW_CONFIG_HOME", filepath.Join(t.TempDir(), "glow"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	if _, err := GlowConfigMutationPath(); err == nil {
		t.Fatal("external highest no-file directory did not block")
	}
}

func TestImportGlowReportsInactiveExistingCandidates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	high := filepath.Join(home, "high")
	low := filepath.Join(home, "low")
	t.Setenv("GLOW_CONFIG_HOME", high)
	t.Setenv("XDG_CONFIG_HOME", low)
	for _, path := range []string{filepath.Join(high, "glow.yml"), filepath.Join(low, "glow", "glow.yml")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("style: auto\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ImportGlowConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 2 || !got.Sources[0].Active || got.Sources[1].Active {
		t.Fatalf("sources=%#v", got.Sources)
	}
}

func TestImportGlowDoesNotReportCandidateDirectoryAsExistingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "glow")
	t.Setenv("GLOW_CONFIG_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.MkdirAll(filepath.Join(dir, "glow.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ImportGlowConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range got.Sources {
		if source.Path == filepath.Join(dir, "glow.json") && source.Exists {
			t.Fatalf("directory reported as config source: %+v", source)
		}
	}
}

func bytesContains(haystack, needle []byte) bool {
	return strings.Contains(string(haystack), string(needle))
}
