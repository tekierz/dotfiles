package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

type statusTestWriter struct {
	bytes.Buffer
	writes int
	short  bool
	err    error
}

func (writer *statusTestWriter) Write(value []byte) (int, error) {
	writer.writes++
	if writer.err != nil {
		return 0, writer.err
	}
	if writer.short && len(value) > 0 {
		_, _ = writer.Buffer.Write(value[:len(value)-1])
		return len(value) - 1, nil
	}
	return writer.Buffer.Write(value)
}

func TestStatusCommandRoutesHumanAndJSONWithoutChangingHumanBytes(t *testing.T) {
	const humanBytes = "Dotfiles Status\n===============\nlegacy human bytes stay unchanged\n"
	const jsonBytes = "{\"schema_version\":1,\"kind\":\"dotfiles.status\"}\n"

	newRuntime := func(humanCalls, jsonCalls *int) statusCommandRuntime {
		return statusCommandRuntime{
			writeHuman: func(writer io.Writer) error {
				*humanCalls++
				_, err := io.WriteString(writer, humanBytes)
				return err
			},
			writeJSON: func(_ context.Context, writer io.Writer) error {
				*jsonCalls++
				_, err := io.WriteString(writer, jsonBytes)
				return err
			},
		}
	}

	humanCalls, jsonCalls := 0, 0
	humanCommand := newStatusCommand(newRuntime(&humanCalls, &jsonCalls))
	if humanCommand.Use != "status" || humanCommand.Short != "Show current configuration status" {
		t.Fatalf("status command identity changed: Use=%q Short=%q", humanCommand.Use, humanCommand.Short)
	}
	jsonFlag := humanCommand.Flags().Lookup("json")
	if jsonFlag == nil || jsonFlag.DefValue != "false" || jsonFlag.Changed {
		t.Fatalf("local --json flag = %#v, want present and default false", jsonFlag)
	}
	var humanOutput bytes.Buffer
	humanCommand.SetOut(&humanOutput)
	humanCommand.SetErr(io.Discard)
	humanCommand.SetArgs([]string{})
	if err := humanCommand.Execute(); err != nil {
		t.Fatalf("execute human status: %v", err)
	}
	if humanOutput.String() != humanBytes || humanCalls != 1 || jsonCalls != 0 {
		t.Fatalf("human route output/calls = %q %d/%d, want exact legacy bytes and 1/0", humanOutput.String(), humanCalls, jsonCalls)
	}

	humanCalls, jsonCalls = 0, 0
	jsonCommand := newStatusCommand(newRuntime(&humanCalls, &jsonCalls))
	var jsonOutput bytes.Buffer
	jsonCommand.SetOut(&jsonOutput)
	jsonCommand.SetErr(io.Discard)
	jsonCommand.SetArgs([]string{"--json"})
	if err := jsonCommand.Execute(); err != nil {
		t.Fatalf("execute JSON status: %v", err)
	}
	if jsonOutput.String() != jsonBytes || humanCalls != 0 || jsonCalls != 1 {
		t.Fatalf("JSON route output/calls = %q %d/%d, want exact JSON bytes and 0/1", jsonOutput.String(), humanCalls, jsonCalls)
	}
}

func TestStatusCommandJSONFailureIsAtomicGenericAndDoesNotFallback(t *testing.T) {
	const rawFailure = "/Users/alice token=SECRET raw encoder failure"
	humanCalls, jsonCalls := 0, 0
	command := newStatusCommand(statusCommandRuntime{
		writeHuman: func(io.Writer) error {
			humanCalls++
			return nil
		},
		writeJSON: func(context.Context, io.Writer) error {
			jsonCalls++
			return errors.New(rawFailure)
		},
	})
	var stdout, stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs([]string{"--json"})
	err := command.Execute()
	if err == nil || err.Error() != "status collection failed" {
		t.Fatalf("JSON failure = %v, want exact generic error", err)
	}
	if stdout.Len() != 0 || humanCalls != 0 || jsonCalls != 1 {
		t.Fatalf("failed JSON route stdout/calls = %q %d/%d, want empty and 0/1", stdout.String(), humanCalls, jsonCalls)
	}
	if strings.Contains(stderr.String(), rawFailure) || strings.Contains(err.Error(), rawFailure) {
		t.Fatalf("raw failure leaked through command boundary: stderr=%q err=%q", stderr.String(), err.Error())
	}
}

func TestStatusCommandUnknownFlagRemainsCobraErrorWithoutCollection(t *testing.T) {
	humanCalls, jsonCalls := 0, 0
	command := newStatusCommand(statusCommandRuntime{
		writeHuman: func(io.Writer) error { humanCalls++; return nil },
		writeJSON:  func(context.Context, io.Writer) error { jsonCalls++; return nil },
	})
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"--definitely-unknown"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --definitely-unknown") {
		t.Fatalf("unknown flag error = %v, want normal Cobra error", err)
	}
	if stdout.Len() != 0 || humanCalls != 0 || jsonCalls != 0 {
		t.Fatalf("unknown flag reached formatter: stdout=%q calls=%d/%d", stdout.String(), humanCalls, jsonCalls)
	}
}

func TestRegisteredStatusCommandUsesInjectedRuntimeForActualRootRoute(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"status"})
	if err != nil {
		t.Fatalf("find registered status command: %v", err)
	}
	if found != statusCmd {
		t.Fatalf("registered status command = %p, want exact global statusCmd %p", found, statusCmd)
	}
	flag := statusCmd.Flags().Lookup("json")
	if flag == nil || flag.DefValue != "false" {
		t.Fatalf("actual registered status --json flag = %#v, want local default false", flag)
	}

	previous := statusRuntime
	humanCalls, jsonCalls := 0, 0
	statusRuntime = statusCommandRuntime{
		writeHuman: func(writer io.Writer) error {
			humanCalls++
			_, writeErr := io.WriteString(writer, "actual legacy human bytes\n")
			return writeErr
		},
		writeJSON: func(_ context.Context, writer io.Writer) error {
			jsonCalls++
			_, writeErr := io.WriteString(writer, "{\"actual_json\":true}\n")
			return writeErr
		},
	}
	t.Cleanup(func() {
		statusRuntime = previous
		resetActualStatusCommandForTest(t)
	})

	resetActualStatusCommandForTest(t)
	var humanOutput bytes.Buffer
	rootCmd.SetOut(&humanOutput)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs([]string{"status"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute actual human status route: %v", err)
	}
	if humanOutput.String() != "actual legacy human bytes\n" || humanCalls != 1 || jsonCalls != 0 {
		t.Fatalf("actual human route output/calls = %q %d/%d", humanOutput.String(), humanCalls, jsonCalls)
	}

	resetActualStatusCommandForTest(t)
	var jsonOutput bytes.Buffer
	rootCmd.SetOut(&jsonOutput)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs([]string{"status", "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute actual JSON status route: %v", err)
	}
	if jsonOutput.String() != "{\"actual_json\":true}\n" || humanCalls != 1 || jsonCalls != 1 {
		t.Fatalf("actual JSON route output/calls = %q %d/%d", jsonOutput.String(), humanCalls, jsonCalls)
	}
}

