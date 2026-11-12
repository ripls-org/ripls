// Label-based upsert for GitHub issues. Used by all alert-handler trigger
// types (Crashlytics, Cloud Monitoring) so a single dedup pattern governs
// every auto-filed issue.
//
// Dedup strategy: each call supplies a dedupLabel that is unique to the
// underlying incident (e.g., "crashlytics:abc123" for a Crashlytics issue,
// "gcp-incident:42" for a Cloud Monitoring incident, or
// "gcp-policy:all_errors-20260507" for an aggregated daily-summary policy).
// We list issues by that label; 0 hits → create, 1 open hit → comment,
// 1 closed hit → comment (and optionally reopen for regression cases).
//
// The dedupLabel must be applied to every auto-filed issue. Callers may
// supply extra labels (severity, platform, etc.) and may opt in to the
// `claude-fix` label which triggers the existing claude_plan_issue.yaml
// workflow. See docs/issues/1148-1149-alert-to-issue.md §"Claude-trigger
// gating" for when to opt in.

import type { Octokit } from '@octokit/rest';

/**
 * Standard label applied to every issue this helper creates. Used by humans
 * and search filters to find auto-filed issues vs human-filed issues.
 */
export const AUTO_FILED_LABEL = 'auto-filed';

/**
 * Label that triggers .github/workflows/claude_plan_issue.yaml. Adding this
 * label to an issue causes Claude to draft a plan PR. The workflow's
 * self-trigger guard skips events where github.actor === 'ripls-agent[bot]',
 * so the auto-filer's identity must NOT be ripls-agent[bot], and the agent
 * repo only accepts it because it is listed in AGENT_TRUSTED_BOTS (#2970).
 * See plan Q9.
 */
export const CLAUDE_FIX_LABEL = 'claude-fix';

export interface UpsertIssueOptions {
  /** Repo owner (e.g., "Type2Fun-Foundation"). */
  owner: string;
  /** Repo name (e.g., "ripls"). */
  repo: string;
  /**
   * Label that uniquely identifies this incident across firings. Must be
   * applied to the issue at creation. Examples:
   *   - "crashlytics:abc123"  (Crashlytics issue id)
   *   - "gcp-incident:42"     (Cloud Monitoring incident id)
   *   - "gcp-policy:all_errors-20260507" (per-day aggregation key)
   */
  dedupLabel: string;
  title: string;
  body: string;
  /** Additional labels (severity, platform, etc.). Auto-filed and dedup labels are added automatically. */
  extraLabels?: string[];
  /**
   * If true and the matched issue is closed, reopen it. Used for regression
   * alerts where a previously-fixed crash returns. Defaults to false.
   */
  reopenIfClosed?: boolean;
  /**
   * If true, add the claude-fix label so Claude drafts a plan automatically.
   * Defaults to false — the plan's gating section explains when to opt in.
   */
  addClaudeFixLabel?: boolean;
  /** Authenticated Octokit instance. Constructed by the caller from GH_AUTOMATION_TOKEN. */
  octokit: Octokit;
}

export type UpsertIssueResult =
  | { action: 'created'; issueNumber: number }
  | { action: 'commented'; issueNumber: number }
  | { action: 'reopened'; issueNumber: number };

/**
 * Upserts a GitHub issue identified by `dedupLabel`. Returns the action
 * taken so the caller can log it for observability.
 *
 * Throws on any GitHub API error — the caller (a Cloud Function) should
 * let the error propagate so the platform retries (Pub/Sub triggers) or
 * surfaces in Cloud Logging (Crashlytics triggers).
 */
