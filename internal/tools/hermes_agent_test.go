package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
)

const hermesAgentPendingReason = "verified artifact and architecture support pending"

type hermesUnavailableReasonProvider interface {
	InstallationUnavailableReason() string
}

func TestHermesAgentRegistryMetadataIsDistinctAndUnsupported(t *testing.T) {
	tool, ok := NewRegistry().Get("hermes")
	if !ok {
		t.Fatal("hermes missing from registry")
	}
	if tool.ID() != "hermes" || tool.Name() != "Hermes Agent" || tool.Description() != "Autonomous AI agent runtime by Nous Research (discovery only)" || tool.Icon() != "\U000f06a9" || tool.Category() != CategoryUtility || tool.IsHeavy() {
		t.Fatalf("Hermes identity=%q/%q/%q icon=%q category=%q heavy=%v", tool.ID(), tool.Name(), tool.Description(), tool.Icon(), tool.Category(), tool.IsHeavy())
	}
	if tool.UIGroup() != UIGroupCLITools || tool.DefaultEnabled() || tool.PlatformFilter() != "" || ApplicationTypeOf(tool) != ApplicationTypeAIAgent {
		t.Fatalf("Hermes UI metadata=group:%q default:%v platform:%q type:%q", tool.UIGroup(), tool.DefaultEnabled(), tool.PlatformFilter(), ApplicationTypeOf(tool))
	}
	if tool.HasConfig() || tool.ConfigScreen() != 0 || len(tool.ConfigPaths()) != 0 || len(tool.Packages()) != 0 {
		t.Fatalf("Hermes advertised config/package/architecture support: screen=%d config=%v packages=%v", tool.ConfigScreen(), tool.ConfigPaths(), tool.Packages())
	}
	reason, ok := tool.(hermesUnavailableReasonProvider)
	if !ok || reason.InstallationUnavailableReason() != hermesAgentPendingReason {
		t.Fatalf("Hermes unavailable reason=(provider=%v,value=%q)", ok, func() string {
			if ok {
				return reason.InstallationUnavailableReason()
			}
			return ""
		}())
	}
	if tool.IsInstalled() {
		t.Fatal("unverified Hermes reported installed")
	}
	if _, legacy := tool.(DirectInstallationDetector); legacy {
		t.Fatal("Hermes must not expose the legacy boolean direct detector")
	}
	if _, recipe := tool.(InstallRecipeProvider); recipe {
		t.Fatal("Hermes must not publish an unverified install recipe")
	}
	if _, installer := tool.(interface{ InstallerAvailable(pkg.Platform) bool }); installer {
		t.Fatal("Hermes must not advertise structural platform installer availability")
	}
	policy, ok := tool.(interface{ PackageMetadataIsAuthoritative() bool })
	if !ok || policy.PackageMetadataIsAuthoritative() {
		t.Fatal("Hermes package metadata must be explicitly non-authoritative")
	}
	direct, ok := tool.(InstallationDirectAlternativesProvider)
	if !ok {
		t.Fatal("Hermes omitted typed provenance observation")
	}
	alternatives := direct.InstallationDirectAlternatives(DirectInstallationObservation{})
	if len(alternatives) != 1 {
		t.Fatalf("Hermes direct alternatives=%+v", alternatives)
	}
	assertHermesUnknownAlternative(t, alternatives[0])
}

func TestHermesAgentHasNoInstallOrPlatformRecipe(t *testing.T) {
	tool := NewHermesAgentTool()
	manager := pkg.NewMockPackageManager()
	if err := tool.Install(manager); err == nil || err.Error() != "hermes: "+hermesAgentPendingReason || len(manager.InstallCalls) != 0 {
		t.Fatalf("Hermes direct Install error=%v calls=%v", err, manager.InstallCalls)
	}
	for _, environment := range []InstallEnvironment{
		{Platform: pkg.PlatformMacOS, Manager: "brew"},
		{Platform: pkg.PlatformArch, Manager: "paru"},
		{Platform: pkg.PlatformDebian, Manager: "apt"},
		{Platform: pkg.PlatformPi, Manager: "apt"},
		{Platform: pkg.PlatformUnknown},
	} {
		recipe, err := DescribeInstall(tool, environment)
		if err == nil || err.Error() != "hermes: "+hermesAgentPendingReason {
			t.Fatalf("Hermes DescribeInstall(%+v) error=%v", environment, err)
		}
		if recipe.ToolID != "" || recipe.Platform != "" || recipe.Manager != "" || len(recipe.Steps) != 0 || len(recipe.Detector.Values) != 0 {
			t.Fatalf("failed Hermes recipe leaked support/execution input for %+v: %+v", environment, recipe)
		}
	}
	for _, platform := range []pkg.Platform{pkg.PlatformMacOS, pkg.PlatformArch, pkg.PlatformDebian, pkg.PlatformPi, pkg.PlatformUnknown} {
		manager := pkg.NewMockPackageManager()
		if err := tool.InstallForPlatform(manager, platform); err == nil || err.Error() != "hermes: "+hermesAgentPendingReason || len(manager.InstallCalls) != 0 {
			t.Fatalf("Hermes InstallForPlatform(%s)=(err=%v,calls=%v), want unavailable without mutation", platform, err, manager.InstallCalls)
		}
	}
}

