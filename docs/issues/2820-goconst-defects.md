# Issue #2820: [tech-debt] goconst: fix the three defects behind its 64 findings (not by adding constants)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2820
**Filed:** 2026-07-30 | **Last activity:** 2026-07-30
**Labels:** `review:code_quality` | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`; anchors `docs/server/architecture.md`,
`docs/server/conventions.md`, `docs/proto_conventions.md`; leaves
`docs/server/l10n.md`, `docs/server/portfolio.md`, `docs/client/inbox.md`,
`docs/client/i18n.md`; review prompts `docs/reviews/code_quality_review_prompt.md`
(§2 Strong Typing, §5 Dead Code, §10 Magic Values),
`docs/reviews/tech_debt_review_prompt.md`, `docs/reviews/api_design_review_prompt.md`.

**UI investigation:** skipped — no UI-observable misbehavior. This is a
lint-backlog / code-shape issue with no user-visible symptom to reproduce.

## Problem Statement

`goconst` is parked in `server/.golangci.yml`. Turning it on at
`min-occurrences: 3` would surface 64 findings, and the issue's core claim is
correct and important: **mechanically hoisting those strings into Go constants
would freeze three real defects in place.** The right move is to fix what the
strings are evidence of, then enable the linter as a regression guard.

I reproduced the measurement — exactly 64 non-test findings on `main` with
`uniq-by-line: false` — and verified each group against the code. Two of the
issue's three defect claims survive verification; the third does not, and the
correction makes the work *cheaper*, not harder. Three material corrections:

**1. Groups 2 and 3 describe dead code, not a live render-boundary violation.**
The issue says `assembly.go` "ships English UI copy in the `api.Home*`
response." It ships it in `api.DailyItem`, and **no non-generated Dart reads any
of it**. `state_label`, `action_label`, `type_label`, `inbox_status_label`,
`action_description`, `action_button_label`, `action_button_style`, and `emoji`
have zero client readers. `GetPortfolioInboxView` has exactly one client caller
(`directoryPeopleProvider` in `app/lib/presentation/viewmodels/directory_view_model.dart:31`)
and it reads only `view.people`. `GetPortfolioInboxFeed` has repository and
service wrappers but no viewmodel or screen caller at all. The client reads were
removed in `1774bf648` ("Retire legacy portfolio inbox…", 2026-07-22, first
shipped in **v0.28.0**).

That reframes the work. Group 2 is not an ARB migration — the labels are not
moving to the client, they are going away. And Group 3 (control flow keyed on
display text) dissolves with them: those `switch` arms exist *only* to compute
the dead labels. The issue proposes building an internal row-state enum to
protect a code path with a scheduled delete date.

**2. The live render-boundary violation is somewhere else, and the issue misses
it.** `server/services/portfolio/home_view_decisions.go` builds
`HomeDecision.kind_label` / `accept_label` / `why` as English
("Lending request", "Lend it", "Mark picked up", "Say thanks", "Wrap up",
`"due " + homeDayLabel(...)`) and the client **does** render them —
`home_tab_screen.dart:196,220`, `needs_you_group_row.dart:93,168,207,217`,
`home_decision_routing.dart:146,150`. `server/services/workshop/brief.go:397`
does the same for `WorkshopBriefRow.action_label`. This is the real, live
instance of the defect the issue describes. It is invisible to `goconst` because
each literal appears once or twice.

**3. Group 1's finding count is off by ~5x, but its *real* surface is wider than
the issue says.** "~28 findings" is 6 goconst findings covering 44 occurrences.
The true blast radius is larger: **82 non-test occurrences of bare
`"YES"`/`"MAYBE"`/`"NO"`/`"UNKNOWN"` across 34 files**, because goconst only
fires at ≥3 occurrences *per file*. Files like `known_for/experiences.go`,
`notifications/community_subscriber/recipients.go`, and
`jobs/scheduled_notifications/experience_reminders.go` carry the same
stringly-typed comparisons and contribute zero findings. A fix scoped to what
goconst reports leaves most of the defect standing.

Everything else in the issue holds up. Confirmed as written:

- `proto/ripls/models/experience.proto:173-174` — `string intention = 5` and
  `string attended = 6`, comments admitting they hold enum values.
- `services/experience/build_api.go:196` — `switch rsvp.Intention { case "YES": … }`.
- The API/models separation rule (`docs/proto_conventions.md` §The separation
  rule) forbids `models` importing `api.RSVPIntention`. (The enum is
  `api.RSVPIntention`, not `api.RsvpIntention`.)
- Field 5 cannot be reused: proto3 `string` is wire type 2, `enum` is wire type
  0. Redefining it mis-decodes every stored RSVP.
- **`buf breaking` is not wired into CI.** `buf.yaml` declares
  `breaking: use: [FILE]`, but `npm run lint:proto` runs `buf lint` only and no
  workflow invokes `buf breaking`. A wire-incompatible proto change passes CI.

## Context from Comments

**The issue has no comments.** It was filed by `/ripls-reviews` (label
`review:code_quality`) on 2026-07-30 and split out of #2816 the same day, so
there has been no maintainer discussion. Two consequences:

- The Group 1 → Group 4 sequencing is one author's proposal, not a decision.
- The issue cites `docs/issues/2816-lint-ratchet.md` as #2816's plan. **That file
  does not exist** — `docs/issues/` has no `2816-*` document. #2816's body is the
  only spec for the ratchet.

Context inherited from the split, verified against `server/.golangci.yml`:

- #2816's own goconst section reaches the same three-group conclusion and says
  "Decide (1) and (2) before writing any constant." #2820 is the follow-through.
- The config comment block (lines 224–238) already records `goconst 64 at
  min-occurrences 3` and flags `"YES" x19` and the `"Hosting"`/`"On loan"`/
  `"Review"` labels as non-mechanical. The analysis is already in-tree.
- #2816 notes a config-only ratchet fails: a green `min-occurrences` today is
  **20**, which catches nothing.

## Potential Solutions

### Option A: Delete the dead label plumbing; type the RSVP strings (recommended)

- **Approach:** Four sequenced steps. (1) Land the genuinely-mechanical
  constants. (2) Delete the eight dead `DailyItem` label fields and their Go
  helpers outright, alongside #2020's own deletion cohort, and close #2020 —
  Groups 2 and 3 both resolve, no ARB keys, no internal enum.
  (3) Introduce a typed `RSVPIntention`/`AttendedStatus` string type with a
  constant block in a shared server package and sweep all 82 occurrences, not
  just goconst's 6 findings. (4) Enable `goconst` at `min-occurrences: 3`.
  File the live `HomeDecision`/`WorkshopBriefRow` render-boundary violation as
  its own issue.
- **Pros:** Removes ~15 of 64 findings by deleting code rather than annotating
  it. Avoids building an internal state enum and an ARB migration for a response
  nothing reads. Step 3 fixes the whole defect rather than the linted subset.
  Each step ships independently.
- **Cons:** Step 2 is a wire-breaking deletion that `buf breaking` cannot review
  (not wired into CI), and clients on ≤0.27.x lose inbox label chrome until they
  update. Step 3 touches 34 files.
- **Effort:** Medium. Step 1 small, step 2 medium (now includes closing out
  #2020), step 3 medium, step 4 trivial.

### Option B: The issue as written — internal state enum, then ARB migration

- **Approach:** Group 4 consts → internal row-state enum replacing the display-
  string branching → move labels to `app_en.arb`, update `docs/client/inbox.md`,
  extend `check_no_inline_user_strings.js` → typed RSVP constants → enable
  `goconst`.
- **Pros:** Follows the filed plan literally.
- **Cons:** Builds an enum and ships ARB keys for labels no client reads and that
  are being deleted in Phase 2 — net new code on a dying path. With the
  accelerated schedule the intermediate enum has no window to protect at all.
  Also, **the proposed
  guard would not fire**: `check_no_inline_user_strings.js`'s `INLINE_PATTERN`
  requires `fmt.Sprintf("Capital…{8,}")` and `PROSE_PATTERN` requires trailing
  `.!?`; `"Borrow Request"`, `"On Loan"`, and `"Lending"` match neither. Adding
  `assembly.go` to `SCANNED_FILES` is a no-op without a new pattern.
- **Effort:** Large.

### Option C: Config-only floor now, defect work later

- **Approach:** Enable `goconst` at `min-occurrences: 20` immediately as a
  regression floor; ratchet 20 → 10 → 5 → 3 as the defects are fixed.
- **Pros:** Zero-risk, one-line, stops the worst offender growing.
- **Cons:** #2816 already measured this: at 20 the only finding is `"YES" x19` in
  `build_api.go`, so the gate catches essentially nothing. It is a floor, not a
  fix.
- **Effort:** Trivial.

## Recommendation

**Option A**, with Option C's floor folded into Phase 1 as a cheap interim guard.

The deciding fact is that Groups 2 and 3 are dead code. Once that is established,
the issue's most expensive proposals — an internal row-state enum and an ARB
migration — are work spent on a response payload nothing reads. Deletion is both
the cheaper path and the root-cause fix: it removes the string-keyed control flow
by removing the strings.

Per the 2026-07-30 decision, Phase 2 deletes rather than deprecates, and closes
out #2020 in the same PR. That turns out to be less aggressive than it sounds —
#2020's "two full release cycles" prerequisite is **already met** for its own
cohort (#2012 closed 2026-05-19; four releases have shipped since). The only real
acceleration is the eight label fields, whose readers disappeared in v0.28.0 two
days ago; the accepted cost is blank inbox chrome on ≤0.27.x builds until users
update. Ship the two cohorts as separate commits so that cost is independently
revertible.

Two things I am deliberately *not* folding in (plus one carve-out from #2020's
acceptance list — see Open Questions):

- **The live render-boundary violation gets its own issue.** `HomeDecision` and
  `WorkshopBriefRow` are a real, user-visible instance of the defect #2820
  describes, but they are a different code path with a different fix (structured
  `{template_key, params}` + ARB, per `docs/server/l10n.md` §Adding a new in-app
  server-emitted string) and a different blast radius. Folding it in would turn a
  lint-backlog issue into a client-coordinated l10n migration.
- **The `models.RSVPIntention` enum migration stays priced separately.** The
  issue is right that the typed-constant step is the cheap 80%. The wire-
  incompatible field migration needs dual-population, a backfill, a pre-filed
  cleanup issue (per CLAUDE.md), *and* the missing `buf breaking` CI gate — which
  is itself a prerequisite worth its own issue, since CI currently cannot catch a
  mistake in exactly this class of change.

I recommend the CI gate land before, not after, the enum migration.

## Open Questions

- **Is `GetPortfolioInboxFeed` dead end-to-end, or does something outside
  `app/lib` call it?** — Recommendation: treat it as dead but verify before
  deleting anything beyond the label fields. I found repository and service
  wrappers plus unit tests, and no viewmodel or screen caller. Confirm against
  `e2e/` and `server/integration_tests/` before proposing RPC removal; that is a
  bigger question than #2820 and should be its own issue if confirmed.
- ~~**How long is #2020's deprecation window, and can the label fields ride
  it?**~~ — **RESOLVED (Bryan, 2026-07-30): don't wait.** Delete both cohorts and
  close #2020 in this PR. Note that #2020's prerequisite was in fact already met
  for its own cohort (#2012 closed 2026-05-19; four releases since), so only the
  eight label fields are a real acceleration.
- ~~**Should the string-keyed branching be neutralized during the window, or just
  frozen?**~~ — **RESOLVED: neutralized, by deletion.** No internal row-state
  enum, no contract-frozen comment. The strings and the branches go together.
- **Does closing #2020 require settling its `EntityKind` criterion?** —
  Recommendation: no, carve it out. "`MetricLineItem` / `SharedItem` /
  `DailyAction` migration to `EntityKind` is decided" is an open design question
  that #2020's own body marks "separate scope if non-trivial". Settling it inside
  a lint-cleanup PR is how a scoped PR becomes an unscoped one. File it as a
  follow-up and record the carve-out in the closing comment. **This is the one
  place I'd push back on "close out #2020 in this PR"** — everything else in
  #2020's acceptance list is mechanical deletion and belongs here.
- **Is the blank-inbox-chrome symptom on ≤0.27.x acceptable?** —
  Recommendation: yes, given the stated decision, but ship 2a and 2b as separate
  commits so 2b can be reverted alone if the symptom shows up in support traffic.
  There is no force-upgrade gate in the codebase, so old builds persist
  indefinitely; the exposure shrinks only as users update.
- **Where should the typed RSVP constants live?** — Recommendation: a small new
  `server/rsvp` package, not `services/experience/`. The 34 affected files span
  `known_for/`, `notifications/`, `jobs/`, `esm/`, `story/`, `impact_metrics/`,
  and seven `services/` packages; per `docs/server/architecture.md`, a shared
  library is the right home and a service package is not importable from peers.
- **Is `"UNKNOWN"` always `AttendedStatus`?** — Recommendation: no, sweep
  carefully. `services/experience/convert_informal_time.go` uses `"UNKNOWN"` /
  `"EXPLICIT"` / `"INFERRED"` for *time precision*, a different stringly-typed
  field that happens to share a token. Scope the sweep per call site, not by
  literal.
- **Should `weight_material_carbon` be in scope?** — Recommendation: include it
  for symmetry, but note the issue lists it in error. It has 2 non-test
  occurrences (`impact_metrics/estimator/gear_carbon.go:25`,
  `savings.go:156`) and is **not** among the 64. Only `spend_based_carbon` (×3)
  is.

## Implementation Plan

### Phase 1: The genuinely mechanical constants
**Goal:** Clear every finding that really is a missing constant, and put a
regression floor under the rest. Independent of every other phase.
**Tasks:**
- [ ] `USER_PRIMARY_LOCATION` — extract a shared constant used by
      `services/experience/gen_ai.go` (×9) and `services/request/gen.go` (×5).
      Both compare against the same AI-response sentinel; it belongs in one place.
- [ ] `spend_based_carbon` (×3, `impact_metrics/estimator/gear_carbon.go`) plus
      `weight_material_carbon` for symmetry — provenance formula names.
- [ ] `"http"` / `"https"` in `webfetch/fetcher.go` (×4 / ×5).
- [ ] Section names in `simulation/report.go`: `gear`, `loans`, `requests`,
      `giveaways`, `experiences`, `cancellations` (×4 each).
- [ ] `location/google.go` place-type literals (`Address`, `City`, `Locality`,
      `POI`, `Place`, `locality`) and `storage/media_sanitize.go` MIME types.
- [ ] `STORY_TYPE_*` (`services/feed/items.go`, `story/templates.go`) — use
      `api.StoryType_X.String()` rather than new constants, as the issue says.
      Note in the PR that `models.Story.story_type` is a bare `string`
      (`proto/ripls/models/story.proto:20`), so this decodes storage using API
      enum names; that coupling is the same class as Group 1 and is *not* fixed
      here.
- [ ] Enable `goconst` at `min-occurrences: 20` in `server/.golangci.yml` as an
      interim floor. **Verified green on `main`:** running goconst at
      `min-occurrences: 20` with `uniq-by-line: false` and `tests: false`
      produces zero findings today (the worst literal is `"YES"` ×19, one under
      the threshold). This is a *growth* guard — it fires only if some literal
      reaches 20 occurrences — not a gate. It cannot turn the required check red.
- [ ] Do not name `USER_PRIMARY_LOCATION` in any `.proto` comment.
      `docs/proto_conventions.md` §Proto comments names it explicitly as an
      internal sentinel that must not leak into the API contract. The Go constant
      is fine; a proto comment referencing it is not.
**Validates:** That the mechanical subset is genuinely mechanical, and gives an
exact remaining count for the defect groups.
**Verify:** `npm run build` (runs `npm run lint:go` then `go build ./...` — the
same gate CI runs; never `golangci-lint run ./...` from the repo root, per
`docs/server/conventions.md` §Go Server Build Verification), then
`go test ./server/...`. The `STORY_TYPE_*` → `.String()` swap is behavior-
adjacent, not cosmetic: confirm `services/feed` and `story` package tests pass.

### Phase 2: Delete the dead label plumbing and close out #2020
**Goal:** Neutralize the string-keyed control flow by deleting it, and finish
#2020's deletion pass in the same PR.

> **Decision (Bryan, 2026-07-30):** neutralize *and* close out #2020 in this PR —
> accelerating the deprecation schedule is acceptable. This supersedes the
> earlier "deprecate now, delete after the window" shape and resolves the
> freeze-vs-neutralize open question: **deletion is the neutralization**, so no
> internal row-state enum gets built.

Two cohorts are in scope, with materially different exposure. Keep them as
separate commits so either can be reverted alone.

**2a — #2020's own cohort (prerequisite already met; low risk).** #2012 closed
**2026-05-19**, and four releases have shipped since (v0.26.0 7/02, v0.27.0 7/17,
v0.28.0 7/28, v0.28.1 7/30). #2020's stated prerequisite — "in production for at
least two full release cycles" — is already satisfied. This is not an
acceleration at all; #2020 was simply never picked up.

- [ ] `proto/ripls/api/portfolio.proto` — delete `DailyItem.title` (2),
      `item_type` (10), `item_id` (11), `background_media_id` (14),
      `content_id` (19), `description` (20). **Reserve every field number** per
      `docs/reviews/api_design_review_prompt.md` §4 (Field Removal).
- [ ] `proto/ripls/api/workshop_service.proto` — delete `BriefCTARow.cta_action`
      (5), `context_id` (6), `thumbnail_media_id` (8); `BriefLever.cta_action`
      (2); and the vestigial `BriefCTARow.thumbnail_caption` (9),
      `headline_emphasis` (10), `BriefLever.visible_text` (1). Reserve the
      numbers. Re-verify the vestigial three still have zero readers — #2020's
      audit predates several releases.
- [ ] Delete the dual-population code and its `//nolint:staticcheck` markers:
      `services/workshop/brief.go:407,416`; `services/profile/postcards.go`
      (11 sites including the header comment at :29) and `postcards_test.go`;
      `services/portfolio/assembly_helpers.go::populateRowItem` (:26).
