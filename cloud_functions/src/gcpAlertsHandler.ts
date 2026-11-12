// Cloud Monitoring → GitHub issue handler. Subscribes to the
// gcp-alerts-to-github Pub/Sub topic that Cloud Monitoring's
// notification channel publishes to (configured in
// terraform/modules/monitoring/main.tf, gated by
// var.enable_github_pubsub_channel).
//
// Lifecycle:
//   incident.state=open  → upsertIssue (create or comment)
//   incident.state=closed → closeIssue (page-severity policies only)
//
// Dedup strategy:
//   - page severity (uptime / health-check / database / unknown):
//       gcp-incident:<incident_id>  — one issue per incident, closed on resolve
//   - warn severity (rpc errors / service-level errors / latency / SLO burn):
//       gcp-policy:<policy-slug>-<yyyymmdd>  — daily aggregation
//       (closed by the daily-sweep function, not by individual closes)
//
// Claude-trigger gating per the plan's Cost section:
//   - page severity → claude-fix on create
//   - warn severity → claude-fix on first occurrence of the day
//   - unknown severity → no claude-fix (don't burn API on misclassified events)

import { CloudEvent } from 'firebase-functions/v2';
import { logger } from 'firebase-functions/v2';
import { MessagePublishedData, onMessagePublished } from 'firebase-functions/v2/pubsub';
import type { Octokit } from '@octokit/rest';
import {
  REPO_NAME,
  REPO_OWNER,
  githubAppPrivateKeyBase64,
  octokitFromAppSecret,
} from './githubAuth';
import { closeIssue, upsertIssue } from './upsertIssue';

/**
 * Subset of the Cloud Monitoring Pub/Sub incident payload we consume.
 * Full schema: https://cloud.google.com/monitoring/support/notification-options#pubsub
 *
 * Only fields we actually read are typed; everything else is allowed
 * via the index signature so future schema changes don't break parsing.
 */
export interface CloudMonitoringIncidentPayload {
  version: string;
  incident: {
    incident_id: string;
    state: 'open' | 'closed';
    summary?: string;
    url?: string;
    started_at?: number;
    ended_at?: number | null;
    policy_name?: string;
    policy_user_labels?: Record<string, string>;
    condition_name?: string;
    condition?: {
      name?: string;
      displayName?: string;
      [k: string]: unknown;
    };
    threshold_value?: string;
    observed_value?: string;
    severity?: string;
    resource?: {
      type?: string;
      labels?: Record<string, string>;
    };
    metric?: {
      type?: string;
      displayName?: string;
      labels?: Record<string, string>;
    };
    documentation?: { content?: string; mime_type?: string };
    metadata?: { system_labels?: Record<string, string>; user_labels?: Record<string, string> };
    [k: string]: unknown;
  };
}

export type AlertSeverity = 'page' | 'warn' | 'unknown';

// Pattern lists are ordered: page wins over warn if both match (no policy
// hits both today, but order is defensive). Patterns match the CONDITION
// display name (falling back to summary) — the strings set in
// terraform/modules/monitoring/{main,alerts,slos}.tf. The unit tests pin the
// actual Terraform strings so a condition rename that stops matching breaks
// CI instead of silently demoting a policy to "unknown" (#2622 found two
// policies in exactly that state).
const PAGE_PATTERNS: RegExp[] = [
  /uptime\s*check/i,
  /health\s*check/i,
  /database/i,
];
const WARN_PATTERNS: RegExp[] = [
  // all_errors: condition is "Service-level error logged (excluding
  // middleware and infrastructure)". The older /server\s*error\s*logged/i
  // matched the POLICY name, not the condition name — kept for defense.
  /server\s*error\s*logged/i,
  /service[-\s]?level\s*error/i,
  // rpc_error_rate: "RPC errors exceed N% of requests".
  /rpc\s*errors/i,
  // rpc_p95_latency (both tiers): "P95 latency above Nms for ...".
  /latency/i,
  // SLO burn-rate policies (#2624): "... SLO burn rate - fast/slow window".
  // Warn until they prove signal; promote fast-burn to page if warranted.
  /burn\s*rate/i,
  // Outbound delivery policies (#2569 SMS, #2862 email): "SMS delivery
  // failures exceed N% of messages". Deliverability degrades a notification
  // channel rather than taking the service down, and the remedy is usually a
  // provider-side or content change rather than a code fix — warn, not page.
  /delivery\s*fail/i,
];

