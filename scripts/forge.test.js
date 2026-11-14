'use strict';

const test = require('node:test');
const assert = require('node:assert');

const { main, parseArgs, parseRemote, detect } = require('./forge.js');
const { matchesSearch } = require('./forge_forgejo.js');

const FORGEJO_ENV = { FORGE_KIND: 'forgejo', FORGE_URL: 'https://forge.example', FORGE_TOKEN: 't0ken', GH_REPO: 'org/app' };

// Shapes below are trimmed copies of real Forgejo 17 responses.
function forgejoPull(overrides = {}) {
  return {
    number: 3,
    title: 'probe: PR by bot',
    body: 'probe body',
    state: 'open',
    merged: false,
    draft: false,
    mergeable: true,
    html_url: 'https://forge.example/org/app/pulls/3',
    user: { login: 'ripls-agent' },
    head: { ref: 'claude/probe-pr', sha: 'abc123', repo: { full_name: 'org/app' } },
    base: { ref: 'main', repo: { full_name: 'org/app' } },
    labels: [{ id: 7, name: 'claude-plan-pr' }],
    assignees: [],
    requested_reviewers: [],
    merge_commit_sha: null,
    created_at: '2026-11-13T12:00:00Z',
    ...overrides,
  };
}

// A fake Forgejo: `routes` maps "METHOD /path" to a body or a function of the
// request. Every call is recorded so tests can assert what was sent.
function fakeForge(routes) {
  const calls = [];
  const fetch = async (url, init) => {
    const u = new URL(url);
    const path = u.pathname.replace('/api/v1', '');
    const call = { method: init.method, path, query: Object.fromEntries(u.searchParams), body: init.body ? JSON.parse(init.body) : undefined, headers: init.headers };
    calls.push(call);
    const route = routes[`${init.method} ${path}`];
    if (route === undefined) return { ok: false, status: 404, text: async () => JSON.stringify({ message: 'not found' }) };
    const value = typeof route === 'function' ? route(call) : route;
    return { ok: true, status: 200, text: async () => (typeof value === 'string' ? value : JSON.stringify(value)) };
  };
  return { fetch, calls };
}

async function forge(args, routes, env = FORGEJO_ENV) {
  const { fetch, calls } = fakeForge(routes);
  let out = '';
  const code = await main(args, env, { fetch, remote: null, write: (s) => (out += s) });
  return { code, out, calls };
}

test('pr view answers in gh field names and values', async () => {
  const { out, calls } = await forge(
    ['pr', 'view', '3', '--json', 'number,state,isDraft,headRefName,headRefOid,baseRefName,isCrossRepository,mergeable,author,labels'],
    { 'GET /repos/org/app/pulls/3': forgejoPull() },
  );
  assert.deepStrictEqual(JSON.parse(out), {
    number: 3,
    state: 'OPEN',
    isDraft: false,
    headRefName: 'claude/probe-pr',
    headRefOid: 'abc123',
    baseRefName: 'main',
    isCrossRepository: false,
    mergeable: 'MERGEABLE',
    author: { login: 'ripls-agent' },
    labels: [{ name: 'claude-plan-pr' }],
  });
  assert.strictEqual(calls.length, 1, 'no extra requests for fields that were not asked for');
  assert.strictEqual(calls[0].headers.Authorization, 'token t0ken');
});

test('pr view maps merged, conflicting and cross-repository pulls', async () => {
  const view = async (pull) =>
    JSON.parse((await forge(['pr', 'view', '3', '--json', 'state,mergeable,isCrossRepository'], { 'GET /repos/org/app/pulls/3': pull })).out);
  assert.deepStrictEqual(await view(forgejoPull({ state: 'closed', merged: true })), {
    state: 'MERGED', mergeable: 'UNKNOWN', isCrossRepository: false,
  });
  assert.deepStrictEqual(await view(forgejoPull({ mergeable: false })), {
    state: 'OPEN', mergeable: 'CONFLICTING', isCrossRepository: false,
  });
  // A draft is never "mergeable" on Forgejo, conflict or not.
  assert.strictEqual((await view(forgejoPull({ mergeable: false, draft: true }))).mergeable, 'UNKNOWN');
  assert.strictEqual((await view(forgejoPull({ mergeable: true, draft: true }))).mergeable, 'MERGEABLE');
  assert.strictEqual(
    (await view(forgejoPull({ head: { ref: 'x', sha: 'y', repo: { full_name: 'fork/app' } } }))).isCrossRepository,
    true,
  );
});

