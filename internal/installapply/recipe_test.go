package installapply

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	lines     []string
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
	recipe := reviewedExecutionRecipe(
		operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"node"}},
		operation.InstallStep{Kind: operation.InstallStepHomebrewCask, Provider: "brew", Casks: []string{"t3-code"}},
	)
	var output []string
	if err := ExecuteRecipe(context.Background(), recipe, manager, func(line string) { output = append(output, line) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manager.order, []string{"package:node", "cask:t3-code"}) || !reflect.DeepEqual(output, []string{"package output", "cask output"}) || !reflect.DeepEqual(manager.casks, [][]string{{"t3-code"}}) {
		t.Fatalf("order=%v output=%v casks=%v", manager.order, output, manager.casks)
	}

	plain := pkg.NewMockPackageManager()
	plain.ManagerName = "brew"
	if err := ExecuteRecipe(context.Background(), reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepHomebrewCask, Provider: "brew", Casks: []string{"t3-code"}}), plain, nil); err == nil {
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
	if err := ExecuteRecipe(context.Background(), recipe, manager, nil); err == nil || len(manager.order) != 0 {
		t.Fatalf("mismatch error=%v mutations=%v", err, manager.order)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ExecuteRecipe(cancelled, reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"first"}}), manager, nil); !errors.Is(err, context.Canceled) || len(manager.order) != 0 {
		t.Fatalf("cancel error=%v mutations=%v", err, manager.order)
	}
}

func TestExecuteRecipeNPMUsesExactArgvWithoutShell(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args")
	marker := filepath.Join(dir, "pwned")
	npm := filepath.Join(dir, "npm")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARG_LOG\"\nprintf 'npm output\\n'\n"
	if err := os.WriteFile(npm, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("ARG_LOG", logPath)
	args := []string{"install", "-g", "package;touch " + marker}
	var output []string
	if err := ExecuteRecipe(context.Background(), reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepNPMGlobal, Provider: "npm", Args: args}), nil, func(line string) { output = append(output, line) }); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(logged) != "install\n-g\npackage;touch "+marker+"\n" || !reflect.DeepEqual(output, []string{"npm output"}) {
		t.Fatalf("argv=%q output=%v", logged, output)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("npm arguments reached a shell: %v", err)
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
	if err := ExecuteRecipe(context.Background(), reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepKind("future"), Provider: "future"}), manager, nil); err == nil {
		t.Fatal("unsupported step did not fail closed")
	}
	if err := ExecuteRecipe(context.Background(), operation.InstallRecipe{}, manager, nil); err == nil {
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

func TestExecuteRecipeResolvesNPMAtReviewedStepAndCancelsRunningProcess(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	npm := filepath.Join(dir, "npm")
	manager := &recipeManager{MockPackageManager: pkg.NewMockPackageManager()}
	manager.ManagerName = "brew"
	manager.onInstall = func() {
		if err := os.WriteFile(npm, []byte("#!/bin/sh\nprintf 'after prerequisite\\n'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	recipe := reviewedExecutionRecipe(
		operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"node"}},
		operation.InstallStep{Kind: operation.InstallStepNPMGlobal, Provider: "npm", Args: []string{"install", "-g", "pi@1.2.3"}},
	)
	var output []string
	if err := ExecuteRecipe(context.Background(), recipe, manager, func(line string) { output = append(output, line) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manager.order, []string{"package:node"}) || !reflect.DeepEqual(output, []string{"package output", "after prerequisite"}) {
		t.Fatalf("order=%v output=%v", manager.order, output)
	}

	if err := os.WriteFile(npm, []byte("#!/bin/sh\nprintf 'started\\n'\nwhile :; do :; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	npmOnly := reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepNPMGlobal, Provider: "npm", Args: []string{"install", "-g", "pi@1.2.3"}})
	if err := ExecuteRecipe(ctx, npmOnly, nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("running npm cancellation=%v", err)
	}
}

func TestExecuteRecipeCancellationBoundsMalformedStreamingManager(t *testing.T) {
	manager := &nonClosingRecipeManager{MockPackageManager: pkg.NewMockPackageManager()}
	manager.ManagerName = "brew"
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	recipe := reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"node"}})
	if err := ExecuteRecipe(ctx, recipe, manager, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("non-closing manager cancellation=%v", err)
	}
}

func TestRecipeBoundaryRejectsTypedNilManagerWithoutPanic(t *testing.T) {
	var manager *pkg.MockPackageManager
	recipe := reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"node"}})
	if err := ExecuteRecipe(context.Background(), recipe, manager, nil); err == nil {
		t.Fatal("typed-nil manager crossed execution boundary")
	}
	packageRecipe := operation.CloneInstallRecipe(recipe)
	packageRecipe.Detector = operation.InstallDetector{Kind: operation.InstallDetectorPackageReceipt, Values: []string{"node"}}
	if detected, err := DetectRecipe(packageRecipe, manager); err == nil || detected {
		t.Fatalf("typed-nil manager detected=%v err=%v", detected, err)
	}
}
