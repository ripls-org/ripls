# Issue #1152: How change lend to giveaway?

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1152
**Filed:** 2026-04-16 | **Last activity:** 2026-04-25
**Labels:** user-feedback | **Assignees:** unassigned
**Priority:** P2 (triaged 2026-04-24 — product lens, meaningful for multiple personas)
**Context docs:** client/architecture.md ⚓, client/conventions.md ⚓, proto_conventions.md ⚓, users/personas.md ⚓, users/priorities.md ⚓, client/manage_menus.md, workflows/delta.md, workflows/giveaway.md, workflows/loan.md, client/testing.md, client/testing/semantics_identifiers.md, reviews/{api_design,error_handling,testing,accessibility}_review_prompt.md

## As-Built (implemented)

Shipped on `loan_giveaway_toggle`. Deviations from the plan above, and why:

- **RPC lives on `CommunityService`, not `GearService`.** All the gear-sharing
  machinery (`CommunityGear` junctions, `systemMessageWriter`, the bus, the
  `ShareGear`/`UpdateGearSharing`/`UnshareGear` siblings) is on CommunityService,
  and GearService is deliberately decoupled from it. `SetGearAvailability` sits
  next to its siblings there; still gear-wide, request `{gear_id, availability}`.
- **No CommunityEvent is emitted.** The plan said reuse `GEAR_SHARED`; verifying
  end-to-end showed `GEAR_SHARED` drives a feed card *and* a community-wide
  "shared an item" push, which would spam members on every toggle. The flip now
  emits only the gear-conversation anchor system message; the actor's client
  invalidates its own caches.
- **Guard rejection is a toast, not a fatal screen.** `gear_content_view` renders
  a full-screen error on any `GearState.error`, so the action deliberately does
  not set it — it rethrows and the toggle shows a toast (the specific in-progress
  hint on `FailedPrecondition`).
- **Client lock is best-effort; the server guard is authoritative.** The toggle
  pre-disables + hints from the loaded transfer context when present, but the
  owner-open-item-directly path doesn't always have it loaded; the server
  `FailedPrecondition` guard is the guarantee, verified by e2e.

Commits: proto + server RPC (`gear_availability_test.go`), client repo/viewmodel
+ toggle UI (`gear_sharing_mode_toggle.dart`, ARB, viewmodel tests), guard-surfacing
hardening, and `e2e/tests/workflows/gear-lend-to-giveaway.spec.ts` (2 tests, green).

## Problem Statement

A user (a community reporter, iOS PROD, app 0.14.0) shared a physical item — a
1000-piece Harry Potter puzzle — as a **loan** ("lend"), but actually wants to
**give it away** permanently ("You only want to do this 1000 piece puzzle
once!"). They found no way in the app to change an item's mode from lend to
giveaway after it was created. The issue title — *"How change lend to
giveaway?"* — is literally the question the app can't answer today.

This is a **UX gap / missing control**, not a misbehavior. The underlying data
and RPC already exist:

- **`CommunityGear.availability`** (`AVAILABILITY_FOR_LOAN` /
  `AVAILABILITY_FOR_GIVEAWAY`) is a **per-community-gear** setting stored on the
  `CommunityGear` junction. It's chosen at creation (the create preview's
  Lend/Give choice, plumbed through `SaveGearRequest.availability`) and is
  **"honored on insert only — after creation, per-community availability is
  managed via ShareGear"** (`gear_service.proto:61-66`).
- A server RPC **already exists** to change it post-creation:
  `CommunityService.UpdateGearSharing(gear_id, community_id, availability)`
  (`server/services/community/gear_sharing.go:365`). It checks owner + community
  membership, finds the `CommunityGear` junction, and flips
  `availability` — **but nothing in the app calls it**. The client only has the
  generated bindings.

