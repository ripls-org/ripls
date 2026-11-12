# Issue #1528: [security] Access token still stored in plaintext SharedPreferences

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1528
**Filed:** 2026-04-28 | **Last activity:** 2026-04-30
**Labels:** auto-filed, review:security | **Assignees:** unassigned
**Context docs:** `docs/reviews/reports/security_review.md`

## Problem Statement

The access token (~1 hour TTL JWT) is persisted to `SharedPreferencesAsync` in
`app/lib/services/auth_state.dart`. On Android this lands in
`/data/data/<pkg>/shared_prefs/*.xml` as plaintext; on iOS it lives inside the
app sandbox `NSUserDefaults` plist. Either is recoverable by an attacker with
root/jailbreak access or via OS backup extraction (Android Auto Backup, iOS
unencrypted iTunes backups). For up to one hour after extraction, the value is
a valid bearer for every authenticated RPC.

The refresh token migration to `FlutterSecureStorage` already shipped (see
`_secureStorage.read/write(key: _refreshTokenKey)` at lines 290, 347, 368, 417),
so the encrypted-storage seam is in place — the fix completes a partial
migration.

The four call sites that touch `'access_token'` via `_prefs`:

- **read on cold start** — `loadAuthState` at line 197
- **write on login** — `setAuthState` at line 286
- **write on refresh** — `refreshAccessToken` at line 365
- **delete on logout** — `logout` at line 413 (the explicit `_prefs.remove`;
  the subsequent `prefs.clear()` at line 435 also clears it)

## Context from Comments

The issue is auto-filed by `/ripls-reviews` and tagged with the
fingerprint `review:security:access-token-in-sharedprefs` (so the next review
run will recognise the fix and stop re-filing).

**Triage update (2026-04-28, maintainer):** Type=Task, Priority=P2.
Rationale recorded as "security Medium: pre-distribution-stage acceptable;
access token in plaintext SharedPreferences."

The original review report (`docs/reviews/reports/security_review.md` §2,
lines 124–130) gives the explicit fix:

> Move `'access_token'` reads/writes to `_secureStorage`. Add a one-shot
> migration on read: if a value exists in SharedPreferences, copy it to
> secure storage and clear the SharedPreferences key.

The same finding has been open 146 days at the time of the latest review.

## Potential Solutions

### Option A: Migrate access token to FlutterSecureStorage with one-shot read migration

- **Approach:** Introduce a new constant `_accessTokenKey = 'access_token'`
  and switch all four call sites in `auth_state.dart` from `_prefs` to
  `_secureStorage`. In `loadAuthState`, after reading from secure storage,
  if the value is null, fall back to a one-time read of the legacy
  SharedPreferences key. If found, copy it into secure storage and remove
  the legacy key. In `logout`, add an explicit `_secureStorage.delete(...)`
  alongside the existing `_secureStorage.delete(_refreshTokenKey)` call;
  keep the legacy `_prefs.remove('access_token')` for one release cycle as
  a belt-and-suspenders cleanup.
- **Pros:** Matches the security review recommendation verbatim. Reuses the
  storage seam that the refresh-token migration already validated. No
  user-facing impact — existing sessions migrate transparently on the next
  app launch. Minimal blast radius (single file, ~20 LOC delta).
- **Cons:** Adds one extra storage round-trip on cold start when the
  legacy key is unset (a single null check). FlutterSecureStorage on web
  falls back to `localStorage` and is not meaningfully more secure than
  SharedPreferences there — but the app's threat model is mobile-first and
  the refresh-token migration already accepts this trade-off.
- **Effort:** Small.

### Option B: Wait for the #1450 injection-seam refactor to land first

- **Approach:** PR for #1450 (branch `claude/fix-issue-1450-25021528965`,
  commit `b977437ec`) introduces `sharedPreferencesAsyncProvider` and
  `secureStorageProvider` so tests can override storage without platform
  channel mocks. Wait for it to merge, then build this fix on top with
  proper unit-test coverage of the migration path.
- **Pros:** Cleaner test seam — can verify the migration via Mockito
  rather than `setMockInitialValues` platform channel calls.
