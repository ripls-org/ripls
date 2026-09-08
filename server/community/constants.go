package community

// MaxCommunityMembers is the maximum number of members allowed in a community.
// See docs/community_health.md.
const MaxCommunityMembers = 32

// RejoinWindowDays is the maximum age (in days) of a soft-deleted
// CommunityUser row that still permits the user to rejoin without a
// fresh invite. Past this boundary, the daily purge job (#1620)
// hard-deletes the row and the normal invite flow is required.
// See docs/community_delete_and_leave.md §2.6.
const RejoinWindowDays = 30

// RejoinWindowSeconds is RejoinWindowDays expressed in seconds for
// direct comparison against Unix timestamps. The boundary is
// inclusive: a row whose deleted_at is exactly RejoinWindowSeconds
// in the past still qualifies for rejoin.
const RejoinWindowSeconds int64 = RejoinWindowDays * 24 * 60 * 60

// PurgeReminderLeadDays is the number of days before the purge
// boundary at which the day-before-purge reminder push fires.
// Set to RejoinWindowDays - 1 so the reminder lands one day
// before the rejoin window expires (and one day before #1620's
// hard-delete job runs). See docs/community_delete_and_leave.md §6.6.
const PurgeReminderLeadDays = RejoinWindowDays - 1

// PurgeReminderLeadSeconds is PurgeReminderLeadDays expressed in
// seconds. The daily reminder job fires for any soft-deleted
// community whose deleted_at is at least this many seconds in the
// past and whose purge-reminder field is still unset.
const PurgeReminderLeadSeconds int64 = PurgeReminderLeadDays * 24 * 60 * 60

// PurgeReminderBatchSize caps the number of communities processed
// in a single CommunityPurgeReminderJob.Run invocation. Picked
// generously since each iteration is bounded work (one claim
// query + a small fan-out of pushes). Set above the realistic
// daily-volume ceiling so the job never has to chunk.
const PurgeReminderBatchSize = 1000

// PurgeWindowDays is the maximum age (in days) of a soft-deleted
// Community row before the daily purge job (#1620) hard-deletes
// the community and its cascade. Distinct from RejoinWindowDays
// — the two happen to share the same value today, but the
// product semantics are independent (per-user rejoin window vs.
// per-community retention window) so the constants are separate
// to allow them to diverge.
// See docs/community_delete_and_leave.md §6.6.
const PurgeWindowDays = 30

// PurgeWindowSeconds is PurgeWindowDays expressed in seconds for
// direct comparison against Unix timestamps. A community whose
// deleted_at_unix_sec is older than now-PurgeWindowSeconds is a
// hard-delete candidate.
const PurgeWindowSeconds int64 = PurgeWindowDays * 24 * 60 * 60

// CommunityPurgeBatchSize caps the number of communities
// processed in a single CommunityPurgeJob.Run invocation. Lower
// than PurgeReminderBatchSize because each iteration here runs a
// per-table cascade, which is meaningfully more work than a
// single notify call.
const CommunityPurgeBatchSize = 100
