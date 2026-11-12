## Issue #1202: Apply Experience needs UX to standalone help Requests (owner asks + helper offers)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1202
**Filed:** 2026-04-21 | **Last activity:** 2026-04-21
**Labels:** none | **Assignees:** unassigned
**Context docs:** [docs/experience_needs.md](../experience_needs.md)

## Audit Notes (2026-04-21)

Ran against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), [docs/server/architecture.md](../server/architecture.md). Changes made:

- **Server substrate moved out of `services/`.** Original draft placed the shared helper at `server/services/coordination/`, which violates the Service Independence principle in [server/architecture.md § Core Design Principles](../server/architecture.md) — services must not depend on other services; shared logic belongs in a sibling library package. Relocated to `server/coordination/` (peer of `server/storage`, `server/auth`, `server/chat`).
- **Cache key convention fixed.** Original draft proposed `needs_and_contributions:request:{requestId}`. Per [client/caching.md § Cache Key Naming Convention](../client/caching.md), the namespace is the repository, not a segment inside the key. Using `CacheService(cacheManager, 'request')` + key `needs_and_contributions:{requestId}` keeps Request-scope keys within the Request repository's namespace.
- **Added i18n task to Phase 6.** Per [client/i18n.md](../client/i18n.md), every user-visible string must route through ARB files (`app_en.arb` + `app_es.arb`). New strings ("Add to your ask", "Break it down", "Ways to help", etc.) were previously inline in the plan without flagging the ARB/gen-l10n step.
- **Added structured-logging requirement to Phase 3.** Per [server/architecture.md § Observability](../server/architecture.md), all RPC handlers use `logging.LoggerWithContext(ctx)`. Original draft didn't mention it.
- **Added shared-mock task to Phase 1.** Per [server/architecture.md § Testing Strategy](../server/architecture.md): "Define mocks in the library package (e.g., `storage/mock.go`). Reuse mocks across services." New coordination library gets its own mock.
- **Made Riverpod family provider / AsyncNotifier guidance explicit in Phase 5.** Per [client/architecture.md § Async Safety in Notifiers](../client/architecture.md), a scope-parameterized notifier should use family providers + AsyncNotifier to avoid disposal-during-async crashes.
- **Added disposal tests and theme-aware color requirement.** Per [client/architecture.md § Disposal Testing and § Colors and Theming](../client/architecture.md), both are non-negotiable for new async Notifiers and new UI.

## Post-Review Updates (2026-04-21)

Author reviewed the plan and answered all open questions; two of the answers changed the structure of the plan. Recorded here so the rewritten sections below are traceable:

