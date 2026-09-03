package tui

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/theme"
)

// TestBannerTextContainsIdentityFields covers CONTEXT.md's Banner
// definition: "Liam" + version, provider paired with model, and cwd all
// appear in the rendered text.
func TestBannerTextContainsIdentityFields(t *testing.T) {
	got := bannerText(theme.Frappe, "v1.2.3", "openrouter", "openrouter/auto", "/home/user/project")
	for _, want := range []string{"Liam", "v1.2.3", "openrouter", "openrouter/auto", "/home/user/project", "·"} {
		if !strings.Contains(got, want) {
			t.Errorf("bannerText(...) = %q, want it to contain %q", got, want)
		}
	}
}

// TestBannerTextBoldsLiam covers the spec's "Liam rendered bold" — the
// rendered text carries lipgloss's bold SGR sequence somewhere around the
// literal "Liam".
func TestBannerTextBoldsLiam(t *testing.T) {
	got := bannerText(theme.Frappe, "v1.2.3", "openrouter", "openrouter/auto", "/cwd")
	bold := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Frappe.Text)).Bold(true).Render("Liam")
	if !strings.Contains(got, bold) {
		t.Errorf("bannerText(...) = %q, want it to contain the bold-rendered %q", got, bold)
	}
}

// TestBannerTextFixedColorsIgnoreTheme covers the acceptance criterion that
// only the logo's colors are exempt from theming — the identity text itself
// still follows the active theme.Palette, so Frappe and Latte must render
// differently.
func TestBannerTextFixedColorsIgnoreTheme(t *testing.T) {
	frappe := bannerText(theme.Frappe, "v1", "p", "m", "/cwd")
	latte := bannerText(theme.Latte, "v1", "p", "m", "/cwd")
	if frappe == latte {
		t.Error("bannerText rendered identically for Frappe and Latte, want the identity text to follow the active palette")
	}
}

// TestBannerLinesEndsWithBlankSeparator covers "a single blank line
// separates the banner from whatever content follows it".
func TestBannerLinesEndsWithBlankSeparator(t *testing.T) {
	lines := bannerLines(theme.Frappe, "v1", "p", "m", "/cwd")
	if len(lines) < 2 {
		t.Fatalf("bannerLines(...) returned %d lines, want at least 2 (banner + blank separator)", len(lines))
	}
	last := lines[len(lines)-1]
	if last.text != "" {
		t.Errorf("last bannerLines() line = %q, want empty (the blank separator)", last.text)
	}
}

// TestBannerLinesAreRawRole covers renderLine's contract: banner content is
// already fully styled (bold "Liam", dimmed metadata, fixed-color logo), so
// it must use the "raw" role to render verbatim rather than being
// re-styled by renderLine's per-role switch.
func TestBannerLinesAreRawRole(t *testing.T) {
	for _, l := range bannerLines(theme.Frappe, "v1", "p", "m", "/cwd") {
		if l.role != "raw" {
			t.Errorf("bannerLines() line role = %q, want %q", l.role, "raw")
		}
	}
}

// TestBannerProviderNameNilSafe covers the guard against agent.Loop{}'s
// zero-value Provider (every existing New(...) test call site) — the
// banner must build cleanly rather than panic on a nil interface call.
func TestBannerProviderNameNilSafe(t *testing.T) {
	if got := bannerProviderName(nil); got != "" {
		t.Errorf("bannerProviderName(nil) = %q, want empty", got)
	}
}

// TestBannerProviderNameReportsProviderName covers the ordinary path: a
// configured Provider's Name() reaches the banner unchanged.
func TestBannerProviderNameReportsProviderName(t *testing.T) {
	fp := &multiCallProvider{}
	if got, want := bannerProviderName(fp), fp.Name(); got != want {
		t.Errorf("bannerProviderName(fp) = %q, want %q", got, want)
	}
}

