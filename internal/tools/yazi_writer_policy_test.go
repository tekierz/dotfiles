package tools

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestWriteYaziConfigTrackedUsesResolvedXDGPaths(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "custom-config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("YAZI_CONFIG_HOME", "")

	paths, err := ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	cfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "natural", LineMode: "none", ScrollOff: 5}
	evidence, err := WriteYaziConfigTracked(cfg, "nord")
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{paths.Main, paths.Keymap, paths.Theme}
	if len(evidence) != len(wantPaths) {
		t.Fatalf("evidence count = %d, want %d: %#v", len(evidence), len(wantPaths), evidence)
	}
	for i, want := range wantPaths {
		if evidence[i].Path != want {
			t.Fatalf("evidence[%d].Path = %q, want %q", i, evidence[i].Path, want)
		}
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("resolved target %s was not written: %v", want, err)
		}
	}
	imported, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if imported.Main.Ownership != YaziOwnershipExactCurrent || imported.Keymap.Ownership != YaziOwnershipExactCurrent || imported.Theme.Ownership != YaziOwnershipExactCurrent {
		t.Fatalf("resolved generated triple is not ExactCurrent: main=%#v keymap=%#v theme=%#v", imported.Main, imported.Keymap, imported.Theme)
	}
	defaultDir := filepath.Join(home, ".config", "yazi")
	if _, err := os.Lstat(defaultDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inactive default path was created: %v", err)
	}
}

func TestWriteYaziConfigTrackedUsesInHomeYaziConfigHomeOverride(t *testing.T) {
	home := t.TempDir()
	override := filepath.Join(home, "active-yazi")
	xdg := filepath.Join(home, "inactive-xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("YAZI_CONFIG_HOME", override)

	paths, err := ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := WriteYaziConfigTracked(YaziConfig{Keymap: "emacs", LineMode: "none", ScrollOff: 5}, "nord")
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{paths.Main, paths.Keymap, paths.Theme}
	if len(evidence) != len(wantPaths) {
		t.Fatalf("evidence count = %d, want %d", len(evidence), len(wantPaths))
	}
	for i, want := range wantPaths {
		if evidence[i].Path != want {
			t.Fatalf("evidence[%d].Path = %q, want %q", i, evidence[i].Path, want)
		}
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("override target %s was not written: %v", want, err)
		}
	}
	imported, err := ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if imported.Main.Ownership != YaziOwnershipExactCurrent || imported.Keymap.Ownership != YaziOwnershipExactCurrent || imported.Theme.Ownership != YaziOwnershipExactCurrent {
		t.Fatalf("override generated triple is not ExactCurrent: %#v", imported)
	}
	for _, inactive := range []string{filepath.Join(xdg, "yazi"), filepath.Join(home, ".config", "yazi")} {
		if _, err := os.Lstat(inactive); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("inactive path %s was created: %v", inactive, err)
		}
	}
}

func TestWriteYaziConfigTrackedBlocksExternalYaziConfigHomeBeforeMutation(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "inactive-xdg"))
	t.Setenv("YAZI_CONFIG_HOME", external)
	sentinel := []byte("[mgr]\nshow_hidden = true\n")
	mainPath := filepath.Join(external, YaziFileMain)
	if err := os.WriteFile(mainPath, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := WriteYaziConfigTracked(YaziConfig{Keymap: "emacs", LineMode: "none"}, "nord")
	if !errors.Is(err, ErrUnmanagedConfig) {
		t.Fatalf("external writer error = %v, want ErrUnmanagedConfig", err)
	}
	if message := err.Error(); !strings.Contains(message, "external") && !strings.Contains(message, "outside HOME") && !strings.Contains(message, "read-only") {
		t.Fatalf("external writer error lacks active-source reason: %q", message)
	}
	got, readErr := os.ReadFile(mainPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, sentinel) {
		t.Fatalf("external sentinel changed: %q", got)
	}
	for _, name := range []string{YaziFileKeymap, YaziFileTheme} {
		if _, statErr := os.Lstat(filepath.Join(external, name)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("external sibling %s was created: %v", name, statErr)
		}
	}
}

func TestWriteYaziConfigTrackedRejectsRelativeOverrideWithoutFallbackWrites(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "inactive-xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("YAZI_CONFIG_HOME", "relative/yazi")
	_, err := WriteYaziConfigTracked(YaziConfig{Keymap: "vim", LineMode: "none"}, "nord")
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative override error = %v, want absolute-path resolver failure", err)
	}
	for _, inactive := range []string{filepath.Join(home, ".config", "yazi"), filepath.Join(xdg, "yazi"), filepath.Join(home, "relative")} {
		if _, statErr := os.Lstat(inactive); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("resolver failure created fallback path %s: %v", inactive, statErr)
		}
	}
}

func TestPreflightYaziOwnershipSetRejectsMalformedNonexistentObservation(t *testing.T) {
	imported := YaziConfigImport{
		Main: YaziFileObservation{Kind: YaziFileKindMain, Path: "/home/user/.config/yazi/yazi.toml", Exists: true, Ownership: YaziOwnershipExactCurrent},
		Keymap: YaziFileObservation{
			Kind: YaziFileKindKeymap, Path: "/home/user/.config/yazi/keymap.toml", Exists: false,
			Ownership: YaziOwnershipMalformed, Error: "ancestor symlink refused",
		},
		Theme: YaziFileObservation{Kind: YaziFileKindTheme, Path: "/home/user/.config/yazi/theme.toml", Exists: false, Ownership: YaziOwnershipMissing},
	}
	err := preflightYaziOwnershipSet(imported)
	if !errors.Is(err, ErrUnmanagedConfig) {
		t.Fatalf("preflight error = %v, want ErrUnmanagedConfig", err)
	}
	message := strings.ToLower(err.Error())
	for _, want := range []string{YaziFileKeymap, "malformed", "ancestor symlink"} {
		if !strings.Contains(message, want) {
			t.Fatalf("preflight error %q omits %q", message, want)
		}
	}
}

