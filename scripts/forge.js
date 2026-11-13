#!/usr/bin/env node
'use strict';

/**
 * forge — one command for issues and pull requests on either forge (#3293).
 *
 * The project is moving from GitHub to Forgejo, and until the cut-over the
 * same workflows, agent prompts and skills run on both. `gh` only talks to
 * GitHub, so they call this instead. It takes the `gh` forms the repo
 * already uses and answers in `gh`'s JSON shape:
 *
 *   forge pr view 12 --json headRefName,mergeable --jq .mergeable
 *   forge pr list --head claude/fix-1 --state open --json number
 *   forge pr comment 12 --body-file notes.md
 *   forge issue create --title "…" --label claude-fix
 *
 * On GitHub it hands the arguments to `gh` untouched, so nothing about
 * GitHub behaviour changes. On Forgejo it calls the REST API
 * (forge_forgejo.js).
 *
 * Which forge: FORGE_KIND=github|forgejo if set; otherwise the server URL
 * Actions provides (GITHUB_SERVER_URL); otherwise the `origin` remote.
 *
 * On Forgejo it reads, first one set wins:
 *   server   FORGE_URL, FORGEJO_SERVER_URL, GITHUB_SERVER_URL, origin's host
 *   token    FORGE_TOKEN, GH_TOKEN, FORGEJO_TOKEN, GITHUB_TOKEN
 *   repo     -R/--repo, GH_REPO, FORGEJO_REPOSITORY, GITHUB_REPOSITORY, origin
 *
 * Covered: pr view|list|create|edit|ready|merge|diff|checks|comment,
 * issue view|list|create|edit|comment, label create, run view,
 * workflow run, release create. Anything else exits 2 on Forgejo rather than
 * guessing.
 *
 * Two commands are forge's own, for things `gh` only reaches through
 * `gh api` with a GitHub-shaped path. They work on both forges:
 *
 *   forge file <path> [--ref <ref>]   print a file's contents at a ref
 *   forge permission <login>          print admin | write | read | none
 */

const fs = require('fs');
const { spawnSync } = require('child_process');
const { createClient, commands, ForgeError } = require('./forge_forgejo.js');

// flag → [option key, takes a value, repeatable]
const FLAGS = {
  '--repo': ['repo', true], '-R': ['repo', true],
  '--json': ['json', true], '--jq': ['jq', true], '-q': ['jq', true],
  '--state': ['state', true], '-s': ['state', true],
  '--head': ['head', true], '-H': ['head', true],
  '--base': ['base', true], '-B': ['base', true],
  '--author': ['author', true], '-A': ['author', true],
  '--label': ['label', true, true], '-l': ['label', true, true],
  '--search': ['search', true], '-S': ['search', true],
  '--limit': ['limit', true], '-L': ['limit', true],
  '--title': ['title', true], '-t': ['title', true],
  '--body': ['body', true], '-b': ['body', true],
  '--body-file': ['bodyFile', true], '-F': ['bodyFile', true],
  '--assignee': ['assignee', true, true], '-a': ['assignee', true, true],
  '--add-label': ['addLabel', true, true],
  '--remove-label': ['removeLabel', true, true],
  '--add-assignee': ['addAssignee', true, true],
  '--add-reviewer': ['addReviewer', true, true],
  '--color': ['color', true], '-c': ['color', true],
  '--description': ['description', true],
  '--draft': ['draft', false], '--undo': ['undo', false],
  '--auto': ['auto', false], '--merge': ['merge', false],
  '--squash': ['squash', false], '--rebase': ['rebase', false],
  '--delete-branch': ['deleteBranch', false],
  '--force': ['force', false],
  '--ref': ['ref', true], '-r': ['ref', true],
  '--field': ['field', true, true, true], '--raw-field': ['field', true, true, true],
  '--log-failed': ['logFailed', false],
  '--notes': ['notes', true], '-n': ['notes', true],
  '--notes-file': ['notesFile', true],
  '--target': ['target', true],
  '--prerelease': ['prerelease', false], '-p': ['prerelease', false],
};

// gh reuses short flags across commands; these are the ones that differ.
const FLAGS_BY_COMMAND = {
  'workflow run': { '-f': FLAGS['--field'], '-F': FLAGS['--field'] },
  'label create': { '-f': FLAGS['--force'], '-d': FLAGS['--description'] },
  'release create': { '-F': FLAGS['--notes-file'], '-d': FLAGS['--draft'] },
  'pr create': { '-d': FLAGS['--draft'] },
  'pr merge': { '-d': FLAGS['--delete-branch'], '-m': FLAGS['--merge'], '-s': FLAGS['--squash'] },
};

