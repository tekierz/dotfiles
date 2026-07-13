package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strings"
	"syscall"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/tekierz/dotfiles/internal/installapply"
	"github.com/tekierz/dotfiles/internal/installplan"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/planpublic"
)

const (
	applySyntaxMessage       = "invalid apply request"
	applyHashMessage         = "installation plan changed; run dotfiles plan --json again"
	applyNotReadyMessage     = "installation plan is not ready; run dotfiles plan --json again"
	applyCancelledMessage    = "installation apply cancelled"
	applyFailedMessage       = "installation apply failed"
	applyOutputFailedMessage = "installation applied but result output failed"
)

type applyCommandRuntime struct {
	apply         func(context.Context, installapply.Request) (installapply.Result, error)
	signalContext func(context.Context) (context.Context, context.CancelFunc)
}

func defaultApplyCommandRuntime() applyCommandRuntime {
	return applyCommandRuntime{
		apply: func(ctx context.Context, request installapply.Request) (installapply.Result, error) {
			return installapply.Apply(ctx, request, installapply.SystemDependencies(func(ctx context.Context, rawTools []string) (installplan.FreshSession, error) {
				return installplan.PlanFresh(ctx, rawTools, currentPlanFreshDependencies())
			}))
		},
		signalContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
		},
	}
}

func currentPlanFreshDependencies() installplan.FreshDependencies {
	return installplan.FreshDependencies{
		Registry: planRuntime.registry, DetectPlatform: planRuntime.detectPlatform, DetectManager: planRuntime.detectManager,
		Collect: planRuntime.collect, DescribeInstall: planRuntime.describe, CaptureStatePlan: planRuntime.capture, Now: planRuntime.now,
	}
}

var applyRuntime = defaultApplyCommandRuntime()

func newRegisteredApplyCommand() *cobra.Command {
	return newApplyCommand(applyCommandRuntime{
		apply: func(ctx context.Context, request installapply.Request) (installapply.Result, error) {
			if applyRuntime.apply == nil {
				return installapply.Result{}, installapply.ErrApplyFailed
			}
			return applyRuntime.apply(ctx, request)
		},
		signalContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			if applyRuntime.signalContext == nil {
				return context.WithCancel(parent)
			}
			return applyRuntime.signalContext(parent)
		},
	})
}

func newApplyCommand(runtime applyCommandRuntime) *cobra.Command {
	command := &cobra.Command{
		Use:           "apply",
		Short:         "Apply an exact freshly reviewed installation plan",
		Args:          applyNoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			yes, yesErr := command.Flags().GetBool("yes")
			hash, hashErr := command.Flags().GetString("plan-hash")
			rawTools, toolsErr := command.Flags().GetStringArray("tool")
			tools, normalizeErr := canonicalApplyTools(rawTools)
			if yesErr != nil || hashErr != nil || toolsErr != nil || !yes || !validApplyCommandHash(hash) || normalizeErr != nil || len(tools) == 0 {
				return applyCommandExit(2, applySyntaxMessage)
			}
			if runtime.apply == nil || runtime.signalContext == nil {
				return applyCommandExit(1, applyFailedMessage)
			}
			ctx, stop := runtime.signalContext(command.Context())
			if stop == nil {
				return applyCommandExit(1, applyFailedMessage)
			}
			defer stop()
			if ctx == nil {
				return applyCommandExit(1, applyFailedMessage)
			}
			result, err := runtime.apply(ctx, installapply.Request{RawTools: tools, ExpectedHash: hash})
			if err != nil {
				switch {
				case errors.Is(err, context.Canceled):
					return applyCommandExit(130, applyCancelledMessage)
				case errors.Is(err, installapply.ErrInvalidApplyRequest):
					return applyCommandExit(2, applySyntaxMessage)
				case errors.Is(err, installapply.ErrPlanHashMismatch):
					return applyCommandExit(2, applyHashMessage)
				case errors.Is(err, installapply.ErrPlanNotReady):
					return applyCommandExit(2, applyNotReadyMessage)
				default:
					return applyCommandExit(1, applyFailedMessage)
				}
			}
			if !validApplySuccess(result, hash) {
				return applyCommandExit(1, applyFailedMessage)
			}
			line := fmt.Sprintf("installation applied: operation=%s plan_hash=%s succeeded=%d failed=0\n", result.OperationID, result.PlanHash, result.Succeeded)
			if err := writeApplyResult(command.OutOrStdout(), []byte(line)); err != nil {
				return applyCommandExit(1, applyOutputFailedMessage)
			}
			return nil
		},
	}
	command.SetFlagErrorFunc(func(*cobra.Command, error) error { return applyCommandExit(2, applySyntaxMessage) })
	command.Flags().Bool("yes", false, "Apply noninteractively using the exact reviewed plan hash")
	command.Flags().String("plan-hash", "", "Exact plan hash from a fresh plan --json result")
	command.Flags().StringArray("tool", nil, "Select one reviewed registry tool (repeatable)")
	return command
}

func applyNoArgs(_ *cobra.Command, args []string) error {
	if len(args) != 0 {
		return applyCommandExit(2, applySyntaxMessage)
	}
	return nil
}

func applyCommandExit(code int, message string) error {
	return &commandExitError{code: code, message: message}
}

func canonicalApplyTools(raw []string) ([]string, error) {
	known := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		id := strings.TrimSpace(value)
		if id == "" {
			return nil, planpublic.ErrInvalidIntent
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			known = append(known, id)
		}
	}
	sort.Strings(known)
	intent, err := planpublic.NormalizeExplicitTools(raw, known)
	if err != nil {
		return nil, err
	}
	return slices.Clone(intent.Tools), nil
}

func validApplyCommandHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == value
}

func validApplySuccess(result installapply.Result, expectedHash string) bool {
	return result.Status == operation.StatusSucceeded && result.PlanHash == expectedHash && result.Succeeded > 0 && result.Failed == 0 && validPublicOperationID(result.OperationID)
}

func validPublicOperationID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r > unicode.MaxASCII || !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '.') {
			return false
		}
	}
	return true
}

func writeApplyResult(writer io.Writer, output []byte) error {
	if writer == nil {
		return io.ErrClosedPipe
	}
	written, err := writer.Write(output)
	if err != nil {
		return err
	}
	if written != len(output) {
		return io.ErrShortWrite
	}
	return nil
}
