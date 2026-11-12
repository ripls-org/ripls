# Issue #2595: Inline email registration into PhoneAuthScreen + retire RegisterScreen (#2492)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2595
**Filed:** 2026-06-30 | **Last activity:** 2026-06-30
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/registration_and_login.md`, `docs/client/deep_linking.md`, `docs/client/architecture.md` (⚓), `docs/client/conventions.md` (⚓), `proto_conventions.md` (⚓), `docs/client/testing/semantics_identifiers.md`, `e2e/README.md`, `docs/reviews/accessibility_review_prompt.md`, `docs/reviews/testing_review_prompt.md`

> **Scope decision (owner directive, 2026-07-01):** *Only inline the email form
> onto `PhoneAuthScreen`. Do **not** retire `RegisterScreen` in this issue.* The
> larger retirement (re-homing the invite-hub, first-launch, and manual-code
> responsibilities) stays deferred. This plan is written to that scope; the
> retirement is captured under **Deferred / follow-up** so the tracking isn't
> lost.

## Problem Statement

On the phone-first guest screen (`PhoneAuthScreen`, register mode), the
below-the-fold "other ways to sign in" block offers an **email** option that
route-replaces to a *separate* screen: `_goToEmailRegister()` does
`context.go('/register')` (`phone_auth_screen.dart:851`). That yanks the guest
out of the phone-first flow they were in — losing the hero/backdrop framing and
the in-flight action context — just to fill in an email + password.

Email registration is a single RPC (`authService.registerWithPassword(email,
name, password, shortCode)` → `EmailRegister`). `PhoneAuthScreen` already owns
the post-auth plumbing this needs — `_authQueryParams()`, `_navigateToHome()`,
`setAuthState()` — and already inlines the *Google* secondary path
(`_handleGoogleSignIn`). So the email path can be inlined the same way: the guest
expands an email/password mini-form in place and never leaves the screen.

The issue also frames a **large** follow-on — actually retiring `RegisterScreen`
(~867 lines) by re-homing its invite-hub / first-launch / manual-invite-code /
community-full / invitation-required responsibilities. **Per the owner directive
above, that is out of scope here.**

### Important constraint the issue flags

Inlining email *by itself* "adds a duplicate email form without removing
anything." We address that within the inline-only scope by **extracting the
email/password mini-form into one shared widget** that both `PhoneAuthScreen`
(new host) and `RegisterScreen` (existing `_buildEmailExpander`) render — so we
end up with **one** email form living in two hosts, not two copies.

### Line-count constraint (this forces the shared-widget design)

`app/lib/presentation/screens/auth/phone_auth_screen.dart` is **957 lines**; the
CI-enforced Dart cap (`npm run lint:dart:size`) is **1,000**. Adding the email
form's fields, four `TextEditingController`s, validation, and the RPC handler
*inline* on the phone screen (~200 lines) blows the cap. Extracting the form into
its own `StatefulWidget` (which owns its controllers) keeps `phone_auth_screen`
essentially flat — it only gains a small `ExpansionTile` wrapper.

## Context from Comments

**The issue has no comments.** All direction comes from (a) the issue body's own
"Sizing / Recommendation" split and (b) the owner's 2026-07-01 directive to do
*only* the inline-email step and keep `RegisterScreen`. No further clarification
needed to proceed at this scope.

## Potential Solutions

### Option A: Shared `EmailRegisterForm` widget, hosted by both screens (recommended)
- **Approach:** Extract a new `StatefulWidget` (e.g.
  `app/lib/presentation/widgets/auth/email_register_form.dart`) that owns the
  name/email/password/confirm `TextFormField`s + `AutofillGroup`, runs
  validation, calls `registerWithPassword` + `setAuthState` +
  `TextInput.finishAutofillContext()`, surfaces inline errors, and invokes an
  `onRegistered` callback so each host does its own post-auth navigation. Props:
  `shortCode` and `onRegistered` (host navigates: `_navigateToHome()` on
  `PhoneAuthScreen`, `context.go(_postRegistrationDestination())` on
  `RegisterScreen`). `PhoneAuthScreen._buildSecondaryAuthOptions` swaps its
  "Register with email" `OutlinedButton` for an `ExpansionTile` hosting the
  widget; `RegisterScreen._buildEmailExpander` is re-pointed at the same widget.
- **Pros:** No duplicate form (satisfies the issue's explicit concern); keeps
  `phone_auth_screen.dart` under the 1,000-line cap; `RegisterScreen` untouched
  in role (only its form internals move to the shared widget), so nothing is
  "retired"; one place to test the email-register logic; renders correctly under
  both the normal theme and the forced `AppTheme.darkTheme` guest backdrop
  because it reads `Theme.of(context)`.
- **Cons:** Slightly more up-front refactor than a raw inline; requires threading
  an `onRegistered` callback for host-specific navigation.
- **Effort:** Small (~1–2 hr incl. tests).

### Option B: Inline the form directly on `PhoneAuthScreen`, leave RegisterScreen's copy alone
- **Approach:** Add the four controllers + fields + handler straight into
  `_PhoneAuthScreenState`.
- **Pros:** Most literal reading of "inline the email."
- **Cons:** Pushes `phone_auth_screen.dart` well over the 1,000-line CI cap
  (would need a `// dart-line-count-allow` hatch + issue — not acceptable per
  conventions); leaves **two** divergent email forms (the exact duplication the
  issue warns against).
