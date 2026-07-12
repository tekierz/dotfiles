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

func TestNormalizeExplicitToolsRequiresNonEmptyIntent(t *testing.T) {
	for _, values := range [][]string{nil, {}, {""}, {"  ", "\t"}} {
		intent, err := NormalizeExplicitTools(values, []string{"codex", "pi"})
		if !errors.Is(err, ErrIntentRequired) {
			t.Fatalf("NormalizeExplicitTools(%q) error=%v, want ErrIntentRequired", values, err)
		}
		if intent.Source != "" || len(intent.Tools) != 0 || intent.Digest != "" {
			t.Fatalf("missing intent leaked a partial value: %+v", intent)
		}
	}
}

func TestNormalizeExplicitToolsTrimsDeduplicatesSortsAndDigests(t *testing.T) {
	registered := []string{"pi", "codex", "opencode"}
	intent, err := NormalizeExplicitTools([]string{" pi ", "codex", "pi", " codex "}, registered)
	if err != nil {
		t.Fatal(err)
	}
	wantTools := []string{"codex", "pi"}
	if intent.Source != "explicit_tools" || !reflect.DeepEqual(intent.Tools, wantTools) {
		t.Fatalf("normalized intent=%+v, want source explicit_tools and tools %v", intent, wantTools)
	}
	canonical, err := json.Marshal(struct {
		SchemaVersion int      `json:"schema_version"`
		Source        string   `json:"source"`
		Tools         []string `json:"tools"`
	}{SchemaVersion: 1, Source: "explicit_tools", Tools: wantTools})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	if want := hex.EncodeToString(digest[:]); intent.Digest != want {
		t.Fatalf("intent digest=%q, want SHA-256(versioned explicit-intent envelope) %q", intent.Digest, want)
	}
	if len(intent.Digest) != 64 {
		t.Fatalf("intent digest length=%d, want 64", len(intent.Digest))
	}
}

func TestNormalizeExplicitToolsRejectsUnknownOrNoncanonicalIDsAtomically(t *testing.T) {
	registered := []string{"codex", "pi", "cursor-agent"}
	for _, values := range [][]string{
		{"unknown"},
		{"Codex"},
		{"cursor_agent"},
		{"../codex"},
		{"codex", "PI"},
		{"codex", " "},
		{" ", "codex"},
		{"codex\npi"},
		{"codex‮"},
		{"-codex"},
		{"codex-"},
		{"co--dex"},
		{strings.Repeat("a", 65)},
		{"codex\x00"},
	} {
		intent, err := NormalizeExplicitTools(values, registered)
		if !errors.Is(err, ErrInvalidIntent) {
			t.Fatalf("NormalizeExplicitTools(%q) error=%v, want ErrInvalidIntent", values, err)
		}
		if intent.Source != "" || len(intent.Tools) != 0 || intent.Digest != "" {
			t.Fatalf("invalid intent leaked a partial value: %+v", intent)
		}
	}
}

func TestNormalizeExplicitToolsRejectsInvalidRegistryMetadataAtomically(t *testing.T) {
	for _, registered := range [][]string{
		{"codex", "codex"},
		{"codex", "Codex"},
		{"codex", ""},
		{"codex", "cursor_agent"},
		{"codex", "../pi"},
		{"codex", "-pi"},
		{"codex", "pi-"},
		{"codex", "pi--agent"},
		{"codex", strings.Repeat("a", 65)},
		{"codex", "pi\t"},
	} {
		intent, err := NormalizeExplicitTools([]string{"codex"}, registered)
		if !errors.Is(err, ErrInvalidIntent) {
			t.Fatalf("NormalizeExplicitTools with registry %q error=%v, want ErrInvalidIntent", registered, err)
		}
		if intent.Source != "" || len(intent.Tools) != 0 || intent.Digest != "" {
			t.Fatalf("invalid registry metadata leaked partial intent: %+v", intent)
		}
	}
}

func TestNormalizeExplicitToolsAcceptsExactBoundedIDGrammar(t *testing.T) {
	maxID := strings.Repeat("a", 64)
	registered := []string{"a", "a1", "cursor-agent", maxID}
	intent, err := NormalizeExplicitTools([]string{maxID, "cursor-agent", "a1", "a"}, registered)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "a1", maxID, "cursor-agent"}
	if !reflect.DeepEqual(intent.Tools, want) {
		t.Fatalf("bounded grammar tools=%v, want %v", intent.Tools, want)
	}
}

func TestNormalizeExplicitToolsDoesNotAliasCallerInputs(t *testing.T) {
	raw := []string{"pi", "codex"}
	registered := []string{"codex", "pi"}
	intent, err := NormalizeExplicitTools(raw, registered)
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = "unknown"
	registered[0] = "unknown"
	if !reflect.DeepEqual(intent.Tools, []string{"codex", "pi"}) {
		t.Fatalf("normalized intent aliases caller input: %+v", intent)
	}
	intent.Tools[0] = "mutated"
	second, err := NormalizeExplicitTools([]string{"pi", "codex"}, []string{"codex", "pi"})
	if err != nil || !reflect.DeepEqual(second.Tools, []string{"codex", "pi"}) {
		t.Fatalf("caller mutation contaminated later normalization: %+v err=%v", second, err)
	}
}
