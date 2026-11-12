# Issue #1326: [api_design] Partial-update semantics broken on SaveUser/SaveGear

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1326
**Filed:** 2026-04-25 | **Last activity:** 2026-04-27
**Labels:** auto-filed, review:api_design | **Assignees:** unassigned
**Context docs:** `docs/proto_conventions.md`

## Problem Statement

`SaveUserRequest` and `SaveGearRequest` document "omitted fields are ignored,"
but they declare bare proto3 scalars (`string name = 2;` etc.) and proto3 bare
scalars cannot express presence: an unset field and a field set to `""` are
indistinguishable on the wire.

Today the server papers over this by checking `req.Msg.X != ""` for every
updatable string field. That makes partial updates work for the common case
(omit a field → it stays unchanged), but it has two correctness gaps:

1. **Cannot clear a field.** A user who wants to delete their bio
   (`description`), unset their preferred timezone, or wipe a gear's stored
   location has no way to say so. The server silently ignores the empty
   string.
2. **Cannot tell "unset" from "explicitly empty".** This means we cannot
   ever evolve toward true presence semantics without a wire-format change.

`SaveGearRequest.source_url` already uses `optional` — proving the pattern
works in this codebase. The fix is to apply it to every other updatable
scalar on the two save RPCs and switch the handlers from `!= ""` to `!= nil`.
It is wire-compatible (proto3 already encodes `optional` scalars with the
explicit-presence bit).

The issue has been carried forward 143 days across two reviews; the
recommendation in the issue is exactly what the existing partial code
patterns and conventions already point at.

### Field-by-field gap

| RPC | Field | Current handler check | Problem when client sends `""` |
|-----|-------|------------------------|--------------------------------|
| SaveUser | `name` | `!= ""` | Ignored — can't clear |
| SaveUser | `primary_residence_location_id` | `!= ""` | Ignored — can't clear |
| SaveUser | `description` | `!= ""` | Ignored — can't clear |
| SaveUser | `preferred_timezone` | `!= ""` | Ignored — can't clear |
| SaveUser | `media_id` | `!= ""` (paired with `media_ids`) | Ignored — can't clear avatar via this scalar. Note: the Dart client (`user_service.dart`) does **not** currently send `media_id`; avatar updates flow through `media_ids` (repeated). The proto-side `optional` change is still wire-compatible and worth shipping for consistency, but the Dart "clear avatar" path remains the repeated-field path and is out of scope here. |
| SaveGear | `name` | `!= ""` | Ignored — can't clear |
| SaveGear | `description` | `!= ""` | Ignored — can't clear |
| SaveGear | `location_id` | `!= ""` | Ignored — can't clear |
| SaveGear | `generation_mode` | (only used at insert) | n/a |
| SaveGear | `source_url` | `!= nil` | **Already correct** |

Repeated fields (`other_location_ids`, `media_ids`) are a separate question,
discussed under Open Questions — proto3 has no `optional` for `repeated`.

## Context from Comments

The issue has three `@claude` pings from a maintainer (maintainer) but no
substantive design discussion. The recommendation in the issue body is
unambiguous: *"Add `optional` to every updatable scalar field. Wire-compatible.
Pair with round-trip tests that assert empty-string writes either round-trip
or preserve prior values."*

No disagreement, no clarifying questions, no alternative proposed.

## Potential Solutions

### Option A: Add `optional` to every updatable scalar on the two save RPCs

- **Approach:** Add `optional` to each scalar in `SaveUserRequest` (name,
  primary_residence_location_id, media_id, description, preferred_timezone)
  and `SaveGearRequest` (name, description, location_id, generation_mode).
  In the Go handlers, replace every `if req.Msg.X != ""` with `if req.Msg.X
  != nil` and dereference. Update the Dart clients to forward `null` /
  `String?` straight through (drop the `isNotEmpty` guards on save). Add
  round-trip tests that assert empty-string clears and that omitted fields
  preserve prior values. Leave repeated fields as-is for now (they keep
  the documented "always replaced" semantics — see Open Questions).
