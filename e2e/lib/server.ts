// e2e/lib/server.ts — start/stop the Ripls server for an e2e run.
//
// Owns no startup flags. Shells out to scripts/start_e2e_server.sh
// which owns the flag set. This file is the TypeScript-side
// lifecycle: spawn, wait for /health, return a handle, kill on
// teardown.
//
// Why a shell helper instead of inlining the flags here: when the
// server adds a required flag, every duplicated invocation drifts.
// scripts/start_e2e_server.sh is the single point of truth.

import { spawn, execFileSync, type ChildProcess } from 'node:child_process';
import { createServer } from 'node:net';
import {
  closeSync,
  createWriteStream,
  existsSync,
  mkdirSync,
  openSync,
  readdirSync,
  statSync,
} from 'node:fs';
import { dirname, join, resolve } from 'node:path';

export interface TestServer {
  /** Base URL the Playwright browser dials, e.g. http://localhost:8080. */
  baseUrl: string;
  /** PID of the spawned server process. start_e2e_server.sh `exec`s the binary,
   *  so this is the Go server's pid — the one teardown must kill. (Deriving it
   *  via `lsof -ti:PORT` is unsafe: the runner's own /health connections to the
   *  port make lsof return the runner's pid → teardown kills itself.) */
  pid: number;
  /** Absolute path to the captured server log (slog stream). */
  logPath: string;
  /** Stop the server process. Idempotent. */
  kill: () => Promise<void>;
}

export interface StartTestServerOptions {
  /** Postgres connection URL (testcontainer or local). */
  dbUrl: string;
  /** Port to bind. Default 8080. */
  port?: number;
  /**
   * Log format. CI uses 'json' so triagers can grep structured
   * fields (request_id, operation, …). Local dev uses 'text'.
   */
  logFormat?: 'json' | 'text';
  /** Log level. Default 'info'. */
  logLevel?: 'debug' | 'info' | 'warn' | 'error';
  /**
   * Where to write the captured server log. Default
   * `${process.env.TMPDIR ?? '/tmp'}/ripls-e2e-server-${ts}.log`.
   */
  logPath?: string;
  /**
   * Path to the server binary. Default 'tmp/server' (relative to
   * repo root); caller is responsible for building it.
   */
  serverBin?: string;
  /** How long to wait for /health to come up. Default 30 s. */
  readyTimeoutMs?: number;
  /**
   * Spawn the server detached, with its log written via an inherited
   * file descriptor instead of a pipe through this process — so the
   * server survives this process exiting. Used by the standalone
   * investigation environment (scripts/investigate_env.ts), which
   * starts the stack and exits, leaving `stop` to reap by pid. The
   * default (false) keeps the Playwright-fixture behaviour: the
   * server's lifetime is tied to the runner.
   */
  detached?: boolean;
}

const REPO_ROOT = resolve(__dirname, '..', '..');
const HEALTH_POLL_INTERVAL_MS = 500;

/**
 * Fail fast when the server binary is missing or OLDER than the web bundle.
 *
 * The Flutter Web bundle is EMBEDDED in the server binary at Go build time
 * (`//go:embed app_assets` in server/services/web/app_embed.go), so rebuilding
 * the bundle changes nothing served until the binary is rebuilt — the classic
 * symptom is the app silently behaving like an older build. `build:web:e2e`
 * copies the bundle into server/services/web/app_assets/, so "any asset newer
 * than the binary" means the binary embeds a stale app. One command rebuilds
 * both: `npm run build:e2e`.
 */
function assertServerBinaryFresh(serverBin: string): void {
  const binPath = resolve(REPO_ROOT, serverBin);
  if (!existsSync(binPath)) {
    throw new Error(`${serverBin} not found — build it with: npm run build:e2e`);
  }
  const assetsDir = resolve(REPO_ROOT, 'server', 'services', 'web', 'app_assets');
  if (!existsSync(assetsDir)) return;
  const binMtimeMs = statSync(binPath).mtimeMs;
  for (const entry of readdirSync(assetsDir)) {
    if (statSync(join(assetsDir, entry)).mtimeMs > binMtimeMs) {
      throw new Error(
        `${serverBin} is OLDER than the web bundle in server/services/web/app_assets ` +
          '— the bundle is embedded at Go build time, so this server would serve a ' +
          'STALE app. Rebuild with: npm run build:e2e ' +
          '(or just: go build -o tmp/server ./server)',
      );
    }
  }
}

/**
 * Fail fast when something else already owns the port.
 *
 * Without this the failure is silent and expensive. `waitForHealth` races the
 * spawned child's exit, and an orphan server left behind by a killed run
 * answers /health on the very first poll — so the race is won by the orphan,
 * startup reports "ready", and every request in the run is served by whatever
 * binary that orphan embeds. That is indistinguishable from a stale bundle:
 * source edits and rebuilds appear to have no effect, because they genuinely
 * have none on the process actually answering.
 *
 * The binary-freshness guard above cannot catch this — that binary is never
 * executed. Binding the port ourselves is deterministic where the health race
 * is not: if the bind succeeds the port was genuinely free.
 */
export async function assertPortFree(port: number): Promise<void> {
  if (!(await isPortInUse(port))) return;
  throw new Error(
    `port ${port} is already in use — refusing to start.\n` +
      'A server already answering here will win the /health race and silently\n' +
      'serve THIS ENTIRE RUN, so rebuilds would appear to have no effect.\n' +
      `Occupant:\n${describeOccupant(port)}\n` +
      `Stop it first (npm run env:stop -- --slug <slug>), or kill the pid above.`,
  );
}

