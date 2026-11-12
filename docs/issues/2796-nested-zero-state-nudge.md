# Issue #2796: Home screen shows "plan an experience" nudge inside "need a hand with something" nudge

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2796
**Filed:** 2026-07-29 | **Last activity:** 2026-07-29
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`; anchors — `docs/client/architecture.md`, `docs/client/conventions.md`, `docs/proto_conventions.md`, `docs/users/priorities.md`, `docs/users/personas.md`; leaves — `docs/client/inbox.md`, `docs/nudges.md`, `docs/client/design.md`, `docs/client/i18n.md`; review dimensions — `docs/reviews/accessibility_review_prompt.md`, `docs/reviews/code_quality_review_prompt.md`, `docs/reviews/testing_review_prompt.md`

## Problem Statement

The Home tab's zero state renders **one full-bleed nudge card inside another
prompt card**. `HomeZeroStateCard` frames a request-specific hero — "Need a hand
with something?" / "A ladder, a ride, an hour of help. Start small." / **Ask the
crew ›** — and then, when the server sent a contextual nudge, drops a 320 px-tall
`NudgeContentView` *between the subtext and the hero's own CTA*
([`home_empty_states.dart:121–131`](../../app/lib/presentation/widgets/home/home_empty_states.dart)).
The reporter's screenshot shows the result: a photographic "Get the Space Team
together for an adventure" card with its own **Plan an Experience** button
nested inside the "Need a hand with something?" card, with **Ask the crew ›**
sitting underneath it.

The nesting is not a rendering accident — it is unconditional whenever
`nudge != null`. The design intent is visible in the widget's own comment: the
nudge was meant to *replace the rotating canned example*, i.e. to slot into the
space held by a single italic sentence. But `NudgeContentView` is not a sentence.
It is the same full-screen composition the feed uses: background photograph,
radial overlay, serif headline, italic body, and its **own filled sage CTA**
([`nudge_card_variants.dart`](../../app/lib/presentation/widgets/nudge/nudge_card_variants.dart)).
Putting it inside a card that already has a headline, a body, and a filled sage
CTA produces two heroes and two primary buttons in one box.

Three concrete defects fall out of the nesting, all visible in the screenshot:

1. **Semantic collision.** The hero is hard-coded to the *request* frame, but a
   nudge's `cta_action` is one of four — `plan_experience`, `list_item`,
   `ask_for_help`, `check_listings` ([nudges.md](../nudges.md) → *CTA Actions*).
   Only `ask_for_help` is coherent under "Need a hand with something?"; the other
   three (75% of the pool, and the prompt explicitly tells the model to
   "distribute cta_action values … so no single action dominates") read as a
   non-sequitur. The screenshot is the `plan_experience` case.
2. **Chrome designed for full-screen leaks in.** `buildVariant1` positions the
   attribution ⓘ button at `MediaQuery.padding.top + 12`
   ([`nudge_card_variants.dart:237–241`](../../app/lib/presentation/widgets/nudge/nudge_card_variants.dart)) —
   correct over a full-screen card, but inside the 320 px box it lands on top of
   the headline ("adventure" in the screenshot).
3. **Text truncation.** The description is `maxLines: 4` + ellipsis inside a
   `Flexible`; squeezed into the nested box it clips mid-sentence ("…so why
   not"). This is the same failure mode reported the same morning against the
   calendar surface in **#2801**.

The reporter's own verdict — *"These don't make sense nested. Maybe one or the
other"* — is the right frame: the zero state should have **exactly one hero**.

**No contradictions** between the issue body and later material; there are no
comments. The issue body is one sentence, so the design decision (which of the
two survives) is not specified by the reporter and is resolved below.

### UI investigation — not run, and why

`ripls-ui-investigate` was **skipped deliberately**. Its job is to bisect a
UI-observable report to a layer (client render / client state / server / data /
UX gap); here the layer is not in question. The nesting is a *static,
unconditional composition* in the widget tree — `NudgeContentView` is a literal
child of the ask hero's `Column` at
[`home_empty_states.dart:121–131`](../../app/lib/presentation/widgets/home/home_empty_states.dart) —
and the existing widget test at
[`home_zero_state_card_test.dart:72–90`](../../app/test/presentation/widgets/home/home_zero_state_card_test.dart)
already asserts the exact behavior the reporter is complaining about ("the ask /
offer CTAs stay alongside the nudge"). There is no state, timing, or data
question to reproduce. The verdict is **client render, by design**.

The hermetic env is still the right place to *validate the fix* — the e2e AI
provider ships a deterministic four-nudge pool including a `plan_experience`
entry ([`server/ai/provider_e2e.go:144–171`](../../server/ai/provider_e2e.go)),
and `InboxNudge` does not require imagery, so the zero-state-with-nudge state is
reachable end to end. That is folded into Phase 1's validation.

## Context from Comments

**The issue has no comments.** It is an in-app feedback submission (label
`user-feedback`) from user2@example.org on Android 17 / Pixel 9 Pro, app 0.28.0
build 9326, PROD. All of the context is the one-line body plus the screenshot.

Because there is no discussion, two things are worth stating explicitly rather
than assumed:

- The reporter says "maybe one or the other" — an *invitation* to choose, not a
  decision. Picking which of the two survives is this plan's job (see
  **Recommendation**), and it is the one thing worth confirming before code.
- The screenshot also exposes an unrelated defect the reporter did not call out:
  the nudge's CTA reads **"Plan an Experience"**, violating the
  Event-not-Experience rule in
  [`client/conventions.md`](../client/conventions.md) → *UI Terminology*. Its
  source is the AI prompt itself, which offers `"Plan an Experience"` as an
  exemplar CTA label at
  [`server/ai/prompts.go:819`](../../server/ai/prompts.go). Scoping decision in
  **Open Questions**.

### Related context: the same reporter filed #2801 seven minutes later

**#2801** ("Planning nudge text is cut off on calendar screen") is the same
underlying pressure — a full-bleed `NudgeContentView` forced into a fixed-height
host until its text ellipsizes — on the calendar surface
([`calendar_day_detail.dart:255–300`](../../app/lib/presentation/widgets/home/calendar/calendar_day_detail.dart),
`_nudgeCardHeight = 280`, `_nudgeCardMinHeight = 200`). #2796 and #2801 are
distinct bugs (composition vs. fit) but share a cause: **the nudge card has one
presentation mode, sized for full-screen, and two hosts that aren't
full-screen.** The plan keeps them as separate fixes but names the shared seam.

## Potential Solutions

### Option A: One hero, chosen by nudge presence — the nudge becomes the zero state's lead

- **Approach:** Restructure `HomeZeroStateCard` into *one hero + a quiet prompt
  pair*. When `nudge != null`, the nudge card **is** the hero — full width, its
  own frame, its own CTA, nothing wrapping it — and the request and offer prompts
  render below it as two symmetric quiet prompts ("Need a hand with something?" →
  **Ask the crew ›**, filled sage; "Have something to share?" → **Offer
  something**, ghost). When `nudge == null`, the layout is exactly today's:
  the request hero with its rotating canned examples, offer prompt below. The
  `320` `SizedBox` and its `ClipRRect` go away in favor of a host that lets the
  card size itself.
- **Pros:** Deletes the nesting outright — the reporter's "one or the other",
  with the personalized card winning when there is one. Keeps the AI co-catalyst
  on the app's most-seen empty surface, which is the product spine
  ([priorities.md](../users/priorities.md): *"the system itself watches for
  momentum and proposes the next instance"*). Removes the semantic collision
  structurally rather than by filtering — once the nudge owns its own frame, all
  four `cta_action`s are coherent. Mirrors the shape the calendar already uses
  successfully (nudge as the whole day-detail body). Pure client layout: no
  proto, no RPC, no server. Degrades to today's exact pixels when no nudge exists
  — which is every install with no AI provider configured.
- **Cons:** The request CTA loses hero status precisely when a nudge is present,
  and reviving requesting was the deliberate point of the current design
  (#2724/#2784 both invested in this card). Mitigated by keeping the request
  prompt first and filled while the offer stays ghost. Also makes the zero
  state's lead non-deterministic — what the user sees depends on what the pool
  generated.
- **Effort:** Small (one widget's `build`, its four widget tests, two doc
  updates).

### Option B: Drop the nudge from the zero state entirely

- **Approach:** Delete the `nudge` / `onNudgeConsumed` parameters from
  `HomeZeroStateCard` and the wiring at
  [`home_tab_screen.dart:126–137`](../../app/lib/presentation/screens/portfolio/home_tab_screen.dart).
  The request hero always shows its rotating canned examples. The nudge keeps
  living in the feed and on empty calendar days.
- **Pros:** Smallest possible diff and zero new layout risk. Restores the
  card exactly as designed and polished. The canned examples are arguably
  *better* teaching material than a nudge here: they are five concrete requests
  that model the norm we're trying to revive, and they can't drift off-message
  the way generated copy can. `GetHomeViewResponse.nudge` stays populated, so
  nothing is lost server-side and the decision is reversible.
- **Cons:** Walks back part of #2488 (surfacing contextual nudges in the inbox)
  on its highest-traffic surface. The zero state is exactly where a personalized
  "Doug and Ali are ready for a new outing" earns its keep — a brand-new user
  with nothing on their plate is who the co-catalyst exists for. Trades a real
  product capability for a layout convenience.
- **Effort:** Small (smaller than A).

### Option C: Keep both, side by side as sibling sections

- **Approach:** Leave the request hero intact and move the nudge *out* of it, to
  a full-width sibling card below (or above) it, with the offer prompt last.
- **Pros:** Un-nests without choosing a winner; both the revive-requesting design
  and the nudge survive at full strength.
- **Cons:** Three stacked primary CTAs on a screen whose entire premise is "you
  have nothing to do" — the empty state becomes the busiest screen in the app.
  Directly contradicts the reporter ("maybe one or the other") and the widget's
  own stated goal of collapsing empty sections into *one focused invitation*.
  Two competing filled sage buttons still compete, just vertically instead of
  nested.
- **Effort:** Small, but the design debt is the largest of the three.

### Option D: Give the nudge a genuine compact presentation

- **Approach:** Add a second presentation mode to the nudge system — a
  content-sized, chrome-light variant (no background photo, no ⓘ overlay, no
  filled CTA; headline + description as an attributed inline prompt) — and use
  it for *both* constrained hosts: the zero state (inline, replacing the canned
  example as originally intended) and the calendar day detail (#2801).
- **Pros:** Fixes the real seam — one composition, two host classes — and pays
  down #2796 and #2801 together. Honors the zero-state card's original intent
  literally: the nudge replaces the example *sentence*.
- **Cons:** Meaningfully larger: a new variant surface, new tests across three
  variants × two modes, and a design decision about what a chrome-light nudge
  looks like in the Ink & Sage language. Also strips the imagery, which is a
  deliberate product commitment ([priorities.md](../users/priorities.md):
  backgrounds from working artists are how local life competes for attention).
  Solving two reported bugs by inventing a new presentation mode is more than
  either report asks for.
- **Effort:** Medium-to-large.

## Recommendation

**Option A**, with #2801 kept as its own fix.

The decisive argument is that the collision is *structural, not cosmetic*. The
hero's copy is permanently request-framed while the nudge's action is drawn from
a pool the server is explicitly instructed to diversify across four actions. Any
solution that keeps a general-purpose nudge inside a request-specific frame is
wrong roughly three times in four, no matter how the padding is tuned. Only
giving the nudge its own frame removes that — and once it has its own frame, the
"which action is it" question stops mattering.

Between A and B — the two that actually satisfy "one or the other" — A keeps the
capability and B keeps the polish. A wins because the zero state is the single
highest-value place for the co-catalyst: a user with nothing needing them is
exactly the latent host the product is built for, and "Doug and Ali are ready for
a new outing" is the proposal that gets them moving. B's argument (canned
examples teach the requesting norm better than generated copy) is real but is
preserved: A still shows the request prompt with its **Ask the crew ›** pill on
every zero state, and the full hero-with-examples treatment survives untouched
for the no-nudge case, which is the common case wherever no AI provider is
configured.

Option C is rejected on the reporter's own terms and on the widget's stated
purpose. Option D is the correct *eventual* architecture and should be
reconsidered if a third constrained host appears — but shipping a new nudge
presentation mode to fix a nesting bug is scope the report does not justify, and
it would trade away the imagery the product deliberately invests in.

**Tradeoff being accepted:** when a nudge exists, requesting drops from hero to
quiet-but-primary prompt. This is a deliberate demotion, and it is the cost of
having one hero. If measurement later shows request creation falling off the zero
state, the cheap lever is Option B (drop the nudge here) — the wiring stays in
place, so that reversal is a few lines.

## Open Questions

- **When a nudge is the hero, does the request prompt stay visually primary over
  the offer prompt?** — Recommendation: **yes.** Render the pair in
  request-then-offer order, keep the request CTA as the filled sage pill and the
  offer CTA as the bordered ghost pill (both already exist in the widget). This
  preserves the requesting emphasis at prompt scale and costs nothing. Do *not*
  make both filled — that reintroduces competing primaries one level down.

- **Should any `cta_action` be suppressed on this surface?** — Recommendation:
  **no filtering.** All four actions are legitimate things to do from an empty
  Home. Filtering would re-import the coherence problem Option A deletes, and it
  would silently blank the surface whenever the pool happened to hold only
  filtered actions. If a specific action later proves wrong here, that is a
  server-side `InboxNudge` selection concern, not a client render condition.

- **How tall is the nudge in its new host?** — Recommendation: give it a
  content-appropriate fixed height in the 280–320 range (matching the calendar's
  `_nudgeCardHeight = 280`) rather than unbounded — `buildVariant*` returns a
  `Stack(fit: StackFit.expand)` and cannot self-size, so an unconstrained host
  will throw. Do **not** invent a new sizing scheme here; #2801 owns the
  fit/clipping question and its answer should apply to both hosts. Phase 1's
  only obligation is *don't make clipping worse than the calendar's*.

- **Does the attribution ⓘ button need a non-full-screen position?** —
  Recommendation: **yes, and it belongs in #2801's scope**, since the calendar
  host has the identical overlap. If #2801 slips, Phase 2 here is the fallback
  owner. The one-line version is to stop adding `MediaQuery.padding.top` when the
  card is not full-bleed.

- **Fix "Plan an Experience" (the terminology violation) in this PR or file it
  separately?** — Recommendation: **file it separately and cross-link**, then
  decide. It is a *server + CI* change (an AI prompt exemplar plus extending
  `scripts/check_event_terminology_go.sh`, which today scans
  `server/services/feed/nudges.go` but not `server/ai/prompts.go`), and mixing it
  into a client-layout PR muddies both reviews. It is ~10 lines, so folding it in
  is defensible if the preference is one PR per screenshot — see Phase 3, which
  is written to be liftable either way.

- **Should the zero state get `semanticsIdentifier`s?** — ~~Recommendation:
  yes, in Phase 1~~ → **Resolved during implementation: no.**
  [semantics_identifiers.md](../client/testing/semantics_identifiers.md) says
  `Tappable.semanticsIdentifier` is merged away by the engine and **does not
  reach the Playwright DOM** ("Playwright should not depend on it"), and that
  identifiers should only be added when a spec already queries them ("Don't
  pre-tag widgets speculatively"). The recommendation was written on a false
  premise. The regression guard is the Dart widget test instead, which needs no
  identifiers — see Phase 1.

## Resolutions

Decisions taken before implementation, recorded here so the plan reads as the
history of the fix:

| Question | Resolution |
|---|---|
| Which of the two survives? | **Option A** — the nudge is the hero when present; the request drops to a quiet prompt. |
| Request prompt still primary? | **Yes** — filled sage pill for the request, ghost for the offer. |
| Filter any `cta_action`? | **No** — all four are legitimate from an empty Home. |
| Nudge height in the new host? | **320**, the value the nested card already used, now at full card width. #2801 owns tuning it. |
| Terminology fix here? | **Filed separately as #2803** (server + CI scope). |
| Semantics identifiers? | **No** — see the struck-through question above. |

## Implementation Plan

**Scope boundaries (audit).** Phases 1–2 are **pure client presentation**: no
proto or RPC change, so [`proto_conventions.md`](../proto_conventions.md) is
considered and N/A — `GetHomeViewResponse.nudge` keeps its shape and meaning.
`HomeZeroStateCard` stays a presentational widget with no repository access
([client/architecture.md](../client/architecture.md) rule 1; nudge media
resolution stays inside `NudgeContentView` where it already lives), and no
viewmodel changes, so `scripts/check_viewmodel_tests.sh` is N/A. Phase 3 is
server + CI only. Across all phases: **no implementation-phase markers in Dart
or Go comments** — `npm run lint:ephemeral-comments` blocks "Phase 1"-style
references; cite `#2796` instead.

