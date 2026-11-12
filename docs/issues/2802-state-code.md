# Issue #2802: [bug] extractStateCode always returns empty, so state regions are persisted with no code

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2802
**Filed:** 2026-07-29 | **Last activity:** 2026-07-29
**Labels:** none | **Assignees:** unassigned
**Context docs:**
- `docs/server/architecture.md`, `docs/server/conventions.md`, `docs/proto_conventions.md` (server anchors)
- `docs/ai/regions.md` (region-model design notes; US-only scope)
- `server/services/location/regions.go`, `server/location/regions.go`, `server/location/mapbox.go`, `server/location/google.go` (touched code)
- `proto/ripls/api/location_service.proto` (request/response contract for the affected RPC)
- `app/lib/services/location_service.dart`, `app/lib/services/mapbox_location_service.dart`,
  `app/lib/presentation/viewmodels/governance_view_model.dart` (client caller for the RPC)

## Problem Statement

`extractStateCode(stateName string) string` in
`server/services/location/regions.go:329` is a stub that always returns `""`.
It is called from four live sites inside `CreateRegionFromAddress`, two of
which pass its result straight into `regionlib.GetOrCreateRegion` as the new
region's `region_code`. Every US-state region created through this RPC is
therefore persisted with an empty `code`, and every child city that this RPC
tries to scope with its state (`"Boulder, CO"`) silently falls back to the bare
name (`"Boulder"`) because the state-code check at line 275 always fails.

The downstream damage is real, not cosmetic:

- `GetRegionDisplayName` (`server/location/regions.go:124`) uses the state's
  `region_code` to shorten `"Boulder, Colorado"` → `"Boulder, CO"` on
  search results. With the code empty it always falls back to the long form.
- The city-naming step at `services/location/regions.go:275` builds
  `region_name = "Boulder, CO"` and passes it to `GetOrCreateRegion`, whose
  dedup key is `(region_type, region_name, parent_region_id)`. Because the
  suffix silently drops, two clients that both search "Boulder, CO" produce a
  `region_name` of just `"Boulder"` — correct dedup for now, but the moment
  the fix lands, new writes use `"Boulder, CO"` and no longer dedup with the
  existing `"Boulder"` row for the same parent state. That is a real
  compat/migration surface, not just an inert TODO.
- All already-written state rows have `region_code = ""`. Because
  `FindRegionByDetails` (`server/location/regions.go:147`) keys on
  `(region_type, region_name)` and ignores `region_code`, new calls after the
  fix will find those rows and skip creation — so the codes stay empty
  forever without an explicit backfill.

