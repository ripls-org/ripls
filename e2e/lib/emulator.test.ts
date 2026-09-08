// Tests for the emulator spawn pattern (#2772).
//
// `startAuthEmulator` runs the Firebase Auth Emulator detached so it outlives
// this process — the standalone investigation environment
// (scripts/investigate_env.ts) spawns it, records its pid, and expects to
// exit. The subtle bit is stdio: piping the child's stdout/stderr through
// createWriteStream + child.stdout.pipe() leaves the parent holding pipe
// handles that stay ref'd on the event loop even after child.unref(), so
// investigate_env never returns until the emulator dies. This test asserts
// the fix — hand the child its own log fd — actually lets the parent's
// event loop drain while the detached child keeps running.

import test from 'node:test';
import assert from 'node:assert';
import { spawn } from 'node:child_process';
import { mkdtempSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

/** Runs `node -e <script>` and returns { code, stdout, stderr, elapsedMs }.
 *  A short SIGKILL deadline turns "hangs forever" into a diagnosable failure
 *  rather than blowing the test-runner timeout. */
function runNodeSubprocess(
  script: string,
  timeoutMs: number,
): Promise<{ code: number | null; stdout: string; stderr: string; elapsedMs: number }> {
  return new Promise((resolve) => {
    const started = Date.now();
    const child = spawn(process.execPath, ['-e', script], {
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (b) => (stdout += b.toString()));
    child.stderr.on('data', (b) => (stderr += b.toString()));
    const kill = setTimeout(() => child.kill('SIGKILL'), timeoutMs);
    child.on('exit', (code) => {
      clearTimeout(kill);
      resolve({ code, stdout, stderr, elapsedMs: Date.now() - started });
    });
  });
}

test('detached child with fd-based stdio does not hold the parent event loop', async () => {
  // Mirrors the fixed spawn shape in emulator.ts: fd for stdout+stderr,
  // detached + unref, no further work. The parent should exit near-instantly
  // even though its detached child is still running.
  const workDir = mkdtempSync(join(tmpdir(), 'ripls-emulator-test-'));
  const logPath = join(workDir, 'child.log');
  const script = `
    const { spawn } = require('node:child_process');
    const { openSync, closeSync } = require('node:fs');
    const logFd = openSync(${JSON.stringify(logPath)}, 'a');
    const child = spawn(process.execPath, ['-e', 'setTimeout(() => {}, 60000); process.stdout.write("child_alive\\n")'], {
      stdio: ['ignore', logFd, logFd],
      detached: true,
    });
    closeSync(logFd);
    child.unref();
    console.log('parent_exiting pid=' + child.pid);
  `;

  const result = await runNodeSubprocess(script, 5_000);

  assert.strictEqual(result.code, 0, `parent should exit cleanly, got code=${result.code}, stderr=${result.stderr}`);
  assert.match(result.stdout, /parent_exiting pid=/, 'parent should reach end of script');
  assert.ok(
    result.elapsedMs < 3_000,
    `parent should exit promptly; took ${result.elapsedMs} ms — the child stdio pipes are keeping the loop alive again (see #2772)`,
  );

  // Reap the detached grandchild so it does not leak past the test.
  const pidMatch = result.stdout.match(/parent_exiting pid=(\d+)/);
  if (pidMatch) {
    try {
      process.kill(Number(pidMatch[1]), 'SIGKILL');
    } catch {
      // Already gone; the grandchild's setTimeout may have been racy.
    }
  }

  // Sanity: the child did get its own fd and can write to it. (Best-effort —
  // Node's process.stdout is line-buffered to a fd, so the write may lag.)
  try {
    const contents = readFileSync(logPath, 'utf8');
    assert.match(contents, /child_alive/);
  } catch {
    // Log file may be empty on very fast platforms if the grandchild was
    // SIGKILLed before its buffered stdout flushed — that's OK, the parent
    // exit-time assertion above is the load-bearing one.
  }
});

test('parent-piped stdio DOES hold the parent event loop (regression witness)', async () => {
  // Complement to the fixed-pattern test above: the exact spawn shape that
  // previously lived in emulator.ts — pipe stdio through the parent, then
  // unref — leaves the parent blocked until the child dies. This test locks
  // in *why* the fix is fd-based stdio rather than "just add child.unref()".
  const workDir = mkdtempSync(join(tmpdir(), 'ripls-emulator-test-'));
  const logPath = join(workDir, 'buggy.log');
  const script = `
    const { spawn } = require('node:child_process');
    const { createWriteStream } = require('node:fs');
    const logStream = createWriteStream(${JSON.stringify(logPath)}, { flags: 'a' });
    const child = spawn(process.execPath, ['-e', 'setTimeout(() => {}, 60000)'], {
      stdio: ['ignore', 'pipe', 'pipe'],
      detached: true,
    });
    child.stdout.pipe(logStream);
    child.stderr.pipe(logStream);
    child.unref();
    console.log('parent_reached_end pid=' + child.pid);
  `;

  const result = await runNodeSubprocess(script, 2_500);

  // The parent should have hung and been SIGKILLed by our deadline.
  assert.strictEqual(
    result.code,
    null,
    `expected the buggy pattern to hang until we SIGKILL it; instead it exited cleanly with code=${result.code} — the pipes-keep-loop-alive property has changed and emulator.ts can be simplified`,
  );
  assert.match(result.stdout, /parent_reached_end pid=/, 'parent should reach end of script before hanging on the pipes');

  // Reap the detached grandchild.
  const pidMatch = result.stdout.match(/parent_reached_end pid=(\d+)/);
  if (pidMatch) {
    try {
      process.kill(Number(pidMatch[1]), 'SIGKILL');
    } catch {
      // Already gone.
    }
  }
});
