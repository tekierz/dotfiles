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
	"github.com/tekierz/dotfiles/internal/theme"
)

const (
	neovimManagedStart = "-- >>> dotfiles neovim (managed)"
	neovimManagedEnd   = "-- <<< dotfiles neovim (managed)"
	neovimRequireLine  = `pcall(require, "custom.options")`
)

var neovimLegacyAppend = []byte("\n\n-- User options from dotfiles\n" + neovimRequireLine + "\n")

// NeovimConfig holds Neovim configuration settings
type NeovimConfig struct {
	ConfigPreset string   // "kickstart", "lazyvim", "custom", "minimal"
	LSPs         []string // LSP servers to configure
	Plugins      []string // Plugins to enable
	TabWidth     int
	Wrap         bool
	CursorLine   bool
	Clipboard    string // "unnamedplus", "unnamed", "none"

	// LineNumbers is the SINGLE control for the line-number gutter:
	//   "absolute" -> number=true,  relativenumber=false
	//   "relative" -> number=true,  relativenumber=true
	//   "none"     -> number=false, relativenumber=false
	LineNumbers string
	ExpandTab   bool // use spaces instead of tabs
	UndoFile    bool // persistent undo on disk
}

// NeovimTool represents the Neovim editor
type NeovimTool struct {
	BaseTool
}

// NewNeovimTool creates a new Neovim tool
func NewNeovimTool() *NeovimTool {
	home, _ := os.UserHomeDir()
	return &NeovimTool{
		BaseTool: BaseTool{
			id:          "neovim",
			name:        "Neovim",
			description: "Hyperextensible Vim-based text editor",
			icon:        "",
			category:    CategoryEditor,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"neovim"},
				pkg.PlatformArch:   {"neovim"},
				pkg.PlatformDebian: {"neovim"},
			},
			configPaths: []string{
				filepath.Join(home, ".config", "nvim", "init.lua"),
			},
			// UI metadata
			uiGroup:        UIGroupNone,
			configScreen:   12, // ScreenConfigNeovim
			defaultEnabled: true,
		},
	}
}

// neovimConfigRepos maps preset names to their git repositories.
// Every preset listed here is cloned into ~/.config/nvim via setupNeovimPreset.
// The "custom" preset is intentionally absent — it is non-destructive and
// preserves whatever config the user already has.
var neovimConfigRepos = map[string]string{
	"kickstart": "https://github.com/nvim-lua/kickstart.nvim.git",
	"lazyvim":   "https://github.com/LazyVim/starter.git",
	"nvchad":    "https://github.com/NvChad/starter.git",
}

// ValidNeovimPresetOrder is the authoritative ordered list of preset identifiers
// in the order the config screen displays them.  screen_config_neovim.go MUST
// consume this slice for its option list — that is the compile-time guarantee
// that UI options and generator dispatch cannot drift independently.
var ValidNeovimPresetOrder = []string{"kickstart", "lazyvim", "nvchad", "custom"}

// ValidNeovimPresets is derived from ValidNeovimPresetOrder and provides O(1)
// membership checks.  Do not edit this directly; edit ValidNeovimPresetOrder.
var ValidNeovimPresets = func() map[string]struct{} {
	m := make(map[string]struct{}, len(ValidNeovimPresetOrder))
	for _, p := range ValidNeovimPresetOrder {
		m[p] = struct{}{}
	}
	return m
}()

type neovimLSPServer struct {
	masonPackage string
	configName   string
}

// neovimLSPServers maps the LSP identifiers exposed by the config screen to
// their canonical Mason package names and vim.lsp config names. Mason downloads
// the language-server binary; vim.lsp.enable activates the matching config when
// the preset/lspconfig provides one. Ids not present here are skipped rather
// than guessed.
var neovimLSPServers = map[string]neovimLSPServer{
	"lua_ls":        {masonPackage: "lua-language-server", configName: "lua_ls"},
	"pyright":       {masonPackage: "pyright", configName: "pyright"},
	"tsserver":      {masonPackage: "typescript-language-server", configName: "ts_ls"},
	"gopls":         {masonPackage: "gopls", configName: "gopls"},
	"rust_analyzer": {masonPackage: "rust-analyzer", configName: "rust_analyzer"},
	"clangd":        {masonPackage: "clangd", configName: "clangd"},
}

