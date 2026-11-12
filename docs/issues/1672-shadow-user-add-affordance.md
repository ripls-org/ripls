# Issue #1672: Community people list references non-existent "add someone new"

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1672
**Filed:** 2026-05-02 | **Last activity:** 2026-05-04
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

> **Audit notes (2026-05-04):** Reviewed against `docs/client/architecture.md`,
> `docs/client/caching.md`, `docs/client/i18n.md`, `docs/client/accessibility.md`,
> and `docs/client/testing.md`. The plan complies with the MVVM/Riverpod rules
> in `architecture.md` — no new screen→repository access, view-model state
> changes are stripped down rather than expanded, and the `UserError? error`
> shape is preserved (per `i18n.md` §"Best Practices", error variants stay
> typed in the view-model). Both `app_en.arb` and `app_es.arb` are updated in
> the same phase (`i18n.md` §"Each new string needs both locales"); `npm run
> generate` is the canonical regen entry point. The `manageMembersEmptyShadow`
> empty-state is non-interactive `Text(...)`, so `accessibility.md`'s
> `Tappable`/`IconAction` primitives don't apply — the only a11y obligation is
> sourcing the string from `context.l10n.*`, which the existing screen already
> does. Per `testing.md`'s view-model-first guidance, a widget test for the new
> copy is unnecessary; the existing view-model test file
> (`manage_members_view_model_test.dart`) is the only test surface that
> changes. Confirmed `RpcErrorHandler` stays imported because
> `_loadMembers`/`_loadShadowUsers` still use `RpcErrorHandler.classify`. Plan
> updated below to call out these compliance points explicitly.

## Problem Statement