func TestWriteHumanStatusPreservesLegacyLabelsAndIsNotJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	cfg := config.DefaultGlobalConfig()
	cfg.Theme = "nord"
	cfg.NavStyle = "vim"
	if err := config.SaveGlobalConfig(cfg); err != nil {
		t.Fatalf("save isolated global config: %v", err)
	}

	var output bytes.Buffer
	if err := writeHumanStatus(&output); err != nil {
		t.Fatalf("write human status: %v", err)
	}
	for _, label := range []string{"Dotfiles Status\n===============", "Theme:      nord", "Navigation: vim", "Config dir:", "Installed Tools:"} {
		if !strings.Contains(output.String(), label) {
			t.Errorf("legacy human output missing %q:\n%s", label, output.String())
		}
	}
	if json.Valid(output.Bytes()) || strings.HasPrefix(strings.TrimSpace(output.String()), "{") {
		t.Fatalf("human status unexpectedly emitted JSON: %s", output.String())
	}
}

func TestExecuteRootMapsActualStatusJSONToShippedExitContract(t *testing.T) {
	previous := statusRuntime
	t.Cleanup(func() {
		statusRuntime = previous
		resetActualStatusCommandForTest(t)
	})

	const rawFailure = "/Users/alice token=SECRET raw collector failure"
	statusRuntime = statusCommandRuntime{
		writeHuman: func(io.Writer) error { return nil },
		writeJSON:  func(context.Context, io.Writer) error { return errors.New(rawFailure) },
	}
	resetActualStatusCommandForTest(t)
	var stdout, stderr bytes.Buffer
	if code := executeRoot([]string{"status", "--json"}, &stdout, &stderr); code != 1 {
		t.Fatalf("fatal JSON exit = %d, want 1", code)
	}
	if stdout.Len() != 0 || stderr.String() != "status collection failed\n" || strings.Contains(stderr.String(), rawFailure) {
		t.Fatalf("fatal shipped output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	statusRuntime = statusCommandRuntime{
		writeHuman: func(io.Writer) error { return errors.New("unexpected human formatter") },
		writeJSON: func(_ context.Context, writer io.Writer) error {
			_, err := io.WriteString(writer, "{\"schema_version\":1}\n")
			return err
		},
	}
	resetActualStatusCommandForTest(t)
	stdout.Reset()
	stderr.Reset()
	if code := executeRoot([]string{"status", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("successful JSON exit = %d, want 0; stderr=%q", code, stderr.String())
	}
	if stdout.String() != "{\"schema_version\":1}\n" || stderr.Len() != 0 {
		t.Fatalf("successful shipped output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func resetActualStatusCommandForTest(t *testing.T) {
	t.Helper()
	if flag := statusCmd.Flags().Lookup("json"); flag != nil {
		if err := flag.Value.Set("false"); err != nil {
			t.Fatalf("reset actual status --json: %v", err)
		}
		flag.Changed = false
	}
	rootCmd.SetArgs([]string{})
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}

func TestWriteStatusJSONDeterministicExactSchemaAndEvidence(t *testing.T) {
	snapshot := statusJSONSnapshot(t)
	runtime := statusJSONRuntime{
		tools:          func() []tools.Tool { return []tools.Tool{tools.NewZshTool(), tools.NewGhosttyTool()} },
		detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
		detectManager: func() pkg.PackageManager {
			manager := pkg.NewMockPackageManager()
			manager.ManagerName = "brew"
			return manager
		},
		collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			return snapshot, nil
		},
	}

	var first, second bytes.Buffer
	if err := writeStatusJSON(context.Background(), &first, runtime); err != nil {
		t.Fatalf("first status JSON: %v", err)
	}
	if err := writeStatusJSON(context.Background(), &second, runtime); err != nil {
		t.Fatalf("second status JSON: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("status JSON is nondeterministic:\nfirst=%s\nsecond=%s", first.String(), second.String())
	}
	publicDigest := independentStatusPublicDigest(t, first.Bytes())
	if got, want := first.String(), statusJSONGolden(publicDigest); got != want {
		t.Fatalf("status JSON bytes changed (schema changes require a version bump):\ngot:  %s\nwant: %s", got, want)
	}
	if first.Len() == 0 || first.Bytes()[first.Len()-1] != '\n' || bytes.Count(first.Bytes(), []byte{'\n'}) != 1 {
		t.Fatalf("status JSON must be exactly one newline-terminated object: %q", first.String())
	}

	var document map[string]any
	if err := json.Unmarshal(first.Bytes(), &document); err != nil {
		t.Fatalf("decode status JSON: %v\n%s", err, first.String())
	}
	assertJSONKeys(t, document, "schema_version", "kind", "platform", "manager", "snapshot", "capabilities", "tools")
	if document["schema_version"] != float64(1) || document["kind"] != "dotfiles.status" || document["platform"] != "macos" || document["manager"] != "brew" {
		t.Fatalf("unexpected status envelope: %#v", document)
	}
	assertJSONKeys(t, objectValue(t, document, "snapshot"), "schema_version", "generation", "digest")
	if got := objectValue(t, document, "snapshot"); got["schema_version"] != float64(1) || got["generation"] != float64(1) || got["digest"] != publicDigest {
		t.Fatalf("unexpected snapshot identity: %#v", got)
	}
	if got := objectValue(t, document, "snapshot")["digest"]; got != publicDigest {
		t.Fatalf("snapshot digest = %#v, want independently computed public digest %q", got, publicDigest)
	}
	if publicDigest == snapshot.Digest() {
		t.Fatal("public status digest must bind the public projection, not the private health snapshot encoding")
	}
	capabilities := objectValue(t, document, "capabilities")
	assertJSONKeys(t, capabilities, "installation", "config", "service", "auth")
	wantCapabilities := map[string]string{"installation": "collected", "config": "not_collected", "service": "not_collected", "auth": "not_collected"}
	for key, want := range wantCapabilities {
		if capabilities[key] != want {
			t.Errorf("capability %s = %#v, want %q", key, capabilities[key], want)
		}
	}

	rows := arrayValue(t, document, "tools")
	if len(rows) != 2 || objectAt(t, rows, 0)["tool_id"] != "ghostty" || objectAt(t, rows, 1)["tool_id"] != "zsh" {
		t.Fatalf("tools are not stable-sorted by tool_id: %#v", rows)
	}
	for _, raw := range rows {
		row := raw.(map[string]any)
		assertJSONKeys(t, row, "tool_id", "presence", "installability", "install_recipe_digest", "package", "direct")
		packageEvidence := objectValue(t, row, "package")
		assertJSONKeys(t, packageEvidence, "state", "provider", "expected_receipts", "observed_receipts", "missing_receipts", "unresolved_receipts", "authoritative", "complete", "namespaces", "diagnostic")
		for _, key := range []string{"expected_receipts", "observed_receipts", "missing_receipts", "unresolved_receipts", "namespaces"} {
			if packageEvidence[key] == nil {
				t.Errorf("package.%s must be a non-null array", key)
			}
		}
		assertDiagnostic(t, objectValue(t, packageEvidence, "diagnostic"))
		for _, rawNamespace := range arrayValue(t, packageEvidence, "namespaces") {
			namespace := rawNamespace.(map[string]any)
			assertJSONKeys(t, namespace, "namespace", "state", "expected_receipts", "observed_receipts", "missing_receipts", "unresolved_receipts", "complete", "diagnostic")
			assertDiagnostic(t, objectValue(t, namespace, "diagnostic"))
		}
		direct := objectValue(t, row, "direct")
		assertJSONKeys(t, direct, "state", "authoritative", "alternatives", "diagnostic")
		if direct["alternatives"] == nil {
			t.Error("direct.alternatives must be a non-null array")
		}
		assertDiagnostic(t, objectValue(t, direct, "diagnostic"))
		for _, rawAlternative := range arrayValue(t, direct, "alternatives") {
			alternative := rawAlternative.(map[string]any)
			assertJSONKeys(t, alternative, "kind", "identifiers", "state", "diagnostic")
			if alternative["identifiers"] == nil {
				t.Error("direct alternative identifiers must be a non-null array")
			}
			assertDiagnostic(t, objectValue(t, alternative, "diagnostic"))
		}
	}
}

func TestCollectAndMarshalStatusDocumentPreserveGoldenBytes(t *testing.T) {
	snapshot := statusJSONSnapshot(t)
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	runtime := statusJSONRuntime{
		tools:          func() []tools.Tool { return []tools.Tool{tools.NewZshTool(), tools.NewGhosttyTool()} },
		detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
		detectManager:  func() pkg.PackageManager { return manager },
		collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			return snapshot, nil
		},
	}
	document, err := collectStatusDocument(context.Background(), runtime)
	if err != nil {
		t.Fatalf("collect status document: %v", err)
	}
	encoded, err := marshalStatusDocument(document)
	if err != nil {
		t.Fatalf("marshal status document: %v", err)
	}
	standalone := append(bytes.Clone(encoded), '\n')
	publicDigest := independentStatusPublicDigest(t, standalone)
	if got, want := string(standalone), statusJSONGolden(publicDigest); got != want {
		t.Fatalf("extracted status bytes changed:\ngot:  %s\nwant: %s", got, want)
	}
	var output bytes.Buffer
	if err := writeStatusJSON(context.Background(), &output, runtime); err != nil {
		t.Fatalf("write status JSON: %v", err)
	}
	if !bytes.Equal(standalone, output.Bytes()) {
		t.Fatalf("collect/marshal bytes differ from command writer:\ncollect=%s\nwrite=%s", standalone, output.Bytes())
	}
}

func TestValidatedStatusDocumentEmbedsExactlyWithoutRecollectionOrAliases(t *testing.T) {
	expected := []string{"package-a"}
	missing := []string{"package-a"}
	identifiers := []string{"status-binary"}
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: "status-tool", Installability: health.InstallabilityUnknown,
		Package: health.PackageFacet{
			State: health.PackageMissing, Provider: "brew", ExpectedReceipts: expected, MissingReceipts: missing, Authoritative: true, Complete: true,
		},
		Direct: health.DirectFacet{State: health.ComponentMissing, Authoritative: true, Alternatives: []health.DirectAlternative{{
			Kind: health.DirectSourceBinary, Identifiers: identifiers, State: health.ComponentMissing,
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 1, Platform: "macos", Manager: "brew", Tools: []health.InstallationObservation{observation}})
	if err != nil {
		t.Fatal(err)
	}
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	collectCalls := 0
	runtime := statusJSONRuntime{
		tools:          func() []tools.Tool { return []tools.Tool{tools.NewZshTool()} },
		detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
		detectManager:  func() pkg.PackageManager { return manager },
		collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			collectCalls++
			return snapshot, nil
		},
	}
	document, err := collectStatusDocument(context.Background(), runtime)
	if err != nil {
		t.Fatal(err)
	}
	standalone, err := marshalStatusDocument(document)
	if err != nil {
		t.Fatal(err)
	}

	// Poison every caller-owned source slice after collection. The validated
	// document must retain its own redacted projection.
	expected[0], missing[0], identifiers[0] = "poison-expected", "poison-missing", "poison-binary"
	packageCopy := snapshot.Tools()[0].Package()
	packageCopy.ExpectedReceipts[0] = "poison-snapshot-copy"
	directCopy := snapshot.Tools()[0].Direct()
	directCopy.Alternatives[0].Identifiers[0] = "poison-direct-copy"

	envelope, err := json.Marshal(struct {
		Status validatedStatusDocument `json:"status"`
	}{Status: document})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Status json.RawMessage `json:"status"`
	}
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.Status, standalone) {
		t.Fatalf("embedded status differs from standalone:\nembedded=%s\nstandalone=%s", decoded.Status, standalone)
	}
	if collectCalls != 1 {
		t.Fatalf("embedding recollected status %d times", collectCalls)
	}

	expectedObject := bytes.Clone(standalone)
	standalone[0] = '['
	again, err := document.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, expectedObject) || bytes.Equal(again, standalone) {
		t.Fatalf("returned JSON bytes alias document state: again=%s mutated=%s", again, standalone)
	}
	if bytes.Contains(again, []byte("poison")) {
		t.Fatalf("source slice mutation reached validated document: %s", again)
	}
	var object map[string]any
	if err := json.Unmarshal(again, &object); err != nil {
		t.Fatal(err)
	}
	if object["schema_version"] != float64(statusSchemaVersion) || objectValue(t, object, "snapshot")["digest"] == "" {
		t.Fatalf("embedded schema/digest invalid: %#v", object)
	}
}

