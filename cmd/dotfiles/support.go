package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"slices"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tekierz/dotfiles/internal/operation"
)

const (
	supportSyntaxMessage = "invalid support request"
	supportFailedMessage = "support collection failed"
)

type supportCommandRuntime struct {
	build           func() supportBuildInput
	installation    func(context.Context) (validatedStatusDocument, error)
	provenance      func(context.Context) (doctorReport, error)
	operations      func(context.Context) (operation.JournalSummarySet, error)
	signalContext   func(context.Context) (context.Context, context.CancelFunc)
	afterInstall    func()
	afterProvenance func()
}

func defaultSupportCommandRuntime() supportCommandRuntime {
	return supportCommandRuntime{
		build: defaultSupportBuildInput,
		installation: func(ctx context.Context) (validatedStatusDocument, error) {
			return collectStatusDocument(ctx, defaultStatusJSONRuntime())
		},
		provenance: func(ctx context.Context) (doctorReport, error) {
			return collectDoctorReport(ctx, systemDoctorDependencies(), version)
		},
		operations: operation.ReadJournalSummaries,
		signalContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
		},
	}
}

var supportRuntime = defaultSupportCommandRuntime()

func newRegisteredSupportCommand() *cobra.Command {
	return newSupportCommand(supportCommandRuntime{
		build: func() supportBuildInput {
			if supportRuntime.build == nil {
				return supportBuildInput{}
			}
			return supportRuntime.build()
		},
		installation: func(ctx context.Context) (validatedStatusDocument, error) {
			if supportRuntime.installation == nil {
				return validatedStatusDocument{}, errStatusCollection
			}
			return supportRuntime.installation(ctx)
		},
		provenance: func(ctx context.Context) (doctorReport, error) {
			if supportRuntime.provenance == nil {
				return doctorReport{}, errSupportProvenanceInvalid
			}
			return supportRuntime.provenance(ctx)
		},
		operations: func(ctx context.Context) (operation.JournalSummarySet, error) {
			if supportRuntime.operations == nil {
				return operation.JournalSummarySet{Records: []operation.JournalSummary{}}, operation.ErrJournalSummariesUnavailable
			}
			return supportRuntime.operations(ctx)
		},
		signalContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			if supportRuntime.signalContext == nil {
				return context.WithCancel(parent)
			}
			return supportRuntime.signalContext(parent)
		},
	})
}

func newSupportCommand(commandRuntime supportCommandRuntime) *cobra.Command {
	command := &cobra.Command{
		Use:           "support",
		Short:         "Print a redacted support document",
		Args:          supportNoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			jsonMode, err := command.Flags().GetBool("json")
			if err != nil || !jsonMode {
				return supportCommandExit(2, supportSyntaxMessage, false)
			}
			if commandRuntime.signalContext == nil {
				return supportCommandExit(1, supportFailedMessage, false)
			}
			ctx, stop := commandRuntime.signalContext(command.Context())
			if stop == nil {
				return supportCommandExit(1, supportFailedMessage, false)
			}
			defer stop()
			if ctx == nil || commandRuntime.build == nil || commandRuntime.installation == nil ||
				commandRuntime.provenance == nil || commandRuntime.operations == nil {
				return supportCommandExit(1, supportFailedMessage, false)
			}

			document, err := collectSupportDocument(ctx, commandRuntime)
			if err != nil {
				return supportCommandExit(1, supportFailedMessage, false)
			}
			line, err := marshalSupportDocumentLine(document)
			if err != nil || writeSupportLine(command.OutOrStdout(), line) != nil {
				return supportCommandExit(1, supportFailedMessage, false)
			}
			if document.value.Outcome == supportPartial {
				return supportCommandExit(2, "", true)
			}
			return nil
		},
	}
	command.SetFlagErrorFunc(func(*cobra.Command, error) error {
		return supportCommandExit(2, supportSyntaxMessage, false)
	})
	command.Flags().Bool("json", false, "Print a redacted, share-safe support document")
	return command
}

