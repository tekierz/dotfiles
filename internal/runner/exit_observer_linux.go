//go:build linux

package runner

import (
	"errors"

	"golang.org/x/sys/unix"
)

type linuxExitObserver struct {
	pid int
	exitObserverState
}

func newStreamingExitObserver(pid int) (streamingExitObserver, error) {
	if pid <= 0 {
		return nil, errInvalidStreamingExitObserver
	}
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return nil, err
	}
	return &linuxExitObserver{pid: pid, exitObserverState: exitObserverState{fd: fd}}, nil
}

func (observer *linuxExitObserver) Wait() error {
	poll := []unix.PollFd{{Fd: int32(observer.fd), Events: unix.POLLIN}} // #nosec G115 -- pidfd_open returns a non-negative C int descriptor.
	for {
		if observer.closed.Load() {
			return errStreamingExitObserverClosed
		}
		n, err := unix.Poll(poll, 50)
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case observer.closed.Load():
			return errStreamingExitObserverClosed
		case err != nil:
			return err
		case n == 0:
			continue
		case poll[0].Revents&unix.POLLIN == 0:
			return unix.EIO
		}
		var info unix.Siginfo
		err = unix.EINTR
		for errors.Is(err, unix.EINTR) {
			err = unix.Waitid(unix.P_PID, observer.pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
		}
		return err
	}
}
