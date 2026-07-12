package operation

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func validConfigAction() Action {
	return Action{
		ID:            "config:ghostty",
		Kind:          KindWriteConfig,
		ToolID:        "ghostty",
		Target:        ".config/ghostty/config",
		Description:   "merge managed Ghostty settings",
		Disposition:   DispositionApply,
		DesiredDigest: strings.Repeat("a", 64),
		Ownership:     OwnershipManagedFragment,
		Reversibility: ReversibilityBackup,
		BackupTarget:  ".config/ghostty/config",
		Observation:   Observation{Exists: true, Source: ".config/ghostty/config", Digest: strings.Repeat("b", 64), Managed: true},
		Observations:  []Observation{{Exists: true, Source: ".config/ghostty/config", Digest: strings.Repeat("b", 64), Managed: true}},
	}
}

func validInstallAction(t *testing.T) Action {
	t.Helper()
	detected := false
	recipe := InstallRecipe{
		SchemaVersion: CurrentInstallRecipeSchemaVersion,
		ToolID:        "codex",
		Platform:      "macos",
		Manager:       "brew",
		Steps: []InstallStep{
			{Kind: InstallStepPackageManager, Provider: "brew", Packages: []string{"node"}},
			{Kind: InstallStepNPMGlobal, Provider: "npm", Args: []string{"install", "-g", "@openai/codex"}},
		},
		Detector: InstallDetector{Kind: InstallDetectorBinary, Values: []string{"codex"}},
		Risk:     "downloads package lifecycle code",
	}
	digest, err := InstallRecipeDigest(recipe)
	if err != nil {
		t.Fatal(err)
	}
	return Action{
		ID: "install:codex", Kind: KindInstallTool, ToolID: "codex", Target: "codex",
		Description: "install Codex", Disposition: DispositionApply, DesiredDigest: digest,
		Ownership: OwnershipPackageManager, Reversibility: ReversibilityManual,
		InstallRecipe: &recipe, InstallDetected: &detected,
	}
}

func TestInstallRecipeIsPublicImmutableAndDigestBound(t *testing.T) {
	action := validInstallAction(t)
	plan, err := NewPlan(time.Now(), []Action{action})
	if err != nil {
		t.Fatal(err)
	}
	action.InstallRecipe.Steps[1].Args[2] = "malicious-package"
	copyOne := plan.Actions()
	copyOne[0].InstallRecipe.Steps[0].Packages[0] = "malicious-package"
	if got := plan.Actions()[0].InstallRecipe.Steps[1].Args[2]; got != "@openai/codex" {
		t.Fatalf("accepted recipe was mutable: %q", got)
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"install_recipe", "@openai/codex", "downloads package lifecycle code", "install_detected"} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("public plan omitted %q: %s", want, encoded)
		}
	}

	tampered := validInstallAction(t)
	tampered.InstallRecipe.Steps[1].Args[2] = "malicious-package"
	if _, err := NewPlan(time.Now(), []Action{tampered}); !errors.Is(err, ErrInvalidPlan) || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("tampered recipe error = %v", err)
	}
}

func TestManagedSetOwnershipIsValidatedHashedAndImmutable(t *testing.T) {
	action := validConfigAction()
	action.Target = "btop.conf, theme"
	action.Ownership = OwnershipManagedSet
	action.BackupTarget = ""
	action.BackupTargets = []string{"btop.conf", "theme"}
	action.Observations = []Observation{{Source: "btop.conf", Exists: true, Digest: strings.Repeat("c", 64)}, {Source: "theme"}}
	action.TargetOwnership = map[string]Ownership{"btop.conf": OwnershipManagedFragment, "theme": OwnershipManagedFile}
	plan, err := NewPlan(time.Now(), []Action{action})
	if err != nil {
		t.Fatal(err)
	}
	hash := plan.Hash()
	action.TargetOwnership["theme"] = OwnershipManagedFragment
	copyOne := plan.Actions()
	copyOne[0].TargetOwnership["theme"] = OwnershipManagedFragment
	if got := plan.Actions()[0].TargetOwnership["theme"]; got != OwnershipManagedFile || plan.Hash() != hash {
		t.Fatalf("managed-set ownership was mutable: %s hash=%s", got, plan.Hash())
	}
	changed := plan.Actions()[0]
	changed.TargetOwnership["theme"] = OwnershipManagedFragment
	changed.TargetOwnership["btop.conf"] = OwnershipManagedFile
	changedPlan, err := NewPlan(plan.CreatedAt(), []Action{changed})
	if err != nil || changedPlan.Hash() == hash {
		t.Fatalf("target ownership did not affect plan hash: %v", err)
	}
}

