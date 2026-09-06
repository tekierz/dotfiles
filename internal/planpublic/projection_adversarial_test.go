package planpublic

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestPublicPlanRejectsNonCanonicalDigestsAndSnapshotSchema(t *testing.T) {
	valid := strings.Repeat("a", 64)
	for _, test := range []struct {
		name   string
		mutate func(*DocumentSpec)
	}{
		{name: "snapshot schema zero", mutate: func(spec *DocumentSpec) { spec.Snapshot.SchemaVersion = 0 }},
		{name: "snapshot schema future", mutate: func(spec *DocumentSpec) { spec.Snapshot.SchemaVersion = 2 }},
		{name: "snapshot short digest", mutate: func(spec *DocumentSpec) { spec.Snapshot.PublicDigest = "abc" }},
		{name: "snapshot uppercase digest", mutate: func(spec *DocumentSpec) { spec.Snapshot.PublicDigest = strings.Repeat("A", 64) }},
		{name: "snapshot nonhex digest", mutate: func(spec *DocumentSpec) { spec.Snapshot.PublicDigest = strings.Repeat("z", 64) }},
		{name: "recipe short digest", mutate: func(spec *DocumentSpec) { spec.Actions[0].Install = validInstallSpec("abc") }},
		{name: "recipe uppercase digest", mutate: func(spec *DocumentSpec) { spec.Actions[0].Install = validInstallSpec(strings.Repeat("A", 64)) }},
		{name: "recipe nonhex digest", mutate: func(spec *DocumentSpec) { spec.Actions[0].Install = validInstallSpec(strings.Repeat("z", 64)) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec := readyProjectionSpec()
			spec.Actions[0].Install = validInstallSpec(valid)
			test.mutate(&spec)
			if document, err := NewDocument(spec); !errors.Is(err, ErrInvalidDocument) || !reflect.ValueOf(document).IsZero() {
				t.Fatalf("NewDocument accepted invalid digest/schema: document=%+v err=%v", document, err)
			}
		})
	}
}

func TestPublicPlanRejectsFreeTextButPackageTokensRemainExact(t *testing.T) {
	canaries := []string{
		"/etc/private", "/var/db/foo", "secrets/file", "foo/bar", `C:\Windows\Temp`,
		"./secret", "~/secret", "file:///tmp/private", "token = value", "api_key=value",
		"client-secret: value", "Authorization: Basic abc", "raw dial error 10.0.0.1",
		"line\nfeed", "bidi\u202esecret",
	}
	for _, canary := range canaries {
		t.Run(canary, func(t *testing.T) {
			spec := readyProjectionSpec()
			spec.Actions[0].Description = canary
			if _, err := NewDocument(spec); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("free text %q crossed projection: %v", canary, err)
			}
		})
	}

	spec := readyProjectionSpec()
	install := validInstallSpec(strings.Repeat("a", 64))
	install.Steps[0].Packages = []string{"@scope/package", "ordinary-package"}
	spec.Actions[0].Install = install
	document, err := NewDocument(spec)
	if err != nil {
		t.Fatal(err)
	}
	packages := document.Actions()[0].Install.Steps[0].Packages
	if !reflect.DeepEqual(packages, []string{"@scope/package", "ordinary-package"}) {
		t.Fatalf("validated package tokens were over-redacted: %v", packages)
	}
}

func TestIntentRequiredRejectsAnyCollectedOrPlannedState(t *testing.T) {
	base := DocumentSpec{Status: StatusIntentRequired, Intent: Intent{Source: "explicit_tools", Tools: []string{}}}
	for _, mutate := range []func(*DocumentSpec){
		func(spec *DocumentSpec) { spec.Platform = "macos" },
		func(spec *DocumentSpec) { spec.Manager = "brew" },
		func(spec *DocumentSpec) {
			spec.Snapshot = &Snapshot{SchemaVersion: 1, Generation: 1, PublicDigest: strings.Repeat("a", 64)}
		},
		func(spec *DocumentSpec) {
			spec.Actions = []ActionSpec{{ActionID: "install:codex", Kind: "install_tool", ToolID: "codex", Description: "install codex", Disposition: "apply", Ownership: "package_manager", Reversibility: "external"}}
		},
	} {
		spec := base
		mutate(&spec)
		if _, err := NewDocument(spec); !errors.Is(err, ErrInvalidDocument) {
			t.Fatalf("intent_required accepted collected/planned state: %+v err=%v", spec, err)
		}
	}
}

