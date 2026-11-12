# Issue #2814: [tech-debt] Finish the GetConversationContext migration and delete InboxItem.fromConversation

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2814
**Filed:** 2026-07-29 | **Last activity:** 2026-07-29
**Labels:** `review:code_quality` | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`; anchors `docs/client/architecture.md`,
`docs/client/conventions.md`, `docs/proto_conventions.md`; leaf docs
`docs/client/chat.md`, `docs/client/i18n.md`, `docs/client/inbox.md`,
`docs/client/testing.md`; review dimensions `docs/reviews/code_quality_review_prompt.md`,
`docs/reviews/tech_debt_review_prompt.md`, `docs/reviews/testing_review_prompt.md`;
plus `app/lib/presentation/models/README.md`.

## Outcome (implemented)

Delivered as **Option C taken to its conclusion**: once the legacy factory was
gone, `InboxItem`'s only fields with a live reader were `conversation` and
`unreadCount`, so the model was deleted outright rather than converted to
Freezed — those two fields now live directly on `ConversationState` and the
`copyWith` the issue asked for is the one Freezed already generates there. Net
`app/lib`: **+59 / −365**.

Two things in this plan were superseded by that:

- **Phase 2's Freezed conversion did not happen and was not needed.** Freezing
  `InboxItem` would have been boilerplate around a class that had no reason to
  exist. The `ConversationState` value-`==` concern it raised is moot for the
  same reason.
- **`hasInitializedRequest` / `hasInitializedExperience` also went.** They were
  not in the plan's Phase 3 list; they turned out to gate nothing but the
  "first call: use cached InboxItem data" branches, so removing those branches
  left both flags write-only.

Everything else landed as planned, including the `ref.mounted` guard and the
decision to keep refreshing `unreadCount` from the refetched conversation.
Verified with `flutter analyze` (clean), the full Flutter suite (4285 passed,
1 skipped), `lint:dart:dead`, `lint:todo-refs`, `lint:ephemeral-comments`,
`lint:dart:viewmodel-tests`, `lint:dart:size`, `lint:context-map`.

Commits: `7a31c05b9` (characterization tests), `f4c5cf097` (removal), plus a
docs commit for `docs/client/chat.md` and `app/lib/presentation/models/README.md`.

## Problem Statement

`InboxItem.fromConversation` (`app/lib/presentation/models/inbox_item.dart:123`)
is the pre-`GetConversationContext` factory. It has exactly one caller left —
`ConversationViewModel.refreshParticipantsFromSystemMessage`
(`app/lib/presentation/viewmodels/conversation_view_model.dart:539`) — and the
issue asks for that caller to be migrated and the factory deleted.

Codebase research changes the shape of the job in three ways, all of which make
it **easier and larger** than the issue supposes:

1. **The obstacle the issue names third is not real.** The issue's step 3 asks
   whether `state.cachedTransfer` / `cachedRequest` / `cachedExperience` "still
   have any other consumer; if not, they can go too." They do have other
   consumers and must stay: `cachedTransfer` at `conversation_view_model.dart:301`
   (`fetchGearThumbnail`), `:339` (`selectRecipient`), `:447`/`:458`
   (`_invalidateAndRefreshGear`); `cachedRequest` at
   `conversation_request_actions.dart:71` (`fetchRequestImpact`) and `:88`
   (`fetchRequestMedia`); `cachedExperience` throughout
   `conversation_experience_actions.dart`. Only the *pass-through into the dead
   factory* goes away.

2. **The obstacle the issue names second dissolves on inspection.**
   `InboxItem.gearThumbnailUrl` does not need "a home in the new factory" — it
   is a **write-only field**. Its only read in the entire app is
   `conversation_view_model.dart:546`, where it is fed straight back into
   `fromConversation` to be re-stored on the next `InboxItem`. It is a closed
   loop with no consumer. The gear thumbnail that the UI actually uses is the
   *separate* `ConversationState.gearThumbnailUrl`, written by
   `fetchGearThumbnail` at `conversation_view_model.dart:326`. So the field is
   deleted, not relocated.

3. **The obstacle the issue names first is narrower than stated, but one field
   is real.** Almost everything `fromConversation` computes is consumed by
   nobody. Across all of `app/lib` and `app/test`, `InboxItem.description`,
   `.state` and `.timestamp` have **zero read sites** — the factory's entire
   string-composition body (`'${transfer.gearName} $transferType from
   ${transfer.owner.name}'`, `'Request: $displayText'`, `'Helping with: …'`) and
   its max-of-four-timestamps arithmetic produce values that are stored and
   never read. The only field the factory genuinely refreshes is
   **`unreadCount`**, taken from the refetched `ConversationItem.unread_count`
   (server-populated — `server/services/chat/conversations.go:357` et al.) and
   read by `conversation_messages_notifier.dart:92` and `:941` (the
   `refreshAfterResume` mark-read gate). A bare
   `copyWith(conversation: updated)` would silently drop that refresh; the
   correct swap is `copyWith(conversation: c, unreadCount: c.unreadCount)`.

