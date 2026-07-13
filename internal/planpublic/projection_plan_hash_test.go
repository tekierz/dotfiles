package planpublic

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestReadyDocumentRequiresCanonicalPlanHashAndPublishesItFirst(t *testing.T) {
	for _, invalid := range []string{"", "short", strings.Repeat("A", 64), strings.Repeat("g", 64), strings.Repeat("a", 63), strings.Repeat("a", 65)} {
		spec := readyProjectionSpec()
		spec.PlanHash = invalid
		if document, err := NewDocument(spec); !errors.Is(err, ErrInvalidDocument) || !reflect.ValueOf(document).IsZero() {
			t.Fatalf("ready plan hash %q produced document=%+v err=%v", invalid, document, err)
		}
	}
	spec := readyProjectionSpec()
	document, err := NewDocument(spec)
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
	if got := orderedJSONKeys(t, root["authority"]); !reflect.DeepEqual(got, []string{"plan_hash", "public_digest"}) {
		t.Fatalf("ready authority order=%v", got)
	}
	var authority struct {
		PlanHash string `json:"plan_hash"`
	}
	if err := json.Unmarshal(root["authority"], &authority); err != nil || authority.PlanHash != spec.PlanHash {
		t.Fatalf("ready authority=%s err=%v", root["authority"], err)
	}
}

func TestNonReadyDocumentsRequireEmptyPlanHashAndOmitIt(t *testing.T) {
	noChanges := readyProjectionSpec()
	noChanges.Status = StatusNoChanges
	noChanges.PlanHash = ""
	noChanges.Actions = []ActionSpec{{ActionID: "install:codex", Kind: "install_tool", ToolID: "codex", Description: "install codex", Disposition: "skip", ReasonCode: "present", Reason: "already present", Ownership: "package_manager", Reversibility: "external"}}
	noChanges.Summary = Summary{Skip: 1}
	blocked := readyProjectionSpec()
	blocked.Status = StatusBlocked
	blocked.PlanHash = ""
	blocked.Actions = []ActionSpec{{ActionID: "install:codex", Kind: "install_tool", ToolID: "codex", Description: "install codex", Disposition: "blocked", ReasonCode: "unknown", Reason: "installation status unknown", Ownership: "package_manager", Reversibility: "external"}}
	blocked.Summary = Summary{Blocked: 1}
	intentRequired := DocumentSpec{Status: StatusIntentRequired, Intent: Intent{Source: "explicit_tools", Tools: []string{}}}

	for _, test := range []struct {
		name string
		spec DocumentSpec
	}{
		{name: "no changes", spec: noChanges},
		{name: "blocked", spec: blocked},
		{name: "intent required", spec: intentRequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := NewDocument(test.spec)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := MarshalDocument(document)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, []byte(`"plan_hash"`)) {
				t.Fatalf("non-ready document published plan hash: %s", encoded)
			}
			poisoned := test.spec
			poisoned.PlanHash = strings.Repeat("a", 64)
			if result, err := NewDocument(poisoned); !errors.Is(err, ErrInvalidDocument) || !reflect.ValueOf(result).IsZero() {
				t.Fatalf("non-ready accepted plan hash: result=%+v err=%v", result, err)
			}
		})
	}
}

func TestReadyPlanHashParticipatesInPublicDigest(t *testing.T) {
	firstSpec := readyProjectionSpec()
	secondSpec := readyProjectionSpec()
	secondSpec.PlanHash = strings.Repeat("2", 64)
	first, err := NewDocument(firstSpec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewDocument(secondSpec)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := MarshalDocument(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := MarshalDocument(second)
	if err != nil {
		t.Fatal(err)
	}
	if publicDigestFromJSON(t, firstJSON) == publicDigestFromJSON(t, secondJSON) || independentlyRecomputePublicDigest(t, firstJSON) != publicDigestFromJSON(t, firstJSON) || independentlyRecomputePublicDigest(t, secondJSON) != publicDigestFromJSON(t, secondJSON) {
		t.Fatalf("plan hash did not participate in public digest:\n%s\n%s", firstJSON, secondJSON)
	}
}
