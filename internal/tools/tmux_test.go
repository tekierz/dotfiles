package tools

import (
	"strings"
	"testing"
)

func TestGenerateTmuxConfigSplitBindingsSurvive(t *testing.T) {
	cases := []struct {
		name          string
		splitBinds    string
		wantBinds     []string
		forbiddenKeys []string
	}{
		{
			name:       "documented pipe splits",
			splitBinds: "pipes",
			wantBinds: []string{
				"bind | split-window -h -c \"#{pane_current_path}\"",
				"bind - split-window -v -c \"#{pane_current_path}\"",
			},
			forbiddenKeys: []string{"|", "-"},
		},
		{
			name:       "percent splits",
			splitBinds: "percent",
			wantBinds: []string{
				"bind % split-window -h -c \"#{pane_current_path}\"",
				"bind '\"' split-window -v -c \"#{pane_current_path}\"",
			},
			forbiddenKeys: []string{"%", "'\"'"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := GenerateTmuxConfig(TmuxConfig{SplitBinds: tc.splitBinds}, "catppuccin-mocha")
			for _, want := range tc.wantBinds {
				if !strings.Contains(out, want) {
					t.Fatalf("generated config missing split binding %q:\n%s", want, out)
				}
			}
			for _, key := range tc.forbiddenKeys {
				if strings.Contains(out, "unbind "+key+"\n") {
					t.Fatalf("generated config binds and unbinds split key %s:\n%s", key, out)
				}
			}
		})
	}
}
