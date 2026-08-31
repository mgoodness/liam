package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Options configures Discover.
type Options struct {
	// Cwd is the current working directory, used to locate project-scope
	// skill directories (walked from the git root down to Cwd).
	Cwd string
	// ExtraPaths are additional directories to scan unconditionally
	// (skills.paths config) — not subject to the project trust gate,
	// since listing one here is itself an explicit user opt-in.
	ExtraPaths []string
	// Disabled lists skill names to exclude entirely (skills.disabled
	// config), regardless of where they were discovered.
	Disabled []string
	// ProjectTrusted gates whether project-scope directories are scanned
	// at all. The caller resolves this via ResolveProjectTrust before
	// calling Discover — Discover itself has no trust logic.
	ProjectTrusted bool
}

// Discover finds skills across user scope, ExtraPaths, and (if
// ProjectTrusted) project scope, returning the merged catalog — project
// wins over user/extra on a name collision — plus diagnostics for
// anything skipped or worth a warning along the way.
func Discover(opts Options) ([]Skill, []Diagnostic) {
	var diags []Diagnostic
	byName := map[string]Skill{}

	scan := func(dirs []string, scope Scope) {
		for _, root := range dirs {
			entries, err := os.ReadDir(root)
			if err != nil {
				continue // absent/unreadable dir is the normal case, not worth a diagnostic
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				s, diag, ok := loadSkillDir(filepath.Join(root, e.Name()), scope)
				if diag != nil {
					diags = append(diags, *diag)
				}
				if ok {
					byName[s.Name] = s
				}
			}
		}
	}

	// Order matters: later scans overwrite earlier ones on a name
	// collision, so project (scanned last) wins over extra and user.
	scan(userDirs(), ScopeUser)
	scan(opts.ExtraPaths, ScopeExtra)
	if opts.ProjectTrusted {
		scan(ProjectDirs(opts.Cwd), ScopeProject)
	}

	disabled := make(map[string]bool, len(opts.Disabled))
	for _, n := range opts.Disabled {
		disabled[n] = true
	}

	skills := make([]Skill, 0, len(byName))
	for name, s := range byName {
		if disabled[name] {
			continue
		}
		skills = append(skills, s)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })

	return skills, diags
}

// userDirs returns the two user-scope default skill directories:
// ~/.agents/skills (cross-client convention) and
// $XDG_CONFIG_HOME/liam/skills (liam's own).
func userDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".agents", "skills"))
	}
	if configHome, err := xdgConfigHome(); err == nil {
		dirs = append(dirs, filepath.Join(configHome, "liam", "skills"))
	}
	return dirs
}

// ProjectDirs returns the project-scope candidate skill directories
// (.agents/skills and .liam/skills) for every directory from the git root
// down to cwd — monorepo support, per the agentskills.io implementation
// guide. Directories are returned whether or not they exist; use
// HasProjectSkills to check first.
func ProjectDirs(cwd string) []string {
	root := ProjectRoot(cwd)
	chain := dirChain(root, cwd)
	dirs := make([]string, 0, len(chain)*2)
	for _, d := range chain {
		dirs = append(dirs, filepath.Join(d, ".agents", "skills"), filepath.Join(d, ".liam", "skills"))
	}
	return dirs
}

// HasProjectSkills reports whether any project-scope skill directory
// exists under cwd, regardless of whether it holds any valid SKILL.md —
// existence alone is enough to warrant a trust decision before scanning.
func HasProjectSkills(cwd string) bool {
	for _, d := range ProjectDirs(cwd) {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// ProjectRoot walks up from cwd looking for a .git entry (a directory for
// an ordinary repo, or a file for a worktree/submodule), returning the
// first directory found — or cwd itself if none is found up to the
// filesystem root.
func ProjectRoot(cwd string) string {
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}

// dirChain returns every directory from root down to cwd, root first —
// []string{root} if cwd isn't under root.
func dirChain(root, cwd string) []string {
	rel, err := filepath.Rel(root, cwd)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return []string{root}
	}
	parts := strings.Split(rel, string(filepath.Separator))
	chain := make([]string, 0, len(parts)+1)
	d := root
	chain = append(chain, d)
	for _, p := range parts {
		d = filepath.Join(d, p)
		chain = append(chain, d)
	}
	return chain
}

// loadSkillDir loads dir as a candidate skill directory, running the
// hidden-character security scan before parsing frontmatter. ok is false
// when dir isn't a skill at all (no SKILL.md — the normal case for most
// subdirectories) or was excluded; diag, when non-nil, explains why (a
// skip) or flags a lenient-parsing warning on an otherwise-loaded skill.
func loadSkillDir(dir string, scope Scope) (s Skill, diag *Diagnostic, ok bool) {
	mdPath := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return Skill{}, nil, false
	}

	if findings := ScanHidden(string(data)); len(findings) > 0 {
		return Skill{}, &Diagnostic{
			Path:    mdPath,
			Level:   DiagnosticSkip,
			Message: fmt.Sprintf("hidden characters detected (possible prompt injection), excluded from loading: %s", describeFindings(findings)),
		}, false
	}

	fields, body, err := parseFrontmatter(data)
	if err != nil {
		return Skill{}, &Diagnostic{
			Path:    mdPath,
			Level:   DiagnosticSkip,
			Message: fmt.Sprintf("unparsable frontmatter, excluded from loading: %v", err),
		}, false
	}
	description := fields["description"]
	if description == "" {
		return Skill{}, &Diagnostic{
			Path:    mdPath,
			Level:   DiagnosticSkip,
			Message: `missing or empty "description" frontmatter field, excluded from loading`,
		}, false
	}

	name := fields["name"]
	dirName := filepath.Base(dir)
	if name == "" {
		name = dirName
	}

	s = Skill{
		Name:                   name,
		Description:            description,
		DisableModelInvocation: fields["disable-model-invocation"] == "true",
		Scope:                  scope,
		Dir:                    dir,
		Path:                   mdPath,
		Body:                   body,
	}

	if name != dirName {
		diag = &Diagnostic{
			Path:    mdPath,
			Level:   DiagnosticWarn,
			Message: fmt.Sprintf("frontmatter name %q does not match directory name %q", name, dirName),
		}
	}

	return s, diag, true
}
