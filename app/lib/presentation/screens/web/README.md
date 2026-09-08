# presentation/screens/web

Flutter **Web-only** guest entry points. Each screen here is the second half of
a `/go/{code}` share link: the Go web service renders a public, pre-auth SSR
landing (`server/services/web/templates/`), and its single primary CTA hands
off to one of these routes inside the Flutter Web bundle, where the visitor
verifies a phone and the action completes.

The mobile app never visits these routes — it deep-links to the normal detail
screens (`/gear/{id}`, `/request/{id}`, `/experience/{id}`) instead.

## Key files

- **`web_item_action_screen.dart`** — `WebItemActionScreen`, mounted at
  `/item/{gearId}` and `/need/{requestId}` (#2492 WEB-3/WEB-4). One screen, two
  `WebItemKind`s, because gear and requests differ only in which content view
  they compose and which action auto-fires (`ExpressInterest` /
  `OfferToFulfill`).
- **`web_community_screen.dart`** — `WebCommunityScreen`, mounted at
  `/group/{communityId}` (#2875). The community analog: a plain community
  invite whose action *is* the join, so there is no separate auto-fire step —
  the join notifier is both the gate and the payload. **This one is no longer
  web-only**: a member-joined push tap routes here on mobile as well (#2876),
  where it skips the join gate and renders the ordinary `CommunityPublicScreen`.
  Its name and this directory are now slightly wrong for it; the rename is
  deferred because `web-community-screen` is a published e2e locator.

`WebExperienceScreen` (event RSVP, `/event/{id}`) predates this directory and
lives under `screens/experience/`; it follows the same shape.

## The shape they share

1. **Unauthenticated + an intent carrier** (`?intent=…&code=…` from the SSR
   CTA) → `context.go('/verify-phone?…')`, threading the entity id so the
   router's post-auth redirect can send the guest back here.
2. **Authenticated + a share code** → join the community first via
   `eventInviteJoinProvider(shortCode)` (idempotent `AcceptInvitationLink`),
   because the community-scoped access gate rejects the content RPCs otherwise.
3. **Joined** → render the content, auto-firing the action where there is one.
4. Join and action failures get a `liveRegion` retry surface, not a dead end.

## When to add a screen here

Only when a new share-link target needs a *public, pre-auth* web landing of its
own. A surface that only authenticated users reach belongs in its normal
feature directory — these screens exist specifically to carry someone who has
no account and no app across the auth boundary without losing their intent.

## Gotchas

- **Don't add these paths to `apple-app-site-association`.** Only `/go/*` is
  universal-linked. Universal-linking `/item`, `/need`, or `/group` would hand
  the flow back to the native app, which is the exact failure these screens
  exist to fix.
- **Both `postRegistrationDestination` call sites in `PhoneAuthScreen` must
  carry the same params.** The phone-OTP success path and the
  existing-account hop each build a destination; threading a new entity id
  through only one of them leaves the below-the-fold Google/email guests
  stranded on `/` while the OTP path looks fine.
- **e2e locates these by `Semantics(identifier:)`** (`web-gear-screen`,
  `web-request-screen`, `web-community-screen`) — see
  `docs/client/testing/semantics_identifiers.md`.