The issue title frames this as "stub always returns empty," but the actual
change spans a naming decision (USPS vs. ISO-3166-2), a wire-format decision
(let the server look it up vs. have the client send it), and a data-migration
decision (backfill or don't).

## Context from Comments

Only comment is `@claude` from the maintainer requesting a plan. All
substantive direction is in the issue body:

- The maintainer explicitly asks whether `region_code` should be the USPS
  two-letter abbreviation (what the doc comment promises: `"Colorado"` → `"CO"`)
  or the full state name (what the stub actually stores today — i.e., empty).
  The two `GetOrCreateRegion` call sites pass the full name **separately** as
  `regionName`, so an empty `code` is clearly unintended.
- The maintainer flags that the geocoding provider already returns a
  subdivision short code — `administrative_area_level_1` `short_name` in
  Google, `region_code` / `short_code` in Mapbox — and suggests we could pass
  that through instead of hardcoding a US-only table. Codebase confirms this:
  `server/location/mapbox.go:412-414` and `server/location/google.go:709-711`
  both write `RegionCode = "CO"`-style values into the `Place` struct, and
  the Flutter side (`mapbox_location_service.dart`) already has the value.
- Per CLAUDE.md, any backfill TODO left in the fix must reference a
  tracking issue filed before the TODO lands. The maintainer explicitly
  asks us to decide what to do about the rows already written with empty
  codes.

## Potential Solutions

### Option A: Static US state name → USPS code map, server-side

- **Approach:** Replace the `extractStateCode` stub with a small
  `map[string]string` of the 50 states + DC + US territories (`"Colorado"` →
  `"CO"`, `"Puerto Rico"` → `"PR"`, etc.), keyed case-insensitively on the
  trimmed input. Return `""` for anything not in the map (so non-US callers
  and typos fall through gracefully). Change nothing on the wire; change
  nothing on the client. File a separate issue for the one-off backfill of
  existing `state`-type rows with empty `region_code` and add a `TODO(#NNNN):`
  in the seam that will do the backfill (a migration script under
  `server/storage/` or a one-shot `main` under `server/tools/` — whichever
  pattern the reviewer prefers; there is no existing precedent for either in
  this repo, which is worth flagging).
- **Pros:**
  - Single file changed (`services/location/regions.go`) + one test file +
    one backfill-tracking issue.
  - Zero proto churn, zero client change, zero regeneration cascade.
  - Matches the scope declared in `docs/ai/regions.md` ("US-only deployment")
    and the doc comment already on the function.
  - Trivial to unit-test with a table-driven test.
- **Cons:**
  - US-only. If/when the app goes international this map is dead weight, and
    non-US callers still get an empty code (same as today, but by design).
  - Duplicates knowledge the geocoding provider already returned upstream —
    the client asked Mapbox for the exact code we're now re-deriving on the
    server. Feels architecturally silly even if it works.
  - The static map has to include `"Washington, D.C."`, `"District of
    Columbia"`, `"D.C."`, and territories; input normalization has to handle
    each of the shapes Mapbox and Google actually emit.
- **Effort:** Small (~1–2 hours + backfill issue).

### Option B: Plumb the subdivision short code from the client through the RPC

- **Approach:** Add a new `administrative_area_code` field to
  `CreateRegionFromAddressRequest` (proto field 10). Update the Dart client
  in `app/lib/services/location_service.dart` and its caller in
  `governance_view_model.dart` to pass `location.adminCode` (currently only
  `location.regionCode` — the country CLDR code — is on the wire; the
  subdivision short code from Mapbox is captured in the Dart layer at
  `mapbox_location_service.dart:110-121` but not carried on any Dart model
  yet, so a new field on `LocationResult` is needed too). Replace
  `extractStateCode(req.Msg.AdministrativeArea)` at all four call sites with
  `req.Msg.AdministrativeAreaCode`, and keep the four call sites otherwise
  intact. If the client didn't send a code (older builds, missing Mapbox
  data), fall back to Option A's map so the server never regresses today's
  behavior. Same backfill story as Option A.
- **Pros:**
  - Works outside the US the day we cross the border; no future retrofit.
  - Uses data the geocoding provider already gave us — no drift between our
    static table and Mapbox/Google's canonical short codes.
  - `region_code` becomes a straightforward pass-through instead of a
    string-inference problem.
- **Cons:**
  - Wire-format change → proto edit + `npm run generate` + Dart model edit
    + Dart caller edit + a new field on the Dart `LocationResult` for the
    subdivision code, currently not on the Dart model. Wider blast radius.
  - Requires client-version handling: old app builds continue to send an
    empty field, so we still need Option A's static fallback for the US
    (which is the entire user base today) to fix the bug immediately for
    users on shipped clients. So B does not eliminate A; it adds on top.
  - Server side has to decide whether to trust the client-provided code or
    validate it against a canonical set — the naïve "trust the client" is
    fine as long as `region_code` is only used for display, but downstream
    code (`GetRegionDisplayName`) does `strings.ToUpper`-style formatting on
    it, and we don't currently validate ISO shape.
- **Effort:** Medium.

### Option C: Server-side reverse-geocode on the request lat/lon

