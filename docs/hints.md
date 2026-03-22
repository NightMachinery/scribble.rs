# Hint system

This document explains how Scribble.rs generates, reveals, and renders word hints during a round.

## Overview

The hint system is implemented mostly in:

- `internal/game/lobby.go`
- `internal/game/shared.go`
- `internal/frontend/lobby.js`
- `internal/sanitize/sanitize.go`

At a high level:

1. The server picks a word.
2. It builds two parallel hint arrays.
3. Guessers/spectators get the masked version.
4. The drawer and players who already guessed get the fully visible version.
5. During the round, the server periodically reveals random hidden letters.
6. Earlier guesses are worth more because fewer hints have been spent.

## The `WordHint` model

Each displayed character is represented by `game.WordHint` in `internal/game/shared.go`:

- `Character rune`
  - `0` means the character is still hidden.
  - non-zero means the client should display that rune.
- `Underline bool`
  - marks guessable characters with an underline.
  - separators such as spaces, `_`, and `-` are not underlined.
- `Revealed bool`
  - marks letters that have been publicly revealed as hints.

## Two server-side hint arrays

When a word is chosen, the server builds two arrays in `selectWord` (`internal/game/lobby.go`):

- `lobby.wordHints`
  - what guessers and spectators see
  - hidden letters use `Character == 0`
- `lobby.wordHintsShown`
  - what the drawer and already-correct players see
  - all characters are always present

This is why the drawer can see the whole word immediately, while guessers cannot.

## Which characters are always visible

The following characters are treated as always-visible separators:

- space: `' '`
- underscore: `'_'`
- dash: `'-'`

They are inserted into both hint arrays immediately and are not underlined.

Everything else starts hidden for guessers and spectators.

Example:

- word: `Pac-Man`
- initial guesser view: `_ _ _ - _ _ _`
- initial drawer view: `P a c - M a n`

## How many hints a word gets

The number of revealable hints is fixed when the word is chosen.

Code: `selectWord` in `internal/game/lobby.go`

Thresholds are based on `utf8.RuneCountInString(currentWord)`:

- `<= 2` runes: `0` hints
- `<= 4` runes: `1` hint
- `<= 9` runes: `2` hints
- `>= 10` runes: `3` hints

This is hard-coded; there is no lobby setting for it.

## When hints are sent

Before the drawer chooses a word, there are no active hints for the round yet.
In that state, `ready.wordHints` may be absent and the client shows the
"waiting for word choice" UI instead.

### Initial word selection

When the drawer chooses a word, the server sends `word-chosen`:

- guessers/spectators receive `lobby.wordHints`
- drawer/standby players receive `lobby.wordHintsShown`

The payload is `WordChosen{Hints, TimeLeft}`.

### Reconnect / initial sync

When a client joins or reconnects, the `ready` event includes `ready.wordHints` from `GetAvailableWordHints`:

- `Guessing` and `Spectating` -> masked hints
- everyone else (`Drawing`, `Standby`) -> fully visible hints

### After a correct guess

When a player guesses correctly, the server immediately sends that player `update-wordhint` with `lobby.wordHintsShown`, so they can see the full word.

## How reveal timing works

Hint reveals happen in `tickLogic` (`internal/game/lobby.go`).

If there are hints left, the server computes:

```go
revealHintEveryXMilliseconds := DrawingTime * 1000 / (hintCount + 1)
```

Then it reveals a new hint whenever:

```go
timeLeft <= revealHintEveryXMilliseconds * hintsLeft
```

This spreads hints roughly evenly across the round.

Example for a 120 second round with 3 hints:

- first reveal at about 90s remaining
- second reveal at about 60s remaining
- third reveal at about 30s remaining

If the round starts late relative to this schedule, a hint may be revealed immediately on the next tick.

## How a letter is chosen

When it is time to reveal a hint, the server:

1. decrements `hintsLeft`
2. picks random indices until it finds a still-hidden slot in `lobby.wordHints`
3. copies the real rune from `CurrentWord` into that slot
4. marks that slot as `Revealed` in both arrays
5. broadcasts `update-wordhint`

Important detail:

- only slots with `Character == 0` are eligible
- always-visible separators are never randomly revealed because they were never hidden

## Who receives reveal updates

The server broadcasts two versions of `update-wordhint`:

- `IsAllowedToSeeHints` (`Guessing` or `Spectating`)
  - receives `lobby.wordHints`
  - only revealed letters become visible
- `IsAllowedToSeeRevealedHints` (`Standby` or `Drawing`)
  - receives `lobby.wordHintsShown`
  - the full word stays visible, but `Revealed` marks which letters have now been made public

## Client rendering

Client code: `internal/frontend/lobby.js`

`applyWordHints` renders one `<span>` per `WordHint`:

- `Character == 0` -> blank underlined placeholder
- visible character + `Underline == true` -> underlined visible character
- visible character + `Underline == false` -> separator such as space/dash/underscore

The client also appends a compact word-length indicator like:

```text
(3)
(3, 5)
```

This is split on spaces only. Dashes and underscores remain part of the displayed segment length.

## Hint chat message behavior

On every `update-wordhint`, the client also emits a system chat message:

- translation key: `word-hint-revealed`
- followed by the current masked word state

For that chat preview:

- spaces are shown directly
- publicly revealed letters are shown directly
- everything else is shown as `_`

The drawer additionally gets a visual highlight (`hint-revealed`) for newly revealed letters in the top word display.

## Relationship to guess checking

Guess checking normalizes both the player's input and the target word before comparison.

Code path:

- `internal/game/lobby.go` -> `CheckGuess(...)`
- `internal/sanitize/sanitize.go` -> `CleanText(...)`

`CleanText` removes:

- spaces
- dashes
- underscores

It also transliterates some accented characters.

That means the always-visible separators are only visual aids; players do not need to type them exactly for a guess to count.

## Relationship to scoring

Hints directly affect guesser score.

Code: `CalculateGuesserScoreInternal` in `internal/game/lobby.go`

The score includes a hint bonus based on `hintsLeft`:

```go
if hintCount > 0 {
    score += hintsLeft * (maxHintBonusScore / hintCount)
}
```

So:

- guessing before any hint is revealed gives the highest hint bonus
- each revealed hint reduces the remaining bonus
- 2-letter words avoid division by zero because they have `hintCount == 0`

## Tested behavior

`internal/game/lobby_test.go` verifies the initial split between drawer and guesser hints:

- drawer receives fully visible characters
- guesser receives hidden characters (`Character == 0`)
- guessable characters are underlined

## Summary

The hint system is built around two parallel views of the same word:

- a masked version for active guessers/spectators
- a fully visible version for the drawer and already-correct players

Hints are revealed by time, chosen randomly from still-hidden letters, surfaced to the UI through `word-chosen` / `update-wordhint`, and also reduce the score available for later guesses.
