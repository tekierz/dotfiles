package main

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/spf13/cobra"
	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/installplan"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/planpublic"
	"github.com/tekierz/dotfiles/internal/tools"
)

var errPlanCollection = errors.New("plan collection failed")

type commandExitError struct {
	code    int
	message string
	silent  bool
}

func (err *commandExitError) Error() string {
	if err == nil || err.message == "" {
		return "command exited"
	}
	return err.message
}

type planJSONRuntime struct {
	registry       func() []tools.Tool
	detectPlatform func() pkg.Platform
	detectManager  func() pkg.PackageManager
	collect        func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error)
	describe       func(tools.Tool, tools.InstallEnvironment) (operation.InstallRecipe, error)
	capture        func() (*operation.StatePlan, error)
	now            func() time.Time
}

func defaultPlanJSONRuntime() planJSONRuntime {
	return planJSONRuntime{
		registry:       func() []tools.Tool { return tools.GetRegistry().All() },
		detectPlatform: pkg.DetectPlatform,
		detectManager:  pkg.DetectManager,
		collect:        tools.ObserveInstallationHealth,
		describe:       tools.DescribeInstall,
		capture:        operation.CaptureStatePlan,
		now:            time.Now,
	}
}

var planRuntime = defaultPlanJSONRuntime()

func newRegisteredPlanCommand() *cobra.Command {
	return newPlanCommand(planJSONRuntime{
		registry: func() []tools.Tool {
			if planRuntime.registry == nil {
				return nil
			}
			return planRuntime.registry()
		},
		detectPlatform: func() pkg.Platform {
			if planRuntime.detectPlatform == nil {
				return pkg.PlatformUnknown
			}
			return planRuntime.detectPlatform()
		},
		detectManager: func() pkg.PackageManager {
			if planRuntime.detectManager == nil {
				return nil
			}
			return planRuntime.detectManager()
		},
		collect: func(ctx context.Context, registry []tools.Tool, manager pkg.PackageManager, platform pkg.Platform, generation uint64) (health.InstallationSnapshot, error) {
			if planRuntime.collect == nil {
				return health.InstallationSnapshot{}, errPlanCollection
			}
			return planRuntime.collect(ctx, registry, manager, platform, generation)
		},
		describe: func(tool tools.Tool, environment tools.InstallEnvironment) (operation.InstallRecipe, error) {
			if planRuntime.describe == nil {
				return operation.InstallRecipe{}, errPlanCollection
			}
			return planRuntime.describe(tool, environment)
		},
		capture: func() (*operation.StatePlan, error) {
			if planRuntime.capture == nil {
				return nil, errPlanCollection
			}
			return planRuntime.capture()
		},
		now: func() time.Time {
			if planRuntime.now == nil {
				return time.Time{}
			}
			return planRuntime.now()
		},
	})
}

func newPlanCommand(runtime planJSONRuntime) *cobra.Command {
	command := &cobra.Command{
		Use:           "plan",
		Short:         "Print a deterministic installation plan",
		Args:          planNoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			jsonMode, err := command.Flags().GetBool("json")
			if err != nil || !jsonMode {
				return planSyntaxError()
			}
			selected, err := command.Flags().GetStringArray("tool")
			if err != nil {
				return planSyntaxError()
			}
			status, err := writePlanJSON(command.Context(), command.OutOrStdout(), selected, runtime)
			if err != nil {
				var exit *commandExitError
				if errors.As(err, &exit) {
					return exit
				}
				return errPlanCollection
			}
			switch status {
			case planpublic.StatusReady, planpublic.StatusNoChanges:
				return nil
			case planpublic.StatusBlocked, planpublic.StatusIntentRequired:
				return &commandExitError{code: 2, silent: true}
			default:
				return errPlanCollection
			}
		},
	}
	command.SetFlagErrorFunc(func(*cobra.Command, error) error { return planSyntaxError() })
	command.Flags().Bool("json", false, "Print a machine-readable installation plan")
	command.Flags().StringArray("tool", nil, "Select one registry tool (repeatable)")
	return command
}

func planNoArgs(_ *cobra.Command, args []string) error {
	if len(args) != 0 {
		return planSyntaxError()
	}
	return nil
}

func planSyntaxError() error {
	return &commandExitError{code: 2, message: "invalid plan request"}
}

func writePlanJSON(ctx context.Context, writer io.Writer, rawTools []string, runtime planJSONRuntime) (planpublic.Status, error) {
	session, err := installplan.PlanFresh(ctx, rawTools, installplan.FreshDependencies{
		Registry: runtime.registry, DetectPlatform: runtime.detectPlatform, DetectManager: runtime.detectManager,
		Collect: runtime.collect, DescribeInstall: runtime.describe, CaptureStatePlan: runtime.capture, Now: runtime.now,
	})
	if err != nil {
		if errors.Is(err, planpublic.ErrInvalidIntent) {
			return "", planSyntaxError()
		}
		return "", errPlanCollection
	}
	return writePublicPlan(writer, session.Result().Public())
}

func writePublicPlan(writer io.Writer, document planpublic.Document) (planpublic.Status, error) {
	if writer == nil {
		return "", errPlanCollection
	}
	encoded, err := planpublic.MarshalDocument(document)
	if err != nil {
		return "", errPlanCollection
	}
	written, err := writer.Write(encoded)
	if err != nil || written != len(encoded) {
		return "", errPlanCollection
	}
	return document.Status(), nil
}