// Commands gh does not have. On GitHub these are answered through `gh api`.
const FORGE_ONLY = new Set(['file', 'permission']);

function parseArgs(argv, command = '') {
  const flags = { ...FLAGS, ...(FLAGS_BY_COMMAND[command] || {}) };
  const positional = [];
  const opts = {};
  for (let i = 0; i < argv.length; i++) {
    let arg = argv[i];
    let inline;
    if (arg.startsWith('--') && arg.includes('=')) {
      inline = arg.slice(arg.indexOf('=') + 1);
      arg = arg.slice(0, arg.indexOf('='));
    }
    const spec = flags[arg];
    if (!spec) {
      if (arg.startsWith('-') && arg !== '-') throw new ForgeError(`unsupported flag ${arg}`);
      positional.push(arg);
      continue;
    }
    const [key, takesValue, repeatable, verbatim] = spec;
    if (!takesValue) {
      opts[key] = true;
      continue;
    }
    const value = inline !== undefined ? inline : argv[++i];
    if (value === undefined) throw new ForgeError(`${arg} needs a value`);
    // gh accepts `--label a,b` as well as repeating the flag. A workflow
    // input is taken whole: its value may contain commas.
    if (verbatim) opts[key] = [...(opts[key] || []), value];
    else if (repeatable) opts[key] = [...(opts[key] || []), ...value.split(',').filter(Boolean)];
    else opts[key] = value;
  }
  return { positional, opts };
}

function originUrl() {
  const res = spawnSync('git', ['remote', 'get-url', 'origin'], { encoding: 'utf8' });
  return res.status === 0 ? res.stdout.trim() : '';
}

// https://host/owner/name(.git), git@host:owner/name(.git), ssh://git@host:port/owner/name
function parseRemote(url) {
  const m = url.match(/^(?:[a-z+]+:\/\/)?(?:[^@/]+@)?([^:/]+)(?::\d+)?[:/](.+?)(?:\.git)?\/?$/);
  return m ? { host: m[1], repo: m[2] } : null;
}

function detect(env, remote) {
  const server = env.FORGE_URL || env.FORGEJO_SERVER_URL || env.GITHUB_SERVER_URL || (remote ? `https://${remote.host}` : '');
  let kind = env.FORGE_KIND;
  if (!kind) kind = !server || /^https?:\/\/github\.com\/?$/.test(server) ? 'github' : 'forgejo';
  if (!['github', 'forgejo'].includes(kind)) throw new ForgeError(`FORGE_KIND must be github or forgejo, not "${kind}"`);
  return {
    kind,
    server,
    token: env.FORGE_TOKEN || env.GH_TOKEN || env.FORGEJO_TOKEN || env.GITHUB_TOKEN || '',
    repo: env.GH_REPO || env.FORGEJO_REPOSITORY || env.GITHUB_REPOSITORY || (remote ? remote.repo : ''),
  };
}

