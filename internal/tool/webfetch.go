package tool

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// webFetchTimeout bounds a single fetch — the internal timeout guard
	// issue #50's spec calls for, since web_fetch does no robots.txt check.
	webFetchTimeout = 15 * time.Second
	// webFetchMaxBytes caps how much of a response body is read, guarding
	// against a huge response — the internal response-size cap issue #50's
	// spec calls for. Well above outputCap: Markdown conversion needs the
	// full (bounded) document before truncate() cuts the final output.
	webFetchMaxBytes = 2 << 20 // 2 MiB
)

// WebFetch fetches a URL and converts it to Markdown (issue #50). The model
// sees only url. No robots.txt check — this is a user-directed fetch, not a
// crawl — but webFetchTimeout and webFetchMaxBytes guard against a huge or
// slow response.
type WebFetch struct {
	// Client overrides the http.Client used to fetch; nil means a client
	// with webFetchTimeout.
	Client *http.Client
}

func (WebFetch) Name() string { return "web_fetch" }
func (WebFetch) Description() string {
	return "Fetch a URL and return its content as Markdown. Output over ~8000 bytes is truncated."
}

func (WebFetch) Parameters() Schema {
	return Schema{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to fetch.",
			},
		},
		"required": []string{"url"},
	}
}

func (WebFetch) Safety() Safety {
	return Safety{SideEffect: SideEffectNetwork}
}

func (f WebFetch) Run(ctx context.Context, args map[string]any) Result {
	rawURL, ok := args["url"].(string)
	if !ok || rawURL == "" {
		return errorResult(`web_fetch: "url" argument is required`)
	}

	ctx, cancel := context.WithTimeout(ctx, webFetchTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return errorResult(fmt.Sprintf("web_fetch: invalid url: %v", err))
	}

	resp, err := defaultHTTPClient(f.Client, webFetchTimeout).Do(httpReq)
	if err != nil {
		return errorResult(fmt.Sprintf("web_fetch: %v", err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxBytes))
	if err != nil {
		return errorResult(fmt.Sprintf("web_fetch: read response: %v", err))
	}
	if resp.StatusCode != http.StatusOK {
		return errorResult(fmt.Sprintf("web_fetch: %s returned %s", rawURL, resp.Status))
	}

	content := string(body)
	if isHTMLContent(resp.Header.Get("Content-Type"), body) {
		md, err := htmlToMarkdown(body)
		if err != nil {
			return errorResult(fmt.Sprintf("web_fetch: convert html: %v", err))
		}
		content = md
	}

	return Result{Content: truncate(content)}
}

// isHTMLContent decides whether body should go through htmlToMarkdown:
// trust the Content-Type header when the server sent one, otherwise sniff
// body's own leading bytes for an HTML doctype/tag.
func isHTMLContent(contentType string, body []byte) bool {
	if contentType != "" {
		return strings.Contains(strings.ToLower(contentType), "html")
	}
	lower := bytes.ToLower(bytes.TrimSpace(body))
	return bytes.HasPrefix(lower, []byte("<!doctype html")) || bytes.HasPrefix(lower, []byte("<html"))
}
