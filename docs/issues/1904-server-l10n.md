# Issue #1904: Add localization / internationalization for server-side strings

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1904
**Filed:** 2026-05-13 | **Last activity:** 2026-05-20
**Labels:** none | **Assignees:** maintainer
**Priority:** P1 (closed-beta blocker per triage 2026-05-19)
**Context docs:** `docs/server/l10n.md` (render boundary, locale-resolution order, contributor workflow — committed 2026-05-24)

## Status (2026-05-24)

| Phase | Status | PR | Notes |
|-------|--------|------|-------|
| Phase 0 — Scaffolding | ✅ Shipped | #2153 (commit `61c7596`) | server/l10n package, design doc, empty TOML catalogs, 24 tests |
| Phase 1 — User identity (locale storage + transport) | ✅ Shipped | #2153 (commit `b5c8cae`) | `preferred_language` on User proto (field 19; 17 was taken by `simulation_id`), AcceptLanguage middleware, registration seeding, client UserRepository.updatePreferredLanguage, disposal-safety tests |
| Phase 2 — Push notifications | ✅ Shipped | #2153 (commit `8612f73`) | ~50 keys in en + es; community + chat subscribers refactored to per-locale render; EN tests preserved, ES tests added |
| Phase 3 — Email (user-facing only) | ✅ Shipped | this branch | password_reset + waitlist_welcome localized (en/es); admin emails (activity digest, waitlist notification) intentionally English-only |
| Phase 4a — RPC error infrastructure | ✅ Shipped | this branch | LocalizedErrorDetail proto, connecterr.UserVisible helper, client UserError.localizedCode variant; first exemplar (DeleteExperience permission check) migrated |
| Phase 4b — Stories + chat system messages migration | ✅ Shipped | this branch | Schema + client resolver shipped; 5 story templates migrated, all 40 chat emit sites migrated (transfer/experience/request lifecycle + interest/undo/poll/share/save/rsvp/offers/community-gear/planning); remaining sites use generic fallback only when server is newer than client |
| Phase 4c — Bulk RPC-error migration | ✅ Shipped | this branch | All clearly-user-facing ownership/state-precondition errors migrated to connecterr.UserVisible across gear / request / transfer / community / experience / media / esm / workshop / chat services. Auth-style errors (e.g. "not authorized to view this conversation") remain on per-Code rendering — that's the correct generic UX |
| Phase 5 — AI-generated content | ✅ Shipped | this branch | Nudge + impact-row narrative prompts carry recipient locale via top-of-prompt directive; ImpactRowPromptVersion bumped to v2 |
| Phase 6 — Guardrails | ✅ Shipped | this branch | TOML parity script + inline-strings guard wired into `npm run lint:go`; runtime warn-once already shipped in Phase 0; contributor docs updated |

PR #2153 is open against `main` with Phases 0–2 bundled. Merge that before starting Phase 3 so the Localizer + AcceptLanguage middleware are on `main`.

### Key files and APIs landed in Phases 0–2

A future session continuing this work should know these entry points exist (verify with `grep` / `Read` before relying on any of them — memories age):

- **Server library:** `server/l10n/` — `Bundle()`, `MustBundle()`, `NewLocalizer(language.Tag)`, `NewLocalizerForContext(ctx)`, `Localizer.T(ctx, key, params)`, `Normalize(tag)`, `Supported()`, `DefaultTag` (= `language.English`), `WithLocale`/`WithAcceptLanguage`/`WithPreferredLanguage` context helpers, `LocaleFromContext(ctx)`.
- **Catalog source:** `server/l10n/source/{en,es}.toml` (go-i18n v2 format, ICU-style `{{.Param}}` placeholders, `other = "..."` for non-plural messages). Embedded via `go:embed`.
- **Middleware:** `server/middleware/accept_language.go` — captures `Accept-Language` header into ctx via `l10n.WithAcceptLanguage`. Wired in `server/main.go` muxStack alongside `RequestID` and `RemoteAddr`.
- **Proto fields (added in Phase 1):**
  - `models.User.preferred_language` — `optional string`, field **19** (not 17 — `simulation_id` already had 17).
  - `api.User.preferred_language` — field 8.
  - `api.GetUserResponse.preferred_language` — field 10.
  - `api.SaveUserRequest.preferred_language` — `optional string`, field 9.
- **Registration seeding helper:** `server/services/login/helpers.go::seedPreferredLanguage(ctx, user)` — called from EmailRegister, OIDCRegister, PhoneRegister before insert.
- **Client transport:** `RpcUtils.configureLocaleGetter(() => String?)` in `app/lib/core/utils/rpc_utils.dart`; wired in `app/lib/main.dart::initState` to read `localePreferenceProvider` with device-locale fallback.
- **Client repository:** `UserRepository.updatePreferredLanguage(userId, languageCode)` in `app/lib/data/repositories/user_repository.dart`.
- **Client viewmodel:** `LocalePreferenceNotifier.setLocale` in `app/lib/presentation/viewmodels/locale_view_model.dart` mirrors to server fire-and-forget with `ref.mounted` + try/catch guards.
- **Per-recipient render pattern (Phase 2 reference for Phase 3):** Subscribers batch-load `User.preferred_language` for all recipients via `s.GetByIDs(ctx, ids, &models.User{})` then resolve via `l10n.Normalize`. Render is cached per-locale in a `map[language.Tag]*models.Notification` so a fan-out renders each language once. See `server/notifications/community_subscriber/subscriber.go::loadRecipientLocales` and `server/notifications/chat_subscriber/subscriber.go::loadRecipientLocales`.

### Decisions captured during Phases 0–2 (relevant for later phases)

