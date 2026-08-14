#!/usr/bin/env bash
set -euo pipefail

STATE_PATH="${1:-$HOME/Library/Containers/com.vpncheap.macnative/Data/Library/Preferences/com.vpncheap.macnative.plist}"
OUTPUT_PATH="${2:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/easy_proxies-config.yaml}"

die() {
  if [ -f "$OUTPUT_PATH" ]; then
    rm -f -- "$OUTPUT_PATH" 2>/dev/null || true
  fi
  printf '[sync-vpncheap-subscription] ERROR: %s\n' "$*" >&2
  exit 1
}

[ -f "$STATE_PATH" ] || die "VPNCheap state file is not available: $STATE_PATH"

RAW="$(/usr/bin/plutil -extract xboard_subscription raw -o - "$STATE_PATH" 2>/dev/null || true)"
case "$RAW" in
  '{"'*) ;;
  '"{'*) ;;
  *) die "VPNCheap subscription data is missing" ;;
esac

URL="$(printf '%s' "$RAW" | python3 -c '
import json, sys
try:
    state = json.load(sys.stdin)
except Exception:
    sys.exit(1)
try:
    value = state["value"]
    url = value["subscribe_url"]
except Exception:
    sys.exit(2)
if not isinstance(url, str) or not url.strip():
    sys.exit(3)
print(url.strip())
')" || die "VPNCheap subscription URL is missing or not a string"

case "$URL" in
  https://*) ;;
  *) die "VPNCheap subscription URL must use https" ;;
esac

OUT_DIR="$(dirname "$OUTPUT_PATH")"
mkdir -p "$OUT_DIR"
[ -e "$OUTPUT_PATH" ] && [ ! -f "$OUTPUT_PATH" ] && die "output config path is not a file"
umask 077
TMP_OUT="$(mktemp "$OUT_DIR/.easy_proxies-config.XXXXXX")"
trap 'rm -f -- "$TMP_OUT"' EXIT

printf '%s\n' \
  'mode: hybrid' \
  'log_level: info' \
  '' \
  'log:' \
  '  output: file' \
  '  file: easy_proxies.log' \
  '  max_size: 20' \
  '  max_backups: 3' \
  '  max_age: 7' \
  '  compress: true' \
  '' \
  'skip_cert_verify: false' \
  '' \
  'listener:' \
  '  address: "127.0.0.1"' \
  '  port: 2323' \
  '' \
  'multi_port:' \
  '  address: "127.0.0.1"' \
  '  base_port: 24000' \
  '' \
  'pool:' \
  '  mode: latency' \
  '  failure_threshold: 3' \
  '  blacklist_duration: 24h' \
  '  retry_enabled: true' \
  '  retry_attempts: 3' \
  '' \
  'sticky:' \
  '  enabled: false' \
  '  port: 2324' \
  '' \
  'management:' \
  '  listen: "127.0.0.1:9091"' \
  '  probe_target: "http://cp.cloudflare.com/generate_204"' \
  '' \
  'nodes_file: nodes.txt' \
  '' \
  'subscriptions:' \
  "  - \"$URL\"" \
  '' \
  'subscription_refresh:' \
  '  enabled: true' \
  '  interval: 1h' \
  '  timeout: 30s' \
  '  fetch_concurrency: 8' \
  '  health_check_timeout: 60s' \
  '  drain_timeout: 30s' \
  '  min_available_nodes: 1' \
  > "$TMP_OUT"

chmod 600 "$TMP_OUT"
mv -f "$TMP_OUT" "$OUTPUT_PATH"
trap - EXIT
printf '[sync-vpncheap-subscription] VPNCheap subscription resolved and direct config generated\n'