- **Pros:**
    - Wire-compatible: proto3 `optional` on existing field numbers is a
      transparent upgrade for existing clients. Zero-downtime, no migration.
    - Mirrors the pattern already proven by `SaveGearRequest.source_url`.
    - Eliminates the silent "can't clear" bug categorically — there's only
      one place per field where presence is checked, and the type system
      guarantees we check it.
    - Round-trip tests can grow a "clear then re-fetch" assertion that
      mechanically prevents regression.
    - Small enough that a single PR is reviewable end-to-end.
- **Cons:**
    - Generated Go signatures change: `req.Msg.Name` becomes `*string`. Any
      caller that reads these fields outside the handler (there are none
      today, but it's a contract surface) needs updating.
    - Generated Dart code adds `has*()` methods; existing callers keep
      working but the Dart client wrapper should drop its `isNotEmpty`
      guards so that `description = ""` actually reaches the server.
    - Doesn't address repeated-field semantics — `media_ids = []` is still
      ambiguous. (See Open Question on repeated fields.)
- **Effort:** Small. Two proto edits, two handler refactors, two Dart-client
  edits, expanded round-trip tests.

### Option B: FieldMask-based partial update

- **Approach:** Add `google.protobuf.FieldMask update_mask = N;` to each save
  request. Server only writes fields that are listed in the mask. Bare
  scalars stay as scalars; the client says "update name and description" by
  setting `update_mask.paths = ["name", "description"]`.
- **Pros:**
    - Industry-standard pattern (Google APIs, AIP-134). Generalizes to all
      future save RPCs.
    - Cleanly distinguishes "clear" from "don't touch" without per-field
      `optional` annotations.
    - One mechanism solves scalars *and* repeated fields uniformly.
- **Cons:**
    - Larger client surface: every Dart caller has to build a FieldMask
      (currently they pass `name: "x"` directly). High refactor cost across
      `user_service.dart`, `gear_service.dart`, repositories, viewmodels.
    - Server has to interpret string paths and dispatch — error-prone,
      and validating mask paths is a new boilerplate burden on every
      handler.
    - Connect-Go has no first-class FieldMask helpers; we'd be the first
      RPC in this codebase to use one.
    - Doesn't deliver any user-visible benefit beyond what Option A does
      for the in-scope fields.
- **Effort:** Medium-Large.

### Option C: Wrapper messages (per-field StringValue)

- **Approach:** Use `google.protobuf.StringValue` (or hand-rolled wrapper
  messages) for each updatable field. `nil` wrapper = "don't touch",
  wrapper containing `""` = "clear".
- **Pros:**
    - Same correctness properties as Option A.
    - Works uniformly for repeated fields too if you wrap them in a
      sub-message.
- **Cons:**
    - Strictly more verbose than `optional` for the same semantics. The
      proto3 `optional` keyword *is* the modern equivalent of these
      wrappers and was added to the language specifically to retire them.
    - Requires importing `google/protobuf/wrappers.proto`.
    - No precedent in this codebase. We use `optional` everywhere field
      presence is needed today.
- **Effort:** Small-Medium (slightly more boilerplate than A).

## Recommendation

**Option A.** The issue itself recommends it, the codebase already uses the
pattern (`SaveGearRequest.source_url`, `SaveUserRequest.phone_number` on the
storage side, plus many response-side `optional` fields), it's
wire-compatible, and the surface change is mechanical and small. Options B
and C solve the same in-scope problem with strictly more machinery and
weaker codebase precedent.

Repeated-field semantics (`media_ids = []` ambiguity) are left out of scope
for this fix — the documented behavior today is "repeated fields are
replaced entirely," which is internally consistent and which existing
callers rely on. Changing that is a separate API decision that should not
be bundled with the scalar fix; surfacing it as an Open Question is enough.

## Open Questions

- **Should repeated fields (`media_ids`, `other_location_ids`) also gain
  presence semantics?** — Recommendation: **No, not in this fix.** Proto3
  has no `optional` for `repeated`; supporting "leave unchanged" requires a
  wrapper message (e.g. `MediaIdList { repeated string ids = 1; }`), which
  is a heavier API change with downstream callers in the Flutter app
  (`gear_repository.dart`, `user_profile_view_model.dart`) that already
  rely on the "replace entirely" contract. The current behavior is
  documented and consistent — file a separate issue if the product ever
  needs "leave list unchanged" as a distinct signal from "clear list to
  empty".

- **Should we update the proto comments to spell out the new semantics?**
  — Recommendation: **Yes.** The "omitted fields are ignored" sentence in
  the message-level comment is technically still true, but the per-field
  comments should explicitly say *"Set to clear; omit to leave unchanged."*
  for the now-`optional` scalars. This is the durable contract clients
  read; documenting it once removes future ambiguity.

- **Should the Dart client API surface change to allow callers to express
  "clear this field"?** — Recommendation: **Yes, but minimally.** Today
  `userService.saveUser(name: "")` silently does nothing because the
  client guards `isNotEmpty` before assigning. After this fix, drop those
  guards so that passing an empty string clears the field; passing `null`
  continues to mean "don't touch". This matches Dart's `String?` semantics
  naturally. We should *not* introduce a separate sentinel
  (`UnsetSentinel`) — the empty-string-vs-null distinction is enough, and
  it mirrors the proto3 `optional` semantic exactly.

- **Does any production caller currently pass `""` expecting the server to
  ignore it?** — Recommendation: **Audit before merge, but expect no.**
  The Dart wrappers already filter empty strings client-side
  (`if name != null && name.isNotEmpty`) for `name`, `primary_residence_location_id`,
  `preferred_timezone`, `id`, `description` (gear), `mediaIds` (gear),
  `locationId`, `generationMode`, `sourceUrl`. Only
  `user_service.dart:127-129` (description) does **not** check
  `isNotEmpty` — but it's currently broken: an empty description sent
  today is dropped by the server. After this fix it will work as
  intended. Search Connect-Go test fixtures and any non-Dart client
  (there are none) for `Name: ""` patterns; flag any that exist as
  caller bugs to fix in the same PR.

- **Should the round-trip test helpers grow a "clear" mode?** —
  Recommendation: **Yes.** Add a sibling `AssertFieldClear` (or extend
  `AssertFieldRoundTrip` with a clear-after step) so that every
  `optional` API field gets a single line in `roundtrip_test.go`
  asserting "save value, save empty/clear, get returns empty". This is
  the structural guard the issue specifically calls for.

## Implementation Plan

### Phase 1: Proto changes + handler refactor (atomic)

**Goal:** Change the wire format and switch the server to nil-checks in a
single coherent PR. After this phase, partial-update semantics are correct
on the scalar fields.

**Tasks:**

- [ ] Edit `proto/ripls/api/user_service.proto:45-74`. Add `optional` to
      `name`, `primary_residence_location_id`, `media_id`, `description`,
      `preferred_timezone`. Update each field comment from "(optional
      update)" to "Optional. Set to clear; omit to leave unchanged."
      Update the message-level comment so a reader doesn't have to
      reason from "ignored" to "omitted vs nil".
