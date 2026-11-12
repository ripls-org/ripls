// Crashlytics → GitHub issue handlers. Wraps the firebase-functions v2
// alert triggers and routes through the shared upsertIssue helper.
//
// Gating per the #1148/#1149 plan's §"Claude-trigger gating"
// (archived):
//   - Velocity alerts: file always; claude-fix only when crashPercentage
//     >= CLAUDE_FIX_VELOCITY_THRESHOLD_PCT.
//   - Regression alerts: file always; claude-fix always (regressions are
//     by definition something we already fixed once — Claude has the diff
//     context to land another fix).
//
// onNewFatalIssuePublished and onNewNonfatalIssuePublished are
// intentionally NOT exported — too noisy at our volume, per the plan's
// "Issue-creation gating" table. Re-enable in Phase 2 if signal is sparse.

import { Octokit } from '@octokit/rest';
import {
  CrashlyticsEvent,
  RegressionAlertPayload,
  StabilityDigestPayload,
  VelocityAlertPayload,
  onRegressionAlertPublished,
  onStabilityDigestPublished,
  onVelocityAlertPublished,
} from 'firebase-functions/v2/alerts/crashlytics';
import { logger } from 'firebase-functions/v2';
import {
  REPO_NAME,
  REPO_OWNER,
  githubAppPrivateKeyBase64,
  octokitFromAppSecret,
} from './githubAuth';
import { upsertIssue } from './upsertIssue';

/**
 * Crash percentage at or above which we trigger Claude planning automatically.
 * Below this we still file the issue but don't auto-plan — humans triage.
 *
 * 1% chosen as a starting point per the plan's Phase 1 acceptance. Tune in
 * Phase 3 based on observed noise.
 */
export const CLAUDE_FIX_VELOCITY_THRESHOLD_PCT = 1;

/** Public Firebase console URL for a Crashlytics issue. */
function firebaseConsoleUrl(issueId: string): string {
  // The console accepts the raw issue id and resolves the project from the
  // session. We deploy under a single Firebase project so this is fine.
  return `https://console.firebase.google.com/project/_/crashlytics/app/-/issues/${issueId}`;
}

/**
 * Builds the Markdown body posted to GitHub for a velocity alert. Pure
 * function — easy to unit-test against fixed payloads.
 */
export function buildVelocityIssueBody(payload: VelocityAlertPayload): string {
  const { issue, crashCount, crashPercentage, firstVersion, createTime } = payload;
  const link = firebaseConsoleUrl(issue.id);
  const pct = Number(crashPercentage).toFixed(2);
  return `## Velocity alert (trending crash)

**Crashlytics issue:** [${issue.id}](${link})
**Title:** ${issue.title}
**Subtitle:** ${issue.subtitle}
**App version (alert):** ${issue.appVersion}
**First seen in version:** ${firstVersion}
**Sessions affected (last alert window):** ${crashCount} (${pct}%)
**Alert created:** ${formatTimestamp(createTime)}

[View in Firebase Console](${link})

_Auto-filed by ripls-alerts Cloud Function. Dedup label: \`crashlytics:${issue.id}\`._`;
}

/**
 * Builds the Markdown body posted to GitHub for a regression alert.
 * The same Crashlytics issue id reopens an existing issue (or creates one
 * if the previous fix predates the auto-filer).
 */
export function buildRegressionIssueBody(payload: RegressionAlertPayload): string {
  const { issue, type, resolveTime } = payload;
  const link = firebaseConsoleUrl(issue.id);
  return `## Regression alert (previously-fixed crash returned)

**Crashlytics issue:** [${issue.id}](${link})
**Title:** ${issue.title}
**Subtitle:** ${issue.subtitle}
**App version (regression seen in):** ${issue.appVersion}
**Type:** ${type}
**Previously resolved at:** ${formatTimestamp(resolveTime)}

[View in Firebase Console](${link})

_Auto-filed by ripls-alerts Cloud Function. Dedup label: \`crashlytics:${issue.id}\`. \
Reopened from prior closure if the GitHub issue still exists._`;
}

/**
 * Returns true if a velocity alert should automatically receive the
 * `claude-fix` label (which fires the existing claude_plan_issue.yaml
 * workflow). Lifted out of the trigger handler so tests can cover the
 * threshold logic without spinning up the firebase event harness.
 */
export function shouldClaudeFixVelocity(payload: VelocityAlertPayload): boolean {
  return Number(payload.crashPercentage) >= CLAUDE_FIX_VELOCITY_THRESHOLD_PCT;
}

/**
 * Pure-async handler for velocity alerts. Exported so it's directly testable
 * by passing a mock Octokit. The trigger wrapper at module bottom adapts the
 * Firebase event shape into a call to this function.
 */
