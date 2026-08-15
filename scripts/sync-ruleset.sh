#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/internal/routing/ruleset_snapshot.json"

fetch() {
  curl -fsSL --max-time 30 "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/$1"
}

parse_suffixes() {
  # v2fly data files use plain domains for suffix rules, `full:` for exact
  # domains, `regexp:` for regular expressions. Keep only suffix-style entries
  # for runtime routing; broaden service coverage later by extending categories.
  sed -E -e '/^#/d' -e '/^include:/d' -e '/^full:/d' -e '/^regexp:/d' -e '/^keyword:/d' -e '/^attribute:/d' \
    | sed -E -e 's/^domain://' -e 's/^[[:space:]]+//' -e '/^$/d'
}

if ! command -v python3 >/dev/null 2>&1; then
  echo "sync-ruleset requires python3" >&2
  exit 1
fi

python3 - "$ROOT" "$OUT" <<'PY'
import json, pathlib, subprocess, sys

root, out = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
categories = [
    ("openai", "OpenAI / ChatGPT"),
    ("google", "Google"),
    ("github", "GitHub"),
    ("microsoft", "Microsoft / Bing / Copilot"),
    ("twitter", "Twitter / X"),
    ("meta", "Meta / Facebook / Instagram"),
    ("netflix", "Netflix"),
    ("telegram", "Telegram"),
    ("cloudflare", "Cloudflare"),
    ("anthropic", "Anthropic / Claude"),
]
data = {"categories": [], "china": {"domain_suffix": [], "ip_cidr": []}}
default_cn_domains = ["163.com", "qq.com", "baidu.com", "taobao.com", "tmall.com", "jd.com", "weibo.com", "bilibili.com", "douyin.com", "zhihu.com", "aliyun.com", "tencent.com", "weixin.qq.com"]
default_cn_ips = ["1.0.1.0/24", "1.116.0.0/15", "101.200.0.0/15", "103.21.244.0/22", "111.13.0.0/16", "114.114.114.114/32"]
if out.exists():
    try:
        existing = json.loads(out.read_text())
        data["china"] = existing.get("china", data["china"])
    except Exception:
        pass

def fetch(name):
    cp = subprocess.run(["curl", "-fsSL", "--max-time", "30", f"https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/{name}"], capture_output=True, text=True)
    if cp.returncode != 0:
        return []
    lines = []
    for line in cp.stdout.splitlines():
        if line.startswith(("#", "include:", "full:", "regexp:", "keyword:")):
            continue
        if ":" in line and not line.startswith(("domain:", "full:")):
            continue
        item = line
        if item.startswith("domain:"):
            item = item[len("domain:"):]
        item = item.strip()
        if item and not item.startswith("@"):
            lines.append(item)
    return lines

for cid, name in categories:
    domains = fetch(cid)
    if domains:
        data["categories"].append({"id": cid, "name": name, "domain_suffix": sorted(set(domains))})

cn = fetch("cn")
data["china"]["domain_suffix"] = sorted(set(cn)) if cn else (data["china"]["domain_suffix"] or default_cn_domains)
data["china"]["ip_cidr"] = data["china"]["ip_cidr"] or default_cn_ips
out.write_text(json.dumps(data, ensure_ascii=False, indent=2))
print(f"wrote {out}")
PY
