package game

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func Test_wordListsContainNoCarriageReturns(t *testing.T) {
	t.Parallel()

	for _, entry := range WordpackDataByName {
		fileName := entry.FileName
		fileBytes, err := wordFS.ReadFile("words/" + fileName)
		if err != nil {
			t.Errorf("wordpack file '%s' could not be read: %s", fileName, err)
		} else if bytes.ContainsRune(fileBytes, '\r') {
			t.Errorf("wordpack file '%s' contains a carriage return", fileName)
		}
	}
}

func Test_readWordList(t *testing.T) {
	t.Parallel()

	t.Run("test invalid language file", func(t *testing.T) {
		t.Parallel()

		words, err := readDefaultWordList(cases.Lower(language.English), "nonexistent")
		assert.ErrorIs(t, err, ErrUnknownWordpack)
		assert.Empty(t, words)
	})

	for wordpack := range WordpackDataByName {
		t.Run(wordpack, func(t *testing.T) {
			t.Parallel()

			testWordList(t, wordpack)
			testWordList(t, wordpack)
		})
	}
}

func testWordList(t *testing.T, chosenWordpack string) {
	t.Helper()

	lowercaser := WordpackDataByName[chosenWordpack].Lowercaser()
	words, err := readDefaultWordList(lowercaser, chosenWordpack)
	if err != nil {
		t.Errorf("Error reading wordpack %s: %s", chosenWordpack, err)
	}

	if len(words) == 0 {
		t.Errorf("Wordlist for wordpack %s was empty.", chosenWordpack)
	}

	for _, word := range words {
		if word == "" {
			// We can't print the faulty line, since we are shuffling
			// the words in order to avoid predictability.
			t.Errorf("Wordlist for wordpack %s contained empty word", chosenWordpack)
		}

		if strings.TrimSpace(word) != word {
			t.Errorf("Word has surrounding whitespace characters: '%s'", word)
		}

		if lowercaser.String(word) != word {
			t.Errorf("Word hasn't been lowercased: '%s'", word)
		}
	}
}

func Test_getRandomWords(t *testing.T) {
	t.Parallel()

	t.Run("Test getRandomWords with 3 words in list", func(t *testing.T) {
		t.Parallel()

		lobby := &Lobby{
			CurrentWord: "",
			EditableLobbySettings: EditableLobbySettings{
				CustomWordsPerTurn: 0,
			},
			words: []string{"a", "b", "c"},
		}

		randomWords := GetRandomWords(3, lobby)
		for _, lobbyWord := range lobby.words {
			if !slices.Contains(randomWords, lobbyWord) {
				t.Errorf("Random words %s, didn't contain lobbyWord %s", randomWords, lobbyWord)
			}
		}
	})

	t.Run("Test getRandomWords with 3 words in list and 3 more in custom word list, but with 0 chance", func(t *testing.T) {
		t.Parallel()

		lobby := &Lobby{
			CurrentWord: "",
			words:       []string{"a", "b", "c"},
			EditableLobbySettings: EditableLobbySettings{
				CustomWordsPerTurn: 0,
			},

			CustomWords: []string{"d", "e", "f"},
		}

		randomWords := GetRandomWords(3, lobby)
		for _, lobbyWord := range lobby.words {
			if !slices.Contains(randomWords, lobbyWord) {
				t.Errorf("Random words %s, didn't contain lobbyWord %s", randomWords, lobbyWord)
			}
		}
	})

	t.Run("Test getRandomWords with 3 words in list and 100% custom word chance, but without custom words", func(t *testing.T) {
		t.Parallel()

		lobby := &Lobby{
			CurrentWord: "",
			words:       []string{"a", "b", "c"},
			EditableLobbySettings: EditableLobbySettings{
				CustomWordsPerTurn: 3,
			},
			CustomWords: nil,
		}

		randomWords := GetRandomWords(3, lobby)
		for _, lobbyWord := range lobby.words {
			if !slices.Contains(randomWords, lobbyWord) {
				t.Errorf("Random words %s, didn't contain lobbyWord %s", randomWords, lobbyWord)
			}
		}
	})

	t.Run("Test getRandomWords with 3 words in list and 100% custom word chance, with 3 custom words", func(t *testing.T) {
		t.Parallel()

		lobby := &Lobby{
			CurrentWord: "",
			words:       []string{"a", "b", "c"},
			EditableLobbySettings: EditableLobbySettings{
				CustomWordsPerTurn: 3,
			},
			CustomWords: []string{"d", "e", "f"},
		}

		randomWords := GetRandomWords(3, lobby)
		for _, customWord := range lobby.CustomWords {
			if !slices.Contains(randomWords, customWord) {
				t.Errorf("Random words %s, didn't contain customWord %s", randomWords, customWord)
			}
		}
	})
}