func TestCollectStatusDocumentCallsRuntimeOnceInExistingOrderAndMarshalDoesNotRecollect(t *testing.T) {
	snapshot := statusJSONSnapshot(t)
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	registry := []tools.Tool{tools.NewGhosttyTool(), tools.NewZshTool()}
	var sequence []string
	runtime := statusJSONRuntime{
		tools: func() []tools.Tool {
			sequence = append(sequence, "tools")
			return registry
		},
		detectPlatform: func() pkg.Platform {
			sequence = append(sequence, "platform")
			return pkg.PlatformMacOS
		},
		detectManager: func() pkg.PackageManager {
			sequence = append(sequence, "manager")
			return manager
		},
		collect: func(_ context.Context, gotTools []tools.Tool, gotManager pkg.PackageManager, gotPlatform pkg.Platform, generation uint64) (health.InstallationSnapshot, error) {
			sequence = append(sequence, "collect")
			if len(gotTools) != len(registry) || gotTools[0] != registry[0] || gotTools[1] != registry[1] || gotManager != manager || gotPlatform != pkg.PlatformMacOS || generation != 1 {
				t.Fatalf("collector boundary tools=%#v manager=%v platform=%q generation=%d", gotTools, gotManager == manager, gotPlatform, generation)
			}
			return snapshot, nil
		},
	}
	document, err := collectStatusDocument(context.Background(), runtime)
	if err != nil {
		t.Fatal(err)
	}
	wantSequence := []string{"tools", "platform", "manager", "collect"}
	if !reflect.DeepEqual(sequence, wantSequence) {
		t.Fatalf("runtime sequence=%v want=%v", sequence, wantSequence)
	}
	if _, err := marshalStatusDocument(document); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sequence, wantSequence) {
		t.Fatalf("marshal recollected runtime: %v", sequence)
	}
}

