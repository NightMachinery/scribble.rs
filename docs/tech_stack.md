# Architecture and tech stack

This document gives a high-level map of the Scribble.rs codebase and the main technologies it is built on.

## System shape

- **Single-process Go web application**: one binary serves the HTTP API, WebSocket API, SSR pages, JS, CSS, images, sounds, and embedded word lists (`cmd/scribblers/main.go`, `internal/frontend/http.go`, `internal/game/words.go`).
- **No database / no external state store**: active lobbies and drawing state live in memory only (`internal/state/lobbies.go`, `internal/game/data.go`). Restarting the process drops live games.
- **Server-authoritative game logic**: game state transitions, turn timing, scoring, hint reveals, kick votes, drawing validation, and word choice all run on the server (`internal/game/lobby.go`, `internal/game/shared.go`).

## Runtime model

- **Process entrypoint**: `cmd/scribblers/main.go`
  - loads config
  - creates a stdlib `http.ServeMux`
  - wires API + frontend routes into the same server
  - optionally starts CPU profiling
  - starts a global lobby cleanup goroutine
  - handles graceful shutdown
- **Concurrency model**:
  - global lobby registry protected by an RW mutex (`internal/state/lobbies.go`)
  - each lobby has its own mutex for game-state mutation (`internal/game/data.go`)
  - each active turn uses a per-lobby ticker goroutine for round timing and hint reveal logic (`internal/game/lobby.go`)
  - WebSocket read loops run per connection (`internal/api/ws.go`)

## Major packages / modules

### `cmd/scribblers`

- **Main binary** (`cmd/scribblers/main.go`)
- Composes the whole app from config, API routes, frontend routes, state cleanup, and shutdown handling.

### `internal/game`

Core domain/gameplay package.

- **Lobby and player state**: `internal/game/data.go`
- **Game event schema shared with clients**: `internal/game/shared.go`
- **Main state machine / event handling**: `internal/game/lobby.go`
- **Word list loading + guess checking**: `internal/game/words.go`

Responsibilities:

- lobby creation and turn progression
- player state (`Guessing`, `Drawing`, `Standby`, `Ready`, `Spectating`)
- drawing-board event handling (`line`, `fill`, `undo`, `clear-drawing-board`)
- scoring modes (`chill`, `competitive`)
- hint generation/reveal
- custom words and built-in word packs

### `internal/api`

Public HTTP + WebSocket interface.

- **Route registration**: `internal/api/http.go`
- **REST-style handlers**: `internal/api/v1.go`
- **WebSocket upgrade + socket callbacks**: `internal/api/ws.go`
- **Input parsing/validation helpers**: `internal/api/createparse.go`

Responsibilities:

- `/v1/lobby` list/create/update/join endpoints
- `/v1/lobby/{id}/ws` real-time game connection
- cookie/session handling (`usersession`, `lobby-id`, optional Discord-specific cookies)
- request IP extraction from proxy headers
- marshaling lobby/game data into JSON

### `internal/frontend`

Official web client delivery and SSR.

- **Embedded templates/resources + route setup**: `internal/frontend/http.go`
- **SSR page handlers + templated JS**: `internal/frontend/index.go`, `internal/frontend/lobby.go`
- **Client runtime**: `internal/frontend/index.js`, `internal/frontend/lobby.js`
- **Static assets**: `internal/frontend/resources/*`
- **HTML templates**: `internal/frontend/templates/*`

Responsibilities:

- homepage and lobby page SSR
- serving embedded JS/CSS/images/audio
- cache-busting for static assets
- language selection from `Accept-Language`
- CSP headers for the official client

### `internal/state`

Global process-level application state.

- **Lobby registry and cleanup loop**: `internal/state/lobbies.go`

Responsibilities:

- in-memory list of active lobbies
- public-lobby discovery
- aggregate stats
- periodic cleanup of deserted lobbies
- graceful lobby shutdown on process exit

### `internal/config`

- **Environment / `.env` config loading**: `internal/config/config.go`

Responsibilities:

- parse env vars with defaults
- support root URL/path and optional extra static directories
- expose lobby setting defaults and bounds
- configure CORS and cleanup timing

### `internal/translations`

- **UI string registry**: `internal/translations/translations.go`
- **Locale bundles**: `internal/translations/*.go`

Responsibilities:

- register built-in translations at startup
- provide fallback to English
- mark RTL locales where needed

### `internal/metrics`

- **Prometheus metrics registry and handler**: `internal/metrics/metrics.go`

Currently exposes at least a connected-player gauge via `/v1/metrics`.

### `internal/sanitize`

- **Text normalization / transliteration**: `internal/sanitize/sanitize.go`

Used mainly for name handling and guess comparison.

### `internal/version`

- **Build-time version metadata**: `internal/version/version.go`

Fed by `-ldflags` in CI/builds.

## Networking and protocols