func TestWriteYaziConfigTrackedPreflightsExactPerFileOwnershipBeforeTripleMutation(t *testing.T) {
	historicalCfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "modified", SortReverse: true, LineMode: "permissions", ScrollOff: 9}
	tests := []struct {
		name       string
		file       string
		content    string
		ownership  YaziFileOwnership
		reasonPart string
	}{
		{"main native", YaziFileMain, "[mgr]\nshow_hidden = true\n", YaziOwnershipNative, "native"},
		{"main malformed", YaziFileMain, "[mgr\nshow_hidden = true\n", YaziOwnershipMalformed, "malformed"},
		{"main header extra", YaziFileMain, GenerateYaziConfig(YaziConfig{SortBy: "natural", LineMode: "none", ScrollOff: 5}, "nord") + "# user extension\n", YaziOwnershipNative, "native"},
		{"main old full", YaziFileMain, generateHistoricalFullYaziConfig(historicalCfg, "nord"), YaziOwnershipExactHistorical, "historical"},
		{"main pre-bb", YaziFileMain, generateHistoricalYaziMain(historicalCfg, "nord"), YaziOwnershipExactHistorical, "historical"},
		{"keymap malformed", YaziFileKeymap, "[mgr\nkeymap = []\n", YaziOwnershipMalformed, "malformed"},
		{"keymap header extra", YaziFileKeymap, GenerateYaziKeymap(YaziConfig{Keymap: "vim"}, "nord") + "# user extension\n", YaziOwnershipNative, "native"},
		{"keymap old full", YaziFileKeymap, generateHistoricalFullYaziKeymap(historicalCfg, "nord"), YaziOwnershipExactHistorical, "historical"},
		{"keymap pre-bb", YaziFileKeymap, generateHistoricalYaziKeymap(historicalCfg), YaziOwnershipExactHistorical, "historical"},
		{"theme flavor native", YaziFileTheme, "[flavor]\ndark = \"catppuccin-mocha\"\n", YaziOwnershipNative, "native"},
		{"theme malformed", YaziFileTheme, "[mgr\ncwd = {}\n", YaziOwnershipMalformed, "malformed"},
		{"theme header extra", YaziFileTheme, GenerateYaziTheme("nord") + "# user extension\n", YaziOwnershipNative, "native"},
		{"theme old full", YaziFileTheme, generateHistoricalFullYaziTheme("nord"), YaziOwnershipExactHistorical, "historical"},
		{"theme pre-bb", YaziFileTheme, generateHistoricalYaziTheme("nord"), YaziOwnershipExactHistorical, "historical"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("YAZI_CONFIG_HOME", "")
			dir := filepath.Join(home, ".config", "yazi")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			blockerPath := filepath.Join(dir, test.file)
			if err := os.WriteFile(blockerPath, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			imported, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			observed := map[string]YaziFileObservation{YaziFileMain: imported.Main, YaziFileKeymap: imported.Keymap, YaziFileTheme: imported.Theme}[test.file]
			if observed.Ownership != test.ownership {
				t.Fatalf("fixture ownership = %s, want %s: %#v", observed.Ownership, test.ownership, observed)
			}

			_, err = WriteYaziConfigTracked(YaziConfig{Keymap: "vim", SortBy: "natural", LineMode: "none", ScrollOff: 5}, "dracula")
			if !errors.Is(err, ErrUnmanagedConfig) {
				t.Fatalf("writer error = %v, want ErrUnmanagedConfig", err)
			}
			message := strings.ToLower(err.Error())
			if !strings.Contains(message, strings.ToLower(test.file)) || !strings.Contains(message, test.reasonPart) {
				t.Fatalf("writer reason %q omits %s and %s", err, test.file, test.reasonPart)
			}
			got, readErr := os.ReadFile(blockerPath)
			if readErr != nil || !bytes.Equal(got, []byte(test.content)) {
				t.Fatalf("blocker changed: data=%q err=%v", got, readErr)
			}
			for _, sibling := range []string{YaziFileMain, YaziFileKeymap, YaziFileTheme} {
				if sibling == test.file {
					continue
				}
				if _, statErr := os.Lstat(filepath.Join(dir, sibling)); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("missing sibling %s was created: %v", sibling, statErr)
				}
			}
		})
	}
}

func TestWriteYaziConfigTrackedAllowsOnlyMissingAndExactCurrentMixture(t *testing.T) {
	initial := YaziConfig{Keymap: "vim", SortBy: "natural", LineMode: "none", ScrollOff: 5}
	fixtures := map[string]string{
		YaziFileMain:   GenerateYaziConfig(initial, "nord"),
		YaziFileKeymap: GenerateYaziKeymap(initial, "nord"),
		YaziFileTheme:  GenerateYaziTheme("nord"),
	}
	for existing, content := range fixtures {
		t.Run(existing, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("YAZI_CONFIG_HOME", "")
			dir := filepath.Join(home, ".config", "yazi")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, existing), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			desired := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", LineMode: "permissions", ScrollOff: 9}
			if _, err := WriteYaziConfigTracked(desired, "dracula"); err != nil {
				t.Fatal(err)
			}
			imported, err := ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			if imported.Main.Ownership != YaziOwnershipExactCurrent || imported.Keymap.Ownership != YaziOwnershipExactCurrent || imported.Theme.Ownership != YaziOwnershipExactCurrent {
				t.Fatalf("successful mixture did not become ExactCurrent: %#v", imported)
			}
			wantBytes := map[string]string{
				YaziFileMain:   GenerateYaziConfig(desired, "dracula"),
				YaziFileKeymap: GenerateYaziKeymap(desired, "dracula"),
				YaziFileTheme:  GenerateYaziTheme("dracula"),
			}
			for name, want := range wantBytes {
				got, readErr := os.ReadFile(filepath.Join(dir, name))
				if readErr != nil || !bytes.Equal(got, []byte(want)) {
					t.Fatalf("%s bytes = %q err=%v, want exact desired bytes %q", name, got, readErr, want)
				}
			}
		})
	}
}

func TestAcceptedYaziWriterUsesFrozenResolvedPathsAfterEnvironmentChanges(t *testing.T) {
	home := t.TempDir()
	configA := filepath.Join(home, "config-a")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configA)
	t.Setenv("YAZI_CONFIG_HOME", "")

	pathsA, err := ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pathsA.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
		t.Helper()
		rel, relErr := filepath.Rel(home, path)
		if relErr != nil {
			t.Fatal(relErr)
		}
		_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
		if observeErr != nil {
			t.Fatal(observeErr)
		}
		return revision, parents
	}
	mainRevision, mainParents := observe(pathsA.Main)
	keymapRevision, keymapParents := observe(pathsA.Keymap)
	themeRevision, themeParents := observe(pathsA.Theme)

	configBParent := filepath.Join(home, "config-b-parent")
	configBOverride := filepath.Join(home, "config-b-override")
	t.Setenv("XDG_CONFIG_HOME", configBParent)
	t.Setenv("YAZI_CONFIG_HOME", configBOverride)

	cfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", LineMode: "permissions", ScrollOff: 9}
	evidence, err := WriteYaziConfigAtResolvedAuthoritiesTracked(
		cfg, "dracula", pathsA,
		mainRevision, mainParents,
		keymapRevision, keymapParents,
		themeRevision, themeParents,
		operation.DefaultLocker,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 3 {
		t.Fatalf("evidence count = %d, want 3", len(evidence))
	}
	want := []struct {
		path    string
		content string
	}{
		{pathsA.Main, GenerateYaziConfig(cfg, "dracula")},
		{pathsA.Keymap, GenerateYaziKeymap(cfg, "dracula")},
		{pathsA.Theme, GenerateYaziTheme("dracula")},
	}
	for i, target := range want {
		assertExactMutationEvidence(t, target.path, evidence[i])
		got, readErr := os.ReadFile(target.path)
		if readErr != nil || !bytes.Equal(got, []byte(target.content)) {
			t.Fatalf("frozen target %s bytes = %q err=%v, want %q", target.path, got, readErr, target.content)
		}
	}
	for _, dir := range []string{filepath.Join(configBParent, "yazi"), configBOverride} {
		for _, name := range []string{YaziFileMain, YaziFileKeymap, YaziFileTheme} {
			if _, statErr := os.Lstat(filepath.Join(dir, name)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("environment-drift target %s was mutated: %v", filepath.Join(dir, name), statErr)
			}
		}
	}
}

