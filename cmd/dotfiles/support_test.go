package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/tekierz/dotfiles/internal/operation"
)

func TestSupportCommandRejectsInvalidSyntaxBeforeSignalSetup(t *testing.T) {
	tests := [][]string{
		nil,
		{"--json=false"},
		{"--json", "unexpected"},
		{"--json", "--output", "support.json"},
	}
	for _, args := range tests {
		signalCalls := 0
		runtime := supportTestCommandRuntime(t)
		runtime.signalContext = func(context.Context) (context.Context, context.CancelFunc) {
			signalCalls++
			return context.WithCancel(context.Background())
		}
		command := newSupportCommand(runtime)
		command.SetArgs(args)
		command.SetOut(io.Discard)
		err := command.Execute()
		var exit *commandExitError
		if !errors.As(err, &exit) || exit.code != 2 || exit.message != supportSyntaxMessage || signalCalls != 0 {
			t.Fatalf("args=%v exit=%+v signal calls=%d error=%v", args, exit, signalCalls, err)
		}
	}
}

func TestSupportCommandCollectsInOrderAndWritesCompleteDocumentOnce(t *testing.T) {
	trace := []string{}
	runtime := supportTestCommandRuntime(t)
	runtime.build = func() supportBuildInput {
		trace = append(trace, "build")
		return supportTestBuild()
	}
	installation := runtime.installation
	runtime.installation = func(ctx context.Context) (validatedStatusDocument, error) {
		trace = append(trace, "installation")
		return installation(ctx)
	}
	provenance := runtime.provenance
	runtime.provenance = func(ctx context.Context) (doctorReport, error) {
		trace = append(trace, "provenance")
		return provenance(ctx)
	}
	operations := runtime.operations
	runtime.operations = func(ctx context.Context) (operation.JournalSummarySet, error) {
		trace = append(trace, "operations")
		return operations(ctx)
	}

	writes := &supportCountingWriter{}
	command := newSupportCommand(runtime)
	command.SetArgs([]string{"--json"})
	command.SetOut(writes)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := trace; !equalSupportStrings(got, []string{"build", "installation", "provenance", "operations"}) {
		t.Fatalf("trace=%v", got)
	}
	if writes.calls != 1 || bytes.Count(writes.output.Bytes(), []byte{'\n'}) != 1 || !bytes.HasSuffix(writes.output.Bytes(), []byte{'\n'}) || !bytes.Contains(writes.output.Bytes(), []byte(`"outcome":"complete"`)) {
		t.Fatalf("writes=%d output=%q", writes.calls, writes.output.Bytes())
	}
}

func TestSupportCommandMapsSectionFailuresToSilentPartialDocument(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*supportCommandRuntime)
		reason string
	}{
		{name: "status", mutate: func(runtime *supportCommandRuntime) {
			runtime.installation = func(context.Context) (validatedStatusDocument, error) {
				return validatedStatusDocument{}, errors.New("private status error")
			}
		}, reason: supportReasonStatusUnavailable},
		{name: "invalid status projection", mutate: func(runtime *supportCommandRuntime) {
			runtime.installation = func(context.Context) (validatedStatusDocument, error) {
				return validatedStatusDocument{}, nil
			}
		}, reason: supportReasonStatusUnavailable},
		{name: "provenance", mutate: func(runtime *supportCommandRuntime) {
			runtime.provenance = func(context.Context) (doctorReport, error) {
				return doctorReport{}, errors.New("private doctor error")
			}
		}, reason: supportReasonProvenanceUnavailable},
		{name: "journal missing is complete", mutate: func(runtime *supportCommandRuntime) {
			runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
				return operation.JournalSummarySet{Records: []operation.JournalSummary{}}, operation.ErrJournalSummariesNotPresent
			}
		}, reason: `"collection":"not_present"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := supportTestCommandRuntime(t)
			test.mutate(&runtime)
			var output bytes.Buffer
			command := newSupportCommand(runtime)
			command.SetArgs([]string{"--json"})
			command.SetOut(&output)
			err := command.Execute()
			if test.name == "journal missing is complete" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var exit *commandExitError
				if !errors.As(err, &exit) || exit.code != 2 || !exit.silent {
					t.Fatalf("exit=%+v error=%v", exit, err)
				}
			}
			if !bytes.Contains(output.Bytes(), []byte(test.reason)) || bytes.Contains(output.Bytes(), []byte("private")) {
				t.Fatalf("output=%s", output.Bytes())
			}
		})
	}
}

func TestSupportCommandEntryCancellationKeepsBuildAndSkipsCollectors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	collectorCalls := 0
	runtime := supportTestCommandRuntime(t)
	runtime.signalContext = func(context.Context) (context.Context, context.CancelFunc) {
		return ctx, func() {}
	}
	runtime.installation = func(context.Context) (validatedStatusDocument, error) {
		collectorCalls++
		return validatedStatusDocument{}, nil
	}
	runtime.provenance = func(context.Context) (doctorReport, error) {
		collectorCalls++
		return doctorReport{}, nil
	}
	runtime.operations = func(context.Context) (operation.JournalSummarySet, error) {
		collectorCalls++
		return operation.JournalSummarySet{}, nil
	}
	var output bytes.Buffer
	command := newSupportCommand(runtime)
	command.SetArgs([]string{"--json"})
	command.SetOut(&output)
	err := command.Execute()
	var exit *commandExitError
	if !errors.As(err, &exit) || exit.code != 2 || !exit.silent || collectorCalls != 0 {
		t.Fatalf("exit=%+v calls=%d error=%v", exit, collectorCalls, err)
	}
	if bytes.Count(output.Bytes(), []byte(supportReasonCancelled)) != 3 {
		t.Fatalf("cancelled output=%s", output.Bytes())
	}
}

func TestSupportCommandRejectsShortWriteWithGenericFatalError(t *testing.T) {
	runtime := supportTestCommandRuntime(t)
	command := newSupportCommand(runtime)
	command.SetArgs([]string{"--json"})
	command.SetOut(supportShortWriter{})
	err := command.Execute()
	var exit *commandExitError
	if !errors.As(err, &exit) || exit.code != 1 || exit.message != supportFailedMessage || exit.silent {
		t.Fatalf("exit=%+v error=%v", exit, err)
	}
}

func supportTestCommandRuntime(t *testing.T) supportCommandRuntime {
	t.Helper()
	return supportCommandRuntime{
		build: func() supportBuildInput { return supportTestBuild() },
		installation: func(context.Context) (validatedStatusDocument, error) {
			return supportTestStatus(t), nil
		},
		provenance: func(context.Context) (doctorReport, error) {
			return supportTestDoctorReport(), nil
		},
		operations: func(context.Context) (operation.JournalSummarySet, error) {
			return operation.JournalSummarySet{Records: []operation.JournalSummary{}}, nil
		},
		signalContext: context.WithCancel,
	}
}

type supportCountingWriter struct {
	calls  int
	output bytes.Buffer
}

func (writer *supportCountingWriter) Write(value []byte) (int, error) {
	writer.calls++
	return writer.output.Write(value)
}

type supportShortWriter struct{}

func (supportShortWriter) Write(value []byte) (int, error) {
	if len(value) == 0 {
		return 0, nil
	}
	return len(value) - 1, nil
}

func equalSupportStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
