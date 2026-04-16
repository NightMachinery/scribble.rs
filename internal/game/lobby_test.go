package game

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/gofrs/uuid/v5"
	"github.com/lxzan/gws"
	"github.com/scribble-rs/scribble.rs/internal/sanitize"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func createLobbyWithDemoPlayers(playercount int) *Lobby {
	owner := &Player{}
	lobby := &Lobby{
		OwnerID: owner.ID,
	}
	for range playercount {
		lobby.players = append(lobby.players, &Player{
			Connected: true,
		})
	}

	return lobby
}

func noOpWriteObject(_ *Player, _ any) error {
	return nil
}

func noOpWritePreparedMessage(_ *Player, _ *gws.Broadcaster) error {
	return nil
}

func Test_Locking(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{}
	lobby.mutex.Lock()
	if lobby.mutex.TryLock() {
		t.Error("Mutex shouldn't be acquiredable at this point")
	}
}

func Test_CalculateVotesNeededToKick(t *testing.T) {
	t.Parallel()

	expectedResults := map[int]int{
		// Kinda irrelevant since you can't kick yourself, but who cares.
		1:  2,
		2:  2,
		3:  2,
		4:  2,
		5:  3,
		6:  3,
		7:  4,
		8:  4,
		9:  5,
		10: 5,
	}

	for playerCount, expctedRequiredVotes := range expectedResults {
		lobby := createLobbyWithDemoPlayers(playerCount)
		result := calculateVotesNeededToKick(lobby)
		if result != expctedRequiredVotes {
			t.Errorf("Error. Necessary vote amount was %d, but should've been %d", result, expctedRequiredVotes)
		}
	}
}

func Test_RemoveAccents(t *testing.T) {
	t.Parallel()

	expectedResults := map[string]string{
		"é":     "e",
		"É":     "E",
		"à":     "a",
		"À":     "A",
		"ç":     "c",
		"Ç":     "C",
		"ö":     "oe",
		"Ö":     "OE",
		"œ":     "oe",
		"\n":    "\n",
		"\t":    "\t",
		"\r":    "\r",
		"":      "",
		"·":     "·",
		"?:!":   "?:!",
		"ac-ab": "acab",
		//nolint:gocritic
		"ac - _ab-- ": "acab",
	}

	for k, v := range expectedResults {
		result := sanitize.CleanText(k)
		if result != v {
			t.Errorf("Error. Char was %s, but should've been %s", result, v)
		}
	}
}