func TestAcceptedYaziWriterReleaseFailureReturnsAllCommittedEvidence(t *testing.T) {
	for _, test := range []struct {
		name        string
		failRelease int
		committed   int
	}{{"main", 4, 1}, {"keymap", 5, 2}, {"theme", 6, 3}} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			t.Setenv("YAZI_CONFIG_HOME", "")
			paths, err := ResolveYaziConfigPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
			observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
				t.Helper()
				rel, relErr := filepath.Rel(home, path)
				if relErr != nil {
					t.Fatal(relErr)
				}
				_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
				if observeErr != nil {
					t.Fatal(observeErr)
				}
				return revision, parents
			}
			mainRevision, mainParents := observe(paths.Main)
			keymapRevision, keymapParents := observe(paths.Keymap)
			themeRevision, themeParents := observe(paths.Theme)

			releaseErr := errors.New("injected Yazi unlock failure")
			releases := 0
			locker := operation.Locker(func(string, string) (func() error, error) {
				return func() error {
					releases++
					if releases == test.failRelease {
						return releaseErr
					}
					return nil
				}, nil
			})
			cfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", LineMode: "permissions", ScrollOff: 9}
			result, err := WriteYaziConfigAtResolvedAuthoritiesTracked(
				cfg, "dracula", paths,
				mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, locker,
			)
			var partial *PartialMutationError
			if result != nil || !errors.Is(err, releaseErr) || !errors.As(err, &partial) {
				t.Fatalf("result=%+v error=%v partial=%+v", result, err, partial)
			}
			orderedPaths := []string{paths.Main, paths.Keymap, paths.Theme}
			orderedParents := []*safefile.ParentChain{mainParents, keymapParents, themeParents}
			desiredBytes := [][]byte{[]byte(GenerateYaziConfig(cfg, "dracula")), []byte(GenerateYaziKeymap(cfg, "dracula")), []byte(GenerateYaziTheme("dracula"))}
			if len(partial.Evidence) != test.committed {
				t.Fatalf("committed evidence=%+v, want %d entries", partial.Evidence, test.committed)
			}
			for i, path := range orderedPaths {
				if i < test.committed {
					assertExactMutationEvidence(t, path, partial.Evidence[i])
					if partial.Evidence[i].Parents != orderedParents[i] {
						t.Fatalf("evidence[%d] parents=%p, want accepted chain %p", i, partial.Evidence[i].Parents, orderedParents[i])
					}
					if got, readErr := os.ReadFile(path); readErr != nil || !bytes.Equal(got, desiredBytes[i]) {
						t.Fatalf("committed %s bytes=%q err=%v", path, got, readErr)
					}
				} else if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("later sibling %s was mutated: %v", path, statErr)
				}
			}
		})
	}
}

func TestAcceptedYaziWriterNoOpUnlockFailureDoesNotFabricateCommittedEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("YAZI_CONFIG_HOME", "")
	paths, err := ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", LineMode: "permissions", ScrollOff: 9}
	desired := map[string][]byte{paths.Main: []byte(GenerateYaziConfig(cfg, "dracula")), paths.Keymap: []byte(GenerateYaziKeymap(cfg, "dracula")), paths.Theme: []byte(GenerateYaziTheme("dracula"))}
	for path, content := range desired {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
		t.Helper()
		rel, _ := filepath.Rel(home, path)
		_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
		if observeErr != nil {
			t.Fatal(observeErr)
		}
		return revision, parents
	}
	mainRevision, mainParents := observe(paths.Main)
	keymapRevision, keymapParents := observe(paths.Keymap)
	themeRevision, themeParents := observe(paths.Theme)
	releaseErr := errors.New("injected no-op unlock failure")
	releases := 0
	locker := operation.Locker(func(string, string) (func() error, error) {
		return func() error {
			releases++
			if releases == 4 {
				return releaseErr
			}
			return nil
		}, nil
	})
	result, err := WriteYaziConfigAtResolvedAuthoritiesTracked(cfg, "dracula", paths, mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, locker)
	var partial *PartialMutationError
	if result != nil || !errors.Is(err, releaseErr) {
		t.Fatalf("result=%+v error=%v partial=%+v, want empty committed evidence", result, err, partial)
	}
	if errors.As(err, &partial) && len(partial.Evidence) != 0 {
		t.Fatalf("no-op unlock failure fabricated committed evidence: %+v", partial.Evidence)
	}
	accepted := map[string]safefile.Revision{paths.Main: mainRevision, paths.Keymap: keymapRevision, paths.Theme: themeRevision}
	for path, wantRevision := range accepted {
		rel, relErr := filepath.Rel(home, path)
		if relErr != nil {
			t.Fatal(relErr)
		}
		got, current, _, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
		if observeErr != nil {
			t.Fatal(observeErr)
		}
		if current != wantRevision || !bytes.Equal(got, desired[path]) {
			t.Fatalf("no-op target %s changed: revision=%+v want=%+v bytes=%q", path, current, wantRevision, got)
		}
	}
}

func TestAcceptedYaziWriterRejectsFrozenExternalPathsDespiteEnvironmentDrift(t *testing.T) {
	home := t.TempDir()
	externalRoot := t.TempDir()
	externalDir := filepath.Join(externalRoot, "yazi")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("YAZI_CONFIG_HOME", externalDir)
	paths, err := ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := []byte(GenerateYaziConfig(YaziConfig{SortBy: "natural", LineMode: "none", ScrollOff: 5}, "nord"))
	if err := os.WriteFile(paths.Main, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
		t.Helper()
		rel, relErr := filepath.Rel(externalRoot, path)
		if relErr != nil {
			t.Fatal(relErr)
		}
		_, revision, parents, observeErr := safefile.ObserveFileWithin(externalRoot, filepath.ToSlash(rel))
		if observeErr != nil {
			t.Fatal(observeErr)
		}
		return revision, parents
	}
	mainRevision, mainParents := observe(paths.Main)
	keymapRevision, keymapParents := observe(paths.Keymap)
	themeRevision, themeParents := observe(paths.Theme)

	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "trusted-now"))
	t.Setenv("YAZI_CONFIG_HOME", filepath.Join(home, "trusted-now", "yazi"))
	_, err = WriteYaziConfigAtResolvedAuthoritiesTracked(
		YaziConfig{Keymap: "emacs", SortBy: "size", LineMode: "permissions", ScrollOff: 9}, "dracula", paths,
		mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, operation.DefaultLocker,
	)
	if !errors.Is(err, ErrUnmanagedConfig) || !strings.Contains(strings.ToLower(err.Error()), "read-only") {
		t.Fatalf("external accepted writer error=%v, want typed read-only ErrUnmanagedConfig", err)
	}
	if got, readErr := os.ReadFile(paths.Main); readErr != nil || !bytes.Equal(got, sentinel) {
		t.Fatalf("external sentinel changed: data=%q err=%v", got, readErr)
	}
	for _, path := range []string{paths.Keymap, paths.Theme} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("external sibling %s was mutated: %v", path, statErr)
		}
	}
}

