package ui

import (
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
	"github.com/tekierz/dotfiles/internal/tools"
)

// MockPackageManagerProvider is a test implementation of PackageManagerProvider.
type MockPackageManagerProvider struct {
	Manager     pkg.PackageManager
	Platform    pkg.Platform
	Updates     []pkg.Package
	UpdatesErr  error
}

func (m *MockPackageManagerProvider) DetectManager() pkg.PackageManager {
	return m.Manager
}

func (m *MockPackageManagerProvider) DetectPlatform() pkg.Platform {
	return m.Platform
}

func (m *MockPackageManagerProvider) CheckUpdates() ([]pkg.Package, error) {
	return m.Updates, m.UpdatesErr
}

// MockConfigProvider is a test implementation of ConfigProvider.
type MockConfigProvider struct {
	GlobalConfig   *config.GlobalConfig
	ManageConfig   *ManageConfig
	UserProfiles   map[string]*config.UserProfile
	ActiveUser     *config.UserProfile

	SavedGlobalConfig *config.GlobalConfig
	SavedManageConfig *ManageConfig
	SavedProfiles     map[string]*config.UserProfile
	DeletedProfiles   []string
	AppliedProfiles   []*config.UserProfile

	LoadGlobalErr    error
	SaveGlobalErr    error
	LoadManageErr    error
	SaveManageErr    error
	LoadUserErr      error
	SaveUserErr      error
	DeleteUserErr    error
	ListUsersErr     error
	ApplyUserErr     error
	GetActiveUserErr error
}

func NewMockConfigProvider() *MockConfigProvider {
	return &MockConfigProvider{
		GlobalConfig: &config.GlobalConfig{
			Theme:    "catppuccin-mocha",
			NavStyle: "emacs",
		},
		ManageConfig:  NewManageConfig(),
		UserProfiles:  make(map[string]*config.UserProfile),
		SavedProfiles: make(map[string]*config.UserProfile),
	}
}

func (m *MockConfigProvider) LoadGlobalConfig() (*config.GlobalConfig, error) {
	return m.GlobalConfig, m.LoadGlobalErr
}

func (m *MockConfigProvider) SaveGlobalConfig(cfg *config.GlobalConfig) error {
	if m.SaveGlobalErr != nil {
		return m.SaveGlobalErr
	}
	m.SavedGlobalConfig = cfg
	return nil
}

func (m *MockConfigProvider) LoadManageConfig() (*ManageConfig, error) {
	return m.ManageConfig, m.LoadManageErr
}

func (m *MockConfigProvider) SaveManageConfig(cfg *ManageConfig) error {
	if m.SaveManageErr != nil {
		return m.SaveManageErr
	}
	m.SavedManageConfig = cfg
	return nil
}

func (m *MockConfigProvider) LoadUserProfile(name string) (*config.UserProfile, error) {
	if m.LoadUserErr != nil {
		return nil, m.LoadUserErr
	}
	if profile, ok := m.UserProfiles[name]; ok {
		return profile, nil
	}
	return nil, nil
}

func (m *MockConfigProvider) SaveUserProfile(profile *config.UserProfile) error {
	if m.SaveUserErr != nil {
		return m.SaveUserErr
	}
	m.UserProfiles[profile.Name] = profile
	m.SavedProfiles[profile.Name] = profile
	return nil
}

func (m *MockConfigProvider) DeleteUserProfile(name string) error {
	if m.DeleteUserErr != nil {
		return m.DeleteUserErr
	}
	delete(m.UserProfiles, name)
	m.DeletedProfiles = append(m.DeletedProfiles, name)
	return nil
}

func (m *MockConfigProvider) ListUserProfiles() ([]string, error) {
	if m.ListUsersErr != nil {
		return nil, m.ListUsersErr
	}
	var names []string
	for name := range m.UserProfiles {
		names = append(names, name)
	}
	return names, nil
}

func (m *MockConfigProvider) UserExists(name string) bool {
	_, ok := m.UserProfiles[name]
	return ok
}

func (m *MockConfigProvider) ApplyUserProfile(profile *config.UserProfile) error {
	if m.ApplyUserErr != nil {
		return m.ApplyUserErr
	}
	m.AppliedProfiles = append(m.AppliedProfiles, profile)
	return nil
}

func (m *MockConfigProvider) GetActiveUser() (*config.UserProfile, error) {
	return m.ActiveUser, m.GetActiveUserErr
}

// MockToolRegistryProvider is a test implementation of ToolRegistryProvider.
type MockToolRegistryProvider struct {
	Registry *tools.Registry
}

func (m *MockToolRegistryProvider) NewRegistry() *tools.Registry {
	if m.Registry != nil {
		return m.Registry
	}
	return tools.NewRegistry()
}

// MockRunnerProvider is a test implementation of RunnerProvider.
type MockRunnerProvider struct {
	SudoCached     bool
	RequiresSudo   bool
}

func (m *MockRunnerProvider) NewRunner() *runner.Runner {
	return runner.NewRunner()
}

func (m *MockRunnerProvider) CheckSudoCached() bool {
	return m.SudoCached
}

func (m *MockRunnerProvider) NeedsSudo() bool {
	return m.RequiresSudo
}

// NewTestDependencies creates a Dependencies struct with all mock providers.
func NewTestDependencies() *Dependencies {
	return &Dependencies{
		PackageManager: &MockPackageManagerProvider{
			Manager:  pkg.NewMockPackageManager(),
			Platform: pkg.PlatformMacOS,
		},
		Config:       NewMockConfigProvider(),
		ToolRegistry: &MockToolRegistryProvider{},
		Runner:       &MockRunnerProvider{},
	}
}

// NewTestScreenContext creates a ScreenContext for testing.
func NewTestScreenContext() *ScreenContext {
	return &ScreenContext{
		Deps:              NewTestDependencies(),
		Theme:             "catppuccin-mocha",
		NavStyle:          "emacs",
		AnimationsEnabled: false, // Disable for tests
		Width:             80,
		Height:            24,
	}
}
