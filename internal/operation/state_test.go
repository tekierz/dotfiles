package operation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestStateSubdirectoryReturnsPrivateChildWithoutCreatingIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	path, err := StateSubdirectory("backups")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "state", "dotfiles", "backups")
	if path != want {
		t.Fatalf("StateSubdirectory = %q, want %q", path, want)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("StateSubdirectory created state: %v", err)
	}
	for _, invalid := range []string{"", ".", "..", "nested/path", "two words"} {
		if _, err := StateSubdirectory(invalid); err == nil {
			t.Fatalf("StateSubdirectory(%q) accepted invalid name", invalid)
		}
	}
}

func TestCreateAndRemoveStateStagingDirectoryUsesExactPrivateSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	path, authority, err := CreateStateStagingDirectoryTracked("neovim-clone")
	if err != nil {
		t.Fatal(err)
	}
	stagingRoot := filepath.Join(home, ".local", "state", "dotfiles", "staging")
	if filepath.Dir(path) != stagingRoot || !strings.HasPrefix(filepath.Base(path), ".stage-") {
		t.Fatalf("staging path = %q, want direct randomized child of %q", path, stagingRoot)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("staging directory info = %v err=%v", info, err)
	}
	if err := os.WriteFile(filepath.Join(path, "config"), []byte("staged"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := SnapshotStateStagingDirectory(path, authority)
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveStateStagingDirectoryAuthorized(path, authority, expected); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging directory remains: %v", err)
	}
}

func TestSnapshotStateStagingDirectoryRejectsReplacement(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	path, created, err := CreateStateStagingDirectoryTracked("replacement")
	if err != nil {
		t.Fatal(err)
	}
	moved := path + ".moved"
	if err := os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := SnapshotStateStagingDirectory(path, created); !errors.Is(err, safefile.ErrDirectoryChanged) {
		t.Fatalf("replacement snapshot error = %v, want ErrDirectoryChanged", err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("replacement was not preserved: info=%v err=%v", info, err)
	}
}

func TestSnapshotStateStagingDirectoryRejectsModeChange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	path, authority, err := CreateStateStagingDirectoryTracked("mode-change")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := SnapshotStateStagingDirectory(path, authority); !errors.Is(err, safefile.ErrDirectoryChanged) {
		t.Fatalf("mode change error = %v, want ErrDirectoryChanged", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o777 {
		t.Fatalf("changed staging directory was not preserved: info=%v err=%v", info, err)
	}
}

func TestStateStagingUsesConfiguredXDGStateRoot(t *testing.T) {
	home := t.TempDir()
	state := filepath.Join(t.TempDir(), "state", "root")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", state)
	path, created, err := CreateStateStagingDirectoryTracked("xdg")
	if err != nil {
		t.Fatal(err)
	}
	wantParent := filepath.Join(state, "dotfiles", "staging")
	if filepath.Dir(path) != wantParent {
		t.Fatalf("staging parent = %q, want %q", filepath.Dir(path), wantParent)
	}
	final, err := SnapshotStateStagingDirectory(path, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveStateStagingDirectoryAuthorized(path, created, final); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".local")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("XDG staging unexpectedly created default state: %v", err)
	}
}

func TestStateStagingAuthorityRejectsParentGraft(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	path, authority, err := CreateStateStagingDirectoryTracked("parent-graft")
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(path)
	moved := parent + ".moved"
	if err := os.Rename(parent, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(moved, filepath.Base(path)), path); err != nil {
		t.Fatal(err)
	}
	if _, err := SnapshotStateStagingDirectory(path, authority); !errors.Is(err, safefile.ErrParentChanged) {
		t.Fatalf("grafted staging error = %v, want ErrParentChanged", err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("grafted leaf was not preserved: info=%v err=%v", info, err)
	}
}

func TestStateStagingDescendantAuthorityRejectsLeafReplacement(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	path, authority, err := CreateStateStagingDirectoryTracked("descendant")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := StateStagingDescendantAuthority(path, authority, "config"); err != nil {
		t.Fatalf("initial descendant authority: %v", err)
	}
	if err := os.Rename(path, path+".moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config"), []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := StateStagingDescendantAuthority(path, authority, "config"); !errors.Is(err, safefile.ErrParentChanged) {
		t.Fatalf("replacement descendant error = %v, want ErrParentChanged", err)
	}
	data, err := os.ReadFile(filepath.Join(path, "config"))
	if err != nil || string(data) != "replacement" {
		t.Fatalf("replacement descendant changed: data=%q err=%v", data, err)
	}
}

func TestStateStagingDescendantAuthorityRejectsRootModeChange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	path, authority, err := CreateStateStagingDirectoryTracked("descendant-mode")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config"), []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := StateStagingDescendantAuthority(path, authority, "config"); !errors.Is(err, safefile.ErrParentChanged) {
		t.Fatalf("staging root mode error = %v, want ErrParentChanged", err)
	}
	data, err := os.ReadFile(filepath.Join(path, "config"))
	if err != nil || string(data) != "preserve" {
		t.Fatalf("staging descendant changed: data=%q err=%v", data, err)
	}
}

