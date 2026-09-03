package tui

// This file implements liam's startup banner (issue #169, CONTEXT.md's
// Banner): three lines of session identity text, shown once as the
// transcript's first entry at session start and again immediately after
// /clear. bannerLines is the single shared construction helper backing
// both call sites (New()/Init() in tui.go, submit()'s "/clear" case) so
// they can never drift apart.
//
// Three logo/wordmark treatments beside the text were tried and dropped
// (see this file's git history): a block-art rendering decoded from
// assets/liam.png at two resolutions, then a hand-drawn block-letter
// wordmark styled after charmbracelet/crush's own technique. All three
// were rejected on visual review in favor of the plain text alone.

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/theme"
)

// banner builds the startup banner's transcript lines from m's current
// state — pal, version, the loop's Provider name, reqModel, and cwd — the
// single call shared by showBannerOnce and submit()'s "/clear" case, so
// neither has to repeat bannerLines' argument list by hand.
func (m Model) banner() []line {
	return bannerLines(m.pal, m.version, bannerProviderName(m.loop.Provider), m.reqModel, m.cwd)
}

// showBannerOnce prepends the startup banner to m.lines using m's current
// theme.Palette, but only the first time it's called per session: in
// theme.mode "auto", Init() races BackgroundColorMsg against
// defaultBannerTimeout's fallback (see Init's doc comment in tui.go), and
// bannerShown guards against both of them actually showing it. Pointer
// receiver so the mutation lands on the caller's own m — every call site is
// already inside Update, which discards nothing.
func (m *Model) showBannerOnce() {
	if m.bannerShown {
		return
	}
	m.bannerShown = true
	m.lines = append(m.banner(), m.lines...)
}

// bannerLines builds the startup banner's transcript lines: the identity
// text, followed by exactly one blank line — the spec's "a single blank
// line separates the banner from whatever content follows it". Both are
// "raw"-role lines (renderLine's convention for content the caller has
// already fully styled), so they render verbatim rather than through
// renderLine's own per-role styling.
func bannerLines(pal theme.Palette, version, providerName, model, cwd string) []line {
	text := bannerText(pal, version, providerName, model, cwd)
	return []line{{role: "raw", text: text}, {role: "raw", text: ""}}
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
