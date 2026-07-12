package health

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const validRecipeDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func fullDigestSpec() InstallationObservationSpec {
	return InstallationObservationSpec{
		ToolID: "tool", Installability: InstallabilitySupported, InstallRecipeDigest: validRecipeDigest,
		Package: PackageFacet{
			State: PackagePartial, Provider: "brew", ExpectedReceipts: []string{"unknown", "formula", "cask"}, ObservedReceipts: []string{"formula"}, MissingReceipts: []string{"cask"}, UnresolvedReceipts: []string{"unknown"}, Authoritative: true, Complete: false,
			DiagnosticCode: DiagnosticPackageBatchFailed, DiagnosticSummary: "one package namespace failed",
			Namespaces: []PackageNamespaceFacet{
				{Namespace: PackageNamespaceFormula, State: PackagePartial, ExpectedReceipts: []string{"unknown", "formula"}, ObservedReceipts: []string{"formula"}, UnresolvedReceipts: []string{"unknown"}, Complete: false, DiagnosticCode: DiagnosticPackageBatchFailed, DiagnosticSummary: "formula failed"},
				{Namespace: PackageNamespaceCask, State: PackageUnknown, ExpectedReceipts: []string{"unknown", "cask"}, MissingReceipts: []string{"cask"}, UnresolvedReceipts: []string{"unknown"}, Complete: false, DiagnosticCode: DiagnosticPackageBatchFailed, DiagnosticSummary: "cask failed"},
			},
		},
		Direct: DirectFacet{
			State: ComponentUnknown, Authoritative: true, DiagnosticCode: DiagnosticProbeFailed, DiagnosticSummary: "one direct probe failed",
			Alternatives: []DirectAlternative{
				{Kind: DirectSourceBinary, Identifiers: []string{"tool-z", "tool-a"}, State: ComponentUnknown, DiagnosticCode: DiagnosticProbeFailed, DiagnosticSummary: "binary failed"},
				{Kind: DirectSourceAppBundle, Identifiers: []string{"com.example.Tool"}, State: ComponentMissing},
			},
		},
	}
}

func mustDigestObservation(t *testing.T, spec InstallationObservationSpec) InstallationObservation {
	t.Helper()
	observation, err := NewInstallationObservation(spec)
	if err != nil {
		t.Fatalf("construct digest observation: %v", err)
	}
	return observation
}

func mustDigestSnapshot(t *testing.T, generation uint64, platform, manager string, observations ...InstallationObservation) InstallationSnapshot {
	t.Helper()
	snapshot, err := NewInstallationSnapshot(InstallationSnapshotSpec{Generation: generation, Platform: platform, Manager: manager, Tools: observations})
	if err != nil {
		t.Fatalf("construct digest snapshot: %v", err)
	}
	return snapshot
}

func TestInstallationObservationRecipeDigestStrictLowerHexContract(t *testing.T) {
	observation := mustDigestObservation(t, fullDigestSpec())
	if observation.InstallRecipeDigest() != validRecipeDigest {
		t.Fatalf("recipe digest=%q", observation.InstallRecipeDigest())
	}
	invalid := map[string]string{
		"empty": "", "63": strings.Repeat("a", 63), "65": strings.Repeat("a", 65), "uppercase": strings.Repeat("A", 64), "mixed case": "a" + strings.Repeat("B", 63),
		"leading whitespace": " " + strings.Repeat("a", 63), "trailing whitespace": strings.Repeat("a", 63) + " ", "non-ascii": strings.Repeat("a", 62) + "é", "control": strings.Repeat("a", 63) + "\n",
	}
	for name, digest := range invalid {
		t.Run(name, func(t *testing.T) {
			spec := fullDigestSpec()
			spec.InstallRecipeDigest = digest
			if _, err := NewInstallationObservation(spec); err == nil {
				t.Fatal("invalid digest accepted")
			}
		})
	}
	for _, installability := range []Installability{InstallabilityUnsupported, InstallabilityUnknown} {
		t.Run("non-supported "+string(installability), func(t *testing.T) {
			spec := fullDigestSpec()
			spec.Installability, spec.InstallRecipeDigest = installability, ""
			observation := mustDigestObservation(t, spec)
			if observation.InstallRecipeDigest() != "" {
				t.Fatalf("non-supported digest=%q", observation.InstallRecipeDigest())
			}
			spec.InstallRecipeDigest = validRecipeDigest
			if _, err := NewInstallationObservation(spec); err == nil {
				t.Fatal("non-supported observation accepted digest")
			}
		})
	}
}

