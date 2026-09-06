package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/operation"
)

func TestAdversarialSupportCommandCancellationTiming(t *testing.T) {
	t.Run("provenance cancellation preserves installation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := []string{}
		runtime := supportTestCommandRuntime(t)
		runtime.signalContext = func(context.Context) (context.Context, context.CancelFunc) { return ctx, func() {} }
		runtime.installation = func(context.Context) (validatedStatusDocument, error) {
			calls = append(calls, "installation")
			return supportTestStatus(t), nil
		}
		runtime.provenance = func(context.Context) (doctorReport, error) {
			calls = append(calls, "provenance")
			cancel()
			return supportTestDoctorReport(), nil
		}
		runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
			calls = append(calls, "operations")
			return operation.JournalSummarySet{Records: []operation.JournalSummary{}}, nil
		}
		output, exit := executeAdversarialSupportCommand(t, runtime)
		if exit == nil || exit.code != 2 || !exit.silent || !equalSupportStrings(calls, []string{"installation", "provenance"}) {
			t.Fatalf("exit=%+v calls=%v output=%s", exit, calls, output)
		}
		document := decodeAdversarialSupportDocument(t, output)
		if document.Installation.Collection != supportCollected || document.Provenance.ReasonCode != supportReasonCancelled || document.Operations.ReasonCode != supportReasonCancelled {
			t.Fatalf("cancellation shape=%+v", document)
		}
	})

	t.Run("successful operations ignore later cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runtime := supportTestCommandRuntime(t)
		runtime.signalContext = func(context.Context) (context.Context, context.CancelFunc) { return ctx, func() {} }
		runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
			cancel()
			return operation.JournalSummarySet{Records: []operation.JournalSummary{}}, nil
		}
		output, exit := executeAdversarialSupportCommand(t, runtime)
		if exit != nil {
			t.Fatalf("successful final collection rewritten by cancellation: %+v output=%s", exit, output)
		}
		if document := decodeAdversarialSupportDocument(t, output); document.Outcome != supportComplete || document.Operations.Collection != supportCollected {
			t.Fatalf("final cancellation shape=%+v", document)
		}
	})
}

func TestAdversarialSupportCommandReasonClassification(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*supportCommandRuntime)
		section    string
		collection string
		reason     string
		complete   bool
	}{
		{name: "provenance limit", mutate: func(runtime *supportCommandRuntime) {
			runtime.provenance = func(context.Context) (doctorReport, error) {
				report := supportTestDoctorReport()
				report.PATHMatches = make([]doctorExecutable, 1025)
				return report, nil
			}
		}, section: "provenance", collection: supportUnavailable, reason: supportReasonProvenanceLimit},
		{name: "provenance invalid", mutate: func(runtime *supportCommandRuntime) {
			runtime.provenance = func(context.Context) (doctorReport, error) {
				report := supportTestDoctorReport()
				report.RunningExecutable.OwnershipHint = "PRIVATE"
				return report, nil
			}
		}, section: "provenance", collection: supportUnavailable, reason: supportReasonProvenanceInvalid},
		{name: "journal not present", mutate: func(runtime *supportCommandRuntime) {
			runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
				return operation.JournalSummarySet{Records: []operation.JournalSummary{}}, operation.ErrJournalSummariesNotPresent
			}
		}, section: "operations", collection: supportNotPresent, complete: true},
		{name: "journal limit", mutate: func(runtime *supportCommandRuntime) {
			runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
				return operation.JournalSummarySet{Records: []operation.JournalSummary{}}, operation.ErrJournalSummariesLimit
			}
		}, section: "operations", collection: supportUnavailable, reason: supportReasonJournalLimit},
		{name: "journal invalid", mutate: func(runtime *supportCommandRuntime) {
			runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
				return operation.JournalSummarySet{Records: []operation.JournalSummary{}}, operation.ErrJournalSummariesInvalid
			}
		}, section: "operations", collection: supportUnavailable, reason: supportReasonJournalInvalid},
		{name: "journal unavailable", mutate: func(runtime *supportCommandRuntime) {
			runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
				return operation.JournalSummarySet{Records: []operation.JournalSummary{}}, errors.New("/Users/private token=SECRET")
			}
		}, section: "operations", collection: supportUnavailable, reason: supportReasonJournalUnavailable},
		{name: "invalid journal projection", mutate: func(runtime *supportCommandRuntime) {
			runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
				return operation.JournalSummarySet{}, nil
			}
		}, section: "operations", collection: supportUnavailable, reason: supportReasonJournalInvalid},
		{name: "returned journal cancellation", mutate: func(runtime *supportCommandRuntime) {
			runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
				return operation.JournalSummarySet{}, context.Canceled
			}
		}, section: "operations", collection: supportUnavailable, reason: supportReasonCancelled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := supportTestCommandRuntime(t)
			test.mutate(&runtime)
			output, exit := executeAdversarialSupportCommand(t, runtime)
			if test.complete {
				if exit != nil {
					t.Fatalf("complete exit=%+v", exit)
				}
			} else if exit == nil || exit.code != 2 || !exit.silent {
				t.Fatalf("partial exit=%+v", exit)
			}
			if bytes.Contains(output, []byte("SECRET")) || bytes.Contains(output, []byte("/Users")) {
				t.Fatalf("private error leaked: %s", output)
			}
			document := decodeAdversarialSupportDocument(t, output)
			var collection, reason string
			switch test.section {
			case "provenance":
				collection, reason = document.Provenance.Collection, document.Provenance.ReasonCode
			case "operations":
				collection, reason = document.Operations.Collection, document.Operations.ReasonCode
			default:
				t.Fatalf("unknown section %q", test.section)
			}
			if collection != test.collection || reason != test.reason {
				t.Fatalf("section=%s/%s want=%s/%s document=%+v", collection, reason, test.collection, test.reason, document)
			}
		})
	}
}

