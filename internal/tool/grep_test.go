package tool

import (
	"context"
	"testing"
)

func TestGrepSafety(t *testing.T) {
	want := Safety{SideEffect: SideEffectRead}
	if got := (Grep{}).Safety(); got != want {
		t.Errorf("Safety() = %+v, want %+v", got, want)
	}
}

func TestGrepRunMissingQueryArg(t *testing.T) {
	got := Grep{Searcher: StdlibSearch{}}.Run(context.Background(), map[string]any{})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}

func TestGrepRunSurfacesSearcherError(t *testing.T) {
	got := Grep{Searcher: StdlibSearch{}}.Run(context.Background(), map[string]any{"query": "(unclosed"})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}
