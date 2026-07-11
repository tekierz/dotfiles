package tools

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	TmuxFieldPrefix           = "prefix"
	TmuxFieldSplitBinds       = "split_binds"
	TmuxFieldStatusPosition   = "status_position"
	TmuxFieldMouse            = "mouse"
	TmuxFieldBaseIndex        = "base_index"
	TmuxFieldPaneBorder       = "pane_border_lines"
	TmuxFieldHistoryLimit     = "history_limit"
	TmuxFieldEscapeTime       = "escape_time"
	TmuxFieldAggressiveResize = "aggressive_resize"
	TmuxFieldTPMEnabled       = "tpm_enabled"
	TmuxFieldPluginSensible   = "plugin_sensible"
	TmuxFieldPluginResurrect  = "plugin_resurrect"
	TmuxFieldPluginContinuum  = "plugin_continuum"
	TmuxFieldPluginYank       = "plugin_yank"
	TmuxFieldContinuumSaveMin = "continuum_save_minutes"
	TmuxFieldContinuumRestore = "continuum_restore"
)

// TmuxConfigImport is a conservative static observation of the active user
// tmux config. The parser never starts tmux or evaluates config commands.
type TmuxConfigImport struct {
	Config   TmuxConfig
	Fields   map[string]ConfigFieldProvenance
	Sources  []ConfigImportSource
	Warnings []string
}

// ImportTmuxConfig observes the same active source selected by
// TmuxConfigMutationPath and reports inactive candidates for provenance UI.
func ImportTmuxConfig() (TmuxConfigImport, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return TmuxConfigImport{}, fmt.Errorf("determine HOME for tmux import: %w", err)
	}
	candidates, err := tmuxConfigCandidates(home)
	if err != nil {
		return TmuxConfigImport{}, err
	}
	contents := make([][]byte, len(candidates))
	exists := make([]bool, len(candidates))
	active := -1
	result := TmuxConfigImport{Fields: make(map[string]ConfigFieldProvenance)}
	for index, candidate := range candidates {
		content, found, readErr := readNativeConfig(candidate.path)
		if readErr != nil {
			return TmuxConfigImport{}, readErr
		}
		contents[index], exists[index] = content, found
		if active == -1 && found {
			active = index
		}
	}
	for index, candidate := range candidates {
		managed := exists[index] && (hasGeneratedConfigHeader(contents[index]) || hasExactTmuxManagedSection(contents[index]))
		result.Sources = append(result.Sources, ConfigImportSource{
			Path: candidate.path, Exists: exists[index], Active: index == active, Managed: managed,
		})
	}
	if active == -1 {
		return result, nil
	}
	parsed := parseTmuxConfigImport(candidates[active].path, contents[active])
	parsed.Sources = result.Sources
	return parsed, nil
}

func hasExactTmuxManagedSection(content []byte) bool {
	text := string(content)
	starts := exactManagedMarkerLines(text, tmuxManagedStart)
	ends := exactManagedMarkerLines(text, tmuxManagedEnd)
	return len(starts) == 1 && len(ends) == 1 && starts[0].start < ends[0].start
}

type tmuxParsedValue[T any] struct {
	value      T
	provenance ConfigFieldProvenance
	present    bool
}

type tmuxImportParser struct {
	result        TmuxConfigImport
	path          string
	legacyManaged bool
	inManaged     bool
	baseIndex     tmuxParsedValue[int]
	paneBaseIndex tmuxParsedValue[int]
	splitKeys     map[string]tmuxParsedValue[string]
}

func parseTmuxConfigImport(path string, content []byte) TmuxConfigImport {
	parser := tmuxImportParser{
		result:        TmuxConfigImport{Fields: make(map[string]ConfigFieldProvenance)},
		path:          path,
		legacyManaged: hasGeneratedConfigHeader(content),
		splitKeys:     make(map[string]tmuxParsedValue[string]),
	}
	text := string(content)
	starts := exactManagedMarkerLines(text, tmuxManagedStart)
	ends := exactManagedMarkerLines(text, tmuxManagedEnd)
	if !parser.legacyManaged && (len(starts) != len(ends) || len(starts) > 1 || (len(starts) == 1 && starts[0].start >= ends[0].start)) {
		parser.warn("ambiguous or incomplete dotfiles tmux managed section")
		return parser.result
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		parser.parseLine(strings.TrimSuffix(scanner.Text(), "\r"), line)
	}
	if err := scanner.Err(); err != nil {
		parser.warn("tmux config exceeds the static import line limit")
		return parser.result
	}
	parser.finishCompositeFields()
	return parser.result
}

