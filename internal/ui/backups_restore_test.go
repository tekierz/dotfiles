package ui

import (
	"strings"
	"testing"
)

// TestRestoreReportingReflectsSkipped verifies the Backups screen surfaces
// skipped files instead of reporting a partial/failed restore as plain success
// (C3). A restore where files were skipped must show the skipped count and use
// the non-success (warning) wording, mirroring the CLI.
func TestRestoreReportingReflectsSkipped(t *testing.T) {
	t.Run("all files skipped is not reported as clean success", func(t *testing.T) {
		ctx := newGoldenContext(t)
		s := NewBackupsScreen(ctx)

		s.Update(backupRestoreDoneMsg{name: "2026-01-01_00-00-00", count: 0, skipped: 3})

		got := ctx.app.backupStatus
		if !strings.Contains(got, "skipped") {
			t.Errorf("backupStatus = %q, want it to mention skipped files", got)
		}
		if !strings.Contains(got, "3") {
			t.Errorf("backupStatus = %q, want it to include the skipped count 3", got)
		}
	})

	t.Run("partial restore reports both counts", func(t *testing.T) {
		ctx := newGoldenContext(t)
		s := NewBackupsScreen(ctx)

		s.Update(backupRestoreDoneMsg{name: "bk", count: 2, skipped: 1})

		got := ctx.app.backupStatus
		if !strings.Contains(got, "2") || !strings.Contains(got, "skipped") {
			t.Errorf("backupStatus = %q, want both restored and skipped counts", got)
		}
	})

	t.Run("clean restore keeps the simple success message", func(t *testing.T) {
		ctx := newGoldenContext(t)
		s := NewBackupsScreen(ctx)

		s.Update(backupRestoreDoneMsg{name: "bk", count: 5, skipped: 0})

		got := ctx.app.backupStatus
		if strings.Contains(got, "skipped") {
			t.Errorf("backupStatus = %q, did not expect 'skipped' on a clean restore", got)
		}
		if !strings.Contains(got, "Restored 5") {
			t.Errorf("backupStatus = %q, want clean 'Restored 5' message", got)
		}
	})
}
