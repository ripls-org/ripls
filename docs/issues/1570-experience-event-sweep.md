# Issue #1570: Sweep remaining user-visible 'Experience' strings and rename to 'Event'

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1570
**Filed:** 2026-04-29 | **Last activity:** 2026-04-30
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** none provided (relevant existing docs read: `CLAUDE.md` UI Terminology section, `docs/client/i18n.md`, `docs/client/accessibility.md`, `docs/client/architecture.md`, `docs/client/testing.md`, `docs/server/observability.md`, `scripts/check_i18n.js`, `scripts/check_a11y.js`)

## Audit Findings (2026-04-30)

Audited against `docs/client/architecture.md`, `docs/client/i18n.md`,
`docs/client/accessibility.md`, `docs/client/testing.md`,
`docs/server/observability.md`, and CLAUDE.md. Issues found and
addressed below; corrections folded into the relevant phases.

- **Overlap with `scripts/check_a11y.js` (now in Phase 2 — Q4
  refined).** The existing `require_l10n_for_semantic_labels` rule
  already rejects raw string literals on `tooltip:`,
  `semanticLabel:`, and `Semantics(label/value/hint:)` across
  `app/lib/presentation/`. The new Dart-side lint must **not**
  re-check those API surfaces — doing so would create a second gate
  on the same code paths and conflict with the
  `scripts/a11y_allowlist.txt` ratchet semantics. The new lint's API
  surface is now scoped to APIs that `check_a11y.js` does **not**
  cover: `Text(...)`, `hintText:`, `helperText:`, `errorText:`,
  `labelText:`, `SnackBar(content: Text(...))`, `AlertDialog`,
  `ToastHelper.show*` first arg.
- **Server log lines must be excluded from the Go lint (now explicit
  in Phase 1).** Per `docs/server/observability.md`, structured-log
  message strings ("failed to update experience state", etc.) are
  operator-facing and stay as "experience". The Go lint now
  explicitly filters out lines whose first non-whitespace token is a
  logger call (`logger.`, `slog.`, `log.Printf`, `log.Println`,
  `fmt.Errorf`, `connecterr.Internal`, `connect.NewError`).
- **a11y key naming (Phase 2 ARB section).** Per
  `docs/client/accessibility.md`, accessibility-only keys use the
  `a11y` prefix. None of the 11 new keys here are a11y-only — they
  feed `Text(...)`, `hintText:`, error toasts, etc. — so the
  `experience*` / `feedAction*` / `qt*` prefixes are correct.
  `accessibility.md` is the source of truth for any future
  `Semantics(label:)` strings the implementer adds.
- **Drill-down helpers and viewmodel rule (Phase 2 refactor).**
  `docs/client/i18n.md` rule "Viewmodels never resolve strings"
  applies recursively to helpers that produce text. The two
  `core/utils/drill_down/` helpers must return typed values (enum or
  key constant) and the widget calls `context.l10n.X` — option (a)
  in the original plan, now confirmed as the only acceptable shape.
- **Allowlist mechanism (Phase 2 lint).** `accessibility.md`
  documents `scripts/a11y_allowlist.txt` as an empty CI-enforced
  ratchet (entries shrink, never grow). For this lint we use **only**
  the inline `// experience-string-allow:` hatch — no separate
  allowlist file — keeping the implementer's path of least
  resistance toward fixing rather than allowlisting. If review wants
  a ratchet file later, it can be added without changing the hatch
  semantics.
- **Test layer per `docs/client/testing.md` (Phase 2 tests).**
  Tests for these strings are widget tests — the strings drive
  `Text` / `hintText` / `errorText` rendering and do not exercise
  ViewModel state. Keep tests in `app/test/presentation/screens/`
  (mirroring source layout) using `app/test/helpers/l10n_helpers.dart`,
  not `ProviderContainer`. ViewModel tests would not surface a string
  regression here.

## Problem Statement

