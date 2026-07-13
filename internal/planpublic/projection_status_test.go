package planpublic

import (
	"bytes"
	"testing"
)

func TestDocumentStatusIsTypedReadOnlyOutcome(t *testing.T) {
	ready := readyProjectionSpec()
	noChanges := readyProjectionSpec()
	noChanges.Status = StatusNoChanges
	noChanges.Actions = []ActionSpec{{
		ActionID: "install:codex", Kind: "install_tool", ToolID: "codex", Description: "install codex",
		Disposition: "skip", ReasonCode: "present", Reason: "already present", Ownership: "package_manager", Reversibility: "external",
	}}
	noChanges.Summary = Summary{Skip: 1}
	blocked := readyProjectionSpec()
	blocked.Status = StatusBlocked
	blocked.Actions = []ActionSpec{{
		ActionID: "install:codex", Kind: "install_tool", ToolID: "codex", Description: "install codex",
		Disposition: "blocked", ReasonCode: "unknown", Reason: "installation status unknown", Ownership: "package_manager", Reversibility: "external",
	}}
	blocked.Summary = Summary{Blocked: 1}
	intentRequired := DocumentSpec{Status: StatusIntentRequired, Intent: Intent{Source: "explicit_tools", Tools: []string{}}}

	for _, test := range []struct {
		name string
		spec DocumentSpec
		want Status
	}{
		{name: "ready", spec: ready, want: StatusReady},
		{name: "no changes", spec: noChanges, want: StatusNoChanges},
		{name: "blocked", spec: blocked, want: StatusBlocked},
		{name: "intent required", spec: intentRequired, want: StatusIntentRequired},
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
