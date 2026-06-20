package pkg

import (
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
