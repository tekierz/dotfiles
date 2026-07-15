package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/safefile"
	dotfilesTheme "github.com/tekierz/dotfiles/internal/theme"
)

const (
	tmuxManagedStart = "# >>> dotfiles tmux (managed)"
	tmuxManagedEnd   = "# <<< dotfiles tmux (managed)"
)

// TmuxTool represents tmux terminal multiplexer
type TmuxTool struct {
	BaseTool
}

// TmuxConfig holds tmux configuration settings
type TmuxConfig struct {
	// Basic settings
	Prefix     string
	SplitBinds string
	StatusBar  string
	MouseMode  bool

	BaseIndex        int    // Starting index for windows/panes
	PaneBorderStyle  string // "single", "double", "heavy", "simple"
	HistoryLimit     int    // Scrollback buffer size
	EscapeTime       int    // Escape key delay in ms
	AggressiveResize bool   // Aggressively resize panes on window changes

	// TPM settings
	TPMEnabled       bool
	PluginSensible   bool
	PluginResurrect  bool
	PluginContinuum  bool
	PluginYank       bool
	ContinuumSaveMin int
	ContinuumRestore bool // Restore sessions on tmux start
}

// NewTmuxTool creates a new Tmux tool
func NewTmuxTool() *TmuxTool {
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".tmux.conf")
	if active, err := TmuxConfigMutationPath(); err == nil {
		configPath = active
	}
	return &TmuxTool{
		BaseTool: BaseTool{
			id:          "tmux",
			name:        "Tmux",
			description: "Terminal multiplexer",
			icon:        "",
			category:    CategoryTerminal,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"tmux"},
				pkg.PlatformArch:   {"tmux"},
				pkg.PlatformDebian: {"tmux"},
			},
			configPaths: []string{
				configPath,
			},
			// UI metadata
			uiGroup:        UIGroupNone,
			configScreen:   10, // ScreenConfigTmux
			defaultEnabled: true,
		},
	}
}

type tmuxConfigTarget struct {
	path       string
	reloadExpr string
}

// tmuxConfigCandidates mirrors tmux's user-config search order after the
// system configuration: legacy HOME first, then explicit XDG_CONFIG_HOME, then
// the default ~/.config location. The first existing candidate is active.
func tmuxConfigCandidates(home string) ([]tmuxConfigTarget, error) {
	if home == "" || !filepath.IsAbs(home) {
		return nil, fmt.Errorf("tmux config discovery requires an absolute HOME")
	}
	candidates := []tmuxConfigTarget{{
		path:       filepath.Join(home, ".tmux.conf"),
		reloadExpr: "~/.tmux.conf",
	}}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return nil, fmt.Errorf("XDG_CONFIG_HOME must be absolute: %q", xdg)
		}
		candidates = append(candidates, tmuxConfigTarget{
			path:       filepath.Join(filepath.Clean(xdg), "tmux", "tmux.conf"),
			reloadExpr: `"$XDG_CONFIG_HOME/tmux/tmux.conf"`,
		})
	}
	defaultXDG := filepath.Join(home, ".config", "tmux", "tmux.conf")
	if len(candidates) == 1 || filepath.Clean(candidates[len(candidates)-1].path) != filepath.Clean(defaultXDG) {
		candidates = append(candidates, tmuxConfigTarget{path: defaultXDG, reloadExpr: "~/.config/tmux/tmux.conf"})
	}
	return candidates, nil
}

func tmuxConfigMutationTarget() (tmuxConfigTarget, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return tmuxConfigTarget{}, fmt.Errorf("failed to get home directory: %w", err)
	}
	candidates, err := tmuxConfigCandidates(home)
	if err != nil {
		return tmuxConfigTarget{}, err
	}
	for _, candidate := range candidates {
		if _, statErr := os.Lstat(candidate.path); statErr == nil {
			return candidate, nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return tmuxConfigTarget{}, fmt.Errorf("inspect tmux config candidate %s: %w", candidate.path, statErr)
		}
	}
	return candidates[0], nil
}

// TmuxConfigMutationPath returns the exact active user config path that tmux
// itself will prefer. It is side-effect free and never creates missing parents.
func TmuxConfigMutationPath() (string, error) {
	target, err := tmuxConfigMutationTarget()
	return target.path, err
}

// TPMPath returns the TPM installation directory
func TPMPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tmux", "plugins", "tpm")
}

