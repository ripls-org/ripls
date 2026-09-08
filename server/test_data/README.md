# server/test_data

Binary fixtures used by Go tests under `server/`: product photos for the AI-eval harness, EXIF samples for the location parser, format samples for the media pipeline, and the time-parsing golden set.

The hand-curated images at the top level (`atomic_maverick_ski.webp`, `mixer.jpg`, `rei_half_dome.jpg`, `rocky_talkie.jpeg`, `tent.jpg`, `ski.jpeg`, `community.jpg`, `experience.jpg`, etc.) back the existing image-mode goldens in `server/ai/eval/testdata/*.json` and a handful of HTTP-roundtrip tests. Add new hand-curated images here directly when their case is bespoke.

The `manual/` subdirectory holds images downloaded by `server/cmd/eval-seed-from-urls` from manufacturer product pages — diverse-brand seed data for the image-mode gear-detection eval. Each corresponding golden case in `gear_image_goldens.json` records the source page in its `source_url` field; that URL is the attribution pointer. See `server/ai/eval/README.md` for the seeding workflow.

EXIF / HEIC / format-specific fixtures (`books_exif.jpg`, `exif_rotated.jpg`, `example.heic`, `example.webp`, `isobmff_rotated.heic`) are checked in for the media-pipeline tests in `server/media/`. The time-parsing golden (`time_parsing_golden.json`, `time_parsing_responses.json`) backs `server/ai/time_parsing_test.go`.
