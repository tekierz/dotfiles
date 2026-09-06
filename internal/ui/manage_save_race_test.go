package ui

import (
	"runtime"
	"sync"
	"testing"

	"github.com/tekierz/dotfiles/internal/config"
)

func TestManageSaveUsesSnapshotDuringConcurrentMutation(t *testing.T) {
	a, _, _ := newPlanTestApp(t)
	a.manageConfig.GhosttyFontSize++
	want := a.manageConfig.GhosttyFontSize
	_ = a.prepareManageSave()
	cmd := a.executeManageSavePlanCmd()
	if cmd == nil {
		t.Fatal("executeManageSavePlanCmd returned nil")
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var msg any
	var saved manageSaveDoneMsg
	var ok bool

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		msg = cmd()
		saved, ok = msg.(manageSaveDoneMsg)
	}()

	close(start)
	cfg := a.manageConfig
	for i := 0; i < 100000; i++ {
		cfg.GhosttyFontSize = 10 + i%40
		cfg.GhosttyOpacity = i % 101
		cfg.GhosttyConfirmClose = i%2 == 0
		cfg.TmuxHistoryLimit = 1000 + i
		cfg.TmuxMouseMode = !cfg.TmuxMouseMode
		cfg.ZshAutoCD = !cfg.ZshAutoCD
		if i%2 == 0 {
			cfg.GitDefaultBranch = "main"
			cfg.FzfLayout = "reverse"
		} else {
			cfg.GitDefaultBranch = "develop"
			cfg.FzfLayout = "default"
		}
		if i%128 == 0 {
			runtime.Gosched()
		}
	}
	wg.Wait()

	if !ok {
		t.Fatalf("executeManageSavePlanCmd returned %T, want manageSaveDoneMsg", msg)
	}
	if saved.err != nil {
		t.Fatalf("executeManageSavePlanCmd returned error: %v", saved.err)
	}
	persisted, err := config.LoadToolConfig("manage", NewManageConfig)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.GhosttyFontSize != want {
		t.Fatalf("persisted accepted font size=%d, want %d", persisted.GhosttyFontSize, want)
	}
}

func TestSanitizeLogLineStripsANSIAndControls(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "ordinary text",
			in:   "ordinary package output",
			want: "ordinary package output",
		},
		{
			name: "csi color",
			in:   "installing \x1b[31mred\x1b[0m package",
			want: "installing red package",
		},
		{
			name: "osc title",
			in:   "before \x1b]0;pwned\x07 after",
			want: "before  after",
		},
		{
			name: "osc clipboard",
			in:   "copy \x1b]52;c;cHduZWQ=\x07 blocked",
			want: "copy  blocked",
		},
		{
			name: "bare controls",
			in:   "bad\rover\bwrite\x07\nend\x7f\u0085",
			want: "badoverwriteend",
		},
		{
			name: "tab to space",
			in:   "left\tright",
			want: "left right",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeLogLine(tt.in); got != tt.want {
				t.Fatalf("sanitizeLogLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInstallCacheScreensInitLoadsCache(t *testing.T) {
	tests := []struct {
		name      string
		newScreen func(*ScreenContext) ScreenHandler
	}{
		{"cli tools", func(ctx *ScreenContext) ScreenHandler { return NewConfigCLIToolsScreen(ctx) }},
		{"cli utilities", func(ctx *ScreenContext) ScreenHandler { return NewConfigCLIUtilitiesScreen(ctx) }},
		{"utilities", func(ctx *ScreenContext) ScreenHandler { return NewConfigUtilitiesScreen(ctx) }},
		{"mac apps", func(ctx *ScreenContext) ScreenHandler { return NewConfigMacAppsScreen(ctx) }},
		{"gui apps", func(ctx *ScreenContext) ScreenHandler { return NewConfigGUIAppsScreen(ctx) }},
		{"file tree", func(ctx *ScreenContext) ScreenHandler { return NewFileTreeScreen(ctx) }},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/not-ready", func(t *testing.T) {
			ctx := newGoldenContext(t)
			ctx.app.manageInstalledReady = false
			ctx.app.installCacheLoading = false

			if cmd := tt.newScreen(ctx).Init(); cmd == nil {
				t.Fatal("Init() returned nil, want async install-cache command")
			}
			if !ctx.app.installCacheLoading {
				t.Fatal("Init() did not mark install cache as loading")
			}
		})

		t.Run(tt.name+"/already-ready", func(t *testing.T) {
			ctx := newGoldenContext(t)
			ctx.app.manageInstalledReady = true
			ctx.app.installCacheLoading = false

			if cmd := tt.newScreen(ctx).Init(); cmd != nil {
				t.Fatal("Init() returned command while install cache was already ready")
			}
			if ctx.app.installCacheLoading {
				t.Fatal("Init() marked install cache as loading while cache was already ready")
			}
		})
	}
}
