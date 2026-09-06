package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/backup"
	"github.com/tekierz/dotfiles/internal/config"
)

func TestConvenienceBackupFilesResolveSafeYaziInventory(t *testing.T) {
	tests := []struct {
		name      string
		configure func(t *testing.T, home string)
		wantDir   string
	}{
		{name: "default", configure: func(t *testing.T, _ string) {
			t.Setenv("YAZI_CONFIG_HOME", "")
			t.Setenv("XDG_CONFIG_HOME", "")
		}, wantDir: ".config/yazi"},
		{name: "in-home XDG", configure: func(t *testing.T, home string) {
			t.Setenv("YAZI_CONFIG_HOME", "")
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".xdg"))
		}, wantDir: ".xdg/yazi"},
		{name: "safe override", configure: func(t *testing.T, home string) {
			t.Setenv("YAZI_CONFIG_HOME", filepath.Join(home, "custom", "yazi"))
			t.Setenv("XDG_CONFIG_HOME", "")
		}, wantDir: "custom/yazi"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := withTempHome(t)
			test.configure(t, home)
			files, omission, err := convenienceBackupFiles(home)
			if err != nil {
				t.Fatal(err)
			}
			if omission != "" {
				t.Fatalf("safe inventory reported omission %q", omission)
			}
			for _, base := range defaultBackupFiles {
				if !slices.Contains(files, base) {
					t.Errorf("inventory omitted non-Yazi default %q", base)
				}
			}
			for _, name := range []string{"yazi.toml", "keymap.toml", "theme.toml"} {
				want := filepath.Join(test.wantDir, name)
				if !slices.Contains(files, want) {
					t.Errorf("inventory missing resolved Yazi path %q: %v", want, files)
				}
			}
			seen := map[string]bool{}
			for _, file := range files {
				if filepath.IsAbs(file) || file == ".." || strings.HasPrefix(file, ".."+string(filepath.Separator)) {
					t.Errorf("inventory contains unsafe path %q", file)
				}
				if seen[file] {
					t.Errorf("inventory contains duplicate %q", file)
				}
				seen[file] = true
			}
		})
	}
}

func TestConvenienceBackupFilesOmitUnsafeYaziLocations(t *testing.T) {
	tests := []struct {
		name      string
		configure func(t *testing.T, home string)
	}{
		{name: "external override", configure: func(t *testing.T, _ string) {
			t.Setenv("YAZI_CONFIG_HOME", filepath.Join(t.TempDir(), "external-yazi"))
		}},
		{name: "relative override", configure: func(t *testing.T, _ string) {
			t.Setenv("YAZI_CONFIG_HOME", "relative/yazi")
		}},
		{name: "external XDG", configure: func(t *testing.T, _ string) {
			t.Setenv("YAZI_CONFIG_HOME", "")
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		}},
		{name: "relative XDG", configure: func(t *testing.T, _ string) {
			t.Setenv("YAZI_CONFIG_HOME", "")
			t.Setenv("XDG_CONFIG_HOME", "relative-xdg")
		}},
		{name: "control character", configure: func(t *testing.T, home string) {
			t.Setenv("YAZI_CONFIG_HOME", filepath.Join(home, "bad\npath"))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := withTempHome(t)
			t.Setenv("XDG_CONFIG_HOME", "")
			test.configure(t, home)
			files, omission, err := convenienceBackupFiles(home)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(files, defaultBackupFiles) {
				t.Fatalf("unsafe Yazi location changed safe base inventory: got %v want %v", files, defaultBackupFiles)
			}
			if omission == "" {
				t.Fatal("unsafe Yazi location omitted without explicit metadata")
			}
		})
	}
}

func TestConvenienceBackupKeymapAndThemeOnlyManifest(t *testing.T) {
	home := withTempHome(t)
	dir := filepath.Join(home, "active-yazi")
	t.Setenv("YAZI_CONFIG_HOME", dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"keymap.toml", "theme.toml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, omission, err := convenienceBackupFiles(home)
	if err != nil || omission != "" {
		t.Fatalf("inventory omission=%q err=%v", omission, err)
	}
	backupDir := filepath.Join(home, ".config", "dotfiles", "backups", "keymap-theme")
	count, err := backup.Create(home, backupDir, files)
	if err != nil || count != 2 {
		t.Fatalf("backup count=%d err=%v", count, err)
	}
	entries, err := backup.ReadManifest(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	wants := []string{filepath.Join("active-yazi", "keymap.toml"), filepath.Join("active-yazi", "theme.toml")}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.RelPath)
	}
	slices.Sort(got)
	slices.Sort(wants)
	if !slices.Equal(got, wants) {
		t.Fatalf("manifest entries=%v want=%v", got, wants)
	}
}

