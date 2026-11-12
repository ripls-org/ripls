# Issue #1518: [error_handling] Sanitize raw exception messages shown to users in ViewModels

**Source:** <https://github.com/Type2Fun-Foundation/ripls/issues/1518>
**Filed:** 2026-04-28T16:16:40Z | **Last activity:** 2026-04-29T14:17:45Z
**Labels:** auto-filed, review:error_handling | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Across `app/lib/presentation/viewmodels/`, ~67 call sites in 30 files
assign `e.toString()` (or `'Failed to X: ${e.toString()}'`) directly to a
user-visible `errorMessage` / `mutationError` / `…Error` field on view-model
state. As of this branch, a refresh count shows **34 prefixed sites**
(`errorMessage: 'Failed to X: ${e.toString()}'`) and **33 plain sites**
(`errorMessage: e.toString()`) spread across **30 files** — slightly more
than the 28 files / 67 sites the original review reported.

The pattern leaks technical text whenever the caught exception is **not** a
`ServiceException`. Concrete leaks observed today:

- A raw `ConnectException` that bypasses `RpcUtils.executeRpc` (e.g. from
  streaming code or a service that throws before reaching `executeRpc`)
  renders as `ConnectException(code: …, message: …)`.
- A `FormatException` from a Postgres-shaped error string surfaced through
  generated parsers shows the full SQL hint.
- A `StateError`, `TypeError`, or `_TypeError` from a nullability bug
  surfaces a stack-shaped string mentioning Dart internals.
- Even when the exception **is** a `ServiceException` (whose
  `toString()` already returns a friendly localized message), the
  prefix style `'Failed to X: ${e.toString()}'` produces awkward
  doubled-up phrasing such as `Failed to load transfer: The requested
  item could not be found.` and is not localized — the
  `'Failed to X: '` prefixes are hardcoded English even though
  `app/lib/l10n/app_en.arb` already carries a full set of `rpcError*`
  localizations and the app ships an `app_es.arb`.

The root cause is two-fold:

1. There is no single helper for the "exception → user-visible state"
   conversion, and no CI gate to keep the pattern from re-spreading.
2. View-model state classes store the *resolved English string* on
   `errorMessage` fields, which forces viewmodels to do localization
   work they're not equipped for. Per `docs/client/i18n.md`:
   **"Viewmodels never resolve strings. Viewmodels expose typed state
   (enums, error codes, data objects). Widgets resolve to localized
   text in `build()` via `context.l10n`."** The doc comments in
   `app/lib/core/errors/rpc_error_handler.dart` (lines 204-208 / 213-216)
   echo the same guidance.

This plan addresses both root causes together: a typed error
representation on view-model state, a classifier that converts any
caught exception into that representation, a widget-layer resolver
that turns it into localized text, the call-site migration, and a CI
gate that prevents regression.

## Context from Comments

- **2026-04-28 — maintainer / `/ripls-triage`:** classified as
  Type=Task, Priority=P1 with rationale "ViewModels surface raw
  exception messages and stack traces to users; cuts across both
  lenses." The triage doc lives at `docs/issues/triage-2026-04-28.md`.
- **2026-04-29 — maintainer:** `@claude` ping requesting work on
  this issue.
- **2026-04-29 — maintainer (this PR, comment 4345278946):**
  "i prefer option B rewrite plan to choose that one." The plan is
  rewritten accordingly: Option B (typed error state + widget-side
  localization) is now the recommended path.

## Potential Solutions

### Option A: Static helper on `RpcErrorHandler` + thin wrapper, English-only fallback

- **Approach:** Add a static
  `RpcErrorHandler.extractMessage(Object e, {AppLocalizations? l10n,
  String? fallback})` returning a `String`. Migrate every call site to
  `errorMessage: RpcErrorHandler.extractMessage(e, fallback: '<context>')`.
- **Pros:** Minimal new surface area; viewmodels stay context-free; a
  near-mechanical rewrite plus a small per-file judgment call about
  whether to keep a prefix.
- **Cons:** Strings are still English at the viewmodel layer when the
  caller hasn't threaded an `AppLocalizations`. Ships a sanitization
  win without a localization win, and consolidates a pattern
  (`String? errorMessage` resolved in the viewmodel) that
  `docs/client/i18n.md` explicitly forbids.