test('pr view fetches comments and reviews only on request, with gh review states', async () => {
  const { out } = await forge(['pr', 'view', '3', '--json', 'comments,reviews'], {
    'GET /repos/org/app/pulls/3': forgejoPull(),
    'GET /repos/org/app/issues/3/comments': [{ id: 1, body: 'hi', user: { login: 'bryan' }, created_at: '2026-11-13T12:01:00Z' }],
    'GET /repos/org/app/pulls/3/reviews': [
      { id: 1, state: 'APPROVED', body: 'lgtm', user: { login: 'bryan' }, commit_id: 'abc123' },
      { id: 2, state: 'REQUEST_CHANGES', body: 'fix', user: { login: 'bryan' }, commit_id: 'abc123' },
      { id: 3, state: 'COMMENT', body: 'note', user: { login: 'ian' }, commit_id: 'abc123' },
    ],
  });
  const got = JSON.parse(out);
  assert.deepStrictEqual(got.comments.map((c) => [c.author.login, c.body, c.createdAt]), [['bryan', 'hi', '2026-11-13T12:01:00Z']]);
  assert.deepStrictEqual(got.reviews.map((r) => r.state), ['APPROVED', 'CHANGES_REQUESTED', 'COMMENTED']);
});

test('--jq prints raw strings the way gh does', async () => {
  const { out } = await forge(['pr', 'view', '3', '--json', 'headRefName', '--jq', '.headRefName'], {
    'GET /repos/org/app/pulls/3': forgejoPull(),
  });
  assert.strictEqual(out, 'claude/probe-pr\n');
});

test('pr list filters by head here, because the API cannot', async () => {
  const { out, calls } = await forge(['pr', 'list', '--head', 'claude/probe-pr', '--state', 'open', '--json', 'number'], {
    'GET /repos/org/app/pulls': [forgejoPull(), forgejoPull({ number: 4, head: { ref: 'other', sha: 'd', repo: { full_name: 'org/app' } } })],
  });
  assert.deepStrictEqual(JSON.parse(out), [{ number: 3 }]);
  assert.strictEqual(calls[0].query.state, 'open');
});

test('pr list pages past 50 results', async () => {
  const page = (start, n) => Array.from({ length: n }, (_, i) => forgejoPull({ number: start + i }));
  const { out, calls } = await forge(['pr', 'list', '--state', 'all', '--limit', '60', '--json', 'number'], {
    'GET /repos/org/app/pulls': (call) => (call.query.page === '1' ? page(1, 50) : page(51, 20)),
  });
  assert.strictEqual(JSON.parse(out).length, 60);
  assert.deepStrictEqual(calls.map((c) => c.query.page), ['1', '2']);
});

test('pr list --search finds a marker in the body', async () => {
  const { out } = await forge(['pr', 'list', '--state', 'all', '--search', 'in:body "<!-- claude-plan-pr: 12 -->"', '--json', 'number'], {
    'GET /repos/org/app/pulls': [
      forgejoPull({ number: 5, body: 'x\n<!-- claude-plan-pr: 12 -->' }),
      forgejoPull({ number: 6, body: '<!-- claude-plan-pr: 120 -->', title: '<!-- claude-plan-pr: 12 -->' }),
    ],
  });
  assert.deepStrictEqual(JSON.parse(out), [{ number: 5 }]);
});

