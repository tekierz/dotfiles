package tools

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestImportGitConfigReadsKnownNativeValuesWithProvenance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".gitconfig")
	content := []byte("# user identity and unknown keys stay native\n" +
		"[user]\n\tname = Keep Me\n" +
		"[init]\n\tdefaultBranch = old\n\tdefaultBranch = trunk # last value wins\n" +
		"[pull]\n\trebase = no\n" +
		"[push]\n\tautoSetupRemote\n" +
		"[credential]\n\thelper = cache\n" +
		"[commit]\n\tgpgsign = yes\n" +
		"[merge]\n\ttool = meld\n" +
		"[core]\n\tpager = delta --paging=never\n" +
		"[delta]\n\tside-by-side = on\n" +
		"[alias]\n\tst = status\n\tx-custom = !echo preserve\n" +
		"[credential \"https://example.test\"]\n\thelper = store\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ImportGitConfig()
	if err != nil {
		t.Fatalf("ImportGitConfig: %v", err)
	}
	if got.ManagedInclude {
		t.Fatal("native-only Git config reported a managed include")
	}
	if got.Config.DefaultBranch != "trunk" || got.Config.PullRebase || !got.Config.AutoSetupRemote ||
		got.Config.CredentialHelper != "cache" || !got.Config.SignCommits || got.Config.MergeTool != "meld" ||
		got.Config.DiffTool != "delta" || !got.Config.DeltaSideBySide {
		t.Fatalf("unexpected imported Git config: %+v", got.Config)
	}
	if len(got.Config.Aliases) != 1 || got.Config.Aliases[0] != "st" {
		t.Fatalf("imported aliases = %#v, want [st]", got.Config.Aliases)
	}
	branchSource := got.Fields[GitFieldDefaultBranch]
	if branchSource.Path != path || branchSource.Line != 6 || branchSource.Key != "init.defaultbranch" || branchSource.Scope != ConfigValueNative {
		t.Fatalf("default branch provenance = %+v", branchSource)
	}
	if got.Fields[GitFieldCredentialHelper].Line == 22 {
		t.Fatal("subsection-specific credential helper overrode global helper")
	}
	if len(got.Sources) != 3 || got.Sources[0].Active || !got.Sources[1].Exists || !got.Sources[1].Active || got.Sources[2].Active {
		t.Fatalf("Git sources = %+v", got.Sources)
	}
}

