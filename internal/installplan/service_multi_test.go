package installplan

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/planpublic"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestBuildMixedPresentMissingAndPartialPreservesCanonicalIntentAndAuthority(t *testing.T) {
	codexRecipe := reviewedPackageRecipe("codex")
	gitRecipe := reviewedPackageRecipe("git")
	zshRecipe := reviewedPackageRecipe("zsh")
	snapshot := mustSnapshot(t, 23,
		mustObservation(t, "codex", health.PackagePresent, codexRecipe),
		mustObservation(t, "git", health.PackageMissing, gitRecipe),
		mustPartialObservation(t, "zsh", zshRecipe),
	)
	counts := &dependencyCounts{}
	deps := countingDependencies(counts, operation.InstallRecipe{})
	recipes := map[string]operation.InstallRecipe{"git": gitRecipe, "zsh": zshRecipe}
	deps.DescribeInstall = func(tool tools.Tool, _ tools.InstallEnvironment) (operation.InstallRecipe, error) {
		counts.describe++
		counts.sequence = append(counts.sequence, "describe:"+tool.ID())
		return operation.CloneInstallRecipe(recipes[tool.ID()]), nil
	}
	intent := mustIntent(t, "zsh", "codex", "git")
	result, err := Build(Request{Intent: intent, Snapshot: snapshot, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 23}}, deps)
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := result.Accepted()
	if !ok {
		t.Fatal("mixed plan omitted accepted authority")
	}
	privateActions := accepted.Operation().Actions()
	if len(privateActions) != 2 || privateActions[0].ToolID != "git" || privateActions[0].Description != "install git" || privateActions[1].ToolID != "zsh" || privateActions[1].Description != "repair zsh" {
		t.Fatalf("private mutation order/actions=%+v", privateActions)
	}
	publicActions := result.Public().Actions()
	if len(publicActions) != 3 || publicActions[0].ToolID != "codex" || publicActions[0].Disposition != "skip" || publicActions[0].ReasonCode != "present" ||
		publicActions[1].ToolID != "git" || publicActions[1].Disposition != "apply" || publicActions[2].ToolID != "zsh" || publicActions[2].Disposition != "apply" || publicActions[2].Description != "install zsh" {
		t.Fatalf("public selected-tool order/actions=%+v", publicActions)
	}
	for id, want := range map[string]struct {
		presence health.Presence
		intent   string
		digest   string
	}{
		"codex": {presence: health.PresencePresent, intent: "none"},
		"git":   {presence: health.PresenceMissing, intent: "install", digest: mustRecipeDigest(t, gitRecipe)},
		"zsh":   {presence: health.PresencePartial, intent: "repair", digest: mustRecipeDigest(t, zshRecipe)},
	} {
		authority, found := accepted.ToolAuthority(id)
		if !found || authority.Presence != want.presence || authority.Intent != want.intent || authority.RecipeDigest != want.digest {
			t.Fatalf("tool authority %s=%+v found=%v, want %+v", id, authority, found, want)
		}
	}
	if counts.lookup != 2 || counts.describe != 2 || counts.capture != 1 || counts.clock != 1 {
		t.Fatalf("mixed dependency counts=%+v", *counts)
	}
	wantSequence := []string{"lookup:git", "describe:git", "lookup:zsh", "describe:zsh", "capture", "clock"}
	if !reflect.DeepEqual(counts.sequence, wantSequence) {
		t.Fatalf("mixed dependency sequence=%v, want %v", counts.sequence, wantSequence)
	}
	originalTools := append([]string(nil), intent.Tools...)
	intent.Tools[0] = "mutated-request"
	acceptedIntent := accepted.Intent()
	if !reflect.DeepEqual(acceptedIntent.Tools, originalTools) {
		t.Fatalf("accepted intent aliases request: %v, want %v", acceptedIntent.Tools, originalTools)
	}
	acceptedIntent.Tools[0] = "mutated-accessor"
	if got := accepted.Intent().Tools; !reflect.DeepEqual(got, originalTools) {
		t.Fatalf("accepted intent accessor aliases private intent: %v", got)
	}
}