func TestStatusDocumentExtractionFailsClosedAndWriterIsSingleCall(t *testing.T) {
	if document, err := collectStatusDocument(context.Background(), statusJSONRuntime{}); !errors.Is(err, errStatusCollection) || !reflect.DeepEqual(document, validatedStatusDocument{}) {
		t.Fatalf("nil runtime document=%#v error=%v", document, err)
	}
	if encoded, err := marshalStatusDocument(validatedStatusDocument{}); !errors.Is(err, errStatusCollection) || encoded != nil {
		t.Fatalf("zero document encoded=%q error=%v", encoded, err)
	}

	snapshot := statusJSONSnapshot(t)
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	collectCalls := 0
	runtime := statusJSONRuntime{
		tools:          func() []tools.Tool { return []tools.Tool{tools.NewGhosttyTool(), tools.NewZshTool()} },
		detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
		detectManager:  func() pkg.PackageManager { return manager },
		collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			collectCalls++
			return snapshot, nil
		},
	}
	for _, writer := range []*statusTestWriter{{}, {short: true}, {err: errors.New("/Users/private token=SECRET")}} {
		collectCalls = 0
		err := writeStatusJSON(context.Background(), writer, runtime)
		if writer.short || writer.err != nil {
			if !errors.Is(err, errStatusCollection) || err.Error() != "status collection failed" {
				t.Fatalf("writer failure=%v want generic", err)
			}
		} else if err != nil {
			t.Fatalf("successful writer error=%v", err)
		}
		if collectCalls != 1 || writer.writes != 1 {
			t.Fatalf("calls collect/write=%d/%d", collectCalls, writer.writes)
		}
		if err != nil && (strings.Contains(err.Error(), "Users") || strings.Contains(err.Error(), "SECRET")) {
			t.Fatalf("writer detail leaked: %v", err)
		}
	}
}

func TestCollectStatusDocumentRawAndProvenanceFailuresReturnZeroGenericDocument(t *testing.T) {
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	base := statusJSONSnapshot(t)
	mismatch, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 2, Platform: "macos", Manager: "brew", Tools: base.Tools()})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		snapshot health.InstallationSnapshot
		err      error
	}{
		{name: "raw collector", err: errors.New("/Users/private token=SECRET")},
		{name: "zero snapshot"},
		{name: "provenance mismatch", snapshot: mismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := statusJSONRuntime{
				tools:          func() []tools.Tool { return []tools.Tool{tools.NewZshTool()} },
				detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
				detectManager:  func() pkg.PackageManager { return manager },
				collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
					return tc.snapshot, tc.err
				},
			}
			document, err := collectStatusDocument(context.Background(), runtime)
			if !errors.Is(err, errStatusCollection) || err.Error() != "status collection failed" || !reflect.DeepEqual(document, validatedStatusDocument{}) {
				t.Fatalf("document=%#v error=%v", document, err)
			}
			if strings.Contains(err.Error(), "Users") || strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("raw detail leaked: %v", err)
			}
		})
	}
}