- [ ] Edit `proto/ripls/api/gear_service.proto:30-58`. Add `optional` to
      `name`, `description`, `location_id`, `generation_mode`. Same
      comment treatment. Leave `source_url` as-is (already optional).
      Leave `id` (insert vs update sentinel — empty means insert; this
      is documented contract not a partial-update field) and `metadata`
      (already a message, optional by proto3 default) alone.
- [ ] Run `npm run generate` to regenerate Go and Dart bindings. Confirm
      Go now exposes `*string` for the changed fields and Dart adds
      `has*()` methods.
- [ ] Edit `server/services/user/service.go:104-164`. Replace each
      `if req.Msg.X != ""` with `if req.Msg.X != nil` and dereference.
      For `preferred_timezone`, keep the IANA validation but only run
      it when `*req.Msg.PreferredTimezone != ""` so clients can clear
      the timezone without tripping validation. For
      `primary_residence_location_id`, the "demote old primary" logic
      (lines 144-152) currently triggers on `!= ""`; preserve that
      behavior — it should only fire when the client is *setting* a
      primary, not clearing it. Document the semantics inline in a
      one-line comment.
- [ ] Edit `server/services/gear/service.go:127-245`. Two passes —
      insert path (lines 126-134) needs to dereference the new
      optionals into the storage model, defaulting to `""` if nil
      (insert semantics: missing field = empty string in storage).
      Update path (lines 231-238) replaces `!= ""` with `!= nil`.
