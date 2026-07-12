package tools

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestYaziConfigPathsDefaultToHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("YAZI_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	got, err := ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(home, ".config", "yazi")
	if got.Origin != YaziConfigOriginDefault || got.Dir != wantDir || got.Main != filepath.Join(wantDir, "yazi.toml") || got.Keymap != filepath.Join(wantDir, "keymap.toml") || got.Theme != filepath.Join(wantDir, "theme.toml") {
		t.Fatalf("unexpected Yazi paths: %#v", got)
	}
}

func TestYaziConfigPathsHonorAbsoluteOverrideAsExactDirectory(t *testing.T) {
	override := filepath.Join(t.TempDir(), "custom", "..", "yazi")
	t.Setenv("HOME", "")
	t.Setenv("YAZI_CONFIG_HOME", override)
	t.Setenv("XDG_CONFIG_HOME", "relative/ignored")

	got, err := ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Clean(override)
	if got.Origin != YaziConfigOriginOverride || got.Dir != wantDir || got.Main != filepath.Join(wantDir, "yazi.toml") || got.Keymap != filepath.Join(wantDir, "keymap.toml") || got.Theme != filepath.Join(wantDir, "theme.toml") {
		t.Fatalf("override must be the exact config directory: %#v", got)
	}
}

func TestYaziConfigPathsHonorAbsoluteXDGParent(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), "xdg", "..", "config")
	t.Setenv("HOME", "")
	t.Setenv("YAZI_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got, err := ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(filepath.Clean(xdg), "yazi")
	if got.Origin != YaziConfigOriginXDG || got.Dir != wantDir || got.Main != filepath.Join(wantDir, "yazi.toml") || got.Keymap != filepath.Join(wantDir, "keymap.toml") || got.Theme != filepath.Join(wantDir, "theme.toml") {
		t.Fatalf("XDG_CONFIG_HOME must be treated as a parent directory: %#v", got)
	}
}

func TestYaziConfigPathsRejectRelativeOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("YAZI_CONFIG_HOME", "relative/yazi")
	if _, err := ResolveYaziConfigPaths(); err == nil {
		t.Fatal("expected relative YAZI_CONFIG_HOME to fail closed")
	}
}

func TestYaziConfigPathsRejectRelativeXDG(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("YAZI_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	if _, err := ResolveYaziConfigPaths(); err == nil {
		t.Fatal("expected relative XDG_CONFIG_HOME to fail closed")
	}
}

func TestYaziConfigPathsRejectUnsafeXDG(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("YAZI_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/config\u202e")
	if _, err := ResolveYaziConfigPaths(); err == nil {
		t.Fatal("expected bidi control in XDG_CONFIG_HOME to fail closed")
	}
}

func TestYaziConfigPathsRejectRelativeHome(t *testing.T) {
	t.Setenv("HOME", "relative-home")
	t.Setenv("YAZI_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if _, err := ResolveYaziConfigPaths(); err == nil {
		t.Fatal("expected relative HOME to fail closed")
	}
}

func TestYaziConfigPathsRejectMissingHome(t *testing.T) {
	t.Setenv("YAZI_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if _, err := ResolveYaziConfigPaths(); err == nil {
		t.Fatal("expected missing HOME to fail closed")
	}
}

func TestYaziConfigPathsRejectControlCharacters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("YAZI_CONFIG_HOME", "/tmp/yazi\nother")
	if _, err := ResolveYaziConfigPaths(); err == nil {
		t.Fatal("expected control characters to fail closed")
	}
}

func TestYaziConfigPathsRejectUnsafeHome(t *testing.T) {
	t.Setenv("HOME", "/tmp/home\u202e")
	t.Setenv("YAZI_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if _, err := ResolveYaziConfigPaths(); err == nil {
		t.Fatal("expected bidi control in HOME to fail closed")
	}
}

func TestImportYaziConfigKeepsValidSiblingsWhenOneFileIsMalformed(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "yazi")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("YAZI_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.WriteFile(filepath.Join(dir, "yazi.toml"), []byte("[mgr]\nshow_hidden = true\nscrolloff = 9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keymap.toml"), []byte("[mgr\nkeymap = []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "theme.toml"), []byte("[flavor]\ndark = \"catppuccin-mocha\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Config.ShowHidden || got.Config.ScrollOff != 9 {
		t.Fatalf("valid main observations were discarded: %#v", got.Config)
	}
	if got.Main.Ownership != YaziOwnershipNative || got.Keymap.Ownership != YaziOwnershipMalformed || got.Keymap.Error == "" || got.Theme.Ownership != YaziOwnershipNative {
		t.Fatalf("independent file states not retained: main=%#v keymap=%#v theme=%#v", got.Main, got.Keymap, got.Theme)
	}
	if got.Theme.ReadOnlyReason != "native Yazi flavor selection is read-only in this release" {
		t.Fatalf("unexpected flavor classification: %q", got.Theme.ReadOnlyReason)
	}
	if got.Fields[YaziFieldShowHidden].Line != 2 || got.Fields[YaziFieldScrollOff].Line != 3 {
		t.Fatalf("missing field provenance: %#v", got.Fields)
	}
}

func TestImportYaziConfigReportsIndependentMissingFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("YAZI_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, observation := range []YaziFileObservation{got.Main, got.Keymap, got.Theme} {
		if observation.Exists || observation.Ownership != YaziOwnershipMissing || observation.ReadOnlyReason != "" {
			t.Fatalf("unexpected missing observation: %#v", observation)
		}
	}
}

