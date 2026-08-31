package tool

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// blankLinesRe collapses 3+ consecutive newlines (left behind by adjacent
// block elements each requesting their own blank-line separation) down to a
// single blank line.
var blankLinesRe = regexp.MustCompile(`\n{3,}`)

// spaceRe collapses runs of whitespace (including newlines) within a single
// HTML text node into one space, matching how a browser collapses
// insignificant whitespace before layout.
var spaceRe = regexp.MustCompile(`\s+`)

// htmlToMarkdown converts an HTML document to a readable Markdown
// approximation: headings, paragraphs, links, emphasis, lists, and code
// blocks are rendered with their Markdown syntax; everything else is
// flattened to its text content. It's a lightweight best-effort renderer,
// not a spec-complete HTML-to-Markdown converter — web_fetch's job is
// giving the model readable page content, not a faithful document
// round-trip.
func htmlToMarkdown(body []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("parse html: %w", err)
	}

	var b strings.Builder
	renderNode(&b, doc)

	out := blankLinesRe.ReplaceAllString(b.String(), "\n\n")
	return strings.TrimSpace(out), nil
}

// renderNode writes n's Markdown-ish rendering to b, recursing into
// children as needed.
func renderNode(b *strings.Builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		if text := spaceRe.ReplaceAllString(n.Data, " "); text != "" {
			b.WriteString(text)
		}
	case html.ElementNode:
		renderElement(b, n)
	default:
		renderChildren(b, n)
	}
}

// renderElement is renderNode's ElementNode case, split out for its size:
// script/style/head are dropped whole (never recursed into), a handful of
// tags get their Markdown equivalent, and anything else just recurses into
// its children with no markup of its own.
func renderElement(b *strings.Builder, n *html.Node) {
	switch n.Data {
	case "script", "style", "noscript", "svg", "head":
		return
	case "br":
		b.WriteString("\n")
	case "hr":
		b.WriteString("\n\n---\n\n")
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		b.WriteString("\n" + strings.Repeat("#", level) + " ")
		renderChildren(b, n)
		b.WriteString("\n\n")
	case "p", "div":
		renderChildren(b, n)
		b.WriteString("\n\n")
	case "li":
		b.WriteString("- ")
		renderChildren(b, n)
		b.WriteString("\n")
	case "pre":
		b.WriteString("\n```\n")
		b.WriteString(strings.Trim(rawText(n), "\n"))
		b.WriteString("\n```\n\n")
	case "code":
		b.WriteString("`")
		renderChildren(b, n)
		b.WriteString("`")
	case "strong", "b":
		b.WriteString("**")
		renderChildren(b, n)
		b.WriteString("**")
	case "em", "i":
		b.WriteString("*")
		renderChildren(b, n)
		b.WriteString("*")
	case "a":
		renderLink(b, n)
	default:
		renderChildren(b, n)
	}
}

// renderLink renders an <a> as Markdown's "[text](href)", or bare text when
// there's no href or no text to link.
func renderLink(b *strings.Builder, n *html.Node) {
	href := attr(n, "href")
	var inner strings.Builder
	renderChildren(&inner, n)
	text := strings.TrimSpace(inner.String())

	if href == "" || text == "" {
		b.WriteString(text)
		return
	}
	fmt.Fprintf(b, "[%s](%s)", text, href)
}

func renderChildren(b *strings.Builder, n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderNode(b, c)
	}
}

// rawText concatenates n's descendant text nodes verbatim, with none of
// renderNode's whitespace collapsing — used inside <pre> where whitespace
// (indentation, line breaks) is significant.
func rawText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// attr returns n's key attribute value, or "" if n has no such attribute.
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