test('matchesSearch treats an unknown qualifier as text, so it finds nothing', () => {
  assert.strictEqual(matchesSearch({ title: 'a', body: 'b' }, 'merged:>2026-01-01'), false);
  assert.strictEqual(matchesSearch({ title: 'Fix login', body: '' }, 'in:title login'), true);
  assert.strictEqual(matchesSearch({ title: '', body: 'Fix login' }, 'in:title login'), false);
});

test('pr create marks a draft with the WIP prefix and prints the URL', async () => {
  const { out, calls } = await forge(['pr', 'create', '--title', 'Plan', '--body', 'b', '--head', 'claude/x', '--base', 'main', '--draft'], {
    'POST /repos/org/app/pulls': (call) => forgejoPull({ title: call.body.title, draft: true }),
  });
  assert.strictEqual(calls[0].body.title, 'WIP: Plan');
  assert.deepStrictEqual([calls[0].body.head, calls[0].body.base], ['claude/x', 'main']);
  assert.strictEqual(out, 'https://forge.example/org/app/pulls/3\n');
});

test('pr ready strips the draft prefix, and --undo adds it', async () => {
  const ready = await forge(['pr', 'ready', '3'], {
    'GET /repos/org/app/pulls/3': forgejoPull({ title: 'WIP: Plan' }),
    'PATCH /repos/org/app/pulls/3': forgejoPull(),
  });
  assert.deepStrictEqual(ready.calls.at(-1).body, { title: 'Plan' });
  const undo = await forge(['pr', 'ready', '3', '--undo'], {
    'GET /repos/org/app/pulls/3': forgejoPull({ title: 'Plan' }),
    'PATCH /repos/org/app/pulls/3': forgejoPull(),
  });
  assert.deepStrictEqual(undo.calls.at(-1).body, { title: 'WIP: Plan' });
  const already = await forge(['pr', 'ready', '3'], { 'GET /repos/org/app/pulls/3': forgejoPull({ title: 'Plan' }) });
  assert.strictEqual(already.calls.length, 1, 'no write when the title is already right');
});

test('pr edit turns label names into ids and merges assignees', async () => {
  const { calls } = await forge(['pr', 'edit', '3', '--add-label', 'a,b', '--remove-label', 'c', '--add-assignee', '@me', '--add-reviewer', 'ian'], {
    'GET /user': { login: 'bryan' },
    'GET /repos/org/app/labels': [{ id: 1, name: 'a' }, { id: 2, name: 'b' }, { id: 3, name: 'c' }],
    'POST /repos/org/app/issues/3/labels': [],
    'DELETE /repos/org/app/issues/3/labels/3': '',
    'GET /repos/org/app/issues/3': { number: 3, assignees: [{ login: 'ian' }] },
    'PATCH /repos/org/app/issues/3': {},
    'POST /repos/org/app/pulls/3/requested_reviewers': [],
    'GET /repos/org/app/pulls/3': forgejoPull(),
  });
  const sent = (method, path) => calls.find((c) => c.method === method && c.path === path)?.body;
  assert.deepStrictEqual(sent('POST', '/repos/org/app/issues/3/labels'), { labels: [1, 2] });
  assert.ok(calls.some((c) => c.method === 'DELETE' && c.path === '/repos/org/app/issues/3/labels/3'));
  assert.deepStrictEqual(sent('PATCH', '/repos/org/app/issues/3'), { assignees: ['ian', 'bryan'] });
  assert.deepStrictEqual(sent('POST', '/repos/org/app/pulls/3/requested_reviewers'), { reviewers: ['ian'] });
});

test('pr edit fails on a label that does not exist', async () => {
  await assert.rejects(
    forge(['pr', 'edit', '3', '--add-label', 'missing'], { 'GET /repos/org/app/labels': [] }),
    /label "missing" does not exist/,
  );
});