- **Cons:** Couples timeline of two open security fixes. #1450 has been
  open for several days already. Test coverage of the migration is not
  blocked by the seam — `SharedPreferences.setMockInitialValues` and
  `FlutterSecureStorage` mock platform-channel handlers both work today
  (see `app/test/services/selected_community_test.dart` for the
  SharedPreferences pattern). The injection seam from #1450 will rebase
  cleanly on top of this change anyway.
- **Effort:** Same eventual code change, but blocked on another PR.

### Option C: Encrypt the access token in place using a derived key

- **Approach:** Keep using SharedPreferences but encrypt the value with a
  symmetric key stored in FlutterSecureStorage.
- **Pros:** No SharedPreferences key change, so no migration needed.
- **Cons:** Reinvents what `FlutterSecureStorage` already provides via
  Keychain (iOS) and Keystore-backed EncryptedSharedPreferences (Android).
  Adds new crypto code to maintain. No upside over Option A.
- **Effort:** Medium.

## Recommendation

**Option A.** It is exactly what the security review recommends, follows the
established refresh-token pattern in the same file, and is the smallest
focused change that closes the finding. The migration semantics are simple
and self-healing (re-attempted on every cold start until the legacy key is
gone). No reason to wait on #1450 — that PR will rebase on top of this with
a one-line provider swap.

## Open Questions

- **Should the legacy `_prefs.remove('access_token')` line in `logout` stay,
  or should it be deleted now?** — Recommendation: **keep it for now**. The
  cost is one no-op call on devices that already migrated; the benefit is
  that any user who never opened the app between now and the next logout
  still gets the legacy key cleared. Remove it in a follow-up after one
  release cycle.

- **What if both the secure-storage value and the legacy SharedPreferences
  value exist on read (e.g., the user opened an old build, then a new
  build, then an old build again)?** — Recommendation: **trust the secure
  storage value and clear the legacy key**. The user can only have a single
  active session, and the refresh-token rotation logic already prefers the
  most recent write. Clearing the legacy key on every cold start where it
  exists is idempotent.

- **Should this also migrate `'user'` and `'server_url'` to secure
  storage?** — Recommendation: **no, out of scope**. The user JSON contains
  only the user's own id/name/mediaId (already exposed via every authed
  RPC) and the server URL is not a credential. Expanding scope would slow
  the security fix and is not what the review recommends.

- **What about the JWT-decode path in `_parseJwtExpiry`? Does anything
  else read `'access_token'` from SharedPreferences?** — Recommendation:
  **no — confirmed by grep**. Only the four sites in `auth_state.dart`
  touch the key (`mapbox_location_service.dart` uses
  `'access_token'` as a query parameter for Mapbox, unrelated; the
  generated proto file references it as a field name; tests use string
  literals). No other reader needs updating.

- **Test approach without the #1450 injection seam?** — Recommendation:
  **use `SharedPreferences.setMockInitialValues` plus a
  `FlutterSecureStorage` platform channel mock**. The
  `selected_community_test.dart` file shows the SharedPreferences pattern;
  for FlutterSecureStorage we use
  `TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(MethodChannel('plugins.it_nomads.com/flutter_secure_storage'), ...)`
  to back the storage with an in-memory map. If even that proves brittle,
  fall back to a focused integration-style test that only exercises the
  migration logic via a thin wrapper, and document the gap.

## Implementation Plan

### Phase 1: Migrate read/write/delete sites and add migration

**Goal:** Access token stops being written to SharedPreferences; existing
sessions migrate transparently on next cold start.

**Tasks:**

- [ ] Add `static const _accessTokenKey = 'access_token';` next to
  `_refreshTokenKey` in `AuthStateNotifier`.
- [ ] In `loadAuthState` (line 197): replace
  `final accessToken = await _prefs.getString('access_token');` with a
  read from `_secureStorage.read(key: _accessTokenKey)`. If the result is
  null, attempt `_prefs.getString('access_token')`; if that returns
  non-null, write it to secure storage via
  `_secureStorage.write(key: _accessTokenKey, value: legacy)` and remove
  the legacy key with `_prefs.remove('access_token')`. Use the migrated
  value as the effective `accessToken` for the rest of the function. Log
  at INFO when a migration runs.