func TestImportYaziPreviewClassificationRequiresBothPluginArrays(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    string
	}{
		{"both empty", "[plugin]\npreviewers = []\npreloaders = []\n", "never"},
		{"previewers only", "[plugin]\npreviewers = []\n", "custom"},
		{"preloaders only", "[plugin]\npreloaders = []\n", "custom"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := prepareYaziImportDir(t)
			writeYaziTestFile(t, filepath.Join(dir, "yazi.toml"), test.content)
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got.Config.PreviewMode != test.want {
				t.Fatalf("preview mode = %q, want %q", got.Config.PreviewMode, test.want)
			}
		})
	}
}

func TestImportYaziPreviewClassificationAutoAndAlways(t *testing.T) {
	for name, test := range map[string]struct {
		content string
		want    string
	}{
		"auto":   {"[preview]\nimage_delay = 30\n", "auto"},
		"always": {"[preview]\nimage_delay = 0\n", "always"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := prepareYaziImportDir(t)
			writeYaziTestFile(t, filepath.Join(dir, YaziFileMain), test.content)
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got.Config.PreviewMode != test.want || got.Main.Ownership != YaziOwnershipNative || got.Main.ReadOnlyReason == "" {
				t.Fatalf("preview classification = %q, observation %#v; want %q native read-only", got.Config.PreviewMode, got.Main, test.want)
			}
		})
	}
}

func TestImportYaziPreviewClassificationTreatsOverridesAsCustomReadOnly(t *testing.T) {
	tests := map[string]string{
		"previewers only":     "[plugin]\npreviewers = []\n",
		"preloaders only":     "[plugin]\npreloaders = []\n",
		"nonempty previewers": "[plugin]\npreviewers = [{ mime = \"*\", run = \"noop\" }]\npreloaders = []\n",
		"nonempty preloaders": "[plugin]\npreviewers = []\npreloaders = [{ mime = \"*\", run = \"noop\" }]\n",
		"prepend previewers":  "[plugin]\nprepend_previewers = [{ mime = \"*\", run = \"noop\" }]\n",
		"append previewers":   "[plugin]\nappend_previewers = [{ mime = \"*\", run = \"noop\" }]\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := prepareYaziImportDir(t)
			writeYaziTestFile(t, filepath.Join(dir, YaziFileMain), content)
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got.Config.PreviewMode != "custom" || got.Main.Ownership != YaziOwnershipNative || got.Main.ReadOnlyReason == "" {
				t.Fatalf("plugin override = mode %q, observation %#v; want custom native read-only", got.Config.PreviewMode, got.Main)
			}
		})
	}
}

func TestImportYaziExternalOverrideHydratesButStaysReadOnly(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("YAZI_CONFIG_HOME", external)
	t.Setenv("XDG_CONFIG_HOME", "")
	writeYaziTestFile(t, filepath.Join(external, "yazi.toml"), "[mgr]\nshow_hidden = true\nscrolloff = 17\n")
	writeYaziTestFile(t, filepath.Join(external, "keymap.toml"), "[mgr]\nkeymap = [{ on = [\"g\", \"g\"], run = \"arrow -99999999\" }]\n")

	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Main.External || !got.Config.ShowHidden || got.Config.ScrollOff != 17 || got.Main.ReadOnlyReason == "" {
		t.Fatalf("external native values must hydrate read-only: observation=%#v config=%#v", got.Main, got.Config)
	}
	if !got.Keymap.External || got.Config.Keymap != "custom" || got.Keymap.Ownership != YaziOwnershipNative || got.Keymap.ReadOnlyReason == "" {
		t.Fatalf("external native keymap must hydrate read-only: observation=%#v config=%#v", got.Keymap, got.Config)
	}
}

func TestImportYaziExternalOverrideRejectsFinalSymlink(t *testing.T) {
	external := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("YAZI_CONFIG_HOME", external)
	t.Setenv("XDG_CONFIG_HOME", "")
	target := filepath.Join(t.TempDir(), "native.toml")
	writeYaziTestFile(t, target, "[mgr]\nshow_hidden = true\n")
	if err := os.Symlink(target, filepath.Join(external, YaziFileMain)); err != nil {
		t.Fatal(err)
	}
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Main.Exists || !got.Main.External || got.Main.Ownership != YaziOwnershipMalformed || got.Main.Error == "" || got.Config.ShowHidden {
		t.Fatalf("external symlink was followed or incompletely reported: observation=%#v config=%#v", got.Main, got.Config)
	}
}

func TestImportYaziRejectsInHomeAncestorAndFinalSymlinks(t *testing.T) {
	for _, leaf := range []bool{false, true} {
		name := "ancestor"
		if leaf {
			name = "final"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			external := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("YAZI_CONFIG_HOME", "")
			t.Setenv("XDG_CONFIG_HOME", "")
			var target, link string
			if leaf {
				dir := filepath.Join(home, ".config", "yazi")
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				target = filepath.Join(external, YaziFileMain)
				link = filepath.Join(dir, YaziFileMain)
			} else {
				if err := os.Mkdir(filepath.Join(home, ".config"), 0o700); err != nil {
					t.Fatal(err)
				}
				target = external
				link = filepath.Join(home, ".config", "yazi")
			}
			if leaf {
				writeYaziTestFile(t, target, "[mgr]\nshow_hidden = true\n")
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got.Main.Ownership != YaziOwnershipMalformed || got.Main.Error == "" || got.Config.ShowHidden {
				t.Fatalf("%s symlink was not rejected: observation=%#v config=%#v", name, got.Main, got.Config)
			}
		})
	}
}

func TestImportYaziRejectsHardlinkedConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardlink policy is exercised on Unix")
	}
	dir := prepareYaziImportDir(t)
	original := filepath.Join(t.TempDir(), "shared.toml")
	writeYaziTestFile(t, original, "[mgr]\nshow_hidden = true\n")
	if err := os.Link(original, filepath.Join(dir, YaziFileMain)); err != nil {
		t.Fatal(err)
	}
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Main.Exists || got.Main.Ownership != YaziOwnershipMalformed || got.Main.Error == "" || got.Config.ShowHidden {
		t.Fatalf("hardlinked config was not rejected: observation=%#v config=%#v", got.Main, got.Config)
	}
}

func TestImportYaziRejectsRecognizedSectionWrongTypes(t *testing.T) {
	for _, test := range []struct {
		file    string
		content string
		state   func(YaziConfigImport) YaziFileObservation
	}{
		{"yazi.toml", "mgr = 1\n", func(v YaziConfigImport) YaziFileObservation { return v.Main }},
		{"yazi.toml", "preview = []\n", func(v YaziConfigImport) YaziFileObservation { return v.Main }},
		{"yazi.toml", "plugin = \"x\"\n", func(v YaziConfigImport) YaziFileObservation { return v.Main }},
		{"keymap.toml", "mgr = false\n", func(v YaziConfigImport) YaziFileObservation { return v.Keymap }},
		{"theme.toml", "flavor = \"x\"\n", func(v YaziConfigImport) YaziFileObservation { return v.Theme }},
		{"theme.toml", "mgr = false\n", func(v YaziConfigImport) YaziFileObservation { return v.Theme }},
		{"theme.toml", "filetype = []\n", func(v YaziConfigImport) YaziFileObservation { return v.Theme }},
		{"keymap.toml", "[mgr]\nkeymap = false\n", func(v YaziConfigImport) YaziFileObservation { return v.Keymap }},
		{"keymap.toml", "[mgr]\nprepend_keymap = false\n", func(v YaziConfigImport) YaziFileObservation { return v.Keymap }},
		{"keymap.toml", "[mgr]\nappend_keymap = \"bad\"\n", func(v YaziConfigImport) YaziFileObservation { return v.Keymap }},
	} {
		t.Run(test.file+test.content, func(t *testing.T) {
			dir := prepareYaziImportDir(t)
			writeYaziTestFile(t, filepath.Join(dir, test.file), test.content)
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if observed := test.state(got); observed.Ownership != YaziOwnershipMalformed || observed.Error == "" {
				t.Fatalf("recognized wrong-type section accepted: %#v", observed)
			}
		})
	}
}

func TestImportYaziUnrelatedPathsStayNativeAndPreserveModeledValues(t *testing.T) {
	dir := prepareYaziImportDir(t)
	content := "[mgr]\nshow_hidden = true\n\n[preview]\nshow_hidden = false\n\n[plugin]\nimage_delay = 0\n\n[extension]\ncustom = true\n"
	writeYaziTestFile(t, filepath.Join(dir, YaziFileMain), content)
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Config.ShowHidden || got.Main.Ownership != YaziOwnershipNative || got.Main.ReadOnlyReason == "" || got.Main.Error != "" {
		t.Fatalf("unrelated native paths did not preserve modeled hydration: observation=%#v config=%#v", got.Main, got.Config)
	}
}

func TestImportYaziValidatesKeymapEntryAndOnTypes(t *testing.T) {
	tests := map[string]string{
		"scalar entry": "[mgr]\nkeymap = [1]\n",
		"scalar on":    "[mgr]\nkeymap = [{ on = 1, run = \"noop\" }]\n",
		"mixed on":     "[mgr]\nkeymap = [{ on = [\"g\", 1], run = \"noop\" }]\n",
		"scalar run":   "[mgr]\nkeymap = [{ on = \"g\", run = 1 }]\n",
		"array desc":   "[mgr]\nkeymap = [{ on = \"g\", run = \"noop\", desc = [] }]\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := prepareYaziImportDir(t)
			writeYaziTestFile(t, filepath.Join(dir, YaziFileKeymap), content)
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got.Keymap.Ownership != YaziOwnershipMalformed || got.Keymap.Error == "" {
				t.Fatalf("invalid keymap entry accepted: %#v", got.Keymap)
			}
		})
	}
}

func TestImportYaziAcceptsMultiKeyBindingAsCustomReadOnly(t *testing.T) {
	for name, content := range map[string]string{
		"keymap":         "[mgr]\nkeymap = [{ on = [\"g\", \"g\"], run = \"arrow -99999999\" }]\n",
		"prepend_keymap": "[mgr]\nprepend_keymap = [{ on = [\"g\", \"g\"], run = \"arrow -99999999\" }]\n",
		"append_keymap":  "[mgr]\nappend_keymap = [{ on = \"x\", run = \"noop\" }]\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := prepareYaziImportDir(t)
			writeYaziTestFile(t, filepath.Join(dir, YaziFileKeymap), content)
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got.Config.Keymap != "custom" || got.Keymap.Ownership != YaziOwnershipNative || got.Keymap.ReadOnlyReason == "" || got.Keymap.Error != "" {
				t.Fatalf("valid multi-key binding not retained as custom native: observation=%#v config=%#v", got.Keymap, got.Config)
			}
		})
	}
}

func TestImportYaziEmptyAndUnrelatedKeymapLayersRetainVimDefault(t *testing.T) {
	for name, content := range map[string]string{
		"empty":          "",
		"unrelated only": "[tasks]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := prepareYaziImportDir(t)
			writeYaziTestFile(t, filepath.Join(dir, YaziFileKeymap), content)
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got.Config.Keymap != "vim" || got.Keymap.Ownership != YaziOwnershipNative || got.Keymap.ReadOnlyReason == "" || got.Keymap.Error != "" {
				t.Fatalf("unmodeled keymap layer changed defaults: observation=%#v config=%#v", got.Keymap, got.Config)
			}
			if _, exists := got.Fields[YaziFieldKeymap]; exists {
				t.Fatalf("unmodeled keymap layer emitted provenance: %#v", got.Fields[YaziFieldKeymap])
			}
		})
	}
}