function number(positional, what) {
  // gh also takes a URL or a branch here; the callers only ever pass a number.
  const n = String(positional[0] || '').replace(/^#/, '');
  if (!/^\d+$/.test(n)) throw new ForgeError(`${what} needs a number on Forgejo, got "${positional[0] || ''}"`);
  return Number(n);
}

function bodyOf(opts) {
  if (opts.bodyFile === undefined) return opts.body;
  return fs.readFileSync(opts.bodyFile === '-' ? 0 : opts.bodyFile, 'utf8');
}

const fields = (opts) => (opts.json ? opts.json.split(',').map((f) => f.trim()).filter(Boolean) : null);

async function resolveMe(client, logins = []) {
  if (!logins.includes('@me')) return logins;
  const me = (await client.request('GET', '/user')).login;
  return logins.map((l) => (l === '@me' ? me : l));
}

function inputsOf(pairs = []) {
  const inputs = {};
  for (const pair of pairs) {
    const eq = pair.indexOf('=');
    if (eq < 1) throw new ForgeError(`-f needs key=value, got "${pair}"`);
    inputs[pair.slice(0, eq)] = pair.slice(eq + 1);
  }
  return inputs;
}

// forge's own commands on GitHub. `{owner}/{repo}` is filled in by gh from
// GH_REPO or the checkout when no repository was named.
function githubOwn(group, verb, opts, env, spawn) {
  const repo = opts.repo || env.GH_REPO || env.GITHUB_REPOSITORY || '{owner}/{repo}';
  let args;
  if (group === 'file') {
    if (!verb) throw new ForgeError('file needs a path');
    const ref = opts.ref ? `?ref=${encodeURIComponent(opts.ref)}` : '';
    args = ['api', `repos/${repo}/contents/${verb}${ref}`, '-H', 'Accept: application/vnd.github.raw'];
  } else {
    if (!verb) throw new ForgeError('permission needs a login');
    args = ['api', `repos/${repo}/collaborators/${verb}/permission`, '--jq', '.permission'];
  }
  const res = spawn('gh', args, { stdio: 'inherit' });
  if (res.error) throw new ForgeError(`could not run gh: ${res.error.message}`);
  return res.status === null ? 1 : res.status;
}

function mergeMethod(opts) {
  if (opts.squash) return 'squash';
  if (opts.rebase) return 'rebase';
  return 'merge';
}

// Returns {json} for structured answers, or {text} for what gh prints plainly.
async function run(client, repo, group, verb, positional, opts) {
  const key = `${group} ${verb}`;
  switch (key) {
    case 'pr view':
      return { json: await commands.prView(client, repo, { number: number(positional, key), fields: fields(opts) }) };
    case 'pr list':
      return {
        json: await commands.prList(client, repo, {
          state: opts.state, head: opts.head, base: opts.base, author: opts.author,
          label: (opts.label || [])[0], search: opts.search,
          limit: opts.limit ? Number(opts.limit) : undefined, fields: fields(opts),
        }),
      };
    case 'pr create': {
      if (!opts.title || !opts.head || !opts.base) throw new ForgeError('pr create needs --title, --head and --base on Forgejo');
      const created = await commands.prCreate(client, repo, {
        title: opts.title, body: bodyOf(opts), head: opts.head, base: opts.base, draft: opts.draft,
      });
      if (opts.label || opts.assignee) {
        await commands.prEdit(client, repo, {
          number: created.number, addLabels: opts.label, addAssignees: await resolveMe(client, opts.assignee),
        });
      }
      return { text: created.url };
    }
    case 'pr edit':
      return {
        text: (await commands.prEdit(client, repo, {
          number: number(positional, key), title: opts.title, body: bodyOf(opts),
          addLabels: opts.addLabel, removeLabels: opts.removeLabel,
          addAssignees: await resolveMe(client, opts.addAssignee), addReviewers: opts.addReviewer,
        })).url,
      };
    case 'pr ready':
      await commands.prReady(client, repo, { number: number(positional, key), undo: opts.undo });
      return { text: '' };
    case 'pr merge':
      await commands.prMerge(client, repo, {
        number: number(positional, key), method: mergeMethod(opts), auto: opts.auto, deleteBranch: opts.deleteBranch,
      });
      return { text: '' };
    case 'pr diff':
      return { text: await commands.prDiff(client, repo, { number: number(positional, key) }), exact: true };
    case 'pr checks':
      return { json: await commands.prChecks(client, repo, { number: number(positional, key) }), jsonAlways: true };
    case 'pr comment':
    case 'issue comment': {
      const body = bodyOf(opts);
      if (!body) throw new ForgeError(`${key} needs --body or --body-file`);
      return { text: (await commands.comment(client, repo, { number: number(positional, key), body })).url };
    }
    case 'issue view':
      return { json: await commands.issueView(client, repo, { number: number(positional, key), fields: fields(opts) }) };
    case 'issue list':
      return {
        json: await commands.issueList(client, repo, {
          state: opts.state, label: (opts.label || []).join(','), author: opts.author, search: opts.search,
          limit: opts.limit ? Number(opts.limit) : undefined, fields: fields(opts),
        }),
      };
    case 'issue create': {
      if (!opts.title) throw new ForgeError('issue create needs --title');
      const created = await commands.issueCreate(client, repo, {
        title: opts.title, body: bodyOf(opts), labels: opts.label, assignees: await resolveMe(client, opts.assignee),
      });
      return { text: created.url };
    }
    case 'issue edit':
      return {
        text: (await commands.issueEdit(client, repo, {
          number: number(positional, key), title: opts.title, body: bodyOf(opts),
          addLabels: opts.addLabel, removeLabels: opts.removeLabel,
          addAssignees: await resolveMe(client, opts.addAssignee),
        })).url,
      };
    case 'label create': {
      if (!positional[0]) throw new ForgeError('label create needs a name');
      await commands.labelCreate(client, repo, {
        name: positional[0], color: opts.color, description: opts.description, force: opts.force,
      });
      return { text: '' };
    }
    case 'run view': {
      if (!/^\d+$/.test(positional[0] || '')) throw new ForgeError('run view needs a run id');
      if (opts.logFailed) return { text: await commands.runLogFailed(client, repo, { id: positional[0] }), exact: true };
      return { json: await commands.runView(client, repo, { id: positional[0], fields: fields(opts) }) };
    }
    case 'workflow run': {
      if (!positional[0] || !opts.ref) throw new ForgeError('workflow run needs a workflow file and --ref on Forgejo');
      await commands.workflowRun(client, repo, { workflow: positional[0], ref: opts.ref, inputs: inputsOf(opts.field) });
      return { text: '' };
    }
    case 'release create': {
      if (!positional[0]) throw new ForgeError('release create needs a tag');
      const notes = opts.notesFile !== undefined ? fs.readFileSync(opts.notesFile === '-' ? 0 : opts.notesFile, 'utf8') : opts.notes;
      const created = await commands.releaseCreate(client, repo, {
        tag: positional[0], title: opts.title, notes, target: opts.target, draft: opts.draft, prerelease: opts.prerelease,
      });
      return { text: created.url };
    }
    default: {
      // forge's own commands: the word after the command is its argument.
      if (group === 'file') {
        if (!verb) throw new ForgeError('file needs a path');
        return { text: await commands.fileRaw(client, repo, { path: verb, ref: opts.ref }), exact: true };
      }
      if (group === 'permission') {
        if (!verb) throw new ForgeError('permission needs a login');
        return { text: await commands.permission(client, repo, { login: verb }) };
      }
      const err = new ForgeError(`"forge ${key}" is not supported on Forgejo yet`);
      err.exitCode = 2;
      throw err;
    }
  }
}

// gh's --jq prints strings raw, like `jq -r`. Use the real jq so expressions
// behave identically on both forges.
function applyJq(value, expr) {
  const res = spawnSync('jq', ['-r', expr], { input: JSON.stringify(value), encoding: 'utf8' });
  if (res.error) throw new ForgeError(`--jq needs the jq program: ${res.error.message}`);
  if (res.status !== 0) throw new ForgeError(`jq: ${res.stderr.trim()}`);
  return res.stdout;
}

function render(result, opts) {
  if (result.json !== undefined) {
    if (!opts.json && !result.jsonAlways) {
      throw new ForgeError('this command needs --json <fields> on Forgejo (there is no human-readable view)');
    }
    return opts.jq ? applyJq(result.json, opts.jq) : `${JSON.stringify(result.json)}\n`;
  }
  if (result.exact) return result.text;
  return result.text ? `${result.text}\n` : '';
}

async function main(argv, env = process.env, deps = {}) {
  const remote = deps.remote !== undefined ? deps.remote : parseRemote(originUrl());
  const target = detect(env, remote);
  const spawn = deps.spawn || spawnSync;
  const own = FORGE_ONLY.has(argv[0]);
  if (target.kind === 'github' && !own) {
    const res = spawn('gh', argv, { stdio: 'inherit' });
    if (res.error) throw new ForgeError(`could not run gh: ${res.error.message}`);
    return res.status === null ? 1 : res.status;
  }
  // forge's own commands take their argument where others take a verb.
  const [group, verb = '', ...rest] = argv;
  const { positional, opts } = parseArgs(rest, own ? group : `${group} ${verb}`);
  if (target.kind === 'github') return githubOwn(group, verb, opts, env, spawn);
  const repo = opts.repo || target.repo;
  if (!target.server) throw new ForgeError('no Forgejo server: set FORGE_URL');
  const client = createClient({ baseUrl: target.server, token: target.token, fetch: deps.fetch || fetch });
  const result = await run(client, repo, group, verb, positional, opts);
  (deps.write || ((s) => process.stdout.write(s)))(render(result, opts));
  return 0;
}

if (require.main === module) {
  main(process.argv.slice(2)).then(
    (code) => process.exit(code),
    (err) => {
      process.stderr.write(`forge: ${err.message}\n`);
      process.exit(err.exitCode || 1);
    },
  );
}

module.exports = { main, parseArgs, parseRemote, detect };
