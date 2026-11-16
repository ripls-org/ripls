#!/usr/bin/env node
/**
 * apply_podfile_lock_patch.js — apply the Podfile.lock change that Test iOS
 * Build printed in its log.
 *
 * `app/ios/Podfile.lock` can only be regenerated on a Mac. When a Flutter
 * plugin bump changes the pods, the iOS job (the only one on a Mac) fails its
 * "Verify Podfile.lock is up to date" step and prints the change it wanted as
 * a gzip + base64 patch between two marker lines. This reads a saved copy of
 * that log, decodes the patch and applies it, so that whoever fixes the pull
 * request from Linux (the CI fixer, usually) does not copy pod versions and
 * checksums by hand.
 *
 *     npm run ios:apply-podfile-lock-patch -- /tmp/ci-failure/failed.log
 *
 * It is a script, not a shell pipeline, for two reasons. The CI fixer may run
 * `npm` but not arbitrary pipelines. And a log is not a trusted place to take
 * a patch from: this applies it only if it changes app/ios/Podfile.lock and
 * nothing else.
 *
 * Exit 0 when applied, 1 when the log has no usable patch or git refuses it.
 */

'use strict';

const fs = require('node:fs');
const path = require('node:path');
const zlib = require('node:zlib');
const { spawnSync } = require('node:child_process');

const BEGIN = '-----BEGIN PODFILE LOCK PATCH-----';
const END = '-----END PODFILE LOCK PATCH-----';
const TARGET = 'app/ios/Podfile.lock';

/**
 * Pull the patch out of a job log. Each log line may carry a timestamp or
 * other prefix, so the payload is the last whitespace-separated field of each
 * line between the markers. If the log holds several patches (a job re-run),
 * the last one wins.
 */
function extractPatch(log) {
  let chunks = null;
  let found = null;
  for (const line of log.split(/\r?\n/)) {
    if (line.includes(BEGIN)) {
      chunks = [];
    } else if (line.includes(END)) {
      if (chunks) found = chunks;
      chunks = null;
    } else if (chunks) {
      // An empty output line is still a line in the log, with only its
      // prefix on it; a timestamp is not base64, so it is passed over.
      const field = line.trim().split(/\s+/).pop();
      if (field && /^[A-Za-z0-9+/]+=*$/.test(field)) chunks.push(field);
    }
  }
  if (!found) throw new Error(`no patch between "${BEGIN}" and "${END}" in the log`);
  const encoded = found.join('');
  if (!encoded) throw new Error('the text between the markers is not base64');
  let patch;
  try {
    patch = zlib.gunzipSync(Buffer.from(encoded, 'base64')).toString('utf8');
  } catch (err) {
    throw new Error(`could not decompress the patch: ${err.message}`);
  }
  return patch;
}

/** Throw unless the patch changes app/ios/Podfile.lock and only that. */
function checkPatch(patch) {
  const headers = patch.split('\n').filter((l) => l.startsWith('diff --git '));
  const want = `diff --git a/${TARGET} b/${TARGET}`;
  if (headers.length !== 1 || headers[0] !== want) {
    throw new Error(`the patch must change ${TARGET} and nothing else; it has: ${headers.join(' | ') || 'no file header'}`);
  }
  // A rename or mode change would let one header reach another path.
  if (/^(rename|copy) (from|to) |^(new|deleted) file mode |^(old|new) mode /m.test(patch)) {
    throw new Error('the patch renames, creates, deletes or re-modes a file');
  }
}

function main(argv) {
  const logPath = argv[0];
  if (!logPath) {
    process.stderr.write('usage: apply_podfile_lock_patch.js <saved job log>\n');
    return 1;
  }
  let patch;
  try {
    patch = extractPatch(fs.readFileSync(logPath, 'utf8'));
    checkPatch(patch);
  } catch (err) {
    process.stderr.write(`apply_podfile_lock_patch: ${err.message}\n`);
    return 1;
  }
  const root = path.resolve(__dirname, '..');
  const res = spawnSync('git', ['apply', '--whitespace=nowarn', '-'], { cwd: root, input: patch, encoding: 'utf8' });
  if (res.status !== 0) {
    process.stderr.write(`apply_podfile_lock_patch: git apply failed: ${(res.stderr || '').trim()}\n`);
    return 1;
  }
  process.stdout.write(`Applied the Podfile.lock patch from ${logPath}. Review with: git diff ${TARGET}\n`);
  return 0;
}

if (require.main === module) {
  process.exitCode = main(process.argv.slice(2));
}

module.exports = { extractPatch, checkPatch, BEGIN, END, TARGET };
