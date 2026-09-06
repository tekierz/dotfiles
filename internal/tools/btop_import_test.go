package tools

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestBtopConfigMutationPathUsesAbsoluteXDGAndRejectsRelative(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if got, err := BtopConfigMutationPath(); err != nil || got != filepath.Join(xdg, "btop", "btop.conf") {
		t.Fatalf("absolute XDG path = %q, %v", got, err)
	}
	t.Setenv("XDG_CONFIG_HOME", "relative")
	if got, err := BtopConfigMutationPath(); err == nil || got != "" {
		t.Fatalf("relative XDG path = %q, %v", got, err)
	}
	tool := NewBtopTool()
	if !tool.HasConfig() || len(tool.ConfigPaths()) != 0 {
		t.Fatalf("registry advertised config under invalid XDG: %v", tool.ConfigPaths())
	}
}

func TestBtopConfigMutationPathDoesNotRequireHomeWithAbsoluteXDG(t *testing.T) {
	t.Setenv("HOME", "")
	xdg := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if got, err := BtopConfigMutationPath(); err != nil || got != filepath.Join(xdg, "btop", "btop.conf") {
		t.Fatalf("absolute XDG without HOME = %q, %v", got, err)
	}
}

func TestBtopReviewedReleaseFailureReturnsCommittedEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	configPath := filepath.Join(home, ".config", "btop", "btop.conf")
	themePath := filepath.Join(home, ".config", "btop", "themes", "dracula.theme")
	if err := os.MkdirAll(filepath.Dir(themePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("# native\nupdate_ms = 1000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, configRevision, configParents, err := safefile.ObserveFileWithin(home, ".config/btop/btop.conf")
	if err != nil {
		t.Fatal(err)
	}
	_, themeRevision, themeParents, err := safefile.ObserveFileWithin(home, ".config/btop/themes/dracula.theme")
	if err != nil {
		t.Fatal(err)
	}
	releaseErr := errors.New("injected btop unlock failure")
	locker := operation.Locker(func(string, string) (func() error, error) {
		return func() error { return releaseErr }, nil
	})
	evidence, err := WriteBtopConfigAtAuthoritiesTracked(
		BtopConfig{UpdateMs: 2500, GraphType: "block", ShowTemp: true, TempScale: "celsius", ShownBoxes: "cpu mem"}, "dracula",
		themeRevision, themeParents, configRevision, configParents, locker,
	)
	var partial *PartialMutationError
	if evidence != nil || !errors.Is(err, releaseErr) || !errors.As(err, &partial) || len(partial.Evidence) != 2 {
		t.Fatalf("evidence=%+v error=%v partial=%+v", evidence, err, partial)
	}
}

func TestParseBtopConfigImportUsesLastValidAssignmentWithProvenance(t *testing.T) {
	content := []byte("update_ms = 1000\n# native duplicate\nupdate_ms = 2750\ncolor_theme = \"nord\"\nshow_coretemp = false\ngraph_symbol = \"block\"\ntemp_scale = \"fahrenheit\"\nshown_boxes = \"cpu mem\"\n")
	got, err := parseBtopConfigImport("/tmp/btop.conf", content, true)
	if err != nil || len(got.Warnings) != 0 {
		t.Fatalf("parse = %+v, %v", got, err)
	}
	if got.Config.UpdateMs != 2750 || got.Fields[BtopFieldUpdateMs].Line != 3 || got.Fields[BtopFieldUpdateMs].Scope != ConfigValueNative {
		t.Fatalf("last update assignment not imported: %+v %+v", got.Config, got.Fields[BtopFieldUpdateMs])
	}
	if got.Config.Theme != "nord" || got.Config.ShowTemp || got.Config.GraphType != "block" || got.Config.TempScale != "fahrenheit" || got.Config.ShownBoxes != "cpu mem" {
		t.Fatalf("modeled values = %+v", got.Config)
	}
}

func TestParseBtopConfigImportRejectsMalformedModeledLineAndMarkerAmbiguity(t *testing.T) {
	got, err := parseBtopConfigImport("/tmp/btop.conf", []byte("update_ms 2000\n"), true)
	if err != nil || len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "malformed") {
		t.Fatalf("malformed import = %+v, %v", got, err)
	}
	_, err = parseBtopConfigImport("/tmp/btop.conf", []byte(btopManagedStart+"\nupdate_ms = 2000\n"), true)
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("incomplete marker error = %v", err)
	}
	// Indented marker text is native comment text, not an ownership boundary.
	got, err = parseBtopConfigImport("/tmp/btop.conf", []byte("  "+btopManagedStart+"\nupdate_ms = 2000\n"), true)
	if err != nil || got.Managed || got.Fields[BtopFieldUpdateMs].Scope != ConfigValueNative {
		t.Fatalf("indented marker granted ownership: %+v, %v", got, err)
	}
}