func TestStateStagingDescendantAuthorityRejectsAncestorModeChange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	path, authority, err := CreateStateStagingDirectoryTracked("ancestor-mode")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config"), []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	stateRoot := filepath.Join(home, ".local", "state", "dotfiles")
	if err := os.Chmod(stateRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := StateStagingDescendantAuthority(path, authority, "config"); !errors.Is(err, safefile.ErrParentChanged) {
		t.Fatalf("staging ancestor mode error = %v, want ErrParentChanged", err)
	}
	data, err := os.ReadFile(filepath.Join(path, "config"))
	if err != nil || string(data) != "preserve" {
		t.Fatalf("staging descendant changed: data=%q err=%v", data, err)
	}
}

func TestStateStagingAuthorityRejectsStateRootReplacement(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	path, authority, err := CreateStateStagingDirectoryTracked("state-root")
	if err != nil {
		t.Fatal(err)
	}
	stateRoot := filepath.Join(home, ".local", "state", "dotfiles")
	moved := stateRoot + ".moved"
	if err := os.Rename(stateRoot, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(moved, "staging", filepath.Base(path)), path); err != nil {
		t.Fatal(err)
	}
	if _, err := SnapshotStateStagingDirectory(path, authority); !errors.Is(err, safefile.ErrParentChanged) {
		t.Fatalf("replaced state root error = %v, want ErrParentChanged", err)
	}
}

func TestRemoveStateStagingDirectoryPreservesExternalEdit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	path, authority, err := CreateStateStagingDirectoryTracked("tpm-clone")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := SnapshotStateStagingDirectory(path, authority)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "external"), []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveStateStagingDirectoryAuthorized(path, authority, expected); !errors.Is(err, safefile.ErrDirectoryChanged) {
		t.Fatalf("changed staging cleanup error = %v, want ErrDirectoryChanged", err)
	}
	data, err := os.ReadFile(filepath.Join(path, "external"))
	if err != nil || string(data) != "preserve" {
		t.Fatalf("external edit was not preserved: data=%q err=%v", data, err)
	}
}

func TestRemoveStateStagingDirectoryRejectsReplacementSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	path, authority, err := CreateStateStagingDirectoryTracked("cleanup-replacement")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "preserve"), []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	replacement, err := safefile.SnapshotDirectoryWithin(authority.root, authority.rel)
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveStateStagingDirectoryAuthorized(path, authority, replacement); !errors.Is(err, safefile.ErrDirectoryChanged) {
		t.Fatalf("replacement cleanup error = %v, want ErrDirectoryChanged", err)
	}
	data, err := os.ReadFile(filepath.Join(path, "preserve"))
	if err != nil || string(data) != "replacement" {
		t.Fatalf("replacement staging tree was not preserved: data=%q err=%v", data, err)
	}
}

func TestRemoveStateStagingDirectoryRejectsOutsidePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	outside := filepath.Join(home, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	expected, err := safefile.SnapshotDirectoryWithin(home, "outside")
	if err != nil {
		t.Fatal(err)
	}
	_, authority, err := CreateStateStagingDirectoryTracked("outside-rejection")
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveStateStagingDirectoryAuthorized(outside, authority, expected); err == nil {
		t.Fatal("outside path was accepted for staging cleanup")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside path was changed: %v", err)
	}
}
