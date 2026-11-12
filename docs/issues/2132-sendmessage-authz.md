# Issue #2132: Security: ChatService.SendMessage skips participant/community-member authorization check

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2132
**Filed:** 2026-05-23 | **Last activity:** 2026-05-23
**Labels:** bug, security, server
**Assignees:** unassigned
**Context docs:** none

> **Audit (2026-05-23):** Plan reviewed against `docs/server/architecture.md`
> and `docs/server/observability.md`. No changes required:
>
> 1. **Authorization-helper shape matches the architecture doc.**
>    `docs/server/architecture.md:330-396` explicitly endorses
>    "Option 2: Helper functions in same package" for service-specific
>    authorization — exactly what `requireConversationAccess`
>    (`server/services/chat/helpers.go:18`) is. The recommended fix
>    drops in the existing helper rather than inventing a new pattern.
> 2. **Error-code conventions match the observability doc.**
>    `docs/server/observability.md` does not require any additional
>    log emission on a `CodePermissionDenied` return. The existing
>    `logging.LoggerWithContext(ctx)` setup at the top of
>    `SendMessage` (lines 30-34) attaches `user_id` and
>    `conversation_id` before any access check, so the rejected-path
>    log context is already complete; the helper itself returns a
>    `connect.Error` that propagates without extra logging boilerplate.
> 3. **Client docs not applicable.** This is a server-side
>    authorization fix; the Dart client only reacts to the resulting
>    Connect error code (no behavior change beyond `NotFound` →
>    `PermissionDenied` for the unauthorized case, addressed in
>    Phase 2).

## Problem Statement

`ChatService.SendMessage` (`server/services/chat/messages.go:21-153`) is the only
writer in the chat surface that does not verify the caller is allowed to post to
the conversation. The handler calls `auth.RequireAuth(ctx)` and immediately
loads the conversation with `s.storage.GetByID(ctx, req.Msg.ConversationId, …)`
(`messages.go:43-46`), then proceeds straight to mention processing, message
insert, conversation timestamp update, request-state transition, and chat-bus
publish. There is no participant check, no community-membership check, and no
soft-archive guard.

Every adjacent RPC in the same package routes its access check through
`requireConversationAccess` (`server/services/chat/helpers.go:18`), which
returns the loaded conversation after confirming the caller is either (a) a
participant or (b) a member of any community the topic is currently shared
with via the `CommunityGear` / `CommunityExperience` / `CommunityRequest`
fan-out collected by `collectSharedCommunityIDs`
(`server/services/chat/authorization.go:36`). The three sibling read paths use
it correctly — `GetConversationHistory` (`messages.go:193`), `MarkMessagesRead`
(`messages.go:278`), and `StreamMessages` (`streaming.go:41`) — as does the
creation path, which enforces the equivalent invariant via
`auth.RequireMemberOfActiveCommunity` in `StartConversation`
(`conversations.go:185`). `SendMessage` is the only RPC missing the gate.

Exploitation requires only a known `conversation_id` (UUID, but easy to learn
for any user who was once a participant and is now a former member) and a
valid auth token. With both, an attacker can persist arbitrary messages with
their own `SenderId`, manipulate the conversation's `LastMessageText` preview
for legitimate participants, advance the request state machine from `ACTIVE`
to `OFFERS_RECEIVED` for request-topic conversations
(`maybeTransitionRequestState`, `messages.go:123`), trigger push notifications,
and — via `processMentions` (`mention_handler.go:24-118`) — add new
participants to the conversation by `@`-mentioning users who happen to be in
the conversation's community. The conversation's `ParticipantIds` list is
mutated by an unauthorized actor.

The existing test `TestSendMessage_NonParticipantCanSend`
(`messages_test.go:16`) accidentally documents the gap: it asserts "any
community member may comment" but its fixture pre-creates the "stranger" user
as a community member via `createTestCommunityMembership(...)`. There is no
test that exercises the case where the sender is **neither** a participant
**nor** a member of any community the topic is shared with. That missing
invariant is exactly the production hole.

## Context from Comments

One comment from a maintainer (`@briangbrown`, 2026-05-23): `@claude plan`.
That selects the plan path for this run. There is no design pushback or
counter-proposal in-thread, so the issue body's recommended fix is treated
as the working direction and this plan justifies (rather than re-litigates)
it.

## Potential Solutions

### Option A: Drop in `requireConversationAccess` at the top of `SendMessage` (the issue's recommendation)

