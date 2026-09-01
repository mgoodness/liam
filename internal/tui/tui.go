// Package tui implements liam's interactive Bubbletea shell: the
// stacked-stream layout (conversation scrollback with inline tool calls,
// a bubbles/textarea input line), Catppuccin Frappe/Latte theming
// auto-detected at startup, and the /quit, /clear, /skills, /compact,
// /<skill-name>, Escape-cancel session commands wired to the agent loop's
// context.Context cancellation.
//
// The customizable statusLine (issue #60) is a status block pinned above
// the input line, refreshed on session start, after each response, after
// each tool call, and an optional timer — every trigger debounced at
// 300ms via statusGen/statusRefreshMsg/statusRenderedMsg's generation
// check, so a burst of triggers within the debounce window only actually
// runs the (potentially external-process) render once.
//
// Conversation-viewport scrolling (issue #59) is backed by bubbles/
// viewport: PageUp/PageDown/Ctrl+U/Ctrl+D and the mouse wheel call the
// viewport's scroll methods directly rather than going through its own
// default KeyMap, which binds j/k/h/l/Up/Down and would collide with
// typing and input-history recall. The viewport auto-follows the bottom
// as new content streams in; any other scroll pins the position, and End
// resumes auto-follow.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/mcp"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/render"
	"github.com/mgoodness/liam/internal/session"
	"github.com/mgoodness/liam/internal/skill"
	"github.com/mgoodness/liam/internal/statusline"
	"github.com/mgoodness/liam/internal/theme"
	"github.com/mgoodness/liam/internal/tool"
)

// line is one rendered row of the conversation scrollback.
type line struct {
	role string // "user" | "assistant" | "tool" | "system"
	text string
}

// streamMsg wraps one provider.Event streamed from an in-flight turn.
type streamMsg struct{ ev provider.Event }

// turnDoneMsg reports an in-flight turn's completion: agent.Loop.Run's
// return value, threaded back through the channel a turn's goroutine
// writes to.
type turnDoneMsg struct {
	messages []provider.Message
	err      error
}

// systemLineMsg appends a system-role scrollback line for an out-of-band
// notice — e.g. an MCP load timeout or per-server error — that isn't part
// of the streamed provider.Event sequence.
type systemLineMsg struct{ text string }

// Model is the Bubbletea model driving liam's interactive shell.
type Model struct {
	loop         agent.Loop
	reqModel     string // cfg.Provider.Model, passed through on every turn
	systemPrompt string // set via WithSystemPrompt, passed through on every turn
	sess         *session.Session
	skills       []skill.Skill // liam's discovered skill catalog, for /skills

	themeMode string // cfg.Theme.Mode: "auto" (default), "dark", "light"
	pal       theme.Palette

	lines []line
	// streaming accumulates the current turn's in-progress assistant text.
	// It must be a *strings.Builder, never a value: Model is copied by
	// value on every single Update/View call (Bubbletea's Elm
	// architecture, plus tea.Model interface boxing), and strings.Builder
	// panics ("illegal use of non-zero Builder copied by value") the
	// moment a write lands on a copy taken after an earlier write set its
	// internal self-pointer — see TestStreamingSurvivesModelCopiedByValue.
	streaming *strings.Builder

	viewport     viewport.Model // scrolls the read-only transcript (issue #59)
	followBottom bool           // true = auto-follow new content; false while a manual scroll has pinned the position away from the bottom

	input  textarea.Model
	width  int
	height int

	busy   bool
	cancel context.CancelFunc
	events chan tea.Msg // non-nil while a turn is in flight

	mcpLoader    mcp.ToolLoader // set via WithMCPLoader; nil means no mcpServers configured
	mcpAttempted bool           // guards waiting on mcpLoader to exactly the session's first turn

	findSearcher tool.FindSearcher // set via WithFindSearcher; nil disables "@"-file-reference autocomplete
	hist         history           // Up/Down input-recall stack (issue #58), session-only
	mention      mentionState      // in-progress "@"-file-reference autocomplete, if any

	cwd string // set via WithCwd; statusLine's "cwd" field and the built-in renderer's git-info root

	statusCfg      config.StatusLineConfig
	statusTracker  *statusline.Tracker // tool-call count/duration, reset alongside sess.Clear() on /clear
	statusLines    []string            // the status block's current rows, refreshed asynchronously
	statusGen      int                 // bumped by requestStatusRefresh; the debounce/staleness generation counter
	statusDebounce time.Duration       // defaultStatusDebounce in New(); tests may override to avoid a real sleep
}