func TestHermesAgentPATHCollisionsNeverBecomePresent(t *testing.T) {
	for _, test := range []struct {
		name string
		seed func(*testing.T, string)
	}{
		{name: "executable", seed: func(t *testing.T, dir string) { writeHermesCollision(t, filepath.Join(dir, "hermes"), 0o700) }},
		{name: "non executable", seed: func(t *testing.T, dir string) { writeHermesCollision(t, filepath.Join(dir, "hermes"), 0o600) }},
		{name: "symlink", seed: func(t *testing.T, dir string) {
			target := filepath.Join(dir, "foreign-hermes")
			writeHermesCollision(t, target, 0o700)
			if err := os.Symlink(target, filepath.Join(dir, "hermes")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(t.TempDir(), "executed")
			seed := test.seed
			// Seed functions create only inert collisions. Replace executable bytes
			// with a marker-writing script so any accidental probe execution is
			// independently observable.
			seed(t, dir)
			for _, name := range []string{"hermes", "foreign-hermes"} {
				path := filepath.Join(dir, name)
				if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
					if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf executed > \""+marker+"\"\n"), info.Mode().Perm()); err != nil {
						t.Fatal(err)
					}
				}
			}
			t.Setenv("PATH", dir)
			tool := NewHermesAgentTool()
			if tool.IsInstalled() {
				t.Fatal("unverified Hermes collision satisfied IsInstalled")
			}
			provider, ok := any(tool).(InstallationDirectAlternativesProvider)
			if !ok || len(provider.InstallationDirectAlternatives(DirectInstallationObservation{})) != 1 {
				t.Fatal("Hermes typed provenance provider missing")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("Hermes discovery executed PATH collision before health: %v", err)
			}
			snapshot, err := ObserveInstallationHealth(context.Background(), []Tool{tool}, hermesObservationManagerNamed("brew"), pkg.PlatformMacOS, 121)
			if err != nil {
				t.Fatal(err)
			}
			observation, ok := snapshot.Tool("hermes")
			if !ok || observation.Presence() == health.PresencePresent || observation.Presence() != health.PresenceUnknown {
				t.Fatalf("Hermes collision health=(found=%v,presence=%q)", ok, observation.Presence())
			}
			assertHermesDirectFacet(t, observation.Direct())
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("Hermes health executed PATH collision: %v", err)
			}
		})
	}
}

func TestHermesAgentTypedHealthIsUnknownAndUnsupportedEverywhere(t *testing.T) {
	for _, test := range []struct {
		name     string
		platform pkg.Platform
		manager  pkg.PackageManager
	}{
		{name: "macos", platform: pkg.PlatformMacOS, manager: hermesObservationManagerNamed("brew")},
		{name: "arch", platform: pkg.PlatformArch, manager: hermesObservationManagerNamed("paru")},
		{name: "debian", platform: pkg.PlatformDebian, manager: hermesObservationManagerNamed("apt")},
		{name: "pi", platform: pkg.PlatformPi, manager: hermesObservationManagerNamed("apt")},
		{name: "unknown platform with manager", platform: pkg.PlatformUnknown, manager: hermesObservationManagerNamed("custom")},
		{name: "unknown manager", platform: pkg.PlatformUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot, err := ObserveInstallationHealth(context.Background(), []Tool{NewHermesAgentTool()}, test.manager, test.platform, 123)
			if err != nil {
				t.Fatal(err)
			}
			observation, ok := snapshot.Tool("hermes")
			if !ok || observation.Presence() != health.PresenceUnknown || observation.InstallRecipeDigest() != "" {
				t.Fatalf("Hermes health=(found=%v,presence=%q,digest=%q)", ok, observation.Presence(), observation.InstallRecipeDigest())
			}
			if observation.Installability() != health.InstallabilityUnsupported {
				t.Fatalf("Hermes installability=%q, want unsupported", observation.Installability())
			}
			packageFacet := observation.Package()
			if packageFacet.State != health.PackageNotApplicable || packageFacet.Authoritative || len(packageFacet.ExpectedReceipts) != 0 || len(packageFacet.ObservedReceipts) != 0 || len(packageFacet.MissingReceipts) != 0 || len(packageFacet.UnresolvedReceipts) != 0 {
				t.Fatalf("Hermes package facet=%+v", observation.Package())
			}
			assertHermesDirectFacet(t, observation.Direct())
		})
	}
}

func assertHermesDirectFacet(t *testing.T, direct health.DirectFacet) {
	t.Helper()
	if direct.State != health.ComponentUnknown || !direct.Authoritative || direct.DiagnosticCode != health.DiagnosticProbeFailed || direct.DiagnosticSummary != "verified hermes provenance unavailable" || len(direct.Alternatives) != 1 {
		t.Fatalf("Hermes direct facet=%+v", direct)
	}
	assertHermesUnknownAlternative(t, direct.Alternatives[0])
}

func assertHermesUnknownAlternative(t *testing.T, alternative health.DirectAlternative) {
	t.Helper()
	if alternative.Kind != health.DirectSourceBinary || len(alternative.Identifiers) != 1 || alternative.Identifiers[0] != "hermes" || alternative.State != health.ComponentUnknown || alternative.DiagnosticCode != health.DiagnosticProbeFailed || alternative.DiagnosticSummary != "verified hermes provenance unavailable" || len(alternative.DiagnosticSummary) > 96 {
		t.Fatalf("Hermes direct alternative=%+v", alternative)
	}
}

func writeHermesCollision(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
		t.Fatal(err)
	}
}

func hermesObservationManagerNamed(name string) pkg.PackageManager {
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = name
	return manager
}