func TestInstallationSnapshotDigestBindsEveryCanonicalField(t *testing.T) {
	baselineSpec := fullDigestSpec()
	baseline := mustDigestSnapshot(t, 7, "macos", "brew", mustDigestObservation(t, baselineSpec))
	mutations := map[string]func(*InstallationObservationSpec){
		"tool ID": func(s *InstallationObservationSpec) { s.ToolID = "other-tool" },
		"installability": func(s *InstallationObservationSpec) {
			s.Installability, s.InstallRecipeDigest = InstallabilityUnknown, ""
		},
		"recipe digest":              func(s *InstallationObservationSpec) { s.InstallRecipeDigest = strings.Repeat("b", 64) },
		"package provider":           func(s *InstallationObservationSpec) { s.Package.Provider = "apt" },
		"package authority":          func(s *InstallationObservationSpec) { s.Package.Authoritative = false },
		"package diagnostic code":    func(s *InstallationObservationSpec) { s.Package.DiagnosticCode = DiagnosticFallbackTimeout },
		"package diagnostic summary": func(s *InstallationObservationSpec) { s.Package.DiagnosticSummary = "different package summary" },
		"namespace diagnostic code": func(s *InstallationObservationSpec) {
			s.Package.Namespaces[0].DiagnosticCode = DiagnosticFallbackTimeout
		},
		"namespace diagnostic summary": func(s *InstallationObservationSpec) {
			s.Package.Namespaces[0].DiagnosticSummary = "different namespace summary"
		},
		"direct authority":          func(s *InstallationObservationSpec) { s.Direct.Authoritative = false },
		"direct diagnostic code":    func(s *InstallationObservationSpec) { s.Direct.DiagnosticCode = DiagnosticProbeTimeout },
		"direct diagnostic summary": func(s *InstallationObservationSpec) { s.Direct.DiagnosticSummary = "different direct summary" },
		"alternative kind":          func(s *InstallationObservationSpec) { s.Direct.Alternatives[1].Kind = DirectSourceFlatpak },
		"alternative identifiers": func(s *InstallationObservationSpec) {
			s.Direct.Alternatives[1].Identifiers = []string{"com.example.Other"}
		},
		"alternative state":           func(s *InstallationObservationSpec) { s.Direct.Alternatives[1].State = ComponentNotApplicable },
		"alternative diagnostic code": func(s *InstallationObservationSpec) { s.Direct.Alternatives[0].DiagnosticCode = DiagnosticProbeTimeout },
		"alternative diagnostic summary": func(s *InstallationObservationSpec) {
			s.Direct.Alternatives[0].DiagnosticSummary = "different alternative summary"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			spec := fullDigestSpec()
			mutate(&spec)
			changed := mustDigestSnapshot(t, 7, "macos", "brew", mustDigestObservation(t, spec))
			if changed.Digest() == baseline.Digest() {
				t.Fatal("field did not affect digest")
			}
		})
	}

	// Partition/state/complete/membership fields are necessarily correlated by
	// the domain invariants; each fixture remains valid while changing that axis.
	correlated := map[string]InstallationObservationSpec{}
	present := fullDigestSpec()
	present.Package.State, present.Package.ObservedReceipts, present.Package.MissingReceipts, present.Package.UnresolvedReceipts, present.Package.Complete = PackagePresent, []string{"cask", "formula", "unknown"}, nil, nil, true
	present.Package.Namespaces = []PackageNamespaceFacet{{Namespace: PackageNamespaceSystem, State: PackagePresent, ExpectedReceipts: []string{"cask", "formula", "unknown"}, ObservedReceipts: []string{"cask", "formula", "unknown"}, Complete: true}}
	correlated["package state/observed/missing/unresolved/complete invariant-bound group"] = present
	membership := fullDigestSpec()
	membership.Package.State, membership.Package.ExpectedReceipts, membership.Package.ObservedReceipts, membership.Package.MissingReceipts, membership.Package.UnresolvedReceipts, membership.Package.Complete = PackagePresent, []string{"formula"}, []string{"formula"}, nil, nil, true
	membership.Package.Namespaces = []PackageNamespaceFacet{{Namespace: PackageNamespaceFormula, State: PackagePresent, ExpectedReceipts: []string{"formula"}, ObservedReceipts: []string{"formula"}, Complete: true}}
	correlated["namespace membership/expected/state/partition/complete invariant-bound group"] = membership
	direct := fullDigestSpec()
	direct.Direct.State = ComponentPresent
	direct.Direct.Alternatives = append(direct.Direct.Alternatives, DirectAlternative{Kind: DirectSourceLegacy, Identifiers: []string{"tool"}, State: ComponentPresent})
	correlated["direct state and alternative membership invariant-bound group"] = direct
	for name, spec := range correlated {
		t.Run(name, func(t *testing.T) {
			changed := mustDigestSnapshot(t, 7, "macos", "brew", mustDigestObservation(t, spec))
			if changed.Digest() == baseline.Digest() {
				t.Fatal("correlated canonical fields did not affect digest")
			}
		})
	}
	for name, snapshot := range map[string]InstallationSnapshot{
		"generation": mustDigestSnapshot(t, 8, "macos", "brew", mustDigestObservation(t, baselineSpec)),
		"platform":   mustDigestSnapshot(t, 7, "arch", "brew", mustDigestObservation(t, baselineSpec)),
		"manager":    mustDigestSnapshot(t, 7, "macos", "apt", mustDigestObservation(t, baselineSpec)),
	} {
		if snapshot.Digest() == baseline.Digest() {
			t.Errorf("%s did not affect digest", name)
		}
	}
}