- [ ] Remove any client fallback reads (`if .core unset then read top-level`)
      added by #2012.
- [ ] **Two acceptance criteria are already moot — tick and say so when closing.**
      `AvailableNowItem` / `AvailableNowKind` no longer exist anywhere in
      `proto/`. The `@Deprecated('use pushToEntity')` shim on `pushToItemScreen`
      was never added — `navigation_helpers.dart:61` carries no annotation and
      13+ live call sites use it.

**2b — the eight label fields (the genuine acceleration).** Their client readers
were removed in `1774bf648`, first shipped in **v0.28.0 on 2026-07-28 — two days
before this plan**. See the risk note below.

- [ ] Re-verify the dead-field claim at implementation time: no non-generated
      Dart or `e2e/` reader for `state_label`, `action_label`, `type_label`,
      `inbox_status_label`, `action_description`, `action_button_label`,
      `action_button_style`, `emoji` on `DailyItem`.
- [ ] Delete those eight fields from `portfolio.proto`; reserve the numbers.
- [ ] Delete `resolveActionButton`, `resolveInboxStatusLabel`,
      `resolveActionDescription`, `resolveTypeLabel`, `resolveEmoji`,
      `transferStateLabels`, `transferDoneStateLabel`,
      `experienceDoneStateLabel`, their call sites in `assembly.go` /
      `inbox_feed.go` / `completed_inbox_feed.go` / `library_items.go` /
      `inbox_view.go`, and the 19 assertions in `assembly_test.go`.
