//go:build darwin || linux

package runner

import (
	"errors"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
)

type privilegedTestObserver struct {
	wait  func() error
	close func() error
}

func (o privilegedTestObserver) Wait() error  { return o.wait() }
func (o privilegedTestObserver) Close() error { return o.close() }

func TestPrivilegedLifecycleSignalsBeforeReaping(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		name := "natural"
		if cancel {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			var mu sync.Mutex
			var trace []string
			record := func(event string) { mu.Lock(); defer mu.Unlock(); trace = append(trace, event) }
			control := make(chan struct{})
			if cancel {
				close(control)
			}
			observerClosed := make(chan struct{})
			waitErr := errors.New("terminal status")
			deps := privilegedLifecycleDeps{
				newExitObserver: func(int) (streamingExitObserver, error) {
					record("observe")
					return privilegedTestObserver{
						wait: func() error {
							if cancel {
								<-observerClosed
								return errStreamingExitObserverClosed
							}
							return nil
						},
						close: func() error { record("close"); close(observerClosed); return nil },
					}, nil
				},
				killGroup:       func(int) error { record("signal"); return nil },
				killLeader:      func(*exec.Cmd) error { record("leader"); return nil },
				waitCommand:     func(*exec.Cmd) error { record("wait"); return waitErr },
				reapDescendants: func() error { record("descendants"); return nil },
			}
			got, cleanup := finishPrivilegedCommand(&exec.Cmd{Process: &os.Process{Pid: 123}}, control, deps)
			if !errors.Is(got, waitErr) || cleanup != nil {
				t.Fatalf("status=%v cleanup=%v", got, cleanup)
			}
			want := []string{"observe", "signal", "close", "wait", "descendants"}
			if !reflect.DeepEqual(trace, want) {
				t.Fatalf("cleanup order=%v, want %v", trace, want)
			}
		})
	}
}

func TestPrivilegedReapingDeadlineCoversEveryIteration(t *testing.T) {
	for _, mode := range []string{"reaped", "interrupted"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			now := time.Unix(0, 0)
			err := reapPrivilegedChildrenWithDeps(func() (int, error) {
				calls++
				now = now.Add(time.Second)
				if calls > 4 {
					return 0, syscall.ECHILD
				}
				if mode == "interrupted" {
					return 0, syscall.EINTR
				}
				return 123, nil
			}, func() error { t.Fatal("should not kill while reaping/interrupted"); return nil }, func() time.Time { return now }, func(time.Duration) { t.Fatal("unexpected sleep") })
			if !errors.Is(err, errPrivilegedSupervisorCleanup) || calls > 2 {
				t.Fatalf("unbounded %s loop: calls=%d error=%v", mode, calls, err)
			}
		})
	}
}

func TestPrivilegedLifecycleCleanupFailures(t *testing.T) {
	for _, failure := range []string{"registration", "observer", "close", "group", "leader", "descendants"} {
		t.Run(failure, func(t *testing.T) {
			sentinel := errors.New(failure)
			waited := false
			signalled := false
			fallback := false
			deps := privilegedLifecycleDeps{
				newExitObserver: func(int) (streamingExitObserver, error) {
					if failure == "registration" {
						return nil, sentinel
					}
					return privilegedTestObserver{
						wait: func() error {
							if failure == "observer" {
								return sentinel
							}
							return nil
						},
						close: func() error {
							if failure == "close" {
								return sentinel
							}
							return nil
						},
					}, nil
				},
				killGroup: func(int) error {
					if waited {
						t.Error("group signalled after reaping")
					}
					signalled = true
					if failure == "group" {
						return sentinel
					}
					if failure == "leader" {
						return syscall.EPERM
					}
					return nil
				},
				killLeader: func(*exec.Cmd) error {
					if waited {
						t.Error("leader signalled after reaping")
					}
					fallback = true
					if failure == "leader" {
						return sentinel
					}
					return nil
				},
				waitCommand: func(*exec.Cmd) error {
					if !signalled {
						t.Error("reaped without cleanup")
					}
					waited = true
					return nil
				},
				reapDescendants: func() error {
					if !waited {
						t.Error("descendants reaped before exact leader")
					}
					if failure == "descendants" {
						return sentinel
					}
					return nil
				},
			}
			_, err := finishPrivilegedCommand(&exec.Cmd{Process: &os.Process{Pid: 123}}, make(chan struct{}), deps)
			if !errors.Is(err, sentinel) || !waited {
				t.Fatalf("cleanup lost error or wait: %v waited=%v", err, waited)
			}
			if fallback != (failure == "group" || failure == "leader") {
				t.Fatalf("fallback=%v for %s", fallback, failure)
			}
		})
	}
}

