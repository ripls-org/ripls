// e2e/global-teardown.ts — stop the test server started by
// global-setup.ts. Reads ${TMPDIR}/ripls-e2e-state.json to find the
// server PID and SIGTERMs it; SIGKILLs after 5 s if it doesn't
// surrender. Idempotent and no-op when the state file is missing
// (e.g., setup never ran or already cleaned up).

import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  rmSync,
} from 'node:fs';
import { join, relative } from 'node:path';
import { STATE_FILE } from './global-setup.js';
import { killEmulatorGroup } from './lib/emulator.js';
import { dropEphemeralDatabase } from './lib/ephemeral-db.js';

/**
 * Flatten every per-test video Playwright recorded this run into one browsable
 * directory (e2e/videos/), named after the test's output folder. With
 * `video: 'on'` each test records a clip (multi-context specs produce several);
 * by default they're buried under per-test test-results/ subfolders. The
 * collected directory is gitignored and rebuilt fresh each run so it always
 * holds just the latest run's clips.
 */
function collectVideos(): void {
  const src = join(__dirname, 'test-results');
  const dest = join(__dirname, 'videos');
  rmSync(dest, { recursive: true, force: true });
  if (!existsSync(src)) return;

  // Group every recorded .webm by its containing test folder (Playwright's
  // sanitized test title), so the collected name reads like the test.
  const byTest = new Map<string, string[]>();
  const walk = (dir: string): void => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const p = join(dir, entry.name);
      if (entry.isDirectory()) walk(p);
      else if (entry.name.endsWith('.webm')) {
        // Key by the video's relative folder (slashes → "__"), minus the noisy
        // "workflows-" path prefix and the "-<project>" suffix Playwright adds,
        // so the collected name reads like the scenario. Disambiguating the
        // project/path can be re-added if those ever vary (#2492 video legibility).
        const key = (relative(src, dir).replace(/[\\/]/g, '__') || 'video')
          .replace(/^workflows-/, '')
          .replace(/-(mobile-chromium|webkit|walkthrough)$/, '');
        byTest.set(key, [...(byTest.get(key) ?? []), p]);
      }
    }
  };
  walk(src);
  if (byTest.size === 0) return;

  mkdirSync(dest, { recursive: true });
  let count = 0;
  for (const [testDir, files] of byTest) {
    files.sort();
    // One clip → "<test>.webm"; multi-context specs → "<test>-1.webm", etc.
    files.forEach((f, i) => {
      const name = files.length === 1 ? `${testDir}.webm` : `${testDir}-${i + 1}.webm`;
      copyFileSync(f, join(dest, name));
      count++;
    });
  }
  console.log(`[e2e:teardown] collected ${count} video(s) → ${dest}`);
}

export default async function globalTeardown(): Promise<void> {
  // Always collect videos first — independent of whether a server was spawned.
  collectVideos();

  if (!existsSync(STATE_FILE)) return;
  const state = JSON.parse(readFileSync(STATE_FILE, 'utf-8')) as {
    serverPid?: number;
    baseUrl?: string;
    logPath?: string;
    authEmulatorPid?: number;
    ephemeralDbName?: string;
    ephemeralAdminUrl?: string;
  };
  if (state.serverPid && state.serverPid > 0) {
    console.log(`[e2e:teardown] stopping server pid=${state.serverPid}`);
    try {
      process.kill(state.serverPid, 'SIGTERM');
    } catch {
      // Already gone.
    }
    // Escalate after 5 s if still alive.
    const deadline = Date.now() + 5_000;
    while (Date.now() < deadline) {
      try {
        process.kill(state.serverPid, 0); // probe
      } catch {
        break;
      }
      await new Promise((r) => setTimeout(r, 200));
    }
    try {
      process.kill(state.serverPid, 'SIGKILL');
    } catch {
      // Already gone.
    }
  } else if (state.baseUrl) {
    console.log(`[e2e:teardown] reused external server at ${state.baseUrl}; leaving it running`);
  }
  if (state.authEmulatorPid && state.authEmulatorPid > 0) {
    console.log(`[e2e:teardown] stopping auth emulator group pid=${state.authEmulatorPid}`);
    await killEmulatorGroup(state.authEmulatorPid);
  }
  // Drop the per-run ephemeral DB now the server (its only client) is stopped.
  if (state.ephemeralAdminUrl && state.ephemeralDbName) {
    dropEphemeralDatabase(state.ephemeralAdminUrl, state.ephemeralDbName);
  }
  rmSync(STATE_FILE, { force: true });
}
