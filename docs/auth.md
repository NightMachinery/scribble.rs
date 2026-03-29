# Auth and reconnection

## Current auth model

Scribble.rs does **not** have account login or durable user identities today.

What it has instead is **session-based player identity**:

- each player gets a random `usersession` UUID
- the official web client stores it in a `usersession` cookie
- unofficial clients can also send it as the `Usersession` request header
- the lobby context is usually carried by the `lobby-id` cookie

Relevant code:

- cookie / header lookup: `internal/api/v1.go`
- websocket auth: `internal/api/ws.go`
- player lookup by session: `internal/game/data.go`

## What counts as authorization

There are a few separate checks in play:

- **Player identity**: the `usersession` maps back to an existing in-memory `Player`
- **Lobby selection**: the server uses the path lobby ID or `lobby-id` cookie
- **Lobby password**: optional join gate for private lobbies; this is **not** a user identity system
- **Owner permissions**: lobby-owner actions are authorized by comparing the owner player's `usersession`

So the current system is better described as **cookie/header session identity + lobby-level authorization**, not account auth.

## Cookie behavior

### Normal web client

`SetGameplayCookies` writes:

- `usersession`
- `lobby-id`

with:

- `Path=/`
- `SameSite=Strict`

### Discord activity case

When running through the Discord integration, the same gameplay cookies are set on the Discord proxy domain with:

- `SameSite=None`
- `Partitioned`
- `Secure`

There is also a `discord-instance-id` cookie used for that integration path.

## Reconnection model

Reconnects work by reusing the same `usersession` and finding the same in-memory `Player`.

That means reconnect continuity only exists while all of the following remain true:

- the browser still has the same `usersession`
- the lobby still exists in memory
- the process has not restarted
- the lobby has not been cleaned up for inactivity

If any of those are no longer true, the client will not reconnect as the same player and may end up creating/joining as a different player identity.

## What the websocket does on connect

The websocket upgrade:

1. reads `usersession`
2. resolves the target lobby
3. finds the existing player for that session
4. attaches the new websocket as the authoritative connection for that player
5. sends a `ready` event to rehydrate client state

The `ready` event includes the current game snapshot, including:

- player ID / player name
- player list and scores
- round / game state
- current drawing
- current word hints appropriate for that player state

If the reconnecting player is the drawer and is still in word-choice state, the server also sends `your-turn`.

Relevant code:

- `ready` payload: `internal/game/shared.go`
- reconnect payload generation: `internal/game/lobby.go`

## Duplicate tabs / refresh behavior

The server treats websocket connections for the same player session as **single-owner**:

- the **newest** websocket wins
- any previous websocket for that same player session is closed with close code **4001**
- the replaced tab should **not** auto-reconnect
- kick still uses close code **4000**

This makes refresh/new-tab behavior reliable even when the browser does not deliver `beforeunload` cleanly.

## Score preservation

Player score is stored on the in-memory `Player` object, not in the browser.

Because reconnects resolve back to the same player object, a refresh/reconnect preserves:

- `Score`
- `LastScore`
- drawing / guessing role
- current-round context

This is why score survives a normal refresh, but **does not** survive:

- process restart
- lobby cleanup/removal
- losing the session cookie
- reconnecting from a different browser profile without the same cookies

## Important non-goals of the current system

This system does **not** provide:

- user accounts
- login/logout
- cross-device identity portability by default
- durable persistence of scores or lobbies
- cryptographic proof of user identity beyond possession of the session token
