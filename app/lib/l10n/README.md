# l10n

Localization resources and generated Dart bindings.

## Key files

- **`app_en.arb`** — English strings (source of truth). Each entry has a key, a value, and a `@key` metadata block with a `description`.
- **`app_es.arb`** — Spanish translations. Keys must match `app_en.arb`.
- **`app_localizations.dart`** / **`app_localizations_en.dart`** / **`app_localizations_es.dart`** — generated Dart classes (do not edit by hand). Regenerate with `npm run generate`.

## How to add a new string

1. Add the key and value to `app_en.arb` with a `description` field.
2. Add the same key with the translated value to every other locale file (e.g., `app_es.arb`). Use English as a placeholder if the translation is not yet available.
3. Run `npm run generate` to regenerate the Dart classes.
4. Access the string in widgets via `context.l10n.yourKey` (see `app/lib/core/extensions/l10n_extensions.dart`).

## Conventions

- Key names use camelCase with a feature prefix: `commonSave`, `gearEditTitle`, `loanStatusActive`.
- Avoid embedding layout markup or punctuation that varies by locale in keys — let the ARB value carry locale-specific punctuation.
- Parameterized strings use ICU `{placeholderName}` syntax.

## Rules

**Viewmodels never resolve strings.** Viewmodels expose typed state (enums, error codes, data objects). Widgets resolve to localized text in `build()` via `context.l10n`. This keeps viewmodels testable without a widget tree.

**CI enforces the `context.l10n.*` pattern.** `npm run lint:dart:i18n` (`scripts/check_i18n.js`) catches hardcoded user-visible strings in the presentation layer. Run it locally before pushing.

**CI enforces key parity across ARB files.** `npm run lint:dart:l10n-parity` (`scripts/check_l10n_parity.js`) fails if any key in `app_en.arb` is missing from another locale's ARB (or vice-versa). English placeholders are fine when a real translation is not yet available — the point is that every key exists in every locale so `flutter gen-l10n` produces no untranslated warnings (issue #1972).