- **Approach:** `CreateRegionFromAddressRequest` already has `latitude_deg`
  and `longitude_deg` (fields 1–2). Wire the server's existing
  `location.Provider` (Google or Mapbox — the same one the client just used)
  into `Service` and reverse-geocode server-side to pull the subdivision
  short code. Use whatever the provider returns; skip the static map.
- **Pros:**
  - Works globally without a table.
  - Nothing on the wire changes.
- **Cons:**
  - Second geocoding call per region creation (client already made one). Extra
    latency, extra provider bill, extra failure mode. All to derive a
    two-letter string.
  - The `location.Service` in `server/services/location/` currently doesn't
    hold a provider (`service.go` has no `Provider` field); wiring one is
    non-trivial and touches DI + tests.
  - Introduces provider dependency into an RPC that today is a pure
    write-through — regresses a nice architectural property.
- **Effort:** Medium–large; not obviously worth it.

## Recommendation

**Option A now, keep Option B as a follow-up.**

Rationale:

1. The bug the issue is filed against is *actively wrong data on writes,
   today, for US users*. Option A closes it in one file with a table-driven
   test. Option B alone requires app-store propagation before it takes
   effect for shipped clients (still on the buggy behavior), so we'd need
   Option A as a fallback under B anyway.
2. The `docs/ai/regions.md` design doc pins the near-term scope to
   US-only. The static map is exactly the right shape for that scope.
3. Option B is genuinely better long-term architecturally, but it is a
   proto/wire change with client work, and the maintainer's issue frames the
   near-term ask as fixing the empty write. File B as a follow-up
   *enhancement* issue when we do international expansion, or during a
   sprint focused on the governance flow; don't bundle it with the
   correctness fix.
4. Option C's extra provider call is not worth it for two letters.

## Open Questions

- **Q1: What's the exact map — 50 states, plus D.C., plus territories?**
  Recommendation: 50 states + `"District of Columbia"` (with `"Washington,
  D.C."`, `"Washington DC"`, `"D.C."` normalized to `"DC"`) + PR, GU, VI, AS,
  MP (US inhabited territories). USPS-style two-letter codes. Case-insensitive
  lookup on trimmed input; empty return for non-US or unrecognized inputs.
- **Q2: Where does the backfill live, and what triggers it?**
  Recommendation: file a companion tracking issue titled "Backfill empty
  region_code for existing state regions" and add
  `// TODO(#<companion-issue>): backfill existing state rows with empty
  region_code` near `extractStateCode`. The backfill itself is one-off
  (small US-region table x count-of-empty-state-rows), can be a manual SQL
  UPDATE at deploy time or a `go run ./server/tools/backfill_state_codes`
  script — either is acceptable. Do not implement the backfill in this PR.
  Recommendation for the SQL shape:

  ```sql
  UPDATE regions
     SET region_code = <lookup(region_name)>,
         updated_at_unix_sec = extract(epoch from now())
   WHERE region_type = 'state'
     AND (region_code IS NULL OR region_code = '');
  ```

- **Q3: Does the fix change any city-region `region_name` values (and thus
  break dedup with previously-written `"Boulder"` rows)?**
  Recommendation: yes — the code path at `services/location/regions.go:275`
  will start emitting `"Boulder, CO"` where it previously silently emitted
  `"Boulder"`. This is the *correct* behavior per the code intent, but it
  means the first post-fix write for each US city creates a new row rather
  than dedup'ing with the pre-fix one. Options: (a) accept the split and let
  the backfill cover it (rewrite `region_name` for existing rows to
  `"City, CODE"` in the same one-off script); or (b) leave cities untouched
  and only fix the state-region code path (i.e., stop building
  `"Boulder, CO"` in the city step). Recommendation: **(a)** — the code
  clearly intends scoped city names, and search UX is worse without them
  ("Springfield, MO" vs. "Springfield, IL"). Include this in the same
  companion backfill issue.