func TestImportGitConfigRespectsManagedIncludePositionAndTrailingNativeOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".gitconfig")
	original := []byte("[init]\n\tdefaultBranch = before\n[user]\n\tname = Keep\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := GitConfig{DefaultBranch: "managed", PullRebase: true, AutoSetupRemote: true, DiffTool: "difftastic", MergeTool: "nvimdiff"}
	if err := WriteGitConfig(cfg, "dracula"); err != nil {
		t.Fatalf("WriteGitConfig: %v", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\n[init]\n\tdefaultBranch = after\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := ImportGitConfig()
	if err != nil {
		t.Fatalf("ImportGitConfig: %v", err)
	}
	if !got.ManagedInclude || got.Config.DefaultBranch != "after" || got.Config.DiffTool != "difftastic" {
		t.Fatalf("managed/trailing Git precedence not reflected: %+v", got)
	}
	if got.Fields[GitFieldDefaultBranch].Scope != ConfigValueNative {
		t.Fatalf("trailing native branch provenance = %+v", got.Fields[GitFieldDefaultBranch])
	}
	if got.Fields[GitFieldDiffTool].Scope != ConfigValueManaged || got.Fields[GitFieldDiffTool].Path != filepath.Join(home, filepath.FromSlash(gitManagedConfigRel)) {
		t.Fatalf("managed diff provenance = %+v", got.Fields[GitFieldDiffTool])
	}
	managedSource := got.Sources[len(got.Sources)-1]
	if !managedSource.Exists || !managedSource.Active || !managedSource.Managed {
		t.Fatalf("managed Git source = %+v", managedSource)
	}
}

func TestImportGitConfigHonorsXDGThenDotGitconfigPrecedence(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	xdgPath := filepath.Join(xdg, "git", "config")
	if err := os.MkdirAll(filepath.Dir(xdgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(xdgPath, []byte("[init]\n\tdefaultBranch = develop\n[pull]\n\trebase = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rootPath := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(rootPath, []byte("[init]\n\tdefaultBranch = main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ImportGitConfig()
	if err != nil {
		t.Fatalf("ImportGitConfig: %v", err)
	}
	if got.Config.DefaultBranch != "main" || got.Config.PullRebase {
		t.Fatalf("effective XDG/root values = %+v", got.Config)
	}
	if got.Fields[GitFieldDefaultBranch].Path != rootPath || got.Fields[GitFieldPullRebase].Path != xdgPath {
		t.Fatalf("XDG/root provenance = %+v", got.Fields)
	}
}

func TestImportGitConfigHonorsAbsoluteGlobalOverrideAndRejectsAmbiguousEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	override := filepath.Join(home, "global.gitconfig")
	if err := os.WriteFile(override, []byte("[init]\n\tdefaultBranch = develop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", override)
	got, err := ImportGitConfig()
	if err != nil || got.Config.DefaultBranch != "develop" || got.Fields[GitFieldDefaultBranch].Path != override {
		t.Fatalf("absolute GIT_CONFIG_GLOBAL import = %+v, %v", got, err)
	}

	t.Setenv("GIT_CONFIG_GLOBAL", "relative/config")
	if _, err := ImportGitConfig(); err == nil {
		t.Fatal("relative GIT_CONFIG_GLOBAL was accepted")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	if _, err := ImportGitConfig(); err == nil {
		t.Fatal("GIT_CONFIG_COUNT command override was ignored")
	}
}

func TestImportGitConfigFailsClosedOnContinuationSyntaxAndMissingManagedSource(t *testing.T) {
	root := []byte("[init]\n\tdefaultBranch = dev\\\nelop\n")
	if _, err := parseGitConfigImport("/home/test/.gitconfig", root, true, "/home/test/managed", nil, false); err == nil {
		t.Fatal("continued Git assignment was accepted")
	}
	root = wrapManagedConfigSection(gitManagedIncludeStart, gitManagedIncludeEnd, "[include]\n\tpath = "+gitManagedIncludePath)
	if _, err := parseGitConfigImport("/home/test/.gitconfig", root, true, "/home/test/managed", nil, false); err == nil {
		t.Fatal("active missing managed Git source was accepted")
	}
}

func TestImportGitConfigRejectsNonGitQuotedEscapes(t *testing.T) {
	root := []byte("[init]\n\tdefaultBranch = \"dev\\x65lop\"\n")
	if _, err := parseGitConfigImport("/home/test/.gitconfig", root, true, "/home/test/managed", nil, false); err == nil {
		t.Fatal("Go-only quoted escape was accepted as Git syntax")
	}
}

func TestImportGitConfigFailsClosedOnModifiedOwnershipBoundaries(t *testing.T) {
	cases := map[string]string{
		"incomplete marker": gitManagedIncludeStart + "\n[include]\n\tpath = " + gitManagedIncludePath + "\n",
		"modified path": string(wrapManagedConfigSection(gitManagedIncludeStart, gitManagedIncludeEnd,
			"[include]\n\tpath = ~/.config/other/config")),
		"extra assignment": string(wrapManagedConfigSection(gitManagedIncludeStart, gitManagedIncludeEnd,
			"[include]\n\tpath = "+gitManagedIncludePath+"\n[core]\n\teditor = injected")),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ImportGitConfig(); err == nil {
				t.Fatal("ImportGitConfig accepted modified ownership boundary")
			}
		})
	}
}

func TestImportGitConfigFailsClosedOnUnfollowedNativeIncludesWithoutLeakingValues(t *testing.T) {
	root := []byte("[include]\n\tpath = ~/.config/git/private-work-config\n" +
		"[includeIf \"gitdir:~/secret-client/\"]\n\tpath = ~/secret-client/config\n")
	_, err := parseGitConfigImport("/home/test/.gitconfig", root, true, "/home/test/managed", nil, false)
	if err == nil {
		t.Fatal("parseGitConfigImport accepted unresolved native include sources")
	}
	joined := err.Error()
	for _, secret := range []string{"private-work-config", "secret-client"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("Git include warning leaked %q: %s", secret, joined)
		}
	}
}

func TestImportGitConfigRefusesActiveUnownedManagedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	root := wrapManagedConfigSection(gitManagedIncludeStart, gitManagedIncludeEnd, "[include]\n\tpath = "+gitManagedIncludePath)
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), root, 0o600); err != nil {
		t.Fatal(err)
	}
	managedPath := filepath.Join(home, filepath.FromSlash(gitManagedConfigRel))
	if err := os.MkdirAll(filepath.Dir(managedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managedPath, []byte("[init]\n\tdefaultBranch = stolen\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ImportGitConfig()
	if !errors.Is(err, ErrUnmanagedConfig) {
		t.Fatalf("ImportGitConfig error = %v, want ErrUnmanagedConfig", err)
	}
}

func TestImportGhosttyConfigReadsKnownNativeValuesWithProvenance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "ghostty", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("# comments and unknown keys are ignored, not rewritten\n" +
		"font-family = First Font\nfont-family = Last Font\n" +
		"font-size = 18\nbackground-opacity = 0.87\nbackground-blur = 22\n" +
		"cursor-style = bar\nscrollback-limit = 123456\n" +
		"window-decoration = false\nconfirm-close-surface = yes\n" +
		"keybind = ctrl+shift+t=new_tab\nkeybind = ctrl+shift+w=close_surface\n" +
		"future-setting = preserve-me\nfont-size = 18.5\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ImportGhosttyConfig()
	if err != nil {
		t.Fatalf("ImportGhosttyConfig: %v", err)
	}
	if got.Managed || got.Config.FontFamily != "First Font" || got.Config.FontSize != 0 || got.Config.Opacity != 87 ||
		got.Config.BlurRadius != 22 || got.Config.CursorStyle != "bar" || got.Config.ScrollbackLines != 123456 ||
		got.Config.WindowDecorations || !got.Config.ConfirmClose || got.Config.TabBindings != "ctrl-shift" {
		t.Fatalf("unexpected imported Ghostty config: %+v", got.Config)
	}
	if _, ok := got.Fields[GhosttyFieldFontSize]; ok {
		t.Fatal("unrepresentable fractional font size was imported")
	}
	fontSource := got.Fields[GhosttyFieldFontFamily]
	if fontSource.Path != path || fontSource.Line != 2 || fontSource.Key != "font-family" || fontSource.Scope != ConfigValueNative {
		t.Fatalf("font family provenance = %+v", fontSource)
	}
	activeSources := 0
	for _, source := range got.Sources {
		if source.Active {
			activeSources++
			if source.Path != path || !source.Exists || source.Managed {
				t.Fatalf("unexpected active Ghostty source: %+v", source)
			}
		}
	}
	if activeSources != 1 {
		t.Fatalf("Ghostty sources = %+v", got.Sources)
	}
}

func TestImportGhosttyConfigRespectsManagedSectionPositionAndTrailingNativeOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "ghostty", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("font-size = 11\nfuture-setting = keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := ownershipGhosttyConfig()
	if err := WriteGhosttyConfig(cfg, "dracula"); err != nil {
		t.Fatalf("WriteGhosttyConfig: %v", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\nfont-size = 20\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := ImportGhosttyConfig()
	if err != nil {
		t.Fatalf("ImportGhosttyConfig: %v", err)
	}
	if !got.Managed || got.Config.FontSize != 20 || got.Config.Opacity != cfg.Opacity {
		t.Fatalf("managed/trailing Ghostty precedence not reflected: %+v", got)
	}
	if got.Fields[GhosttyFieldFontSize].Scope != ConfigValueNative || got.Fields[GhosttyFieldOpacity].Scope != ConfigValueManaged {
		t.Fatalf("Ghostty provenance did not follow section order: font=%+v opacity=%+v", got.Fields[GhosttyFieldFontSize], got.Fields[GhosttyFieldOpacity])
	}
}

func TestGhosttyMacOSApplicationSupportSourceOverridesXDGAndReceivesManagedBlock(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Ghostty Application Support precedence is macOS-specific")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	xdgPath := filepath.Join(home, ".config", "ghostty", "config")
	appPath := filepath.Join(home, "Library", "Application Support", "com.mitchellh.ghostty", "config")
	if err := os.MkdirAll(filepath.Dir(xdgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(appPath), 0o700); err != nil {
		t.Fatal(err)
	}
	xdgOriginal := []byte("font-size = 12\nxdg-only-setting = keep\n")
	appOriginal := []byte("font-size = 21\napp-only-setting = keep\n")
	if err := os.WriteFile(xdgPath, xdgOriginal, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appPath, appOriginal, 0o600); err != nil {
		t.Fatal(err)
	}

	imported, err := ImportGhosttyConfig()
	if err != nil {
		t.Fatalf("ImportGhosttyConfig: %v", err)
	}
	if imported.Config.FontSize != 21 || imported.Fields[GhosttyFieldFontSize].Path != appPath {
		t.Fatalf("Application Support did not override XDG: config=%+v source=%+v", imported.Config, imported.Fields[GhosttyFieldFontSize])
	}
	if len(imported.Sources) != 4 || imported.Sources[0].Active || !imported.Sources[1].Active || imported.Sources[2].Active || !imported.Sources[3].Active {
		t.Fatalf("Ghostty source chain = %+v", imported.Sources)
	}
	mutationPath, err := GhosttyConfigMutationPath()
	if err != nil {
		t.Fatalf("GhosttyConfigMutationPath: %v", err)
	}
	if mutationPath != appPath {
		t.Fatalf("GhosttyConfigMutationPath() = %q, want active Application Support source %q", mutationPath, appPath)
	}

	cfg := ownershipGhosttyConfig()
	if err := WriteGhosttyConfig(cfg, "dracula"); err != nil {
		t.Fatalf("WriteGhosttyConfig: %v", err)
	}
	if got := mustReadOwnershipFile(t, xdgPath); !bytes.Equal(got, xdgOriginal) {
		t.Fatalf("lower-precedence XDG source changed:\n%s", got)
	}
	appUpdated := mustReadOwnershipFile(t, appPath)
	if !bytes.HasPrefix(appUpdated, appOriginal) || !bytes.Contains(appUpdated, []byte(ghosttyManagedStart)) {
		t.Fatalf("active Application Support source did not receive preserved managed block:\n%s", appUpdated)
	}
	if _, err := os.Stat(appPath + ".dotfiles.bak"); !os.IsNotExist(err) {
		t.Fatalf("Application Support adoption created an unplanned sidecar: %v", err)
	}
}

func TestGhosttyConfigCandidatesHonorAbsoluteXDGRoot(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	paths := ghosttyConfigCandidates(home)
	if paths[0] != filepath.Join(xdg, "ghostty", "config.ghostty") {
		t.Fatalf("first Ghostty source = %q, want external XDG path", paths[0])
	}
	if runtime.GOOS == "darwin" {
		if len(paths) != 4 || !strings.Contains(paths[2], filepath.Join("Library", "Application Support", "com.mitchellh.ghostty")) {
			t.Fatalf("macOS Ghostty source chain = %#v", paths)
		}
	} else if len(paths) != 2 {
		t.Fatalf("non-macOS Ghostty source chain = %#v", paths)
	}
}

func TestImportGhosttyConfigFailsClosedOnAmbiguousMarkers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "ghostty", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := ghosttyManagedStart + "\nfont-size = 12\n" + ghosttyManagedStart + "\n" + ghosttyManagedEnd + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportGhosttyConfig(); err == nil {
		t.Fatal("ImportGhosttyConfig accepted ambiguous markers")
	}
	if got := mustReadOwnershipFile(t, path); string(got) != content {
		t.Fatalf("read-only import changed malformed Ghostty config: %s", got)
	}
}

func TestImportGhosttyConfigRejectsConfigFileAndIgnoresCaseMismatches(t *testing.T) {
	if _, err := parseGhosttyConfigImport("/home/test/ghostty", []byte("config-file = private.conf\n"), true); err == nil {
		t.Fatal("Ghostty config-file source was accepted")
	}
	got, err := parseGhosttyConfigImport("/home/test/ghostty", []byte("Font-Size = 31\nfont-size = 17\nwindow-decoration =\n"), true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.FontSize != 17 || got.Fields[GhosttyFieldWindowDecorations].Key != "" {
		t.Fatalf("case/empty Ghostty semantics imported incorrectly: %+v", got)
	}
}

func TestGitImportOnlyAdoptsCanonicalManagedAliases(t *testing.T) {
	content := []byte("[alias]\n\tst = !echo custom\n\tco = checkout\n\tlg = log --oneline --graph --decorate\n")
	got, err := parseGitConfigImport("/home/test/.gitconfig", content, true, "/home/test/managed", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Config.Aliases, ",") != "co,lg" {
		t.Fatalf("aliases = %#v, want only canonical co,lg", got.Config.Aliases)
	}
}

func TestNativeImportsDoNotCreateMissingConfigParents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	gitImport, err := ImportGitConfig()
	if err != nil {
		t.Fatalf("ImportGitConfig: %v", err)
	}
	ghosttyImport, err := ImportGhosttyConfig()
	if err != nil {
		t.Fatalf("ImportGhosttyConfig: %v", err)
	}
	if gitImport.Sources[0].Exists || ghosttyImport.Sources[0].Exists {
		t.Fatalf("missing sources reported as existing: git=%+v ghostty=%+v", gitImport.Sources, ghosttyImport.Sources)
	}
	assertOwnershipPathAbsent(t, filepath.Join(home, ".config"))
}

func TestNativeImportsRefuseSymlinkedSources(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink import checks are Unix-specific")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	victim := filepath.Join(home, "victim")
	original := []byte("[init]\n\tdefaultBranch = victim\n")
	if err := os.WriteFile(victim, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(home, ".gitconfig")); err != nil {
		t.Fatal(err)
	}

	_, err := ImportGitConfig()
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("ImportGitConfig error = %v, want safefile.ErrSymlink", err)
	}
	if got := mustReadOwnershipFile(t, victim); !bytes.Equal(got, original) {
		t.Fatalf("read-only import changed symlink victim: %q", got)
	}
}

func TestGitAndGhosttyImportWarningsDoNotContainUnknownValues(t *testing.T) {
	// Warnings identify only path/line and parse class. This guards against
	// accidentally surfacing secrets from unknown native settings in the UI.
	_, gitWarnings := parseGitAssignments([]byte("[credential]\nmalformed secret-token-value\n"), "/home/test/.gitconfig", ConfigValueNative, 1)
	ghostty, err := parseGhosttyConfigImport("/home/test/ghostty", []byte("malformed secret-token-value\n"), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range append(gitWarnings, ghostty.Warnings...) {
		if strings.Contains(warning, "secret-token-value") {
			t.Fatalf("warning exposed native value: %q", warning)
		}
	}
}
