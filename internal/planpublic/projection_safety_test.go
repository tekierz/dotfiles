package planpublic

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func safetyProjectionSpec() DocumentSpec {
	spec := readyProjectionSpec()
	spec.Actions = []ActionSpec{
		{
			ActionID: "install:pi", Kind: "install_tool", ToolID: "pi",
			Description: "install pi", Disposition: "apply",
			Ownership: "package_manager", Reversibility: "external",
			Observation: &ObservationSpec{Exists: false, Managed: false},
			Install: &InstallSpec{
				SchemaVersion: 1, Platform: "macos", Manager: "brew",
				Steps: []InstallStepSpec{
					{Kind: "package_manager", Provider: "brew", Packages: []string{"node"}, Casks: []string{}, Arguments: []string{}},
					{Kind: "npm_global", Provider: "npm", Packages: []string{}, Casks: []string{}, Arguments: []string{"install", "-g", "package@1.0.0"}},
				},
				Detector:       DetectorSpec{Kind: "binary", Values: []string{"pi"}},
				Authentication: "interactive_provider_login", Risk: "npm_lifecycle_code", RecipeDigest: strings.Repeat("3", 64),
			},
		},
		{ActionID: "install:codex", Kind: "install_tool", ToolID: "codex", Description: "install codex", Disposition: "skip", ReasonCode: "present", Reason: "already present", Ownership: "package_manager", Reversibility: "external"},
	}
	spec.Summary = Summary{Apply: 1, Skip: 1}
	return spec
}

func TestPublicProjectionPreservesSafeShapeOrderAndNonNullArrays(t *testing.T) {
	document, err := NewDocument(safetyProjectionSpec())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &root); err != nil {
		t.Fatal(err)
	}
	assertRawKeys(t, root["intent"], []string{"source", "tools", "digest"})
	assertRawKeys(t, root["snapshot"], []string{"schema_version", "generation", "public_digest"})
	assertRawKeys(t, root["authority"], []string{"plan_hash", "public_digest"})
	assertRawKeys(t, root["capabilities"], []string{"installation", "config", "service", "auth", "apply"})
	assertRawKeys(t, root["summary"], []string{"apply", "skip", "blocked", "backup_targets"})
	var actions []map[string]json.RawMessage
	if err := json.Unmarshal(root["actions"], &actions); err != nil || len(actions) != 2 {
		t.Fatalf("actions len=%d err=%v", len(actions), err)
	}
	var rawActions []json.RawMessage
	if err := json.Unmarshal(root["actions"], &rawActions); err != nil {
		t.Fatal(err)
	}
	for index, wantID := range []string{"install:pi", "install:codex"} {
		var ordinal int
		var actionID string
		_ = json.Unmarshal(actions[index]["ordinal"], &ordinal)
		_ = json.Unmarshal(actions[index]["action_id"], &actionID)
		if ordinal != index || actionID != wantID {
			t.Fatalf("actions[%d]=ordinal:%d id:%q", index, ordinal, actionID)
		}
	}
	assertRawKeys(t, rawActions[0], []string{"ordinal", "action_id", "kind", "tool_id", "description", "disposition", "reason_code", "reason", "ownership", "reversibility", "observation", "install"})
	assertRawKeys(t, rawActions[1], []string{"ordinal", "action_id", "kind", "tool_id", "description", "disposition", "reason_code", "reason", "ownership", "reversibility"})
	assertRawKeys(t, actions[0]["observation"], []string{"exists", "managed"})
	var install map[string]json.RawMessage
	if err := json.Unmarshal(actions[0]["install"], &install); err != nil {
		t.Fatal(err)
	}
	assertRawKeys(t, actions[0]["install"], []string{"schema_version", "platform", "manager", "steps", "detector", "authentication", "risk", "recipe_digest"})
	var steps []map[string]json.RawMessage
	var rawSteps []json.RawMessage
	if err := json.Unmarshal(install["steps"], &steps); err != nil || len(steps) != 2 {
		t.Fatalf("install steps len=%d err=%v", len(steps), err)
	}
	if err := json.Unmarshal(install["steps"], &rawSteps); err != nil {
		t.Fatal(err)
	}
	for index, want := range []struct{ kind, provider string }{{"package_manager", "brew"}, {"npm_global", "npm"}} {
		assertRawKeys(t, rawSteps[index], []string{"kind", "provider", "packages", "casks", "arguments"})
		var kind, provider string
		_ = json.Unmarshal(steps[index]["kind"], &kind)
		_ = json.Unmarshal(steps[index]["provider"], &provider)
		if kind != want.kind || provider != want.provider {
			t.Fatalf("steps[%d]=%q/%q, want %q/%q", index, kind, provider, want.kind, want.provider)
		}
		for _, key := range []string{"packages", "casks", "arguments"} {
			if bytes.Equal(steps[index][key], []byte("null")) || len(steps[index][key]) == 0 {
				t.Fatalf("steps[%d].%s must be a non-null array: %s", index, key, steps[index][key])
			}
		}
	}
	var detector map[string]json.RawMessage
	_ = json.Unmarshal(install["detector"], &detector)
	assertRawKeys(t, install["detector"], []string{"kind", "values"})
	if bytes.Equal(detector["values"], []byte("null")) {
		t.Fatal("detector.values must be non-null")
	}
	for _, forbidden := range []string{"path", "observation_source", "target_source", "backup_source", "backup_path", "backup_target", "config_content", "private_digest", "snapshot_digest", "desired_digest", "content_digest", "parent_chain"} {
		if bytes.Contains(encoded, []byte(`"`+forbidden+`"`)) {
			t.Fatalf("public JSON exposed forbidden key %q: %s", forbidden, encoded)
		}
	}
	if !bytes.Contains(encoded, []byte("install pi")) {
		t.Fatalf("safe description was not preserved: %s", encoded)
	}
	if bytes.Contains(root["actions"], []byte("install:codex")) && !bytes.Contains(root["actions"], []byte("install:pi")) {
		t.Fatal("action identity/order collapsed")
	}
}

