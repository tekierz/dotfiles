package installapply

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
)

type recipeManager struct {
	*pkg.MockPackageManager
	order     []string
	casks     [][]string
	onInstall func()
}

type nonClosingRecipeManager struct{ *pkg.MockPackageManager }

func (manager *nonClosingRecipeManager) InstallStreaming(context.Context, ...string) (*runner.StreamingCmd, error) {
	return &runner.StreamingCmd{Output: make(chan string), Done: make(chan error)}, nil
}

func (manager *recipeManager) InstallStreaming(_ context.Context, packages ...string) (*runner.StreamingCmd, error) {
	manager.order = append(manager.order, "package:"+packages[0])
	if manager.onInstall != nil {
		manager.onInstall()
	}
	return completedStreaming([]string{"package output"}, nil), nil
}

func (manager *recipeManager) InstallCasksStreaming(_ context.Context, casks ...string) (*runner.StreamingCmd, error) {
	manager.order = append(manager.order, "cask:"+casks[0])
	manager.casks = append(manager.casks, slices.Clone(casks))
	return completedStreaming([]string{"cask output"}, nil), nil
}

func completedStreaming(lines []string, err error) *runner.StreamingCmd {
	output := make(chan string, len(lines))
	for _, line := range lines {
		output <- line
	}
	close(output)
	done := make(chan error, 1)
	done <- err
	close(done)
	return &runner.StreamingCmd{Output: output, Done: done}
}

func reviewedExecutionRecipe(steps ...operation.InstallStep) operation.InstallRecipe {
	return operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        "reviewed-tool", Platform: "macos", Manager: "brew",
		Steps: steps, Detector: operation.InstallDetector{Kind: operation.InstallDetectorBinary, Values: []string{"reviewed-tool"}},
		Risk: "reviewed test install",
	}
}

func TestExecuteRecipePreservesStepOrderStreamingAndCaskTyping(t *testing.T) {
	manager := &recipeManager{MockPackageManager: pkg.NewMockPackageManager()}
	manager.ManagerName = "brew"
	managerIdentity, _ := applyManagerIdentity(t, "ordered-brew", "exit 0")
	if err := manager.SetExecutableIdentity(managerIdentity); err != nil {
		t.Fatal(err)
	}
	recipe := reviewedExecutionRecipe(
		operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"node"}},
		operation.InstallStep{Kind: operation.InstallStepHomebrewCask, Provider: "brew", Casks: []string{"t3-code"}},
	)
	var output []string
	if err := ExecuteRecipe(context.Background(), recipe, manager, managerIdentity, func(line string) { output = append(output, line) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manager.order, []string{"package:node", "cask:t3-code"}) || !reflect.DeepEqual(output, []string{"package output", "cask output"}) || !reflect.DeepEqual(manager.casks, [][]string{{"t3-code"}}) {
		t.Fatalf("order=%v output=%v casks=%v", manager.order, output, manager.casks)
	}

	plain := pkg.NewMockPackageManager()
	plain.ManagerName = "brew"
	if err := ExecuteRecipe(context.Background(), reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepHomebrewCask, Provider: "brew", Casks: []string{"t3-code"}}), plain, pkg.ExecutableIdentity{}, nil); err == nil {
		t.Fatal("non-cask manager accepted cask step")
	}
}

func TestExecuteRecipePreflightsProviderAndCancellationBeforeMutation(t *testing.T) {
	manager := &recipeManager{MockPackageManager: pkg.NewMockPackageManager()}
	manager.ManagerName = "brew"
	recipe := reviewedExecutionRecipe(
		operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"first"}},
		operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "apt", Packages: []string{"second"}},
	)
	if err := ExecuteRecipe(context.Background(), recipe, manager, pkg.ExecutableIdentity{}, nil); err == nil || len(manager.order) != 0 {
		t.Fatalf("mismatch error=%v mutations=%v", err, manager.order)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ExecuteRecipe(cancelled, reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"first"}}), manager, pkg.ExecutableIdentity{}, nil); !errors.Is(err, context.Canceled) || len(manager.order) != 0 {
		t.Fatalf("cancel error=%v mutations=%v", err, manager.order)
	}
}

