package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestGenerateLazyGitConfigIsBounded(t *testing.T) {
	got := GenerateLazyGitConfig(LazyGitConfig{SidePanelWidth: "0.5", MouseEvents: false, ColorPreset: "standard", PagerPreset: "builtin"}, "ocean")
	for _, want := range []string{"gui:\n", "sidePanelWidth: 0.5", "mouseEvents: false", "theme:\n", "git:\n  pagers: []"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"git:\n  paging:", "gui.authorLength", "showRandomTip", "ocean"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("generated hidden setting %q in:\n%s", forbidden, got)
		}
	}
	delta := GenerateLazyGitConfig(LazyGitConfig{SidePanelWidth: "0.5", ColorPreset: "standard", PagerPreset: "delta"}, "ocean")
	if !strings.Contains(delta, "git:\n  pagers:\n    - colorArg: always\n      pager: delta --dark --paging=never\n") {
		t.Fatalf("unexpected delta pager:\n%s", delta)
	}
}

func TestParseLazyGitCanonicalizesLeadingDotFraction(t *testing.T) {
	got, err := InspectLazyGitConfigContent("native.yml", []byte("gui:\n  sidePanelWidth: .5\n"), true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.SidePanelWidth != "0.5" {
		t.Fatalf("fraction = %q", got.Config.SidePanelWidth)
	}
	if got.Fields[LazyGitFieldSidePanelWidth].Line != 2 {
		t.Fatalf("provenance = %#v", got.Fields[LazyGitFieldSidePanelWidth])
	}
}

func TestGeneratedLeadingDotFractionRoundTripsAsManaged(t *testing.T) {
	cfg := LazyGitConfig{SidePanelWidth: ".5", MouseEvents: true, ColorPreset: "standard", PagerPreset: "builtin"}
	generated := []byte(GenerateLazyGitConfig(cfg, "theme"))
	if !strings.Contains(string(generated), "sidePanelWidth: 0.5\n") {
		t.Fatalf("generator did not canonicalize fraction:\n%s", generated)
	}
	if !IsManagedLazyGitConfigContent(generated) {
		t.Fatal("generator output was not recognized as managed")
	}
	inspected, err := InspectLazyGitConfigContent("managed.yml", generated, true)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Config.SidePanelWidth != "0.5" || !inspected.Managed {
		t.Fatalf("round trip = %#v", inspected)
	}
}

func TestInspectLazyGitCombinesCustomReasonsAndDisclosesOverrides(t *testing.T) {
	content := []byte("gui:\n  theme:\n    activeBorderColor: [red]\ngit:\n  pagers:\n    - pager: less\n")
	got, err := InspectLazyGitConfigContent("native.yml", content, true)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RepoOverridesPossible {
		t.Fatal("repo override disclosure missing")
	}
	for _, want := range []string{"arbitrary native", "custom LazyGit colors", "custom LazyGit pagers"} {
		if !strings.Contains(got.ReadOnlyReason, want) {
			t.Fatalf("reason %q missing %q", got.ReadOnlyReason, want)
		}
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("nonblocking disclosure must not be a warning: %#v", got.Warnings)
	}
}

func TestManagedLazyGitRecognitionIsExact(t *testing.T) {
	cfg := LazyGitConfig{SidePanelWidth: "0.5", MouseEvents: true, ColorPreset: "standard", PagerPreset: "builtin"}
	exact := []byte(GenerateLazyGitConfig(cfg, "ignored"))
	if !IsManagedLazyGitConfigContent(exact) {
		t.Fatal("exact generated file not recognized")
	}
	inspected, err := InspectLazyGitConfigContent("managed.yml", exact, true)
	if err != nil {
		t.Fatal(err)
	}
	if !inspected.Managed || inspected.Fields[LazyGitFieldSidePanelWidth].Scope != ConfigValueManaged {
		t.Fatalf("managed provenance = %#v", inspected.Fields)
	}
	for name, forged := range map[string][]byte{
		"copied header": append(append([]byte{}, exact...), []byte("showRandomTip: false\n")...),
		"forged body":   []byte(lazyGitManagedHeader + "# Color Preset: standard\n\ngui:\n  sidePanelWidth: 0.5\n  mouseEvents: true\n  theme:\n    activeBorderColor: [red]\ngit:\n  pagers: []\n"),
		"legacy header": []byte(lazyGitManagedHeader + "gui:\n  sidePanelWidth: 0.5\n"),
	} {
		if IsManagedLazyGitConfigContent(forged) || isLegacyManagedLazyGitConfigContent(forged) {
			t.Fatalf("%s granted ownership", name)
		}
	}
}

func TestLazyGitConfigDirsPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CONFIG_DIR", filepath.Join(home, "exact"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	modern, legacy, err := lazyGitConfigDirs(home)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "exact")
	if modern != want || legacy != "" {
		t.Fatalf("CONFIG_DIR got (%q,%q), want exact %q with no legacy fallback", modern, legacy, want)
	}

	t.Setenv("CONFIG_DIR", "")
	modern, legacy, err = lazyGitConfigDirs(home)
	if err != nil {
		t.Fatal(err)
	}
	if modern != filepath.Join(home, "xdg", "lazygit") || legacy != filepath.Join(home, "xdg", "jesseduffield", "lazygit") {
		t.Fatalf("XDG dirs = (%q,%q)", modern, legacy)
	}
}

func TestLazyGitPlatformConfigRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	modern, legacy, err := lazyGitConfigDirsForOS(home, "darwin")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "Library", "Application Support")
	if modern != filepath.Join(root, "lazygit") || legacy != filepath.Join(root, "jesseduffield", "lazygit") {
		t.Fatalf("darwin = (%q,%q)", modern, legacy)
	}
	modern, legacy, err = lazyGitConfigDirsForOS(home, "linux")
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(home, ".config")
	if modern != filepath.Join(root, "lazygit") || legacy != filepath.Join(root, "jesseduffield", "lazygit") {
		t.Fatalf("linux = (%q,%q)", modern, legacy)
	}
}

func TestValidateLazyGitWritableValues(t *testing.T) {
	valid := []string{"0", "0.0", "0.3333", ".5", "0.5", "1", "1.000"}
	for _, width := range valid {
		if err := ValidateLazyGitConfig(LazyGitConfig{SidePanelWidth: width, ColorPreset: "standard", PagerPreset: "builtin"}, "theme"); err != nil {
			t.Errorf("valid width %q: %v", width, err)
		}
	}
	invalid := []string{"", "-0.1", "+.5", "1.01", "2", "1.", ".", "01", "1e-2", " 0.5", "0.5\n", "NaN", "Inf"}
	for _, width := range invalid {
		if err := validateLazyGitFraction(width); err == nil {
			t.Errorf("invalid width %q accepted", width)
		}
	}
	for _, cfg := range []LazyGitConfig{
		{SidePanelWidth: "0.5", ColorPreset: "custom", PagerPreset: "builtin"},
		{SidePanelWidth: "0.5", ColorPreset: "standard", PagerPreset: "custom"},
	} {
		if err := ValidateLazyGitConfig(cfg, "theme"); err == nil {
			t.Fatalf("read-only preset accepted: %#v", cfg)
		}
	}
}