- **Effort:** Medium (~150 lines helper + tests, 30 file edits, one CI
  script).
- **Status:** **Deferred.** Reviewer (maintainer) prefers Option B.

### Option B: Typed error on view-model state + widget-side localization (RECOMMENDED)

- **Approach:** Stop storing a resolved string in viewmodel state.
  Instead:
  1. Define a small typed error representation
     `app/lib/core/errors/user_error.dart` covering every category
     the helper has to express today: server-supplied
     `ServiceException` message, ConnectRPC `Code`, transport
     (`socket`/`timeout`/`http`), and a generic fallback that can
     carry a context-specific English fallback string for the
     "anything weird" bucket.
  2. Add a static classifier
     `RpcErrorHandler.classify(Object e, {String? fallback}) → UserError`
     that produces the typed value from any caught exception. This
     replaces the resolved-string helper described in Option A.
  3. Add a static localizer
     `RpcErrorHandler.localize(UserError e, AppLocalizations l10n) → String`
     that resolves the typed value to localized text using the
     existing `_localizeCode` / `getTransportErrorMessage` paths.
  4. Replace `String? errorMessage` on every view-model state class
     with `UserError? error` (and corresponding fields like
     `membersError`, `eventsError`, `gearError`,
     `communitiesError`, `activitiesError`, `userError`,
     `sharingError` on the multi-error states).
  5. Migrate every viewmodel call site from
     `copyWith(errorMessage: 'Failed to X: ${e.toString()}')` to
     `copyWith(error: RpcErrorHandler.classify(e, fallback: '<English context>'))`.
  6. Migrate every widget that displays an error from
     `Text(state.errorMessage!)` to
     `Text(RpcErrorHandler.localize(state.error!, context.l10n))`,
     plus any guards from `state.errorMessage != null` to
     `state.error != null`.
  7. Add a CI lint that fails if anything matching
     `errorMessage[^\n]*\$\{e\.toString\(\)\}` or
     `(errorMessage|mutationError|\w+Error):\s*e\.toString\(\)`
     reappears under `app/lib/presentation/viewmodels/`. Also fail
     if a freezed state class re-introduces a `String? errorMessage`
     field by grepping for the literal pattern under
     `app/lib/presentation/viewmodels/*_state.dart`. Track allowlist
     escapes via `// dart-error-tostring-allow` comments mirroring
     `dart-line-count-allow`.
- **Pros:**
  - Fixes the sanitization leak **and** the localization gap in one
    pass — Spanish users finally get Spanish error text.
  - Aligns view-model layer with `docs/client/i18n.md` and the
    existing architectural notes in
    `app/lib/core/errors/rpc_error_handler.dart` (lines 204-208 /
    213-216): widgets resolve, viewmodels expose typed state.
  - Forward-compatible with future error categories (e.g. domain
    enums for specific flows): adding a variant to `UserError` is a
    single-file change with the compiler enforcing every call site.
- **Cons:**
  - Touches every freezed `*_state.dart` (15 files) plus a handful
    of inline state classes embedded in viewmodels, every viewmodel
    that catches an exception (~30 files), and every widget that
    reads `state.errorMessage` (~40 files under
    `app/lib/presentation/widgets/` and `app/lib/presentation/screens/`).
    Existing viewmodel tests assert on literal `'Failed to X: …'`
    strings; widget tests assert on rendered error text. Both need
    updating.
  - `flutter pub run build_runner build` must run after
    `*_state.dart` edits to regenerate `*.freezed.dart`. The Flutter
    test workflow already runs build_runner before `flutter test`,
    so no CI change is needed — but the local dev loop is heavier.
- **Effort:** Large. Roughly 2× Option A's footprint. Realistic
  breakdown:
  - 1 new file (`user_error.dart` + tests)
  - 1 modified file (`rpc_error_handler.dart` to add `classify` and
    `localize` methods)
  - 15 freezed state classes edited
  - ~5 inline state classes embedded in viewmodels edited
  - ~30 viewmodels edited (call-site migration)
  - ~40 widgets/screens edited (resolution at render time)
  - ~12 viewmodel test files edited (assertions move from string to
    typed)
  - ~10 widget test files edited (assertions on rendered text need
    `MaterialApp` with `localizationsDelegates`; many already have
    them per `app/test/helpers/l10n_helpers.dart`)
  - 1 CI script + workflow wire-up
  - 2 docs updates (`app/lib/core/errors/README.md`,
    `docs/client/i18n.md` cross-reference)

