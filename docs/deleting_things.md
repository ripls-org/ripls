---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: The project-wide deletion model — user-directed soft deletion via DeletedMetadata, cascade deletion of conversations and media, per-type rules, and the future hard-deletion plan for GDPR purges.
  globs: [server/storage/cascade_delete.go, server/storage/hard_cascade_delete.go, server/storage/cascade_leaver.go]
  triggers: [soft-delete, hard-delete, cascade-delete, deleted-metadata, gdpr, anonymization, restore]
  lens: [domain, server, architecture]
  domain: content
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Item Deletion

Ripls is partly a content creation platform: users create communities and fill them with things to share, requests for help, and experiences to have together. But like any workshop or studio, Ripls needs mechanisms to clean things up, to keep things tidy, fresh, and to make room for the new.

## Summary: Soft Deletion and Hard Deletion

Deletion in Ripls can be "soft" or "hard."

### Soft Deletion

- Ripls supports user-directed _soft deletion_ of any content they create: gear, requests, experiences, transfers, locations, media, communities, and their own account.
- Soft deletion means that the content is hidden from the user and from others but still exists in the Ripls backend database.
- Soft deleted items maintain historical linkages (e.g., gear with transfers, communities with items, items with skills).
- From the user's perspective, soft deletion is permanent and irreversible. Restoration may be added in the future.
- Only the creator of content can delete it. There are no community admins with deletion powers; content reporting is a separate feature.

Soft deletion is implemented with a metadata field `deleted`, absent until the item is deleted, that records the actor who deleted the item and when.

### Hard Deletion (Future Work)

- Hard deletion means actually removing items from the Ripls database.
- Required for GDPR/privacy compliance and removing policy-violating content.
- Hard deletion replaces references to deleted items with reserved UUIDs (one per deletable item type) to indicate deletion, preserving referential integrity.
- Implementation follows soft deletion.

## Implementation Priority

1. **Core items**: Gear, requests, experiences, transfers, and associated media
2. **User deletion**: Account deletion with content anonymization
3. **Community deletion**: With creator transfer logic
4. **Hard deletion**: Future work

## Deletable Item Types

### Gear, Requests, Experiences

These are the primary content types. Deletion behavior:

- Creator confirms deletion intent via UI
- Server sets `deleted` metadata with actor ID and timestamp
- Item disappears from all listings, searches, and detail views
- Associated media becomes inaccessible (but remains in storage)
- Skills associations are preserved for historical record

**Active transfers**: If gear has an active loan, warn the user that deletion will cancel the loan. If they proceed, mark the loan as cancelled before deleting the gear.

### Transfers (Loans and Giveaways)

- Only the transfer initiator can delete/cancel a transfer
- Active loans are cancelled; giveaways are revoked
- The underlying gear is unaffected
- Notification sent to the other party

### Locations

- Locations can be soft-deleted by their creator
- Items referencing a deleted location should display gracefully (e.g., "Location unavailable")

### Media

- Media can be deleted via the shared media carousel component
- Deleting media removes it from all items that reference it
- Media files remain in storage (for hard deletion later) but become inaccessible

### Conversations