- **Effort:** Small, but creates debt.

### Option C: Make the "email" secondary option push a lightweight modal/bottom-sheet
- **Approach:** Keep `PhoneAuthScreen` thin; the email button opens a
  `showModalBottomSheet` hosting the (still-extracted) form.
- **Pros:** Zero line-cap pressure on the phone screen; strong visual separation.
- **Cons:** It's arguably not "inline" — the guest still leaves the fold into a
  sheet; more moving parts (sheet lifecycle, keyboard insets, dark-theme
  plumbing) than an `ExpansionTile`. Doesn't match the existing
  expander-in-place idiom already used on both `LoginScreen` and
  `RegisterScreen`.
- **Effort:** Small–medium.

## Recommendation

**Option A.** It's the only approach that satisfies all three constraints at
once: (1) the owner's "inline email, keep RegisterScreen" scope, (2) the issue's
"don't ship a duplicate form" concern, and (3) the 1,000-line Dart cap. The
`ExpansionTile`-hosts-a-shared-widget shape already exists on both `LoginScreen`
(`_buildEmailExpander`) and `RegisterScreen` (`_buildEmailExpander`), so the phone
screen picks up a familiar pattern and the shared widget is a straight lift of
`RegisterScreen`'s existing form fields + `_handleRegister`.

Post-auth navigation stays host-owned via the `onRegistered` callback so the
guest-action handoff still fires: on `PhoneAuthScreen` the callback runs
`_navigateToHome()` (which routes web guests back to `/event/{id}` /
`/item/{id}` / `/need/{id}` so the auto-RSVP / interest / offer handoff runs);
on `RegisterScreen` it runs `context.go(_postRegistrationDestination())`,
preserving today's behavior exactly.

## Open Questions

- **Where should the shared widget live?** — Recommendation:
  `app/lib/presentation/widgets/auth/email_register_form.dart` (new `auth/`
  widgets dir needs a `README.md` per the multi-file-dir rule; if we don't want a
  new dir, `app/lib/presentation/screens/auth/email_register_form.dart` beside
  the two screens also works and avoids the README). Prefer the `screens/auth/`
  co-location to keep the change tight.
- **Does the widget own `setAuthState` + autofill-finish, or just the fields?**
  — Recommendation: own everything through `setAuthState` and
  `TextInput.finishAutofillContext()`, then fire `onRegistered()`. Both current
  hosts do byte-identical work there, so centralizing it removes real
  duplication rather than just moving fields.
- **Should the guest-flow email expander log the existing
  `RegisterMethodSelectedEvent(method: 'email')` analytics event?** —
  Recommendation: yes, on first expansion, mirroring
  `RegisterScreen._buildEmailExpander`'s `onExpansionChanged` so email-method
  selection stays measurable across both surfaces.
