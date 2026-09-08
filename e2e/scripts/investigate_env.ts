// e2e/scripts/investigate_env.ts — standalone hermetic environment for
// agent-driven UI investigations (the ripls-ui-investigate skill).
//
// `start` boots the same stack a Playwright worker gets — Firebase Auth
// Emulator + ephemeral Postgres database + Ripls server — but DETACHED, so
// the stack outlives this process and stays up while an investigator iterates
// on probe scripts against it (lib/probe.ts). State (pids, DB, URLs, logs) is
// persisted to investigations/<slug>/env.json so `stop` reaps everything from
// a fresh process. Logs land in the same per-slug workspace.
//
// Usage (from e2e/):
//   npm run env:start  -- --slug 2638 [--port 8100]
//   npm run env:status -- --slug 2638
//   npm run env:stop   -- --slug 2638
//
// Prereqs are the same as the test suite (see README.md § Running locally):
// Postgres up (`npm run db:start` at the repo root), web bundle built
// (build:web:e2e) and the server binary REBUILT AFTER the bundle
// (go build -o tmp/server ./server — the bundle is embedded at Go build time).
//
// The emulator is shared infrastructure (fixed ports 9099/4400): if one is
// already answering — e.g. a concurrent `npm test` run started it — this env
// reuses it and records emulatorPid=0 so `stop` leaves it alone.

import { unlinkSync } from 'node:fs';
import { join } from 'node:path';
import { AUTH_EMULATOR_HOST, startAuthEmulator, killEmulatorGroup } from '../lib/emulator.js';
import { createEphemeralDatabase, dropEphemeralDatabase } from '../lib/ephemeral-db.js';
import { startTestServer } from '../lib/server.js';
import {
  envStatePath,
  investigationDir,
  readEnvState,
  writeEnvState,
  type InvestigationEnvState,
} from '../lib/investigation-state.js';

// Clear of the Playwright workers (8080..808N) and the README's local-run
// examples (8090), so a running investigation never collides with `npm test`.
const DEFAULT_PORT = 8100;
const DEFAULT_DB_URL =
  'postgres://ripls:ripls_dev@localhost:5432/ripls_e2e_proof?sslmode=disable';

async function main(): Promise<void> {
  const [command] = process.argv.slice(2);
  const slug = argValue('--slug') ?? 'scratch';

  switch (command) {
    case 'start':
      await start(slug, Number(argValue('--port') ?? DEFAULT_PORT));
      return;
    case 'stop':
      await stop(slug);
      return;
    case 'status':
      await status(slug);
      return;
    default:
      console.error(
        'usage: investigate_env.ts start|stop|status --slug <slug> [--port <port>]',
      );
      process.exit(2);
  }
}

async function start(slug: string, port: number): Promise<void> {
  // Idempotent: a healthy env for this slug just gets reported.
  const existing = readEnvState(slug);
  if (existing) {
    if (await healthy(existing.baseUrl)) {
      console.log(`investigation env "${slug}" already running`);
      printState(existing);
      return;
    }
    console.log(`stale env.json for "${slug}" (server not answering); cleaning up…`);
    await reapState(existing);
  }

  const dir = investigationDir(slug);
  const baseDbUrl = process.env.E2E_DB_URL ?? DEFAULT_DB_URL;

  // Emulator: reuse a live one (a concurrent test run's, or a previous
  // investigation's), else start our own — detached, reaped on stop.
  let emulatorPid = 0;
  let emulatorLogPath = '';
  if (await emulatorAnswering()) {
    console.log(`reusing Auth Emulator already at ${AUTH_EMULATOR_HOST}`);
  } else {
    console.log('starting Firebase Auth Emulator…');
    const emulator = await startAuthEmulator({
      logPath: join(dir, 'auth-emulator.log'),
    });
    emulatorPid = emulator.pid;
    emulatorLogPath = emulator.logPath;
  }
  process.env.FIREBASE_AUTH_EMULATOR_HOST = AUTH_EMULATOR_HOST;

  const eph = createEphemeralDatabase(baseDbUrl);
  console.log(`ephemeral DB: ${eph.dbName || '(fallback to base DB — psql unavailable?)'}`);

  console.log(`starting server on :${port}…`);
  let server;
  try {
    server = await startTestServer({
      dbUrl: eph.url,
      port,
      logFormat: 'text',
      logPath: join(dir, 'server.log'),
      detached: true,
    });
  } catch (err) {
    dropEphemeralDatabase(eph.adminUrl, eph.dbName);
    if (emulatorPid) await killEmulatorGroup(emulatorPid);
    throw err;
  }

  const state: InvestigationEnvState = {
    slug,
    baseUrl: server.baseUrl,
    serverPid: server.pid,
    serverLogPath: server.logPath,
    dbName: eph.dbName,
    dbUrl: eph.url,
    adminUrl: eph.adminUrl,
    emulatorHost: AUTH_EMULATOR_HOST,
    emulatorPid,
    emulatorLogPath,
    startedAt: new Date().toISOString(),
  };
  writeEnvState(state);
  console.log(`investigation env "${slug}" ready`);
  printState(state);
}

