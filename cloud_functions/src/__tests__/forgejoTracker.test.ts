import { describe, expect, test } from 'vitest';
import { forgejoTracker } from '../forgejoTracker';

// A Forgejo API for one repository, enough for the five operations. Records
// every request so tests can assert on what was sent.
interface Call {
  method: string;
  path: string;
  query: URLSearchParams;
  body: unknown;
  auth: string | null;
}

const repo = { owner: 'acme', repo: 'tracker' };

function fakeForgejo(opts: {
  labels?: Array<{ id: number; name: string }>;
  issues?: Array<Record<string, unknown>>;
  failWith?: { status: number; body: string };
}) {
  const calls: Call[] = [];
  const labels = [...(opts.labels ?? [])];
  let nextLabelId = 1000;
  const page = <T>(items: T[], query: URLSearchParams) => {
    const n = Number(query.get('page') ?? '1');
    const limit = Number(query.get('limit') ?? '50');
    return items.slice((n - 1) * limit, n * limit);
  };
  const json = (status: number, value: unknown) =>
    new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } });

  const fetchImpl = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? 'GET';
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    const headers = new Headers(init?.headers);
    calls.push({ method, path: url.pathname, query: url.searchParams, body, auth: headers.get('Authorization') });
    if (opts.failWith) return new Response(opts.failWith.body, { status: opts.failWith.status });

    const base = '/api/v1/repos/acme/tracker';
    if (method === 'GET' && url.pathname === `${base}/labels`) return json(200, page(labels, url.searchParams));
    if (method === 'POST' && url.pathname === `${base}/labels`) {
      const created = { id: (nextLabelId += 1), name: (body as { name: string }).name };
      labels.push(created);
      return json(201, created);
    }
    if (method === 'GET' && url.pathname === `${base}/issues`) return json(200, page(opts.issues ?? [], url.searchParams));
    if (method === 'POST' && url.pathname === `${base}/issues`) return json(201, { number: 7 });
    if (method === 'POST' && /\/issues\/\d+\/(comments|labels)$/.test(url.pathname)) return json(201, {});
    if (method === 'PATCH' && /\/issues\/\d+$/.test(url.pathname)) return json(201, {});
    return json(404, { message: `unexpected ${method} ${url.pathname}` });
  }) as typeof fetch;

  return { fetchImpl, calls };
}

