#!/usr/bin/env bash
# Fetches the manufacturer hero images the gear-image eval runs on.
#
# Those images are not committed (#3228): they belong to the manufacturers, so
# server/test_data/manual/ is gitignored and each golden case records its
# product page in `source_url`. Run this once before the image-mode eval;
# cases whose image is missing are skipped, not failed. The page may have
# changed since the case was written, so a re-fetched image can differ.
#
# Why this is a separate script: the cowork browser pipeline that
# generated the cases hit two recurring blockers when trying to
# download images itself:
#   1. CDNs that 404 the asset URL when accessed without a Referer
#      header from the parent product page (assets.dewalt.com, etc).
#   2. CORS that blocks page-context fetch() of cross-origin images.
# A server-side curl with the brand domain as the Referer side-steps
# both and is also much faster.
#
# Usage:
#   npm run eval:fetch-images
#   bash scripts/fetch_manual_case_images.sh [path/to/cases.json]
#
# Default cases path: server/ai/eval/testdata/gear_image_goldens.json. Also
# accepts a bare array of cases, as eval-seed-from-urls writes them.
# Output dir: server/test_data/manual/
#
# Per case the script:
#   1. Loads the source_url to read its application/ld+json Product
#      block (or og:image as a fallback) and pull the hero URL.
#   2. Curls the hero URL with the source_url as the Referer.
#   3. Writes to server/test_data/manual/<id>.<ext>, where <ext>
#      matches the response Content-Type (jpg/png/webp). If the
#      cases JSON already lists an image_file extension, that wins.
#
# Bring your own jq (`brew install jq`).

set -euo pipefail

CASES="${1:-server/ai/eval/testdata/gear_image_goldens.json}"
OUT_DIR="server/test_data/manual"

if [[ ! -f "$CASES" ]]; then
  echo "cases file not found: $CASES" >&2
  exit 1
fi
mkdir -p "$OUT_DIR"

UA="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36"

count=0
ok=0
fail=0

while IFS=$'\t' read -r ID SRC IMAGE_FILE; do
  count=$((count + 1))
  out="$OUT_DIR/$(basename "$IMAGE_FILE")"

  if [[ -s "$out" ]]; then
    echo "[skip] $ID — already present at $out"
    ok=$((ok + 1))
    continue
  fi

  # 1. Fetch product page, extract hero image URL.
  page=$(curl -sL --max-time 25 -A "$UA" "$SRC" || true)
  if [[ -z "$page" ]]; then
    echo "[fail] $ID — could not fetch product page"
    fail=$((fail + 1))
    continue
  fi

  hero=$(printf '%s' "$page" \
    | python3 -c '
import sys, re, json
html = sys.stdin.read()
hero = None
# Try Schema.org Product JSON-LD blocks first.
for m in re.finditer(r"<script[^>]*application/ld\+json[^>]*>(.*?)</script>", html, re.DOTALL | re.IGNORECASE):
    try:
        data = json.loads(m.group(1).strip())
    except Exception:
        continue
    nodes = []
    def walk(o):
        if isinstance(o, list):
            for x in o: walk(x)
        elif isinstance(o, dict):
            nodes.append(o)
            for k in ("@graph", "hasVariant", "item"):
                if k in o: walk(o[k])
    walk(data)
    for n in nodes:
        t = n.get("@type")
        if isinstance(t, list): t = next((x for x in t), None)
        if not (isinstance(t, str) and t.lower() in ("product", "productgroup")): continue
        img = n.get("image")
        if isinstance(img, list) and img: img = img[0]
        if isinstance(img, dict): img = img.get("url")
        if isinstance(img, str):
            hero = img
            break
    if hero: break
# Fall back to og:image.
if not hero:
    m = re.search(r"<meta[^>]+property=[\"\047]og:image[\"\047][^>]+content=[\"\047]([^\"\047]+)", html, re.IGNORECASE)
    if m: hero = m.group(1)
print(hero or "")
') || true

  if [[ -z "$hero" ]]; then
    echo "[fail] $ID — no hero image URL on page"
    fail=$((fail + 1))
    continue
  fi

  # 2. Resolve relative URLs.
  case "$hero" in
    http*) ;;
    //*) hero="https:$hero" ;;
    /*)
      origin=$(printf '%s' "$SRC" | sed -E 's#^(https?://[^/]+).*#\1#')
      hero="$origin$hero"
      ;;
    *)
      base=$(printf '%s' "$SRC" | sed -E 's#(.*/).*#\1#')
      hero="$base$hero"
      ;;
  esac

  # 3. Download with Referer of the product page.
  if curl -sL --max-time 30 -A "$UA" -e "$SRC" -o "$out" "$hero"; then
    if [[ -s "$out" ]]; then
      size=$(wc -c < "$out" | tr -d ' ')
      if (( size < 2000 )); then
        echo "[fail] $ID — response too small (${size}B), likely an error page"
        rm -f "$out"
        fail=$((fail + 1))
        continue
      fi
      echo "[ok]   $ID — $hero ($size bytes)"
      ok=$((ok + 1))
    else
      echo "[fail] $ID — empty response"
      rm -f "$out"
      fail=$((fail + 1))
    fi
  else
    echo "[fail] $ID — curl exited non-zero"
    fail=$((fail + 1))
  fi

  sleep 0.5
done < <(jq -r '(if type == "array" then . else .cases end)[]
  | select(.source_url != null and (.image_file | startswith("manual/")))
  | [.id, .source_url, .image_file] | @tsv' "$CASES")

echo
echo "summary: $ok ok / $fail failed / $count total"
