package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGlobalDir_UsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg-config")

	got, err := GlobalDir()
	if err != nil {
		t.Fatalf("GlobalDir: %v", err)
	}

	want := filepath.Join("/xdg-config", "liam", "skills")
	if got != want {
		t.Errorf("GlobalDir = %q, want %q", got, want)
	}
}

func TestGlobalDir_FallsBackToHomeConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/testuser")

	got, err := GlobalDir()
	if err != nil {
		t.Fatalf("GlobalDir: %v", err)
	}

	want := filepath.Join("/home/testuser", ".config", "liam", "skills")
	if got != want {
		t.Errorf("GlobalDir = %q, want %q", got, want)
	}
}

// requireGit skips the test if the git binary isn't on PATH.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestProjectDir_ResolvesAtGitRootFromNestedDir(t *testing.T) {
	requireGit(t)

	root := t.TempDir()
	runGit(t, root, "init")

	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	// Place a skill at the resolved project directory and confirm it's
	// found from the nested working directory — the behavior that
	// matters, rather than asserting on the resolved path string, which
	// can differ from t.TempDir()'s value after symlink resolution
	// (e.g. macOS's /tmp -> /private/tmp).
	projectDir := ProjectDir(nested)
	writeSkill(t, projectDir, "team-skill",
		"name: team-skill\ndescription: A project skill.\n", "body\n")

	skills, err := Discover([]string{projectDir})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "team-skill" {
		t.Fatalf("ProjectDir(%q) = %q, did not resolve to a dir containing team-skill: %+v", nested, projectDir, skills)
	}
}

func TestProjectDir_FallsBackToGivenDirOutsideRepo(t *testing.T) {
	requireGit(t)

	dir := t.TempDir()
	// Deliberately not a git repo.

	projectDir := ProjectDir(dir)
	want := filepath.Join(dir, ".liam", "skills")
	if projectDir != want {
		t.Errorf("ProjectDir(%q) = %q, want %q", dir, projectDir, want)
	}
}
