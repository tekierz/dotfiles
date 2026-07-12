package health

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func packageNamespaceForTest(t *testing.T, facet PackageFacet, namespace PackageNamespace) PackageNamespaceFacet {
	t.Helper()
	for _, candidate := range facet.Namespaces {
		if candidate.Namespace == namespace {
			return candidate
		}
	}
	t.Fatalf("missing namespace %q in %+v", namespace, facet)
	return PackageNamespaceFacet{}
}

func TestNewInstallationObservationValidatesVersionOneDomain(t *testing.T) {
	valid := InstallationObservationSpec{
		ToolID:         "ghostty",
		Installability: InstallabilitySupported,
		Package: PackageFacet{
			State:            PackagePartial,
			Provider:         "brew",
			ExpectedReceipts: []string{"ghostty", "ghostty-fonts"},
			ObservedReceipts: []string{"ghostty"},
			MissingReceipts:  []string{"ghostty-fonts"},
			Authoritative:    true,
			Complete:         true,
			Namespaces: []PackageNamespaceFacet{
				{Namespace: PackageNamespaceFormula, State: PackagePresent, ExpectedReceipts: []string{"ghostty"}, ObservedReceipts: []string{"ghostty"}, Complete: true},
				{Namespace: PackageNamespaceCask, State: PackageMissing, ExpectedReceipts: []string{"ghostty-fonts"}, MissingReceipts: []string{"ghostty-fonts"}, Complete: true},
			},
		},
		Direct: DirectFacet{
			State: ComponentPresent,
			Alternatives: []DirectAlternative{{
				Kind:        DirectSourceAppBundle,
				Identifiers: []string{"com.mitchellh.ghostty"},
				State:       ComponentPresent,
			}},
		},
	}
	if _, err := NewInstallationObservation(valid); err != nil {
		t.Fatalf("valid observation: %v", err)
	}

	for name, mutate := range map[string]func(*InstallationObservationSpec){
		"empty tool ID":   func(spec *InstallationObservationSpec) { spec.ToolID = "" },
		"invalid support": func(spec *InstallationObservationSpec) { spec.Installability = Installability("maybe") },
		"invalid code": func(spec *InstallationObservationSpec) {
			spec.Package.DiagnosticCode = DiagnosticCode("whatever")
		},
		"invalid package": func(spec *InstallationObservationSpec) { spec.Package.State = PackageState("broken") },
		"inconsistent package": func(spec *InstallationObservationSpec) {
			spec.Package.State = PackagePresent
		},
		"invalid direct": func(spec *InstallationObservationSpec) { spec.Direct.State = ComponentState("maybe") },
		"invalid source kind": func(spec *InstallationObservationSpec) {
			spec.Direct.Alternatives = []DirectAlternative{{Kind: DirectSourceKind("shell"), State: ComponentPresent}}
		},
		"control identifier": func(spec *InstallationObservationSpec) {
			spec.Direct.Alternatives = []DirectAlternative{{Kind: DirectSourceBinary, State: ComponentPresent, Identifiers: []string{"bad\nname"}}}
		},
		"bidi identifier": func(spec *InstallationObservationSpec) {
			spec.Direct.Alternatives = []DirectAlternative{{Kind: DirectSourceBinary, State: ComponentPresent, Identifiers: []string{"safe\u202Eexe"}}}
		},
		"oversize identifier": func(spec *InstallationObservationSpec) {
			spec.Direct.Alternatives = []DirectAlternative{{Kind: DirectSourceBinary, State: ComponentPresent, Identifiers: []string{strings.Repeat("a", 257)}}}
		},
		"too many identifiers": func(spec *InstallationObservationSpec) {
			identifiers := make([]string, 65)
			for index := range identifiers {
				identifiers[index] = fmt.Sprintf("binary-%d", index)
			}
			spec.Direct.Alternatives = []DirectAlternative{{Kind: DirectSourceBinary, State: ComponentPresent, Identifiers: identifiers}}
		},
		"inconsistent direct": func(spec *InstallationObservationSpec) {
			spec.Direct.State = ComponentMissing
		},
	} {
		t.Run(name, func(t *testing.T) {
			spec := valid
			mutate(&spec)
			if _, err := NewInstallationObservation(spec); err == nil {
				t.Fatal("invalid observation was accepted")
			}
		})
	}
}