- **Field number for `preferred_language` is 19, not 17.** The plan originally said 17, but that tag was already taken by `simulation_id`. Field 19 placed semantically next to `preferred_timezone` (field 16) in declaration order. **Phase 3+ should add new proto fields with the same care** — check `grep -n "= [0-9]\+" proto/ripls/models/user.proto` for the next free tag.
- **TOML chosen for go-i18n source format.** Already committed; do not revisit.
- **English catalog is the source of truth.** ES translations were authored in the same PR as the EN extraction. Future phases should follow the same pattern: add the EN key first, then translate, in one PR.
- **Sender names, gear names, request titles, experience names, community names are never translated.** They round-trip in whatever language the author typed them in. Templates parameterize them as `{{.ActorName}}`, `{{.GearName}}`, etc.
- **Chat author message body (`UserChatMessage.text`) is never translated.** Only the frame (sender-fallback name + "mentioned you" suffix) is localized.
- **Subscribers cache per-locale.** Reuse this pattern in Phase 3 if an email fan-out has multiple recipients in different languages.

### Outstanding items not on a phase

- `app/pubspec.lock` had a stray unrelated `meta`/`test` minor-version bump in the working tree that predated this work. It was deliberately excluded from PR #2153. Either commit it separately to main or revert before starting Phase 3.

## Audit Notes (2026-05-23)

Audited against `docs/client/i18n.md`, `docs/client/architecture.md`,
`docs/client/testing.md`, `docs/client/caching.md`,
`docs/client/accessibility.md`, `docs/server/architecture.md`, and
`docs/server/observability.md`. Corrections applied:

- **ARB access pattern.** Client uses `context.l10n.*` (extension in
  `app/lib/core/extensions/l10n_extensions.dart`), not
  `AppLocalizations.of(context)`. Plan now uses the canonical form.
- **ARB key prefix for server errors.** `rpcError*` is the established
  prefix per `docs/client/i18n.md`. Phase 4 error keys now follow it
  (was `error*` in the draft).
- **Client error-state model.** The canonical shape is
  `UserError? error` on viewmodel state with
  `RpcErrorHandler.classify` / `RpcErrorHandler.localize`
  (`app/lib/core/errors/README.md`). Phase 4 routes the server's
  user-visible error code + params through `RpcErrorHandler` rather
  than via a new `_extractServerUserMessage()` shape.
- **Viewmodels never resolve strings.** Per `docs/client/i18n.md` and
  `docs/client/architecture.md`. Phase 4 client tasks state explicitly
  that the resolution happens in widget `build()` via `context.l10n`.
- **Reuse existing CI scripts.** Phase 6 now extends
  `scripts/check_l10n_parity.js` (already wired to
  `npm run lint:dart:l10n-parity`) instead of inventing a parallel
  parity check. Same for `scripts/check_i18n.js`
  (`npm run lint:dart:i18n`) — kept as the Dart-side hardcoded-string
  guard; the new Go-side guard (`check_no_inline_user_strings.js`)
  remains net-new because there is no Go equivalent today.
- **`connecterr.Internal` public string.** Per
  `docs/server/architecture.md` it is the fixed string
  `"internal server error"` (lowercased), not
  `"An internal error occurred"` as the draft claimed. Plan corrected.
- **Repository pattern for profile updates.** Per
  `docs/client/architecture.md`, ViewModels go through repositories,
  not services. Phase 1's locale-write-on-change now goes through
  `UserRepository` (which handles the underlying RPC and cache
  invalidation).
- **Structured logging usage.** Runtime "missing translation" warning
  must use `logging.LoggerWithContext(ctx)` (the standard helper from
  `docs/server/observability.md`), not raw `slog`. Phase 6 corrected.
- **A11y label sourcing.** Anywhere a server-emitted string ends up as
  an accessibility label or screen-reader announcement, the
  client-side ARB key needs the `a11y` prefix per
  `docs/client/accessibility.md`. Flagged in Phase 4 (notably story
  prose announced via `LiveRegion` / `SemanticAnnouncer`).
- **Server library, not service.** `server/l10n/` already structured as
  a shared library (per `docs/server/architecture.md` §1 "Service
  Independence"). No change.
- **Disposal-safety tests.** Per `docs/client/testing.md`, the new
  async locale-persist path in `LocalePreferenceNotifier` needs a
  disposal-safety test. Added to Phase 1 task list.

## Problem Statement

The server emits user-visible strings — push notification titles/bodies, email
subjects and templates, in-app chat system messages, generated story prose,
nudge headlines, and a small number of user-visible RPC error messages — and
every one of them is hardcoded English. The Flutter client already ships
localized strings in `en` and `es` via ARB files and respects a user-selected
locale in `MaterialApp.locale`, but that locale is **never sent to the server**:
no `Accept-Language` header, no `preferred_language` on the `User` or
`UserDevice` protos. The server has no way to know what language a recipient
prefers and no infrastructure to render in anything but English even if it did.

A code-scan inventory found roughly 150–200 distinct user-visible strings
across six surfaces. The key inventory observations that drive the plan:

- **Push notification copy** (~50 strings) lives in `fmt.Sprintf` calls in
  `server/notifications/community_subscriber/copy.go` (lines 14–182) and
  `server/notifications/chat_subscriber/copy.go` (lines 12–163). The server
  is the terminal renderer here — FCM/APNs payloads have pre-rendered
  `title` and `body` fields the OS reads directly.
- **Email content** (~30 strings + several HTML/text templates) in
  `server/email/mailgun.go`, `server/email/activity_digest_render.go`, and
  `server/email/*_template.{html,txt}`. SMTP target gets pre-rendered.
- **In-app story prose** (~10 templates) in `server/story/templates.go`,
  written into persisted rows and shown later in the app.
- **In-app chat *system* messages** (~25 strings) — server-generated
  `SystemChatMessage` rows (`proto/ripls/models/chat.proto:129`) keyed
  by `ChatSystemAction`, with copy assembled inline in
  `server/chat/system_messages.go` and in lifecycle files
  (`server/services/transfer/lifecycle.go:111`,
  `server/services/experience/lifecycle.go:74-75`). **User-typed chat
  messages (`UserChatMessage`, `chat.proto:105`) are explicitly out of
  scope** — those stay stored verbatim in the author's language and
  the app does not attempt to translate them. The phrase "chat
  messages" elsewhere in this plan means system chat messages only.
- **Nudge headlines/descriptions** — AI-generated, free-form. The
  proto comments in `proto/ripls/models/nudge.proto:80-87` already say
  "Pre-localized headline/description text (server-generated,
  locale-specific)" — the contract exists; the plumbing does not.