func Test_simplifyText(t *testing.T) {
	t.Parallel()

	// We only test the replacement we do ourselves. We won't test
	// the "sanitize", or furthermore our expectations of it for now.

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "dash",
			input: "-",
			want:  "",
		},
		{
			name:  "underscore",
			input: "_",
			want:  "",
		},
		{
			name:  "space",
			input: " ",
			want:  "",
		},
		{
			name:  "persian halfspace",
			input: "\u200c",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := sanitize.CleanText(tt.input); got != tt.want {
				t.Errorf("simplifyText() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_recalculateRanks(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{}
	lobby.players = append(lobby.players, &Player{
		ID:        uuid.Must(uuid.NewV4()),
		Score:     1,
		Connected: true,
	})
	lobby.players = append(lobby.players, &Player{
		ID:        uuid.Must(uuid.NewV4()),
		Score:     1,
		Connected: true,
	})
	recalculateRanks(lobby)

	rankPlayerA := lobby.players[0].Rank
	rankPlayerB := lobby.players[1].Rank
	if rankPlayerA != 1 || rankPlayerB != 1 {
		t.Errorf("With equal score, ranks should be equal. (A: %d; B: %d)",
			rankPlayerA, rankPlayerB)
	}

	lobby.players = append(lobby.players, &Player{
		ID:        uuid.Must(uuid.NewV4()),
		Score:     0,
		Connected: true,
	})
	recalculateRanks(lobby)

	rankPlayerA = lobby.players[0].Rank
	rankPlayerB = lobby.players[1].Rank
	if rankPlayerA != 1 || rankPlayerB != 1 {
		t.Errorf("With equal score, ranks should be equal. (A: %d; B: %d)",
			rankPlayerA, rankPlayerB)
	}

	rankPlayerC := lobby.players[2].Rank
	if rankPlayerC != 2 {
		t.Errorf("new player should be rank 2, since the previous two players had the same rank. (C: %d)", rankPlayerC)
	}
}

func Test_recalculateRanksIncludesDisconnectedNonSpectators(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{}
	lobby.players = append(lobby.players, &Player{
		ID:        uuid.Must(uuid.NewV4()),
		Score:     10,
		Connected: true,
		State:     Guessing,
	})
	lobby.players = append(lobby.players, &Player{
		ID:        uuid.Must(uuid.NewV4()),
		Score:     5,
		Connected: false,
		State:     Standby,
	})

	recalculateRanks(lobby)

	require.Equal(t, 1, lobby.players[0].Rank)
	require.Equal(t, 2, lobby.players[1].Rank)
}

func Test_readyToStartIgnoresSpectators(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{}
	lobby.players = []*Player{
		{Connected: true, State: Ready},
		{Connected: true, State: Spectating},
	}

	require.True(t, lobby.readyToStart())
}

func Test_OnPlayerDisconnectPreservesSpectatorOutsideOngoing(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		State: Unstarted,
	}
	lobby.WriteObject = noOpWriteObject
	lobby.WritePreparedMessage = noOpWritePreparedMessage

	spectator := lobby.JoinPlayer("spectator")
	spectator.Connected = true
	spectator.State = Spectating
	spectator.SetWebsocket(&gws.Conn{})

	lobby.OnPlayerDisconnect(spectator)

	require.Equal(t, Spectating, spectator.State)
	require.False(t, spectator.Connected)
}

func Test_GetAvailableWordHintsMasksSpectators(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		wordHints: []*WordHint{
			{Character: 0, Underline: true},
		},
		wordHintsShown: []*WordHint{
			{Character: 'a', Underline: true},
		},
	}

	require.Equal(t, lobby.wordHints, lobby.GetAvailableWordHints(&Player{State: Guessing}))
	require.Equal(t, lobby.wordHints, lobby.GetAvailableWordHints(&Player{State: Spectating}))
	require.Equal(t, lobby.wordHintsShown, lobby.GetAvailableWordHints(&Player{State: Drawing}))
	require.Equal(t, lobby.wordHintsShown, lobby.GetAvailableWordHints(&Player{State: Standby}))
}

func Test_chillScoring_calculateGuesserScore(t *testing.T) {
	t.Parallel()

	score := ChillScoring.CalculateGuesserScoreInternal(0, 0, 120, time.Now().Add(115*time.Second).UnixMilli())
	if score >= ChillScoring.MaxScore() {
		t.Errorf("Score should have declined, but was bigger than or "+
			"equal to the max score. (LastScore: %d; MaxScore: %d)", score, ChillScoring.MaxScore())
	}

	lastDecline := -1
	for secondsLeft := 105; secondsLeft >= 5; secondsLeft -= 10 {
		roundEndTime := time.Now().Add(time.Duration(secondsLeft) * time.Second).UnixMilli()
		newScore := ChillScoring.CalculateGuesserScoreInternal(0, 0, 120, roundEndTime)
		if newScore > score {
			t.Errorf("Score with more time taken should be lower. (LastScore: %d; NewScore: %d)", score, newScore)
		}
		newDecline := score - newScore
		if lastDecline != -1 && newDecline > lastDecline {
			t.Errorf("Decline should get lower with time taken. (LastDecline: %d; NewDecline: %d)\n", lastDecline, newDecline)
		}
		score = newScore
		lastDecline = newDecline
	}
}

func Test_handleNameChangeEvent(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{}
	lobby.WriteObject = noOpWriteObject
	lobby.WritePreparedMessage = noOpWritePreparedMessage
	player := lobby.JoinPlayer("Kevin")

	handleNameChangeEvent(player, lobby, "Jim")

	expectedName := "Jim"
	if player.Name != expectedName {
		t.Errorf("playername didn't change; Expected %s, but was %s", expectedName, player.Name)
	}
}

func Test_handleMessageAcceptsConfiguredEditDistance(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		EditableLobbySettings: EditableLobbySettings{
			DrawingTime:                60,
			AllowedEditDistancePercent: 25,
		},
		CurrentWord:      "abcd",
		lowercaser:       cases.Lower(language.English),
		ScoreCalculation: ChillScoring,
		roundEndTime:     time.Now().Add(30 * time.Second).UnixMilli(),
	}
	lobby.WriteObject = noOpWriteObject
	lobby.WritePreparedMessage = noOpWritePreparedMessage

	sender := lobby.JoinPlayer("sender")
	sender.Connected = true
	sender.State = Guessing

	other := lobby.JoinPlayer("other")
	other.Connected = true
	other.State = Guessing

	handleMessage("abce", sender, lobby)

	require.Equal(t, Standby, sender.State)
	require.Positive(t, sender.Score)
}

