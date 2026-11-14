'use strict';

const assert = require('node:assert');
const { execFileSync, spawnSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const {
  denylistPath,
  parseDenylist,
  scanText,
  summarize,
  isExempt,
  isScannable,
} = require('./oss_scan.js');

// This file is on oss_scan.js's EXEMPT_PATHS list, which is why it can spell out
// the shapes it tests without tripping its own gate.

const RULES = parseDenylist(
  [
    '# comment',
    '[org-legal-identity]',
    'ExampleCorp',
    '',
    '[cloud-projects]',
    'demo-(dev|prod)',
    '[people-and-pii]',
    '\\+1[2-9][0-9]{9}',
  ].join('\n'),
);

// --- denylist parsing ------------------------------------------------------

test('parses sections into categories', () => {
  assert.deepStrictEqual(
    RULES.map((r) => r.category),
    ['org-legal-identity', 'cloud-projects', 'people-and-pii'],
  );
});

test('skips comments and blank lines', () => {
  assert.strictEqual(RULES.length, 3);
});

test('patterns are case-insensitive', () => {
  const findings = scanText('a.go', 'examplecorp', RULES);
  assert.strictEqual(findings.length, 1);
});

test('lookbehind patterns compile and constrain the match', () => {
  // The real denylist uses `(?<!org\.)` so a reverse-DNS bundle id is not
  // counted as a hostname leak (Correction 8). Guard both that the syntax
  // survives parsing and that it actually discriminates.
  const rules = parseDenylist(['[domains]', '(?<!org\\.)demo\\.app'].join('\n'));

  assert.strictEqual(scanText('a.go', 'https://demo.app/x', rules).length, 1);
  assert.strictEqual(scanText('a.go', 'org.demo.app', rules).length, 0);
});

// --- scanning --------------------------------------------------------------

test('reports file and line but never the matched text', () => {
  const findings = scanText('server/main.go', ['clean', 'ExampleCorp here'].join('\n'), RULES);
  assert.deepStrictEqual(findings, [
    { file: 'server/main.go', line: 2, category: 'org-legal-identity' },
  ]);
  // The finding object must carry no match/excerpt field — the report and the
  // --json output both serialize whatever is here.
  assert.deepStrictEqual(Object.keys(findings[0]).sort(), ['category', 'file', 'line']);
});

test('reports one finding per category per line, not per pattern', () => {
  const rules = parseDenylist(['[dupes]', 'alpha', 'beta'].join('\n'));
  const findings = scanText('a.md', 'alpha and beta on one line', rules);
  assert.strictEqual(findings.length, 1);
});

test('reports separate categories on the same line', () => {
  const findings = scanText('a.md', 'ExampleCorp runs demo-prod', RULES);
  assert.deepStrictEqual(findings.map((f) => f.category).sort(), [
    'cloud-projects',
    'org-legal-identity',
  ]);
});

test('finds an alternation branch', () => {
  assert.strictEqual(scanText('a.tf', 'demo-dev', RULES).length, 1);
  assert.strictEqual(scanText('a.tf', 'demo-prod', RULES).length, 1);
  assert.strictEqual(scanText('a.tf', 'demo-staging', RULES).length, 0);
});

test('clean text produces no findings', () => {
  assert.deepStrictEqual(scanText('a.go', 'package main\n\nfunc main() {}\n', RULES), []);
});

// --- reserved phone numbers ------------------------------------------------

test('reserved 555 test numbers do not trip the phone pattern', () => {
  // Subtracted from the line before matching, so the pattern stays strong.
  assert.deepStrictEqual(scanText('x_test.go', 'phone := "+15551234567"', RULES), []);
  assert.deepStrictEqual(scanText('x_test.go', 'phone := "+14155551234"', RULES), []);
});

test('the canonical Twilio example number is allowed', () => {
  assert.deepStrictEqual(scanText('twilio_test.go', '+14158675309', RULES), []);
});

test('a real-looking phone number is still caught', () => {
  const findings = scanText('docs/note.md', 'call +12223334444 for details', RULES);
  assert.deepStrictEqual(
    findings.map((f) => f.category),
    ['people-and-pii'],
  );
});

test('a reserved number on the same line does not mask a real one', () => {
  const findings = scanText('a.md', '+15551234567 and +12223334444', RULES);
  assert.strictEqual(findings.length, 1);
});

// --- exemptions ------------------------------------------------------------

test('TRADEMARK.md and NOTICE are exempt — they must name the marks', () => {
  assert.ok(isExempt('TRADEMARK.md'));
  assert.ok(isExempt('NOTICE'));
});

test('the denylist and scanner are exempt from themselves', () => {
  assert.ok(isExempt('scripts/oss_denylist.txt'));
  assert.ok(isExempt('scripts/oss_scan.js'));
});

test('ordinary product files are not exempt', () => {
  assert.ok(!isExempt('server/main.go'));
  assert.ok(!isExempt('README.md'));
  assert.ok(!isExempt('docs/client/chat.md'));
});

test('exemption is exact, not prefix-based, for single files', () => {
  // A file that merely starts with an exempt name must not inherit exemption.
  assert.ok(!isExempt('TRADEMARK.md.bak'));
  assert.ok(!isExempt('NOTICE_TO_USERS.md'));
});

// --- binary detection ------------------------------------------------------

test('binary extensions are skipped', () => {
  assert.ok(!isScannable('app/assets/icon.png'));
  assert.ok(!isScannable('model_tuning/ripls_embedding.onnx'));
  assert.ok(!isScannable('e2e/fixtures/hero.MP4'));
});

test('text and source files are scanned', () => {
  assert.ok(isScannable('server/main.go'));
  assert.ok(isScannable('docs/plan.md'));
  assert.ok(isScannable('Dockerfile'));
});

// --- summary ---------------------------------------------------------------

test('summarize counts hits and distinct files per category', () => {
  const byCategory = summarize([
    { file: 'a.go', line: 1, category: 'cloud-projects' },
    { file: 'a.go', line: 9, category: 'cloud-projects' },
    { file: 'b.go', line: 3, category: 'cloud-projects' },
    { file: 'c.md', line: 2, category: 'people-and-pii' },
  ]);
  assert.strictEqual(byCategory.get('cloud-projects').hits, 3);
  assert.strictEqual(byCategory.get('cloud-projects').files.size, 2);
  assert.strictEqual(byCategory.get('people-and-pii').hits, 1);
});

// --- the CLI: denylist location, tree mode, redaction -----------------------
//
// These run the script as a child process against a throwaway checkout and a
// fixture denylist, because what matters is what lands in stdout/stderr — on a
// public repo, a CI log is a publication channel.

const SCRIPT = path.join(__dirname, 'oss_scan.js');

// The pattern and the text it matches differ, so a leak of either the rule or
// the match would show up in the assertions below.
const PLANTED = 'Zq7SecretMarker';
const FIXTURE_DENYLIST = ['[planted]', 'zq7[a-z]+marker'].join('\n');

function makeCheckout() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'oss-scan-'));
  execFileSync('git', ['init', '-q', dir]);
  fs.writeFileSync(path.join(dir, 'clean.md'), 'nothing to see\n');
  fs.writeFileSync(path.join(dir, 'leak.md'), `line one\nhello ${PLANTED}\n`);
  execFileSync('git', ['-C', dir, 'add', 'clean.md', 'leak.md']);
  // Untracked: tree mode must not scan it.
  fs.writeFileSync(path.join(dir, 'scratch.md'), `${PLANTED}\n`);
  const denylist = path.join(dir, '..', `${path.basename(dir)}.denylist`);
  fs.writeFileSync(denylist, FIXTURE_DENYLIST);
  return { dir, denylist };
}