**Approach:** Replace the bare `GetByID` block at `messages.go:43-46` with
a call to the existing `s.requireConversationAccess(ctx,
req.Msg.ConversationId, authInfo.UserID)` helper, using its returned
`conversation` value for the rest of the handler (sender fetch, mention
processing, message insert, timestamp update, request-state transition, chat
bus publish). Re-order the empty-message validation
(`messages.go:37-40`) to run **before** the access check so the
`InvalidArgument` semantics of `TestSendMessage_EmptyMessageRejected`
(`messages_test.go:164`) are preserved. Add a regression test
`TestSendMessage_NonMemberRejected` covering the missing case (authenticated
caller, not a participant, not a member of any community the topic is
shared with → `connect.CodePermissionDenied`).

- **Pros:**
  - One-line behavioral change matched to the rest of the chat surface,
    using the helper that already exists and is already covered by tests
    in `authorization_test.go` and `messages_test.go`.
  - Error-code shape (`NotFound` for missing conversation,
    `PermissionDenied` for unauthorized sender) lines up exactly with
    `GetConversationHistory`, `MarkMessagesRead`, and `StreamMessages`.
  - Removes the duplicate `GetByID` (the helper already loads the
    conversation), so net code is smaller, not bigger.
  - No proto change, no migration, no client change. CI's existing chat
    test surface validates the fix.
- **Cons:**
  - One more SQL fan-out call on every message send: gear/experience/
    request topics issue a `QueryByField` against
    `CommunityGear`/`CommunityExperience`/`CommunityRequest` to collect
    shared communities, plus up to one `RequireMemberOfActiveCommunity`
    lookup. Read-side RPCs (`GetConversationHistory`, `StreamMessages`)
    already pay this cost; write-side now matches. Not a regression
    relative to the read surface.
- **Effort:** Small.

### Option B: Tighten to participant-only (`requireParticipant`)

**Approach:** Use `s.requireParticipant` (`authorization.go:14`) instead of
`requireConversationAccess`, so only users already listed in
`conversation.ParticipantIds` may post.

- **Pros:**
  - Tightest possible gate; removes the "any community member may post"
    behavior entirely.
- **Cons:**
  - Breaks the explicitly-coded policy: existing test
    `TestSendMessage_NonParticipantCanSend` (`messages_test.go:16`)
    asserts that a community member who is *not* a participant **can**
    send a message, the same policy `addUserAsParticipant`
    (`conversations.go:102`) materializes when a community member posts.
    The conversation surface treats community membership as the inner
    ring of access; participant membership is a derivative bookkeeping
    list. Tightening to participant-only is a product-policy change, not
    a security fix.
  - Misaligned with sibling RPCs: read paths use
    `requireConversationAccess`, so write would be stricter than read,
    which is the wrong asymmetry (write should be at most as permissive
    as read, never *less*).
- **Effort:** Small in code; medium-to-large in product-policy review.

### Option C: Add a new, write-specific check helper

**Approach:** Introduce a third helper (e.g. `requireWriteAccess`) layered
on `requireConversationAccess` that additionally checks `Archived` /
`Deleted` state on the conversation, the topic's gear/experience/request
soft-delete state, and any future write-specific gates. Use it from
`SendMessage` (and migrate `MarkMessagesRead` separately).

- **Pros:**
  - Lets `SendMessage` enforce additional invariants (archived/soft-deleted
    parents) that read-side does not need.
- **Cons:**
  - The dangling `checkArchivedConversationAccess` doc comment at
    `messages.go:437-439` shows this helper was intentionally **not**
    landed yet — archival of chat-bearing parents is not currently a
    blocker for write (verify in code; no callers exist). Bundling
    archival policy with the missing-access-check fix conflates one
    security bug with one open product question.
  - Yet another access helper in a package that already has
    `requireParticipant`, `requireConversationAccess`, and
    `addUserAsParticipant`. Adds surface area without clear current
    payoff.
- **Effort:** Medium (new helper + tests + decision on archival
  semantics).

## Recommendation

**Option A.** The issue identifies a discrete missing access check on one RPC
where every sibling RPC routes through the same helper. The fix is to call
that helper, not to redesign the access model. It:

1. Matches the explicit recommendation in the issue body,
2. Reuses an existing helper that already has independent test coverage,
3. Keeps the error-code shape and the "community-member may post" product
   policy unchanged,
4. Removes a duplicate `GetByID`, and
5. Closes the regression test gap with one focused new test.

Archival semantics (`checkArchivedConversationAccess` placeholder at
`messages.go:437-439`) and the per-write fan-out cost of
`collectSharedCommunityIDs` are real follow-up topics, but neither is on the
critical path for closing the security hole. File them separately if
needed.

## Open Questions