func TestExecuteRecipeNPMOnlyRequiresAcceptedAuthorityWithoutProcessStart(t *testing.T) {
	npmPath, marker := installHostileNPM(t)
	recipe := reviewedExecutionRecipe(
		npmInstallStep("private-first-package@1.2.3"),
		npmInstallStep("private-second-package@4.5.6"),
	)
	var output []string
	err := ExecuteRecipe(context.Background(), recipe, nil, pkg.ExecutableIdentity{}, func(line string) { output = append(output, line) })
	assertNPMExecutionAuthorityRequired(t, err, npmPath, marker, "private-first-package@1.2.3", "private-second-package@4.5.6")
	if len(output) != 0 {
		t.Fatalf("blocked npm emitted output: %v", output)
	}
	assertNoNPMMarker(t, marker)
}

func TestExecuteRecipeNPMMixedWithManagerFailsBeforeEveryManagerMutation(t *testing.T) {
	for _, test := range []struct {
		name  string
		steps []operation.InstallStep
	}{
		{name: "manager-then-npm", steps: []operation.InstallStep{
			{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"node"}},
			npmInstallStep("private-package@1.2.3"),
		}},
		{name: "npm-then-manager", steps: []operation.InstallStep{
			npmInstallStep("private-package@1.2.3"),
			{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"node"}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			npmPath, marker := installHostileNPM(t)
			manager, identity := authorizedRecipeManager(t, test.name)
			err := ExecuteRecipe(context.Background(), reviewedExecutionRecipe(test.steps...), manager, identity, nil)
			assertNPMExecutionAuthorityRequired(t, err, npmPath, marker)
			if len(manager.order) != 0 {
				t.Fatalf("blocked mixed recipe called manager: %v", manager.order)
			}
			assertNoNPMMarker(t, marker)
		})
	}
}

func TestExecuteRecipeNPMMixedWithCaskFailsBeforeEveryCaskMutationOrOutput(t *testing.T) {
	for _, test := range []struct {
		name  string
		steps []operation.InstallStep
	}{
		{name: "cask-then-npm", steps: []operation.InstallStep{
			{Kind: operation.InstallStepHomebrewCask, Provider: "brew", Casks: []string{"reviewed-app"}},
			npmInstallStep("private-package@1.2.3"),
		}},
		{name: "npm-then-cask", steps: []operation.InstallStep{
			npmInstallStep("private-package@1.2.3"),
			{Kind: operation.InstallStepHomebrewCask, Provider: "brew", Casks: []string{"reviewed-app"}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			npmPath, marker := installHostileNPM(t)
			manager, identity := authorizedRecipeManager(t, test.name)
			var output []string
			err := ExecuteRecipe(context.Background(), reviewedExecutionRecipe(test.steps...), manager, identity, func(line string) { output = append(output, line) })
			assertNPMExecutionAuthorityRequired(t, err, npmPath, marker)
			if len(manager.order) != 0 || len(manager.casks) != 0 || len(output) != 0 {
				t.Fatalf("blocked mixed recipe mutated casks or emitted output: order=%v casks=%v output=%v", manager.order, manager.casks, output)
			}
			assertNoNPMMarker(t, marker)
		})
	}
}

func TestDetectRecipeExactPostconditionsAndUnsupportedFailClosed(t *testing.T) {
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	manager.InstalledPkgs["one"] = "1"
	packageRecipe := reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"one", "two"}})
	packageRecipe.Detector = operation.InstallDetector{Kind: operation.InstallDetectorPackageReceipt, Values: []string{"one", "two"}}
	if detected, err := DetectRecipe(packageRecipe, manager); err != nil || detected {
		t.Fatalf("partial package receipt detected=%v err=%v", detected, err)
	}
	manager.InstalledPkgs["two"] = "1"
	if detected, err := DetectRecipe(packageRecipe, manager); err != nil || !detected {
		t.Fatalf("complete package receipt detected=%v err=%v", detected, err)
	}
	if _, err := DetectRecipe(operation.InstallRecipe{Detector: operation.InstallDetector{Kind: operation.InstallDetectorKind("future")}}, manager); err == nil {
		t.Fatal("unsupported detector did not fail closed")
	}
	if err := ExecuteRecipe(context.Background(), reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepKind("future"), Provider: "future"}), manager, pkg.ExecutableIdentity{}, nil); err == nil {
		t.Fatal("unsupported step did not fail closed")
	}
	if err := ExecuteRecipe(context.Background(), operation.InstallRecipe{}, manager, pkg.ExecutableIdentity{}, nil); err == nil {
		t.Fatal("structurally invalid recipe crossed the apply boundary")
	}
	if detected, err := DetectRecipe(operation.InstallRecipe{}, manager); err == nil || detected {
		t.Fatalf("structurally invalid detector detected=%v err=%v", detected, err)
	}
}

