package installapply

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

var (
	ErrNPMExecutionAuthorityChanged = errors.New("reviewed npm execution authority changed")
	ErrNPMRecipeInvalid             = errors.New("reviewed npm recipe is not executable")
	ErrNPMExecutionFailed           = errors.New("reviewed npm execution failed")
	ErrNPMPostconditionFailed       = errors.New("reviewed npm postcondition failed")
)

// ExecuteAcceptedNPMRecipe executes only a pure, reviewed npm recipe through
// its opaque Node/npm identity. The identity owns exact executable selection,
// sanitized environment, fixed cwd, bounded output, cancellation, and process
// cleanup; this boundary owns exact recipe grammar and postcondition checking.
func ExecuteAcceptedNPMRecipe(ctx context.Context, recipe operation.InstallRecipe, identity pkg.NPMExecutionIdentity, emitLine func(string)) error {
	if ctx == nil {
		return fmt.Errorf("reviewed install context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	recipe = operation.CloneInstallRecipe(recipe)
	if _, err := operation.InstallRecipeDigest(recipe); err != nil || !validAcceptedNPMRecipe(recipe) {
		return ErrNPMRecipeInvalid
	}
	if identity.SchemaVersion() != pkg.CurrentNPMExecutionIdentitySchemaVersion || identity.Digest() == "" || identity.Revalidate() != nil {
		return ErrNPMExecutionAuthorityChanged
	}

	for _, step := range recipe.Steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		command, err := identity.StartStreaming(ctx, append([]string(nil), step.Args...)...)
		if err != nil {
			return npmExecutionError(err)
		}
		if err := consumeAcceptedNPMStream(ctx, command, emitLine); err != nil {
			return npmExecutionError(err)
		}
		if identity.Revalidate() != nil {
			return ErrNPMExecutionAuthorityChanged
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	detected, err := DetectRecipe(operation.CloneInstallRecipe(recipe), nil)
	if err != nil || !detected {
		return ErrNPMPostconditionFailed
	}
	return nil
}

func consumeAcceptedNPMStream(ctx context.Context, command *pkg.NPMStreamingCommand, emitLine func(string)) error {
	if command == nil || command.Output() == nil {
		return ErrNPMExecutionFailed
	}
	for {
		select {
		case <-ctx.Done():
			command.Cancel()
			_ = command.Wait()
			return ctx.Err()
		case line, ok := <-command.Output():
			if !ok {
				return command.Wait()
			}
			if emitLine != nil {
				emitLine(line)
			}
		}
	}
}

func npmExecutionError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, ErrNPMExecutionAuthorityChanged):
		return ErrNPMExecutionAuthorityChanged
	default:
		return ErrNPMExecutionFailed
	}
}

func validAcceptedNPMRecipe(recipe operation.InstallRecipe) bool {
	if len(recipe.Steps) == 0 || len(recipe.Steps) > 16 || recipe.Detector.Kind != operation.InstallDetectorBinary ||
		len(recipe.Detector.Values) == 0 || len(recipe.Detector.Values) > 32 {
		return false
	}
	for _, detector := range recipe.Detector.Values {
		if !validNPMDetectorName(detector) {
			return false
		}
	}
	for _, step := range recipe.Steps {
		if step.Kind != operation.InstallStepNPMGlobal || step.Provider != "npm" || len(step.Packages) != 0 || len(step.Casks) != 0 || !validNPMGlobalArguments(step.Args) {
			return false
		}
	}
	return true
}

func validNPMGlobalArguments(arguments []string) bool {
	if len(arguments) != 3 && len(arguments) != 4 || arguments[0] != "install" || arguments[1] != "-g" {
		return false
	}
	packageIndex := 2
	if len(arguments) == 4 {
		if arguments[2] != "--ignore-scripts" {
			return false
		}
		packageIndex = 3
	}
	return validNPMRecipeToken(arguments[packageIndex]) && !strings.HasPrefix(arguments[packageIndex], "-")
}

func validNPMDetectorName(value string) bool {
	if value == "" || len(value) > 128 || filepath.Base(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "-") {
		return false
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("-._", character) {
			continue
		}
		return false
	}
	return true
}

func validNPMRecipeToken(value string) bool {
	if value == "" || len(value) > 256 || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") || strings.HasPrefix(value, ".") || strings.Contains(value, "\\") {
		return false
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"authorization", "bearer ", "sk-proj", "api_key", "api-key", "token=", "password=", "secret=", "credential=", "file:"} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
		for _, character := range segment {
			if unicode.IsControl(character) || unicode.In(character, unicode.Cf) {
				return false
			}
			if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("@._+-=", character) {
				continue
			}
			return false
		}
	}
	return true
}
