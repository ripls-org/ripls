# Issue #2109: Didn't receive notification for new request

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2109
**Filed:** 2026-05-22 | **Last activity:** 2026-05-22
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** docs/push_notifications.md

## Problem Statement

Bryan (`user2@example.org`, Pixel 9 Pro, Android 16, app v0.22.0 build 7130, PROD)
reports that he did not receive an expected push notification when someone
posted a new request in a community he is a member of. He also says he did
not receive push notifications for **new offers on events**, which most
naturally maps to either `REQUEST_OFFER_MADE` (the offer-to-fulfill flow on
a request) or `PLANNING_CONTRIBUTION_ADDED` (a contribution to a planned
need on an experience — "Event" in user-facing copy per CLAUDE.md).

Both of these event types are wired through the `community_event_bus` →
`community_subscriber` → `notifications.Service` pipeline and should fire
a push under the default preferences (unset = on). The fact that neither
fired suggests either:

1. A gate in the pipeline (soft-delete, active-stream suppression, category
   preference, missing device, FCM token state) silently suppressed the
   push for this user, OR
2. The event was never published (e.g. the producer path errored before
   `bus.Publish`), OR
3. The push was dispatched and accepted by FCM but never displayed by the
   device (notification permission, DND, app-level blocking, OS battery
   optimisation).

We cannot tell which without per-recipient delivery logs. Bryan himself
flags this: *"We should audit logs to confirm these events are triggering
notifications or perhaps it's delivery difficulties."*

The current code logs at INFO when a push **is** sent
(`subscriber.go:144 "sending notification to user"`) and at DEBUG when it
is **suppressed** by a gate (active stream, category disabled, no recipients
resolved). DEBUG is below the production log level, so suppression
decisions are effectively invisible. There is also no per-(event_id,
user_id) audit row, so reconstructing "what happened to this push"
requires combining several free-text log lines across services with no
correlation key.

## Context from Comments

Single comment from Bryan: `@claude`, which is the trigger token for this
agent. No further diagnostic information was attached — no event ID, no
community ID, no approximate timestamp, no second-user perspective ("the
person who posted the request").

This means the plan cannot assume we can pinpoint *this specific* missed
push. The right first step is to make every future missed push diagnosable
end-to-end, then use that data to triage Bryan's report (and the broader
question of whether others are silently affected).

## Potential Solutions

### Option A: Observability-first — promote suppression logs and add a per-dispatch summary line

- **Approach:** In `server/notifications/community_subscriber/subscriber.go`,
  promote each per-recipient gate decision (`actor_self`, `active_stream`,
  `category_disabled`, `no_devices`, `sent_ok`, `send_failed`) from DEBUG to
  INFO and emit them all on a single structured log line per recipient
  (one record per (community_event_id, user_id) pair). Mirror the same
  per-recipient outcome on the `notifications.Service.NotifyUser` path so
  that a `no_devices` outcome (`service.go:91`, currently DEBUG) becomes
  visible too. Use the standard observability field names defined in
  `docs/server/observability.md` §"Streams and per-event correlation":
  `operation` (`community_notifications.handle`), `community_event_id`
  (the per-event correlation key already used by the community-event
  streaming path — **not** `event_id`, which would collide with chat
  conventions), `event_type`, `community_id`, `user_id`, and a single
  `outcome` enum field. Add the same outcome field to the per-device FCM
  send path so token-level failures (`THIRD_PARTY_AUTH_ERROR`,
  `NotRegistered`) are correlatable back to the triggering event. Phase 2
  follows up with a unit test that fixes the outcomes and a Cloud
  Logging saved query / alert, optionally as a new log-based metric in
  `terraform/modules/monitoring`.
- **Pros:** Smallest blast radius — no schema changes, no new RPCs, no
  client work. Directly answers Bryan's stated request ("audit the logs").
  Future user reports of the same shape become a 30-second triage instead
  of a multi-hour investigation. Lays the groundwork for any subsequent
  targeted fix because the gate that silenced the push is now identifiable.
- **Cons:** Does not, by itself, *fix* a missed notification — only makes
  the cause discoverable. Increases log volume on the notification path
  (one INFO per recipient per event, broadcast events have N recipients).
- **Effort:** Small.

### Option B: Persist a NotificationDeliveryAttempt audit row per recipient

- **Approach:** Introduce a `models.NotificationDeliveryAttempt` table
  (one row per (community_event_id, user_id)) capturing the gate outcome,
  device count, FCM message IDs, and any error. Written from the subscriber
  + `notifications.Service` boundary. Surface a read endpoint
  (`AdminGetEventDispatchTrace`) or query directly. Could optionally drive
  a future per-user "missed notification" surface in the app.
