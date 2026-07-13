package installplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestAcceptedHashSupersedesLegacyPackageOnlyAuthorityFormula(t *testing.T) {
	gitRecipe := reviewedPackageRecipe("git")
	missingSnapshot := mustSnapshot(t, 51, mustObservation(t, "git", health.PackageMissing, gitRecipe))
	missing, err := Build(Request{Intent: mustIntent(t, "git"), Snapshot: missingSnapshot, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 51}}, countingDependencies(&dependencyCounts{}, gitRecipe))
	if err != nil {
		t.Fatal(err)
	}
	missingAccepted, ok := missing.Accepted()
	if !ok {
		t.Fatal("missing-tool plan omitted accepted authority")
	}
	if legacy := legacyPackageOnlyHash(missingAccepted); missingAccepted.Hash() == legacy {
		t.Fatalf("missing accepted hash still matches incomplete legacy authority %q", legacy)
	}

	codexRecipe := reviewedPackageRecipe("codex")
	zshRecipe := reviewedPackageRecipe("zsh")
	mixedSnapshot := mustSnapshot(t, 52,
		mustObservation(t, "codex", health.PackagePresent, codexRecipe),
		mustObservation(t, "git", health.PackageMissing, gitRecipe),
		mustPartialObservation(t, "zsh", zshRecipe),
	)
	counts := &dependencyCounts{}
	deps := countingDependencies(counts, operation.InstallRecipe{})
	deps.DescribeInstall = func(tool tools.Tool, _ tools.InstallEnvironment) (operation.InstallRecipe, error) {
		counts.describe++
		counts.sequence = append(counts.sequence, "describe:"+tool.ID())
		return operation.CloneInstallRecipe(map[string]operation.InstallRecipe{"git": gitRecipe, "zsh": zshRecipe}[tool.ID()]), nil
	}
	mixed, err := Build(Request{Intent: mustIntent(t, "zsh", "codex", "git"), Snapshot: mixedSnapshot, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 52}}, deps)
	if err != nil {
		t.Fatal(err)
	}
	mixedAccepted, ok := mixed.Accepted()
	if !ok {
		t.Fatal("mixed plan omitted accepted authority")
	}
	if legacy := legacyPackageOnlyHash(mixedAccepted); mixedAccepted.Hash() == legacy {
		t.Fatalf("mixed accepted hash still matches incomplete legacy authority %q", legacy)
	}
}

func TestPublicPackageObservationFactsReflectPresenceWithoutClaimingManagement(t *testing.T) {
	codexRecipe := reviewedPackageRecipe("codex")
	gitRecipe := reviewedPackageRecipe("git")
	zshRecipe := reviewedPackageRecipe("zsh")
	snapshot := mustSnapshot(t, 53,
		mustObservation(t, "codex", health.PackagePresent, codexRecipe),
		mustObservation(t, "git", health.PackageMissing, gitRecipe),
		mustPartialObservation(t, "zsh", zshRecipe),
	)
	counts := &dependencyCounts{}
	deps := countingDependencies(counts, operation.InstallRecipe{})
	deps.DescribeInstall = func(tool tools.Tool, _ tools.InstallEnvironment) (operation.InstallRecipe, error) {
		counts.describe++
		counts.sequence = append(counts.sequence, "describe:"+tool.ID())
		return operation.CloneInstallRecipe(map[string]operation.InstallRecipe{"git": gitRecipe, "zsh": zshRecipe}[tool.ID()]), nil
	}
	result, err := Build(Request{Intent: mustIntent(t, "zsh", "git", "codex"), Snapshot: snapshot, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 53}}, deps)
	if err != nil {
		t.Fatal(err)
	}
	wantExists := map[string]bool{"codex": true, "git": false, "zsh": true}
	for _, action := range result.Public().Actions() {
		if action.Observation == nil || action.Observation.Exists != wantExists[action.ToolID] || action.Observation.Managed {
			t.Fatalf("public observation for %s=%+v, want exists=%v managed=false", action.ToolID, action.Observation, wantExists[action.ToolID])
		}
	}

	partial := mustPartialObservation(t, "zsh", zshRecipe)
	driftSnapshot := mustSnapshot(t, 54, partial)
	driftDeps := countingDependencies(&dependencyCounts{}, zshRecipe)
	driftDeps.DescribeInstall = func(tools.Tool, tools.InstallEnvironment) (operation.InstallRecipe, error) {
		drifted := operation.CloneInstallRecipe(zshRecipe)
		drifted.Risk = "different reviewed risk"
		return drifted, nil
	}
	blocked, err := Build(Request{Intent: mustIntent(t, "zsh"), Snapshot: driftSnapshot, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 54}}, driftDeps)
	if err != nil {
		t.Fatal(err)
	}
	actions := blocked.Public().Actions()
	if len(actions) != 1 || actions[0].ReasonCode != "recipe_drift" || actions[0].Observation == nil || !actions[0].Observation.Exists || actions[0].Observation.Managed {
		t.Fatalf("blocked partial observation=%+v", actions)
	}
}

func TestTypedNilLookupFailsClosedWithoutPanicOrLaterDependencies(t *testing.T) {
	recipe := reviewedPackageRecipe("git")
	snapshot := mustSnapshot(t, 55, mustObservation(t, "git", health.PackageMissing, recipe))
	counts := &dependencyCounts{}
	deps := countingDependencies(counts, recipe)
	var nilGit *tools.GitTool
	deps.LookupTool = func(id string) (tools.Tool, bool) {
		counts.lookup++
		counts.sequence = append(counts.sequence, "lookup:"+id)
		return nilGit, true
	}
	var result Result
	var err error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("typed-nil lookup panicked: %v", recovered)
			}
		}()
		result, err = Build(Request{Intent: mustIntent(t, "git"), Snapshot: snapshot, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 55}}, deps)
	}()
	assertBlockedWithoutAccepted(t, result, err, "recipe_drift")
	if !reflect.DeepEqual(counts.sequence, []string{"lookup:git"}) || counts.describe != 0 || counts.capture != 0 || counts.clock != 0 {
		t.Fatalf("typed-nil lookup dependency calls=%+v", *counts)
	}
}

func legacyPackageOnlyHash(accepted AcceptedPlan) string {
	authority := accepted.SnapshotAuthority()
	authorities := accepted.ToolAuthorities()
	rows := make([]string, 0, len(authorities))
	for id, value := range authorities {
		rows = append(rows, id+":"+string(value.Presence)+":"+value.Intent+":"+value.RecipeDigest)
	}
	sort.Strings(rows)
	canonical, err := json.Marshal(struct {
		Document   string
		Schema     int
		Generation uint64
		Platform   string
		Manager    string
		Snapshot   string
		Recipes    []string
	}{accepted.Operation().Hash(), authority.SchemaVersion, authority.Generation, authority.Platform, authority.Manager, authority.Digest, rows})
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}