// neovimLSPInstallLua installs the servers named in the file-local
// `dotfiles_lsp_servers` table through Mason, then enables the matching names
// from `dotfiles_lsp_config_servers` through Neovim's 0.11+ LSP API when the
// preset has not already enabled them. It never calls lspconfig/setup, so the
// cloned preset still owns attachment/config details. Every step is
// pcall-guarded, the mason-registry require is deferred to VimEnter (so a config
// without Mason is a silent no-op and Mason is not force-loaded mid-startup), and
// `:is_installed()` makes re-runs idempotent.
const neovimLSPInstallLua = `pcall(function()
  local function dotfiles_lsp_already_enabled(name)
    if not vim.lsp then
      return true
    end
    if type(vim.lsp.is_enabled) == "function" then
      local ok_enabled, enabled = pcall(vim.lsp.is_enabled, name)
      if ok_enabled and enabled then
        return true
      end
    end
    if type(vim.lsp.get_clients) == "function" then
      local ok_clients, clients = pcall(vim.lsp.get_clients, { name = name })
      if ok_clients and type(clients) == "table" and #clients > 0 then
        return true
      end
    end
    return false
  end
  local function dotfiles_enable_lsp_servers()
    if not vim.lsp or type(vim.lsp.enable) ~= "function" then
      return
    end
    for _, name in ipairs(dotfiles_lsp_config_servers) do
      if not dotfiles_lsp_already_enabled(name) then
        pcall(vim.lsp.enable, name)
      end
    end
  end
  local function dotfiles_schedule_lsp_enable()
    if type(vim.schedule) == "function" then
      pcall(vim.schedule, dotfiles_enable_lsp_servers)
    else
      pcall(dotfiles_enable_lsp_servers)
    end
  end
  vim.api.nvim_create_autocmd("VimEnter", {
    once = true,
    callback = function()
      local ok, registry = pcall(require, "mason-registry")
      if not ok then
        return
      end
      local function ensure()
        for _, name in ipairs(dotfiles_lsp_servers) do
          local ok_pkg, pkg = pcall(registry.get_package, name)
          if ok_pkg and pkg and not pkg:is_installed() then
            pcall(function()
              pkg:install()
            end)
          end
        end
        dotfiles_schedule_lsp_enable()
      end
      if type(registry.refresh) == "function" then
        registry.refresh(ensure)
      else
        ensure()
      end
    end,
  })
end)
`

