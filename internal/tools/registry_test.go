package tools

import (
	"reflect"
	"runtime"
	"sort"
	"testing"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// toolSpec captures the full observable metadata of a registered tool. It is
// used by TestRegistryToolMetadataSnapshot as a characterization (golden) test:
// the expected table below was captured from the current registry, so any drift
// introduced by refactoring (consolidating tool files into a data table,
// removing the dead config schema, etc.) will fail loudly here.
type toolSpec struct {
	name           string
	description    string
	category       Category
	icon           string
	uiGroup        UIGroup
	configScreen   int
	isHeavy        bool
	defaultEnabled bool
	platformFilter pkg.Platform
	hasConfig      bool
	configPaths    int
	packages       map[pkg.Platform][]string
}

// expectedTools is the golden snapshot of every tool returned by
// NewRegistry().All(), keyed by ID(). Captured from the pre-refactor registry.
var expectedTools = map[string]toolSpec{
	"appcleaner": {
		name: "AppCleaner", description: "Thoroughly uninstall macOS apps", category: CategoryApp, icon: "\U000f00e2",
		uiGroup: UIGroupMacApps, configScreen: 0, isHeavy: false, defaultEnabled: true, platformFilter: pkg.PlatformMacOS,
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformMacOS: {"appcleaner"}},
	},
	"bat": {
		name: "bat", description: "A cat clone with syntax highlighting", category: CategoryUtility, icon: "\U000f0b5f",
		uiGroup: UIGroupCLIUtilities, configScreen: 0, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: 1,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"bat"}, pkg.PlatformDebian: {"bat"}, pkg.PlatformMacOS: {"bat"}},
	},
	"btop": {
		name: "btop", description: "Resource monitor with TUI", category: CategoryUtility, icon: "\U000f0128",
		uiGroup: UIGroupCLITools, configScreen: 30, isHeavy: true, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: 1,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"btop"}, pkg.PlatformDebian: {"btop"}, pkg.PlatformMacOS: {"btop"}},
	},
	"claude-code": {
		name: "Claude Code", description: "AI-powered coding assistant", category: CategoryUtility, icon: "\U000f06a9",
		uiGroup: UIGroupCLITools, configScreen: 32, isHeavy: false, defaultEnabled: false, platformFilter: "",
		hasConfig: true, configPaths: 1,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"nodejs", "npm"}, pkg.PlatformDebian: {"nodejs", "npm"}, pkg.PlatformMacOS: {"node"}},
	},
	"codex": {
		name: "Codex", description: "OpenAI coding agent for the terminal", category: CategoryUtility, icon: "\U000f06a9",
		uiGroup: UIGroupCLITools, configScreen: 0, isHeavy: false, defaultEnabled: false, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformMacOS: {"node"}},
	},
	"cursor": {
		name: "Cursor", description: "AI-first code editor", category: CategoryApp, icon: "\U000f09a8",
		uiGroup: UIGroupGUIApps, configScreen: 0, isHeavy: false, defaultEnabled: false, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"cursor-bin"}, pkg.PlatformMacOS: {"cursor"}},
	},
	"delta": {
		name: "Delta", description: "Syntax-highlighting pager for git diffs", category: CategoryGit, icon: "\U000f0627",
		uiGroup: UIGroupCLIUtilities, configScreen: 0, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"git-delta"}, pkg.PlatformDebian: {"git-delta"}, pkg.PlatformMacOS: {"git-delta"}},
	},
	"eza": {
		name: "eza", description: "Modern replacement for ls", category: CategoryUtility, icon: "\U000f0645",
		uiGroup: UIGroupCLIUtilities, configScreen: 0, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"eza"}, pkg.PlatformDebian: {"eza"}, pkg.PlatformMacOS: {"eza"}},
	},
	"fd": {
		name: "fd", description: "Simple, fast find alternative", category: CategoryUtility, icon: "\U000f021e",
		uiGroup: UIGroupCLIUtilities, configScreen: 0, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"fd"}, pkg.PlatformDebian: {"fd-find"}, pkg.PlatformMacOS: {"fd"}},
	},
	"fswatch": {
		name: "fswatch", description: "Cross-platform file change monitor", category: CategoryUtility, icon: "\U000f1104",
		uiGroup: UIGroupCLIUtilities, configScreen: 0, isHeavy: false, defaultEnabled: false, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"fswatch"}, pkg.PlatformDebian: {"fswatch"}, pkg.PlatformMacOS: {"fswatch"}},
	},
	"fzf": {
		name: "fzf", description: "Command-line fuzzy finder", category: CategoryUtility, icon: "\U000f0349",
		uiGroup: UIGroupNone, configScreen: 15, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: 1,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"fzf"}, pkg.PlatformDebian: {"fzf"}, pkg.PlatformMacOS: {"fzf"}},
	},
	"ghostty": {
		name: "Ghostty", description: "GPU-accelerated terminal emulator", category: CategoryTerminal, icon: "\U000f018d",
		uiGroup: UIGroupNone, configScreen: 9, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: ghosttyExpectedConfigPathCount(),
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"ghostty"}, pkg.PlatformMacOS: {"ghostty"}},
	},
	"git": {
		name: "Git", description: "Distributed version control system", category: CategoryGit, icon: "",
		uiGroup: UIGroupNone, configScreen: 13, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: 2,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"git"}, pkg.PlatformDebian: {"git"}, pkg.PlatformMacOS: {"git"}},
	},
	"glow": {
		name: "Glow", description: "Render markdown on the CLI", category: CategoryUtility, icon: "\U000f0219",
		uiGroup: UIGroupCLITools, configScreen: 31, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: 1,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"glow"}, pkg.PlatformMacOS: {"glow"}},
	},
	"iina": {
		name: "IINA", description: "Modern media player for macOS", category: CategoryApp, icon: "\U000f057c",
		uiGroup: UIGroupMacApps, configScreen: 0, isHeavy: false, defaultEnabled: false, platformFilter: pkg.PlatformMacOS,
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformMacOS: {"iina"}},
	},
	"lazydocker": {
		name: "LazyDocker", description: "Simple terminal UI for Docker", category: CategoryContainer, icon: "",
		uiGroup: UIGroupCLITools, configScreen: 0, isHeavy: true, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: 1,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"lazydocker"}, pkg.PlatformMacOS: {"lazydocker"}},
	},
	"lazygit": {
		name: "LazyGit", description: "Simple terminal UI for Git commands", category: CategoryGit, icon: "\U000f02a2",
		uiGroup: UIGroupCLITools, configScreen: 28, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: 1,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"lazygit"}, pkg.PlatformMacOS: {"lazygit"}},
	},
	"lm-studio": {
		name: "LM Studio", description: "Local LLM runner", category: CategoryApp, icon: "\U000f06a9",
		uiGroup: UIGroupGUIApps, configScreen: 0, isHeavy: false, defaultEnabled: false, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"lmstudio-bin"}, pkg.PlatformMacOS: {"lm-studio"}},
	},
	"moonlight": {
		name: "Moonlight", description: "Open-source game streaming client", category: CategoryUtility, icon: "🌙",
		uiGroup: UIGroupGUIApps, configScreen: 0, isHeavy: false, defaultEnabled: false, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"moonlight-qt"}, pkg.PlatformDebian: {"moonlight-qt"}, pkg.PlatformMacOS: {"moonlight"}},
	},
	"neovim": {
		name: "Neovim", description: "Hyperextensible Vim-based text editor", category: CategoryEditor, icon: "",
		uiGroup: UIGroupNone, configScreen: 12, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: 1,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"neovim"}, pkg.PlatformDebian: {"neovim"}, pkg.PlatformMacOS: {"neovim"}},
	},
	"obs": {
		name: "OBS Studio", description: "Streaming and recording software", category: CategoryApp, icon: "",
		uiGroup: UIGroupGUIApps, configScreen: 0, isHeavy: false, defaultEnabled: false, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"obs-studio"}, pkg.PlatformDebian: {"obs-studio"}, pkg.PlatformMacOS: {"obs"}},
	},
	"opencode": {
		name: "OpenCode", description: "Open-source coding agent for the terminal", category: CategoryUtility, icon: "\U000f06a9",
		uiGroup: UIGroupCLITools, configScreen: 0, isHeavy: false, defaultEnabled: false, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"opencode"}, pkg.PlatformMacOS: {"anomalyco/tap/opencode"}},
	},
	"pi": {
		name: "Pi", description: "Minimal, extensible coding agent", category: CategoryUtility, icon: "π",
		uiGroup: UIGroupCLITools, configScreen: 0, isHeavy: false, defaultEnabled: false, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformMacOS: {"node"}},
	},
	"raycast": {
		name: "Raycast", description: "Productivity launcher for macOS", category: CategoryApp, icon: "\U000f0238",
		uiGroup: UIGroupMacApps, configScreen: 0, isHeavy: false, defaultEnabled: true, platformFilter: pkg.PlatformMacOS,
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformMacOS: {"raycast"}},
	},
	"rectangle": {
		name: "Rectangle", description: "Window management for macOS", category: CategoryApp, icon: "\U000f0379",
		uiGroup: UIGroupMacApps, configScreen: 0, isHeavy: false, defaultEnabled: true, platformFilter: pkg.PlatformMacOS,
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformMacOS: {"rectangle"}},
	},
	"ripgrep": {
		name: "ripgrep", description: "Fast recursive grep alternative", category: CategoryUtility, icon: "\U000f0450",
		uiGroup: UIGroupCLIUtilities, configScreen: 0, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: 1,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"ripgrep"}, pkg.PlatformDebian: {"ripgrep"}, pkg.PlatformMacOS: {"ripgrep"}},
	},
	"sunshine": {
		name: "Sunshine", description: "Self-hosted game streaming server", category: CategoryUtility, icon: "☀",
		uiGroup: UIGroupGUIApps, configScreen: 0, isHeavy: false, defaultEnabled: false, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"sunshine"}, pkg.PlatformDebian: {"sunshine"}, pkg.PlatformMacOS: {"sunshine"}},
	},
	"tailscale": {
		name: "Tailscale", description: "Mesh VPN for secure networking", category: CategoryUtility, icon: "\U000f0582",
		uiGroup: UIGroupCLIUtilities, configScreen: 0, isHeavy: false, defaultEnabled: false, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"tailscale"}, pkg.PlatformDebian: {"tailscale"}, pkg.PlatformMacOS: {"tailscale"}},
	},
	"tmux": {
		name: "Tmux", description: "Terminal multiplexer", category: CategoryTerminal, icon: "",
		uiGroup: UIGroupNone, configScreen: 10, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: 1,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"tmux"}, pkg.PlatformDebian: {"tmux"}, pkg.PlatformMacOS: {"tmux"}},
	},
	"yazi": {
		name: "Yazi", description: "Blazing fast terminal file manager", category: CategoryFile, icon: "\U000f024b",
		uiGroup: UIGroupNone, configScreen: 14, isHeavy: true, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: 3,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"yazi", "ffmpegthumbnailer", "unarchiver", "jq", "poppler", "fd", "ripgrep", "fzf", "zoxide", "imagemagick"}, pkg.PlatformMacOS: {"yazi", "ffmpegthumbnailer", "unar", "jq", "poppler", "fd", "ripgrep", "fzf", "zoxide", "imagemagick"}},
	},
	"zen-browser": {
		name: "Zen Browser", description: "Privacy-focused browser based on Firefox", category: CategoryApp, icon: "\U000f059f",
		uiGroup: UIGroupGUIApps, configScreen: 0, isHeavy: false, defaultEnabled: false, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"zen-browser-bin"}, pkg.PlatformMacOS: {"zen-browser"}},
	},
	"zoxide": {
		name: "zoxide", description: "Smarter cd command with learning", category: CategoryUtility, icon: "\U000f011b",
		uiGroup: UIGroupCLIUtilities, configScreen: 0, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: false, configPaths: 0,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"zoxide"}, pkg.PlatformDebian: {"zoxide"}, pkg.PlatformMacOS: {"zoxide"}},
	},
	"zsh": {
		name: "Zsh", description: "Z shell with plugins and customization", category: CategoryShell, icon: "",
		uiGroup: UIGroupNone, configScreen: 11, isHeavy: false, defaultEnabled: true, platformFilter: "",
		hasConfig: true, configPaths: 2,
		packages: map[pkg.Platform][]string{pkg.PlatformArch: {"zsh", "zsh-autosuggestions", "zsh-syntax-highlighting", "zsh-completions", "zsh-theme-powerlevel10k"}, pkg.PlatformDebian: {"zsh", "zsh-autosuggestions", "zsh-syntax-highlighting"}, pkg.PlatformMacOS: {"zsh", "zsh-autosuggestions", "zsh-syntax-highlighting", "zsh-completions", "powerlevel10k"}},
	},
}

