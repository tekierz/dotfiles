package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
)

const cursorAgentUnavailableReason = "verified artifact support pending"

type installationUnavailableReasonProvider interface {
	InstallationUnavailableReason() string
}

func TestCursorAgentRegistryMetadataIsDistinctAndInstallUnsupported(t *testing.T) {
	tool, ok := NewRegistry().Get("cursor-agent")
	if !ok {
		t.Fatal("cursor-agent missing from registry")
	}
	if tool.ID() != "cursor-agent" || tool.Name() != "Cursor Agent" || tool.Description() != "Cursor's coding agent for the terminal" {
		t.Fatalf("Cursor Agent identity=%q/%q/%q", tool.ID(), tool.Name(), tool.Description())
	}
	reason, ok := tool.(installationUnavailableReasonProvider)
	if !ok || reason.InstallationUnavailableReason() != cursorAgentUnavailableReason {
		t.Fatalf("Cursor Agent unavailable reason=(provider=%v,value=%q)", ok, func() string {
			if ok {
				return reason.InstallationUnavailableReason()
			}
			return ""
		}())
	}
	if tool.UIGroup() != UIGroupCLITools || tool.DefaultEnabled() || ApplicationTypeOf(tool) != ApplicationTypeAIAgent {
		t.Fatalf("Cursor Agent UI metadata=group:%q default:%v type:%q", tool.UIGroup(), tool.DefaultEnabled(), ApplicationTypeOf(tool))
	}
	if tool.HasConfig() || len(tool.ConfigPaths()) != 0 || len(tool.Packages()) != 0 {
		t.Fatalf("Cursor Agent must own no config or package metadata: config=%v packages=%v", tool.ConfigPaths(), tool.Packages())
	}
	policy, ok := tool.(interface{ PackageMetadataIsAuthoritative() bool })
	if !ok || policy.PackageMetadataIsAuthoritative() {
		t.Fatal("Cursor Agent package-less direct evidence must be authoritative")
	}
	if _, recipeProvider := tool.(InstallRecipeProvider); recipeProvider {
		t.Fatal("Cursor Agent must not publish an unverified install recipe")
	}
	mgr := pkg.NewMockPackageManager()
	if err := tool.Install(mgr); err == nil || !strings.Contains(err.Error(), cursorAgentUnavailableReason) || len(mgr.InstallCalls) != 0 {
		t.Fatalf("Cursor Agent direct Install error=%v calls=%v", err, mgr.InstallCalls)
	}
	if _, err := DescribeInstall(tool, InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"}); err == nil || !strings.Contains(err.Error(), cursorAgentUnavailableReason) {
		t.Fatalf("Cursor Agent DescribeInstall error=%v, want unavailable reason", err)
	}
}

func TestCursorAgentRejectsUnverifiedPATHCollisions(t *testing.T) {
	tool := NewCursorAgentTool()
	detector, ok := any(tool).(DirectInstallationDetector)
	if !ok {
		t.Fatal("Cursor Agent exposes no direct detector")
	}
	for _, test := range []struct {
		name string
		seed func(*testing.T, string)
	}{
		{name: "executable cursor-agent", seed: func(t *testing.T, dir string) {
			t.Helper()
			writeCursorAgentCollision(t, filepath.Join(dir, "cursor-agent"), 0o700)
		}},
		{name: "desktop cursor command", seed: func(t *testing.T, dir string) {
			t.Helper()
			writeCursorAgentCollision(t, filepath.Join(dir, "cursor"), 0o700)
		}},
		{name: "generic agent command", seed: func(t *testing.T, dir string) {
			t.Helper()
			writeCursorAgentCollision(t, filepath.Join(dir, "agent"), 0o700)
		}},
		{name: "regular cursor-agent file", seed: func(t *testing.T, dir string) {
			t.Helper()
			writeCursorAgentCollision(t, filepath.Join(dir, "cursor-agent"), 0o600)
		}},
		{name: "cursor-agent symlink collision", seed: func(t *testing.T, dir string) {
			t.Helper()
			target := filepath.Join(dir, "unverified-target")
			writeCursorAgentCollision(t, target, 0o700)
			if err := os.Symlink(target, filepath.Join(dir, "cursor-agent")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			test.seed(t, dir)
			t.Setenv("PATH", dir)
			if detector.IsInstalledOutsidePackageManager(DirectInstallationObservation{}) || tool.IsInstalled() {
				t.Fatal("unverified PATH collision was accepted as Cursor Agent")
			}
		})
	}
}

func writeCursorAgentCollision(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
		t.Fatal(err)
	}
}

func TestCursorAgentHealthNeverPromotesUnverifiedPATH(t *testing.T) {
	dir := t.TempDir()
	writeCursorAgentCollision(t, filepath.Join(dir, "cursor-agent"), 0o700)
	t.Setenv("PATH", dir)
	tests := []struct {
		name     string
		platform pkg.Platform
		manager  pkg.PackageManager
	}{
		{name: "macos", platform: pkg.PlatformMacOS, manager: observationManagerNamed("brew")},
		{name: "arch", platform: pkg.PlatformArch, manager: observationManagerNamed("paru")},
		{name: "debian", platform: pkg.PlatformDebian, manager: observationManagerNamed("apt")},
		{name: "pi", platform: pkg.PlatformPi, manager: observationManagerNamed("apt")},
		{name: "unknown manager", platform: pkg.PlatformUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, err := ObserveInstallationHealth(context.Background(), []Tool{NewCursorAgentTool()}, test.manager, test.platform, 91)
			if err != nil {
				t.Fatal(err)
			}
			observation, ok := snapshot.Tool("cursor-agent")
			if !ok {
				t.Fatal("health snapshot omitted cursor-agent")
			}
			if observation.Presence() == health.PresencePresent || observation.InstallRecipeDigest() != "" {
				t.Fatalf("unverified Cursor Agent health=%s digest=%q", observation.Presence(), observation.InstallRecipeDigest())
			}
			if test.manager != nil && observation.Installability() != health.InstallabilityUnsupported {
				t.Fatalf("Cursor Agent installability=%q, want unsupported", observation.Installability())
			}
			direct := observation.Direct()
			if direct.State != health.ComponentUnknown || len(direct.Alternatives) != 1 {
				t.Fatalf("Cursor Agent direct health=%+v, want one unknown provenance alternative", direct)
			}
			alternative := direct.Alternatives[0]
			if alternative.State != health.ComponentUnknown || len(alternative.Identifiers) != 1 || alternative.Identifiers[0] != "cursor-agent" || alternative.DiagnosticCode != health.DiagnosticProbeFailed || alternative.DiagnosticSummary != "verified cursor-agent provenance unavailable" || len(alternative.DiagnosticSummary) > 96 {
				t.Fatalf("Cursor Agent provenance alternative=%+v", alternative)
			}
		})
	}
}

func observationManagerNamed(name string) pkg.PackageManager {
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = name
	return manager
}
