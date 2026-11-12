# Issue #1826: Chat messages out of order

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1826
**Filed:** 2026-05-10 | **Last activity:** 2026-05-10
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

A user (user2@example.org) observed chat messages rendered out of chronological
order on Android (app version 0.19.2 / build 6469 / PROD). The reporter noted
that the bug **persisted across pull-to-refresh** and was only resolved by an
**app restart**. The reporter further clarified on the planning PR that **the
two visibly out-of-order messages were sent ~20 minutes apart**, so this is
not a same-second / sub-second tie-breaking problem at all.

That eliminates timestamp precision as the root cause and leaves two
mutually-reinforcing defects:

1. **Server `StreamMessages` initial backlog is unsorted**
   (`server/services/chat/streaming.go:54`). The handler calls
   `s.storage.QueryByField(ctx, "conversation_id", ...)` which has no
   `ORDER BY` (`server/storage/protosql.go:222`), so PostgreSQL returns
   rows in arbitrary heap order. The server then sends each row to the
   client in that order. Heap order is unrelated to insertion time —
   `VACUUM`, `UPDATE`-driven HOT moves, page splits, and physical-row
   reuse from soft-deleted predecessors can all permute it. Two messages
   sent 20 minutes apart can absolutely be returned in the wrong order
   here.

2. **Client never defensively re-sorts**. The Flutter viewmodel
   (`app/lib/presentation/viewmodels/conversation_messages_notifier.dart`)
   trusts that any list it produces — from history, from streamed
   backlog, from `_mergeServerHistory` — is already in the right order
   and only ever appends. There is no defensive sort by
   `sent_at_unix_sec` at the display boundary. Whatever the server hands
   over (in whatever order) becomes the on-screen order.

These two together explain the diagnostic symptom precisely: pull-to-refresh
re-runs `GetConversationHistory` and the resulting messages are filtered
through `_mergeServerHistory` (line 686), which only keeps items not already
in `_messageIds` and **appends** them — it never re-sorts the existing local
state. So the corrupted ordering, established when the stream backlog was
appended in heap order, survives every refresh. A full restart wipes local
state, so `loadMessages()` runs first against `GetConversationHistory` (which
*is* sorted, in Go, by `sent_at_unix_sec` — see `messages.go:254-265`), and
when the stream backlog later arrives every row is dedup'd out by
`_messageIds`. Order is correct after restart.

The minimal fix is therefore: (a) make the server sort the stream backlog by
`sent_at_unix_sec` before sending it, and (b) make the client defensively
sort by `sent_at_unix_sec` at every state-merge point so that any future
out-of-order delivery (network reorder on reconnect, late stream message,
etc.) self-corrects.

A separate, latent same-second tie-breaking bug exists at
`server/services/chat/messages.go:254-265` — the in-Go selection sort uses
the v4 UUID as a tiebreaker, which is not chronological. That bug is
*not* what the reporter hit (his messages were 20 minutes apart) and is
out of scope for this fix; it is captured in **Open Questions** for a
follow-up.

## Context from Comments

Three comments inform the diagnosis:

