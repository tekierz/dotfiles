package planpublic

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

const explicitIntentSchemaVersion = 1
const maxExplicitTools = 256

var (
	ErrIntentRequired = errors.New("explicit tool intent is required")
	ErrInvalidIntent  = errors.New("invalid explicit tool intent")
)

// Intent is the normalized, non-secret selection that a caller explicitly
// requested. Tools is always sorted and contains no duplicates.
type Intent struct {
	Source string   `json:"source"`
	Tools  []string `json:"tools"`
	Digest string   `json:"digest,omitempty"`
}

// NormalizeExplicitTools validates both the registry identity set and the
// caller's explicit selections before returning any partial result.
func NormalizeExplicitTools(values, registered []string) (Intent, error) {
	if len(values) > maxExplicitTools || len(registered) > maxExplicitTools {
		return Intent{}, ErrInvalidIntent
	}
	known := make(map[string]struct{}, len(registered))
	for _, id := range registered {
		if !validToolID(id) {
			return Intent{}, ErrInvalidIntent
		}
		if _, duplicate := known[id]; duplicate {
			return Intent{}, ErrInvalidIntent
		}
		known[id] = struct{}{}
	}

	selected := make(map[string]struct{}, len(values))
	anyBlank := false
	for _, raw := range values {
		id := strings.TrimSpace(raw)
		if id == "" {
			anyBlank = true
			continue
		}
		if !validToolID(id) {
			return Intent{}, ErrInvalidIntent
		}
		if _, exists := known[id]; !exists {
			return Intent{}, ErrInvalidIntent
		}
		selected[id] = struct{}{}
	}
	if len(selected) == 0 {
		return Intent{}, ErrIntentRequired
	}
	if anyBlank {
		return Intent{}, ErrInvalidIntent
	}

	tools := make([]string, 0, len(selected))
	for id := range selected {
		tools = append(tools, id)
	}
	sort.Strings(tools)
	canonical, err := json.Marshal(struct {
		SchemaVersion int      `json:"schema_version"`
		Source        string   `json:"source"`
		Tools         []string `json:"tools"`
	}{SchemaVersion: explicitIntentSchemaVersion, Source: "explicit_tools", Tools: tools})
	if err != nil {
		return Intent{}, ErrInvalidIntent
	}
	digest := sha256.Sum256(canonical)
	return Intent{Source: "explicit_tools", Tools: append([]string(nil), tools...), Digest: hex.EncodeToString(digest[:])}, nil
}

func validToolID(value string) bool {
	if value == "" || len(value) > 64 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	previousDash := false
	for _, r := range value {
		if r == '-' {
			if previousDash {
				return false
			}
			previousDash = true
			continue
		}
		previousDash = false
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}