On the Manage Members screen, the **People** section (shadow users — participants
who don't have the app yet) shows an empty-state hint:

> "No one added yet. Tap **Add someone new** to reference a participant who
> doesn't have the app."

The button this hint refers to does not exist on the screen. The only
floating action button is labeled **"Invite Someone"**, which opens the
community-wide `InviteSheet` (a share-link / QR flow) — it does **not**
create a shadow user. The result is that a user reads the hint, scans
the screen for the named button, and never finds it.

This is a regression introduced in commit `4c95171fa` ("Add manage
members support", 2026-04-05). That commit replaced the original
`_buildAddButton` (a FAB labeled `manageMembersAddPersonButton` →
"Add someone new" that opened a name-entry dialog) with the new
`_buildInviteButton` (labeled "Invite Someone" → opens
`InviteSheet`). The empty-state copy `manageMembersEmptyShadow`
was not updated to match, so it now points at a button that has
been gone for a month.

The view-model side of the original flow is still wired up:
`ManageMembersNotifier.createShadowUser()` exists at
`app/lib/presentation/viewmodels/manage_members_view_model.dart:88-112`,
along with localized strings `manageMembersAddPersonButton`,
`manageMembersAddPersonTitle`, and `manageMembersAddPersonHint`.
The dialog UI is the only piece that was removed.

## Context from Comments

The only comment is `@claude` from a maintainer triggering this
planning run — no clarification on intent. Bryan (the submitter)
filed this as user feedback from the prod app; the ambiguity is
whether he expected the button to exist (i.e. wants it restored)
or merely noticed the broken hint (i.e. wants the text fixed).
Either reading is plausible from the title alone.

## Codebase Findings

- **Bug site:** `app/lib/presentation/screens/communities/manage_members_screen.dart:209-217`
  (`_buildEmptyShadowState`) renders `context.l10n.manageMembersEmptyShadow`,
  which references the missing button.
- **String definitions:** `app/lib/l10n/app_en.arb:3292` and
  `app/lib/l10n/app_es.arb:1413` — both locales contain the stale hint.
- **Stale strings (only referenced by the empty-state hint or unused):**
  `manageMembersAddPersonButton`, `manageMembersAddPersonTitle`, and
  `manageMembersAddPersonHint`. A repo-wide grep
  (`grep -rn manageMembersAddPerson app/lib`) finds them only in the ARB
  files and the generated localization classes — no widget references them
  any more. They are dead code that the codegen carries forward.
- **View-model dead code:** `ManageMembersNotifier.createShadowUser`
  (`app/lib/presentation/viewmodels/manage_members_view_model.dart:88-112`)
  has no call sites in the app — every UI invocation now goes through
  other view-models that talk to `ShadowUserRepository.createShadowUser`
  directly (`past_transfer_modal.dart:451`,
  `fulfill_modal_view_model.dart:205`, `event_modal_view_model.dart:694`,
  `gen_experience_view_model.dart:828`). The method is still covered by
  three unit tests in
  `app/test/presentation/viewmodels/manage_members_view_model_test.dart:91-141`.
- **Where shadow users actually get created in the live UI:** as a
  side-effect of completing a past transfer, RSVPing a guest who isn't
  on Ripls, or generating an event. There is no other proactive
  "add a person" surface — the manage-members FAB was the only one,
  and it was replaced.
- **Existing modal pattern:** the screen already uses
  `showAccessibleModal<void>` for the invite sheet
  (`manage_members_screen.dart:231, 248`); the original "add someone"
  dialog used `showDialog<String>` with an `AlertDialog`. If we restore
  the create flow, `showAccessibleModal` is the current pattern (the old
  `showDialog` predates the accessibility wrapper).
- **Tests at risk:** the three `createShadowUser` tests in
  `manage_members_view_model_test.dart` cover happy-path, error-path,
  and empty-name-guard. If the view-model method is removed (Option A),
  those tests must be removed alongside it.

## Potential Solutions

### Option A: Update the empty-state copy and clean up dead code *(recommended)*

- **Approach:**
  1. Rewrite `manageMembersEmptyShadow` so it stops referring to a
     button. New copy describes how shadow users actually appear here —
     as a side-effect of inviting them to an event, completing a past
     loan/giveaway, or fulfilling a request. Both `app_en.arb` and
     `app_es.arb` get the new value.
  2. Remove the now-unused strings `manageMembersAddPersonButton`,
     `manageMembersAddPersonTitle`, `manageMembersAddPersonHint` from
     both ARB files.
  3. Remove `ManageMembersNotifier.createShadowUser` from
     `manage_members_view_model.dart` and the matching `isCreatingShadowUser`
     state field, plus the three unit tests in
     `manage_members_view_model_test.dart` that cover it.
  4. Run `npm run generate` so `app_localizations*.dart` and
     `manage_members_view_model.freezed.dart` regenerate.
  5. Run `npm run test:app`.
- **Pros:** Smallest user-visible change. Aligns the screen with the
  product direction the team already chose in `4c95171fa` (proactive
  shadow-user creation moved into the activity flows where the shadow
  user has a *purpose* — an event to attend, a loan to record). Removes
  three pieces of dead code that would otherwise drift further (l10n
  strings, view-model method, tests for an unreferenced method).
- **Cons:** If product *actually* wants a proactive add affordance back,
  this codifies its absence and we'll re-add it later. Mitigated by the
  Open Question below — if the answer is "we want the button," switch
  to Option B before merging.
- **Effort:** Small.

### Option B: Restore the "Add someone new" button + dialog

- **Approach:**
  1. Keep the empty-state copy (or only lightly tweak it).
  2. Add a second floating-action affordance that calls
     `manageMembersProvider.notifier.createShadowUser`. Two viable
     placements:
     - Convert the single FAB into a `SpeedDial` / two-button row:
       "Invite Someone" + "Add someone new".
     - Add an inline button under the section header (e.g. a `TextButton.icon`
       at the top of `_buildShadowSection`) so it's discoverable even
       when the list is non-empty.
  3. Show a name-entry sheet via `showAccessibleModal<void>` (the
     current accessibility-aware modal helper). Reuse
     `manageMembersAddPersonTitle` / `manageMembersAddPersonHint`.
     `manageMembersAddPersonButton` becomes the trigger label again.
  4. On submit, call
     `ref.read(manageMembersProvider.notifier).createShadowUser(...)`,
     surface duplicate-name handling via `manageMembersDuplicatePrompt`
     (already present in `app_en.arb:3294`), and refresh the list.
  5. Add a widget test that taps the button, types a name, and asserts
     the shadow user appears.
- **Pros:** Restores a feature that was clearly intended (strings,
  view-model method, and even a duplicate-name prompt are all still in
  the codebase). Lets community admins add a participant before any
  event/loan exists for them — useful for onboarding a known offline
  group.
- **Cons:** A shadow user with no associated activity has no real
  purpose in the system (other create paths attach the shadow to a
  transfer/event/request immediately). Re-adds a UX surface the team
  deliberately removed a month ago, without a reviewer signal that this
  was a mistake. Larger diff (new modal widget, new state plumbing,
  new tests).
- **Effort:** Medium.

### Option C: Add a passive "how people get added" link instead of a button

- **Approach:** Update the empty-state hint to a tappable text link or
  bullet list ("People appear here when…") that explains the three
  paths: complete a past loan/giveaway with someone offline, RSVP a
  guest who isn't on Ripls, etc. Optionally turn each bullet into a
  shortcut into the relevant flow.
- **Pros:** Educates the user about the actual mental model. No new
  creation surface added.
- **Cons:** More copy work, more design, and only marginally clearer
  than Option A. The shortcuts would be nice but cross too many
  screens to build for one feedback ticket. Not worth the complexity
  unless we already see this confusion at scale.
- **Effort:** Medium.

## Recommendation

**Option A.** It's the smallest change that fixes the literal bug Bryan
reported (the hint references a button that doesn't exist), and it
aligns the screen with the product direction that commit `4c95171fa`
already encoded. The view-model method, the three unused strings, and
the three tests are stranded dead code that this fix should pick up.

If a maintainer comments on the PR saying "actually we want the button
back" — easy to pivot to Option B. The view-model method is a 25-line
diff; the dialog is the only meaningful work.

**Out of scope for this fix:**
- Redesigning how shadow users are surfaced or named on the screen.
- Changing the "Invite Someone" FAB behavior.
- Touching `manageMembersDuplicatePrompt` (still used by other flows
  via `gen_experience_view_model.dart` and `event_modal_view_model.dart`).
  A quick grep confirms — leave it alone.

## Open Questions

- **Is Option A or Option B the product intent?** — Recommendation:
  **Option A**. The team already moved away from proactive shadow-user
  creation a month ago in `4c95171fa`, and Bryan's title focuses on the
  broken reference, not on a missing capability. If a reviewer comments
  `@claude switch to Option B`, pivot before implementation.
- **What should the new empty-state copy say?** — Recommendation:
  *"Anyone you've referenced in events, loans, or giveaways but who
  isn't on Ripls yet will appear here. Send them an invite link from
  this list."* Both English and Spanish need to be updated. Keep it
  factual — don't introduce new product terminology.
- **Should we also remove `isCreatingShadowUser` from `ManageMembersState`?**
  — Recommendation: **yes**. Once the only writer (`createShadowUser`)
  is gone, the field is unused dead state. Removing it is a one-line
  Freezed change that regenerates cleanly.
- **Should we delete the unused localized strings or just leave them?**
  — Recommendation: **delete**. Per `CLAUDE.md`'s "no half-finished
  implementations" guidance and to keep `app_*.arb` honest about what
  the app actually supports. Translators waste time keeping them in
  sync.
- **Do any screen/widget tests exist for the empty-state path today?**
  — As of writing, no widget test file exists for
  `manage_members_screen.dart`. Recommendation: skip adding one for
  this fix — the change is l10n-only at the screen level. The
  view-model unit tests cover the state changes.

## Implementation Plan

### Phase 1: Rewrite the empty-state copy

**Goal:** Stop pointing users at a button that doesn't exist. Independently
shippable — even if Phase 2 is reverted, this fixes the reported bug.

**Tasks:**
- [ ] Update `manageMembersEmptyShadow` in `app/lib/l10n/app_en.arb`
      (line 3292) to: *"Anyone you've referenced in events, loans, or
      giveaways but who isn't on Ripls yet will appear here. Send them
      an invite link from this list."* (Or whatever the maintainer
      lands on — see the second open question.)
- [ ] Update the same key in `app/lib/l10n/app_es.arb` (line 1413)
      with the corresponding Spanish translation. Both locales must
      change in the same commit per `docs/client/i18n.md`.
- [ ] Run `npm run generate` so `app/lib/l10n/app_localizations*.dart`
      regenerates.
- [ ] Manual sanity check: no other `.dart` file references
      `manageMembersEmptyShadow` other than the screen, and the screen
      passes the value through unchanged. (`grep -rn
      manageMembersEmptyShadow app/lib` → only the screen and the
      generated localization files.)

**Validates:** Empty state hint reads correctly and matches reality.
Localization codegen passes.

### Phase 2: Remove the dead "Add someone new" surface

**Goal:** Delete the strings, view-model method, and tests that exist
only to support a button that no longer exists. Keeps the codebase
honest about what the app supports today.

**Tasks:**
- [ ] Remove `manageMembersAddPersonButton`,
      `manageMembersAddPersonTitle`, `manageMembersAddPersonHint`
      (and their `@`-description siblings) from both
      `app/lib/l10n/app_en.arb` and `app/lib/l10n/app_es.arb`.
- [ ] In `app/lib/presentation/viewmodels/manage_members_view_model.dart`,
      remove the `createShadowUser` method (lines 88-112) and the
      `isCreatingShadowUser` field on `ManageMembersState` (line 24).
      **Keep** the `RpcErrorHandler` import — `_loadMembers` and
      `_loadShadowUsers` still call `RpcErrorHandler.classify(e)` to
      populate the typed `UserError? error` state field (this is the
      shape `docs/client/i18n.md` mandates — viewmodels classify, widgets
      localize). Also keep the `getInviteLink` and `clearInviteLink`
      methods and their state fields — only `createShadowUser` and
      `isCreatingShadowUser` are dead.
- [ ] Run `npm run generate` so `manage_members_view_model.freezed.dart`
      and `app_localizations*.dart` regenerate without the removed
      symbols.
- [ ] In `app/test/presentation/viewmodels/manage_members_view_model_test.dart`,
      remove the entire `group('createShadowUser', ...)` block
      (lines 91-141). Other test groups stay.
- [ ] Run `npm run test:app` from the project root and confirm the
      remaining `manage_members_view_model_test.dart` groups still
      pass alongside the rest of the Flutter suite.
- [ ] Run `npm run lint:dart:size` to confirm no file crossed the
      1,000-line cap (this PR shrinks code, but worth a quick check
      since regenerated localization files can fluctuate).

**Validates:** Codebase no longer carries strings, state, or tests for
a feature the app doesn't expose. The Flutter test suite stays green.

## Acceptance Criteria

- The Manage Members → People section's empty-state text no longer
  mentions an "Add someone new" button. The new copy accurately
  describes how shadow users appear on this screen.
- Both `app_en.arb` and `app_es.arb` are updated; localization codegen
  produces a clean diff.
- `manageMembersAddPersonButton`, `manageMembersAddPersonTitle`, and
  `manageMembersAddPersonHint` are gone from both ARB files.
- `ManageMembersNotifier.createShadowUser` and the related
  `isCreatingShadowUser` state field are gone, and the three unit
  tests covering them are removed.
- `npm run test:app` passes locally. `npm run lint:dart:size` passes.

## Related Issues

- None identified. The previous shadow-user-related plan in
  `docs/issues/1161-access-sheet-zero-members.md` covers a different
  surface (`AccessSheet`, not `ManageMembersScreen`) and is unrelated.
