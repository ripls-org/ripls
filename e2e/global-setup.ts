// e2e/global-setup.ts — start the SHARED pieces of the e2e stack: the Firebase
// Auth Emulator (one process for the whole run). The per-worker pieces — a
// server + its own ephemeral database — are owned by the worker-scoped fixture
// in lib/fixtures.ts so the suite can run in parallel (workers > 1).
//
// Env contract:
//
//   E2E_DB_URL    Required unless E2E_BASE_URL is set. Postgres URL of the
//                 SHARED container; each worker creates its own database inside
//                 it. Local dev: the default from `npm run db:start`. CI: the URL
//                 from `go run ./server/cmd/web-smoke-db`.
//
//   E2E_BASE_URL  Optional. If set, setup is a near-no-op: it assumes a server is
//                 already running and reachable at this URL, and every worker
//                 reuses it (the "leave the server up, iterate on specs" loop).
//                 Leaving it UNSET is the signal each worker uses to start its
//                 own server — so global-setup must not set it here.
//
//   E2E_PORT      Optional, default 8080 — the BASE port. Worker i binds
//                 E2E_PORT + i. Exported as E2E_BASE_PORT for the fixture.
//
// Writes {authEmulatorPid, …} to `${TMPDIR}/ripls-e2e-state.json` so
// globalTeardown can stop the emulator. Playwright passes nothing between
// globalSetup and globalTeardown other than via the filesystem.

import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import type { FullConfig } from '@playwright/test';
import { startAuthEmulator } from './lib/emulator.js';

export const STATE_FILE = stateFilePath();

interface PersistedState {
  baseUrl: string;
  logPath: string;
  /** PID of the spawned server process; empty when reusing an external server. */
  serverPid: number;
  /** PID (process-group leader) of the Auth Emulator; absent when not started. */
  authEmulatorPid?: number;
  /** Per-run ephemeral DB name to drop in teardown; absent when not created. */
  ephemeralDbName?: string;
  /** Maintenance-DB URL used to drop the ephemeral DB. */
  ephemeralAdminUrl?: string;
}

export default async function globalSetup(config: FullConfig): Promise<void> {
  const preexistingUrl = process.env.E2E_BASE_URL;
  if (preexistingUrl) {
    persist({ baseUrl: preexistingUrl, logPath: '', serverPid: 0 });
    process.env.E2E_BASE_URL = preexistingUrl;
    console.log(`[e2e:setup] reusing external server at ${preexistingUrl}`);
    return;
  }

  const baseDbUrl = process.env.E2E_DB_URL;
  if (!baseDbUrl) {
    throw new Error(
      'globalSetup: set E2E_DB_URL (Postgres URL) or E2E_BASE_URL (existing server)',
    );
  }

  // Each worker starts its own server + ephemeral DB (lib/fixtures.ts). Publish
  // the base port so the fixture binds E2E_BASE_PORT + parallelIndex, and
  // deliberately leave E2E_BASE_URL UNSET — that's how the fixture knows to start
  // its own server rather than reuse an external one.
  const basePort = Number(process.env.E2E_PORT ?? '8080');
  process.env.E2E_BASE_PORT = String(basePort);
  const workers = Math.max(1, config.workers || 1);

  // Defensive: a prior run that didn't tear down cleanly can leave the Auth
  // Emulator (9099 + hub 4400) or a per-worker server bound, which would make
  // this run dial a stale process. Reap those ports before starting.
  await reapPort(9099);
  await reapPort(4400);
  for (let i = 0; i < workers; i++) {
    await reapPort(basePort + i);
  }

  // Boot the Firebase Auth Emulator once so phone OTP works through the web UI
  // deterministically; export its host so each worker's server (spawned with
  // ...process.env) validates emulator tokens.
  console.log('[e2e:setup] starting Firebase Auth Emulator…');
  const emulator = await startAuthEmulator();
  process.env.FIREBASE_AUTH_EMULATOR_HOST = emulator.host;
  console.log(
    `[e2e:setup] auth emulator ready at ${emulator.host} (pid=${emulator.pid}, log=${emulator.logPath})`,
  );

  persist({ baseUrl: '', logPath: '', serverPid: 0, authEmulatorPid: emulator.pid });
  console.log(
    `[e2e:setup] per-worker servers will bind ${basePort}..${basePort + workers - 1} ` +
      `(each its own database in the shared Postgres container)`,
  );
}

function persist(state: PersistedState): void {
  mkdirSync(stateDir(), { recursive: true });
  writeFileSync(STATE_FILE, JSON.stringify(state, null, 2));
}

/** reapPort SIGKILLs whatever is listening on the given TCP port. Best-effort:
 *  used to clear stale emulator/server processes a prior run leaked. No-op when
 *  nothing is bound. */
async function reapPort(port: number): Promise<void> {
  const { spawn } = require('node:child_process') as typeof import('node:child_process');
  await new Promise<void>((resolve) => {
    const p = spawn('sh', ['-c', `lsof -ti:${port} | xargs kill -9 2>/dev/null || true`]);
    p.on('exit', () => resolve());
    p.on('error', () => resolve());
  });
}

function stateDir(): string {
  return process.env.TMPDIR ?? '/tmp';
}

function stateFilePath(): string {
  return join(stateDir(), 'ripls-e2e-state.json');
}