### Option C: Per-viewmodel `try/on ServiceException catch` rewrite without a helper

- **Approach:** Walk every `catch (e)` and convert it to
  `on ServiceException catch (e) { errorMessage: e.message; } catch (e) {
  errorMessage: <hardcoded fallback>; }`.
- **Pros:** No new helper API; localized rewrites only.
- **Cons:** Repeats the same conditional 67 times; loses the
  `ConnectException`/`SocketException`/`TimeoutException` mapping that
  `RpcErrorHandler` already does cleanly; doesn't address the
  localization-at-viewmodel architectural violation; future
  regressions are likely.
- **Effort:** Medium-Large with worse long-term ergonomics.
- **Status:** Rejected — duplicates logic and doesn't fix either
  root cause.

## Recommendation

**Option B.** Reviewer (maintainer) selected this over Option A on
the rationale that it fixes the localization gap at the same time as
the sanitization leak and aligns view-model state with the existing
architectural guidance in `docs/client/i18n.md`. Option A is the
narrower path and remains a viable fallback if scope creeps; Option C
is rejected.

### Per-site judgment call

The migration includes one judgment call per site: when classifying a
generic `catch (e)` that isn't a `ServiceException`/`ConnectException`/
transport exception, what `fallback` string do we pass? In practice:

- For load/fetch flows, omit `fallback` and rely on
  `rpcErrorGeneric` ("Something went wrong. Please try again.").
  `ServiceException` and `ConnectException` paths still surface
  precise messages.
- For mutation flows where the action is non-obvious from
  surrounding UI (e.g. `'Could not start the loan'`,
  `'Could not share with group'`), pass that as `fallback`. It only
  surfaces when the helper hits the generic bucket — the typed
  branches still win and remain localized via `context.l10n` at
  render time.

Note that `fallback` is **English only** in this migration. Localizing
the fallback strings is a separate ARB-coverage exercise; the typed
branches (`Code`, transport, `ServiceException.message` from the
server) carry the localization win that motivates Option B.

### Why store both `code` and `serverMessage` in `UserError`?

`ServiceException` already carries both `message` (a localized,
user-friendly string from the server) and `code` (the structured
`Code?`). The classifier preserves both: widgets that want to render
the server's exact wording use `serverMessage` when present;
widgets that want to switch on category (e.g. show a sign-in CTA on
`Code.unauthenticated`) still have `code`. The localizer prefers
`serverMessage` when present and falls back to `_localizeCode(code, l10n)`.

### What about `errorMessage` getters for backwards compatibility?

Don't add transitional `String? get errorMessage => …` getters on
state classes. They mute the migration's value (widgets keep
reading the resolved string) and require threading a
`BuildContext`/`AppLocalizations` into the getter, which freezed
doesn't support cleanly. Migrate widgets in the same PR.

## Open Questions

- **Where does `UserError` live — alongside `RpcErrorHandler` or in a
  separate file?** — Recommendation: separate file
  `app/lib/core/errors/user_error.dart`. Keeps the freezed/sealed
  type isolated from the handler class, mirrors the existing
  `app/lib/core/errors/` directory structure, and avoids forcing
  every viewmodel to import the `RpcErrorHandler` class symbol just
  to reference the type. The handler stays in
  `rpc_error_handler.dart` and gains `classify` / `localize`
  static methods that operate on `UserError`.
- **Is `UserError` a freezed sealed class or a plain class?** —
  Recommendation: freezed sealed class with named variants
  (`.serverMessage(String)`, `.rpcCode(Code)`, `.transport(TransportKind)`,
  `.generic({String? fallback})`). Pattern-matching in the localizer
  becomes exhaustive and the compiler catches missed branches when
  variants are added later. Tests under `app/test/core/errors/`
  cover each variant.
- **Should `localize` be on `RpcErrorHandler` or as an extension on
  `UserError`?** — Recommendation: static method on `RpcErrorHandler`
  (`RpcErrorHandler.localize(UserError, AppLocalizations)`). Keeps
  the localization pipeline in one place; an extension would split
  it. Widget-side ergonomics with
  `RpcErrorHandler.localize(state.error!, context.l10n)` are
  acceptable and grep-friendly.
