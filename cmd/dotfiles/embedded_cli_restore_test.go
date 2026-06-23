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
	if runtime.GOOS == osWindows {
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

// Shared literals for the hardening regression tests below. goconst/prealloc are
// NOT excluded for _test.go, so repeated strings live here as consts.
const (
	restoreSession   = "20240101-000000"
	restoreConfirm   = "y\n"
	osWindows        = "windows"
	unsafePathMsg    = "Skipping unsafe restore path"
	outOfTreeMsg     = "Skipping manifest entry with out-of-tree source"
	manifestFileMeta = "|yes|file\n"
)

// writeEmbeddedCLI extracts the generated CLI and writes it into a fresh temp
// HOME, returning (home, cliPath). It also guards the windows/no-bash skips so
// each hardening test reads the same way as TestEmbeddedCLIRestoreActuallyRestores.
func writeEmbeddedCLI(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == osWindows {
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
	return home, cliPath
}

// runRestore invokes `bash <cli> restore <args...>` with the fixture HOME and an
// affirmative confirmation on stdin, mirroring the smoke test's invocation.
func runRestore(t *testing.T, home, cliPath string, args ...string) ([]byte, error) {
	t.Helper()
	cmdArgs := append([]string{cliPath, "restore"}, args...)
	cmd := exec.Command("bash", cmdArgs...)
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
	)
	cmd.Stdin = strings.NewReader(restoreConfirm)
	out, err := cmd.CombinedOutput()
	return out, err
}

// TestEmbeddedCLIRestoreRejectsSymlinkEscape guards the is_safe_restore_path
// hardening. The destination "<home>/escape/loot" is lexically under $HOME and
// contains no "..", so the OLD lexical HasPrefix check accepted it and the restore
// copied the backup THROUGH the symlinked "escape" dir into a directory OUTSIDE
// $HOME. The fixed guard resolves the realpath of the deepest existing ancestor
// ("escape" -> outsideDir) and rejects it because it escapes the canonical home.
func TestEmbeddedCLIRestoreRejectsSymlinkEscape(t *testing.T) {
	home, cliPath := writeEmbeddedCLI(t)

	sessionDir := filepath.Join(home, ".config", "dotfiles", "backups", restoreSession)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}

	// A directory OUTSIDE $HOME that the attacker wants the restore to write into.
	outsideDir := t.TempDir()

	// A symlinked intermediate dir inside $HOME pointing OUTSIDE $HOME.
	escapeLink := filepath.Join(home, "escape")
	if err := os.Symlink(outsideDir, escapeLink); err != nil {
		t.Fatalf("symlink escape: %v", err)
	}

	backupFile := filepath.Join(sessionDir, "loot.backup")
	const lootContent = "ATTACKER CONTROLLED PAYLOAD\n"
	if err := os.WriteFile(backupFile, []byte(lootContent), 0o644); err != nil {
		t.Fatalf("write backup file: %v", err)
	}

	// dest traverses the symlink: <home>/escape/loot resolves to <outsideDir>/loot.
	destFile := filepath.Join(escapeLink, "loot")
	manifest := destFile + "|" + backupFile + manifestFileMeta
	if err := os.WriteFile(filepath.Join(sessionDir, "manifest.txt"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	out, err := runRestore(t, home, cliPath, restoreSession)
	if err != nil {
		t.Fatalf("restore returned error: %v\noutput:\n%s", err, out)
	}
	output := string(out)

	// The file must NOT have been written into the outside directory.
	escapedFile := filepath.Join(outsideDir, "loot")
	if _, statErr := os.Stat(escapedFile); !os.IsNotExist(statErr) {
		t.Fatalf("symlink escape NOT blocked: %s exists (err=%v)\noutput:\n%s",
			escapedFile, statErr, output)
	}
	if !strings.Contains(output, unsafePathMsg) {
		t.Fatalf("expected %q in output for symlinked-intermediate escape:\n%s",
			unsafePathMsg, output)
	}
}

// TestEmbeddedCLIRestoreRejectsTraversingSession guards the session-name check.
// A traversing session name like "../evil" must be rejected up front so a crafted
// name cannot escape the backups directory to read an attacker-controlled
// manifest. The fixed code prints "Invalid backup session name" and exits non-zero
// before ever opening a manifest; nothing is restored.
func TestEmbeddedCLIRestoreRejectsTraversingSession(t *testing.T) {
	home, cliPath := writeEmbeddedCLI(t)

	// Plant an attacker-controlled manifest one level ABOVE the backups dir, where
	// "../evil" would resolve to, pointing at a writable dest inside $HOME. If the
	// session-name guard were missing, this manifest would be honored.
	backupsParent := filepath.Join(home, ".config", "dotfiles")
	evilDir := filepath.Join(backupsParent, "evil")
	if err := os.MkdirAll(evilDir, 0o755); err != nil {
		t.Fatalf("mkdir evil: %v", err)
	}
	evilBackup := filepath.Join(evilDir, "payload.backup")
	if err := os.WriteFile(evilBackup, []byte("EVIL\n"), 0o644); err != nil {
		t.Fatalf("write evil backup: %v", err)
	}
	pwned := filepath.Join(home, ".pwned")
	manifest := pwned + "|" + evilBackup + manifestFileMeta
	if err := os.WriteFile(filepath.Join(evilDir, "manifest.txt"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write evil manifest: %v", err)
	}

	// backups/../evil -> .config/dotfiles/evil, the dir we just planted.
	out, err := runRestore(t, home, cliPath, "../evil")
	output := string(out)
	if err == nil {
		t.Fatalf("expected non-zero exit for traversing session name:\n%s", output)
	}
	if !strings.Contains(output, "Invalid backup session name") {
		t.Fatalf("expected 'Invalid backup session name' for %q:\n%s", "../evil", output)
	}
	if _, statErr := os.Stat(pwned); !os.IsNotExist(statErr) {
		t.Fatalf("traversing session restored something: %s exists\noutput:\n%s", pwned, output)
	}
}

// TestEmbeddedCLIRestoreRejectsOutOfTreeSource guards the manifest-SOURCE check.
// The destination is a legitimate path under $HOME, but the manifest SOURCE points
// OUTSIDE the session's backup dir (here /etc/hostname). The fixed code refuses to
// copy any source that does not live inside "$backup_dir/", so the destination is
// never written and the out-of-tree warning is emitted.
func TestEmbeddedCLIRestoreRejectsOutOfTreeSource(t *testing.T) {
	home, cliPath := writeEmbeddedCLI(t)

	sessionDir := filepath.Join(home, ".config", "dotfiles", "backups", restoreSession)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}

	// A valid, in-home destination; only the SOURCE is out of tree.
	destFile := filepath.Join(home, ".hostname")
	// An out-of-tree source outside the session backup dir.
	const outOfTreeSource = "/etc/hostname"
	manifest := destFile + "|" + outOfTreeSource + manifestFileMeta
	if err := os.WriteFile(filepath.Join(sessionDir, "manifest.txt"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	out, err := runRestore(t, home, cliPath, restoreSession)
	if err != nil {
		t.Fatalf("restore returned error: %v\noutput:\n%s", err, out)
	}
	output := string(out)

	if !strings.Contains(output, outOfTreeMsg) {
		t.Fatalf("expected %q for out-of-tree manifest source:\n%s", outOfTreeMsg, output)
	}
	// The dest must NOT have been created from the out-of-tree source.
	if _, statErr := os.Stat(destFile); !os.IsNotExist(statErr) {
		t.Fatalf("out-of-tree source was copied to %s\noutput:\n%s", destFile, output)
	}
}

// TestEmbeddedCLIRestoreReportsCorrectCounts guards against the `((restored++))`
// post-increment off-by-one: `cp && ((restored++)) || ((errors++))` evaluates the
// OLD value of restored, so on the FIRST success (restored=0) the increment
// returns exit status 1 and the `|| ((errors++))` branch ALSO fires, inflating
// the error count. With an all-success multi-file manifest the correct report is
// "N files restored, 0 errors"; the buggy idiom reports a spurious error.
func TestEmbeddedCLIRestoreReportsCorrectCounts(t *testing.T) {
	if runtime.GOOS == osWindows {
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

	sessionDir := filepath.Join(home, ".config", "dotfiles", "backups", "20240101-000000")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}

	// Lay down several backup files all under $HOME so the restore guard accepts
	// them and every copy succeeds. The first success is the one that triggers
	// the off-by-one, so a multi-file all-success manifest is the key fixture.
	var manifest strings.Builder
	const fileCount = 3
	for i := 0; i < fileCount; i++ {
		name := []string{"zshrc", "tmux.conf", "gitconfig"}[i]
		backupFile := filepath.Join(sessionDir, name+".backup")
		content := "ORIGINAL " + name + " CONTENTS\n"
		if err := os.WriteFile(backupFile, []byte(content), 0o644); err != nil {
			t.Fatalf("write backup file %s: %v", name, err)
		}
		destFile := filepath.Join(home, "."+name)
		manifest.WriteString(destFile + "|" + backupFile + "|yes|file\n")
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "manifest.txt"), []byte(manifest.String()), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := exec.Command("bash", cliPath, "restore", "20240101-000000")
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
	)
	cmd.Stdin = strings.NewReader("y\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("restore failed: %v\noutput:\n%s", err, out)
	}
	output := string(out)

	// All files were valid and under $HOME, so there must be zero errors.
	if !strings.Contains(output, "3 files restored") {
		t.Fatalf("expected '3 files restored' in output (off-by-one or miscount):\n%s", output)
	}
	if strings.Contains(output, "errors occurred") {
		t.Fatalf("restore reported spurious errors on all-success restore (((restored++)) off-by-one):\n%s", output)
	}
}
