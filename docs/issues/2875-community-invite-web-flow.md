# Issue #2875: Community invitation does not lead to a web-based registration flow

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2875
**Filed:** 2026-08-09 | **Last activity:** 2026-08-09
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/invitations.md`, `docs/workflows/phone_first_rsvp.md`,
`docs/workflows/phone_first_gear_interest.md` (globs),
`docs/registration_and_login.md`, `docs/client/deep_linking.md`,
`docs/ad_hoc_communities.md`, `docs/server/l10n.md`, anchors
(`docs/client/architecture.md`, `docs/client/conventions.md`,
`docs/server/architecture.md`, `docs/server/conventions.md`,
`docs/proto_conventions.md`, `docs/users/priorities.md`,
`docs/users/personas.md`, `docs/testing/architecture.md`), review prompts
(security, accessibility, error_handling, observability, performance, testing,
api_design)
**Delivery:** one PR (per Bryan). The phases below are *sequencing within that
PR*, not separate ships — see the note under Implementation Plan.
**Acceptance test:** `e2e/tests/workflows/phone-community-join-full-loop.spec.ts`
(Phase 3) — a Playwright spec driving the real Flutter Web bundle through the
phone-first community-invite loop. Required for merge.

## Problem Statement

`/go/{code}` is one URL with four destinations. Three of them — event, gear/
transfer, request — are phone-first web landings: a public SSR page that shows
the thing, with a single primary CTA that hands off into the Flutter Web bundle
(`/event/{id}`, `/item/{id}`, `/need/{id}`), verifies a phone, joins the
community, and fires the action. The fourth — a plain **community invite**
(`ShareLink.community_invite_id`) — falls through to `invite.html`, which
predates the phone-first pivot (#2492): it renders "You're Invited!", a community
photo, and an **Open Ripls App** button that navigates to `ripls://invite`, then
after a 2s timeout says *"Don't have the app? Download it below"* and shows
TestFlight / Google Play buttons plus a manual invite code. On desktop it
short-circuits straight to *"To accept this invitation, download the Ripls
app."* There is **no path to continue in the browser at all**.

The routing fork is explicit at `server/services/web/service.go:234-245`: typed
item links go to `handleEventLanding` / `handleGearLanding` /
`handleRequestLanding`; anything else falls through to the generic
`invite.html`. So a community invite — the original invitation primitive, and
still the link every named community's Invite sheet mints — is the one invite
shape that requires an app install before the recipient can do anything.

This matters beyond aesthetics. The whole point of #2492 was that a person with
no account and no app can respond on the web and become a real member. A
community invite is the *broadest* front door (join the group, not just this one
item), and it's the only one still gated on an app-store round trip.

Two contradictions worth naming up front, neither in the issue text:

1. **The community-invite landing is also the off-app notification fallback.**
   `ShortLinkCodeForEntity` mints a community-invite link when a notification has
   no entity target (`docs/invitations.md` §Components). So every "someone joined
   your community" email/SMS lands on this same install-the-app page — which is
   exactly what **#2876** reports, including for users who are *already signed
   in*. #2875 and #2876 are the same page; they want different things from it.
2. **The client already has a web registration surface for this link.**
   `/invite?token={code}` → `RegisterScreen` hosts the invitation hero and every
   auth method. It works. The SSR page simply never links to it. So this is not
   "build a web flow from nothing" — it is "stop dead-ending, and bring the
   community invite up to the phone-first bar the item invites set."

## Context from Comments