- **User-visible RPC errors** (~5–10 strings) sprinkled across service
  files, plus the fixed public string in `server/connecterr/internal.go`.

No existing i18n infrastructure on the server side. The User proto has
`preferred_timezone` (field 16) but no language field.

## Context from Comments

There is exactly one comment — a `/ripls-triage` bot escalation to P1 on
2026-05-19, rationale "closed-beta blocker or P1 carry-forward." No human
discussion. The architectural fork the issue body calls out
(client-renders-from-structured-data vs server-side l10n) has not been
resolved in-thread. The design captured below was decided in a session
discussion with the assignee on 2026-05-22.

## Potential Solutions

### Option A: Server-side l10n only (go-i18n catalog for everything)

- **Approach:** Standard go-i18n v2 setup. Server holds a translation
  catalog for every user-visible string, including stories, chat system
  messages, nudges, and errors. Client receives pre-rendered text and
  displays as-is.
- **Pros:** Single library; one place to add new server strings.
- **Cons:** Creates two parallel localization systems with overlapping
  scope — the server's catalog renders strings displayed inside the
  same screens as the client's ARB-rendered strings. This invites
  drift: translators update one side and not the other, terminology
  diverges between catalogs, the user sees inconsistent voice
  in-app. Also tight-couples adding a new in-app language to a
  server release.
- **Effort:** Medium-large.

### Option B: Hybrid — server renders push/email, client renders in-app — **recommended**

- **Approach:** Server renders only where it is the terminal emitter
  (push notifications, email, SMS). Everything rendered inside the
  app — stories, chat system messages, user-visible RPC error text —
  is refactored to a structured `{template_key, params}` payload that
  the client looks up in its ARB catalog. AI-generated nudge content
  remains rendered by the server but the AI is prompted in the
  recipient's locale (it can't be templated because the AI authors
  the prose itself).
- **Pros:**
  - One catalog per surface, no parallel-catalog drift. The
    client ARB is the single source of truth for everything the user
    reads in the app; the server catalog is the single source of
    truth for push and email.
  - Adding a new in-app language is a client-only release (modulo
    AI-content quality validation).
  - The recipient's *current* device locale wins for in-app content,
    even if it changed after the content was authored. The story
    written when the user was an English-speaker renders in Spanish
    if they switch their locale.
  - Server catalog stays small (~80 strings: push + email). Much
    less server-side translation surface to maintain.
- **Cons:**
  - Story and chat-message storage migrates from free-text to
    `{template_key, params_map}`. This is a real schema change with
    a backfill story for historical rows.
  - Two systems with different mechanics, even if they don't share
    keys. Engineers have to learn the boundary.
  - AI-generated nudges sit in an awkward middle ground: rendered
    server-side, but displayed in-app. They can't be templated, so
    the boundary doesn't cleanly apply. Plan treats them as a
    special case (see Phase 5).
- **Effort:** Large. The structured-template migration for stories and
  chat is the biggest single piece.

### Option C: Pure client-side rendering — send only structured data

- **Approach:** Server emits structured events with template keys
  everywhere; client renders push notifications too, via background
  notification-service extensions.
- **Pros:** Single l10n system (client ARB only).
- **Cons:** Push notifications cannot be done cleanly. iOS Notification
  Service Extensions can mutate payloads but a killed-app state on
  either platform can't render at all. Email and SMS have no client
  to do the rendering. Structural mismatch.
- **Effort:** Largest, with structural blockers on push.

## Recommendation

**Option B (hybrid)**. The defining principle:

> The client renders any string the user reads inside the app.
> The server renders only strings where the server is the terminal
> emitter (push notifications, emails, SMS).

This boundary is chosen specifically to avoid the parallel-catalog
drift risk of Option A. The two l10n systems do not share keys; their
message sets are largely disjoint (push/email vs UI labels), so they
do not need a shared catalog format and do not need a build-step
converter. The client keeps `app/lib/l10n/app_*.arb`; the server gets
its own catalog under `server/l10n/source/` in go-i18n's native
format. ARB-as-shared-source is **not** adopted — the disjoint
message sets don't justify the tooling cost.

Concrete decisions confirmed in design discussion:

1. **Locale on the user.** Add `preferred_language` to `models.User` as
   field 17, immediately after `preferred_timezone` (field 16),
   `optional string` holding a BCP-47 tag (`en`, `es`). On
   registration, seed from the registering request's `Accept-Language`
   header; default to `"en"` if absent.
2. **Transport.** Client adds `Accept-Language` to every RPC via
   `buildHeaders()` in `app/lib/core/utils/rpc_utils.dart`. When the
   user picks a new locale in `LocalePreferenceNotifier`, the client
   also persists it server-side via `UpdateUserProfile`.
3. **Locale resolution on the server.** `l10n.LocaleFromContext(ctx)`
   helper picks with priority: explicit RPC arg > `Accept-Language`
   header > user record's `preferred_language` > `en`.
4. **Server catalog.** `nicksnyder/go-i18n` v2. Source files at
   `server/l10n/source/{en,es}.toml` (or JSON — pick at scaffold
   time), embedded into the binary via `go:embed`. Catalog covers
   push notification copy, email subjects, and the localizable text
   inside email templates. **It does not contain story, chat, or
   in-app error text** — those live in the client ARB.
5. **In-app structured templates.** Stories and chat system messages
   migrate from free-text fields to `{template_key, params_map}`.
   The client renders them at fetch time from its own ARB. Phase 4
   covers the migration and the historical-row backfill.
6. **User-visible RPC errors.** Server returns a stable error code
   plus parameters; client maps the code to an ARB key. The fixed
   `connecterr.Internal` public string also becomes a code.
7. **AI-generated content (nudges, AI story prose if any).** Server
   prompts the AI in the recipient's locale: a top-of-prompt
   directive instructs the model to write in that language. AI
   output is the localized text directly. Not in a catalog on
   either side. Accepts a quality-validation risk addressed in
   Phase 5.

