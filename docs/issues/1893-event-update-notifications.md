# Issue #1893: Updates to events (time, location) should trigger notifications for attendees

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1893
**Filed:** 2026-05-12 | **Last activity:** 2026-05-12
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When the owner of an Experience (an Event in user-facing copy) edits the
time or location via `SaveExperience`, the change is announced today
only as a `CHAT_SYSTEM_ACTION_DETAIL_CHANGED` system message inside the
event's conversation. There is no push notification, so any Yes/Maybe
RSVP who is not actively watching the conversation finds out only when
they next open the app — at which point they may already be heading to
the wrong place or showing up at the wrong time.

The user-facing concern is straightforward: time and location are the
two pieces of information an attendee needs to actually show up.
Silent changes to either are a real-world problem. Yes/Maybe RSVPs
should be notified out-of-band, the way they already are for
`EXPERIENCE_STARTED` and `EXPERIENCE_CANCELLED`.

## Context from Comments

The only comment is `@claude` mentioning this bot. No additional design
discussion. The issue body itself is short and unambiguous: "these
changes are swallowed silently … Yes/maybe RSVPs should be notified
when they change." There is no debate about which fields qualify
(time + location are explicitly named), about whether NO RSVPs should
be notified (the author says yes/maybe only), or about preference
gating.

## Potential Solutions

### Option A: One generic `EXPERIENCE_UPDATED` event

- **Approach:** Add a single `COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED`
  enum value. Emit it once from `SaveExperience` (per
  `CommunityExperience`) when *either* time or location changed,
  carrying `time_changed` / `location_changed` booleans on the
  `CommunityEvent` itself. Push copy blends them
  ("{actor} updated time and location" or single-field variants).
  Recipients = Yes/Maybe RSVPs minus actor, gated by the existing
  `notify_experience_rsvps` preference (same bucket as
  `EXPERIENCE_STARTED` / `EXPERIENCE_CANCELLED`).
- **Pros:** Smallest enum surface (one value, not two). Single event
  on the bus when an owner changes both at once — no double-buzz.
  Copy can be specific or generic depending on which flag(s) are set.
  Matches the existing system-message behavior, which already emits
  one `DETAIL_CHANGED` per save regardless of how many fields moved.
  Cleanest fit for the undo registry — one entry, classification
  `ClientOnly` (the inverse is editing again).
- **Cons:** Push body has to branch on event-level flags, which is a
  little more code in `copy.go` than the existing handlers.
  Notification analytics can't distinguish "time changed" vs
  "location changed" without inspecting the event fields.
- **Effort:** Medium.

### Option B: Two separate event types

- **Approach:** Add
  `COMMUNITY_EVENT_TYPE_EXPERIENCE_TIME_CHANGED` and
  `COMMUNITY_EVENT_TYPE_EXPERIENCE_LOCATION_CHANGED`. Emit one or
  both from `SaveExperience` based on which fields changed.
- **Pros:** Maps cleanly to one fixed push copy per type. Easier to
  filter in analytics. Mirrors how transfer events are split by
  sub-action.
- **Cons:** A single owner edit that touches both fields now fires
  two notifications, which is exactly the user complaint inverted
  (noisy). Two enum values means two undo-registry entries, two
  recipient-funnel branches, two copy-test rows, two
  preference-category test rows. The system-message side already
  collapses to one `DETAIL_CHANGED`, so the events would be
  out of sync with the chat record.
- **Effort:** Medium-large.

### Option C: Reuse `DETAIL_CHANGED` system message only (no push)

- **Approach:** Keep the existing `DETAIL_CHANGED` system message and
  rely on the chat-unread badge / chat preview to surface the change.
  Optionally make that system message generate an in-app banner.
- **Pros:** No new event type, no recipients code, no push copy.
- **Cons:** Does not solve the user's reported problem. The whole
  point of the issue is that the system message is "swallowed
  silently" for users who aren't already in the conversation.
- **Effort:** Small but rejected on requirements.

## Recommendation

**Option A.** A single `EXPERIENCE_UPDATED` event is the right shape:

1. One save = one push, even when both fields change. This matches
   the existing single `DETAIL_CHANGED` system message and is what
   the user expects (the complaint is about silence, not granularity).