- [ ] Confirm nothing surviving depends on the deleted helpers. `IsActionRequired`
      is computed from `transfer.State` / `request.State` / ownership, never from
      a label — verify that still holds after the cut.

**Closing out #2020:**

- [ ] **Amend #2020's scope before closing.** Its body explicitly says *"Keep all
      30+ inbox-specific fields: … `action_label`, `state_label`, … `emoji`"* —
      written before `1774bf648` retired the legacy inbox and killed those
      readers. That keep-list is stale. Post the dead-code evidence on #2020,
      state that cohort 2b is folded in, then close.
- [ ] **Carve out one criterion.** #2020's "`MetricLineItem` / `SharedItem` /
      `DailyAction` migration to `EntityKind` is decided" is an open *design*
      question that its own body marks "separate scope if non-trivial". Do not
      settle it inside a lint-cleanup PR — file it as a follow-up and note the
      carve-out in #2020's closing comment.
- [ ] `buf breaking` **cannot review this PR.** `buf.yaml` declares
      `breaking: use: [FILE]` — exactly the category that would flag these
      deletions — but nothing invokes it. Wire compatibility here is
      human-reviewed only until Phase 5's CI-gate issue lands. Say so in the PR
      description.

**Risk accepted (2b).** Removing a field does not crash an old client — proto3
decodes the absent field as `""` — but clients on **≤0.27.x are still running the
legacy inbox**, which reads these labels. On those builds, inbox rows will render
with blank action-button text, blank status labels, and no type label or emoji
until the user updates. There is **no force-upgrade or minimum-version gate** in
the codebase (nothing matches `min_version` / `force_upgrade` in `server/` or
`app/lib/`), so old builds keep running indefinitely. Cohort 2a does not carry
this risk. If the blank-chrome symptom is judged too visible, holding 2b for one
release while shipping 2a is the fallback — but the stated decision is to ship
both.