export async function handleVelocityAlert(
  payload: VelocityAlertPayload,
  octokit: Octokit,
): Promise<void> {
  const dedupLabel = `crashlytics:${payload.issue.id}`;
  const platformLabel = derivePlatformLabel(payload.issue);
  const result = await upsertIssue({
    owner: REPO_OWNER,
    repo: REPO_NAME,
    dedupLabel,
    title: `[crash] ${payload.issue.title}`,
    body: buildVelocityIssueBody(payload),
    extraLabels: ['crash', ...platformLabel],
    reopenIfClosed: false,
    addClaudeFixLabel: shouldClaudeFixVelocity(payload),
    octokit,
  });
  logger.info('crashlytics velocity alert handled', {
    crashlyticsIssueId: payload.issue.id,
    crashPercentage: payload.crashPercentage,
    githubIssue: result.issueNumber,
    action: result.action,
  });
}

/**
 * Pure-async handler for regression alerts. Always opts into claude-fix —
 * regressions have a prior fix history that gives Claude useful context.
 */
export async function handleRegressionAlert(
  payload: RegressionAlertPayload,
  octokit: Octokit,
): Promise<void> {
  const dedupLabel = `crashlytics:${payload.issue.id}`;
  const platformLabel = derivePlatformLabel(payload.issue);
  const result = await upsertIssue({
    owner: REPO_OWNER,
    repo: REPO_NAME,
    dedupLabel,
    title: `[regression] ${payload.issue.title}`,
    body: buildRegressionIssueBody(payload),
    extraLabels: ['crash', 'regression', ...platformLabel],
    reopenIfClosed: true,
    addClaudeFixLabel: true,
    octokit,
  });
  logger.info('crashlytics regression alert handled', {
    crashlyticsIssueId: payload.issue.id,
    githubIssue: result.issueNumber,
    action: result.action,
  });
}

/**
 * Coerces a Crashlytics-style date field into a `Date`. The
 * `firebase-functions` types declare these fields as `string`, but the
 * runtime payload sometimes arrives as a `google.type.Date` proto
 * (`{year, month, day}`) or a `google.protobuf.Timestamp` proto
 * (`{seconds, nanos?}`). Accept all three so a future SDK change does not
 * silently drop alerts (#1804).
 *
 * Throws an `Error` whose message includes the JSON-serialized raw value
 * so logs surface the actual shape on mismatch.
 */
export function coerceCrashlyticsDate(value: unknown): Date {
  if (value instanceof Date && !Number.isNaN(value.getTime())) return value;

  if (typeof value === 'string') {
    const iso = /^\d{4}-\d{2}-\d{2}$/.test(value) ? `${value}T00:00:00Z` : value;
    const d = new Date(iso);
    if (!Number.isNaN(d.getTime())) return d;
  }

  if (value && typeof value === 'object') {
    const obj = value as Record<string, unknown>;
    if (
      typeof obj.year === 'number' &&
      typeof obj.month === 'number' &&
      typeof obj.day === 'number'
    ) {
      // google.type.Date: month is 1-based.
      const d = new Date(Date.UTC(obj.year, obj.month - 1, obj.day));
      if (!Number.isNaN(d.getTime())) return d;
    }
    if (obj.seconds !== undefined) {
      const seconds =
        typeof obj.seconds === 'string' ? Number(obj.seconds) : (obj.seconds as number);
      const nanos = typeof obj.nanos === 'number' ? obj.nanos : 0;
      if (Number.isFinite(seconds)) {
        const d = new Date(seconds * 1000 + Math.floor(nanos / 1_000_000));
        if (!Number.isNaN(d.getTime())) return d;
      }
    }
  }

  throw new Error(`invalid digestDate: ${JSON.stringify(value)}`);
}

/** Formats a `Date` as `YYYY-MM-DD` in UTC. */
export function formatYMD(d: Date): string {
  return d.toISOString().slice(0, 10);
}

/**
 * Renders a Crashlytics timestamp field for inclusion in a Markdown body.
 * Returns the original string if it is one, otherwise an ISO-8601 datetime
 * derived from a recognized object shape, otherwise a JSON dump (so logs
 * never surface the bare `[object Object]` placeholder).
 */
export function formatTimestamp(value: unknown): string {
  if (typeof value === 'string') return value;
  try {
    return coerceCrashlyticsDate(value).toISOString();
  } catch {
    return JSON.stringify(value);
  }
}

/**
 * Returns the ISO date (YYYY-MM-DD) of the Monday in the same week as the
 * supplied date. Used as the dedup-label suffix for stability digests so
 * multiple digests within a week roll up onto a single GitHub issue.
 *
 * Treats Monday as the start of the week (ISO 8601 convention). Accepts
 * the same shapes as `coerceCrashlyticsDate` and throws if the input is
 * not parseable.
 */
export function isoMondayOfWeek(digestDate: unknown): string {
  const d = coerceCrashlyticsDate(digestDate);
  const dow = d.getUTCDay(); // 0=Sun, 1=Mon, ..., 6=Sat
  const offset = (dow + 6) % 7; // days since Monday (Mon=0, Sun=6)
  d.setUTCDate(d.getUTCDate() - offset);
  return formatYMD(d);
}