func ghosttyExpectedConfigPathCount() int {
	if runtime.GOOS == "darwin" {
		return 4
	}
	return 2
}

// TestRegistryToolMetadataSnapshot is a characterization test that asserts the
// complete observable metadata of every registered tool matches the golden
// snapshot captured in expectedTools. It guards the tools-package refactor
// (consolidating boilerplate tool files into a data table and removing the dead
// GenerateConfig/ApplyConfig schema) against any behavioral drift.
func TestRegistryToolMetadataSnapshot(t *testing.T) {
	r := NewRegistry()
	all := r.All()

	// 1. The set of IDs must match exactly (no tool added or dropped).
	gotIDs := make(map[string]bool, len(all))
	for _, tool := range all {
		gotIDs[tool.ID()] = true
	}
	for id := range expectedTools {
		if !gotIDs[id] {
			t.Errorf("expected tool %q is missing from the registry", id)
		}
	}
	for id := range gotIDs {
		if _, ok := expectedTools[id]; !ok {
			t.Errorf("registry has unexpected tool %q not present in the golden snapshot", id)
		}
	}

	// 2. Every tool's full metadata must match the snapshot exactly.
	for _, tool := range all {
		id := tool.ID()
		want, ok := expectedTools[id]
		if !ok {
			continue // already reported above
		}
		t.Run(id, func(t *testing.T) {
			if tool.Name() != want.name {
				t.Errorf("Name() = %q, want %q", tool.Name(), want.name)
			}
			if tool.Description() != want.description {
				t.Errorf("Description() = %q, want %q", tool.Description(), want.description)
			}
			if tool.Category() != want.category {
				t.Errorf("Category() = %q, want %q", tool.Category(), want.category)
			}
			if tool.Icon() != want.icon {
				t.Errorf("Icon() = %q, want %q", tool.Icon(), want.icon)
			}
			if tool.UIGroup() != want.uiGroup {
				t.Errorf("UIGroup() = %q, want %q", tool.UIGroup(), want.uiGroup)
			}
			if tool.ConfigScreen() != want.configScreen {
				t.Errorf("ConfigScreen() = %d, want %d", tool.ConfigScreen(), want.configScreen)
			}
			if tool.IsHeavy() != want.isHeavy {
				t.Errorf("IsHeavy() = %t, want %t", tool.IsHeavy(), want.isHeavy)
			}
			if tool.DefaultEnabled() != want.defaultEnabled {
				t.Errorf("DefaultEnabled() = %t, want %t", tool.DefaultEnabled(), want.defaultEnabled)
			}
			if tool.PlatformFilter() != want.platformFilter {
				t.Errorf("PlatformFilter() = %q, want %q", tool.PlatformFilter(), want.platformFilter)
			}
			if tool.HasConfig() != want.hasConfig {
				t.Errorf("HasConfig() = %t, want %t", tool.HasConfig(), want.hasConfig)
			}
			if got := len(tool.ConfigPaths()); got != want.configPaths {
				t.Errorf("len(ConfigPaths()) = %d, want %d", got, want.configPaths)
			}
			// Full per-platform package map must match exactly.
			gotPkgs := tool.Packages()
			if len(gotPkgs) != len(want.packages) {
				t.Errorf("Packages() has %d platforms, want %d", len(gotPkgs), len(want.packages))
			}
			for plat, wantList := range want.packages {
				gotList := gotPkgs[plat]
				if !reflect.DeepEqual(gotList, wantList) {
					t.Errorf("Packages()[%q] = %v, want %v", plat, gotList, wantList)
				}
			}
			for plat := range gotPkgs {
				if _, ok := want.packages[plat]; !ok {
					t.Errorf("Packages() has unexpected platform %q = %v", plat, gotPkgs[plat])
				}
			}
		})
	}

	// 3. Sanity: count matches.
	wantIDs := make([]string, 0, len(expectedTools))
	for id := range expectedTools {
		wantIDs = append(wantIDs, id)
	}
	sort.Strings(wantIDs)
	if len(all) != len(wantIDs) {
		t.Errorf("registry has %d tools, golden snapshot has %d", len(all), len(wantIDs))
	}
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()

	// Should have tools registered
	tools := r.All()
	if len(tools) == 0 {
		t.Error("expected tools to be registered")
	}

	// Should have at least core tools
	coreTools := []string{"zsh", "ghostty", "tmux", "neovim", "yazi", "git", "fzf"}
	for _, id := range coreTools {
		if _, ok := r.Get(id); !ok {
			t.Errorf("expected tool %q to be registered", id)
		}
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()

	// Get existing tool
	tool, ok := r.Get("zsh")
	if !ok {
		t.Fatal("expected to find zsh tool")
	}
	if tool.ID() != "zsh" {
		t.Errorf("ID() = %q, want %q", tool.ID(), "zsh")
	}
	if tool.Name() != "Zsh" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "Zsh")
	}

	// Get non-existent tool
	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("should not find nonexistent tool")
	}
}