func TestAdversarialSupportCommandCancellationAndSignalLifecycle(t *testing.T) {
	t.Run("installation success concurrent with cancellation discards installation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		runtime := supportTestCommandRuntime(t)
		runtime.signalContext = func(context.Context) (context.Context, context.CancelFunc) { return ctx, func() {} }
		laterCalls := 0
		runtime.installation = func(context.Context) (validatedStatusDocument, error) {
			cancel()
			return supportTestStatus(t), nil
		}
		runtime.provenance = func(context.Context) (doctorReport, error) { laterCalls++; return doctorReport{}, nil }
		runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
			laterCalls++
			return operation.JournalSummarySet{}, nil
		}
		output, exit := executeAdversarialSupportCommand(t, runtime)
		if exit == nil || exit.code != 2 || !exit.silent || laterCalls != 0 {
			t.Fatalf("exit=%+v later=%d", exit, laterCalls)
		}
		document := decodeAdversarialSupportDocument(t, output)
		if document.Installation.ReasonCode != supportReasonCancelled || document.Provenance.ReasonCode != supportReasonCancelled || document.Operations.ReasonCode != supportReasonCancelled {
			t.Fatalf("installation cancellation=%+v", document)
		}
	})

	t.Run("cancellation before provenance preserves installation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		runtime := supportTestCommandRuntime(t)
		runtime.signalContext = func(context.Context) (context.Context, context.CancelFunc) { return ctx, func() {} }
		runtime.afterInstall = cancel
		laterCalls := 0
		runtime.provenance = func(context.Context) (doctorReport, error) { laterCalls++; return doctorReport{}, nil }
		runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
			laterCalls++
			return operation.JournalSummarySet{}, nil
		}
		output, exit := executeAdversarialSupportCommand(t, runtime)
		if exit == nil || exit.code != 2 || !exit.silent || laterCalls != 0 {
			t.Fatalf("exit=%+v later=%d", exit, laterCalls)
		}
		document := decodeAdversarialSupportDocument(t, output)
		if document.Installation.Collection != supportCollected || document.Provenance.ReasonCode != supportReasonCancelled || document.Operations.ReasonCode != supportReasonCancelled {
			t.Fatalf("pre-provenance cancellation=%+v", document)
		}
	})

	t.Run("cancellation before operations preserves provenance", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		runtime := supportTestCommandRuntime(t)
		runtime.signalContext = func(context.Context) (context.Context, context.CancelFunc) { return ctx, func() {} }
		runtime.afterProvenance = cancel
		operationsCalls := 0
		runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
			operationsCalls++
			return operation.JournalSummarySet{}, nil
		}
		output, exit := executeAdversarialSupportCommand(t, runtime)
		if exit == nil || exit.code != 2 || !exit.silent || operationsCalls != 0 {
			t.Fatalf("exit=%+v operations=%d", exit, operationsCalls)
		}
		document := decodeAdversarialSupportDocument(t, output)
		if document.Installation.Collection != supportCollected || document.Provenance.Collection != supportCollected || document.Operations.ReasonCode != supportReasonCancelled {
			t.Fatalf("pre-operations cancellation=%+v", document)
		}
	})

	t.Run("installation deadline cancels all sections", func(t *testing.T) {
		runtime := supportTestCommandRuntime(t)
		laterCalls := 0
		runtime.installation = func(context.Context) (validatedStatusDocument, error) {
			return validatedStatusDocument{}, context.DeadlineExceeded
		}
		runtime.provenance = func(context.Context) (doctorReport, error) { laterCalls++; return doctorReport{}, nil }
		runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
			laterCalls++
			return operation.JournalSummarySet{}, nil
		}
		output, exit := executeAdversarialSupportCommand(t, runtime)
		if exit == nil || exit.code != 2 || !exit.silent || laterCalls != 0 {
			t.Fatalf("exit=%+v later=%d", exit, laterCalls)
		}
		document := decodeAdversarialSupportDocument(t, output)
		if document.Installation.ReasonCode != supportReasonCancelled || document.Provenance.ReasonCode != supportReasonCancelled || document.Operations.ReasonCode != supportReasonCancelled {
			t.Fatalf("deadline shape=%+v", document)
		}
	})

	t.Run("journal error concurrent with cancellation is cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		runtime := supportTestCommandRuntime(t)
		runtime.signalContext = func(context.Context) (context.Context, context.CancelFunc) { return ctx, func() {} }
		runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
			cancel()
			return operation.JournalSummarySet{}, errors.New("PRIVATE")
		}
		output, exit := executeAdversarialSupportCommand(t, runtime)
		if exit == nil || exit.code != 2 || !exit.silent {
			t.Fatalf("exit=%+v", exit)
		}
		if document := decodeAdversarialSupportDocument(t, output); document.Operations.ReasonCode != supportReasonCancelled {
			t.Fatalf("journal cancellation shape=%+v", document)
		}
	})

	for _, test := range []struct {
		name   string
		mutate func(*supportCommandRuntime)
		writer io.Writer
		code   int
	}{
		{name: "complete", writer: &bytes.Buffer{}},
		{name: "partial", mutate: func(runtime *supportCommandRuntime) {
			runtime.provenance = func(context.Context) (doctorReport, error) { return doctorReport{}, errors.New("PRIVATE") }
		}, writer: &bytes.Buffer{}, code: 2},
		{name: "fatal writer", writer: supportFailWriter{}, code: 1},
		{name: "fatal nil context", mutate: func(runtime *supportCommandRuntime) {
			runtime.signalContext = func(context.Context) (context.Context, context.CancelFunc) { return nil, func() {} }
		}, writer: io.Discard, code: 1},
	} {
		t.Run("signal "+test.name, func(t *testing.T) {
			runtime := supportTestCommandRuntime(t)
			if test.mutate != nil {
				test.mutate(&runtime)
			}
			signalCalls, stopCalls := 0, 0
			if test.name != "fatal nil context" {
				runtime.signalContext = func(parent context.Context) (context.Context, context.CancelFunc) {
					signalCalls++
					return parent, func() { stopCalls++ }
				}
			} else {
				original := runtime.signalContext
				runtime.signalContext = func(parent context.Context) (context.Context, context.CancelFunc) {
					signalCalls++
					ctx, _ := original(parent)
					return ctx, func() { stopCalls++ }
				}
			}
			command := newSupportCommand(runtime)
			command.SetArgs([]string{"--json"})
			command.SetOut(test.writer)
			err := command.Execute()
			if test.code == 0 && err != nil {
				t.Fatal(err)
			}
			if test.code != 0 {
				var exit *commandExitError
				if !errors.As(err, &exit) || exit.code != test.code {
					t.Fatalf("exit=%+v error=%v", exit, err)
				}
			}
			if signalCalls != 1 || stopCalls != 1 {
				t.Fatalf("signal=%d stop=%d", signalCalls, stopCalls)
			}
		})
	}
}