- **HTTP server**: Go standard library `net/http` (`cmd/scribblers/main.go`)
- **Routing style**: stdlib `ServeMux` method+path patterns (`cmd/scribblers/main.go`, `internal/api/http.go`, `internal/frontend/http.go`)
- **CORS**: `github.com/go-chi/cors` middleware wrapped around registered handlers (`cmd/scribblers/main.go`)
- **WebSockets**: `github.com/lxzan/gws` (`internal/api/ws.go`)
  - async writes
  - permessage-deflate enabled
  - ping/pong deadline management
- **Application protocol**: JSON event messages shared between server and client (`internal/game/shared.go`)
  - examples: `ready`, `word-chosen`, `update-wordhint`, `line`, `fill`, `message`, `next-turn`
- **Sessions**: cookie-based (`usersession`, `lobby-id`, optional Discord activity cookies) (`internal/api/v1.go`)

## Frontend delivery model

- **Server-side rendered HTML** via Go templates (`internal/frontend/templates/*`, `internal/frontend/index.go`, `internal/frontend/lobby.go`)
- **Vanilla browser JavaScript** (no React/Vue/Svelte build pipeline) (`internal/frontend/index.js`, `internal/frontend/lobby.js`)
- **Embedded assets** via `go:embed` (`internal/frontend/http.go`, `internal/frontend/index.go`, `internal/game/words.go`)
- **Templated JS**: JS files are rendered through Go templates to inject config, URLs, translations, and constants (`internal/frontend/index.go`, `internal/frontend/lobby.go`)
- **Static asset strategy**:
  - embedded resources served from `/resources/*`
  - MD5-based cache busting
  - long cache lifetime headers

## Persistence and state management

- **Primary state model**: in-memory lobbies (`internal/state/lobbies.go`)
- **Per-lobby data kept in memory** (`internal/game/data.go`):
  - players
  - current word/hints
  - round timers
  - current drawing instruction list
  - custom words / shuffled word lists
- **No durable persistence**:
  - no SQL/NoSQL database
  - no Redis/cache tier
  - no event store
- **Reconnect model**:
  - players reconnect using cookie identity
  - `ready` event rehydrates current game state (`internal/game/shared.go`, `internal/game/lobby.go`)

## Configuration and operability

- **Config sources**: environment variables and optional `.env` file (`internal/config/config.go`)
- **Operational knobs** include:
  - port / bind address
  - root URL / root path
  - CORS settings
  - lobby cleanup timing
  - default lobby settings and bounds
  - optional extra served directories
- **Observability**:
  - Prometheus endpoint `/v1/metrics` (`internal/metrics/metrics.go`)
  - aggregate stats endpoint `/v1/stats` (`internal/api/http.go`, `internal/state/lobbies.go`)
  - optional CPU profiling file path (`cmd/scribblers/main.go`)
- **Deployment shape**:
  - primarily designed as a single Go service behind a reverse proxy
  - repo includes Dockerfiles and Fly.io config, but runtime architecture is still one app process (`linux.Dockerfile`, `windows.Dockerfile`, `fly.toml`)

## Built-in content and localization

- **Word packs** are embedded from `internal/game/words/*` and selected per lobby (`internal/game/words.go`)
- **Supported UI/game languages** are registered in code (`internal/game/lobby.go`, `internal/translations/*.go`)
- **RTL support** exists for languages such as Arabic, Hebrew, and Persian (`internal/game/words.go`, `internal/translations/translations.go`)

## Notable third-party libraries

From `go.mod` and usage in code:

- `github.com/lxzan/gws`
  - WebSocket server implementation (`internal/api/ws.go`)
- `github.com/go-chi/cors`
  - CORS middleware (`cmd/scribblers/main.go`)
- `github.com/caarlos0/env/v11`
  - env var parsing into structs (`internal/config/config.go`)
- `github.com/subosito/gotenv`
  - `.env` loading (`internal/config/config.go`)
- `github.com/prometheus/client_golang`
  - Prometheus metrics export (`internal/metrics/metrics.go`)
- `golang.org/x/text`
  - locale handling, lowercasing, and `Accept-Language` parsing (`internal/game/words.go`, `internal/frontend/lobby.go`, `internal/frontend/index.go`)
- `github.com/gofrs/uuid/v5`
  - lobby/player/session IDs (`internal/game`, `internal/api`, `internal/frontend`)
- `github.com/Bios-Marcel/go-petname`
  - default generated player names (`internal/game/lobby.go`)
- `github.com/Bios-Marcel/discordemojimap/v2`
  - emoji-code replacement in chat / names (`internal/game/data.go`, `internal/game/lobby.go`)
- `github.com/stretchr/testify`
  - tests only

## Summary

Scribble.rs is a **monolithic Go application** with:

- **stdlib HTTP + JSON/WebSocket APIs**
- **server-authoritative multiplayer game logic**
- **embedded SSR + vanilla-JS frontend**
- **in-memory lobby/state management**
- **no database dependency**
- **light observability/config via env vars and Prometheus**

It is architecturally simple: one process, one in-memory state model, one real-time protocol, and one embedded web client.
