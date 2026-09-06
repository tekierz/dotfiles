package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/tekierz/dotfiles/internal/operation"
)

const (
	supportSchemaVersion = 1
	supportKind          = "dotfiles.support"
	supportLineLimit     = 1 << 20

	supportCollected    = "collected"
	supportUnavailable  = "unavailable"
	supportNotPresent   = "not_present"
	supportNotCollected = "not_collected"

	supportComplete = "complete"
	supportPartial  = "partial"

	supportReasonCancelled             = "collection_cancelled"
	supportReasonStatusUnavailable     = "status_unavailable"
	supportReasonProvenanceUnavailable = "provenance_unavailable"
	supportReasonProvenanceInvalid     = "provenance_invalid"
	supportReasonProvenanceLimit       = "provenance_limit_exceeded"
	supportReasonJournalUnavailable    = "journal_unavailable"
	supportReasonJournalInvalid        = "journal_invalid"
	supportReasonJournalLimit          = "journal_limit_exceeded"
)

var (
	errSupportProjection        = errors.New("support projection failed")
	errSupportProvenanceInvalid = errors.New("support provenance is invalid")
	errSupportProvenanceLimit   = errors.New("support provenance limit exceeded")
)

type supportDocument struct {
	SchemaVersion int                 `json:"schema_version"`
	Kind          string              `json:"kind"`
	Outcome       string              `json:"outcome"`
	Authority     supportAuthority    `json:"authority"`
	Build         supportBuild        `json:"build"`
	Installation  supportInstallation `json:"installation"`
	Provenance    supportProvenance   `json:"provenance"`
	Operations    supportOperations   `json:"operations"`
	Capabilities  supportCapabilities `json:"capabilities"`
}

type supportAuthority struct {
	PublicDigest string `json:"public_digest"`
}

