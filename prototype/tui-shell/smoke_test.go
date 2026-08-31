// PROTOTYPE — throwaway smoke test, not part of liam. Just checks View()
// doesn't panic across every variant/theme/prompt combination.
package main

import "testing"

func TestAllCombinationsRender(t *testing.T) {
	for _, dark := range []bool{true, false} {
		for _, showPrompt := range []bool{true, false} {
			for v := variantA; v < variantCount; v++ {
				m := model{variant: v, dark: dark, showPrompt: showPrompt, width: 100, height: 30}
				out := m.View()
				if len(out) == 0 {
					t.Fatalf("empty output for variant=%v dark=%v showPrompt=%v", v, dark, showPrompt)
				}
			}
		}
	}
}

func TestNarrowTerminal(t *testing.T) {
	for v := variantA; v < variantCount; v++ {
		m := model{variant: v, dark: true, showPrompt: true, width: 40, height: 15}
		_ = m.View()
	}
}
