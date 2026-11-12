# Issue #2801: Planning nudge text is cut off on calendar screen

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2801
**Filed:** 2026-07-29 | **Last activity:** 2026-07-29
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** [client/architecture.md](../client/architecture.md),
[client/conventions.md](../client/conventions.md),
[proto_conventions.md](../proto_conventions.md),
[client/calendar.md](../client/calendar.md),
[client/inbox.md](../client/inbox.md),
[nudges.md](../nudges.md),
[stock_imagery.md](../stock_imagery.md),
[client/accessibility.md](../client/accessibility.md),
[client/testing.md](../client/testing.md),
[reviews/accessibility_review_prompt.md](../reviews/accessibility_review_prompt.md),
[reviews/code_quality_review_prompt.md](../reviews/code_quality_review_prompt.md),
[reviews/testing_review_prompt.md](../reviews/testing_review_prompt.md),
plus the sibling plan [2796-nested-zero-state-nudge.md](2796-nested-zero-state-nudge.md)

## Problem Statement

On the **Plans** tab (`HomeCalendarScreen`, `embedded: true`), selecting an open
day renders the inbox nudge as a full-bleed `NudgeContentView` card in a
**fixed-height** host. The reporter's screenshot shows the card's description
line sliced horizontally through the middle of its glyphs — not ellipsized,
*clipped*. The issue body ("Should scroll or fit") names the two acceptable
outcomes.

The reporter's follow-up adds a second defect visible in the same screenshot:
the **circled ⓘ** in the card's top-right corner "seems to do nothing."

These are one bug and one near-bug in the same 40 lines of code:

1. **The fit is broken (confirmed defect).** The host pins the card to
   `clamp(200, 280)` px, but the card's internals are sized for a full screen.
   The headline is deliberately *not* flexible, so the description — the sole
   `Flexible` — absorbs the entire squeeze and is handed a box shorter than one
   line of its own text. `TextOverflow.ellipsis` does **not** rescue this:
   ellipsis only applies at the `maxLines` boundary, so when *height* is the
   binding constraint `RenderParagraph` sets `_needsClipping` and hard-clips the
   raster mid-glyph. At the 200 px floor the whole content `Column` overflows
   and the `ClipRRect` silently hides it.
2. **The ⓘ is wired, but reads as inert (design defect, not a dead handler).**
   It opens a photo-attribution bottom sheet, and that path is verified working
   (see *UI Investigation*). But the button is a **30×30 px** target with **no
   press feedback**, labeled with the generic semantics string **"Learn more"**,
   and its payoff is a single line of photo credit that appears **at the very
   bottom of the screen** — ~270 px away from where the finger landed, in the
   band the nav dock occupies. From the user's seat that is indistinguishable
   from "nothing happened."

