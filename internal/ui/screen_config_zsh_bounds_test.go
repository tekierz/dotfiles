package ui

import "testing"

func TestZshAdjustIgnoresNegativeField(t *testing.T) {
	a := &App{deepDiveConfig: NewDeepDiveConfig(), configFieldIndex: -1}
	before := a.deepDiveConfig.ZshPromptStyle
	zshAdjust(a, "right", true)
	if a.deepDiveConfig.ZshPromptStyle != before {
		t.Fatal("invalid field changed prompt style")
	}
}
