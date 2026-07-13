package ui

import (
	"fmt"
	"slices"

	"github.com/tekierz/dotfiles/internal/health"
	headless "github.com/tekierz/dotfiles/internal/installplan"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/planpublic"
)

// adoptHeadlessInstallPlan translates one already-accepted neutral package plan
// into the private UI execution snapshot. It performs no detection, planning,
// configuration hydration, state capture, or filesystem access.
func adoptHeadlessInstallPlan(app *App, accepted headless.AcceptedPlan) (*installPlan, error) {
	if app == nil || accepted.StatePlan() == nil || !validAdapterDigest(accepted.Hash()) {
		return nil, fmt.Errorf("headless install authority is unavailable")
	}
	document := accepted.Operation()
	actions := document.Actions()
	if document.SchemaVersion() != operation.CurrentPlanSchemaVersion || document.Hash() == "" || len(actions) == 0 {
		return nil, fmt.Errorf("headless operation document is unavailable")
	}
	snapshot := accepted.SnapshotAuthority()
	if snapshot.SchemaVersion != health.CurrentInstallationSchemaVersion || snapshot.Generation == 0 || snapshot.Platform == "" || snapshot.Manager == "" || !validAdapterDigest(snapshot.Digest) {
		return nil, fmt.Errorf("headless installation snapshot authority is invalid")
	}
	intent := accepted.Intent()
	normalized, err := planpublic.NormalizeExplicitTools(intent.Tools, intent.Tools)
	if err != nil || intent.Source != normalized.Source || intent.Digest != normalized.Digest || !slices.Equal(intent.Tools, normalized.Tools) {
		return nil, fmt.Errorf("headless explicit intent is invalid")
	}
	neutralAuthorities := accepted.ToolAuthorities()
	neutralRecipes := accepted.Recipes()
	if len(neutralAuthorities) != len(intent.Tools) {
		return nil, fmt.Errorf("headless tool authority does not cover explicit intent")
	}

	installAuthorities := make(map[string]installToolAuthority, len(neutralAuthorities))
	for _, id := range intent.Tools {
		authority, ok := neutralAuthorities[id]
		if !ok {
			return nil, fmt.Errorf("headless tool authority for %s is unavailable", id)
		}
		switch authority.Presence {
		case health.PresencePresent:
			if authority.Intent != "none" || authority.RecipeDigest != "" {
				return nil, fmt.Errorf("present headless tool %s has mutation authority", id)
			}
		case health.PresenceMissing, health.PresencePartial:
			wantIntent := "install"
			if authority.Presence == health.PresencePartial {
				wantIntent = "repair"
			}
			if authority.Intent != wantIntent || !validAdapterDigest(authority.RecipeDigest) {
				return nil, fmt.Errorf("headless mutation authority for %s is invalid", id)
			}
		default:
			return nil, fmt.Errorf("headless tool %s has unresolved presence", id)
		}
		installAuthorities[id] = installToolAuthority{presence: authority.Presence, intent: authority.Intent, recipeDigest: authority.RecipeDigest}
	}

	mutationCount := 0
	for _, action := range actions {
		if action.Kind != operation.KindInstallTool || action.Disposition != operation.DispositionApply || action.ToolID == "" || action.Target != action.ToolID || action.InstallRecipe == nil || action.InstallDetected == nil || *action.InstallDetected {
			return nil, fmt.Errorf("headless action %s is not a reviewed package mutation", action.ID)
		}
		authority, ok := installAuthorities[action.ToolID]
		recipe, recipeOK := neutralRecipes[action.ToolID]
		if !ok || !recipeOK || (authority.intent != "install" && authority.intent != "repair") || action.Description != authority.intent+" "+action.ToolID ||
			action.DesiredDigest != authority.recipeDigest || installRecipeDigest(recipe) != authority.recipeDigest || !operationRecipesEqual(*action.InstallRecipe, recipe) {
			return nil, fmt.Errorf("headless action %s disagrees with accepted authority", action.ID)
		}
		mutationCount++
	}
	if mutationCount == 0 || mutationCount != len(neutralRecipes) {
		return nil, fmt.Errorf("headless recipe authority contains an unbound mutation")
	}

	plan := &installPlan{
		document: document, installHash: accepted.Hash(),
		installSnapshot: installationSnapshotAuthority{schema: snapshot.SchemaVersion, generation: snapshot.Generation, platform: snapshot.Platform, manager: snapshot.Manager, digest: snapshot.Digest},
		installTools:    installAuthorities, installRecipes: cloneInstallRecipes(neutralRecipes),
		selectedTools: nil, configTools: nil, config: DeepDiveConfig{},
		theme: app.theme, navStyle: app.navStyle, animations: app.animationsEnabled,
		authority: make(map[string]map[string]acceptedTarget), parentDirs: nil,
		statePlan: accepted.StatePlan(),
	}
	if _, err := plan.installExecutionSnapshot(); err != nil {
		return nil, fmt.Errorf("validate adopted execution snapshot: %w", err)
	}
	return plan, nil
}

func validAdapterDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func operationRecipesEqual(left, right operation.InstallRecipe) bool {
	leftDigest, leftErr := operation.InstallRecipeDigest(left)
	rightDigest, rightErr := operation.InstallRecipeDigest(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}