- [ ] In `setAuthState` (line 286): replace
  `await _prefs.setString('access_token', accessToken);` with
  `await _secureStorage.write(key: _accessTokenKey, value: accessToken);`.
- [ ] In `refreshAccessToken` (line 365): replace
  `await _prefs.setString('access_token', result.accessToken!);` with
  `await _secureStorage.write(key: _accessTokenKey, value: result.accessToken!);`.
- [ ] In `logout` (line 413): add
  `await _secureStorage.delete(key: _accessTokenKey);` next to the
  existing `_secureStorage.delete(key: _refreshTokenKey)` call. Keep the
  existing `await _prefs.remove('access_token');` for one release cycle as
  belt-and-suspenders cleanup of any pre-migration value.
- [ ] Verify `_parseJwtExpiry` and the proactive-refresh timer continue to
  work — they consume the in-memory `state.accessToken`, not storage.

**Validates:** Cold-start sessions for an existing user keep working
(migration runs); fresh logins write only to secure storage; logout clears
both locations.

### Phase 2: Add unit tests

**Goal:** Prevent regression on the four call sites and the migration
path.

**Tasks:**

- [ ] In a new `AuthStateNotifier` test group in
  `app/test/services/auth_state_test.dart`, set up a `ProviderContainer`
  per the [client testing convention](../client/testing.md) — including a
  `tearDown` that calls `container.dispose()` — and override the
  `authServiceProvider` (and any other minimum dependencies needed by
  `loadAuthState` / `setAuthState` / `logout`) with Mockito mocks
  generated via `@GenerateMocks`. Run
  `flutter pub run build_runner build` to refresh `*.mocks.dart`.
- [ ] In `setUp`, reset both storage backends:
  - `SharedPreferences.setMockInitialValues({})` (existing pattern from
    `app/test/services/selected_community_test.dart`).
  - Wire a `setMockMethodCallHandler` for the
    `plugins.it_nomads.com/flutter_secure_storage` channel backed by an
    in-memory `Map<String, String>`. Handle the `read`, `write`,
    `delete`, `readAll`, and `deleteAll` method calls. In `tearDown`,
    null out the handler.
- [ ] Test: when secure storage is empty and SharedPreferences contains a
  legacy `'access_token'` (plus a matching `'server_url'` and `'user'`
  JSON), `loadAuthState` migrates the token to secure storage, removes
  the legacy key, exposes the token as `state.accessToken`, and ends in
  the authenticated state.
- [ ] Test: when secure storage already has a value, `loadAuthState`
  uses it and ignores any legacy SharedPreferences value, and clears the
  legacy key as cleanup.
- [ ] Test: when both storages are empty, `loadAuthState` ends in
  `AuthStateData.empty` (unauthenticated).
- [ ] Test: `setAuthState` writes the access token only to the secure
  storage in-memory map (not to SharedPreferences).
- [ ] Test: `logout` clears the access token from the secure storage
  in-memory map (alongside the existing `refresh_token` clear).
- [ ] Test: a write failure on secure storage propagates as an exception
  (matches the existing setAuthState contract; do not silently swallow).
- [ ] If the FlutterSecureStorage mock channel turns out to be brittle on
  this Flutter version, document the gap in a test-skipped note rather
  than ship without coverage; keep the SharedPreferences-only tests.

**Validates:** Migration is correct and idempotent; the four call sites
behave as expected; no future change can silently move the access token
back to SharedPreferences without a test failure.

### Phase 3: Update internal docs

**Goal:** Keep contributor-facing docs in sync with where the access
token actually lives.

**Tasks:**

- [ ] Update `docs/client/logout.md` step 2 of the teardown sequence:
  change "Remove access_token from SharedPreferences" to "Delete
  access_token from SecureStorage" so the diagram reflects the new
  storage location. Leave the legacy `_prefs.remove('access_token')`
  cleanup line in code (and as a parenthetical in the doc) until it is
  removed in a follow-up.