func TestImportYaziNativeLegacyManagerMainIsIgnoredByV26(t *testing.T) {
	dir := prepareYaziImportDir(t)
	writeYaziTestFile(t, filepath.Join(dir, YaziFileMain), "[manager]\nshow_hidden = true\n")
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.ShowHidden || got.Main.Ownership != YaziOwnershipNative || got.Main.ReadOnlyReason == "" || got.Main.Error != "" {
		t.Fatalf("native legacy manager table affected v26 config: observation=%#v config=%#v", got.Main, got.Config)
	}
	if _, exists := got.Fields[YaziFieldShowHidden]; exists {
		t.Fatalf("ignored legacy manager value emitted provenance: %#v", got.Fields[YaziFieldShowHidden])
	}
}

func TestImportYaziModifiedHistoricalKeymapIsNativeAndIgnoredByV26(t *testing.T) {
	dir := prepareYaziImportDir(t)
	cfg := YaziConfig{Keymap: "emacs"}
	content := generateHistoricalYaziKeymap(cfg) + "# user modification\n"
	writeYaziTestFile(t, filepath.Join(dir, YaziFileKeymap), content)
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.Keymap != "vim" || got.Keymap.Ownership != YaziOwnershipNative || got.Keymap.ReadOnlyReason == "" || got.Keymap.Error != "" {
		t.Fatalf("modified historical keymap affected v26 config: observation=%#v config=%#v", got.Keymap, got.Config)
	}
	if _, exists := got.Fields[YaziFieldKeymap]; exists {
		t.Fatalf("ignored modified historical keymap emitted provenance: %#v", got.Fields[YaziFieldKeymap])
	}
}

func TestImportYaziAcceptsStringArrayRunAndRejectsMixedRun(t *testing.T) {
	for name, test := range map[string]struct {
		content   string
		malformed bool
	}{
		"string array": {"[mgr]\nkeymap = [{ on = \"g\", run = [\"noop\", \"arrow 1\"] }]\n", false},
		"mixed array":  {"[mgr]\nkeymap = [{ on = \"g\", run = [\"noop\", 1] }]\n", true},
		"scalar":       {"[mgr]\nkeymap = [{ on = \"g\", run = 1 }]\n", true},
	} {
		t.Run(name, func(t *testing.T) {
			dir := prepareYaziImportDir(t)
			writeYaziTestFile(t, filepath.Join(dir, YaziFileKeymap), test.content)
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if test.malformed {
				if got.Keymap.Ownership != YaziOwnershipMalformed || got.Keymap.Error == "" {
					t.Fatalf("invalid run accepted: %#v", got.Keymap)
				}
				return
			}
			if got.Keymap.Ownership != YaziOwnershipNative || got.Keymap.Error != "" || got.Keymap.ReadOnlyReason == "" {
				t.Fatalf("valid run string array rejected: %#v", got.Keymap)
			}
		})
	}
}

func TestImportYaziPreloaderMixingOverridesAreTypedAndCustom(t *testing.T) {
	for _, key := range []string{"prepend_preloaders", "append_preloaders"} {
		t.Run(key+" valid", func(t *testing.T) {
			dir := prepareYaziImportDir(t)
			content := "[plugin]\npreviewers = []\npreloaders = []\n" + key + " = []\n"
			writeYaziTestFile(t, filepath.Join(dir, YaziFileMain), content)
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got.Config.PreviewMode != "custom" || got.Main.Ownership != YaziOwnershipNative || got.Main.Error != "" {
				t.Fatalf("mixing override did not force custom: observation=%#v config=%#v", got.Main, got.Config)
			}
		})
		t.Run(key+" wrong type", func(t *testing.T) {
			dir := prepareYaziImportDir(t)
			writeYaziTestFile(t, filepath.Join(dir, YaziFileMain), "[plugin]\n"+key+" = false\n")
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got.Main.Ownership != YaziOwnershipMalformed || got.Main.Error == "" {
				t.Fatalf("wrong-type mixing override accepted: %#v", got.Main)
			}
		})
	}
}

func TestImportYaziCurrentVimProvidesSyntheticManagedProvenance(t *testing.T) {
	dir := prepareYaziImportDir(t)
	content := "# Generated by dotfiles TUI\n# Compatibility: Yazi 26.5.6\n# Keymap style: vim\n"
	path := filepath.Join(dir, YaziFileKeymap)
	writeYaziTestFile(t, path, content)
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	field := got.Fields[YaziFieldKeymap]
	if got.Keymap.Ownership != YaziOwnershipExactCurrent || got.Config.Keymap != "vim" || field.Path != path || field.Line != 3 || field.Key != "header.keymap-style" || field.Scope != ConfigValueManaged {
		t.Fatalf("synthetic Vim provenance mismatch: observation=%#v config=%#v field=%#v", got.Keymap, got.Config, field)
	}
}

func TestImportYaziCustomPreviewProvenanceUsesActualFirstOverride(t *testing.T) {
	dir := prepareYaziImportDir(t)
	path := filepath.Join(dir, YaziFileMain)
	writeYaziTestFile(t, path, "[plugin]\nappend_preloaders = []\nprepend_previewers = []\n")
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	field := got.Fields[YaziFieldPreviewMode]
	if got.Config.PreviewMode != "custom" || field.Path != path || field.Line != 2 || field.Key != "plugin.append_preloaders" || field.Scope != ConfigValueNative {
		t.Fatalf("custom preview provenance mismatch: config=%#v field=%#v", got.Config, field)
	}
}