### Phase 1: Un-nest the zero state

**Goal:** The Home zero state renders exactly one hero. `NudgeContentView` is
never a descendant of the request prompt.

**Tasks:**
- [x] Restructure `HomeZeroStateCard.build`
      ([`home_empty_states.dart`](../../app/lib/presentation/widgets/home/home_empty_states.dart))
      into two branches over `widget.nudge`:
      - `nudge != null` → nudge card as the lead (own frame, own CTA), then the
        request prompt (title + subtext + filled **Ask the crew ›**), then the
        offer prompt (title + subtext + ghost **Offer something**).
      - `nudge == null` → today's layout, byte-for-byte: request hero with the
        rotating `AnimatedSwitcher` examples, offer prompt below.
- [x] Keep the example-rotation timer gated on `nudge == null` (it already is,
      `initState:63`) so no timer runs in the nudge branch.
- [x] Reuse the existing request/offer prompt bodies rather than duplicating
      their `Text`/`Tappable` trees — extract them to private widgets or builders
      so the two branches share one definition (code-quality: DRY). *Shipped as
      `_ZeroPrompt` (heading + subtext + CTA) over a shared `_ZeroPromptButton`,
      with a `_PromptEmphasis` enum for the filled/ghost treatment.*
- [x] No new user-facing strings. If any copy must change, it goes through
      `app/lib/l10n/app_en.arb` with an `@key.description`
      ([i18n.md](../client/i18n.md)) — never inline. *None were needed — both
      branches reuse the four existing `homeZero*` keys.*
