package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// simpleToolSpec is a declarative description of a "simple" tool: one whose
// behavior is entirely covered by BaseTool (no custom IsInstalled, no config
// writer, no special install logic). Collapsing these into a data table avoids
// a dedicated file + boilerplate constructor per tool.
//
// configPath, when non-empty, is joined under the user's home directory to
// produce the tool's single ConfigPaths() entry.
type simpleToolSpec struct {
	id             string
	name           string
	description    string
	icon           string
	category       Category
	packages       map[pkg.Platform][]string
	configPath     []string // path components under $HOME; empty = no config file
	heavy          bool
	uiGroup        UIGroup
	configScreen   int
	defaultEnabled bool
	platformFilter pkg.Platform
}

// simpleTools is the registry of every pure-metadata tool. Each entry replaces
// what used to be its own *Tool type and NewXTool() constructor.
var simpleTools = []simpleToolSpec{
	{
		id:          "bat",
		name:        "bat",
		description: "A cat clone with syntax highlighting",
		icon:        "󰭟",
		category:    CategoryUtility,
		packages: map[pkg.Platform][]string{
			pkg.PlatformMacOS:  {"bat"},
			pkg.PlatformArch:   {"bat"},
			pkg.PlatformDebian: {"bat"},
		},
		configPath:     []string{".config", "bat", "config"},
		uiGroup:        UIGroupCLIUtilities,
		defaultEnabled: true,
	},
	{
		id:          "eza",
		name:        "eza",
		description: "Modern replacement for ls",
		icon:        "󰙅",
		category:    CategoryUtility,
		packages: map[pkg.Platform][]string{
			pkg.PlatformMacOS:  {"eza"},
			pkg.PlatformArch:   {"eza"},
			pkg.PlatformDebian: {"eza"},
		},
		uiGroup:        UIGroupCLIUtilities,
		defaultEnabled: true,
	},
	{
		id:          "zoxide",
		name:        "zoxide",
		description: "Smarter cd command with learning",
		icon:        "󰄛",
		category:    CategoryUtility,
		packages: map[pkg.Platform][]string{
			pkg.PlatformMacOS:  {"zoxide"},
			pkg.PlatformArch:   {"zoxide"},
			pkg.PlatformDebian: {"zoxide"},
		},
		uiGroup:        UIGroupCLIUtilities,
		defaultEnabled: true,
	},
	{
		id:          "ripgrep",
		name:        "ripgrep",
		description: "Fast recursive grep alternative",
		icon:        "󰑐",
		category:    CategoryUtility,
		packages: map[pkg.Platform][]string{
			pkg.PlatformMacOS:  {"ripgrep"},
			pkg.PlatformArch:   {"ripgrep"},
			pkg.PlatformDebian: {"ripgrep"},
		},
		configPath:     []string{".config", "ripgrep", "config"},
		uiGroup:        UIGroupCLIUtilities,
		defaultEnabled: true,
	},
	{
		id:          "fd",
		name:        "fd",
		description: "Simple, fast find alternative",
		icon:        "󰈞",
		category:    CategoryUtility,
		packages: map[pkg.Platform][]string{
			pkg.PlatformMacOS:  {"fd"},
			pkg.PlatformArch:   {"fd"},
			pkg.PlatformDebian: {"fd-find"},
		},
		uiGroup:        UIGroupCLIUtilities,
		defaultEnabled: true,
	},
	{
		id:          "fswatch",
		name:        "fswatch",
		description: "Cross-platform file change monitor",
		icon:        "󱄄",
		category:    CategoryUtility,
		packages: map[pkg.Platform][]string{
			pkg.PlatformMacOS:  {"fswatch"},
			pkg.PlatformArch:   {"fswatch"},
			pkg.PlatformDebian: {"fswatch"},
		},
		uiGroup:        UIGroupCLIUtilities,
		defaultEnabled: false,
	},
	{
		id:          "delta",
		name:        "Delta",
		description: "Syntax-highlighting pager for git diffs",
		icon:        "󰘧",
		category:    CategoryGit,
		packages: map[pkg.Platform][]string{
			pkg.PlatformMacOS:  {"git-delta"},
			pkg.PlatformArch:   {"git-delta"},
			pkg.PlatformDebian: {"git-delta"},
		},
		uiGroup:        UIGroupCLIUtilities,
		defaultEnabled: true,
	},
	{
		id:          "lazydocker",
		name:        "LazyDocker",
		description: "Simple terminal UI for Docker",
		icon:        "",
		category:    CategoryContainer,
		packages: map[pkg.Platform][]string{
			pkg.PlatformMacOS: {"lazydocker"},
			pkg.PlatformArch:  {"lazydocker"},
			// lazydocker is not in stock Debian/Ubuntu repos; install via Go or
			// manually — omitting the Debian entry prevents a guaranteed-failing apt.
		},
		configPath: []string{".config", "lazydocker", "config.yml"},
		heavy:      true, // Skip on low-memory systems (Pi Zero 2)
		uiGroup:    UIGroupCLITools,
		// configScreen: 0 — lazydocker has NO config generator (no
		// tools.WriteLazyDockerConfig), so it is installable but not "configurable".
		// `dotfiles config lazydocker` is intentionally not offered; its
		// install-time selection lives on the CLI Tools group screen.
		configScreen:   0,
		defaultEnabled: true,
	},
}

// newSimpleTool builds a BaseTool from a declarative spec. The returned
// *BaseTool already satisfies the Tool interface, so simple tools no longer
// need their own wrapper type.
func newSimpleTool(spec simpleToolSpec) *BaseTool {
	var configPaths []string
	if len(spec.configPath) > 0 {
		home, _ := os.UserHomeDir()
		configPaths = []string{filepath.Join(append([]string{home}, spec.configPath...)...)}
	}
	return &BaseTool{
		id:             spec.id,
		name:           spec.name,
		description:    spec.description,
		icon:           spec.icon,
		category:       spec.category,
		packages:       spec.packages,
		configPaths:    configPaths,
		heavyTool:      spec.heavy,
		uiGroup:        spec.uiGroup,
		configScreen:   spec.configScreen,
		defaultEnabled: spec.defaultEnabled,
		platformFilter: spec.platformFilter,
	}
}

// registerSimpleTools registers every tool defined in the simpleTools table.
func (r *Registry) registerSimpleTools() {
	for _, spec := range simpleTools {
		r.Register(newSimpleTool(spec))
	}
}
