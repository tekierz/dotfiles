package installplan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/planpublic"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestRealNPMToolHealthReachesPublicInstallPhases(t *testing.T) {
	for _, tool := range []tools.Tool{tools.NewCodexTool(), tools.NewPiTool()} {
		t.Run(tool.ID(), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
			bin := t.TempDir()
			t.Setenv("PATH", bin)
			manager := pkg.NewMockPackageManager()
			manager.ManagerName = "brew"
			environment := managerTestEnvironment(t, pkg.PlatformMacOS, "brew", 1)
			npmIdentity, _ := observedNPMExecutionIdentity(t, "real-observer")
			environment.NPMIdentity = npmIdentity
			recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
			if err != nil {
				t.Fatal(err)
			}
			for index, phase := range []string{"prerequisite", "npm", "present"} {
				if phase == "npm" {
					manager.InstalledPkgs["node"] = "24.20.0"
				}
				if phase == "present" {
					if err := os.WriteFile(filepath.Join(bin, tool.ID()), []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
						t.Fatal(err)
					}
				}
				environment.ExpectedGeneration = uint64(index + 1)
				snapshot, err := tools.ObserveInstallationHealth(context.Background(), []tools.Tool{tool}, manager, pkg.PlatformMacOS, environment.ExpectedGeneration)
				if err != nil {
					t.Fatal(err)
				}
				counts := &dependencyCounts{}
				result, err := BuildPhased(Request{Intent: mustIntent(t, tool.ID()), Snapshot: snapshot, Environment: environment}, countingDependencies(counts, recipe))
				if err != nil {
					t.Fatalf("phase=%s failed: %v", phase, err)
				}
				if phase == "present" {
					if result.Public().Status() != planpublic.StatusNoChanges || counts.capture != 0 {
						t.Fatalf("installed binary did not yield read-only no_changes: %+v", result.Public())
					}
					continue
				}
				publicPhase := result.Public().Phase()
				if result.Public().Status() != planpublic.StatusReady || publicPhase == nil || publicPhase.Kind != phase || publicPhase.Index != index+1 {
					t.Fatalf("phase=%s public status=%s phase=%+v", phase, result.Public().Status(), publicPhase)
				}
				if _, ok := result.Accepted(); !ok {
					t.Fatal("ready phase omitted private accepted plan")
				}
			}
		})
	}
}

func TestNPMPrerequisiteReceiptsRequireMatchingCompleteNamespace(t *testing.T) {
	for _, name := range []string{"present", "missing", "wrong-provider", "wrong-manager", "cask-only", "cask-with-missing-formula", "unknown", "wrong-receipt", "no-namespace", "malformed-recipe", "wrong-step-provider", "system"} {
		t.Run(name, func(t *testing.T) {
			recipe := mustDescribedRecipe(t, "codex")
			digest, err := operation.InstallRecipeDigest(recipe)
			if err != nil {
				t.Fatal(err)
			}
			prerequisite, ok := npmPrerequisitePhaseRecipe(recipe)
			if !ok {
				t.Fatal("missing prerequisite fixture")
			}
			manager := "brew"
			facet := health.PackageFacet{
				State: health.PackagePresent, Provider: manager, ExpectedReceipts: []string{"node"}, ObservedReceipts: []string{"node"}, Complete: true,
				Namespaces: []health.PackageNamespaceFacet{{Namespace: health.PackageNamespaceFormula, State: health.PackagePresent, ExpectedReceipts: []string{"node"}, ObservedReceipts: []string{"node"}, Complete: true}},
			}
			wantInstalled, wantKnown := false, false
			switch name {
			case "present":
				wantInstalled, wantKnown = true, true
			case "missing":
				facet.State, facet.ObservedReceipts, facet.MissingReceipts = health.PackageMissing, nil, []string{"node"}
				facet.Namespaces[0].State, facet.Namespaces[0].ObservedReceipts, facet.Namespaces[0].MissingReceipts = health.PackageMissing, nil, []string{"node"}
				wantKnown = true
			case "wrong-provider":
				facet.Provider = "apt"
			case "wrong-manager":
				manager = "apt"
			case "cask-only":
				facet.Namespaces[0].Namespace = health.PackageNamespaceCask
			case "cask-with-missing-formula":
				facet.Namespaces[0].Namespace = health.PackageNamespaceCask
				facet.Namespaces = append(facet.Namespaces, health.PackageNamespaceFacet{Namespace: health.PackageNamespaceFormula, State: health.PackageMissing, ExpectedReceipts: []string{"node"}, MissingReceipts: []string{"node"}, Complete: true})
				wantKnown = true
			case "unknown":
				facet.State, facet.ObservedReceipts, facet.UnresolvedReceipts, facet.Complete = health.PackageUnknown, nil, []string{"node"}, false
				facet.Namespaces[0].State, facet.Namespaces[0].ObservedReceipts, facet.Namespaces[0].UnresolvedReceipts, facet.Namespaces[0].Complete = health.PackageUnknown, nil, []string{"node"}, false
			case "wrong-receipt":
				facet.ExpectedReceipts, facet.ObservedReceipts = []string{"other"}, []string{"other"}
				facet.Namespaces[0].ExpectedReceipts, facet.Namespaces[0].ObservedReceipts = []string{"other"}, []string{"other"}
			case "no-namespace":
				facet.Namespaces = nil
			case "malformed-recipe":
				prerequisite.SchemaVersion = 0
			case "wrong-step-provider":
				prerequisite.Steps[0].Provider = "apt"
			case "system":
				manager, facet.Provider, prerequisite.Manager, prerequisite.Steps[0].Provider = "apt", "apt", "apt", "apt"
				prerequisite.Platform = string(pkg.PlatformDebian)
				facet.Namespaces[0].Namespace = health.PackageNamespaceSystem
				wantInstalled, wantKnown = true, true
			}
			observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{ToolID: "codex", Installability: health.InstallabilitySupported, InstallRecipeDigest: digest, Package: facet, Direct: health.DirectFacet{State: health.ComponentNotApplicable}})
			if err != nil {
				t.Fatal(err)
			}
			installed, known := observedNPMPrerequisiteReceipts(observation, prerequisite, manager)
			if installed != wantInstalled || known != wantKnown {
				t.Fatalf("prerequisite=(%t,%t), want (%t,%t)", installed, known, wantInstalled, wantKnown)
			}
			if _, known := ObservedInstallDetector(observation, operation.InstallDetector{Kind: operation.InstallDetectorPackageReceipt, Values: []string{"node"}}); known {
				t.Fatal("generic product detector accepted nonauthoritative prerequisite receipts")
			}
		})
	}
}
