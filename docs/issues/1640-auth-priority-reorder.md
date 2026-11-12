# Issue #1640: Update registration and login UI to emphasize phone and OIDC (Google) login

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1640
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

Both auth screens give email/password top billing. On the login screen, email and password fields are rendered first with the "Login with Email" button, and Google / Phone are tucked under an "OR" divider as outlined-button alternates. On the register screen, the form opens with Name + Email + Password + Confirm Password (all required up front) before the OIDC section appears below the primary "Register" button.

This visually positions email/password as the default — the opposite of the desired posture. The product preference is **Phone first, Google second, Email last**, since phone and OIDC are lower-friction and (for OIDC) carry the user's name in the identity token. Two concrete user-confusion problems follow from the current layout:

1. **Visual hierarchy lies.** A user landing on Register sees a four-field form and assumes they must fill it out, even though they intended to use Google or phone.
2. **Name is collected too early.** The Register screen demands "Your Name" up front, but for phone we can collect it after OTP succeeds (already implemented in `phone_auth_screen.dart`'s `_PhoneAuthStep.nameInput`) and for Google the name comes from the ID token (already extracted in `oidc_service.dart`). The early Name field is dead weight on the OIDC and phone paths.

The desired state is: each auth screen leads with Phone and Google as primary actions, with the email/password form available on the same screen but visually demoted and tucked behind an expander so it's not the first thing a user sees.

## Context from Comments

The issue has no comments — only the description from the author (also the assignee). That makes the description authoritative. One nuance worth flagging: the description hedges on phone ("maybe we can get the phone verification done before we insert the new user, and prompt for the user then") — but the codebase already does exactly this. The phone register flow today is OTP first, then name input, then `phoneRegister` RPC call. So no server change is needed for the phone path; we can confirm and move on.

The issue's last sentence ("Only for the email/pass signup do we ask for all three things at once, and that will be on a separate screen after the user chooses that path") suggests a separate-screen approach, but the user (post-filing) has expressed a preference for keeping email on the same screen behind an expander to minimize taps.

## Potential Solutions

### Option A: Reorder buttons in place, email form behind an expander (recommended)

- **Approach:** Restructure both Register and Login screens with Phone and Google as primary visible actions at the top, and the email form collapsed behind a "Sign in with email" / "Sign up with email" expander/disclosure widget below an "OR" divider. Tapping the expander reveals the existing email fields in place; submitting still calls the existing `_handleLogin` / `_handleRegister`. No new routes, no new screens, no navigation step for the email path beyond a single tap to expand.
- **Pros:** Smallest diff. No router changes. One screen per flow keeps deep-link params (`shortCode`, `gearId`, etc.) in one place. The expander tap is one extra interaction vs. zero today, but the user is explicit that they accept this in exchange for not having a navigation transition. Server untouched.
- **Cons:** A single screen now serves three flows, which is more state to reason about. The expander needs careful animation handling so the layout doesn't jump unexpectedly when a user expands it on a small viewport. Form validators on the (initially-hidden) email fields must not run while collapsed.
- **Effort:** Small.

### Option B: Three-button chooser screen, dedicated email screen

- **Approach:** Reshape both screens into a clean three-button chooser; tapping Email pushes a new `EmailLoginScreen` / `EmailRegisterScreen` that hosts only the email form.
- **Pros:** Each screen has one job. No expander state to manage. Most discoverable hierarchy.
- **Cons:** Adds a navigation step on the email path, which the user has explicitly said they want to avoid. Two new screen files plus router/param-threading work.
- **Effort:** Medium.

### Option C: Server-side change — make `EmailRegisterRequest.name` optional and unify post-auth name prompt

- **Approach:** Strip name from the email register form too, and prompt for name on a shared "Welcome, what should we call you?" screen that fires after any first-time registration completes. Requires `EmailRegisterRequest.name` to become optional (proto change), plus a server-side state for "user exists but has no display name yet."
- **Pros:** Consistent — every new user goes through the same "set your name" beat regardless of method.
- **Cons:** Touches the proto and server, expanding blast radius well beyond a UI tweak. Doesn't align with the issue, which explicitly groups name+email+password together for the email path. Out of scope.
- **Effort:** Large.

## Recommendation

**Option A.** The user has explicitly chosen this for tap-count reasons. It also has the smallest blast radius — no router work, no new screens, no parameter threading. The risks are containable: the expander pattern is well-understood (Flutter's `ExpansionTile` or a simple animated `Visibility`/`AnimatedSize` is enough), and form validation can be gated on the expanded state.

The one design judgment to make carefully is whether to use an `ExpansionTile` (which has a built-in chevron and standard expand animation) or a custom `TextButton` toggle with `AnimatedSize`. Recommendation in implementation: start with `ExpansionTile` for the standard look-and-feel, and only build a custom toggle if the tile's styling clashes with the surrounding buttons.

**Architecture note (out of scope but worth flagging).** Both screens currently violate the MVVM boundary in `docs/client/architecture.md` — `_handleLogin` and `_handleRegister` are `StatefulWidget` methods that call `authServiceProvider` (a service) directly, with no ViewModel. The doc's rule is "ViewModels are the ONLY layer that calls Repositories" and widgets call ViewModels. This plan **preserves** the existing structure to keep the change scoped to layout; we are not introducing new architecture violations, but we are also not fixing the existing one. If a future PR rewrites these screens against an `AuthLoginNotifier` / `AuthRegisterNotifier`, that work should land separately. For this PR, `setState` on `_emailExpanded` is fine — the architecture doc explicitly permits `setState` for "local UI state like text field focus," which an expander toggle is.

Option C remains interesting long-term but isn't what this issue asks for. If we want unified post-signup onboarding, that should be its own issue.

## Open Questions

- **Should the expander default to collapsed or pre-expanded if the user has previously used email?** — Recommendation: always collapsed. We don't currently persist "last used auth method" anywhere, and adding storage for that is scope creep. Users who use email habitually pay one extra tap; that's the explicit goal.
- **When the email expander is collapsed, should its form fields participate in `Form` validation?** — Recommendation: no. Wrap the email fields in a `Visibility(maintainState: false, ...)` or conditionally include them in the widget tree based on `_emailExpanded`. Otherwise the `_isFormValid` getter on Register will block the submit button even when the user intends to use Google or phone.
- **How does the primary "submit" button behave when the email expander is collapsed?** — Recommendation: it shouldn't exist when collapsed. The "Login with Email" / "Register" submit button lives *inside* the expander, alongside the form. The chooser area at the top has only Phone and Google buttons; tapping the email expander reveals fields + submit button as a unit.
- **Where does "Forgot password?" live?** — Recommendation: inside the email expander on the Login screen (it only applies to the email path).
- **For the Register screen, does the existing "you're invited" status card stay above the chooser buttons?** — Recommendation: yes. It's a precondition for any registration method, not an email-specific concern.
- **For the Login screen, where does the "New to Ripls? Invitation required" support card live?** — Recommendation: stays at the bottom of the screen, below the email expander. It applies to all three methods.
- **Localization: do we need new ARB strings?** — Recommendation: most strings already exist (`loginWithPhone`, `loginWithGoogle`, `registerWithPhone`, `loginWithEmail`, `registerButton`). Likely net-new: a header for the expander like "Sign in with email" / "Sign up with email" if the existing button labels read awkwardly as section headers. Per `docs/client/i18n.md`, every new key must be added to **both** `app_en.arb` and `app_es.arb` (with `@key` metadata in en) before running `npm run generate`; CI lint (`npm run lint:dart:i18n`) blocks hardcoded strings. Use the existing `login*` / `register*` namespace, not `a11y*` (the `a11y` prefix is reserved for accessibility-only labels per `docs/client/accessibility.md`).
- **Analytics: should we track expander-open events?** — Recommendation: yes, lightweight — emit a tap event with `method: email` when the user opens the expander, so we can measure how often users still pick email after the redesign. The existing `LoginSuccessEvent` / `LoginFailedEvent` (in `app/lib/core/observability/events.dart`) already use a `method` string field, so the new event should follow that exact pattern for consistency.
- **Accessibility: is `ExpansionTile` properly announced by VoiceOver / TalkBack?** — Recommendation: verify during Phase 3. `ExpansionTile` is generally accessible, but if the announcement is poor wrap the toggle in `Semantics(button: true, expanded: _expanded, ...)`. Any custom toggle (Option A's fallback path with `TextButton` + `AnimatedSize`) must use `accessibleDuration(context, ...)` for the animation duration so reduce-motion users get `Duration.zero` (per `docs/client/accessibility.md`). If the toggle ends up as a tappable widget rather than a button, use the `Tappable` primitive from `app/lib/presentation/widgets/accessibility/` rather than `GestureDetector` / `InkWell` — the `avoid_raw_gesture_detector` lint blocks the latter.

## Implementation Plan

### Phase 1: Login screen reorder + expander

**Goal:** `login_screen.dart` shows Phone and Google as primary actions at the top, with the email form collapsed behind an expander.

**Tasks:**

- [ ] In `login_screen.dart`, restructure the build method: header → `_buildPhoneLoginButton` (re-styled as primary `ElevatedButton.icon`) → `GoogleSignInButton` → "OR" divider → expander containing `_buildEmailField`, `_buildPasswordField`, `_buildForgotPasswordLink`, `_buildErrorMessage`, `_buildLoginButton` → "New to Ripls?" support card.
- [ ] Add a `bool _emailExpanded = false` state field. Use `ExpansionTile` (or a `TextButton` + `AnimatedSize` if `ExpansionTile` styling clashes) keyed off `_emailExpanded`.
- [ ] Conditionally include the email form widgets in the tree only when `_emailExpanded` is true (use `Visibility(maintainState: false)` or an `if` gate). This prevents `Form` validation from running on the hidden fields.
- [ ] Move the `_handleLogin` error display so `_errorMessage` only shows when the expander is open (errors from the email path are only meaningful in that context). Phone/Google errors continue to display in their own areas as today.
- [ ] If introducing any new strings (e.g. an expander header), add the keys to `app/lib/l10n/app_en.arb` (with `@key` metadata) **and** `app/lib/l10n/app_es.arb`, then run `npm run generate`. Use the `login*` namespace (not `a11y*`).
- [ ] Add a `LoginMethodSelectedEvent` to `app/lib/core/observability/events.dart` following the same `method` field shape as the existing `LoginSuccessEvent` / `LoginFailedEvent`. Emit `method: email` when the expander opens, plus `method: phone` and `method: google` on those button taps.
- [ ] Update `LoginScreen` widget tests: verify the initial state shows Phone + Google but not the email form, that tapping the expander reveals the email fields, that the existing `Key('login_email_field')` / `Key('login_password_field')` still resolve once expanded, and that submit-when-collapsed is impossible (no submit button visible). Note: per `docs/client/testing.md` the project's primary testing focus is ViewModels, not widgets — but this change is intentionally widget-state-only (expander toggle via `setState`), so widget tests are the correct layer here. Use the `MaterialApp` wrapper with `localizationsDelegates` from `app/test/helpers/l10n_helpers.dart`.

**Validates:** Login still works end-to-end for all three methods. Hierarchy matches the issue's intent. Existing autofill/credential-save behavior survives.

### Phase 2: Register screen reorder + expander

**Goal:** `register_screen.dart` (the `widget.shortCode != null` branch) leads with Phone and Google, with the name+email+password+confirm form collapsed behind an expander.

**Tasks:**

- [ ] In `register_screen.dart`, restructure the `widget.shortCode != null` branch: invitation status card → `_buildPhoneRegisterButton` (primary) → `GoogleSignInButton` → "OR" divider → expander containing `_buildNameField`, `_buildEmailField`, `_buildPasswordField`, `_buildConfirmPasswordField`, `_buildErrorMessage`, `_buildRegisterButton`.
- [ ] Add a `bool _emailExpanded = false` state field; same pattern as Phase 1.
- [ ] Conditionally include email form widgets only when expanded so `_isFormValid` doesn't block on empty hidden fields, and so the email register button doesn't render when collapsed.
- [ ] Confirm the existing `_handleRegister` post-success deep-link forwarding (`gearId` / `requestId` / `experienceId`) is unchanged — the refactor is layout-only.
- [ ] Leave the `widget.shortCode == null` branch (invitation-required + manual-code-entry) unchanged. That screen is about getting an invite, not about choosing a registration method.
- [ ] If introducing any new strings, add to both `app_en.arb` and `app_es.arb` under the `register*` namespace, then run `npm run generate`.
- [ ] Add a `RegisterMethodSelectedEvent` to `app/lib/core/observability/events.dart` with `method: phone|google|email`, mirroring the existing event shape. Emit on chooser button taps and on email expander open.
- [ ] Update `RegisterScreen` widget tests: verify the initial post-invite state shows Phone + Google but not name/email/password fields, that tapping the expander reveals all four fields, and that deep-link params still flow through to `_handleRegister` after a successful email signup. Use the localization-aware test wrapper from `app/test/helpers/l10n_helpers.dart`.

**Validates:** All three registration methods still work end-to-end. Invitation status card still appears. Phone path still collects name post-OTP (no change — verifies the phone screen wasn't affected).

### Phase 3: Polish + verification

**Goal:** Confirm the new flow feels right and nothing regressed.

**Tasks:**

- [ ] Manual smoke test on iOS simulator and Android emulator: phone register, Google register, email register (with expander), phone login, Google login, email login (with expander), forgot password (from inside expander), manual invite code entry, deep link from `/go/CODE` → register chooser → each method.
- [ ] Verify accessibility: VoiceOver/TalkBack announces the expander state correctly ("collapsed, double-tap to expand" / "expanded"); tab order on the chooser is phone → Google → email expander; screen reader announces the screen title correctly.
- [ ] Run `npm run lint:dart:a11y` to confirm no new accessibility violations (the allowlist at `scripts/a11y_allowlist.txt` is empty — any violation introduced here must be fixed in the same PR).
- [ ] Run `npm run lint:dart:i18n` to confirm no hardcoded user-visible strings.
- [ ] Run `npm run lint:nav` to confirm `context.push()` usage stays scoped to auth flows (the only place the project allows it).
- [ ] If a custom toggle (not `ExpansionTile`) ends up being used, confirm any `Duration(...)` argument is wrapped in `accessibleDuration(context, ...)` so reduce-motion users get instant transitions; the `prefer_accessible_duration` lint rule enforces this.
- [ ] Confirm `LoginSuccessEvent` and analogous events still fire with the correct `method` value on completion.
- [ ] Confirm the expander animation looks correct on both iOS and Android — no jarring jumps, scroll position holds when the user expands at the bottom of the viewport.
- [ ] Skim `docs/registration_and_login.md` for any UI ordering claims that need updating; edit if so.

**Validates:** Production-ready — the change ships without regressions and the docs reflect reality.

## Related Issues

- None linked from the issue itself. If the team later wants Option C (unified post-signup name prompt), that would be a follow-up issue building on this work.

## Audit Notes (2026-05-01)

The plan was reviewed against `docs/client/architecture.md`, `docs/client/i18n.md`, `docs/client/accessibility.md`, and `docs/client/testing.md`. Changes applied in-place:

- **i18n compliance.** Open Questions and both Phase 1 / Phase 2 task lists now require any new ARB keys to land in **both** `app_en.arb` and `app_es.arb` before `npm run generate`, and steer keys into the `login*` / `register*` namespaces (the `a11y` prefix is reserved for accessibility-only labels). Phase 3 adds an explicit `npm run lint:dart:i18n` check.
- **Accessibility primitives.** Open Questions now spells out: use `accessibleDuration(context, ...)` for any custom-toggle animation duration (else the `prefer_accessible_duration` lint fails), and use the `Tappable` primitive instead of raw `GestureDetector` / `InkWell` if the toggle is non-button (the `avoid_raw_gesture_detector` lint fails). Phase 3 adds an explicit `npm run lint:dart:a11y` check and notes the allowlist is empty.
- **Architecture boundary called out.** A new paragraph in Recommendation flags that the existing `_handleLogin` / `_handleRegister` widget-to-service path violates MVVM ("ViewModels are the ONLY layer that calls Repositories") but is intentionally preserved as out-of-scope. Confirms `setState` on `_emailExpanded` is permitted by the architecture doc as local UI state.
- **Analytics consistency.** Phase 1 / Phase 2 tasks now point at `app/lib/core/observability/events.dart` and require the new method-selected events to mirror the existing `LoginSuccessEvent` / `LoginFailedEvent` `method` field shape rather than inventing a new pattern.
- **Testing layer expectations.** Phase 1 / Phase 2 acknowledge that `docs/client/testing.md` prioritizes ViewModel tests, but justify widget tests here because the change is widget-state-only. Both phases now reference `app/test/helpers/l10n_helpers.dart` for the localization-aware test wrapper.
- **Navigation lint.** Phase 3 adds `npm run lint:nav` to confirm the existing `context.push('/forgot-password')` usage (which lives inside the email expander) stays inside its allowed auth-flow exemption.

No changes were needed to caching, repositories, or server boundaries — this issue is purely a client-side UI reorder with no data-layer impact.
