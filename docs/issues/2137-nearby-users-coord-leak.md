# Issue #2137: Security: GetNearbyUsers discloses exact home coordinates of every user to any authenticated caller

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2137
**Filed:** 2026-05-23 | **Last activity:** 2026-05-23
**Labels:** bug, security, server, privacy
**Assignees:** unassigned
**Context docs:** none

> **Audit (2026-05-23):** Plan reviewed against `docs/server/architecture.md`
> and `docs/server/observability.md` (no client-side work — both RPCs have
> no Flutter callers, so `docs/client/*.md` is not in scope). Six changes
> applied:
>
> 1. **Soft-delete filter column name corrected.** The plan originally
>    said "filter on `deleted IS NULL`." The protosql convention used
>    everywhere in `server/storage/` (`protosql_types.go:75-76`,
>    `chat_message.go:33`, `protosql_search.go:44,334,691`,
>    `chat_conversation.go:63`, `activity_digest.go:117,169,217`) is
>    `AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec
>    IS NULL)` against the flattened `DeletedMetadata.deleted_at_unix_sec`
>    column. The constant `storage.deletedFilterColumn =
>    "deleted_deleted_at_unix_sec"` documents this. Updated Phase 1 to
>    reference the constant by name and the exact `COALESCE(..., 0) = 0`
>    form used by `protosql_search.go:44` so the SQL is consistent with
>    every other JOIN-based exclusion in the codebase.
> 2. **Auth helper name unified.** The plan referenced both
>    `auth.RequireAuth` (correct) and `auth.UserFromContext` (an older
>    pattern documented in `docs/server/architecture.md:336` but no
>    longer used in `server/services/`). Confirmed via `grep -rn
>    "RequireAuth\|UserFromContext" server/services` that every modern
>    handler in `server/services/` uses `auth.RequireAuth` →
>    `authInfo.UserID`. Plan now consistently uses `RequireAuth`.
> 3. **Logging stanza spelled out.** `docs/server/observability.md`
>    requires `logging.LoggerWithContext(ctx)` with `user_id` and
>    `user_email = logging.MaskEmail(authInfo.Email)`, plus a structured
>    `operation` field. Added an explicit logging block to the
>    handler-refactor task so the implementer doesn't drop the
>    masked-email field or use a bare `log.Printf`. Errors path through
>    `connecterr.Internal(ctx, "GetNearbyUsers", err, ...)` per
>    `CLAUDE.md` (bare `connect.NewError(connect.CodeInternal, ...)` is
>    CI-gated).
> 4. **Test-isolation expectation made explicit.** Per
>    `docs/server/architecture.md:163-210`, unit tests go in
>    `*_test.go` next to the implementation, mock external dependencies
>    where possible, and use `storage.SetupTestStorage(t)`. The existing
>    `proximity_test.go` already uses `setupTestService(t)` from
>    `common_test.go`. Phase 2 task list now names that helper so the
>    test rewrite reuses the existing infrastructure instead of
>    inventing parallel scaffolding.
> 5. **Audit-emitted clarification on the `ownerField` parameter.**
>    For `GetNearbyUsers` the target's "owner" is the User itself
>    (`User.id == CommunityUser.user_id`), so the new storage helper
>    accepts the entity table's primary-key column (`"id"`) as the
>    membership-join field. For `GetNearbyGear` the field is
>    `"owner_id"`. Documented inline so the implementer doesn't
>    accidentally JOIN gear-→-community on `gear.id`.
> 6. **Phase 1 proto-comment vendor-and-implementation hygiene called
>    out.** Per `CLAUDE.md` ("Proto Comments"), the new field's doc
>    must not name SQL columns, the SQL JOIN, the `CommunityUser`
>    table, or "membership check at the handler" — only the contract
>    the client sees (required; caller must be a member of this
>    community; results restricted to fellow members). The internal
>    invariant lives in the README and `doc.go`, not in the `.proto`.

## Problem Statement

