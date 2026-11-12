# Issue #1637: Community-wide chat: add Discuss tab to community + pin community chat to inbox in single-community context

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1637
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Today every conversation in Ripls is scoped to a leaf entity — a `Transfer`, `Request`, `Experience`, or `Gear` — via the `oneof topic_id` in [proto/ripls/models/conversation_topic.proto](../../proto/ripls/models/conversation_topic.proto). There is no surface for a community-wide discussion. Members of a community who want to talk about something that isn't tied to a specific item or event have nowhere in-app to do it.

This issue asks for two changes:

1. The community content view ([app/lib/presentation/screens/communities/community_content_view.dart](../../app/lib/presentation/screens/communities/community_content_view.dart)) should expose a **Discuss** tab, mirroring the pattern in [gear_content_view.dart](../../app/lib/presentation/screens/gear/gear_content_view.dart) and [experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart). Each community gets its own associated conversation.
2. To make the community-wide chat a one-tap destination, when the user is scoped to a single community (i.e. `selectedCommunityProvider.selectedCommunityId != null`, distinct from the All Circles aggregate added by #1629), the inbox ([portfolio_inbox_screen.dart](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart)) should pin a card for that community's chat at the top of the list.

There is one piece of latent tension in the current code that this issue forces us to resolve: `ConversationTopic` was deliberately designed in #289 to be the single source of truth for *what a conversation is about*, but its oneof currently only enumerates leaf entities. Adding `community_id` as a topic variant is the natural extension — but it's the first non-leaf topic, and it surfaces some downstream questions (notification routing, undo classification, lifecycle) that don't have to be answered for entity-scoped conversations.

## Context from Comments

The issue has no comments yet — it was filed today. The "Open questions" block in the issue body itself flags four unresolved areas (lifecycle, membership sync, notification defaults, posting permissions); this plan offers a recommendation on each.

There is also no precedent in the issue tracker for community-scoped conversations. The closest prior art is #289 (top-level `ConversationTopic` proto type) and #1629 (single-community / All Circles selection state) — both relevant prerequisites that have shipped or partially shipped, see Related Issues.

## Current State Snapshot

Useful baseline drawn from research, since the plan depends on what's already in place:

- **Shared chat widget:** [InlineConversationView](../../app/lib/presentation/widgets/content/inline_conversation_view.dart) is the reusable Discuss-tab body across gear and experience views. It takes a `conversationId`, an `isActive` flag, an `accentColor`, and a `ConversationContext`. The community Discuss tab can adopt it as-is.
- **Conversation viewmodel:** [conversation_view_model.dart](../../app/lib/presentation/viewmodels/conversation_view_model.dart) is a `Notifier` family parameterized by conversation ID. It does not need to be re-architected for community chat.
- **`ConversationTopic` proto:** [conversation_topic.proto](../../proto/ripls/models/conversation_topic.proto) defines the topic oneof in both `api` and `models` packages with reserved fields and a clean extension surface. There is currently no `community_id` variant.
- **`ConversationItem` already has `community_id`:** [chat_service.proto](../../proto/ripls/api/chat_service.proto) line 338+ has `string community_id = 6` as **conversation metadata** (which community the conversation lives within), distinct from the topic. This is important — we are *not* repurposing that field; we are adding `community_id` as a topic variant.
- **Community content view today:** Single-view, no tab bar. Adding a Discuss tab is structurally a bigger lift than for gear/experience, which already had multi-tab layouts.
- **Inbox today:** Renders `DailyItem` entries (transfer / giveaway / experience / request). No pinning mechanism exists. Inbox items are server-driven; a pinned community chat would be a client-side decoration above the list.
- **Selection state:** [selectedCommunityProvider](../../app/lib/services/providers/community_providers.dart) exposes `selectedCommunityId` (nullable; null = All Circles). The "single community context" flag is `state.selectedCommunityId != null`.
- **Notifications:** `NOTIFICATION_CATEGORY_CHATS` exists in [server/community/notifications.go](../../server/community/notifications.go), but no `CommunityEventType` currently maps to it — chat notifications today appear to flow via per-conversation unread-count polling, not events.
- **Undo:** Per [server/undo/registry.go](../../server/undo/registry.go), every new `CommunityEventType` must be classified before merge or `TestEveryEventTypeClassified` fails the build.

## Potential Solutions

### Option A: New `community_id` variant in `ConversationTopic` (recommended)

- **Approach:** Add `string community_id = 10;` (next free field number, mirrored across the api and models copies) to the `oneof topic_id` in `ConversationTopic`. Each `Community` gets exactly one conversation, identified by `topic.community_id == community.id`. Lookup uses a new `GetConversationForCommunity(community_id)` RPC, paralleling the existing `GetConversationForTransfer/Request/Experience` getters. Conversation membership is the community's membership; `participants` is computed from `community_users`.
- **Pros:**
  - Builds on the abstraction #289 was explicitly designed for.
  - Reuses every downstream piece — `ChatService`, `InlineConversationView`, `ConversationViewModel`, message storage, read-receipts, message streaming — without forking the chat stack.
  - Adding a new oneof field is wire-compatible with stored protos (no migration of existing rows).
  - The Dart helper sites that already switch on `topic.whichTopicId()` (~50 of them per the #289 audit) get one new case each — predictable, mechanical work.
- **Cons:**
  - First non-leaf-entity topic; forces decisions on notification routing and undo classification that don't have prior community-chat parallels.
  - Touch surface includes every switch over `ConversationTopic` in Go and Dart; mostly mechanical but easy to miss a site.
- **Effort:** Medium.

### Option B: Separate `CommunityChatService` with its own conversation type

- **Approach:** Treat community-wide chat as a new feature stack. New proto types (`CommunityChat`, `CommunityChatMessage`), new RPCs (`PostCommunityMessage`, `StreamCommunityMessages`), new server service, new Flutter viewmodel.
- **Pros:** Clean isolation; no risk of regressions in existing entity-scoped chats.
- **Cons:** Massive duplication of every feature the existing chat stack already supports — message reactions, read-receipts, attachments, mentions, undo, notifications, message streaming reconnect. Doubles the maintenance surface for chat permanently. Inbox/Discuss UI would have to branch on which kind of chat it's rendering.
- **Effort:** Large, and the wrong long-term shape.

### Option C: Synthetic "community" topic via existing leaf types

- **Approach:** Don't extend `ConversationTopic`. Instead, mark community conversations with a dedicated convention — e.g., a sentinel `community_id` set in `ConversationItem.community_id` with `topic` left empty.
- **Pros:** No proto schema change.
- **Cons:** Conflates two distinct semantic fields (`community_id` is conversation metadata; topic is *what the conversation is about*). Every code path that switches on topic now has to handle "topic is empty, fall back to community_id". Defeats the abstraction #289 created.
- **Effort:** Small to write, but creates a long tail of bugs from the implicit-topic convention.

## Recommendation

**Option A.** Adding a `community_id` variant to `ConversationTopic` is exactly the extension the type was designed to support, and it lets the entire existing chat stack (server service, storage, streaming, Flutter viewmodel, `InlineConversationView`) be reused without forking. The downstream decisions Option A forces on us (notifications, undo, lifecycle) are decisions we'd have to make anyway — Option B and Option C just defer them.

Two specific design choices inside Option A:

- **Lifecycle: eager.** Create the community conversation transactionally when the community is created. Backfill existing communities via a one-shot startup job (same pattern as #289's migration). Rationale: avoids first-write race conditions, makes the inbox pin always have something to point at, and matches the server-side ergonomics of "this entity has a conversation, period."
- **Inbox pin: client-side decoration, not a `DailyItem`.** Render the pin as a dedicated widget slot above the `DailyItem` list when `selectedCommunityProvider.selectedCommunityId != null`, hydrated by `GetConversationForCommunity`. Don't introduce a new `DailyItemType` — `DailyItem`s are server-driven and ephemeral (current week scope), and the community chat pin is durable and client-derived.

## Resolved Decisions

The original "Open Questions" section has been resolved by accepting every recommendation. The decisions are recorded here as the binding spec for the implementation phases below; if any of these need to be revisited, update this section first and then propagate the change into the affected phase.

- **Field number for `community_id` in `ConversationTopic`:** field number **10** (next free) in both the api and models packages. No reserved range for non-entity topics — the topic enum is small enough that we don't need that complexity.
- **Lifecycle / backfill:** **eager creation**, transactional with the community insert (per the architectural choice in Phase 1). Existing communities are backfilled by a **one-shot startup job** at `server/jobs/community_conversation_backfill.go`, idempotent (skip communities that already have a conversation), marked `// TODO(#1637): Delete after backfill has run in production`. Same shape as the #289 migration script.
- **Membership sync:** **compute participants on the fly from `community_users`** — do not materialize them into `ChatConversation.participant_ids`. Materializing duplicates the source of truth and forces every `JoinCommunity` / `LeaveCommunity` to fan out a write to the conversation row. The compute-on-fly approach reuses the existing `validateAndCleanParticipants` pattern in [server/services/chat/conversations.go](../../server/services/chat/conversations.go).
- **Notification defaults:** **opt-out**, gated by the existing `NotifyChats` preference in `CommunityNotificationPreferences`. Per-user / per-community override remains available (and #1107 is the escape hatch for noisy communities).
- **Posting permissions:** **all community members can post.** No admin-only gate in the MVP. Membership is the only check.
- **Notification routing path:** **reuse the existing per-conversation unread-count path** that gear/experience chat already uses. Do **not** add a new `CommunityEventType` enum value or undo-registry entry in this issue. If a post-launch product need surfaces for distinct push payload routing on community chat (e.g. a custom title/body), add the event type then — but the MVP ships without it.
- **Undo classification:** moot under the routing decision above (no new event type → no new registry entry needed). If a future change adds a `COMMUNITY_EVENT_TYPE_COMMUNITY_CHAT_MESSAGE_SENT` enum value, classify it as **`ClassificationClientOnly`** with rationale "users delete individual messages via the chat UI; no server-side retraction RPC needed." Carry this forward verbatim if the situation changes.
- **Removal semantics:** when a user leaves the community, their access to the conversation **revokes immediately on next `StreamMessages` open**. Cached client-side history stays locally on their device until normal cache eviction; they cannot load older messages or send new ones. Falls naturally out of the compute-on-fly participant decision.
- **Tab restructuring on `community_content_view.dart`:** **use the shared tab infrastructure** (`content_view_tab_bar.dart` + `content_view_pane_switcher.dart`). No one-off tab plumbing.
- **Cross-service coupling for transactional conversation creation (resolves the audit's Phase 1 "Architectural choice"):** **Option A — shared library at `server/chat/`** (separate from `server/services/chat/`). Both `CommunityService` and `ChatService` depend on it. `CreateCommunity` calls `chat.CreateCommunityConversation(ctx, tx, communityID)` inside the same transaction as the community insert, preserving the "every community has a conversation, period" invariant. Drop to job-based fan-out (Option B) only if a concrete dependency-graph problem prevents the new import direction during implementation.

## Implementation Plan

### Phase 1: Proto + storage (server foundation)

**Goal:** Extend `ConversationTopic` so a community can be a topic, and ensure every community has exactly one conversation.

**Tasks:**
- [ ] Add `string community_id = 10;` to the `oneof topic_id` in both [proto/ripls/models/conversation_topic.proto](../../proto/ripls/models/conversation_topic.proto) and [proto/ripls/api/conversation_topic.proto](../../proto/ripls/api/conversation_topic.proto). Keep field numbers identical across the two packages (matches the #289 convention).
- [ ] Add `GetConversationForCommunity(GetConversationForCommunityRequest) returns (GetConversationForCommunityResponse)` RPC to [chat_service.proto](../../proto/ripls/api/chat_service.proto). Per project rules, dedicated request/response messages — do not reuse existing types.
- [ ] **Service boundaries (per Resolved Decisions):** factor conversation creation into a shared library at `server/chat/` (separate from `server/services/chat/`) that both `CommunityService` and `ChatService` depend on. Implement `chat.CreateCommunityConversation(ctx, tx, communityID)` there and call it from the `CreateCommunity` handler **inside the same transaction** as the community insert, preserving the "every community has a conversation, period" invariant. Per [docs/server/architecture.md](../../docs/server/architecture.md) §"Service Independence", do not introduce a `CommunityService → ChatService` direct call. (If implementation surfaces a concrete dependency-graph problem with this import direction, fall back to the job-based fan-out described in Resolved Decisions and update this plan accordingly.)
- [ ] Implement the `GetConversationForCommunity` handler in [server/services/chat/conversations.go](../../server/services/chat/conversations.go). Auth: extract caller via `auth.UserFromContext(ctx)`. Membership check: caller must be in `community_users` for the requested community. On unexpected errors, **use `connecterr.Internal(ctx, "GetConversationForCommunity", err, "community_id", communityID)`** — never `connect.NewError(connect.CodeInternal, err)` (gated by `scripts/check_codeinternal.js`).
- [ ] Update `validateAndCleanParticipants` ([server/services/chat/conversations.go:30-94](../../server/services/chat/conversations.go)) to handle topic variant `community_id`: participants come from `community_users` for that community.
- [ ] Backfill job: `server/jobs/community_conversation_backfill.go` — idempotent, iterates communities, creates conversations for any that don't yet have one. Wire as a startup job in `main.go`. Mark with `// TODO(#1637): Delete after backfill has run in production`. Any goroutines spawned by the job must use `logging.GoSafe(ctx, "community-conversation-backfill", fn)` per [docs/server/architecture.md](../../docs/server/architecture.md) §"Goroutine Safety" — `server/cmd/check-goroutines` will fail the build otherwise.
- [ ] **Structured logging** in every new handler and the backfill job: pull `logger := logging.LoggerWithContext(ctx).With("operation", "...", "community_id", communityID)` and emit at INFO for normal flow, ERROR only for system failures (per [docs/server/observability.md](../../docs/server/observability.md) §"Standard Field Names" / §"Appropriate Log Levels"). Member-not-found and not-in-community paths are INFO/WARN, not ERROR.
- [ ] Unit tests covering: create-community-creates-conversation; GetConversationForCommunity for member / non-member; backfill idempotency; participant resolution from community_users. Use table-driven tests per [docs/server/architecture.md](../../docs/server/architecture.md) §"Unit Tests".
- [ ] `npm run generate && npm run build` clean (runs `golangci-lint` + `check_codeinternal.js` + `check-goroutines`).

**Validates:** Every community has a conversation, server can fetch it, non-members are denied. No client work yet.

### Phase 2: Go switch-site sweep

**Goal:** Every place in the Go server that switches on `ConversationTopic` handles the new variant.

**Tasks:**
- [ ] Audit Go code for switches on `topic.GetTopicId()` / `WhichTopicId`. Add the `*models.ConversationTopic_CommunityId` case everywhere.
- [ ] Update [server/services/chat/context.go](../../server/services/chat/context.go) to populate `ConversationContext` (topic title = community name, topic image = community avatar) for community topics. Any error wrapping must use `connecterr.Internal(ctx, op, err, kv...)` — not bare `CodeInternal`.
- [ ] Update [server/conversation/state.go](../../server/conversation/state.go) `IsItemDone()` — community conversations are never "done" (no completion state); return `false`.
- [ ] Tests for context population and `IsItemDone` behavior.
- [ ] `npm run build` and `go build -tags=integration ./...` clean (the `integration` build is required for any change to exported `server/ai/` or `server/services/experience/` signatures per [CLAUDE.md](../../CLAUDE.md) — confirm no integration-tagged callers reference touched APIs).

**Validates:** No `default:` panics or silently-wrong behavior on the new topic variant in any server path.

### Phase 3: Notifications (reuse the existing chat-notification path)

**Goal:** Community-wide chat messages reach other members through the same per-conversation unread-count + push path that gear/experience chat already uses, with **no new `CommunityEventType` enum value and no new undo-registry entry** (per Resolved Decisions).

**Tasks:**
- [ ] Confirm by reading the chat-notification path end-to-end (start at the `SendMessage` handler in [server/services/chat/](../../server/services/chat/) and follow through `server/community/notifications.go` and the FCM dispatch) that a community-topic conversation already produces the expected per-recipient unread-count update and push notification, gated by `NotifyChats`. If yes, this phase is a no-op confirmation.
- [ ] If the existing path drops community-topic conversations on the floor anywhere (e.g. a topic-type switch that has no `community_id` case), add the missing case so the path treats community-topic conversations the same as entity-topic conversations. Do **not** add a new event type — the routing decision is to stay on the unread-count path.
- [ ] Verify `NOTIFICATION_CATEGORY_CHATS` is the category that fires (it should already be, since this is just another conversation). If a new category is somehow needed, that's a signal to revisit the Resolved Decision — pause and update the plan.
- [ ] Verify the chat-notification tests still pass and add a new test exercising the community-topic path specifically.
- [ ] `npm run build` clean (no `TestEveryEventTypeClassified` change expected since no new enum value is added).

**Validates:** Sending a message in a community chat results in a push notification to other members (subject to their `NotifyChats` preference), parity with gear/experience chat.

### Phase 4: Flutter — Discuss tab on community content view

**Goal:** Restructure `community_content_view.dart` to expose a Discuss tab using the shared chat widget, with all data access flowing through a repository per [docs/client/architecture.md](../../docs/client/architecture.md).

**Repository layer (mandatory before viewmodel work):**
- [ ] Add `getConversationForCommunity(String communityId) → Future<ConversationItem>` to the existing chat repository (or create one if there isn't a `ChatRepository` today). Per [docs/client/architecture.md](../../docs/client/architecture.md), the viewmodel cannot call `ChatService` directly. Cache key: `'conversation:community:$communityId'` per the namespace convention in [docs/client/caching.md](../../docs/client/caching.md). Pair the getter with a `refreshConversationForCommunity(communityId)` invalidation method for the inbox-pin and chat-tab refresh paths.
- [ ] Provide a `chatRepositoryProvider` if not already present. Repository takes `cacheManagerProvider` + `chatServiceProvider` per the standard DI shape.
- [ ] Repository unit tests covering cache hit / cache miss / invalidation per [docs/client/testing.md](../../docs/client/testing.md) §"Repository Testing".

**Viewmodel + view:**
- [ ] Refactor [community_content_view.dart](../../app/lib/presentation/screens/communities/community_content_view.dart) to use the shared tab infrastructure ([content_view_tab_bar.dart](../../app/lib/presentation/widgets/content/content_view_tab_bar.dart) and [content_view_pane_switcher.dart](../../app/lib/presentation/widgets/content/content_view_pane_switcher.dart)). The current single-pane layout becomes the first tab; Discuss is the second.
- [ ] Add `community_chat_pane.dart` under `app/lib/presentation/screens/communities/widgets/`, modeled on [experience_chat_pane.dart](../../app/lib/presentation/screens/experience/widgets/experience_chat_pane.dart). It wraps `InlineConversationView` with the community accent color sourced via `AppColors.<semantic>(context)` — never a hardcoded `Color(0xFF...)` or raw `AppColors.primary` static (must be theme-aware per [docs/client/architecture.md](../../docs/client/architecture.md) §"Colors and Theming"). If a new semantic color is needed, add it to `AppColors` first.
- [ ] Extend [community_content_view_model.dart](../../app/lib/presentation/viewmodels/community_content_view_model.dart) and [community_content_state.dart](../../app/lib/presentation/viewmodels/community_content_state.dart) to track `activeTab` and the community's cached `ConversationItem`. The viewmodel calls `chatRepositoryProvider`'s `getConversationForCommunity` — never `chatServiceProvider` directly.
- [ ] **Async safety:** every `state = state.copyWith(...)` after an `await` needs an `if (!ref.mounted) return;` guard, or refactor to `AsyncNotifier`/`SafeNotifierMixin` per [docs/client/architecture.md](../../docs/client/architecture.md) §"Async Safety in Notifiers". The viewmodel must add a disposal test (the testing doc explicitly requires this for any async Notifier).
- [ ] Update [conversation_view_model.dart](../../app/lib/presentation/viewmodels/conversation_view_model.dart) and helpers (e.g. `core/utils/conversation_helper.dart`, `presentation/models/inbox_item.dart`) to handle the new `community_id` topic variant in their `whichTopicId()` switches. Topic title = community name; topic image = community avatar.

**i18n + accessibility:**
- [ ] Add new ARB keys to **both** `app/lib/l10n/app_en.arb` and `app/lib/l10n/app_es.arb` per [docs/client/i18n.md](../../docs/client/i18n.md): `communityTabDiscuss` (the tab label), `communityChatEmpty` (empty state), and any other user-visible strings introduced. Include `@key` metadata in the English ARB. Tab label must come from `context.l10n.communityTabDiscuss`, never a Dart literal — `npm run lint:dart:i18n` will reject violations.
- [ ] Accessibility (per [docs/client/accessibility.md](../../docs/client/accessibility.md)): the Discuss tab tap target uses the existing `Toggle` (segmented-control style) primitive that backs the other content views' tab bars — do **not** wrap in raw `GestureDetector`/`InkWell`. The tab bar widget should already do this, but verify. Any `IconAction`/`Tappable` introduced needs a `semanticsLabel` populated from `context.l10n` with the `a11y` prefix (e.g. `a11yCommunityChatTab`). Add the corresponding key to both ARB files. The lint allowlist (`scripts/a11y_allowlist.txt`) is empty — any new violation must be fixed in the same PR.

**File-size + tests:**
- [ ] Keep file size under the 1000-line lint gate for any Dart files touched ([CLAUDE.md](../../CLAUDE.md) "File Organization" — `npm run lint:dart:size`). The community content view is currently small but a refactor that adds tab plumbing risks bloating it; split the chat pane into its own file as planned.
- [ ] **Tests** (per [docs/client/testing.md](../../docs/client/testing.md)): viewmodel test using `ProviderContainer` with `MockChatRepository` for tab switching + conversation fetch (success and error paths); disposal test for the new viewmodel; widget test (with `localizationsDelegates` from `app/test/helpers/l10n_helpers.dart`) that the Discuss tab renders `InlineConversationView` with the right `conversationId`.
- [ ] `npm run test:app`, `npm run lint:dart:size`, `npm run lint:dart:i18n`, `npm run lint:dart:a11y` clean.

**Validates:** A user in any community can open the community content view, switch to Discuss, send and receive messages in real time. All strings localized in en + es. No new a11y violations.

### Phase 5: Inbox pin for single-community context

**Goal:** When `selectedCommunityProvider.selectedCommunityId != null`, the inbox shows a pinned community-chat card above the `DailyItem` list, kept fresh by the existing event-driven invalidation infrastructure.

**Tasks:**
- [ ] Add a pinned-card widget at the top of [portfolio_inbox_screen.dart](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart). The widget is conditional on `selectedCommunityProvider.selectedCommunityId != null` — hidden in All Circles aggregate context.
- [ ] **Hydration goes through the repository, not the service.** The pin's data source is `chatRepositoryProvider.getConversationForCommunity(selectedCommunityId)` (added in Phase 4) — viewmodel-only access per [docs/client/architecture.md](../../docs/client/architecture.md). Display: community name, avatar, last message preview, unread count badge, last-activity timestamp. Reuse the same row layout as existing `DailyItem` rows for visual consistency.
- [ ] **Live freshness** (per [docs/client/caching.md](../../docs/client/caching.md) §Pattern 7): the pin's viewmodel listens to `dailyCacheInvalidationProvider` (so it refreshes after any chat mutation) and to `unreadCountProvider` (so the unread badge applies optimistic deltas without a server round-trip). Reuse the same debounce/cooldown shape as `DailyNotifier`.
- [ ] **Image caching** (per [docs/client/caching.md](../../docs/client/caching.md) §"Image Cache Keys"): the community avatar must use `CachedMediaImage` with a stable `cacheKey` from `MediaUrl.cacheKey` (or `ImageCacheKeys.thumbnail(mediaId)` directly) — never `Image.network` of a presigned URL. [CLAUDE.md](../../CLAUDE.md) calls this out explicitly under "Media Display".
- [ ] **Navigation**: tap the pin → `NavigationHelpers.pushScreen(...)` (or `pushToItemScreen(...)` if it covers the community case) per [docs/client/architecture.md](../../docs/client/architecture.md) §"Pushing screens". Never `Navigator.push(MaterialPageRoute(...))` (lint-gated by `npm run lint:nav`) and never `context.push()` outside auth flows. Pass `initialTab` set to the Discuss-tab index, parity with how `_navigateToItem` currently routes to `initialTab: 2` when there are unreads.
- [ ] **Accessibility** (per [docs/client/accessibility.md](../../docs/client/accessibility.md)): the pinned card's tap target uses `Tappable` with a `semanticsLabel` from `context.l10n.a11yCommunityChatPin` (new key, both locales). The unread-count badge falls under the `avoid_color_only_status` rule — must have a `Semantics(...)` or `Tooltip(...)` wrapper announcing the count. The community avatar uses `CachedMediaImage(semanticsLabel: 'Photo of ${community.name}')` (or the equivalent l10n-driven label).
- [ ] **i18n**: add `inboxPinnedCommunityChatTitle` (or similar — re-use the `community*` or `inbox*` prefix per [docs/client/i18n.md](../../docs/client/i18n.md) §"Key Naming Convention") and the `a11yCommunityChatPin` key to both `app_en.arb` and `app_es.arb`. Viewmodel returns typed state (community name, unread count); the widget resolves the label in `build()`.
- [ ] Listen for selection changes — switching from one community to another should re-hydrate the pin; switching to All Circles should hide it. Use `ref.listen(selectedCommunityProvider, ...)` in the inbox viewmodel.
- [ ] **Tests** (per [docs/client/testing.md](../../docs/client/testing.md)): ViewModel test with `ProviderContainer` + mocked `ChatRepository` for pin visibility under both selection states (single community / All Circles); test that `dailyCacheInvalidationProvider` triggers re-fetch; test that `unreadCountProvider` deltas update the displayed count without a fetch. Widget test for accessibility-label rendering. Disposal test for any new stream subscription.
- [ ] `npm run test:app`, `npm run lint:dart:i18n`, `npm run lint:dart:a11y`, `npm run lint:nav`, `npm run lint:dart:size` clean.

**Validates:** A user in a single community sees the community chat pinned at the top of their inbox at all times, with badge counts that update in real time. A user in All Circles does not.

### Phase 6: End-to-end verification

**Goal:** Validate the full user flow on a real emulator before declaring complete.

**Tasks:**
- [ ] Run the app on Android emulator + iOS simulator (per the `ripls-app-logs` skill). Manually exercise: create a fresh community → conversation auto-creates → switch to that community in the sidebar → see the pin in inbox → tap pin → land in Discuss tab → send a message → second user (test account) receives the push → tapping the push deep-links to the same Discuss tab.
- [ ] Verify backfill ran for any pre-existing test communities (check server logs for the `community-conversation-backfill` operation field; counts in INFO entries should reconcile to the community count).
- [ ] Run `npm run build`, `go build -tags=integration ./...`, `npm run test:app`, full server `go test ./...`. Also `go test -tags=integration ./...` for any package whose exported API changed (per [CLAUDE.md](../../CLAUDE.md) "Go Server Build Verification"). All clean.
- [ ] Update / add system test if the Discuss-tab flow is in scope for `ripls-update-system-tests`.

**Validates:** Real-world behavior matches the spec.

## Related Issues

- **#289 — Top-level `ConversationTopic` proto type.** Direct prerequisite. The shared topic abstraction this issue extends. Per research, the shared type already exists in both packages; this issue adds one new oneof variant.
- **#1629 — Sidebar single-select + All Circles.** Direct prerequisite for Phase 5. `selectedCommunityProvider.selectedCommunityId == null` is the "All Circles" state; Phase 5 hides the inbox pin in that state.
- **#1107 — Per-community notifications.** Provides the `NotifyChats` preference per community that Phase 3 leverages for the opt-out default.
- **#1280 — `ListConversations` N+1.** The community-chat additions should not regress this. Phase 1 should keep the conversation-fetch path batched.
- **#1524 — Chat conversation participant index.** Adjacent perf issue; community chat (with potentially large participant sets) makes the index more important. Worth checking that Phase 1's compute-on-fly participant resolution doesn't depend on a missing index.
- **#1141 / #1542 — (chat-stream-related issues if any are open).** Not direct dependencies; flagged for awareness in case Phase 4 surfaces stream-related regressions.

---

## Audit Notes

Reviewed against [docs/client/architecture.md](../../docs/client/architecture.md), [docs/client/caching.md](../../docs/client/caching.md), [docs/client/i18n.md](../../docs/client/i18n.md), [docs/client/accessibility.md](../../docs/client/accessibility.md), [docs/client/testing.md](../../docs/client/testing.md), [docs/server/architecture.md](../../docs/server/architecture.md), [docs/server/observability.md](../../docs/server/observability.md).

**Changes made during audit:**

1. **Server architecture — service-to-service coupling (Phase 1).** Original plan said "wire `CreateCommunityConversation` into the community-creation path", which would have made `CommunityService` depend on `ChatService` — a direct violation of [docs/server/architecture.md](../../docs/server/architecture.md) §"Service Independence". Replaced with a two-option architectural choice (shared library at `server/chat/` vs. job-based fan-out) with a clear default and the rationale for picking it.

2. **Connect error wrapping (Phase 1, Phase 2).** Plan was silent on the `connecterr.Internal()` requirement. Added explicit calls to use `connecterr.Internal(ctx, op, err, kv...)` instead of bare `connect.NewError(connect.CodeInternal, err)`. The latter is gated by `scripts/check_codeinternal.js` and would fail the build.

3. **Goroutine safety (Phase 1).** Backfill jobs commonly spawn goroutines. Added explicit `logging.GoSafe(ctx, "name", fn)` requirement per [docs/server/architecture.md](../../docs/server/architecture.md) §"Goroutine Safety", which is enforced by `server/cmd/check-goroutines`.

4. **Structured logging (Phase 1).** Added a dedicated bullet enumerating the standard field names (`operation`, `community_id`), level guidance (INFO/WARN for user errors, ERROR for system failures), and the `logging.LoggerWithContext(ctx)` call site per [docs/server/observability.md](../../docs/server/observability.md). Original plan ignored observability entirely.

5. **Repository layer mandatory (Phase 4).** Original plan had the viewmodel calling `GetConversationForCommunity` directly via the service. That is the single most common architecture violation per [docs/client/architecture.md](../../docs/client/architecture.md) — viewmodels can only call repositories, never services. Promoted "add to ChatRepository with cache key `'conversation:community:$id'`" to a mandatory pre-step at the top of Phase 4, with paired repository tests per [docs/client/testing.md](../../docs/client/testing.md). Phase 5 was updated similarly to consume the repository instead of the service.

6. **Cache namespace + invalidation hooks (Phase 5).** Phase 5 originally said "Listen for selection changes" but had no plan for *live* updates when chat messages arrive. Added explicit hooks into `dailyCacheInvalidationProvider` and `unreadCountProvider` per [docs/client/caching.md](../../docs/client/caching.md) §Pattern 7. Without this, the pin would show stale unread counts until the user manually refreshed.

7. **Image caching (Phase 5).** Added explicit `CachedMediaImage` + stable `cacheKey` requirement for the community avatar per [docs/client/caching.md](../../docs/client/caching.md) §"Image Cache Keys" / [CLAUDE.md](../../CLAUDE.md) §"Media Display". Using `Image.network` of a presigned URL would re-download on every render and conflict with the rest of the app's image cache.

8. **i18n compliance (Phase 4, Phase 5).** Plan had user-visible strings ("Discuss tab label", "pinned community-chat card") with no l10n plan. Added concrete ARB key proposals (`communityTabDiscuss`, `communityChatEmpty`, `inboxPinnedCommunityChatTitle`, `a11yCommunityChatPin`, `a11yCommunityChatTab`) using the namespaced `community*`/`inbox*`/`a11y` prefixes from [docs/client/i18n.md](../../docs/client/i18n.md). Both `app_en.arb` and `app_es.arb` must be updated. Verified the viewmodels-never-resolve-strings rule applies (typed state from VM, l10n in `build()`).

9. **Accessibility primitives (Phase 4, Phase 5).** Plan said nothing about a11y. Added explicit references to `Tappable`, `Toggle`, `IconAction`, `CachedMediaImage(semanticsLabel:)`, and `Semantics`/`Tooltip` for the unread-count badge per [docs/client/accessibility.md](../../docs/client/accessibility.md). Also flagged that the allowlist (`scripts/a11y_allowlist.txt`) is empty, so any new violation must be fixed in the same PR.

10. **Theme-aware colors (Phase 4).** Original plan said "wrap with the community accent color (`AppColors.primary`)". Static `AppColors.primary` is not context-aware; in dark mode this would produce wrong contrast. Updated to require `AppColors.<semantic>(context)` per [docs/client/architecture.md](../../docs/client/architecture.md) §"Colors and Theming".

11. **Navigation primitives (Phase 5).** Original plan said "navigate to the community content view" without specifying the helper. Added explicit `NavigationHelpers.pushScreen(...)` requirement per [docs/client/architecture.md](../../docs/client/architecture.md) §"Pushing screens" — using `Navigator.push(MaterialPageRoute(...))` would flash the home screen and is lint-gated.

12. **Async safety in viewmodels (Phase 4).** Added explicit requirement for `if (!ref.mounted) return;` guards (or migration to `AsyncNotifier`/`SafeNotifierMixin`) and a disposal test per [docs/client/architecture.md](../../docs/client/architecture.md) §"Async Safety in Notifiers" / [docs/client/testing.md](../../docs/client/testing.md) §"Disposal Testing". Without this, autoDispose providers crash on rapid screen exit.

13. **Test patterns spelled out (Phase 4, Phase 5).** Original test bullets were generic ("viewmodel test", "widget test"). Updated to name `ProviderContainer`, `MockChatRepository`, `MockChatService`, the `localizationsDelegates` helper at `app/test/helpers/l10n_helpers.dart`, and the disposal-test pattern from [docs/client/testing.md](../../docs/client/testing.md).

14. **Lint gates enumerated.** Each Flutter phase now lists the specific lint commands (`npm run lint:dart:i18n`, `lint:dart:a11y`, `lint:nav`, `lint:dart:size`) that the CI will run, so the implementer knows what will fail before submitting a PR.

**No changes needed:**

- **Proto separation (api ↔ models).** The plan already respects [CLAUDE.md](../../CLAUDE.md)'s rule that `api/` and `models/` packages must not import each other — the new `community_id` variant is added in parallel to both.
- **Dedicated request/response per RPC.** `GetConversationForCommunityRequest`/`GetConversationForCommunityResponse` already follow the rule against reusing RPC message types.
- **Optional proto fields.** No new fields in this issue need `optional` — the new `community_id` is a oneof variant (which already has presence semantics) and the existing `ConversationItem.community_id` field stays unchanged.
- **Event-vs-Experience UI rule.** This issue is not about the `Experience` domain object, so no naming conflicts.
- **Server file-size guidance.** Conversation creation logic added to `server/chat/` shared library is small enough to fit existing files.
