# Issue #1533: [tech_debt] Experience integration test asserts known-bad notification count

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1533
**Filed:** 2026-04-28 | **Last activity:** 2026-04-29
**Labels:** auto-filed, review:tech_debt | **Assignees:** unassigned
**Context docs:** docs/workflows/experience.md, docs/issues/1107-per-community-notifications.md, docs/reviews/reports/tech_debt_review.md
**Related PR (already merged):** #1553 (closes #1107) — per-community notification preferences

## Problem Statement

`server/integration_tests/experience_test.go` carries two relaxed notification-count assertions and four TODO blocks calling out a "spurious-fan-out bug" in the experience-lifecycle pings:

- `Postcondition_OwnerNotifiedOnRSVPs` (lines 311–326) asserts `len(ownerNotifs) >= 3` instead of an exact count.
- `Postcondition_NoNotificationsForAttendanceRecording` (lines 1644–1656) asserts `len(notifLogs) >= 6` instead of an exact count.

Both TODOs cite `docs/workflows/experience.md` line 695–696 ("Experience started (IN_PROCESS) - no notification" / "Experience completed - no notification") as the contract and treat the surplus pings as a bug.

**That premise is no longer true.** PR #1553 (merged 2026-04-29, closing #1107) re-introduced these as intentional, user-gated notifications:

- `notifyRSVPedUsers` is still called from `MarkExperienceInProcess` (`lifecycle.go:82–83`), `CompleteExperience` (`:354–356`), and `CancelExperience` (`:549`), but every push is now filtered through `rsvpUserShouldBeNotified` against the per-(user, community) `notify_experience_rsvps` preference (default on). See `lifecycle.go:721–853`.
- `CompleteExperience` additionally emits an `EXPERIENCE_COMPLETED` `CommunityEvent` per shared community (`:311–343`), which the standard `notifyTargetedUsers` funnel broadcasts to all community members minus the actor, gated by `notify_experience_completed`.
- The PR #1553 description explicitly enumerates the lifecycle-RSVP notifications and notes that the legacy `notifyRSVPedUsers` path is tracked for refactor in **issue #1552** (move it onto `RecordCommunityEventAndNotify` so a single funnel handles preferences, stream-suppression, and payload shape). The path is not slated for *removal* — only for normalization.

The drift is therefore inverted from how the original review (and the now-stale plan revision) framed it: the docs and the test assertions are the lying parties; the code is the new contract.

Concretely:

- `docs/workflows/experience.md` line 253 (Mark Experience In Process Effect) and line 260 (Complete Experience Effect) still say "Sends notifications to all RSVPed users" without mentioning the preference gate or the new `EXPERIENCE_COMPLETED` broadcast.
- `docs/workflows/experience.md` lines 685–696 (Notifications table + "Events that do NOT trigger notifications") still claims IN_PROCESS and COMPLETED are silent. After #1553 they are not.
- `experience_test.go` TODOs still call the lifecycle pings spurious and the assertions still tolerate the surplus.

This issue's job is to land the docs+test reconciliation so the relaxed assertions go away and the contract documented in PR #1553 is the contract enforced by CI. The deeper `notifyRSVPedUsers` → `RecordCommunityEventAndNotify` refactor stays on **#1552**.

## Context from Comments