func TestAcceptedYaziWriterGloballyRejectsMixedOwnership(t *testing.T) {
	historicalCfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "modified", SortReverse: true, LineMode: "permissions", ScrollOff: 9}
	tests := []struct {
		name      string
		file      string
		content   string
		ownership YaziFileOwnership
	}{
		{"main historical", YaziFileMain, generateHistoricalFullYaziConfig(historicalCfg, "nord"), YaziOwnershipExactHistorical},
		{"main header extra", YaziFileMain, GenerateYaziConfig(YaziConfig{SortBy: "natural", LineMode: "none", ScrollOff: 5}, "nord") + "# user extension\n", YaziOwnershipNative},
		{"keymap historical", YaziFileKeymap, generateHistoricalFullYaziKeymap(historicalCfg, "nord"), YaziOwnershipExactHistorical},
		{"keymap header extra", YaziFileKeymap, GenerateYaziKeymap(YaziConfig{Keymap: "vim"}, "nord") + "# user extension\n", YaziOwnershipNative},
		{"theme historical", YaziFileTheme, generateHistoricalFullYaziTheme("nord"), YaziOwnershipExactHistorical},
		{"theme header extra", YaziFileTheme, GenerateYaziTheme("nord") + "# user extension\n", YaziOwnershipNative},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("YAZI_CONFIG_HOME", "")
			paths, err := ResolveYaziConfigPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
			blocker := filepath.Join(paths.Dir, test.file)
			if err := os.WriteFile(blocker, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
				t.Helper()
				rel, relErr := filepath.Rel(home, path)
				if relErr != nil {
					t.Fatal(relErr)
				}
				_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
				if observeErr != nil {
					t.Fatal(observeErr)
				}
				return revision, parents
			}
			mainRevision, mainParents := observe(paths.Main)
			keymapRevision, keymapParents := observe(paths.Keymap)
			themeRevision, themeParents := observe(paths.Theme)

			_, err = WriteYaziConfigAtResolvedAuthoritiesTracked(
				YaziConfig{Keymap: "emacs", SortBy: "size", LineMode: "permissions", ScrollOff: 9}, "dracula", paths,
				mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, operation.DefaultLocker,
			)
			if !errors.Is(err, ErrUnmanagedConfig) {
				t.Fatalf("accepted writer error=%v, want ErrUnmanagedConfig", err)
			}
			message := strings.ToLower(err.Error())
			if !strings.Contains(message, strings.ToLower(test.file)) || !strings.Contains(message, strings.ToLower(string(test.ownership))) {
				t.Fatalf("accepted writer reason %q omits %s and %s", err, test.file, test.ownership)
			}
			if got, readErr := os.ReadFile(blocker); readErr != nil || !bytes.Equal(got, []byte(test.content)) {
				t.Fatalf("blocker changed: data=%q err=%v", got, readErr)
			}
			for _, sibling := range []string{paths.Main, paths.Keymap, paths.Theme} {
				if sibling == blocker {
					continue
				}
				if _, statErr := os.Lstat(sibling); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("missing sibling %s was mutated: %v", sibling, statErr)
				}
			}
		})
	}
}

func TestAcceptedYaziWriterGloballyRejectsByteIdenticalRevisionDrift(t *testing.T) {
	for _, driftedFile := range []string{YaziFileMain, YaziFileKeymap, YaziFileTheme} {
		t.Run(driftedFile, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			t.Setenv("YAZI_CONFIG_HOME", "")
			paths, err := ResolveYaziConfigPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
			initial := YaziConfig{Keymap: "vim", ShowHidden: false, PreviewMode: "auto", SortBy: "natural", LineMode: "none", ScrollOff: 5}
			before := map[string][]byte{
				paths.Main:   []byte(GenerateYaziConfig(initial, "nord")),
				paths.Keymap: []byte(GenerateYaziKeymap(initial, "nord")),
				paths.Theme:  []byte(GenerateYaziTheme("nord")),
			}
			for path, content := range before {
				if err := os.WriteFile(path, content, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
				t.Helper()
				rel, relErr := filepath.Rel(home, path)
				if relErr != nil {
					t.Fatal(relErr)
				}
				_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
				if observeErr != nil {
					t.Fatal(observeErr)
				}
				return revision, parents
			}
			mainRevision, mainParents := observe(paths.Main)
			keymapRevision, keymapParents := observe(paths.Keymap)
			themeRevision, themeParents := observe(paths.Theme)

			driftedPath := filepath.Join(paths.Dir, driftedFile)
			replaceFileWithIdenticalNewIdentity(t, driftedPath)
			desired := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", SortReverse: true, LineMode: "permissions", ScrollOff: 9}
			_, err = WriteYaziConfigAtResolvedAuthoritiesTracked(
				desired, "dracula", paths,
				mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, operation.DefaultLocker,
			)
			if !errors.Is(err, safefile.ErrRevisionChanged) || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(driftedFile)) {
				t.Fatalf("revision drift error=%v, want ErrRevisionChanged naming %s", err, driftedFile)
			}
			for path, want := range before {
				got, readErr := os.ReadFile(path)
				if readErr != nil || !bytes.Equal(got, want) {
					t.Fatalf("%s changed after global preflight failure: data=%q err=%v want=%q", path, got, readErr, want)
				}
			}
		})
	}
}

func TestAcceptedYaziWriterGloballyRejectsByteIdenticalParentDirectoryDrift(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("YAZI_CONFIG_HOME", "")
	paths, err := ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	initial := YaziConfig{Keymap: "vim", PreviewMode: "auto", SortBy: "natural", LineMode: "none", ScrollOff: 5}
	before := map[string][]byte{
		paths.Main:   []byte(GenerateYaziConfig(initial, "nord")),
		paths.Keymap: []byte(GenerateYaziKeymap(initial, "nord")),
		paths.Theme:  []byte(GenerateYaziTheme("nord")),
	}
	for path, content := range before {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
		t.Helper()
		rel, relErr := filepath.Rel(home, path)
		if relErr != nil {
			t.Fatal(relErr)
		}
		_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
		if observeErr != nil {
			t.Fatal(observeErr)
		}
		return revision, parents
	}
	mainRevision, mainParents := observe(paths.Main)
	keymapRevision, keymapParents := observe(paths.Keymap)
	themeRevision, themeParents := observe(paths.Theme)

	replaceDirectoryWithIdenticalNewIdentity(t, paths.Dir)
	desired := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", SortReverse: true, LineMode: "permissions", ScrollOff: 9}
	_, err = WriteYaziConfigAtResolvedAuthoritiesTracked(
		desired, "dracula", paths,
		mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, operation.DefaultLocker,
	)
	if !errors.Is(err, safefile.ErrParentChanged) {
		t.Fatalf("parent directory drift error=%v, want ErrParentChanged", err)
	}
	for path, want := range before {
		got, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s changed after parent preflight failure: data=%q err=%v want=%q", path, got, readErr, want)
		}
	}
}

