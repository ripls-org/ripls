# Single Player Mode: Remaining Work

**Date:** 2026-04-05
**Status:** Draft

---

## Summary

Single player mode (SPM) lets users retroactively log community activities with participants who may not have the app. Phases 1, 2, and 2.1 are complete — past-event experience creation, shadow users, and polish. What remains is Phase 3 from the implementation plan (giveaways/loans/requests) plus nine outstanding items from the feature doc (`docs/single_player_mode.md`).

This document consolidates all remaining work into a single prioritized plan.

---

## Problems

### P1: Participant resolution creates duplicate shadow users

`resolveParticipantsForCompletion()` searches registered members by name but does not search existing shadow users before creating a new one. Logging "morning run with Tom" twice creates two separate "Tom" shadow users. This pollutes the shadow user list and fragments activity history.

### P2: AI descriptions use invitational tone for past events

The `ExperienceFromTextPrompt` instructs the LLM to write "inviting, inclusive language" ("Join us for friendly basketball games!"). Past events get the same prompt, producing descriptions that read as future invitations rather than records of what happened.

### P3: Extracted time not surfaced to client in creation flow

`GenExperience` already extracts date and time inline from the prompt — `ExperienceGeneration` has `Date` (YYYY-MM-DD), `Time` (HH:MM), and `TimeConfidence` (EXPLICIT/INFERRED/UNKNOWN) fields. The server computes `extracted_time_unix_sec` from those fields and returns it in `GenExperienceResponse`. However, `suggested_time` is hardcoded to TBD in the response regardless, so the client ignores the extracted value and the user must set the time manually. The fix is to wire the existing extracted time into `suggested_time` and have the preview modal react to it.

`ConvertInformalTime` is a separate RPC used only by the manual time picker modal (calendar chips) for free-text user input — it is not the right tool for the `GenExperience` flow.

Giveaway and loan `Gen*` calls don't extract time at all. They should follow the same pattern as experiences — add inline time extraction to their LLM prompts and output schemas.

### P4: Impact metrics don't update as attendees change

The completion modal snapshots `baseCount = yesRsvps.length` at load time. Adding/removing shadow attendees does not recompute the scale factor. Division by zero is possible when all attendees are removed.

### P5: Shadow users have no profile or per-person metrics

Tapping a shadow user's name anywhere does nothing. There is no way to see a shadow user's cumulative participation or impact. Community metrics show no per-person breakdown that includes shadow users.

### P6: Shadow users not shown in share modal

Shadow user invite links are only accessible via Manage Members. The share modal for an experience does not list shadow attendees or offer "Send invite link" actions.

### P7: No visual distinction between shadow and real users

Attendance lists, completion stories, and attendee sheets render shadow users identically to registered users. There is no indication that a person hasn't joined the app.

### P8: Giveaways don't support past-tense creation or shadow recipients

Giveaways require a registered recipient and follow the forward-facing interest/selection state machine. There is no way to log a giveaway that already happened or reference a non-member recipient.

### P9: Loans don't support past-tense creation or shadow borrowers

Same as P8 for loans. No way to log a loan that already started or already returned, and no shadow user support for the borrower field.

### P10: Requests don't support retroactive logging

Lower priority. No way to back-fill a request that was fulfilled offline or reference a shadow requester.

### P11: Editing existing activities after the fact

Raised in the original issue — add participants, correct location, update description on an already-completed activity. Separate workstream but shares completion modal and shadow user infrastructure.

---

## Open Questions

### Q1: Should time extraction happen inline in Gen* or as a separate call?

**Decision: Inline, same as the existing experience pattern.** `GenExperience` already extracts date, time, and confidence in a single LLM call — the `ExperienceGeneration` struct has `Date`, `Time`, `TimeConfidence` fields and the server already returns `extracted_time_unix_sec`. No second call to `ConvertInformalTime` is needed for experiences; the fix is just to wire the existing extracted value into `suggested_time`.

`ConvertInformalTime` is a separate RPC for the manual time picker modal (calendar chips). It is NOT part of the `Gen*` flow.

For giveaways and loans, the same inline approach applies: add `date`, `time`, `time_confidence` fields to their `Gen*` output schemas and update those prompts to extract temporal language. This keeps all item types consistent — one LLM call extracts content + location + time + metadata together — and avoids a second round-trip.

