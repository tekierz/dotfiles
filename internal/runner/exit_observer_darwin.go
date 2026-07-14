//go:build darwin

package runner

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

type darwinExitObserver struct {
	exitObserverState
}

func newStreamingExitObserver(pid int) (streamingExitObserver, error) {
	if pid <= 0 {
		return nil, errInvalidStreamingExitObserver
	}
	fd, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}
	change := unix.Kevent_t{Fflags: unix.NOTE_EXIT}
	unix.SetKevent(&change, pid, unix.EVFILT_PROC, unix.EV_ADD|unix.EV_ONESHOT)
	if _, err = unix.Kevent(fd, []unix.Kevent_t{change}, nil, nil); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return &darwinExitObserver{exitObserverState{fd: fd}}, nil
}

func (observer *darwinExitObserver) Wait() error {
	events := make([]unix.Kevent_t, 1)
	for {
		if observer.closed.Load() {
			return errStreamingExitObserverClosed
		}
		n, err := unix.Kevent(observer.fd, nil, events, &unix.Timespec{Nsec: 50 * 1e6})
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case observer.closed.Load():
			return errStreamingExitObserverClosed
		case err != nil:
			return err
		case n > 0 && events[0].Flags&unix.EV_ERROR != 0:
			return syscall.Errno(events[0].Data)
		case n > 0:
			return nil
		}
	}
}