`location.GetNearbyUsers` (`server/services/location/proximity.go:15-69`) and
its sibling `location.GetNearbyGear` (`proximity.go:71-125`) accept a
caller-supplied `(latitude_deg, longitude_deg, radius_meters)` triple and
return — for every entity whose stored location falls inside the radius —
the entity's ID, distance, and the **exact stored latitude/longitude** from
the joined `Location` row. The only authorization gate is
`auth.RequireAuth`. There is no radius cap, no community scope, no
caller-vs-target relationship check, no opt-in flag, and no result cap.

For `GetNearbyUsers`, the coordinates returned are
`User.primary_residence_location_id`'s lat/lng — the user's home
address with the sub-meter precision Mapbox returns. Any authenticated
user can therefore enumerate the stored home location of every other
user in the system by sweeping query points across a coverage grid: a
~50 km radius per call covers a metro area, and a hundred-or-so calls
cover any region of interest. The `user_id`s returned then chain into
the existing IDORs flagged in #2138 (`user.GetUser` discloses email +
location ID) and #2136 (cross-community experience reads), turning the
pinned home coordinates into named profiles.

`GetNearbyGear` has the same shape against `Gear.location_id` and the same
absence of gating. Gear coordinates are owner-volunteered and slightly
less sensitive than home addresses, but they were also not shared with
the consent of "every authenticated user on the platform."

The redaction surface is wider than just the two RPCs: the storage
helper `ProtoSQLStorage.QueryByProximityWithJoin` returns the raw
location coordinates by design. Any future RPC that calls it inherits
the disclosure unless the caller explicitly drops the lat/lng on the
way back out. This plan treats that as a contributing factor, not a
separate fix — see *Open Questions* below.

## Context from Comments

One comment, from `briangbrown` (maintainer / collaborator):

> `@claude plan`

No clarifications, no constraint adjustments, no scope adjustments.
The maintainer wants a planning document before any code change — i.e.
the recommended-fix language in the issue body is a starting point, not
a decided design.

Neither `GetNearbyUsers` nor `GetNearbyGear` has any non-generated
caller in `app/lib/` — `grep -rn` against the Flutter source returns
only `app/lib/data/gen/ripls/api/location_service.*.dart` (the generated
Connect client and protobuf message types). Confirmed via:

```
grep -rn 'getNearbyUsers\|getNearbyGear' app/lib | grep -v gen/
# → no matches
```

That is load-bearing for the plan: the RPCs are **wired up but not
yet used in any UI**, so the fastest correct fix is to harden the
contract before any caller depends on the unrestricted shape, rather
than coordinating a client migration. The companion security work
(#2132–#2136, #2138) is the same broad theme — "auth-only checks on
read paths that should be membership-scoped" — and the chosen pattern
across those fixes is `RequireMemberOfActiveCommunity` plus community
scoping. Aligning with that pattern keeps reviewer load low.

## Potential Solutions

### Option A: Require a `community_id`, scope results to caller's active membership, quantize coordinates

**Approach:** Add `community_id` as a required field on both
`GetNearbyUsersRequest` and `GetNearbyGearRequest`. At the start of each
handler call `auth.RequireMemberOfActiveCommunity(ctx, s.storage,
req.Msg.CommunityId, authInfo.UserID)` — same helper #2136 standardizes
on. After the proximity query, intersect results against the membership
table for that community (only return users / gear owned by users who
are *also* active members of `community_id`). Cap radius at a small
value (recommend 50 km) and result count at 50. **Always** quantize the
returned lat/lng to 3 decimals (~110 m grid) at the service layer
before returning, regardless of how the call resolved. Drop the request
on unauthenticated callers via the existing `RequireAuth` path; reject
zero-`CommunityId` / out-of-range radius with
`connect.CodeInvalidArgument`. Replace the existing
`QueryByProximityWithJoin` call with one that filters at the SQL level
on `community_id` (extend the helper, see Implementation Plan) so we
don't pull thousands of rows just to discard them in Go.

- **Pros:**
  - Mirrors the membership pattern every other read RPC in the
    codebase uses — single mental model for the reviewer.
  - Closes the disclosure entirely: a non-member of community C cannot
    enumerate members of C even with a valid token.
  - Quantization closes the residual "I am a member, can I doxx fellow
    members?" risk — a 110 m grid is enough for "find someone near me"
    UX without leaking the building.
  - The radius cap + result cap keeps the SQL bounded for any
    plausible UI.
  - Wire-compatible at the proto level (adding a non-`optional` string
    field is a no-op for existing zero-value clients, of which there
    are none in `app/lib/`).
- **Cons:**
  - Forces the caller to know a `community_id`. That is fine for the
    intended community-discovery UX but rules out "find any nearby
    user across all my communities" UX without an additional RPC.
  - Quantization is permanent. If a later product requirement asks
    for exact pins (e.g. "set a meetup pin on the gear's location"),
    the gear endpoint will need a separate, more privileged RPC.
  - Two-sided membership intersection (caller AND target both in
    `community_id`) is one extra SQL clause vs. the current single
    proximity query.
- **Effort:** Medium. Touches the proto, both handlers, the storage
  helper (to push the community filter into SQL), all tests in
  `proximity_test.go`, plus regression tests for the new error paths.

### Option B: Force the query point to the caller's own primary residence; otherwise unchanged

**Approach:** Ignore `latitude_deg` / `longitude_deg` from the request.
Resolve the caller's `User.primary_residence_location_id` server-side,
fetch its coordinates, and use those as the query center. Keep the
proximity result shape but quantize the returned lat/lng to 3 decimals.
Cap radius at 50 km. No community scope.

- **Pros:**
  - Simpler — no new request field, no membership lookup, no SQL change.
  - Defeats the documented exploit (sweeping arbitrary query points
    over a grid) entirely: the only point Mallory can query from is
    her own home, which she already knew.
  - Quantization still protects against "snap to a target user's pin
    by triangulating from multiple of my own residences" (which would
    require multiple accounts to be useful anyway).
