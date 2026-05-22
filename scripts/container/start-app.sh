#!/bin/sh
set -eu

/app/bin/server &
backend_pid="$!"

(
  cd /app/frontend
  node server.js
) &
frontend_pid="$!"

terminate() {
  kill "$frontend_pid" 2>/dev/null || true
  kill "$backend_pid" 2>/dev/null || true
  wait "$frontend_pid" 2>/dev/null || true
  wait "$backend_pid" 2>/dev/null || true
}

trap terminate INT TERM

while true; do
  if ! kill -0 "$backend_pid" 2>/dev/null; then
    wait "$backend_pid" || true
    kill "$frontend_pid" 2>/dev/null || true
    wait "$frontend_pid" 2>/dev/null || true
    exit 1
  fi

  if ! kill -0 "$frontend_pid" 2>/dev/null; then
    wait "$frontend_pid" || true
    kill "$backend_pid" 2>/dev/null || true
    wait "$backend_pid" 2>/dev/null || true
    exit 1
  fi

  sleep 2
done