func TestDetectRecipeBinaryAndAppBundleRequireExactTargetTypes(t *testing.T) {
	bin := t.TempDir()
	command := filepath.Join(bin, "reviewed-command")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	binaryRecipe := reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepNPMGlobal, Provider: "npm", Args: []string{"install", "-g", "reviewed-tool@1.0.0"}})
	binaryRecipe.Detector = operation.InstallDetector{Kind: operation.InstallDetectorBinary, Values: []string{"reviewed-command"}}
	if detected, err := DetectRecipe(binaryRecipe, nil); err != nil || !detected {
		t.Fatalf("binary detected=%v err=%v", detected, err)
	}
	badBinary := operation.CloneInstallRecipe(binaryRecipe)
	badBinary.Detector.Values = []string{"nested/reviewed-command"}
	if _, err := DetectRecipe(badBinary, nil); err == nil {
		t.Fatal("binary detector accepted a path")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	apps := filepath.Join(home, "Applications")
	if err := os.MkdirAll(apps, 0o700); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(apps, "Reviewed.app")
	if err := os.WriteFile(bundle, []byte("not a bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	appRecipe := reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepHomebrewCask, Provider: "brew", Casks: []string{"reviewed-app"}})
	appRecipe.Detector = operation.InstallDetector{Kind: operation.InstallDetectorAppBundle, Values: []string{"Reviewed.app"}}
	if detected, err := DetectRecipe(appRecipe, nil); err != nil || detected {
		t.Fatalf("plain app file detected=%v err=%v", detected, err)
	}
	if err := os.Remove(bundle); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	if detected, err := DetectRecipe(appRecipe, nil); err != nil || !detected {
		t.Fatalf("app directory detected=%v err=%v", detected, err)
	}
	if err := os.Remove(bundle); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "real-app-directory")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, bundle); err != nil {
		t.Fatal(err)
	}
	if detected, err := DetectRecipe(appRecipe, nil); err != nil || detected {
		t.Fatalf("symlink app detected=%v err=%v", detected, err)
	}
}

