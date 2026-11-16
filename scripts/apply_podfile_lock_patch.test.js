'use strict';

const test = require('node:test');
const assert = require('node:assert');
const zlib = require('node:zlib');

const { extractPatch, checkPatch, BEGIN, END, TARGET } = require('./apply_podfile_lock_patch.js');

const DIFF = [
  `diff --git a/${TARGET} b/${TARGET}`,
  'index 1111111..2222222 100644',
  `--- a/${TARGET}`,
  `+++ b/${TARGET}`,
  '@@ -1,3 +1,3 @@',
  ' PODS:',
  '-  - Firebase/Auth (12.18.0):',
  '+  - Firebase/Auth (12.19.3):',
  '     - FirebaseAuth (~> 12.19.3)',
  '',
].join('\n');

// What the workflow step prints: base64 of the gzipped diff, folded at 76,
// each line prefixed the way a saved job log prefixes it.
function asLog(diff, prefix = '2026-11-16T22:55:01.1234567Z ') {
  const encoded = zlib.gzipSync(Buffer.from(diff)).toString('base64');
  const folded = encoded.match(/.{1,76}/g);
  return [`::error::${TARGET} changed after 'pod install'.`, 'some diff text', BEGIN, ...folded, '', END, 'exit 1']
    .map((l) => prefix + l)
    .join('\n');
}

test('extractPatch recovers the diff from a prefixed, folded log', () => {
  assert.strictEqual(extractPatch(asLog(DIFF)), DIFF);
  assert.strictEqual(extractPatch(asLog(DIFF, '')), DIFF);
});

test('extractPatch takes the last patch when the log holds two', () => {
  const second = DIFF.replace('12.19.3', '12.20.0');
  assert.strictEqual(extractPatch(`${asLog(DIFF)}\n${asLog(second)}`), second);
});

test('extractPatch refuses a log without a usable patch', () => {
  assert.throws(() => extractPatch('nothing here'), /no patch between/);
  assert.throws(() => extractPatch(`${BEGIN}\nnot base64 !!\n${END}`), /not base64/);
  assert.throws(() => extractPatch(`${BEGIN}\naGVsbG8=\n${END}`), /could not decompress/);
});

test('checkPatch accepts a change to Podfile.lock only', () => {
  assert.doesNotThrow(() => checkPatch(DIFF));
});

test('checkPatch refuses any other file, a second file, or a rename', () => {
  assert.throws(() => checkPatch(DIFF.replaceAll(TARGET, 'app/pubspec.yaml')), /nothing else/);
  const two = `${DIFF}diff --git a/.github/workflows/x.yaml b/.github/workflows/x.yaml\n`;
  assert.throws(() => checkPatch(two), /nothing else/);
  assert.throws(() => checkPatch('no header at all'), /no file header/);
  const renamed = DIFF.replace('index 1111111..2222222 100644', 'rename from app/ios/Podfile.lock\nrename to scripts/forge');
  assert.throws(() => checkPatch(renamed), /renames/);
});
