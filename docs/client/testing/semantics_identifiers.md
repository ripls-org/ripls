---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Semantics(identifier) and the semanticsIdentifier param on accessibility primitives — how the Playwright e2e harness finds Flutter Web widgets via the flt-semantics DOM tree, the kebab-case convention, and where identifiers do and don't reach DOM.
  globs: [app/lib/presentation/widgets/accessibility/**, e2e/**]
  triggers: [semantics, semantics-identifier, accessibility, playwright, e2e, flt-semantics, kebab-case]
  lens: [client, testing]
  domain: client
freshness:
  verified_commit: "e5b434c33"
  verified_on: "2026-07-12"
---
# Semantics identifiers

This is the one-pager for `Semantics(identifier: …)` and the
`semanticsIdentifier:` parameter on the accessibility primitives. Both
are how the Playwright e2e harness (#2162) finds Flutter widgets in
the browser DOM. Read this once before touching either.

## Why a separate identifier field exists

Flutter Web renders widgets to a `<canvas>` by default. Playwright sees
one giant black box. Calling `SemanticsBinding.instance.ensureSemantics()`
at app start (see [`app/lib/main.dart`](../../../app/lib/main.dart))
forces Flutter to also emit a parallel `<flt-semantics>` DOM tree —
the accessibility tree, serialized to ARIA-labelled elements. That's
the tree Playwright queries.

Two ways the tree carries information:

| Source | DOM attribute | Spoken by screen readers? |
|---|---|---|
| `Semantics(label: …)` | `aria-label="…"` | Yes |
| `Semantics(identifier: …)` | `flt-semantics-identifier="…"` | **No** |

`label:` is the user-facing announcement and is l10n-bound (the
`require_l10n_for_semantic_labels` rule rejects raw strings on it).
`identifier:` is the test contract: stable, raw, not spoken.

`ValueKey` and `Key` do not appear in the DOM. They're only reachable
via Flutter's `integration_test` driver bridge, which the harness
doesn't use. Patrol-era `Key('login_email_field')` instances in
`screens/auth/` are dead weight for the Playwright path; new code
must use `Semantics(identifier: …)` instead.

## The convention

- **Lowercase kebab.** `event-hero-video-player`, not
  `eventHeroVideoPlayer` or `'Event Hero Video Player'`. Enforced by
  the `require_kebab_case_for_semantics_identifier` lint rule:
  `^[a-z0-9-]+$`.
- **No project prefix.** Use `event-hero-video-player`, not
  `ripls:event-hero-video-player`. The Semantics tree is already
  namespaced by ancestor route; a synthetic prefix is noise.
- **No l10n.** Identifiers are not user-facing. They live in code as
  raw strings.

## When to add one

**Only when a test asks for it.** Don't pre-tag widgets speculatively.
A stable test identifier is contract surface — unused ones rot, and
removed-but-unreferenced identifiers leave silent gaps in the next
spec that comes along expecting them.

If you're working in feature code and find yourself wanting to tag a
widget "for future tests", stop. The lint rules will not block you,
but the convention is that identifiers and the specs that query them
land together.

## Where identifiers actually reach the DOM (and where they don't)

This is the part you'll want to read before annotating anything.
Flutter Web's accessibility tree is aggressively merged and pruned on
its way to DOM — not every `Semantics(identifier: …)` survives
serialization. Phase-0 and Phase-2 of #2162 mapped this out:

- **Scaffold.body-level wrappers reliably reach DOM.** The
  `web-event-screen` anchor on `WebExperienceScreen` works because
  it sits at `Scaffold.body` depth with `explicitChildNodes: true,
  container: true` — the engine has nothing to merge it into. Use
  this pattern for top-of-route landmarks.
- **`Tappable.semanticsIdentifier`** (and the matching params on
  `Toggle`, `IconAction`, `CachedMediaImage`) is forwarded to the
  primitive's internal `Semantics(identifier: …)`, but the engine
  merges that into descendant subtrees and **drops it from DOM
  serialization** in practice. A Playwright `page.locator('[flt-
  semantics-identifier="…"]')` typically will not find an
  identifier set this way. The parameter still has a non-test
  purpose (it's queryable from in-process Dart widget tests), but
  Playwright should not depend on it.
- **Deeply-nested explicit wrappers** — `Semantics(identifier: …,
  explicitChildNodes: true, container: true)` placed inside a route
  subtree (e.g., on a list-row valueBuilder) — get pruned for being
  "uninteresting": no label, no button, no role. Adding
  `liveRegion: true` does **not** save them.

### What to do instead

For widgets that aren't reachable via an anchor:

1. Use **Playwright's accessibility-tree locators**
   (`page.getByRole('button', { name: 'View attendees' })`,
   `page.getByLabel(…)`). These read the browser's accessibility
   API, which has more nodes than the DOM exposes, and they find
   the localized `semanticsLabel` on any `Tappable` / `Toggle` /
   `IconAction`.
2. For "state truth" (count, status, list contents), use a **server
   RPC via `e2e/lib/connect.ts`**'s `createTestClient`. The bound
   widget tree reads from these RPCs; if the RPC says count=1, the
   UI reflects 1 — separating the "did the screen render" question
   from the "what value is shown" question.
3. For visual concerns (rendered text alignment, colour, layout),
   use `toHaveScreenshot` golden diffs as a last resort.

Detailed assertion strategy and worked examples live in
[`e2e/README.md`](../../../e2e/README.md) § "What's reliably
reachable on Flutter Web".

## Adding a route-level anchor (the reliable case)

```dart
Scaffold(
  body: Semantics(
    explicitChildNodes: true,
    container: true,
    identifier: 'web-event-screen',
    child: Stack(/* … */),
  ),
)
```

Playwright queries it via
`page.locator('[flt-semantics-identifier="web-event-screen"]')`.

## Nested interactive widgets are swallowed (hoist them out)

The merge that drops primitive identifiers (above) also eats **whole
interactive widgets** nested inside another interactive widget's
subtree: a `Toggle` or `Tappable` rendered *inside* a row-level
`Tappable`'s child tree is merged into the row's single semantics
node. On Flutter Web that means no locator finds the inner widget —
not by role, not by label — and taps aimed at it land on the row.

The fix is structural, not a locator trick: render per-row actions as
**siblings** of the tappable area, not descendants. Worked example —
the request helpers panel's accept chip (#2702),
`request_helpers_panel.dart`:

```dart
// ✗ swallowed: the Toggle lives inside the row's Tappable subtree
Tappable(semanticsLabel: label, onTap: openRow,
  child: Row(children: [leading, title, acceptToggle]))

