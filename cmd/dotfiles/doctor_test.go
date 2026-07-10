package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type doctorFixture struct {
	report          doctorReport
	runningPath     string
	shadowPath      string
	brewPATHLink    string
	legacyTUI       string
	legacySetup     string
	executionMarker string
}

func TestCollectDoctorReportFindsPATHHomebrewAndLegacyCollisions(t *testing.T) {
	fixture := newDoctorFixture(t)
	report := fixture.report

	if report.SchemaVersion != doctorSchemaVersion {
		t.Fatalf("schema version = %d, want %d", report.SchemaVersion, doctorSchemaVersion)
	}
	if report.RunningExecutable.Path != fixture.runningPath || report.RunningExecutable.VersionHint != "2.1.2" {
		t.Fatalf("running executable = %+v", report.RunningExecutable)
	}
	if report.RunningExecutable.OwnershipHint != "Homebrew formula "+doctorFormula {
		t.Fatalf("running ownership = %q", report.RunningExecutable.OwnershipHint)
	}
	if len(report.PATHMatches) != 2 {
		t.Fatalf("PATH matches = %+v, want 2", report.PATHMatches)
	}
	first, second := report.PATHMatches[0], report.PATHMatches[1]
	if first.Path != fixture.shadowPath || !first.SelectedByPATH || first.SameAsRunning {
		t.Fatalf("first PATH match = %+v", first)
	}
	if first.VersionHint != "1.4.0" || first.VersionSource != "static script VERSION assignment" {
		t.Fatalf("shadow version = %q from %q", first.VersionHint, first.VersionSource)
	}
	if second.Path != fixture.brewPATHLink || second.SelectedByPATH || !second.SameAsRunning {
		t.Fatalf("second PATH match = %+v", second)
	}
	if second.VersionHint != "2.1.2" || second.OwnershipHint != "Homebrew formula "+doctorFormula+"; same as running executable" {
		t.Fatalf("running PATH match metadata = %+v", second)
	}
	if !report.Homebrew.Installed || report.Homebrew.Formula != doctorFormula || report.Homebrew.ResolvedExecutable != report.RunningExecutable.ResolvedPath || report.Homebrew.VersionHint != "2.1.2" {
		t.Fatalf("Homebrew report = %+v", report.Homebrew)
	}
	if len(report.LegacyBinaries) != 2 || !hasDoctorExecutable(report.LegacyBinaries, fixture.legacyTUI) || !hasDoctorExecutable(report.LegacyBinaries, fixture.legacySetup) {
		t.Fatalf("legacy binaries = %+v", report.LegacyBinaries)
	}

	for _, code := range []string{"path-shadowing", "multiple-path-matches", "version-mismatch", "legacy-binary"} {
		if !hasDoctorFinding(report, code) {
			t.Errorf("missing finding %q: %+v", code, report.Findings)
		}
	}
	if hasDoctorFinding(report, "homebrew-not-running") || hasDoctorFinding(report, "running-not-on-path") {
		t.Errorf("incorrect running/Homebrew finding: %+v", report.Findings)
	}
	if _, err := os.Stat(fixture.executionMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discovered candidate was executed, marker stat error = %v", err)
	}
}

