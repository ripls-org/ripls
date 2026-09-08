'use strict';

const test = require('node:test');
const assert = require('node:assert');
const path = require('path');

const {
  violationsForComment,
  scanSource,
  isExcluded,
} = require('./check_todo_refs.js');

// Note: this file is on check_todo_refs.js's self-exempt list, which is why it
// can spell out the banned markers below without failing its own gate.

test('flags a bare TODO with no issue reference', () => {
  assert.deepStrictEqual(violationsForComment('// TODO: fix this later'), ['todo-no-issue']);
});

test('accepts TODO with an issue reference', () => {
  assert.deepStrictEqual(violationsForComment('// TODO(#1234): explain the gap'), []);
});

test('rejects a non-numeric TODO tag', () => {
  // TODO(some-slug) reads like it is tracked but points at nothing retrievable.
  assert.deepStrictEqual(violationsForComment('// TODO(catalyst-pull-rpc): later'), [
    'todo-no-issue',
  ]);
});

test('rejects FIXME even when it carries an issue number', () => {
  // CLAUDE.md standardizes on TODO(#NNNN); FIXME is not used at all.
  const reasons = violationsForComment('// FIXME(#1234): still banned');
  assert.ok(reasons.includes('fixme'), `expected fixme, got ${JSON.stringify(reasons)}`);
});

test('reports both reasons when a comment has TODO and FIXME', () => {
  const reasons = violationsForComment('// TODO and FIXME on one line');
  assert.deepStrictEqual(reasons.sort(), ['fixme', 'todo-no-issue']);
});

test('does not flag lowercase todo prose', () => {
  // The rule targets the marker, not the English word.
  assert.deepStrictEqual(violationsForComment('// things todo before release'), []);
});

test('does not flag TODO inside a string literal', () => {
  const src = ['func f() {', '\tmsg := "TODO: not a comment"', '}'].join('\n');
  assert.deepStrictEqual(scanSource('a.go', src), []);
});

test('flags TODO in an inline trailing comment', () => {
  const src = ['func f() {', '\tx := 1 // TODO: use a constant', '}'].join('\n');
  const v = scanSource('a.go', src);
  assert.strictEqual(v.length, 1);
  assert.strictEqual(v[0].line, 2);
  assert.strictEqual(v[0].reason, 'todo-no-issue');
});

test('flags TODO inside a multi-line block comment', () => {
  const src = ['/*', ' * TODO: split this file', ' */', 'package main'].join('\n');
  const v = scanSource('a.go', src);
  assert.strictEqual(v.length, 1);
  assert.strictEqual(v[0].line, 2);
});

test('flags TODO in a Dart doc comment', () => {
  const src = ['/// TODO: remove this factory', 'class A {}'].join('\n');
  assert.strictEqual(scanSource('a.dart', src).length, 1);
});

test('reports only the marker line of a multi-line TODO', () => {
  // The continuation lines carry no marker; one report per TODO keeps output
  // proportional to the number of things to fix.
  const src = [
    '// TODO(#42): first line is compliant',
    '// second line has no marker',
    '// TODO: this one is not',
  ].join('\n');
  const v = scanSource('a.go', src);
  assert.strictEqual(v.length, 1);
  assert.strictEqual(v[0].line, 3);
});

test('excludes generated and l10n files', () => {
  const cases = [
    path.join('/repo', 'server', 'gen', 'ripls', 'api', 'x.go'),
    path.join('/repo', 'app', 'lib', 'x.g.dart'),
    path.join('/repo', 'app', 'lib', 'x.freezed.dart'),
    path.join('/repo', 'app', 'lib', 'x.mocks.dart'),
    path.join('/repo', 'server', 'foo.pb.go'),
    path.join('/repo', 'server', 'foo_connect.go'),
    path.join('/repo', 'app', 'lib', 'l10n', 'app_localizations_en.dart'),
  ];
  for (const c of cases) {
    assert.ok(isExcluded(c), `expected excluded: ${c}`);
  }
});

test('does NOT exclude test files', () => {
  // CLAUDE.md's TODO rule has no test exemption, and two of the twelve TODOs
  // #1611 triaged lived in server/integration_tests.
  assert.strictEqual(isExcluded(path.join('/repo', 'server', 'foo_test.go')), false);
  assert.strictEqual(isExcluded(path.join('/repo', 'app', 'lib', 'foo_test.dart')), false);
});
