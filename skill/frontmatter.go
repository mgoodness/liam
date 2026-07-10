package skill

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// delim is the YAML frontmatter fence SKILL.md must open and close with.
const delim = "---"

// parseFrontmatter extracts and parses the YAML frontmatter block a
// SKILL.md file must begin with. Parsing is permissive: unknown keys are
// ignored, and the caller decides which of the parsed fields to validate.
func parseFrontmatter(data []byte) (frontmatter, error) {
	text := string(data)
	if !strings.HasPrefix(text, delim) {
		return frontmatter{}, errors.New("SKILL.md missing frontmatter delimiter")
	}

	rest := text[len(delim):]
	end := strings.Index(rest, "\n"+delim)
	if end == -1 {
		return frontmatter{}, errors.New("SKILL.md frontmatter is not terminated")
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return frontmatter{}, fmt.Errorf("parsing frontmatter: %w", err)
	}
	return fm, nil
}

// nameRE matches the open spec's name rule in one pattern: lowercase
// alphanumeric segments joined by single hyphens. This alone rules out an
// empty name and a leading, trailing, or consecutive hyphen — anything
// matching it still needs a separate length check, since regex
// quantifiers can't naturally bound total string length here.
var nameRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// validateName checks name against the open spec's rules: lowercase
// alphanumeric and hyphens only, no leading/trailing/consecutive hyphens,
// at most 64 characters, and equal to dir's own base name.
func validateName(name, dir string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 64 {
		return fmt.Errorf("name %q exceeds 64 characters", name)
	}
	if !nameRE.MatchString(name) {
		return fmt.Errorf("name %q must be lowercase alphanumeric and hyphens only, with no leading, trailing, or consecutive hyphens", name)
	}
	if parent := filepath.Base(dir); name != parent {
		return fmt.Errorf("name %q does not match parent directory %q", name, parent)
	}
	return nil
}