## Open Questions

- **AI-generated nudges live in a gray zone — they're rendered
  server-side but displayed in-app. Does this violate the "client
  renders in-app" principle?** — Recommendation: it does not, in
  practice. The boundary's purpose is to avoid drift between parallel
  static-string catalogs, and AI content lives in no catalog at all.
  Document the exception explicitly in `docs/server/l10n.md` so the
  next reader doesn't mistake nudges for a generic in-app server
  string.

- **Catalog source format on the server — TOML, JSON, or YAML?** —
  Recommendation: TOML. go-i18n v2 supports all three; TOML is the
  most readable for a translator working on dozens of short
  push/email strings with ICU plural rules. JSON is a fine second.

- **Schema for the structured story / chat-message template payload.**
  — Recommendation: `string template_key` + `map<string, string>
  params`. Keep the existing free-text field for one release as a
  transition fallback so we don't break currently-shipped clients.
  Mark the old field with a `TODO(#NNNN)` referencing a follow-up
  cleanup issue (file the cleanup issue before the change lands —
  see [[feedback-backfill-cleanup-issue]]).

- **Historical rows in story / chat-message tables — backfill or
  fallback?** — Recommendation: fallback. When the server returns a
  story whose `template_key` is empty (old row), include the
  English text in the legacy field and let the client display it
  unchanged. Don't try to reverse-engineer a key from the text.
  The legacy field gets deleted in the cleanup issue once historical
  rows age out or get re-rendered.

- **How does the server pick a locale for emails fired from a path
  without a live `User.preferred_language` (e.g. password reset for
  a brand-new account, waitlist welcome)?** — Recommendation:
  capture the `Accept-Language` of the triggering request and pass
  it through to the email-send code path; fall back to `en`.

- **Per-locale HTML/text email templates: one file per locale or
  one template with embedded localizer calls?** — Recommendation:
  one file per locale (`password_reset_template_en.html`,
  `password_reset_template_es.html`). Mixing go-i18n placeholders
  inside Go `html/template` becomes unreadable fast.

- **Catalog-completeness CI guardrail — strict (fail on missing key)
  or lenient (warn and fall back to en)?** — Recommendation:
  strict at CI (script asserts every server key present in `en` is
  also present in every other locale's source file), lenient at
  runtime (missing key logs a rate-limited Warn and renders en).
  Engineering catches the gap before users do.

- **AI nudge quality in `es` — how do we validate before shipping?** —
  Recommendation: integration test under `//go:build integration`
  that calls the nudge generator with `locale=es`, asserts non-empty
  output, and uses `golang.org/x/text/language.Detect` (or a small
  heuristic) to assert Spanish-detected. If quality is a concern
  later, add a translation-review process — but ship the plumbing
  unblocked.

- **Per-device vs per-user locale.** — Recommendation: per-user for
  v1. Multi-device users in mixed-language households are a real
  edge case but not closed-beta-critical. The data model on
  `UserDevice` can hold a locale later if we need it; today push
  goes to a user, not a device-locale-aware fanout.

- **Should `Accept-Language` parsing respect `q=` quality values?** —
  Recommendation: ship v1 with a "first supported tag wins"
  parser. RFC 7231 full parser is a follow-up worth filing if the
  simple version causes any real-world misrouting.

- **Where does the in-app locale picker live, if anywhere?** —
  Recommendation: it already exists at
  `app/lib/presentation/viewmodels/locale_view_model.dart` as
  `localePreferenceProvider`. Add a server-write side effect when
  it changes; no new UI work needed.

## Implementation Plan

### Phase 0: Scaffolding — server l10n package, design doc, no behavior change — ✅ SHIPPED (PR #2153, commit `61c7596`)
**Goal:** Lock the design and ship empty plumbing.
**Tasks:**
- [x] Commit `docs/server/l10n.md` documenting the boundary, the
  locale-resolution order, the catalog format, and the in-app
  template-key contract. This is the durable doc that backs the
  TODO comments and the contributor workflow.
- [x] Create `server/l10n/` package: `doc.go`, `README.md`,
  `Localizer` wrapper over `nicksnyder/go-i18n` v2,
  `LocaleFromContext(ctx)`, `Normalize(tag)`, `Supported`. Empty
  catalog — no real keys yet.
  - Also added: `Bundle()`/`MustBundle()` singletons; `WithLocale`/`WithAcceptLanguage`/`WithPreferredLanguage` ctx setters; `Localizer.T(ctx, key, params)` with nil-safe receiver + missing-key warn-once gate (`missLog sync.Map`); `ResetMissLogForTest()` for test isolation.
- [x] `server/l10n/source/{en,es}.toml` skeletons committed empty.
  `go:embed` wires them into the package.
- [x] Unit tests for `Normalize` (unknown tag → `en`, case folding,
  region stripping) and `LocaleFromContext` (priority order).
  - 24 tests across `locale_test.go` and `localizer_test.go`.
**Validates:** The library renders a single placeholder correctly
end-to-end. No production code uses it yet.

### Phase 1: User identity — locale storage and transport — ✅ SHIPPED (PR #2153, commit `b5c8cae`)
**Goal:** The server reliably knows the recipient's language for
every RPC and for every persisted user.
**Tasks:**
- [x] Add `optional string preferred_language = 17` to
  `proto/ripls/models/user.proto`, just below `preferred_timezone`.
  Regenerate. Update `server/services/user/` to persist via the
  existing profile-update RPC.
  - **Landed at field 19, not 17** — `simulation_id` already had 17. Placed semantically next to `preferred_timezone` in declaration order. Also added `optional string preferred_language` to `api.User` (field 8), `api.GetUserResponse.preferred_language` (field 10), `api.SaveUserRequest.preferred_language` (field 9, optional).
  - SaveUser handler (`server/services/user/service.go`) normalizes via `l10n.Normalize` and stores. Empty string clears.
- [x] On `RegisterUser` (and any account-creation path), seed the
  field from the request's `Accept-Language`, normalized via
  `l10n.Normalize`. Default `"en"`.
  - Helper `seedPreferredLanguage(ctx, user)` in `server/services/login/helpers.go`, called from EmailRegister, OIDCRegister, PhoneRegister before insert. Reads the locale via `l10n.LocaleFromContext(ctx)` which the AcceptLanguage middleware populates.
- [x] Client: `buildHeaders()` in
  `app/lib/core/utils/rpc_utils.dart` adds `Accept-Language` from
  `localePreferenceProvider`'s current value (falling back to the
  device locale).
  - Implemented as `RpcUtils.configureLocaleGetter(() => String?)` (matches the existing `configureTokenRefresh` pattern). `main.dart::initState` wires it to `ref.read(localePreferenceProvider).asData?.value?.languageCode ?? PlatformDispatcher.instance.locale.languageCode`.