func TestRegistry_All(t *testing.T) {
	r := NewRegistry()

	tools := r.All()
	if len(tools) < 10 {
		t.Errorf("expected at least 10 tools, got %d", len(tools))
	}

	// Verify sorted by name
	for i := 1; i < len(tools); i++ {
		if tools[i-1].Name() > tools[i].Name() {
			t.Errorf("tools not sorted: %s > %s", tools[i-1].Name(), tools[i].Name())
		}
	}
}

func TestRegistry_ByCategory(t *testing.T) {
	r := NewRegistry()

	// Test shell category
	shellTools := r.ByCategory(CategoryShell)
	if len(shellTools) == 0 {
		t.Error("expected shell tools")
	}
	for _, tool := range shellTools {
		if tool.Category() != CategoryShell {
			t.Errorf("expected shell category, got %v", tool.Category())
		}
	}

	// Test terminal category
	terminalTools := r.ByCategory(CategoryTerminal)
	if len(terminalTools) == 0 {
		t.Error("expected terminal tools")
	}

	// Test utility category
	utilityTools := r.ByCategory(CategoryUtility)
	if len(utilityTools) < 5 {
		t.Errorf("expected at least 5 utility tools, got %d", len(utilityTools))
	}
}

func TestRegistry_Register(t *testing.T) {
	r := &Registry{
		tools: make(map[string]Tool),
	}

	// Register a tool
	tool := &mockTool{id: "test", name: "Test Tool"}
	r.Register(tool)

	// Verify registered
	got, ok := r.Get("test")
	if !ok {
		t.Fatal("expected to find registered tool")
	}
	if got.Name() != "Test Tool" {
		t.Errorf("Name() = %q, want %q", got.Name(), "Test Tool")
	}
}

