## Issue #1144: Document proto API/models conventions and add sync checks

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1144
**Filed:** 2026-04-16 | **Last activity:** 2026-04-16
**Labels:** none | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

The conventions that govern `proto/ripls/api/` vs `proto/ripls/models/` — what
belongs where, how fields propagate across the boundary, how requests and
responses are shaped, how conversion logic is organized — exist only as short,
scattered paragraphs in `CLAUDE.md` (lines 65-72) and `README.md` (lines
60-63). New contributors (and Claude) have to infer the rest from existing
code, which works until the boundary has a gap — as it did in #1142, where
`mapbox_feature_id` was added to `models.Location` and to
`api.SaveLocationRequest` but was missed on `api.GeocodedLocation`, the return
path from `GenExperience`. The client had no way to forward an ID it could
never receive, and no test, lint rule, or review caught it because there was
nothing codified to check against.

This issue is not about changing the convention — the separation and flattening
rules are working. It is about (1) writing the convention down in a single
place, (2) giving contributors a checklist for the "add a field end-to-end"
operation, and (3) catching the specific class of omission that caused #1142
before it reaches prod.

## Context from Comments

There are **no comments** on this issue. The body is self-contained and
specific. The scope, tradeoffs, and motivating incident (#1142) are all laid
out in the description.

Because nobody pushed back on the framing or proposed alternatives, this plan
uses the issue body as the de facto spec and calls out anywhere we are making
judgment calls that a reviewer might want to revisit.

## Current State

Repo inspection on 2026-04-17 confirms the issue description:

- `proto/ripls/api/` — 35 `.proto` files (services + shared types like
  `location.proto`, `gear.proto`, `user.proto`, `portfolio.proto`, etc.).
- `proto/ripls/models/` — 19 `.proto` files mirroring storage entities.
- `CLAUDE.md:65-72` — two IMPORTANT paragraphs (separation rule, no reuse of
  RPC message types).
- `README.md:60-63` — repeats the separation rule.
- `docs/server/architecture.md:49-52` — one bullet line pointing at the two
  directories.
- `proto/ripls/models/README.md` — documents state machines, not proto
  conventions.
- `buf.yaml` — STANDARD lint with `PACKAGE_VERSION_SUFFIX` excepted; `buf
  breaking` uses `FILE` rules. No custom plugins, no sync-enforcement logic.
- `docs/release/cheat_sheet.md:9-18` — checks for renames/renumbers in
  `proto/ripls/models` only; no prompt to verify API surface.

The fields that #1142 had to add end-to-end (`external_place_id` +
`external_place_provider`) now appear on:

