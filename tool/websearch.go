package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// braveSearchEndpoint is the Brave Search API's web search endpoint.
const braveSearchEndpoint = "https://api.search.brave.com/res/v1/web/search"

var webSearchDefinition = Definition{
	Name:        "web_search",
	Description: "Search the web via the Brave Search API and return matching results (title, URL, and snippet for each).",
	Parameters: Parameters{
		Type: "object",
		Properties: map[string]Property{
			"query": {Type: "string", Description: "The search query."},
		},
		Required: []string{"query"},
	},
}

type webSearchArgs struct {
	Query string `json:"query"`
}

// braveSearchResponse is the subset of the Brave Search API's response
// shape web_search needs.
type braveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// NewWebSearchTool returns the web_search Tool, whose Handler queries the
// Brave Search API using apiKey. endpoint overrides the production Brave
// endpoint when non-empty, so tests can point it at an httptest.Server.
func NewWebSearchTool(apiKey, endpoint string) Tool {
	if endpoint == "" {
		endpoint = braveSearchEndpoint
	}
	return Tool{
		Definition: webSearchDefinition,
		Handler:    webSearchHandler(apiKey, endpoint),
	}
}

func webSearchHandler(apiKey, endpoint string) Handler {
	return func(ctx context.Context, args json.RawMessage) (string, error) {
		var a webSearchArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("parsing web_search args: %w", err)
		}
		if a.Query == "" {
			return "", errors.New("web_search: query is required")
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", fmt.Errorf("web_search: %w", err)
		}
		q := req.URL.Query()
		q.Set("q", a.Query)
		req.URL.RawQuery = q.Encode()
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Subscription-Token", apiKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("web_search: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return "", fmt.Errorf("web_search: unexpected status %s: %s", resp.Status, body)
		}

		var out braveSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", fmt.Errorf("web_search: decoding response: %w", err)
		}

		if len(out.Web.Results) == 0 {
			return "no results found", nil
		}

		var b strings.Builder
		for i, r := range out.Web.Results {
			if i > 0 {
				b.WriteString("\n\n")
			}
			fmt.Fprintf(&b, "%s\n%s\n%s", r.Title, r.URL, r.Description)
		}
		return b.String(), nil
	}
}