// WithMCPLoader attaches loader, whose tools are waited for (bounded by
// mcp.DefaultLoadTimeout) and merged into the toolset on the session's
// first turn only — after which every subsequent turn uses the merged
// registry with no further wait. A nil loader (the default, unset) means
// no mcpServers are configured; every existing New(...) call site is
// unaffected.
func (m Model) WithMCPLoader(loader mcp.ToolLoader) Model {
	m.mcpLoader = loader
	return m
}

// WithSystemPrompt attaches prompt (issue #56's discovered AGENTS.md/
// LIAM.md project instructions), carried as every turn's
// provider.Request.SystemPrompt. A zero-value Model (no WithSystemPrompt
// call) sends no SystemPrompt, matching every existing New(...) call site.
func (m Model) WithSystemPrompt(prompt string) Model {
	m.systemPrompt = prompt
	return m
}

// WithFindSearcher attaches searcher, backing the "@"-file-reference
// autocomplete popup (issue #58) with the same searcher the find tool
// itself uses. A zero-value Model (no WithFindSearcher call) leaves it nil,
// disabling "@" autocomplete entirely — matching every existing New(...)
// call site.
func (m Model) WithFindSearcher(searcher tool.FindSearcher) Model {
	m.findSearcher = searcher
	return m
}

// WithCwd attaches cwd — the harness's working directory — used as
// statusLine's "cwd" field and to resolve git branch/dirty status for the
// built-in default renderer. A zero-value Model (no WithCwd call) leaves
// it "", matching every existing New(...) call site: the built-in
// renderer simply omits the cwd segment and git info then.
func (m Model) WithCwd(cwd string) Model {
	m.cwd = cwd
	return m
}

// New builds the initial Model for an interactive session. skills is
// liam's discovered skill catalog (nil if none), used by /skills. If
// loop.Hooks is set, its SessionID is pointed at the new session and its
// sessionStart hooks fire immediately. loop.Session is pointed at the new
// session too (issue #54's compaction wiring), and loop.ContextLookup is
// set from loop.Provider when it also implements session.ContextLookup
// (true for the real openrouter.Provider) — enabling the ~85%
// auto-compaction trigger; a fake Provider in tests that doesn't implement
// it simply leaves auto-compaction disabled.
func New(loop agent.Loop, cfg config.Config, skills []skill.Skill) Model {
	mode := cfg.Theme.Mode

	sess := session.New()
	loop.Session = sess
	if lookup, ok := loop.Provider.(session.ContextLookup); ok {
		loop.ContextLookup = lookup
	}
	startSession(loop, sess)

	m := Model{
		loop:      loop,
		reqModel:  cfg.Provider.Model,
		sess:      sess,
		skills:    skills,
		themeMode: mode,
		// Assume dark until/unless auto-detection says otherwise — matches
		// the spec's "default to dark (Frappe) on detection failure".
		pal:          theme.Resolve(mode, true),
		input:        newTextarea(),
		hist:         newHistory(),
		viewport:     viewport.New(),
		followBottom: true,
		streaming:    &strings.Builder{},

		statusCfg:      cfg.StatusLine,
		statusTracker:  statusline.NewTracker(),
		statusGen:      1, // Init() schedules gen 1's debounce tick directly, without mutating a throwaway Model copy
		statusDebounce: defaultStatusDebounce,
	}
	applyTextareaTheme(&m.input, m.pal)
	return m
}

