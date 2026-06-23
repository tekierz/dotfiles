package ui

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderMiniGlobe renders a small ASCII "globe" using sphere shading + a simple
// landmass model so it's recognizable even at small sizes.
//
// This is not meant to be a perfect simulation; it's a lightweight, always-on
// animation that fits nicely into a corner of the management UI.
func RenderMiniGlobe(width, height int, frame int) string {
	if width < 8 || height < 4 {
		return ""
	}

	// Rotation (yaw) makes it feel alive (keep it slow/smooth).
	yaw := float64(frame) * 0.08
	pitch := 0.35

	// Light direction (slightly above and to the right).
	lx, ly, lz := 0.35, -0.25, 1.0
	ll := math.Sqrt(lx*lx + ly*ly + lz*lz)
	lx, ly, lz = lx/ll, ly/ll, lz/ll

	// Shading ramps (dark → bright).
	landRamp := []rune(" .:-=+*#%@")
	waterRamp := []rune(" .,:;~")

	// Terminal cells are typically taller than they are wide; compensate so the
	// sphere looks round-ish.
	aspect := float64(width) / float64(height) * 0.55

	brightLand := lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
	midLand := lipgloss.NewStyle().Foreground(ColorCyan)
	dimLand := lipgloss.NewStyle().Foreground(ColorMuted).Faint(true)

	brightWater := lipgloss.NewStyle().Foreground(ColorNeonBlue)
	midWater := lipgloss.NewStyle().Foreground(ColorBorder)
	dimWater := lipgloss.NewStyle().Foreground(ColorBorder).Faint(true)

	rimStyle := lipgloss.NewStyle().Foreground(ColorBorder).Faint(true)

	lines := make([]string, 0, height)
	for y := 0; y < height; y++ {
		ny := 2*float64(y)/float64(height-1) - 1

		var sb strings.Builder
		for x := 0; x < width; x++ {
			nx := 2*float64(x)/float64(width-1) - 1
			nx *= aspect

			r2 := nx*nx + ny*ny
			if r2 > 1 {
				sb.WriteByte(' ')
				continue
			}

			z := math.Sqrt(1 - r2)

			// Rotate around Y (yaw).
			px := nx*math.Cos(yaw) + z*math.Sin(yaw)
			pz := -nx*math.Sin(yaw) + z*math.Cos(yaw)
			py := ny

			// Rotate around X (pitch) for a nicer perspective.
			py2 := py*math.Cos(pitch) - pz*math.Sin(pitch)
			pz2 := py*math.Sin(pitch) + pz*math.Cos(pitch)
			py, pz = py2, pz2

			// Lambertian shading.
			intensity := px*lx + py*ly + pz*lz
			if intensity < 0 {
				intensity = 0
			}
			if intensity > 1 {
				intensity = 1
			}

			// Slight rim highlight to make the sphere pop.
			if r2 > 0.985 && r2 <= 1.0 {
				sb.WriteString(rimStyle.Render("·"))
				continue
			}

			lon := math.Atan2(px, pz) // [-pi, pi]
			lat := math.Asin(py)      // [-pi/2, pi/2]
			lonDeg := lon * 180 / math.Pi
			latDeg := lat * 180 / math.Pi

			if globeIsLand(lonDeg, latDeg) {
				ch := globeRampChar(landRamp, intensity)
				sb.WriteString(globeShade(ch, intensity, brightLand, midLand, dimLand))
			} else {
				ch := globeRampChar(waterRamp, intensity)
				sb.WriteString(globeShade(ch, intensity, brightWater, midWater, dimWater))
			}
		}

		lines = append(lines, sb.String())
	}

	return strings.Join(lines, "\n")
}

// globeWrapDeltaDeg normalizes a longitude delta to [-180, 180).
func globeWrapDeltaDeg(d float64) float64 {
	d = math.Mod(d+180.0, 360.0)
	if d < 0 {
		d += 360.0
	}
	return d - 180.0
}

// globeIsLand reports whether a lon/lat (degrees) falls inside the simple
// landmass model (a few elliptical blobs roughly tracing Earth's continents).
func globeIsLand(lonDeg, latDeg float64) bool {
	type blob struct{ lon, lat, rx, ry float64 }
	blobs := []blob{
		// Rough continents (hand-tuned; good enough to read as "Earth").
		{lon: -100, lat: 40, rx: 35, ry: 20}, // North America
		{lon: -75, lat: 10, rx: 18, ry: 18},  // Central America
		{lon: -60, lat: -20, rx: 20, ry: 28}, // South America
		{lon: -40, lat: 70, rx: 16, ry: 10},  // Greenland
		{lon: 60, lat: 45, rx: 75, ry: 25},   // Eurasia
		{lon: 20, lat: 5, rx: 30, ry: 35},    // Africa
		{lon: 95, lat: 20, rx: 22, ry: 18},   // India / SE Asia
		{lon: 135, lat: -25, rx: 22, ry: 12}, // Australia
		{lon: 0, lat: -78, rx: 180, ry: 10},  // Antarctica belt
		{lon: 140, lat: 35, rx: 18, ry: 12},  // Japan-ish bump
		{lon: -10, lat: 55, rx: 14, ry: 10},  // Europe-ish bump
	}

	for _, b := range blobs {
		dx := globeWrapDeltaDeg(lonDeg-b.lon) / b.rx
		dy := (latDeg - b.lat) / b.ry
		if dx*dx+dy*dy <= 1.0 {
			return true
		}
	}
	return false
}

// globeRampChar picks the shading-ramp glyph for a given intensity, clamping the
// index into the ramp's valid range.
func globeRampChar(ramp []rune, intensity float64) string {
	idx := int(intensity * float64(len(ramp)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(ramp) {
		idx = len(ramp) - 1
	}
	return string(ramp[idx])
}

// globeShade renders ch with the bright/mid/dim style selected by intensity.
func globeShade(ch string, intensity float64, bright, mid, dim lipgloss.Style) string {
	switch {
	case intensity > 0.65:
		return bright.Render(ch)
	case intensity > 0.35:
		return mid.Render(ch)
	default:
		return dim.Render(ch)
	}
}