func TestImportYaziRecognizesFrozenPreBB6DE48Files(t *testing.T) {
	dir := prepareYaziImportDir(t)
	cfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "modified", SortReverse: true, LineMode: "permissions", ScrollOff: 9}
	fixtures := map[string]struct {
		content string
		hash    string
	}{
		"yazi.toml":   {generateHistoricalYaziMain(cfg, "nord"), "31d4c5fbc1b3f60a1e9aefff39cba0cdd6d88e009ec0aaf39ce059c05d0653d9"},
		"keymap.toml": {generateHistoricalYaziKeymap(cfg), "c8d0d1c7d838b01e2e57a4825d9233a3b67c9f9321a8cf32192ff2bf90ec9de6"},
		"theme.toml":  {generateHistoricalYaziTheme("nord"), "21bc3944746767582d44022c6d810105aa70c649121d7e131ece786bc3cfffaa"},
	}
	for name, fixture := range fixtures {
		actual := fmt.Sprintf("%x", sha256.Sum256([]byte(fixture.content)))
		if actual != fixture.hash {
			t.Fatalf("frozen %s bytes drifted: sha256=%s", name, actual)
		}
		writeYaziTestFile(t, filepath.Join(dir, name), fixture.content)
	}
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Main.Ownership != YaziOwnershipExactHistorical || got.Keymap.Ownership != YaziOwnershipExactHistorical || got.Theme.Ownership != YaziOwnershipExactHistorical {
		t.Fatalf("historical ownership not recognized: main=%#v keymap=%#v theme=%#v", got.Main, got.Keymap, got.Theme)
	}
	if got.Config.PreviewMode != "never" || got.Config.Keymap != "emacs" || !got.Config.ShowHidden || got.Config.ScrollOff != 9 {
		t.Fatalf("historical values not hydrated: %#v", got.Config)
	}
	for fieldID, wantKey := range map[string]string{
		YaziFieldShowHidden: "manager.show_hidden",
		YaziFieldLineMode:   "manager.linemode",
	} {
		field, exists := got.Fields[fieldID]
		if !exists || field.Path != filepath.Join(dir, YaziFileMain) || field.Line == 0 || field.Key != wantKey || field.Scope != ConfigValueManaged {
			t.Fatalf("historical main provenance %s = %#v, exists=%t; want path/line/%s/managed", fieldID, field, exists, wantKey)
		}
	}
	keymapField, exists := got.Fields[YaziFieldKeymap]
	if !exists || keymapField.Path != filepath.Join(dir, YaziFileKeymap) || keymapField.Line == 0 || keymapField.Key != "manager.keymap" || keymapField.Scope != ConfigValueManaged {
		t.Fatalf("historical keymap provenance = %#v, exists=%t; want actual manager.keymap source", keymapField, exists)
	}
}

func TestImportYaziRecognizesDefaultValuedHistoricalFilesWithProvenance(t *testing.T) {
	dir := prepareYaziImportDir(t)
	cfg := defaultImportedYaziConfig()
	cfg.Keymap = "vim"
	writeYaziTestFile(t, filepath.Join(dir, YaziFileMain), generateHistoricalYaziMain(cfg, "nord"))
	writeYaziTestFile(t, filepath.Join(dir, YaziFileKeymap), generateHistoricalYaziKeymap(cfg))
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Main.Ownership != YaziOwnershipExactHistorical || got.Keymap.Ownership != YaziOwnershipExactHistorical || got.Config.Keymap != "vim" || got.Config.LineMode != "none" {
		t.Fatalf("default historical files not recognized: main=%#v keymap=%#v config=%#v", got.Main, got.Keymap, got.Config)
	}
	for fieldID, wantKey := range map[string]string{
		YaziFieldShowHidden: "manager.show_hidden",
		YaziFieldLineMode:   "manager.linemode",
		YaziFieldKeymap:     "manager.keymap",
	} {
		field, exists := got.Fields[fieldID]
		if !exists || field.Line == 0 || field.Key != wantKey || field.Scope != ConfigValueManaged {
			t.Fatalf("default historical provenance %s = %#v, exists=%t", fieldID, field, exists)
		}
	}
}