- **Do we keep the `is ServiceException → message` shortcut even
  though `ServiceException.toString() == message`?** —
  Recommendation: yes — the classifier reads `e.message` (and
  `e.code` when present) directly, never `toString()`, so a future
  `ServiceException.toString()` override can't leak.
- **Do we remove the existing `RpcErrorHandler.handleError(...)`
  instance method?** — Recommendation: no, leave it. It's used by
  service-layer code that already has a `BuildContext` and wants
  crash reporting. The new path (`classify` + state field +
  widget-side `localize`) is for viewmodel call sites that don't
  have a `BuildContext`. The two paths coexist; widgets can choose
  whichever is more ergonomic.
- **What grep pattern does the CI gate use?** — Recommendation:
  `npm run lint:dart:errortostring` (a new script) running two
  greps:
  1. The original raw-toString regex on viewmodel call sites
     (catches regression of the original leak).
  2. `'^\s*String\?\s+(errorMessage|mutationError|\w+Error)\b'`
     under `app/lib/presentation/viewmodels/*_state.dart` (catches
     re-introduction of the `String? errorMessage` field that the
     migration removes). Both behind the
     `// dart-error-tostring-allow` escape hatch with tracking-issue
     requirement, mirroring `dart-line-count-allow`. Wire into
     `.github/workflows/test_flutter.yaml`.
- **Should the helper log via `Logger`?** — Recommendation: no.
  Logging stays in `RpcUtils.executeRpc` and the
  `_log.severe('Failed to …', e, stackTrace)` calls already
  surrounding every viewmodel `catch`. Per
  `docs/client/observability.md`, breadcrumbs and crash-report
  reporting hang off `ObservableLogger` and `ObservabilityService`,
  not the error-text path. Mixing logging into the classifier
  would double-log every error and risk PII (the `LogRedactor`
  applies at the observability layer, not at string-format
  helpers).

## Implementation Plan

### Phase 1: Define the typed error and update `RpcErrorHandler`

**Goal:** Land the new types and the classifier/localizer with full
unit-test coverage so the rest of the migration can refer to a stable
API.
**Tasks:**

- [x] Create `app/lib/core/errors/user_error.dart` defining a
      `@freezed sealed class UserError` with variants:
  - `UserError.serverMessage({required String message, Code? code})`
    — produced from `ServiceException`. Carries both fields so
    widgets can switch on `code` without losing the server's exact
    wording.
  - `UserError.rpcCode(Code code)` — produced from a raw
    `ConnectException` that didn't pass through `executeRpc` (so
    no `ServiceException` wrap).
  - `UserError.transport(TransportKind kind)` where
    `enum TransportKind { socket, timeout, http }`.
  - `UserError.generic({String? fallback})` — the catch-all bucket;
    `fallback` is the optional English context string for "anything
    weird" cases.
- [x] Add `static UserError classify(Object e, {String? fallback})`
      to `app/lib/core/errors/rpc_error_handler.dart` matching the
      precedence: `ServiceException > ConnectException >
      SocketException / TimeoutException / HttpException > generic(fallback)`.
      The classifier reads `e.message` and `e.code` directly from
      `ServiceException`; never calls `e.toString()`.
- [x] Add `static String localize(UserError error, AppLocalizations l10n)`
      using exhaustive pattern-matching:
  - `serverMessage(message: m)` → `m` (already localized by the
    server / by the `ServiceException` constructor).
  - `rpcCode(code)` → `_localizeCode(code, l10n)` (existing private).
  - `transport(socket)` → `l10n.rpcErrorTransportSocket`, etc.
  - `generic(fallback: f)` → `f ?? l10n.rpcErrorGeneric`.
- [x] Document precedence in a doc comment on `classify` and
      reference `localize` from there. Note that the helper is a
      pure transformation — no logging.
- [x] Update `app/lib/core/errors/README.md` (already exists per
      CLAUDE.md README rule) to describe the new types, the
      sanitization-leak class they prevent, the prohibition on
      `e.toString()` in viewmodel error fields, and the prohibition
      on resolved-string `errorMessage` fields on state classes.
