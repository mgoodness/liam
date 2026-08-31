package openrouter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/session"
)

var _ session.ContextLookup = (*Provider)(nil)

func modelServer(t *testing.T, contextLength int64, hits *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":{"id":"anthropic/claude-3.7-sonnet","canonical_slug":"anthropic/claude-3.7-sonnet","name":"Claude 3.7 Sonnet","created":0,"context_length":%d,"architecture":{"modality":"text","input_modalities":[],"output_modalities":[],"tokenizer":"Claude"},"pricing":{"prompt":"0","completion":"0"},"top_provider":{"is_moderated":false},"per_request_limits":null,"default_parameters":null,"links":{},"supported_parameters":[],"supported_voices":null}}`, contextLength)
	}))
}

func TestMaxContextLengthFetchesAndCaches(t *testing.T) {
	var hits int
	srv := modelServer(t, 200000, &hits)
	defer srv.Close()

	p := newTestProvider(srv.URL)

	for i := 0; i < 2; i++ {
		got, err := p.MaxContextLength(context.Background(), "anthropic/claude-3.7-sonnet")
		if err != nil {
			t.Fatalf("MaxContextLength() error = %v", err)
		}
		if want := 200000; got != want {
			t.Errorf("MaxContextLength() = %d, want %d", got, want)
		}
	}
	if hits != 1 {
		t.Errorf("server was hit %d times, want 1 (second call should be cached)", hits)
	}
}

func TestMaxContextLengthCachesPerModelID(t *testing.T) {
	var hits int
	srv := modelServer(t, 200000, &hits)
	defer srv.Close()

	p := newTestProvider(srv.URL)

	if _, err := p.MaxContextLength(context.Background(), "anthropic/claude-3.7-sonnet"); err != nil {
		t.Fatalf("MaxContextLength() error = %v", err)
	}
	if _, err := p.MaxContextLength(context.Background(), "openai/gpt-4.1"); err != nil {
		t.Fatalf("MaxContextLength() error = %v", err)
	}
	if hits != 2 {
		t.Errorf("server was hit %d times, want 2 (distinct model ids each miss the cache)", hits)
	}
}

func TestMaxContextLengthRejectsModelIDWithoutSlash(t *testing.T) {
	p := newTestProvider("http://unused.invalid")

	if _, err := p.MaxContextLength(context.Background(), "openrouter-auto"); err == nil {
		t.Fatal("MaxContextLength() error = nil, want an error for a slug lacking author/slug form")
	}
}

func TestMaxContextLengthMapsHTTPErrorToProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"code":404,"message":"model not found"}}`)
	}))
	defer srv.Close()

	p := newTestProvider(srv.URL)

	_, err := p.MaxContextLength(context.Background(), "nonexistent/model")
	if err == nil {
		t.Fatal("MaxContextLength() error = nil, want an error")
	}
	perr, ok := err.(*provider.ProviderError)
	if !ok {
		t.Fatalf("error %v (%T) is not a *provider.ProviderError", err, err)
	}
	if perr.Kind != provider.ErrorKindInvalidRequest {
		t.Errorf("Kind = %v, want %v", perr.Kind, provider.ErrorKindInvalidRequest)
	}
}

func TestSplitModelID(t *testing.T) {
	tests := []struct {
		model      string
		wantAuthor string
		wantSlug   string
		wantOK     bool
	}{
		{"anthropic/claude-3.7-sonnet", "anthropic", "claude-3.7-sonnet", true},
		{"openai/gpt-4:free", "openai", "gpt-4:free", true},
		{"openrouter/auto", "openrouter", "auto", true},
		{"no-slash", "", "", false},
		{"/leading-slash", "", "", false},
		{"trailing-slash/", "", "", false},
	}
	for _, tt := range tests {
		author, slug, ok := splitModelID(tt.model)
		if author != tt.wantAuthor || slug != tt.wantSlug || ok != tt.wantOK {
			t.Errorf("splitModelID(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.model, author, slug, ok, tt.wantAuthor, tt.wantSlug, tt.wantOK)
		}
	}
}
