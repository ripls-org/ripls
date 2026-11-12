# Issue #1160: [Bug Report] Coffee at Juniper Goods

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1160
**Filed:** 2026-04-17 | **Last activity:** 2026-04-17
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** docs/client/caching.md

## Problem Statement

Three bugs reported around experience RSVP behavior on iOS (v0.15.0, DEV):

1. **Missing inbox thumbnail** - An experience appears in the inbox with a missing thumbnail image. The user identifies this as a caching issue.
2. **RSVP not reflected in attendee list** - After tapping "Yes" in the RSVP modal, the user did not appear as an attendee in the responses section.
3. **Duplicate RSVP system messages** - Tapping the RSVP button multiple times produced multiple "X is attending" system chat messages, even though the RSVP record itself is correctly deduplicated on the server.

## Context from Comments

No comments on this issue. All context comes from the description.

## Analysis

### Bug 1: Inbox thumbnail missing

Two inbox views display experience thumbnails:

- **`inbox_row.dart:101-116`** - Uses `mediaAsync.value?.url ?? ''` which evaluates to empty string during loading and error states, silently falling through to the generic fallback icon. This differs from the correct `.when()` pattern used in `inbox_action_card.dart:161-176` and `daily_emoji_thumbnail.dart:43-57`.
- **`inbox_action_card.dart:155-178`** - Correctly uses `.when()` to handle loading/data/error states.

The `inbox_row.dart` pattern has two problems:
1. During loading, the thumbnail shows the generic inventory icon instead of a loading placeholder, causing a flash of wrong content.
2. If `mediaObjectProvider` errors (e.g., presigned URL generation fails), the error is silently swallowed and the fallback icon is permanently shown with no retry.

Additionally, if the experience has no `media_ids` (the `background_media_id` field is empty), both views correctly fall through to their respective fallbacks. This is expected behavior, not a bug, but the experience thumbnail fallback for `inbox_row.dart` shows a generic inventory icon (`Icons.inventory_2_outlined`) rather than an experience-appropriate fallback (e.g., the emoji or indigo-tinted background used by `daily_emoji_thumbnail.dart`).

### Bug 2: RSVP not showing as attendee

The RSVP submission flow in `rsvp_modal.dart:192-200`:
1. User taps a button -> `onChanged` fires
2. `setState(() => _selectedIntention = intention)` updates the toggle button immediately
3. `submitRsvp(intention, communityId)` is called
4. In `event_modal_view_model.dart:385`, `isLoading: true` is set
5. This causes the **entire modal body** to be replaced with a `CircularProgressIndicator` (`rsvp_modal.dart:121-123`)
6. After `_refreshExperience()` completes, the modal body rebuilds with updated RSVPs

The modal should show the user as attending after refresh because:
- Server correctly creates/updates the RSVP record (`rsvp.go:97-125`)
- Repository invalidates the cache (`experience_repository.dart:294-295`)
- `_refreshExperience()` fetches fresh data and updates `state.rsvps`

**Most likely cause**: The `isLoading: true` replaces the entire body with a spinner. If the user doesn't wait for the refresh to complete (or the refresh is slow/fails), they won't see themselves in the list. There's also no optimistic update — the user's RSVP isn't added to `state.rsvps` immediately; it waits for a full server round-trip.

Additionally, the `RSVPToggleButtons` widget has **no guard against re-tapping the same selection**. Tapping "Yes" when already selected as "Yes" triggers another full RSVP submission + refresh cycle unnecessarily.

### Bug 3: Duplicate system messages

The server creates a system chat message on **every** call to `RSVPToExperience` (`rsvp.go:197-220`), regardless of whether the intention actually changed. The RSVP *record* is correctly deduplicated (lines 97-125 update rather than insert), but the system message and community event are always written.

This means:
- Tapping "Yes" three times produces three "X is attending" messages
- Even changing from "Yes" to "Yes" (same intention) writes a new message