func TestWriteStatusJSONRedactsUnsafeEvidenceBeforePublicDigest(t *testing.T) {
	firstSnapshot := statusJSONUnsafeSnapshot(t, "/Users/alice/private", "token=SECRET_A", "raw dial error secret A", true)
	secondSnapshot := statusJSONUnsafeSnapshot(t, "/Users/bob/private", "token=SECRET_B", "raw dial error secret B", true)
	publicMutation := statusJSONUnsafeSnapshot(t, "/Users/alice/private", "token=SECRET_A", "raw dial error secret A", false)

	write := func(snapshot health.InstallationSnapshot) string {
		t.Helper()
		runtime := statusJSONRuntime{
			tools:          func() []tools.Tool { return []tools.Tool{tools.NewZshTool()} },
			detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
			detectManager: func() pkg.PackageManager {
				manager := pkg.NewMockPackageManager()
				manager.ManagerName = "brew"
				return manager
			},
			collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
				return snapshot, nil
			},
		}
		var output bytes.Buffer
		if err := writeStatusJSON(context.Background(), &output, runtime); err != nil {
			t.Fatalf("write status JSON: %v", err)
		}
		return output.String()
	}

	first, second, mutated := write(firstSnapshot), write(secondSnapshot), write(publicMutation)
	for _, output := range []string{first, second, mutated} {
		for _, unsafe := range []string{"Users/alice", "Users/bob", "SECRET", "token=", "raw dial"} {
			if strings.Contains(output, unsafe) {
				t.Fatalf("status JSON exposed unsafe evidence %q: %s", unsafe, output)
			}
		}
	}
	if first != second {
		t.Fatalf("sensitive-only changes altered public status JSON:\nfirst=%s\nsecond=%s", first, second)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(first), &document); err != nil {
		t.Fatalf("decode public status JSON: %v", err)
	}
	digest, ok := objectValue(t, document, "snapshot")["digest"].(string)
	if !ok || len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
		t.Fatalf("snapshot.digest = %#v, want 64 lowercase hexadecimal characters", objectValue(t, document, "snapshot")["digest"])
	}
	var mutatedDocument map[string]any
	if err := json.Unmarshal([]byte(mutated), &mutatedDocument); err != nil {
		t.Fatalf("decode public mutation JSON: %v", err)
	}
	if want := independentStatusPublicDigest(t, []byte(first)); digest != want {
		t.Fatalf("baseline digest = %q, want independently computed %q", digest, want)
	}
	mutatedDigest, ok := objectValue(t, mutatedDocument, "snapshot")["digest"].(string)
	if !ok {
		t.Fatalf("mutated snapshot digest = %#v, want string", objectValue(t, mutatedDocument, "snapshot")["digest"])
	}
	if want := independentStatusPublicDigest(t, []byte(mutated)); mutatedDigest != want {
		t.Fatalf("mutated digest = %q, want independently computed %q", mutatedDigest, want)
	}
	if mutatedDigest == digest {
		t.Fatalf("safe public evidence mutation did not alter public digest: %v", digest)
	}

	rows := arrayValue(t, document, "tools")
	if len(rows) != 1 {
		t.Fatalf("redacted tools = %#v, want one structurally preserved tool", rows)
	}
	row := objectAt(t, rows, 0)
	if row["tool_id"] != "sensitive-tool" || row["presence"] != "partial" || row["installability"] != "unknown" || row["install_recipe_digest"] != "" {
		t.Fatalf("redaction changed typed tool identity/state: %#v", row)
	}
	packageEvidence := objectValue(t, row, "package")
	if packageEvidence["state"] != "partial" || packageEvidence["provider"] != "[redacted]" || packageEvidence["authoritative"] != true || packageEvidence["complete"] != false {
		t.Fatalf("redaction changed typed package evidence: %#v", packageEvidence)
	}
	assertRedactedArray(t, packageEvidence, "expected_receipts", 3)
	assertRedactedArray(t, packageEvidence, "observed_receipts", 1)
	assertRedactedArray(t, packageEvidence, "missing_receipts", 1)
	assertRedactedArray(t, packageEvidence, "unresolved_receipts", 1)
	assertRedactedDiagnostic(t, objectValue(t, packageEvidence, "diagnostic"), "package_batch_failed")

	namespaces := arrayValue(t, packageEvidence, "namespaces")
	if len(namespaces) != 1 {
		t.Fatalf("redacted namespaces = %#v, want one", namespaces)
	}
	namespace := objectAt(t, namespaces, 0)
	if namespace["namespace"] != "system" || namespace["state"] != "partial" || namespace["complete"] != false {
		t.Fatalf("redaction changed typed namespace evidence: %#v", namespace)
	}
	assertRedactedArray(t, namespace, "expected_receipts", 3)
	assertRedactedArray(t, namespace, "observed_receipts", 1)
	assertRedactedArray(t, namespace, "missing_receipts", 1)
	assertRedactedArray(t, namespace, "unresolved_receipts", 1)
	assertRedactedDiagnostic(t, objectValue(t, namespace, "diagnostic"), "package_batch_failed")

	direct := objectValue(t, row, "direct")
	if direct["state"] != "unknown" || direct["authoritative"] != true {
		t.Fatalf("redaction changed typed direct evidence: %#v", direct)
	}
	assertRedactedDiagnostic(t, objectValue(t, direct, "diagnostic"), "probe_failed")
	alternatives := arrayValue(t, direct, "alternatives")
	if len(alternatives) != 1 {
		t.Fatalf("redacted alternatives = %#v, want one", alternatives)
	}
	alternative := objectAt(t, alternatives, 0)
	if alternative["kind"] != "binary" || alternative["state"] != "unknown" {
		t.Fatalf("redaction changed typed direct alternative: %#v", alternative)
	}
	assertRedactedArray(t, alternative, "identifiers", 2)
	assertRedactedDiagnostic(t, objectValue(t, alternative, "diagnostic"), "probe_failed")
}

func TestWriteStatusJSONCollectorBoundaryAndManagerModes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		manager     pkg.PackageManager
		managerName string
	}{
		{name: "brew", manager: func() pkg.PackageManager {
			manager := pkg.NewMockPackageManager()
			manager.ManagerName = "brew"
			return manager
		}(), managerName: "brew"},
		{name: "manager unavailable", manager: nil, managerName: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registryTools := []tools.Tool{tools.NewGhosttyTool(), tools.NewZshTool()}
			snapshot := statusJSONMissingUnknownSnapshot(t, tc.managerName)
			toolsCalls, platformCalls, managerCalls, collectCalls := 0, 0, 0, 0
			runtime := statusJSONRuntime{
				tools: func() []tools.Tool {
					toolsCalls++
					return registryTools
				},
				detectPlatform: func() pkg.Platform {
					platformCalls++
					return pkg.PlatformMacOS
				},
				detectManager: func() pkg.PackageManager {
					managerCalls++
					return tc.manager
				},
				collect: func(_ context.Context, gotTools []tools.Tool, gotManager pkg.PackageManager, gotPlatform pkg.Platform, generation uint64) (health.InstallationSnapshot, error) {
					collectCalls++
					if len(gotTools) != len(registryTools) || gotTools[0] != registryTools[0] || gotTools[1] != registryTools[1] {
						t.Fatalf("collector tools = %#v, want exact registry identities/order %#v", gotTools, registryTools)
					}
					if gotManager != tc.manager {
						t.Fatalf("collector manager = %#v, want exact detected manager %#v", gotManager, tc.manager)
					}
					if gotPlatform != pkg.PlatformMacOS || generation != 1 {
						t.Fatalf("collector provenance = platform %q generation %d, want macos/1", gotPlatform, generation)
					}
					return snapshot, nil
				},
			}

			var output bytes.Buffer
			if err := writeStatusJSON(context.Background(), &output, runtime); err != nil {
				t.Fatalf("write missing/unknown status JSON: %v", err)
			}
			if toolsCalls != 1 || platformCalls != 1 || managerCalls != 1 || collectCalls != 1 {
				t.Fatalf("runtime calls tools/platform/manager/collect = %d/%d/%d/%d, want 1/1/1/1", toolsCalls, platformCalls, managerCalls, collectCalls)
			}
			var document map[string]any
			if err := json.Unmarshal(output.Bytes(), &document); err != nil {
				t.Fatalf("decode status JSON: %v", err)
			}
			if document["manager"] != tc.managerName {
				t.Fatalf("manager = %#v, want %q", document["manager"], tc.managerName)
			}
			rows := arrayValue(t, document, "tools")
			if len(rows) != 2 || objectAt(t, rows, 0)["presence"] != "missing" || objectAt(t, rows, 1)["presence"] != "unknown" {
				t.Fatalf("missing/unknown observations were not published: %#v", rows)
			}
		})
	}
}

func TestWriteStatusJSONCollectorFailureIsGenericAndAtomic(t *testing.T) {
	raw := errors.New("/Users/alice token=SECRET dial error")
	runtime := statusJSONRuntime{
		tools:          func() []tools.Tool { return []tools.Tool{tools.NewZshTool()} },
		detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
		detectManager:  func() pkg.PackageManager { return nil },
		collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			return health.InstallationSnapshot{}, raw
		},
	}
	var output bytes.Buffer
	err := writeStatusJSON(context.Background(), &output, runtime)
	if err == nil || err.Error() != "status collection failed" {
		t.Fatalf("error = %v, want exact generic status collection failed", err)
	}
	if output.Len() != 0 {
		t.Fatalf("fatal collection failure wrote partial stdout: %q", output.String())
	}
	if strings.Contains(err.Error(), "/Users/alice") || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "dial error") {
		t.Fatalf("fatal error leaked collector details: %q", err)
	}
}

