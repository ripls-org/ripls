# Issue #289: Create top-level ConversationTopic proto type

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/289
**Filed:** 2025-11-14 | **Last activity:** 2026-03-26
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The concept of a "conversation topic" — the entity a conversation is about (transfer, request, experience, or gear) — is duplicated as a `oneof` block in at least 5 separate proto messages across 3 proto files, plus a hand-rolled Go struct in `server/chat/conversations.go`. Each copy uses the same 4 fields but with **different field numbers** and **inconsistent coverage** (e.g., `StartConversationRequest` and `GetConversationHistoryResponse` are missing `gear_id`).

This duplication causes:
1. **Inconsistency bugs** — missing topic types in some messages.
2. **Boilerplate** — every Go service that touches conversations has repetitive switch statements to convert between the different topic oneof representations.
3. **Maintenance risk** — adding a new topic type requires changes across 5+ proto messages and dozens of Go/Dart switch sites.

The issue author's comment specifies the exact shape of the new `ConversationTopic` message and asks for versions in both the `api` and `models` proto packages.

## Context from Comments

- **Triage (tescobar, 2026-03-05):** Rated P3, categorized as code organization. Not blocking features but would clean up the chat module.
- **Specification (maintainer, 2026-03-26):** Provided the exact message definition with a `oneof topic_id` containing `transfer_id` (3), `request_id` (4), `experience_id` (8), `gear_id` (9). Notes that both api and models packages need a version, and that it should replace overlapping fields in `conversation_service.proto`, `chat.proto`, `chat_service.proto`, plus Go and Dart code.

## Current State

### Proto messages with topic oneof (field numbers shown)

| Message | File | Fields |
|---|---|---|
| `ChatConversation` | `models/chat.proto` | transfer=3, request=4, experience=8, gear=9 |
| `StartConversationRequest` | `api/chat_service.proto` | transfer=2, request=3, experience=4 (**no gear**) |
| `GetConversationHistoryResponse` | `api/chat_service.proto` | transfer=2, request=4, experience=5 (**no gear**) |
| `ConversationItem` | `api/chat_service.proto` | transfer=2, request=7, experience=10, gear=12 |
| `ConversationContext` | `api/conversation.proto` | transfer=10, request=11, experience=12, gear=13 |

### Go struct (`server/chat/conversations.go:16`)

```go
type ConversationTopic struct {
    TransferID   string
    RequestID    string
    ExperienceID string
    GearID       string
}
```

Manual validation in `CreateOrGetConversation()` ensures exactly one field is set. Multiple switch statements across `server/services/chat/` convert between this struct and the various proto oneofs.

### Dart usage

`conversation_view_model.dart` has ~30 `hasTransferId()`/`hasRequestId()`/`hasExperienceId()`/`hasGearId()` call sites switching on topic type. `inbox_item.dart` and `conversation_screen.dart` have another ~15 each. These check different proto message types (`ConversationItem`, `ConversationContext`) which will unify after the proto change.

## Potential Solutions

### Option A: Shared ConversationTopic message in both api and models

- **Approach:** Create `ConversationTopic` in both `proto/ripls/api/conversation_topic.proto` and `proto/ripls/models/conversation_topic.proto` with identical structure. Replace all inline `oneof topic` blocks with a `ConversationTopic topic` field. Replace the Go struct with the generated proto type.
- **Pros:** Follows the existing separation between api and models packages. Single source of truth per package. Eliminates all topic-related switch/conversion boilerplate. Fixes missing `gear_id` inconsistency.
- **Cons:** Two nearly-identical proto files (one api, one models). Requires field number changes in existing messages (wire-breaking for stored protos without careful migration).
- **Effort:** Medium — proto changes are small but Go/Dart update surface is wide.

### Option B: Shared ConversationTopic in models only, imported by api

- **Approach:** Define `ConversationTopic` once in `models/` and import it from `api/` protos.
- **Pros:** Single definition, zero duplication.
- **Cons:** **Violates the project's core rule** that api and models packages must never import each other. Also exposes storage-layer types to clients.
- **Effort:** Small, but architecturally prohibited.

### Option C: Migrate inline oneofs to use the new message, keeping old field numbers reserved

- **Approach:** Same as Option A, but explicitly `reserved` the old field numbers in each message to prevent accidental reuse, and assign new field numbers for the `ConversationTopic topic` field. This is wire-breaking for any messages that are persisted (only `ChatConversation` in models is stored; api messages are transient).
- **Pros:** Clean break, no ambiguity about old vs new fields.
- **Cons:** Requires a database migration for `ChatConversation` (the stored proto). Slightly more effort than Option A.
- **Effort:** Medium.

## Recommendation

**Option A** is the right approach, consistent with the issue author's specification and the project's api/models separation rule.