func TestDoctorCommandJSONIsDeterministicAndStructured(t *testing.T) {
	fixture := newDoctorFixture(t)
	collect := func(context.Context) (doctorReport, error) { return fixture.report, nil }

	first := executeDoctorCommand(t, newDoctorCommand(collect), "--json")
	second := executeDoctorCommand(t, newDoctorCommand(collect), "--json")
	if first != second {
		t.Fatalf("JSON output changed between runs:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	var decoded doctorReport
	if err := json.Unmarshal([]byte(first), &decoded); err != nil {
		t.Fatalf("decode doctor JSON: %v\n%s", err, first)
	}
	if decoded.SchemaVersion != doctorSchemaVersion || len(decoded.PATHMatches) != 2 || len(decoded.LegacyBinaries) != 2 || len(decoded.Findings) == 0 {
		t.Fatalf("decoded JSON missing structured fields: %+v", decoded)
	}
	if strings.Contains(first, "checked_at") || strings.Contains(first, "generated_at") {
		t.Fatalf("JSON contains nondeterministic timestamp field:\n%s", first)
	}
}

func TestDoctorCommandHumanOutputIsReadOnlyAndActionable(t *testing.T) {
	fixture := newDoctorFixture(t)
	collect := func(context.Context) (doctorReport, error) { return fixture.report, nil }
	output := executeDoctorCommand(t, newDoctorCommand(collect))

	for _, expected := range []string{
		"Dotfiles Doctor",
		"Running executable: " + fixture.runningPath,
		fixture.shadowPath + " [selected by PATH]",
		"managed executable:",
		fixture.legacyTUI,
		"WARNING path-shadowing",
		"Read-only check complete; no files were changed.",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("human output missing %q:\n%s", expected, output)
		}
	}
}

func TestCollectDoctorReportTreatsHomebrewProbeFailureAsFinding(t *testing.T) {
	running := writeDoctorScript(t, filepath.Join(t.TempDir(), "dotfiles"), "2.1.2", "")
	deps := doctorDependencies{
		executable:  func() (string, error) { return running, nil },
		userHomeDir: func() (string, error) { return t.TempDir(), nil },
		getenv: func(key string) string {
			if key == "PATH" {
				return filepath.Dir(running)
			}
			return ""
		},
		homebrewProbe: func(context.Context, string) (string, string, error) {
			return "", "", errors.New("brew was not found on PATH")
		},
	}
	report, err := collectDoctorReport(context.Background(), deps, "2.1.2")
	if err != nil {
		t.Fatal(err)
	}
	if report.Homebrew.Installed || report.Homebrew.ProbeError != "brew was not found on PATH" {
		t.Fatalf("Homebrew failure = %+v", report.Homebrew)
	}
	if !hasDoctorFinding(report, "homebrew-unavailable") {
		t.Fatalf("missing Homebrew unavailable finding: %+v", report.Findings)
	}
}

func TestCollectDoctorReportFailsWhenRunningExecutableCannotBeLocated(t *testing.T) {
	deps := doctorDependencies{
		executable:  func() (string, error) { return "", errors.New("unavailable") },
		userHomeDir: os.UserHomeDir,
		getenv:      os.Getenv,
		homebrewProbe: func(context.Context, string) (string, string, error) {
			return "", "", errors.New("unused")
		},
	}
	if _, err := collectDoctorReport(context.Background(), deps, "dev"); err == nil || !strings.Contains(err.Error(), "locate running executable") {
		t.Fatalf("collect error = %v, want running executable failure", err)
	}
}

func TestDoctorCommandReturnsCollectorAndWriterErrors(t *testing.T) {
	collectorErr := errors.New("probe failed")
	cmd := newDoctorCommand(func(context.Context) (doctorReport, error) {
		return doctorReport{}, collectorErr
	})
	cmd.SetArgs(nil)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	if err := cmd.Execute(); !errors.Is(err, collectorErr) {
		t.Fatalf("collector error = %v, want %v", err, collectorErr)
	}

	writerErr := errors.New("writer failed")
	cmd = newDoctorCommand(func(context.Context) (doctorReport, error) {
		return doctorReport{SchemaVersion: doctorSchemaVersion}, nil
	})
	cmd.SetOut(errorWriter{err: writerErr})
	cmd.SetArgs([]string{"--json"})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	if err := cmd.Execute(); !errors.Is(err, writerErr) {
		t.Fatalf("writer error = %v, want %v", err, writerErr)
	}
}

func TestDoctorCommandRegisteredWithAccurateHelp(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"doctor"})
	if err != nil {
		t.Fatal(err)
	}
	if found != doctorCmd {
		t.Fatalf("root doctor command = %p, want %p", found, doctorCmd)
	}
	help := strings.Join(strings.Fields(doctorCmd.Long+" "+doctorCmd.Flags().Lookup("json").Usage), " ")
	for _, expected := range []string{"read-only", "does not execute discovered dotfiles binaries", "never deletes or modifies files", "deterministic JSON"} {
		if !strings.Contains(help, expected) {
			t.Errorf("doctor help missing %q: %s", expected, help)
		}
	}
}

