package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/pkg"
)

func TestClaudeInstallWithContextCancelsAndStreamsNPM(t *testing.T) {
	binDir := t.TempDir()
	npmPath := filepath.Join(binDir, "npm")
	// exec replaces the shell with sleep, ensuring CommandContext kills the
	// actual long-running process instead of leaving a child holding its pipes.
	if err := os.WriteFile(npmPath, []byte("#!/bin/sh\necho npm-started\nexec /bin/sleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	tool := NewClaudeCodeTool()
	mgr := pkg.NewMockPackageManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lines := make(chan string, 8)
	done := make(chan error, 1)
	go func() {
		done <- tool.InstallWithContext(ctx, mgr, func(line string) { lines <- line })
	}()

	const startupTimeout = 10 * time.Second
	var cancelledAt time.Time
	select {
	case line := <-lines:
		if line != "npm-started" {
			t.Fatalf("first npm output = %q, want npm-started", line)
		}
		cancelledAt = time.Now()
		cancel()
	case <-time.After(startupTimeout):
		t.Fatal("npm output was not streamed")
	}

	var err error
	const cancellationTimeout = 5 * time.Second
	select {
	case err = <-done:
	case <-time.After(cancellationTimeout):
		t.Fatal("cancelled npm install did not terminate")
	}
	cancellationElapsed := time.Since(cancelledAt)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("InstallWithContext error = %v, want context cancellation", err)
	}
	if cancellationElapsed > cancellationTimeout {
		t.Fatalf("cancelled npm install took %v to terminate", cancellationElapsed)
	}
}
