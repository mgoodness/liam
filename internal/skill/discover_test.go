package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverFindsUserScopeSkill(t *testing.T) {
	home := isolateHome(t)

	writeSkill(t, filepath.Join(home, ".agents", "skills"), "my-skill",
		[][2]string{{"name", "my-skill"}, {"description", "Does a thing."}}, "# instructions")

	skills, diags := Discover(Options{Cwd: t.TempDir()})
	if len(diags) != 0 {
		t.Fatalf("diags = %+v, want none", diags)
	}
	if len(skills) != 1 {
		t.Fatalf("skills = %+v, want 1", skills)
	}
	if skills[0].Name != "my-skill" || skills[0].Description != "Does a thing." || skills[0].Body != "# instructions" {
		t.Errorf("skills[0] = %+v, want name/description/body populated", skills[0])
	}
	if skills[0].Scope != ScopeUser {
		t.Errorf("Scope = %q, want %q", skills[0].Scope, ScopeUser)
	}
}

func TestDiscoverIgnoresProjectScopeWhenUntrusted(t *testing.T) {
	isolateHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".agents", "skills"), "proj-skill",
		[][2]string{{"name", "proj-skill"}, {"description", "d"}}, "body")

	skills, _ := Discover(Options{Cwd: cwd, ProjectTrusted: false})
	if len(skills) != 0 {
		t.Fatalf("skills = %+v, want none (project untrusted)", skills)
	}
}

func TestDiscoverScansProjectScopeWhenTrusted(t *testing.T) {
	isolateHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".agents", "skills"), "proj-skill",
		[][2]string{{"name", "proj-skill"}, {"description", "d"}}, "body")
	writeSkill(t, filepath.Join(cwd, ".liam", "skills"), "liam-skill",
		[][2]string{{"name", "liam-skill"}, {"description", "d"}}, "body")

	skills, diags := Discover(Options{Cwd: cwd, ProjectTrusted: true})
	if len(diags) != 0 {
		t.Fatalf("diags = %+v, want none", diags)
	}
	if len(skills) != 2 {
		t.Fatalf("skills = %+v, want 2", skills)
	}
}

func TestDiscoverProjectOverridesUserOnNameCollision(t *testing.T) {
	home := isolateHome(t)
	writeSkill(t, filepath.Join(home, ".agents", "skills"), "shared",
		[][2]string{{"name", "shared"}, {"description", "user version"}}, "user body")

	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".agents", "skills"), "shared",
		[][2]string{{"name", "shared"}, {"description", "project version"}}, "project body")

	skills, _ := Discover(Options{Cwd: cwd, ProjectTrusted: true})
	if len(skills) != 1 {
		t.Fatalf("skills = %+v, want 1 (merged by name)", skills)
	}
	if skills[0].Description != "project version" || skills[0].Scope != ScopeProject {
		t.Errorf("skills[0] = %+v, want the project-scope version to win", skills[0])
	}
}

func TestDiscoverExtraPathsScannedUnconditionally(t *testing.T) {
	isolateHome(t)
	extra := t.TempDir()
	writeSkill(t, extra, "extra-skill", [][2]string{{"name", "extra-skill"}, {"description", "d"}}, "body")

	skills, _ := Discover(Options{Cwd: t.TempDir(), ExtraPaths: []string{extra}})
	if len(skills) != 1 || skills[0].Name != "extra-skill" {
		t.Fatalf("skills = %+v, want extra-skill", skills)
	}
}

func TestDiscoverRespectsDisabledConfig(t *testing.T) {
	home := isolateHome(t)
	writeSkill(t, filepath.Join(home, ".agents", "skills"), "on",
		[][2]string{{"name", "on"}, {"description", "d"}}, "body")
	writeSkill(t, filepath.Join(home, ".agents", "skills"), "off",
		[][2]string{{"name", "off"}, {"description", "d"}}, "body")

	skills, _ := Discover(Options{Cwd: t.TempDir(), Disabled: []string{"off"}})
	if len(skills) != 1 || skills[0].Name != "on" {
		t.Fatalf("skills = %+v, want only \"on\"", skills)
	}
}

