# Issue #953: Invite / Share link preview should show item details

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/953
**Filed:** 2026-03-31 | **Last activity:** 2026-03-31
**Labels:** P1 | **Assignees:** maintainer
**Context docs:** docs/invitations.md, docs/ai/invitation_improvements.md

## Problem Statement

When a user shares an invitation link that deep-links to a specific item (gear,
request, or experience), the link preview (OG metadata) only shows generic
community information: "Join {CommunityName} on Ripls" with the community image.
This is uninformative and unpersuasive — the recipient doesn't see what's
actually being shared.

The desired behavior is context-aware previews that vary by item type and intent:
- **Gear (loan):** "Borrow Ryobi Pressure Washer from Thomas" + item photo
- **Gear (giveaway):** "Free Ryobi Pressure Washer from Thomas" + item photo
- **Experience:** "Join Bryan for Climbing in Rifle next Tuesday" + experience photo
- **Request:** "Thomas needs a Tent" + request photo (if any)
- **Community-only (no item):** Current behavior (community name + image)

The loan vs. giveaway distinction matters because the call-to-action is
fundamentally different: borrowing vs. claiming something free. The availability
type lives on `CommunityGear.availability` (per community+gear pair), so the
web service needs to look up the `CommunityGear` record to determine this.

## Context from Comments

One comment from the issue author (maintainer) pointing to
`docs/ai/invitation_improvements.md` for context on the most recent invitation
system work. Key takeaway: the `/go/{code}` web service and OG metadata
infrastructure are fully implemented — this issue is about enriching the existing
metadata with item-specific details.

## Potential Solutions

### Option A: Server-side item lookup in web service

**Approach:** When `HandleInvitePage` receives a request with query params
(`?gear_id=`, `?request_id=`, `?experience_id=`), fetch the item, its
owner/creator, and (for gear) the `CommunityGear` record to determine
loan vs. giveaway availability. Use that data to build richer OG tags. The
template conditionally renders item-specific title, description, and image.

**Pros:**
- Uses existing infrastructure (web service, template, presigned URLs).
- Query params are already present in the URL when shared from the app.
- No new RPCs, no new storage, no schema changes needed.
- Social crawlers see the enriched metadata on first fetch.
- `CommunityGear` lookup is straightforward since the invitation already
  carries `community_id` and the URL carries `gear_id`.

**Cons:**
- Adds 2-3 extra DB queries per page load (item + owner + community_gear for
  gear), but this is a low-traffic page (invitation clicks, not API calls).
- Must handle missing/deleted items gracefully (fall back to community preview).

**Effort:** Small.

### Option B: Store item context on the invitation link

**Approach:** Persist `gear_id`, `request_id`, `experience_id` on
`CommunityInvitationLink` so the web service doesn't need to parse query params.

**Pros:**
- Cleaner data model — the invitation "knows" what it's for.

**Cons:**
- Requires proto + storage migration for `CommunityInvitationLink`.
- One invitation link per inviter+community is currently reused — adding an item
  ID would either break reuse or require creating separate links per item.
- Query params are needed anyway for the deep link into the app, so this
  duplicates data without clear benefit.

**Effort:** Medium.

### Option C: Separate "share link" system distinct from invitations

**Approach:** Create a new `ShareLink` entity and `/share/{code}` route for
item-specific sharing, keeping `/go/{code}` for community invitations only.

**Pros:**
- Clean separation of concerns (invitation vs. sharing).

**Cons:**
- Significantly more scope — new proto types, new storage, new service, new
  routes, new app handling.
- Doesn't solve the immediate problem any better than Option A.

**Effort:** Large.

## Recommendation

**Option A** — server-side item lookup from query params. The infrastructure is
already in place. The web service already receives the full URL including query
params. This is a small, focused change: read the query params, fetch the item
if present, look up availability from `CommunityGear`, and pass richer data to
the template.

## Open Questions

- **Should the item image replace the community image, or appear alongside it?**
  Recommendation: Replace it. OG previews have one primary image. The item is
  more compelling and specific than the community photo. Fall back to community
  image if the item has no media.

- **What if the item has been deleted or is in a terminal state (given away,
  cancelled)?** Recommendation: Fall back gracefully to the community-only
  preview. The link still works for joining the community; the item context is
  just a bonus for the preview.

- **Should the preview text mention the community at all when an item is
  present?** Recommendation: Yes, keep it in the description. Title focuses on
  the item action, description mentions the community. E.g., title: "Borrow
  Ryobi Pressure Washer from Thomas", description: "Available on Ripls in
  {CommunityName}".

- **Should experience previews include the date/time?** Recommendation: Yes,
  if the experience has a start time and it's in the future, include it
  naturally (e.g., "March 15" or "this Saturday").

