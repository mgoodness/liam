package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func callWebFetch(t *testing.T, url string) (string, error) {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"url": url})
	if err != nil {
		t.Fatalf("marshaling args: %v", err)
	}
	return WebFetch(context.Background(), raw)
}

func TestWebFetch_ConvertsHTMLToReadableText(t *testing.T) {
	const page = `<!DOCTYPE html>
<html>
<head>
<title>Example Page</title>
<style>body { color: red; }</style>
<script>console.log("should not appear");</script>
</head>
<body>
<nav><a href="/">Home</a></nav>
<h1>Welcome</h1>
<p>This is a <strong>test</strong> paragraph with a <a href="https://example.com">link</a>.</p>
<ul>
<li>First item</li>
<li>Second item</li>
</ul>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(page))
	}))
	t.Cleanup(srv.Close)

	got, err := callWebFetch(t, srv.URL)
	if err != nil {
		t.Fatalf("WebFetch: %v", err)
	}

	for _, want := range []string{"Welcome", "This is a", "test", "paragraph with a", "link", "First item", "Second item", "Home"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected result to contain %q, got %q", want, got)
		}
	}
	for _, notWant := range []string{"<h1>", "<p>", "<script", "console.log", "color: red"} {
		if strings.Contains(got, notWant) {
			t.Errorf("expected result not to contain %q, got %q", notWant, got)
		}
	}
}

func TestWebFetch_MissingURL(t *testing.T) {
	if _, err := callWebFetch(t, ""); err == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestWebFetch_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	if _, err := callWebFetch(t, srv.URL); err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestWebFetch_InvalidURL(t *testing.T) {
	if _, err := callWebFetch(t, "://not-a-url"); err == nil {
		t.Fatal("expected error for invalid url")
	}
}

func TestWebFetch_UnreachableHost(t *testing.T) {
	if _, err := callWebFetch(t, "http://127.0.0.1:0"); err == nil {
		t.Fatal("expected error for unreachable host")
	}
}
