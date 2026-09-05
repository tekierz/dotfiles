//go:build darwin || linux

package runner

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
)

var execLookPath = exec.LookPath

func validateTrustedPrivilegedFile(path string, requireRootOwner bool) error {
	info, err := os.Lstat(path) // #nosec G703 -- this read-only check validates the allowlisted absolute executable before spawn.
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Mode().Perm()&0o022 != 0 {
		return errInvalidPrivilegedRequest
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || requireRootOwner && stat.Uid != 0 {
		return errInvalidPrivilegedRequest
	}
	return nil
}

func runPrivilegedSupervisor(args []string, control io.Reader, stdout, stderr io.Writer) int {
	if os.Geteuid() != 0 || len(args) == 0 || control == nil || stdout == nil || stderr == nil || !validPrivilegedArguments(args[1:]) {
		return 125
	}
	target, err := validatePrivilegedTarget(args[0])
	if err != nil || !validPrivilegedCommand(target, args[1:]) || enablePrivilegedDescendantReaping() != nil {
		return 125
	}

	// Revalidate immediately before the exact spawn. No PATH or shell lookup is
	// performed for the accepted package-manager executable.
	if err := validateTrustedPrivilegedFile(target, true); err != nil {
		return 125
	}
	// #nosec G204 G702 -- target is a clean absolute, root-owned, non-writable,
	// allowlisted package-manager executable and args are literal bounded argv.
	//nolint:noctx // The supervisor control pipe and explicit group kill/reap path own cancellation.
	command := exec.Command(target, append([]string(nil), args[1:]...)...)
	command.Env = privilegedTargetEnvironment()
	command.Dir = "/"
	command.Stdin = nil
	command.Stdout = stdout
	command.Stderr = stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return 125
	}

	controlClosed := make(chan struct{}, 1)
	go func() {
		var one [1]byte
		_, _ = control.Read(one[:])
		controlClosed <- struct{}{}
	}()
	waitErr, cleanupErr := finishPrivilegedCommand(command, controlClosed, defaultPrivilegedLifecycleDeps())
	if cleanupErr != nil {
		_, _ = io.WriteString(stderr, errPrivilegedSupervisorCleanup.Error()+"\n")
		return 125
	}
	return privilegedExitCode(waitErr)
}

func killPrivilegedGroup(pgid int) error {
	if pgid <= 0 {
		return errInvalidPrivilegedRequest
	}
	for {
		err := syscall.Kill(-pgid, syscall.SIGKILL)
		switch {
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.ESRCH):
			return nil
		default:
			return err
		}
	}
}

func privilegedExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ProcessState == nil {
		return 125
	}
	if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	code := exit.ExitCode()
	if code < 1 || code > 255 {
		return 125
	}
	return code
}