async function stop(slug: string): Promise<void> {
  const state = readEnvState(slug);
  if (!state) {
    console.log(`no env.json for "${slug}" — nothing to stop`);
    return;
  }
  await reapState(state);
  unlinkSync(envStatePath(slug));
  console.log(`investigation env "${slug}" stopped (logs/shots kept in investigations/${slug}/)`);
}

async function status(slug: string): Promise<void> {
  const state = readEnvState(slug);
  if (!state) {
    console.log(`no env.json for "${slug}"`);
    process.exitCode = 1;
    return;
  }
  const up = await healthy(state.baseUrl);
  console.log(`investigation env "${slug}": server ${up ? 'UP' : 'DOWN'}`);
  printState(state);
  if (!up) process.exitCode = 1;
}

async function reapState(state: InvestigationEnvState): Promise<void> {
  await killPid(state.serverPid);
  if (state.emulatorPid) await killEmulatorGroup(state.emulatorPid);
  dropEphemeralDatabase(state.adminUrl, state.dbName);
}

/** SIGTERM → grace → SIGKILL, from a process that didn't spawn the target. */
async function killPid(pid: number): Promise<void> {
  if (!pid || pid <= 0) return;
  try {
    process.kill(pid, 'SIGTERM');
  } catch {
    return; // already gone
  }
  const deadline = Date.now() + 5_000;
  while (Date.now() < deadline) {
    try {
      process.kill(pid, 0);
    } catch {
      return;
    }
    await sleep(200);
  }
  try {
    process.kill(pid, 'SIGKILL');
  } catch {
    // gone
  }
}

async function healthy(baseUrl: string): Promise<boolean> {
  try {
    const resp = await fetch(`${baseUrl}/health`, { signal: AbortSignal.timeout(2_000) });
    return resp.ok;
  } catch {
    return false;
  }
}

async function emulatorAnswering(): Promise<boolean> {
  try {
    const resp = await fetch(`http://${AUTH_EMULATOR_HOST}/`, {
      signal: AbortSignal.timeout(2_000),
    });
    return resp.ok;
  } catch {
    return false;
  }
}

function printState(s: InvestigationEnvState): void {
  console.log(`  server:    ${s.baseUrl} (pid ${s.serverPid})`);
  console.log(`  serverLog: ${s.serverLogPath}`);
  console.log(`  db:        ${s.dbName || '(base DB)'}`);
  console.log(
    `  emulator:  ${s.emulatorHost} ${s.emulatorPid ? `(pid ${s.emulatorPid}, owned)` : '(reused)'}`,
  );
  console.log(`  state:     ${envStatePath(s.slug)}`);
  console.log('');
  console.log(`  export E2E_BASE_URL=${s.baseUrl}`);
}

function argValue(flag: string): string | undefined {
  const args = process.argv.slice(2);
  const idx = args.indexOf(flag);
  if (idx !== -1 && idx + 1 < args.length) return args[idx + 1];
  const prefixed = args.find((a) => a.startsWith(`${flag}=`));
  return prefixed?.slice(flag.length + 1);
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

main().catch((err) => {
  console.error(err instanceof Error ? err.message : err);
  process.exit(1);
});
