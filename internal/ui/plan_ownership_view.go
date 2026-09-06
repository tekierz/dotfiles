package ui

import (
	"sort"

	"github.com/tekierz/dotfiles/internal/operation"
)

type targetOwnershipView struct {
	target    string
	ownership operation.Ownership
}

func orderedTargetOwnership(action operation.Action) []targetOwnershipView {
	if len(action.TargetOwnership) == 0 {
		return nil
	}
	result := make([]targetOwnershipView, 0, len(action.TargetOwnership))
	seen := make(map[string]bool, len(action.TargetOwnership))
	for _, observation := range action.Observations {
		if ownership, ok := action.TargetOwnership[observation.Source]; ok && !seen[observation.Source] {
			result = append(result, targetOwnershipView{observation.Source, ownership})
			seen[observation.Source] = true
		}
	}
	remaining := make([]string, 0, len(action.TargetOwnership)-len(result))
	for target := range action.TargetOwnership {
		if !seen[target] {
			remaining = append(remaining, target)
		}
	}
	sort.Strings(remaining)
	for _, target := range remaining {
		result = append(result, targetOwnershipView{target, action.TargetOwnership[target]})
	}
	return result
}