// mockTool is a simple Tool implementation for testing
type mockTool struct {
	id          string
	name        string
	description string
	icon        string
	category    Category
	packages    map[pkg.Platform][]string
	installed   bool
}

func (t *mockTool) ID() string                           { return t.id }
func (t *mockTool) Name() string                         { return t.name }
func (t *mockTool) Description() string                  { return t.description }
func (t *mockTool) Icon() string                         { return t.icon }
func (t *mockTool) Category() Category                   { return t.category }
func (t *mockTool) Packages() map[pkg.Platform][]string  { return t.packages }
func (t *mockTool) IsInstalled() bool                    { return t.installed }
func (t *mockTool) Install(mgr pkg.PackageManager) error { return nil }
func (t *mockTool) ConfigPaths() []string                { return nil }
func (t *mockTool) HasConfig() bool                      { return false }
func (t *mockTool) IsHeavy() bool                        { return false }
func (t *mockTool) UIGroup() UIGroup                     { return UIGroupNone }
func (t *mockTool) ConfigScreen() int                    { return 0 }
func (t *mockTool) DefaultEnabled() bool                 { return false }
func (t *mockTool) PlatformFilter() pkg.Platform         { return "" }

func TestCategoryConstants(t *testing.T) {
	// Verify all category constants are distinct
	categories := map[Category]bool{
		CategoryShell:     true,
		CategoryTerminal:  true,
		CategoryEditor:    true,
		CategoryFile:      true,
		CategoryGit:       true,
		CategoryContainer: true,
		CategoryUtility:   true,
		CategoryApp:       true,
	}

	if len(categories) != 8 {
		t.Errorf("expected 8 distinct categories, got %d", len(categories))
	}
}