- **Cons:**
  - Does not address the cross-membership disclosure: any authenticated
    user with a residence set can still pull a list of every user
    within 50 km of *their* home, including users they share no
    community with. That's a smaller blast radius than the original
    bug — bounded by population density near the attacker's home,
    not "every user globally" — but it's still cross-boundary PII
    disclosure, which is the same category of finding #2138 flags
    against `user.GetUser`.
  - Doesn't match the membership-scoped pattern the rest of the
    security batch is converging on.
  - Breaks the "show users near a map pin I dropped" UX if it ever
    ships. The current UI has no such UX, but the proto shape says
    that's what the RPC is for, and walking that back later requires
    another proto change.
- **Effort:** Small.

### Option C: Quantize-only — no scope change, no membership check

**Approach:** Keep the current request shape, keep `RequireAuth`-only
authorization, but quantize the returned lat/lng to 3 decimals (~110 m
grid). Cap radius at 50 km and result count at 50. Document the
endpoint as "discover roughly-where people / gear are."

- **Pros:**
  - Minimal code change.
  - Removes "pinpoint home address to 1 m" — the building-level
    disclosure described in the issue is gone.
  - Avoids tying the API shape to community membership before product
    has decided whether proximity discovery is community-scoped or
    global.
- **Cons:**
  - Still leaks **the presence and approximate location** of every
    user globally to any authenticated caller — i.e., "user X lives
    somewhere in this 110 m × 110 m block of Denver" is still bulk
    PII disclosure, just at a coarser resolution. Combined with the
    `user_id` returned, it's still a profile-to-neighborhood mapping
    that the targeted user did not consent to.
  - Doesn't match the membership pattern used by every other
    community-read RPC.
  - The 110 m grid is enough to identify a single apartment block in
    Manhattan and a 5-house cul-de-sac in suburban Denver.
- **Effort:** Small.

## Recommendation

**Option A.** The membership-scoped pattern is the right shape for two
reasons:

1. It matches what every other community-read RPC enforces
   (`ListCommunityUsers`, `ListCommunityGear`, `ListCompletedGiveaways`,
   `ListCommunityEvents`, `GetConversationForCommunity`, and — after
   #2136 lands — `GetExperience` / `ListExperiences`). One mental model,
   one auth helper. Reviewing the diff against the existing
   `ListCommunityUsers` handler will be straightforward.

2. The intended product UX is "find people / gear in my community(ies)
   near me," not "find any user on Earth." If product later wants a
   cross-community discovery surface, that's a new endpoint with a
   different — almost certainly *more restricted* — authorization
   shape; baking the cross-community-by-default shape into the
   existing RPC because no UI uses it yet is the wrong default.

Quantization is included on top of membership because membership alone
does not protect a member's home from triangulation by a hostile
co-member. A 3-decimal grid (~110 m) keeps the proximity UX usable
(nearest neighbors still sort correctly within a block) while
removing the building-pin precision that makes this PII rather than
neighborhood-level metadata.

The radius and result caps are belt-and-braces: even inside a
community, returning 5000 users at unbounded radius is a DoS-shaped
foot-gun and not a UX the proto is documenting.

Because no UI calls either RPC today, this can ship as a
**wire-compatible-but-semantically-breaking** change: existing
zero-value clients (none in our tree) would now get
`InvalidArgument` because they pass an empty `community_id`. That's
acceptable — the RPCs are not yet on a release path that has external
callers.

## Open Questions

- **Should `community_id` be required, or should we let callers omit
  it and scope results to "the union of all of my communities"?** —
  Recommendation: required, single-community-only, for v1. Multi-
  community scope is straightforward to add later (a second RPC, or a
  `repeated string community_ids` field on a fresh request type) and
  doesn't bake in an ambiguous default. Single-community matches the
  existing pattern reviewers will be looking for.

- **What radius cap?** — Recommendation: 50 km, encoded as a
  package-level constant `maxNearbyRadiusMeters = 50_000`. That's
  comfortably larger than any single Ripls community's geography
  (cities, neighborhoods) and small enough that "sweep the planet"
  attacks cost ~10,000× more queries per square kilometer than today.
  Reject larger radii with `connect.CodeInvalidArgument` and a
  message that names the cap. Reject `radius_meters <= 0` similarly.

- **What result cap?** — Recommendation: 50, encoded as
  `maxNearbyResults = 50`. Returns the 50 closest results sorted by
  distance, drops the rest silently (no pagination cursor — the v1
  UX is "users near me," not "scroll through ranked-by-distance"). If
  product later needs pagination, add a `page_token` field; until
  then, "50 closest" is the documented contract.

- **What coordinate granularity?** — Recommendation: 3 decimals
  (`math.Round(x * 1000) / 1000`), applied at the service layer
  before constructing the response. ~110 m at the equator. Document
  the quantization in the proto comment on
  `NearbyUser.latitude_deg` / `NearbyUser.longitude_deg` so the
  contract is explicit. Same quantization for gear.

- **Do we need a `discoverable_by_proximity` flag on `User`?** —
  Recommendation: no, not in v1. The membership scope already
  encodes consent ("you joined this community → other members of
  this community can see roughly where you are"). Adding an opt-out
  flag is fine future work, but it's premature given that v1 is
  membership-scoped and no UI exposes the surface yet. If product
  decides to ship a cross-community discovery surface, the flag
  becomes load-bearing at that point.

- **Should the membership intersection also exclude soft-deleted
  users?** — Recommendation: yes. The proximity helper joins
  through `Location` and currently has no `User.deleted IS NULL`
  filter, so soft-deleted users are leaking too. Add the
  `deleted IS NULL` filter in the same SQL change. Same for
  soft-deleted gear (`Gear.deleted IS NULL`).

- **Push the community filter into SQL, or apply it in Go after the
  proximity query?** — Recommendation: push it into SQL. Either
  (a) extend `QueryByProximityWithJoin` with an optional
  `additionalJoin` parameter that lets the caller join through
  `CommunityUser` (caller's membership) and a second `CommunityUser`
  (target's membership), or (b) add a new helper
  `QueryByProximityForCommunity(ctx, lat, lng, radius, communityID,
  entityMsgType, locationRefField, ownerField)`. Recommend (b) — it's
  one new function with a tight contract, vs. growing a generic helper
  into a kitchen-sink. The new helper does the bounding-box prefilter
  identically, but JOINs to `CommunityUser` keyed on the entity's
  owner (`User.id` for users, `Gear.owner_id` for gear) and adds
  `WHERE cu.community_id = $N AND cu.deleted IS NULL`.

- **Should we hide `latitude_deg` / `longitude_deg` from `NearbyUser`
  entirely and just return `distance_meters`?** — Recommendation: no.
  The UX is "render pins on a map sorted by distance." Returning
  *only* distance forces the client to invent a fake pin position to
  sort against, which is worse. Quantized coordinates is the right
  tradeoff. The proto comment should make the quantization explicit
  so clients don't assume precision they don't have.

- **What does the maintainer expect for `GetNearbyGear`?** — The
  issue body says "apply the same fix shape: restrict by community
  membership and/or quantize coordinates." Recommendation: apply the
  same shape verbatim (community-scoped, quantized, radius/result
  capped). Same proto-shape change (`community_id` added to
  `GetNearbyGearRequest`), same handler-side membership check, same
  storage helper, same quantization constant, same test surface.

- **Tag numbers for the new `community_id` field on each request
  message?** — Recommendation: tag 4 on `GetNearbyUsersRequest` and
  tag 4 on `GetNearbyGearRequest`. Tags 1–3 are already used for
  lat/lng/radius. Document as required (non-`optional`); empty string
  yields `InvalidArgument`.

## Implementation Plan

### Phase 1: Proto + storage helper

**Goal:** API contract and SQL primitive in place; nothing wired up to
handlers yet so it's a clean, small diff.

**Tasks:**
- [ ] Edit `proto/ripls/api/location_service.proto`:
  - Add `string community_id = 4;` to `GetNearbyUsersRequest`. Proto
    comment names the field as required, says results are restricted
    to fellow members of the same community, and notes that the
    returned coordinates are quantized. **Per `CLAUDE.md` "Proto
    Comments"** the comment must not name SQL columns, the
    `CommunityUser` table, the JOIN, or the in-server helper — only
    the client-visible contract.
  - Same `string community_id = 4;` on `GetNearbyGearRequest`, same
    comment shape.
  - Update the `NearbyUser.latitude_deg` / `NearbyUser.longitude_deg`
    comments to call out the ~110 m quantization. Same for
    `NearbyGear`.
- [ ] Run `npm run generate` to regenerate Go + Dart bindings.
- [ ] In `server/storage/protosql_spatial.go`, add a new helper
  `QueryByProximityForCommunity(ctx, centerLat, centerLng,
  radiusMeters, communityID string, entityMsgType proto.Message,
  locationRefField, ownerField string) ([]SpatialQueryResult, error)`.
  - **`ownerField` contract:** the column on the entity table that
    holds the user-ID against which community membership is checked.
    For `&models.User{}` pass `"id"` (a User is its own owner). For
    `&models.Gear{}` pass `"owner_id"`. Document this in the helper's
    doc comment.
  - Implementation steps:
    - Resolve `entityTableName`, `locationTableName`, `latField`,
      `lonField` as `QueryByProximityWithJoin` does (re-use the
      existing `detectGeospatialFields` and `allowedTypes` lookups).
    - Compute bounding box via `CalculateBoundingBox` (existing).
    - Build a query that JOINs `entity e` → `Location l` ON
      `e.<locationRefField> = l.id` and JOINs
      `community_user cu` ON `cu.user_id = e.<ownerField> AND
      cu.community_id = $N`. Use the
      protosql soft-delete column constant
      `storage.deletedFilterColumn`
      (`"deleted_deleted_at_unix_sec"`, defined in
      `protosql_types.go:75-76`) for the soft-delete exclusion. The
      WHERE clause therefore reads (matching the form used in
      `protosql_search.go:44`):
      `AND COALESCE(e.deleted_deleted_at_unix_sec, 0) = 0
       AND COALESCE(cu.deleted_deleted_at_unix_sec, 0) = 0
       AND l.<latField> BETWEEN $1 AND $2
       AND l.<lonField> BETWEEN $3 AND $4`.
      (No filter needed on `Location.deleted_deleted_at_unix_sec` —
      locations are not soft-deleted in current code; if that changes,
      add the third clause.)
    - In-Go Haversine + radius filter on the bounding-box prefiltered
      rows (matching the existing pattern at
      `protosql_spatial.go:174-194`).
    - Sort by distance ascending; do not truncate inside the helper
      (caller applies the result cap so the test surface stays tight).
- [ ] Add storage tests in `server/storage/protosql_spatial_test.go`
  covering: (a) returns entities whose owners are members of the
  community; (b) excludes entities whose owners are not members;
  (c) excludes soft-deleted memberships; (d) excludes soft-deleted
  entity rows; (e) honors bounding-box prefilter + Haversine final
  filter; (f) returns empty slice when `communityID` is unknown.
- [ ] `npm run build` (which runs `golangci-lint` + `go build ./...`),
  then `go test ./server/storage/...`.

**Validates:** API contract is final; storage primitive enforces the
community-intersection invariant in SQL; soft-deleted entities/memberships
are correctly excluded. No handler change yet, so no existing tests
break.

### Phase 2: Handler refactor + quantization

**Goal:** `GetNearbyUsers` and `GetNearbyGear` enforce the new
contract, returning quantized coordinates intersected on community
membership.

**Tasks:**
- [ ] In `server/services/location/proximity.go`:
  - Declare package-level constants
    `maxNearbyRadiusMeters = 50_000.0`, `maxNearbyResults = 50`,
    `nearbyCoordinatePrecision = 1000.0` (× then ÷ for 3-decimal
    quantization).
  - Add a small package-private helper
    `quantizeCoordinate(x float64) float64` that rounds to the grid;
    unit-tested.
  - `GetNearbyUsers`:
    - **Auth + logger stanza** — mirror `ListCommunityUsers`
      (`server/services/community/membership.go:25-37`):
      ```go
      authInfo, err := auth.RequireAuth(ctx)
      if err != nil { return nil, err }
      logger := logging.LoggerWithContext(ctx).With(
          "operation",     "GetNearbyUsers",
          "user_id",       authInfo.UserID,
          "user_email",    logging.MaskEmail(authInfo.Email),
          "community_id",  req.Msg.CommunityId,
      )
      ```
      Per `docs/server/observability.md:117-130`, masked email is
      required; `operation` is the standard handler-name field.
    - Validate `req.Msg.CommunityId != ""`, `req.Msg.RadiusMeters > 0`,
      `req.Msg.RadiusMeters <= maxNearbyRadiusMeters` — reject with
      `connect.NewError(connect.CodeInvalidArgument, ...)`. Include
      the cap in the error message so the cap is discoverable from
      the wire response.
    - Call `auth.RequireMemberOfActiveCommunity(ctx, s.storage,
      req.Msg.CommunityId, authInfo.UserID)` — same pattern as
      `ListCommunityUsers` (`server/services/community/membership.go:39`).
      Propagate its error directly (it already returns the right
      `CodeNotFound` / `CodeFailedPrecondition` /
      `CodePermissionDenied` codes).
    - Replace the `QueryByProximityWithJoin` call with
      `s.storage.QueryByProximityForCommunity(..., communityID,
      &models.User{}, "primary_residence_location_id", "id")` (target
      owner field for a User row is the User's own ID).
    - On storage error wrap via
      `connecterr.Internal(ctx, "GetNearbyUsers", err)` — never bare
      `connect.NewError(connect.CodeInternal, err)` (CI-gated per
      `CLAUDE.md`).
    - Truncate to `maxNearbyResults`.
    - In the response-building loop, quantize lat/lng via
      `quantizeCoordinate`.
  - `GetNearbyGear`: same shape, same logger fields (substitute
    `"operation": "GetNearbyGear"`). The owner-field argument is
    `"owner_id"` (the gear's owner is who needs the community
    membership). The locationRefField stays `"location_id"`.
- [ ] Delete (or leave in place; see below) the still-existing call
  sites that used the legacy `QueryByProximityWithJoin` helper. The
  helper itself stays — `server/services/location/deduplication.go`
  uses the related `QueryByProximity` (no JOIN), not
  `QueryByProximityWithJoin`, so the legacy JOIN helper has no other
  callers in `server/`. **Recommendation:** leave the legacy helper in
  the file for now (it's a small amount of code, and #2020-style
  deprecation churn is not the point of this change). If the audit
  agent disagrees, remove it; the change is one file and one test.
- [ ] Rewrite `server/services/location/proximity_test.go`:
  - Existing tests already use `setupTestService(t)` from
    `common_test.go` (which wraps `storage.SetupTestStorage(t)`). Reuse
    that helper — do not introduce parallel test scaffolding. Each
    subtest gets an isolated DB container, so seeding can be naive.
  - Update every existing test to populate a `Community`, a
    `CommunityUser` row for the caller, and a `CommunityUser` row for
    each target user/owner, and to pass `CommunityId` in the request.
    Confirm the assertions stay valid (Austin / Dallas / multi-user
    cases).
  - Add new test cases:
    - **Quantization:** assert returned coordinates match
      `math.Round(rawLat*1000)/1000` for known seeded rows. Two
      adjacent users within the same ~110 m grid still sort by
      distance correctly (because the Haversine distance is computed
      on the raw stored coordinates, not the quantized ones).
    - **Non-member caller:** caller has no `CommunityUser` row for
      `community_id` → `PermissionDenied`.
    - **Non-member target:** caller is a member, but the user/gear
      whose location is in range belongs to a non-member → not
      returned.
    - **Empty `community_id`:** `InvalidArgument`.
    - **Radius == 0:** `InvalidArgument`.
    - **Radius > cap:** `InvalidArgument` (use 50,001 m).
    - **Result cap:** seed 60 in-range members, assert response
      returns exactly 50, all members of the community, sorted by
      distance ascending.
    - **Soft-deleted target user:** in range, member, but
      `deleted != nil` → not returned (covered by storage test too,
      but assert through the handler).
- [ ] `npm run build` and `go test ./server/services/location/... ./server/storage/...`.

**Validates:** End-to-end: an authenticated caller who is not a member
of `community_id` is rejected; an authenticated member sees only fellow
members within 50 km, capped at 50 results, with coordinates rounded to
the 110 m grid. The original exploit (sweep a remote query point) is
not just mitigated but the precondition (cross-community visibility) is
removed.

### Phase 3: Documentation + tracker for follow-ups

**Goal:** Capture the privacy contract somewhere reviewers can find it,
and file follow-up issues for the small pieces deferred out of this
change.

**Tasks:**
- [ ] Update `server/services/location/README.md` (or create one if
  absent — `ls` shows it exists) to document the proximity-RPC
  privacy contract: membership-scoped, quantized to ~110 m, capped at
  50 km / 50 results, soft-deleted excluded. One paragraph; link to
  `auth.RequireMemberOfActiveCommunity`.
- [ ] Update the `NearbyUser` / `NearbyGear` doc comments in the
  proto (done in Phase 1) — verify they were committed. The proto
  doc is the API contract; the README is the implementer's contract.
- [ ] File a follow-up issue: **"Add `discoverable_by_proximity`
  opt-out flag on `User`"** — scope: per-user toggle that suppresses
  the user from proximity results even within their own communities,
  for users who want to be in a community without their (quantized)
  home location surfaced to other members. Punt the work; just
  record it.
- [ ] File a follow-up issue: **"Decide whether the legacy
  `ProtoSQLStorage.QueryByProximityWithJoin` helper should be
  removed"** — scope: confirm no remaining callers, remove if so;
  otherwise document why it stays. Low-priority cleanup.

**Validates:** The privacy contract is documented; the deferred
pieces are tracked, not forgotten.

## Related Issues

- #2136 — Companion finding: `experience.GetExperience` /
  `ListExperiences` skip the same membership check. The fix pattern
  there (`RequireMemberOfActiveCommunity`) is exactly the pattern this
  plan adopts.
- #2138 — Companion finding: `user.GetUser` discloses every user's
  email + `primary_residence_location_id`. Combined with the exploit
  documented in this issue, the disclosed `user_id` + lat/lng would
  resolve to a named person + email. Fixing both removes the chain.
- #2132, #2133, #2134, #2135 — Same broader class of "auth-only
  checks on read paths that should be membership-scoped" (or similar
  scope-leak / SSRF in #2135). Cross-link in the PR description for
  context.