func TestLazyGitDefaultsAndProvenance(t *testing.T) {
	got, err := InspectLazyGitConfigContent("native.yml", []byte("gui:\n  mouseEvents: false\n"), true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.SidePanelWidth != "0.3333" || got.Config.MouseEvents || got.Config.ColorPreset != "standard" || got.Config.PagerPreset != "builtin" {
		t.Fatalf("defaults/config = %#v", got.Config)
	}
	if len(got.Fields) != 1 || got.Fields[LazyGitFieldMouseEvents] != (ConfigFieldProvenance{Path: "native.yml", Line: 2, Key: "gui.mouseEvents", Scope: ConfigValueNative}) {
		t.Fatalf("provenance = %#v", got.Fields)
	}
}

func TestObserveLazyGitModernAndLegacyFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("LG_CONFIG_FILE", "")
	legacy := filepath.Join(home, "xdg", "jesseduffield", "lazygit", "config.yml")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("gui:\n  mouseEvents: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	observed, err := observeLazyGitSources()
	if err != nil {
		t.Fatal(err)
	}
	if observed.target != legacy || len(observed.sources) != 2 || !observed.sources[1].active {
		t.Fatalf("legacy observation = %#v", observed)
	}
	if !strings.Contains(observed.readOnly, "legacy jesseduffield") {
		t.Fatalf("legacy fallback was not explicitly read-only: %#v", observed)
	}
	modern := filepath.Join(home, "xdg", "lazygit", "config.yml")
	if err := os.MkdirAll(filepath.Dir(modern), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modern, []byte("gui:\n  mouseEvents: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	observed, err = observeLazyGitSources()
	if err != nil {
		t.Fatal(err)
	}
	if observed.target != modern || !observed.sources[0].active || observed.sources[1].active {
		t.Fatalf("modern observation = %#v", observed)
	}
}

func TestLazyGitLegacyFallbackNeverGrantsManagedOwnership(t *testing.T) {
	oldGenerated := []byte(`# Generated by dotfiles TUI
# Theme: dark

gui:
  sidePanelWidth: 0.3333
  mouseEvents: false
  theme:
    activeBorderColor: [green, bold]
    inactiveBorderColor: [default]
    selectedLineBgColor: [blue]
    defaultFgColor: [default]
git:
  paging:
    colorArg: always
    pager: delta --dark --paging=never
`)
	currentLooking := []byte(GenerateLazyGitConfig(LazyGitConfig{
		SidePanelWidth: "0.42", MouseEvents: false,
		ColorPreset: "light-high-contrast", PagerPreset: "delta",
	}, "ignored"))
	for _, tc := range []struct {
		name    string
		content []byte
		width   string
		color   string
	}{
		{"old generated bytes", oldGenerated, "0.3333", "standard"},
		{"current generator bytes", currentLooking, "0.42", "light-high-contrast"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("CONFIG_DIR", "")
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
			t.Setenv("LG_CONFIG_FILE", "")
			legacy := filepath.Join(home, "xdg", "jesseduffield", "lazygit", "config.yml")
			if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(legacy, tc.content, 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := ImportLazyGitConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got.Config.SidePanelWidth != tc.width || got.Config.MouseEvents || got.Config.ColorPreset != tc.color || got.Config.PagerPreset != "delta" {
				t.Fatalf("legacy values were not hydrated: %#v", got.Config)
			}
			if got.Managed || len(got.Sources) != 2 || got.Sources[1].Managed || !got.Sources[1].Active {
				t.Fatalf("legacy source was misclassified as managed: %#v", got)
			}
			if source := got.Fields[LazyGitFieldSidePanelWidth]; source.Path != legacy || source.Scope != ConfigValueNative {
				t.Fatalf("legacy provenance = %#v", source)
			}
			if !strings.Contains(got.ReadOnlyReason, "legacy jesseduffield") {
				t.Fatalf("legacy read-only reason = %q", got.ReadOnlyReason)
			}
			if path, err := LazyGitConfigMutationPath(); err == nil || path != "" || !strings.Contains(err.Error(), "legacy jesseduffield") {
				t.Fatalf("legacy mutation path = %q, err=%v", path, err)
			}
		})
	}
}

func TestLazyGitConfigPathBlocksRelativeAndExternal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LG_CONFIG_FILE", "")
	for name, key := range map[string]string{"config": "CONFIG_DIR", "xdg": "XDG_CONFIG_HOME"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("CONFIG_DIR", "")
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv(key, "relative")
			if _, err := LazyGitConfigMutationPath(); err == nil {
				t.Fatal("relative root accepted")
			}
		})
	}
	t.Setenv("CONFIG_DIR", filepath.Join(filepath.Dir(home), "outside"))
	t.Setenv("XDG_CONFIG_HOME", "")
	if _, err := LazyGitConfigMutationPath(); err == nil || !strings.Contains(err.Error(), "outside HOME") {
		t.Fatalf("external override error = %v", err)
	}
}

