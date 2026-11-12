# Issue #2443: Fix small issues with requests & experiences (deep links, swipe-to-close, chip reuse, participation list cap)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2443
**Filed:** 2026-06-12 | **Last activity:** 2026-06-12
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/client/deep_linking.md`, `docs/client/modals.md` (Morph-reveal content panels), `docs/client/needs.md` (parity contract), `docs/planning.md`, `docs/client/i18n.md`, `docs/client/content.md`, `docs/client/architecture.md` (anchor), `docs/client/conventions.md` (anchor), `docs/proto_conventions.md` (anchor); review prompts: accessibility, code_quality, testing, tech_debt

## Problem Statement

Four small, mostly-independent polish items across the **request** and
**experience** content surfaces. Crucially, three of the four are **verification /
regression-guard** tasks — the feature already exists in the code and the issue
asks us to confirm it still works after the recent content-view rewrite — while
**item 3 (chip reuse + quantity)** is the only one requiring real new
implementation. The plan below is honest about that split so effort lands where
it matters.

The four items:

1. **Notification deep links open the expanded conversation view** on the new
   `requestContentView` / `experienceContentView`.
2. **Swipe-to-close still works** on `requestScreen` / `experienceScreen`.
3. **Need/contribution chips reuse one shared widget** and show a `[n]X` quantity
   prefix whenever quantity > 1.
4. **Cap the participation list** on the request content view to fewer than five
   items (needs + contributions), with a "+N more" indicator.

## Context from Comments

The issue has **no comments** — it was filed and is being planned in the same
session. There is no maintainer back-and-forth to reconcile, so the four bullet
points in the body are the full spec. This is a signal that the "Open Questions"
below should be read carefully before implementation, especially the `[n]X` vs
`×N` format question (item 3) and whether item 4's existing cap already satisfies
the request.

## What the codebase already does (research findings)

| Item | Current state | Verdict |
|------|---------------|---------|
| 1 — deep link → expanded conversation | `RequestContentView.initState` calls `_expandConversation(_defaultPanelRect())` when `widget.initialTab != 0` ([request_content_view.dart:127-129](../../app/lib/presentation/screens/request/request_content_view.dart#L127-L129)) → morph panel. **Experience did NOT** — it called `setActiveTab(chatTabIndex)`, opening the in-place conversation *carousel* instead of the expanded morph panel, diverging from both the request and the experience's own discussion-card tap (`onExpandConversation: _expandConversation`). Notifications append `?tab=chat` when a `conversation_id` is present ([fcm_service.dart:557-575](../../app/lib/services/fcm_service.dart#L557-L575)); the router maps `?tab=chat` → `initialTab` ([app_router.dart:580-618](../../app/lib/core/router/app_router.dart#L580-L618)). | **FIXED (not verify-only).** Experience deep-link now calls `_expandConversation(...)` so both content views open the morph expanded panel. `ripls://conversation/<id>` is declared in `AndroidManifest.xml` but **not** parsed by the router (out of scope, see Open Questions). |
| 2 — swipe-to-close | Both screens mix in `SwipeToCloseMixin` and wrap their body in `buildSwipeableContent(...)` → `SwipeToCloseWrapper` (`onHorizontalDragEnd`, 300 px/s, right-swipe) ([request_screen.dart:35-38](../../app/lib/presentation/screens/request/request_screen.dart#L35-L38), [experience_screen.dart:36-39](../../app/lib/presentation/screens/experience/experience_screen.dart#L36-L39), [swipe_to_close_wrapper.dart](../../app/lib/presentation/widgets/swipe_to_close_wrapper.dart)). | **Implemented — verify only.** Risk: inner horizontal-drag handlers (`ContentMorphPanel`, the experience conversation carousel) may swallow the screen-level swipe. |
| 3 — chip reuse + quantity | **Fragmented.** Three+ renderings of needs/contributions: `ContentNeedChip` (dark surface, **no quantity**), `NeedClaimChip`→`GlassChip` (**`×N` badge**), and the request card's **bespoke `_itemRow`** rows (`_needRow` / `_bringingRow` in [request_read_shell.dart:389-434](../../app/lib/presentation/screens/request/widgets/request_read_shell.dart#L389-L434), **not a shared chip, no quantity**), plus `ClaimedRow` ("+N" name suffix). | **Real work.** Consolidation + add `[n]X`. |
| 4 — participation list cap | The request card already caps at **4** items total with a "+N more" row ([request_read_shell.dart:278-301](../../app/lib/presentation/screens/request/widgets/request_read_shell.dart#L278-L301), `requestMoreItems(count)` l10n). The experience equivalent (`experience_read_shell.dart`) does **not** cap. | **Largely implemented — verify the cap math + "+N more" copy.** 4 < 5 already satisfies "less than five". **⚠ Parity flag — see below.** |

> **⚠ Parity contract (audit).** `docs/client/needs.md` and `docs/planning.md`
> make Experience/Request parity a *design contract*: "New differences between
> the two scopes should be treated as bugs" unless listed under *Intentional
> differences* (capping is **not** listed). The request participation card capping
> at 4 while the experience card shows all participants is exactly such a
> divergence. The issue only asks to fix the request side, so this plan does too —
> but the divergence must be **explicitly resolved**: either (a) apply the same cap
> to the experience collapsed card (preferred — true parity, and the experience
> card has the same unbounded-height risk), or (b) record it as a deliberate
> divergence with rationale. Defaulting to silent request-only behaviour violates
> the contract. See Open Questions and Phase 1.

## Potential Solutions

The four items are independent; the only design choice with real options is
**item 3** (and a smaller one for item 4). Items 1 and 2 are verification tasks
with at most a small fix if the verification fails.

### Item 3 — chip consolidation strategy

#### Option A: One canonical chip widget, quantity built in
- **Approach:** Pick `NeedClaimChip` (already glass-based, already quantity-aware
  via `GlassChip`) as the single chip for *interactive* claim/bring surfaces, and
  add an optional `quantity` to `ContentNeedChip` for the dark content surface
  (pitching-in). Migrate the request card's bespoke `_itemRow` rows to the shared
  chip so the participation widget reuses the same widget as everywhere else.
  Standardize the badge format on `[n]X` (e.g. `2X`).
- **Pros:** True single-source-of-truth; matches the issue literally ("reusing the
  same widgets"); quantity becomes a first-class chip concern.
- **Cons:** The request participation card today is a *row* layout (leading
  avatar/circle + name + trailing who), not a *chip* layout — swapping it to a
  chip changes that surface's visual density. Largest blast radius.
- **Effort:** Medium.

#### Option B: Shared quantity helper, keep two chip families
- **Approach:** Keep `ContentNeedChip` (content/chip surfaces) and the row
  renderers as distinct layouts, but extract a single `QuantityBadge`/`[n]X`
  formatter + widget and ensure **every** need/contribution rendering that can
  have quantity > 1 composes it. Audit all call sites; wire quantity through where
  it's currently dropped (`ContentNeedChip`, `_itemRow`).
- **Pros:** Minimal visual disruption; addresses the real gap (quantity is
  inconsistently shown) without forcing a row→chip redesign; smaller diff.
- **Cons:** "Reusing the same widgets" is satisfied for the *badge* but not for the
  *chip shell* — two chip layouts remain. Slightly weaker reading of the issue.
- **Effort:** Small–Medium.

#### Option C: Status quo + quantity only where chips already exist
- **Approach:** Only add `[n]X` to the chips that are already shared
  (`NeedClaimChip`/`GlassChip` already do `×N` → reformat to `[n]X`); leave the
  request `_itemRow` and `ContentNeedChip` as-is.
- **Pros:** Tiny diff.
- **Cons:** Fails the issue's core ask ("**wherever** a need or contribution chip
  is shown, reuse the same widgets" + quantity). Rejected.
- **Effort:** Small.

### Item 4 — cap

#### Option A: Confirm current cap satisfies the spec (recommended)
- **Approach:** The card already caps at 4 (< 5) with "+N more". Verify the cap
  math counts needs **and** contributions toward the same ceiling (it does — see
  `maxItems` allocation at [request_read_shell.dart:280-284](../../app/lib/presentation/screens/request/widgets/request_read_shell.dart#L280-L284)) and that "+N more" sums both overflow buckets. No code change unless a bug surfaces.
- **Effort:** Small (verification).

#### Option B: Make the ceiling an explicit shared constant
- **Approach:** If item 3 introduces a shared participation widget, hoist
  `maxItems` into it so request (and any future experience cap) share one constant.
- **Effort:** Small, only if Option A for item 3 lands.

## Recommendation

- **Item 1:** Verify only. Confirm a request/experience chat notification lands on
  the **expanded morph conversation panel** (not just the screen with a chat tab),
  on both a cold start (app terminated) and warm tap. Add a widget/unit test that a
  non-zero `initialTab` triggers `_expandConversation` if one doesn't already exist.
- **Item 2:** Verify only, with a regression test. Confirm the right-swipe pops both
  screens and is **not** swallowed by `ContentMorphPanel` (only active when a panel
  is open) or the experience carousel. If swallowed, scope the inner gesture or
  raise the screen-level handler — but only if verification fails.
- **Item 3:** **Option B** (shared quantity, audited call sites) as the floor, with
  **Option A**'s single-chip consolidation only for the *interactive* claim/bring
  surfaces where a chip shell already exists. Concretely: standardize the badge to
  `[n]X`, add `quantity` to `ContentNeedChip`, route quantity through the request
  participation rows, and confirm `NeedClaimChip` call sites. Do **not** force the
  request card's row layout into a chip if it harms density — but it must show
  `[n]X` when quantity > 1. This is the pragmatic reading: one badge, shared, shown
  everywhere; chip shells consolidated where they already are chips.
- **Item 4:** **Option A** — verify the existing 4-item cap and "+N more" satisfy the
  spec; no change unless the cap math or copy is wrong.

The honest headline: **this is ~1 substantive item (3) plus three verifications.**
Budget accordingly and do not over-build items 1, 2, 4.

## Open Questions

- **Badge format: `[n]X` vs the existing `×N`?** The issue literally writes
  `[n]X` (e.g. `2X`); `GlassChip` currently renders `×2`. — **Recommendation:**
  Treat `[n]X` as "2X" and change `GlassChip`'s badge to match so all surfaces are
  consistent, but flag it to design first since `×N` is the shipped convention and
  this touches every quantity badge (RSVP composer, request composer, roster). If
  design prefers `×N`, keep `×N` everywhere and consider the issue's `[n]X` as
  shorthand for "a quantity badge". Either way, **one** format across all chips.
- **Does "reuse the same widgets" require the request participation rows to become
  chips?** — **Recommendation:** No. The row layout (avatar + name + who) is a
  deliberate density choice for the collapsed card. Satisfy the issue by sharing the
  *quantity badge* and confirming the interactive chip surfaces use one chip widget;
  don't redesign the row into a chip. Confirm with the issue author if they meant a
  literal visual unification.
- **Where does the per-item quantity come from?** There is **no first-class
  `quantity` field** on the need/contribution protos — today quantity is derived at
  render time (group-by-title `.length`, or `need.slots`). — **Recommendation:**
  Keep deriving it the same way the composers already do (`NeedClaimChip`
  call sites); do **not** add a proto field for this issue. If a need's `slots`
  is the intended "quantity" for the participation card, use `slots`; clarify which
  number `[n]X` should show (slots requested vs. claims received) before wiring it.
- **Item 1 — should `ripls://conversation/<id>` be wired now?** It's in the Android
  manifest but unparsed by the router. — **Recommendation:** Out of scope for #2443
  (the `?tab=chat` path already opens the expanded view); leave it to its existing
  tracking work and only verify the notification→panel path here.
- **Item 4 — is "fewer than five" exactly 4, or up to 4?** Current cap is 4. —
  **Recommendation:** 4 satisfies "less than five"; keep it. Only change if the
  author wants exactly 4 needs **plus** 4 contributions, etc. (unlikely — the card
  must stay short).
- **Item 4 — apply the cap to the Experience card too (parity)?** The Experience
  collapsed participation card does not cap. — **Recommendation:** Yes, apply the
  same cap to `experience_read_shell.dart` in the same change to honor the
  `needs.md`/`planning.md` parity contract, hoisting the `maxItems` constant + the
  "+N more" row into a shared helper so both content views stay in lockstep. If the
  author insists request-only, add an *Intentional differences* entry to
  `docs/client/needs.md` documenting why. **Do not leave it silently divergent.**
- **Item 3 — is the `[n]X` badge a translatable string?** Today `GlassChip`
  hardcodes `×${quantity}` (no ARB key). — **Recommendation:** Keep the quantity
  badge as a numeric+symbol glyph (exempt from `lint:dart:i18n` the same way `×N`
  is today), but the **screen-reader semantics must be a localized phrase**, not the
  raw glyph — e.g. an l10n key `needsChipQuantitySemantics(count)` → "quantity 2".
  If any new *word* is introduced, it lands in both `app_en.arb` and `app_es.arb`
  in the same change (`lint:dart:l10n-parity`).

## Implementation Plan

### Phase 1: Verify items 1, 2, 4 (no/low code) — establish the real scope
**Goal:** Confirm which "verify" items actually pass, converting the issue from
"four changes" into "one change + N confirmed-OK".
**Tasks:**
- [ ] **Item 1:** Send/simulate a chat notification for a request and an experience
      (`?tab=chat` route) on cold-start and warm-tap; confirm the morph conversation
      panel opens over the hero (per [modals.md → Morph-reveal content panels](../client/modals.md)), not just the screen. Note pass/fail.
- [ ] **Item 2:** On both `requestScreen` and `experienceScreen`, right-swipe to
      close; confirm it pops. Repeat with a morph panel open (panel should close
      first) and, on experience, with the conversation carousel present. Note pass/fail.
- [ ] **Item 4:** Construct a request with > 4 needs+contributions; confirm the
      card shows ≤ 4 rows and a correct "+N more" count summing both buckets. Note pass/fail.
- [ ] **Item 4 parity decision:** check whether `experience_read_shell.dart` caps its
      participation card. If not, decide per the parity contract — apply the same cap
      (preferred) or record an *Intentional differences* entry in `docs/client/needs.md`.
- [ ] Write down which items need a fix (likely only those that fail) and which only
      need a regression test.
**Validates:** The true size of the change; prevents over-building already-working features.

### Phase 2: Item 3 — one shared quantity badge, shown everywhere
**Goal:** Every need/contribution chip reuses shared widgets and shows `[n]X` when
quantity > 1.
**Tasks:**
- [ ] Decide `[n]X` vs `×N` (Open Questions) and apply it in **one** place —
      `GlassChip`'s quantity badge ([glass_chip.dart:117-126](../../app/lib/presentation/widgets/modal/glass/glass_chip.dart#L117-L126)) — or a shared `QuantityBadge` helper if more than one shell needs it.
- [ ] Add an optional `quantity` to `ContentNeedChip` and render `[n]X` when > 1
      (dark pitching-in surface — [experience_pitching_in_screen.dart:191](../../app/lib/presentation/screens/experience/widgets/experience_pitching_in_screen.dart#L191)).
- [ ] Route quantity through the request participation rows
      (`_needRow` / `_bringingRow` in [request_read_shell.dart:389-434](../../app/lib/presentation/screens/request/widgets/request_read_shell.dart#L389-L434)) so a need with slots/claims > 1 or grouped contributions show `[n]X`.
- [ ] Audit **all** call sites (the four `NeedClaimChip` sites, `ContentNeedChip`,
      `ClaimedRow`, `_itemRow`) and confirm each uses the shared widget + badge; remove
      any one-off quantity rendering.
- [ ] **Parity (needs.md/planning.md):** confirm the badge renders identically in
      Experience and Request scopes — the chip is one of the shared `needs/` widgets,
      so any scope difference is a bug. No new *Intentional differences* entry should
      be needed for quantity.
- [ ] **Accessibility (`reviews/accessibility_review_prompt.md`, `client/accessibility.md`):**
      put the quantity in the **localized semantics label** (not just the `[n]X`
      glyph) so screen readers announce "quantity N", and keep selectable claim chips
      on `Toggle` (not `Tappable`) so the `selected` flag is exposed; display-only
      content-card rows may stay `Tappable`.
- [ ] **i18n (`client/i18n.md`):** any new user-visible *word* (e.g. a semantics
      phrase) lands in `app_en.arb` + `app_es.arb` together; the numeric badge glyph
      itself stays exempt like the existing `×N`. Run `npm run lint:dart:i18n` +
      `lint:dart:l10n-parity`.
- [ ] Unit/widget tests: badge hidden at quantity 1, shown as `[n]X` at quantity ≥ 2,
      and the semantics label carries the count, for `GlassChip`/`NeedClaimChip` and
      `ContentNeedChip`. Use `app/test/helpers/l10n_helpers.dart` for the widget tests.
**Validates:** Single source of truth for the chip + quantity; the issue's core ask, a11y + i18n + parity compliant.

### Phase 3: Fixes for any failed verification + regression tests
**Goal:** Repair only what Phase 1 found broken; lock it with tests.
**Tasks:**
- [ ] **If item 1 failed:** ensure `initialTab != 0` reliably opens the panel
      post-load on both content views; add a test asserting the panel route is pushed.
- [ ] **If item 2 failed:** scope/raise the conflicting inner horizontal-drag handler
      (`ContentMorphPanel` only-when-open, or experience carousel thresholds) so the
      screen-level right-swipe still pops; add a gesture regression test.
- [ ] **If item 4 failed:** correct the cap math / "+N more" sum; otherwise add a
      widget test pinning "≤ 4 shown, +N more = total − shown".
- [ ] If everything passed, add the regression tests anyway so these don't silently
      break in the next content-view refactor.
**Validates:** No open regressions; the three "verify" items are pinned by tests.

## Related Issues
- #2280 — pitching-in expand: introduced `ContentNeedChip` and the morph-reveal
  panels these items build on (referenced by `docs/client/modals.md`).
- #2291 — WHERE-card expansion: same morph-reveal surface family (context for item 1/2).
- #2058 — legacy URL-format deep-link support (adjacent to item 1's routing).

---

## Audit notes (ripls-audit)

**Scope:** client-only (`app/lib/presentation/**`, `app/lib/services/fcm_service.dart`,
`app/lib/core/router/**`). No proto/RPC, server, or dependency changes — `api_design`,
`security`, `observability`, and server dimensions are not engaged. Engaged dimensions:
**accessibility, code_quality, testing, tech_debt, i18n.**

**Findings applied to the plan:**
1. **Parity contract (needs.md/planning.md) — highest-impact.** Item 4 caps the
   request card but not the experience card; capping is not an *Intentional
   differences* entry, so it is a contract violation by default. Added a ⚠ callout,
   an Open Question, and a Phase 1 decision step to either apply the cap to both or
   document the divergence. Item 3 (chip) is a shared `needs/` widget, so its quantity
   behaviour must be scope-identical — added a parity check task.
2. **Accessibility (`reviews/accessibility_review_prompt.md`, `client/accessibility.md`).**
   The `[n]X` badge must be in the **localized semantics label**, not just the visual
   glyph; selectable claim chips stay on `Toggle` (exposes `selected`), display-only
   rows may stay `Tappable`. Added as a Phase 2 task + test.
3. **i18n (`client/i18n.md`).** New user-visible words land in both ARB locales
   (`lint:dart:l10n-parity`); the numeric badge glyph stays exempt like the shipped
   `×N`. The existing "+N more" already uses `requestMoreItems(count)` — keep that
   pattern. Added Phase 2 tasks + Open Question.
4. **Testing (`testing/architecture.md`, `client/testing.md`).** Widget tests for the
   chip must use `app/test/helpers/l10n_helpers.dart`; assert badge visibility
   thresholds and the semantics-label count. Already reflected in Phase 2/3.
5. **Tech_debt / code_quality.** Item 3 *reduces* debt by collapsing three chip/row
   renderings onto shared widgets — consistent with the consolidation the
   code_quality rubric favors; no new TODOs without `TODO(#NNNN)`.

**No blocking architecture conflicts** with the client MVVM/Riverpod conventions —
the plan keeps quantity derivation at the widget layer (no new proto field), matching
`docs/planning.md`'s "Multi-slot claim batching" deferral.

**Docs consulted:** `client/architecture.md` + `client/conventions.md` +
`proto_conventions.md` (anchors), `client/deep_linking.md`, `client/modals.md`,
`client/needs.md`, `planning.md`, `client/i18n.md`, `client/content.md`,
`notifications/push_notifications.md`; review rubrics: `reviews/accessibility_review_prompt.md`,
`reviews/code_quality_review_prompt.md`, `reviews/testing_review_prompt.md`,
`reviews/tech_debt_review_prompt.md`.
