# Issue #1972: Untranslated strings keep creeping into app

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1972
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`flutter gen-l10n` prints `"es": 14 untranslated message(s)` on every build but
nothing fails. New keys added to `app_en.arb` without a matching `app_es.arb`
entry slip through code review, type-check, and CI. We need a build/CI gate
that prevents the gap from growing.

The README in `app/lib/l10n/README.md` already documents the intended policy:
*"Add the same key with the translated value to every other locale file. Use
English as a placeholder if the translation is not yet available."* The
project also (incorrectly) claims `nullable-getter: false` *"catches missing
keys at compile time"* — in practice that flag controls only the Dart getter's
nullability; Flutter silently falls back to the template locale when a key is
absent from a non-template ARB and prints a warning that the build does not
honor.

Today the gap is 14 keys, all added in the recent `workshop_2` PR
(`workshopActsCategory*`, `workshopActsDetail*`). Manually reproducing the
delta:

```
en total: 1801, es total: 1787, missing in es: 14
```

There are no comments on the issue, but the fix is well-scoped — the existing
codebase already has a robust pattern for ratchet/parity lints (see
`lint:dart:a11y`, `lint:dart:colors`, `lint:dart:ref-listen-in-build`,
`lint:dart:route-names`).

## Context from Comments

No comments on the issue. The author (project owner) filed it as a one-shot
problem report with no discussion. That is fine — the symptom and desired
outcome are unambiguous. No clarification needed.

## Potential Solutions

### Option A: Hard key-parity gate (recommended)

- **Approach:** Add `scripts/check_l10n_parity.js` that loads every ARB file
  under `app/lib/l10n/`, treats `app_en.arb` as the source of truth, and
  fails if any non-template ARB is missing or has extra non-`@`-prefixed
  keys. Wire it into a new `lint:dart:l10n-parity` npm script and add a
  step to `.github/workflows/test_flutter.yaml`. As a one-time cleanup,
  copy the 14 missing English values into `app_es.arb` (English placeholders
  are explicit project policy per the l10n README).
- **Pros:** Matches the documented policy ("use English as a placeholder").
  Zero allowlist, zero ratchet bookkeeping. Catches drift in either
  direction (missing *and* stale keys). Cheap to run (two JSON loads + a
  set diff). Aligns with the existing `check_*.js` pattern.
- **Cons:** Forces every PR that adds an English string to also touch
  `app_es.arb` (acceptable — that *is* the policy). Doesn't distinguish
  between "translated" and "English placeholder," so future translation
  work can't be tracked from this check alone.
- **Effort:** Small (~1 hour).

### Option B: Ratchet allowlist (untranslated count may shrink but not grow)

- **Approach:** Set `untranslated-messages-file: untranslated.txt` in
  `app/l10n.yaml`, run `flutter gen-l10n` in CI, and use the same
  ratchet pattern as `a11y_allowlist.txt` etc.: a tracked file
  `scripts/untranslated_allowlist.txt` lists the 14 current missing keys;
  CI fails if `untranslated.txt` contains any key not in the allowlist, and
  a second step fails if the allowlist itself grew vs the base branch.
- **Pros:** Lets a maintainer ship a feature in English-only and defer the
  Spanish translation deliberately. Matches the established ratchet
  pattern (a11y, colors, ref-listen, route-names) exactly.
- **Cons:** Contradicts the README policy that already says "Use English as
  a placeholder." Two files of bookkeeping (the allowlist + the report)
  for a problem we currently solve with `nullable-getter: false` at the
  *parity* level. Encourages the "I'll translate later" pattern that
  caused the issue in the first place.
- **Effort:** Small/medium (~2 hours).

### Option C: Set Flutter's own `untranslated-messages-file` and fail the build

- **Approach:** Add `untranslated-messages-file` to `l10n.yaml` and add a
  post-`flutter gen-l10n` CI step that fails if that file is non-empty,
  with no allowlist support.
