package commerce

import "testing"

// One visible line holds no character a display leaves out. The whole list is compared with the database's, code
// point by code point, in TestPostgresHiddenCharactersAreTheSameInOpsdAndTheDatabase; this holds the corners
// without one.
func TestVisibleLine(t *testing.T) {
	for _, text := range []string{"", "여울 커피", "Café №1 ☕ 営業中", "a b", "Wi‑Fi"} {
		if !visibleLine(text) {
			t.Errorf("%q is refused", text)
		}
	}
	for _, text := range []string{" ", "   ", "a⁥b", "a⁠b", "a⁯b", "a​b", "a b", "aㅤb", "a️b", "a⠀b", "a\nb", "a\U000E0041b"} {
		if visibleLine(text) {
			t.Errorf("%q is taken as one visible line", text)
		}
	}
}
