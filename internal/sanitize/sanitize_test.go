package sanitize

import "testing"

func TestCleanTextStripsPersianCombiningModifiers(t *testing.T) {
	t.Parallel()

	got := CleanText("بوسهٔ")
	want := "بوسه"

	if got != want {
		t.Fatalf("CleanText() = %q, want %q", got, want)
	}
}

func TestStripModifierCharactersKeepsBaseLetters(t *testing.T) {
	t.Parallel()

	got := StripModifierCharacters("بوسهٔ")
	want := "بوسه"

	if got != want {
		t.Fatalf("StripModifierCharacters() = %q, want %q", got, want)
	}
}