export function classifySeverity(payload: CloudMonitoringIncidentPayload): AlertSeverity {
  // Prefer the policy's condition displayName (stable, set by Terraform).
  // Only fall back to summary when displayName is absent — concatenating
  // both would let a long summary trip a pattern intended to match the
  // policy itself, e.g., a "Server Error Logged" condition whose summary
  // happens to mention a database resource shouldn't classify as page.
  const haystack = payload.incident.condition?.displayName ?? payload.incident.summary ?? '';
  if (PAGE_PATTERNS.some((p) => p.test(haystack))) return 'page';
  if (WARN_PATTERNS.some((p) => p.test(haystack))) return 'warn';
  return 'unknown';
}

/**
 * Maximum slug length so the full `gcp-policy:<slug>-<yyyymmdd>` label
 * stays under GitHub's 50-char label-name limit:
 *   `gcp-policy:` (11) + slug + `-yyyymmdd` (9) = 20 + slug ≤ 50  ⇒  slug ≤ 30.
 *
 * The numeric-policy-ID path (Cloud Monitoring's typical case, ~19 digits)
 * already fits without truncation. The displayName-fallback path is what
 * needed bounding — see #1786 for the smoke-test that surfaced this.
 */
const POLICY_SLUG_MAX_LEN = 30;

/**
 * Extracts a stable, human-meaningful slug from a policy name. Cloud
 * Monitoring uses opaque numeric IDs (e.g.
 * `projects/ripls-prod/alertPolicies/12345678901234567890`) which are
 * stable per environment but unhelpful in label names. We fall back to
 * the condition.displayName lowercased + slugified when policy_name
 * isn't available.
 *
 * Whichever path is taken, the returned slug is capped at 30 chars so
 * the GitHub label `gcp-policy:<slug>-<yyyymmdd>` stays under the
 * 50-char limit. The numeric-ID path almost always fits naturally
 * (~19 digits) and the cap is a defensive backstop.
 */
export function policySlug(payload: CloudMonitoringIncidentPayload): string {
  const policyId = payload.incident.policy_name?.split('/').pop();
  if (policyId && /^\d+$/.test(policyId)) {
    return policyId.slice(0, POLICY_SLUG_MAX_LEN);
  }
  // Fallback: slugify the condition display name. Replace runs of non-alnum
  // with single hyphens, trim leading/trailing hyphens, lowercase, then cap
  // at 30 chars. Trim trailing hyphens again *after* truncation so a
  // slice that lands mid-word doesn't leave a dangling hyphen.
  const display =
    payload.incident.condition?.displayName ?? payload.incident.summary ?? 'unknown';
  const truncated = display
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, POLICY_SLUG_MAX_LEN)
    .replace(/-+$/, '');
  return truncated || 'unknown';
}

/** YYYY-MM-DD in UTC for the incident's started_at (or now if absent). */
export function incidentDateUtc(payload: CloudMonitoringIncidentPayload): string {
  const ms = (payload.incident.started_at ?? Math.floor(Date.now() / 1000)) * 1000;
  return new Date(ms).toISOString().slice(0, 10);
}

/**
 * Strips PII from a key/value record. Drops any key matching email/phone
 * patterns; for the remaining values, masks anything that *looks* like an
 * email so a misnamed-but-PII-bearing field doesn't leak. Returns a fresh
 * object so the caller doesn't mutate the upstream payload.
 */
export function redactLabels(labels: Record<string, string> | undefined): Record<string, string> {
  if (!labels) return {};
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(labels)) {
    if (/(^|_)(email|phone|user_email|user_phone|phone_number)(_|$)/i.test(k)) {
      continue; // drop entirely
    }
    out[k] = maskEmailLikeValue(v);
  }
  return out;
}

function maskEmailLikeValue(v: string): string {
  const m = v.match(/^([^\s@]+)@([^\s@]+)$/);
  if (!m) return v;
  const user = m[1];
  const domain = m[2];
  const head = user.length <= 2 ? user[0] ?? '' : user.slice(0, 2);
  return `${head}***@${domain}`;
}

