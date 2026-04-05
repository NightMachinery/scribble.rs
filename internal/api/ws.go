package api

import (
	json "encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/lxzan/gws"

	"github.com/scribble-rs/scribble.rs/internal/game"
	"github.com/scribble-rs/scribble.rs/internal/metrics"
	"github.com/scribble-rs/scribble.rs/internal/state"
)

var (
	ErrPlayerNotConnected = errors.New("player not connected")

	upgrader = gws.NewUpgrader(&socketHandler{}, &gws.ServerOption{
		Recovery:          gws.Recovery,
		ParallelEnabled:   true,
		PermessageDeflate: gws.PermessageDeflate{Enabled: true},
	})
)

func (handler *V1Handler) websocketUpgrade(writer http.ResponseWriter, request *http.Request) {
	userSession, err := GetUserSession(request)
	if err != nil {
		log.Printf("error getting user session: %v", err)
		userSession = uuid.Nil
	}
	clientID, clientIDErr := GetClientID(request)
	if clientIDErr != nil {
		log.Printf("error getting client id: %v", clientIDErr)
		clientID = uuid.Nil
	}

	if userSession == uuid.Nil && clientID == uuid.Nil {
		// This issue can happen if you illegally request a websocket
		// connection without ever having had a usersession or client-id or your
		// client having deleted the identity cookies.
		http.Error(writer, "you don't have access to this lobby;player identity not set", http.StatusUnauthorized)
		return
	}

	lobbyId := GetLobbyId(request)
	if lobbyId == "" {
		http.Error(writer, "lobby id missing", http.StatusBadRequest)
		return
	}

	lobby := state.GetLobby(lobbyId)
	if lobby == nil {
		http.Error(writer, ErrLobbyNotExistent.Error(), http.StatusNotFound)
		return
	}

	lobby.Synchronized(func() {
		player := GetPlayer(lobby, request)
		if player == nil {
			http.Error(writer, "you don't have access to this lobby;player identity unknown", http.StatusUnauthorized)
			return
		}

		socket, err := upgrader.Upgrade(writer, request)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		metrics.TrackPlayerConnect()

		previousSocket := player.GetWebsocket()
		connectionVersion := player.NextConnectionVersion()

		player.SetWebsocket(socket)
		socket.Session().Store(socketSessionPlayerKey, player)
		socket.Session().Store(socketSessionLobbyKey, lobby)
		socket.Session().Store(socketSessionConnectionVersionKey, connectionVersion)

		if previousSocket != nil && previousSocket != socket {
			if err := previousSocket.WriteClose(closeCodeConnectionReplaced, nil); err != nil && !errors.Is(err, gws.ErrConnClosed) {
				log.Printf("error closing replaced websocket: %v", err)
			}
		}

		lobby.OnPlayerConnectUnsynchronized(player)

		go socket.ReadLoop()
	})
}

const (
	pingInterval = 10 * time.Second
	pingWait     = 5 * time.Second

	closeCodeKicked             = 4000
	closeCodeConnectionReplaced = 4001

	socketSessionPlayerKey            = "player"
	socketSessionLobbyKey             = "lobby"
	socketSessionConnectionVersionKey = "connectionVersion"
)

type socketHandler struct{}

func (c *socketHandler) resetDeadline(socket *gws.Conn) {
	if err := socket.SetDeadline(time.Now().Add(pingInterval + pingWait)); err != nil {
		log.Printf("error resetting deadline: %s\n", err)
	}
}

func (c *socketHandler) OnOpen(socket *gws.Conn) {
	c.resetDeadline(socket)
}

func extract(x any, _ bool) any {
	return x
}

func (c *socketHandler) OnClose(socket *gws.Conn, _ error) {
	defer socket.Session().Delete(socketSessionPlayerKey)
	defer socket.Session().Delete(socketSessionLobbyKey)
	defer socket.Session().Delete(socketSessionConnectionVersionKey)

	player, ok := extract(socket.Session().Load(socketSessionPlayerKey)).(*game.Player)
	if !ok {
		return
	}
	lobby, ok := extract(socket.Session().Load(socketSessionLobbyKey)).(*game.Lobby)
	if !ok {
		return
	}
	connectionVersion, ok := extract(socket.Session().Load(socketSessionConnectionVersionKey)).(uint64)
	if !ok || !player.ConnectionVersionMatches(connectionVersion) || player.GetWebsocket() != socket {
		return
	}

	metrics.TrackPlayerDisconnect()
	lobby.OnPlayerDisconnect(player)
}

func (c *socketHandler) OnPing(socket *gws.Conn, _ []byte) {
	c.resetDeadline(socket)
	_ = socket.WritePong(nil)
}

func (c *socketHandler) OnPong(socket *gws.Conn, _ []byte) {
	c.resetDeadline(socket)
}

func (c *socketHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	defer c.resetDeadline(socket)

	player, ok := extract(socket.Session().Load(socketSessionPlayerKey)).(*game.Player)
	if !ok {
		return
	}
	lobby, ok := extract(socket.Session().Load(socketSessionLobbyKey)).(*game.Lobby)
	if !ok {
		return
	}
	connectionVersion, ok := extract(socket.Session().Load(socketSessionConnectionVersionKey)).(uint64)
	if !ok || !player.ConnectionVersionMatches(connectionVersion) || player.GetWebsocket() != socket {
		return
	}

	bytes := message.Bytes()
	handleIncommingEvent(lobby, player, bytes)
}

func handleIncommingEvent(lobby *game.Lobby, player *game.Player, data []byte) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("Error occurred in incomming event listener.\n\tError: %s\n\tPlayer: %s(%s)\nStack %s\n", err, player.Name, player.ID, string(debug.Stack()))
			// FIXME Should this lead to a disconnect?
		}
	}()

	var event game.EventTypeOnly
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("Error unmarshalling message: %s\n", err)
		err := WriteObject(player, game.Event{
			Type: game.EventTypeSystemMessage,
			Data: fmt.Sprintf("error parsing message, please report this issue via Github: %s!", err),
		})
		if err != nil {
			log.Printf("Error sending errormessage: %s\n", err)
		}
		return
	}

	if err := lobby.HandleEvent(event.Type, data, player); err != nil {
		log.Printf("Error handling event: %s\n", err)
	}
}

func WriteObject(player *game.Player, object any) error {
	socket := player.GetWebsocket()
	if socket == nil || !player.Connected {
		return ErrPlayerNotConnected
	}

	bytes, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("error marshalling payload: %w", err)
	}

	// We write async, as broadcast always uses the queue. If we use write, the
	// order will become messed up, potentially causing issues in the frontend.
	socket.WriteAsync(gws.OpcodeText, bytes, func(err error) {
		if err != nil {
			log.Println("Error responding to player:", err.Error())
		}
	})
	return nil
}

func WritePreparedMessage(player *game.Player, message *gws.Broadcaster) error {
	socket := player.GetWebsocket()
	if socket == nil || !player.Connected {
		return ErrPlayerNotConnected
	}

	return message.Broadcast(socket)
}
