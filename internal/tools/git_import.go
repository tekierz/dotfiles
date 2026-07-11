package tools

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	GitFieldDeltaSideBySide  = "delta_side_by_side"
	GitFieldDefaultBranch    = "default_branch"
	GitFieldPullRebase       = "pull_rebase"
	GitFieldSignCommits      = "sign_commits"
	GitFieldCredentialHelper = "credential_helper"
	GitFieldAutoSetupRemote  = "auto_setup_remote"
	GitFieldMergeTool        = "merge_tool"
	GitFieldDiffTool         = "diff_tool"
	GitFieldAliasSt          = "alias_st"
	GitFieldAliasCo          = "alias_co"
	GitFieldAliasBr          = "alias_br"
	GitFieldAliasCi          = "alias_ci"
	GitFieldAliasLg          = "alias_lg"
)

// GitConfigImport is a non-mutating projection of recognized effective Git
// values. Presence in Fields distinguishes an imported false/zero value from a
// field that was absent or not representable. Unknown sections and keys are
// intentionally not copied into GitConfig; the managed-include writer preserves
// their original bytes instead.
type GitConfigImport struct {
	Config         GitConfig
	Fields         map[string]ConfigFieldProvenance
	Sources        []ConfigImportSource
	ManagedInclude bool
	Warnings       []string
}

type gitConfigAssignment struct {
	key        string
	value      string
	provenance ConfigFieldProvenance
}

// ImportGitConfig observes ~/.gitconfig and the dotfiles managed include using
// descriptor-anchored no-follow reads. It does not run Git, follow arbitrary
// include paths, create directories, or write migration state.
func ImportGitConfig() (GitConfigImport, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return GitConfigImport{}, fmt.Errorf("failed to get home directory: %w", err)
	}
	rootPath := filepath.Join(home, ".gitconfig")
	managedPath := filepath.Join(home, filepath.FromSlash(gitManagedConfigRel))
	if count := strings.TrimSpace(os.Getenv("GIT_CONFIG_COUNT")); count != "" && count != "0" {
		return GitConfigImport{}, fmt.Errorf("refusing to infer Git values while GIT_CONFIG_COUNT command overrides are active")
	}
	if globalOverride := os.Getenv("GIT_CONFIG_GLOBAL"); globalOverride != "" {
		if !filepath.IsAbs(globalOverride) {
			return GitConfigImport{}, fmt.Errorf("GIT_CONFIG_GLOBAL must be absolute for safe import: %q", globalOverride)
		}
		content, exists, err := readNativeConfig(filepath.Clean(globalOverride))
		if err != nil {
			return GitConfigImport{}, err
		}
		return parseStandaloneGitImport(filepath.Clean(globalOverride), content, exists)
	}
	xdgPath := gitXDGConfigPath(home)
	xdg, xdgExists, err := readNativeConfig(xdgPath)
	if err != nil {
		return GitConfigImport{}, err
	}
	root, rootExists, err := readNativeConfig(rootPath)
	if err != nil {
		return GitConfigImport{}, err
	}
	managed, managedExists, err := readNativeConfig(managedPath)
	if err != nil {
		return GitConfigImport{}, err
	}
	return parseGitConfigImportWithPrefix(
		[]gitImportSource{{path: xdgPath, content: xdg, exists: xdgExists}},
		rootPath, root, rootExists, managedPath, managed, managedExists,
	)
}

type gitImportSource struct {
	path    string
	content []byte
	exists  bool
}

func gitXDGConfigPath(home string) string {
	root := filepath.Join(home, ".config")
	if xdg := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) {
		root = filepath.Clean(xdg)
	}
	return filepath.Join(root, "git", "config")
}

func parseStandaloneGitImport(path string, content []byte, exists bool) (GitConfigImport, error) {
	result := GitConfigImport{Fields: make(map[string]ConfigFieldProvenance), Sources: []ConfigImportSource{{Path: path, Exists: exists, Active: exists}}}
	if !exists {
		return result, nil
	}
	assignments, warnings := parseGitAssignments(content, path, ConfigValueNative, 1)
	if len(warnings) != 0 {
		return GitConfigImport{}, fmt.Errorf("refusing ambiguous Git import from %s: %s", path, strings.Join(warnings, "; "))
	}
	if hasNativeGitInclude(assignments) {
		return GitConfigImport{}, fmt.Errorf("refusing to infer effective Git values from %s because it contains include sources", path)
	}
	applyGitImport(&result, assignments)
	return result, nil
}

