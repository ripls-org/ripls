import { describe, expect, test, vi } from 'vitest';
import type { IssueTracker } from '../issueTracker';
import type {
  RegressionAlertPayload,
  StabilityDigestPayload,
  VelocityAlertPayload,
} from 'firebase-functions/v2/alerts/crashlytics';
import {
  buildRegressionIssueBody,
  buildStabilityDigestBody,
  buildVelocityIssueBody,
  CLAUDE_FIX_VELOCITY_THRESHOLD_PCT,
  coerceCrashlyticsDate,
  formatTimestamp,
  formatYMD,
  handleRegressionAlert,
  handleStabilityDigest,
  handleVelocityAlert,
  isoMondayOfWeek,
  shouldClaudeFixVelocity,
} from '../crashlyticsHandler';
import { AUTO_FILED_LABEL, CLAUDE_FIX_LABEL } from '../upsertIssue';

const baseIssue = {
  id: 'cl-issue-abc123',
  title: 'java.lang.NullPointerException at FooActivity.onCreate',
  subtitle: 'Crash in com.ripls.FooActivity:42',
  appVersion: '0.19.2',
};

const velocityPayload: VelocityAlertPayload = {
  '@type':
    'type.googleapis.com/google.events.firebase.firebasealerts.v1.CrashlyticsVelocityAlertPayload',
  issue: baseIssue,
  createTime: '2026-05-07T12:00:00Z',
  crashCount: 250,
  crashPercentage: 1.42,
  firstVersion: '0.19.0',
};

const regressionPayload: RegressionAlertPayload = {
  '@type':
    'type.googleapis.com/google.events.firebase.firebasealerts.v1.CrashlyticsRegressionAlertPayload',
  issue: baseIssue,
  type: 'fatal',
  resolveTime: '2026-04-15T08:00:00Z',
};

