# Issue #1196: Batch mode for adding Experience Requests and Offers

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1196
**Filed:** 2026-04-20 | **Last activity:** 2026-04-20
**Labels:** none | **Assignees:** unassigned
**Context docs:** [docs/experience_needs.md](../experience_needs.md), [docs/cowork/mocks/requests/offer-modal-batch.jsx](../cowork/mocks/requests/offer-modal-batch.jsx)

## Problem Statement

The add-Request and add-Offer flows on the Experience Plan tab are single-item round-trips today: open modal → type one item → save → modal closes → tap "+" → repeat. For the most common seeding scenario — the Experience owner (or an eager participant) populating an initial list of 3–10 items right after the Experience is created — this ceremony is a real friction. Each round-trip costs a modal animation, a network call, a cache invalidation, a re-render, and a fresh tap.

Batch mode should become the **default** for both the add-Request and add-Offer flows. A single modal lets the user stack multiple items in a pending list (each with its own name + optional note), then commit them all with one "Save all (N)" tap. The mock in [docs/cowork/mocks/requests/offer-modal-batch.jsx](../cowork/mocks/requests/offer-modal-batch.jsx) shows the exact UX — pending items stack as sage rows above the input, Enter-to-add for rapid keyboard entry, close-with-unsaved triggers a discard confirmation, and the trailing in-flight draft is folded into the batch on save.

The claim flow ("I Got This") stays single-item — batch mode is for *adding* new items, not for claiming.

## Context from Comments

The issue has no comments — it was just filed. All the requirements are in the body. Two important signals from the body:

- A **true batch RPC is preferred** over client-looping single-item RPCs, specifically to unify the chat side-effect (one system message, not N) and the cache invalidation (one refresh, not N).
- Chat coalescing is called out as aligning with #1188. That issue is already about collapsing rapid system messages in general. This batch feature lets us do the coalescing at the *source* (one batch-scoped message) instead of at render-time.

## Potential Solutions

### Option A: True batch RPCs with transactional semantics (recommended)

Add two new RPCs — `BatchAddExperienceNeeds` and `BatchAddExperienceContributions` — each taking a `repeated` of items. Server validates permissions once, inserts all rows in a single `storage.InsertBatch` transaction, posts a single coalesced chat system message (e.g. "Alice added 5 needs"), and returns the enriched responses in one shot. Clients get one cache invalidation and one refresh.

- **Approach:** New proto messages `BatchAddExperienceNeedsRequest { string experience_id; repeated BatchNeedItem items }` where `BatchNeedItem` is `{ string name; optional string note; int32 slots }`. Symmetric for contributions. Handler calls `requireParticipant()` + `loadActiveExperience()` once, validates each item, uses `InsertBatch` for the DB write, posts one system message via `SystemMessageWriter.InsertSystemMessage` (with a new `CHAT_SYSTEM_ACTION_EXPERIENCE_NEEDS_ADDED_BATCH` action — plural), and returns `repeated ExperienceNeedResponse`.
- **Pros:** Atomic — either all items land or none do; single chat message is clean; clean cache invalidation; no N+1 on the server; matches how other batch operations work in the codebase ([server/storage/protosql_batch.go](../../server/storage/protosql_batch.go)).
- **Cons:** Requires new proto types and new server handlers. If any single item is invalid (e.g. slot out of range), the whole batch rejects — which might surprise users mid-entry, but is the right default for atomicity.
- **Effort:** Medium.

### Option B: Client-loop over existing single-item RPCs

Keep the existing single-item RPCs untouched. The new batch UI calls `AddExperienceNeed` N times from the client, awaiting each, then issues a single cache refresh at the end.

- **Approach:** Client-only change. `ExperienceRepository.addNeedsBatch(List<NeedDraft>)` wraps a loop of single-item RPCs. Suppress the intermediate refreshes and do one at the end.
- **Pros:** Zero server work. Ships faster. Server contract stays minimal.
- **Cons:** N chat system messages — defeats the "clean" aspect of the feature and makes #1188 worse, not better; N round-trips over a mobile network (2–3s of latency for a 10-item list); no atomicity, so a mid-batch failure leaves a partial list the user has to clean up; race with cache invalidation means the client sees a flicker of partial state unless we carefully suppress refreshes.
- **Effort:** Small, but the UX degrades noticeably at list sizes > 3.

