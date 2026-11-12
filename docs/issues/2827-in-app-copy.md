# Issue #2827: [tech-debt] Move server-rendered in-app copy to the client (the real #1904 Phase 4)

**Source:** <https://github.com/Type2Fun-Foundation/ripls/issues/2827>
**Filed:** 2026-07-30 | **Last activity:** 2026-07-31
**Labels:** review:code_quality | **Assignees:** unassigned
**Context docs:** `docs/server/l10n.md`, `docs/client/i18n.md`, `docs/proto_conventions.md`, `docs/client/inbox.md`, `docs/client/architecture.md`, `docs/client/conventions.md`, `docs/server/architecture.md`, `docs/server/conventions.md`, `docs/users/priorities.md`, `docs/impact_metrics.md` (§ Metrics Screen Reference), `docs/client/chat.md` (§ system messages); review rubrics: `api_design`, `code_quality`, `testing`, `tech_debt`, `accessibility` (Areas to Review)

> **UI investigation skipped:** this is a render-boundary/tech-debt issue, not
> UI-observable misbehavior. In the English locale every screen looks correct;
> the defect only manifests as untranslated fragments for a Spanish-locale
> viewer (e.g. a Spanish chat sentence with a literal English `TBD` spliced
> in). Nothing to bisect in the hermetic environment.

## Problem Statement

`docs/server/l10n.md` § "The render boundary" says the client renders every
string a user reads inside the app; the server renders only terminal-emitter
surfaces (push, email, SMS). Several live RPC responses violate this: the
server composes English copy (`"Lend it"`, `"due Mon"`, `"Loans"`, `"TBD"`)
into proto string fields and the client renders it verbatim. A Spanish-locale
user gets English fragments inside otherwise-localized screens, and the
server's `time.Format`-based date strings (`"Mon"`, `"Jan 2"`, `"3:04 PM"`)
are structurally untranslatable server-side.

The doc's escape hatch ("Phase 4 of #1904 covers the structured-template
migration; until that lands…") points at a **closed** issue, so the migration
has no owner. #2827 is that owner.

The maintainer's follow-up comment already corrected the original sizing: the
fix is mostly **structure, not translation** — delete label fields the client
can derive from enums it already receives, replace pre-formatted date strings
with timestamps the client formats in the viewer's locale, and split fields
that mix user content with composed copy. This plan's codebase research
(2026-07-31, branch `in_app_copy` at `ec80ab0cb`) corrects the surface
inventory further — see the next section, because several "live" strings in
the issue turn out to be dead on the wire, and one live violation isn't listed
in the issue at all.

## Context from Comments

One comment, from the maintainer (maintainer), 2026-07-31:

- **`kind_label` / `accept_label` should be deleted, not translated** — the
  client can switch on `HomeDecisionKind` and `HomeTransferAction`, which
  exist precisely so the client doesn't parse labels.
- **`why` is two fields wearing one**: sometimes a chat-message preview (user
  content, must pass through untranslated), sometimes composed copy
  (`"due " + homeDayLabel(...)`). Splitting it needs a typed reason enum plus
  a timestamp.
- **`homeDayLabel` emits English dates** — the fix is a raw timestamp the
  client formats, a proto change rather than a copy change.
- **Composed titles** (`"%s wants your %s"`) need a template key plus params,
  not a lookup.
- **Corrected liveness**: `impact_metrics/metric_detail_*` is shared by
  `GetCommunityMetricDetail` (live caller), so those labels are in scope even
  though the portfolio-side wrappers were deleted (#2831 / commit
  `24a2bc942`). `"Lent"` (`portfolio/actions.go`) left the surface with that
  deletion.
- **Suggested split: three PRs** — (1) HomeDecision, (2) home_view_gear ICU
  plurals, (3) impact metric detail — plus the guard-script gap and per-file
  removal of the `goconst` exclusion.
- **`app_es.arb` is parity-gated, and every key needs a real Spanish
  translation** (stricter than the #1972 "English placeholder is fine"
  baseline — treat the comment as the bar for this issue).

## Corrected surface inventory (research findings)

The issue and comment both predate two findings that change the work
materially. Verified by repo-wide grep on 2026-07-31; each PR should re-verify
its slice before building on it.

### Live — client renders these today