Beyond the issue's scope, the same read shows that `InboxItem` has quietly
become a conversation-only type whose transfer-era surface is unreachable:

- `InboxItem.fromTransferRequestReceived` and `.fromTransferRequestExtended`
  (`inbox_item.dart:53`, `:71`) have **no call sites at all** in `app/lib`.
- Consequently `InboxItemType.transferRequestReceived` / `.transferRequestExtended`
  are never constructed in production, so the two branches that test for them —
  `getOtherParticipantName` (`:233`) and `getOtherParticipant` (`:262`) — are
  dead; the `conversation != null` guard above them always wins on the only
  production construction path (`fromConversationContext`, which never sets
  `transfer`). Only `conversation_view_model_test.dart:630` constructs that type.
- `fromConversationContext` declares a `String? currentUserId` parameter it
  never reads (`inbox_item.dart:92`).
- Because `fromConversation` was laundering `state.cachedRequest` /
  `cachedExperience` back onto `state.inboxItem`, the "first call: use cached
  InboxItem data" short-circuits in `fetchRequestDetails`
  (`conversation_request_actions.dart:33`) and `fetchExperienceDetails`
  (`conversation_experience_actions.dart:58`) are the only thing that ever
  repopulates those fields. Delete the factory and those branches become
  provably unreachable.

**Why CI never flagged any of this.** `npm run lint:dart:dead`
(`app/tool/dead_code.dart`) deliberately "skips constructors and operators …
they are invoked through the type, not by name" (`dead_code.dart:597-598`), so
the two dead named-constructor factories are invisible to it. And its field
pass reports "public declarations whose name never appears outside their own
library" — `gearThumbnailUrl` appears in `conversation_state.dart:34` as a
*different* field, and `description`/`state`/`timestamp` are ubiquitous
identifiers, so all four survive by name collision. This is the tool's
documented conservative failure mode ("misses are the expected failure mode"),
not a bug, but it means the deletions in this issue must be verified by hand.

There is no contradiction between the issue body and later discussion, because
there is none — see below.

## Context from Comments

**The issue has zero comments.** It was auto-filed on 2026-07-29 while auditing
untracked `TODO:` comments for the `check_todo_refs.js` gate in #1611, and the
`TODO(#2814):` marker already landed in `inbox_item.dart:117-122` (commit
`92aab84c2`, "Triage the untracked TODO comments"). Nobody has weighed in since.

That absence matters less than it usually would. This is a self-contained,
single-file-blast-radius cleanup with no product-visible surface, and the issue
body is unusually specific about what it believes blocks the change. The
research above resolves each of its three stated blockers with direct evidence
from the code, so the plan does not need maintainer input to proceed — but the
**scope question** (minimal swap vs. full collapse of the now-dead transfer-era
surface) is a judgement call and is raised in Open Questions.

Two pieces of relevant history not in the issue:

