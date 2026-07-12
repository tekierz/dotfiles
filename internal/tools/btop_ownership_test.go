package tools

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBtopConfigFailsClosedOnUnmarkedLegacyTheme(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	themePath := filepath.Join(home, ".config", "btop", "themes", "dracula.theme")
	if err := os.MkdirAll(filepath.Dir(themePath), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte("theme[main_bg]=\"#282a36\"\n")
	if err := os.WriteFile(themePath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	err := WriteBtopConfig(BtopConfig{}, "dracula")
	if !errors.Is(err, ErrUnmanagedConfig) || !strings.Contains(err.Error(), "require explicit adoption") {
		t.Fatalf("WriteBtopConfig error = %v, want explicit fail-closed legacy limitation", err)
	}
	if got := mustReadOwnershipFile(t, themePath); !bytes.Equal(got, legacy) {
		t.Fatalf("unmarked legacy btop theme changed: %q", got)
	}
	assertOwnershipPathAbsent(t, filepath.Join(home, ".config", "btop", "btop.conf"))
}

func TestWriteBtopConfigAdoptsNativeConfigWithoutReplacingItsBytes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	themePath := filepath.Join(home, ".config", "btop", "themes", "dracula.theme")
	configPath := filepath.Join(home, ".config", "btop", "btop.conf")
	if err := os.MkdirAll(filepath.Dir(themePath), 0o700); err != nil {
		t.Fatal(err)
	}
	originalTheme := []byte(GenerateBtopTheme("nord"))
	if err := os.WriteFile(themePath, originalTheme, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("# user-owned btop config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteBtopConfig(BtopConfig{}, "dracula"); err != nil {
		t.Fatalf("WriteBtopConfig error = %v", err)
	}
	if got := mustReadOwnershipFile(t, themePath); bytes.Equal(got, originalTheme) {
		t.Fatal("managed theme did not update")
	}
	got := mustReadOwnershipFile(t, configPath)
	if !bytes.HasPrefix(got, []byte("# user-owned btop config\n")) || bytes.Count(got, []byte(btopManagedStart)) != 1 {
		t.Fatalf("native config was not preserved during fragment adoption: %q", got)
	}
}
