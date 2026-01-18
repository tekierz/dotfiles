package pkg

import (
	"testing"
)

func TestPackageStruct(t *testing.T) {
	pkg := Package{
		Name:           "vim",
		CurrentVersion: "9.0.0",
		LatestVersion:  "9.1.0",
		Outdated:       true,
		InstalledBy:    "brew",
		Description:    "Vim - Vi IMproved",
	}

	if pkg.Name != "vim" {
		t.Errorf("Name = %q, want %q", pkg.Name, "vim")
	}
	if pkg.CurrentVersion != "9.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", pkg.CurrentVersion, "9.0.0")
	}
	if pkg.LatestVersion != "9.1.0" {
		t.Errorf("LatestVersion = %q, want %q", pkg.LatestVersion, "9.1.0")
	}
	if !pkg.Outdated {
		t.Error("Outdated should be true")
	}
}

func TestPlatformConstants(t *testing.T) {
	// Verify platform constants are distinct
	platforms := map[Platform]bool{
		PlatformMacOS:   true,
		PlatformArch:    true,
		PlatformDebian:  true,
		PlatformUnknown: true,
	}

	if len(platforms) != 4 {
		t.Error("expected 4 distinct platform constants")
	}

	// Verify string representation
	if PlatformMacOS != "macos" {
		t.Errorf("PlatformMacOS = %q, want %q", PlatformMacOS, "macos")
	}
	if PlatformArch != "arch" {
		t.Errorf("PlatformArch = %q, want %q", PlatformArch, "arch")
	}
	if PlatformDebian != "debian" {
		t.Errorf("PlatformDebian = %q, want %q", PlatformDebian, "debian")
	}
	if PlatformUnknown != "unknown" {
		t.Errorf("PlatformUnknown = %q, want %q", PlatformUnknown, "unknown")
	}
}

func TestMockPackageManager_Basic(t *testing.T) {
	mock := NewMockPackageManager()

	// Test defaults
	if mock.Name() != "mock" {
		t.Errorf("Name() = %q, want %q", mock.Name(), "mock")
	}
	if !mock.IsAvailable() {
		t.Error("IsAvailable() should be true by default")
	}
	if mock.NeedsSudo() {
		t.Error("NeedsSudo() should be false by default")
	}
}

func TestMockPackageManager_Install(t *testing.T) {
	mock := NewMockPackageManager()

	// Install packages
	err := mock.Install("vim", "git")
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify packages are installed
	if !mock.IsInstalled("vim") {
		t.Error("vim should be installed")
	}
	if !mock.IsInstalled("git") {
		t.Error("git should be installed")
	}
	if mock.IsInstalled("emacs") {
		t.Error("emacs should not be installed")
	}

	// Verify call tracking
	if len(mock.InstallCalls) != 1 {
		t.Errorf("InstallCalls count = %d, want 1", len(mock.InstallCalls))
	}
	if len(mock.InstallCalls[0]) != 2 {
		t.Errorf("InstallCalls[0] length = %d, want 2", len(mock.InstallCalls[0]))
	}
}

func TestMockPackageManager_Uninstall(t *testing.T) {
	mock := NewMockPackageManager()

	// Install first
	mock.SetInstalled("vim", "9.0.0")
	mock.SetInstalled("git", "2.40.0")

	// Uninstall
	err := mock.Uninstall("vim")
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	if mock.IsInstalled("vim") {
		t.Error("vim should not be installed after uninstall")
	}
	if !mock.IsInstalled("git") {
		t.Error("git should still be installed")
	}

	// Verify call tracking
	if len(mock.UninstallCalls) != 1 {
		t.Errorf("UninstallCalls count = %d, want 1", len(mock.UninstallCalls))
	}
}

func TestMockPackageManager_GetVersion(t *testing.T) {
	mock := NewMockPackageManager()

	// Package not installed
	_, err := mock.GetVersion("vim")
	if err == nil {
		t.Error("GetVersion should fail for uninstalled package")
	}

	// Install with version
	mock.SetInstalled("vim", "9.0.0")

	version, err := mock.GetVersion("vim")
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if version != "9.0.0" {
		t.Errorf("GetVersion = %q, want %q", version, "9.0.0")
	}
}

