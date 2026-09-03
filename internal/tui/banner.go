package tui

// This file implements liam's startup banner (issue #169, CONTEXT.md's
// Banner): the block-art logo (logo.go/logo_gen.go) beside three lines of
// session identity text, shown once as the transcript's first entry at
// session start and again immediately after /clear. bannerLines is the
// single shared construction helper backing both call sites (New()/Init()
// in tui.go, submit()'s "/clear" case) so they can never drift apart.

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/theme"
)

// bannerLogoGap is the fixed column gap between the block-art logo and the
// identity text beside it.
const bannerLogoGap = 2

// banner builds the startup banner's transcript lines from m's current
// state — pal, version, the loop's Provider name, reqModel, and cwd — the
// single call shared by Init()'s showBanner and submit()'s "/clear" case,
// so neither has to repeat bannerLines' argument list by hand.
func (m Model) banner() []line {
	return bannerLines(m.pal, m.version, bannerProviderName(m.loop.Provider), m.reqModel, m.cwd)
}

// bannerLines builds the startup banner's transcript lines: the logo and
// text joined side by side (vertically centered against whichever is
// taller), followed by exactly one blank line — the spec's "a single blank
// line separates the banner from whatever content follows it". Both are
// "raw"-role lines (renderLine's convention for content the caller has
// already fully styled), so they render verbatim rather than through
// renderLine's own per-role styling.
func bannerLines(pal theme.Palette, version, providerName, model, cwd string) []line {
	logo := lipgloss.NewStyle().PaddingRight(bannerLogoGap).Render(renderLogo())
	text := bannerText(pal, version, providerName, model, cwd)
	banner := lipgloss.JoinHorizontal(lipgloss.Center, logo, text)
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