- **Confirm-password field on the phone-first fold — keep it?** —
  Recommendation: keep it. It's cheap, matches `RegisterScreen`, and prevents
  typo lockouts for a passwordless-first audience that rarely types passwords.
- **Should `RegisterScreen` be converted to the shared widget in this same PR?**
  — Recommendation: yes. It's a pure internal refactor that does **not** retire
  or re-home the screen (its routes, hero, first-launch, and manual-code roles
  are untouched), and it's what turns "one more email form" into "one shared
  form." This is fully consistent with "don't kill RegisterScreen."

## Implementation Plan

### Phase 1: Extract the shared `EmailRegisterForm` widget
**Goal:** One tested widget that encapsulates email/password registration, with
no behavior change to `RegisterScreen`.
**Tasks:**
- [ ] Create `EmailRegisterForm` (`StatefulWidget`) that owns
      `name`/`email`/`password`/`confirmPassword` controllers, `AutofillGroup`,
      the field validators, obscure-toggles, and inline error text — lifted from
      `RegisterScreen._buildNameField/_buildEmailField/_buildPasswordField/`
      `_buildConfirmPasswordField/_buildErrorMessage/_handleRegister`.
- [ ] Props: `String? shortCode`, `VoidCallback onRegistered`, optional
      `submitLabel`. The widget calls `registerWithPassword` → `setAuthState` →
      `TextInput.finishAutofillContext()` → `onRegistered()`, and shows
      `result.error` inline on failure.
- [ ] Re-point `RegisterScreen._buildEmailExpander` to render
      `EmailRegisterForm(shortCode: widget.shortCode, onRegistered: () =>
      context.go(_postRegistrationDestination()))`; delete the now-dead field
      builders + `_handleRegister` from `RegisterScreen`.
- [ ] Keep the `register_email_expander` `Key` (and add an
      `email_register_form` key) so existing widget tests still resolve.
- [ ] While lifting, l10n the confirm-password `labelText` (today a raw
      `'Confirm Password'` string in `RegisterScreen._buildConfirmPasswordField`)
      — these `InputDecoration.labelText`s become the accessible names the
      Phase-4 e2e reaches via `getByLabel`, so they must be real, stable strings
      (and l10n keeps them consistent between the two hosts).
**Validates:** `RegisterScreen` behaves identically with the form extracted;
`register_screen_test.dart` stays green.

### Phase 2: Host the shared form inline on `PhoneAuthScreen`
**Goal:** The guest expands email in place on the phone screen — no
`/register` route hop.
**Tasks:**
- [ ] In `_buildSecondaryAuthOptions`, replace the "Register with email"
      `OutlinedButton` (and delete `_goToEmailRegister`) with an
      `ExpansionTile` (dark-theme-safe, matches the RSVP backdrop) hosting
      `EmailRegisterForm(shortCode: widget.shortCode, onRegistered:
      _navigateToHome)`.
- [ ] Fire `RegisterMethodSelectedEvent(method: 'email')` on first expansion.
- [ ] Verify `phone_auth_screen.dart` stays **< 1,000 lines**
      (`npm run lint:dart:size`).
- [ ] Confirm the forced-dark guest backdrop renders the expanded fields
      legibly (white-on-dark), same as the phone/OTP inputs.
**Validates:** A phone-first guest can register by email without leaving the
screen; web guests still land back on the item view and the action handoff
fires (because navigation runs through `_navigateToHome`).

### Phase 3: Widget tests + docs
**Goal:** Lock in behavior and keep the durable docs true to the code.
**Tasks:**
- [ ] Widget tests for `EmailRegisterForm` (validation, mismatch, success →
      `onRegistered` called; failure → inline error).
- [ ] Widget test on `PhoneAuthScreen`: guest-action flow shows the email
      expander below the fold and it expands to the form.
- [ ] Update `docs/registration_and_login.md` (the email-registration section
      now notes `PhoneAuthScreen` also hosts the inline email form via the
      shared `EmailRegisterForm`) and note in `docs/client/deep_linking.md` that
      the phone-first guest email path no longer route-hops to `/register`.
      Advance each doc's `freshness:` stamp.
- [ ] `RegisterScreen` and the `/register` route remain in place — this phase
      changes **no** routing.