func TestInstallationSnapshotDigestCanonicalizesAllInputOrdering(t *testing.T) {
	firstSpec := fullDigestSpec()
	secondSpec := fullDigestSpec()
	slices.Reverse(secondSpec.Package.ExpectedReceipts)
	slices.Reverse(secondSpec.Package.Namespaces)
	for index := range secondSpec.Package.Namespaces {
		slices.Reverse(secondSpec.Package.Namespaces[index].ExpectedReceipts)
		slices.Reverse(secondSpec.Package.Namespaces[index].ObservedReceipts)
		slices.Reverse(secondSpec.Package.Namespaces[index].MissingReceipts)
		slices.Reverse(secondSpec.Package.Namespaces[index].UnresolvedReceipts)
	}
	slices.Reverse(secondSpec.Direct.Alternatives)
	for index := range secondSpec.Direct.Alternatives {
		slices.Reverse(secondSpec.Direct.Alternatives[index].Identifiers)
	}
	first, second := mustDigestObservation(t, firstSpec), mustDigestObservation(t, secondSpec)
	otherSpec := fullDigestSpec()
	otherSpec.ToolID, otherSpec.InstallRecipeDigest = "zeta", strings.Repeat("b", 64)
	other := mustDigestObservation(t, otherSpec)
	one := mustDigestSnapshot(t, 7, "macos", "brew", other, first)
	two := mustDigestSnapshot(t, 7, "macos", "brew", second, other)
	if one.Digest() != two.Digest() || !reflect.DeepEqual(one.CanonicalBytes(), two.CanonicalBytes()) {
		t.Fatalf("input order changed canonical identity: %q != %q", one.Digest(), two.Digest())
	}
}

func TestInstallationSnapshotDigestResistsAllConstructorAndAccessorAliases(t *testing.T) {
	spec := fullDigestSpec()
	expected := slices.Clone(spec.Package.ExpectedReceipts)
	namespaces := spec.Package.Namespaces
	alternatives := spec.Direct.Alternatives
	observation := mustDigestObservation(t, spec)
	snapshotInput := []InstallationObservation{observation}
	snapshot := mustDigestSnapshot(t, 7, "macos", "brew", snapshotInput...)
	digest, canonical := snapshot.Digest(), slices.Clone(snapshot.CanonicalBytes())
	expected[0] = "caller-only"
	spec.Package.ExpectedReceipts[0] = "mutated"
	namespaces[0].ExpectedReceipts[0] = "mutated"
	namespaces[0] = PackageNamespaceFacet{}
	alternatives[0].Identifiers[0] = "mutated"
	alternatives[0] = DirectAlternative{}
	snapshotInput[0] = InstallationObservation{}
	returned := snapshot.Tools()
	returned[0] = InstallationObservation{}
	lookup, ok := snapshot.Tool("tool")
	if !ok {
		t.Fatal("lookup missing")
	}
	packageFacet, directFacet := lookup.Package(), lookup.Direct()
	packageFacet.ExpectedReceipts[0] = "mutated"
	packageFacet.Namespaces[0].ExpectedReceipts[0] = "mutated"
	packageFacet.Namespaces[0] = PackageNamespaceFacet{}
	directFacet.Alternatives[0].Identifiers[0] = "mutated"
	directFacet.Alternatives[0] = DirectAlternative{}
	if snapshot.Digest() != digest || !reflect.DeepEqual(snapshot.CanonicalBytes(), canonical) {
		t.Fatal("alias mutation changed snapshot identity")
	}
	again, _ := snapshot.Tool("tool")
	if !reflect.DeepEqual(again.Package().ExpectedReceipts, []string{"cask", "formula", "unknown"}) || len(again.Package().Namespaces) != 2 || len(again.Direct().Alternatives) != 2 {
		t.Fatalf("alias mutation changed returned content: package=%+v direct=%+v", again.Package(), again.Direct())
	}
}

