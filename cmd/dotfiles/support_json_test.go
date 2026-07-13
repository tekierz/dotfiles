package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestSupportProjectionCompleteCanonicalDigestAndEmbeddedStatus(t *testing.T) {
	status := supportTestStatus(t)
	provenance, err := projectSupportProvenance(supportTestDoctorReport())
	if err != nil {
		t.Fatal(err)
	}
	operations, err := projectSupportOperations(operation.JournalSummarySet{Records: []operation.JournalSummary{}})
	if err != nil {
		t.Fatal(err)
	}
	document, err := newValidatedSupportDocument(supportProjectionSpec{
		Build: supportTestBuild(), Installation: collectedSupportInstallation(status),
		Provenance: collectedSupportProvenance(provenance), Operations: collectedSupportOperations(operations),
	})
	if err != nil {
		t.Fatal(err)
	}
	object, err := marshalSupportDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	line, err := marshalSupportDocumentLine(document)
	if err != nil || !bytes.Equal(line, append(bytes.Clone(object), '\n')) || bytes.Count(line, []byte("\n")) != 1 {
		t.Fatalf("line=%q error=%v", line, err)
	}
	if !bytes.HasPrefix(object, []byte(`{"schema_version":1,"kind":"dotfiles.support","outcome":"complete","authority":{"public_digest":"`)) {
		t.Fatalf("unexpected key order: %s", object)
	}
	decoded := cloneSupportDocument(document.value)
	if decoded.Outcome != supportComplete || decoded.Capabilities.Operations != supportCollected || decoded.Installation.Document == nil ||
		!validSHA256String(decoded.Authority.PublicDigest) {
		t.Fatalf("document=%+v", decoded)
	}
	if got := independentSupportDigest(t, decoded); got != decoded.Authority.PublicDigest {
		t.Fatalf("digest=%q want=%q", decoded.Authority.PublicDigest, got)
	}
	statusBytes, err := status.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(object, statusBytes) {
		t.Fatal("support output does not embed exact status object")
	}

	mutated := bytes.Clone(object)
	mutated[0] = '['
	again, err := document.MarshalJSON()
	if err != nil || !bytes.Equal(again, object) || bytes.Equal(again, mutated) {
		t.Fatalf("marshal aliases caller bytes: %s %v", again, err)
	}
}

func TestSupportProjectionDerivesPartialShapesAndCapabilities(t *testing.T) {
	status := supportTestStatus(t)
	provenance, _ := projectSupportProvenance(supportTestDoctorReport())
	operations, _ := projectSupportOperations(operation.JournalSummarySet{Records: []operation.JournalSummary{}})
	tests := []struct {
		name         string
		installation supportInstallationInput
		provenance   supportProvenanceInput
		operations   supportOperationsInput
		outcome      string
		operationCap string
	}{
		{name: "complete collected", installation: collectedSupportInstallation(status), provenance: collectedSupportProvenance(provenance), operations: collectedSupportOperations(operations), outcome: supportComplete, operationCap: supportCollected},
		{name: "complete no journal", installation: collectedSupportInstallation(status), provenance: collectedSupportProvenance(provenance), operations: notPresentSupportOperations(), outcome: supportComplete, operationCap: supportNotPresent},
		{name: "installation partial", installation: unavailableSupportInstallation(supportReasonStatusUnavailable), provenance: collectedSupportProvenance(provenance), operations: notPresentSupportOperations(), outcome: supportPartial, operationCap: supportNotPresent},
		{name: "provenance partial", installation: collectedSupportInstallation(status), provenance: unavailableSupportProvenance(supportReasonCancelled), operations: notPresentSupportOperations(), outcome: supportPartial, operationCap: supportNotPresent},
		{name: "operations partial", installation: collectedSupportInstallation(status), provenance: collectedSupportProvenance(provenance), operations: unavailableSupportOperations(supportReasonJournalInvalid), outcome: supportPartial, operationCap: supportUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, err := newValidatedSupportDocument(supportProjectionSpec{Build: supportTestBuild(), Installation: test.installation, Provenance: test.provenance, Operations: test.operations})
			if err != nil {
				t.Fatal(err)
			}
			if document.value.Outcome != test.outcome || document.value.Capabilities.Operations != test.operationCap || document.value.Capabilities.Installation != document.value.Installation.Collection || document.value.Capabilities.Provenance != document.value.Provenance.Collection {
				t.Fatalf("derived shape=%+v", document.value)
			}
		})
	}

	invalid := []supportProjectionSpec{
		{Build: supportTestBuild(), Installation: unavailableSupportInstallation("raw error"), Provenance: collectedSupportProvenance(provenance), Operations: notPresentSupportOperations()},
		{Build: supportTestBuild(), Installation: collectedSupportInstallation(status), Provenance: unavailableSupportProvenance("unknown"), Operations: notPresentSupportOperations()},
		{Build: supportTestBuild(), Installation: collectedSupportInstallation(status), Provenance: collectedSupportProvenance(provenance), Operations: unavailableSupportOperations("unknown")},
	}
	for _, spec := range invalid {
		if _, err := newValidatedSupportDocument(spec); !errors.Is(err, errSupportProjection) {
			t.Fatalf("invalid spec error=%v", err)
		}
	}
}

