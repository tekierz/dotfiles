// Package installapply executes and verifies already-reviewed installation
// recipes without depending on terminal UI state.
package installapply

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
)

// ErrManagerIdentityChanged is the path-free authority failure returned when
// the reviewed package-manager executable is unavailable, substituted, or
// changes before a manager-backed detector or mutation.
var ErrManagerIdentityChanged = errors.New("reviewed package manager identity changed")

// ErrNPMExecutionAuthorityRequired is the fixed, path-free failure returned
// when a recipe reaches this boundary without its accepted npm identity or
// still mixes manager and npm authority classes.
var ErrNPMExecutionAuthorityRequired = errors.New("reviewed npm execution authority is required")

// RecipeExecutionAuthority carries only opaque identities retained by an
// accepted plan. NP4 can pass the npm member after its fresh phase checks;
// existing manager-only callers retain their current boundary.
type RecipeExecutionAuthority struct {
	Manager pkg.ExecutableIdentity
	NPM     pkg.NPMExecutionIdentity
}

type streamingInstallManager struct {
	pkg.PackageManager
	ctx      context.Context
	emitLine func(string)
}

func (manager *streamingInstallManager) Install(packages ...string) error {
	if err := manager.ctx.Err(); err != nil {
		return err
	}
	command, err := manager.InstallStreaming(manager.ctx, packages...)
	if err != nil || command == nil {
		return err
	}
	if err := streamOutput(manager.ctx, command, manager.emitLine); err != nil {
		return err
	}
	return waitStreaming(manager.ctx, command)
}

// ExecuteRecipe runs a reviewed manager recipe in exact step order. The
// compatibility boundary carries manager authority only; NP4 uses
// ExecuteRecipeWithAuthority for a separately reviewed npm phase.
func ExecuteRecipe(ctx context.Context, recipe operation.InstallRecipe, manager pkg.PackageManager, acceptedIdentity pkg.ExecutableIdentity, emitLine func(string)) error {
	return ExecuteRecipeWithAuthority(ctx, recipe, manager, RecipeExecutionAuthority{Manager: acceptedIdentity}, emitLine)
}

// ExecuteRecipeWithAuthority executes one already-separated recipe authority
// class. Mixed manager/npm recipes remain blocked until NP4 coordinates a
// fresh phase; a pure npm recipe may execute only with its accepted identity.
func ExecuteRecipeWithAuthority(ctx context.Context, recipe operation.InstallRecipe, manager pkg.PackageManager, authority RecipeExecutionAuthority, emitLine func(string)) error {
	if ctx == nil {
		return fmt.Errorf("reviewed install context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := operation.InstallRecipeDigest(recipe); err != nil {
		return fmt.Errorf("reviewed install recipe is invalid: %w", err)
	}
	hasNPM, hasManager := false, false
	for _, step := range recipe.Steps {
		switch step.Kind {
		case operation.InstallStepNPMGlobal:
			hasNPM = true
		case operation.InstallStepPackageManager, operation.InstallStepHomebrewCask:
			hasManager = true
		}
	}
	if hasNPM {
		if hasManager || authority.Manager != (pkg.ExecutableIdentity{}) || authority.NPM.SchemaVersion() != pkg.CurrentNPMExecutionIdentitySchemaVersion || authority.NPM.Digest() == "" {
			return ErrNPMExecutionAuthorityRequired
		}
		return ExecuteAcceptedNPMRecipe(ctx, recipe, authority.NPM, emitLine)
	}
	if authority.NPM != (pkg.NPMExecutionIdentity{}) {
		return ErrNPMExecutionAuthorityRequired
	}
	for _, step := range recipe.Steps {
		switch step.Kind {
		case operation.InstallStepPackageManager:
			if packageManagerNil(manager) || manager.Name() != step.Provider {
				return fmt.Errorf("reviewed package manager %q is unavailable", step.Provider)
			}
			if err := validateAcceptedManagerIdentity(manager, authority.Manager, false); err != nil {
				return err
			}
		case operation.InstallStepHomebrewCask:
			if step.Provider != "brew" || packageManagerNil(manager) || manager.Name() != "brew" {
				return fmt.Errorf("reviewed Homebrew cask installer is unavailable")
			}
			if _, ok := manager.(pkg.HomebrewCaskManager); !ok {
				return fmt.Errorf("reviewed Homebrew cask installer is unavailable")
			}
			if err := validateAcceptedManagerIdentity(manager, authority.Manager, false); err != nil {
				return err
			}
		case operation.InstallStepNPMGlobal:
			// Pure npm recipes return through ExecuteAcceptedNPMRecipe above;
			// mixed authority recipes are rejected before reaching this loop.
			return ErrNPMExecutionAuthorityRequired
		default:
			return fmt.Errorf("unsupported reviewed install step %q", step.Kind)
		}
	}
	for _, step := range recipe.Steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch step.Kind {
		case operation.InstallStepPackageManager:
			if err := validateAcceptedManagerIdentity(manager, authority.Manager, true); err != nil {
				return err
			}
			adapted := &streamingInstallManager{PackageManager: manager, ctx: ctx, emitLine: emitLine}
			if err := adapted.Install(step.Packages...); err != nil {
				return err
			}
		case operation.InstallStepHomebrewCask:
			if err := validateAcceptedManagerIdentity(manager, authority.Manager, true); err != nil {
				return err
			}
			command, err := manager.(pkg.HomebrewCaskManager).InstallCasksStreaming(ctx, step.Casks...)
			if err != nil {
				return err
			}
			if command != nil {
				if err := streamOutput(ctx, command, emitLine); err != nil {
					return err
				}
				if err := waitStreaming(ctx, command); err != nil {
					return err
				}
			}
		case operation.InstallStepNPMGlobal:
			// This remains a fail-closed guard if the authority partitioning
			// above is ever changed without updating manager execution.
			return ErrNPMExecutionAuthorityRequired
		}
	}
	return nil
}

