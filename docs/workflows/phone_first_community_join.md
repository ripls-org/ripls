---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Phone-first workflow for plain community invites (#2875) — a non-app guest opens a /go/ community link, sees the group on the web, registers via Firebase phone OTP, and lands inside the community as a member without installing anything.
  globs: [server/services/web/community_page.go, server/services/web/templates/community.html, app/lib/presentation/screens/web/web_community_screen.dart, app/lib/core/utils/post_registration_destination.dart, app/lib/presentation/screens/auth/phone_auth_screen.dart, e2e/tests/workflows/phone-community-join-full-loop.spec.ts]
  triggers: [phone-first, phone-otp, community-invite, web-join, invite-link, group-landing, auth-emulator]
  lens: [workflow, domain, client, server]
  domain: registration
freshness:
  verified_commit: "041b93570"
  verified_on: "2026-08-09"
---
# Phone-First Invite → Verify → Join Workflow (communities)

The phone-first pivot (epic #2492) let a person with **no Ripls account and no
app** respond to a shared *item* on the web. A plain **community invite** — the
oldest invitation primitive, and still what every named community's Invite sheet
mints — was left behind: it rendered an install-only card with an "Open Ripls
App" button, a 2-second timeout, and app-store badges, with no way to continue in
a browser. #2875 closed that gap. This doc describes the loop; the Playwright
spec `e2e/tests/workflows/phone-community-join-full-loop.spec.ts` is its
executable form.

## What makes this one different

The three item workflows ([events](./phone_first_rsvp.md),
[gear](./phone_first_gear_interest.md), [requests](./phone_first_request_offer.md))
have two steps after auth: join the item's ad-hoc community, then auto-fire the
action (`RSVPToExperience` / `ExpressInterest` / `OfferToFulfill`). On a
community invite **the join is the action**, so there is no separate auto-fire —
`eventInviteJoinProvider` is both the gate and the payload, and the
postcondition is membership itself.

## The pieces this composes

- **The SSR community landing** — `handleCommunityLanding` +
  `templates/community.html` (`server/services/web/community_page.go`), the
  fourth landing behind the `HandleInvitePage` fork.
- **The web community guest screen** — `WebCommunityScreen` at
  `/group/{communityId}` (`app/lib/presentation/screens/web/`).
- **Phone registration** — `PhoneRegister` already joins the short code's
  community and runs `provisional.PromoteByPhone`; no server auth change was
  needed.
- **The idempotent join** — `eventInviteJoinProvider(shortCode)` wrapping
  `AcceptInvitationLink`, reused unchanged from the event flow.

## How the loop runs

1. A host invites someone from a named community's Invite sheet, producing a
   `/go/{code}` link whose `ShareLink` row has `community_invite_id` set.
2. The guest opens `/go/{code}` → the SSR community landing shows the group
   photo, name, member **count**, description, and the inviter's first name,
   with a **Join the group** CTA. No roster: the page is readable by anyone
   holding an 8-character code.
3. Tapping **Join the group** routes to the Flutter web app at
   `/group/{id}?intent=join&code={code}&n={name}&img={hero}`. Unauthenticated,
   the guest is sent to `/verify-phone` with `community_id` and `token`
   threaded through.
4. The guest registers via **phone OTP** (`PhoneAuthScreen`, framed "Confirm
   your phone to join {group}"): enter phone → receive code → enter code →
   enter name → `PhoneRegister`.
5. Server: `PhoneRegister` creates the `AUTH_METHOD_PHONE` user, joins the
   invite link's community, and promotes any provisional placeholders for that
   number.
6. The web app returns to `/group/{id}?intent=join&code={code}`; now
   authenticated, `WebCommunityScreen` joins (idempotent — the registration
   already did it) and renders the community.
7. The guest is a real member. No app install.

An **already-authenticated** visitor skips steps 3–5 entirely: the screen joins
and shows the community directly. That case is why the route is keyed on the
community id rather than the share code — it can be reached without a link.

## Workflow Example — Guest joins a community via phone OTP

Exercised by `e2e/tests/workflows/phone-community-join-full-loop.spec.ts`
against the **Firebase Auth Emulator** (deterministic OTP).

### Preconditions

- A host is registered and owns a named community.
- The guest is not registered and has no app.

### Steps

1. **Host** (RPC, off-camera): creates the community and mints a community
   invite link.
2. **Guest**: opens `/go/{code}` → sees the SSR community landing.
3. **Guest**: taps **Join the group** → lands on
   `/group/{id}?intent=join&code={code}` → is routed to `/verify-phone`.
4. **Guest**: enters the phone number, receives + enters the OTP, enters a
   name, submits.
5. **Server**: `PhoneRegister` creates the user and joins the community.
6. **Client**: returns to `/group/{id}`; the join settles and the community
   renders.

### Postconditions

- The guest is a real `AUTH_METHOD_PHONE` user and a member of the community
  (asserted over RPC via `GetCommunity.numMembers`, not from pixels — a screen
  that rendered but never joined would pass a screenshot check).
- The browser is at `/group/{communityId}`, showing the community.
- The landing rendered **no** install-only affordances ("Open Ripls App",
  "download the Ripls app") — the regression guard for this issue.

## Gotchas

- **Only `/go/*` is universal-linked.** `/group/*` must stay out of
  `apple-app-site-association`; adding it would hand the flow back to the
  native app, which is the dead end this workflow exists to remove.
- **Both `postRegistrationDestination` call sites matter.** `PhoneAuthScreen`
  builds a destination in `_navigateToHome` (register success) and in
  `_goToLogin` (the existing-account hop, via `?from=`). A carrier threaded
  through only the first leaves guests who take the below-the-fold Google/email
  option stranded on `/` while the OTP path looks fine.
- **A community-invite link can point at a nameless (ad-hoc) community.**
  `ShortLinkCodeForEntity` mints one as the no-entity fallback for
  community-wide notification links, so the landing renders the generic
  "a Ripls group" label and takes the headline variant that omits the name
  rather than splicing the label mid-sentence.
- **The same landing serves the "say hi" notification** (#2876), whose
  recipients are existing members rather than invitees. That link carries
  `?to=discuss`, which switches the landing to member framing (no "invited you"
  row, "Open the discussion" CTA) and threads `tab=discuss` into the
  `/group/{id}` hand-off so the conversation opens on arrival. The hint is a
  closed vocabulary validated server-side; an unrecognized value renders the
  ordinary invite. See `invitations.md` → destination hint.
