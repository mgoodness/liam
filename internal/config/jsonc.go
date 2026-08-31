package config

// stripComments removes JSONC's `//` line comments and `/* */` block
// comments from src, leaving valid JSON for encoding/json.Unmarshal.
// Comment-like sequences inside quoted strings (including escaped quotes)
// are left untouched. Each stripped comment is replaced by the newlines it
// contained (or a single newline for a line comment) so error line numbers
// from json.Unmarshal on the result stay meaningful.
func stripComments(src []byte) []byte {
	out := make([]byte, 0, len(src))
	inString := false
	escaped := false

	for i := 0; i < len(src); i++ {
		c := src[i]

		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}

		switch {
		case c == '"':
			inString = true
			out = append(out, c)

		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if i < len(src) {
				out = append(out, '\n')
			}

		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				if src[i] == '\n' {
					out = append(out, '\n')
				}
				i++
			}
			i++ // land on the closing '*'; outer i++ steps past the '/'

		default:
			out = append(out, c)
		}
	}

	return out
}
