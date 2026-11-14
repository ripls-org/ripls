// Daily sweep that closes stale gcp-policy:* tracker issues. Used by the
// warn-severity Cloud Monitoring path: those issues aggregate per
// policy-per-day (label `gcp-policy:<slug>-<yyyymmdd>`) and don't have
// a natural "close" event from Cloud Monitoring (incident.state=closed
// for warn alerts is dropped — see gcpAlertsHandler.ts).
//
// Schedule: every day at 09:00 UTC. Closes any open issue carrying a
// `gcp-policy:*` label whose `created_at` is older than 24h. Comments
// with "Auto-closed by daily sweep" before closing so the trail is
// visible in the issue history.
//
// Firebase's onSchedule trigger provisions a Cloud Scheduler job + a
// Pub/Sub topic + the runtime service account binding automatically —
// no extra Terraform resources needed.

import { onSchedule } from 'firebase-functions/v2/scheduler';
import { logger } from 'firebase-functions/v2';
import type { IssueTracker } from './issueTracker';
import {
  REPO_NAME,
  REPO_OWNER,
  forgejoAlertsToken,
  trackerFromSecret,
} from './forgejoTracker';

/** Label prefix that the warn-severity gcpAlertsHandler stamps on issues. */
const GCP_POLICY_LABEL_PREFIX = 'gcp-policy:';

/** Minimum issue age before the sweep is willing to close it. */
const STALE_AFTER_MS = 24 * 60 * 60 * 1000;

interface SweepReport {
  scanned: number;
  closed: number;
  skipped: number;
}

/**
 * Pure-async sweep logic. Exported for direct unit testing with a mocked
 * IssueTracker. The trigger wrapper at the bottom of this file adapts the
 * scheduler event to a call here.
 */
export async function runDailySweep(
  tracker: IssueTracker,
  now: Date = new Date(),
): Promise<SweepReport> {
  const cutoffMs = now.getTime() - STALE_AFTER_MS;

  // List open issues with the auto-filed label. The gcp-policy:* prefix
  // alone is not a valid label filter (labels are exact-match), so we
  // pull the broader auto-filed set and filter client-side. Volume is
  // bounded — at our scale, a few hundred issues max.
  const { data: candidates } = await tracker.issues.listForRepo({
    owner: REPO_OWNER,
    repo: REPO_NAME,
    state: 'open',
    labels: 'auto-filed',
    per_page: 100,
    sort: 'created',
    direction: 'asc',
  });

  const report: SweepReport = { scanned: 0, closed: 0, skipped: 0 };

  for (const issue of candidates) {
    if ('pull_request' in issue && issue.pull_request) continue;
    const hasPolicyLabel = (issue.labels ?? []).some((label) => {
      const name = typeof label === 'string' ? label : label.name;
      return typeof name === 'string' && name.startsWith(GCP_POLICY_LABEL_PREFIX);
    });
    if (!hasPolicyLabel) continue;
    report.scanned += 1;

    const createdAt = issue.created_at ? Date.parse(issue.created_at) : NaN;
    if (Number.isNaN(createdAt) || createdAt > cutoffMs) {
      report.skipped += 1;
      continue;
    }

    await tracker.issues.createComment({
      owner: REPO_OWNER,
      repo: REPO_NAME,
      issue_number: issue.number,
      body:
        `Auto-closed by daily sweep at ${now.toISOString()}. ` +
        `Issue is older than 24h and aggregates a daily warn-severity policy. ` +
        `Reopen if the same policy fires again today; the next firing creates a fresh ` +
        `\`gcp-policy:<slug>-<yyyymmdd>\` issue regardless.`,
    });
    await tracker.issues.update({
      owner: REPO_OWNER,
      repo: REPO_NAME,
      issue_number: issue.number,
      state: 'closed',
    });
    report.closed += 1;
  }

  return report;
}

export const dailyGcpPolicySweep = onSchedule(
  {
    schedule: 'every day 09:00',
    timeZone: 'UTC',
    secrets: [forgejoAlertsToken],
  },
  async () => {
    const report = await runDailySweep(trackerFromSecret());
    logger.info('daily gcp-policy sweep complete', report);
  },
);