/**
 * Whether something is already listening on [port], decided by trying to bind
 * it.
 *
 * This used to ask `lsof`, and treated ANY failure of that call as "the port is
 * free" — including `lsof` not being installed. On a machine without it the
 * guard therefore did nothing at all, silently, which is precisely the failure
 * it exists to prevent; the CI container is such a machine. Binding needs no
 * external tool and answers the actual question: if the bind succeeds, the port
 * was genuinely free.
 *
 * Binds 127.0.0.1 specifically, because that is the interface `waitForHealth`
 * polls. A listener on 0.0.0.0 collides with it too, so both cases are caught.
 */
function isPortInUse(port: number): Promise<boolean> {
  return new Promise((resolve, reject) => {
    const probe = createServer();
    probe.once('error', (err: NodeJS.ErrnoException) => {
      if (err.code === 'EADDRINUSE' || err.code === 'EACCES') {
        resolve(true);
        return;
      }
      reject(err);
    });
    probe.listen(port, '127.0.0.1', () => probe.close(() => resolve(false)));
  });
}

/**
 * Best-effort description of what holds the port. Decoration only — occupancy
 * has already been decided by [isPortInUse], so every failure here degrades the
 * message rather than the verdict.
 */
function describeOccupant(port: number): string {
  let pids: string[];
  try {
    pids = execFileSync('lsof', ['-nP', `-iTCP:${port}`, '-sTCP:LISTEN', '-t'], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    })
      .trim()
      .split('\n')
      .filter(Boolean);
  } catch {
    return '  (lsof unavailable or found nothing — cannot name the process)';
  }
  if (pids.length === 0) return '  (no listening pid reported)';
  return pids
    .map((pid) => {
      try {
        const desc = execFileSync('ps', ['-o', 'pid=,lstart=,command=', '-p', pid], {
          encoding: 'utf8',
          stdio: ['ignore', 'pipe', 'ignore'],
        }).trim();
        return `  ${desc.slice(0, 200)}`;
      } catch {
        return `  pid ${pid}`;
      }
    })
    .join('\n');
}

export async function startTestServer(opts: StartTestServerOptions): Promise<TestServer> {
  const port = opts.port ?? 8080;
  const logFormat = opts.logFormat ?? (process.env.CI ? 'json' : 'text');
  const logLevel = opts.logLevel ?? 'info';
  const logPath = opts.logPath ?? defaultLogPath();
  const serverBin = opts.serverBin ?? 'tmp/server';
  const readyTimeoutMs = opts.readyTimeoutMs ?? 30_000;

  assertServerBinaryFresh(serverBin);
  await assertPortFree(port);

  mkdirSync(dirname(logPath), { recursive: true });

  // Detached mode writes the log through an fd the child owns outright;
  // pipe mode streams through this process (and dies with it).
  const logFd = opts.detached ? openSync(logPath, 'a') : undefined;
  const child = spawn(
    join(REPO_ROOT, 'scripts/start_e2e_server.sh'),
    [`--log-format=${logFormat}`],
    {
      cwd: REPO_ROOT,
      env: {
        ...process.env,
        E2E_DB_URL: opts.dbUrl,
        E2E_PORT: String(port),
        E2E_SERVER_BIN: serverBin,
        E2E_LOG_LEVEL: logLevel,
      },
      stdio: logFd !== undefined ? ['ignore', logFd, logFd] : ['ignore', 'pipe', 'pipe'],
      detached: opts.detached ?? false,
    },
  );

  if (logFd !== undefined) {
    // The child holds its own copy of the descriptor.
    closeSync(logFd);
    child.unref();
  } else {
    const logStream = createWriteStream(logPath, { flags: 'a' });
    child.stdout!.pipe(logStream);
    child.stderr!.pipe(logStream);
  }

  const baseUrl = `http://localhost:${port}`;
  const earlyExit = new Promise<never>((_, reject) => {
    child.on('exit', (code, signal) => {
      reject(
        new Error(
          `server exited before /health (code=${code} signal=${signal}); see ${logPath}`,
        ),
      );
    });
  });

  try {
    await Promise.race([
      waitForHealth(baseUrl, readyTimeoutMs),
      earlyExit,
    ]);
  } catch (err) {
    await killChild(child);
    throw err;
  }

  return {
    baseUrl,
    pid: child.pid ?? 0,
    logPath,
    kill: () => killChild(child),
  };
}

async function waitForHealth(baseUrl: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const resp = await fetch(`${baseUrl}/health`);
      if (resp.ok) return;
    } catch {
      // Server not up yet; keep polling.
    }
    await sleep(HEALTH_POLL_INTERVAL_MS);
  }
  throw new Error(`server /health did not respond within ${timeoutMs} ms`);
}

async function killChild(child: ChildProcess): Promise<void> {
  if (child.exitCode !== null || child.signalCode !== null) return;
  const exited = new Promise<void>((resolve) => child.once('exit', () => resolve()));
  child.kill('SIGTERM');
  // Hard kill after a grace period if SIGTERM didn't take.
  const escalate = setTimeout(() => child.kill('SIGKILL'), 5_000);
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
  return join(tmp, `ripls-e2e-server-${ts}.log`);
}
