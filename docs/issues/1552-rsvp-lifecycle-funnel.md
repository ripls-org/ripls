# Issue #1552: Normalize experience-lifecycle RSVP notifications onto the community-event funnel

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1552
**Filed:** 2026-04-28 | **Last activity:** 2026-04-29
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** docs/workflows/experience.md, docs/issues/1107-per-community-notifications.md, docs/issues/1533-experience-lifecycle-notifications.md, docs/ai/undo_plan.md

## Problem Statement

Today, the three experience-lifecycle pushes — STARTED (mark in-process), COMPLETED, CANCELLED — bypass the central community-event notification funnel. `server/services/experience/lifecycle.go:82,357,553` each fire `notifyRSVPedUsers`, which:

- Builds a hand-rolled `Notification` payload (no `EventId`, no `EventType` string, partial `CommunityEventPayload`) — clients can't deep-link the same way they do for every other community event.
- Skips `shouldNotify` / `getNotificationRecipients` and so duplicates work that the funnel already does for other event types.
- Skips the `streamChecker` suppression that `notifyTargetedUsers` applies (`server/community/events.go:79`), so users with an active event stream get a push on top of the in-app event.
- Does not exclude the actor from candidates (`lifecycle.go:746–759`), so the owner self-pings on every lifecycle transition. The relaxed assertions in `experience_test.go` (`Postcondition_OwnerNotifiedOnRSVPs` lines 311–328 and `Postcondition_NotificationCountMatchesContract` lines 1645–1656) document this off-by-one and explicitly point at #1552 for the fix.
- Re-implements the per-community preference filter (`rsvpUserShouldBeNotified` at `lifecycle.go:840–859`) instead of falling out of `CategoryFor` like every other category.

The RSVP **action** is already a normal community event (`COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES/MAYBE/NO`, emitted from `server/services/experience/events.go:45`), so the surrounding plumbing — `RecordCommunityEventAndNotify`, the recipient resolver, the preference fetch, the broadcaster — is already in place. Only the lifecycle pings remain a special case.

There is one substantive design decision in this issue that the original filing handles in a single sentence ("Use `COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED` (already in the enum)"): the existing `EXPERIENCE_COMPLETED` event today fans out to every community member (broadcast), gated by `notify_experience_completed`. Routing the lifecycle RSVP ping through the same event type changes its recipient semantics from "everyone in the community" to "RSVPers only". Resolving that tradeoff is the bulk of this plan.

## Context from Comments

The issue has one comment — a `@claude` ping from a maintainer triggering this run. There is no in-thread discussion to draw on. Context comes from sibling issues:

- **#1107 (closed by PR #1553):** introduced the `notify_experience_rsvps` per-community preference and the dual-path lifecycle pings. The proto comment on `NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS` (`proto/ripls/models/community.proto:332–334`) explicitly enumerates "RSVPs to experiences the user hosts plus lifecycle pings (started, completed, cancelled) for experiences the user said yes/maybe to" — i.e. the RSVPS gate was always meant to cover all three lifecycle pings. The dual-path implementation was a deliberate intermediate step that explicitly named #1552 as the cleanup.
- **#1533 (just landed):** tightened the integration-test assertions to lock the *current* counts, including the owner self-ping that #1552 will fix. Comments in `experience_test.go` reference #1552 by number with phrasing like "actor exclusion tracked in #1552" — the test owner expects the counts to drop after this issue lands.

The proto file already reserves `EXPERIENCE_COMPLETED` enum value 29 with a comment that ties it to the generalized-undo plan (`docs/ai/undo_plan.md`); it is *currently* used both as a broadcast hook and (informally, via the parallel `notifyRSVPedUsers` call) as a lifecycle ping.

## Potential Solutions

### Option A: Repurpose `EXPERIENCE_COMPLETED` to RSVPers only — issue's stated intent

