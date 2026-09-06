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
	manager := pkg.NewAptManager()
	expected, err := pkg.ObserveExecutableIdentity("/usr/bin/apt")
	captured, ok := manager.ExecutableIdentity()
	if err != nil || !ok || captured.Digest() != expected.Digest() {
		fmt.Fprintln(os.Stderr, "unexpected APT executable identity")
		return 1
	}
	switch os.Args[1] {
	case "receipt":
		err = verifyReceipt(ctx, manager, "install ok installed")
	case "receipt-held":
		err = verifyReceipt(ctx, manager, "hold ok installed")
	case "update":
		err = verifyUpdate(ctx, manager)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "platform acceptance failed: %v\n", err)
		return 1
	}
	fmt.Printf("DOTFILES_DEBIAN_QA_EXECUTED mode=%s\n", os.Args[1])
	return 0
}

func normalRequestAllowed(uid int, ci, github, optin string, args []string) bool {
	if uid == 0 || ci != "true" || github != "true" || optin != "1" || len(args) != 1 {
		return false
	}
	return args[0] == "receipt" || args[0] == "receipt-held" || args[0] == "update"
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
	if string(data) != qaAuthorization {
		return errors.New("missing acceptance authorization")
	}
	return nil
}

func verifyReceipt(ctx context.Context, manager *pkg.AptManager, wantStatus string) error {
	output, err := exec.CommandContext(ctx, "/usr/bin/dpkg-query", "-W", "-f=${Status}\t${Version}", "fzf").Output()
	if err != nil {
		return err
	}
	fields := strings.Split(strings.TrimSpace(string(output)), "\t")
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
		return errors.New("individual receipt version disagrees with dpkg")
	}
	installed, err := manager.ListInstalledContext(ctx)
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

func verifyUpdate(ctx context.Context, manager *pkg.AptManager) error {
	before, err := manager.GetVersion("fzf")
	if err != nil {
		return err
	}
	command, err := manager.UpdateStreaming(ctx, "fzf")
	if err != nil {
		return err
	}
	if command == nil {
		return errors.New("missing APT update sequence")
	}
	// The real sequence drains apt update before apt install, using captured
	// identity and the production sudo supervisor. No test factory is injected.
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
	if before != after {
		return errors.New("repository version changed during no-op acceptance; rerun on a stable snapshot")
	}
	fmt.Printf("update evidence=no-op version=%s\n", after)
	return nil
}
