package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func providerExecutableFixture(t *testing.T) ([]pkg.Package, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("PATH", dir)
	t.Setenv("PROVIDER_TEST_LOG", log)
	scripts := map[string]string{
		"brew": `#!/bin/sh
case "$1" in
outdated) printf '%s\n' '{"formulae":[{"name":"shared","installed_versions":["1"],"current_version":"2"}]}' ;;
upgrade) printf 'brew:%s\n' "$*" >> "$PROVIDER_TEST_LOG" ;;
*) exit 91 ;;
esac
`,
		"apt": `#!/bin/sh
case "$1" in
list) printf '%s\n' 'shared/stable 2 amd64 [upgradable from: 1]' ;;
*) exit 92 ;;
esac
`,
		// Never launch a privileged supervisor or forward any command. Sudo checks
		// and mutation launch both fail safely inside this fixture.
		"sudo": "#!/bin/sh\nprintf 'sudo:blocked\\n' >> \"$PROVIDER_TEST_LOG\"\nexit 1\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	packages, err := pkg.CheckAllUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 2 || packages[0].Name != "shared" || packages[1].Name != "shared" {
		t.Fatalf("discovery: %+v", packages)
	}
	return packages, log
}

func TestUpdateProviderStreamingSameNameFailureIsolation(t *testing.T) {
	withTempHome(t)
	packages, log := providerExecutableFixture(t)
	a := NewApp(true)
	a.streamingUpdateCmd(packages)
	t.Cleanup(a.teardownStream)
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case msg, ok := <-a.updateStream:
			if !ok {
				t.Fatal("stream closed without terminal result")
			}
			if !msg.done {
				continue
			}
			if len(msg.results) != 2 {
				t.Fatalf("results: %+v", msg.results)
			}
			for _, result := range msg.results {
				want := result.Package.ExecutionProvider() == pkg.ExecutionProviderBrew
				if result.Success != want {
					t.Fatalf("cross-provider result contamination: %+v", msg.results)
				}
				if !want && result.Error == nil {
					t.Fatal("APT failure lost")
				}
			}
			data, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data), "brew:upgrade shared\n") != 1 {
				t.Fatalf("targeted brew invocation: %q", data)
			}
			return
		case <-timeout.C:
			t.Fatal("update worker did not terminate")
		}
	}
}

func TestUpdateProviderSelectionAndAllRetainAuthority(t *testing.T) {
	for _, route := range []string{"selected", "all"} {
		t.Run(route, func(t *testing.T) {
			withTempHome(t)
			packages, _ := providerExecutableFixture(t)
			a := NewApp(true)
			a.updateResults = packages
			a.updateSelected = map[int]bool{1: true}
			screen := NewUpdateScreen(a.screenMgr.Context())
			key := tea.KeyMsg{Type: tea.KeyEnter}
			if route == "all" {
				key = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}
			}
			cmd := screen.handleKey(key)
			if cmd == nil {
				t.Fatal("no route command")
			}
			var got []pkg.Package
			switch msg := cmd().(type) {
			case updateStartMsg:
				got = msg.packages
			case updateSudoRequiredMsg:
				got = msg.packages
			default:
				t.Fatalf("unexpected route result %T", msg)
			}
			expected := packages
			if route == "selected" {
				expected = packages[1:]
			}
			if len(got) != len(expected) {
				t.Fatalf("route count %d want %d", len(got), len(expected))
			}
			for i, p := range got {
				if p.ExecutionProvider() != expected[i].ExecutionProvider() || p.Name != expected[i].Name {
					t.Fatalf("authority changed: %+v", got)
				}
			}
			// The discovery records must fail closed when their managers disappear.
			t.Setenv("PATH", t.TempDir())
			if msg, ok := checkSudoAndUpdateCmd(got, false)().(updateRunDoneMsg); !ok || msg.err == nil {
				t.Fatal("unavailable provider did not fail closed")
			}
		})
	}
}