func TestWriteStatusJSONRejectsCollectorProvenanceMismatchAndZeroSnapshot(t *testing.T) {
	brew := pkg.NewMockPackageManager()
	brew.ManagerName = "brew"
	base := statusJSONSnapshot(t)
	makeSnapshot := func(generation uint64, platform, manager string) health.InstallationSnapshot {
		t.Helper()
		snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{
			Generation: generation, Platform: platform, Manager: manager, Tools: base.Tools(),
		})
		if err != nil {
			t.Fatalf("build mismatch snapshot: %v", err)
		}
		return snapshot
	}

	for _, tc := range []struct {
		name     string
		manager  pkg.PackageManager
		snapshot health.InstallationSnapshot
	}{
		{name: "generation", manager: brew, snapshot: makeSnapshot(2, "macos", "brew")},
		{name: "platform", manager: brew, snapshot: makeSnapshot(1, "arch", "brew")},
		{name: "manager", manager: brew, snapshot: makeSnapshot(1, "macos", "apt")},
		{name: "manager unavailable", manager: nil, snapshot: makeSnapshot(1, "macos", "brew")},
		{name: "zero snapshot", manager: brew, snapshot: health.InstallationSnapshot{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rawDetail := "/Users/alice token=abc provenance mismatch " + tc.name
			runtime := statusJSONRuntime{
				tools:          func() []tools.Tool { return []tools.Tool{tools.NewZshTool()} },
				detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
				detectManager:  func() pkg.PackageManager { return tc.manager },
				collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
					return tc.snapshot, nil
				},
			}
			var output bytes.Buffer
			err := writeStatusJSON(context.Background(), &output, runtime)
			if err == nil || err.Error() != "status collection failed" {
				t.Fatalf("mismatch error = %v, want exact generic status collection failed (%s)", err, rawDetail)
			}
			if output.Len() != 0 {
				t.Fatalf("mismatch published stdout: %q", output.String())
			}
			if strings.Contains(err.Error(), "provenance") || strings.Contains(err.Error(), "alice") || strings.Contains(err.Error(), "token=") {
				t.Fatalf("mismatch error leaked details: %q", err)
			}
		})
	}

	for _, tc := range []struct {
		name     string
		manager  pkg.PackageManager
		snapshot health.InstallationSnapshot
	}{
		{name: "brew", manager: brew, snapshot: makeSnapshot(1, "macos", "brew")},
		{name: "manager unavailable", manager: nil, snapshot: makeSnapshot(1, "macos", "")},
	} {
		t.Run("valid "+tc.name, func(t *testing.T) {
			runtime := statusJSONRuntime{
				tools:          func() []tools.Tool { return []tools.Tool{tools.NewZshTool()} },
				detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
				detectManager:  func() pkg.PackageManager { return tc.manager },
				collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
					return tc.snapshot, nil
				},
			}
			var output bytes.Buffer
			if err := writeStatusJSON(context.Background(), &output, runtime); err != nil {
				t.Fatalf("valid snapshot rejected: %v", err)
			}
			var document map[string]any
			if err := json.Unmarshal(output.Bytes(), &document); err != nil {
				t.Fatalf("decode valid status: %v", err)
			}
			snapshot := objectValue(t, document, "snapshot")
			if snapshot["schema_version"] != float64(health.CurrentInstallationSchemaVersion) {
				t.Fatalf("published schema = %#v, want current %d", snapshot["schema_version"], health.CurrentInstallationSchemaVersion)
			}
			if digest, ok := snapshot["digest"].(string); !ok || digest == "" {
				t.Fatalf("published digest = %#v, want non-empty string", snapshot["digest"])
			}
		})
	}
}

func TestWriteStatusJSONFieldAwareRedaction(t *testing.T) {
	unsafeValues := []string{
		"/Users/a/x",
		`C:\Users\a\x`,
		`\\server\share`,
		"~/x",
		"../x",
		"password=hunter2",
		"api_key=abc",
		"token=abc",
		"Bearer abc",
		"Authorization: Bearer abc",
		"open /Users/alice/private: permission denied",
		`failed C:\Users\alice\x`,
		`lookup \\server\share failed`,
		"read ~/private failed",
		"request Authorization: Bearer abc failed",
		"request bearer abc failed",
		"request password=hunter2 failed",
		"request api_key=abc failed",
	}
	for _, value := range unsafeValues {
		t.Run("unsafe "+value, func(t *testing.T) {
			document := writeStatusFixtureDocument(t, statusJSONFieldFixture(t, value, value))
			assertStatusFieldValue(t, document, "[redacted]", "[redacted]")
		})
	}

	for _, value := range []string{"secret-tool", "secretary", "client-secret-helper", "@openai/codex"} {
		t.Run("safe "+value, func(t *testing.T) {
			document := writeStatusFixtureDocument(t, statusJSONFieldFixture(t, value, "probe failed"))
			assertStatusFieldValue(t, document, value, "probe failed")
			row := objectAt(t, arrayValue(t, document, "tools"), 0)
			packageEvidence := objectValue(t, row, "package")
			if objectValue(t, packageEvidence, "diagnostic")["summary"] != "probe failed" {
				t.Fatalf("safe package diagnostic was redacted: %#v", packageEvidence)
			}
			namespace := objectAt(t, arrayValue(t, packageEvidence, "namespaces"), 0)
			direct := objectValue(t, row, "direct")
			alternative := objectAt(t, arrayValue(t, direct, "alternatives"), 0)
			for _, diagnostic := range []map[string]any{
				objectValue(t, namespace, "diagnostic"), objectValue(t, direct, "diagnostic"), objectValue(t, alternative, "diagnostic"),
			} {
				if diagnostic["summary"] != "probe failed" {
					t.Fatalf("safe diagnostic was redacted: %#v", diagnostic)
				}
			}
		})
	}
}

