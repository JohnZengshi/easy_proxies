#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUNTIME="$ROOT/runtime"
PP_CFG="$RUNTIME/proxypool-config.yaml"
EP_CFG="$RUNTIME/config.yaml"
PP_BIN="$RUNTIME/proxypool"
EP_BIN="$RUNTIME/easy_proxies"
PP_PID="$RUNTIME/proxypool.pid"
EP_PID="$RUNTIME/easy_proxies.pid"
NODES="$RUNTIME/nodes.txt"
PP_LOG="$RUNTIME/proxypool.log"
EP_LOG="$RUNTIME/easy_proxies.log"
PP_STATUS_PORT=18080
EP_WEBUI=9091
EP_LISTEN=2323

log() { printf '[proxy-chain] %s\n' "$*"; }
die() { log "ERROR: $*" >&2; exit 1; }

pid_alive() {
  [ -f "$1" ] || return 1
  local pid
  pid="$(cat "$1")"
  [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

status_healthy() {
  local code
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "http://127.0.0.1:$PP_STATUS_PORT/healthz" 2>/dev/null || true)"
  [ "$code" = "200" ]
}

has_healthy_vpncheap() {
  curl -s --max-time 5 "http://127.0.0.1:$PP_STATUS_PORT/status" 2>/dev/null | python3 -c '
import sys, json
try:
    s = json.load(sys.stdin)
except Exception:
    sys.exit(1)
sys.exit(0 if any(n.get("tag") == "vpncheap" and n.get("healthy") for n in s) else 1)
'
}

start_proxypool() {
  if status_healthy && has_healthy_vpncheap; then
    log "reusing healthy proxypool on $PP_STATUS_PORT"
    return 0
  fi
  [ -x "$PP_BIN" ] || die "proxypool binary missing: $PP_BIN"
  log "starting proxypool"
  nohup "$PP_BIN" -config "$PP_CFG" >> "$PP_LOG" 2>&1 &
  echo $! > "$PP_PID"
  local i
  for i in $(seq 1 30); do
    if status_healthy; then
      log "proxypool healthy after ${i}x2s"
      return 0
    fi
    sleep 2
  done
  die "proxypool did not become healthy; tail $PP_LOG"
}

generate_nodes() {
  log "generating $NODES"
  curl -s --max-time 10 "http://127.0.0.1:$PP_STATUS_PORT/status" | python3 -c '
import sys, json
s = json.load(sys.stdin)
for n in sorted(s, key=lambda x: x.get("port", 0)):
    if n.get("healthy"):
        print("http://127.0.0.1:%s#vpncheap-%s" % (n["port"], n["port"]))
' > "$NODES"
  if [ ! -s "$NODES" ]; then
    die "no healthy VPNCheap nodes in $PP_STATUS_PORT/status"
  fi
  log "$(wc -l < "$NODES") healthy nodes"
}

start_easy_proxies() {
  if pid_alive "$EP_PID"; then
    log "easy_proxies already running"
    return 0
  fi
  [ -x "$EP_BIN" ] || die "easy_proxies binary missing: $EP_BIN"
  log "starting easy_proxies"
  nohup "$EP_BIN" -config "$EP_CFG" >> "$EP_LOG" 2>&1 &
  echo $! > "$EP_PID"
  local i
  for i in $(seq 1 20); do
    if curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$EP_WEBUI/"; then
      log "easy_proxies ready on $EP_WEBUI"
      return 0
    fi
    sleep 2
  done
  die "easy_proxies did not become ready; tail $EP_LOG"
}

stop_easy_proxies() {
  if pid_alive "$EP_PID"; then
    kill "$(cat "$EP_PID")" 2>/dev/null || true
    sleep 1
    log "stopped easy_proxies"
  fi
}

stop_proxypool() {
  # Only stop a proxypool we started; never kill a pre-existing healthy instance.
  if pid_alive "$PP_PID"; then
    kill "$(cat "$PP_PID")" 2>/dev/null || true
    sleep 1
    log "stopped proxypool"
  fi
}

case "${1:-status}" in
  start)
    start_proxypool
    generate_nodes
    start_easy_proxies
    log "HTTP  http://127.0.0.1:$EP_LISTEN"
    log "SOCKS socks5://127.0.0.1:$EP_LISTEN"
    ;;
  stop)
    stop_easy_proxies
    stop_proxypool
    ;;
  restart)
    "$0" stop
    "$0" start
    ;;
  status)
    if pid_alive "$EP_PID" || status_healthy; then
      log "running (proxypool health=$PP_STATUS_PORT easy_proxies_pid=$(cat "$EP_PID" 2>/dev/null || echo none))"
    else
      log "stopped"
    fi
    ;;
  *)
    die "usage: $0 start|stop|restart|status"
    ;;
esac