- **Where exactly does the access check go relative to the empty-message
  validation?** — Recommendation: keep the empty-message validation
  (`messages.go:37-40`) where it is, **before** the access check.
  `TestSendMessage_EmptyMessageRejected` (`messages_test.go:164`)
  asserts `connect.CodeInvalidArgument` for an empty-body message, which
  is information that doesn't depend on conversation membership; running
  the cheap input validation before the SQL-side access check both
  preserves the existing error code and avoids a needless DB round-trip
  on malformed requests. The access check then replaces the
  `s.storage.GetByID(...)` block at `messages.go:43-46`.

- **Should the test for the new behavior assert
  `CodePermissionDenied` or `CodeNotFound`?** — Recommendation:
  `CodePermissionDenied`. `requireConversationAccess` returns
  `CodeNotFound` only when the conversation row doesn't exist
  (`helpers.go:21-23`); for an existing conversation the unauthorized
  user is not a participant of, the helper returns
  `CodePermissionDenied` (`helpers.go:42-53`), matching what
  `GetConversationHistory` and `MarkMessagesRead` already return for
  the same shape of caller. Test the existing-conversation case so the
  assertion is meaningful.

- **Do we need an additional test for the request-state-transition side
  effect?** — Recommendation: no separate test for this fix.
  `maybeTransitionRequestState` (`messages.go:123`,
  `messages.go:393-435`) is called *after* the access check, so the
  same `CodePermissionDenied` rejection prevents the state transition
  outright; a dedicated test would be testing the access check twice.
  If `maybeTransitionRequestState`'s logic itself needs coverage, that
  belongs in a separate test for that function, not this PR.

- **Do we need to worry about a participant who was a community member
  at conversation start, was then removed from the community, and now
  posts?** — Recommendation: out of scope here.
  `requireConversationAccess` short-circuits on participant membership
  (`helpers.go:25-30`) **before** the community check, so a stored
  participant continues to have write access even after being removed
  from the community. That's the existing behavior on the read side and
  the participant-cleanup path (`validateAndCleanParticipants`,
  `conversations.go:30`) defers community-membership pruning to specific
  call sites — not to every read/write. Changing this would change
  product behavior for *every* RPC, not just `SendMessage`, and is a
  separate decision.

- **Performance cost?** — Recommendation: accept it. Each `SendMessage`
  now issues at most one extra `QueryByField` (the fan-out across
  `CommunityGear` / `CommunityExperience` / `CommunityRequest`) and one
  `RequireMemberOfActiveCommunity` lookup for non-participants. Read-side
  RPCs already pay this cost on every call; the write path matching is
  not a new latency floor. If profiling later shows this is hot, the
  optimization (cache the fan-out, batch the membership lookup) lifts
  every chat RPC, not just `SendMessage`.

## Implementation Plan

### Phase 1: Replace bare `GetByID` block with `requireConversationAccess` and add the missing regression test
**Goal:** Close the security hole and lock the invariant down with a test
that fails today.
**Tasks:**
- [ ] In `server/services/chat/messages.go`, edit `SendMessage` so the
  body, top-to-bottom, becomes:
  1. `authInfo, err := auth.RequireAuth(ctx)` (unchanged, lines 25-28).
  2. `logger := logging.LoggerWithContext(ctx).With(...)` (unchanged,
     lines 30-34).
  3. Empty-message validation: keep the existing
     `if req.Msg.Text == "" && len(req.Msg.MediaIds) == 0 { ... }`
     block at lines 37-40 in place.
  4. **Replace** the existing `conversation := &models.ChatConversation{}`
     + `s.storage.GetByID(...)` block at lines 42-46 with:
     ```go
     conversation, err := s.requireConversationAccess(ctx, req.Msg.ConversationId, authInfo.UserID)
     if err != nil {
         return nil, err
     }
     ```
     `requireConversationAccess` already loads the conversation and
     returns it, so the rest of the handler keeps using the same
     `conversation` variable. No subsequent line in `SendMessage`
     needs to change.
- [ ] Confirm no other file in the chat package relies on the duplicate
  `GetByID` shape (it's a local variable in one function — should be a
  no-op, but verify with `grep -n "SendMessage" server/services/chat`).
- [ ] In `server/services/chat/messages_test.go`, add
  `TestSendMessage_NonMemberRejected`:
  - Set up a community with `lender1` and `borrower1` as members; create a
    transfer-topic conversation between them via `StartConversation`.
  - Create a second user `outsider` via `createTestUsers` but **do
    not** create a community membership for them in this community or
    any other community that shares the transfer's topic.
  - Call `svc.SendMessage` with the outsider's auth context and the
    new conversation's ID.
  - Assert the error is non-nil and `connect.CodeOf(err) ==
    connect.CodePermissionDenied`.
  - Assert no message was inserted (query
    `s.storage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})`
    and check the count is unchanged from the immediate post-
    `StartConversation` baseline — `StartConversation` inserts a
    system message, so the test must capture the pre-call count, not
    assert zero).
