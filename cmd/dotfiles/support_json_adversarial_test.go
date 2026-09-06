package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/operation"
)

func TestAdversarialSupportEmbeddedStatusRejectsCanonicalByteCorruption(t *testing.T) {
	document := supportTestDocument(t)
	original := bytes.Clone(document.value.Installation.Document.encoded)
	corruptions := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{name: "trailing whitespace", mutate: func(value []byte) []byte { return append(value, ' ') }},
		{name: "trailing object", mutate: func(value []byte) []byte { return append(value, []byte(`{}`)...) }},
		{name: "duplicate top key", mutate: func(value []byte) []byte {
			return []byte(strings.Replace(string(value), `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1))
		}},
		{name: "unknown top key", mutate: func(value []byte) []byte {
			return []byte(strings.Replace(string(value), `"kind":"dotfiles.status"`, `"kind":"dotfiles.status","private_path":"/Users/secret"`, 1))
		}},
		{name: "status digest", mutate: func(value []byte) []byte {
			copy := bytes.Clone(value)
			marker := []byte(`"digest":"`)
			index := bytes.Index(copy, marker)
			if index < 0 {
				t.Fatal("status digest marker missing")
			}
			if copy[index+len(marker)] == 'f' {
				copy[index+len(marker)] = 'e'
			} else {
				copy[index+len(marker)] = 'f'
			}
			return copy
		}},
	}
	for _, test := range corruptions {
		t.Run(test.name, func(t *testing.T) {
			candidate := validatedSupportDocument{value: cloneSupportDocument(document.value)}
			candidate.value.Installation.Document.encoded = test.mutate(bytes.Clone(original))
			encoded, err := marshalSupportDocument(candidate)
			if encoded != nil || !errors.Is(err, errSupportProjection) {
				t.Fatalf("corrupted status accepted: %q %v", encoded, err)
			}
		})
	}
}

func TestAdversarialSupportInstallationClonesAcceptedStatusOnce(t *testing.T) {
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
	before, err := marshalSupportDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	status.value.Tools[0].ToolID = "private-mutated-tool"
	status.value.Snapshot.Digest = strings.Repeat("f", 64)
	after, err := marshalSupportDocument(document)
	if err != nil || !bytes.Equal(before, after) || bytes.Contains(after, []byte("private-mutated-tool")) {
		t.Fatalf("accepted status aliases source: equal=%t error=%v", bytes.Equal(before, after), err)
	}
}

func TestAdversarialSupportBuildNormalizationBoundaries(t *testing.T) {
	for _, test := range []struct {
		value  string
		want   string
		source string
	}{
		{value: "dev", want: "dev", source: "development"},
		{value: "Dev", want: "unknown", source: "unknown"},
		{value: "1.2.99999", want: "1.2.99999", source: "compiled"},
		{value: "1.2.100000", want: "unknown", source: "unknown"},
		{value: "1.2.00000", want: "unknown", source: "unknown"},
		{value: "1.2.3+build", want: "unknown", source: "unknown"},
		{value: "1.2.3\n", want: "unknown", source: "unknown"},
	} {
		got, source := normalizeSupportVersion(test.value)
		if got != test.want || source != test.source {
			t.Fatalf("version %q = %q/%q, want %q/%q", test.value, got, source, test.want, test.source)
		}
	}

	go64 := "g" + strings.Repeat("a", 63)
	if got := normalizeSupportToken(go64, 64, false); got != go64 {
		t.Fatalf("64-byte Go token = %q", got)
	}
	for _, value := range []string{"." + strings.Repeat("a", 10), "go_1", strings.Repeat("a", 65), "go1\u202e", "go1\n"} {
		if got := normalizeSupportToken(value, 64, false); got != "unknown" {
			t.Fatalf("unsafe Go token %q = %q", value, got)
		}
	}
	os32 := strings.Repeat("a", 32)
	if got := normalizeSupportToken(os32, 32, true); got != os32 {
		t.Fatalf("32-byte OS token = %q", got)
	}
	for _, value := range []string{strings.Repeat("a", 33), "Darwin", "arm-64", "darwin\n"} {
		if got := normalizeSupportToken(value, 32, true); got != "unknown" {
			t.Fatalf("unsafe OS token %q = %q", value, got)
		}
	}

	settings := []debug.BuildSetting{{Key: "vcs.modified", Value: "true"}, {Key: "vcs.modified", Value: "false"}, {Key: "vcs.revision", Value: "PRIVATE"}}
	if got := normalizeSupportVCS(true, settings); got != "unavailable" {
		t.Fatalf("duplicate VCS state = %q", got)
	}
}

func TestAdversarialSupportProvenanceEntryAndFindingLimits(t *testing.T) {
	t.Run("1024 entries accepted", func(t *testing.T) {
		report := supportTestDoctorReport()
		report.PATHMatches = make([]doctorExecutable, 1024)
		for index := range report.PATHMatches {
			report.PATHMatches[index].SameAsRunning = true
		}
		report.Findings = []doctorFinding{{Severity: "warning", Code: "multiple-path-matches"}}
		projection, err := projectSupportProvenance(report)
		if err != nil || projection.PATHMatchCount != 1024 || projection.FindingCount != 1 {
			t.Fatalf("1024 projection = %+v, %v", projection, err)
		}
	})

	t.Run("1025 limit precedes invalid finding", func(t *testing.T) {
		report := supportTestDoctorReport()
		report.PATHMatches = make([]doctorExecutable, 1025)
		report.Findings = []doctorFinding{{Severity: "secret", Code: "private"}}
		if _, err := projectSupportProvenance(report); !errors.Is(err, errSupportProvenanceLimit) {
			t.Fatalf("1025 precedence error = %v", err)
		}
	})

	for _, count := range []int{256, 257} {
		t.Run(strings.Repeat("x", count/256+1), func(t *testing.T) {
			report := adversarialSupportLegacyReport(count)
			projection, err := projectSupportProvenance(report)
			if err != nil || projection.FindingCount != count || len(projection.Findings) != min(count, 256) || projection.Truncated != (count > 256) {
				t.Fatalf("%d findings = %+v, %v", count, projection, err)
			}
		})
	}

	t.Run("invalid finding after publication boundary", func(t *testing.T) {
		report := adversarialSupportLegacyReport(257)
		report.Findings[256].Code = "private-invalid"
		if _, err := projectSupportProvenance(report); !errors.Is(err, errSupportProvenanceInvalid) {
			t.Fatalf("invalid 257th finding error = %v", err)
		}
	})

	t.Run("stable public sorting", func(t *testing.T) {
		report := adversarialSupportLegacyReport(1)
		report.Homebrew.ProbeError = "/Users/private brew token=SECRET"
		report.Findings = append(report.Findings, doctorFinding{Severity: "info", Code: "homebrew-unavailable", Summary: "PRIVATE"})
		projection, err := projectSupportProvenance(report)
		if err != nil || len(projection.Findings) != 2 || projection.Findings[0] != (supportFinding{Severity: "info", Code: "homebrew-unavailable"}) || projection.Findings[1].Code != "legacy-binary" {
			t.Fatalf("sorted findings = %+v, %v", projection.Findings, err)
		}
	})
}

func TestAdversarialSupportRejectsContradictoryCollectedProvenance(t *testing.T) {
	status := supportTestStatus(t)
	projection := supportProvenanceData{
		RunningOwnership: "homebrew", HomebrewInstalled: false,
		Findings: []supportFinding{},
	}
	_, err := newValidatedSupportDocument(supportProjectionSpec{
		Build: supportTestBuild(), Installation: collectedSupportInstallation(status),
		Provenance: collectedSupportProvenance(projection), Operations: notPresentSupportOperations(),
	})
	if !errors.Is(err, errSupportProjection) {
		t.Fatalf("homebrew ownership without installation accepted: %v", err)
	}
}

func TestAdversarialSupportOperationsOverflowAndAliasBoundaries(t *testing.T) {
	maximum := int(^uint(0) >> 1)
	overflow := operation.JournalSummarySet{Records: []operation.JournalSummary{{
		Ordinal: 0, Status: operation.StatusRunning,
		Actions: operation.JournalActionCounts{Pending: maximum, Succeeded: 1},
	}}}
	if _, err := projectSupportOperations(overflow); !errors.Is(err, errSupportProjection) {
		t.Fatalf("overflowing action counts accepted: %v", err)
	}
	validMaximum := overflow
	validMaximum.Records = append([]operation.JournalSummary(nil), overflow.Records...)
	validMaximum.Records[0].Actions.Succeeded = 0
	if _, err := projectSupportOperations(validMaximum); err != nil {
		t.Fatalf("representable maximum count rejected: %v", err)
	}

	duration := int64(1)
	rollback := &operation.JournalRollbackSummary{Status: operation.RollbackSucceeded, Restored: 1}
	set := operation.JournalSummarySet{Records: []operation.JournalSummary{{
		Ordinal: 0, Status: operation.StatusSucceeded, DurationMilliseconds: &duration, Rollback: rollback,
	}}}
	projected, err := projectSupportOperations(set)
	if err != nil {
		t.Fatal(err)
	}
	document, err := newValidatedSupportDocument(supportProjectionSpec{
		Build: supportTestBuild(), Installation: collectedSupportInstallation(supportTestStatus(t)),
		Provenance: unavailableSupportProvenance(supportReasonProvenanceUnavailable), Operations: collectedSupportOperations(projected),
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := marshalSupportDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	projected.Records[0].Status = operation.StatusRunning
	projected.Records[0].Rollback.Status = operation.RollbackFailed
	*projected.Records[0].DurationMilliseconds = 999
	after, err := marshalSupportDocument(document)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("support document aliases operations source: equal=%t error=%v", bytes.Equal(before, after), err)
	}
}

func TestAdversarialSupportUnavailableShapesAndForbiddenKeys(t *testing.T) {
	document, err := newValidatedSupportDocument(supportProjectionSpec{
		Build:        supportTestBuild(),
		Installation: unavailableSupportInstallation(supportReasonCancelled),
		Provenance:   unavailableSupportProvenance(supportReasonProvenanceLimit),
		Operations:   unavailableSupportOperations(supportReasonJournalUnavailable),
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := marshalSupportDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if document.value.Outcome != supportPartial || document.value.Installation.Document != nil || document.value.Provenance.Findings == nil || len(document.value.Provenance.Findings) != 0 || document.value.Operations.Records == nil || len(document.value.Operations.Records) != 0 {
		t.Fatalf("unavailable shapes = %+v", document.value)
	}
	for _, forbidden := range []string{"operation_id", "plan_hash", "started_at", "finished_at", "backup_path", "private_path", "vcs_revision", "generated_at"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("forbidden key %q in %s", forbidden, encoded)
		}
	}
}

func TestAdversarialSupportExactOneMiBFraming(t *testing.T) {
	exact := adversarialSupportDocumentWithObjectSize(t, supportLineLimit-1)
	line, err := marshalSupportDocumentLine(exact)
	if err != nil || len(line) != supportLineLimit || line[len(line)-1] != '\n' {
		t.Fatalf("exact 1 MiB line = %d bytes, %v", len(line), err)
	}

	over := adversarialSupportDocumentWithObjectSize(t, supportLineLimit)
	line, err = marshalSupportDocumentLine(over)
	if line != nil || !errors.Is(err, errSupportProjection) {
		t.Fatalf("oversize support line = %d bytes, %v", len(line), err)
	}
}

func TestAdversarialSupportExhaustiveSectionStateMatrix(t *testing.T) {
	status := supportTestStatus(t)
	provenance, err := projectSupportProvenance(supportTestDoctorReport())
	if err != nil {
		t.Fatal(err)
	}
	operations, err := projectSupportOperations(operation.JournalSummarySet{Records: []operation.JournalSummary{}})
	if err != nil {
		t.Fatal(err)
	}
	installations := []supportInstallationInput{
		collectedSupportInstallation(status),
		unavailableSupportInstallation(supportReasonCancelled),
		unavailableSupportInstallation(supportReasonStatusUnavailable),
	}
	provenances := []supportProvenanceInput{
		collectedSupportProvenance(provenance),
		unavailableSupportProvenance(supportReasonCancelled),
		unavailableSupportProvenance(supportReasonProvenanceUnavailable),
		unavailableSupportProvenance(supportReasonProvenanceInvalid),
		unavailableSupportProvenance(supportReasonProvenanceLimit),
	}
	operationSections := []supportOperationsInput{
		collectedSupportOperations(operations),
		notPresentSupportOperations(),
		unavailableSupportOperations(supportReasonCancelled),
		unavailableSupportOperations(supportReasonJournalUnavailable),
		unavailableSupportOperations(supportReasonJournalInvalid),
		unavailableSupportOperations(supportReasonJournalLimit),
	}

	for installationIndex, installation := range installations {
		for provenanceIndex, provenanceInput := range provenances {
			for operationsIndex, operationsInput := range operationSections {
				document, err := newValidatedSupportDocument(supportProjectionSpec{
					Build: supportTestBuild(), Installation: installation,
					Provenance: provenanceInput, Operations: operationsInput,
				})
				if err != nil {
					t.Fatalf("state matrix %d/%d/%d: %v", installationIndex, provenanceIndex, operationsIndex, err)
				}
				wantOutcome := supportPartial
				if installation.collection == supportCollected && provenanceInput.collection == supportCollected &&
					(operationsInput.collection == supportCollected || operationsInput.collection == supportNotPresent) {
					wantOutcome = supportComplete
				}
				if document.value.Outcome != wantOutcome ||
					document.value.Capabilities.Installation != installation.collection ||
					document.value.Capabilities.Provenance != provenanceInput.collection ||
					document.value.Capabilities.Operations != operationsInput.collection {
					t.Fatalf("state matrix %d/%d/%d derived %+v", installationIndex, provenanceIndex, operationsIndex, document.value)
				}
				if _, err := marshalSupportDocumentLine(document); err != nil {
					t.Fatalf("state matrix %d/%d/%d marshal: %v", installationIndex, provenanceIndex, operationsIndex, err)
				}
			}
		}
	}
}

func TestAdversarialSupportHomebrewConsistencyMatrix(t *testing.T) {
	validInstalled := doctorHomebrew{
		Formula: doctorFormula, Installed: true,
		Prefix:             "/opt/homebrew/opt/dotfiles",
		ManagedExecutable:  "/opt/homebrew/opt/dotfiles/bin/dotfiles",
		ResolvedExecutable: "/opt/homebrew/Cellar/dotfiles/2.1.2/bin/dotfiles",
		VersionHint:        "2.1.2",
	}
	validReport := supportTestDoctorReport()
	validReport.RunningExecutable.OwnershipHint = "Homebrew formula " + doctorFormula
	validReport.Homebrew = validInstalled
	if _, err := projectSupportProvenance(validReport); err != nil {
		t.Fatalf("valid installed Homebrew rejected: %v", err)
	}

	invalidInstalled := []doctorHomebrew{
		func() doctorHomebrew { value := validInstalled; value.Formula = "private/tap/dotfiles"; return value }(),
		func() doctorHomebrew { value := validInstalled; value.ProbeError = "private error"; return value }(),
		func() doctorHomebrew { value := validInstalled; value.Prefix = "relative/prefix"; return value }(),
		func() doctorHomebrew {
			value := validInstalled
			value.Prefix = "/opt/homebrew/../private"
			return value
		}(),
		func() doctorHomebrew {
			value := validInstalled
			value.ManagedExecutable = "/private/bin/dotfiles"
			return value
		}(),
		func() doctorHomebrew {
			value := validInstalled
			value.ResolvedExecutable = "relative/dotfiles"
			return value
		}(),
		func() doctorHomebrew {
			value := validInstalled
			value.ResolvedExecutable = "/private/bin/not-dotfiles"
			return value
		}(),
		func() doctorHomebrew {
			value := validInstalled
			value.ResolvedExecutable = "/private/../bin/dotfiles"
			return value
		}(),
		func() doctorHomebrew { value := validInstalled; value.ResolvedExecutable = ""; return value }(),
	}
	for index, homebrew := range invalidInstalled {
		report := validReport
		report.Homebrew = homebrew
		if _, err := projectSupportProvenance(report); !errors.Is(err, errSupportProvenanceInvalid) {
			t.Fatalf("invalid installed Homebrew %d accepted: %+v, %v", index, homebrew, err)
		}
	}

	for index, mutate := range []func(*doctorHomebrew){
		func(value *doctorHomebrew) { value.Prefix = "/private" },
		func(value *doctorHomebrew) { value.ManagedExecutable = "/private/bin/dotfiles" },
		func(value *doctorHomebrew) { value.ResolvedExecutable = "/private/bin/dotfiles" },
		func(value *doctorHomebrew) { value.VersionHint = "private-version" },
	} {
		report := supportTestDoctorReport()
		mutate(&report.Homebrew)
		if _, err := projectSupportProvenance(report); !errors.Is(err, errSupportProvenanceInvalid) {
			t.Fatalf("absent Homebrew evidence %d accepted: %+v, %v", index, report.Homebrew, err)
		}
	}

	homebrewOwnershipWithoutInstall := supportTestDoctorReport()
	homebrewOwnershipWithoutInstall.RunningExecutable.OwnershipHint = "Homebrew formula " + doctorFormula
	if _, err := projectSupportProvenance(homebrewOwnershipWithoutInstall); !errors.Is(err, errSupportProvenanceInvalid) {
		t.Fatalf("Homebrew ownership without installation accepted: %v", err)
	}
}

func TestAdversarialSupportOperationInvariantMatrix(t *testing.T) {
	duration := int64(0)
	validRecords := make([]operation.JournalSummary, 20)
	for index := range validRecords {
		validRecords[index] = operation.JournalSummary{Ordinal: index, Status: operation.StatusSucceeded, DurationMilliseconds: &duration}
	}
	for _, truncated := range []bool{false, true} {
		if projection, err := projectSupportOperations(operation.JournalSummarySet{Truncated: truncated, Records: validRecords}); err != nil || len(projection.Records) != 20 {
			t.Fatalf("20 records truncated=%t rejected: %+v, %v", truncated, projection, err)
		}
	}
	if _, err := projectSupportOperations(operation.JournalSummarySet{Truncated: true, Records: validRecords[:19]}); !errors.Is(err, errSupportProjection) {
		t.Fatalf("truncated 19 records accepted: %v", err)
	}
	over := append(append([]operation.JournalSummary(nil), validRecords...), operation.JournalSummary{Ordinal: 20, Status: operation.StatusSucceeded, DurationMilliseconds: &duration})
	if _, err := projectSupportOperations(operation.JournalSummarySet{Truncated: true, Records: over}); !errors.Is(err, errSupportProjection) {
		t.Fatalf("21 records accepted: %v", err)
	}

	negative := int64(-1)
	invalid := []operation.JournalSummary{
		{Ordinal: 1, Status: operation.StatusRunning},
		{Ordinal: 0, Status: operation.Status("unknown")},
		{Ordinal: 0, Status: operation.StatusRunning, DurationMilliseconds: &duration},
		{Ordinal: 0, Status: operation.StatusSucceeded},
		{Ordinal: 0, Status: operation.StatusCancelled, DurationMilliseconds: &negative},
		{Ordinal: 0, Status: operation.StatusRunning, Actions: operation.JournalActionCounts{Pending: -1}},
		{Ordinal: 0, Status: operation.StatusRunning, Rollback: &operation.JournalRollbackSummary{Status: operation.RollbackStatus("unknown")}},
		{Ordinal: 0, Status: operation.StatusRunning, Rollback: &operation.JournalRollbackSummary{Status: operation.RollbackSucceeded, Warnings: -1}},
	}
	for index, record := range invalid {
		if _, err := projectSupportOperations(operation.JournalSummarySet{Records: []operation.JournalSummary{record}}); !errors.Is(err, errSupportProjection) {
			t.Fatalf("invalid operation %d accepted: %+v, %v", index, record, err)
		}
	}
}

func TestAdversarialSupportValidatorRejectsRedigestedCorruption(t *testing.T) {
	document := supportTestDocument(t)
	corruptions := []struct {
		name   string
		mutate func(*supportDocument)
	}{
		{name: "build collection", mutate: func(value *supportDocument) { value.Build.Collection = supportUnavailable }},
		{name: "version source", mutate: func(value *supportDocument) { value.Build.VersionSource = "private" }},
		{name: "ownership", mutate: func(value *supportDocument) { value.Provenance.RunningOwnership = "private" }},
		{name: "finding pair", mutate: func(value *supportDocument) {
			value.Provenance.FindingCount = 1
			value.Provenance.Findings = []supportFinding{{Severity: "info", Code: "path-missing"}}
		}},
		{name: "operation status", mutate: func(value *supportDocument) {
			value.Operations.Records = []supportOperationRecord{{Ordinal: 0, Status: operation.Status("private")}}
		}},
		{name: "operation ordinal", mutate: func(value *supportDocument) {
			value.Operations.Records = []supportOperationRecord{{Ordinal: 1, Status: operation.StatusRunning}}
		}},
		{name: "capability", mutate: func(value *supportDocument) { value.Capabilities.Config = supportCollected }},
		{name: "outcome", mutate: func(value *supportDocument) { value.Outcome = supportPartial }},
	}
	for _, test := range corruptions {
		t.Run(test.name, func(t *testing.T) {
			candidate := validatedSupportDocument{value: cloneSupportDocument(document.value)}
			test.mutate(&candidate.value)
			digest, err := supportPublicDigest(candidate.value)
			if err != nil {
				t.Fatal(err)
			}
			candidate.value.Authority.PublicDigest = digest
			if encoded, err := marshalSupportDocument(candidate); encoded != nil || !errors.Is(err, errSupportProjection) {
				t.Fatalf("redigested corruption accepted: %s, %v", encoded, err)
			}
		})
	}
}

func TestAdversarialSupportPrivateBuildAndDoctorInputsDoNotAlterBytes(t *testing.T) {
	status := supportTestStatus(t)
	buildA := supportTestBuild()
	buildA.Settings = append(buildA.Settings,
		debug.BuildSetting{Key: "vcs.revision", Value: "SECRET-A"},
		debug.BuildSetting{Key: "vcs.time", Value: "PRIVATE-TIME-A"},
	)
	buildB := supportTestBuild()
	buildB.Settings = append(buildB.Settings,
		debug.BuildSetting{Key: "vcs.revision", Value: "SECRET-B"},
		debug.BuildSetting{Key: "vcs.time", Value: "PRIVATE-TIME-B"},
		debug.BuildSetting{Key: "private.setting", Value: "/Users/private/token=SECRET"},
	)

	reportA := supportTestDoctorReport()
	reportA.RunningExecutable.Path = "/Users/alice/private/dotfiles"
	reportA.RunningExecutable.ResolvedPath = "/private/A"
	reportA.RunningExecutable.SymlinkTarget = "/private/target/A"
	reportA.RunningExecutable.InspectionError = "token=SECRET-A"
	reportA.PATHMatches[0].Path = "/Users/alice/bin/dotfiles"
	reportA.Homebrew.BrewExecutable = "/private/brew-A"
	reportA.Homebrew.ProbeError = "dial private A"
	reportA.Findings = []doctorFinding{{Severity: "info", Code: "homebrew-unavailable", Summary: "private A", Path: "/Users/alice"}}
	reportB := supportTestDoctorReport()
	reportB.RunningExecutable.Path = "C:\\Users\\bob\\private\\dotfiles.exe"
	reportB.RunningExecutable.ResolvedPath = `\\server\private\B`
	reportB.RunningExecutable.SymlinkTarget = "/private/target/B"
	reportB.RunningExecutable.InspectionError = "authorization: Bearer SECRET-B"
	reportB.PATHMatches[0].Path = "/home/bob/bin/dotfiles"
	reportB.Homebrew.BrewExecutable = "/private/brew-B"
	reportB.Homebrew.ProbeError = "dial private B"
	reportB.Findings = []doctorFinding{{Severity: "info", Code: "homebrew-unavailable", Summary: "private B", Path: "/home/bob"}}

	projectedA, err := projectSupportProvenance(reportA)
	if err != nil {
		t.Fatal(err)
	}
	projectedB, err := projectSupportProvenance(reportB)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := projectSupportOperations(operation.JournalSummarySet{Records: []operation.JournalSummary{}})
	if err != nil {
		t.Fatal(err)
	}
	makeDocument := func(build supportBuildInput, provenance supportProvenanceData) []byte {
		document, err := newValidatedSupportDocument(supportProjectionSpec{
			Build: build, Installation: collectedSupportInstallation(status),
			Provenance: collectedSupportProvenance(provenance), Operations: collectedSupportOperations(operations),
		})
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := marshalSupportDocument(document)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	encodedA := makeDocument(buildA, projectedA)
	encodedB := makeDocument(buildB, projectedB)
	if !bytes.Equal(encodedA, encodedB) {
		t.Fatalf("private-only inputs altered support bytes:\nA=%s\nB=%s", encodedA, encodedB)
	}
	for _, canary := range []string{"SECRET", "/Users", "/home/bob", "C:\\\\Users", "\\\\server", "Bearer", "private.setting", "vcs.revision", "vcs.time"} {
		if bytes.Contains(encodedA, []byte(canary)) || bytes.Contains(encodedB, []byte(canary)) {
			t.Fatalf("private canary %q leaked", canary)
		}
	}
}

func adversarialSupportLegacyReport(count int) doctorReport {
	report := supportTestDoctorReport()
	report.LegacyBinaries = make([]doctorExecutable, count)
	report.Findings = make([]doctorFinding, count)
	for index := range report.Findings {
		report.LegacyBinaries[index].Path = "/private/legacy"
		report.Findings[index] = doctorFinding{Severity: "warning", Code: "legacy-binary", Summary: "PRIVATE", Path: "/private/legacy"}
	}
	return report
}

func adversarialSupportDocumentWithObjectSize(t *testing.T, target int) validatedSupportDocument {
	t.Helper()
	build := func(padding int) validatedSupportDocument {
		status := supportTestStatus(t)
		status.value.Tools[0].Package.Diagnostic.Summary = strings.Repeat("x", padding)
		digest, err := statusPublicDigest(status.value)
		if err != nil {
			t.Fatal(err)
		}
		status.value.Snapshot.Digest = digest
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
		return document
	}
	base := build(0)
	encoded, err := marshalSupportDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	padding := target - len(encoded)
	if padding < 0 {
		t.Fatalf("target %d smaller than base document %d", target, len(encoded))
	}
	document := build(padding)
	encoded, err = marshalSupportDocument(document)
	if err != nil || len(encoded) != target {
		t.Fatalf("padded support object = %d bytes, want %d: %v", len(encoded), target, err)
	}
	return document
}

func TestAdversarialSupportJSONEnvelopeHasExactTopLevelOrder(t *testing.T) {
	encoded, err := marshalSupportDocument(supportTestDocument(t))
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil || len(object) != 9 {
		t.Fatalf("top-level envelope = %v, %v", object, err)
	}
	previous := -1
	for _, key := range []string{"schema_version", "kind", "outcome", "authority", "build", "installation", "provenance", "operations", "capabilities"} {
		relative := bytes.Index(encoded[previous+1:], []byte(`"`+key+`":`))
		if relative < 0 {
			t.Fatalf("key %q missing from %s", key, encoded)
		}
		index := previous + 1 + relative
		if index <= previous {
			t.Fatalf("key %q out of order in %s", key, encoded)
		}
		previous = index
	}
}