// GenerateNeovimConfig builds basic neovim settings as a Lua string.
// For preset configs (kickstart, lazyvim), this generates a user preferences file.
func GenerateNeovimConfig(cfg NeovimConfig, themeName string) string {
	var sb strings.Builder
	p := theme.GetOrDefault(themeName)
	colorscheme := neovimColorschemeForTheme(themeName)

	// Header
	sb.WriteString("-- Generated by dotfiles TUI\n")
	sb.WriteString(fmt.Sprintf("-- Theme: %s\n\n", themeName))

	// Basic settings
	sb.WriteString("-- Basic settings\n")
	// LineNumbers is the single source for both opts: "absolute"/"relative" show
	// the number column ("none" hides it); only "relative" enables relative numbers.
	sb.WriteString(fmt.Sprintf("vim.opt.number = %t\n", cfg.LineNumbers != "none"))
	sb.WriteString(fmt.Sprintf("vim.opt.relativenumber = %t\n", cfg.LineNumbers == "relative"))
	sb.WriteString(fmt.Sprintf("vim.opt.tabstop = %d\n", cfg.TabWidth))
	sb.WriteString(fmt.Sprintf("vim.opt.shiftwidth = %d\n", cfg.TabWidth))
	sb.WriteString(fmt.Sprintf("vim.opt.expandtab = %t\n", cfg.ExpandTab))
	sb.WriteString(fmt.Sprintf("vim.opt.wrap = %t\n", cfg.Wrap))
	sb.WriteString(fmt.Sprintf("vim.opt.cursorline = %t\n", cfg.CursorLine))
	sb.WriteString(fmt.Sprintf("vim.opt.undofile = %t\n", cfg.UndoFile))
	sb.WriteString("\n")

	// Clipboard
	sb.WriteString("-- Clipboard\n")
	if cfg.Clipboard != "none" {
		sb.WriteString(fmt.Sprintf("vim.opt.clipboard = \"%s\"\n", cfg.Clipboard))
	}
	sb.WriteString("\n")

	// Search settings
	sb.WriteString("-- Search\n")
	sb.WriteString("vim.opt.ignorecase = true\n")
	sb.WriteString("vim.opt.smartcase = true\n")
	sb.WriteString("vim.opt.hlsearch = true\n")
	sb.WriteString("vim.opt.incsearch = true\n\n")

	// UI settings
	sb.WriteString("-- UI\n")
	sb.WriteString("vim.opt.termguicolors = true\n")
	sb.WriteString("vim.opt.signcolumn = \"yes\"\n")
	sb.WriteString("vim.opt.scrolloff = 8\n")
	sb.WriteString("vim.opt.sidescrolloff = 8\n\n")

	// Theme
	sb.WriteString("-- Theme\n")
	if colorscheme != "" {
		sb.WriteString(fmt.Sprintf("pcall(vim.cmd.colorscheme, %q)\n", colorscheme))
	} else {
		sb.WriteString("-- Keep the preset's active colorscheme when no bundled match is available.\n")
	}
	// Full per-theme Neovim theming would require installing the matching
	// colorscheme plugin, which is out of scope; this only keeps dark themes
	// aligned without creating a light-background/dark-syntax clash.
	if isDarkHexColor(p.Bg) {
		sb.WriteString(fmt.Sprintf("vim.api.nvim_set_hl(0, \"Normal\", { fg = %q, bg = %q })\n", p.Text, p.Bg))
		sb.WriteString(fmt.Sprintf("vim.api.nvim_set_hl(0, \"NormalFloat\", { fg = %q, bg = %q })\n", p.Text, p.Surface))
		sb.WriteString(fmt.Sprintf("vim.api.nvim_set_hl(0, \"FloatBorder\", { fg = %q, bg = %q })\n", p.Border, p.Surface))
		sb.WriteString(fmt.Sprintf("vim.api.nvim_set_hl(0, \"Visual\", { fg = %q, bg = %q })\n\n", p.TextBright, p.Overlay))
	} else {
		sb.WriteString("\n")
	}

	// Performance
	sb.WriteString("-- Performance\n")
	sb.WriteString("vim.opt.updatetime = 250\n")
	sb.WriteString("vim.opt.timeoutlen = 300\n\n")

	// Leader key
	sb.WriteString("-- Leader\n")
	sb.WriteString("vim.g.mapleader = \" \"\n")
	sb.WriteString("vim.g.maplocalleader = \" \"\n")

	// LSP servers.
	// The config screen's "LSP Servers" checkboxes are wired here: each selected
	// server is mapped to its Mason package and vim.lsp config name, then
	// installed/enabled via neovimLSPInstallLua. This makes the selection real —
	// ticking Go installs and enables gopls, Rust installs and enables
	// rust_analyzer — while leaving the preset's own LSP setup untouched, so it
	// cannot break the cloned config.
	var masonPkgs []string
	var lspConfigNames []string
	for _, id := range cfg.LSPs {
		if server, ok := neovimLSPServers[id]; ok {
			masonPkgs = append(masonPkgs, server.masonPackage)
			lspConfigNames = append(lspConfigNames, server.configName)
		}
	}
	if len(masonPkgs) > 0 {
		sb.WriteString("\n-- LSP servers (installed via Mason)\n")
		sb.WriteString("local dotfiles_lsp_servers = {")
		for i, pkgName := range masonPkgs {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%q", pkgName))
		}
		sb.WriteString("}\n")
		sb.WriteString("local dotfiles_lsp_config_servers = {")
		for i, configName := range lspConfigNames {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%q", configName))
		}
		sb.WriteString("}\n")
		sb.WriteString(neovimLSPInstallLua)
	}

	return sb.String()
}

func neovimColorschemeForTheme(themeName string) string {
	switch themeName {
	case "tokyo-night":
		return "tokyonight"
	default:
		return ""
	}
}

// WriteNeovimConfig writes the neovim configuration to disk
func WriteNeovimConfig(cfg NeovimConfig, theme string) error {
	_, err := WriteNeovimConfigTracked(cfg, theme)
	return err
}

func WriteNeovimConfigTracked(cfg NeovimConfig, theme string) (MutationEvidence, error) {
	return WriteNeovimConfigTrackedWithContext(context.Background(), cfg, theme)
}

