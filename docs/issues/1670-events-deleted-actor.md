# Issue #1670: ListCommunityEvents 500s when an event's actor user has been deleted

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1670
**Filed:** 2026-05-02 | **Last activity:** 2026-05-02
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`ListCommunityEvents` returns `connect.CodeInternal` for the entire community
whenever any `CommunityEvent` in the result set references an `actor_id` that
`FetchAPIUsersBatch` no longer returns (i.e. the actor was soft-deleted). The
same hazard exists for `object_user_id`. The events feed for any member of
the affected community stays broken until the offending event drops out of
the 7-day window enforced at `server/services/community/events.go:71`.

Production evidence (ripls-prod, revision `server-00086-h7m`):

- 4 errors on 2026-05-02 between 00:12 UTC and 10:58 UTC, fired on every
  refresh.
- Single missing actor `00000000-0000-4000-8000-000000000006`, single
  community `00000000-0000-4000-8000-000000000003`, single calling user
  `d2aae17b-…` (a staff account).
- The same actor id appears earlier in `sending notification to user` logs
  through 2026-04-30 and then in `user not found for RSVP` on 2026-05-02 —
  an account-deletion fingerprint, not a never-existed-id corruption.

The bug will recur on every account deletion: `GetByIDs` in
`server/storage/protosql_batch.go:24` documents `"By default, soft-deleted
items are excluded"`, and that exclusion is what trips the missing-actor
path.

The PR should fix not only `events.go` but every other RPC that joins users
into responses via `services.FetchAPIUsersBatch` and silently propagates
nils into proto fields the client treats as non-optional.

## Context from Comments

No comments on the issue (filed during this session). Bug was found
end-to-end: prod alert → log dive → root-cause read of the source. There is
no debate to summarize.

The user's direction at planning time: *use Option B (placeholder), make the
displayed string a localized "Former Member", and proactively fix similar
hazards across all `FetchAPIUsersBatch` callers in this PR*.

## Potential Solutions

### Option A: Skip events with unresolvable actor (lenient drop)

- **Approach:** Skip events whose actor or object_user can't be resolved.
- **Pros:** Minimal surface change; matches the existing lenient gear branch
  at `events.go:167-169`.
- **Cons:** History gets quietly thinner. Doesn't help the other RPCs that
  hit the same `userMap[id]` pattern with non-skip semantics (transfers,
  gear/people, chat reactions, chat last-message-sender) — those need a
  placeholder anyway.
- **Effort:** Small.

### Option B: "Former Member" placeholder with proto-level signal — RECOMMENDED

- **Approach:** Add `bool former_member = 7;` to `api.User`. Server emits a
  placeholder `api.User{Id: <id>, Name: "", FormerMember: true}` whenever a
  referenced user can't be resolved. Client checks `formerMember` and
  renders a localized string from `app_*.arb`. Same pattern applies to
  every `FetchAPIUsersBatch` consumer that propagates a user into a
  response.
- **Pros:** Preserves event/transfer/reaction history without leaking
  deleted-user PII (Name/MediaId stay empty server-side). Localization
  lives on the client where it belongs (server doesn't know the caller's
  locale). Same helper plugs into every site that has the same hazard.
- **Cons:** Proto change to a widely-used type — has to be wire-compatible
  (a new optional bool is, by construction). Client has to be updated
  everywhere `actor.name` etc. is rendered raw to use the new helper.
- **Effort:** Medium.

### Option C: Include soft-deleted users via `QueryOptions{IncludeDeleted: true}`

- **Approach:** Pass `IncludeDeleted: true` in `FetchAPIUsersBatch` so
  soft-deleted user rows still surface.
- **Pros:** One-line storage change; no proto, no client work.
- **Cons:** Surfaces deleted-user PII (name, media id) in feeds *forever*,
  defeating the point of soft-delete. Fails open if we ever hard-delete.
  Doesn't address the symmetric nil-propagation bugs in transfer / gear /
  chat.
- **Effort:** Small, but the privacy regression makes it a non-starter.

## Recommendation

**Option B**, applied consistently across every `FetchAPIUsersBatch` caller
that currently propagates a missing user to the API layer. Per the user's
direction, all of this lands in a single PR.

In-scope sites (audited):