func TestParseBtopLiteralMatchesUpstreamFirstTokenAndFirstQuoteSemantics(t *testing.T) {
	if value, ok := parseBtopLiteral("2000#comment"); !ok {
		t.Fatal("upstream token was not classified")
	} else if _, mistakenForInteger := value.(int); mistakenForInteger {
		t.Fatal("2000#comment was incorrectly accepted as integer 2000")
	}
	if value, ok := parseBtopLiteral("+2000"); !ok {
		t.Fatal("signed token was not classified")
	} else if _, mistakenForInteger := value.(int); mistakenForInteger {
		t.Fatal("+2000 was incorrectly accepted as upstream integer")
	}
	for token, want := range map[string]bool{"True": true, "False": false} {
		if value, ok := parseBtopLiteral(token); !ok || value != want {
			t.Fatalf("upstream bool %q = %#v, %v", token, value, ok)
		}
	}
	if value, ok := parseBtopLiteral(`"nord"#comment`); !ok || value != "nord" {
		t.Fatalf("quoted trailing text semantics = %#v, %v", value, ok)
	}
	if value, ok := parseBtopLiteral(`"private\"theme"`); !ok || value != `private\` {
		t.Fatalf("first quote semantics = %#v, %v", value, ok)
	}
	for _, valid := range []string{"2000 # comment", "true # comment", `"nord" # comment`, "nord # comment", "block"} {
		if _, ok := parseBtopLiteral(valid); !ok {
			t.Fatalf("valid upstream literal rejected: %q", valid)
		}
	}
}

func TestMergeBtopManagedSectionPreservesNativeBytesAndReplacesExactly(t *testing.T) {
	native := []byte("# native\r\nproc_sorting = \"memory\"\r\nupdate_ms = 999\r\n")
	cfg := BtopConfig{Theme: "nord", UpdateMs: 2500, GraphType: "block", ShownBoxes: "cpu mem", TempScale: "fahrenheit"}
	first, added, err := mergeBtopManagedSection(native, cfg, "catppuccin-mocha")
	if err != nil || !added || !bytes.HasPrefix(first, native) {
		t.Fatalf("first merge did not preserve native prefix: added=%v err=%v\n%q", added, err, first)
	}
	if bytes.Count(first, []byte(btopManagedStart)) != 1 || !strings.HasSuffix(string(first), btopManagedEnd+"\n") {
		t.Fatalf("managed section is not exact/trailing:\n%s", first)
	}
	secondCfg := cfg
	secondCfg.UpdateMs = 3000
	second, added, err := mergeBtopManagedSection(first, secondCfg, "catppuccin-mocha")
	if err != nil || added || !bytes.HasPrefix(second, native) || bytes.Count(second, []byte(btopManagedStart)) != 1 || !bytes.Contains(second, []byte("update_ms = 3000")) {
		t.Fatalf("replacement was not bounded/idempotent: added=%v err=%v\n%s", added, err, second)
	}
}

func TestImportBtopConfigIsReadOnlyWhenConfigRootMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	got, err := ImportBtopConfig()
	if err != nil || len(got.Sources) != 1 || got.Sources[0].Exists {
		t.Fatalf("missing import = %+v, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config")); !os.IsNotExist(err) {
		t.Fatalf("read-only import created config root: %v", err)
	}
}
