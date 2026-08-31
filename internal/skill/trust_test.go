package skill

import (
	"errors"
	"testing"
)

func newTestStore(t *testing.T) *TrustStore {
	t.Helper()
	isolateHome(t)
	store, err := OpenTrustStore()
	if err != nil {
		t.Fatalf("OpenTrustStore: %v", err)
	}
	return store
}

func TestTrustStoreRecordAndDecision(t *testing.T) {
	store := newTestStore(t)

	if _, decided, err := store.Decision("/some/project"); err != nil || decided {
		t.Fatalf("Decision() = (_, %v, %v), want (_, false, nil) before any Record", decided, err)
	}

	if err := store.Record("/some/project", true); err != nil {
		t.Fatalf("Record: %v", err)
	}
	trusted, decided, err := store.Decision("/some/project")
	if err != nil {
		t.Fatalf("Decision: %v", err)
	}
	if !decided || !trusted {
		t.Errorf("Decision() = (%v, %v), want (true, true)", trusted, decided)
	}
}

func TestTrustStorePersistsAcrossInstances(t *testing.T) {
	isolateHome(t)
	store1, err := OpenTrustStore()
	if err != nil {
		t.Fatalf("OpenTrustStore: %v", err)
	}
	if err := store1.Record("/proj", false); err != nil {
		t.Fatalf("Record: %v", err)
	}

	store2, err := OpenTrustStore()
	if err != nil {
		t.Fatalf("OpenTrustStore: %v", err)
	}
	trusted, decided, err := store2.Decision("/proj")
	if err != nil {
		t.Fatalf("Decision: %v", err)
	}
	if !decided || trusted {
		t.Errorf("Decision() = (%v, %v), want (false, true)", trusted, decided)
	}
}

func TestResolveProjectTrustUsesPersistedDecisionFirst(t *testing.T) {
	store := newTestStore(t)
	if err := store.Record("/proj", true); err != nil {
		t.Fatalf("Record: %v", err)
	}

	override := false
	trusted, err := ResolveProjectTrust(store, "/proj", &override, nil)
	if err != nil {
		t.Fatalf("ResolveProjectTrust: %v", err)
	}
	if !trusted {
		t.Errorf("trusted = %v, want true (persisted decision wins over override)", trusted)
	}
}

func TestResolveProjectTrustUsesOverrideWhenNoPersistedDecision(t *testing.T) {
	store := newTestStore(t)

	override := true
	trusted, err := ResolveProjectTrust(store, "/proj", &override, nil)
	if err != nil {
		t.Fatalf("ResolveProjectTrust: %v", err)
	}
	if !trusted {
		t.Errorf("trusted = %v, want true", trusted)
	}

	// The override decision is persisted for next time.
	persisted, decided, err := store.Decision("/proj")
	if err != nil {
		t.Fatalf("Decision: %v", err)
	}
	if !decided || !persisted {
		t.Errorf("Decision() = (%v, %v), want (true, true) after an override decision", persisted, decided)
	}
}

func TestResolveProjectTrustCallsPromptWhenNoPersistedDecisionOrOverride(t *testing.T) {
	store := newTestStore(t)

	var askedQuestion string
	prompt := func(q string) (bool, error) {
		askedQuestion = q
		return true, nil
	}

	trusted, err := ResolveProjectTrust(store, "/proj", nil, prompt)
	if err != nil {
		t.Fatalf("ResolveProjectTrust: %v", err)
	}
	if !trusted {
		t.Errorf("trusted = %v, want true", trusted)
	}
	if askedQuestion == "" {
		t.Error("prompt was never called")
	}

	persisted, decided, err := store.Decision("/proj")
	if err != nil {
		t.Fatalf("Decision: %v", err)
	}
	if !decided || !persisted {
		t.Errorf("Decision() = (%v, %v), want (true, true) after a prompt answer", persisted, decided)
	}
}

// TestResolveProjectTrustHeadlessDefaultsUntrustedWithoutPersisting is the
// headless-mode equivalent for the interactive trust prompt: no override,
// no prompt (nil, as in headless mode) — the safe default is "don't
// load", and crucially that default is NOT persisted, so a later
// interactive run can still ask.
func TestResolveProjectTrustHeadlessDefaultsUntrustedWithoutPersisting(t *testing.T) {
	store := newTestStore(t)

	trusted, err := ResolveProjectTrust(store, "/proj", nil, nil)
	if err != nil {
		t.Fatalf("ResolveProjectTrust: %v", err)
	}
	if trusted {
		t.Errorf("trusted = %v, want false (safe default)", trusted)
	}

	if _, decided, err := store.Decision("/proj"); err != nil || decided {
		t.Errorf("Decision() decided = %v, want false (silent default must not be persisted)", decided)
	}
}

func TestResolveProjectTrustWorksWithNilStore(t *testing.T) {
	override := true
	trusted, err := ResolveProjectTrust(nil, "/proj", &override, nil)
	if err != nil {
		t.Fatalf("ResolveProjectTrust: %v", err)
	}
	if !trusted {
		t.Errorf("trusted = %v, want true", trusted)
	}
}

func TestResolveProjectTrustPropagatesPromptError(t *testing.T) {
	store := newTestStore(t)
	wantErr := errors.New("boom")
	prompt := func(string) (bool, error) { return false, wantErr }

	_, err := ResolveProjectTrust(store, "/proj", nil, prompt)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResolveProjectTrust() error = %v, want %v", err, wantErr)
	}
}