Conversations cannot be directly deleted by users. Each conversation is associated with a topic (an item, transfer, request, etc.). When that topic is deleted, the conversation is cascade-deleted along with it and cannot be retrieved. See [Cascade Deletion](#cascade-deletion) below.

### Cascade Deletion

When items are soft-deleted, certain related entities are also cascade-deleted to maintain consistency and prevent orphaned data from appearing in listings:

**Conversations**: Gear, requests, and experiences can each have associated conversations. When the parent item is deleted:

- All conversations linked to the item (via `gear_id`, `request_id`, or `experience_id` topic fields) are soft-deleted
- The same `DeletedMetadata` (user ID and timestamp) is applied to the cascade-deleted conversations
- Deleted conversations no longer appear in inbox listings

**Media**: Gear, requests, and experiences can have associated media (photos, images). When the parent item is deleted:

- All media referenced by the item's `media_ids` field are soft-deleted
- Media files remain in bucket storage (for potential hard deletion later) but become inaccessible
- Deleted media no longer appears in search results or detail views

**Why cascade delete media?**

- Media uploaded for an item is contextually tied to that item
- Without cascade deletion, orphaned media would remain accessible via direct URL or could reappear if caching issues occur
- Users expect that deleting an item removes all associated content
- Media remains recoverable (soft-deleted) until hard deletion is implemented

### Skills

Skills are system-created and are out of scope for user-initiated soft deletion. Skills can only be hard-deleted by administrators for compliance or policy reasons.

## User Deletion

When a user deletes their account:

1. **Content anonymization**: All content they created (gear, requests, experiences, transfers) is retained but anonymized:

   - Creator/owner references point to a reserved "deleted user" UUID
   - Display name shown as "Former Member" or similar
   - Profile photo removed

2. **Community ownership transfer**: For each community they created:

   - Ownership transfers to the next oldest member (by join date)
   - If no other members exist, the community is soft-deleted

3. **Active transfers**: All active loans where they are lender or borrower are cancelled with notification to the other party.

4. **User record**: The user record itself is soft-deleted, retaining the account for potential hard deletion later.

## Community Deletion

Community deletion has its own design and behavior spec at
[`community_delete_and_leave.md`](./community_delete_and_leave.md).
That doc covers the 30-day soft-delete window, the restore flow,
the snapshot eligibility set, the leave-with-ownership-handoff flow,
the sole-member-leave conversion, and the rejoin window — none of
which are captured by the simple soft-delete model used elsewhere
in this document. Read the spec before changing any community
lifecycle code.

Quick summary:

- The community record is soft-deleted with `DeletedMetadata`.
- Member IDs at delete time are snapshotted into
  `Community.deleted_snapshot.member_user_ids`; only those users
  can restore the community within 30 days.
- Cascade soft-deletes apply to community-level join rows
  (`CommunityGear`, `CommunityRequest`, `CommunityExperience`,
  `CommunityNotificationPreferences`, `CommunityInvitationLink`).
- The restorer becomes the new owner regardless of who deleted it.
- Leaving a community runs a separate leaver-scoped cascade and
  soft-deletes the leaver's `CommunityUser` row, preserving a
  30-day rejoin window.
- After 30 days, a daily job hard-deletes the community and its
  cascade.

## Server Implementation

### Deleted Metadata Structure

```proto
message DeletedMetadata {
  string deleted_by_user_id = 1;      // User ID of the actor who deleted
  int64 deleted_at_unix_sec = 2;
}
```

This is added as an optional field to each deletable model type.

### Query Filtering

All list/search queries must filter out soft-deleted items by default:

- Add `WHERE deleted IS NULL` (or equivalent) to storage queries
- Consider adding an admin flag to include deleted items for debugging

### Delete RPC Pattern

Each deletable type gets a `Delete{Type}` RPC:

- Validates the caller owns the item
- Handles pre-deletion side effects (cancel loans, transfer ownership)
- Sets the `deleted` metadata
- Invalidates relevant caches

## Client Implementation

### UI Patterns

- Delete actions accessible via item detail screens (e.g., overflow menu)
- Confirmation dialog explaining consequences (especially for items with active relationships)
- For gear with active loans: explicit warning that the loan will be cancelled
- For user deletion: multi-step flow with clear explanation of what happens to their content

### Media Carousel Enhancement

The shared media carousel component should be enhanced to support:

- Delete action for each media item (owner only)
- Confirmation before deletion
- Graceful handling when media becomes unavailable

### Cache Invalidation

After deletion, invalidate:

- The deleted item's cache entry
- Any list caches that might contain the item
- Related items' caches if relationships changed (e.g., cancelled loans)

## Edge Cases

- **Deleting the only community member**: Community is soft-deleted (no one to transfer to)
- **Deleting gear referenced in completed transfers**: Transfers retain historical reference; gear shows as deleted in transfer history
- **Concurrent deletion**: If two requests try to delete the same item, the second should succeed idempotently
- **Deleted item in deep links**: Show a consistent "This item has been removed" screen rather than a hard error. This applies to all navigation paths: deep links, in-app links, and stale references.
