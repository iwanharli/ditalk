#!/usr/bin/env bash
#
# Runs the ditalk backend and frontend together.
#
#   ./run.sh              backend + frontend
#   ./run.sh --worker     also start the queue worker
#   ./run.sh --help
#
# Written for bash 3.2, the version macOS ships, so no `wait -n`,
# associative arrays, or namerefs.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

WITH_WORKER=0
for arg in "$@"; do
  case "$arg" in
    --worker) WITH_WORKER=1 ;;
    -h|--help)
      sed -n '3,8p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "opsi tidak dikenal: $arg (pakai --help)" >&2
      exit 2
      ;;
  esac
done

# ---------------------------------------------------------------- output helpers

if [ -t 1 ]; then
  C_BE=$'\033[36m'; C_FE=$'\033[35m'; C_WK=$'\033[33m'
  C_WARN=$'\033[33m'; C_ERR=$'\033[31m'; C_OK=$'\033[32m'; C_OFF=$'\033[0m'
else
  C_BE=; C_FE=; C_WK=; C_WARN=; C_ERR=; C_OK=; C_OFF=
fi

info()  { printf '%s\n' "$*"; }
ok()    { printf '%s✓%s %s\n' "$C_OK" "$C_OFF" "$*"; }
warn()  { printf '%s!%s %s\n' "$C_WARN" "$C_OFF" "$*" >&2; }
fatal() { printf '%s✗%s %s\n' "$C_ERR" "$C_OFF" "$*" >&2; exit 1; }

# Tags each line of a service's output so interleaved logs stay readable.
prefix() {
  local tag="$1" color="$2"
  while IFS= read -r line; do
    printf '%s[%s]%s %s\n' "$color" "$tag" "$C_OFF" "$line"
  done
}

# ------------------------------------------------------------------- environment

# load_env applies .env without clobbering variables already in the environment.
# Sourcing the file would invert that precedence, so `PORT=9000 ./run.sh` would
# be silently ignored, and it would also disagree with godotenv on the Go side.
load_env() {
  local file="$1" line key val
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      ''|'#'*) continue ;;
    esac

    line="${line#export }"
    key="${line%%=*}"
    val="${line#*=}"

    case "$key" in
      ''|*[!A-Za-z0-9_]*) continue ;;
    esac

    # Strip one layer of surrounding quotes, as dotenv files commonly use them.
    case "$val" in
      \"*\") val="${val#\"}"; val="${val%\"}" ;;
      \'*\') val="${val#\'}"; val="${val%\'}" ;;
    esac

    if [ -z "${!key:-}" ]; then
      export "$key=$val"
    fi
  done < "$file"
}

if [ -f .env ]; then
  load_env .env
  ok ".env dimuat"
else
  warn ".env tidak ada. Jalankan: cp .env.example .env"
fi

PORT="${PORT:-8080}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"
DATABASE_URL="${DATABASE_URL:-postgres://localhost:5432/db_ditalk?sslmode=disable}"
REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"

if [ -z "${ENCRYPTION_KEY:-}" ]; then
  warn "ENCRYPTION_KEY kosong; import chat akan ditolak 503."
  warn "  Buat kunci: openssl rand -base64 32"
fi

# ------------------------------------------------------------------ prerequisites

for cmd in go npm psql lsof; do
  command -v "$cmd" >/dev/null || fatal "$cmd tidak ditemukan di PATH"
done

if ! psql "$DATABASE_URL" -tAc 'select 1' >/dev/null 2>&1; then
  fatal "PostgreSQL tidak dapat dihubungi di $DATABASE_URL
  Pastikan Postgres berjalan, lalu: createdb db_ditalk"
fi
ok "PostgreSQL terhubung"

# Redis only matters for the worker; the API starts fine without it.
redis_up=0
if command -v redis-cli >/dev/null; then
  redis_host="${REDIS_ADDR%%:*}"
  redis_port="${REDIS_ADDR##*:}"
  if [ "$(redis-cli -h "$redis_host" -p "$redis_port" ping 2>/dev/null)" = "PONG" ]; then
    redis_up=1
    ok "Redis terhubung"
  fi