The same issue applies to the community event recording (`rsvp.go:193-195`) and the watch/unread marking.

## Potential Solutions

### Option A: Fix all three bugs independently

- **Approach:**
  1. Fix `inbox_row.dart` to use the `.when()` pattern and add an experience-appropriate fallback
  2. Add optimistic RSVP update to the modal and prevent re-submitting the same intention
  3. Add server-side guard to skip system messages when intention hasn't changed
- **Pros:** Each fix is small, focused, and independently testable. Addresses root causes.
- **Cons:** Three separate changes to review.
- **Effort:** Small-medium.

### Option B: Client-only fixes (skip server change for messages)

- **Approach:** Fix bugs 1 and 2 on the client. For bug 3, prevent duplicate taps on the client side only (disable button while loading, don't re-submit same intention).
- **Pros:** No server deployment needed. Faster to ship.
- **Cons:** Doesn't fix the root cause of duplicate messages — any API client or race condition could still produce them. Other API consumers (future web client, etc.) would hit the same issue.
- **Effort:** Small.

### Option C: Full solution with client + server guards

- **Approach:** All of Option A, plus add a unique constraint or idempotency check at the database level to prevent duplicate system messages.
- **Pros:** Belt-and-suspenders protection at every layer.
- **Cons:** Database constraint for system messages is overkill; the simpler server-side if-check is sufficient since the RSVP record itself is already deduplicated.
- **Effort:** Medium.

## Recommendation

**Option A** — Fix all three bugs at their root cause with minimal changes:

1. Client: Fix `inbox_row.dart` thumbnail rendering
2. Client: Guard against re-submitting same RSVP intention in toggle buttons + add optimistic update
3. Server: Skip system message + community event when intention hasn't changed

This addresses all reported symptoms without over-engineering. The server-side guard is important because the client can't prevent all duplicate submissions (network retries, multiple devices, etc.).

## Open Questions

- **Is the missing thumbnail specific to experiences without media, or experiences that have media but it fails to load?** — Recommendation: Fix both cases. Update `inbox_row.dart` to use `.when()` (fixes failed loads) and consider an experience-appropriate fallback icon/color (fixes no-media case). The user can confirm which case applies.
- **Should RSVP submission show a loading spinner over the entire modal body?** — Recommendation: No. Set a more targeted loading state (e.g., disable the buttons and show a small spinner) instead of replacing the entire body. This also addresses bug 2 since the user can still see the responses list.

## Implementation Plan

### Phase 1: Fix duplicate RSVP system messages (server)

**Goal:** Prevent duplicate system chat messages when the user re-submits the same RSVP intention.

**Tasks:**
- [ ] In `server/services/experience/rsvp.go`, after determining whether this is a create or update (lines 97-125), track whether the intention actually changed
- [ ] Skip the system message write (lines 197-220) when the intention is unchanged
- [ ] Skip the community event recording (lines 193-195) when the intention is unchanged
- [ ] Add unit test: RSVP with same intention twice should produce only one system message
- [ ] Add unit test: RSVP changing intention (Yes -> No -> Yes) should produce three system messages
- [ ] Run `npm run build` to verify lint + compile

**Validates:** System messages are only created on actual intention changes.

### Phase 2: Fix RSVP modal UX (client)

**Goal:** Prevent duplicate RSVP submissions and show RSVP status immediately.

**Tasks:**
- [ ] In `rsvp_toggle_buttons.dart`, skip `onChanged` callback when tapping the already-selected intention
- [ ] In `event_modal_view_model.dart:submitRsvp`, skip the API call if the intention matches `state.currentUserIntention` (belt + suspenders)
- [ ] Add a dedicated `isSubmittingRsvp` field to `EventModalState` (Freezed class) so RSVP submission does not replace the entire modal body with a spinner. The existing `isLoading` field gates the full-body loading indicator; RSVP submission should only disable the toggle buttons. Run `npm run generate` after modifying the Freezed state class.
- [ ] In `rsvp_modal.dart`, use `state.isSubmittingRsvp` to disable the RSVP buttons instead of checking `state.isLoading`. Keep the responses list visible during submission.
- [ ] Add an optimistic RSVP entry to `state.rsvps` immediately in `submitRsvp()`, following the documented Optimistic Updates pattern in `docs/client/architecture.md` (construct a temp `RSVP` API proto with current user info and the selected intention; replace with server data on refresh). The client must only use `ripls/api` proto types, not `ripls/models`.
- [ ] If any new user-visible strings are introduced (button labels, error messages), add them to both `app_en.arb` and `app_es.arb` per i18n guidelines, and run `npm run generate`.
- [ ] Run `flutter test --reporter expanded` for relevant viewmodel tests

**Validates:** User sees themselves in the attendee list immediately after tapping. No duplicate submissions on rapid taps.

### Phase 3: Fix inbox thumbnail (client)

**Goal:** Show experience thumbnails correctly in the inbox row view.

**Tasks:**
- [ ] In `inbox_row.dart:_buildPhotoContent`, refactor to use `.when()` pattern matching the approach in `inbox_action_card.dart:161-176`. Use `CachedMediaImage` with `ImageCacheKeys.thumbnail(mediaId)` — consistent with the Image Cache Keys section of `docs/client/caching.md`.
- [ ] Verify that the fallback for experiences shows an appropriate visual (not the generic inventory icon). Use `AppColors` theme-aware helpers for any fallback colors — no hardcoded hex or `Colors.white`/`Colors.grey` (per `docs/client/architecture.md` theming rules).
- [ ] Add or update widget test for `InboxRow` with experience items. Use `ProviderContainer` with mock overrides for `mediaObjectProvider` per the testing patterns in `docs/client/architecture.md`.

**Validates:** Experience thumbnails load correctly in the inbox and show appropriate fallbacks.

## Related Issues

- #665 — Cross-community RSVP leakage (related RSVP deduplication logic)
- #920 — RSVP realtime updates (related to RSVP UX feedback)
- #921 — Thumbnail not showing (related inbox thumbnail issue)
- #925 — System message not showing (related system message infrastructure)

---

## Audit

**Audited against:** `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`

**Changes made:**

1. **Phase 2 — Freezed state field**: Added explicit task to introduce `isSubmittingRsvp` in `EventModalState` instead of reusing `isLoading`. The existing `isLoading` gates the full-body spinner (architecture pattern: loading states should be granular per operation). Added reminder to run `npm run generate` after changing the Freezed class.

2. **Phase 2 — Optimistic update pattern**: Referenced the documented Optimistic Updates pattern from `docs/client/architecture.md` (temp entry → replace on server response). Added reminder that the client must only use `ripls/api` proto types, not `ripls/models` (per CLAUDE.md API/model separation rule).

3. **Phase 2 — i18n**: Added task to localize any new user-visible strings via ARB files per `docs/client/i18n.md`.

4. **Phase 3 — Caching best practices**: Added explicit reference to `ImageCacheKeys.thumbnail()` and `CachedMediaImage` usage per `docs/client/caching.md` Image Cache Keys section.

5. **Phase 3 — Theme-aware colors**: Added reminder to use `AppColors` theme-aware helpers for fallback visuals — no hardcoded hex or light-only colors (per `docs/client/architecture.md` Colors and Theming rules).

6. **Phase 3 — Test pattern**: Added specifics about using `ProviderContainer` with mock provider overrides per the architecture doc testing patterns.

**No compliance issues found with:**
- Server architecture (Phase 1 changes are correctly scoped within the experience service package, follows unit test patterns)
- Service independence (no cross-service dependencies introduced)
- SQL efficiency (no new queries added)
- Cache invalidation patterns (existing invalidation in `experience_repository.rsvp()` is comprehensive)