**Validates:** That ~15 of the 64 findings are deletable rather than fixable, and
that Group 3's "actual bug risk" is removed rather than documented.
**Docs:** No prose fix needed. `docs/server/portfolio.md` (globs
`server/services/portfolio/**`) does not describe these labels, and
`docs/client/inbox.md`'s `acceptLabel` references are `HomeDecision`, untouched
here — so the issue's proposed "update `docs/client/inbox.md`" task is
**deliberately dropped**. Both docs' `freshness:` stamps go stale; the nightly
`claude_refresh_docs` ratchet re-verifies them.
**Verify:** `npm run generate` first — the proto deletions regenerate Go *and*
Dart. Then `npm run build`, `go test ./server/...`, `npm run lint:proto`,
`flutter analyze`, and the Flutter suite. A deleted proto field is a **compile
error** in Dart, so the client build is the real gate for this phase, not the
server one. `DailyItem` stays live on `GetLibraryItems` and `GetHomeView` —
confirm those responses are otherwise unchanged.

### Phase 3: Type the RSVP and attendance strings
**Goal:** Kill the stringly-typed comparisons across the whole surface, not just
where goconst can see them.
**Tasks:**
- [ ] Add `server/rsvp` with `type Intention string` / `type AttendedStatus string`
      and constant blocks (`IntentionYes`, `IntentionMaybe`, `IntentionNo`;
      `AttendedUnknown`, `AttendedYes`, `AttendedNo`). A top-level `server/<name>`
      shared library is the established shape here — `server/itemkind`,
      `server/quiethours`, `server/eventtime`, `server/contact` are precedents.
      Add `doc.go` per CLAUDE.md; a `README.md` is required only if the package
      ends up multi-file.
