package main

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/backup"
	"github.com/tekierz/dotfiles/internal/config"
)

func TestRunUninstallForceWithoutKeepRetainsUnownedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinked XDG ancestor fixture requires Unix semantics")
	}

	workspace := t.TempDir()
	home := filepath.Join(workspace, "home")
	externalXDG := filepath.Join(workspace, "external-xdg")
	if err := os.MkdirAll(filepath.Join(home, ".local", "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(externalXDG, "dotfiles"), 0o700); err != nil {
		t.Fatal(err)
	}
	xdgLink := filepath.Join(home, "linked-xdg")
	if err := os.Symlink(externalXDG, xdgLink); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdgLink)

	marker := filepath.Join(externalXDG, "dotfiles", "user-owned.json")
	if err := os.WriteFile(marker, []byte("do not delete"), 0o600); err != nil {
		t.Fatal(err)
	}

	var binaryPaths []string
	for _, name := range []string{"dotfiles", "dotfiles-tui", "dotfiles-setup", "hk", "caff", "sshh"} {
		path := filepath.Join(home, ".local", "bin", name)
		if err := os.WriteFile(path, []byte("user-owned "+name), 0o700); err != nil {
			t.Fatal(err)
		}
		binaryPaths = append(binaryPaths, path)
	}

	stdout, stderr := captureUninstallOutput(t, func() {
		// Both keep flags are false and force is true: even the most permissive
		// legacy invocation must remain unable to delete path-based candidates.
		if err := runUninstall(false, false, true, true); err != nil {
			t.Fatalf("no-restore uninstall: %v", err)
		}
	})

	for _, path := range binaryPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("retained candidate %s was removed: %v", path, err)
		}
		if !strings.HasPrefix(string(data), "user-owned ") {
			t.Fatalf("retained candidate %s was changed: %q", path, data)
		}
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "do not delete" {
		t.Fatalf("config behind symlinked XDG ancestor changed: data=%q err=%v", data, err)
	}
	if info, err := os.Lstat(xdgLink); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("XDG symlink was changed: info=%v err=%v", info, err)
	}

	output := stdout + "\n" + stderr
	for _, required := range []string{
		"Automatic deletion is disabled in this release.",
		"cannot yet prove artifact ownership",
		"brew uninstall tekierz/tap/dotfiles",
		filepath.Join(xdgLink, "dotfiles"),
		filepath.Join(home, ".local", "bin", "sshh"),
		"verify ownership before manual removal",
	} {
		if !strings.Contains(output, required) {
			t.Errorf("uninstall output missing %q:\n%s", required, output)
		}
	}
	for _, forbidden := range []string{
		"Removed:",
		"Removing binaries",
		"Removing configuration directory",
		"Configuration directory removed",
		"Uninstall complete",
	} {
		if strings.Contains(output, forbidden) {
			t.Errorf("uninstall output falsely claims deletion with %q:\n%s", forbidden, output)
		}
	}
}

func TestRunUninstallContainsNoPathBasedRemovalCalls(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	mainPath := filepath.Join(filepath.Dir(thisFile), "main.go")
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, mainPath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", mainPath, err)
	}

	var uninstall *ast.FuncDecl
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "runUninstall" {
			uninstall = fn
			break
		}
	}
	if uninstall == nil {
		t.Fatal("runUninstall declaration not found")
	}

	ast.Inspect(uninstall.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || pkg.Name != "os" {
			return true
		}
		if selector.Sel.Name == "Remove" || selector.Sel.Name == "RemoveAll" {
			t.Errorf("runUninstall must not call path-based os.%s at %s", selector.Sel.Name, fset.Position(call.Pos()))
		}
		return true
	})
}

