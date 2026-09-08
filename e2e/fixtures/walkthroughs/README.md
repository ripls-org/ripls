# Walkthrough fixtures

Committed, deterministic content for the user journey walkthroughs
(`e2e/tests/walkthroughs/`, #2684) and the phone-first workflow specs. The full
process — rendering reels, fixture-pack rules, the prod extraction runbook —
is in [`docs/walkthroughs.md`](../../../docs/walkthroughs.md).

Two kinds of content live here:

- **Stock set** (this directory): Unsplash-fetched heroes/avatars/item
  images with attribution in `manifest.json`. Regenerate missing files with
  `e2e/scripts/fetch_walkthrough_fixtures.mjs` (idempotent; delete a file to
  refresh it).
- **Real-content packs** (subdirectories, e.g. `brunch/`): images +
  screenplay (`content.json`) extracted from one real prod event by
  `server/cmd/walkthrough-export`. Each pack's `manifest.json` records
  provenance (source experience, per-file `gs://` origin) and the **consent
  statement** for the people pictured.

Rules for real-content packs: consent recorded before commit; scrubbed
content only (first names, no contact details, no street addresses — the
extractor enforces the floor, human review is the backstop); raw exports
never enter git; the repo is private and every pack is marked
`purgeIfRepoGoesPublic` — purge packs from history before any repo
visibility change.