// TestInitShowsBannerAsFirstLine covers the top-level AC: a new session's
// Init(), driven through Update the same way a real tea.Program would, ends
// up with the banner as m.lines[0] before any user input.
func TestInitShowsBannerAsFirstLine(t *testing.T) {
	fp := &multiCallProvider{}
	m := New(agent.Loop{Provider: fp}, config.Config{Provider: config.ProviderConfig{Model: "openrouter/auto"}}, nil).
		WithVersion("v9.9.9").
		WithCwd("/some/project")
	m.statusDebounce = 0 // avoid a real 300ms sleep when a batched cmd is invoked below
	m.bannerTimeout = 0  // ditto — theme.mode defaults to "auto" here, so Init() batches bannerTimeoutCmd()

	msg := m.Init()()
	bm, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init() produced %#v, want a tea.BatchMsg", msg)
	}

	var found bool
	for _, c := range bm {
		if bmsg, ok := c().(bannerMsg); ok {
			next, _ := m.Update(bmsg)
			m = next.(Model)
			found = true
		}
	}
	if !found {
		t.Fatal("Init()'s batch never produced a bannerMsg")
	}

	if len(m.lines) == 0 {
		t.Fatal("m.lines is empty after the bannerMsg round trip")
	}
	first := m.lines[0].text
	for _, want := range []string{"Liam", "v9.9.9", fp.Name(), "openrouter/auto", "/some/project"} {
		if !strings.Contains(first, want) {
			t.Errorf("m.lines[0] = %q, want it to contain %q", first, want)
		}
	}
}

// TestBackgroundColorMsgShowsBannerWithResolvedPalette covers the theme-
// detection race fix: in theme.mode "auto", the banner must reflect
// BackgroundColorMsg's resolved palette (light, here), not New()'s
// dark-assumed placeholder — the bug this guards against would have baked
// the banner's colors in before detection ever ran.
func TestBackgroundColorMsgShowsBannerWithResolvedPalette(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil).WithCwd("/cwd")
	if !m.pal.Dark {
		t.Fatal("pal.Dark = false before any BackgroundColorMsg, want the dark-assumed default (test's own premise)")
	}

	next, _ := m.Update(tea.BackgroundColorMsg{Color: color.White})
	mm := next.(Model)

	if len(mm.lines) == 0 {
		t.Fatal("m.lines is empty after BackgroundColorMsg")
	}
	want := bannerText(theme.Latte, mm.version, "", mm.reqModel, mm.cwd)
	if !strings.Contains(mm.lines[0].text, want) {
		t.Errorf("m.lines[0] = %q, want it to contain the light-palette banner text %q", mm.lines[0].text, want)
	}
}

// TestBannerTimeoutFallbackShowsBannerOnce covers Init's fallback path (a
// terminal that never answers the background-color query) and the
// bannerShown guard: a fallback bannerMsg shows the banner using whatever
// palette is current, and a BackgroundColorMsg arriving afterward updates
// the palette without appending a second banner.
func TestBannerTimeoutFallbackShowsBannerOnce(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)

	next, _ := m.Update(bannerMsg{})
	mm := next.(Model)
	if len(mm.lines) == 0 {
		t.Fatal("m.lines is empty after the fallback bannerMsg")
	}

	next, _ = mm.Update(tea.BackgroundColorMsg{Color: color.White})
	mm = next.(Model)
	if mm.pal.Dark {
		// Not itself a bug — this documents the accepted tradeoff of a
		// terminal slower than defaultBannerTimeout: the palette does
		// still update for everything rendered after this point, only the
		// already-shown banner keeps the fallback's colors.
		t.Fatal("pal.Dark = true after a light BackgroundColorMsg, want it still updated even though the banner already showed")
	}
	count := 0
	for _, l := range mm.lines {
		if strings.Contains(l.text, "Liam") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("banner appears %d times after a late BackgroundColorMsg, want exactly 1 (bannerShown guard)", count)
	}
}

// TestSubmitOrdinaryTurnDoesNotRepeatBanner covers the AC "the banner
// appears exactly once per session start (not repeated on ordinary turns)":
// a normal submit() call must never itself append another banner.
func TestSubmitOrdinaryTurnDoesNotRepeatBanner(t *testing.T) {
	fp := &multiCallProvider{turns: [][]provider.Event{
		{provider.TextDeltaEvent{Text: "hi"}, provider.DoneEvent{FinishReason: "stop"}},
	}}
	m := New(agent.Loop{Provider: fp}, config.Config{}, nil)
	m.lines = bannerLines(m.pal, "v1", "p", "m", "/cwd")
	m.input.SetValue("hello")

	next, cmd := m.submit()
	mm := drain(t, next.(Model), cmd)

	count := 0
	for _, l := range mm.lines {
		if strings.Contains(l.text, "Liam") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("banner appears %d times in m.lines after an ordinary turn, want exactly 1", count)
	}
}

// TestWithVersionSetsField covers the With* option's own contract: a
// zero-value Model (no WithVersion call) leaves version "".
func TestWithVersionSetsField(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	if m.version != "" {
		t.Fatalf("version = %q before WithVersion, want empty", m.version)
	}
	m = m.WithVersion("v1.0.0")
	if m.version != "v1.0.0" {
		t.Errorf("version after WithVersion(%q) = %q, want %q", "v1.0.0", m.version, "v1.0.0")
	}
}
