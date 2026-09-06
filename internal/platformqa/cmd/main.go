// Command platformqa is an acceptance harness, excluded from release packaging.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
)

const qaRoot = "/opt/dotfiles-platform-qa"
const qaExecutable = qaRoot + "/platformqa"
const qaAuthorization = "dotfiles-platform-qa-debian-v1\n"
const qaArchAuthorization = "dotfiles-platform-qa-arch-v1\n"

func main() { os.Exit(run()) }

func run() int {
	executable, err := os.Executable()
	if runtime.GOOS != "linux" || err != nil || checkPreparation(executable, os.Lstat, os.ReadFile) != nil {
		fmt.Fprintln(os.Stderr, "platform acceptance requires the prepared disposable Linux container")
		return 125
	}
	// sudo deliberately strips CI/opt-in variables. Root-owned installation and
	// authorization are checked first; the product dispatcher still checks exact
	// private argv, root identity, trusted target and bounded package grammar.
	if handled, code := runner.DispatchPrivilegedSupervisor(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); handled {
		return code
	}
	if !normalRequestAllowed(os.Geteuid(), os.Getenv("CI"), os.Getenv("GITHUB_ACTIONS"), os.Getenv("DOTFILES_PLATFORM_ACCEPTANCE"), os.Args[1:]) {
		fmt.Fprintln(os.Stderr, "platform acceptance request refused")
		return 125
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var manager acceptanceManager = pkg.NewAptManager()
	platform, managerPath := "DEBIAN", "/usr/bin/apt"
	authorization, err := os.ReadFile(qaRoot + "/authorized")
	if err != nil {
		return 125
	}
	if string(authorization) == qaArchAuthorization {
		manager = pkg.NewPacmanManager(false)
		platform, managerPath = "ARCH", "/usr/bin/pacman"
		if os.Args[1] == "receipt-held" || os.Args[1] == "upgrade" {
			return 125
		}
	}
	expected, err := pkg.ObserveExecutableIdentity(managerPath)
	captured, ok := manager.ExecutableIdentity()
	if err != nil || !ok || captured.Digest() != expected.Digest() {
		fmt.Fprintln(os.Stderr, "unexpected package manager executable identity")
		return 1
	}
	switch os.Args[1] {
	case "receipt":
		err = verifyReceipt(ctx, manager, "install ok installed")
	case "receipt-held":
		err = verifyReceipt(ctx, manager, "hold ok installed")
	case "update":
		err = verifyUpdate(ctx, manager, false)
	case "upgrade":
		err = verifyUpdate(ctx, manager, true)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "platform acceptance failed: %v\n", err)
		return 1
	}
	fmt.Printf("DOTFILES_%s_QA_EXECUTED mode=%s\n", platform, os.Args[1])
	return 0
}

func normalRequestAllowed(uid int, ci, github, optin string, args []string) bool {
	if uid == 0 || ci != "true" || github != "true" || optin != "1" || len(args) != 1 {
		return false
	}
	return args[0] == "receipt" || args[0] == "receipt-held" || args[0] == "update" || args[0] == "upgrade"
}

// A temporary HOME or ambient opt-in alone cannot authorize this executable on
// an owner's machine. Only the container bootstrap installs these root-owned
// immutable files; every ancestor is checked without following symlinks.
func checkPreparation(executable string, lstat func(string) (os.FileInfo, error), readFile func(string) ([]byte, error)) error {
	if executable != qaExecutable {
		return errors.New("unexpected acceptance executable")
	}
	for _, path := range []string{"/", "/opt", qaRoot, qaExecutable, qaRoot + "/authorized"} {
		info, err := lstat(path)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 || info.Mode().Perm()&0o022 != 0 {
			return errors.New("untrusted acceptance installation")
		}
		directory := path == "/" || path == "/opt" || path == qaRoot
		if directory && !info.IsDir() || !directory && !info.Mode().IsRegular() {
			return errors.New("unexpected acceptance file type")
		}
	}
	data, err := readFile(qaRoot + "/authorized")
	if err != nil {
		return err
	}
	if string(data) != qaAuthorization && string(data) != qaArchAuthorization {
		return errors.New("missing acceptance authorization")
	}
	return nil
}

type acceptanceManager interface {
	pkg.PackageManager
	pkg.ExecutableIdentityProvider
	IsInstalledContext(context.Context, string) bool
}

func verifyReceipt(ctx context.Context, manager acceptanceManager, wantStatus string) error {
	command := exec.CommandContext(ctx, "/usr/bin/dpkg-query", "-W", "-f=${Status}\t${Version}", "fzf")
	if manager.Name() == "pacman" {
		command = exec.CommandContext(ctx, "/usr/bin/pacman", "-Q", "fzf")
	}
	output, err := command.Output()
	if err != nil {
		return err
	}
	fields := strings.Split(strings.TrimSpace(string(output)), "\t")
	if manager.Name() == "pacman" {
		fields = strings.Fields(string(output))
		wantStatus = "fzf"
	}
	if len(fields) != 2 || fields[0] != wantStatus || fields[1] == "" {
		return fmt.Errorf("native receipt mismatch: %q", output)
	}
	if !manager.IsInstalledContext(ctx, "fzf") {
		return errors.New("healthy receipt was not installed")
	}
	version, err := manager.GetVersion("fzf")
	if err != nil {
		return err
	}
	if version != fields[1] {
		return errors.New("individual receipt version disagrees with native manager")
	}
	installed, err := manager.ListInstalled()
	if err != nil {
		return err
	}
	count := 0
	for _, item := range installed {
		if item.Name == "fzf" {
			count++
			if item.CurrentVersion != version {
				return errors.New("batch receipt version disagrees")
			}
		}
	}
	if count != 1 {
		return errors.New("batch receipt missing or duplicated")
	}
	fmt.Printf("receipt status=%s version=%s\n", wantStatus, version)
	return nil
}

func verifyUpdate(ctx context.Context, manager acceptanceManager, requireUpgrade bool) error {
	before, err := manager.GetVersion("fzf")
	if err != nil {
		return err
	}
	command, err := manager.UpdateStreaming(ctx, "fzf")
	if err != nil {
		return err
	}
	if command == nil {
		return errors.New("missing package update sequence")
	}
	// Execute the real production update with captured identity and the sudo
	// supervisor. No test factory is injected.
	for line := range command.Output {
		fmt.Println(line)
	}
	if err := errors.Join(command.Wait(), ctx.Err()); err != nil {
		return err
	}
	if err := verifyReceipt(ctx, manager, "install ok installed"); err != nil {
		return err
	}
	after, err := manager.GetVersion("fzf")
	if err != nil {
		return err
	}
	if requireUpgrade {
		// #nosec G204 -- Fixed dpkg comparison; both operands are native receipts for the fixed fzf fixture, without a shell.
		if manager.Name() != "apt" || exec.CommandContext(ctx, "/usr/bin/dpkg", "--compare-versions", before, "lt", after).Run() != nil {
			return errors.New("expected a real version-increasing APT update")
		}
		fmt.Printf("update evidence=version-change before=%s after=%s\n", before, after)
	} else {
		if before != after {
			return errors.New("repository version changed during no-op acceptance; rerun on a stable snapshot")
		}
		fmt.Printf("update evidence=no-op version=%s\n", after)
	}
	return nil
}
