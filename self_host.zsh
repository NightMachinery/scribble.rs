#!/usr/bin/env zsh
set -euo pipefail

DEFAULT_PUBLIC_URL='http://scribble.pinky.lilf.ir'
APP_HOST='127.0.0.1'
APP_PORT='38180'
SESSION_NAME='scribble-rs-self-host'
CADDY_BLOCK_START='# BEGIN scribble.rs self-host'
CADDY_BLOCK_END='# END scribble.rs self-host'

REPO_ROOT="${0:A:h}"
RUNTIME_DIR="$REPO_ROOT/.self-host"
BIN_DIR="$RUNTIME_DIR/bin"
LOG_DIR="$RUNTIME_DIR/logs"
BIN_PATH="$BIN_DIR/scribblers"
ENV_PATH="$RUNTIME_DIR/app.env"
STATE_PATH="$RUNTIME_DIR/state.env"
LOG_PATH="$LOG_DIR/scribble-rs.log"
CADDYFILE="$HOME/Caddyfile"

PUBLIC_URL=''
SITE_ADDRESS=''
ROOT_URL=''
ROOT_PATH=''
ROOT_PATH_WITH_SLASH=''
PUBLIC_HOST=''

usage() {
  cat <<USAGE
Usage:
  ./self_host.zsh setup [public-url]
  ./self_host.zsh redeploy [public-url]
  ./self_host.zsh start
  ./self_host.zsh stop

Default public URL: $DEFAULT_PUBLIC_URL
Internal app bind:  http://$APP_HOST:$APP_PORT
USAGE
}

tmuxnew () {
  local session="$1"
  shift

  tmux kill-session -t "$session" &>/dev/null || true
  tmux new-session -d -s "$session" "$@"
}

tmuxnew_with_env() {
  local session="$1"
  shift
  local command="$1"
  shift
  local -a tmux_args=(-d -s "$session")
  local env_assignment

  for env_assignment in "$@"; do
    tmux_args+=(-e "$env_assignment")
  done

  tmux kill-session -t "$session" &>/dev/null || true
  tmux new-session "${tmux_args[@]}" "$command"
}

die() {
  print -u2 -- "Error: $*"
  exit 1
}

note() {
  print -- "==> $*"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"
}

load_proxy_env() {
  export ALL_PROXY=http://127.0.0.1:2097 all_proxy=http://127.0.0.1:2097 http_proxy=http://127.0.0.1:2097 https_proxy=http://127.0.0.1:2097 HTTP_PROXY=http://127.0.0.1:2097 HTTPS_PROXY=http://127.0.0.1:2097 npm_config_proxy=http://127.0.0.1:2097 npm_config_https_proxy=http://127.0.0.1:2097
}

ensure_prereqs() {
  require_command git
  require_command go
  require_command tmux
  require_command caddy
  require_command python3
}

normalize_public_url() {
  local input="${1:-$DEFAULT_PUBLIC_URL}"
  input="${input%/}"
  if [[ "$input" != *"://"* ]]; then
    input="http://$input"
  fi
  print -r -- "$input"
}

