//go:build linux

package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func requirePrivilegedCI(t *testing.T) {
	t.Helper()
	if os.Getenv("DOTFILES_PRIVILEGED_CI") != "1" {
		t.Skip("privileged supervisor requires explicit ephemeral-CI opt-in")
	}
	// An explicitly activated but misconfigured CI job must fail, not pass
	// through a skipped test that would conceal missing privileged coverage.
	if os.Getenv("CI") != "true" || os.Geteuid() != 0 {
		t.Fatal("privileged supervisor opt-in requires CI=true and euid=0")
	}
}

// TestPrivilegedSupervisorHelper is the only process that enters the actual
// supervisor. Its subreaper and Wait4(-1) cannot adopt/reap the test runner's
// unrelated children. The workflow invokes the parent CI test, not this helper.
func TestPrivilegedSupervisorHelper(t *testing.T) {
	if os.Getenv("DOTFILES_PRIVILEGED_HELPER") != "1" {
		t.Skip("isolated supervisor helper only")
	}
	requirePrivilegedCI(t)
	root := os.Getenv("DOTFILES_PRIVILEGED_FIXTURE_ROOT")
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved != root || filepath.Dir(root) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(root), "dotfiles-privileged-ci-") {
		t.Fatal("helper requires an isolated real CI fixture directory")
	}
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		t.Fatal("helper target missing")
	}
	args := os.Args[separator+1:]
	if args[0] != filepath.Join(root, "apt") && args[0] != filepath.Join(root, "pacman") {
		t.Fatal("helper refuses installed package-manager paths")
	}
	os.Exit(runPrivilegedSupervisor(args, os.Stdin, os.Stdout, os.Stderr))
}

