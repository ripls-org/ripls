# Issue #2892: "sharing event" nudge is nonsense

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2892
**Filed:** 2026-08-10 | **Last activity:** 2026-08-10
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** [workshop.md](../workshop.md), [nudges.md](../nudges.md),
[client/inbox.md](../client/inbox.md),
[server/architecture.md](../server/architecture.md),
[server/conventions.md](../server/conventions.md),
[server/observability.md](../server/observability.md),
[proto_conventions.md](../proto_conventions.md),
[testing/architecture.md](../testing/architecture.md),
[client/conventions.md](../client/conventions.md),
[users/priorities.md](../users/priorities.md), plus the review prompts for the
engaged dimensions under [reviews/](../reviews/) — see *Audit notes* at the
foot of this doc for the full list and what the audit changed.

## Problem Statement

The Home tab's zero-state hero rendered this card in production (v0.29.1, Android):

> **Share your 12-Piece Metric Ratcheting with the circle**
> *Create a sharing event so everyone can borrow it at once.*
> `[ Set one up ]`

Every line of it is wrong, and each in a different way:

1. **The headline asks for something already done.** The card only exists
   *because* a `CommunityGear` row joins that gear to that circle — i.e. the
   gear is already shared. The nudge asks the host to do the thing that
   triggered the nudge.
