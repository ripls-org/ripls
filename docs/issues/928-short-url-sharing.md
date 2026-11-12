# Issue #928: Sharing modal should use short URLs

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/928
**Filed:** 2026-03-27 | **Last activity:** 2026-03-27
**Labels:** none | **Assignees:** tescobar
**Context docs:** docs/invitations.md

## Problem Statement

When sharing an item via the sharing modal, the URL contains a full UUID as the
"short code" instead of an 8-character code:

```
https://dev.ripls.app/go/00000000-0000-4000-8000-000000000011?experience_id=...
                        ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                        Should be 8 chars like "Xk9mN2pQ"
```

The issue affects Thomas but not Bryan because their underlying
`CommunityInvitationLink` records were created at different times — sharing an
item reuses the user's existing per-community invitation link, it doesn't
create a new one. Thomas's link predates the proto field renumbering.

Additionally, the "Copy Link" toast confirmation is likely hidden behind the
bottom sheet modal.

## Context from Comments

No comments on the issue.

## Root Cause: Proto Field Number Reuse

The root cause is a **protobuf field number reuse** in
`proto/ripls/models/community.proto` for `CommunityInvitationLink`.

### Timeline

1. **Original proto** — `invitation_token = 4` (a UUID string)
2. **Commit `fdf4c497`** — Added `short_code = 7` as a new field alongside
   `invitation_token = 4`
3. **Commit `9ce3a60a`** — Removed `invitation_token`, reserved field 4:
   ```protobuf
   reserved 4;
   reserved "invitation_token";
   ```
4. **Commit `c48d4c30`** — Moved `short_code` from field 7 to field 4,
   **reusing the reserved field number**:
   ```protobuf
   string short_code = 4;  // was invitation_token (UUID)
   ```

### Why this causes the bug

The proto-sql storage layer stores a serialized `binary_proto` blob for each
record (`server/storage/protosql.go:104-121`). When reading, it deserializes
the blob with `proto.Unmarshal()`.

Old `CommunityInvitationLink` records (created before short codes existed)
have the UUID from `invitation_token` serialized at wire field 4. With the
new proto definition where field 4 is `short_code`, deserializing these old
records maps the UUID to the `short_code` field.

The server then builds the URL using this UUID as the code:
```go
inviteURL := fmt.Sprintf("%s/go/%s", baseURL(s.inviteLinkHostname), shortCode)
// → https://dev.ripls.app/go/00000000-0000-4000-8000-000000000011
```

### Why it's user-specific

Sharing an item reuses the user's existing per-community
`CommunityInvitationLink` — it doesn't create a new link per item. Thomas's
invitation link was created **during the transition period** (between commits
`fdf4c497` and `c48d4c30`) when both `invitation_token = 4` and
`short_code = 7` existed. His `binary_proto` blob has the UUID at field 4 and
the proper 8-char code at field 7. After the field renumbering, field 4 is
read as `short_code` (yielding the UUID) and field 7 is silently discarded.

Bryan's invitation link was created **after** the renumbering, so his blob
only has the 8-char code at field 4.

The backfill logic that existed in the transitional period (commit `8433472b`)
was removed in commit `8e81d1a4` along with all `invitation_token` references.

### Secondary issue: Toast hidden behind modal

The copy-to-clipboard code (`item_share_sheet.dart:134-135`) calls
`ToastHelper.showSuccess()` which uses `ScaffoldMessenger.of(context)`. Inside
a `showModalBottomSheet`, the `ScaffoldMessenger` context may target the
modal's scaffold rather than the app's root scaffold, causing the toast to
render behind or within the modal where it's not visible.

## Potential Solutions

### Option A: Server-side backfill on read

- **Approach:** In `GetOrCreateInviteLink`, when an existing invitation is
  found, check if `ShortCode` looks like a UUID (length > 8 or contains `-`).
  If so, generate a proper 8-char short code, update the record, and return
  the new code.
- **Pros:** Self-healing — fixes data on first access. No migration needed.
  Works for all existing users automatically.
