package operation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"path"
	"strings"
	"unicode"
)

const CurrentRemoteArtifactSchemaVersion = 1

var ErrInvalidRemoteArtifact = errors.New("invalid remote artifact descriptor")

// RemoteArtifactSource identifies the only remote source forms accepted by
// artifact-backed actions. Generic scripts and mutable release endpoints are
// intentionally absent.
type RemoteArtifactSource string

const (
	RemoteArtifactHTTPSArchive  RemoteArtifactSource = "https_archive"
	RemoteArtifactGitRepository RemoteArtifactSource = "git_repository"
)

// RemoteArtifactVerification declares how staged bytes or repository state
// must be verified before anything can enter the live destination.
type RemoteArtifactVerification string

const (
	RemoteArtifactVerifySHA256    RemoteArtifactVerification = "sha256"
	RemoteArtifactVerifyGitCommit RemoteArtifactVerification = "git_commit"
)

// RemoteArtifactRisk is closed public vocabulary for the effect of accepting
// a verified artifact. Verification proves identity, not that code is safe.
type RemoteArtifactRisk string

const (
	RemoteArtifactRiskVerifiedData RemoteArtifactRisk = "installs_verified_data"
	RemoteArtifactRiskRuntimeCode  RemoteArtifactRisk = "loads_downloaded_runtime_code"
	RemoteArtifactRiskExecutable   RemoteArtifactRisk = "executes_downloaded_code"
)

// RemoteArtifactSpec is caller-owned input. ActionID and Destination bind the
// source directly to the mutation identity that will consume it.
type RemoteArtifactSpec struct {
	ActionID     string
	Destination  string
	Source       RemoteArtifactSource
	URL          string
	Version      string
	ImmutableRef string
	Verification RemoteArtifactVerification
	Digest       string
	Risk         RemoteArtifactRisk
}

// RemoteArtifactReview is the complete display-safe projection callers show
// during review. It contains no credentials, local paths, or execution input.
type RemoteArtifactReview struct {
	SchemaVersion   int                        `json:"schema_version"`
	ActionID        string                     `json:"action_id"`
	Destination     string                     `json:"destination"`
	Source          RemoteArtifactSource       `json:"source"`
	URL             string                     `json:"url"`
	Version         string                     `json:"version"`
	ImmutableRef    string                     `json:"immutable_ref"`
	Verification    RemoteArtifactVerification `json:"verification"`
	Digest          string                     `json:"digest"`
	Risk            RemoteArtifactRisk         `json:"risk"`
	AuthorityDigest string                     `json:"authority_digest"`
}

type remoteArtifactDocument struct {
	SchemaVersion int                        `json:"schema_version"`
	ActionID      string                     `json:"action_id"`
	Destination   string                     `json:"destination"`
	Source        RemoteArtifactSource       `json:"source"`
	URL           string                     `json:"url"`
	Version       string                     `json:"version"`
	ImmutableRef  string                     `json:"immutable_ref"`
	Verification  RemoteArtifactVerification `json:"verification"`
	Digest        string                     `json:"digest"`
	Risk          RemoteArtifactRisk         `json:"risk"`
}

// RemoteArtifact is an immutable descriptor. The zero value is invalid.
type RemoteArtifact struct {
	document        remoteArtifactDocument
	authorityDigest string
}

func NewRemoteArtifact(spec RemoteArtifactSpec) (RemoteArtifact, error) {
	document := remoteArtifactDocument{
		SchemaVersion: CurrentRemoteArtifactSchemaVersion,
		ActionID:      spec.ActionID, Destination: spec.Destination, Source: spec.Source,
		URL: spec.URL, Version: spec.Version, ImmutableRef: spec.ImmutableRef,
		Verification: spec.Verification, Digest: spec.Digest, Risk: spec.Risk,
	}
	if !validRemoteArtifactDocument(document) {
		return RemoteArtifact{}, ErrInvalidRemoteArtifact
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return RemoteArtifact{}, ErrInvalidRemoteArtifact
	}
	digest := sha256.Sum256(append([]byte("dotfiles.operation.remote-artifact.v1\x00"), encoded...))
	return RemoteArtifact{document: document, authorityDigest: hex.EncodeToString(digest[:])}, nil
}

func (artifact RemoteArtifact) AuthorityDigest() string {
	if !artifact.valid() {
		return ""
	}
	return artifact.authorityDigest
}

