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

// statusBlock mimics Claude Code's own external-command statusline
// primitive: a script gets session data and prints one or more lines,
// each becoming a row (see claude-hud, the user's own active statusline,
// for a rich real-world example of the "expanded, multi-line, several
// segments" end of that spectrum). "compact" here corresponds to
// claude-hud's LineLayoutType "compact"; "expanded" to its "expanded".
func (m model) statusBlock(p palette, width int) string {
	badge := func(text, fg string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.base)).Background(lipgloss.Color(fg)).Bold(true).Padding(0, 1).Render(text)
	}
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(p.subtext))

	theme := themeName(m.dark)
	line1 := badge("liam", p.mauve) + " " +
		dim.Render(mockModel+"  •  ~/code/liam  •  🌿 main  •  mode: prompt")

	if m.statusLayout == statusCompact {
		return line1
	}

	// expanded: a second line with a claude-hud-style metrics bar.
	pct := 34
	filled := pct / 10
	barColor := p.green
	if pct >= 90 {
		barColor = p.red
	} else if pct >= 70 {
		barColor = p.yellow
	}
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(barColor)).Render(strings.Repeat("▓", filled)) +
		lipgloss.NewStyle().Foreground(lipgloss.Color(p.overlay)).Render(strings.Repeat("░", 10-filled))
	line2 := bar + dim.Render(fmt.Sprintf(" %d%% ctx  •  3 tools run  •  ⏱ 2m14s  •  %s", pct, theme))

	return line1 + "\n" + line2
}

func (m model) inputLine(p palette, width int) string {
	box := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.text)).
		Background(lipgloss.Color(p.surface)).
		Width(width-4).
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

// Variant A: conversation + inline tool calls + inline permission prompt
// stacked in one continuous column, with a customizable status block
// pinned to the bottom (just above input) — matching where Claude Code's
// own statusline actually sits, not the top-of-screen placement most TUIs
// default to. Press 's' to cycle status layouts (compact/expanded), the
// prototype's stand-in for the fact that this would be a user-scriptable
// external-command hook in the real harness, the same shape as Claude
// Code's own `statusLine` config: a command gets session JSON on stdin,
// prints one or more lines to stdout, each line becomes a row.
func (m model) renderA() string {
	p := m.pal()
	w, h := m.dims()

	var lines []string
	for _, t := range mockConversation {
		lines = append(lines, renderTurn(p, t, w))
	}
	if m.showPrompt {
		lines = append(lines, "")
		lines = append(lines, m.permissionBox(p, w))
	}

	status := m.statusBlock(p, w)
	statusHeight := strings.Count(status, "\n") + 1
	convHeight := h - statusHeight - 1 // -1 for the input line

	content := padTo(strings.Join(lines, "\n"), w, convHeight)
	return content + "\n" + status + "\n" + m.inputLine(p, w)
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
	return m.statusBlock(p, w) + "\n\n" + row + "\n" + m.inputLine(p, w)
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
		Width(w-2).Padding(0, 1).Render(stripLabel + stripBody)

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