func TestLazyGitConfigFileChainOrderingAndMerge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	one, two := filepath.Join(home, "one.yml"), filepath.Join(home, "two.yml")
	if err := os.WriteFile(one, []byte("gui:\n  sidePanelWidth: .5\n  theme:\n    activeBorderColor: [green, bold]\n    inactiveBorderColor: [default]\n    selectedLineBgColor: [blue]\n    defaultFgColor: [default]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(two, []byte("gui:\n  mouseEvents: false\ngit:\n  pagers: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LG_CONFIG_FILE", one+","+two)
	got, err := ImportLazyGitConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.SidePanelWidth != "0.5" || got.Config.MouseEvents || got.Config.ColorPreset != "standard" || got.Config.PagerPreset != "builtin" {
		t.Fatalf("merged = %#v", got.Config)
	}
	if len(got.Sources) != 2 || got.Sources[0].Path != one || got.Sources[1].Path != two || !got.Sources[0].Active || !got.Sources[1].Active {
		t.Fatalf("sources = %#v", got.Sources)
	}
	if !strings.Contains(got.ReadOnlyReason, "LG_CONFIG_FILE") {
		t.Fatalf("reason = %q", got.ReadOnlyReason)
	}
}

func TestLazyGitConfigFileCustomReasonsFollowEffectiveMerge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	standard := []byte("gui:\n  theme:\n    activeBorderColor: [green, bold]\n    inactiveBorderColor: [default]\n    selectedLineBgColor: [blue]\n    defaultFgColor: [default]\ngit:\n  pagers: []\n")
	custom := []byte("gui:\n  theme:\n    activeBorderColor: [red]\ngit:\n  pagers:\n    - pager: less\n")
	for _, tc := range []struct {
		name          string
		first, second []byte
		wantCustom    bool
	}{
		{"custom then standard", custom, standard, false},
		{"standard then custom", standard, custom, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, second := filepath.Join(home, strings.ReplaceAll(tc.name, " ", "-")+"-1.yml"), filepath.Join(home, strings.ReplaceAll(tc.name, " ", "-")+"-2.yml")
			if err := os.WriteFile(first, tc.first, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(second, tc.second, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("LG_CONFIG_FILE", first+","+second)
			got, err := ImportLazyGitConfig()
			if err != nil {
				t.Fatal(err)
			}
			hasColor := strings.Contains(got.ReadOnlyReason, "custom LazyGit colors")
			hasPager := strings.Contains(got.ReadOnlyReason, "custom LazyGit pagers")
			if hasColor != tc.wantCustom || hasPager != tc.wantCustom {
				t.Fatalf("effective %#v, reason %q", got.Config, got.ReadOnlyReason)
			}
		})
	}
}

func TestLazyGitConfigFileChainRejectsInvalidEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	file := filepath.Join(home, "one.yml")
	if err := os.WriteFile(file, []byte("gui: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "dir")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, chain := range map[string]string{"relative": "one.yml", "missing": filepath.Join(home, "missing.yml"), "duplicate": file + "," + file, "empty": file + ",", "directory": dir} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("LG_CONFIG_FILE", chain)
			if _, err := observeLazyGitSources(); err == nil {
				t.Fatal("invalid chain accepted")
			}
		})
	}
}

