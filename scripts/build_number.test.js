'use strict';

// Tests for scripts/build_number.sh. Each case builds a throwaway git repo
// holding a copy of the script and a versions.env, since the script reads
// both relative to its own location.

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execFileSync, spawnSync } = require('child_process');

const SCRIPT = path.join(__dirname, 'build_number.sh');

function makeRepo(versionsEnv, commits) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'build-number-'));
  const git = (...args) => execFileSync('git', ['-C', dir, ...args], { stdio: 'ignore' });
  git('init', '-q');
  git('config', 'user.email', 'test@example.com');
  git('config', 'user.name', 'Test');
  git('config', 'commit.gpgsign', 'false');
  fs.mkdirSync(path.join(dir, 'scripts'));
  fs.copyFileSync(SCRIPT, path.join(dir, 'scripts', 'build_number.sh'));
  fs.writeFileSync(path.join(dir, 'versions.env'), versionsEnv);
  for (let i = 0; i < commits; i++) {
    git('commit', '-q', '--allow-empty', '-m', `c${i}`);
  }
  return dir;
}

function run(dir) {
  return spawnSync('bash', [path.join(dir, 'scripts', 'build_number.sh')], { encoding: 'utf8' });
}

test('offset 0 gives the commit count', () => {
  const r = run(makeRepo('GO_VERSION=1.0\nBUILD_NUMBER_OFFSET=0\n', 3));
  assert.strictEqual(r.status, 0, r.stderr);
  assert.strictEqual(r.stdout.trim(), '3');
});

test('a non-zero offset is added to the commit count', () => {
  const r = run(makeRepo('# comment\nBUILD_NUMBER_OFFSET=10300\n', 5));
  assert.strictEqual(r.status, 0, r.stderr);
  assert.strictEqual(r.stdout.trim(), '10305');
});

test('a missing offset fails rather than defaulting', () => {
  const r = run(makeRepo('GO_VERSION=1.0\n', 2));
  assert.notStrictEqual(r.status, 0);
  assert.match(r.stderr, /BUILD_NUMBER_OFFSET/);
});

test('a non-integer offset fails', () => {
  for (const bad of ['-5', '10k', '1.5', '']) {
    const r = run(makeRepo(`BUILD_NUMBER_OFFSET=${bad}\n`, 1));
    assert.notStrictEqual(r.status, 0, `offset '${bad}' should fail`);
    assert.match(r.stderr, /non-negative integer/);
  }
});