func TestInstallationSnapshotCanonicalBytesBindSchemaAndDigest(t *testing.T) {
	snapshot := mustDigestSnapshot(t, 7, "macos", "brew", mustDigestObservation(t, fullDigestSpec()))
	expected := []byte(`{"schema_version":1,"generation":7,"platform":"macos","manager":"brew","tools":[{"tool_id":"tool","installability":"supported","install_recipe_digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","presence":"partial","package":{"state":"partial","provider":"brew","expected_receipts":["cask","formula","unknown"],"observed_receipts":["formula"],"missing_receipts":["cask"],"unresolved_receipts":["unknown"],"authoritative":true,"complete":false,"namespaces":[{"namespace":"cask","state":"unknown","expected_receipts":["cask","unknown"],"observed_receipts":[],"missing_receipts":["cask"],"unresolved_receipts":["unknown"],"complete":false,"diagnostic_code":"package_batch_failed","diagnostic_summary":"cask failed"},{"namespace":"formula","state":"partial","expected_receipts":["formula","unknown"],"observed_receipts":["formula"],"missing_receipts":[],"unresolved_receipts":["unknown"],"complete":false,"diagnostic_code":"package_batch_failed","diagnostic_summary":"formula failed"}],"diagnostic_code":"package_batch_failed","diagnostic_summary":"one package namespace failed"},"direct":{"state":"unknown","authoritative":true,"alternatives":[{"kind":"app_bundle","identifiers":["com.example.Tool"],"state":"missing","diagnostic_code":"","diagnostic_summary":""},{"kind":"binary","identifiers":["tool-a","tool-z"],"state":"unknown","diagnostic_code":"probe_failed","diagnostic_summary":"binary failed"}],"diagnostic_code":"probe_failed","diagnostic_summary":"one direct probe failed"}}]}`)
	const expectedDigest = "8378bf42bc0541d551013f9889a04aa0f328d1f58d65766a41774f9c33849164"
	canonical := snapshot.CanonicalBytes()
	if !reflect.DeepEqual(canonical, expected) {
		t.Fatalf("canonical bytes changed:\n got: %s\nwant: %s", canonical, expected)
	}
	hash := sha256.Sum256(expected)
	if got := hex.EncodeToString(hash[:]); got != expectedDigest {
		t.Fatalf("test golden SHA typo: computed %q want literal %q", got, expectedDigest)
	}
	if snapshot.Digest() != expectedDigest {
		t.Fatalf("digest=%q want golden %q", snapshot.Digest(), expectedDigest)
	}
	canonical[0] ^= 0xff
	if snapshot.Digest() != expectedDigest {
		t.Fatal("CanonicalBytes exposed mutable snapshot storage")
	}
}

func TestInstallationSnapshotDigestZeroAndMaximumBoundaries(t *testing.T) {
	if got := (InstallationSnapshot{}).Digest(); got != "" {
		t.Fatalf("zero digest=%q", got)
	}
	tools := make([]InstallationObservation, MaxInstallationTools)
	for index := range tools {
		spec := InstallationObservationSpec{ToolID: fmt.Sprintf("tool-%04d", index), Installability: InstallabilityUnknown, Package: PackageFacet{State: PackageNotApplicable}, Direct: DirectFacet{State: ComponentNotApplicable}}
		tools[index] = mustDigestObservation(t, spec)
	}
	snapshot := mustDigestSnapshot(t, 1, "macos", "brew", tools...)
	if len(snapshot.Digest()) != 64 {
		t.Fatalf("max-boundary digest=%q", snapshot.Digest())
	}
}