| Site | Current behavior on missing user | Action |
|---|---|---|
| `server/services/community/events.go:148-152` (actor) | **500 Internal** | Replace with placeholder; demote log to `Warn` |
| `server/services/community/events.go:157-163` (object_user) | **500 Internal** | Replace with placeholder; demote log to `Warn` |
| `server/services/transfer/service.go:254` (Owner) | Sets `api.Transfer.Owner = nil` (proto field is non-optional User) | Use placeholder |
| `server/services/transfer/service.go:259` (Recipient) | Sets `api.Transfer.Recipient = nil` | Use placeholder (or leave nil if proto says optional — verify) |
| `server/services/gear/people.go:115-134` (currentBorrower / pastBorrowers) | `nil` borrower can be assigned to `CurrentBorrower` and (worse) appended to `PastBorrowers`, which the client iterates | Use placeholder for both |
| `server/services/chat/reactions.go:188+` (Reaction.Sender) | Sets `Sender = nil`; client iterates reactions and reads `sender.name` | Use placeholder |
| `server/services/chat/conversations.go:970` (LastMessageSender) | Sets `LastMessageSender = nil`; field may be optional client-side, verify before changing | Use placeholder if rendered raw; otherwise leave |

Out of scope (deliberate skips, *not* nil-propagation hazards):

- `server/planning/list.go:79-92` — already silently skips needs/contributions
  with deleted authors. Removing a need entirely is the intentional product
  behavior; placeholder would render an unactionable row.
- `server/services/experience/build_api.go:129-134` — already silently skips
  experiences whose owner is missing. An owner-less experience has no
  meaningful product surface.
- `server/services/experience/service.go:270` — already silently skips
  RSVPs with deleted users. RSVP semantics requires a real user.
- `server/services/chat/conversations.go:515` — already filters nil from
  `Participants`. Conversation participant list semantics is "currently
  active participants", so dropping deleted users is correct.

To support Option B we add:

1. `api.User.former_member` (bool field, new tag) — wire-compatible.
2. `services.FormerMemberPlaceholder(userID string) *api.User` — single
   constructor for the placeholder so every site emits an identical shape.
3. `services.ResolveUserOrFormer(userMap map[string]*api.User, id string)
   *api.User` — convenience for the common "lookup or placeholder" pattern.
   Returns `nil` for empty `id` so optional fields keep their nil
   semantics; returns the placeholder for non-empty ids that miss the map.
4. `app/lib/core/utils/user_display.dart` (new) — `userDisplayName(User?,
   AppLocalizations)` extension that returns the localized "Former Member"
   string when `user.formerMember == true`. All sites currently reading
   `user.name` for display switch to this helper.
5. ARB string `commonFormerMember` ("Former Member") in
   `app/lib/l10n/app_en.arb` and translations in `app_es.arb`.

While we're in `events.go`, also fix the editor-mangled `connecterr.Internal`
calls at lines 113-118 and 122-127 — the `// Batch-fetch all users …` and
`// Build response items …` comments got swept into the `Internal(ctx, op,
err, "detail", …)` argument lists and now sit between `"detail"` and the
detail string. Compiles, but structurally broken.

## Audit Findings (2026-05-02)

Audited against `docs/client/{architecture,caching,i18n,accessibility,testing}.md`
and `docs/server/{architecture,observability}.md`. Updates folded into the
phases below; this section summarizes what changed and why.

1. **i18n: codegen command.** Project uses `npm run generate` (which wraps
   `flutter gen-l10n` per `docs/client/i18n.md` §Architecture). Phase 3
   updated to call `npm run generate`, not `flutter gen-l10n` directly.
2. **i18n: key prefix.** `commonFormerMember` is correct per
   `docs/client/i18n.md` "Key Naming Convention" (`common*` is the right
   scope for cross-screen reuse). No change.
3. **i18n: viewmodels never resolve strings.** Confirmed the placeholder
   never enters a viewmodel — the "Former Member" string is resolved in
   `build()` via `context.l10n` inside the display helper. No viewmodel
   work is required by this fix; called out explicitly in Phase 3.
4. **a11y: avatar color-only-status.** The current
   `community_event_card.dart` avatar at lines 86-102 is a colored circle
   with initials. With an empty name + neutral grey for former members,
   the avatar conveys "former" purely via color/shape. Per
   `docs/client/accessibility.md` "avoid_color_only_status" rule (catches
   `*Dot/*Indicator/*Badge` without `Semantics`), the avatar isn't named
   one of those classes and so won't currently trip the lint, *but* the
   intent of the rule applies. Phase 3 now requires
   `Semantics(label: l10n.a11yFormerMemberAvatar)` (or an equivalent
   `Tooltip`) wrapping the placeholder avatar so screen readers announce
   it. Add `a11yFormerMemberAvatar` to ARB with the `a11y` prefix per
   `docs/client/accessibility.md`.
