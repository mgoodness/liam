package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSkill writes a SKILL.md at dir/name/SKILL.md with the given
// frontmatter fields (rendered as literal "key: value" lines, in order)
// and body, failing the test on error, and returns the skill's directory.
func writeSkill(t *testing.T, dir, name string, frontmatter [][2]string, body string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	content := "---\n"
	for _, kv := range frontmatter {
		content += kv[0] + ": " + kv[1] + "\n"
	}
	content += "---\n" + body

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return skillDir
}

// isolateHome points HOME and XDG_CONFIG_HOME at fresh temp dirs so tests
// never pick up the real developer machine's ~/.agents/skills or
// $XDG_CONFIG_HOME/liam/skills, and returns the fresh HOME so callers
// don't need their own os.UserHomeDir() round trip.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	return home
}
