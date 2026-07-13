package installplan

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/planpublic"
)

func TestReadyPublicDocumentPublishesAcceptedHashAndNeutralDocumentsOmitIt(t *testing.T) {
	recipe := reviewedPackageRecipe("git")
	readySnapshot := mustSnapshot(t, 81, mustObservation(t, "git", health.PackageMissing, recipe))
	ready, err := Build(Request{
		Intent: mustIntent(t, "git"), Snapshot: readySnapshot,
		Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 81},
	}, countingDependencies(&dependencyCounts{}, recipe))
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := ready.Accepted()
	if !ok {
		t.Fatal("ready result omitted accepted authority")
	}
	encoded, err := planpublic.MarshalDocument(ready.Public())
	if err != nil {
		t.Fatal(err)
	}
	var public struct {
		Authority struct {
			PlanHash string `json:"plan_hash"`
		} `json:"authority"`
	}
	if err := json.Unmarshal(encoded, &public); err != nil || public.Authority.PlanHash != accepted.Hash() {
		t.Fatalf("public plan hash=%q accepted=%q err=%v document=%s", public.Authority.PlanHash, accepted.Hash(), err, encoded)
	}

	presentSnapshot := mustSnapshot(t, 82, mustObservation(t, "git", health.PackagePresent, recipe))
	noChanges, err := Build(Request{
		Intent: mustIntent(t, "git"), Snapshot: presentSnapshot,
		Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 82},
	}, countingDependencies(&dependencyCounts{}, recipe))
	if err != nil {
		t.Fatal(err)
	}
	noChangesJSON, err := planpublic.MarshalDocument(noChanges.Public())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(noChangesJSON, []byte(`"plan_hash"`)) {
		t.Fatalf("no_changes published private authority: %s", noChangesJSON)
	}

	blockedSnapshot := mustSnapshot(t, 83)
	blocked, err := Build(Request{
		Intent: mustIntent(t, "git"), Snapshot: blockedSnapshot,
		Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 83},
	}, countingDependencies(&dependencyCounts{}, recipe))
	if err != nil {
		t.Fatal(err)
	}
	blockedJSON, err := planpublic.MarshalDocument(blocked.Public())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blockedJSON, []byte(`"plan_hash"`)) {
		t.Fatalf("blocked published private authority: %s", blockedJSON)
	}
}
