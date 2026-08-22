#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/runtime/proxy-chain-macos.sh"
TEMP="$(mktemp -d "${TMPDIR:-/tmp}/proxy-chain-macos-test.XXXXXX")"
FAILED=0

cleanup() {
  if [ -f "$TEMP/easy_proxies.pid" ]; then
    local pid
    pid="$(cat "$TEMP/easy_proxies.pid")"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
  fi
  rm -rf "$TEMP"
}
trap cleanup EXIT

mkdir -p "$TEMP/bin"

cat >"$TEMP/bin/fake-easy" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$FAKE_EASY_ARGS"
exec python3 - <<'PY'
import os
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Length", "2")
        self.end_headers()
        self.wfile.write(b"ok")
    def log_message(self, *args):
        pass
HTTPServer(("127.0.0.1", int(os.environ["FAKE_EASY_PORT"])), H).serve_forever()
PY
SH
chmod +x "$TEMP/bin/fake-easy"

cat >"$TEMP/bin/fake-resolver" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
[ -f "$1" ] || exit 1
umask 077
printf 'mode: hybrid\nlistener:\n  address: "127.0.0.1"\n  port: 2323\n' >"$2"
SH
chmod +x "$TEMP/bin/fake-resolver"

cat >"$TEMP/bin/fail-resolver" <<'SH'
#!/usr/bin/env bash
exit 1
SH
chmod +x "$TEMP/bin/fail-resolver"

cat >"$TEMP/state.plist" <<'SH'
fixture
SH

web_port="$(python3 - <<'PY'
import socket
s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()
PY
)"

invoke() {
  local name="$1"
  local out="$TEMP/$name.out"
  local err="$TEMP/$name.err"
  set +e
  EASY_PROXIES_BIN="$TEMP/bin/fake-easy" \
  EASY_PROXIES_CONFIG="$TEMP/easy_proxies-config.yaml" \
  EASY_PROXIES_PID="$TEMP/easy_proxies.pid" \
  EASY_PROXIES_LOG="$TEMP/easy_proxies.log" \
  EASY_PROXIES_ERR_LOG="$TEMP/easy_proxies.err.log" \
  FAKE_EASY_ARGS="$TEMP/fake-easy-args" \
  FAKE_EASY_PORT="$web_port" \
  VPNCHEAP_RESOLVER="$RESOLVER" \
  VPNCHEAP_STATE_PATH="$TEMP/state.plist" \
  WEBUI_PORT="$web_port" \
  SKIP_READY=0 \
  "$SCRIPT" "$name" >"$out" 2>"$err"
  local code=$?
  set -e
  printf '%s' "$code" >"$TEMP/last-code"
  cp "$out" "$TEMP/last-out"
  cp "$err" "$TEMP/last-err"
}

RESOLVER="$TEMP/bin/fake-resolver"
invoke start
[ "$(cat "$TEMP/last-code")" = "0" ] || { echo "FAIL: start exited $(cat "$TEMP/last-code")"; FAILED=1; }
if [ "$(cat "$TEMP/last-code")" != "0" ]; then cat "$TEMP/last-out" "$TEMP/last-err"; fi
[ -f "$TEMP/easy_proxies-config.yaml" ] || { echo "FAIL: direct config missing"; FAILED=1; }
[ -f "$TEMP/easy_proxies.pid" ] || { echo "FAIL: pid file missing"; FAILED=1; }
[ -f "$TEMP/fake-easy-args" ] || { echo "FAIL: easy_proxies was not started"; FAILED=1; }
rg -q -- '-config' "$TEMP/fake-easy-args" || { echo "FAIL: fake easy args missing config"; FAILED=1; }
rg -q -- 'easy_proxies-config.yaml' "$TEMP/fake-easy-args" || { echo "FAIL: fake easy got wrong config"; FAILED=1; }
rg -q 'direct mode: proxypool stopped, port 18080 not used' "$TEMP/last-out" || { echo "FAIL: direct mode message missing"; FAILED=1; }
rg -q '9router export http://127\.0\.0\.1:'"$web_port"'/api/export\?target=9router' "$TEMP/last-out" || { echo "FAIL: 9router export message missing"; FAILED=1; }
rg -q 'fixture-secret|example\.test' "$TEMP/last-out" "$TEMP/last-err" && { echo "FAIL: output leaked fixture secret"; FAILED=1; }

old_pid="$(cat "$TEMP/easy_proxies.pid")"
invoke status
[ "$(cat "$TEMP/last-code")" = "0" ] || { echo "FAIL: status failed"; FAILED=1; }
rg -q 'running \(easy_proxies_pid=' "$TEMP/last-out" || { echo "FAIL: status did not report running"; FAILED=1; }

invoke stop
[ "$(cat "$TEMP/last-code")" = "0" ] || { echo "FAIL: stop failed"; FAILED=1; }
for i in $(seq 1 20); do
  if ! kill -0 "$old_pid" 2>/dev/null; then break; fi
  sleep 0.1
done
kill -0 "$old_pid" 2>/dev/null && { echo "FAIL: easy_proxies still running"; FAILED=1; }
[ ! -f "$TEMP/easy_proxies.pid" ] || { echo "FAIL: stale pid file remained"; FAILED=1; }

invoke status
rg -q 'stopped' "$TEMP/last-out" || { echo "FAIL: status after stop not stopped"; FAILED=1; }