- [x] Add cross-reference in `docs/client/i18n.md` from the
      "Viewmodels never resolve strings" paragraph to the new
      `UserError` type as the canonical error-state shape.
- [x] Add `app/test/core/errors/user_error_test.dart` and
      `app/test/core/errors/rpc_error_handler_test.dart` (new files;
      current handler tests live elsewhere). Coverage:
  - `classify(ServiceException('msg', code: Code.notFound))` → `serverMessage('msg', code: notFound)`.
  - `classify(ConnectException(code: notFound))` → `rpcCode(notFound)`.
  - `classify(SocketException('…'))` → `transport(socket)`.
  - `classify(TimeoutException('…'))` → `transport(timeout)`.
  - `classify(HttpException('…'))` → `transport(http)`.
  - `classify(FormatException('SQL error: …'), fallback: 'Could not save')` → `generic(fallback: 'Could not save')`.
  - `classify(StateError('null check'))` → `generic(fallback: null)` — pins the leak fix.
  - `classify(Object())` → `generic(fallback: null)`.
  - `localize(...)` for each variant against an English
    `AppLocalizations` and a Spanish-stub `AppLocalizations`,
    proving the localization plumb works end-to-end.
- [x] Run `cd app && flutter pub run build_runner build` to
      regenerate `user_error.freezed.dart`, then
      `cd app && flutter test --reporter expanded test/core/errors/`.
      **Validates:** Types compile, classifier covers every branch,
      a non-`Exception` `Error` subclass no longer leaks `'Bad
      state: …'`, and Spanish text renders when an `app_es`
      `AppLocalizations` is supplied.

### Phase 2: Migrate freezed state classes

**Goal:** Every `*_state.dart` (and embedded inline state class)
replaces `String? errorMessage` with `UserError? error` (and
mirror-renames any multi-error fields).
**Tasks:**

- [x] For each of the 15 explicit `app/lib/presentation/viewmodels/*_state.dart`
      files, change `String? errorMessage,` →
      `UserError? error,` in the freezed factory. Add the import
      for `package:ripls/core/errors/user_error.dart`.
- [x] For state classes with multiple error fields, mirror-rename:
  - `community_content_state.dart`: `String? membersError`,
    `String? eventsError`, `String? gearError` →
    `UserError? membersError`, `UserError? eventsError`,
    `UserError? gearError`.
  - `user_profile_state.dart`: `String? userError`,
    `String? communitiesError`, `String? activitiesError` →
    `UserError? userError`, etc.
  - Any `String? sharingError` / `String? communitiesError` on
    `experience_sharing_view_model.dart`'s inline state.
- [x] For the ~5 viewmodels that define their state inline rather
      than in a separate `*_state.dart` (e.g. ones in this list
      whose adjacent `*_state.dart` does not exist —
      `community_creation_view_model.dart`, `community_edit_view_model.dart`,
      `community_view_model.dart`, `feed_view_model.dart`,
      `gear_view_model.dart`, `governance_view_model.dart`,
      `inbox_view_model.dart`, `location_picker_view_model.dart`,
      `manage_members_view_model.dart`, `portfolio_inbox_view_model.dart`,
      `profile_gear_view_model.dart`, `request_creation_view_model.dart`,
      `request_needs_view_model.dart`, `request_sharing_view_model.dart`,
      `request_view_model.dart`, `search_view_model.dart`,
      `experience_creation_view_model.dart`,
      `experience_needs_view_model.dart`,
      `experience_view_model.dart`,
      `experience_sharing_view_model.dart`,
      `feedback_sheet_view_model.dart`,
      `giveaway_giver_view_model.dart`,
      `giveaway_receiver_view_model.dart`), grep for the inline
      state class definition and apply the same rename. If the
      state is a plain Dart class (not freezed), update the
      constructor and `copyWith` manually; if it is freezed, edit
      the factory.
- [x] Update `bool get hasError => errorMessage != null;` getters
      on state classes to `bool get hasError => error != null;`.
- [x] Run `cd app && flutter pub run build_runner build` to
      regenerate every `*_state.freezed.dart`. Verify the build
      completes with no missing-import errors.
      **Validates:** State classes compile against the new typed
      field; freezed regen leaves `*.g.dart` untouched (none of
      these states use json_serializable).

