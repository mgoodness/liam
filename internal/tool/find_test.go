package tool

import (
	"context"
	"testing"
)

func TestFindSafety(t *testing.T) {
	want := Safety{SideEffect: SideEffectRead}
	if got := (Find{}).Safety(); got != want {
		t.Errorf("Safety() = %+v, want %+v", got, want)
	}
}

func TestFindRunEmptyQueryLists(t *testing.T) {
	got := Find{Searcher: StdlibSearch{Dir: "testdata/fixture_find"}}.Run(context.Background(), map[string]any{})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if got.Content == "" {
		t.Error("Content is empty, want a listing of every file")
	}
}
