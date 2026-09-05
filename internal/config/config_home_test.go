package config

import "testing"

func TestConfigDirRejectsRelativeHome(t *testing.T) {
	t.Setenv("HOME", "relative/home")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got := ConfigDir(); got != "" {
		t.Fatalf("relative HOME produced config path %q", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "/absolute/config")
	if got := ConfigDir(); got != "/absolute/config/dotfiles" {
		t.Fatalf("absolute XDG root ignored: %q", got)
	}
}