func TestDiscoverSkipsMissingDescription(t *testing.T) {
	home := isolateHome(t)
	writeSkill(t, filepath.Join(home, ".agents", "skills"), "bad", [][2]string{{"name", "bad"}}, "body")

	skills, diags := Discover(Options{Cwd: t.TempDir()})
	if len(skills) != 0 {
		t.Fatalf("skills = %+v, want none", skills)
	}
	if len(diags) != 1 || diags[0].Level != DiagnosticSkip {
		t.Fatalf("diags = %+v, want 1 skip diagnostic", diags)
	}
}

func TestDiscoverWarnsOnNameDirectoryMismatchButStillLoads(t *testing.T) {
	home := isolateHome(t)
	writeSkill(t, filepath.Join(home, ".agents", "skills"), "dir-name",
		[][2]string{{"name", "different-name"}, {"description", "d"}}, "body")

	skills, diags := Discover(Options{Cwd: t.TempDir()})
	if len(skills) != 1 || skills[0].Name != "different-name" {
		t.Fatalf("skills = %+v, want the mismatched skill still loaded", skills)
	}
	if len(diags) != 1 || diags[0].Level != DiagnosticWarn {
		t.Fatalf("diags = %+v, want 1 warn diagnostic", diags)
	}
}

func TestDiscoverExcludesSkillWithHiddenCharactersDistinctFromTrustDenial(t *testing.T) {
	home := isolateHome(t)
	malicious := "Looks normal" + string(rune(0x200B)) + "but isn't."
	writeSkill(t, filepath.Join(home, ".agents", "skills"), "sneaky",
		[][2]string{{"name", "sneaky"}, {"description", malicious}}, "body")

	skills, diags := Discover(Options{Cwd: t.TempDir()})
	if len(skills) != 0 {
		t.Fatalf("skills = %+v, want none (excluded by security scan)", skills)
	}
	if len(diags) != 1 {
		t.Fatalf("diags = %+v, want 1", diags)
	}
	if diags[0].Level != DiagnosticSkip {
		t.Errorf("Level = %q, want %q", diags[0].Level, DiagnosticSkip)
	}
	if !strings.Contains(diags[0].Message, "hidden characters") {
		t.Errorf("Message = %q, want it to mention hidden characters (distinct from an untrusted-directory denial)", diags[0].Message)
	}
}

// TestDiscoverLoadsDisableModelInvocationSkillButModelCatalogExcludesIt
// covers the non-spec disable-model-invocation frontmatter boolean: the
// skill still loads (Find can still force-activate it directly), but
// ModelCatalog — what activate_skill's catalog is built from — excludes
// it, matching Claude Code's own behavior for the same field.
func TestDiscoverLoadsDisableModelInvocationSkillButModelCatalogExcludesIt(t *testing.T) {
	home := isolateHome(t)
	writeSkill(t, filepath.Join(home, ".agents", "skills"), "hidden",
		[][2]string{{"name", "hidden"}, {"description", "d"}, {"disable-model-invocation", "true"}}, "body")

	skills, _ := Discover(Options{Cwd: t.TempDir()})
	if len(skills) != 1 || !skills[0].DisableModelInvocation {
		t.Fatalf("skills = %+v, want 1 skill with DisableModelInvocation = true", skills)
	}
	if _, found := Find(skills, "hidden"); !found {
		t.Error("Find() didn't find the disabled skill; it should still be force-activatable")
	}
	if catalog := ModelCatalog(skills); len(catalog) != 0 {
		t.Errorf("ModelCatalog() = %+v, want none (excluded)", catalog)
	}
}

func TestProjectDirsWalksFromGitRootToCwd(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("Mkdir .git: %v", err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	dirs := ProjectDirs(sub)
	want := []string{
		filepath.Join(root, ".agents", "skills"),
		filepath.Join(root, ".liam", "skills"),
		filepath.Join(root, "a", ".agents", "skills"),
		filepath.Join(root, "a", ".liam", "skills"),
		filepath.Join(sub, ".agents", "skills"),
		filepath.Join(sub, ".liam", "skills"),
	}
	if len(dirs) != len(want) {
		t.Fatalf("ProjectDirs() = %v, want %v", dirs, want)
	}
	for i := range want {
		if dirs[i] != want[i] {
			t.Errorf("ProjectDirs()[%d] = %q, want %q", i, dirs[i], want[i])
		}
	}
}

func TestProjectRootFallsBackToCwdWithoutGit(t *testing.T) {
	cwd := t.TempDir()
	if got := ProjectRoot(cwd); got != cwd {
		t.Errorf("ProjectRoot() = %q, want %q", got, cwd)
	}
}
