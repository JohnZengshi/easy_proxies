#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESOLVER="$ROOT/runtime/sync-vpncheap-subscription.sh"
TEMP="$(mktemp -d "${TMPDIR:-/tmp}/vpncheap-resolver-test.XXXXXX")"
FAILED=0

cleanup() {
  rm -rf "$TEMP"
}
trap cleanup EXIT

make_plist() {
  local path="$1"
  local json="$2"
  python3 - "$path" "$json" <<'PY'
import json, plistlib, sys
path, raw = sys.argv[1], sys.argv[2]
with open(path, "wb") as f:
    plistlib.dump(json.loads(raw), f)
PY
}

run_resolver() {
  local state="$1"
  local output="$2"
  local log="$TEMP/run.log"
  set +e
  "$RESOLVER" "$state" "$output" >"$log" 2>&1
  local code=$?
  set -e
  printf '%s' "$code" >"$TEMP/last-code"
  cat "$log" >"$TEMP/last-log"
}

expect_happy() {
  local output="$TEMP/happy/easy_proxies-config.yaml"
  [ -f "$output" ] || { echo "FAIL: happy output missing"; FAILED=1; return; }
  [ "$(stat -f '%Lp' "$output")" = "600" ] || { echo "FAIL: output mode is not 600"; FAILED=1; return; }
  rg -q '^mode: hybrid$' "$output" || { echo "FAIL: hybrid mode missing"; FAILED=1; }
  rg -q 'address: "127\.0\.0\.1"' "$output" || { echo "FAIL: loopback bind missing"; FAILED=1; }
  rg -q 'port: 2323' "$output" || { echo "FAIL: listener port missing"; FAILED=1; }
  rg -q 'base_port: 24000' "$output" || { echo "FAIL: multi-port base missing"; FAILED=1; }
  rg -q 'listen: "127\.0\.0\.1:9091"' "$output" || { echo "FAIL: management port missing"; FAILED=1; }
  rg -q 'skip_cert_verify: false' "$output" || { echo "FAIL: cert verification disabled"; FAILED=1; }
  rg -q "https://example\.test/subscribe\?token=fixture-secret" "$output" || { echo "FAIL: subscription URL missing"; FAILED=1; }
  if rg -q '^routing:' "$output"; then
    echo "FAIL: fresh config unexpectedly contains routing block"
    FAILED=1
  fi
  if rg -q 'system_proxy_enabled' "$output"; then
    echo "FAIL: fresh config unexpectedly contains system_proxy_enabled"
    FAILED=1
  fi
  if rg -q 'fixture-secret' "$TEMP/last-log"; then
    echo "FAIL: resolver output leaked subscription secret"
    FAILED=1
  fi
}

expect_failure() {
  local output="$1"
  if [ "$(cat "$TEMP/last-code")" = "0" ]; then
    echo "FAIL: resolver unexpectedly succeeded"
    FAILED=1
  fi
  if [ -e "$output" ]; then
    echo "FAIL: stale output remained"
    FAILED=1
  fi
  if rg -q 'fixture-secret|example\.test' "$TEMP/last-log"; then
    echo "FAIL: failure output leaked URL or secret"
    FAILED=1
  fi
}

mkdir -p "$TEMP/happy"
make_plist "$TEMP/happy/app_state.plist" '{"xboard_subscription":"{\"value\":{\"subscribe_url\":\"https://example.test/subscribe?token=fixture-secret\"}}"}'
run_resolver "$TEMP/happy/app_state.plist" "$TEMP/happy/easy_proxies-config.yaml"
expect_happy

mkdir -p "$TEMP/missing_state"
run_resolver "$TEMP/missing_state/app_state.plist" "$TEMP/missing_state/output.yaml"
expect_failure "$TEMP/missing_state/output.yaml"
rg -q 'state file is not available' "$TEMP/last-log" || { echo "FAIL: missing state message wrong"; FAILED=1; }

mkdir -p "$TEMP/missing_key"
make_plist "$TEMP/missing_key/app_state.plist" '{"other":"value"}'
printf stale >"$TEMP/missing_key/output.yaml"
run_resolver "$TEMP/missing_key/app_state.plist" "$TEMP/missing_key/output.yaml"
expect_failure "$TEMP/missing_key/output.yaml"
rg -q 'subscription data is missing' "$TEMP/last-log" || { echo "FAIL: missing key message wrong"; FAILED=1; }

mkdir -p "$TEMP/malformed"
printf '{ broken' >"$TEMP/malformed/app_state.plist"
printf stale >"$TEMP/malformed/output.yaml"
run_resolver "$TEMP/malformed/app_state.plist" "$TEMP/malformed/output.yaml"
expect_failure "$TEMP/malformed/output.yaml"
rg -q 'subscription data is missing' "$TEMP/last-log" || { echo "FAIL: malformed message wrong"; FAILED=1; }

