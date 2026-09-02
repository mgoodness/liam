// Package theme provides liam's Catppuccin Frappe (dark) and Latte (light)
// palettes, and the startup-only auto-detection/override logic that picks
// between them.
package theme

// Palette is the subset of Catppuccin colors liam's TUI and headless output
// use.
type Palette struct {
	Base, Text, Subtext, Overlay    string
	Blue, Green, Yellow, Red, Mauve string
	Dark                            bool
}

// Frappe is Catppuccin's dark palette, liam's default.
var Frappe = Palette{
	Base: "#303446", Text: "#c6d0f5", Subtext: "#a5adce", Overlay: "#737994",
	Blue: "#8caaee", Green: "#a6d189", Yellow: "#e5c890", Red: "#e78284", Mauve: "#ca9ee6",
	Dark: true,
}

// Latte is Catppuccin's light palette.
var Latte = Palette{
	Base: "#eff1f5", Text: "#4c4f69", Subtext: "#6c6f85", Overlay: "#9ca0b0",
	Blue: "#1e66f5", Green: "#40a02b", Yellow: "#df8e1d", Red: "#d20f39", Mauve: "#8839ef",
	Dark: false,
}

// Resolve picks Frappe or Latte given the config's theme.mode ("auto" or
// empty, "dark", "light") and, for "auto", the detected background
// darkness. Detection is startup-only and, per spec, defaults dark on
// failure — callers pass isDark=true when detection didn't produce a
// definitive answer.
func Resolve(mode string, isDark bool) Palette {
	switch mode {
	case "light":
		return Latte
	case "dark":
		return Frappe
	default:
		if isDark {
			return Frappe
		}
		return Latte
	}
}
