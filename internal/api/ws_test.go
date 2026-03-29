package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lxzan/gws"
	"github.com/scribble-rs/scribble.rs/internal/config"
	"github.com/scribble-rs/scribble.rs/internal/game"
	"github.com/scribble-rs/scribble.rs/internal/state"
	"github.com/stretchr/testify/require"
)

type testSocketHandler struct {
	closeCh chan *gws.CloseError
	msgCh   chan []byte
}

func (handler *testSocketHandler) OnOpen(_ *gws.Conn) {}

func (handler *testSocketHandler) OnClose(_ *gws.Conn, err error) {
	if closeErr, ok := err.(*gws.CloseError); ok {
		select {
		case handler.closeCh <- closeErr:
		default:
		}
	}
}

func (handler *testSocketHandler) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.WritePong(payload)
}

func (handler *testSocketHandler) OnPong(_ *gws.Conn, _ []byte) {}

func (handler *testSocketHandler) OnMessage(_ *gws.Conn, message *gws.Message) {
	defer message.Close()

	bytes := append([]byte(nil), message.Bytes()...)
	select {
	case handler.msgCh <- bytes:
	default:
	}
}

func mustConnectTestClient(t *testing.T, serverURL string, lobby *game.Lobby, player *game.Player, handler *testSocketHandler) *gws.Conn {
	t.Helper()

	headers := make(http.Header)
	headers.Set("Cookie", fmt.Sprintf("usersession=%s; lobby-id=%s", player.GetUserSession().String(), lobby.LobbyID))

	socket, response, err := gws.NewClient(handler, &gws.ClientOption{
		Addr:          "ws" + serverURL[len("http"):] + "/v1/lobby/ws",
		RequestHeader: headers,
	})
	if response != nil {
		defer response.Body.Close()
	}
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)

	go socket.ReadLoop()
	return socket
}

func requireSocketMessage(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()

	select {
	case message := <-ch:
		return message
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for websocket message")
		return nil
	}
}

func requireSocketClose(t *testing.T, ch <-chan *gws.CloseError) *gws.CloseError {
	t.Helper()

	select {
	case closeErr := <-ch:
		return closeErr
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for websocket close")
		return nil
	}
}

func TestWebsocketReconnectReplacesPreviousConnectionAndPreservesScore(t *testing.T) {
	t.Parallel()

	handler := NewHandler(&config.Default)
	player, lobby, err := game.CreateLobby("", "player", "english", &game.EditableLobbySettings{
		Public:             false,
		DrawingTime:        120,
		Rounds:             4,
		MaxPlayers:         8,
		CustomWordsPerTurn: 3,
		ClientsPerIPLimit:  1,
		WordsPerTurn:       3,
	}, nil, game.ChillScoring)
	require.NoError(t, err)

	lobby.WriteObject = WriteObject
	lobby.WritePreparedMessage = WritePreparedMessage
	player.Score = 123

	state.AddLobby(lobby)
	defer state.RemoveLobby(lobby.LobbyID)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/lobby/ws", handler.websocketUpgrade)
	server := httptest.NewServer(mux)
	defer server.Close()

	firstHandler := &testSocketHandler{
		closeCh: make(chan *gws.CloseError, 2),
		msgCh:   make(chan []byte, 4),
	}
	firstSocket := mustConnectTestClient(t, server.URL, lobby, player, firstHandler)
	defer func() { _ = firstSocket.WriteClose(1000, nil) }()

	firstReadyRaw := requireSocketMessage(t, firstHandler.msgCh)
	var firstReadyEnvelope game.Event
	require.NoError(t, json.Unmarshal(firstReadyRaw, &firstReadyEnvelope))
	require.Equal(t, game.EventTypeReady, firstReadyEnvelope.Type)

	secondHandler := &testSocketHandler{
		closeCh: make(chan *gws.CloseError, 2),
		msgCh:   make(chan []byte, 4),
	}
	secondSocket := mustConnectTestClient(t, server.URL, lobby, player, secondHandler)
	defer func() { _ = secondSocket.WriteClose(1000, nil) }()

	closeErr := requireSocketClose(t, firstHandler.closeCh)
	require.Equal(t, uint16(closeCodeConnectionReplaced), closeErr.Code)

	secondReadyRaw := requireSocketMessage(t, secondHandler.msgCh)
	var envelope struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(secondReadyRaw, &envelope))
	require.Equal(t, game.EventTypeReady, envelope.Type)

	var ready game.ReadyEvent
	require.NoError(t, json.Unmarshal(envelope.Data, &ready))
	require.Equal(t, player.ID, ready.PlayerID)
	require.True(t, player.Connected)
	require.Equal(t, 123, player.Score)
	require.Len(t, ready.Players, 1)
	require.Equal(t, 123, ready.Players[0].Score)
}