func statusJSONFieldFixture(t *testing.T, evidence, diagnostic string) health.InstallationSnapshot {
	t.Helper()
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: "field-aware", Installability: health.InstallabilityUnknown,
		Package: health.PackageFacet{
			State: health.PackagePresent, Provider: evidence, ExpectedReceipts: []string{evidence}, ObservedReceipts: []string{evidence}, Authoritative: true, Complete: true,
			Namespaces:     []health.PackageNamespaceFacet{{Namespace: health.PackageNamespaceSystem, State: health.PackagePresent, ExpectedReceipts: []string{evidence}, ObservedReceipts: []string{evidence}, Complete: true, DiagnosticCode: health.DiagnosticProbeFailed, DiagnosticSummary: diagnostic}},
			DiagnosticCode: health.DiagnosticProbeFailed, DiagnosticSummary: diagnostic,
		},
		Direct: health.DirectFacet{
			State: health.ComponentUnknown, Authoritative: true, DiagnosticCode: health.DiagnosticProbeFailed, DiagnosticSummary: diagnostic,
			Alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{evidence}, State: health.ComponentUnknown, DiagnosticCode: health.DiagnosticProbeFailed, DiagnosticSummary: diagnostic}},
		},
	})
	if err != nil {
		t.Fatalf("build field-aware observation: %v", err)
	}
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 1, Platform: "macos", Manager: "brew", Tools: []health.InstallationObservation{observation}})
	if err != nil {
		t.Fatalf("build field-aware snapshot: %v", err)
	}
	return snapshot
}

func writeStatusFixtureDocument(t *testing.T, snapshot health.InstallationSnapshot) map[string]any {
	t.Helper()
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	runtime := statusJSONRuntime{
		tools:          func() []tools.Tool { return []tools.Tool{tools.NewZshTool()} },
		detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
		detectManager:  func() pkg.PackageManager { return manager },
		collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			return snapshot, nil
		},
	}
	var output bytes.Buffer
	if err := writeStatusJSON(context.Background(), &output, runtime); err != nil {
		t.Fatalf("write field-aware status: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode field-aware status: %v", err)
	}
	return document
}

func assertStatusFieldValue(t *testing.T, document map[string]any, wantEvidence, wantDiagnostic string) {
	t.Helper()
	row := objectAt(t, arrayValue(t, document, "tools"), 0)
	packageEvidence := objectValue(t, row, "package")
	if packageEvidence["provider"] != wantEvidence {
		t.Fatalf("provider = %#v, want %q", packageEvidence["provider"], wantEvidence)
	}
	for _, key := range []string{"expected_receipts", "observed_receipts"} {
		values := arrayValue(t, packageEvidence, key)
		if len(values) != 1 || values[0] != wantEvidence {
			t.Fatalf("package.%s = %#v, want one %q", key, values, wantEvidence)
		}
	}
	namespace := objectAt(t, arrayValue(t, packageEvidence, "namespaces"), 0)
	for _, key := range []string{"expected_receipts", "observed_receipts"} {
		values := arrayValue(t, namespace, key)
		if len(values) != 1 || values[0] != wantEvidence {
			t.Fatalf("namespace.%s = %#v, want one %q", key, values, wantEvidence)
		}
	}
	direct := objectValue(t, row, "direct")
	alternative := objectAt(t, arrayValue(t, direct, "alternatives"), 0)
	identifiers := arrayValue(t, alternative, "identifiers")
	if len(identifiers) != 1 || identifiers[0] != wantEvidence {
		t.Fatalf("direct identifiers = %#v, want one %q", identifiers, wantEvidence)
	}
	for _, diagnostic := range []map[string]any{
		objectValue(t, packageEvidence, "diagnostic"), objectValue(t, namespace, "diagnostic"), objectValue(t, direct, "diagnostic"), objectValue(t, alternative, "diagnostic"),
	} {
		if diagnostic["summary"] != wantDiagnostic {
			t.Fatalf("diagnostic summary = %#v, want %q", diagnostic["summary"], wantDiagnostic)
		}
	}
}

func statusJSONMissingUnknownSnapshot(t *testing.T, manager string) health.InstallationSnapshot {
	t.Helper()
	missing, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: "missing", Installability: health.InstallabilityUnknown,
		Package: health.PackageFacet{State: health.PackageMissing, Provider: manager, ExpectedReceipts: []string{"missing"}, MissingReceipts: []string{"missing"}, Authoritative: true, Complete: true},
		Direct:  health.DirectFacet{State: health.ComponentNotApplicable},
	})
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: "unknown", Installability: health.InstallabilityUnknown,
		Package: health.PackageFacet{State: health.PackageUnknown, Provider: manager, ExpectedReceipts: []string{"unknown"}, UnresolvedReceipts: []string{"unknown"}, Complete: false, DiagnosticCode: health.DiagnosticPackageBatchFailed, DiagnosticSummary: "typed package evidence unavailable"},
		Direct: health.DirectFacet{State: health.ComponentUnknown, Authoritative: true, Alternatives: []health.DirectAlternative{{
			Kind: health.DirectSourceBinary, Identifiers: []string{"unknown"}, State: health.ComponentUnknown, DiagnosticCode: health.DiagnosticProbeFailed, DiagnosticSummary: "typed direct evidence unavailable",
		}}, DiagnosticCode: health.DiagnosticProbeFailed, DiagnosticSummary: "typed direct evidence unavailable"},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 1, Platform: "macos", Manager: manager, Tools: []health.InstallationObservation{unknown, missing}})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func statusJSONUnsafeSnapshot(t *testing.T, privatePath, token, rawError string, directAuthoritative bool) health.InstallationSnapshot {
	t.Helper()
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: "sensitive-tool", Installability: health.InstallabilityUnknown,
		Package: health.PackageFacet{
			State: health.PackagePartial, Provider: token,
			ExpectedReceipts: []string{privatePath, token, rawError}, ObservedReceipts: []string{privatePath}, MissingReceipts: []string{token}, UnresolvedReceipts: []string{rawError},
			Authoritative: true, Complete: false, DiagnosticCode: health.DiagnosticPackageBatchFailed, DiagnosticSummary: rawError,
			Namespaces: []health.PackageNamespaceFacet{{
				Namespace: health.PackageNamespaceSystem, State: health.PackagePartial,
				ExpectedReceipts: []string{privatePath, token, rawError}, ObservedReceipts: []string{privatePath}, MissingReceipts: []string{token}, UnresolvedReceipts: []string{rawError},
				Complete: false, DiagnosticCode: health.DiagnosticPackageBatchFailed, DiagnosticSummary: token,
			}},
		},
		Direct: health.DirectFacet{
			State: health.ComponentUnknown, Authoritative: directAuthoritative, DiagnosticCode: health.DiagnosticProbeFailed, DiagnosticSummary: privatePath,
			Alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{privatePath, token}, State: health.ComponentUnknown, DiagnosticCode: health.DiagnosticProbeFailed, DiagnosticSummary: rawError}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 1, Platform: "macos", Manager: "brew", Tools: []health.InstallationObservation{observation}})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func independentStatusPublicDigest(t *testing.T, encoded []byte) string {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode status JSON for independent digest: %v", err)
	}
	snapshot := objectValue(t, document, "snapshot")
	projection := map[string]any{
		"schema_version": snapshot["schema_version"],
		"generation":     snapshot["generation"],
		"platform":       document["platform"],
		"manager":        document["manager"],
		"tools":          document["tools"],
	}
	canonical, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("marshal independent canonical public projection: %v", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}