func (p *tmuxImportParser) warn(message string) {
	p.result.Warnings = append(p.result.Warnings, message)
}

func (p *tmuxImportParser) provenance(line int, key string) ConfigFieldProvenance {
	scope := ConfigValueNative
	if p.legacyManaged || p.inManaged {
		scope = ConfigValueManaged
	}
	return ConfigFieldProvenance{Path: p.path, Line: line, Key: key, Scope: scope}
}

func (p *tmuxImportParser) parseLine(raw string, line int) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		switch trimmed {
		case tmuxManagedStart:
			p.inManaged = true
		case tmuxManagedEnd:
			p.inManaged = false
		}
		return
	}
	if strings.HasSuffix(trimmed, "\\") {
		p.warn(fmt.Sprintf("line %d uses a continuation that cannot be imported safely", line))
		return
	}
	tokens := tmuxLiteralTokens(trimmed)
	if len(tokens) == 0 {
		return
	}
	command := tokens[0]
	if strings.HasPrefix(command, "%") || command == "source" || command == "source-file" || command == "if" || command == "if-shell" || command == "run-shell" {
		p.warn(fmt.Sprintf("line %d can change effective settings through indirection", line))
		return
	}
	if command == "run" {
		if len(tokens) == 2 && trimTmuxLiteral(tokens[1]) == "~/.tmux/plugins/tpm/tpm" {
			p.setBool(TmuxFieldTPMEnabled, true, line, "run")
			return
		}
		p.warn(fmt.Sprintf("line %d runs a command that cannot be imported safely", line))
		return
	}
	if strings.Contains(trimmed, ";") {
		// A semicolon inside a bind command defines what the key will execute; it
		// does not change import-time option precedence. Other command sequences
		// are not statically representable.
		if command != "bind" && command != "bind-key" {
			p.warn(fmt.Sprintf("line %d contains multiple tmux commands", line))
			return
		}
	}
	switch command {
	case "set", "set-option", "setw", "set-window-option":
		p.parseSet(tokens, line)
	case "bind", "bind-key", "unbind", "unbind-key":
		p.parseSplitBinding(tokens, line)
	case "se", "seto", "set-opt", "set-w":
		p.warn(fmt.Sprintf("line %d uses a command abbreviation that cannot be imported safely", line))
	}
}

func tmuxLiteralTokens(line string) []string {
	tokens := strings.Fields(line)
	for index, token := range tokens {
		if strings.HasPrefix(token, "#") {
			return tokens[:index]
		}
	}
	return tokens
}

func trimTmuxLiteral(value string) string {
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
		return value[1 : len(value)-1]
	}
	return value
}

func (p *tmuxImportParser) parseSet(tokens []string, line int) {
	if len(tokens) < 3 {
		return
	}
	optionIndex := 1
	var flags string
	if strings.HasPrefix(tokens[optionIndex], "-") {
		flags = tokens[optionIndex]
		optionIndex++
	}
	if optionIndex >= len(tokens) {
		return
	}
	option := tokens[optionIndex]
	if !knownTmuxImportOption(option) {
		for _, later := range tokens[optionIndex+1:] {
			if knownTmuxImportOption(later) {
				p.warn(fmt.Sprintf("line %d has an unrepresentable targeted assignment for %s", line, later))
				break
			}
		}
		return
	}
	if optionIndex+2 != len(tokens) || !validTmuxImportFlags(tokens[0], option, flags) {
		p.warn(fmt.Sprintf("line %d has an unrepresentable assignment for %s", line, option))
		return
	}
	value := trimTmuxLiteral(tokens[optionIndex+1])
	if value == "" || strings.ContainsAny(value, "$\\") || strings.Contains(value, "#{") {
		p.warn(fmt.Sprintf("line %d has a dynamic value for %s", line, option))
		return
	}
	p.applyOption(option, value, line)
}