func TestAcceptedYaziWriterRejectsPostAcceptanceKeymapSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("YAZI_CONFIG_HOME", "")
	paths, err := ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
		t.Helper()
		rel, relErr := filepath.Rel(home, path)
		if relErr != nil {
			t.Fatal(relErr)
		}
		_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
		if observeErr != nil {
			t.Fatal(observeErr)
		}
		return revision, parents
	}
	mainRevision, mainParents := observe(paths.Main)
	keymapRevision, keymapParents := observe(paths.Keymap)
	themeRevision, themeParents := observe(paths.Theme)

	outside := filepath.Join(t.TempDir(), "outside-keymap.toml")
	sentinel := []byte(GenerateYaziKeymap(YaziConfig{Keymap: "vim"}, "nord"))
	if err := os.WriteFile(outside, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, paths.Keymap); err != nil {
		t.Fatal(err)
	}
	desired := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", LineMode: "permissions", ScrollOff: 9}
	_, err = WriteYaziConfigAtResolvedAuthoritiesTracked(
		desired, "dracula", paths,
		mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, operation.DefaultLocker,
	)
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("post-acceptance keymap symlink error=%v, want ErrSymlink", err)
	}
	if got, readErr := os.ReadFile(outside); readErr != nil || !bytes.Equal(got, sentinel) {
		t.Fatalf("outside keymap sentinel changed: data=%q err=%v", got, readErr)
	}
	if target, readErr := os.Readlink(paths.Keymap); readErr != nil || target != outside {
		t.Fatalf("keymap symlink changed: target=%q err=%v", target, readErr)
	}
	for _, path := range []string{paths.Main, paths.Theme} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("sibling %s was mutated: %v", path, statErr)
		}
	}
}

func TestAcceptedYaziWriterRejectsAlreadyAcceptedThemeHardlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("YAZI_CONFIG_HOME", "")
	paths, err := ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-theme.toml")
	sentinel := []byte(GenerateYaziTheme("nord"))
	if err := os.WriteFile(outside, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, paths.Theme); err != nil {
		t.Fatal(err)
	}
	observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
		t.Helper()
		rel, relErr := filepath.Rel(home, path)
		if relErr != nil {
			t.Fatal(relErr)
		}
		_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
		if observeErr != nil {
			t.Fatal(observeErr)
		}
		return revision, parents
	}
	mainRevision, mainParents := observe(paths.Main)
	keymapRevision, keymapParents := observe(paths.Keymap)
	themeRevision, themeParents := observe(paths.Theme)
	if themeRevision.LinkCount() != 2 {
		t.Fatalf("accepted theme LinkCount()=%d, want 2", themeRevision.LinkCount())
	}

	desired := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", LineMode: "permissions", ScrollOff: 9}
	_, err = WriteYaziConfigAtResolvedAuthoritiesTracked(
		desired, "dracula", paths,
		mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, operation.DefaultLocker,
	)
	if (!errors.Is(err, ErrUnmanagedConfig) && !errors.Is(err, safefile.ErrHardlink)) || !strings.Contains(strings.ToLower(err.Error()), "hardlink") {
		t.Fatalf("accepted theme hardlink error=%v, want typed hardlink refusal", err)
	}
	for _, path := range []string{outside, paths.Theme} {
		if got, readErr := os.ReadFile(path); readErr != nil || !bytes.Equal(got, sentinel) {
			t.Fatalf("hardlinked theme %s changed: data=%q err=%v", path, got, readErr)
		}
	}
	outsideInfo, statErr := os.Stat(outside)
	if statErr != nil {
		t.Fatal(statErr)
	}
	themeInfo, statErr := os.Stat(paths.Theme)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if !os.SameFile(outsideInfo, themeInfo) {
		t.Fatal("hardlinked theme identity was replaced")
	}
	themeRel, relErr := filepath.Rel(home, paths.Theme)
	if relErr != nil {
		t.Fatal(relErr)
	}
	_, postRevision, _, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(themeRel))
	if observeErr != nil {
		t.Fatal(observeErr)
	}
	if postRevision.LinkCount() != 2 {
		t.Fatalf("post-refusal theme LinkCount()=%d, want 2", postRevision.LinkCount())
	}
	for _, path := range []string{paths.Main, paths.Keymap} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("sibling %s was mutated: %v", path, statErr)
		}
	}
}

func TestAcceptedYaziWriterRejectsTamperedResolvedPathsBeforeMutation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("YAZI_CONFIG_HOME", "")
	baseline, err := ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(baseline.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
		t.Helper()
		rel, relErr := filepath.Rel(home, path)
		if relErr != nil {
			t.Fatal(relErr)
		}
		_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
		if observeErr != nil {
			t.Fatal(observeErr)
		}
		return revision, parents
	}
	mainRevision, mainParents := observe(baseline.Main)
	keymapRevision, keymapParents := observe(baseline.Keymap)
	themeRevision, themeParents := observe(baseline.Theme)

	externalDir := filepath.Join(t.TempDir(), "external-yazi")
	tests := []struct {
		name       string
		mutate     func(*YaziConfigPaths)
		reasonPart string
		checkPaths []string
	}{
		{"swapped main keymap", func(paths *YaziConfigPaths) { paths.Main, paths.Keymap = paths.Keymap, paths.Main }, "main", nil},
		{"main outside dir within home", func(paths *YaziConfigPaths) { paths.Main = filepath.Join(home, "outside-main.toml") }, "main", []string{filepath.Join(home, "outside-main.toml")}},
		{"mismatched dir", func(paths *YaziConfigPaths) { paths.Dir = filepath.Join(home, "other-yazi") }, "main", []string{filepath.Join(home, "other-yazi", YaziFileMain), filepath.Join(home, "other-yazi", YaziFileKeymap), filepath.Join(home, "other-yazi", YaziFileTheme)}},
		{"dir outside frozen root", func(paths *YaziConfigPaths) {
			paths.Dir = externalDir
			paths.Main = filepath.Join(externalDir, YaziFileMain)
			paths.Keymap = filepath.Join(externalDir, YaziFileKeymap)
			paths.Theme = filepath.Join(externalDir, YaziFileTheme)
		}, "external-yazi", []string{filepath.Join(externalDir, YaziFileMain), filepath.Join(externalDir, YaziFileKeymap), filepath.Join(externalDir, YaziFileTheme)}},
		{"relative theme", func(paths *YaziConfigPaths) { paths.Theme = filepath.Join("relative", YaziFileTheme) }, "theme", nil},
		{"changed main basename", func(paths *YaziConfigPaths) { paths.Main = filepath.Join(paths.Dir, "manager.toml") }, "main", []string{filepath.Join(baseline.Dir, "manager.toml")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths := baseline
			test.mutate(&paths)
			result, err := WriteYaziConfigAtResolvedAuthoritiesTracked(
				YaziConfig{Keymap: "emacs", SortBy: "size", LineMode: "permissions", ScrollOff: 9}, "dracula", paths,
				mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, operation.DefaultLocker,
			)
			if len(result) != 0 || !errors.Is(err, ErrUnmanagedConfig) || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.reasonPart)) {
				t.Fatalf("result=%+v error=%v, want typed pre-mutation refusal naming %q", result, err, test.reasonPart)
			}
			for _, path := range append([]string{baseline.Main, baseline.Keymap, baseline.Theme}, test.checkPaths...) {
				if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("tampered path validation created %s: %v", path, statErr)
				}
			}
		})
	}
}

