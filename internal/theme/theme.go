package theme

// DefaultTheme is used when an unknown theme name is requested.
const DefaultTheme = "neon-seapunk"

// Get returns the palette for the named theme and whether it was found.
func Get(name string) (Palette, bool) {
	p, ok := palettes[name]
	return p, ok
}

// GetOrDefault returns the named theme's palette, falling back to DefaultTheme
// (guaranteed to exist) when name is unknown.
func GetOrDefault(name string) Palette {
	if p, ok := palettes[name]; ok {
		return p
	}
	return palettes[DefaultTheme]
}

// Names returns the theme names in canonical order.
func Names() []string {
	out := make([]string, len(names))
	copy(out, names)
	return out
}

// Exists reports whether name is a known theme.
func Exists(name string) bool {
	_, ok := palettes[name]
	return ok
}
