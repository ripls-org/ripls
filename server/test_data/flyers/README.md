# Event-flyer eval fixtures

Flyer images used by the `experience_image` eval suite
(`server/ai/eval/testdata/experience_image_goldens.json`) to test event
extraction from posters — dates with and without years, explicit and
missing times, venues, multi-day ranges, and no-signal designs.

Collected 2026-07-08 from publicly available flyer-template galleries
(Canva-style template sites and EDIT.org's free template pages) for
#1265 Phase 3. All names, addresses, and phone numbers visible in the
designs are the templates' own placeholder text ("Borcelle", "123
Anywhere St.", 555-prefix numbers) — no real PII. WebP/SVG originals
were converted to PNG for vision-API compatibility. Internal eval use
only, same fair-use footprint as the rest of `server/test_data/` (see
`server/ai/eval/README.md` → Attribution).

Ground truth per file lives in the goldens JSON; the flyer text is the
source of truth — verify against the image, not the case comment.
