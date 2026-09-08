'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');

const {
  parseVersionsEnv,
  extractGo,
  extractNode,
  extractFlutter,
  extractJava,
  extractXcode,
  extractCocoaPods,
  extractRuby,
  verdict,
  major,
} = require('./doctor_versions');

test('parseVersionsEnv reads KEY=VALUE and ignores comments/blanks', () => {
  const env = parseVersionsEnv([
    '# a comment',
    '',
    'GO_VERSION=1.25.9',
    '# renovate: datasource=node-version',
    'NODE_MAJOR=22',
    '  ', // whitespace-only
  ].join('\n'));
  assert.deepEqual(env, { GO_VERSION: '1.25.9', NODE_MAJOR: '22' });
});

test('extractGo parses `go version` output', () => {
  assert.equal(extractGo('go version go1.25.9 darwin/arm64'), '1.25.9');
  assert.equal(extractGo('nonsense'), null);
});

test('extractNode parses `node --version` (with and without v prefix)', () => {
  assert.equal(extractNode('v22.14.0'), '22.14.0');
  assert.equal(extractNode('22.14.0'), '22.14.0');
  assert.equal(extractNode(''), null);
});

test('extractFlutter parses the Flutter banner line', () => {
  assert.equal(extractFlutter('Flutter 3.44.0 • channel stable • https://...'), '3.44.0');
  assert.equal(extractFlutter('Dart 3.9.0'), null);
});

test('extractJava parses the openjdk version line (stderr)', () => {
  assert.equal(extractJava('openjdk version "17.0.13" 2024-10-15'), '17');
  assert.equal(extractJava('openjdk version "21" 2024'), '21');
  assert.equal(extractJava('garbage'), null);
});

test('extractXcode / extractCocoaPods / extractRuby', () => {
  assert.equal(extractXcode('Xcode 26.4.1\nBuild version 17E202'), '26.4.1');
  assert.equal(extractCocoaPods('1.16.2'), '1.16.2');
  assert.equal(extractRuby('ruby 4.0.3 (2025-12-25 revision abc) [arm64-darwin]'), '4.0.3');
});

test('major returns the leading component', () => {
  assert.equal(major('17.0.13'), '17');
  assert.equal(major('22'), '22');
});

test('verdict: exact mode', () => {
  assert.equal(verdict('exact', '3.44.0', '3.44.0'), 'ok');
  assert.equal(verdict('exact', '3.44.0', '3.43.0'), 'mismatch');
  assert.equal(verdict('exact', '3.44.0', null), 'undetected');
});

test('verdict: major mode compares only the major component', () => {
  assert.equal(verdict('major', '22', '22.14.0'), 'ok');
  assert.equal(verdict('major', '17', '17.0.13'), 'ok');
  assert.equal(verdict('major', '22', '20.10.0'), 'mismatch');
  assert.equal(verdict('major', '22', null), 'undetected');
});