### Q2: Should past-tense giveaways/loans use the same creation modals or a separate flow?

**Decision:** Same modal, conditional behavior. Follow the experience pattern — when the user sets a time in the past, skip the state machine and go straight to confirmation. No separate "Log past activity" entry point. Keeps the mental model consistent and avoids a parallel UI to maintain.

### Q3: How should shadow user profiles surface invite prompts?

**Decision:** Always show an "Invite to Ripls" button on the profile sheet. Add a subtle highlight (e.g., a banner or badge) when the shadow user has 3+ activities, since they are strong invite candidates with compelling accumulated history. Feed nudges are a separate engagement feature and not coupled here.

### Q4: Should impact recalculation be client-side or server-side?

**Decision:** Client-side for now. Fix `baseCount` to recompute dynamically as attendees change and add a division-by-zero guard (default `scale` to `1.0`). Server-side re-estimation can come later if the scaling formula proves insufficient.

### Q5: How should the "edit existing activity" flow relate to SPM?

**Decision:** Separate workstream. Editing is a distinct user intent with its own edge cases (permissions, conflict resolution, notification of changes). It should be tracked as a separate issue that depends on SPM infrastructure but is planned and built independently.

---

## Recommendations

1. **Fix deduplication first (P1).** This is a data quality bug that gets worse with every use. Small change, high impact.
2. **Ship visual distinction (P7) early.** It's a UI-only change that makes shadow users comprehensible throughout the app and unblocks P5 and P6 naturally.
3. **Bundle giveaways and loans (P8, P9) together.** They share the same pattern: past-tense detection on transfers + shadow user as recipient/borrower. The proto and server changes are symmetric.
4. **Use inline time extraction in every `Gen*` call (Phase B + E).** Experiences already do this. Giveaways, loans, and requests should extend their `Gen*` output schemas with `date`/`time`/`time_confidence` — one LLM call extracts everything. `ConvertInformalTime` is for the manual time picker only.
5. **Defer requests (P10).** Lower priority and high complexity.
6. **Editing is a separate issue (P11).** Create a dedicated GitHub issue. Do not build as part of these phases.
7. **AI improvements (P2, P3) can ship independently.** They improve creation flow but don't block other work.

---

## Phase Implementation

### Phase A: Data quality & visual polish (small)

**Goal:** Fix the deduplication bug and make shadow users visually distinct everywhere they appear.

**Problems addressed:** P1, P7

**Tasks:**

*Flutter changes (no proto or server changes):*

- [ ] **P1 — Deduplicate shadow users during participant resolution:** In `gen_experience_view_model.dart`, update `resolveParticipantsForCompletion()`: after finding zero member matches for a name, call `shadowUserRepository.searchShadowUsers(communityId, query: name)` before calling `createShadowUser()`. If one shadow matches, add as pre-tagged attendee. If multiple match, add all as suggestions. Only create a new shadow user when zero shadows match.
- [ ] **P7 — Shadow user visual distinction:** Create a shared widget (e.g., `ShadowUserAvatar`) that renders a greyed-out avatar with initials and a small "Not on Ripls" badge. Use it in: attendance lists (`mark_completed_modal.dart`), attendee sheet (`attendee_list_sheet.dart`), story cards, and Manage Members screen. Add i18n key for the badge label.
- [ ] Add unit tests for the updated `resolveParticipantsForCompletion()` covering: single shadow match, multiple shadow matches, zero matches (new shadow created), mixed member + shadow results.
- [ ] Add widget tests for `ShadowUserAvatar` rendering.

---

### Phase B: AI improvements (medium)

**Goal:** Surface the time the LLM already extracts to the client, and generate retrospective descriptions for past events.

**Problems addressed:** P2, P3

**Background — how LLM extraction already works:**

The current architecture uses a single LLM call per `Gen*` RPC that returns content, location, metadata, and time together. For `GenExperience`, `ExperienceGeneration` already has `Date` (YYYY-MM-DD), `Time` (HH:MM), and `TimeConfidence` fields. `gen_ai.go` already computes `extracted_time_unix_sec` from these and returns it in `GenExperienceResponse`. The gap is that `suggested_time` is hardcoded to TBD, so the client ignores the extracted value.

`ConvertInformalTime` is a separate, unrelated RPC used by the manual time picker modal (calendar chips). It should not be called from `GenExperience` ViewModel or any other `Gen*` flow.

**Tasks:**

