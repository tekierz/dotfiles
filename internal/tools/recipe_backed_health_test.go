package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func TestRecipeBackedCLIHealthUsesTypedBinaryEvidence(t *testing.T) {
	for _, tool := range []Tool{NewCodexTool(), NewPiTool(), NewOpenCodeTool(), NewClaudeCodeTool()} {
		t.Run(tool.ID(), func(t *testing.T) {
			binary := tool.ID()
			if binary == "claude-code" {
				binary = "claude"
			}
			for _, test := range []struct {
				name string
				want health.ComponentState
			}{
				{"missing", health.ComponentMissing},
				{"empty-path", health.ComponentMissing},
				{"present", health.ComponentPresent},
				{"non-executable", health.ComponentUnknown},
				{"dangling", health.ComponentUnknown},
				{"directory", health.ComponentUnknown},
				{"relative-path", health.ComponentUnknown},
				{"empty-entry", health.ComponentUnknown},
				{"inaccessible", health.ComponentUnknown},
			} {
				t.Run(test.name, func(t *testing.T) {
					dir := t.TempDir()
					path := filepath.Join(dir, binary)
					search := dir
					switch test.name {
					case "empty-path":
						search = ""
					case "present", "non-executable":
						mode := os.FileMode(0o700)
						if test.name == "non-executable" {
							mode = 0o600
						}
						if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 99\n"), mode); err != nil {
							t.Fatal(err)
						}
					case "dangling":
						if err := os.Symlink(filepath.Join(dir, "missing-target"), path); err != nil {
							t.Fatal(err)
						}
					case "directory":
						if err := os.Mkdir(path, 0o700); err != nil {
							t.Fatal(err)
						}
					case "relative-path":
						search = "relative-missing-bin"
					case "empty-entry":
						search = string(os.PathListSeparator) + dir
					case "inaccessible":
						if os.Geteuid() == 0 {
							t.Skip("root bypasses directory permissions")
						}
						if err := os.Chmod(dir, 0); err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
					}
					t.Setenv("PATH", search)
					manager := pkg.NewMockPackageManager()
					manager.ManagerName = "brew"
					snapshot, err := ObserveInstallationHealth(context.Background(), []Tool{tool}, manager, pkg.PlatformMacOS, 1)
					if err != nil {
						t.Fatal(err)
					}
					observation, _ := snapshot.Tool(tool.ID())
					direct := observation.Direct()
					if direct.State != test.want || !direct.Authoritative || len(direct.Alternatives) != 1 {
						t.Fatalf("direct=%+v, want authoritative %s", direct, test.want)
					}
					alternative := direct.Alternatives[0]
					if alternative.Kind != health.DirectSourceBinary || len(alternative.Identifiers) != 1 || alternative.Identifiers[0] != binary {
						t.Fatalf("recipe detector cannot consume alternative=%+v", alternative)
					}
					wantPresence := health.Presence(test.want)
					if observation.Presence() != wantPresence {
						t.Fatalf("presence=%s, want %s", observation.Presence(), wantPresence)
					}
					if observation.Package().Authoritative {
						t.Fatal("prerequisite receipt became product authority")
					}
				})
			}
		})
	}
}
