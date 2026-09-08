// Tests for the e2e server-lifecycle guards (#2770).
//
// `assertPortFree` exists because the failure it prevents is silent and very
// expensive: an orphan server left by a killed run answers /health on the first
// poll, wins the race against the freshly-spawned child, and then serves the
// whole run from whatever binary it embeds. Source edits and rebuilds appear to
// do nothing, which reads exactly like a stale bundle. A regression here would
// restore that, so the occupied case asserts on real listening sockets rather
// than a mocked lsof.

import test from 'node:test';
import assert from 'node:assert';
import { createServer, type Server } from 'node:net';

import { assertPortFree } from './server.js';

/** Binds an ephemeral port and resolves once it is genuinely listening. */
function listenEphemeral(): Promise<{ server: Server; port: number }> {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (address === null || typeof address === 'string') {
        reject(new Error('expected a TCP address'));
        return;
      }
      resolve({ server, port: address.port });
    });
  });
}

function close(server: Server): Promise<void> {
  return new Promise((resolve) => server.close(() => resolve()));
}

test('assertPortFree accepts a port nothing is listening on', async () => {
  // Bind then release, so the port is known-valid and known-free rather than
  // guessed — a hardcoded number could collide with something on the machine
  // and turn this into a flake that only fails on one developer's laptop.
  const { server, port } = await listenEphemeral();
  await close(server);

  await assertPortFree(port);
});

test('assertPortFree rejects a port already being listened on', async () => {
  const { server, port } = await listenEphemeral();
  try {
    await assert.rejects(assertPortFree(port), (err: Error) => {
      assert.match(err.message, /already in use/);
      // The occupant listing is the actionable part: without it the operator
      // cannot tell an orphan from a legitimate concurrent run. Its CONTENT is
      // best-effort — naming the pid needs lsof, which the CI container does
      // not have — so the section must exist but is not asserted further.
      assert.match(err.message, /Occupant:/);
      return true;
    });
  } finally {
    await close(server);
  }
});

test('assertPortFree releases its verdict on the same port once freed', async () => {
  // Guards that latch would block a legitimate restart after env:stop, which is
  // the single most common thing an operator does after hitting this error.
  const { server, port } = await listenEphemeral();
  try {
    await assert.rejects(assertPortFree(port), /already in use/);
  } finally {
    // In a finally, because a failure above used to leave this socket open —
    // which kept node's event loop alive and hung `node --test` forever. In CI
    // that consumed the Web E2E job's entire 35-minute budget before Playwright
    // ever started. A leaked handle turns one failed assertion into a timeout.
    await close(server);
  }
  await assertPortFree(port);
});
