# Issue #2049: Web build target + SSR event landing page (1195 sub-issue 2)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2049
**Filed:** 2026-05-19 | **Last activity:** 2026-05-20
**Labels:** (none) | **Assignees:** unassigned | **Priority:** P3 | **Type:** Feature
**Context docs:** `docs/issues/1195-web-guest-rsvp.md` (parent plan)

> **Audit pass — 2026-05-20**
>
> Audited against `docs/client/architecture.md`, `docs/client/accessibility.md`,
> `docs/client/i18n.md`, `docs/client/caching.md`, `docs/client/testing.md`,
> `docs/server/architecture.md`, `docs/server/observability.md`. The parent
> plan's Compliance A–L sections are inherited and referenced inline. Audit
> findings folded in: explicit invalid-state rendering in Phase 1 (soft-deleted
> / cancelled / completed events get the same error page as a missing link,
> not an empty render); explicit `//go:embed` directive update for
> `tokens.css` in `website/embed.go`; Phase 1 task to update
> `server/services/web/README.md`; Phase 0 mocks marked reference-only
> (not maintained post-approval); Phase 1 logging adds an explicit `Warn`
> level for fetch failures on dependent entities. Markdown lint warnings
> (MD032 / MD004) fixed inline.

## Problem Statement

A non-member who receives a `/go/{code}` URL where the code points at an
event must currently install the Ripls app to see what the event is. The
SSR landing for that URL exists for community-invite-flavored ShareLinks
(`server/services/web/templates/invite.html`), and the polymorphic
`ShareLink` table in `proto/ripls/models/share_link.proto` already
supports `experience_id`-flavored rows — but the `/go/{code}` router in
`server/services/web/service.go:149-160` short-circuits event-flavored
share links to an "open this link in the Ripls app" placeholder.