- [x] ~~Add `semanticsIdentifier`s~~ — **dropped.** `Tappable.semanticsIdentifier`
      is merged away before DOM serialization, so a Playwright locator can't see
      it, and the convention is not to pre-tag speculatively
      ([semantics_identifiers.md](../client/testing/semantics_identifiers.md)).
      The Dart widget test is the regression guard and needs no identifiers.
      The existing `semanticsLabel`s stay l10n-sourced
      (`require_l10n_for_semantic_labels`, [accessibility.md](../client/accessibility.md)).
- [x] Touch targets: with the request and offer CTAs promoted to a sibling pair,
      check both clear **48×48 dp**
      ([accessibility_review_prompt.md](../reviews/accessibility_review_prompt.md)
      → *Touch Targets*). The offer pill's `vertical: 11` + 13 px label measured
      ≈38 dp and the request pill ≈46 dp. *Fixed with
      `ConstrainedBox(minHeight: kMinInteractiveDimension)` + an
      `Align(widthFactor: 1)` rather than tuned padding — a minimum survives the
      platform text scale, and font metrics don't decide whether the gate holds.*
- [x] Rewrite
      [`home_zero_state_card_test.dart`](../../app/test/presentation/widgets/home/home_zero_state_card_test.dart).
      The existing nudge test asserts the *buggy* arrangement and must be
      replaced, not extended. New assertions:
      - with a nudge: `NudgeContentView` exists, the canned example does not, the
        request title renders at **prompt** scale (16) rather than hero scale
        (22), and the nudge paints **above** both prompts — the two guards that
        together mean "one hero, not nested";
      - without a nudge: unchanged from today (hero + example + both CTAs);
      - both CTAs still fire `onAsk` / `onOffer` in both branches;
      - both pills clear 48 dp in both branches.
      *Dropped: asserting the nudge CTA fires `onNudgeConsumed`. The tap runs
      `NudgeContentView._openAction` first, which opens a create modal needing a
      fully-overridden `ProviderScope`; the pre-existing suite didn't cover it
      either, and the wiring is unchanged by this fix.*
