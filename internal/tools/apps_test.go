package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasDesktopEntryTokenMatching(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	tests := []struct {
		name  string
		files map[string]string
		names []string
		want  bool
	}{
		{
			name:  "cursor exact desktop",
			files: map[string]string{"cursor.desktop": "[Desktop Entry]\n"},
			names: []string{"cursor"},
			want:  true,
		},
		{
			name:  "cursor dotted desktop token",
			files: map[string]string{"com.cursor.Cursor.desktop": "[Desktop Entry]\n"},
			names: []string{"cursor"},
			want:  true,
		},
		{
			name:  "cursor does not match cursor theme",
			files: map[string]string{"kcm_cursortheme.desktop": "[Desktop Entry]\n"},
			names: []string{"cursor"},
			want:  false,
		},
		{
			name:  "zen browser desktop",
			files: map[string]string{"zen-browser.desktop": "[Desktop Entry]\n"},
			names: []string{"zen-browser", "zen"},
			want:  true,
		},
		{
			name:  "zen does not match zenity",
			files: map[string]string{"org.gnome.Zenity.desktop": "[Desktop Entry]\n"},
			names: []string{"zen"},
			want:  false,
		},
		{
			name: "exec basename token match",
			files: map[string]string{
				"appimagekit_random.desktop": "[Desktop Entry]\nExec=\"/tmp/Zen Browser.AppImage\" %U\n",
			},
			names: []string{"zen"},
			want:  true,
		},
		{
			name: "exec basename rejects substring",
			files: map[string]string{
				"appimagekit_random.desktop": "[Desktop Entry]\nExec=/usr/bin/zenity %U\n",
			},
			names: []string{"zen"},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desktopDir := filepath.Join(tmpHome, ".local", "share", "applications", tt.name)
			if err := os.MkdirAll(desktopDir, 0700); err != nil {
				t.Fatalf("MkdirAll returned error: %v", err)
			}
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(desktopDir, name), []byte(content), 0600); err != nil {
					t.Fatalf("WriteFile returned error: %v", err)
				}
			}

			got := hasDesktopEntryInDirs([]string{desktopDir}, tt.names...)
			if got != tt.want {
				t.Fatalf("hasDesktopEntryInDirs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasAppImageTokenMatching(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	tests := []struct {
		name     string
		fileName string
		patterns []string
		want     bool
	}{
		{
			name:     "cursor appimage token",
			fileName: "Cursor-0.45.11-x86_64.AppImage",
			patterns: []string{"cursor"},
			want:     true,
		},
		{
			name:     "zen browser appimage token",
			fileName: "Zen-Browser-1.0.AppImage",
			patterns: []string{"zen"},
			want:     true,
		},
		{
			name:     "zen rejects frozen substring",
			fileName: "frozen-1.0.AppImage",
			patterns: []string{"zen"},
			want:     false,
		},
		{
			name:     "zen rejects zenith substring",
			fileName: "zenith.AppImage",
			patterns: []string{"zen"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appDir := filepath.Join(tmpHome, "Applications", tt.name)
			if err := os.MkdirAll(appDir, 0700); err != nil {
				t.Fatalf("MkdirAll returned error: %v", err)
			}
			if err := os.WriteFile(filepath.Join(appDir, tt.fileName), []byte{}, 0600); err != nil {
				t.Fatalf("WriteFile returned error: %v", err)
			}

			got := hasAppImageInDirs([]string{appDir}, tt.patterns...)
			if got != tt.want {
				t.Fatalf("hasAppImageInDirs() = %v, want %v", got, tt.want)
			}
		})
	}
}
