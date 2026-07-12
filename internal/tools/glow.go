package tools

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/safefile"
)

// GlowConfig holds Glow configuration settings
type GlowConfig struct {
	// Pager remains a string for compatibility with existing persisted dashboard
	// settings. The only canonical values are "auto" (true) and "never" (false).
	Pager            string
	Style            string
	Width            int
	Mouse            bool
	All              bool
	ShowLineNumbers  bool
	PreserveNewLines bool
}

// GlowTool represents glow markdown viewer
type GlowTool struct {
	BaseTool
}

func normalizeGlowConfig(cfg GlowConfig) GlowConfig {
	if cfg.Style == "" {
		cfg.Style = "auto"
	}
	if cfg.Pager == "" {
		cfg.Pager = "auto"
	}
	return cfg
}

func ValidateGlowConfig(cfg GlowConfig, theme string) error {
	cfg = normalizeGlowConfig(cfg)
	if err := validateConfigToken("global Glow theme", theme); err != nil {
		return err
	}
	if err := validateGlowStyle(cfg.Style); err != nil {
		return err
	}
	switch cfg.Pager {
	case "auto", "less", "more", "none", "never":
	default:
		return fmt.Errorf("unsupported Glow pager %q", cfg.Pager)
	}
	if cfg.Width < 0 {
		return fmt.Errorf("glow width must be non-negative")
	}
	return nil
}

func validateGlowStyle(style string) error {
	if err := validateGlowStyleSyntax(style); err != nil {
		return err
	}
	switch strings.TrimSpace(style) {
	case "auto", "ascii", "dark", "dracula", "tokyo-night", "light", "notty", "pink":
		return nil
	}
	return fmt.Errorf("glow custom styles are preserved read-only; writable validation requires pinned Glamour v0.10 support, so choose a built-in style")
}

func validateGlowStyleSyntax(style string) error {
	trimmed := strings.TrimSpace(style)
	if style != trimmed || style == "" || len(style) > 1024 || strings.ContainsAny(style, "\r\n\x00") {
		return fmt.Errorf("invalid Glow style")
	}
	for _, r := range style {
		if unicode.IsControl(r) || isBidiControl(r) {
			return fmt.Errorf("invalid Glow style")
		}
	}
	return nil
}

// NewGlowTool creates a new glow tool
func NewGlowTool() *GlowTool {
	configPath, err := glowConfigPath()
	var configPaths []string
	if err == nil {
		configPaths = []string{configPath}
	}

	return &GlowTool{
		BaseTool: BaseTool{
			id:          "glow",
			name:        "Glow",
			description: "Render markdown on the CLI",
			icon:        "󰈙",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"glow"},
				pkg.PlatformArch:  {"glow"},
				// glow is not in stock Debian/Ubuntu repos (requires Charm keyring);
				// omitting Debian entry prevents a guaranteed-failing apt install.
			},
			configPaths: configPaths,
			// UI metadata
			uiGroup:        UIGroupCLITools,
			configScreen:   31, // ScreenConfigGlow - has dedicated config screen
			defaultEnabled: true,
		},
	}
}

func (t *GlowTool) ConfigPaths() []string {
	path, err := GlowConfigMutationPath()
	if err != nil {
		return nil
	}
	return []string{path}
}
func (t *GlowTool) HasConfig() bool { return true }

// GenerateGlowConfig builds the glow.yml content
func GenerateGlowConfig(cfg GlowConfig, theme string) string {
	cfg = normalizeGlowConfig(cfg)
	return string(generateGlowManagedSection(cfg))
}

// WriteGlowConfig writes the glow config to disk
func WriteGlowConfig(cfg GlowConfig, theme string) error {
	_, err := WriteGlowConfigTracked(cfg, theme)
	return err
}

// WriteGlowConfigTracked writes the glow config and returns the exact committed
// revision for rollback evidence.
func WriteGlowConfigTracked(cfg GlowConfig, theme string) (MutationEvidence, error) {
	if err := ValidateGlowConfig(cfg, theme); err != nil {
		return MutationEvidence{}, err
	}
	configPath, err := glowConfigPath()
	if err != nil {
		return MutationEvidence{}, err
	}

	return writeGlowManagedConfigTracked(configPath, cfg)
}