- [x] Client: extend `LocalePreferenceNotifier` so picking a new
  locale calls a new `UserRepository.updatePreferredLanguage(tag)`
  method (per `docs/client/architecture.md` — ViewModels go through
  repositories, not services). The repository wraps the underlying
  `UpdateUserProfile` RPC and invalidates the `user:<id>` cache key.
  Fire-and-forget on the call site; don't block the UI.
  - `UserRepository.updatePreferredLanguage(userId, language)` delegates to `saveUser(preferredLanguage: language ?? '')` which invalidates the user + media_urls cache keys.
  - `LocalePreferenceNotifier._persistToServer(locale)` is fire-and-forget, wrapped in try/catch + `ref.mounted` guard. No-ops when no user is signed in (locale is captured into preferred_language on next registration from the Accept-Language header).
- [x] Round-trip tests on both sides confirming the header is set
  and the field is persisted. Include a disposal-safety test on
  `LocalePreferenceNotifier` (per `docs/client/testing.md` §
  "Disposal Testing") — the locale-persist call is async on an
  autoDispose-adjacent provider and must not crash if the user
  navigates away mid-flight.
  - Server: `server/middleware/accept_language_test.go` (header capture + absent-header passthrough); `server/services/login/preferred_language_test.go` (Accept-Language seeding + default fallback on EmailRegister); `server/services/user/roundtrip_test.go` extended for `preferred_language` round-trip + clear pass.
  - Client: `app/test/presentation/viewmodels/locale_view_model_test.dart` extended from 6 → 11 tests including server-persistence (signed-in / clear / not signed in) and two disposal-safety cases.
**Validates:** Every server handler can call `LocaleFromContext(ctx)`
and get a usable tag. Every persisted user has a non-empty
`preferred_language`.

