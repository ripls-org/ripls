# Issue #2690: Completing a cross-community event: RecordAttendance 404s for RSVPs from other communities

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2690
**Filed:** 2026-07-09 | **Last activity:** 2026-07-09
**Labels:** bug | **Assignees:** unassigned
**Context docs:** `docs/workflows/experience.md` (attendance tracking + cross-community sharing, §7 Recording Attendance), `docs/server/conventions.md` (fail-fast / error handling), `docs/proto_conventions.md` (API vs. models separation), memory `project-2684-marketing-sizzle-videos` (where the bug was found and worked around)

> **Status: IMPLEMENTED** on branch `fix_rsvp` (Option A). Server fix + flipped
> test + new N+1-guarded unit test + end-to-end integration test
> (`TestExperience_CrossCommunityCompletion`) + doc + walkthrough-comment updates
> all landed and green (`npm run build` + `go test` on
> `services/experience` and `integration_tests`). Phase 2's optional reel-seeding
> restructure was **not** done (it would require a full reel re-run to validate
> safely); the cross-community path is instead regression-guarded by the new
> integration test, and the reel's stale constraint comment was corrected.

> **UI-investigate skill: intentionally skipped.** The `/ripls-issue` protocol
> asks for a hermetic web reproduction of UI-observable bugs to bisect them to a
> layer. Here the failure is *already* bisected to the **server** layer with
> certainty: the issue carries the exact server log signature, and the code path
> that emits the 404 (`server/services/experience/rsvp.go` `RecordAttendance`) is
> read in full below. There is already a deterministic Go reproduction —
> `TestService_RecordAttendance_CrossCommunity` — that today *asserts the buggy
> 404*. A web repro would re-confirm a server 404 reproducible with `go test`,
> adding cost without changing the diagnosis.

## Problem Statement

`RecordAttendance` scopes its per-attendee RSVP lookup to a **single** community
(`req.community_id`), but the completion modal's attendee roster is
**cross-community**. When an attendee's RSVP lives in a different community than
the one the completion request carries, the lookup misses, a fallback query
discovers the RSVP "in the wrong community," and the handler **returns 404 for
the whole batch** — aborting completion for *everyone*, not just the mismatched
attendee.

Concretely (verified by reading the code):

1. The host's completion modal (`MarkCompletedModal` → `EventModalNotifier`)
   loads its roster from `GetExperienceDetails` → `buildRSVPs`, which is
   **deliberately not community-scoped**: "who's going is a property of the
   event, not the community lens" (`service.go:233-321`). One deduped RSVP per
   user across every shared community. The API `RSVP` message carries **no**
   `community_id`, so the client cannot know which community an attendee's RSVP
   lives in.