### Option C: Batch RPC but client still sends single-item shape

Add one new RPC per domain (`BatchAddExperienceNeeds`) but reuse the existing `AddExperienceNeedRequest` as the repeated payload type inside the batch request.

- **Approach:** `BatchAddExperienceNeedsRequest { repeated AddExperienceNeedRequest items }`.
- **Pros:** Less new proto surface.
- **Cons:** Violates the project convention that each RPC has its own dedicated request/response types ([CLAUDE.md](../../CLAUDE.md#L95-L99), [docs/proto_conventions.md](../proto_conventions.md)). Reusing `AddExperienceNeedRequest` inside a batch couples the two RPCs' contracts; an additive change to the single-item request (e.g. a new field we want only in the batch case) would leak across.
- **Effort:** Small, but it's the wrong shape for this repo.

## Recommendation

**Option A.** The whole reason we're doing this is to get atomic adds with one chat message and one cache refresh — Option B throws that away, and Option C trades a tiny amount of proto savings for a conventions violation that will cause drift later. Option A costs one extra afternoon of server work and gives the UX the cleanliness the mock implies.

Scope the first cut to **Needs and Contributions only** (no batch claim, no batch edit, no batch remove). Claim stays single because the mock explicitly keeps "I Got This" untouched, and the atomic slot decrement is the feature's whole point — batching claims would require thinking hard about partial-success semantics and we don't need that now.

## Audit Notes (2026-04-20)

Cross-referenced against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), and [docs/server/architecture.md](../server/architecture.md). Findings folded into the plan below:

- **i18n coverage was missing.** The mock hardcodes English strings ("Add another…", "What are you bringing?", "Save all (N)", "Or pick one", "Discard N items?", "Your list · N", "Offer something", etc.). Per [i18n.md](../client/i18n.md), every user-visible string must live in `app_en.arb` + `app_es.arb`, and plurals must use ICU message format (e.g. `Save all ({count})` → `{count, plural, =1{Save} other{Save all ({count})}}`). Added to Phase 3. The coalesced chat system message also needs ICU plural in ARB — added to Phase 4. Reminder from [i18n.md](../client/i18n.md): viewmodels must not resolve strings; the widget layer resolves via `context.l10n`.
- **Theme-aware colors were missing.** The mock uses literal tokens (T.coral, T.sageDeep, T.inputBg, etc.). Per [architecture.md §Colors and Theming](../client/architecture.md), UI must use `AppColors.xxx(context)` helpers so the modal works in both light and dark mode. Added to Phase 3 — map mock tokens to existing `AppColors` semantic colors, and add new named helpers only if no existing semantic fits.
- **Daily cache invalidation signal was missing.** Per [caching.md Pattern 7](../client/caching.md), mutation repositories call `dailyCacheInvalidationProvider.notify()` so the Daily screen refreshes automatically. The existing single-item `addNeed` / `addContribution` in [experience_repository.dart](../../app/lib/data/repositories/experience_repository.dart) does this. The new batch methods must mirror it. Added to Phase 2.
- **Transaction atomicity for insert + chat was not explicit.** Per [server/architecture.md §Storage Best Practices](../server/architecture.md#storage-best-practices), multi-operation writes should use `storage.BeginTx()`. Batch insert + coalesced chat system message is a multi-op write; if the chat insert fails mid-commit, we'd have orphaned needs with no system message. Added to Phase 1 — wrap the batch insert and the chat write in a single transaction.
- **Async safety in notifier methods was not called out.** Per [architecture.md §Async Safety in Notifiers](../client/architecture.md#async-safety-in-notifiers), `autoDispose` notifiers must guard `state =` assignments after awaits with `if (!ref.mounted) return;` — or use `SafeNotifierMixin` — since the Plan tab's `experienceNeedsProvider` is auto-dispose family. Added to Phase 2. Also added disposal tests per [architecture.md §Disposal Testing](../client/architecture.md#disposal-testing).
- **Server integration test was not listed.** Per [server/architecture.md §Testing Strategy](../server/architecture.md#testing-strategy), cross-component flows (RPC + storage + chat + cache side-effects) warrant an integration test in addition to unit tests. Added to Phase 1.
- **No architecture violations found.** Option A already respects service independence (single `ExperienceService`, no cross-service calls), the "each RPC has its own request/response" convention, the proto API/models separation, and the "repositories own caching, not viewmodels" rule.

## Open Questions

- **Should the batch RPC be all-or-nothing, or partial-success?** — *Recommendation: all-or-nothing.* Validate every item server-side first (name non-empty, slots in [1, 20]); if any fails, reject the whole batch with a structured error naming the offending index. Users aren't mixing valid + invalid items on purpose — if slot 3 fails validation, we want to surface it before half the list is committed. The client already validates before enabling Save, so server-side failures are rare-path. Partial-success would force us to design a "here's what succeeded and what didn't" UI that we probably never use.

- **How should the coalesced chat system message render?** — *Recommendation: one-line summary plus an expandable chip in the message payload carrying the item IDs.* For a batch of N, render as "[Name] added N needs" (or "added N contributions"). The message's structured fields carry the IDs of the created items so the client can render inline chips like it does for single adds today. For N=1, fall back to the existing single-item message shape (don't pluralize oddly for edge cases). For batches that include notes, the notes don't go in the system message body — they live on the need/contribution records and are already shown in the Plan tab.

- **Should the client's pending-list draft survive a modal dismissal?** — *Recommendation: no, but confirm before discarding.* The mock already shows the discard confirmation overlay ("Discard N items? Nothing's been saved yet"). That's enough. Persisting drafts across modal lifecycles introduces a new "where did my list go?" class of bug and a storage footprint we don't need.

- **Do we keep the single-item "Add" RPCs in the API or retire them?** — *Recommendation: keep them.* The batch RPC with a single item is functionally equivalent, but deprecating a public API is more work than it's worth given both server handlers share most of their logic. The single-item RPCs also handle the auto-RSVP path a bit differently (fewer items → simpler logging). Keep both; the client simply routes to the batch RPC from the new modal flow.

- **What about the suggestion chips?** — *Recommendation: unchanged behavior.* Tapping a suggestion populates the name input as it does today; the user still has to decide whether to "Add to list" or "Save" directly. The suggestion list doesn't need to become multi-select in v1 — if we later see users tapping multiple chips in rapid succession, we can add multi-select as a follow-up.

- **Slots input for batch needs** — *Recommendation: per-item slot stepper inline in the pending row, defaulting to 1.* The mock shows the Offer flow (no slots). For the Request flow, each pending row should display a slot pill/stepper next to the × button so the user can bump it from the default of 1 without re-opening anything. This keeps rapid entry fast (the common case is slots=1) while still allowing per-item tuning.

## Implementation Plan

### Phase 1: Server batch RPCs

**Goal:** Both batch RPCs exist, are tested, and produce one coalesced chat message per batch — atomically.

**Tasks:**
- [ ] Add `BatchAddExperienceNeedsRequest` / `BatchAddExperienceNeedsResponse` and `BatchAddExperienceContributionsRequest` / `BatchAddExperienceContributionsResponse` to [proto/ripls/api/experience_service.proto](../../proto/ripls/api/experience_service.proto), with inline repeated item messages (per proto conventions — each RPC owns its types; no reuse of the single-item request type).
- [ ] Add `CHAT_SYSTEM_ACTION_EXPERIENCE_NEEDS_ADDED_BATCH` and `CHAT_SYSTEM_ACTION_EXPERIENCE_CONTRIBUTIONS_ADDED_BATCH` enum values to the chat system action proto.
- [ ] Implement `BatchAddExperienceNeeds` in [server/services/experience/needs.go](../../server/services/experience/needs.go): single permission check (`requireParticipant` + `loadActiveExperience`), per-item validation, then open a single `storage.BeginTx` transaction that (a) runs `InsertBatch` for the needs and (b) calls `SystemMessageWriter.InsertSystemMessage` for the coalesced plural action — commit together, rollback together, so we never end up with orphaned needs or orphaned chat messages.
- [ ] Implement symmetric `BatchAddExperienceContributions` in [server/services/experience/contributions.go](../../server/services/experience/contributions.go) with the same transactional pattern.
- [ ] Server unit tests: happy path for 1, 2, and 10 items; rejection on any invalid item (validates all-or-nothing semantics); chat message action/payload IDs are correct; terminal-experience guard still fires; non-participant rejection still fires; simulated chat write failure rolls back the need/contribution inserts.
- [ ] Server integration test in [server/main_integration_test.go](../../server/main_integration_test.go): full RPC → storage → chat → list flow for a 5-item batch, asserting one system message row and N need rows in the DB.
- [ ] Run `npm run generate` + `npm run build` to confirm proto regen, `golangci-lint`, and `go build ./...` pass.

**Validates:** The server contract is correct and atomic. We can exercise it via grpcurl / a manual test harness before any client work lands.

### Phase 2: Client repository + view model

**Goal:** The Flutter layer can call the new RPCs and cache-refresh correctly, with no disposal crashes.

**Tasks:**
- [ ] Add `addNeedsBatch(String experienceId, List<NeedDraft>)` and `addContributionsBatch(String experienceId, List<ContributionDraft>)` to [app/lib/data/repositories/experience_repository.dart](../../app/lib/data/repositories/experience_repository.dart). Each method invalidates the `needs_and_contributions:{experienceId}` cache key exactly once after the RPC returns, then calls `dailyCacheInvalidationProvider.notify()` so the Daily screen refreshes (per [caching.md Pattern 7](../client/caching.md)) — matching what the single-item methods already do.
- [ ] Add `addNeedsBatch` / `addContributionsBatch` methods to [app/lib/presentation/viewmodels/experience_needs_view_model.dart](../../app/lib/presentation/viewmodels/experience_needs_view_model.dart), wrapping with `isMutating=true` and calling `_refreshAfterMutation()`. Because the provider is auto-dispose family, every `state = state.copyWith(...)` after an await must be guarded with `if (!ref.mounted) return;` — or adopt `SafeNotifierMixin` on the notifier if it isn't already.
- [ ] Handle the auto-RSVP path: if the user has no RSVP, submit a single YES RSVP *before* the batch call (same pattern as the single-item path today). If the RSVP fails, abort without calling the batch RPC.
- [ ] Repository unit tests with mocked service: batch of 1 (equivalence to single-add), batch of N, error passthrough, cache invalidation is called exactly once, `dailyCacheInvalidationProvider.notify()` is called exactly once.
- [ ] View-model unit tests for the new methods, including a disposal test per [architecture.md §Disposal Testing](../client/architecture.md#disposal-testing) that starts a batch call, disposes the container mid-flight, and asserts the future completes without throwing.

**Validates:** End-to-end plumbing is correct, cache side-effects propagate to the Daily screen, and the viewmodel survives mid-flight disposal.

### Phase 3: Batch modal UI

**Goal:** Ship the mock, localized and theme-aware. Batch mode is the default add flow for both Requests and Offers.

**Tasks:**
- [ ] Rewrite `AddContributionSheet` in [app/lib/presentation/widgets/experience/experience_needs_sheets.dart](../../app/lib/presentation/widgets/experience/experience_needs_sheets.dart) to match the mock: pending list, Enter-to-add, adaptive placeholder, "Add to list" + "Save / Save all (N)" actions, discard-confirmation overlay.
- [ ] Mirror the same structure in `AddNeedSheet`, adding a per-item slot stepper to each pending row.
- [ ] Preserve the existing suggestion-chip behavior and the filtered-by-existing-labels logic.
- [ ] Wire both sheets to call the new `addNeedsBatch` / `addContributionsBatch` view-model methods.
- [ ] **i18n:** Add ARB entries to both [app/lib/l10n/app_en.arb](../../app/lib/l10n/app_en.arb) and [app/lib/l10n/app_es.arb](../../app/lib/l10n/app_es.arb) for every new string: `experienceBatchOfferTitle` ("Offer something"), `experienceBatchNeedTitle` ("Request something"), `experienceBatchInputHintFirst` ("What are you bringing?"), `experienceBatchInputHintSubsequent` ("Add another…"), `experienceBatchNoteHint` ("Add a note"), `experienceBatchSuggestionsLabel` ("Or pick one"), `experienceBatchListLabel` ("Your list · {count}") — with `{count}` placeholder, `experienceBatchAddToList` ("Add to list"), `experienceBatchSaveLabel` ("{count, plural, =0{Save} =1{Save} other{Save all ({count})}}") — ICU plural, `experienceBatchDiscardTitle` ("{count, plural, =0{Discard what you've typed?} =1{Discard 1 item?} other{Discard {count} items?}}") — ICU plural, `experienceBatchDiscardBody` ("Nothing's been saved yet. You can go back and tap Save to keep it."), `experienceBatchKeepEditing` ("Keep editing"), `experienceBatchDiscard` ("Discard"). All resolution happens in the widget layer via `context.l10n` — the viewmodel holds counts/state only, never rendered strings ([i18n.md §Best Practices](../client/i18n.md)).
- [ ] **Theme colors:** Replace the mock's hardcoded tokens with `AppColors.xxx(context)` helpers per [architecture.md §Colors and Theming](../client/architecture.md). The sage-background rows for pending items will likely need a new semantic helper in [app/lib/core/theme/app_colors.dart](../../app/lib/core/theme/app_colors.dart) (e.g. `AppColors.confirmedBackground(context)`); the coral accents should map to the existing primary/accent helpers. Verify dark mode manually before merge.
- [ ] Widget tests (use the `localizationsDelegates` helper from [app/test/helpers/l10n_helpers.dart](../../app/test/helpers/l10n_helpers.dart)): rapid-entry flow (type → Enter → type → Save all), removing a pending item, discard confirmation on close-with-unsaved, suggestion-chip tap populates the input, plural "Save all (N)" renders correctly for N=1/2/5.
- [ ] Run `npm run lint:dart:i18n` to catch any hardcoded strings that slipped in.
- [ ] Manual QA on iOS and Android via the [ripls-app-logs](../../.claude/skills/ripls-app-logs/SKILL.md) skill: seed a 5-item list on a fresh Experience in both English and Spanish locales; verify Plan tab reflects all 5 after save; verify chat shows exactly one "added N needs" system message; verify dark mode.

**Validates:** The end-user-visible feature works, renders correctly in both locales and themes, and the chat output matches the coalesced-message goal.

### Phase 4: Chat message rendering

**Goal:** The coalesced batch system message renders cleanly, localized, and keeps parity with single-item system messages.

**Tasks:**
- [ ] Handle the two new system action enums in [app/lib/presentation/widgets/chat](../../app/lib/presentation/widgets/chat/) (wherever system messages are rendered — follow the pattern used for existing `CHAT_SYSTEM_ACTION_EXPERIENCE_NEED_ADDED`).
- [ ] Add ARB entries for the coalesced system message using ICU plural so N=1 collapses to the single-item phrasing without a Dart conditional: `chatSystemNeedsAddedBatch` → `{count, plural, =1{{actor} added a need} other{{actor} added {count} needs}}` and the symmetric `chatSystemContributionsAddedBatch`. Both locales (en/es).
- [ ] Keep all string resolution in the widget layer — the client should read the count + actor from the structured message payload and pass them to `context.l10n.*` for rendering ([i18n.md §Best Practices](../client/i18n.md): viewmodels never resolve strings).
- [ ] Widget test for both plural-batch and single-from-batch rendering in both locales.

**Validates:** The chat output in the Experience conversation is clean after a batch add — one row, correct wording in each supported locale, tappable chips for each created item.

### Phase 5 (optional, follow-up): Retire single-item modal paths

**Goal:** Simplify the codebase once the batch modal has baked.

**Tasks:**
- [ ] Remove any dead UI paths that went through the old single-item sheet layout.
- [ ] Keep the single-item RPCs alive on the server (see Open Questions) — but delete the now-unused single-item client wrapper methods if truly unused.

**Validates:** Nothing — this is pure cleanup. Only do after a few days of the batch flow being live.

## Related Issues
- #1186 — Original "experience collaborative lists" feature this extends. Batch mode is a UX refinement on top of the single-item flows shipped there.
- #1188 — Coalesce rapid system messages in chat. Batch-add is the cleanest way to coalesce at the source; #1188 handles the render-time case when separate actions happen in rapid succession.
- #1193 — Rethink Plan tab UX: surface descriptions and reduce chat system message noise. Batch-add reduces system message noise directly (N messages → 1).
- #1194 — Auto-generate Plan tab suggestions. Suggestion chips feed the batch input; no blocking dependency, but both touch the same sheets.