func Test_handleMessageExactOnlyRejectsNearGuess(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		EditableLobbySettings: EditableLobbySettings{
			DrawingTime:                60,
			AllowedEditDistancePercent: 0,
		},
		CurrentWord:      "abcd",
		lowercaser:       cases.Lower(language.English),
		ScoreCalculation: ChillScoring,
		roundEndTime:     time.Now().Add(30 * time.Second).UnixMilli(),
	}
	lobby.WriteObject = noOpWriteObject
	lobby.WritePreparedMessage = noOpWritePreparedMessage

	sender := lobby.JoinPlayer("sender")
	sender.Connected = true
	sender.State = Guessing

	other := lobby.JoinPlayer("other")
	other.Connected = true
	other.State = Guessing

	handleMessage("abce", sender, lobby)

	require.Equal(t, Guessing, sender.State)
	require.Zero(t, sender.Score)
}

func getUnexportedField(field reflect.Value) any {
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
}

func Test_wordSelectionEvent(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		EditableLobbySettings: EditableLobbySettings{
			DrawingTime:  10,
			Rounds:       10,
			WordsPerTurn: 3,
		},
		words: []string{"abc", "def", "ghi"},
	}
	wordHintEvents := make(map[uuid.UUID][]*WordHint)
	var wordChoice []string
	lobby.WriteObject = func(_ *Player, message any) error {
		event, ok := message.(*Event)
		if ok {
			if event.Type == EventTypeYourTurn {
				yourTurn := event.Data.(*YourTurn)
				wordChoice = yourTurn.Words
			}
		}

		return nil
	}
	lobby.WritePreparedMessage = func(player *Player, message *gws.Broadcaster) error {
		data := getUnexportedField(reflect.ValueOf(message).Elem().FieldByName("payload")).([]byte)
		type event struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		var e event
		if err := json.Unmarshal(data, &e); err != nil {
			t.Fatal("error unmarshalling message", err)
		}

		t.Log(e.Type)
		if e.Type == "word-chosen" {
			var event WordChosen
			if err := json.Unmarshal(e.Data, &event); err != nil {
				t.Fatal("error unmarshalling word hints:", err)
			}
			wordHintEvents[player.ID] = event.Hints
		}
		return nil
	}

	drawer := lobby.JoinPlayer("Drawer")
	drawer.Connected = true
	lobby.OwnerID = drawer.ID

	if err := lobby.HandleEvent(EventTypeStart, nil, drawer); err != nil {
		t.Errorf("Couldn't start lobby: %s", err)
	}

	guesser := lobby.JoinPlayer("Guesser")
	guesser.Connected = true

	err := lobby.HandleEvent(EventTypeChooseWord, []byte(`{"data": 0}`), drawer)
	if err != nil {
		t.Errorf("Couldn't choose word: %s", err)
	}

	wordHintsForDrawer := wordHintEvents[drawer.ID]
	if len(wordHintsForDrawer) != 3 {
		t.Errorf("Word hints for drawer were of incorrect length; %d != %d", len(wordHintsForDrawer), 3)
	}

	for index, wordHint := range wordHintsForDrawer {
		if wordHint.Character == 0 {
			t.Error("Word hints for drawer contained invisible character")
		}

		if !wordHint.Underline {
			t.Error("Word hints for drawer contained not underlined character")
		}

		expectedRune := rune(wordChoice[0][index])
		if wordHint.Character != expectedRune {
			t.Errorf("Character at index %d was %c instead of %c", index, wordHint.Character, expectedRune)
		}
	}

	wordHintsForGuesser := wordHintEvents[guesser.ID]
	if len(wordHintsForGuesser) != 3 {
		t.Errorf("Word hints for guesser were of incorrect length; %d != %d", len(wordHintsForGuesser), 3)
	}

	for _, wordHint := range wordHintsForGuesser {
		if wordHint.Character != 0 {
			t.Error("Word hints for guesser contained visible character")
		}

		if !wordHint.Underline {
			t.Error("Word hints for guesser contained not underlined character")
		}
	}
}