func statusJSONGolden(publicDigest string) string {
	return `{"schema_version":1,"kind":"dotfiles.status","platform":"macos","manager":"brew","snapshot":{"schema_version":1,"generation":1,"digest":"` + publicDigest + `"},"capabilities":{"installation":"collected","config":"not_collected","service":"not_collected","auth":"not_collected"},"tools":[{"tool_id":"ghostty","presence":"partial","installability":"supported","install_recipe_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","package":{"state":"partial","provider":"brew","expected_receipts":["ghostty","ghostty-font"],"observed_receipts":["ghostty"],"missing_receipts":["ghostty-font"],"unresolved_receipts":[],"authoritative":true,"complete":true,"namespaces":[{"namespace":"cask","state":"present","expected_receipts":["ghostty"],"observed_receipts":["ghostty"],"missing_receipts":[],"unresolved_receipts":[],"complete":true,"diagnostic":{"code":"","summary":""}},{"namespace":"formula","state":"missing","expected_receipts":["ghostty-font"],"observed_receipts":[],"missing_receipts":["ghostty-font"],"unresolved_receipts":[],"complete":true,"diagnostic":{"code":"probe_failed","summary":"receipt probe returned typed negative evidence"}}],"diagnostic":{"code":"package_batch_failed","summary":"one package namespace used bounded fallback evidence"}},"direct":{"state":"unknown","authoritative":true,"alternatives":[{"kind":"app_bundle","identifiers":["Ghostty.app"],"state":"unknown","diagnostic":{"code":"probe_timeout","summary":"application lookup timed out"}}],"diagnostic":{"code":"probe_timeout","summary":"direct evidence incomplete"}}},{"tool_id":"zsh","presence":"unknown","installability":"unsupported","install_recipe_digest":"","package":{"state":"not_applicable","provider":"","expected_receipts":[],"observed_receipts":[],"missing_receipts":[],"unresolved_receipts":[],"authoritative":false,"complete":true,"namespaces":[],"diagnostic":{"code":"","summary":""}},"direct":{"state":"not_applicable","authoritative":false,"alternatives":[],"diagnostic":{"code":"","summary":""}}}]}` + "\n"
}

func statusJSONSnapshot(t *testing.T) health.InstallationSnapshot {
	t.Helper()
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ghostty, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: "ghostty", Installability: health.InstallabilitySupported, InstallRecipeDigest: digest,
		Package: health.PackageFacet{
			State: health.PackagePartial, Provider: "brew", ExpectedReceipts: []string{"ghostty", "ghostty-font"}, ObservedReceipts: []string{"ghostty"}, MissingReceipts: []string{"ghostty-font"}, Authoritative: true, Complete: true,
			Namespaces: []health.PackageNamespaceFacet{
				{Namespace: health.PackageNamespaceCask, State: health.PackagePresent, ExpectedReceipts: []string{"ghostty"}, ObservedReceipts: []string{"ghostty"}, Complete: true},
				{Namespace: health.PackageNamespaceFormula, State: health.PackageMissing, ExpectedReceipts: []string{"ghostty-font"}, MissingReceipts: []string{"ghostty-font"}, Complete: true, DiagnosticCode: health.DiagnosticProbeFailed, DiagnosticSummary: "receipt probe returned typed negative evidence"},
			},
			DiagnosticCode: health.DiagnosticPackageBatchFailed, DiagnosticSummary: "one package namespace used bounded fallback evidence",
		},
		Direct: health.DirectFacet{State: health.ComponentUnknown, Authoritative: true, Alternatives: []health.DirectAlternative{{Kind: health.DirectSourceAppBundle, Identifiers: []string{"Ghostty.app"}, State: health.ComponentUnknown, DiagnosticCode: health.DiagnosticProbeTimeout, DiagnosticSummary: "application lookup timed out"}}, DiagnosticCode: health.DiagnosticProbeTimeout, DiagnosticSummary: "direct evidence incomplete"},
	})
	if err != nil {
		t.Fatal(err)
	}
	zsh, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: "zsh", Installability: health.InstallabilityUnsupported,
		Package: health.PackageFacet{State: health.PackageNotApplicable, Complete: true},
		Direct:  health.DirectFacet{State: health.ComponentNotApplicable, Alternatives: []health.DirectAlternative{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 1, Platform: "macos", Manager: "brew", Tools: []health.InstallationObservation{zsh, ghostty}})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func assertJSONKeys(t *testing.T, value map[string]any, expected ...string) {
	t.Helper()
	if len(value) != len(expected) {
		t.Fatalf("keys = %#v, want exactly %#v", value, expected)
	}
	for _, key := range expected {
		if _, ok := value[key]; !ok {
			t.Fatalf("missing key %q in %#v", key, value)
		}
	}
}

func objectValue(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want object", key, parent[key])
	}
	return value
}

func arrayValue(t *testing.T, parent map[string]any, key string) []any {
	t.Helper()
	value, ok := parent[key].([]any)
	if !ok {
		t.Fatalf("%s = %#v, want non-null array", key, parent[key])
	}
	return value
}

func objectAt(t *testing.T, values []any, index int) map[string]any {
	t.Helper()
	value, ok := values[index].(map[string]any)
	if !ok {
		t.Fatalf("array[%d] = %#v, want object", index, values[index])
	}
	return value
}

func assertDiagnostic(t *testing.T, diagnostic map[string]any) {
	t.Helper()
	assertJSONKeys(t, diagnostic, "code", "summary")
	summary, ok := diagnostic["summary"].(string)
	if !ok || len(summary) > 256 || !utf8.ValidString(summary) {
		t.Fatalf("invalid diagnostic summary %#v", diagnostic["summary"])
	}
	for _, r := range summary {
		if unicode.IsControl(r) || r == '\u202a' || r == '\u202b' || r == '\u202c' || r == '\u202d' || r == '\u202e' || r == '\u2066' || r == '\u2067' || r == '\u2068' || r == '\u2069' {
			t.Fatalf("unsafe diagnostic summary %q", summary)
		}
	}
}

func assertRedactedArray(t *testing.T, parent map[string]any, key string, wantLength int) {
	t.Helper()
	values := arrayValue(t, parent, key)
	if len(values) != wantLength {
		t.Fatalf("%s = %#v, want %d structurally preserved entries", key, values, wantLength)
	}
	for _, value := range values {
		if value != "[redacted]" {
			t.Fatalf("%s contains non-redacted entry %#v", key, value)
		}
	}
}

func assertRedactedDiagnostic(t *testing.T, diagnostic map[string]any, wantCode string) {
	t.Helper()
	assertJSONKeys(t, diagnostic, "code", "summary")
	if diagnostic["code"] != wantCode || diagnostic["summary"] != "[redacted]" {
		t.Fatalf("redacted diagnostic = %#v, want code %q and stable marker", diagnostic, wantCode)
	}
}
