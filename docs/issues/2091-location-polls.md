# Issue #2091: Location polls: let the group vote on where to meet (parallel to time polls)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2091
**Filed:** 2026-05-21 | **Last activity:** 2026-05-21
**Labels:** none | **Assignees:** unassigned
**Context docs:** none (design HTML referenced in the issue: [docs/cowork/App Design/location-redesign.html](../cowork/App%20Design/location-redesign.html), [docs/cowork/App Design/location-poll-redesign.html](../cowork/App%20Design/location-poll-redesign.html))

**Audit pass:** 2026-05-21 — Reviewed against [`docs/client/architecture.md`](../client/architecture.md), [`docs/client/caching.md`](../client/caching.md), [`docs/client/i18n.md`](../client/i18n.md), [`docs/client/accessibility.md`](../client/accessibility.md), [`docs/client/testing.md`](../client/testing.md), [`docs/client/time.md`](../client/time.md), [`docs/server/architecture.md`](../server/architecture.md), and [`docs/server/observability.md`](../server/observability.md). Changes folded into the body below; the **Audit Notes** section near the end lists what shifted and why.

## Problem Statement

Today an organizer must pick a single location when scheduling an experience. If the right spot is genuinely uncertain — a hike trailhead, whose backyard for a potluck, which park for a picnic — coordination spills into chat and rarely converges cleanly.

Time polls already solve the parallel problem for *when* (`ProposeTime` / `VoteOnTime` / `ConfirmTime` / `UnlockTime` / `CancelTimePoll` with `time_poll_active` / `time_poll_completed` / `current_poll_id` on the experience). The design at `docs/cowork/App Design/location-redesign.html` proposes the same shape for *where*: organizer can add one spot (sets it directly) or several spots (opens a poll), voters tap any spot that works ("you can pick more than one"), organizer locks in a winner.

The work is a near-1:1 transcription of the time-poll architecture into a location-poll architecture, with one twist: a location proposal can point to either a saved `Location` (id reference) or an unsaved `GeocodedLocation` (drop-pin / search result), and the server has to materialize the right thing on confirm.

## Context from Comments

No comments — the issue was just filed. This planning doc is the first artifact downstream of it. Flag for the reviewer: nobody has weighed in on the proposed RPC names or the saved-vs-unsaved proposal representation yet. The Open Questions section below names the calls that need confirmation.

## Potential Solutions

### Option A: Mirror the time-poll architecture 1:1
- **Approach:** Replicate every layer. New `proto/ripls/api/location_poll.proto` types (`LocationProposal`, `LocationVote`, `LocationVoteStatus`), parallel RPCs on `ExperienceService` (`ProposeLocation` / `VoteOnLocation` / `ConfirmLocation` / `UnlockLocation` / `CancelLocationPoll`), parallel storage table `ExperienceLocationProposal`, parallel server file `server/services/experience/location_proposals.go`, parallel client modals (`location_poll_propose_modal.dart`, `location_poll_vote_modal.dart`, `location_poll_confirm_modal.dart`, `unified_location_modal.dart`) and view models. Reuse the existing `LocationPickerModal` for spot entry. Embed in the same UI surfaces (`experience_event_pane.dart`, `poll_banner.dart`, `poll_results_view.dart`).
- **Pros:** Predictable, low cognitive cost for a reviewer, follows precedent set by time polls, easy to test against existing time-poll tests.
- **Cons:** Code duplication. If we add a third poll type later (e.g. who-brings-what), we'll have three copies of vote-tallying / poll-lifecycle code.
- **Effort:** Large — but mostly mechanical.

