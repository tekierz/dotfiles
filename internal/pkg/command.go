package pkg

import (
	"context"
	"os/exec"
	"time"
)

const (
	packageQueryTimeout    = 30 * time.Second
	packageRefreshTimeout  = 5 * time.Minute
	packageMutationTimeout = time.Hour
)

// packageCommand bounds legacy synchronous PackageManager operations. The
// streaming API receives a caller-owned context; these compatibility methods
// cannot, so a generous operation-specific deadline prevents a lost child
// process from hanging the application forever.
func packageCommand(timeout time.Duration, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	return packageCommandWithContext(context.Background(), timeout, name, args...)
}

func packageCommandWithContext(parent context.Context, timeout time.Duration, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	// #nosec G204 -- executables are fixed package-manager commands or paths resolved by exec.LookPath; arguments are registry package IDs.
	return exec.CommandContext(ctx, name, args...), cancel
}