**Storage migration detail:** protosql flattens nested messages into prefixed SQL columns. The current inline oneof fields produce flat columns (`transfer_id`, `request_id`, `experience_id`, `gear_id`). After the change, a `ConversationTopic topic = 11` field on `ChatConversation` will produce prefixed columns (`topic_transfer_id`, `topic_request_id`, `topic_experience_id`, `topic_gear_id`). This means we need a SQL migration to rename the existing columns and a Go backfill/conversion script to update any serialized `proto_data` blobs.

For API protos (transient request/response), field number changes are safe as long as client and server deploy together.

**Practical approach:** For the models proto, replace the inline oneof with a `ConversationTopic topic` field at a **new field number** (11), mark old fields 3, 4, 8, 9 as `reserved`, and write both a SQL column-rename migration and a Go backfill script to rewrite `proto_data`. For api protos, simply replace inline oneofs with the new `ConversationTopic` field.

## Open Questions

- **Should `ConversationTopic` live in its own `.proto` file or be added to existing files?** — Recommendation: Own file (`conversation_topic.proto`) in each package, since it's a shared type used across multiple services. This follows the project pattern of separating types by domain.

- **Should `GetConversationHistory` and `StartConversation` RPCs have separate request/response types that embed `ConversationTopic`, or should they use the topic as-is?** — Recommendation: Use `ConversationTopic` directly as a field. Per project rules, each RPC keeps its own request/response type, but the shared `ConversationTopic` becomes a field within those types.

- **Field number for `ConversationTopic` in `ChatConversation`?** — Recommendation: Use field 11 and reserve old fields 3, 4, 8, 9. This is cleanest for proto evolution.

## Implementation Plan

### Phase 1: Proto definitions
**Goal:** Create the `ConversationTopic` type in both packages and update all proto messages.
**Tasks:**
- [ ] Create `proto/ripls/models/conversation_topic.proto` with `ConversationTopic` message (oneof topic_id: transfer_id=3, request_id=4, experience_id=8, gear_id=9)
- [ ] Create `proto/ripls/api/conversation_topic.proto` with identical `ConversationTopic` message
- [ ] Update `proto/ripls/models/chat.proto`: replace inline oneof in `ChatConversation` with `ConversationTopic topic = 11`, reserve fields 3, 4, 8, 9
- [ ] Update `proto/ripls/api/chat_service.proto`: replace inline oneofs in `StartConversationRequest`, `GetConversationHistoryResponse`, and `ConversationItem` with `ConversationTopic topic` fields (add `gear_id` support to StartConversation and GetConversationHistory)
- [ ] Update `proto/ripls/api/conversation.proto`: replace inline oneof in `ConversationContext` with `ConversationTopic topic` field
- [ ] Run `npm run generate` and verify compilation
**Validates:** Proto definitions compile, generated Go and Dart code builds.

### Phase 2: Storage migration and backfill script
**Goal:** Migrate existing conversation data to the new nested column layout.

protosql flattens nested messages with a prefix. The old inline oneof produces flat columns (`transfer_id`, `request_id`, etc.). The new `ConversationTopic topic = 11` field will produce prefixed columns (`topic_transfer_id`, `topic_request_id`, etc.). Both the SQL columns and the serialized `proto_data` blobs need updating.

**Tasks:**
- [ ] Write a Go backfill/conversion script in `server/jobs/conversation_topic_migration.go` that:
  1. Queries all `ChatConversation` rows
  2. For each row, reads the old flat topic columns and populates the new `topic_*` prefixed columns
  3. Re-serializes the proto with the new `ConversationTopic` field set and updates `proto_data`
  4. Runs idempotently (safe to re-run; skips rows already migrated)
- [ ] Add unit tests for the migration in `server/jobs/conversation_topic_migration_test.go`
- [ ] Wire migration as a startup job in `main.go` (same pattern as impact backfill)
- [ ] Mark the migration script with a `// TODO(#289): Delete after migration has run in production` comment so it can be removed once complete
**Validates:** Existing conversations load correctly with the new schema; migration is idempotent.

### Phase 3: Go server — chat library
**Goal:** Update the core Go chat library to use the proto type.
**Tasks:**
- [ ] Remove the `ConversationTopic` Go struct from `server/chat/conversations.go`, replace with the generated `models.ConversationTopic` proto type
- [ ] Update `CreateOrGetConversation()` to accept `*models.ConversationTopic` and set it on `ChatConversation.Topic`
- [ ] Update topic validation to use the proto oneof's `GetTopicId()` interface
- [ ] Update `server/conversation/state.go` `IsItemDone()` to switch on the new nested `ConversationTopic`
- [ ] Update unit tests in `server/chat/` and `server/conversation/`
**Validates:** Chat library compiles and tests pass; conversations can be created and queried.