- [x] **Verify the new tests fail on the old widget** before trusting them —
      `git checkout HEAD -- home_empty_states.dart`, re-run, restore. 4 of 8 fail
      (all three nudge-branch tests + touch targets), so none are vacuous.
- [x] Run `npm run generate`, then `cd app && flutter analyze` and
      `flutter test test/presentation/widgets/home/ --reporter expanded`
      (always `--reporter expanded` locally — the compact reporter duplicates
      lines via `\r`). *34/34 green.*
- [x] Run the client gates: `npm run lint:dart:a11y`, `npm run lint:dart:i18n`,
      `npm run lint:dart:size`, `npm run lint:ephemeral-comments` (or
      `npm run lint` for all of them). `homeZeroExample1..5` stay referenced by
      the no-nudge branch, so `check_arb_orphans.js` stays clean — if a later
      change drops the rotation, those five keys must be deleted in the same PR.
      *All green, including `lint:context-map`.*
- [x] Validate end to end in the hermetic env. Build first with
      `npm run build:e2e` — driving a stale embedded bundle is the top e2e trap
      ([client/testing.md](../client/testing.md) → *three traps*) — then seed a
      user with zero Needs-you decisions, let the e2e AI provider's nudge land,
      and screenshot the Home root. Confirm one hero, no ⓘ/headline overlap, and
      no clipped description. **Confirmed** — see *E2E validation* below.