// ✓ reachable: the Toggle is a sibling of the tappable area
Row(children: [
  Expanded(child: Tappable(semanticsLabel: label, onTap: openRow,
    child: Row(children: [leading, title]))),
  acceptToggle,
])
```

This matters for real users too, not just tests: the swallowed inner
widget is equally invisible to screen readers.

## Adding a primitive-level identifier (the Dart-widget-test case)

If a Dart widget test under `app/test/` needs to find a primitive,
the parameter pass-through is the supported path:

```diff
 Tappable(
   semanticsLabel: context.l10n.a11yRsvpYes,
+  semanticsIdentifier: 'event-hero-rsvp-yes',
   onTap: _handleRsvp,
   child: ...,
 )
```

**Do not** assume a Playwright spec will locate this via DOM. If the
Playwright spec is the consumer, use a role/label locator instead
(see § "What to do instead" above) and skip the
`semanticsIdentifier:` parameter.

## Choosing the value

Format: `{surface}-{component}-{role-if-needed}`. Examples:

- `web-event-screen` — top-level route landmark
- `event-hero-video-player` — the hero video element
- `event-hero-rsvp-yes` / `event-hero-rsvp-no` — RSVP toggle buttons
- `email-capture-email-field` — input on the email-capture form
- `email-capture-submit-button` — submit button on the email-capture form
- `feed-empty-state` — empty-feed placeholder
- `feed-first-card` — first card in the feed list

Identifiers must be unique within the rendered tree at any given time.
Two widgets with the same identifier on screen at once means
Playwright sees two matches and either picks one ambiguously or
throws. If a widget repeats (a list item, a poll proposal), append
a stable suffix derived from data, e.g.
`gear-card-${gear.id}` — *but* the test still has to know the ID to
query it, which is friction. Prefer to anchor with the parent
container and let the test scope from there.

## Lint enforcement

`scripts/check_a11y.js` enforces:

| Rule | What it does |
|---|---|
| `require_kebab_case_for_semantics_identifier` | Identifier values must match `^[a-z0-9-]+$`. Empty strings, spaces, camelCase, snake_case all fail. |

The rule follows the empty-allowlist ratchet pattern used by the
other a11y rules — adding to the allowlist on `main` requires
removing an existing entry.

The existing `require_l10n_for_semantic_labels` rule **does not**
apply to `identifier:` — verified by the regex in
`scripts/check_a11y.js`. So raw kebab strings on `identifier:` and
`semanticsIdentifier:` are fine, while raw strings on `label:`,
`tooltip:`, `semanticLabel:`, etc. continue to be rejected.

## See also

- [`app/lib/presentation/widgets/accessibility/README.md`](../../../app/lib/presentation/widgets/accessibility/README.md) — primitives.
- [`docs/client/accessibility.md`](../accessibility.md) — broader a11y discipline.
- [`docs/issues/2162-web-e2e-harness.md`](../../issues/2162-web-e2e-harness.md) — the harness plan.
- [`e2e/README.md`](../../../e2e/README.md) — the harness itself.
