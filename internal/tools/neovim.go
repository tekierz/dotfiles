package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/safefile"
	"github.com/tekierz/dotfiles/internal/theme"
)

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
		installed, err := setupNeovimPresetTracked(cfg, theme, nvimDir)
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
		return MutationEvidence{}, fmt.Errorf("failed to get home directory: %w", err)
	}
	nvimDir := filepath.Join(home, ".config", "nvim")
	switch cfg.ConfigPreset {
	case "kickstart", "lazyvim", "nvchad":
		installed, err := setupNeovimPresetAtSnapshotTracked(cfg, theme, nvimDir, accepted)
		if err != nil {
			var committed []MutationEvidence
			if installed != nil {
				committed = append(committed, MutationEvidence{Path: nvimDir, Directory: installed})
			}
			return MutationEvidence{}, partialMutationError(err, committed)
		}
		return MutationEvidence{Path: nvimDir, Directory: installed}, nil
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
	return setupNeovimPresetAtSnapshotTracked(cfg, theme, nvimDir, nil)
}

func setupNeovimPresetAtSnapshotTracked(cfg NeovimConfig, theme, nvimDir string, accepted *safefile.DirectorySnapshot) (result *safefile.DirectorySnapshot, returnErr error) {
	repoURL, ok := neovimConfigRepos[cfg.ConfigPreset]
	if !ok {
		return nil, fmt.Errorf("refusing unknown Neovim preset %q", cfg.ConfigPreset)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	targetRel := filepath.ToSlash(filepath.Join(".config", "nvim"))
	if err := safefile.VerifyDirectoryWithinSnapshot(home, targetRel, accepted); err != nil {
		return nil, fmt.Errorf("preflight accepted Neovim preset target: %w", err)
	}

	// Clone under the trusted home root, then copy the validated snapshot into
	// place with an absence precondition. This keeps network/git work away from
	// the live namespace and refuses a target created after planning.
	tempDir, err := os.MkdirTemp(home, ".nvim-clone-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary neovim config directory: %w", err)
	}
	tempRel := filepath.Base(tempDir)
	defer func() {
		if cleanupErr := safefile.RemoveDirectoryWithin(home, tempRel); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			returnErr = errors.Join(returnErr, fmt.Errorf("clean Neovim clone staging directory: %w", cleanupErr))
		}
	}()

	// Clone the preset into a temp directory first.  The user's existing config
	// must not be moved unless the network/git operation has fully succeeded.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// #nosec G204 -- repoURL is selected from the immutable neovimConfigRepos allowlist.
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", repoURL, tempDir)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to clone %s config: %w", cfg.ConfigPreset, err)
	}

	// Remove clone metadata through the same descriptor-anchored directory API
	// used by rollback, then install only the captured user-owned tree.
	if err := safefile.RemoveDirectoryWithin(home, filepath.ToSlash(filepath.Join(tempRel, ".git"))); err != nil {
		return nil, fmt.Errorf("remove Neovim preset git metadata: %w", err)
	}
	if err := writeNeovimStagedPrefs(home, tempRel, cfg, theme); err != nil {
		return nil, fmt.Errorf("apply preferences to staged Neovim preset: %w", err)
	}
	presetSnapshot, err := safefile.SnapshotDirectoryWithin(home, tempRel)
	if err != nil {
		return nil, fmt.Errorf("capture staged Neovim preset: %w", err)
	}
	installed, err := safefile.RestoreDirectoryWithinSnapshotTracked(home, targetRel, presetSnapshot, accepted)
	if err != nil {
		return installed, fmt.Errorf("install Neovim preset: %w", err)
	}
	return installed, nil
}

func writeNeovimStagedPrefs(home, stagingRel string, cfg NeovimConfig, theme string) error {
	initRel := filepath.ToSlash(filepath.Join(stagingRel, "init.lua"))
	initContent, initRevision, err := safefile.ReadWithin(home, initRel)
	if err != nil {
		return fmt.Errorf("read staged init.lua: %w", err)
	}
	if !initRevision.Exists() {
		return errors.New("staged Neovim preset has no init.lua")
	}
	requireLine := "pcall(require, \"custom.options\")"
	if !strings.Contains(string(initContent), requireLine) {
		updated := string(initContent) + "\n\n-- User options from dotfiles\n" + requireLine + "\n"
		if err := safefile.ReplaceWithinRevision(home, initRel, initRevision, []byte(updated), 0600); err != nil {
			return fmt.Errorf("update staged init.lua: %w", err)
		}
	}
	prefsRel := filepath.ToSlash(filepath.Join(stagingRel, "lua", "custom", "options.lua"))
	if err := safefile.ReplaceWithin(home, prefsRel, []byte(GenerateNeovimConfig(cfg, theme)), 0600); err != nil {
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

		prefsPath := filepath.Join(nvimDir, "lua", "custom", "options.lua")
		content := GenerateNeovimConfig(cfg, theme)
		if err := writeToolConfig(prefsPath, []byte(content)); err != nil {
			return err
		}

		// Add require to init.lua if not already present. The revision check in
		// replaceToolConfigAtRevision prevents a stale read from erasing edits
		// made by a non-cooperating process after the preflight above.
		requireLine := "pcall(require, \"custom.options\")"
		if !strings.Contains(string(initContent), requireLine) {
			newContent := string(initContent) + "\n\n-- User options from dotfiles\n" + requireLine + "\n"
			if err := replaceToolConfigAtRevision(root, rel, revision, []byte(newContent)); err != nil {
				return fmt.Errorf("failed to update neovim init.lua: %w", err)
			}
		}

		return nil
	})
}

// writeMinimalNeovimConfig writes a minimal standalone neovim config
func writeMinimalNeovimConfig(cfg NeovimConfig, theme, nvimDir string) error {
	initPath := filepath.Join(nvimDir, "init.lua")
	content := GenerateNeovimConfig(cfg, theme)
	return writeToolConfig(initPath, []byte(content))
}
