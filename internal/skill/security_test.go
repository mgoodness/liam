package skill

import "testing"

func TestScanHiddenCleanContentFindsNothing(t *testing.T) {
	if got := ScanHidden("---\nname: s\ndescription: a normal description\n---\nordinary body text.\n"); len(got) != 0 {
		t.Errorf("ScanHidden() = %+v, want none", got)
	}
}

func TestScanHiddenDetectsZeroWidthCharacter(t *testing.T) {
	content := "normal text" + string(rune(0x200B)) + "hidden"
	got := ScanHidden(content)
	if len(got) != 1 {
		t.Fatalf("ScanHidden() = %+v, want 1 finding", got)
	}
	if got[0].Rune != rune(0x200B) {
		t.Errorf("Rune = %U, want U+200B", got[0].Rune)
	}
}

func TestScanHiddenDetectsBiDiOverride(t *testing.T) {
	content := "safe" + string(rune(0x202E)) + "malicious" + string(rune(0x202C))
	got := ScanHidden(content)
	if len(got) != 2 {
		t.Fatalf("ScanHidden() = %+v, want 2 findings", got)
	}
}

func TestScanHiddenDetectsUnicodeTagCharacter(t *testing.T) {
	content := "hi" + string(rune(0xE0001)) + "there"
	got := ScanHidden(content)
	if len(got) != 1 {
		t.Fatalf("ScanHidden() = %+v, want 1 finding", got)
	}
	if got[0].Name != "Unicode tag character" {
		t.Errorf("Name = %q, want %q", got[0].Name, "Unicode tag character")
	}
}

func TestScanHiddenDetectsBiDiIsolate(t *testing.T) {
	content := "safe" + string(rune(0x2066)) + "isolated" + string(rune(0x2069))
	got := ScanHidden(content)
	if len(got) != 2 {
		t.Fatalf("ScanHidden() = %+v, want 2 findings", got)
	}
}