func TestSupportBuildNormalizationIsClosedAndOmitsRevision(t *testing.T) {
	versions := map[string][2]string{
		"dev": {"dev", "development"}, "0.0.0": {"0.0.0", "compiled"}, "99999.1.2": {"99999.1.2", "compiled"},
		"01.2.3": {"unknown", "unknown"}, "1.2": {"unknown", "unknown"}, "1.2.3-beta": {"unknown", "unknown"}, " 1.2.3": {"unknown", "unknown"}, "１２.2.3": {"unknown", "unknown"},
	}
	for input, want := range versions {
		got := normalizeSupportBuild(supportBuildInput{ProductVersion: input, GoVersion: "go1.26.1", GOOS: "darwin", GOARCH: "arm64", BuildInfoReadable: true, Settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "false"}, {Key: "vcs.revision", Value: "SECRETREVISION"}}})
		if got.Version != want[0] || got.VersionSource != want[1] || got.VCSState != "clean" {
			t.Fatalf("version %q => %+v want=%v", input, got, want)
		}
		encoded, _ := json.Marshal(got)
		if bytes.Contains(encoded, []byte("SECRETREVISION")) || bytes.Contains(encoded, []byte("revision")) {
			t.Fatalf("revision leaked: %s", encoded)
		}
	}
	for _, test := range []struct {
		readable bool
		settings []debug.BuildSetting
		want     string
	}{
		{want: "unavailable"},
		{readable: true, want: "unavailable"},
		{readable: true, settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "true"}}, want: "modified"},
		{readable: true, settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "false"}}, want: "clean"},
		{readable: true, settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "false"}, {Key: "vcs.modified", Value: "false"}}, want: "unavailable"},
	} {
		if got := normalizeSupportVCS(test.readable, test.settings); got != test.want {
			t.Fatalf("VCS=%q want=%q", got, test.want)
		}
	}
	bad := normalizeSupportBuild(supportBuildInput{ProductVersion: "1.2.3", GoVersion: ".go", GOOS: "Darwin", GOARCH: "arm-64", BuildInfoReadable: true})
	if bad.GoVersion != "unknown" || bad.OS != "unknown" || bad.Arch != "unknown" {
		t.Fatalf("unsafe build tokens=%+v", bad)
	}
}