- `models.Location` (`proto/ripls/models/location.proto:100-105`)
- `api.Location` — not yet exposed via an API message (internal only)
- `api.GeocodedLocation` (`proto/ripls/api/location.proto:24-28`)
- `api.SaveLocationRequest` (per #1142 plan)
- `location_repository.dart`, `LocationResult`, `LocationPickerViewModel`

No file in the repo currently documents *why* those fields had to be added in
six places, or gives the next contributor a template for the same operation.

## Potential Solutions

### Option A: Docs-only — design doc + release cheat-sheet section

- **Approach:** Write `docs/server/proto-conventions.md` covering the
  separation rule, message-shape rules (flattened inline fields, dedicated
  request/response per RPC), naming conventions, where conversion logic
  lives, and an "add a field end-to-end" checklist. Update
  `docs/release/cheat_sheet.md` to check API/model sync when model fields
  change. Update `CLAUDE.md` and `README.md` to link at the new doc instead
  of duplicating the rules.
- **Pros:** Cheap, immediately useful, captures everything a human or an
  agent needs to avoid the #1142 mistake. Zero risk of breaking builds. Can
  be written in a few hours and reviewed in a single PR.
- **Cons:** Purely human-enforced. Future omissions are still possible —
  the doc tells you what to check, but nothing blocks a PR that forgets.
  Depends on reviewers (and Claude) actually reading the checklist.
- **Effort:** Small.

### Option B: Docs + round-trip test pattern

- **Approach:** Everything in Option A, plus a repeatable Go test pattern
  that exercises round-trips for user-provided fields: client input →
  `Save*` RPC → storage → `Get*` / `Gen*` RPC → client-visible output. One
  test per exposed field on `Location`, `Gear`, `User`, etc. Capture the
  pattern as a named helper (e.g. `testutil.AssertFieldRoundTrip`) so the
  next person wiring a field end-to-end adds one line.
- **Pros:** Catches the #1142 class of bug mechanically. Tests are the
  natural place to enforce "the API can carry what the model can store."
  Pattern scales: each new user-visible field gets one line of coverage.
  Keeps the enforcement where contributors already look (a red test).
- **Cons:** Requires deciding which fields are "user-visible" vs internal
  (not all model fields are exposed on purpose — e.g. soft-delete metadata,
  audit timestamps). Tests need updating when fields are intentionally
  server-only. Medium write-up cost.
- **Effort:** Medium.

### Option C: Docs + automated proto diff in CI

- **Approach:** Docs (A), plus a script that, on every PR touching
  `proto/ripls/models/*.proto`, diffs the new model fields against the API
  protos and posts a comment or fails the check if a new model field has no
  obvious match in any API message. Script would be heuristic (name match,
  type compatibility) and opt-out-able.
- **Pros:** Catches omissions at PR time without a test needing to exist.
  Visible to the PR author before anyone reviews.
- **Cons:** High false-positive rate — plenty of model fields are
  intentionally server-only (audit timestamps, soft-delete markers, denorm
  counters). Tuning the heuristic and maintaining the allowlist is ongoing
  work. Risk of the check becoming noise that people dismiss, which is
  worse than no check at all. Also, the bug it catches is a subset of what
  Option B's round-trip tests catch more reliably.
- **Effort:** Medium-to-large, with ongoing maintenance.

## Recommendation

**Option B: docs + round-trip test pattern.** The docs alone are table-stakes
and should ship regardless (CLAUDE.md already gestures at these rules — we
owe contributors the full version). The round-trip test pattern is where the
real value is: it is the smallest durable mechanism that would have caught
#1142, it lives in the place contributors already look when wiring a field
(tests next to the service), and it scales linearly in effort per field
added.

Option C's CI check is tempting but pays out worst: the rule it would encode
("new model field → add to API") has too many intentional exceptions to run
unsupervised, and Option B catches the same class of bug more reliably with
less drift risk.

Scope note: this issue is not asking us to backfill round-trip coverage for
every existing field. The proposal is to *establish the pattern* and add
coverage for the field families most exposed to this bug (`Location`, `Gear`,
`Experience`, `Request`, `User`). Existing features without coverage can
accrete tests as they are touched.

## Open Questions

- **Where does the convention doc live — `docs/server/` or a new
  `docs/proto/`?** — Recommendation: `docs/server/proto-conventions.md`.
  The proto layer is consumed by both server and app, but server
  conversion logic is where the convention actually binds. Matches the
  existing `docs/server/architecture.md` and avoids a one-file directory.

- **Which model fields should be considered "API-exposed" by default, and
  which are intentionally server-only?** — Recommendation: document a
  small explicit list in the new doc (`created_at_unix_sec`,
  `updated_at_unix_sec`, `deleted`, search index vectors, cached denorm
  counters, anything under a `// server-only` comment block). Everything
  else is presumed API-exposed unless a field comment says otherwise.
  Makes the implicit rule from CLAUDE.md concrete.

- **Should dedicated request/response message naming be enforced by a
  buf-custom lint rule?** — Recommendation: no, keep it as a documented
  convention. The rule "never reuse request/response types across RPCs"
  is rarely broken in practice and a lint rule would require a buf
  plugin. Re-evaluate if we see a recurrence.

- **Does the round-trip helper live in `server/services/testutil` or
  per-service?** — Recommendation: add it to the existing
  `server/services` shared-helper package, alongside
  `server/services/test_util.go` (where `WaitForNotification` /
  `WaitForStockImagery` already live and are imported by subpackage
  tests via `"type2fun.org/ripls/server/services"`). File:
  `server/services/roundtrip_helpers.go`, `package services`. This
  matches the existing pattern for cross-service test helpers and avoids
  introducing a new `testutil` subdirectory for one helper. Reuse
  `storage.SetupTestStorage` per CLAUDE.md.

- **Do we need a Dart-side equivalent round-trip test?** — Recommendation:
  no. The round trip is: client proto → server RPC → storage → server
  RPC → client proto. Testing that in Go covers both ends of the wire
  because the generated Dart code is a direct mirror. A Dart test would
  only catch codegen misconfiguration, which `npm run generate`
  already fails on.

## Implementation Status (2026-04-17)

All phases implemented on branch `issue-1144-proto-conventions`:

- **Phase 1** — `bd214cce` Document proto API/models conventions.
- **Phase 2** — `db8a43d0` AssertFieldRoundTrip helper + location
  round-trip tests. Incidentally fixed a discovered #1142-class gap:
  `GetLocationResponse` was not exposing `external_place_id` /
  `external_place_provider` even though `SaveLocationRequest` was. Added
  those fields to the response proto and populated them in
  `places.go:GetLocation`.
- **Phase 3** — `d1bd6a2d` Round-trip tests for Gear, User, Request +
  release cheat-sheet safety check.
- **Phase 3 follow-up** — `aa80c8eb` Round-trip test for Experience
  (originally deferred in the plan; added once it was clear
  `SaveExperience`'s direct path does not require AI/Mapbox mocks — only
  `GenExperience` does, and that is already covered by
  `gen_ai_integration_test.go`).
- **Housekeeping** — cross-link from
  `docs/reviews/api_design_review_prompt.md` to
  `proto-conventions.md`, and this status block.

Plan retained as a record of the decision process, not an active TODO.

## Implementation Plan

### Phase 1: Write `docs/server/proto-conventions.md`

**Goal:** A single document a human or agent can read in 10 minutes that
codifies every rule currently spread across CLAUDE.md, README.md, and tribal
knowledge.

**Tasks:**
- [ ] Create `docs/server/proto-conventions.md` with these sections:
    - The separation rule (API vs models never import each other, why).
    - Flattened request/response: each RPC gets dedicated inline-field
      messages (`SaveGearRequest`, `GetGearResponse`, not shared types).
    - Field-naming parity across layers (`model.X.foo` and the API message
      that carries it use the same field name; document known exceptions).
    - Where conversion logic lives (service layer, inline in the
      feature file as `convertXToAPI` / `convertXToStorage` helpers —
      point at a concrete example, e.g.
      `server/services/gear/service.go:1092 convertStorageGearMetadataToAPI`).
      Dedicated `convert.go` files are *not* the current pattern;
      document this so future contributors don't create one
      speculatively.
    - "Expose a model field end-to-end" checklist: models proto → storage
      SQL (automatic), API proto(s) on the save path, API proto(s) on the
      read/gen path(s), service-layer conversion, repository / viewmodel
      on the client, test that round-trips the field.
    - Intentionally-server-only field list and how to mark them.
    - Worked example based on `external_place_id` from #1142 (short
      narrative of the six places it had to land).