5. **a11y: l10n on semantic labels.** Any new `Semantics` /
   `tooltip` / `semanticLabel` value must come from `context.l10n` per
   the `require_l10n_for_semantic_labels` rule. Plan updated accordingly.
6. **Server observability: log fields.** Per
   `docs/server/observability.md` "Standard Field Names", the existing
   fields `user_id`, `user_email`, `community_id` are correct, and an
   `operation` field is conventional. Phase 2 now spells out the full
   field set for the demoted `Warn` log: `operation`,
   `actor_id`/`object_user_id`, `community_id`, `event_id`,
   `event_type`. (`request_id` is auto-injected by `LoggerWithContext`,
   so we do not pass it explicitly.)
7. **Server observability: log level.** "User errors (invalid input, not
   found) are INFO or WARN, not ERROR. ERROR is reserved for system
   failures." A deleted user is exactly that — a non-failure, expected
   data state — so `Warn` is correct. Confirmed.
8. **Server alert noise.** The "Server Error Logged" alert (per
   `docs/server/observability.md` Part 4 "Alert Policies") fires on any
   ERROR-level log not excluded by middleware. Demoting to `Warn`
   eliminates the false page directly. Mentioned in the rollout
   verification step.
9. **Server architecture: helper placement.** `server/services/user_util.go`
   is the right home for `FormerMemberPlaceholder` and
   `ResolveUserOrFormer` — it already houses `FetchAPIUsersBatch` and
   the user-conversion helpers. No new package needed. Confirmed.
10. **Caching impact (client).** Adding `bool former_member` to
    `api.User` is wire-compatible (proto3 unknown-field tolerance plus
    zero-value default). Existing cached responses in
    `StashCacheManager` (per `docs/client/caching.md`) will deserialize
    with `formerMember == false`, which is exactly the correct
    pre-existing behavior for users who *aren't* former members.
    No cache migration needed. Called out in Phase 1.
11. **Client testing layer.** Per `docs/client/testing.md` "Test
    business logic, not Flutter widgets," the helper functions
    (`userDisplayName`, `userInitials`, `userAvatarColor`) get unit
    tests in `app/test/core/utils/user_display_test.dart`. ViewModel
    tests are not affected (no viewmodel changes). Phase 3 now lists
    these test files explicitly.
12. **Proto comment style.** Per project CLAUDE.md "Proto Comments,"
    the field doc must avoid server-side mechanics. Reworded the
    proto comment to client-facing semantics: "True when the
    referenced user is no longer available (e.g. account deletion).
    Clients should render a localized placeholder rather than the
    `name`/`media_id` fields, which may be empty for these users."
13. **Lint pre-flight.** Per CLAUDE.md, run `npm run lint:dart:i18n`
    and `npm run lint:dart:a11y` before opening the PR — not just
    `npm run build`. Phase 4 updated.

## Open Questions

- **Does `api.Transfer.Recipient` (and similar fields elsewhere) tolerate
  nil today?** — Recommendation: read the proto for each field on the
  in-scope list before wiring the placeholder. If a field is `optional`,
  preserve nil for the empty-id case (`ResolveUserOrFormer` already does
  this). If non-optional, always emit the placeholder. The audit table
  above is a starting point — verify field shapes during Phase 1.
- **Should the placeholder leak the deleted user's id?** — Recommendation:
  yes, keep `Id` populated. Clients use it as a stable identity for
  list-keying and de-duplication, and the id alone isn't PII (it's a UUID,
  not an email). `Name` and `MediaId` stay empty.
- **Localization: emit "Former Member" server-side or client-side?** —
  Recommendation: client-side via ARB. The server doesn't know the
  caller's locale, and proto comments forbid leaking server-rendered
  display strings into the API contract anyway. Proto carries
  `former_member: bool`; client maps it.
- **Should `getInitials("")` and `_getAvatarColor("")` in
  `community_event_card.dart` get a special case?** — Recommendation: yes
  — generate "?" initials over a neutral grey when the user is a former
  member. The avatar color hash currently keys on `name`; with empty name
  every former-member avatar would otherwise be identical.
