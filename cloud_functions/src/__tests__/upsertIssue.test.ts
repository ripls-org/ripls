import { describe, expect, test, vi } from 'vitest';
import type { IssueTracker } from '../issueTracker';
import {
  AUTO_FILED_LABEL,
  CLAUDE_FIX_LABEL,
  closeIssue,
  upsertIssue,
} from '../upsertIssue';

// Build a minimally-typed fake IssueTracker. We only use the four endpoints
// upsertIssue calls; the rest of the surface area is irrelevant.
interface FakeIssue {
  number: number;
  state: 'open' | 'closed';
  pull_request?: { url: string };
}

const makeTracker = (existing: FakeIssue[] = [], createdNumber = 999) => {
  const listForRepo = vi.fn().mockResolvedValue({ data: existing });
  const create = vi.fn().mockResolvedValue({ data: { number: createdNumber } });
  const update = vi.fn().mockResolvedValue({ data: {} });
  const createComment = vi.fn().mockResolvedValue({ data: {} });
  const addLabels = vi.fn().mockResolvedValue({ data: {} });
  return {
    tracker: {
      issues: { listForRepo, create, update, createComment, addLabels },
    } as unknown as IssueTracker,
    spies: { listForRepo, create, update, createComment, addLabels },
  };
};

const baseArgs = {
  owner: 'Type2Fun-Foundation',
  repo: 'ripls',
  dedupLabel: 'crashlytics:abc123',
  title: 'Fatal: java.lang.NullPointerException at FooActivity:42',
  body: '## Summary\nVelocity 1.2% in last 1h\n\n[Firebase console](https://...)',
};

describe('upsertIssue', () => {
  test('creates a new issue when no existing match (auto-filed + dedup label always set)', async () => {
    const { tracker, spies } = makeTracker([], 555);

    const result = await upsertIssue({
      ...baseArgs,
      tracker,
    });

    expect(result).toEqual({ action: 'created', issueNumber: 555 });
    expect(spies.create).toHaveBeenCalledTimes(1);
    expect(spies.createComment).not.toHaveBeenCalled();
    expect(spies.update).not.toHaveBeenCalled();

    const created = spies.create.mock.calls[0][0];
    expect(created.title).toBe(baseArgs.title);
    expect(created.body).toBe(baseArgs.body);
    expect(created.labels).toContain(AUTO_FILED_LABEL);
    expect(created.labels).toContain(baseArgs.dedupLabel);
    expect(created.labels).not.toContain(CLAUDE_FIX_LABEL);
  });

  test('includes extra labels and dedupes against duplicates', async () => {
    const { tracker, spies } = makeTracker([]);

    await upsertIssue({
      ...baseArgs,
      extraLabels: ['crash', 'platform:android', AUTO_FILED_LABEL /* dup */],
      tracker,
    });

    const labels = spies.create.mock.calls[0][0].labels as string[];
    expect(labels).toContain('crash');
    expect(labels).toContain('platform:android');
    // AUTO_FILED_LABEL appears exactly once even though caller passed it twice.
    expect(labels.filter((l) => l === AUTO_FILED_LABEL)).toHaveLength(1);
  });

  test('adds claude-fix label when addClaudeFixLabel=true', async () => {
    const { tracker, spies } = makeTracker([]);

    await upsertIssue({
      ...baseArgs,
      addClaudeFixLabel: true,
      tracker,
    });

    const labels = spies.create.mock.calls[0][0].labels as string[];
    expect(labels).toContain(CLAUDE_FIX_LABEL);
  });

  test('comments on existing open issue without creating a duplicate', async () => {
    const { tracker, spies } = makeTracker([{ number: 42, state: 'open' }]);

    const result = await upsertIssue({ ...baseArgs, tracker });

    expect(result).toEqual({ action: 'commented', issueNumber: 42 });
    expect(spies.create).not.toHaveBeenCalled();
    expect(spies.createComment).toHaveBeenCalledTimes(1);
    expect(spies.createComment.mock.calls[0][0]).toMatchObject({
      issue_number: 42,
      body: baseArgs.body,
    });
    expect(spies.update).not.toHaveBeenCalled();
  });

  test('reopens a closed issue when reopenIfClosed=true (regression case)', async () => {
    const { tracker, spies } = makeTracker([{ number: 42, state: 'closed' }]);

    const result = await upsertIssue({
      ...baseArgs,
      reopenIfClosed: true,
      tracker,
    });

    expect(result).toEqual({ action: 'reopened', issueNumber: 42 });
    expect(spies.createComment).toHaveBeenCalledTimes(1);
    expect(spies.update).toHaveBeenCalledWith({
      owner: baseArgs.owner,
      repo: baseArgs.repo,
      issue_number: 42,
      state: 'open',
    });
  });

  test('does not reopen a closed issue when reopenIfClosed=false', async () => {
    const { tracker, spies } = makeTracker([{ number: 42, state: 'closed' }]);

    const result = await upsertIssue({
      ...baseArgs,
      reopenIfClosed: false,
      tracker,
    });

    expect(result).toEqual({ action: 'commented', issueNumber: 42 });
    expect(spies.update).not.toHaveBeenCalled();
  });

  test('on regression-reopen with addClaudeFixLabel, restores the claude-fix label', async () => {
    // Simulates a closed issue that lost the claude-fix label (e.g., human
    // removed it during a prior triage). On regression we re-add it so the
    // existing claude_plan_issue.yaml workflow re-fires.
    const { tracker, spies } = makeTracker([{ number: 42, state: 'closed' }]);

    await upsertIssue({
      ...baseArgs,
      reopenIfClosed: true,
      addClaudeFixLabel: true,
      tracker,
    });

    expect(spies.addLabels).toHaveBeenCalledWith({
      owner: baseArgs.owner,
      repo: baseArgs.repo,
      issue_number: 42,
      labels: [CLAUDE_FIX_LABEL],
    });
  });

  test('regression-reopen without addClaudeFixLabel does NOT add claude-fix', async () => {
    const { tracker, spies } = makeTracker([{ number: 42, state: 'closed' }]);

    await upsertIssue({
      ...baseArgs,
      reopenIfClosed: true,
      addClaudeFixLabel: false,
      tracker,
    });

    expect(spies.addLabels).not.toHaveBeenCalled();
  });

  test('ignores PRs that happen to carry the dedup label', async () => {
    // A defensive case: if a PR (e.g., the auto-filed plan PR) somehow ends
    // up with `crashlytics:abc123`, listForRepo will return it because the
    // GitHub API returns issues + PRs for that endpoint. We skip PRs and
    // create a fresh issue.
    const { tracker, spies } = makeTracker([
      { number: 999, state: 'open', pull_request: { url: 'https://...' } },
    ]);

    const result = await upsertIssue({ ...baseArgs, tracker });

    expect(result.action).toBe('created');
    expect(spies.create).toHaveBeenCalledTimes(1);
    expect(spies.createComment).not.toHaveBeenCalled();
  });

  test('list query uses state=all so closed issues match (regression path)', async () => {
    const { tracker, spies } = makeTracker([]);
    await upsertIssue({ ...baseArgs, tracker });

    expect(spies.listForRepo).toHaveBeenCalledWith(
      expect.objectContaining({
        owner: baseArgs.owner,
        repo: baseArgs.repo,
        state: 'all',
        labels: baseArgs.dedupLabel,
        per_page: 1,
      }),
    );
  });
});