- [x] Update [`client/inbox.md`](../client/inbox.md) → *Zero state* (line ~86)
      and *Inbox nudge* (line ~388), and [`nudges.md`](../nudges.md) →
      *Reused beyond the feed*, all of which currently describe the nudge as
      living inside the request hero. Advance both `freshness:` stamps; if a
      doc's `description:` changes, re-run `npm run generate:context-map` so
      `npm run lint:context-map` stays green. *Both stamped to `632668d0d`;
      `nudges.md` gained an own-frame architecture principle.*

**Validates:** That the reporter's screenshot cannot recur, and that the
no-nudge path is untouched. Also proves the zero-state-with-nudge state is
reachable in the hermetic env — which no test covers today.

**E2E validation (done).** Stack: `npm run db:start` → `npm run build:e2e` →
`cd e2e && npm run env:start -- --slug 2796` (port 8100), seeded one user in one
community with nothing on their plate. The e2e AI provider's `list_item` nudge
("Something gathering dust?") was ready on the first load. Reading order from
the ARIA snapshot, which is the ordering assertion the widget test makes,
confirmed in the real bundle:

```
group "Something gathering dust? A drill, a tent, a stand mixer — someone in
       Space Team could use it this week.
       Need a hand with something? A ladder, a ride, an hour of help. Start small.
       Have something to share? Lend a tool, host a walk, teach a skill.":
  - button "Share an item"      ← the nudge's own CTA, in its own frame
  - button "Ask the crew"
  - button "Offer something"
```

