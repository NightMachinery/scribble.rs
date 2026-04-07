package frontend

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lxzan/gws"
	"github.com/scribble-rs/scribble.rs/internal/api"
	"github.com/scribble-rs/scribble.rs/internal/config"
	"github.com/scribble-rs/scribble.rs/internal/game"
	"github.com/scribble-rs/scribble.rs/internal/state"
	"github.com/stretchr/testify/require"
)

func TestCreateLobby(t *testing.T) {
	t.Parallel()

	data := api.CreateLobbyData(
		&config.Default,
		&game.Lobby{
			LobbyID: "TEST",
		})

	var previousSize uint8
	for _, suggestedSize := range data.SuggestedBrushSizes {
		if suggestedSize < previousSize {
			t.Error("Sorting in SuggestedBrushSizes is incorrect")
		}
	}

	for _, suggestedSize := range data.SuggestedBrushSizes {
		if suggestedSize < game.MinBrushSize {
			t.Errorf("suggested brushsize %d is below MinBrushSize %d", suggestedSize, game.MinBrushSize)
		}

		if suggestedSize > game.MaxBrushSize {
			t.Errorf("suggested brushsize %d is above MaxBrushSize %d", suggestedSize, game.MaxBrushSize)
		}
	}
}

func TestJoinLobbyNoChecksAllowsRefreshingConnectedPlayer(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&config.Default)
	require.NoError(t, err)

	player, lobby, err := game.CreateLobby("", "player", "english", &game.EditableLobbySettings{
		Public:             false,
		DrawingTime:        120,
		Rounds:             4,
		MaxPlayers:         8,
		CustomWordsPerTurn: 3,
		ClientsPerIPLimit:  2,
		WordsPerTurn:       3,
	}, nil, game.ChillScoring)
	require.NoError(t, err)

	player.Connected = true
	player.SetWebsocket(&gws.Conn{})

	request := httptest.NewRequest("GET", "/lobby/"+lobby.LobbyID, nil)
	recorder := httptest.NewRecorder()

	pageData := handler.joinLobbyNoChecks(lobby, recorder, request, func() *game.Player {
		return player
	})

	require.NotNil(t, pageData)

	var foundUserSession bool
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "usersession" && cookie.Value == player.GetUserSession().String() {
			foundUserSession = true
			break
		}
	}
	require.True(t, foundUserSession)
}

func TestEnterLobbyRendersCSPCompatibleRestorePage(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&config.Default)
	require.NoError(t, err)

	_, lobby, err := game.CreateLobby("", "player", "english", &game.EditableLobbySettings{
		Public:             false,
		DrawingTime:        120,
		Rounds:             4,
		MaxPlayers:         8,
		CustomWordsPerTurn: 3,
		ClientsPerIPLimit:  2,
		WordsPerTurn:       3,
	}, nil, game.ChillScoring)
	require.NoError(t, err)

	state.AddLobby(lobby)
	t.Cleanup(func() {
		state.RemoveLobby(lobby.LobbyID)
	})

	request := httptest.NewRequest(http.MethodGet, "/lobby/"+lobby.LobbyID, nil)
	request.SetPathValue("lobby_id", lobby.LobbyID)
	request.Header.Set("User-Agent", "Mozilla/5.0 Chrome/123.0")

	recorder := httptest.NewRecorder()
	handler.ssrEnterLobby(recorder, request)

	response := recorder.Result()
	body := recorder.Body.String()

	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Contains(t, body, "Restoring session…")
	require.Contains(t, body, "resources/restore-session.js?cache_bust=")
	require.Contains(t, body, "client_id_restore_attempted=1")
	require.NotContains(t, body, "localStorage.getItem(\"scribble.client-id\")")
	require.NotContains(t, body, "<script>")
}

func TestEnterLobbySkipsRestorePageAfterRestoreAttempt(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&config.Default)
	require.NoError(t, err)

	_, lobby, err := game.CreateLobby("", "player", "english", &game.EditableLobbySettings{
		Public:             false,
		DrawingTime:        120,
		Rounds:             4,
		MaxPlayers:         8,
		CustomWordsPerTurn: 3,
		ClientsPerIPLimit:  2,
		WordsPerTurn:       3,
	}, nil, game.ChillScoring)
	require.NoError(t, err)

	state.AddLobby(lobby)
	t.Cleanup(func() {
		state.RemoveLobby(lobby.LobbyID)
	})

	request := httptest.NewRequest(http.MethodGet,
		"/lobby/"+lobby.LobbyID+"?"+clientIDRestoreAttempted+"=1", nil)
	request.SetPathValue("lobby_id", lobby.LobbyID)
	request.Header.Set("User-Agent", "Mozilla/5.0 Chrome/123.0")

	recorder := httptest.NewRecorder()
	handler.ssrEnterLobby(recorder, request)

	response := recorder.Result()
	body := recorder.Body.String()

	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NotContains(t, body, "Restoring session…")

	var foundUserSession bool
	for _, cookie := range response.Cookies() {
		if cookie.Name == "usersession" && strings.TrimSpace(cookie.Value) != "" {
			foundUserSession = true
			break
		}
	}
	require.True(t, foundUserSession)
}

func TestShouldAttemptClientIDRestoreStopsAfterQueryParam(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/lobby/TEST?"+clientIDRestoreAttempted+"=1", nil)

	require.False(t, shouldAttemptClientIDRestore(request))
}