### Option B: Generalize polling now — abstract a `Poll<T>` concept
- **Approach:** Refactor time polls into a generic `Poll` storage entity with a typed payload (time *or* location *or* future kinds). One set of RPCs (`ProposeOption`, `Vote`, `Confirm`, `Cancel`) parameterized by poll kind. Single set of client modals routed by poll kind.
- **Pros:** Avoids future duplication if more poll types arrive.
- **Cons:** Speculative — we don't know whether other poll types are coming, and "generic" data models in proto are awkward (oneof everywhere, no shared validation). High refactor risk on a working feature time polls. Doesn't ship the user value any faster. Counter to CLAUDE.md's "Don't design for hypothetical future requirements."
- **Effort:** Very large.

### Option C: Mirror at the data/RPC layer; share at the UI banner layer
- **Approach:** Same as Option A for proto / storage / server. On the client, extract the parts that already duplicate between "experience time poll" and the new "experience location poll" into shared widgets — `poll_banner.dart` and `poll_results_view.dart` already lean generic; promote them to accept a poll-kind-agnostic state object. Each poll kind still has its own propose / vote / confirm modals because their inner UIs differ (date pickers vs map / saved spots).
- **Pros:** Captures the only duplication that genuinely hurts (the banner + result view appear on every screen) without paying the cost of a full refactor. Matches CLAUDE.md's "Three similar lines is better than a premature abstraction" — we'd only abstract where we actually have two concrete instances.
- **Cons:** Slightly more design work on the banner abstraction. Risk of getting the abstraction wrong and having to undo it.
- **Effort:** Large, ~10% more than Option A.

## Recommendation

**Option C.** Option A is the safe default and Option B is over-engineering. Option C captures the small win of factoring out `poll_banner` and `poll_results_view` (which already look poll-kind-agnostic on the design side) without touching the proto/server layer. If the banner abstraction turns out to be the wrong shape during Phase 3, we can fall back to Option A by inlining the two banners — the cost is bounded.

A note on the proposal shape: a `LocationProposal` should carry `oneof { string location_id; GeocodedLocation geocoded; }` so the organizer can propose either a saved spot or a freshly-dropped pin. On `ConfirmLocation`, the server resolves any unsaved `GeocodedLocation` into a stored `Location` (using the existing save path) and updates the experience's `location_id` / `latitude_deg` / `longitude_deg`. This mirrors how the create-experience flow already handles the "did the user pick a saved spot or a new one?" choice and avoids forcing a save before the poll closes (proposals can be ephemeral if the poll is cancelled).

## Open Questions

- **Does a confirmed location update `Experience.location_id` only, or also `latitude_deg`/`longitude_deg`?** — Recommendation: write all three. The current single-location flow populates them in parallel, and the embedded lat/lng makes feed cards and map markers cheaper to render without re-joining `Location`. Matches the existing storage pattern.
- **Should proposals reference saved `Location` IDs or always carry inline `GeocodedLocation`?** — Recommendation: support both via `oneof`. Saved-spot picks reference the `location_id` (efficient, dedupes); drop-pin / search picks carry `GeocodedLocation` and are saved on confirm. This avoids creating thousands of one-off `Location` rows for proposals that lose the vote.
- **Do we need `UnlockLocation` parity with `UnlockTime`?** — Recommendation: yes. Symmetry is cheap and the same "organizer changed their mind after confirm" case applies. The server logic is identical: clear `is_confirmed` on the winning proposal, leave proposals in place, re-set `location_poll_active` if there are >1 proposals.
- **Reuse `CommunityEventType.EXPERIENCE_UPDATED` or add `LOCATION_POLL_*` event types?** — Recommendation: reuse `EXPERIENCE_UPDATED`, mirroring the time-poll precedent. Adding new `CommunityEventType` values triggers the undo-registry classification work in `server/undo/registry.go` and there's no behavioral reason to distinguish location-poll updates from any other experience update in the activity feed.
- **Add `CHAT_SYSTEM_ACTION_LOCATION_PROPOSED` and `CHAT_SYSTEM_ACTION_LOCATION_POLL_CANCELLED`?** — Recommendation: yes to both, mirroring `CHAT_SYSTEM_ACTION_TIME_PROPOSED` (value 14) and `CHAT_SYSTEM_ACTION_TIME_POLL_CANCELLED` (value 17). Without these, the chat transcript misses the same lifecycle beats it already shows for time polls.
- **Can the organizer have a time poll and a location poll active simultaneously?** — Recommendation: yes, treat them as fully independent state machines. The design HTML does not suggest mutual exclusion, and the cleanest data model lets both `time_poll_active` and `location_poll_active` be `true` at once on the same experience. UX implication: the Plan tab shows both banners stacked; voters tap each one independently.
- **What happens to vote state when the organizer adds a new spot after voting has started?** — Recommendation: keep all existing votes; the new proposal starts with zero votes, and users who already voted are not forced to re-vote on the new option (they can opt in by tapping it). Mirrors time-poll behavior, where adding a new time proposal mid-poll doesn't reset existing votes.
- **Sharing the `poll_banner.dart` widget across time and location polls (Option C choice) — is the existing widget actually generic enough?** — Recommendation: read it first in Phase 1 before committing to the abstraction. If the time-specific copy is deeply baked in, fall back to a second `location_poll_banner.dart` for v1 and revisit consolidation later.

