package tui

// This file implements liam's startup banner (issue #169, CONTEXT.md's
// Banner): a wordmark beside three lines of session identity text, shown
// once as the transcript's first entry at session start and again
// immediately after /clear. bannerLines is the single shared construction
// helper backing both call sites (New()/Init() in tui.go, submit()'s
// "/clear" case) so they can never drift apart.
//
// The spec's first choice was a block-art logo decoded from assets/
// liam.png; that was tried (see this file's git history) and dropped after
// visual review found it illegible at a compact size and noisy at a
// legible one — the spec explicitly calls a plain bold wordmark instead of
// art "an accepted outcome, not a defect" for exactly this case.

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/theme"
)

// wordmarkColor is liam's fixed brand color (sampled from assets/liam.png),
// used for the banner's wordmark regardless of the active theme.Palette —
// the one deliberate exception to every other themed color in this
// package, per the spec's "not remapped to the active Catppuccin theme
// palette".
const wordmarkColor = "#7D6249"

// bannerWordmarkGap is the fixed column gap between the wordmark and the
// identity text beside it.
const bannerWordmarkGap = 2

// banner builds the startup banner's transcript lines from m's current
// state — pal, version, the loop's Provider name, reqModel, and cwd — the
// single call shared by Init()'s showBanner and submit()'s "/clear" case,
// so neither has to repeat bannerLines' argument list by hand.
func (m Model) banner() []line {
	return bannerLines(m.pal, m.version, bannerProviderName(m.loop.Provider), m.reqModel, m.cwd)
}

// bannerLines builds the startup banner's transcript lines: the wordmark
// and text joined side by side (vertically centered against whichever is
// taller), followed by exactly one blank line — the spec's "a single blank
// line separates the banner from whatever content follows it". Both are
// "raw"-role lines (renderLine's convention for content the caller has
// already fully styled), so they render verbatim rather than through
// renderLine's own per-role styling.
func bannerLines(pal theme.Palette, version, providerName, model, cwd string) []line {
	wordmark := lipgloss.NewStyle().PaddingRight(bannerWordmarkGap).
		Foreground(lipgloss.Color(wordmarkColor)).Bold(true).Render("Liam")
	text := bannerText(pal, version, providerName, model, cwd)
	banner := lipgloss.JoinHorizontal(lipgloss.Center, wordmark, text)
	return []line{{role: "raw", text: banner}, {role: "raw", text: ""}}
}

// bannerText renders the banner's three identity lines: "Liam" (bold) plus
// version, provider paired with model, and cwd — CONTEXT.md's Banner
// definition verbatim.
func bannerText(pal theme.Palette, version, providerName, model, cwd string) string {
	name := lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Text)).Bold(true).Render("Liam")
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Subtext))
	lines := []string{
		name + " " + version,
		dim.Render(providerName + " · " + model),
		dim.Render(cwd),
	}
	return strings.Join(lines, "\n")
}

// bannerProviderName reports p.Name(), or "" when p is nil — every existing
// New(...) test call site passes an agent.Loop{} with no Provider set, and
// the banner must build cleanly from it rather than panicking on a nil
// interface call.
func bannerProviderName(p provider.Provider) string {
	if p == nil {
		return ""
	}
	return p.Name()
}