func TestLazyGitThemeAndPagerClassification(t *testing.T) {
	standard := "gui:\n  theme:\n    activeBorderColor: [green, bold]\n    inactiveBorderColor: [default]\n    selectedLineBgColor: [blue]\n    defaultFgColor: [default]\n"
	light := "gui:\n  theme:\n    activeBorderColor: [blue, bold]\n    inactiveBorderColor: [default]\n    selectedLineBgColor: [reverse]\n    defaultFgColor: [black]\n"
	cases := map[string]struct{ yaml, color, pager string }{
		"standard builtin": {standard + "git:\n  pagers: []\n", "standard", "builtin"},
		"light delta":      {light + "git:\n  pagers:\n    - colorArg: always\n      pager: delta --dark --paging=never\n", "light-high-contrast", "delta"},
		"custom colors":    {"gui:\n  theme:\n    activeBorderColor: [red]\n", "custom", "builtin"},
		"custom pager":     {standard + "git:\n  pagers:\n    - pager: less\n", "standard", "custom"},
		"multiple pager":   {standard + "git:\n  pagers:\n    - pager: less\n    - pager: more\n", "standard", "custom"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := InspectLazyGitConfigContent("native.yml", []byte(tc.yaml), true)
			if err != nil {
				t.Fatal(err)
			}
			if got.Config.ColorPreset != tc.color || got.Config.PagerPreset != tc.pager {
				t.Fatalf("got %#v", got.Config)
			}
		})
	}
}

func TestLazyGitWritersRejectDiscoveryDriftAtLockTime(t *testing.T) {
	home := t.TempDir()
	baseXDG := filepath.Join(home, "base")
	cfg := LazyGitConfig{SidePanelWidth: "0.5", MouseEvents: true, ColorPreset: "standard", PagerPreset: "builtin"}
	for _, tc := range []struct {
		name   string
		inject func(t *testing.T)
	}{
		{"config dir", func(t *testing.T) { t.Setenv("CONFIG_DIR", filepath.Join(home, "other")) }},
		{"xdg", func(t *testing.T) { t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "other-xdg")) }},
		{"lg chain", func(t *testing.T) {
			p := filepath.Join(home, "override.yml")
			if err := os.WriteFile(p, []byte("gui: {}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("LG_CONFIG_FILE", p)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("CONFIG_DIR", "")
			t.Setenv("XDG_CONFIG_HOME", baseXDG)
			t.Setenv("LG_CONFIG_FILE", "")
			path, err := LazyGitConfigMutationPath()
			if err != nil {
				t.Fatal(err)
			}
			accepted := readAcceptedFileRevision(t, path)
			parents, err := compatibilityToolConfigParents(path)
			if err != nil {
				t.Fatal(err)
			}
			locker := operation.Locker(func(string, string) (func() error, error) { tc.inject(t); return func() error { return nil }, nil })
			if _, err := writeLazyGitConfigTrackedWithLocker(cfg, "theme", locker); err == nil {
				t.Fatal("direct writer accepted discovery drift")
			}

			t.Setenv("CONFIG_DIR", "")
			t.Setenv("XDG_CONFIG_HOME", baseXDG)
			t.Setenv("LG_CONFIG_FILE", "")
			if _, err := writeLazyGitConfigAtAuthorityTracked(path, cfg, "theme", accepted, parents, locker); err == nil {
				t.Fatal("reviewed writer accepted discovery drift")
			}
		})
	}
}

