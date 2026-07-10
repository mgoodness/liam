// Package skill discovers, validates, and indexes skill-name/SKILL.md
// directories conforming to the open Agent Skills specification
// (https://agentskills.io/specification), so liam can inject a
// model-facing name+description index into its system prompt and let the
// model decide on its own to load a skill's full body via the existing
// read tool.
package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Skill is a validated skill-name/SKILL.md directory. Path is the skill's
// directory, not the SKILL.md file itself — see SkillMDPath.
type Skill struct {
	Name        string
	Description string
	Path        string

	// Optional frontmatter fields, parsed and retained but not acted on
	// (see CONTEXT.md's Skill entry).
	License                string
	Compatibility          string
	Metadata               map[string]string
	AllowedTools           string
	DisableModelInvocation bool
}

// frontmatter mirrors the open spec's SKILL.md YAML frontmatter fields
// exactly (https://agentskills.io/specification), plus
// disable-model-invocation, a Claude-Code-originated convention liam
// honors though it isn't part of the open spec.
type frontmatter struct {
	Name                   string            `yaml:"name"`
	Description            string            `yaml:"description"`
	License                string            `yaml:"license"`
	Compatibility          string            `yaml:"compatibility"`
	Metadata               map[string]string `yaml:"metadata"`
	AllowedTools           string            `yaml:"allowed-tools"`
	DisableModelInvocation bool              `yaml:"disable-model-invocation"`
}

// errNotASkill marks a subdirectory with no SKILL.md, which discoverDir
// treats as "simply not a skill" rather than a validation failure worth
// warning about.
var errNotASkill = errors.New("no SKILL.md")

// Discover scans each directory in dirs for immediate subdirectories
// containing a valid SKILL.md, returning the combined set in stable
// (name-sorted) order. A subdirectory with no SKILL.md is silently
// skipped. One whose SKILL.md exists but fails validation is skipped with
// a warning to stderr — not a startup failure. On a name collision, the
// skill from the later directory in dirs wins, so callers pass the global
// skills directory before the project one to get "project shadows
// global".
func Discover(dirs []string) ([]Skill, error) {
	byName := make(map[string]Skill)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", dir, err)
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			skillDir := filepath.Join(dir, e.Name())
			s, err := load(skillDir)
			if errors.Is(err, errNotASkill) {
				continue
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "skill: skipping %s: %v\n", skillDir, err)
				continue
			}
			byName[s.Name] = s
		}
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	skills := make([]Skill, len(names))
	for i, name := range names {
		skills[i] = byName[name]
	}
	return skills, nil
}

// load reads and validates dir/SKILL.md, returning errNotASkill if there
// is no SKILL.md at all.
func load(dir string) (Skill, error) {
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return Skill{}, errNotASkill
		}
		return Skill{}, err
	}

	fm, err := parseFrontmatter(data)
	if err != nil {
		return Skill{}, err
	}
	if err := validateName(fm.Name, dir); err != nil {
		return Skill{}, err
	}
	if fm.Description == "" {
		return Skill{}, errors.New("description is required")
	}

	return Skill{
		Name:                   fm.Name,
		Description:            fm.Description,
		Path:                   dir,
		License:                fm.License,
		Compatibility:          fm.Compatibility,
		Metadata:               fm.Metadata,
		AllowedTools:           fm.AllowedTools,
		DisableModelInvocation: fm.DisableModelInvocation,
	}, nil
}
