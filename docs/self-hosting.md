# Self-hosting

This repo already ships the browser client inside the Go server binary.

- Frontend templates are embedded with `//go:embed`.
- Static assets under `internal/frontend/resources/` are embedded too.
- No CDN, webfont download, or Docker image is required for normal intranet use.

The `self_host.zsh` helper below builds the current local checkout, updates `~/Caddyfile`, reloads Caddy, and runs Scribble.rs in `tmux`.

## What it does

- public URL: defaults to `http://m2.pinky.lilf.ir`
- internal bind: `127.0.0.1:38180`
- process manager: `tmux`
- reverse proxy: `Caddy`
- redeploy source: latest local files in this checkout

Port `3000` is intentionally not used.

## Prerequisites

- `go`
- `git`
- `tmux`
- `caddy`
- Go 1.25 support (`go` may auto-download the Go 1.25 toolchain on first build)
- a running Caddy instance that can be reloaded with:

```zsh
caddy reload --config ~/Caddyfile
```

## Commands

From the repo root:

```zsh
./self_host.zsh setup
./self_host.zsh setup http://m2.pinky.lilf.ir
./self_host.zsh redeploy
./self_host.zsh redeploy https://scribble.example.internal
./self_host.zsh start
./self_host.zsh stop
```

### `setup [url]`

- writes runtime files to `.self-host/`
- builds the Go binary from the current checkout
- writes or updates the managed `scribble.rs` block in `~/Caddyfile`
- validates and reloads Caddy
- starts the app in tmux session `scribble-rs-self-host`

### `redeploy [url]`

Same as `setup`, but meant for rebuilding and restarting after local edits.

### `start`

Starts the already-built app again using the last saved config.

### `stop`

Stops the tmux session.

## URL behavior

The script accepts a full public URL.

Examples:

```zsh
./self_host.zsh setup http://m2.pinky.lilf.ir
./self_host.zsh setup https://games.example.internal
./self_host.zsh setup http://intranet.example.local/scribble
```

It uses:

- `ROOT_URL` = scheme + host portion
- `ROOT_PATH` = optional path portion
- Caddy reverse proxy rules that match the same URL/path

If you deploy under a subpath on a host that is already handled elsewhere in `~/Caddyfile`, you may need to merge the generated block into that existing site block manually.

## Logs and tmux

- tmux session: `scribble-rs-self-host`
- log file: `.self-host/logs/scribble-rs.log`

Useful commands:

```zsh
tmux attach -t scribble-rs-self-host
tail -f .self-host/logs/scribble-rs.log
```

## Managed Caddy block

The script manages this marker range inside `~/Caddyfile`:

```text
# BEGIN scribble.rs self-host
...
# END scribble.rs self-host
```

Running `setup` or `redeploy` replaces only that block.

## Proxy / Node notes

The script exports the proxy environment requested for build/runtime commands:

```zsh
export ALL_PROXY=http://127.0.0.1:2097 all_proxy=http://127.0.0.1:2097 http_proxy=http://127.0.0.1:2097 https_proxy=http://127.0.0.1:2097 HTTP_PROXY=http://127.0.0.1:2097 HTTPS_PROXY=http://127.0.0.1:2097 npm_config_proxy=http://127.0.0.1:2097 npm_config_https_proxy=http://127.0.0.1:2097
```

Node is not needed for this deployment path because the client assets are already embedded in the Go server. If you later add a Node-based build step in zsh, load Node with:

```zsh
nvm-load
nvm use VERSION
```
