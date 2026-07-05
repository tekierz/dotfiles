package tools

import "fmt"

type yaziThemePalette struct {
	Cyan    string
	BG      string
	Accent  string
	Accent2 string
	Accent3 string
	Yellow  string
	Green   string
	Magenta string
	FG      string
	Overlay string
	Surface string
	Muted   string
}

var yaziThemePalettes = map[string]yaziThemePalette{
	"catppuccin-mocha": {
		Cyan:    "#94e2d5",
		BG:      "#1e1e2e",
		Accent:  "#89b4fa",
		Accent2: "#a6e3a1",
		Accent3: "#f38ba8",
		Yellow:  "#f9e2af",
		Green:   "#a6e3a1",
		Magenta: "#f5c2e7",
		FG:      "#cdd6f4",
		Overlay: "#45475a",
		Surface: "#313244",
		Muted:   "#6c7086",
	},
	"catppuccin-latte": {
		Cyan:    "#179299",
		BG:      "#eff1f5",
		Accent:  "#1e66f5",
		Accent2: "#40a02b",
		Accent3: "#d20f39",
		Yellow:  "#df8e1d",
		Green:   "#40a02b",
		Magenta: "#ea76cb",
		FG:      "#4c4f69",
		Overlay: "#bcc0cc",
		Surface: "#ccd0da",
		Muted:   "#8c8fa1",
	},
	"catppuccin-frappe": {
		Cyan:    "#81c8be",
		BG:      "#303446",
		Accent:  "#8caaee",
		Accent2: "#a6d189",
		Accent3: "#e78284",
		Yellow:  "#e5c890",
		Green:   "#a6d189",
		Magenta: "#f4b8e4",
		FG:      "#c6d0f5",
		Overlay: "#51576d",
		Surface: "#414559",
		Muted:   "#737994",
	},
	"catppuccin-macchiato": {
		Cyan:    "#8bd5ca",
		BG:      "#24273a",
		Accent:  "#8aadf4",
		Accent2: "#a6da95",
		Accent3: "#ed8796",
		Yellow:  "#eed49f",
		Green:   "#a6da95",
		Magenta: "#f5bde6",
		FG:      "#cad3f5",
		Overlay: "#494d64",
		Surface: "#363a4f",
		Muted:   "#6e738d",
	},
	"dracula": {
		Cyan:    "#8be9fd",
		BG:      "#282a36",
		Accent:  "#bd93f9",
		Accent2: "#50fa7b",
		Accent3: "#ff79c6",
		Yellow:  "#f1fa8c",
		Green:   "#50fa7b",
		Magenta: "#ff79c6",
		FG:      "#f8f8f2",
		Overlay: "#6272a4",
		Surface: "#44475a",
		Muted:   "#6272a4",
	},
	"gruvbox-dark": {
		Cyan:    "#689d6a",
		BG:      "#282828",
		Accent:  "#83a598",
		Accent2: "#b8bb26",
		Accent3: "#fb4934",
		Yellow:  "#d79921",
		Green:   "#98971a",
		Magenta: "#b16286",
		FG:      "#ebdbb2",
		Overlay: "#504945",
		Surface: "#3c3836",
		Muted:   "#928374",
	},
	"gruvbox-light": {
		Cyan:    "#689d6a",
		BG:      "#fbf1c7",
		Accent:  "#076678",
		Accent2: "#79740e",
		Accent3: "#9d0006",
		Yellow:  "#d79921",
		Green:   "#98971a",
		Magenta: "#b16286",
		FG:      "#3c3836",
		Overlay: "#d5c4a1",
		Surface: "#ebdbb2",
		Muted:   "#928374",
	},
	"nord": {
		Cyan:    "#88c0d0",
		BG:      "#2e3440",
		Accent:  "#88c0d0",
		Accent2: "#a3be8c",
		Accent3: "#bf616a",
		Yellow:  "#ebcb8b",
		Green:   "#a3be8c",
		Magenta: "#b48ead",
		FG:      "#d8dee9",
		Overlay: "#434c5e",
		Surface: "#3b4252",
		Muted:   "#4c566a",
	},
	"tokyo-night": {
		Cyan:    "#7dcfff",
		BG:      "#1a1b26",
		Accent:  "#7aa2f7",
		Accent2: "#9ece6a",
		Accent3: "#f7768e",
		Yellow:  "#e0af68",
		Green:   "#9ece6a",
		Magenta: "#bb9af7",
		FG:      "#a9b1d6",
		Overlay: "#414868",
		Surface: "#24283b",
		Muted:   "#565f89",
	},
	"solarized-dark": {
		Cyan:    "#2aa198",
		BG:      "#002b36",
		Accent:  "#268bd2",
		Accent2: "#859900",
		Accent3: "#dc322f",
		Yellow:  "#b58900",
		Green:   "#859900",
		Magenta: "#d33682",
		FG:      "#839496",
		Overlay: "#586e75",
		Surface: "#073642",
		Muted:   "#657b83",
	},
	"solarized-light": {
		Cyan:    "#2aa198",
		BG:      "#fdf6e3",
		Accent:  "#268bd2",
		Accent2: "#859900",
		Accent3: "#dc322f",
		Yellow:  "#b58900",
		Green:   "#859900",
		Magenta: "#d33682",
		FG:      "#657b83",
		Overlay: "#93a1a1",
		Surface: "#eee8d5",
		Muted:   "#839496",
	},
	"monokai": {
		Cyan:    "#a1efe4",
		BG:      "#272822",
		Accent:  "#66d9ef",
		Accent2: "#a6e22e",
		Accent3: "#f92672",
		Yellow:  "#f4bf75",
		Green:   "#a6e22e",
		Magenta: "#ae81ff",
		FG:      "#f8f8f2",
		Overlay: "#49483e",
		Surface: "#3e3d32",
		Muted:   "#75715e",
	},
	"rose-pine": {
		Cyan:    "#ebbcba",
		BG:      "#191724",
		Accent:  "#c4a7e7",
		Accent2: "#9ccfd8",
		Accent3: "#eb6f92",
		Yellow:  "#f6c177",
		Green:   "#9ccfd8",
		Magenta: "#c4a7e7",
		FG:      "#e0def4",
		Overlay: "#26233a",
		Surface: "#1f1d2e",
		Muted:   "#6e6a86",
	},
	"everforest": {
		Cyan:    "#83c092",
		BG:      "#2d353b",
		Accent:  "#7fbbb3",
		Accent2: "#a7c080",
		Accent3: "#e67e80",
		Yellow:  "#dbbc7f",
		Green:   "#a7c080",
		Magenta: "#d699b6",
		FG:      "#d3c6aa",
		Overlay: "#3d484d",
		Surface: "#343f44",
		Muted:   "#859289",
	},
	"one-dark": {
		Cyan:    "#56b6c2",
		BG:      "#282c34",
		Accent:  "#61afef",
		Accent2: "#98c379",
		Accent3: "#e06c75",
		Yellow:  "#e5c07b",
		Green:   "#98c379",
		Magenta: "#c678dd",
		FG:      "#abb2bf",
		Overlay: "#2c323c",
		Surface: "#21252b",
		Muted:   "#5c6370",
	},
	"neon-seapunk": {
		Cyan:    "#00F5D4",
		BG:      "#070B1A",
		Accent:  "#00F5D4",
		Accent2: "#00F5A0",
		Accent3: "#FF4D6D",
		Yellow:  "#FEE440",
		Green:   "#00F5A0",
		Magenta: "#F15BB5",
		FG:      "#E6F1FF",
		Overlay: "#172046",
		Surface: "#0F1633",
		Muted:   "#97A7C7",
	},
}

