package game

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/scribble-rs/scribble.rs/internal/sanitize"
	"github.com/scribble-rs/scribble.rs/wordpacks"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type WordpackData struct {
	Lowercaser func() cases.Caser
	FileName   string
	IsRtl      bool
}

var (
	ErrUnknownWordpack = errors.New("wordpack unknown")
	WordpackDataByName = map[string]WordpackData{}
	knownWordpackData  = map[string]WordpackData{
		"english_gb": {
			FileName:   "en_gb",
			Lowercaser: func() cases.Caser { return cases.Lower(language.BritishEnglish) },
		},
		"english": {
			FileName:   "en_us",
			Lowercaser: func() cases.Caser { return cases.Lower(language.AmericanEnglish) },
		},
		"italian": {
			FileName:   "it",
			Lowercaser: func() cases.Caser { return cases.Lower(language.Italian) },
		},
		"german": {
			FileName:   "de",
			Lowercaser: func() cases.Caser { return cases.Lower(language.German) },
		},
		"french": {
			FileName:   "fr",
			Lowercaser: func() cases.Caser { return cases.Lower(language.French) },
		},
		"dutch": {
			FileName:   "nl",
			Lowercaser: func() cases.Caser { return cases.Lower(language.Dutch) },
		},
		"ukrainian": {
			FileName:   "ua",
			Lowercaser: func() cases.Caser { return cases.Lower(language.Ukrainian) },
		},
		"russian": {
			FileName:   "ru",
			Lowercaser: func() cases.Caser { return cases.Lower(language.Russian) },
		},
		"polish": {
			FileName:   "pl",
			Lowercaser: func() cases.Caser { return cases.Lower(language.Polish) },
		},
		"arabic": {
			IsRtl:      true,
			FileName:   "ar",
			Lowercaser: func() cases.Caser { return cases.Lower(language.Arabic) },
		},
		"hebrew": {
			IsRtl:      true,
			FileName:   "he",
			Lowercaser: func() cases.Caser { return cases.Lower(language.Hebrew) },
		},
		"persian": {
			IsRtl:      true,
			FileName:   "fa",
			Lowercaser: func() cases.Caser { return cases.Lower(language.Persian) },
		},
		"Persian_1": {
			IsRtl:      true,
			FileName:   "Persian_1",
			Lowercaser: func() cases.Caser { return cases.Lower(language.Persian) },
		},
		"HP_2_med": {
			FileName:   "HP_2_med",
			Lowercaser: func() cases.Caser { return cases.Lower(language.AmericanEnglish) },
		},
	}
)

const wordpackDir = "wordpacks"

func init() {
	if err := loadWordpacks(); err != nil {
		log.Printf("Error loading wordpacks: %s\n", err)
	}
}

func loadWordpacks() error {
	loadedWordpacks := map[string]WordpackData{}

	if err := registerWordpackFiles(loadedWordpacks, wordpacks.Files, "."); err != nil {
		return fmt.Errorf("error loading embedded wordpacks: %w", err)
	}

	if err := registerWordpackFiles(loadedWordpacks, os.DirFS(wordpackDir), "."); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("error loading wordpacks from %s: %w", wordpackDir, err)
		}
	}

	WordpackDataByName = loadedWordpacks
	SupportedWordpacks = make(map[string]string, len(loadedWordpacks))
	for wordpackName := range loadedWordpacks {
		SupportedWordpacks[wordpackName] = wordpackDisplayName(wordpackName)
	}

	return nil
}

func registerWordpackFiles(wordpackDataByName map[string]WordpackData, fileSystem fs.FS, dir string) error {
	entries, err := fs.ReadDir(fileSystem, dir)
	if err != nil {
		return err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || filepath.Ext(entry.Name()) == ".go" {
			continue
		}

		fileName := entry.Name()
		content, err := fs.ReadFile(fileSystem, fileName)
		if err != nil {
			return fmt.Errorf("error reading wordpack %s: %w", fileName, err)
		}
		if !utf8.Valid(content) {
			return fmt.Errorf("wordpack %s is not valid UTF-8 text", fileName)
		}

		wordpackName, wordpackData, ok := knownWordpackForFile(fileName)
		if !ok {
			wordpackName = wordpackNameForFile(fileName)
			wordpackData = WordpackData{
				Lowercaser: func() cases.Caser { return cases.Lower(language.AmericanEnglish) },
			}
		}
		wordpackData.FileName = fileName
		wordpackDataByName[wordpackName] = wordpackData
	}

	return nil
}

