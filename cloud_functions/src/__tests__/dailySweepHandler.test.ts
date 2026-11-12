import { describe, expect, test, vi } from 'vitest';
import type { Octokit } from '@octokit/rest';
import { runDailySweep } from '../dailySweepHandler';

const NOW = new Date('2026-05-08T09:00:00Z');

const issue = (
  number: number,
  hoursOld: number,
  labels: string[],
  extras: Record<string, unknown> = {},
) => ({
  number,
  state: 'open',
  created_at: new Date(NOW.getTime() - hoursOld * 60 * 60 * 1000).toISOString(),
  labels: labels.map((name) => ({ name })),
  ...extras,
});

const makeOctokit = (issues: ReturnType<typeof issue>[]) => {
  const listForRepo = vi.fn().mockResolvedValue({ data: issues });
  const createComment = vi.fn().mockResolvedValue({ data: {} });
  const update = vi.fn().mockResolvedValue({ data: {} });
  return {
    octokit: { issues: { listForRepo, createComment, update } } as unknown as Octokit,
    spies: { listForRepo, createComment, update },
  };
};

describe('runDailySweep', () => {
  test('closes gcp-policy issues older than 24h', async () => {
    const { octokit, spies } = makeOctokit([
      issue(1, 30, ['auto-filed', 'gcp-policy:9876-20260507']),
    ]);

    const report = await runDailySweep(octokit, NOW);

    expect(report).toEqual({ scanned: 1, closed: 1, skipped: 0 });
    expect(spies.update).toHaveBeenCalledWith(
      expect.objectContaining({ issue_number: 1, state: 'closed' }),
    );
    expect(spies.createComment).toHaveBeenCalledTimes(1);
  });

  test('skips fresh gcp-policy issues (under 24h)', async () => {
    const { octokit, spies } = makeOctokit([
      issue(2, 6, ['auto-filed', 'gcp-policy:9876-20260508']),
    ]);

    const report = await runDailySweep(octokit, NOW);

    expect(report).toEqual({ scanned: 1, closed: 0, skipped: 1 });
    expect(spies.update).not.toHaveBeenCalled();
  });

  test('ignores issues without a gcp-policy:* label', async () => {
    const { octokit, spies } = makeOctokit([
      issue(3, 30, ['auto-filed', 'gcp-incident:abc']), // page-severity, not warn
      issue(4, 30, ['auto-filed', 'crashlytics:xyz']), // crashlytics
      issue(5, 30, ['auto-filed', 'stability-digest']), // stability digest
    ]);

    const report = await runDailySweep(octokit, NOW);

    expect(report.scanned).toBe(0);
    expect(spies.update).not.toHaveBeenCalled();
  });

  test('handles a mix correctly: some closed, some skipped, some ignored', async () => {
    const { octokit, spies } = makeOctokit([
      issue(10, 30, ['auto-filed', 'gcp-policy:9876-20260507']), // close
      issue(11, 6, ['auto-filed', 'gcp-policy:9876-20260508']), // skip (fresh)
      issue(12, 30, ['auto-filed', 'gcp-incident:abc']), // ignore (page)
      issue(13, 48, ['auto-filed', 'gcp-policy:1234-20260506']), // close
    ]);

    const report = await runDailySweep(octokit, NOW);

    expect(report).toEqual({ scanned: 3, closed: 2, skipped: 1 });
    expect(spies.update).toHaveBeenCalledTimes(2);
    expect(spies.update.mock.calls.map((c) => c[0].issue_number).sort()).toEqual([10, 13]);
  });

  test('skips PRs that happen to carry a gcp-policy:* label', async () => {
    const { octokit, spies } = makeOctokit([
      issue(99, 48, ['auto-filed', 'gcp-policy:9876-20260506'], {
        pull_request: { url: 'https://...' },
      }),
    ]);

    const report = await runDailySweep(octokit, NOW);

    expect(report.scanned).toBe(0);
    expect(spies.update).not.toHaveBeenCalled();
  });

  test('list query filters to state=open and label=auto-filed', async () => {
    const { octokit, spies } = makeOctokit([]);
    await runDailySweep(octokit, NOW);
    expect(spies.listForRepo).toHaveBeenCalledWith(
      expect.objectContaining({
        state: 'open',
        labels: 'auto-filed',
      }),
    );
  });

  test('comment body explains why the issue was closed and how to reopen the loop', async () => {
    const { octokit, spies } = makeOctokit([
      issue(1, 30, ['auto-filed', 'gcp-policy:9876-20260507']),
    ]);
    await runDailySweep(octokit, NOW);

    const body = spies.createComment.mock.calls[0][0].body as string;
    expect(body).toContain('daily sweep');
    expect(body).toContain('older than 24h');
    expect(body).toMatch(/gcp-policy/);
  });

  test('handles legacy label shape where labels are bare strings', async () => {
    const { octokit, spies } = makeOctokit([
      // GitHub returns labels as objects but the type allows strings too;
      // be defensive against future schema changes.
      {
        number: 1,
        state: 'open',
        created_at: new Date(NOW.getTime() - 30 * 60 * 60 * 1000).toISOString(),
        labels: ['auto-filed', 'gcp-policy:abc-20260507'] as unknown as { name: string }[],
      },
    ]);

    const report = await runDailySweep(octokit, NOW);
    expect(report.closed).toBe(1);
    expect(spies.update).toHaveBeenCalledTimes(1);
  });
});
