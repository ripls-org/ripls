'use strict';

/**
 * forge_forgejo.js — the Forgejo side of scripts/forge (#3293).
 *
 * Each command here answers in the shape `gh` answers in, so a caller written
 * against `gh pr view --json …` reads the same fields on both forges. That
 * means translating in both directions:
 *
 *   gh                               Forgejo REST
 *   state OPEN|CLOSED|MERGED         state open|closed, merged true|false
 *   isDraft                          draft (set by a `WIP:` title prefix)
 *   mergeable MERGEABLE|CONFLICTING  mergeable true|false
 *   review state CHANGES_REQUESTED   REQUEST_CHANGES
 *   author.login, createdAt          user.login, created_at
 *
 * Nothing in this file reads the environment or the network directly: the
 * client is built from an explicit {baseUrl, token, fetch}, which is what lets
 * the tests run against recorded responses.
 */

const DRAFT_PREFIX = 'WIP: ';
const DRAFT_PATTERN = /^\s*(\[wip\]|wip:|draft:)\s*/i;

class ForgeError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
}

function createClient({ baseUrl, token, fetch: fetchImpl }) {
  const api = `${baseUrl.replace(/\/+$/, '')}/api/v1`;

  async function request(method, path, { body, query, raw, accept } = {}) {
    const url = new URL(path.startsWith('http') ? path : `${api}${path}`);
    for (const [k, v] of Object.entries(query || {})) {
      if (v !== undefined && v !== null && v !== '') url.searchParams.set(k, String(v));
    }
    const headers = { Accept: accept || 'application/json' };
    if (token) headers.Authorization = `token ${token}`;
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    const res = await fetchImpl(url.toString(), {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await res.text();
    if (!res.ok) {
      let detail = text;
      try {
        detail = JSON.parse(text).message || text;
      } catch {
        // Not JSON; report the body as it came.
      }
      throw new ForgeError(`${method} ${url.pathname}: HTTP ${res.status} ${String(detail).slice(0, 300)}`, res.status);
    }
    if (raw) return text;
    return text ? JSON.parse(text) : null;
  }

  // Forgejo caps a page at 50 items whatever `limit` asks for.
  async function paginate(path, query = {}, max = Infinity) {
    const out = [];
    for (let page = 1; out.length < max; page++) {
      const batch = await request('GET', path, { query: { ...query, limit: 50, page } });
      if (!Array.isArray(batch) || batch.length === 0) break;
      out.push(...batch);
      if (batch.length < 50) break;
    }
    return out.slice(0, max);
  }

  // The bare status of a GET, redirects not followed. For endpoints that
  // answer with a status code alone.
  async function status(path) {
    const headers = { Accept: 'application/json' };
    if (token) headers.Authorization = `token ${token}`;
    const res = await fetchImpl(`${api}${path}`, { method: 'GET', headers, redirect: 'manual' });
    return res.status;
  }

  return { request, paginate, status };
}

function repoPath(repo) {
  if (!/^[^/\s]+\/[^/\s]+$/.test(repo || '')) throw new ForgeError(`not an owner/name repository: "${repo}"`);
  return `/repos/${repo}`;
}

const user = (u) => (u ? { login: u.login } : null);

function comment(c) {
  return { id: c.id, author: user(c.user), body: c.body || '', createdAt: c.created_at, url: c.html_url };
}

const REVIEW_STATES = { REQUEST_CHANGES: 'CHANGES_REQUESTED', COMMENT: 'COMMENTED' };

function review(r) {
  return {
    id: r.id,
    author: user(r.user),
    body: r.body || '',
    state: REVIEW_STATES[r.state] || r.state,
    submittedAt: r.submitted_at,
    commit: { oid: r.commit_id },
  };
}

function pullState(p) {
  if (p.merged) return 'MERGED';
  return p.state === 'open' ? 'OPEN' : 'CLOSED';
}

// Forgejo's `mergeable` means "can be merged right now", which is false for a
// real conflict, while it is still checking, and for any draft. So:
//   true            MERGEABLE
//   false, draft    UNKNOWN: the flag says nothing about conflicts
//   false, ready    CONFLICTING, or still being checked just after a push
// A caller that must know (the conflict dispatcher) compares the branches with
// git instead of trusting this.
function mergeable(p) {
  if (p.state !== 'open') return 'UNKNOWN';
  if (p.mergeable) return 'MERGEABLE';
  return p.draft ? 'UNKNOWN' : 'CONFLICTING';
}

function pull(p) {
  return {
    number: p.number,
    title: p.title,
    body: p.body || '',
    state: pullState(p),
    isDraft: Boolean(p.draft),
    url: p.html_url,
    author: user(p.user),
    headRefName: p.head?.ref,
    headRefOid: p.head?.sha,
    baseRefName: p.base?.ref,
    headRepository: p.head?.repo ? { nameWithOwner: p.head.repo.full_name } : null,
    isCrossRepository: Boolean(p.head?.repo && p.base?.repo && p.head.repo.full_name !== p.base.repo.full_name),
    labels: (p.labels || []).map((l) => ({ name: l.name })),
    assignees: (p.assignees || []).map(user),
    reviewRequests: (p.requested_reviewers || []).map(user),
    mergeable: mergeable(p),
    mergeCommit: p.merge_commit_sha ? { oid: p.merge_commit_sha } : null,
    createdAt: p.created_at,
    updatedAt: p.updated_at,
    mergedAt: p.merged_at,
    closedAt: p.closed_at,
  };
}

function issue(i) {
  return {
    number: i.number,
    title: i.title,
    body: i.body || '',
    state: i.state === 'open' ? 'OPEN' : 'CLOSED',
    url: i.html_url,
    author: user(i.user),
    labels: (i.labels || []).map((l) => ({ name: l.name })),
    assignees: (i.assignees || []).map(user),
    createdAt: i.created_at,
    updatedAt: i.updated_at,
    closedAt: i.closed_at,
  };
}

// Fields that cost an extra request are fetched only when asked for.
async function withExtras(client, repo, number, base, fields, isPull) {
  const want = (f) => !fields || fields.includes(f);
  const r = repoPath(repo);
  if (want('comments')) {
    base.comments = (await client.paginate(`${r}/issues/${number}/comments`)).map(comment);
  }
  if (isPull && (want('reviews') || want('latestReviews'))) {
    const all = (await client.paginate(`${r}/pulls/${number}/reviews`)).map(review);
    base.reviews = all;
    const latest = new Map();
    for (const rv of all) if (rv.author) latest.set(rv.author.login, rv);
    base.latestReviews = [...latest.values()];
  }
  if (isPull && want('statusCheckRollup')) {
    const status = await client.request('GET', `${r}/commits/${base.headRefOid}/status`);
    base.statusCheckRollup = (status?.statuses || []).map((s) => ({
      name: s.context,
      state: String(s.status || '').toUpperCase(),
      targetUrl: s.target_url,
    }));
  }
  if (isPull && want('files')) {
    base.files = (await client.paginate(`${r}/pulls/${number}/files`)).map((f) => ({
      path: f.filename,
      additions: f.additions,
      deletions: f.deletions,
    }));
  }
  return base;
}

function pick(obj, fields) {
  if (!fields) return obj;
  const out = {};
  for (const f of fields) {
    if (!(f in obj)) throw new ForgeError(`unknown JSON field "${f}" (available: ${Object.keys(obj).sort().join(', ')})`);
    out[f] = obj[f];
  }
  return out;
}

async function labelIds(client, repo, names) {
  const all = await client.paginate(`${repoPath(repo)}/labels`);
  return names.map((name) => {
    const found = all.find((l) => l.name === name);
    if (!found) throw new ForgeError(`label "${name}" does not exist in ${repo}`);
    return found.id;
  });
}

// Labels and assignees hang off the issue half of a pull request, so one
// helper serves `pr edit` and `issue edit`.
async function editLabelsAndAssignees(client, repo, { number, addLabels = [], removeLabels = [], addAssignees = [] }) {
  const r = repoPath(repo);
  if (addLabels.length) {
    await client.request('POST', `${r}/issues/${number}/labels`, { body: { labels: await labelIds(client, repo, addLabels) } });
  }
  for (const id of removeLabels.length ? await labelIds(client, repo, removeLabels) : []) {
    await client.request('DELETE', `${r}/issues/${number}/labels/${id}`);
  }
  if (addAssignees.length) {
    const current = await client.request('GET', `${r}/issues/${number}`);
    const logins = new Set([...(current.assignees || []).map((a) => a.login), ...addAssignees]);
    await client.request('PATCH', `${r}/issues/${number}`, { body: { assignees: [...logins] } });
  }
}

const commands = {
  async prView(client, repo, { number, fields }) {
    const p = pull(await client.request('GET', `${repoPath(repo)}/pulls/${number}`));
    return pick(await withExtras(client, repo, number, p, fields, true), fields);
  },

  async prList(client, repo, { state = 'open', head, base, author, label, search, limit = 30, fields }) {
    const query = { state: state === 'merged' ? 'closed' : state };
    // Head, author and search are filtered here: the API takes none of them.
    let pulls = (await client.paginate(`${repoPath(repo)}/pulls`, query, search || head || author ? Infinity : limit)).map(pull);
    if (state === 'merged') pulls = pulls.filter((p) => p.state === 'MERGED');
    if (head) pulls = pulls.filter((p) => p.headRefName === head);
    if (base) pulls = pulls.filter((p) => p.baseRefName === base);
    if (author) pulls = pulls.filter((p) => p.author?.login === author);
    if (label) pulls = pulls.filter((p) => p.labels.some((l) => l.name === label));
    if (search) pulls = pulls.filter((p) => matchesSearch(p, search));
    pulls = pulls.slice(0, limit);
    const out = [];
    for (const p of pulls) {
      const extras = (fields || []).some((f) => ['comments', 'reviews', 'latestReviews', 'statusCheckRollup', 'files'].includes(f));
      out.push(pick(extras ? await withExtras(client, repo, p.number, p, fields, true) : p, fields));
    }
    return out;
  },

  async prCreate(client, repo, { title, body = '', head, base, draft }) {
    const created = await client.request('POST', `${repoPath(repo)}/pulls`, {
      body: { title: draft && !DRAFT_PATTERN.test(title) ? DRAFT_PREFIX + title : title, body, head, base },
    });
    return pull(created);
  },

  async prEdit(client, repo, { number, title, body, addLabels, removeLabels, addAssignees, addReviewers = [] }) {
    const r = repoPath(repo);
    if (title !== undefined || body !== undefined) {
      await client.request('PATCH', `${r}/pulls/${number}`, { body: { title, body } });
    }
    await editLabelsAndAssignees(client, repo, { number, addLabels, removeLabels, addAssignees });
    if (addReviewers.length) {
      await client.request('POST', `${r}/pulls/${number}/requested_reviewers`, { body: { reviewers: addReviewers } });
    }
    return pull(await client.request('GET', `${r}/pulls/${number}`));
  },

  // A draft on Forgejo is a title prefix; "ready" removes it.
  async prReady(client, repo, { number, undo }) {
    const r = repoPath(repo);
    const current = await client.request('GET', `${r}/pulls/${number}`);
    const bare = current.title.replace(DRAFT_PATTERN, '');
    const title = undo ? DRAFT_PREFIX + bare : bare;
    if (title !== current.title) await client.request('PATCH', `${r}/pulls/${number}`, { body: { title } });
    return { number, title, isDraft: Boolean(undo) };
  },

  async prMerge(client, repo, { number, method = 'merge', auto, deleteBranch }) {
    await client.request('POST', `${repoPath(repo)}/pulls/${number}/merge`, {
      body: { Do: method, merge_when_checks_succeed: Boolean(auto), delete_branch_after_merge: Boolean(deleteBranch) },
    });
    return { number, merged: !auto, autoMerge: Boolean(auto) };
  },

  prDiff(client, repo, { number }) {
    return client.request('GET', `${repoPath(repo)}/pulls/${number}.diff`, { raw: true, accept: 'text/plain' });
  },

  async prChecks(client, repo, { number }) {
    const p = await client.request('GET', `${repoPath(repo)}/pulls/${number}`);
    const status = await client.request('GET', `${repoPath(repo)}/commits/${p.head.sha}/status`);
    return (status?.statuses || []).map((s) => ({ name: s.context, state: String(s.status || '').toUpperCase(), link: s.target_url }));
  },

  async comment(client, repo, { number, body }) {
    return comment(await client.request('POST', `${repoPath(repo)}/issues/${number}/comments`, { body: { body } }));
  },

  async issueView(client, repo, { number, fields }) {
    const i = issue(await client.request('GET', `${repoPath(repo)}/issues/${number}`));
    return pick(await withExtras(client, repo, number, i, fields, false), fields);
  },

  async issueList(client, repo, { state = 'open', label, author, search, limit = 30, fields }) {
    const query = { state, type: 'issues', labels: label, created_by: author };
    let issues = (await client.paginate(`${repoPath(repo)}/issues`, query, search ? Infinity : limit)).map(issue);
    if (search) issues = issues.filter((i) => matchesSearch(i, search));
    return issues.slice(0, limit).map((i) => pick(i, fields));
  },

  async issueCreate(client, repo, { title, body = '', labels = [], assignees = [] }) {
    const payload = { title, body, assignees };
    if (labels.length) payload.labels = await labelIds(client, repo, labels);
    return issue(await client.request('POST', `${repoPath(repo)}/issues`, { body: payload }));
  },

  async issueEdit(client, repo, { number, title, body, state, addLabels, removeLabels, addAssignees }) {
    const r = repoPath(repo);
    if (title !== undefined || body !== undefined || state !== undefined) {
      await client.request('PATCH', `${r}/issues/${number}`, { body: { title, body, state } });
    }
    await editLabelsAndAssignees(client, repo, { number, addLabels, removeLabels, addAssignees });
    return issue(await client.request('GET', `${r}/issues/${number}`));
  },

  async labelCreate(client, repo, { name, color = 'ededed', description = '', force }) {
    const payload = { name, color: `#${color.replace(/^#/, '')}`, description };
    const r = repoPath(repo);
    const existing = (await client.paginate(`${r}/labels`)).find((l) => l.name === name);
    if (existing && !force) throw new ForgeError(`label "${name}" already exists in ${repo} (use --force to update it)`);
    if (existing) return client.request('PATCH', `${r}/labels/${existing.id}`, { body: payload });
    return client.request('POST', `${r}/labels`, { body: payload });
  },

  async runView(client, repo, { id, fields }) {
    const r = repoPath(repo);
    const base = run(await client.request('GET', `${r}/actions/runs/${id}`));
    if (!fields || fields.includes('jobs')) {
      base.jobs = (await client.request('GET', `${r}/actions/runs/${id}/jobs`)).map(job);
    }
    return pick(base, fields);
  },

  // What `gh run view --log-failed` gives: the log of every failed job, each
  // line prefixed with the job's name. Forgejo keeps logs per job, not per
  // step, so there is no step column.
  async runLogFailed(client, repo, { id }) {
    const r = repoPath(repo);
    const failed = (await client.request('GET', `${r}/actions/runs/${id}/jobs`)).filter((j) => j.status === 'failure');
    let out = '';
    for (const j of failed) {
      let log;
      try {
        log = await client.request('GET', `${r}/actions/jobs/${j.id}/logs`, { raw: true, accept: 'text/plain' });
      } catch (e) {
        // A job that failed before a runner picked it up has no log.
        log = `(no log: ${e.message})\n`;
      }
      for (const line of log.split('\n')) if (line) out += `${j.name}\t${line}\n`;
    }
    return out;
  },

  async workflowRun(client, repo, { workflow, ref, inputs = {} }) {
    await client.request('POST', `${repoPath(repo)}/actions/workflows/${encodeURIComponent(workflow)}/dispatches`, {
      body: { ref, inputs },
    });
    return { workflow, ref };
  },

  async releaseCreate(client, repo, { tag, title, notes = '', target, draft, prerelease }) {
    const created = await client.request('POST', `${repoPath(repo)}/releases`, {
      body: { tag_name: tag, name: title || tag, body: notes, target_commitish: target, draft: Boolean(draft), prerelease: Boolean(prerelease) },
    });
    return { url: created.html_url, tagName: created.tag_name };
  },

  fileRaw(client, repo, { path, ref }) {
    const encoded = path.split('/').map(encodeURIComponent).join('/');
    return client.request('GET', `${repoPath(repo)}/raw/${encoded}`, { query: { ref }, raw: true, accept: '*/*' });
  },

  // admin | write | read | none, in GitHub's words. Forgejo calls a repo's
  // owner "owner"; that is admin here. Only a repo admin's token may ask
  // about another user, so a 403 is reported, not read as "none".
  async permission(client, repo, { login }) {
    const got = await client.request('GET', `${repoPath(repo)}/collaborators/${encodeURIComponent(login)}/permission`);
    const p = got?.permission;
    if (p === 'owner' || p === 'admin') return 'admin';
    return ['write', 'read'].includes(p) ? p : 'none';
  },
};

// Is `login` a member of the organization? Forgejo answers 204 or 404 to a
// member's token, and redirects a non-member's to the public-members list. A
// redirect is an error here, not "no": the caller is asking a trust question
// with a token that cannot see the answer.
async function orgMember(client, org, login) {
  const code = await client.status(`/orgs/${encodeURIComponent(org)}/members/${encodeURIComponent(login)}`);
  if (code === 204) return true;
  if (code === 404) return false;
  throw new ForgeError(`cannot read membership of ${org} with this token (HTTP ${code}); it must belong to a member`, code);
}

// --- lookups by id, for the agent's request validation ------------------------
//
// GitHub and Forgejo answer these two in the same REST shape, so one function
// each serves both forges (forge.js feeds the GitHub one from `gh api`).

function commentInfo(c) {
  // Forgejo fills pull_request_url for a comment on a pull request and leaves
  // issue_url empty; GitHub always fills issue_url.
  const url = c.pull_request_url || c.issue_url || '';
  const number = Number((url.match(/\/(\d+)$/) || [])[1]);
  return {
    id: c.id,
    body: c.body || '',
    author: user(c.user),
    number: Number.isInteger(number) ? number : null,
    createdAt: c.created_at,
    url: c.html_url,
  };
}

function reviewInfo(r, comments) {
  const out = review(r);
  out.state = String(out.state || '').toUpperCase();
  if (comments) {
    out.comments = comments.map((c) => ({
      id: c.id,
      path: c.path,
      line: c.line ?? c.position ?? c.original_line ?? null,
      body: c.body || '',
      diffHunk: c.diff_hunk || '',
      author: user(c.user),
    }));
  }
  return out;
}

const lookups = {
  // A run as the agent's validation reads it. Forgejo has no workflow name on
  // a run, so `name` is the workflow file; `failedJobs` lets a caller see a
  // failure before the run as a whole has concluded.
  async runInfo(client, repo, { id }) {
    const r = repoPath(repo);
    const base = run(await client.request('GET', `${r}/actions/runs/${id}`));
    const jobs = await client.request('GET', `${r}/actions/runs/${id}/jobs`);
    return {
      ...base,
      name: base.workflowFile,
      failedJobs: (jobs || []).filter((j) => j.status === 'failure').map((j) => j.name),
    };
  },

  async commentInfo(client, repo, { id }) {
    return commentInfo(await client.request('GET', `${repoPath(repo)}/issues/comments/${id}`));
  },

  async reviewInfo(client, repo, { number, id, withComments }) {
    const r = repoPath(repo);
    const raw = await client.request('GET', `${r}/pulls/${number}/reviews/${id}`);
    const comments = withComments ? await client.paginate(`${r}/pulls/${number}/reviews/${id}/comments`) : null;
    return reviewInfo(raw, comments);
  },
};

const RUN_DONE = ['success', 'failure', 'cancelled', 'skipped'];

// gh splits a run's state into status (queued | in_progress | completed) and
// conclusion; Forgejo has one field covering both.
function runState(status) {
  if (RUN_DONE.includes(status)) return { status: 'completed', conclusion: status };
  return { status: status === 'running' ? 'in_progress' : 'queued', conclusion: '' };
}

function run(r) {
  let event = {};
  try {
    event = JSON.parse(r.event_payload || '{}');
  } catch {
    // The payload is advisory here; a run without one still has its own fields.
  }
  const pr = event.pull_request;
  return {
    databaseId: r.id,
    number: r.index_in_repo,
    displayTitle: r.title,
    workflowFile: r.workflow_id,
    event: r.trigger_event || r.event,
    ...runState(r.status),
    headSha: r.commit_sha,
    // For a pull-request run Forgejo's own ref is "#N"; the branch is in the event.
    headBranch: pr?.head?.ref || r.prettyref,
    headRepository: { nameWithOwner: pr?.head?.repo?.full_name || r.repository?.full_name || null },
    pullRequestNumber: pr ? event.number || pr.number : null,
    actor: user(r.trigger_user),
    url: r.html_url,
    createdAt: r.created,
    updatedAt: r.updated,
  };
}

function job(j) {
  return { databaseId: j.id, name: j.name, ...runState(j.status), url: j.html_url };
}

// The subset of GitHub's search syntax the callers use: `in:body` /
// `in:title` scoping and quoted phrases. Anything else is matched as plain
// text against title and body, so an unsupported qualifier finds nothing
// rather than everything.
function matchesSearch(item, search) {
  let scope = ['title', 'body'];
  const terms = [];
  for (const m of search.matchAll(/"([^"]*)"|(\S+)/g)) {
    const term = m[1] !== undefined ? m[1] : m[2];
    if (m[1] === undefined && /^in:(title|body)$/.test(term)) scope = [term.slice(3)];
    else terms.push(term.toLowerCase());
  }
  const hay = scope.map((f) => String(item[f] || '').toLowerCase()).join('\n');
  return terms.every((t) => hay.includes(t));
}

module.exports = { createClient, commands, lookups, commentInfo, reviewInfo, orgMember, ForgeError, matchesSearch, pull, issue, review, comment };