func TestAcceptedYaziWriterRechecksRevisionAfterGlobalPreflight(t *testing.T) {
	for _, test := range []struct {
		name        string
		acquisition int
		racedIndex  int
	}{{"main", 4, 0}, {"keymap", 5, 1}, {"theme", 6, 2}} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			t.Setenv("YAZI_CONFIG_HOME", "")
			paths, err := ResolveYaziConfigPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
			observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
				t.Helper()
				rel, relErr := filepath.Rel(home, path)
				if relErr != nil {
					t.Fatal(relErr)
				}
				_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
				if observeErr != nil {
					t.Fatal(observeErr)
				}
				return revision, parents
			}
			mainRevision, mainParents := observe(paths.Main)
			keymapRevision, keymapParents := observe(paths.Keymap)
			themeRevision, themeParents := observe(paths.Theme)
			orderedPaths := []string{paths.Main, paths.Keymap, paths.Theme}
			orderedParents := []*safefile.ParentChain{mainParents, keymapParents, themeParents}

			desiredCfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", LineMode: "permissions", ScrollOff: 9}
			desired := [][]byte{[]byte(GenerateYaziConfig(desiredCfg, "dracula")), []byte(GenerateYaziKeymap(desiredCfg, "dracula")), []byte(GenerateYaziTheme("dracula"))}
			injected := append(append([]byte(nil), desired[test.racedIndex]...), []byte("# raced user extension\n")...)
			acquisitions := 0
			locker := operation.Locker(func(_ string, target string) (func() error, error) {
				acquisitions++
				if acquisitions == test.acquisition {
					if target != orderedPaths[test.racedIndex] {
						t.Fatalf("race acquisition target=%s, want %s", target, orderedPaths[test.racedIndex])
					}
					if err := os.WriteFile(target, injected, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				return func() error { return nil }, nil
			})
			result, err := WriteYaziConfigAtResolvedAuthoritiesTracked(
				desiredCfg, "dracula", paths,
				mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, locker,
			)
			if len(result) != 0 || !errors.Is(err, safefile.ErrRevisionChanged) || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(filepath.Base(orderedPaths[test.racedIndex]))) {
				t.Fatalf("result=%+v race error=%v, want ErrRevisionChanged naming %s", result, err, orderedPaths[test.racedIndex])
			}
			var partial *PartialMutationError
			var committed []MutationEvidence
			if errors.As(err, &partial) {
				committed = partial.Evidence
			}
			if len(committed) != test.racedIndex {
				t.Fatalf("committed evidence=%+v, want prefix length %d", committed, test.racedIndex)
			}
			for i := 0; i < test.racedIndex; i++ {
				assertExactMutationEvidence(t, orderedPaths[i], committed[i])
				if committed[i].Parents != orderedParents[i] {
					t.Fatalf("evidence[%d] parents=%p, want %p", i, committed[i].Parents, orderedParents[i])
				}
				if got, readErr := os.ReadFile(orderedPaths[i]); readErr != nil || !bytes.Equal(got, desired[i]) {
					t.Fatalf("prior committed %s data=%q err=%v", orderedPaths[i], got, readErr)
				}
			}
			if got, readErr := os.ReadFile(orderedPaths[test.racedIndex]); readErr != nil || !bytes.Equal(got, injected) {
				t.Fatalf("raced target %s changed: data=%q err=%v", orderedPaths[test.racedIndex], got, readErr)
			}
			for i := test.racedIndex + 1; i < len(orderedPaths); i++ {
				if _, statErr := os.Lstat(orderedPaths[i]); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("later sibling %s was mutated: %v", orderedPaths[i], statErr)
				}
			}
		})
	}
}

func TestAcceptedYaziWriterAllowsExactCurrentAndMissingPermutations(t *testing.T) {
	initialCfg := YaziConfig{Keymap: "vim", PreviewMode: "auto", SortBy: "natural", LineMode: "none", ScrollOff: 5}
	for _, existingFile := range []string{YaziFileMain, YaziFileKeymap, YaziFileTheme} {
		t.Run(existingFile, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			t.Setenv("YAZI_CONFIG_HOME", "")
			paths, err := ResolveYaziConfigPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
			initial := map[string][]byte{
				YaziFileMain:   []byte(GenerateYaziConfig(initialCfg, "nord")),
				YaziFileKeymap: []byte(GenerateYaziKeymap(initialCfg, "nord")),
				YaziFileTheme:  []byte(GenerateYaziTheme("nord")),
			}
			if err := os.WriteFile(filepath.Join(paths.Dir, existingFile), initial[existingFile], 0o600); err != nil {
				t.Fatal(err)
			}
			observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
				t.Helper()
				rel, relErr := filepath.Rel(home, path)
				if relErr != nil {
					t.Fatal(relErr)
				}
				_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
				if observeErr != nil {
					t.Fatal(observeErr)
				}
				return revision, parents
			}
			mainRevision, mainParents := observe(paths.Main)
			keymapRevision, keymapParents := observe(paths.Keymap)
			themeRevision, themeParents := observe(paths.Theme)
			desiredCfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", SortReverse: true, LineMode: "permissions", ScrollOff: 9}
			result, err := WriteYaziConfigAtResolvedAuthoritiesTracked(
				desiredCfg, "dracula", paths,
				mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, operation.DefaultLocker,
			)
			if err != nil || len(result) != 3 {
				t.Fatalf("result=%+v error=%v, want three evidence entries", result, err)
			}
			orderedPaths := []string{paths.Main, paths.Keymap, paths.Theme}
			orderedParents := []*safefile.ParentChain{mainParents, keymapParents, themeParents}
			desired := [][]byte{[]byte(GenerateYaziConfig(desiredCfg, "dracula")), []byte(GenerateYaziKeymap(desiredCfg, "dracula")), []byte(GenerateYaziTheme("dracula"))}
			for i, path := range orderedPaths {
				assertExactMutationEvidence(t, path, result[i])
				if result[i].Parents != orderedParents[i] {
					t.Fatalf("evidence[%d] parents=%p, want %p", i, result[i].Parents, orderedParents[i])
				}
				if got, readErr := os.ReadFile(path); readErr != nil || !bytes.Equal(got, desired[i]) {
					t.Fatalf("desired target %s data=%q err=%v", path, got, readErr)
				}
			}
		})
	}
}

