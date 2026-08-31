package tool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetchSafety(t *testing.T) {
	want := Safety{SideEffect: SideEffectNetwork}
	if got := (WebFetch{}).Safety(); got != want {
		t.Errorf("Safety() = %+v, want %+v", got, want)
	}
}

func TestWebFetchRunMissingURLArg(t *testing.T) {
	got := WebFetch{}.Run(context.Background(), map[string]any{})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}

func TestWebFetchRunConvertsHTMLToMarkdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><h1>Title</h1><p>Hello <strong>world</strong>, see <a href="https://example.com">this link</a>.</p></body></html>`))
	}))
	defer srv.Close()

	got := WebFetch{}.Run(context.Background(), map[string]any{"url": srv.URL})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	for _, want := range []string{"# Title", "**world**", "[this link](https://example.com)"} {
		if !strings.Contains(got.Content, want) {
			t.Errorf("Content missing %q:\n%s", want, got.Content)
		}
	}
}

// TestWebFetchRunMatchesGoldenOutput is this ticket's golden-file coverage
// (issue #50's acceptance criteria) for web_fetch's HTML-to-Markdown output
// convention, matching find/grep's own testdata/*.golden pattern.
func TestWebFetchRunMatchesGoldenOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><h1>Effective Go</h1><p>Go is a <strong>new</strong> language.</p><ul><li>Fast</li><li>Simple</li></ul></body></html>`))
	}))
	defer srv.Close()

	got := WebFetch{}.Run(context.Background(), map[string]any{"url": srv.URL})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	want := readFile(t, "testdata/webfetch_effective_go.golden")
	if got.Content != want {
		t.Errorf("Content = %q, want golden %q", got.Content, want)
	}
}

func TestWebFetchRunPassesThroughNonHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("plain body content"))
	}))
	defer srv.Close()

	got := WebFetch{}.Run(context.Background(), map[string]any{"url": srv.URL})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if got.Content != "plain body content" {
		t.Errorf("Content = %q, want %q", got.Content, "plain body content")
	}
}

func TestWebFetchRunTruncatesLargeOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(strings.Repeat("a\n", outputCap)))
	}))
	defer srv.Close()

	got := WebFetch{}.Run(context.Background(), map[string]any{"url": srv.URL})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if len(got.Content) >= outputCap*2 {
		t.Errorf("Content len = %d, want truncated to ~%d", len(got.Content), outputCap)
	}
	if !strings.Contains(got.Content, "truncated") {
		t.Error("Content missing truncation marker")
	}
}

func TestWebFetchRunSurfacesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	got := WebFetch{}.Run(context.Background(), map[string]any{"url": srv.URL})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}

func TestWebFetchRunSurfacesInvalidURL(t *testing.T) {
	got := WebFetch{}.Run(context.Background(), map[string]any{"url": "://not-a-url"})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}

func TestIsHTMLContent(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        bool
	}{
		{"html content-type", "text/html; charset=utf-8", "irrelevant", true},
		{"plain content-type", "text/plain", "<html>", false},
		{"sniffs doctype when no content-type", "", "<!DOCTYPE html><html></html>", true},
		{"sniffs html tag when no content-type", "", "<html><body></body></html>", true},
		{"plain text when no content-type", "", "just some text", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHTMLContent(tt.contentType, []byte(tt.body)); got != tt.want {
				t.Errorf("isHTMLContent(%q, %q) = %v, want %v", tt.contentType, tt.body, got, tt.want)
			}
		})
	}
}

func TestHTMLToMarkdown(t *testing.T) {
	tests := []struct {
		name string
		html string
		want []string
	}{
		{
			name: "headings and paragraph",
			html: `<h1>Title</h1><p>Body text.</p>`,
			want: []string{"# Title", "Body text."},
		},
		{
			name: "list items",
			html: `<ul><li>one</li><li>two</li></ul>`,
			want: []string{"- one", "- two"},
		},
		{
			name: "code block preserves whitespace",
			html: `<pre>line one
  line two</pre>`,
			want: []string{"```\nline one\n  line two\n```"},
		},
		{
			name: "script and style are dropped",
			html: `<p>keep</p><script>drop-me</script><style>.also-drop{}</style>`,
			want: []string{"keep"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := htmlToMarkdown([]byte(tt.html))
			if err != nil {
				t.Fatalf("htmlToMarkdown() error = %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("htmlToMarkdown(%q) = %q, want to contain %q", tt.html, got, want)
				}
			}
			if strings.Contains(got, "drop-me") || strings.Contains(got, "also-drop") {
				t.Errorf("htmlToMarkdown(%q) = %q, want script/style content dropped", tt.html, got)
			}
		})
	}
}