func TestPrivilegedLifecycleJoinsObserverBeforeWait(t *testing.T) {
	control := make(chan struct{})
	close(control)
	observerClosed := make(chan struct{})
	release := make(chan struct{})
	waited := make(chan struct{})
	finished := make(chan struct{})
	deps := privilegedLifecycleDeps{
		newExitObserver: func(int) (streamingExitObserver, error) {
			return privilegedTestObserver{
				wait:  func() error { <-release; return errStreamingExitObserverClosed },
				close: func() error { close(observerClosed); return nil },
			}, nil
		},
		killGroup:       func(int) error { return nil },
		waitCommand:     func(*exec.Cmd) error { close(waited); return nil },
		reapDescendants: func() error { return nil },
	}
	go func() {
		defer close(finished)
		_, _ = finishPrivilegedCommand(&exec.Cmd{Process: &os.Process{Pid: 123}}, control, deps)
	}()
	<-observerClosed
	select {
	case <-waited:
		t.Error("reaped while observer still active")
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not finish after observer joined")
	}
}

func TestPrivilegedLifecycleRealUnprivilegedExit(t *testing.T) {
	// Only this unprivileged child runs; adopted-child reaping is replaced below.
	// The trusted supervisor entrypoint and root-only harness are never invoked.
	//nolint:noctx // The lifecycle under test owns cancellation and exact-child reaping.
	command := exec.Command("/bin/sh", "-c", "exit 7")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	deps := defaultPrivilegedLifecycleDeps()
	deps.reapDescendants = func() error { return nil }
	waitErr, cleanupErr := finishPrivilegedCommand(command, make(chan struct{}), deps)
	// Darwin can deny an unprivileged signal to an exited group; retain that
	// cleanup failure. The root-only Linux harness checks privileged success.
	if (cleanupErr != nil && (runtime.GOOS != "darwin" || !errors.Is(cleanupErr, syscall.EPERM))) || privilegedExitCode(waitErr) != 7 {
		t.Fatalf("exit=%v cleanup=%v", waitErr, cleanupErr)
	}
}

func TestPrivilegedReapingOutcomes(t *testing.T) {
	sentinel := errors.New("reaping failure")
	for _, mode := range []string{"no-children", "wait-error", "kill-error", "kill-then-reap"} {
		t.Run(mode, func(t *testing.T) {
			calls, kills, sleeps := 0, 0, 0
			err := reapPrivilegedChildrenWithDeps(func() (int, error) {
				calls++
				switch mode {
				case "no-children":
					return 0, syscall.ECHILD
				case "wait-error":
					return 0, sentinel
				}
				if calls > 1 {
					return 0, syscall.ECHILD
				}
				return 0, nil
			}, func() error {
				kills++
				if mode == "kill-error" {
					return sentinel
				}
				return nil
			}, time.Now, func(time.Duration) { sleeps++ })
			if mode == "wait-error" || mode == "kill-error" {
				if !errors.Is(err, sentinel) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if mode == "kill-then-reap" && (calls != 2 || kills != 1 || sleeps != 1) {
				t.Fatalf("calls=%d kills=%d sleeps=%d", calls, kills, sleeps)
			}
		})
	}
}
