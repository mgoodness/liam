package skill

import "fmt"

// Finding is one hidden-content character ScanHidden flagged as a
// possible prompt-injection vector: invisible instructions
// steganographically encoded via characters that don't render visibly in
// a normal text view (Unicode tag characters, zero-width/invisible format
// characters, BiDi embedding/override/isolate characters). Prior art:
// pi-go's `pi audit` scanner (docs/research/pi-go-jcode-prior-art.md,
// finding #3).
type Finding struct {
	Rune   rune
	Name   string
	Offset int // byte offset into the scanned content
}

// ScanHidden scans content for hidden-content characters, returning one
// Finding per occurrence. It complements, rather than replaces, the
// directory-level project trust prompt: trust gates whether an untrusted
// directory's skills are read at all, this catches steganographic content
// hiding inside an otherwise-trusted file.
func ScanHidden(content string) []Finding {
	var out []Finding
	for i, r := range content {
		if name, ok := suspiciousRune(r); ok {
			out = append(out, Finding{Rune: r, Name: name, Offset: i})
		}
	}
	return out
}

// suspiciousRune classifies r as a hidden-content character, if it is
// one.
func suspiciousRune(r rune) (name string, ok bool) {
	switch {
	case r >= 0xE0000 && r <= 0xE007F:
		return "Unicode tag character", true
	case r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF || r == 0x2060:
		return "zero-width/invisible character", true
	case r >= 0x202A && r <= 0x202E:
		return "BiDi embedding/override character", true
	case r >= 0x2066 && r <= 0x2069:
		return "BiDi isolate character", true
	}
	return "", false
}

// describeFindings summarizes findings into a short, human-readable
// message: a count plus the distinct kinds found.
func describeFindings(findings []Finding) string {
	seen := map[string]bool{}
	var names []string
	for _, f := range findings {
		if !seen[f.Name] {
			seen[f.Name] = true
			names = append(names, f.Name)
		}
	}
	return fmt.Sprintf("%d occurrence(s) of %v", len(findings), names)
}
