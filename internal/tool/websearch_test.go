package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSearchSafety(t *testing.T) {
	want := Safety{SideEffect: SideEffectNetwork}
	if got := (WebSearch{}).Safety(); got != want {
		t.Errorf("Safety() = %+v, want %+v", got, want)
	}
}

func TestWebSearchRunMissingQueryArg(t *testing.T) {
	got := WebSearch{}.Run(context.Background(), map[string]any{})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}

func TestWebSearchRunSendsQueryCategoryAndAPIKey(t *testing.T) {
	var gotReq exaSearchRequest
	var gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		json.NewEncoder(w).Encode(exaSearchResponse{})
	}))
	defer srv.Close()

	got := WebSearch{APIKey: "test-key", BaseURL: srv.URL}.Run(context.Background(), map[string]any{
		"query":    "golang concurrency",
		"category": "news",
	})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if gotAPIKey != "test-key" {
		t.Errorf("x-api-key header = %q, want %q", gotAPIKey, "test-key")
	}
	if gotReq.Query != "golang concurrency" {
		t.Errorf("request Query = %q, want %q", gotReq.Query, "golang concurrency")
	}
	if gotReq.Category != "news" {
		t.Errorf("request Category = %q, want %q", gotReq.Category, "news")
	}
	if !gotReq.Contents.Highlights || !gotReq.Contents.Summary {
		t.Errorf("request Contents = %+v, want highlights and summary both requested", gotReq.Contents)
	}
}

func TestWebSearchRunNeverExposesFullPageText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(exaSearchResponse{Results: []exaResult{
			{
				Title:      "Example",
				URL:        "https://example.com",
				Highlights: []string{"a relevant snippet"},
				Summary:    "a short summary",
			},
		}})
	}))
	defer srv.Close()

	got := WebSearch{APIKey: "k", BaseURL: srv.URL}.Run(context.Background(), map[string]any{"query": "example"})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	for _, want := range []string{"Example", "https://example.com", "a relevant snippet", "a short summary"} {
		if !strings.Contains(got.Content, want) {
			t.Errorf("Content missing %q:\n%s", want, got.Content)
		}
	}
}

func TestWebSearchRunCapsResultsAndSnippetLength(t *testing.T) {
	var results []exaResult
	for i := range 8 {
		results = append(results, exaResult{
			Title:   "Result",
			URL:     "https://example.com",
			Summary: strings.Repeat("x", 900) + string(rune('a'+i)),
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(exaSearchResponse{Results: results})
	}))
	defer srv.Close()

	got := WebSearch{APIKey: "k", BaseURL: srv.URL}.Run(context.Background(), map[string]any{"query": "example"})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if strings.Count(got.Content, "https://example.com") != webSearchMaxResults {
		t.Errorf("Content has %d results, want %d (capped)", strings.Count(got.Content, "https://example.com"), webSearchMaxResults)
	}
	if strings.Contains(got.Content, strings.Repeat("x", 900)) {
		t.Error("Content contains an uncapped 900-char snippet, want it capped to ~500 chars")
	}
}

func TestWebSearchRunSurfacesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	got := WebSearch{APIKey: "bad-key", BaseURL: srv.URL}.Run(context.Background(), map[string]any{"query": "example"})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}

// TestWebSearchRunMatchesGoldenOutput is this ticket's golden-file coverage
// (issue #50's acceptance criteria) for web_search's plain-text output
// convention, matching find/grep's own testdata/*.golden pattern.
func TestWebSearchRunMatchesGoldenOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(exaSearchResponse{Results: []exaResult{
			{
				Title:      "Effective Go",
				URL:        "https://go.dev/doc/effective_go",
				Highlights: []string{"Go is a new language."},
				Summary:    "The official guide to writing clear, idiomatic Go code.",
			},
			{
				Title:      "The Go Programming Language Specification",
				URL:        "https://go.dev/ref/spec",
				Highlights: []string{"This is a reference manual for the Go programming language."},
			},
		}})
	}))
	defer srv.Close()

	got := WebSearch{APIKey: "k", BaseURL: srv.URL}.Run(context.Background(), map[string]any{"query": "go documentation"})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	want := readFile(t, "testdata/websearch_go_docs.golden")
	if got.Content != want {
		t.Errorf("Content = %q, want golden %q", got.Content, want)
	}
}

func TestFormatWebSearchResultsEmpty(t *testing.T) {
	got := formatWebSearchResults(nil)
	want := "Found 0 results."
	if got != want {
		t.Errorf("formatWebSearchResults(nil) = %q, want %q", got, want)
	}
}

func TestCapSnippet(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"under cap unchanged", "short", 10, "short"},
		{"exact cap unchanged", "1234567890", 10, "1234567890"},
		{"over cap gets ellipsis", "12345678901", 10, "1234567890..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := capSnippet(tt.s, tt.max); got != tt.want {
				t.Errorf("capSnippet(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}
