package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/tekierz/dotfiles/internal/pkg"
)

func TestClaudeLegacyInstallWithContextFailsClosed(t *testing.T) {
	tool := NewClaudeCodeTool()
	mgr := pkg.NewMockPackageManager()
	var lines []string
	err := tool.InstallWithContext(context.Background(), mgr, func(line string) {
		lines = append(lines, line)
	})
	if !errors.Is(err, ErrReviewedInstallRequired) {
		t.Fatalf("InstallWithContext error = %v, want reviewed recipe requirement", err)
	}
	if len(lines) != 0 || len(mgr.InstallCalls) != 0 {
		t.Fatalf("legacy Claude installer mutated or streamed output: lines=%v installs=%v", lines, mgr.InstallCalls)
	}
}
