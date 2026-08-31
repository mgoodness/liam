package theme

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		name   string
		mode   string
		isDark bool
		want   Palette
	}{
		{"dark mode forces Frappe regardless of detection", "dark", false, Frappe},
		{"light mode forces Latte regardless of detection", "light", true, Latte},
		{"auto mode follows detection: dark", "auto", true, Frappe},
		{"auto mode follows detection: light", "auto", false, Latte},
		{"empty mode defaults to auto: dark", "", true, Frappe},
		{"empty mode defaults to auto: light", "", false, Latte},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.mode, tc.isDark)
			if got != tc.want {
				t.Errorf("Resolve(%q, %v) = %+v, want %+v", tc.mode, tc.isDark, got, tc.want)
			}
		})
	}
}
