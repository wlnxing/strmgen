#!/bin/sh
set -eu

: "${STRM_LISTEN_ADDR:=127.0.0.1:18080}"
export STRM_LISTEN_ADDR

/app/strm &
api_pid="$!"

caddy run --config /etc/caddy/Caddyfile --adapter caddyfile &
caddy_pid="$!"

term() {
  kill "$api_pid" "$caddy_pid" 2>/dev/null || true
  wait "$api_pid" 2>/dev/null || true
  wait "$caddy_pid" 2>/dev/null || true
}
trap term INT TERM

while true; do
  if ! kill -0 "$api_pid" 2>/dev/null; then
    wait "$api_pid"
    exit $?
  fi
  if ! kill -0 "$caddy_pid" 2>/dev/null; then
    wait "$caddy_pid"
    exit $?
  fi
  sleep 1
done