// IsTPMInstalled checks if TPM is installed
func IsTPMInstalled() bool {
	_, err := os.Stat(TPMPath())
	return err == nil
}

// InstallTPM clones TPM repository
func InstallTPM() error {
	_, err := installTPMTracked()
	return err
}

func installTPMTracked() (result MutationEvidence, returnErr error) {
	return installTPMTrackedWithContext(context.Background())
}

func installTPMTrackedWithContext(ctx context.Context) (result MutationEvidence, returnErr error) {
	return installTPMAtSnapshotTrackedWithContext(ctx, nil, nil)
}

func installTPMAtSnapshotTrackedWithContext(ctx context.Context, accepted *safefile.DirectorySnapshot, parents *safefile.ParentChain, states ...*operation.StateAuthority) (result MutationEvidence, returnErr error) {
	if ctx == nil {
		return MutationEvidence{}, errors.New("TPM operation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return MutationEvidence{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return MutationEvidence{}, fmt.Errorf("determine HOME for TPM: %w", err)
	}
	var staging string
	var createdStaging *operation.StateStagingAuthority
	if len(states) != 0 && states[0] != nil {
		staging, createdStaging, err = operation.CreateStateStagingDirectoryWithAuthorityTracked(states[0], "tmux-tpm")
	} else {
		staging, createdStaging, err = operation.CreateStateStagingDirectoryTracked("tmux-tpm")
	}
	if err != nil {
		return MutationEvidence{}, fmt.Errorf("create TPM clone staging directory: %w", err)
	}
	var cleanupExpected *safefile.DirectorySnapshot
	defer func() {
		if cleanupExpected == nil {
			cleanupExpected, err = operation.SnapshotStateStagingDirectory(staging, createdStaging)
			if err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("capture TPM staging cleanup authority: %w", err))
				return
			}
		}
		if cleanupErr := operation.RemoveStateStagingDirectoryAuthorized(staging, createdStaging, cleanupExpected); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			returnErr = errors.Join(returnErr, fmt.Errorf("clean TPM clone staging directory: %w", cleanupErr))
		}
	}()

	// Clone outside the live plugin path. A failed clone is deleted without ever
	// becoming a planned mutation target.
	cloneCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	stagingFD, err := operation.OpenStateStagingDirectory(staging, createdStaging)
	if err != nil {
		return MutationEvidence{}, fmt.Errorf("open exact TPM staging directory: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, stagingFD.Close()) }()
	artifact, err := TPMRemoteArtifact()
	if err != nil {
		return MutationEvidence{}, fmt.Errorf("resolve pinned TPM artifact: %w", err)
	}
	// Keep the exact staging descriptor open across acquisition. The shared
	// artifact loader fetches only the reviewed commit and verifies HEAD before
	// any bytes can enter the live namespace.
	commandFactory := func(ctx context.Context, name string, arguments ...string) *exec.Cmd {
		return exec.CommandContext(ctx, name, arguments...)
	}
	if err := clonePinnedGitArtifact(cloneCtx, staging, artifact, commandFactory); err != nil {
		return MutationEvidence{}, fmt.Errorf("stage pinned TPM artifact: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return MutationEvidence{}, err
	}
	snapshot, err := operation.SnapshotStateStagingDirectory(staging, createdStaging)
	if err != nil {
		return MutationEvidence{}, fmt.Errorf("snapshot cloned TPM: %w", err)
	}
	cleanupExpected = snapshot
	if err := ctx.Err(); err != nil {
		return MutationEvidence{}, err
	}
	var live *safefile.DirectorySnapshot
	if accepted != nil {
		if parents != nil {
			live, err = safefile.RestoreDirectoryWithinSnapshotNoCreateAuthorizedTracked(home, filepath.ToSlash(filepath.Join(".tmux", "plugins", "tpm")), snapshot, accepted, parents)
		} else {
			live, err = safefile.RestoreDirectoryWithinSnapshotNoCreateTracked(home, filepath.ToSlash(filepath.Join(".tmux", "plugins", "tpm")), snapshot, accepted)
		}
	} else {
		live, err = safefile.RestoreDirectoryWithinSnapshotTracked(home, filepath.ToSlash(filepath.Join(".tmux", "plugins", "tpm")), snapshot, accepted)
	}
	if err != nil {
		return MutationEvidence{Path: TPMPath(), Directory: live, Parents: parents}, err
	}
	return MutationEvidence{Path: TPMPath(), Directory: live, Parents: parents}, nil
}

// RunTPMInstall triggers TPM to install plugins
func RunTPMInstall() error {
	installScript := filepath.Join(TPMPath(), "scripts", "install_plugins.sh")

	if _, err := os.Stat(installScript); err != nil {
		return fmt.Errorf("TPM install script not found: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	// #nosec G204 -- running the installed TPM script is the explicit purpose of this operation.
	cmd := exec.CommandContext(ctx, installScript)
	return cmd.Run()
}

// GenerateTmuxConfig builds the tmux.conf content
func GenerateTmuxConfig(cfg TmuxConfig, theme string) string {
	return generateTmuxConfig(cfg, theme, "~/.tmux.conf")
}

func generateTmuxConfig(cfg TmuxConfig, theme, reloadExpr string) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# Generated by dotfiles TUI\n")
	p := dotfilesTheme.GetOrDefault(theme)
	sb.WriteString(fmt.Sprintf("set -g status-style \"fg=%s,bg=%s\"\n", p.Text, p.Bg))
	sb.WriteString(fmt.Sprintf("set -g pane-border-style \"fg=%s\"\n", p.Border))
	sb.WriteString(fmt.Sprintf("set -g pane-active-border-style \"fg=%s\"\n", p.Accent))
	sb.WriteString(fmt.Sprintf("set -g window-status-current-style \"fg=%s,bg=%s,bold\"\n", p.Bg, p.Accent))
	sb.WriteString(fmt.Sprintf("set -g window-status-style \"fg=%s,bg=%s\"\n", p.TextMuted, p.Bg))
	sb.WriteString(fmt.Sprintf("set -g message-style \"fg=%s,bg=%s\"\n", p.Bg, p.AccentAlt))
	sb.WriteString(fmt.Sprintf("set -g mode-style \"fg=%s,bg=%s\"\n\n", p.Bg, p.Accent))

	// Terminal settings
	sb.WriteString("# Terminal settings\n")
	sb.WriteString("set -g default-terminal \"tmux-256color\"\n")
	sb.WriteString("set -ga terminal-overrides \",xterm-256color:Tc\"\n\n")

	// Prefix key
	sb.WriteString("# Prefix key\n")
	prefix := prefixToTmuxFormat(cfg.Prefix)
	sb.WriteString(fmt.Sprintf("set -g prefix %s\n", prefix))
	sb.WriteString("unbind C-b\n")
	sb.WriteString(fmt.Sprintf("bind %s send-prefix\n\n", prefix))

	// Split bindings
	sb.WriteString("# Split panes\n")
	if cfg.SplitBinds == "pipes" {
		sb.WriteString("bind | split-window -h -c \"#{pane_current_path}\"\n")
		sb.WriteString("bind - split-window -v -c \"#{pane_current_path}\"\n")
		sb.WriteString("unbind '\"'\n")
		sb.WriteString("unbind %\n")
	} else {
		sb.WriteString("bind % split-window -h -c \"#{pane_current_path}\"\n")
		sb.WriteString("bind '\"' split-window -v -c \"#{pane_current_path}\"\n")
	}
	sb.WriteString("\n")

	// Mouse mode
	sb.WriteString("# Mouse\n")
	if cfg.MouseMode {
		sb.WriteString("set -g mouse on\n")
	} else {
		sb.WriteString("set -g mouse off\n")
	}
	sb.WriteString("\n")

	// Window settings
	sb.WriteString("# Window settings\n")
	sb.WriteString(fmt.Sprintf("set -g base-index %d\n", cfg.BaseIndex))
	sb.WriteString(fmt.Sprintf("setw -g pane-base-index %d\n", cfg.BaseIndex))
	sb.WriteString("set -g renumber-windows on\n")
	if cfg.AggressiveResize {
		sb.WriteString("setw -g aggressive-resize on\n")
	} else {
		sb.WriteString("setw -g aggressive-resize off\n")
	}
	sb.WriteString(fmt.Sprintf("set -g pane-border-lines %s\n\n", paneBorderToTmuxFormat(cfg.PaneBorderStyle)))

	// Status bar
	sb.WriteString("# Status bar\n")
	sb.WriteString(fmt.Sprintf("set -g status-position %s\n", cfg.StatusBar))
	sb.WriteString("set -g status-left-length 30\n")
	sb.WriteString("set -g status-right-length 50\n\n")

	// Performance
	sb.WriteString("# Performance\n")
	sb.WriteString(fmt.Sprintf("set -sg escape-time %d\n", cfg.EscapeTime))
	sb.WriteString(fmt.Sprintf("set -g history-limit %d\n\n", cfg.HistoryLimit))

	// Reload binding
	sb.WriteString("# Reload config\n")
	sb.WriteString(fmt.Sprintf("bind r source-file %s \\; display \"Config reloaded!\"\n\n", reloadExpr))

	// Quick window switching
	sb.WriteString("# Quick window switching (Alt + number)\n")
	for i := 1; i <= 5; i++ {
		sb.WriteString(fmt.Sprintf("bind -n M-%d select-window -t %d\n", i, i))
	}
	sb.WriteString("\n")

	// Pane navigation
	sb.WriteString("# Pane navigation (Alt + arrow)\n")
	sb.WriteString("bind -n M-Left select-pane -L\n")
	sb.WriteString("bind -n M-Right select-pane -R\n")
	sb.WriteString("bind -n M-Up select-pane -U\n")
	sb.WriteString("bind -n M-Down select-pane -D\n\n")

	// Pane navigation (Alt + vim keys) — mirrors Alt-Arrow for vim-style nav
	sb.WriteString("# Pane navigation (Alt + h/j/k/l)\n")
	sb.WriteString("bind -n M-h select-pane -L\n")
	sb.WriteString("bind -n M-j select-pane -D\n")
	sb.WriteString("bind -n M-k select-pane -U\n")
	sb.WriteString("bind -n M-l select-pane -R\n\n")

	// Pane resizing (Prefix + H/J/K/L, repeatable)
	sb.WriteString("# Pane resizing (Prefix + H/J/K/L)\n")
	sb.WriteString("bind -r H resize-pane -L 5\n")
	sb.WriteString("bind -r J resize-pane -D 5\n")
	sb.WriteString("bind -r K resize-pane -U 5\n")
	sb.WriteString("bind -r L resize-pane -R 5\n\n")

	// TPM configuration
	if cfg.TPMEnabled {
		sb.WriteString("# TPM (Tmux Plugin Manager)\n")
		sb.WriteString("set -g @plugin 'tmux-plugins/tpm'\n")

		// Add enabled plugins
		if cfg.PluginSensible {
			sb.WriteString("set -g @plugin 'tmux-plugins/tmux-sensible'\n")
		}
		if cfg.PluginResurrect {
			sb.WriteString("set -g @plugin 'tmux-plugins/tmux-resurrect'\n")
		}
		if cfg.PluginContinuum {
			sb.WriteString("set -g @plugin 'tmux-plugins/tmux-continuum'\n")
		}
		if cfg.PluginYank {
			sb.WriteString("set -g @plugin 'tmux-plugins/tmux-yank'\n")
		}
		sb.WriteString("\n")

		// Plugin-specific settings
		if cfg.PluginContinuum {
			sb.WriteString("# Continuum settings\n")
			sb.WriteString(fmt.Sprintf("set -g @continuum-save-interval '%d'\n", cfg.ContinuumSaveMin))
			restore := "off"
			if cfg.ContinuumRestore {
				restore = "on"
			}
			sb.WriteString(fmt.Sprintf("set -g @continuum-restore '%s'\n\n", restore))
		}

		if cfg.PluginResurrect {
			sb.WriteString("# Resurrect settings\n")
			sb.WriteString("set -g @resurrect-capture-pane-contents 'on'\n")
			sb.WriteString("set -g @resurrect-strategy-nvim 'session'\n\n")
		}

		// TPM init (must be at the very end)
		sb.WriteString("# Initialize TPM (keep at bottom)\n")
		sb.WriteString("run '~/.tmux/plugins/tpm/tpm'\n")
	}

	return sb.String()
}

// prefixToTmuxFormat converts config format to tmux format
func prefixToTmuxFormat(prefix string) string {
	switch prefix {
	case "ctrl-a":
		return "C-a"
	case "ctrl-b":
		return "C-b"
	case "ctrl-space":
		return "C-Space"
	default:
		return "C-a"
	}
}

// paneBorderToTmuxFormat maps the Manage UI's pane-border vocabulary onto the
// values tmux's pane-border-lines option accepts. tmux supports
// single/double/heavy/simple/number; the UI offers the first four, so anything
// unrecognized falls back to "single".
func paneBorderToTmuxFormat(style string) string {
	switch style {
	case "single", "double", "heavy", "simple":
		return style
	default:
		return "single"
	}
}

// WriteTmuxConfig writes the tmux.conf file
func WriteTmuxConfig(cfg TmuxConfig, theme string) error {
	_, err := WriteTmuxConfigTracked(cfg, theme)
	return err
}

func WriteTmuxConfigTracked(cfg TmuxConfig, theme string) (MutationEvidence, error) {
	return writeTmuxConfigAtRevisionTracked(cfg, theme, nil, nil)
}

// WriteTmuxConfigAtRevisionTracked applies a plan-accepted tmux revision.
func WriteTmuxConfigAtRevisionTracked(cfg TmuxConfig, theme string, accepted safefile.Revision) (MutationEvidence, error) {
	if !accepted.Tracked() {
		return MutationEvidence{}, fmt.Errorf("%w: accepted tmux revision is untracked", safefile.ErrRevisionChanged)
	}
	return writeTmuxConfigAtRevisionTracked(cfg, theme, &accepted, nil)
}

func WriteTmuxConfigAtAuthorityTracked(cfg TmuxConfig, theme string, accepted safefile.Revision, parents *safefile.ParentChain, locker operation.Locker) (MutationEvidence, error) {
	if !parents.Tracked() || locker == nil {
		return MutationEvidence{}, fmt.Errorf("%w: accepted tmux authority is incomplete", safefile.ErrParentChanged)
	}
	return writeTmuxConfigAtRevisionTracked(cfg, theme, &accepted, parents, locker)
}

func writeTmuxConfigAtRevisionTracked(cfg TmuxConfig, theme string, accepted *safefile.Revision, parents *safefile.ParentChain, lockers ...operation.Locker) (MutationEvidence, error) {
	target, err := tmuxConfigMutationTarget()
	if err != nil {
		return MutationEvidence{}, err
	}
	configPath := target.path
	managed := wrapManagedConfigSection(tmuxManagedStart, tmuxManagedEnd, generateTmuxConfig(cfg, theme, target.reloadExpr))
	lock := withToolConfigLock
	if accepted != nil {
		locker := operation.DefaultLocker
		if len(lockers) != 0 && lockers[0] != nil {
			locker = lockers[0]
		}
		lock = func(path string, mutate func(string, string) error) error {
			return withToolConfigLockAuthorized(path, locker, mutate)
		}
	}

	var evidence MutationEvidence
	err = lock(configPath, func(root, rel string) error {
		var existing []byte
		var revision safefile.Revision
		var readErr error
		if parents != nil {
			existing, revision, readErr = safefile.ReadWithinAuthorized(root, rel, parents)
		} else {
			existing, revision, readErr = readToolConfig(root, rel)
		}
		if readErr != nil {
			return readErr
		}
		if accepted != nil && revision != *accepted {
			return fmt.Errorf("%w: tmux config changed after plan acceptance", safefile.ErrRevisionChanged)
		}

		merged := managed
		if revision.Exists() {
			if hasGeneratedConfigHeader(existing) {
				// Prototype builds owned the complete file. Migrate that exact
				// ownership shape to the delimited fragment without retaining a
				// second active copy of the old generated settings.
				merged = managed
			} else {
				var ownershipAdded bool
				merged, ownershipAdded, readErr = mergeManagedConfigSection(existing, managed, tmuxManagedStart, tmuxManagedEnd, "tmux config")
				if readErr != nil {
					return readErr
				}
				if ownershipAdded && accepted == nil {
					return fmt.Errorf("%w: native tmux config requires reviewed adoption with a rollback point", ErrUnmanagedConfig)
				}
			}
		}
		if revision.Exists() && bytes.Equal(existing, merged) {
			evidence = MutationEvidence{Path: configPath, Revision: revision, Parents: parents}
			return nil
		}

		expected := revision
		if accepted != nil {
			expected = *accepted
		}
		var committed safefile.Revision
		if accepted != nil {
			if parents != nil {
				committed, readErr = replaceToolConfigAtRevisionNoCreateAuthorizedTracked(root, rel, expected, parents, merged)
			} else {
				committed, readErr = replaceToolConfigAtRevisionNoCreateTracked(root, rel, expected, merged)
			}
		} else {
			committed, readErr = replaceToolConfigAtRevisionTracked(root, rel, expected, merged)
		}
		if readErr != nil {
			return fmt.Errorf("write managed tmux config: %w", readErr)
		}
		evidence = MutationEvidence{Path: configPath, Revision: committed, Parents: parents}
		return nil
	})
	return evidence, err
}

// SetupTPM handles TPM installation and plugin setup
func SetupTPM(cfg TmuxConfig, theme string) error {
	_, err := SetupTPMTracked(cfg, theme)
	return err
}

func SetupTPMTracked(cfg TmuxConfig, theme string) ([]MutationEvidence, error) {
	return SetupTPMTrackedWithContext(context.Background(), cfg, theme)
}

// SetupTPMTrackedWithContext keeps remote acquisition and every later mutation
// boundary under the caller's operation lifetime.
func SetupTPMTrackedWithContext(ctx context.Context, cfg TmuxConfig, theme string) ([]MutationEvidence, error) {
	if ctx == nil {
		return nil, errors.New("TPM operation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Write config first
	configEvidence, err := WriteTmuxConfigTracked(cfg, theme)
	if err != nil {
		return nil, err
	}
	committed := []MutationEvidence{configEvidence}

	if !cfg.TPMEnabled {
		return committed, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, partialMutationError(err, committed)
	}

	// Install TPM transactionally. Existing targets are refused at the snapshot
	// commit boundary instead of being trusted because they appeared mid-plan.
	tpmEvidence, err := installTPMTrackedWithContext(ctx)
	if err != nil {
		if tpmEvidence.Directory != nil || tpmEvidence.Revision.Tracked() {
			committed = append(committed, tpmEvidence)
		}
		return nil, partialMutationError(err, committed)
	}
	committed = append(committed, tpmEvidence)

	// Plugin installation remains an explicit prefix+I user action. The third-
	// party installer has a broader mutation surface than this reviewed plan.
	return committed, nil
}

// SetupTPMAtAuthorityTracked applies only exact plan-accepted tmux and TPM
// namespace authority with one already-bound operational state.
func SetupTPMAtAuthorityTracked(cfg TmuxConfig, theme string, tmuxAccepted safefile.Revision, tmuxParents *safefile.ParentChain, tpmAccepted *safefile.DirectorySnapshot, tpmParents *safefile.ParentChain, state *operation.StateAuthority) ([]MutationEvidence, error) {
	return SetupTPMAtAuthorityTrackedWithContext(context.Background(), cfg, theme, tmuxAccepted, tmuxParents, tpmAccepted, tpmParents, state)
}

// SetupTPMAtAuthorityTrackedWithContext is the reviewed install entry point.
// Compatibility callers retain the context-free wrapper above.
func SetupTPMAtAuthorityTrackedWithContext(ctx context.Context, cfg TmuxConfig, theme string, tmuxAccepted safefile.Revision, tmuxParents *safefile.ParentChain, tpmAccepted *safefile.DirectorySnapshot, tpmParents *safefile.ParentChain, state *operation.StateAuthority) ([]MutationEvidence, error) {
	if ctx == nil {
		return nil, errors.New("TPM operation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determine HOME for TPM: %w", err)
	}
	if !tmuxAccepted.Tracked() || !tmuxParents.Tracked() || state == nil {
		return nil, fmt.Errorf("%w: accepted tmux/TPM authority is incomplete", safefile.ErrParentChanged)
	}
	// Preflight the complete accepted set before the first config mutation.
	if cfg.TPMEnabled {
		if !tpmParents.Tracked() {
			return nil, fmt.Errorf("%w: accepted TPM parent authority is incomplete", safefile.ErrParentChanged)
		}
		if err := safefile.VerifyDirectoryWithinSnapshot(home, filepath.ToSlash(filepath.Join(".tmux", "plugins", "tpm")), tpmAccepted); err != nil {
			return nil, fmt.Errorf("preflight accepted TPM target: %w", err)
		}
	}
	configEvidence, err := WriteTmuxConfigAtAuthorityTracked(cfg, theme, tmuxAccepted, tmuxParents, operation.BoundLocker(state))
	if err != nil {
		return nil, err
	}
	var committed []MutationEvidence
	if configEvidence.Revision != tmuxAccepted {
		committed = append(committed, configEvidence)
	}
	if !cfg.TPMEnabled {
		return []MutationEvidence{configEvidence}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, partialMutationError(err, committed)
	}
	tpmEvidence, err := installTPMAtSnapshotTrackedWithContext(ctx, tpmAccepted, tpmParents, state)
	if err != nil {
		if tpmEvidence.Directory != nil || tpmEvidence.Revision.Tracked() {
			committed = append(committed, tpmEvidence)
		}
		return nil, partialMutationError(err, committed)
	}
	return []MutationEvidence{configEvidence, tpmEvidence}, nil
}