func parseGitConfigImport(rootPath string, root []byte, rootExists bool, managedPath string, managed []byte, managedExists bool) (GitConfigImport, error) {
	return parseGitConfigImportWithPrefix(nil, rootPath, root, rootExists, managedPath, managed, managedExists)
}

func parseGitConfigImportWithPrefix(prefix []gitImportSource, rootPath string, root []byte, rootExists bool, managedPath string, managed []byte, managedExists bool) (GitConfigImport, error) {
	result := GitConfigImport{
		Fields: make(map[string]ConfigFieldProvenance),
	}
	var assignments []gitConfigAssignment
	for _, source := range prefix {
		result.Sources = append(result.Sources, ConfigImportSource{Path: source.path, Exists: source.exists, Active: source.exists})
		if !source.exists {
			continue
		}
		parsed, warnings := parseGitAssignments(source.content, source.path, ConfigValueNative, 1)
		if len(warnings) != 0 {
			return GitConfigImport{}, fmt.Errorf("refusing ambiguous Git import from %s: %s", source.path, strings.Join(warnings, "; "))
		}
		if hasNativeGitInclude(parsed) {
			return GitConfigImport{}, fmt.Errorf("refusing to infer effective Git values from %s because it contains include sources", source.path)
		}
		assignments = append(assignments, parsed...)
	}
	result.Sources = append(result.Sources,
		ConfigImportSource{Path: rootPath, Exists: rootExists, Active: rootExists},
		ConfigImportSource{Path: managedPath, Exists: managedExists, Managed: managedExists && hasGeneratedConfigHeader(managed)},
	)
	managedSourceIndex := len(result.Sources) - 1
	if !rootExists {
		applyGitImport(&result, assignments)
		return result, nil
	}

	starts := exactManagedMarkerLines(string(root), gitManagedIncludeStart)
	ends := exactManagedMarkerLines(string(root), gitManagedIncludeEnd)
	switch {
	case len(starts) == 0 && len(ends) == 0:
		parsed, warnings := parseGitAssignments(root, rootPath, ConfigValueNative, 1)
		if len(warnings) != 0 {
			return GitConfigImport{}, fmt.Errorf("refusing ambiguous Git import from %s: %s", rootPath, strings.Join(warnings, "; "))
		}
		assignments = append(assignments, parsed...)
	case len(starts) == 1 && len(ends) == 1 && starts[0].start < ends[0].start:
		includeBody := root[starts[0].end:ends[0].start]
		if err := validateManagedGitInclude(includeBody, rootPath, bytes.Count(root[:starts[0].end], []byte("\n"))+1); err != nil {
			return GitConfigImport{}, err
		}
		result.ManagedInclude = true
		result.Sources[managedSourceIndex].Active = true
		if !managedExists {
			return GitConfigImport{}, fmt.Errorf("managed Git include %s is active but missing", managedPath)
		} else if !hasGeneratedConfigHeader(managed) {
			return GitConfigImport{}, fmt.Errorf("%w: active managed Git include %s", ErrUnmanagedConfig, managedPath)
		}

		before, warnings := parseGitAssignments(root[:starts[0].start], rootPath, ConfigValueNative, 1)
		assignments = append(assignments, before...)
		result.Warnings = append(result.Warnings, warnings...)
		if managedExists {
			included, warnings := parseGitAssignments(managed, managedPath, ConfigValueManaged, 1)
			assignments = append(assignments, included...)
			result.Warnings = append(result.Warnings, warnings...)
		}
		afterLine := bytes.Count(root[:ends[0].end], []byte("\n")) + 1
		after, warnings := parseGitAssignments(root[ends[0].end:], rootPath, ConfigValueNative, afterLine)
		assignments = append(assignments, after...)
		result.Warnings = append(result.Warnings, warnings...)
		if len(result.Warnings) != 0 {
			return GitConfigImport{}, fmt.Errorf("refusing ambiguous Git import: %s", strings.Join(result.Warnings, "; "))
		}
	default:
		return GitConfigImport{}, fmt.Errorf("refusing to import .gitconfig with ambiguous or incomplete dotfiles managed includes (%d start, %d end)", len(starts), len(ends))
	}

	if hasNativeGitInclude(assignments) {
		return GitConfigImport{}, fmt.Errorf("refusing to infer effective Git values because native include sources are present")
	}
	applyGitImport(&result, assignments)
	return result, nil
}

