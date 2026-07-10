package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// webFetchTimeout bounds how long a web_fetch request may take, so a slow
// or hanging server can't stall the agent loop indefinitely.
const webFetchTimeout = 30 * time.Second

var webFetchDefinition = Definition{
	Name:        "web_fetch",
	Description: "Fetch a URL via HTTP GET and return its content as readable text. HTML is converted to text; no JavaScript is executed.",
	Parameters: Parameters{
		Type: "object",
		Properties: map[string]Property{
			"url": {Type: "string", Description: "The URL to fetch."},
		},
		Required: []string{"url"},
	},
}

type webFetchArgs struct {
	URL string `json:"url"`
}

// WebFetch performs an HTTP GET against args.URL and returns its content as
// readable text, converting HTML to text along the way. It requires no API
// key, unlike web_search, since it talks directly to the given URL rather
// than a third-party indexing service.
func WebFetch(ctx context.Context, args json.RawMessage) (string, error) {
	var a webFetchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing web_fetch args: %w", err)
	}
	if a.URL == "" {
		return "", errors.New("web_fetch: url is required")
	}

	reqCtx, cancel := context.WithTimeout(ctx, webFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", fmt.Errorf("web_fetch: building request for %s: %w", a.URL, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("web_fetch: fetching %s: %w", a.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("web_fetch: %s returned status %d", a.URL, resp.StatusCode)
	}

	text, err := htmlToText(resp.Body)
	if err != nil {
		return "", fmt.Errorf("web_fetch: converting %s to text: %w", a.URL, err)
	}
	return text, nil
}

// skippedElements are tags whose text content isn't part of a page's
// readable text — script/style bodies aren't prose, and head/noscript
// content is either invisible or a fallback never meant to be read directly.
var skippedElements = map[string]bool{
	"script":   true,
	"style":    true,
	"noscript": true,
	"head":     true,
}

// blockElements are tags that should force a line break after their
// content, so e.g. list items and paragraphs don't run together into one
// unreadable line the way inline elements (span, a, strong, ...) should.
var blockElements = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "tr": true, "td": true, "th": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"ul": true, "ol": true, "table": true, "section": true, "article": true,
	"header": true, "footer": true, "nav": true, "blockquote": true, "hr": true,
}

// htmlToText parses r as HTML and extracts its readable text, dropping
// markup, script/style bodies, and hidden head metadata, and collapsing
// whitespace so the result reads like prose instead of a token dump.
func htmlToText(r io.Reader) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", fmt.Errorf("parsing html: %w", err)
	}

	var sb strings.Builder
	extractText(doc, &sb)
	return normalizeWhitespace(sb.String()), nil
}

// extractText walks the HTML node tree depth-first, writing text node
// content to sb while skipping non-prose elements and inserting line
// breaks after block-level elements.
func extractText(n *html.Node, sb *strings.Builder) {
	if n.Type == html.ElementNode && skippedElements[n.Data] {
		return
	}
	if n.Type == html.TextNode {
		sb.WriteString(n.Data)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractText(c, sb)
	}
	if n.Type == html.ElementNode && blockElements[n.Data] {
		sb.WriteString("\n")
	}
}

// normalizeWhitespace collapses each line's internal whitespace (HTML
// source indentation is not meaningful) and squashes runs of blank lines
// left by adjacent block elements down to a single blank line.
func normalizeWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	cleaned := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}
