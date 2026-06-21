package ui

import (
	"github.com/tekierz/dotfiles/internal/config"
)

// NewScreenContext creates a new screen context, loading initial values from
// persisted config to seed the theme / nav style / animation defaults.
func NewScreenContext() *ScreenContext {
	ctx := &ScreenContext{
		Theme:             "catppuccin-mocha",
		NavStyle:          "emacs",
		AnimationsEnabled: true,
		Width:             80,
		Height:            24,
	}

	// Load persisted settings if available. Only override the defaults above when
	// the persisted value is set, so a partial/hand-edited config that omits
	// theme/nav_style doesn't clobber the sensible defaults with empty strings.
	if cfg, err := config.LoadGlobalConfig(); err == nil {
		if cfg.Theme != "" {
			ctx.Theme = cfg.Theme
		}
		if cfg.NavStyle != "" {
			ctx.NavStyle = cfg.NavStyle
		}
		ctx.AnimationsEnabled = !cfg.DisableAnimations
	}

	return ctx
}
