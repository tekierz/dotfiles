package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreExpectedPreservesExternalEditAfterMutation(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backups", "selected")
	path := filepath.Join(home, "config")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlan(home, backupDir, []Target{{RelPath: "config", Kind: TargetFile}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("operation post-state\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureExpectedState(home, Target{RelPath: "config", Kind: TargetFile})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external edit after mutation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := RestoreExpected(backupDir, home, map[string]ExpectedState{"config": expected})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skipped) != 1 || len(result.Restored) != 0 {
		t.Fatalf("conditional restore result = %+v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "external edit after mutation\n" {
		t.Fatalf("external edit overwritten: %q err=%v", got, err)
	}
}

func TestRestoreExpectedPreservesExternallyChangedCreatedTarget(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backups", "selected")
	if _, err := CreatePlan(home, backupDir, []Target{{RelPath: "created", Kind: TargetFile}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "created")
	if err := os.WriteFile(path, []byte("operation creation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureExpectedState(home, Target{RelPath: "created", Kind: TargetFile})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external replacement\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := RestoreExpected(backupDir, home, map[string]ExpectedState{"created": expected})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skipped) != 1 || len(result.Removed) != 0 {
		t.Fatalf("conditional removal result = %+v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "external replacement\n" {
		t.Fatalf("external replacement removed: %q err=%v", got, err)
	}
}

func TestRestoreExpectedFailsClosedWithoutPostWriteCapture(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backups", "selected")
	if err := os.WriteFile(filepath.Join(home, "config"), []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlan(home, backupDir, []Target{{RelPath: "config", Kind: TargetFile}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config"), []byte("current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := RestoreExpected(backupDir, home, map[string]ExpectedState{"config": {Attempted: true}})
	if err != nil {
		t.Fatal(err)
	}
	if reason := result.Skipped["config"]; !strings.Contains(reason, "no proven post-write state") {
		t.Fatalf("missing capture skip reason = %q; result=%+v", reason, result)
	}
}

func TestRestoreExpectedEmptyAuthorityLeavesUnexecutedTargetsUncounted(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backups", "selected")
	path := filepath.Join(home, "config")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlan(home, backupDir, []Target{{RelPath: "config", Kind: TargetFile}}); err != nil {
		t.Fatal(err)
	}
	result, err := RestoreExpected(backupDir, home, map[string]ExpectedState{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Restored) != 0 || len(result.Removed) != 0 || len(result.Skipped) != 0 {
		t.Fatalf("unexecuted target affected rollback accounting: %+v", result)
	}
}

func TestRestoreExpectedDoesNotClaimRemovalWhenTargetStayedAbsent(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backups", "selected")
	target := Target{RelPath: "never-created", Kind: TargetFile}
	if _, err := CreatePlan(home, backupDir, []Target{target}); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureExpectedState(home, target)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RestoreExpected(backupDir, home, map[string]ExpectedState{"never-created": expected})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 0 || len(result.Skipped) != 0 {
		t.Fatalf("unchanged absent target was misreported: %+v", result)
	}
}
