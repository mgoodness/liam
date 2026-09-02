package render

import "testing"

func TestPluralize(t *testing.T) {
	if got := Pluralize(1, "skill", "skills"); got != "skill" {
		t.Errorf("Pluralize(1, ...) = %q, want %q", got, "skill")
	}
	if got := Pluralize(0, "skill", "skills"); got != "skills" {
		t.Errorf("Pluralize(0, ...) = %q, want %q", got, "skills")
	}
	if got := Pluralize(2, "skill", "skills"); got != "skills" {
		t.Errorf("Pluralize(2, ...) = %q, want %q", got, "skills")
	}
}