| Surface | What's live | Reader |
|---|---|---|
| `HomeDecision.accept_label` | `"Lend it"`, `"Give it"`, `"Mark picked up"`, `"Mark returned"`, `"Say thanks"`, `"Done"`, `"Reply"` | `home_tab_screen.dart:196`, `needs_you_group_row.dart:93,207,217`, `home_decision_routing.dart:146,150` (interpolated into the confirm-dialog l10n template) |
| `HomeDecision.why` | Mixed: chat preview (user text) OR composed `"pickup Mon"`, `"due Jun 20"`, `"ready to pick up"`, `"borrowing"`, `"Was Mon · how'd it go?"` (FNV-picked phrase, `home_view_format.go:23-38`) | `home_tab_screen.dart:220`, `needs_you_group_row.dart:168,185` |
| `HomeDecision.title` | For LEND/GIVE kinds only: composed `"%s wants your %s"` (`home_view_decisions.go:123`); other kinds are user-data pass-through | `home_tab_screen.dart:192`, `needs_you_group_row.dart:59` |
| `SourceBreakdown.label` (`metric_detail_calculator.go:555-562`) | `"Loans"` / `"Giveaways"` / `"Requests"` / `"Events"` — a **live string-matching contract**: all three workshop detail screens hardcode `_sourceOrder = ['Loans', 'Giveaways', 'Requests', 'Events']` as lookup keys and render the label verbatim | `workshop_money_detail_screen.dart:95,153`, `workshop_time_detail_screen.dart:101`, `workshop_co2_detail_screen.dart:104` |
| `RecentActivity.label` / `.person_name` / `.value` | Fallbacks `"Item"`, `"Help request"`, `"Community Event"`, `"Unknown"`; formatted values `"%.0f min"`, `"%.1f hrs"`, `"%.1f kg"` (`metric_detail_subsidiary.go`, `metric_detail_rankings.go`) | `recent_items_list.dart:85-142` on all three workshop detail screens |
| `TimeSeriesPoint.label` | `.Format("Jan")` English month abbreviations (`metric_detail_subsidiary.go:220`) | money screen chart via `cumulativeTrend` (`workshop_money_detail_screen.dart:130`) |
| Chat system-message **params** | `"TBD"` / `"Mon, Jan 2 · 3:04 PM"` (`experience/service.go:469-489`), `"the new location"` / `"unknown location"` (`experience/location_proposals.go:777-796`, `service.go:447-449`) — English **param values** spliced into already-localized ARB sentence templates (`app_es.arb:389-393`) | `system_message_text_resolver.dart:82-91` via `message_list.dart:262` |
| GetHomeView **calendar** strings (not listed in the issue) | `home_view_upnext.go` / `home_view_format.go` copy that reaches the `calendar` field (`"Hosting · %d going"`, `"Pick up %s from %s"`, `"All day"`, `"3:04 PM"` clock labels, `homeDayLabel` output) — the calendar **is** rendered as the Plans dock tab (`docs/client/inbox.md` § Calendar data) | `home_calendar_screen.dart` + `widgets/home/calendar/` |

### Dead on the wire — populated, but no client reader