mkdir -p "$TEMP/missing_url"
make_plist "$TEMP/missing_url/app_state.plist" '{"xboard_subscription":"{\"value\":{\"other\":\"x\"}}"}'
printf stale >"$TEMP/missing_url/output.yaml"
run_resolver "$TEMP/missing_url/app_state.plist" "$TEMP/missing_url/output.yaml"
expect_failure "$TEMP/missing_url/output.yaml"
rg -q 'missing or not a string' "$TEMP/last-log" || { echo "FAIL: missing URL message wrong"; FAILED=1; }

mkdir -p "$TEMP/non_string_url"
make_plist "$TEMP/non_string_url/app_state.plist" '{"xboard_subscription":"{\"value\":{\"subscribe_url\":123}}"}'
printf stale >"$TEMP/non_string_url/output.yaml"
run_resolver "$TEMP/non_string_url/app_state.plist" "$TEMP/non_string_url/output.yaml"
expect_failure "$TEMP/non_string_url/output.yaml"
rg -q 'missing or not a string' "$TEMP/last-log" || { echo "FAIL: non-string URL message wrong"; FAILED=1; }

mkdir -p "$TEMP/http_url"
make_plist "$TEMP/http_url/app_state.plist" '{"xboard_subscription":"{\"value\":{\"subscribe_url\":\"http://example.test/sub\"}}"}'
printf stale >"$TEMP/http_url/output.yaml"
run_resolver "$TEMP/http_url/app_state.plist" "$TEMP/http_url/output.yaml"
expect_failure "$TEMP/http_url/output.yaml"
rg -q 'must use https' "$TEMP/last-log" || { echo "FAIL: http URL message wrong"; FAILED=1; }

mkdir -p "$TEMP/output_dir"
make_plist "$TEMP/output_dir/app_state.plist" '{"xboard_subscription":"{\"value\":{\"subscribe_url\":\"https://example.test/sub\"}}"}'
run_resolver "$TEMP/output_dir/app_state.plist" "$TEMP/output_dir"
[ "$(cat "$TEMP/last-code")" = "0" ] && { echo "FAIL: directory output unexpectedly succeeded"; FAILED=1; }

mkdir -p "$TEMP/preserve_routing"
cp "$TEMP/happy/easy_proxies-config.yaml" "$TEMP/preserve_routing/easy_proxies-config.yaml"
printf '\nrouting:\n  china_direct_enabled: true\n  rules:\n    - name: openai\n      domain_suffix:\n        - openai.com\n      target: proxy-pool\n' >> "$TEMP/preserve_routing/easy_proxies-config.yaml"
make_plist "$TEMP/preserve_routing/app_state.plist" '{"xboard_subscription":"{\"value\":{\"subscribe_url\":\"https://example.test/subscribe?token=preserve-secret\"}}"}'
run_resolver "$TEMP/preserve_routing/app_state.plist" "$TEMP/preserve_routing/easy_proxies-config.yaml"
[ "$(cat "$TEMP/last-code")" = "0" ] || { echo "FAIL: routing preserve resolver failed"; FAILED=1; }
rg -q '^routing:' "$TEMP/preserve_routing/easy_proxies-config.yaml" || { echo "FAIL: routing block lost after regeneration"; FAILED=1; }
rg -q 'openai.com' "$TEMP/preserve_routing/easy_proxies-config.yaml" || { echo "FAIL: routing rule content lost after regeneration"; FAILED=1; }
rg -q 'preserve-secret' "$TEMP/last-log" && { echo "FAIL: preserve fixture leaked secret"; FAILED=1; }

mkdir -p "$TEMP/preserve_system_proxy"
cat >"$TEMP/preserve_system_proxy/easy_proxies-config.yaml" <<'EOF'
mode: hybrid
management:
  listen: "127.0.0.1:9091"
  probe_target: "http://cp.cloudflare.com/generate_204"
  system_proxy_enabled: true
nodes_file: nodes.txt
routing:
  fallback: proxy-pool
EOF
make_plist "$TEMP/preserve_system_proxy/app_state.plist" '{"xboard_subscription":"{\"value\":{\"subscribe_url\":\"https://example.test/subscribe?token=proxy-secret\"}}"}'
run_resolver "$TEMP/preserve_system_proxy/app_state.plist" "$TEMP/preserve_system_proxy/easy_proxies-config.yaml"
[ "$(cat "$TEMP/last-code")" = "0" ] || { echo "FAIL: system proxy preserve resolver failed"; FAILED=1; }
rg -q 'system_proxy_enabled: true' "$TEMP/preserve_system_proxy/easy_proxies-config.yaml" || { echo "FAIL: system_proxy_enabled lost after regeneration"; FAILED=1; }
rg -q '^  system_proxy_enabled: true$' "$TEMP/preserve_system_proxy/easy_proxies-config.yaml" || { echo "FAIL: system_proxy_enabled has wrong indentation"; FAILED=1; }
rg -q 'proxy-secret' "$TEMP/last-log" && { echo "FAIL: system proxy fixture leaked secret"; FAILED=1; }

if [ "$FAILED" = "0" ]; then
  echo "sync-vpncheap-subscription tests passed"
else
  echo "sync-vpncheap-subscription tests failed"
  exit 1
fi