// startSession points loop.Hooks (if set) at sess and fires sessionStart.
func startSession(loop agent.Loop, sess *session.Session) {
	if loop.Hooks == nil {
		return
	}
	loop.Hooks.SessionID = sess.ID
	loop.Hooks.SessionStart(context.Background())
}

// endSession fires loop.Hooks' sessionEnd (if set) for the session it's
// currently pointed at. Only /clear calls this directly — it's a genuine
// mid-program session boundary; program exit's own sessionEnd is instead a
// single structural guarantee in runInteractive (main.go), fired once after
// p.Run() returns rather than threaded into every quit path here.
func endSession(loop agent.Loop) {
	if loop.Hooks == nil {
		return
	}
	loop.Hooks.SessionEnd(context.Background())
}

func newTextarea() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Type a message… (Enter to send, Shift+Enter/Ctrl+J for a newline)"
	ta.Prompt = "> "
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 8
	// Enter submits (handled at the top-level Update, never forwarded
	// here); Shift+Enter inserts a newline, with Ctrl+J as the documented
	// fallback for terminals that can't distinguish Shift+Enter from Enter.
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "ctrl+j"))
	ta.Focus()
	return ta
}

// Init requests the terminal's background color for theme auto-detection
// (unless theme.mode already forces dark/light), and kicks off statusLine's
// session-start refresh trigger — plus its optional periodic timer, when
// config.StatusLineConfig.RefreshInterval is configured. m.statusGen is
// already 1 by construction (see New()), so this schedules that same
// generation's debounce tick directly rather than going through
// requestStatusRefresh: Init has a value receiver, and any mutation it
// made to its own m would be discarded rather than persisted into the
// Model Bubbletea actually keeps.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.statusTick(m.statusGen)}
	if m.themeMode != "dark" && m.themeMode != "light" {
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	if t := m.scheduleStatusTimer(); t != nil {
		cmds = append(cmds, t)
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(m.width)
		m.refreshViewport()
		return m, nil

	case tea.BackgroundColorMsg:
		m.pal = theme.Resolve(m.themeMode, msg.IsDark())
		applyTextareaTheme(&m.input, m.pal)
		m.refreshViewport()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseWheelMsg:
		m.syncViewportDims()
		before := m.viewport.YOffset()
		m.viewport, _ = m.viewport.Update(msg)
		m.pinIfScrolled(before)
		return m, nil

	case streamMsg:
		refresh := m.handleEvent(msg.ev)
		m.refreshViewport()
		cmd := waitForMsg(m.events)
		if refresh {
			// Batched, not run in the linear streamMsg/waitForMsg chain
			// itself: the refresh trigger ("after each response"/"after
			// each tool call") is independent of waiting for the next
			// provider.Event, and must never delay it.
			cmd = tea.Batch(cmd, m.requestStatusRefresh())
		}
		return m, cmd

	case turnDoneMsg:
		m.finishTurn(msg.messages, msg.err)
		m.refreshViewport()
		return m, m.requestStatusRefresh()

	case compactDoneMsg:
		m.busy = false
		m.cancel = nil
		switch {
		case msg.canceled:
			m.lines = append(m.lines, line{role: "system", text: "[interrupted]"})
		case msg.ok:
			m.sess.Messages = msg.messages
			m.lines = append(m.lines, line{role: "info", text: "Conversation compacted."})
		default:
			m.lines = append(m.lines, line{role: "info", text: "Nothing to compact."})
		}
		m.refreshViewport()
		return m, m.requestStatusRefresh()

	case systemLineMsg:
		m.lines = append(m.lines, line{role: "system", text: msg.text})
		m.refreshViewport()
		return m, waitForMsg(m.events)

	case statusRefreshMsg:
		if msg.gen != m.statusGen {
			return m, nil // superseded by a newer request — the debounce itself
		}
		return m, m.statusRefreshCmd(msg.gen)

	case statusRenderedMsg:
		if msg.gen != m.statusGen {
			return m, nil // a newer refresh was requested while this one was rendering
		}
		m.statusLines = msg.lines
		if msg.warn != "" {
			m.lines = append(m.lines, line{role: "system", text: msg.warn})
		}
		m.refreshViewport()
		return m, nil

	case statusTimerMsg:
		return m, tea.Batch(m.requestStatusRefresh(), m.scheduleStatusTimer())
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// handleKey dispatches one key press. While the "@"-file-reference popup
// (m.mention) is active it owns Up/Down/Tab/Enter/Esc outright — history
// recall and turn-cancellation are suspended until it closes. Otherwise
// Up/Down are routed to history recall or ordinary cursor movement by
// handleUp/handleDown, and every other key falls through to the textarea,
// after which the mention state is recomputed from the new cursor position.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mention.active {
		switch msg.String() {
		case "up":
			m.mention.selected = (m.mention.selected - 1 + len(m.mention.matches)) % max(1, len(m.mention.matches))
			return m, nil
		case "down":
			m.mention.selected = (m.mention.selected + 1) % max(1, len(m.mention.matches))
			return m, nil
		case "tab", "enter":
			return m.selectMention(), nil
		case "esc":
			m.mention = mentionState{}
			return m, nil
		}
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.cancelTurn()
		return m, nil
	case "enter":
		return m.submit()
	case "up":
		return m.handleUp(msg)
	case "down":
		return m.handleDown(msg)
	case "pgup":
		m.scrollViewport(m.viewport.PageUp)
		return m, nil
	case "pgdown":
		m.scrollViewport(m.viewport.PageDown)
		return m, nil
	case "ctrl+u":
		m.scrollViewport(m.viewport.HalfPageUp)
		return m, nil
	case "ctrl+d":
		m.scrollViewport(m.viewport.HalfPageDown)
		return m, nil
	case "end":
		m.syncViewportDims()
		m.viewport.GotoBottom()
		m.followBottom = true
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.updateMention()
	return m, cmd
}

// handleUp recalls the previous history entry when the input is empty or
// the cursor sits on the textarea's first line; otherwise it moves the
// cursor up normally, the standard resolution to the conflict between
// multi-line editing and history navigation (issue #58).
func (m Model) handleUp(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	atBoundary := m.input.Value() == "" || m.input.Line() == 0
	cmd := m.recallOrMove(msg, atBoundary, func(h *history) (string, bool) { return h.up(m.input.Value()) })
	return m, cmd
}

// handleDown is handleUp's Down-key counterpart: it recalls the next-newer
// history entry (or the preserved draft) when the input is empty or the
// cursor sits on the textarea's last line; otherwise it moves the cursor
// down normally.
func (m Model) handleDown(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	atBoundary := m.input.Value() == "" || m.input.Line() == m.input.LineCount()-1
	cmd := m.recallOrMove(msg, atBoundary, func(h *history) (string, bool) { return h.down() })
	return m, cmd
}

// recallOrMove is handleUp/handleDown's shared boundary-check shape: at the
// history-recall boundary it applies recall's result to the input (a no-op
// when recall reports nothing to change), otherwise it forwards msg to the
// textarea for ordinary cursor movement. Pointer receiver so the mutation
// lands on the caller's own m, not a copy — recall closes over m.input to
// read the pre-recall draft, which only holds if it's the same m handleUp/
// handleDown go on to return.
func (m *Model) recallOrMove(msg tea.KeyPressMsg, atBoundary bool, recall func(*history) (string, bool)) tea.Cmd {
	if atBoundary {
		if val, ok := recall(&m.hist); ok {
			m.input.SetValue(val)
		}
		return nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

// syncViewportDims recomputes the viewport's width/height from the
// terminal size and the input's current row count. It's called before
// every scroll action — not just on resize — because the textarea's height
// changes as the user types multi-line input, and paging math (and the
// AtBottom check scrollViewport relies on) needs to measure against the
// layout currently on screen. The "-1" reserves the single blank
// separator row baked into renderTranscript's trailing newline; the
// statusLine block (when it has any rows) reserves one row per line on
// top of that, pinned above the input per spec.
func (m *Model) syncViewportDims() {
	m.viewport.SetWidth(m.width)
	m.viewport.SetHeight(max(0, m.height-m.input.Height()-1-len(m.statusLines)))
}

// refreshViewport rebuilds the viewport's content from the current
// scrollback, first resyncing dimensions — the input's height (and so the
// viewport's own height) can have changed since the last resize event, and
// GotoBottom must land against the height actually on screen or the
// transcript stops tracking the true last line. While auto-follow is
// active (m.followBottom, the default) this also re-pins the view to the
// bottom so streamed content stays visible; once a manual scroll has
// pinned the position (m.followBottom == false), the offset is left alone
// until End resumes auto-follow.
func (m *Model) refreshViewport() {
	m.syncViewportDims()
	m.viewport.SetContent(m.renderTranscript())
	if m.followBottom {
		m.viewport.GotoBottom()
	}
}

// pinIfScrolled re-derives m.followBottom from where the scroll actually
// landed — left alone on a no-op scroll (e.g. PageDown pressed while
// already at the bottom, so an accidental extra keypress can't spuriously
// break auto-follow), otherwise pinned false unless the scroll happened to
// land exactly back at the bottom row, in which case auto-follow resumes.
// This mirrors End's own "true" assignment (handleKey's "end" case) so a
// manual scroll that returns to the bottom behaves the same as explicitly
// pressing End, matching how chat/pager UIs conventionally resume
// following once the reader is back at the live edge.
func (m *Model) pinIfScrolled(before int) {
	if m.viewport.YOffset() == before {
		return
	}
	m.followBottom = m.viewport.AtBottom()
}

// scrollViewport re-syncs the viewport's dimensions, applies scroll, then
// pins via pinIfScrolled — the shared shape behind every key-driven scroll
// (PageUp/PageDown/Ctrl+U/Ctrl+D). The mouse wheel follows the same
// sync-then-pin shape but is handled inline in Update, since it needs
// viewport.Update(msg) rather than a bare method call.
func (m *Model) scrollViewport(scroll func()) {
	m.syncViewportDims()
	before := m.viewport.YOffset()
	scroll()
	m.pinIfScrolled(before)
}

// cancelTurn cancels the in-flight turn's context, if there is one — the
// single mechanism behind Escape-cancellation, unified across both a
// Provider.Stream call and a Tool.Run call since both already thread the
// same context.Context through.
func (m Model) cancelTurn() {
	if m.busy && m.cancel != nil {
		m.cancel()
	}
}

// submit handles an Enter press: /quit, /clear, /skills, /compact run
// immediately; /<skill-name> force-activates the skill and, if any text
// follows the name, immediately starts a turn with that text as the
// message (skillCommandName); an empty input or a turn already in flight
// is a no-op; anything else starts a new agent-loop turn.
func (m Model) submit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" || m.busy {
		return m, nil
	}
	m.input.Reset()
	m.hist.add(text)

	switch text {
	case "/quit":
		return m, tea.Quit
	case "/clear":
		endSession(m.loop)
		m.sess.Clear()
		startSession(m.loop, m.sess)
		m.statusTracker.Reset()
		m.lines = nil
		m.streaming.Reset()
		m.refreshViewport()
		return m, m.requestStatusRefresh()
	case "/skills":
		m.lines = append(m.lines, line{role: "info", text: render.SkillList(m.skills)})
		m.refreshViewport()
		return m, nil
	case "/compact":
		return m.compact()
	}
	// A skill named "quit", "clear", "skills", or "compact" is shadowed by
	// the reserved commands above and can only be reached via /skills'
	// listing + the model's own activate_skill — an accepted, narrow edge
	// case rather than one worth complicating dispatch over.
	if name, rest, ok := skillCommandName(text); ok {
		if s, found := skill.Find(m.skills, name); found {
			m.activateSkill(s)
			if rest == "" {
				return m, nil
			}
			// Trailing text after the skill name becomes the first
			// turn's message, sent immediately below — see
			// skillCommandName's doc comment.
			text = rest
		}
		// No matching skill: fall through and send text as an ordinary
		// chat message, same as any other unrecognized input — liam has
		// no fixed slash-command registry to validate against, so a
		// leading "/" alone isn't enough to treat this as a failed
		// command rather than a message that happens to start with one.
	}

	m.lines = append(m.lines, line{role: "user", text: text})
	m.sess.Messages = append(m.sess.Messages, provider.Message{Role: "user", Content: text})
	m.refreshViewport()

	req := provider.Request{
		Model:        m.reqModel,
		SystemPrompt: m.systemPrompt,
		Messages:     append([]provider.Message(nil), m.sess.Messages...),
	}

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan tea.Msg, 1)
	m.busy = true
	m.cancel = cancel
	m.events = events

	// mcpLoader is waited on (bounded by mcp.DefaultLoadTimeout) inside
	// runTurn's own background goroutine, never here — submit() runs on
	// Bubbletea's main Update loop, so blocking here would freeze the UI.
	// mcpAttempted guards this to the session's first turn only; loop.Tools
	// is a map (a reference type), so runTurn's merge into it is visible to
	// every later turn without needing to thread anything back through m.
	waitForMCP := !m.mcpAttempted && m.mcpLoader != nil
	m.mcpAttempted = true

	loop := m.loop
	loader := m.mcpLoader
	go runTurn(ctx, loop, req, events, loader, waitForMCP)

	return m, waitForMsg(events)
}

