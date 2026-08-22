#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUNTIME="$ROOT/runtime"
EP_CFG="${EASY_PROXIES_CONFIG:-$RUNTIME/easy_proxies-config.yaml}"
EP_BIN="${EASY_PROXIES_BIN:-$RUNTIME/easy_proxies}"
EP_PID="${EASY_PROXIES_PID:-$RUNTIME/easy_proxies.pid}"
EP_LOG="${EASY_PROXIES_LOG:-$RUNTIME/easy_proxies.log}"
EP_ERR_LOG="${EASY_PROXIES_ERR_LOG:-$RUNTIME/easy_proxies.err.log}"
RESOLVER="${VPNCHEAP_RESOLVER:-$RUNTIME/sync-vpncheap-subscription.sh}"
STATE_PATH="${VPNCHEAP_STATE_PATH:-$HOME/Library/Containers/com.vpncheap.macnative/Data/Library/Preferences/com.vpncheap.macnative.plist}"
WEBUI_PORT="${WEBUI_PORT:-9091}"
LISTEN_PORT="${LISTEN_PORT:-2323}"
SKIP_READY="${SKIP_READY:-0}"

log() { printf '[proxy-chain-macos] %s\n' "$*"; }
die() { log "ERROR: $*" >&2; exit 1; }

pid_alive() {
  [ -f "$1" ] || return 1
  local pid
  pid="$(cat "$1")"
  [ -n "$pid" ] && [ "$pid" -gt 0 ] 2>/dev/null && kill -0 "$pid" 2>/dev/null
}

easy_ready() {
  [ "$SKIP_READY" = "1" ] && return 0
  [ "$WEBUI_PORT" = "0" ] && return 0
  curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$WEBUI_PORT/" 2>/dev/null
}

start_easy_proxies() {
  if pid_alive "$EP_PID"; then
    log "easy_proxies already running"
    return 0
  fi
  [ -x "$EP_BIN" ] || die "easy_proxies binary missing: $EP_BIN"
  [ -f "$EP_CFG" ] || die "easy_proxies config missing: $EP_CFG"

  log "starting easy_proxies"
  nohup "$EP_BIN" -config "$EP_CFG" >>"$EP_LOG" 2>>"$EP_ERR_LOG" &
  local child_pid=$!
  disown || true
  echo "$child_pid" >"$EP_PID"

  local i
  for i in $(seq 1 20); do
    if easy_ready && pid_alive "$EP_PID"; then
      log "easy_proxies ready on $WEBUI_PORT"
      return 0
    fi
    if ! pid_alive "$EP_PID"; then
      break
    fi
    sleep 2
  done

  if kill -0 "$child_pid" 2>/dev/null; then
    kill "$child_pid" 2>/dev/null || true
    sleep 1
  fi
  rm -f -- "$EP_PID"
  die "easy_proxies did not become ready; tail $EP_ERR_LOG"
}

stop_easy_proxies() {
  if pid_alive "$EP_PID"; then
    local pid
    pid="$(cat "$EP_PID")"
    kill "$pid" 2>/dev/null || true
    log "waiting for easy_proxies to stop"
    local i
    for i in $(seq 1 30); do
      if ! kill -0 "$pid" 2>/dev/null; then
        rm -f -- "$EP_PID"
        log "stopped easy_proxies"
        return 0
      fi
      sleep 1
    done
    if kill -0 "$pid" 2>/dev/null; then
      log "easy_proxies did not stop within 30s"
      return 1
    fi
    rm -f -- "$EP_PID"
  else
    rm -f -- "$EP_PID"
  fi
}

start_direct() {
  stop_easy_proxies
  [ -x "$RESOLVER" ] || die "subscription resolver missing: $RESOLVER"
  if ! "$RESOLVER" "$STATE_PATH" "$EP_CFG"; then
    rm -f -- "$EP_CFG"
    die "failed to regenerate VPNCheap direct config"
  fi
  start_easy_proxies
  log "direct mode: proxypool stopped, port 18080 not used"
  log "9router export http://127.0.0.1:$WEBUI_PORT/api/export?target=9router"
}

case "${1:-status}" in
  start)
    start_direct
    log "HTTP  http://127.0.0.1:$LISTEN_PORT"
    log "SOCKS socks5://127.0.0.1:$LISTEN_PORT"
    ;;
  stop)
    stop_easy_proxies
    ;;
  restart)
    "$0" stop
    "$0" start
    ;;
  status)
    if pid_alive "$EP_PID" || easy_ready; then
      log "running (easy_proxies_pid=$(cat "$EP_PID" 2>/dev/null || echo none))"
    else
      log "stopped"
    fi
    ;;
  *)
    die "usage: $0 start|stop|restart|status"
    ;;
esac
