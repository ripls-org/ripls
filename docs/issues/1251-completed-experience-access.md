## Problem Statement

When a user completes an experience, the client UI shows the access state as empty: the access ring displays 0 and the access sheet says the experience is "not shared with any communities." The underlying sharing relationships have not been revoked — the experience is still shared with the same communities it was shared with before completion. The problem is purely in how the server reports the sharing state after completion.

## Context from Comments

No comments on the issue. Issue is newly filed and reflects direct observation by the reporter. Flagging this in Open Questions in case there's additional context (e.g., whether the product intent is to *hide* completed experiences from the access sheet, in which case this is a spec question rather than a bug).

## Root Cause

The server has a deliberate double-meaning collision on `CommunityExperience.Archived`:

1. **On completion**, [lifecycle.go:368-381](server/services/experience/lifecycle.go#L368-L381) flips `Archived = true` on every `CommunityExperience` row for the experience. The stated intent is *"experience no longer appears in feed"*. Cancellation does the same at [lifecycle.go:538-557](server/services/experience/lifecycle.go#L538-L557).

2. **On read**, [get.go:83-85](server/services/experience/get.go#L83-L85) filters archived rows out of the `SharedCommunity` list that powers the access ring and access sheet:
   ```go
   if ce.Archived {
       continue
   }
   ```

So the `SharedCommunities` field returned to the client is empty for every completed (or cancelled) experience, which is exactly what the access ring and sheet render.

User-initiated unshare is not involved here — [unshare.go:62](server/services/experience/unshare.go#L62) physically `Delete`s the `CommunityExperience` row rather than archiving it. `Archived = true` is only ever set for terminal-state transitions (completion, cancellation). That means "archived" and "should not appear in access UI" are orthogonal concepts that the current code conflates.

### Key file references

- Server archives on complete: [lifecycle.go:368-381](server/services/experience/lifecycle.go#L368-L381)
- Server archives on cancel: [lifecycle.go:538-557](server/services/experience/lifecycle.go#L538-L557)
- Server filters on read: [get.go:70-130](server/services/experience/get.go#L70-L130)
- User-unshare physically deletes: [unshare.go:62](server/services/experience/unshare.go#L62)
- Proto field: [experience_service.pb.dart:1974-1977](app/lib/data/gen/ripls/api/experience_service.pb.dart#L1974-L1977)
- Client access ring reads `state.sharedCommunities`: [experience_content_view.dart:236-248](app/lib/presentation/screens/experience/experience_content_view.dart#L236-L248)
- Client access sheet groups: [experience_content_view.dart:302-331](app/lib/presentation/screens/experience/experience_content_view.dart#L302-L331)
- Client refresh after completion: [event_modal_view_model.dart:758-808](app/lib/presentation/viewmodels/event_modal_view_model.dart#L758-L808)

## Potential Solutions

### Option A: Stop filtering archived rows in `populateExperienceSharedCommunities`

- **Approach:** Remove the `if ce.Archived { continue }` filter in [get.go:83-85](server/services/experience/get.go#L83-L85). `Archived` continues to mean "hide from community feed"; the access-UI query now includes all non-deleted `CommunityExperience` rows.
- **Pros:** Smallest possible change. The field `SharedCommunities` goes back to meaning what its proto comment already promises ("All communities this experience is shared with. Populated on every GetExperience call for the community-switcher dropdown."). Feed-hiding behavior is untouched because feed queries filter on `Archived` independently. Unshare still works correctly because it physically deletes, not archives.
- **Cons:** Leaves `Archived` with a subtly-overloaded meaning (terminal-state-reached, not actually-removed). Any future caller of `populateExperienceSharedCommunities` that wants "currently active communities only" would need to re-introduce filtering.
- **Effort:** Small.

### Option B: Stop setting `Archived = true` on completion/cancellation; filter feeds on `experience.state` instead

- **Approach:** Remove the archival loops in both `lifecycle.go` sites. Update every feed query that currently relies on `CommunityExperience.Archived = false` to instead check `Experience.State != COMPLETED AND State != CANCELLED` via JOIN.
- **Pros:** Single source of truth for terminal state (on the `Experience` row). Eliminates the overloaded flag.
- **Cons:** Much larger blast radius — every feed query needs auditing and updating. Risks regressing feed behavior. Adds a JOIN to hot-path feed queries where the `Archived` flag was specifically denormalized for read speed. Harder to reason about rollback if something breaks.
- **Effort:** Large.

### Option C: Track the reason for archival

- **Approach:** Add `archive_reason` to `CommunityExperience` (enum: TERMINAL_STATE, some_future_reason). Update the access-UI query to include archived-for-terminal-state rows. Feed queries continue to filter on `Archived` regardless of reason.
- **Pros:** Explicit semantics, future-proof if more archive reasons appear.
- **Cons:** Schema change and proto change for a bug with no second reason currently on the horizon. Over-engineering for the observed issue. CLAUDE.md's "minimal dependencies / avoid over-engineering" principle argues against this.
- **Effort:** Medium.

## Recommendation

**Option A.** It's a one-line fix that restores the behavior the proto comment already documents. The current bug is a straightforward symptom of the filter being too aggressive, not of a missing abstraction. If a future requirement introduces a second archival reason where we *do* want to hide a community from the access UI, that's the time to introduce Option C — not now.

The critical sanity check before landing: there must not be any existing product expectation that the access ring goes to 0 when an experience is completed. If there is, this is a spec dispute rather than a bug (see Open Questions).

## Open Questions

- **Is hiding the communities in the access UI actually intentional post-completion?** — Recommendation: treat it as unintentional. The proto comment on `SharedCommunities` says "All communities this experience is shared with. Populated on every GetExperience call for the community-switcher dropdown." That contract makes no exception for completed experiences, and the reporter's observation is that the current behavior is wrong. Confirm with reporter before shipping; update the proto comment if product decides otherwise.
- **Should cancelled experiences also show sharing state?** — Recommendation: yes, same as completed. Users need to know which communities a cancelled experience was shared with, and the symmetry with completion keeps the mental model simple. The fix in Option A naturally covers both because both completion and cancellation use the same archival path.
- **Does the community-switcher dropdown (which uses the same `SharedCommunities` field) have any regression risk?** — Recommendation: low risk, but verify manually. After the fix, the switcher will show communities for completed/cancelled experiences too, which is consistent with letting the owner still browse their own completed experiences by community context. Audit the dropdown's empty-state handling once.
- **Are there tests asserting that `SharedCommunities` is empty after completion?** — Recommendation: grep `share_test.go`, `delete_test.go`, and `undo_roundtrip_test.go` for assertions that would flip. If such tests exist, they encode the bug — update them to assert the new correct behavior and add a regression test.

## Implementation Plan

### Phase 1: Fix the server read filter

**Goal:** Restore `SharedCommunities` to include all non-deleted `CommunityExperience` rows so the access ring and access sheet show the true sharing state after completion or cancellation.

**Tasks:**
- [ ] In [server/services/experience/get.go](server/services/experience/get.go), `populateExperienceSharedCommunities`, remove the `if ce.Archived { continue }` skip at lines 83-85. Include a short code comment explaining that `Archived` governs feed visibility, not access-sheet visibility.
- [ ] Grep the experience service for other callers of `populateExperienceSharedCommunities` (or that iterate `CommunityExperience` rows with an `Archived` filter intended for the same UI concern) and adjust them consistently.
- [ ] Search `server/services/experience/*_test.go` for tests that assert empty `SharedCommunities` after completion/cancellation. If found, flip the assertion to expect the communities to still be present.
- [ ] Add a regression test in `server/services/experience/` that: creates an experience shared with 2+ communities, completes it, calls `GetExperience`, and asserts `SharedCommunities` has the original communities (by ID and by count). Add the symmetric test for `CancelExperience`. Use `storage.SetupTestStorage(t)` (same pattern as sibling tests in the package) so the test runs in the default tier, not behind the `integration` build tag — this keeps the regression guard in the fast local suite per CLAUDE.md's testing guidance.
- [ ] Run `npm run build` (lint + `go build`) and `go test ./...` per CLAUDE.md. The `integration` build tag is not needed for these tests since they use testcontainers via `SetupTestStorage`, not live external APIs.

**Validates:** Server side is correct. The RPC contract now matches the proto-level documentation. Client needs no changes if the server returns the right data, because the client already renders whatever `sharedCommunities` it gets.

**Why Option A is safe (audit confirmation):** User-initiated unshare physically `Delete`s the `CommunityExperience` row ([unshare.go:62](server/services/experience/unshare.go#L62)) rather than setting `Archived = true`. That means `Archived` currently only ever flips true for terminal-state transitions (completion, cancellation) — never for revocation — so including archived rows in the access-UI query cannot accidentally surface revoked sharing relations.

### Phase 2: Client verification

**Goal:** Confirm the UI renders correctly end-to-end on a completed experience. No Flutter code changes are expected.

**Tasks:**
- [ ] Verify cache invalidation flow. The existing [experience_repository.dart:326-348](app/lib/data/repositories/experience_repository.dart#L326-L348) already calls `invalidatePattern('experience:$id*')` after `completeExperience()`, matching the repository-owns-caching rule in [docs/client/caching.md](docs/client/caching.md). Confirm (via manual run or an existing test) that the refresh at [event_modal_view_model.dart:808](app/lib/presentation/viewmodels/event_modal_view_model.dart#L808) triggers a fresh `GetExperience` fetch rather than returning cached (stale) data.
- [ ] Smoke-test on one platform: complete an experience shared with 2+ communities, confirm the access ring shows the correct count and the access sheet lists the communities. Repeat for cancellation.
- [ ] No new i18n strings are introduced. The existing "not shared with any communities" string in the access sheet continues to serve the genuine empty-sharing case (an experience that actually has zero `CommunityExperience` rows). No ARB file changes required.
- [ ] No Flutter code changes expected. If the UI still shows 0 after the server fix, re-investigate (likely a client-cache or state-propagation issue in the view model, not the server).

**Validates:** Bug is fully resolved from the user's perspective. The client follows the architecture rule in [docs/client/architecture.md](docs/client/architecture.md) — the server is the authoritative source for `sharedCommunities`, and the client renders it.

### Phase 3: Optional — clarify `Archived` semantics

**Goal:** Prevent this class of bug recurring by documenting what `Archived` means.

**Tasks:**
- [ ] Add a short comment to the `CommunityExperience.Archived` field in `proto/ripls/models/` (not the API proto — this is a storage concept, and the API/models separation rule in CLAUDE.md says these packages must never import each other) explaining that it governs community-feed visibility only, not the authoritative "is this shared" relation. The authoritative relation is presence of the row itself, and physical deletion (per [unshare.go:62](server/services/experience/unshare.go#L62)) is what revokes sharing.
- [ ] Optional: rename the flag to `HiddenFromFeed` in a follow-up if the team has appetite; requires a migration and is out of scope for this fix.

**Validates:** Future contributors can read the field and know which queries should filter on it.

## Related Issues

- None surfaced in the comments on #1251. If there are prior bug reports about completed-experience UI, they should be linked in the PR.

---

## Audit Notes (2026-04-23)

Audited against [docs/client/architecture.md](docs/client/architecture.md), [docs/client/caching.md](docs/client/caching.md), [docs/client/i18n.md](docs/client/i18n.md), and [docs/server/architecture.md](docs/server/architecture.md). Changes applied:

1. **Phase 1 test tier clarified.** Explicitly called out that the regression test uses `storage.SetupTestStorage(t)` and runs in the default tier (no `integration` build tag), matching the sibling tests in `server/services/experience/`. This aligns with CLAUDE.md's guidance that tests hitting live external APIs are gated behind the `integration` tag — testcontainers-based tests aren't.
2. **Phase 1 "why Option A is safe" callout added.** Cross-referenced `unshare.go:62` to make explicit that `Archived` never conflates with user-initiated revocation. This addresses the main risk of Option A.
3. **Phase 2 cache invalidation explicitly cross-referenced to [docs/client/caching.md](docs/client/caching.md).** The existing `invalidatePattern('experience:$id*')` call follows the namespace:identifier convention documented there; repository owns the invalidation, viewmodel does not.
4. **Phase 2 i18n note added.** Confirmed no new strings are introduced and the existing "not shared" string remains valid for genuine empty-sharing cases, so no ARB file work per [docs/client/i18n.md](docs/client/i18n.md).
5. **Phase 3 model/API separation rationale added.** Cited CLAUDE.md's rule that `proto/ripls/models/` and `proto/ripls/api/` must never import each other to explain why the comment goes on the models side.

No architectural contradictions found. The recommended fix (Option A) is aligned with all four reference documents: it preserves the RPC contract in the proto comment, respects the repository-caching boundary on the client, requires no i18n work, and stays within the experience service package on the server (no cross-service coupling introduced).
