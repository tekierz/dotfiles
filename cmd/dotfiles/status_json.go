package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

const (
	statusSchemaVersion = 1
	statusKind          = "dotfiles.status"
	statusRedacted      = "[redacted]"
)

var errStatusCollection = errors.New("status collection failed")

type statusJSONRuntime struct {
	tools          func() []tools.Tool
	detectPlatform func() pkg.Platform
	detectManager  func() pkg.PackageManager
	collect        func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error)
}

type statusCommandRuntime struct {
	writeHuman func(io.Writer) error
	writeJSON  func(context.Context, io.Writer) error
}

var statusRuntime = statusCommandRuntime{
	writeHuman: writeHumanStatus,
	writeJSON: func(ctx context.Context, writer io.Writer) error {
		return writeStatusJSON(ctx, writer, defaultStatusJSONRuntime())
	},
}

type statusDocument struct {
	SchemaVersion int                    `json:"schema_version"`
	Kind          string                 `json:"kind"`
	Platform      string                 `json:"platform"`
	Manager       string                 `json:"manager"`
	Snapshot      statusSnapshotIdentity `json:"snapshot"`
	Capabilities  statusCapabilities     `json:"capabilities"`
	Tools         []statusTool           `json:"tools"`
}

// validatedStatusDocument is a fully collected, redacted status projection.
// Keeping its value private prevents callers from treating a partially built
// statusDocument as safe to marshal or embed.
type validatedStatusDocument struct {
	value statusDocument
}

type statusSnapshotIdentity struct {
	SchemaVersion int    `json:"schema_version"`
	Generation    uint64 `json:"generation"`
	Digest        string `json:"digest"`
}

type statusCapabilities struct {
	Installation string `json:"installation"`
	Config       string `json:"config"`
	Service      string `json:"service"`
	Auth         string `json:"auth"`
}

type statusTool struct {
	ToolID              string                `json:"tool_id"`
	Presence            health.Presence       `json:"presence"`
	Installability      health.Installability `json:"installability"`
	InstallRecipeDigest string                `json:"install_recipe_digest"`
	Package             statusPackage         `json:"package"`
	Direct              statusDirect          `json:"direct"`
}

type statusPackage struct {
	State              health.PackageState `json:"state"`
	Provider           string              `json:"provider"`
	ExpectedReceipts   []string            `json:"expected_receipts"`
	ObservedReceipts   []string            `json:"observed_receipts"`
	MissingReceipts    []string            `json:"missing_receipts"`
	UnresolvedReceipts []string            `json:"unresolved_receipts"`
	Authoritative      bool                `json:"authoritative"`
	Complete           bool                `json:"complete"`
	Namespaces         []statusNamespace   `json:"namespaces"`
	Diagnostic         statusDiagnostic    `json:"diagnostic"`
}

type statusNamespace struct {
	Namespace          health.PackageNamespace `json:"namespace"`
	State              health.PackageState     `json:"state"`
	ExpectedReceipts   []string                `json:"expected_receipts"`
	ObservedReceipts   []string                `json:"observed_receipts"`
	MissingReceipts    []string                `json:"missing_receipts"`
	UnresolvedReceipts []string                `json:"unresolved_receipts"`
	Complete           bool                    `json:"complete"`
	Diagnostic         statusDiagnostic        `json:"diagnostic"`
}

type statusDirect struct {
	State         health.ComponentState `json:"state"`
	Authoritative bool                  `json:"authoritative"`
	Alternatives  []statusAlternative   `json:"alternatives"`
	Diagnostic    statusDiagnostic      `json:"diagnostic"`
}

type statusAlternative struct {
	Kind        health.DirectSourceKind `json:"kind"`
	Identifiers []string                `json:"identifiers"`
	State       health.ComponentState   `json:"state"`
	Diagnostic  statusDiagnostic        `json:"diagnostic"`
}

type statusDiagnostic struct {
	Code    health.DiagnosticCode `json:"code"`
	Summary string                `json:"summary"`
}

func defaultStatusJSONRuntime() statusJSONRuntime {
	return statusJSONRuntime{
		tools:          func() []tools.Tool { return tools.GetRegistry().All() },
		detectPlatform: pkg.DetectPlatform,
		detectManager:  pkg.DetectManager,
		collect:        tools.ObserveInstallationHealth,
	}
}

func newRegisteredStatusCommand() *cobra.Command {
	return newStatusCommand(statusCommandRuntime{
		writeHuman: func(writer io.Writer) error { return statusRuntime.writeHuman(writer) },
		writeJSON:  func(ctx context.Context, writer io.Writer) error { return statusRuntime.writeJSON(ctx, writer) },
	})
}