parse_public_url() {
  local url="$1"
  local scheme rest hostport raw_path

  [[ "$url" == http://* || "$url" == https://* ]] || die "Only http:// and https:// URLs are supported: $url"
  [[ "$url" != *\?* && "$url" != *\#* ]] || die "Query strings and fragments are not supported in the public URL: $url"

  scheme="${url%%://*}"
  rest="${url#*://}"
  [[ -n "$rest" && "$rest" != */ ]] || true

  if [[ "$rest" == */* ]]; then
    hostport="${rest%%/*}"
    raw_path="/${rest#*/}"
  else
    hostport="$rest"
    raw_path=''
  fi

  [[ -n "$hostport" ]] || die "Could not parse host from public URL: $url"
  [[ "$hostport" != *' '* ]] || die "Spaces are not allowed in the public URL host: $url"

  if [[ "$raw_path" == '/' ]]; then
    raw_path=''
  else
    raw_path="${raw_path%/}"
  fi

  PUBLIC_URL="$url"
  SITE_ADDRESS="$scheme://$hostport"
  ROOT_URL="$scheme://$hostport"
  ROOT_PATH_WITH_SLASH="$raw_path"
  ROOT_PATH="${raw_path#/}"

  if [[ "$hostport" == \[* ]]; then
    PUBLIC_HOST="${hostport#\[}"
    PUBLIC_HOST="${PUBLIC_HOST%%\]*}"
  else
    PUBLIC_HOST="${hostport%%:*}"
  fi
}

should_apply_ir_defaults() {
  [[ "${PUBLIC_HOST:l}" == *.ir ]]
}

ensure_runtime_dirs() {
  mkdir -p "$BIN_DIR" "$LOG_DIR"
}

write_runtime_files() {
  ensure_runtime_dirs

  cat > "$ENV_PATH" <<EOF_ENV
PORT=$APP_PORT
NETWORK_ADDRESS=$APP_HOST
ROOT_URL=$ROOT_URL
CANONICAL_URL=$ROOT_URL
ALLOW_INDEXING=false
ROOT_PATH=$ROOT_PATH
EOF_ENV

  if should_apply_ir_defaults; then
    cat >> "$ENV_PATH" <<EOF_IR
LOBBY_SETTING_DEFAULTS_WORDPACK=Persian_1
EOF_IR
  fi

  cat > "$STATE_PATH" <<EOF_STATE
PUBLIC_URL=$PUBLIC_URL
SITE_ADDRESS=$SITE_ADDRESS
ROOT_URL=$ROOT_URL
ROOT_PATH=$ROOT_PATH
ROOT_PATH_WITH_SLASH=$ROOT_PATH_WITH_SLASH
APP_HOST=$APP_HOST
APP_PORT=$APP_PORT
SESSION_NAME=$SESSION_NAME
BIN_PATH=$BIN_PATH
ENV_PATH=$ENV_PATH
LOG_PATH=$LOG_PATH
EOF_STATE
}

load_state() {
  [[ -f "$STATE_PATH" ]] || die "Missing $STATE_PATH. Run ./self_host.zsh setup [public-url] first."
  source "$STATE_PATH"

  PUBLIC_URL="$PUBLIC_URL"
  SITE_ADDRESS="$SITE_ADDRESS"
  ROOT_URL="$ROOT_URL"
  ROOT_PATH="$ROOT_PATH"
  ROOT_PATH_WITH_SLASH="$ROOT_PATH_WITH_SLASH"
}

go_mod_version() {
  sed -nE 's/^go[[:space:]]+([0-9]+\.[0-9]+(\.[0-9]+)?).*/\1/p' "$REPO_ROOT/go.mod" | head -n 1
}

required_go_family() {
  local version="${1:-}"
  if [[ "$version" == *.*.* ]]; then
    print -r -- "${version%.*}"
  else
    print -r -- "$version"
  fi
}

toolchain_cache_dir() {
  local version="$1"
  local gomodcache goos goarch
  gomodcache="$(GOTOOLCHAIN=local go env GOMODCACHE 2>/dev/null || true)"
  goos="$(GOTOOLCHAIN=local go env GOOS 2>/dev/null || true)"
  goarch="$(GOTOOLCHAIN=local go env GOARCH 2>/dev/null || true)"

  [[ -n "$version" && -n "$gomodcache" && -n "$goos" && -n "$goarch" ]] || return 0
  print -r -- "$gomodcache/golang.org/toolchain@v0.0.1-go${version}.${goos}-${goarch}"
}

repair_incomplete_go_toolchain() {
  local required_version toolchain_dir
  required_version="$(go_mod_version)"
  toolchain_dir="$(toolchain_cache_dir "$required_version")"

  [[ -n "$toolchain_dir" && -d "$toolchain_dir" ]] || return 0
  [[ -x "$toolchain_dir/bin/go" ]] && return 0

  note "Found incomplete cached Go toolchain for go$required_version; clearing it so Go can reinstall it."
  chmod -R u+w "$toolchain_dir" 2>/dev/null || true
  rm -rf "$toolchain_dir"
}

build_binary() {
  ensure_runtime_dirs
  load_proxy_env

  local version ldflags local_go_version required_version required_family
  version="$(git -C "$REPO_ROOT" describe --tags --dirty --always 2>/dev/null || print dev)"
  ldflags="-w -s -X github.com/scribble-rs/scribble.rs/internal/version.Version=$version"
  local_go_version="$(GOTOOLCHAIN=local go env GOVERSION 2>/dev/null || true)"
  required_version="$(go_mod_version)"
  required_family="$(required_go_family "$required_version")"

  if [[ -n "$local_go_version" && -n "$required_family" && "$local_go_version" != go${required_family}* ]]; then
    note "Local Go is $local_go_version; the first build may download Go $required_version automatically."
  fi

  repair_incomplete_go_toolchain

  note "Building Scribble.rs ($version)"
  (
    cd "$REPO_ROOT"
    CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$BIN_PATH" ./cmd/scribblers
  )
}

render_caddy_block() {
  if [[ -n "$ROOT_PATH_WITH_SLASH" ]]; then
    cat <<EOF_BLOCK
$SITE_ADDRESS {
    encode zstd gzip

    @scribble_rs path $ROOT_PATH_WITH_SLASH $ROOT_PATH_WITH_SLASH/*
    handle @scribble_rs {
        reverse_proxy $APP_HOST:$APP_PORT
    }
}
EOF_BLOCK
  else
    cat <<EOF_BLOCK
$SITE_ADDRESS {
    encode zstd gzip
    reverse_proxy $APP_HOST:$APP_PORT
}
EOF_BLOCK
  fi
}

write_managed_caddy_block() {
  local block_file backup_file
  block_file="$(mktemp)"
  backup_file="$(mktemp)"

  render_caddy_block > "$block_file"
  if [[ -f "$CADDYFILE" ]]; then
    cp "$CADDYFILE" "$backup_file"
  else
    : > "$backup_file"
    : > "$CADDYFILE"
  fi

  python3 - "$CADDYFILE" "$CADDY_BLOCK_START" "$CADDY_BLOCK_END" "$block_file" <<'PY'
from pathlib import Path
import sys

caddy_path = Path(sys.argv[1])
start = sys.argv[2]
end = sys.argv[3]
block_path = Path(sys.argv[4])

existing = caddy_path.read_text() if caddy_path.exists() else ""
managed = f"{start}\n{block_path.read_text().rstrip()}\n{end}\n"

if start in existing and end in existing:
    before, rest = existing.split(start, 1)
    _, after = rest.split(end, 1)
    before = before.rstrip()
    after = after.lstrip("\n")
    pieces = []
    if before:
        pieces.append(before)
    pieces.append(managed.rstrip())
    if after:
        pieces.append(after.rstrip())
    updated = "\n\n".join(pieces) + "\n"
else:
    existing = existing.rstrip()
    if existing:
        updated = existing + "\n\n" + managed
    else:
        updated = managed

caddy_path.write_text(updated)
PY

  if ! caddy validate --config "$CADDYFILE" >/dev/null; then
    cp "$backup_file" "$CADDYFILE"
    rm -f "$block_file" "$backup_file"
    die "Caddyfile validation failed; restored the previous ~/Caddyfile"
  fi

  rm -f "$block_file" "$backup_file"
}

reload_caddy() {
  note "Reloading Caddy"
  caddy reload --config "$CADDYFILE" >/dev/null || die "Failed to reload Caddy with $CADDYFILE"
}

start_app() {
  ensure_runtime_dirs
  [[ -f "$BIN_PATH" ]] || die "Missing $BIN_PATH. Run ./self_host.zsh setup or redeploy first."
  [[ -f "$ENV_PATH" ]] || die "Missing $ENV_PATH. Run ./self_host.zsh setup first."

  note "Starting app in tmux session $SESSION_NAME"
  tmuxnew_with_env "$SESSION_NAME" "zsh -lc 'cd ${(q)REPO_ROOT} && set -a && source ${(q)ENV_PATH} && set +a && exec ${(q)BIN_PATH} >> ${(q)LOG_PATH} 2>&1'" \
    "ALL_PROXY=http://127.0.0.1:2097" \
    "all_proxy=http://127.0.0.1:2097" \
    "http_proxy=http://127.0.0.1:2097" \
    "https_proxy=http://127.0.0.1:2097" \
    "HTTP_PROXY=http://127.0.0.1:2097" \
    "HTTPS_PROXY=http://127.0.0.1:2097" \
    "npm_config_proxy=http://127.0.0.1:2097" \
    "npm_config_https_proxy=http://127.0.0.1:2097"
}

stop_app() {
  if tmux has-session -t "$SESSION_NAME" 2>/dev/null; then
    note "Stopping tmux session $SESSION_NAME"
    tmux kill-session -t "$SESSION_NAME"
  else
    note "tmux session $SESSION_NAME is not running"
  fi
}

deploy() {
  local requested_url
  requested_url="$(normalize_public_url "${1:-${PUBLIC_URL:-$DEFAULT_PUBLIC_URL}}")"
  parse_public_url "$requested_url"
  note "Using public URL: $PUBLIC_URL"

  ensure_runtime_dirs
  write_runtime_files
  build_binary
  write_managed_caddy_block
  reload_caddy
  start_app

  note "Done"
  print -- "App session : $SESSION_NAME"
  print -- "Public URL  : $PUBLIC_URL"
  print -- "Local bind  : http://$APP_HOST:$APP_PORT"
  print -- "Logs        : $LOG_PATH"
}

main() {
  local command="${1:-}"
  case "$command" in
    setup)
      ensure_prereqs
      deploy "${2:-$DEFAULT_PUBLIC_URL}"
      ;;
    redeploy)
      ensure_prereqs
      if [[ -f "$STATE_PATH" ]]; then
        load_state
      fi
      deploy "${2:-${PUBLIC_URL:-$DEFAULT_PUBLIC_URL}}"
      ;;
    start)
      ensure_prereqs
      load_state
      start_app
      print -- "Started $SESSION_NAME for $PUBLIC_URL"
      ;;
    stop)
      stop_app
      ;;
    ''|-h|--help|help)
      usage
      ;;
    *)
      usage >&2
      die "Unknown command: $command"
      ;;
  esac
}

main "$@"
