---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client content sharing — community-level access for gear/requests/experiences via the sidebar enabled set, Access Pill/Sheet, creation-time community pickers, per-type sharing notifiers, and invite links.
  globs: [app/lib/presentation/widgets/sharing/**, app/lib/presentation/widgets/content/**, app/lib/presentation/viewmodels/*sharing*]
  triggers: [sharing, community, access-ring, access-sheet, invite, community-selection, enabled-communities]
  lens: [client, domain]
  domain: client
freshness:
  verified_commit: "11503e4ea"
  verified_on: "2026-07-03"
---
# Client Sharing

Sharing controls who can see a piece of content (gear, request, experience). Access is granted at the community level — a user can see an item if they belong to at least one community that has been granted access.

---

## Community Scope (full portfolio)

[`app/lib/services/providers/community_providers.dart`](../../app/lib/services/providers/community_providers.dart) (`communitiesProvider`)

The nav restructure (#1895) removed the sidebar and its per-community "enabled" toggle. There is **no selected-community filter** anymore: `communitiesProvider` holds every community the user belongs to (the full portfolio), and `communitiesProvider.communityIds` is the canonical list everything reads. This drives two things:

1. **What the feed shows** — content shared with *any* of the user's communities. The feed view-model is initialized from `communitiesProvider.communityIds`, not a user-toggled subset.
2. **What communities are pre-selected** when creating a new item — the creation community picker initialises from the same full list.

The Workshop tab keeps its own *independent* single-select scope (`workshopCommunityProvider`, "Everything" pill default) for the Workshop brief only; it deliberately does **not** propagate to feed, inbox, or item creation.

**Creating a community** — `CommunityCreationModal` is reached from the Workshop tab and from the sticky "Create new community" footer inside `CommunitySelectionSheet`. On success the new community is added to the portfolio list.

---

## Access Pill

[`app/lib/presentation/widgets/content/content_access_pill.dart`](../../app/lib/presentation/widgets/content/content_access_pill.dart)

The trailing affordance in a content view's tab bar, styled to match an
inactive tab pill. It shows an invite count drawn from the server field
`totalDistinctMemberCount`, which counts each unique member once regardless of
how many shared communities they belong to. Tapping it opens the Access Sheet.

---

## Access Sheet

[`app/lib/presentation/widgets/content/access_sheet.dart`](../../app/lib/presentation/widgets/content/access_sheet.dart)

A bottom sheet showing the full access picture. The header is a static "Shared with" label; the deduplicated `totalDistinctMemberCount` surfaces via the Access Pill and the Shared with card rather than being repeated here.

**Summary banner** — stacked avatars of up to 7 unique members across all communities, with first names listed. Members are fetched eagerly when the sheet opens and deduplicated by user ID.

**Community accordion** — one expandable row per shared community. Each row shows who shared it, when, and how many members that community has. Expanding a row lists those members individually (fetched lazily on first open).

**Adding a community (owner only)** — "Add Community" opens [`CommunitySelectionSheet`](../../app/lib/presentation/widgets/sharing/community_selection_sheet.dart) in `immediate` mode (`showForImmediate`), which lists all communities the owner belongs to with a toggle for each. Toggling fires the API synchronously through the appropriate sharing provider (`gearSharingProvider` / `requestSharingProvider` / `experienceSharingNotifierProvider`); the sheet refreshes from optimistic state when the modal closes, with a full server sync deferred until the sheet itself is dismissed.

**Inviting a user (owner only)** — "Invite Person" opens [`ItemShareSheet`](../../app/lib/presentation/widgets/sharing/item_share_sheet.dart), which finds-or-provisions the item's per-item ad-hoc community, renders the resulting open link (QR + truncated URL + Copy/Share), and offers "Invite people" (existing Ripls members, via `InviteMembersSheet`; off-app phone/email invites are gated on platform SMS and surfaced as "coming soon") and "Invite community" (additive multi-select that hides communities the item is already shared with). The sheet itself is member-safe (#2630): any member with view access can open it and reshare the link — `ShareItemResponse.can_manage_audience` (server-derived) decides whether the two audience rows render, so non-owners get QR/Copy/Share only. See [docs/client/create.md](create.md) for the full sheet.

The separate `InviteSheet` (community-scoped invite link with per-community pick via [`CommunitySelectionSheet.showForInvite`](../../app/lib/presentation/widgets/sharing/community_selection_sheet.dart) — single-select, tap-to-pop, plus copy/revoke) is still reached from community-level surfaces: the workshop screen's plus-group icon (pre-selected to the carousel-pinned community), Manage Members, and the community content view.

### Shared with card (gear & requests)

[`SharedWithCard`](../../app/lib/presentation/widgets/sharing/shared_with_card.dart) gives gear and request read shells the same prominent sharing surface events have, so inviting reads the same across item types (#2492). Built on [`ContentEdgesCard`](../../app/lib/presentation/widgets/content/content_edges_card.dart), it shows "Shared with N people" plus a small cluster of the faces directly invited to the item's own ad-hoc community, tapping the card opens the [`AccessSheet`](../../app/lib/presentation/widgets/content/access_sheet.dart) ("who can see this"), and an Invite button — shown to **every** viewer, not just the owner (#2630) — opens [`ItemShareSheet`](../../app/lib/presentation/widgets/sharing/item_share_sheet.dart). Any member may reshare the item's open link; the sheet hides its audience-management rows for non-owners (server-derived `can_manage_audience`), so a non-owner gets QR + Copy/Share only. The face cluster reads `total_distinct_member_count` and the new `invited_individuals` field (the people the owner invited one by one) off `GetGearResponse` / `Request`. Gear previously had no access/invite affordance on its read shell — its `AccessSheet` plumbing (`_showAccessSheet` / `_buildAccessGroups`) lives in `gear_content_view.dart`, mirroring the request flow. After an invite, `ItemShareSheet` refreshes the gear/request detail so the card updates live.

---

## Add Modal — Creating an Item

Audience selection during creation varies by flow.

The **unified-create flow** (gear / request / event from the home-screen `+`) no longer picks an audience during creation — the `UnifiedPreviewCard` ends at Save and the home screen opens [`ItemShareSheet`](../../app/lib/presentation/widgets/sharing/item_share_sheet.dart) afterward (see [docs/client/create.md](create.md)). The remaining creation-time community pickers live in the legacy / Workshop-reachable preview modals:

- Request: [`app/lib/presentation/screens/request/request_preview_modal.dart`](../../app/lib/presentation/screens/request/request_preview_modal.dart)
- Experience: [`app/lib/presentation/screens/experience/experience_preview_modal.dart`](../../app/lib/presentation/screens/experience/experience_preview_modal.dart)

Both follow the same pattern:

1. The community picker initialises with the user's full community portfolio (`communitiesProvider.communityIds`).
2. Tapping the community row opens [`CommunitySelectionSheet`](../../app/lib/presentation/widgets/sharing/community_selection_sheet.dart) in `deferred` mode (`showForDeferred`), which lists all the user's communities and lets the user toggle individual ones. Confirm returns the selected IDs to the caller; the sheet also offers a "Create new community" footer that opens `CommunityCreationModal` inline.
3. **At least one community is required.** Submit is blocked and an error (`communitySelectRequired`) is shown if no community is selected.
4. On submit, the item is created and shared with the selected communities in a single operation.

---

## Community Invite Sheet

[`app/lib/presentation/screens/communities/invite_sheet.dart`](../../app/lib/presentation/screens/communities/invite_sheet.dart)

`InviteSheet` generates a **community-scoped** invite link (not item-scoped — for the item path use `ItemShareSheet`, see above). It is reached from community-level surfaces: the workshop plus-group icon, Manage Members, and the community content view. The invite link is scoped to a specific community; if the owner belongs to multiple communities they can switch between them in the sheet. Sharing or copying the link lets recipients join that community and immediately see its content.

---

## Sharing Providers (Post-Creation)

After an item is created, sharing is managed by a per-content-type notifier:

- Gear: [`app/lib/presentation/viewmodels/gear_sharing_view_model.dart`](../../app/lib/presentation/viewmodels/gear_sharing_view_model.dart) (`gearSharingProvider`)
- Request: [`app/lib/presentation/viewmodels/request_sharing_view_model.dart`](../../app/lib/presentation/viewmodels/request_sharing_view_model.dart) (`requestSharingProvider`)
- Experience: [`app/lib/presentation/viewmodels/experience_sharing_view_model.dart`](../../app/lib/presentation/viewmodels/experience_sharing_view_model.dart) (`experienceSharingNotifierProvider`)

Each notifier tracks which communities the item is currently shared with and which are in-flight. Toggle operations fire immediately against the API (optimistic).

Community add/remove has **converged onto `CommunityService.ShareItem` / `UnshareItem`** (#2526): the per-type `shareExperience` / `shareGear` / `shareRequest` and `unshare*` service wrappers still exist and keep the same signatures — they are Dart-side names now, not RPC names — but internally call `ShareItem` (with `share_to_community_ids`) and `UnshareItem`. The per-type `Share*` / `Unshare*` RPCs they used to call were removed from the API in #2526. Gear no longer carries a per-community share-time availability (loan/giveaway is item-wide since #2492/#2687, set at creation or via `SetGearAvailability`).

Minimum-community rules differ by type: gear and experiences can be removed from every community, but a **request cannot be unshared from its last remaining community** (enforced server-side in `UnshareItem` and mirrored client-side in `RequestSharingNotifier`).

---

## Conversations and Community Sharing

Every item type now uses **one entity-scoped conversation** shared by all communities the item is in. The per-community `conversation_id` fields on `CommunityGear` / `CommunityRequest` / `CommunityExperience` are deprecated (retained for migration reads only); the live conversation moved onto the entity itself:

| Item | Conversation model |
|------|--------------------|
| Gear (transfer / giveaway / listing) | **One conversation per gear item** (`Gear.conversation_id`, field 27 — migrated off the now-deprecated `CommunityGear.conversation_id`). Scoped to the gear itself, not to a specific community. |
| Request | **One conversation per request** (`Request.conversation_id`, field 23 — migrated off the now-deprecated `CommunityRequest.conversation_id`). Scoped to the request itself. |
| Experience | **One global conversation** (`Experience.conversation_id`) shared by all attendees across all communities. The deprecated `CommunityExperience.conversation_id` is not surfaced — all chat takes place in the single global thread. |

Scoping chat to the entity is intentional: people interacting with the same gear item, request, or event should coordinate in one place regardless of which community surfaced it to them. The Chat tab in [ExperienceScreen](../../app/lib/presentation/screens/experience/experience_screen.dart) and the inbox row both use `Experience.conversation_id` for consistency.

---

## RSVPs and Community Sharing Are Independent

**Sharing an experience with a community never adds or removes RSVPs. Removing a community never removes RSVPs. These two operations are completely independent.**

- An RSVP records a user's intention to attend the experience itself, not a particular community's view of it.
- The RSVP list always shows one entry per unique user, regardless of how many communities the experience is shared with.
- Deduplication is enforced server-side in `buildRSVPs` ([`server/services/experience/service.go`](../../server/services/experience/service.go)) by querying all RSVPs across communities and keeping the most recently updated entry per user.