func (artifact RemoteArtifact) Review() RemoteArtifactReview {
	if !artifact.valid() {
		return RemoteArtifactReview{}
	}
	document := artifact.document
	return RemoteArtifactReview{
		SchemaVersion: document.SchemaVersion, ActionID: document.ActionID, Destination: document.Destination,
		Source: document.Source, URL: document.URL, Version: document.Version, ImmutableRef: document.ImmutableRef,
		Verification: document.Verification, Digest: document.Digest, Risk: document.Risk,
		AuthorityDigest: artifact.authorityDigest,
	}
}

func (artifact RemoteArtifact) MarshalJSON() ([]byte, error) {
	if !artifact.valid() {
		return nil, ErrInvalidRemoteArtifact
	}
	return json.Marshal(artifact.Review())
}

func (artifact RemoteArtifact) valid() bool {
	if !validRemoteArtifactDocument(artifact.document) || len(artifact.authorityDigest) != sha256.Size*2 {
		return false
	}
	encoded, err := json.Marshal(artifact.document)
	if err != nil {
		return false
	}
	digest := sha256.Sum256(append([]byte("dotfiles.operation.remote-artifact.v1\x00"), encoded...))
	return artifact.authorityDigest == hex.EncodeToString(digest[:])
}

func validRemoteArtifactDocument(document remoteArtifactDocument) bool {
	if document.SchemaVersion != CurrentRemoteArtifactSchemaVersion || !validArtifactActionID(document.ActionID) || !validRelativeTarget(document.Destination) || strings.ContainsRune(document.Destination, '\\') ||
		!validArtifactVersion(document.Version) || !validArtifactRisk(document.Risk) || !validArtifactURL(document.URL) {
		return false
	}
	switch document.Source {
	case RemoteArtifactHTTPSArchive:
		return document.Verification == RemoteArtifactVerifySHA256 && validArtifactRef(document.ImmutableRef) &&
			validLowerHex(document.Digest, sha256.Size*2) && artifactURLContainsPin(document.URL, document.Version, document.ImmutableRef)
	case RemoteArtifactGitRepository:
		return document.Verification == RemoteArtifactVerifyGitCommit && validLowerHex(document.ImmutableRef, 40) && document.Digest == document.ImmutableRef
	default:
		return false
	}
}

func artifactURLContainsPin(raw, version, immutableRef string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	foldedSegments := strings.Split(strings.ToLower(parsed.Path), "/")
	for _, segment := range foldedSegments {
		switch segment {
		case "latest", "current", "stable", "nightly", "head", "main", "master", "trunk":
			return false
		}
	}
	return strings.Contains(parsed.Path, version) || strings.Contains(parsed.Path, immutableRef)
}

func validArtifactURL(raw string) bool {
	if raw == "" || len(raw) > 2048 || hasUnsafeArtifactText(raw) {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return false
	}
	if parsed.Port() != "" || parsed.Path == "" || parsed.Path == "/" || path.Clean(parsed.Path) != parsed.Path || strings.Contains(parsed.EscapedPath(), "%2f") || strings.Contains(parsed.EscapedPath(), "%2F") {
		return false
	}
	return parsed.Hostname() == strings.ToLower(parsed.Hostname()) && parsed.String() == raw
}

func validArtifactActionID(value string) bool {
	if value == "" || len(value) > 128 || hasUnsafeArtifactText(value) {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && !strings.ContainsRune("-._:", character) {
			return false
		}
	}
	return true
}

func validArtifactVersion(value string) bool {
	if !validArtifactRef(value) || strings.ContainsRune(value, '/') {
		return false
	}
	switch strings.ToLower(value) {
	case "latest", "current", "stable", "nightly", "head", "main", "master", "trunk":
		return false
	default:
		return true
	}
}

func validArtifactRef(value string) bool {
	if value == "" || len(value) > 128 || hasUnsafeArtifactText(value) || strings.HasPrefix(value, "-") ||
		strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.Contains(value, "..") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	for _, character := range value {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) && !strings.ContainsRune("+._/-", character) {
			return false
		}
	}
	return true
}

func validArtifactRisk(value RemoteArtifactRisk) bool {
	return value == RemoteArtifactRiskVerifiedData || value == RemoteArtifactRiskRuntimeCode || value == RemoteArtifactRiskExecutable
}

func validLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func hasUnsafeArtifactText(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) || unicode.In(character, unicode.Cf) {
			return true
		}
	}
	return false
}