### Phase 3: Migrate the 30 viewmodel call sites

**Goal:** Every `errorMessage`/`mutationError`/`*Error` assignment that
reads from `e.toString()` switches to
`copyWith(error: RpcErrorHandler.classify(e, fallback: …))`.
**Tasks:**

- [x] Migrate plain sites (`errorMessage: e.toString()` — 33
      occurrences) to
      `error: RpcErrorHandler.classify(e)`. Files: all 30 under
      `app/lib/presentation/viewmodels/` listed by
      `grep -rln 'errorMessage:.*toString()\|mutationError:.*toString()\|.*Error:.*toString()'
      app/lib/presentation/viewmodels/`.
- [x] Migrate prefixed sites (`'Failed to X: ${e.toString()}'` — 34
      occurrences) to
      `error: RpcErrorHandler.classify(e, fallback: 'Could not X')`.
      Default action: drop the prefix entirely and let the
      `ServiceException` / `ConnectException` branches surface
      their own (localized) message; only pass `fallback:` when
      the action is non-obvious from surrounding UI (e.g.
      `event_modal_view_model.dart:782` `'Could not share with group'`).
- [x] Update `copyWith(errorMessage: null)` clear-on-loading sites
      to `copyWith(error: null)`. Same shape, mechanical rename.
- [x] Add `import 'package:ripls/core/errors/user_error.dart';` and
      `import 'package:ripls/core/errors/rpc_error_handler.dart';`
      where missing.
- [x] Preserve the `_log.severe('Failed to …', e, stackTrace);`
      calls that surround every catch — the helper does not replace
      logging. `ObservableLogger` continues to handle breadcrumbs
      and crash reporting per `docs/client/observability.md`.
- [x] Run `cd app && flutter analyze` to confirm there are no
      stale `state.errorMessage` references inside viewmodel
      files (analyzer surfaces them once the field is gone).
      **Validates:** Viewmodels compile; no `e.toString()` leaks
      remain.

### Phase 4: Migrate widgets and screens to resolve at render time

**Goal:** Every widget that displayed `state.errorMessage` now resolves
the typed error via `RpcErrorHandler.localize(state.error!, context.l10n)`.
**Tasks:**

- [x] List all reader sites:
      `grep -rEn 'state\.errorMessage|\.errorMessage\b' app/lib/presentation/widgets/ app/lib/presentation/screens/`.
      Expect ~40 matches.
- [x] For each match:
  - Replace `state.errorMessage != null` guards with
    `state.error != null`.
  - Replace `Text(state.errorMessage!)` /
    `SnackBar(content: Text(state.errorMessage!))` /
    `errorMessage: state.errorMessage` widget params with
    `Text(RpcErrorHandler.localize(state.error!, context.l10n))`
    (or thread it through the relevant widget's `errorMessage:`
    constructor param if the param accepts a `String`).
  - For multi-error states, do the same per field
    (`state.membersError`, `state.eventsError`, etc.).
- [x] Add the `import 'package:ripls/core/errors/rpc_error_handler.dart';`
      where missing.
- [x] For widgets that pass error text down through a constructor
      param, prefer keeping the param as `String?` and resolving at
      the call site so leaf widgets remain context-free. If a leaf
      widget genuinely needs `UserError?` (e.g. it switches behavior
      on `Code.unauthenticated` to show a sign-in CTA), allow
      passing the typed value through and resolving inside the
      leaf — but the migration does not change widget signatures
      preemptively; only when the existing call site needs it.
- [x] Run `cd app && flutter analyze` and the dart-line-count gate
      (`npm run lint:dart:size`).
      **Validates:** Widget tree compiles end-to-end; analyzer is
      clean.

### Phase 5: Update tests

**Goal:** Existing viewmodel and widget tests assert on the typed
state or on rendered text, not on the obsolete English literal
`'Failed to X: …'`.
**Tasks:**

