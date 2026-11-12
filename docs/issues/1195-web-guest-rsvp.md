# Issue #1195: Web UI for shared event planning that allows non-members to participate

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1195
**Filed:** 2026-04-20 | **Last activity:** 2026-05-19
**Labels:** (none) | **Assignees:** unassigned | **Priority:** P1
**Context docs:** `docs/users/growth-use-cases.md`, `docs/users/priorities.md`, `docs/users/personas.md`, `docs/invitations.md`

> **Audit pass — 2026-05-19**
>
> Audited against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/client/accessibility.md`, `docs/client/testing.md`, `docs/server/architecture.md`, `docs/server/observability.md`. Audit findings folded into a new **Architecture Compliance Notes** section (sections A–L) and threaded into the sub-issue task lists. Key additions: MVVM + Riverpod + Stash pattern for new Flutter screens (A), ARB keys in both en + es (B), Tappable / IconAction / semantic HTML primitives (C), `logging.LoggerWithContext` + masked short codes (D), `connecterr.Internal` for internal errors (E), service-independence + file split (F), indexed storage queries + `AssertMaxQueries` budget (G), proto conventions including the storage/API split (H), per-layer testing (I), Firebase Auth Web reCAPTCHA Enterprise gotcha (J), Flutter Web HTML renderer for the guest bundle (K), session-cookie semantics for cross-surface auth (L).
>
> **Revision — 2026-05-21 (sub-issue 3 planning, [[2050-web-rsvp-actions]]):**
> Compliance J revised from reCAPTCHA Enterprise to **Firebase App Check** with the reCAPTCHA v3 web provider — same Google-recommended posture, simpler setup, and sets up the path for native App Check (App Attest + Play Integrity) to mitigate phone-OTP billing fraud on the existing mobile flow.
> Compliance K revised from "use HTML renderer ≤1 MB" to "CanvasKit default, measure-baseline + ~10% headroom" because the HTML renderer was removed in Flutter 3.27+.
> Compliance L revised from signed-JWT session cookie + new Connect interceptor to a **content-free breadcrumb cookie** that redirects authenticated visitors from `/go/{code}` straight into the Flutter Web app. Sub-issue 4's task list shrinks accordingly.
> Sub-issue 3's `AcceptEventInvite` RPC dropped in favor of a **two-step client-side flow** (`PhoneRegister`/`EmailRegister` with short_code → `RSVPToExperience` with the resulting JWT). The combined-transaction RPC is replaced by client-side retry on the idempotent RSVP write.
> Sub-issue 3's separate "Flutter Web event view + RSVP confirm + Plan claim" screens replaced by an **extraction**: pull the existing mobile `ExperienceScreen`'s content body into `ExperienceContent`, then write a thin `WebExperienceScreen` shell that composes the same widget tree. Establishes the pattern for future web surfaces.

## Problem Statement

Today every Ripls participant must be a community member, and every community member must have completed app registration. This single assumption blocks the product's top-ranked growth use cases — the Potluck/Event Host (Emma), the Crisis Coordinator (Betty), the Ride/Trip Sharer, the Group Gift Organizer — because each one depends on inviting people who *aren't* already users and letting them act on the invitation in one tap. A friend forwarding a Saturday block-party link to her sister has no path that doesn't end in "download this app first."

Issue #1195 proposes a login-free web surface that lets someone with a shareable shortcode view an event and act on it (RSVP, claim a slot from the Plan) without installing the app. The product narrative is "the invite IS the event" — Partiful's breakout property — adapted to Ripls' invite-only, bounded-circle constraints.

The latest direction (May 19, 2026 comment) narrowed the v1 scope and gave clear architectural opinions: start with events only, defer payments, build on the existing Flutter codebase with as little web-specific code as possible, share theming, focus on mobile-web with adaptive design, encode community+target item in the shortcode, and phase the work into sub-issues.

## Context from Comments

Three comments, all from the issue author:

**Triage 2026-04-20** — flagged four scope questions to resolve before execution: payment flow, identity continuity, notification channel, anti-abuse posture. Required a design doc before implementation.

**Triage 2026-04-24** — priority set to P1, rationale being "high-impact issue for key personas tied to top-tier priorities."

**Direction 2026-05-19** (the load-bearing comment) — narrowed scope to:

- **Start with events.** Giveaways/loans/requests/community-invites layered later.
- **Defer payments/donations.** No money handling in v1.
- **Build on Flutter / current client codebase** as much as feasible; some web-specific code accepted.
- **Share look-and-feel constants** (colors, fonts) with the mobile app.
- **Mobile-phone-web focus**, adaptive design preferred.
- **Improve shortcodes** to encode (community, target item type, target item id) in a single 8-char code.
- **Phase into sub-issues.**
- **Think broadly** and compare against Partiful, Evite, Meetup, GoFundMe before anchoring to existing UI.

The four scope questions from the 2026-04-20 triage are answered below in "Resolved Design Decisions."

## Resolved Design Decisions

These were worked through interactively before writing this plan. Captured here so sub-issues can reference them without re-litigating.

### 1. Architecture: SSR landing + Flutter Web for actions

- `/go/{code}` is server-rendered HTML (Go templates), fast first paint, identical to OG-link-preview content. The page shows the event hero, time, place, host, and RSVP'd attendees, with Yes / Maybe / No CTAs. The SSR page is functional, not just a billboard.
- The Plan ("what's needed") list is *not* on the SSR landing; it appears in Flutter Web after a successful RSVP. This keeps the landing tight and the post-RSVP experience richer.
- Tapping Yes/Maybe loads the Flutter Web bundle, which uses **Firebase Auth (phone provider)** for OTP — the same code path the mobile app already uses. This reuses the existing auth widgets and avoids a duplicate Go OTP implementation.
- Tapping No is a pure client-side acknowledgment (see Decision 5).
- The mobile app continues to handle the same `/go/{code}` URLs via Universal/App Links. Installed-app users follow the existing native invitation flow; the web flow only kicks in when the app is not installed (or on desktop).

### 2. Flutter Web hosting: embed in Go binary

- Build pipeline: `flutter build web` produces `app/build/web/`; that directory is embedded into the Go server via `go:embed` and served from a route like `/app/*`.
- Mirrors the existing `website/embed.go` pattern. Single Cloud Run process, no nginx changes, atomic deploys.
- The Flutter Web bundle uses **deferred / lazy-loaded routes**: only the guest-reachable surface (event view, Firebase Auth, RSVP, Plan claim) is in the initial bundle. The full mobile app's screens stay out of the guest bundle. Bundle target: ≤1 MB gzipped initial download.

### 3. Identity: phone OTP creates a real Ripls User and joins the community

- There is no "guest" entity. A successful "yes" or "maybe" RSVP runs the existing registration flow end-to-end: phone OTP → creates a `User` row identical to a natively-registered user → creates a `CommunityUser` row (joins the hosting community) → records the `Rsvp`. All in one atomic operation. They simply haven't installed the app.
- **Every successful RSVP yields a `User` with at least one `CommunityUser` row.** There is no new "memberless user" state in the system. If the community is at the 32-cap, the RSVP is refused before OTP (see Decision 6) and no `User` is created.
- The mobile app's auth path uses the same Firebase phone provider, so if the user later installs the app and signs in with the same phone, they pick up their existing account, community membership, and RSVPs intact — no "claim history" flow required.
- **Email is a quieter fallback** to phone (Firebase Auth supports email link). The phone option is the visual default; "use email instead" is a secondary link on the entry screen. Email-OTP'd users go through the same registration + community-join path.

### 4. ShareLink: one polymorphic table with proto `oneof` target

- New table `ShareLink` with a proto `oneof target { string community_invite_id; string event_id; ... }`. The populated oneof variant is itself the discriminator — no separate `target_kind` enum.

```proto
message ShareLink {
  string id = 1;
  string short_code = 2;
  string issuer_id = 3;          // user who created the link
  string community_id = 4;       // community this link is associated with (always set)

  // Populated variant identifies the target. Exactly one is set.
  oneof target {
    string community_invite_id = 5;  // legacy community invite; value mirrors community_id
    string event_id = 6;             // event RSVP target
    // Reserved for future:
    // string giveaway_id = 7;
    // string loan_id = 8;
    // string request_id = 9;
  }

  bool is_revoked = 10;
  int64 created_at_unix_sec = 11;
}
```

- Existing `CommunityInvitationLink` rows migrate into `ShareLink` with `community_invite_id` set. Existing RPCs (`GetOrCreateInviteLink`, `AcceptInvitationLink`, `RevokeInviteLink`, `CheckInvitation`) keep their current contracts but read/write `ShareLink` underneath.
- The shortcode itself remains opaque — 8 chars from the existing 54-char alphabet, ~46 bits of entropy, no semantic encoding. The "encode the target item in the code" framing from the May 19 comment is satisfied by the typed `target` `oneof` in the row that the code points to, not by bits inside the URL string. URLs stay random-looking and the type taxonomy isn't baked into URL strings.
- The SSR router does one `ShareLink` lookup, branches on which `oneof` variant is set to render the appropriate template. The community-invite template stays byte-identical to today's `templates/invite.html`; the event template is new.

### 5. "No" RSVP is a no-op with anonymous counter

- Tapping No on the landing page is purely client-side: a "thanks" confetti / message and an increment to an event-scoped anonymous `web_decline_count`. No User created, no OTP, no SMS, no abuse vector.
- The host sees the counter in the app: "No (web): 3" — no names, no identity. Distinguishes web-declines from member-declines (which do have names).
- This asymmetry — declining costs nothing, committing requires real identity — matches the spirit of "saying no should never feel like signing up for something."

### 6. Capacity behavior

- Community 32-member cap (configurable 15–50 per priorities-doc) applies to Yes/Maybe RSVPs from web exactly as to today's `AcceptInvitationLink`.
- **Check capacity at SSR render time.** When the SSR landing renders, it queries community member count (already shipped in #2049 Phase 1). If full, the page hides the Yes/Maybe CTAs and shows a "community is full" message bar; "I'm out" remains available. No wasted SMS because the user never reaches the OTP screen.
- For Flutter Web visitors who breadcrumb-redirect into the app: the app's existing experience screen already handles at-capacity and existing-membership cases for mobile, so web inherits that behavior via `ExperienceContent` (see [[2050-web-rsvp-actions]] D5). No separate web-only `CanRSVP` RPC is needed — the existing capacity logic on `AcceptInvitationLink` and `RSVPToExperience` suffices.

### 7. Scoped transparency

- A guest on the SSR landing sees: event name, time, place, host name, hosting community name, hero image, list of attendees (first name + profile photo), RSVP counts, and the three CTAs.
- A guest does NOT see: other community events, full community member list, chat history, other community gear/requests, member profile detail screens, the community feed, or anything that would require an authenticated session in a different scope.
- After RSVP (post-Firebase Auth, in Flutter Web), the new user sees the Plan list and can claim a slot. They also see the event's conversation (chat with host + other attendees) because they're now a `CommunityUser` with full membership rights for that community. They do not see any other community's content unless they accept additional invites.
- Profile-photo exposure to anyone with a shortcode is a deliberate trade for social proof. Privacy opt-out (members who don't want their photo public) is a v2 concern — flagged in Open Questions.

### 8. Anti-abuse posture (Tier 2)

- Phone OTP gates state-mutating actions. Rate limits: 5 OTP requests/hour/IP, 3 OTP requests/hour/phone. These are existing infrastructure-level limits we can extend.
- The shortcode itself is a meaningful barrier — 46 bits of entropy, not enumerable, not indexed anywhere. An attacker needs a valid live shortcode to even reach the OTP endpoint.
- Schema-prep an organizer-approval queue (`Event.guest_approval_required` bool, `GuestApprovalRequest` table) but ship with toggle defaulted off and no UI. Sub-issue 5 covers the schema; UI lands in a future issue when crisis-rally use cases need it.

## Architecture Compliance Notes

The plan must follow established Ripls architectural conventions. Audit notes below; each sub-issue references these by section number.

### A. Flutter Web follows the same MVVM + Riverpod + Stash pattern as mobile

Per `docs/client/architecture.md`:
- Guest event view, OTP entry, RSVP confirm, and Plan-claim screens each get a **ViewModel** (Riverpod `Notifier` with **Freezed** immutable state). Business logic lives in the ViewModel; widgets only compose UI.
- Screens consume state via `ref.watch()`, trigger actions via `ref.read(provider.notifier)`.
- Use `NotifierProvider.autoDispose` for screen-scoped state. Apply `SafeNotifierMixin` (or migrate to `AsyncNotifier`) on any notifier with async work — `ref.mounted` checks are required after every `await` before `state =`.
- Data access goes through **repositories** with **Stash** caching; ViewModels never call services directly. Add `EventShareRepository` (new) for read access to event share-link metadata, and reuse `ExperienceRepository` / `RsvpRepository` (existing patterns) for the event entity and RSVP mutations.
- All colors via `AppColors.xxx(context)`. No hardcoded `Color(0xFF...)` or `Colors.white`. Theme tokens from `app/lib/core/theme/` are the source of truth; the SSR landing's CSS mirrors them (via the `website/` shared asset directory pattern).

### B. i18n is required for every user-visible string

Per `docs/client/i18n.md`:
- Every Flutter user-visible string uses `context.l10n.*`. ViewModels never resolve strings — they expose typed state (enums, error codes); widgets resolve in `build()`.
- New ARB keys land in **both** `app/lib/l10n/app_en.arb` and `app/lib/l10n/app_es.arb`, each with `@key` metadata. CI (`npm run lint:dart:i18n`) fails on hardcoded strings.
- Key prefix conventions: this work uses `eventShare*` for the event-share landing flow, `rsvpWeb*` for the web RSVP flow, `guestOtp*` for the Firebase Auth phone flow, and `commonRsvp*` for shared RSVP-state language.
- For RPC errors, the `UserError` + `RpcErrorHandler.classify`/`localize` pair is the canonical shape — see `app/lib/core/errors/README.md`.
- **SSR templates** sit outside the Flutter i18n system but should use English-only v1 strings clearly marked for follow-up translation. A separate issue can introduce Go-side i18n if/when needed.

### C. Accessibility primitives are required (Flutter) and semantic HTML is required (SSR)

Per `docs/client/accessibility.md`:
- All taps on Flutter Web use `Tappable` (not raw `GestureDetector` / `InkWell`). RSVP-state toggles use `Toggle`. Any icon-only button uses `IconAction` with both `semanticsLabel` and `tooltip`.
- Semantic labels come from `context.l10n.a11y*` keys. Lint rule `require_l10n_for_semantic_labels` rejects raw string literals on `semanticLabel:` / `tooltip:`.
- `CachedMediaImage` for event hero / community thumbnail / RSVP'd-attendee avatars — each with a content-specific `semanticsLabel`.
- For Plan-claim status changes ("Slot claimed"), wrap the affected widget in `LiveRegion` or fire `SemanticAnnouncer.announce`.
- Animations use `accessibleDuration(context, ...)` so reduce-motion users get zero-duration transitions.
- The `scripts/a11y_allowlist.txt` is empty on `main` — any new violations must be fixed in the same PR.
- **SSR landing** uses semantic HTML: `<main>`, `<button>` (not `<div onclick>`), `aria-label` on icon-only controls, `aria-live="polite"` regions for client-side decline confirmation. Keyboard navigation (Tab order, Enter to activate) is required. Test with VoiceOver and a screen reader.

### D. Server uses structured logging via `logging.LoggerWithContext(ctx)`

Per `docs/server/observability.md` and `CLAUDE.md`:
- All new RPC handlers begin with `logger := logging.LoggerWithContext(ctx).With("operation", "<RPCName>", "short_code", logging.MaskToken(shortCode), "community_id", ..., "experience_id", ...)`.
- Never log the bare short code (it's a long-lived bearer secret) — always wrap in `logging.MaskToken(...)`. The existing `HandleInvitePage` already follows this pattern; new code must too.
- Standard field names: `request_id` is reserved for the HTTP correlation ID (auto-injected by `LoggerWithContext`). Entity IDs use their own names: `community_id`, `experience_id`, `share_link_id`, `user_id`, `target_request_id` (for `models.Request`, never `request_id`). Mask emails with `logging.MaskEmail`.
- Log levels: `Info` for normal RPC entry/exit, `Warn` for handled-but-unexpected (capacity refusal, OTP rate-limit hit), `Error` only for system failures (DB/Firebase Auth/Twilio failures requiring attention). Capacity refusals and bad OTPs are `Info` or `Warn`, not `Error` — they're user errors and would otherwise blow up the auto-filed-issue pipeline.
- Goroutines (if any) use `logging.GoSafe(ctx, "<name>", fn)` per the goroutine-safety convention.

### E. Connect errors use `connecterr.Internal` for internal failures

Per `docs/server/architecture.md` and `CLAUDE.md`:
- Internal server errors use `connecterr.Internal(ctx, "<Operation>.<step>", err, "share_link_id", id, ...)` — never bare `connect.NewError(connect.CodeInternal, err)`. The bare form is gated by `scripts/check_codeinternal.js` and fails CI.
- User-visible errors (invalid OTP, expired shortcode, capacity refused, etc.) return appropriate Connect codes (`InvalidArgument`, `FailedPrecondition`, `ResourceExhausted`) — these carry a public message back to the client and *should* be specific.
- Localized client-side rendering of these errors flows through `RpcErrorHandler.classify` (Dart side) — see `app/lib/core/errors/README.md`.

### F. Server services follow the independence + shared-library conventions

Per `docs/server/architecture.md`:
- **Service-to-service calls are forbidden.** Where the new flow needs cross-service logic (e.g., `AcceptEventInvite` needs registration + community-join + RSVP), the implementation must either live in *one* service or factor the shared logic into a library.
- Concretely:
  - `AcceptEventInvite(short_code, rsvp_state)` lives in `server/services/community/` next to `AcceptInvitationLink` (it reuses that logic and adds an RSVP write). Community service depends on storage and the impact estimator; it does not depend on the experience service. RSVP writes go through the existing `models.Rsvp` storage path.
  - `CanRSVP(short_code)` also in `server/services/community/`. Pure storage read of `ShareLink` + `CommunityUser` count + (existence-check on) `Rsvp`.
  - `RecordWebDecline(short_code)` lives in `server/services/web/` (web-only, not RPC). Simple counter increment on the `Experience` row (or sibling table).
  - `CreateEventShareLink(experience_id)` in `server/services/community/` alongside `GetOrCreateInviteLink`. Both produce `ShareLink` rows.
- New files split per the < 1000-line-per-file convention. Expected new files: `server/services/community/share_links.go` (CreateEventShareLink, CanRSVP, AcceptEventInvite), `server/services/community/share_links_test.go`, `server/services/web/event_page.go`, `server/services/web/event_page_test.go`, `server/services/web/web_decline.go`.

### G. Storage queries: indexed, batched, LIMIT'd

Per `CLAUDE.md` "SQL Efficiency":
- `ShareLink.short_code` gets a **unique index** (lookup hot-path); `ShareLink.community_id` gets a non-unique index (for "list a community's share links"); `ShareLink.event_id` (when populated) gets an index for "find the canonical share link for an event."
- The SSR event landing render must avoid N+1: fetch event, community, host, RSVP list, hero media — preferably via batch APIs (`GetByIDs`) or JOINs. Add a `storage.AssertMaxQueries(t, ctx, max, fn)` test for the landing-render path. Target: ≤5 queries per landing render.
- Capacity check (`CanRSVP`) uses a single `COUNT(*)` against `CommunityUser` filtered by `community_id` — not a full list-then-len.
- "List my RSVPed events" (if added to the post-RSVP confirm screen) uses an indexed `Rsvp.user_id` lookup, paginated via `LIMIT`.

### H. Proto conventions

Per `docs/proto_conventions.md` and `CLAUDE.md`:
- Each new RPC gets its own dedicated request/response message types with inline fields (no reuse).
- The `ShareLink` is a *storage model* in `proto/ripls/models/`. The API surface uses its own flattened message types in `proto/ripls/api/community_service.proto` — conversion lives in the community service's `gen.go` / `convert.go`.
- Optional fields use the `optional` keyword (Go: pointer types, Dart: `has*()` checks). For example, `EventShareLink.completed_summary` would be `optional string`.
- Field comments describe semantics for downstream clients; no vendor names ("Firebase," "Twilio," "Mapbox"), no transport plumbing, no server-side mechanics — those go in this plan and the READMEs.

### I. Testing layers

Per `docs/client/testing.md` and `docs/server/architecture.md`:
- **Server unit tests:** Each new function (storage helpers, conversion functions, validation) gets a `*_test.go` next to the implementation. Mock external dependencies (Firebase Auth, Twilio) via interfaces.
- **Server integration tests:** `AcceptEventInvite` end-to-end (User created → CommunityUser created → Rsvp created in one transaction; rollback on failure) lands in `server/main_integration_test.go` with `storage.SetupTestStorage(t)` (real Postgres via testcontainers).
- **SSR golden-file tests:** `server/services/web/event_page_test.go` covers variants: normal, at-capacity, missing hero, very-long name, RSVPed returning user (with session cookie).
- **Flutter ViewModel tests:** Each new ViewModel gets a `*_view_model_test.go` under `app/test/presentation/viewmodels/` using `ProviderContainer` + Mockito-generated repository mocks. Add `@GenerateMocks` annotations. Run `flutter pub run build_runner build` after.
- **Flutter repository tests:** New `EventShareRepository` gets cache-hit/cache-miss/invalidation tests under `app/test/data/repositories/`.
- **No new client-side integration tests** — Patrol was removed (#1887). Cross-layer coverage is server-side.
- **All Flutter tests use `--reporter expanded`** to avoid CR-overflow output (npm script convention).

### J. Firebase Auth Web requires reCAPTCHA Enterprise (phone path only)

This is a real gotcha worth flagging. Firebase Auth's phone provider on web requires either:
- **invisible reCAPTCHA v2** (legacy, works but deprecated by Google), or
- **reCAPTCHA Enterprise** (current recommendation, requires GCP setup).

Sub-issue 3 must include reCAPTCHA Enterprise setup as a task. The mobile-app Firebase config is not enough; the web app needs additional `firebase-auth` web SDK initialization and a reCAPTCHA Enterprise site key. Document the gotcha in the sub-issue so it isn't a surprise mid-implementation.

**This only applies to the phone path.** The email fallback uses Ripls' existing in-house email registration (server-issued one-time codes via Mailgun), which doesn't route through Firebase Auth and doesn't require reCAPTCHA. The web flow reuses that existing path verbatim.

### K. Flutter Web renderer (revised 2026-05-21)

**Originally:** prefer HTML renderer for the guest bundle to hit a ≤1 MB gzipped target via `flutter build web --web-renderer=html`.

**Revised:** the HTML renderer was removed in Flutter 3.27+, and this project is on Flutter 3.41.6. Only CanvasKit (JS) and CanvasKit-on-WASM (`--wasm`) remain — the `--web-renderer` flag itself is gone. The realistic bundle floor is ~1.5-2 MB gzipped for a minimal guest app, not 1 MB.

The bundle-size budget therefore becomes **measure baseline + ~10% headroom** rather than a hard ≤1 MB cap. CI catches regressions; absolute floor is whatever a minimal `flutter build web` produces. Deferred imports remain the primary lever for keeping the guest path small once #2050's screens land.

This work moved from sub-issue 2 (#2049) to sub-issue 3 (#2050) as part of the same 2026-05-21 revision — the Flutter Web foundation is most useful when built alongside the screens that consume it.

### L. Breadcrumb cookie for cross-surface SSR-to-app handoff (revised 2026-05-21)

**Originally:** an HTTP-only signed-JWT session cookie issued by a new `EstablishWebSession` RPC, verified by a new Connect interceptor that accepted either `Authorization: Bearer` or the cookie. Sub-issue 3 would set the cookie on OTP success; sub-issue 4 would consume it for SSR personalization.

**Revised (per sub-issue 3 / #2050 D3):** dropped entirely in favor of a **content-free breadcrumb cookie**.

- Flutter Web sets `ripls_visited=1; Max-Age=31536000; SameSite=Lax; Secure; Path=/` on first successful token return. No JWT, no signing, no server knowledge of the cookie value.
- The `/go/{code}` handler inspects only the cookie's **presence**. If present and the share-link target is an event, the handler **302s to `/app/event/{experience_id}?code={short_code}`** before rendering SSR HTML. The Flutter Web app then loads, reads its JWT from localStorage, and renders the (full, personalized) event view using `ExperienceContent` — the same widget tree the mobile app uses.
- Stale-cookie failure mode: Flutter Web detects no JWT on a breadcrumb-redirect load, clears `ripls_visited`, and bounces back to `/go/{code}` for the standard SSR landing path. One-time cleanup; no loop.
- The Flutter Web app sends `Authorization: Bearer <jwt>` for all RPCs, exactly like the mobile app. No Connect-interceptor changes.

**Why the revision:** the original cookie's only consumer was SSR personalization. Redirecting authenticated visitors into the Flutter Web app collapses "render personalized SSR" into "let the app render itself," which it already does well using the same `ExperienceContent` widget tree as mobile (see [[2050-web-rsvp-actions]] D5). Server-implementation cost drops to one `if cookie != "" { redirect }` line. Privacy footprint shrinks from "identity token" to "presence bit." Sub-issue 4's scope shrinks accordingly (host views + at-capacity polish; no cookie or interceptor work).

**Where it lives:** the breadcrumb (Flutter Web sets it; `/go/{code}` honors it) ships in #2050 Phase 2.

## Comparison with Incumbents

The May 19 direction asked us to think broadly against best-in-class. The relevant comparisons:

### Partiful (closest analog)
- **Identity:** phone-only. SMS OTP is the entire auth surface. No password, no profile setup beyond a name.
- **Guest persistence:** phone is the key; one number = one identity across all events on the platform.
- **Visibility:** full attendee list (first + last name) on the public event page, plus photos for those who upload them.
- **Plan/dish-claiming:** Partiful has a similar feature ("Bring a Dish"), visible on the event page, claimable from the same surface as RSVP.
- **What Ripls does differently:** Phone OTP + email fallback (Ripls accommodates older / phone-shy users). RSVPing joins a bounded community (Partiful has no concept of persistent membership). Names limited to first-only (vs Partiful's first+last).

### Evite
- **Identity:** email magic link.
- **Guest persistence:** per-event email; no cross-event identity.
- **Visibility:** RSVP list visible to invited recipients only (requires the link).
- **What Ripls does differently:** Phone-first (mobile-native, faster, no inbox checking). Single identity that persists across events. Joining a community is a real artifact.

### Meetup
- **Identity:** required full account creation.
- **Visibility:** events are *public* and discoverable.
- **What Ripls does differently:** Invite-only, no discoverability — the shortcode is the only path in. Aligned with the priorities-doc constraint against stranger discovery.

### GoFundMe
- **Identity:** anonymous contributions allowed; phone/email collected for receipts.
- **Visibility:** fully public, performative giving with public donor list.
- **What Ripls does differently:** No public discovery, no public donor lists, no performative giving. (Payments out of scope for v1 anyway.)

### Mealtrain
- **Identity:** email-only, per-event.
- **Visibility:** signers see other signers; no cross-event identity.
- **What Ripls does differently:** Phone-first and identity persists. Mealtrain's narrow scope (meals only) collapses into Ripls' more general Plan primitive.

The cleanest pull from this set: **Partiful's phone-keyed identity model** with a few Ripls-specific adaptations — email fallback, bounded community membership instead of open RSVPs, first-name-only display, and integration with the existing trust-circle data model.

## Implementation Plan

Five sub-issues, each independently shippable. Sub-issues 1–3 deliver the v1-visible feature (a non-member can RSVP to an event via the web). Sub-issues 4–5 polish the host side and harden against abuse.

### Sub-issue 1: Infrastructure — ShareLink generalization

**Goal:** Data layer ready. No user-visible change yet.

**Tasks:**
- Create `ShareLink` proto model with the `oneof target { community_invite_id, event_id, ... }` shape from Decision 4. Storage model in `proto/ripls/models/share_link.proto`. Per **Compliance H**, API messages stay separate and dedicated per RPC.
- Add storage indexes: unique on `short_code`, non-unique on `community_id`, non-unique on `event_id` (per **Compliance G**).
- Migrate existing `CommunityInvitationLink` rows to `ShareLink` with `community_invite_id` set. The migration is idempotent and runs in a single transaction.
- Update existing RPCs (`GetOrCreateInviteLink`, `AcceptInvitationLink`, `RevokeInviteLink`, `CheckInvitation`) to read/write `ShareLink` underneath; preserve their proto contracts and observable behavior. Each handler updated per **Compliance D** logging conventions and **Compliance E** error wrapping.
- Add a new `CreateEventShareLink` RPC in `server/services/community/share_links.go` (per **Compliance F** file split) that, given an `experience_id`, returns a `ShareLink` with `event_id` populated. Dedicated request/response types per **Compliance H**.
- No new registration RPC. The existing registration + invite-acceptance flow already produces `User` + `CommunityUser` atomically and is the only path that should create users.
- **Native-app deep-link UI for event share links:** when an Android or iOS device with the app installed follows `/go/{code}` to an event-flavored ShareLink, the native app currently routes through the existing AcceptInvitationLink path with minimal event context. This sub-issue adds an event-aware deep-link landing screen in the native app: shows the event hero, time, place, host, hosting community, and a one-tap RSVP CTA. Existing community-only deep-link UI stays unchanged. Same Flutter widgets as the post-RSVP Plan-claim screen from sub-issue 3 to keep the codebase tight.
- Unit tests next to each new file; integration test in `server/main_integration_test.go` exercising the migration on a populated test database (per **Compliance I**); regression test verifying existing community-invite deep links still route correctly.

**Validates:** Data model supports the new shapes; existing community-invite behavior unchanged in production.

### Sub-issue 2: SSR event landing (read-only) — #2049

**Goal:** Anyone with an event-flavored shortcode can view the SSR landing. No actions yet.

**Rescoped 2026-05-21:** the Flutter Web build pipeline (originally part of this sub-issue) moved into sub-issue 3 (#2050). With no Flutter Web screens to consume the foundation here, Phase 2 ships scaffolding only — most useful built alongside #2050's actual screens. This sub-issue is now purely SSR.

**Tasks:**
- Add an `event.html` template under `server/services/web/templates/` rendering the SSR landing: hero image, name, time, place, host, community name, attendee list (first name + profile photo), three CTA buttons (Yes/Maybe/No initially as static links, wired in sub-issue 3). Semantic HTML per **Compliance C**: `<main>`, `<button>`, `aria-label` on icon controls, keyboard-navigable order.
- Extend the `/go/{code}` router in `server/services/web/service.go` to branch on which `oneof` variant the looked-up `ShareLink` row has set: `community_invite_id` → existing `invite.html`, `event_id` → new `event.html`. Logging follows **Compliance D**: mask the short code, structured fields for `community_id`, `experience_id`, `operation`.
- Avoid N+1 on landing render per **Compliance G**: batch-fetch event + community + host + media + RSVP list via `GetByIDs`. Add `storage.AssertMaxQueries(t, ctx, 7, fn)` to the landing test (budget includes Location + member-count COUNT).
- Add OG metadata (title, description, image) to the event landing for link-preview consumers. Cache the OG image presigned URL for the duration of `ogImageDuration` (existing constant in `web/service.go`).
- Add golden-file tests in `server/services/web/event_page_test.go` per **Compliance I**: normal render, at-capacity variant, missing hero, very-long event name, very-long host name. Mark v1 strings English-only per **Compliance B** with a follow-up issue for Go-side i18n.
- Bring `invite.html` / `deeplink.css` onto the same shared design tokens as the event landing so both surfaces feel native to the app (coral logo stays as the brand mark; interactive surfaces move to sage).

**Validates:** End-to-end SSR landing renders for an event shortcode; existing community-invite landing visually aligned; OG previews work in Signal / iMessage / Slack.

### Sub-issue 3: Core actions — RSVP yes/maybe/no + Plan claim — #2050

> **Revision note (2026-05-21):** the sub-issue 3 task list below is
> retained as a reference to the original intent of this epic. The
> authoritative plan for #2050 — including substantive revisions to
> the RPC shape (no `AcceptEventInvite`; two-step client-side),
> session cookies (replaced by breadcrumb; see Compliance L above),
> Plan-claim screen (reused via `ExperienceContent` extraction from
> the existing mobile event screen), and bot protection (Firebase App
> Check instead of reCAPTCHA Enterprise) — lives at
> [[2050-web-rsvp-actions]]. Read that plan first; refer here only
> for the original framing.


**Goal:** The headline user story works. Someone with the link can RSVP, become a community member, and claim a slot.

**Scope expansion 2026-05-21:** absorbs the Flutter Web build pipeline tasks that were originally in sub-issue 2 (#2049) — the foundation is most useful when built alongside the screens that consume it. See "Flutter Web foundation" task block below.

**Tasks (Flutter Web foundation, moved from sub-issue 2):**
- Set up `flutter build web` in `app/` (Flutter 3.41+ uses default CanvasKit; `--web-renderer` is gone). Document the renderer choice and pinned Flutter version in a new `app/web/README.md`.
- Add a `build:web` npm script that runs `flutter build web --release` and emits `app/build/web/` (or directly into `server/services/web/app_assets/`).
- Add `server/services/web/app_embed.go` with `//go:embed app_assets/*` mirroring `website/embed.go`.
- Add `/app/*` route in `server/main.go` adjacent to the existing `/go/`, `/css/`, `/assets/`, `/.well-known/` routes. Cache headers: long-cache for hashed assets, no-cache for `index.html` and Flutter manifest.
- Wider `Platform.is*` audit: 7 files outside `app/lib/core/theme/` use `Platform.is*` (feedback, FCM, location-picker, deferred deep links, app badge). Add `kIsWeb` guards (or move behind deferred imports) for any that become reachable from a guest entry point.
- Add CI gate `scripts/check_web_bundle_size.sh`: enforces measured baseline + ~10% headroom (not a hard ≤1 MB cap — see revised Compliance K). Wire into `.github/workflows/test_flutter_web.yaml` triggered on changes to `app/lib/**`, `app/web/**`, `pubspec.yaml`, the script itself.
- Configure Flutter Web routing for **deferred routes** — only `event view`, `Firebase Auth`, `RSVP confirm`, `Plan claim` screens (this sub-issue's own work) belong in the initial bundle.

**Tasks (actions — original scope):**
- **"No" path:** clicking No on the SSR landing fires a small inline JS handler that calls a `RecordWebDecline` endpoint (event-scoped anonymous counter, lives in `server/services/web/web_decline.go` per **Compliance F**). Increment is rate-limited (per **Compliance F** sub-issue 5 details). Show a "thanks" confirmation in-place via an `aria-live="polite"` region per **Compliance C**. No page reload.
- **Capacity check:** the SSR landing pre-renders Yes/Maybe enabled/disabled based on community capacity at render time (one extra storage query, cached briefly). Disabled buttons get explanatory `aria-label`. A `CanRSVP(short_code)` RPC also lives in `server/services/community/share_links.go` for the Flutter Web RSVP confirm screen to re-check before submission.
- **"Yes"/"Maybe" path for existing members:** if the session cookie identifies an authenticated `User` who is already a `CommunityUser` of the hosting community, skip OTP — submit RSVP directly via Connect with the cookie-derived auth. See **Compliance L** for cookie semantics.
- **"Yes"/"Maybe" path for non-members:** click handler navigates to `/app/event/{experience_id}?rsvp=yes&code={short_code}`. Flutter Web loads (CanvasKit, bundle measured + ~10% CI cap), reads URL params, and offers the user a phone OTP (primary) or "use email instead" (secondary).
- **Phone path:** Firebase Auth phone provider (web SDK), gated by reCAPTCHA Enterprise per **Compliance J**.
- **Email path:** uses Ripls' existing in-house email registration (server-issued one-time code via Mailgun) — *not* Firebase Auth email link. Reuses the existing `email.Service` plumbing; the Flutter Web screen just wraps the existing email-OTP widgets.
- **reCAPTCHA Enterprise setup** (per **Compliance J**, phone path only): set up a Firebase Auth-compatible reCAPTCHA Enterprise site key in GCP, wire it into the Flutter Web `firebase_auth` initialization, document the rotation procedure. This is a non-trivial subtask — surface as a sub-task on the sub-issue ticket.
- On OTP success: calls a single transactional RPC `AcceptEventInvite(short_code, rsvp_state)` in `server/services/community/share_links.go` per **Compliance F**. The handler wraps the existing registration + `AcceptInvitationLink` logic (creates `User` + `CommunityUser`) and records the `Rsvp` in one DB transaction. Rollback on any step. Logging per **Compliance D**; internal errors via `connecterr.Internal` per **Compliance E**.
- **EstablishWebSession** RPC: on successful auth, server sets the SameSite=Lax HTTPS-only cookie per **Compliance L**. Cookie is verified by the same Connect interceptor that handles `Authorization: Bearer` today.
- **Plan claim after RSVP:** post-RSVP, Flutter Web shows the event's Plan list with current slot counts. User can claim a slot in one tap; calls the existing `ClaimExperienceNeed` RPC. The Plan list view is a ViewModel + Repository + Notifier per **Compliance A**.
- **Flutter ViewModels** created for: event view (read), OTP entry, RSVP confirm, Plan claim. Each uses Riverpod `NotifierProvider.autoDispose` + Freezed state per **Compliance A**. ARB keys added per **Compliance B** to both `app_en.arb` and `app_es.arb`. Tappable / IconAction primitives per **Compliance C**.
- **Tests** (per **Compliance I**): ViewModel tests with mocked repositories; repository cache tests; server integration test for `AcceptEventInvite` covering happy path, capacity-refusal, rollback on RSVP failure, idempotency when re-submitted with same OTP state.

**Validates:** Full guest-to-member journey works on web. Includes the headline acceptance criteria.

### Sub-issue 4: Polish — host views and at-cap UX

**Goal:** Host-side experience handles the new state cleanly.

**Tasks:**
- Surface the web-decline counter in the host's view of an event ("No (web): N") alongside member declines. Flutter UI uses existing `AppColors` + `Tappable` per **Compliance A** and **C**; ARB keys via **Compliance B**.
- Add a "joined via event" badge or visual marker on the host's CommunityUser list for members who joined via an event shortcode rather than a community-invite link. Implementation: extend `CommunityUser` (storage proto) with an optional `source` enum (`SOURCE_DIRECT_INVITE`, `SOURCE_EVENT_INVITE`) and stamp it at join time. `optional` keyword per **Compliance H**.
- "At capacity" SSR variant of the event landing: clear messaging, hidden Yes/Maybe buttons replaced with a "community is full ({N} of {max})" message bar (already landed in #2049 Phase 1); this sub-issue may refine the copy or add a "ask your inviter for options" CTA per host feedback.
- ~~Personalized SSR landing for an authenticated returning user via the session cookie from **Compliance L**~~ **Removed:** authenticated returning visitors are redirected from `/go/{code}` straight into the Flutter Web app via the breadcrumb cookie (see revised **Compliance L** and [[2050-web-rsvp-actions]] D3). SSR personalization is no longer a sub-issue 4 deliverable; the app renders the personalized event view itself.

**Validates:** Host has full awareness of web participation; at-cap edge case is handled. (Returning-user personalization is now handled by the Flutter Web app, not SSR.)

### Sub-issue 5: Hardening — rate limits and schema-prep approval queue

**Goal:** Production-grade anti-abuse posture; schema groundwork for organizer-approval UI in a future issue.

**Tasks:**
- Add rate limits on the OTP-request endpoint: 5/hour/IP, 3/hour/phone. Use existing rate-limiting middleware. Rate-limit violations log as `Warn`, not `Error`, per **Compliance D** (they're expected user errors that would otherwise pollute alerts).
- Add rate limit on `RecordWebDecline`: 30/hour/IP to keep the anonymous counter from being trivially polluted.
- Add `Experience.guest_approval_required` `optional bool` (default false) and `GuestApprovalRequest { id, experience_id, candidate_user_id, requested_at_unix_sec, decided_at_unix_sec, decision }` storage model. Per **Compliance H**: `optional` for the bool, indexed `experience_id`, dedicated API request/response types if/when the UI ships.
- Wire `AcceptEventInvite` to short-circuit into "pending approval" state when this flag is set — but ship the toggle default off and no host UI for it.
- Document the schema-prepped surface in `docs/invitations.md` so the future issue picks it up.
- Add an integration test for the rate-limit paths (per **Compliance I**) — verifies OTP rate-limit, decline rate-limit, capacity refusal, and approval-pending path each return the right Connect codes with non-leaky error messages per **Compliance E**.

**Validates:** Production-safe to ship. Approval-queue UI is one small follow-up away.

## Decisions made during planning

The following questions were raised in this plan and resolved interactively:

- **Email fallback uses Ripls' existing in-house email registration**, not Firebase Auth email link. The existing path issues a one-time code via Mailgun and creates the `User` + `CommunityUser` through the same registration flow the mobile app uses today. Phone and email are equivalent identity primaries — whichever the user verifies creates the canonical User. (This narrows the Firebase-Auth-Web scope in sub-issue 3 to phone OTP only.)
- **"No" RSVPers see nothing different on a second visit.** Declining is meant to feel disposable. A cookie can suppress the CTAs on revisit, but the page must never surface "you declined" language.
- **Universal Link / App Link on Android with the app installed needs a nice in-app UI treatment**, not just a regression test. Today the app handles deep-link arrivals via the existing AcceptInvitationLink path with minimal UX. Sub-issue 1 includes UI work for an event-flavored deep-link landing: clean event-context-aware screen showing where the link is taking the user, the event details, and a one-tap RSVP. The new ShareLink table must also continue to support the existing community-only deep-link path unchanged.
- **CI gate enforces a Flutter Web bundle-size budget** — measure baseline + ~10% headroom (Flutter 3.41 has no HTML renderer; ≤1 MB cap from the original plan is no longer achievable). Now a sub-issue 3 task; moved out of sub-issue 2 on 2026-05-21.
- **SSR template test strategy:** golden-file tests in `server/services/web/event_page_test.go`. Variants: normal, at-capacity, missing hero image, very-long event name, very-long host name, RSVPed returning user.
- **SSR landing cache TTL:** short (30 seconds). Acceptable staleness for the rare event-cancellation case; avoids ETag complexity. Sub-issue 2 task.

## Acceptance Criteria (v1)

- A browser hitting `/go/{code}` where the code's `target_kind=EVENT` sees an SSR-rendered landing page in <500ms TTI showing event details and three CTAs.
- The landing page renders correctly when shared in Signal, iMessage, Slack, and Twitter/X (OG metadata).
- Tapping "No" shows a confirmation and increments an anonymous server-side decline counter visible to the host.
- Tapping "Yes" or "Maybe" when the community is at capacity shows an "at capacity" message and does not initiate OTP.
- Tapping "Yes" or "Maybe" when capacity is available initiates a Flutter-Web Firebase phone OTP flow; on success, creates a `User`, a `CommunityUser`, and an `Rsvp` in one atomic operation.
- After RSVP, the user can claim a slot from the event's Plan list via the same Flutter Web session.
- The host, in the mobile app, sees the new member listed with first name and profile photo; sees the web-decline counter; sees no other distinguishing information unless we add the optional source badge.
- Existing community-invite shortcodes (those with `community_invite_id` set on `ShareLink`) continue to work exactly as today.
- The mobile app, when it receives the same `/go/{code}` URL via Universal/App Link, handles it via the existing AcceptInvitationLink path enhanced to also record the event RSVP.
- A user who later installs the app and logs in with the same phone picks up their existing User, CommunityUser, and RSVPs without any "claim history" UI.

## Follow-ups (not in this issue)

- **File issue:** Mobile app's `AcceptInvitationLink` UX should explicitly say "by accepting, you'll join Two Oaks Block" — keep web + mobile flows consistent. Discussed in the planning conversation and explicitly deferred from #1195's scope.
- **File issue:** Web-RSVPed user installs the app — design the post-install onboarding for a user who joined via an event RSVP rather than a community-invite link. The current onboarding assumes "no communities yet" or "invite code in hand."

## Related Issues

- **#757** — Login friction on existing users. Complementary: this issue eliminates login friction for *invitees*; #757 reduces login friction for *existing users*.
- **#1067** — Community-wide chat. Complementary: gives existing members a place to respond; this issue gives invitees a place to act.
- **Branch `improve_invitations`** — `/go/{shortcode}` routing, short-code generation, and existing CommunityInvitationLink infrastructure that this builds on. Already shipped.

## References

- `docs/users/growth-use-cases.md` — strategic rationale; this issue directly enables use cases 2 (Potluck/Event Host), 3 (Standing Thing Founder), 4 (Ride/Trip Sharer), and 7 (Crisis Coordinator) for the non-member-invitee surface.
- `docs/users/priorities.md` — the constraints (32-cap, invite-only, no stranger discovery, no public feeds, no ledgers) that this issue preserves while extending the participation surface.
- `docs/users/personas.md` — Betty, Emma, Henry, Ian — the personas whose growth loops are blocked by the existing "every participant must be a registered member" assumption.
- `docs/invitations.md` — the existing shortcode + AcceptInvitationLink architecture this generalizes.
