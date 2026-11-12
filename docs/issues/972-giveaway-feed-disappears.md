# Issue #972: Active giveaways sometimes disappear from the top of the feed

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/972
**Filed:** 2026-04-01 | **Last activity:** 2026-04-03
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

Active giveaways disappear from the giver's feed after being viewed and going stale (24h with no new activity). This happens because the feed freshness logic treats giveaway events the same as regular loan events, even though giveaways are ephemeral time-limited opportunities — just like experiences and requests.

A secondary symptom: when the giveaway disappears from the feed, the user may try to find it via their profile, where it can appear labeled as a loan (since `PortfolioItemCard` uses the transfer record to determine status, and the portfolio filter chips only distinguish "On Loan" and "Available" — there's no "Giveaway" filter).

## Context from Comments

The issue author (tescobar) identified this as related to #941, where experiences and requests were exempted from the staleness expiry via `isEphemeralEvent()`. The fix for #941 added an exemption for `REQUEST_CREATED`, `REQUEST_OFFER_MADE`, `EXPERIENCE_CREATED`, and `EXPERIENCE_RSVP_*` event types but did not cover giveaways. The comment explicitly states giveaways should be exempted in the same way.

## Potential Solutions

### Option A: Expand `isEphemeralEvent` to check `CommunityGear.Availability`

- **Approach:** Change `isEphemeralEvent` to accept the `feedLookups` context. For gear-related events (`GEAR_SHARED`, `TRANSFER_INTEREST_EXPRESSED`, `TRANSFER_RECIPIENT_SELECTED`, `TRANSFER_ACTIVE`), look up the `CommunityGear` from `fc.communityGearMap` and return `true` when `Availability == AVAILABILITY_FOR_GIVEAWAY`. The lookup data is already batch-fetched — no new queries needed.
- **Pros:** Minimal code change. Uses data already available. Consistent with how `createGearSharedItem` already reads `communityGear.Availability`. No schema changes.
- **Cons:** Couples the ephemeral check to `feedLookups`, changing the function signature. The `isEphemeralEvent` unit test becomes slightly more complex.
- **Effort:** Small.

### Option B: Add a dedicated `CommunityEventType` for giveaways

- **Approach:** Add `COMMUNITY_EVENT_TYPE_GEAR_GIVEAWAY` to the proto enum. Emit this event type (instead of `GEAR_SHARED`) when a gear is shared with `AVAILABILITY_FOR_GIVEAWAY`. Add it to `isEphemeralEvent`. Update `eventToFeedItem` and `itemKeyForEvent` to handle the new type.
- **Pros:** Makes the distinction explicit at the event level. `isEphemeralEvent` stays a pure event-type switch with no external lookups.
- **Cons:** Larger change surface — proto schema change, code generation, all event-creation call sites, deduplication logic, feed item rendering, and potentially client-side code that switches on event types. Higher risk of regressions. Giveaway vs loan is a property of the gear's community listing, not the event itself.
- **Effort:** Medium-large.

### Option C: Store `availability` on `CommunityEvent` directly

- **Approach:** Add an `availability` field to the `CommunityEvent` proto message. Populate it when creating gear-shared events. Check it in `isEphemeralEvent`.
- **Pros:** Makes the ephemeral check self-contained (no lookup needed). `isEphemeralEvent` stays pure.
- **Cons:** Proto schema change. Requires backfilling or handling old events that lack the field. Duplicates data already on `CommunityGear`. Adds complexity for marginal purity benefit.
- **Effort:** Medium.

## Recommendation

**Option A** is the clear winner. The `communityGearMap` is already batch-fetched as part of `feedLookups` and is available at the call site where `isEphemeralEvent` is invoked. The change is minimal, contained, and follows the existing pattern. The function signature change from `isEphemeralEvent(event)` to `isEphemeralEvent(event, fc)` is trivial and actually makes the function more useful for future ephemeral-type extensions.

Options B and C involve proto schema changes that are disproportionate to the problem and introduce migration complexity for no practical benefit.

## Decisions

- **Transfer lifecycle events for giveaways are ephemeral.** `TRANSFER_INTEREST_EXPRESSED`, `TRANSFER_RECIPIENT_SELECTED`, and `TRANSFER_ACTIVE` events on giveaway gear must also bypass the staleness expiry. After deduplication these share the `gear:<gear_id>` key, so only the latest event survives — if that event is a transfer lifecycle event on a giveaway, it must stay. The `communityGearMap` lookup covers all of them since they all carry `event.GearId`.
- **Feed documentation must be updated.** The "Ephemeral Items" section in `docs/client/the_feed.md` currently describes only experiences and requests; it will be updated to include giveaways.
- **Profile/loan display is a separate bug.** Tracked in a newly filed issue (see Related Issues). The portfolio filter chips lack a "Giveaway" category and `PortfolioItemCard` has no giveaway-specific labeling — that work is independent of this feed fix.
- **`feed_status.go` does not need updating.** The badge logic uses `isItemUnread()` with a 7-day window, not the 24h freshness expiry path. No changes needed.

## Implementation Plan

### Phase 1: Fix ephemeral check for giveaways
**Goal:** Giveaway items no longer expire from the feed after 24h.
**Tasks:**
- [ ] Modify `isEphemeralEvent` in `server/services/feed/service.go` to accept `*feedLookups` as a second parameter
- [ ] For gear-related event types (`GEAR_SHARED`, `TRANSFER_INTEREST_EXPRESSED`, `TRANSFER_RECIPIENT_SELECTED`, `TRANSFER_ACTIVE`), look up `fc.communityGearMap[event.GearId]` and return `true` when `Availability == AVAILABILITY_FOR_GIVEAWAY`
- [ ] Update the call site at line 578 to pass `fc`
- [ ] Update the function comment to mention giveaways
- [ ] Run `npm run build` to verify lint and compilation pass
**Validates:** Giveaway items survive the staleness expiry while loan items still expire normally.

**Note — no client changes required.** This is entirely a server-side feed generation fix. The client already renders giveaway cards correctly via `GearSharedPayload.availability`; the bug is that the server filters them out before the client ever sees them. No cache invalidation, repository, or viewmodel changes needed.

### Phase 2: Update tests
**Goal:** Regression coverage for giveaway ephemeral behavior.
**Tasks:**
- [ ] Update `TestIsEphemeralEvent` to test gear events with giveaway availability (requires passing `feedLookups` with a `communityGearMap`)
- [ ] Update `TestGetFeed_EphemeralItems` to add a giveaway gear alongside the existing loan gear, and verify the giveaway survives expiry while the loan expires
- [ ] Add test cases for transfer lifecycle events (`TRANSFER_INTEREST_EXPRESSED`, etc.) on giveaway gear to confirm they're also ephemeral
**Validates:** Test suite catches regressions if ephemeral logic changes.

### Phase 3: Update documentation
**Goal:** Feed documentation reflects the new behavior.
**Tasks:**
- [ ] Update the "Ephemeral Items" section in `docs/client/the_feed.md` to include giveaways alongside experiences and requests
- [ ] Update the "Filtered-Out Items" section to mention giveaways in the expiry exception
**Validates:** Documentation matches implementation.

### Phase 4: Exempt open loan transfers from staleness expiry
**Goal:** A gear card representing an in-progress loan (interest expressed, recipient selected, or active) never expires from the feed for either the owner or the receiver.
**Tasks:**
- [ ] In `isEphemeralEvent`, add a branch for gear events where `event.EventType` is one of `TRANSFER_INTEREST_EXPRESSED`, `TRANSFER_RECIPIENT_SELECTED`, or `TRANSFER_ACTIVE` and `communityGear.Availability == AVAILABILITY_FOR_LOAN` — return `true`
- [ ] Leave `GEAR_SHARED` on a loan gear unchanged: a gear that is merely available (no open transfer) can still expire normally
- [ ] Add test cases to `TestGetFeed_EphemeralItems` covering a loan gear whose champion event is each of the three transfer lifecycle types, verifying each survives past the 24h expiry window
- [ ] Run `npm run build` to verify lint and compilation pass
**Validates:** Owner and receiver both see an in-progress loan card indefinitely until it completes or is cancelled; available-but-unseen loan gear still expires after 24h.

## Related Issues
- #941 — Original fix that made experiences and requests ephemeral; this issue extends the same pattern to giveaways
- #1013 — Profile screen shows active giveaway as a loan (separate UX bug filed from the secondary symptom in #972)
