//go:build darwin || linux

package safefile

import (
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDirectoryAuthorityBindsRootAndRecursiveContents(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "tree"), 0o700)
	mustWrite(t, filepath.Join(root, "tree", "value"), "accepted", 0o600)
	snapshot, err := SnapshotDirectoryWithin(root, "tree")
	if err != nil {
		t.Fatal(err)
	}
	authority, err := CompactDirectoryAuthority(snapshot)
	if err != nil || !authority.Tracked() || !authority.Matches(snapshot) || (DirectoryAuthority{}).Matches(snapshot) {
		t.Fatalf("compact authority did not bind snapshot: %v", err)
	}
	for name, mutate := range map[string]func(*DirectorySnapshot){
		"device":  func(s *DirectorySnapshot) { s.device++ },
		"inode":   func(s *DirectorySnapshot) { s.inode++ },
		"uid":     func(s *DirectorySnapshot) { s.uid++ },
		"gid":     func(s *DirectorySnapshot) { s.gid++ },
		"mode":    func(s *DirectorySnapshot) { s.root.mode = 0o750 },
		"name":    func(s *DirectorySnapshot) { s.root.entries[0].name = "different" },
		"content": func(s *DirectorySnapshot) { s.root.entries[0].data = []byte("changed") },
		"file-mode": func(s *DirectorySnapshot) {
			s.root.entries[0].mode = 0o640
		},
		"type": func(s *DirectorySnapshot) {
			s.root.entries[0].data = nil
			s.root.entries[0].dir = &directorySnapshotNode{mode: s.root.entries[0].mode}
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := *snapshot
			changed.root = cloneDirectorySnapshotNode(snapshot.root)
			mutate(&changed)
			changed.digest = newDirectorySnapshot(changed.root).digest
			if _, err := CompactDirectoryAuthority(&changed); err != nil {
				t.Fatalf("comparison fixture invalid: %v", err)
			}
			if authority.Matches(&changed) {
				t.Fatal("changed snapshot matched accepted authority")
			}
		})
	}
	for name, mutate := range map[string]func(*DirectorySnapshot){
		"untracked":       func(s *DirectorySnapshot) { s.tracked = false },
		"root-only":       func(s *DirectorySnapshot) { s.rootOnly = true },
		"zero-inode":      func(s *DirectorySnapshot) { s.inode = 0 },
		"zero-digest":     func(s *DirectorySnapshot) { s.digest = [32]byte{} },
		"wrong-digest":    func(s *DirectorySnapshot) { s.digest[0] ^= 1 },
		"non-permissions": func(s *DirectorySnapshot) { s.root.mode |= fs.ModeDir },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := *snapshot
			mutate(&invalid)
			if got, err := CompactDirectoryAuthority(&invalid); !errors.Is(err, ErrInvalidAuthority) || got.Tracked() || authority.Matches(&invalid) {
				t.Fatalf("invalid snapshot accepted: %v", err)
			}
		})
	}
	if got, err := CompactDirectoryAuthority(nil); !errors.Is(err, ErrInvalidAuthority) || got.Tracked() || authority.Matches(nil) {
		t.Fatalf("nil snapshot accepted: %v", err)
	}
	var inspect func(reflect.Type)
	inspect = func(typ reflect.Type) {
		switch typ.Kind() { //nolint:exhaustive // All non-scalar representations are rejected.
		case reflect.Struct:
			for i := range typ.NumField() {
				inspect(typ.Field(i).Type)
			}
		case reflect.Array:
			inspect(typ.Elem())
		case reflect.Bool, reflect.Uint8, reflect.Uint32, reflect.Uint64:
		default:
			t.Errorf("compact authority retains a reference-bearing field of type %v", typ)
		}
	}
	inspect(reflect.TypeFor[DirectoryAuthority]())
}
