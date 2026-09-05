//go:build darwin || linux

package runner

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

type privilegedLifecycleDeps struct {
	newExitObserver func(int) (streamingExitObserver, error)
	killGroup       func(int) error
	killLeader      func(*exec.Cmd) error
	waitCommand     func(*exec.Cmd) error
	reapDescendants func() error
}

func defaultPrivilegedLifecycleDeps() privilegedLifecycleDeps {
	return privilegedLifecycleDeps{
		newExitObserver: newStreamingExitObserver,
		killGroup:       killPrivilegedGroup,
		killLeader:      func(command *exec.Cmd) error { return command.Process.Kill() },
		waitCommand:     func(command *exec.Cmd) error { return command.Wait() },
		reapDescendants: reapPrivilegedDescendants,
	}
}

func finishPrivilegedCommand(command *exec.Cmd, controlClosed <-chan struct{}, deps privilegedLifecycleDeps) (error, error) {
	// Keep the leader unreaped until every group/leader signal is finished.
	// Its reserved PID also reserves the process-group ID against reuse.
	observer, observerErr := deps.newExitObserver(command.Process.Pid)
	if errors.Is(observerErr, syscall.ESRCH) {
		observer, observerErr = completedStreamingExitObserver{}, nil
	}
	var observed chan error
	observedExit := false
	if observerErr == nil {
		observed = make(chan error, 1)
		go func() { observed <- observer.Wait() }()
		select {
		case observerErr = <-observed:
			observedExit = true
		case <-controlClosed:
		}
	}

	cleanupErr := deps.killGroup(command.Process.Pid)
	if cleanupErr != nil {
		cleanupErr = errors.Join(cleanupErr, deps.killLeader(command))
	}
	if observed != nil {
		cleanupErr = errors.Join(cleanupErr, observer.Close())
		if !observedExit {
			err := <-observed
			if !errors.Is(err, errStreamingExitObserverClosed) {
				observerErr = err
			}
		}
	}
	waitErr := deps.waitCommand(command)
	return waitErr, errors.Join(observerErr, cleanupErr, deps.reapDescendants())
}

func reapPrivilegedChildrenWithDeps(waitChild func() (int, error), killChildren func() error, now func() time.Time, sleep func(time.Duration)) error {
	deadline := now().Add(2 * time.Second)
	for {
		if !now().Before(deadline) {
			return errPrivilegedSupervisorCleanup
		}
		pid, err := waitChild()
		switch {
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.ECHILD):
			return nil
		case err != nil:
			return err
		case pid > 0:
			continue
		}
		if err := killChildren(); err != nil {
			return err
		}
		sleep(10 * time.Millisecond)
	}
}
