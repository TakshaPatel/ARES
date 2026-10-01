#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

BUILD=1
ARGS=()
for a in "$@"; do
  if [ "$a" = "--no-build" ]; then
    BUILD=0
  else
    ARGS+=("$a")
  fi
done

command -v go >/dev/null 2>&1 || { echo "error: go is required" >&2; exit 1; }

if [ "$BUILD" -eq 1 ]; then
  command -v npm >/dev/null 2>&1 || {
    echo "error: npm is required to build the frontend (or pass --no-build if frontend/dist already exists)" >&2
    exit 1
  }
  if [ ! -d frontend/node_modules ]; then
    echo ">> installing frontend dependencies"
    (cd frontend && npm install)
  fi
  echo ">> building frontend"
  (cd frontend && npm run build)
fi

if [ ! -d frontend/dist ]; then
  echo "error: frontend/dist is missing; run without --no-build" >&2
  exit 1
fi

ADDR="${ARES_ADDR:-:8080}"
echo ">> starting ARES on http://localhost:${ADDR##*:}  (Ctrl-C to stop)"
exec go run . -frontend ${ARGS[@]+"${ARGS[@]}"}