func TestAcceptedYaziWriterGloballyRejectsMalformedFiles(t *testing.T) {
	tests := []struct {
		file    string
		content string
	}{
		{YaziFileMain, "[mgr\nshow_hidden = true\n"},
		{YaziFileKeymap, "[mgr\nkeymap = []\n"},
		{YaziFileTheme, "[mgr\ncwd = {}\n"},
	}
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			t.Setenv("YAZI_CONFIG_HOME", "")
			paths, err := ResolveYaziConfigPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
			blocker := filepath.Join(paths.Dir, test.file)
			if err := os.WriteFile(blocker, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
				t.Helper()
				rel, relErr := filepath.Rel(home, path)
				if relErr != nil {
					t.Fatal(relErr)
				}
				_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
				if observeErr != nil {
					t.Fatal(observeErr)
				}
				return revision, parents
			}
			mainRevision, mainParents := observe(paths.Main)
			keymapRevision, keymapParents := observe(paths.Keymap)
			themeRevision, themeParents := observe(paths.Theme)
			result, err := WriteYaziConfigAtResolvedAuthoritiesTracked(
				YaziConfig{Keymap: "emacs", SortBy: "size", LineMode: "permissions", ScrollOff: 9}, "dracula", paths,
				mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, operation.DefaultLocker,
			)
			if err == nil {
				t.Fatalf("result=%+v error=nil, want malformed refusal naming %s", result, test.file)
			}
			message := strings.ToLower(err.Error())
			if len(result) != 0 || !errors.Is(err, ErrUnmanagedConfig) || !strings.Contains(message, strings.ToLower(test.file)) || !strings.Contains(message, "malformed") {
				t.Fatalf("result=%+v error=%v, want malformed refusal naming %s", result, err, test.file)
			}
			if got, readErr := os.ReadFile(blocker); readErr != nil || !bytes.Equal(got, []byte(test.content)) {
				t.Fatalf("malformed blocker changed: data=%q err=%v", got, readErr)
			}
			for _, sibling := range []string{paths.Main, paths.Keymap, paths.Theme} {
				if sibling == blocker {
					continue
				}
				if _, statErr := os.Lstat(sibling); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("sibling %s was mutated: %v", sibling, statErr)
				}
			}
		})
	}
}

func TestAcceptedYaziWriterPreflightLockFailuresNeverMutate(t *testing.T) {
	for _, test := range []struct {
		name            string
		failAcquisition int
		failRelease     int
	}{
		{"main acquisition", 1, 0}, {"keymap acquisition", 2, 0}, {"theme acquisition", 3, 0},
		{"main release", 0, 1}, {"keymap release", 0, 2}, {"theme release", 0, 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			t.Setenv("YAZI_CONFIG_HOME", "")
			paths, err := ResolveYaziConfigPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
			observe := func(path string) (safefile.Revision, *safefile.ParentChain) {
				t.Helper()
				rel, relErr := filepath.Rel(home, path)
				if relErr != nil {
					t.Fatal(relErr)
				}
				_, revision, parents, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
				if observeErr != nil {
					t.Fatal(observeErr)
				}
				return revision, parents
			}
			mainRevision, mainParents := observe(paths.Main)
			keymapRevision, keymapParents := observe(paths.Keymap)
			themeRevision, themeParents := observe(paths.Theme)

			injected := errors.New("injected Yazi preflight lock failure")
			acquisitions, releases := 0, 0
			locker := operation.Locker(func(string, string) (func() error, error) {
				acquisitions++
				if acquisitions == test.failAcquisition {
					return nil, injected
				}
				return func() error {
					releases++
					if releases == test.failRelease {
						return injected
					}
					return nil
				}, nil
			})
			result, err := WriteYaziConfigAtResolvedAuthoritiesTracked(
				YaziConfig{Keymap: "emacs", SortBy: "size", LineMode: "permissions", ScrollOff: 9}, "dracula", paths,
				mainRevision, mainParents, keymapRevision, keymapParents, themeRevision, themeParents, locker,
			)
			var partial *PartialMutationError
			if len(result) != 0 || !errors.Is(err, injected) || errors.As(err, &partial) {
				t.Fatalf("result=%+v error=%v partial=%+v, want plain preflight failure", result, err, partial)
			}
			for _, path := range []string{paths.Main, paths.Keymap, paths.Theme} {
				if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("preflight lock failure created %s: %v", path, statErr)
				}
			}
		})
	}
}

func TestAcceptedYaziPerFileWriterUsesFrozenResolvedAuthority(t *testing.T) {
	for _, test := range []struct {
		name string
		kind YaziFileKind
		path func(YaziConfigPaths) string
		want func(YaziConfig, string) string
	}{
		{"main", YaziFileKindMain, func(paths YaziConfigPaths) string { return paths.Main }, func(cfg YaziConfig, theme string) string { return GenerateYaziConfig(cfg, theme) }},
		{"keymap", YaziFileKindKeymap, func(paths YaziConfigPaths) string { return paths.Keymap }, func(cfg YaziConfig, theme string) string { return GenerateYaziKeymap(cfg, theme) }},
		{"theme", YaziFileKindTheme, func(paths YaziConfigPaths) string { return paths.Theme }, func(_ YaziConfig, theme string) string { return GenerateYaziTheme(theme) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			configA := filepath.Join(home, "config-a")
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", configA)
			t.Setenv("YAZI_CONFIG_HOME", "")
			paths, err := ResolveYaziConfigPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
			selectedPath := test.path(paths)
			rel, err := filepath.Rel(home, selectedPath)
			if err != nil {
				t.Fatal(err)
			}
			_, accepted, parents, err := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
			if err != nil {
				t.Fatal(err)
			}

			configBParent := filepath.Join(home, "config-b-parent")
			configBOverride := filepath.Join(home, "config-b-override")
			t.Setenv("XDG_CONFIG_HOME", configBParent)
			t.Setenv("YAZI_CONFIG_HOME", configBOverride)
			cfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", SortReverse: true, LineMode: "permissions", ScrollOff: 9}
			theme := "dracula"
			evidence, err := WriteYaziFileAtResolvedAuthorityTracked(test.kind, cfg, theme, paths, accepted, parents, operation.DefaultLocker)
			if err != nil {
				t.Fatal(err)
			}
			assertExactMutationEvidence(t, selectedPath, evidence)
			if evidence.Parents != parents {
				t.Fatalf("evidence parents=%p, want accepted chain %p", evidence.Parents, parents)
			}
			if got, readErr := os.ReadFile(selectedPath); readErr != nil || !bytes.Equal(got, []byte(test.want(cfg, theme))) {
				t.Fatalf("selected %s bytes=%q err=%v", selectedPath, got, readErr)
			}
			for _, path := range []string{paths.Main, paths.Keymap, paths.Theme} {
				if path == selectedPath {
					continue
				}
				if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("unselected sibling %s was mutated: %v", path, statErr)
				}
			}
			for _, dir := range []string{filepath.Join(configBParent, "yazi"), configBOverride} {
				for _, name := range []string{YaziFileMain, YaziFileKeymap, YaziFileTheme} {
					if _, statErr := os.Lstat(filepath.Join(dir, name)); !errors.Is(statErr, os.ErrNotExist) {
						t.Fatalf("environment-drift target %s was mutated: %v", filepath.Join(dir, name), statErr)
					}
				}
			}
		})
	}
}

