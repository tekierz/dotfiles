package pkg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBrewManager_Name and NeedsSudo guard the basic contract.
func TestBrewManager_Name(t *testing.T) {
	mgr := &BrewManager{}
	if mgr.Name() != "brew" {
		t.Errorf("Name() = %q, want %q", mgr.Name(), "brew")
	}
}

func TestBrewManager_NeedsSudo(t *testing.T) {
	mgr := &BrewManager{}
	if mgr.NeedsSudo() {
		t.Error("NeedsSudo() should be false for brew")
	}
}

func TestHomebrewCaskTokenValidationRejectsArgumentSmuggling(t *testing.T) {
	for _, valid := range []string{"t3-code", "font-fira-code@6", "app_1.2+beta"} {
		if !validCaskToken(valid) {
			t.Errorf("valid cask token rejected: %q", valid)
		}
	}
	for _, invalid := range []string{"", "--formula", "../t3", "tap/t3", "t3 code", "t3\tcode", "t3\ncode"} {
		if validCaskToken(invalid) {
			t.Errorf("unsafe cask token accepted: %q", invalid)
		}
	}
}

func TestBrewListInstalledUsesFormulaBatchAndParsesMultipleVersions(t *testing.T) {
	dir := t.TempDir()
	brew := filepath.Join(dir, "brew")
	script := `#!/bin/sh
if [ "$#" -ne 3 ] || [ "$1" != "list" ] || [ "$2" != "--formula" ] || [ "$3" != "--versions" ]; then
  exit 9
fi
printf 'alpha 1.0 1.1\nbeta 2.0\n'
`
	if err := os.WriteFile(brew, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	manager := newBrewManager(func(name string) (string, error) {
		if name != "brew" {
			t.Fatalf("lookup requested %q, want brew", name)
		}
		return brew, nil
	})
	packages, err := manager.ListInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 2 || packages[0].Name != "alpha" || packages[0].CurrentVersion != "1.0" || packages[1].Name != "beta" || packages[1].CurrentVersion != "2.0" {
		t.Fatalf("packages=%+v", packages)
	}
}

// TestParseBrewOutdated exercises the real parseBrewOutdated helper with
// sample `brew outdated --json=v2 --greedy` output to verify:
//   - Pinned formulae are excluded from the actionable list.
//   - Casks with empty current_version are excluded (auto-updating casks).
//   - Valid formulae and casks produce correctly-tagged Package records.
func TestParseBrewOutdated(t *testing.T) {
	raw := []byte(`{
		"formulae": [
			{
				"name": "node",
				"installed_versions": ["20.0.0"],
				"current_version": "22.0.0",
				"pinned": true
			},
			{
				"name": "ripgrep",
				"installed_versions": ["14.0.0"],
				"current_version": "14.1.0",
				"pinned": false
			}
		],
		"casks": [
			{
				"name": "auto-update-only",
				"installed_version": "1.0.0",
				"current_version": ""
			},
			{
				"name": "real-cask",
				"installed_version": "2.0.0",
				"current_version": "2.1.0"
			}
		]
	}`)

	packages, err := parseBrewOutdated(raw)
	if err != nil {
		t.Fatalf("parseBrewOutdated returned error: %v", err)
	}

	// Expect exactly 2: ripgrep (formula) + real-cask (cask).
	// node (pinned) and auto-update-only (empty version) must be absent.
	if len(packages) != 2 {
		t.Fatalf("got %d packages, want 2: %+v", len(packages), packages)
	}

	byName := make(map[string]Package, len(packages))
	for _, p := range packages {
		byName[p.Name] = p
	}

	if _, ok := byName["node"]; ok {
		t.Error("pinned formula 'node' must be excluded")
	}
	if _, ok := byName["auto-update-only"]; ok {
		t.Error("cask with empty current_version must be excluded")
	}

	rg, ok := byName["ripgrep"]
	if !ok {
		t.Fatal("ripgrep must be present")
	}
	if rg.InstalledBy != "brew" {
		t.Errorf("ripgrep InstalledBy = %q, want %q", rg.InstalledBy, "brew")
	}
	if rg.CurrentVersion != "14.0.0" || rg.LatestVersion != "14.1.0" {
		t.Errorf("ripgrep versions = %q -> %q, want 14.0.0 -> 14.1.0", rg.CurrentVersion, rg.LatestVersion)
	}
	if !rg.Outdated {
		t.Error("ripgrep Outdated should be true")
	}

	rc, ok := byName["real-cask"]
	if !ok {
		t.Fatal("real-cask must be present")
	}
	if rc.InstalledBy != "brew-cask" {
		t.Errorf("real-cask InstalledBy = %q, want %q", rc.InstalledBy, "brew-cask")
	}
	if rc.CurrentVersion != "2.0.0" || rc.LatestVersion != "2.1.0" {
		t.Errorf("real-cask versions = %q -> %q, want 2.0.0 -> 2.1.0", rc.CurrentVersion, rc.LatestVersion)
	}
}

// TestParseBrewOutdated_InvalidJSON verifies a clear error is returned for
// malformed input rather than a silent zero-value result.
func TestParseBrewOutdated_InvalidJSON(t *testing.T) {
	_, err := parseBrewOutdated([]byte(`not json`))
	if err == nil {
		t.Error("parseBrewOutdated should return an error for invalid JSON")
	}
}

// TestCheckOutdatedNonGreedy_ReliableOracleExcludesGreedyLatestCasks verifies
// the package-level contract used by reliableOutdated/recheckOutdatedNames:
// the post-batch failure oracle must query brew without --greedy so auto-update
// / :latest casks that only appear in greedy output are not counted as failures.
func TestCheckOutdatedNonGreedy_ReliableOracleExcludesGreedyLatestCasks(t *testing.T) {
	tempDir := t.TempDir()
	argsPath := filepath.Join(tempDir, "brew-args")
	fakeBrewPath := filepath.Join(tempDir, "brew")

	script := `#!/bin/sh
if [ "$1" != "outdated" ]; then
	printf 'unexpected brew command: %s\n' "$1" >&2
	exit 64
fi

printf '%s\n' "$*" > "$BREW_ARGS_FILE"

for arg in "$@"; do
	if [ "$arg" = "--greedy" ]; then
		printf '%s' "$BREW_GREEDY_JSON"
		exit 0
	fi
done

printf '%s' "$BREW_NONGREEDY_JSON"
`
	if err := os.WriteFile(fakeBrewPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake brew: %v", err)
	}

	normalFormula := `{
		"name": "ripgrep",
		"installed_versions": ["14.0.0"],
		"current_version": "14.1.0",
		"pinned": false
	}`
	normalCask := `{
		"name": "tracked-app",
		"installed_version": "2.0.0",
		"current_version": "2.1.0"
	}`
	latestOnlyGreedyCask := `{
		"name": "auto-updater-latest",
		"installed_version": "latest",
		"current_version": ":latest"
	}`

	t.Setenv("BREW_ARGS_FILE", argsPath)
	t.Setenv("BREW_NONGREEDY_JSON", `{
		"formulae": [`+normalFormula+`],
		"casks": [`+normalCask+`]
	}`)
	t.Setenv("BREW_GREEDY_JSON", `{
		"formulae": [`+normalFormula+`],
		"casks": [`+normalCask+`, `+latestOnlyGreedyCask+`]
	}`)

	mgr := newBrewManager(func(name string) (string, error) {
		if name != "brew" {
			t.Fatalf("lookup requested %q, want brew", name)
		}
		return fakeBrewPath, nil
	})
	packages, err := mgr.CheckOutdatedNonGreedy()
	if err != nil {
		t.Fatalf("CheckOutdatedNonGreedy returned error: %v", err)
	}

	argsBytes, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake brew args: %v", err)
	}
	if args := strings.TrimSpace(string(argsBytes)); args != "outdated --json=v2" {
		t.Fatalf("brew args = %q, want %q", args, "outdated --json=v2")
	}

	byName := make(map[string]Package, len(packages))
	for _, p := range packages {
		byName[p.Name] = p
	}

	if _, ok := byName["auto-updater-latest"]; ok {
		t.Fatalf(":latest cask from greedy-only output must be excluded from non-greedy oracle: %+v", packages)
	}

	rg, ok := byName["ripgrep"]
	if !ok {
		t.Fatalf("normal formula ripgrep must be included: %+v", packages)
	}
	if rg.InstalledBy != "brew" || rg.CurrentVersion != "14.0.0" || rg.LatestVersion != "14.1.0" || !rg.Outdated {
		t.Errorf("ripgrep package = %+v, want brew 14.0.0 -> 14.1.0 and Outdated=true", rg)
	}

	tracked, ok := byName["tracked-app"]
	if !ok {
		t.Fatalf("normal cask tracked-app must be included: %+v", packages)
	}
	if tracked.InstalledBy != "brew-cask" || tracked.CurrentVersion != "2.0.0" || tracked.LatestVersion != "2.1.0" || !tracked.Outdated {
		t.Errorf("tracked-app package = %+v, want brew-cask 2.0.0 -> 2.1.0 and Outdated=true", tracked)
	}
}