- **Q4: Should we validate the state name is a US state at the
  `CreateRegionFromAddress` boundary?**
  Recommendation: no. `extractStateCode` returning `""` for non-US inputs is
  the same behavior we ship today; the state region still gets created with
  the correct `region_name`. Adding a boundary rejection would break the
  RPC for non-US inputs that currently work (badly, but work).

## Implementation Plan

### Phase 1: Fix the write path (this PR)

**Goal:** Every state region created through `CreateRegionFromAddress`
persists the correct USPS two-letter code, and city regions get their
`", CO"` suffix.

**Tasks:**
- [ ] **Place the lookup in the `regionlib` shared library, not the service.**
  Add a new file `server/location/state_codes.go` in the `location` package
  (same package as `regionlib.GetOrCreateRegion`). Export
  `ExtractStateCode(stateName string) string`. Rationale from the anchor
  `docs/server/architecture.md` §"Shared Functionality in Libraries" — the
  service should own RPC wiring, not a lookup table that a second caller may
  well want later (e.g. the Phase 2 backfill script, or `regionlib` itself
  when it later needs to auto-populate `RegionCode` for state-typed writes).
  Delete the service-package `extractStateCode` and call
  `regionlib.ExtractStateCode` from all four call sites.
- [ ] Populate the map with 50 states + `"District of Columbia"` (with
  `"Washington, D.C."`, `"Washington DC"`, `"D.C."` normalized to `"DC"`) +
  the 5 inhabited US territories (PR, GU, VI, AS, MP). Key on
  `strings.ToLower(strings.TrimSpace(input))`. Names must match what
  Mapbox's `context.region.name` and Google's
  `administrative_area_level_1` `long_name` actually emit — verify
  against the fixtures already in `server/location/mapbox_test.go` and
  `server/location/google_test.go` before committing the map.