test('pr merge --auto asks Forgejo to merge when checks succeed', async () => {
  const { calls } = await forge(['pr', 'merge', '3', '--auto', '--merge'], { 'POST /repos/org/app/pulls/3/merge': '' });
  assert.deepStrictEqual(calls[0].body, { Do: 'merge', merge_when_checks_succeed: true, delete_branch_after_merge: false });
});

test('pr diff returns the diff untouched', async () => {
  const diff = 'diff --git a/x b/x\n+line\n';
  const { out } = await forge(['pr', 'diff', '3'], { 'GET /repos/org/app/pulls/3.diff': diff });
  assert.strictEqual(out, diff);
});

test('comment posts to the issue endpoint for PRs and issues alike', async () => {
  for (const group of ['pr', 'issue']) {
    const { out, calls } = await forge([group, 'comment', '3', '--body', 'hello'], {
      'POST /repos/org/app/issues/3/comments': { id: 9, body: 'hello', user: { login: 'bryan' }, html_url: 'https://forge.example/c/9' },
    });
    assert.deepStrictEqual(calls[0].body, { body: 'hello' });
    assert.strictEqual(out, 'https://forge.example/c/9\n');
  }
});

test('issue create resolves labels and prints the URL', async () => {
  const { out, calls } = await forge(['issue', 'create', '--title', 'T', '--body', 'B', '--label', 'claude-fix'], {
    'GET /repos/org/app/labels': [{ id: 4, name: 'claude-fix' }],
    'POST /repos/org/app/issues': { number: 8, title: 'T', state: 'open', html_url: 'https://forge.example/org/app/issues/8' },
  });
  assert.deepStrictEqual(calls.at(-1).body, { title: 'T', body: 'B', assignees: [], labels: [4] });
  assert.strictEqual(out, 'https://forge.example/org/app/issues/8\n');
});

test('issue list asks for issues only and filters by label on the server', async () => {
  const { out, calls } = await forge(['issue', 'list', '--label', 'bug', '--state', 'open', '--json', 'number,state'], {
    'GET /repos/org/app/issues': [{ number: 2, title: 't', state: 'open' }],
  });
  assert.deepStrictEqual(JSON.parse(out), [{ number: 2, state: 'OPEN' }]);
  assert.deepStrictEqual([calls[0].query.type, calls[0].query.labels], ['issues', 'bug']);
});

test('label create refuses a duplicate without --force and updates with it', async () => {
  const routes = {
    'GET /repos/org/app/labels': [{ id: 5, name: 'stage' }],
    'PATCH /repos/org/app/labels/5': {},
  };
  await assert.rejects(forge(['label', 'create', 'stage', '--color', '00aa00'], routes), /already exists/);
  const { calls } = await forge(['label', 'create', 'stage', '--color', '00aa00', '--description', 'd', '--force'], routes);
  assert.deepStrictEqual(calls.at(-1).body, { name: 'stage', color: '#00aa00', description: 'd' });
});