/**
 * Builds the Markdown body posted to GitHub for a stability digest.
 * Renders the trending issues as a numbered list with per-issue console
 * links so a human can drill in on any one. The full digest is included
 * because the email format already includes it — readers expect parity.
 */
export function buildStabilityDigestBody(payload: StabilityDigestPayload): string {
  const digestYMD = formatYMD(coerceCrashlyticsDate(payload.digestDate));
  const isoMonday = isoMondayOfWeek(payload.digestDate);
  const lines: string[] = [
    `## Stability digest for ${digestYMD}`,
    '',
    `Top ${payload.trendingIssues.length} trending issues by Crashlytics' growth ranking:`,
    '',
  ];
  for (let i = 0; i < payload.trendingIssues.length; i++) {
    const t = payload.trendingIssues[i];
    const link = firebaseConsoleUrl(t.issue.id);
    lines.push(
      `${i + 1}. **${t.issue.title}** — ${t.userCount} users, ${t.eventCount} events ([console](${link}))`,
      `   - \`${t.issue.subtitle}\``,
      `   - Type: ${t.type} · App version: ${t.issue.appVersion}`,
    );
  }
  lines.push(
    '',
    `_Auto-filed by ripls-alerts Cloud Function. Dedup label: \`crashlytics-digest-week:${isoMonday}\`. Each weekday's digest within this week appends a comment; a fresh issue opens each Monday._`,
  );
  return lines.join('\n');
}

/**
 * Pure-async handler for stability digests. Files (or comments on) a
 * weekly rolling issue keyed by the digest's ISO Monday. First digest of
 * the week creates the issue with `claude-fix` so Claude proposes a plan
 * for the highest-ranked trending entry; subsequent days append comments
 * (the existing-issue path in upsertIssue does not re-add labels).
 */
export async function handleStabilityDigest(
  payload: StabilityDigestPayload,
  octokit: Octokit,
): Promise<void> {
  const isoMonday = isoMondayOfWeek(payload.digestDate);
  const dedupLabel = `crashlytics-digest-week:${isoMonday}`;
  const result = await upsertIssue({
    owner: REPO_OWNER,
    repo: REPO_NAME,
    dedupLabel,
    title: `[stability digest] week of ${isoMonday}`,
    body: buildStabilityDigestBody(payload),
    extraLabels: ['stability-digest'],
    // If a human closed the issue early to silence further comments, leave
    // it closed — the next week's digest opens a fresh issue regardless.
    reopenIfClosed: false,
    // claude-fix lands on the create path only (upsertIssue's buildLabelSet
    // does not run on the comment path). So the first digest of the week
    // gets a claude-fix; subsequent days within the same week do not.
    addClaudeFixLabel: true,
    octokit,
  });
  logger.info('crashlytics stability digest handled', {
    digestDate: payload.digestDate,
    isoMonday,
    trendingCount: payload.trendingIssues.length,
    githubIssue: result.issueNumber,
    action: result.action,
  });
}

/**
 * Best-effort platform inference from the issue title/subtitle. Crashlytics
 * payloads don't include an explicit platform field, but the title typically
 * carries class names that disambiguate (e.g., "java.lang." → Android,
 * "_NSError" / Swift symbols → iOS). Returns 0 or 1 labels — never both.
 */
function derivePlatformLabel(issue: { title: string; subtitle: string }): string[] {
  const haystack = `${issue.title} ${issue.subtitle}`.toLowerCase();
  if (/(java\.|kotlin|androidx|com\.google\.android)/i.test(haystack)) {
    return ['platform:android'];
  }
  if (/(swift|nsexception|nserror|uikit|foundation\.|objc)/i.test(haystack)) {
    return ['platform:ios'];
  }
  return [];
}

// ---------------------------------------------------------------------------
// Firebase trigger wrappers (deployed CloudFunctions). These are thin
// adapters so handler logic stays unit-testable.
// ---------------------------------------------------------------------------

export const crashlyticsVelocityAlert = onVelocityAlertPublished(
  { secrets: [githubAppPrivateKeyBase64] },
  async (event: CrashlyticsEvent<VelocityAlertPayload>) => {
    await handleVelocityAlert(event.data.payload, octokitFromAppSecret());
  },
);

export const crashlyticsRegressionAlert = onRegressionAlertPublished(
  { secrets: [githubAppPrivateKeyBase64] },
  async (event: CrashlyticsEvent<RegressionAlertPayload>) => {
    await handleRegressionAlert(event.data.payload, octokitFromAppSecret());
  },
);

export const crashlyticsStabilityDigest = onStabilityDigestPublished(
  { secrets: [githubAppPrivateKeyBase64] },
  async (event: CrashlyticsEvent<StabilityDigestPayload>) => {
    await handleStabilityDigest(event.data.payload, octokitFromAppSecret());
  },
);