type supportBuild struct {
	Collection    string `json:"collection"`
	Version       string `json:"version"`
	VersionSource string `json:"version_source"`
	GoVersion     string `json:"go_version"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	VCSState      string `json:"vcs_state"`
}

type supportInstallation struct {
	Collection string                 `json:"collection"`
	ReasonCode string                 `json:"reason_code"`
	Document   *supportEmbeddedStatus `json:"document"`
}

type supportEmbeddedStatus struct {
	encoded []byte
}

func (status supportEmbeddedStatus) MarshalJSON() ([]byte, error) {
	return validateEmbeddedSupportStatus(status.encoded)
}

type supportProvenance struct {
	Collection        string           `json:"collection"`
	ReasonCode        string           `json:"reason_code"`
	RunningOwnership  string           `json:"running_ownership"`
	PATHMatchCount    int              `json:"path_match_count"`
	LegacyBinaryCount int              `json:"legacy_binary_count"`
	HomebrewInstalled bool             `json:"homebrew_installed"`
	FindingCount      int              `json:"finding_count"`
	Truncated         bool             `json:"truncated"`
	Findings          []supportFinding `json:"findings"`
}

type supportFinding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
}

type supportOperations struct {
	Collection string                   `json:"collection"`
	ReasonCode string                   `json:"reason_code"`
	Truncated  bool                     `json:"truncated"`
	Records    []supportOperationRecord `json:"records"`
}

type supportOperationRecord struct {
	Ordinal              int                 `json:"ordinal"`
	Status               operation.Status    `json:"status"`
	Actions              supportActionCounts `json:"actions"`
	BackupRecorded       bool                `json:"backup_recorded"`
	Rollback             *supportRollback    `json:"rollback"`
	DurationMilliseconds *int64              `json:"duration_ms"`
}

type supportActionCounts struct {
	Pending   int `json:"pending"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

type supportRollback struct {
	Status   operation.RollbackStatus `json:"status"`
	Restored int                      `json:"restored"`
	Removed  int                      `json:"removed"`
	Skipped  int                      `json:"skipped"`
	Warnings int                      `json:"warnings"`
}

type supportCapabilities struct {
	Build        string `json:"build"`
	Installation string `json:"installation"`
	Provenance   string `json:"provenance"`
	Operations   string `json:"operations"`
	Config       string `json:"config"`
	Service      string `json:"service"`
	Auth         string `json:"auth"`
}

type validatedSupportDocument struct{ value supportDocument }

func (document validatedSupportDocument) MarshalJSON() ([]byte, error) {
	return marshalSupportDocument(document)
}

type supportProjectionSpec struct {
	Build        supportBuildInput
	Installation supportInstallationInput
	Provenance   supportProvenanceInput
	Operations   supportOperationsInput
}

type supportBuildInput struct {
	ProductVersion    string
	GoVersion         string
	GOOS              string
	GOARCH            string
	BuildInfoReadable bool
	Settings          []debug.BuildSetting
}

type supportInstallationInput struct {
	collection string
	reason     string
	document   *validatedStatusDocument
}

type supportProvenanceData struct {
	RunningOwnership  string
	PATHMatchCount    int
	LegacyBinaryCount int
	HomebrewInstalled bool
	FindingCount      int
	Truncated         bool
	Findings          []supportFinding
}

type supportProvenanceInput struct {
	collection string
	reason     string
	data       supportProvenanceData
}

type supportOperationsData struct {
	Truncated bool
	Records   []supportOperationRecord
}

type supportOperationsInput struct {
	collection string
	reason     string
	data       supportOperationsData
}

func collectedSupportInstallation(document validatedStatusDocument) supportInstallationInput {
	return supportInstallationInput{collection: supportCollected, document: &document}
}

func unavailableSupportInstallation(reason string) supportInstallationInput {
	return supportInstallationInput{collection: supportUnavailable, reason: reason}
}

func collectedSupportProvenance(data supportProvenanceData) supportProvenanceInput {
	return supportProvenanceInput{collection: supportCollected, data: data}
}

func unavailableSupportProvenance(reason string) supportProvenanceInput {
	return supportProvenanceInput{collection: supportUnavailable, reason: reason}
}

func collectedSupportOperations(data supportOperationsData) supportOperationsInput {
	return supportOperationsInput{collection: supportCollected, data: data}
}

func notPresentSupportOperations() supportOperationsInput {
	return supportOperationsInput{collection: supportNotPresent}
}

func unavailableSupportOperations(reason string) supportOperationsInput {
	return supportOperationsInput{collection: supportUnavailable, reason: reason}
}

func newValidatedSupportDocument(spec supportProjectionSpec) (validatedSupportDocument, error) {
	return newValidatedSupportDocumentFromBuild(normalizeSupportBuild(spec.Build), spec.Installation, spec.Provenance, spec.Operations)
}

func newValidatedSupportDocumentFromBuild(build supportBuild, installationInput supportInstallationInput, provenanceInput supportProvenanceInput, operationsInput supportOperationsInput) (validatedSupportDocument, error) {
	if validateSupportBuild(build) != nil {
		return validatedSupportDocument{}, errSupportProjection
	}
	installation, err := buildSupportInstallation(installationInput)
	if err != nil {
		return validatedSupportDocument{}, errSupportProjection
	}
	provenance, err := buildSupportProvenance(provenanceInput)
	if err != nil {
		return validatedSupportDocument{}, errSupportProjection
	}
	operations, err := buildSupportOperations(operationsInput)
	if err != nil {
		return validatedSupportDocument{}, errSupportProjection
	}
	document := supportDocument{
		SchemaVersion: supportSchemaVersion,
		Kind:          supportKind,
		Build:         build,
		Installation:  installation,
		Provenance:    provenance,
		Operations:    operations,
	}
	document.Outcome = supportComplete
	if installation.Collection != supportCollected || provenance.Collection != supportCollected ||
		(operations.Collection != supportCollected && operations.Collection != supportNotPresent) {
		document.Outcome = supportPartial
	}
	document.Capabilities = supportCapabilities{
		Build: supportCollected, Installation: installation.Collection, Provenance: provenance.Collection,
		Operations: operations.Collection, Config: supportNotCollected, Service: supportNotCollected, Auth: supportNotCollected,
	}
	digest, err := supportPublicDigest(document)
	if err != nil {
		return validatedSupportDocument{}, errSupportProjection
	}
	document.Authority.PublicDigest = digest
	result := validatedSupportDocument{value: cloneSupportDocument(document)}
	if _, err := marshalSupportDocument(result); err != nil {
		return validatedSupportDocument{}, errSupportProjection
	}
	return result, nil
}

func marshalSupportDocument(document validatedSupportDocument) ([]byte, error) {
	value := cloneSupportDocument(document.value)
	if err := validateSupportDocument(value); err != nil {
		return nil, errSupportProjection
	}
	digest, err := supportPublicDigest(value)
	if err != nil || digest != value.Authority.PublicDigest {
		return nil, errSupportProjection
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, errSupportProjection
	}
	return encoded, nil
}

func marshalSupportDocumentLine(document validatedSupportDocument) ([]byte, error) {
	encoded, err := marshalSupportDocument(document)
	if err != nil {
		return nil, errSupportProjection
	}
	return enforceSupportLineLimit(encoded, supportLineLimit)
}

func enforceSupportLineLimit(encoded []byte, limit int) ([]byte, error) {
	if limit <= 0 || len(encoded) >= limit {
		return nil, errSupportProjection
	}
	line := make([]byte, len(encoded)+1)
	copy(line, encoded)
	line[len(encoded)] = '\n'
	if len(line) > limit {
		return nil, errSupportProjection
	}
	return line, nil
}

func buildSupportInstallation(input supportInstallationInput) (supportInstallation, error) {
	if input.collection == supportCollected {
		if input.reason != "" || input.document == nil {
			return supportInstallation{}, errSupportProjection
		}
		encoded, err := input.document.MarshalJSON()
		if err != nil {
			return supportInstallation{}, errSupportProjection
		}
		validated, err := validateEmbeddedSupportStatus(encoded)
		if err != nil {
			return supportInstallation{}, errSupportProjection
		}
		return supportInstallation{Collection: supportCollected, Document: &supportEmbeddedStatus{encoded: bytes.Clone(validated)}}, nil
	}
	if input.collection != supportUnavailable || !allowedInstallationReason(input.reason) || input.document != nil {
		return supportInstallation{}, errSupportProjection
	}
	return supportInstallation{Collection: supportUnavailable, ReasonCode: input.reason}, nil
}

func buildSupportProvenance(input supportProvenanceInput) (supportProvenance, error) {
	if input.collection == supportUnavailable {
		if !allowedProvenanceReason(input.reason) || !zeroSupportProvenanceData(input.data) {
			return supportProvenance{}, errSupportProjection
		}
		return supportProvenance{Collection: supportUnavailable, ReasonCode: input.reason, Findings: []supportFinding{}}, nil
	}
	if input.collection != supportCollected || input.reason != "" || validateSupportProvenanceData(input.data) != nil {
		return supportProvenance{}, errSupportProjection
	}
	data := input.data
	return supportProvenance{
		Collection: supportCollected, RunningOwnership: data.RunningOwnership, PATHMatchCount: data.PATHMatchCount,
		LegacyBinaryCount: data.LegacyBinaryCount, HomebrewInstalled: data.HomebrewInstalled,
		FindingCount: data.FindingCount, Truncated: data.Truncated, Findings: cloneSupportFindings(data.Findings),
	}, nil
}

func buildSupportOperations(input supportOperationsInput) (supportOperations, error) {
	switch input.collection {
	case supportNotPresent:
		if input.reason != "" || !zeroSupportOperationsData(input.data) {
			return supportOperations{}, errSupportProjection
		}
		return supportOperations{Collection: supportNotPresent, Records: []supportOperationRecord{}}, nil
	case supportUnavailable:
		if !allowedOperationsReason(input.reason) || !zeroSupportOperationsData(input.data) {
			return supportOperations{}, errSupportProjection
		}
		return supportOperations{Collection: supportUnavailable, ReasonCode: input.reason, Records: []supportOperationRecord{}}, nil
	case supportCollected:
		if input.reason != "" || validateSupportOperationsData(input.data) != nil {
			return supportOperations{}, errSupportProjection
		}
		return supportOperations{Collection: supportCollected, Truncated: input.data.Truncated, Records: cloneSupportRecords(input.data.Records)}, nil
	default:
		return supportOperations{}, errSupportProjection
	}
}

func validateEmbeddedSupportStatus(encoded []byte) ([]byte, error) {
	if len(encoded) == 0 || bytes.Contains(encoded, []byte{'\n'}) {
		return nil, errSupportProjection
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var value statusDocument
	if err := decoder.Decode(&value); err != nil {
		return nil, errSupportProjection
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errSupportProjection
	}
	reencoded, err := marshalStatusDocument(validatedStatusDocument{value: value})
	if err != nil || !bytes.Equal(reencoded, encoded) {
		return nil, errSupportProjection
	}
	return bytes.Clone(encoded), nil
}

func normalizeSupportBuild(input supportBuildInput) supportBuild {
	version, source := normalizeSupportVersion(input.ProductVersion)
	return supportBuild{
		Collection: supportCollected, Version: version, VersionSource: source,
		GoVersion: normalizeSupportToken(input.GoVersion, 64, false),
		OS:        normalizeSupportToken(input.GOOS, 32, true), Arch: normalizeSupportToken(input.GOARCH, 32, true),
		VCSState: normalizeSupportVCS(input.BuildInfoReadable, input.Settings),
	}
}

func normalizeSupportVersion(value string) (string, string) {
	if value == "dev" {
		return "dev", "development"
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return "unknown", "unknown"
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 5 || (len(part) > 1 && part[0] == '0') {
			return "unknown", "unknown"
		}
		for _, character := range []byte(part) {
			if character < '0' || character > '9' {
				return "unknown", "unknown"
			}
		}
	}
	return value, "compiled"
}

func normalizeSupportToken(value string, limit int, lowerOnly bool) string {
	if len(value) == 0 || len(value) > limit {
		return "unknown"
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if lowerOnly {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
				return "unknown"
			}
			continue
		}
		alphanumeric := (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9')
		if !alphanumeric && (index == 0 || (character != '.' && character != '+' && character != '-')) {
			return "unknown"
		}
	}
	return value
}

func normalizeSupportVCS(readable bool, settings []debug.BuildSetting) string {
	if !readable {
		return "unavailable"
	}
	found := false
	value := ""
	for _, setting := range settings {
		if setting.Key != "vcs.modified" {
			continue
		}
		if found {
			return "unavailable"
		}
		found, value = true, setting.Value
	}
	if !found {
		return "unavailable"
	}
	switch value {
	case "true":
		return "modified"
	case "false":
		return "clean"
	default:
		return "unavailable"
	}
}

func projectSupportProvenance(report doctorReport) (supportProvenanceData, error) {
	if report.SchemaVersion != doctorSchemaVersion || report.PATHMatches == nil || report.LegacyBinaries == nil || report.Findings == nil {
		return supportProvenanceData{}, errSupportProvenanceInvalid
	}
	if len(report.PATHMatches) > 1024 || len(report.LegacyBinaries) > 1024 {
		return supportProvenanceData{}, errSupportProvenanceLimit
	}
	ownership := ""
	switch report.RunningExecutable.OwnershipHint {
	case "Homebrew formula " + doctorFormula:
		ownership = "homebrew"
	case "running executable; ownership not yet verified":
		ownership = "unverified"
	default:
		return supportProvenanceData{}, errSupportProvenanceInvalid
	}
	if report.Homebrew.Formula != doctorFormula || !validSupportHomebrew(report.Homebrew, ownership) {
		return supportProvenanceData{}, errSupportProvenanceInvalid
	}
	expected := supportDoctorFindingPairs(report, ownership)
	if len(expected) != len(report.Findings) {
		return supportProvenanceData{}, errSupportProvenanceInvalid
	}
	findings := make([]supportFinding, len(report.Findings))
	for index, finding := range report.Findings {
		pair := supportFinding{Severity: finding.Severity, Code: finding.Code}
		if !validSupportFinding(pair) || pair != expected[index] {
			return supportProvenanceData{}, errSupportProvenanceInvalid
		}
		findings[index] = pair
	}
	sort.SliceStable(findings, func(left, right int) bool {
		if findings[left].Severity != findings[right].Severity {
			return findings[left].Severity < findings[right].Severity
		}
		return findings[left].Code < findings[right].Code
	})
	count := len(findings)
	truncated := count > 256
	if truncated {
		findings = findings[:256]
	}
	return supportProvenanceData{
		RunningOwnership: ownership, PATHMatchCount: len(report.PATHMatches), LegacyBinaryCount: len(report.LegacyBinaries),
		HomebrewInstalled: report.Homebrew.Installed, FindingCount: count, Truncated: truncated, Findings: cloneSupportFindings(findings),
	}, nil
}

func supportDoctorFindingPairs(report doctorReport, ownership string) []supportFinding {
	result := make([]supportFinding, 0)
	if len(report.PATHMatches) == 0 {
		result = append(result, supportFinding{Severity: "warning", Code: "path-missing"})
	} else if !report.PATHMatches[0].SameAsRunning {
		result = append(result, supportFinding{Severity: "warning", Code: "path-shadowing"})
	}
	if len(report.PATHMatches) > 1 {
		result = append(result, supportFinding{Severity: "warning", Code: "multiple-path-matches"})
	}
	runningOnPATH := false
	for _, match := range report.PATHMatches {
		if match.SameAsRunning {
			runningOnPATH = true
		}
		if !match.SameAsRunning && knownDifferentVersion(match.VersionHint, report.RunningExecutable.VersionHint) {
			result = append(result, supportFinding{Severity: "warning", Code: "version-mismatch"})
		}
		if !match.SameAsRunning && match.ModulePath != "" && match.ModulePath == report.RunningExecutable.ModulePath &&
			match.GoVersion != "" && report.RunningExecutable.GoVersion != "" && match.GoVersion != report.RunningExecutable.GoVersion {
			result = append(result, supportFinding{Severity: "info", Code: "build-toolchain-mismatch"})
		}
	}
	if !runningOnPATH {
		result = append(result, supportFinding{Severity: "info", Code: "running-not-on-path"})
	}
	if report.Homebrew.Installed && ownership != "homebrew" {
		result = append(result, supportFinding{Severity: "warning", Code: "homebrew-not-running"})
	}
	for range report.LegacyBinaries {
		result = append(result, supportFinding{Severity: "warning", Code: "legacy-binary"})
	}
	if report.Homebrew.ProbeError != "" {
		result = append(result, supportFinding{Severity: "info", Code: "homebrew-unavailable"})
	}
	return result
}

func validSupportHomebrew(value doctorHomebrew, ownership string) bool {
	if value.Installed {
		resolvedValid := value.ResolvedExecutable == "" || (filepath.IsAbs(value.ResolvedExecutable) && filepath.Clean(value.ResolvedExecutable) == value.ResolvedExecutable && filepath.Base(value.ResolvedExecutable) == "dotfiles")
		return value.ProbeError == "" && filepath.IsAbs(value.Prefix) && filepath.Clean(value.Prefix) == value.Prefix &&
			value.ManagedExecutable == filepath.Join(value.Prefix, "bin", "dotfiles") && resolvedValid && (value.VersionHint == "" || value.ResolvedExecutable != "")
	}
	return ownership != "homebrew" && value.Prefix == "" && value.ManagedExecutable == "" && value.ResolvedExecutable == "" && value.VersionHint == ""
}

func projectSupportOperations(set operation.JournalSummarySet) (supportOperationsData, error) {
	if set.Records == nil || len(set.Records) > 20 || (set.Truncated && len(set.Records) != 20) {
		return supportOperationsData{}, errSupportProjection
	}
	records := make([]supportOperationRecord, len(set.Records))
	for index, source := range set.Records {
		if source.Ordinal != index {
			return supportOperationsData{}, errSupportProjection
		}
		record := supportOperationRecord{
			Ordinal: index, Status: source.Status, BackupRecorded: source.BackupRecorded,
			Actions: supportActionCounts{Pending: source.Actions.Pending, Succeeded: source.Actions.Succeeded, Failed: source.Actions.Failed, Skipped: source.Actions.Skipped},
		}
		if source.Rollback != nil {
			record.Rollback = &supportRollback{Status: source.Rollback.Status, Restored: source.Rollback.Restored, Removed: source.Rollback.Removed, Skipped: source.Rollback.Skipped, Warnings: source.Rollback.Warnings}
		}
		if source.DurationMilliseconds != nil {
			duration := *source.DurationMilliseconds
			record.DurationMilliseconds = &duration
		}
		if validateSupportOperationRecord(record, index) != nil {
			return supportOperationsData{}, errSupportProjection
		}
		records[index] = record
	}
	return supportOperationsData{Truncated: set.Truncated, Records: records}, nil
}

func validateSupportDocument(document supportDocument) error {
	if document.SchemaVersion != supportSchemaVersion || document.Kind != supportKind ||
		(document.Outcome != supportComplete && document.Outcome != supportPartial) || !validSHA256String(document.Authority.PublicDigest) ||
		validateSupportBuild(document.Build) != nil || validateSupportInstallation(document.Installation) != nil ||
		validateSupportProvenance(document.Provenance) != nil || validateSupportOperations(document.Operations) != nil {
		return errSupportProjection
	}
	wantOutcome := supportComplete
	if document.Installation.Collection != supportCollected || document.Provenance.Collection != supportCollected ||
		(document.Operations.Collection != supportCollected && document.Operations.Collection != supportNotPresent) {
		wantOutcome = supportPartial
	}
	if document.Outcome != wantOutcome || document.Capabilities != (supportCapabilities{
		Build: supportCollected, Installation: document.Installation.Collection, Provenance: document.Provenance.Collection,
		Operations: document.Operations.Collection, Config: supportNotCollected, Service: supportNotCollected, Auth: supportNotCollected,
	}) {
		return errSupportProjection
	}
	return nil
}

func validateSupportBuild(value supportBuild) error {
	if value.Collection != supportCollected || (value.VersionSource != "compiled" && value.VersionSource != "development" && value.VersionSource != "unknown") ||
		(value.VCSState != "clean" && value.VCSState != "modified" && value.VCSState != "unavailable") {
		return errSupportProjection
	}
	version, source := normalizeSupportVersion(value.Version)
	if version != value.Version || source != value.VersionSource || normalizeSupportToken(value.GoVersion, 64, false) != value.GoVersion ||
		normalizeSupportToken(value.OS, 32, true) != value.OS || normalizeSupportToken(value.Arch, 32, true) != value.Arch {
		return errSupportProjection
	}
	return nil
}

func validateSupportInstallation(value supportInstallation) error {
	if value.Collection == supportCollected {
		if value.ReasonCode != "" || value.Document == nil {
			return errSupportProjection
		}
		_, err := value.Document.MarshalJSON()
		return err
	}
	if value.Collection != supportUnavailable || !allowedInstallationReason(value.ReasonCode) || value.Document != nil {
		return errSupportProjection
	}
	return nil
}

func validateSupportProvenance(value supportProvenance) error {
	if value.Collection == supportUnavailable {
		if !allowedProvenanceReason(value.ReasonCode) || value.RunningOwnership != "" || value.PATHMatchCount != 0 || value.LegacyBinaryCount != 0 || value.HomebrewInstalled || value.FindingCount != 0 || value.Truncated || value.Findings == nil || len(value.Findings) != 0 {
			return errSupportProjection
		}
		return nil
	}
	if value.Collection != supportCollected || value.ReasonCode != "" {
		return errSupportProjection
	}
	return validateSupportProvenanceData(supportProvenanceData{RunningOwnership: value.RunningOwnership, PATHMatchCount: value.PATHMatchCount, LegacyBinaryCount: value.LegacyBinaryCount, HomebrewInstalled: value.HomebrewInstalled, FindingCount: value.FindingCount, Truncated: value.Truncated, Findings: value.Findings})
}

func validateSupportProvenanceData(value supportProvenanceData) error {
	if (value.RunningOwnership != "homebrew" && value.RunningOwnership != "unverified") || value.PATHMatchCount < 0 || value.PATHMatchCount > 1024 || value.LegacyBinaryCount < 0 || value.LegacyBinaryCount > 1024 || value.FindingCount < 0 || value.Findings == nil {
		return errSupportProjection
	}
	if value.RunningOwnership == "homebrew" && !value.HomebrewInstalled {
		return errSupportProjection
	}
	want := value.FindingCount
	if want > 256 {
		want = 256
	}
	if len(value.Findings) != want || value.Truncated != (value.FindingCount > 256) {
		return errSupportProjection
	}
	for index, finding := range value.Findings {
		if !validSupportFinding(finding) || (index > 0 && supportFindingLess(finding, value.Findings[index-1])) {
			return errSupportProjection
		}
	}
	return nil
}

func validateSupportOperations(value supportOperations) error {
	if value.Collection == supportNotPresent {
		if value.ReasonCode != "" || value.Truncated || value.Records == nil || len(value.Records) != 0 {
			return errSupportProjection
		}
		return nil
	}
	if value.Collection == supportUnavailable {
		if !allowedOperationsReason(value.ReasonCode) || value.Truncated || value.Records == nil || len(value.Records) != 0 {
			return errSupportProjection
		}
		return nil
	}
	if value.Collection != supportCollected || value.ReasonCode != "" {
		return errSupportProjection
	}
	return validateSupportOperationsData(supportOperationsData{Truncated: value.Truncated, Records: value.Records})
}

func validateSupportOperationsData(value supportOperationsData) error {
	if value.Records == nil || len(value.Records) > 20 || (value.Truncated && len(value.Records) != 20) {
		return errSupportProjection
	}
	for index, record := range value.Records {
		if validateSupportOperationRecord(record, index) != nil {
			return errSupportProjection
		}
	}
	return nil
}

func validateSupportOperationRecord(record supportOperationRecord, ordinal int) error {
	if record.Ordinal != ordinal || (record.Status != operation.StatusRunning && record.Status != operation.StatusSucceeded && record.Status != operation.StatusPhaseComplete && record.Status != operation.StatusFailed && record.Status != operation.StatusCancelled) ||
		record.Actions.Pending < 0 || record.Actions.Succeeded < 0 || record.Actions.Failed < 0 || record.Actions.Skipped < 0 || !supportCountsDoNotOverflow(record.Actions) {
		return errSupportProjection
	}
	if record.Status == operation.StatusRunning {
		if record.DurationMilliseconds != nil {
			return errSupportProjection
		}
	} else if record.DurationMilliseconds == nil || *record.DurationMilliseconds < 0 {
		return errSupportProjection
	}
	if record.Rollback != nil {
		rollback := record.Rollback
		if (rollback.Status != operation.RollbackSucceeded && rollback.Status != operation.RollbackIncomplete && rollback.Status != operation.RollbackFailed) || rollback.Restored < 0 || rollback.Removed < 0 || rollback.Skipped < 0 || rollback.Warnings < 0 {
			return errSupportProjection
		}
	}
	return nil
}

func supportCountsDoNotOverflow(value supportActionCounts) bool {
	total := 0
	for _, count := range []int{value.Pending, value.Succeeded, value.Failed, value.Skipped} {
		if count > int(^uint(0)>>1)-total {
			return false
		}
		total += count
	}
	return true
}

func supportPublicDigest(document supportDocument) (string, error) {
	type digestDocument struct {
		SchemaVersion int                 `json:"schema_version"`
		Kind          string              `json:"kind"`
		Outcome       string              `json:"outcome"`
		Authority     struct{}            `json:"authority"`
		Build         supportBuild        `json:"build"`
		Installation  supportInstallation `json:"installation"`
		Provenance    supportProvenance   `json:"provenance"`
		Operations    supportOperations   `json:"operations"`
		Capabilities  supportCapabilities `json:"capabilities"`
	}
	encoded, err := json.Marshal(digestDocument{SchemaVersion: document.SchemaVersion, Kind: document.Kind, Outcome: document.Outcome, Build: document.Build, Installation: document.Installation, Provenance: document.Provenance, Operations: document.Operations, Capabilities: document.Capabilities})
	if err != nil {
		return "", errSupportProjection
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validSHA256String(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}

func validSupportFinding(value supportFinding) bool {
	allowed := map[supportFinding]struct{}{
		{Severity: "warning", Code: "path-missing"}: {}, {Severity: "warning", Code: "path-shadowing"}: {},
		{Severity: "warning", Code: "multiple-path-matches"}: {}, {Severity: "warning", Code: "version-mismatch"}: {},
		{Severity: "info", Code: "build-toolchain-mismatch"}: {}, {Severity: "info", Code: "running-not-on-path"}: {},
		{Severity: "warning", Code: "homebrew-not-running"}: {}, {Severity: "warning", Code: "legacy-binary"}: {},
		{Severity: "info", Code: "homebrew-unavailable"}: {},
	}
	_, ok := allowed[value]
	return ok
}

func supportFindingLess(left, right supportFinding) bool {
	return left.Severity < right.Severity || (left.Severity == right.Severity && left.Code < right.Code)
}

func allowedInstallationReason(reason string) bool {
	return reason == supportReasonCancelled || reason == supportReasonStatusUnavailable
}

func allowedProvenanceReason(reason string) bool {
	return reason == supportReasonCancelled || reason == supportReasonProvenanceUnavailable || reason == supportReasonProvenanceInvalid || reason == supportReasonProvenanceLimit
}

func allowedOperationsReason(reason string) bool {
	return reason == supportReasonCancelled || reason == supportReasonJournalUnavailable || reason == supportReasonJournalInvalid || reason == supportReasonJournalLimit
}

func cloneSupportDocument(value supportDocument) supportDocument {
	if value.Installation.Document != nil {
		value.Installation.Document = &supportEmbeddedStatus{encoded: bytes.Clone(value.Installation.Document.encoded)}
	}
	value.Provenance.Findings = cloneSupportFindings(value.Provenance.Findings)
	value.Operations.Records = cloneSupportRecords(value.Operations.Records)
	return value
}

func cloneSupportFindings(values []supportFinding) []supportFinding {
	if values == nil {
		return nil
	}
	result := make([]supportFinding, len(values))
	copy(result, values)
	return result
}

func cloneSupportRecords(values []supportOperationRecord) []supportOperationRecord {
	if values == nil {
		return nil
	}
	result := make([]supportOperationRecord, len(values))
	for index, record := range values {
		if record.Rollback != nil {
			rollback := *record.Rollback
			record.Rollback = &rollback
		}
		if record.DurationMilliseconds != nil {
			duration := *record.DurationMilliseconds
			record.DurationMilliseconds = &duration
		}
		result[index] = record
	}
	return result
}

func zeroSupportProvenanceData(value supportProvenanceData) bool {
	return value.RunningOwnership == "" && value.PATHMatchCount == 0 && value.LegacyBinaryCount == 0 && !value.HomebrewInstalled && value.FindingCount == 0 && !value.Truncated && value.Findings == nil
}

func zeroSupportOperationsData(value supportOperationsData) bool {
	return !value.Truncated && value.Records == nil
}