func TestImportYaziRecognizesCurrentCanonicalFiles(t *testing.T) {
	dir := prepareYaziImportDir(t)
	main := "# Generated by dotfiles TUI\n# Compatibility: Yazi 26.5.6\n\n[mgr]\nsort_by = \"natural\"\nsort_reverse = false\nlinemode = \"none\"\nscrolloff = 5\nshow_hidden = true\n\n[preview]\nimage_delay = 30\n\n[plugin]\npreviewers = []\npreloaders = []\n"
	keymap := "# Generated by dotfiles TUI\n# Compatibility: Yazi 26.5.6\n# Keymap style: emacs\n\n[mgr]\nprepend_keymap = [\n  { on = \"<C-p>\", run = \"arrow -1\", desc = \"Move up\" },\n  { on = \"<C-n>\", run = \"arrow 1\", desc = \"Move down\" },\n  { on = \"<C-b>\", run = \"leave\", desc = \"Go to parent\" },\n  { on = \"<C-f>\", run = \"enter\", desc = \"Enter directory\" },\n  { on = \"<Space>\", run = \"toggle\", desc = \"Toggle selection\" },\n]\n"
	oldTheme := GenerateYaziTheme("nord")
	const oldThemeHeader = "# Generated by dotfiles TUI\n"
	if !strings.HasPrefix(oldTheme, oldThemeHeader) {
		t.Fatalf("old theme fixture lacks expected frozen header: %q", oldTheme)
	}
	theme := oldThemeHeader + "# Compatibility: Yazi 26.5.6\n" + strings.TrimPrefix(oldTheme, oldThemeHeader)
	for name, fixture := range map[string]struct{ content, hash string }{
		YaziFileMain:   {main, "aeec6c7f395a16d7212c27bcdfc0d3f1e69845843183c37ac2af55060b931f0e"},
		YaziFileKeymap: {keymap, "0a2fd17b29dedb66a5af0cb976e9c1bfd16e036a57336d4447f1cc0f1e84c9de"},
		YaziFileTheme:  {theme, "846959713f83ef8750d1b69f51f8f29ee71a433ca9b01d3473e21fff61bfa656"},
	} {
		if actual := fmt.Sprintf("%x", sha256.Sum256([]byte(fixture.content))); actual != fixture.hash {
			t.Fatalf("frozen current %s bytes drifted: sha256=%s", name, actual)
		}
	}
	if strings.Contains(keymap, "select --state=none") || !strings.Contains(keymap, `run = "toggle"`) {
		t.Fatalf("canonical Emacs keymap must use current non-destructive toggle action: %q", keymap)
	}
	writeYaziTestFile(t, filepath.Join(dir, YaziFileMain), main)
	writeYaziTestFile(t, filepath.Join(dir, YaziFileKeymap), keymap)
	writeYaziTestFile(t, filepath.Join(dir, YaziFileTheme), theme)
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Main.Ownership != YaziOwnershipExactCurrent || got.Keymap.Ownership != YaziOwnershipExactCurrent || got.Theme.Ownership != YaziOwnershipExactCurrent {
		t.Fatalf("canonical ownership not recognized: main=%#v keymap=%#v theme=%#v", got.Main, got.Keymap, got.Theme)
	}
	if got.Config.PreviewMode != "never" || got.Config.Keymap != "emacs" || got.Config.LineMode != "none" {
		t.Fatalf("canonical values not hydrated: %#v", got.Config)
	}
}

func TestImportYaziRecognizesCurrentCanonicalVimAsNonDestructive(t *testing.T) {
	dir := prepareYaziImportDir(t)
	keymap := "# Generated by dotfiles TUI\n# Compatibility: Yazi 26.5.6\n# Keymap style: vim\n"
	if strings.Contains(keymap, "keymap =") || strings.Contains(keymap, "select --state=none") {
		t.Fatalf("canonical Vim bytes must rely on Yazi defaults without replacing the full keymap: %q", keymap)
	}
	writeYaziTestFile(t, filepath.Join(dir, YaziFileKeymap), keymap)
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Keymap.Ownership != YaziOwnershipExactCurrent || got.Config.Keymap != "vim" {
		t.Fatalf("canonical Vim ownership not recognized: observation=%#v config=%#v", got.Keymap, got.Config)
	}
}

func TestImportYaziRecognizesFrozenCurrentOldFullFormatsAsHistorical(t *testing.T) {
	dir := prepareYaziImportDir(t)
	cfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "modified", SortReverse: true, LineMode: "permissions", ScrollOff: 9}
	writeYaziTestFile(t, filepath.Join(dir, YaziFileMain), GenerateYaziConfig(cfg, "nord"))
	writeYaziTestFile(t, filepath.Join(dir, YaziFileKeymap), GenerateYaziKeymap(cfg, "nord"))
	writeYaziTestFile(t, filepath.Join(dir, YaziFileTheme), GenerateYaziTheme("nord"))
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Main.Ownership != YaziOwnershipExactHistorical || got.Keymap.Ownership != YaziOwnershipExactHistorical || got.Theme.Ownership != YaziOwnershipExactHistorical {
		t.Fatalf("current-old formats not frozen as historical: main=%#v keymap=%#v theme=%#v", got.Main, got.Keymap, got.Theme)
	}
	if got.Config.PreviewMode != "never" || got.Config.Keymap != "emacs" || !got.Config.ShowHidden || got.Config.LineMode != "permissions" {
		t.Fatalf("current-old values not hydrated: %#v", got.Config)
	}
}

func TestImportYaziGeneratedHeaderWithExtraBytesIsNativeReadOnly(t *testing.T) {
	tests := map[string]string{
		YaziFileMain:   GenerateYaziConfig(YaziConfig{Keymap: "vim", PreviewMode: "auto", SortBy: "natural", LineMode: "none", ScrollOff: 5}, "nord"),
		YaziFileKeymap: GenerateYaziKeymap(YaziConfig{Keymap: "vim"}, "nord"),
		YaziFileTheme:  GenerateYaziTheme("nord"),
	}
	for name, canonical := range tests {
		t.Run(name, func(t *testing.T) {
			dir := prepareYaziImportDir(t)
			writeYaziTestFile(t, filepath.Join(dir, name), canonical+"\n# user extension\n")
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			var observation YaziFileObservation
			switch name {
			case YaziFileMain:
				observation = got.Main
			case YaziFileKeymap:
				observation = got.Keymap
			default:
				observation = got.Theme
			}
			if observation.Ownership != YaziOwnershipNative || observation.ReadOnlyReason == "" {
				t.Fatalf("header-plus-extra claimed managed ownership: %#v", observation)
			}
		})
	}
}