func newStatusCommand(runtime statusCommandRuntime) *cobra.Command {
	command := &cobra.Command{
		Use:           "status",
		Short:         "Show current configuration status",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			jsonMode, err := command.Flags().GetBool("json")
			if err != nil {
				return errStatusCollection
			}
			if jsonMode {
				if runtime.writeJSON == nil || runtime.writeJSON(command.Context(), command.OutOrStdout()) != nil {
					return errStatusCollection
				}
				return nil
			}
			if runtime.writeHuman == nil {
				return errors.New("status formatter unavailable")
			}
			return runtime.writeHuman(command.OutOrStdout())
		},
	}
	command.Flags().Bool("json", false, "Print machine-readable installation status")
	return command
}

func writeStatusJSON(ctx context.Context, writer io.Writer, runtime statusJSONRuntime) error {
	document, err := collectStatusDocument(ctx, runtime)
	if err != nil {
		return errStatusCollection
	}
	encoded, err := marshalStatusDocument(document)
	if err != nil {
		return errStatusCollection
	}
	encoded = append(encoded, '\n')
	written, err := writer.Write(encoded)
	if err != nil || written != len(encoded) {
		return errStatusCollection
	}
	return nil
}

func collectStatusDocument(ctx context.Context, runtime statusJSONRuntime) (validatedStatusDocument, error) {
	if runtime.tools == nil || runtime.detectPlatform == nil || runtime.detectManager == nil || runtime.collect == nil {
		return validatedStatusDocument{}, errStatusCollection
	}
	registryTools := runtime.tools()
	platform := runtime.detectPlatform()
	manager := runtime.detectManager()
	snapshot, err := runtime.collect(ctx, registryTools, manager, platform, 1)
	if err != nil {
		return validatedStatusDocument{}, errStatusCollection
	}
	expectedManager := ""
	if manager != nil {
		expectedManager = manager.Name()
	}
	if snapshot.SchemaVersion() != health.CurrentInstallationSchemaVersion ||
		snapshot.Generation() != 1 ||
		snapshot.Platform() != string(platform) ||
		snapshot.Manager() != expectedManager ||
		snapshot.Digest() == "" {
		return validatedStatusDocument{}, errStatusCollection
	}

	document := statusDocument{
		SchemaVersion: statusSchemaVersion,
		Kind:          statusKind,
		Platform:      snapshot.Platform(),
		Manager:       snapshot.Manager(),
		Snapshot: statusSnapshotIdentity{
			SchemaVersion: snapshot.SchemaVersion(),
			Generation:    snapshot.Generation(),
		},
		Capabilities: statusCapabilities{
			Installation: "collected",
			Config:       "not_collected",
			Service:      "not_collected",
			Auth:         "not_collected",
		},
		Tools: make([]statusTool, 0, len(snapshot.Tools())),
	}
	for _, observation := range snapshot.Tools() {
		document.Tools = append(document.Tools, publicStatusTool(observation))
	}
	digest, err := statusPublicDigest(document)
	if err != nil {
		return validatedStatusDocument{}, errStatusCollection
	}
	document.Snapshot.Digest = digest
	return validatedStatusDocument{value: document}, nil
}

func marshalStatusDocument(document validatedStatusDocument) ([]byte, error) {
	value := document.value
	if value.SchemaVersion != statusSchemaVersion || value.Kind != statusKind ||
		value.Snapshot.SchemaVersion != health.CurrentInstallationSchemaVersion || value.Snapshot.Generation != 1 ||
		value.Snapshot.Digest == "" || value.Tools == nil || value.Capabilities != (statusCapabilities{
		Installation: "collected", Config: "not_collected", Service: "not_collected", Auth: "not_collected",
	}) {
		return nil, errStatusCollection
	}
	digest, err := statusPublicDigest(value)
	if err != nil || digest != value.Snapshot.Digest {
		return nil, errStatusCollection
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, errStatusCollection
	}
	return encoded, nil
}

// MarshalJSON embeds the exact validated public status object without a
// command-line newline. json.Marshal returns caller-owned bytes, so embedding
// or mutating one result cannot alter later encodings of the document.
func (document validatedStatusDocument) MarshalJSON() ([]byte, error) {
	return marshalStatusDocument(document)
}