// validateAcceptedManagerIdentity intentionally reports no executable path or
// digest. Revalidation immediately precedes each manager mutation, but the OS
// spawn still occurs after this check; that remaining revalidate-to-spawn race
// is explicit and is not claimed closed by this authority layer.
func validateAcceptedManagerIdentity(manager pkg.PackageManager, accepted pkg.ExecutableIdentity, revalidate bool) error {
	if packageManagerNil(manager) || accepted.SchemaVersion() != pkg.CurrentExecutableIdentitySchemaVersion || accepted.Digest() == "" {
		return ErrManagerIdentityChanged
	}
	provider, ok := manager.(pkg.ExecutableIdentityProvider)
	if !ok {
		return ErrManagerIdentityChanged
	}
	current, ok := provider.ExecutableIdentity()
	if !ok || current.SchemaVersion() != accepted.SchemaVersion() || current.Digest() != accepted.Digest() {
		return ErrManagerIdentityChanged
	}
	if revalidate && accepted.Revalidate() != nil {
		return ErrManagerIdentityChanged
	}
	return nil
}

func streamOutput(ctx context.Context, command *runner.StreamingCmd, emitLine func(string)) error {
	if command == nil || command.Output == nil || command.Done == nil {
		return fmt.Errorf("reviewed streaming command is invalid")
	}
	for {
		select {
		case <-ctx.Done():
			command.Cancel()
			return ctx.Err()
		case line, ok := <-command.Output:
			if !ok {
				return nil
			}
			if emitLine != nil {
				emitLine(line)
			}
		}
	}
}

func waitStreaming(ctx context.Context, command *runner.StreamingCmd) error {
	select {
	case <-ctx.Done():
		command.Cancel()
		return ctx.Err()
	case err, ok := <-command.Done:
		if !ok {
			return nil
		}
		return err
	}
}

// DetectRecipe verifies every reviewed postcondition without mutating state.
func DetectRecipe(recipe operation.InstallRecipe, manager pkg.PackageManager) (bool, error) {
	if _, err := operation.InstallRecipeDigest(recipe); err != nil {
		return false, fmt.Errorf("reviewed install recipe is invalid: %w", err)
	}
	switch recipe.Detector.Kind {
	case operation.InstallDetectorPackageReceipt:
		if packageManagerNil(manager) || manager.Name() != recipe.Manager {
			return false, fmt.Errorf("reviewed package manager %q is unavailable", recipe.Manager)
		}
		for _, name := range recipe.Detector.Values {
			if !manager.IsInstalled(name) {
				return false, nil
			}
		}
		return true, nil
	case operation.InstallDetectorBinary:
		for _, name := range recipe.Detector.Values {
			if filepath.Base(name) != name {
				return false, fmt.Errorf("binary detector must be a command name")
			}
			_, err := exec.LookPath(name)
			if errors.Is(err, exec.ErrNotFound) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
		}
		return true, nil
	case operation.InstallDetectorAppBundle:
		home, err := os.UserHomeDir()
		if err != nil {
			return false, err
		}
		for _, name := range recipe.Detector.Values {
			if filepath.Base(name) != name || strings.TrimSuffix(name, ".app") == "" {
				return false, fmt.Errorf("app detector must be an application name")
			}
			bundle := name
			if !strings.HasSuffix(bundle, ".app") {
				bundle += ".app"
			}
			found := false
			for _, root := range []string{"/Applications", filepath.Join(home, "Applications")} {
				info, statErr := os.Lstat(filepath.Join(root, bundle))
				if statErr == nil {
					if info.Mode()&os.ModeSymlink == 0 && info.IsDir() {
						found = true
						break
					}
					continue
				}
				if !errors.Is(statErr, os.ErrNotExist) {
					return false, statErr
				}
			}
			if !found {
				return false, nil
			}
		}
		return true, nil
	default:
		return false, fmt.Errorf("unsupported reviewed detector %q", recipe.Detector.Kind)
	}
}

func packageManagerNil(manager pkg.PackageManager) bool {
	if manager == nil {
		return true
	}
	value := reflect.ValueOf(manager)
	//nolint:exhaustive // Only nil-capable reflect kinds may be passed to IsNil.
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