func TestParseYaziTOMLEnforcesResourceCaps(t *testing.T) {
	tests := map[string]string{
		"bytes":  strings.Repeat("#", maxYaziTOMLBytes+1),
		"lines":  strings.Repeat("#\n", maxYaziTOMLLines+1),
		"string": "value = \"" + strings.Repeat("x", 70_000) + "\"\n",
		"depth":  strings.Repeat("a.", 80) + "z = true\n",
		"array":  "items = [" + strings.Repeat("1,", 12_000) + "1]\n",
		"nodes": func() string {
			var b strings.Builder
			for i := 0; i < 12_000; i++ {
				fmt.Fprintf(&b, "value%d = 1\n", i)
			}
			return b.String()
		}(),
		"tables": func() string {
			var b strings.Builder
			for i := 0; i < 4_100; i++ {
				fmt.Fprintf(&b, "[table%d]\nvalue = true\n", i)
			}
			return b.String()
		}(),
		"control": "value = \"bad\x00value\"\n",
		"bidi":    "value = \"bad\u202evalue\"\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseYaziTOML("limits.toml", []byte(content)); err == nil {
				t.Fatal("resource-limit input was accepted")
			}
		})
	}
}

func TestPreflightYaziTOMLShapeRejectsLargeStructuresBeforeDecode(t *testing.T) {
	tests := map[string]string{
		"depth":        strings.Repeat("a.", maxYaziTOMLDepth+1) + "z = true\n",
		"inline depth": "value = " + strings.Repeat("{ child = ", maxYaziTOMLDepth+1) + "1" + strings.Repeat(" }", maxYaziTOMLDepth+1) + "\n",
		"array depth":  "value = " + strings.Repeat("[", maxYaziTOMLDepth+1) + "1" + strings.Repeat("]", maxYaziTOMLDepth+1) + "\n",
		"nodes": func() string {
			var b strings.Builder
			for i := 0; i <= maxYaziTOMLNodes; i++ {
				fmt.Fprintf(&b, "v%d = 1\n", i)
			}
			return b.String()
		}(),
		"tables": func() string {
			var b strings.Builder
			for i := 0; i <= maxYaziTOMLTables; i++ {
				fmt.Fprintf(&b, "[t%d]\n", i)
			}
			return b.String()
		}(),
		"collection": "items = [" + strings.Repeat("1,", maxYaziTOMLArrayItems) + "1]\n",
		"string":     "value = \"" + strings.Repeat("x", maxYaziTOMLStringBytes+1) + "\"\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			if err := preflightYaziTOMLShape("preflight.toml", []byte(content)); err == nil {
				t.Fatal("oversized structure passed preflight")
			}
		})
	}
}

func TestPreflightYaziTOMLShapeAllowsStructuralCharactersInsideString(t *testing.T) {
	content := "value = \"" + strings.Repeat("[,],", maxYaziTOMLArrayItems+1) + "\"\n"
	if len(content) >= maxYaziTOMLStringBytes {
		t.Fatalf("fixture unexpectedly exceeds string cap: %d", len(content))
	}
	if err := preflightYaziTOMLShape("quoted.toml", []byte(content)); err != nil {
		t.Fatalf("quoted structural characters rejected: %v", err)
	}
}

func TestPreflightYaziTOMLShapeRejectsDeepDottedKeyInsideInlineTable(t *testing.T) {
	content := "value = { " + strings.Repeat("a.", maxYaziTOMLDepth-2) + "z = true }\n"
	if err := preflightYaziTOMLShape("inline-dotted.toml", []byte(content)); err == nil {
		t.Fatal("deep dotted key inside inline table passed preflight")
	}
	if _, err := parseYaziTOML("inline-dotted.toml", []byte(content)); err == nil {
		t.Fatal("deep dotted key inside inline table passed full parse")
	}
}

func TestPreflightYaziTOMLShapeAllowsExactDottedKeyDepthBoundary(t *testing.T) {
	content := "value = { " + strings.Repeat("a.", maxYaziTOMLDepth-3) + "z = true }\n"
	if err := preflightYaziTOMLShape("inline-dotted-boundary.toml", []byte(content)); err != nil {
		t.Fatalf("exact dotted-key depth boundary rejected: %v", err)
	}
	if _, err := parseYaziTOML("inline-dotted-boundary.toml", []byte(content)); err != nil {
		t.Fatalf("exact inline dotted-key boundary rejected by full parse: %v", err)
	}
}

func TestYaziTOMLTopLevelDottedKeyDepthBoundaryMatchesDecodedWalk(t *testing.T) {
	passing := strings.Repeat("a.", maxYaziTOMLDepth-2) + "z = true\n"
	if err := preflightYaziTOMLShape("top-dotted-boundary.toml", []byte(passing)); err != nil {
		t.Fatalf("exact top-level dotted-key boundary rejected by preflight: %v", err)
	}
	if _, err := parseYaziTOML("top-dotted-boundary.toml", []byte(passing)); err != nil {
		t.Fatalf("exact top-level dotted-key boundary rejected by full parse: %v", err)
	}

	failing := strings.Repeat("a.", maxYaziTOMLDepth-1) + "z = true\n"
	if err := preflightYaziTOMLShape("top-dotted-too-deep.toml", []byte(failing)); err == nil {
		t.Fatal("over-depth top-level dotted key passed preflight")
	}
	if _, err := parseYaziTOML("top-dotted-too-deep.toml", []byte(failing)); err == nil {
		t.Fatal("over-depth top-level dotted key passed full parse")
	}
}