func collectSupportDocument(ctx context.Context, commandRuntime supportCommandRuntime) (validatedSupportDocument, error) {
	build := normalizeSupportBuild(commandRuntime.build())
	installation := unavailableSupportInstallation(supportReasonCancelled)
	provenance := unavailableSupportProvenance(supportReasonCancelled)
	operations := unavailableSupportOperations(supportReasonCancelled)

	if supportContextDone(ctx, nil) {
		return newValidatedSupportDocumentFromBuild(build, installation, provenance, operations)
	}

	status, statusErr := commandRuntime.installation(ctx)
	if supportContextDone(ctx, statusErr) {
		return newValidatedSupportDocumentFromBuild(build, installation, provenance, operations)
	}
	if statusErr != nil {
		installation = unavailableSupportInstallation(supportReasonStatusUnavailable)
	} else {
		if _, validateErr := status.MarshalJSON(); validateErr != nil {
			installation = unavailableSupportInstallation(supportReasonStatusUnavailable)
		} else {
			installation = collectedSupportInstallation(status)
		}
	}
	if commandRuntime.afterInstall != nil {
		commandRuntime.afterInstall()
	}

	if supportContextDone(ctx, nil) {
		return newValidatedSupportDocumentFromBuild(build, installation, provenance, operations)
	}
	report, provenanceErr := commandRuntime.provenance(ctx)
	if supportContextDone(ctx, provenanceErr) {
		return newValidatedSupportDocumentFromBuild(build, installation, provenance, operations)
	}
	if provenanceErr != nil {
		provenance = unavailableSupportProvenance(supportReasonProvenanceUnavailable)
	} else {
		projected, projectErr := projectSupportProvenance(report)
		switch {
		case errors.Is(projectErr, errSupportProvenanceLimit):
			provenance = unavailableSupportProvenance(supportReasonProvenanceLimit)
		case projectErr != nil:
			provenance = unavailableSupportProvenance(supportReasonProvenanceInvalid)
		default:
			provenance = collectedSupportProvenance(projected)
		}
	}
	if commandRuntime.afterProvenance != nil {
		commandRuntime.afterProvenance()
	}

	if supportContextDone(ctx, nil) {
		return newValidatedSupportDocumentFromBuild(build, installation, provenance, operations)
	}
	set, operationsErr := commandRuntime.operations(ctx)
	if operationsErr != nil && supportContextDone(ctx, operationsErr) {
		return newValidatedSupportDocumentFromBuild(build, installation, provenance, operations)
	}
	switch {
	case errors.Is(operationsErr, operation.ErrJournalSummariesNotPresent):
		operations = notPresentSupportOperations()
	case errors.Is(operationsErr, operation.ErrJournalSummariesLimit):
		operations = unavailableSupportOperations(supportReasonJournalLimit)
	case errors.Is(operationsErr, operation.ErrJournalSummariesInvalid):
		operations = unavailableSupportOperations(supportReasonJournalInvalid)
	case operationsErr != nil:
		operations = unavailableSupportOperations(supportReasonJournalUnavailable)
	default:
		projected, projectErr := projectSupportOperations(set)
		if projectErr != nil {
			operations = unavailableSupportOperations(supportReasonJournalInvalid)
		} else {
			operations = collectedSupportOperations(projected)
		}
	}
	return newValidatedSupportDocumentFromBuild(build, installation, provenance, operations)
}

func defaultSupportBuildInput() supportBuildInput {
	input := supportBuildInput{
		ProductVersion: version,
		GoVersion:      runtime.Version(),
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
	}
	if info, ok := debug.ReadBuildInfo(); ok && info != nil {
		input.BuildInfoReadable = true
		input.Settings = slices.Clone(info.Settings)
	}
	return input
}

func supportContextDone(ctx context.Context, err error) bool {
	return (ctx != nil && ctx.Err() != nil) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func supportNoArgs(_ *cobra.Command, args []string) error {
	if len(args) != 0 {
		return supportCommandExit(2, supportSyntaxMessage, false)
	}
	return nil
}

func supportCommandExit(code int, message string, silent bool) error {
	return &commandExitError{code: code, message: message, silent: silent}
}

func writeSupportLine(writer io.Writer, line []byte) error {
	if writer == nil || len(line) == 0 {
		return io.ErrClosedPipe
	}
	written, err := writer.Write(line)
	if err != nil || written != len(line) {
		return io.ErrShortWrite
	}
	return nil
}
