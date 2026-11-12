# Issue #378: Need a button to get back to login from registration screen

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/378
**Filed:** 2025-11-28 | **Last activity:** 2026-03-05
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When a user taps an invitation link and lands on the registration screen, there
is no way to navigate back to the login screen. The only "Back to Login" button
that exists today appears when registration is accessed *without* a short code
(the "Invitation Required" fallback at line 519 of `register_screen.dart`). But
in the normal flow — user taps invite link → `InvitationLandingScreen` →
"Create a new account" → `RegisterScreen` with a valid short code — there is no
back-navigation affordance at all.

This blocks existing users who already have an account but tapped "Create a new
account" by mistake, as well as new users who realize they already registered
and want to log in instead.

## Context from Comments

- **Triage (2026-03-04) — P1:** Maintainer flagged this as blocking the
  onboarding flow for all personas. Users who tap "Register" by mistake have no
  way back to Login.

No other comments or discussion. The issue body is empty, so all context comes
from the title and the triage comment.

## Potential Solutions

### Option A: Add a "Already have an account? Log in" link to the registration form

- **Approach:** Add a text button at the bottom of the registration form (below
  the "Register" / Google sign-in buttons) that navigates to `/login`. Preserve
  the invitation context by passing `from=/register?token={shortCode}` so the
  user can return to registration after logging in if needed — though in
  practice, logging in means they're done registering.
- **Pros:** Minimal change, follows a near-universal UX pattern (every major app
  puts "Already have an account?" on the registration form). Discoverable,
  low-risk.
- **Cons:** None significant. This is table-stakes UX.
- **Effort:** Small.

### Option B: Add an AppBar with a back button

- **Approach:** Add a leading back-arrow in the AppBar of `RegisterScreen` that
  pops the navigation stack or goes to `/login`.
- **Pros:** Provides standard mobile back-navigation. Works regardless of scroll
  position.
- **Cons:** The registration screen currently has no AppBar (it's a full-bleed
  form). Adding one changes the visual design. The AppBar back button is also
  ambiguous — does it go to `InvitationLandingScreen` or to `/login`? If the
  user came from a deep link, there may be nothing to pop back to.
- **Effort:** Small, but requires design consideration for the AppBar.

### Option C: Both — AppBar back + inline text link

- **Approach:** Add both an AppBar back button (navigating to the invitation
  landing or login) and an "Already have an account?" link in the form body.
- **Pros:** Maximum discoverability. Covers users who expect top-nav and users
  who scroll to the bottom.
- **Cons:** Two navigation targets that go to potentially different places
  (landing vs. login) could confuse users. Over-engineered for the problem.
- **Effort:** Small-medium.

## Recommendation

**Option A** — Add an "Already have an account? Log in" link at the bottom of
the registration form. This is the simplest, most conventional solution. It
matches the pattern used by virtually every app with login/registration flows.
No AppBar changes needed, no ambiguity about where "back" goes.

The link should navigate to `/login` directly. There's no need to preserve the
invitation token in the login redirect because:
1. If the user already has an account, they don't need the invitation.
2. If they log in and are already a member of the community, they'll land on
   the home screen as expected.

## Open Questions

- **Should the link also appear when the registration form shows the "Invitation
  Required" fallback (no short code)?** — Recommendation: Yes, but it already
  does (line 519). No change needed for that path.

- **Should we also add a back link on `InvitationLandingScreen`?** —
  Recommendation: Not in this issue. `InvitationLandingScreen` already has
  both "Create a new account" and "I already have an account" buttons, so
  the navigation gap is only on `RegisterScreen` itself.

## Implementation Plan

### Phase 1: Add "Already have an account?" link to RegisterScreen

**Goal:** Give users a clear path from the registration form back to login.

**Tasks:**
- [ ] In `app/lib/presentation/screens/auth/register_screen.dart`, add an
      "Already have an account? Log in" row below the registration action
      buttons (after the Google sign-in button, around line 470). Use a `Row`
      with a `Text` label and a `TextButton` that calls `context.go('/login')`.
- [ ] Ensure the link appears in both the valid-invitation form and the
      no-invitation fallback (the fallback already has one, so just verify).
- [ ] Style the link consistently with the existing "Back to Login" button in
      the no-invitation fallback (same font size, color, alignment).
- [ ] Write a widget test that verifies: (a) the "Log in" link is present when
      a valid short code is provided, and (b) tapping it navigates to `/login`.

**Validates:** Users can navigate from registration back to login in the normal
invitation flow.

## Related Issues

None identified.
