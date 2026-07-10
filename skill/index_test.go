package skill

import (
	"strings"
	"testing"
)

func TestIndex_ExcludesDisableModelInvocation(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "pdf-processing",
		"name: pdf-processing\ndescription: Handles PDFs.\n",
		"body\n")
	writeSkill(t, dir, "internal-only",
		"name: internal-only\ndescription: Only invoked explicitly.\ndisable-model-invocation: true\n",
		"body\n")

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("got %d skills, want 2 (disable-model-invocation must still be in Discover's result for future /name invocation): %+v", len(skills), skills)
	}

	index := Index(skills)
	if !strings.Contains(index, "pdf-processing") {
		t.Errorf("Index missing eligible skill pdf-processing:\n%s", index)
	}
	if strings.Contains(index, "internal-only") {
		t.Errorf("Index must exclude disable-model-invocation skill internal-only:\n%s", index)
	}
}
