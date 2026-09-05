package instructions

import (
	"strings"
	"testing"
)

func TestPreambleIsLiamsFixedIdentityBlock(t *testing.T) {
	if !strings.HasPrefix(Preamble, "You are Liam, an agentic CLI coding assistant.") {
		t.Errorf("Preamble = %q, want it to open by naming Liam", Preamble)
	}
	if !strings.Contains(Preamble, "\n\nGuidelines:\n") {
		t.Error(`Preamble should separate its role sentence from a "Guidelines:" block with a blank line`)
	}
}

// TestPreambleCarriesNoPermissionLanguage guards ADR-0004's "no built-in
// permission system" constraint: the preamble must never imply liam has a
// confirmation/gating mechanism it doesn't have.
func TestPreambleCarriesNoPermissionLanguage(t *testing.T) {
	lower := strings.ToLower(Preamble)
	for _, forbidden := range []string{"permission", "confirm", "ask before", "approval"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("Preamble contains %q, want no caution/confirmation/permission language (ADR-0004)", forbidden)
		}
	}
}

// TestPreambleDiscouragesPrematureStop guards against the model treating a
// turn as finished once it's merely investigated/reported back, without
// having made the change the request actually called for.
func TestPreambleDiscouragesPrematureStop(t *testing.T) {
	lower := strings.ToLower(Preamble)
	for _, want := range []string{"until", "fully resolved"} {
		if !strings.Contains(lower, want) {
			t.Errorf("Preamble = %q, want it to contain %q (guard against ending a turn before the task is actually done)", Preamble, want)
		}
	}
}