func Test_kickDrawer(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		EditableLobbySettings: EditableLobbySettings{
			DrawingTime:  10,
			Rounds:       10,
			WordsPerTurn: 3,
		},
		ScoreCalculation: ChillScoring,
		words:            []string{"a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a"},
	}
	lobby.WriteObject = noOpWriteObject
	lobby.WritePreparedMessage = noOpWritePreparedMessage

	marcel := lobby.JoinPlayer("marcel")
	marcel.Connected = true
	lobby.OwnerID = marcel.ID

	kevin := lobby.JoinPlayer("kevin")
	kevin.Connected = true
	chantal := lobby.JoinPlayer("chantal")
	chantal.Connected = true

	if err := lobby.HandleEvent(EventTypeStart, nil, marcel); err != nil {
		t.Errorf("Couldn't start lobby: %s", err)
	}

	if lobby.Drawer() == nil {
		t.Error("Drawer should've been a, but was nil")
	}

	if lobby.Drawer() != marcel {
		t.Errorf("Drawer should've been a, but was %s", lobby.Drawer().Name)
	}

	lobby.Synchronized(func() {
		advanceLobby(lobby)
	})

	if lobby.Drawer() == nil {
		t.Error("Drawer should've been b, but was nil")
	}

	if lobby.Drawer() != kevin {
		t.Errorf("Drawer should've been b, but was %s", lobby.Drawer().Name)
	}

	lobby.Synchronized(func() {
		kickPlayer(lobby, kevin, 1)
	})

	if lobby.Drawer() == nil {
		t.Error("Drawer should've been c, but was nil")
	}

	if lobby.Drawer() != chantal {
		t.Errorf("Drawer should've been c, but was %s", lobby.Drawer().Name)
	}
}

func Test_lobby_calculateDrawerScore(t *testing.T) {
	t.Parallel()

	t.Run("only disconnected players, with score", func(t *testing.T) {
		t.Parallel()
		drawer := &Player{State: Drawing}
		lobby := Lobby{
			players: []*Player{
				drawer,
				{
					Connected: false,
					LastScore: 100,
				},
				{
					Connected: false,
					LastScore: 200,
				},
			},
			ScoreCalculation: ChillScoring,
		}

		require.Equal(t, 150, lobby.calculateDrawerScore())
	})
	t.Run("only disconnected players, with no score", func(t *testing.T) {
		t.Parallel()
		drawer := &Player{State: Drawing}
		lobby := Lobby{
			players: []*Player{
				drawer,
				{
					Connected: false,
					LastScore: 0,
				},
				{
					Connected: false,
					LastScore: 0,
				},
			},
			ScoreCalculation: ChillScoring,
		}

		require.Equal(t, 0, lobby.calculateDrawerScore())
	})
	t.Run("connected players, but no score", func(t *testing.T) {
		t.Parallel()
		drawer := &Player{State: Drawing}
		lobby := Lobby{
			players: []*Player{
				drawer,
				{
					Connected: true,
					LastScore: 0,
				},
				{
					Connected: true,
					LastScore: 0,
				},
			},
			ScoreCalculation: ChillScoring,
		}

		require.Equal(t, 0, lobby.calculateDrawerScore())
	})
	t.Run("connected players", func(t *testing.T) {
		t.Parallel()
		drawer := &Player{State: Drawing}
		lobby := Lobby{
			players: []*Player{
				drawer,
				{
					Connected: true,
					LastScore: 100,
				},
				{
					Connected: true,
					LastScore: 200,
				},
			},
			ScoreCalculation: ChillScoring,
		}

		require.Equal(t, 150, lobby.calculateDrawerScore())
	})
	t.Run("some connected players, some disconnected, some without score", func(t *testing.T) {
		t.Parallel()
		drawer := &Player{State: Drawing}
		lobby := Lobby{
			players: []*Player{
				drawer,
				{
					Connected: true,
					LastScore: 100,
				},
				{
					Connected: false,
					LastScore: 200,
				},
				{
					Connected: true,
					LastScore: 0,
				},
				{
					Connected: false,
					LastScore: 0,
				},
			},
			ScoreCalculation: ChillScoring,
		}

		require.Equal(t, 100, lobby.calculateDrawerScore())
	})
	t.Run("some connected players, some disconnected", func(t *testing.T) {
		t.Parallel()
		drawer := &Player{State: Drawing}
		lobby := Lobby{
			players: []*Player{
				drawer,
				{
					Connected: true,
					LastScore: 100,
				},
				{
					Connected: true,
					LastScore: 200,
				},
				{
					Connected: false,
					LastScore: 300,
				},
				{
					Connected: false,
					LastScore: 400,
				},
			},
			ScoreCalculation: ChillScoring,
		}

		require.Equal(t, 250, lobby.calculateDrawerScore())
	})
}

