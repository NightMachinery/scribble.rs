# Scoring rules

This document explains how Scribble.rs awards points to guessers, drawers, and ranks.

## Overview

The scoring logic lives mostly in:

- `internal/game/lobby.go`
- `internal/game/data.go`
- `internal/api/createparse.go`
- `internal/game/lobby_test.go`

There are two built-in scoring modes:

- `chill`
- `competitive`

Both use the same formulas. They only differ in constants.

## What `LastScore` means

`LastScore` is a player's score for the current round, or, once the round has
ended, the score they most recently earned in the previous round.

It is used for three things:

- showing the per-turn score change in the UI
- calculating the drawer's score at round end
- rolling back a round's points when the drawer is kicked or forcibly spectated

Typical values:

- correct guesser: their earned points for that round
- drawer: the drawer score computed at round end
- player who did not guess correctly that round: `0`

## When points are awarded

### Guessers

An active guesser gets points immediately after an exact correct guess in `handleMessage`:

1. `LastScore` is set to the current guesser score.
2. That value is added to the player's total `Score`.
3. The player moves from `Guessing` to `Standby`.

Close guesses do not award points.

So for guessers, `LastScore` means "the points this player earned for this
round's guess."

### Drawer

The drawer gets points only when the round advances in `advanceLobbyPredefineDrawer`.

The drawer's `LastScore` is calculated from the other players' scores for that round, then added to the drawer's total `Score`.

So for the drawer, `LastScore` means "the drawer bonus for the round that just
ended."

## Guesser score formula

`adjustableScoringAlgorithm.CalculateGuesserScoreInternal` computes:

```go
declineFactor := bonusBaseScoreDeclineFactor / float64(drawingTime)

score := int(
    baseScore +
        maxBonusBaseScore*math.Pow(
            1.0-declineFactor,
            float64(drawingTime-secondsLeft),
        ),
)

if hintCount > 0 {
    score += hintsLeft * (int(maxHintBonusScore) / hintCount)
}
```

Where:

- `drawingTime` is the lobby's round length in seconds
- `secondsLeft` is the server-side integer number of whole seconds remaining
- `hintCount` is the number of revealable hints for the chosen word
- `hintsLeft` is how many of those hints have not been revealed yet

So a guesser's score is:

- a fixed base score
- plus a time bonus that declines as the round goes on
- plus a hint bonus that shrinks as hints are revealed

## Scoring modes

### `chill`

- `baseScore = 100`
- `maxBonusBaseScore = 100`
- `bonusBaseScoreDeclineFactor = 2`
- `maxHintBonusScore = 60`
- theoretical max score: `260`

This mode keeps the guaranteed base score high and makes the time bonus fall more gently.

### `competitive`

- `baseScore = 10`
- `maxBonusBaseScore = 290`
- `bonusBaseScoreDeclineFactor = 3`
- `maxHintBonusScore = 120`
- theoretical max score: `420`

This mode shifts much more of the reward into fast guessing and unrevealed hints.

## Time bonus behavior

The time component is not linear. It uses an exponential-style decay:

- earlier correct guesses are worth more
- the score always trends downward as time runs out
- `competitive` falls faster than `chill`

`internal/game/lobby_test.go` includes a test that verifies the guesser score declines as more time is taken.

## Hint bonus behavior

Hints directly reduce the available guesser score.

The bonus added is:

```go
hintsLeft * (maxHintBonusScore / hintCount)
```

Important details:

- guessing before any hint is revealed gives the largest hint bonus
- each revealed hint removes one chunk of that bonus
- words with `hintCount == 0` get no hint bonus and avoid division by zero

Hint counts are described in `docs/hints.md`.

## Drawer score formula

The drawer score is the integer average of eligible non-drawer players' `LastScore` values:

```go
drawerScore = scoreSum / playerCount
```

Eligibility rules from `CalculateDrawerScore`:

- exclude the current drawer
- exclude spectators
- include connected players, even if their `LastScore` is `0`
- include disconnected players only if they already earned points that round (`LastScore > 0`)

This means:

- if nobody guessed correctly, the drawer gets `0`
- if some players guessed correctly and then disconnected, their round score can still count toward the drawer average
- the result is truncated to an integer because it uses integer division

More concretely, the drawer gets more points when the average `LastScore` of
eligible non-drawer players is higher.

That means the drawer is rewarded indirectly for:

- players guessing earlier rather than later
- players guessing before more hints are revealed
- more connected players guessing correctly instead of staying at `0`

### Does a player who never guesses lower the drawer score?

Usually yes.

If a non-drawer player stays connected for the round and never guesses
correctly, they still count in the drawer-score average with:

- `LastScore = 0`

That lowers the average unless the drawer score was already `0`.

Example:

- two connected guessers
- one scores `200`
- one never guesses and stays at `0`
- drawer score = `(200 + 0) / 2 = 100`

If both had guessed for `200`, the drawer score would have been `200`.

### Does "more players guessed correctly" always increase drawer score?

For connected non-drawer players, yes in the normal case: connected players are
already part of the average even before they guess, so changing one player's
`LastScore` from `0` to a positive value increases the drawer score.

For disconnected players, the rule is slightly different: a disconnected player
with `LastScore == 0` is ignored entirely, while a disconnected player with
`LastScore > 0` is included. So a disconnected player's correct guess can raise
or lower the final average depending on how their score compares to the current
average.

## When points are removed or suppressed

### Drawer kicked

If the drawer is kicked mid-round, the server removes every player's current-round score:

- `Score -= LastScore`
- `LastScore = 0`

Then the lobby advances. Net effect: nobody keeps points from that round.

### Drawer forced to spectate

If the drawer is forcibly moved to spectator mode during the round, the same rollback happens: nobody keeps points from that round.

### Round ends normally

Players still in `Guessing` at round end get `LastScore = 0` for that round.

Already-correct guessers keep the points they earned earlier in the round.

## Ranking rules

Ranks are recalculated from total `Score`:

- higher total score ranks above lower total score
- tied scores share the same rank
- disconnected non-spectators are skipped while disconnected

The player list order is not sorted by rank because that order is also used for draw rotation.

## Parsing / API values

`ParseScoreCalculation` accepts:

- empty string -> `chill`
- `chill`
- `competitive`

Any other value is rejected.

## Summary

Scribble.rs scoring is built around three ideas:

- guess fast
- guess before hints are revealed
- reward the drawer based on how well the round's guessers performed

`chill` softens the importance of speed with a large base score, while `competitive` makes fast, early guesses much more valuable.