/** Builds the Markdown body posted on incident open/comment. */
export function buildIncidentBody(payload: CloudMonitoringIncidentPayload): string {
  const inc = payload.incident;
  const startedAt = inc.started_at
    ? new Date(inc.started_at * 1000).toISOString()
    : 'unknown';
  const lines: string[] = [
    `## ${inc.condition?.displayName ?? inc.summary ?? 'Cloud Monitoring incident'}`,
    '',
    `**Incident:** \`${inc.incident_id}\``,
    `**State:** ${inc.state}`,
    `**Started:** ${startedAt}`,
  ];
  if (inc.severity) lines.push(`**Severity (Cloud Monitoring):** ${inc.severity}`);
  if (inc.policy_name) lines.push(`**Policy:** \`${inc.policy_name}\``);
  if (inc.summary) lines.push('', `> ${inc.summary}`);
  if (inc.threshold_value || inc.observed_value) {
    lines.push(
      '',
      `**Threshold:** ${inc.threshold_value ?? 'n/a'} · **Observed:** ${inc.observed_value ?? 'n/a'}`,
    );
  }
  const resourceLabels = redactLabels(inc.resource?.labels);
  if (Object.keys(resourceLabels).length) {
    lines.push('', '**Resource labels:**');
    for (const [k, v] of Object.entries(resourceLabels)) {
      lines.push(`- \`${k}\`: \`${v}\``);
    }
  }
  const metricLabels = redactLabels(inc.metric?.labels);
  if (Object.keys(metricLabels).length) {
    lines.push('', '**Metric labels:**');
    for (const [k, v] of Object.entries(metricLabels)) {
      lines.push(`- \`${k}\`: \`${v}\``);
    }
  }
  if (inc.url) {
    lines.push('', `[View in Cloud Monitoring](${inc.url})`);
  }
  return lines.join('\n');
}

/** Body for the close-comment posted when an incident resolves. */
export function buildResolutionBody(payload: CloudMonitoringIncidentPayload): string {
  const endedAt = payload.incident.ended_at
    ? new Date(payload.incident.ended_at * 1000).toISOString()
    : new Date().toISOString();
  return `## Incident resolved

**Incident:** \`${payload.incident.incident_id}\`
**Ended:** ${endedAt}

Auto-closed by ripls-alerts on receipt of the \`state=closed\` Pub/Sub event.`;
}

/**
 * Pure-async handler for Cloud Monitoring alerts. Routes by severity +
 * state. Exported for direct unit testing; the trigger wrapper at the
 * bottom of this file adapts the firebase-functions event shape.
 */
export async function handleGcpAlert(
  payload: CloudMonitoringIncidentPayload,
  octokit: Octokit,
): Promise<void> {
  const severity = classifySeverity(payload);
  const inc = payload.incident;

  // Page-severity policies dedup per-incident with full open/close lifecycle.
  if (severity === 'page' || severity === 'unknown') {
    const dedupLabel = `gcp-incident:${inc.incident_id}`;
    if (inc.state === 'closed') {
      const result = await closeIssue({
        owner: REPO_OWNER,
        repo: REPO_NAME,
        dedupLabel,
        body: buildResolutionBody(payload),
        octokit,
      });
      logger.info('gcp incident close handled', {
        incidentId: inc.incident_id,
        severity,
        action: result.action,
        ...(result.action === 'closed' ? { githubIssue: result.issueNumber } : {}),
      });
      return;
    }
    const result = await upsertIssue({
      owner: REPO_OWNER,
      repo: REPO_NAME,
      dedupLabel,
      title: titleForPage(payload),
      body: buildIncidentBody(payload),
      extraLabels: extraLabelsForPage(payload, severity),
      reopenIfClosed: false,
      // Page-severity always opts into claude-fix; unknown severity never
      // does (we don't know what it is, no point burning Claude-API on a
      // mis-classified event).
      addClaudeFixLabel: severity === 'page',
      octokit,
    });
    logger.info('gcp incident open handled', {
      incidentId: inc.incident_id,
      severity,
      githubIssue: result.issueNumber,
      action: result.action,
    });
    return;
  }

  // Warn-severity: aggregate per-policy-per-day. closes don't fire — the
  // daily auto-close sweeps stale gcp-policy:* issues separately.
  if (inc.state === 'closed') {
    logger.debug('gcp warn incident closed; ignored (daily aggregation)', {
      incidentId: inc.incident_id,
    });
    return;
  }
  const date = incidentDateUtc(payload);
  const slug = policySlug(payload);
  const dedupLabel = `gcp-policy:${slug}-${date.replace(/-/g, '')}`;
  const result = await upsertIssue({
    owner: REPO_OWNER,
    repo: REPO_NAME,
    dedupLabel,
    title: titleForWarn(payload, date),
    body: buildIncidentBody(payload),
    extraLabels: ['ops', 'severity:warn'],
    reopenIfClosed: false,
    // claude-fix only when the issue is being created (first occurrence
    // of the day). upsertIssue's create-only label path means subsequent
    // occurrences within the same day won't re-fire Claude.
    addClaudeFixLabel: true,
    octokit,
  });
  logger.info('gcp warn incident handled', {
    incidentId: inc.incident_id,
    policySlug: slug,
    date,
    githubIssue: result.issueNumber,
    action: result.action,
  });
}

