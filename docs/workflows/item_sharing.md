---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: In-app item authoring + sharing for gear and requests (#2492) — the host creates an item through unified-create, invites people and communities from the auto-opened share sheet, and inspects the "Shared with" card (→ access sheet, "who can see this"). The non-RSVP analog of the event Who's-In authoring flow.
  globs: [app/lib/presentation/widgets/sharing/shared_with_card.dart, app/lib/presentation/widgets/sharing/item_share_sheet.dart, app/lib/presentation/widgets/sharing/invite_members_sheet.dart, app/lib/presentation/screens/gear/gear_content_view.dart, app/lib/presentation/screens/request/request_content_view.dart, server/services/gear/service.go, server/services/request/queries.go, e2e/tests/workflows/gear-authoring-share.spec.ts, e2e/tests/workflows/request-authoring-share.spec.ts]
  triggers: [shared-with, sharing, invite, share-sheet, access-sheet, who-can-see, invited-individuals, gear-sharing, request-sharing, authoring]
  lens: [workflow, domain, client, server]
  domain: client
freshness:
  verified_commit: "11503e4ea"
  verified_on: "2026-07-03"
---
# Item Authoring → Sharing → "Shared with" Workflow (Gear & Requests)

Every gear item and request is **born with its own audience** — a nameless per-item
("ad-hoc") community provisioned at create time (#2492). This doc describes how a host
shares an item with people and communities and inspects who can see it, the non-RSVP
analog of the event [Who's-In flow](./experience.md). The **"Shared with" card** on the
gear/request read shell is the per-type sharing surface, reusing the same generic
`ItemShareSheet` / `InviteMembersSheet` / `AccessSheet` the event flow uses.

## The pieces this composes

- **Per-item ad-hoc community** — `SaveGear` / `SubmitRequest` provision the item's
  nameless community and share the item into it on create, so a new item has an audience.
- **The share sheet auto-opens after Save** — the unified-create flow ends at Save and the
  home screen opens [`ItemShareSheet`](../../app/lib/presentation/widgets/sharing/item_share_sheet.dart)
  (QR + `/go/{code}` link + **Invite people** + **Invite community**) for every item type.
- **Invite people** — [`InviteMembersSheet`](../../app/lib/presentation/widgets/sharing/invite_members_sheet.dart)
  lists the host's community members; selected members are added to the item's ad-hoc
  community via `ShareItem` (a `member_user_id` invitee — no consent needed, they're on
  Ripls). Off-app phone/email invites are gated on the SMS product flip ("coming soon").
- **Community add/remove is uniform** — sharing an item into (or removing it from) an
  existing community goes through `CommunityService.ShareItem` (`share_to_community_ids`)
  and `UnshareItem` for every item type. These are the only add/remove RPCs: the per-type
  `ShareGear` / `ShareRequest` / `ShareExperience` and `Unshare*` RPCs were removed in
  #2526.
- **The "Shared with" card** — [`SharedWithCard`](../../app/lib/presentation/widgets/sharing/shared_with_card.dart)
  on the gear/request read shell shows "Shared with N people" + a face cluster of the
  invited individuals; tapping it opens the [`AccessSheet`](../../app/lib/presentation/widgets/content/access_sheet.dart)
  ("who can see this": communities + deduped member count), and an Invite button — shown to
  every viewer (#2630) — re-opens the share sheet. Any member may reshare the item's open
  link; the sheet hides its audience rows for non-owners (server-derived
  `can_manage_audience`), so only the owner sees "Invite people" / "Invite community".
  Built from `GetGearResponse` / `Request` `total_distinct_member_count`
  + the `invited_individuals` field (the people the owner invited one by one).

## How the loop runs

0. The host **creates a gear/request** through unified-create (text prompt → Generate →
   Save). The item's per-item community is provisioned and the share sheet auto-opens.
1. From the share sheet, the host taps **Invite people**, picks members, and confirms —
   each is added to the item's ad-hoc community as an invited individual.
2. The host opens the item (`/gear/{id}` or `/request/{id}`); the **"Shared with" card**
   shows the count + the invited faces.
3. Tapping the card opens the **access sheet** ("Shared with") — the communities the item
   is shared with and the deduped audience total.
4. Additional sharing (communities, more people) flows through the same share sheet, reached
   from the card's Invite button.

## Workflow Example — Host invites people and inspects "Shared with"

Exercised by the Playwright e2e (`e2e/tests/workflows/gear-authoring-share.spec.ts` and
`request-authoring-share.spec.ts`) against the real Flutter Web UI; the audience is also
confirmed via RPC so the card's counts are anchored to server truth.

### Preconditions

- A host is registered and belongs to a community with at least the people they will invite.

### Steps

1. **Host** (UI): create a gear/request via unified-create → the share sheet auto-opens.
2. **Host** (UI): **Invite people** → pick two community members → **Invite 2**.
3. **Host** (UI): open the item → the **"Shared with" card** reads "Shared with 3 people"
   (host + the two invitees); tap it → the access sheet ("Shared with") lists the
   item's communities and the invited people by name.

### Postconditions

- `GetGear` / `GetRequest` returns the two as `invited_individuals`, and
  `total_distinct_member_count` is 3 (host + two invitees, deduped).
- Community-level sharing from the share sheet is covered separately by
  `event-whos-in-counts.spec.ts` / `share-sheet-community-picker.spec.ts`.
