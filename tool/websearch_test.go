package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestWebSearchServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func callWebSearch(t *testing.T, handler Handler, query string) (string, error) {
	t.Helper()
	args, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("marshaling args: %v", err)
	}
	return handler(context.Background(), args)
}

func TestWebSearch_SendsQueryAndAPIKey(t *testing.T) {
	var gotQuery, gotToken, gotAccept string

	srv := newTestWebSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		gotToken = r.Header.Get("X-Subscription-Token")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(braveSearchResponse{})
	})

	tool := NewWebSearchTool("test-api-key", srv.URL)

	if _, err := callWebSearch(t, tool.Handler, "golang generics"); err != nil {
		t.Fatalf("web_search: %v", err)
	}

	if gotQuery != "golang generics" {
		t.Errorf("query param = %q, want %q", gotQuery, "golang generics")
	}
	if gotToken != "test-api-key" {
		t.Errorf("X-Subscription-Token = %q, want %q", gotToken, "test-api-key")
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept header = %q, want %q", gotAccept, "application/json")
	}
}

func TestWebSearch_FormatsResults(t *testing.T) {
	srv := newTestWebSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"web": map[string]any{
				"results": []map[string]string{
					{
						"title":       "Go generics tutorial",
						"url":         "https://example.com/generics",
						"description": "A guide to generics in Go.",
					},
					{
						"title":       "Effective Go",
						"url":         "https://go.dev/doc/effective_go",
						"description": "Tips for writing clear Go code.",
					},
				},
			},
		})
	})

	tool := NewWebSearchTool("test-api-key", srv.URL)

	got, err := callWebSearch(t, tool.Handler, "golang generics")
	if err != nil {
		t.Fatalf("web_search: %v", err)
	}

	for _, want := range []string{
		"Go generics tutorial", "https://example.com/generics", "A guide to generics in Go.",
		"Effective Go", "https://go.dev/doc/effective_go", "Tips for writing clear Go code.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("result %q missing from output:\n%s", want, got)
		}
	}
}

func TestWebSearch_NoResults(t *testing.T) {
	srv := newTestWebSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(braveSearchResponse{})
	})

	tool := NewWebSearchTool("test-api-key", srv.URL)

	got, err := callWebSearch(t, tool.Handler, "no such thing exists anywhere")
	if err != nil {
		t.Fatalf("web_search: %v", err)
	}
	if got == "" {
		t.Error("expected a non-empty message noting no results, got empty string")
	}
}

func TestWebSearch_MissingQuery(t *testing.T) {
	tool := NewWebSearchTool("test-api-key", "http://unused.invalid")

	if _, err := callWebSearch(t, tool.Handler, ""); err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestWebSearch_NonOKStatusReturnsError(t *testing.T) {
	srv := newTestWebSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	})

	tool := NewWebSearchTool("bad-key", srv.URL)

	_, err := callWebSearch(t, tool.Handler, "golang generics")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to mention the status code, got %v", err)
	}
}

func TestWebSearch_DefinitionShape(t *testing.T) {
	tool := NewWebSearchTool("test-api-key", "")

	if tool.Definition.Name != "web_search" {
		t.Errorf("Definition.Name = %q, want %q", tool.Definition.Name, "web_search")
	}
	if _, ok := tool.Definition.Parameters.Properties["query"]; !ok {
		t.Error("Definition.Parameters.Properties missing \"query\"")
	}
	if len(tool.Definition.Parameters.Required) != 1 || tool.Definition.Parameters.Required[0] != "query" {
		t.Errorf("Definition.Parameters.Required = %v, want [query]", tool.Definition.Parameters.Required)
	}
}
