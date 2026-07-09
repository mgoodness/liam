package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSkill creates dir/name/SKILL.md with the given frontmatter+body,
// returning the skill directory path.
func writeSkill(t *testing.T, dir, name, frontmatter, body string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "---\n" + frontmatter + "---\n" + body
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("writing SKILL.md: %v", err)
	}
	return skillDir
}

func TestDiscover_ParsesOptionalFrontmatterFields(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "pdf-processing", `name: pdf-processing
description: A description.
license: Apache-2.0
compatibility: Requires git, docker, jq, and access to the internet
metadata:
  author: example-org
  version: "1.0"
allowed-tools: Bash(git:*) Bash(jq:*) Read
`, "body\n")

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1: %+v", len(skills), skills)
	}

	got := skills[0]
	if got.License != "Apache-2.0" {
		t.Errorf("License = %q", got.License)
	}
	if got.Compatibility != "Requires git, docker, jq, and access to the internet" {
		t.Errorf("Compatibility = %q", got.Compatibility)
	}
	wantMetadata := map[string]string{"author": "example-org", "version": "1.0"}
	if len(got.Metadata) != len(wantMetadata) || got.Metadata["author"] != wantMetadata["author"] || got.Metadata["version"] != wantMetadata["version"] {
		t.Errorf("Metadata = %+v, want %+v", got.Metadata, wantMetadata)
	}
	if got.AllowedTools != "Bash(git:*) Bash(jq:*) Read" {
		t.Errorf("AllowedTools = %q", got.AllowedTools)
	}
}

func TestDiscover_SkipsInvalidNamesWithoutFailing(t *testing.T) {
	cases := []struct {
		name    string // subtest name
		dirName string
		fmName  string
	}{
		{"uppercase", "PDF-Processing", "PDF-Processing"},
		{"leading hyphen", "-pdf-processing", "-pdf-processing"},
		{"trailing hyphen", "pdf-processing-", "pdf-processing-"},
		{"consecutive hyphens", "pdf--processing", "pdf--processing"},
		{"over 64 characters", strings.Repeat("a", 65), strings.Repeat("a", 65)},
		{"name does not match parent directory", "pdf-processing", "other-name"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSkill(t, dir, c.dirName,
				fmt.Sprintf("name: %s\ndescription: A description.\n", c.fmName),
				"body\n")

			skills, err := Discover([]string{dir})
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if len(skills) != 0 {
				t.Errorf("got %d skills, want 0 (invalid skill should be skipped): %+v", len(skills), skills)
			}
		})
	}
}

func TestDiscover_LaterDirWinsNameCollision(t *testing.T) {
	global := t.TempDir()
	project := t.TempDir()
	writeSkill(t, global, "pdf-processing",
		"name: pdf-processing\ndescription: The global version.\n", "body\n")
	writeSkill(t, project, "pdf-processing",
		"name: pdf-processing\ndescription: The project version.\n", "body\n")

	skills, err := Discover([]string{global, project})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1: %+v", len(skills), skills)
	}
	if got := skills[0].Description; got != "The project version." {
		t.Errorf("Description = %q, want the project skill to win the collision", got)
	}
	if got := skills[0].Path; got != filepath.Join(project, "pdf-processing") {
		t.Errorf("Path = %q, want the project skill's path", got)
	}
}

func TestDiscover_FindsValidSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "pdf-processing",
		"name: pdf-processing\ndescription: Extract PDF text, fill forms, merge files. Use when handling PDFs.\n",
		"# PDF Processing\n\nInstructions here.\n")

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1: %+v", len(skills), skills)
	}

	got := skills[0]
	if got.Name != "pdf-processing" {
		t.Errorf("Name = %q, want %q", got.Name, "pdf-processing")
	}
	if got.Description != "Extract PDF text, fill forms, merge files. Use when handling PDFs." {
		t.Errorf("Description = %q", got.Description)
	}
	if got.Path != filepath.Join(dir, "pdf-processing") {
		t.Errorf("Path = %q, want %q", got.Path, filepath.Join(dir, "pdf-processing"))
	}
}