func TestDoctorImplementationHasNoFilesystemMutationOrCandidateExecution(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	sourcePath := filepath.Join(filepath.Dir(thisFile), "doctor.go")
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, sourcePath, nil, 0)
	if err != nil {
		t.Fatalf("parse doctor implementation: %v", err)
	}

	forbiddenOSCalls := map[string]struct{}{
		"Chmod": {}, "Chown": {}, "Create": {}, "CreateTemp": {}, "Link": {}, "Mkdir": {}, "MkdirAll": {},
		"Remove": {}, "RemoveAll": {}, "Rename": {}, "Symlink": {}, "Truncate": {}, "WriteFile": {},
	}
	commandCalls := 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if pkg.Name == "os" {
			if _, forbidden := forbiddenOSCalls[selector.Sel.Name]; forbidden {
				t.Errorf("doctor must remain read-only; found os.%s at %s", selector.Sel.Name, fset.Position(call.Pos()))
			}
		}
		if pkg.Name == "exec" && selector.Sel.Name == "CommandContext" {
			commandCalls++
			if len(call.Args) < 3 {
				t.Errorf("unexpected command invocation at %s", fset.Position(call.Pos()))
				return true
			}
			argument, ok := call.Args[2].(*ast.BasicLit)
			if !ok || argument.Value != `"--prefix"` {
				t.Errorf("doctor may execute only the read-only brew --prefix probe at %s", fset.Position(call.Pos()))
			}
		}
		return true
	})
	if commandCalls != 1 {
		t.Fatalf("doctor command execution count = %d, want one brew --prefix probe", commandCalls)
	}
}

func newDoctorFixture(t *testing.T) doctorFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("doctor collision fixture requires Unix symlink semantics")
	}
	workspace := t.TempDir()
	executionMarker := filepath.Join(workspace, "candidate-executed")
	runningPath := writeDoctorScript(t, filepath.Join(workspace, "cellar", "dotfiles", "2.1.2", "bin", "dotfiles"), "2.1.2", executionMarker)
	shadowPath := writeDoctorScript(t, filepath.Join(workspace, "shadow", "dotfiles"), "1.4.0", executionMarker)
	legacyTUI := writeDoctorScript(t, filepath.Join(workspace, "shadow", "dotfiles-tui"), "0.9.0", executionMarker)
	home := filepath.Join(workspace, "home")
	legacySetup := writeDoctorScript(t, filepath.Join(home, ".local", "bin", "dotfiles-setup"), "1.0.1", executionMarker)

	brewPrefix := filepath.Join(workspace, "opt", "dotfiles")
	if err := os.MkdirAll(filepath.Join(brewPrefix, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	managedExecutable := filepath.Join(brewPrefix, "bin", "dotfiles")
	if err := os.Symlink(runningPath, managedExecutable); err != nil {
		t.Fatal(err)
	}
	brewBin := filepath.Join(workspace, "brew-bin")
	if err := os.MkdirAll(brewBin, 0o700); err != nil {
		t.Fatal(err)
	}
	brewPATHLink := filepath.Join(brewBin, "dotfiles")
	if err := os.Symlink(managedExecutable, brewPATHLink); err != nil {
		t.Fatal(err)
	}

	environment := map[string]string{
		"PATH":    strings.Join([]string{filepath.Dir(shadowPath), brewBin}, string(os.PathListSeparator)),
		"PATHEXT": "",
	}
	deps := doctorDependencies{
		executable:  func() (string, error) { return runningPath, nil },
		userHomeDir: func() (string, error) { return home, nil },
		getenv:      func(key string) string { return environment[key] },
		homebrewProbe: func(_ context.Context, formula string) (string, string, error) {
			if formula != doctorFormula {
				t.Fatalf("formula = %q, want %q", formula, doctorFormula)
			}
			return filepath.Join(workspace, "brew"), brewPrefix, nil
		},
	}
	report, err := collectDoctorReport(context.Background(), deps, "2.1.2")
	if err != nil {
		t.Fatal(err)
	}
	return doctorFixture{
		report:          report,
		runningPath:     runningPath,
		shadowPath:      shadowPath,
		brewPATHLink:    brewPATHLink,
		legacyTUI:       legacyTUI,
		legacySetup:     legacySetup,
		executionMarker: executionMarker,
	}
}

func writeDoctorScript(t *testing.T, path, scriptVersion, executionMarker string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\nVERSION=\"" + scriptVersion + "\"\n"
	if executionMarker != "" {
		body += "printf executed > \"" + executionMarker + "\"\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(path)
}

func executeDoctorCommand(t *testing.T, cmd *cobra.Command, args ...string) string {
	t.Helper()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute doctor command: %v", err)
	}
	return output.String()
}

type errorWriter struct{ err error }

func (writer errorWriter) Write(_ []byte) (int, error) { return 0, writer.err }

func hasDoctorFinding(report doctorReport, code string) bool {
	for _, finding := range report.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func hasDoctorExecutable(executables []doctorExecutable, path string) bool {
	for _, executable := range executables {
		if executable.Path == path {
			return true
		}
	}
	return false
}