func TestExecuteRecipeNPMGuardPreservesValidationAndCancellationPrecedence(t *testing.T) {
	t.Run("nil-context", func(t *testing.T) {
		_, marker := installHostileNPM(t)
		var ctx context.Context
		err := ExecuteRecipe(ctx, reviewedExecutionRecipe(npmInstallStep("private-package@1.2.3")), nil, pkg.ExecutableIdentity{}, nil)
		if err == nil || errors.Is(err, ErrNPMExecutionAuthorityRequired) || !strings.Contains(err.Error(), "reviewed install context is unavailable") {
			t.Fatalf("nil context precedence error=%v", err)
		}
		assertNoNPMMarker(t, marker)
	})
	t.Run("invalid-recipe", func(t *testing.T) {
		_, marker := installHostileNPM(t)
		recipe := reviewedExecutionRecipe(npmInstallStep("private-package@1.2.3"))
		recipe.SchemaVersion = 0
		err := ExecuteRecipe(context.Background(), recipe, nil, pkg.ExecutableIdentity{}, nil)
		if err == nil || errors.Is(err, ErrNPMExecutionAuthorityRequired) || !strings.Contains(err.Error(), "reviewed install recipe is invalid") {
			t.Fatalf("invalid recipe precedence error=%v", err)
		}
		assertNoNPMMarker(t, marker)
	})
	t.Run("pre-cancelled", func(t *testing.T) {
		_, marker := installHostileNPM(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := ExecuteRecipe(ctx, reviewedExecutionRecipe(npmInstallStep("private-package@1.2.3")), nil, pkg.ExecutableIdentity{}, nil)
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrNPMExecutionAuthorityRequired) {
			t.Fatalf("pre-cancelled precedence error=%v", err)
		}
		assertNoNPMMarker(t, marker)
	})
}

func npmInstallStep(packageName string) operation.InstallStep {
	return operation.InstallStep{Kind: operation.InstallStepNPMGlobal, Provider: "npm", Args: []string{"install", "-g", packageName}}
}

func authorizedRecipeManager(t *testing.T, name string) (*recipeManager, pkg.ExecutableIdentity) {
	t.Helper()
	manager := &recipeManager{MockPackageManager: pkg.NewMockPackageManager()}
	manager.ManagerName = "brew"
	identity, _ := applyManagerIdentity(t, "mixed-"+name, "exit 0")
	if err := manager.SetExecutableIdentity(identity); err != nil {
		t.Fatal(err)
	}
	return manager, identity
}

func installHostileNPM(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	marker := filepath.Join(dir, "private-npm-mutation-marker")
	npmPath := filepath.Join(dir, "npm")
	if err := os.WriteFile(npmPath, []byte("#!/bin/sh\nprintf 'private npm output\\n'\n: > \""+marker+"\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return npmPath, marker
}

func assertNPMExecutionAuthorityRequired(t *testing.T, err error, privateValues ...string) {
	t.Helper()
	if !errors.Is(err, ErrNPMExecutionAuthorityRequired) {
		t.Fatalf("npm authority error=%v, want ErrNPMExecutionAuthorityRequired", err)
	}
	formatted := err.Error() + fmt.Sprintf(" %#v", err)
	for _, value := range privateValues {
		if value != "" && strings.Contains(formatted, value) {
			t.Fatalf("npm authority error leaks private value %q: %q", value, formatted)
		}
	}
}

func assertNoNPMMarker(t *testing.T, marker string) {
	t.Helper()
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blocked npm process created marker: %v", err)
	}
}

func TestExecuteRecipeCancellationBoundsMalformedStreamingManager(t *testing.T) {
	manager := &nonClosingRecipeManager{MockPackageManager: pkg.NewMockPackageManager()}
	manager.ManagerName = "brew"
	managerIdentity, _ := applyManagerIdentity(t, "nonclosing-brew", "exit 0")
	if err := manager.SetExecutableIdentity(managerIdentity); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	recipe := reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"node"}})
	if err := ExecuteRecipe(ctx, recipe, manager, managerIdentity, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("non-closing manager cancellation=%v", err)
	}
}

func TestRecipeBoundaryRejectsTypedNilManagerWithoutPanic(t *testing.T) {
	var manager *pkg.MockPackageManager
	recipe := reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"node"}})
	if err := ExecuteRecipe(context.Background(), recipe, manager, pkg.ExecutableIdentity{}, nil); err == nil {
		t.Fatal("typed-nil manager crossed execution boundary")
	}
	packageRecipe := operation.CloneInstallRecipe(recipe)
	packageRecipe.Detector = operation.InstallDetector{Kind: operation.InstallDetectorPackageReceipt, Values: []string{"node"}}
	if detected, err := DetectRecipe(packageRecipe, manager); err == nil || detected {
		t.Fatalf("typed-nil manager detected=%v err=%v", detected, err)
	}
}
