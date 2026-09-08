---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Ad-hoc (nameless) per-item communities — the audience for a single item, created at item-creation time, that becomes a real persistent community the moment it is named.
  globs: [proto/ripls/models/community.proto, server/services/community/adhoc.go, server/services/community/lifecycle.go, app/lib/core/utils/community_display.dart, app/lib/presentation/widgets/group_avatar.dart]
  triggers: [ad-hoc, adhoc-community, unnamed-community, per-item-community, origin-item, promote-by-naming, nameless-community, group-label]
  lens: [domain, server, client]
  skills: [issue]
  domain: community
freshness:
  verified_commit: "4b8c3b978"
  verified_on: "2026-08-16"
---
# Ad-hoc communities

An **ad-hoc community** is a per-item `Community` — the audience for a single
item (an event, request, giveaway, or loan). It is created at item-creation time
so that **every participant, even a brand-new phone/email invitee, is always a
member of some community**, and visibility is cleanly "the audience of this
item." This is part of the account-optional, phone-first pivot
([epic #2492](issues/2492-phone-first-pivot.md), item COMM-1).

Ad-hoc communities are not a second container type. They are ordinary
`Community` rows that differ in exactly one way: they have **no name**. Sharing
junctions, the community-event bus, streams, push recipients, and visibility all
already key off `community_id`, so they work unchanged.

## Name-presence is the discriminator — there is no `kind` enum

A community with an empty `name` is ad-hoc; a named one is "real" (persistent).
That's the whole signal. `community.IsAdHoc(c)`
(`server/services/community/adhoc.go`) is the canonical check:

```go
func IsAdHoc(c *models.Community) bool { return c != nil && c.Name == "" }
```

We deliberately avoid a `kind` REAL/AD_HOC enum (Decision 2 in the epic): a
nameless community **becomes a real, persistent community the moment someone
names it** — the natural "this one-off turned into a standing thing" path, which
is a product goal. A `kind` enum would fight that (you'd flip a kind instead of
just adding a name) and add a second discriminator to thread through every
reader.

## `origin_item` provenance

`Community.origin_item` (`proto/ripls/models/community.proto`) is an optional
`oneof` recording the single item the community was spun up for:

```proto
oneof origin_item {
  string origin_experience_id = 12;
  string origin_gear_id       = 13;
  string origin_request_id    = 14;
  string origin_transfer_id   = 15;
}
```

The variant names the item kind; the value is that item's id. It mirrors
`ShareLink.target`. It is **provenance only** — *not* the ad-hoc-vs-real signal
(name-presence is). `origin_item` is **retained across promotion**, so a standing
group can still answer "this began as an event." Communities created directly
via the "Create community" flow leave `origin_item` unset.

## Provisioning

`Service.ProvisionAdHocCommunity(ctx, hostUserID, origin, audience)`
(`server/services/community/adhoc.go`) returns the nameless community for an
item's audience. The host becomes creator + owner + first member. `audience`
(`AdHocAudience`) carries both `MemberUserIDs` (real-user invitees, seeded as
members and deduped against the host) and `ContactHandles` (normalized
phone/email handles of provisional invitees). Provisional invitees are *seeded*
separately via the provisional-user path (see
[single_player_mode.md](single_player_mode.md)); the handles are passed here only
so they count toward the dedup signature (below).

Origin is supplied via the exported `AdHocOrigin` struct (exactly one id set),
which maps to the `origin_item` oneof internally — callers in other service
packages never touch the generated (unexported) oneof wrapper types.

`ProvisionAdHocCommunity` and the `CreateCommunity` RPC share one service-level
entry point, `createCommunity` (`adhoc.go`), which delegates to the shared
community-creation library `communitylib.CreateCommunity`
(`server/community/provision.go`). That library is the single owner of the
create sequence — insert the row, seed membership, compute regions, publish
`COMMUNITY_CREATED`, and open the community conversation — so item services can
create per-item communities without depending on this service (it also exports
`ProvisionPerItemCommunity`, the origin-keyed per-item path). The only
differences between the two service callers are the empty name and the origin on
the ad-hoc path.

CREATE-1 wires item-creation flows (`SaveExperience`, `SubmitRequest`, …) to
provision a host-only per-item community at insert time. They call the shared
library's `ProvisionPerItemCommunity` (`server/community/provision.go`) directly
— an origin-keyed, idempotent path that creates one nameless community per item
(no audience-signature dedup). `ProvisionAdHocCommunity` (the service method,
with audience seeding + signature dedup) is the alternate entry point used when
an audience is supplied up front. COMM-1 provides the mechanism.

## Deduplication by audience

Without dedup, a host who creates three events for the same three friends would
spawn three identical nameless communities — confusing, and cluttering their
community list. So **one host + one exact set of invitees ⇒ one ad-hoc
community.** On provisioning, the host plus every invitee (real-user id or
normalized contact handle) is hashed into a canonical, order-independent
`member_signature` (`Community.member_signature`, server-only). If the host
already has a live ad-hoc community with the same signature,
`ProvisionAdHocCommunity` **returns that community** instead of creating a new
one; the caller associates the new item with it. The reused community keeps its
original `origin_item` — provenance stays the *first* item the group was spun up
for. An audience-less (host-only) item is never deduped (its signature is empty).

The signature is the **creation-time intended audience**, not live membership.
Post-creation drift from open-link joins is *not* retroactively merged — that
"community merging" is deliberately out of scope (it would be surprising to have
groups silently combine as people join). Naming a community also takes it out of
the ad-hoc pool entirely (named communities carry no signature and never dedup).

## Promote-by-naming

There is **no dedicated promote RPC**. Naming an unnamed community *is* the
promotion: `UpdateCommunity` (`server/services/community/lifecycle.go`), which
already sets `name` when the request supplies a non-empty one. On the first
unnamed→named transition it emits a `COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED` event
(actor = the member who named it). Renaming an already-named community does
**not** re-emit, and a description-only update does not promote.

After promotion the community is an ordinary persistent community; `origin_item`
remains as provenance.

## Membership & open-link join

Open-link join reuses the existing share-link path
(`GetOrCreateShareLink` → `/go/{code}` → `AcceptInvitationLink`,
`server/services/community/invitations.go`) — no special casing for ad-hoc
targets. This is the same emergent-membership behavior persistent communities
already have via ShareLink, scoped tighter (a single-item audience).

## Surfacing

Ad-hoc communities surface like any community, with two name-presence
adjustments in `ListCommunities` (`server/services/community/membership.go`).
**Host-only ad-hoc communities are suppressed:** a nameless community whose only
member is the owner (member count ≤ 1) is a per-item community with no audience —
pure clutter — so it is skipped entirely; it reappears once anyone else joins.
For the remaining nameless communities the server fills two `CommunityItem`
fields so the client can render group-chat style without a name lookup:
`member_count` and `member_preview_first_names` (up to `memberPreviewLimit` = 3
other members' first names), plus `origin_item_name`. Named communities pass
`community.Name` straight through with an empty preview list.

### One derivation, two audiences

Every client surface that shows a nameless community derives its label through
`app/lib/core/utils/community_display.dart` — never `CommunityItem.name`, which
is empty by definition. Reading the field directly is how the settings hub, the
governance chain, and the invite sheet all came to render blank rows and a
generic "C" avatar in production (#2937). Three functions divide the work, and
which one you want depends on **who reads the string**:

| Function | Renders | Use for |
|---|---|---|
| `communityDisplayName` | `"You, Alex, and Sam"`, or `"N members"` when no preview names resolve. Never empty. | Anything the **viewer** reads — they are one of the named people. |
| `communitySubtitle` | The description; else `"from {origin item}"`; else a member count. Null when the title already *is* the count. | The second line of a community row. |
| `communityPublicName` | The generic `"a Ripls group"`. | Anything that **leaves the group** — invite share text and its subject. |

That last row is a PII boundary, not a style preference: composing an invite with
the member rollup hands members' first names to a non-member. It is the client
half of the same rule the SSR landing follows below.

Paired with the label, `GroupAvatar`
(`app/lib/presentation/widgets/group_avatar.dart`) stands a cluster of member
faces in for the photo a nameless community doesn't have. `CommunityAvatar`
falls back to it automatically, so a call site cannot reintroduce the bare "C".

The settings hub additionally lists nameless groups under their own **GROUPS**
section, separate from named communities — a user's group count grows with the
number of items they touch (see Lifecycle), so interleaving them buries the
communities.

The **public `/go/` SSR landing** (`server/services/web`) is a different,
pre-auth surface: it cannot use the member-name group-text rendering (that would
leak PII) and an empty name would produce broken copy/OG cards. So
`communityDisplayName` (`service.go`) substitutes a generic public label —
"a Ripls group" — for a nameless community, applied at both the invite and event
landing builders. The label is intentionally generic; the landing already
surfaces the item and host. (LINK-1.) The client's `communityPublicName` mirrors
it word for word.

## Lifecycle

An ad-hoc community **persists** — it may evolve into a real one. Don't auto-kill
a group that's still active. Archive/soft-delete it alongside its item (reuse
`DeletedMetadata` + the existing cascade) only if it stays a true one-off with no
further activity.

Because they persist and are created per item, **a user's community count grows
with the number of items they touch** — tens or hundreds, not the "1-5" the
pre-pivot codebase assumed. Anything sized per community has to be checked
against that. The realtime layer was the first thing it broke (#2867): one
stream and one poll timer per community exhausted the browser's connection pool
and multiplied server load, and both are now per user instead. See
[realtime_updates.md](realtime_updates.md) § "Why Per-User".

## The 32-member cap (current limitation)

For now an ad-hoc community is subject to the **same 32-member cap as any
community** — open-link join enforces it in
`AcceptInvitationLink` (`invitations.go`) and register-via-invite enforces it in
`server/services/login/registration.go`. **Exempting unnamed communities from
the cap is deferred future work** (epic #2492, COMM-1 "Deferred"): it requires
auditing every cap-enforcement site to branch on name-presence, plus a
grandfather rule so a group that grew over-cap while unnamed is never blocked
from being named. Until that lands, the cap applies uniformly, which is fine for
the initial events slice. The bounded-circle thesis for persistent named
communities is documented in [community_health.md](community_health.md).
