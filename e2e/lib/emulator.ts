// e2e/lib/emulator.ts — start/stop the Firebase Auth Emulator for an e2e run.
//
// The phone-first pivot e2e (WEB-2) drives real Firebase phone OTP through the
// web UI. To keep that hermetic and deterministic we run the Firebase Auth
// Emulator: the Flutter web client points at it (useAuthEmulator) and the Go
// server validates its tokens (FIREBASE_AUTH_EMULATOR_HOST — see
// scripts/start_e2e_server.sh + server/auth/firebase_emulator_test.go). No real
// Firebase project, no reCAPTCHA, no secrets, deterministic verification codes.
//
// firebase-tools is pinned as a devDependency (15.x supports node 24/25; the
// older CLI dies on node 24's SlowBuffer removal). `firebase.json` at the repo
// root configures the auth emulator on 127.0.0.1:9099.

import { spawn, type ChildProcess } from 'node:child_process';
import { closeSync, mkdirSync, openSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';

export const AUTH_EMULATOR_HOST = '127.0.0.1:9099';
export const EMULATOR_PROJECT = 'demo-ripls';

const REPO_ROOT = resolve(__dirname, '..', '..');
const E2E_ROOT = resolve(__dirname, '..');
const POLL_INTERVAL_MS = 500;

export interface AuthEmulator {
  /** Value for FIREBASE_AUTH_EMULATOR_HOST (host:port, no scheme). */
  host: string;
  /** PID of the detached CLI (process-group leader); persist it so a separate
   *  teardown process can reap the whole group (CLI + Java + hub). */
  pid: number;
  /** Absolute path to the captured emulator log. */
  logPath: string;
  /** Stop the emulator process group. Idempotent. */
  kill: () => Promise<void>;
}

export interface StartAuthEmulatorOptions {
  /** How long to wait for the emulator to answer. Default 90 s (first run downloads the JAR). */
  readyTimeoutMs?: number;
  logPath?: string;
}

export async function startAuthEmulator(opts: StartAuthEmulatorOptions = {}): Promise<AuthEmulator> {
  const readyTimeoutMs = opts.readyTimeoutMs ?? 90_000;
  const logPath = opts.logPath ?? defaultLogPath();
  mkdirSync(dirname(logPath), { recursive: true });

  // Resolve the pinned firebase CLI from e2e's node_modules.
  const firebaseBin = join(E2E_ROOT, 'node_modules', '.bin', 'firebase');

  // Hand the child its own fd for stdout/stderr rather than piping through
  // this process. A parent-owned pipe stays ref'd on the event loop even
  // after child.unref(), so investigate_env.ts (which is expected to return
  // once state is written) would sit blocked reading emulator output for the
  // life of the emulator — the exact hang tracked by #2772. Matches the
  // detached-log pattern in server.ts.
  const logFd = openSync(logPath, 'a');

  // detached so the emulator survives this process and can be reaped later
  // by pid (env:stop / globalTeardown); the CLI spawns the Java emulator +
  // hub as children of its own process group.
  const child = spawn(
    firebaseBin,
    ['emulators:start', '--only', 'auth', '--project', EMULATOR_PROJECT],
    {
      cwd: REPO_ROOT,
      env: { ...process.env },
      stdio: ['ignore', logFd, logFd],
      detached: true,
    },
  );
  // The child holds its own copy of the descriptor; drop ours so it doesn't
  // keep the parent's event loop alive.
  closeSync(logFd);
  child.unref();

  const earlyExit = new Promise<never>((_, reject) => {
    child.on('exit', (code, signal) => {
      reject(
        new Error(
          `auth emulator exited before ready (code=${code} signal=${signal}); see ${logPath}`,
        ),
      );
    });
  });

  try {
    await Promise.race([waitForEmulator(readyTimeoutMs), earlyExit]);
  } catch (err) {
    await killGroup(child);
    throw err;
  }

  return {
    host: AUTH_EMULATOR_HOST,
    pid: child.pid ?? 0,
    logPath,
    kill: () => killGroup(child),
  };
}

/** killEmulatorGroup reaps the emulator by the firebase-CLI pid, from a process
 *  that did not spawn it (e.g. globalTeardown). A positive-pid SIGTERM lets the
 *  CLI shut its Java emulator + hub down gracefully; we never signal the process
 *  GROUP (-pid), which on some platforms can reach the runner itself. SIGKILL
 *  the pid as a last resort. */
export async function killEmulatorGroup(pid: number): Promise<void> {
  if (!pid || pid <= 0) return;
  try {
    process.kill(pid, 'SIGTERM');
  } catch {
    return; // already gone
  }
  const deadline = Date.now() + 8_000;
  while (Date.now() < deadline) {
    try {
      process.kill(pid, 0); // probe the CLI pid
    } catch {
      return; // gone (CLI exited after reaping its children)
    }
    await sleep(200);
  }
  try {
    process.kill(pid, 'SIGKILL');
  } catch {
    // gone
  }
}

async function waitForEmulator(timeoutMs: number): Promise<void> {
  // The auth emulator root returns 200 with {"authEmulator":{"ready":true}}.
  const url = `http://${AUTH_EMULATOR_HOST}/`;
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const resp = await fetch(url);
      if (resp.ok) return;
    } catch {
      // Not up yet; keep polling.
    }
    await sleep(POLL_INTERVAL_MS);
  }
  throw new Error(`auth emulator did not answer at ${url} within ${timeoutMs} ms`);
}

async function killGroup(child: ChildProcess): Promise<void> {
  if (child.pid === undefined || child.exitCode !== null || child.signalCode !== null) return;
  const exited = new Promise<void>((resolve) => child.once('exit', () => resolve()));
  // SIGTERM the CLI directly so it shuts the emulators down gracefully; never
  // signal the process group (-pid), which can reach the runner.
  child.kill('SIGTERM');
  const escalate = setTimeout(() => {
    child.kill('SIGKILL');
  }, 8_000);
  try {
    await exited;
  } finally {
    clearTimeout(escalate);
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function defaultLogPath(): string {
  const tmp = process.env.TMPDIR ?? '/tmp';
  const ts = new Date().toISOString().replace(/[:.]/g, '-');
  return join(tmp, `ripls-e2e-auth-emulator-${ts}.log`);
}
