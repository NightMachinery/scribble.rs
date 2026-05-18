package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scribble-rs/scribble.rs/internal/config"
	"github.com/scribble-rs/scribble.rs/internal/state"
	"github.com/stretchr/testify/require"
)

func TestPostLobbyDefaultsAssignRandomNamesToFalseWhenOmitted(t *testing.T) {
	t.Parallel()

	handler := NewHandler(&config.Default)
	lobbyID := "11111111-1111-1111-1111-111111111111"
	body := strings.NewReader(
		"lobby_id=" + lobbyID +
			"&username=tester" +
			"&wordpack=english" +
			"&drawing_time=120" +
			"&rounds=4" +
			"&max_players=8" +
			"&custom_words_per_turn=1" +
			"&clients_per_ip_limit=2" +
			"&public=false" +
			"&words_per_turn=3",
	)

	request := httptest.NewRequest(http.MethodPost, "/v1/lobby", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	handler.postLobby(recorder, request)
	t.Cleanup(func() {
		state.RemoveLobby(lobbyID)
	})

	response := recorder.Result()
	defer response.Body.Close()

	require.Equal(t, http.StatusOK, response.StatusCode)

	var lobbyData LobbyData
	require.NoError(t, json.NewDecoder(response.Body).Decode(&lobbyData))
	require.Equal(t, 120, lobbyData.DrawingTime)
	require.False(t, lobbyData.AssignRandomNames)
}