### Phase 4: Go server — service layer
**Goal:** Update all RPC service implementations to use the new proto type.
**Tasks:**
- [ ] Update `server/services/chat/conversations.go`: replace all topic oneof switch statements with `ConversationTopic` field assignments (ListConversations, GetConversation, GetConversationHistory, StartConversation, GetConversationFor* RPCs)
- [ ] Update `server/services/chat/context.go`: use `ConversationTopic` for context population
- [ ] Update `server/services/transfer/interest.go`: construct `ConversationTopic` with `TransferId` for gear conversation creation
- [ ] Update `server/services/request/sharing.go`: construct `ConversationTopic` with `RequestId`
- [ ] Update `server/services/experience/share.go`: construct `ConversationTopic` with `ExperienceId`
- [ ] Update `server/services/community/gear_sharing.go`: construct `ConversationTopic` with `GearId`
- [ ] Add a `models→api` conversion helper in `server/chat/`: `func ModelsTopicToAPI(t *models.ConversationTopic) *api.ConversationTopic` (shared library, not in a service — per server architecture doc)
- [ ] Run `npm run build` (lint + compile)
- [ ] Run `go test ./...` to verify all server tests pass
**Validates:** All server code compiles, lint passes, tests pass.

### Phase 5: Client updates
**Goal:** Update Dart code to use the new unified `ConversationTopic` type.

Note: Dart generated code (`app/lib/data/gen/`) is auto-generated and not checked in. Only hand-written Dart code needs manual updates.

**Tasks:**
- [ ] Run `npm run generate` to regenerate Dart proto code
- [ ] Create a topic helper in `app/lib/core/utils/conversation_topic_helper.dart` to centralize topic-type checks (e.g., `isTransfer(topic)`, `isRequest(topic)`, `topicId(topic)`) — this reduces the 50+ scattered `has*Id()` call sites
- [ ] Update `app/lib/presentation/viewmodels/conversation_view_model.dart`: refactor topic-type checks to use `ConversationTopic` from the unified field via the helper
- [ ] Update `app/lib/presentation/models/inbox_item.dart`: refactor `fromConversation` and `fromConversationContext` to use unified topic field
- [ ] Update `app/lib/presentation/screens/inbox/conversation_screen.dart`: refactor topic checks
- [ ] Update `app/lib/core/utils/conversation_helper.dart`: refactor transfer check to use topic field
- [ ] Ensure no viewmodel accesses services directly for topic resolution (viewmodels call repositories only, per client architecture doc)
- [ ] Run `flutter test --reporter expanded` from `app/`
**Validates:** Client compiles, topic display works across all conversation types.

### Phase 6: Cleanup and verification
**Goal:** Remove dead code and verify end-to-end.
**Tasks:**
- [ ] Remove any remaining references to old inline topic fields in Go and Dart code
- [ ] Remove old `ConversationTopic` struct from `server/chat/conversations.go`
- [ ] Verify integration/system tests pass
- [ ] Run `npm run build` final verification
**Validates:** Clean codebase, no deprecated field usage, all tests green.

### Post-migration cleanup (after production deployment)
- [ ] Remove `server/jobs/conversation_topic_migration.go` and its test file once the migration has run successfully in production (tracked by `TODO(#289)`)

## Related Issues
- No directly related issues found. This is a standalone refactoring for code organization.

---

## Audit Notes

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`.

**Changes made during audit:**

1. **Storage migration detail added (Phase 2).** Original plan was vague about protosql implications. protosql flattens nested messages with column prefixes — moving from inline oneof (`transfer_id`) to nested message (`topic_transfer_id`) requires both SQL column renames and `proto_data` blob re-serialization. Added a dedicated migration phase with a Go backfill script, idempotency requirement, unit tests, and a `TODO(#289)` marker for post-migration deletion.

2. **Conversion helper placement fixed (Phase 4).** Moved `ModelsTopicToAPI` from the service layer to `server/chat/` as a shared library function. Per server architecture doc, services should not contain shared functionality — shared conversion helpers belong in library packages.

3. **Client architecture compliance (Phase 5).** Added explicit check that viewmodels don't bypass the repository layer for topic resolution. Added note that generated Dart code doesn't need manual edits. Promoted the optional Dart helper to a concrete task in `core/utils/` (the right layer for pure utility functions per client architecture doc).

4. **Open question removed.** Resolved the protosql wire-compatibility question in the Recommendation section with concrete details about column prefixing behavior, making the open question redundant.

5. **No i18n impact.** This refactoring doesn't add user-visible strings — it's a structural change to how topic data flows. No ARB file changes needed.

6. **No caching impact.** Chat data access doesn't currently go through a chat repository with Stash caching; conversations are fetched directly via the chat service. This is a pre-existing pattern, not introduced by this change.
