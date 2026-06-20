package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// extractEmbeddedCLI pulls the generated `dotfiles` management CLI out of the
// bin/dotfiles-setup heredoc (between the DOTFILES_CLI_EOF markers). This is the
// exact text written to ~/.local/bin/dotfiles at install time, so testing it
// exercises the installed CLI rather than a re-implementation.
func extractEmbeddedCLI(t *testing.T) string {
	t.Helper()
	// Walk up from this test file to the repo root, then read bin/dotfiles-setup.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile))) // cmd/dotfiles -> cmd -> root
	setupPath := filepath.Join(repoRoot, "bin", "dotfiles-setup")
	data, err := os.ReadFile(setupPath)
	if err != nil {
		t.Fatalf("read %s: %v", setupPath, err)
	}

	const startMarker = "cat > ~/.local/bin/dotfiles << 'DOTFILES_CLI_EOF'"
	const endMarker = "DOTFILES_CLI_EOF"
	src := string(data)
	startIdx := strings.Index(src, startMarker)
	if startIdx < 0 {
		t.Fatal("could not find embedded CLI heredoc start marker")
	}
	// Body begins on the line after the start marker.
	bodyStart := startIdx + len(startMarker)
	bodyStart += strings.IndexByte(src[bodyStart:], '\n') + 1
	endIdx := strings.Index(src[bodyStart:], "\n"+endMarker)
	if endIdx < 0 {
		t.Fatal("could not find embedded CLI heredoc end marker")
	}
	return src[bodyStart : bodyStart+endIdx]
}

// TestEmbeddedCLIDefinesRestoreGuard asserts is_safe_restore_path is defined
// inside the heredoc body. The embedded restore_backup calls it for every
// manifest line; if it is missing the (no set -e/-u) CLI treats the resulting
// command-not-found as "unsafe" and skips every file (the C22 regression).
func TestEmbeddedCLIDefinesRestoreGuard(t *testing.T) {
	body := extractEmbeddedCLI(t)
	if !strings.Contains(body, "is_safe_restore_path()") {
		t.Fatal("is_safe_restore_path() is not defined inside the embedded CLI heredoc; " +
			"`dotfiles restore` will skip every file (C22)")
	}
}

// TestEmbeddedCLIRestoreActuallyRestores is the real smoke test: it extracts the
// generated CLI, lays down a fixture backup + manifest under a temp HOME, runs
// `dotfiles restore <session>` and asserts the files are ACTUALLY restored (not
// all-skipped). This reproduces and guards against C22.
func TestEmbeddedCLIRestoreActuallyRestores(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("embedded bash CLI not applicable on Windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	body := extractEmbeddedCLI(t)

	home := t.TempDir()
	cliPath := filepath.Join(home, "dotfiles")
	if err := os.WriteFile(cliPath, []byte(body), 0o755); err != nil {
		t.Fatalf("write CLI: %v", err)
	}

	// Build a fixture backup session under $HOME/.config/dotfiles/backups.
	sessionDir := filepath.Join(home, ".config", "dotfiles", "backups", "20240101-000000")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}

	// A previously backed-up file whose contents must be restored.
	backupFile := filepath.Join(sessionDir, "zshrc.backup")
	const restoredContent = "ORIGINAL ZSHRC CONTENTS\n"
	if err := os.WriteFile(backupFile, []byte(restoredContent), 0o644); err != nil {
		t.Fatalf("write backup file: %v", err)
	}

	// The destination the manifest points at (must be restored from backupFile).
	destFile := filepath.Join(home, ".zshrc")
	// Pre-existing content that restore must overwrite.
	if err := os.WriteFile(destFile, []byte("MODIFIED BY SETUP\n"), 0o644); err != nil {
		t.Fatalf("seed dest: %v", err)
	}

	// manifest format: original|backup|existed|type
	manifest := destFile + "|" + backupFile + "|yes|file\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "manifest.txt"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := exec.Command("bash", cliPath, "restore", "20240101-000000")
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
	)
	cmd.Stdin = strings.NewReader("y\n") // answer the confirmation prompt
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("restore failed: %v\noutput:\n%s", err, out)
	}
	output := string(out)

	if strings.Contains(output, "Skipping unsafe restore path") {
		t.Fatalf("restore skipped paths as unsafe (C22 regression):\n%s", output)
	}
	if strings.Contains(output, "0 files restored") {
		t.Fatalf("restore reported 0 files restored (C22 regression):\n%s", output)
	}

	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("read restored dest: %v", err)
	}
	if string(got) != restoredContent {
		t.Fatalf("destination was not restored to backup contents.\nwant: %q\ngot:  %q\noutput:\n%s",
			restoredContent, string(got), output)
	}
}