// runTurn drives one agent.Loop turn in the background, forwarding every
// streamed Event and the final result over events for Update to pick up.
// When waitForMCP is set, loader's tools are waited for (bounded by
// mcp.DefaultLoadTimeout, or by ctx — e.g. Escape-cancellation) and merged
// into loop.Tools before the turn runs, with any timeout/per-server error
// sent back as a systemLineMsg.
func runTurn(ctx context.Context, loop agent.Loop, req provider.Request, events chan<- tea.Msg, loader mcp.ToolLoader, waitForMCP bool) {
	if waitForMCP && loader != nil {
		if loop.Tools == nil {
			loop.Tools = tool.NewRegistry()
		}
		mcp.Merge(ctx, loop.Tools, loader, func(msg string) {
			events <- systemLineMsg{text: msg}
		})
	}

	messages, err := loop.Run(ctx, req, func(ev provider.Event) {
		events <- streamMsg{ev: ev}
	})
	events <- turnDoneMsg{messages: messages, err: err}
}

func waitForMsg(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

// handleEvent folds one streamed provider.Event into the conversation
// scrollback: text deltas accumulate, a tool call flushes any accumulated
// text (it belongs to the same assistant turn as the call), and a tool
// result renders as its own inline line via the shared render.ToolCall
// convention.
// handleEvent reports whether ev should trigger a statusLine refresh — true
// for a completed tool call and a completed response, matching the spec's
// "after each tool call"/"after each response" triggers; a text delta or a
// requested-but-not-yet-resolved tool call don't.
func (m *Model) handleEvent(ev provider.Event) bool {
	switch e := ev.(type) {
	case provider.TextDeltaEvent:
		m.streaming.WriteString(e.Text)
	case provider.ToolCallEvent:
		m.flushStreaming()
	case provider.ToolResultEvent:
		m.flushStreaming()
		m.lines = append(m.lines, line{role: "tool", text: render.ToolCall(e.Name, e.ArgsJSON, e.Content, e.IsError)})
		m.statusTracker.RecordToolCall()
		return true
	case provider.DoneEvent:
		m.sess.Record(e.ModelUsed, e.Usage)
		return true
	}
	return false
}

func (m *Model) flushStreaming() {
	if m.streaming.Len() == 0 {
		return
	}
	m.lines = append(m.lines, line{role: "assistant", text: m.streaming.String()})
	m.streaming.Reset()
}

// finishTurn handles a turn's end: a canceled context (Escape) marks the
// turn "[interrupted]" while preserving whatever partial output already
// made it into the scrollback; any other error gets an "[error: ...]"
// marker, matching the same convention. Either way m.sess.Messages is
// replaced with the loop's returned history, which already includes
// whatever partial assistant/tool output survived.
func (m *Model) finishTurn(messages []provider.Message, err error) {
	m.flushStreaming()
	m.busy = false
	m.cancel = nil
	m.events = nil
	m.sess.Messages = messages

	switch {
	case err == nil:
	case errors.Is(err, context.Canceled):
		m.lines = append(m.lines, line{role: "system", text: "[interrupted]"})
	default:
		m.lines = append(m.lines, line{role: "system", text: fmt.Sprintf("[error: %v]", err)})
	}
}

// renderTranscript renders the full scrollback — every line plus any
// in-progress streaming text — into a single palette-styled, width-clamped
// string: the content refreshViewport feeds to m.viewport. Each appended
// line ends in "\n", so the string always carries one trailing blank row;
// that row is what separates the transcript from the input below it.
func (m Model) renderTranscript() string {
	var b strings.Builder
	for _, l := range m.lines {
		b.WriteString(renderLine(m.pal, l))
		b.WriteString("\n")
	}
	if m.streaming.Len() > 0 {
		b.WriteString(renderLine(m.pal, line{role: "assistant", text: m.streaming.String()}))
		b.WriteString("\n")
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.pal.Text)).
		Background(lipgloss.Color(m.pal.Base)).
		Width(m.width).
		Render(b.String())
}