func knownWordpackForFile(fileName string) (string, WordpackData, bool) {
	for wordpackName, wordpackData := range knownWordpackData {
		if wordpackData.FileName == fileName {
			return wordpackName, wordpackData, true
		}
	}
	return "", WordpackData{}, false
}

func wordpackNameForFile(fileName string) string {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".txt", ".text":
		return strings.TrimSuffix(fileName, filepath.Ext(fileName))
	default:
		return fileName
	}
}

func wordpackDisplayName(wordpackName string) string {
	switch wordpackName {
	case "english_gb":
		return "English (GB)"
	case "english":
		return "English (US)"
	case "italian":
		return "Italian"
	case "german":
		return "German"
	case "french":
		return "French"
	case "dutch":
		return "Dutch"
	case "ukrainian":
		return "Ukrainian"
	case "russian":
		return "Russian"
	case "polish":
		return "Polish"
	case "arabic":
		return "Arabic"
	case "hebrew":
		return "Hebrew"
	case "persian":
		return "Persian"
	default:
		return wordpackName
	}
}

func getWordpackFileName(wordpack string) string {
	if wordpackData, ok := WordpackDataByName[wordpack]; ok {
		return wordpackData.FileName
	}
	return ""
}

// readWordListInternal exists for testing purposes, it allows passing a custom
// wordListSupplier, in order to avoid having to write tests aggainst the
// default wordpack lists.
func readWordListInternal(
	lowercaser cases.Caser, chosenWordpack string,
	wordlistSupplier func(string) (string, error),
) ([]string, error) {
	wordpackFileName := getWordpackFileName(chosenWordpack)
	if wordpackFileName == "" {
		return nil, ErrUnknownWordpack
	}

	wordListFile, err := wordlistSupplier(wordpackFileName)
	if err != nil {
		return nil, fmt.Errorf("error invoking wordlistSupplier: %w", err)
	}

	// Wordpacks are newline-separated text files. Empty lines are ignored so a
	// conventional trailing newline does not become an empty word.
	words := []string{}
	for _, word := range strings.Split(sanitize.StripModifierCharacters(lowercaser.String(wordListFile)), "\n") {
		word = strings.TrimSpace(word)
		if word != "" {
			words = append(words, word)
		}
	}
	shuffleWordList(words)
	return words, nil
}

// readDefaultWordList reads the wordlist for the given wordpack from the filesystem.
// If found, the list is cached and will be read from the cache upon next
// request. The returned slice is a safe copy and can be mutated. If the
// specified has no corresponding wordlist, an error is returned. This has been
// a panic before, however, this could enable a user to forcefully crash the
// whole application.
func readDefaultWordList(lowercaser cases.Caser, chosenWordpack string) ([]string, error) {
	log.Printf("Loading wordpack '%s'\n", chosenWordpack)
	defer log.Printf("Wordpack loaded '%s'\n", chosenWordpack)
	return readWordListInternal(lowercaser, chosenWordpack, func(key string) (string, error) {
		if wordBytes, err := os.ReadFile(filepath.Join(wordpackDir, key)); err == nil {
			return strings.ReplaceAll(string(wordBytes), "\r", ""), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("error reading wordfile: %w", err)
		}

		wordBytes, err := wordpacks.Files.ReadFile(key)
		if err != nil {
			return "", fmt.Errorf("error reading embedded wordfile: %w", err)
		}
		return strings.ReplaceAll(string(wordBytes), "\r", ""), nil
	})
}

func reloadLobbyWords(lobby *Lobby) ([]string, error) {
	return readDefaultWordList(lobby.lowercaser, lobby.Wordpack)
}

// GetRandomWords gets a custom amount of random words for the passed Lobby.
// The words will be chosen from the custom words and the default
// dictionary, depending on the settings specified by the lobbies creator.
func GetRandomWords(wordCount int, lobby *Lobby) []string {
	return getRandomWords(wordCount, lobby, reloadLobbyWords)
}

