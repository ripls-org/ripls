# core/config

Build-time configuration and feature flags for the Ripls app.

## Key files

- **environment.dart** — Static getters backed by `--dart-define-from-file`
  environment variables (server URL, Mapbox token, cache TTL, feature flags).
  Values are compile-time constants; changing them requires a rebuild.

- **feature_flags.dart** — Riverpod `Provider<bool>` wrappers for each
  feature flag in `Environment`. Wrapping compile-time constants in providers
  lets widget tests override flag values via `ProviderScope.overrides` without
  needing `--dart-define` arguments.

## When to add code here

Add a new file here when you need app-wide configuration that is:
- Set at build time (not fetched from the server at runtime), or
- A feature flag that widget tests must be able to toggle.

Runtime remote config (e.g., server-driven kill-switches) belongs in
`services/` as a provider backed by an API call, not here.
