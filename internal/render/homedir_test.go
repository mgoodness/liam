package render

import "testing"

func TestCollapseHome(t *testing.T) {
	t.Setenv("HOME", "/home/liam")

	cases := []struct {
		name string
		path string
		want string
	}{
		{"exact home dir", "/home/liam", "~"},
		{"path inside home dir", "/home/liam/project/main.go", "~/project/main.go"},
		{"path outside home dir", "/etc/hosts", "/etc/hosts"},
		{"sibling dir sharing a prefix, not actually inside home", "/home/liam2/project", "/home/liam2/project"},
		{"empty path", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CollapseHome(tc.path); got != tc.want {
				t.Errorf("CollapseHome(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