func TestBuildEquivalentIntentOrderIsDeterministic(t *testing.T) {
	gitRecipe := reviewedPackageRecipe("git")
	piRecipe := reviewedPackageRecipe("pi")
	snapshot := mustSnapshot(t, 29, mustObservation(t, "git", health.PackageMissing, gitRecipe), mustObservation(t, "pi", health.PackageMissing, piRecipe))
	build := func(raw []string) Result {
		t.Helper()
		intent, err := planpublic.NormalizeExplicitTools(raw, []string{"git", "pi"})
		if err != nil {
			t.Fatal(err)
		}
		counts := &dependencyCounts{}
		deps := countingDependencies(counts, operation.InstallRecipe{})
		deps.DescribeInstall = func(tool tools.Tool, _ tools.InstallEnvironment) (operation.InstallRecipe, error) {
			counts.describe++
			counts.sequence = append(counts.sequence, "describe:"+tool.ID())
			return operation.CloneInstallRecipe(map[string]operation.InstallRecipe{"git": gitRecipe, "pi": piRecipe}[tool.ID()]), nil
		}
		result, err := Build(Request{Intent: intent, Snapshot: snapshot, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 29}}, deps)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := build([]string{"pi", "git", "pi"})
	second := build([]string{"git", "pi"})
	firstAccepted, firstOK := first.Accepted()
	secondAccepted, secondOK := second.Accepted()
	if !firstOK || !secondOK {
		t.Fatal("deterministic mutation plans omitted accepted authority")
	}
	if firstAccepted.Hash() != secondAccepted.Hash() || !reflect.DeepEqual(firstAccepted.Operation().Actions(), secondAccepted.Operation().Actions()) {
		t.Fatal("equivalent explicit intent changed private actions/hash")
	}
	firstJSON, _ := planpublic.MarshalDocument(first.Public())
	secondJSON, _ := planpublic.MarshalDocument(second.Public())
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatalf("equivalent explicit intent changed public bytes:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestSelectedToolPublicSnapshotDigestIsScopedAndPrivateDriftChangesOnlyPublishedAuthority(t *testing.T) {
	gitRecipe := reviewedPackageRecipe("git")
	otherRecipe := reviewedPackageRecipe("pi")
	intent := mustIntent(t, "git")
	build := func(snapshot health.InstallationSnapshot, recipe operation.InstallRecipe, environment Environment) Result {
		t.Helper()
		result, err := Build(Request{Intent: intent, Snapshot: snapshot, Environment: environment}, countingDependencies(&dependencyCounts{}, recipe))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	baseObservation := mustObservationWithReceipt(t, "git", "git", gitRecipe)
	privateEvidenceDrift := mustObservationWithReceipt(t, "git", "git-private-alias", gitRecipe)
	baseSnapshot := mustSnapshot(t, 31, baseObservation, mustObservation(t, "pi", health.PackagePresent, otherRecipe))
	unselectedDriftSnapshot := mustSnapshot(t, 31, baseObservation, mustObservation(t, "pi", health.PackageMissing, otherRecipe))
	privateEvidenceSnapshot := mustSnapshot(t, 31, privateEvidenceDrift, mustObservation(t, "pi", health.PackagePresent, otherRecipe))
	publicFactSnapshot := mustSnapshot(t, 31, mustPartialObservation(t, "git", gitRecipe), mustObservation(t, "pi", health.PackagePresent, otherRecipe))
	generationSnapshot := mustSnapshot(t, 32, baseObservation, mustObservation(t, "pi", health.PackagePresent, otherRecipe))

	baseEnvironment := Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 31}
	first := build(baseSnapshot, gitRecipe, baseEnvironment)
	second := build(unselectedDriftSnapshot, gitRecipe, baseEnvironment)
	privateDrift := build(privateEvidenceSnapshot, gitRecipe, baseEnvironment)
	publicDrift := build(publicFactSnapshot, gitRecipe, baseEnvironment)
	generationDrift := build(generationSnapshot, gitRecipe, Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 32})
	firstDigest := publicSnapshotDigest(t, first.Public())
	secondDigest := publicSnapshotDigest(t, second.Public())
	privateDriftDigest := publicSnapshotDigest(t, privateDrift.Public())
	publicDriftDigest := publicSnapshotDigest(t, publicDrift.Public())
	generationDriftDigest := publicSnapshotDigest(t, generationDrift.Public())
	if firstDigest != secondDigest {
		t.Fatal("unselected observation changed selected-tool public snapshot digest")
	}
	if firstDigest != privateDriftDigest {
		t.Fatal("selected private evidence changed public snapshot digest despite identical public facts")
	}
	firstJSON := mustPublicJSON(t, first.Public())
	privateDriftJSON := mustPublicJSON(t, privateDrift.Public())
	firstAccepted, firstOK := first.Accepted()
	privateAccepted, privateOK := privateDrift.Accepted()
	if !firstOK || !privateOK || firstAccepted.Hash() == privateAccepted.Hash() {
		t.Fatalf("private-evidence drift hashes=%q/%q accepted=%v/%v", firstAccepted.Hash(), privateAccepted.Hash(), firstOK, privateOK)
	}
	var firstRedacted, privateRedacted map[string]any
	if err := json.Unmarshal(firstJSON, &firstRedacted); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(privateDriftJSON, &privateRedacted); err != nil {
		t.Fatal(err)
	}
	delete(firstRedacted, "authority")
	delete(privateRedacted, "authority")
	if !reflect.DeepEqual(firstRedacted, privateRedacted) {
		t.Fatalf("private-evidence-only drift changed redacted public payload:\n%s\n%s", firstJSON, privateDriftJSON)
	}
	if firstDigest == publicDriftDigest {
		t.Fatal("selected observation drift did not change public snapshot digest")
	}
	if firstDigest == generationDriftDigest {
		t.Fatal("snapshot generation drift did not change public snapshot digest")
	}
	for _, document := range []struct {
		result   Result
		snapshot health.InstallationSnapshot
	}{{first, baseSnapshot}, {second, unselectedDriftSnapshot}, {privateDrift, privateEvidenceSnapshot}, {publicDrift, publicFactSnapshot}, {generationDrift, generationSnapshot}} {
		encoded := mustPublicJSON(t, document.result.Public())
		if strings.Contains(string(encoded), document.snapshot.Digest()) {
			t.Fatalf("public JSON leaked private snapshot digest %q: %s", document.snapshot.Digest(), encoded)
		}
	}
	if firstDigest == baseSnapshot.Digest() {
		t.Fatal("public selected-tool digest exposed/equaled private snapshot digest")
	}

	archRecipe := recipeForEnvironment(gitRecipe, pkg.PlatformArch, "paru")
	archSnapshot := mustSnapshotForEnvironment(t, 31, pkg.PlatformArch, "paru", mustObservation(t, "git", health.PackageMissing, archRecipe))
	archResult := build(archSnapshot, archRecipe, Environment{Platform: pkg.PlatformArch, Manager: "paru", ExpectedGeneration: 31})
	if firstDigest == publicSnapshotDigest(t, archResult.Public()) {
		t.Fatal("platform/manager drift did not change public snapshot digest")
	}
}