The screenshot shows the nudge as a single full-width hero, then the request
prompt with its filled sage pill, then the offer prompt with its ghost pill —
no nesting, no ⓘ/headline collision, no clipped description. The card renders
without background imagery in the hermetic env (the e2e provider produces
text-only nudges and no stock-image fetch runs), which is expected; prod
composites the same layout over a photograph.

Artifacts are under `e2e/investigations/2796/` (gitignored): `seed.mts`,
`probe-01-zero-state-with-nudge.mts`, `shots/`, `logs/`. Env torn down.

### Phase 2: Fit and chrome in the new host (contingent)

**Goal:** The nudge card, in its new zero-state host, doesn't clip its text or
paint its attribution button over its headline.

**Tasks:**
- [ ] Confirm against #2801's fix. If #2801 lands a shared sizing/chrome
      correction in `NudgeContentView` / `NudgeCardVariants`, this phase is
      **no work** — verify the zero-state host inherits it and close.
- [ ] If #2801 has not landed: stop `buildVariant1/2/3` from adding
      `MediaQuery.padding.top` to the ⓘ button's offset when the card is not
      full-bleed. Express this as a **named enum** (e.g.
      `NudgePresentation.fullScreen` / `.embedded`) threaded through
      `NudgeContentView`, not a `bool fullBleed` — a bare bool encoding a mode
      is exactly what
      [code_quality_review_prompt.md](../reviews/code_quality_review_prompt.md)
      §11 rejects, and three builders would each grow an opaque `true` at the
      call site. Default to `fullScreen` so the feed is untouched.