`CLAUDE.md` codifies the rule that any string a user can read or hear says
**Event**, while code identifiers, proto messages, RPC names, file paths,
and internal docs/log lines stay **Experience**. The previous sweep
(#1107) cleaned up everything we touched plus the obvious widget-test
hits, but there are still stragglers in two places:

1. **Server code** that produces user-visible text — chat system
   messages, story-card copy, push-notification builders that fall
   outside the templates rewritten in #1107, and impact/feed copy that
   still says "experiences" colloquially.
2. **Client code** with hardcoded literals (mostly in the `experience/`
   screens and the impact/quality-time drill-downs) that escaped the
   ARB sweep.

The author also asks (2026-04-30 comment) for a regression-prevention
mechanism so we don't re-introduce these strings as the codebase
grows. We need both halves: clean up the existing stragglers, and
land a CI gate that fails on user-visible "Experience"/"experience"
literals outside an allowlist.

## Context from Comments

One comment from the author (project maintainer):

- **2026-04-30 — `@claude and consider whether there's a way to prevent
  regressions in the future as part of the plan`.** This expands scope
  beyond the original sweep: the plan must include a CI lint check, and
  it has to be precise enough to fail on real user-visible strings
  without flagging the ~200+ legitimate identifier matches
  (`ExperienceState`, `experienceProvider`, `experience_id`, etc.).

The original issue body is the spec. Out of scope is unchanged: code
identifiers, proto messages/enums, file paths, RPC names, server logs,
and internal docs. ARB `@key.description` translator notes are
optional but receive a consistency pass.

## Codebase Findings

### Server (Go) — user-visible literals to fix

| File:Line | Current text | Notes |
|-----------|--------------|-------|
| `server/services/experience/share.go:120` | `"%s created an experience: %s"` | Chat system message anchoring an experience conversation. |
| `server/services/experience/lifecycle.go:76` | `"%s started the experience"` | Chat system message on `MarkExperienceInProcess`. |
| `server/services/experience/lifecycle.go:321` | `"%s marked the experience complete"` | Chat system message on `CompleteExperience`. |
| `server/services/experience/lifecycle.go:571` | `"%s cancelled the experience"` | Chat system message on `CancelExperience`. |
| `server/story/templates.go:35` | `TitleTemplate: "Experience Completed"` | Story-card title for `STORY_TYPE_EXPERIENCE_CONCLUDED`. |
| `server/story/templates.go:105` | `"The experience %s was completed."` | Story-card description fallback. |
| `server/services/feed/nudges.go:239` | `"%d items shared, %d requests posted, %d experiences planned"` | User-facing nudge summary. |
| `server/impact_metrics/metric_detail_factors.go:528` | `"...saving %s through shared experiences."` | Impact-detail card body — colloquial use, but in an Events section. |

### Server tests that bake in the old text

- `server/community/notifications_test.go:731` — asserts `"New Experience"` (the production string already says "New Event"; this is a stale assertion that should be re-checked against the current source and updated to match).
- `server/story/generator_test.go:273` — table-driven case `{"Experience Concluded", "STORY_TYPE_EXPERIENCE_CONCLUDED"}` will need its title expectation flipped when the template changes.

`server/community/notifications.go` is **already correct** — the
`buildNotification` switch produces `"New Event"`, `"Event
Completed"`, `"Event has started"`, `"Event has been cancelled"`. No
change needed there.

`server/email/` and `server/chat/system_messages.go` produce no
user-visible "experience" copy directly (the chat system-message
*payloads* are constructed by the experience-lifecycle callers
above; the helpers in `system_messages.go` only handle structure).

### Client (Dart) — hardcoded literals to fix

All of these need to move to ARB entries (per `docs/client/i18n.md`,
"every user-visible string should use `context.l10n.*`"). Splitting
into ARB also makes the regression gate strict-mode in the
presentation layer.

| File:Line | Current text | Widget context |
|-----------|--------------|----------------|
| `app/lib/core/utils/experience_helper.dart:75` | `'shared an experience'` | `ExperienceHelper.getFeedActionText()`, used by `feed_screen.dart:465`. |
| `app/lib/presentation/screens/experience/experience_preview_modal.dart:185` | `'Experience description is required'` | `ToastHelper.showError()` content. |
| `app/lib/presentation/screens/experience/experience_preview_modal.dart:192` | `'Experience description must be at least 10 characters'` | `ToastHelper.showError()` content. |
| `app/lib/presentation/screens/experience/experience_preview_modal.dart:561` | `'Experience description'` | `TextField` `hintText`. |
| `app/lib/presentation/screens/experience/widgets/experience_event_pane.dart:405` | `'Experience description'` | `TextField` `hintText` (sibling to above). |
| `app/lib/presentation/screens/experience/experience_content_view.dart:439` | `'Failed to load experience details'` | Error-state `AlertDialog` title. |
| `app/lib/presentation/screens/experience/event_organizer_modal.dart:455` | `'Wrap up ${experience.name}'` | `TransferModalTitle.title`. The string is fine — `experience.name` is a proper name, not the word — but if we localize this we want a stable ARB key. |
| `app/lib/core/utils/drill_down/time_drill_down.dart:150` | `'Default assumed duration of a comparable experience or event...'` | Tooltip body. |
| `app/lib/core/utils/drill_down/quality_time_drill_down.dart:281` | `'Shared experience — mutual participation (1.0×).'` | Drill-down explanation. |
| `app/lib/presentation/widgets/impact/quality_time_detail_view.dart:430` | `'Shared experiences are your highest-yield category...'` | Modal explainer body. |
| `app/lib/presentation/widgets/impact/completion/quality_time_detail_modal.dart:272` | `'Quality Time measures...shared experiences...'` | Completion-modal explainer. |

Two of the drill-down helpers (`time_drill_down.dart`,
`quality_time_drill_down.dart`) are currently **outside the
`presentation/` tree**, which is what `scripts/check_i18n.js` scans.
Moving the strings into ARB and resolving in widgets is the standard
fix; the helper should return a typed enum or key, and the widget
calls `context.l10n.X`.

### ARB descriptions (translator hints)

51 `@key.description` entries in `app/lib/l10n/app_en.arb` still say
"experience" in the description field. Two have the
"(UI surface uses 'Event' — see CLAUDE.md UI Terminology rule)"
hint added during #1107. The issue marks this as optional — leaving
them is fine, but adding the hint consistently is cheap and helps
translators.

### Regression prevention — current state

`scripts/check_i18n.js` scans `app/lib/presentation/` for a fixed list
of common hardcoded strings (`Cancel`, `Save`, `Delete`, ...). It
does not look for "experience" today. Two more relevant scripts:

- `scripts/check_a11y.js` — example of a richer Dart-aware lint that
  greps for AST-shaped patterns and excludes generated code.
- `npm run lint:dart:size` (`scripts/check_dart_file_size.sh`) — example
  of an allowlist comment hatch (`// dart-line-count-allow`).

There is no existing Go-side string-literal lint. CI runs `npm run lint`
on every PR; adding two scripts to the chain is a one-line `package.json`
change.

## Potential Solutions

### Option A: Two-PR sweep (server + client) and a single CI gate

- **Approach.**
  - **PR 1 (server).** Update the eight server-side literals in chat
    system messages, story templates, the feed nudge, and the impact
    detail; update the two stale test assertions; add a Go-side lint
    `scripts/check_event_terminology_go.sh` that fails on a hardcoded
    list of user-visible string contexts containing "experience" /
    "Experience" outside an allowlist (with a comment hatch
    `// experience-string-allow: <reason>`).
  - **PR 2 (client).** Hoist each of the 11 hardcoded Dart literals to
    `app_en.arb` and `app_es.arb`, refactor the two
    `core/utils/drill_down/` helpers to return typed values resolved by
    widgets via `context.l10n`, run `npm run generate`, add widget tests
    for each newly-localized string, extend
    `scripts/check_i18n.js` (or add a sibling
    `scripts/check_event_terminology_dart.js`) so it scans all of
    `app/lib/` excluding `data/gen/`, `*.g.dart`, `*.freezed.dart`,
    log calls, comments, and identifier suffixes for
    "Experience"/"experience" string literals in user-visible
    contexts. Same `// experience-string-allow:` hatch.
  - **Both PRs** wire the new script(s) into `npm run lint`.
- **Pros.** Matches the maintainer's guidance ("split into two PRs").
  Each PR is independently testable. The two lints share a single
  pattern definition (we can DRY by putting the allowlist + hatch
  syntax in one place).
- **Cons.** The CI gate has to land in both PRs (Go-side in PR 1,
  Dart-side in PR 2), so the PRs land in series, not parallel. If the
  Dart lint is too strict it'll block the client PR until the
  allowlist is right.
- **Effort.** Medium. ~12 file edits + 11 ARB pairs + 2 lint scripts.

### Option B: Single PR for both server and client

- **Approach.** One PR covers all 19 string changes plus the lint(s).
- **Pros.** One round-trip, one review.
- **Cons.** Mixes Go and Dart changes in one PR, which the issue
  explicitly counsels against ("split into two PRs"). The Dart side
  needs `npm run generate` and ARB-locale work that's risky to mix
  with server semantics. Larger PR is harder to review.
- **Effort.** Medium, same total work; worse review experience.

### Option C: Lint-only ("rename later")

- **Approach.** Add the two lints with the current strings allowlisted,
  promising to clean up over time.
- **Pros.** Lowest immediate risk.
- **Cons.** Defeats the issue. The whole point is to clean these up
  now and prevent regressions; we already have a long allowlist
  candidate set. **Rejected.**
- **Effort.** Small but wrong shape.

## Recommendation

**Option A**, two PRs in series. It matches the maintainer's
explicit guidance, keeps each PR small enough to review carefully,
and lets the regression gate land alongside the corresponding fixes
(so the lint genuinely passes on a clean tree the moment it's
introduced). The PR sequencing is server-first because it has fewer
moving parts (no l10n machinery), so the lint pattern can stabilize
on Go before we apply the same shape to Dart.

The CI gates use the same shape as the existing
`scripts/check_i18n.js`: a small Node script that greps for offending
patterns, filters out comments / `data/gen/` / `*.g.dart` /
identifier suffixes / log calls, and prints violations with
file:line. The hatch comment lets us legitimately keep
`experience` in a user-visible string when it really is the right
word (e.g., a hypothetical "comparable experience or event" that
domain-deliberately uses both terms).

## Open Questions

- **Q1: Should `event_organizer_modal.dart:455`'s "Wrap up
  ${experience.name}" change?** The literal is "Wrap up " — it doesn't
  contain "experience" outside the variable name. — *Recommendation:*
  leave the string as-is (it doesn't trip the rule), but hoist it to
  ARB during PR 2 anyway since it's a user-visible literal and
  `docs/client/i18n.md` requires localization. Do not include it in
  the lint allowlist.
- **Q2: How should the Go lint identify "user-visible" string
  contexts?** Go has no presentation layer marker. — *Recommendation:*
  the lint scans a narrow allowlist of source files that we know
  produce user-visible text: `server/services/experience/{share,lifecycle,...}.go`,
  `server/community/notifications.go`, `server/story/templates.go`,
  `server/services/feed/nudges.go`, `server/impact_metrics/*.go`,
  and `server/email/*` templates. We do not try to scan the whole
  `server/` tree; doing so produces hundreds of identifier-y matches
  in storage and RPC code. A documented allowlist of "files known to
  produce user-visible strings" is a feature, not a limitation: it
  forces the implementer to think about whether any new file belongs
  in the list.
- **Q3: Should the ARB `@key.description` translator-note hint sweep
  ship in PR 2 or as a separate PR?** Issue marks it optional. —
  *Recommendation:* fold it into PR 2 as a final commit. The hint is
  one regex-replace per file, low-risk, and avoids a third PR for
  trivial work. If review wants to defer, drop the commit and ship.
- **Q4: Which categories of Dart literals does the regression lint
  cover?** Naively, "any literal containing 'Experience'/'experience'
  outside generated/log/identifier paths" — but identifiers like
  `'experience_state_active'` (analytics event name) trip on it. —
  *Recommendation (refined per audit):* the lint targets literals
  passed to user-visible widget APIs that **`scripts/check_a11y.js`
  does not already cover**: `Text(`, `hintText:`, `helperText:`,
  `errorText:`, `labelText:`, `SnackBar(content: Text(`,
  `AlertDialog(title:`, `AlertDialog(content:`, `ToastHelper.show*`
  first arg. **Excluded** (covered by the existing a11y rule
  `require_l10n_for_semantic_labels`): `Tooltip(message:`,
  `tooltip:`, `Semantics(label:` / `value:` / `hint:`,
  `semanticsLabel:`, `semanticLabel:`. This avoids two lints fighting
  over the same line and respects the
  `scripts/a11y_allowlist.txt` ratchet semantics
  (`docs/client/accessibility.md`).
- **Q5: The hatch comment.** Should it be `// experience-string-allow:`,
  or fold into the existing `// dart-line-count-allow:` semantics? —
  *Recommendation:* dedicated `// experience-string-allow: <reason>`
  on the line above the literal. Different gate, different reason
  format, different hatch keeps them independent. (The hatch carries
  a free-text reason so we can grep for them later and audit the
  exception set.)
- **Q6: Should we also address the 51 `experience_id` /
  `experience.name` references in tests, debug logs, and analytics?**
  No — these are identifiers and stay per CLAUDE.md.
- **Q7: Translation-locale impact.** Adding new ARB keys requires
  updating `app_es.arb` too. — *Recommendation:* both PR 2 commits
  add Spanish translations. Use the same translation patterns
  established in #1107 (search the existing `experience*` Spanish
  values for tone/register).

## Implementation Plan

### Phase 1: Server-side sweep + Go regression lint (PR 1)

**Goal:** Fix every user-visible Go literal containing "experience",
update stale test assertions, and add a CI gate that fails on new
violations in the same files.

**Tasks:**

*Code changes:*
- [ ] `server/services/experience/share.go:120` — change
      `"%s created an experience: %s"` to `"%s created an event: %s"`.
- [ ] `server/services/experience/lifecycle.go:76` — change
      `"%s started the experience"` to `"%s started the event"`.
- [ ] `server/services/experience/lifecycle.go:321` — change
      `"%s marked the experience complete"` to
      `"%s marked the event complete"`.
- [ ] `server/services/experience/lifecycle.go:571` — change
      `"%s cancelled the experience"` to `"%s cancelled the event"`.
- [ ] `server/story/templates.go:35` — change `TitleTemplate:
      "Experience Completed"` to `TitleTemplate: "Event Completed"`.
- [ ] `server/story/templates.go:105` — change
      `"The experience %s was completed."` to
      `"The event %s was completed."`.
- [ ] `server/services/feed/nudges.go:239` — change
      `"%d items shared, %d requests posted, %d experiences planned"`
      to `"%d items shared, %d requests posted, %d events planned"`.
- [ ] `server/impact_metrics/metric_detail_factors.go:528` — change
      `"...saving %s through shared experiences."` to
      `"...saving %s through shared events."`.

*Test updates:*
- [ ] `server/community/notifications_test.go:731` — re-read against
      the current production string (`buildNotification` now produces
      `"New Event"`); update expected to match. If the test already
      passes today, no change is needed; verify.
- [ ] `server/story/generator_test.go:273` — change the table-driven
      case from `{"Experience Concluded", ...}` to
      `{"Event Concluded", ...}` to match the new title template.
      (Confirm `generator.go` doesn't independently produce the title
      — if it does, that's the source of truth and the template
      string above must match it.)

*Regression lint (Go):*
- [ ] Add `scripts/check_event_terminology_go.sh` (bash + ripgrep, to
      match the style of `scripts/check_dart_file_size.sh`). It scans
      a hardcoded allowlist of files known to produce user-visible
      strings:
      ```
      server/services/experience/share.go
      server/services/experience/lifecycle.go
      server/services/experience/(any other *.go that constructs system messages)
      server/community/notifications.go
      server/community/events.go
      server/story/templates.go
      server/services/feed/nudges.go
      server/impact_metrics/metric_detail_factors.go
      server/email/*.html
      server/email/*.txt
      ```
      For each file, fail on a string-literal regex
      `"[^"]*[eE]xperience[^"]*"` or
      `` `[^`]*[eE]xperience[^`]*` `` *unless* the line above carries
      `// experience-string-allow: <reason>`. Identifier-y matches
      where the literal is a struct/proto field name (e.g.
      `"experience_id"`, `"ExperienceState"`) are filtered out by an
      explicit allowlist of known internal-key patterns.
      Server-log call sites are excluded by skipping any line whose
      first non-whitespace token matches one of: `logger.`, `slog.`,
      `log.Printf`, `log.Println`, `fmt.Errorf`,
      `connecterr.Internal`, `connect.NewError`. (Per
      `docs/server/observability.md`, log message strings are
      operator-facing and stay as "experience".)
- [ ] Add `npm run lint:go:event-terminology` in `package.json` and
      append it to `npm run lint:go`.
- [ ] Document the script in `scripts/README_BENCHMARKS.md` (or a new
      `scripts/README.md` if one exists) — keep the doc one paragraph.
- [ ] Unit-test the lint by running it against the cleaned-up tree
      (must pass) and against a synthetic file containing a forbidden
      literal (must fail).

*Validation:*
- [ ] `npm run build` passes (lint + go build).
- [ ] `go test -tags=integration -timeout 20m ./...` passes.
- [ ] `npm run lint:go:event-terminology` passes on the cleaned tree
      and fails when reverting any of the 8 string changes above
      (smoke-test).
- [ ] Manually verify the chat anchor message rendering for a newly-
      shared experience (system message text should now read
      `"… created an event: …"`).

**Validates:** All known server-side stragglers are gone and CI
catches new ones in known-user-visible files.

### Phase 2: Client-side sweep + Dart regression lint (PR 2)

**Goal:** Hoist every hardcoded Dart literal containing "experience"
in a user-visible widget API into ARB, with both English and Spanish
values; refactor the two drill-down helpers so widgets resolve text;
add widget tests that exercise the new ARB keys; add a CI gate.

**Tasks:**

*ARB additions* (both `app/lib/l10n/app_en.arb` and
`app_es.arb` — Spanish translations follow the patterns established
in #1107):
- [ ] `feedActionExperienceShared` → "shared an event" / "compartió un evento"
      (replaces `ExperienceHelper.getFeedActionText()`).
- [ ] `experienceDescriptionRequired` → "Event description is required" / "La descripción del evento es obligatoria"
      (replaces `experience_preview_modal.dart:185`).
- [ ] `experienceDescriptionMinLength` → "Event description must be at least 10 characters" / "La descripción del evento debe tener al menos 10 caracteres"
      (replaces `experience_preview_modal.dart:192`).
- [ ] `experienceDescriptionHint` → "Event description" / "Descripción del evento"
      (replaces both `experience_preview_modal.dart:561` and
      `experience_event_pane.dart:405`).
- [ ] `experienceDetailsLoadFailed` → "Failed to load event details" / "No se pudieron cargar los detalles del evento"
      (replaces `experience_content_view.dart:439`).
- [ ] `experienceWrapUpTitle` → "Wrap up {name}" / "Concluir {name}"
      (replaces `event_organizer_modal.dart:455`; declare `name`
      placeholder in `@key`).
- [ ] `qtTimeDrillDownComparableEvent` → "Default assumed duration of a comparable event ..." / "Duración asumida por defecto de un evento comparable ..."
      (replaces `time_drill_down.dart:150`).
- [ ] `qtMutualReciprocityDescription` → "Shared event — mutual participation (1.0×)." / "Evento compartido — participación mutua (1,0×)."
      (replaces `quality_time_drill_down.dart:281`).
- [ ] `qtSharedEventsHighestYield` → "Shared events are your highest-yield category ..." / Spanish equivalent
      (replaces `quality_time_detail_view.dart:430`).
- [ ] `qtMeasuresSharedEvents` → "Quality Time measures ... shared events ..." / Spanish equivalent
      (replaces `quality_time_detail_modal.dart:272`).

For each new key, include `@key` metadata with `description`,
`placeholders` (where applicable), and the
"(UI surface uses 'Event' — see CLAUDE.md UI Terminology rule)" hint.

*Code refactors:*
- [ ] `app/lib/core/utils/experience_helper.dart` — change
      `getFeedActionText()` to take `BuildContext` and return
      `context.l10n.feedActionExperienceShared`; or, more cleanly,
      delete the helper method and inline `context.l10n.feedActionExperienceShared`
      at `feed_screen.dart:465`. (Match the pattern used by
      `CommunityHelper.getFeedActionText()` and `GearHelper.getFeedActionText(...)`.)
- [ ] `app/lib/core/utils/drill_down/time_drill_down.dart` and
      `quality_time_drill_down.dart` — these helpers currently return
      raw strings. Two options: (a) have them return a typed enum /
      key that the widget resolves to `context.l10n.X`, or (b) pass
      `BuildContext` in. Prefer (a) for testability — `time_drill_down`
      methods become pure functions returning a typed result, and the
      widget calls `context.l10n` on render. The exact shape depends
      on how the helper is called; pick whichever produces the
      smallest diff but follow `docs/client/i18n.md` "viewmodels never
      resolve strings".
- [ ] All other call sites: replace the literal with
      `context.l10n.<keyName>` (or `AppLocalizations.of(context).<keyName>`
      where `context.l10n` isn't in scope — match local style).

*Regression lint (Dart):*
- [ ] Extend `scripts/check_i18n.js`, OR add a sibling
      `scripts/check_event_terminology_dart.js` (preferred — keeps the
      i18n script focused on the small fixed-list of common words).
      The new script:
      - Scans all of `app/lib/` excluding `app/lib/data/gen/`,
        `*.g.dart`, `*.freezed.dart`, `app/lib/l10n/app_localizations*.dart`.
      - Greps for string literals containing `Experience`/`experience`
        passed to a user-visible widget API. Allowlist of APIs (from
        Q4 above) — it's the same regex shape as `check_i18n.js`.
      - Skips lines containing `// experience-string-allow: <reason>`
        on the line above (mirroring the file-size hatch convention).
      - Skips lines that are themselves comments
        (`^\s*(//|\*|///)`).
      - Skips lines that are clearly identifiers
        (the literal is `'experience_id'`, `'experience.name'`,
        analytics event names — matched by a separate "internal key"
        regex).
      - Prints violations as `file:line: <literal>` and exits 1.
- [ ] Add `npm run lint:dart:event-terminology` in `package.json` and
      append it to `npm run lint`.

*Tests:*
- [ ] Update or add widget tests for each of the 11 hoisted strings.
      Use `app/test/helpers/l10n_helpers.dart` per `docs/client/i18n.md`.
      For each, render the surface and assert the new ARB key resolves
      in both `en` and `es`. (For the existing screens with widget
      tests, just update expectations; for those without, add the
      smallest test that exercises the affected branch.)
- [ ] Smoke test: `npm run lint:dart:event-terminology` passes on the
      cleaned tree and fails when reverting any of the changes
      (verify by hand on one literal).
- [ ] `flutter test --reporter github` from `app/`.
- [ ] `npm run lint` (full chain) is green.

*Optional ARB-description sweep (final commit of PR 2):*
- [ ] For each `@key.description` entry in `app/lib/l10n/app_en.arb`
      whose key is `experience*`, append the
      "(UI surface uses 'Event' — see CLAUDE.md UI Terminology rule)"
      suffix if it isn't already there. ~49 small edits in one
      sed-able pass. Re-run `npm run generate`.
- [ ] No code change is needed in `app_es.arb` for the description
      sweep — descriptions are translator-facing.

**Validates:** Every Dart user-visible literal containing "experience"
is gone or allowlisted; CI catches new violations; both locales
render correctly.

### Phase 3: Document the rule in `docs/client/i18n.md` (optional, fold into PR 2)

**Goal:** Make the regression rule discoverable from the docs the
implementer reads first.

**Tasks:**
- [ ] Add a "User-facing terminology" subsection to
      `docs/client/i18n.md` summarizing the
      `Experience` → `Event` rule (one paragraph, link to
      `CLAUDE.md`).
- [ ] Add a one-paragraph "Server-side user-visible strings"
      subsection to `docs/server/architecture.md` (or a new
      `docs/server/user_strings.md`) describing the Go-side lint and
      the file allowlist.
- [ ] Mention the `// experience-string-allow:` hatch in both docs
      with one example.

**Validates:** Future contributors find the rule and the lint without
having to grep.

## Related Issues

- **#1107 (per-community notifications)** — Origin issue. Did the
  bulk of the rename for strings touched along the way; left the
  stragglers this issue is sweeping.
- **#1340 / #1107 follow-ups** — Other UI polish from the same family.
- **CLAUDE.md "UI Terminology: 'Event', not 'Experience'"** — The
  authoritative rule. This issue does not change the rule, only
  enforces it.
