package backup

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

// TestCreateZeroFiles verifies that a backup of a home directory containing
// none of the requested files returns an error (and writes no manifest),
// instead of silently reporting success (C4/C5). A "successful" empty backup
// would let the pre-install auto-backup claim a rollback point exists when it
// does not.
func TestCreateZeroFiles(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backup")

	files := []string{".zshrc", ".gitconfig"}

	count, err := Create(home, backupDir, files)
	if err == nil {
		t.Fatalf("Create with no capturable files = nil error, want error")
	}
	if count != 0 {
		t.Errorf("Create count = %d, want 0", count)
	}

	// No manifest should have been written for an empty backup.
	if _, statErr := os.Stat(filepath.Join(backupDir, ManifestName)); statErr == nil {
		t.Errorf("manifest was written for an empty backup; want none")
	}
}

func TestCreatePlanRecordsAbsentTargetsAndRollbackRemovesThem(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "nested", "sessions", "plan")
	targets := []Target{
		{RelPath: ".config/tool/config", Kind: TargetFile},
		{RelPath: ".config/new-tree", Kind: TargetDirectory},
	}
	count, err := CreatePlan(home, backupDir, targets)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("captured count = %d, want 0 existing targets", count)
	}
	manifest, err := os.ReadFile(filepath.Join(backupDir, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), "|no|file") || !strings.Contains(string(manifest), "|no|directory") {
		t.Fatalf("absence manifest = %s", manifest)
	}
	filePath := filepath.Join(home, ".config", "tool", "config")
	dirPath := filepath.Join(home, ".config", "new-tree")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("created"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "child"), []byte("created"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skipped) != 0 || len(result.Removed) != 2 {
		t.Fatalf("absence rollback result = %+v", result)
	}
	for _, path := range []string{filePath, dirPath} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("created target survived rollback %s: %v", path, err)
		}
	}
}