func Test_getRandomWordsReloading(t *testing.T) {
	t.Parallel()

	loadWordList := func() []string { return []string{"a", "b", "c"} }
	reloadWordList := func(_ *Lobby) ([]string, error) { return loadWordList(), nil }
	wordList := loadWordList()

	t.Run("test reload with 3 words and 0 custom words and 0 chance", func(t *testing.T) {
		t.Parallel()

		lobby := &Lobby{
			words: wordList,
			EditableLobbySettings: EditableLobbySettings{
				CustomWordsPerTurn: 0,
			},
			CustomWords: nil,
		}

		// Running this 10 times, expecting it to get 3 words each time, even
		// though our pool has only got a size of 3.
		for range 10 {
			words := getRandomWords(3, lobby, reloadWordList)
			if len(words) != 3 {
				t.Errorf("Test failed, incorrect wordcount: %d", len(words))
			}
		}
	})

	t.Run("test reload with 3 words and 0 custom words and 100 chance", func(t *testing.T) {
		t.Parallel()

		lobby := &Lobby{
			words: wordList,
			EditableLobbySettings: EditableLobbySettings{
				CustomWordsPerTurn: 3,
			},
			CustomWords: nil,
		}

		// Running this 10 times, expecting it to get 3 words each time, even
		// though our pool has only got a size of 3.
		for range 10 {
			words := getRandomWords(3, lobby, reloadWordList)
			if len(words) != 3 {
				t.Errorf("Test failed, incorrect wordcount: %d", len(words))
			}
		}
	})

	t.Run("test reload with 3 words and 1 custom words and 0 chance", func(t *testing.T) {
		t.Parallel()

		lobby := &Lobby{
			words: wordList,
			EditableLobbySettings: EditableLobbySettings{
				CustomWordsPerTurn: 3,
			},
			CustomWords: []string{"a"},
		}

		// Running this 10 times, expecting it to get 3 words each time, even
		// though our pool has only got a size of 3.
		for range 10 {
			words := getRandomWords(3, lobby, reloadWordList)
			if len(words) != 3 {
				t.Errorf("Test failed, incorrect wordcount: %d", len(words))
			}
		}
	})
}

var poximityBenchCases = [][]string{
	{"", ""},
	{"a", "a"},
	{"ab", "ab"},
	{"abc", "abc"},
	{"abc", "abcde"},
	{"cde", "abcde"},
	{"a", "abcdefghijklmnop"},
	{"cde", "abcde"},
	{"cheese", "wheel"},
	{"abcdefg", "bcdefgh"},
}

func Benchmark_proximity_custom(b *testing.B) {
	for _, benchCase := range poximityBenchCases {
		b.Run(fmt.Sprint(benchCase[0], " ", benchCase[1]), func(b *testing.B) {
			var sink int
			for i := 0; i < b.N; i++ {
				sink = CheckGuess(benchCase[0], benchCase[1])
			}
			_ = sink
		})
	}
}

