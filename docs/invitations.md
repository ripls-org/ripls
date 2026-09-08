---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: How invitation links grow communities — reusable per-member links, 8-char short codes, server-rendered /go/ landing pages with Open Graph previews, deep-linking to items, capacity limits, and revocation.
  globs: [server/services/community/invitations.go, server/services/web/**, app/lib/presentation/screens/communities/invite_sheet.dart]
  triggers: [invitation, invite-link, short-code, deep-link, og-metadata, accept-invitation, revoke]
  lens: [domain, client, server]
  domain: invitations
freshness:
  verified_commit: "041b93570"
  verified_on: "2026-08-09"
---
# Invitation System

This document describes how the Ripls invitation system works, from link generation through community membership.

## Overview

Ripls uses invitation links to grow communities. Each community member can generate a shareable link that allows new users to join their community. The system supports:

- Reusable invitation links (one per member per community)
- Short, clean URLs with 8-character codes
- Server-rendered invite pages with Open Graph metadata for rich link previews
- Deep linking to specific items (gear, requests, experiences)
- Capacity limits (max 32 members per community)
- Link revocation

## Components

### Server

- **GetOrCreateShareLink** - Creates or retrieves a share link for a target (community invite, gear, transfer, request, or experience). The canonical link-creation RPC. Item-target links (gear, experience, request, transfer) are always scoped to the item's ad-hoc origin community; the `community_id` argument is used only for caller-membership auth, not for the link scope. (The legacy `GetOrCreateInviteLink` shim was removed in #2058.)
- **ShortLinkCodeForEntity** - Server-internal (no RPC) find-or-create that returns the `/go` short code for an entity (experience/gear/request) in a community, keyed on the resolved community's owner. Item-target links are re-scoped to the item's ad-hoc origin community. With no entity it falls back to a **community-invite** link that lands on the community itself (for community-wide notifications like a member joining). Used by the off-app notification channel so SMS and email carry a short `/go` link rather than a long entity-ID URL (`server/notifications/deeplink.go`). Skips auth and the "shared with this community" verify — the notification's community already references the entity.
- **AcceptInvitationLink** - Adds an authenticated user to a community
- **RevokeShareLink** - Invalidates a share link by short code (caller must be the inviter)
- **CheckInvitation** - Validates a short code without authentication (for landing pages); defined on the login service (`server/services/login/registration.go`)

### Web Service (`server/services/web/`)

- **HandleInvitePage** - Looks up the short code and dispatches to the landing for the row's target: `community.html`, `event.html`, `gear.html`, or `request.html`. Every one is a phone-first landing with OG metadata; `invite.html` is only the shared error surface.
- **Static assets** - Serves shared CSS and images embedded from `website/`

### Client

- **WebCommunityScreen** (`/group/{communityId}`) - The web guest entry a community landing's CTA hands off to (#2875): routes an unauthenticated guest phone-first, joins on return, then shows the community
- **RegisterScreen** - Shown to unauthenticated users clicking a link; hosts the invitation hero, all auth methods, and an "I already have an account" link
- **Deep link routing** - Handles `ripls://invite` and `https://ripls.app/go/` URLs (`app/lib/core/router/app_router.dart`)

### Web

- **Landing page** (`website/content/invite.html`) - Static fallback served by nginx

## Invitation Link Lifecycle

### 1. Link Generation

When a member wants to invite someone:

```
Member taps "Invite" → GetOrCreateShareLink RPC → Server returns URL
```

The server either:

- Returns an existing active link for this member+community pair
- Creates a new `ShareLink` record with a unique short code

Short codes are 8 characters, generated with `crypto/rand`, using a charset that excludes confusable characters (0/O, 1/l/I). Uniqueness is verified against the database before use.

Each member has at most one active (non-revoked) link per community. This simplifies management and prevents link sprawl.

### 2. Link Structure

URL format:

```
https://ripls.app/go/Xk9mN2pQ
```

The hostname is environment-specific:

- Production: `ripls.app`
- Development: `dev.ripls.app`
- Local: `localhost:8080`

The `/go/` prefix is intentionally generic — it's a short-link system that started with invitations and is now also the link form used in **off-app notifications** (SMS/email): a short, legible `/go/{code}` that deep-links the recipient to the entity the notification is about, and (unlike `/experience/…`, `/gear/…`, `/request/…`) one of the few ripls.app paths universal-linked into the native app.

**Item-target share links** (gear, experience, request, transfer) are always scoped to
the item's ad-hoc origin community, regardless of the mint-time `community_id`
supplied by the caller. The `community_id` argument is used only for
caller-membership auth (the caller must be a member of an active community); the
row's scope and the response's `community_id` both reflect the resolved ad-hoc
community. A joiner accepting an item-target link joins the item's ad-hoc
audience, not the caller's named community. For legacy items that have no
ad-hoc community yet (predating #2492), the caller-supplied community is used
as a fallback and a WARN is logged.

The deep-link target lives on the `ShareLink` row (`oneof target`), so a
`/go/{code}` URL is bare of *target* parameters. Legacy
`?gear_id=`/`?request_id=`/`?experience_id=` query params (minted by the removed
`GetOrCreateInviteLink` shim) are no longer honored (#2562).

**The one exception is the `to` destination hint** (#2876). It does not name a
target — the row still does that — it names which *surface of the resolved
target* the reader was promised. Closed vocabulary of one:

| `to` | Meaning |
| --- | --- |
| `discuss` | Land in the community's discussion, and address the reader as an existing member rather than an invitee. |

Only the member-joined ("say hi") notification sets it, in
`server/notifications/deeplink.go`; the SSR community landing validates it
(anything unrecognized is ignored) and forwards it as `tab=discuss` into the
`/group/{id}` hand-off. The client `/go/:code` redirect forwards it too, so an
app user and a browser user following the same link end up in the same place.

It is a **presentation** signal, never an authorization one: a forged
`?to=discuss` gets the same public page and the same join CTA, and membership is
still established by `AcceptInvitationLink`.

### 3. Link Validation

The `CheckInvitation` RPC validates short codes without requiring authentication:

```go
CheckInvitation(short_code) → {
    is_valid: bool,
    community_id: string,
    community_name: string,
    inviter_name: string,
    community_image_url: string,
    num_members: int,
    max_members: int,
    error_message: string,
}
```

Validation checks:

1. Short code exists in database
2. Link is not revoked
3. Community exists
4. Inviter exists

Note: Capacity is checked but doesn't invalidate the link — it's reported via `num_members` and `max_members` so the UI can show appropriate messaging.

### 4. Server-Rendered Invite Page

When a browser requests `/go/{code}`, the Go web service looks up the row and
renders the landing for its target — `community.html`, `event.html`,
`gear.html`, or `request.html` — each with Open Graph metadata for rich link
previews.

All four are **phone-first**: a public, pre-auth page showing the thing, with a
single primary CTA that hands off into the Flutter Web bundle
(`/group/{id}`, `/event/{id}`, `/item/{id}`, `/need/{id}`) where the visitor
verifies a phone and the action completes. None of them requires an app
install. A secondary "Open in Ripls" hint covers the residual case (in-app
webviews, copy-paste, typed URLs) where the app is installed but Universal
Links didn't fire.

The **community landing** (#2875) shows the group photo, name, member count,
description, and the inviter's first name, with a "Join the group" CTA. It
deliberately shows a member **count** and never the roster: the page is
readable by anyone holding an 8-character code. It replaced an install-only
card — an "Open Ripls App" button, a 2-second timeout, then TestFlight/Play
badges — which was the last landing that dead-ended a recipient without the
app. That card's install/manual-code recovery affordances survive on the error
surface, where they still help someone whose link didn't resolve.

**Localization (#2090):** all four SSR landing templates (`invite.html`,
`event.html`, `gear.html`, `request.html`) render their visitor-facing
copy — CTAs, error states, OG/title text, the event's weekday/month date
labels — through the server `l10n` catalog (`web.*`/`common.*` keys, en+es)
in the visitor's `Accept-Language`, falling back to English. Each render
sets `Vary: Accept-Language` (so a shared cache keys on locale) and stamps
`<html lang>`. The copy is built in Go and passed to the templates as
finished strings; see `docs/server/l10n.md`.

**Nameless (ad-hoc) communities:** every item-target `/go/{code}` link is
scoped to the item's per-item ad-hoc origin community (see
`ad_hoc_communities.md`), which has no name. `communityDisplayName`
(`service.go`) substitutes a generic public label ("a Ripls group") so the
plain community invite page (no item) never renders an empty name. The
per-item builders (event, gear, request, and the item variants of the invite
page) instead carry a `CommunityIsNamed` flag (`community.GetName() != ""`)
and omit the "in {CommunityName}"/kicker phrase entirely for a nameless
community, rather than rendering the generic label in item copy. This invariant
is enforced server-side in `GetOrCreateShareLink` and `ShortLinkCodeForEntity`:
both resolve the ad-hoc community regardless of the mint-time `community_id`
argument, so no caller path can leak a named community into an item share
preview.

### 5. Accepting an Invitation

**For authenticated users:**

```
Click link → App opens → AcceptInvitationLink RPC → Join community → Navigate
```

**For unauthenticated users following a `/go/{code}` link on the web:**

```
/go/{code} → community landing → "Join the group"
  → /group/{id}?intent=join&code={code} → /verify-phone (phone OTP)
  → PhoneRegister (joins the link's community) → back to /group/{id} → community
```

**For unauthenticated users reaching `/invite` directly** (custom-scheme deep
link, deferred install referrer, manual code entry):

```
Click link → RegisterScreen (invitation hero + auth methods) → Complete auth → Auto-accept
```

The `AcceptInvitationLink` RPC:

1. Validates the short code
2. Checks if user is already a member (idempotent — returns success)
3. Checks community capacity
4. Creates `CommunityUser` membership record
5. Records a community event for the activity feed

### 6. Link Revocation

Members can revoke their share links:

```
RevokeShareLink(short_code) → Sets is_revoked=true on the row owned by the caller
```

Authorization: the caller must be the link's inviter (`link.InviterId == authInfo.UserID`). No community-membership re-check is required — a user who has left the community should still be able to revoke a link they minted before leaving.

Idempotent — revoking an already-revoked link returns success without re-writing the row.

Revoked links return an error when validated. The member can generate a new link afterward via `GetOrCreateShareLink`.

## Data Model

### ShareLink

Share links are stored as a single polymorphic `ShareLink` model
(`proto/ripls/models/share_link.proto`). The `oneof target` identifies what the
link points at — the community itself (a plain invite) or a specific item within
it. The populated variant is itself the discriminator.

```protobuf
message ShareLink {
    string id = 1;                    // Primary key
    string short_code = 2;            // 8-char alphanumeric code for URLs
    string inviter_id = 3;            // User who created the link
    string community_id = 4;          // Community the link is scoped to (always set)
    oneof target {                    // Exactly one variant set
        string community_invite_id = 5; // Plain "join this community" invite (mirrors community_id)
        string gear_id = 6;             // Link to a specific gear item
        string transfer_id = 7;         // Link to a specific loan/giveaway
        string request_id = 8;          // Link to a specific request
        string experience_id = 9;       // Link to a specific experience (event)
    }
    bool is_revoked = 10;             // Manual revocation flag
    int64 created_at_unix_sec = 11;   // Creation timestamp
    optional string provisional_user_id = 12; // Tied provisional account, merged on registration
}
```

The deep-link target (gear/request/experience) lives in the `oneof target`, so
URLs produced by `GetOrCreateShareLink` are bare (`/go/{code}`); the client reads
the target back from `CheckInvitation` (`target_kind`/`target_id`), and the SSR
landing pages read it straight off the row. The legacy `GetOrCreateInviteLink`
shim that appended `gear_id`/`request_id`/`experience_id` query params was removed
in #2058, and the server- and client-side handling of those legacy query params
was dropped in #2562: an already-minted decorated URL still resolves (the
recipient joins the community) but no longer carries the item into a rich preview
or an in-app deep link.

> **Legacy model:** the original `CommunityInvitationLink`
> (`proto/ripls/models/community.proto`) is superseded by `ShareLink` — the
> one-time backfill job that mirrored its rows into `ShareLink` with
> `community_invite_id` set (`server/jobs/share_link_backfill.go`) was
> deleted in #2643 after verifying as a no-op in production, closing #2056.
> All live RPC paths read `ShareLink` exclusively; the legacy table remains
> only for cascade-delete/restore and hard-delete handling of any surviving
> rows.

Constraints:

- One active (non-revoked) link per inviter+target pair
- Short codes are 8-char alphanumeric (charset excludes confusable characters)
- Links never expire (only revocation invalidates them)

### CommunityUser

When an invitation is accepted, a membership record is created:

```protobuf
message CommunityUser {
    string community_id = 1;
    string user_id = 2;
    string inviter_id = 3;           // Tracks who invited this member
    int64 created_at_unix_sec = 4;
}
```

## Capacity Management

Communities have a maximum capacity of 32 members. This is enforced at:

1. **Registration** - New users can't register via invitation to a full community
2. **AcceptInvitationLink** - Authenticated users can't join a full community
3. **Landing page** - Shows "community full" messaging before login/register

The limit is defined in the shared community library
(`server/community/constants.go`) and referenced as
`communitylib.MaxCommunityMembers`:

```go
const MaxCommunityMembers = 32
```

## Deep Linking

Invitation links support deep linking to specific content. The target lives on
the `ShareLink` row's `oneof target`; a `/go/{code}` URL is bare, and the target
is resolved from the row — server-side by the SSR landing pages and client-side
via `CheckInvitation`'s `target_kind`/`target_id`. A typed gear/request/experience
share link routes to its own phone-first SSR landing page (`gear.html` /
`request.html` / `event.html`); a community-invite link routes to
`community.html`, which is phone-first too (#2875).

Legacy `?gear_id=`/`?request_id=`/`?experience_id=` query-param URLs (minted by
the removed `GetOrCreateInviteLink` shim) are no longer honored (#2562).

## URL Schemes

The app handles two URL schemes:

1. **Custom scheme**: `ripls://invite?token=<short_code>`
   - Handled directly by the app
   - Used for app-to-app deep links

2. **Universal/App Links**: `https://ripls.app/go/<short_code>`
   - Requires associated domains configuration
   - Falls back to the server-rendered landing if the app isn't installed
   - `/go/*` is configured in `apple-app-site-association` for iOS

   **Only `/go/*` is universal-linked**, deliberately. The web guest routes the
   landings hand off to (`/group/*`, `/event/*`, `/item/*`, `/need/*`) must
   stay browser-only — universal-linking them would bounce the visitor into the
   app mid-flow, which is the dead end the phone-first landings exist to fix.

## Bootstrap Case

For the first user (when no users exist in the system), any short code is accepted. This allows initial setup without requiring a valid invitation link.

## Error Handling

| Error          | Cause                     | User Experience                         |
| -------------- | ------------------------- | --------------------------------------- |
| Invalid code   | Short code doesn't exist  | "This invitation link is not valid"     |
| Revoked        | Inviter revoked the link  | "This invitation link has been revoked" |
| Community full | 32 members reached        | "Community is at capacity"              |
| Already member | User already in community | Silent success (idempotent)             |

The "User Experience" strings above are the English defaults; the SSR
landing renders them in the visitor's `Accept-Language` via the
`web.error.*` catalog keys (#2090).

## Event Tracking

Successful invitation acceptance creates a community event:

```
EventType: COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED
ActorId: new member's user ID
InvitationShortCode: the short code used
GearId: optional, if deep-linked to gear
```

This appears in the community's activity feed.

## Security Considerations

- Short codes are 8 characters from a 54-char alphabet (~46 bits of entropy), generated with `crypto/rand`
- No authentication required for `CheckInvitation` (allows landing page validation)
- Uniqueness checked against database before use (retry up to 10 times on collision)
- Revocation is permanent — cannot un-revoke a link
- Rate limiting should be applied to public endpoints

## Related Files

| File                                                | Purpose                                          |
| --------------------------------------------------- | ------------------------------------------------ |
| `server/services/community/share_links.go`          | `GetOrCreateShareLink`, `RevokeShareLink`, `lookupShareLink`, polymorphic target dispatch |
| `server/services/community/invitations.go`          | `AcceptInvitationLink`, short code generation |
| `server/services/login/registration.go`             | CheckInvitation RPC                              |
| `server/services/web/service.go`                    | `HandleInvitePage` — looks up the short code and dispatches by target |
| `server/services/web/community_page.go`             | `handleCommunityLanding` — the phone-first community landing (#2875) |
| `server/services/web/templates/community.html`      | Community landing HTML template                  |
| `server/services/web/templates/invite.html`         | Shared share-link error surface                  |
| `app/lib/presentation/screens/web/web_community_screen.dart` | Web guest join surface at `/group/{communityId}` |
| `app/lib/presentation/screens/auth/register_screen.dart` | Unauthenticated invitee landing (RegisterScreen) |
| `app/lib/core/router/app_router.dart`               | Deep link routing                                |
| `website/content/invite.html`                       | Static web fallback page                         |
| `website/embed.go`                                  | Shared CSS/asset embedding for Go server         |
| `proto/ripls/api/community_service.proto`           | RPC definitions                                  |
| `proto/ripls/models/share_link.proto`               | ShareLink model (live)                           |
| `proto/ripls/models/community.proto`                | CommunityInvitationLink model (legacy)           |