test('an unknown JSON field is an error that lists the valid ones', async () => {
  await assert.rejects(
    forge(['pr', 'view', '3', '--json', 'nope'], { 'GET /repos/org/app/pulls/3': forgejoPull() }),
    /unknown JSON field "nope" \(available: .*headRefName/,
  );
});

test('an unsupported command exits 2 instead of guessing', async () => {
  await assert.rejects(forge(['repo', 'view'], {}), (err) => err.exitCode === 2 && /not supported on Forgejo yet/.test(err.message));
});

test('a server error carries the status and message', async () => {
  await assert.rejects(forge(['pr', 'view', '99', '--json', 'number'], {}), /GET \/api\/v1\/repos\/org\/app\/pulls\/99: HTTP 404 not found/);
});

test('issue view gives the milestone in gh\'s shape, or null', async () => {
  const base = { number: 7, title: 't', state: 'open', user: { login: 'a' } };
  const withOne = await forge(['issue', 'view', '7', '--json', 'number,milestone'], {
    'GET /repos/org/app/issues/7': { ...base, milestone: { id: 2, title: 'v1', description: 'first', due_on: '2026-12-01T00:00:00Z' } },
  });
  assert.deepStrictEqual(JSON.parse(withOne.out).milestone, { number: 2, title: 'v1', description: 'first', dueOn: '2026-12-01T00:00:00Z' });
  const without = await forge(['issue', 'view', '7', '--json', 'number,milestone'], { 'GET /repos/org/app/issues/7': base });
  assert.strictEqual(JSON.parse(without.out).milestone, null);
});

test('a pull request reference must be a number on Forgejo', async () => {
  await assert.rejects(forge(['pr', 'view', 'my-branch', '--json', 'number'], {}), /needs a number on Forgejo/);
});

test('run view splits Forgejo status into status and conclusion, and finds a PR run\'s branch', async () => {
  const payload = JSON.stringify({ number: 3, pull_request: { number: 3, head: { ref: 'claude/x', repo: { full_name: 'org/app' } } } });
  const { out } = await forge(['run', 'view', '6', '--json', 'databaseId,number,event,status,conclusion,headBranch,headSha,pullRequestNumber,workflowFile,name,jobs'], {
    'GET /repos/org/app/actions/runs/6': {
      id: 6, index_in_repo: 4, title: 't', workflow_id: 'probe.yaml', trigger_event: 'pull_request', status: 'failure',
      prettyref: '#3', commit_sha: 'abc', event_payload: payload, trigger_user: { login: 'ripls-agent' },
    },
    'GET /repos/org/app/actions/runs/6/jobs': [
      { id: 10, name: 'build', status: 'success' },
      { id: 11, name: 'test', status: 'running' },
      { id: 12, name: 'deploy', status: 'waiting' },
    ],
  });
  const got = JSON.parse(out);
  assert.deepStrictEqual(
    [got.databaseId, got.number, got.event, got.status, got.conclusion, got.headBranch, got.headSha, got.pullRequestNumber, got.workflowFile, got.name],
    [6, 4, 'pull_request', 'completed', 'failure', 'claude/x', 'abc', 3, 'probe.yaml', 'probe.yaml'],
  );
  assert.deepStrictEqual(got.jobs.map((j) => [j.name, j.status, j.conclusion]), [
    ['build', 'completed', 'success'],
    ['test', 'in_progress', ''],
    ['deploy', 'queued', ''],
  ]);
});

test('run view --log-failed prints only failed jobs, prefixed with the job name', async () => {
  const { out } = await forge(['run', 'view', '6', '--log-failed'], {
    'GET /repos/org/app/actions/runs/6/jobs': [
      { id: 10, name: 'build', status: 'success' },
      { id: 11, name: 'test', status: 'failure' },
      { id: 12, name: 'never-started', status: 'failure' },
    ],
    'GET /repos/org/app/actions/jobs/11/logs': 'line one\n\nline two\n',
  });
  const lines = out.trimEnd().split('\n');
  assert.deepStrictEqual(lines.slice(0, 2), ['test\tline one', 'test\tline two']);
  assert.match(lines[2], /^never-started\t\(no log: .*HTTP 404/);
  assert.strictEqual(lines.length, 3);
});

test('workflow run sends inputs whole, commas and equals signs included', async () => {
  const { calls } = await forge(['workflow', 'run', 'agent.yaml', '-R', 'org/agent', '--ref', 'main', '-f', 'mode=plan', '-f', 'instruction=a, b=c'], {
    'POST /repos/org/agent/actions/workflows/agent.yaml/dispatches': '',
  });
  assert.deepStrictEqual(calls[0].body, { ref: 'main', inputs: { mode: 'plan', instruction: 'a, b=c' } });
});

test('release create posts the tag, title and notes', async () => {
  const { out, calls } = await forge(['release', 'create', 'v1.2.3', '--title', 'One', '--notes', 'N', '--target', 'main'], {
    'POST /repos/org/app/releases': { html_url: 'https://forge.example/org/app/releases/tag/v1.2.3', tag_name: 'v1.2.3' },
  });
  assert.deepStrictEqual(calls[0].body, { tag_name: 'v1.2.3', name: 'One', body: 'N', target_commitish: 'main', draft: false, prerelease: false });
  assert.strictEqual(out, 'https://forge.example/org/app/releases/tag/v1.2.3\n');
});

test('file prints the content at a ref, byte for byte', async () => {
  const { out, calls } = await forge(['file', 'scripts/a b.js', '--ref', 'main'], {
    'GET /repos/org/app/raw/scripts/a%20b.js': 'no trailing newline',
  });
  assert.strictEqual(out, 'no trailing newline');
  assert.strictEqual(calls[0].query.ref, 'main');
});

test('permission answers in GitHub\'s words and does not hide a refusal', async () => {
  const ask = async (permission) =>
    (await forge(['permission', 'someone'], { 'GET /repos/org/app/collaborators/someone/permission': { permission } })).out.trim();
  assert.deepStrictEqual(
    [await ask('owner'), await ask('admin'), await ask('write'), await ask('read'), await ask('none'), await ask(undefined)],
    ['admin', 'admin', 'write', 'read', 'none', 'none'],
  );
  await assert.rejects(forge(['permission', 'someone'], {}), /HTTP 404/);
});

test('member answers true or false, and refuses to guess when the token cannot see', async () => {
  const ask = async (status) => {
    let out = '';
    const fetch = async (url, init) => {
      assert.match(url, /\/api\/v1\/orgs\/org\/members\/someone$/);
      assert.strictEqual(init.redirect, 'manual');
      return { status };
    };
    await main(['member', 'someone'], FORGEJO_ENV, { fetch, remote: null, write: (s) => (out += s) });
    return out.trim();
  };
  assert.strictEqual(await ask(204), 'true');
  assert.strictEqual(await ask(404), 'false');
  await assert.rejects(ask(303), /cannot read membership of org with this token \(HTTP 303\)/);
});

test('member takes the organization from --org when given', async () => {
  let asked = '';
  const fetch = async (url) => {
    asked = url;
    return { status: 204 };
  };
  await main(['member', 'someone', '--org', 'other'], FORGEJO_ENV, { fetch, remote: null, write: () => {} });
  assert.match(asked, /\/orgs\/other\/members\/someone$/);
});

test('on GitHub, forge\'s own commands go through gh api', async () => {
  const seen = [];
  const spawn = (cmd, args) => {
    seen.push(args);
    return { status: 0 };
  };
  const env = { GITHUB_SERVER_URL: 'https://github.com', GITHUB_REPOSITORY: 'o/r' };
  await main(['file', 'renovate.json', '--ref', 'main'], env, { spawn, remote: null });
  await main(['permission', 'octocat'], env, { spawn, remote: null });
  assert.deepStrictEqual(seen, [
    ['api', 'repos/o/r/contents/renovate.json?ref=main', '-H', 'Accept: application/vnd.github.raw'],
    ['api', 'repos/o/r/collaborators/octocat/permission', '--jq', '.permission'],
  ]);
});

test('run-info reports a failed job before the run has concluded', async () => {
  const payload = JSON.stringify({ number: 7, pull_request: { number: 7, head: { ref: 'claude/x', repo: { full_name: 'org/app' } } } });
  const { out } = await forge(['run-info', '196'], {
    'GET /repos/org/app/actions/runs/196': {
      id: 196, index_in_repo: 9, title: 'Fix it', workflow_id: 'test_go.yaml', trigger_event: 'pull_request', status: 'running',
      commit_sha: 'abc', prettyref: '#7', event_payload: payload, trigger_user: { login: 'ripls-agent' }, created: '2026-11-13T14:39:40-06:00',
    },
    'GET /repos/org/app/actions/runs/196/jobs': [{ id: 1, name: 'test', status: 'failure' }, { id: 2, name: 'fixer', status: 'running' }],
  });
  const got = JSON.parse(out);
  assert.deepStrictEqual(
    [got.workflowFile, got.name, got.event, got.status, got.conclusion, got.headBranch, got.headRepository.nameWithOwner, got.actor.login, got.failedJobs],
    ['test_go.yaml', 'test_go.yaml', 'pull_request', 'in_progress', '', 'claude/x', 'org/app', 'ripls-agent', ['test']],
  );
});

test('comment-info finds the number for a comment on an issue or on a pull request', async () => {
  const on = async (extra) =>
    JSON.parse((await forge(['comment-info', '37'], {
      'GET /repos/org/app/issues/comments/37': { id: 37, body: '@claude hi', user: { login: 'bryan' }, ...extra },
    })).out);
  // Forgejo: a PR comment has pull_request_url and an empty issue_url.
  assert.deepStrictEqual(await on({ issue_url: '', pull_request_url: 'https://forge.example/org/app/pulls/4' }), {
    id: 37, body: '@claude hi', author: { login: 'bryan' }, number: 4,
  });
  assert.strictEqual((await on({ issue_url: 'https://forge.example/org/app/issues/12' })).number, 12);
  assert.strictEqual((await on({})).number, null);
});

test('review-info gives the gh state name, and inline comments on request', async () => {
  const routes = {
    'GET /repos/org/app/pulls/4/reviews/8': { id: 8, state: 'REQUEST_CHANGES', body: 'rename', user: { login: 'bryan' }, commit_id: 'abc' },
    'GET /repos/org/app/pulls/4/reviews/8/comments': [{ id: 21, path: 'src/a.txt', position: 2, body: 'here', diff_hunk: '@@', user: { login: 'bryan' } }],
  };
  const plain = JSON.parse((await forge(['review-info', '4', '8'], routes)).out);
  assert.deepStrictEqual([plain.state, plain.body, plain.author.login, 'comments' in plain], ['CHANGES_REQUESTED', 'rename', 'bryan', false]);
  const full = JSON.parse((await forge(['review-info', '4', '8', '--comments'], routes)).out);
  assert.deepStrictEqual(full.comments, [{ id: 21, path: 'src/a.txt', line: 2, body: 'here', diffHunk: '@@', author: { login: 'bryan' } }]);
  await assert.rejects(forge(['review-info', '4'], routes), /needs a numeric id/);
});

test('on GitHub the lookups read gh api and answer in the same shape', async () => {
  const answers = {
    'repos/o/r/actions/runs/5': {
      id: 5, run_number: 2, display_title: 'agent-dispatch plan 9', path: '.github/workflows/claude_plan_issue.yaml', event: 'issues',
      status: 'completed', conclusion: 'success', head_sha: 'abc', head_branch: 'main', head_repository: { full_name: 'o/r' },
      triggering_actor: { login: 'octocat' }, created_at: '2026-11-13T00:00:00Z', name: 'Claude Plan Issue',
    },
    'repos/o/r/issues/comments/7': { id: 7, body: 'hi', user: { login: 'octocat' }, issue_url: 'https://api.github.com/repos/o/r/issues/9' },
    'repos/o/r/pulls/9/reviews/3': { id: 3, state: 'COMMENTED', body: 'note', user: { login: 'octocat' } },
  };
  const spawn = (cmd, args) => {
    assert.deepStrictEqual([cmd, args[0]], ['gh', 'api']);
    return { status: 0, stdout: JSON.stringify(answers[args[1]]) };
  };
  const ask = async (argv) => {
    let out = '';
    await main(argv, { GITHUB_SERVER_URL: 'https://github.com', GITHUB_REPOSITORY: 'o/r' }, { spawn, remote: null, write: (s) => (out += s) });
    return JSON.parse(out);
  };
  const run = await ask(['run-info', '5']);
  assert.deepStrictEqual(
    [run.workflowFile, run.event, run.displayTitle, run.conclusion, run.actor.login, run.headRepository.nameWithOwner, run.failedJobs],
    ['claude_plan_issue.yaml', 'issues', 'agent-dispatch plan 9', 'success', 'octocat', 'o/r', []],
  );
  assert.deepStrictEqual([(await ask(['comment-info', '7'])).number, (await ask(['review-info', '9', '3'])).state], [9, 'COMMENTED']);
});

test('short flags mean what gh means by them in each command', () => {
  assert.deepStrictEqual(parseArgs(['-f', 'a=1'], 'workflow run').opts, { field: ['a=1'] });
  assert.deepStrictEqual(parseArgs(['-f'], 'label create').opts, { force: true });
  assert.deepStrictEqual(parseArgs(['-F', 'notes.md'], 'release create').opts, { notesFile: 'notes.md' });
  assert.deepStrictEqual(parseArgs(['-F', 'body.md'], 'pr comment').opts, { bodyFile: 'body.md' });
});

test('on GitHub every argument goes to gh untouched', async () => {
  const seen = [];
  const spawn = (cmd, args) => {
    seen.push([cmd, args]);
    return { status: 3 };
  };
  const args = ['pr', 'view', '12', '--json', 'title', '--some-flag-forge-does-not-know'];
  const code = await main(args, { GITHUB_SERVER_URL: 'https://github.com' }, { spawn, remote: null });
  assert.deepStrictEqual(seen, [['gh', args]]);
  assert.strictEqual(code, 3, 'gh exit status is passed through');
});

test('detect picks the forge from the environment, then the remote', () => {
  assert.strictEqual(detect({ GITHUB_SERVER_URL: 'https://github.com' }, null).kind, 'github');
  assert.strictEqual(detect({ GITHUB_SERVER_URL: 'https://code.example' }, null).kind, 'forgejo');
  assert.strictEqual(detect({ GITHUB_SERVER_URL: 'https://code.example', FORGE_KIND: 'github' }, null).kind, 'github');
  assert.strictEqual(detect({}, { host: 'github.com', repo: 'o/r' }).kind, 'github');
  const local = detect({}, { host: 'code.example', repo: 'o/r' });
  assert.deepStrictEqual([local.kind, local.server, local.repo], ['forgejo', 'https://code.example', 'o/r']);
  assert.strictEqual(detect({}, null).kind, 'github', 'nothing to go on: behave like gh');
  assert.throws(() => detect({ FORGE_KIND: 'gitlab' }, null), /FORGE_KIND must be/);
});

test('detect prefers the explicit token and repo', () => {
  const t = detect({ FORGE_KIND: 'forgejo', FORGE_TOKEN: 'a', GH_TOKEN: 'b', GITHUB_TOKEN: 'c', GH_REPO: 'x/y', GITHUB_REPOSITORY: 'p/q' }, null);
  assert.deepStrictEqual([t.token, t.repo], ['a', 'x/y']);
});

test('parseRemote reads https, scp-style and ssh remotes', () => {
  assert.deepStrictEqual(parseRemote('https://github.com/o/r.git'), { host: 'github.com', repo: 'o/r' });
  assert.deepStrictEqual(parseRemote('git@code.example:o/r.git'), { host: 'code.example', repo: 'o/r' });
  assert.deepStrictEqual(parseRemote('ssh://git@code.example:222/o/r'), { host: 'code.example', repo: 'o/r' });
  assert.strictEqual(parseRemote(''), null);
});

test('parseArgs handles short flags, =values, repeats and comma lists', () => {
  const { positional, opts } = parseArgs(['12', '-R', 'o/r', '--json=a,b', '--add-label', 'x,y', '--add-label', 'z', '--draft']);
  assert.deepStrictEqual(positional, ['12']);
  assert.deepStrictEqual(opts, { repo: 'o/r', json: 'a,b', addLabel: ['x', 'y', 'z'], draft: true });
  assert.throws(() => parseArgs(['--nope']), /unsupported flag --nope/);
  assert.throws(() => parseArgs(['--title']), /--title needs a value/);
});