## Implementation Plan

### Phase 1: Proto + storage + server RPCs (no client changes)
**Goal:** Wire-format and server logic land with full test coverage. Nothing user-visible yet.
**Tasks:**
- [ ] Define `proto/ripls/api/location_poll.proto` with `LocationProposal`, `LocationVote`, `LocationVoteStatus` (mirror `proto/ripls/api/time.proto`). Use the `oneof { string location_id; GeocodedLocation geocoded; }` shape for the proposed location.
- [ ] Define `proto/ripls/models/location_poll.proto` storage analogs (`ExperienceLocationProposal`, `LocationVote`) — mirror `proto/ripls/models/time.proto`.
- [ ] Add API `Experience` fields: `repeated LocationProposal location_proposals`, `bool location_poll_active`, `optional bool location_poll_completed`, `optional string current_location_poll_id` ([proto/ripls/api/experience.proto:122-137](../../proto/ripls/api/experience.proto#L122-L137)).
- [ ] Add storage `Experience` fields to match ([proto/ripls/models/experience.proto:68-78](../../proto/ripls/models/experience.proto#L68-L78)).
- [ ] Add five RPCs on `ExperienceService` in [proto/ripls/api/experience_service.proto](../../proto/ripls/api/experience_service.proto): `ProposeLocation`, `VoteOnLocation`, `ConfirmLocation`, `UnlockLocation`, `CancelLocationPoll`. Each gets its own request/response message per CLAUDE.md's "never reuse RPC request types" rule.
- [ ] Add `CHAT_SYSTEM_ACTION_LOCATION_PROPOSED` and `CHAT_SYSTEM_ACTION_LOCATION_POLL_CANCELLED` enum values in [proto/ripls/api/chat_service.proto](../../proto/ripls/api/chat_service.proto) and [proto/ripls/models/chat.proto](../../proto/ripls/models/chat.proto). Pick the next free numbers.
- [ ] Implement RPC handlers in `server/services/experience/location_proposals.go` (mirror `server/services/experience/time_proposals.go`). Auth: participants can propose/vote; owner only can confirm/unlock/cancel. Use `auth.UserFromContext(ctx)` and the existing participant-check helper.
- [ ] On `ConfirmLocation`: if the winning proposal carries `geocoded` (not yet saved), save it via the existing location-save path and write the resulting `location_id` plus `latitude_deg`/`longitude_deg` to the experience. If it carries `location_id`, just denormalize lat/lng from the referenced `Location`.
- [ ] Emit the "location poll opened" chat system message **only on the first proposal of a fresh poll** (when `location_poll_active` flips from `false` to `true`), mirroring how `ProposeTime` handles it. This is what makes sequential proposal submission necessary on the client.
- [ ] Emit FCM notifications mirroring the time-poll plumbing (`server/community/notifications.go` if a notification copy update is needed; check what `ProposeTime` does today).
- [ ] Add storage migrations / table for `experience_location_proposals` mirroring the time-proposal table.
- [ ] **Use `connecterr.Internal(ctx, op, err, kv...)` for internal-error returns** — not `connect.NewError(connect.CodeInternal, err)`. The bare form is gated by `scripts/check_codeinternal.js` and fails CI ([`docs/server/architecture.md` § "Error wrapping"](../server/architecture.md)).
- [ ] **Structured logging** ([`docs/server/observability.md` § "Logging Usage Pattern"](../server/observability.md)): every handler builds its logger via `logger := logging.LoggerWithContext(ctx).With("operation", "ProposeLocation", "experience_id", req.Msg.ExperienceId, "location_proposal_id", proposal.Id)`. Use new entity-id field names (`location_proposal_id`, `location_poll_id`) consistent with the existing `gear_id`/`community_id`/`experience_id` convention. Do **not** call `log.Printf` / `fmt.Printf`.
- [ ] Write `server/services/experience/location_proposals_test.go` covering: propose, vote, change vote, multiple proposers, confirm (saved-spot path), confirm (drop-pin path that materializes a new `Location`), cancel, unlock, auth boundaries (non-participant can't propose, non-owner can't confirm). Use `storage.SetupTestStorage(t)` for the per-package shared testcontainer. Prefer table-driven unit tests where possible; integration coverage lives alongside.
- [ ] Run `npm run build` and `go build -tags=integration ./...` per CLAUDE.md's exported-API rule.

**Validates:** the data model is sound and the server can be driven through the full poll lifecycle from generated Go clients before any UI exists.

### Phase 2: Client end-to-end happy path
**Goal:** An organizer can open a location poll and voters can vote and the organizer can lock in a winner. UI is functional but unpolished.

**Architectural anchors** (per [`docs/client/architecture.md`](../client/architecture.md) and [`docs/client/time.md`](../client/time.md)):
- ViewModels are the **only** layer that calls repositories; repositories are the only layer that calls services. The new view model lands as an `AsyncNotifierProvider.family<LocationModalNotifier, LocationModalData, String>` keyed by `experienceId`, mirroring `TimeModalNotifier` ([time_modal_view_model.dart:881-895](../../app/lib/presentation/viewmodels/time_modal_view_model.dart#L881-L895)). On mutation, call `ref.invalidateSelf()` so `build()` re-runs against the fresh experience data.
- Modals open via `showAccessibleModal<T>(...)` wrapping a `GlassSheet` — never raw `showModalBottomSheet` (the `prefer_accessible_modal` lint rule fails the build otherwise). See [time_poll_propose_modal.dart] for the canonical shape.
- Tap interactions on proposal rows / vote tiles use `Tappable` / `Toggle` from [`app/lib/presentation/widgets/accessibility/`](../../app/lib/presentation/widgets/accessibility/) with `semanticsLabel:` sourced from `context.l10n.*`. `GestureDetector` / `InkWell` outside the accessibility primitives is rejected by `avoid_raw_gesture_detector`.

**Tasks:**
- [ ] Add client service methods in `app/lib/services/experience/` (mirror [rsvp_time_methods.dart](../../app/lib/services/experience/rsvp_time_methods.dart)): `proposeLocation()`, `voteOnLocation()`, `confirmLocation()`, `unlockLocation()`, `cancelLocationPoll()`. Use `RpcUtils.executeRpc()` for error handling.
- [ ] Extend `app/lib/data/repositories/experience_repository.dart` with mutation wrappers (`proposeLocation`, `voteOnLocation`, etc.) that call the service and then invalidate the experience cache (`_cache.invalidate(experienceId)`). ViewModels must call the repository, not the service directly.
- [ ] Build `app/lib/presentation/screens/experience/location_poll_propose_modal.dart` opened via `showAccessibleModal` + `GlassSheet`. Reuse [`LocationPickerModal`](../../app/lib/presentation/widgets/location/location_picker_modal.dart) for each spot. Bulk add (all spots → one submit) and live add (one at a time during an active poll) — mirror `time_poll_propose_modal.dart`. **Submit options sequentially**, never in parallel — parallel `ProposeLocation` calls would each see `location_poll_active=false` and race to mint duplicate poll-opened system messages (same hazard documented in [`docs/client/time.md` § "TimePollProposeModal"](../client/time.md)).
- [ ] Build `app/lib/presentation/screens/experience/location_poll_vote_modal.dart` — multi-select voting UI per the design ("tap any spot that works"). Use `Toggle` for each spot tile. Apply **optimistic UI** (toggle locally, fire RPC, roll back on failure) and scope the local selection state to an `autoDispose` selection provider so it doesn't leak between modal openings.
- [ ] Build `app/lib/presentation/screens/experience/location_poll_confirm_modal.dart` — owner picks the winner.
- [ ] Build `app/lib/presentation/screens/experience/unified_location_modal.dart` — routes between propose / vote / confirm based on role and `location_poll_active` / `location_poll_completed`. Mirror `unified_time_modal.dart`.
- [ ] Add `app/lib/presentation/viewmodels/location_modal_view_model.dart` as `AsyncNotifier.family<LocationModalData, String>` and a Freezed `location_modal_state.dart` (or carry data on `LocationModalData` directly). Every async method gates state writes behind `if (!ref.mounted) return;`.
- [ ] Extend `app/lib/presentation/viewmodels/conversation_experience_actions.dart` with `locationPollActive`, `isLocationPollVoter`, and helpers for the new RPCs.
- [ ] Wire the unified modal into [app/lib/presentation/screens/experience/widgets/experience_event_pane.dart](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart) — tap the location row to open it, parallel to how the time row opens `unified_time_modal`.
- [ ] Localization: add l10n keys to **both** `app/lib/l10n/app_en.arb` **and** `app_es.arb` (parity is enforced by `npm run lint:dart:l10n-parity`; English placeholders in `app_es.arb` are acceptable when a translation isn't ready). Use the `locationPoll*` prefix to mirror `timePoll*`. Accessibility labels use the `a11y` prefix. Per CLAUDE.md, user-facing copy must say **"Event"** not "Experience".

**Tests (alongside the implementation, per [`docs/client/testing.md`](../client/testing.md)):**
- [ ] `app/test/presentation/viewmodels/location_modal_view_model_test.dart` — `ProviderContainer` + Mockito-generated mocks of `ExperienceRepository`. Cover: propose → vote → confirm happy path; vote rollback on RPC failure; sequential submission of bulk proposals; disposal safety (start an in-flight RPC, dispose container, assert no throw).
- [ ] `app/test/data/repositories/experience_repository_test.dart` — extend with assertions that the new mutation wrappers invalidate the experience cache (cache miss after mutation, service called again).
- [ ] `app/test/presentation/screens/experience/location_poll_vote_modal_test.dart` — widget test for the optimistic toggle behavior, mirroring `time_poll_vote_modal_test.dart`.

**Validates:** the core UX works end-to-end on both organizer and voter devices; no surprises in the proposal-shape decision (saved vs unsaved).

### Phase 3: Polish — banners, results, notifications, parity
**Goal:** Location polls reach feature parity with time polls in every UI surface and in the chat transcript.
**Tasks:**
- [ ] Read [app/lib/presentation/widgets/planning/poll_banner.dart](../../app/lib/presentation/widgets/planning/poll_banner.dart) and [app/lib/presentation/widgets/experience/poll_results_view.dart](../../app/lib/presentation/widgets/experience/poll_results_view.dart). If they're genuinely poll-kind-agnostic, extend their inputs to accept either time- or location-poll state. If they're tightly coupled to time data, ship a parallel `location_poll_banner.dart` / `location_poll_results_view.dart` and file a follow-up TODO for consolidation.
- [ ] Wire location-poll banner into the Plan tab. **Scope active-poll state to `current_location_poll_id`** so a YES vote on a historical poll doesn't make the banner show "Voted ✓" for a newly-opened poll, and doesn't turn the experience FAB sage. This is the same scoping rule `PollBanner` and `experience_content_view.dart` already apply for time polls (see [`docs/client/time.md` § "Plan-Tab FAB Color"](../client/time.md)).
- [ ] Emit chat system messages on propose / cancel via `app/lib/presentation/widgets/chat/system_pill_content_factory.dart`. Verify the chat transcript renders cleanly.
- [ ] Update server notification copy and FCM payloads so voters get pinged when a poll opens or a winner is locked in.
- [ ] Full l10n sweep: every string in the new modals localized in **both** `app_en.arb` **and** `app_es.arb`; `npm run lint:dart:l10n-parity` and `npm run lint:dart:i18n` must pass.
- [ ] **Accessibility lint:** `npm run lint:dart:a11y` must report zero new violations. The `scripts/a11y_allowlist.txt` is empty and only shrinks. Verify `Tappable` / `Toggle` everywhere a tap or selection happens, `IconAction` for icon-only buttons, `LiveRegion` on the live "Poll is live · N of M voted" banner so screen-reader users hear the count update.
- [ ] Update `server/undo/registry.go` if any new `CommunityEventType` values were added (currently the plan reuses `EXPERIENCE_UPDATED`, so this is likely a no-op — but recheck after Phase 1). If any new `CommunityEventType` values **were** added, classify them per CLAUDE.md (`ClassificationClientOnly` is the right answer for vote/propose since they have natural client-side reversals, with `ClassificationServerSnackbar` for confirm if the design wants an undo affordance).
- [ ] End-to-end manual test on the iOS simulator and Android emulator with at least three participants (organizer + two voters) using the `/ripls-app-logs` skill to catch regressions.

**Validates:** parity with time polls — anything a time poll does in chat, notifications, banners, and the Plan tab, a location poll does too.

## Audit Notes

Changes folded into the plan after auditing against the reference docs listed at the top. Each item is the kind of detail that's easy to miss when copying the time-poll pattern by eye.

- **AsyncNotifier.family, not plain Notifier** ([`docs/client/architecture.md` § "Async Safety in Notifiers"](../client/architecture.md), [`docs/client/time.md`](../client/time.md)). The original plan said "mirror `time_modal_view_model.dart`" but didn't spell out that `TimeModalNotifier extends AsyncNotifier` with a family provider keyed by `experienceId`. AsyncNotifier handles disposal automatically in `build()` — preferred over manual `ref.mounted` checks. Phase 2 now states this explicitly.
- **Repository, not service, from view models** ([`docs/client/architecture.md` § "Key Architectural Rules"](../client/architecture.md), [`docs/client/caching.md` § "Caching Rules"](../client/caching.md)). The original plan had view models calling service methods directly. Corrected: services are wrapped by `ExperienceRepository`, view models call repository methods, and the repository owns cache invalidation. Phase 2 now adds explicit repository tasks.
- **`showAccessibleModal` + `GlassSheet`** ([`docs/client/accessibility.md`](../client/accessibility.md), [`docs/client/modals.md`](../client/modals.md)). `showModalBottomSheet` is gated by the `prefer_accessible_modal` lint. Confirmed both `unified_time_modal.dart:51` and `:105` use the accessible pair. Phase 2 now requires it.
- **Accessibility primitives** ([`docs/client/accessibility.md`](../client/accessibility.md)). Tap targets must be `Tappable` / `Toggle`, icon-only buttons `IconAction`, labels from `context.l10n.a11y*`. The `a11y_allowlist.txt` is empty and CI rejects any new violation. Phase 2 and 3 call this out as a hard gate.
- **`LiveRegion` on the live vote counter** ([`docs/client/accessibility.md`](../client/accessibility.md)). Status content that updates on screen ("Poll is live · 2 of 5 voted") should wrap in `LiveRegion` so screen readers re-announce on change. Added to Phase 3.
- **Locale parity in `app_es.arb`** ([`docs/client/i18n.md`](../client/i18n.md)). The original plan only mentioned `app_en.arb`. `npm run lint:dart:l10n-parity` will fail the build if a key exists in English but not Spanish. Phase 2 now lists both files and notes that English placeholders in `app_es.arb` are acceptable.
- **Sequential proposal submission** ([`docs/client/time.md` § "TimePollProposeModal"](../client/time.md)). Parallel `ProposeLocation` calls would each see `location_poll_active=false` and race to emit duplicate poll-opened system messages. Phase 2 now requires sequential submission; Phase 1 explains the server-side guarantee that pairs with it.
- **`currentLocationPollId` scoping for banner/FAB state** ([`docs/client/time.md` § "Plan-Tab FAB Color"](../client/time.md)). Without scoping, a YES vote on a previous poll triggers a false "Voted ✓" or sage FAB on the new poll. The proto fields already include `current_location_poll_id` — Phase 3 now calls out the scoping rule explicitly.
- **`autoDispose` selection provider** ([`docs/client/time.md`](../client/time.md)). `TimePollVoteSelectionProvider` is `autoDispose` so local selection state doesn't leak between modal openings. Phase 2 mirrors this requirement.
- **Optimistic UI for voting** ([`docs/client/time.md` § "TimePollVoteModal"](../client/time.md), [`docs/client/architecture.md` § "Optimistic Updates"](../client/architecture.md)). Tap should toggle locally before the round-trip and roll back on failure. Phase 2 calls this out.
- **`connecterr.Internal` for server errors** ([`docs/server/architecture.md` § "Error wrapping"](../server/architecture.md)). Bare `connect.NewError(connect.CodeInternal, err)` is gated by `scripts/check_codeinternal.js`. Phase 1 now mandates `connecterr.Internal(ctx, op, err, kv...)`.
- **Structured logging with `LoggerWithContext`** ([`docs/server/observability.md` § "Logging Usage Pattern"](../server/observability.md)). Every handler builds its logger from context, includes the standard `operation` field, and adds entity-specific fields (`location_proposal_id`, `location_poll_id`) following the existing naming convention. `log.Printf` is forbidden. Phase 1 spells this out.
- **`storage.SetupTestStorage(t)` for tests** ([`docs/server/architecture.md` § "Testing Strategy"](../server/architecture.md)). The shared testcontainer pattern is the convention; the original plan said "testcontainers pattern" but not the helper. Phase 1 now names it. Also adds the unit-test-first preference per the doc's "Unit tests are always preferable to integration tests."
- **Client test layering** ([`docs/client/testing.md`](../client/testing.md)). Phase 2 now has an explicit Tests subsection split by layer: view model tests using `ProviderContainer` + Mockito, repository tests asserting cache invalidation behavior, and a widget test for the optimistic toggle. Disposal safety test added explicitly per the doc's "All async Notifiers must have disposal tests."
- **`CommunityEventType` undo classification** (CLAUDE.md § "Adding New CommunityEventType Values"). If new event types are introduced, the `undo.Registry` entry is required to pass the `TestEveryEventTypeClassified` CI check. Phase 3 names the most likely classifications.
- **First-emission rule for poll-opened system messages** ([`docs/client/time.md` § "Inline Chat Banner"](../client/time.md)). The server should only emit the "poll opened" chat system message the first time `ProposeLocation` flips `location_poll_active` from `false` to `true`. Phase 1 calls this out — it's what the sequential-submission rule on the client is protecting.

## Related Issues

- #2068 — `docs/triage-2026-05-19` triage doc (referenced for context on the broader experience-planning roadmap).
- None of the open issues mention location polls directly; this is greenfield.