2. The change is small in surface area: one enum value, one funnel
   branch, one copy switch case, one undo-registry entry, one
   notification-category mapping. Option B doubles every one of those.
3. Reusing the existing `NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS`
   preference is correct: it already covers `EXPERIENCE_STARTED` and
   `EXPERIENCE_CANCELLED`, and its in-app subtitle is "RSVPs to
   events you host or attend" — semantically a bucket of "event
   lifecycle pokes for events I'm part of." A user who has opted out
   of starting/cancellation pings is opting out of the same
   conceptual class. Adding a new preference for this single ping
   would over-fragment UX; if users want finer-grained control later,
   the preference can be split without changing the event model.

The recipients funnel for this event reuses the existing Yes/Maybe-
RSVP-minus-actor logic shared by `EXPERIENCE_STARTED` /
`EXPERIENCE_CANCELLED` / planning events in
`server/notifications/community_subscriber/recipients.go`. No new
query patterns.

## Open Questions

- **Should the event carry which fields changed (time and/or location)
  on the wire?** — **Recommendation: yes, on `CommunityEvent`
  itself.** Add `optional bool time_changed = 15;` and
  `optional bool location_changed = 16;` on `CommunityEvent` (next
  free field numbers — current max is 14). The push copy and any
  in-app feed handler both need these to vary the message. Adding
  them as `optional bool` is wire-compatible per the project's proto
  conventions (CLAUDE.md "Use `optional` for conditionally-present
  fields") and they only have meaning on `EXPERIENCE_UPDATED` events.
  Also mirror them onto `CommunityEventPayload` (notification.proto,
  next free field numbers after 9 → 10, 11) so the Flutter client
  can render the same nuance for any in-app surfacing without an
  extra lookup.

- **Does a no-op `SaveExperience` (e.g. owner submitted the same
  time) also fire?** — **Recommendation: no.** Reuse the existing
  `timeChanged` / `locationChanged` flags computed in `save.go`
  lines 274–277. Those compare the request against the stored value
  before mutation; if neither flipped, no push fires. This mirrors
  what `DETAIL_CHANGED` already does.

- **Suppression when the owner is also a Yes/Maybe RSVP to their own
  event?** — **Recommendation: already handled.** The existing
  funnel filters by `rsvp.UserId == event.ActorId` (see
  `recipients.go` line 230), so the owner won't push themselves.

- **What about updates to `name`, `description`, or
  `max_participants`?** — **Recommendation: out of scope.** The
  issue explicitly names time and location. `name` and `description`
  aren't show-up-critical the way time/location are, and
  `max_participants` is a separate user-experience question (does
  it warn waitlisted attendees?). Restrict `EXPERIENCE_UPDATED`
  emission to the existing `timeChanged || locationChanged`
  condition so future fields don't silently widen the push surface.

- **Stream / story side: should the event also surface in the
  activity stream?** — **Recommendation: no story, no stream
  entry.** This is a transient lifecycle ping (like
  `EXPERIENCE_STARTED`), not a durable accomplishment. Treat it the
  same way the lifecycle-ping branch in
  `recipients.go` (line 43) treats `EXPERIENCE_STARTED` /
  `EXPERIENCE_CANCELLED`.

- **Client-side deep link on push tap?** — **Recommendation: same
  handler as `EXPERIENCE_STARTED` / `EXPERIENCE_CANCELLED`.** The
  push carries `experience_id`; the existing tap handler routes to
  the event detail screen by experience id. No new client routing
  code required.

- **Does the `notify_experience_rsvps` preference subtitle need a
  copy update?** — **Recommendation: yes, lightweight.** Current
  English copy: `settingsNotifyExperienceRsvpsSubtitle` =
  "RSVPs to events you host or attend." With this change the
  preference also gates lifecycle pings *about* events you're
  attending (start, cancel, and now update). Suggested revised
  copy: "RSVPs and updates for events you host or attend." Update
  both `app_en.arb` and `app_es.arb` in lockstep.

- **Where is the in-app community-event feed?** — Confirm at
  implementation time. If there is no per-`CommunityEventType`
  switch in any Flutter feed widget today, no client-side code
  needs to change beyond ARB strings; the server-generated push
  body is what users see. If there is such a feed, add a
  `EXPERIENCE_UPDATED` case that reads the payload booleans.

## Implementation Plan

### Phase 1: Proto + enum + undo registry

**Goal:** Make `COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED` a first-class
event type without yet emitting it.

**Tasks:**
- [ ] In `proto/ripls/models/community.proto`:
  - Add `COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED = 44;` to
    `CommunityEventType` (next free slot — current max is 43).
    Place it adjacent to `EXPERIENCE_STARTED` /
    `EXPERIENCE_CANCELLED` with a proto comment describing what it
    means, scoped to the wire contract (no server mechanics).
  - Add `optional bool time_changed = 15;` and
    `optional bool location_changed = 16;` to `CommunityEvent`
    (next free numbers — current max is 14). Proto comments should
    describe what *true* means (the time / location actually
    changed in this `SaveExperience`), without naming the call site.
- [ ] In `proto/ripls/models/notification.proto`, add
      `optional bool time_changed = 10;` and
      `optional bool location_changed = 11;` to
      `CommunityEventPayload` so Flutter clients can render
      nuanced copy without an extra fetch.
- [ ] Run `npm run generate` (Go + Dart).
- [ ] Classify the new event in `server/undo/registry.go` as
      `ClassificationClientOnly` with rationale: "Inverse is
      editing the experience again via SaveExperience. No server
      retraction needed."
- [ ] Run `npm run build` and `go test ./server/undo/...`. Confirm
      `TestEveryEventTypeClassified` passes.

**Validates:** Build is green; the registry gate accepts the new
enum value; existing tests still pass.

### Phase 2: Server emission from SaveExperience

**Goal:** Emit `EXPERIENCE_UPDATED` per `CommunityExperience` link
when time or location changes, populating the changed-field
booleans on the event.

**Tasks:**
- [ ] In `server/services/experience/save.go`, after the existing
      `Update` (line 383) and after the `DETAIL_CHANGED` system
      message block (lines 391–412), add a new block scoped to the
      *update* branch (not create) that:
  - Returns early if `!(timeChanged || locationChanged)`.
  - Queries `CommunityExperience` rows by `experience_id` (mirror
    `lifecycle.go` lines 83–106). Treat the query error as
    best-effort: log via `logger.WarnContext(ctx,
    "failed to query community experiences for EXPERIENCE_UPDATED
    notification", "error", err)` and skip emission rather than
    failing the save.
  - For each linked community, publish a `CommunityEvent` with
    `EventType = COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED`,
    `ActorId = authInfo.UserID`,
    `Topic = ExperienceId(experienceID)`,
    `TimeChanged = &timeChanged`, `LocationChanged = &locationChanged`,
    and `OccurredAtUnixSec = clock.UnixSec(ctx)`. Use the same
    `s.bus.Publish` pattern as `MarkExperienceInProcess`.
- [ ] Logging: scope a logger with `logging.LoggerWithContext(ctx)`
      plus `operation`, `experience_id`, and (per linked community)
      `community_id`, `time_changed`, `location_changed`. Mirror
      `MarkExperienceInProcess` lines 33–36 in shape. Per
      `docs/server/observability.md`, do not introduce new entity-ID
      log field names; reuse `experience_id` and `community_id`.
- [ ] If `s.bus.Publish` returns an error, log with `WarnContext`
      and `continue` — do not fail `SaveExperience`. Mirror
      `MarkExperienceInProcess` lines 99–104.
- [ ] Add a unit/integration test under
      `server/services/experience/` (new file, e.g.
      `save_update_notify_test.go`):
  - Time-only change → one event per linked community with
    `time_changed=true`, `location_changed=false`.
  - Location-only change → mirror with flags flipped.
  - Both fields → one event per community, both flags true.
  - No-op (same time, same location) → zero events.
  - Description-only update (unchanged time/location) → zero events.
  - Multi-community share → one event per `CommunityExperience` row.
  - Test should use `storage.SetupTestStorage(t)` and a fake/recording
    `CommunityEventBus`. Build tag with `//go:build integration` only
    if it requires AI APIs; otherwise keep it as a regular test.

**Validates:** `npm run build`, `go test ./server/services/experience/...`,
and (for cross-package signatures) `go build -tags=integration ./...`
all pass. Manual repro is deferred to Phase 5.

### Phase 3: Notification copy, category, recipients, and gating

**Goal:** Yes/Maybe RSVPs minus the actor get a push, gated by
`notify_experience_rsvps`. Copy varies with which fields changed.

**Tasks:**
- [ ] In `server/notifications/community_subscriber/recipients.go`:
  - Add `EXPERIENCE_UPDATED` to the `shouldNotify` allow-list
    (the lifecycle-pings branch around line 43).
  - Add `EXPERIENCE_UPDATED` to the `getNotificationRecipients`
    switch — specifically the `EXPERIENCE_STARTED /
    EXPERIENCE_CANCELLED` case at line 213. The recipient set
    (Yes/Maybe RSVPs minus actor) is identical, so just extend
    the case label rather than duplicating the body.
- [ ] In `server/community/notifications.go`, add
      `EXPERIENCE_UPDATED` to the `CategoryFor` case list for
      `NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS` (lines 57–62). No
      new preference field is needed.
- [ ] In `server/notifications/community_subscriber/copy.go`, add a
      `case` for `EXPERIENCE_UPDATED`. Read `event.GetTimeChanged()`
      and `event.GetLocationChanged()`:
  - Title: `experienceName`.
  - Body, depending on flags:
    - both true → `fmt.Sprintf("%s updated time and location",
      actorName)`
    - time only → `fmt.Sprintf("%s updated the time", actorName)`
    - location only → `fmt.Sprintf("%s updated the location",
      actorName)`
    - neither (defensive fallback for replayed malformed events)
      → `"Event details updated"`
  - Populate `payload.TimeChanged` and `payload.LocationChanged`
    when constructing `CommunityEventPayload`.
  - Do not vendor-name the notification provider in any proto
    comment.
- [ ] Add tests:
  - `recipients_test.go`: add table rows mirroring the
    `EXPERIENCE_STARTED` rows (around line 1093 and 926) asserting
    `shouldNotify` is true and that the recipient set equals
    Yes/Maybe RSVPs minus actor. Cover the NO-RSVP-excluded case
    and the actor-excluded case.
  - `copy_test.go`: three rows — time-only, location-only,
    both-fields-changed — asserting the body string and that
    `Payload.TimeChanged` / `Payload.LocationChanged` round-trip
    correctly.
  - `preferences_filter_test.go`: a row asserting that the
    `notify_experience_rsvps` preference gates `EXPERIENCE_UPDATED`
    the same way it gates `EXPERIENCE_STARTED`.

**Validates:** `go test ./server/notifications/...
./server/community/... ./server/services/experience/...
-tags=integration` passes. A focused run with
`go test -run TestEveryEventTypeClassified ./server/undo/...`
confirms registry coverage. End-to-end: saving a time change
publishes an event, the dispatcher resolves Yes/Maybe RSVPs minus
actor, and only users with `notify_experience_rsvps != false` get a
notification with copy matching the changed-field set.

### Phase 4: Client side — ARB strings + optional in-app surface

**Goal:** No Flutter-side behavioral changes are required for push
delivery (the server formats title/body). But:
1. Update the `notify_experience_rsvps` preference subtitle so the
   bucket label honestly covers the new event type.
2. Add ARB strings for any in-app banner / event-feed surface that
   chooses to render the payload.

**Tasks:**
- [ ] Update preference subtitle (per i18n.md, both locales required
      before `npm run generate`):
  - `app/lib/l10n/app_en.arb` →
    `settingsNotifyExperienceRsvpsSubtitle`: "RSVPs and updates for
    events you host or attend."
  - `app/lib/l10n/app_es.arb` → translate the new copy:
    "Confirmaciones y actualizaciones para eventos que organizas o
    asistes."
- [ ] Add ARB strings for the in-app surface (use the `experience*`
      prefix per i18n.md and refer to "Event" not "Experience" in
      values per CLAUDE.md):
  - `experienceUpdatedTimeAndLocationBody` →
    "{actor} updated time and location"
  - `experienceUpdatedTimeBody` → "{actor} updated the time"
  - `experienceUpdatedLocationBody` → "{actor} updated the location"
  - Declare `{actor}` placeholders in the `@key` metadata.
  - Mirror to `app_es.arb`. Suggested Spanish copy:
    - "{actor} actualizó la hora y el lugar"
    - "{actor} actualizó la hora"
    - "{actor} actualizó el lugar"
- [ ] Run `npm run generate` (regenerates `AppLocalizations`).
- [ ] Optional: if a feed widget in
      `app/lib/presentation/widgets/community/` or
      `app/lib/presentation/screens/` switches on
      `CommunityEventType`, add a case for `experienceUpdated` that
      uses the payload booleans and the ARB strings. Per
      `docs/client/architecture.md`, route the data through the
      relevant repository / viewmodel; do **not** call services or
      repositories directly from widgets.
- [ ] Per `docs/client/accessibility.md`, any new Tappable or
      banner that surfaces this in-app must compose with the
      accessibility primitives. The push notification itself does
      not introduce new in-app widgets, so the a11y gate is
      typically not triggered by this change. If a feed handler is
      added, give it a `semanticsLabel:` sourced from `context.l10n`
      (likely `a11y`-prefixed key).
- [ ] Per `docs/client/i18n.md` and `npm run lint:dart:i18n`, do not
      hardcode any strings in widget code — every user-visible string
      must resolve through `context.l10n`.
- [ ] Verify `flutter test --reporter expanded` from `app/` passes.
      `npm run lint:dart:size` still passes (the changes here are
      ARB-only or tiny widget additions; no file-length risk).

**Validates:** Generation is clean, both locales build, lints pass,
and the ARB additions are picked up by tests.

### Phase 5: Acceptance verification

**Goal:** Confirm the fix resolves the reported bug from a user
perspective.

**Tasks:**
- [ ] Manual repro on dev: as Owner, share an event into a community
      with two other members. As Member A, RSVP Yes. As Member B,
      RSVP No. As Owner, edit the time. Verify Member A receives a
      push and Member B does not.
- [ ] Repeat for a location change.
- [ ] Repeat with both fields changed in a single save — verify
      exactly one push to Member A with body "updated time and
      location".
- [ ] Toggle `notify_experience_rsvps` off for Member A and verify
      no push is delivered.
- [ ] Verify the existing `DETAIL_CHANGED` system message in the
      conversation is unchanged.
- [ ] Verify push tap deep-links into the event detail screen.

**Validates:** Closes #1893.

## Audit Notes (added after `/ripls-audit` pass)

Changes I made vs. the original draft to align with project docs:

- **Logging conventions** (`docs/server/observability.md`): all
  per-event emit logs go through `logging.LoggerWithContext(ctx)`
  scoped with `operation`, `experience_id`, `community_id`, plus
  the changed-field booleans. No `log.Printf` / `fmt.Printf`. No
  ERROR-level for handled, best-effort failures — those stay
  `WarnContext`. Entity IDs use `experience_id` / `community_id`
  (not `request_id`-style overrides).
- **Failure mode** (`server/architecture` §Error Handling): the
  best-effort fan-out is documented in `MarkExperienceInProcess`'s
  shape (warn-and-continue), so it's a known acceptable pattern for
  notification fan-out. The doc-comment on the new block explicitly
  names that as the rationale.
- **i18n** (`docs/client/i18n.md`): both `app_en.arb` and
  `app_es.arb` updated in the same phase, before `npm run
  generate`. Placeholders declared in `@key` metadata. ARB values
  say "Event", not "Experience", per CLAUDE.md.
- **a11y** (`docs/client/accessibility.md`): only relevant if an
  in-app feed handler is added; the plan calls that out instead of
  silently bypassing the rule.
- **Proto conventions** (`docs/proto_conventions.md`): proto
  comments describe wire meaning only — no transport plumbing, no
  server mechanics, no vendor names. The two new booleans use
  `optional` per the CLAUDE.md guidance.
- **Testing** (`docs/server/architecture.md` and
  `docs/client/testing.md`): unit/integration coverage is named per
  layer — server table-driven recipient/copy/preference tests, an
  integration-style emission test for `SaveExperience` that uses
  the real storage testcontainer via `storage.SetupTestStorage`,
  and Flutter tests only on the ViewModel layer (no widget tests)
  if a feed surface is added.
- **Architecture** (`docs/client/architecture.md`,
  `docs/client/caching.md`): client surface, if added, routes
  data through ViewModels → Repositories; widgets never call
  services directly and do not implement manual cache maps.

## Related Issues
- (no directly linked issues found at plan time; if a follow-up
  for surfacing event-update activity in the community feed is
  filed, link it here on review)