const makeTracker = () => {
  const listForRepo = vi.fn().mockResolvedValue({ data: [] });
  const create = vi.fn().mockResolvedValue({ data: { number: 700 } });
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

describe('shouldClaudeFixVelocity threshold', () => {
  test('returns true at the threshold (>= rule, not >)', () => {
    expect(
      shouldClaudeFixVelocity({
        ...velocityPayload,
        crashPercentage: CLAUDE_FIX_VELOCITY_THRESHOLD_PCT,
      }),
    ).toBe(true);
  });

  test('returns true above the threshold', () => {
    expect(
      shouldClaudeFixVelocity({ ...velocityPayload, crashPercentage: 5 }),
    ).toBe(true);
  });

  test('returns false below the threshold', () => {
    expect(
      shouldClaudeFixVelocity({ ...velocityPayload, crashPercentage: 0.4 }),
    ).toBe(false);
  });
});

describe('buildVelocityIssueBody', () => {
  test('includes Crashlytics issue id, version, percent, count, and console link', () => {
    const body = buildVelocityIssueBody(velocityPayload);
    expect(body).toContain('cl-issue-abc123');
    expect(body).toContain('0.19.2');
    expect(body).toContain('1.42');
    expect(body).toContain('250');
    expect(body).toContain('console.firebase.google.com');
    expect(body).toContain('crashlytics:cl-issue-abc123');
  });
});

describe('buildRegressionIssueBody', () => {
  test('includes type, resolve time, issue id, and dedup label', () => {
    const body = buildRegressionIssueBody(regressionPayload);
    expect(body).toContain('cl-issue-abc123');
    expect(body).toContain('fatal');
    expect(body).toContain('2026-04-15T08:00:00Z');
    expect(body).toContain('crashlytics:cl-issue-abc123');
  });
});

describe('handleVelocityAlert', () => {
  test('files an issue with crash + platform:android labels and dedup label', async () => {
    const { tracker, spies } = makeTracker();
    await handleVelocityAlert(velocityPayload, tracker);

    expect(spies.create).toHaveBeenCalledTimes(1);
    const call = spies.create.mock.calls[0][0];
    expect(call.title).toContain(baseIssue.title);
    expect(call.labels).toContain(AUTO_FILED_LABEL);
    expect(call.labels).toContain('crashlytics:cl-issue-abc123');
    expect(call.labels).toContain('crash');
    // baseIssue title contains "java.lang" → Android.
    expect(call.labels).toContain('platform:android');
  });

  test('opts into claude-fix when crashPercentage >= threshold', async () => {
    const { tracker, spies } = makeTracker();
    await handleVelocityAlert(
      { ...velocityPayload, crashPercentage: 2 },
      tracker,
    );

    expect(spies.create.mock.calls[0][0].labels).toContain(CLAUDE_FIX_LABEL);
  });

  test('does NOT add claude-fix when crashPercentage below threshold', async () => {
    const { tracker, spies } = makeTracker();
    await handleVelocityAlert(
      { ...velocityPayload, crashPercentage: 0.4 },
      tracker,
    );

    const labels = spies.create.mock.calls[0][0].labels as string[];
    expect(labels).not.toContain(CLAUDE_FIX_LABEL);
  });

  test('comments rather than creating when issue already exists for same Crashlytics id', async () => {
    const { tracker, spies } = makeTracker();
    spies.listForRepo.mockResolvedValueOnce({
      data: [{ number: 99, state: 'open' }],
    });

    await handleVelocityAlert(velocityPayload, tracker);

    expect(spies.create).not.toHaveBeenCalled();
    expect(spies.createComment).toHaveBeenCalledTimes(1);
  });

  test('does not reopen a closed issue (velocity is not a regression signal)', async () => {
    const { tracker, spies } = makeTracker();
    spies.listForRepo.mockResolvedValueOnce({
      data: [{ number: 99, state: 'closed' }],
    });

    await handleVelocityAlert(velocityPayload, tracker);

    expect(spies.update).not.toHaveBeenCalled();
    expect(spies.createComment).toHaveBeenCalledTimes(1);
  });
});

describe('handleRegressionAlert', () => {
  test('files with regression label and always opts into claude-fix', async () => {
    const { tracker, spies } = makeTracker();
    await handleRegressionAlert(regressionPayload, tracker);

    const labels = spies.create.mock.calls[0][0].labels as string[];
    expect(labels).toContain('crash');
    expect(labels).toContain('regression');
    expect(labels).toContain(CLAUDE_FIX_LABEL);
    expect(labels).toContain('crashlytics:cl-issue-abc123');
  });

  test('reopens a closed issue (the whole point of regression alerts)', async () => {
    const { tracker, spies } = makeTracker();
    spies.listForRepo.mockResolvedValueOnce({
      data: [{ number: 50, state: 'closed' }],
    });

    await handleRegressionAlert(regressionPayload, tracker);

    expect(spies.update).toHaveBeenCalledWith(
      expect.objectContaining({ issue_number: 50, state: 'open' }),
    );
    expect(spies.addLabels).toHaveBeenCalledWith(
      expect.objectContaining({
        issue_number: 50,
        labels: [CLAUDE_FIX_LABEL],
      }),
    );
  });
});

describe('isoMondayOfWeek', () => {
  test('Monday returns the same date', () => {
    // 2026-05-04 is a Monday.
    expect(isoMondayOfWeek('2026-05-04')).toBe('2026-05-04');
  });

  test('Tuesday rolls back one day', () => {
    expect(isoMondayOfWeek('2026-05-05')).toBe('2026-05-04');
  });

  test('Sunday rolls back six days', () => {
    // 2026-05-10 is a Sunday → previous Monday is 2026-05-04.
    expect(isoMondayOfWeek('2026-05-10')).toBe('2026-05-04');
  });

  test('handles month and year boundaries', () => {
    // 2026-01-03 is a Saturday → previous Monday is 2025-12-29.
    expect(isoMondayOfWeek('2026-01-03')).toBe('2025-12-29');
  });

  test('throws on invalid date string', () => {
    expect(() => isoMondayOfWeek('not-a-date')).toThrow(/invalid digestDate/);
  });

  test('accepts a google.type.Date proto shape (#1804)', () => {
    // The runtime payload from Firebase Crashlytics stability digests
    // sometimes arrives as { year, month, day } despite the SDK type
    // declaring it as `string`. 2026-05-08 is a Friday → Monday is
    // 2026-05-04.
    expect(
      isoMondayOfWeek({ year: 2026, month: 5, day: 8 } as unknown as string),
    ).toBe('2026-05-04');
  });

  test('accepts a google.protobuf.Timestamp proto shape', () => {
    // 2026-05-04T00:00:00Z = 1778198400 seconds since epoch.
    expect(
      isoMondayOfWeek({ seconds: 1778198400 } as unknown as string),
    ).toBe('2026-05-04');
  });

  test('error message includes JSON-serialized raw value', () => {
    expect(() =>
      isoMondayOfWeek({ wrongShape: true } as unknown as string),
    ).toThrow(/wrongShape/);
  });
});

describe('coerceCrashlyticsDate', () => {
  test('returns Date instances unchanged', () => {
    const d = new Date('2026-05-04T00:00:00Z');
    expect(coerceCrashlyticsDate(d).getTime()).toBe(d.getTime());
  });

  test('parses YYYY-MM-DD strings as UTC midnight', () => {
    expect(coerceCrashlyticsDate('2026-05-04').toISOString()).toBe(
      '2026-05-04T00:00:00.000Z',
    );
  });

  test('parses ISO datetime strings', () => {
    expect(coerceCrashlyticsDate('2026-05-04T12:34:56Z').toISOString()).toBe(
      '2026-05-04T12:34:56.000Z',
    );
  });

  test('parses google.type.Date {year, month, day}', () => {
    // month is 1-based per the proto.
    expect(coerceCrashlyticsDate({ year: 2026, month: 1, day: 1 }).toISOString())
      .toBe('2026-01-01T00:00:00.000Z');
  });

  test('parses google.protobuf.Timestamp {seconds, nanos}', () => {
    // 2026-05-04T00:00:00Z = 1777852800 seconds since epoch.
    const d = coerceCrashlyticsDate({ seconds: 1777852800, nanos: 500_000_000 });
    expect(d.toISOString()).toBe('2026-05-04T00:00:00.500Z');
  });

  test('parses Timestamp with string seconds (proto3 JSON convention)', () => {
    expect(
      coerceCrashlyticsDate({ seconds: '1777852800' }).toISOString(),
    ).toBe('2026-05-04T00:00:00.000Z');
  });

  test('throws on null', () => {
    expect(() => coerceCrashlyticsDate(null)).toThrow(/invalid digestDate/);
  });

  test('throws on unrecognized object shape', () => {
    expect(() => coerceCrashlyticsDate({ foo: 'bar' })).toThrow(
      /invalid digestDate/,
    );
  });
});

describe('formatYMD', () => {
  test('formats UTC midnight', () => {
    expect(formatYMD(new Date('2026-05-04T00:00:00Z'))).toBe('2026-05-04');
  });

  test('truncates time-of-day', () => {
    expect(formatYMD(new Date('2026-05-04T23:59:59Z'))).toBe('2026-05-04');
  });
});

describe('formatTimestamp', () => {
  test('returns strings unchanged', () => {
    expect(formatTimestamp('2026-05-07T12:00:00Z')).toBe('2026-05-07T12:00:00Z');
  });

  test('formats google.type.Date as ISO datetime', () => {
    expect(formatTimestamp({ year: 2026, month: 5, day: 7 })).toBe(
      '2026-05-07T00:00:00.000Z',
    );
  });

  test('formats Timestamp proto as ISO datetime', () => {
    // 2026-05-04T00:00:00Z = 1777852800 seconds since epoch.
    expect(formatTimestamp({ seconds: 1777852800 })).toBe(
      '2026-05-04T00:00:00.000Z',
    );
  });

  test('falls back to JSON dump on unrecognized shape (never `[object Object]`)', () => {
    expect(formatTimestamp({ unknown: 1 })).toBe('{"unknown":1}');
  });
});

describe('handleStabilityDigest with object-shaped digestDate (#1804)', () => {
  test('files an issue with correct dedup label when digestDate is google.type.Date', async () => {
    const { tracker, spies } = makeTracker();
    await handleStabilityDigest(
      {
        ...stabilityDigestPayload,
        // Cast through unknown because the SDK type insists on string.
        digestDate: { year: 2026, month: 5, day: 8 } as unknown as string,
      },
      tracker,
    );

    expect(spies.create).toHaveBeenCalledTimes(1);
    const call = spies.create.mock.calls[0][0];
    // 2026-05-08 is a Friday → Monday of the same week is 2026-05-04.
    expect(call.title).toBe('[stability digest] week of 2026-05-04');
    expect(call.labels).toContain('crashlytics-digest-week:2026-05-04');
    expect(call.body).toContain('Stability digest for 2026-05-08');
    expect(call.body).not.toContain('[object Object]');
  });
});

describe('buildVelocityIssueBody with object-shaped createTime (#1804)', () => {
  test('renders timestamp instead of `[object Object]`', () => {
    const body = buildVelocityIssueBody({
      ...velocityPayload,
      // 2026-05-04T00:00:00Z = 1777852800 seconds since epoch.
      createTime: { seconds: 1777852800 } as unknown as string,
    });
    expect(body).not.toContain('[object Object]');
    expect(body).toContain('2026-05-04T00:00:00.000Z');
  });

  test('coerces string crashPercentage so .toFixed never throws', () => {
    const body = buildVelocityIssueBody({
      ...velocityPayload,
      crashPercentage: '1.42' as unknown as number,
    });
    expect(body).toContain('1.42%');
  });
});

describe('buildRegressionIssueBody with object-shaped resolveTime (#1804)', () => {
  test('renders timestamp instead of `[object Object]`', () => {
    const body = buildRegressionIssueBody({
      ...regressionPayload,
      // 2026-05-04T00:00:00Z = 1777852800 seconds since epoch.
      resolveTime: { seconds: 1777852800 } as unknown as string,
    });
    expect(body).not.toContain('[object Object]');
    expect(body).toContain('2026-05-04T00:00:00.000Z');
  });
});

const stabilityDigestPayload: StabilityDigestPayload = {
  '@type':
    'type.googleapis.com/google.events.firebase.firebasealerts.v1.CrashlyticsStabilityDigestPayload',
  digestDate: '2026-05-05', // Tuesday → ISO Monday is 2026-05-04
  trendingIssues: [
    {
      type: 'fatal',
      issue: { ...baseIssue, id: 'cl-trend-1', title: 'permission_denied trace' },
      eventCount: 240,
      userCount: 18,
    },
    {
      type: 'nonFatal',
      issue: { ...baseIssue, id: 'cl-trend-2', title: 'IndexOutOfBoundsException at FooList' },
      eventCount: 90,
      userCount: 7,
    },
  ],
};

describe('buildStabilityDigestBody', () => {
  test('includes digest date, trending count, and per-issue links', () => {
    const body = buildStabilityDigestBody(stabilityDigestPayload);
    expect(body).toContain('Stability digest for 2026-05-05');
    expect(body).toContain('Top 2 trending issues');
    expect(body).toContain('cl-trend-1');
    expect(body).toContain('cl-trend-2');
    expect(body).toContain('240 events');
    expect(body).toContain('18 users');
    expect(body).toContain('crashlytics-digest-week:2026-05-04');
  });
});

describe('handleStabilityDigest', () => {
  test('creates a weekly rolling issue with stability-digest + claude-fix labels', async () => {
    const { tracker, spies } = makeTracker();
    await handleStabilityDigest(stabilityDigestPayload, tracker);

    expect(spies.create).toHaveBeenCalledTimes(1);
    const call = spies.create.mock.calls[0][0];
    expect(call.title).toBe('[stability digest] week of 2026-05-04');
    expect(call.labels).toContain(AUTO_FILED_LABEL);
    expect(call.labels).toContain('crashlytics-digest-week:2026-05-04');
    expect(call.labels).toContain('stability-digest');
    expect(call.labels).toContain(CLAUDE_FIX_LABEL);
  });

  test('subsequent days within the same week comment on the existing issue', async () => {
    // First digest of the week (Monday) creates the issue. The mock returns
    // it on the second call so the second handler invocation hits the
    // existing-issue path.
    const { tracker, spies } = makeTracker();
    spies.listForRepo
      .mockResolvedValueOnce({ data: [] })
      .mockResolvedValueOnce({ data: [{ number: 333, state: 'open' }] });

    await handleStabilityDigest(
      { ...stabilityDigestPayload, digestDate: '2026-05-04' },
      tracker,
    );
    await handleStabilityDigest(
      { ...stabilityDigestPayload, digestDate: '2026-05-05' },
      tracker,
    );

    expect(spies.create).toHaveBeenCalledTimes(1); // one create on day 1
    expect(spies.createComment).toHaveBeenCalledTimes(1); // comment on day 2
    expect(spies.createComment.mock.calls[0][0].issue_number).toBe(333);
  });

  test('a digest from the next week creates a fresh issue (distinct dedup label)', async () => {
    const { tracker, spies } = makeTracker();
    // Two distinct weeks: 2026-05-04 (Monday wk1) and 2026-05-11 (Monday wk2).
    await handleStabilityDigest(
      { ...stabilityDigestPayload, digestDate: '2026-05-04' },
      tracker,
    );
    await handleStabilityDigest(
      { ...stabilityDigestPayload, digestDate: '2026-05-11' },
      tracker,
    );

    expect(spies.create).toHaveBeenCalledTimes(2);
    const wk1Labels = spies.create.mock.calls[0][0].labels as string[];
    const wk2Labels = spies.create.mock.calls[1][0].labels as string[];
    expect(wk1Labels).toContain('crashlytics-digest-week:2026-05-04');
    expect(wk2Labels).toContain('crashlytics-digest-week:2026-05-11');
    // listForRepo is called per upsert with the week-specific label, so the
    // two weeks dedup independently.
    expect(spies.listForRepo.mock.calls[0][0].labels).toBe(
      'crashlytics-digest-week:2026-05-04',
    );
    expect(spies.listForRepo.mock.calls[1][0].labels).toBe(
      'crashlytics-digest-week:2026-05-11',
    );
  });
});

describe('platform inference', () => {
  test('iOS-flavored title gets platform:ios', async () => {
    const { tracker, spies } = makeTracker();
    await handleVelocityAlert(
      {
        ...velocityPayload,
        issue: {
          ...baseIssue,
          title: 'NSException: NSInvalidArgumentException at UIKit',
          subtitle: 'Crash in Foundation.NSArray',
        },
      },
      tracker,
    );

    const labels = spies.create.mock.calls[0][0].labels as string[];
    expect(labels).toContain('platform:ios');
    expect(labels).not.toContain('platform:android');
  });

  test('ambiguous titles get no platform label (rather than guessing)', async () => {
    const { tracker, spies } = makeTracker();
    await handleVelocityAlert(
      {
        ...velocityPayload,
        issue: {
          ...baseIssue,
          title: 'Generic crash',
          subtitle: 'No idea',
        },
      },
      tracker,
    );

    const labels = spies.create.mock.calls[0][0].labels as string[];
    expect(labels.some((l) => l.startsWith('platform:'))).toBe(false);
  });
});