1. *"This was fixed after an app restart."*
   (https://github.com/Type2Fun-Foundation/ripls/issues/1826#issuecomment-4414090504)
   — Confirms the bug is client-state-shaped (state survives
   pull-to-refresh, doesn't survive a full restart).

2. *"@claude"* (issue) — Triggered the initial planning run.

3. *"@claude these messages were sent 20 minutes apart, so it's not a
   timestamp precision issue"*
   (https://github.com/Type2Fun-Foundation/ripls/pull/1829#issuecomment-4414142818)
   — Reporter rules out same-second precision as the cause. This is the
   correction that drove this revision.

No other reports, no workaround discussion, no conflicting evidence.

## Potential Solutions

### Option A: Sort server stream backlog + add client defensive sort (Recommended)

- **Approach:** Two small changes, no proto changes, no migrations:

  - **Server** (`server/services/chat/streaming.go:54`). Replace the
    untyped `s.storage.QueryByField(ctx, "conversation_id", ...)` call
    with a small typed storage helper
    `chatmessage.ListByConversation(ctx, store, conversationID, asc bool, limit int)`
    that runs:

        SELECT binary_proto FROM "chat_message"
         WHERE conversation_id = $1
           AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec IS NULL)
         ORDER BY sent_at_unix_sec {ASC|DESC}, id ASC
         LIMIT $2

    For the stream backlog we want oldest-first (`asc = true`) with a
    sane default limit (200) per CLAUDE.md "always LIMIT unbounded
    queries". `id ASC` is a deterministic — but not chronological —
    tiebreaker; that's fine for messages 20 minutes apart, and the
    same-second-tie bug (Open Questions) is tracked separately.

    `GetConversationHistory` (`server/services/chat/messages.go:222`) is
    *not* in scope to migrate to this helper — its existing in-Go sort
    is correct for the bug under report (same-second ties aside). We
    leave it alone to keep the change minimal; its `BeforeUnixSec`
    pagination logic also stays as-is.

  - **Client**
    (`app/lib/presentation/viewmodels/conversation_messages_notifier.dart`).
    Add a private `List<ChatMessage> _sortByChronology(List<ChatMessage> msgs)`
    that does a stable sort on `(sentAtUnixSec, messageId)` ascending
    using `[...msgs]..sort(...)` so the input is not mutated. Call it at
    the end of:
      - `loadMessages` (after building `newMessages`, before the
        `state.copyWith(messages: ...)` write — currently line ~123).
      - `_handleStreamedMessage` (after the `..add(chatMessage)` for
        non-pending messages — currently lines ~315/378).
      - `_mergeServerHistory` (after the new-messages append — currently
        line ~715). This is the path pull-to-refresh exercises and is
        where the corrupted local ordering survives today.
      - `refreshAfterResume` (which delegates to `_mergeServerHistory`).

    Optimistic-send keeps its current `sentAtUnixSec = now()` so the
    temp message naturally sorts to the bottom; no other change needed.

- **Pros:**
  - Minimal: ~30 lines on the server, ~30 lines on the client, no
    proto/codegen, no migrations, no backfill.
  - Server fix is the actual root cause; client fix is defense in depth
    so that any future ordering bug (reconnect race, gap-fill from a
    different code path) self-corrects on next render rather than
    persisting until restart.
  - Wire-compatible by definition (no schema or API surface changes).
- **Cons:**
  - Does not address the latent same-second tie-breaking bug in
    `GetConversationHistory`'s in-Go sort. We accept that as a
    follow-up because no user has hit it, and addressing it touches
    proto, server, and client (see deferred Option B below).
- **Effort:** Small.

### Option B (deferred): Add millisecond-precision `sent_at_unix_milli` ordering key

- **Approach:** Same plan as the previous revision of this document —
  add `optional int64 sent_at_unix_milli` to
  `proto/ripls/models/chat.proto` `ChatMessage` and to the API surface
  (`MessageHistoryItem`, `StreamMessagesResponse`,
  `SendMessageResponse`); have the server populate it on every write
  and sort by it everywhere; one-shot transactional backfill of
  existing rows; client uses it as the primary sort key with fallback
  to `sent_at_unix_sec * 1000`.
- **Why deferred:** This *would* close the latent same-second
  tie-breaking gap. But the reporter has clarified that's not what
  caused this report, and Option A is enough to ship a full fix to the
  user-visible bug. Deferring keeps the user-facing fix small,
  reviewable, and revertible without coupling it to a multi-layer
  schema change.
- **When to revisit:** If a future user report does involve ties at
  the second granularity (high-throughput chat, scripted bots, fast
  back-and-forth replies), promote this to its own issue and pick up
  the prior plan from Git history.

### Option C: Client-only defensive sort (do nothing on the server)

- **Approach:** Skip the server change; only add the client-side
  defensive sort.
- **Pros:** Even smaller; one-side change.
- **Cons:**
  - Leaves the server emitting unsorted backlog over the stream to
    every other current and future client (web app, future native
    rewrites, integration test fixtures). The server is the
    authoritative source of order; relying on every client to fix it
    is fragile.
  - Not strictly necessary for the bug to recur in other shapes — a
    stream re-subscribe after backgrounding could re-deliver an
    unsorted backlog and we'd be one client-side bug away from the
    same report again.
- **Why rejected:** The server query is a one-line `ORDER BY` add. The
  cost is trivial, and centralizing the contract on the server is the
  right architectural choice.

## Recommendation

**Option A.** Sort the server stream backlog *and* add the client defensive
sort. This is the smallest change that fully resolves the user-visible bug
(unsorted heap-order backlog), is backed by an explicit cause-and-effect
chain matching all three reporter symptoms (out-of-order display,
pull-to-refresh doesn't help, restart fixes), and adds a defense-in-depth
client invariant so future delivery-order bugs self-correct on next render.
The latent same-second tie-breaking issue (Option B) is captured for a
separate ticket if and when it surfaces.

## Open Questions

- **Should we also fix the same-second tie-breaking bug in
  `GetConversationHistory`'s in-Go sort?** — Recommendation: **no, not
  in this PR.** It's a real latent bug — the v4 UUID secondary sort
  key is not chronological — but it's not what the reporter hit and
  fixing it cleanly requires either (a) Option B's milli-precision
  field across proto + server + client, or (b) accepting "stable but
  not chronological" same-second ordering, which is what we have
  today. Either choice deserves its own user signal and its own PR.
  File a follow-up issue when this user-visible report is closed.

- **Should we migrate `GetConversationHistory` to the new
  `ListByConversation` helper too?** — Recommendation: **no, leave
  it.** Its current in-Go sort plus `BeforeUnixSec` filter works for
  the bug under report and is unrelated to it. Migrating it adds
  scope and risk (especially around pagination) for no
  user-observable benefit. If a future PR adds the milli-precision
  key, that PR will rewrite both call sites consistently.

- **Why sort on the client at all if the server is now sorted?** —
  Defense in depth. A late-arriving message on the open stream
  (network jitter, reconnect race, server-side broadcast ordering not
  guaranteed across goroutines) can still arrive after a newer one.
  The cost of `[...msgs]..sort(...)` on every state-merge is O(n log
  n) on a chat list of at most a few hundred messages — trivial — and
  it eliminates an entire class of ordering bugs from ever reaching
  the screen.

- **Pagination: does the `BeforeUnixSec` cursor still work?** —
  Yes, unchanged. We are not touching `GetConversationHistory` here.

## Implementation Plan

### Phase 1: Server stream-backlog ordering

**Goal:** Make `StreamMessages` deliver the initial backlog in chronological
order rather than PostgreSQL heap order.

**Tasks:**
- [ ] Add `ListByConversation(ctx context.Context, store storage.Store, conversationID string, asc bool, limit int) ([]*models.ChatMessage, error)`
      to a new file `server/storage/chat_message.go` (mirror the style of
      the existing `chat_conversation.go` helpers). Body runs:

          SELECT binary_proto FROM "chat_message"
           WHERE conversation_id = $1
             AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec IS NULL)
           ORDER BY sent_at_unix_sec {ASC|DESC}, id ASC
           LIMIT $2

      with `LIMIT` defaulting to 200 when `limit ≤ 0` (per CLAUDE.md
      "always LIMIT unbounded queries"). Decode each row's
      `binary_proto` into `*models.ChatMessage`. Log via
      `logging.LoggerWithContext(ctx)` with the standard
      `conversation_id` field.
- [ ] In `server/services/chat/streaming.go:54`, replace the
      `s.storage.QueryByField(...)` call with
      `chatmessagestorage.ListByConversation(ctx, s.storage, req.Msg.ConversationId, true /*asc*/, 0 /*default limit*/)`.
      Drop the type assertion loop's `msg.(*models.ChatMessage)`
      since the helper returns the typed slice directly.
- [ ] Add `TestStreamMessages_BacklogReturnedInChronologicalOrder`
      under `server/services/chat/streaming_test.go`. Insert three
      messages into a single conversation in a deliberately
      non-chronological order (use `clock.WithSimulationTime` to set
      `sent_at_unix_sec` to t+30, t, t+15 in that insert order), open a
      stream, drain the backlog, and assert the received order is t,
      t+15, t+30.
- [ ] Add `TestChatMessageStorage_ListByConversation_OrdersBySentAt`
      under `server/storage/chat_message_test.go` covering ASC and
      DESC, plus respect of the `LIMIT` parameter and the
      soft-deleted-row filter. Use `storage.SetupTestStorage(t)` per
      CLAUDE.md.
- [ ] These are ordinary unit tests — do **not** add the
      `//go:build integration` tag (that tag is reserved for tests
      calling external AI APIs).

**Validates:** `npm run build` passes; new tests fail without the
streaming-backlog change and pass with it.

### Phase 2: Client defensive sort

**Goal:** Make the Flutter ViewModel re-sort messages on every state-merge
so any out-of-order arrival self-corrects on the next render.

Per `docs/client/architecture.md`, all message ordering belongs in the
ViewModel layer (`conversation_messages_notifier.dart`); widgets are thin
and never re-sort domain data. The `message_list.dart` TIME_PROPOSED ordering
is local to the poll-banner correlation and stays as-is.

**Tasks:**
- [ ] In `app/lib/presentation/viewmodels/conversation_messages_notifier.dart`
      add a private helper:

          List<ChatMessage> _sortByChronology(List<ChatMessage> msgs) {
            final sorted = [...msgs];
            sorted.sort((a, b) {
              final cmp = a.sentAtUnixSec.compareTo(b.sentAtUnixSec);
              if (cmp != 0) return cmp;
              return a.messageId.compareTo(b.messageId);
            });
            return sorted;
          }

      Stable sort on `(sentAtUnixSec, messageId)` ascending so display
      order matches conversation order. Returns a new list so callers
      can keep their `state.copyWith(messages: ...)` pattern.
- [ ] Call `_sortByChronology` at every state-merge point:
      - `loadMessages` — after the `newMessages` list is built, before
        the `state = state.copyWith(messages: newMessages, ...)` write
        (around line 123 today).
      - `_handleStreamedMessage` — after the `..add(chatMessage)`
        append paths for both the temp-replacement branch and the
        plain-append branch (around lines 315 and 378). Build the new
        messages list, then sort, then assign.
      - `_mergeServerHistory` — after the `[...state.messages, ...newMessages]`
        merge (around line 715). This is the path pull-to-refresh
        exercises today and the one where the corrupted local order
        survives.
      - `refreshAfterResume` — already delegates to
        `_mergeServerHistory`, so it's covered transitively. No direct
        change needed; verify by reading the code.
- [ ] Add `app/test/presentation/viewmodels/conversation_messages_ordering_test.dart`
      following the existing `FakeChatService implements ChatService`
      hand-written-fake pattern from
      `conversation_view_model_test.dart` (per
      `docs/client/testing.md` ViewModel-first guidance — not
      Mockito). Tests:
      - `loadMessages sorts deliberately-shuffled history into chronological order by sentAtUnixSec`.
      - `streamed messages arriving out of order are reordered (a t+30 message followed by a t+10 message renders t+10, t+30)`.
      - `_mergeServerHistory after a pull-to-refresh re-sorts existing local state, not just the newly-merged tail` — this is
        the regression test for the actual user-reported bug.
      - `optimistic message stays anchored at the bottom (its
        sentAtUnixSec = now() is the largest in the conversation) and
        is replaced cleanly when the server echo arrives`.
      Override providers via
      `ProviderContainer(overrides: [chatServiceProvider.overrideWithValue(fake)])`
      per `docs/client/testing.md` §1.

**Validates:** `cd app && flutter test --reporter expanded` (per CLAUDE.md);
new ordering tests fail without the client changes and pass with them.
`npm run lint:dart:size` unaffected (no file crosses 1,000 lines).

### Phase 3: Documentation

**Goal:** Lock in the new server-side and client-side invariants so the
next person who edits these files doesn't re-introduce the bug.

**Tasks:**
- [ ] Add a one-paragraph note to `server/services/chat/README.md`
      stating that any code path returning a list of `ChatMessage`s
      must use `chatmessagestorage.ListByConversation` (which has
      `ORDER BY sent_at_unix_sec`); the bare
      `storage.QueryByField(..., "conversation_id", ...)` returns
      heap order and must not be used for user-visible message lists.
      Reference this issue (#1826) for context.
- [ ] Add a one-paragraph note to
      `app/lib/presentation/viewmodels/README.md` (or
      `docs/client/chat.md` if that's the canonical chat doc) stating
      that `conversation_messages_notifier.dart` is the sole owner of
      chronological order; every state-merge point must re-sort, and
      widgets must not assume any particular order.
- [ ] Verify `npm run lint:ephemeral-comments` clean (no phase-marker
      comments per CLAUDE.md).

**Validates:** Docs match code; no lint regressions.

## Related Issues

- The latent same-second tie-breaking bug at
  `server/services/chat/messages.go:254-265` (v4 UUID is the
  secondary sort key, which is not chronological) is **out of scope
  here** but should be filed as a follow-up issue once this PR is
  merged. The previously-recommended Option B (add
  `sent_at_unix_milli`) is the right fix when that issue gets a user
  signal; the prior revision of this plan in Git history has the full
  design.

---

## Audit Notes

The plan was audited against the architectural reference docs and
CLAUDE.md. Adjustments applied:

- **Reporter correction (PR #1829 comment):** The previous revision
  recommended Option A (add `sent_at_unix_milli`) under the assumption
  that same-second ties were the cause. The reporter clarified the
  out-of-order messages were 20 minutes apart, eliminating that
  hypothesis. This revision recenters on the unsorted server stream
  backlog (heap order) and missing client defensive sort, which both
  match the diagnostic symptoms. The milli-precision change is
  preserved as a deferred Option B for the latent same-second
  tie-breaking bug.
- **Server SQL conventions (CLAUDE.md "always LIMIT unbounded queries"):**
  The new `ListByConversation` helper always passes a limit, defaulting
  to 200 when caller passes ≤0.
- **Server logging (`docs/server/observability.md`):** The new storage
  helper logs via `logging.LoggerWithContext(ctx)` with the standard
  `conversation_id` field.
- **Client testing pattern (`docs/client/testing.md`):** Uses the
  existing `FakeChatService implements ChatService` hand-written-fake
  pattern, not Mockito. Overrides via
  `ProviderContainer(overrides: [...])`.
- **Client architecture (`docs/client/architecture.md`):** All ordering
  logic lives in the ViewModel; widgets do not re-sort. New doc note
  in Phase 3 makes that invariant explicit.
- **No proto changes:** This revision deliberately avoids proto/codegen
  changes to keep the user-facing fix small and revertible. The Phase
  3 Phase-1 milli-precision plan from the prior revision has been
  preserved as Option B (deferred) for the latent same-second issue.
- **Server testing (CLAUDE.md):** The new chat ordering tests do
  **not** use the `//go:build integration` tag (reserved for tests
  calling external AI APIs).
- **No accessibility / i18n surface affected:** This change does not
  add user-facing strings.
- **Caching layer (`docs/client/caching.md`):** No changes. The chat
  repository's existing `refreshConversationHistory` cache
  invalidation continues to drive `_mergeServerHistory` from the
  freshest server data.
- **Service independence (`docs/server/architecture.md`):** No new
  cross-service dependencies; the new helper lives in
  `server/storage/` and is consumed by the chat service only.