**Validates:** `flutter test` green (run the **full** suite — a shared auth
widget touches multiple call sites); docs match code.

### Phase 4: Playwright e2e — guest email register on the phone-first screen
**Goal:** An end-to-end spec that drives the *real* inline email UI on the
phone-first guest screen and proves the action handoff fires — the coverage that
does **not** exist today.

**Current state (researched):** No e2e drives the UI email-register form. Both
email specs — `e2e/tests/guest-rsvp-confirm.spec.ts` and
`e2e/tests/ported-smoke.spec.ts` — seed the guest server-side via `registerUser`
(→ `EmailRegister` RPC) and `injectAuth`, *skipping* the UI form. The reason is
called out verbatim in `guest-rsvp-confirm.spec.ts:15`: *"the form lives on
RegisterScreen and would need additional Semantics identifiers to be
locator-able from Playwright."* The phone-first full-loop specs
(`e2e/tests/workflows/phone-rsvp-full-loop.spec.ts`, `phone-gear-*`,
`phone-request-offer-*`) DO drive the real phone-auth UI on `/verify-phone` and
assert the handoff — that's the pattern to mirror. An email spec is **simpler
than the phone loop**: `EmailRegister` is a single RPC, so it needs **no Firebase
Auth Emulator and no OTP read-back**.

**Tasks:**
- [ ] **Locatability — use accessibility-tree locators, NOT
      `flt-semantics-identifier`.** Per
      `docs/client/testing/semantics_identifiers.md`, `Semantics(identifier:)`
      on **deeply-nested** widgets (fields inside an `ExpansionTile` several
      levels below `Scaffold.body`) gets **merged/pruned out of DOM
      serialization** — a `page.locator('[flt-semantics-identifier="…"]')`
      won't find them. Only the existing `phone-auth-screen` `Scaffold.body`
      anchor reliably serializes. So the e2e must reach the fields the way
      `phone-register.ts` already does: **`getByRole` / `getByLabel`** against
      the accessibility tree. The four fields are disambiguated by their
      `InputDecoration.labelText` (Name / Email / Password / Confirm Password)
      via `page.getByLabel(…)`; the submit + expander via `getByRole('button',
      …)` / their visible text. The older `guest-rsvp-confirm.spec.ts:15`
      "needs Semantics identifiers" note predates this pruning guidance — do
      **not** add nested field identifiers; they'd be dead weight the convention
      forbids ("only when a test asks for it", and here they wouldn't even
      work).