- The migration's *other* caller was never migrated — `inbox_view_model.dart`
  was **deleted wholesale** by commit `ab3d703e0` ("Remove 146 Flutter files
  reachable only from their own tests") during the dead-Flutter-code sweep. The
  "one caller away from done" state is an accident of that sweep, which is why
  the remaining caller is the awkward one.
- `docs/reviews/reports/tech_debt_review.md:237` already carried this finding
  ("Legacy `InboxItem.fromConversation` Factory Still Has Live Callers", est.
  2–3 hours) against the two-caller world.

## Potential Solutions

### Option A: Minimal swap — hand-written `copyWith`, delete the one factory

- **Approach:** Add a hand-written `copyWith` to the plain `InboxItem` class,
  rewrite `refreshParticipantsFromSystemMessage` as
  `state.inboxItem.copyWith(conversation: c, unreadCount: c.unreadCount)`,
  delete `fromConversation`. Leave everything else — the dead transfer
  factories, `gearThumbnailUrl`, `description`/`state`/`timestamp` — in place.
- **Pros:** Smallest diff; closes the issue exactly as written; no codegen step;
  trivially reviewable.
- **Cons:** Adds ~15 lines of hand-maintained `copyWith` boilerplate that
  `docs/client/architecture.md:121-173` and
  `app/lib/presentation/models/README.md:7` both say should be Freezed-generated.
  Leaves the dead surface that this same investigation just proved dead, so the
  next reader repeats the analysis. `gearThumbnailUrl` survives as a field with
  no reader and no writer — strictly worse than today, where at least the
  round-trip explained its existence.
- **Effort:** Small (~1–2 h including a regression test).

### Option B: Freezed `InboxItem` + swap + delete the factory

- **Approach:** Convert `InboxItem` to a Freezed data class (`@freezed`,
  `const factory InboxItem({...})`), keeping all named parameters and the
  surviving named factories as `factory InboxItem.fromX(...) => InboxItem(...)`
  redirects. Then do Option A's swap and deletion on top.
- **Pros:** `copyWith` for free, matching the codebase norm — `ConversationState`
  itself is Freezed, and 20+ viewmodels/state classes already are. Removes
  hand-maintained boilerplate rather than adding it. Makes the model immutable
  by construction.
- **Cons:** One real behavioural side effect: Freezed generates value `==`, so
  `ConversationState`'s equality (`conversation_state.freezed.dart:336`) starts
  comparing `inboxItem` **by value** instead of by identity. Protobuf-Dart
  `GeneratedMessage` and `DateTime` both have value equality, so this is
  well-defined; the effect is *fewer* rebuilds of the conversation view when an
  equal-valued item is re-set. Desirable, but it is a change and belongs in the
  test plan. Also adds `inbox_item.freezed.dart` to the codegen set.
- **Effort:** Small–medium (~2–3 h).

### Option C: Collapse `InboxItem` to its live surface

- **Approach:** Option B, plus delete everything the read proved unreachable:
  `fromTransferRequestReceived`, `fromTransferRequestExtended`, the two
  `InboxItemType` variants and the dead branches in `getOtherParticipantName` /
  `getOtherParticipant`, the `gearThumbnailUrl` field, the unused
  `currentUserId` parameter on `fromConversationContext`, and — pending
  confirmation — `description` / `state` / `timestamp` and the
  `transfer`/`requestItem`/`experience` fields together with the now-unreachable
  "first call: use cached InboxItem data" short-circuits in
  `fetchRequestDetails` / `fetchExperienceDetails`.
- **Pros:** Leaves `InboxItem` describing what it actually is — a conversation
  plus its context — instead of a three-variant union with one live variant.
  Removes the last client-side user-copy composition in the model layer, which
  `docs/client/i18n.md:50` ("Viewmodels never resolve strings") forbids on
  principle. Deletes the dead code the gate structurally cannot see. Net line
  count strongly negative.
- **Cons:** Widest blast radius: touches five files plus tests, and requires
  updating `docs/client/chat.md:340-356` (which currently documents the
  `InboxItemType` three-way discrimination and the optional full
  `Transfer`/`Request`/`GetExperienceResponse` objects as live design). Each
  deletion needs manual confirmation because, as established, the dead-code gate
  cannot corroborate any of them. If `InboxItemType` collapses to a single
  variant, the enum itself should arguably go, which ripples into the test file.
- **Effort:** Medium (~4–6 h with tests and docs).

## Recommendation

**Option C, delivered as Option B first.** Ship the behaviour-neutral core
(Freezed + swap + factory deletion) as a self-contained increment, then take the
dead surface in a second increment that can be reviewed and reverted
independently.

The reasoning:

- Freezed rather than a hand-written `copyWith` because the project has already
  made that call twice in writing (`docs/client/architecture.md:167` "Immutable
  state via Freezed"; the models README's "Many are Freezed classes for
  immutability and `copyWith` support"), and adding hand-rolled boilerplate to
  close a tech-debt issue is a poor trade.
- Collapsing the dead surface rather than stopping at the issue's literal ask
  because the evidence for its deadness is a by-product of work this issue
  requires anyway. Leaving it means someone re-derives that
  `InboxItem.gearThumbnailUrl` has no reader — and the tooling that would
  normally answer that question is structurally blind here.
- Splitting B from C because they carry different risk: B is provably
  behaviour-preserving modulo the `==` change; C rests on exhaustive-grep
  negative results, which are strong but not machine-checked.

The one substantive behavioural decision inside B is `unreadCount`. Today
`fromConversation` overwrites the `unreadCountRepository`-sourced count that
`fromConversationContext` installed with `ConversationItem.unread_count` from
the server — two different sources of truth for the same field, swapped silently
by any JOINED/LEFT system message. I recommend **preserving current behaviour**
(`unreadCount: updated.unreadCount`) so this change stays neutral, and filing
the source-of-truth inconsistency separately rather than fixing it under a
delete-a-factory issue.

## Open Questions

- **How far to collapse — Option B only, or B then C?** — Recommendation: B then
  C, as above. If the reviewer wants the issue closed narrowly, land B, close
  #2814, and file C as a follow-up "InboxItem is conversation-only; delete the
  transfer-era surface" — but do not lose the analysis, because the dead-code
  gate will not regenerate it.
- **Should `unreadCount` be refreshed at all on a JOINED/LEFT message?** —
  Recommendation: yes, preserve today's refresh in this change. A JOINED/LEFT
  system message *is* a message and can move the count, and changing this under
  a refactor would make the diff non-neutral. The deeper question — which of
  `unreadCountRepository` and `ConversationItem.unread_count` is authoritative —
  deserves its own issue.
- **Does `InboxItemType` survive?** — Recommendation: if C lands and only
  `conversation` remains, delete the enum and the `type` field entirely rather
  than keeping a one-valued discriminator. Revisit if #2436 (DM conversation
  type) is imminent, since that may want a discriminator back — but a
  speculative one-variant enum is worse than re-adding a purposeful one.
- **Should `dead_code.dart` learn about named constructors?** —
  Recommendation: not in this issue. Named-constructor liveness is genuinely
  harder than the tool's current model (a `factory Foo.bar()` really can be
  reached through the type in ways the token index cannot distinguish), and
  bolting it on here would blur a clean cleanup. File it as a follow-up against
  the detector.
