package planpublic

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestDocumentStatusIsTypedReadOnlyOutcome(t *testing.T) {
	ready := readyProjectionSpec()
	noChanges := readyProjectionSpec()
	noChanges.Status = StatusNoChanges
	noChanges.PlanHash = ""
	noChanges.Capabilities.Apply = "not_available"
	noChanges.Actions = []ActionSpec{{
		ActionID: "install:codex", Kind: "install_tool", ToolID: "codex", Description: "install codex",
		Disposition: "skip", ReasonCode: "present", Reason: "already present", Ownership: "package_manager", Reversibility: "external",
	}}
	noChanges.Summary = Summary{Skip: 1}
	blocked := readyProjectionSpec()
	blocked.Status = StatusBlocked
	blocked.PlanHash = ""
	blocked.Capabilities.Apply = "not_available"
	blocked.Actions = []ActionSpec{{
		ActionID: "install:codex", Kind: "install_tool", ToolID: "codex", Description: "install codex",
		Disposition: "blocked", ReasonCode: "unknown", Reason: "installation status unknown", Ownership: "package_manager", Reversibility: "external",
	}}
	blocked.Summary = Summary{Blocked: 1}
	intentRequired := DocumentSpec{Status: StatusIntentRequired, Intent: Intent{Source: "explicit_tools", Tools: []string{}}}

	for _, test := range []struct {
		name      string
		spec      DocumentSpec
		want      Status
		wantApply string
	}{
		{name: "ready", spec: ready, want: StatusReady, wantApply: "hash_required"},
		{name: "no changes", spec: noChanges, want: StatusNoChanges, wantApply: "not_available"},
		{name: "blocked", spec: blocked, want: StatusBlocked, wantApply: "not_available"},
		{name: "intent required", spec: intentRequired, want: StatusIntentRequired, wantApply: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := NewDocument(test.spec)
			if err != nil {
				t.Fatal(err)
			}
			before, err := MarshalDocument(document)
			if err != nil {
				t.Fatal(err)
			}
			if got := document.Status(); got != test.want {
				t.Fatalf("Status() = %q, want %q", got, test.want)
			}
			if !strings.Contains(string(before), `"apply":"`+test.wantApply+`"`) {
				t.Fatalf("status %q apply capability mismatch: %s", test.want, before)
			}
			after, err := MarshalDocument(document)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("Status() changed canonical marshaling:\nbefore %s\nafter  %s", before, after)
			}
		})
	}

	if got := (Document{}).Status(); got != Status("") || validStatus(got) {
		t.Fatalf("zero Document Status() = %q, want invalid empty status", got)
	}
}

func TestApplyCapabilityMustMatchStatusAuthorityShape(t *testing.T) {
	ready := readyProjectionSpec()
	noChanges := readyProjectionSpec()
	noChanges.Status = StatusNoChanges
	noChanges.PlanHash = ""
	noChanges.Capabilities.Apply = "not_available"
	noChanges.Actions = []ActionSpec{{
		ActionID: "install:codex", Kind: "install_tool", ToolID: "codex", Description: "install codex",
		Disposition: "skip", ReasonCode: "present", Reason: "already present", Ownership: "package_manager", Reversibility: "external",
	}}
	noChanges.Summary = Summary{Skip: 1}
	blocked := noChanges
	blocked.Status = StatusBlocked
	blocked.Actions = []ActionSpec{{
		ActionID: "install:codex", Kind: "install_tool", ToolID: "codex", Description: "install codex",
		Disposition: "blocked", ReasonCode: "unknown", Reason: "installation status unknown", Ownership: "package_manager", Reversibility: "external",
	}}
	blocked.Summary = Summary{Blocked: 1}

	for _, test := range []struct {
		name  string
		spec  DocumentSpec
		apply string
	}{
		{name: "ready cannot disable hash gate", spec: ready, apply: "not_available"},
		{name: "no changes cannot advertise hash gate", spec: noChanges, apply: "hash_required"},
		{name: "blocked cannot advertise hash gate", spec: blocked, apply: "hash_required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.spec.Capabilities.Apply = test.apply
			if _, err := NewDocument(test.spec); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("status %q accepted apply=%q: %v", test.spec.Status, test.apply, err)
			}
		})
	}
}
