// Tests for runners/forgejo/mac/prune.sh, the nightly disk prune on the Mac
// that builds iOS. Each test builds a fake $HOME and runs the real script.
const { test } = require('node:test');
const assert = require('node:assert');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const SCRIPT = path.join(__dirname, '..', 'runners', 'forgejo', 'mac', 'prune.sh');

function makeHome() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'runner-prune-'));
}

// Creates directories under `root`, oldest first, one hour apart, so mtime
// order matches the order given.
function makeVersions(root, names) {
  const base = Date.now() / 1000 - names.length * 3600;
  names.forEach((name, i) => {
    const dir = path.join(root, name);
    fs.mkdirSync(dir, { recursive: true });
    fs.writeFileSync(path.join(dir, 'sdk'), 'x');
    fs.utimesSync(dir, base + i * 3600, base + i * 3600);
  });
}

function prune(home, env = {}, args = []) {
  return execFileSync('bash', [SCRIPT, ...args], {
    env: {
      PATH: process.env.PATH,
      HOME: home,
      PRUNE_BUSY: '0',
      PRUNE_FREE_GB: '100',
      ...env,
    },
    encoding: 'utf8',
  });
}

const ls = (dir) => fs.readdirSync(dir).sort();

test('keeps the two newest versions of each tool in both tool caches', () => {
  const home = makeHome();
  const forgejo = path.join(home, 'forgejo-runner/work/tool_cache');
  const github = path.join(home, 'actions-runner/_work/_tool');
  makeVersions(path.join(forgejo, 'flutter'), ['stable-3.47.7-arm64', 'stable-3.47.8-arm64', 'stable-3.47.9-arm64']);
  makeVersions(path.join(github, 'flutter'), ['stable-3.44.0-arm64', 'stable-3.47.8-arm64', 'stable-3.47.9-arm64']);
  makeVersions(path.join(github, 'go'), ['1.26.6', '1.27.0', '1.27.1']);
  makeVersions(path.join(github, 'node'), ['24.16.0']);

  prune(home);

  assert.deepStrictEqual(ls(path.join(forgejo, 'flutter')), ['stable-3.47.8-arm64', 'stable-3.47.9-arm64']);
  assert.deepStrictEqual(ls(path.join(github, 'flutter')), ['stable-3.47.8-arm64', 'stable-3.47.9-arm64']);
  assert.deepStrictEqual(ls(path.join(github, 'go')), ['1.27.0', '1.27.1']);
  assert.deepStrictEqual(ls(path.join(github, 'node')), ['24.16.0']);
});

test('orders versions by install time, not by name', () => {
  const home = makeHome();
  const flutter = path.join(home, 'forgejo-runner/work/tool_cache/flutter');
  // A rollback: 3.47.8 was installed last.
  makeVersions(flutter, ['stable-3.47.9-arm64', 'stable-3.50.0-arm64', 'stable-3.47.8-arm64']);

  prune(home, { KEEP_VERSIONS: '1' });

  assert.deepStrictEqual(ls(flutter), ['stable-3.47.8-arm64']);
});

test('removes GitHub runner versions the links no longer point at', () => {
  const home = makeHome();
  const runner = path.join(home, 'actions-runner');
  makeVersions(runner, ['bin.2.338.0', 'bin.2.338.2', 'externals.2.337.0', 'externals.2.338.2', '_work/_update']);
  fs.symlinkSync(path.join(runner, 'bin.2.338.2'), path.join(runner, 'bin'));
  fs.symlinkSync(path.join(runner, 'externals.2.338.2'), path.join(runner, 'externals'));

  prune(home);

  assert.deepStrictEqual(ls(runner), ['_work', 'bin', 'bin.2.338.2', 'externals', 'externals.2.338.2']);
  assert.deepStrictEqual(ls(path.join(runner, '_work')), []);
});

test('leaves runner versions alone when the link is missing', () => {
  const home = makeHome();
  const runner = path.join(home, 'actions-runner');
  makeVersions(runner, ['bin.2.338.0', 'bin.2.338.2']);

  prune(home);

  assert.deepStrictEqual(ls(runner), ['bin.2.338.0', 'bin.2.338.2']);
});

test('clears download caches only under the low-water mark', () => {
  const home = makeHome();
  const caches = ['go/pkg/mod', '.pub-cache/hosted', 'Library/Caches/CocoaPods'];
  makeVersions(home, caches);
  // Like the real Go module cache: read-only.
  fs.chmodSync(path.join(home, 'go/pkg/mod/sdk'), 0o444);
  fs.chmodSync(path.join(home, 'go/pkg/mod'), 0o555);
  makeVersions(home, ['Library/Developer/Xcode/Archives/2026-10-23', 'Library/Developer/Xcode/DerivedData/Runner-abc']);

  prune(home, { PRUNE_FREE_GB: '100' });
  for (const c of caches) assert.ok(fs.existsSync(path.join(home, c)), `${c} kept with space free`);

  const out = prune(home, { PRUNE_FREE_GB: '10' });
  for (const c of caches) assert.ok(!fs.existsSync(path.join(home, c)), `${c} cleared when low`);
  assert.match(out, /WARNING: still under 30 GB free/);
  // Never caches, whatever the pressure.
  assert.ok(fs.existsSync(path.join(home, 'Library/Developer/Xcode/Archives/2026-10-23')));
  assert.ok(fs.existsSync(path.join(home, 'Library/Developer/Xcode/DerivedData/Runner-abc')));
});

test('does nothing while a job is running', () => {
  const home = makeHome();
  const flutter = path.join(home, 'forgejo-runner/work/tool_cache/flutter');
  makeVersions(flutter, ['a', 'b', 'c']);

  const out = prune(home, { PRUNE_BUSY: '1', PRUNE_FREE_GB: '1' });

  assert.match(out, /a job is running; skipped/);
  assert.deepStrictEqual(ls(flutter), ['a', 'b', 'c']);
});

test('--dry-run reports and removes nothing', () => {
  const home = makeHome();
  const flutter = path.join(home, 'forgejo-runner/work/tool_cache/flutter');
  makeVersions(flutter, ['a', 'b', 'c']);
  makeVersions(home, ['go/pkg/mod']);

  const out = prune(home, { PRUNE_FREE_GB: '1' }, ['--dry-run']);

  assert.match(out, /would remove .*tool_cache\/flutter\/a/);
  assert.match(out, /would remove .*go\/pkg\/mod/);
  assert.deepStrictEqual(ls(flutter), ['a', 'b', 'c']);
  assert.ok(fs.existsSync(path.join(home, 'go/pkg/mod')));
});

test('an empty home is not an error', () => {
  prune(makeHome());
});