// WriteGlowConfigAtRevisionTracked applies a plan-accepted Glow revision.
func WriteGlowConfigAtRevisionTracked(cfg GlowConfig, theme string, accepted safefile.Revision) (MutationEvidence, error) {
	if err := ValidateGlowConfig(cfg, theme); err != nil {
		return MutationEvidence{}, err
	}
	configPath, err := glowConfigPath()
	if err != nil {
		return MutationEvidence{}, err
	}
	parents, err := compatibilityToolConfigParents(configPath)
	if err != nil {
		return MutationEvidence{}, err
	}
	return writeGlowConfigAtAuthorityTracked(configPath, cfg, accepted, parents, operation.DefaultLocker)
}

func WriteGlowConfigAtAuthorityTracked(cfg GlowConfig, theme string, accepted safefile.Revision, parents *safefile.ParentChain, locker operation.Locker) (MutationEvidence, error) {
	if err := ValidateGlowConfig(cfg, theme); err != nil {
		return MutationEvidence{}, err
	}
	configPath, err := glowConfigPath()
	if err != nil {
		return MutationEvidence{}, err
	}
	return writeGlowConfigAtAuthorityTracked(configPath, cfg, accepted, parents, locker)
}

func glowConfigPath() (string, error) {
	return GlowConfigMutationPath()
}

func writeGlowManagedConfigTracked(path string, cfg GlowConfig) (MutationEvidence, error) {
	var evidence MutationEvidence
	err := withToolConfigLock(path, func(root, rel string) error {
		active, resolveErr := GlowConfigMutationPath()
		if resolveErr != nil {
			return resolveErr
		}
		if active != path {
			return fmt.Errorf("active Glow config changed from %s to %s while acquiring authority", path, active)
		}
		existing, revision, err := readToolConfig(root, rel)
		if err != nil {
			return err
		}
		merged, _, err := mergeGlowManagedSection(existing, cfg)
		if err != nil {
			return err
		}
		if revision.Exists() && bytes.Equal(existing, merged) {
			evidence = MutationEvidence{Path: path, Revision: revision}
			return nil
		}
		committed, err := replaceToolConfigAtRevisionTracked(root, rel, revision, merged)
		if err == nil {
			evidence = MutationEvidence{Path: path, Revision: committed}
		}
		return err
	})
	return evidence, err
}

func writeGlowConfigAtAuthorityTracked(path string, cfg GlowConfig, accepted safefile.Revision, parents *safefile.ParentChain, locker operation.Locker) (MutationEvidence, error) {
	if !accepted.Tracked() || parents == nil || !parents.Tracked() || locker == nil {
		return MutationEvidence{}, fmt.Errorf("%w: accepted Glow authority is incomplete", safefile.ErrParentChanged)
	}
	var evidence MutationEvidence
	err := withToolConfigLockAuthorized(path, locker, func(root, rel string) error {
		active, resolveErr := GlowConfigMutationPath()
		if resolveErr != nil {
			return resolveErr
		}
		if active != path {
			return fmt.Errorf("active Glow config changed from %s to %s while acquiring authority", path, active)
		}
		existing, current, err := safefile.ReadWithinAuthorized(root, rel, parents)
		if err != nil {
			return err
		}
		if current != accepted {
			return fmt.Errorf("%w: Glow config changed after plan acceptance", safefile.ErrRevisionChanged)
		}
		merged, _, err := mergeGlowManagedSection(existing, cfg)
		if err != nil {
			return err
		}
		evidence = MutationEvidence{Path: path, Revision: current, Parents: parents}
		if current.Exists() && bytes.Equal(existing, merged) {
			return nil
		}
		committed, err := replaceToolConfigAtRevisionNoCreateAuthorizedTracked(root, rel, accepted, parents, merged)
		if err == nil {
			evidence.Revision = committed
		}
		return err
	})
	return evidence, err
}
