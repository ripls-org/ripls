# Issue #2797: Feedback hidden in "help" item

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2797
**Filed:** 2026-07-29 | **Last activity:** 2026-07-29
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/client/architecture.md` ⚓, `docs/client/conventions.md` ⚓,
`docs/client/i18n.md`, `docs/client/modals.md`, `docs/users/roles.md`,
`docs/reviews/accessibility_review_prompt.md`

> Companion issue: **#2798** covers the contrast defects on the sheet this row
> opens — see [`2798-feedback-sheet-contrast.md`](2798-feedback-sheet-contrast.md).
> They share a file boundary but not a fix; keep them as separate changes.

## Problem Statement

The profile menu ("me sheet") has a row labelled **Help** that does not open
help. `app/lib/presentation/widgets/profile_menu_modal.dart:130-134`:

```dart
_MeSheetRow(
  icon: Icons.help_outline,
  title: l10n.meSheetHelp,        // "Help"
  onTap: () => _openFeedback(context),   // -> FeedbackSheet.show(context)
),
```

`_openFeedback` (line 214) pops the menu and shows `FeedbackSheet`. There is no
help content anywhere behind this row — the destination is a feedback form, and
only a feedback form. The ARB metadata already admits it:

```json
"meSheetHelp": "Help",
"@meSheetHelp": { "description": "Me-sheet row opening help/feedback" },
```

So the label under-describes its own destination, and the description knew it.

Two things make this more than a cosmetic mismatch:

1. **This is the app's only in-product feedback path.** `FeedbackSheet` files
   GitHub issues directly — both #2797 and #2798 arrived through it. A label
   that reads "Help" suppresses the channel the team depends on for user
   reports. The reporter is the person who most wants that channel discoverable.
2. **The row is the quietest in its section.** Every other row that needs
   explaining carries a subtitle (`meSheetInvitationsSubtitle`,
   `meSheetMyGearSubtitle`, `meSheetMyPlansSubtitle`, `meSheetProfileSubtitle`).
   This one has `title` only, so nothing on screen hints that tapping it opens a
   "tell us what's wrong" form.

**Scope of the ask.** The reporter's clarification is explicit and narrow: the
label "should say *help and feedback* at least". "At least" is the reporter
leaving room for more, not asking for more. Treat the copy change as the
deliverable and the subtitle as the one cheap addition that serves the same goal.

## Context from Comments

**No comments on the issue.** The only elaboration is the reporter's out-of-band
clarification captured in the task invocation: *"this is in the profile menu
actually, says 'help' should say 'help and feedback'"* — which corrects the
issue title's implication that the problem is inside the feedback screen. It is
in the profile menu, one level up.

Worth flagging: the screenshot attached to this issue is byte-identical
(md5 `d4276fac495e6a70e5bf5729a2989694`) to the one on #2798 — it shows the
feedback sheet, not the profile menu, so it does not evidence this report. Both
submissions reused a single capture taken at 06:27 while the sheet was open. Not
a blocker here, but see Open Questions: it may be a real defect in the feedback
capture flow.

## Potential Solutions

### Option A: Rename to "Help & Feedback", add a subtitle
- **Approach:** Change the `meSheetHelp` value in `app_en.arb` to
  `Help & Feedback` and in `app_es.arb` to `Ayuda y comentarios`. Add a new
  `meSheetHelpSubtitle` pair ("Report a bug or share an idea" /
  "Informa de un error o comparte una idea") and pass it as the row's
  `subtitle`, matching the neighbouring rows. Update the `@meSheetHelp`
  description so it stops describing a destination the label now names.
- **Pros:** Exactly what the reporter asked for. Keeps "Help" as the leading
  word, so anyone scanning the menu for help still lands correctly, while
  "Feedback" makes the actual destination discoverable. Matches the platform
  convention users already know ("Help & feedback" in Google's Android apps).
  The subtitle costs one ARB pair and removes the last bit of ambiguity.
- **Cons:** The label still promises help that does not exist. That is a
  content gap, not a copy bug, and closing it is Option C's job.
- **Effort:** Small.

### Option B: Rename to "Send Feedback"
- **Approach:** Retitle the row for what it actually does; drop the help framing
  and switch the icon to `Icons.feedback_outlined`.
- **Pros:** Most honest label available today — there is no help content, so
  "Help" is a promise the app cannot keep.
- **Cons:** Loses the users who go looking for "help" when they are stuck; they
  are the same people whose problems the team wants reported. Also forecloses
  this row being the natural home for help content later, forcing a second
  rename. Diverges from the reporter's stated wording for no user-visible gain.
- **Effort:** Small.

### Option C: Split into two rows — Help, and Feedback
- **Approach:** A real Help row (FAQ / support content / contact) plus a
  separate Feedback row opening the sheet.
- **Pros:** Every label tells the truth and each destination is one tap.
- **Cons:** Requires help content that does not exist — a support surface, its
  copy, and its localization are the actual work, and none of it is in this
  issue. Adds a menu row to a section that already has five.
- **Effort:** Large, and almost entirely content rather than code.

## Recommendation

**Option A.** The reporter named the string they want and the reason is sound:
the app's only feedback channel is hidden behind a word that does not mention
feedback. A one-line copy change fixes the reported harm today, and adding the
subtitle brings the row into line with its neighbours for the cost of one ARB
pair.

Option B is more literally accurate but trades away the discoverability that
motivated the report. Option C is the right end state and should be filed as its
own issue — it is blocked on writing help content, and letting that block a
two-string fix would leave the feedback channel buried for however long the
content takes.

Keep the ARB **key** as `meSheetHelp`. Renaming keys churns both catalogs and
every reference for zero user-visible benefit; only the value and the `@`
description need to move.

## Open Questions

- **"Help & Feedback" or "Help and Feedback"?** — Recommendation: the
  ampersand. It matches the Android convention users have seen for a decade and
  keeps the row title short enough to avoid ellipsis on narrow devices (the row
  already sets `overflow: TextOverflow.ellipsis`). ARB values are plain text, so
  `&` needs no escaping.
- **Add the subtitle, or title only?** — Recommendation: add it, but understand
  what it does and does not buy. `_MeSheetRow` builds
  `Tappable(semanticsLabel: title)` (`profile_menu_modal.dart:244-245`), so the
  **subtitle is never announced to screen readers** — true of every existing
  subtitle in this menu, not something this change introduces. So the subtitle
  helps sighted scanning only, and the *title* carries the entire burden for
  screen-reader users. That is an argument for the rename being the load-bearing
  half of this fix, and for not substituting a subtitle for it.
  Note `scripts/check_l10n_parity.js` treats `app_en.arb` as the source of truth
  and requires **exactly** the same key set in `app_es.arb` — a new key added to
  one file and not the other fails `npm run lint:dart:l10n-parity`.
- **Should `_MeSheetRow` announce its subtitle?** — Recommendation: not here.
  It affects five rows and is a behaviour change to a shared widget; folding it
  into a copy fix would make this PR about something else. File it against
  `docs/client/accessibility.md`'s semantic-label conventions.
- **Change the icon?** — Recommendation: keep `Icons.help_outline`. The label
  leads with "Help", so the icon still reads correctly, and swapping to a
  feedback glyph would undercut the discoverability the rename is buying.
- **Is the duplicate screenshot a bug?** — Both #2797 and #2798 carry the same
  image bytes, captured before either was written. Recommendation: do not fix it
  here, but check whether `FeedbackSheet`'s picked-file state survives a
  submit→reopen cycle (`FeedbackSheetNotifier` state after `isSubmitted`) — if a
  second report silently inherits the first report's attachment, every follow-up
  bug arrives with the wrong evidence. File separately if confirmed.
- **Should the whole sheet be localized?** — `feedback_sheet.dart` renders ~15
  hardcoded English strings ("We'd love to hear from you!", "Bug", "Feature",
  "Other", "Title", "Description", "Submit Feedback", the privacy footer, the
  success state). It survives because `lint:dart:i18n` is defined in
  `package.json` but **not** included in the aggregate `lint` script.
  Recommendation: out of scope here; file it as its own issue rather than
  smuggling a 15-string migration into a label fix.

## Implementation Plan

### Phase 1: Rename the row and describe it
**Goal:** The profile menu names the feedback channel it opens.
**Tasks:**
- [ ] `app/lib/l10n/app_en.arb`: `meSheetHelp` → `"Help & Feedback"`; update
      `@meSheetHelp` description to "Me-sheet row opening the feedback sheet".
- [ ] `app/lib/l10n/app_en.arb`: add `meSheetHelpSubtitle` →
      `"Report a bug or share an idea"` with an `@` description.
- [ ] `app/lib/l10n/app_es.arb`: `meSheetHelp` → `"Ayuda y comentarios"`; add
      `meSheetHelpSubtitle` → `"Informa de un error o comparte una idea"`.
- [ ] `app/lib/presentation/widgets/profile_menu_modal.dart:130-134`: pass
      `subtitle: l10n.meSheetHelpSubtitle` on the row.
- [ ] `npm run lint:dart:l10n-parity` and `npm run lint:dart:arb-orphans` clean.
- [ ] `cd app && flutter test --reporter expanded` clean (regenerate l10n first
      via `npm run generate` — `app_localizations*.dart` is generated).
**Validates:** The reported harm is gone — the row states that feedback lives
behind it, and the subtitle says what tapping it does.

### Phase 2: Confirm on the surface the reporter used
**Goal:** The change reads correctly at the real row width, in both themes.
**Tasks:**
- [ ] Open the profile menu in the running app (`/ripls-run` or an emulator) and
      confirm no ellipsis at the narrowest supported width in light and dark.
      The me-sheet is a `SolidSheet`, not glass (`docs/client/modals.md`), so its
      colours are theme-aware and both themes are genuinely distinct here.
- [ ] Confirm the title still fits at 200% text scale
      (`docs/reviews/accessibility_review_prompt.md` §4) — "Help & Feedback" is
      roughly twice the width of "Help" and the row sets
      `overflow: TextOverflow.ellipsis`.
**Validates:** That the longer string did not truncate or break the row layout
on the device class the report came from (Pixel 9 Pro, Android).

## Related Issues
- **#2798** — the contrast defects on the sheet this row opens. Same feature,
  independent fix; both were filed through this row's destination.
- **#2764 / #2770** — the on-glass contrast work that #2798 continues.

## Audit notes

Audited against the context map. Findings applied in place:

- **Corrected a check that would have failed.** The draft's Phase 2 asked to
  confirm the row's semantics label reads "title plus subtitle". It does not:
  `_MeSheetRow` passes `semanticsLabel: title` only, so no subtitle in this menu
  reaches a screen reader. Replaced with an accurate check, and the consequence
  — the title carries the whole burden for screen-reader users — now appears in
  Open Questions as support for the rename over a subtitle-only fix.
- **Added the 200% text-scale check** (`accessibility_review_prompt.md` §4). The
  new title is ~2× the width of "Help" in a row that already ellipsizes; this is
  the plausible way the change regresses.
- **Noted the me-sheet is a `SolidSheet`, not glass** (`docs/client/modals.md`),
  so unlike #2798's surface its colours are theme-aware and both themes need
  looking at.
- Confirmed the ARB-key decision against `docs/client/i18n.md`: keeping
  `meSheetHelp` and changing only the value plus `@description` is consistent
  with the key-naming convention, and `check_l10n_parity.js` constrains only the
  key set, not values.
- No architecture, security, api_design, or testing dimension is engaged beyond
  the l10n gates — this is two ARB values, one new ARB pair, and one widget
  argument.

**Docs consulted:** `docs/llms.txt`; anchors `client/architecture.md`,
`client/conventions.md`, `reviews/accessibility_review_prompt.md`; leaf docs
`client/i18n.md`, `client/modals.md`, `client/accessibility.md`,
`users/roles.md`.
