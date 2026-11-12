# Issue #1965: Login and invite reminder screen look bad on dark mode

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1965
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

> **Audit pass — 2026-05-16:** Reviewed against `docs/client/architecture.md`
> ("Colors and Theming" section), `docs/client/i18n.md`,
> `docs/client/accessibility.md`, and `docs/client/testing.md`. Plan aligns
> with the documented theming guidance. Fixes applied: (a) build/lint
> commands corrected (`npm run build` runs the **Go server** build — for
> Flutter we want `npm run lint:dart`, `npm run lint:dart:colors`, and
> `npm run test:app`); (b) token mapping per role clarified using the
> architecture doc's quick-reference table (`cardBackground` for primary
> cards, `surface` for inset/chip-like callouts); (c) added a sub-step to
> fix the adjacent hardcoded `'Back to Login'` string per
> `docs/client/i18n.md` ("Every user-visible string should use
> `context.l10n.*`"); (d) added an explicit WCAG-contrast spot-check in QA;
> (e) documented why no unit test ships with the change (no ViewModel
> touched). See **Audit deltas** section at the bottom for the full
> change list.

## Problem Statement

Several card-style containers on the unauthenticated auth flows render bright,
hardcoded light-mode backgrounds on dark mode and use Material `Colors.blue.*`
instead of the brand's sage-green accent. In dark mode the cards look like
brightly-lit panels punched out of the dark warm-brown app background, with
poor visual hierarchy and an off-brand blue palette.

The reporter calls out two surfaces: **the invite-reminder screen** (no
short-code path) and **the plain login screen**. Both share the same root
cause — `Colors.blue.shade50` / `Colors.grey.shade50` / `Colors.grey.shade100`
container backgrounds, blue icon/text/button styling, and no `Theme.of`
adaptation — even though the codebase already exposes a complete theme-aware
palette in `app/lib/core/theme/app_colors.dart` (Heritage Sage primary, warm
cream/brown surfaces in light, warm dark brown in dark).

The reporter mentions a "new green accent color" — there is no *new* token;
the brand accent is `AppColors.lightPrimary` / `darkPrimary` (Heritage Sage,
#5B8268 / #7BA088). Recent transfer-modal work (#1946) already migrated other
surfaces from coral to sage, so the auth-screen blue is a holdover this issue
asks us to retire.

## Context from Comments

The issue has no comments — only the reporter's bug description, screenshot,
and device context (Android 16, Pixel 9 Pro, app v0.21.0, PROD). Worth
flagging: the reporter is the project owner (`user2@example.org`), so the
intent is clear enough that we don't need to round-trip for clarification
before planning.

The screenshot shows the **invite-reminder screen** specifically — the path
hit when a user lands on `/register` without a short code. The card stack:

1. A pale-blue card with an envelope icon, "Ripls is invitation-only…" body
   copy, and a blue-bordered "Back to Login" outlined button.
2. A near-white card with "Have an invite code?" header, dark input field
   (only this one is themed), and a washed-out gray "Apply Code" button.

The user's other call-out — "similar boxes on the plain login screen" — points
at the "New to Ripls?" / `loginInvitationRequired` panel at the bottom of
`LoginScreen`, which uses `Colors.grey.shade100` with shade-300 borders and
shade-700/shade-900 text.

## Codebase Findings

Hardcoded Material-palette colors on the affected surfaces:

**`app/lib/presentation/screens/auth/register_screen.dart`**
- `_buildInvitationStatus()` (lines 306–329) — "Verifying invitation…" spinner
  card. `Colors.blue.shade50` fill, `Colors.blue.shade200` border. Visible only
  during the in-flight RPC, but same root cause.
- `_buildInviteCodeEntry()` (lines 704–773) — "Have an invite code?" card.
  `Colors.grey.shade50` fill, `Colors.grey.shade200` border,
  `Colors.grey.shade800` / `.shade600` heading + help text, and a button
  with hardcoded `Colors.grey.shade300/500` disabled colors.
- `_buildInvitationRequiredMessage()` (lines 868–901) — main "Ripls is
  invitation-only" card. `Colors.blue.shade50` fill, `Colors.blue.shade200`
  border, `Colors.blue.shade700/800` for icon + text + outlined button.

**`app/lib/presentation/screens/auth/login_screen.dart`**
- `_buildNewUserMessage()` (lines 404–451) — "New to Ripls? Invitation
  required" panel. `Colors.grey.shade100` fill, `Colors.grey.shade300` border,
  `Colors.grey.shade700/900` icon + text. The "Contact Support" link already
  uses `Theme.of(context).colorScheme.primary` (correctly themed) — that's
  our model for the rest.

**`app/lib/core/theme/app_colors.dart`** already exposes the right tokens.
None of the hardcoded surfaces would need new color constants:
- `AppColors.cardBackground(context)` — white in light (#FFFFFF), warm dark
  in dark (#2D2420). **Use for primary cards** (the dominant container on
  the screen).
- `AppColors.surface(context)` — slightly tinted (light beige #F8F3EE / dark
  warm #3A2F2A). **Use for inset/secondary panels** — spinner chips, inline
  callouts, chips on an already-card-backed surface.
- `AppColors.border(context)` — light beige / dark warm brown.
- `AppColors.textPrimary` / `textSecondary` / `textTertiary` — themed.
- `AppColors.primary(context)` — Heritage Sage (the "green accent").
- `Theme.of(context).colorScheme.primary` — same value; preferred when we're
  already in a Material widget. Existing code in `_buildNewUserMessage`'s
  Contact Support link already uses this pattern.

**Architecture-doc alignment** (`docs/client/architecture.md` §"Colors and
Theming"):

> ❌ DON'T use hardcoded `Colors.grey[50]`, `Colors.grey[100]`,
> `Colors.grey[200]`, `Colors.grey[300]` for backgrounds or borders —
> these are light-only colors. Use `AppColors.surface(context)` or
> `AppColors.border(context)` instead.

The four `_build*` methods are textbook violations of that rule. The quick-
reference table in the same doc is what the role mapping below uses.

**Lint posture.** `npm run lint:dart:colors`
(`scripts/check_no_hardcoded_accent.js`) only flags Material orange/amber
and specific accent hex literals (post-#1946 sage palette). It does **not**
flag `Colors.blue.*` / `Colors.grey.*`, so this fix is doc-driven, not
lint-driven — but doc compliance is the bar per the audit.

No other auth-flow file (`phone_auth_screen.dart`, `forgot_password_screen.dart`,
`signup_screen.dart`, etc.) showed up in a grep for `Colors.blue.shade` /
`Colors.grey.shade` — scope is genuinely the two screens the reporter named.

There is **no dark-mode visual test infrastructure** in `app/test/` for the
auth screens (a grep for `MediaQuery.*Brightness.dark` in tests returns no
results inside the auth feature). This is consistent with how dark-mode bugs
keep slipping in — see "Related work" below.

## Potential Solutions

### Option A: Replace hardcoded colors with existing `AppColors` getters in place

- **Approach:** For each of the four `_build*` methods, swap
  `Colors.blue.shade*` / `Colors.grey.shade*` for the theme-aware equivalents
  (`AppColors.cardBackground(context)`, `AppColors.border(context)`,
  `AppColors.textPrimary/Secondary(context)`, and
  `Theme.of(context).colorScheme.primary` for the brand-accent role currently
  filled by blue). Leave the structural layout (Container/Column/Icon/Text
  shapes, paddings, sizes) untouched.
- **Pros:** Minimal blast radius — four self-contained widget methods. Uses
  tokens that already exist, are documented, and are used elsewhere in the
  app. Easy to review (a tight diff). No new public API.
- **Cons:** Each method still owns its own Container + BoxDecoration; if the
  visual treatment drifts again in the future (a fifth auth surface lands
  with a copy-pasted Container), we'll be back here.
- **Effort:** Small (1–2h including manual dark-mode QA on both surfaces).

### Option B: Extract a shared `AuthInfoCard` widget and reuse across both screens

- **Approach:** Build a small `AuthInfoCard({icon, title, body, action})`
  widget that bakes in the themed background/border/text colors, then use it
  in `_buildInvitationStatus`, `_buildInviteCodeEntry`,
  `_buildInvitationRequiredMessage`, and `_buildNewUserMessage`. Two of those
  hold a form (input + submit) or a spinner, so the widget would need a
  `child` / `trailing` slot or two specialisations.
- **Pros:** Single place to tune visual treatment of auth-screen cards;
  guarantees no future drift. Tests can target one widget.
- **Cons:** Three of the four cards have meaningfully different content
  shapes (spinner card, form card, message+button card, message+linkbutton
  card). Forcing one component over all four invites either a fat parameter
  list or a heap of named constructors. **Speculative abstraction** — we
  don't have a fifth caller on the whiteboard, and CLAUDE.md is explicit that
  three similar lines beat a premature abstraction.
- **Effort:** Medium (3–5h including widget tests for the new component).

### Option C: Lift to `ThemeData.extensions` — define `AuthCardTheme`

- **Approach:** Define a `ThemeExtension<AuthCardTheme>` that holds the four
  values (bg, border, primaryText, secondaryText) and register it on
  `appTheme()` + `darkTheme()`. Each `_build*` method reads via
  `Theme.of(context).extension<AuthCardTheme>()`.
- **Pros:** Most "correct" Flutter idiom for theme-scoped tokens; surfaces a
  semantic role ("auth card") instead of leaning on the generic AppColors
  helpers.
- **Cons:** Heaviest change for zero behavioural difference vs. Option A.
  The codebase already has the same pattern via `AppColors.cardBackground` —
  adding a parallel `ThemeExtension` system for one screen would fragment
  the styling story. CLAUDE.md: avoid abstractions beyond what the task
  requires.
- **Effort:** Medium-large.

## Recommendation

**Option A — replace hardcoded colors with existing `AppColors` getters in
place.** The job is purely a colour swap on four self-contained widget
builder methods. The tokens we need are already defined and already used by
neighbouring screens (the Contact Support link in `LoginScreen` even points
the way: `Theme.of(context).colorScheme.primary`). Anything more is
speculative — we don't have a third or fourth caller for a shared widget,
and a `ThemeExtension` for one screen is a wrong-size hammer.

The brand-accent role (currently blue) should land on
`Theme.of(context).colorScheme.primary` rather than `AppColors.primary(context)`
when the surrounding code is already pulling other things off `Theme.of`,
because Material widgets pick up that channel anyway and it keeps the call
site self-documenting. Both resolve to the same Heritage Sage in the current
theme.

## Open Questions

- **The pre-form "Verifying invitation…" spinner card (`_buildInvitationStatus`)
  in `register_screen.dart` — same fix, or out of scope?** —
  **Recommendation: include it.** It's the same exact pattern
  (`Colors.blue.shade50` / `.shade200` container) on the same screen and a
  user *can* see it briefly during a slow network round-trip. Skipping it
  leaves a known-bad surface a couple of seconds away from the one we're
  fixing.

- **The "Have an invite code?" `Apply Code` button uses hardcoded
  `disabledBackgroundColor: Colors.grey.shade300` /
  `disabledForegroundColor: Colors.grey.shade500`. Theme it too, or leave?**
  — **Recommendation: drop the explicit overrides.** Flutter's default
  `ElevatedButton` disabled treatment already adapts to the active
  `ColorScheme`, so removing the override gives correct dark-mode behaviour
  for free. Less code, less to maintain.

- **Should the "Ripls is invitation-only" card's `OutlinedButton.icon`
  ("Back to Login") keep the explicit `foregroundColor` / `side` override?**
  — **Recommendation: drop it.** With the surrounding card themed, the
  default outlined-button style picks up `colorScheme.primary` for both
  foreground and outline and renders the sage accent correctly in both modes.

- **Do we want a visual/golden test for these screens in dark mode?** —
  **Recommendation: defer to a follow-up if at all.** The existing test
  surface doesn't include auth-flow goldens. Adding one only here would be
  an isolated piece of golden infrastructure with no other callers. Manual
  QA in both modes is sufficient for the fix; a broader golden-test rollout
  belongs to its own issue.

- **Is there a "new green accent" token I missed (the reporter's words)?** —
  **Recommendation: no, treat `lightPrimary`/`darkPrimary` (Heritage Sage)
  as the brand accent.** `app/lib/core/theme/app_colors.dart` is explicit
  about this — the recent #1946 work documented the coral→sage migration,
  and there is no other "green" constant. If the reporter has a specific
  swatch in mind that isn't in `AppColors`, that's a separate design ticket.

## Implementation Plan

### Phase 1: Fix the auth surfaces named in the issue
**Goal:** All four cards on `LoginScreen` and `RegisterScreen` render
correctly themed in both light and dark mode, using sage green (not blue)
for accent roles.

**Token role map** (per `docs/client/architecture.md` quick-reference table):

| Surface | Token |
|---------|-------|
| Spinner callout inside register form (`_buildInvitationStatus`) | `AppColors.surface(context)` (inset) |
| "Have an invite code?" form card (`_buildInviteCodeEntry`) | `AppColors.cardBackground(context)` (primary card with form) |
| "Ripls is invitation-only" main card (`_buildInvitationRequiredMessage`) | `AppColors.cardBackground(context)` (primary card on screen) |
| "New to Ripls? / Invitation required" panel on login (`_buildNewUserMessage`) | `AppColors.surface(context)` (secondary inset on a content-rich page) |

**Tasks:**
- [ ] In `register_screen.dart` `_buildInvitationStatus` (≈L306–329): swap
      `Colors.blue.shade50` → `AppColors.surface(context)` and
      `Colors.blue.shade200` → `AppColors.border(context)`.
- [ ] In `register_screen.dart` `_buildInviteCodeEntry` (≈L704–773): swap
      `Colors.grey.shade50` → `AppColors.cardBackground(context)`,
      `Colors.grey.shade200` → `AppColors.border(context)`,
      `Colors.grey.shade800/600` → `AppColors.textPrimary/textSecondary(context)`.
      Drop the `disabledBackgroundColor` / `disabledForegroundColor`
      overrides on the `ElevatedButton.styleFrom` so Material picks up
      the theme.
- [ ] In `register_screen.dart` `_buildInvitationRequiredMessage` (≈L868–901):
      swap `Colors.blue.shade50` → `AppColors.cardBackground(context)`,
      `Colors.blue.shade200` → `AppColors.border(context)`,
      `Colors.blue.shade700` on the icon → `Theme.of(context).colorScheme.primary`,
      `Colors.blue.shade800` on the body text → `AppColors.textPrimary(context)`.
      Drop the `OutlinedButton.icon`'s explicit `foregroundColor` and
      `side` so it picks up the themed `OutlinedButtonTheme`. **Also
      replace the hardcoded `const Text('Back to Login')` with a new
      `context.l10n.commonBackToLogin` key** (or reuse `loginScreenTitle`
      if appropriate after review) — see Adjacent i18n fix below.
- [ ] In `login_screen.dart` `_buildNewUserMessage` (≈L404–451): swap
      `Colors.grey.shade100` → `AppColors.surface(context)`,
      `Colors.grey.shade300` → `AppColors.border(context)`,
      `Colors.grey.shade700` on the icon and secondary body →
      `AppColors.textSecondary(context)`,
      `Colors.grey.shade900` on the heading → `AppColors.textPrimary(context)`.
      The "Contact Support" link already uses `colorScheme.primary` and
      stays unchanged.
- [ ] Add `import 'package:ripls/core/theme/app_colors.dart';` to both
      files if it isn't already present.

**Adjacent i18n fix (in-scope, same widget):**
- [ ] Add `commonBackToLogin` to `app/lib/l10n/app_en.arb` **and**
      `app_es.arb`, with `@commonBackToLogin` metadata describing the
      placement.
- [ ] Run `npm run generate` so `AppLocalizations` regenerates.
- [ ] Use `context.l10n.commonBackToLogin` in
      `_buildInvitationRequiredMessage`.
- [ ] Note: the existing `'I already have an account'` hardcoded string
      at `register_screen.dart:783` is **out of scope** — different
      widget, different concern. It's noted here only to acknowledge it
      and avoid scope creep.

**Build verification (corrected from prior draft):**
- [ ] `npm run lint:dart` → `cd app && flutter analyze`. Must pass with
      no new warnings.
- [ ] `npm run lint:dart:colors` → still passes (not exercised by the
      new code, but confirms no regression).
- [ ] `npm run lint:dart:i18n` → still passes (the existing narrow
      lint shouldn't change behaviour; the new ARB key gives us
      defence in depth if the lint expands later).
- [ ] `npm run test:app` → `cd app && flutter test --reporter expanded`.
      Existing tests must pass; we are not adding new tests this phase
      (see "No-test rationale" below).

**No-test rationale:** Per `docs/client/architecture.md` §Testing, the
unit-test target is *ViewModels*, tested via `ProviderContainer` with
fake repositories. This fix touches **only** widget builder methods
inside `StatefulWidget`s — there is no Notifier, no Provider, no
business logic change. The right test surface for a colour swap is a
golden test, which the codebase doesn't yet have for auth flows.
Adding a one-off golden test here would be isolated infrastructure
with no other callers; the QA-via-emulator step in Phase 2 is the
appropriate validation for this change. A broader golden-test rollout
belongs to its own issue.

**Validates:** That the existing theme-aware tokens are sufficient — i.e.
we don't actually need to extend `AppColors` for this fix.

### Phase 2: Manual QA in both modes on both platforms
**Goal:** Prove the visual fix before shipping.

**Tasks:**
- [ ] Launch the app via the `ripls-app-logs` skill on the Android emulator;
      navigate to `/login` (sign out if needed), screenshot in light and
      dark mode.
- [ ] Navigate to `/register` (with no short code) → invite-reminder card;
      screenshot in light and dark mode.
- [ ] Navigate to `/register?code=invalid` to force a verification fetch and
      visually verify the spinner card in both modes (or just briefly during
      the loading state).
- [ ] Repeat on iOS simulator.
- [ ] Confirm: no blue anywhere on these surfaces, every text/border legible
      in dark mode, "Back to Login" outlined button reads as sage-on-dark,
      "Apply Code" disabled state legible in both modes.
- [ ] **WCAG-contrast spot-check:** confirm `colorScheme.primary` text and
      outline read clearly against `AppColors.cardBackground(context)` and
      `AppColors.surface(context)` in **both** modes. Reference numbers
      live in `app_colors.dart` lines 688–697 (the modal-button contrast
      math from #1934) — we expect comparable contrast here because the
      sage→cardBackground pairing matches the documented pattern. If the
      outlined button looks washed out on dark, fall back to
      `AppColors.primary(context)` (Heritage Sage, the explicit
      non-Material reference) and verify it picks the higher-contrast
      sage variant.

### Phase 3: Commit + PR
**Goal:** Ship.

**Tasks:**
- [ ] Commit on `dark_mode` branch with a message that names the issue:
      `Fix #1965: theme login + invite-reminder cards for dark mode`.
- [ ] Open a PR with before/after screenshots (light + dark, both screens).
- [ ] Tag the PR `closes #1965`.

**Validates:** Reviewable, ships through normal PR flow per the
`feedback_pr_workflow_only` memory.

## Related Issues / Work

- **#1946** — Coral → Sage palette migration. The blue we're removing is the
  last holdout outside that migration's scope. Worth referencing in the
  PR description so reviewers see the continuity.
- **#1934** — Contrast rationale for accent fills (the
  `accentButtonBackground` neutral). Same family of theming concerns; no
  direct overlap.
- **Branch context:** `dark_mode` branch is currently even with `main` —
  it's a clean starting point for this fix.

---

## Audit deltas (2026-05-16)

What the audit pass changed against the initial draft, with the reference
doc that drove each change:

1. **Build command corrected** — `npm run build` runs the **Go server**
   build, not the app. For Flutter, the right invocations are
   `npm run lint:dart`, `npm run lint:dart:colors`, and
   `npm run test:app`. Updated Phase 1's build-verification block.
   *(Verified via `package.json` scripts.)*

2. **Token role mapping clarified** — initial draft defaulted every
   surface to `AppColors.surface(context)`. The architecture doc's
   quick-reference table separates "Card / modal background" from
   "Surface (input, chip)". Added the per-surface role map: primary
   cards (`_buildInviteCodeEntry`, `_buildInvitationRequiredMessage`)
   now use `cardBackground`, while inset/secondary callouts
   (`_buildInvitationStatus`, `_buildNewUserMessage`) use `surface`.
   *(Driven by `docs/client/architecture.md` §"Colors and Theming",
   lines 311–322.)*

3. **Adjacent i18n fix folded in** — `_buildInvitationRequiredMessage`
   contains a hardcoded `const Text('Back to Login')` (line 892). The
   narrow `lint:dart:i18n` check doesn't flag it (its trigger list is
   limited to common UI words like Cancel/Save/Close), but
   `docs/client/i18n.md` is explicit that "Every user-visible string
   should use `context.l10n.*`." Added a sub-bullet to introduce
   `commonBackToLogin` in `app_en.arb` + `app_es.arb` and `npm run
   generate`. The neighbouring `'I already have an account'` string at
   line 783 is **explicitly out of scope** — different widget, would
   inflate the PR for no functional reason.

4. **WCAG-contrast spot-check added to QA** — `app_colors.dart` lines
   688–697 document the contrast math behind the sage primary fill
   (#5B8268 white-on-sage ≈ 4.4:1, passing 4.5:1 normal-text). The
   plan now calls out comparable verification for the outlined button
   on dark backgrounds, with a fallback to `AppColors.primary(context)`
   if `colorScheme.primary` picks the lighter sage that may not clear
   contrast. *(Driven by `docs/client/accessibility.md` and #1934
   rationale embedded in `app_colors.dart`.)*

5. **No-test rationale documented** — `docs/client/architecture.md`
   §Testing prescribes ViewModel-first testing via
   `ProviderContainer`. This change touches zero Notifiers, so there's
   nothing to ViewModel-test. The architecture doc also lists golden
   tests as a future-improvement under "Next Steps" (#6); adding one
   here would be isolated infrastructure. Explicitly recorded the
   "manual QA is the right validation" reasoning so a reviewer doesn't
   ask for missing tests.

6. **Caching / server / observability docs reviewed and not relevant**
   — `docs/client/caching.md`, `docs/server/architecture.md`,
   `docs/server/observability.md` were on the audit checklist. This
   fix is a pure UI-styling change with no repository, service, RPC,
   or logging surface, so nothing applies. Recorded here so future
   reviewers can see they were considered.

7. **Existing patterns reused** — the audit confirmed
   `_buildNewUserMessage`'s Contact Support link already uses
   `Theme.of(context).colorScheme.primary` correctly. The fix
   extends that pattern to the rest of the cards rather than
   introducing a new convention. Mentioned explicitly in Phase 1's
   `_buildNewUserMessage` task.