- **Approach:**
  - Add `COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED` (= 38) and `COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED` (= 39) to `proto/ripls/models/community.proto`.
  - In `server/community/notifications.go`:
    - Add the two new event types to `shouldNotify`'s allow-list.
    - In `getNotificationRecipients`, replace the `EXPERIENCE_COMPLETED` case (currently in the broadcast bucket alongside `EXPERIENCE_CREATED`/`GEAR_SHARED`/`REQUEST_CREATED`) with a new case that handles all three lifecycle events — `EXPERIENCE_STARTED`, `EXPERIENCE_COMPLETED`, `EXPERIENCE_CANCELLED` — by returning Yes/Maybe RSVPs minus the actor (same query `notifyRSVPedUsers` runs today).
    - Map all three to `NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS` in `CategoryFor`. The existing `NOTIFICATION_CATEGORY_EXPERIENCE_COMPLETED` becomes orphaned — see Open Questions for handling.
    - Add `buildNotification` cases for `EXPERIENCE_STARTED` ("Event in progress" / `"<actor> started '<experience>'"`) and `EXPERIENCE_CANCELLED` ("Event cancelled" / `"'<experience>' was cancelled"`); existing `EXPERIENCE_COMPLETED` case stays.
  - In `server/services/experience/lifecycle.go`:
    - Replace each `notifyRSVPedUsers` call (lines 82, 357, 553) with a `RecordCommunityEventAndNotify` invocation, one per `CommunityExperience` row (the COMPLETED path already does this for the broadcast event — extend the loop to the STARTED and CANCELLED paths).
    - Remove `notifyRSVPedUsers`, `experienceCommunityIDs`, `fetchExperienceRsvpPrefs`, and `rsvpUserShouldBeNotified` (lines 727–859) — the funnel handles all of it.
  - In `server/undo/registry.go`, classify both new types as `ClassificationClientOnly` with a rationale matching the existing `EXPERIENCE_RSVP_YES` entry (the lifecycle UI exposes the inverse — restart/uncancel are not user actions, only re-state-transition flows under the hood).
  - In `proto/ripls/models/community.proto`, update the `NOTIFICATION_CATEGORY_EXPERIENCE_COMPLETED` comment to flag it as deprecated and the `NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS` comment is already correct.
  - Update the integration-test contract comments and counts in `server/integration_tests/experience_test.go` (Examples 1, 2, 7) — the actor self-ping disappears (drop one per lifecycle event from each affected count) and stream-suppression now applies (no per-test impact since tests don't open streams, but the comment needs updating).
  - Update `docs/workflows/experience.md` Effect lines for operations 6/7/8 and the Notifications section to describe a single funnel.
- **Pros:**
  - Matches the issue's stated intent verbatim ("Use `COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED`", "Delete `notifyRSVPedUsers`", "Add a `getNotificationRecipients` case").
  - Single funnel, single preference gate, single payload shape for clients.
  - Owner self-ping bug (`#1552` in test comments) is fixed for free.
  - Stream-suppression applies uniformly.
- **Cons:**
  - Drops the community-wide broadcast on COMPLETED. Today, a member who didn't RSVP but has `notify_experience_completed=on` gets a push when an event in their community wraps up. After this change they don't. The user-visible delta is small (event broadcasts persist via `eventBroadcaster` for in-app activity feed; only the *push* is gone for non-RSVPers), but it is a real reduction in coverage.
  - The `notify_experience_completed` toggle (proto field 7 on `CommunityNotificationPreferences`, plus the Flutter switch in `manage_notifications_screen.dart:157`) becomes orphaned — see Open Questions for handling.
- **Effort:** Medium. Touches proto + Go server + integration tests + docs; Flutter changes are minimal (the toggle decision drives it).

### Option B: Add `EXPERIENCE_STARTED` / `EXPERIENCE_CANCELLED`, leave `EXPERIENCE_COMPLETED` alone, delete only the STARTED/CANCELLED `notifyRSVPedUsers` calls

- **Approach:**
  - Add the two new enum values and route them through the funnel as in Option A.
  - Keep `EXPERIENCE_COMPLETED` as a community-broadcast event with the existing `notify_experience_completed` gate.
  - Keep one residual `notifyRSVPedUsers` call from `CompleteExperience` so RSVPers still get a push gated by `notify_experience_rsvps` — and accept that COMPLETED still doubles up for users who have both toggles on (matching today's behavior).
  - Same undo classification, doc, and proto-comment work.
- **Pros:**
  - No coverage reduction for COMPLETED. The `notify_experience_completed` toggle stays meaningful.
  - Smallest diff; lowest risk to the lifecycle-completion notification UX that #1107/PR#1553 just shipped.
- **Cons:**
  - Doesn't fully achieve the issue's "one funnel" goal — `notifyRSVPedUsers` survives in a degenerate form solely for COMPLETED. The actor-exclusion bug isn't fixed for COMPLETED. Stream-suppression still doesn't apply to COMPLETED.
  - Codebase still carries two parallel notification paths for the experience lifecycle, which is the exact tech-debt #1552 is meant to retire.
- **Effort:** Small.

### Option C: Add `EXPERIENCE_STARTED` / `EXPERIENCE_CANCELLED`, delete `notifyRSVPedUsers` entirely, leave `EXPERIENCE_COMPLETED` as the existing community broadcast (no separate RSVPer ping)

- **Approach:** Same as Option A *except* `EXPERIENCE_COMPLETED` keeps its current resolver (broadcast to all community members minus actor) and category (`EXPERIENCE_COMPLETED`). The only behavior change for COMPLETED is that RSVPers stop getting the *separate* `notifyRSVPedUsers` push — they still get the community broadcast push if `notify_experience_completed` is on for any of the experience's communities.
- **Pros:**
  - Fully deletes `notifyRSVPedUsers` (cleanest tech-debt outcome).
  - Owner self-ping bug fixed for all three lifecycle events.
  - One funnel, one stream-suppression rule, consistent payload shape.
  - `notify_experience_completed` toggle remains meaningful.
- **Cons:**
  - Subtle UX regression for RSVPers who turn `notify_experience_completed=off` but leave `notify_experience_rsvps=on`: they would no longer receive a completion push. Today they receive one (via `notifyRSVPedUsers`). Probably an acceptable simplification, but worth flagging.
  - The proto comment on `NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS` ("...lifecycle pings (started, completed, cancelled)...") needs to be tightened to "...started, cancelled..." since COMPLETED is no longer covered.
- **Effort:** Medium.

## Recommendation

**Option C.** It captures the issue's core ask — delete `notifyRSVPedUsers`, route through the funnel, fix the actor-exclusion bug — without making the more invasive semantic change of repurposing `EXPERIENCE_COMPLETED` (Option A's main downside). The cost is the small UX regression noted above, which is well-defined, unlikely to surprise anyone (each user has explicit toggles), and trivially reversible by re-enabling either toggle.

Option A is the *literal* reading of the issue. If the maintainer prefers to consolidate `notify_experience_completed` and `notify_experience_rsvps` into one toggle (a UI simplification), Option A is the right path — but that is a Flutter design decision worth confirming via comment before proceeding. Defaulting to Option C preserves the existing toggle and ships the minimum-surprise change.

Option B is listed for completeness; the maintainer's `@claude` ping suggests no-op-on-`notifyRSVPedUsers` is unacceptable.

## Open Questions

- **Option C vs Option A: do we want to drop the community-broadcast push on COMPLETED?** — Recommendation: Option C, keep the broadcast. The broadcast is a useful "your community has activity" hook for non-RSVPers; removing it requires a UX-level argument that I can't make from inside this issue. Reviewer can flip to Option A by commenting `@claude switch to Option A` on the plan PR.
- **Naming: `EXPERIENCE_STARTED` vs `EXPERIENCE_IN_PROCESS`?** — Recommendation: `EXPERIENCE_STARTED`. Matches the existing `ChatSystemAction_CHAT_SYSTEM_ACTION_STARTED` (`proto/ripls/models/chat.proto`) emitted from the same lifecycle hook, and the public-facing string ("Event started" / "started the experience") is clearer than "In Process". The internal state name `EXPERIENCE_STATE_IN_PROCESS` is consistent because it describes a state, not an event.
- **Should `EXPERIENCE_COMPLETED_UNDONE` (enum 37) gain `EXPERIENCE_STARTED_UNDONE` / `EXPERIENCE_CANCELLED_UNDONE` siblings?** — Recommendation: no. The undo registry classifies both new types as `ClassificationClientOnly` (the lifecycle is naturally re-traversable — the owner can mark in-process again or re-create a cancelled experience), so no retraction event is needed. If the undo design later adds server-authoritative undo for cancellations, the retraction enum value can be added at that time.
- **Are `EXPERIENCE_STARTED` and `EXPERIENCE_CANCELLED` public-facing enum names ok?** — Recommendation: yes. The proto-comments rule (CLAUDE.md) requires no vendor names and no transport plumbing; these are purely domain-event names that match the user-facing UX terminology ("Event started" / "Event cancelled" — `Experience` is the code term, `Event` is the user-visible term per the CLAUDE.md "Event, not Experience" rule, but enum-value naming follows the code term).
- **Test count expectations after the fix?** — Recommendation: drop one notification per lifecycle event (the actor self-ping). For Example 1 (TestExperience_Example1_Successful, 17→14: subtract 1 from MarkInProcess and 1 from CompleteExperience RSVP-pings, no change to broadcast count). For Example 2 (Cancelled, 7→6: subtract 1 owner self-ping). For Example 7 (RecordingAttendance, 17→14: same as Example 1). Phase 4 has the implementer re-run and lock the actual counts; the test comments referencing #1552 already anticipate this drop.

## Implementation Plan

### Phase 1: Proto + funnel scaffolding

**Goal:** New event types exist, undo registry green, funnel knows about them. No behavior change yet — `notifyRSVPedUsers` still runs.

**Tasks:**
- [ ] Edit `proto/ripls/models/community.proto`:
  - Add `COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED = 38;` and `COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED = 39;` immediately after `COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED_UNDONE = 37;` (preserve enum-value-order convention; numbers reserved for future retraction-event siblings can wait).
  - Add comment block: "Lifecycle transitions on an Experience that ping Yes/Maybe RSVPs (gated by `notify_experience_rsvps`)." Match the comment style of the surrounding planning-event block. Do not mention vendors, transport, or implementation details (per CLAUDE.md proto-comments rule).
- [ ] `npm run generate:proto` — regenerate Go and Dart bindings.
- [ ] Edit `server/undo/registry.go`: add two `Registry` entries, both `Classification: ClassificationClientOnly`, in enum-value order alongside the existing `EXPERIENCE_*` entries. Rationales must name the explicit inverse (per CLAUDE.md "default to ClassificationClientOnly with a rationale explaining why no server-side undo is needed"):
  - `EXPERIENCE_STARTED`: "Inverse is `CompleteExperience` or `CancelExperience` from `IN_PROCESS`. No `UnmarkInProcess` RPC; the owner moves the lifecycle forward instead. No server retraction needed."
  - `EXPERIENCE_CANCELLED`: "Inverse is creating a new experience via `SaveExperience`. No `UncancelExperience` RPC by design — cancellation is intentionally terminal. No server retraction needed."
- [ ] Edit `server/community/notifications.go`:
  - Add both new event types to `shouldNotify`'s targeted-events case.
  - Add a new case in `getNotificationRecipients` that returns the Yes/Maybe RSVPs minus the actor. The query mirrors `notifyRSVPedUsers`'s candidate-collection step (`lifecycle.go:752–759`): `s.QueryByField(ctx, "experience_id", event.GetExperienceId(), &models.ExperienceRSVP{})`, then filter `rsvp.Intention IN ("YES","MAYBE") && rsvp.UserId != event.ActorId`. Handle a missing `experience_id` by warn-log-and-return-nil (matches the existing planning-events case at `notifications.go:238`).
  - Add both new event types to `CategoryFor`'s `NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS` case (the existing case currently lists `EXPERIENCE_RSVP_YES/MAYBE/NO`; extend it to `EXPERIENCE_STARTED/CANCELLED` as well).
  - Add `buildNotification` cases:
    - `EXPERIENCE_STARTED`: title `experienceName`, body `"Event has started"`. (Match the existing `notifyRSVPedUsers` body string.)
    - `EXPERIENCE_CANCELLED`: title `experienceName`, body `"Event has been cancelled"`.
- [ ] Add unit-test rows in `server/community/notifications_test.go` for `shouldNotify` (both new types should pass) and `CategoryFor` (both should map to `EXPERIENCE_RSVPS`). Add a unit test for the new `getNotificationRecipients` case covering: actor exclusion, intention-filter (NO RSVPs excluded, YES/MAYBE included, multi-RSVP scenarios), and missing-experience-id warn-and-return-nil.

**Validates:** Build is green (`npm run build`), `TestEveryEventTypeClassified` passes, generated Go enum values are wired through every switch in `community/notifications.go`.

### Phase 2: Switch lifecycle hooks onto the funnel

**Goal:** STARTED, COMPLETED-RSVP-side, CANCELLED all run through `RecordCommunityEventAndNotify`. `notifyRSVPedUsers` and its helpers are deleted.

**Tasks:**
- [ ] In `server/services/experience/lifecycle.go`:
  - In `MarkExperienceInProcess` (around line 82), delete the entire `logging.GoSafe(...)` wrapper *and* the `notifyRSVPedUsers` body it wraps. Replace with a synchronous per-`CommunityExperience` loop that calls `community.RecordCommunityEventAndNotify` for `COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED`, mirroring the `EXPERIENCE_COMPLETED` loop at `lifecycle.go:299–345`. (Per docs/server/architecture.md §Goroutine Safety, the inner notifier goroutine inside `RecordCommunityEventAndNotify` already uses `logging.GoSafe`, so the outer wrapper is redundant and should be removed entirely.) The loop needs to fetch `CommunityExperience` rows (currently `MarkExperienceInProcess` doesn't — add the same `s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.CommunityExperience{})` as in `CompleteExperience`). Set `Topic: &models.CommunityEvent_ExperienceId{ExperienceId: ...}`, `ActorId: authInfo.UserID`, `OccurredAtUnixSec: now`, no `UndoData` (client-only classification). On per-community emit failure, log via the parent logger with `WarnContext(ctx, "...", "community_id", ce.CommunityId, "error", err)` and `continue`.
  - In `CompleteExperience` (Option C, recommended): delete the `logging.GoSafe(...)` wrapper plus the `notifyRSVPedUsers` call inside it at line 357 — entire block removed. The existing `EXPERIENCE_COMPLETED` event-emission loop at lines 313–344 stays as-is (broadcast, no behavior change).
    - Option A variant (only if reviewer flips the recommendation): add the new RSVPer recipient case to the `EXPERIENCE_COMPLETED` resolver as in Option A above; otherwise leave the broadcast resolver alone.
  - In `CancelExperience` (around line 553), same pattern as `MarkExperienceInProcess`: delete the `logging.GoSafe(...)` wrapper, hoist the `CommunityExperience` fetch (currently at line 559) up to before the new event-emission loop, then emit one `COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED` event per community via `RecordCommunityEventAndNotify`. Re-use the same fetched `communityExperiences` slice for the existing `Archived = true` loop further down so we don't query twice.
  - Delete `notifyRSVPedUsers` (lines 727–793), `experienceCommunityIDs` (lines 799–814), `fetchExperienceRsvpPrefs` (lines 820–838), and `rsvpUserShouldBeNotified` (lines 844–859) along with any imports they uniquely required (`type2fun.org/ripls/server/community` is still used elsewhere, won't unblock).
- [ ] Edit `server/community/preferences_filter_test.go` if it covers the `notifyRSVPedUsers` path specifically (skim for `notifyRSVP` references — current grep shows none, but verify).
- [ ] `npm run build` and `go build -tags=integration ./...` — confirm no orphaned references.

**Validates:** `lifecycle.go` shrinks by ~130 lines; the only surviving experience-lifecycle notification path is `RecordCommunityEventAndNotify`. Stream-suppression and actor-exclusion now apply uniformly.

### Phase 3: Tests and docs

**Goal:** Integration tests assert the post-fix counts; workflow doc reflects the single funnel.

**Tasks:**
- [ ] Run `cd server && go test -tags=integration -timeout 20m -run 'TestExperience_Example1|TestExperience_Example2|TestExperience_Example7|TestExperience_Example8' ./integration_tests/...` once with the assertions tightened to a deliberately wrong number (e.g. `!= 0`) to capture actual notification counts and per-user breakdowns from the printed log output.
- [ ] Update the contract comments in `server/integration_tests/experience_test.go`:
  - `Postcondition_OwnerNotifiedOnRSVPs` (Example 1, ~line 311): drop the actor self-ping note; expected owner count drops from 5 to 3 (3 RSVP pings, no lifecycle self-ping).
  - `Postcondition_NotificationsForCancellation` (Example 2, ~line 497): expected total drops from 7 to 6 (no owner self-ping on cancel).
  - `Postcondition_NotificationCountMatchesContract` (Example 7, ~line 1645): expected total drops from 17 to 14 (no owner self-ping on either lifecycle event); update the breakdown comment to match.
  - Replace every "tracked in #1552" comment with "Funnel-normalized in PR #<this-pr>"; remove the relaxed-assertion phrasing — the new assertions should be exact equality, no "actor exclusion tracked" caveats.
- [ ] Skim Examples 3, 4, 5, 6, 8 for any other relaxed-assertion or `#1552`-marked TODOs that the fix unblocks.
- [ ] Grep `app/lib/` for the existing `EXPERIENCE_COMPLETED` payload-routing site (notification-tap deep-link handler and event-stream consumers, typically a `switch` on `CommunityEventPayload.eventType`). Add cases for `COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED` and `COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED` so deep links work when the user taps the new push. Without these, the new pushes still arrive but tapping them falls through to a default handler.
- [ ] Update `docs/workflows/experience.md`:
  - Operation 6 ("Mark Experience In Process", line 250–255): rewrite the Effect line to describe `EXPERIENCE_STARTED` event emission per shared community plus stream-suppression and the `notify_experience_rsvps` gate; drop "Sends a push notification to every Yes/Maybe RSVP" boilerplate.
  - Operation 7 ("Complete Experience", line 257–263): under Option C, rewrite to describe one `EXPERIENCE_COMPLETED` event per shared community (broadcast, gated by `notify_experience_completed`), removed the separate `notifyRSVPedUsers` push line. Under Option A, rewrite to describe `EXPERIENCE_COMPLETED` as RSVPer-only and gated by `notify_experience_rsvps`.
  - Operation 8 ("Cancel Experience", line 265–270): describe `EXPERIENCE_CANCELLED` event per shared community.
  - Notifications section table (around line 681): update the lifecycle rows to reflect the new event types and gates; under Option C, remove the line that says "Sends a push to every Yes/Maybe RSVP gated by `notify_experience_rsvps`" from the Complete Experience row.
  - Cross-link this PR/issue at the top of the Notifications section so a future reader knows when this consolidation landed.
- [ ] If Option A is selected, also update:
  - `proto/ripls/models/community.proto` comments on `NOTIFICATION_CATEGORY_EXPERIENCE_COMPLETED` and field `notify_experience_completed` to mark them deprecated. Do *not* delete the field (wire compatibility); add a `// Deprecated:` prefix per buf style.
  - `app/lib/presentation/screens/communities/manage_notifications_screen.dart:157` and `app/lib/presentation/viewmodels/community_notifications_view_model.dart` — hide or remove the toggle. ARB strings in `app/lib/l10n/app_*.arb`.
- [ ] `npm run build`, `go test -tags=integration -timeout 20m ./...`, `cd app && flutter test --reporter github`.

**Validates:** Tests assert exact counts; CI fails red on regression. Workflow doc is the contract; the code matches.

### Phase 4: Verify and clean up

**Goal:** Confirm nothing else relies on the deleted helpers; update any stale references to `notifyRSVPedUsers`.

**Tasks:**
- [ ] `grep -rn "notifyRSVPedUsers\|experienceCommunityIDs\|fetchExperienceRsvpPrefs\|rsvpUserShouldBeNotified" .` — should return zero hits outside historical commits.
- [ ] `grep -rn "#1552" .` — should return zero hits in `server/`, `app/`, `proto/`, `docs/`. The references in `experience_test.go` from #1533 should all be cleaned in Phase 3.
- [ ] If Option C selected: tighten the `NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS` comment in `proto/ripls/models/community.proto:332–334` to drop "completed" from the lifecycle list; regenerate.
- [ ] Final `npm run build` + integration tests + Flutter tests.
- [ ] Push and update PR.

**Validates:** No tech debt left behind; the doc/test/code triangle agrees.

## Related Issues

- **#1107** — Per-community notification preferences. Closed by PR #1553. Established `notify_experience_rsvps` and the dual-path lifecycle-ping implementation that this issue retires.
- **#1533** — Tightened the integration-test assertions to lock the *current* (pre-fix) counts. The contract comments in `experience_test.go` referencing #1552 by number anticipate the count drop this issue ships.
- **#1226** — Recent schema consolidation in the experience domain; touched the same files but doesn't intersect with notification handling.
- **#1104** — Direct-completion-from-JOINED flow (Example 8). Worth confirming Example 8's notification count assertions stay green after the count drop.

## Audit Notes

Audited against `docs/server/architecture.md`, `docs/server/observability.md`, `docs/client/architecture.md`, `docs/client/i18n.md`, `docs/client/testing.md`, `docs/proto_conventions.md`, and CLAUDE.md. Findings folded into the relevant phases; cross-cutting notes below.

- **Goroutine safety (CLAUDE.md / docs/server/architecture.md §Goroutine Safety).** The plan replaces three `logging.GoSafe` blocks (one per lifecycle hook) with synchronous `RecordCommunityEventAndNotify` calls. That is correct: `RecordCommunityEventAndNotify` itself spawns a `logging.GoSafe`-wrapped notifier goroutine internally (`server/community/events.go:76–84`), so each call site stays panic-safe without an outer `GoSafe`. Phase 2 should explicitly delete the outer `GoSafe` at lines 82, 357, 553 — not just the inner `notifyRSVPedUsers` body. (Updated Phase 2 task wording.)

- **Structured logging (docs/server/observability.md §Service Patterns).** New event-emission failures inside the loops must use the same `logger.WarnContext(ctx, "...", "community_id", ce.CommunityId, "error", err)` shape that the existing `EXPERIENCE_COMPLETED` loop uses (`lifecycle.go:335–340`). `operation`, `experience_id`, `actor_id`, and `event_type` fields should already be on the parent logger via `logging.LoggerWithContext(ctx).With(...)` at the top of each handler. Verify no new direct `log.Printf` / `fmt.Printf` calls slip in.

- **Storage / SQL efficiency (CLAUDE.md §SQL Efficiency).** The new `getNotificationRecipients` case runs one `QueryByField("experience_id", …, &models.ExperienceRSVP{})` per emitted event. With N communities sharing the experience, the lifecycle hooks emit N events and therefore run N identical RSVP queries. Same pattern the existing planning-events case already uses (`notifications.go:243`), so it isn't a regression — but worth flagging for a future "memoize per-request" optimization. Not in scope here; the funnel resolves recipients per-event by design. **Do not** introduce a manual cache.

- **Proto conventions (CLAUDE.md §Proto Comments + docs/proto_conventions.md).** The new enum comment block must use the capability ("Lifecycle transitions on an Experience…"), not vendor or transport language. The reference to `notify_experience_rsvps` in the comment is fine — that is a domain field name, not transport plumbing. Do **not** mention `RecordCommunityEventAndNotify`, `streamChecker`, or `notifyTargetedUsers` in proto comments; those are server-side mechanics. Phase 1 task wording updated.

- **Undo registry coverage (CLAUDE.md §Adding New CommunityEventType Values).** Both new types map cleanly to `ClassificationClientOnly`. Plan rationale ("inverse is the same lifecycle hook in reverse") needs to be specific: for `EXPERIENCE_STARTED`, the natural inverse is the `CompleteExperience` or `CancelExperience` RPC (no `Unstart` action exists; if the owner needs to "rewind" they can cancel and recreate). For `EXPERIENCE_CANCELLED`, the natural inverse is creating a new experience — there is no `UncancelExperience`. Phase 1 task should record those explicit inverses in the rationale; vague language ("reverse hook") risks failing the registry-test premise that classifications carry domain meaning.

- **Client architecture (docs/client/architecture.md §Repository Pattern + i18n.md).** Option A would touch the Flutter notification settings screen. The full client surface for Option A is: `app/lib/presentation/screens/communities/manage_notifications_screen.dart` (UI toggle row), `app/lib/presentation/viewmodels/community_notifications_view_model.dart` (Freezed state field, the `experienceCompleted` enum case, and the `unsetIsOn` mapper), the regenerated `*.freezed.dart` (run `dart run build_runner build`), repository `app/lib/data/repositories/` (no change — this view model already routes through `CommunityNotificationsRepository`), and ARB strings under `app/lib/l10n/app_en.arb` / `app_es.arb` for the row title/subtitle that gets removed. Accessibility labels on the removed row should be deleted alongside. Phase 3 task list updated. Under Option C (recommended), no Flutter work is required.

- **Server/client boundary (docs/proto_conventions.md).** New event types appear in `proto/ripls/models/community.proto` only. There is **no** API-side proto change required: clients consume `CommunityEventType` via the existing `CommunityEvent` and `CommunityEventPayload` messages. The `EventType string` field in `CommunityEventPayload` carries the new enum's `.String()` (`"COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED"`), so client-side payload routing — typically a string-equality switch — needs to add cases. Phase 3 should grep `app/lib/` for the existing `EXPERIENCE_COMPLETED` payload-routing site and extend it; suspect locations are notification-tap deep-link handlers and event-stream consumers. (Added to Phase 3.)

- **Testing conventions (docs/client/testing.md and CLAUDE.md §Testing).** The integration-test changes follow the established pattern (`SetupTestDatabase` per test, `WaitForNotificationCount` polling, `t.Logf` debug output). No new mock generation needed — `notifications.Service` already has a mock used by these tests. The new `getNotificationRecipients` case warrants a unit test in `server/community/notifications_test.go` covering: actor exclusion, intention-filter (NO RSVPs excluded, YES/MAYBE included), and missing-experience-id warn-and-return-nil. Added to Phase 1 tasks.

- **File-size rule (CLAUDE.md).** `server/community/notifications.go` is 666 lines today; adding two `buildNotification` cases, two `shouldNotify` enum entries, two `CategoryFor` entries, and one `getNotificationRecipients` case adds ~25–30 lines. Stays well under the soft 1000-line preference. `server/services/experience/lifecycle.go` is 860 lines and *shrinks* by ~130 lines after `notifyRSVPedUsers` and helpers are deleted — net compliance improvement.

- **i18n / "Event, not Experience" rule (CLAUDE.md §UI Terminology).** Notification body strings ("Event has started", "Event has been cancelled") in `buildNotification` are user-visible and must use "Event". The plan's proposed copy already complies. Confirm the existing `EXPERIENCE_COMPLETED` body ("'<name>' wrapped up") uses no `Experience` literal — it does not, good.

- **Out-of-scope (deliberately):** Batching the per-community RSVP query into a single fetch, deduplicating recipients across multi-community experiences, and reworking the `notify_experience_completed` toggle UX. Each is an independently reviewable change. The `notify_experience_completed` toggle handling is conditional on Option A vs C selection (Open Questions).
