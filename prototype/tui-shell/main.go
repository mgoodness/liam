// PROTOTYPE — throwaway code, not part of liam. Do not import from elsewhere.
//
// Plan: three variants of liam's TUI shell, switchable with ← / → (or h/l),
// showing conversation view + input + status line + tool-call rendering +
// a permission prompt, using mocked/static conversation data. Press 't' to
// toggle Catppuccin Frappe (dark) / Latte (light). Press 'p' to toggle
// whether the permission prompt is showing.
//
// Question this answers: liam wayfinder ticket #23 — what should liam's
// Bubbletea TUI shell look like and behave like?
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---- Catppuccin palette (subset) ----

type palette struct {
	base, text, subtext, overlay, surface string
	blue, green, yellow, red, mauve       string
}

var frappe = palette{
	base: "#303446", text: "#c6d0f5", subtext: "#a5adce", overlay: "#737994", surface: "#414559",
	blue: "#8caaee", green: "#a6d189", yellow: "#e5c890", red: "#e78284", mauve: "#ca9ee6",
}

var latte = palette{
	base: "#eff1f5", text: "#4c4f69", subtext: "#6c6f85", overlay: "#9ca0b0", surface: "#ccd0da",
	blue: "#1e66f5", green: "#40a02b", yellow: "#df8e1d", red: "#d20f39", mauve: "#8839ef",
}

// ---- mocked conversation data ----

type turn struct {
	role string // "user" | "assistant" | "tool"
	text string
}

var mockConversation = []turn{
	{"user", "add error handling to the CSV parser"},
	{"assistant", "I'll take a look at the parser first."},
	{"tool", "grep(pattern: \"func ParseCSV\", path: \"internal/csv\") → internal/csv/parser.go:14"},
	{"assistant", "Found it. Reading the file, then I'll add error handling around the row-splitting logic."},
	{"tool", "read(path: \"internal/csv/parser.go\") → 82 lines"},
	{"assistant", "Now applying the edit."},
}

const mockModel = "anthropic/claude-opus-4.7"
const mockPendingTool = `bash(command: "go test ./internal/csv/...")`

// ---- app model ----

type variant int

const (
	variantA variant = iota
	variantB
	variantC
	variantCount
)

func (v variant) label() string {
	switch v {
	case variantA:
		return "A — Stacked stream"
	case variantB:
		return "B — Split panel"
	case variantC:
		return "C — Command center"
	}
	return "?"
}

type model struct {
	variant      variant
	dark         bool
	showPrompt   bool
	width        int
	height       int
	input        string
	statusLayout statusLayout
}

type statusLayout int

const (
	statusCompact statusLayout = iota
	statusExpanded
)

func initialModel() model {
	return model{variant: variantA, dark: true, showPrompt: true, input: "", statusLayout: statusExpanded}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "left", "h":
			m.variant = (m.variant - 1 + variantCount) % variantCount
		case "right", "l", "tab":
			m.variant = (m.variant + 1) % variantCount
		case "t":
			m.dark = !m.dark
		case "p":
			m.showPrompt = !m.showPrompt
		case "s":
			if m.statusLayout == statusCompact {
				m.statusLayout = statusExpanded
			} else {
				m.statusLayout = statusCompact
			}
		}
	}
	return m, nil
}

func (m model) pal() palette {
	if m.dark {
		return frappe
	}
	return latte
}

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	p := m.pal()
	w, h := m.dims()
	var body string
	switch m.variant {
	case variantA:
		body = m.renderA()
	case variantB:
		body = m.renderB()
	case variantC:
		body = m.renderC()
	}
	// Paint the theme's base background across the full canvas, not just
	// individual badges — otherwise blank space and plain text fall through
	// to the terminal's own default background regardless of theme.
	canvas := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.text)).
		Background(lipgloss.Color(p.base)).
		Width(w).
		Render(padTo(body, w, h))
	switcher := m.renderSwitcher()
	return canvas + "\n" + switcher
}

func (m model) renderSwitcher() string {
	p := m.pal()
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.base)).
		Background(lipgloss.Color(p.yellow)).
		Padding(0, 1).
		Bold(true)
	help := lipgloss.NewStyle().Foreground(lipgloss.Color(p.overlay))
	return style.Render(fmt.Sprintf(" ← %s → ", m.variant.label())) +
		help.Render("  [t] theme  [p] permission prompt  [s] status layout  [q] quit")
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--dump" {
		dumpAll()
		return
	}
	if _, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// dumpAll renders every variant once, non-interactively, so the result can
// be inspected without a TTY (e.g. `go run . --dump`).
func dumpAll() {
	for v := variantA; v < variantCount; v++ {
		m := model{variant: v, dark: true, showPrompt: true, statusLayout: statusExpanded, width: 96, height: 28}
		fmt.Printf("\n========== Variant %s ==========\n\n", v.label())
		fmt.Println(m.View())
	}
	fmt.Println("\n========== Variant A, compact status ==========")
	fmt.Println(model{variant: variantA, dark: true, showPrompt: true, statusLayout: statusCompact, width: 96, height: 28}.View())
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, n)
}
