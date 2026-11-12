# Issue #2936: Remove AI-generated prose from the app; render system copy from deterministic templates

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2936
**Filed:** 2026-08-16 | **Last activity:** 2026-08-16
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`, `docs/server/llm.md`, `docs/server/conventions.md`,
`docs/server/architecture.md` (skim), `docs/server/l10n.md`,
`docs/client/conventions.md`, `docs/client/create.md`, `docs/client/the_feed.md`,
`docs/client/inbox.md` (nudge sections), `docs/proto_conventions.md`,
`docs/stories.md`, `docs/nudges.md`, `docs/workshop.md` (hero-card sections),
`docs/design/unified-create.md` (RPC contract)

## Problem Statement

The app authors prose no user asked for. Nudge cards, story cards, Workshop hero
copy, and impact narratives are LLM-written sentences rendered as the product's
own voice — generically upbeat, subtly off, different every time for the same
underlying fact. The issue author's framing: it "feels like slop and degrades the
authenticity of the app experience."

Every fact these surfaces describe is already structured and known server-side:
who did what, when, with whom, and what it saved. The fix is to render those
facts through deterministic templates instead of narrating them.

The work is unusually tractable because **the deterministic replacement already
exists and is wired as the fallback**. `template_key` + `template_params` is
shipped and proven in two places (`server/story/templates.go` ↔
`story_text_resolver.dart`; `server/chat/system_message_templates.go` ↔
`system_message_text_resolver.dart`). `proto/ripls/models/story.proto:74` names
the exact gap: *"Empty `template_key` means … covers AI-generated story prose."*

A reachability trace found the scope is smaller than the surface count suggests:
**four of the ten AI-prose surfaces never reach a user at all.** They are
generated, paid for, persisted, and dropped.

## Context from Comments

**The issue has zero GitHub comments.** All requirements were settled in the
filing conversation with the issue author, and are recorded in the issue body
under *Decisions (settled)*. Reproduced here because they are the spec:

- **Creation assist (Tier 2) stays.** `GenGear` / `GenRequest` / `GenExperience`
  are untouched — the user asked for the draft and edits it before save.
- **Experience completion summary: strike the LLM, keep the field.** Users must
  still be able to type their own recap.
- **Generic feed nudges are deleted**, not re-templated. Replaced on Home by
  three always-present buttons — **Plan an event**, **Ask for help**, **Offer
  something** — and on the calendar open day by **Plan an event** alone.
- **Workshop hero cards keep their surface** with hand-written copy; they ride
  real signals (repeat detection, calendar gaps, idle offers).
- **Unified create refinement:** "Plan an event" and "Ask for help" open on
  **text** entry with type-specific hint text, so the user can see what to do.

Two decisions remain open (see Open Questions): suggestion chips and the
value-estimate `reasoning` line.

⚠️ **This issue is epic-sized.** It spans ~10 surfaces across Go, Dart, and
protos. The phasing below is designed so each phase is independently shippable;
Phase 0 alone is a meaningful win. If the reviewer prefers, Phases 4–6 could be
split into sub-issues — but the issue author has previously asked for one PR per
coherent unit (#2686 was reverted for being split prematurely), so the default
here is **one PR covering Phases 0–5, with Phase 6 deferred by hard constraint.**

## Potential Solutions

### Option A: Delete AI prose paths wholesale, protos included, in one PR
- **Approach:** Remove every Tier-1 generation path, its `ai.Provider` methods,
  its prompts, and its proto fields/RPCs in a single change.
- **Pros:** One review, one mental model, no half-migrated state, no follow-up
  bookkeeping.
- **Cons:** **Violates `docs/proto_conventions.md`.** API proto fields and RPCs
  require *one shipped release plus ~24 hours* after the client stops reading
  them before deletion. Released builds still call the server; a removed RPC
  returns `UNIMPLEMENTED` and breaks the screen rather than rendering blank.
- **Effort:** Large — and not compliant.

### Option B: Behavior-complete now, proto cleanup after a release soak
- **Approach:** Delete all *generation* (AI calls, prompts, provider methods) and
  all *client readers* now. Models protos (no version skew) get deleted +
  `reserved` immediately. API protos get `[deprecated = true]` with a
  `TODO(#NNNN)` pointing at a pre-filed cleanup issue, and stop being populated.
  A follow-up PR retires them one release later.
- **Pros:** Compliant with the documented soak. Ships the entire user-visible
  win — no AI prose anywhere — in the first PR. The residue is inert deprecated
  fields, which is exactly the pattern #2827 → #2835 already ran.
- **Cons:** Requires a follow-up issue and a second PR; the tree carries dead
  proto fields for ~a release.
- **Effort:** Large, split across two PRs.

### Option C: Per-surface incremental PRs
- **Approach:** One PR per surface (stories, then nudges, then hero cards…).
- **Pros:** Small reviews; easy bisect if a surface regresses.
- **Cons:** The `ai.Provider` interface can only shrink once *all* consumers are
  gone, so the cleanup tail sits unmerged for weeks. Repeated churn in the same
  files (provider implementations, wiring, prompt-size budgets). The issue author
  has explicitly disliked this shape before (#2686).
- **Effort:** Large in aggregate; slowest to a clean end state.

## Recommendation

**Option B.** It is the only approach that both ships the complete user-visible
change immediately and respects the API-proto soak the repo documents. The
soak's rationale is entirely about installed builds — for the four dead surfaces
nothing renders the fields at all, but the RPCs are still *callable* by a
stranded build, and `docs/proto_conventions.md` is explicit that a green
`lint:proto:breaking` "says nothing about whether an RPC is safe to drop."

Concretely, in the first PR:
- **Models protos** (`StoredNudge`, `UserImpactNarratives`) — delete fields,
  `reserved` their numbers. Storage protos have no version skew.
- **API protos** — `[deprecated = true]` + `TODO(#NNNN)`, stop populating, keep
  serving. Retired by the follow-up.

## Open Questions

- **Suggestion chips (`GenerateExperienceSuggestions` / `GenerateRequestSuggestions`)** —
  Recommendation: **replace with hand-written per-category catalogs.** They are
  1–4 word labels, not prose, but `seed_needs` persist as real `Need` rows
  (`request/lifecycle.go:202`, `contributions.go:308`), so a bad roll becomes
  durable data. The category set is small and closed. Deferring is acceptable —
  they are the weakest slop case — but leaving them means the AI provider stays
  wired into `experience/save.go` and `request/gen.go`.
- **Value-estimate `reasoning` (≤10 words, e.g. "DeWalt 20V drills retail $80-120")** —
  Recommendation: **drop the field from the metric sheets** and show only the
  deterministic provenance the estimator already computes. A model's recollection
  of retail prices presented as a citation is worse than no citation. It is
  rendered at `content_metric_sheet.dart:389`, `gear_metadata_formatter.dart:296`,
  `savings_formatter.dart:117`.
- **Does the completion summary keep a placeholder when empty?** —
  Recommendation: **no.** Drop the `'Had a great time at ${experience.name}!'`
  fallback (`event_modal_view_model.dart:583`, `:592`). An empty recap should read
  as empty, not as something the host wrote. The recap already renders
  conditionally (`experience_read_shell.dart:929` returns `Widget?`).
- **Does the swipe `FeedScreen` still need nudge rendering removed?** —
  Recommendation: **yes, remove it.** `docs/client/the_feed.md` notes no dock
  destination points at `FeedScreen`, but the code path exists
  (`feed_screen.dart:400`) and would otherwise reference deleted payload fields.
- **Do the three Home buttons need analytics parity with `ConsumeNudge`?** —
  Recommendation: **no new events in this PR.** `ConsumeNudge` existed to expire
  a pooled row; always-present buttons have no pool. Flag to the author if
  nudge-consumption analytics were being used for activation measurement.

## Implementation Plan

### Phase 0: Delete the four dead paths
**Goal:** Remove generation that no user ever sees. Pure deletion, no replacement
copy to design, no UI risk.

> **Precondition — file the Phase 6 cleanup issue first.** Phase 0 introduces the
> first `TODO(#NNNN)` deprecation markers, and both CLAUDE.md and
> `docs/proto_conventions.md` require the tracking issue to pre-exist the TODO.
> `npm run lint:todo-refs` fails on a bare `TODO:`. Do this before writing any
> deprecation comment.

**Tasks:**
- [ ] Chat summaries — delete `server/services/chat/summary.go`
      (`GenConversationSummary`), the client wrappers
      (`chat_service.dart:285`, `chat_repository.dart:283`), the
      `loadGiveawaySummary` mixin (`gear_sharing_giveaway_actions.dart`), and the
      `giveawayConversationSummary` / `isLoadingGiveawaySummary` state fields.
      Keep `giveawayParticipants` / `giveawayParticipantCount` **only if** a
      widget reads them — verify; the summary itself has zero widget reads and
      `loadGiveawaySummary` has zero production callers.
- [ ] Request resolution summary — delete `GenerateResolutionSummary`
      (`server/services/request/gen.go:452-565`) and its client wrappers
      (`request_repository.dart:354`, `request_service.dart:310`).
- [ ] Profile impact-row narratives — delete
      `server/services/profile/narratives.go`, `server/ai/impact_row.go`,
      `server/ai/prompts_impact_row.go`, the background call at
      `profile/profile.go:160`, and the viewmodel fields at
      `viewer_profile_view_model.dart:134-139`. Delete
      `proto/ripls/models/user_impact_narratives.proto` fields + `reserved`.
      Deprecate API fields 10–12 on `GetUserProfileForViewerResponse`.
      The deterministic tier ladder (`impact_row_narrative_tiers.dart`) already
      ships and is untouched.
- [ ] `DraftImpactEstimateResponse.llm_reasoning` — deprecate the API field, stop
      populating at `impact_metrics/drafting.go:195`, `:251`.
- [ ] Delete dead prompts `server/services/experience/prompts.go:5-93`
      (`fromTextPrompt` / `fromTextPromptPast`) and the test that pins them.
- [ ] Update the `*_test.go` and `*_test.dart` files that exercise these. Note
      `gear_sharing_view_model_test.dart:521-719` is the *only* caller of
      `loadGiveawaySummary` — delete the group rather than rewriting it, and check
      whether `gear_sharing_view_model_test.dart:609` still needs its
      `GenConversationSummaryResponse` fixture.
- [ ] Re-run any `roundtrip_test.go` in the touched services — proto field
      changes are exactly what those pin (`api_design` rubric §1).
**Validates:** That the reachability trace was right — nothing visually changes.
Any UI diff here means a reader was missed.

### Phase 1: Stories render from templates only
**Goal:** Every story carries a `template_key` and localizes.
**Tasks:**
- [ ] Delete the `if g.aiProvider != nil && !clock.IsSimulated(ctx)` branch in
      `server/story/generator.go:123-152`; `generateFromTemplate(req)` already
      returns complete key-carrying output.
- [ ] Drop `aiProvider` from `Generator` / `NewGenerator`; update `wiring.go:216`
      and `generator_test.go` (8 call sites).
- [ ] Remove `ai.StoryContentRequest` / `GenerateStoryContent` from the provider
      interface and all five implementations; delete `server/ai/prompts_story.go`
      and its `TestPromptSizes` budget entry.
- [ ] Review the `ai.CallWithTimeout(60s)` wrapper in `story/subscriber/*.go` —
      template generation is synchronous and sub-millisecond; the budget is now
      vestigial but harmless. Prefer removing it for clarity.
- [ ] Update `docs/stories.md` (its "Hybrid Generation" principle, the flow
      diagram, `NewGenerator(storyStorage, aiProvider)` signature, the
      "AI-Generated" content example, and the AI Cost / AI Latency trade-offs).
- [ ] **New test — the regression guard for this whole issue.** Table-driven test
      asserting every `ai.StoryType*` constant produces a non-empty `TemplateKey`
      and non-empty params. Today `getTemplateForStoryType` has a `default:` arm
      returning "Community Story" with **no** template key, so a new story type
      would silently ship untranslated literal prose — the exact failure mode this
      issue removes. Make the default arm fail the test rather than fall through.
**Validates:** Highest-volume surface converts with the smallest diff, and every
story becomes localizable — the l10n win the issue predicts.

### Phase 2: Experience completion summary becomes user-authored
**Goal:** Hosts still write recaps; the model does not.
**Tasks:**
- [ ] Delete `server/services/experience/summary.go`
      (`GenerateCompletionSummary` RPC) and
      `GenerateExperienceCompletionSummary` from the provider interface + all
      implementations; delete the prompt at `prompts_summaries.go:144-203`.
- [ ] Remove the auto-generate-on-open call and "Regenerate" button
      (`event_modal_view_model.dart:563-599`); drop the
      `'Had a great time at …'` fallbacks.
- [ ] Keep `models.Experience.completion_summary`, the editor, and the
      post-to-chat step on confirm (`experience/lifecycle.go:363` →
      `chat.ExperienceCompletionSummaryMessage`).
- [ ] Deprecate the `GenerateCompletionSummary` RPC + its request/response
      messages in `experience_service.proto`.
- [ ] Delete `prompts_summaries.go` entirely once Phase 0 removed the other two
      summary prompts.
**Validates:** A user-authored field behaves correctly when empty — the recap
must not render an empty card.

### Phase 3: Unified create accepts a target type
**Goal:** Make the Phase 4 buttons land somewhere distinct. **This is
prerequisite plumbing, not polish.**
**Tasks:**
- [ ] Add an optional target-type parameter to `UnifiedCreateModal.show`
      (`unified_create_modal.dart:57`) alongside `initialPrompt` /
      `initialStartUnixSec`.
- [ ] Seed `state.type` and reuse the existing
      `StreamGenUnifiedCreateRequest.force_type` (field 9) so the server skips
      the classifier — the same mechanism `flipType` already uses. No new RPC
      surface.
- [ ] Set the input mode from the type: experience/request → `CreateInputMode.text`;
      gear → `image` (today's `@Default(CreateInputMode.image)` at
      `unified_create_state.dart:87` is already right for gear).
- [ ] Add ARB keys `unifiedCreatePromptHint{Event,Request,Item}` following the
      existing `unifiedCreateDescriptionHint{Item,Request,Event,Default}`
      precedent. Copy, verbatim from the author:
      - Event — `Describe your event naturally, like "hike at Zilker park Sunday 2pm"`
      - Request — `Describe what help you're looking for, like "I need two lawn chairs and a cooler for an upcoming campout"`
- [ ] In `_TextPanelState` (`unified_create_input_drawer.dart:143-179`): when a
      type is seeded, render one static hint and **do not start `_cycleTimer`**;
      unseeded keeps the 12-example shuffled tour.
- [ ] Point `openBlankCreateGear` / `openBlankCreateRequest` at the seeded call
      so their names stop lying (today all three are the same call under the
      flag).
- [ ] **New viewmodel test** (`lint:dart:viewmodel-tests` gates this): seeding a
      type sets `state.type`, sets the expected `inputMode`, and sends
      `force_type` on the stream. Cover the unseeded case still defaulting to
      `image` with the cycling tour.
- [ ] **a11y:** `hintText` merges into the field's accessible *name*, so any
      semantics assertion on the prompt field must use a contains-style matcher,
      not `equals`. Stopping `_cycleTimer` for seeded types also removes a
      3-second self-rewriting label — dynamic content that a screen reader
      re-announces (accessibility rubric §2, "Live Regions").
**Validates:** Three entry points produce three visibly different composers —
the core UX claim of Phase 4.

### Phase 4: Delete feed nudges; add the Home and calendar affordances
**Goal:** The user-visible heart of the issue.
**Tasks:**
- [ ] `home_empty_states.dart` — delete `_buildNudgeHero` and the
      `nudge == null` branch (`:105-121`); add a **Plan an event** `_ZeroPrompt`
      with an `onPlan` callback and `homeZeroPlan{Title,Subtext,Cta}` ARB keys;
      render Plan / Ask / Offer unconditionally. Wire `onPlan` at
      `home_tab_screen.dart:134` and repoint `onOffer` from the generic
      `openBlankCreate` to the gear-seeded call.
- [ ] `calendar_day_detail.dart` — delete the nudge branch (`:261-299`) and the
      `nudge` field; an open day falls through to a single **Plan an event**
      affordance. **Keep the `OpenDaySuggestion` panel** — it is not AI
      (`home_suggestions.go:296`, `:344` are pure Go over weather + activity).
- [ ] `feed_screen.dart:400` — remove nudge rendering.
- [ ] Server — delete `GenerateNudgeContent`, `generateAndPersistNudges`
      (`feed/nudges.go`), the pool top-up and `InboxNudge` seams
      (`feed_nudges.go:49`, `:181`), the async stock-imagery fetch, and
      `pruneExpiredNudges`. Delete the nudge prompt (`prompts.go:792-876`).
- [ ] Keep the **terminator** pool (`terminator_pool.go` — already static
      hand-written copy) and keep Workshop-surface rows flowing through
      `isHostPrompt` (`nudge_storage.go:41-49`).
- [ ] Delete the `NudgeContentView` variants only if no surface still renders a
      Workshop/terminator nudge — verify before deleting; Phase 5 keeps hero
      cards on this widget.
- [ ] Deprecate the `ConsumeNudge` RPC path for feed-surface rows and the
      `debug_nudge_count` field on `GetFeedRequest`; drop the `NUDGE_TEST_COUNT`
      dart-define (`environment.dart:131-136`).
- [ ] Update `docs/nudges.md` (also correct its stale claim that the terminator
      costs one LLM call per slot per day), `docs/client/inbox.md`,
      `docs/client/the_feed.md`.
- [ ] Check the three e2e specs touching nudges — `runclub.spec.ts`,
      `people-directory.spec.ts`, and `community-promote-nudge.spec.ts` (the last
      is the *name-this-group promote* flow, a different feature; confirm before
      touching).
- [ ] **a11y for the three new buttons** (accessibility rubric §1, §3, §4). They
      are permanent primary affordances, so they carry the rubric's full weight:
      - ≥48×48 dp touch targets with adequate spacing (§3).
      - A `semanticsLabel` on each `Tappable` — the existing `_ZeroPrompt` pattern
        already splits `ctaLabel` (which carries a `›` glyph) from
        `ctaSemanticsLabel`; follow it so the chevron is not announced.
      - Must survive 200% text scaling — three stacked prompts is materially more
        vertical content than one nudge hero plus two. This is the open #2916
        rubric gap; at minimum do not regress it, and prefer a layout that wraps
        rather than clips.
      - Colors from design tokens only — `lint:design-tokens`, `lint:dart:colors`,
        and `lint:glass-foreground` all gate this, and the calendar detail sits on
        a glass/backdrop surface where foreground contrast is separately checked.
**Validates:** The replacement is always-present and needs no server round trip —
the zero state can no longer be empty.

### Phase 5: Workshop hero cards to a hand-written catalog
**Goal:** Keep the surface, drop the narration.
**Tasks:**
- [ ] Delete `GenerateHeroCardCopy` from the provider interface + Anthropic
      implementation (`provider_anthropic.go:194`) and the Gemini/OpenAI
      not-implemented stubs; delete `server/ai/prompts_hero_card.go`,
      `server/ai/hero_card.go`, `services/workshop/copy_provider.go`, and the
      `applyAI` path in `momentum/copy_generator.go:64-106`.
- [ ] The detector's template copy is already the fallback *and* already the only
      thing the scheduled job persists (`jobs/workshop_generation.go:184`, `:196`
      — "no AI pass ever rewrites what it persists"). Promote it to the only
      path; keep `ValidateLeverCopy`.
- [ ] Update `docs/workshop.md` (§"There is no AI-generated prose brief" becomes
      literally true).
**Validates:** The surface survives on copy that was already being written and
persisted — the smallest-risk half of the nudge story.

### Phase 6 (follow-up PR, after one shipped release + ~24h): retire the API protos
**Goal:** Remove the deprecated husks.
**Tasks:**
- [ ] File the cleanup issue **before** any `TODO(#NNNN)` from Phases 0–5 lands
      (CLAUDE.md requires the issue to pre-exist the TODO).
- [ ] Confirm the release carrying Phases 0–5 shipped and soaked ~24h.
- [ ] Delete the deprecated API fields and RPCs; `reserved` field numbers;
      delete request/response messages in the same change.
- [ ] Shrink `ai.Provider` to its final Tier-2/Tier-3 surface; delete orphaned
      eval suites under `server/ai/eval/` and their baselines.
- [ ] `npm run lint:proto:breaking` clean.
**Validates:** The interface shrink is the real regression guard — a deleted
method cannot be called by a future surface.

### Gates to run (all phases)
- `npm run generate` first — generated code is not checked in.
- `npm run build` (= `lint:go` + `go build ./...`), **plus
  `go build -tags=integration ./...`** — `docs/server/conventions.md` calls this
  out specifically for `server/ai/` signature changes, which every phase makes.
- `cd app && flutter analyze` — `--fatal-infos` is on; info-level findings block.
- `npm run lint:dart:l10n-parity`, `lint:dart:arb-orphans`, `lint:dart:dead`,
  `lint:dart:viewmodel-tests`, `lint:dart:a11y`, `lint:dart:colors`,
  `lint:design-tokens`, `lint:glass-foreground`, `lint:ephemeral-comments`,
  `lint:todo-refs`, `lint:dart:size`, `lint:go:size`.
- `npm run lint:proto` and `lint:proto:breaking` after every proto edit. Note the
  gate **misses RPC deletion entirely** (`RPC_NO_DELETE` is `FILE`-scoped) — a
  green run is not evidence an RPC is safe to drop, which is why Phase 6 is
  time-gated rather than gate-gated.
- `npm run lint:context-map` after doc edits; regenerate with
  `npm run generate:context-map`.
- `go test ./...` and `go test -tags=integration ./...`;
  `cd app && flutter test --reporter expanded`.
- Playwright e2e for the feed/home/calendar surfaces (Phase 4).

### Docs requiring updates
`docs/nudges.md` · `docs/stories.md` · `docs/workshop.md` · `docs/server/llm.md` ·
`docs/client/the_feed.md` · `docs/client/inbox.md` · `docs/client/create.md` ·
`docs/users/profile.md` · `docs/impact_metrics.md` · `docs/provenance.md` ·
`docs/design/unified-create.md`

**`docs/server/l10n.md` needs special attention.** It carries a section titled
*"AI-generated content sits outside both catalogs"* describing AI prose as an
"explicit exception to the client-renders-in-app boundary." This issue deletes
the exception — that section must go, and the boundary becomes unconditional.

## Audit Notes

Applied against the review rubrics for the engaged dimensions
(`api_design`, `accessibility`, `testing`, `tech_debt`, `error_handling`,
`observability`, `code_quality`). Changes made to the plan:

1. **Phase 0 gained a precondition** — the Phase 6 cleanup issue must be filed
   *before* Phase 0 lands, because Phase 0 writes the first `TODO(#NNNN)` markers
   and `lint:todo-refs` rejects a bare `TODO:`.
2. **Phase 1 gained the issue's real regression guard.** `templates.go`'s
   `default:` arm deliberately emits **no** `template_key` ("client falls back to
   the literal description"). With the AI branch gone that arm becomes the only
   remaining path that ships untranslated literal English — and
   `STORY_TYPE_EXPERIENCE_RSVP = 6` is already reserved in the proto and mapped in
   the converters with no subscriber producing it, so the arm is reachable the
   moment someone adds one. The new table-driven test closes that door.
3. **Phase 3/4 gained accessibility requirements** — touch targets, semantics
   labels, 200% text scaling (#2916), and the token/contrast gates. Three
   always-present prompts is more vertical content than the nudge hero they
   replace, which is a scaling risk the original plan did not name.
4. **Testing tasks made concrete** — the plan previously said "update tests"; it
   now names the specific test groups to delete, the round-trip tests to re-run,
   and the two new tests to write.
5. **Gate list expanded** to the full lint chain that this change actually trips,
   plus the explicit note that `lint:proto:breaking` cannot see RPC deletion.

Checks run during the audit that produced **no** findings, recorded so they are
not re-litigated:

- **Observability.** No log-based metric, SLO, or alert in
  `terraform/modules/monitoring/` filters on any removed operation name
  (`GenerateStoryContent`, `GenerateNudgeContent`, `GenerateHeroCardCopy`,
  `GenerateConversationSummary`). Deleting these log sites breaks no dashboard.
  `ai.Provider.CheckHealth` is retained — creation assist keeps the provider
  wired — so AI dependency-health monitoring is unaffected.
- **Error handling.** The deletions *remove* warn-and-continue patterns rather
  than adding them: every AI call site is an "AI failed → fall back" branch, which
  is the shape `server/conventions.md` calls out as masking bugs. Template
  generation is pure and cannot fail, so no new error path is introduced. Watch
  for error branches that become vacuous once their only failure source is gone.
- **Security.** No auth, secrets, PII, or outbound-HTTP surface changes. Removing
  provider calls slightly *reduces* egress.
- **Performance.** Strictly positive: removes per-user LLM calls from feed/inbox
  read paths, a nightly batch job, and a background goroutine on profile reads.
- **`lint:go:no-inline-user-strings`.** Scope is a hard-coded file whitelist; none
  of the files this change touches are on it, so the new hand-written catalogs are
  not affected.

**Docs consulted:** `docs/llms.txt`, `docs/proto_conventions.md`,
`docs/server/conventions.md`, `docs/server/architecture.md`,
`docs/client/conventions.md`, `docs/server/l10n.md`, `docs/client/create.md`,
`docs/design/unified-create.md`, `docs/stories.md`, `docs/nudges.md`,
`docs/workshop.md`, `docs/client/the_feed.md`, `docs/client/inbox.md`,
`docs/server/llm.md`, and the review rubrics
`docs/reviews/{api_design,accessibility,testing,tech_debt}_review_prompt.md`.

## Related Issues
- **#2159** — retires the legacy `Story.title`/`description` fields. Removing the
  AI path makes them pure legacy immediately, so #2159 can close sooner.
- **#992** — removed the request resolution-summary *modal step*; left the RPC and
  client wrappers. Phase 0 finishes it.
- **#2827 / #2835 / #2836–#2839** — the in-app copy migration that established the
  `template_key` pattern and the deprecate-then-retire cadence Phase 6 follows.
- **#2892** — retired the `propose_share` CTA action; same nudge dispatch surface.
- **#2796** — the nudge-nesting bug in the Home zero state; Phase 4 removes the
  widget whose framing caused it.
- **#2824 / #2823** — declined analyzer rules; relevant if Phase 3/4 Dart edits
  trip `discarded_futures`-adjacent patterns.