func hasNativeGitInclude(assignments []gitConfigAssignment) bool {
	for _, assignment := range assignments {
		if assignment.provenance.Scope == ConfigValueNative && assignment.key == "include.path" {
			return true
		}
	}
	return false
}

func validateManagedGitInclude(content []byte, path string, startLine int) error {
	assignments, warnings := parseGitAssignments(content, path, ConfigValueManaged, startLine)
	if len(warnings) != 0 || len(assignments) != 1 || assignments[0].key != "include.path" || assignments[0].value != gitManagedIncludePath {
		return fmt.Errorf("refusing to import modified dotfiles Git include block in %s", path)
	}
	return nil
}

func parseGitAssignments(content []byte, path string, scope ConfigValueScope, startLine int) ([]gitConfigAssignment, []string) {
	lines := strings.Split(string(content), "\n")
	section := ""
	assignments := make([]gitConfigAssignment, 0)
	var warnings []string
	skippingContinuation := false
	for index, raw := range lines {
		lineNumber := startLine + index
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if skippingContinuation {
			skippingContinuation = hasUnescapedGitContinuation(line)
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				section = ""
				warnings = append(warnings, fmt.Sprintf("%s:%d: ignored malformed Git section", path, lineNumber))
				continue
			}
			name := strings.TrimSpace(line[1 : len(line)-1])
			lowerName := strings.ToLower(name)
			// Subsection-specific values do not map to the global dashboard fields.
			if name == "" || strings.ContainsAny(name, " \t\"") {
				if strings.HasPrefix(lowerName, "includeif ") {
					warnings = append(warnings, fmt.Sprintf("%s:%d: conditional Git include source was not followed", path, lineNumber))
				}
				section = ""
				continue
			}
			section = lowerName
			continue
		}
		if section == "" {
			continue
		}
		if hasUnescapedGitContinuation(line) {
			warnings = append(warnings, fmt.Sprintf("%s:%d: ignored continued Git value", path, lineNumber))
			skippingContinuation = true
			continue
		}
		name, rawValue, ok := splitGitAssignment(line)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s:%d: ignored malformed Git assignment", path, lineNumber))
			continue
		}
		value, ok := parseGitValue(rawValue)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s:%d: ignored malformed quoted Git value", path, lineNumber))
			continue
		}
		key := section + "." + strings.ToLower(name)
		assignments = append(assignments, gitConfigAssignment{
			key:   key,
			value: value,
			provenance: ConfigFieldProvenance{
				Path: path, Line: lineNumber, Key: key, Scope: scope,
			},
		})
	}
	return assignments, warnings
}

func splitGitAssignment(line string) (name, value string, ok bool) {
	if equal := strings.IndexByte(line, '='); equal >= 0 {
		name = strings.TrimSpace(line[:equal])
		value = strings.TrimSpace(line[equal+1:])
	} else {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return "", "", false
		}
		name = fields[0]
		if len(fields) == 1 {
			value = "true"
		} else {
			value = strings.TrimSpace(line[len(name):])
		}
	}
	if name == "" || strings.ContainsAny(name, " \t") {
		return "", "", false
	}
	return name, value, true
}

func parseGitValue(value string) (string, bool) {
	value = stripGitInlineComment(strings.TrimSpace(value))
	if value == "" {
		return "", true
	}
	if value[0] != '"' {
		return strings.TrimSpace(value), true
	}
	if len(value) < 2 || value[len(value)-1] != '"' {
		return "", false
	}
	var result strings.Builder
	body := value[1 : len(value)-1]
	for index := 0; index < len(body); index++ {
		current := body[index]
		if current == '"' {
			return "", false
		}
		if current != '\\' {
			result.WriteByte(current)
			continue
		}
		index++
		if index >= len(body) {
			return "", false
		}
		switch body[index] {
		case 'n':
			result.WriteByte('\n')
		case 't':
			result.WriteByte('\t')
		case 'b':
			result.WriteByte('\b')
		case '\\', '"':
			result.WriteByte(body[index])
		default:
			return "", false
		}
	}
	return result.String(), true
}

