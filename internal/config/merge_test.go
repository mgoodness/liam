package config

import (
	"reflect"
	"testing"
)

func TestMergeMaps(t *testing.T) {
	cases := []struct {
		name     string
		dst, src map[string]any
		want     map[string]any
	}{
		{
			name: "scalar override",
			dst:  map[string]any{"a": 1.0, "b": 2.0},
			src:  map[string]any{"b": 3.0},
			want: map[string]any{"a": 1.0, "b": 3.0},
		},
		{
			name: "deep nested merge",
			dst: map[string]any{
				"provider": map[string]any{"model": "openrouter/auto", "costTier": "low"},
			},
			src: map[string]any{
				"provider": map[string]any{"model": "openai/gpt-4o"},
			},
			want: map[string]any{
				"provider": map[string]any{"model": "openai/gpt-4o", "costTier": "low"},
			},
		},
		{
			name: "src-only key added",
			dst:  map[string]any{"a": 1.0},
			src:  map[string]any{"c": 4.0},
			want: map[string]any{"a": 1.0, "c": 4.0},
		},
		{
			// A conflicting non-map value in src wins outright rather than
			// being merged into the dst map.
			name: "scalar replaces map",
			dst:  map[string]any{"provider": map[string]any{"model": "openrouter/auto"}},
			src:  map[string]any{"provider": "not-a-map"},
			want: map[string]any{"provider": "not-a-map"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeMaps(tc.dst, tc.src)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("mergeMaps(%#v, %#v) = %#v, want %#v", tc.dst, tc.src, got, tc.want)
			}
		})
	}
}

func TestMergeMapsDoesNotMutateInputs(t *testing.T) {
	dst := map[string]any{"a": 1.0}
	src := map[string]any{"b": 2.0}

	mergeMaps(dst, src)

	if _, ok := dst["b"]; ok {
		t.Errorf("mergeMaps mutated dst: %#v", dst)
	}
	if _, ok := src["a"]; ok {
		t.Errorf("mergeMaps mutated src: %#v", src)
	}
}