func TestToolProperties(t *testing.T) {
	r := NewRegistry()

	// Test a few specific tools
	tests := []struct {
		id       string
		name     string
		category Category
	}{
		{"zsh", "Zsh", CategoryShell},
		{"ghostty", "Ghostty", CategoryTerminal},
		{"tmux", "Tmux", CategoryTerminal},
		{"neovim", "Neovim", CategoryEditor},
		{"yazi", "Yazi", CategoryFile},
		{"git", "Git", CategoryGit},
		{"fzf", "fzf", CategoryUtility},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			tool, ok := r.Get(tt.id)
			if !ok {
				t.Fatalf("expected to find %q", tt.id)
			}

			if tool.Name() != tt.name {
				t.Errorf("Name() = %q, want %q", tool.Name(), tt.name)
			}
			if tool.Category() != tt.category {
				t.Errorf("Category() = %v, want %v", tool.Category(), tt.category)
			}
			if tool.Description() == "" {
				t.Error("Description() should not be empty")
			}
			// Icon is optional for some tools
			_ = tool.Icon()
		})
	}
}

func TestToolPackages(t *testing.T) {
	r := NewRegistry()

	// Tools should have packages defined for at least one platform
	for _, tool := range r.All() {
		packages := tool.Packages()
		if len(packages) == 0 {
			// Some tools (like apps) may have no packages
			continue
		}

		hasPackages := false
		for _, pkgs := range packages {
			if len(pkgs) > 0 {
				hasPackages = true
				break
			}
		}

		// Skip app category - they may have no packages
		if tool.Category() == CategoryApp {
			continue
		}

		if !hasPackages {
			t.Errorf("tool %s should have packages for at least one platform", tool.ID())
		}
	}
}

