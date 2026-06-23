package ui

// manage_field_apply.go is the SINGLE source of truth for "which Manage editor
// fields actually apply to a real config file." Both the UI (manage_dualpane.go,
// to mark a control as not-applied) and the guardrail test
// (manage_field_apply_test.go) consume this so the two can never drift: a field
// the UI presents as editable-and-saving MUST be classified here, and the test
// fails if any presented field key is unclassified.
//
// The P1-B fix this enforces: the Manage save reports "Saved ✓"; before this, ~25
// editable fields had NO generator behind them, so editing them changed nothing on
// disk while the UI claimed success — a correctness lie. Now every editable field
// is EITHER wired through a generator (round-trips to a file; proven by the
// round-trip tests) OR explicitly listed as not-applied so the UI can label it and
// the user is never misled.

// manageNotAppliedFields lists, per tool ID, the field keys (as declared in
// manageFieldsFor) that are intentionally NOT wired to any generator output. The
// UI renders these with a "(not applied)" marker; the save path never claims they
// were written. Every key here MUST be a real field key presented by
// manageFieldsFor (the guardrail test enforces both directions).
//
// LazyDocker has no generator and no config-file writer at all (see
// internal/tools — there is no lazydocker.go), so all of its controls are
// not-applied. They remain editable only so the preference is remembered in
// manage.json; they do not write a config file.
var manageNotAppliedFields = map[string]map[string]bool{
	"lazydocker": {
		"mouse": true, // LazyDockerMouseMode — no generator
		"tail":  true, // LazyDockerLogsTail — no generator
	},
}

// manageFieldIsApplied reports whether the given Manage field (toolID + field key)
// is wired through a generator (true) or is an explicitly not-applied control
// (false). Unknown/unclassified fields default to applied; the guardrail test is
// what guarantees no field is silently unclassified, so the default only ever
// applies to fields the test has already vetted.
//
// The manageItemGlobal tool's fields (theme/nav/animations) are not tool config generator
// fields — they persist through the global config + theme re-apply path — so they
// are treated as applied (not subject to the not-applied marker).
func manageFieldIsApplied(toolID, fieldKey string) bool {
	if tool, ok := manageNotAppliedFields[toolID]; ok {
		if tool[fieldKey] {
			return false
		}
	}
	return true
}