func TestImportYaziPreservesSiblingsForEveryMalformedSource(t *testing.T) {
	for _, malformed := range []string{YaziFileMain, YaziFileKeymap, YaziFileTheme} {
		t.Run(malformed, func(t *testing.T) {
			dir := prepareYaziImportDir(t)
			writeYaziTestFile(t, filepath.Join(dir, YaziFileMain), "[mgr]\nshow_hidden = true\nlinemode = \"permissions\"\n")
			writeYaziTestFile(t, filepath.Join(dir, YaziFileKeymap), "[mgr]\nkeymap = [{ on = \"<C-n>\", run = \"arrow 1\" }]\n")
			writeYaziTestFile(t, filepath.Join(dir, YaziFileTheme), "[flavor]\ndark = \"catppuccin-mocha\"\n")
			writeYaziTestFile(t, filepath.Join(dir, malformed), "[broken\nvalue = true\n")
			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			observations := map[string]YaziFileObservation{YaziFileMain: got.Main, YaziFileKeymap: got.Keymap, YaziFileTheme: got.Theme}
			if observations[malformed].Ownership != YaziOwnershipMalformed || observations[malformed].Error == "" {
				t.Fatalf("malformed source not isolated: %#v", observations[malformed])
			}
			for name, observation := range observations {
				if name != malformed && observation.Ownership == YaziOwnershipMalformed {
					t.Fatalf("valid sibling %s was discarded: %#v", name, observation)
				}
			}
			if malformed != YaziFileMain && (!got.Config.ShowHidden || got.Config.LineMode != "permissions") {
				t.Fatalf("valid main values were discarded: %#v", got.Config)
			}
			if malformed != YaziFileKeymap && got.Config.Keymap != "emacs" {
				t.Fatalf("valid keymap value was discarded: %#v", got.Config)
			}
		})
	}
}

func TestImportYaziMissingMainDefaultsLineModeToNone(t *testing.T) {
	prepareYaziImportDir(t)
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Main.Ownership != YaziOwnershipMissing || got.Config.LineMode != "none" {
		t.Fatalf("missing main defaults = observation %#v config %#v; want linemode none", got.Main, got.Config)
	}
}

func TestImportYaziRejectsWrongPluginArrayTypes(t *testing.T) {
	for _, content := range []string{
		"[plugin]\npreviewers = false\npreloaders = []\n",
		"[plugin]\npreviewers = []\npreloaders = \"none\"\n",
	} {
		dir := prepareYaziImportDir(t)
		writeYaziTestFile(t, filepath.Join(dir, "yazi.toml"), content)
		got, err := ImportYaziConfig()
		if err != nil {
			t.Fatal(err)
		}
		if got.Main.Ownership != YaziOwnershipMalformed || got.Main.Error == "" {
			t.Fatalf("wrong modeled type must fail closed: %#v", got.Main)
		}
	}
}

func TestImportYaziRejectsSymlinkWithoutFollowingIt(t *testing.T) {
	dir := prepareYaziImportDir(t)
	target := filepath.Join(t.TempDir(), "outside.toml")
	writeYaziTestFile(t, target, "[mgr]\nshow_hidden = true\n")
	if err := os.Symlink(target, filepath.Join(dir, "yazi.toml")); err != nil {
		t.Fatal(err)
	}
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Main.Exists || got.Main.Ownership != YaziOwnershipMalformed || got.Config.ShowHidden {
		t.Fatalf("symlink was followed or not reported as existing: observation=%#v config=%#v", got.Main, got.Config)
	}
}

func TestImportYaziUnreadableKindPreservesExistsAndSiblings(t *testing.T) {
	dir := prepareYaziImportDir(t)
	writeYaziTestFile(t, filepath.Join(dir, "yazi.toml"), "[mgr]\nshow_hidden = true\n")
	if err := os.Mkdir(filepath.Join(dir, "keymap.toml"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Main.Exists || !got.Config.ShowHidden || !got.Keymap.Exists || got.Keymap.Ownership != YaziOwnershipMalformed {
		t.Fatalf("file failure discarded existence or sibling: main=%#v keymap=%#v config=%#v", got.Main, got.Keymap, got.Config)
	}
}

func TestImportYaziOversizedMainIsMalformedWithoutHydrationAndKeepsSibling(t *testing.T) {
	for _, external := range []bool{false, true} {
		name := "in-home"
		if external {
			name = "external"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".config", "yazi")
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			if external {
				dir = t.TempDir()
				t.Setenv("YAZI_CONFIG_HOME", dir)
			} else {
				t.Setenv("YAZI_CONFIG_HOME", "")
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			mainPath := filepath.Join(dir, YaziFileMain)
			file, err := os.OpenFile(mainPath, os.O_CREATE|os.O_WRONLY, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			if err := file.Truncate(maxYaziTOMLBytes + 1); err != nil {
				_ = file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			writeYaziTestFile(t, filepath.Join(dir, YaziFileKeymap), "[mgr]\nkeymap = [{ on = \"<C-n>\", run = \"arrow 1\" }]\n")

			got, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if !got.Main.Exists || got.Main.Ownership != YaziOwnershipMalformed || got.Main.Error == "" || got.Config.ShowHidden {
				t.Fatalf("oversized main was hydrated or incompletely reported: observation=%#v config=%#v", got.Main, got.Config)
			}
			if got.Keymap.Ownership != YaziOwnershipNative || got.Keymap.Error != "" || got.Config.Keymap != "emacs" {
				t.Fatalf("valid sibling was discarded: observation=%#v config=%#v", got.Keymap, got.Config)
			}
			if got.Main.External != external {
				t.Fatalf("external classification = %t, want %t", got.Main.External, external)
			}
		})
	}
}

func TestParseYaziTOMLRejectsDuplicatesAndDottedCollisions(t *testing.T) {
	for _, content := range []string{
		"[mgr]\nshow_hidden = true\nshow_hidden = false\n",
		"mgr.show_hidden = true\n[mgr]\nshow_hidden = false\n",
	} {
		if _, err := parseYaziTOML("test.toml", []byte(content)); err == nil {
			t.Fatalf("ambiguous TOML accepted: %q", content)
		}
	}
}

func prepareYaziImportDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "yazi")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("YAZI_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	return dir
}

func writeYaziTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
