// e2e/lib/investigation-state.ts — persisted state for the standalone
// investigation environment (see scripts/investigate_env.ts and lib/probe.ts).
//
// An investigation is a slug-named workspace under e2e/investigations/<slug>/
// (gitignored) holding everything one UI investigation produces: the running
// environment's state file, server + emulator logs, probe scripts, screenshots,
// and console captures. The state file is what lets `stop` — and any probe
// script — find the environment from a fresh process.

import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';

export const INVESTIGATIONS_ROOT = resolve(__dirname, '..', 'investigations');

export interface InvestigationEnvState {
  slug: string;
  /** Server base URL probe scripts dial, e.g. http://localhost:8100. */
  baseUrl: string;
  /** PID of the Go server (start_e2e_server.sh execs the binary). */
  serverPid: number;
  serverLogPath: string;
  /** Ephemeral DB name; '' when psql was unavailable and we fell back. */
  dbName: string;
  /** URL the server connects to (the ephemeral DB). */
  dbUrl: string;
  /** Maintenance-DB URL used to drop the ephemeral DB on stop. */
  adminUrl: string;
  /** host:port of the Firebase Auth Emulator. */
  emulatorHost: string;
  /** Emulator CLI pid when this env started it; 0 when reusing one it found
   *  already running (not ours to kill on stop). */
  emulatorPid: number;
  emulatorLogPath: string;
  startedAt: string;
}

/** Workspace directory for a slug (created on demand). */
export function investigationDir(slug: string): string {
  const dir = join(INVESTIGATIONS_ROOT, slug);
  mkdirSync(dir, { recursive: true });
  return dir;
}

export function envStatePath(slug: string): string {
  return join(investigationDir(slug), 'env.json');
}

export function readEnvState(slug: string): InvestigationEnvState | undefined {
  const path = envStatePath(slug);
  if (!existsSync(path)) return undefined;
  return JSON.parse(readFileSync(path, 'utf-8')) as InvestigationEnvState;
}

export function writeEnvState(state: InvestigationEnvState): void {
  writeFileSync(envStatePath(state.slug), JSON.stringify(state, null, 2));
}