func TestInstallationObservationDerivesPresenceFromIndependentAlternatives(t *testing.T) {
	observation, err := NewInstallationObservation(InstallationObservationSpec{
		ToolID:         "mixed",
		Installability: InstallabilityUnsupported,
		Package: PackageFacet{
			State: PackageUnknown, ExpectedReceipts: []string{"receipt"}, Complete: false,
		},
		Direct: DirectFacet{
			State: ComponentPresent,
			Alternatives: []DirectAlternative{
				{Kind: DirectSourceBinary, Identifiers: []string{"mixed"}, State: ComponentUnknown},
				{Kind: DirectSourceFlatpak, Identifiers: []string{"com.example.Mixed"}, State: ComponentPresent},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if observation.Presence() != PresencePresent || observation.Installability() != InstallabilityUnsupported {
		t.Fatalf("presence=%q installability=%q", observation.Presence(), observation.Installability())
	}
	got := observation.Direct().Alternatives
	if len(got) != 2 || got[0].Kind != DirectSourceBinary || got[0].State != ComponentUnknown || got[1].Kind != DirectSourceFlatpak || got[1].State != ComponentPresent {
		t.Fatalf("mixed alternatives were not preserved: %+v", got)
	}
}

func TestDirectFacetAggregateTruthTable(t *testing.T) {
	for _, tc := range []struct {
		name          string
		alternatives  []DirectAlternative
		state         ComponentState
		authoritative bool
		presence      Presence
	}{
		{name: "missing and unknown", alternatives: []DirectAlternative{{Kind: DirectSourceBinary, Identifiers: []string{"direct"}, State: ComponentMissing}, {Kind: DirectSourceFlatpak, Identifiers: []string{"com.example.Direct"}, State: ComponentUnknown}}, state: ComponentUnknown, presence: PresenceUnknown},
		{name: "all missing authoritative", alternatives: []DirectAlternative{{Kind: DirectSourceBinary, Identifiers: []string{"direct"}, State: ComponentMissing}, {Kind: DirectSourceAppBundle, Identifiers: []string{"com.example.Direct"}, State: ComponentMissing}}, state: ComponentMissing, authoritative: true, presence: PresenceMissing},
		{name: "all missing non-authoritative", alternatives: []DirectAlternative{{Kind: DirectSourceBinary, Identifiers: []string{"direct"}, State: ComponentMissing}, {Kind: DirectSourceAppBundle, Identifiers: []string{"com.example.Direct"}, State: ComponentMissing}}, state: ComponentMissing, presence: PresenceUnknown},
		{name: "all not applicable", alternatives: []DirectAlternative{{Kind: DirectSourceFlatpak, Identifiers: []string{"com.example.Direct"}, State: ComponentNotApplicable}}, state: ComponentNotApplicable, presence: PresenceUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observation, err := NewInstallationObservation(InstallationObservationSpec{
				ToolID: "direct", Installability: InstallabilityUnknown,
				Package: PackageFacet{State: PackageNotApplicable},
				Direct:  DirectFacet{State: tc.state, Authoritative: tc.authoritative, Alternatives: tc.alternatives},
			})
			if err != nil {
				t.Fatal(err)
			}
			if observation.Direct().State != tc.state || observation.Presence() != tc.presence {
				t.Fatalf("direct=%q presence=%q", observation.Direct().State, observation.Presence())
			}
		})
	}
}

func TestInstallationPresenceAuthorityTruthTable(t *testing.T) {
	packageFacet := func(state PackageState, authoritative bool) PackageFacet {
		facet := PackageFacet{State: state, Provider: "brew", Authoritative: authoritative}
		switch state {
		case PackagePresent:
			facet.ExpectedReceipts, facet.ObservedReceipts, facet.Complete = []string{"one"}, []string{"one"}, true
		case PackagePartial:
			facet.ExpectedReceipts, facet.ObservedReceipts, facet.MissingReceipts, facet.Complete = []string{"one", "two"}, []string{"one"}, []string{"two"}, true
		case PackageMissing:
			facet.ExpectedReceipts, facet.MissingReceipts, facet.Complete = []string{"one"}, []string{"one"}, true
		case PackageUnknown:
			facet.ExpectedReceipts, facet.UnresolvedReceipts = []string{"one"}, []string{"one"}
		case PackageNotApplicable:
			// No receipt evidence is valid for a non-applicable package facet.
		}
		return facet
	}
	directFacet := func(state ComponentState, authoritative bool) DirectFacet {
		return DirectFacet{State: state, Authoritative: authoritative, Alternatives: []DirectAlternative{{Kind: DirectSourceBinary, Identifiers: []string{"tool"}, State: state}}}
	}
	for _, tc := range []struct {
		name     string
		pkg      PackageFacet
		direct   DirectFacet
		presence Presence
	}{
		{name: "direct authoritative missing receipt present", pkg: packageFacet(PackagePresent, false), direct: directFacet(ComponentMissing, true), presence: PresencePartial},
		{name: "direct authoritative missing receipt absent", pkg: packageFacet(PackageMissing, false), direct: directFacet(ComponentMissing, true), presence: PresenceMissing},
		{name: "direct authoritative missing package n/a", pkg: PackageFacet{State: PackageNotApplicable}, direct: directFacet(ComponentMissing, true), presence: PresenceMissing},
		{name: "direct unknown receipt present", pkg: packageFacet(PackagePresent, false), direct: directFacet(ComponentUnknown, true), presence: PresencePartial},
		{name: "direct positive dominates package unknown", pkg: packageFacet(PackageUnknown, true), direct: directFacet(ComponentPresent, true), presence: PresencePresent},
		{name: "package authoritative missing direct unknown", pkg: packageFacet(PackageMissing, true), direct: directFacet(ComponentUnknown, false), presence: PresenceUnknown},
		{name: "package partial never present", pkg: packageFacet(PackagePartial, true), direct: DirectFacet{State: ComponentNotApplicable}, presence: PresencePartial},
		{name: "non-authoritative receipt positive direct n/a", pkg: packageFacet(PackagePresent, false), direct: DirectFacet{State: ComponentNotApplicable}, presence: PresencePartial},
		{name: "non-authoritative missing cannot prove absence", pkg: packageFacet(PackageMissing, false), direct: DirectFacet{State: ComponentNotApplicable}, presence: PresenceUnknown},
		{name: "non-authoritative partial retains positive", pkg: packageFacet(PackagePartial, false), direct: DirectFacet{State: ComponentNotApplicable}, presence: PresencePartial},
		{name: "direct authority proves missing", pkg: packageFacet(PackageMissing, false), direct: directFacet(ComponentMissing, true), presence: PresenceMissing},
		{name: "direct authority unknown with receipt positive", pkg: packageFacet(PackagePresent, false), direct: directFacet(ComponentUnknown, true), presence: PresencePartial},
		{name: "direct authority unknown without positive", pkg: packageFacet(PackageMissing, false), direct: directFacet(ComponentUnknown, true), presence: PresenceUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observation, err := NewInstallationObservation(InstallationObservationSpec{ToolID: "tool", Installability: InstallabilitySupported, Package: tc.pkg, Direct: tc.direct})
			if err != nil {
				t.Fatal(err)
			}
			if got := observation.Presence(); got != tc.presence {
				t.Fatalf("presence=%q want %q (package=%+v direct=%+v)", got, tc.presence, observation.Package(), observation.Direct())
			}
		})
	}
}

func TestInstallationDiagnosticIsStableBoundedAndSanitized(t *testing.T) {
	raw := "brew failed\x00\n\u0085\x1b[31m\u202E" + strings.Repeat("界", 300)
	observation, err := NewInstallationObservation(InstallationObservationSpec{
		ToolID:         "safe",
		Installability: InstallabilitySupported,
		Package: PackageFacet{
			State: PackageUnknown, ExpectedReceipts: []string{"safe"}, Complete: false,
			DiagnosticCode: DiagnosticPackageBatchFailed, DiagnosticSummary: raw,
		},
		Direct: DirectFacet{State: ComponentNotApplicable},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := observation.Package()
	if got.DiagnosticCode != DiagnosticPackageBatchFailed {
		t.Fatalf("diagnostic code=%q", got.DiagnosticCode)
	}
	if len(got.DiagnosticSummary) > 256 || !utf8.ValidString(got.DiagnosticSummary) || strings.ContainsAny(got.DiagnosticSummary, "\x00\x1b\r\n\u0085") || strings.ContainsRune(got.DiagnosticSummary, '\u202e') || strings.Contains(got.DiagnosticSummary, "[31m") {
		t.Fatalf("diagnostic is unbounded or contains controls: %q", got.DiagnosticSummary)
	}
}

func TestInstallationObservationRejectsDuplicateConflictingAndOversizeEvidence(t *testing.T) {
	base := func() InstallationObservationSpec {
		return InstallationObservationSpec{
			ToolID: "tool", Installability: InstallabilitySupported,
			Package: PackageFacet{State: PackageMissing, Provider: "brew", ExpectedReceipts: []string{"one"}, MissingReceipts: []string{"one"}, Complete: true},
			Direct:  DirectFacet{State: ComponentMissing, Alternatives: []DirectAlternative{{Kind: DirectSourceBinary, Identifiers: []string{"tool"}, State: ComponentMissing}}},
		}
	}
	for name, mutate := range map[string]func(*InstallationObservationSpec){
		"duplicate receipt": func(spec *InstallationObservationSpec) {
			spec.Package.ExpectedReceipts, spec.Package.MissingReceipts = []string{"one", "one"}, []string{"one", "one"}
		},
		"conflicting receipt sets": func(spec *InstallationObservationSpec) { spec.Package.ObservedReceipts = []string{"one"} },
		"observed unresolved conflict": func(spec *InstallationObservationSpec) {
			spec.Package.ObservedReceipts, spec.Package.UnresolvedReceipts = []string{"one"}, []string{"one"}
		},
		"missing unresolved conflict": func(spec *InstallationObservationSpec) { spec.Package.UnresolvedReceipts = []string{"one"} },
		"duplicate namespaces": func(spec *InstallationObservationSpec) {
			spec.Package.Namespaces = []PackageNamespaceFacet{
				{Namespace: PackageNamespaceFormula, State: PackageMissing, ExpectedReceipts: []string{"one"}, MissingReceipts: []string{"one"}, Complete: true},
				{Namespace: PackageNamespaceFormula, State: PackageMissing, ExpectedReceipts: []string{"one"}, MissingReceipts: []string{"one"}, Complete: true},
			}
		},
		"duplicate identifier": func(spec *InstallationObservationSpec) {
			spec.Direct.Alternatives[0].Identifiers = []string{"tool", "tool"}
		},
		"empty identifiers": func(spec *InstallationObservationSpec) { spec.Direct.Alternatives[0].Identifiers = nil },
		"duplicate direct alternative": func(spec *InstallationObservationSpec) {
			spec.Direct.Alternatives = append(spec.Direct.Alternatives, spec.Direct.Alternatives[0])
		},
		"too many direct alternatives": func(spec *InstallationObservationSpec) {
			spec.Direct.Alternatives = make([]DirectAlternative, 65)
			for index := range spec.Direct.Alternatives {
				spec.Direct.Alternatives[index] = DirectAlternative{Kind: DirectSourceBinary, Identifiers: []string{fmt.Sprintf("binary-%d", index)}, State: ComponentMissing}
			}
		},
		"too many receipts": func(spec *InstallationObservationSpec) {
			spec.Package.ExpectedReceipts = make([]string, 257)
			spec.Package.MissingReceipts = make([]string, 257)
			for index := range spec.Package.ExpectedReceipts {
				value := fmt.Sprintf("receipt-%03d", index)
				spec.Package.ExpectedReceipts[index], spec.Package.MissingReceipts[index] = value, value
			}
		},
		"invalid tool id":  func(spec *InstallationObservationSpec) { spec.ToolID = "bad\u202eid" },
		"oversize tool id": func(spec *InstallationObservationSpec) { spec.ToolID = strings.Repeat("x", 257) },
		"invalid provider": func(spec *InstallationObservationSpec) { spec.Package.Provider = "brew\nTOKEN=x" },
	} {
		t.Run(name, func(t *testing.T) {
			spec := base()
			mutate(&spec)
			if _, err := NewInstallationObservation(spec); err == nil {
				t.Fatal("invalid evidence was accepted")
			}
		})
	}
}

func TestInstallationObservationSortsEvidenceWithoutDeduplicatingInvalidInput(t *testing.T) {
	observation, err := NewInstallationObservation(InstallationObservationSpec{
		ToolID: "sorted", Installability: InstallabilitySupported,
		Package: PackageFacet{State: PackageMissing, Provider: "brew", ExpectedReceipts: []string{"z", "a"}, MissingReceipts: []string{"z", "a"}, Complete: true},
		Direct: DirectFacet{State: ComponentMissing, Alternatives: []DirectAlternative{
			{Kind: DirectSourceFlatpak, Identifiers: []string{"z", "a"}, State: ComponentMissing},
			{Kind: DirectSourceBinary, Identifiers: []string{"z", "a"}, State: ComponentMissing},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(observation.Package().ExpectedReceipts, []string{"a", "z"}) || observation.Direct().Alternatives[0].Kind != DirectSourceBinary || !reflect.DeepEqual(observation.Direct().Alternatives[0].Identifiers, []string{"a", "z"}) {
		t.Fatalf("evidence was not canonicalized: package=%+v direct=%+v", observation.Package(), observation.Direct())
	}
	withNamespaces, err := NewInstallationObservation(InstallationObservationSpec{
		ToolID: "namespaces", Installability: InstallabilitySupported,
		Package: PackageFacet{State: PackageUnknown, ExpectedReceipts: []string{"one"}, UnresolvedReceipts: []string{"one"}, Namespaces: []PackageNamespaceFacet{
			{Namespace: PackageNamespaceFormula, State: PackageUnknown, ExpectedReceipts: []string{"one"}, UnresolvedReceipts: []string{"one"}},
			{Namespace: PackageNamespaceCask, State: PackageUnknown, ExpectedReceipts: []string{"one"}, UnresolvedReceipts: []string{"one"}},
		}},
		Direct: DirectFacet{State: ComponentNotApplicable},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := withNamespaces.Package().Namespaces; len(got) != 2 || got[0].Namespace != PackageNamespaceCask || got[1].Namespace != PackageNamespaceFormula {
		t.Fatalf("namespaces were not canonicalized: %+v", got)
	}
}

func TestPackageNamespacesReconcileWithAggregateEvidence(t *testing.T) {
	for _, facet := range []PackageFacet{
		{
			State: PackagePartial, ExpectedReceipts: []string{"formula", "cask"}, ObservedReceipts: []string{"formula"}, MissingReceipts: []string{"cask"}, Complete: true,
			Namespaces: []PackageNamespaceFacet{
				{Namespace: PackageNamespaceFormula, State: PackagePresent, ExpectedReceipts: []string{"formula"}, ObservedReceipts: []string{"formula"}, Complete: true},
				{Namespace: PackageNamespaceCask, State: PackageMissing, ExpectedReceipts: []string{"cask"}, MissingReceipts: []string{"cask"}, Complete: true},
			},
		},
		{
			State: PackagePresent, ExpectedReceipts: []string{"shared"}, ObservedReceipts: []string{"shared"}, Complete: true,
			Namespaces: []PackageNamespaceFacet{
				{Namespace: PackageNamespaceFormula, State: PackagePresent, ExpectedReceipts: []string{"shared"}, ObservedReceipts: []string{"shared"}, Complete: true},
				{Namespace: PackageNamespaceCask, State: PackageMissing, ExpectedReceipts: []string{"shared"}, MissingReceipts: []string{"shared"}, Complete: true},
			},
		},
	} {
		if _, err := NewInstallationObservation(InstallationObservationSpec{ToolID: "valid", Installability: InstallabilitySupported, Package: facet, Direct: DirectFacet{State: ComponentNotApplicable}}); err != nil {
			t.Fatalf("valid namespace aggregate rejected: %v", err)
		}
	}

	base := PackageFacet{
		State: PackagePresent, ExpectedReceipts: []string{"one"}, ObservedReceipts: []string{"one"}, Complete: true,
		Namespaces: []PackageNamespaceFacet{{Namespace: PackageNamespaceFormula, State: PackagePresent, ExpectedReceipts: []string{"one"}, ObservedReceipts: []string{"one"}, Complete: true}},
	}
	for name, mutate := range map[string]func(*PackageFacet){
		"namespace outside aggregate": func(facet *PackageFacet) {
			facet.Namespaces[0] = PackageNamespaceFacet{Namespace: PackageNamespaceFormula, State: PackagePresent, ExpectedReceipts: []string{"two"}, ObservedReceipts: []string{"two"}, Complete: true}
		},
		"observed contradicted by aggregate missing": func(facet *PackageFacet) {
			facet.State, facet.ObservedReceipts, facet.MissingReceipts = PackageMissing, nil, []string{"one"}
		},
		"namespace evidence contradicts aggregate state": func(facet *PackageFacet) {
			facet.State, facet.ObservedReceipts, facet.UnresolvedReceipts, facet.Complete = PackageUnknown, nil, []string{"one"}, false
		},
	} {
		t.Run(name, func(t *testing.T) {
			facet := clonePackageFacet(base)
			mutate(&facet)
			if _, err := NewInstallationObservation(InstallationObservationSpec{ToolID: "invalid", Installability: InstallabilitySupported, Package: facet, Direct: DirectFacet{State: ComponentNotApplicable}}); err == nil {
				t.Fatal("contradictory namespace aggregate was accepted")
			}
		})
	}
}

func TestDirectDiagnosticBelongsToItsAlternative(t *testing.T) {
	observation, err := NewInstallationObservation(InstallationObservationSpec{
		ToolID: "direct", Installability: InstallabilityUnknown,
		Package: PackageFacet{State: PackageNotApplicable},
		Direct: DirectFacet{State: ComponentUnknown, DiagnosticCode: DiagnosticProbeTimeout, DiagnosticSummary: "probe timed out", Alternatives: []DirectAlternative{{
			Kind: DirectSourceBinary, Identifiers: []string{"direct"}, State: ComponentUnknown,
			DiagnosticCode: DiagnosticProbeTimeout, DiagnosticSummary: "probe timed out",
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	alternative := observation.Direct().Alternatives[0]
	direct := observation.Direct()
	if direct.DiagnosticCode != DiagnosticProbeTimeout || direct.DiagnosticSummary != "probe timed out" || alternative.DiagnosticCode != DiagnosticProbeTimeout || alternative.DiagnosticSummary != "probe timed out" {
		t.Fatalf("direct diagnostic lost association: %+v", alternative)
	}
}

func TestInstallationDiagnosticCodesAreStable(t *testing.T) {
	want := map[DiagnosticCode]string{
		DiagnosticPackageBatchFailed: "package_batch_failed",
		DiagnosticFallbackTimeout:    "fallback_timeout",
		DiagnosticCancelled:          "cancelled",
		DiagnosticProbeFailed:        "probe_failed",
		DiagnosticProbeTimeout:       "probe_timeout",
	}
	for code, value := range want {
		if string(code) != value {
			t.Errorf("diagnostic code %q changed, want %q", code, value)
		}
	}
}

func TestInstallationObservationIsImmutable(t *testing.T) {
	expectedAll := []string{"one", "two", "three"}
	observed := []string{"one"}
	missing := []string{"two"}
	unresolved := []string{"three"}
	identifiers := []string{"com.example.app"}
	formulaExpected, formulaObserved := []string{"one"}, []string{"one"}
	caskExpected, caskMissing, caskUnresolved := []string{"two", "three"}, []string{"two"}, []string{"three"}
	namespaces := []PackageNamespaceFacet{
		{Namespace: PackageNamespaceFormula, State: PackagePresent, ExpectedReceipts: formulaExpected, ObservedReceipts: formulaObserved, Complete: true},
		{Namespace: PackageNamespaceCask, State: PackageUnknown, ExpectedReceipts: caskExpected, MissingReceipts: caskMissing, UnresolvedReceipts: caskUnresolved, Complete: false},
	}
	alternatives := []DirectAlternative{{Kind: DirectSourceAppBundle, Identifiers: identifiers, State: ComponentPresent}}
	observation, err := NewInstallationObservation(InstallationObservationSpec{
		ToolID:         "example",
		Installability: InstallabilitySupported,
		Package: PackageFacet{
			State: PackagePartial, Provider: "brew", ExpectedReceipts: expectedAll, ObservedReceipts: observed, MissingReceipts: missing, UnresolvedReceipts: unresolved, Authoritative: true, Complete: false,
			Namespaces: namespaces,
		},
		Direct: DirectFacet{State: ComponentPresent, Alternatives: alternatives},
	})
	if err != nil {
		t.Fatal(err)
	}

	expectedAll[0], observed[0], missing[0], unresolved[0], identifiers[0] = "mutated", "mutated", "mutated", "mutated", "mutated"
	formulaExpected[0], formulaObserved[0], caskExpected[0], caskMissing[0], caskUnresolved[0] = "mutated", "mutated", "mutated", "mutated", "mutated"
	namespaces[0] = PackageNamespaceFacet{Namespace: PackageNamespaceCask}
	alternatives[0].Identifiers[0] = "mutated-again"
	alternatives[0] = DirectAlternative{Kind: DirectSourceBinary}
	packageFacet := observation.Package()
	directFacet := observation.Direct()
	formulaFacet := packageNamespaceForTest(t, packageFacet, PackageNamespaceFormula)
	caskFacet := packageNamespaceForTest(t, packageFacet, PackageNamespaceCask)
	if !reflect.DeepEqual(packageFacet.ExpectedReceipts, []string{"one", "three", "two"}) ||
		!reflect.DeepEqual(packageFacet.ObservedReceipts, []string{"one"}) ||
		!reflect.DeepEqual(packageFacet.MissingReceipts, []string{"two"}) ||
		!reflect.DeepEqual(packageFacet.UnresolvedReceipts, []string{"three"}) ||
		!reflect.DeepEqual(formulaFacet.ExpectedReceipts, []string{"one"}) || !reflect.DeepEqual(formulaFacet.ObservedReceipts, []string{"one"}) ||
		!reflect.DeepEqual(caskFacet.ExpectedReceipts, []string{"three", "two"}) || !reflect.DeepEqual(caskFacet.MissingReceipts, []string{"two"}) || !reflect.DeepEqual(caskFacet.UnresolvedReceipts, []string{"three"}) ||
		!reflect.DeepEqual(directFacet.Alternatives[0].Identifiers, []string{"com.example.app"}) {
		t.Fatalf("constructor retained caller slices: package=%+v direct=%+v", packageFacet, directFacet)
	}

	packageFacet.ExpectedReceipts[0] = "changed"
	packageFacet.ObservedReceipts[0] = "changed"
	packageFacet.MissingReceipts[0] = "changed"
	packageFacet.UnresolvedReceipts[0] = "changed"
	for index := range packageFacet.Namespaces {
		if len(packageFacet.Namespaces[index].ExpectedReceipts) > 0 {
			packageFacet.Namespaces[index].ExpectedReceipts[0] = "changed"
		}
		if len(packageFacet.Namespaces[index].ObservedReceipts) > 0 {
			packageFacet.Namespaces[index].ObservedReceipts[0] = "changed"
		}
		if len(packageFacet.Namespaces[index].MissingReceipts) > 0 {
			packageFacet.Namespaces[index].MissingReceipts[0] = "changed"
		}
		if len(packageFacet.Namespaces[index].UnresolvedReceipts) > 0 {
			packageFacet.Namespaces[index].UnresolvedReceipts[0] = "changed"
		}
	}
	packageFacet.Namespaces[0] = PackageNamespaceFacet{Namespace: PackageNamespaceFormula}
	directFacet.Alternatives[0].Identifiers[0] = "changed"
	directFacet.Alternatives[0] = DirectAlternative{Kind: DirectSourceBinary}
	packageAgain := observation.Package()
	formulaAgain := packageNamespaceForTest(t, packageAgain, PackageNamespaceFormula)
	caskAgain := packageNamespaceForTest(t, packageAgain, PackageNamespaceCask)
	directAgain := observation.Direct()
	if !reflect.DeepEqual(packageAgain.ExpectedReceipts, []string{"one", "three", "two"}) ||
		!reflect.DeepEqual(packageAgain.ObservedReceipts, []string{"one"}) ||
		!reflect.DeepEqual(packageAgain.MissingReceipts, []string{"two"}) ||
		!reflect.DeepEqual(packageAgain.UnresolvedReceipts, []string{"three"}) ||
		!reflect.DeepEqual(formulaAgain.ExpectedReceipts, []string{"one"}) || !reflect.DeepEqual(formulaAgain.ObservedReceipts, []string{"one"}) ||
		!reflect.DeepEqual(caskAgain.ExpectedReceipts, []string{"three", "two"}) || !reflect.DeepEqual(caskAgain.MissingReceipts, []string{"two"}) || !reflect.DeepEqual(caskAgain.UnresolvedReceipts, []string{"three"}) ||
		len(directAgain.Alternatives) != 1 || directAgain.Alternatives[0].Kind != DirectSourceAppBundle || !reflect.DeepEqual(directAgain.Alternatives[0].Identifiers, []string{"com.example.app"}) {
		t.Fatal("accessor exposed mutable observation storage")
	}
}

func TestNewInstallationSnapshotSortsAndClonesObservations(t *testing.T) {
	makeObservation := func(id string) InstallationObservation {
		observation, err := NewInstallationObservation(InstallationObservationSpec{
			ToolID: id, Installability: InstallabilitySupported,
			Package: PackageFacet{State: PackageMissing, Provider: "brew", ExpectedReceipts: []string{"receipt"}, MissingReceipts: []string{"receipt"}, Authoritative: true, Complete: true},
			Direct:  DirectFacet{State: ComponentNotApplicable},
		})
		if err != nil {
			t.Fatal(err)
		}
		return observation
	}
	input := []InstallationObservation{makeObservation("zeta"), makeObservation("alpha")}
	snapshot, err := NewInstallationSnapshot(InstallationSnapshotSpec{
		Generation: 7,
		Platform:   "macos",
		Manager:    "brew",
		Tools:      input,
	})
	if err != nil {
		t.Fatal(err)
	}
	input[0] = makeObservation("mutated")
	got := snapshot.Tools()
	if CurrentInstallationSchemaVersion != 1 || snapshot.SchemaVersion() != CurrentInstallationSchemaVersion || snapshot.Generation() != 7 || snapshot.Platform() != "macos" || snapshot.Manager() != "brew" {
		t.Fatalf("snapshot metadata changed: schema=%d generation=%d platform=%q manager=%q", snapshot.SchemaVersion(), snapshot.Generation(), snapshot.Platform(), snapshot.Manager())
	}
	if len(got) != 2 || got[0].ToolID() != "alpha" || got[1].ToolID() != "zeta" {
		t.Fatalf("tools are not deterministic: %#v", got)
	}
	got[0] = makeObservation("changed")
	if snapshot.Tools()[0].ToolID() != "alpha" {
		t.Fatal("Tools exposed mutable snapshot storage")
	}
	alpha, ok := snapshot.Tool("alpha")
	if !ok || alpha.ToolID() != "alpha" {
		t.Fatalf("Tool lookup=(%q,%v)", alpha.ToolID(), ok)
	}
	returned := alpha.Package()
	returned.ExpectedReceipts[0] = "changed"
	alphaAgain, _ := snapshot.Tool("alpha")
	if alphaAgain.Package().ExpectedReceipts[0] != "receipt" {
		t.Fatal("Tool lookup exposed mutable snapshot storage")
	}
}

func TestNewInstallationSnapshotRejectsDuplicateIDsAndInvalidMetadata(t *testing.T) {
	observation, err := NewInstallationObservation(InstallationObservationSpec{
		ToolID: "same", Installability: InstallabilityUnknown,
		Package: PackageFacet{State: PackageUnknown, ExpectedReceipts: []string{"receipt"}, Complete: false},
		Direct:  DirectFacet{State: ComponentUnknown},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, spec := range map[string]InstallationSnapshotSpec{
		"zero generation":  {Platform: "macos", Manager: "brew", Tools: []InstallationObservation{observation}},
		"empty platform":   {Generation: 1, Manager: "brew", Tools: []InstallationObservation{observation}},
		"invalid platform": {Generation: 1, Platform: "macos\nTOKEN=x", Manager: "brew", Tools: []InstallationObservation{observation}},
		"invalid manager":  {Generation: 1, Platform: "macos", Manager: "brew\u202E", Tools: []InstallationObservation{observation}},
		"oversize manager": {Generation: 1, Platform: "macos", Manager: strings.Repeat("m", 257), Tools: []InstallationObservation{observation}},
		"duplicate ID":     {Generation: 1, Platform: "macos", Manager: "brew", Tools: []InstallationObservation{observation, observation}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewInstallationSnapshot(spec); err == nil {
				t.Fatal("invalid snapshot was accepted")
			}
		})
	}
	tooMany := make([]InstallationObservation, 0, MaxInstallationTools+1)
	for index := 0; index <= MaxInstallationTools; index++ {
		candidate, candidateErr := NewInstallationObservation(InstallationObservationSpec{
			ToolID: fmt.Sprintf("tool-%04d", index), Installability: InstallabilityUnknown,
			Package: PackageFacet{State: PackageNotApplicable}, Direct: DirectFacet{State: ComponentNotApplicable},
		})
		if candidateErr != nil {
			t.Fatal(candidateErr)
		}
		tooMany = append(tooMany, candidate)
	}
	if _, err := NewInstallationSnapshot(InstallationSnapshotSpec{Generation: 1, Platform: "macos", Manager: "brew", Tools: tooMany[:MaxInstallationTools]}); err != nil {
		t.Fatalf("snapshot rejected exact MaxInstallationTools boundary: %v", err)
	}
	if _, err := NewInstallationSnapshot(InstallationSnapshotSpec{Generation: 1, Platform: "macos", Manager: "brew", Tools: tooMany}); err == nil {
		t.Fatalf("snapshot accepted %d tools (maximum %d)", len(tooMany), MaxInstallationTools)
	}
}

func TestValidateInstallationToolIDsMatchesSnapshotPreflightContract(t *testing.T) {
	if err := ValidateInstallationToolIDs([]string{"zeta", "alpha"}); err != nil {
		t.Fatalf("valid IDs rejected: %v", err)
	}
	for name, ids := range map[string][]string{
		"empty": {""}, "control": {"bad\nID"}, "duplicate": {"same", "same"},
		"too many": make([]string, MaxInstallationTools+1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateInstallationToolIDs(ids); err == nil {
				t.Fatal("invalid IDs accepted")
			}
		})
	}
}