func TestUninstallHelpDoesNotPromiseAutomaticDeletion(t *testing.T) {
	help := uninstallCmd.Short + "\n" + uninstallCmd.Long
	for _, flagName := range []string{"keep-config", "keep-binaries", "force"} {
		flag := uninstallCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Fatalf("uninstall flag %q not registered", flagName)
		}
		help += "\n" + flag.Usage
	}
	normalizedHelp := strings.Join(strings.Fields(help), " ")
	for _, required := range []string{
		"Automatic deletion is disabled",
		"brew uninstall tekierz/tap/dotfiles",
		"retained",
		"does not enable deletion",
	} {
		if !strings.Contains(normalizedHelp, required) {
			t.Errorf("uninstall help missing %q:\n%s", required, help)
		}
	}
	for _, forbidden := range []string{
		"Uninstall dotfiles completely",
		"Remove dotfiles binaries",
		"Remove utility scripts",
		"Remove dotfiles configuration directory",
	} {
		if strings.Contains(normalizedHelp, forbidden) {
			t.Errorf("uninstall help still promises deletion with %q:\n%s", forbidden, help)
		}
	}
}

func TestRunUninstallPrintsRestoreOnceAndKeepsBackup(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	const session = "2026-07-10_12-00-00"
	backupDir := filepath.Join(xdg, "dotfiles", "backups", session)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	const relPath = ".zshrc"
	if err := os.WriteFile(filepath.Join(backupDir, backup.EncodeName(relPath)), []byte("restored\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, backup.ManifestName), []byte(backup.ManifestLine(relPath, 0o600)), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _ := captureUninstallOutput(t, func() {
		if err := runUninstall(false, false, false, true); err != nil {
			t.Fatalf("clean restore uninstall: %v", err)
		}
	})
	if got := strings.Count(stdout, "Restoring backup: "+session); got != 1 {
		t.Fatalf("restore announcement count = %d, want 1:\n%s", got, stdout)
	}
	if strings.Contains(stdout, "Restoring from backup:") {
		t.Fatalf("duplicate restore announcement remains:\n%s", stdout)
	}
	if _, err := os.Stat(backupDir); err != nil {
		t.Fatalf("backup was not retained after successful restore: %v", err)
	}
}

func TestRunUninstallReturnsRestoreFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	backupDir := filepath.Join(config.ConfigDir(), "backups", "broken")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, backup.ManifestName), []byte(backup.ManifestLine(".zshrc", 0o600)), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := captureUninstallOutput(t, func() {
		if err := runUninstall(false, false, false, true); err == nil {
			t.Fatal("failed requested restore returned nil")
		}
	})
	output := stdout + stderr
	if !strings.Contains(output, "Restore did not complete successfully") || !strings.Contains(output, "Automatic deletion is disabled") {
		t.Fatalf("failure did not retain actionable guidance:\n%s", output)
	}
	if _, err := os.Stat(backupDir); err != nil {
		t.Fatalf("failed restore removed backup: %v", err)
	}
}

func TestRunUninstallReportsBackupDirectoryReadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	wantErr := errors.New("injected backup read failure")
	previous := readUninstallBackupDir
	readUninstallBackupDir = func(string) ([]os.DirEntry, error) { return nil, wantErr }
	t.Cleanup(func() { readUninstallBackupDir = previous })

	stdout, stderr := captureUninstallOutput(t, func() {
		err := runUninstall(false, false, false, true)
		if !errors.Is(err, wantErr) {
			t.Fatalf("backup read error = %v, want %v", err, wantErr)
		}
	})
	output := stdout + stderr
	if strings.Contains(output, "No backups found") || strings.Contains(output, "No backup directory found") {
		t.Fatalf("read failure was reported as no backups:\n%s", output)
	}
	if !strings.Contains(output, "Could not inspect backups") || !strings.Contains(output, "left untouched") {
		t.Fatalf("read failure guidance missing:\n%s", output)
	}
}

func captureUninstallOutput(t *testing.T, run func()) (string, string) {
	t.Helper()
	stdoutFile, err := os.CreateTemp(t.TempDir(), "stdout-")
	if err != nil {
		t.Fatal(err)
	}
	stderrFile, err := os.CreateTemp(t.TempDir(), "stderr-")
	if err != nil {
		_ = stdoutFile.Close()
		t.Fatal(err)
	}

	oldStdout, oldStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutFile, stderrFile
	func() {
		defer func() {
			os.Stdout, os.Stderr = oldStdout, oldStderr
		}()
		run()
	}()
	if err := stdoutFile.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stderrFile.Close(); err != nil {
		t.Fatal(err)
	}

	stdout, err := os.ReadFile(stdoutFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.ReadFile(stderrFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(stdout), string(stderr)
}
