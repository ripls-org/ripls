#!/usr/bin/env bash
# Fetch GitHub PR media (user-attachments) using a logged-in browser session
# cookie. Required because GitHub's REST API does not expose a download
# endpoint for user-attachments — they're served from a session-locked
# uploads service.
#
# Usage:
#   1. Drop the cookie value at $COOKIE_FILE (single line, just the value
#      of the user_session cookie from a logged-in github.com tab).
#      In Chrome: DevTools → Application → Cookies → https://github.com →
#      user_session → Value.
#   2. Run: scripts/fetch_pr_media.sh <pr-number> [<pr-number> ...]
#      Outputs to /tmp/pr-media/<pr>/<asset-uuid>.<ext>
#
# Notes:
#   - The cookie file is gitignored. Never commit it.
#   - Cookies expire after ~2 weeks; if downloads start 404'ing, refresh
#     the cookie value.
#   - This script writes nothing to git-tracked paths. Curated final assets
#     are staged manually after review.

set -euo pipefail

COOKIE_FILE="${GH_USER_SESSION_FILE:-$HOME/.gh-user-session}"
OUT_BASE="${PR_MEDIA_DIR:-/tmp/pr-media}"

if [[ ! -f "$COOKIE_FILE" ]]; then
    echo "error: cookie file not found at $COOKIE_FILE" >&2
    echo "save the value of the user_session cookie there (single line, no quotes)" >&2
    exit 1
fi

USER_SESSION=$(<"$COOKIE_FILE")
USER_SESSION="${USER_SESSION//$'\n'/}"
USER_SESSION="${USER_SESSION//$'\r'/}"

if [[ -z "$USER_SESSION" ]]; then
    echo "error: cookie file is empty" >&2
    exit 1
fi

if [[ $# -eq 0 ]]; then
    echo "usage: $0 <pr-number> [<pr-number> ...]" >&2
    exit 1
fi

mkdir -p "$OUT_BASE"

for pr in "$@"; do
    body=$(gh pr view "$pr" --json body --jq .body 2>/dev/null || true)
    if [[ -z "$body" ]]; then
        echo "skip: PR #$pr has no body or could not be fetched" >&2
        continue
    fi

    # Extract every user-attachments asset UUID from the body.
    mapfile -t uuids < <(echo "$body" | grep -oE 'user-attachments/assets/[a-f0-9-]+' | awk -F/ '{print $NF}' | sort -u)

    if [[ ${#uuids[@]} -eq 0 ]]; then
        echo "skip: PR #$pr has no user-attachments"
        continue
    fi

    pr_dir="$OUT_BASE/$pr"
    mkdir -p "$pr_dir"

    for uuid in "${uuids[@]}"; do
        # Probe the URL to discover content-type so we pick a sane extension.
        # First call follows the redirect to private-user-images.githubusercontent.com.
        tmp="$pr_dir/.$uuid.tmp"
        url="https://github.com/user-attachments/assets/$uuid"
        http_code=$(curl -sLo "$tmp" -w "%{http_code}" \
            --cookie "user_session=$USER_SESSION" \
            -H "User-Agent: Mozilla/5.0 (asset-fetcher)" \
            "$url")
        if [[ "$http_code" != "200" ]]; then
            echo "  fail: PR #$pr asset $uuid → HTTP $http_code"
            rm -f "$tmp"
            continue
        fi

        # Detect type from magic bytes. file(1) is widely available on macOS.
        mime=$(file --brief --mime-type "$tmp")
        case "$mime" in
            video/mp4)        ext=mp4 ;;
            video/quicktime)  ext=mov ;;
            video/webm)       ext=webm ;;
            image/png)        ext=png ;;
            image/jpeg)       ext=jpg ;;
            image/gif)        ext=gif ;;
            image/webp)       ext=webp ;;
            *)                ext=bin ;;
        esac

        out="$pr_dir/$uuid.$ext"
        mv "$tmp" "$out"
        size=$(wc -c < "$out" | tr -d ' ')
        echo "  ok:   PR #$pr asset $uuid → $out ($mime, ${size} bytes)"
    done
done
