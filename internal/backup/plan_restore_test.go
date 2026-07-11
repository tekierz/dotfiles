//go:build darwin || linux

package backup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreExpectedPlanReadsExactAuthorizedBackupSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	live := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(live, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(home, ".state", "backup")
	plan, err := CreatePlanTracked(home, backupDir, []Target{{RelPath: ".zshrc", Kind: TargetFile}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, []byte("installed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	post, err := CaptureExpectedState(home, Target{RelPath: ".zshrc", Kind: TargetFile})
	if err != nil {
		t.Fatal(err)
	}
	restorePlanTestHooks.afterRootValidation = func(PlanResult) error {
		return os.WriteFile(filepath.Join(backupDir, ".zshrc"), []byte("poisoned\n"), 0o600)
	}
	t.Cleanup(func() { restorePlanTestHooks.afterRootValidation = nil })
	result, err := RestoreExpectedPlan(plan, home, map[string]ExpectedState{".zshrc": post})
	if err == nil {
		t.Fatalf("poisoned exact source was accepted: %+v", result)
	}
	data, readErr := os.ReadFile(live)
	if readErr != nil || string(data) != "installed\n" {
		t.Fatalf("live config changed before source refusal: %q, %v", data, readErr)
	}
}

func TestRestoreExpectedPlanRestoresExactOriginalAndRemovesAcceptedCreation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("original\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(home, ".state", "backup")
	plan, err := CreatePlanTracked(home, backupDir, []Target{
		{RelPath: ".zshrc", Kind: TargetFile},
		{RelPath: ".config/new/config", Kind: TargetFile},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("installed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "new"), 0o700); err != nil {
		t.Fatal(err)
	}
	created := filepath.Join(home, ".config", "new", "config")
	if err := os.WriteFile(created, []byte("created\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	zshPost, err := CaptureExpectedState(home, Target{RelPath: ".zshrc", Kind: TargetFile})
	if err != nil {
		t.Fatal(err)
	}
	createdPost, err := CaptureExpectedState(home, Target{RelPath: ".config/new/config", Kind: TargetFile})
	if err != nil {
		t.Fatal(err)
	}
	result, err := RestoreExpectedPlan(plan, home, map[string]ExpectedState{
		".zshrc":             zshPost,
		".config/new/config": createdPost,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skipped) != 0 || len(result.Restored) != 1 || len(result.Removed) != 1 {
		t.Fatalf("restore result = %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil || string(data) != "original\n" {
		t.Fatalf("restored config = %q, %v", data, err)
	}
	if info, err := os.Stat(filepath.Join(home, ".zshrc")); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("restored mode = %v, %v", info, err)
	}
	if _, err := os.Lstat(created); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("accepted creation remains: %v", err)
	}
}