func TestCreatePlanRestoresExistingFileAndDirectoryExactly(t *testing.T) {
	home := t.TempDir()
	fileRel := ".toolrc"
	dirRel := ".config/tool-tree"
	filePath := filepath.Join(home, fileRel)
	dirPath := filepath.Join(home, filepath.FromSlash(dirRel))
	if err := os.WriteFile(filePath, []byte("original file\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dirPath, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dirPath, 0o710); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "nested", "value"), []byte("original tree\n"), 0o604); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(t.TempDir(), "plan")
	count, err := CreatePlan(home, backupDir, []Target{{fileRel, TargetFile}, {dirRel, TargetDirectory}})
	if err != nil || count != 2 {
		t.Fatalf("CreatePlan count=%d err=%v", count, err)
	}
	if err := os.WriteFile(filePath, []byte("mutated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dirPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "other"), []byte("mutated"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Restore(backupDir, home)
	if err != nil || len(result.Skipped) != 0 {
		t.Fatalf("Restore result=%+v err=%v", result, err)
	}
	data, _ := os.ReadFile(filePath)
	if string(data) != "original file\n" {
		t.Fatalf("restored file = %q", data)
	}
	assertCreateMode(t, filePath, 0o640)
	data, _ = os.ReadFile(filepath.Join(dirPath, "nested", "value"))
	if string(data) != "original tree\n" {
		t.Fatalf("restored directory content = %q", data)
	}
	assertCreateMode(t, dirPath, 0o710)
	assertCreateMode(t, filepath.Join(dirPath, "nested", "value"), 0o604)
}

func TestCreatePlanTrackedBindsFinalRootAndContents(t *testing.T) {
	for _, test := range []struct {
		name   string
		poison func(t *testing.T, backupDir string)
	}{
		{
			name: "content edit",
			poison: func(t *testing.T, backupDir string) {
				t.Helper()
				manifest := filepath.Join(backupDir, ManifestName)
				if err := os.WriteFile(manifest, []byte("poisoned\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "root replacement",
			poison: func(t *testing.T, backupDir string) {
				t.Helper()
				if err := os.Rename(backupDir, backupDir+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(backupDir, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("original\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			backupDir := filepath.Join(t.TempDir(), "plan")
			result, err := CreatePlanTracked(home, backupDir, []Target{{RelPath: ".zshrc", Kind: TargetFile}})
			if err != nil || result.Count != 1 || result.Directory == nil || !result.Parents.Tracked() {
				t.Fatalf("CreatePlanTracked result=%+v err=%v", result, err)
			}
			if err := ValidatePlanRoot(result); err != nil {
				t.Fatalf("fresh plan authority rejected: %v", err)
			}
			test.poison(t, backupDir)
			if err := ValidatePlanRoot(result); err == nil {
				t.Fatal("poisoned plan backup retained valid authority")
			}
		})
	}
}

func TestCreatePlanTrackedRejectsManifestMutationBeforeAuthorityCapture(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(t.TempDir(), "plan")
	backupCreateTestHooks.afterManifestWrite = func(path string) error {
		return os.WriteFile(path, []byte("poisoned\n"), 0o600)
	}
	t.Cleanup(func() { backupCreateTestHooks.afterManifestWrite = nil })

	if _, err := CreatePlanTracked(home, backupDir, []Target{{RelPath: ".zshrc", Kind: TargetFile}}); err == nil || !strings.Contains(err.Error(), "manifest differs") {
		t.Fatalf("manifest mutation error = %v", err)
	}
}

func TestCreatePlanRefusesSymlinkedSourceAndUnsafeScope(t *testing.T) {
	home := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(home, ".toolrc")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := CreatePlan(home, filepath.Join(t.TempDir(), "symlink"), []Target{{".toolrc", TargetFile}}); !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("symlink CreatePlan error = %v, want ErrSymlink", err)
	}
	traversalBackup := filepath.Join(t.TempDir(), "traversal")
	if _, err := CreatePlan(home, traversalBackup, []Target{{"../escape", TargetFile}}); err == nil {
		t.Fatal("CreatePlan accepted traversal target")
	}
	if _, err := os.Lstat(traversalBackup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid plan target created backup directory: %v", err)
	}
}

func TestCreatePlanRefusesSymlinkedBackupAncestorBelowHome(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, ".config")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	backupDir := filepath.Join(home, ".config", "dotfiles", "backups", "plan")
	_, err := CreatePlan(home, backupDir, []Target{{RelPath: ".toolrc", Kind: TargetFile}})
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("CreatePlan error = %v, want ErrSymlink", err)
	}
	if entries, readErr := os.ReadDir(outside); readErr != nil || len(entries) != 0 {
		t.Fatalf("symlink target was changed: entries=%v err=%v", entries, readErr)
	}
}

func assertCreateMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}

// TestCreateCapturesFilesAndManifest verifies a normal backup writes each
// present file, records a manifest, and returns the real success count.
func TestCreateCapturesFilesAndManifest(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backup")

	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export A=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// .gitconfig intentionally absent; should be skipped without failing.

	count, err := Create(home, backupDir, []string{".zshrc", ".gitconfig"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("Create count = %d, want 1", count)
	}

	// Manifest must exist and round-trip through ReadManifest to the one file.
	entries, err := ReadManifest(backupDir)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if len(entries) != 1 || entries[0].RelPath != ".zshrc" {
		t.Errorf("manifest entries = %+v, want single .zshrc", entries)
	}
	manifest, err := os.ReadFile(filepath.Join(backupDir, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(manifest), "# dotfiles-backup-manifest v2\n") {
		t.Fatalf("manifest does not declare v2 capabilities: %q", manifest)
	}
	assertCreateMode(t, filepath.Join(backupDir, EncodeName(".zshrc")), 0o600)
	assertCreateMode(t, filepath.Join(backupDir, ManifestName), 0o600)
}

func TestCreateRefusesSymlinkedCandidateInsteadOfClaimingPartialSuccess(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backup")
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, ".gitconfig")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	count, err := Create(home, backupDir, []string{".zshrc", ".gitconfig"})
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("Create count=%d error=%v, want ErrSymlink", count, err)
	}
	if count != 1 {
		t.Fatalf("count before refused candidate = %d, want 1", count)
	}
	if _, statErr := os.Lstat(filepath.Join(backupDir, ManifestName)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial backup received an authoritative manifest: %v", statErr)
	}
}

func TestCreateRefusesSymlinkedBackupAncestor(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, ".config")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	backupDir := filepath.Join(home, ".config", "dotfiles", "backups", "manual")
	if _, err := Create(home, backupDir, []string{".zshrc"}); !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("Create error=%v, want ErrSymlink", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("symlink destination changed: entries=%v err=%v", entries, err)
	}
}

func TestCreateRefusesSymlinkedBackupDirectory(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backup")
	if err := os.Symlink(outside, backupDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Create(home, backupDir, []string{".zshrc"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Create error=%v, want existing-directory refusal", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("symlink destination changed: entries=%v err=%v", entries, err)
	}
}

func TestCreateRefusesWritableBackupDirectory(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(t.TempDir(), "backup")
	if err := os.Mkdir(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backupDir, 0o770); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(home, backupDir, []string{".zshrc"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Create error=%v, want existing-directory refusal", err)
	}
}

func TestCreateProvesExistingAncestorsBeforeCreatingDescendants(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(home, ".config")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configDir, 0o770); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(configDir, "dotfiles", "backups", "manual")
	if _, err := Create(home, backupDir, []string{".zshrc"}); !errors.Is(err, safefile.ErrParentChanged) {
		t.Fatalf("Create error=%v, want ErrParentChanged", err)
	}
	if _, err := os.Lstat(filepath.Join(configDir, "dotfiles")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe ancestor was mutated before proof: %v", err)
	}
}

func TestBackupLocationRejectsCreatedRootReplacement(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backup")
	location, err := prepareBackupDirectory(home, backupDir)
	if err != nil {
		t.Fatal(err)
	}
	original := backupDir + "-original"
	if err := os.Rename(backupDir, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := location.writeFile("config", []byte("must not write\n"), 0o600); !errors.Is(err, safefile.ErrParentChanged) {
		t.Fatalf("write after root replacement error=%v, want ErrParentChanged", err)
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("replacement backup root was modified: entries=%v err=%v", entries, err)
	}
}

func TestBackupBootstrapRejectsPrefixReplacementBetweenIterations(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(home, ".config", "dotfiles", "backups", "plan")
	replaced := false
	backupCreateTestHooks.afterRootPrefix = func(prefix string) error {
		if prefix != ".config" || replaced {
			return nil
		}
		replaced = true
		configDir := filepath.Join(home, ".config")
		if err := os.Rename(configDir, configDir+"-original"); err != nil {
			return err
		}
		return os.Mkdir(configDir, 0o700)
	}
	t.Cleanup(func() { backupCreateTestHooks.afterRootPrefix = nil })

	if _, err := prepareBackupDirectory(home, backupDir); !errors.Is(err, safefile.ErrParentChanged) {
		t.Fatalf("prefix replacement error=%v, want ErrParentChanged", err)
	}
	if !replaced {
		t.Fatal("hostile prefix replacement hook did not run")
	}
	entries, err := os.ReadDir(filepath.Join(home, ".config"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("replacement prefix was mutated: entries=%v err=%v", entries, err)
	}
}

func TestBackupLocationRejectsNestedStorageParentReplacement(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backup")
	location, err := prepareBackupDirectory(home, backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := location.ensureParent("nested/tool/config"); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(backupDir, "nested", "tool")
	if err := os.Rename(nested, nested+"-original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := location.writeFile("nested/tool/config", []byte("must not write\n"), 0o600); !errors.Is(err, safefile.ErrParentChanged) {
		t.Fatalf("nested replacement error=%v, want ErrParentChanged", err)
	}
	entries, err := os.ReadDir(nested)
	if err != nil || len(entries) != 0 {
		t.Fatalf("replacement nested parent was modified: entries=%v err=%v", entries, err)
	}
}

func TestCapturePlanSourcesComparesCopiedDigestAndMode(t *testing.T) {
	t.Run("file digest", func(t *testing.T) {
		home := t.TempDir()
		location, err := prepareBackupDirectory(home, filepath.Join(t.TempDir(), "backup"))
		if err != nil {
			t.Fatal(err)
		}
		if err := location.writeFile("config", []byte("copied\n"), 0o640); err != nil {
			t.Fatal(err)
		}
		if err := location.writeFile(ManifestName, []byte("manifest\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		manifestData := []byte("manifest\n")
		_, err = location.capturePlanSources([]planSourceExpectation{{
			target: Target{RelPath: "config", Kind: TargetFile},
			digest: sha256.Sum256([]byte("different\n")),
			mode:   0o640,
		}}, manifestData)
		if err == nil || !strings.Contains(err.Error(), "differs from observed source") {
			t.Fatalf("file mismatch error=%v", err)
		}
	})

	t.Run("directory mode", func(t *testing.T) {
		home := t.TempDir()
		location, err := prepareBackupDirectory(home, filepath.Join(t.TempDir(), "backup"))
		if err != nil {
			t.Fatal(err)
		}
		sourceRoot := t.TempDir()
		if err := os.Mkdir(filepath.Join(sourceRoot, "tree"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceRoot, "tree", "value"), []byte("copied\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		snapshot, err := safefile.SnapshotDirectoryWithin(sourceRoot, "tree")
		if err != nil {
			t.Fatal(err)
		}
		if err := location.writeDirectory("tree", snapshot); err != nil {
			t.Fatal(err)
		}
		if err := location.writeFile(ManifestName, []byte("manifest\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		manifestData := []byte("manifest\n")
		_, err = location.capturePlanSources([]planSourceExpectation{{
			target: Target{RelPath: "tree", Kind: TargetDirectory},
			digest: snapshot.Digest(),
			mode:   0o700,
		}}, manifestData)
		if err == nil || !strings.Contains(err.Error(), "differs from observed source") {
			t.Fatalf("directory mismatch error=%v", err)
		}
	})
}

func TestCreateRefusesFlatStorageCollision(t *testing.T) {
	home := t.TempDir()
	first := filepath.Join(home, ".config", "a_b")
	second := filepath.Join(home, ".config", "a", "b")
	for _, path := range []string{first, second} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(path), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	backupDir := filepath.Join(t.TempDir(), "backup")
	if _, err := Create(home, backupDir, []string{".config/a_b", ".config/a/b"}); err == nil || !strings.Contains(err.Error(), "collide") {
		t.Fatalf("Create collision error=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(backupDir, ManifestName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("collision backup received a manifest: %v", err)
	}
	if _, err := os.Lstat(backupDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("predictable collision created backup directory: %v", err)
	}
}

func TestCreateRefusesForeignOwnedSourceWhenChownAvailable(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(source, []byte("foreign\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	foreignUID := os.Geteuid() + 1
	if foreignUID == 0 {
		foreignUID++
	}
	if err := os.Chown(source, foreignUID, os.Getegid()); err != nil {
		t.Skipf("changing test ownership is unavailable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chown(source, os.Geteuid(), os.Getegid()) })

	backupDir := filepath.Join(t.TempDir(), "backup")
	if _, err := Create(home, backupDir, []string{".zshrc"}); !errors.Is(err, safefile.ErrRevisionChanged) {
		t.Fatalf("Create error=%v, want ErrRevisionChanged", err)
	}
}