- **Conversation lifecycle claim was factually wrong.** Verified in [server/services/request/lifecycle.go L93–109](../../server/services/request/lifecycle.go#L93-L109): `SubmitRequest` calls `chat.CreateOrGetConversation()` immediately and persists the `conversation_id` on the Request. `OfferToFulfill` only *adds participants* to an already-existing conversation. The "lazy conversation creation" phase task is removed — every Request has a conversation from submit time, and all need/contribution messages post into it.
- **Proto strategy flipped from Option B → Option A.** The author confirmed Experience Needs has not actually launched to users yet, even though the docs label it "production-ready." That removes the main blocker against merging. The plan now merges `ExperienceNeed` + the planned `RequestNeed` into a unified `CoordinationNeed` with a `oneof scope { experience_id, request_id }`, and likewise for `CoordinationContribution`. One storage table, one storage model, one library, two RPC surfaces (one per scope, per proto conventions).
- **Owner CAN add freestanding offers on their own Request.** The author's rationale: sometimes a requester ends up doing the work themselves, and that should be recordable as a contribution. The permission table below reflects this.
- **Claim auto-creates a `RequestOffer`** (confirmed).
- **Suggestions are per-Request**, not per-viewer (confirmed).
- **Sub-needs inherit the Request's `shared_community_ids`** (confirmed).
- **Terminal states (`FULFILLED` / `CANCELLED`) block all mutations** (confirmed).
- **Request content view gets three tabs**: "Request" / "Plan" / "Discuss", mirroring [experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart) (which uses tab labels "Event" / "Details" / "Chat" via [ContentViewTabBar](../../app/lib/presentation/widgets/content_view_tab_bar.dart#L75-L83)). The Plan tab swaps to a results pane (with impact metrics) only when the Request is in a completed state — mirroring the Experience `_buildResultsPane()` vs `_buildDetailsPane()` split at [experience_content_view.dart L674–679](../../app/lib/presentation/screens/experience/experience_content_view.dart#L674-L679). Today the Request content view shows metrics on the second tab regardless of state; that has to move.

## Problem Statement

The Experience Plan tab gives participants a guided, chip-driven way to coordinate ("bring the napkins, the truck, the card table") with atomic slot claiming, unified modal chrome, and chat pill integration. Standalone help Requests have none of that machinery — a requester posts a single title + description and everything else happens in freeform chat.

The issue proposes porting the Plan-tab UX to a help Request with permissions inverted for the asymmetric nature of the domain: the requester is the only one who can add sub-needs (both "things that round out the ask" and "breakdown pieces that make the ask more claimable"), and only non-requesters can claim a slot or add a freestanding offer.

A critical constraint the issue calls out: **maximize reuse from the Experience implementation**. Two parallel forks of this UX would drift — the goal is one substrate, two scopes.

## Context from Comments

No comments yet — this is a fresh issue filed by the maintainer who also wrote the Experience Needs spec. No disagreements or revisions to the design. The lack of discussion isn't a red flag here: the proposal is internally consistent with the already-shipped Experience Needs spec and the author is the same person.

## Potential Solutions

### Option A — Generalize: merge types into a single scope-polymorphic entity
Rename `ExperienceNeed`/`ExperienceContribution` to `CoordinationNeed`/`Contribution` with a `oneof scope { experience_id, request_id }`. One storage table (`coordination_need`, `contribution`), one server package, one client repository/viewmodel/widget.

- **Approach:** Data migration for the two existing Experience tables; refactor every Experience call site; add scope-switch branching where permission checks diverge.
- **Pros:** Truly DRY. Zero drift risk as UX evolves. One test surface. The Appendix in `docs/experience_needs.md` explicitly rejects bolting onto the standalone Request system — but this direction is the inverse: lift the coordination layer out of Experience and share it.
- **Cons:** Large, risky refactor. Rewrites live proto fields, live storage tables, live cache keys, live chat-pill payloads in a currently production-ready feature. Migration is non-trivial. Any regression breaks Experience Plan tab.
- **Effort:** Large.

### Option B — Parallel scopes on a shared substrate
Add `RequestNeed`/`RequestContribution` protos and storage tables that mirror the Experience pair. Extract the reusable server logic (atomic slot helpers already shared in `server/storage/protosql.go`; batch user enrichment; chat writer glue) and the reusable client components (needs section widget, sheets, actions handler, viewmodel) into a generic substrate parameterized by scope. Route Experience and Request through the same substrate while keeping the on-wire proto messages distinct.

- **Approach:** Keep Experience storage/protos exactly as-is. Add parallel Request-scope types. Factor client widgets/viewmodels behind an interface that abstracts "which repository, which RPC surface, which permission rule." Factor server permission rules behind a small interface, reuse everything else directly.
- **Pros:** Zero risk to the shipped Experience flow. No data migration. Smaller blast radius. The Go server helpers that matter (`DecrementFieldIfPositive`, `IncrementField`, `SystemMessageWriter`, `FetchAPIUsersBatch`) are already generic enough to reuse directly without refactor. Client widget extraction is straightforward because the sheets already operate on generic "need" / "contribution" response objects.
- **Cons:** Two proto pairs, two tables, two RPC surfaces. Proto-to-response conversion code is duplicated per scope. As the UX evolves, keeping the two in sync is a discipline problem (mitigated by pulling shared chrome into shared widgets).
- **Effort:** Medium.

### Option C — Bolt coordination fields onto `Request`/`RequestOffer` directly
Add `parent_request_id` to `Request` so breakdowns become child Requests. Add slot counting to `Request`. Add a suggestion chip strip to the request detail screen as a one-off.

- **Approach:** No shared substrate. No generic needs section. Request screen grows its own claim/offer UI inline.
- **Pros:** Smallest proto change.
- **Cons:** Directly contradicts the Appendix in `docs/experience_needs.md`, which lists the concrete reasons the standalone Request system was rejected as a reuse target for Experience needs — the same reasons apply here in reverse. Pollutes `Request` with slot semantics. Two parallel UX implementations that will drift. Most work and worst DRY.
- **Effort:** Medium (deceptively — the parallel implementation is the expensive part).

## Recommendation

**Option A — updated by author review.** The original draft chose Option B on the basis that Experience Needs was shipped and production-ready; the author confirmed it has not actually launched to users yet, which removes the "don't risk the live flow" blocker that was the main argument against A.

With that constraint gone, Option A is strictly better:

- One storage table (`coordination_need`) with `oneof scope { experience_id, request_id }` instead of two parallel tables.
- One proto-level domain model (`CoordinationNeed`, `CoordinationContribution`) instead of two.
- One `server/coordination/` library that owns all the shared logic (atomic slot helpers, chat integration, batch user enrichment, permission helpers).
- Two *RPC surfaces* — Experience-scoped RPCs and Request-scoped RPCs — each with dedicated request/response types per [docs/proto_conventions.md](../proto_conventions.md). The surfaces differ only in permission rules and the scope token they pass into the library.
- One Flutter widget tree (`NeedsSection`, `NeedsActions`, `NeedsNotifier`) parameterized by scope, with the Experience Plan tab and the new Request Plan tab both consuming it.

Option C remains eliminated by the architectural argument already codified in the Experience spec (the Appendix in [docs/experience_needs.md](../experience_needs.md)).

## Resolved Decisions

All questions from the initial plan have been answered by the author. The decisions below are authoritative for the implementation:

- **Conversation lifecycle.** Not an issue: every Request already has a `conversation_id` from the moment `SubmitRequest` completes (verified in [server/services/request/lifecycle.go L93–109](../../server/services/request/lifecycle.go#L93-L109)). All need/contribution chat messages post to that existing conversation. No lazy-create path is required.

- **Suggestions are per-Request**, not per-viewer. Three suggestion buckets are stored on the Request: `additional_asks`, `breakdown_pieces`, `offer_ideas`. The client renders the right ones for the viewer's role.

- **Unified proto types.** `ExperienceNeed` + the planned `RequestNeed` merge into `CoordinationNeed` with a `oneof scope { string experience_id; string request_id; }`. Same for `CoordinationContribution`. One storage table per type.

- **Visibility inherits from the parent.** Sub-needs and contributions on a Request are visible wherever the parent Request is visible (follows `shared_community_ids`). Likewise Experience coordination inherits Experience visibility. No new sharing semantics.

- **Owner can add freestanding offers on their own Request.** Sometimes the requester ends up doing the work themselves and that should be recordable as a contribution. The permission table is therefore more permissive than the original proposal: the only owner-exclusive action is adding/removing needs; contributions are open to everyone (including the owner), with edit/remove gated to the original contributor.

- **Claiming auto-creates a `RequestOffer`** so the helper shows up in `GetRequestPeople` and is added to the Request's conversation.

- **Terminal states block all mutations.** `FULFILLED` and `CANCELLED` are hard guards, mirroring the Experience `COMPLETED` / `CANCELLED` pattern.

- **Request content view restructures into three tabs** mirroring [experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart): **Request** (the equivalent of Experience's "Event" tab — summary + media + people), **Plan** (coordination UI; becomes the results/metrics pane only once the Request reaches a completed state), **Discuss** (the existing conversation). Metrics currently live on the second tab unconditionally; they have to move to the completed-state branch so active Requests show needs/contributions on the Plan tab.

### Permission Table (authoritative)

| Action | Experience rule | Request rule |
|---|---|---|
| Add / remove need | Any participant (owner or YES/MAYBE RSVP) | Requester only |
| Claim a need (takes a slot) | Any participant | Anyone (owner included) |
| Unclaim / release a slot | Original contributor | Original contributor |
| Add freestanding contribution | Any participant | Anyone (owner included) |
| Edit / remove contribution | Original contributor | Original contributor |
| Any mutation in terminal state | Blocked (`COMPLETED` / `CANCELLED`) | Blocked (`FULFILLED` / `CANCELLED`) |

## Implementation Plan

### Phase 1 — Unify coordination model and extract the shared library
**Goal:** merge Experience and Request coordination into one proto model and one storage table, served by a new `server/coordination/` library. The Experience service is refactored to call through the library; no behavior change visible to Experience users.
**Tasks:**
- [ ] Create `server/coordination/` (sibling of `server/storage`, `server/auth`, `server/chat` — **not** under `server/services/`, per Service Independence in [server/architecture.md](../server/architecture.md)) with `permission.go`, `chat.go`, `list.go`, `slots.go`, `mock.go`.
- [ ] Create `proto/ripls/models/coordination.proto` with `CoordinationNeed` and `CoordinationContribution` messages. Each carries a `oneof scope { string experience_id = N; string request_id = N+1; }` replacing the current `experience_id` field. Remaining fields mirror the current `ExperienceNeed` / `ExperienceContribution` shape.
- [ ] Delete `ExperienceNeed` and `ExperienceContribution` from [proto/ripls/models/experience.proto](../../proto/ripls/models/experience.proto). Since the Experience flow has not launched to users, this is a straight rename-and-restructure with no wire-compat obligations.
- [ ] Rename storage tables `experience_need` → `coordination_need`, `experience_contribution` → `coordination_contribution`. Because ProtoSQL autogenerates schema from proto definitions, this follows from the proto change — no hand-written migration required beyond dropping the old tables in dev/local environments.
- [ ] Regenerate code via `npm run generate`.
- [ ] Library APIs take a `Scope` argument (`coordination.Scope{ExperienceID, RequestID}`) and expose: `ClaimSlot`, `ReleaseSlot`, `PostAddNeed`, `PostRemoveNeed`, `PostClaim`, `PostAddContribution`, `PostRemoveContribution`, `EnrichNeedsAndContributions`, plus permission helpers `LoadActiveScope` / `RequireActor`.
- [ ] Refactor the Experience service's RPC handlers (`services/experience/needs.go`, `contributions.go`, `needs_and_contributions.go`) to call through the library, passing an `ExperienceID`-bearing `Scope`.
- [ ] Update Experience API protos and Go handlers wherever they reference `ExperienceNeed` / `ExperienceContribution` to use the new coordination types; responses still use Experience-scoped API message names per [docs/proto_conventions.md](../proto_conventions.md).
- [ ] Update Experience service tests to use the shared `coordination/mock.go`.
**Validates:** existing Experience test suite still passes; one storage table backs both scopes; Experience service depends only on the coordination library, not on anything else.

### Phase 2 — Request-scope RPCs
**Goal:** the Request service exposes the full coordination surface with Request-specific permission rules, backed by the shared library and the unified storage.
**Tasks:**
- [ ] Add to `proto/ripls/api/request_service.proto`: `AddRequestNeed`, `RemoveRequestNeed`, `BatchAddRequestNeeds`, `ClaimRequestNeed`, `UnclaimRequestNeed`, `AddRequestContribution`, `EditRequestContribution`, `RemoveRequestContribution`, `ListRequestNeedsAndContributions` — each with its own dedicated request/response types per [docs/proto_conventions.md](../proto_conventions.md).
- [ ] Implement handlers in `server/services/request/needs.go` and `contributions.go`, calling through `server/coordination` with `Scope{RequestID: req.Msg.RequestId}`. The Request service depends only on the library — not on the Experience service.
- [ ] Every handler uses `logging.LoggerWithContext(ctx)` with structured fields (`operation`, `request_id`, `user_id`, `need_id` / `contribution_id` as applicable) per [server/architecture.md § Observability](../server/architecture.md).
- [ ] Permission rules (matches the authoritative table above):
  - Add / remove need → requester only.
  - Claim / unclaim → anyone (owner included).
  - Add / edit / remove contribution → anyone can add; edit/remove restricted to the original contributor.
  - All mutations guarded by terminal-state check (`FULFILLED` / `CANCELLED` block).
- [ ] Chat messages post to the existing `Request.conversation_id` — **no lazy-create needed**; the conversation is set by `SubmitRequest` at the moment of creation (verified in [server/services/request/lifecycle.go L93–109](../../server/services/request/lifecycle.go#L93-L109)).
- [ ] Claiming a Request need auto-creates a `RequestOffer` for the claimer if one does not exist yet, so the helper appears in `GetRequestPeople` and is added as a conversation participant via the existing path.
- [ ] Round-trip tests for every new RPC; N+1 guard via `storage.AssertMaxQueries` on the list handler.
- [ ] Permission-boundary tests: non-requester cannot add a need; non-contributor cannot edit someone else's contribution; terminal-state mutations rejected. Specifically assert that the owner CAN add a freestanding contribution on their own Request.
- [ ] Unit tests use the shared `server/coordination/mock.go`; integration tests use `storage.SetupTestStorage(t)` (testcontainers + pgvector).
**Validates:** the Request service handles the full UX surface server-side, with the permission table enforced by tests.

### Phase 3 — AI suggestion prompts for Requests
**Goal:** the server generates owner-side "additional asks" + "breakdown pieces" and helper-side "offer ideas" suggestions for a Request.
**Tasks:**
- [ ] Add to the `ai.Provider` interface: `GenerateRequestSuggestions(ctx, title, description, location) (*RequestSuggestionResult, error)`. The result carries three lists: `additional_asks`, `breakdown_pieces`, `offer_ideas`.
- [ ] Implement in `server/ai/provider_openai.go` (and the fallback / loadbalanced providers) with new prompts.
- [ ] Persist on `Request` as three separate `repeated string` fields — wire-compatible additions.
- [ ] Generation path: same as Experience — best-effort, warn-on-failure, not blocking `SubmitRequest`. Generation skipped for the simulated clock, matching the Experience gate.
- [ ] Lazy-fill on `ListRequestNeedsAndContributions` for Requests whose suggestions are empty (mirrors the pattern being added by #1194).
**Validates:** a fresh Request has all three chip strips populated within the first open of its detail view.

### Phase 4 — Extract shared client widget substrate
**Goal:** the Flutter needs section, sheets, actions handler, and viewmodel are parameterized by scope. Experience UI still works identically.
**Tasks:**
- [ ] Define a `NeedsScope` sealed class (Freezed per [client/architecture.md](../client/architecture.md)) with variants `NeedsScope.experience(experienceId)` and `NeedsScope.request(requestId)`. Each variant carries the role-resolution and permission-check methods for its scope.
- [ ] Generalize `ExperienceNeedsSection` → `NeedsSection`, taking a `NeedsScope`.
- [ ] Generalize `ExperienceNeedsActions` → `NeedsActions`, calling through the scope's repository.
- [ ] Generalize `ExperienceNeedsNotifier` → `NeedsNotifier`. Use a **Riverpod family provider** keyed on `NeedsScope` (per [client/architecture.md § Recommended Improvements](../client/architecture.md)), and migrate to **`AsyncNotifier`** — `build()` handles disposal automatically and `AsyncValue.when()` cleans up the widget. Mutation methods and `refresh()` that set state manually must guard with `if (!ref.mounted) return;` per [client/architecture.md § Async Safety in Notifiers](../client/architecture.md).
- [ ] Sheets (`ViewNeedSheet`, `ViewContributionSheet`, `AddContributionSheet`, batch sheets) already operate on generic response objects — verify they don't reach back into Experience-specific types; extract where needed. All colors use `AppColors.xxx(context)` helpers (no hardcoded `Colors.white`, `Colors.grey[N]`, `Color(0xFF...)`) per [client/architecture.md § Colors and Theming](../client/architecture.md).
- [ ] `ExperienceRepository` keeps its existing needs/contributions methods but now talks to the unified storage. Add the Request-scope surface on `RequestRepository` (not a new repository) via `CacheService(cacheManager, 'request')` with keys `needs_and_contributions:{requestId}` — namespace owned by the repository per [client/caching.md § Cache Key Naming Convention](../client/caching.md).
- [ ] Repository mutations invalidate the relevant cache key before returning (force-refresh pattern per [client/caching.md § Cache Invalidation](../client/caching.md)).
- [ ] Keep repository dependencies shallow (max 1 level) — `RequestRepository` may depend on `UserRepository` / `MediaRepository` but not on `ExperienceRepository`.
- [ ] Add disposal tests for `NeedsNotifier` per [client/architecture.md § Disposal Testing](../client/architecture.md).
- [ ] Refactor the Experience Plan tab to instantiate `NeedsSection` with `NeedsScope.experience(...)`. Verify no regression via the existing Experience widget tests.
**Validates:** the Experience Plan tab and a scaffolded Request Plan tab both render from the same widgets; disposal tests pass.

### Phase 5 — Restructure the Request content view into three tabs
**Goal:** [request_content_view.dart](../../app/lib/presentation/screens/request/request_content_view.dart) mirrors [experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart) — three tabs, with coordination on the middle tab and metrics deferred to a completed state.
**Tasks:**
- [ ] Restructure to use [ContentViewTabBar](../../app/lib/presentation/widgets/content_view_tab_bar.dart) (or the same component used by Experience) with three tabs: **Request** / **Plan** / **Discuss**. The first tab shows what the Request tab shows today (summary, media, people); the third tab wraps the existing `InlineConversationView`.
- [ ] Implement the Plan tab with a state split matching Experience's `_buildResultsPane()` vs `_buildDetailsPane()` ([experience_content_view.dart L674–679](../../app/lib/presentation/screens/experience/experience_content_view.dart#L674-L679)):
  - Active Request (`ACTIVE` / `OFFERS_RECEIVED`): render `NeedsSection` with the Request scope. No metrics.
  - Completed Request (`FULFILLED`): render the existing metrics / impact tiles pane. `NeedsSection` renders read-only below.
  - `CANCELLED`: read-only summary, no mutations.
- [ ] Move impact metrics off the unconditional tab — today they render regardless of state. The metrics pane only appears on the Plan tab when the Request is in a completed state, matching Experience.
- [ ] Owner viewing an active Request sees two chip strips on the Plan tab: "Add to your ask" (`additional_asks`) and "Break it down" (`breakdown_pieces`). Tap → existing batch-add sheet pre-filled.
- [ ] Non-owner (and owner, if they choose) viewing an active Request sees one chip strip: "Ways to help" (`offer_ideas`). Tap → existing `AddContributionSheet` pre-filled.
- [ ] Chat pills in the Discuss tab route to the same `NeedsActions` as the Plan tab.
- [ ] **i18n:** all new strings go through ARB files per [client/i18n.md](../client/i18n.md). Add keys to `app_en.arb` (with `@key` metadata) and `app_es.arb`, consume via `context.l10n.*`. Suggested keys: `requestTabLabel`, `requestTabPlanLabel`, `requestTabDiscussLabel`, `requestNeedsAddToAsk`, `requestNeedsBreakItDown`, `requestNeedsWaysToHelp`, `requestNeedsStillNeeded`, `requestNeedsWhosOffered`, plus empty-state and terminal-state strings. Regenerate via `npm run generate`.
- [ ] Viewmodel exposes typed state (enums for tab + viewer role, not localized strings); widgets resolve to localized text in `build()` per [client/i18n.md § Best Practices](../client/i18n.md).
- [ ] All new UI uses `AppColors.xxx(context)`.
- [ ] Add widget tests covering: tab switching, metrics hidden on active Request, metrics shown on completed Request, chip-strip visibility per viewer role, read-only rendering on `CANCELLED`.
**Validates:** end-to-end in both light and dark mode, and in both `en` and `es` locales: owner adds a breakdown piece → helper sees it and claims it → both see the Plan-tab list update + chat messages post in the Discuss tab. Metrics appear only when the Request is marked fulfilled.

### Phase 6 — Docs and cleanup
**Goal:** the unified coordination system is documented as a single feature, not two parallel features.
**Tasks:**
- [ ] Rename [docs/experience_needs.md](../experience_needs.md) → `docs/coordination.md`. Rewrite to describe the shared system, with Experience and Request as two scopes sharing the same substrate. Include the authoritative permission table and the three-tab content view pattern.
- [ ] Delete the Appendix "Why Not Reuse the Request/Offer System?" — the rationale is no longer relevant now that coordination is its own first-class concept distinct from standalone `Request` / `RequestOffer`.
- [ ] Remove any Experience-only code paths left behind by the Phase 1 consolidation.
- [ ] Close #1202 once the acceptance criteria are verified end-to-end in the app.
**Validates:** a contributor reading `docs/coordination.md` understands both scopes in one place.

## Related Issues

- #1194 — Auto-generate Plan tab suggestions for experiences that are missing them. The lazy-on-read generation pattern there applies verbatim to Request suggestions in Phase 4.
- #1196 — Batch mode for Experience Requests and Offers. The batch-add sheet reused in Phase 6 is the one that ships in #1196.
- #1193 — Rethink plan tab UX: surface request/offer descriptions and reduce chat system message noise. Any UX polish that lands there automatically applies to the Request flow once the substrate is shared.