- **Pros:** Uses Flutter's own knowledge of what counts as untranslated —
  no parallel implementation. Behaviorally identical to Option A for the
  current single-locale-pair setup.
- **Cons:** Relies on a generated artifact in CI rather than checking a
  static input (`*.arb`). Awkward when adding a third locale because
  `gen-l10n` writes one file per locale to the same path. Conflates the
  generate step with the lint step, making local dev cycles slightly
  noisier. The script we'd write is barely simpler than Option A.
- **Effort:** Small (~1 hour).

## Recommendation

**Option A.** The project policy (per `app/lib/l10n/README.md`) is already
"every key, every locale, English as placeholder when needed." The 14 current
missing keys are out-of-policy, not deliberate deferrals — they slipped in
because nothing enforced the rule. A simple parity gate enforces what the
README says and matches the existing `scripts/check_*.js` pattern (small Node
script, wired through one `npm run lint:dart:*` line, one CI step, no
allowlist plumbing).

The one tradeoff worth naming: Option A loses the ability to track
"translation in progress" — every English-placeholder entry is invisible
to the check. That is acceptable for the current bilingual (en/es) setup;
when we onboard a real translation workflow (third locale, vendor handoff,
etc.) we can layer on a real translation-status tool then. We should not
build that infrastructure now to solve a problem we don't have.

## Open Questions

- **Should English placeholders be allowed long-term, or do we want a
  separate signal that says "this Spanish string is actually identical to
  English on purpose"?** — Recommendation: allow English placeholders with
  no extra signal for now. The cost of a richer tracking scheme (custom
  `@@untranslated_keys` block, sidecar JSON, etc.) is not worth it for a
  one-developer team that hand-edits ARB files. Revisit when a real
  translator joins the workflow.
- **Should the gate also check that `@key` metadata blocks (descriptions,
  placeholder definitions) are present in non-template ARBs?** —
  Recommendation: no. Flutter's tooling only requires `@key` metadata in
  the template file; non-template ARBs traditionally omit it. Checking
  only non-`@`-prefixed keys keeps the script simple and matches Flutter's
  own model.
- **Where does the cleanup PR for the 14 keys live — in this PR or a
  separate one?** — Recommendation: same PR. The keys are all `workshop*`,
  small, and gating CI without cleaning them up would land the build red.
  Splitting buys nothing.
- **Should the script also fail on *extra* keys in `app_es.arb` (i.e.,
  keys in Spanish that aren't in English)?** — Recommendation: yes. A
  Spanish-only key is a code smell — either a typo or a dead key after a
  rename. The script should flag both directions, and the cost is one
  extra set-diff line.

## Implementation Plan

### Phase 1: Add the parity script and CI gate — IMPLEMENTED

**Goal:** A failing CI step exists that catches `app_en.arb` ↔ `app_es.arb`
key drift, and the current 14-key gap is fixed.

**Tasks:**
- [x] Add `scripts/check_l10n_parity.js`:
  - Loads every `app/lib/l10n/app_*.arb` file.
  - Treats `app_en.arb` as template (matches `l10n.yaml`).
  - Computes `template_keys - locale_keys` and `locale_keys - template_keys`
    over keys that do not start with `@`.
  - Prints both diffs and exits non-zero if either is non-empty.
  - Mirrors the style of `scripts/check_i18n.js` (CommonJS, no deps,
    short success message).
- [x] Add `"lint:dart:l10n-parity": "node scripts/check_l10n_parity.js"`
  to `package.json` and append it to the aggregate `lint` script.
- [x] Add a CI step in `.github/workflows/test_flutter.yaml`
  ("Check l10n key parity (issue #1972)") that runs the new lint.
- [x] Add the script path to the `paths:` filters in the same workflow
  so changes to the script also trigger the workflow.
- [x] Add real Spanish translations for the 14 missing keys in
  `app_es.arb` (chosen over English placeholders because the strings
  are short and `impactPillActions: "ACCIONES"` already establishes
  `Acts → Acciones`). Keys: `workshopActsCategoryEventsCreated`,
  `workshopActsCategoryHelpOffered`, `workshopActsCategoryHelpRequested`,
  `workshopActsCategoryItemsShared`, `workshopActsCategoryLoansGiveaways`,
  `workshopActsCategoryPitchedIn`, `workshopActsCategoryRsvps`,
  `workshopActsDetailBreakdownColCategory`,
  `workshopActsDetailBreakdownColCount`,
  `workshopActsDetailBreakdownHeader`, `workshopActsDetailChartHeader`,
  `workshopActsDetailHeroSubtitle`,
  `workshopActsDetailHeroSubtitleNoAnchor`, `workshopActsDetailTitle`).