- [ ] Update `CLAUDE.md:65-72` to keep the short rules but link at the new
  doc for the full version.
- [ ] Update `README.md:60-63` the same way.
- [ ] Add a pointer from `proto/ripls/models/README.md` and from
  `docs/server/architecture.md:49-52`.

**Validates:** A new contributor (or Claude on a fresh session) can find
and follow the convention without reading existing code. Diffs against the
#1142 plan should be zero — we're only documenting what that plan already
had to discover.

### Phase 2: Round-trip test pattern + one concrete instance

**Goal:** Establish `AssertFieldRoundTrip` as the repeatable way to prove
"this model field survives the save → read journey," and use it for the
field family the incident came from.

**Tasks:**
- [ ] Add `server/services/roundtrip_helpers.go` (`package services`,
  alongside the existing `test_util.go`) exporting
  `AssertFieldRoundTrip[T]` — generic over field type, takes a save
  function, a fetch function, and a field accessor, asserts input ==
  output. Use `storage.SetupTestStorage` per CLAUDE.md.
- [ ] Add `server/services/roundtrip_helpers_test.go` exercising the
  helper against a trivial in-memory double so the helper itself has
  coverage (required by CLAUDE.md: "always add unit tests for each new
  library or significant library function").
- [ ] Add `server/services/location/roundtrip_test.go` covering:
    - `external_place_id` + `external_place_provider` via `SaveLocation` →
      `GetLocation`.
    - Same pair via the `GenExperience` return path on
      `api.GeocodedLocation` (the path that #1142 missed).
    - `neighborhood`, `county`, `administrative_area` via both paths.
- [ ] Run `npm run build` to verify lint + compile; tests pass with
  `go test ./server/services/location/...`.

**Validates:** The helper catches the exact regression class of #1142 when
simulated (temporarily drop a field from the response path and watch the
test fail). Provides a copy-paste template for Phase 3.

### Phase 3: Extend round-trip coverage and update the release cheat sheet

**Goal:** Broaden coverage to the other high-exposure entities, and add
the human checkpoint that catches what tests cannot.

**Tasks:**
- [ ] Add round-trip tests for the save/read pairs on: `Gear`,
  `Experience`, `Request`, `User` (fields that cross the API boundary per
  the Phase 1 exposed-field convention).
- [ ] Update `docs/release/cheat_sheet.md` "Safety check" section to add
  a bullet: "Model field changes — verify the corresponding API messages
  (both request and response paths) were updated. Link to
  proto-conventions.md."
- [ ] Cross-link the checklist from `docs/reviews/` if a reviews
  prompt covers this area (not a blocker if none).

**Validates:** End-to-end: new model fields added after this ships have
both a documented process and a red test waiting when the API side is
forgotten. Release-time verification is in the cheat sheet, not just
tribal.

## Related Issues

- #1142 — Mapbox API: capture `mapbox_id`. The motivating incident.
  Already merged; Phase 2's concrete test covers the exact path that
  originally regressed.
- #1143 — Proximity bias. Indirectly related: #1143 exposed more fields
  on `Gear`/`Experience`/`Request` across the boundary and is a recent
  example of the same end-to-end pattern. Good source of test cases for
  Phase 3.

---

## Audit log (2026-04-17, /ripls-audit)

Reviewed against `docs/server/architecture.md`,
`docs/client/architecture.md`, `docs/client/caching.md`, and
`docs/client/i18n.md`. Client-side docs are not directly in scope for
this plan (no caching, viewmodel, or i18n changes proposed); server
architecture is the relevant reference.

Changes applied:

- **Test helper location corrected.** Original plan placed the helper
  in a new `server/services/testutil/` subpackage. Repo already has
  `server/services/test_util.go` in `package services` — imported by
  subpackage tests (e.g. `server/services/chat/*.go`, `web/service_test.go`)
  as `"type2fun.org/ripls/server/services"`. Plan now aligns with the
  existing pattern (`server/services/roundtrip_helpers.go`,
  `package services`). Avoids introducing a parallel shared-helpers
  directory for one file.
- **Conversion-logic description corrected.** Original plan described
  conversion as living in a `convert.go` sibling file. Actual
  convention (verified in `server/services/gear/service.go:698-1207`) is
  inline helpers named `convertXToAPI` / `convertXToStorage` within the
  feature file. Plan now documents the real convention and flags that
  a dedicated `convert.go` would be *non-standard*, so the new
  `proto-conventions.md` doesn't codify an invented pattern.
- **Helper unit-test obligation made explicit.** CLAUDE.md requires a
  unit test for each new library/helper. Added an explicit task to
  Phase 2 for `roundtrip_helpers_test.go`, with a note pointing at the
  rule.

No compliance issues found with:

- Service independence (plan adds no cross-service dependencies).
- API/models import rule (plan reinforces the existing rule rather than
  relaxing it).
- Client architecture (plan explicitly declines a Dart-side round-trip
  test; rationale is consistent with the generated-code model in
  `docs/client/architecture.md` and `docs/client/i18n.md`).
- Caching (not in scope — plan touches neither repositories nor
  viewmodels).
- Testing philosophy (`docs/server/architecture.md` §Testing Strategy:
  "Unit tests are always preferable to integration tests"). Round-trip
  tests as proposed run against `SetupTestStorage` (testcontainers), so
  they are integration-flavored. That is appropriate for this class of
  bug — the bug is precisely that the contract across layers was wrong,
  which a unit test with mocks would not catch. Retained as-is.