### Phase 2: Push notifications — server catalog, highest user impact — ✅ SHIPPED (PR #2153, commit `8612f73`)
**Goal:** Every push notification ships in the recipient's language.
**Tasks:**
- [x] Extract every literal in
  `server/notifications/community_subscriber/copy.go` (lines 14–182)
  and `server/notifications/chat_subscriber/copy.go` (lines 12–163)
  into keys in `server/l10n/source/en.toml`. Naming convention:
  `notif.community_event.transfer_interest_expressed.title` /
  `.body`, etc. Note: the chat-notification *frame* ("Alice sent a
  message") is localized; the user's actual message body interpolated
  into the preview stays in whatever language the author typed.
  - ~50 keys total. Chat catalog: `notif.chat.sender_fallback`, `notif.chat.mentioned_you`. Community catalog covers every CommunityEventType that fires a push, including the COMMUNITY_DELETED fallback when the soft-deleted row read fails.
- [x] Translate to `es` in the same PR.
- [x] Refactor `buildNotification(ctx, s, event)` to look up the
  recipient's `preferred_language` (loaded once per event fan-out),
  build a `Localizer` for that tag, and render via
  `loc.T("notif.community_event...", map[string]any{...})` instead
  of `fmt.Sprintf`.
  - Signature changed to `buildNotification(ctx, s, event, loc *l10n.Localizer)`. Subscriber.Handle batch-loads recipient `preferred_language` via `s.GetByIDs(ctx, recipients, &models.User{})` in `loadRecipientLocales(...)`, then caches rendered notifications per `language.Tag` in `notificationsByLocale map[language.Tag]*models.Notification` so each language renders once even when a fan-out has many recipients in the same locale.
  - Localizer failures fall through to nil (which `T()` handles by returning the message id), with a `WarnContext` log per fan-out.
  - Dispatch log lines (`notification dispatched to user` / `failed to send notification to user`) now carry `"locale", tag.String()` for observability.
- [x] Per-event-type table-driven tests asserting EN and ES
  rendering. The existing English-only tests in
  `server/notifications/community_subscriber/*_test.go` continue to
  pass with `locale=en`.
  - `community_subscriber/copy_es_test.go` and `chat_subscriber/copy_es_test.go` (representative coverage, not every event type — full per-event coverage stays in EN tests).
  - `TestLoadRecipientLocales` covers the batch resolver (stored preference, no-preference fallback, missing user).
  - `l10n_helpers_test.go` in each subscriber package provides `enLoc(t)` and `esLoc(t)` test helpers.
- [ ] Integration test that snapshots the rendered FCM payload to
  catch accidental key drift.
  - **Skipped for the Phase 2 PR.** The per-event-type EN/ES tests already assert the rendered title/body; a payload snapshot is largely redundant and adds maintenance overhead. Revisit if real-world key drift turns up; otherwise consider dropping this task.
**Validates:** A user with `preferred_language="es"` receiving a
loan-interest push sees Spanish on the lock screen. This unblocks
the largest user-visible category in a single shipping phase.

### Phase 3: Email — user-facing emails localized — ✅ SHIPPED

**Scope decision (2026-05-24):** Only the two user-facing emails
(password reset and waitlist welcome) were localized. The activity
digest and waitlist notification both send to a single configured
staff recipient (`ActivityDigestConfig.Recipient` wired in
`server/main.go`; `s.notifyEmail` on the waitlist service), so
localizing them would have ~3× the surface for zero user benefit.
The plan's original "activity digest section labels" task was
dropped. Admin-email scope is open to a follow-up if/when there is
a non-staff recipient.

### Key files and APIs landed in Phase 3

- **Catalog additions** (subjects only — body strings live in
  per-locale template files):
  - `email.password_reset.subject`
  - `email.waitlist_welcome.subject`
- **Per-locale templates:**
  - `server/email/password_reset_template_{en,es}.{html,txt}`
  - `server/email/waitlist_welcome_template_{en,es}.{html,txt}`
- **Locale resolver:** `server/email/locale.go::localeForRecipient(ctx, recipientPreferred)`
  — **inverts** the general RPC priority: recipient's stored
  `preferred_language` wins over the request's `Accept-Language`
  (rationale: the email is read by the recipient, not the requester).
- **Render-only helpers** (added for testability, also reusable for
  any future preview tooling):
  `MailgunService.RenderPasswordReset(ctx, resetURL, preferredLanguage)`
  and `RenderWaitlistWelcome(ctx)` — return `(html, text, subject, tag, err)`.
- **`SendPasswordReset` signature change:** added
  `preferredLanguage string` parameter (4th arg). Mock + Service
  interface updated. Callers pass `user.GetPreferredLanguage()`.
- **`MockEmailService.WaitlistWelcomes`** changed from `[]string` to
  `[]WaitlistWelcome` (`{ToEmail, Locale}`) so tests can verify
  locale propagation. Existing call sites only inspected `len(...)`
  so no widespread updates were needed.
- **Goroutine ctx propagation:** `waitlist/service.go::JoinWaitlist`
  captures `l10n.LocaleFromContext(ctx).String()` before spawning
  the welcome-email goroutine, re-injects via
  `l10n.WithLocale(context.Background(), signupLocale)`. Without
  this, every backgrounded email would render in the default
  locale because `context.Background()` strips Accept-Language.

### Decisions captured during Phase 3

- **Body strings live in per-locale template files, not the TOML
  catalog.** Only subjects flow through the localizer. Plan
  explicitly said "one file per locale" and "do not revisit that
  choice" — Phase 3 followed that.
- **Email locale priority inverts the RPC priority.** Documented
  in `docs/server/l10n.md` § "Email locale resolution". This is a
  small but load-bearing distinction.
- **Render extracted from Send for testability.** Each Send method
  now delegates to a sibling `Render*` method that returns the
  rendered bytes. Tests assert on the rendered output without
  invoking the Mailgun SDK. Pattern mirrors the existing
  `RenderActivityDigest`.

**Validates:** A user with `preferred_language="es"` requesting a
password reset receives Spanish subject + body. A waitlist signup
with `Accept-Language: es` receives a Spanish welcome email even
though the send is detached into a goroutine.

### Phase 4a: RPC error infrastructure — ✅ SHIPPED

**Scope:** Build the mechanism for the server to attach a stable, locale-renderable code (+ params) to a Connect error, and the client-side path that surfaces it through the existing `UserError` / `RpcErrorHandler` pair. Migrate one exemplar production site so the wiring is exercised end-to-end. Bulk migration of the remaining user-visible error sites is deferred to Phase 4c (mechanical).

### Key files and APIs landed in Phase 4a

- **Proto:** `proto/ripls/api/errors.proto` — `LocalizedErrorDetail { string code = 1; map<string, string> params = 2; }`. Codes are an append-only contract.
- **Server helper:** `server/connecterr/user_visible.go::UserVisible(ctx, connectCode, code, fallbackMessage, params)`. Mirrors the existing `undoFailure` pattern (attaches detail via `connect.NewErrorDetail` + `cerr.AddDetail`). Keeps the English fallback message visible on the wire so older clients still display readable text.
- **Migrated site:** `server/services/experience/delete.go::DeleteExperience` — non-owner case now uses `connecterr.UserVisible(..., "experience_owner_required_for_delete", ...)`. Existing test that asserts only the Connect code keeps passing (wire-compatible).
- **Client `UserError`:** new `localizedCode({required String code, Map<String,String> params})` variant in `app/lib/core/errors/user_error.dart`.
- **Client extractor:** `_extractLocalizedErrorDetail` in `rpc_error_handler.dart` walks `ConnectException.details` for `ripls.api.LocalizedErrorDetail`; classify surfaces a `localizedCode` UserError when present, otherwise falls through to the existing `rpcCode` branch.
- **Client code→ARB switch:** `_localizeServerCode(code, params, l10n)` in `rpc_error_handler.dart` — one case per known code, default falls back to `rpcErrorGeneric` so an unknown code (server newer than client) still renders something readable.
- **ARB additions:** `rpcErrorExperienceOwnerRequiredForDelete` in `app_en.arb` and `app_es.arb`.

### Decisions captured during Phase 4a

- **Append-only code contract.** Renaming or repurposing a code would silently break older clients still mapping the old code. Add a new code instead.
- **Wire-compatible migration.** The Connect error's public message stays in English so any pre-Phase-4 client still shows something readable. The detail is additive.
- **Unknown codes render `rpcErrorGeneric`, not crash.** Client may be older than the server's code set; the user must see something.
- **No bulk migration yet.** Only `DeleteExperience` is migrated in this PR. The proto + helper + client wiring is the load-bearing part; subsequent sites can be migrated incrementally without changing the mechanism.

**Validates:** A client receiving the new attached detail renders the locale-specific ARB string for `experience_owner_required_for_delete`. Older clients keep working via the English wire fallback.

### Phase 4b: In-app surfaces — structured-template migration — ✅ SHIPPED

All emit sites migrated to structured `template_key` + `params` payloads:
- **Story:** 5 templates × 2 variants migrated; `resolveStoryText` resolver in `app/lib/presentation/widgets/story/story_text_resolver.dart` switches on key.
- **SystemChatMessage:** All 40 server emit sites refactored to use `chat.InsertLocalized` with paired `LocalizedMessage` helpers; `resolveSystemMessageText` resolver in `app/lib/presentation/widgets/chat/system_message_text_resolver.dart`.
- **`connecterr.Internal`:** every `CodeInternal` error carries `LocalizedErrorDetail{code: "internal_error_generic"}` — client maps to `rpcErrorInternal` in the recipient's locale. Wire-compatible: the fixed `"internal server error"` public string stays on the wire for older clients.
- **Round-trip tests:** `server/storage/story_test.go::TestStoryStorage_TemplateKeyAndParamsRoundTrip` + `TestStoryStorage_TemplateKeyOmittedForAIPath` verify the Story proto's structured fields persist correctly through `Insert`/`GetByID`. `server/chat/system_messages_test.go::TestSystemMessageWriter_TemplateKeyAndParamsRoundTrip` + `TestSystemMessageWriter_LegacyInsertOmitsTemplateKey` verify the same for SystemChatMessage (both new structured emit path and legacy fallback path).

Original detail items from the implementation plan (now resolved):

**How to resume (cold start):** This is the riskiest remaining phase. Schema change + historical-data fallback + cross-stack client/server coordination. Recommend splitting into sub-PRs by surface (stories, chat system messages) rather than landing all of Phase 4b at once. The RPC-error mechanism is already in place (Phase 4a).

**Pre-work before writing code:**
1. **File the cleanup issue first.** The TODO(#NNNN) markers in the new `template_key`/`params` schema reference an issue that must exist before this PR lands. Per memory `feedback_backfill_cleanup_issue`, the cleanup issue tracks deleting the legacy free-text field once historical rows age out. File via `gh issue create` with the proto field path, then capture the number for the TODO.
2. **Confirm proto field numbers.** `proto/ripls/models/chat.proto:129` was the line ref at plan-write time; verify with `grep -n "SystemChatMessage" proto/ripls/models/chat.proto`. Add the new fields (`template_key`, `params`) at the next free tag, semantically grouped.
3. **Audit the existing `UserError` / `RpcErrorHandler` pair.** The new RPC-error code mechanism integrates into the canonical pair (`app/lib/core/errors/README.md` is the source of truth). Read that file before sketching the API change so the integration is clean, not a parallel mechanism.

**Goal:** Stories, **system** chat messages (`SystemChatMessage`
only — `UserChatMessage` rows stay verbatim), and user-visible RPC
errors become `{template_key, params}` payloads rendered by the
client from its ARB.
**Tasks:**
- [ ] Schema. Add `string template_key` + `map<string, string> params`
  to the Story proto and to `SystemChatMessage`
  (`proto/ripls/models/chat.proto:129`) — **not** to
  `UserChatMessage`. Emit sites in `server/story/templates.go`,
  `server/chat/system_messages.go`,
  `server/services/transfer/lifecycle.go:111`, and
  `server/services/experience/lifecycle.go:74-75`. Keep the
  existing free-text field on each for one release with
  `TODO(#NNNN)` referencing the cleanup issue (file that issue
  first — [[feedback-backfill-cleanup-issue]]).
- [ ] Server: every story / system-chat-message emit site populates
  the structured fields. The legacy free-text field is still set
  to a pre-rendered English string as a transition fallback for
  old clients and old persisted rows.
- [ ] Add the corresponding keys to `app/lib/l10n/app_en.arb` and
  `app/lib/l10n/app_es.arb`. Naming convention: `serverStory<Type>`
  and `serverChatSystem<Action>` in `scopeContextDescription`
  lowerCamelCase (per `docs/client/i18n.md`). Each new key needs an
  `@key` metadata block declaring placeholders for ICU substitution.
  Per `docs/client/accessibility.md`, if any story or chat system
  message is also announced via `LiveRegion` /
  `SemanticAnnouncer`, that announcement string needs a paired
  `a11y`-prefixed key (often the same content phrased for a screen
  reader). Audit emit sites for those announcement paths during
  Phase 4 client work.
- [ ] Client: where stories and `SystemChatMessage` rows are
  displayed, switch from "use the server-rendered text" to a small
  client-side resolver that reads `template_key` and `params` and
  returns the localized string from `context.l10n.*` (access
  pattern per `docs/client/i18n.md` — the `context.l10n` extension
  in `app/lib/core/extensions/l10n_extensions.dart`, not
  `AppLocalizations.of(context)` directly). Resolution happens in
  the widget's `build()` method — ViewModels stay string-free
  (`docs/client/i18n.md` § "Viewmodels never resolve strings"). Fall
  back to the legacy free-text field if `template_key` is empty
  (handles historical rows). `UserChatMessage` rendering is
  untouched.
- [ ] User-visible RPC errors: introduce a small stable
  string-code mechanism for the ~5–10 user-visible errors
  identified in the inventory (e.g.
  `experience_must_be_owner_to_start`,
  `experience_invalid_state_transition`). Server emits the code
  and any params via `connect.Error` details. Client maps each
  code to an `rpcError*` ARB key
  (`rpcErrorExperienceMustBeOwnerToStart`, etc.) — `rpcError*` is
  the established prefix per `docs/client/i18n.md`. Integrate
  through the canonical `UserError` / `RpcErrorHandler` pair
  (see `app/lib/core/errors/README.md`): the viewmodel `catch`
  block calls `RpcErrorHandler.classify(e)`, which now also
  carries the structured code+params; the widget `build()` calls
  `RpcErrorHandler.localize(state.error!, context.l10n)`, which
  reads the new keys. This replaces the placeholder
  `_extractServerUserMessage()` shape — it is not a new
  parallel mechanism.
- [ ] Localize the public string in `server/connecterr/internal.go`
  via the same code-based mechanism. Today
  `connecterr.Internal(ctx, op, err, kv...)` returns the fixed
  public string `"internal server error"` (per
  `docs/server/architecture.md` § "Error wrapping"). After this
  change the public payload remains the same string, but the
  Connect error detail carries a stable code
  (`internal_error_generic`) that the client's `RpcErrorHandler`
  maps to `rpcErrorInternal`. Existing callers of
  `connecterr.Internal` are unchanged.
- [ ] Tests: persistence round-trip + render-at-read assertions
  in EN and ES; an integration test exercising one user-visible
  error path end-to-end.
**Validates:** Story feed, chat system actions, and the small set
of user-visible errors render in the viewer's locale. The catalog
boundary holds: nothing in the server catalog overlaps with anything
in the client ARB.

### Phase 5: AI-generated content — prompt the model in the recipient's locale — ✅ SHIPPED

**Scope:** Plumb the recipient's `preferred_language` into the two AI prompts that produce per-user prose (nudges + impact-row narratives). Add a top-of-prompt directive so the directive dominates the model's planning before it sees the JSON-shape instructions. Story content (`StoryContentRequest`) is intentionally **not** in scope — stories are community-scoped, not per-user, and the right answer is Phase 4b's structured-template migration, not picking one viewer's locale to render in.

### Key files and APIs landed in Phase 5

- **Helper:** `l10n.DisplayName(tag language.Tag) string` returns the English-language name of a supported tag ("English", "Spanish") for deterministic prompt construction. Used by AI prompts where the directive needs a stable, unambiguous name.
- **Nudge prompt** (`server/ai/prompts.go::buildNudgeContentPrompt`): top-of-prompt `LANGUAGE: Respond entirely in {Name}. Headline, description, and cta_label must all be in {Name}. Do not mix languages.` injected when the locale != default. Empty/unsupported locales fold to English (no directive, same prompt bytes as pre-Phase-5).
- **Nudge caller** (`server/services/feed/nudges.go::generateAndPersistNudges`): now passes `user.GetPreferredLanguage()` instead of hardcoded `"en"`.
- **Impact-narrative prompt** (`server/ai/prompts_impact_row.go::buildImpactRowNarrativePrompt`): same top-of-prompt directive shape. `ImpactRowSignal.Locale` field added.
- **Impact-narrative caller** (`server/services/profile/narratives.go::EnsureNarratives`): loads the target user to pass `preferred_language` into the signal. User-load failure logs Warn and continues with default locale — narrative rendering is best-effort with a static-string client fallback.
- **`ImpactRowPromptVersion` bumped to `impact_row_v2`** so eval results correlate to the new prompt revision.

### Decisions captured during Phase 5

- **Stories deferred to Phase 4b.** A story is generated once but read by many users in possibly different locales. There's no good "single locale" answer here — the right design is the structured `{template_key, params}` migration. Localizing the story PROMPT alone would push us toward picking one viewer's locale arbitrarily.
- **English display name in prompts.** Using "Spanish" (not "español") in the directive. Modern LLMs understand both; the English form is deterministic across providers and easy to verify in tests.
- **Empty/unsupported locale produces no directive.** Same prompt bytes as before Phase 5 — zero regression risk for the en path, and unsupported tags don't accidentally hide a locale bug behind a misleading directive.
- **No integration test shipped.** Per-provider integration tests would invoke real AI APIs; the meaningful contract we control is the prompt shape, which the unit tests cover. AI Spanish quality is a separate concern that needs human review, not an automated assertion that "the output looks Spanish-ish."

**Validates:** A Spanish user (`preferred_language="es"`) sees Spanish nudges and Spanish impact-row narratives generated for them. Catalog drift is structurally impossible because no catalog covers AI content on either side.

### Phase 6: Guardrails — prevent regression of inline English strings — ✅ SHIPPED

**Scope:** Wire CI-level guards that catch the two ways a new contributor would accidentally break the catalog boundary: (1) adding a key in one locale TOML and forgetting the other, (2) writing inline English in a file that's supposed to render via the localizer. The runtime warn-once gate already shipped in Phase 0.

### Key files landed in Phase 6

- **`scripts/check_l10n_toml_parity.js`** — server-side TOML parity. Treats `server/l10n/source/en.toml` as source of truth and fails CI if any locale TOML is missing keys or has extras. Mirrors `scripts/check_l10n_parity.js` style (the ARB equivalent).
- **`scripts/check_no_inline_user_strings.js`** — Go-side guard against inline `fmt.Sprintf("Capitalized English ...")` regressions in fully-migrated user-facing surfaces (`community_subscriber/copy.go`, `chat_subscriber/copy.go`). Narrowly scoped on purpose — admin emails and not-yet-migrated surfaces are out of scope to avoid false positives. Contributors adding a new fully-migrated surface should extend `SCANNED_FILES`.
- **`package.json`** — `lint:go:l10n-parity` and `lint:go:no-inline-user-strings` added, both chained into the existing `lint:go` script (which `npm run build` runs).
- **`docs/server/l10n.md`** — contributor workflow section names both scripts and explains how to extend the inline-strings guard.

### Decisions captured during Phase 6

- **Inline-strings scope is allowlisted, not blocklisted.** Scanning every `.go` file for `fmt.Sprintf` with capitalized English would generate hundreds of false positives (log messages, error messages bound for `connecterr`, internal-only strings). The maintainable shape is "files we have *committed* to running through the localizer get scanned" — which is exactly the two `copy.go` files today. Future migrations extend the list.
- **Client ARB parity script reused as-is.** `scripts/check_l10n_parity.js` already covers every key in `app_en.arb` including the `rpcError*` family added in Phase 4a. No new client-side script needed.
- **Runtime warn-once already shipped.** Phase 0 implemented `Localizer.T()`'s `missLog sync.Map` per the design (`server/l10n/localizer.go`). No-op for Phase 6.

**Validates:** Adding a new notification key in one TOML and forgetting the other fails CI. Switching back to `fmt.Sprintf("Owner approved the request")` inside a `copy.go` file fails CI.

## Related Issues

- None directly linked in #1904 comments. The work touches the
  surfaces of several existing improvement areas (push notification
  polish, email digest formatting, story generation, nudge quality)
  but no cross-referenced issues were found during research. File
  follow-ups for: (a) the story/chat free-text field cleanup
  (Phase 4 dependency), (b) AI-content quality validation in
  non-English locales, (c) per-device locale if/when multi-device
  mixed-locale users come up in support.