- [ ] Widget test: with a long headline + long description at the zero-state
      height, neither ellipsizes and the ⓘ button does not intersect the
      headline's rect.

**Validates:** That un-nesting fixed composition without inheriting the
constrained-host clipping #2801 reports. Explicitly scoped as contingent so the
two issues don't both edit the same sizing code.

### Phase 3: Kill "Plan an Experience" at the source → **split out as #2803**

*Filed as its own issue per the resolution above; the tasks below moved there
verbatim and are not part of this PR.*

**Goal:** No AI-generated user-facing string says "Experience".

**Tasks:**
- [ ] In [`server/ai/prompts.go`](../../server/ai/prompts.go), replace the
      `plan_experience` exemplar CTA `"Plan an Experience"` (line ~819) with
      Event-safe examples (`"Plan an Event"`, `"Plan something"`,
      `"Organize something"`), and add an explicit vocabulary rule to the nudge
      prompt: user-facing copy says *event*, never *experience*.
- [ ] Extend `SCANNED_FILES` in
      [`scripts/check_event_terminology_go.sh`](../../scripts/check_event_terminology_go.sh)
      to cover `server/ai/prompts.go`, adding
      `// experience-string-allow: <reason>` on the two genuinely
      prompt-internal lines (the app description at ~797 and the
      experience-suggestions prompt at ~857+). Note the existing exemption for
      space-free literals already skips `plan_experience` itself.
- [ ] Go test asserting the built nudge prompt contains no user-facing
      "Experience" exemplar.
- [ ] Run `./scripts/check_event_terminology_go.sh` and `npm run lint`.
- [ ] No backfill: nudges age out after 24h
      ([nudges.md](../nudges.md) → *Age expiry*), so the pool self-heals.

**Validates:** That the terminology rule is enforced where the strings are
actually *authored* (the prompt) rather than only where they are assembled, and
that the gate can't be routed around by an AI exemplar again.

## Related Issues

