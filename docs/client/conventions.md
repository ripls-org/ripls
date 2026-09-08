---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Flutter client coding conventions — lib/ layout, api-protos-only rule, screen decomposition, CachedMediaImage for user media, repository caching, the "Event" not "Experience" UI-terminology rule, and client testing.
  globs: [app/lib/**]
  triggers: [flutter, dart, widget, viewmodel, riverpod, cachedmediaimage, experience-vs-event]
  lens: [client, conventions]
  alwaysApply: true
  domain: client
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Client Coding Conventions (Flutter/Dart)

Conventions every change under `app/lib/` is expected to follow. This is the
coding-rules companion to [`client/architecture.md`](architecture.md) (which
covers the MVVM + Riverpod structure) and
[`proto_conventions.md`](../proto_conventions.md) (which covers the proto/API
contract).

## Flutter code structure

```
lib/
├── core/            # Core functionality and utilities
│   ├── config/      # App-wide configuration
│   ├── theme/       # Theme data
│   └── utils/       # Utility functions
│
├── data/            # Data layer (protos, server calls)
│   ├── gen/         # Auto-generated Dart code from proto files, the main business objects
│   └── datasources/ # Local APIs, storage
│
├── services/        # Remote API access
│
├── presentation/    # UI layer
│   ├── screens/     # Full page widgets
│   ├── widgets/     # Reusable UI components
│   └── viewmodels/  # State management
│
└── main.dart        # Entry point
```

## Flutter & Protos

The client should always work with the `ripls/api` protos and proto-generated files. The client should never directly reference the protos or proto-generated files in `ripls/models`. Those are server and storage-specific.

## Dart comments

- Comments describe durable invariants, not the rollout phase, task, or branch
  that motivated a change. **Phase markers in `*.dart` comments fail the build**
  — "Phase 3a", "pre-refactor", "new in this PR" and the like are rejected by
  `scripts/check_ephemeral_comments.js` via `npm run lint:ephemeral-comments`.
  That context belongs in the commit message, PR description, or plan docs under
  `docs/`. Issue and PR numbers (e.g. `#1157`) are allowed — they're stable and
  retrievable.

## Analyzer and lint rules

`cd app && flutter analyze` is the Dart gate, and it is the **same command CI
runs** (`.github/workflows/test_flutter.yaml`, "Analyze Flutter code" — a
failure blocks the PR). Everything it enforces lives in
[`app/analysis_options.yaml`](../../app/analysis_options.yaml): `flutter_lints`
plus two analyzer strictness modes (`strict-raw-types`, `strict-casts`) and 22
lint rules the Dart SDK ships. No extra dependency; the set was chosen by
measuring the whole tree rather than adopting a vendor preset (#2794).

**`flutter analyze` treats *info*-level findings as fatal; `dart analyze` does
not.** `flutter analyze` defaults `--fatal-infos` to on, `dart analyze` defaults
it to off — so a clean local `dart analyze` can sit on a tree that fails CI, and
most lint rules report at info level. Always check with `flutter analyze` (or
`npm run lint:dart`, which is the same thing).

The practical consequence: **there is no advisory tier.** Turning on a rule
makes every existing finding a build failure, so a rule flip has to ship in the
same PR as its fixes. That is the property that keeps this gate green rather
than muted.

Three rules were measured and deliberately **declined**; `analysis_options.yaml`
records each with its rationale so they are not re-proposed:
`discarded_futures` (382 sites, mostly correct fire-and-forget — follow-up
#2823), `strict-inference` (175 mechanical type arguments, no measured
defect — follow-up #2824),
and `avoid_positional_boolean_parameters` (17 of its 19 findings are single-bool
setters, which [`code_quality_review_prompt.md`](../reviews/code_quality_review_prompt.md)
§11 explicitly tolerates).

### Fire-and-forget futures

`unawaited_futures` is on, so an un-awaited `Future` inside an `async` body is a
build failure unless you mark it. Mark it with `unawaited(...)` from
`dart:async` — **not** an `// ignore:` comment, which is invisible to anyone
grepping for deliberate fire-and-forget. Anything on the logout path is held to
a higher bar: see [`client/logout.md`](logout.md) for the teardown ordering that
an accidental un-awaited step would break.

`docs/client/architecture.md` §"Async-First Architecture" is the rule of thumb —
an un-awaited *repository* call in a ViewModel is presumptively a bug; an
un-awaited analytics, haptic, or navigation side effect is presumptively fine.

`npm run lint:dart` is only the analyzer. The other Dart gates
(`lint:dart:size`, `lint:dart:dead`, `lint:dart:a11y`, `lint:dart:colors`,
`lint:ephemeral-comments`, `lint:todo-refs`, …) are separate npm scripts and run
as separate CI steps; `npm run lint` chains all of them.

## UI Terminology: "Event", not "Experience"

**Never use the word "experience" in user-facing strings.** The domain
concept is called `Experience` everywhere in code (proto messages,
service names, file paths, identifiers, log fields, READMEs, internal
docs) because the programming concept of "event" is overloaded — we
already have `CommunityEvent`, `ChatSystemAction`, FCM event payloads,
and so on. But to users, an `Experience` is just an **Event**.

This rule applies to anything a user can read or hear:

- ARB strings (`app/lib/l10n/app_*.arb`) — both the values and the
  user-visible portions of `@key.description`.
- Server-generated notification titles and bodies
  (`server/community/notifications.go`, the `notifyRSVPedUsers` copy in
  `server/services/experience/lifecycle.go`, etc.).
- Email and SMS templates.
- Any string a screen reader announces (accessibility labels).

The rule does **not** apply to:

- Code identifiers, package names, file paths, RPC names, proto
  messages, or enum values — these stay `Experience`.
- Internal docs (`docs/`, READMEs, `doc.go`, plan files) — these
  describe the system and use the code term.
- Server log lines and structured-log field names.

When in doubt: would this string appear in the app, in a push
notification, in an email, or be read aloud by a screen reader? If
yes, use **Event**. Otherwise, use `Experience`.

## Flutter Screens

Flutter UX code is highly nested and this can cause readability and maintenance
issues. Keep build methods in screens from becoming too large and monolithic.

- **Extract Methods**: Break down complex screens and widgets into smaller
  private methods (prefixed with `_`) within the same class. This reduces
  nesting and gives each section a clear name.
- **Create Custom Widgets**: Split larger widgets into smaller, reusable custom
  widgets. This is especially useful when a piece of UI is used in multiple
  places or represents a logical component.
- **Use Builder Methods**: For complex build logic, create separate builder
  methods that return different parts of the widget tree.

Dart files under `app/lib/` are capped at **1,000 lines** by
`npm run lint:dart:size` (see [File Organization in CLAUDE.md](../../CLAUDE.md)).

## Media Display

- **Always use CachedMediaImage** for displaying user-uploaded media (images from MediaRepository or mediaUrlProvider).
- **Never use Image.network directly** for user media — it caches by URL which causes bugs with presigned URLs.
- **Use stable cache keys** based on media ID: use the `cacheKey` property from `MediaUrl` objects returned by the repository.

See [`client/media.md`](media.md) for the full media system (presigned URLs,
thumbnail selection, video lifecycle).

## Client-Side Caching

- **Repository Pattern**: All data access should go through repositories (MediaRepository, UserRepository, GearRepository, LoanRepository, etc.).
- **Cache Key Naming Convention**:
  - Repository namespace: `'repository_name'` (e.g., 'user', 'gear', 'loan', 'community')
  - Item keys: `'namespace:item_id'` (e.g., 'user:user123')
  - List keys: `'namespace:list_type:filter'` (e.g., 'gear:user:list', 'loan:received:gear:gear123')
- **Repository Composition**: Keep dependencies shallow (max 1 level deep) and one-way; avoid circular dependencies.
- **Manual Caching**: Do NOT add manual cache maps (`Map<String, T>`) to viewmodels; use repositories instead.
- **Cache Invalidation**: Call repository `refresh*()` or `invalidate*()` methods after mutations, not manual cache clearing.

See [`client/caching.md`](caching.md) for the full caching architecture.

## Client Testing

- Flutter tests are in `app/test/` mirroring `app/lib/` structure:
  - Cache tests: `app/test/data/cache/`
  - Repository tests: `app/test/data/repositories/` (use Mockito for service mocking)
  - ViewModel tests: `app/test/presentation/viewmodels/`
- Run with `flutter test --reporter expanded` or `npm run test:app` from project root.
- **Always use `--reporter expanded`** for Flutter tests — the default `compact` reporter uses carriage returns that produce enormous output when piped.
- **Mock Generation**: Use `flutter pub run build_runner build` to generate Mockito mocks from `@GenerateMocks` annotations.

See [`client/testing.md`](testing.md) for the full client testing strategy.
