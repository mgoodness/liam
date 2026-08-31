package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mgoodness/liam/internal/tool"
)

// fffMCPCommand is the fff-mcp binary name, auto-detected on $PATH.
// liam's find/grep tools (ticket #49) connect to it over an internal MCP
// connection that's never listed in mcpServers config and never surfaced
// to the model as a server or tool name of its own — it only ever backs
// tool.Find/tool.Grep, falling back to tool.StdlibSearch when absent
// (ticket #18's resolution).
const fffMCPCommand = "fff-mcp"

// fffFindTool and fffGrepTool are fff-mcp's own MCP tool names, verified
// live (ListTools against a locally installed fff-mcp v0.x, not from any
// doc) rather than trusted from ticket #18's resolution comment, which
// names them "ffgrep"/"fffind"/"fff-multi-grep" — stale, or describing a
// different fff-mcp version/integration than the Homebrew-installed
// binary this package actually spawns. maxResults, passed on every call
// below, is likewise verified live: both tools' inputSchema declare it
// (default 20 server-side; liam always requests tool.MaxSearchResults).
const (
	fffFindTool = "find_files"
	fffGrepTool = "grep"
)

// DetectFFF reports whether fff-mcp is available on $PATH.
func DetectFFF() bool {
	_, err := exec.LookPath(fffMCPCommand)
	return err == nil
}

// FFF is find/grep's fff-mcp-backed searcher, satisfying both
// tool.FindSearcher and tool.GrepSearcher.
type FFF struct {
	session *sdkmcp.ClientSession
}

// StartFFF spawns "fff-mcp <dir>" and connects to it over stdio, reusing
// the same MCP client machinery as liam's user-configured mcpServers
// (ticket #7/#15) — fff-mcp indexes dir once at startup and has no
// per-call directory argument, so liam's find/grep tools are always
// scoped to dir (liam's own working directory in production).
func StartFFF(ctx context.Context, dir string) (*FFF, error) {
	cmd := exec.Command(fffMCPCommand, dir)
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "liam", Version: "dev"}, &sdkmcp.ClientOptions{Capabilities: clientCapabilities})
	session, err := client.Connect(ctx, &sdkmcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: connecting to fff-mcp: %w", err)
	}
	return &FFF{session: session}, nil
}

// Find implements tool.FindSearcher via fff-mcp's find_files tool.
func (f *FFF) Find(ctx context.Context, query string) ([]string, int, error) {
	text, err := f.call(ctx, fffFindTool, map[string]any{"query": query, "maxResults": tool.MaxSearchResults})
	if err != nil {
		return nil, 0, err
	}
	paths, total := parseFFFFindOutput(text)
	return paths, total, nil
}

// Grep implements tool.GrepSearcher via fff-mcp's grep tool.
func (f *FFF) Grep(ctx context.Context, query string) ([]tool.GrepMatch, int, error) {
	text, err := f.call(ctx, fffGrepTool, map[string]any{"query": query, "maxResults": tool.MaxSearchResults})
	if err != nil {
		return nil, 0, err
	}
	matches, total := parseFFFGrepOutput(text)
	return matches, total, nil
}

// call invokes name on the fff-mcp session and concatenates its
// TextContent blocks via textContent, matching mcpTool.Run's own
// convention (internal/mcp/tool.go).
func (f *FFF) call(ctx context.Context, name string, args map[string]any) (string, error) {
	res, err := f.session.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", fmt.Errorf("mcp: calling fff-mcp %s: %w", name, err)
	}

	text := textContent(res.Content)
	if res.IsError {
		return "", fmt.Errorf("fff-mcp %s: %s", name, text)
	}
	return text, nil
}

// fffHeaderRe matches fff-mcp's own "<shown>/<total> matches[ shown]" or
// "<shown>/<total> results" header line, printed only when it capped its
// own results — an uncapped result has no header line at all.
var fffHeaderRe = regexp.MustCompile(`^(\d+)/(\d+) (?:matches(?: shown)?|results)$`)

// fffMatchLineRe matches one of fff-mcp grep's own " <line>: <text>" rows.
var fffMatchLineRe = regexp.MustCompile(`^ *(\d+): (.*)$`)

// fffPathTagRe strips the trailing metadata tag (e.g. "git:clean") fff-mcp
// appends to each find_files result line.
var fffPathTagRe = regexp.MustCompile(`^(.+?)\s+git:\S+$`)

// fffHeaderTotal reports the total fff-mcp printed in a "<shown>/<total>
// ..." header line, and whether line was one — shared by
// parseFFFGrepOutput/parseFFFFindOutput, both of which fall back to their
// own result count when a header is absent (the uncapped case).
func fffHeaderTotal(line string) (total int, ok bool) {
	m := fffHeaderRe.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	t, err := strconv.Atoi(m[2])
	if err != nil {
		return 0, false
	}
	return t, true
}

// parseFFFGrepOutput turns fff-mcp grep's raw plain-text response into
// structured matches plus the total match count it reported, tolerating
// the extra lines fff-mcp sometimes prints around its results (a
// "→ Read <file> (only match)" hint, a "cursor: <n>" pagination line —
// not followed up on in v1). Any line that isn't recognized as one of
// those, the header, or a match row is treated as a new current file
// heading, matching fff-mcp's own grouped-by-file output shape.
func parseFFFGrepOutput(text string) (matches []tool.GrepMatch, total int) {
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "0 matches") {
		return nil, 0
	}

	haveHeader := false
	currentFile := ""
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "→ "), strings.HasPrefix(line, "cursor:"):
			continue
		}

		if t, ok := fffHeaderTotal(line); ok {
			total, haveHeader = t, true
			continue
		}

		if m := fffMatchLineRe.FindStringSubmatch(line); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				matches = append(matches, tool.GrepMatch{File: currentFile, Line: n, Text: m[2]})
			}
			continue
		}

		if strings.TrimSpace(line) != "" {
			currentFile = line
		}
	}

	if !haveHeader {
		total = len(matches)
	}
	return matches, total
}

// parseFFFFindOutput turns fff-mcp find_files' raw plain-text response
// into a path list plus the total result count it reported, mirroring
// parseFFFGrepOutput's tolerance of fff-mcp's extra lines.
func parseFFFFindOutput(text string) (paths []string, total int) {
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "0 results") {
		return nil, 0
	}

	haveHeader := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "cursor:") {
			continue
		}
		if t, ok := fffHeaderTotal(line); ok {
			total, haveHeader = t, true
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}

		path := line
		if m := fffPathTagRe.FindStringSubmatch(line); m != nil {
			path = m[1]
		}
		paths = append(paths, path)
	}

	if !haveHeader {
		total = len(paths)
	}
	return paths, total
}