func TestSupportProvenanceProjectionValidatesAndOmitsPrivateInputs(t *testing.T) {
	report := supportTestDoctorReport()
	report.PATHMatches = []doctorExecutable{{Path: "/Users/private/token=SECRET", SameAsRunning: false, VersionHint: "1.0.0"}}
	report.RunningExecutable.VersionHint = "2.0.0"
	report.Findings = []doctorFinding{
		{Severity: "warning", Code: "path-shadowing", Summary: "SECRET", Path: "/Users/private"},
		{Severity: "warning", Code: "version-mismatch", Summary: "token=SECRET"},
		{Severity: "info", Code: "running-not-on-path", Summary: "private"},
	}
	data, err := projectSupportProvenance(report)
	if err != nil || data.PATHMatchCount != 1 || data.FindingCount != 3 || len(data.Findings) != 3 {
		t.Fatalf("projection=%+v err=%v", data, err)
	}
	encoded, _ := json.Marshal(data)
	if bytes.Contains(encoded, []byte("SECRET")) || bytes.Contains(encoded, []byte("/Users")) || bytes.Contains(encoded, []byte("token")) {
		t.Fatalf("private provenance leaked: %s", encoded)
	}

	changed := report
	changed.PATHMatches = append([]doctorExecutable(nil), report.PATHMatches...)
	changed.PATHMatches[0].Path = "/different/private/path"
	changed.Findings = append([]doctorFinding(nil), report.Findings...)
	changed.Findings[0].Summary = "different raw detail"
	second, err := projectSupportProvenance(changed)
	if err != nil || !reflect.DeepEqual(data, second) {
		t.Fatalf("sensitive-only change altered projection: %+v %+v %v", data, second, err)
	}

	invalid := report
	invalid.Findings = append([]doctorFinding(nil), report.Findings...)
	invalid.Findings[0].Code = "unknown"
	if _, err := projectSupportProvenance(invalid); !errors.Is(err, errSupportProvenanceInvalid) {
		t.Fatalf("invalid finding error=%v", err)
	}
	over := supportTestDoctorReport()
	over.PATHMatches = make([]doctorExecutable, 1025)
	if _, err := projectSupportProvenance(over); !errors.Is(err, errSupportProvenanceLimit) {
		t.Fatalf("PATH limit error=%v", err)
	}

	unverifiedInstalled := supportTestDoctorReport()
	unverifiedInstalled.Homebrew = doctorHomebrew{Formula: doctorFormula, Installed: true, Prefix: "/opt/homebrew/opt/dotfiles", ManagedExecutable: "/opt/homebrew/opt/dotfiles/bin/dotfiles"}
	unverifiedInstalled.Findings = []doctorFinding{{Severity: "warning", Code: "homebrew-not-running"}}
	if _, err := projectSupportProvenance(unverifiedInstalled); err != nil {
		t.Fatalf("installed but non-running Homebrew should project: %v", err)
	}
}

func TestSupportOperationsProjectionValidatesAndDeepClones(t *testing.T) {
	duration := int64(1250)
	rollback := &operation.JournalRollbackSummary{Status: operation.RollbackIncomplete, Restored: 1, Removed: 2, Skipped: 3, Warnings: 4}
	set := operation.JournalSummarySet{Truncated: false, Records: []operation.JournalSummary{{
		Ordinal: 0, Status: operation.StatusFailed, Actions: operation.JournalActionCounts{Succeeded: 1, Failed: 1}, BackupRecorded: true, Rollback: rollback, DurationMilliseconds: &duration,
	}}}
	data, err := projectSupportOperations(set)
	if err != nil || len(data.Records) != 1 || data.Records[0].Rollback == nil || data.Records[0].DurationMilliseconds == nil {
		t.Fatalf("operations=%+v err=%v", data, err)
	}
	rollback.Status = operation.RollbackFailed
	duration = 99
	set.Records[0].Status = operation.StatusRunning
	if data.Records[0].Status != operation.StatusFailed || data.Records[0].Rollback.Status != operation.RollbackIncomplete || *data.Records[0].DurationMilliseconds != 1250 {
		t.Fatalf("source aliases projection: %+v", data)
	}

	badDuration := int64(-1)
	invalid := []operation.JournalSummarySet{
		{},
		{Records: []operation.JournalSummary{{Ordinal: 1, Status: operation.StatusRunning}}},
		{Records: []operation.JournalSummary{{Ordinal: 0, Status: operation.StatusSucceeded}}},
		{Records: []operation.JournalSummary{{Ordinal: 0, Status: operation.StatusFailed, DurationMilliseconds: &badDuration}}},
		{Truncated: true, Records: []operation.JournalSummary{}},
	}
	for _, value := range invalid {
		if _, err := projectSupportOperations(value); !errors.Is(err, errSupportProjection) {
			t.Fatalf("invalid operations=%+v error=%v", value, err)
		}
	}
}

