package operation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestStatePlanAuthorityDigestBindsExactPrivateAuthority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	plan, err := CaptureStatePlan()
	if err != nil {
		t.Fatal(err)
	}
	first, err := StatePlanAuthorityDigest(plan)
	if err != nil || len(first) != 64 {
		t.Fatalf("state digest=%q err=%v", first, err)
	}
	again, err := CaptureStatePlan()
	if err != nil {
		t.Fatal(err)
	}
	second, err := StatePlanAuthorityDigest(again)
	if err != nil || second != first {
		t.Fatalf("stable state digest=%q/%q err=%v", first, second, err)
	}
	if err := os.Mkdir(filepath.Join(home, ".local"), 0o700); err != nil {
		t.Fatal(err)
	}
	changed, err := CaptureStatePlan()
	if err != nil {
		t.Fatal(err)
	}
	changedDigest, err := StatePlanAuthorityDigest(changed)
	if err != nil || changedDigest == first {
		t.Fatalf("changed namespace digest=%q original=%q err=%v", changedDigest, first, err)
	}
	otherRoot := *plan
	otherRoot.root = filepath.Join(filepath.Dir(plan.root), "other-root")
	otherDigest, err := StatePlanAuthorityDigest(&otherRoot)
	if err != nil || otherDigest == first {
		t.Fatalf("exact root digest=%q original=%q err=%v", otherDigest, first, err)
	}
}

func TestStatePlanAuthorityDigestRejectsNilZeroAndMalformed(t *testing.T) {
	for name, plan := range map[string]*StatePlan{
		"nil":           nil,
		"zero":          {},
		"relative root": {root: "relative", stateRel: "state", entries: []statePlanEntry{{rel: "state"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if digest, err := StatePlanAuthorityDigest(plan); err == nil || digest != "" {
				t.Fatalf("malformed state digest=%q err=%v", digest, err)
			}
		})
	}
}

func TestStatePlanAuthorityDigestRejectsMalformedEntryRelationships(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	base, err := CaptureStatePlan()
	if err != nil {
		t.Fatal(err)
	}
	clone := func() *StatePlan {
		copy := *base
		copy.entries = append([]statePlanEntry(nil), base.entries...)
		return &copy
	}
	malformed := map[string]func(*StatePlan){
		"duplicate":           func(plan *StatePlan) { plan.entries[1] = plan.entries[0] },
		"out of order":        func(plan *StatePlan) { plan.entries[0], plan.entries[1] = plan.entries[1], plan.entries[0] },
		"rel mismatch":        func(plan *StatePlan) { plan.entries[0].rel = "wrong" },
		"exists without leaf": func(plan *StatePlan) { plan.entries[0].exists = true },
		"missing with leaf":   func(plan *StatePlan) { plan.entries[0].leaf = &safefile.DirectorySnapshot{} },
		"untracked parent":    func(plan *StatePlan) { plan.entries[0].parents = nil },
		"dot state rel":       func(plan *StatePlan) { plan.stateRel = "." },
		"parent state rel":    func(plan *StatePlan) { plan.stateRel = ".." },
		"escaping state rel":  func(plan *StatePlan) { plan.stateRel = "../escape" },
	}
	for name, mutate := range malformed {
		t.Run(name, func(t *testing.T) {
			plan := clone()
			mutate(plan)
			if digest, err := StatePlanAuthorityDigest(plan); err == nil || digest != "" {
				t.Fatalf("malformed state digest=%q err=%v", digest, err)
			}
		})
	}
	if err := os.MkdirAll(filepath.Join(home, ".local", "state", "dotfiles"), 0o700); err != nil {
		t.Fatal(err)
	}
	existing, err := CaptureStatePlan()
	if err != nil {
		t.Fatal(err)
	}
	untrackedLeaf := *existing
	untrackedLeaf.entries = append([]statePlanEntry(nil), existing.entries...)
	for index := range untrackedLeaf.entries {
		if untrackedLeaf.entries[index].exists {
			untrackedLeaf.entries[index].leaf = &safefile.DirectorySnapshot{}
			break
		}
	}
	if digest, err := StatePlanAuthorityDigest(&untrackedLeaf); err == nil || digest != "" {
		t.Fatalf("untracked leaf digest=%q err=%v", digest, err)
	}
	if _, err := BootstrapStateNamespaceTracked(existing); err != nil {
		t.Fatal(err)
	}
	complete, err := CaptureStatePlan()
	if err != nil {
		t.Fatal(err)
	}

	transplant := func(targetRel string) *StatePlan {
		plan := clone()
		for index := range plan.entries {
			if plan.entries[index].rel != targetRel {
				continue
			}
			for _, candidate := range complete.entries {
				if candidate.rel == targetRel {
					plan.entries[index] = candidate
					return plan
				}
			}
		}
		t.Fatalf("state fixture missing %q", targetRel)
		return nil
	}
	for _, targetRel := range []string{filepath.ToSlash(filepath.Join(base.stateRel, "operations")), filepath.ToSlash(filepath.Join(base.stateRel, ".."))} {
		plan := transplant(targetRel)
		if digest, err := StatePlanAuthorityDigest(plan); err == nil || digest != "" {
			t.Fatalf("existing %q below missing prefix digest=%q err=%v", targetRel, digest, err)
		}
	}
}