- **Is a `ripls-ui-investigate` run needed?** — No. Recommendation is to skip
  it and record why: #2814 reports no UI-observable misbehavior. It is a
  structural tech-debt issue whose entire evidence base is static call-site
  analysis, and the target change is designed to be behaviour-neutral. The
  behavioural risk it *does* carry (participant refresh after a JOINED/LEFT
  system message) is better covered by the viewmodel test in Phase 1 than by a
  hermetic web probe, since it is stream-driven and has no distinct visual
  state.

## Implementation Plan

### Phase 1: Characterize today's behaviour

**Goal:** A failing-if-broken test for `refreshParticipantsFromSystemMessage`
before anything changes. There is currently **no test for this method at all** —
`conversation_view_model_test.dart` never exercises it, and no e2e spec covers
`CHAT_SYSTEM_ACTION_JOINED` / `_LEFT` participant refresh.

**Tasks:**
- [ ] Add a test group to
      `app/test/presentation/viewmodels/conversation_view_model_test.dart`
      driving `refreshParticipantsFromSystemMessage` through the mocked
      `ChatRepository` (`refreshConversation` + `getConversation` +
      `getConversationContext`).
- [ ] Assert on the observable contract: `state.participants` excludes the
      current user and reflects the refetched membership;
      `state.inboxItem.unreadCount` tracks the refetched
      `ConversationItem.unread_count`; `state.conversationContext` and
      `state.transferStatusMap` update.
- [ ] Assert the failure path: when `getConversationContext` throws,
      `state.conversationContext` falls back to the prior value and participants
      still refresh (this is the fallback the issue worried about — pin it now).
- [ ] Run `npm run test:app` (or `cd app && flutter test --reporter expanded`).

**Validates:** That the current behaviour is pinned, and specifically that
`unreadCount` refresh is observable — the one field whose loss a naive
`copyWith` would cause and which no existing test would catch.

### Phase 2: Freezed `InboxItem`, swap the caller, delete the factory

**Goal:** Close the issue as written, behaviour-neutral, with Phase 1 green
throughout.

**Tasks:**
- [ ] Convert `InboxItem` to `@freezed` in
      `app/lib/presentation/models/inbox_item.dart`; keep every parameter name
      and the surviving named factories as redirecting factories.
- [ ] `npm run generate` (or `flutter pub run build_runner build`) to produce
      `inbox_item.freezed.dart`; confirm it is gitignored alongside the other
      `*.freezed.dart` outputs.
- [ ] Rewrite `conversation_view_model.dart:539-547` as
      `state.inboxItem.copyWith(conversation: updatedConversation, unreadCount: updatedConversation.unreadCount)`.
- [ ] Add the missing `if (!ref.mounted) return;` guard before the
      `state = state.copyWith(...)` at `:559` — the method awaits three times
      and writes state unguarded, unlike every neighbouring method in the file
      (`:476`, `:493`, `:587`, `:599`). Fire-and-forget from the message stream
      (`conversation_messages_notifier.dart:403`) makes a disposed-notifier
      write reachable.