This sub-issue closes that gap: route the experience-flavored branch to
a new, dedicated `event.html` SSR template that renders the event
landing (hero, time, place, host, hosting community, attendee list, OG
metadata, three CTA placeholders). Actions land in sub-issue 3 (#2050);
this sub-issue is read-only.

A second, partly overlapping concern is groundwork: sub-issue 3 needs a
Flutter Web build target embedded in the Go server, under a bundle-size
budget, with shared theme tokens. The issue body folds that into this
sub-issue too. The two strands — SSR landing and Flutter Web build —
are mostly independent; only the cache-headers / `/app/*` routing and
the shared theme-token surface touch both.

## Context from Comments

One comment, the triage update from 2026-05-19: priority P3, type
Feature, rationale "lower-priority polish / sub-issue / engineering
tail." That priority sits below the higher-priority infrastructure
work (sub-issue 1 was P-driven by the parent), but the sub-issue
ships on the critical path to #1195 v1 — sub-issue 3 (#2050) is
blocked until either the Flutter Web build target or the SSR
landing is in place (it consumes both).

No clarifications, no disagreements, no open debates. The parent plan
(`docs/issues/1195-web-guest-rsvp.md`) is the authoritative spec; this
sub-issue is one of five.

## Current State of the Codebase

- **`ShareLink` storage model** is in place with the `oneof target { community_invite_id, gear_id, transfer_id, request_id, experience_id }` shape (`proto/ripls/models/share_link.proto`). The polymorphic migration shipped in sub-issue 1 (#2048, CLOSED).
- **`/go/{code}` router** lives in `server/services/web/service.go:84-231`. It looks up the share link, then:
  - if `experience_id` is set → returns a placeholder "open in the Ripls app" error page (lines 149-160). **This is the branch this sub-issue replaces.**
  - otherwise → renders `templates/invite.html` with per-variant item preview from `item_preview.go`.
- **`fetchExperiencePreview`** already exists (`server/services/web/item_preview.go:192-232`) and reads experience + creator + media for the existing invite-template preview when an experience-flavored ShareLink lands. We can reuse / extend its read pattern in `event.html`'s data builder, but the rendering target is a different template.
- **`website/embed.go`** is the existing `go:embed` pattern: serves `/css/*`, `/assets/*`, and `/.well-known/*` from `website/content/`. Mounted in `server/main.go:1370-1374`.
- **Flutter Web scaffolding** exists in `app/web/` (default `index.html`, `favicon.png`, `manifest.json`, `icons/`). No build pipeline, no `go:embed`, no CI gate, no shared theme tokens for web yet.
- **`MaxCommunityMembers = 32`** is in `server/community/constants.go:5`. Capacity check is sub-issue 3's responsibility, but the visual "at capacity" variant of `event.html` does need to exist for golden tests.
- **`fetchItemPreview`** in `item_preview.go:45-72` is the single switch-on-`oneof` point that picks per-variant preview. The `experience_id` case (line 50) is currently dead (never reached, because of the upstream short-circuit at `service.go:149`). Removing that short-circuit is part of this sub-issue.

## Potential Solutions

### Option A: Ship both strands together (full scope as written)

- **Approach:** Implement all 9 tasks from the issue body in one
  sub-issue: Flutter Web build target + `go:embed` + bundle-size CI
  gate + theme-token audit + new `event.html` + router branch + N+1
  test + OG metadata + golden tests.
- **Pros:** Matches the issue spec; sub-issue 3 inherits a fully
  configured Flutter Web foundation and only adds ViewModels +
  screens + actions; validates the Flutter Web pipeline (HTML
  renderer, bundle budget, theme tokens) before any user-visible
  Flutter Web code lands; one PR keeps the cross-cutting concerns
  (cache headers, theme tokens) in a single review.
- **Cons:** Larger PR (~10–15 files across server + Flutter + CI);
  the Flutter Web bundle setup carries the most schedule risk
  (reCAPTCHA Enterprise gotcha is in sub-issue 3, but theme-token
  web-safety audit and bundle-size gate are non-trivial); review
  surface is wider than necessary for the user-visible feature
  (which is purely SSR).
- **Effort:** medium.

### Option B: Split — ship the SSR landing first, defer Flutter Web build to sub-issue 3

- **Approach:** Drop the Flutter Web build tasks from this sub-issue.
  Ship only the dedicated SSR `event.html` template, the router
  branch on `experience_id`, the N+1 budget test, OG metadata,
  golden tests. Move Flutter Web build + `go:embed` + bundle gate +
  theme-token audit into sub-issue 3 (where they're needed for
  Firebase Auth + RSVP + Plan claim screens).
- **Pros:** Smaller, more focused PR; SSR landing is the only
  user-visible deliverable for this sub-issue, so scope matches
  user-visible value; sub-issue 3 doing the Flutter Web setup
  alongside the screens means setup decisions are validated by
  actual screens, not in isolation.
- **Cons:** Departs from the issue spec; sub-issue 3 grows; the
  bundle-budget CI gate doesn't exist when sub-issue 3 starts,
  which removes a guardrail; loses the chance to validate the
  Flutter Web architecture (HTML renderer, theme tokens) before
  any user-visible Flutter Web work lands.
- **Effort:** small for sub-issue 2, larger for sub-issue 3.

### Option C: Split into two phased PRs within this sub-issue

- **Approach:** One sub-issue, two PRs. PR 1 = SSR landing + router
  branch + golden tests + OG metadata + N+1 budget. PR 2 = Flutter
  Web build pipeline + `go:embed` + CI bundle gate + theme-token
  audit. Ship PR 1 first; user-visible feature lands. Ship PR 2
  before sub-issue 3 starts.
- **Pros:** Best of both — user-visible feature ships fast, full
  scope still ships, sub-issue 3 still inherits the Flutter Web
  foundation. Two smaller reviews. PR 1 unblocks anyone who wants
  to validate the SSR experience independently.
- **Cons:** More git ceremony (two PRs, two reviews); PR 2 may sit
  in review while PR 1 ships, creating an interleaving risk; the
  CI bundle gate doesn't exist between PR 1 and PR 2.
- **Effort:** medium (same total work as A, distributed differently).

## Recommendation

**Option B — defer the Flutter Web work to sub-issue 3 entirely
(picked 2026-05-21).**

The user-visible deliverable (event-flavored shortcodes render a real
SSR landing instead of a placeholder) is functionally complete with
just the SSR + router + tests work. Shipping it standalone unblocks:
(a) manually testing the SSR experience end-to-end, (b) using the
landing in field-testing event shares, (c) validating the OG metadata
in real Signal / iMessage / Slack previews, and (d) parent-issue
#1195 progress signaling.

The Flutter Web build pipeline is groundwork for sub-issue 3 (#2050)
and has no user-visible effect on its own — it would ship as
scaffolding only. The original plan called for Option C (split into
two PRs in this sub-issue), but with Flutter 3.41's loss of the HTML
renderer and the realization that the bundle gate enforces a limit
on a near-empty bundle, the value of building the foundation now
versus alongside #2050's actual screens flipped — #2050 is the
right home.

Trade-off: #2050 grows by ~30%, but its bundle-size gate gets set up
alongside the screens that need it (so the budget is grounded in
real measurements rather than scaffolding overhead).

## Open Questions

- **What do the three CTA buttons link to in this sub-issue?**
  Recommendation: render them as `<button disabled>` with an
  explanatory `aria-label` ("Coming soon — RSVP available shortly")
  and an `aria-disabled="true"` attribute. Don't link them to the
  app deep link (that creates a confusing experience where Yes/Maybe
  do nothing on the web but force-install the app on mobile);
  don't link them to anchors that scroll nowhere. Sub-issue 3 wires
  them. This matches the issue body's "initially as static links,
  wired in sub-issue 3" instruction interpreted as "static / no-op"
  rather than "static href to app URL."

- **What does the attendee-list section render when no one has
  RSVPed yet?** Recommendation: show the host card alone with
  "Hosting" label and no attendee section header. Empty attendee
  state is a common case for a fresh event share; the host's face
  is enough social proof. Document the empty case as a golden-file
  variant.

- **What does the "at capacity" SSR variant render?** Decision:
  same template, capacity check done at render time using
  `community.GetNumCommunityMembers` (already a shipped helper at
  `server/community/constants.go`-adjacent) + `MaxCommunityMembers`
  comparison. When at-capacity, **all three CTA buttons are hidden
  entirely** (not greyed-out / `disabled`) and replaced with a
  single message: "Can't RSVP because the hosting community is full
  (32 of 32 members). Ask your inviter for options." Rationale:
  greyed-out controls are an accessibility anti-pattern (screen
  readers still announce them; visitors wonder why they can't
  click) and convey no actionable next step. The replacement
  message tells the visitor exactly what to do. Hardcoding a fake
  at-capacity flag for golden tests is acceptable in this sub-issue
  since the rendering is identical to what sub-issue 3 will trigger
  from the same check.

- **`og:image` for the event landing — hero or community thumb?**
  Recommendation: hero image (event's `media_ids[0]`), fall back to
  community thumbnail, fall back to default Ripls logo. Mirrors
  `getCommunityImageURL` (`service.go:262-289`) and `getMediaImageURL`
  (`item_preview.go:236-263`) — both already cache for `ogImageDuration`
  (1h). The 30-second SSR HTML cache and the 1-hour presigned-URL
  cache are independent.

- **Where does the bundle-size CI gate run, and how does it fail?**
  Recommendation: shell script under `scripts/check_web_bundle_size.sh`
  that runs `gzip -c app/build/web/main.dart.js | wc -c` (plus
  `flutter_bootstrap.js` and the deferred-loaded chunks tagged as
  initial) and fails if total > 1 MB. Wire into a new GitHub Actions
  job `.github/workflows/test_flutter_web.yaml`. Stays out of the
  default `test_flutter.yaml` so it only runs when web assets
  change. Document the script in the new `app/web/README.md`.

- **`--web-renderer=html` is deprecated in recent Flutter.** As of
  Flutter 3.27+, `--web-renderer` is gone and the renderer choice
  moved to `<head>` config. Recommendation: verify the project's
  Flutter version (`flutter --version`); if <3.27, use the flag; if
  >=3.27, configure the renderer via `index.html` or via the
  `flutter_bootstrap.js` initialization options. Either way, the
  goal is "render to DOM, not CanvasKit." Document the choice in
  `app/web/README.md` with the Flutter version it's pinned to.

- **`/app/*` cache headers?** Recommendation: long cache
  (`Cache-Control: public, max-age=31536000, immutable`) for hashed
  assets (`*.js`, `*.css`, `*.png` with a content-hash in the
  filename); no cache (`Cache-Control: no-store`) for `index.html`
  and the Flutter manifest. Mirrors the standard SPA-on-CDN
  pattern. The `/go/{code}` SSR landing gets its own short-TTL
  cache header (`Cache-Control: public, max-age=30`) per the
  issue task.

- **Should `event.html` reuse the existing `templates/invite.html`
  styles or get its own CSS?** Recommendation: factor shared CSS
  into `website/content/css/shared.css` (already an embedded
  asset) plus a new `website/content/css/tokens.css` mirrored from
  `app/lib/core/theme/app_colors.dart`. The event landing is
  visually distinct enough — hero image, attendee list, three CTA
  buttons — that copy-pasting `invite.html`'s body is a non-starter.
  Lift the shared bits (header, footer, typography) into
  `shared.css`; isolate the color/spacing tokens in `tokens.css`;
  each template imports both. Phase 0 produces the token file.

- **How do we keep `tokens.css` in sync with `app_colors.dart`?**
  Recommendation: hand-mirror with a top-of-file comment naming
  `app/lib/core/theme/app_colors.dart` as the source of truth, plus
  a follow-up issue to codegen the CSS from the Dart constants.
  Hand-mirroring is acceptable in the short term because the color
  set is small (~20 tokens) and changes rarely. Codegen lifts the
  drift risk longer term. Until codegen ships, any color change in
  `app_colors.dart` includes a `tokens.css` update in the same PR;
  a CI check that diffs the two files is a stretch goal for the
  codegen follow-up.

- **English-only v1 strings — who files the Go-side i18n
  follow-up?** Recommendation: file it from this sub-issue as a
  P3 issue titled "Server-side i18n for SSR templates" referencing
  this issue and #1195. Keeps the trail clean for when a
  non-English market needs it.

## Implementation Plan

Three phases, one sub-issue. Phase 0 is an interactive design-review
gate that produces an approved visual direction. Phase 1 implements
that direction as the SSR event landing. Phase 2 lands the Flutter
Web build groundwork that sub-issue 3 needs.

### Phase 0 (DONE — 2026-05-20): HTML mock variants + design approval

**Outcome:** **Mock H (Hybrid)** approved as the design direction.
Synthesizes Mock A's full-bleed hero + sticky footer with Mock C's
compact body. Specifically:

- Hero: ~52vh full-bleed photo, dark scrim fading through cream
  gradient into body background (no hard cut). Kicker
  "{community} · {day}" + serif headline.
- Body: compact pill-row metadata (📅 date · ⏰ time · 📍 place),
  single-row host card with 40px avatar + role line, avatar
  cluster + "X, Y, Z + N others going" inline.
- Sticky footer CTA bar with three buttons: **"I'm in"** (sage
  primary), **"Maybe"** (sage outline), **"I'm out"** (subtle
  neutral). Casual copy, not formal "Yes / No."
- At-capacity: sticky footer swaps entirely to a message bar — no
  greyed-out buttons. Message: "Can't RSVP because the hosting
  community is full ({N} of {max} members). Ask your inviter for
  options."

Mock file: [`2049-mocks/mock-h-hybrid.html`](2049-mocks/mock-h-hybrid.html).
The mock is the structural template Phase 1 ports verbatim.

**Goal (original):** Before touching the Go templates or server code, the user
and I agree on one visual direction for the event landing — hero
treatment, attendee-list density, CTA layout, typography, spacing —
using the *Flutter app's actual color tokens* so the SSR landing
feels native to the brand.

**Why a separate phase:** Implementing the wrong look first means
either (a) a throwaway redo after the design feedback lands, or (b)
worse, shipping a landing that doesn't feel like Ripls. The mock
review is cheap (static HTML files, no build pipeline, no server
plumbing) and produces a single artifact — the approved mock — that
Phase 1 then ports into the Go template.

The mock files themselves are **reference-only artifacts**: once
Phase 1 ships, they aren't kept in sync with the production
`event.html`. They're a design-conversation record, not living docs.

**Tasks:**

- [ ] Extract the Flutter app's color tokens from
  `app/lib/core/theme/app_colors.dart` into a generated
  `website/content/css/tokens.css` file with both light-mode and
  dark-mode custom properties (`--color-primary`,
  `--color-background`, `--color-text-primary`, `--color-surface`,
  etc.). Source of truth stays the Dart file; this CSS is generated
  from it (or hand-mirrored with a comment noting the source). Dark
  mode via `@media (prefers-color-scheme: dark)`.
  - Include at minimum: `lightPrimary` / `darkPrimary` (heritage
    sage), `lightSecondary` / `darkSecondary` (warm beige),
    `lightBackground` / `darkBackground` (cream / dark brown),
    `lightSurface` / `darkSurface`, `lightTextPrimary` /
    `darkTextPrimary`, `lightTextSecondary`, `lightBorder`, plus
    semantic colors (`success`, `warning`, `error`).
  - Document the mirror mechanism in `app/lib/core/theme/README.md`
    so future token changes have a clear update path. File a
    follow-up issue for the eventual codegen ("Generate
    `tokens.css` from `app_colors.dart`") if hand-mirroring.
  - **Dark mode scope:** the SSR landing inherits the visitor's OS
    appearance via `@media (prefers-color-scheme: dark)`, mirroring
    the Flutter app's auto-theming. No user toggle in v1 — adding
    one would require server-side cookie state, which is sub-issue
    4's concern. If the visitor has no preference, light is the
    default (matches the Flutter app).
- [ ] Build **3–4 static HTML mock variants** under
  `docs/issues/2049-mocks/`, each a self-contained `.html` file
  consuming `tokens.css` and using realistic sample data
  (event "Saturday Pickup Soccer", host "Emma R.", community
  "Two Oaks Block", 6 attendees including some with photos and
  some without, time = next Saturday 10am). Each mock explores a
  different direction:
  - **Mock A — "Editorial":** full-bleed hero image with title
    overlay, attendee strip below, CTAs as outlined buttons in a
    sticky footer bar. Magazine-style typography
    (larger title, generous whitespace).
  - **Mock B — "Card-stack":** centered card layout, hero as a
    rounded image at the top of the card, time/place as
    inline-icon rows, attendees in a horizontal scroll strip,
    three CTAs stacked vertically below. Closer to Partiful's
    feel.
  - **Mock C — "Compact list":** dense single-column layout with
    smaller hero, time/place/host as a metadata block,
    attendees as a single avatar-cluster ("Emma + 5 others"),
    three CTAs in a horizontal row. Optimized for fast scan and
    small screens.
  - **(Optional) Mock D — "Hybrid":** picks the strongest
    elements from A/B/C after I see how each one feels.
- [ ] Each mock includes an **at-capacity variant** (disabled CTAs,
  banner) and an **empty-attendee variant** (no one RSVPed yet)
  side-by-side, so the edge cases get reviewed at the same time as
  the happy path.
- [ ] Each mock uses **semantic HTML** that Phase 1's Go template
  will inherit verbatim: `<main>`, `<button>`, `aria-label`,
  keyboard focus order. The mocks aren't just visual — they're
  the structural template for Phase 1's `event.html`.
- [ ] Mocks are openable directly in a browser (no server, no
  build step). Include a tiny `docs/issues/2049-mocks/README.md`
  with "open these files in a browser" instructions and a
  side-by-side dark/light mode toggle hint (just toggle OS
  appearance — the CSS already responds).
- [ ] **Present the mocks to the user via `AskUserQuestion`** with
  each mock's hero approach in the preview field. The user picks
  one (or asks for changes). Iterate until approved. The approved
  mock's HTML *is* the starting structure for Phase 1.

**Validates:** The visual direction is locked before any Go template
work. The Flutter app's color tokens are extracted into reusable CSS
that future SSR templates (and this one) consume — so the SSR
landing reads as the same product as the mobile app, not a separate
web property.

### Phase 1 (PR 1): SSR event landing + router branch

**Goal:** A browser hitting `/go/{code}` where the code points at an
experience sees a server-rendered event landing in <500ms TTI, with
correct OG metadata for Signal / iMessage / Slack previews.

**Tasks:**

- [ ] Port the **Phase 0 approved mock** into `event.html` under
  `server/services/web/templates/` — same DOM structure, same
  semantic HTML, swap the static sample data for Go template
  variables. Include hero image, event name, time, place, host
  name (first name only), hosting community name, attendee list
  (first name + profile photo, first 8 RSVPed), three disabled
  CTA buttons (Yes / Maybe / No) with explanatory `aria-label`,
  OG metadata block. Semantic HTML: `<main>`, `<button>`,
  `aria-label` on icon controls, focusable in keyboard order.
- [ ] Add `tokens.css` (created in Phase 0) to the `go:embed`
  surface in `website/embed.go`. Both `invite.html` and
  `event.html` `<link rel="stylesheet">` it. Lift any shared
  header / footer / typography rules from `templates/invite.html`
  into `website/content/css/shared.css` and have it `@import`
  `tokens.css`. Single source of truth for visual tokens.
- [ ] Replace the experience-id short-circuit at
  `server/services/web/service.go:149-160` with a new
  `renderEventLanding(...)` codepath. Reuse `fetchExperiencePreview`
  (extending it as needed) and add `fetchHostingCommunity`,
  `fetchAttendeeList` (first 8 RSVPed users, ordered by `rsvped_at`).
- [ ] **Invalid-state rendering:** when the experience is
  soft-deleted, cancelled, completed, or its hosting community has
  been deleted, render the same error page the missing-link branch
  uses today (`s.renderInvitePage(w, invitePageData{IsValid:
  false, ...})` with an event-appropriate `ErrorMessage`). Do not
  fall through to an empty event template render. Sub-issue 3
  needs the same gating before triggering OTP; consolidating the
  "is this event RSVPable" check in Phase 1 means sub-issue 3
  inherits it.
- [ ] Update `//go:embed` directive in `server/services/web/service.go`
  to include `templates/event.html`, and update `website/embed.go`'s
  `//go:embed` directive to include `content/css/tokens.css`. Both
  templates parsed in `web.New(...)`.
- [ ] Update `server/services/web/README.md` to document the new
  `event.html` template, the `renderEventLanding` path, and the
  `event_page_test.go` golden-file convention. Per CLAUDE.md: every
  multi-file directory under `server/` has a README; this directory's
  already exists and needs updating.
- [ ] Avoid N+1 on landing render per **Compliance G**: batch-fetch
  event + community + location + RSVP list + users (host + first 8
  attendees in one `QueryByFieldIn`) + media (hero + community
  thumb + all avatars in one `QueryByFieldIn`) + member count, for
  ≤7 storage queries total. Add `storage.AssertMaxQueries(t, ctx,
  7, fn)` to a new test in
  `server/services/web/event_page_test.go`. The budget was estimated
  at ≤5 during planning; the actual figure includes the Location
  fetch (place name in the meta-pill) and the `GetNumCommunityMembers`
  COUNT for the at-capacity check, neither of which is foldable
  into a single bulk query. The N+1 protection is still meaningful
  because attendee user/media fetches are batched.
- [ ] OG metadata: `og:title` ("Join {host} for {event} in
  {community}"), `og:description` (event time formatted), `og:image`
  (hero → community thumb → Ripls logo fallback, presigned URL via
  `getMediaImageURL` / `getCommunityImageURL`, cached for
  `ogImageDuration` = 1h). `og:url`, `og:type=website`,
  `og:site_name=Ripls`.
- [ ] SSR HTTP cache header: `Cache-Control: public, max-age=30`
  on the response.
- [ ] Logging per **Compliance D**: `logger.With("operation",
  "HandleEventLanding", "short_code", logging.MaskToken(shortCode),
  "community_id", ..., "experience_id", ...)`. Log levels:
  - `Info` for normal renders, capacity-disabled renders,
    invalid-state renders (soft-deleted / cancelled / completed
    events); these are user-facing states, not system failures.
  - `Warn` for partial fetch failures that fall back to defaults
    (missing hero media → community thumb fallback, missing host
    user record → render anyway with placeholder).
  - `Error` only for storage failures that prevent rendering
    (`s.storage.GetByID(experience)` returns a non-not-found
    error). Wrap internal-error responses via existing patterns;
    no new RPC handlers in this sub-issue so `connecterr.Internal`
    doesn't apply directly here, but the HTTP handler should
    return a 500 with a generic public message rather than leaking
    storage error text.
- [ ] Golden-file tests in `event_page_test.go` per **Compliance I**:
  normal render, at-capacity variant (cap-1 == numMembers), empty
  attendee list, missing hero image, very-long event name (60+ chars),
  very-long host name, RSVPed-by-host variant. v1 strings English-only
  per **Compliance B**.
- [x] File a follow-up issue "Server-side i18n for SSR templates"
  referencing this issue, marked P3. Filed as **#2090**.

**Validates:** End-to-end SSR landing renders for an event shortcode;
existing community-invite landing unchanged; OG previews work in
Signal / iMessage / Slack; ≤5 queries per render; English-only v1
strings tracked.

### Phase 2 (moved to sub-issue 3 / #2050)

**Originally:** `flutter build web` + `go:embed` pipeline +
`/app/*` route + bundle-size CI gate + theme-token audit +
deferred-route infrastructure, shipped here as PR 2 so sub-issue 3
would inherit a working foundation.

**Rescoped 2026-05-21:** moved into sub-issue 3 (#2050). With
no Flutter Web screens to consume the foundation, Phase 2 ships
scaffolding only — `/app/*` serves a placeholder `index.html`,
the bundle gate enforces a limit on a near-empty bundle, the
theme audit prevents future drift. Real value lands when #2050
wires guest screens, so the foundation is most useful when built
alongside them.

**Findings from Phase 2 reconnaissance that #2050 inherits:**

- This project runs Flutter 3.41.6, which removed `--web-renderer`
  entirely. The HTML renderer is gone; only CanvasKit (JS) and
  CanvasKit-on-WASM (`--wasm`) remain. **Compliance K's "use HTML
  renderer" path is no longer available.** The realistic bundle
  floor is ~1.5-2 MB gzipped for a minimal app rather than the
  original 1 MB target.
- Theme-token audit findings: zero `Platform.is*` / `dart:io`
  references in `app/lib/core/theme/`, so theme tokens are 100%
  web-safe out of the box. The wider app has 7 files using
  `Platform.is*` (feedback, FCM, location-picker, deferred deep
  links, app badge) — none reachable from a guest-only entry
  point, so they're a sub-issue #2050 concern when guest screens
  wire actual reachability.
- Revised bundle-budget approach: **measure baseline + ~10%
  headroom** rather than a hard 1 MB cap that's not achievable
  on current Flutter. CI gate catches regressions; absolute floor
  is whatever a minimal `flutter build web` produces.

**Tasks (relocated to #2050):**

The original Phase 2 task list — `flutter build web` pipeline, the
`go:embed` wiring under `server/services/web/app_embed.go`, the
`/app/*` route, the bundle-size CI gate, the deferred-route
infrastructure, and `app/web/README.md` — moved into #2050
unchanged, with two notes:

1. Bundle target adjusted: **measure baseline + ~10% headroom**
   (not a hard ≤1 MB cap — see findings above).
2. Wider `Platform.is*` audit (7 files outside `core/theme/`)
   becomes a #2050 concern once guest screens actually reach
   those code paths via the initial bundle.

## Related Issues

- **#1195** — parent issue. This sub-issue is one of five.
- **#2048** — sub-issue 1 (CLOSED). Polymorphic `ShareLink` table +
  migration. Prerequisite for the `experience_id` branch this
  sub-issue routes on.
- **#2050** — sub-issue 3. Consumes both deliverables: the SSR
  landing (provides the CTAs that #2050 wires) and the Flutter Web
  build pipeline (where the OTP / RSVP / Plan-claim screens land).
- **#2051** — sub-issue 4. Polish: surfaces the personalized SSR
  landing variant via session cookie. Builds on this sub-issue's
  template structure.
- **#2052** — sub-issue 5. Hardening: rate limits + approval queue
  schema. Independent of this sub-issue's deliverables.
- **#2058** — referenced from `item_preview.go:41` as the
  deprecation horizon for the `GetOrCreateInviteLink` shim. Not
  blocking this sub-issue.

## References

- `docs/issues/1195-web-guest-rsvp.md` — parent plan, §"Sub-issue 2"
  and the compliance sections referenced throughout (Compliance A
  through L).
- `proto/ripls/models/share_link.proto` — polymorphic share-link
  schema.
- `server/services/web/service.go` — current `/go/{code}` router;
  this is where the new branch lands.
- `server/services/web/item_preview.go` — existing
  `fetchExperiencePreview`; reusable read pattern for the new
  landing.
- `website/embed.go` — `go:embed` pattern to mirror for `/app/*`.
- `server/community/constants.go` — `MaxCommunityMembers = 32`
  for the at-capacity variant.
