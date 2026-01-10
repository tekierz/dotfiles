package ui

import (
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
	"github.com/tekierz/dotfiles/internal/tools"
)

// PackageManagerProvider abstracts package manager detection and operations.
// This allows mocking for tests.
type PackageManagerProvider interface {
	// DetectManager returns the detected package manager for the current system
	DetectManager() pkg.PackageManager

	// DetectPlatform returns the detected platform
	DetectPlatform() pkg.Platform

	// CheckUpdates checks for package updates
	CheckUpdates() ([]pkg.Package, error)
}

// ConfigProvider abstracts configuration loading and saving.
// This allows mocking for tests.
type ConfigProvider interface {
	// Global config
	LoadGlobalConfig() (*config.GlobalConfig, error)
	SaveGlobalConfig(cfg *config.GlobalConfig) error

	// Tool configs
	LoadManageConfig() (*ManageConfig, error)
	SaveManageConfig(cfg *ManageConfig) error

	// User profiles
	LoadUserProfile(name string) (*config.UserProfile, error)
	SaveUserProfile(profile *config.UserProfile) error
	DeleteUserProfile(name string) error
	ListUserProfiles() ([]string, error)
	UserExists(name string) bool
	ApplyUserProfile(profile *config.UserProfile) error
	GetActiveUser() (*config.UserProfile, error)
}

// ToolRegistryProvider abstracts tool registry access.
// This allows mocking for tests.
type ToolRegistryProvider interface {
	// NewRegistry creates a new tool registry
	NewRegistry() *tools.Registry
}

// RunnerProvider abstracts bash script execution.
// This allows mocking for tests.
type RunnerProvider interface {
	// NewRunner creates a new bash runner
	NewRunner() *runner.Runner

	// CheckSudoCached checks if sudo credentials are cached
	CheckSudoCached() bool

	// NeedsSudo returns whether the package manager needs sudo
	NeedsSudo() bool
}

// Dependencies holds all injected dependencies for the UI layer.
// This enables testing by allowing mock implementations.
type Dependencies struct {
	PackageManager PackageManagerProvider
	Config         ConfigProvider
	ToolRegistry   ToolRegistryProvider
	Runner         RunnerProvider
}

// ============================================================================
// Real implementations (production)
// ============================================================================

// RealPackageManagerProvider is the production implementation
type RealPackageManagerProvider struct{}

func (p *RealPackageManagerProvider) DetectManager() pkg.PackageManager {
	return pkg.DetectManager()
}

func (p *RealPackageManagerProvider) DetectPlatform() pkg.Platform {
	return pkg.DetectPlatform()
}

func (p *RealPackageManagerProvider) CheckUpdates() ([]pkg.Package, error) {
	return pkg.CheckDotfilesUpdates()
}

// RealConfigProvider is the production implementation
type RealConfigProvider struct{}

func (c *RealConfigProvider) LoadGlobalConfig() (*config.GlobalConfig, error) {
	return config.LoadGlobalConfig()
}

func (c *RealConfigProvider) SaveGlobalConfig(cfg *config.GlobalConfig) error {
	return config.SaveGlobalConfig(cfg)
}

func (c *RealConfigProvider) LoadManageConfig() (*ManageConfig, error) {
	return config.LoadToolConfig("manage", NewManageConfig)
}

func (c *RealConfigProvider) SaveManageConfig(cfg *ManageConfig) error {
	return config.SaveToolConfig("manage", cfg)
}

func (c *RealConfigProvider) LoadUserProfile(name string) (*config.UserProfile, error) {
	return config.LoadUserProfile(name)
}

func (c *RealConfigProvider) SaveUserProfile(profile *config.UserProfile) error {
	return config.SaveUserProfile(profile)
}

func (c *RealConfigProvider) DeleteUserProfile(name string) error {
	return config.DeleteUserProfile(name)
}

func (c *RealConfigProvider) ListUserProfiles() ([]string, error) {
	return config.ListUserProfiles()
}

func (c *RealConfigProvider) UserExists(name string) bool {
	return config.UserExists(name)
}

func (c *RealConfigProvider) ApplyUserProfile(profile *config.UserProfile) error {
	return config.ApplyUserProfile(profile)
}

func (c *RealConfigProvider) GetActiveUser() (*config.UserProfile, error) {
	return config.GetActiveUser()
}

// RealToolRegistryProvider is the production implementation
type RealToolRegistryProvider struct{}

func (t *RealToolRegistryProvider) NewRegistry() *tools.Registry {
	return tools.NewRegistry()
}

// RealRunnerProvider is the production implementation
type RealRunnerProvider struct {
	mgr pkg.PackageManager
}

func (r *RealRunnerProvider) NewRunner() *runner.Runner {
	return runner.NewRunner()
}

func (r *RealRunnerProvider) CheckSudoCached() bool {
	return runner.CheckSudoCached()
}

func (r *RealRunnerProvider) NeedsSudo() bool {
	if r.mgr == nil {
		r.mgr = pkg.DetectManager()
	}
	if r.mgr == nil {
		return false
	}
	return r.mgr.NeedsSudo()
}

// NewDependencies creates a production dependency container
func NewDependencies() *Dependencies {
	return &Dependencies{
		PackageManager: &RealPackageManagerProvider{},
		Config:         &RealConfigProvider{},
		ToolRegistry:   &RealToolRegistryProvider{},
		Runner:         &RealRunnerProvider{},
	}
}

// ============================================================================
// Default context creation
// ============================================================================

// NewScreenContext creates a new screen context with the given dependencies.
// It loads initial values from persisted config.
func NewScreenContext(deps *Dependencies) *ScreenContext {
	ctx := &ScreenContext{
		Deps:              deps,
		Theme:             "catppuccin-mocha",
		NavStyle:          "emacs",
		AnimationsEnabled: true,
		Width:             80,
		Height:            24,
	}

	// Load persisted settings if available
	if deps != nil && deps.Config != nil {
		if cfg, err := deps.Config.LoadGlobalConfig(); err == nil {
			ctx.Theme = cfg.Theme
			ctx.NavStyle = cfg.NavStyle
			ctx.AnimationsEnabled = !cfg.DisableAnimations
		}
	}

	return ctx
}
