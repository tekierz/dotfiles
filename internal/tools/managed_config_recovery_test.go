package tools

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestAdoptionCleanupRefusesToEraseArtifactChangedByAnotherActor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "ghostty", "config.ghostty")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("other actor\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := withToolConfigLock(path, func(root, rel string) error {
		return rollbackManagedArtifact(root, rel, []byte("our bytes\n"), nil, false)
	})
	if err == nil {
		t.Fatal("cleanup erased an artifact whose bytes no longer matched")
	}
	if got := mustReadOwnershipFile(t, path); string(got) != "other actor\n" {
		t.Fatalf("changed artifact was erased: %q", got)
	}
}

func TestAdoptionCleanupRevisionRefusesReplacementAndSymlinkRaces(t *testing.T) {
	for _, test := range []struct {
		name    string
		replace func(t *testing.T, path, replacement string)
	}{
		{
			name: "regular replacement",
			replace: func(t *testing.T, path, replacement string) {
				t.Helper()
				if err := os.WriteFile(replacement, []byte("external replacement\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlink replacement",
			replace: func(t *testing.T, path, replacement string) {
				t.Helper()
				if err := os.WriteFile(replacement, []byte("external victim\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(replacement, path); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, "artifact")
			if err := os.WriteFile(path, []byte("our applied bytes\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, revision, err := safefile.ReadWithin(home, "artifact")
			if err != nil {
				t.Fatal(err)
			}
			replacement := filepath.Join(home, "replacement")
			test.replace(t, path, replacement)

			err = removeManagedArtifactAtRevision(home, "artifact", revision)
			if !errors.Is(err, safefile.ErrRevisionChanged) && !errors.Is(err, safefile.ErrSymlink) {
				t.Fatalf("removeManagedArtifactAtRevision error = %v, want revision/symlink refusal", err)
			}
			if test.name == "regular replacement" {
				if got := mustReadOwnershipFile(t, path); string(got) != "external replacement\n" {
					t.Fatalf("replacement was removed: %q", got)
				}
			} else {
				if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("replacement symlink changed: info=%v err=%v", info, err)
				}
				if got := mustReadOwnershipFile(t, replacement); string(got) != "external victim\n" {
					t.Fatalf("symlink victim changed: %q", got)
				}
			}
		})
	}
}