describe('forgejoTracker', () => {
  test('create resolves label names to ids and creates the ones that are missing', async () => {
    const { fetchImpl, calls } = fakeForgejo({ labels: [{ id: 3, name: 'auto-filed' }] });
    const tracker = forgejoTracker('https://code.example.org/', 'secret-token', fetchImpl);

    const { data } = await tracker.issues.create({
      ...repo,
      title: 'Crash',
      body: 'details',
      labels: ['auto-filed', 'crashlytics:abc123'],
    });

    expect(data.number).toBe(7);
    const newLabel = calls.find((c) => c.method === 'POST' && c.path.endsWith('/labels'));
    expect(newLabel?.body).toEqual({ name: 'crashlytics:abc123', color: '#ededed' });
    const issue = calls.find((c) => c.method === 'POST' && c.path.endsWith('/issues'));
    expect(issue?.body).toEqual({ title: 'Crash', body: 'details', labels: [3, 1001] });
    // The trailing slash on the base URL did not double up, and every call
    // carried the token.
    expect(calls.every((c) => c.path.startsWith('/api/v1/repos/acme/tracker/'))).toBe(true);
    expect(calls.every((c) => c.auth === 'token secret-token')).toBe(true);
  });

  test('a label on the second page is found, not created again', async () => {
    const labels = Array.from({ length: 50 }, (_, i) => ({ id: i + 1, name: `filler-${i}` }));
    labels.push({ id: 777, name: 'ops' });
    const { fetchImpl, calls } = fakeForgejo({ labels });
    const tracker = forgejoTracker('https://code.example.org', 't', fetchImpl);

    await tracker.issues.addLabels({ ...repo, issue_number: 12, labels: ['ops'] });

    expect(calls.filter((c) => c.method === 'POST' && c.path.endsWith('/labels') && !c.path.includes('/issues/'))).toHaveLength(0);
    const added = calls.find((c) => c.path.endsWith('/issues/12/labels'));
    expect(added?.body).toEqual({ labels: [777] });
  });

  test('listForRepo asks for issues only, by label and state, and returns the newest first', async () => {
    const { fetchImpl, calls } = fakeForgejo({
      issues: [
        { number: 1, state: 'closed', created_at: '2026-05-01T00:00:00Z', labels: [{ name: 'crashlytics:abc123' }] },
        { number: 3, state: 'open', created_at: '2026-05-03T00:00:00Z', labels: [{ name: 'crashlytics:abc123' }] },
        { number: 2, state: 'open', created_at: '2026-05-02T00:00:00Z', labels: [{ name: 'crashlytics:abc123' }] },
      ],
    });
    const tracker = forgejoTracker('https://code.example.org', 't', fetchImpl);

    const { data } = await tracker.issues.listForRepo({
      ...repo,
      state: 'all',
      labels: 'crashlytics:abc123',
      per_page: 1,
      sort: 'created',
      direction: 'desc',
    });

    expect(data.map((i) => i.number)).toEqual([3]);
    const list = calls[0];
    expect(list.query.get('type')).toBe('issues');
    expect(list.query.get('labels')).toBe('crashlytics:abc123');
    expect(list.query.get('state')).toBe('all');
  });

  test('listForRepo reads every page and can return the oldest first', async () => {
    const issues = Array.from({ length: 60 }, (_, i) => ({
      number: i + 1,
      state: 'open',
      created_at: new Date(Date.UTC(2026, 0, 1) + (60 - i) * 60_000).toISOString(),
      labels: [{ name: 'auto-filed' }],
    }));
    const { fetchImpl, calls } = fakeForgejo({ issues });
    const tracker = forgejoTracker('https://code.example.org', 't', fetchImpl);

    const { data } = await tracker.issues.listForRepo({
      ...repo,
      state: 'open',
      labels: 'auto-filed',
      per_page: 100,
      sort: 'created',
      direction: 'asc',
    });

    expect(data).toHaveLength(60);
    expect(data[0].number).toBe(60); // created last in the list, earliest in time
    expect(calls.filter((c) => c.path.endsWith('/issues'))).toHaveLength(2);
  });

  test('issues without the label are dropped: Forgejo ignores a filter naming an unknown label', async () => {
    // What the server does when the dedup label does not exist yet: it
    // answers with every issue in the repository.
    const { fetchImpl } = fakeForgejo({
      issues: [
        { number: 9, state: 'open', created_at: '2026-05-03T00:00:00Z', labels: [{ name: 'bug' }] },
        { number: 8, state: 'open', created_at: '2026-05-02T00:00:00Z', labels: [] },
        { number: 7, state: 'open', created_at: '2026-05-01T00:00:00Z' },
      ],
    });
    const tracker = forgejoTracker('https://code.example.org', 't', fetchImpl);

    const { data } = await tracker.issues.listForRepo({
      ...repo,
      state: 'all',
      labels: 'crashlytics:brand-new',
      per_page: 1,
      sort: 'created',
      direction: 'desc',
    });

    expect(data).toEqual([]);
  });

  test('comment, close and reopen go to the issue by number', async () => {
    const { fetchImpl, calls } = fakeForgejo({});
    const tracker = forgejoTracker('https://code.example.org', 't', fetchImpl);

    await tracker.issues.createComment({ ...repo, issue_number: 5, body: 'again' });
    await tracker.issues.update({ ...repo, issue_number: 5, state: 'closed' });

    expect(calls[0]).toMatchObject({ method: 'POST', path: '/api/v1/repos/acme/tracker/issues/5/comments', body: { body: 'again' } });
    expect(calls[1]).toMatchObject({ method: 'PATCH', path: '/api/v1/repos/acme/tracker/issues/5', body: { state: 'closed' } });
  });

  test('an API error carries the status and the server message, never the token', async () => {
    const { fetchImpl } = fakeForgejo({ failWith: { status: 403, body: '{"message":"token does not have required scope"}' } });
    const tracker = forgejoTracker('https://code.example.org', 'secret-token', fetchImpl);

    const failure = tracker.issues.createComment({ ...repo, issue_number: 5, body: 'x' });
    await expect(failure).rejects.toThrowError(/403 token does not have required scope/);
    await expect(failure).rejects.not.toThrowError(/secret-token/);
  });
});