func TestPrivilegedSupervisorCI(t *testing.T) {
	requirePrivilegedCI(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		manager string
		args    []string
		body    string
		child   bool
		cancel  bool
		code    int
	}{
		{"apt-argv-environment", "apt", []string{"install", "-y", "libc6:arm64", "git"}, "exit 0\n", false, false, 0},
		{"pacman-argv", "pacman", []string{"-S", "--noconfirm", "--needed", "git"}, "exit 0\n", false, false, 0},
		{"exit-status", "apt", []string{"update"}, "exit 7\n", false, false, 7},
		{"invalid-grammar", "apt", []string{"remove", "git"}, "exit 0\n", false, false, 125},
		{"natural-group", "apt", []string{"update"}, "/bin/sleep 30 &\nprintf '%s\\n' \"$!\" > \"$root/child.pid\"\nexit 0\n", true, false, 0},
		{"cancel-group", "apt", []string{"update"}, "/bin/sleep 30 &\nprintf '%s\\n' \"$!\" > \"$root/child.pid\"\nwait\n", true, true, 137},
		{"natural-escaped", "apt", []string{"update"}, escapedPrivilegedFixture(false), true, false, 0},
		{"cancel-escaped", "apt", []string{"update"}, escapedPrivilegedFixture(true), true, true, 137},
	}
	executed := 0
	for _, test := range tests {
		if t.Run(test.name, func(t *testing.T) {
			root, err := os.MkdirTemp("", "dotfiles-privileged-ci-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(root) })
			// Every PID file is emitted by this generated fixture. Cleanup is only
			// a failure backstop; successful cases require all fixture PIDs gone.
			t.Cleanup(func() {
				for _, name := range []string{"child.pid", "leader.pid"} {
					if pid, err := readPrivilegedFixturePID(filepath.Join(root, name)); err == nil {
						_ = syscall.Kill(pid, syscall.SIGKILL)
					}
				}
			})
			target := filepath.Join(root, test.manager)
			prefix := "#!/bin/sh\nroot=${0%/*}\nprintf '%s\\n' \"$$\" > \"$root/leader.pid\"\n" +
				"printf 'cwd=%s\\npath=%s\\nhome=%s\\nuser=%s\\nsecret=%s\\n' \"$PWD\" \"$PATH\" \"$HOME\" \"$USER\" \"${PRIVATE_TEST_SECRET-unset}\"\n" +
				"printf 'arg=%s\\n' \"$@\"\n"
			if err := os.WriteFile(target, []byte(prefix+test.body), 0o700); err != nil {
				t.Fatal(err)
			}
			unrelated := exec.CommandContext(context.Background(), "/bin/sleep", "30")
			if err := unrelated.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() })
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			args := append([]string{"-test.run=^TestPrivilegedSupervisorHelper$", "--", target}, test.args...)
			command := exec.CommandContext(ctx, executable, args...)
			command.Env = []string{"CI=true", "DOTFILES_PRIVILEGED_CI=1", "DOTFILES_PRIVILEGED_HELPER=1", "DOTFILES_PRIVILEGED_FIXTURE_ROOT=" + root,
				"TMPDIR=" + os.TempDir(), "PATH=/hostile", "PRIVATE_TEST_SECRET=must-not-reach-target"}
			command.WaitDelay = 2 * time.Second
			control, err := command.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = control.Close() }()
			var output bytes.Buffer
			command.Stdout, command.Stderr = &output, &output
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			if test.cancel {
				if err := waitPrivilegedFixturePID(ctx, filepath.Join(root, "child.pid")); err != nil {
					_ = control.Close()
					_ = command.Wait()
					t.Fatalf("fixture did not start: %v; output=%s", err, output.String())
				}
				if err := control.Close(); err != nil {
					t.Fatal(err)
				}
			}
			waitErr := command.Wait()
			if ctx.Err() != nil {
				t.Fatalf("supervisor exceeded lifecycle bound: %v; output=%s", ctx.Err(), output.String())
			}
			if got := privilegedExitCode(waitErr); got != test.code {
				t.Fatalf("supervisor exit=%d want=%d error=%v output=%s", got, test.code, waitErr, output.String())
			}
			if test.code == 125 {
				if _, err := os.Stat(filepath.Join(root, "leader.pid")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("invalid grammar started a target: %v", err)
				}
			} else {
				for _, line := range []string{"cwd=/", "path=/usr/sbin:/usr/bin:/sbin:/bin", "home=/root", "user=root", "secret=unset"} {
					if !strings.Contains(output.String(), line+"\n") {
						t.Errorf("missing target environment %q in %s", line, output.String())
					}
				}
				for _, arg := range test.args {
					if !strings.Contains(output.String(), "arg="+arg+"\n") {
						t.Errorf("literal target argument %q missing from %s", arg, output.String())
					}
				}
				for _, name := range []string{"leader.pid", "child.pid"} {
					if name == "child.pid" && !test.child {
						continue
					}
					pid, err := readPrivilegedFixturePID(filepath.Join(root, name))
					if err != nil || !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
						t.Errorf("supervisor returned before %s was gone: pid=%d error=%v", name, pid, err)
					}
				}
			}
			if err := syscall.Kill(unrelated.Process.Pid, 0); err != nil {
				t.Errorf("unrelated harness child did not survive: %v", err)
			}
		}) {
			executed++
		}
	}
	if executed != len(tests) {
		t.Fatalf("privileged cases completed=%d want=%d", executed, len(tests))
	}
	t.Logf("DOTFILES_PRIVILEGED_CI_EXECUTED cases=%d", executed)
}

func escapedPrivilegedFixture(cancel bool) string {
	// The ready PID is written after setsid, so even the natural-exit case
	// establishes an escaped session before the target leader can exit.
	body := "/usr/bin/setsid /bin/sh -c 'printf \"%s\\n\" \"$$\" > \"$0/child.pid\"; exec /bin/sleep 30' \"$root\" &\n" +
		"while [ ! -s \"$root/child.pid\" ]; do /bin/sleep 0.01; done\n"
	if cancel {
		return body + "wait\n"
	}
	return body + "exit 0\n"
}

func readPrivilegedFixturePID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 1 {
		return 0, fmt.Errorf("invalid fixture pid %q", data)
	}
	return pid, nil
}

func waitPrivilegedFixturePID(ctx context.Context, path string) error {
	for {
		if _, err := readPrivilegedFixturePID(path); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