func TestBlockedStatusAndSummaryMustMatchProjectedActions(t *testing.T) {
	blocked := readyProjectionSpec()
	blocked.Status = StatusBlocked
	blocked.PlanHash = ""
	blocked.Capabilities.Apply = "not_available"
	blocked.Actions = []ActionSpec{{ActionID: "install:codex", Kind: "install_tool", ToolID: "codex", Description: "install codex", Disposition: "blocked", ReasonCode: "unsupported", Reason: "installation unsupported", Ownership: "package_manager", Reversibility: "external"}}
	blocked.Summary = Summary{Blocked: 1}
	if _, err := NewDocument(blocked); err != nil {
		t.Fatalf("valid blocked document rejected: %v", err)
	}

	for _, mutate := range []func(*DocumentSpec){
		func(spec *DocumentSpec) { spec.Actions[0].Disposition = "apply"; spec.Summary = Summary{Apply: 1} },
		func(spec *DocumentSpec) { spec.Actions = []ActionSpec{}; spec.Summary = Summary{} },
		func(spec *DocumentSpec) { spec.Summary = Summary{Blocked: 2} },
		func(spec *DocumentSpec) { spec.Summary = Summary{Apply: -1, Blocked: 1} },
		func(spec *DocumentSpec) { spec.Summary = Summary{Blocked: 1, BackupTargets: -1} },
	} {
		spec := blocked
		spec.Actions = append([]ActionSpec(nil), blocked.Actions...)
		mutate(&spec)
		if _, err := NewDocument(spec); !errors.Is(err, ErrInvalidDocument) {
			t.Fatalf("blocked/summary invariant accepted: %+v err=%v", spec, err)
		}
	}

	ready := readyProjectionSpec()
	ready.Summary.Apply = 1
	if _, err := NewDocument(ready); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("inconsistent ready summary accepted: %v", err)
	}
	ready = readyProjectionSpec()
	ready.Actions = nil
	ready.Summary = Summary{}
	if _, err := NewDocument(ready); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("ready status without an applicable action accepted: %v", err)
	}
	ready = readyProjectionSpec()
	ready.Actions[0].Disposition = "blocked"
	ready.Summary = Summary{Apply: 1, Blocked: 1}
	if _, err := NewDocument(ready); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("ready status with a blocked action accepted: %v", err)
	}
}

func TestPublicPlanRejectsInvalidStructuralActionAndInstallShapes(t *testing.T) {
	validDigest := strings.Repeat("a", 64)
	for _, mutate := range []func(*DocumentSpec){
		func(spec *DocumentSpec) { spec.Actions[0].ActionID = "bad id" },
		func(spec *DocumentSpec) { spec.Actions[0].ToolID = "Codex" },
		func(spec *DocumentSpec) { spec.Actions[0].Kind = "shell" },
		func(spec *DocumentSpec) { spec.Actions[0].Disposition = "maybe" },
		func(spec *DocumentSpec) { spec.Actions[0].Ownership = "root" },
		func(spec *DocumentSpec) { spec.Actions[0].Reversibility = "irreversible" },
		func(spec *DocumentSpec) {
			spec.Actions[0].Install = validInstallSpec(validDigest)
			spec.Actions[0].Install.SchemaVersion = 0
		},
		func(spec *DocumentSpec) {
			spec.Actions[0].Install = validInstallSpec(validDigest)
			spec.Actions[0].Install.Steps = nil
		},
		func(spec *DocumentSpec) {
			spec.Actions[0].Install = validInstallSpec(validDigest)
			spec.Actions[0].Install.Steps[0].Kind = "shell"
		},
		func(spec *DocumentSpec) {
			spec.Actions[0].Install = validInstallSpec(validDigest)
			spec.Actions[0].Install.Detector.Kind = "command"
		},
		func(spec *DocumentSpec) {
			spec.Actions[0].Install = validInstallSpec(validDigest)
			spec.Actions[0].Install.Detector.Values = nil
		},
	} {
		spec := readyProjectionSpec()
		mutate(&spec)
		if _, err := NewDocument(spec); !errors.Is(err, ErrInvalidDocument) {
			t.Fatalf("invalid structural shape accepted: %+v err=%v", spec.Actions[0], err)
		}
	}
}

func TestPublicDigestPreservesFullUint64GenerationIdentity(t *testing.T) {
	first := readyProjectionSpec()
	first.Snapshot.Generation = 1 << 53
	second := readyProjectionSpec()
	second.Snapshot.Generation = 1<<53 + 1
	firstDocument, err := NewDocument(first)
	if err != nil {
		t.Fatal(err)
	}
	secondDocument, err := NewDocument(second)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := MarshalDocument(firstDocument)
	secondJSON, _ := MarshalDocument(secondDocument)
	firstDigest := publicDigestFromJSON(t, firstJSON)
	secondDigest := publicDigestFromJSON(t, secondJSON)
	if firstDigest == secondDigest {
		t.Fatalf("distinct uint64 generations collapsed to digest %q", firstDigest)
	}
	if firstDigest != independentlyRecomputePublicDigest(t, firstJSON) || secondDigest != independentlyRecomputePublicDigest(t, secondJSON) {
		t.Fatal("published digest does not match UseNumber canonical recomputation")
	}
}

func validInstallSpec(digest string) *InstallSpec {
	return &InstallSpec{
		SchemaVersion: 1, Platform: "macos", Manager: "brew",
		Steps:          []InstallStepSpec{{Kind: "package_manager", Provider: "brew", Packages: []string{"codex"}, Casks: []string{}, Arguments: []string{}}},
		Detector:       DetectorSpec{Kind: "package_receipt", Values: []string{"codex"}},
		Authentication: "interactive_provider_login", Risk: "package_manager_install", RecipeDigest: digest,
	}
}

func publicDigestFromJSON(t *testing.T, encoded []byte) string {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		t.Fatal(err)
	}
	authority := document["authority"].(map[string]any)
	digest := authority["public_digest"].(string)
	return digest
}

func independentlyRecomputePublicDigest(t *testing.T, encoded []byte) string {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		t.Fatal(err)
	}
	authority := document["authority"].(map[string]any)
	delete(authority, "public_digest")
	canonical, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
