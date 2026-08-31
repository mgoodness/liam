package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// exaDefaultBaseURL is Exa's API origin. WebSearch.BaseURL overrides
	// this for tests, pointing at an httptest.Server instead.
	exaDefaultBaseURL = "https://api.exa.ai"
	// exaSearchTimeout bounds a single Exa call, used as the default
	// http.Client's timeout when WebSearch.Client is nil.
	exaSearchTimeout = 15 * time.Second
	// webSearchMaxResults caps how many Exa results reach the model
	// (issue #50's spec).
	webSearchMaxResults = 5
	// webSearchSnippetCap caps each result's highlights/summary text at
	// ~500 chars (issue #50's spec) — Exa's own "text" (full page) field is
	// never requested or surfaced; web_fetch covers that.
	webSearchSnippetCap = 500
)

// WebSearch searches the web via Exa's /search API (issue #50). The model
// sees only query/category; the response surfaces only highlights/summary,
// capped at webSearchMaxResults results and webSearchSnippetCap chars each
// — never a result's full page text. main.go silently unregisters this
// tool when EXA_API_KEY is unset, per the spec.
type WebSearch struct {
	APIKey string
	// BaseURL overrides exaDefaultBaseURL; tests point it at an
	// httptest.Server instead of the live Exa API.
	BaseURL string
	// Client overrides the http.Client used to call Exa; nil means a
	// client with exaSearchTimeout.
	Client *http.Client
}

func (WebSearch) Name() string { return "web_search" }
func (WebSearch) Description() string {
	return "Search the web via Exa. Returns each result's title, URL, and a lightweight highlights/summary snippet (never full page text), capped at 5 results."
}

func (WebSearch) Parameters() Schema {
	return Schema{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query.",
			},
			"category": map[string]any{
				"type":        "string",
				"description": "Restrict results to a content type.",
				// Matches Exa's documented category enum verified in
				// docs/research/exa-search-api.md.
				"enum": []string{"publication", "news", "company", "people"},
			},
		},
		"required": []string{"query"},
	}
}

func (WebSearch) Safety() Safety {
	return Safety{SideEffect: SideEffectNetwork}
}

func (w WebSearch) Run(ctx context.Context, args map[string]any) Result {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return errorResult(`web_search: "query" argument is required`)
	}
	category, _ := args["category"].(string)

	results, err := w.search(ctx, query, category)
	if err != nil {
		return errorResult(fmt.Sprintf("web_search: %v", err))
	}
	return Result{Content: formatWebSearchResults(results)}
}

// webSearchResult is one Exa result reduced to what the model is allowed to
// see: title, URL, and a capped highlights/summary snippet.
type webSearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// exaSearchRequest is the request body for Exa's POST /search.
type exaSearchRequest struct {
	Query      string      `json:"query"`
	Category   string      `json:"category,omitempty"`
	NumResults int         `json:"numResults,omitempty"`
	Contents   exaContents `json:"contents"`
}

// exaContents requests only highlights and a summary per result — never
// Exa's "text" field (a result's full page content), matching this tool's
// spec of never surfacing full page text.
type exaContents struct {
	Highlights bool `json:"highlights"`
	Summary    bool `json:"summary"`
}

type exaSearchResponse struct {
	Results []exaResult `json:"results"`
}

type exaResult struct {
	Title      string   `json:"title"`
	URL        string   `json:"url"`
	Highlights []string `json:"highlights"`
	Summary    string   `json:"summary"`
}

// search calls Exa's /search API and reduces the response to
// webSearchResults, already capped at webSearchMaxResults results and
// webSearchSnippetCap chars each.
func (w WebSearch) search(ctx context.Context, query, category string) ([]webSearchResult, error) {
	reqBody, err := json.Marshal(exaSearchRequest{
		Query:      query,
		Category:   category,
		NumResults: webSearchMaxResults,
		Contents:   exaContents{Highlights: true, Summary: true},
	})
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	baseURL := w.BaseURL
	if baseURL == "" {
		baseURL = exaDefaultBaseURL
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/search", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", w.APIKey)

	resp, err := defaultHTTPClient(w.Client, exaSearchTimeout).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call exa: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read exa response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exa returned %s: %s", resp.Status, bytes.TrimSpace(respBody))
	}

	var exaResp exaSearchResponse
	if err := json.Unmarshal(respBody, &exaResp); err != nil {
		return nil, fmt.Errorf("decode exa response: %w", err)
	}

	results := exaResp.Results
	if len(results) > webSearchMaxResults {
		results = results[:webSearchMaxResults]
	}

	out := make([]webSearchResult, 0, len(results))
	for _, r := range results {
		var parts []string
		if len(r.Highlights) > 0 {
			parts = append(parts, strings.Join(r.Highlights, " "))
		}
		if r.Summary != "" {
			parts = append(parts, r.Summary)
		}
		out = append(out, webSearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: capSnippet(strings.TrimSpace(strings.Join(parts, " ")), webSearchSnippetCap),
		})
	}
	return out, nil
}

// capSnippet cuts s at max bytes, backed up to a UTF-8 rune boundary via
// truncate()'s own backUpToRuneBoundary (ADR-0005), appending "..." when
// actually cut.
func capSnippet(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := backUpToRuneBoundary(s, max)
	return strings.TrimSpace(s[:cut]) + "..."
}

// formatWebSearchResults renders results into liam's shared find/grep-style
// plain-text convention: a header count line followed by one numbered entry
// per result (title, URL, snippet).
func formatWebSearchResults(results []webSearchResult) string {
	header := formatHeader("result", "results", len(results), len(results))
	if len(results) == 0 {
		return header + "."
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n\n")
	for i, r := range results {
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, r.Title, r.URL)
		if r.Snippet != "" {
			fmt.Fprintf(&b, "   %s\n", r.Snippet)
		}
		if i != len(results)-1 {
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