- [ ] Delete `InboxItem.fromConversation` and its `logging` import if now unused.
- [ ] Remove the `TODO(#2814):` comment block (`inbox_item.dart:117-122`).
- [ ] Verify `ConversationState` equality behaviour under the new value `==`:
      re-setting an equal-valued `inboxItem` should no longer notify listeners.
      Add an explicit assertion so the change is documented, not incidental.
- [ ] `npm run lint:dart`, `npm run lint:dart:dead`, `npm run lint:todo-refs`,
      `npm run test:app`.

**Validates:** That the migration is genuinely finished and behaviour-neutral —
Phase 1's tests pass unchanged, and the `==` change is an asserted improvement
rather than a surprise.

### Phase 3: Collapse the dead transfer-era surface

**Goal:** `InboxItem` describes what it is. Each deletion is confirmed by a
targeted grep across `app/lib`, `app/test` and `e2e/` before it is made, since
the dead-code gate cannot corroborate any of them.

**Tasks:**
- [ ] Delete `gearThumbnailUrl` from `InboxItem` (write-only; the live one is
      `ConversationState.gearThumbnailUrl`).
- [ ] Delete `fromTransferRequestReceived` and `fromTransferRequestExtended`.
- [ ] Delete the `InboxItemType.transferRequestReceived` / `.transferRequestExtended`
      branches in `getOtherParticipantName` (`:230-237`) and
      `getOtherParticipant` (`:259-266`); update
      `conversation_view_model_test.dart:628-646` accordingly.
- [ ] Drop the unused `currentUserId` parameter from `fromConversationContext`
      and its two call sites.
- [ ] Confirm-then-delete `description`, `state`, `timestamp` from `InboxItem`
      (no read sites found; re-verify before cutting).
- [ ] Confirm-then-delete `transfer`, `requestItem`, `experience` and the
      now-unreachable "first call: use cached InboxItem data" short-circuits in
      `fetchRequestDetails` (`conversation_request_actions.dart:32-39`) and
      `fetchExperienceDetails` (`conversation_experience_actions.dart:57-74`).
      **Keep** `state.cachedTransfer` / `cachedRequest` / `cachedExperience` —
      they have live consumers.
- [ ] If only `InboxItemType.conversation` remains, delete the enum and the
      `type` field (see Open Questions).
- [ ] Re-run `npm run lint:dart:dead` after each removal — "removals cascade"
      per the tool's own header.
- [ ] Full gate: `npm run lint`, `npm run test:app`.

**Validates:** That the model's surface matches its single production
construction path, and that removing the laundering path did not strand a
consumer — the request/experience "cached data" branches are the specific place
a mistake would show.

### Phase 4: Docs

**Goal:** The durable docs stop describing a design that no longer exists.

**Tasks:**
- [ ] Update `docs/client/chat.md:340-356` (§Data Models → InboxItem): drop the
      "legacy stream-driven inbox" framing, the optional full
      `Transfer`/`Request`/`GetExperienceResponse` objects, and the
      three-variant `InboxItemType` description; state that
      `fromConversationContext` is the only constructor path.
- [ ] Update `docs/client/chat.md:156` (`ConversationState` contents) if
      `InboxItem`'s described payload changed.
- [ ] Advance the `freshness:` stamp on `docs/client/chat.md` per
      `docs/context_map.md`; run `npm run lint:context-map`.
- [ ] Refresh `app/lib/presentation/models/README.md:14` for the new `InboxItem`
      shape. While there: the README lists `metric_detail_item.dart` and
      `profile_metrics_data.dart`, neither of which exists in the directory —
      fix or file separately.

**Validates:** That `docs/llms.txt`-routed readers land on an accurate
description, closing the loop the freshness ratchet would otherwise open.

## Related Issues

- **#1611** — Go lint enforcement; its `check_todo_refs.js` gate is what forced
  the `TODO(#2814):` marker into `inbox_item.dart` and surfaced this issue.
- **#2436** — Add DM conversation type for a unified Messages list. The only
  plausible future consumer of a multi-variant `InboxItemType`; relevant to the
  "delete the enum?" open question.
- **#2435 / #2488 / #2514** — Home tab (portfolio inbox) redesign and
  follow-ups. Adjacent naming only: the portfolio inbox is server-assembled via
  `GetHomeView` and does **not** use this `InboxItem` model — worth stating so a
  future reader does not conflate them.
- **`docs/reviews/reports/tech_debt_review.md:237`** — the original
  `/ripls-reviews` finding, written when there were two callers.