- **Pros:** Persistent, queryable, auditable — survives log retention. Could
  become the source of truth for `NotifyUser` retry / DLQ logic and for
  Metabase dashboards on notification health.
- **Cons:** Significant: new proto model, new storage, write-path overhead
  on every push (one extra INSERT per recipient), data growth, retention /
  GDPR considerations on storing per-user delivery records. Over-engineered
  for the immediate question of "why didn't Bryan get one push?".
- **Effort:** Large.

### Option C: Direct fix on the single highest-suspicion gate (active-stream suppression)

- **Approach:** The most plausible silent suppressor today is
  `streamCheck(community_id, user_id)` in `subscriber.go:127`. If Bryan
  had the app open in the background with stale stream state, the server
  would still report him as actively streaming, suppress the push, and
  the stream itself wouldn't deliver because the OS has paused the network
  socket. Stop suppressing when the stream has no recent heartbeat (e.g.
  `last_event_sent_unix_sec` < `now - 60s`), and/or remove
  active-stream suppression entirely so push delivery is the source of
  truth.
- **Pros:** If this is the bug, the fix is small and targeted.
- **Cons:** We do not have evidence this is the bug — we are guessing.
  Removing stream suppression entirely causes double-notification UX for
  users who really do have the app foregrounded (push + in-app toast on
  the stream). Tightening with a heartbeat check requires server-side
  tracking of `last_event_sent_unix_sec` per stream and is itself a
  small refactor of `streaming.go`. Either way, this is a speculative fix
  without diagnostic data.
- **Effort:** Small if we just remove suppression, medium if we add a
  heartbeat-based check.

## Recommendation

**Option A.** Bryan's own framing ("audit the logs to confirm these events
are triggering notifications or perhaps it's delivery difficulties") is
explicit: we cannot fix what we cannot see, and right now the system is
genuinely opaque past the "sending notification" line. The current
per-recipient DEBUG suppression logs are dead weight in production. Option
A is one focused change to `subscriber.go` + `service.go` + the FCM
provider, no schema change, no client change, immediately useful for this
and every future "missed push" report.

Option B is the right long-term direction but is too big for the
investigation we need to do *first*. Option C is a guess — we should not
ship guesses to production notification code. Treat Option C as a
follow-up that becomes well-defined once the Option A logs identify the
actual gate that fired.

## Open Questions

- **Which event covers "new offers on events"? `REQUEST_OFFER_MADE` or
  `PLANNING_CONTRIBUTION_ADDED`?** — Recommendation: instrument both. They
  share a code path (the subscriber) so the work is identical; let the
  logs disambiguate which one Bryan was thinking of and whether one or
  both are silently failing.
- **Should we suppress push when there's an active stream at all?** —
  Recommendation: keep current behaviour for now, but make
  `active_stream_suppressed` count visible in logs (Option A delivers
  this). If Option A's data shows this is the dominant outcome for missed
  notifications, file a follow-up to remove or heartbeat-gate the
  suppression. Do not change this gate in this PR.
- **Should `no_devices` log at INFO or WARN?** — Recommendation: INFO.
  Many users legitimately have no registered devices (web-only, never
  granted permission); WARN would be noise. The aggregate ratio is what
  matters and that's a Metabase query, not a per-event alert.
- **Should we add a Metabase dashboard for notification outcomes?** —
  Recommendation: defer to a follow-up issue once we have structured
  logs to query. Out of scope for this PR.
- **Do we need to backfill / triage Bryan's specific incident?** —
  Recommendation: yes, but as a parallel ops task (search Cloud Logging
  for `target_request_id` filtered to his communities around the report
  timestamp) — not as part of the code change. The plan above makes
  *future* incidents diagnosable; the May 22 incident may be
  reconstructable from existing logs by hand.

## Implementation Plan

### Phase 1: Structured per-recipient dispatch outcome logs

**Goal:** Every push attempt — successful, suppressed, or failed — emits
a single structured INFO log line keyed by (community_event_id, user_id)
with an explicit `outcome` field.

**Tasks:**
- [ ] Define an `outcome` taxonomy in `server/notifications/community_subscriber/subscriber.go`:
      `sent`, `suppressed_actor`, `suppressed_active_stream`,
      `suppressed_category`, `suppressed_no_devices`, `send_failed`,
      `skipped_community_deleted`. Centralise the strings as constants in
      this package.
- [ ] Replace the three existing DEBUG suppression logs and the existing
      INFO "sending notification to user" line in `Handle` with one
      structured INFO call per recipient containing
      `operation` (`community_notifications.handle`), `community_event_id`,
      `event_type`, `community_id`, `user_id`, `outcome`,
      and (for `sent`/`send_failed`) `device_count`. Use `community_event_id`
      (not `event_id`) per the per-event correlation convention in
      `docs/server/observability.md` §"Streams and per-event correlation",
      so this lines up with the existing `StreamCommunityEvents` per-event
      logging.
