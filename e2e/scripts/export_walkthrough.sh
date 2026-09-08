#!/usr/bin/env bash
#
# export_walkthrough.sh [reel-slug] — post-process the latest run of one
# walkthrough reel (#2684) into shareable deliverables + QC frames. Normally
# invoked by run_walkthrough.sh; can be run standalone from the e2e/
# directory after a `--project=walkthrough` run.
#
# A reel's scenes are the tests of tests/walkthroughs/<reel>.spec.ts, each of
# which records exactly one clip into its own Playwright output directory
# (test-results/walkthroughs-<reel>-<NN>-…-walkthrough/). Scene titles carry
# a zero-padded index, so lexicographic directory order IS reel order. A
# scene's first settle() writes trim.json next to the clip (lib/walkthrough.ts)
# — the seconds of blank-boot head footage to cut.
#
# Hi-DPI capture: specs record a 390×844 phone viewport with the browser
# launched at --force-device-scale-factor=2, so the screencast captures at 2×
# device pixels (780×1688) — full-frame and crisp (see e2e/lib/capture.ts).
#
# Produces under e2e/videos-walkthroughs/<reel>/ (gitignored):
#   scene-NN-<name>.webm   per-scene native masters (untrimmed)
#   scene-NN-<name>.mp4    per-scene H.264 clips (head-trimmed)
#   <reel>.mp4             the assembled reel (scene concat)
#   walkthrough.html       scene-by-scene walkthrough page, when a
#                          fixture pack provides copy (walkthrough.md whose
#                          frontmatter names this reel)
#   frames/frame_*.png     one frame every 2s of the assembled reel, for QC
set -euo pipefail
cd "$(dirname "$0")/.."

REEL="${1:-onboarding}"

# Desktop-aspect renders (#2912) are LOCAL evaluation artifacts: they land in
# a "-desktop"-suffixed directory so they never clobber the phone
# deliverables, and they skip the walkthrough-page build below entirely — the
# page frames clips in a phone frame and auto-deploys to
# website/content/walkthroughs/, neither of which applies to a desktop
# evaluation pass.
DESKTOP_RENDER=false
if [[ "${WALKTHROUGH_VIEWPORT:-}" == "desktop" ]]; then
  DESKTOP_RENDER=true
fi

# Collect each scene's clip: newest .webm per matching output dir, dirs in
# lexicographic (= scene) order. NOTE: reel slugs must not be prefixes of one
# another (e.g. 'events' and 'events'), or the glob would mix reels.
SCENE_CLIPS=()
for dir in $(ls -d "test-results/walkthroughs-${REEL}-"* 2>/dev/null | sort); do
  clip=$(ls -t "${dir}"/*.webm 2>/dev/null | head -1 || true)
  [[ -n "${clip}" ]] && SCENE_CLIPS+=("${clip}")
done
if [[ ${#SCENE_CLIPS[@]} -eq 0 ]]; then
  echo "no clips for reel '${REEL}' under test-results/ — run the spec first:" >&2
  echo "  npx playwright test tests/walkthroughs/${REEL}.spec.ts --project=walkthrough" >&2
  exit 1
fi

OUT="videos-walkthroughs/${REEL}"
if [[ "${DESKTOP_RENDER}" == "true" ]]; then
  OUT="videos-walkthroughs/${REEL}-desktop"
fi
rm -rf "${OUT}"
mkdir -p "${OUT}/frames"

# Per-scene deliverables: copy the untrimmed master, transcode a head-trimmed
# H.264 clip (yuv420p for QuickTime/browser compatibility) at native size.
i=0
CONCAT_LIST="${OUT}/.concat.txt"
: >"${CONCAT_LIST}"
for clip in "${SCENE_CLIPS[@]}"; do
  i=$((i + 1))
  dir=$(dirname "${clip}")
  # Scene name from the output dir: strip the spec prefix + project suffix,
  # cap the length so filenames stay manageable.
  dirbase=$(basename "${dir}")
  name=$(printf '%s' "${dirbase}" |
    sed -E "s/^walkthroughs-${REEL}-//; s/^-?walkthrough-//; s/-walkthrough$//" | cut -c1-48)
  scene=$(printf 'scene-%02d-%s' "${i}" "${name}")
  trim=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['trimSec'])" \
    "${dir}/trim.json" 2>/dev/null || echo 0)
  cp "${clip}" "${OUT}/${scene}.webm"
  ffmpeg -hide_banner -loglevel error -y -ss "${trim}" -i "${OUT}/${scene}.webm" \
    -c:v libx264 -pix_fmt yuv420p -crf 18 -movflags +faststart \
    "${OUT}/${scene}.mp4"
  echo "file '$(pwd)/${OUT}/${scene}.mp4'" >>"${CONCAT_LIST}"
done

# The assembled reel: the trimmed scene mp4s share one encoder + size, so the
# concat is a lossless stream copy.
ffmpeg -hide_banner -loglevel error -y -f concat -safe 0 -i "${CONCAT_LIST}" \
  -c copy -movflags +faststart "${OUT}/${REEL}.mp4"
rm -f "${CONCAT_LIST}"

# QC frames (every 2s) from the assembled reel.
ffmpeg -hide_banner -loglevel error -y -i "${OUT}/${REEL}.mp4" \
  -vf "fps=1/2" "${OUT}/frames/frame_%03d.png"

# Scene-by-scene walkthrough page, when a fixture pack carries copy for this
# reel (walkthrough.md with `reel: <slug>` frontmatter). Never built for
# desktop renders — see DESKTOP_RENDER above.
WALKTHROUGH_MD=""
if [[ "${DESKTOP_RENDER}" != "true" ]]; then
  for md in fixtures/walkthroughs/*/walkthrough.md; do
    [[ -f "${md}" ]] || continue
    if grep -qE "^reel: *${REEL}$" "${md}"; then
      WALKTHROUGH_MD="${md}"
      break
    fi
  done
fi
if [[ -n "${WALKTHROUGH_MD}" ]]; then
  node scripts/build_walkthrough_page.mjs "${REEL}" "${WALKTHROUGH_MD}" "${OUT}"
  # Style the standalone artifact: the page's colors come from the site's
  # generated design tokens (raw colors are lint-banned on the site copy);
  # a sibling copy makes the relative <link> resolve off-site too.
  cp ../server/services/web/static/css/gen/tokens.gen.css "${OUT}/tokens.gen.css"
fi

echo "wrote (reel '${REEL}', ${i} scene(s)):"
for f in "${OUT}"/scene-*.mp4; do
  echo "  ${f}  ($(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0:s=x "${f}"))"
done
echo "  ${OUT}/${REEL}.mp4  ($(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0:s=x "${OUT}/${REEL}.mp4")) assembled reel"
[[ -n "${WALKTHROUGH_MD}" ]] && echo "  ${OUT}/walkthrough.html  (copy: ${WALKTHROUGH_MD})"
echo "  ${OUT}/frames/ ($(ls "${OUT}"/frames | wc -l | tr -d ' ') frames)"