func TestAdversarialSupportCommandOversizeFailsBeforeWriter(t *testing.T) {
	status := supportTestStatus(t)
	status.value.Tools[0].Package.Diagnostic.Summary = strings.Repeat("x", supportLineLimit)
	digest, err := statusPublicDigest(status.value)
	if err != nil {
		t.Fatal(err)
	}
	status.value.Snapshot.Digest = digest
	runtime := supportTestCommandRuntime(t)
	runtime.installation = func(context.Context) (validatedStatusDocument, error) { return status, nil }
	writer := &supportCountingWriter{}
	command := newSupportCommand(runtime)
	command.SetArgs([]string{"--json"})
	command.SetOut(writer)
	err = command.Execute()
	var exit *commandExitError
	if !errors.As(err, &exit) || exit.code != 1 || exit.message != supportFailedMessage || writer.calls != 0 || writer.output.Len() != 0 {
		t.Fatalf("exit=%+v calls=%d output=%q error=%v", exit, writer.calls, writer.output.Bytes(), err)
	}
}

func TestAdversarialRegisteredSupportCommandExitContract(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"support"})
	if err != nil || found != supportCmd {
		t.Fatalf("registered=%p want=%p error=%v", found, supportCmd, err)
	}
	previous := supportRuntime
	t.Cleanup(func() {
		supportRuntime = previous
		resetActualSupportCommandForTest(t)
	})
	supportRuntime = supportTestCommandRuntime(t)

	for _, test := range []struct {
		name   string
		args   []string
		writer io.Writer
		code   int
		stderr string
	}{
		{name: "complete", args: []string{"support", "--json"}, writer: &bytes.Buffer{}},
		{name: "missing json", args: []string{"support"}, writer: &bytes.Buffer{}, code: 2, stderr: supportSyntaxMessage + "\n"},
		{name: "positional", args: []string{"support", "--json", "extra"}, writer: &bytes.Buffer{}, code: 2, stderr: supportSyntaxMessage + "\n"},
		{name: "unknown flag", args: []string{"support", "--json", "--output", "x"}, writer: &bytes.Buffer{}, code: 2, stderr: supportSyntaxMessage + "\n"},
		{name: "failed writer", args: []string{"support", "--json"}, writer: supportFailWriter{}, code: 1, stderr: supportFailedMessage + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetActualSupportCommandForTest(t)
			var stderr bytes.Buffer
			code := executeRoot(test.args, test.writer, &stderr)
			if code != test.code || stderr.String() != test.stderr || strings.Contains(stderr.String(), "PRIVATE") {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
		})
	}

	supportRuntime = supportTestCommandRuntime(t)
	supportRuntime.provenance = func(context.Context) (doctorReport, error) {
		return doctorReport{}, errors.New("/Users/private token=SECRET")
	}
	resetActualSupportCommandForTest(t)
	var partialOutput, partialError bytes.Buffer
	if code := executeRoot([]string{"support", "--json"}, &partialOutput, &partialError); code != 2 || partialError.Len() != 0 {
		t.Fatalf("partial exit=%d stdout=%q stderr=%q", code, partialOutput.String(), partialError.String())
	}
	if document := decodeAdversarialSupportDocument(t, partialOutput.Bytes()); document.Outcome != supportPartial || document.Provenance.ReasonCode != supportReasonProvenanceUnavailable {
		t.Fatalf("partial root document=%+v", document)
	}
}