- **Backfill of historical `CommunityEvent` rows whose actor is gone?** —
  Recommendation: no, not in this PR. The 7-day cap at `events.go:71`
  prunes the read window. If a future policy requires it, do it as a
  separate `server/jobs/`-style backfill.

## Implementation Plan

### Phase 1: Proto change + server helpers

**Goal:** Land the placeholder mechanism without changing any caller yet.

**Tasks:**
- [ ] Add `bool former_member = 7;` to `proto/ripls/api/user.proto`. Use
  client-facing wording per CLAUDE.md proto-comment rules: *"True when the
  referenced user is no longer available (e.g. account deletion). Clients
  should render a localized placeholder rather than the `name`/`media_id`
  fields, which may be empty for these users."*
- [ ] `npm run generate` (drives both Go + Dart codegen and `flutter
  gen-l10n` per `docs/client/i18n.md`).
- [ ] Note for reviewers: existing `StashCacheManager` entries deserialize
  with `formerMember == false`, which matches pre-existing behavior for
  non-former users. No cache migration required.
- [ ] Add `services.FormerMemberPlaceholder(userID string) *api.User` and
  `services.ResolveUserOrFormer(userMap, id)` in
  `server/services/user_util.go`. Cover both with unit tests in
  `server/services/user_util_test.go` (empty id → nil; missing in map →
  placeholder; present in map → resolved value).
- [ ] `npm run build` to verify lint and compile.

**Validates:** Helpers exist and compile; downstream sites can adopt them
incrementally.

### Phase 2: Server call-site fixes

**Goal:** Eliminate every nil-or-500 hazard in the audited in-scope list.

**Tasks:**
- [ ] `server/services/community/events.go`:
  - Replace 147-152 actor branch with `ResolveUserOrFormer`; demote the
    `ErrorContext` to `WarnContext`. Use the standard observability
    field set per `docs/server/observability.md`: `operation`,
    `actor_id`, `community_id`, `event_id`, `event_type`. Don't pass
    `request_id` — `LoggerWithContext` injects it automatically.
  - Same for 157-163 (object_user_id) — log key is `object_user_id`.
  - Repair the mangled `connecterr.Internal` calls at 113-118 and
    122-127 (move stray comments out of the argument lists).
- [ ] `server/services/transfer/service.go`: confirm `api.Transfer.Owner`
  / `Recipient` shape; for non-optional, swap `userMap[id]` → helper at
  254 and 259.
- [ ] `server/services/gear/people.go`: swap `recipientUser :=
  userMap[transfer.RecipientId]` at 115 → helper. Verify `pastBorrowers`
  iteration tolerates the placeholder.
- [ ] `server/services/chat/reactions.go`: swap `Sender: userMap[r.UserId]`
  at 188+ → helper.
- [ ] `server/services/chat/conversations.go:970`: swap if
  `LastMessageSender` is rendered raw on the client. Audit
  `app/lib/.../conversation*.dart` first to decide.
- [ ] Add a regression test in `server/services/community/events_test.go`
  covering the original bug: insert a `CommunityEvent` with a soft-deleted
  actor, call `ListCommunityEvents` as a member, assert the response is
  200 and the offending event surfaces with `actor.formerMember == true`,
  `actor.id == <deleted-id>`, `actor.name == ""`.
- [ ] Add a sibling test for `object_user_id` deletion.
- [ ] Add a regression test in `server/services/transfer/service_test.go`
  covering a transfer whose recipient was deleted (response is 200,
  recipient surfaces as former-member placeholder).
- [ ] Add a regression test in `server/services/gear/people_test.go` for
  past-borrower deletion.
- [ ] `npm run build` and `go test ./server/...`.

**Validates:** Every listed RPC tolerates a deleted referenced user
without 500ing or emitting nil into a non-optional field.

### Phase 3: Client placeholder rendering

**Goal:** Show a localized "Former Member" wherever a placeholder user
flows through the UI; preserve avatar/initials behavior gracefully and
keep screen readers informed.

No viewmodels are touched in this phase — per `docs/client/i18n.md`
"Viewmodels never resolve strings," all localization happens in
widgets/utils that have `BuildContext` access.

