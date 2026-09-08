# lib/

lib/ is the Flutter app root. The codebase follows a layered structure: data → services → presentation.

## Directories

- [`core/`](core/) — config, theme, routing, and shared utilities used across all layers
- [`data/`](data/) — generated proto types, caching infrastructure, and repositories
- [`services/`](services/) — remote API clients (gRPC/Connect over HTTP)
- [`presentation/`](presentation/) — screens, widgets, and viewmodels (UI layer)
- [`l10n/`](l10n/) — localization (ARB files and generated Dart bindings)

## Where to add new code

- **New API client** → `services/`. One file per server-side RPC service.
- **New cached data type** → `data/repositories/`. Wrap the service call with a `CacheService`.
- **New screen or widget** → `presentation/screens/<feature>/` or `presentation/widgets/`.
- **New shared utility** (format, parse, compute) → `core/utils/`.
- **New theme token** (color, text style) → `core/theme/`.
- **New localized string** → `l10n/app_en.arb` (add translations to other locales in parallel).

Keep dependencies one-way: `presentation` may import `data` and `services`; `data` may import `services`; `core` has no app-layer imports. Never let `services` import `presentation`.