- [ ] No proto-comment changes — none of this affects the API surface.
  No contributor-facing vendor-name changes — `FlutterSecureStorage` is
  the actual Flutter package and is already named in code comments per
  the project convention for internal docs.

**Validates:** Logout teardown documentation matches the implementation.

### Phase 4: Build and verify

**Goal:** Lint, build, and full test suite pass.

**Tasks:**

- [ ] `cd app && flutter analyze`
- [ ] `cd app && flutter test --reporter expanded` (or
  `--reporter github` from CI). Must pass full suite.
- [ ] `npm run lint:dart:size` to confirm no file-size regression in
  `auth_state.dart` (currently ~490 lines; the plan adds ~20 LOC, well
  under the 1,000-line gate).
- [ ] Verify the redactor (`app/lib/core/observability/logging/redactor.dart`
  per `app/test/core/observability/logging/redactor_test.dart`) still
  masks any log line that includes `access_token` — the migration log
  must not include the raw token value (log only that a migration ran,
  not the value).
- [ ] Smoke-test on Android emulator: log in on a build with the legacy
  key already in SharedPreferences (simulate by manually seeding the
  shared-prefs XML), confirm the next launch migrates and authenticates.
  If emulator access is not available, document the manual test steps in
  the PR description for the human reviewer.

**Validates:** No CI regressions; no PII leakage in logs; the security
finding is closed.

## Related Issues

- **#1450** (in flight) — adds Riverpod provider injection seams for
  `SharedPreferencesAsync` and `FlutterSecureStorage` in
  `auth_state.dart`. This fix is independent and will rebase cleanly on
  top of #1450.
- **#1455** — merged auth refresh-token resilience PR that established
  the FlutterSecureStorage usage pattern this fix extends.
- The other open security findings tracked alongside this one in the
  April 30 review (per `docs/reviews/reports/security_review.md`) are
  out of scope: hardcoded JWT secret, GetMedia ACL, GetUser/FindUserByEmail
  enumeration, no rate limiting, password-reset token invalidation, debug
  router diagnostics.

---

## Audit Notes

**Audited against:** `docs/client/architecture.md`,
`docs/client/testing.md`, `docs/client/caching.md`,
`docs/client/observability.md`, `docs/client/logout.md`,
`docs/client/startup.md`.

**Findings & changes applied:**

1. **MVVM layering (OK):** The change is confined to
   `app/lib/services/auth_state.dart`, the existing service-level
   Notifier. No ViewModels, repositories, or screens are touched.
   Storage is a service concern — correct layer.

2. **Test layering (addressed):** The original Phase 2 referenced
   platform mocks but did not specify the `ProviderContainer` setup.
   Updated to require `ProviderContainer` with provider overrides and
   `tearDown` disposal per `docs/client/testing.md`. Existing
   `auth_state_test.dart` only covers `AuthStateData`; the new tests
   exercise `AuthStateNotifier` for the first time.

3. **Documentation drift (added Phase 3):** `docs/client/logout.md`
   currently says "Remove access_token from SharedPreferences" — that
   line will be wrong once the migration ships. Phase 3 adds the doc
   update so logout teardown documentation tracks the implementation.

4. **Observability / PII (added Phase 4 task):** The `LogRedactor`
   masks `access_token` keys (verified in
   `app/test/core/observability/logging/redactor_test.dart`). Phase 4
   includes an explicit verification step that the migration log line
   logs only that a migration occurred — never the token value — so PII
   does not leak even if the redactor regresses.

5. **No caching impact:** The access token is a session credential,
   not cacheable data. `cacheManagerProvider` and the Stash repository
   layer are not touched.

6. **No proto / API contract change:** Protocol Buffers and RPC
   contracts are unaffected. Vendor-name conventions (proto comments
   forbid vendor names; READMEs and `.dart` code allow them) are
   respected — `FlutterSecureStorage` is referenced only in code
   comments and contributor docs.

7. **No i18n / a11y impact:** No user-facing strings are added or
   changed.

8. **Risk surface (small and contained):** Single file edit + tests +
   one doc update. The migration is idempotent and self-healing on
   every cold start until the legacy key is gone.