- [ ] Route `convertRSVPIntentionToString` (`services/experience/rsvp.go:487`)
      through the new type; it is the single API-enum → storage-string boundary.
- [ ] Sweep all 82 non-test occurrences across the 34 files. Verify each
      `"UNKNOWN"` before converting — `convert_informal_time.go` uses it for time
      precision, not attendance.
- [ ] Unit tests for the new package and its converter.
- [ ] Confirm `models` still imports nothing from `api`
      (`grep "ripls/api" proto/ripls/models/*.proto` returns empty).
- [ ] If any `TODO` in this sweep points at the enum migration, **file that issue
      first** (Phase 5) — CLAUDE.md forbids a `TODO(#NNNN)` whose issue does not
      yet exist.
**Validates:** Whether a typed-constant layer is sufficient, or whether the
remaining friction justifies the full enum migration.
**Verify:** This is a cross-package signature change (`convertRSVPIntentionToString`
plus 34 files spanning `known_for/`, `notifications/`, `jobs/`, `esm/`, `story/`,
`impact_metrics/`, and seven `services/` packages), so `npm run build` alone is
insufficient — it does not compile `//go:build integration` files. Per
`docs/server/conventions.md` §Go Server Build Verification, also run
`go build -tags=integration ./...`, then `go test -tags=integration ./server/...`
(what CI runs). Run these locally; do not defer validation to CI.

