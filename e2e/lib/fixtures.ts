// e2e/lib/fixtures.ts — the `test` object every spec imports, extended with a
// worker-scoped server so the suite can run in parallel (playwright.config.ts
// `workers` > 1).
//
// Why per-worker: all specs share one Auth Emulator (started once in
// global-setup.ts — see below) but they MUST NOT share a server + database, or
// two specs running concurrently would stomp each other's state (a shared home
// feed, overlapping communities, the phone-first specs' fixtures). So each
// Playwright worker gets:
//   - its own server, bound to BASE_PORT + parallelIndex (parallelIndex is the
//     bounded worker-slot id, 0..workers-1, reused when a worker restarts — so
//     ports never collide among concurrent workers and never grow unbounded);
//   - its own ephemeral database, created in the SHARED Postgres container (one
//     container, many DBs — cheap) and dropped when the worker exits.
//
// The worker publishes its server URL via `process.env.E2E_BASE_URL` (each
// worker is its own OS process, so this mutation is worker-local). That keeps
// each spec's existing `requireBaseUrl()` — which reads E2E_BASE_URL —
// transparently returning THIS worker's server, so specs only had to swap their
// `@playwright/test` import for this module.
//
// The Auth Emulator stays shared (one Java process on 9099): the web bundle is
// built once against a fixed Firebase project, so per-worker emulators would buy
// nothing. Its auth store is keyed by phone number; specs use distinct phone
// fixtures, so concurrent workers never collide there.

import { test as base } from '@playwright/test';
import { startTestServer, type TestServer } from './server.js';
import {
  createEphemeralDatabase,
  dropEphemeralDatabase,
} from './ephemeral-db.js';

// Re-export everything from @playwright/test (expect, devices, types, …) so a
// spec's only change is the import specifier. The explicit `test` below shadows
// the star-exported one.
export * from '@playwright/test';

/** First server port; worker i binds BASE_PORT + i. Matches global-setup. */
const BASE_PORT = Number(process.env.E2E_BASE_PORT ?? '8080');

interface WorkerFixtures {
  /** Auto-applied worker fixture that owns this worker's server + DB. */
  workerServer: void;
}

export const test = base.extend<object, WorkerFixtures>({
  workerServer: [
    async ({}, use, workerInfo) => {
      // External-server override (the local "leave the server up and iterate"
      // loop): global-setup set E2E_BASE_URL, so reuse it and start nothing.
      if (process.env.E2E_BASE_URL) {
        await use();
        return;
      }

      const baseDbUrl = process.env.E2E_DB_URL;
      if (!baseDbUrl) {
        throw new Error(
          'workerServer: neither E2E_BASE_URL nor E2E_DB_URL set; ' +
            'global-setup.ts is supposed to provide E2E_DB_URL',
        );
      }

      const port = BASE_PORT + workerInfo.parallelIndex;
      const eph = createEphemeralDatabase(baseDbUrl);

      let server: TestServer;
      try {
        server = await startTestServer({
          dbUrl: eph.url,
          port,
          logFormat: process.env.CI ? 'json' : 'text',
        });
      } catch (err) {
        // Don't leak the DB if the server fails to come up.
        dropEphemeralDatabase(eph.adminUrl, eph.dbName);
        throw err;
      }

      // Publish for this worker's specs (worker-local env mutation).
      process.env.E2E_BASE_URL = server.baseUrl;
      // The worker's own database, for the rare fixture that has to be written
      // directly rather than through an RPC — see seedLegacyPasswordUser.
      process.env.E2E_WORKER_DB_URL = eph.url;
      console.log(
        `[e2e:worker ${workerInfo.parallelIndex}] server ${server.baseUrl} ` +
          `(pid=${server.pid}, db=${eph.dbName || 'base'})`,
      );

      try {
        await use();
      } finally {
        await server.kill();
        dropEphemeralDatabase(eph.adminUrl, eph.dbName);
      }
    },
    { scope: 'worker', auto: true },
  ],
});