- [ ] Edit `server/services/user/service.go` (PreferredTimezone clear
      branch): if `req.Msg.PreferredTimezone != nil &&
      *req.Msg.PreferredTimezone == ""`, skip validation and set to
      empty. This is the only field with a non-trivial clear story.
- [ ] Run `npm run build` and `go test -tags=integration -timeout 20m
      ./server/services/user/... ./server/services/gear/...`. Iterate
      until green.

**Validates:** Server compiles, existing tests pass, the "partial update"
tests in `user/service_test.go:677` and `:752` still pass, and the
round-trip test in `user/roundtrip_test.go` still passes (the assigned
non-empty values still round-trip).

### Phase 2: New tests for clear semantics

**Goal:** Lock in the new "clear" behavior with regression tests so the
fix doesn't silently regress.

**Tasks:**

- [ ] Add `server/services/user/service_test.go` test
      `TestSaveUser_ClearOptionalFields`:
        - Create user with name + description + preferred_timezone +
          primary_residence_location_id populated.
        - Call SaveUser with each optional field set to a pointer-to-empty
          string in turn (one per sub-test). Verify GetUser shows the
          field cleared and other fields unchanged.
        - Call SaveUser with the field omitted (nil pointer). Verify
          GetUser shows the field unchanged.
- [ ] Add `server/services/gear/service_test.go` test
      `TestSaveGear_ClearOptionalFields`:
        - Same shape as the user test, for `name`, `description`,
          `location_id`. (Note: clearing `name` may be semantically
          undesirable at the product layer; either allow it and add a
          UI guard later, or reject it in the handler with
          `InvalidArgument`. **Recommendation: allow it at the API
          layer; UI guards belong in UI.**)
- [ ] Extend `server/services/user/roundtrip_test.go`. After the existing
      assertions, add a "clear" pass: send a SaveUser with each
      now-optional field pointing at an empty string, then assert
      GetUser returns empty for that field. Use the existing
      `services.AssertFieldRoundTrip` if extended, or write a new
      `AssertFieldClear` helper in `server/services/roundtrip_helpers.go`.
- [ ] Extend `server/services/gear/roundtrip_test.go` similarly.
- [ ] Run `go test -tags=integration -timeout 20m
      ./server/services/user/... ./server/services/gear/...
      ./server/services/...`.

**Validates:** Empty-string writes now round-trip correctly; missing
fields preserve prior values; both behaviors are tested independently.

### Phase 3: Dart client surface

**Goal:** Let callers actually exercise the new "clear" capability from
the app, without breaking existing callers that pass `null` to mean
"leave alone".

**Tasks:**

- [ ] Edit `app/lib/services/user_service.dart:114-132`. Drop the
      `&& X.isNotEmpty` guards on `name`, `primaryResidenceLocationId`,
      `preferredTimezone`. Keep the `null` checks. After the change:
      `null` ⇒ field unset on the request (preserves server-side
      value); `""` ⇒ field set to empty (clears server-side value).
      The `description` branch already has no `isNotEmpty` guard;
      it's fine as-is.
- [ ] Edit `app/lib/services/gear_service.dart:92-112`. Same change for
      `name`, `description`, `locationId`, `generationMode`. Leave
      `id` alone — empty `id` is the documented insert sentinel, not a
      "clear" semantic.