### Phase 4: Enable goconst as a regression guard
**Goal:** Lock in the work.
**Tasks:**
- [ ] Re-measure with `uniq-by-line: false` and `--tests=false`. Expect a small
      residual (`ai/eval/experience.go` `"all"` ×6,
      `impact_metrics/metric_detail_*.go` `"Unknown"`/`"Giveaways"`/`"Loans"`,
      `services/experience/convert_informal_time.go` precision tokens).
- [ ] Resolve the residual — real constants where they are real constants; note
      any that are user-facing copy on a live path and route them to the l10n
      issue instead.
- [ ] Drop `min-occurrences` from 20 to 3 in `server/.golangci.yml`; remove the
      `goconst` entry from the parked block and its comment.
- [ ] Update #2816 to strike goconst from its ladder.
- [ ] Per `docs/server/conventions.md`, enable only when findings are **fixed**,
      not suppressed — no `//nolint:goconst` may be added to reach green.
**Validates:** `npm run lint:go` green with goconst at target; the defect cannot
regress.
**Verify:** `npm run build`. Re-measure with `uniq-by-line: false` still set in
`server/.golangci.yml` — per #1611, any count taken with other linters active and
`uniq-by-line` defaulted to `true` is wrong.

### Phase 5 (separate issues — file, don't implement here)
**Goal:** Capture what this investigation surfaced but should not absorb.
**Tasks:**
- [ ] File: **live server-rendered in-app copy.** `home_view_decisions.go`
      (`kind_label`, `accept_label`, `why`) and `workshop/brief.go:397`
      (`action_label`) render English the client displays. This is the
      `docs/server/l10n.md` §Adding a new in-app server-emitted string migration.
      Note that #1904, which that doc names as the owner of "Phase 4", is
      **CLOSED** — so this work currently has no tracking issue.
- [ ] File: **wire `buf breaking` into CI.** `buf.yaml` declares the config;
      nothing runs it. Prerequisite for the `models.RSVPIntention` enum migration.
- [ ] File: **`models.RSVPIntention` / `models.AttendedStatus` enum migration.**
      New field numbers, dual-population, backfill, retirement of fields 5 and 6.
      Per CLAUDE.md, file the backfill *cleanup* issue before the `TODO(#NNNN)`
      lands. Depends on the `buf breaking` gate.
- [ ] Consider filing: **is `GetPortfolioInboxFeed` dead?** No client caller
      found. If confirmed, RPC removal is its own piece of work.
- [ ] Fix the dangling reference in #2820's body to
      `docs/issues/2816-lint-ratchet.md`, which does not exist.

## Related Issues
- **#2816** — the funlen/cyclop/dupl/goconst ratchet this was split from. Phase 4
  here removes goconst from its ladder. Its body, not the cited missing doc, is
  the spec.
- **#1611** — the lint-enforcement rollout; source of the `uniq-by-line` trap that
  makes any measurement taken with other linters active wrong.
- **#2020** — the deprecated entity-card fields from #2012, spanning
  `portfolio.proto` and `workshop_service.proto`. **Phase 2 executes and closes
  it**, folding in the eight label fields its body currently (and now wrongly)
  lists under "Keep". One acceptance criterion — the `EntityKind` migration for
  `MetricLineItem` / `SharedItem` / `DailyAction` — is carved out as a follow-up.
- **#2012** — CLOSED 2026-05-19. The consolidation that created #2020's
  deprecations; its four-release age is what makes #2020's cohort safe to cut.
- **#1904** — CLOSED. `docs/server/l10n.md` still points at its "Phase 4" for the
  in-app structured-template migration, which is the fix for the live violation
  in Phase 5. The doc's forward reference is stale.
- **#1424** — the 1,000-line Go file gate. `assembly.go` is 672 lines; Phase 2's
  deletions move it further from the gate, not toward it.

---

## Audit (`/ripls-audit`, 2026-07-30)

**Scope:** server-only Go, `proto/ripls/api` + `proto/ripls/models`, lint config.
**Engaged dimensions:** `code_quality`, `tech_debt`, `api_design`, `testing`, plus
the architecture/conventions anchors. Not engaged: accessibility (no UI),
security, observability, performance, dependency — no phase touches auth, secrets,
logging, hot-path query shape, or `go.mod`.

**Findings applied:**

