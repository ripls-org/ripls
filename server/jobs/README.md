# Jobs

The `jobs` package contains server-side background jobs that run at startup or on a schedule. Jobs operate on stored data rather than handling live requests — they purge expired records, send scheduled digests, and pre-warm derived content.

## Key files

- `periodic.go` — `RunPeriodic`: the shared driver for every periodic job — panic-safe goroutine (`logging.GoSafe`), immediate startup run, ticker loop, and `errs.TransientStreak`-classified error logging. Job failure-message strings are alert-query surface; treat existing ones as frozen.
- `community_purge.go` — `CommunityPurgeJob`: daily hard-delete of soft-deleted communities (and their cascade) whose 30-day window expired. Kill switch and dry-run flags; see #1620.
- `community_purge_reminder.go` — `CommunityPurgeReminderJob`: dispatches the day-before-purge push reminder to snapshot members of soft-deleted communities crossing the 29-day mark. Idempotent via the conditional-UPDATE primitive on `Community.purge_reminder_sent_at_unix_sec`. See `docs/community_delete_and_leave.md` §6.6 and #1659.
- `activity_digest.go` / `activity_digest_weekly.go` — daily and weekly ops-facing activity digest emails (#1924); at most one send per local day/ISO-week via same-period claims.
- `workshop_generation.go` — `WorkshopGenerationJob`: startup pre-warm of Workshop Hero / Bring-Back nudges for every (host, community) pair. Deliberately does not enqueue notifications (docs/ai/workshop.md Decision 8).
- `scheduled_notifications/` — the reconciler/dispatcher subsystem for time-anchored notifications (event reminders, loan-return prompts); see `docs/server/scheduled_notifications.md`.
- `shutdown.go` — helpers that let batch loops distinguish process teardown from genuine per-row failures.

## When to add code here vs. elsewhere

A job belongs here when it runs asynchronously of any user request, operates on a batch of existing records, and is triggered at startup or on a timer rather than in response to an RPC. Logic triggered directly by an RPC (even if deferred to a goroutine) belongs in the service package that owns the entity.

One-shot data migrations and backfills may live here temporarily: write them idempotent, wire them at startup, and **delete them once production logs show they no-op** — completed transition code should not keep running on every boot (#2643 removed the last sweep of them).
