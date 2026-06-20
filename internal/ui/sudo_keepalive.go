package ui

import (
	"runtime"
	"time"

	"github.com/tekierz/dotfiles/internal/runner"
)

// sudoKeepAliveInterval is how often the keep-alive loop refreshes the sudo
// credential cache. The sudoers timestamp default is ~5 minutes; refreshing
// every 60s keeps it warm with a wide safety margin for slow refreshes (C16).
const sudoKeepAliveInterval = 60 * time.Second

// sudoKeepAliveLoop refreshes the sudo timestamp by calling refresh() once per
// interval until stop is closed, then returns. A transient refresh error is
// tolerated (ignored): the next privileged install step will surface a real
// failure, and a single failed `sudo -v` must not abort the whole install. The
// loop selects on stop on EVERY wait so it terminates promptly on
// cancel/teardown with no goroutine leak. It is decoupled from sudo itself
// (refresh + interval are injected) so it is unit-testable without invoking
// real sudo.
func sudoKeepAliveLoop(stop <-chan struct{}, interval time.Duration, refresh func() error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			// Ignore a transient failure; keep the loop alive.
			_ = refresh()
		}
	}
}

// startSudoKeepAlive launches the sudo keep-alive loop for the duration of a
// Linux install and returns a stop function that terminates the goroutine and
// blocks until it has exited (so callers can rely on no goroutine surviving the
// install/teardown). On non-Linux platforms — where the install path uses
// Homebrew and never sudo — it is a complete no-op: no goroutine is started and
// the returned stop func does nothing.
//
// The lifecycle is owned by the install flow: startInstallation() calls this
// when the worker starts, and the returned stop func is invoked from both the
// normal-completion path (installDoneMsg) and the cancel/teardown path
// (teardownStream), so the keep-alive never outlives the install.
func startSudoKeepAlive(refresh func() error) (stop func()) {
	if runtime.GOOS != "linux" {
		return func() {}
	}
	stopCh := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		sudoKeepAliveLoop(stopCh, sudoKeepAliveInterval, refresh)
	}()

	var stopped bool
	return func() {
		if stopped {
			return
		}
		stopped = true
		close(stopCh)
		<-done
	}
}

// refreshSudo is the production refresh action used by the install keep-alive.
// It extends the cached sudo timestamp non-interactively.
func refreshSudo() error { return runner.RefreshSudo() }