func yaziPaletteForTheme(theme string) yaziThemePalette {
	if palette, ok := yaziThemePalettes[theme]; ok {
		return palette
	}
	return yaziThemePalettes["catppuccin-mocha"]
}

// GenerateYaziTheme builds theme.toml content using the same section structure
// as the legacy installer's generate_yazi_theme function.
func GenerateYaziTheme(theme string) string {
	p := yaziPaletteForTheme(theme)
	return fmt.Sprintf(`# Theme: %s (generated by dotfiles)

[manager]
cwd = { fg = "%s" }
hovered = { fg = "%s", bg = "%s" }
preview_hovered = { underline = true }

find_keyword = { fg = "%s", bold = true, italic = true, underline = true }
find_position = { fg = "%s", bg = "reset", bold = true }

marker_copied = { fg = "%s", bg = "%s" }
marker_cut = { fg = "%s", bg = "%s" }
marker_marked = { fg = "%s", bg = "%s" }
marker_selected = { fg = "%s", bg = "%s" }

tab_active = { fg = "%s", bg = "%s" }
tab_inactive = { fg = "%s", bg = "%s" }

border_symbol = "│"
border_style = { fg = "%s" }

[status]
separator_open = ""
separator_close = ""
separator_style = { fg = "%s", bg = "%s" }

mode_normal = { fg = "%s", bg = "%s", bold = true }
mode_select = { fg = "%s", bg = "%s", bold = true }

progress_label = { fg = "%s", bold = true }
progress_normal = { fg = "%s", bg = "%s" }
progress_error = { fg = "%s", bg = "%s" }

permissions_t = { fg = "%s" }
permissions_r = { fg = "%s" }
permissions_w = { fg = "%s" }
permissions_x = { fg = "%s" }
permissions_s = { fg = "%s" }

[input]
border = { fg = "%s" }
title = {}
value = {}
selected = { reversed = true }

[select]
border = { fg = "%s" }
active = { fg = "%s", bold = true }
inactive = {}

[tasks]
border = { fg = "%s" }
title = {}
hovered = { fg = "%s", underline = true }

[which]
cols = 3
mask = { bg = "%s" }
cand = { fg = "%s" }
rest = { fg = "%s" }
desc = { fg = "%s" }
separator = "  "
separator_style = { fg = "%s" }

[help]
on = { fg = "%s" }
run = { fg = "%s" }
desc = {}
hovered = { reversed = true, bold = true }
footer = { fg = "%s", bg = "%s" }

[filetype]
rules = [
  { mime = "image/*", fg = "%s" },
  { mime = "video/*", fg = "%s" },
  { mime = "audio/*", fg = "%s" },
  { mime = "application/zip", fg = "%s" },
  { mime = "application/gzip", fg = "%s" },
  { mime = "application/x-tar", fg = "%s" },
  { mime = "application/pdf", fg = "%s" },
  { url = "*", fg = "%s" },
  { url = "*/", fg = "%s" },
]
`, theme,
		p.Cyan,
		p.BG, p.Accent,
		p.Yellow,
		p.Accent3,
		p.Green, p.Green,
		p.Accent3, p.Accent3,
		p.Accent, p.Accent,
		p.Yellow, p.Yellow,
		p.BG, p.FG,
		p.FG, p.Overlay,
		p.Muted,
		p.Overlay, p.Overlay,
		p.BG, p.Accent,
		p.BG, p.Accent2,
		p.FG,
		p.Accent, p.Overlay,
		p.Accent3, p.Overlay,
		p.Accent,
		p.Yellow,
		p.Accent3,
		p.Green,
		p.Muted,
		p.Accent,
		p.Accent,
		p.Accent3,
		p.Accent,
		p.Accent3,
		p.Surface,
		p.Cyan,
		p.Muted,
		p.Accent3,
		p.Overlay,
		p.Cyan,
		p.Accent3,
		p.Overlay, p.FG,
		p.Yellow,
		p.Accent3,
		p.Accent3,
		p.Magenta,
		p.Magenta,
		p.Magenta,
		p.Accent3,
		p.FG,
		p.Accent,
	)
}
