# Issue #2657: Flaky: notification-count integration tests over-count (N vs N+1) — WaitForNotificationCount wait-then-assert-exact race

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2657
**Filed:** 2026-07-06 | **Last activity:** 2026-07-06
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/server/architecture.md`, `docs/server/conventions.md` (§ Server Testing), `docs/testing/architecture.md`, `docs/testing/update_system_tests.md` (§ Notification Testing), `docs/push_notifications.md` (via map), `docs/workflows/experience.md` (notification contract), `docs/reviews/testing_review_prompt.md` (Areas to Review) — all auto-selected from `docs/llms.txt`; none passed as input.

> **UI investigation skipped:** this is a CI-only flake in the Go server
> integration suite (`server/integration_tests/`), with no UI-observable
> surface — nothing for the web bundle to reproduce. Investigation was done
> instead by pulling the failing CI job's per-notification dump (below),
> which is exactly the evidence the issue asked to capture.

## Problem Statement

The workflow notification-count integration tests intermittently fail in CI
with an over-count (`Expected 12 notifications per docs/workflows/experience.md
contract, got 13`). The issue as filed attributes this to a test-harness
timing race: `WaitForNotificationCount` (`server/integration_tests/test_helpers.go:842`)
returns as soon as the captured count reaches `>= N`, and the tests then
assert `== N`, leaving an unbounded window for a late N+1.

**The captured evidence shows the filed root cause is incomplete.** The issue
itself flagged that a failing run's per-notification dump should be captured
"to rule out a genuine duplicate" — this plan did that (CI run 28794731181,
job 85382112421), and the dump shows a **genuine duplicate push**, not a
harness misread:

```
Expected 12 notifications per docs/workflows/experience.md contract, got 13
  Notification: user=ab23986e-… title=New Event          ← duplicate (participant A)
  Notification: user=ab23986e-… title=New Event          ← duplicate (participant A)
  Notification: user=31c3f505-… title=New Event            (participant B)
  Notification: user=eb281270-… title=New Event            (participant C)
  Notification: user=3620d78c-… title=RSVP to …  ×3        (owner)
  Notification: user=…          title=Attendance Test Experience ×3
  Notification: user=…          title=Event Completed ×3
