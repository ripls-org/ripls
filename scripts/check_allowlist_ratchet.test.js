'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execFileSync, spawnSync } = require('child_process');

const SCRIPT = path.join(__dirname, 'check_allowlist_ratchet.sh');
const ALLOWLIST = 'scripts/allow.txt';

function git(cwd, ...args) {
  return execFileSync('git', args, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
}

// A bare "origin" whose main carries `baseContent` (or no allowlist at all when
// it is null), and a clone of it whose working tree carries `headContent`.
function makeRepos(baseContent, headContent) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'ratchet-'));
  const origin = path.join(root, 'origin.git');
  const work = path.join(root, 'work');
  git(root, 'init', '-q', '--bare', '-b', 'main', origin);
  git(root, 'clone', '-q', origin, work);
  git(work, 'config', 'user.email', 'test@example.com');
  git(work, 'config', 'user.name', 'Test');
  fs.mkdirSync(path.join(work, 'scripts'));
  fs.writeFileSync(path.join(work, 'README.md'), 'x\n');
  if (baseContent !== null) fs.writeFileSync(path.join(work, ALLOWLIST), baseContent);
  git(work, 'add', '-A');
  git(work, 'commit', '-q', '-m', 'base');
  git(work, 'push', '-q', 'origin', 'HEAD:main');
  fs.writeFileSync(path.join(work, ALLOWLIST), headContent);
  return { root, work };
}

// Run under the strictest shell a CI runner uses, so a pipeline that only
// works without pipefail fails here rather than on the runner.
function run(work, baseRef = 'main') {
  const result = spawnSync(
    'bash',
    ['--noprofile', '--norc', '-e', '-o', 'pipefail', SCRIPT, ALLOWLIST, baseRef, 'Color', 'Fix it instead.'],
    { cwd: work, encoding: 'utf8' },
  );
  return { status: result.status, out: result.stdout + result.stderr };
}

test('passes when both allowlists are empty', (t) => {
  const { root, work } = makeRepos('# only a comment\n\n', '# only a comment\n\n');
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const { status, out } = run(work);
  assert.strictEqual(status, 0, out);
  assert.match(out, /base=0 head=0/);
});

test('passes when the allowlist shrinks', (t) => {
  const { root, work } = makeRepos('# c\nlib/a.dart\nlib/b.dart\n', '# c\nlib/a.dart\n');
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const { status, out } = run(work);
  assert.strictEqual(status, 0, out);
  assert.match(out, /base=2 head=1/);
});

test('fails when the allowlist grows', (t) => {
  const { root, work } = makeRepos('lib/a.dart\n', 'lib/a.dart\n\n# why\nlib/b.dart\n');
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const { status, out } = run(work);
  assert.strictEqual(status, 1, out);
  assert.match(out, /::error::Color allowlist grew from 1 to 2 entries\. Fix it instead\./);
});

test('treats a base without the file as zero entries', (t) => {
  const { root, work } = makeRepos(null, 'lib/a.dart\n');
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const { status, out } = run(work);
  assert.strictEqual(status, 1, out);
  assert.match(out, /base=0 head=1/);
});

test('treats an unfetchable base ref as zero entries', (t) => {
  const { root, work } = makeRepos('lib/a.dart\n', '# nothing left\n');
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const { status, out } = run(work, 'no-such-branch');
  assert.strictEqual(status, 0, out);
  assert.match(out, /base=0 head=0/);
});

test('does not move the checked-out branch', (t) => {
  const { root, work } = makeRepos('lib/a.dart\n', 'lib/a.dart\n');
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const before = git(work, 'rev-parse', 'HEAD');
  assert.strictEqual(run(work).status, 0);
  assert.strictEqual(git(work, 'rev-parse', 'HEAD'), before);
  assert.strictEqual(git(work, 'branch', '--show-current').trim(), 'main');
});

test('rejects a wrong argument count', () => {
  const result = spawnSync('bash', [SCRIPT, ALLOWLIST], { encoding: 'utf8' });
  assert.strictEqual(result.status, 2);
  assert.match(result.stderr, /usage:/);
});