describe('closeIssue', () => {
  const closeArgs = {
    owner: 'Type2Fun-Foundation',
    repo: 'ripls',
    dedupLabel: 'gcp-incident:42',
    body: 'Incident resolved at 2026-05-07T10:00:00Z',
  };

  test('comments and closes the matching open issue', async () => {
    const { tracker, spies } = makeTracker([{ number: 77, state: 'open' }]);

    const result = await closeIssue({ ...closeArgs, tracker });

    expect(result).toEqual({ action: 'closed', issueNumber: 77 });
    expect(spies.createComment).toHaveBeenCalledWith({
      owner: closeArgs.owner,
      repo: closeArgs.repo,
      issue_number: 77,
      body: closeArgs.body,
    });
    expect(spies.update).toHaveBeenCalledWith({
      owner: closeArgs.owner,
      repo: closeArgs.repo,
      issue_number: 77,
      state: 'closed',
    });
  });

  test('listForRepo uses state=open (does not waste calls on already-closed issues)', async () => {
    const { tracker, spies } = makeTracker([]);
    await closeIssue({ ...closeArgs, tracker });
    expect(spies.listForRepo.mock.calls[0][0].state).toBe('open');
  });

  test('returns noop when no matching issue exists', async () => {
    const { tracker, spies } = makeTracker([]);

    const result = await closeIssue({ ...closeArgs, tracker });

    expect(result).toEqual({ action: 'noop', reason: 'not_found' });
    expect(spies.createComment).not.toHaveBeenCalled();
    expect(spies.update).not.toHaveBeenCalled();
  });

  test('skips PRs that happen to carry the dedup label', async () => {
    const { tracker, spies } = makeTracker([
      { number: 999, state: 'open', pull_request: { url: 'https://...' } },
    ]);

    const result = await closeIssue({ ...closeArgs, tracker });

    expect(result.action).toBe('noop');
    expect(spies.update).not.toHaveBeenCalled();
  });
});