invoke restart
[ "$(cat "$TEMP/last-code")" = "0" ] || { echo "FAIL: restart failed"; FAILED=1; }
invoke stop

cat >"$TEMP/bin/slow-stop-easy" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$FAKE_EASY_ARGS"
python3 - <<'PY' &
import os
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Length", "2")
        self.end_headers()
        self.wfile.write(b"ok")
    def log_message(self, *args):
        pass
HTTPServer(("127.0.0.1", int(os.environ["FAKE_EASY_PORT"])), H).serve_forever()
PY
child=$!
trap 'sleep 2; kill "$child" 2>/dev/null || true; exit 0' TERM
wait "$child"
SH
chmod +x "$TEMP/bin/slow-stop-easy"

rm -f "$TEMP/easy_proxies.pid" "$TEMP/fake-easy-args"
EASY_PROXIES_BIN="$TEMP/bin/slow-stop-easy" \
EASY_PROXIES_CONFIG="$TEMP/easy_proxies-config.yaml" \
EASY_PROXIES_PID="$TEMP/easy_proxies.pid" \
EASY_PROXIES_LOG="$TEMP/easy_proxies.log" \
EASY_PROXIES_ERR_LOG="$TEMP/easy_proxies.err.log" \
FAKE_EASY_ARGS="$TEMP/fake-easy-args" \
FAKE_EASY_PORT="$web_port" \
VPNCHEAP_RESOLVER="$TEMP/bin/fake-resolver" \
VPNCHEAP_STATE_PATH="$TEMP/state.plist" \
WEBUI_PORT="$web_port" \
SKIP_READY=0 \
"$SCRIPT" start >"$TEMP/slow-stop-start.out" 2>&1
slow_stop_pid="$(cat "$TEMP/easy_proxies.pid")"
EASY_PROXIES_BIN="$TEMP/bin/slow-stop-easy" \
EASY_PROXIES_CONFIG="$TEMP/easy_proxies-config.yaml" \
EASY_PROXIES_PID="$TEMP/easy_proxies.pid" \
EASY_PROXIES_LOG="$TEMP/easy_proxies.log" \
EASY_PROXIES_ERR_LOG="$TEMP/easy_proxies.err.log" \
FAKE_EASY_ARGS="$TEMP/fake-easy-args" \
FAKE_EASY_PORT="$web_port" \
VPNCHEAP_RESOLVER="$TEMP/bin/fake-resolver" \
VPNCHEAP_STATE_PATH="$TEMP/state.plist" \
WEBUI_PORT="$web_port" \
SKIP_READY=0 \
"$SCRIPT" stop >"$TEMP/slow-stop-stop.out" 2>&1
kill -0 "$slow_stop_pid" 2>/dev/null && { echo "FAIL: slow-stop easy_proxies still running after stop"; FAILED=1; }
[ ! -f "$TEMP/easy_proxies.pid" ] || { echo "FAIL: stale pid file remained after slow stop"; FAILED=1; }

rm -f "$TEMP/easy_proxies-config.yaml" "$TEMP/fake-easy-args"
RESOLVER="$TEMP/bin/fail-resolver"
invoke start
[ "$(cat "$TEMP/last-code")" != "0" ] || { echo "FAIL: failing resolver unexpectedly succeeded"; FAILED=1; }
[ ! -f "$TEMP/easy_proxies-config.yaml" ] || { echo "FAIL: stale config remained after resolver failure"; FAILED=1; }
[ ! -f "$TEMP/fake-easy-args" ] || { echo "FAIL: easy_proxies started after resolver failure"; FAILED=1; }

rm -f "$TEMP/easy_proxies.pid"
RESOLVER="$TEMP/bin/fake-resolver"
cat >"$TEMP/bin/slow-easy" <<'SH'
#!/usr/bin/env bash
exec sleep 300
SH
chmod +x "$TEMP/bin/slow-easy"
EASY_PROXIES_BIN="$TEMP/bin/slow-easy" \
EASY_PROXIES_CONFIG="$TEMP/easy_proxies-config.yaml" \
EASY_PROXIES_PID="$TEMP/easy_proxies.pid" \
EASY_PROXIES_LOG="$TEMP/easy_proxies.log" \
EASY_PROXIES_ERR_LOG="$TEMP/easy_proxies.err.log" \
VPNCHEAP_RESOLVER="$TEMP/bin/fake-resolver" \
VPNCHEAP_STATE_PATH="$TEMP/state.plist" \
WEBUI_PORT=0 \
SKIP_READY=1 \
"$SCRIPT" start >"$TEMP/slow.out" 2>&1
slow_pid="$(cat "$TEMP/easy_proxies.pid")"
EASY_PROXIES_PID="$TEMP/easy_proxies.pid" "$SCRIPT" stop >/dev/null 2>&1
for i in $(seq 1 20); do
  if ! kill -0 "$slow_pid" 2>/dev/null; then break; fi
  sleep 0.1
done
kill -0 "$slow_pid" 2>/dev/null && { echo "FAIL: slow easy_proxies not stopped"; FAILED=1; }

if [ "$FAILED" = "0" ]; then
  echo "proxy-chain-macos tests passed"
else
  echo "proxy-chain-macos tests failed"
  exit 1
fi