func TestConvenienceBackupManualAndAutoUseContainedOverride(t *testing.T) {
	home := withTempHome(t)
	dir := filepath.Join(home, "contained-yazi")
	t.Setenv("YAZI_CONFIG_HOME", dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keymap.toml"), []byte("keys"), 0o600); err != nil {
		t.Fatal(err)
	}

	manualMsg, ok := createBackupCmd()().(backupCreateDoneMsg)
	if !ok || manualMsg.err != nil {
		t.Fatalf("manual backup result=%#v", manualMsg)
	}
	manualDir := filepath.Join(config.ConfigDir(), "backups", manualMsg.name)
	assertBackupManifestHasOnly(t, manualDir, filepath.Join("contained-yazi", "keymap.toml"))

	auto, err := autoBackupIfEnabled()
	if err != nil || auto.omission != "" || auto.backupDir == "" {
		t.Fatalf("auto backup=%+v err=%v", auto, err)
	}
	assertBackupManifestHasOnly(t, auto.backupDir, filepath.Join("contained-yazi", "keymap.toml"))
}

func TestConvenienceBackupOmissionIsSurfacedAndZeroStillFails(t *testing.T) {
	home := withTempHome(t)
	t.Setenv("YAZI_CONFIG_HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("zsh"), 0o600); err != nil {
		t.Fatal(err)
	}
	manualMsg := createBackupCmd()().(backupCreateDoneMsg)
	if manualMsg.err != nil || strings.Contains(manualMsg.name, "Yazi configs omitted") || !strings.Contains(manualMsg.warning, "Yazi configs omitted") {
		t.Fatalf("manual omission result=%#v", manualMsg)
	}
	if filepath.Base(manualMsg.name) != manualMsg.name {
		t.Fatalf("manual backup name is not a pure catalog basename: %q", manualMsg.name)
	}
	if _, err := os.Stat(filepath.Join(config.ConfigDir(), "backups", manualMsg.name)); err != nil {
		t.Fatalf("pure manual name no longer resolves catalog entry: %v", err)
	}
	auto, err := autoBackupIfEnabled()
	if err != nil || auto.omission == "" {
		t.Fatalf("auto omission=%q err=%v", auto.omission, err)
	}

	emptyHome := withTempHome(t)
	t.Setenv("YAZI_CONFIG_HOME", t.TempDir())
	_ = emptyHome
	if msg := createBackupCmd()().(backupCreateDoneMsg); msg.err == nil {
		t.Fatal("zero-file convenience backup reported success")
	}
}

func assertBackupManifestHasOnly(t *testing.T, backupDir, want string) {
	t.Helper()
	entries, err := backup.ReadManifest(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].RelPath != want {
		t.Fatalf("manifest=%v want only %q", entries, want)
	}
}

func TestBackupCreateWarningIsStructuralAndNameRemainsCatalogSafe(t *testing.T) {
	tests := []struct {
		name      string
		warning   string
		wantColor lipgloss.TerminalColor
	}{
		{name: "warning", warning: "Yazi configs omitted: active config is outside HOME", wantColor: ColorYellow},
		{name: "clean", wantColor: ColorGreen},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := newGoldenContext(t)
			ctx.app.backupsLoaded = true
			ctx.app.backupsLoading = false
			screen := NewBackupsScreen(ctx)
			const backupName = "2026-07-12_12-34-56"
			_, cmd := screen.Update(backupCreateDoneMsg{name: backupName, warning: test.warning})
			if cmd == nil || !ctx.app.backupsLoading {
				t.Fatal("successful backup create did not preserve list refresh")
			}
			if ctx.app.backupStatusWarning != (test.warning != "") {
				t.Fatalf("structural warning=%t want=%t", ctx.app.backupStatusWarning, test.warning != "")
			}
			if !strings.Contains(ctx.app.backupStatus, "Created backup: "+backupName) || strings.Contains(backupName, "warning") {
				t.Fatalf("backup identity/status mismatch name=%q status=%q", backupName, ctx.app.backupStatus)
			}
			ctx.app.backupsLoading = false
			view := screen.View(ctx.Width, ctx.Height)
			wantStyled := lipgloss.NewStyle().Foreground(test.wantColor).Render(ctx.app.backupStatus)
			if !strings.Contains(view, wantStyled) {
				t.Fatalf("backup status did not render with expected structural color: status=%q", ctx.app.backupStatus)
			}
		})
	}
}

