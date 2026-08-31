// PROTOTYPE — throwaway code, not part of liam.
package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) dims() (int, int) {
	w, h := m.width, m.height
	if w < 60 {
		w = 80
	}
	if h < 20 {
		h = 24
	}
	return w, h - 1 // reserve one line for the switcher bar
}

func renderTurn(p palette, t turn, width int) string {
	switch t.role {
	case "user":
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(p.blue)).Bold(true)
		return style.Render("› " + t.text)
	case "assistant":
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(p.text))
		return style.Render(t.text)
	case "tool":
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(p.subtext)).Italic(true)
		return style.Render("  ⚙ " + t.text)
	}
	return t.text
}

func (m model) statusLine(p palette, width int) string {
	mode := "prompt"
	modeColor := p.yellow
	theme := "frappe (dark)"
	if !m.dark {
		theme = "latte (light)"
	}
	left := fmt.Sprintf(" liam  •  %s  •  %s ", mockModel, theme)
	right := fmt.Sprintf(" mode: %s ", mode)
	leftStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.base)).Background(lipgloss.Color(p.mauve)).Bold(true)
	rightStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.base)).Background(lipgloss.Color(modeColor)).Bold(true)
	l, r := leftStyle.Render(left), rightStyle.Render(right)
	gap := width - lipgloss.Width(l) - lipgloss.Width(r)
	if gap < 0 {
		gap = 0
	}
	bg := lipgloss.NewStyle().Background(lipgloss.Color(p.surface)).Render(strings.Repeat(" ", gap))
	return l + bg + r
}

func (m model) inputLine(p palette, width int) string {
	box := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.text)).
		Background(lipgloss.Color(p.surface)).
		Width(width - 4).
		Padding(0, 1)
	prompt := lipgloss.NewStyle().Foreground(lipgloss.Color(p.green)).Bold(true).Render("> ")
	return prompt + box.Render("edit the tests too█")
}

func (m model) permissionBox(p palette, width int) string {
	if !m.showPrompt {
		return ""
	}
	title := lipgloss.NewStyle().Foreground(lipgloss.Color(p.base)).Background(lipgloss.Color(p.red)).Bold(true).Padding(0, 1).
		Render(" permission needed ")
	body := lipgloss.NewStyle().Foreground(lipgloss.Color(p.text)).Render(mockPendingTool)
	options := lipgloss.NewStyle().Foreground(lipgloss.Color(p.overlay)).
		Render("[1] allow once   [2] allow for session   [3] deny")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(p.red)).
		Padding(0, 1).
		Width(width - 4)
	return box.Render(title + "\n" + body + "\n" + options)
}

// Variant A: everything (status, conversation + inline tool calls +
// inline permission prompt, input) stacked in one continuous column —
// closest to how Claude Code itself renders.
func (m model) renderA() string {
	p := m.pal()
	w, h := m.dims()
	var lines []string
	lines = append(lines, m.statusLine(p, w))
	lines = append(lines, "")
	for _, t := range mockConversation {
		lines = append(lines, renderTurn(p, t, w))
	}
	if m.showPrompt {
		lines = append(lines, "")
		lines = append(lines, m.permissionBox(p, w))
	}
	content := strings.Join(lines, "\n")
	content = padTo(content, w, h-2)
	return content + "\n" + m.inputLine(p, w)
}

// Variant B: conversation and tool/permission activity are two
// independent panels side by side — main pane stays purely conversational.
func (m model) renderB() string {
	p := m.pal()
	w, h := m.dims()
	sidebarW := w / 3
	mainW := w - sidebarW - 3

	var convLines []string
	for _, t := range mockConversation {
		if t.role == "tool" {
			continue
		}
		convLines = append(convLines, renderTurn(p, t, mainW))
	}
	mainPane := lipgloss.NewStyle().Width(mainW).Height(h - 4).Render(strings.Join(convLines, "\n\n"))

	var actLines []string
	actTitle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.subtext)).Bold(true).Render("ACTIVITY")
	actLines = append(actLines, actTitle, "")
	for _, t := range mockConversation {
		if t.role != "tool" {
			continue
		}
		actLines = append(actLines, lipgloss.NewStyle().Foreground(lipgloss.Color(p.subtext)).Render("⚙ "+truncate(t.text, sidebarW-2)))
	}
	if m.showPrompt {
		actLines = append(actLines, "")
		promptStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.red)).
			Foreground(lipgloss.Color(p.text)).Padding(0, 1).Width(sidebarW - 4)
		actLines = append(actLines, promptStyle.Render(
			lipgloss.NewStyle().Foreground(lipgloss.Color(p.red)).Bold(true).Render("permission needed")+"\n"+
				mockPendingTool+"\n"+
				lipgloss.NewStyle().Foreground(lipgloss.Color(p.overlay)).Render("[1] once [2] session [3] deny")))
	}
	sidebar := lipgloss.NewStyle().Width(sidebarW).Height(h - 4).
		BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(p.surface)).BorderLeft(true).
		PaddingLeft(1).
		Render(strings.Join(actLines, "\n"))

	row := lipgloss.JoinHorizontal(lipgloss.Top, mainPane, sidebar)
	return m.statusLine(p, w) + "\n\n" + row + "\n" + m.inputLine(p, w)
}

// Variant C: three horizontal bands — a fuller status block, conversation,
// and a dedicated always-visible "activity strip" for the most recent
// tool call / permission prompt, distinct from scrollback (k9s/lazygit-style).
func (m model) renderC() string {
	p := m.pal()
	w, h := m.dims()

	mode := "prompt"
	statusBlock := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.text)).Background(lipgloss.Color(p.surface)).
		Width(w).Padding(0, 1).
		Render(fmt.Sprintf("liam  •  model: %s  •  mode: %s\ntheme: %s", mockModel, mode, themeName(m.dark)))

	var convLines []string
	for _, t := range mockConversation {
		if t.role == "tool" {
			continue
		}
		convLines = append(convLines, renderTurn(p, t, w))
	}
	conv := lipgloss.NewStyle().Width(w).Height(h - 8).Render(strings.Join(convLines, "\n"))

	var latest string
	for _, t := range mockConversation {
		if t.role == "tool" {
			latest = t.text
		}
	}
	stripLabel := lipgloss.NewStyle().Foreground(lipgloss.Color(p.subtext)).Bold(true).Render("ACTIVITY ")
	stripBody := lipgloss.NewStyle().Foreground(lipgloss.Color(p.yellow)).Render("⚙ " + latest)
	strip := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(p.surface)).
		Width(w - 2).Padding(0, 1).Render(stripLabel + stripBody)

	var out string
	out = statusBlock + "\n\n" + conv + "\n" + strip
	if m.showPrompt {
		out += "\n" + m.permissionBox(p, w)
	}
	return out + "\n" + m.inputLine(p, w)
}

func themeName(dark bool) string {
	if dark {
		return "Catppuccin Frappe (dark)"
	}
	return "Catppuccin Latte (light)"
}

func truncate(s string, n int) string {
	if n <= 1 || len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func padTo(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