func TestMockPackageManager_CheckOutdated(t *testing.T) {
	mock := NewMockPackageManager()

	// Set outdated packages
	mock.SetOutdated([]Package{
		{Name: "vim", CurrentVersion: "9.0.0", LatestVersion: "9.1.0", Outdated: true},
		{Name: "git", CurrentVersion: "2.40.0", LatestVersion: "2.41.0", Outdated: true},
	})

	outdated, err := mock.CheckOutdated()
	if err != nil {
		t.Fatalf("CheckOutdated failed: %v", err)
	}
	if len(outdated) != 2 {
		t.Errorf("CheckOutdated returned %d packages, want 2", len(outdated))
	}
}

func TestMockPackageManager_Update(t *testing.T) {
	mock := NewMockPackageManager()

	err := mock.Update("vim", "git")
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if len(mock.UpdateCalls) != 1 {
		t.Errorf("UpdateCalls count = %d, want 1", len(mock.UpdateCalls))
	}

	// UpdateAll
	err = mock.UpdateAll()
	if err != nil {
		t.Fatalf("UpdateAll failed: %v", err)
	}
	if !mock.UpdateAllCalled {
		t.Error("UpdateAllCalled should be true")
	}
}

func TestMockPackageManager_ListInstalled(t *testing.T) {
	mock := NewMockPackageManager()

	mock.SetInstalled("vim", "9.0.0")
	mock.SetInstalled("git", "2.40.0")

	installed, err := mock.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(installed) != 2 {
		t.Errorf("ListInstalled returned %d packages, want 2", len(installed))
	}

	// Verify package details
	found := make(map[string]bool)
	for _, pkg := range installed {
		found[pkg.Name] = true
		if pkg.InstalledBy != "mock" {
			t.Errorf("InstalledBy = %q, want %q", pkg.InstalledBy, "mock")
		}
	}
	if !found["vim"] {
		t.Error("vim should be in list")
	}
	if !found["git"] {
		t.Error("git should be in list")
	}
}

func TestMockPackageManager_Search(t *testing.T) {
	mock := NewMockPackageManager()

	// Set search results
	mock.SearchResults = []Package{
		{Name: "vim", Description: "Vim - Vi IMproved"},
	}

	results, err := mock.Search("vim")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search returned %d packages, want 1", len(results))
	}
}

func TestMockPackageManager_Errors(t *testing.T) {
	mock := NewMockPackageManager()

	// Set errors
	mock.InstallErr = ErrMockInstallFailed
	mock.UninstallErr = ErrMockUninstallFailed
	mock.UpdateErr = ErrMockUpdateFailed
	mock.CheckOutdatedErr = ErrMockCheckFailed
	mock.SearchErr = ErrMockSearchFailed

	if err := mock.Install("vim"); err != ErrMockInstallFailed {
		t.Errorf("Install should return ErrMockInstallFailed")
	}
	if err := mock.Uninstall("vim"); err != ErrMockUninstallFailed {
		t.Errorf("Uninstall should return ErrMockUninstallFailed")
	}
	if err := mock.Update("vim"); err != ErrMockUpdateFailed {
		t.Errorf("Update should return ErrMockUpdateFailed")
	}
	if _, err := mock.CheckOutdated(); err != ErrMockCheckFailed {
		t.Errorf("CheckOutdated should return ErrMockCheckFailed")
	}
	if _, err := mock.Search("vim"); err != ErrMockSearchFailed {
		t.Errorf("Search should return ErrMockSearchFailed")
	}
}

func TestMockPackageManager_Reset(t *testing.T) {
	mock := NewMockPackageManager()

	// Make some calls
	mock.Install("vim")
	mock.Uninstall("vim")
	mock.Update("git")
	mock.UpdateAll()

	// Verify calls were tracked
	if len(mock.InstallCalls) != 1 {
		t.Error("expected 1 install call")
	}

	// Reset
	mock.Reset()

	// Verify cleared
	if len(mock.InstallCalls) != 0 {
		t.Error("InstallCalls should be empty after reset")
	}
	if len(mock.UninstallCalls) != 0 {
		t.Error("UninstallCalls should be empty after reset")
	}
	if len(mock.UpdateCalls) != 0 {
		t.Error("UpdateCalls should be empty after reset")
	}
	if mock.UpdateAllCalled {
		t.Error("UpdateAllCalled should be false after reset")
	}
}

// Sentinel errors for testing
var (
	ErrMockInstallFailed   = &mockError{"mock install failed"}
	ErrMockUninstallFailed = &mockError{"mock uninstall failed"}
	ErrMockUpdateFailed    = &mockError{"mock update failed"}
	ErrMockCheckFailed     = &mockError{"mock check failed"}
	ErrMockSearchFailed    = &mockError{"mock search failed"}
)

type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}