- **#2801** — *Planning nudge text is cut off on calendar screen.* Same reporter,
  seven minutes later, same root pressure (full-bleed nudge card in a
  fixed-height host). Distinct bug; owns the fit/clipping and ⓘ-position work
  that Phase 2 defers to it.
- **#2803** — *Nudge CTAs say "Plan an Experience".* Split out of this issue's
  Phase 3; the terminology violation visible in the same screenshot, whose
  source is the AI prompt exemplar at `server/ai/prompts.go:819`.
- **#2488** — *Inbox follow-ups: action cleanup, contextual nudges…* The parent
  that put the nudge on this surface in the first place. #2796 corrects the
  composition without walking back the capability.
- **#2435** — *Redesign Home tab (portfolio inbox) to v4.1 build spec.* The
  redesign that introduced `HomeZeroStateCard`.
- **#2799**, **#2800** — Same-day Home-screen user-feedback reports ("old events
  and items are awkward", "prompts to join but I'm already going"). Independent
  defects, but if a Home-screen batch is assembled they belong together.
- **#2016** — *Funnel Workshop and Nudge prefilled-draft success paths through
  UnifiedCreateModal.* Touches `NudgeContentView._openAction`; no conflict with
  this plan, which changes only where the card is mounted.

---

## Audit notes

Audited against the documentation context map (`docs/llms.txt`). Engaged
dimensions: **accessibility**, **code_quality**, **testing**, **tech_debt**;
`api_design`, `security`, `performance`, `error_handling`, and `dependency` were
considered and found not engaged (no proto/RPC surface, no auth or PII, no hot
path, no new dependency, no new failure mode — the nudge is already best-effort
server-side and null-safe client-side).

Fixes applied to the plan by the audit:

1. **Scope boundaries** added up front — proto conventions considered and N/A,
   widget-vs-repository layering unchanged, `check_viewmodel_tests.sh` N/A.
2. **Ephemeral phase markers** — explicit prohibition on "Phase N" comments in
   Dart/Go, enforced by `npm run lint:ephemeral-comments`.
3. **Missing CI gates** added to Phase 1: `lint:dart:a11y`, `lint:dart:i18n`,
   `lint:dart:size`, `lint:ephemeral-comments`, `flutter analyze`, plus the
   `--reporter expanded` requirement.
4. **Touch targets** — new task; the two prompt CTAs measure ≈38 dp and ≈46 dp
   against the 48 dp floor and become more exposed under Option A.
5. **Semantics identifiers vs. labels** — clarified that the new
   `semanticsIdentifier`s don't relax the l10n-sourced-`semanticsLabel` rule.
6. **e2e stale-bundle trap** — Phase 1's validation now requires
   `npm run build:e2e` before driving the hermetic stack.
7. **ARB orphans** — noted that the five example keys stay live via the no-nudge
   branch, and what would have to happen if that branch ever goes away.
8. **Boolean parameter** — Phase 2's proposed `bool fullBleed` replaced with a
   named `NudgePresentation` enum per the code-quality rubric §11.
9. **Context-map freshness** — doc updates must advance `freshness:` stamps and
   regenerate the map if any `description:` changes.

**Docs consulted:** `docs/llms.txt`; anchors `client/architecture.md`,
`client/conventions.md`, `proto_conventions.md`, `testing/architecture.md`,
`users/priorities.md`; review rubrics `reviews/accessibility_review_prompt.md`,
`reviews/code_quality_review_prompt.md`, `reviews/testing_review_prompt.md`;
leaves `client/inbox.md`, `client/accessibility.md`, `client/i18n.md`,
`client/testing.md`, `client/testing/semantics_identifiers.md`, `nudges.md`;
gates `scripts/check_a11y.js`, `scripts/check_ephemeral_comments.js`,
`scripts/check_event_terminology_go.sh`, `scripts/check_viewmodel_tests.sh`,
`scripts/check_arb_orphans.js`.