// We've replaced levensthein with the implementation from proximity_custom
// func Benchmark_proximity_levensthein(b *testing.B) {
// 	for _, benchCase := range poximityBenchCases {
// 		b.Run(fmt.Sprint(benchCase[0], " ", benchCase[1]), func(b *testing.B) {
// 			var sink int
// 			for i := 0; i < b.N; i++ {
// 				sink = levenshtein.ComputeDistance(benchCase[0], benchCase[1])
// 			}
// 			_ = sink
// 		})
// 	}
// }

func Test_CheckGuess_Negative(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		a, b string
	}

	cases := []testCase{
		{
			a: "abc",
			b: "abcde",
		},
		{
			a: "abc",
			b: "01abc",
		},
		{
			a: "abc",
			b: "a",
		},
		{
			a: "c",
			b: "abc",
		},
		{
			a: "abc",
			b: "c",
		},
		{
			a: "hallo",
			b: "welt",
		},
		{
			a: "abcd",
			b: "badc",
		},
		{
			name: "emoji_a",
			a:    "😲",
			b:    "abc",
		},
		{
			name: "emoji_a_same_byte_count",
			a:    "abcda",
			b:    "😲",
		},
		{
			name: "emoji_a_higher_byte_count",
			a:    "abcda",
			b:    "😲",
		},
		{
			name: "emoji_b",
			a:    "abc",
			b:    "😲",
		},
		{
			a: "cheese",
			b: "wheel",
		},
		{
			a: "a",
			b: "bcdefg",
		},
	}

	for _, c := range cases {
		name := fmt.Sprintf("%s ~ %s", c.a, c.b)
		if c.name != "" {
			name = c.name
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 2, CheckGuess(c.a, c.b))
		})
	}
}

func Test_CheckGuess_Positive(t *testing.T) {
	t.Parallel()

	type testCase struct {
		a, b string
		dist int
	}

	cases := []testCase{
		{
			a:    "abc",
			b:    "abc",
			dist: EqualGuess,
		},
		{
			a:    "abc",
			b:    "abcd",
			dist: CloseGuess,
		},
		{
			a:    "abc",
			b:    "ab",
			dist: CloseGuess,
		},
		{
			a:    "abc",
			b:    "bc",
			dist: CloseGuess,
		},
		{
			a:    "abcde",
			b:    "abde",
			dist: CloseGuess,
		},
		{
			a:    "abc",
			b:    "adc",
			dist: CloseGuess,
		},
		{
			a:    "abc",
			b:    "acb",
			dist: CloseGuess,
		},
		{
			a:    "abcd",
			b:    "acbd",
			dist: CloseGuess,
		},
		{
			a:    "äbcd",
			b:    "abcd",
			dist: CloseGuess,
		},
		{
			a:    "abcd",
			b:    "bacd",
			dist: CloseGuess,
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%s ~ %s", c.a, c.b), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.dist, CheckGuess(c.a, c.b))
		})
	}
}

func Test_ComputeEditDistance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		a, b        string
		maxDistance int
		want        int
	}{
		{name: "exact", a: "painting", b: "painting", maxDistance: 0, want: 0},
		{name: "one replacement", a: "painting", b: "paintxng", maxDistance: 1, want: 1},
		{name: "adjacent transposition", a: "painting", b: "paitning", maxDistance: 1, want: 1},
		{name: "two edits allowed", a: "painting", b: "paintxxg", maxDistance: 2, want: 2},
		{name: "exceeds max distance", a: "painting", b: "pxxntxxg", maxDistance: 2, want: 3},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, testCase.want, ComputeEditDistance(testCase.a, testCase.b, testCase.maxDistance))
		})
	}
}

func Test_allowedGuessDistance(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, allowedGuessDistance("painting", 0))
	assert.Equal(t, 1, allowedGuessDistance("word", 25))
	assert.Equal(t, 2, allowedGuessDistance("painting", 25))
	assert.Equal(t, 1, allowedGuessDistance("abc", 25))
}
