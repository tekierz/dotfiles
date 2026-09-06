//go:build darwin || linux

package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func writeCatalogManifest(t *testing.T, root, name, manifest string, payload []byte) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		if err := os.WriteFile(filepath.Join(dir, EncodeName(".zshrc")), payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCatalogDoesNotRetainPayloadSnapshots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "backups")
	for i := range 4 {
		writeCatalogManifest(t, root, fmt.Sprint(i), ".zshrc\t600\n", make([]byte, 4<<20))
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	entries, err := ListCatalog(root)
	if err != nil || len(entries) != 4 {
		t.Fatalf("catalog count=%d error=%v", len(entries), err)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(entries)
	if after.HeapAlloc > before.HeapAlloc && after.HeapAlloc-before.HeapAlloc > 4<<20 {
		t.Errorf("catalog retained %d bytes for 16 MiB payload, want bounded metadata only", after.HeapAlloc-before.HeapAlloc)
	}
	snapshotType := reflect.TypeFor[safefile.DirectorySnapshot]()
	seen := make(map[reflect.Type]bool)
	var inspect func(reflect.Type)
	inspect = func(typ reflect.Type) {
		if seen[typ] {
			return
		}
		seen[typ] = true
		if typ == snapshotType {
			t.Error("catalog authority retains a recursive payload snapshot")
			return
		}
		switch typ.Kind() { //nolint:exhaustive // Only reference containers need traversal.
		case reflect.Pointer, reflect.Slice, reflect.Array:
			inspect(typ.Elem())
		case reflect.Struct:
			for i := range typ.NumField() {
				inspect(typ.Field(i).Type)
			}
		}
	}
	inspect(reflect.TypeFor[catalogAuthority]())
}

func TestCatalogRejectsAggregateMetadataOverflow(t *testing.T) {
	for _, test := range []struct {
		name     string
		accepted int
		manifest string
	}{
		{"items", 4, func() string {
			var manifest strings.Builder
			for i := range 4096 {
				fmt.Fprintf(&manifest, ".config/test/file-%04d\t600\n", i)
			}
			return manifest.String()
		}()},
		{"manifest-bytes", 8, strings.Repeat("#"+strings.Repeat("c", 1022)+"\n", 1023) + ".zshrc\t600\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			root := filepath.Join(home, "backups")
			for i := range test.accepted {
				writeCatalogManifest(t, root, fmt.Sprint(i), test.manifest, nil)
			}
			entries, err := ListCatalog(root)
			if err != nil || len(entries) != test.accepted {
				t.Fatalf("within-budget catalog count=%d error=%v", len(entries), err)
			}
			writeCatalogManifest(t, root, "overflow", test.manifest, nil)
			entries, err = ListCatalog(root)
			if err == nil || entries != nil {
				t.Fatalf("over-budget catalog count=%d error=%v, want explicit error and no partial catalog", len(entries), err)
			}
			for i := range test.accepted {
				if _, err := os.Stat(filepath.Join(root, fmt.Sprint(i), ManifestName)); err != nil {
					t.Fatalf("catalog limit changed existing backup: %v", err)
				}
			}
		})
	}
}
