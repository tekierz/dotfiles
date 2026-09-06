package tools

// ApplicationType is display-only product taxonomy. It intentionally does not
// replace Category (operational grouping) or UIGroup (installer routing).
type ApplicationType string

const (
	ApplicationTypeSystem        ApplicationType = "system"
	ApplicationTypeTerminal      ApplicationType = "terminal"
	ApplicationTypeEditor        ApplicationType = "editor"
	ApplicationTypeFileManager   ApplicationType = "file-manager"
	ApplicationTypeDeveloperTool ApplicationType = "developer-tool"
	ApplicationTypeAIAgent       ApplicationType = "ai-agent"
	ApplicationTypeService       ApplicationType = "service"
	ApplicationTypeUtility       ApplicationType = "utility"
	ApplicationTypeApp           ApplicationType = "app"
	ApplicationTypeUnknown       ApplicationType = "unknown"
)

// ApplicationTypeProvider lets a tool override the total metadata fallback
// without expanding the operational Tool interface.
type ApplicationTypeProvider interface {
	ApplicationType() ApplicationType
}

var knownApplicationTypes = map[ApplicationType]struct{}{
	ApplicationTypeSystem: {}, ApplicationTypeTerminal: {}, ApplicationTypeEditor: {},
	ApplicationTypeFileManager: {}, ApplicationTypeDeveloperTool: {}, ApplicationTypeAIAgent: {},
	ApplicationTypeService: {}, ApplicationTypeUtility: {}, ApplicationTypeApp: {}, ApplicationTypeUnknown: {},
}

func normalizeApplicationType(value ApplicationType) ApplicationType {
	if _, known := knownApplicationTypes[value]; known {
		return value
	}
	return ApplicationTypeUnknown
}

// ApplicationTypeOf resolves display metadata without performing installation,
// package-manager, platform, service, authentication, or filesystem probes.
func ApplicationTypeOf(tool Tool) ApplicationType {
	if tool == nil {
		return ApplicationTypeUnknown
	}
	if provider, ok := tool.(ApplicationTypeProvider); ok {
		return normalizeApplicationType(provider.ApplicationType())
	}

	// Product identity takes precedence over the legacy operational category.
	// Several newer integrations intentionally retain CategoryUtility/CategoryApp
	// for existing install behavior while presenting a clearer product type.
	switch tool.ID() {
	case "claude-code", "codex", "cursor-agent", "hermes", "pi", "opencode", "t3-code":
		return ApplicationTypeAIAgent
	case "ghostty", "tmux":
		return ApplicationTypeTerminal
	case "neovim", "cursor":
		return ApplicationTypeEditor
	case "yazi":
		return ApplicationTypeFileManager
	case "tailscale", "sunshine":
		return ApplicationTypeService
	case "moonlight":
		return ApplicationTypeApp
	case "zsh", "btop":
		return ApplicationTypeSystem
	}

	switch tool.Category() {
	case CategoryShell:
		return ApplicationTypeSystem
	case CategoryTerminal:
		return ApplicationTypeTerminal
	case CategoryEditor:
		return ApplicationTypeEditor
	case CategoryFile:
		return ApplicationTypeFileManager
	case CategoryGit, CategoryContainer:
		return ApplicationTypeDeveloperTool
	case CategoryUtility:
		return ApplicationTypeUtility
	case CategoryApp:
		return ApplicationTypeApp
	default:
		return ApplicationTypeUnknown
	}
}

// ApplicationTypeOrder is the canonical stable selector order. Unknown and
// future values share the final bucket.
func ApplicationTypeOrder(value ApplicationType) int {
	switch normalizeApplicationType(value) {
	case ApplicationTypeSystem:
		return 0
	case ApplicationTypeTerminal:
		return 1
	case ApplicationTypeEditor:
		return 2
	case ApplicationTypeFileManager:
		return 3
	case ApplicationTypeDeveloperTool:
		return 4
	case ApplicationTypeAIAgent:
		return 5
	case ApplicationTypeService:
		return 6
	case ApplicationTypeUtility:
		return 7
	case ApplicationTypeApp:
		return 8
	case ApplicationTypeUnknown:
		return 9
	default:
		return 9
	}
}

// ApplicationTypeToken is a bounded ASCII label; color and glyph support are
// optional enhancements and never carry the taxonomy meaning.
func ApplicationTypeToken(value ApplicationType) string {
	switch normalizeApplicationType(value) {
	case ApplicationTypeSystem:
		return "SYS"
	case ApplicationTypeTerminal:
		return "TERM"
	case ApplicationTypeEditor:
		return "EDIT"
	case ApplicationTypeFileManager:
		return "FILE"
	case ApplicationTypeDeveloperTool:
		return "DEV"
	case ApplicationTypeAIAgent:
		return "AI"
	case ApplicationTypeService:
		return "SVC"
	case ApplicationTypeUtility:
		return "UTIL"
	case ApplicationTypeApp:
		return "APP"
	case ApplicationTypeUnknown:
		return "OTHER"
	default:
		return "OTHER"
	}
}