*Server changes:*

- [ ] **P3 — Wire extracted time into `suggested_time`:** In `server/services/experience/gen_ai.go`, replace the hardcoded TBD `suggestedTime` with a `SpecificTime` built from `extractedTimeUnixSec` when it is non-zero and `timeConfidence` is EXPLICIT or INFERRED. Keep TBD when `timeConfidence` is UNKNOWN or the date string couldn't be parsed.
- [ ] **P2 — Retrospective prompt for past events:** Create `ExperienceFromTextPromptPast` in `server/services/experience/prompts.go` — past-tense language, framed as a record ("We hiked the ridge trail together") not an invitation. In `gen_ai.go`, select the prompt variant when `extractedTimeUnixSec` is non-zero and in the past.
- [ ] Add unit tests for prompt selection (past vs. future extracted time)
- [ ] Add unit tests for `suggested_time` construction from extracted fields
- [ ] Run `npm run build`

*Flutter changes:*

- [ ] **P3 — Client reacts to `suggested_time`:** In `GenExperienceViewModel`, when the `GenExperienceResponse` contains a non-TBD `suggested_time`, pre-populate the time picker in the preview modal. Show a subtle indicator ("Suggested from your description") so the user knows to confirm.
- [ ] Add i18n strings for time suggestion UI.
- [ ] Add ViewModel unit tests: `suggested_time` present + past, present + future, absent (TBD), low confidence (UNKNOWN → no pre-fill).

---

### Phase C: Impact metrics fix (small)

**Goal:** Fix the completion modal so impact metrics stay accurate as attendees are added/removed.

**Problems addressed:** P4

**Tasks:**

*Flutter changes only:*

- [ ] **Recompute `baseCount` dynamically:** In the completion modal's impact calculation, derive `baseCount` from the current attendee list length rather than snapshotting at modal open. Recalculate whenever attendees are added or removed.
- [ ] **Division-by-zero guard:** When `baseCount == 0`, default `scale` to `1.0` (a one-person activity still has full impact).
- [ ] **Reactive UI:** Ensure the impact display widgets rebuild when the attendee list changes (verify `ref.watch` or equivalent triggers a rebuild on list mutation).
- [ ] Add unit tests: zero attendees (scale = 1.0), add attendee (scale increases), remove all added attendees (returns to baseline), rapid add/remove cycles.

---

### Phase D: Shadow user profiles & share modal (medium)

**Goal:** Give shadow users a profile page and surface invite links contextually in the share modal.

**Problems addressed:** P5, P6

**Tasks:**

*Proto changes:*

- [ ] Add `GetShadowUserActivities` RPC to `proto/ripls/api/community_service.proto` — returns list of experiences the shadow user participated in, plus aggregated impact metrics. Dedicated request (shadow_user_id, community_id) and response types.
- [ ] Run `npm run generate`

*Server changes:*

- [ ] Implement `GetShadowUserActivities` in `server/services/community/shadow_users.go` — JOIN across attendance/RSVP records to get experiences, aggregate impact. Use `LIMIT` on results.
- [ ] Add unit tests for the new RPC.
- [ ] Run `npm run build`

*Flutter changes:*

- [ ] **P5 — Shadow user profile sheet:** Create `ShadowUserProfileSheet` (modal bottom sheet). Shows: name, avatar (using `ShadowUserAvatar` from Phase A), list of activities with dates, cumulative impact metrics, "Invite to Ripls" button (generates/resends invite link). Data loaded via `ShadowUserRepository` and new activities method.
- [ ] Wire up tap handlers on shadow user names/avatars throughout the app to open the profile sheet.
- [ ] **P6 — Share modal shadow users:** In the share modal, append shadow attendees of the current experience below registered members. Show "Send invite link" action instead of RSVP invite. Use `ShadowUserRepository` for data.
- [ ] Highlight shadow users with 3+ activities with a subtle visual indicator encouraging the owner to invite them.
- [ ] Add i18n strings for profile sheet and share modal shadow user labels.
- [ ] Add ViewModel unit tests for profile data loading and share modal shadow user display.

---

### Phase E: Giveaways & loans (large)

**Goal:** Extend single player mode to giveaways and loans — retroactive logging with shadow user recipients/borrowers.

**Problems addressed:** P8, P9

**Tasks:**

*Proto changes:*