So the fix is primarily **surfacing** an existing capability, plus closing three
real gaps in the existing RPC (see below). The user's steer — *"we can add the
toggle to the item details edit screen; we should add a playwright test
exercising the toggle"* — sets the surface (the gear edit pane) and a testing
requirement.

### Three gaps that make a naive toggle a bug

Reading `UpdateGearSharing` against the loan/giveaway workflows surfaces three
problems a straight wire-up would ship:

1. **No in-flight-transfer guard.** `UpdateGearSharing` flips availability
   unconditionally. But a loan can be **ACTIVE** (item physically out with a
   borrower) or have a **selected recipient** / **pending interest**. Giveaways
   skip the ACTIVE state entirely and transfer ownership permanently
   (`workflows/delta.md`). Flipping an item that's out on loan — or that has
   loan-typed interest transfers (`Transfer.transfer_type = LOAN`) sitting in
   `INTEREST_EXPRESSED`/`RECIPIENT_SELECTED` — to giveaway leaves the transfer
   state machine and the availability contradicting each other. This must be
   guarded, not just at the UI.

2. **No notification / conversation trace.** `shareGearToCommunity` emits a
   `GIVEAWAY_SHARED` / `LOAN_SHARED` system message into the gear conversation
   and publishes a `GEAR_SHARED` community event. `UpdateGearSharing` emits
   **neither**. A silent flip means: other members' feeds/caches don't refresh
   in realtime, and the perpetual gear conversation shows no record that the
   item became a giveaway. The conversation's own `availability` label
   (`conversation.proto:39`) is derived from the junction, so it will
   eventually reflect the change, but no one is told.

3. **Per-community divergence is semantically wrong for giveaways.**
   Availability lives per-`CommunityGear`. An item shared into three communities
   has three independent availability rows. In practice they're uniform (the
   create choice seeds the first, and every later `ShareItem` invite inherits it
   via `existingGearAvailability`). But `UpdateGearSharing` is per-community, so
   a toggle wired to the current community context would let one physical item
   be "giveaway here, loan there" — impossible for a permanent transfer. The
   flip should be **gear-wide**.

## Context from Comments

Three comments, all from the maintainer (maintainer), no reporter follow-up:

- **Duplicate consolidation:** #1235 was closed in favor of this issue —
  independent corroboration that users hit this.
- **Acknowledgement:** "tracked here. Will follow up when fix is in."
- **Triage (2026-04-24):** Priority set to **P2** on the product lens —
  "meaningful improvement for multiple personas."

No design direction was recorded in the comments; the design steer ("edit
screen toggle + playwright test") comes from the issue-plan invocation, not the
thread. There's no disagreement to resolve.

## UI Investigation

**Skipped — deliberately.** `ripls-ui-investigate` reproduces *UI-observable
misbehavior* and bisects it to a layer. Here nothing misbehaves: the control
simply doesn't exist. Reproducing "there is no lend/give toggle" yields no
diagnostic signal — there's no wrong render, misfired gesture, or stale state to
bisect. The relevant layers are already pinned by code reading: the capability
lives server-side (`UpdateGearSharing`, unused by the client), and the surface
is the owner-only gear edit pane (`GearEditPane`). The Playwright spec in Phase 3
is the right place to exercise the new behavior end-to-end.

## Potential Solutions

All three options put a **Lend / Give-away control in `GearEditPane`** (per the
steer). They differ in the **RPC contract** behind it and therefore in how the
gear-wide + guard + notification gaps get solved.

### Option A: Reuse per-community `UpdateGearSharing`, iterate on the client

- **Approach:** Add a client `updateGearSharing` method to `GearRepository`.
  The edit-pane toggle loops over `state.sharedCommunities`, calling the
  existing RPC once per community (mirroring `shareMultipleWithCommunity`).
  Guards + system-message + bus-event get added to the existing server RPC.
- **Pros:** No proto/RPC surface change; smallest server diff; the RPC already
  has an integration test to extend.
- **Cons:** N round-trips and **partial-failure states** across communities
  (item ends up half-giveaway) — exactly the client-side-orchestration kludge
  the "fix root cause on the server" convention warns against. The
  in-flight-transfer guard has to run per-call, so "block the whole flip if any
  community has an active loan" is awkward (some calls succeed before one
  fails). System message would fire once per community unless special-cased.