func TestAcceptedAndPublicAccessorsAreDeeplyDefensive(t *testing.T) {
	recipe := reviewedPackageRecipe("git")
	intent := mustIntent(t, "git")
	snapshot := mustSnapshot(t, 41, mustObservation(t, "git", health.PackageMissing, recipe))
	result, err := Build(Request{Intent: intent, Snapshot: snapshot, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 41}}, countingDependencies(&dependencyCounts{}, recipe))
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := result.Accepted()
	if !ok {
		t.Fatal("ready mutation result omitted accepted authority")
	}
	beforeHash := accepted.Hash()
	beforePublic := mustPublicJSON(t, result.Public())

	intent.Tools[0] = "mutated-request"
	recipe.Steps[0].Packages[0] = "mutated-source"
	recipe.Detector.Values[0] = "mutated-source-detector"
	recipes := accepted.Recipes()
	entry := recipes["git"]
	entry.Steps[0].Packages[0] = "mutated-recipe-step"
	entry.Detector.Values[0] = "mutated-recipe-detector"
	recipes["git"] = entry
	authority, found := accepted.ToolAuthority("git")
	if !found {
		t.Fatal("accepted tool authority missing")
	}
	authority.Intent = "mutated-authority"
	authorities := accepted.ToolAuthorities()
	entryAuthority := authorities["git"]
	entryAuthority.Intent = "mutated-authority-map"
	authorities["git"] = entryAuthority
	operationActions := accepted.Operation().Actions()
	operationActions[0].InstallRecipe.Steps[0].Packages[0] = "mutated-operation"
	operationActions[0].InstallRecipe.Detector.Values[0] = "mutated-operation-detector"
	publicActions := result.Public().Actions()
	publicActions[0].Install.Steps[0].Packages[0] = "mutated-public"
	publicActions[0].Install.Detector.Values[0] = "mutated-public-detector"
	acceptedIntent := accepted.Intent()
	acceptedIntent.Tools[0] = "mutated-intent-accessor"

	if accepted.Hash() != beforeHash || !reflect.DeepEqual(mustPublicJSON(t, result.Public()), beforePublic) {
		t.Fatal("mutable source/accessor data changed accepted hash or public bytes")
	}
	if got := accepted.Recipes()["git"]; got.Steps[0].Packages[0] != "git" || got.Detector.Values[0] != "git" {
		t.Fatalf("recipe accessor mutation escaped: %+v", got)
	}
	if got, _ := accepted.ToolAuthority("git"); got.Intent != "install" {
		t.Fatalf("tool authority accessor mutation escaped: %+v", got)
	}
	if got := accepted.ToolAuthorities()["git"]; got.Intent != "install" {
		t.Fatalf("tool authority map mutation escaped: %+v", got)
	}
}