func (m Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("")
	}

	m.syncViewportDims()
	content := m.viewport.View() + "\n" + m.renderStatusBlock() + m.input.View()
	if m.mention.active {
		content += "\n" + renderMentionPopup(m.pal, m.mention)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.BackgroundColor = lipgloss.Color(m.pal.Base)
	v.MouseMode = tea.MouseModeCellMotion
	// Offset the textarea's own cursor position by the viewport's fixed
	// row count, the blank separator line, and the statusLine block's own
	// row count. This doesn't account for line-wrapped rows (no
	// width-aware wrapping yet), so a very long unwrapped line can throw
	// it off; harmless beyond a cosmetic cursor-position glitch.
	if cur := m.input.Cursor(); cur != nil {
		v.Cursor = tea.NewCursor(cur.X, cur.Y+m.viewport.Height()+1+len(m.statusLines))
	}
	return v
}

func renderLine(p theme.Palette, l line) string {
	switch l.role {
	case "user":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Blue)).Bold(true).Render("› " + l.text)
	case "assistant":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text)).Render(l.text)
	case "tool":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext)).Italic(true).Render("  ⚙ " + l.text)
	case "system":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Red)).Render(l.text)
	case "info":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Mauve)).Render(l.text)
	default:
		return l.text
	}
}

func applyTextareaTheme(ta *textarea.Model, p theme.Palette) {
	s := textarea.DefaultStyles(p.Dark)
	for _, state := range []*textarea.StyleState{&s.Focused, &s.Blurred} {
		state.Base = state.Base.Background(lipgloss.Color(p.Surface)).Foreground(lipgloss.Color(p.Text))
		state.Text = state.Text.Foreground(lipgloss.Color(p.Text))
		state.Prompt = state.Prompt.Foreground(lipgloss.Color(p.Green)).Bold(true)
		state.Placeholder = state.Placeholder.Foreground(lipgloss.Color(p.Overlay))
	}
	ta.SetStyles(s)
}
