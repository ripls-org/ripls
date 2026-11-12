# Issue #1580: Lightweight experimental sampling (ESM): post-event signal cards in the feed (manual v1)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1580
**Filed:** 2026-04-29 | **Last activity:** 2026-04-29
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:**
- [docs/users/priorities.md](../users/priorities.md)
- [docs/cowork/App UXR/impact-screen-proactive-host-brief.md](../cowork/App%20UXR/impact-screen-proactive-host-brief.md)
- [docs/users/host-problems.md](../users/host-problems.md)
- Companion plan: [docs/issues/1579-workshop-tab.md](1579-workshop-tab.md)

## Problem Statement

The Workshop tab (#1579) needs a steady supply of *post-event signal* — short prompts answered by attendees telling the system whether an event was worth doing again, what was missing, whether the time worked. Without that signal, the Workshop's most important cascade priority (*recently completed event with strong ESM repeat signal ≥60%*) and the repeat-or-rest accept lever both fall back to empty states every time.

This issue ships a **manual** v1: an operator authors an ESM prompt against a specific event, the prompt surfaces as a feed card to the recipient set the operator chose, recipients answer with a quick tap, and the responses are persisted in a host-aggregable form. v2 (out of scope, follow-up issue) automates the trigger from event-end events, the targeting from confirmed attendees, and the prompt selection from a small bank.

The card is the *artifact in the field* — even hand-rolled — that we need before the Workshop ships, both to validate the surface and to give the Workshop real signal to design against.

## Context from Comments

The issue has no comments. The companion plan #1579 references this work as the seam that fills cascade priorities (2) and (3) and the repeat-or-rest lever; the read-side aggregation contract is defined here and consumed there. Privacy and tone constraints in the issue body are explicit and load-bearing — responses must stay inside the system, the host sees aggregates only (no per-user attribution), and prompts are about events, never about people.

## Potential Solutions

### Option A: New FeedItem oneof case + dedicated ESM service + shared library, modeled on the nudge infrastructure

- **Approach:** Add a new `esm_prompt` case to the `FeedItem.payload` oneof in [proto/ripls/api/feed_service.proto](../../proto/ripls/api/feed_service.proto) (lines 112–124). Add a new storage model `StoredESMPrompt` mirroring the [StoredNudge](../../proto/ripls/models/nudge.proto) shape (pre-persisted, soft-deletable, consumption-tracked). Create a **dedicated ESM service** at [server/services/esm/](../../server/services/esm/) hosting `RespondToESMPrompt` and `GetESMSignal`. Storage queries, aggregation, and feed-item production live in a shared library at [server/esm/](../../server/esm/) consumed by both the new ESM service and the existing feed service — neither service depends on the other. Persist responses in a separate `StoredESMResponse` table keyed by `(user_id, prompt_id)`. Expose aggregated signal via `GetESMSignal` returning counts/percentages — never per-user attribution. **No operator CLI** in v1 — manual authoring is a documented runbook using the shared library helpers directly. Client renders via a new `EsmPromptContentView` dispatched from the feed screen's switch in [feed_screen.dart:392–486](../../app/lib/presentation/screens/feed/feed_screen.dart#L392-L486).
- **Pros:** Cleanest separation; nudge infrastructure is the closest existing template (pre-persist, mark consumed, soft-delete) so the cost is mostly in plumbing, not in inventing patterns. Privacy boundary is clear because responses live in their own table the read API never exposes raw. v2 (automated trigger) drops in behind the same authoring surface.
- **Cons:** New oneof case touches feed protos, server generation, and client dispatch — three coordinated changes. Adding a new RPC for aggregation requires careful contract design so #1579 can consume it without rework.
- **Effort:** Medium.

### Option B: Reuse the nudge type with a new nudge subkind for ESM

- **Approach:** Don't add a new feed item type. Add a `prompt` variant to `StoredNudge` (currently a server-generated nudge with a CTA) carrying a question + 2–4 quick-tap response options. The client renders a different layout when the nudge is a prompt subkind. Responses captured via a new `RespondToNudge` RPC.
- **Pros:** No new feed item type, no new dispatch case in `feed_screen.dart`, less new infrastructure.
- **Cons:** Conflates two different jobs into one type. Nudges are *the system suggesting an action to a user* (single CTA tap → side effect); ESM prompts are *the system asking the user a structured question* (1-of-N response, eventual aggregation). Mixing them complicates the nudge consumption model (`ConsumeNudge` returns no payload), forces the aggregation read API to filter nudge rows by subkind on every query, and makes the privacy story harder (nudges have no aggregation surface; the moment they do, the boundary between "consumed action" and "structured response" gets blurry). Also forces decisions about whether nudge stock-imagery and AI-generation paths apply to ESM prompts (they don't).
- **Effort:** Medium — superficially smaller but with real downstream cost.

### Option C: Out-of-band channel (push notification or email) instead of feed card

- **Approach:** Skip the feed entirely. Send the ESM prompt as a push notification or email with response options as deep links.
- **Pros:** Decoupled from the feed; no proto changes; pre-existing channels.
- **Cons:** The issue explicitly calls for "cards in the feed, visually similar to existing content views" — a push/email channel is a different product. Push has friction (permission, opt-out, OS UX), email is high-latency. Feed cards land where the user already is and match the brief's expectation that ESM is a quiet, in-app, low-friction surface. Out-of-band response also fragments the response capture (deep link → app open → response submission → server) versus a single in-app tap.
- **Effort:** Small (push) to Medium (email), but ships the wrong product.

## Recommendation

**Option A.** The nudge infrastructure is the right template, but ESM prompts are a *different shape of object* (structured question + multi-option response + aggregable signal vs. nudge's single CTA action). Forcing them into the same type (Option B) saves a few lines of proto and costs us a clean privacy boundary on the aggregation read API. Option C ships the wrong channel.

The biggest tradeoff with Option A is the coordinated change across three layers (proto, server feed assembly, client dispatch), but each layer's change is small and follows an existing template. The resulting privacy story is also clean: responses live in their own table, the read API exposes only aggregated counts, and operator authoring is a separate CLI rather than entangled with the dev-only admin RPCs.

The aggregation contract for `GetESMSignal` is the most consequential part of this issue — once #1579 is built against it, it's load-bearing. v2 (automated trigger) can change *who* gets prompts and *when*, but the response shape and aggregation API should stay stable.

## Open Questions

- **Should responses be tied to `ExperienceRSVP.attended` for eligibility, or trust the operator's recipient list in v1?** — → **Decided (2026-04-29):** Trust the operator's recipient list, but the manual authoring path **prefills** the list with all users who have `ExperienceRSVP.attended == YES` for the target event. The operator can edit the list before submission. This gives v2 a forward-compatible contract (its automated trigger uses the same `attended == YES` filter, just without the human-in-the-loop edit step).
- **Is the ESM prompt scoped per-event, or can one prompt span multiple events?** — → **Decided (2026-04-29):** Per-event in v1. `StoredESMPrompt.experience_id` is required. Cross-event prompts are deferred to v3.
- **Where do free-text follow-up responses go, given the no-PII / no-relationship-evaluation constraint?** — → **Decided (2026-04-29):** Proceed with the recommendation — free-text responses stored alongside structured responses, exposed only to the host of the event in question, never aggregated as a public surface, never visible to other respondents.
- **What's the consumption model — does responding remove the card, or does dismissing also count?** — → **Decided (2026-04-29):** Proceed with the recommendation — both `RESPONDED` and `DISMISSED` clear the card; aggregation only counts `RESPONDED` rows. Matches the nudge model.
- **Recipient deduplication across feed reloads** — → **Decided (2026-04-29):** Proceed with the recommendation — `(user_id, prompt_id)` consumption filter, mirroring the nudge pattern.
- **Aggregation read API: per-question or per-prompt?** — → **Decided (2026-04-29):** Proceed with the recommendation — per-prompt. `GetESMSignal` returns one `ESMSignal` per `(experience_id, prompt_id)` pair.
- **Auth model for the manual authoring path** — → **Decided (2026-04-29):** Do not build an admin tool / operator CLI / new RPC. v1's "manual" authoring is a documented runbook (psql commands or a one-off Go script run from a developer's machine against dev/staging) that inserts `StoredESMPrompt` + `StoredESMResponse` rows directly via the storage layer. No committed binary, no new admin RPC. Phase 2 of the implementation plan was rewritten to reflect this.
- **Which service hosts the new RPCs (feed vs. experience vs. new ESM service)?** — → **Decided (2026-04-29):** Create a dedicated `esm_service` now. Even though v1's RPC surface is small, the user expects ESM to grow substantially in scope (attendance-trend, decay-revival, multi-prompt aggregations, the v2 automated trigger machinery), and the cost of refactoring later outweighs the cost of factoring it cleanly now. New proto: [proto/ripls/api/esm_service.proto](../../proto/ripls/api/esm_service.proto). New service package: [server/services/esm/](../../server/services/esm/). The feed pipeline reads ESM data via a shared library at [server/esm/](../../server/esm/) (Pattern 1 — Standalone Libraries) so feed service does not depend on esm service (Service Independence).
- **Card frequency cap per recipient across all prompts?** — Recommendation: out of scope for v1 (the issue body explicitly punts this to v2). v1 ships without a global cap; the per-user-per-prompt dedup is sufficient because there are only a handful of operator-authored prompts in v1.

## Implementation Plan

### Phase 1: Proto contract, dedicated ESM service, and storage shared library

**Goal:** The data model is stable, ESM is its own service from day one, and the read API is shaped so #1579 can consume it without rework. The feed pipeline reads ESM data via a shared library — no service-to-service coupling.

**Tasks:**

*Storage models (proto/ripls/models/)*
- [ ] Add `StoredESMPrompt` to a new [proto/ripls/models/esm.proto](../../proto/ripls/models/esm.proto). Fields: `id`, `experience_id`, `community_id`, `question` (single string), `response_options` (repeated `ESMResponseOption{key, label}`), `allow_free_text` (optional bool), `created_at_unix_sec`, `closes_at_unix_sec` (default: `experience.completed_at_unix_sec + 48h`), `created_by_user_id`, `deleted` (`DeletedMetadata` for soft delete). Use `optional` for any conditionally-present field per the proto field conventions in [CLAUDE.md](../../CLAUDE.md).
- [ ] Add `StoredESMResponse` to the same file. Fields: `id`, `prompt_id`, `experience_id`, `user_id`, `consumed_at_unix_sec` (optional), `consumed_action` (enum: `UNRESOLVED` / `RESPONDED` / `DISMISSED`; row created with `UNRESOLVED` at prompt-authoring time, the row's existence is what makes the prompt visible to that recipient), `response_option_key` (optional, set only when `consumed_action == RESPONDED`), `free_text` (optional).
- [ ] Document in the proto comment that responses are persisted in a dedicated table (no `CommunityEvent` row), so no `undo.Registry` entry is needed in [server/undo/registry.go](../../server/undo/registry.go).

*New ESM service (proto/ripls/api/ + server/services/esm/)*
- [ ] Create [proto/ripls/api/esm_service.proto](../../proto/ripls/api/esm_service.proto). Service: `ESMService` with RPCs (each with dedicated request/response types per CLAUDE.md):
  - `RespondToESMPrompt(prompt_id, response_option_key?, free_text?, dismissed: bool) → empty`
  - `GetESMSignal(experience_id) → list of ESMSignal{prompt_id, respondent_count, response_distribution: map<option_key, count>, free_text_count, completed_at_unix_sec}`
  - Note: `GetESMSignal` is the read API #1579 consumes for cascade priority (2). Shape it so future #1579 changes don't require server changes.
- [ ] Create the service package [server/services/esm/](../../server/services/esm/) with `service.go` (struct + `New()` constructor taking `*storage.ProtoSQLStorage`), `responses.go` (the `RespondToESMPrompt` handler), `signal.go` (the `GetESMSignal` handler), `authorization.go` (auth helpers — see below), and matching `*_test.go` files. Each file ≤1000 lines per [docs/server/architecture.md](../server/architecture.md).
- [ ] Wire the new service into the server's main route registration alongside existing services.
- [ ] Authorization helpers in `server/services/esm/authorization.go`:
  - `requireESMRespondent(ctx, storage, promptID)` — the calling user must have a `StoredESMResponse` row keyed `(user_id, prompt_id)` with `consumed_action == UNRESOLVED`; prompt must not be expired or soft-deleted.
  - `requireESMSignalReader(ctx, storage, experienceID)` — only the event host (or community owner) can read aggregate signal.
  - If "host of event" or "community owner" checks already exist as service-specific helpers elsewhere, **do not inject those services** (Service Independence rule in [docs/server/architecture.md](../server/architecture.md)). Copy the helper into `esm/authorization.go`, or factor a shared `auth.RequireExperienceHost` into [server/auth/](../../server/auth/) if it's broadly reusable.

*ESM shared library (server/esm/)*
- [ ] Create [server/esm/](../../server/esm/) as a shared library (Pattern 1 — Standalone Libraries for Shared Logic per [docs/server/architecture.md](../server/architecture.md)). Files: `prompts.go` (storage queries for prompts), `responses.go` (storage queries for responses), `aggregation.go` (the pure function `Aggregate([]StoredESMResponse) []ESMSignal`), `feed_items.go` (helper that produces `ESMPromptPayload` feed items for a given user), and matching `*_test.go` files.
- [ ] **Both the new ESM service and the existing feed service consume this library.** The feed service uses `esm.FeedItemsForUser(ctx, storage, userID)` during feed assembly; the ESM service uses `esm.RecordResponse(...)`, `esm.QueryResponses(...)`, and `esm.Aggregate(...)`. Neither service depends on the other; both depend on the storage layer and the shared library. This is the architectural contract that justifies a separate ESM service today.
- [ ] Storage helpers added to the library: `QueryPromptsByExperience(experienceID)`, `QueryResponsesByPrompt(promptID)`, `QueryUnresolvedResponsesForUser(userID)`. Index on `(user_id, consumed_action)` for the feed assembly query, and on `(experience_id, prompt_id)` for `GetESMSignal`. Use `InsertBatch` for the per-recipient row insertion path used in Phase 2 per the SQL Efficiency rules in [CLAUDE.md](../../CLAUDE.md).

*Feed integration (proto/ripls/api/feed_service.proto + server/services/feed/)*
- [ ] Add new `ESMPromptPayload` message to [proto/ripls/api/feed_service.proto](../../proto/ripls/api/feed_service.proto) under the existing `*Payload` patterns. Fields: `prompt_id`, `experience_id`, `experience_title` (denormalized for client display), `question`, `response_options` (repeated `{key, label}`), `allow_free_text`, `closes_at_unix_sec`. API uses inline/flattened scalars per [docs/proto_conventions.md](../proto_conventions.md). The payload definition stays in feed_service.proto because it's a feed item shape; the response/aggregation contract lives in esm_service.proto.
- [ ] Add a new `oneof` case `esm_prompt = 23` to `FeedItem.payload` (after `nudge = 22`). Add a corresponding `FEED_ITEM_TYPE_ESM_PROMPT` value to `FeedItemType` enum.
- [ ] In [server/services/feed/service.go](../../server/services/feed/service.go), call `esm.FeedItemsForUser(...)` from the shared library during feed assembly and merge the returned items with the rest of the feed. If this pushes `feed/service.go` over 1000 lines, factor into `feed/esm_items.go` per the file-organization pattern.

*Codegen and tests*
- [ ] Run `npm run generate` and verify Go + Dart codegen lands.
- [ ] Unit tests in `server/esm/`: storage round-trip; `Aggregate` arithmetic (verify ≥60% / 30–60% / <30% threshold cases match what #1579 expects, including the empty-input and single-respondent edge cases); `FeedItemsForUser` correctly excludes resolved/expired/deleted rows.
- [ ] Unit tests in `server/services/esm/`: RPC handler authorization (host can read aggregate, non-host cannot); RPC handler response validation (rejects responses to expired/deleted prompts); `RespondToESMPrompt` rejects calls from users without an `UNRESOLVED` row.
- [ ] Unit tests in `server/services/feed/`: feed assembly correctly merges ESM items by `occurred_at_unix_sec` and excludes consumed prompts.

**Validates:** The data shape and read API are concrete enough that #1579 can wire `WorkshopRepository` against `GetESMSignal`. Privacy boundary is enforced server-side. The feed pipeline reads ESM data without depending on the ESM service. Tests cover the auth and aggregation logic that the rest of the system will trust.

### Phase 2: Manual prompt authoring runbook (no committed CLI)

**Goal:** A developer can author an ESM prompt against a real event in dev/staging without us building a permanent admin tool. Recipient list defaults to actual attendees so the prompt targets the right people by default.

**Decision (2026-04-29):** No operator CLI binary, no new admin RPC, no new admin UI. v1's "manual" authoring is direct storage manipulation, documented in a runbook. v2's automated trigger is what replaces this — at which point the runbook is retired.

**Tasks:**
- [ ] Add a helper function to the `server/esm/` shared library: `BuildPromptForExperience(ctx, storage, experienceID, question, options, allowFreeText, recipients) (*StoredESMPrompt, []*StoredESMResponse, error)`. Pure function — produces the rows but does not insert them. Returns the prompt row and one `StoredESMResponse` row per recipient with `consumed_action = UNRESOLVED`.
- [ ] Add a helper function `AttendeeUserIDs(ctx, storage, experienceID) []string` that returns the user IDs of `ExperienceRSVP` rows with `attended == YES` for the given experience. This is what the runbook prefills the recipient list from. Document that the attendee list is the prefill default but the caller can replace or edit it before insertion.
- [ ] Write the runbook at [docs/runbooks/esm-manual-authoring.md](../runbooks/esm-manual-authoring.md). It should cover, in order:
  1. SSH/port-forward into dev or staging.
  2. Open a `psql` session OR run a one-off Go script (template provided in the runbook) that uses the storage layer.
  3. Look up the target `experience_id`.
  4. Call `AttendeeUserIDs` (or the equivalent SQL) to get the prefilled recipient list. Optionally edit it.
  5. Call `BuildPromptForExperience` with the question, options, recipient list. Insert the prompt row and the batch of response rows via `InsertBatch`.
  6. Verify the rows landed by querying `StoredESMPrompt` and `StoredESMResponse` for the experience.
- [ ] The runbook is the only "tooling" delivered in v1. The Go script is a template, copy-pasted by the developer for each prompt; **do not commit a binary at `server/cmd/esm-*`**.
- [ ] Tests: integration test in `server/esm/` that round-trips `BuildPromptForExperience` + manual storage insertion + feed assembly query, asserting the rows appear correctly for a target user and that calling `RespondToESMPrompt` flips the row to `RESPONDED` and removes the item from subsequent feed reads. Test that an expired prompt does not appear even if the response row is `UNRESOLVED`. Test that `AttendeeUserIDs` returns the right set for a fixture event.

**Validates:** A developer can author a real ESM prompt against a real completed event in dev/staging following the runbook, and the resulting feed cards land in the right users' feeds. The shared-library helpers are reusable by v2's automated trigger without rework.

### Phase 3: Client card and feed dispatch

**Goal:** The card renders in the feed visually consistent with existing content views, supports quick-tap response and dismiss, and never surfaces twice for the same user.

**Tasks:**
- [ ] Create `EsmPromptContentView` at [app/lib/presentation/widgets/esm/esm_prompt_content_view.dart](../../app/lib/presentation/widgets/esm/esm_prompt_content_view.dart). Modeled on [nudge_content_view.dart](../../app/lib/presentation/widgets/nudge/nudge_content_view.dart). Renders: question text, 2–4 quick-tap response buttons, optional free-text input (if `allow_free_text` is set), dismiss button.
- [ ] All chrome strings via `context.l10n.*` per [docs/client/i18n.md](../client/i18n.md), with new keys (e.g., `esmDismiss`, `esmFreeTextHint`, `esmSubmit`) added to **both** [app/lib/l10n/app_en.arb](../../app/lib/l10n/app_en.arb) and [app/lib/l10n/app_es.arb](../../app/lib/l10n/app_es.arb) before running `npm run generate`. CI's `npm run lint:dart:i18n` will fail otherwise. The question text and option labels are operator-authored data (not chrome) and pass through as dynamic strings — they're not l10n keys. The viewmodel exposes typed state (response option keys + values), the widget resolves chrome via `context.l10n` in `build()`.
- [ ] All colors via `AppColors.xxx(context)` per [docs/client/architecture.md](../client/architecture.md); light + dark theme aware.
- [ ] Dispatch case in [app/lib/presentation/screens/feed/feed_screen.dart](../../app/lib/presentation/screens/feed/feed_screen.dart): add `case FeedItemType.FEED_ITEM_TYPE_ESM_PROMPT: return EsmPromptContentView(...)` to the `_buildFeedItem` switch (lines 392–486).
- [ ] Repository layer: new `EsmRepository` at [app/lib/data/repositories/esm_repository.dart](../../app/lib/data/repositories/esm_repository.dart). Constructor takes `CacheManager` and the generated `FeedServiceClient` exactly like other repositories in the codebase (e.g., [GearRepository](../../app/lib/data/repositories/gear_repository.dart)); internally wraps `CacheService(cacheManager, 'esm')` per [docs/client/caching.md](../client/caching.md). Methods: `respond(promptId, optionKey, freeText)`, `dismiss(promptId)`. ViewModels read from this repository, never from the service directly, per the strict MVVM layering in [docs/client/architecture.md](../client/architecture.md).
- [ ] Cross-repository cache invalidation after a response/dismiss: use the **notification-provider pattern** (per Pattern 6/Pattern 7 in [docs/client/caching.md](../client/caching.md)) — increment a `feedCacheInvalidationProvider`-style counter that `FeedNotifier` listens to in its `build()`. `FeedNotifier._refreshInPlace()` then re-fetches without showing a loading spinner. **Do not invalidate by guessing namespace patterns** (e.g., `'feed:*'`) — the existing feed pipeline doesn't use a `feed:` cache namespace; it computes feed items on demand. If a `feedCacheInvalidationProvider` doesn't yet exist, add one in this issue (small change, mirrors `dailyCacheInvalidationProvider` and `searchCacheInvalidationProvider`). If `EsmRepository` instead opts to depend directly on a feed repository, depth must remain ≤1 level per the repository composition rule.
- [ ] Note that the optimistic-removal pattern below already gives the user instant feedback; the cache invalidation is the consistency belt-and-suspenders that ensures the card doesn't reappear on the next pull-to-refresh. Server-side filtering on `consumed_action != UNRESOLVED` is the authoritative gate.
- [ ] Provider wiring: `esmRepositoryProvider` (Provider). The view's response handler uses an autoDispose `AsyncNotifier` per the AsyncNotifier-preferred guidance in [docs/client/architecture.md](../client/architecture.md). Async safety: any non-`build()` notifier method that performs an `await` and then writes to `state` must guard with `if (!ref.mounted) return;`.
- [ ] Optimistic dismiss: on tap, the card hides locally before the RPC completes. On RPC failure, log + restore the card with a snackbar offering retry. Standard optimistic-update pattern from [docs/client/architecture.md](../client/architecture.md) Common Patterns section.
- [ ] Tests: golden test for the card in light + dark theme; widget test for response → server call → card removed; widget test for dismiss → server call → card removed; widget test for offline RPC failure → card restored with snackbar; disposal test for the response notifier.

**Validates:** The card lands in real users' feeds, looks like the rest of the feed, and responds correctly to taps and dismisses. Privacy holds because responses go through the server and the host-facing aggregation is the only readback surface.

### Phase 4: Workshop read-side integration and forward-compatibility check

**Goal:** Issue #1579's `WorkshopRepository` consumes `GetESMSignal` and the cascade priority (2) lights up correctly. The v2 (automated) seam is documented so v2 doesn't require contract changes.

**Tasks:**
- [ ] Coordinate with #1579 to wire `GetESMSignal` into `WorkshopRepository.getQuestHeroCascade()`. The cascade priority (2) check is `signal.repeat_count / signal.respondent_count >= 0.6` for any experience completed in the last 7 days (the brief's "recently completed event with strong ESM repeat signal ≥60%").
- [ ] End-to-end manual smoke test on dev: operator authors a prompt against a real completed event, multiple users respond yes, the host opens the Workshop, cascade priority (2) renders the corresponding Quest hero card.
- [ ] Document the v2 forward-compatibility contract in a README at [server/services/esm/README.md](../../server/services/esm/README.md): v2 replaces the manual runbook with an automated trigger that fires on `experience.completed_at_unix_sec` being set. The trigger calls the same `esm.BuildPromptForExperience` helper this issue introduces, with `recipients = esm.AttendeeUserIDs(...)`, and inserts the rows directly. The read API (`GetESMSignal`) and feed dispatch don't change. A frequency cap per recipient across all prompts is the v2 trigger's responsibility, not v1's contract.
- [ ] Add a `server/esm/README.md` (the shared library) per [CLAUDE.md](../../CLAUDE.md)'s rule that every multi-file directory under `server/` has a README covering purpose, key files/types, and when to add code here vs. an adjacent directory.
- [ ] Privacy review: confirm by reading that response data is not exposed via any user-visible API surface — `GetESMSignal` to the event host returns aggregates and free-text-count rather than raw rows; the FeedItem ESM payload describes the *prompt* a user is being asked, never any user's prior response. Inspect both `esm_service.proto` and the `ESMPromptPayload` in `feed_service.proto` for any field that could leak per-user response data.

**Validates:** The closed loop the priorities doc describes — *experience sampling after an event tunes the next nudge in the feed; the next nudge converts the signal into the next instance being scheduled* — works end-to-end. The contract is stable enough for v2 to swap in trigger and targeting without touching read or render code.

## Related Issues

- **#1579** — Workshop tab v1 (proactive host workshop). Consumes `GetESMSignal` server-internally via the nudge generation pipeline for Quest hero cascade priority (2). The two issues share the read-side aggregation contract; this issue owns the producer, #1579 owns the consumer. **This issue's Phase 1 is a hard prerequisite for #1579** — see Sequencing below.
- **Future issue** — v2 automated ESM (out of scope here per the issue body): trigger from `experience.completed_at_unix_sec`, prompt selection from a template bank keyed to event flavor, targeting by `attended == YES`, frequency cap per recipient.
- **Future issue** — multi-question prompts and cross-event prompts (v3 concern; not in scope for v1 or v2).
- **Future issue** — ESM-driven decay-revival surfacing in the Workshop (cascade priority #4 / love-revival items). Lifts off the same aggregation API once enough events have signal.

## Sequencing with #1579 (2026-04-29)

**Decision:** This issue ships first, but only **Phase 1** (proto contract + `esm_service` + `server/esm/` shared library + storage models) is a hard prerequisite for #1579. Everything after Phase 1 can run in parallel with #1579.

**Reasoning:** #1579's cascade priority (2) — the canonical "Schedule round 6" Quest hero — lights up only once ESM signal exists in the system. Sequencing this issue's Phase 1 first means the Workshop launches with that surface working from day one rather than dark. This issue also has independent value: hosts can author prompts and get responses with no Workshop, which de-risks the assumption that signal will arrive before #1579 builds its most expensive surface.

**Staged plan across both issues:**

1. **Step 1 (~1 week):** This issue's **Phase 1** — proto contract, ESM service, shared library, storage models. Unblocks all parallel work below.
2. **Step 2 (~2 weeks, parallel):** This issue's **Phase 2** (manual authoring runbook) **and** #1579's Phases 1–2 (tab plumbing + `StoredNudge.surface` enum + `WorkshopTile` + nudge/story detail screen wraps). #1579's Phase 2 depends only on this issue's contract being defined, not on its RPC being live.
3. **Step 3 (~1.5 weeks, parallel):** This issue's **Phase 3** (ESM client card in feed) **and** #1579's Phase 3 (Crew strip + catalyst-pull review screen + surfacing discipline).
4. **Step 4 (~1.5 weeks, integration):** This issue's **Phase 4** (forward-compat README) **and** #1579's Phase 4 (cascade detectors + `GetWorkshopFeed` RPC; the cascade-(2) detector calls `GetESMSignal` server-internally).

**Fallback if the Workshop is needed sooner:** start #1579's Phase 1–2 in parallel with this issue's Phase 1, accept that cascade priority (2) is initially dark in the Workshop, and light it up later via a server-side detector + config flip. The reconciliation between #1579 and this issue makes the fallback cheap — cascade logic lives server-side in the nudge generation pipeline, so no Workshop client re-work is needed when priority (2) lights up.

---

## Audit Notes (2026-04-29)

The plan was reviewed against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), and [docs/server/architecture.md](../server/architecture.md). Changes made in place:

1. **Cache invalidation mechanism corrected.** The original plan said "After a write, invalidate `'feed:*'` cache keys." The existing feed pipeline doesn't use a `feed:` cache namespace — feed items are computed on demand on the server, and the client uses notification-provider patterns (e.g., `dailyCacheInvalidationProvider`, `searchCacheInvalidationProvider` per Patterns 6 and 7 in [docs/client/caching.md](../client/caching.md)) rather than namespace-pattern invalidation for cross-repository signaling. Plan now uses a `feedCacheInvalidationProvider`-style counter that `FeedNotifier` listens to. Optimistic removal handles the immediate UX; cache invalidation is the consistency belt-and-suspenders.
2. **Repository constructor pattern made explicit.** Original plan named `EsmRepository` but didn't specify its constructor. Added `CacheManager + FeedServiceClient` injection mirroring [GearRepository](../../app/lib/data/repositories/gear_repository.dart), per the architecture doc's repository pattern. Explicit `CacheService(cacheManager, 'esm')` namespace.
3. **Authorization helpers per server convention.** Original plan said "inline check in the RPC handler." Per Service Architecture in [docs/server/architecture.md](../server/architecture.md), service-specific auth checks live in `authorization.go` of the service package. Plan now factors `requireESMRespondent` and `requireESMSignalReader` helpers into a new `server/services/feed/authorization.go`. Cross-service helper sharing (e.g., "host of event" check) goes through `server/auth/` shared library, not service-to-service injection (Service Independence rule).
4. **Aggregation logic factored as pure function.** Added a task to extract `aggregateESMResponses([]StoredESMResponse) []ESMSignal` as a pure function so the threshold arithmetic (≥60% / 30–60% / <30%) is unit-testable in isolation without storage. The RPC handler stays a thin wrapper.
5. **Service-placement decision documented as Open Question.** Added an open question covering whether the new RPCs belong on `feed_service`, `experience_service`, or a new `esm_service`. Recommendation: `feed_service` for v1 (consistent with existing `ConsumeNudge`); flag a future home if Workshop grows more aggregation surfaces.
6. **i18n boundary clarified.** Original plan correctly noted operator-authored content passes through unlocalized but didn't enforce the viewmodel/widget split. Plan now states: viewmodel exposes typed state (response option keys + values), widget resolves chrome via `context.l10n.*` in `build()`. Both ARB files updated before `npm run generate`; CI's `npm run lint:dart:i18n` is the gate.
7. **SQL Efficiency rules applied.** Added explicit `InsertBatch` for the operator CLI's per-recipient row insertion (Phase 2) and called out the indexes needed: `(user_id, consumed_action)` for the feed assembly join, `(experience_id, prompt_id)` for `GetESMSignal`. These match the SQL Efficiency rules in [CLAUDE.md](../../CLAUDE.md) (no N+1 inserts; index what you filter on).

No structural reorganization of phases was needed; the original sequence (proto contract → operator CLI → client card → Workshop integration & forward-compat) holds up against the architecture rules. The aggregation read API contract — the seam between this issue and #1579 — is explicitly testable in isolation now, which de-risks the integration.

## Decisions Log (2026-04-29, second pass)

User-supplied direction collapsed all open questions and made two structural changes (dedicated ESM service, no operator CLI). Plan updated in place:

1. **Eligibility / recipient list** — Trust the operator's recipient list, but the manual-authoring path **prefills with attendees** (`ExperienceRSVP.attended == YES`). Phase 2 adds an `esm.AttendeeUserIDs(...)` helper to make this trivial.
2. **Per-event scope** — `StoredESMPrompt.experience_id` is required; cross-event prompts are deferred to v3.
3. **Free-text responses** — Stored alongside structured responses; visible only to the host of the event in question; never aggregated as a public surface.
4. **Consumption model** — Both `RESPONDED` and `DISMISSED` clear the card; aggregation only counts `RESPONDED` rows.
5. **Recipient deduplication** — The `(user_id, prompt_id)` consumption row is what makes the prompt visible to a recipient; once `consumed_action != UNRESOLVED`, the card stops surfacing.
6. **Aggregation read API** — Per-prompt. `GetESMSignal` returns one `ESMSignal` per `(experience_id, prompt_id)` pair.
7. **No admin tool / no operator CLI (load-bearing change).** Phase 2 was rewritten from "Operator CLI for manual prompt authoring" to "Manual prompt authoring runbook (no committed CLI)." A developer in dev/staging follows a runbook at `docs/runbooks/esm-manual-authoring.md` that uses two new shared-library helpers (`BuildPromptForExperience`, `AttendeeUserIDs`) directly. No `server/cmd/esm-*` binary is committed; no new admin RPC is added. v2's automated trigger is what supersedes the runbook — at which point the runbook is retired.
8. **Dedicated ESM service from day one (load-bearing change).** Phase 1 was rewritten to factor ESM into:
   - A new `proto/ripls/api/esm_service.proto` hosting `RespondToESMPrompt` and `GetESMSignal`.
   - A new service package at `server/services/esm/` (with `service.go`, `responses.go`, `signal.go`, `authorization.go`).
   - A new shared library at `server/esm/` containing storage queries, the pure-function `Aggregate` operation, and the `FeedItemsForUser` helper used during feed assembly.
   - The feed service consumes the shared library — it does **not** depend on the ESM service. Both services depend on the storage layer and the shared library, satisfying Service Independence.
9. **Frequency cap** — Out of scope; v2 trigger's responsibility.

## Reconciliation with Nudges and Stories (2026-04-29, third pass)

User direction (from #1579's planning thread): Workshop suggestions are another form of nudges; we should not duplicate data models. The Workshop reuses the existing nudge infrastructure with a new `surface` enum. This affects how `GetESMSignal` is consumed.

Plan updated:

1. **`GetESMSignal` is still a real RPC** with the same shape, the same authorization (host-only via `requireESMSignalReader`), and the same `option_key = "do_again"` convention. No proto changes.
2. **The primary consumer of `GetESMSignal` is now server-internal.** The Workshop nudge generation pipeline (in `server/services/feed/nudges.go`, extended with a `surface` parameter per #1579's plan) calls `GetESMSignal` for each recently-completed experience in a circle to decide whether to generate a `WORKSHOP_QUEST_HERO` nudge with the cascade-priority-2 framing ("8 of 11 want this again"). The aggregated text is baked into the nudge's `description` at generation time. The Workshop client never reads `ESMSignal` directly — it reads curated `NudgePayload`s.
3. **The host-only auth gate is satisfied by the server-internal call path.** When the nudge generation pipeline calls `GetESMSignal`, it operates in a server context (no viewer impersonation); the resulting `WORKSHOP_QUEST_HERO` nudge is then host-scoped via the existing `community_id` + `user_id` filter on `StoredNudge`. Privacy is preserved end-to-end — non-host users never see the aggregate, and the aggregate text lives only in nudges scoped to the host.
4. **`GetESMSignal` may still be useful as a client-facing RPC** for non-Workshop surfaces (e.g., a future "host details" view that shows the per-prompt response distribution). Keep the RPC public; the host-only auth gate already protects it. The `aggregateESMResponses` pure function from Phase 1 is the same code path either way.
5. **No structural changes to Phase 1, 2, or 3 of this issue.** The ESM service, the storage models, the operator runbook, and the client card all work the same. The only practical difference is that #1579 consumes the aggregation seam from inside the server rather than from the client.
6. **No data-model changes from this reconciliation.** The reuse-nudges decision affects #1579's data model (`StoredNudge` gets a `surface` field) and #1579's client (`WorkshopTile` over `NudgePayload`). #1580's data model (`StoredESMPrompt`, `StoredESMResponse`, `ESMSignal`) is unchanged.
