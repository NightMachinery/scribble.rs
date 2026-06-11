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

func TestCleanTextStripsPunctuationAndSymbols(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ascii punctuation and symbols",
			input: "a&b.c=d",
			want:  "abcd",
		},
		{
			name:  "unicode punctuation and symbols",
			input: "a؟b★c",
			want:  "abc",
		},
		{
			name:  "letters and numbers stay guessable",
			input: "abc123",
			want:  "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := CleanText(tt.input); got != tt.want {
				t.Fatalf("CleanText() = %q, want %q", got, tt.want)
			}
		})
	}
}