func Test_NoPrematureGameOver(t *testing.T) {
	t.Parallel()

	player, lobby, err := CreateLobby("", "test", "english", &EditableLobbySettings{
		Public:             false,
		DrawingTime:        120,
		Rounds:             4,
		MaxPlayers:         4,
		CustomWordsPerTurn: 3,
		ClientsPerIPLimit:  1,
		WordsPerTurn:       3,
	}, nil, ChillScoring)
	require.NoError(t, err)

	lobby.WriteObject = noOpWriteObject
	lobby.WritePreparedMessage = noOpWritePreparedMessage

	require.Equal(t, Unstarted, lobby.State)
	require.Equal(t, Standby, player.State)

	// The socket won't be called anyway, so its fine.
	player.ws = &gws.Conn{}
	player.Connected = true

	lobby.OnPlayerDisconnect(player)
	require.False(t, player.Connected)
	require.Equal(t, Standby, player.State)
	require.Equal(t, Unstarted, lobby.State)

	lobby.OnPlayerConnectUnsynchronized(player)
	require.True(t, player.Connected)
	require.Equal(t, Standby, player.State)
	require.Equal(t, Unstarted, lobby.State)
}

func Test_LobbyPasswordValidation(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{}
	require.True(t, lobby.ValidateJoinPassword(""))

	lobby.SetJoinPassword("secret")
	require.True(t, lobby.HasPassword)
	require.True(t, lobby.ValidateJoinPassword("secret"))
	require.False(t, lobby.ValidateJoinPassword("wrong"))

	lobby.Public = true
	require.True(t, lobby.ValidateJoinPassword(""))

	lobby.Public = false
	lobby.ClearJoinPassword()
	require.False(t, lobby.HasPassword)
	require.True(t, lobby.ValidateJoinPassword(""))
}

func Test_ownerForceSpectateDrawer(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		EditableLobbySettings: EditableLobbySettings{
			DrawingTime:  10,
			Rounds:       10,
			WordsPerTurn: 3,
		},
		ScoreCalculation: ChillScoring,
		words:            []string{"a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a"},
	}
	lobby.WriteObject = noOpWriteObject
	lobby.WritePreparedMessage = noOpWritePreparedMessage

	owner := lobby.JoinPlayer("owner")
	owner.Connected = true
	lobby.OwnerID = owner.ID

	target := lobby.JoinPlayer("target")
	target.Connected = true

	replacement := lobby.JoinPlayer("replacement")
	replacement.Connected = true

	require.NoError(t, lobby.HandleEvent(EventTypeStart, nil, owner))

	lobby.Synchronized(func() {
		advanceLobby(lobby)
	})
	require.Equal(t, target, lobby.Drawer())

	payload, err := json.Marshal(Event{
		Type: EventTypeOwnerForceSpectate,
		Data: target.ID.String(),
	})
	require.NoError(t, err)
	require.NoError(t, lobby.HandleEvent(EventTypeOwnerForceSpectate, payload, owner))

	require.Equal(t, Spectating, target.State)
	require.Equal(t, replacement, lobby.Drawer())
}

func Test_wordHintsShowPersianHalfspace(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		EditableLobbySettings: EditableLobbySettings{
			DrawingTime:       10,
			Rounds:            2,
			WordsPerTurn:      1,
			AssignRandomNames: true,
		},
		ScoreCalculation: ChillScoring,
		words:            []string{"a\u200cb"},
	}
	wordHintEvents := make(map[uuid.UUID][]*WordHint)
	lobby.WriteObject = noOpWriteObject
	lobby.WritePreparedMessage = func(player *Player, message *gws.Broadcaster) error {
		data := getUnexportedField(reflect.ValueOf(message).Elem().FieldByName("payload")).([]byte)
		type outgoingEvent struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		var event outgoingEvent
		require.NoError(t, json.Unmarshal(data, &event))
		if event.Type == EventTypeWordChosen {
			var chosen WordChosen
			require.NoError(t, json.Unmarshal(event.Data, &chosen))
			wordHintEvents[player.ID] = chosen.Hints
		}
		return nil
	}

	drawer := lobby.JoinPlayer("Drawer")
	drawer.Connected = true
	lobby.OwnerID = drawer.ID

	guesser := lobby.JoinPlayer("Guesser")
	guesser.Connected = true

	require.NoError(t, lobby.HandleEvent(EventTypeStart, nil, drawer))
	require.NoError(t, lobby.HandleEvent(EventTypeChooseWord, []byte(`{"data":0}`), drawer))

	hints := wordHintEvents[guesser.ID]
	require.Len(t, hints, 3)
	require.Equal(t, rune('\u200c'), hints[1].Character)
	require.False(t, hints[1].Underline)
}