1. **No validation commands (all phases).** `docs/server/conventions.md` §Go
   Server Build Verification requires `npm run build` after any Go change, and
   `go build -tags=integration ./...` for cross-package signature changes — which
   Phase 3 is, across 34 files. Added a **Verify** block to every phase, with the
   integration-tag build and test called out on Phase 3.
2. **Interim goconst floor was asserted, not measured.** Phase 1 proposed
   enabling `min-occurrences: 20` without evidence it is green. Ran it: **zero
   findings on `main`** (worst literal `"YES"` ×19, one under threshold). Recorded
   the result and reframed it as a growth guard, not a gate.
3. **Proto-comment convention violation.** Phase 2 originally instructed writing
   the release-window rationale into the `.proto` deprecation comments.
   `docs/proto_conventions.md` §Proto comments bans server-side mechanics and
   project conventions from `.proto` files. *Superseded by the 2026-07-30
   decision — Phase 2 now deletes the fields rather than annotating them, so no
   new proto comment is written. The convention still governs the `reserved`
   markers left behind: keep them bare.*
4. **`USER_PRIMARY_LOCATION` leak risk.** The same section names this sentinel
   explicitly as forbidden in proto comments. Added a guard to Phase 1.
5. **Over-specified package scaffolding.** Phase 3 required a `README.md` for
   `server/rsvp`; CLAUDE.md requires one only for **multi-file** directories.
   Softened to conditional, and cited `server/itemkind` / `server/quiethours` /
   `server/eventtime` / `server/contact` as precedent for the top-level shared-
   library placement.
6. **TODO hygiene ordering.** CLAUDE.md forbids a `TODO(#NNNN)` referencing an
   issue that does not exist yet. Added an explicit Phase 3 → Phase 5 dependency.
7. **Field reservation + missing `buf breaking` review.** `api_design` §4 requires
   removed fields be reserved; added the citation. Also attached the "CI cannot
   check this" caveat to the Phase 2 task where it is actionable, not only to the
   Problem Statement.
8. **Doc-freshness impact was unstated.** Verified `docs/server/portfolio.md` does
   not describe these labels and `docs/client/inbox.md`'s `acceptLabel`
   references are `HomeDecision` (untouched by Phase 2). Recorded that the issue's
   proposed `docs/client/inbox.md` update is deliberately dropped, and that the
   nightly ratchet will still re-verify both stamps.
9. **Suppression escape hatch.** Added the `docs/server/conventions.md` rule that
   a parked linter is un-parked by fixing findings, not by `//nolint`.

**No unresolved conflicts** with existing documented decisions. The plan's central
divergence from the issue — delete the dead labels rather than migrate them to ARB
— is consistent with `docs/server/l10n.md` (the render boundary applies to strings
users read; these reach no user) and `code_quality` §5 (Dead Code: flag all forms
for removal).

### Re-audit after the 2026-07-30 acceleration decision

Phase 2 was rewritten from "deprecate and wait" to "delete now and close #2020".
Re-checked the affected dimensions:

- **`api_design` §4 (Backwards Compatibility).** The deletion is now in-PR rather
  than deferred, so field-number reservation moves from a future task to a
  blocking one — stated on both cohorts. The "`buf breaking` is declared but never
  invoked" gap escalates from a footnote to the *only* control on a wire-breaking
  change; Phase 2 now requires calling it out in the PR description.
- **New risk surfaced and recorded.** Deleting a proto field is a **Dart compile
  error**, not just a Go change — so `npm run generate` + `flutter analyze` + the
  Flutter suite become the real gate for Phase 2. The original phase listed only
  `npm run build`, which would not have caught it.
- **Scope check against `architecture` / `tech_debt`.** Closing #2020 pulls in
  `workshop_service.proto`, `services/profile/postcards.go`, and
  `services/workshop/brief.go` — beyond `server/services/portfolio/**`. That is
  acceptable for a deletion pass, but one #2020 acceptance criterion (the
  `EntityKind` migration for `MetricLineItem` / `SharedItem` / `DailyAction`) is
  an unsettled *design* question its own body marks "separate scope if
  non-trivial". Carved out as a follow-up and flagged in Open Questions as the
  single point of pushback on "close out #2020 in this PR".
- **User-facing risk stated, not buried.** ≤0.27.x clients still run the legacy
  inbox and read these labels; there is no force-upgrade or minimum-version gate
  anywhere in `server/` or `app/lib/`. Symptom is blank inbox chrome, not a crash.
  Recorded as accepted, with two-commit separation as the revert path.
- Findings 1, 2, 4, 5, 6, 8 and 9 above are unaffected. Finding 3 is superseded
  (no new proto comment is written); finding 7 is strengthened.

**Docs consulted:** `docs/llms.txt`; `docs/server/architecture.md`;
`docs/server/conventions.md`; `docs/proto_conventions.md`; `docs/server/l10n.md`;
`docs/server/portfolio.md`; `docs/client/inbox.md`; `docs/client/i18n.md`;
`docs/reviews/code_quality_review_prompt.md`;
`docs/reviews/tech_debt_review_prompt.md`;
`docs/reviews/api_design_review_prompt.md`;
`docs/reviews/testing_review_prompt.md`; `CLAUDE.md`.