func publicStatusTool(observation health.InstallationObservation) statusTool {
	packages := observation.Package()
	direct := observation.Direct()
	result := statusTool{
		ToolID: observation.ToolID(), Presence: observation.Presence(), Installability: observation.Installability(), InstallRecipeDigest: observation.InstallRecipeDigest(),
		Package: statusPackage{
			State: packages.State, Provider: redactStatusValue(packages.Provider),
			ExpectedReceipts: redactStatusValues(packages.ExpectedReceipts), ObservedReceipts: redactStatusValues(packages.ObservedReceipts), MissingReceipts: redactStatusValues(packages.MissingReceipts), UnresolvedReceipts: redactStatusValues(packages.UnresolvedReceipts),
			Authoritative: packages.Authoritative, Complete: packages.Complete, Namespaces: make([]statusNamespace, 0, len(packages.Namespaces)), Diagnostic: publicStatusDiagnostic(packages.DiagnosticCode, packages.DiagnosticSummary),
		},
		Direct: statusDirect{State: direct.State, Authoritative: direct.Authoritative, Alternatives: make([]statusAlternative, 0, len(direct.Alternatives)), Diagnostic: publicStatusDiagnostic(direct.DiagnosticCode, direct.DiagnosticSummary)},
	}
	for _, namespace := range packages.Namespaces {
		result.Package.Namespaces = append(result.Package.Namespaces, statusNamespace{
			Namespace: namespace.Namespace, State: namespace.State,
			ExpectedReceipts: redactStatusValues(namespace.ExpectedReceipts), ObservedReceipts: redactStatusValues(namespace.ObservedReceipts), MissingReceipts: redactStatusValues(namespace.MissingReceipts), UnresolvedReceipts: redactStatusValues(namespace.UnresolvedReceipts),
			Complete: namespace.Complete, Diagnostic: publicStatusDiagnostic(namespace.DiagnosticCode, namespace.DiagnosticSummary),
		})
	}
	for _, alternative := range direct.Alternatives {
		result.Direct.Alternatives = append(result.Direct.Alternatives, statusAlternative{
			Kind: alternative.Kind, Identifiers: redactStatusValues(alternative.Identifiers), State: alternative.State, Diagnostic: publicStatusDiagnostic(alternative.DiagnosticCode, alternative.DiagnosticSummary),
		})
	}
	return result
}

func publicStatusDiagnostic(code health.DiagnosticCode, summary string) statusDiagnostic {
	return statusDiagnostic{Code: code, Summary: redactStatusValue(summary)}
}

func redactStatusValues(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = redactStatusValue(value)
	}
	return result
}

func redactStatusValue(value string) string {
	if statusValueIsSensitive(value) {
		return statusRedacted
	}
	return value
}

func statusValueIsSensitive(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if containsStatusPath(trimmed) ||
		containsBoundedStatusToken(lower, "bearer ") ||
		containsBoundedStatusToken(lower, "authorization:") ||
		strings.Contains(lower, "raw dial") ||
		strings.Contains(lower, "dial error") {
		return true
	}
	for _, key := range []string{
		"password", "passwd", "api_key", "api-key", "token", "access_token", "client_secret", "private_key",
	} {
		if containsCredentialAssignment(lower, key) {
			return true
		}
	}
	return false
}

func containsStatusPath(value string) bool {
	for index := 0; index < len(value); index++ {
		if index > 0 && isStatusTokenByte(value[index-1]) {
			continue
		}
		if statusValueIsPath(value[index:]) {
			return true
		}
	}
	return false
}

func containsBoundedStatusToken(value, token string) bool {
	for offset := 0; offset < len(value); {
		index := strings.Index(value[offset:], token)
		if index < 0 {
			return false
		}
		index += offset
		if index == 0 || !isStatusTokenByte(value[index-1]) {
			return true
		}
		offset = index + len(token)
	}
	return false
}

func statusValueIsPath(value string) bool {
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\\`) || strings.HasPrefix(value, "//") {
		return true
	}
	for _, prefix := range []string{"~/", `~\`, "./", `.\`, "../", `..\`} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return len(value) >= 3 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':' && (value[2] == '/' || value[2] == '\\')
}

func containsCredentialAssignment(value, key string) bool {
	for offset := 0; offset < len(value); {
		index := strings.Index(value[offset:], key)
		if index < 0 {
			return false
		}
		index += offset
		beforeBoundary := index == 0 || !isCredentialNameByte(value[index-1])
		after := index + len(key)
		for after < len(value) && (value[after] == ' ' || value[after] == '\t') {
			after++
		}
		if beforeBoundary && after < len(value) && value[after] == '=' {
			return true
		}
		offset = index + len(key)
	}
	return false
}

func isCredentialNameByte(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9') || value == '_' || value == '-'
}

func isStatusTokenByte(value byte) bool {
	return isCredentialNameByte(value) || (value >= 'A' && value <= 'Z') || value == '@'
}

func statusPublicDigest(document statusDocument) (string, error) {
	encodedTools, err := json.Marshal(document.Tools)
	if err != nil {
		return "", fmt.Errorf("marshal public status tools: %w", err)
	}
	var canonicalTools any
	if err := json.Unmarshal(encodedTools, &canonicalTools); err != nil {
		return "", fmt.Errorf("canonicalize public status tools: %w", err)
	}
	projection := map[string]any{
		"schema_version": document.Snapshot.SchemaVersion,
		"generation":     document.Snapshot.Generation,
		"platform":       document.Platform,
		"manager":        document.Manager,
		"tools":          canonicalTools,
	}
	canonical, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("marshal public status projection: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}
