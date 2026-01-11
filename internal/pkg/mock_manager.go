package pkg

import (
	"context"
	"fmt"

	"github.com/tekierz/dotfiles/internal/runner"
)

// MockPackageManager is a test implementation of PackageManager.
type MockPackageManager struct {
	// Configuration
	ManagerName     string
	Available       bool
	RequiresSudo    bool
	InstalledPkgs   map[string]string // package name -> version
	OutdatedPkgs    []Package
	SearchResults   []Package
	InstallErr      error
	UninstallErr    error
	UpdateErr       error
	CheckOutdatedErr error
	SearchErr       error

	// Call tracking
	InstallCalls     [][]string
	UninstallCalls   [][]string
	UpdateCalls      [][]string
	UpdateAllCalled  bool
}

// NewMockPackageManager creates a new mock package manager.
func NewMockPackageManager() *MockPackageManager {
	return &MockPackageManager{
		ManagerName:   "mock",
		Available:     true,
		InstalledPkgs: make(map[string]string),
	}
}

// Name returns the package manager name.
func (m *MockPackageManager) Name() string {
	return m.ManagerName
}

// IsAvailable checks if this package manager is available.
func (m *MockPackageManager) IsAvailable() bool {
	return m.Available
}

// Install installs packages.
func (m *MockPackageManager) Install(packages ...string) error {
	m.InstallCalls = append(m.InstallCalls, packages)
	if m.InstallErr != nil {
		return m.InstallErr
	}
	for _, pkg := range packages {
		m.InstalledPkgs[pkg] = "1.0.0"
	}
	return nil
}

// Uninstall removes packages.
func (m *MockPackageManager) Uninstall(packages ...string) error {
	m.UninstallCalls = append(m.UninstallCalls, packages)
	if m.UninstallErr != nil {
		return m.UninstallErr
	}
	for _, pkg := range packages {
		delete(m.InstalledPkgs, pkg)
	}
	return nil
}

// IsInstalled checks if a package is installed.
func (m *MockPackageManager) IsInstalled(pkg string) bool {
	_, ok := m.InstalledPkgs[pkg]
	return ok
}

// GetVersion returns the installed version.
func (m *MockPackageManager) GetVersion(pkg string) (string, error) {
	if version, ok := m.InstalledPkgs[pkg]; ok {
		return version, nil
	}
	return "", fmt.Errorf("package %s not installed", pkg)
}

// CheckOutdated returns outdated packages.
func (m *MockPackageManager) CheckOutdated() ([]Package, error) {
	if m.CheckOutdatedErr != nil {
		return nil, m.CheckOutdatedErr
	}
	return m.OutdatedPkgs, nil
}

// Update updates specific packages.
func (m *MockPackageManager) Update(packages ...string) error {
	m.UpdateCalls = append(m.UpdateCalls, packages)
	return m.UpdateErr
}

// UpdateAll updates all outdated packages.
func (m *MockPackageManager) UpdateAll() error {
	m.UpdateAllCalled = true
	return m.UpdateErr
}

// Search searches for packages.
func (m *MockPackageManager) Search(query string) ([]Package, error) {
	if m.SearchErr != nil {
		return nil, m.SearchErr
	}
	return m.SearchResults, nil
}

// ListInstalled returns all installed packages.
func (m *MockPackageManager) ListInstalled() ([]Package, error) {
	var result []Package
	for name, version := range m.InstalledPkgs {
		result = append(result, Package{
			Name:           name,
			CurrentVersion: version,
			InstalledBy:    m.ManagerName,
		})
	}
	return result, nil
}

// NeedsSudo returns whether sudo is required.
func (m *MockPackageManager) NeedsSudo() bool {
	return m.RequiresSudo
}

// InstallStreaming installs packages with streaming output.
func (m *MockPackageManager) InstallStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error) {
	m.InstallCalls = append(m.InstallCalls, packages)
	if m.InstallErr != nil {
		return nil, m.InstallErr
	}
	for _, pkg := range packages {
		m.InstalledPkgs[pkg] = "1.0.0"
	}
	// Return a no-op streaming command
	return nil, nil
}

// UpdateStreaming updates packages with streaming output.
func (m *MockPackageManager) UpdateStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error) {
	m.UpdateCalls = append(m.UpdateCalls, packages)
	return nil, m.UpdateErr
}

// UpdateAllStreaming updates all packages with streaming output.
func (m *MockPackageManager) UpdateAllStreaming(ctx context.Context) (*runner.StreamingCmd, error) {
	m.UpdateAllCalled = true
	return nil, m.UpdateErr
}

// SetInstalled marks packages as installed with version.
func (m *MockPackageManager) SetInstalled(pkg, version string) {
	m.InstalledPkgs[pkg] = version
}

// SetOutdated sets the list of outdated packages.
func (m *MockPackageManager) SetOutdated(pkgs []Package) {
	m.OutdatedPkgs = pkgs
}

// Reset clears all call tracking.
func (m *MockPackageManager) Reset() {
	m.InstallCalls = nil
	m.UninstallCalls = nil
	m.UpdateCalls = nil
	m.UpdateAllCalled = false
}