export async function upsertIssue(
  options: UpsertIssueOptions,
): Promise<UpsertIssueResult> {
  const {
    owner,
    repo,
    dedupLabel,
    title,
    body,
    extraLabels = [],
    reopenIfClosed = false,
    addClaudeFixLabel = false,
    octokit,
  } = options;

  // GitHub's `listForRepo` with a `labels` filter is strongly consistent
  // (unlike the search API). Fetching state=all so we see closed issues for
  // the regression-reopen path. per_page=1 with descending creation date
  // gives us the most-recent issue with that label.
  const { data: candidates } = await octokit.issues.listForRepo({
    owner,
    repo,
    state: 'all',
    labels: dedupLabel,
    per_page: 1,
    sort: 'created',
    direction: 'desc',
  });

  // listForRepo returns both issues and PRs; PR objects have a `pull_request`
  // key. Defensive: if a PR was somehow given the dedup label, ignore it and
  // create a fresh issue.
  const existing = candidates.find((c) => !('pull_request' in c && c.pull_request));

  if (!existing) {
    const labels = buildLabelSet({ dedupLabel, extraLabels, addClaudeFixLabel });
    const { data: created } = await octokit.issues.create({
      owner,
      repo,
      title,
      body,
      labels,
    });
    return { action: 'created', issueNumber: created.number };
  }

  // Always comment on the existing issue with the new payload — the body
  // typically carries fresh stats (velocity %, occurrence count, etc.) that
  // are useful even if we don't reopen.
  await octokit.issues.createComment({
    owner,
    repo,
    issue_number: existing.number,
    body,
  });

  if (existing.state === 'closed' && reopenIfClosed) {
    await octokit.issues.update({
      owner,
      repo,
      issue_number: existing.number,
      state: 'open',
    });
    // If the caller opted in to claude-fix on this firing, ensure the label
    // is present even though the issue may have lost it during a previous
    // close. Idempotent — addLabels is upsert-shaped.
    if (addClaudeFixLabel) {
      await octokit.issues.addLabels({
        owner,
        repo,
        issue_number: existing.number,
        labels: [CLAUDE_FIX_LABEL],
      });
    }
    return { action: 'reopened', issueNumber: existing.number };
  }

  return { action: 'commented', issueNumber: existing.number };
}

interface BuildLabelSetOptions {
  dedupLabel: string;
  extraLabels: string[];
  addClaudeFixLabel: boolean;
}

function buildLabelSet({
  dedupLabel,
  extraLabels,
  addClaudeFixLabel,
}: BuildLabelSetOptions): string[] {
  const labels = new Set<string>([AUTO_FILED_LABEL, dedupLabel, ...extraLabels]);
  if (addClaudeFixLabel) {
    labels.add(CLAUDE_FIX_LABEL);
  }
  return Array.from(labels);
}

export interface CloseIssueOptions {
  owner: string;
  repo: string;
  /** Label that identifies the issue to close (must be unique). */
  dedupLabel: string;
  /** Comment body posted before closing (e.g., "Incident resolved at ..."). */
  body: string;
  octokit: Octokit;
}

export type CloseIssueResult =
  | { action: 'closed'; issueNumber: number }
  | { action: 'noop'; reason: 'not_found' };

/**
 * Closes the open GitHub issue matching the dedupLabel, after appending a
 * comment with the supplied body. Used by the Cloud Monitoring incident
 * lifecycle (state=closed → close the corresponding GH issue).
 *
 * No-op if no matching open issue exists (already closed by a human, or
 * the create path never fired in the first place — defensive). Throws
 * on any GitHub API error so the caller can surface to Cloud Logging.
 */
export async function closeIssue(
  options: CloseIssueOptions,
): Promise<CloseIssueResult> {
  const { owner, repo, dedupLabel, body, octokit } = options;

  const { data: candidates } = await octokit.issues.listForRepo({
    owner,
    repo,
    state: 'open',
    labels: dedupLabel,
    per_page: 1,
    sort: 'created',
    direction: 'desc',
  });

  const existing = candidates.find((c) => !('pull_request' in c && c.pull_request));
  if (!existing) {
    return { action: 'noop', reason: 'not_found' };
  }

  await octokit.issues.createComment({
    owner,
    repo,
    issue_number: existing.number,
    body,
  });
  await octokit.issues.update({
    owner,
    repo,
    issue_number: existing.number,
    state: 'closed',
  });
  return { action: 'closed', issueNumber: existing.number };
}
