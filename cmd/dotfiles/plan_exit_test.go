package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestRegisteredPlanCommandAndExecuteRootSyntaxContract(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"plan"})
	if err != nil || found != planCmd {
		t.Fatalf("registered plan command=%p want=%p err=%v", found, planCmd, err)
	}
	for _, args := range [][]string{{"plan"}, {"plan", "--json", "positional"}, {"plan", "--json", "--tool", "Zsh"}, {"plan", "--unknown"}} {
		resetActualPlanCommandForTest(t)
		var stdout, stderr bytes.Buffer
		if code := executeRoot(args, &stdout, &stderr); code != 2 {
			t.Fatalf("syntax %v exit=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
		if stdout.Len() != 0 || stderr.Len() == 0 || strings.Contains(stderr.String(), "Zsh") {
			t.Fatalf("syntax %v stdout=%q stderr=%q", args, stdout.String(), stderr.String())
		}
	}
}

func TestExecuteRootPlanIntentRequiredIsSilentTwoAfterOneJSONWrite(t *testing.T) {
	previous := planRuntime
	planRuntime = planJSONRuntime{}
	t.Cleanup(func() { planRuntime = previous; resetActualPlanCommandForTest(t) })
	resetActualPlanCommandForTest(t)
	var stdout, stderr bytes.Buffer
	if code := executeRoot([]string{"plan", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("intent_required exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 || !bytes.Contains(stdout.Bytes(), []byte(`"status":"intent_required"`)) || bytes.Count(stdout.Bytes(), []byte("\n")) != 1 {
		t.Fatalf("intent_required stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestExecuteRootPlanFatalCollectionIsGenericOne(t *testing.T) {
	previous := planRuntime
	planRuntime = planJSONRuntime{
		registry:       func() []tools.Tool { return []tools.Tool{tools.NewZshTool()} },
		detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
		detectManager: func() pkg.PackageManager {
			manager := pkg.NewMockPackageManager()
			manager.ManagerName = "brew"
			return manager
		},
		collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			return health.InstallationSnapshot{}, errors.New("/Users/private token=SECRET")
		},
	}
	t.Cleanup(func() { planRuntime = previous; resetActualPlanCommandForTest(t) })
	resetActualPlanCommandForTest(t)
	var stdout, stderr bytes.Buffer
	if code := executeRoot([]string{"plan", "--json", "--tool", "zsh"}, &stdout, &stderr); code != 1 {
		t.Fatalf("fatal exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 || stderr.String() != "plan collection failed\n" || strings.Contains(stderr.String(), "SECRET") {
		t.Fatalf("fatal stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestExecuteRootPlanReadyAndUnknownUseShippedExits(t *testing.T) {
	for _, test := range []struct {
		name      string
		selection string
		wantCode  int
		status    string
	}{
		{name: "ready", selection: "zsh", status: "ready"},
		{name: "unknown", selection: "unknown-tool", wantCode: 2, status: "blocked"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, _ := planRuntimeFixture(t, health.PresenceMissing)
			previous := planRuntime
			planRuntime = runtime
			t.Cleanup(func() { planRuntime = previous; resetActualPlanCommandForTest(t) })
			resetActualPlanCommandForTest(t)
			var stdout, stderr bytes.Buffer
			code := executeRoot([]string{"plan", "--json", "--tool", test.selection}, &stdout, &stderr)
			if code != test.wantCode || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"status":"`+test.status+`"`) {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestExecuteRootKeepsExistingNonPlanErrorsAtOne(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := executeRoot([]string{"definitely-not-a-command"}, &stdout, &stderr); code != 1 || stderr.Len() == 0 {
		t.Fatalf("non-plan exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func resetActualPlanCommandForTest(t *testing.T) {
	t.Helper()
	if flag := planCmd.Flags().Lookup("json"); flag != nil {
		if err := flag.Value.Set("false"); err != nil {
			t.Fatal(err)
		}
		flag.Changed = false
	}
	if flag := planCmd.Flags().Lookup("tool"); flag != nil {
		replacer, ok := flag.Value.(interface{ Replace([]string) error })
		if !ok {
			t.Fatal("plan --tool flag cannot be reset")
		}
		if err := replacer.Replace(nil); err != nil {
			t.Fatal(err)
		}
		flag.Changed = false
	}
	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(io.Discard)
}