function runScan(args, denylist) {
  const env = { ...process.env, OSS_DENYLIST_FILE: denylist };
  const res = spawnSync(process.execPath, [SCRIPT, ...args], { env, encoding: 'utf8' });
  return { code: res.status, out: res.stdout + res.stderr, stdout: res.stdout };
}

test('denylistPath prefers OSS_DENYLIST_FILE and falls back to the repo copy', () => {
  assert.strictEqual(denylistPath({ OSS_DENYLIST_FILE: '/x/list.txt' }), '/x/list.txt');
  assert.strictEqual(
    denylistPath({}),
    path.join(__dirname, 'oss_denylist.txt'),
  );
});

test('a missing denylist exits 2 instead of scanning with no rules', () => {
  const { dir } = makeCheckout();
  const res = runScan(['--gate', '--tree', '--dir', dir], path.join(dir, 'nope.txt'));
  assert.strictEqual(res.code, 2);
  assert.match(res.out, /OSS_DENYLIST_FILE/);
});

test('tree mode scans tracked files only', () => {
  const { dir, denylist } = makeCheckout();
  const res = runScan(['--report', '--json', '--tree', '--dir', dir], denylist);
  assert.strictEqual(res.code, 0);
  const report = JSON.parse(res.stdout);
  assert.strictEqual(report.scanned, 2);
  assert.deepStrictEqual(report.locations, ['leak.md:2 [planted]']);
});