- [ ] **Fail-fast discipline:** Returning `""` from `ExtractStateCode` for
  unrecognized non-empty input is the one silent-failure exception, and
  per `docs/server/conventions.md` §"Error Handling and Robustness" it
  needs an inline comment naming the fallback strategy (non-US /
  unknown-name input is intentionally preserved as `""` so the state
  region is still created with `region_name` = the full string, matching
  today's behavior for those callers). Add that comment.
- [ ] **Observability hook:** At each of the four call sites in
  `server/services/location/regions.go`, log a **Warn** through the
  existing scoped logger when `req.Msg.AdministrativeArea != "" &&
  ExtractStateCode(...) == ""` — surface it as
  `logger.WarnContext(ctx, "unrecognized administrative_area, storing
  state region with empty code", "administrative_area",
  req.Msg.AdministrativeArea)`. This is our drift signal: it should fire
  ~never for US-only traffic, so any spike means Mapbox handed us a
  spelling variant we didn't expect. `administrative_area` is public
  place-name data — no masking needed per
  `docs/server/observability.md`. Compute the code exactly **once** per
  branch and reuse the value; do not call `ExtractStateCode` twice at the
  city and county branches (today the code calls `extractStateCode` twice
  per branch, at the naming step and again at the parent-creation step;
  factor into a local `stateCode := ...` and thread it through).
- [ ] Add a table-driven test file `server/location/state_codes_test.go`
  covering: all 56 lookups, case variants, whitespace trimming, D.C. name
  variants, empty input (`""` → `""`), and non-US input (`"Ontario"` →
  `""`, `"São Paulo"` → `""`). No `//go:build integration` tag — this is
  a pure-function unit test. Use table-driven form per
  `docs/testing/architecture.md`.
- [ ] Add a small integration-style test in a new
  `server/services/location/regions_test.go` (this file doesn't exist
  yet) covering `CreateRegionFromAddress` with `AdministrativeArea =
  "Colorado"` + `Locality = "Boulder"` — assert the returned state's
  `region_code` is `"CO"` and the returned city's `region_name` is
  `"Boulder, CO"`. Use `storage.SetupTestStorage(t)` per the server
  testing convention. This is the regression test for the bug the issue
  filed.
- [ ] File the companion backfill issue **before** committing the TODO
  (per CLAUDE.md's TODO policy). Title: "Backfill empty `region_code` for
  existing `state` regions written before #2802 fix, and rescope
  `region_name` for cities to `"City, CODE"`". Body: what's wrong, what
  to update, sample SQL (from Open Question Q2 + Q3), rate-limit /
  migration notes.
- [ ] Add a `TODO(#<companion-issue>):` comment near
  `ExtractStateCode` in `server/location/state_codes.go` pointing at the
  backfill issue, with 2–3 lines of inline context (what the code does
  today, what the backfill will fix, why we deferred it). Format matches
  the existing `TODO(#2020):` proto-deprecation TODOs called out in
  CLAUDE.md — inline body is self-contained, not "see #NNNN".
- [ ] Run `npm run build` (golangci-lint + `go build ./...`) and
  `go test -tags=integration -timeout 20m ./...` before pushing, per
  `docs/server/conventions.md` §"Go Server Build Verification". No proto
  regeneration needed — this PR does not change any `.proto` file.

**Validates:** New state rows persist with the right two-letter code; new
city rows persist with `"City, CODE"` `region_name`;
`GetRegionDisplayName` on state parents starts returning `"Boulder, CO"`
correctly. The existing state and city rows are unchanged (still empty
codes / unscoped names) — that's the backfill issue's job. The Warn log
gives us a live drift signal for any unrecognized state-name inputs after
the fix ships.

### Phase 2 (follow-up, not this PR): backfill the existing rows

Owned by the companion issue filed in Phase 1. Out of scope here; captured
in Open Question Q2 and Q3.

### Phase 3 (follow-up, not this PR): plumb client-provided code

If/when the app goes international, take Option B — add
`administrative_area_code` to `CreateRegionFromAddressRequest`, thread it
through the Dart client, and keep the static map as the fallback for
older clients and unrecognized names. Not in scope for the correctness
fix.

## Related Issues

- #1611 — TODO-gate enforcement (`check_todo_refs.js`). The
  `TODO(#<companion-issue>):` we leave for the backfill must satisfy this
  gate; that's why the companion issue has to be filed first.
- #2020 — proto-deprecation TODOs, cited in CLAUDE.md as the exemplar
  of the TODO-with-issue format we're following.

## Audit notes

Audited against `docs/server/architecture.md`, `docs/server/conventions.md`,
`docs/server/observability.md`, `docs/proto_conventions.md`,
`docs/testing/architecture.md`, and the review prompts for
`api_design`, `error_handling`, `code_quality`, and `testing`. Fixes
applied to the plan as a result:

- Moved `extractStateCode` from the service package into the
  `server/location/` shared library, per the architecture anchor's
  "shared functionality in libraries" rule — the service should not own
  a domain lookup another caller (backfill script, region library) will
  reasonably want.
- Called out the fail-fast exception: silent `""` for unrecognized input
  needs a rationale comment per `conventions.md` §"Error Handling and
  Robustness"; and added a Warn-level log at each call site so the
  silent path is observable, satisfying the observability review's
  "silent failures leave you blind" concern.
- Named the specific test files (`state_codes_test.go` + a new
  `regions_test.go`), pointed at the existing test-storage helper, and
  required a regression test at the RPC boundary — the testing review's
  "critical-path coverage" bar.
- Removed a latent double-call bug in the target code: today
  `extractStateCode(req.Msg.AdministrativeArea)` is invoked twice per
  city/county branch. The plan now factors the result into a local so
  we don't compute it twice with the fixed implementation either.
- Confirmed **no proto change** in Phase 1, so no API-design review
  concerns (backwards compatibility, field numbering, read-path coverage
  per the #1142 pattern) apply here. Phase 3, if picked up later, will
  add `administrative_area_code` as a new numbered field — that PR gets
  its own audit.