- [ ] Add `optional int64 completed_at_unix_sec` to `CreateTransferRequest` in `proto/ripls/api/transfer_service.proto` — when set, the transfer was completed at this time (past-tense creation). No `is_past_event` bool; same pattern as experiences where the server checks if the time is in the past.
- [ ] Add `optional string shadow_user_id` as an alternative to `recipient_user_id` in transfer creation request types (giveaway recipient, loan borrower).
- [ ] For loans: add `optional int64 returned_at_unix_sec` to allow logging an already-returned loan in a single step.
- [ ] Run `npm run generate`

*Server changes:*

- [ ] **Past giveaways:** In the transfer service handler, when `completed_at_unix_sec` is set and in the past, create transfer in COMPLETED state. Skip interest/selection phase. Set recipient (real or shadow user). Generate completion story synchronously.
- [ ] **Past loans:** When `completed_at_unix_sec` is set and in the past, create loan with `start_date` set to the past time. If `returned_at_unix_sec` is also set, mark as RETURNED immediately.
- [ ] **Shadow recipients/borrowers:** Update transfer creation to accept `shadow_user_id` in place of `recipient_user_id`. Update the shadow merge logic in `server/shadow/merge.go` to also migrate transfer records (both as giver and recipient/borrower).
- [ ] Add unit tests for past-tense giveaway creation (with real user, with shadow user)
- [ ] Add unit tests for past-tense loan creation (started, started+returned, with shadow borrower)
- [ ] Add unit tests for merge logic covering transfer records
- [ ] Run `npm run build`

*Flutter changes:*

- [ ] **Giveaway creation flow:** Update the giveaway creation modal to allow setting a past completion time. When past time is detected, change button label to "Mark Completed" (same pattern as experiences). Allow selecting a shadow user as recipient via the existing shadow user search/create pattern.
- [ ] **Loan creation flow:** Same as giveaway plus an optional "Already returned" toggle with return date picker when the loan start time is in the past.
- [ ] **Shadow user recipient picker:** Reuse the search-first bottom sheet pattern from the completion modal — search members and shadow users, create new shadow user if no match.
- [ ] **Time extraction for giveaways and loans:** The giveaway and loan `Gen*` LLM output schemas should be extended with `date`, `time`, and `time_confidence` fields (same pattern as `ExperienceGeneration`). The server populates `suggested_time` in the response. The client pre-populates the time picker from `suggested_time`, same as experiences. No separate `ConvertInformalTime` call needed.
- [ ] Add i18n strings for past-tense giveaway/loan UI: button labels, confirmation text, shadow recipient labels.
- [ ] Add ViewModel unit tests for past-tense giveaway and loan creation flows.
- [ ] Add disposal safety tests for any new async Notifiers.

---

### Phase F: Requests (deferred)

**Goal:** Retroactive request logging — back-fill a request that was fulfilled offline, with shadow requester support. Defer until Phases A–E are validated.

**Problems addressed:** P10

**Tasks (high-level — detailed planning deferred):**

- [ ] **Past requests:** Allow back-filling a request that was fulfilled offline. Shadow requester support. Lower priority per original issue. Time extraction should follow the same inline `Gen*` pattern — add `date`, `time`, `time_confidence` to `RequestGeneration` output schema if not already present.

---

### Editing existing activities (separate issue)

P11 (edit after the fact) is a distinct workstream. It shares completion modal and shadow user picker components but has separate entry points, a different permissions model, and notification requirements. **Track as a new GitHub issue** that depends on SPM infrastructure. Do not build as part of these phases.

---

## Phase Dependency Graph

```
Phase A (dedup + visual)
  └─► Phase D (profiles + share modal — needs ShadowUserAvatar from A)

Phase B (AI improvements — wire suggested_time for experiences)
  (Phase E + F extend the same inline pattern to their own Gen* calls independently)

Phase C (impact metrics) — independent

Phase D (profiles + share modal)
  └─► Phase E (giveaways + loans — shadow user infrastructure validated)

Phase E (giveaways + loans)
  └─► Phase F (requests — deferred)

P11 (editing) — separate issue, not in these phases
```

Phases A, B, and C can run in parallel. Phase D depends on A. Phase E depends on D (shadow user infrastructure must be validated). Phase F is deferred and depends on E. The time extraction pattern is per-phase and additive — B fixes experiences, E extends the same pattern to giveaways/loans, F extends it to requests.
