# Issue #1289: [testing] auth_repository (Flutter) has no tests -- recurring 142+ days

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1289
**Filed:** 2026-04-25 | **Last activity:** 2026-04-25
**Labels:** auto-filed, review:testing, claude-fix | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`AuthRepository` has no unit tests despite handling authentication operations that
could lock users out or silently fail on token refresh. The repository currently
exposes only `checkInvitation`, while `AuthService` (which the repository wraps)
implements the full auth surface: password login/registration, OIDC, phone, token
refresh, and password reset. Screens bypass the repository and call `AuthService`
directly for most operations. This flagged gap has recurred in at least two prior
reviews (142+ days old).

> **Architectural violation (audit finding):** `docs/client/architecture.md` Rule 2
> states "Repositories are the ONLY layer that calls Services — ViewModels never
> access services directly." Seven screens currently read `authServiceProvider`
> directly, bypassing `AuthRepository` entirely. Adding the missing repository
> methods (Option B) does not fix the violation — screen migration is required
> separately. This issue focuses on the testing gap; the architectural violation
> should be tracked as a follow-up.

## Context from Comments

One triage comment (maintainer, 2026-04-24) confirmed this as P2, classifying it
as a testing/security gap. No disagreements or alternative directions were raised.
The issue is straightforward: add tests.

## Potential Solutions

### Option A: Test only the existing `checkInvitation` method
- **Approach:** Write `auth_repository_test.dart` covering only what `AuthRepository`
  currently exposes (`checkInvitation`) — valid result, invalid result, ConnectException,
  and generic exception paths.
- **Pros:** Minimal scope; no production code changes.
- **Cons:** Does not cover login, token refresh, or session expiry scenarios as the
  issue explicitly requires. Leaves the test suite thin and the security gap open.
- **Effort:** Small.

### Option B: Add key auth operations to `AuthRepository`, then test them all
- **Approach:** Add `loginWithPassword`, `registerWithPassword`, and `refreshToken`
  delegating to `AuthService` (same pass-through pattern as `checkInvitation`). Write
  tests covering success, `ConnectException` (invalid credentials / session expiry), and
  network failures for all methods. Screens continue using `authServiceProvider` directly
  — the repository additions are additive only.
- **Pros:** Meets the issue's stated coverage requirements (login, token refresh, session
  expiry, invalid credentials, network failures). Brings the repository closer to being
  the consistent auth surface described in the issue. No breaking changes.
- **Cons:** Adds methods to the repository that screens don't use yet; could cause mild
  confusion until screens are migrated. Slightly larger change.
- **Effort:** Small-Medium.

### Option C: Full migration — move all auth calls in screens to use `AuthRepository`
- **Approach:** Option B plus updating every screen that calls `authServiceProvider` to
  use `authRepositoryProvider` instead.
- **Pros:** Achieves the clean architecture the repository pattern promises.
- **Cons:** Touches many screens (7+ files); risk of regression; out of scope for a
  testing task. Should be its own issue.
- **Effort:** Large.

## Recommendation

**Option B.** Adding the three missing pass-through methods (`loginWithPassword`,
`registerWithPassword`, `refreshToken`) is a small, additive change that makes the test
coverage meaningful. All three follow the identical pattern already used by
`checkInvitation`: call the service, return the result. Tests for all four methods cover
the security scenarios the issue demands. Screen migration (Option C) is deferred as a
separate concern.

## Open Questions

- **Should OIDC and phone auth methods also be added to `AuthRepository`?**
  Recommendation: No — add only the password and refresh operations that the issue
  specifically names. OIDC/phone can follow in a separate PR if needed.
- **Should tests also cover `requestPasswordReset`, `checkResetPasswordToken`, and
  `resetPassword`?**
  Recommendation: Add them to the repository for completeness (they follow the same
  pass-through pattern) and write basic success/failure tests. Password reset flows are
  user-facing and security-relevant.

## Implementation Plan

### Phase 1: Expand `AuthRepository`
**Goal:** Expose the full auth operation set through the repository.
**Tasks:**
- [ ] Add `loginWithPassword` delegating to `_service.loginWithPassword`
- [ ] Add `registerWithPassword` delegating to `_service.registerWithPassword`
- [ ] Add `refreshToken` delegating to `_service.refreshToken`
**Validates:** Repository covers all operations the test suite needs to exercise.

### Phase 2: Write and generate tests
**Goal:** Achieve full test coverage for all `AuthRepository` methods.
**Tasks:**
- [ ] Create `app/test/data/repositories/auth_repository_test.dart` with `@GenerateMocks([AuthService])`
- [ ] Cover `checkInvitation`: valid, invalid, `ConnectException`, generic exception
- [ ] Cover `loginWithPassword`: success, `ConnectException` (invalid credentials), generic exception (network failure)
- [ ] Cover `registerWithPassword`: success, `ConnectException`, generic exception
- [ ] Cover `refreshToken`: success, `ConnectException` (session expiry), generic exception
- [ ] Run `flutter pub run build_runner build` to generate mocks
- [ ] Run `flutter test --reporter expanded app/test/data/repositories/auth_repository_test.dart`

## Related Issues
- None referenced in issue comments.
- **Follow-up needed:** Migrate auth screens from `authServiceProvider` to
  `authRepositoryProvider` to satisfy `docs/client/architecture.md` Rule 2.
  That migration should be a separate issue/PR.

---
*Audit applied 2026-04-25: Added architectural violation note (screens bypass repository layer) per docs/client/architecture.md Rule 2. Screen migration deferred as follow-up.*
