# Auth and reconnection

## Current auth model

Scribble.rs does **not** have account login or full durable user identities.

What it has instead is **session-based player identity with browser fallback**:

- each player gets a random `usersession` UUID
- the official web client stores it in a `usersession` cookie
- each browser also gets a stable `client-id` UUID
- the official web client stores `client-id` in both a cookie and local storage
- unofficial clients can also send it as the `Usersession` request header
- unofficial clients can also send the browser identity as the `Client-Id` request header
- the lobby context is usually carried by the `lobby-id` cookie
- display names are durably persisted by `client-id`

Relevant code:

- cookie / header lookup: `internal/api/v1.go`
- websocket auth: `internal/api/ws.go`
- player lookup by session: `internal/game/data.go`

## What counts as authorization

There are a few separate checks in play:

- **Player identity**: the server first tries `usersession`, then falls back to `client-id`
- **Lobby selection**: the server uses the path lobby ID or `lobby-id` cookie
- **Lobby password**: optional join gate for private lobbies; this is **not** a user identity system
- **Owner permissions**: lobby-owner actions are authorized by comparing the owner player's `usersession`

So the current system is better described as **cookie/header session identity + lobby-level authorization**, not account auth.

## Cookie behavior

### Normal web client

`SetGameplayCookies` writes:

- `usersession`
- `client-id`
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

Reconnects work by reusing the same `usersession` or, if that is missing,
falling back to the same `client-id` and finding the same in-memory `Player`.

That means reconnect continuity only exists while all of the following remain true:

- the browser still has the same `usersession` or `client-id`
- the lobby still exists in memory
- the lobby has not been cleaned up for inactivity

If any of those are no longer true, the client will not reconnect as the same player and may end up creating/joining as a different player identity.

## What the websocket does on connect

The websocket upgrade:

1. reads `usersession` / `client-id`
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

- lobby cleanup/removal
- losing both the session cookie and the browser `client-id`
- reconnecting from a different browser profile without the same browser identity

## Important non-goals of the current system

This system does **not** provide:

- user accounts
- login/logout
- cross-device identity portability by default
- durable persistence of scores or lobbies
- cryptographic proof of user identity beyond possession of the session token