fi
if [ "$redis_up" -eq 0 ]; then
  if [ "$WITH_WORKER" -eq 1 ]; then
    fatal "Redis tidak berjalan, padahal --worker diminta.
  Jalankan: brew services start redis"
  fi
  warn "Redis tidak berjalan; queue tidak akan memproses job."
fi

for p in "$PORT" "$FRONTEND_PORT"; do
  if lsof -ti:"$p" >/dev/null 2>&1; then
    fatal "Port $p sudah dipakai oleh PID $(lsof -ti:"$p" | tr '\n' ' ')
  Hentikan proses itu, atau ubah PORT / FRONTEND_PORT."
  fi
done

if [ ! -d frontend/node_modules ]; then
  info "Memasang dependency frontend..."
  (cd frontend && npm install) || fatal "npm install gagal"
fi

# ------------------------------------------------------------------------- build

# Build first, then run the binary directly. `go run` spawns a child that
# survives a kill of its parent and leaves the port occupied.
info "Membangun backend..."
mkdir -p backend/bin
(cd backend && go build -o bin/api ./cmd/api) || fatal "build backend gagal"
if [ "$WITH_WORKER" -eq 1 ]; then
  (cd backend && go build -o bin/worker ./cmd/worker) || fatal "build worker gagal"
fi
ok "Backend terbangun"

# ------------------------------------------------------------------------ launch

PIDS=""
SHUTTING_DOWN=0

# kill_tree stops a process and its descendants. npm spawns vite as a child, and
# signalling only the parent can leave the port held.
kill_tree() {
  local pid="$1" sig="${2:-TERM}" child
  for child in $(pgrep -P "$pid" 2>/dev/null); do
    kill_tree "$child" "$sig"
  done
  kill -"$sig" "$pid" 2>/dev/null
}

alive() { kill -0 "$1" 2>/dev/null; }

shutdown() {
  [ "$SHUTTING_DOWN" -eq 1 ] && return
  SHUTTING_DOWN=1
  trap - INT TERM EXIT
  printf '\n'
  info "Menghentikan service..."

  local pid i any
  for pid in $PIDS; do
    kill_tree "$pid" TERM
  done

  # Let each process exit cleanly before forcing it.
  i=0
  while [ "$i" -lt 20 ]; do
    any=0
    for pid in $PIDS; do
      alive "$pid" && any=1
    done
    [ "$any" -eq 0 ] && break
    sleep 0.25
    i=$((i + 1))
  done

  for pid in $PIDS; do
    alive "$pid" && kill_tree "$pid" KILL
  done

  ok "Selesai"
}

# A signal means the user asked to stop, so exit quietly. Falling back into the
# watch loop would print a misleading "a service died" warning.
on_signal() {
  shutdown
  exit 0
}
trap on_signal INT TERM
trap shutdown EXIT

PORT="$PORT" backend/bin/api > >(prefix "be" "$C_BE") 2>&1 &
PIDS="$PIDS $!"

if [ "$WITH_WORKER" -eq 1 ]; then
  backend/bin/worker > >(prefix "worker" "$C_WK") 2>&1 &
  PIDS="$PIDS $!"
fi

(cd frontend && exec npm run dev -- --port "$FRONTEND_PORT") > >(prefix "fe" "$C_FE") 2>&1 &
PIDS="$PIDS $!"

printf '\n'
ok "backend   http://localhost:$PORT"
ok "frontend  http://localhost:$FRONTEND_PORT"
info "Tekan Ctrl-C untuk menghentikan semuanya."
printf '\n'

# bash 3.2 has no `wait -n`, so poll instead. Exiting as soon as any service dies
# keeps a crashed backend from hiding behind a still-running frontend.
while :; do
  for pid in $PIDS; do
    if ! alive "$pid"; then
      warn "Salah satu service berhenti."
      exit 1
    fi
  done
  sleep 1
done
