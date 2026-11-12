# Issue #2050: Web RSVP — yes/maybe/no + plan claim (1195 sub-issue 3)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2050
**Filed:** 2026-05-19 | **Last activity:** 2026-05-21
**Labels:** (none) | **Assignees:** unassigned | **Priority:** P3 | **Type:** Feature
**Context docs:** `docs/issues/1195-web-guest-rsvp.md` (parent plan), `docs/issues/2049-web-event-landing.md` (sibling, Phase 1 DONE)

> **Audit pass — 2026-05-21**
>
> Audited against `docs/client/architecture.md`, `docs/client/caching.md`,
> `docs/client/i18n.md`, `docs/client/accessibility.md`,
> `docs/client/testing.md`, `docs/server/architecture.md`,
> `docs/server/observability.md`. Compliance A–L from the parent plan
> apply (with deviations recorded under §Deviations from Parent Plan).
> Audit findings folded in: prefer `AsyncNotifier` for the new OTP /
> auth-handoff ViewModels (Compliance A — heavy async work), explicit
> `SafeNotifierMixin` + `ref.mounted` requirements if staying on
> `Notifier`; `UserError` + `RpcErrorHandler.classify/localize` for the
> registration → RSVP retry banner (Compliance B); `showAccessibleModal`
> for the "use email instead" sheet and `LiveRegion` /
> `SemanticAnnouncer.announce` for OTP error / success announcements
> (Compliance C); `a11y*` ARB keys for all `semanticsLabel:` and
> `tooltip:` strings (Compliance C); disposal tests for every new async
> Notifier (Compliance I); `flutter test --reporter expanded` for all
> new test files (Compliance I); `Cache-Control: no-store` + `Vary:
> Cookie` on the `/go/{code}` redirect response (Compliance L — CDN /
> shared-cache safety); explicit `connecterr.Internal(ctx, op, err,
> kv...)` for `RecordWebDecline` and the redirect handler's internal
> failures (Compliance E); standard log-field names (`share_link_id`,
> `experience_id`, `community_id`, masked `short_code`; never
> `request_id` for entity IDs — Compliance D); empty
> `scripts/a11y_allowlist.txt` invariant preserved.
>
> **Net new endpoints:** one HTTP `RecordWebDecline`. No new Connect
> RPCs. (`AcceptEventInvite` dropped per D1; `CanRSVP` covered by SSR
> render-time check + existing RPCs per D3 / parent Decision 6;
> `EstablishWebSession` dropped per D3.) This is a deliberately small
> server surface.

## Problem Statement

Phase 1 of the #1195 epic (sub-issue 2 / #2049) shipped a server-rendered
event landing at `/go/{code}` for event-flavored share links. The CTAs on
that landing are currently disabled placeholders (`server/services/web/templates/event.html:91-101`).
This sub-issue makes the headline user story work: someone with the link can
press **I'm in / Maybe / I'm out**, become a community member if they
weren't one already, RSVP, and then claim a slot from the event's Plan
list — entirely from the web, without installing the app.

In the same revision (2026-05-21) the parent epic absorbed the Flutter Web
build pipeline tasks that originally lived in #2049 into this sub-issue, on
the reasoning that a bundle-size budget grounded in real screens is more
honest than one measured against scaffolding. So #2050 is now two strands:

- **Flutter Web foundation** — `flutter build web` pipeline embedded into
  the Go server, `/app/*` route, bundle-size CI gate, `Platform.is*` audit,
  deferred-route infrastructure.
- **Action surface** — SSR "No" path, OTP-driven phone/email registration
  reused from the existing native flow, RSVP write, Plan-claim screen,
  Flutter ViewModels and ARB keys for everything new.

The parent plan's **Architecture Compliance Notes A–L** apply throughout;
this plan references them by letter rather than restating them, with two
exceptions documented in §Deviations from Parent Plan below.

## Context from Comments

