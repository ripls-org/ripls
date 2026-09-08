# server/l10n

Server-side localization for strings the server is the terminal
renderer of: push notification copy and email content.

## Render boundary

This package does **not** localize strings the user reads inside the
Flutter app (story prose, chat system messages, RPC error text).
Those live in the client ARB catalog under `app/lib/l10n/` and the
server emits them as `{template_key, params}` payloads.

The defining principle, from `docs/server/l10n.md`:

> The client renders any string the user reads inside the app. The
> server renders only strings where the server is the terminal
> emitter — push notifications, emails, SMS.

## Key files

- `localizer.go` — `Localizer` wraps `nicksnyder/go-i18n` v2 and
  exposes `T(key, params)` for code sites.
- `locale.go` — `LocaleFromContext(ctx)`, `Normalize(tag)`,
  `Supported`. The resolver picks the recipient's locale with
  priority (explicit arg → `Accept-Language` header → stored
  `preferred_language` → `en`).
- `bundle.go` — embeds the `source/*.toml` catalogs and builds the
  process-wide `*i18n.Bundle`.
- `source/{en,es}.toml` — translation source files. `en.toml` is
  the source of truth; CI fails if any other locale is missing a key
  that exists in `en.toml`.

## When to add code here

- A new push notification key or email subject — add the TOML entries
  and use `Localizer.T(key, params)` at the call site.
- A new way to resolve recipient locale (e.g. a fan-out path that
  needs per-recipient locale resolution distinct from
  `LocaleFromContext`) — extend `locale.go`.

## When to add code elsewhere

- New in-app strings (stories, chat, errors) — add to the client ARB
  files under `app/lib/l10n/`. The server emits the template key, not
  the rendered text.
- New AI-generated user-visible prose (nudges) — prompt the model in
  the recipient's locale; no catalog entry needed.

## Tests

- `locale_test.go` exercises `Normalize` (case folding, region
  stripping, unsupported-tag fallback) and `LocaleFromContext`
  priority order.
- `localizer_test.go` exercises template rendering, missing-key
  fallback, and the per-key warn-once log gate.

See `docs/server/l10n.md` for the full design and contributor
workflow.