type supportFailWriter struct{}

func (supportFailWriter) Write([]byte) (int, error) {
	return 0, errors.New("PRIVATE writer failure")
}

func resetActualSupportCommandForTest(t *testing.T) {
	t.Helper()
	if flag := supportCmd.Flags().Lookup("json"); flag != nil {
		if err := flag.Value.Set("false"); err != nil {
			t.Fatal(err)
		}
		flag.Changed = false
	}
	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(io.Discard)
}

func executeAdversarialSupportCommand(t *testing.T, runtime supportCommandRuntime) ([]byte, *commandExitError) {
	t.Helper()
	var output bytes.Buffer
	command := newSupportCommand(runtime)
	command.SetArgs([]string{"--json"})
	command.SetOut(&output)
	err := command.Execute()
	if err == nil {
		return bytes.Clone(output.Bytes()), nil
	}
	var exit *commandExitError
	if !errors.As(err, &exit) {
		t.Fatalf("unexpected command error: %v", err)
	}
	return bytes.Clone(output.Bytes()), exit
}

func decodeAdversarialSupportDocument(t *testing.T, line []byte) supportDocument {
	t.Helper()
	if len(line) == 0 || line[len(line)-1] != '\n' || bytes.Count(line, []byte{'\n'}) != 1 {
		t.Fatalf("invalid support framing: %q", line)
	}
	var document supportDocument
	decoder := json.NewDecoder(bytes.NewReader(line))
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf("decode support output: %v", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		t.Fatalf("trailing support output: %v", err)
	}
	return document
}