- [ ] Audit `app/lib/data/repositories/user_repository.dart:92-114` and
      `app/lib/data/repositories/gear_repository.dart:113-139` for the
      same pattern. The repositories pass through to services; if
      they have their own `isNotEmpty` guards, drop them too. If they
      have nullable parameters, those keep their meaning.
      **Cache invalidation is unaffected.** Both repos already call
      `invalidate(userId)` / `invalidate(id)` and downstream cache
      flushes (`_cache.invalidate('media_urls:$userId')`,
      `refreshUserGear()`, `_searchRepository.invalidateSearches()`,
      `_onDailyInvalidated?.call()`) after every `saveUser`/`saveGear`,
      regardless of whether a field was set, omitted, or cleared. A
      "clear" mutation re-uses the existing invalidation pathway
      verbatim — no new cache plumbing is required by this fix. This
      matches the cache-invalidation convention in
      `docs/client/caching.md` § "Cache Invalidation".
- [ ] Verify that `app/lib/data/repositories/user_repository.dart:129-137`
      (`updatePreferredTimezone`) — which already passes `''` to
      `saveUser` when `timezone` is null — now actually clears the
      stored timezone. This is a previously-broken path that becomes
      correct as a side-effect of this fix.
- [ ] Run `cd app && flutter test --reporter github`. Iterate until green.
- [ ] Add service-layer tests under `app/test/services/` (e.g.
      `user_service_test.dart`, `gear_service_test.dart`) that assert
      passing `""` produces a `SaveUserRequest` / `SaveGearRequest` with
      the corresponding `has*()` returning true and the value being
      empty, while passing `null` leaves the field unset
      (`has*()` returns false). This is the correct level for the
      assertion: per `docs/client/architecture.md`, the service layer
      owns request construction; viewmodels go through repositories
      with mocks and never see the wire-level request shape.
- [ ] If any existing viewmodel tests under
      `app/test/presentation/viewmodels/` mocked the repository in a
      way that depended on the old "empty string is dropped" behavior,
      update them. Do **not** add new viewmodel tests asserting wire
      shape — that belongs at the service layer.

**Validates:** End-to-end clear path works from a viewmodel through
service → server → storage → GetUser response. UI work to surface a
"clear" affordance (e.g. a button on the bio editor) is **out of scope**
for this fix — that's a product call, separate issue.

### Phase 4: Doc + lint hygiene

**Goal:** Make sure the new contract is reflected in the durable docs.

**Tasks:**

- [ ] Update `docs/proto_conventions.md` if there's a section on
      partial-update / `optional` semantics; if not, add a short
      subsection under "API message shape" called *"Updatable scalars
      use `optional`"* with one-paragraph rationale and a pointer to
      `SaveGearRequest.source_url` and `SaveUserRequest.description`
      as canonical examples.
- [ ] Run `npm run lint:proto` and `npm run build`.

**Validates:** Future RPCs that add updatable scalar fields default to
the right pattern instead of the bare-scalar trap.

## Acceptance Criteria

- All updatable scalar fields on `SaveUserRequest` and `SaveGearRequest`
  use `optional`.
- Server handlers check `!= nil` (not `!= ""`) for those fields.
- A SaveUser/SaveGear call that sets a field to `""` (presence bit on,
  empty value) clears that field in storage; a subsequent GetUser/GetGear
  returns empty for that field.
- A SaveUser/SaveGear call that omits a field leaves the prior value
  unchanged.
- Existing partial-update tests still pass.
- New round-trip tests cover the "clear" path for every now-optional
  field.
- Dart clients pass through `null` vs `""` correctly (`null` = omit;
  `""` = clear).
- `npm run build` is green; `npm run lint:proto` is green; full Go test
  suite (`go test -tags=integration -timeout 20m ./...`) is green;
  `flutter test --reporter github` is green.
