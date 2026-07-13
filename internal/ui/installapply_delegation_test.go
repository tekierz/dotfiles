package ui

import (
	"context"
	"reflect"
	"testing"

	"github.com/tekierz/dotfiles/internal/installapply"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func TestUIReviewedRecipeHelpersDelegateWithInstallapplyParity(t *testing.T) {
	recipe := operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        "zsh", Platform: "macos", Manager: "brew", Risk: "reviewed test install",
		Steps:    []operation.InstallStep{{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"zsh"}}},
		Detector: operation.InstallDetector{Kind: operation.InstallDetectorPackageReceipt, Values: []string{"zsh"}},
	}
	directManager, uiManager := pkg.NewMockPackageManager(), pkg.NewMockPackageManager()
	directManager.ManagerName, uiManager.ManagerName = "brew", "brew"
	if directErr := installapply.ExecuteRecipe(context.Background(), recipe, directManager, nil); directErr != nil {
		t.Fatal(directErr)
	}
	if uiErr := executeInstallRecipe(context.Background(), recipe, uiManager, nil); uiErr != nil {
		t.Fatal(uiErr)
	}
	if !reflect.DeepEqual(directManager.InstallCalls, uiManager.InstallCalls) {
		t.Fatalf("execution parity direct=%v ui=%v", directManager.InstallCalls, uiManager.InstallCalls)
	}
	directDetected, directErr := installapply.DetectRecipe(recipe, directManager)
	uiDetected, uiErr := installRecipeDetected(recipe, uiManager)
	if directDetected != uiDetected || (directErr == nil) != (uiErr == nil) {
		t.Fatalf("detection parity direct=(%v,%v) ui=(%v,%v)", directDetected, directErr, uiDetected, uiErr)
	}
}