func TestPublicProjectionRejectsUntypedAuthenticationRiskAndCapabilities(t *testing.T) {
	for _, mutate := range []func(*DocumentSpec){
		func(spec *DocumentSpec) { spec.Actions[0].Install.Authentication = "provider sign-in for alice" },
		func(spec *DocumentSpec) { spec.Actions[0].Install.Risk = "connect: connection refused 10.0.0.1:443" },
		func(spec *DocumentSpec) { spec.Capabilities.Apply = "eventually maybe" },
		func(spec *DocumentSpec) { spec.Capabilities.Installation = strings.Repeat("x", 512) },
	} {
		spec := safetyProjectionSpec()
		mutate(&spec)
		if _, err := NewDocument(spec); !errors.Is(err, ErrInvalidDocument) {
			t.Fatalf("untyped public value accepted: %+v err=%v", spec, err)
		}
	}
}

func TestPublicProjectionDigestChangesForPublicChangeAndDocumentIsDefensive(t *testing.T) {
	spec := safetyProjectionSpec()
	document, err := NewDocument(spec)
	if err != nil {
		t.Fatal(err)
	}
	baseline, _ := MarshalDocument(document)
	spec.Intent.Tools[0] = "mutated"
	spec.Actions[0].Description = "mutated"
	spec.Actions[0].Install.Steps[1].Arguments[0] = "mutated"
	spec.Snapshot.Generation = 99
	actions := document.Actions()
	actions[0].Description = "mutated again"
	actions[0].Install.Steps[1].Arguments[0] = "mutated again"
	after, _ := MarshalDocument(document)
	if !bytes.Equal(baseline, after) {
		t.Fatal("caller/accessor mutation changed immutable public document")
	}
	changed := safetyProjectionSpec()
	changed.Snapshot.Generation++
	if bytes.Equal(baseline, mustMarshalPublic(t, changed)) {
		t.Fatal("legitimate public change did not alter document/digest")
	}
}

func TestPublicProjectionRejectsInvalidStatusAndIntentRequiredHasExactShape(t *testing.T) {
	if _, err := NewDocument(DocumentSpec{Status: Status("future")}); err == nil {
		t.Fatal("unknown public status was accepted")
	}
	document, err := NewDocument(DocumentSpec{Status: StatusIntentRequired, Intent: Intent{Source: "explicit_tools", Tools: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := MarshalDocument(document)
	if got := orderedJSONKeys(t, encoded); !reflect.DeepEqual(got, []string{"schema_version", "kind", "status", "intent", "authority", "capabilities", "summary", "actions"}) {
		t.Fatalf("intent_required keys=%v", got)
	}
}

func TestPublicProjectionRejectsMismatchedOrMalformedReadyIntentDigest(t *testing.T) {
	for _, digest := range []string{strings.Repeat("f", 64), "not-a-digest", ""} {
		spec := readyProjectionSpec()
		spec.Intent.Digest = digest
		if _, err := NewDocument(spec); err == nil {
			t.Fatalf("ready projection accepted intent digest %q", digest)
		}
	}
}

func mustMarshalPublic(t *testing.T, spec DocumentSpec) []byte {
	t.Helper()
	document, err := NewDocument(spec)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func assertRawKeys(t *testing.T, raw json.RawMessage, want []string) {
	t.Helper()
	if got := orderedJSONKeys(t, raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("keys=%v, want %v; json=%s", got, want, raw)
	}
}