func TestAcceptedYaziPerFileWriterCommittedUnlockFailureReturnsPartialEvidence(t *testing.T) {
	for _, kind := range []YaziFileKind{YaziFileKindMain, YaziFileKindKeymap, YaziFileKindTheme} {
		t.Run(string(kind), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			t.Setenv("YAZI_CONFIG_HOME", "")
			paths, err := ResolveYaziConfigPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
			selected := map[YaziFileKind]string{YaziFileKindMain: paths.Main, YaziFileKindKeymap: paths.Keymap, YaziFileKindTheme: paths.Theme}[kind]
			rel, _ := filepath.Rel(home, selected)
			_, accepted, parents, err := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
			if err != nil {
				t.Fatal(err)
			}
			releaseErr := errors.New("injected per-file committed unlock failure")
			releases := 0
			locker := operation.Locker(func(string, string) (func() error, error) {
				return func() error {
					releases++
					if releases == 2 {
						return releaseErr
					}
					return nil
				}, nil
			})
			cfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", LineMode: "permissions", ScrollOff: 9}
			ordinary, err := WriteYaziFileAtResolvedAuthorityTracked(kind, cfg, "dracula", paths, accepted, parents, locker)
			var partial *PartialMutationError
			if ordinary.Path != "" || ordinary.Revision.Tracked() || ordinary.Directory != nil || ordinary.Parents != nil || !errors.Is(err, releaseErr) || !errors.As(err, &partial) || len(partial.Evidence) != 1 {
				t.Fatalf("ordinary=%+v error=%v partial=%+v, want one committed partial evidence", ordinary, err, partial)
			}
			committed := partial.Evidence[0]
			assertExactMutationEvidence(t, selected, committed)
			if committed.Parents != parents {
				t.Fatalf("committed parents=%p, want accepted %p", committed.Parents, parents)
			}
			want := map[YaziFileKind]string{YaziFileKindMain: GenerateYaziConfig(cfg, "dracula"), YaziFileKindKeymap: GenerateYaziKeymap(cfg, "dracula"), YaziFileKindTheme: GenerateYaziTheme("dracula")}[kind]
			if got, readErr := os.ReadFile(selected); readErr != nil || !bytes.Equal(got, []byte(want)) {
				t.Fatalf("selected committed bytes=%q err=%v", got, readErr)
			}
		})
	}
}

func TestAcceptedYaziPerFileWriterNoOpUnlockFailureDoesNotFabricateCommit(t *testing.T) {
	for _, kind := range []YaziFileKind{YaziFileKindMain, YaziFileKindKeymap, YaziFileKindTheme} {
		t.Run(string(kind), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			t.Setenv("YAZI_CONFIG_HOME", "")
			paths, err := ResolveYaziConfigPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
			cfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", LineMode: "permissions", ScrollOff: 9}
			selected := map[YaziFileKind]string{YaziFileKindMain: paths.Main, YaziFileKindKeymap: paths.Keymap, YaziFileKindTheme: paths.Theme}[kind]
			content := map[YaziFileKind]string{YaziFileKindMain: GenerateYaziConfig(cfg, "dracula"), YaziFileKindKeymap: GenerateYaziKeymap(cfg, "dracula"), YaziFileKindTheme: GenerateYaziTheme("dracula")}[kind]
			if err := os.WriteFile(selected, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			rel, _ := filepath.Rel(home, selected)
			_, accepted, parents, err := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
			if err != nil {
				t.Fatal(err)
			}
			releaseErr := errors.New("injected per-file no-op unlock failure")
			releases := 0
			locker := operation.Locker(func(string, string) (func() error, error) {
				return func() error {
					releases++
					if releases == 2 {
						return releaseErr
					}
					return nil
				}, nil
			})
			ordinary, err := WriteYaziFileAtResolvedAuthorityTracked(kind, cfg, "dracula", paths, accepted, parents, locker)
			var partial *PartialMutationError
			if ordinary.Path != "" || ordinary.Revision.Tracked() || ordinary.Directory != nil || ordinary.Parents != nil || !errors.Is(err, releaseErr) || errors.As(err, &partial) {
				t.Fatalf("ordinary=%+v error=%v partial=%+v, want raw no-op unlock error", ordinary, err, partial)
			}
			got, current, _, observeErr := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
			if observeErr != nil || current != accepted || !bytes.Equal(got, []byte(content)) {
				t.Fatalf("no-op target changed: revision=%+v want=%+v bytes=%q err=%v", current, accepted, got, observeErr)
			}
		})
	}
}

func TestAcceptedYaziPerFileWriterIgnoresAndPreservesHostileSiblings(t *testing.T) {
	for _, selectedKind := range []YaziFileKind{YaziFileKindMain, YaziFileKindKeymap, YaziFileKindTheme} {
		t.Run(string(selectedKind), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			t.Setenv("YAZI_CONFIG_HOME", "")
			paths, err := ResolveYaziConfigPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
			pathFor := map[YaziFileKind]string{YaziFileKindMain: paths.Main, YaziFileKindKeymap: paths.Keymap, YaziFileKindTheme: paths.Theme}
			nativeFor := map[YaziFileKind][]byte{
				YaziFileKindMain: []byte("[mgr]\nshow_hidden = true\n"), YaziFileKindKeymap: []byte("[mgr]\nkeymap = []\n"), YaziFileKindTheme: []byte("[mgr]\ncwd = { fg = \"#ffffff\" }\n"),
			}
			var siblings []YaziFileKind
			for _, kind := range []YaziFileKind{YaziFileKindMain, YaziFileKindKeymap, YaziFileKindTheme} {
				if kind != selectedKind {
					siblings = append(siblings, kind)
				}
			}
			sentinels := map[YaziFileKind][]byte{siblings[0]: nativeFor[siblings[0]], siblings[1]: []byte("[mgr\nmalformed = true\n")}
			before := map[YaziFileKind]safefile.Revision{}
			for kind, sentinel := range sentinels {
				if err := os.WriteFile(pathFor[kind], sentinel, 0o600); err != nil {
					t.Fatal(err)
				}
				observed := inspectYaziContentObservation(kind, pathFor[kind], sentinel)
				wantOwnership := YaziOwnershipNative
				if kind == siblings[1] {
					wantOwnership = YaziOwnershipMalformed
				}
				if observed.Ownership != wantOwnership {
					t.Fatalf("sibling %s ownership=%s, want %s", kind, observed.Ownership, wantOwnership)
				}
				rel, _ := filepath.Rel(home, pathFor[kind])
				_, revision, readErr := safefile.ReadWithin(home, filepath.ToSlash(rel))
				if readErr != nil {
					t.Fatal(readErr)
				}
				before[kind] = revision
			}
			selectedPath := pathFor[selectedKind]
			selectedRel, _ := filepath.Rel(home, selectedPath)
			_, accepted, parents, err := safefile.ObserveFileWithin(home, filepath.ToSlash(selectedRel))
			if err != nil {
				t.Fatal(err)
			}
			cfg := YaziConfig{Keymap: "emacs", ShowHidden: true, PreviewMode: "never", SortBy: "size", LineMode: "permissions", ScrollOff: 9}
			if _, err := WriteYaziFileAtResolvedAuthorityTracked(selectedKind, cfg, "dracula", paths, accepted, parents, operation.DefaultLocker); err != nil {
				t.Fatal(err)
			}
			for kind, sentinel := range sentinels {
				rel, _ := filepath.Rel(home, pathFor[kind])
				got, current, readErr := safefile.ReadWithin(home, filepath.ToSlash(rel))
				if readErr != nil || current != before[kind] || !bytes.Equal(got, sentinel) {
					t.Fatalf("hostile sibling %s changed: revision=%+v want=%+v bytes=%q err=%v", kind, current, before[kind], got, readErr)
				}
			}
		})
	}
}