- [ ] Add `e2e/lib/ui/email-register.ts` — `completeEmailRegister(page, {name,
      email, password})` sibling to `phone-register.ts`: tap the "Register with
      email" expander (by its visible text), then for each field `click +
      pressSequentially` (Flutter-web ignores `.fill()`), locating each via
      `getByLabel`; click the submit button via `getByRole('button', …)`.
- [ ] New spec `e2e/tests/workflows/phone-guest-email-register.spec.ts`: seed a
      host + community + event share link (reuse `registerUser`,
      `createCommunity`, `mintExperienceShareLink`, `installRequestIdOverride`);
      open the phone-first guest screen as an **unauth** guest via
      `/event/{id}?rsvp=yes&code={shortCode}` (routes to `/verify-phone`); drive
      the inline email form; assert (a) the guest is authenticated and landed on
      the item view, and (b) the RSVP handoff fired — the host sees
      `rsvpYesCount` increment (same assertion as `guest-rsvp-confirm` /
      `phone-rsvp-full-loop`).
- [ ] Run locally per `e2e/README.md` (gcloud is available — `build:web:e2e`
      then rebuild `tmp/server` so the embedded Flutter bundle is fresh, per
      `2492-web2-pivot-e2e.md`).
**Validates:** the whole inline-email guest path works against a real server +
bundle, and the guest-action handoff (auto-RSVP) still fires when the guest
chooses email instead of phone — closing the UI-form e2e gap the older specs
deliberately left open.

## Deferred / follow-up (explicitly out of scope here)

The issue's "Large" half — **retiring `RegisterScreen`** — is deferred by owner
directive. When picked up, it must re-home:
- the unauthenticated `/invite` landing hero + auth chooser (`app_router.dart`
  `/invite` route, unauth branch),
- the first-launch redirect (`app_router.dart` `FirstLaunchFlag` → `/register`),
- the manual invite-code entry card + `parseInviteCodeInput` + the
  "community full" / "invitation required" outcome screens,
- the `/register` route itself and callers
  (`web_experience_screen.dart:_navigateToRegister`).

Track that separately (keep it on #2595 as the "Large" item, or split a new
issue). Note the phone-first pivot de-emphasizes the community-cap concept
("no 32-cap" for ad-hoc communities, per epic #2492), which may let the
"community full" screen simply drop rather than be re-homed — decide at that
time.

## Related Issues
- #2492 — parent epic (phone-first / SMS-native pivot); this is polish on the
  phone-first guest auth surface it introduced.
- #2058 — legacy `gear_id`/`request_id`/`experience_id` URL-query-param cleanup
  that the `/register` and `/invite` routes still carry; relevant to the
  deferred retirement, not to the inline-email step.
- #2571 — "email shadowing" accepted risk on unverified email registration; the
  inline email path inherits it unchanged (same `EmailRegister` RPC).

---

## Audit Notes (ripls-audit, 2026-07-01)

**Scope:** client-only (`app/lib/presentation/**`, `e2e/**`). Engaged dimensions:
`testing` (e2e + widget), `accessibility` (auth UI, semantic locatability),
`code_quality` (shared-widget extraction, line cap), `i18n` (UI strings). No
server, proto, security-surface, or performance change — email register reuses
the existing `EmailRegister` RPC unchanged, so `api_design`/`security` are not
engaged beyond the already-tracked #2571 email-shadowing risk (noted).

**Fixes applied during audit:**
1. **Corrected the e2e locator strategy (Phase 4).** The draft said to add
   `Semantics(identifier:)` to the form fields so Playwright could find them.
   `docs/client/testing/semantics_identifiers.md` documents that nested
   identifiers (fields inside an `ExpansionTile` well below `Scaffold.body`) are
   **merged/pruned from DOM serialization** and won't be found by a
   `flt-semantics-identifier` locator — only route-level `Scaffold.body` anchors
   reliably serialize. Rewrote the task to use accessibility-tree locators
   (`getByRole`/`getByLabel`), matching what `e2e/lib/ui/phone-register.ts`
   already does, and disambiguating the four fields by their `labelText`. This
   also honors the doc's "add identifiers only when a test asks for it" rule.
2. **Added an i18n/accessibility task to Phase 1.** The lifted confirm-password
   `labelText` is a raw `'Confirm Password'` string today; l10n it, since these
   `labelText`s are the accessible names the e2e reaches via `getByLabel` and
   must be stable and consistent across both hosts.
3. **Recorded the 1,000-line Dart cap** (`client/conventions.md` §"Flutter
   Screens" / CLAUDE.md) as the concrete forcing function for the shared-widget
   design — `phone_auth_screen.dart` is at 957/1000, so inlining fields directly
   (Option B) would breach the cap and require a hatch, which the plan now
   rejects.

**No conflicts found** with the retained-`RegisterScreen` scope: the plan makes
no routing change, keeps `/register` and the `/invite` unauth landing on
`RegisterScreen`, and only relocates the email form internals into a shared
widget both screens render — consistent with the owner directive.

**Docs consulted:** `docs/client/conventions.md` (⚓), `docs/client/architecture.md`
(⚓, skimmed for MVVM/widget-extraction), `proto_conventions.md` (⚓, confirmed no
proto change), `docs/registration_and_login.md`, `docs/client/deep_linking.md`,
`docs/client/testing/semantics_identifiers.md`, `e2e/README.md` (referenced),
plus source: `phone_auth_screen.dart`, `register_screen.dart`,
`register_view_model.dart`, `app_router.dart`, `auth_service.dart`,
`web_experience_screen.dart`, and the existing e2e specs/helpers
(`guest-rsvp-confirm.spec.ts`, `phone-rsvp-full-loop.spec.ts`,
`e2e/lib/ui/phone-register.ts`).