func TestManagedSetOwnershipRejectsIncompleteOrMisclassifiedMaps(t *testing.T) {
	base := validConfigAction()
	base.Ownership = OwnershipManagedSet
	base.BackupTarget = ""
	base.BackupTargets = []string{"one", "two"}
	base.Observations = []Observation{{Source: "one"}, {Source: "two"}}
	for name, ownership := range map[string]map[string]Ownership{
		"missing target": {"one": OwnershipManagedFragment},
		"unknown target": {"one": OwnershipManagedFragment, "other": OwnershipManagedFile},
		"invalid child":  {"one": OwnershipManagedFragment, "two": OwnershipUser},
	} {
		t.Run(name, func(t *testing.T) {
			action := base
			action.TargetOwnership = ownership
			if _, err := NewPlan(time.Now(), []Action{action}); !errors.Is(err, ErrInvalidPlan) {
				t.Fatalf("managed set error = %v", err)
			}
		})
	}
	for name, mutate := range map[string]func(*Action){
		"homogeneous set": func(action *Action) {
			action.TargetOwnership = map[string]Ownership{"one": OwnershipManagedFile, "two": OwnershipManagedFile}
		},
		"wrong kind":     func(action *Action) { action.Kind = KindUpdateState },
		"missing backup": func(action *Action) { action.BackupTargets = []string{"one"} },
	} {
		t.Run(name, func(t *testing.T) {
			action := base
			action.TargetOwnership = map[string]Ownership{"one": OwnershipManagedFragment, "two": OwnershipManagedFile}
			mutate(&action)
			if _, err := NewPlan(time.Now(), []Action{action}); !errors.Is(err, ErrInvalidPlan) {
				t.Fatalf("managed set error = %v", err)
			}
		})
	}
}

func TestInstallRecipeRejectsGenericOrAmbiguousExecution(t *testing.T) {
	for _, mutate := range []func(*Action){
		func(a *Action) { a.InstallRecipe.Steps[0].Kind = "shell" },
		func(a *Action) { a.InstallRecipe.Steps[0].Provider = "apt" },
		func(a *Action) { a.InstallRecipe.Detector.Kind = "path" },
		func(a *Action) { a.InstallRecipe.Risk = "unsafe\nsecret" },
		func(a *Action) { a.InstallRecipe.Risk = "unsafe\tspoof" },
		func(a *Action) { a.InstallRecipe.Risk = "unsafe\u202Espoof" },
		func(a *Action) { a.InstallRecipe = nil },
		func(a *Action) { a.InstallDetected = nil },
	} {
		action := validInstallAction(t)
		mutate(&action)
		if _, err := NewPlan(time.Now(), []Action{action}); !errors.Is(err, ErrInvalidPlan) {
			t.Fatalf("unsafe recipe accepted: %#v err=%v", action, err)
		}
	}
}

func TestHomebrewCaskRecipeIsNarrowAndDigestBound(t *testing.T) {
	detected := false
	recipe := InstallRecipe{
		SchemaVersion: CurrentInstallRecipeSchemaVersion, ToolID: "t3-code", Platform: "macos", Manager: "brew",
		Steps:    []InstallStep{{Kind: InstallStepHomebrewCask, Provider: "brew", Casks: []string{"t3-code"}}},
		Detector: InstallDetector{Kind: InstallDetectorAppBundle, Values: []string{"T3 Code.app"}}, Risk: "reviewed cask",
	}
	digest, err := InstallRecipeDigest(recipe)
	if err != nil {
		t.Fatal(err)
	}
	action := Action{ID: "install:t3-code", Kind: KindInstallTool, ToolID: "t3-code", Target: "t3-code", Description: "install T3 Code", Disposition: DispositionApply, DesiredDigest: digest, Ownership: OwnershipPackageManager, Reversibility: ReversibilityManual, InstallRecipe: &recipe, InstallDetected: &detected}
	if _, err := NewPlan(time.Now(), []Action{action}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "--formula", "../t3", "tap/t3", "t3 code", "t3\tcode", "t3\u202Ecode"} {
		candidate := CloneInstallRecipe(recipe)
		candidate.Steps[0].Casks = []string{bad}
		if _, err := InstallRecipeDigest(candidate); err == nil {
			t.Errorf("unsafe cask token %q was accepted", bad)
		}
	}
	for _, mutate := range []func(*InstallRecipe){
		func(r *InstallRecipe) { r.Manager = "apt" },
		func(r *InstallRecipe) { r.Platform = "debian" },
		func(r *InstallRecipe) { r.Steps[0].Provider = "apt" },
		func(r *InstallRecipe) { r.Steps[0].Packages = []string{"t3-code"} },
		func(r *InstallRecipe) { r.Steps[0].Args = []string{"--formula"} },
	} {
		candidate := CloneInstallRecipe(recipe)
		mutate(&candidate)
		if _, err := InstallRecipeDigest(candidate); err == nil {
			t.Errorf("ambiguous cask recipe accepted: %#v", candidate)
		}
	}
}