func mustPartialObservation(t *testing.T, id string, recipe operation.InstallRecipe) health.InstallationObservation {
	t.Helper()
	digest := mustRecipeDigest(t, recipe)
	packageFacet := completePackageFacet(health.PackagePresent, recipePackageReceipts(recipe))
	directFacet := directDetectorFacet(recipe.Detector, health.ComponentMissing)
	if recipe.Detector.Kind == operation.InstallDetectorPackageReceipt {
		directFacet = health.DirectFacet{State: health.ComponentUnknown, Authoritative: true, Alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{id}, State: health.ComponentUnknown}}}
	}
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: id, Installability: health.InstallabilitySupported, InstallRecipeDigest: digest,
		Package: packageFacet,
		Direct:  directFacet,
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func mustObservationWithReceipt(t *testing.T, id, receipt string, recipe operation.InstallRecipe) health.InstallationObservation {
	t.Helper()
	receipts := append([]string(nil), recipe.Detector.Values...)
	found := false
	for _, candidate := range receipts {
		if candidate == receipt {
			found = true
			break
		}
	}
	if !found {
		receipts = append(receipts, receipt)
	}
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: id, Installability: health.InstallabilitySupported, InstallRecipeDigest: mustRecipeDigest(t, recipe),
		Package: completePackageFacet(health.PackageMissing, receipts),
		Direct:  health.DirectFacet{State: health.ComponentNotApplicable},
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func recipeForEnvironment(recipe operation.InstallRecipe, platform pkg.Platform, manager string) operation.InstallRecipe {
	result := operation.CloneInstallRecipe(recipe)
	result.Platform = string(platform)
	result.Manager = manager
	for index := range result.Steps {
		if result.Steps[index].Kind == operation.InstallStepPackageManager {
			result.Steps[index].Provider = manager
		}
	}
	return result
}

func mustSnapshotForEnvironment(t *testing.T, generation uint64, platform pkg.Platform, manager string, observations ...health.InstallationObservation) health.InstallationSnapshot {
	t.Helper()
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: generation, Platform: string(platform), Manager: manager, Tools: observations})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func mustRecipeDigest(t *testing.T, recipe operation.InstallRecipe) string {
	t.Helper()
	digest, err := operation.InstallRecipeDigest(recipe)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func publicSnapshotDigest(t *testing.T, document planpublic.Document) string {
	t.Helper()
	encoded, err := planpublic.MarshalDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Snapshot struct {
			PublicDigest string `json:"public_digest"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	if len(value.Snapshot.PublicDigest) != 64 || strings.Trim(value.Snapshot.PublicDigest, "0123456789abcdef") != "" {
		t.Fatalf("invalid public snapshot digest %q", value.Snapshot.PublicDigest)
	}
	return value.Snapshot.PublicDigest
}

func mustPublicJSON(t *testing.T, document planpublic.Document) []byte {
	t.Helper()
	encoded, err := planpublic.MarshalDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