One comment (2026-05-20): triage set Priority **P3** and Type **Feature**
with rationale "lower-priority polish / sub-issue / engineering tail." The
P3 label is misleading in isolation — this sub-issue is on the critical
path to the **P1** parent (#1195) and is the headline user-visible
deliverable. Sub-issues 4 (#2051, polish) and 5 (#2052, hardening) build
on what lands here.

No clarifying discussion in the issue itself. The issue body is the
authoritative scope statement; the parent plan supplies architecture and
compliance context.

The issue body was revised on 2026-05-21 to fold in the Flutter Web build
tasks moved from #2049 — those tasks are now part of this sub-issue's
"Flutter Web foundation" track. The CI bundle gate's stated cap (≤1 MB)
is no longer achievable on Flutter 3.41 (which removed the HTML renderer);
the parent plan's Compliance K and #2049's reconnaissance updated the
budget to **measure baseline + ~10% headroom**.

## Current State of the Codebase

Spot-checked before drafting; key findings:

- **Phase 1 SSR landing is production-ready.** `server/services/web/event_page.go`
  (576 lines) batch-fetches event + community + RSVPs + media for the
  landing render. `templates/event.html:83-101` renders three disabled
  buttons in the happy path and a "community is full" message bar in the
  at-capacity path. No `aria-live` region exists today — needed for the
  "No" path's in-place confirmation.
- **`requireAcceptableViaCommunityJoin`** in `server/services/community/share_links.go:314-327`
  is the single chokepoint that currently bounces event-flavored share
  links from `AcceptInvitationLink`. Its referenced replacement RPC
  (`AcceptEventInvite`) does **not exist** — the comment at line 312–317
  is a forward-looking placeholder that this sub-issue replaces.
- **Registration already accepts event-flavored share codes.**
  `server/services/login/registration.go:387-388` and the shared
  `validateShortCode` helper switch on `link.GetExperienceId() != ""` and
  return `SHARE_LINK_TARGET_KIND_EVENT`. The `completeRegistration` helper
  at `registration.go:486-548` already inserts the `User`, calls
  `acceptInviteLink` to add the `CommunityUser`, and issues tokens —
  **the only missing piece for events is the `Rsvp` write**, which the
  client fires separately after registration.
- **Transactions are available** via `server/storage/tx.go:35` (`ProtoSQLStorage.WithTx`).
  Not used by `completeRegistration` today; not needed for this sub-issue's
  client-side two-step (see §Design Decisions below).
- **RSVP RPC exists.** `server/services/experience/rsvp.go:22` (`RSVPToExperience`)
  is the canonical write path; the Flutter Web client calls it with a
  bearer token after registration. The mobile app has been calling this
  RPC for months — same wire contract on web.
- **`ClaimExperienceNeed`** exists at `server/services/experience/needs_and_contributions.go`
  (referenced from tests at `needs_and_contributions_test.go:142, 224, 497, 643`).
  The wire shape is what the Flutter Web Plan-claim screen calls.
- **No `EstablishWebSession`, no cookie middleware, no `RecordWebDecline`,
  no decline counter on `Experience`, no Firebase App Check integration.**
  All greenfield.
- **`EventShareLinkLandingScreen`** at `app/lib/presentation/screens/share_link/event_share_link_landing_screen.dart`
  exists as a minimal placeholder with a `TODO(#1195-sub3)` (line 23–27)
  pointing directly at this work. It will gain the post-RSVP polish in
  Phase 2.
- **`Platform.is*` / `dart:io` audit** — actual count is ~18 files, not
  the 7 the parent plan estimated. None are reachable from a guest-only
  entry point today; the audit task is to add `kIsWeb` guards (or
  conditional imports) on the subset that becomes reachable once the
  guest bundle's initial route graph is wired.
- **`build:web` script, `check_web_bundle_size.sh`, `test_flutter_web.yaml`,
  reCAPTCHA / App Check integration** — all absent. Greenfield.

## Design Decisions

These were resolved interactively before this plan was written.

### D1. Two-step client-side: register-then-RSVP, no combined RPC

The parent plan's §"Sub-issue 3" tasks describe a single transactional
`AcceptEventInvite(short_code, rsvp_state)` RPC that combines registration,
community-join, and RSVP. We're not building that.

**Approach instead:** the Flutter Web client fires the existing RPCs
back-to-back:

1. Phone path: `PhoneRegister(firebase_id_token, name, short_code)` (or
   `PhoneLogin` for a returning user — the client picks based on whether
   the verified phone is already on a `User`).
2. Email path: existing `EmailRegister` / `EmailLogin` flow with the
   short_code carrying the event context. Reuses Ripls' in-house email
   OTP via Mailgun — no Firebase Auth email link.
3. On token receipt, the client immediately fires
   `RSVPToExperience(experience_id, intention)` using the new JWT.

**Why:**
- Reuses every tested code path. `completeRegistration` already inserts
  `User` + `CommunityUser` from an event-flavored short_code (see
  `registration.go:387-388, 486-548`). The only missing step is the RSVP
  write, which is one existing RPC.
- No refactor of `completeRegistration` to take a `*sql.Tx` parameter.
  No new server RPC to test, document, version.
- Mirrors what the **mobile app already does** for a user who registers
  via an event share link today (registration writes `User` + joins
  community; experience-screen tap writes the RSVP separately). Web
  behavior matches mobile by construction.

**Failure mode the parent plan flagged:**
If `RSVPToExperience` fails after registration succeeds, the user is
**registered + joined the community** but without an `Rsvp` row. Mitigation:

- The Flutter Web client retries `RSVPToExperience` with exponential
  backoff (3 attempts) before surfacing the error. Network blips don't
  leave the user half-onboarded.
- If all retries fail, the UI lands on the post-RSVP screen with a
  "RSVP not recorded — tap to try again" banner. The user is in the
  community (which they implicitly opted into by tapping Yes/Maybe), and
  one more tap completes the intent.
- Server side, the `RSVPToExperience` path is **idempotent on
  (user_id, experience_id)** today (`server/services/experience/rsvp.go`),
  so retries from a deferred state work cleanly.

**Net:** the worst-case partial state ("joined community but no RSVP") is
recoverable from the same UI tap and produces no broken row in the
database. The parent plan's atomicity argument was strong but solved a
smaller real problem than the implementation cost.

### D2. Existing returning-user community-join routes through AcceptInvitationLink

The chokepoint at `share_links.go:314-318` that rejects event share links
gets removed. Returning users (existing `User`, not yet a `CommunityUser`
of the hosting community) call `AcceptInvitationLink(short_code)` to
join the community. The existing wire contract is unchanged; the only
change is that the function no longer returns `CodeInvalidArgument` for
event-flavored share links.

The follow-up reference comment in `invitations.go:173` ("share links flow
through AcceptEventInvite per #1195 sub-issue 3") gets revised to "event
share links join the community here; RSVP is written separately via
`RSVPToExperience`."

### D3. Breadcrumb cookie replaces session-cookie infrastructure

The parent plan's Compliance L specified an HTTP-only signed JWT
cookie holding a session token, verified by a new Connect interceptor,
issued by a new `EstablishWebSession` RPC. **None of that is built.**

**Approach instead:** a tiny content-free **breadcrumb cookie**.

- On successful auth in Flutter Web (token returned from `PhoneRegister`
  / `EmailRegister` / `PhoneLogin` / `EmailLogin`), the client sets
  `ripls_visited=1; Max-Age=31536000; SameSite=Lax; Secure; Path=/`
  via `document.cookie`. No JWT in the cookie. No signing. No server
  knowledge of the cookie's value.
- The `/go/{code}` handler in `server/services/web/service.go` inspects
  for the cookie's presence. If present and the share-link's target is
  an event, **302 to `/app/event/{experience_id}?code={short_code}`**
  before rendering SSR HTML. The Flutter Web app then loads, reads its
  JWT from localStorage, and renders the (full, personalized) event
  view using the same widgets the mobile app uses.
- Failure modes:
  - **Breadcrumb stale (JWT cleared, user logged out, session
    expired).** Flutter app falls back to its existing anonymous-state
    handling — typically the auth wall — and a "back to event landing"
    link bounces them to `/go/{code}` with the cookie still present.
    Easy escape: clear the cookie if Flutter Web detects no JWT on a
    breadcrumb-redirected load. (One-time client-side cleanup.)
  - **Visitor on a new browser that's never seen Flutter Web.** No
    cookie → SSR landing renders as normal → tap CTA → standard flow.
- The Flutter Web app itself has no cookie dependency. It continues
  to send `Authorization: Bearer <jwt>` for all RPCs.

**Why not the JWT cookie:**
- No server-side state, no Connect-interceptor changes, no cookie
  rotation, no signing-key infra.
- Compliance L's stated goal ("SSR knows the visitor is authenticated
  without parsing Connect's Authorization header") is satisfied at zero
  server-implementation cost. The Flutter app renders the personalized
  state; SSR's only job is to get out of the way.
- Privacy footprint is strictly smaller: a presence bit, not an
  identity token. No CSRF surface (we never act on the cookie's
  contents).

**Where this lives:** the breadcrumb work — Flutter side set, server
side redirect — lands here in **Phase 2** of #2050, not in #2051.
With no cookie infrastructure to build, sub-issue 4's scope shrinks
to host views + at-capacity polish.

This replaces (not defers) the parent plan's Compliance L. Captured
below in §Deviations from Parent Plan.

### D4. Bot protection via Firebase App Check, not reCAPTCHA Enterprise

The parent plan's Compliance J specified **reCAPTCHA Enterprise** for the
phone-OTP path on web. Switching to **Firebase App Check** with the
reCAPTCHA v3 provider for web instead.

**Why App Check:**
- Same Google-recommended posture as Enterprise; lower configuration
  ceiling (no separate Enterprise SKU procurement, simpler site-key
  rotation).
- Token-attached-to-request model means we can also enforce on our own
  Connect endpoints (not just Firebase Auth), giving the same protection
  to `RSVPToExperience` and `ClaimExperienceNeed` once we land enforcement
  in a follow-up.
- Sets up the path to **enable App Check on native iOS + Android too**
  via App Attest / Play Integrity, which is the more material
  billing-fraud mitigation for the existing native phone-OTP flow. That's
  a separate issue (see §Related Issues), but #2050's web setup is a
  forcing function.

**Rollout posture:** monitor mode for ~1 week post-launch, enforce after
false-positive data is clean. Document in `app/web/README.md`.

### D5. Reuse the mobile event-detail screen via `ExperienceContentView`

The parent plan's §"Sub-issue 3" tasks list "Flutter ViewModels for:
event view (read), OTP entry, RSVP confirm, Plan claim" — implying
new screens dedicated to web. We're not building a new event screen.

**Approach instead:** compose the existing
`ExperienceContentView` widget from both mobile and web surfaces.

```
app/lib/presentation/screens/experience/
  experience_screen.dart           # mobile entry — SwipeToCloseMixin, back-button overlay
  experience_content_view.dart     # shared body: hero, RSVP toggle,
                                   # attendees, Plan list, chat preview
                                   # (ALREADY EXISTS — extraction shipped earlier)
  web_experience_screen.dart (NEW) # web entry — thin <header>, no
                                   # BottomNav, no SwipeToClose
```

**The extraction is already done.** `experience_content_view.dart`
shipped as part of the mobile codebase before #2050 was scoped;
`ExperienceScreen` is already a thin 77-line wrapper that adds
`SwipeToCloseMixin`, a `Scaffold`, and the back-button overlay. The
shared widget already exposes the parameters web will need
(`initialCommunityId`, `initialTab`, `showFloatingActions`,
`showEditControls`, `showOwnerInfo`) so a thin web shell can compose
it without modification.

- `ExperienceContentView` is the source of truth for the event-view
  UI. It owns the RSVP toggle (`experience_rsvp_button.dart`,
  `rsvp_toggle_buttons.dart`), the Plan list, the attendee carousel,
  the time/place metadata block, and the chat preview.
- `ExperienceScreen` (mobile, existing) wraps `ExperienceContentView`
  in `SwipeToCloseMixin` + a `Scaffold` with a back-button overlay.
- `WebExperienceScreen` (new — Step 7) wraps `ExperienceContentView`
  in a thin web scaffold: lightweight top bar, no `SwipeToClose`,
  `showFloatingActions: false` so mobile-only FABs stay out of the
  guest path. Same widget tree; different chrome.

**Why this matters beyond #2050:**
- Long-term direction is "make the whole app usable on web." Every
  shared widget extracted now is one less screen to rewrite when the
  next surface goes web-capable (profile, gear detail, request detail).
  This sub-issue sets the pattern.
- Day-one feature parity: the post-RSVP Flutter Web user sees the
  same Plan list, attendee photos, RSVP toggling, and chat preview a
  mobile user sees — because they're the same widgets.
- The `Platform.is*` audit (originally framed as "guard 18 files
  against the web bundle") gets a sharper scope: make
  `ExperienceContent`'s transitive widget tree `kIsWeb`-clean. The 18
  files only matter where they're imported from that tree. Real
  surface is smaller than the parent plan suggested.

**Cost:** one refactor of `ExperienceScreen` in Phase 1 — extract the
content body into `ExperienceContent`, route the existing mobile
imports through it. The existing widget tree mostly lifts cleanly
because `experience_rsvp_button.dart` and friends are already
self-contained.

**Mobile-app `EventShareLinkLandingScreen` placeholder** (the one with
`TODO(#1195-sub3)` at line 23–27) gets deleted in this phase:
installed-app deep-links to `/go/{code}` for event-flavored share
links route directly to `ExperienceScreen`, which is now backed by
`ExperienceContent`. One canonical event-detail screen across all
three surfaces (mobile, web, deep-link landing).

### D6. Two phases — Foundation+Actions, then Plan-claim & polish

Phase 1 ships everything except the Plan-claim screen and the more
elaborate at-capacity / returning-member UX. Phase 1 is the headline
deliverable. Phase 2 finishes the Plan-claim flow, integration tests,
and the post-RSVP polish. Two phases, two PRs, each shippable on its
own.

The Flutter Web foundation lives in Phase 1 — it has to, because Phase 1's
own actions consume it. The foundation lands as a near-empty placeholder
`index.html` for the first commit, then accretes the OTP and RSVP-confirm
screens as the same PR moves through review. Bundle gate's baseline is
measured at the end of Phase 1 (when the initial route graph is set) and
becomes the budget Phase 2 inherits.

### D7. Open-in-app hint on the SSR event landing

Universal Links (iOS, AASA-verified for `/go/*`) and Android App
Links (assetlinks.json) already handle the dominant case
automatically: when a `/go/{code}` link is tapped from Messages /
Slack / Mail / any non-browser context with the app installed, the
OS opens the app and this SSR page is never rendered.

What remains is the residual: in-app webviews (Slack, Twitter,
Instagram) that don't honor Universal Links, copy-paste, typed
URLs. From inside a browser there is no reliable detection of
"is the app installed?" — both iOS and Android removed that
capability for privacy. The next-best UX is a visible "Open in
Ripls app" affordance the user can tap if they have the app.

- **Android.** An [Intent URI][intent-uri]
  (`intent://{host}/go/{code}#Intent;scheme=https;package={pkg};S.browser_fallback_url=…;end`)
  bypasses the in-app webview by handing the URL to Chrome's URI
  dispatcher. Android opens the verified app if installed; the
  embedded `browser_fallback_url` sends the rest to the Play Store
  listing. The `{pkg}` is derived per-environment by
  `androidPackageForHostname()` in `event_page.go` so the dev
  build (`org.ripls.app.dev`) and prod build (`org.ripls.app`) are
  each targeted by the right SSR origin.

- **iOS.** A custom-scheme `ripls://invite?token={code}&experience_id={id}`
  link opens the app if installed (silent no-op if not). The
  existing `ripls://invite` handler in `app_router.dart` accepts
  `experience_id` and routes through AcceptInvitation, which after
  Step 1 of this issue accepts event-flavored short codes without
  the legacy event-link bounce. Apple's Smart App Banner is
  intentionally **not** wired — it requires a numeric App Store ID
  and the app currently ships via TestFlight only. The banner is a
  small follow-up once the public listing exists.

The hint hides on desktop and unknown UAs. The primary user
action remains the bottom RSVP CTA bar; the open-in-app chip is a
low-attention escape hatch above the event meta-pill.

[intent-uri]: https://developer.chrome.com/docs/multidevice/android/intents

## Deviations from Parent Plan

The parent plan's Compliance A–L are inherited as written, with two
explicit deviations:

- **Compliance J (reCAPTCHA Enterprise → Firebase App Check).** See D4
  above. The compliance intent (gate the phone OTP path against bot
  abuse) is preserved; the implementation switches to App Check.
- **Compliance L (session-cookie semantics).** Replaced — not deferred —
  by the breadcrumb cookie per D3. #2050 ships without
  `EstablishWebSession`, without a signed-JWT cookie, and without a
  cookie-aware Connect interceptor. The Connect middleware continues to
  recognize `Authorization: Bearer` only. SSR personalization happens
  by **redirecting authenticated visitors into the Flutter Web app**,
  which renders state itself using the same widgets the mobile app
  uses. Sub-issue 4 (#2051) inherits no cookie work; its scope is
  host views + at-capacity polish.

A third soft deviation: the parent plan's `AcceptEventInvite` RPC is
not built (D1). The forward-looking comments in `share_links.go:312-318`
and `invitations.go:173` get rewritten to reflect the two-step flow.
This is a course correction, not a compliance deviation.

A fourth soft deviation: the parent plan's §"Sub-issue 3" task list
calls for "Flutter ViewModels for: event view (read), OTP entry,
RSVP confirm, Plan claim" — implying new screens dedicated to web.
Per D5, the event-view ViewModel is the existing
`ExperienceScreen`'s state notifier (now backing `ExperienceContent`,
which both mobile and web compose). Only the OTP entry and the
auth-handoff state are net-new; RSVP and Plan-claim are existing
ViewModels in the mobile codebase.

## Open Questions

- **Bundle-budget concrete number?** Recommendation: **measure on the
  first end-of-Phase-1 build, then set the CI gate at measured +
  ~10%.** Don't try to set a number in this plan. The parent plan's
  Compliance K already revised away from the 1 MB cap; the right number
  comes from a real build. The CI gate's failure message should print
  the measured baseline and the budget so future PRs see what they're
  fighting against.

- **What does the SSR "No" path actually look like in the DOM?**
  Recommendation: the three CTA buttons live in a sticky footer per the
  approved Mock H. Tapping "I'm out" replaces the footer's contents with
  a small confirmation message ("Thanks — we'll let the host know") in
  an `aria-live="polite"` region. No page reload. A small inline JS
  handler does the `fetch` to `RecordWebDecline`. Failure mode: the
  fetch fails → the message still shows but with a quieter "couldn't
  reach the server — your reply may not have been recorded" line. The
  decline counter is best-effort by design; we don't want a network
  blip to make declining feel broken.

- **`RecordWebDecline` storage shape — counter on `Experience` or
  separate table?** Recommendation: add an `optional int32 web_decline_count`
  field to the storage `Experience` proto. Single-table, fewer indexes,
  matches the rest of the per-event scalars (attendee count, etc.) and
  the host UI just reads it as part of the existing experience fetch.
  Rate-limit the increment per Compliance D logging conventions; spam
  hardening lives in sub-issue 5.

- **Where does the Flutter Web `index.html` live in source — `app/web/`
  or `server/services/web/app_assets/`?** Recommendation: keep
  `app/web/index.html` as the **source** (Flutter convention), let
  `flutter build web` emit to `app/build/web/`, and have the
  `build:web` npm script **copy** the output into
  `server/services/web/app_assets/` before Go builds. This mirrors how
  `website/embed.go` works today: source files live next to the Go
  embed directive. Don't `go:embed` `app/build/web/` directly — the
  embed path then crosses a build-pipeline boundary that's hard to
  reason about in CI.

- **Deferred route boundaries — which screens are in the initial
  bundle?** Recommendation per the issue body: only `event view`
  (the post-RSVP confirm screen), `Firebase Auth UI` (phone/email OTP
  entry), `RSVP confirm`, and `Plan claim`. Everything else
  (mobile-app screens that happen to compile for web) is behind
  `deferred as` imports and never loads on the guest path. The
  bundle gate measures the initial chunk only.

- **OTP screens — build new or compile the existing mobile widgets?**
  Recommendation: build **new, web-only** OTP-entry widgets in
  `app/lib/presentation/widgets/web_auth/`. The mobile OTP UX has
  platform-specific touches (native keyboard handling, autofill from
  SMS, iOS keyboard avoidance) that we don't need on web and that
  would compile-block on `kIsWeb`. New web-specific widgets are
  smaller, simpler, and don't risk regressing the native auth
  surface. Wrap them in shared `Tappable` / `IconAction` per
  Compliance C. This is the **only** screen we build new — the
  post-OTP event view, RSVP, and Plan-claim all reuse
  `ExperienceContent` per D5.

- **Plan-claim UX — separate widget or part of `ExperienceContent`?**
  Recommendation: the Plan list is already a child of
  `ExperienceContent` on mobile (the existing experience screen
  shows Plan needs inline). Web inherits this for free. Post-RSVP,
  the same scaffold reveals it. No new screen, no animation surgery
  — the existing widget renders the same way it does on mobile,
  and `WebExperienceScreen`'s thin chrome doesn't get in the way.

- **Breadcrumb cookie set timing.** Recommendation: set
  `ripls_visited=1` **immediately on the first successful token
  return** in Flutter Web (`PhoneRegister`, `EmailRegister`,
  `PhoneLogin`, or `EmailLogin`). Don't wait for the user to do
  anything meaningful; the breadcrumb just says "this browser has
  reached an authenticated Ripls Web session at least once" —
  intent doesn't matter, presence does.

## Implementation Plan

### Phase 1 (PR 1): Flutter Web foundation + RSVP actions

**Goal:** A guest with an event share link can tap I'm in / Maybe and
finish a phone or email OTP flow on Flutter Web, ending up as a
registered `User` + `CommunityUser` + `Rsvp`. Tapping I'm out shows a
confirmation and increments an anonymous server-side decline counter.
Plan-claim is **not** in this phase.

**Tasks — Flutter Web foundation:**

- [ ] Add `build:web` npm script that runs `flutter build web --release`
  in `app/` and copies the output into `server/services/web/app_assets/`.
  Pinned Flutter version: whatever the repo currently uses
  (3.41.6 per the #2049 reconnaissance). No `--web-renderer` flag
  (removed in 3.27+).
- [ ] New `server/services/web/app_embed.go` with `//go:embed app_assets/*`
  mirroring `website/embed.go`.
- [ ] New `/app/*` route in `server/main.go` adjacent to the existing
  `/go/`, `/css/`, `/assets/`, `/.well-known/` routes. Cache headers
  per Compliance K: `Cache-Control: public, max-age=31536000, immutable`
  for hashed assets; `Cache-Control: no-store` for `index.html` and
  Flutter's runtime manifest.
- [ ] `app/web/README.md` documenting the pinned Flutter version, the
  renderer choice (CanvasKit by default; no flag), the bundle gate, the
  Firebase App Check site key (or pointer to where it lives in GCP),
  and the `build:web → embed → /app/*` pipeline.
- [ ] `scripts/check_web_bundle_size.sh` that runs `flutter build web --release`,
  gzips the initial-chunk JS, and fails if size > baseline × 1.10.
  Baseline lives in the script (commit the first measured value); the
  failure message prints baseline + measured + delta. Per #2049
  reconnaissance, no absolute ≤1 MB cap.
- [ ] New `.github/workflows/test_flutter_web.yaml` triggered on
  changes to `app/lib/**`, `app/web/**`, `pubspec.yaml`, the script
  itself. Runs the bundle gate.
- [ ] Audit `Platform.is*` / `dart:io` usage in the transitive widget
  tree rooted at `ExperienceContent` (post D5 extraction) plus the
  new `web_auth/` OTP widgets. Add `kIsWeb` guards where needed; move
  cleanly-separable platform-specific imports behind `deferred as`.
  The broader ~18-file list outside that tree is **not** in Phase 1
  scope — those files don't reach the guest bundle's initial chunk
  via the route graph. Document the audit conclusion in
  `app/web/README.md`.
- [ ] Configure Flutter Web routing for **deferred routes**. Initial
  bundle: `WebExperienceScreen` (wrapping `ExperienceContent`),
  Firebase Auth phone OTP, email OTP, RSVP confirm helper. Everything
  else (full mobile app screens) is behind `deferred as` and never
  loads on the guest path.

**Tasks — server-side action plumbing:**

- [ ] Remove the event-flavored bounce in `share_links.go:314-318`
  (`requireAcceptableViaCommunityJoin`). Returning users on event
  share links now flow through `AcceptInvitationLink` exactly like
  community-invite share links. Update the forward-looking comments in
  both `share_links.go` and `invitations.go:173` to reflect the
  two-step (community-join here, RSVP separately) flow.
- [ ] No `AcceptEventInvite` RPC. Confirm in tests that a returning
  user accepting an event-flavored short_code via
  `AcceptInvitationLink` lands as a `CommunityUser` with no `Rsvp`
  yet; the client fires `RSVPToExperience` from the post-acceptance
  screen.
- [ ] `RecordWebDecline(short_code)` HTTP handler in
  `server/services/web/web_decline.go`. Lives in the web service per
  Compliance F (not a Connect RPC — a plain HTTP POST endpoint with
  the short_code in the path or body, returning 204 or a small JSON
  envelope). Logging per Compliance D:
  `logging.LoggerWithContext(ctx).With("operation",
  "RecordWebDecline", "short_code", logging.MaskToken(shortCode),
  "share_link_id", id, "experience_id", expID, "community_id",
  commID)`. Use entity-specific IDs — never `request_id` for entity
  IDs (reserved for HTTP correlation per the observability glossary).
  Internal failures wrap via `connecterr.Internal(ctx,
  "RecordWebDecline.<step>", err, "share_link_id", id, ...)` per
  Compliance E (bare `connect.NewError(connect.CodeInternal, err)`
  fails CI). Rate-limit hookup uses the existing
  `server/middleware/ratelimit/` middleware; sub-issue 5 (#2052)
  tightens the limits — for this PR, attach the middleware with a
  generous default so the path doesn't ship rate-limit-free.
  Log-level discipline: rate-limit refusals at `Warn` (handled
  user error); invalid/revoked short_code at `Info`; internal
  failures at `Error`.
- [ ] Add `optional int32 web_decline_count` to `proto/ripls/models/experience.proto`
  per Compliance H. Per the migration convention, this is wire-compatible
  with stored protos (no migration needed).
- [ ] Integration test in `server/services/web/web_decline_test.go`
  covering: increment success, idempotency-not-promised (each call
  increments — sub-issue 5's rate limit caps that), invalid short_code
  → 404, expired/revoked short_code → 410, rate-limit refusal → 429.
  Existing tests for community-invite share-link behavior must still
  pass (the `AcceptInvitationLink` change to drop the event-link
  bounce is a regression risk worth a guarded test).

**Tasks — Flutter screen refactor (D5):**

- [x] ~~Extract the content body of `experience_screen.dart` into a
  new `experience_content.dart`.~~ **Already shipped** as
  `ExperienceContentView` in `experience_content_view.dart`;
  `ExperienceScreen` is already a thin 77-line wrapper. No work
  needed here.
- [ ] Delete the mobile-app placeholder
  `app/lib/presentation/screens/share_link/event_share_link_landing_screen.dart`
  and its `_view_model.dart`. Installed-app deep-links to `/go/{code}`
  for event-flavored share links route directly to `ExperienceScreen`.
  Update the deep-link router accordingly. Resolves
  `TODO(#1195-sub3)`.
- [ ] New `WebExperienceScreen` in
  `app/lib/presentation/screens/experience/`. Thin web chrome (no
  `BottomNav`, no swipe gestures, lightweight `<header>`-equivalent
  with a small "back to landing" link). Composes `ExperienceContent`.
  Reads URL params `experience_id`, `rsvp`, `code`.

**Tasks — Flutter Web auth screens:**

- [ ] New `app/lib/presentation/widgets/web_auth/phone_otp_entry.dart`
  and `email_otp_entry.dart` per the §Open Questions decision. Wrap
  in `Tappable` / `IconAction` per Compliance C. All `semanticsLabel:`
  and `tooltip:` strings come from `context.l10n.a11y*` keys
  (per `require_l10n_for_semantic_labels` lint rule); no raw string
  literals. The empty `scripts/a11y_allowlist.txt` invariant must be
  preserved — any new violation gets fixed in the same PR.
- [ ] "Use email instead" affordance presented as a bottom sheet via
  `showAccessibleModal` per Compliance C — not `showModalBottomSheet`
  (gated by `prefer_accessible_modal` lint rule). Restores focus on
  dismiss.
- [ ] `WebAuthService` in `app/lib/services/` driving Firebase Auth
  phone OTP on web + the existing in-house email OTP. Firebase App
  Check initialized at app startup with the reCAPTCHA v3 provider
  per D4. ViewModels never call this service directly per
  Compliance A — wrap it in an `AuthRepository` (or extend the
  existing one if it covers mobile auth) so the ViewModel → Repo →
  Service layering is preserved.
- [ ] ViewModels: `OtpEntryViewModel` and a short-lived
  `AuthHandoffViewModel` (drives the post-OTP register → RSVP
  sequence and exposes retry state). **Prefer `AsyncNotifier` over
  plain `Notifier`** per Compliance A — both ViewModels do heavy
  async work (Firebase OTP verification, registration RPC,
  retry-bounded RSVP RPC) and benefit from `AsyncValue`'s typed
  loading/error/data states and built-in disposal handling in
  `build()`. If for any reason `Notifier` is chosen instead,
  `SafeNotifierMixin` is **mandatory** plus `ref.mounted` checks
  after every `await` before `state =`. Both use Freezed for
  immutable state.
- [ ] RSVP and event-view ViewModels are **existing** notifiers from
  the mobile codebase, now consumed by `ExperienceContent`. No new
  data-access ViewModels; no new repositories. Data flows through
  the existing `ShareLinkRepository` + `ExperienceRepository` per
  Compliance A. Image caching follows
  `ImageCacheKeys.thumbnail()` / `.full()` already used by mobile.
- [ ] ARB keys per Compliance B: prefixes `rsvpWeb*`, `guestOtp*`,
  `commonRsvp*`, plus `a11y*` for accessibility labels, added to
  both `app/lib/l10n/app_en.arb` and `app_es.arb` with `@key`
  metadata. CI gate: `npm run lint:dart:l10n-parity` and
  `npm run lint:dart:i18n` must pass. The `eventShare*` prefix
  already exists from the mobile placeholder; reuse where
  applicable, retire keys that go with the deleted placeholder
  screen.
- [ ] **Error handling shape per Compliance B:** ViewModels expose
  typed state via `UserError? error` on the Freezed state class;
  widgets resolve to localized text in `build()` via
  `RpcErrorHandler.localize(state.error!, context.l10n)`. Catch
  blocks in the ViewModel call `RpcErrorHandler.classify(e)`. See
  `app/lib/core/errors/README.md` for the typed variants. The
  "RSVP not recorded — tap to try again" banner copy lives in
  ARB as `rsvpWebRetryBanner` (or similar) and the localized
  string is selected from a typed error variant, not concatenated
  in the ViewModel.
- [ ] On registration success (`PhoneRegister` or `EmailRegister`
  returns tokens), client immediately fires
  `RSVPToExperience(experience_id, intention)` with retry (3
  attempts, exponential backoff). On retry exhaustion, render
  `WebExperienceScreen` (the user is now a `CommunityUser`) with a
  `LiveRegion`-wrapped retry banner per Compliance C — screen
  readers announce the failure on insertion. Banner copy localized
  per Compliance B.
- [ ] On returning-user path (Firebase phone or in-house email
  identifies an existing `User`): if already a `CommunityUser` of
  the hosting community, skip `AcceptInvitationLink` and go straight
  to `RSVPToExperience`; otherwise call `AcceptInvitationLink` first
  (now accepting event-flavored short_codes per the
  `requireAcceptableViaCommunityJoin` removal above), then
  `RSVPToExperience`.
- [ ] SSR template change: replace the disabled buttons at
  `templates/event.html:91-101` with:
  - "I'm out" → inline JS handler that POSTs to `RecordWebDecline`
    and replaces the footer with an `aria-live="polite"` "Thanks"
    region.
  - "Maybe" / "I'm in" → `<a href="/app/event/{experience_id}?rsvp=…&code=…">`.
    Navigation hands off to Flutter Web; no JS required to start the
    flow.
- [ ] Tests per Compliance I:
  - ViewModel tests for `OtpEntryViewModel` and
    `AuthHandoffViewModel` using `ProviderContainer` + Mockito-
    generated mocks via `@GenerateMocks([WebAuthService,
    ShareLinkRepository, ExperienceRepository])` (or whichever
    boundary types the new ViewModels depend on). Run
    `flutter pub run build_runner build` after adding annotations.
  - **Disposal tests** for every new async Notifier per the
    "Disposal Testing" section of `docs/client/architecture.md`.
    Start the async operation (OTP verify, register, RSVP), dispose
    the container before completion, assert
    `expectLater(future, completes)` without throwing. Mandatory
    for `autoDispose` Notifiers with async work.
  - HTTP handler test for `RecordWebDecline` (success, rate-limit
    refusal, invalid short_code, revoked short_code).
  - Golden-file test for the SSR landing's new active-button
    variant in `event_page_test.go` (mirror the #2049 golden-file
    convention).
  - Existing `ExperienceScreen` tests must still pass against the
    refactored `ExperienceContent` decomposition — extraction is a
    refactor, not a behavior change.
  - All Flutter tests use `--reporter expanded` per Compliance I /
    CLAUDE.md (npm script convention).

**Validates:** Headline guest-to-member journey works on web — RSVP
yes/maybe/no end-to-end. No Plan-claim yet.

### Phase 2 (PR 2): Breadcrumb cookie + Plan-claim validation + integration tests

**Goal:** Returning Flutter-Web visitors land in the app directly,
skipping the SSR page. Plan-claim works end-to-end (most widgets land
free via D5's `ExperienceContent` reuse — Phase 2 verifies and
polishes the web-specific flow). Integration tests cover the full
client-server happy path.

**Tasks — breadcrumb cookie (D3):**

- [ ] On successful auth in Flutter Web (`PhoneRegister`,
  `EmailRegister`, `PhoneLogin`, `EmailLogin` returning tokens), set
  `ripls_visited=1; Max-Age=31536000; SameSite=Lax; Secure; Path=/`
  via `document.cookie` (web-only — guard with `kIsWeb`). No JWT in
  the cookie. No content beyond presence.
- [ ] In `server/services/web/service.go`, on `/go/{code}` lookup, if
  the share link's target is an event **and** the request carries
  the `ripls_visited` cookie, return `302 Found` to
  `/app/event/{experience_id}?code={short_code}` instead of
  rendering SSR HTML. Order: cookie check happens **after** share
  link resolution (so revoked / expired codes still get the standard
  error page, not a redirect into a broken Flutter route).
- [ ] **Cache headers on the redirect response:** emit
  `Cache-Control: no-store` and `Vary: Cookie` so a shared cache
  (CDN, corporate proxy) does not pollute non-cookie-bearing
  requests with the redirect, and never caches the redirect on
  behalf of one visitor for another. The existing `/go/{code}` SSR
  path keeps its `Cache-Control: public, max-age=30` (from #2049);
  the divergence is intentional. (If we ever move `/go/{code}`
  behind a CDN, `Vary: Cookie` on both branches becomes a hard
  requirement; documenting now.)
- [ ] When Flutter Web loads from a breadcrumb-redirect and finds no
  JWT in localStorage (cookie stale), clear `ripls_visited` and
  bounce back to `/go/{code}` so the user gets the standard SSR
  landing path. One-time cleanup; no loop risk because the cleared
  cookie means the next `/go/{code}` hit doesn't redirect.
- [ ] Logging per Compliance D: log the redirect at `Info` with
  `operation` ("HandleGoCode.redirect"), masked `short_code`,
  entity IDs (`share_link_id`, `experience_id`, `community_id`),
  and a `redirected_via_breadcrumb=true` field. Never log the bare
  short_code (it's a long-lived bearer secret). Never use
  `request_id` for entity IDs — that field is reserved for the
  HTTP correlation ID auto-injected by `LoggerWithContext`. Sub-issue 5
  can extend this for abuse signals.
- [ ] Internal failures (share-link lookup error, etc.) wrap via
  `connecterr.Internal(ctx, "HandleGoCode.<step>", err, kv...)`
  per Compliance E. The HTTP handler returns a 500 with a generic
  public message; the original error text never reaches the wire.
- [ ] Tests: HTTP integration test for the `/go/{code}` redirect
  branch — cookie present + event link → 302 to `/app/event/…`
  with `Cache-Control: no-store` + `Vary: Cookie`; cookie present
  + community-invite link → SSR landing (no redirect); no cookie
  → SSR landing (no redirect, normal `max-age=30`); cookie present
  + revoked link → error page (no redirect).

**Tasks — Plan-claim validation & polish:**

- [ ] Verify the Plan list inside `ExperienceContent` renders
  correctly under `WebExperienceScreen`. Existing widget;
  cross-platform validation only. Adjust visual padding / hit
  targets for desktop browsers if needed (most mobile sizes are
  fine).
- [ ] Confirm one-tap claim via `ClaimExperienceNeed` works on web.
  Existing RPC (`server/services/experience/needs_and_contributions.go`)
  is the same call mobile makes; no server changes expected.
- [ ] Accessibility regression check on `ExperienceContent`'s Plan
  list: claim-state changes already announce via
  `SemanticAnnouncer.announce` / `LiveRegion` on mobile per
  Compliance C; verify the web shell doesn't break those
  announcements.

**Tasks — integration tests:**

Three test surfaces in increasing order of cost. Each surface
catches a class of regression the cheaper surfaces above it would
miss. The bundle-shape surface (1) shipped on the Phase-1 PR as a
follow-up; surfaces (2) and (3) are the Phase 2 deliverables.

#### Surface 1 — Bundle shape & MIME (server-only, no DB) ✅ SHIPPED

These tests live in `server/services/web/app_handler_test.go` and
catch the class of bug that produced the blank-page report on PR
#2122: build flags drift, asset MIME types drift, the embed surface
loses its index. They're pure HTTP-level asserts against
`webService.ServeApp()`, no DB, no Connect, no widget tree.

- [x] `TestServeApp_BundleHasBaseHref` — skips if no real bundle is
  built. Otherwise GETs `/app/` and asserts the served HTML contains
  `<base href="/app/">`. Catches: `--base-href=/app/` getting dropped
  from `build:web`, an index.html that points at the wrong base, or
  a path-strip mismatch.
- [x] `TestServeApp_BundleAssetsServeWithJSMime` — skips if no real
  bundle is built. Otherwise GETs `/app/flutter_bootstrap.js` and
  `/app/main.dart.js`, asserts 200 + Content-Type containing
  `javascript`. Catches: text/plain 404 fallthroughs that
  `X-Content-Type-Options: nosniff` refuses to execute as JS — the
  exact MIME-mismatch symptom the PR-review bug surfaced.
- [x] `TestServeApp_PlaceholderAtRoot` — inverse: skips if a bundle
  IS built. Verifies the placeholder shell still renders on a fresh
  checkout (no `build:web` yet).

CI gating: Surface 1 tests skip cleanly when `npm run build:web`
hasn't run, so `go test ./...` is non-blocking by default. To make
them load-bearing in CI, the `test_flutter_web.yaml` workflow needs
to (a) build the bundle, (b) re-build the server binary so the new
`//go:embed` picks it up, then (c) run the web-service Go tests.
Cheapest add as a follow-up.

#### Surface 2 — End-to-end happy path (server + DB, no browser)

These tests live in `server/main_integration_test.go` and exercise
the full client-server contract for the web RSVP flow against a
real Postgres testcontainer + a real Connect server. They mock the
**client side** by issuing the same RPC sequence the Flutter Web
app would, in the same order, with the same arguments.

- [ ] **Full guest journey, email path**: SSR `GET /go/{code}` →
  parse the returned event landing for the Maybe href → ensure it
  matches the expected `/app/event/{id}?rsvp=maybe&code=…` shape →
  fire `EmailRegister(email, name, password, shortCode)` → assert
  `User` + `CommunityUser` rows exist → fire
  `RSVPToExperience(experience_id, community_id, MAYBE)` with the
  returned JWT → assert `ExperienceRSVP` row exists with intention
  MAYBE. Uses `storage.SetupTestStorage(t)` with the
  `pgvector/pgvector:pg16` testcontainer per CLAUDE.md.
- [ ] **Full guest journey, phone path**: Same as above with
  `PhoneRegister(firebaseIdToken, name, shortCode)`. Mock the
  Firebase ID token verification via the existing
  `auth.PhoneAuthClient` test fake (already used in
  `server/services/login/phone_auth_test.go`).
- [ ] **Returning-member fast path**: pre-seed an existing `User`
  that's a `CommunityUser` of the hosting community. Fire
  `RSVPToExperience` directly with their JWT — no register, no
  AcceptInvitationLink. Assert single `ExperienceRSVP` row, no
  duplicate community-join.
- [ ] **Returning-non-member path**: existing `User`, NOT a member
  of the hosting community. Fire `AcceptInvitationLink(shortCode)`
  → assert `CommunityUser` row created → fire `RSVPToExperience` →
  assert `ExperienceRSVP` row created. Verifies the
  `requireAcceptableViaCommunityJoin` fix lets returning users
  through.
- [ ] **Partial-failure resilience**: register succeeds, then
  `RSVPToExperience` fails (e.g., experience cancelled between
  steps). Assert `User` + `CommunityUser` exist but no `Rsvp`.
  Client-side retry exercises the same path; server-side test
  confirms the partial state is recoverable.
- [ ] **`RSVPToExperience` idempotency**: fire the RPC twice with
  the same `(user_id, experience_id, community_id, intention)`.
  Assert exactly one `Rsvp` row exists, second call doesn't error,
  and `rsvped_at_unix_sec` is unchanged (or only `last_updated_unix_sec`
  changed). D1's client-side retry strategy depends on this; if the
  assumption turns out to be wrong the retry shape needs to fold in
  delete-and-recreate semantics. Existing
  `server/services/experience/rsvp_test.go` may already cover this
  — extend if it doesn't.
- [ ] **`RecordWebDecline` end-to-end**: pre-seed event-flavored
  ShareLink → POST `/go/{code}/decline` → assert 204 +
  `Experience.web_decline_count = 1` → POST again → assert
  `web_decline_count = 2` → POST 6 more times rapidly from the
  same IP → assert at least one 429 (the new B2 rate limit kicks
  in after the burst of 5).
- [ ] **Breadcrumb cookie redirect**: GET `/go/{code}` with
  `Cookie: ripls_visited=1` for an event share link → assert 302
  to `/app/event/{id}?code={code}` + `Cache-Control: no-store` +
  `Vary: Cookie`. Same request without the cookie → assert 200 +
  SSR HTML. Same request with the cookie but for a
  community-invite share link → assert 200 + SSR HTML (no
  redirect — cookie's redirect target is event-only).
- [ ] **Storage-query budget** for the breadcrumb redirect path per
  Compliance G: `/go/{code}` with cookie present should make ≤2
  storage queries (share-link lookup + optional experience-exists
  validation). Add `storage.AssertMaxQueries(t, ctx, 2, fn)` to
  the redirect integration test.

These tests run as part of the standard `go test ./...` once
testcontainer-Postgres is available. No browser, no Flutter Web
runtime — just the same Connect RPCs the Flutter Web client would
fire.

#### Surface 3 — Browser-driven end-to-end (optional, future)

These tests would actually load the Flutter Web bundle in a
headless browser (Playwright / Puppeteer / Chrome DevTools
Protocol), navigate through the UI, and assert on screen state.
They catch the class of regression Surfaces 1+2 can't see:

- Widget tree breakage that compiles + serves cleanly but renders
  wrong (e.g., RSVP toggle invisible, retry banner mispositioned)
- JS-runtime issues in compiled Dart (rare, but possible after
  Flutter SDK upgrades)
- Auth-handoff race conditions visible only in real browser timing
- Real DOM accessibility surface — VoiceOver / TalkBack /
  axe-core assertions

**Not in scope for Phase 2.** Client-side end-to-end testing was
attempted via Patrol and removed (#1887); Surface 3 stays deferred
until there's a viable iOS-and-Android-and-Web automation story.
The mobile app's E2E gap is already documented as out-of-scope in
`docs/client/testing.md` §"Integration Testing"; the web surface
inherits that decision.

For now, Surface 3 lives as a manual test plan in this PR's body
(see PR #2122's "Test plan" checklist) — open the SSR landing in
a real browser, click the CTAs, confirm the flow works.

#### Phase 2 deliverable: bundle baseline + new integration tests

- [ ] Bundle-size baseline finalization: re-measure with all
  Phase 1 + Phase 2 screens in the initial bundle, commit the
  baseline to `app/web/bundle_baseline.txt`, document the number
  in `app/web/README.md`.

**Validates:** Full guest-to-Plan-claim journey works. Returning
authenticated visitors skip SSR entirely. Headline acceptance
criteria from the parent plan met. Integration tests prevent
regression.

## Acceptance Criteria

Inherited from the parent plan's §"Acceptance Criteria (v1)", scoped
to this sub-issue's deliverables:

- Tapping "I'm out" on the SSR landing shows a confirmation in an
  `aria-live="polite"` region and increments `Experience.web_decline_count`.
- Tapping "I'm in" or "Maybe" navigates to `/app/event/{experience_id}?rsvp=…&code=…`
  on Flutter Web.
- The Flutter Web bundle's initial chunk is under the measured-baseline
  × 1.10 budget; CI gate enforces.
- Phone OTP path: Firebase Auth phone provider, App Check enforced,
  on success → `PhoneRegister` → `RSVPToExperience` → post-RSVP screen.
- Email OTP path: in-house Mailgun OTP → `EmailRegister` →
  `RSVPToExperience` → post-RSVP screen.
- Existing returning-user (already a `CommunityUser` of the hosting
  community) follows the same `RSVPToExperience` call with their
  existing JWT, no OTP.
- Existing returning-user not yet a member of the hosting community
  flows through `AcceptInvitationLink` (now accepting event-flavored
  short_codes), then `RSVPToExperience`.
- Phase 2: a visitor who has previously authenticated in Flutter Web
  is **redirected from `/go/{code}` straight to `/app/event/…`**
  via the `ripls_visited` cookie; the SSR page does not render. The
  Flutter Web app renders the personalized event view using the same
  `ExperienceContent` widget tree as mobile.
- Phase 2: Plan-claim works on web via the existing
  `ExperienceContent` Plan list and `ClaimExperienceNeed` RPC. No
  new claim screen.
- Existing community-invite share links continue to route through
  `AcceptInvitationLink` unchanged.
- Mobile app's Universal/App Link handling for `/go/{code}` event
  share links routes directly to `ExperienceScreen` (which now
  composes the shared `ExperienceContent`). The placeholder
  `EventShareLinkLandingScreen` is deleted.

## Related Issues

- **#1195** — parent issue (P1). This sub-issue is one of five.
- **#2049** — sub-issue 2 (P3, CLOSED). SSR event landing.
- **#2048** — sub-issue 1 (P3, CLOSED). Polymorphic `ShareLink` table.
- **#2051** — sub-issue 4. Host views + at-capacity polish.
  **Scope shrinks** because of #2050's D3: the parent plan's
  personalized-SSR work is no longer needed (returning Flutter-Web
  visitors get redirected into the app instead), and the session-
  cookie infrastructure (`EstablishWebSession`, cookie-aware Connect
  interceptor) is replaced by the breadcrumb cookie that #2050
  delivers. #2051 retains the host-side "joined via event" badge,
  the at-capacity SSR variant copy refinement, and the web-decline
  counter surfacing for the host.
- **#2052** — sub-issue 5. Rate-limit hardening + approval-queue
  schema. Tightens the `RecordWebDecline` rate limit landed here.
- **File new issue:** "Enable Firebase App Check on native iOS + Android
  apps" — P2, references this sub-issue's Web App Check setup as the
  forcing function. Out of scope for #2050, but the more material
  billing-fraud mitigation for the existing native phone-OTP surface.

## References

- `docs/issues/1195-web-guest-rsvp.md` — parent plan, §"Sub-issue 3"
  and Compliance A–L. Inherited by reference throughout; D3 and D4
  deviate.
- `docs/issues/2049-web-event-landing.md` — sibling sub-issue. Phase 1
  DONE. Phase 2 relocated into this sub-issue's Flutter Web foundation
  tasks.
- `server/services/community/share_links.go:314-318` — bounce point
  removed in Phase 1.
- `server/services/community/invitations.go:140-265` — existing
  `AcceptInvitationLink` path, now also accepting event-flavored
  share codes.
- `server/services/login/registration.go:486-548` — `completeRegistration`,
  the helper that already handles `User` + `CommunityUser` for
  event-flavored short_codes.
- `server/services/experience/rsvp.go:22` — `RSVPToExperience`,
  fired client-side after registration.
- `server/services/experience/needs_and_contributions.go` —
  `ClaimExperienceNeed`, fired from Phase 2's Plan-claim screen.
- `server/services/web/event_page.go` + `templates/event.html` —
  Phase 1 SSR landing; CTA buttons get wired in this sub-issue.
- `server/storage/tx.go:35` — `WithTx` helper, available if a future
  combined RPC is justified (not used in this sub-issue per D1).
- `app/lib/presentation/screens/share_link/event_share_link_landing_screen.dart` —
  existing mobile placeholder with `TODO(#1195-sub3)`; resolved by
  routing installed-app users directly to `ExperienceScreen`.