| Field(s) | Evidence |
|---|---|
| `HomeDecision.kind_label` | Zero readers; the only `kindLabel` hit in `app/` is an unrelated local method in `workshop_problems_solved_detail_screen.dart`. The see-all screen's group headers are already client-side l10n (`homeNeedsGroup*` keys, `home_needs_you_see_all_screen.dart:63-93`) |
| `HomeGearItem.title/.subtitle/.pill_label`, `HomeOwnedEvent.subtitle` (all of `home_view_gear.go`'s copy) | The #2634 rewrite deleted the Yours rendering; the fields are deserialized into `HomeTabState.visibleGear/visibleEvents` and never rendered (`docs/client/inbox.md` § "Yours data (no longer rendered)"). The client already receives `HomeGearCategory`, `HomeGearPillStyle`, and `due_at_unix_sec` — everything needed to rebuild the copy locally *except* the gear/counterparty names baked into `title` |
| `GetHomeViewResponse.up_next`, `recent_activity` (all of `home_view_activity.go`'s copy) | Assembled, unconsumed post-#2634 (`docs/client/inbox.md` § Data Model) |
| `ContributingFactor.*` — every string in `metric_detail_factors.go` (5 factor labels, ~20 stat labels, 6 composed insight sentences) | `GetCommunityMetricDetailResponse.factors` has **zero** client readers. The maintainer's table counts these as live because `computeFactors` is reachable from a live RPC — they are live *on the wire* but nothing renders them |
| `TopContributor.detail/.name`, `SubsidiaryMetric.*`, `TimeSavedDetail.avg_per_loan` (the `"—"`), `formatted_total`, `money_detail`, `library_items`, `comparison` | Zero client readers each |
| `TopItem.*` | Read only by `drill_down/*.dart` code whose `detail` argument is always `null` in production (`item_metrics_screen.dart:560-577` calls only the item-level builders); the sole callers passing a detail are in `drill_down_builder_test.dart` |
| `BriefCTARow.action_label` (`workshop/brief.go:397`) | No Dart accessor; `ctaRows` never read; the one client test fixture constructs the row without the field. The maintainer's table correctly omits it |
| `server/services/portfolio/community_ranking.go` | **File already deleted** (commit `24a2bc942`, 2026-07-30) — the issue's reference is stale. The only surviving `"—"` is `metric_detail_calculator.go:299` (dead, above) |

### Adjacent findings to carry into the work

- **Latent client bug:** `conversation_messages_notifier.dart:453-455`
  patches `action` and `description` from a `SystemMessageUpdate` but drops
  `templateKey`/`templateParams` (which the proto carries,
  `chat_service.proto:148-149`). After a coalesce-driven update the card
  renders the *stale* template. Must be fixed before/with the chat-param
  migration or coalesced cards will show outdated text.
- **Formatting round-trip hazard:** `parseDollarValue`
  (`metric_detail_rankings.go:474-497`) re-parses the server's own display
  strings (`"$"`, `" hrs"`, `"K"`…) to sort `TopItem`s. Any change to value
  formatting must give the sort a numeric input instead.
- **Orphaned server code:** `ComputePortfolioMetricDetail`
  (`metric_detail_calculator.go:238`) and `computePortfolioLibraryItems`
  (`metric_detail_rankings.go:816`) survived commit `24a2bc942` with zero
  callers.
- **Guard gap confirmed:** `scripts/check_no_inline_user_strings.js` has no
  allowlist mechanism; `INLINE_PATTERN` requires `fmt.Sprintf("Capital…{8,}")`
  and `PROSE_PATTERN` requires trailing `.!?` — `"Lend it"`, `"Borrowing"`,
  `"TBD"` match neither, so adding files to `SCANNED_FILES` alone is a no-op.
- **Precedent for typed source labels:** `ActsCategoryBreakdown`
  (`impact_service.proto:266-280`) already ships the stable-key +
  fallback-label pattern, and `RecentActivityKind` / `ProblemSolvedKind` are
  in-file enum precedents.
- **E2E copy coupling:** `e2e/tests/workflows/loan-return-mark-returned.spec.ts:139-149`
  locates the Needs-you pill by `getByRole('button', { name: 'Mark returned' })`.
  Client-side rendering must produce the identical English string.

## Potential Solutions

### Option A: Translate everything listed (template keys / ARB for every string)

- **Approach:** Take the issue's inventory at face value: for each server
  string, add `{template_key, params}` (or equivalent) and an ARB pair, and
  render client-side — including the factor cards, stat tiles, insights,
  contributor details, and gear rows.
- **Pros:** Mechanically faithful to the doc's stated migration recipe;
  nothing is left behind; the `goconst` exclusion comes out wholesale.
- **Cons:** Translates ~90 strings that no screen renders (factors, stats,
  insights, subsidiary metrics, gear rows, `TopContributor.detail`), plus
  real Spanish translations for each — pure waste, and it entrenches dead
  API surface the repo has been actively pruning (#2820, #2831). Violates
  the "no speculative API" ethos in `proto_conventions.md`.
- **Effort:** Large (and mostly wasted).

### Option B: Structure-first — enums + timestamps for live surfaces, deprecate dead copy fields (recommended)

- **Approach:** For each **live** surface, replace rendered copy with typed
  structure the client already mostly has (`kind`, `transfer_action`, a new
  reason enum + timestamp, a new source-type enum, timestamps instead of
  formatted dates), render from ARB at widget build time, and keep the old
  string fields populated-but-deprecated for the two-release skew window
  (`proto_conventions.md` § Removing a field from `proto/ripls/api`). For
  each **dead** surface, do not translate: mark the copy fields deprecated
  with a `TODO(#NNNN)` pointing at a batched removal issue, and stop treating
  them as l10n scope.
- **Pros:** Matches the maintainer's own re-sizing ("delete, not translate");
  every ARB key added is one a user actually sees; kills the
  `_sourceOrder` string-matching contract (a real correctness landmine — a
  server label rename silently breaks three screens' render order); dates
  become locale-correct via Dart `intl` instead of a parallel Go date
  formatter; dead fields exit via the same removal protocol #2820 already
  exercised.
- **Cons:** More proto design work than a string sweep; two-release wait
  means the server keeps emitting English fallbacks until ~v0.31; needs a
  scope decision on the gear/up-next/activity dead sections (prune vs.
  hold).
- **Effort:** Medium-large, split across 4 shippable PRs.

### Option C: Minimal live-string patch (no proto changes)

- **Approach:** Only fix what a Spanish user can see, with the smallest
  possible diff: distinguishing template keys for the chat `"TBD"` /
  `"the new location"` params, client-side lookup tables keyed on the
  existing English label strings (`"Lend it"` → ARB key, `"Loans"` → ARB
  key), leaving the wire contract byte-identical.
- **Pros:** Small; no deprecation dance; no proto review.
- **Cons:** Enshrines English strings as wire-format keys — the exact
  anti-pattern `HomeTransferAction` was added to remove, and the doc's
  migration recipe explicitly rejects. Date strings (`"due Mon"`,
  `"Was Mon"`, `"Jan"`) are *inside* composed fields, so they can't be
  localized by lookup at all — the biggest visible violation survives.
  Leaves every dead field in place and the `goconst` exclusion permanent.
- **Effort:** Small, but doesn't actually close the issue.

## Recommendation

**Option B.** The maintainer's comment already points there for HomeDecision
("can be deleted, not translated"); the research extends the same logic to
most of the impact surface (dead on the wire → deprecate, don't translate)
and shrinks the genuinely-translatable core to: HomeDecision's
`accept_label`/`why`/composed-title, the impact source labels + recent-item
fallbacks + value/month formatting, the two chat params, and the Plans-tab
calendar strings. Option A spends real translation effort on invisible
strings; Option C cannot fix the date-format half of the problem.

Sequencing follows the comment's split with two corrections: PR 2 becomes the
chat params (small, self-contained, fixes the one violation Spanish users hit
in *sentences* today) instead of `home_view_gear` (whose strings turn out to
be unrendered — that file needs a prune-vs-hold decision, not an ICU
migration), and the Plans-tab calendar strings are added as an explicit
fourth surface since they're the same class of live violation inside the same
`home_view_*` file family the `goconst` exclusion already covers.

## Open Questions

- **What happens to the dead copy fields (`home_view_gear.go` rows,
  `up_next`/`recent_activity`, `factors`/stats/insights,
  `TopContributor.detail`, `SubsidiaryMetric`, `TopItem` strings,
  `BriefCTARow.action_label`)?** — Recommendation: exclude them from this
  issue's translation scope; mark them `[deprecated = true]` with
  `TODO(#NNNN)` in the per-surface PRs and file **one batched removal issue**
  (the #2020 pattern) that executes the two-release protocol. If instead the
  Yours/Library surface is expected to come back, `HomeGearItem` needs
  structured name fields (`gear_name`, `counterparty_first_name`) before its
  composed `title` can ever be dropped — that decision belongs to the removal
  issue, not this one.
- **Are the Plans-tab calendar strings (`home_view_upnext.go` /
  `home_view_format.go` reaching the `calendar` field) in scope for #2827?**
  — Recommendation: yes (PR 4). They are live render-boundary violations in
  files the issue's own `goconst` exclusion covers; the issue simply predates
  the check of which `home_view_*` outputs are still rendered. If the
  maintainer prefers a separate issue, PR 4 splits off cleanly.
- **Enum or key+label for the impact source types?** — Recommendation: a
  proper `ImpactSourceType` enum (next to `RecentActivityKind`, following the
  prefix/UNSPECIFIED conventions), not the `ActsCategoryBreakdown` string-key
  pattern. The value set is closed and already mirrors `TransferType`; an
  enum gives the Dart switch exhaustiveness checking, which is the point of
  killing `_sourceOrder`.
- **How should `why`'s user-content case be carried after the split?** —
  Recommendation: add a dedicated `message_preview` field (user text only)
  alongside the new reason enum + timestamp, and deprecate `why` entirely,
  rather than redefining `why` as "preview only" — redefining a live field's
  semantics under old clients violates the skew rules.
- **Does the new-key Spanish bar ("real translation, not placeholder") apply
  to strings that must stay byte-identical in English for e2e?** — Not a
  conflict: e2e runs in English; `app_es.arb` gets real Spanish. Flagged only
  so the translation pass isn't skipped on the grounds that "the string must
  equal the old server output".
- **Guard design:** per-file strict mode is only viable for files that should
  contain *no* user-facing literals after migration. Recommendation: add a
  `NO_COPY_FILES` list to `check_no_inline_user_strings.js` that flags any
  double-quoted literal matching `^[A-Z][a-z]` or containing a space
  (comment lines and `experience-string-allow`-style pragmas excluded) in the
  migrated server files. Cheap, and false positives are rare precisely
  because the files are supposed to be copy-free.

## Implementation Plan

Each phase is an independently shippable PR. Common validation for every
phase: `npm run generate`, `npm run build` (Go lint + compile),
`go test ./...`, `cd app && flutter analyze && flutter test --reporter
expanded`, `npm run lint:proto:breaking`, `npm run lint:dart:l10n-parity`.
Phase 2 additionally needs `go build -tags=integration ./...` — the
experience package has integration-tagged tests that `npm run build` does
not compile (`server/conventions.md` § Build Verification).
New ARB keys land in `app_en.arb` **and** `app_es.arb` (real Spanish) in the
same commit.

### Phase 1: HomeDecision (PR 1)

**Goal:** The Needs-you queue renders every label, reason line, and headline
from the client ARB, driven by typed fields; the deprecated copy fields keep
flowing for old builds.

**Tasks:**
- [ ] Proto (`portfolio.proto`): add `HomeDecisionReason reason = 23`
      (`UNSPECIFIED`, `MESSAGE_PREVIEW`, `PICKUP_SCHEDULED`,
      `READY_TO_PICK_UP`, `RETURN_DUE`, `BORROWING`, `PAST_EVENT`,
      `PAST_REQUEST`), `optional int64 due_at_unix_sec = 24` (naming parity
      with `HomeGearItem.due_at_unix_sec`), `optional string
      message_preview = 25` (user text only). Mark `kind_label = 3`,
      `why = 5`, `accept_label = 6` `[deprecated = true]` with
      `TODO(#NNNN)` referencing the batched removal issue (file it first).
      Declare the new fields adjacent to `why` in the message body — field
      *numbers* are append-only but declaration *position* should sit with
      the semantically related fields (`feedback_proto_field_order`).
      Field comments describe meaning only — no server mechanics
      (`proto_conventions.md` § Proto comments).
- [ ] Server (`home_view_decisions.go`): populate the new fields alongside
      the old strings. `due_at_unix_sec` carries the pickup/return/past
      timestamp the copy used to bake in (today it's smuggled through
      `created_at_unix_sec` as `sortAt` — leave that ordering behavior
      untouched).
- [ ] Client: `accept_label` chip + confirm dialog resolve from
      `(kind, transfer_action)` via new `home*` ARB keys whose English values
      are byte-identical to today's server output ("Mark returned", "Lend
      it", …). `why` renders by `reason`: `MESSAGE_PREVIEW` →
      `message_preview` verbatim; dated reasons → ARB templates over a shared
      Dart day-label helper (`today` / `DateFormat.E()` / `DateFormat.MMMd()`
      — the locale-aware mirror of `homeDayLabel`); `PAST_*` → "Was {day} ·
      {phrase}" with the phrase picked client-side by a stable hash of the
      decision id (preserves today's deterministic variety). LEND/GIVE
      headline composes `l10n.homeDecisionWants(counterpartyFirst,
      subjectTitle)` from fields the message already carries; other kinds
      keep rendering `title` (pure user data).
- [ ] Tests: rewrite the ~10 English assertions in
      `home_view_decisions_test.go` (+1 in `home_view_asks_test.go`) to
      assert enums/timestamps; keep one assertion per deprecated field
      proving it still flows until removal. Re-fixture
      `needs_you_group_row_test.dart` / `home_editorial_row_test.dart` to the
      enum-driven API (l10n delegates via `app/test/helpers/l10n_helpers.dart`).
- [ ] Run `e2e/tests/workflows/loan-return-mark-returned.spec.ts` — the
      'Mark returned' pill must still match by name. Note the chip's text is
      also its **semantics label** (`needs_you_group_row.dart:207`): the
      resolved l10n string is what screen readers announce and what the e2e
      `getByRole(name:)` locator matches, so the l10n-sourced label satisfies
      both (this is the `client/accessibility.md` convention — l10n-sourced
      semantic labels — not a new requirement).
- [ ] Narrow the `goconst` path exclusion: `home_view_decisions` drops out of
      the `services/portfolio/home_view_` pattern (list remaining files
      explicitly).

**Validates:** the deprecate-and-derive pattern end to end — proto skew
handling, ARB parity, e2e copy identity — before the mechanical surfaces
reuse it.

### Phase 2: Chat system-message params (PR 2)

**Goal:** No English is synthesized into `template_params`; a Spanish viewer
gets a fully-Spanish sentence including time and location fallbacks.

**Tasks:**
- [ ] Server: `formatExperienceTime` stops emitting `"TBD"` and pre-formatted
      dates — emit structured params (`time_unix_sec`, `all_day` / a
      `time_kind` discriminator; informal user `Description` passes through
      as today) with the rendered English kept in `Text`/`description` as the
      old-client fallback. `locationDisplayName` / `getLocationName`: when no
      name/locality exists, emit a distinguishing template key
      (`chat.experience.detail_changed_location_unnamed`) instead of the
      English phrase.
- [ ] Client: extend `system_message_text_resolver.dart` for the new
      keys/params; format times with `intl` `DateFormat` in the viewer's
      locale; ARB pairs for TBD/unnamed-location variants.
- [ ] Fix `conversation_messages_notifier.dart:453-455` to carry
      `templateKey`/`templateParams` through `SystemMessageUpdate` (latent
      staleness bug; required for coalesced DETAIL_CHANGED cards to re-render
      correctly).
- [ ] Tests: extend `server/chat/system_messages_localization_test.go` and
      resolver widget/unit tests; cover the update-path fix.
- [ ] Drop `services/experience/service` and
      `services/experience/location_proposals` from the `goconst` exclusion.

**Validates:** the params-not-prose contract for chat — the one surface where
Spanish users see mixed-language sentences today.

### Phase 3: Impact metric detail (PR 3)

**Goal:** The three workshop detail screens key on a typed source enum and
localize labels/units/months; dead impact copy is deprecated, not translated.

**Tasks:**
- [ ] Proto (`impact_service.proto`): add `ImpactSourceType` enum
      (`UNSPECIFIED`, `LOANS`, `GIVEAWAYS`, `REQUESTS`, `EVENTS`) and a
      `source_type` enum field on `SourceBreakdown`; deprecate
      `SourceBreakdown.label`. Add a numeric value field
      (`double raw_value`) + reuse the request's dimension for units on
      `RecentActivity`; deprecate the formatted `value`. Add a
      `bucket_start_unix_sec` to `TimeSeriesPoint`; deprecate `label` for
      month buckets (or keep `label` for non-date buckets and document
      which).
- [ ] Server: populate the new fields (`metric_detail_calculator.go`,
      `metric_detail_subsidiary.go`, `metric_detail_rankings.go`); set
      `RecentActivity.kind` in `computeSocialRecentActivity` (currently left
      UNSPECIFIED at L279/309/341); stop inventing `"Item"` / `"Help
      request"` / `"Community Event"` / `"Unknown"` in new fields — absent
      name + `kind` is enough for the client to render a localized fallback.
      Give the `TopItem` sort a numeric input so `parseDollarValue`'s
      string-re-parsing isn't load-bearing for anything new.
- [ ] Client: replace `_sourceOrder` string arrays in the three workshop
      screens with an `ImpactSourceType` switch + `metricDetail*` ARB labels;
      `recent_items_list.dart` renders localized fallbacks and formats
      values/units/months via `intl`.
- [ ] Dead fields (`factors`, `TopContributor.detail`, `SubsidiaryMetric`,
      `TopItem` display strings, `TimeSavedDetail.avg_per_loan`): mark
      deprecated with `TODO(#NNNN)` → batched removal issue. Do **not** add
      ARB keys for them. Note the orphaned `ComputePortfolioMetricDetail` /
      `computePortfolioLibraryItems` pair in that issue too.
- [ ] Tests: migrate the label assertions (~17 in
      `metric_detail_calculator_test.go`, ~8 in `metric_detail_factors_test.go`,
      ~3 in `metric_detail_rankings_test.go`, plus `calculator_counts_test.go`
      et al.) to the enum/numeric fields; update or retire
      `drill_down_builder_test.dart` alongside the production-unreachable
      drill-down branch it exercises.
- [ ] Narrow the `goconst` exclusion for `impact_metrics/metric_detail_` once
      the live strings are gone (the dead-field strings leave with the
      removal issue; keep the exclusion scoped to whichever files still hold
      them, with the comment updated to point at the removal issue).
- [ ] Keep `docs/impact_metrics.md` § "Metrics Screen Reference" true: it is
      the self-declared canonical definition of the source-breakdown row
      labels this PR moves to ARB keys. Update its Source Breakdown Rows
      claim (and `freshness:` stamp) in the same PR rather than waiting for
      the nightly doc ratchet.

**Validates:** enum-typed cross-wire contracts replacing the
string-matching contract; the deprecation ledger for the biggest dead
surface.

### Phase 4: Plans-tab calendar + guard + docs closure (PR 4)

**Goal:** The last live GetHomeView copy (calendar entries) moves client-side;
regressions in migrated files fail CI; the stale doc pointer is fixed.

**Tasks:**
- [ ] Confirm exactly which `home_view_upnext.go` / `home_view_format.go`
      strings reach the rendered `calendar` field vs. the dead `up_next`
      field, then apply the Phase-1 pattern: kind enums exist
      (`HomeUpNextKind`); add the missing typed fields (timestamps are
      already on `HomeUpNextEntry`; the pre-formatted `time_display` and
      composed `subtitle` get deprecated), render via ARB + `intl` on
      `home_calendar_screen.dart` / `widgets/home/calendar/`. Count-bearing
      strings ("Hosting · %d going") use **ICU plurals** in the ARB, and all
      new user-facing keys say **"Event", never "Experience"**
      (`client/conventions.md` § UI Terminology).
- [ ] `home_view_gear.go` / `home_view_activity.go`: no translation (dead);
      deprecation markers + entry in the batched removal issue, per the
      Open-Questions decision.
- [ ] Guard: add a `NO_COPY_FILES` mode to
      `scripts/check_no_inline_user_strings.js` (flag capitalized or
      space-containing double-quoted literals; skip comments; a
      `// user-string-allow: <reason>` pragma for the rare legitimate
      literal) and enroll every file migrated by PRs 1–4.
- [ ] Docs: rewrite `docs/server/l10n.md` § "Adding a new in-app
      server-emitted string" — drop the closed-#1904 forward reference,
      describe the shipped mechanisms (chat `{template_key, params}`, typed
      enums + timestamps for RPC view copy, the `RpcErrorHandler` path), and
      name the guard. Update the `context:`/`freshness:` frontmatter.
- [ ] Verify the `goconst` exclusion block is reduced to exactly the files
      still carrying dead strings awaiting the removal issue, or deleted.
- [ ] Confirm the batched removal issue enumerates every deprecated field
      with the release math (deprecations shipped in vN are removable two
      releases after the client-reader removal ships; current head is
      v0.28.1).

**Validates:** the render boundary holds mechanically (CI guard), the doc
that defines the boundary tells the truth, and the issue can close with a
single tracked successor (the field-removal issue).

## Related Issues

- #2820 — parent thread: found these violations while fixing `goconst`;
  deleted the dead `DailyItem` sibling set; added the `goconst` exclusion
  that this issue retires.
- #2831 — follow-up PR deleted `community_ranking.go`, `actions.go`'s
  `"Lent"`, and the portfolio metric-detail wrappers (commit `24a2bc942`),
  shrinking this issue's surface; left `ComputePortfolioMetricDetail`
  orphaned.
- #1904 — closed l10n epic; its unshipped "Phase 4" is exactly this issue.
  `docs/server/l10n.md` still points at it (fixed in Phase 4 here).
- #2159 — tracks removing the legacy `SystemChatMessage.description` once
  historical rows age out; Phase 2 keeps writing it as the old-client
  fallback, consistent with that plan.
- #2634 — the converged-nav dock rewrite that silently killed the readers of
  `home_view_gear.go` / `up_next` / `recent_activity`, converting those
  surfaces from "migrate" to "deprecate".
- #2020 — the batched-field-removal precedent the new removal issue should
  copy.
- #1972 — the ARB parity gate ("English placeholder OK") that the
  maintainer's real-Spanish bar for this issue supersedes.