func Test_ownerForceParticipateDuringOngoingQueuesReturn(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		EditableLobbySettings: EditableLobbySettings{
			AssignRandomNames: true,
		},
		State: Ongoing,
	}
	lobby.WriteObject = noOpWriteObject
	lobby.WritePreparedMessage = noOpWritePreparedMessage

	owner := lobby.JoinPlayer("owner")
	owner.Connected = true
	lobby.OwnerID = owner.ID

	target := lobby.JoinPlayer("target")
	target.Connected = true
	target.State = Spectating

	payload, err := json.Marshal(Event{
		Type: EventTypeOwnerForceParticipate,
		Data: target.ID.String(),
	})
	require.NoError(t, err)
	require.NoError(t, lobby.HandleEvent(EventTypeOwnerForceParticipate, payload, owner))

	require.Equal(t, Spectating, target.State)
	require.True(t, target.SpectateToggleRequested)
}

func Test_ownerForceParticipateOutsideOngoingRestoresImmediately(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		EditableLobbySettings: EditableLobbySettings{
			AssignRandomNames: true,
		},
		State: Unstarted,
	}
	lobby.WriteObject = noOpWriteObject
	lobby.WritePreparedMessage = noOpWritePreparedMessage

	owner := lobby.JoinPlayer("owner")
	owner.Connected = true
	lobby.OwnerID = owner.ID

	target := lobby.JoinPlayer("target")
	target.Connected = true
	target.State = Spectating

	payload, err := json.Marshal(Event{
		Type: EventTypeOwnerForceParticipate,
		Data: target.ID.String(),
	})
	require.NoError(t, err)
	require.NoError(t, lobby.HandleEvent(EventTypeOwnerForceParticipate, payload, owner))

	require.Equal(t, Standby, target.State)
	require.False(t, target.SpectateToggleRequested)
}

func Test_ownerForceEndGameSendsGameOver(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		EditableLobbySettings: EditableLobbySettings{
			AssignRandomNames: true,
		},
		State:            Ongoing,
		CurrentWord:      "secret",
		ScoreCalculation: ChillScoring,
	}

	var events []GameOverEvent
	lobby.WriteObject = func(_ *Player, message any) error {
		switch event := message.(type) {
		case Event:
			if event.Type == EventTypeGameOver {
				events = append(events, *event.Data.(*GameOverEvent))
			}
		case *Event:
			if event.Type == EventTypeGameOver {
				events = append(events, *event.Data.(*GameOverEvent))
			}
		}
		return nil
	}
	lobby.WritePreparedMessage = noOpWritePreparedMessage

	owner := lobby.JoinPlayer("owner")
	owner.Connected = true
	lobby.OwnerID = owner.ID
	owner.Score = 42

	other := lobby.JoinPlayer("other")
	other.Connected = true
	other.Score = 13

	require.NoError(t, lobby.HandleEvent(EventTypeOwnerForceEndGame, nil, owner))

	require.Equal(t, GameOver, lobby.State)
	require.Empty(t, lobby.CurrentWord)
	require.Len(t, events, 2)
	require.Equal(t, "secret", events[0].PreviousWord)
	require.Equal(t, ownerForcedGameEnd, events[0].RoundEndReason)
}

func Test_JoinPlayerWithoutRandomNamesRequiresName(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		EditableLobbySettings: EditableLobbySettings{
			AssignRandomNames: false,
		},
	}

	player := lobby.JoinPlayer("")
	require.True(t, player.NeedsName)
	require.False(t, player.AutoNamed)
	require.Empty(t, player.Name)
}

func Test_handleNameChangeEventRejectsBlankNameWhenRandomNamesDisabled(t *testing.T) {
	t.Parallel()

	lobby := &Lobby{
		EditableLobbySettings: EditableLobbySettings{
			AssignRandomNames: false,
		},
	}
	lobby.WriteObject = noOpWriteObject
	lobby.WritePreparedMessage = noOpWritePreparedMessage

	player := lobby.JoinPlayer("Kevin")
	handleNameChangeEvent(player, lobby, "")

	require.Equal(t, "Kevin", player.Name)
	require.False(t, player.NeedsName)
}