func TestPackagesForPlatform(t *testing.T) {
	packages := map[pkg.Platform][]string{
		pkg.PlatformMacOS:  {"node"},
		pkg.PlatformArch:   {"nodejs", "npm"},
		pkg.PlatformDebian: {"nodejs", "npm"},
	}

	tests := []struct {
		name     string
		packages map[pkg.Platform][]string
		platform pkg.Platform
		want     []string
	}{
		{
			name:     "exact platform match",
			packages: packages,
			platform: pkg.PlatformMacOS,
			want:     []string{"node"},
		},
		{
			name:     "raspberry pi falls back to debian packages",
			packages: packages,
			platform: pkg.PlatformPi,
			want:     []string{"nodejs", "npm"},
		},
		{
			name:     "pi prefers its own entry over debian when present",
			packages: map[pkg.Platform][]string{pkg.PlatformPi: {"pi-pkg"}, pkg.PlatformDebian: {"deb-pkg"}},
			platform: pkg.PlatformPi,
			want:     []string{"pi-pkg"},
		},
		{
			name:     "all key used as final fallback",
			packages: map[pkg.Platform][]string{"all": {"universal"}},
			platform: pkg.PlatformMacOS,
			want:     []string{"universal"},
		},
		{
			name:     "no packages for platform returns empty",
			packages: map[pkg.Platform][]string{pkg.PlatformMacOS: {"node"}},
			platform: pkg.PlatformArch,
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PackagesForPlatform(tt.packages, tt.platform)
			if len(got) != len(tt.want) {
				t.Fatalf("PackagesForPlatform() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("PackagesForPlatform()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBaseToolPlatformExplicitInstallAndObservation(t *testing.T) {
	mgr := pkg.NewMockPackageManager()
	zsh := NewZshTool()

	if err := zsh.InstallForPlatform(mgr, pkg.PlatformPi); err != nil {
		t.Fatalf("InstallForPlatform(Pi): %v", err)
	}
	want := PackagesForPlatform(zsh.Packages(), pkg.PlatformPi)
	if len(mgr.InstallCalls) != 1 || !reflect.DeepEqual(mgr.InstallCalls[0], want) {
		t.Fatalf("Pi execution installed %v, want Debian fallback %v", mgr.InstallCalls, want)
	}
	if !zsh.IsInstalledForPlatform(mgr, pkg.PlatformPi) {
		t.Fatal("explicit Pi observation did not see its Debian fallback packages")
	}
	if zsh.IsInstalledForPlatform(mgr, pkg.PlatformMacOS) {
		t.Fatal("explicit macOS observation leaked the Pi/Debian package state")
	}
}

// TestRegistryToolsResolveOnPi verifies that every registered tool with packages
// for Debian also resolves a non-empty package list on a Raspberry Pi, so the
// installer never silently skips every tool on a Pi (regression for tools-1).
func TestRegistryToolsResolveOnPi(t *testing.T) {
	r := NewRegistry()
	for _, tool := range r.All() {
		debPkgs := tool.Packages()[pkg.PlatformDebian]
		if len(debPkgs) == 0 {
			continue
		}
		piPkgs := PackagesForPlatform(tool.Packages(), pkg.PlatformPi)
		if len(piPkgs) == 0 {
			t.Errorf("tool %s has Debian packages but resolves no packages on Raspberry Pi", tool.ID())
		}
	}
}

func TestToolConfigPaths(t *testing.T) {
	r := NewRegistry()

	// Tools with HasConfig() should have ConfigPaths()
	for _, tool := range r.All() {
		if tool.HasConfig() {
			paths := tool.ConfigPaths()
			if len(paths) == 0 {
				t.Errorf("tool %s HasConfig() but no ConfigPaths()", tool.ID())
			}
		}
	}
}
