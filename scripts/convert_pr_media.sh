#!/usr/bin/env bash
# Convert raw PR media in /tmp/pr-media/<pr>/<uuid>.<ext> to web-friendly
# variants in /tmp/pr-media-web/<pr>/<uuid>.{webm,mp4,jpg,png}.
#
# - .webm / .mp4 / .gif (videos): re-encode to a smaller .webm (VP9) +
#   .mp4 (H.264) at max 600px wide, and extract a poster JPG from the
#   first frame.
# - .png (screenshots): resize to max 720px wide using sips (macOS),
#   leaving alpha intact.
#
# Idempotent — skips outputs that already exist.

set -euo pipefail

SRC="${1:-/tmp/pr-media}"
DST="${2:-/tmp/pr-media-web}"

mkdir -p "$DST"

convert_video() {
    local in="$1"
    local out_base="$2"
    local webm="${out_base}.webm"
    local mp4="${out_base}.mp4"
    local poster="${out_base}.jpg"

    # Common ffmpeg args: max 600 px wide, even dimensions, no audio (it's all
    # silent screen capture), framerate cap at 24, force yuv420p (libvpx-vp9
    # rejects GIF's native gbrap pixel format and h264 needs yuv420p anyway).
    local vfilter='scale=min(600\,iw):-2:flags=lanczos,fps=24,format=yuv420p'

    if [[ ! -f "$webm" ]]; then
        ffmpeg -y -loglevel error -i "$in" -vf "$vfilter" -an \
            -c:v libvpx-vp9 -b:v 0 -crf 35 -row-mt 1 \
            "$webm"
    fi

    if [[ ! -f "$mp4" ]]; then
        ffmpeg -y -loglevel error -i "$in" -vf "$vfilter" -an \
            -c:v libx264 -preset veryslow -crf 28 \
            -pix_fmt yuv420p -movflags +faststart \
            "$mp4"
    fi

    if [[ ! -f "$poster" ]]; then
        ffmpeg -y -loglevel error -i "$in" -vf "$vfilter" \
            -frames:v 1 -q:v 4 \
            "$poster"
    fi

    local in_size webm_size mp4_size
    in_size=$(wc -c < "$in" | tr -d ' ')
    webm_size=$(wc -c < "$webm" | tr -d ' ')
    mp4_size=$(wc -c < "$mp4" | tr -d ' ')
    printf "  vid: %s\n       in=%dKB webm=%dKB mp4=%dKB\n" \
        "$(basename "$in")" $((in_size / 1024)) $((webm_size / 1024)) $((mp4_size / 1024))
}

convert_image() {
    local in="$1"
    local out="$2"
    if [[ -f "$out" ]]; then
        return
    fi
    sips -Z 720 "$in" --out "$out" >/dev/null 2>&1
    local in_size out_size
    in_size=$(wc -c < "$in" | tr -d ' ')
    out_size=$(wc -c < "$out" | tr -d ' ')
    printf "  img: %s in=%dKB out=%dKB\n" \
        "$(basename "$in")" $((in_size / 1024)) $((out_size / 1024))
}

for pr_dir in "$SRC"/*/; do
    pr=$(basename "$pr_dir")
    out_dir="$DST/$pr"
    mkdir -p "$out_dir"
    echo "PR #$pr"

    for f in "$pr_dir"*.*; do
        [[ -e "$f" ]] || continue
        ext="${f##*.}"
        base=$(basename "$f" ".$ext")
        case "$ext" in
            webm|mp4|mov|gif)
                convert_video "$f" "$out_dir/$base"
                ;;
            png|jpg|jpeg|webp)
                convert_image "$f" "$out_dir/$base.png"
                ;;
            *)
                echo "  skip: $f (unknown ext .$ext)"
                ;;
        esac
    done
done

echo "---"
echo "output sizes by PR:"
du -sh "$DST"/*/ | sort -rh
echo
echo "total: $(du -sh "$DST" | cut -f1)"