func TestLazyGitWritersRejectLegacyFallbackAppearingDuringLock(t *testing.T) {
	for _, reviewed := range []bool{false, true} {
		name := "direct"
		if reviewed {
			name = "reviewed"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			xdg := filepath.Join(home, "xdg")
			t.Setenv("HOME", home)
			t.Setenv("CONFIG_DIR", "")
			t.Setenv("XDG_CONFIG_HOME", xdg)
			t.Setenv("LG_CONFIG_FILE", "")
			modern := filepath.Join(xdg, "lazygit", "config.yml")
			legacy := filepath.Join(xdg, "jesseduffield", "lazygit", "config.yml")
			path, err := LazyGitConfigMutationPath()
			if err != nil || path != modern {
				t.Fatalf("initial target=%q err=%v", path, err)
			}
			accepted := readAcceptedFileRevision(t, modern)
			parents, err := compatibilityToolConfigParents(modern)
			if err != nil {
				t.Fatal(err)
			}
			legacyBytes := []byte("gui:\n  mouseEvents: false\ncustom: keep\n")
			locker := operation.Locker(func(string, string) (func() error, error) {
				if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
					return nil, err
				}
				if err := os.WriteFile(legacy, legacyBytes, 0o600); err != nil {
					return nil, err
				}
				return func() error { return nil }, nil
			})
			cfg := LazyGitConfig{SidePanelWidth: "0.5", MouseEvents: true, ColorPreset: "standard", PagerPreset: "builtin"}
			if reviewed {
				_, err = writeLazyGitConfigAtAuthorityTracked(modern, cfg, "theme", accepted, parents, locker)
			} else {
				_, err = writeLazyGitConfigTrackedWithLocker(cfg, "theme", locker)
			}
			if err == nil || !strings.Contains(err.Error(), "legacy jesseduffield") {
				t.Fatalf("writer accepted legacy precedence drift: %v", err)
			}
			if _, statErr := os.Stat(modern); !os.IsNotExist(statErr) {
				t.Fatalf("writer created modern target: %v", statErr)
			}
			got, readErr := os.ReadFile(legacy)
			if readErr != nil || string(got) != string(legacyBytes) {
				t.Fatalf("writer modified injected legacy target: %q err=%v", got, readErr)
			}
		})
	}
}

func TestLazyGitReviewedWriterNoopEvidenceAndParentAuthority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("LG_CONFIG_FILE", "")
	cfg := LazyGitConfig{SidePanelWidth: "0.5", MouseEvents: true, ColorPreset: "standard", PagerPreset: "builtin"}
	first, err := WriteLazyGitConfigTracked(cfg, "theme")
	if err != nil {
		t.Fatal(err)
	}
	parents, err := compatibilityToolConfigParents(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := WriteLazyGitConfigAtAuthorityTracked(cfg, "theme", first.Revision, parents, operation.DefaultLocker)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != first.Path || got.Revision != first.Revision || got.Parents != parents {
		t.Fatalf("no-op evidence = %#v, first = %#v", got, first)
	}
	if _, err := WriteLazyGitConfigAtAuthorityTracked(cfg, "theme", first.Revision, nil, operation.DefaultLocker); !errors.Is(err, safefile.ErrParentChanged) {
		t.Fatalf("missing parent authority error = %v", err)
	}
}

func TestObserveLazyGitDoesNotCreateDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", filepath.Join(home, "absent", "nested"))
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("LG_CONFIG_FILE", "")
	observed, err := observeLazyGitSources()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(observed.target)); !os.IsNotExist(err) {
		t.Fatalf("observation created directory: %v", err)
	}
}

func TestLazyGitYAMLRejectsUnsafeShapes(t *testing.T) {
	cases := map[string][]byte{
		"invalid utf8": {0xff}, "bom": {0xef, 0xbb, 0xbf, 'g'}, "control": {'g', 0x01},
		"del control": {'g', 0x7f}, "bidi control": []byte("gui: \u202e{}\n"),
		"multidoc":    []byte("gui: {}\n---\ngit: {}\n"),
		"duplicate":   []byte("gui: {}\ngui: {}\n"),
		"alias":       []byte("gui: &g {}\nother: *g\n"),
		"merge":       []byte("base: &b {}\ngui:\n  <<: *b\n"),
		"tag":         []byte("gui: !custom {}\n"),
		"wrong gui":   []byte("gui: []\n"),
		"wrong mouse": []byte("gui:\n  mouseEvents: yes\n"),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := InspectLazyGitConfigContent("bad.yml", content, true); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}
