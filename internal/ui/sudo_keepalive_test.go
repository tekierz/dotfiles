package ui

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// TestSudoKeepAliveLoopTicks verifies the loop invokes the injected refresh
// action on its injected interval. A tiny interval keeps the test fast without
// touching real sudo.
func TestSudoKeepAliveLoopTicks(t *testing.T) {
	var calls int32
	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		sudoKeepAliveLoop(stop, time.Millisecond, func() error {
			atomic.AddInt32(&calls, 1)
			return nil
		})
		close(done)
	}()

	// Wait until we've seen several ticks, then stop.
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&calls) < 3 {
		select {
		case <-deadline:
			t.Fatalf("refresh ticked only %d times; expected >= 3", atomic.LoadInt32(&calls))
		case <-time.After(time.Millisecond):
		}
	}

	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not return after stop was closed (goroutine leak)")
	}
}

// TestSudoKeepAliveLoopStopsImmediately verifies that closing stop terminates
// the loop promptly and that no further refreshes happen afterwards (no leak).
func TestSudoKeepAliveLoopStopsImmediately(t *testing.T) {
	var calls int32
	stop := make(chan struct{})
	done := make(chan struct{})

	// Long interval so the loop would otherwise block until stop fires.
	go func() {
		sudoKeepAliveLoop(stop, time.Hour, func() error {
			atomic.AddInt32(&calls, 1)
			return nil
		})
		close(done)
	}()

	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not return promptly after stop (goroutine leak)")
	}

	// With an hour interval and an immediate stop, refresh must never have run.
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("refresh ran %d times after immediate stop; expected 0", got)
	}
}

// TestSudoKeepAliveLoopToleratesRefreshError verifies a transient refresh
// failure does not abort the keep-alive loop; subsequent ticks still fire.
func TestSudoKeepAliveLoopToleratesRefreshError(t *testing.T) {
	var calls int32
	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		sudoKeepAliveLoop(stop, time.Millisecond, func() error {
			atomic.AddInt32(&calls, 1)
			return errTestRefresh
		})
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&calls) < 3 {
		select {
		case <-deadline:
			t.Fatalf("refresh ticked only %d times despite errors; expected >= 3", atomic.LoadInt32(&calls))
		case <-time.After(time.Millisecond):
		}
	}

	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not return after stop (goroutine leak)")
	}
}

// TestStartSudoKeepAliveNoOpOnNonLinux verifies the launcher is a no-op on
// platforms that do not use sudo (macOS, etc.): it never invokes refresh and
// the returned stop func is safe to call.
func TestStartSudoKeepAliveNoOpOnNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("keep-alive is active on Linux; this test asserts the no-op path")
	}

	var calls int32
	stop := startSudoKeepAlive(func() error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	// Give any (incorrectly) launched goroutine a chance to tick.
	time.Sleep(20 * time.Millisecond)
	stop() // must be safe even though nothing started

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("refresh ran %d times on non-Linux; expected 0 (no-op)", got)
	}
}

// TestTeardownStreamStopsSudoKeepAlive verifies the cancel/teardown path stops
// the keep-alive and clears the handle (no goroutine leak on Ctrl+C).
func TestTeardownStreamStopsSudoKeepAlive(t *testing.T) {
	a := NewApp(true)
	var stopped int32
	a.sudoKeepAliveStop = func() { atomic.AddInt32(&stopped, 1) }

	a.teardownStream()

	if got := atomic.LoadInt32(&stopped); got != 1 {
		t.Fatalf("teardownStream called keep-alive stop %d times; expected 1", got)
	}
	if a.sudoKeepAliveStop != nil {
		t.Fatal("teardownStream did not clear sudoKeepAliveStop handle")
	}

	// Idempotent: a second teardown must not panic or re-invoke the (now nil) stop.
	a.teardownStream()
	if got := atomic.LoadInt32(&stopped); got != 1 {
		t.Fatalf("second teardownStream re-invoked stop (%d total); expected 1", got)
	}
}

// TestInstallDoneStopsSudoKeepAlive verifies the normal-completion path
// (installDoneMsg) stops the keep-alive and clears the handle.
func TestInstallDoneStopsSudoKeepAlive(t *testing.T) {
	a := NewApp(true)
	var stopped int32
	a.sudoKeepAliveStop = func() { atomic.AddInt32(&stopped, 1) }
	a.installRunning = true

	s := NewProgressScreen(&ScreenContext{app: a})
	s.Update(installDoneMsg{})

	if got := atomic.LoadInt32(&stopped); got != 1 {
		t.Fatalf("installDoneMsg called keep-alive stop %d times; expected 1", got)
	}
	if a.sudoKeepAliveStop != nil {
		t.Fatal("installDoneMsg did not clear sudoKeepAliveStop handle")
	}
}

// errTestRefresh is a sentinel error returned by the fake refresh action.
var errTestRefresh = errTest("refresh failed")

type errTest string

func (e errTest) Error() string { return string(e) }
