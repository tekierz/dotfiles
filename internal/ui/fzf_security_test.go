package ui

import (
	"strings"
	"testing"
)

func TestFzfAdditionalFlagsFieldDescribesLiteralDataBoundary(t *testing.T) {
	a := &App{manageConfig: NewManageConfig()}
	fields := a.manageFieldsFor("fzf")
	if len(fields) == 0 || fields[0].key != "opts" {
		t.Fatalf("fzf additional flags field missing: %+v", fields)
	}
	field := fields[0]
	if !strings.Contains(strings.ToLower(field.label), "flags") ||
		!strings.Contains(strings.ToLower(field.description), "stored as data") ||
		!strings.Contains(strings.ToLower(field.description), "never evaluated") {
		t.Fatalf("fzf field still implies raw shell input: label=%q description=%q", field.label, field.description)
	}
}