- **Cons:** Mutates data on a read path (acceptable since it's idempotent).
- **Effort:** Small.

### Option B: Database migration

- **Approach:** Write a one-time migration script that finds all
  `CommunityInvitationLink` records where `short_code` looks like a UUID and
  replaces them with proper 8-char codes.
- **Pros:** Fixes all data at once. Clean separation of migration from
  read path.
- **Cons:** Requires database access and migration tooling. Doesn't prevent
  future occurrences if the proto is reloaded from old blobs.
- **Effort:** Medium.

### Option C: Re-serialize all invitation records

- **Approach:** Read and re-write (Update) all `CommunityInvitationLink`
  records so the `binary_proto` blob is re-serialized with the current proto
  definition. Combined with a backfill of proper short codes.
- **Pros:** Fixes both the flattened column and the blob.
- **Cons:** More complex. Still need to generate new short codes for
  affected records.
- **Effort:** Medium.

## Recommendation

**Option A** — Server-side backfill on read. It's the simplest fix, requires
no migration infrastructure, and self-heals on first access. The check is
trivial (`len(shortCode) != shortCodeLength`) and the update is idempotent.

Combined with:
- Fix the toast visibility by using `rootScaffoldMessenger` context
- Update stale test fixtures

## Open Questions

- ~~**What exact URL format is Thomas seeing?**~~ **Answered:**
  `https://dev.ripls.app/go/00000000-0000-4000-8000-000000000011?experience_id=...`
  — confirms a UUID in the short_code position.
- ~~**Is the toast actually not appearing?**~~ **Answered:** Likely hidden
  behind the modal bottom sheet.

## Implementation Plan

### Phase 1: Fix server-side short code backfill
**Goal:** Old invitation records with UUID short codes get proper 8-char codes.
**Tasks:**
- [ ] In `GetOrCreateInviteLink` (`server/services/community/invitations.go`),
  after reading an existing invitation at line 72-74, add a check:
  if `len(shortCode) != shortCodeLength`, generate a new short code via
  `generateUniqueShortCode`, update the record with `s.storage.Update`, and
  log the backfill
- [ ] Add a unit test: create a `CommunityInvitationLink` with a UUID as
  `ShortCode`, call `GetOrCreateInviteLink`, verify the response contains
  an 8-char short code and the record is updated
- [ ] Run `go test ./server/services/community/...` and `npm run build`
**Validates:** Old invitation records are self-healed on first access.

### Phase 2: Fix toast visibility in share sheet
**Goal:** "Link copied" toast appears above the modal, not behind it.
**Tasks:**
- [ ] In `ItemShareSheet._showMessage()`, use
  `ScaffoldMessenger.of(context)` from the navigator's root context (or use
  `ScaffoldMessenger.maybeOf()` with a fallback) to ensure the snackbar
  renders above the modal bottom sheet
- [ ] Verify same fix is needed in `InviteSheet._showMessage()`
- [ ] Test manually: open share sheet → tap Copy Link → confirm toast visible
**Validates:** Users see confirmation when copying links.

### Phase 3: Update stale test fixtures
**Goal:** Tests use the current URL format.
**Tasks:**
- [ ] Update all test URLs in `item_share_sheet_test.dart` from
  `https://ripls.app/invite?token=abc123` to `https://ripls.app/go/abc12345`
  format (lines 74, 101, 194, 301, 333, 422)
- [ ] Update the display URL assertion at line 194 from
  `ripls.app/invite?...` to the correct `ripls.app/go/...` format
- [ ] Run `flutter test --reporter expanded app/test/presentation/widgets/sharing/`
**Validates:** Tests match production URL format.

## Architecture Audit

> Audited against: `docs/client/architecture.md`, `docs/client/caching.md`,
> `docs/client/i18n.md`, `docs/server/architecture.md`

### Finding 1: Proto field number reuse (CRITICAL — root cause)

**Severity:** Critical

Commit `c48d4c30` reused proto field number 4 (previously `invitation_token`)
for `short_code`. This violates protobuf's wire compatibility guarantees and
causes data corruption when deserializing old records. The fix in Phase 1
adds runtime backfill to heal affected records.

**Plan impact:** Phase 1 addresses this directly.

### Finding 2: Widget bypasses ViewModel layer (architecture violation)

**Severity:** Low (existing pattern, not introduced by this plan)

Both `ItemShareSheet` and `InviteSheet` are `ConsumerStatefulWidget`s that call
`ref.read(communityRepositoryProvider)` directly — violating architecture rule
#1: "ViewModels are the ONLY layer that calls Repositories."

This falls under the documented "Widget Async Exceptions" category
(dialogs/modals), so it is tolerable. The plan does not make this worse.

**Plan impact:** None — no change needed.

### Finding 3: Hardcoded strings in share sheets (i18n gap)

**Severity:** Low (pre-existing, not introduced by this plan)

`ItemShareSheet` and `InviteSheet` contain hardcoded English strings not in
the ARB files ("Link copied to clipboard", "Share Link", "Anyone with link
can join", "Revoke", "spots left", etc.).

**Plan impact:** Out of scope for this bug fix. Should be tracked separately.

### Finding 4: Cache approach not needed

The original hypothesis of stale client-side cache was incorrect. The root
cause is server-side data corruption from proto field reuse. No cache changes
are needed.

**Plan impact:** Removed cache-related tasks from the implementation plan.

## Related Issues

None identified.