// WriteNeovimConfigTrackedWithContext keeps preset acquisition and staged
// mutations under the caller's operation lifetime.
func WriteNeovimConfigTrackedWithContext(ctx context.Context, cfg NeovimConfig, theme string) (MutationEvidence, error) {
	if ctx == nil {
		return MutationEvidence{}, errors.New("Neovim operation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return MutationEvidence{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return MutationEvidence{}, fmt.Errorf("failed to get home directory: %w", err)
	}

	nvimDir := filepath.Join(home, ".config", "nvim")

	// Handle preset configurations. Every install-time mutation must return
	// evidence captured by the directory commit itself; a later path snapshot
	// could accidentally authorize rollback over a superseding writer.
	switch cfg.ConfigPreset {
	case "kickstart", "lazyvim", "nvchad":
		installed, err := setupNeovimPresetTrackedWithContext(ctx, cfg, theme, nvimDir)
		if err != nil {
			var committed []MutationEvidence
			if installed != nil {
				committed = append(committed, MutationEvidence{Path: nvimDir, Directory: installed})
			}
			return MutationEvidence{}, partialMutationError(err, committed)
		}
		return MutationEvidence{Path: nvimDir, Directory: installed}, nil
	case "custom":
		// "custom" means "leave the user's existing config alone".  Do not write
		// anything — the user manages their own ~/.config/nvim.
		return MutationEvidence{}, nil
	default:
		return MutationEvidence{}, fmt.Errorf("refusing unknown Neovim preset %q", cfg.ConfigPreset)
	}
}

// WriteNeovimConfigAtSnapshotTracked applies a plan-accepted Neovim tree.
func WriteNeovimConfigAtSnapshotTracked(cfg NeovimConfig, theme string, accepted *safefile.DirectorySnapshot) (MutationEvidence, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return MutationEvidence{}, err
	}
	parents, err := safefile.CaptureParentChainWithin(home, filepath.ToSlash(filepath.Join(".config", "nvim")))
	if err != nil {
		return MutationEvidence{}, err
	}
	statePlan, err := operation.CaptureStatePlan()
	if err != nil {
		return MutationEvidence{}, err
	}
	state, err := operation.BootstrapStateNamespaceTracked(statePlan)
	if err != nil {
		return MutationEvidence{}, err
	}
	return WriteNeovimConfigAtBoundAuthorityTracked(cfg, theme, accepted, parents, state)
}

func WriteNeovimConfigAtBoundAuthorityTracked(cfg NeovimConfig, theme string, accepted *safefile.DirectorySnapshot, parents *safefile.ParentChain, state *operation.StateAuthority) (MutationEvidence, error) {
	return WriteNeovimConfigAtBoundAuthorityTrackedWithContext(context.Background(), cfg, theme, accepted, parents, state)
}

