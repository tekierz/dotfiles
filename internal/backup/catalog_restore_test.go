package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type catalogRestoreObservation struct {
	source      string
	target      string
	kind        TargetKind
	existed     bool
	desiredMode os.FileMode
}

func TestListCatalogRetainsCompleteParseOnlyRestoreSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustCatalogMkdir(t, filepath.Join(home, ".config", "tree"), 0o710)
	mustCatalogWrite(t, filepath.Join(home, ".config", "tool", "config"), "original file\n", 0o640)
	mustCatalogWrite(t, filepath.Join(home, ".config", "tree", "value"), "original tree\n", 0o604)
	if err := os.Chmod(filepath.Join(home, ".config", "tree"), 0o710); err != nil {
		t.Fatal(err)
	}

	backups := filepath.Join(t.TempDir(), "backups")
	selected := filepath.Join(backups, "selected")
	targets := []Target{
		{RelPath: ".config/tool/config", Kind: TargetFile},
		{RelPath: ".config/tree", Kind: TargetDirectory},
		{RelPath: ".config/new-file", Kind: TargetFile},
		{RelPath: ".config/new-directory", Kind: TargetDirectory},
	}
	if _, err := CreatePlan(home, selected, targets); err != nil {
		t.Fatal(err)
	}

	mustCatalogWrite(t, filepath.Join(home, ".config", "tool", "config"), "live file\n", 0o600)
	mustCatalogWrite(t, filepath.Join(home, ".config", "tree", "value"), "live tree\n", 0o600)
	mustCatalogWrite(t, filepath.Join(home, ".config", "new-file"), "created later\n", 0o600)
	mustCatalogWrite(t, filepath.Join(home, ".config", "new-directory", "marker"), "created later\n", 0o600)

	entries, err := ListCatalog(backups)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ListCatalog entries=%+v err=%v", entries, err)
	}
	want := []catalogRestoreObservation{
		{source: ".config/tool/config", target: ".config/tool/config", kind: TargetFile, existed: true, desiredMode: 0o640},
		{source: ".config/tree", target: ".config/tree", kind: TargetDirectory, existed: true, desiredMode: 0o710},
		{target: ".config/new-file", kind: TargetFile, desiredMode: 0o600},
		{target: ".config/new-directory", kind: TargetDirectory, desiredMode: 0o700},
	}
	got := observeCatalogRestoreSnapshot(t, entries[0])
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog restore snapshot = %+v, want %+v", got, want)
	}
	assertCatalogContent(t, filepath.Join(home, ".config", "tool", "config"), "live file\n")
	assertCatalogContent(t, filepath.Join(home, ".config", "tree", "value"), "live tree\n")
	assertCatalogContent(t, filepath.Join(home, ".config", "new-file"), "created later\n")
	assertCatalogContent(t, filepath.Join(home, ".config", "new-directory", "marker"), "created later\n")

	if err := os.WriteFile(filepath.Join(selected, ManifestName), []byte("poisoned after parse\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries[0].Name = "mutable display"
	entries[0].Path = filepath.Join(t.TempDir(), "redirect")
	if after := observeCatalogRestoreSnapshot(t, entries[0]); !reflect.DeepEqual(after, want) {
		t.Fatalf("retained restore snapshot changed through disk/display mutation: %+v", after)
	}
}

func TestListCatalogRejectsInvalidRestoreManifestBeforeMutation(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*testing.T, string)
		manifest func(home, selected string) string
	}{
		{
			name: "missing source identifier",
			manifest: func(home, selected string) string {
				return fmt.Sprintf("%s||yes|file|600\n", filepath.Join(home, "config"))
			},
		},
		{
			name: "absent target has source identifier",
			manifest: func(home, selected string) string {
				return fmt.Sprintf("%s|%s|no|file|600\n", filepath.Join(home, "config"), filepath.Join(selected, "config"))
			},
		},
		{
			name: "invalid target kind",
			manifest: func(home, selected string) string {
				return fmt.Sprintf("%s|%s|yes|socket|600\n", filepath.Join(home, "config"), filepath.Join(selected, "config"))
			},
		},
		{
			name: "invalid existence marker",
			manifest: func(home, selected string) string {
				return fmt.Sprintf("%s|%s|maybe|file|600\n", filepath.Join(home, "config"), filepath.Join(selected, "config"))
			},
		},
		{
			name: "invalid desired mode",
			manifest: func(home, selected string) string {
				return fmt.Sprintf("%s|%s|yes|file|not-octal\n", filepath.Join(home, "config"), filepath.Join(selected, "config"))
			},
		},
		{
			name: "absent target outside home",
			manifest: func(_, _ string) string {
				return fmt.Sprintf("%s||no|file|600\n", filepath.Join(t.TempDir(), "outside"))
			},
		},
		{
			name: "duplicate target",
			prepare: func(t *testing.T, selected string) {
				mustCatalogWrite(t, filepath.Join(selected, "config"), "source\n", 0o600)
			},
			manifest: func(home, selected string) string {
				line := fmt.Sprintf("%s|%s|yes|file|600\n", filepath.Join(home, "config"), filepath.Join(selected, "config"))
				return line + line
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			mustCatalogWrite(t, filepath.Join(home, "config"), "live sentinel\n", 0o600)
			backups := filepath.Join(t.TempDir(), "backups")
			selected := filepath.Join(backups, "selected")
			mustCatalogMkdir(t, selected, 0o700)
			if tt.prepare != nil {
				tt.prepare(t, selected)
			}
			mustCatalogWrite(t, filepath.Join(selected, ManifestName), tt.manifest(home, selected), 0o600)

			entries, err := ListCatalog(backups)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("invalid catalog became visible: %+v", entries)
			}
			assertCatalogContent(t, filepath.Join(home, "config"), "live sentinel\n")
		})
	}
}

func observeCatalogRestoreSnapshot(t *testing.T, entry CatalogEntry) []catalogRestoreObservation {
	t.Helper()
	if entry.authority == nil {
		t.Fatal("catalog authority is nil")
	}
	authority := reflect.ValueOf(entry.authority).Elem()
	restore := authority.FieldByName("restore")
	if !restore.IsValid() || restore.Kind() != reflect.Pointer || restore.IsNil() {
		t.Fatal("catalog authority did not retain a parse-only restore snapshot")
	}
	snapshot := restore.Elem()
	source := snapshot.FieldByName("source")
	if !source.IsValid() || source.Kind() != reflect.Pointer || source.IsNil() {
		t.Fatal("catalog restore snapshot did not retain immutable source authority")
	}
	items := snapshot.FieldByName("items")
	if !items.IsValid() || items.Kind() != reflect.Slice {
		t.Fatal("catalog restore snapshot did not retain typed items")
	}
	result := make([]catalogRestoreObservation, items.Len())
	for index := range result {
		item := items.Index(index)
		result[index] = catalogRestoreObservation{
			source:      item.FieldByName("source").String(),
			target:      item.FieldByName("target").String(),
			kind:        TargetKind(item.FieldByName("kind").String()),
			existed:     item.FieldByName("existed").Bool(),
			desiredMode: os.FileMode(item.FieldByName("desiredMode").Uint()),
		}
	}
	return result
}

func mustCatalogMkdir(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(path, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mustCatalogWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	mustCatalogMkdir(t, filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func assertCatalogContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("%s content = %q, err=%v; want %q", path, got, err, want)
	}
}