**Tasks:**
- [ ] Add to `app/lib/l10n/app_en.arb` (and parallel entries in
  `app_es.arb` — confirm Spanish phrasing with existing translation
  conventions, e.g. "Antiguo miembro"):
  - `commonFormerMember`: "Former Member" (with `@commonFormerMember`
    description: "Display name for users who are no longer available,
    e.g. due to account deletion.")
  - `a11yFormerMemberAvatar`: "Former member" (with `@a11yFormerMemberAvatar`
    description: "Screen reader label for the placeholder avatar shown
    in place of a deleted user's profile image.")
- [ ] `npm run generate` to regenerate `app_localizations*.dart` (per
  `docs/client/i18n.md`, this is part of the standard generate step).
- [ ] Add `app/lib/core/utils/user_display.dart` with three pure
  functions (per `docs/client/architecture.md` directory rules — utils
  are not controllers, no state):
  - `String userDisplayName(User user, AppLocalizations l10n)` —
    returns `l10n.commonFormerMember` when `user.formerMember`, else
    `user.name`.
  - `String userInitials(User user, AppLocalizations l10n)` — returns
    `"?"` (or other neutral marker) for former members; otherwise
    delegates to existing initials logic from
    `community_event_card.dart`'s `_getInitials` (factor that helper
    out of the widget into this util).
  - `Color userAvatarColor(User user, BuildContext context)` — returns
    a neutral `AppColors.surfaceVariant`-style grey for former members;
    otherwise hashes `user.name` (port the existing
    `_getAvatarColor` from `community_event_card.dart`).
- [ ] Switch `app/lib/core/utils/community_event_helper.dart`
  (every `event.actor.name` and `event.objectUser.name` reference at
  lines 11-35) to `userDisplayName(event.actor, context.l10n)`.
- [ ] Switch
  `app/lib/presentation/widgets/portfolio/community_event_card.dart:91-95`:
  - Use `userInitials` + `userAvatarColor`.
  - Wrap the avatar `Container` in
    `Semantics(label: context.l10n.a11yFormerMemberAvatar, ...)` *only
    when* `event.actor.formerMember == true`. For non-former users keep
    the existing structure (the actor's name is announced via the
    surrounding text node, so a redundant `Semantics` would just
    duplicate it).
- [ ] Audit other call sites with
  `grep -rn 'actor\.name\|recipient\.name\|borrower\.name\|sender\.name\|owner\.name' app/lib/`
  (excluding `*.freezed.dart`, `*.pb.dart`, `*.g.dart`) and update each
  to `userDisplayName`.
- [ ] Add `app/test/core/utils/user_display_test.dart` per
  `docs/client/testing.md` "Test business logic" guidance: pure-function
  unit tests covering normal user, former-member user, empty name,
  empty id. No `ProviderContainer`, no widget tree.
- [ ] Update existing widget tests for `community_event_card` (if any)
  to cover the former-member path. Use the localization helper from
  `app/test/helpers/l10n_helpers.dart` per `docs/client/testing.md` so
  `context.l10n` is available.
- [ ] Run `flutter test --reporter expanded` (and `npm run lint:dart:i18n`
  + `npm run lint:dart:a11y` per CLAUDE.md) and confirm green.

**Validates:** A response with a former-member user displays "Former
Member" wherever the user flows through, and avatar logic doesn't crash on
empty name.

### Phase 4: Ship + verify

**Goal:** Land the fix and confirm prod recovery.

**Tasks:**
- [ ] Pre-PR checks (per CLAUDE.md):
  - `npm run build` (golangci-lint + `go build ./...`).
  - `go build -tags=integration ./...` if any exported signature in
    `services/*` was touched (helpers in `user_util.go` count).
  - `npm run lint:dart:i18n` and `npm run lint:dart:a11y`.
  - `flutter test --reporter expanded`.
  - `go test ./server/...`.
- [ ] Open a PR titled `Fix ListCommunityEvents 500 on deleted actor;
  add Former Member placeholder` referencing #1670.
- [ ] Use the gcloud query from the diagnosis to confirm post-deploy
  that the prod `failed to get actor` ERROR log no longer fires (the
  "Server Error Logged" alert per `docs/server/observability.md` Part 4
  should stop triggering immediately). The replacement `Warn` log
  showing `actor_id` may still surface in dashboards — that's expected.
- [ ] Spot-check the affected member's events feed in prod data (or a
  prod-like environment) to confirm a former-member event renders
  cleanly.

**Validates:** Production no longer pages on this; the affected member's
feed works again.

## Related Issues

- #1641 — transfer history viewable after gear deletion. Same shape
  problem, already solved on the gear side via
  `QueryOptions{IncludeDeleted: true}` in `transfer/service.go:193`. We
  deliberately do *not* take that approach for users (PII), but the
  precedent is informative.