---

## What shipped (2026-07-30)

All five phases landed on `consts`. goconst is enabled at `min-occurrences: 3`
and reports **0 findings**, down from 64.

| Phase | Commit | Notes |
|---|---|---|
| 1 — mechanical constants | `3a615b06a` | Also swept the `STORY_TYPE_*` literals across 6 packages, wider than planned. |
| 2a — #2020's cohort | `ba42ed8d9` | 19 proto fields deleted and reserved. |
| 2b — dead inbox labels | `45529129e` | 8 more fields; the string-keyed branching went with them. |
| 3 — RSVP vocabulary | `01ffadca7` | `server/rsvpstate`; 82 occurrences across 34 files. |
| — bug fix | `7bdd10198` | Three nudges that reached nobody. |
| 4 — enable goconst | `76d8e8ab0` | Plus the residual constants. |

### Where it diverged from the plan

**The interim goconst floor was abandoned, not tuned.** Phase 1 planned to
enable `min-occurrences: 20` as a growth guard, on the strength of a
measurement showing zero findings. That measurement used `--tests=false`; the
real gate counts test files, where `"YES"` is 30 occurrences rather than 19.
An `exclude-rules` entry on `_test.go` does not help — it filters where an
issue is *reported*, not what it counts. The right mechanism is goconst's own
`ignore-tests: true`, which Phase 4 sets. No threshold both cleared the
pre-Phase-3 tree and caught anything, so the floor was dropped and goconst
enabled once, at the end.

**Phase 2a was larger than "delete six fields".** `completed_inbox_feed.go`
and `library_items.go` never called `populateRowItem`, so their rows carried
entity data *only* in the deprecated fields — `Item` was empty on the wire for
the Done feed and the entire "Your Stuff" library. Removing the fields without
noticing would have left those rows with no title and no media. Both populate
`Item` now. Two of #2020's acceptance criteria were also already satisfied:
`AvailableNowItem`/`AvailableNowKind` no longer exist in `proto/`, and the
`@Deprecated('use pushToEntity')` shim was never added.

**Phase 3 found a live bug.** Three RPCs compared `rsvp.Intention` against
`"RSVP_INTENTION_YES"` / `"RSVP_INTENTION_MAYBE"` — the prefixed enum names —
while every writer stores the bare `"YES"` / `"MAYBE"`. `NudgeLocationPollVoters`,
`NudgeTimePollVoters`, and `NudgeUncoveredNeedClaimers` had been sending zero
notifications for as long as that code existed. The existing test asserted only
that the RPC returned no error, with a comment explaining the count was 0
because no notification service was wired — a vacuous assertion that passed
forever. It now wires the recording mock; reverting the fix makes it fail with
`NudgedCount = 0, want 1`. This is the strongest evidence for the issue's core
claim: the findings were symptoms, not formatting.

**Phase 4 excluded rather than fixed 14 findings.** The user-facing labels on
live paths (`impact_metrics/metric_detail_*`, `portfolio/home_view_*`,
`community_ranking`, `actions`, `experience/service`, `location_proposals`) are
excluded by path in `server/.golangci.yml`, pointing at #2827. Hoisting them to
Go constants would have entrenched the render-boundary violation and given the
ARB migration one more indirection to unwind. **That exclusion should be deleted
when #2827 lands.**

**`server/rsvpstate` holds untyped constants, not a defined type.** The plan
called for `type Intention string`. The model fields are `string`, so a defined
type would force a conversion at every call site while still permitting
comparison to a bare literal — noise without safety. Typing belongs with the
enum migration (#2829). The package is also named `rsvpstate` rather than `rsvp`
because `rsvp` is a ubiquitous local variable name in the experience service.

### Follow-ups filed

- **#2827** — the live server-rendered in-app copy (`HomeDecision.kind_label`,
  `accept_label`, `why`; `WorkshopBriefRow.action_label`; the impact-metric
  labels). This is the real owner of "#1904 Phase 4", which had none: #1904 is
  closed while `docs/server/l10n.md` still points forward at it.
- **#2828** — wire `buf breaking` into CI. It reviewed none of the 27 proto
  field deletions in this work.
- **#2829** — the `models.RSVPIntention` / `AttendedStatus` enum migration.
  Depends on #2828.
- **#2830** — the `EntityKind` question carved out of #2020's acceptance list.

#2020 is closed; #2816 updated to strike goconst from its ladder.

### Not done

`GetPortfolioInboxFeed`, `GetPortfolioInboxView`, and `GetMyStuffItems` have no
viewmodel caller in `app/lib` — the whole legacy portfolio-inbox RPC family
appears client-dead. This work deleted fields, not RPCs, per the plan's scope.
Whether those RPCs should go is a larger question and still open.
