# Issue #2636: Notification doesn't take me to the relevant event

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2636
**Filed:** 2026-07-04 | **Last activity:** 2026-07-04
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/push_notifications.md`, `docs/client/deep_linking.md`,
`docs/realtime_updates.md`, `docs/server/l10n.md` (RSVP copy),
`docs/client/architecture.md` (anchor), `docs/client/inbox.md` (Home "Needs
you"), `docs/proto_conventions.md` (anchor)

> **This is a recurring bug.** Prior fixes: #1036 (Apr 2026, "deep links
> failing after background/cold start") and #2117 (closed, "scheduled reminder
> notifications don't deep link correctly"). Both had the *same* symptom the
> user reports here — the tap doesn't route to the item, the app just opens to
> wherever it would open normally. The user's note — *"I swear we've fixed this
> before but it keeps popping up"* — is accurate.

## Problem Statement

Leslie (event host) received a push: **"RSVP to <event>" / "Lisa is attending
your event."** Tapping it dropped her on the **Home tab** ("things that need
your attention"), not on the event, with no path to "what did Lisa say about my
event?" Her explicit fallback ask: *"if you're going to send me here, at least
put a dot on the relevant item!"*

Device: Android 16, Pixel 9 Pro, app **0.26.0**, PROD.

### What the investigation established (with evidence)

I traced the full path and **ruled out the obvious layers by direct test**, not
by reading alone:

1. **Server payload is correct.** An RSVP publishes a `CommunityEvent` with
   `Topic = ExperienceId` (`server/services/experience/events.go:60`).
   `buildNotification` (`copy.go:211`) sets `payload.ExperienceId`, and
   `buildDataPayload` (`fcm/provider.go:317`) puts `experience_id` into the FCM
   `data` map. **Verified in code + covered by `fcm/provider_test.go` + prod
   Cloud Logging** shows real `"notification dispatched to user"` rows with
   title `"RSVP to <event>"`. So the tap *should* carry `experience_id`.

2. **Client route resolution is correct.** `routeNotificationPayload`
   (`fcm_service.dart:548`) maps `{experience_id: X}` → `/experience/X`.
   **Thoroughly unit-tested** (`fcm_navigation_test.dart`, incl. the RSVP shape).

3. **The GoRouter cold-start timing is NOT the bug.** The prime suspect was: the
   app renders a **non-router `MaterialApp`** during splash and only swaps to
   `MaterialApp.router` at `ready` (`main.dart:730` vs `:744`), so a
   `router.go('/experience/X')` fired from `getInitialMessage()` during startup
   hits a detached router. **I wrote two throwaway widget-test reproductions**
   that mirror this exactly — (a) `go()` before `MaterialApp.router` mounts, and
   (b) `go()` during `authState.isLoading` with the real auth-gated `redirect` +
   `refreshListenable` churn. **Both passed** — the deep link survived to
   `/experience/X` in every case. The naive race does not reproduce.

4. **No screen-level bounce.** `ExperienceScreen` does not auto-`go('/')` on load
   failure; nothing navigates to `/` on the splash→ready transition or in
   `HomeScreen.initState`. The removed `_switchToCommunity` helper (#2023) was a
   legitimate no-op deletion after #1895 dropped the per-community filter — not
   the cause.

### The real finding: this bug is *un-observable*, which is why it recurs

The entire client deep-link path is instrumented with **`_log.info` only**.
Per `observability/logging/logger.dart`, only **WARNING+** becomes a Crashlytics
breadcrumb, and **breadcrumbs only surface on a crash**. A lost deep-link is not
a crash. There is **no analytics event** on notification tap (grep of
`observability/events.dart` — none). **Net: when a deep link is dropped in prod,
nothing is recorded anywhere queryable.** Every recurrence is diagnosed by
guesswork, each fix is a hypothesis, and the bug returns. *That* is the root
cause of the recurrence, distinct from the root cause of any single drop.

The residual place the actual drop lives — the only layer left after 1–4 —
is the **FCM/platform boundary on Android background/terminated taps**:
`getInitialMessage()` returning null or a data-less message, `onMessageOpenedApp`
not firing, or OEM/battery process-kill quirks. This is a well-known
`firebase_messaging` Android failure class, it is **intermittent** (matches
"keeps popping up"), and **it cannot be reproduced in a Dart test** (matches
"very difficult to test") — which is exactly why instrumentation, not another
blind code fix, is the correct first move.

There is also one latent **fragility** worth fixing defensively: `onNavigate` is
a mutable nullable field on the `FCMService` instance, wired imperatively from a
single call site (`_initializeFCM`). `fcmServiceProvider` `ref.watch`es
`transportProvider`/`rpcErrorHandlerProvider`. Those happen to be stable today,
so the instance isn't silently rebuilt — but if any dependency ever gains a
reactive input, Riverpod would hand back a fresh `FCMService` with
`onNavigate == null` and every deep link would silently break until the next
`_initializeFCM`. The design invites exactly the "silent null callback" failure
we're chasing.

## Context from Comments

**There are no comments on the issue.** All context is the original report + the
screenshot (Home / "things that need your attention"). No maintainer discussion,
no repro steps beyond the one event. This is a single-user field report — the
plan leans on code tracing, prod logs, and the two prior issues (#1036, #2117)
rather than issue discussion.

## Potential Solutions

### Option A: Instrument the deep-link path end-to-end, then fix from data (RECOMMENDED)
- **Approach:** Emit one structured, **queryable** telemetry event at each
  decisive hop of the tap→navigate pipeline, plus promote the "dropped" branches
  to WARNING (so they also land as breadcrumbs):
  - **tap ingress** (`_onMessageOpenedApp`, `getInitialMessage`,
    `checkForMissedNotification`, local-notification tap): `source`
    (`initial|opened|missed|local`), `message_id`, `has_data`, `data_keys`.
  - **route decision** (`routeNotificationPayload`): resolved entity kind +
    `resolved_route`, or the `no_entity_id` fallback.
  - **navigation execution** (`onNavigate` in `main.dart`): `on_navigate_null`
    (bool), `route`, and the **actual router location a frame later** (did
    `go()` stick?).
  - Fire via `observabilityService.logEvent('notification_deeplink', …)`
    (Firebase Analytics → DebugView + BigQuery, queryable). This inherits the
    existing **consent seam** for free — `ObservabilityService` swaps in a no-op
    analytics provider when consent is not granted (`service.dart`), so no extra
    gating is needed. Run all event params through `LogRedactor.redactMap`
    (`observability/logging/redactor.dart`) and carry **entity IDs only** — no
    titles, names, or bodies — satisfying the observability rubric's "no PII"
    rule (§1).
- **Pros:** Converts an un-testable, invisible failure into a self-diagnosing
  one. The next recurrence pinpoints the exact failing layer in one query.
  Directly answers the user's "hard to test." Low risk (adds logging).
- **Cons:** Doesn't *fix* the drop by itself — it makes the fix knowable. One
  more field report needed to close the loop (but see Option B, which ships
  value immediately and independently).
- **Effort:** Small.

### Option B: Graceful degradation — "put a dot on the relevant item" (RECOMMENDED, ships with A)
- **Approach:** Persist the pending notification target (entity id + kind) when a
  tap is received, and if navigation does **not** land the user on that item
  within a short window, **surface it on Home**: highlight/badge the event in
  "Needs you" / "Up next", or show a lightweight "From your notification →
  <event>" affordance at the top of Home. This is precisely the user's own
  suggestion and it helps **regardless of which platform-layer bug is active.**
- **Pros:** User-visible win now; robust to root-cause uncertainty; turns a
  dead-end into a one-tap recovery. Reuses the existing Home inbox surfaces
  (`docs/client/inbox.md`).
- **Cons:** UX design needed for the badge/affordance; must avoid a stale dot if
  navigation *did* succeed (needs the "did we land there?" signal from A).
- **Effort:** Medium.

### Option C: Harden the fragile navigation seams defensively
- **Approach:** (1) Resolve the router at **call time from `ref`** instead of
  storing a nullable `onNavigate` field — or make `fcmServiceProvider`
  `keepAlive` and re-wire `onNavigate` whenever the instance is (re)built — so a
  provider rebuild can never leave a null callback. (2) Adopt a **pending-route
  holder** applied once `SplashLoadingStep.ready` (mirroring
  `DeferredDeepLinkContextHolder`) so navigation is never fired into a
  transitional/detached router. (3) Consume `getInitialMessage()` exactly once
  across `initialize()` and the resume `checkForMissedNotification()`.
- **Pros:** Removes latent "silent null callback" and transitional-router
  classes even though neither reproduced today; cheap insurance.
- **Cons:** Refactors a delicate area; without A's telemetry we can't *prove* it
  fixed the field bug (risk of another "fixed?—it came back").
- **Effort:** Small–Medium.

### Option D (rejected as a standalone): Blind re-fix of the cold-start race
- Rejected because two reproductions show the timing race doesn't reproduce, and
  #1036/#2117 already tried code-only fixes here. Another blind fix is how this
  became a recurring issue. Any hardening (Option C) ships **with** instrumentation.

## Recommendation

**Ship A + B together, then C as follow-on hardening.** Sequencing:

1. **A (instrumentation) first and immediately** — it's the only thing that
   makes the true drop knowable and stops the guess-fix-recur cycle. It is the
   direct answer to "how do we test a fix to this."
2. **B (graceful degradation) in the same release** — delivers the user's
   requested outcome and de-risks the whole issue: even if we never catch the
   exact platform drop, the user recovers in one tap.
3. **C (hardening)** — land the cheap defensive parts (pending-route holder,
   non-nullable router resolution) alongside, and let A's data confirm the drop
   is gone before we call #2636 closed.

Rationale: the honest "source" here is two-layered — the *drop* is at the
un-instrumented Android FCM boundary (can't be pinned from code alone), and the
*recurrence* is caused by having no prod signal. Fix the recurrence cause
(observability) and the user-facing pain (fallback UX) now; use the resulting
data to close out the specific drop with confidence rather than a third guess.

## Open Questions

- **Is the drop Android-terminated, Android-background, or both?**
  — Recommendation: don't guess — A's `source` field answers it on first
  recurrence. In parallel, run the device matrix (Phase 3) to try to force it.
- **Do we have Firebase Analytics → BigQuery export wired for prod?** *Still
  unconfirmed from the repo* (it's a Firebase console toggle, not in Terraform).
  This matters: DebugView is dev-device-only, and WARNING breadcrumbs only
  surface on a crash — so without the BigQuery export, `notification_deeplink`
  events are only *aggregate*-visible in the console (and only if the params are
  registered as custom dimensions). **Mitigation shipped:** a permanent drop now
  fires a Crashlytics **non-fatal**, which is per-incident visible from prod
  devices regardless of the export. **Follow-ups:** (1) confirm/enable the prod
  BigQuery Analytics export; (2) register `source`/`trigger`/`recovered`/
  `landed`/`entity` as custom dimensions so the non-BQ console path can slice.
- **What exactly should the Home "dot" look like?** — Recommendation: reuse an
  existing "Needs you" row highlight for the target event rather than inventing a
  new component; only show it when A's post-nav signal says we did *not* land on
  the item. Keep it dismiss-on-view.
- **Should `EXPERIENCE_RSVP_*` also deep-link to the Chat tab** so "what did Lisa
  say" lands on discussion? — Recommendation: no — an RSVP is not a chat message;
  land on the event (Event tab) where the roster/RSVP is visible. Chat-tab
  routing stays reserved for `conversation_id` payloads.
- **iOS parity:** the report is Android, but the seam is shared. —
  Recommendation: instrument once (cross-platform) and include iOS in the device
  matrix; don't scope the fix Android-only.

## Implementation Plan

### Phase 1: Make it diagnosable (instrumentation) — ✅ SHIPPED
**Goal:** Any future drop is captured with enough detail to name the failing
layer, with no device or repro needed.
**Tasks:**
- [x] Add a `notification_deeplink` analytics event (typed
      `NotificationDeepLinkEvent`, via the observability service's
      `logAnalyticsEvent`) at the route decision (`routeNotificationPayload`,
      `phase: route`, folding tap-ingress in via a `source` field) and at
      navigation execution (`main.dart` `onNavigate`, `phase: nav_result`).
- [x] Promote the drop branches (`on_navigate_null`, `no_entity_id`,
      empty-data, nav-didn't-stick) to **WARNING-level** logs so they also land
      as Crashlytics breadcrumbs.
- [x] Capture the **post-`go()` router location** one frame later in
      `main.dart`'s `onNavigate` (`addPostFrameCallback`) with a `landed` flag.
- [x] Redact per privacy rules; entity IDs only, `data_keys` are keys-only.
      Sink inherits the observability consent no-op.
- [x] Unit tests: `fcm_navigation_test.dart` (telemetry group) +
      `events_test.dart` (event serialization). All green; `flutter analyze`
      clean on touched files.
**Validates:** On the next field report we can query DebugView/BigQuery (or read
the breadcrumb) and see the exact hop that failed. Confirms/*refutes* the
Android-platform-boundary hypothesis with data.
**Shipped in:** commit `9a742cb29` on `event_bug`. Phases 2 (graceful
degradation + integration harness) and 3 (hardening + device matrix) remain.

### Phase 2: Graceful degradation + deterministic client test — ⛔ DESCOPED (2026-07-22)
**Decision:** the graceful-degradation Home affordance (the "put a dot on the
item" UX) is **not** being built. Recovery is handled instead by **route replay**
in Phase 3 (drain the captured route onto the router, retry once), and when even
the replay can't land, the drop is **reported only** (WARNING + `recovered:false`
telemetry), not surfaced on Home. Rationale: replay heals the client-side drop
classes directly and needs no new UI; the platform-layer drop can't be recovered
client-side regardless, so a Home affordance would add surface without closing
the real gap. Deterministic client coverage ships as
`app/test/services/notification_route_replayer_test.dart` (a real `GoRouter`
driving the holder → `go()` → verify → replay path) rather than a full-`GearApp`
splash harness; the latter remains a possible follow-up but the recover path is
deterministically covered. Original plan retained below for provenance.

**Goal:** User always has a one-tap path to the item; regression-guard the
client integration deterministically.
**Tasks:**
- [ ] Persist the pending notification target on tap; if navigation didn't land
      on it, surface a highlighted "Needs you"/"Up next" row (or top-of-Home
      affordance) for that event. Dismiss on view; suppress if we did land there.
      - **l10n render boundary:** the affordance is client-rendered in-app, so
        any copy comes from `AppLocalizations` (client catalog), never from the
        server push copy — consistent with the render-boundary rule.
      - **Accessibility:** the new affordance is interactive, so build it on the
        `Tappable` primitive (`presentation/widgets/accessibility/tappable.dart`)
        with an l10n-sourced semantic label and a ≥48dp touch target, per the
        accessibility rubric — don't hand-roll a bare `GestureDetector`.
- [ ] Build a widget/integration test that pumps the real `GearApp` with
      `fcmServiceProvider` overridden to inject a fake `FirebaseMessaging`
      returning an RSVP `RemoteMessage` from `getInitialMessage()`; drive the
      true splash→ready sequence; assert final route `/experience/<id>` (and not
      `/`). Parameterize over `source` (initial/opened/missed) and payload kinds
      (RSVP experience / gear / request / chat-tab / community-only).
      *(The two throwaway repros in this investigation prove the harness shape;
      generalize them against the real widget + real `routerProvider`.)*
- [ ] Run the new Flutter test locally with `--reporter expanded` (the compact
      reporter duplicates lines via `\r`); CI uses `--reporter github`.
**Validates:** The user's requested outcome ships; the client integration is now
covered by a deterministic test that would catch a real-app wiring regression.

### Phase 3: Hardening + recovery + device matrix — ✅ CODE SHIPPED (2026-07-22)
**Goal:** Remove the fragile seams, recover client-side drops, and exercise the
actual platform boundary.
**Tasks:**
- [x] **Catch/hardening:** `PendingNotificationRouteHolder` captures the resolved
      route from FCMService's routing path independent of the `onNavigate`
      field (`fcm_service.dart`), so an unwired callback or not-yet-mounted
      router defers rather than drops. `fcmServiceProvider` is `keepAlive` so the
      service/callback is never rebuilt. `checkForMissedNotification()`
      serializes behind the startup `getInitialMessage()` consume
      (single-consume, no startup/resume double-handle).
- [x] **Recover:** `NotificationRouteReplayer`
      (`notification_route_replayer.dart`) is the single executor — drains the
      holder from the live-tap poke, splash→ready, and resume; `go()` + verify a
      frame later + one timer-based replay; `recovered:true|false` telemetry.
      **Replay-only, no fallback UI** (per 2026-07-22 decision).
- [x] **Report:** `NotificationDeepLinkEvent` extended with `trigger` +
      `recovered` so a recovered replay is distinguishable from a permanent drop.
      A **permanent** drop also fires a **Crashlytics non-fatal**
      (`NotificationDeepLinkDroppedException`) so it's visible per-incident from
      prod user devices — not just DebugView (dev-only) or the BigQuery export
      (see follow-ups) — and can drive the alert→issue pipeline.
- [ ] **Device matrix (manual, owner-run):** physical Android + iPhone ×
      foreground / background (process alive) / terminated, verifying landing +
      `nav_result`/`recovered` telemetry in Firebase DebugView. Codified as a
      release-gate checklist in `docs/push_notifications.md` → *Step 6: Deep-Link
      Recovery Release Gate*. **Bryan runs this on real hardware** (the Pixel 9
      where it reproduces); emulators don't reflect the OEM process-kill quirk.
- [ ] Once Phase 1/3 telemetry shows zero unrecovered drops for the field
      pattern over a release cycle, close #2636.
**Shipped in:** commits on `event_bug` (holder+replayer, wiring, single-consume,
event fields, docs) — stacked onto the Phase 1 PR **#2753**.
**Validates:** Client-side drops (unwired callback / detached router /
didn't-stick) are recovered and measured; the platform-layer drop is measured
(telemetry) so the residual is knowable rather than guessed.

## Related Issues
- **#1036** (closed) — "Fix notification deep links failing after background/cold
  start." Same symptom class; prior code-only fix. Its `_switchToCommunity`
  addition was later correctly removed by #2023.
- **#2117** (closed) — "Scheduled reminder notifications don't deep link
  correctly." Same symptom ("opens to previous screen"); fixed server-side by
  adding `experience_id` to scheduled payloads. Recurrence here shows the class
  wasn't fully closed.
- **#2657** (closed) — notification-count flake; related notification
  dispatch/timing area (publish-time member snapshot).
- **#2443** (open) — "Fix small issues with requests & experiences (deep links,
  …)"; overlapping deep-link surface, worth cross-checking during Phase 2.

---

## Audit notes (`/ripls-audit`)

**Verdict: compliant.** The plan aligns with the project's documented standards;
audit tightened three convention ties (edits applied in-place).

- **Observability (§7 Client Observability):** the rubric explicitly wants "key
  user actions tracked" and crash-reporting integration — a notification tap is
  an untracked key action today, so Phase 1 is squarely on-rubric. Tightened the
  telemetry spec to (a) inherit the consent no-op seam rather than add gating,
  (b) route params through `LogRedactor.redactMap`, and (c) carry entity IDs only
  (§1 "no PII").
- **Accessibility:** the new Home affordance (Option B) is interactive UI, so the
  plan now mandates the `Tappable` primitive + l10n semantic label + ≥48dp target
  rather than a bare gesture handler.
- **l10n render boundary:** clarified the in-app affordance draws copy from
  `AppLocalizations` (client catalog), not the server push copy.
- **Testing:** Phase 2's deterministic integration harness + retained server
  contract coverage (`fcm/provider_test.go`) satisfy the testing rubric's
  unit + integration expectation; added the `--reporter expanded` local-run note.
- **Architecture/conventions:** no violation of the API/models separation or the
  server↔client render boundary; the fix is client-side with a server-side
  contract assertion only.

**Docs consulted:** `docs/llms.txt`; `docs/reviews/observability_review_prompt.md`,
`docs/reviews/testing_review_prompt.md`, `docs/reviews/accessibility_review_prompt.md`
(Areas to Review); `docs/push_notifications.md`, `docs/client/deep_linking.md`,
`docs/realtime_updates.md`, `docs/client/inbox.md`, `docs/client/architecture.md`
(anchor); verified against `app/lib/core/observability/service.dart`,
`app/lib/core/observability/logging/redactor.dart`, and
`app/lib/presentation/widgets/accessibility/tappable.dart`.
