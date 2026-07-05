package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tekierz/dotfiles/internal/pkg"
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
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	nvimDir := filepath.Join(home, ".config", "nvim")

	// Handle preset configurations.  Every case here must correspond to an entry
	// in ValidNeovimPresets; adding a new preset requires updating both.
	switch cfg.ConfigPreset {
	case "kickstart", "lazyvim", "nvchad":
		return setupNeovimPreset(cfg, theme, nvimDir)
	case "custom":
		// "custom" means "leave the user's existing config alone".  Do not write
		// anything — the user manages their own ~/.config/nvim.
		return nil
	default:
		// Unknown preset: fall back to a minimal standalone config rather than
		// silently succeeding.  This branch should not be reachable from the UI
		// because neovimAdjust only sets values from ValidNeovimPresets.
		return writeMinimalNeovimConfig(cfg, theme, nvimDir)
	}
}

// setupNeovimPreset clones a preset config and adds user customizations
func setupNeovimPreset(cfg NeovimConfig, theme, nvimDir string) error {
	repoURL, ok := neovimConfigRepos[cfg.ConfigPreset]
	if !ok {
		return writeMinimalNeovimConfig(cfg, theme, nvimDir)
	}

	// Check if config already exists
	initFile := filepath.Join(nvimDir, "init.lua")
	if _, err := os.Stat(initFile); err == nil {
		// Config exists, just update user preferences
		return writeNeovimUserPrefs(cfg, theme, nvimDir)
	}

	parentDir := filepath.Dir(nvimDir)
	if err := os.MkdirAll(parentDir, 0700); err != nil {
		return fmt.Errorf("failed to create neovim config parent: %w", err)
	}

	tempDir, err := os.MkdirTemp(parentDir, ".nvim-clone-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary neovim config directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Clone the preset into a temp directory first.  The user's existing config
	// must not be moved unless the network/git operation has fully succeeded.
	cmd := exec.Command("git", "clone", "--depth", "1", repoURL, tempDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone %s config: %w", cfg.ConfigPreset, err)
	}

	// Remove .git directory to make it user-owned
	gitDir := filepath.Join(tempDir, ".git")
	_ = os.RemoveAll(gitDir)

	var backupDir string
	if _, err := os.Stat(nvimDir); err == nil {
		backupDir = timestampedNeovimBackupDir(nvimDir)
		if err := os.Rename(nvimDir, backupDir); err != nil {
			return fmt.Errorf("failed to backup existing neovim config: %w", err)
		}
	}

	if err := os.Rename(tempDir, nvimDir); err != nil {
		if backupDir != "" {
			if rollbackErr := os.Rename(backupDir, nvimDir); rollbackErr != nil {
				return fmt.Errorf("failed to install neovim config: %w; also failed to restore backup: %v", err, rollbackErr)
			}
		}
		return fmt.Errorf("failed to install neovim config: %w", err)
	}

	// Write user preferences
	return writeNeovimUserPrefs(cfg, theme, nvimDir)
}

func timestampedNeovimBackupDir(nvimDir string) string {
	timestamp := time.Now().Format("20060102_150405.000000000")
	backupDir := nvimDir + ".backup." + timestamp
	if _, err := os.Stat(backupDir); err != nil {
		return backupDir
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s.%d", backupDir, i)
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
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
	if _, statErr := os.Stat(nvimDir); statErr != nil {
		return nil
	}

	return writeNeovimUserPrefs(cfg, theme, nvimDir)
}

// writeNeovimUserPrefs writes user preferences to a separate file
func writeNeovimUserPrefs(cfg NeovimConfig, theme, nvimDir string) error {
	prefsPath := filepath.Join(nvimDir, "lua", "custom", "options.lua")
	content := GenerateNeovimConfig(cfg, theme)
	if err := writeToolConfig(prefsPath, []byte(content)); err != nil {
		return err
	}

	// Add require to init.lua if not already present
	initPath := filepath.Join(nvimDir, "init.lua")
	initContent, err := os.ReadFile(initPath)
	if err == nil {
		requireLine := "pcall(require, \"custom.options\")"
		if !strings.Contains(string(initContent), requireLine) {
			// Append at the end
			newContent := string(initContent) + "\n\n-- User options from dotfiles\n" + requireLine + "\n"
			if err := os.WriteFile(initPath, []byte(newContent), 0600); err != nil {
				return fmt.Errorf("failed to update neovim init.lua: %w", err)
			}
		}
	}

	return nil
}

// writeMinimalNeovimConfig writes a minimal standalone neovim config
func writeMinimalNeovimConfig(cfg NeovimConfig, theme, nvimDir string) error {
	initPath := filepath.Join(nvimDir, "init.lua")
	content := GenerateNeovimConfig(cfg, theme)
	return writeToolConfig(initPath, []byte(content))
}