func TestBackupCreateWarningDoesNotLeakIntoLaterStatuses(t *testing.T) {
	ctx := newGoldenContext(t)
	screen := NewBackupsScreen(ctx)
	warn := backupCreateDoneMsg{name: "warned", warning: "Yazi configs omitted"}
	_, _ = screen.Update(warn)
	if !ctx.app.backupStatusWarning {
		t.Fatal("warned create did not set structural warning")
	}
	_, _ = screen.Update(backupCreateDoneMsg{name: "clean"})
	if ctx.app.backupStatusWarning || ctx.app.backupStatus != "Created backup: clean" {
		t.Fatalf("clean create retained warning: flag=%t status=%q", ctx.app.backupStatusWarning, ctx.app.backupStatus)
	}

	_, _ = screen.Update(warn)
	_, _ = screen.Update(backupRestoreDoneMsg{name: "restored", count: 1})
	if ctx.app.backupStatusWarning || !strings.HasPrefix(ctx.app.backupStatus, "Restored") {
		t.Fatalf("clean restore retained warning: flag=%t status=%q", ctx.app.backupStatusWarning, ctx.app.backupStatus)
	}
	_, _ = screen.Update(warn)
	_, _ = screen.Update(backupDeleteDoneMsg{name: "deleted"})
	if ctx.app.backupStatusWarning || !strings.HasPrefix(ctx.app.backupStatus, "Deleted") {
		t.Fatalf("delete retained warning: flag=%t status=%q", ctx.app.backupStatusWarning, ctx.app.backupStatus)
	}
	_, _ = screen.Update(warn)
	ctx.app.backupRunning = false
	_, _ = screen.Update(keyMsg("r"))
	if ctx.app.backupStatusWarning || ctx.app.backupStatus != "" {
		t.Fatalf("refresh clear retained warning: flag=%t status=%q", ctx.app.backupStatusWarning, ctx.app.backupStatus)
	}
}

func TestConvenienceBackupContainedSymlinkFailsClosedInCreate(t *testing.T) {
	home := withTempHome(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	outside := t.TempDir()
	link := filepath.Join(home, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YAZI_CONFIG_HOME", filepath.Join(link, "yazi"))
	files, omission, err := convenienceBackupFiles(home)
	if err != nil || omission != "" {
		t.Fatalf("lexically contained symlink inventory omission=%q err=%v", omission, err)
	}
	want := filepath.Join("linked", "yazi", "yazi.toml")
	if !slices.Contains(files, want) {
		t.Fatalf("lexical inventory pre-omitted symlink candidate %q: %v", want, files)
	}
	backupDir := filepath.Join(home, ".config", "dotfiles", "backups", "symlink-fail")
	if _, err := backup.Create(home, backupDir, files); err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("backup.Create symlink error=%v", err)
	}
	if _, err := os.Lstat(backupDir); !os.IsNotExist(err) {
		t.Fatalf("failed backup left partial root/manifest: %v", err)
	}
}

func TestConvenienceBackupContainedFileSymlinkFailsClosedInCreate(t *testing.T) {
	home := withTempHome(t)
	dir := filepath.Join(home, "yazi-file-link")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside.toml")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "yazi.toml")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YAZI_CONFIG_HOME", dir)
	files, omission, err := convenienceBackupFiles(home)
	if err != nil || omission != "" || !slices.Contains(files, filepath.Join("yazi-file-link", "yazi.toml")) {
		t.Fatalf("file-symlink inventory=%v omission=%q err=%v", files, omission, err)
	}
	backupDir := filepath.Join(home, ".config", "dotfiles", "backups", "file-symlink-fail")
	if _, err := backup.Create(home, backupDir, files); err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("backup.Create file-symlink error=%v", err)
	}
	if _, err := os.Lstat(backupDir); !os.IsNotExist(err) {
		t.Fatalf("failed file-symlink backup left partial root/manifest: %v", err)
	}
}