function titleForPage(payload: CloudMonitoringIncidentPayload): string {
  const condition = payload.incident.condition?.displayName;
  return `[ops:page] ${condition ?? payload.incident.summary ?? 'Cloud Monitoring incident'}`;
}

function titleForWarn(payload: CloudMonitoringIncidentPayload, date: string): string {
  const condition = payload.incident.condition?.displayName;
  return `[ops:warn] ${condition ?? 'policy'} — ${date}`;
}

function extraLabelsForPage(
  payload: CloudMonitoringIncidentPayload,
  severity: AlertSeverity,
): string[] {
  const labels: string[] = ['ops'];
  if (severity === 'page') labels.push('severity:page');
  if (severity === 'unknown') labels.push('severity:unknown');
  // For health_check_unhealthy alerts, the condition payload carries
  // dependency / backend metric labels that belong on the issue too so
  // flapping ai/anthropic-vs-gemini failures are dedup'd visibly.
  const dep = payload.incident.metric?.labels?.dependency;
  if (dep) labels.push(`dep:${dep}`);
  const backend = payload.incident.metric?.labels?.backend;
  if (backend) labels.push(`backend:${backend}`);
  return labels;
}

// ---------------------------------------------------------------------------
// Firebase trigger wrapper
// ---------------------------------------------------------------------------

/**
 * Parses the base64-encoded Cloud Monitoring payload from a Pub/Sub
 * message. Logs a warning and returns null if the payload doesn't have
 * the expected shape — caller should treat as a no-op rather than throw.
 */
export function parsePubsubMessage(
  message: MessagePublishedData['message'],
): CloudMonitoringIncidentPayload | null {
  // firebase-functions populates message.json when the publisher sends
  // application/json; Cloud Monitoring does. Fall back to manual decode
  // for other publishers (or older payload encodings).
  const candidate: unknown =
    (message as { json?: unknown }).json ??
    (message.data ? safeJsonParse(Buffer.from(message.data, 'base64').toString('utf-8')) : null);
  if (!candidate || typeof candidate !== 'object') return null;
  const c = candidate as Record<string, unknown>;
  const inc = c.incident as Record<string, unknown> | undefined;
  if (!inc || typeof inc.incident_id !== 'string' || typeof inc.state !== 'string') {
    return null;
  }
  return candidate as CloudMonitoringIncidentPayload;
}

function safeJsonParse(s: string): unknown {
  try {
    return JSON.parse(s);
  } catch {
    return null;
  }
}

export const gcpAlertsHandler = onMessagePublished(
  {
    topic: 'gcp-alerts-to-github',
    secrets: [githubAppPrivateKeyBase64],
  },
  async (event: CloudEvent<MessagePublishedData>) => {
    const payload = parsePubsubMessage(event.data.message);
    if (!payload) {
      logger.warn('gcp alerts handler: unparseable payload, dropping', {
        messageId: event.data.message.messageId,
      });
      return;
    }
    await handleGcpAlert(payload, octokitFromAppSecret());
  },
);
