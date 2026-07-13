package planpublic

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func readyProjectionSpec() DocumentSpec {
	intent := testExplicitIntent([]string{"codex", "pi"})
	return DocumentSpec{
		Status:       StatusReady,
		PlanHash:     strings.Repeat("1", 64),
		Platform:     "macos",
		Manager:      "brew",
		Intent:       intent,
		Snapshot:     &Snapshot{SchemaVersion: 1, Generation: 1, PublicDigest: strings.Repeat("2", 64)},
		Capabilities: Capabilities{Installation: "planned", Config: "not_planned", Service: "not_collected", Auth: "not_collected", Apply: "not_available"},
		Summary:      Summary{Apply: 2},
		Actions: []ActionSpec{
			{ActionID: "install:codex", Kind: "install_tool", ToolID: "codex", Description: "install codex", Disposition: "apply", Ownership: "package_manager", Reversibility: "external", Install: validInstallSpec(strings.Repeat("a", 64))},
			{ActionID: "install:pi", Kind: "install_tool", ToolID: "pi", Description: "install pi", Disposition: "apply", Ownership: "package_manager", Reversibility: "external", Install: validInstallSpec(strings.Repeat("b", 64))},
		},
	}
}

func testExplicitIntent(tools []string) Intent {
	canonical, err := json.Marshal(struct {
		SchemaVersion int      `json:"schema_version"`
		Source        string   `json:"source"`
		Tools         []string `json:"tools"`
	}{SchemaVersion: 1, Source: "explicit_tools", Tools: tools})
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(canonical)
	return Intent{Source: "explicit_tools", Tools: append([]string(nil), tools...), Digest: hex.EncodeToString(digest[:])}
}

func TestPublicPlanProjectionHasExactEnvelopeOrderAndDigest(t *testing.T) {
	document, err := NewDocument(readyProjectionSpec())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) == 0 || encoded[len(encoded)-1] != '\n' || (len(encoded) > 1 && encoded[len(encoded)-2] == '\n') {
		t.Fatalf("document must have exactly one trailing newline: %q", encoded)
	}
	var public map[string]any
	if err := json.Unmarshal(encoded, &public); err != nil {
		t.Fatal(err)
	}
	wantTop := []string{"schema_version", "kind", "status", "platform", "manager", "intent", "snapshot", "authority", "capabilities", "summary", "actions"}
	if got := orderedJSONKeys(t, encoded); !reflect.DeepEqual(got, wantTop) {
		t.Fatalf("top-level key order=%v, want %v", got, wantTop)
	}
	if public["schema_version"] != float64(1) || public["kind"] != "dotfiles.plan" || public["status"] != "ready" {
		t.Fatalf("public identity=%v/%v/%v", public["schema_version"], public["kind"], public["status"])
	}
	actions, ok := public["actions"].([]any)
	if !ok || len(actions) != 2 {
		t.Fatalf("actions=%#v, want non-null length 2", public["actions"])
	}
	for index, raw := range actions {
		action := raw.(map[string]any)
		if action["ordinal"] != float64(index) {
			t.Fatalf("actions[%d].ordinal=%v", index, action["ordinal"])
		}
	}
	authority := public["authority"].(map[string]any)
	digest, _ := authority["public_digest"].(string)
	if len(digest) != 64 {
		t.Fatalf("public digest=%q", digest)
	}
	delete(authority, "public_digest")
	canonical, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	if want := hex.EncodeToString(sum[:]); digest != want {
		t.Fatalf("public digest=%q, independently recomputed=%q", digest, want)
	}
}

func TestPublicPlanProjectionRejectsCallerProvidedDescriptionOrReasonProse(t *testing.T) {
	for _, mutate := range []func(*DocumentSpec){
		func(spec *DocumentSpec) {
			spec.Actions[0].Description = "Authorization: Bearer sk-proj-secret /Users/alice/private"
		},
		func(spec *DocumentSpec) { spec.Actions[0].Description = "install codex safely" },
		func(spec *DocumentSpec) { spec.Actions[0].Reason = "connect: connection refused 10.0.0.1:443" },
	} {
		spec := readyProjectionSpec()
		mutate(&spec)
		if _, err := NewDocument(spec); !errors.Is(err, ErrInvalidDocument) {
			t.Fatalf("caller prose crossed the public projection: %+v err=%v", spec.Actions[0], err)
		}
	}
}

func TestIntentRequiredProjectionOmitsPrivateAuthorityAndUsesEmptyArrays(t *testing.T) {
	document, err := NewDocument(DocumentSpec{Status: StatusIntentRequired, Intent: Intent{Source: "explicit_tools", Tools: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"plan_hash", "snapshot", "private_digest", "desired_digest", "content_digest"} {
		if strings.Contains(text, `"`+forbidden+`"`) {
			t.Fatalf("intent_required leaked %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"status":"intent_required"`) || !strings.Contains(text, `"actions":[]`) || !strings.Contains(text, `"tools":[]`) {
		t.Fatalf("intent_required shape=%s", text)
	}
}

func orderedJSONKeys(t *testing.T, encoded []byte) []string {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		t.Fatalf("decode object start: token=%v err=%v", token, err)
	}
	var keys []string
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, token.(string))
		var discard json.RawMessage
		if err := decoder.Decode(&discard); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}