2. **The headline names a mutilated object.** `shortName`
   ([esm_repeat_signal_detector.go:251](../../server/services/workshop/momentum/esm_repeat_signal_detector.go#L251))
   clips to the first three words. That is fine for an experience name
   ("Sunday morning brunch"); on a product name it amputates the head noun —
   "12-Piece Metric Ratcheting Wrench Set" → "12-Piece Metric Ratcheting".
3. **The description proposes an object that does not exist and could not
   work.** There is no "sharing event" primitive, and a single ratchet set
   cannot be borrowed by everyone at once — borrowing is one-at-a-time by
   construction (`Transfer` has one `recipient_id`).
4. **The CTA is a dead end.** "Set one up" dispatches `propose_share`, which
   calls `GenerateWorkshopDraft(experience_id = <a gear id>)`. That lookup
   misses the `Experience` table, returns `CodeNotFound`, and the client
   silently falls back to a **blank** create modal. Every tap costs a
   guaranteed-failing RPC and lands the host nowhere.

Behind the visible card, the same detector persists two claims that are simply
invented: `KickerLabel: "Asked about lately"` and
`AtmosphereLine: "People keep asking about the {gear}"`. Nothing in the
detector queries for anyone asking about anything. Those two fields happen not
to be rendered today — verified: no Dart outside `gen/` references
`atmosphereLine`, `kickerLabel`, or nudge `evidence`, and `BriefCTARow` is
`show`-imported by two repositories with **zero** `ctaRows` consumers, because
the post-#2447 centered overview did not carry the postcard surface forward.
So this is latent, not live. It is the same defect class as the visible one,
and it is what Bryan's "and any similar nudges" is pointing at.

Two sibling detectors carry a worse variant of the same fabrication:
`calendar_gap_detector.go:131-132` and `seasonal_trigger_detector.go:121-122`
ship **invented quotations attributed to real, named sources** — "Harvard
Study of Adult Development" and "Putnam, *Bowling Alone*". Also currently
unrendered. Also indefensible if anything ever renders them.

This is not a copy bug that a rewrite fixes. The detector has no signal to
write honest copy *from*: despite its name, its doc comment
([idle_offer_detector.go:15-18](../../server/services/workshop/momentum/idle_offer_detector.go#L15))
and [workshop.md](../workshop.md) §5 ("gear shared into the community **that
nobody has picked up**"), it never checks idleness, `GearState`,
`CommunityGear.archived`, or `Transfer` activity. It fires on *any* named gear
the user owns that has ever been shared into the circle.

### How it reaches the screen

```
server start (every deploy / cold start)
  └─ server/jobs.go:36        WorkshopGenerationJob.Run
       └─ workshop_generation.go:174  DetectHighestPriority(DefaultDetectors())
            ├─ ActiveQuest / ESMRepeat / CalendarGap / Seasonal → nil for most hosts
            └─ IdleOfferDetector  ← priority-5 catch-all, fires on any owned shared gear
       └─ workshop_generation.go:184  momentum.NewCopyGenerator()   ← TEMPLATE-ONLY,
                                        no AI pass on this path ever
       └─ MaterializeDetection → StoredNudge{
              Surface: WORKSHOP_HERO_CARD, ContextId: <gear id> }

GetHomeView
  └─ portfolio/home_view.go:105  nudgeProvider.InboxNudge(user, allCommunities)
       └─ feed/nudge_storage.go:45  isHostPrompt(n) == true
            (non-feed surface + non-empty context_id)
       └─ feed/feed_nudges.go:170   host prompt returned FIRST — outranks
                                     imagery and every grounded nudge
  └─ home_empty_states.dart:124  HomeZeroStateCard._buildNudgeHero (320 px)
     (and calendar_day_detail.dart on an empty calendar day)
```

Two consequences worth stating plainly:

- **It is the default card for gear-owning hosts.** Priority 5 is the
  catch-all at the bottom of the cascade, so any host with shared gear and no
  event signal gets this and nothing else.
- **It outranks the nudges that work.** `InboxNudge` prefers host prompts over
  everything, and every idle-offer row qualifies. A grounded "Schedule Sunday
  brunch again" in another circle loses to this one.

### Why it is also off-strategy

[users/priorities.md](../users/priorities.md) explicitly de-prioritizes "the
standalone borrow-a-drill product" and says listings "anchor trust by
signaling expertise and willingness to help" — they are not inventory waiting
to be converted into events. The same doc names *the system as catalyst* — the
platform proposing the next real thing — as the compounding advantage. A
fabricated proposal with a dead CTA is the most expensive possible thing to
put in that slot: it teaches the host that the system's suggestions are noise.

### Contract violations this trips

| Rule | Source | How it is broken |
|---|---|---|
| Nudge coherence: headline, description and CTA must all point at the same action, and the reader must be able to predict what the button does | [nudges.md](../nudges.md) → *Coherence rule* | Headline = list gear, description = plan an event, CTA = unnamed, actual behavior = blank modal |
| "Fires when … nobody has picked up" | [workshop.md](../workshop.md) §5 + the detector's own doc comment | No idleness check exists in the code |
| Never query in a loop; always LIMIT unbounded queries | [server/conventions.md](../server/conventions.md) → *SQL Efficiency* 1, 3 | `GetByID` per row inside a loop over an unbounded `CommunityGear` scan ([idle_offer_detector.go:34-49](../../server/services/workshop/momentum/idle_offer_detector.go#L34)) |
| Fail fast / no silent swallow | [server/conventions.md](../server/conventions.md) → *Error Handling* | `GetByID` failure inside the loop is `continue`-d with no log |

## Context from Comments

**There are no comments.** The issue is a one-shot in-app feedback submission
with a screenshot and device context. That is normal for `user-feedback` and
not a signal that the report is thin — the screenshot plus the exact string
pins the defect to a single file. The one thing the report does *not* settle
is scope: "this and any similar nudges should be suppressed" could mean

(a) this detector, (b) every fabricated-claim string across the detector
corpus, or (c) all momentum host prompts on Home. This plan reads it as (a)+(b)
and argues against (c) — see *Open Questions*.

## UI Investigation

**Not run — deliberately.** The `ripls-ui-investigate` skill exists to bisect a
UI-observable failure to a layer when the layer is ambiguous. Here it was not:
the offending text was a static string literal in `idle_offer_detector.go`,
there was exactly one producer (`WorkshopGenerationJob` at server start) and
one consumer (`InboxNudge` → `HomeZeroStateCard`), and the whole path is traced
above from code. A hermetic reproduction would have confirmed what the
screenshot already showed.

The one claim above that was derived rather than observed — the **CTA dead
end** — was verified directly before the path was removed. A throwaway test
drove `GenerateWorkshopDraft` with a gear id through the real service and real
storage:

```
VERIFIED #2892: gear id -> code=not_found
  err=not_found: experience not found: record not found with id 127674d0-…
```

`NotFound` is a user error, which `server/observability.md` classifies as
INFO/WARN rather than ERROR — which is why a CTA that failed on *every* tap
never tripped an alert.

## Implementation status

**All four phases are implemented and merged onto `weird_nudge`.** Every task
below is checked off, with the deviations recorded at the foot of this section.
Verification: full `npm run lint` green; `golangci-lint` clean on the touched
packages; `go test ./server/...` green at 104 packages both plain and with
`-tags=integration`; `flutter analyze` clean; 4,332 Flutter tests pass.

## Potential Solutions

### Option A: Delete the idle-offer detector

- **Approach:** Remove `IdleOfferDetector` and `SlotIdleOffer` from the
  cascade. Rework the three `EnsureHeroCard` tests that use idle gear as their
  fixture vehicle onto a grounded slot. Retire the now-dead `propose_share`
  client branch and the gear arm of `contextMediaID`. Update
  [workshop.md](../workshop.md) §5, its threshold table, and the dev-seeding
  table.
- **Pros:** Removes the whole defect class in one move — no copy to get right,
  no signal to invent, no dead CTA to maintain. Restores the grounded
  event-anchored prompts to the top of the Home hero ranking, which is where
  they were meant to be. Smallest surface area and the only option with no
  residual "is this claim true?" question.
- **Cons:** Gear-owning hosts with no event history lose their only Home hero
  and fall back to the "Need a hand with something?" static prompt — which is
  a *better* card than the one they get today, but it is a reduction in
  personalization. Loses a slot the cascade design reserved.
- **Effort:** Small (one file deleted, ~4 tests reworked, 2 docs updated).

### Option B: Repair the detector against real signal

- **Approach:** Ground it in data that exists. `Transfer` is keyed by
  `gear_id` and carries `state` and `latest_request_unix_sec`, so a genuinely
  idle offer is expressible: gear whose `GearState != GIVEN_AWAY`, whose
  `CommunityGear.archived` is false, with no `Transfer` in a live state and
  none completed in N days. Replace the fabricated atmosphere line with the
  measured fact ("Shared 6 weeks ago, no takers yet"). Replace the CTA with an
  action that exists — the honest one is "mention it in the group", i.e. post
  to the gear's `conversation_id`, not a create-modal dispatch.
- **Pros:** Keeps a signal for the highest-frequency primitive in the product
  (the *offer*). Every claim becomes checkable.
- **Cons:** It is product design, not a bug fix — we would be inventing a new
  lever ("post in chat about your idle drill") against a vertical
  [priorities.md](../users/priorities.md) explicitly de-prioritizes, with no
  evidence any host wants it. `post_in_chat` has no client dispatcher today.
  And it is the *slowest* route to stopping the bad card, which is shipping to
  production users right now.
- **Effort:** Medium–large (new query path, new CTA action end-to-end, new
  copy, new tests, plus the client dispatcher).

### Option C: Keep generating, stop surfacing on Home

- **Approach:** Narrow `isHostPrompt` / `inboxAudience`
  ([nudge_storage.go:45-59](../../server/services/feed/nudge_storage.go#L45))
  so only event-anchored workshop surfaces reach the inbox — e.g. require the
  `context_id` to resolve to an `Experience`.
- **Pros:** One-line-ish change; also hardens the seam against *future* bad
  detectors rather than this one specifically.
- **Cons:** Leaves a detector running that writes false claims into the
  database for a surface that no longer exists — fixes the symptom and keeps
  the bug. Requires an entity-kind resolution on every inbox read (an extra
  lookup per candidate row) to do properly. And it does not touch the
  fabricated quotes in the two sibling detectors, which is half of what the
  issue asks for.
- **Effort:** Small, but it is a kludge: it hides a wrong producer behind a
  consumer-side filter instead of fixing the producer.

## Recommendation

**Option A, plus a de-fabrication pass over the rest of the detector corpus.**

The deciding argument is that there is nothing to preserve. The detector has
never had a signal — its stated contract ("nobody has picked it up") was never
implemented, so there is no working behavior being deleted, only a catch-all
that fires on the mere existence of a listing. Its intended reader (the
Workshop postcard list) was retired with #2568 and the centered overview did
not carry the postcard surface forward, so the only thing still reading it is
a Home hero it actively degrades by outranking grounded prompts.

Option B is the right *eventual* direction if we ever want an offer-side
signal, and the `Transfer` data makes it feasible — but it is new product
design against a de-prioritized vertical, and it does not stop the bleeding
today. File it as a follow-up rather than blocking on it.

Option C's instinct — harden the inbox seam — is worth keeping as a *test*
(Phase 4) rather than a filter: assert that the Home hero preference cannot
promote a prompt whose context entity does not resolve. That gets the
regression protection without the per-read lookup or the kludge.

The de-fabrication pass is what makes this answer "and any similar nudges"
rather than just this one. Concretely: delete the two invented research
quotations and the invented "people keep asking" claim, and write the rule
down where the next detector author will read it.

**What this does not fix:** the trade-off is real and worth stating. A
gear-owning host with no event history sees the static "Need a hand with
something?" prompt instead of a personalized card. That is a downgrade in
personalization and an upgrade in truthfulness, and truthfulness is the one
the catalyst thesis cannot trade away.

## Open Questions

- **Does "any similar nudges" mean all momentum host prompts on Home, or just
  the fabricated ones?** — Recommendation: just the fabricated ones. The other
  four detectors write copy derived from values they measured ("Hosted 4 times
  before", "3 of 5 said yes", "Ran 6 times, then quiet"). Those are the
  product's differentiator working correctly, and
  [nudges.md](../nudges.md) records that the host-prompt preference was added
  precisely because the `runclub` walkthrough caught them being buried.
  Suppressing them would re-open a fixed bug. Worth confirming with Bryan
  before Phase 1 lands, since it is the one place a wrong reading changes the
  scope materially.
- **Delete `IdleOfferDetector` or leave it behind a flag?** — Recommendation:
  delete. It has no correct behavior to preserve, and a dormant detector is a
  future reader's trap. Git history and this issue hold the intent; Option B
  starts from a clean design, not from this code.
- **Should `SlotIdleOffer` stay in the `Slot` enum?** — Recommendation: yes,
  keep the constant, its `String()` case, and its arm of `SurfaceFor`
  ([generator.go:15-22](../../server/services/workshop/momentum/generator.go#L15)).
  `Slot` values are written into logs, and removing an `iota` member
  renumbers `SlotSeasonalTrigger` and friends. Drop it from
  `DefaultDetectors()` only, and comment the gap. Note that `stubDetector`
  ([detector.go:132-141](../../server/services/workshop/momentum/detector.go#L132))
  looks orphaned but is **not** dead — `detectors_test.go:525` asserts no
  live detector is one. Leave it.
- **Do we purge existing `stored_nudge` rows in prod?** — Recommendation: no.
  `pruneExpiredNudges` soft-deletes at 24 h
  ([feed_nudges.go:91](../../server/services/feed/feed_nudges.go#L91)) and the
  producing job only runs at server start, so the fix deploy is itself the
  last boot that could have created one. A manual purge buys at most 24 h and
  risks touching rows retained for analytics.
- **Keep `propose_share` in the action vocabulary?** — Recommendation: retire
  it. `cta_action` is a plain `string` on the wire, so dropping the client
  branch is not a proto break. Leave a note in
  [nudge.proto](../../proto/ripls/models/nudge.proto) that the value is
  retired, and keep the `default:` fallback (blank create) so any stale
  persisted row degrades to exactly what it already degrades to today.
- **Delete the two invented research quotes, or source them?** —
  Recommendation: delete. Both are unrendered today, so nothing is lost, and
  "find a real citation for a card nobody sees" is work with no user on the
  other end. If evidence lines return, they should carry a real quote and a
  real citation, added deliberately.

## Implementation Plan

### Phase 1: Stop the bad card

**Goal:** No host can be served the idle-offer nudge again.
**Tasks:**
- [x] Verify the CTA dead end before removing it: seed gear + `CommunityGear`,
      let the detector produce a nudge, tap "Set one up", and confirm
      `GenerateWorkshopDraft` returns `NotFound` and the client opens a blank
      modal. Record the result here — if it *doesn't* dead-end, the CTA
      analysis above needs correcting.
- [x] Delete `server/services/workshop/momentum/idle_offer_detector.go` and
      drop `IdleOfferDetector{}` from `DefaultDetectors()`
      ([detector.go:122](../../server/services/workshop/momentum/detector.go#L122)),
      leaving `SlotIdleOffer` in the enum with a comment saying why the slot is
      unfilled.
- [x] Rework the tests that use idle gear as a generic fixture vehicle onto a
      grounded slot (seasonal or calendar-gap seeds):
      `TestEnsureHeroCard_IdleOfferProducesHeroCard`,
      `TestEnsureHeroCard_IsIdempotent`,
      `TestEnsureHeroCard_RegeneratesAfterConsumed`, and the
      `contextMediaID` test at `generation_test.go:249`. Delete
      `seedIdleGear` once unused.
- [x] Rework `TestWorkshopGenerationJob_PersistsForIdleOfferHost`
      (`server/jobs/workshop_generation_test.go:27`) the same way, and drop
      the idle-offer cases from `momentum/detector_test.go:65-144`.
- [x] Add a regression test: gear + `CommunityGear` and no event history
      produces **zero** hero cards. **Assert on the persisted `StoredNudge`
      rows, not only on `EnsureHeroCard`'s return slice** — the two can
      diverge, and a slice-only assertion passes vacuously.
- [x] Update `server/services/workshop/momentum/README.md` — its *Files*
      table lists `idle_offer_detector.go` and its cascade list names
      `SlotIdleOffer` as the only implemented slot. CLAUDE.md requires every
      multi-file `server/` directory README to stay accurate.
- [x] `go test ./server/services/workshop/... ./server/jobs/... ./server/services/feed/...`
      (these are plain unit tests — no `-tags=integration` — but they use
      `setupTestStorage` → testcontainers, so Docker must be running.)

**Validates:** That the cascade's four remaining detectors carry the load on
their own, and that no path can still mint a gear-context host prompt.

### Phase 2: Remove the dead CTA path

**Goal:** No orphaned client branch or server lookup left behind.
**Tasks:**
- [x] Drop the `case 'propose_share':` branch from
      [nudge_content_view.dart:126](../../app/lib/presentation/widgets/nudge/nudge_content_view.dart#L126)
      so it falls to `default:` (blank create) like any unrecognized action,
      and remove the gear caveat from the `openRepeatDraft` doc comment
      ([blank_create_dispatcher.dart:199-202](../../app/lib/presentation/screens/create/blank_create_dispatcher.dart#L199)).
- [x] Drop the gear arm of `contextMediaID`
      ([generation.go:210-213](../../server/services/workshop/generation.go#L210))
      — with the idle-offer lever gone, every `context_id` is an experience.
      Update the surrounding comment, which names gear explicitly. Keep the
      swallowed-`GetByID` behavior as-is (a missing entity legitimately means
      "no background"), but make sure the comment still says so — post-edit it
      is the function's only remaining error path.
- [x] Update [nudges.md](../nudges.md): remove the `propose_share` row from
      the workshop CTA table and the gear clause from *They wear the
      referenced entity's own photo*. While in the file, fix the stale
      `InboxNudge` ranking sentence (~line 419) which still describes the
      pre-host-prompt two-tier order.
- [x] Update [workshop.md](../workshop.md): delete §5 *Idle Offer*, the
      Idle-Offer row in *Data prerequisites for testing in dev*, the
      idle-gear row in the host-journey coverage table, and the `propose_share`
      mentions in the action-vocabulary paragraph. Re-run
      `npm run generate:context-map` if any `context:` block changes.
- [x] Note the retirement of `propose_share` at **all three** proto comment
      sites — [nudge.proto](../../proto/ripls/models/nudge.proto) `surface`
      (lines 139-140) *and* `context_id` (line 151, which explicitly says "or
      a gear id (for propose_share)"), plus
      [workshop_service.proto](../../proto/ripls/api/workshop_service.proto)
      `BriefLever.action_name` (line 142). Comment-only: `cta_action` /
      `action_name` are plain `string`s, so nothing is removed, no `reserved`
      is needed, and the `WIRE` gate is unaffected (it rejects only type,
      cardinality, `oneof`, number-deletion and RPC-signature changes — see
      [proto_conventions.md](../proto_conventions.md) → *Deprecating and
      removing fields*). Still run `npm run generate` — comments flow into
      generated code.
- [x] While in `nudge.proto`, **scope the edit or fix it properly, don't
      extend it.** The surrounding comments already carry server mechanics
      that [proto_conventions.md](../proto_conventions.md) → *Proto comments*
      forbids ("Populated by the workshop generation pipeline",
      "filtered out of feed assembly"). Either leave them and change only the
      value list, or clean them in the same pass — but do not add more
      pipeline description.
- [x] Update `docs/ai/workshop.md:89`, which also lists `propose_share`. It
      is excluded from `docs/llms.txt` but still goes stale.
- [x] `npm run generate` + `npm run lint` + `flutter analyze` +
      `flutter test --reporter expanded`

**Validates:** That deleting the detector left no reader stranded, and that
the docs no longer describe a detector that does not exist.

### Phase 3: De-fabricate the rest of the corpus

**Goal:** No detector ships a claim it did not measure.
**Tasks:**
- [x] Delete the invented `EvidenceQuote` / `EvidenceAttribution` pairs from
      [calendar_gap_detector.go:131-132](../../server/services/workshop/momentum/calendar_gap_detector.go#L131)
      and
      [seasonal_trigger_detector.go:121-122](../../server/services/workshop/momentum/seasonal_trigger_detector.go#L121).
      Keep the `Detection.Evidence*` fields and the `StoredNudgeEvidence`
      message — they are the right shape for a real citation later.
- [x] Audit the remaining static copy for any other unmeasured claim. From the
      current corpus the survivors are all count- or date-derived
      ("Hosted %d times before", "Ran %d times, then quiet", "Same week last
      year"); confirm each is populated from a value the detector actually
      read, and fix or delete any that isn't.
- [x] Add a golden test in `momentum` pinning the full set of static copy
      strings each detector can emit, so a new fabrication has to be
      explicitly added to a list a reviewer reads. This is the honest
      enforceable version — a general "no invented claims" linter is not
      buildable, but making the corpus visible is.
- [x] Write the rule into [workshop.md](../workshop.md) → *Adding a new
      detector*: every user-visible field must be derivable from a value the
      detector read from storage; social proof requires a query, not a
      template.

**Validates:** That "and any similar nudges" is actually answered, not just
the one card in the screenshot.

### Phase 4 (optional): Guard the Home hero seam

**Goal:** A future bad detector cannot inherit the top slot on Home.
**Tasks:**
- [x] Add a `server/services/feed` test asserting the host-prompt preference
      ranks a grounded event prompt above a generic pool nudge **and** that a
      workshop-surface row whose `context_id` resolves to nothing does not
      win outright.
- [x] Add the firing slot to the "workshop hero cards generated" log line
      ([generation.go:153](../../server/services/workshop/generation.go#L153))
      and to the job's completion line, so what we are actually serving is
      answerable from Cloud Logging rather than from a screenshot in a
      feedback ticket. Per
      [server/observability.md](../server/observability.md), use existing
      field conventions — `slot` alongside the existing `count`. **Add a
      field; do not change the message text.** Log-based metrics (#1613) match
      on message strings. Checked: nothing under
      `terraform/modules/monitoring/` matches either message today, so this is
      safe — but the discipline is what keeps it safe next time.

**Validates:** That the next detector regression shows up in logs and in CI
rather than in user feedback.

## Related Issues

- **#2568** — retired the Workshop tab; the reason these nudges have no
  reader except the Home hero, and the reason the idle-offer card is the
  *only* thing many hosts see.
- **#2488** — inbox follow-ups incl. contextual nudges; the umbrella under
  which the Home nudge surface was added.
- **#2796 / #2801** — the Home zero-state nudge hero's framing and
  fit-to-height behavior; the frame this card renders in.
- **#2803** — the "Event", not "Experience" vocabulary rule enforced in the
  nudge prompt; the closest precedent for policing generated nudge copy.
- **#1579 / #1618** — the original Workshop plans that specified the cascade
  and the idle-offer slot. Worth reading before any Option-B revival.
- **Follow-up to file:** a grounded offer-side signal built on `Transfer`
  (`state`, `latest_request_unix_sec`) + `CommunityGear.archived` +
  `GearState`, with a CTA that exists. Option B above is its starting point.

---

## Deviations from the plan as written

Four, all recorded here rather than silently absorbed:

1. **`docs/ai/workshop.md` was annotated, not edited.** Phase 2 said to update
   line 89. On opening it, the file is a dated frozen plan — *"Date:
   2026-04-29 / Status: Planning — do not implement yet"* — and surgically
   deleting `propose_share` from it would falsify the April record of what was
   planned. It gets a **Superseded in part** note at the top pointing at
   `docs/workshop.md` §5 instead; the body is untouched.
2. **One extra fabrication was found and fixed.** The Phase 3 audit turned up
   `"Most of the circle wants this on the calendar"` in *two* files. The ESM
   threshold is ≥60% of **respondents** with a minimum of two — in a circle of
   twenty, that sentence is false. Now "Most who answered". Same class as the
   defect the issue reported, so it is in scope.
3. **Seasonal's atmosphere line was also wrong.** `"Same week last year — and
   it landed"` claimed two unmeasured things: the window is ±15 days, not a
   week, and completion is not evidence an event went well. Now `"Ran around
   this time last year"`.
4. **The golden-test walker was fixed rather than the list trimmed.** Its
   first version missed copy built into a local before assignment, which is
   where the ESM atmosphere lines live — precisely the copy most at risk of
   overclaiming. Resolving locals per function was the right fix; trimming the
   golden list to match a weak walker would have produced a passing test that
   covered less than it appeared to.

The gate is verified by mutation, not just by passing: introducing
`"Three people asked about this last week."` into `bring_back.go` fails
`TestCopyCorpus_IsAccountedFor`, and the change was reverted.

---

## Audit notes

Audited against the documentation context map on 2026-08-10. Engaged lenses:
`architecture`, `conventions`, `server`, `client`, `product`; engaged review
dimensions: `code_quality`, `tech_debt`, `testing`, `error_handling`,
`observability`, `performance`, `api_design`. `security`, `accessibility`, and
`dependency` were assessed and found not engaged — the change adds no
auth/PII/secret surface, alters no rendered widget (it removes a `switch`
branch so the card falls to the existing default), and adds no dependency.

Findings applied to the plan above:

1. **Missing README update (CLAUDE.md rule).** Phase 1 did not touch
   `server/services/workshop/momentum/README.md`, whose *Files* table and
   cascade list both name the deleted detector. Added as a Phase 1 task.
2. **Two proto comment sites missed, and one misidentified.** The plan named
   a single "`cta_action` comment" in `nudge.proto`; `propose_share` actually
   appears in `nudge.proto`'s `surface` **and** `context_id` comments and in
   `workshop_service.proto`'s `BriefLever.action_name`. Phase 2 corrected,
   with the no-break/`npm run generate` implication spelled out.
3. **`stubDetector` is not dead code.** A cleanup sweep would plausibly
   remove it; `detectors_test.go:525` asserts against it. Called out in *Open
   Questions* so the sweep does not take it.
4. **`SurfaceFor` also names `SlotIdleOffer`.** Recorded alongside the
   keep-the-enum decision so the arm is left in place deliberately rather
   than found and "cleaned up" later.
5. **Vacuous-assertion risk in the new regression test.** `EnsureHeroCard`'s
   return slice and the persisted rows can diverge; the test now has to assert
   on storage.
6. **Log-based-metric hazard in Phase 4.** Adding observability to an existing
   INFO line must add a field, not reword the message (#1613 metrics match on
   message text). Verified nothing in `terraform/modules/monitoring/` matches
   these two messages today, and said so.
7. **Test-run preconditions.** These are unit tests (no `-tags=integration`)
   but they go through `setupTestStorage` → testcontainers, so Docker is a
   precondition. Noted on the Phase 1 command.
8. **`docs/ai/workshop.md` staleness.** Outside `docs/llms.txt` but still names
   `propose_share`. Added to Phase 2.
9. **Unrendered-fields claim hardened.** The "latent, not live" argument is
   load-bearing for the recommendation, so it now cites the specific
   verification (no `atmosphereLine`/`kickerLabel`/`evidence` reads outside
   `gen/`; `BriefCTARow` imported with zero `ctaRows` consumers).
10. **Proto-comment convention.** The comments Phase 2 edits already violate
    [proto_conventions.md](../proto_conventions.md) → *Proto comments* (they
    describe server pipeline mechanics). Phase 2 now says to scope the edit or
    fix them deliberately rather than silently extend them.
11. **`WIRE`-gate claim sourced.** The "comment-only, no break" assertion is
    now backed by the gate's actual reject/permit list rather than asserted.

One thing the audit checked and deliberately did **not** add: the plan does
not claim the dead-end CTA pollutes error metrics. It doesn't —
`connect.CodeNotFound` is a user error, which
[server/observability.md](../server/observability.md) classifies as INFO/WARN,
not ERROR. That is precisely why a CTA that fails on every single tap has been
failing silently in production with nothing alerting on it.

No architecture violations found: the plan removes code rather than adding
cross-service dependencies, touches no proto field numbering, introduces no
new storage query, and the N+1 / unbounded-scan violations it identifies
disappear with the deleted file rather than needing a separate fix.

**Docs consulted:** `docs/llms.txt`; anchors
[server/architecture.md](../server/architecture.md),
[server/conventions.md](../server/conventions.md),
[server/observability.md](../server/observability.md),
[client/architecture.md](../client/architecture.md),
[client/conventions.md](../client/conventions.md),
[proto_conventions.md](../proto_conventions.md),
[testing/architecture.md](../testing/architecture.md),
[users/priorities.md](../users/priorities.md); leaf docs
[workshop.md](../workshop.md), [nudges.md](../nudges.md),
[client/inbox.md](../client/inbox.md); review rubrics
[code_quality](../reviews/code_quality_review_prompt.md),
[tech_debt](../reviews/tech_debt_review_prompt.md),
[testing](../reviews/testing_review_prompt.md),
[error_handling](../reviews/error_handling_review_prompt.md),
[observability](../reviews/observability_review_prompt.md),
[performance](../reviews/performance_review_prompt.md),
[api_design](../reviews/api_design_review_prompt.md),
[architecture](../reviews/architecture_review_prompt.md); plus project
`CLAUDE.md`.