func TestSupportProjectionRejectsCorruptionAndEnforcesFramingLimit(t *testing.T) {
	document := supportTestDocument(t)
	object, err := marshalSupportDocument(document)
	if err != nil {
		t.Fatal(err)
	}

	corruptions := []func(*validatedSupportDocument){
		func(value *validatedSupportDocument) { value.value.Authority.PublicDigest = strings.Repeat("f", 64) },
		func(value *validatedSupportDocument) { value.value.Outcome = supportPartial },
		func(value *validatedSupportDocument) { value.value.Capabilities.Auth = "collected" },
		func(value *validatedSupportDocument) { value.value.Provenance.Findings = nil },
		func(value *validatedSupportDocument) { value.value.Installation.Document.encoded[0] = '[' },
	}
	for _, corrupt := range corruptions {
		copy := validatedSupportDocument{value: cloneSupportDocument(document.value)}
		corrupt(&copy)
		if encoded, err := marshalSupportDocument(copy); !errors.Is(err, errSupportProjection) || encoded != nil {
			t.Fatalf("corruption accepted: %s %v", encoded, err)
		}
	}

	if line, err := enforceSupportLineLimit(bytes.Repeat([]byte{'a'}, 9), 10); err != nil || len(line) != 10 || line[9] != '\n' {
		t.Fatalf("exact line=%q err=%v", line, err)
	}
	if line, err := enforceSupportLineLimit(bytes.Repeat([]byte{'a'}, 10), 10); !errors.Is(err, errSupportProjection) || line != nil {
		t.Fatalf("oversize line=%q err=%v", line, err)
	}
	if len(object)+1 > supportLineLimit {
		t.Fatal("ordinary support fixture unexpectedly exceeds limit")
	}
}

func supportTestStatus(t *testing.T) validatedStatusDocument {
	t.Helper()
	snapshot := statusJSONSnapshot(t)
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	document, err := collectStatusDocument(context.Background(), statusJSONRuntime{
		tools:          func() []tools.Tool { return []tools.Tool{tools.NewGhosttyTool(), tools.NewZshTool()} },
		detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
		detectManager:  func() pkg.PackageManager { return manager },
		collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			return snapshot, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func supportTestBuild() supportBuildInput {
	return supportBuildInput{ProductVersion: "2.1.2", GoVersion: "go1.26.1", GOOS: "darwin", GOARCH: "arm64", BuildInfoReadable: true, Settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "false"}}}
}

func supportTestDoctorReport() doctorReport {
	return doctorReport{
		SchemaVersion:     doctorSchemaVersion,
		RunningExecutable: doctorExecutable{OwnershipHint: "running executable; ownership not yet verified"},
		PATHMatches:       []doctorExecutable{{SameAsRunning: true}},
		Homebrew:          doctorHomebrew{Formula: doctorFormula},
		LegacyBinaries:    []doctorExecutable{}, Findings: []doctorFinding{},
	}
}

func supportTestDocument(t *testing.T) validatedSupportDocument {
	t.Helper()
	provenance, err := projectSupportProvenance(supportTestDoctorReport())
	if err != nil {
		t.Fatal(err)
	}
	operations, err := projectSupportOperations(operation.JournalSummarySet{Records: []operation.JournalSummary{}})
	if err != nil {
		t.Fatal(err)
	}
	document, err := newValidatedSupportDocument(supportProjectionSpec{Build: supportTestBuild(), Installation: collectedSupportInstallation(supportTestStatus(t)), Provenance: collectedSupportProvenance(provenance), Operations: collectedSupportOperations(operations)})
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func independentSupportDigest(t *testing.T, document supportDocument) string {
	t.Helper()
	canonical, err := json.Marshal(struct {
		SchemaVersion int                 `json:"schema_version"`
		Kind          string              `json:"kind"`
		Outcome       string              `json:"outcome"`
		Authority     struct{}            `json:"authority"`
		Build         supportBuild        `json:"build"`
		Installation  supportInstallation `json:"installation"`
		Provenance    supportProvenance   `json:"provenance"`
		Operations    supportOperations   `json:"operations"`
		Capabilities  supportCapabilities `json:"capabilities"`
	}{
		SchemaVersion: document.SchemaVersion, Kind: document.Kind, Outcome: document.Outcome,
		Build: document.Build, Installation: document.Installation, Provenance: document.Provenance,
		Operations: document.Operations, Capabilities: document.Capabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}
