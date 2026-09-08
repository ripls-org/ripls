// e2e/lib/ephemeral-db.ts — make each `npx playwright test` run hermetic by
// giving it its OWN Postgres database, created in global-setup and dropped in
// global-teardown.
//
// Without this, the fixed local DB (ripls_e2e_proof) persists across runs, so
// reused fixtures collide on re-run — e.g. the phone-first specs use fixed phone
// numbers, so a second run's PhoneRegister hits AlreadyExists and the flow
// stalls. CI already provisions a throwaway container per run
// (server/cmd/web-smoke-db), so this is belt-and-braces there.
//
// Best-effort: if psql is unavailable or CREATE fails, falls back to the base
// URL unchanged (the run still works, just non-hermetic) so this never breaks a
// runner that lacks psql.

import { spawnSync } from 'node:child_process';

export interface EphemeralDb {
  /** URL the server connects to — the ephemeral DB, or the base on fallback. */
  url: string;
  /** Admin (maintenance-DB) URL used to DROP the ephemeral DB in teardown. */
  adminUrl: string;
  /** Name of the ephemeral DB, or '' when we fell back to the base DB. */
  dbName: string;
}

function withDatabase(baseUrl: string, dbName: string): string {
  const u = new URL(baseUrl);
  u.pathname = `/${dbName}`;
  return u.toString();
}

/**
 * Create a uniquely-named database on the same server as `baseUrl` (connecting
 * to the `postgres` maintenance DB to issue CREATE). The server builds its
 * schema on startup, so an empty database is all that's needed.
 */
export function createEphemeralDatabase(baseUrl: string): EphemeralDb {
  const adminUrl = withDatabase(baseUrl, 'postgres');
  const baseName = (
    new URL(baseUrl).pathname.replace(/^\//, '') || 'ripls_e2e'
  ).toLowerCase();
  const rand = Math.random().toString(36).slice(2, 8);
  const dbName = `${baseName}_${Date.now()}_${rand}`
    .replace(/[^a-z0-9_]/g, '_')
    .slice(0, 60);

  const r = spawnSync(
    'psql',
    [adminUrl, '-v', 'ON_ERROR_STOP=1', '-c', `CREATE DATABASE "${dbName}"`],
    { encoding: 'utf-8' },
  );
  if (r.status !== 0) {
    console.warn(
      `[e2e:setup] ephemeral DB unavailable (psql exit=${r.status ?? r.error?.message}); ` +
        `using the base DB (non-hermetic). stderr: ${(r.stderr ?? '').trim()}`,
    );
    return { url: baseUrl, adminUrl, dbName: '' };
  }
  return { url: withDatabase(baseUrl, dbName), adminUrl, dbName };
}

/** Drop a database created by createEphemeralDatabase. No-op when dbName is ''. */
export function dropEphemeralDatabase(adminUrl: string, dbName: string): void {
  if (!dbName) return;
  const r = spawnSync(
    'psql',
    [adminUrl, '-c', `DROP DATABASE IF EXISTS "${dbName}" WITH (FORCE)`],
    { encoding: 'utf-8' },
  );
  if (r.status !== 0) {
    console.warn(
      `[e2e:teardown] failed to drop ephemeral DB ${dbName}: ${(r.stderr ?? '').trim()}`,
    );
  } else {
    console.log(`[e2e:teardown] dropped ephemeral DB ${dbName}`);
  }
}
