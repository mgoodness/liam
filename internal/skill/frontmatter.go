package skill

import (
	"bufio"
	"errors"
	"strings"
)

// parseFrontmatter splits data into SKILL.md's YAML frontmatter fields and
// Markdown body. Only flat, top-level scalar fields are parsed — full YAML
// isn't needed, since every frontmatter field liam reads (name,
// description, disable-model-invocation) is a plain scalar; an indented
// (nested) line, such as a metadata map's entries, is skipped rather than
// rejected, matching the spec's lenient-parsing guidance. A line is split
// on its first colon only, so an unquoted colon inside a description
// (a documented cross-client parsing pitfall) doesn't corrupt the value.
func parseFrontmatter(data []byte) (fields map[string]string, body string, err error) {
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if !sc.Scan() {
		return nil, "", errors.New("empty file")
	}
	if sc.Text() != "---" {
		return nil, "", errors.New(`missing frontmatter delimiter ("---") on the first line`)
	}

	fields = map[string]string{}
	var bodyLines []string
	inBody := false
	closed := false

	for sc.Scan() {
		line := sc.Text()
		if inBody {
			bodyLines = append(bodyLines, line)
			continue
		}
		if line == "---" {
			inBody = true
			closed = true
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			// Nested/indented line (e.g. a metadata map entry) — not a
			// top-level scalar field.
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(key)] = unquote(strings.TrimSpace(value))
	}
	if err := sc.Err(); err != nil {
		return nil, "", err
	}
	if !closed {
		return nil, "", errors.New(`missing closing frontmatter delimiter ("---")`)
	}

	return fields, strings.TrimSpace(strings.Join(bodyLines, "\n")), nil
}

// unquote strips a single layer of matching surrounding quotes ('...' or
// "..."), if present.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
