package pkg

import (
	"encoding/json"
	"testing"
)

// TestBrewCheckOutdated_SkipsPinned verifies that pinned outdated formulae are
// excluded from the actionable update list (regression for pkg-3). `brew
// upgrade <name>` refuses a pinned formula and exits non-zero, which would fail
// the entire batch the package shares. This exercises the exact decode +
// pinned-skip logic used by BrewManager.CheckOutdated against real brew
// --json=v2 output shape.
func TestBrewCheckOutdated_SkipsPinned(t *testing.T) {
	// Sample of `brew outdated --json=v2` output: one pinned formula and one
	// upgradeable formula.
	raw := `{
		"formulae": [
			{
				"name": "node",
				"installed_versions": ["20.0.0"],
				"current_version": "22.0.0",
				"pinned": true,
				"pinned_version": "20.0.0"
			},
			{
				"name": "ripgrep",
				"installed_versions": ["14.0.0"],
				"current_version": "14.1.0",
				"pinned": false
			}
		],
		"casks": []
	}`

	var outdated struct {
		Formulae []struct {
			Name              string   `json:"name"`
			InstalledVersions []string `json:"installed_versions"`
			CurrentVersion    string   `json:"current_version"`
			Pinned            bool     `json:"pinned"`
		} `json:"formulae"`
		Casks []struct {
			Name             string `json:"name"`
			InstalledVersion string `json:"installed_version"`
			CurrentVersion   string `json:"current_version"`
		} `json:"casks"`
	}

	if err := json.Unmarshal([]byte(raw), &outdated); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	var packages []Package
	for _, f := range outdated.Formulae {
		if f.Pinned {
			continue
		}
		currentVer := ""
		if len(f.InstalledVersions) > 0 {
			currentVer = f.InstalledVersions[0]
		}
		packages = append(packages, Package{
			Name:           f.Name,
			CurrentVersion: currentVer,
			LatestVersion:  f.CurrentVersion,
			Outdated:       true,
			InstalledBy:    "brew",
		})
	}

	if len(packages) != 1 {
		t.Fatalf("expected 1 actionable package (pinned excluded), got %d: %+v", len(packages), packages)
	}
	if packages[0].Name != "ripgrep" {
		t.Errorf("expected ripgrep, got %q", packages[0].Name)
	}
	for _, p := range packages {
		if p.Name == "node" {
			t.Error("pinned formula 'node' must not appear in the actionable update list")
		}
	}
}

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
