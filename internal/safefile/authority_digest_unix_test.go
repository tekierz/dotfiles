//go:build darwin || linux

package safefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateAuthorityDigestsAreStableDistinctAndPreservePublicDigestSemantics(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "parent"), 0o700)
	parents, err := CaptureParentChainWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	first, err := ParentChainAuthorityDigest(parents)
	if err != nil || len(first) != 64 {
		t.Fatalf("parent digest=%q err=%v", first, err)
	}
	again, err := CaptureParentChainWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParentChainAuthorityDigest(again)
	if err != nil || second != first {
		t.Fatalf("stable parent digest=%q/%q err=%v", first, second, err)
	}
	otherTarget, err := CaptureParentChainWithin(root, "parent/other/config")
	if err != nil {
		t.Fatal(err)
	}
	otherDigest, err := ParentChainAuthorityDigest(otherTarget)
	if err != nil || otherDigest == first {
		t.Fatalf("distinct parent shape digest=%q first=%q err=%v", otherDigest, first, err)
	}

	rootOnly, _, err := CaptureDirectoryRootWithin(root, "parent")
	if err != nil {
		t.Fatal(err)
	}
	if rootOnly.Digest() != ([32]byte{}) {
		t.Fatal("root-only snapshot changed recursive Digest semantics")
	}
	rootDigest, err := DirectorySnapshotAuthorityDigest(rootOnly)
	if err != nil || len(rootDigest) != 64 {
		t.Fatalf("root-only authority digest=%q err=%v", rootDigest, err)
	}
	if err := os.Rename(filepath.Join(root, "parent"), filepath.Join(root, "parent-old")); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "parent"), 0o700)
	replacement, _, err := CaptureDirectoryRootWithin(root, "parent")
	if err != nil {
		t.Fatal(err)
	}
	replacementDigest, err := DirectorySnapshotAuthorityDigest(replacement)
	if err != nil || replacementDigest == rootDigest {
		t.Fatalf("replacement root digest=%q original=%q err=%v", replacementDigest, rootDigest, err)
	}
}

func TestPrivateAuthorityDigestsRejectNilUntrackedAndMalformedValues(t *testing.T) {
	for name, chain := range map[string]*ParentChain{
		"nil":          nil,
		"untracked":    {},
		"missing root": {tracked: true, entries: []parentChainEntry{{rel: ""}}},
		"noncanonical path": {tracked: true, entries: []parentChainEntry{
			{exists: true, device: 1, inode: 1, mode: 0o700},
			{rel: "parent/../escape", exists: false},
		}},
	} {
		t.Run("parent "+name, func(t *testing.T) {
			if digest, err := ParentChainAuthorityDigest(chain); err == nil || digest != "" {
				t.Fatalf("malformed parent digest=%q err=%v", digest, err)
			}
		})
	}
	for name, snapshot := range map[string]*DirectorySnapshot{
		"nil":                      nil,
		"untracked":                {},
		"missing identity":         {tracked: true, rootOnly: true, root: directorySnapshotNode{mode: 0o700}},
		"recursive missing digest": {tracked: true, device: 1, inode: 1, root: directorySnapshotNode{mode: 0o700}},
	} {
		t.Run("directory "+name, func(t *testing.T) {
			if digest, err := DirectorySnapshotAuthorityDigest(snapshot); err == nil || digest != "" {
				t.Fatalf("malformed directory digest=%q err=%v", digest, err)
			}
		})
	}
}