// getRandomWords exists for test purposes, allowing to define a custom
// reloader, allowing us to specify custom wordlists in the tests without
// running into a panic on reload.
func getRandomWords(wordCount int, lobby *Lobby, reloadWords func(lobby *Lobby) ([]string, error)) []string {
	words := make([]string, wordCount)

	for customWordsLeft, i := lobby.CustomWordsPerTurn, 0; i < wordCount; i++ {
		if customWordsLeft > 0 && len(lobby.CustomWords) > 0 {
			customWordsLeft--
			words[i] = popCustomWord(lobby)
		} else {
			words[i] = popWordpackWord(lobby, reloadWords)
		}
	}

	return words
}

func popCustomWord(lobby *Lobby) string {
	lastIndex := len(lobby.CustomWords) - 1
	lastWord := lobby.CustomWords[lastIndex]
	lobby.CustomWords = lobby.CustomWords[:lastIndex]
	return lastWord
}

// popWordpackWord gets X words from the wordpack. The major difference to
// popCustomWords is, that the wordlist gets reset and reshuffeled once every
// item has been popped.
func popWordpackWord(lobby *Lobby, reloadWords func(lobby *Lobby) ([]string, error)) string {
	if len(lobby.words) == 0 {
		var err error
		lobby.words, err = reloadWords(lobby)
		if err != nil {
			// Since this list should've been successfully read once before, we
			// can "safely" panic if this happens, assuming that there's a
			// deeper problem.
			panic(err)
		}
	}
	lastIndex := len(lobby.words) - 1
	lastWord := lobby.words[lastIndex]
	lobby.words = lobby.words[:lastIndex]
	return lastWord
}

func shuffleWordList(wordlist []string) {
	rand.Shuffle(len(wordlist), func(a, b int) {
		wordlist[a], wordlist[b] = wordlist[b], wordlist[a]
	})
}

const (
	EqualGuess   = 0
	CloseGuess   = 1
	DistantGuess = 2
)

// CheckGuess compares the strings with eachother. Possible results:
//   - EqualGuess (0)
//   - CloseGuess (1)
//   - DistantGuess (2)
//
// This works mostly like levensthein distance, but doesn't check further than
// to a distance of 2 and also handles transpositions where the runes are
// directly next to eachother.
func CheckGuess(a, b string) int {
	distance := ComputeEditDistance(a, b, 1)
	if distance == 0 {
		return EqualGuess
	}
	if distance == 1 {
		return CloseGuess
	}

	return DistantGuess
}

func allowedGuessDistance(targetWord string, allowedEditDistancePercent int) int {
	if allowedEditDistancePercent <= 0 {
		return 0
	}

	targetLength := utf8.RuneCountInString(targetWord)
	return int(math.Round(float64(targetLength) * float64(allowedEditDistancePercent) / 100.0))
}

// ComputeEditDistance returns the optimal string alignment distance between a
// and b and treats adjacent transpositions as a single edit. If the edit
// distance exceeds maxDistance, maxDistance+1 is returned early.
func ComputeEditDistance(a, b string, maxDistance int) int {
	if maxDistance < 0 {
		maxDistance = 0
	}

	aRunes := []rune(a)
	bRunes := []rune(b)

	if abs(len(aRunes)-len(bRunes)) > maxDistance {
		return maxDistance + 1
	}

	if len(aRunes) == 0 {
		if len(bRunes) > maxDistance {
			return maxDistance + 1
		}
		return len(bRunes)
	}
	if len(bRunes) == 0 {
		if len(aRunes) > maxDistance {
			return maxDistance + 1
		}
		return len(aRunes)
	}

	prevPrev := make([]int, len(bRunes)+1)
	prev := make([]int, len(bRunes)+1)
	curr := make([]int, len(bRunes)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(aRunes); i++ {
		curr[0] = i
		rowMin := curr[0]

		for j := 1; j <= len(bRunes); j++ {
			cost := 1
			if aRunes[i-1] == bRunes[j-1] {
				cost = 0
			}

			deletion := prev[j] + 1
			insertion := curr[j-1] + 1
			substitution := prev[j-1] + cost

			value := min(deletion, min(insertion, substitution))

			if i > 1 &&
				j > 1 &&
				aRunes[i-1] == bRunes[j-2] &&
				aRunes[i-2] == bRunes[j-1] {
				value = min(value, prevPrev[j-2]+1)
			}

			curr[j] = value
			rowMin = min(rowMin, value)
		}

		if rowMin > maxDistance {
			return maxDistance + 1
		}

		prevPrev, prev, curr = prev, curr, prevPrev
	}

	if prev[len(bRunes)] > maxDistance {
		return maxDistance + 1
	}

	return prev[len(bRunes)]
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