- [x] Run `npm run lint:dart:l10n-parity` locally to confirm green,
  and verify it fails on a synthetic missing key.

**Validates:** A correctly-formed ARB pair passes the gate, and the gate
fires on a synthetic missing key (verified by temporarily deleting one
ES key and re-running — script exited 1 with a clear violation message).

### Phase 2: Update documentation — IMPLEMENTED

**Goal:** The l10n README accurately describes what the gate does and
how to add a new string in a way that passes CI.

**Tasks:**
- [x] Replace the misleading line in `app/lib/l10n/README.md` claiming
  `nullable-getter: false` catches missing keys at compile time
  (it does not — it controls return-type nullability). Replaced with
  a sentence pointing to `npm run lint:dart:l10n-parity`.

### Phase 3 (optional, defer): Per-locale translation-status tracking

**Goal:** Distinguish "English placeholder pending translation" from
"intentionally identical Spanish."

Skip until we onboard a third locale or a real translation workflow.
Capture the design when it becomes load-bearing — not before.

## Related Issues

- None directly referenced. The pattern this plan adopts is established
  by the `a11y`, `color`, `ref_listen_in_build`, and `route_names` lint
  ratchets in `.github/workflows/test_flutter.yaml`; this gate is simpler
  because the policy (parity, not "may shrink") doesn't need an allowlist.

## Audit findings (post-implementation)

Audit against `docs/client/i18n.md`, `docs/client/accessibility.md`, and
the existing lint-ratchet patterns surfaced three meaningful items. All
fixed in this PR:

1. **`docs/client/i18n.md` contained the same false claim about
   `nullable-getter: false` that the l10n README did.** Both said the
   flag "catches missing keys at compile time." It does not — it only
   controls the return-type nullability of generated getters. Both
   files were updated to attribute missing-key enforcement to the new
   parity gate, and `docs/client/i18n.md` now lists
   `scripts/check_l10n_parity.js` in its **Related** section alongside
   `check_i18n.js`.
2. **The parity check is key-based; placeholder parity for ICU strings
   is enforced by `flutter gen-l10n` itself.** A Spanish string
   declared without a placeholder that the English template requires
   (or vice-versa) fails codegen with a clear error, so we did not need
   to extend the parity script to also walk `@key` metadata. This is
   documented above in the Open Questions section's answer about `@key`
   metadata.
3. **The parity gate runs against ARB JSON files directly — no Flutter
   toolchain needed.** This means the check is fast (single Node
   invocation, two `JSON.parse` calls) and is unaffected by the
   `flutter: generate: true` pubspec setting needed for `flutter
   gen-l10n` to run locally. Useful as a fast pre-commit signal even
   in environments where the Flutter SDK isn't fully configured.

The architectural fit was good: the plan correctly identified that the
parity policy (every key in every locale, English placeholders allowed)
fits the project's pre-existing README guidance and doesn't need the
allowlist-ratchet machinery used by the a11y/colors/ref-listen/route-names
lints. No revisions to the implementation approach were required.

A pre-existing gap not addressed in this PR (out of scope, captured here
for future work): the `workshop*` key prefix is now the largest single
namespace in the ARB files (~180 keys in `app_en.arb`) but is not
listed in the **Key Naming Convention** table in `docs/client/i18n.md`.
A follow-up PR should add it.