**There are no comments on this issue.** The body is one paragraph plus a
screenshot of the current `invite.html` render. That's thin for a change that
touches SSR, routing, auth, and l10n — but the intent is unambiguous ("replace
this with a web onboarding flow that matches the other kinds of invitations")
and the "other kinds" are fully specified in code and docs, so the ambiguity is
in *scope and polish*, not direction. The Open Questions section carries the
decisions that would otherwise have been asked in comments.

Filed the same minute as **#2876** by the same reporter, against the same page.
Treat them as a pair.

### UI Investigation — deliberately skipped

`ripls-ui-investigate` was not run. The skill reserves it for UI-observable
*misbehavior* that needs bisecting to a layer; here the layer is already proven
from code and from a committed test:

- `server/services/web/service.go:234-245` routes community-invite links past the
  three item landings into `renderInvitePage`.
- `server/services/web/templates/invite.html:118-225` is the install-the-app
  script; its only in-browser affordance is `openApp()` → `ripls://invite`.
- `e2e/tests/workflows/invite-link-deep-link-paths.spec.ts` test 1 **already
  asserts this exact rendering as expected behavior** ("bare community
  `/go/{code}` renders the community-invite SSR landing" → "You're Invited",
  "invited you to join", and no item CTA), with `javaScriptEnabled: false`
  because the page auto-navigates to `ripls://` on a mobile UA.

A hermetic reproduction would re-derive a verdict of "UX gap" that the repo
already encodes. That spec is also the regression this change has to update —
see Phase 3.

## Potential Solutions

### Option A: Add a web CTA to the existing `invite.html`

- **Approach:** Leave the page structure alone; add a primary "Join in your
  browser" button that links to `/invite?token={code}`, and demote the store
  links to a secondary block. `RegisterScreen` (all auth methods) handles the
  rest, unchanged.
- **Pros:** Smallest possible diff — one template, one l10n key, no new route, no
  new screen, no client state. Ships in an afternoon. Immediately unblocks
  desktop recipients, who today get nothing.
- **Cons:** Doesn't "match the other kinds of invitations," which is the literal
  ask. The recipient still sees a 2010-era store-badge page first; the hero is a
  logo header and a `deeplink.css` card, not the `event.css` hero the item
  landings use. Post-auth the guest is dumped at `/` (home) rather than in the
  community they were invited to. Sign-up is register-screen-first, not
  phone-first, so the community invite converts worse than every item invite.
  Does nothing for #2876.
- **Effort:** Small.

### Option B: Full parity — SSR community landing + Flutter Web community guest screen

- **Approach:** Mirror the three item landings exactly.
  1. **Server:** new `handleCommunityLanding` + `templates/community.html`
     (styled with the shared `event.css`, like gear/request), rendered from the
     same `HandleInvitePage` fork. Hero = community photo + name; meta pill =
     member count; host row = inviter first name + "invited you"; description;
     one primary CTA → `/group/{communityId}?intent=join&code={code}&n=…&img=…`;
     the same `open-in-app` hint and at-capacity banner the item pages use.
     `invite.html` stays as the shared **error** surface (`renderInviteError`)
     and loses its valid branch.
  2. **Client:** new `/group/:communityId` route → `WebCommunityScreen`, the
     community analog of `WebItemActionScreen`. Unauthenticated + intent →
     `/verify-phone?token=…&community_id=…`; authenticated → idempotent
     `AcceptInvitationLink` via the existing `eventInviteJoinProvider(shortCode)`
     → render `CommunityPublicScreen(communityId:)`.
  3. **Plumbing:** `postRegistrationDestination` gains a `communityId` branch;
     the router's authenticated-on-auth-screen redirect gains `community_id`
     alongside `experience_id`/`gear_id`/`request_id`; `PhoneAuthScreen` gains a
     `communityId` param and community framing (its
     `PhoneConsentContext.community` case **already exists** as the fallback).
- **Pros:** Literally what the issue asks for. Reuses the shipped machinery
  end-to-end — `eventInviteJoinProvider` already does exactly the required join
  and is already idempotent; `PhoneRegister` already joins the short code's
  community and runs `PromoteByPhone`; `looksLikeAssetPath` already SPA-falls-
  back arbitrary client routes, so no server routing change is needed for
  `/group/*`. **No proto change at all** — `ShareLinkTargetKind` already has a
  `COMMUNITY` variant and `CheckInvitation` already returns community name,
  image, inviter, and member counts. Leaves the guest *in the community* rather
  than at `/`, which is where #2876 needs them too, and the `/group/{id}` route
  is the seam that closes #2876 with a `tab` param.
- **Cons:** Largest surface: one new Go file + template + ~15 l10n keys × 2
  locales, one new Flutter screen + route, three shared helpers touched, a new
  e2e spec, and an existing e2e spec rewritten. Touches auth routing, which is
  the highest-blast-radius code in the client.
- **Effort:** Large (but almost entirely additive and pattern-matched).

### Option C: Route the community CTA into the existing `/invite` route, made phone-first

- **Approach:** Server-side as in Option B (real SSR community landing), but the
  CTA targets `/invite?token={code}` and the *existing* `/invite` route learns a
  web branch: unauthenticated + `kIsWeb` → `/verify-phone?token=…`; authenticated
  → accept then land on the community instead of `/`.
- **Pros:** No new route or screen; the short code stays the only identifier in
  the URL (no community id in a public URL, no `postRegistrationDestination`
  change if `/invite` is also the post-auth destination). Fixes desktop and
  phone-first conversion together.
- **Cons:** `/invite` is a heavily overloaded route — custom-scheme deep links,
  deferred install-referrer recovery, first-launch registration, manual code
  entry, mobile auto-accept, and the `EventInviteJoinGate` branch all land there.
  Adding a fourth auth-dependent web branch to a builder that already nests two
  `FutureBuilder`s inside a router `builder` is the opposite of what
  `client/architecture.md` asks for. It also diverges from the three item flows
  (which use dedicated `/event`,`/item`,`/need` screens), so "matches the other
  kinds of invitations" stays half-true, and #2876 has no clean place to hang a
  `tab` param.
- **Effort:** Medium.

## Recommendation

**Option B**, shipped in the phases below, with Phase 1 designed to stand alone.

The issue asks for parity and parity is what's cheap here: three landings already
exist to copy, the join notifier already exists and is already idempotent, the
phone screen already has a `community` consent context waiting unused, and the
SPA fallback already serves arbitrary client routes. The genuinely new code is
one Go file, one template, one Flutter screen, and l10n. Option A is a patch that
leaves the community invite converting worse than every item invite — and the
community invite is the *widest* front door in the product, which by
`users/priorities.md` (the latent host trying to get people in) is the wrong
place to under-invest. Option C saves a route at the cost of piling a fourth
auth-conditional branch onto `/invite`, which is already the most tangled builder
in `app_router.dart`.

The trade I'm making explicitly: Option B touches auth routing, and auth routing
regressions are expensive. Phases 1 and 2 are ordered so the SSR page can be
eyeballed in dev before the router change lands, and every routing edit is
additive (a new `community_id` arm beside three existing ones, never a change to
the existing arms). Both land in the same PR.

One thing Option B does *not* do, deliberately: it does not preserve the Android
Install Referrer / iOS clipboard recovery machinery as the primary path. That
code (`invite.html:127-161`, from #1099) is how a community invite survives an
app install today, and the three item landings already dropped it. Phase 1 keeps
it on the secondary open-in-app affordance rather than deleting it — see Open
Questions.

## Open Questions

- **Route name: `/group/{communityId}` vs `/community/{id}` vs `/join/{code}`?**
  — Recommendation: **`/group/{communityId}`**. The existing guest routes are
  short and noun-shaped (`/event`, `/item`, `/need`), the product's public label
  for a community is already "group" (`common.adhoc_community_name` = "a Ripls
  group"; the promote nudge is "name-this-group"; `CommunityPublicScreen` is
  documented as "the community (group) profile"), and keying on the community id
  rather than the code lets an authenticated member reach it without a share
  link — which #2876 needs. There is no `/group/` pattern in `server/routes.go`,
  so it SPA-falls-back cleanly, and `apple-app-site-association` universal-links
  only `/go/*`, so iOS won't hijack it.

- **Phone-first, or keep the community invite on the all-methods register
  screen?** — Recommendation: **phone-first**, matching the item flows.
  `PhoneAuthScreen` already renders Google and inline email below the fold for
  guest-action flows (#2595), so nothing is lost, and `PhoneConsentContext.
  community` is already the fallback branch — the consent copy is written.

- **How much of the member roster does the pre-auth page show?** —
  Recommendation: **member count only — no names, no faces.** The event landing
  shows attendee first names because those people opted into a specific public
  event; a community roster is not that. The codebase already states this
  posture: *"a public pre-auth page must not leak member names"*
  (`service.go:57-62`). Show the **inviter's first name** (already shown today,
  and it's the trust signal that makes the link legible) and `NumMembers`. This
  is the security-review-relevant decision on this change.

- **Keep the Play install-referrer + iOS clipboard deferred-deep-link recovery
  (#1099)?** — Recommendation: **keep it, attached to the secondary open-in-app
  affordance**, not the primary CTA. It's ~35 lines, it already works, and
  dropping it silently regresses Android install attribution for the one link
  type that still commonly precedes an install. The item landings' Android
  `intent://` fallback goes to a bare Play listing; the community landing can do
  strictly better at no cost.

- **Does `invite.html`'s valid branch get deleted, or kept as a fallback?** —
  Recommendation: **delete it**; keep `invite.html` as the error-only surface it
  already is for all four link types (`renderInviteError`). Server-rendered copy
  has no client version skew, so the now-dead `web.invite.title_valid`,
  `og_desc_valid`, `youre_invited`, `invited_to_join`, `open_app` keys go in the
  same PR (both `en.toml` and `es.toml` — `server/l10n/source/` parity). The
  store-button, reopen-CTA, and invite-code keys stay: they're still reachable
  from the error surface and the recovery path.

- **At-capacity copy: reuse `web.capacity.body_respond` or add
  `web.capacity.body_join`?** — Recommendation: **add `body_join`**. "Respond" is
  wrong for a join, and `buildCapacityStrings` already takes the body key as a
  parameter precisely so each landing picks its verb.

- **Nameless (ad-hoc) communities on a community-invite link.** — Recommendation:
  render them, don't error. `ShortLinkCodeForEntity` mints community-invite links
  as the no-entity fallback for community-wide notifications, and that community
  may be ad-hoc. Reuse `communityDisplayName` + the `CommunityIsNamed` flag
  exactly as gear/request do: the generic "a Ripls group" label for the title,
  and drop "in {community}" phrasing. `nameless_community_test.go` gets a case.

- **Does the guest land on `CommunityPublicScreen` or `CommunityContentView`?** —
  Recommendation: **`CommunityPublicScreen`** — it takes only `communityId`
  (the content view needs name/description threaded in) and is the group profile.
  Two things to verify in Phase 2 rather than assume: it mixes in
  `SwipeToCloseMixin` (behavior on web with no navigation stack), and it fans out
  to workshop/impact providers, which is a heavy first paint for a guest who just
  registered.

## Implementation Plan

**This ships as one PR.** The phases are build order, not release increments —
Phase 1 is verifiable on its own in dev (the CTA lands on the SPA shell until
Phase 2 exists), which is why it goes first, but nothing merges until Phase 3 is
green. Two consequences worth stating because they bite:

- **No phase markers in code comments.** `scripts/check_ephemeral_comments.js`
  (`npm run lint:ephemeral-comments`) fails the build on "Phase 1", "new in this
  PR", and friends, in both `*.go` and `*.dart`. Reference `#2875` instead.
- **The whole gate runs at once.** `npm run lint` chains `lint:go` (fmt,
  golangci via `server/.golangci.yml`, `check-goroutines`, `codeinternal`,
  `event-terminology`, `active-community-gate`, **`l10n-parity`**,
  **`no-inline-user-strings`**), `lint:dart`, `lint:dart:size`,
  `lint:dart:dead`, `lint:dart:a11y`, `lint:dart:colors`,
  `lint:ephemeral-comments`, `lint:todo-refs`, and `lint:context-map`. Two of
  those are load-bearing for this change specifically: `no-inline-user-strings`
  means every new visitor-facing Go string goes through the catalog, and
  `l10n-parity` means `en.toml` and `es.toml` move together.

**No proto change, and no registry work.** `ShareLinkTargetKind.COMMUNITY` and
`CheckInvitation`'s `community_name` / `community_image_url` / `inviter_name` /
`num_members` / `max_members` already exist; the SSR page reads storage directly
and the client uses `AcceptInvitationLink` + `GetCommunity`. Joining still emits
the existing `COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED`, so **no new
`CommunityEventType`** — and therefore no undo-registry or digest-registry entry
(`server/conventions.md` §Adding New CommunityEventType Values). If a task in
this plan seems to need a new enum value, that's a signal the design drifted.

### Phase 1: SSR community landing

**Goal:** `/go/{code}` for a community invite renders a phone-first landing that
looks and behaves like the event/gear/request landings, with a CTA into the web
app.

**Tasks:**
- [ ] Add `server/services/web/community_page.go`: `communityPageData` +
      `handleCommunityLanding` + `buildCommunityPageData` + `renderCommunityPage`,
      modeled on `request_page.go` (same localizer threading, same
      `renderInviteError` fallbacks for missing/soft-deleted community, same
      `Cache-Control: public, max-age=30` + `Vary: Accept-Language`).
- [ ] Add `templates/community.html` using `event.css` — hero (community photo
      via `getCommunityImageURL`, kicker, `<h1>` community name), member-count
      meta pill, inviter row, description, primary CTA
      `/group/{{.CommunityID}}?intent=join&code={{.ShortCode}}&n=…&img=…`,
      at-capacity banner, `open-in-app` hint carrying the Play referrer.
- [ ] Register it in the `HandleInvitePage` fork (`service.go`) and in the
      `template.ParseFS` list + the `//go:embed` directive.
- [ ] Add `web.community.*` l10n keys (kicker, headline, inviter line, CTA label,
      CTA aria, member count with plural, OG title/description in both the named
      and nameless variants) and `web.capacity.body_join` — `en.toml` **and**
      `es.toml`.
- [ ] Delete `invite.html`'s valid branch + its now-dead `web.invite.*` keys;
      keep the error surface, store links, reopen CTA, and code display.
- [ ] Log with the standard fields on the new handler — `operation`,
      `community_id`, `short_code` via `logging.MaskToken` — matching
      `handleRequestLanding`. Where a fetch degrades rather than fails (member
      count, description), carry the explicit "why this is safe to ignore"
      comment the fail-fast rule requires; never a bare warn-and-continue.
- [ ] Update `server/services/web/README.md` — its `ShareLink.target` → template
      routing table is the map of this package and currently says
      `community_invite_id` → `invite.html`, "install-flow CTAs included".
- [ ] Check `website/content/invite.html` (the static nginx fallback listed in
      `docs/invitations.md` §Web) — decide whether it needs the same treatment or
      is genuinely unreachable now, and say which in the PR.
- [ ] Go tests: `community_page_test.go` (valid render, nameless community,
      at-capacity, missing/soft-deleted community → error surface, CTA href
      shape); extend `localization_test.go` and `nameless_community_test.go`.
- [ ] `npm run build` (Go lint + compile) before moving on.

**Validates:** That the SSR fork, hero/OG composition, and localization all work
for a community target — the parts with zero client risk — and that the page
looks right in dev before any routing changes. The CTA lands on the bare SPA
shell until Phase 2, which is the expected intermediate state.

**Security decisions this phase makes** (call them out in the PR body so the
security review has something to sign off on):

- The page adds **community description** to what an 8-char code discloses
  pre-auth. Today `invite.html` shows name, photo, and inviter first name only.
  The description is authored *for* invitees, so this is defensible — but
  truncate it, and treat it as a deliberate widening, not an accident.
- **No member names or faces**, per the Open Question above and the existing
  posture in `service.go:57-62`.
- `n=` and `img=` ride into the CTA href as query values. `html/template`
  escapes contextually inside `href`, which is what makes the community name
  safe there; the item landings rely on the same property. The `img` value is a
  1-hour presigned URL that ends up visible in the browser URL bar and any
  copy-paste of it — accepted precedent from `gear.html`/`request.html`, not a
  new exposure class, but worth naming.
- The landing is a GET with the same query set as today's invite page (community
  + inviter + member count + one media presign), so no new rate-limiting surface
  and no N+1. It *does* newly set `Cache-Control: public, max-age=30`; keep the
  payload viewer-independent (it is) so a shared cache keyed only on
  `Vary: Accept-Language` stays correct.

### Phase 2: Flutter Web community guest screen

**Goal:** The CTA completes the loop: an unregistered guest verifies a phone,
joins, and lands inside the community; an already-authenticated visitor joins and
lands there directly (no register screen, no store badge).

**Tasks:**
- [ ] Add `app/lib/presentation/screens/web/web_community_screen.dart` —
      `WebCommunityScreen`, modeled on `WebItemActionScreen`: unauthenticated +
      intent + code → `context.go('/verify-phone?token=…&community_id=…&n=…&img=…')`;
      authenticated + code → `eventInviteJoinProvider(shortCode).join()` (reused
      as-is), joining/join-failed/retry surfaces, then
      `CommunityPublicScreen(communityId:)`. `Semantics(identifier:
      'web-community-screen')` for e2e locators.
- [ ] Add the `/group/:communityId` `GoRoute`; add `path.startsWith('/group/')`
      to the `isGoingToWebItem`-style auth-screen allowlist so an unauthenticated
      visitor isn't bounced to `/login`.
- [ ] Extend `postRegistrationDestination` with a `communityId` →
      `/group/{id}?intent=join&code=…` branch, and the router's
      authenticated-on-auth-screen redirect to read `community_id`.
- [ ] Add `communityId` to `PhoneAuthScreen`, include it in `_isGuestActionFlow`,
      and give `PhoneAuthHeader` the community framing ("Confirm your phone to
      join {community}"). `_consentContext` already returns
      `PhoneConsentContext.community` — verify its disclosure copy reads right
      as a *deliberate* branch rather than a fallback. For a nameless ad-hoc
      community `n=` is the generic label, so the header reads "…join a Ripls
      group"; confirm that's acceptable rather than suppressing the name clause.
- [ ] **Thread `communityId` through *both* `postRegistrationDestination` call
      sites in `PhoneAuthScreen`** — `_navigateToHome` (the phone-OTP success
      path) and `_goToLogin` (which builds the `?from=` for the existing-account
      hop). Missing the second one is exactly the class of bug that
      `phone-guest-email-register.spec.ts` and `phone-gear-email-register.spec.ts`
      exist to catch: the primary phone path works, the below-the-fold
      Google/email path silently lands the guest at `/`.
- [ ] **Emit the join analytics.** The `/invite` route logs
      `CommunityJoinedEvent(communityId:, joinMethod: 'invite_link')` after a
      successful accept. `WebCommunityScreen` becomes a second join path; if it
      doesn't log the same event, community-join volume silently under-reports as
      the new flow takes traffic.
- [ ] Accessibility, per `client/accessibility.md` + the a11y lint
      (`npm run lint:dart:a11y`): `Tappable`/`IconAction` with `semanticsLabel`
      for every interactive element, `liveRegion` on the joining / join-failed /
      retry surfaces, `AppColors` for every color (the colors lint enforces the
      routing discipline), ≥48dp targets. Prefer **reusing** the existing ARB
      keys — `webEventJoiningCommunity` ("Joining community…"),
      `webEventJoinFailedBanner`, `a11yWebEventJoinRetry`, `a11yWebEventHome` —
      over minting near-duplicates; they are already community-join copy despite
      the `Event` prefix, and new keys mean ARB-parity and orphan-gate work for
      no reader benefit.
- [ ] Add `app/lib/presentation/screens/web/README.md`. That directory holds one
      file today; a second makes it a multi-file directory, which CLAUDE.md
      requires a README for (purpose, key files, when to add here vs. an adjacent
      directory).
- [ ] **Do not add `/group/*` to `apple-app-site-association`.** Only `/go/*` is
      universal-linked, and that's deliberate: the CTA's whole job is to stay in
      the browser. Adding it would hand the flow back to the app and re-open this
      issue.
- [ ] Flutter unit tests: `post_registration_destination` community branch (both
      call sites); router redirect with `community_id`; `WebCommunityScreen` join
      → content transition, join-failure retry, and the unauthenticated →
      `/verify-phone` hop (ProviderContainer + Mockito, per
      `client/conventions.md`). Run with `--reporter expanded`.
- [ ] Verify `CommunityPublicScreen` on web: swipe-to-close with no nav stack,
      and first-paint cost for a just-registered guest.

**Validates:** The end-to-end claim of the issue — a person with no app and no
account gets from a community `/go/` link to being a member, in a browser. Also
proves the `/group/{id}` seam that #2876 will hang a `tab` param on.

### Phase 3: e2e, regression sweep, and docs

**Goal:** The new loop is covered, the old assertions don't silently pass, and
the docs describe what shipped.

**Tasks:**

- [ ] **Required: `e2e/tests/workflows/phone-community-join-full-loop.spec.ts`** —
      the Playwright spec exercising the phone-first community-invite loop
      end-to-end. This is the acceptance test for the issue; the PR does not
      merge without it green. Model it on
      `phone-request-offer-full-loop.spec.ts` (the closest sibling — same
      Firebase Auth Emulator OTP setup, same guest-context pattern), driving the
      **real Flutter Web bundle**, not a mock:

      1. Host registers, creates a **named** community, mints a community invite
         via the existing `mintCommunityInviteLink` seed helper
         (`e2e/lib/seed/communities.ts`).
      2. Guest (fresh browser context, unauthenticated, no app) opens
         `/go/{code}` → asserts the **new** SSR community landing renders:
         community name, inviter line, member count, and the primary CTA. Assert
         the install-only affordances are *not* the primary path.
      3. Guest taps the CTA → lands on `/group/{communityId}?intent=join&code=…`
         → is routed to `/verify-phone` with `community_id` + `token` on the URL.
      4. Guest completes phone OTP against the **Auth Emulator** (deterministic
         code) and enters a name → `PhoneRegister`.
      5. Guest is returned to `/group/{communityId}`, the join settles, and the
         community view renders — located via the
         `Semantics(identifier: 'web-community-screen')` handle and `getByRole`,
         per `client/testing/semantics_identifiers.md` (never `getByLabel`).
      6. **Assert membership over RPC**, not pixels: the guest appears in the
         community's member list and their auth method is `PHONE`. A screenshot
         assertion alone would pass on a screen that rendered but never joined.

      Guard against the known traps: run `npm run build:e2e` first (stale
      embedded bundle), don't wait on `networkidle` (a long-lived stream never
      settles — #2867), and give the join a settle window rather than asserting
      on the first frame.
- [ ] Cover the **secondary registration path** too — the guest who takes the
      below-the-fold Google or email option on `/verify-phone` instead of the
      OTP. Either extend the new spec or add a community sibling to
      `phone-guest-email-register.spec.ts`. This is the `_goToLogin` /
      `_navigateToHome` threading from Phase 2, and it is the half that
      historically regresses.
- [ ] **Rewrite `invite-link-deep-link-paths.spec.ts` tests 1 and 2** — they
      assert the page this change deletes. Re-point them at the new landing's
      copy and CTA, and re-check every `toHaveCount(0)` in that file: a negative
      assertion keyed on removed copy passes forever and covers nothing. Note
      test 1 runs with `javaScriptEnabled: false` because today's page
      auto-navigates to `ripls://`; the new landing doesn't, so that workaround
      should come out rather than be inherited.
- [ ] Run the Go web-service suite and the Playwright workflow suite locally
      (`npm run build:e2e` first — a stale embedded bundle is the standing trap).
- [ ] Docs: update `docs/invitations.md` (§4 server-rendered page, §5 accepting,
      related-files table), `docs/client/deep_linking.md` (§3/§4 route lists),
      `docs/registration_and_login.md` (Phase 2 unauthenticated landing); add
      `docs/workflows/phone_first_community_join.md` as the fourth sibling to the
      three `phone_first_*` workflow docs, with its `context:` block.
- [ ] `npm run generate:context-map`, then `npm run lint` (context-map gate,
      l10n parity, a11y, Go/Dart size gates); advance `freshness:` stamps on
      every doc touched.

**Validates:** That the flow survives a real browser against a real server, and
that no test is quietly asserting nothing.

### Phase 4 (adjacent, tracked separately): community notification links

**Goal:** Not part of #2875 — recorded so Phase 2's design doesn't foreclose it.

**Tasks:**
- [ ] #2876: teach the "say hi" / member-joined notification link to carry a
      destination (e.g. `/group/{id}?tab=discuss`) and land an already-signed-in
      user in the community discussion instead of the join page. The
      `/group/{id}` route and the "authenticated visitor joins and stays" branch
      from Phase 2 are the prerequisites.

## Related Issues

- **#2876** — "say hi" shortlink lands on the same install-the-app page, even for
  signed-in users. Same page, filed one minute later by the same reporter. #2875
  replaces the page; #2876 needs the link to carry a deeper destination. Fix
  #2875 first; #2876 becomes a small param on the route it creates.
- **#2492** (epic, phone-first pivot) — built the three item landings this change
  copies. Canonical plan: `docs/issues/2492-phone-first-pivot.md`.
- **#2562** — removed legacy `?gear_id=`/`?request_id=`/`?experience_id=` query
  params; the target now lives on the `ShareLink` row. Constrains the CTA design:
  the code is bare, the target is read from the row.
- **#1099** — deferred deep linking (Play Install Referrer / iOS clipboard). The
  machinery living in `invite.html` today; Phase 1 preserves it on the secondary
  affordance.
- **#2630** — member reshare links. A guest who joins via the new flow should be
  able to reshare; worth an assertion once Phase 2 lands.

## Audit Notes

Audited with `ripls-audit` after drafting. Scope: server (`server/services/web/**`,
`server/l10n/source/**`) + client (`app/lib/presentation/screens/web/**`,
`app/lib/core/router/**`, `app/lib/presentation/screens/auth/**`) + e2e.
Dimensions engaged: security, accessibility, error_handling, observability,
performance, testing, api_design, code_quality, tech_debt.

Findings folded into the plan:

1. **Analytics regression risk** (observability) — `/invite` logs
   `CommunityJoinedEvent(joinMethod: 'invite_link')`; a second join path that
   doesn't would silently under-report joins as it takes traffic. → Phase 2 task.
2. **Second `postRegistrationDestination` call site** (testing/code_quality) —
   `PhoneAuthScreen` calls it from both `_navigateToHome` and `_goToLogin`.
   Threading `communityId` through only the first reproduces the exact bug the
   `*-email-register` specs exist to catch. → Phase 2 task + Phase 3 coverage.
3. **New multi-file directory needs a README** (CLAUDE.md) —
   `app/lib/presentation/screens/web/` goes from one file to two. → Phase 2 task.
4. **`server/services/web/README.md` routing table** is stale the moment the fork
   changes. → Phase 1 task.
5. **`no-inline-user-strings` + `l10n-parity`** are the two Go gates this change
   runs straight at; `es.toml` moves with `en.toml`. → stated in the plan preamble.
6. **Phase markers fail the build** (`check_ephemeral_comments.js`) — a real trap
   for a plan organized in phases that ships as one PR. → stated in the preamble.
7. **No new `CommunityEventType`**, therefore no undo/digest registry entries.
   Stated affirmatively so nobody adds one and trips
   `TestEveryEventTypeClassified`. → preamble.
8. **Pre-auth disclosure widens** to include the community description; `n=`/`img=`
   in the CTA href are safe via `html/template` contextual escaping, and the
   presigned `img` in the URL bar is accepted precedent from the item landings.
   → Phase 1 "Security decisions this phase makes".
9. **Fail-fast** — degraded fetches (member count, description) need the explicit
   "safe to ignore" comment, not a bare warn-and-continue. → Phase 1 task.
10. **A11y specifics** — `liveRegion` on join/retry surfaces, `Tappable`
    `semanticsLabel`, `AppColors` routing, ≥48dp, and reuse of the existing
    `webEventJoiningCommunity` / `a11yWebEvent*` ARB keys rather than
    near-duplicates. → Phase 2 task.
11. **`apple-app-site-association` must not gain `/group/*`** — doing so hands the
    flow back to the native app and re-opens this issue. → Phase 2 task.
12. **The `javaScriptEnabled: false` workaround** in the existing e2e exists only
    because today's page auto-navigates to `ripls://`; it should be removed, not
    inherited. → Phase 3 task.
13. **Delivery is one PR** (Bryan) — phases reframed as build order. → header +
    Implementation Plan preamble.
14. **Playwright acceptance test is required, not optional** (Bryan) — the
    phone-first community-invite loop gets a full-loop spec against the real
    Flutter Web bundle, with membership asserted over RPC. → promoted to the
    header and expanded as the first Phase 3 task.

Clean against: proto conventions (no proto change — `ShareLinkTargetKind.COMMUNITY`
and `CheckInvitation` already carry everything); SQL safety and N+1 (no new
queries beyond today's invite page, all in `server/storage/`); `connecterr.Internal`
(no new RPC handlers); file-size gates; caching conventions (client reads go
through existing repositories/providers).

Docs consulted: `docs/llms.txt`; anchors `server/architecture.md`,
`server/conventions.md`, `client/architecture.md`, `client/conventions.md`,
`proto_conventions.md`, `server/observability.md`, `testing/architecture.md`;
review prompts `security`, `accessibility`, `error_handling`, `observability`,
`performance`, `testing`, `api_design`; leaf docs `invitations.md`,
`registration_and_login.md`, `client/deep_linking.md`, `client/accessibility.md`,
`client/testing/semantics_identifiers.md`, `server/l10n.md`,
`ad_hoc_communities.md`, `community_health.md`,
`workflows/phone_first_rsvp.md`, `workflows/phone_first_gear_interest.md`;
plus `CLAUDE.md` and `server/services/web/README.md`.