Both trace to one root cause, already named in the #2796 plan: **the nudge card
has one presentation mode, sized for full-screen, and two hosts that aren't
full-screen** (the calendar's open-day card at 200–280 px and the Home zero
state's hero at 320 px).

**No contradictions** between the issue body and the follow-up — the follow-up
widens scope to a second defect in the same card.

## Context from Comments

**The GitHub issue has zero comments.** It is an app-feedback submission
(`user-feedback` label, auto-filed from the in-app reporter with device context:
Android 17, Pixel 9 Pro, app 0.28.0 build 9326, PROD). The only additional
requirement came through the plan request itself ("fix the 'circled i' icon that
seems to do nothing").

The absence of discussion is not a blocker here, because the surrounding
paper trail is unusually complete — the same reporter filed **#2796** seven
minutes earlier about the sibling host, and that plan pre-assigned this work:

- [`2796-nested-zero-state-nudge.md:98–108`](2796-nested-zero-state-nudge.md)
  names the shared root cause and explicitly keeps #2796 (composition) and
  #2801 (fit) as separate fixes.
- [`2796-nested-zero-state-nudge.md:252–257`](2796-nested-zero-state-nudge.md):
  *"Does the attribution ⓘ button need a non-full-screen position? —
  Recommendation: **yes, and it belongs in #2801's scope**, since the calendar
  host has the identical overlap."*
- [`2796-nested-zero-state-nudge.md:418–441`](2796-nested-zero-state-nudge.md)
  (Phase 2, contingent) prescribes the shape: a **named enum**
  (`NudgePresentation.fullScreen` / `.embedded`) threaded through
  `NudgeContentView` — explicitly *not* a `bool fullBleed`, which
  [`code_quality_review_prompt.md`](../reviews/code_quality_review_prompt.md)
  §11 rejects. #2796 is **CLOSED**, so that phase never ran and this issue owns
  it outright.
- [`home_empty_states.dart:87–94`](../../app/lib/presentation/widgets/home/home_empty_states.dart)
  carries the matching in-code note: *"Tuning the fit so long copy stops
  ellipsizing is #2801's job, and its answer should land here and on the
  calendar's open-day card together."*
- [`client/inbox.md:407–411`](../client/inbox.md) documents both hosts' bounds
  and points at #2801.

Related but **out of scope**: **#2803** (open) — the CTA label "Plan an
Experience" violates the Event-not-Experience rule; its source is an AI prompt
exemplar at `server/ai/prompts.go:819`. Server + CI change, deliberately split
out of #2796.

## UI Investigation

The failure was bisected deterministically with a throwaway Flutter
widget test that reproduced the reporter's exact host and copy (Pixel 9 Pro
geometry: 448×952 logical; the real `CalendarDayDetail` host with
`SingleChildScrollView(reverse: true)` + `Align(bottomLeft)` +
`EdgeInsets.fromLTRB(22, 24, 22, 92)`; the screenshot's headline "Get the Space
Team together for an adventure" and a weather-woven description). The probe was
deleted after measurement; the numbers below are what it printed.

A first pass at this plan claimed the card *couldn't* be reached in the hermetic
e2e stack, on the grounds that nudge generation needs both an `ai.Provider` and a
`StockImageryProvider` and the stack injects neither. **Half wrong:**
[`scripts/start_e2e_server.sh`](../../scripts/start_e2e_server.sh) passes
`--mock-ai-provider`, and `InboxNudge` queries with `requireMedia: false` — so a
text-only nudge *does* surface on an open calendar day, on its solid background
rather than a photo. The stock-imagery half holds, which is the part that matters
for the attribution line: without a `StockImageryProvider` there is no
`Attribution`, so the credit can't be exercised there. Screenshots of the card
live under `e2e/investigations/2801/` (gitignored).

**Verdict: client render.** No server, data, or state involvement.

### Measured at the calendar's preferred height (card = 280 px)

| Element | Measured box | Needed |
|---|---|---|
| Headline (`nudgeHeadlineLargeStyle`, 32 px / 1.15, `maxLines: 3`) | 348 × **111** | 111 (fits — it is not `Flexible`) |
| Description (`nudgeBodyStyle`, 16 px / 1.6 → **25.6 px/line**, `maxLines: 4`) | 348 × **15** | **104** (`min`/`maxIntrinsicHeight` both 104) |
| CTA pill label | 253.8 × 20 | — |

The arithmetic that produces it: `280 − 32×2` padding `= 216` for the `Column`;
`216 − 111` (headline) `− 14 − 28` (gaps) `− 48` (CTA) **`= 15`**. The
description gets 15 px against a 25.6 px line box → **59 % of one line renders,
sliced through the glyphs**. Exactly the screenshot.

### Measured at the floor (card = 200 px, small screens / landscape)

```
exception          = A RenderFlex overflowed by 65 pixels on the bottom.
description size   = Size(348.0, 0.0)
```

Worse and previously unreported: the description disappears entirely **and** the
content `Column` overflows by 65 px — hidden in production only because the host
wraps the card in `ClipRRect`. The CTA button itself is inside that overflow.

### The ⓘ button

```
info icon found    = 1
info icon size     = Size(30.0, 30.0)  topLeft = Offset(379.0, 593.0)
after tap: "Photo by " found = 1
after tap: "Jane Doe"  found = 1
sheet text global pos = Offset(178.3, 892.0)  in screen Size(448.0, 952.0)
```

So the handler **is** wired and the attribution sheet **does** open. The
"does nothing" perception is fully explained by four verifiable properties:

1. **30×30 px target** — violates
   [`accessibility_review_prompt.md`](../reviews/accessibility_review_prompt.md)
   §3 (*"touch targets are at least 48×48 dp"*). Easy to miss on a real thumb,
   especially at a card corner inside a scroll view.
2. **No press feedback** — `_NudgeInfoButton` passes no `inkBorderRadius`, so
   `Tappable` falls through to a bare `GestureDetector`; there is no ripple, no
   highlight, nothing to confirm the tap registered.
3. **The payoff lands 270 px away.** The sheet's text sits at y ≈ 892 of 952 —
   the bottom band the `NavDock` capsule occupies — while the tap happened at
   y ≈ 600. Eyes stay at the corner; nothing changes there.
4. **The sheet says almost nothing** — one line, "Photo by *X* on *Unsplash*",
   which is not what a ⓘ on an AI-generated planning card promises. Its
   semantics label is the generic `a11yLearnMore` = "Learn more".

Additionally, `Positioned(top: MediaQuery.of(context).padding.top + 12)` is a
full-screen assumption. On the calendar it happens to evaluate to `12` (the
screen's own `SafeArea` already consumed the inset via `MediaQuery.removePadding`),
so it looks right today — but it is right by accident, and it is the exact line
#2796 flagged.

### Key files

| File | Role |
|---|---|
| [`calendar_day_detail.dart:255–297`](../../app/lib/presentation/widgets/home/calendar/calendar_day_detail.dart) | The host: `ClipRRect(18)` + `SizedBox(height: clamp(200, 280))`; `_nudgeCardHeight = 280`, `_nudgeCardMinHeight = 200`; weaves the weather summary into the description via `homeCalNudgeWeatherWeave` (making it *longer* than the server's copy) |
| [`home_calendar_screen.dart:203–239`](../../app/lib/presentation/screens/portfolio/home_calendar_screen.dart) | Computes `maxHeight = constraints.maxHeight − 24 − bottomPad`, `bottomPad = 24 + NavDock.bottomContentInset (70)` when embedded |
| [`nudge_card_variants.dart:177–245`](../../app/lib/presentation/widgets/nudge/nudge_card_variants.dart) | `buildVariant1` — `Stack(fit: StackFit.expand)`, `SafeArea`, `EdgeInsets.symmetric(horizontal: 28, vertical: 32)`, non-flexible headline, `Flexible` description |
| [`nudge_card_variants.dart:138–162, 237–242`](../../app/lib/presentation/widgets/nudge/nudge_card_variants.dart) | `_NudgeInfoButton` + its three `Positioned` call sites (variants 1/2/3) |
| [`nudge_content_view.dart:42–92, 194–300`](../../app/lib/presentation/widgets/nudge/nudge_content_view.dart) | Resolves `mediaAttributionProvider`, gates `onInfoTap` on a non-null `Attribution`, and owns `_NudgeAttributionSheet` |
| [`photo_attribution_line.dart`](../../app/lib/presentation/widgets/content/photo_attribution_line.dart) | **The existing, shipped pattern** for the same job: a small right-aligned italic "Photo by *X* on *Y*" credit with tappable links and a `boxed` variant for legibility over imagery |
| [`app_theme.dart:28–51`](../../app/lib/core/theme/app_theme.dart) | `nudgeHeadlineLargeStyle` (32/1.15), `nudgeHeadlineMediumStyle` (26/1.15), `nudgeBodyStyle` (16/1.6) |

## Potential Solutions

### Option A: A named `NudgePresentation` mode, plus a height-derived line budget

- **Approach:** Thread `NudgePresentation { fullScreen, embedded }` through
  `NudgeContentView` into all three `buildVariant*` builders (defaulting to
  `fullScreen`, so the feed is byte-identical). In `embedded`:
  - drop the internal `SafeArea` and stop adding `MediaQuery.padding.top` to the
    chrome offset — an embedded card owns no screen edges;
  - tighten the metrics (padding `28/32 → 20/18`, gaps `14/28 → 10/16`) and use
    the medium headline style rather than the 32 px large one;
  - **make each text block derive its own line budget.** Wrap both the headline
    and the description in `Flexible` (loose, flex `3` and `2`), and inside each
    put a `LayoutBuilder` that computes
    `maxLines = (constraints.maxHeight ÷ scaledLineHeight).floor()` — rendering
    `SizedBox.shrink()` when that is `0` — where
    `scaledLineHeight = MediaQuery.textScalerOf(context).scale(style.fontSize!) *
    style.height!`.

  The last bullet is the part that actually fixes the reported symptom, and the
  reason it is expressed this way rather than as arithmetic over the card height
  matters: **a `Flexible`'s incoming `maxHeight` already *is* the space left
  after every fixed sibling was measured.** Deriving `maxLines` from it needs no
  assumption about the CTA's height, the gaps, or the padding — so it stays
  correct under 200 % text scaling (`accessibility_review_prompt.md` §4), where
  a hard-coded reserve would silently reintroduce the clip. The ellipsis then
  lands on a **line boundary** instead of the raster being cut, for any copy
  length the LLM produces.

  Flex factors also answer the concern recorded in the existing comment at
  [`nudge_card_variants.dart:204–210`](../../app/lib/presentation/widgets/nudge/nudge_card_variants.dart)
  (an equal split starves a multi-line headline): `3:2` is not a 50/50 split.
  When the headline is short its unused share is absorbed by the `Column`'s
  `MainAxisAlignment.center` rather than passed to the description — acceptable,
  because a short headline is precisely the case where the card was never
  cramped.
- **Pros:** Fixes both embedded hosts (calendar 280, zero state 320) from one
  seam; the feed is untouched by construction; matches the fix #2796 already
  designed and reviewed; kills the "right by accident" `padding.top` line;
  robust to unbounded server copy — no magic character budget.
- **Cons:** Two metric sets to keep coherent; two extra `LayoutBuilder`s per
  card (negligible — they add one layout pass on a widget that rebuilds only on
  day change); the embedded card reads visually quieter than the feed's
  (arguably correct, but it is a design change).
- **Effort:** Medium.

### Option B: Height-derived line budget only (no presentation mode)

- **Approach:** Keep one presentation. Add the `Flexible` + `LayoutBuilder`
  line-budget from Option A to both texts, so they shrink in line count rather
  than clip.
- **Pros:** Smallest diff; no new API; fixes the mid-glyph clip and the 65 px
  floor overflow.
- **Cons:** Leaves 32 px type and 32 px padding inside a 280 px box — at the
  floor the headline collapses to ~1 line and the card reads as a shouty
  fragment. Does **not** address the ⓘ offset, so #2796's Phase 2 stays open.
  Trades a clipping bug for a truncation bug.
- **Effort:** Small.

### Option C: Let the card size to its content and scroll

- **Approach:** Give `NudgeContentView` an intrinsic-height composition
  (`Stack` with the background as `Positioned.fill` and the content `Column`
  driving height) and drop the host's `SizedBox`. The calendar's existing
  `SingleChildScrollView(reverse: true)` absorbs any overflow — the issue body's
  first suggestion, "should scroll."
- **Pros:** No clipping possible, ever; deletes `_nudgeCardHeight` /
  `_nudgeCardMinHeight` / `_nudgeHeroHeight` and the "card cannot self-size"
  caveat repeated across three docs; conceptually the smallest model.
- **Cons:** Directly reverses a deliberate decision — the host comment says
  *"Fill the space under the grid without ever forcing a scroll"* — and the
  calendar detail is a bottom-anchored panel under a fixed month grid, so a
  taller card pushes content below the fold on exactly the screens that are
  already tight. Card height would jitter day-to-day as the user taps around the
  month. Touches all three hosts and the feed's `PageView` at once.
- **Effort:** Large.

### The ⓘ, decided separately (it is a chrome question, not a fit question)

- **C1 — Enlarge and label it.** Keep the ⓘ + sheet; pad the target to 48×48
  (visual stays 32), add `inkBorderRadius` for press feedback, swap
  `a11yLearnMore` for a photo-credit-specific label.
  *Cheap, but still a mystery icon whose reward is one line of credit.*
- **C2 — Replace it with the credit itself (recommended).** Delete
  `_NudgeInfoButton` and `_NudgeAttributionSheet`; render the existing
  [`PhotoAttributionLine(boxed: true)`](../../app/lib/presentation/widgets/content/photo_attribution_line.dart)
  as a `Positioned` bottom-right overlay on the card. Attribution becomes
  *always visible* — which is what Unsplash actually requires
  ([`stock_imagery.md:75`](../stock_imagery.md): *"Attribution: Required"*) and
  strictly better than burying it behind an icon — the mystery affordance
  disappears, the corner is reclaimed, and the app converges on one attribution
  pattern instead of two. Costs no layout height (it is an overlay, not a
  `Column` child).
  *Net −60 lines of client code.*
  **Caveat this plan must not gloss:** `PhotoAttributionLine` renders its two
  links as **10.5 px** text with no padding, so on its own it trades one 30 px
  target for two smaller ones — worse against
  [`accessibility_review_prompt.md`](../reviews/accessibility_review_prompt.md)
  §3 (*"Hit Areas: check if small icons have expanded hit areas"*), not better.
  Adopting it here therefore requires giving each `Tappable` a padded hit area
  (transparent `EdgeInsets` inside the `Tappable`, visual size unchanged) — a
  change that also improves every content view already using it.

## Recommendation

**Option A + C2, in one PR.**

Option A because it is the fix #2796 already specified and left for this issue,
because it fixes both embedded hosts from one seam, and because — critically —
the copy is LLM-generated with only *soft* word guidance in the prompt
(`server/ai/prompts.go:774–855` asks for "max 10 words" / "1–2 sentences" and
nothing enforces it). Any fix built on a fixed line count is a fix for today's
prompt output. The height-derived line budget is the only variant that degrades
correctly for copy nobody has seen yet.

Option B is a real improvement and would close the reported symptom, but it
leaves the card visually wrong at 200–280 px and leaves #2796's Phase 2 open —
so the next screenshot re-opens this file.

Option C is the issue reporter's own first suggestion and is not unreasonable,
but it inverts an explicit, commented design decision on a bottom-anchored panel
and would touch the feed's `PageView` sizing. If Option A's embedded metrics
still can't fit the copy on the smallest supported screen, C becomes the
fallback for the calendar host only — that is what the Phase 1 validation below
is designed to detect.

C2 rather than C1 because "make the small mystery button bigger" answers the
letter of the report and not its substance. The user tapped ⓘ expecting to learn
something; the honest answer is that there was never anything worth hiding, and
the credit that *is* legally required should simply be on screen. The widget to
do it already exists and is in production on every content view.

**Explicitly out of scope:** #2803 (the "Plan an Experience" CTA label). It is a
server prompt + CI-gate change and mixing it into a client-layout PR muddies both
reviews — the same call #2796 made.

## Open Questions

- **Should `embedded` scale the type down, or only tighten padding and line
  budgets?** — Recommendation: **scale it down** — use
  `nudgeHeadlineMediumStyle` (26 px) for embedded variant 1. At 280 px, 32 px
  type spends 111 px (40 % of the card) on three headline lines. 26 px yields
  ~90 px and buys two clean description lines. Validate against the reporter's
  real copy, not a synthetic string.
- **Does the calendar host still need `_nudgeCardMinHeight = 200`, or should
  the floor rise?** — Recommendation: **keep 200 but make it survivable.** With
  Option A's line budget the card degrades to headline + one description line +
  CTA at 200 px instead of overflowing by 65 px. Raising the floor would push
  the card under the nav dock on short screens, trading one clip for another.
  Assert the no-overflow property in a test at exactly 200.
- **Attribution overlay on the feed's full-screen card too, or embedded only?**
  — Recommendation: **both**, i.e. remove `_NudgeInfoButton` entirely. Two
  attribution patterns for the same media class is the kind of divergence
  `client/conventions.md` exists to prevent, and the feed card has far more room
  for a 10.5 px line than the calendar does.
- **What happens to the now-orphaned `a11yLearnMore` ARB key?** —
  Recommendation: **delete it from `app_en.arb` and `app_es.arb`.**
  `_NudgeInfoButton` is its only consumer in `app/lib` (verified), so
  `npm run lint:dart:arb-orphans` fails the build otherwise. Do not add it to
  `scripts/arb_orphan_allowlist.txt`.
- **Should the server cap `headline` / `description` length?** —
  Recommendation: **no, not in this PR.** The client must be robust to long copy
  regardless (the calendar *lengthens* the description client-side with the
  weather weave, so no server cap can be authoritative). If the copy is
  consistently too long after this lands, that is prompt tuning under #2803's
  neighborhood, not a layout fix.
- **Should this land golden/visual-regression tests?** —
  [`testing_review_prompt.md`](../reviews/testing_review_prompt.md) §3 lists
  golden tests as an opportunity, and a clipping bug is exactly what they catch.
  Recommendation: **no.** The repo has zero `matchesGoldenFile` usage and no
  golden infrastructure; standing one up for this issue contradicts CLAUDE.md's
  *Minimal Dependencies*. The measured-box assertions in Phase 1 (rendered
  height ≥ one line box, no exception, CTA within bounds) test the same property
  deterministically and without image fixtures.
- **Does the zero-state host (320 px) need its own tuning pass?** —
  Recommendation: **verify, don't assume.** By the same arithmetic 320 px leaves
  ~55 px — two clean description lines — so it may already be adequate once the
  line budget is height-derived. Phase 3 measures it rather than guessing, and
  removes the stale TODO in `home_empty_states.dart` either way.

## Implementation Plan

### Phase 1: Make the fit correct

**Goal:** No text is ever raster-clipped and no `RenderFlex` overflows, at every
height in `[200, 320]`, for copy of any length.

**Tasks:**
- [ ] Add `enum NudgePresentation { fullScreen, embedded }` (new file under
      `app/lib/presentation/widgets/nudge/`, with a doc comment explaining what
      each host owns). Thread it through `NudgeContentView` into
      `buildVariant1/2/3`, defaulting to `fullScreen`.
- [ ] In `embedded`: skip the internal `SafeArea`; use the tightened metrics and
      `nudgeHeadlineMediumStyle`. Every metric (paddings, gaps, flex factors)
      goes in **named constants**, not inline literals — `code_quality_review_prompt.md`
      §10. Doc comments reference `#2801`; **no phase markers**, which
      `npm run lint:ephemeral-comments` fails the build on
      ([`client/conventions.md`](../client/conventions.md) → *Dart comments*).
- [ ] Replace the hard-coded `maxLines: 3` / `maxLines: 4` with the
      `Flexible` + inner-`LayoutBuilder` budget described in Option A, using
      `MediaQuery.textScalerOf(context)` for the line height. Keep
      `TextOverflow.ellipsis` so truncation shows "…".
- [ ] Pass `NudgePresentation.embedded` from `calendar_day_detail.dart` and
      `home_empty_states.dart`; leave `FeedScreen` on the default.
- [ ] Widget tests in
      `app/test/presentation/widgets/nudge/nudge_card_variants_test.dart`:
      at heights 200 / 280 / 320 with a long headline **and** a long
      description, assert (a) `tester.takeException()` is null — this is the
      regression lock for the 65 px floor overflow, (b) the description's
      rendered height is ≥ one full line box **or** it is absent entirely (the
      `SizedBox.shrink()` branch) — never a fractional line, (c) the CTA is
      fully within the card's bounds.  The existing height-constrained group at
      `:260–302` only asserts the *headline*; that gap is what let this ship.
- [ ] Repeat the 200 px case at `TextScaler.linear(2.0)`
      ([`accessibility_review_prompt.md`](../reviews/accessibility_review_prompt.md)
      §4, *"UI handles large text sizes (up to 200%)"*). This is the case a
      fixed line reserve would have broken and the only reason the budget is
      derived from the `Flexible`'s own constraints.
- [ ] Widget test in `calendar_day_detail_test.dart` reproducing the reporter's
      copy at the real Plans geometry, asserting the description is unclipped.
      Note the standing gap this does *not* close: `HomeCalendarScreen` itself
      has **no test file at all**, so the `maxHeight` arithmetic at
      `home_calendar_screen.dart:221–222` stays covered only indirectly. Out of
      scope here; worth its own issue if the screen grows more layout logic.

**Validates:** That an embedded presentation mode is sufficient — i.e. that
Option C (content-sized + scroll) is not needed. If the smallest supported
screen still can't seat headline + one description line + CTA at 200 px, this
phase surfaces it as a failing test rather than a field report.

### Phase 2: Retire the ⓘ in favor of a visible credit

**Goal:** The photo credit is on screen; there is no affordance that looks
tappable and isn't worth tapping.

**Tasks:**
- [ ] Delete `_NudgeInfoButton` and its three `Positioned` call sites; delete
      `_NudgeAttributionSheet` and `_showAttributionSheet` from
      `nudge_content_view.dart`.
- [ ] Render `PhotoAttributionLine(attribution: …, boxed: true)` as a
      `Positioned` bottom-right overlay in each variant when `attribution` is
      non-null, so it costs no layout height. Keep it the **last** `Stack`
      child so the CTA still precedes it in reading order.
- [ ] ~~Give `PhotoAttributionLine`'s two links a padded hit area~~ —
      **descoped by the reporter.** The credit is now always visible, so
      nothing is *hidden* behind a small target; enlarging the links would
      change every content view already using the widget and belongs in its own
      change. The C2 caveat stands as a known follow-up.
- [ ] Remove `a11yLearnMore` from `app_en.arb` and `app_es.arb`; confirm
      `npm run lint:dart:arb-orphans` and `npm run lint:dart:l10n-parity` pass,
      and that nothing was added to `scripts/arb_orphan_allowlist.txt`.
- [ ] Run `npm run lint:dart:a11y`. The allowlist
      (`scripts/a11y_allowlist.txt`) is **empty** and is a ratchet that may
      shrink but never grow — deleting the last `showAccessibleModal` caller in
      this file must not require a new entry.
- [ ] Widget test (none exists today for any of this — `info_outline`,
      `a11yLearnMore`, `mediaAttributionProvider` have zero test hits): with an
      `Attribution` override, the credit renders, both links are `Tappable` with
      `isLink: true`, their hit rects are ≥ 44 px tall, and the line does not
      intersect the headline's or CTA's rect at 200 px.

**Validates:** That the required Unsplash attribution is satisfied more visibly
than before while the reported dead-feeling affordance is gone — and that the
app has one attribution pattern instead of two.

### Phase 3: Adopt on the sibling host and close the paper trail

**Goal:** The zero state inherits the fix; the docs and TODOs stop pointing at
an open issue.

**Tasks:**
- [ ] Measure the zero-state hero at 320 px with the same long-copy fixture;
      tune `_nudgeHeroHeight` only if the test demands it.
- [ ] Remove the stale TODO block in `home_empty_states.dart:87–94` (the note
      that defers fit tuning to #2801) now that it is answered.
- [ ] Update [`nudges.md`](../nudges.md) → *Client Implementation* and *Nudge
      Variants* to document `NudgePresentation` and the attribution line;
      update [`client/inbox.md`](../client/inbox.md) (*Inbox nudge*) and
      [`client/calendar.md`](../client/calendar.md) (*Rendering*) where they
      describe the card's bounds.
- [ ] Run `npm run lint:context-map`. Do **not** hand-edit the `freshness:`
      stamps on the touched docs — [`context_map.md`](../context_map.md) →
      *Prose freshness* says they are "written only by the skill, never by
      hand", and the nightly ratchet re-verifies the docs whose governed code
      changed.
- [ ] Run `npm run lint:dart:a11y`, `lint:dart:size`, and the Flutter suite with
      `--reporter expanded`.

**Validates:** That #2796's contingent Phase 2 is genuinely discharged and that
the next reader of these files is not sent to a closed issue.

## Related Issues

- **#2796** (CLOSED) — *"Home screen shows 'plan an experience' nudge inside
  'need a hand with something' nudge."* Same reporter, filed 7 minutes earlier,
  same root cause (one presentation mode, non-full-screen hosts). Its Phase 1
  (un-nesting the zero state) shipped; its Phase 2 (fit + ⓘ chrome) was written
  as contingent on this issue and never ran. This plan owns it.
- **#2803** (OPEN) — *"Nudge CTAs say 'Plan an Experience' — AI prompt seeds the
  banned term."* Visible in this issue's screenshot; deliberately out of scope
  (server prompt + CI gate).
- **#2488** (OPEN) — *"Inbox follow-ups: action cleanup, contextual nudges,
  transactional bulk edits, and UX test coverage."* The epic that introduced the
  inbox nudge surface.

---

## Audit

Run against the documentation context map. Engaged dimensions: **accessibility**,
**code_quality**, **testing**, **tech_debt**. Side: client only — no proto, RPC,
storage, auth, or infra surface, so `api_design`, `security`, `observability`,
`error_handling`, and `dependency` are not in play.

**Fixes applied to this plan:**

1. **The line-budget algorithm was under-specified and would have broken under
   text scaling.** The first draft said "compute `maxLines` from the height
   remaining after the fixed children" — arithmetic that has to assume the CTA
   button's and gaps' heights, both of which move with
   `MediaQuery.textScaler`. Replaced with `Flexible` + an inner `LayoutBuilder`
   that reads its own incoming `maxHeight`, which *is* the post-fixed-children
   remainder by construction. Added the `textScalerOf` line-height term and a
   200 %-text-scale test case (`accessibility_review_prompt.md` §4).
2. **The C2 recommendation understated its own accessibility cost.**
   `PhotoAttributionLine` renders 10.5 px unpadded links; swapping a 30 px
   button for two smaller targets is a regression against
   `accessibility_review_prompt.md` §3 unless padded hit areas ship with it.
   Added as an explicit caveat and a Phase 2 task.
3. **Magic numbers.** Phase 1 now requires the embedded metrics to be named
   constants (`code_quality_review_prompt.md` §10).
4. **Ephemeral comments.** Added the `client/conventions.md` → *Dart comments*
   constraint: the new enum and metric constants must not carry phase markers;
   `npm run lint:ephemeral-comments` fails the build on them. Issue numbers are
   allowed.
5. **a11y ratchet.** Phase 2 deletes this file's only `showAccessibleModal`
   caller; added the check that `scripts/a11y_allowlist.txt` (currently empty,
   may shrink but never grow) needs no new entry.
6. **Golden tests considered and declined**, with the reason recorded in Open
   Questions — `testing_review_prompt.md` §3 suggests them, but the repo has no
   `matchesGoldenFile` usage at all and CLAUDE.md's *Minimal Dependencies* rule
   argues against standing the infrastructure up for one card.
7. **Named the untested-file gap** rather than silently leaving it:
   `HomeCalendarScreen` has no test file, so its `maxHeight` arithmetic stays
   indirectly covered (`testing_review_prompt.md` §1).

**Checked and already compliant:**

- Widget/ViewModel split (`client/architecture.md`) — this is presentation-only;
  no repository or viewmodel changes, and the card keeps reading media through
  `mediaUrlProvider` / `mediaAttributionProvider` rather than a repository.
- `CachedMediaImage` for user media (`client/conventions.md` → *Media Display*)
  — unchanged; the background already uses it with a stable `mediaId` cache key.
- Accessibility primitives (`client/accessibility.md`) — `Tappable` everywhere,
  no raw `GestureDetector`/`InkWell`, all semantics labels from `context.l10n`.
- l10n (`client/i18n.md`) — the plan adds no strings; it *removes* one and keeps
  `app_en.arb`/`app_es.arb` in parity.
- "Event", not "Experience" (`client/conventions.md`) — no user-facing string is
  authored here; the offending CTA label is #2803's, correctly left out of scope.
- File size (`npm run lint:dart:size`) — `nudge_card_variants.dart` is 443 lines
  and `calendar_day_detail.dart` 441; both stay well under the 1,000-line cap.
- Design tokens (`client/design.md`) — reuses existing `AppTheme` nudge styles
  and `GlassTokens`; introduces no raw colors, so `lint:dart:colors` is unaffected.

**Docs consulted:** `docs/llms.txt`; anchors `client/architecture.md`,
`client/conventions.md`, `proto_conventions.md`, `testing/architecture.md`;
review rubrics `accessibility_review_prompt.md`, `code_quality_review_prompt.md`,
`testing_review_prompt.md`; leaf docs `client/calendar.md`, `client/inbox.md`,
`nudges.md`, `stock_imagery.md`, `client/accessibility.md`, `client/i18n.md`,
`client/design.md`, `client/testing.md`, `weather.md`; plus `CLAUDE.md` and the
sibling plan `docs/issues/2796-nested-zero-state-nudge.md`.