// WriteNeovimConfigAtBoundAuthorityTrackedWithContext is the reviewed install
// entry point. Compatibility callers retain the context-free wrapper above.
func WriteNeovimConfigAtBoundAuthorityTrackedWithContext(ctx context.Context, cfg NeovimConfig, theme string, accepted *safefile.DirectorySnapshot, parents *safefile.ParentChain, state *operation.StateAuthority) (MutationEvidence, error) {
	if ctx == nil {
		return MutationEvidence{}, errors.New("Neovim operation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return MutationEvidence{}, err
	}
	if !parents.Tracked() || state == nil {
		return MutationEvidence{}, fmt.Errorf("%w: accepted Neovim authority is incomplete", safefile.ErrParentChanged)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return MutationEvidence{}, fmt.Errorf("failed to get home directory: %w", err)
	}
	nvimDir := filepath.Join(home, ".config", "nvim")
	switch cfg.ConfigPreset {
	case "kickstart", "lazyvim", "nvchad":
		installed, err := setupNeovimPresetAtSnapshotTrackedWithContext(ctx, cfg, theme, nvimDir, accepted, parents, state)
		if err != nil {
			var committed []MutationEvidence
			if installed != nil {
				committed = append(committed, MutationEvidence{Path: nvimDir, Directory: installed, Parents: parents})
			}
			return MutationEvidence{}, partialMutationError(err, committed)
		}
		return MutationEvidence{Path: nvimDir, Directory: installed, Parents: parents}, nil
	case "custom":
		return MutationEvidence{}, nil
	default:
		return MutationEvidence{}, fmt.Errorf("refusing unknown Neovim preset %q", cfg.ConfigPreset)
	}
}

// setupNeovimPreset clones a preset config and adds user customizations
func setupNeovimPreset(cfg NeovimConfig, theme, nvimDir string) error {
	_, err := setupNeovimPresetTracked(cfg, theme, nvimDir)
	return err
}

func setupNeovimPresetTracked(cfg NeovimConfig, theme, nvimDir string) (result *safefile.DirectorySnapshot, returnErr error) {
	return setupNeovimPresetTrackedWithContext(context.Background(), cfg, theme, nvimDir)
}

func setupNeovimPresetTrackedWithContext(ctx context.Context, cfg NeovimConfig, theme, nvimDir string) (result *safefile.DirectorySnapshot, returnErr error) {
	if ctx == nil {
		return nil, errors.New("Neovim operation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	targetRel := filepath.ToSlash(filepath.Join(".config", "nvim"))
	if _, err := safefile.SnapshotDirectoryWithin(home, targetRel); err == nil {
		return nil, fmt.Errorf("refusing to install Neovim preset over existing directory %s", nvimDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect Neovim preset target: %w", err)
	}
	return setupNeovimPresetAtSnapshotTrackedWithContext(ctx, cfg, theme, nvimDir, nil, nil)
}

func setupNeovimPresetAtSnapshotTrackedWithContext(ctx context.Context, cfg NeovimConfig, theme, nvimDir string, accepted *safefile.DirectorySnapshot, parents *safefile.ParentChain, states ...*operation.StateAuthority) (result *safefile.DirectorySnapshot, returnErr error) {
	if ctx == nil {
		return nil, errors.New("Neovim operation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, ok := neovimConfigRepos[cfg.ConfigPreset]
	if !ok {
		return nil, fmt.Errorf("refusing unknown Neovim preset %q", cfg.ConfigPreset)
	}
	artifact, err := NeovimRemoteArtifact(cfg.ConfigPreset)
	if err != nil {
		return nil, fmt.Errorf("resolve pinned Neovim preset: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	targetRel := filepath.ToSlash(filepath.Join(".config", "nvim"))
	if err := safefile.VerifyDirectoryWithinSnapshot(home, targetRel, accepted); err != nil {
		return nil, fmt.Errorf("preflight accepted Neovim preset target: %w", err)
	}

	// Clone below dotfiles' private operational-state namespace, then copy the
	// validated snapshot into place with an absence precondition. This keeps
	// network/git work out of HOME's product namespace.
	var tempDir string
	var createdStaging *operation.StateStagingAuthority
	if len(states) != 0 && states[0] != nil {
		tempDir, createdStaging, err = operation.CreateStateStagingDirectoryWithAuthorityTracked(states[0], "neovim-"+cfg.ConfigPreset)
	} else {
		tempDir, createdStaging, err = operation.CreateStateStagingDirectoryTracked("neovim-" + cfg.ConfigPreset)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary neovim config directory: %w", err)
	}
	var cleanupExpected *safefile.DirectorySnapshot
	defer func() {
		if cleanupExpected == nil {
			cleanupExpected, err = operation.SnapshotStateStagingDirectory(tempDir, createdStaging)
			if err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("capture Neovim staging cleanup authority: %w", err))
				return
			}
		}
		if cleanupErr := operation.RemoveStateStagingDirectoryAuthorized(tempDir, createdStaging, cleanupExpected); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			returnErr = errors.Join(returnErr, fmt.Errorf("clean Neovim clone staging directory: %w", cleanupErr))
		}
	}()

	// Clone the preset into a temp directory first.  The user's existing config
	// must not be moved unless the network/git operation has fully succeeded.
	cloneCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	stagingFD, err := operation.OpenStateStagingDirectory(tempDir, createdStaging)
	if err != nil {
		return nil, fmt.Errorf("open exact Neovim staging directory: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, stagingFD.Close()) }()
	// The shared artifact loader fetches only the reviewed full commit and
	// verifies the staged checkout before metadata removal or live-path commit.
	commandFactory := func(ctx context.Context, name string, arguments ...string) *exec.Cmd {
		return exec.CommandContext(ctx, name, arguments...)
	}
	if err := clonePinnedGitArtifact(cloneCtx, tempDir, artifact, commandFactory); err != nil {
		return nil, fmt.Errorf("stage pinned %s config: %w", cfg.ConfigPreset, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Remove clone metadata through the same descriptor-anchored directory API
	// used by rollback, then install only the captured user-owned tree.
	gitRoot, gitRel, gitParents, err := operation.StateStagingDescendantAuthority(tempDir, createdStaging, ".git")
	if err != nil {
		return nil, fmt.Errorf("bind Neovim preset git metadata: %w", err)
	}
	gitSnapshot, observedGitParents, err := safefile.ObserveDirectoryWithin(gitRoot, gitRel)
	if err != nil || !safefile.SameParentChain(gitParents, observedGitParents) {
		return nil, fmt.Errorf("observe Neovim preset git metadata: %w", errors.Join(err, safefile.ErrParentChanged))
	}
	if err := safefile.RemoveDirectoryWithinSnapshotAuthorized(gitRoot, gitRel, gitSnapshot, gitParents); err != nil {
		return nil, fmt.Errorf("remove Neovim preset git metadata: %w", err)
	}
	if err := writeNeovimStagedPrefs(ctx, tempDir, createdStaging, cfg, theme); err != nil {
		return nil, fmt.Errorf("apply preferences to staged Neovim preset: %w", err)
	}
	presetSnapshot, err := operation.SnapshotStateStagingDirectory(tempDir, createdStaging)
	if err != nil {
		return nil, fmt.Errorf("capture staged Neovim preset: %w", err)
	}
	cleanupExpected = presetSnapshot
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var installed *safefile.DirectorySnapshot
	if accepted != nil {
		if parents != nil {
			installed, err = safefile.RestoreDirectoryWithinSnapshotNoCreateAuthorizedTracked(home, targetRel, presetSnapshot, accepted, parents)
		} else {
			installed, err = safefile.RestoreDirectoryWithinSnapshotNoCreateTracked(home, targetRel, presetSnapshot, accepted)
		}
	} else {
		installed, err = safefile.RestoreDirectoryWithinSnapshotTracked(home, targetRel, presetSnapshot, accepted)
	}
	if err != nil {
		return installed, fmt.Errorf("install Neovim preset: %w", err)
	}
	return installed, nil
}

func writeNeovimStagedPrefs(ctx context.Context, staging string, authority *operation.StateStagingAuthority, cfg NeovimConfig, theme string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, initRel, initParents, err := operation.StateStagingDescendantAuthority(staging, authority, "init.lua")
	if err != nil {
		return fmt.Errorf("bind staged init.lua: %w", err)
	}
	initContent, initRevision, err := safefile.ReadWithinAuthorized(root, initRel, initParents)
	if err != nil {
		return fmt.Errorf("read staged init.lua: %w", err)
	}
	if !initRevision.Exists() {
		return errors.New("staged Neovim preset has no init.lua")
	}
	updated, err := mergeNeovimInitFragment(initContent)
	if err != nil {
		return fmt.Errorf("merge staged init.lua preferences loader: %w", err)
	}
	if !bytes.Equal(initContent, updated) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := safefile.ReplaceWithinRevisionNoCreateAuthorizedTracked(root, initRel, initRevision, initParents, updated, 0600); err != nil {
			return fmt.Errorf("update staged init.lua: %w", err)
		}
	}
	for _, directory := range []string{"lua", filepath.ToSlash(filepath.Join("lua", "custom"))} {
		if err := ctx.Err(); err != nil {
			return err
		}
		dirRoot, dirRel, dirParents, err := operation.StateStagingDescendantAuthority(staging, authority, directory)
		if err != nil {
			return fmt.Errorf("bind staged Neovim directory %s: %w", directory, err)
		}
		if _, _, err := safefile.CaptureDirectoryRootWithin(dirRoot, dirRel); errors.Is(err, os.ErrNotExist) {
			if _, err := safefile.EnsureShallowDirectoryWithinParentChainTracked(dirRoot, dirRel, nil, dirParents, 0o700); err != nil {
				return fmt.Errorf("create staged Neovim directory %s: %w", directory, err)
			}
		} else if err != nil {
			return fmt.Errorf("validate staged Neovim directory %s: %w", directory, err)
		}
	}
	root, prefsRel, prefsParents, err := operation.StateStagingDescendantAuthority(staging, authority, filepath.ToSlash(filepath.Join("lua", "custom", "options.lua")))
	if err != nil {
		return fmt.Errorf("bind staged options.lua: %w", err)
	}
	_, prefsRevision, err := safefile.ReadWithinAuthorized(root, prefsRel, prefsParents)
	if err != nil {
		return fmt.Errorf("read staged options.lua: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := safefile.ReplaceWithinRevisionNoCreateAuthorizedTracked(root, prefsRel, prefsRevision, prefsParents, []byte(GenerateNeovimConfig(cfg, theme)), 0600); err != nil {
		return fmt.Errorf("write staged options.lua: %w", err)
	}
	return nil
}

// WriteNeovimUserPrefs writes ONLY the user-preferences overlay
// (lua/custom/options.lua, plus a require appended to init.lua if present) into
// the existing ~/.config/nvim. It is the PURE, non-destructive counterpart of
// WriteNeovimConfig used by the config-apply path: it never clones a preset and
// never moves/removes ~/.config/nvim, so saving Neovim settings (Manage save or
// `dotfiles config neovim`) cannot do a network install or clobber the user's
// config. Preset cloning stays at install time only (installation.go).
//
// If ~/.config/nvim does not exist yet (Neovim not installed / no preset cloned),
// this is a deliberate no-op: there is no base config to overlay, and writing a
// bare options.lua there would be meaningless without the preset's init.lua. The
// preset clone at install time is what creates the directory; this writer only
// updates an existing one.
func WriteNeovimUserPrefs(cfg NeovimConfig, theme string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	nvimDir := filepath.Join(home, ".config", "nvim")

	// No existing config dir => nothing to overlay. Do NOT create it (that is the
	// install-time preset clone's job) and do NOT clone here.
	info, statErr := os.Stat(nvimDir)
	if errors.Is(statErr, os.ErrNotExist) {
		return nil
	}
	if statErr != nil {
		return fmt.Errorf("failed to inspect neovim config directory: %w", statErr)
	}
	if !info.IsDir() {
		return fmt.Errorf("neovim config path is not a directory: %s", nvimDir)
	}

	return writeNeovimUserPrefs(cfg, theme, nvimDir)
}

// WriteNeovimUserPrefsAtBoundAuthoritiesTracked applies the reviewed two-file
// overlay to an existing Neovim config. init.lua remains user-owned outside the
// exact managed fragment; options.lua is a product-owned whole file.
func WriteNeovimUserPrefsAtBoundAuthoritiesTracked(cfg NeovimConfig, theme string, initAccepted safefile.Revision, initParents *safefile.ParentChain, optionsAccepted safefile.Revision, optionsParents *safefile.ParentChain, locker operation.Locker) ([]MutationEvidence, error) {
	return writeNeovimUserPrefsAtBoundAuthoritiesTracked(cfg, theme, initAccepted, initParents, optionsAccepted, optionsParents, locker, safefile.ReplaceWithinRevisionNoCreateAuthorizedTracked)
}

type neovimAuthorizedReplace func(string, string, safefile.Revision, *safefile.ParentChain, []byte, os.FileMode) (safefile.Revision, error)

func writeNeovimUserPrefsAtBoundAuthoritiesTracked(cfg NeovimConfig, theme string, initAccepted safefile.Revision, initParents *safefile.ParentChain, optionsAccepted safefile.Revision, optionsParents *safefile.ParentChain, locker operation.Locker, replace neovimAuthorizedReplace) (results []MutationEvidence, returnErr error) {
	if !initAccepted.Tracked() || !optionsAccepted.Tracked() {
		return nil, fmt.Errorf("%w: accepted Neovim overlay revision is untracked", safefile.ErrRevisionChanged)
	}
	if !initParents.Tracked() || !optionsParents.Tracked() || locker == nil {
		return nil, fmt.Errorf("%w: accepted Neovim overlay authority is incomplete", safefile.ErrParentChanged)
	}
	if !initAccepted.Exists() || replace == nil {
		return nil, errors.New("reviewed Neovim overlay requires an existing init.lua")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	initPath := filepath.Join(home, ".config", "nvim", "init.lua")
	optionsPath := filepath.Join(home, ".config", "nvim", "lua", "custom", "options.lua")
	release, err := locker("tool-config", initPath)
	if err != nil {
		return nil, fmt.Errorf("lock reviewed Neovim overlay: %w", err)
	}
	var committed []MutationEvidence
	defer func() {
		if err := release(); err != nil {
			joined := errors.Join(returnErr, fmt.Errorf("release reviewed Neovim overlay lock: %w", err))
			if errorReportsCommittedMutation(returnErr) {
				returnErr = joined
			} else {
				returnErr = partialMutationError(joined, committed)
			}
			results = nil
		}
	}()

	initRoot, initRel, initCreateRoot, err := generatedConfigDestination(initPath)
	if err != nil || initCreateRoot {
		return nil, fmt.Errorf("resolve reviewed Neovim init.lua: %w", errors.Join(err, safefile.ErrParentChanged))
	}
	optionsRoot, optionsRel, optionsCreateRoot, err := generatedConfigDestination(optionsPath)
	if err != nil || optionsCreateRoot {
		return nil, fmt.Errorf("resolve reviewed Neovim options.lua: %w", errors.Join(err, safefile.ErrParentChanged))
	}
	initExisting, initCurrent, err := safefile.ReadWithinAuthorized(initRoot, initRel, initParents)
	if err != nil {
		return nil, fmt.Errorf("read accepted Neovim init.lua: %w", err)
	}
	if initCurrent != initAccepted || !initCurrent.Exists() {
		return nil, fmt.Errorf("%w: Neovim init.lua changed after plan acceptance", safefile.ErrRevisionChanged)
	}
	optionsExisting, optionsCurrent, err := safefile.ReadWithinAuthorized(optionsRoot, optionsRel, optionsParents)
	if err != nil {
		return nil, fmt.Errorf("read accepted Neovim options.lua: %w", err)
	}
	if optionsCurrent != optionsAccepted {
		return nil, fmt.Errorf("%w: Neovim options.lua changed after plan acceptance", safefile.ErrRevisionChanged)
	}
	if optionsCurrent.Exists() && !hasGeneratedConfigHeader(optionsExisting) {
		return nil, fmt.Errorf("%w: %s", ErrUnmanagedConfig, optionsPath)
	}
	initUpdated, err := mergeNeovimInitFragment(initExisting)
	if err != nil {
		return nil, err
	}

	optionsContent := []byte(GenerateNeovimConfig(cfg, theme))
	optionsRevision := optionsCurrent
	if !bytes.Equal(optionsExisting, optionsContent) || !optionsCurrent.Exists() {
		optionsRevision, err = replace(optionsRoot, optionsRel, optionsCurrent, optionsParents, optionsContent, 0o600)
		if err != nil {
			if optionsRevision.Tracked() && optionsRevision.Exists() && errorReportsCommittedMutation(err) {
				committed = append(committed, MutationEvidence{Path: optionsPath, Revision: optionsRevision, Parents: optionsParents})
			} else if errorReportsCommittedMutation(err) && len(committed) == 0 {
				return nil, err
			}
			return nil, partialMutationError(err, committed)
		}
	}
	optionsEvidence := MutationEvidence{Path: optionsPath, Revision: optionsRevision, Parents: optionsParents}
	results = append(results, optionsEvidence)
	if optionsEvidence.Revision != optionsAccepted {
		committed = append(committed, optionsEvidence)
	}

	initRevision := initCurrent
	if !bytes.Equal(initExisting, initUpdated) {
		initRevision, err = replace(initRoot, initRel, initCurrent, initParents, initUpdated, 0o600)
		if err != nil {
			if initRevision.Tracked() && initRevision.Exists() && errorReportsCommittedMutation(err) {
				committed = append(committed, MutationEvidence{Path: initPath, Revision: initRevision, Parents: initParents})
			} else if errorReportsCommittedMutation(err) && len(committed) == 0 {
				return nil, err
			}
			return nil, partialMutationError(err, committed)
		}
	}
	initEvidence := MutationEvidence{Path: initPath, Revision: initRevision, Parents: initParents}
	if initEvidence.Revision != initAccepted {
		committed = append(committed, initEvidence)
	}
	return append(results, initEvidence), nil
}

func mergeNeovimInitFragment(existing []byte) ([]byte, error) {
	base := append([]byte(nil), existing...)
	managed := wrapManagedConfigSection(neovimManagedStart, neovimManagedEnd, neovimRequireLine)
	startCount := len(exactManagedMarkerLines(string(base), neovimManagedStart))
	endCount := len(exactManagedMarkerLines(string(base), neovimManagedEnd))
	legacyCount := bytes.Count(base, neovimLegacyAppend)
	if legacyCount > 1 {
		return nil, errors.New("refusing ambiguous Neovim init.lua with duplicate legacy dotfiles appends")
	}
	if legacyCount == 1 && (startCount != 0 || endCount != 0) {
		return nil, errors.New("refusing ambiguous Neovim init.lua with both legacy and managed dotfiles loaders")
	}
	if legacyCount == 1 {
		return bytes.Replace(base, neovimLegacyAppend, append([]byte("\n\n"), managed...), 1), nil
	}
	if startCount == 0 && endCount == 0 {
		requireLines := exactManagedMarkerLines(string(base), neovimRequireLine)
		if len(requireLines) == 1 {
			return base, nil
		}
		if len(requireLines) > 1 {
			return nil, errors.New("refusing ambiguous Neovim init.lua with duplicate unowned options loaders")
		}
	}
	merged, _, err := mergeManagedConfigSection(base, managed, neovimManagedStart, neovimManagedEnd, "Neovim init.lua")
	return merged, err
}

// writeNeovimUserPrefs writes user preferences to a separate file
func writeNeovimUserPrefs(cfg NeovimConfig, theme, nvimDir string) error {
	initPath := filepath.Join(nvimDir, "init.lua")
	return withToolConfigLock(initPath, func(root, rel string) error {
		// Confirm that the preset entry point exists and is readable before
		// writing the overlay it must load. A missing init.lua is a deliberate
		// no-op; every other descriptor-anchored read error is surfaced. In both
		// cases options.lua remains untouched.
		initContent, revision, err := readToolConfig(root, rel)
		if err != nil {
			return fmt.Errorf("failed to read neovim init.lua: %w", err)
		}
		if !revision.Exists() {
			return nil
		}
		if err := verifyToolConfigRevision(root, rel, revision); err != nil {
			return fmt.Errorf("failed to verify neovim init.lua: %w", err)
		}
		updated, err := mergeNeovimInitFragment(initContent)
		if err != nil {
			return err
		}

		prefsPath := filepath.Join(nvimDir, "lua", "custom", "options.lua")
		content := GenerateNeovimConfig(cfg, theme)
		if err := writeToolConfig(prefsPath, []byte(content)); err != nil {
			return err
		}

		if !bytes.Equal(initContent, updated) {
			if err := replaceToolConfigAtRevision(root, rel, revision, updated); err != nil {
				return fmt.Errorf("failed to update neovim init.lua: %w", err)
			}
		}

		return nil
	})
}