func knownTmuxImportOption(option string) bool {
	switch option {
	case "prefix", "status-position", "mouse", "base-index", "pane-base-index", "pane-border-lines", "history-limit", "escape-time", "aggressive-resize", "@plugin", "@continuum-save-interval", "@continuum-restore":
		return true
	default:
		return false
	}
}

func validTmuxImportFlags(command, option, flags string) bool {
	if strings.ContainsAny(flags, "aut") {
		return false
	}
	switch option {
	case "escape-time":
		return (command == "set" || command == "set-option") && (flags == "-sg" || flags == "-gs")
	case "pane-base-index", "pane-border-lines", "aggressive-resize":
		return (command == "setw" || command == "set-window-option") && flags == "-g"
	default:
		return (command == "set" || command == "set-option") && flags == "-g"
	}
}

func (p *tmuxImportParser) applyOption(option, value string, line int) {
	switch option {
	case "prefix":
		mapped := map[string]string{"C-a": "ctrl-a", "C-b": "ctrl-b", "C-Space": "ctrl-space"}[value]
		if mapped == "" {
			p.warn(fmt.Sprintf("line %d uses unsupported tmux prefix %q", line, value))
			return
		}
		p.result.Config.Prefix = mapped
		p.result.Fields[TmuxFieldPrefix] = p.provenance(line, option)
	case "status-position":
		if value != "top" && value != "bottom" {
			p.warn(fmt.Sprintf("line %d uses unsupported status position %q", line, value))
			return
		}
		p.result.Config.StatusBar = value
		p.result.Fields[TmuxFieldStatusPosition] = p.provenance(line, option)
	case "mouse":
		p.applyBoolOption(TmuxFieldMouse, &p.result.Config.MouseMode, option, value, line)
	case "base-index", "pane-base-index":
		valueInt, err := strconv.Atoi(value)
		if err != nil || valueInt < 0 || valueInt > 10 {
			p.warn(fmt.Sprintf("line %d uses unsupported %s %q", line, option, value))
			return
		}
		parsed := tmuxParsedValue[int]{value: valueInt, provenance: p.provenance(line, option), present: true}
		if option == "base-index" {
			p.baseIndex = parsed
		} else {
			p.paneBaseIndex = parsed
		}
	case "pane-border-lines":
		if value != "single" && value != "double" && value != "heavy" && value != "simple" {
			p.warn(fmt.Sprintf("line %d uses unsupported pane border %q", line, value))
			return
		}
		p.result.Config.PaneBorderStyle = value
		p.result.Fields[TmuxFieldPaneBorder] = p.provenance(line, option)
	case "history-limit":
		p.applyIntOption(TmuxFieldHistoryLimit, &p.result.Config.HistoryLimit, option, value, line, 1000, 200000)
	case "escape-time":
		p.applyIntOption(TmuxFieldEscapeTime, &p.result.Config.EscapeTime, option, value, line, 0, 1000)
	case "aggressive-resize":
		p.applyBoolOption(TmuxFieldAggressiveResize, &p.result.Config.AggressiveResize, option, value, line)
	case "@continuum-save-interval":
		p.applyIntOption(TmuxFieldContinuumSaveMin, &p.result.Config.ContinuumSaveMin, option, value, line, 5, 60)
	case "@continuum-restore":
		p.applyBoolOption(TmuxFieldContinuumRestore, &p.result.Config.ContinuumRestore, option, value, line)
	case "@plugin":
		p.applyPlugin(value, line)
	}
}

func (p *tmuxImportParser) applyBoolOption(field string, destination *bool, option, value string, line int) {
	if value != "on" && value != "off" {
		p.warn(fmt.Sprintf("line %d uses unsupported %s value %q", line, option, value))
		return
	}
	*destination = value == "on"
	p.result.Fields[field] = p.provenance(line, option)
}

