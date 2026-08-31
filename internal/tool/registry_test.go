package tool

import (
	"context"
	"testing"
)

type stubTool struct{ name string }

func (s stubTool) Name() string                             { return s.name }
func (stubTool) Description() string                        { return "stub" }
func (stubTool) Parameters() Schema                         { return Schema{} }
func (stubTool) Safety() Safety                             { return Safety{} }
func (stubTool) Run(context.Context, map[string]any) Result { return Result{} }

func TestNewRegistryKeysByName(t *testing.T) {
	reg := NewRegistry(stubTool{name: "a"}, stubTool{name: "b"})

	if len(reg) != 2 {
		t.Fatalf("len(reg) = %d, want 2", len(reg))
	}
	if _, ok := reg["a"]; !ok {
		t.Error(`reg["a"] missing`)
	}
	if _, ok := reg["b"]; !ok {
		t.Error(`reg["b"] missing`)
	}
}

func TestNewRegistryLaterDuplicateWins(t *testing.T) {
	first := stubTool{name: "a"}
	second := stubTool{name: "a"}
	reg := NewRegistry(first, second)

	if len(reg) != 1 {
		t.Fatalf("len(reg) = %d, want 1", len(reg))
	}
}