- **Triage (2026-04-28):** Set Type=Bug, Priority=P2; explicitly scoped this issue to "both fan-out fix and test cleanup". The "fan-out fix" framing predates #1553 — at filing time, suppression-at-source was the assumed fix.
- **2026-04-29 (initial plan):** Recommended Option A (suppress at source). Option A is no longer correct after the #1553 merge: removing the `notifyRSVPedUsers` calls would silently regress the user-visible feature documented in #1107 and break the new `notify_experience_rsvps` preference toggle.
- **2026-04-29 (this revision):** Maintainer asked to merge `origin/main` (which now contains #1553) and update the plan accordingly. Merge complete on this branch; this revision flips the recommendation to docs+tests reconciliation.

## Potential Solutions

### Option A: ~~Suppress at source~~ — **rejected post-#1553**

Removing the lifecycle `notifyRSVPedUsers` calls would regress the per-community `notify_experience_rsvps` toggle that PR #1553 just shipped (the toggle exists *because* these notifications fire). Listed only so reviewers don't re-propose it.

### Option B: Reconcile docs + tighten tests to the new contract — **recommended**

- **Approach:**
  - Keep all three `notifyRSVPedUsers` call sites and the `EXPERIENCE_COMPLETED` event emission in `lifecycle.go` exactly as they are after the merge.
  - Update `docs/workflows/experience.md` so the Notifications table and the per-operation Effect lines describe what the code actually does — including the per-community preference gate and the broadcast `EXPERIENCE_COMPLETED` event — and remove the stale "Events that do NOT trigger notifications" rows for IN_PROCESS / COMPLETED.
  - Tighten the two relaxed assertions in `experience_test.go` to exact equality, with counts computed from the post-#1553 model. Replace the spurious-fan-out TODOs with comments that point at the Notifications table as the contract and at #1552 as the tracking issue for the eventual funnel normalization.
  - Cross-link #1552 in the new doc copy so a future reader knows the legacy path is on the refactor radar but not blocking.
- **Pros:** Aligns with the design that just shipped. Minimal blast radius (docs + tests only). Future regressions to the lifecycle notifications fail CI immediately. Doesn't touch `lifecycle.go`, so #1552 can land cleanly on top.
- **Cons:** Computing the exact post-#1553 counts requires running the integration tests once locally to confirm the actor-handling specifics in `notifyRSVPedUsers` (which doesn't exclude the actor, unlike the standard funnel). The "off-by-one" the original plan flagged (4 instead of 3 per lifecycle event) is almost certainly the owner's auto-RSVP being included — see Open Questions.
- **Effort:** Small (≈ 8 doc-line edits, 2 assertion tightenings, 4 TODO removals, plus one local test run to lock the count).

### Option C: Refactor `notifyRSVPedUsers` onto the standard funnel as part of this issue

- **Approach:** Do #1552's work here too — delete `notifyRSVPedUsers`, add `EXPERIENCE_STARTED` / `EXPERIENCE_CANCELLED` event types, classify them in `server/undo/registry.go`, route the lifecycle pushes through `RecordCommunityEventAndNotify` with a recipient-resolver that returns Yes/Maybe RSVPs minus the actor. Then tighten tests.
- **Pros:** Single funnel, single filter, fixes the actor-self-notification off-by-one for free, makes the `notify_experience_rsvps` preference behave identically to every other category.
- **Cons:** That's #1552's scope and was explicitly spun out of #1107 review for a reason — it touches new enum values, an undo classification, and changes the wire payload shape clients see for these events. Doing it under #1533's tech-debt umbrella conflates a docs/test cleanup with a server refactor that needs its own review and an undo-registry decision. Also needs a `proto/ripls/api` change and a buf-lint pass.
- **Effort:** Medium-Large.

## Recommendation

**Option B.** The merge of #1553 turned this issue from "fan-out bug" into "stale assertions and stale docs". Option B closes the issue as filed (the test cleanup the reviewer asked for) without re-litigating the design that just landed. Option C is the right *eventual* direction but belongs on #1552 — keeping that scope separate preserves the merge story, lets #1552 land its undo-registry classification on its own merits, and avoids gluing two reviews together.

## Open Questions

- **What is the exact expected notification count for `TestExperience_Example7_RecordingAttendance` after the #1553 merge?** — Recommendation: run the test once locally during Phase 2 implementation and lock the actual count. Heuristic: 3 (share, broadcast `EXPERIENCE_CREATED`) + 3 (RSVPs to owner) + N₁ (`notifyRSVPedUsers` for IN_PROCESS — Yes/Maybe RSVPs in the experience) + N₂ (`notifyRSVPedUsers` for COMPLETED) + N₃ (`EXPERIENCE_COMPLETED` event broadcast to community members minus owner). N₁ and N₂ each likely equal the count of Yes/Maybe RSVPs (3 RSVPs A=YES, B=YES, C=MAYBE, possibly +1 if the owner has an auto-RSVP — see next bullet); N₃ likely equals 3 (community members A, B, C, since the owner is the actor). Don't hand-compute and pin a guess; let the test print the actual list and lock it.
- **Does `notifyRSVPedUsers` notify the owner along with the RSVPed users?** — Almost certainly yes (the original 14-vs-13 off-by-one points there). `lifecycle.go:746–753` does *not* exclude the actor before populating the candidate list, unlike `notifyTargetedUsers` which does (`notifications.go:298`). Recommendation: don't fix this in #1533 — it's a real but small inconsistency worth raising on #1552 (which will fix it for free by going through the standard funnel). For this issue, document the count that includes the owner self-ping and move on.
- **Should the in-app handlers key off the literal notification titles (`"Experience is now in progress"` / `"Experience has been completed"`)?** — Recommendation: this is no longer an open question for this issue (we're keeping the notifications). The relevant client behaviour is `CommunityEvent` payload routing, which doesn't depend on the title strings.

## Implementation Plan

### Phase 1: Reconcile workflow docs

**Goal:** `docs/workflows/experience.md` matches the post-#1553 reality.
**Tasks:**
- [ ] In `docs/workflows/experience.md`, rewrite the **"6. Mark Experience In Process"** Effect line (currently line 253: "Sends notifications to all RSVPed users") to spell out the actual behaviour: posts a `CHAT_SYSTEM_ACTION_STARTED` system message in the experience conversation, *and* sends a push notification to every Yes/Maybe RSVP whose `notify_experience_rsvps` toggle is on for at least one of the experience's shared communities. Match the wording style of the per-operation entries elsewhere in the file.
- [ ] Rewrite the **"7. Complete Experience"** Effect line (currently line 260) similarly: posts a `CHAT_SYSTEM_ACTION_COMPLETED` system message; emits an `EXPERIENCE_COMPLETED` `CommunityEvent` per shared community (broadcast to community members minus the actor, gated by `notify_experience_completed`); sends a separate push to every Yes/Maybe RSVP gated by `notify_experience_rsvps`; generates a story card per shared community.
- [ ] Confirm **"8. Cancel Experience"** Effect line (currently line 268) is accurate. The current wording — "Sends notifications to all RSVPed users" — needs the same preference-gate phrasing as the IN_PROCESS / COMPLETED entries.
- [ ] In the **Notifications** section (lines 681–698):
  - Replace the four-row table with the corresponding rows from PR #1553's PR description: Experience shared, RSVP Yes / Maybe, Experience started, Experience completed (RSVP push), Experience completed (community broadcast), Experience cancelled. Note the toggle category for each (`notify_new_experiences`, `notify_experience_rsvps`, `notify_experience_completed`).
  - Delete the "Experience started (IN_PROCESS) - no notification" and "Experience completed - no notification" bullets. Keep "RSVP No - no notification" — that one is still accurate.
  - Add a sentence under the table noting: every category is opt-out; the gate is per-(user, community); see `docs/issues/1107-per-community-notifications.md` for the design and `#1552` for the planned funnel normalization.
- [ ] Skim the Example walkthroughs (Example 1 around line 391; Example 7 around line 1633 of `experience_test.go` is the integration test, not the doc, but check the doc's Example 1 / Example 7 narrative blocks for any "Notifications sent:" bullet lists that explicitly include or exclude lifecycle pings, and bring them into line).

**Validates:** A reader of `docs/workflows/experience.md` learns the same notification model that PR #1553 shipped.

### Phase 2: Tighten the integration-test assertions

**Goal:** The two relaxed `>= N` checks become exact equalities, with TODOs replaced by contract comments. Lock the actual post-#1553 counts.

**Tasks:**
- [ ] Run `cd server && go test -tags=integration -timeout 20m -run 'TestExperience_Example1|TestExperience_Example7|TestExperience_Example8' ./integration_tests/...` once on this branch (after the merge but before any test edits) and capture the printed notification counts. Use the t.Logf output that the existing `if len(...) < N` branches already emit when a `t.Errorf` fires — temporarily flip the assertions to `!= 0` (or a deliberately impossible exact number) so the failure path prints the full list. Note the exact totals and per-user breakdowns.
- [ ] In `Postcondition_OwnerNotifiedOnRSVPs` (Example 1, lines 311–326): tighten to `len(ownerNotifs) != EXPECTED` where `EXPECTED` is the locked count. The current breakdown is "3 RSVPs + N from `notifyRSVPedUsers` × 2 events", and the owner is included in `notifyRSVPedUsers`'s candidate list whenever the owner has an RSVP record (see Open Questions). Strip the TODO block (lines 315–319) and rewrite the surrounding comment to read as the post-#1553 contract: "owner receives 3 RSVP notifications plus N lifecycle pings (`notifyRSVPedUsers` does not exclude the actor; tracked in #1552)." Leave the `WaitForNotificationCount` poll target updated to match.
- [ ] In `Postcondition_NoNotificationsForAttendanceRecording` (Example 7, lines 1633–1656): tighten to `len(notifLogs) != EXPECTED`. Strip the TODO blocks (lines 1638–1642 and 1648–1649). Rename the subtest if "NoNotificationsForAttendanceRecording" is now misleading — the actual postcondition is "exactly the documented set of pings, no more, no less"; pick something like `Postcondition_NotificationCountMatchesContract`. Rewrite the Per-docs comment block above the test to enumerate the contract: 3 share + 3 RSVPs to owner + (Yes/Maybe ×2) lifecycle pings + (community minus actor) `EXPERIENCE_COMPLETED` broadcast.
- [ ] Scan the rest of `experience_test.go` for any other `len(...) >= N` notification assertion or notification-related TODO that was relaxed under the same "lifecycle pings are spurious" assumption. The other `>=` / `<` checks at lines 499, 691, 842, 1430 reference different postconditions; verify each — if any of them was relaxed under the same TODO premise, tighten it too. The Example 8 test (`TestExperience_Example8_DirectCompletionFromJoined`) also exercises `CompleteExperience`; check whether it has a similar relaxed assertion and tighten if so.
- [ ] Re-run the targeted integration tests; both should now pass with the exact counts.

**Validates:** Future regressions to the lifecycle notification model — e.g. accidental removal during the #1552 refactor, or a missing `notify_experience_rsvps` gate — surface as red CI rather than as a permanently-relaxed assertion.

### Phase 3: Verify

**Goal:** Confirm the docs+tests change is self-consistent and doesn't break adjacent suites.
**Tasks:**
- [ ] `npm run build` (lint + go build).
- [ ] `cd server && go test -tags=integration -timeout 20m ./integration_tests/...` — full integration suite, not just the experience tests, since the chat and giveaway/loan/request integration tests also touch the notification machinery.
- [ ] `npm run lint:dart:size` and `flutter analyze --no-fatal-infos` — should be untouched by this change, but confirm.
- [ ] Push and update PR.

**Validates:** All tests green; the contract documented in `docs/workflows/experience.md` matches the assertions in `experience_test.go` matches the code in `lifecycle.go` after #1553.

## Related Issues

- **#1107** — Per-community notification preferences. Closed by PR #1553. Establishes the new notification model that this issue's docs+tests revision adopts.
- **#1552** — Normalize experience-lifecycle RSVP notifications onto the community-event funnel. Spun out of #1107 review. Will eventually delete `notifyRSVPedUsers`, add `EXPERIENCE_STARTED` / `EXPERIENCE_CANCELLED` event types, route through `RecordCommunityEventAndNotify`, and pick up actor-exclusion for free. Out of scope here; cross-linked from the new doc copy.
- **#1226** — Recent schema consolidation in the experience domain; touched the same area but doesn't intersect with notification handling.
- **#1104** — Referenced by `TestExperience_Example8_DirectCompletionFromJoined` (the "completion direct from JOINED" flow). Worth confirming Example 8 stays green after the assertion tightening since it also exercises `CompleteExperience`.

## Audit Notes

Audited against `docs/server/architecture.md` and the post-merge state of the codebase:

- **Architecture compliance:** Change is contained to `docs/workflows/experience.md` and `server/integration_tests/experience_test.go`. No source-code change in `server/services/experience/` or `server/community/`. No new dependencies.
- **Server/client boundary:** No proto changes. No client-visible payload changes. The Flutter `Manage Notifications` screen shipped in PR #1553 already exposes the relevant toggles; nothing else to wire.
- **Docs-as-contract:** This change's whole point is to make `docs/workflows/experience.md` agree with `lifecycle.go`. After Phase 1, the file is no longer self-contradictory.
- **Testing approach:** Phase 2 tightens existing integration assertions rather than adding new ones. Test isolation, container reuse, and `WaitForNotificationCount` polling are unchanged.
- **Out-of-scope (deliberately):** The `notifyRSVPedUsers` actor-exclusion off-by-one and the legacy-funnel split tracked in #1552. Mentioned in the new comments so a future reader has the breadcrumb.