func (p *tmuxImportParser) applyIntOption(field string, destination *int, option, value string, line, minimum, maximum int) {
	valueInt, err := strconv.Atoi(value)
	if err != nil || valueInt < minimum || valueInt > maximum {
		p.warn(fmt.Sprintf("line %d uses unsupported %s value %q", line, option, value))
		return
	}
	*destination = valueInt
	p.result.Fields[field] = p.provenance(line, option)
}

func (p *tmuxImportParser) applyPlugin(value string, line int) {
	field := map[string]string{
		"tmux-plugins/tpm":            TmuxFieldTPMEnabled,
		"tmux-plugins/tmux-sensible":  TmuxFieldPluginSensible,
		"tmux-plugins/tmux-resurrect": TmuxFieldPluginResurrect,
		"tmux-plugins/tmux-continuum": TmuxFieldPluginContinuum,
		"tmux-plugins/tmux-yank":      TmuxFieldPluginYank,
	}[value]
	if field == "" {
		return
	}
	p.setBool(field, true, line, "@plugin")
}

func (p *tmuxImportParser) setBool(field string, value bool, line int, key string) {
	switch field {
	case TmuxFieldTPMEnabled:
		p.result.Config.TPMEnabled = value
	case TmuxFieldPluginSensible:
		p.result.Config.PluginSensible = value
	case TmuxFieldPluginResurrect:
		p.result.Config.PluginResurrect = value
	case TmuxFieldPluginContinuum:
		p.result.Config.PluginContinuum = value
	case TmuxFieldPluginYank:
		p.result.Config.PluginYank = value
	}
	p.result.Fields[field] = p.provenance(line, key)
}

func (p *tmuxImportParser) parseSplitBinding(tokens []string, line int) {
	command := tokens[0]
	if (command == "unbind" || command == "unbind-key") && len(tokens) == 2 {
		delete(p.splitKeys, trimTmuxLiteral(tokens[1]))
		return
	}
	if command != "bind" && command != "bind-key" {
		return
	}
	if len(tokens) < 4 || (strings.HasPrefix(tokens[1], "-") && tokens[1] != "-") {
		return
	}
	key := trimTmuxLiteral(tokens[1])
	if tokens[2] != "split-window" || (key != "|" && key != "-" && key != "%" && key != `"`) {
		return
	}
	orientation := ""
	for _, token := range tokens[3:] {
		switch token {
		case "-h":
			orientation = "horizontal"
		case "-v":
			orientation = "vertical"
		}
	}
	wantOrientation := map[string]string{"|": "horizontal", "-": "vertical", "%": "horizontal", `"`: "vertical"}[key]
	if orientation != wantOrientation {
		p.warn(fmt.Sprintf("line %d has an unrepresentable split binding", line))
		return
	}
	style := "percent"
	if key == "|" || key == "-" {
		style = "pipes"
	}
	p.splitKeys[key] = tmuxParsedValue[string]{value: style, provenance: p.provenance(line, "bind "+key), present: true}
}

func (p *tmuxImportParser) finishCompositeFields() {
	if p.baseIndex.present || p.paneBaseIndex.present {
		if !p.baseIndex.present || !p.paneBaseIndex.present || p.baseIndex.value != p.paneBaseIndex.value {
			p.warn("base-index and pane-base-index are incomplete or disagree")
		} else {
			p.result.Config.BaseIndex = p.baseIndex.value
			provenance := p.baseIndex.provenance
			if p.paneBaseIndex.provenance.Line > provenance.Line {
				provenance = p.paneBaseIndex.provenance
			}
			p.result.Fields[TmuxFieldBaseIndex] = provenance
		}
	}
	pipes := p.splitKeys["|"].present && p.splitKeys["-"].present
	percent := p.splitKeys["%"].present && p.splitKeys[`"`].present
	anySplit := len(p.splitKeys) != 0
	if pipes == percent {
		if anySplit {
			p.warn("split bindings are incomplete or enable multiple styles")
		}
		return
	}
	style := "percent"
	provenance := p.splitKeys["%"].provenance
	if pipes {
		style = "pipes"
		provenance = p.splitKeys["|"].provenance
	}
	p.result.Config.SplitBinds = style
	p.result.Fields[TmuxFieldSplitBinds] = provenance
}