- [ ] Leave `TestSendMessage_NonParticipantCanSend`
  (`messages_test.go:16`) unchanged — it documents the
  community-member-may-post policy, which Option A preserves.
- [ ] `npm run generate` if any proto files were touched (none expected),
  then `npm run build` to run `golangci-lint` + `go build ./...`.
- [ ] `go test -tags=integration -timeout 20m
  ./server/services/chat/...`. The new test must fail on `main` and pass
  on this branch (verify the "fails on `main`" claim by cherry-picking
  the test alone and running before applying the fix). The pre-existing
  `TestSendMessage_NonParticipantCanSend`,
  `TestSendMessage_StoresMessageCorrectly`,
  `TestSendMessage_UpdatesConversationTimestamp`, and
  `TestSendMessage_EmptyMessageRejected` must all stay green.
- [ ] Run the broader chat package test set:
  `go test -tags=integration -timeout 20m
  ./server/services/chat/... ./server/auth/... ./server/storage/...`
  to cover the helpers exercised by the fix.
**Validates:** The handler now refuses unauthorized senders with
`CodePermissionDenied`, matches the rest of the chat surface, and the
test gap identified in the issue is closed.

### Phase 2: Confirm no caller depends on the duplicate `GetByID` semantics
**Goal:** Guard against subtle behavior drift from removing the bare
`GetByID` (which returned `CodeNotFound` on any missing row, whereas
`requireConversationAccess` returns `CodeNotFound` only for the same
case and `CodePermissionDenied` otherwise).
**Tasks:**
- [ ] Search the Dart client for places that branch on
  `NotFound` vs `PermissionDenied` in the `SendMessage` error path:
  `grep -rn "SendMessage" app/lib`. Confirm callers either re-throw
  generically or only inspect the error message; if any caller
  specifically catches `NotFound` to mean "conversation gone, drop
  draft," document it in this plan (and possibly tighten the helper
  to keep `NotFound` for the missing-conversation case — which it
  already does at `helpers.go:21-23`, so this is more verification
  than change).
- [ ] Cross-check `server/services/chat/streaming.go`,
  `server/services/chat/messages.go::GetConversationHistory`, and
  `server/services/chat/messages.go::MarkMessagesRead` to confirm
  they have not had bespoke error-code mapping added — they're the
  reference behavior for the now-aligned `SendMessage`.
**Validates:** No client-visible regression from the error-code
realignment.

### Phase 3: Document the invariant
**Goal:** Make the "every chat-mutating RPC must route through
`requireConversationAccess`" rule visible to future contributors so this
class of bug is harder to reintroduce.
**Tasks:**
- [ ] Append a short paragraph to `server/services/chat/README.md`
  under an "Access control" subheading (creating it if needed) stating:
  "Every RPC that reads or mutates a `ChatConversation` or its
  `ChatMessage` children must call `requireConversationAccess` (or
  `requireParticipant` where strict participant membership is
  required) before doing any work. The only exception is
  `StartConversation`, which uses
  `auth.RequireMemberOfActiveCommunity` because no conversation exists
  yet." Cite this issue (#2132) in the paragraph as the precedent.
- [ ] Confirm no other RPC in `server/services/chat/` is missing the
  check by grepping for `auth.RequireAuth` and inspecting the next
  ~20 lines of each match:
  `grep -n "auth.RequireAuth" server/services/chat/*.go`. If any other
  handler is missing the gate, flag it in this plan, file a separate
  issue, and do **not** widen the scope of this PR. (Initial scan
  during plan-writing showed only `SendMessage` is affected, but
  re-verify in the implementation pass.)
**Validates:** The rule lives in the package README, future
contributors land their new RPCs with the helper in place, and any
discovered sibling gap goes to its own tracked work.

## Related Issues

- #2127 — `StartConversation` rate-limiting / chat abuse follow-up
  (sibling chat-surface security review item, if open). Not a blocker;
  cross-reference for context if filed.
- The internal security review referenced in the issue description
  ("first batch of RPCs in `proto/ripls/api/` directory order") may
  produce more issues in the same shape; this plan is the template
  for any that surface (drop in the existing
  `requireConversationAccess` / `RequireMemberOfActiveCommunity`
  helper, add the missing regression test, document the invariant).
