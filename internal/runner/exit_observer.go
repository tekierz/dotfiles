//go:build darwin || linux

package runner

import (
	"errors"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

var errInvalidStreamingExitObserver = errors.New("invalid streaming exit observer pid")
var errStreamingExitObserverClosed = errors.New("streaming exit observer closed")

type streamingExitObserver interface {
	Wait() error
	Close() error
}

type exitObserverState struct {
	fd     int
	closed atomic.Bool
}

func (state *exitObserverState) Close() error {
	if state.closed.Swap(true) {
		return nil
	}
	return unix.Close(state.fd)
}