```

`LogCapture` only ever appends entries, so a read of 13 means 13 dispatch
lines were genuinely emitted. Participant A received the `EXPERIENCE_CREATED`
("New Event") push **twice**.

### Root cause of the duplicate (product bug)

Broadcast-notification recipients are resolved **at dispatch time against
live membership**, not at publish time:

1. `SaveExperience` shares the new experience to its **ad-hoc origin
   community** (`server/services/experience/save.go:275` →
   `shareExperienceToCommunity`), publishing `EXPERIENCE_CREATED` for the
   ad-hoc community. At publish time its only member is the owner, so the
   expected fan-out is zero (owner is the actor).
2. `ShareExperience` publishes a second `EXPERIENCE_CREATED` for the test
   community (`server/services/experience/share.go:278`). Expected fan-out:
   participants A, B, C.
3. Participant A RSVPs Yes. Per #2548, a Yes/Maybe RSVP **promotes the RSVPer
   into the ad-hoc origin community** (`server/services/experience/rsvp.go:185`,
   `ensureRSVPerInOriginCommunity`).
4. The notification subscriber is a single FIFO goroutine
   (`server/pubsub/memtopic.go`) doing several DB queries per event. Under CI
   load it lags behind the RPC stream. When it finally processes the ad-hoc
   event from step 1, `getNotificationRecipients`
   (`server/notifications/community_subscriber/recipients.go:299`) reads the
   ad-hoc community's **current** members — now `{owner, A}` — and pushes
   "New Event" to A.
5. It then processes the step-2 event and pushes to A, B, C. A gets two.

The dump's emission order (`A, A, B, C`) matches this exactly. The #2088
dedup gate cannot suppress it because the two events come from different
RPCs and therefore carry different `request_id`s
(`server/notifications/community_subscriber/dedup.go:95` keys on request_id).

This is not test-only: in production, anyone who joins a community (most
easily the ad-hoc origin community, via RSVP/interest/offer) in the window
between a broadcast event's publish and its dispatch receives a push for an
event that predates their membership. `CommunityUser.created_at_unix_sec`
is second-resolution, so a timestamp comparison cannot close the window
(share and RSVP routinely land in the same second).

### The harness defect (secondary, still real)

The filed harness critique stands, but inverted: "wait for `>= N`, assert
`== N`" doesn't just flake — it **masks** the product bug most of the time.
When the duplicate lands after the assert reads, the test passes silently;
CI only went red on the runs where the subscriber's lag happened to flush
the 13th line before the read. The exact-count contracts (intentionally
exact, per actor-exclusion/funnel dedup) are only as good as the harness's
ability to observe the settled total. Separately, several call sites use a
500 ms wait budget (`loan_test.go`, `giveaway_test.go`, most of
`request_test.go`) that a loaded CI runner can plausibly exceed, exposing
the same tests to under-count flakes.

## Context from Comments

The issue has no comments — it was filed today from the PR #2656 CI failure.
The issue body itself is thorough (root-cause hypothesis, scope, suggested
fixes, repro, done-when), and explicitly requests capturing a failing run's
notification dump before concluding it's harness-only. That dump has now
been captured and changes the recommended scope: the fix must include the
product-side duplicate, not just the harness.

## Potential Solutions

### Option A: Harness-only settle window (the issue's suggested fix #1)
- **Approach:** Rework `WaitForNotificationCount` to wait until the count is
  ≥ N **and stable** (no new notification for ~500–750 ms), with a larger
  overall timeout; keep exact asserts. No server changes.
- **Pros:** Small, test-only, no product risk. Preserves the exact-count
  contract and makes over-emission fail loudly instead of racing.
- **Cons:** Now that the dump proves a genuine intermittent duplicate, a
  settle window alone **converts a rare flake into a more frequent one** —
  it reliably observes the 13th push whenever the subscriber lags. CI gets
  worse until the product bug is fixed. Does nothing for users receiving
  duplicate pushes.
- **Effort:** Small.

### Option B: Fix the product race (publish-time recipient snapshot) + settle-window harness (recommended)
- **Approach:** Two coordinated changes:
  1. **Snapshot broadcast recipients at publish time.** The bus already
     hydrates denormalized entities onto the in-memory envelope at publish
     (`server/community_event_bus/prefetch.go`, called from
     `InProcessBus.Publish` before enqueue). Add a `MemberIDsAtPublish`
     field to `PublishedEvent`, populated in the same prefetch pass for the
     live-membership broadcast event types (`EXPERIENCE_CREATED`,
     `GEAR_SHARED`, `REQUEST_CREATED`, `INVITATION_LINK_USED`,
     `COMMUNITY_RESTORED`). `getNotificationRecipients` uses the snapshot
     when present, falling back to the live query (fail-open, WARN) when the
     prefetch failed — the same tolerance contract prefetch already
     documents. Targeted event types (owner, recipient, RSVP-set lookups)
     are unchanged; `COMMUNITY_DELETED` already snapshots via
     `deleted_snapshot`. In-memory only — nothing new is persisted on the
     `CommunityEvent` audit row.
  2. **Settle-window harness helper** as in Option A, applied to all 26
     `WaitForNotificationCount` call sites across the four workflow test
     files, with the 500 ms budgets raised to a uniform generous max (the
     helper returns as soon as the count settles, so a large max costs
     nothing on the happy path).
- **Pros:** Fixes the user-facing duplicate-push bug at its root — recipients
  are membership *as of the event*, which is also the semantically right
  contract (late joiners have their own entry surfaces: the invite link
  lands them on the item, an RSVP means they've seen it, and the feed
  covers catch-up). Immune to the same-second timestamp problem. Follows an
  established bus pattern. The hardened harness then *proves* the fix under
  `-count=20` and guards against future over-emission regressions of any
  cause.
- **Cons:** One extra members query per broadcast publish moves from the
  async subscriber into the RPC request path (small, indexed; the subscriber
  stops issuing the same query). Touches the bus envelope, so
  `docs/push_notifications.md` prose needs updating. Slightly larger diff
  than A.
- **Effort:** Medium.

### Option C: Suppress created-broadcast pushes for ad-hoc origin communities + settle window
- **Approach:** In the subscriber (or at publish), skip `EXPERIENCE_CREATED`
  / `GEAR_SHARED` / `REQUEST_CREATED` push dispatch entirely when the event's
  community is an ad-hoc (nameless per-item) community. Rationale: every
  ad-hoc member either created the item, was directly invited (and is
  notified through the invite channel), or joined *through* the item — a
  "New Event" push is redundant for all of them. Combine with the Option A
  settle window.
- **Pros:** Small server diff; eliminates this flake's specific vector and a
  class of redundant pushes; arguably a product improvement on its own.
- **Cons:** Doesn't fix the general dispatch-lag class — the same race exists
  for **named** communities (invite-link join during subscriber lag ⇒
  spurious "New Event"/"X joined" pushes), so the flake is narrowed, not
  closed. Encodes ad-hoc-specific product policy into the notification
  pipeline. Requires confirming no flow relies on the ad-hoc broadcast push
  today (none found: under normal timing the ad-hoc event fans out to
  nobody, which is why the contract counts 12).
- **Effort:** Small–medium.

**Not viable — for the record:**
- *Relax asserts to `>= N`*: rejected by the issue itself; the contracts are
  intentionally exact and `>=` would have hidden this real product bug —
  which is now the strongest argument for keeping them exact.
- *Timestamp filter* (`member.created_at_unix_sec <= event.occurred_at_unix_sec`):
  second-resolution timestamps can't discriminate same-second join-after-publish,
  which is precisely the failing case.
- *Dev-mode bus-drain endpoint*: `InProcessBus.Drain` /
  `MemTopic.Drain` already exist and would give a deterministic barrier, but
  the integration tests drive a separate server **process** over HTTP, so
  this needs a new dev-mode-only debug endpoint, and a small settle would
  still be required for stdout→pipe→`LogCapture` propagation. More moving
  parts than the settle window for strictly less generality; keep in the
  back pocket if the settle window ever proves insufficient.

## Recommendation

**Option B.** The dump upgraded this issue from "test flake" to "test flake
that intermittently observes a real duplicate-push product bug." Fixing only
the harness (A) makes CI redder; fixing only the ad-hoc vector (C) leaves the
general publish/dispatch race open. The snapshot fix is small, matches the
bus's existing prefetch pattern, states the right product semantics
(recipients = members at event time), and the settle-window harness then
both validates it and permanently converts this failure class from
"intermittent mystery" to "loud, dumped, exact-count failure."

Fix-root-cause over kludge is also the standing project preference.

## Open Questions

- **Should members who join between publish and dispatch receive broadcast
  pushes?** — Recommendation: no. A push about an event that predates your
  membership is noise; every join path already lands the user on (or past)
  the item in question, and the feed covers catch-up. This is the semantic
  justification for the snapshot, and it should be stated in
  `docs/push_notifications.md` as the recipient contract.
- **Snapshot scope: all five live-membership broadcast types, or just the
  three item-created types?** — Recommendation: all five
  (`EXPERIENCE_CREATED`, `GEAR_SHARED`, `REQUEST_CREATED`,
  `INVITATION_LINK_USED`, `COMMUNITY_RESTORED`). Same race, same fix, one
  code path; `INVITATION_LINK_USED` has an analogous absurdity (a brand-new
  joiner being told someone *else* joined before them).
- **Settle-window parameters?** — Recommendation: settle = 750 ms of no
  count growth after reaching ≥ N; overall max wait = 5 s at every call
  site (replacing the mixed 500 ms / 2 s budgets). The helper returns at
  N + settle on the happy path, so the suite only pays the big timeout when
  something is genuinely wrong. Tune down later if suite time matters.
- **Also take Option C's ad-hoc suppression as a follow-up product
  decision?** — Recommendation: file it separately as a product question
  (it changes notification behavior on its own merits, not to fix the race)
  and keep this fix mechanism-only. Don't bundle.
- **Is the `alpine:3` testcontainer image-pull flake in the same red run in
  scope?** — Recommendation: no; unrelated infra flake (`testing_test.go:137`),
  track separately if it recurs.

## Implementation Plan

### Phase 1: Pin the product bug with a deterministic regression test
**Goal:** Reproduce the duplicate deterministically at the subscriber layer, without CI-load luck.
**Tasks:**
- [x] Add a `community_subscriber` unit/module test that calls
      `Subscriber.Handle` **directly** with a constructed `PublishedEvent`
      (not via the bus — going through the bus would reintroduce the timing
      dependence): an `EXPERIENCE_CREATED` event for a community, with a new
      member added to the community *after* the envelope was built. Assert
      the late joiner receives **no** push. Fails against today's
      live-membership lookup, passes with the snapshot — fully deterministic.
      (`recipients_snapshot_test.go`)
- [x] Mirror one case for a second broadcast type (e.g. `GEAR_SHARED`) to
      lock the shared code path.
- [x] Add a publish-side test on `InProcessBus.Publish` (or the prefetch
      helper): the envelope's member snapshot is populated for the
      live-membership broadcast types and left nil for targeted types, and a
      snapshot-fetch failure leaves it nil without failing the publish.
      (`snapshot_test.go`)
- [x] ~~Confirm the existing `-count=20` repro and record the baseline.~~
      Skipped as a pre-fix step: the CI dump plus the deterministic
      subscriber-level tests already prove the mechanism, and the issue
      itself observed 3× local passes (the race needs CI-grade load). The
      `-count=20` run happens post-fix as Phase 3 stress validation instead.
**Validates:** The dispatch-time-membership diagnosis, independent of CI; gives the Phase 2 fix a red test to turn green.

### Phase 2: Publish-time recipient snapshot on the bus envelope
**Goal:** Recipients for live-membership broadcast events are fixed at publish time.
**Tasks:**
- [x] Add `MemberIDsAtPublish []string` to `cebus.PublishedEvent`; populate
      it via a `snapshotMembers` sibling of `prefetchEntities` called from
      `InProcessBus.Publish`, only for the five live-membership broadcast
      types (`cebus.SnapshotsMembers`). Fetch failure logs WARN and leaves
      it nil (fail-open, consistent with the prefetch contract).
      *Implementation note: the query is inlined in the bus rather than
      calling `community.GetCommunityMemberIDs` — `server/community`
      imports `community_event_bus` (provision.go), so the reverse import
      would cycle.*
- [x] Recipient resolution: new `resolveRecipients` wrapper prefers the
      snapshot (minus actor) when non-nil; a snapshotted type without one
      falls back to the live `getNotificationRecipients` query with a WARN
      so background/non-RPC publishers keep working. (`COMMUNITY_DELETED`
      keeps its `deleted_snapshot` path; targeted types unchanged.) The
      `INVITATION_LINK_USED` ordering guarantee (membership writes precede
      `Publish` in the same RPC) is documented on `snapshotMembers`.
- [x] Verify the community **stream** subscriber and feed service are
      unaffected (feed reads audit rows at query time; stream broadcast
      targets live connections by design — asymmetry documented in the
      package READMEs).
- [x] Query-count neutrality: the members query *moved* from the async
      subscriber (which now consumes the snapshot) into the publish path —
      one indexed read either way, no N+1 (single query per broadcast
      publish, same as the subscriber previously issued per dispatch).
- [x] Confirm no log line shapes change: the log-based SLI/SLO metrics stack
      (#1613, `docs/server/observability.md`) keys on existing structured
      fields (`operation`, `outcome`); the only new lines are the WARNs on
      snapshot-fetch failure and live-membership fallback.
- [x] Phase 1 tests go green; full `go test ./...` in `server/` passes.
**Validates:** The duplicate vector is closed at the root, with measured request-path cost.

### Phase 3: Settle-window harness + call-site sweep
**Goal:** The exact-count contract is observed against a settled total; over-emission fails loudly, under-count stops depending on 500 ms budgets.
**Tasks:**
- [x] Rework `WaitForNotificationCount(lc, count, timeout)` to: poll until
      `len(GetNotificationLogs()) >= count`, then keep polling until the
      count has been stable for a settle interval (750 ms,
      `notificationSettleWindow`), returning the settled count. All
      existing call sites discard the return value and re-read, so the
      bool→int change is transparent.
- [x] Sweep all 26 call sites in `experience_test.go`, `loan_test.go`,
      `giveaway_test.go`, `request_test.go`: uniform 5 s max wait; ensure
      every exact-count assert dumps the per-notification list on failure
      (added the one missing dump, `Postcondition_HelpersNewRequestOnly`).
- [x] Stress validation: `-count=20` on
      `TestExperience_Example7_RecordingAttendance` locally — 20/20 passes
      (93 s) with the settle window active. CI validates again on the PR.
**Validates:** Issue's "done when": settle window enforced in the shared helper, exact contracts preserved, flake gone under repetition.

### Phase 4: Documentation and follow-ups
**Goal:** Docs stay true to the code; adjacent decisions get their own tracking.
**Tasks:**
- [x] Update `docs/push_notifications.md`: recipient contract is "membership
      as of the event" for broadcast types; describe `MemberIDsAtPublish`
      and the fail-open fallback.
- [x] Update `docs/testing/update_system_tests.md` § Notification Testing:
      the verification-pattern snippet shows the settle-window helper and
      the failure-dump requirement.
- [x] Update the package READMEs that describe the touched surfaces:
      `server/community_event_bus/README.md` (envelope fields + publisher
      contract) and `server/notifications/community_subscriber/README.md`
      (recipient resolution) — both mention the publish-time member
      snapshot and its fail-open fallback.
- [x] File the Option C product question (suppress ad-hoc origin-community
      created-broadcast pushes entirely?) as a separate issue referencing
      this analysis → **#2662**.
- [ ] Close #2657 with a comment linking the CI dump, the root cause, and
      the fix PR (when the PR lands).
**Validates:** The nightly doc-freshness ratchet won't flag drift; the deliberately-unbundled product decision isn't lost.

## Audit Notes (ripls-audit)

Audited against the documentation context map. Changes applied by the audit:

- **Phase 1:** pinned the regression test to a direct `Subscriber.Handle`
  invocation (going through the bus would reintroduce the timing dependence
  the test exists to remove) and added a publish-side test for snapshot
  population + fail-open on fetch failure (testing rubric: error paths must
  be tested, not just the happy path).
- **Phase 2:** added the ordering constraint that makes the snapshot correct
  for `INVITATION_LINK_USED` (same-RPC membership writes precede `Publish`),
  and a check that no structured-log shapes consumed by the log-based
  SLI/SLO stack (#1613) change.
- **Phase 4:** added the two package READMEs that document the touched
  surfaces (`server/community_event_bus/README.md` describes the
  `PublishedEvent` envelope; `server/notifications/community_subscriber/README.md`
  describes recipient resolution) — the project's README rule requires them
  to stay accurate.

Docs consulted: `docs/llms.txt`; anchors `docs/server/architecture.md`
(§ Domain Event Buses), `docs/server/conventions.md` (§ Server Testing),
`docs/testing/architecture.md`; leaf docs `docs/testing/update_system_tests.md`
(§ Notification Testing, § Strict Alignment), `docs/push_notifications.md`
(§ Targeted Notifications, § Asynchronous Notification Sending, § Event Types);
review rubrics (Areas to Review) for `testing`, `error_handling`,
`performance`; package READMEs `server/community_event_bus/README.md`,
`server/notifications/community_subscriber/README.md`. Verified in code:
`server/integration_tests/test_helpers.go` (LogCapture/WaitForNotificationCount),
`server/notifications/community_subscriber/{subscriber,recipients,dedup}.go`,
`server/community_event_bus/{inprocess,prefetch}.go`, `server/pubsub/memtopic.go`
(FIFO worker, drop-on-full, `Drain`), `server/services/experience/{save,share,rsvp}.go`,
`proto/ripls/models/community.proto` (`CommunityUser.created_at_unix_sec`
second resolution). No proto/API changes are proposed, so the api_design
dimension is not engaged; no new dependencies; no auth/secrets/PII surface
changes beyond existing per-recipient logging.

## Related Issues
- #2656 — the PR whose CI run surfaced the flake (not causal; branch touches none of the involved code).
- #2088 — the request_id-scoped dedup gate; correctly out of reach here because the two `EXPERIENCE_CREATED` events come from different RPCs.
- #2548 — RSVP promotion into the ad-hoc origin community; the membership write that races the dispatch.
- #2526 — `CommunityService.ShareItem` / ad-hoc audience provisioning; the path that publishes the ad-hoc `EXPERIENCE_CREATED` event (via `SaveExperience` today).
- #2492 — phone-first pivot epic; origin of ad-hoc per-item communities and join-through-item flows that widen this race in production.
- #2658 — unrelated pre-existing mock race found recently; same "CI flake" family, different mechanism.