- [x] Update viewmodel tests under
      `app/test/presentation/viewmodels/`. About a dozen files
      assert exact error text today. Per `docs/client/testing.md`,
      these tests use `ProviderContainer` with `Mockito` mocks.
      Replace `expect(state.errorMessage, 'Failed to X: …')` with
      either:
  - `expect(state.error, isA<UserErrorServerMessage>())` (and
    inspect the `message`/`code`), or
  - `expect(state.error, isA<UserErrorTransport>())`, or
  - `expect(state.error, isA<UserErrorGeneric>())` with
    `fallback` matching when relevant.
      Asserting on the typed value survives a future ARB string
      tweak.
- [x] Update widget tests that assert on rendered error text. Most
      already wrap their `MaterialApp` with
      `localizationsDelegates` per `app/test/helpers/l10n_helpers.dart`;
      assertions become `expect(find.text(l10n.rpcErrorNotFound), findsOneWidget)`
      or similar. Tests that don't currently load localizations
      need `l10n_helpers.dart`'s wrapper added.
- [x] Add a small `app/test/presentation/viewmodels/user_error_state_smoke_test.dart`
      that exercises one viewmodel end-to-end (e.g.
      `event_modal_view_model_test.dart` already exists and can be
      extended) to confirm `error: classify(thrown)` round-trips
      through `copyWith` and `localize` to readable text.
- [x] Run `cd app && flutter test --reporter expanded` end-to-end.
      **Validates:** No regression in viewmodel or widget test
      suites.

### Phase 6: Add the CI grep gate

**Goal:** Prevent both the original leak pattern and the
resolved-string state field from re-appearing.
**Tasks:**

- [x] Add `lint:dart:errortostring` script to root `package.json`
      next to `lint:dart:size`. Implementation: a small shell
      script under `scripts/` that runs:
  1. `grep -rEn '(errorMessage|mutationError|[a-zA-Z]+Error):\s*[\x27"]?[^\x27"\n]*\$\{?\s*e\.toString\(\)|^\s*errorMessage:\s*e\.toString\(\)' app/lib/presentation/viewmodels/`
  2. `grep -rEn '^\s*String\?\s+(errorMessage|mutationError|[a-zA-Z]+Error)\b' app/lib/presentation/viewmodels/`
      and exits non-zero on any match (excluding lines containing
      `// dart-error-tostring-allow`).
- [x] Document the allowed escape hatch
      (`// dart-error-tostring-allow` comment + tracking issue),
      mirroring `dart-line-count-allow`.
- [x] Wire the new script into
      `.github/workflows/test_flutter.yaml` alongside the existing
      `lint:dart:size` step.
- [x] Verify the gate fires on a deliberately-introduced violation
      of each regex (revert before commit) and is silent on the
      migrated tree.
      **Validates:** Both regression vectors are blocked at PR
      time, not just "fixed once."

### Phase 7: Cleanup pass and PR-ready polish

**Goal:** Catch incidental usages, finalize docs.
**Tasks:**

- [x] Search for incidental sibling patterns the grep doesn't cover:
      `error.toString()`, `err.toString()`, `exception.toString()`,
      `errorMessage:.*?\\$\\{.*\\.toString\\(\\)\\}` — convert any
      under `app/lib/presentation/viewmodels/`. (Many of these will
      already have been removed during Phases 3–4 because the
      `String? errorMessage` field no longer exists; this pass is
      defensive.)
- [x] Confirm `app/lib/core/errors/README.md` mentions purpose,
      key types (`ServiceException`, `RpcErrorHandler`, `UserError`,
      `TransportKind`), and the leak-prevention rationale per
      CLAUDE.md's README rule.
- [x] Confirm `docs/client/i18n.md`'s "Viewmodels never resolve
      strings" paragraph cross-references `UserError` as the
      canonical error-state shape.
- [x] Run `cd app && flutter test --reporter expanded` and
      `cd app && flutter analyze` on the full tree once more.
      **Validates:** End-to-end clean run, docs match code.

## Related Issues

- **review:error_handling label** — Several other issues filed by the
  same `/ripls-reviews` pass cover adjacent error-handling concerns
  (typed error fields, structured RPC error details, observability
  signal). This issue, with Option B, now owns the typed-error-state
  direction; coordinate scope with adjacent triage if any of those
  issues overlap.
- **app/lib/core/errors/rpc_error_handler.dart i18n note** — the file
  comment on `getTransportErrorMessage` and `getUserMessageForCode`
  documents the long-term direction (typed error states resolved at
  widget layer). Option B implements that direction directly.