2. On confirm, `wrapUp()` calls `recordAttendance` with a **single**
   `communityId = state.experience.communityId` — the community *lens* the host
   opened the event from (typically the event's ad-hoc origin community)
   (`event_modal_view_model.dart:856-864`).
3. For each registered attendee, the server queries
   `(experience_id, user_id, community_id)` (`rsvp.go:398-406`). For an attendee
   whose only RSVP is in community **C** (they RSVP'd via a real community the
   event was shared to), this returns **zero rows** when the request carries the
   ad-hoc community **A**.
4. The fallback then queries `(experience_id, user_id)` across all communities
   (`rsvp.go:436-452`). It finds the C-row, decides "wrong community," and
   **returns `CodeNotFound`** — the whole call fails. The modal shows "The
   requested item could not be found." and the event never completes. Because
   every attendee is pre-confirmed in the modal, the host has no hint which
   attendee is the problem.

**Why the attendee's RSVP is only in C (not also A):** the #2548 promotion
(`ensureRSVPerInOriginCommunity`, `member_management.go:205`) adds a yes/maybe
RSVPer to the origin community as a **member** (`CommunityUser` row) but does
**not** create an `ExperienceRSVP` row there. And `RSVPToExperience` enforces
**one RSVP row per user per experience** — a later RSVP in a second community
updates the original row's intention without moving its `community_id`
(`rsvp.go:110-163`). So the RSVP stays in C; the completion request scoped to A
can never find it.

The failure is **symmetric**: whichever single community the request carries,
attendees whose RSVPs live in *other* shared communities 404. The
walkthrough/sizzle reel dodges it by having all cast join via the share link so
every RSVP lands in the ad-hoc community (`joinAndRsvpYes`). Real cross-community
events hit this in production.

## Context from Comments

**The issue has no comments.** It is a clean, well-scoped bug report authored
during the #2684 walkthrough-video work (PR #2689), and it already contains the
server log signature, the exact repro shape, the prod-risk assessment, and two
candidate fix directions. No further input is needed before planning — the
report is unusually complete for a zero-comment issue.

## Root-cause summary (one line)

`RecordAttendance` treats attendance as **community-scoped** for the RSVP lookup,
while the roster, the impact attendee count (`lifecycle.go:196-208`, no community
filter), and the product's stated model all treat attendance as **event-scoped**.
The lookup scoping is the defect.

## Potential Solutions

### Option A: Resolve registered attendees' RSVP event-scoped, server-only (Recommended)

- **Approach:** In `RecordAttendance`, for a **registered** attendee, resolve the
  RSVP by `(experience_id, user_id)` across all communities (matching
  `buildRSVPs` dedup: most-recently-updated row wins) and set `Attended` on that
  row. Use `req.community_id` **only** when *creating* a brand-new RSVP for a
  completion-time participant who has no RSVP anywhere (the existing past-event
  add path). This deletes the fragile "scoped-miss → cross-community-check →
  404" branch (`rsvp.go:436-452`) entirely. **Provisional** attendees keep their
  `(experience_id, provisional_user_id, community_id)` scoping — they are legitimately
  per-community and have no cross-community identity.
- **Pros:** Root-cause fix; aligns the lookup with the already-event-scoped
  roster and impact count; no proto change, no client change, no migration;
  small and surgical; per-user-one-RSVP invariant means "most recent" == "the
  row," so no double-counting risk. Directly matches the issue's first suggested
  direction.
- **Cons:** Drops the (previously intended) "wrong community_id" client-bug
  signal — but that guard was firing on *legitimate* cross-community completion,
  so it was net-harmful. Mitigate by logging (**DEBUG**, non-fatal) when the
  resolved RSVP's community differs from `req.community_id`, preserving
  observability without failing the request. DEBUG (not Warn) is the
  convention-correct level: cross-community completion is *normal* operation, not
  an unexpected/handled-failure — a Warn would misclassify it and spam the logs
  on every real cross-community event
  ([`server/conventions.md`](../server/conventions.md) → Structured Logging /
  Log levels).
- **Effort:** Small (server: ~1 function; flip 1 test; add 1–2 tests).

### Option B: Add per-attendee community/RSVP identity to the API + client

- **Approach:** Add `community_id` to `AttendanceRecord` (and surface a
  community on the API `RSVP`), have the client send each attendee's community so
  the server can scope precisely.
- **Pros:** Makes the per-attendee community explicit end-to-end.
- **Cons:** Requires proto changes on two messages, regenerated code, and client
  changes; the client roster today has **no** per-attendee community to send
  (it's a cross-community deduped list with no community field), so this also
  forces the roster to carry community — a meaningful surface expansion for zero
  product benefit. Violates "minimal dependencies / don't over-engineer." Solves
  a problem (per-community attendance) the product doesn't have.
- **Effort:** Medium–large (proto + gen + client + server + tests).

### Option C: Client fans attendance out per shared community

- **Approach:** Client calls `RecordAttendance` once per `sharedCommunityId`,
  each scoped, tolerating partial misses.
- **Pros:** No proto change.
- **Cons:** Client still doesn't know which community each attendee is in, so it
  must send every attendee to every community; the server must stop aborting on a
  miss anyway (so you *still* need the server change from Option A); N× the RPCs;
  brittle partial-success semantics. A client-side workaround for a server-side
  scoping bug — exactly the "fix the root cause, don't kludge" anti-pattern.
- **Effort:** Medium (and strictly worse than A).

## Recommendation

**Option A.** The bug is a server-side scoping mismatch: `RecordAttendance` is
the *only* place in the experience flow that scopes attendance to one community,
while the roster, the completion attendee count, and the product model are all
event-scoped. Bringing the registered-attendee lookup in line with them is the
minimal, root-cause fix — no proto, no client, no migration — and it makes the
existing `TestService_RecordAttendance_CrossCommunity` (which today locks in the
404) flip to asserting the correct success behavior. Keep provisional-user
lookups community-scoped (they are genuinely per-community). Add a non-fatal log
when the request's `community_id` differs from the resolved RSVP's community to
retain a signal for genuinely-stale clients.

## Open Questions

- **Fully event-scoped lookup, or scoped-first-with-fallback for registered
  users?** — Recommendation: **fully event-scoped** for registered users. Given
  the one-RSVP-per-user-per-experience invariant, `(experience_id, user_id)`
  returns at most one row, so a "scoped fast path" adds branching for no
  behavioral gain and re-introduces the very split that caused the bug. Collapse
  to a single event-scoped resolve; only fall back to `req.community_id` for the
  *create-new-RSVP* case.
- **Which community should a completion-time-added participant's new RSVP use?**
  — Recommendation: keep `req.community_id` (the origin/lens community), matching
  today's past-event participant-add behavior. No reason to change it; this path
  is unaffected by the bug.
- **Flip or delete `TestService_RecordAttendance_CrossCommunity`?** —
  Recommendation: **flip** it. It already sets up the exact bug scenario (RSVP in
  A, record with B); change its assertion from "expect 404 / `no RSVP found`" to
  "expect success, and the community-A RSVP row now has `Attended=YES` and shows
  as a past attendee in `GetExperienceStats`/`GetExperiencePeople` for community
  A." This keeps the valuable fixture and turns a bug-locking test into a
  regression guard.
- **Do we lose any real protection by removing the 404?** — Recommendation: no
  meaningful loss. The 404 was meant to catch a client sending a wrong
  `community_id`, but for registered users attendance is event-identified, so a
  "wrong" community is harmless. Replace the hard failure with a non-fatal log
  line for observability.
- **Should the walkthrough drop its workaround?** — Recommendation: yes, as a
  Phase 2 follow-up — have one cast member RSVP via a *named* shared community
  (not the share link) so the reel exercises the real cross-community completion
  path and would catch a regression. Track the memory note update too.

## Implementation Plan

### Phase 1: Server fix + unit regression
**Goal:** Cross-community completion succeeds; the batch no longer aborts.
**Tasks:**
- [ ] In `server/services/experience/rsvp.go` `RecordAttendance`, **fetch all
      experience RSVPs once** — `QueryByField(ctx, "experience_id", …)` — and
      resolve each attendance record in-memory instead of per-attendee
      `QueryByFields` inside the loop. This removes the current per-record N+1
      (`rsvp.go:398-406`, plus the second `anyRSVPs` query at `436-452`) in one
      move ([`server/conventions.md`](../server/conventions.md) → SQL Efficiency
      #1 "never query in a loop"). Index the fetched rows two ways: registered by
      `user_id` (event-scoped; if a user somehow has >1 row, pick the
      most-recently-updated, matching `buildRSVPs` dedup), provisional by
      `(provisional_user_id, community_id)`.
- [ ] For a **registered** attendee found in the index, set `Attended` on that
      row regardless of which community it lives in — deleting the `CodeNotFound`
      "wrong community" branch entirely. Keep the never-RSVP'd →
      create-in-`req.community_id` path (registered *and* provisional). Keep
      provisional resolution `(provisional_user_id, community_id)`-scoped.
- [ ] Add a non-fatal **DEBUG** log when the resolved RSVP's `community_id`
      differs from `req.community_id` (observability replacement for the removed
      404; DEBUG because cross-community is normal, not a failure).
- [ ] Preserve the existing guards unchanged: owner-only check
      (`rsvp.go:365-368`), `RequireActiveCommunity` soft-deleted-community reject
      (`372-376`), and the `EXPERIENCE_STATE_UNSPECIFIED` reject (`380-383`).
      **Do not** introduce any *new* silent skip: the cross-community attendee
      must be *found and updated*, never `continue`d past. (The pre-existing
      per-record `continue` on a transient storage error — which silently drops
      one attendee while the RPC still returns OK — is out of scope for this bug;
      note it as a known limitation / possible follow-up rather than expand
      scope here.)
- [ ] Flip `TestService_RecordAttendance_CrossCommunity`
      (`rsvp_attendance_test.go:164`) to assert success + `Attended=YES` on the
      community-A row (drop the `strings.Contains(..., "no RSVP found")` /
      `"not_found"` assertions; prune the now-unused `strings` import if no other
      test in the file uses it).
- [ ] Add a unit test asserting that a single `RecordAttendance` call scoped to
      one community correctly marks attendance for attendees whose RSVPs span
      *multiple* communities in one batch (the real modal shape), and that
      `GetExperienceStats` for each community reflects its own attendee. Wrap the
      call in `storage.AssertMaxQueries` to lock the query count to O(1) and
      prevent an N+1 regression ([`server/conventions.md`](../server/conventions.md)
      → SQL Efficiency #8).
- [ ] `npm run build` (golangci-lint + `go build ./...`) → `go test ./server/services/experience/...`.
**Validates:** The exact issue scenario now returns success; the batch-abort is
gone; per-community stats stay correct; the attendance loop is no longer N+1.

### Phase 2: End-to-end coverage + remove the walkthrough workaround
**Goal:** Prove the fix through the real completion flow and guard against
regression in the walkthrough reel.
**Tasks:**
- [ ] Add/extend a server integration test
      (`server/integration_tests/experience_completion_test.go`) that runs the
      full host flow: create (ad-hoc community) → share to named community C →
      member RSVPs YES **via C** → host `RecordAttendance` (scoped to ad-hoc A)
      then `CompleteExperience` → assert `COMPLETED`, attendee counted in impact,
      and story generated. This is the true reproduction of the reported flow.
- [ ] Update the walkthrough (`e2e/tests/walkthroughs/events.spec.ts`,
      `joinAndRsvpYes`) so at least one cast member RSVPs via a **named** shared
      community rather than the ad-hoc share link, exercising the cross-community
      completion path. Re-run `run_walkthrough.sh events` and QC the frames.
- [ ] Run `server/integration_tests` locally (creds via Secret Manager per the
      "server tests run locally" convention) — do **not** down-scope to CI-only.
**Validates:** The completion succeeds through the real RPC sequence and through
the Flutter Web bundle, not just the unit layer.

### Phase 3: Doc freshness + memory
**Goal:** Keep the durable docs and memory true to the fixed behavior.
**Tasks:**
- [ ] Update `docs/workflows/experience.md`: in §7 Recording Attendance (and the
      Record Attendance step at line ~321), state that attendance is resolved
      **event-scoped** across shared communities; add a "Cross-Community
      Completion" scenario (host records attendance for an attendee who RSVP'd
      via a different community → success). Advance the doc's `freshness:` stamp.
- [ ] If §7 drives a system test, run `/ripls-update-system-tests` so the system
      test matches the updated doc.
- [ ] Update memory `project-2684-marketing-sizzle-videos`: the "RSVPs must live
      in the ad-hoc community or completion's RecordAttendance 404s" gotcha is
      now **fixed** (#2690) — mark it resolved and point at this fix.
- [ ] Open the PR (feature branch → `gh pr create`); gate "green" on the terminal
      `statusCheckRollup`.
**Validates:** Docs, system tests, and memory reflect event-scoped attendance;
no stale "must live in ad-hoc community" guidance remains.

## Related Issues
- #2684 / PR #2689 — user-journey walkthrough work where this bug was found and
  worked around (the workaround this fix lets us remove).
- #2548 — origin-community RSVPer promotion (`ensureRSVPerInOriginCommunity`);
  explains why a cross-community attendee's RSVP stays in the named community
  rather than the ad-hoc one, which is the precondition for this bug.
- #1104 — prior `RecordAttendance` completion-flow bug (rejected JOINED state);
  same call site, same "client calls RecordAttendance then CompleteExperience"
  sequence — useful precedent for the completion-flow test shape.

## Audit notes

Audited against the documentation context map (`/ripls-audit`). Verdict:
**compliant after edits.** Adjustments applied to the plan:

1. **SQL efficiency (`server/conventions.md` → SQL Efficiency #1/#8).** The
   original Phase 1 kept the per-attendee `QueryByFields` loop (a pre-existing
   N+1). Since the fix already moves to an event-scoped resolve, Phase 1 now
   fetches all experience RSVPs **once** and resolves in-memory, removing both
   the per-record query and the second `anyRSVPs` query — and adds a
   `storage.AssertMaxQueries` guard to the new test to lock O(1) query count.
2. **Observability log level (`server/conventions.md` → Structured Logging).**
   The cross-community-mismatch log was specified as INFO/DEBUG; tightened to
   **DEBUG** with rationale — cross-community completion is normal operation, so
   Warn/Info would misclassify it and spam logs on every real cross-community
   event.
3. **Error handling (`error_handling_review_prompt.md` §1, §3).** Confirmed
   `CodeNotFound` is the *correct* code to remove (it was firing on legitimate
   input, not a real not-found). Added explicit "preserve these guards"
   (owner-only, active-community, state) and a "no new silent skip" constraint so
   the fix doesn't trade a fatal-abort for a silent drop; flagged the
   pre-existing transient-error `continue` as an out-of-scope known limitation.
4. **API design / minimal dependencies.** Confirmed the recommendation avoids
   proto/client surface expansion (Option B rejected) — aligns with "Never reuse
   RPC types" (n/a, no new types) and "minimal dependencies."

Consulted docs: `docs/server/conventions.md` (fail-fast, CodeInternal, SQL
efficiency, structured logging, testing), `docs/workflows/experience.md` (§7
Recording Attendance, cross-community sharing, attendance-then-complete
sequence), `docs/reviews/error_handling_review_prompt.md` (Areas to Review §1,
§3, §4), `docs/reviews/testing_review_prompt.md` (Areas to Review §1, §2),
`docs/reviews/observability_review_prompt.md` (Areas to Review §1). Not engaged:
security (no auth-model/secret/SSRF/PII change — ownership check untouched),
accessibility (no UI change), dependency (no new deps), performance beyond the
N+1 already addressed.
