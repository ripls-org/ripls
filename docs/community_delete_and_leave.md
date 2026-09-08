---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Behavior and architecture spec for leaving a community and the owner deleting one — 30-day reversible soft delete, ownership transfer/handoff, single-owner invariant, member cleanup, and restore flow.
  globs: [server/services/community/lifecycle.go, server/storage/cascade_leaver.go, app/lib/presentation/screens/communities/**]
  triggers: [community-delete, leave-community, ownership-transfer, owner-handoff, restore, soft-delete, 30-day]
  lens: [domain, client, server]
  domain: community
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Community Delete & Leave

How a user removes themselves from a community, and how the original creator
removes the community itself. These two flows interleave: when the owner
chooses Delete, they're offered Leave-with-handoff first; when the sole
member Leaves, it converts to Delete. Both are reversible within a 30-day
window.

This document is the **behavior and architecture spec** for those flows. For
the project-wide soft/hard-deletion model, see
[`deleting_things.md`](./deleting_things.md). For the undo taxonomy this doc
references, see [`ai/undo_plan.md`](./ai/undo_plan.md).

---

## 1. Principles

1. **No admin caste.** A community has exactly one **owner**. The owner has
   *one* power that other members lack: deleting the community. Every other
   permission (post, share, RSVP, comment, leave) is the same for owner and
   non-owner members. The owner's account is not a "community admin."
2. **Delete is never destructive on the spot.** Delete is a 30-day soft
   delete. Any member who was in the community at delete time can restore
   it. Hard purge happens via a daily job (see §10).
3. **Owner cannot be trapped.** If the owner wants out, the Delete dialog
   offers a Leave-with-ownership-transfer alternative. The owner only ever
   actually deletes the community when no transfer is wanted.
4. **A community always has exactly one valid owner.** Enforced at the
   database level (see §6.1). It is impossible for a leave/transfer race
   to produce zero owners or two owners.
5. **Leaving is reversible for 30 days too.** Within 30 days of leaving,
   the leaver can rejoin without a new invite. After 30 days, they need a
   fresh invitation.
6. **Bail out, always.** Both flows have a Cancel/Stop affordance at every
   confirmation step. No irreversible action ships from a single tap.

---

## 2. User-visible behavior

### 2.1 Delete Community (owner-only)

The Danger Zone "Delete Community" entry only renders for the owner.
Non-owners never see this control.

1. Owner taps **Delete Community** in community settings.
2. App shows a **choice modal**:
   - "Delete <name> for everyone." → §2.2.
   - "Just leave — let someone else take over." → enters Leave flow with
     ownership transfer (§2.3).
   - Cancel.
3. If owner picks **Delete**:
   - Confirmation dialog summarizes the impact:
     - "<name> will be hidden for all <N> members for 30 days."
     - "Members can restore it during that time. After 30 days it's
       permanent."
     - List of *categories* of content that will become inaccessible
       (gear shared, requests posted, events, chats). No per-item
       enumeration — categories only.
   - Members are notified ("<owner> deleted <name>. You have 30 days
     to restore it.").
   - The day before the 30-day purge, all original members get a final
     reminder push that deep-links to the restore flow (§2.5).

### 2.2 Leave Community (any member)

1. Member taps **Leave**. A confirmation dialog summarizes impact (§4):
   "Your gear shared with <name>, your open requests, and your active
   transfers in this community will be cancelled. Events you host will
   stay. You can rejoin within 30 days without a new invite."
2. Cancel always available.
3. On confirm:
   - Membership row removed.
   - Cleanup runs (§4).
   - `MEMBER_LEFT` event fires (already does today).
   - Community regions recompute (already does today).
4. **If the leaver is the owner:** the dialog *first* prompts to choose a
   new owner from the current member list (§2.3). The owner cannot
   complete a leave without picking one — unless they are the sole
   member, in which case the action converts to Delete (§2.4).

### 2.3 Owner leaving with ownership transfer

1. App fetches current members (`ListCommunityUsers`), excluding the
   owner.
2. Owner picks one. The picked user must still be a member at commit
   time — re-validated server-side. If not, the server returns
   `FAILED_PRECONDITION` and the client re-prompts.
3. Server atomically (a) sets `Community.owner_user_id` to the new
   owner, (b) deletes the old owner's `CommunityUser` row. See §6.1
   for the SQL constraints that make this atomic.
4. New owner gets a push notification: "You're now the owner of
   <name>." No broadcast to other members.
5. Cleanup of leaver-owned content runs as in §4.

### 2.4 Sole-member leave

If the leaver is the only member, the Leave action morphs into a Delete
with one extra screen explaining: "You're the only member. Leaving
deletes <name> for 30 days. You can restore it from Settings →
Communities during that time." On confirm, run §2.1's delete path with
the leaver as actor.

### 2.5 Restore Community

The deleted community is **invisible everywhere except** Settings →
Communities. There it appears with a "Deleted, X days left" label and
a single action: **Restore Community**.

- Eligible restorers: any user who was a `CommunityUser` of this
  community at the moment it was deleted. We snapshot member IDs into
  `Community.deleted` metadata at delete time so eligibility is a static
  lookup, not "current membership of a deleted community."
- Tapping Restore:
  - Clears `Community.deleted`.
  - Sets `Community.owner_user_id = restorer.id` (the restorer becomes
    the new owner, regardless of whether they were the prior owner).
  - Re-creates a `CommunityUser` row for the restorer if they had left
    *before* the delete (edge case: the original owner deletes a
    community that the restorer left from earlier — see §7.6).
  - Records a `COMMUNITY_RESTORED` event.
  - Notifies all snapshotted members: "<restorer> restored <name>.
    <restorer> is the new owner."
- Restore does **not** un-cancel transfers, requests, gear-shares, or
  membership rows that were cleaned up *before* the delete (e.g., a
  member who left day -3, then community deleted day 0). It only
  reactivates content that was implicitly hidden by the delete itself.
  See §5 for the precise rule.

### 2.6 Rejoin window after Leave

If a user leaves a community, they can **rejoin without a new invite**
for 30 days. Mechanism:
- The leaver's `CommunityUser` row is soft-deleted (with
  `DeletedMetadata`) instead of hard-deleted. Today the row is hard
  deleted; this is one of the first changes (§6).
- A new `JoinCommunity` (or restore-membership) RPC accepts a community
  ID with no invite if the caller has a soft-deleted membership less
  than 30 days old **and the community is active**.
- After 30 days, a daily job hard-deletes the row, and the normal
  invite path is required again.

**Rejoin is blocked while the community is deleted, even inside the
30-day window.** The community-deleted state precedes any individual
rejoin right; only an eligible restorer can move the community out of
that state. See §2.7.

### 2.7 Scenario: leaver tries to return to a deleted community

A canonical case the system must handle without ambiguity:

1. Day 0 — user A leaves community C. A's `CommunityUser` row is
   soft-deleted; A's 30-day rejoin clock starts.
2. Day 5 — the owner (or a sole-member-leave path) deletes C. C's
   `Community.deleted_snapshot.member_user_ids` snapshot is taken
   from the *currently active* members at day 5. A is **not** in the
   snapshot, because A had already left.
3. Day 10 — A taps the community in their own UI (e.g., from a
   bookmark, a stale push, or the Settings → Communities entry).

Expected behavior:

- A's `RejoinCommunity` call returns `FailedPrecondition` with reason
  `community_deleted`. The 30-day rejoin window does **not** override
  the community's deleted state.
- C does **not** appear in A's Settings → Communities deleted-list.
  The deleted-list surface is gated on snapshot membership (§2.5),
  and A is not in the snapshot.
- A's `RestoreCommunity` call returns `PermissionDenied` with reason
  `not_in_restorer_snapshot`. A cannot restore C.
- A sees the same surface they would for any other community they're
  not a member of: nothing. From A's perspective, C is gone.
- A's only paths back into C are (a) C gets restored by an eligible
  member and someone subsequently invites A, or (b) C gets purged at
  day 35 and is irrecoverable for everyone.

This rule falls out of two invariants already stated and is repeated
here because it's the exact edge case most likely to confuse support
and most likely to regress in a refactor:
- **Snapshot eligibility is fixed at delete time** (§2.5, §7.6).
- **Rejoin requires the community to be active** (§2.6).

---

## 3. Lifecycle states

### Community
| State | `Community.deleted` | `owner_user_id` | Visible? |
|---|---|---|---|
| Active | unset | a current member | yes, normal UX |
| Deleted (≤30d) | set; `eligible_restorer_ids` populated | preserved | only in Settings → Communities, restore-only |
| Purged (>30d) | row hard-deleted by job | — | gone |

### CommunityUser (membership)
| State | `deleted` | Behavior |
|---|---|---|
| Active | unset | full participation |
| Soft-deleted (≤30d) | set | rejoin without invite |
| Hard-deleted (>30d) | row gone | invite required to rejoin |

---

## 4. What "Leave" cleans up

The leaver's ties to the community are cut atomically inside one
transaction (§6.2). The general rule the user sees:

> "Your gear, requests, and active transfers in this community will be
> cancelled. Your events stay. Past chats stay."

Concretely:

| Entity (scoped to leaver + community) | Action on Leave |
|---|---|
| `CommunityUser` (leaver's row) | soft-delete (rejoin window) |
| `CommunityGear` rows for gear owned by leaver | soft-delete |
| `CommunityRequest` rows for requests created by leaver | cancel the request (existing `CancelRequest` semantics, scoped to this community) |
| `CommunityExperience` rows where leaver hosts | **keep**. Events belong to the community. The leaver is responsible for handing off logistics before they go. |
| Active transfers (loans/giveaways) where leaver is sender or recipient and the transfer is community-scoped to this community | cancel |
| RSVPs (`CommunityExperienceUser`) by leaver | drop |
| `CommunityNotificationPreferences` for leaver | delete |
| `CommunityInvitationLink` created by leaver | revoke |
| `CommunityEvent` rows authored by leaver | leave in place (history) |
| Chat messages authored by leaver in community-scoped conversations | leave in place |
| `CommunityRegion` aggregation | recompute (already done today) |

We deliberately do **not** enumerate per-item state to the user. The
confirmation dialog speaks in categories. Open issue #1571 ("stale
community-gear after leave") will be subsumed by the
`CommunityGear`-soft-delete row of this table.

---

## 5. What "Restore" reactivates

Two mental rules:

1. **Restore reverses the Delete, not history.** Anything that was
   already cancelled or removed *before* the delete stays cancelled.
2. **Restore does not re-add ex-members.** Only the snapshotted
   restorer is re-added (because they have to be a member to be the
   owner). Other members whose membership had been soft-deleted before
   the delete remain in their pre-delete state.

So:

| Pre-delete state | After restore |
|---|---|
| Active gear share | active again (was hidden by community-delete cascade) |
| Gear share that was already soft-deleted by a prior leave | stays soft-deleted |
| Active community-scoped transfer in flight | active again |
| Transfer that had been cancelled by a prior leave | stays cancelled |
| Pre-delete chat history | visible again |
| Pre-delete events (past + future) | visible again |
| Member who left day -3 | still a soft-deleted membership; rejoin-window clock did not pause |

The 30-day rejoin-window clock for *individual* leaves is **not** paused
by a delete. If member A left on day -10 and the community is deleted
on day 0 and restored on day 25, A's rejoin window expired on day 20 and
A needs a fresh invite. This keeps the soft-delete clocks independent
and avoids a "what if multiple deletes overlap" quagmire.

---

## 6. Server changes

### 6.1 Schema and proto

**Owner field.** Today `Community.creator_id` doubles as ownership.
Add `Community.owner_user_id` and migrate `creator_id` to
"historical creator only, never updated." `owner_user_id` starts equal
to `creator_id` for existing rows.

```proto
message Community {
  // …
  string creator_id = 4;       // immutable, historical
  string owner_user_id = 11;   // current owner; transfer changes this
  optional DeletedMetadata deleted = 20;
  optional CommunityDeletedSnapshot deleted_snapshot = 21;
}

message CommunityDeletedSnapshot {
  // CommunityUser.user_ids that were active members at delete time.
  // This is the eligible restorer set.
  repeated string member_user_ids = 1;
}
```

**`CommunityUser.deleted`.** Add `optional DeletedMetadata deleted` so
membership can be soft-deleted for the rejoin window.

**Database invariants.**
- `UNIQUE (community_id, user_id)` on `community_user` already exists
  (or should — verify; if missing, add as part of this work).
- New: `community.owner_user_id` is NOT NULL on active rows. Enforced
  by trigger or check constraint: `(deleted IS NOT NULL) OR
  (owner_user_id IS NOT NULL AND owner_user_id IN (SELECT user_id FROM
  community_user WHERE community_id = community.id AND deleted IS
  NULL))`. Practically this is enforced inside the
  `LeaveCommunity`/`TransferOwnership` transaction (§6.2) using
  `SELECT … FOR UPDATE`; the constraint is the belt-and-suspenders.

### 6.2 New / changed RPCs

| RPC | Status | Behavior |
|---|---|---|
| `DeleteCommunity` | **rewrite** | Owner-only. Soft-delete community. Snapshot member IDs into `CommunityDeletedSnapshot`. Cascade soft-delete to `CommunityGear`, `CommunityRequest`, `CommunityExperience`, `CommunityNotificationPreferences`, `CommunityInvitationLink`. Emit `COMMUNITY_DELETED` event. Notify all members. |
| `RestoreCommunity` | **new** | Caller must be in the snapshot. Inside a transaction: clear `Community.deleted` and `deleted_snapshot`; set `owner_user_id = caller`; if caller has no active `CommunityUser` row, create one (un-soft-delete or insert). Cascade un-soft-delete the join rows we set in DeleteCommunity. Emit `COMMUNITY_RESTORED`. Notify snapshot members. |
| `LeaveCommunity` | **rewrite** | Soft-delete `CommunityUser` (was hard-delete). Run §4 cleanup. If caller is owner, *require* `new_owner_user_id` in the request — server validates it points to a current active member, then transfers ownership (`SELECT … FOR UPDATE` on the candidate's `CommunityUser` row) before soft-deleting the leaver's row. Notify the new owner. Sole-member case: short-circuits to `DeleteCommunity` semantics in the same transaction. |
| `RejoinCommunity` | **new** | Allowed if caller has a soft-deleted `CommunityUser` row in this community within 30 days. Un-soft-deletes the row. No invite required. Emits a join event. Past 30 days returns `FAILED_PRECONDITION`. |
| `ListCommunityCandidates`-for-handoff | **new** or reuse `ListCommunityUsers` | Client renders the picker for the owner-leave flow from existing members minus the caller. Reuse `ListCommunityUsers` if filtering on the client is acceptable. |

The owner-leave + handoff is **one RPC** (not two), so there is no
window where ownership is undefined.

### 6.3 Undo registry

Add to `server/undo/registry.go` (§ enforcement test
`TestEveryEventTypeClassified`):
- `COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED` →
  `ClassificationRetentionRestore` (rationale: 30-day soft-delete
  reversed by `RestoreCommunity` RPC, not by an `Undo*` RPC).
- `COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED` →
  `ClassificationInternal` (system-emitted by RestoreCommunity).
- `COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED` →
  `ClassificationClientOnly`, rationale: "Inverse is the new owner
  initiating their own ownership transfer."
- `COMMUNITY_EVENT_TYPE_MEMBER_LEFT` keeps its current
  `ClassificationClientOnly` rationale, but updated to mention the
  30-day rejoin window as the natural inverse during that window.

### 6.4 Audit trail

A persistent server-side mutation log of "who did what to which
community, when" is required for support, debugging, and the
day-before-purge reminder copy. The community itself already records
state transitions via `CommunityEvent`, which is the right home —
extend the enum:

- `COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED` (actor = deleter)
- `COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED` (actor = restorer)
- `COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED` (actor = previous
  owner; `object_user_id` = new owner)
- `COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW` (actor = rejoiner)

`CommunityEvent` rows are not soft-deleted on community delete; they
remain visible to the daily job and to support tooling. They are
purged when the community itself is hard-deleted.

### 6.5 Read-path filtering

Every read path that scopes by community must skip communities where
`deleted IS NOT NULL`. Audit:

- `ListCommunities`, `GetCommunity`, search, feed, stories, leaderboard,
  impact metrics, gear/request/experience listings — all must filter
  out deleted communities except in the **Settings → Communities**
  surface, which explicitly opts in via `IncludeDeleted: true`.
- `ListCommunityGear`, `ListCommunityRequests`,
  `ListCommunityExperiences` — already filter soft-deleted join rows;
  must also short-circuit to empty when the parent `Community` is
  deleted (cheaper than relying on cascade alone, and prevents races
  where cascade hasn't finished).
- `Search` — `Community`, `Gear`, `Request`, `Experience` indices must
  exclude rows whose community is deleted. Either filter at query time
  or rebuild on `COMMUNITY_DELETED`/`COMMUNITY_RESTORED` events.
- `Chat` — community-scoped conversations must 403 for all callers
  while the community is deleted, and re-open on restore. Personal
  conversations not scoped to a community are unaffected.
- Notifications — outbound push for any community-scoped event must
  skip deleted communities. Add a guard in the notification dispatcher.

### 6.6 Daily purge job (deferred — see §10)

The job runs once per day:

1. Find `Community` rows with `deleted.deleted_at_unix_sec < now -
   30 days`. For each:
   - Hard-delete the cascade: `CommunityUser`, `CommunityGear`,
     `CommunityRequest`, `CommunityExperience`,
     `CommunityNotificationPreferences`, `CommunityInvitationLink`,
     `CommunityEvent`, `CommunityRegion`, community-scoped
     conversations and chat messages, stories scoped to this
     community, nudges, feed entries.
   - Hard-delete the `Community` row last.
2. Find `CommunityUser` rows with `deleted.deleted_at_unix_sec < now -
   30 days`. Hard-delete.
3. Send the **day-before-purge** push notification to snapshot members
   of any community whose 30-day mark falls tomorrow.

Step 3 is **not deferrable** — it's required even before §10 ships,
because the user-visible promise is "we'll warn you the day before."
The hard-delete cascade itself can ship later (§10).

---

## 7. Client changes (Flutter)

### 7.1 Settings → Communities list

- Show active communities as today.
- Show deleted communities (those the user was a member of at delete
  time) with a "Deleted, X days left" badge and a Restore action.
  Tapping the row opens a Restore screen with one CTA.
- Hide deleted communities from every other list, search result, and
  picker in the app.

### 7.2 Danger Zone

- Render "Delete Community" only when `community.owner_user_id ==
  currentUser.id`.
- The Delete button opens the choice modal (Delete vs.
  Leave-with-handoff vs. Cancel).

### 7.3 Leave flow

- Single confirmation for non-owners.
- Owner branch: member-picker → confirm handoff → confirm leave.
- Sole-member branch: the dialog explains "leaving deletes the
  community for 30 days," then runs the delete flow.

### 7.4 Restore deep link

The day-before-purge push notification's tap action deep-links to
`/settings/communities/<id>/restore`.

### 7.5 Cache invalidation

On any of `DeleteCommunity` / `RestoreCommunity` / `LeaveCommunity` /
`RejoinCommunity` / `OwnershipTransfer`, the relevant repositories
must invalidate:
- `CommunityRepository` (the community row)
- `GearRepository`, `RequestRepository`, `ExperienceRepository` keyed
  by community
- `LoanRepository` for any active transfers
- Conversation/chat caches
- Feed and stories caches

### 7.6 Edge case: restorer who had previously left

If user A left day -3 and the community is deleted day 0 and A taps
Restore on day 5: A is in the snapshot (because they were in the
community at delete time? — no, they had left before). The eligible
set is **active members at delete time**, not "ever a member." A is
not eligible. UI does not surface the deleted community to A. (This
keeps the rule simple: snapshot = the member set used by the cascade.)

---

## 8. Notifications

| Event | Recipients | Copy |
|---|---|---|
| `COMMUNITY_DELETED` | all snapshot members except the deleter | "<owner> deleted <name>. You have 30 days to restore it." |
| `COMMUNITY_RESTORED` | all snapshot members except the restorer | "<restorer> restored <name>. <restorer> is the new owner." |
| `OWNERSHIP_TRANSFERRED` (during owner-leave) | the new owner only | "You're now the owner of <name>." |
| Day-before-purge | all snapshot members | "<name> will be permanently deleted tomorrow. Tap to restore." |
| `MEMBER_LEFT` | community owner only (today's behavior) | unchanged |

All copy must say "Event," not "experience" — the existing rule from
`CLAUDE.md`. None of these strings include vendor names or transport
plumbing.

---

## 9. Test surface

### 9.1 Server

- Owner-only delete: non-owner gets `PermissionDenied`.
- Delete cascades to all join rows listed in §4.
- Restore reactivates them.
- Restore by ineligible user: `PermissionDenied`.
- Owner leaves with handoff: ownership transfers, old owner row is
  soft-deleted, atomicity verified by injecting a concurrent leave on
  the candidate (must produce one consistent terminal state, never
  zero owners).
- Owner leaves without handoff target: `InvalidArgument`.
- Sole-member leaves: converts to delete; restore brings everything
  back.
- Rejoin within 30 days: succeeds without invite.
- Rejoin after 30 days: `FailedPrecondition`.
- **Rejoin into a deleted community (the §2.7 scenario):** caller has
  a soft-deleted `CommunityUser` row less than 30 days old, but the
  community is in deleted state. `RejoinCommunity` returns
  `FailedPrecondition` (`community_deleted`); `RestoreCommunity` by
  the same caller returns `PermissionDenied`
  (`not_in_restorer_snapshot`); the community does not appear in the
  caller's Settings → Communities deleted-list.
- Read-path filtering: `Get*`/`List*`/search/feed/notifications all
  skip deleted communities. `AssertMaxQueries` to prevent N+1 on the
  cascade (§ "SQL Efficiency" rules in `CLAUDE.md`).
- `TestEveryEventTypeClassified` covers the new event types.

### 9.2 Client

- Danger Zone visibility gated by ownership.
- Settings → Communities renders deleted entries with countdown and
  restore CTA.
- Leave flow: cancel at every step.
- Owner-leave flow: cancel at picker, cancel at handoff confirm,
  cancel at leave confirm.
- Cache invalidation: deleting a community removes its content from
  every screen.

### 9.3 Workflow / system tests

Update `docs/workflows/` and the system-test corpus
(`/ripls-update-system-tests`):
- `community.md`: new sections for delete, leave-with-handoff,
  sole-member leave, restore, rejoin.
- New end-to-end tests covering owner-leave-with-handoff and
  community-restore.

---

## 10. Deferred work (hard delete)

**Issue #517 ("Implement some hard delete functionality")** is the
parent. The following items belong as children of #517 or as new
issues linked to it:

- **Daily purge job** — implement §6.6 steps 1 and 2. Step 3 (the
  day-before reminder) ships with the soft-delete work.
- **Cascade purge of community-scoped chat history.** Today there is
  no infrastructure for hard-deleting `Conversation` + `ChatMessage`
  rows scoped to a community. See `server/storage/cascade_delete.go`.
- **Cascade purge of stories, nudges, feed entries.**
- **GDPR/account-deletion overlap.** Account deletion already needs a
  hard-delete path. The community purge job can share the cascade
  helpers introduced for that.
- **#1571 (stale community-gear after leave)** is folded into §4 of
  this doc — leaver's `CommunityGear` rows soft-delete on leave, no
  separate fix needed once this ships.
- **#1185-style audit** — verify no other join-row types dangle after
  community soft-delete by adding a query-stats integration test
  similar to `cascade_delete_test.go`.

Until §10 ships, soft-deleted communities and memberships accumulate
in the database. That is acceptable: the volumes are low and the daily
job is bounded work.

---

## 11. Layer-by-layer work breakdown

### Proto (`proto/ripls/`)
- `models/community.proto`: add `Community.owner_user_id`,
  `Community.deleted_snapshot`, `CommunityUser.deleted`. Three new
  `CommunityEventType` values.
- `api/community_service.proto`: rewrite `DeleteCommunityRequest` (no
  shape change — server semantics change), add
  `RestoreCommunityRequest/Response`, add
  `RejoinCommunityRequest/Response`, add `new_owner_user_id` to
  `LeaveCommunityRequest`. New RPCs registered in the service.

### Storage (`server/storage/`)
- New constraint or migration for `community.owner_user_id` non-null
  on active rows.
- Verify `UNIQUE (community_id, user_id)` on `community_user`.
- New cascade helpers for community → all child join types.

### Service (`server/services/community/`)
- `lifecycle.go`: rewrite `DeleteCommunity`. New `RestoreCommunity`
  function.
- `membership.go`: rewrite `LeaveCommunity` with handoff and
  soft-delete. New `RejoinCommunity`.
- `lifecycle_test.go`, `membership_test.go`: cover all §9.1 cases.

### Read paths
- Audit `Get*`/`List*`/search/feed/leaderboard/impact_metrics for
  community-scoped reads. Add a `Community.deleted` filter at every
  one of those edges. Track each one in a child issue.

### Notifications (`server/community/notifications.go` and
`server/notifications/`)
- Implement the four notification types in §8.
- Daily-purge reminder needs to know the snapshot member list, which
  lives on `Community.deleted_snapshot`.

### Undo registry (`server/undo/registry.go`)
- Three new entries (§6.3).

### Client (`app/lib/`)
- `presentation/screens/communities/`: Settings list shows deleted
  state; Danger Zone gated by ownership.
- New screens: Restore confirmation, member-picker for owner handoff.
- `data/repositories/`: invalidation on delete/restore/leave/rejoin.
- `services/`: bind the four new RPCs.
- `l10n/app_en.arb` + `app_es.arb`: copy in §8 plus the dialog text in
  §2.

### Docs / system tests
- `docs/workflows/community.md` updated with new flows.
- `/ripls-update-system-tests` to regenerate the system-test corpus.
- Cross-link this doc from `docs/deleting_things.md`.

---

## 12. Non-obvious impacts

- **Search index invalidation.** Search currently surfaces gear,
  requests, experiences, and communities by ID. When a community is
  deleted, every index entry referencing it must stop scoring.
  Either filter at query time (cheap, slow) or invalidate on the
  `COMMUNITY_DELETED` event (more code, fast queries). Pick query-time
  filter for v1; revisit if it shows up in latency dashboards.
- **Notification fan-out.** Today
  `RecordCommunityEventAndNotify` happily dispatches push
  notifications. After delete, *no* community-scoped push should fire
  — including for events that happened in chats that are now hidden.
  Add the deleted-community guard in the dispatcher itself, not at the
  caller, so we cannot regress this by adding a new event type later.
- **Background jobs in flight.** Async LLM jobs (gen experience, gen
  community, image gen) keyed to a community ID may complete *after*
  delete. Their write paths must check `Community.deleted` and skip
  the write. Otherwise the user restores and finds AI-generated
  content from the deletion window.
- **FCM push tokens.** No change — tokens are per-user, not
  per-community.
- **Impact metrics rollups.** Community-level impact dashboards must
  exclude deleted communities. Restore should not retroactively
  re-credit a deleted-then-restored community for the gap window
  unless we want it to (default: do credit it on restore — content
  was preserved, members still got the benefits).
- **Streaming / realtime.** No stream terminates on delete. A client
  holds one `StreamUserEvents` connection covering every community it
  belongs to (`server/services/community/user_streaming.go`), so
  ending it would drop realtime for all the others. `COMMUNITY_DELETED`
  is delivered like any other event and `app/lib/services/event_router.dart`
  reacts — clearing the community list, the restorable list, and the
  feed/search/content caches so the community leaves every surface. The
  per-community stream that used to terminate on this event was removed
  in #2869.
- **Community creator analytics.** Existing dashboards keyed on
  `creator_id` keep working. Anything keyed on "current owner" must
  use `owner_user_id`.
- **Single-Player Mode.** A single-player community follows the same
  sole-member-leave-equals-delete rule. Verify against
  `docs/single_player_mode.md`.
- **Race: two members tap Restore simultaneously.** The first wins
  (transactional `UPDATE community SET deleted = NULL, owner_user_id
  = $1 WHERE id = $2 AND deleted IS NOT NULL`). The second sees
  `FailedPrecondition` and the client refetches → community is
  active, owned by the first restorer.
- **Race: deleter and a leaver fire concurrently.** If the owner is
  deleting while another member is leaving, the leave's cleanup may
  collide with the delete's cascade. Both target disjoint
  `CommunityUser` rows so the rows are fine; the only shared write is
  `CommunityRegion` recompute. Serialize via row-level locks on the
  community.
- **Owner-leave race.** Two members cannot simultaneously become the
  new owner because the owner-leave RPC takes `SELECT … FOR UPDATE`
  on `Community` for the duration. Documented as a server invariant,
  enforced by the test in §9.1.

---

## 13. Open follow-ups (file as separate issues)

- New issue: "Implement community soft-delete with 30-day restore
  window" — parent for the work in §§ 6, 7, 9.
- New issue (child of #517): "Daily community purge job" — §6.6 and §10.
- New issue: "Read-path audit: filter deleted communities in every
  list/search/feed surface" — §6.5.
- New issue: "Notification dispatcher: skip deleted communities at
  the source" — §12 bullet 2.
- New issue: "Background-job write guard: skip writes to deleted
  communities" — §12 bullet 3.
- Close-as-superseded: #1571 once the §4 cascade ships.