func TestParentChainAuthorityDigestBindsEveryFieldAndCanonicalOrder(t *testing.T) {
	base := &ParentChain{tracked: true, entries: []parentChainEntry{
		{rel: "", exists: true, device: 1, inode: 2, mode: 0o700, uid: 3, gid: 4},
		{rel: "parent", exists: true, device: 5, inode: 6, mode: 0o750, uid: 7, gid: 8},
		{rel: "parent/child", exists: false},
	}}
	baseline, err := ParentChainAuthorityDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*ParentChain){
		"rel": func(value *ParentChain) { value.entries[1].rel = "other"; value.entries[2].rel = "other/child" },
		"exists": func(value *ParentChain) {
			value.entries[2] = parentChainEntry{rel: "parent/child", exists: true, device: 9, inode: 10, mode: 0o700, uid: 11, gid: 12}
		},
		"device": func(value *ParentChain) { value.entries[1].device++ },
		"inode":  func(value *ParentChain) { value.entries[1].inode++ },
		"mode":   func(value *ParentChain) { value.entries[1].mode = 0o700 },
		"uid":    func(value *ParentChain) { value.entries[1].uid++ },
		"gid":    func(value *ParentChain) { value.entries[1].gid++ },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := &ParentChain{tracked: true, entries: append([]parentChainEntry(nil), base.entries...)}
			mutate(changed)
			digest, err := ParentChainAuthorityDigest(changed)
			if err != nil || digest == baseline {
				t.Fatalf("%s digest=%q baseline=%q err=%v", name, digest, baseline, err)
			}
		})
	}
	wrongOrder := &ParentChain{tracked: true, entries: append([]parentChainEntry(nil), base.entries...)}
	wrongOrder.entries[1], wrongOrder.entries[2] = wrongOrder.entries[2], wrongOrder.entries[1]
	if digest, err := ParentChainAuthorityDigest(wrongOrder); err == nil || digest != "" {
		t.Fatalf("out-of-order digest=%q err=%v", digest, err)
	}
}

func TestDirectorySnapshotAuthorityDigestBindsEveryRootFieldAndRecursiveDigest(t *testing.T) {
	base := &DirectorySnapshot{tracked: true, rootOnly: true, device: 1, inode: 2, uid: 3, gid: 4, root: directorySnapshotNode{mode: 0o700}}
	baseline, err := DirectorySnapshotAuthorityDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*DirectorySnapshot){
		"device": func(value *DirectorySnapshot) { value.device++ },
		"inode":  func(value *DirectorySnapshot) { value.inode++ },
		"uid":    func(value *DirectorySnapshot) { value.uid++ },
		"gid":    func(value *DirectorySnapshot) { value.gid++ },
		"mode":   func(value *DirectorySnapshot) { value.root.mode = 0o750 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := *base
			mutate(&changed)
			digest, err := DirectorySnapshotAuthorityDigest(&changed)
			if err != nil || digest == baseline {
				t.Fatalf("%s digest=%q baseline=%q err=%v", name, digest, baseline, err)
			}
		})
	}
	recursiveKind := newDirectorySnapshot(base.root)
	recursiveKind.device, recursiveKind.inode, recursiveKind.uid, recursiveKind.gid = base.device, base.inode, base.uid, base.gid
	recursiveKindDigest, err := DirectorySnapshotAuthorityDigest(recursiveKind)
	if err != nil || recursiveKindDigest == baseline {
		t.Fatalf("rootOnly digest=%q recursive-kind=%q err=%v", baseline, recursiveKindDigest, err)
	}
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "tree"), 0o700)
	mustWrite(t, filepath.Join(root, "tree", "value"), "first", 0o600)
	first, err := SnapshotDirectoryWithin(root, "tree")
	if err != nil {
		t.Fatal(err)
	}
	firstDigest, err := DirectorySnapshotAuthorityDigest(first)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "tree", "value"), "second", 0o600)
	second, err := SnapshotDirectoryWithin(root, "tree")
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := DirectorySnapshotAuthorityDigest(second)
	if err != nil || firstDigest == secondDigest {
		t.Fatalf("recursive digest=%q/%q err=%v", firstDigest, secondDigest, err)
	}
	if rootOnlyDigest, err := DirectorySnapshotAuthorityDigest(base); err != nil || rootOnlyDigest == firstDigest {
		t.Fatalf("rootOnly discriminator=%q recursive=%q err=%v", rootOnlyDigest, firstDigest, err)
	}
}