- **What if `CommunityGear` lookup fails (gear shared but not in this
  community)?** Recommendation: Default to "loan" language since borrowing is
  the primary use case. Giveaway language only when explicitly confirmed.

## Implementation Plan

<!-- AUDIT: Merged original Phases 1+2 — fetching data and rendering it are not
     independently shippable. Moved terminal-state checks from Phase 3 (polish)
     to Phase 1 (correctness): rendering "Borrow X" for given-away gear is wrong,
     not just unpolished. -->

### Phase 1: Item-specific OG metadata (data + template)

**Goal:** When a `/go/{code}` URL includes item query params, render
item-specific OG metadata with distinct treatments for gear loans, gear
giveaways, requests, and experiences.

**Tasks:**
- [ ] Add item fields to `invitePageData` struct: `ItemName`, `ItemDescription`,
  `ItemImageURL`, `ItemType` (string: "gear_loan", "gear_giveaway", "request",
  "experience"), `OwnerName`, `ExperienceDate` (formatted string, optional)
- [ ] In `HandleInvitePage`, parse `gear_id`, `request_id`, `experience_id`
  from `r.URL.Query()`
- [ ] Implement `fetchGearPreviewData(ctx, gearID, communityID)` — fetches
  `Gear` model, owner `User`, and `CommunityGear` (for availability). Returns
  enriched `invitePageData` fields. Falls back on any error.
- [ ] For gear: query `CommunityGear` by `community_id` + `gear_id` to get
  `Availability`. Set `ItemType` to `"gear_loan"` or `"gear_giveaway"`.
  Default to `"gear_loan"` if `CommunityGear` not found.
- [ ] Skip item enrichment for terminal states: gear in `GIVEN_AWAY` state,
  cancelled requests, cancelled experiences — fall back to community preview.
- [ ] Implement `fetchRequestPreviewData(ctx, requestID)` — fetches `Request`
  model and requester `User`.
- [ ] Implement `fetchExperiencePreviewData(ctx, experienceID)` — fetches
  `Experience` model, creator `User`, and formats start time if available.
- [ ] For all item types: if item has `media_ids`, generate presigned URL for
  first image (reuse pattern from `getCommunityImageURL`). This becomes
  `ItemImageURL`, replacing the community image in OG tags.
- [ ] On any fetch error or missing item, log a warning and proceed with
  community-only preview (current behavior).
- [ ] Update `invite.html` OG meta tags with conditional rendering:
  - `gear_loan`: `og:title="Borrow {ItemName} from {OwnerName}"`,
    `og:description="Available to borrow on Ripls in {CommunityName}"`
  - `gear_giveaway`: `og:title="Free: {ItemName} from {OwnerName}"`,
    `og:description="Available for free on Ripls in {CommunityName}"`
  - `experience`: `og:title="Join {OwnerName} for {ItemName}"`,
    `og:description="{ExperienceDate} on Ripls in {CommunityName}"` (or
    without date if not set)
  - `request`: `og:title="{OwnerName} needs: {ItemName}"`,
    `og:description="Can you help? On Ripls in {CommunityName}"`
  - Fallback (no item or item not found): current community-only OG tags
- [ ] Update `og:image` to use `ItemImageURL` when present, community image
  otherwise.
- [ ] Update corresponding `twitter:*` meta tags to match.
- [ ] Update the visible page content (not just meta tags) to show item info:
  item image, item name, action-specific heading and subtext.
- [ ] Handle items with no media (fall back to community image, then logo).
- [ ] If the new fetch helpers push `service.go` past ~400 lines, extract them
  into `item_preview.go` within the same `web` package (per server architecture
  guideline: <500 lines per file ideally).
- [ ] Add unit tests for each fetch helper and for `HandleInvitePage` with
  item query params (gear loan, gear giveaway, request, experience, missing
  item, invalid ID, terminal-state item).

**Validates:** Link previews in messaging apps (iMessage, WhatsApp, Slack) show
item-specific content with correct loan/giveaway/request/experience treatment,
and degrade gracefully for missing or terminal-state items.

### Phase 2: Cross-platform polish

**Goal:** Verify appearance across social platforms and fine-tune rendering.

**Tasks:**
- [ ] For experiences with a future start time, format the date naturally
  (e.g., "March 15" or "Saturday, April 2").
- [ ] Consider adding `og:image:width` and `og:image:height` tags if needed
  for consistent rendering across platforms.
- [ ] Test with real social platform link preview debuggers (Facebook Sharing
  Debugger, Twitter Card Validator, Slack unfurling).

**Validates:** Previews look good across platforms with correct image sizing.

## Related Issues

- #928 — Short URL sharing (prior work on `/go/` route and share links)
- #690 — Invitation cleanup (referenced by TODO comments in invitation code)