- [ ] In `server/notifications/service.go::NotifyUser`, promote the
      `no devices registered for user` DEBUG line to a single per-call
      INFO log with the same shape so the subscriber-side
      `suppressed_no_devices` outcome is corroborated at the service
      layer. Add an `outcome=sent|partial_failure|all_invalid_tokens|all_failed`
      summary at the bottom of `NotifyUser` carrying `community_event_id`.
      Threading: the subscriber already calls `NotifyUser` with the
      already-built `notification`. The cleanest path is to stash the
      `community_event_id` on the dispatch context via a small
      package-private context-key helper in `server/notifications`
      (`notifications.WithCommunityEventID(ctx, id)`) — avoids changing
      the `Notification` proto and avoids changing the `Service`
      interface signature. Do **not** add an `event_id` field to
      `models.Notification`; the per-event correlation belongs in logs,
      not on the wire payload (it would be visible to clients and lacks
      a defined client-side use today).
- [ ] In `server/notifications/fcm/provider.go::Send`, ensure each
      per-device send/error log carries the same `community_event_id`
      field by reading it from the same context-key helper above. Mask
      device tokens via `logging.MaskToken` (already done — keep it).
- [ ] Update README copy in `server/notifications/community_subscriber/README.md`
      to document the new outcome enum and the recommended Cloud Logging
      filter (`jsonPayload.operation = "community_notifications.handle"
      AND jsonPayload.outcome != "sent"`). Cross-link to the per-event
      correlation section in `docs/server/observability.md` so future
      authors of subscribers follow the same shape.

**Validates:** A unit test in `subscriber_test.go` that exercises each
gate (actor self, active stream, category off, no devices, happy path)
and asserts the captured log records carry the expected `outcome` value.
Use the existing `logging` test sink pattern; if no log capture helper
exists, add a minimal one in `server/logging` and unit-test it
separately. No proto change, no schema change, no client change — all
work is confined to `server/notifications/` plus its README.

### Phase 2: Triage Bryan's incident and broader silent-loss rate

**Goal:** Use the Phase 1 logs to (a) identify the gate that suppressed
Bryan's pushes if reproducible in PROD logs, and (b) measure how often
each outcome fires across all users.

**Tasks:**
- [ ] Ops task (not code): search Cloud Logging for community events
      authored in Bryan's communities between 2026-05-20 and 2026-05-22
      with `event_type IN (REQUEST_CREATED, REQUEST_OFFER_MADE,
      PLANNING_CONTRIBUTION_ADDED)` and Bryan's user_id as recipient.
      Confirm the gate.
- [ ] If a specific gate is identified as dominant, file a targeted
      follow-up issue with a clear hypothesis (e.g. "stale active-stream
      registration leaks for ~30s after app background"). Do NOT
      preemptively change suppression logic in this PR.
- [ ] Add a log-based metric in `terraform/modules/monitoring` keyed by
      `jsonPayload.outcome` and `jsonPayload.event_type`, mirroring the
      existing `rpc_errors_{environment}` pattern. Optionally surface a
      Metabase chart counting outcomes by `event_type` over the last 7 /
      30 days. Owner: triage rotation.

**Validates:** A documented answer to "for event X, what happened to
user Y's push?" reachable from one log filter, and a baseline silent-
suppression rate per event type.

### Phase 3 (deferred — only if Phase 2 identifies a root cause)

**Goal:** Targeted fix for the gate Phase 2 identifies (e.g. stale
active-stream suppression, missing devices for users who once granted
permission, category mis-default, etc.).

**Tasks:** Defined in a separate follow-up issue once Phase 2 data is
in hand. Do not pre-design this phase here.

**Validates:** Re-triage with the same Phase 1 logs shows the targeted
outcome dropping to near-zero.

## Related Issues

- #1004 — FCM auth-error log cascade (already addressed; relevant
  because that PR establishes the convention for structured FCM error
  logging this plan extends).
- #1027 — iOS push delivery failures (different platform, same diagnostic
  gap; Phase 1's logs would have made that much easier to triage).
- #1107 — Per-community notification preferences (defines the
  `notify_new_requests` / `notify_request_updates` toggles that gate the
  events at the heart of this report).
- #1254 — `NotRegistered` FCM token cleanup (relevant if Phase 2 finds
  that Bryan's device token had churned; the cleanup path already exists
  but a recently-cleaned token would surface as `suppressed_no_devices`).
- #1622 — Soft-deleted-community notification guard (one of the gates
  Phase 1 explicitly logs).
- #1682, #1892, #1893 — Adjacent notification observability and routing
  work; Phase 1 should align outcome naming with whatever conventions
  these set if any.
