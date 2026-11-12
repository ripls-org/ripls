# Issue #2925: Email OTP code entry UI doesn't match phone

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2925
**Filed:** 2026-08-15 | **Last activity:** 2026-08-15
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/registration_and_login.md` (globs `app/lib/presentation/screens/auth/**`), `docs/client/architecture.md` (anchor), `docs/client/conventions.md` (anchor), `docs/proto_conventions.md` (anchor — no proto surface touched), `docs/client/i18n.md`, `docs/client/testing.md`, `docs/client/testing/semantics_identifiers.md`, `docs/reviews/accessibility_review_prompt.md` (engaged dimension)

## Problem Statement

The phone and email one-time-code screens already render the *same widget*. The
only difference is one argument at each call site:

| | phone (`phone_auth_screen.dart:508`) | email (`email_auth_screen.dart:355`) |
|---|---|---|
| `hintText` | `'------'` (literal, six hyphens) | `context.l10n.emailAuthCodeHint` → `"6-digit code"` |

Because `OtpCodeField` styles its text as `headlineMedium` with
`letterSpacing: 8`, and Flutter's `InputDecorator` derives the hint style from
the field's own `style` (via `baseStyle`), the phone hint paints as a wide
`- - - - - -` ruler that reads as "six digits go here." The email hint paints
the words *"6-digit code"* at the same headline size and letter-spacing, which
is why it looks like a different feature rather than a different label — the
reporter's two screenshots capture exactly this and nothing else about the box
differs.

**The sharp part:** the divergence was authored in a single diff. `4cf4387b6`
("Replace the email password form with a one-time-code flow (#2571 phases 2-3)")
is the commit that extracted `OtpCodeField`, and in that one change it carried
the phone field's pre-existing dash literal into the new shared widget (the
literal dates to `0e12187d5`, 2026-04-14, when the phone screen used a raw
`TextField`), gave the phone field its first real label, **and** created the
email call site with a brand-new translatable hint instead of reusing the dash
literal a few lines away in its own diff. Both values were in front of the same
author in the same change.

The widget's doc comment, written in that commit, states the intent it missed:

> The 6-digit one-time-code entry field, shared by the phone and email
> verification flows **so both look and behave identically** — the two sit next
> to each other as sign-in options, and a code box that differs between them
> reads as two unrelated features.

The widget kept `hintText` as a nullable per-caller parameter, and the
divergence walked straight through the hole that parameter left open. So this is
not a missed detail in an old screen; it is a shared widget whose contract is
documented in prose but not enforced in its API. That is the thing worth fixing,
not the one string.

Two notes on the history, because they cut against an easy assumption. The dash
ruler *is* the older design (April), and `app_en.arb:345` records that the phone
field **"previously had only a `'------'` hint, which screen readers announced as
dashes"** — the fix chosen then was to add a real localized `labelText` rather
than drop the dashes. But that mitigation is **exactly as old as
`emailAuthCodeHint`**: the label landed in `4cf4387b6` too. So the label is not
long-standing precedent that the email flow failed to follow — both approaches
were minted in the same commit. The dashes win on the merits argued below, not on
seniority.

### Everything else that differs between the two code steps

Found while reading both screens. The reporter named only the dashes, but "email
should match phone" is a parity claim, and these are the rest of it:

| aspect | phone | email | who's right |
|---|---|---|---|
| placeholder | `'------'` | `"6-digit code"` | **phone** — this is the issue |
| keyboard submit (`onSubmitted`) | not wired — pressing ✓ on the numeric keypad does nothing | wired to verify | **email**, but its wiring has its own hole (below) |
| submit-enabled check | `_otpController.text.length != 6` (bare literal, no trim) | `.trim().length == OtpCodeField.codeLength` | **neither** — see below |
| submit button label | `"Verify"` | `"Continue"` | **phone** — resolved 2026-08-15, both now share one key |
| resend button label | `"Resend Code"` | `"Send a new code"` | **phone** — resolved 2026-08-15, both now share one key |
| extra affordances | none | "Use a different email", spam-folder hint, "Use my password instead" | intentional — the flows genuinely differ here |

Note the keypad-submit row: screenshot 2 shows the phone numeric keypad with a
✓ key rendered, and on the phone screen that key is inert. The asymmetry the
reporter noticed visually has a functional twin pointing the other way.

**The enable-check row deserves its own paragraph, because the obvious reading of
it is wrong.** It looks like phone is missing a `trim()` that email has. But
`trim()` is itself the workaround, and email carries the same underlying defect:
`OtpCodeField` sets `maxLength: codeLength` with **no `inputFormatters`**, so the
cap counts *characters*, not digits. Type or paste `" 12345"` and the field is
"full" at six characters while holding five digits — and the sixth digit the user
then types is silently swallowed by the cap. On phone the button enables and
submits five digits (rejected by Firebase Auth as `invalid-verification-code`,
surfaced as `phoneAuthInvalidCode` — note there is no Ripls server call in this
path at all). On email the button silently *stays dead* with a field that looks
complete and no message explaining why, which is arguably the worse failure. So
copying email's `trim()` onto phone spreads a workaround and papers over a shared
root cause. The fix belongs one level down, in the shared widget — see Phase 2.

`OtpCodeField` has exactly two **production** call sites app-wide, plus one test
construction site (`email_auth_screen_test.dart:101`, which passes no hint, so
removing the parameter doesn't touch it). It is the app's only code-entry field:
the add-phone flow reuses `PhoneAuthScreen` in `PhoneAuthMode.attach`
(`docs/registration_and_login.md:651-653`) rather than carrying its own copy, and
`phone_verification_setup.dart` is APNs silent-push token setup, not a screen.
`EmailAuthScreen` is hosted from three screens (`login_screen.dart:377`,
`register_screen.dart:240`, `phone_auth_screen.dart:480`), all sharing the one
`_buildCodeStep`. So a single edit fixes every surface.

One pre-emptive note for review: `OtpCodeField.fieldKey` may look like the
Patrol-era dead weight that `docs/client/testing/semantics_identifiers.md:41-45`
tells you to strip from `screens/auth/`. It isn't — it exists for the
keyboardType-swap bug documented at `otp_code_field.dart:32-37` and is used by
the Dart tests. Leave it.

### Why `ripls-ui-investigate` was skipped

The skill's job is to bisect a UI report to a layer (client render / client
state / server / data / UX gap). That bisection is already closed here by static
evidence: two `hintText:` arguments, `git blame`-dated to one commit, with no
state, server, or data participation — the strings are compile-time constants.
Standing up the hermetic stack to watch a placeholder render would confirm what
the reporter's own screenshots already show. The **email** flow is covered by four
existing e2e specs (`login-screen-email-code.spec.ts`,
`email-existing-member-login.spec.ts`, `phone-guest-email-register.spec.ts`,
`phone-gear-email-register.spec.ts`), which is where the runtime check belongs.
If a reviewer wants the reproduction anyway, `login-screen-email-code.spec.ts`
is the matching scenario for screenshot 1. (Note that this coverage is
email-only, and does *not* extend to the phone-side changes in phase 2 — that
flow has its own, much larger spec set, enumerated there. One consequence worth
carrying forward: the phase-2 whitespace defect is **not** e2e-reachable at all,
because Flutter Web renders this field as `<input type="number">`.)

## Context from Comments

**No comments.** The body is one sentence — "Email should match phone with six
dashes" — plus two screenshots, submitted by user2@example.org from the in-app
feedback sheet (v0.30.0, Android 17, Pixel 9 Pro, PROD).

Normally a one-line issue with no discussion is a signal to go back for
clarification. Here it isn't: the two screenshots pin the exact pair of screens,
the requested direction is stated ("six dashes"), and the reporter is the
maintainer. The only thing the issue leaves genuinely open is how far past the
placeholder "match" is meant to reach — see Open Questions.

## Potential Solutions

### Option A: Change the one string at the email call site
- **Approach:** `email_auth_screen.dart:355` → `hintText: '------'`. Delete the
  now-unused `emailAuthCodeHint` key from `app_en.arb` and `app_es.arb` (the
  `lint:dart:arb-orphans` gate fails otherwise, and `lint:dart:l10n-parity`
  requires both locales move together).
- **Pros:** Three-line diff. Zero risk. Ships today.
- **Cons:** Leaves the divergence *reachable* — the next caller, or the next
  edit to either screen, can re-introduce it just as easily, and the widget will
  still be documenting a guarantee it doesn't enforce. It also duplicates the
  same magic literal at two call sites, which is how the first copy drifted.
- **Effort:** Small.

### Option B: Bake the placeholder into `OtpCodeField` and delete the parameter
- **Approach:** Add `static const String placeholder = '------';` to
  `OtpCodeField`, use it in the widget's own `InputDecoration`, and **remove the
  `hintText` parameter entirely**. Both call sites drop the argument. Delete
  `emailAuthCodeHint` from both ARB files.
- **Pros:** The widget's stated promise — "both look and behave identically" —
  becomes structurally true: there is no longer an argument through which the two
  flows *can* differ. The magic literal exists once, next to the `codeLength`
  constant it pairs with, where a comment can explain why it isn't localized.
  Same shipped pixels as Option A.
- **Cons:** Slightly wider diff (three files instead of two). It also cuts
  against the standing preference for *generic* primitives over specialized
  ones: removing a parameter makes the widget less reusable, and a future third
  caller wanting its own placeholder would have to put the knob back. Two things
  make that acceptable here rather than a violation — (1) no third OTP surface
  exists or is imminent (`OtpCodeField` has exactly two call sites and is the
  only code-entry field in the app, including the add-phone flow), and (2) the
  only time that flexibility was ever exercised, it produced this bug. If a
  third caller ever needs an override, re-adding an optional named parameter is
  a two-line change — and at that point it would be re-added *deliberately*,
  with a reason, rather than sitting open by default.
- **Effort:** Small.

### Option C: Replace both with a real segmented six-box field
- **Approach:** Build a proper OTP input — six discrete boxes, per-digit
  focus advance, paste-splitting, auto-submit on the sixth digit — and adopt it
  in both flows.
- **Pros:** The modern convention for code entry; makes "six digits" structural
  rather than a hint that vanishes the moment you type. Auto-submit removes a tap
  from both flows.
- **Cons:** Wrong-sized response to the report. A segmented field is a real
  accessibility project (six inputs must not be announced as six unlabeled
  fields), it interacts with `AutofillHints.oneTimeCode` and Android SMS
  autofill — which currently work for free because there is one field — and it
  re-opens the `getByRole('textbox')` locator assumption that four email specs,
  seven phone workflow specs, and five walkthrough specs all rest on (six boxes
  means `.first()`/`.last()` no longer address the code field). None of that is
  on the critical path to "make the placeholder match."
- **Effort:** Large.

## Recommendation

**Option B**, plus the behavioral parity work in phase 2 — wiring keyboard
submit on phone, and fixing the whitespace defect *in the shared widget* rather
than reconciling the two enable-checks.

Reasoning: the shipped placeholder is identical to Option A, and the extra cost
over Option A is a handful of lines — but it converts the widget's doc comment
from an aspiration into an invariant. The bug here is genuinely that a shared
widget exposed a knob nobody should turn; fixing the value while leaving the knob
is fixing the symptom of the design that produced the report.

The behavioral items ride along because they are the same subsystem, the same
files, and the same "these two flows should match" review question. Bundling them
is cheaper than a second PR, and leaving them means the next person to compare
the flows files this issue again from the other direction. Note that they resolve
*downward* into `OtpCodeField` too, for the same reason the placeholder does —
the enable-check divergence turned out to be two screens working around one
missing input filter, so the fix belongs in the widget, not at either call site.
Whatever else is true of this issue, the answer to it keeps being "put it in the
shared widget."

Option C is a real idea and should be its own issue if wanted — deliberately not
folded in.

## Open Questions

- ~~**Does "match phone" extend to the button labels?**~~ **RESOLVED
  2026-08-15: yes — both flows now read "Verify" and "Resend Code", off one
  shared pair of ARB keys.** Original recommendation and its reasoning kept
  below for the record.
  Recommendation was: **unify on the phone
  wording ("Verify" / "Resend Code") in a follow-up phase, not phase 1.** The
  issue is a parity report and the phone screen is the named reference, so phone
  wording wins on the reporter's own framing. It is held out of phase 1 because
  it is a copy decision the maintainer owns, not a defect, and because it carries
  a specific test trap: `e2e/lib/ui/email-register.ts:132,135` locates the email
  submit button by `getByRole('button', { name: /^continue$/i })`, and both lines
  must change with it. (The follow-on worry — that "Verify" would then match the
  phone step's own button — was checked and ruled out; see phase 3.) Note `emailAuthSendCode`
  ("Email me a code") already carries an ARB comment explaining that it is
  *deliberately* not "Send code" to avoid exactly that collision — so the
  precedent here is that email/phone button labels were split on purpose, and
  this question deserves an explicit answer rather than an assumption.
- **Does giving the email field a dash placeholder regress accessibility?** —
  Recommendation: **no, and the mitigation is already in place — just don't
  break it.** This question already has a recorded answer in the tree:
  `app_en.arb:345` says the dashes *were* announced as dashes by screen readers,
  and the resolution was to give the field a real localized `labelText`
  (`phoneAuthEnterCode`), which `OtpCodeField`'s own doc comment (`:13-16`) calls
  out as load-bearing for both screen-reader announcement and Playwright
  name-based location. The email field already has the equivalent label
  (`emailAuthEnterCode`, same English text).
  **Two mechanism details matter, and both are more specific than "the hint is
  just there."** First, the hint is *merged into the field's `label`* — it is not
  a separate `hint` slot: `_childSemanticsConfigurationDelegate`
  (`input_decorator.dart:1708-1734`) marks it merge-up, so it becomes part of the
  field's accessible **name**. Second, it is in the tree at all only because
  `OtpCodeField` sets `autofocus: true` (`:57`): the hint sits inside an
  `AnimatedOpacity` gated on `isEmpty && !_hasInlineLabel`, whose semantics are
  dropped at alpha 0. Measured on the real widget, focused and empty, the name is
  `"Enter the 6-digit code\n------"`; unfocused, the dashes are absent entirely.
  **This is why the change is an a11y improvement, not a wash** — a point worth
  making explicitly in review. Today the email field announces *"Enter the
  6-digit code, 6-digit code"*: the hint is a verbatim duplicate of the label it
  is merged into. Replacing it with dashes removes a redundant announcement.
  **The actionable rule is therefore a guardrail, not a change:** never let the
  dashes become the field's only accessible name, and don't move them into
  `labelText` — that is precisely the state `app_en.arb:345` records as the bug
  that was already fixed once.
  On the rubric's other axes, only the glyph content changes and both answers
  favor the change. **Contrast** is untouched: the hint color
  (`onSurfaceVariant`, `input_decorator.dart:5960`) and the field fill
  (`surfaceContainerHighest`, `otp_code_field.dart:70`) are the same tokens
  before and after. **Text scaling to 200%** improves: six dashes is half the
  glyph count of "6-digit code" at the same `headlineMedium` + `letterSpacing: 8`,
  so the hint's intrinsic width shrinks and overflow risk drops. That same
  styling path is why the dashes render large and spaced at all — `TextField`
  forwards its `style` as `baseStyle` (`text_field.dart:1770`), the hint style
  merges it, and M3's default contributes *only* a color afterward
  (`:5956-5961`), leaving size and letter-spacing intact.
  **If someone later does want the dashes excluded:** do not reach for
  `InputDecoration.hint:` (the Widget form) naively. `hintStyle`, `textAlign`,
  and the `baseStyle` inheritance that makes the dashes render large and
  letter-spaced apply *only* on the `hintText` path (same SDK lines), so
  switching to `hint:` silently drops the very styling this issue is asking for.
  Note also that `require_l10n_for_semantic_labels` would flag a literal on
  `Semantics(hint:)` but does not see `InputDecoration.hintText` — the lint will
  not catch a mistake here.
- **Should the six dashes be localized?** — Recommendation: **no — but land it
  as an acknowledged exception, not as compliance.** `docs/client/i18n.md:46` is
  unambiguous ("**Strings belong in ARB files, not Dart code.** Every user-visible
  string should use `context.l10n.*`") and grants no carve-out for non-prose
  glyphs. The dashes *are* a user-visible string in the presentation layer, so
  keeping them in Dart is a deviation that needs stating out loud rather than
  waved past. The substantive case for deviating is strong: a hyphen run is
  field-shape iconography with no linguistic content, there is no locale where it
  means something else, and routing it through ARB is precisely what produced this
  bug — a translatable hint existed, so it got used and then drifted. So keep it
  as a `static const` on `OtpCodeField` with a comment saying why, **and** either
  note the exception in the PR description or add a one-line carve-out to
  `i18n.md` for non-linguistic glyph placeholders. Prefer the carve-out: it is
  the durable answer, and the next person reaching for ARB here should find the
  rule rather than re-litigate it. Do not lean on the lint as cover —
  `scripts/check_i18n.js` greps a fixed literal allowlist and would not object
  either way, and it is not even wired into `npm run lint` (only `test:impact`),
  so the comment and the doc are the *only* things preventing a round trip.
- **Should `docs/registration_and_login.md`'s `globs` gain
  `app/lib/presentation/widgets/auth/**`?** — Recommendation: **yes, in phase
  1.** That doc is the one that actually describes both code flows, it already
  *names* the shared widget in prose (`:53` — "The 6-digit code field shared by
  the phone and email flows"), and it carries a freshness stamp
  (`verified_commit: 5e33d2173`, `verified_on: 2026-08-14`) — but its `globs`
  stop at `app/lib/presentation/screens/auth/**`, so edits to the widget itself
  never mark it affected. The widget is not unowned (the broad `app/lib/**` globs
  of `client/architecture.md` and `client/conventions.md` cover it), but those
  anchors say nothing about the phone/email parity guarantee, which is the claim
  that broke. Closing the gap costs one line plus
  `npm run generate:context-map`.

## Implementation Plan

### Phase 1: One placeholder, structurally enforced
**Goal:** The email code field is visually identical to the phone code field,
and the difference cannot be re-introduced by a call site.
**Tasks:**
- [x] Add `static const String placeholder = '------';` to `OtpCodeField`,
      beside `codeLength`, with a comment covering (a) that it renders as a
      spaced ruler because the field's `letterSpacing: 8` reaches the hint via
      `InputDecorator`'s `baseStyle`, (b) why it is not an ARB key, and (c) that
      the localized `labelText` — not this — is what screen readers announce.
      Durable wording only: no "phase N" or issue-scoped markers in the comment
      (`docs/client/conventions.md:51-57`). Keep the constant **public** so the
      test below can name it — and note why that matters: `lint:dart:dead` would
      *not* catch a public member referenced only inside its own `build()`,
      because `app/tool/dead_code.dart:237-247` counts the declaration itself and
      skips anything reaching a total of 2. A public constant with no external
      consumer would ship unnoticed, so either the test references it or it
      should be `_placeholder`.
- [x] Use it in the widget's own `InputDecoration`; **delete the `hintText`
      parameter** and its doc comment. Dropping a named parameter from a const
      constructor is a **compile error** (`UNDEFINED_NAMED_PARAMETER`), not an
      analyzer info — the enforcement Option B buys is stronger than a lint and
      independent of the fatal-infos setting.
- [x] Drop `hintText:` from `phone_auth_screen.dart:508` and
      `email_auth_screen.dart:355`.
- [x] Delete `emailAuthCodeHint` + its `@emailAuthCodeHint` description from
      `app/lib/l10n/app_en.arb:288-289` and `app/lib/l10n/app_es.arb:99`, then
      regenerate l10n (`app_localizations*.dart` are generated and gitignored —
      never hand-edit). This deletion is **mandatory, not a tidy-up**: the
      orphan gate's allowlist "may SHRINK but never GROW"
      (`scripts/check_arb_orphans.js:17-18`), so there is no hatch for leaving
      the key behind. Leave `emailAuthCodeSent` alone —
      `e2e/tests/workflows/signin-unknown-identity.spec.ts:73-76` matches it
      (as a `toHaveCount(0)`, so changing that string would quietly void the
      assertion rather than fail it).
- [x] **Create `app/test/presentation/widgets/auth/otp_code_field_test.dart`**
      and *move* the existing cap test into it. The mirror rule
      (`docs/client/conventions.md:181`, `docs/reviews/testing_review_prompt.md:82`)
      puts a widget's tests at the widget's path, and
      `email_auth_screen_test.dart:98-117` is already misfiled — it constructs a
      bare `OtpCodeField` with no `EmailAuthScreen` in the tree. A single-file
      directory is normal here: `app/test/presentation/widgets/` already has ~35
      subdirectories, several holding exactly one file. (The `CLAUDE.md:204-208`
      README rule governs `app/lib/` and `server/`, not `app/test/`, so it has no
      bearing on this choice — an earlier draft of this plan wrongly invoked it.)
      In the new file:
      (1) **Tighten the moved cap test.** As written it asserts
      `lessThanOrEqualTo(codeLength)`, which passes for *any* length including
      zero — a vacuous assertion (`testing_review_prompt.md:77`), and the one that
      would have caught the whitespace truncation below. Make it
      `expect(controller.text, '123456')`.
      (2) Add the a11y guardrail: `expect(tester.getSemantics(...).label,
      contains('Enter the 6-digit code'))` **and**
      `isNot(equals(OtpCodeField.placeholder))`. It must be `contains`, not
      equality — the focused, empty field's name is
      `"Enter the 6-digit code\n------"` because the hint merges into the label
      (see the a11y open question). The `isNot` half is what makes it a guardrail
      rather than a tautology. Two scaffolding notes: this needs
      `tester.ensureSemantics()`, and there is **no existing semantics assertion
      anywhere in `app/test/`** (no `matchesSemantics`, `containsSemantics`, or
      `SemanticsFlag` usage), so budget for writing that from scratch. It also
      needs `pumpAndSettle()` so `autofocus: true` lands — without focus the hint
      is dropped from the tree and the test passes for the wrong reason.
- [x] Separately, in `email_auth_screen_test.dart`, assert that the **screen**
      wires the *localized* label onto the code step. This can't live in the
      widget test: that test passes its own hardcoded English `labelText`, so it
      proves the widget forwards some label, not that `EmailAuthScreen` supplies
      `context.l10n.emailAuthEnterCode`.
      Deliberately *not* asserting that Flutter paints the hint string:
      `docs/client/testing.md:381` lists "Test Flutter framework behavior" as a
      Don't, and with the parameter gone the parity is guaranteed by construction.
- [x] While editing the widget, correct its own doc comment. `:13-16` says
      Flutter "composes a field's accessible name from its label" — it composes
      it from label **and hint** (they merge; see the a11y open question). The
      claim is load-bearing for the Playwright locators, so leaving it half-right
      in the file that this change makes canonical is how the next reader gets it
      wrong.
- [x] Add `app/lib/presentation/widgets/auth/**` to
      `docs/registration_and_login.md`'s `context.globs`; regenerate with
      `npm run generate:context-map`. Do **not** hand-bump the `freshness:`
      stamp — the nightly ratchet owns it, and `:53`'s prose stays true.
- [x] Gates: `cd app && flutter analyze`, `npm run lint:dart:arb-orphans`,
      `npm run lint:dart:l10n-parity`, `npm run lint:ephemeral-comments` (this
      change deliberately writes a multi-clause comment, so the gate that bans
      phase/ephemeral markers is directly in play),
      `npm run lint:dart:dead`, `npm run lint:context-map`,
      `cd app && flutter test --reporter expanded`.
      `npm run lint:dart:a11y` will pass but proves nothing here — it is an
      allowlist ratchet with no rule that inspects `hintText`, so do not read a
      green from it as an accessibility check.
      Generated code is already present locally, so the Flutter suite runs
      without a `npm run generate` first. All five gates were confirmed
      baseline-clean while planning (`flutter analyze`: no issues;
      arb-orphans: 2,128/2,128 keys referenced; l10n-parity: OK across 2 ARBs;
      a11y: 0 violations; context-map: current) — so any failure they report
      belongs to this change.
- [x] No screenshot baselines to regenerate: no walkthrough spec drives the
      email or phone code flow, and there are no committed PNG/snapshot
      baselines under `e2e/tests/walkthroughs/`.
**Validates:** That the reported divergence is gone on all three hosts of
`EmailAuthScreen` from one edit; that removing the parameter makes the
divergence unrepresentable rather than merely absent; and that nothing depended
on the old placeholder (verified up front — e2e reaches these fields via
`getByRole('textbox')`, and no Dart test or spec references `emailAuthCodeHint`,
the string `"6-digit code"` as a hint, or `'------'`).

### Phase 2: Behavioral parity — fixed at the shared widget, not copied between screens
**Goal:** The two code steps behave the same, and the whitespace defect both of
them carry is fixed once rather than worked around twice.
**Tasks:**
- [x] **Root-cause fix, in the shared widget:** add
      `inputFormatters: const [FilteringTextInputFormatter.digitsOnly]` to
      `OtpCodeField`. This is the actual bug behind the enable-check asymmetry.
      Today `maxLength: codeLength` (`otp_code_field.dart:59`) installs a
      `LengthLimitingTextInputFormatter` that counts **characters**, and there are
      no other formatters, so `" 12345"` fills the field at six characters while
      holding five digits — and silently swallows the sixth digit the user types
      next. Filtering at entry makes the cap count digits, which fixes phone's
      bad submit *and* email's silently-dead button *and* the swallowed digit, in
      one line, for both flows. It is the same argument this plan makes for the
      placeholder: fix the shared widget, don't reconcile two call sites.
- [x] Then give the widget a `static bool isComplete(String)` and use it at both
      call sites, replacing `phone_auth_screen.dart:514`'s bare `!= 6` and
      `email_auth_screen.dart:340`'s `.trim().length ==`. With `digitsOnly` in
      place the `trim()` is no longer load-bearing, and a shared predicate keeps
      the *definition* of "complete" in one place — the alternative duplicates
      `trim().length == codeLength` at both call sites, which is the shape Option
      B's rationale argues against. Bonus: it makes the test a pure-Dart `test()`
      rather than a widget test.
- [x] **Do not** simply port email's `trim()` to phone. It converts a wrong
      submission into a dead button with no explanation — the field looks full,
      the button does nothing, and `_errorMessage`
      (`phone_auth_screen.dart:278-281`) is only ever set by a failed call, so
      nothing tells the user why. That trades `error_handling_review_prompt.md:87-88`
      (specific, actionable, field-level errors) for `:89` (fail fast) and is
      strictly worse than today's "invalid code" message.
- [x] Wire `onSubmitted` on the phone `OtpCodeField` so the keypad's ✓ verifies —
      but gate it on `!_isLoading && isComplete(...)`, **not** on email's current
      condition. Email wires `onSubmitted: codeComplete ? _handleVerifyCode : null`
      (`email_auth_screen.dart:356`) while its button uses
      `_isLoading || !codeComplete` (`:363`), and `_handleVerifyCode` doesn't
      re-check `_isLoading` — so email's keypad ✓ can fire a second verify while
      the first is in flight. Copying it verbatim would port that hole to phone;
      fix both instead.
- [x] Test the completeness predicate directly (pure Dart), and assert
      `" 12345"` can no longer reach the controller at all once `digitsOnly` is
      in. **A widget/unit test is the only possible guard here:** on Flutter Web,
      `TextInputType.number` becomes a real `<input type="number">` that browsers
      refuse non-numeric input into, so no Playwright spec can reproduce or
      regress-guard this defect.
- [x] For keypad-submit, scope the assertion honestly: there is no
      `phone_auth_screen_test.dart` today, and `_handleVerifyCode` early-returns
      when `_verificationId == null`, so the testable claim is that the rendered
      `OtpCodeField.onSubmitted` is non-null when the code is complete and null
      otherwise — not that a verification round-trip happens.
- [x] Manually confirm Android SMS autofill still populates the phone field
      after the `onSubmitted` change; `AutofillHints.oneTimeCode` works today
      because there is exactly one field, and nothing in CI exercises the real
      Firebase SMS path (dev and e2e both use the Auth Emulator).
- [x] Re-run the phone-flow e2e, which is **wider than the email flow's**:
      `e2e/lib/ui/phone-register.ts` is imported by seven workflow specs
      (`phone-{gear-interest,gear-giveaway,request-offer,community-join,rsvp}-full-loop`,
      `phone-existing-member-login`, `add-phone-to-account`) **and five
      walkthrough specs** (`onboarding`, `requests`, `events`, `classroom`,
      `runclub`). Wiring `onSubmitted` is expected to be inert for all of them —
      that helper drives fields with `pressSequentially` and `Backspace` and
      never sends Enter (`phone-register.ts:25,35`), so no double-submit is
      possible in the specs — but this is the phase whose blast radius is worth
      measuring rather than assuming. `npm run build:e2e` first, per the bundle
      trap noted in phase 3.
- [x] Note that `PhoneAuthScreen`'s OTP step is also the **add-phone** flow's
      code entry (it is the app's only phone-code surface;
      `phone_verification_setup.dart` is APNs silent-push token setup, not a
      screen), so the phone-side changes land there too.
**Validates:** That "match phone" holds in both directions; that the whitespace
defect is gone from *both* flows rather than worked around in one; and that the
one-field-plus-autofill setup still works after the submit path changes.

### Phase 3: Copy unification (approved 2026-08-15 — both flows say "Verify" / "Resend Code")
**Goal:** Identical actions read identically across the two flows.
**Tasks:**
- [x] **Collapsed onto one pair of shared keys** rather than pointing the email
      keys at the phone wording. `emailAuthVerify` / `emailAuthResendCode` /
      `phoneAuthVerify` / `phoneAuthResendCode` are replaced by `authVerifyCode`
      ("Verify") and `authResendCode` ("Resend Code"), in `app_en.arb` and
      `app_es.arb` together. Each of the four old keys had exactly one caller, so
      this is a six-line change — and two keys holding identical copy is the same
      drift-enabler as the per-caller `hintText` was: a copy or translation edit
      to one of them silently reproduces #2925 in a new place. Their
      `@` descriptions now say the two flows share them on purpose and why the
      buttons cannot collide.
- [x] **Fix the fifth `/^continue$/i` consumer first — it is the one that fails
      silently.** `e2e/tests/workflows/signin-unknown-identity.spec.ts:69-72`
      asserts `getByRole('button', { name: /^continue$/i })` has
      `.toHaveCount(0)`, guarding "no code step is presented for an address with
      no account." Renaming the button makes that assertion **vacuously true** —
      it would pass even if a code step were wrongly shown, because no button is
      named "Continue" anymore. The spec's companion assertion at `:73-76`
      (`/we sent a 6-digit code/i`, count 0) survives, so the test is not fully
      hollowed, but its primary guard would be. Retarget the locator in the same
      commit as the rename; do not treat this spec as unaffected because it never
      calls the email helper.
- [x] Update `e2e/lib/ui/email-register.ts:132` **and** `:135` off
      `/^continue$/i` — both lines, since they bracket the code step.
      *The obvious objection here was checked and does not hold:* naming the
      email button "Verify" does **not** collide with the phone helper's
      `/^verify$/i` locators (`phone-register.ts:92,95,129,132`), because the two
      buttons can never be in the tree together. `PhoneAuthScreen` is
      step-switched (`:331-339`) and renders the email expander only inside
      `_buildPhoneInputStep` (`:427`), so advancing to the OTP step removes the
      email form entirely; `LoginScreen` (`:340`) and `RegisterScreen` (`:252`)
      *navigate* to `PhoneAuthScreen` rather than inlining the phone OTP. So this
      phase really is a two-line e2e edit plus the ARB change — but re-run the
      phone and walkthrough specs anyway, because that helper backs seven
      workflow and five walkthrough specs and the reasoning above is worth
      confirming empirically rather than trusting.
- [x] Run the four affected email specs — `login-screen-email-code.spec.ts`,
      `email-existing-member-login.spec.ts`, `phone-guest-email-register.spec.ts`,
      `phone-gear-email-register.spec.ts` — plus `signin-unknown-identity.spec.ts`
      for the retargeted negative assertion. **Run `npm run build:e2e` first.**
      The web bundle is `//go:embed`'d into the server binary, so rebuilding the
      bundle alone changes nothing served (`docs/client/testing.md:536-546`);
      skipping it means the specs pass or fail against the *old* button label,
      which on a copy change is the worst possible false signal.
**Validates:** Whether the ARB comment on `emailAuthSendCode` — which says the
email/phone button labels were split deliberately to avoid duplicate-label
ambiguity — argues against this phase. If it does, close the question by
documenting the intentional split instead of erasing it.

## Related Issues
- **#2571** — the passwordless email one-time-code work. Commit `4cf4387b6`
  (its phases 2-3) created both call sites and the shared widget, so it
  introduced this divergence; plan at `docs/issues/2571-email-otp.md`.
- **#2595** — the inline email option's keyboard-avoidance behavior on the
  phone-first screen, cited in `email_auth_screen_test.dart:125`; same code step,
  so its test guarantees must survive these edits.
- **#2724** — journey polish that set the email expander's "Continue with email"
  wording, cited in `e2e/lib/ui/email-register.ts:109-112`; the precedent for
  treating email/phone label wording as a deliberate choice.
- **#2827** — the in-app copy migration that moved UI strings into ARB; phase 1
  deletes one of its keys, and the "don't localize the dashes" recommendation is
  the boundary case that migration didn't rule on.
- **#2492** — the phone-first / account-optional epic these auth screens belong to.

## Audit Notes

Audited per `ripls-audit`. Scope: client-only (`app/lib/presentation/**`, the two
ARB files, one e2e helper, one doc metadata block) in the **auth** subsystem.
Engaged dimensions: accessibility, testing, code_quality, tech_debt,
error_handling. Deliberately not engaged: api_design (no proto/RPC surface),
security (no authz or credential-handling change — client-side submit gating
only; Firebase still validates the code), dependency (no new deps), observability
and performance (no logging, metric, or hot-path surface touched).

**What the audit changed, in rough order of consequence:**

1. **Phase 2 was rewritten around the root cause.** The original plan proposed
   porting email's `trim()` onto phone. That was a kludge: the real defect is that
   `maxLength` counts characters and `OtpCodeField` has no `inputFormatters`, so
   both flows mis-handle whitespace — email's version silently *disables* its
   button and both swallow the sixth digit. The fix is
   `FilteringTextInputFormatter.digitsOnly` in the shared widget, plus a shared
   `isComplete` predicate. Porting the trim would also have replaced a visible
   "invalid code" message with a dead button and no explanation, trading two
   error-handling requirements for one.
2. **A double-submit hole was found in the code being copied.** Email's
   `onSubmitted` isn't gated on `_isLoading` and `_handleVerifyCode` doesn't
   re-check it, so its keypad ✓ can fire a second verify in flight. Phase 2 now
   fixes both rather than propagating it.
3. **A wrong historical claim was corrected.** The plan said both `hintText`
   arguments were written in the same commit. The phone literal actually predates
   it (`0e12187d5`, 2026-04-14); `4cf4387b6` carried it into the new widget and
   added the phone label in the same diff. Consequence: the label mitigation is
   *not* older precedent that email ignored — it is exactly as old as
   `emailAuthCodeHint`, so the dashes win on merit, not seniority.
4. **The accessibility mechanism was wrong in two ways.** The hint doesn't sit
   beside the label — it **merges into the field's accessible name**
   (`input_decorator.dart:1708-1734`), and it reaches the tree only because
   `autofocus: true` keeps the `AnimatedOpacity` at full alpha. This produced the
   plan's strongest a11y argument, which the first draft missed entirely: email
   currently announces *"Enter the 6-digit code, 6-digit code"*, so the change
   **removes** a redundant announcement.
5. **A silently-vacuous e2e assertion was found for phase 3.**
   `signin-unknown-identity.spec.ts:69-72` guards "no code step for an unknown
   address" via `toHaveCount(0)` on `/^continue$/i`. Renaming the button makes it
   pass unconditionally. It is not in any list of "email specs" because it never
   calls the email helper.
6. **Test placement was corrected to the mirror rule**
   (`conventions.md:181`, `testing_review_prompt.md:82`): a new
   `app/test/presentation/widgets/auth/otp_code_field_test.dart`, with the
   already-misfiled cap test moved into it. The first draft's "a new directory
   would hold one file" objection was wrong (`app/test/presentation/widgets/` has
   ~35 subdirectories, several single-file) and its README citation was a
   non-sequitur (that rule governs `app/lib/`, not `app/test/`).
7. **The existing cap test is vacuous** — `lessThanOrEqualTo(codeLength)` passes
   at length 0 — and is the assertion that would have caught the whitespace
   truncation. Now scheduled to be tightened.
8. **One risk I had raised was retracted after checking it.** I warned that
   renaming email's button to "Verify" would collide with the phone helper's
   `/^verify$/i` locator. `PhoneAuthScreen` is step-switched and drops the email
   expander before the OTP step renders, and the login/register screens navigate
   to it rather than inlining it, so the two buttons can never share the tree.
9. **The i18n position was reframed** from "aligned" to an explicit, argued
   exception to `docs/client/i18n.md:46`, with a recommendation to add a durable
   carve-out for non-linguistic glyph placeholders rather than rely on a lint that
   isn't even in `npm run lint`.
10. **Gates and traps added:** `lint:dart:dead` and `lint:ephemeral-comments`;
    `npm run build:e2e` before any spec run (the bundle is `//go:embed`'d, so
    rebuilding it alone serves nothing new); the note that `lint:dart:a11y` passes
    here without checking anything relevant; and that the phase-2 whitespace bug
    is not e2e-reachable at all, because Flutter Web renders this field as
    `<input type="number">`.
11. **Smaller factual fixes:** removing the parameter is a *compile error*, not an
    analyzer info; `lint:dart:dead` cannot see a public member used only in its own
    `build()` (`app/tool/dead_code.dart:237-247`), so the new constant needs a test
    reference or a `_` prefix; three miscited line ranges corrected
    (`testing.md:381`, `conventions.md:51-57`, `CLAUDE.md:204-208`); "two call
    sites" clarified to two production plus one test construction; and the widget's
    own doc comment is now scheduled for correction, since it claims the accessible
    name comes from the label alone.

**Verified clean / no change needed:** `docs/registration_and_login.md` is not
contradicted and none of its prose goes false (it never mentions placeholders or
code-step button labels; its `:53` widget reference becomes *more* true). No
`getByPlaceholder` or `getByLabel` targets any code field anywhere in `e2e/`, so
the placeholder change is e2e-safe. No walkthrough spec drives either code flow
and there are no committed snapshot baselines. Adding a `Semantics(identifier:)`
is correctly declined per `semantics_identifiers.md:61`. All gates were confirmed
baseline-clean before planning.

**Docs consulted:** `docs/llms.txt`; anchors `docs/client/architecture.md`,
`docs/client/conventions.md`, `docs/proto_conventions.md` (no proto surface);
leaf/subsystem `docs/registration_and_login.md`, `docs/client/i18n.md`,
`docs/client/testing.md`, `docs/client/testing/semantics_identifiers.md`;
review rubrics `docs/reviews/accessibility_review_prompt.md`,
`testing_review_prompt.md`, `code_quality_review_prompt.md`,
`tech_debt_review_prompt.md`, `error_handling_review_prompt.md`,
`architecture_review_prompt.md`. Also read as primary sources: the pinned Flutter
SDK's `input_decorator.dart` / `text_field.dart`, and the enforcing scripts
`check_arb_orphans.js`, `check_i18n.js`, `check_a11y.js`, `app/tool/dead_code.dart`.

## Implementation Notes

Phases 1 and 2 are implemented (commit `4ff67a4de`). Phase 3 is deliberately not
— it waits on the button-copy question. Four places where the implementation
departed from the plan, all worth knowing before the next edit here:

1. **The screen-level localized-label test was dropped.** The plan wanted an
   assertion that `EmailAuthScreen` supplies `context.l10n.emailAuthEnterCode`
   rather than a literal. Reaching the code step requires a successful
   `requestEmailCode` — `_handleSendCode` only advances `_step` on
   `result.success` — so the test needs `@GenerateMocks([AuthService])` and a
   build_runner pass, which no test in that file currently has. Not worth the
   scaffolding, because the invariant is **already enforced structurally**:
   dropping the `context.l10n` reference orphans the ARB key and fails
   `npm run lint:dart:arb-orphans`. Noted in the test file so the next reader
   doesn't re-add it.
2. **`tester.getSemantics(find.byType(TextField)).label` returns `''`** — that
   finder does not resolve to the node carrying the merged name, so the plan's
   suggested assertion would have failed for a reason unrelated to what it was
   testing. The test uses `find.bySemanticsLabel` instead, which is
   node-location independent. A throwaway probe confirmed the audit's prediction
   exactly: one `isTextField` node whose label is
   `"Enter the 6-digit code\n------"`, with `maxValueLength: 6`.
3. **`FilteringTextInputFormatter.digitsOnly` is not a constant**, so
   `inputFormatters: const [...]` is a compile error. Hoisted to a
   `static final` list on the widget rather than allocated per build.
4. **Two unrelated things surfaced and should not be attributed to this change.**
   Local `server/gen` was stale (`notification.proto` declares
   `transfer_offer_covered` / `transfer_type`, the generated Go lacked the
   getters), which broke `npm run build:e2e`'s server step until
   `npm run generate`. Separately, `npm run generate` rewrote
   `website/content/release-notes/0.30.0.html`'s release date from "August 14" to
   "August 15". **Diagnosed properly — an earlier draft of this note described it
   as a blanket "stamps today's date" bug, which is wrong.**
   `scripts/generate_release_notes.sh:48-57` takes the date from
   `git tag -l "v$ver" --format='%(creatordate:short)'` and **falls back to
   `date +%Y-%m-%d` only when that tag does not exist.** Two things compound:
   - The 0.30.0 notes were committed in `514e0bb2f` at **2026-08-14 22:45 UTC**,
     but the `v0.30.0` tag was not cut until **2026-08-15 01:12 UTC** — so at
     generation time the tag did not exist and the fallback stamped the
     generation date. Writing notes before tagging is the normal release order, so
     **the newest release's date is a build artifact rather than a release fact**
     until someone regenerates after the tag lands.
   - `%(creatordate:short)` on an annotated tag is when the *tag* was cut, with no
     timezone normalization. This tag was cut at 01:12 UTC = **20:12 US Central on
     Aug 14**, so the tag-derived date falls a day after both the release commit
     (`32b98d821`, 2026-08-14 22:43 UTC) and the local day it was actually cut.

   So regeneration does not *correct* this date — it turns a defensible
   "August 14" into an arguably wrong "August 15", which is why it was reverted
   rather than committed. Only the newest release can be affected (0.29.0 and
   0.29.3 HTML dates match their tags exactly), which is why the churn has gone
   unnoticed: it self-resolves the first time anyone regenerates post-tag, and
   then stays put forever. A real fix would derive the date from the release
   commit rather than the tag, and pin a timezone. Unfiled.

5. **Phase 3 turned up a third `/^continue$/i` consumer that the audit missed.**
   The audit found the count-0 guard at `signin-unknown-identity.spec.ts:69-72`;
   the same file also asserts the *positive* case at `:147` ("a registration
   surface still issues a code"), which matched the old label too. That one fails
   loudly rather than vacuously, and it was caught by running the specs after
   `npm run build:e2e` — not by any of the greps. Worth remembering that a label
   sweep needs the spec run to close it, not just the search.

**Verification:** `flutter analyze` clean; the full Flutter suite green (4,370
passed, 1 skipped) both before and after the copy change; e2e typecheck clean;
`lint:dart:{arb-orphans,l10n-parity,a11y,dead,size}`,
`lint:ephemeral-comments`, and `lint:context-map` all green; `npm run build:e2e`
rebuilt the embedded bundle before any spec run. E2E after the copy change: all
five email specs (`login-screen-email-code`, `email-existing-member-login`,
`signin-unknown-identity`, `phone-guest-email-register`,
`phone-gear-email-register`) and three phone specs
(`phone-existing-member-login`, `add-phone-to-account`,
`phone-community-join-full-loop`) pass. The five walkthrough specs that drive
`phone-register.ts` were not re-run and carry no risk from the copy change: the
phone labels' *text* is unchanged ("Verify" / "Resend Code" both before and
after) — only the ARB key names moved. Note for whoever
runs these next: that phone trio failed 2/3 on one 3-worker run and then passed
3/3 both serially and on a repeat of the identical parallel invocation — a
non-deterministic contention flake around the single shared Firebase Auth
Emulator, not a regression from this change.