test('tree mode gate fails on a tracked hit and passes once it is gone', () => {
  const { dir, denylist } = makeCheckout();
  assert.strictEqual(runScan(['--gate', '--tree', '--dir', dir], denylist).code, 1);

  execFileSync('git', ['-C', dir, 'rm', '-q', '--cached', 'leak.md']);
  assert.strictEqual(runScan(['--gate', '--tree', '--dir', dir], denylist).code, 0);
});

test('no output mode prints the matched text or the pattern', () => {
  const { dir, denylist } = makeCheckout();
  for (const args of [
    ['--report'],
    ['--report', '--json'],
    ['--gate'],
    ['--gate', '--json'],
  ]) {
    const res = runScan([...args, '--tree', '--dir', dir], denylist);
    assert.ok(!res.out.toLowerCase().includes(PLANTED.toLowerCase()), `${args} leaked the match`);
    assert.ok(!res.out.includes('zq7[a-z]+marker'), `${args} leaked the pattern`);
  }
});

test('`!gate-accepted` marks a whole section and does not leak into the next', () => {
  const rules = parseDenylist(
    [
      '[accepted]',
      '!gate-accepted',
      'alpha',
      '[blocking]',
      'beta',
    ].join('\n'),
  );

  assert.deepStrictEqual(
    rules.map((r) => [r.category, r.gateAccepted]),
    [
      ['accepted', true],
      ['blocking', false],
    ],
  );
});

test('the directive is not parsed as a pattern', () => {
  const rules = parseDenylist(['[x]', '!gate-accepted', 'alpha'].join('\n'));
  assert.strictEqual(rules.length, 1);
  assert.strictEqual(rules[0].source, 'alpha');
});

// --- per-path allowance ------------------------------------------------------

test('`!allow-under` skips a section only under the named prefix', () => {
  const rules = parseDenylist(
    ['[public-identity]', '!allow-under site/content/', 'alpha', '[other]', 'beta'].join('\n'),
  );

  assert.deepStrictEqual(scanText('site/content/about.html', 'alpha', rules), []);
  assert.strictEqual(scanText('server/main.go', 'alpha', rules).length, 1);
  // A path that merely starts with the same letters is not under the prefix.
  assert.strictEqual(scanText('site/content-old/a.html', 'alpha', rules).length, 1);
  // The allowance does not carry into the next section.
  assert.strictEqual(scanText('site/content/about.html', 'beta', rules).length, 1);
});

test('`!allow-under` needs a directory prefix', () => {
  assert.throws(() => parseDenylist(['[x]', '!allow-under', 'alpha'].join('\n')), /allow-under/);
  assert.throws(() => parseDenylist(['[x]', '!allow-under site', 'alpha'].join('\n')), /allow-under/);
});