func TestPlanIsImmutableAndHashIdentifiesExactDocument(t *testing.T) {
	created := time.Date(2026, 7, 10, 12, 30, 0, 0, time.FixedZone("offset", -7*60*60))
	actions := []Action{validConfigAction()}
	actions[0].BackupTargets = []string{".config/ghostty/config"}
	actions[0].BackupTarget = ""
	plan, err := NewPlan(created, actions)
	if err != nil {
		t.Fatal(err)
	}

	actions[0].Target = "mutated by caller"
	actions[0].BackupTargets[0] = "mutated backup"
	actions[0].Observations[0].Source = "mutated observation"
	copyOne := plan.Actions()
	copyOne[0].Target = "mutated copy"
	copyOne[0].BackupTargets[0] = "mutated copy backup"
	copyOne[0].Observations[0].Source = "mutated copy observation"
	if got := plan.Actions()[0].Target; got != ".config/ghostty/config" {
		t.Fatalf("plan actions were mutable: %q", got)
	}
	if got := plan.Actions()[0].BackupTargets[0]; got != ".config/ghostty/config" {
		t.Fatalf("plan nested backup targets were mutable: %q", got)
	}
	if got := plan.Actions()[0].Observations[0].Source; got != ".config/ghostty/config" {
		t.Fatalf("plan nested observations were mutable: %q", got)
	}
	if len(plan.Hash()) != 64 {
		t.Fatalf("plan hash length = %d, want 64", len(plan.Hash()))
	}

	sameAction := validConfigAction()
	sameAction.BackupTargets = []string{".config/ghostty/config"}
	sameAction.BackupTarget = ""
	same, err := NewPlan(created, []Action{sameAction})
	if err != nil {
		t.Fatal(err)
	}
	if same.Hash() != plan.Hash() {
		t.Fatalf("equal canonical plans have different hashes: %s != %s", same.Hash(), plan.Hash())
	}
	later, err := NewPlan(created.Add(time.Second), []Action{sameAction})
	if err != nil {
		t.Fatal(err)
	}
	if later.Hash() != plan.Hash() {
		t.Fatal("created_at metadata changed deterministic plan hash")
	}
}

func TestPlanRejectsUnsafeConfigApply(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Action)
		want   string
	}{
		{"unknown ownership", func(a *Action) { a.Ownership = OwnershipUnknown }, "managed ownership"},
		{"user ownership", func(a *Action) { a.Ownership = OwnershipUser }, "managed ownership"},
		{"missing backup", func(a *Action) { a.BackupTarget = "" }, "backup target"},
		{"unsafe backup", func(a *Action) { a.BackupTarget = "../escape" }, "unsafe backup target"},
		{"missing observations", func(a *Action) { a.Observations = nil }, "per-target observations"},
		{"non-hex desired digest", func(a *Action) { a.DesiredDigest = strings.Repeat("z", 64) }, "desired digest"},
		{"duplicate id", nil, "duplicate action id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			action := validConfigAction()
			if tc.mutate != nil {
				tc.mutate(&action)
			}
			actions := []Action{action}
			if tc.name == "duplicate id" {
				actions = append(actions, action)
			}
			_, err := NewPlan(time.Now(), actions)
			if !errors.Is(err, ErrInvalidPlan) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewPlan error = %v, want ErrInvalidPlan containing %q", err, tc.want)
			}
		})
	}
}

func TestBlockedActionIsVisibleButNotAConfigMutation(t *testing.T) {
	action := validConfigAction()
	action.Disposition = DispositionBlocked
	action.Ownership = OwnershipUser
	action.BackupTarget = ""
	action.Reason = "existing file is not managed"
	plan, err := NewPlan(time.Now(), []Action{action})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasBlocked() {
		t.Fatal("blocked action was not reported")
	}
	if got := plan.BackupTargets(); len(got) != 0 {
		t.Fatalf("blocked action leaked into backup targets: %v", got)
	}
}

func TestBackupTargetsAreApplyOnlyUniqueAndSorted(t *testing.T) {
	first := validConfigAction()
	first.BackupTarget = ".config/z-last"
	first.Observations[0].Source = ".config/z-last"
	second := first
	second.ID = "config:second"
	second.BackupTarget = ".config/a-first"
	second.Observations = []Observation{{Exists: true, Source: ".config/a-first", Digest: strings.Repeat("b", 64), Managed: true}}
	third := first
	third.ID = "config:skip"
	third.Disposition = DispositionSkip
	third.Reason = "unchanged"
	plan, err := NewPlan(time.Now(), []Action{first, second, third})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".config/a-first", ".config/z-last"}
	if got := plan.BackupTargets(); !reflect.DeepEqual(got, want) {
		t.Fatalf("BackupTargets = %v, want %v", got, want)
	}
}

func TestPlanJSONCarriesHashAndNoRawConfig(t *testing.T) {
	plan, err := NewPlan(time.Now(), []Action{validConfigAction()})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["hash"] != plan.Hash() || decoded["schema_version"] != float64(CurrentPlanSchemaVersion) {
		t.Fatalf("plan JSON missing identity: %s", data)
	}
	if strings.Contains(string(data), "secret") {
		t.Fatalf("plan JSON unexpectedly contains raw secret material: %s", data)
	}
}