- No production caller relies on the silently-dropped-empty-string
  behavior in a way that would now incorrectly clear data. (Audit:
  Dart wrappers all guard with `isNotEmpty` today, so the only newly
  reachable path is the deliberate one we add.)
- Architecture compliance:
  - **proto/api/models separation** (`docs/server/architecture.md`):
    only `proto/ripls/api/*.proto` is touched. `proto/ripls/models/*`
    is untouched. The handler still does the api ↔ models conversion
    inline, dereferencing optional pointers into the storage model's
    bare scalars on insert and into `Update` calls on update.
  - **Client data flow** (`docs/client/architecture.md`): the change
    is confined to the `services/` layer (request construction).
    Repositories continue to be the only callers of services and
    continue to own cache invalidation.
  - **Cache invalidation** (`docs/client/caching.md`): existing
    `invalidate(userId)` / `invalidate(id)` and downstream
    `refreshUserGear` / `invalidateSearches` / daily invalidation
    calls fire regardless of which fields a save touched, so a
    "clear" save invalidates the cache exactly the same way an
    "update" save does today. No new invalidation hooks needed.
  - **i18n** (`docs/client/i18n.md`): no user-visible strings added
    or changed. The new behavior is a wire-format correctness fix;
    UI affordances for "clear this field" are explicitly out of
    scope.

## Related Issues

- **Round-trip-test convention** (referenced in
  `docs/proto_conventions.md` § Round-trip tests). This fix expands the
  helpers and adds new clear-mode coverage, but doesn't change the
  convention.
- **Future: repeated-field presence.** Out of scope here. If we ever
  need "leave `media_ids` unchanged" as a distinct signal from "clear
  to empty," that's a wrapper-message proto change worth its own issue.
- **`SaveGearRequest.source_url` precedent.** Already correct; this fix
  brings every other field into alignment with that pattern.

## Audit Notes

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, and `docs/server/architecture.md`. Surgical edits made:

1. **Field-by-field gap table (`media_id` row):** Added a clarifier
   that the Dart client (`user_service.dart`) does not currently send
   the singular `media_id` scalar — avatar updates flow through the
   repeated `media_ids` field. The proto change is still worth
   shipping for consistency, but the Dart "clear avatar" UX path is
   the repeated-field path and stays out of scope. Without this
   note a reader could infer a Dart-side change is required for
   `media_id` that isn't.

2. **Phase 3 cache-invalidation note:** Added an explicit pointer to
   the existing `invalidate(userId)` and `invalidate(id)` calls in
   `user_repository.dart:112` and `gear_repository.dart:133`, plus
   the downstream search/feed/daily invalidations. This anchors the
   plan to the cache-invalidation convention in
   `docs/client/caching.md` § "Cache Invalidation" and confirms no
   new cache plumbing is needed for the clear path.

3. **Phase 3 `updatePreferredTimezone` callout:** Added a
   verification step for `user_repository.dart:129-137`, which
   already passes `''` and is therefore a previously-broken path
   that becomes correct as a side-effect of this fix. Worth
   verifying explicitly so the test list covers it.

4. **Phase 3 viewmodel-test misdirection:** The original plan asked
   viewmodel tests to assert wire-level request shape. That
   contradicts `docs/client/architecture.md` § "Repository Pattern"
   (viewmodels mock repositories; they don't see request objects).
   Replaced with service-layer tests under `app/test/services/`,
   which is where request construction is correctly tested. Added a
   note that viewmodel tests should only be touched if their mocks
   relied on the old "empty string dropped" behavior.

5. **Acceptance Criteria — architecture compliance bullets:** Added
   four explicit cross-references (proto separation, client data
   flow, cache invalidation, i18n) so that a reviewer can confirm
   each architectural rule is preserved without re-deriving it.

No structural rewrite. The recommended option (Option A), the
proto/api/models separation, and the wire-compatible upgrade story
were already correct; the edits above tighten the client-side
plumbing details and make the cache-invalidation expectation
explicit.