- **Effort:** Small–Medium.

### Option B: New gear-wide `SetGearAvailability(gear_id, availability)` RPC

- **Approach:** Add a dedicated RPC on `GearService` (dedicated request/response
  per `proto_conventions.md` — no reuse of `UpdateGearSharing`'s messages). The
  server, in one call: runs the in-flight-transfer guard **once** across all the
  gear's transfers; flips **every** `CommunityGear` junction for the gear
  atomically; emits **one** `LOAN_SHARED`/`GIVEAWAY_SHARED` system message on the
  gear's (single, per-gear) conversation; and publishes the availability-changed
  community event(s). Client calls it once from the toggle.
- **Pros:** Correct semantics (gear-wide, atomic, single source of truth); guard
  and notification live at the root; one client call, no partial failures; the
  contract says exactly what it does ("set this item's availability"). Leaves
  the per-community `UpdateGearSharing` intact for any future per-community need.
- **Cons:** New proto surface → regenerate Go + Dart; more server code and a new
  test file; two ways to change availability now coexist (acceptable — different
  scopes).
- **Effort:** Medium.

### Option C: Manage-menu action instead of an edit-pane toggle

- **Approach:** Add a "Switch to giveaway" / "Switch to lending" row to the
  item-level `GearManageMenuSheet` (`GearManageAction`), reusing the Option-B
  RPC. A confirmation dialog fits the one-shot-action pattern naturally.
- **Pros:** Consistent with the existing item-level owner surface (Set details /
  Set location / Close); confirmation is idiomatic there.
- **Cons:** The reporter's mental model is "change this setting," which a
  **toggle showing current state** communicates better than a hidden one-shot
  menu action; and the steer explicitly named the edit screen. Could be added
  later as a second entry point.

## Recommendation

**Option B** — a gear-wide `SetGearAvailability` RPC behind a **Lend / Give-away
segmented control in `GearEditPane`**, applied on tap with a **confirmation for
the loan→giveaway direction**, and **server-guarded against in-flight
transfers**.

Rationale:

- **It fixes the root cause, not the symptom.** Per-community divergence and
  partial-failure loops (Option A) are precisely the client-side workarounds the
  project's "fix on the server" and "prefer generic primitives" conventions push
  against. A single atomic gear-wide RPC is the honest primitive.
- **Guard + notification belong on the server.** The in-flight-transfer guard is
  a correctness invariant (an item on loan cannot become given-away); it must be
  enforced server-side regardless of which client calls it. The system message +
  event are the existing pattern for every other availability write.
- **The toggle matches the user's ask and mental model.** It shows the current
  mode and lets the owner flip it, in the surface they were told to expect.

Tradeoff accepted: a new RPC and codegen versus reusing the existing one. Given
that the reused path (Option A) can't cleanly express "all-or-nothing gear-wide
flip with a single guard + single notification," the new primitive is worth it.

If the reviewer wants the smallest possible footprint for a first cut, Option A
is viable — but only if we scope v1 to **single-community gear** (guard the
toggle off when `sharedCommunities.length > 1`) to sidestep the partial-failure
problem. That's a real product limitation and should be a conscious choice, not
a default.

## Open Questions

- **Gear-wide vs current-community-only flip?** — Recommendation: **gear-wide**.
  A permanent giveaway can't coherently be a loan in a sibling community, and
  availability is already uniform in practice. Option B makes gear-wide the
  natural, atomic behavior.
- **What states block the flip?** — Recommendation: block when the gear has
  **any non-terminal transfer** (`INTEREST_EXPRESSED`, `RECIPIENT_SELECTED`, or
  `ACTIVE`) in any community, returning a user-visible Connect error
  (`FailedPrecondition`) with copy like *"Return or cancel the active loan
  before switching to giveaway."* The client disables the toggle and shows the
  reason when `gearDetails.activeLoan != null` or the interest count > 0.
  Auto-migrating loan-typed interest transfers to giveaway type is a possible
  future enhancement; for v1, block-and-explain is safer and simpler.
- **Apply-on-tap or apply-on-Save?** — Recommendation: **apply-on-tap**, because
  availability is a distinct RPC (not part of `SaveGear`, which ignores
  availability post-insert) and the flip is consequential + visible to other
  members. Pair it with a **confirmation dialog for loan→giveaway** ("Giving
  away transfers ownership permanently…") and an **undo toast**
  (`ToastHelper.showUndo`) for the reverse. Giveaway→loan needs no confirm.
- **Reuse existing system-message actions or add a new one?** —
  Recommendation: **reuse** `CHAT_SYSTEM_ACTION_LOAN_SHARED` /
  `CHAT_SYSTEM_ACTION_GIVEAWAY_SHARED` (already localized in the server catalog)
  rather than minting a new "availability changed" action — the anchor copy ("X
  shared this for giveaway") reads correctly for a mode change and avoids new
  server l10n.
- **RPC name for the gear-wide flip?** — Recommendation: **`SetGearAvailability`**
  on `GearService`. The Get/List/Create/Update/Delete verb convention would
  suggest `Update`, but `UpdateGearSharing` already exists at the *per-community*
  scope; `Set…` reads as an idempotent set-to-value at the *gear-wide* scope and
  keeps the two contracts distinct. Reviewer may prefer `UpdateGearAvailability`
  — either is acceptable; the scope distinction from `UpdateGearSharing` matters
  more than the verb.
- **Where does the toggle sit in the edit pane?** — Recommendation: a segmented
  Lend/Give control near the top of `GearEditPane` (above the metadata form),
  owner-only (edit mode already is), disabled with a hint when guarded. Consider
  a second entry point via `GearManageMenuSheet` (Option C) as a fast follow.

## Implementation Plan

### Phase 1: Server — safe, gear-wide, observable availability change
**Goal:** One RPC that flips an item's availability everywhere it's shared,
guarded and announced, with the correctness invariant enforced server-side.
**Tasks:**
- [ ] Add `SetGearAvailability` to `GearService` (proto): dedicated
  `SetGearAvailabilityRequest { string gear_id; Availability availability; }`
  and empty response (`proto_conventions.md`: no cross-RPC message reuse).
- [ ] Implement it (likely in `server/services/gear/` calling into the
  community gear-sharing helpers, or in `server/services/community/`): verify
  owner; enumerate the gear's `CommunityGear` junctions; **guard** — reject when
  any `Transfer` for the gear is in a non-terminal state (`INTEREST_EXPRESSED`,
  `RECIPIENT_SELECTED`, `ACTIVE`; `COMPLETED`/`CANCELLED` are terminal and don't
  block); update every junction's availability; emit the reused
  `LOAN_SHARED`/`GIVEAWAY_SHARED` system message once on the gear conversation;
  publish the community event(s) so caches/feeds refresh.
- [ ] **Reuse the existing `COMMUNITY_EVENT_TYPE_GEAR_SHARED` event** for the
  cache/feed refresh — do **not** mint a new event type. A new `CommunityEventType`
  value would require classification in **both** `server/undo/registry.go` and
  `server/activity_digest/registry.go` or the build fails (`server/conventions.md`
  → "Adding New CommunityEventType Values"). GEAR_SHARED already means "this gear's
  sharing changed," which fits.
- [ ] **Guard error through `connecterr.UserVisible`**, not a raw Connect error:
  `connecterr.UserVisible(ctx, connect.CodeFailedPrecondition,
  "gear_availability_change_blocked_active_transfer", "<fallback copy>", nil)` —
  matching the existing `gear_owner_required_for_share` pattern so the client can
  render a localized message. Log the rejection at **Warn/Info, not Error** — it's
  an expected client-precondition, not a server fault ("keep ERROR logs loud for
  real client/data bugs" cuts both ways). Log the successful flip at Info with
  masked email + gear_id, as `UpdateGearSharing` already does.
- [ ] Keep the existing `UpdateGearSharing` per-community RPC as-is (still valid
  for any per-community need); optionally add its own guard for consistency.
- [ ] Tests (extend/parallel `gear_sharing_update_test.go`): owner flips a
  single-community loan → giveaway (junction updated, system message written,
  event published); **multi-community** gear flips all junctions atomically;
  **guard** rejects when an ACTIVE loan / selected recipient / pending interest
  exists; non-owner denied; unknown gear → NotFound. Run
  `go test ./server/services/... -count=1` (unit + `-tags=integration`).
**Validates:** The core capability is correct and safe independent of any UI —
you can flip lend↔giveaway via RPC and see the guard + notification fire.

### Phase 2: Client — Lend/Give toggle in the gear edit pane
**Goal:** The owner can flip the item's mode from the edit screen.
**Tasks:**
- [ ] Add `setGearAvailability(gearId, availability)` to `GearRepository`
  (calling the new RPC) with cache invalidation (gear, search, feed) mirroring
  `shareMultipleWithCommunity`'s invalidation set.
- [ ] Add a `GearNotifier` action (`setAvailability`) that calls the repo, then
  `loadGearDetails(communityId: state.communityId)` to refresh
  `gearDetails.availability`; surface guard errors as a `UserError`.
- [ ] Add a Lend/Give segmented control to `GearEditPane` bound to
  `state.gearDetails.availability`, owner-only, **disabled with a hint** when
  guarded (`activeLoan != null` or interest count > 0). Accessibility (per
  `client/accessibility.md` + the accessibility review checklist): build on the
  `Toggle` a11y primitive; give it an **l10n-sourced `semanticsLabel`** (not
  "button 1") plus a stable kebab-case `semanticsIdentifier` (e.g.
  `gear-availability-toggle`) so the e2e harness can find it
  (`client/testing/semantics_identifiers.md`); convey Lend vs Give by **text
  label, never color alone**; keep the touch target ≥48×48; and **announce the
  guard hint to screen readers** (LiveRegion / `SemanticAnnouncer`) rather than
  showing it visually only.
- [ ] Confirmation dialog for loan→giveaway; undo toast for giveaway→loan.
- [ ] New ARB keys (client renders in-app per the l10n render boundary): segment
  labels ("Lend" / "Give away"), confirm dialog copy, guard hint. Run
  `npm run generate` + the i18n/parity lint gates.
- [ ] ViewModel unit tests (Riverpod `ProviderContainer` + Mockito): toggle
  calls the repo with the right availability and refreshes; guarded state
  disables the control; error path sets `UserError`.
**Validates:** The reporter's exact task — open the item, switch it from lend to
give-away — works in the app.

### Phase 3: Playwright e2e — exercise the toggle end-to-end
**Goal:** Lock the behavior against regression through the real Flutter Web
bundle (the testing requirement in the steer).
**Tasks:**
- [ ] Add `e2e/tests/workflows/gear-lend-to-giveaway.spec.ts`, modeled on
  `gear-authoring-share.spec.ts`: seed a host + community + a gear shared
  `FOR_LOAN` (RPC seed); open the gear in the web UI; enter edit mode; tap the
  "Give away" segment (locate by `semanticsIdentifier`, `getByRole` not
  `getByLabel`); confirm the dialog; assert **RPC truth** via
  `GetGear` that `availability == AVAILABILITY_FOR_GIVEAWAY`, and assert the
  in-UI badge/label updates. `test.skip` on webkit per the existing convention.
- [ ] Add a guard assertion: a gear with an active loan shows the toggle
  disabled (or the flip is rejected) — proving the Phase-1 invariant reaches the
  UI. Avoid hollow `toHaveCount(0)` assertions (see testing conventions).
- [ ] Build the e2e bundle with `npm run build:e2e` and run the spec against the
  hermetic stack.
**Validates:** The whole loop — seed loan → toggle → giveaway — holds through
the integration boundary, and the safety guard is observable to users.

### Phase 4 (optional fast-follow): second entry point + docs
**Goal:** Discoverability + keep docs true to code.
**Tasks:**
- [ ] Consider a "Switch to giveaway/lending" row in `GearManageMenuSheet`
  (Option C) reusing the same RPC/action.
- [ ] Update `docs/client/manage_menus.md` and `docs/workflows/{loan,giveaway}.md`
  to document post-creation availability change; refresh freshness stamps.
**Validates:** Owners find the control from more than one place; the durable
docs describe the new capability.

## Related Issues
- **#1235** — closed as a duplicate of this issue (same "change lend→giveaway"
  need); corroborates demand.
- **#2492 (epic)** — per-item community + `SaveGearRequest.availability` create
  choice; this issue extends that plumbing to be editable after creation.
- **#2702 / #2708** — request→loan morph and event child transfers; adjacent
  work on the transfer state machine that the in-flight-transfer guard must
  stay consistent with.

## Audit Notes

Audited against the documentation context map (`/ripls-audit`). Engaged
dimensions: api_design (new RPC), error_handling (guard codes), accessibility
(edit-pane toggle), observability (event/log), testing, plus the proto,
server, and workflow conventions.

Fixes applied to the plan as a result of the audit:

1. **CommunityEventType registry gate** — the original "publish the
   availability-changed event(s)" wording invited a new enum value, which would
   fail the build without entries in *both* `server/undo/registry.go` and
   `server/activity_digest/registry.go`. Phase 1 now mandates **reusing
   `COMMUNITY_EVENT_TYPE_GEAR_SHARED`**.
2. **User-visible guard error** — the guard now returns via
   `connecterr.UserVisible(… CodeFailedPrecondition, "<l10n_key>" …)` (matching
   the existing `gear_owner_required_for_share` pattern) instead of a raw Connect
   error, and logs the rejection at Warn/Info (expected precondition), not Error.
3. **Accessibility requirements** — Phase 2 now specifies an l10n-sourced
   `semanticsLabel`, text-not-color mode indication, ≥48×48 touch target, and a
   screen-reader announcement for the guard hint — not just the e2e
   `semanticsIdentifier`.
4. **RPC naming decision** — added as an open question (`SetGearAvailability`
   vs `UpdateGearAvailability`), noting the scope distinction from the existing
   per-community `UpdateGearSharing`.

Docs consulted: proto_conventions.md ⚓; reviews/api_design_review_prompt.md ⚓,
reviews/error_handling_review_prompt.md ⚓, reviews/accessibility_review_prompt.md ⚓;
server/conventions.md ⚓ (CommunityEventType undo/digest-registry gate),
client/conventions.md ⚓, client/architecture.md ⚓; client/manage_menus.md,
client/accessibility.md, workflows/delta.md, workflows/{loan,giveaway}.md,
client/testing.md, client/testing/semantics_identifiers.md. Cross-checked
against the live code: `server/services/community/gear_sharing.go`
(`UpdateGearSharing`, `shareGearToCommunity`), `gear_sharing_update_test.go`,
`gear_service.proto` / `community_service.proto` availability fields,
`gear_view_model.dart` (`GearState`, save flow), `gear_edit_pane.dart`,
`gear_repository.dart`, and `e2e/tests/workflows/gear-authoring-share.spec.ts`.
