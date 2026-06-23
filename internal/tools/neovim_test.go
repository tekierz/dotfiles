package tools

import (
	"strings"
	"testing"
)

// TestNeovimNumbersDrivesBothOpts proves the C2 reconciliation: the single
// LineNumbers control drives BOTH vim.opt.number and vim.opt.relativenumber, with
// no separate relative-number toggle. Each value must produce the exact pair:
//
//	absolute -> number=true,  relativenumber=false
//	relative -> number=true,  relativenumber=true
//	none     -> number=false, relativenumber=false
//
// The assertions are differential across values (changing LineNumbers changes the
// generated output), which is what the Manage round-trip guardrail relies on.
func TestNeovimNumbersDrivesBothOpts(t *testing.T) {
	cases := []struct {
		lineNumbers     string
		wantNumber      string
		wantRelativeNum string
	}{
		{"absolute", "vim.opt.number = true", "vim.opt.relativenumber = false"},
		{"relative", "vim.opt.number = true", "vim.opt.relativenumber = true"},
		{"none", "vim.opt.number = false", "vim.opt.relativenumber = false"},
	}

	for _, tc := range cases {
		t.Run(tc.lineNumbers, func(t *testing.T) {
			out := GenerateNeovimConfig(NeovimConfig{LineNumbers: tc.lineNumbers}, "catppuccin-mocha")
			if !strings.Contains(out, tc.wantNumber) {
				t.Errorf("LineNumbers=%q: missing %q\n%s", tc.lineNumbers, tc.wantNumber, out)
			}
			if !strings.Contains(out, tc.wantRelativeNum) {
				t.Errorf("LineNumbers=%q: missing %q\n%s", tc.lineNumbers, tc.wantRelativeNum, out)
			}
		})
	}
}