func stripGitInlineComment(value string) string {
	quoted := false
	escaped := false
	for i, r := range value {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && quoted {
			escaped = true
			continue
		}
		if r == '"' {
			quoted = !quoted
			continue
		}
		if !quoted && (r == '#' || r == ';') && (i == 0 || value[i-1] == ' ' || value[i-1] == '\t') {
			return strings.TrimSpace(value[:i])
		}
	}
	return strings.TrimSpace(value)
}

func hasUnescapedGitContinuation(line string) bool {
	backslashes := 0
	for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func applyGitImport(result *GitConfigImport, assignments []gitConfigAssignment) {
	effective := make(map[string]gitConfigAssignment)
	for _, assignment := range assignments {
		effective[assignment.key] = assignment
	}

	if assignment, ok := effective["init.defaultbranch"]; ok && assignment.value != "" && !strings.ContainsAny(assignment.value, "\r\n\x00") {
		result.Config.DefaultBranch = assignment.value
		result.Fields[GitFieldDefaultBranch] = assignment.provenance
	}
	if assignment, ok := effective["pull.rebase"]; ok {
		if value, valid := parseConfigBool(assignment.value); valid {
			result.Config.PullRebase = value
			result.Fields[GitFieldPullRebase] = assignment.provenance
		}
	}
	if assignment, ok := effective["push.autosetupremote"]; ok {
		if value, valid := parseConfigBool(assignment.value); valid {
			result.Config.AutoSetupRemote = value
			result.Fields[GitFieldAutoSetupRemote] = assignment.provenance
		}
	}
	if assignment, ok := effective["credential.helper"]; ok && !strings.ContainsAny(assignment.value, "\r\n\x00") {
		result.Config.CredentialHelper = assignment.value
		if assignment.value == "" {
			result.Config.CredentialHelper = "none"
		}
		result.Fields[GitFieldCredentialHelper] = assignment.provenance
	}
	if assignment, ok := effective["commit.gpgsign"]; ok {
		if value, valid := parseConfigBool(assignment.value); valid {
			result.Config.SignCommits = value
			result.Fields[GitFieldSignCommits] = assignment.provenance
		}
	}
	if assignment, ok := effective["merge.tool"]; ok && assignment.value != "" && !strings.ContainsAny(assignment.value, "\r\n\x00") {
		result.Config.MergeTool = assignment.value
		result.Fields[GitFieldMergeTool] = assignment.provenance
	}
	if assignment, ok := effective["delta.side-by-side"]; ok {
		if value, valid := parseConfigBool(assignment.value); valid {
			result.Config.DeltaSideBySide = value
			result.Fields[GitFieldDeltaSideBySide] = assignment.provenance
		}
	}

	if assignment, ok := effective["diff.external"]; ok && (assignment.value == "difft" || assignment.value == "difftastic") {
		result.Config.DiffTool = "difftastic"
		result.Fields[GitFieldDiffTool] = assignment.provenance
	} else if assignment, ok := effective["diff.tool"]; ok && (assignment.value == "vimdiff" || assignment.value == "nvimdiff") {
		result.Config.DiffTool = assignment.value
		result.Fields[GitFieldDiffTool] = assignment.provenance
	} else if assignment, ok := effective["core.pager"]; ok && strings.Contains(strings.ToLower(assignment.value), "delta") {
		result.Config.DiffTool = "delta"
		result.Fields[GitFieldDiffTool] = assignment.provenance
	}

	aliases := []struct {
		name      string
		field     string
		canonical string
	}{
		{"st", GitFieldAliasSt, "status"},
		{"co", GitFieldAliasCo, "checkout"},
		{"br", GitFieldAliasBr, "branch"},
		{"ci", GitFieldAliasCi, "commit"},
		{"lg", GitFieldAliasLg, "log --oneline --graph --decorate"},
	}
	for _, alias := range aliases {
		if assignment, ok := effective["alias."+alias.name]; ok && assignment.value == alias.canonical {
			result.Config.Aliases = append(result.Config.Aliases, alias.name)
			result.Fields[alias.field] = assignment.provenance
		}
	}
}

func parseConfigBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "on", "1":
		return true, true
	case "false", "no", "off", "0", "":
		return false, true
	default:
		return false, false
	}
}
