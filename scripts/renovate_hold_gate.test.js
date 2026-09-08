'use strict';

const test = require('node:test');
const assert = require('node:assert');

const {
  heldPackages,
  parseDiff,
  hasToken,
  matchHold,
} = require('./renovate_hold_gate.js');

// A trimmed stand-in for renovate.json carrying the holds that actually caused
// trouble: `golang` is the Docker base image, `protobuf` the pub package.
const CONFIG = {
  packageRules: [
    { description: 'not a hold', matchManagers: ['gomod'], automerge: true },
    { matchManagers: ['dockerfile'], matchPackageNames: ['golang'], enabled: false },
    { matchManagers: ['pub'], matchPackageNames: ['protobuf'], enabled: false },
    {
      matchManagers: ['gomod'],
      matchPackageNames: ['github.com/yalue/onnxruntime_go'],
      allowedVersions: '<1.25',
    },
    // Regex matchers can't be looked for as literal tokens.
    { matchManagers: ['npm'], matchPackageNames: ['/^@scope/'], enabled: false },
  ],
};

const diffFor = (path, lines) =>
  [`diff --git a/${path} b/${path}`, '--- a/' + path, '+++ b/' + path, '@@ -1 +1 @@', ...lines].join('\n');

test('only rules with allowedVersions or enabled:false are holds', () => {
  const names = heldPackages(CONFIG).map((h) => h.name);
  assert.deepStrictEqual(names, ['golang', 'protobuf', 'github.com/yalue/onnxruntime_go']);
});

test('parseDiff keeps added/removed content but not file headers', () => {
  const files = parseDiff(diffFor('go.mod', ['-old v1', '+new v2', ' context']));
  assert.strictEqual(files.length, 1);
  assert.strictEqual(files[0].path, 'go.mod');
  assert.deepStrictEqual(files[0].lines, ['old v1', 'new v2']);
});

test('hasToken requires whole-token boundaries', () => {
  assert.ok(hasToken('FROM golang:1.26', 'golang'));
  assert.ok(hasToken('  protobuf: ^4.2.0', 'protobuf'));
  // The bug that closed PR #2655: `golang` must not match a Go module path.
  assert.ok(!hasToken('\tgolang.org/x/text v0.34.0', 'golang'));
  assert.ok(!hasToken('\tgoogle.golang.org/protobuf v1.36.12', 'protobuf'));
});

test('regression #2655: a go.mod-only bump does not trip the dockerfile golang hold', () => {
  // phonenumbers v1 -> v2 touches only go.mod. The old gate matched the string
  // "golang" in the PR body and wrongly reported a capped dependency.
  const diff = diffFor('go.mod', [
    '-\tgithub.com/nyaruka/phonenumbers v1.8.1',
    '+\tgithub.com/nyaruka/phonenumbers/v2 v2.0.8',
    '-\tgolang.org/x/text v0.34.0',
    '+\tgolang.org/x/text v0.35.0',
  ]);
  assert.strictEqual(matchHold(CONFIG, diff), null);
});

test('regression #2943: a large grouped go.mod bump does not trip pub protobuf', () => {
  // google.golang.org/protobuf is a Go module; the `protobuf` hold is the pub
  // package. Padding makes the input far larger than a pipe buffer — the shell
  // gate returned 141 (SIGPIPE under pipefail) and silently matched nothing.
  const padding = Array.from({ length: 4000 }, (_, i) => `+\texample.com/pkg${i} v1.0.0`);
  const diff = diffFor('go.mod', [
    '-\tgoogle.golang.org/protobuf v1.36.11',
    '+\tgoogle.golang.org/protobuf v1.36.12',
    '-\tgolang.org/x/crypto v0.54.0',
    '+\tgolang.org/x/crypto v0.55.0',
    ...padding,
  ]);
  assert.ok(diff.length > 100_000, 'fixture should exceed a pipe buffer');
  assert.strictEqual(matchHold(CONFIG, diff), null);
});

test('a real dockerfile golang bump is still caught', () => {
  const diff = diffFor('Dockerfile', ['-FROM golang:1.26', '+FROM golang:1.27']);
  assert.strictEqual(matchHold(CONFIG, diff), 'golang');
});

test('a real pub protobuf bump is still caught', () => {
  const diff = diffFor('app/pubspec.yaml', ['-  protobuf: ^4.2.0', '+  protobuf: ^6.0.0']);
  assert.strictEqual(matchHold(CONFIG, diff), 'protobuf');
});

test('a capped gomod dependency is caught in go.mod', () => {
  const diff = diffFor('go.mod', [
    '-\tgithub.com/yalue/onnxruntime_go v1.24.0',
    '+\tgithub.com/yalue/onnxruntime_go v1.35.0',
  ]);
  assert.strictEqual(matchHold(CONFIG, diff), 'github.com/yalue/onnxruntime_go');
});

test('manager scoping keeps a hold out of a foreign manifest', () => {
  // The literal word "golang" in a pubspec would not mean the Docker image.
  const diff = diffFor('app/pubspec.yaml', ['+  golang: ^1.0.0']);
  assert.strictEqual(matchHold(CONFIG, diff), null);
});

test('an empty diff matches nothing', () => {
  assert.strictEqual(matchHold(CONFIG, ''), null);
});
