// e2e/lib/probe.ts — session helper for throwaway UI-investigation scripts
// (the ripls-ui-investigate skill; environment from scripts/investigate_env.ts).
//
// A probe is a small `npx tsx` script that seeds state via lib/seed/*, drives
// the Flutter Web bundle in a real Chromium, and dumps evidence an agent can
// read back: numbered screenshots, ARIA snapshots (the accessibility tree is
// the only text channel CanvasKit exposes — see README.md § What's reliably
// reachable), a console/pageerror/failed-RPC capture, and the server log
// (greppable by the probe's request-ID prefix `e2e-{slug}-{role}-…`).
//
// Probes are investigation tools, not specs: they may freely mix UI gestures
// and direct RPCs — that contrast is exactly how you bisect client-vs-server
// (e.g. double-tap a button in the UI, then call the same RPC twice directly,
// and compare resulting server state). The "state changes happen through the
// UI" rule binds tests/, not investigations/.

import { chromium, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { appendFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { injectAuth } from './auth.js';
import { installRequestIdOverride } from './connect.js';
import { investigationDir, readEnvState } from './investigation-state.js';

export interface ProbeUser {
  userId: string;
  name: string;
  accessToken: string;
  refreshToken: string;
  /** Optional avatar media id; '' when absent. */
  mediaId?: string;
}

export interface OpenProbeOptions {
  /** Investigation slug — selects investigations/<slug>/ for env + artifacts. */
  slug: string;
  /**
   * Which user this browser session is, e.g. 'borrower', 'owner'. Names the
   * artifact files and the request-ID prefix. Default 'probe'.
   */
  role?: string;
  /** Signed-in user (a lib/seed SeededUser works as-is). Omit for signed-out. */
  user?: ProbeUser;
  /** Override the server URL; default from investigations/<slug>/env.json. */
  baseUrl?: string;
  /** Default 390×844 phone viewport at 2× (crisp CanvasKit text in shots). */
  viewport?: { width: number; height: number };
  /**
   * Emulated OS colour scheme. `themeModeProvider` defaults to
   * `ThemeMode.system`, so this is what selects the app's light or dark theme
   * (see app/lib/services/providers/app_providers.dart).
   */
  colorScheme?: 'light' | 'dark';
  /** Show the browser window (debugging with a human watching). */
  headed?: boolean;
  /**
   * Reuse an already-launched browser instead of starting one.
   *
   * A launch costs a couple of seconds, which is irrelevant for a handful of
   * probes but dominates a sweep: the contrast gate takes hundreds of captures,
   * where per-capture launches turn ~20 minutes of rendering into hours. When
   * supplied, `close()` disposes only this session's context and leaves the
   * browser to the caller.
   */
  browser?: Browser;
}

export interface Probe {
  browser: Browser;
  context: BrowserContext;
  page: Page;
  baseUrl: string;
  /** This investigation's workspace dir (investigations/<slug>/). */
  dir: string;
  /**
   * Screenshot to shots/<role>-NN-<name>.png (auto-numbered). Returns the
   * absolute path so the caller can print it for the agent to read.
   */
  snap(name: string): Promise<string>;
  /**
   * ARIA snapshot of the page (YAML). Saved to logs/<role>-NN-<name>.aria.yaml
   * and returned — the greppable text view of what CanvasKit rendered.
   */
  aria(name?: string): Promise<string>;
  close(): Promise<void>;
}

/**
 * Launches a Chromium configured the way probes and captures need it.
 *
 * The 2x device-scale flag is a LAUNCH argument, not a context option: without
 * it CanvasKit rasterises text at 1x and screenshots are too soft to judge — or
 * to measure.
 */
export async function launchProbeBrowser(
  opts: { headed?: boolean } = {},
): Promise<Browser> {
  return chromium.launch({
    headless: !opts.headed,
    args: ['--force-device-scale-factor=2', '--high-dpi-support=1'],
  });
}

export async function openProbe(opts: OpenProbeOptions): Promise<Probe> {
  const role = opts.role ?? 'probe';
  const dir = investigationDir(opts.slug);
  const baseUrl =
    opts.baseUrl ?? readEnvState(opts.slug)?.baseUrl ?? process.env.E2E_BASE_URL;
  if (!baseUrl) {
    throw new Error(
      `openProbe: no baseUrl — start the env first (npm run env:start -- --slug ${opts.slug})`,
    );
  }

  const shotsDir = join(dir, 'shots');
  const logsDir = join(dir, 'logs');
  mkdirSync(shotsDir, { recursive: true });
  mkdirSync(logsDir, { recursive: true });
  const consoleLog = join(logsDir, `${role}-console.log`);

  const ownsBrowser = !opts.browser;
  const browser = opts.browser ?? (await launchProbeBrowser({ headed: opts.headed }));
  const context = await browser.newContext({
    viewport: opts.viewport ?? { width: 390, height: 844 },
    deviceScaleFactor: 2,
    hasTouch: true,
    ...(opts.colorScheme ? { colorScheme: opts.colorScheme } : {}),
  });

  if (opts.user) {
    await injectAuth(context, {
      accessToken: opts.user.accessToken,
      refreshToken: opts.user.refreshToken,
      user: {
        id: opts.user.userId,
        name: opts.user.name,
        mediaId: opts.user.mediaId ?? '',
      },
      serverUrl: baseUrl,
    });
  }

  const page = await context.newPage();
  await installRequestIdOverride(page, `${opts.slug}-${role}`);

  // Evidence channels: console, uncaught page errors, failed requests, and
  // non-2xx RPC responses, all timestamped into one per-role capture file.
  const logLine = (kind: string, text: string) => {
    appendFileSync(consoleLog, `${new Date().toISOString()} [${kind}] ${text}\n`);
  };
  page.on('console', (msg) => logLine(`console.${msg.type()}`, msg.text()));
  page.on('pageerror', (err) => logLine('pageerror', err.message));
  page.on('requestfailed', (req) =>
    logLine('requestfailed', `${req.method()} ${req.url()} — ${req.failure()?.errorText}`),
  );
  page.on('response', (resp) => {
    if (resp.status() >= 400 && /\/ripls\.api\./.test(resp.url())) {
      logLine('rpc-error', `${resp.status()} ${resp.url()}`);
    }
  });

  let shotSeq = 0;
  let ariaSeq = 0;

  return {
    browser,
    context,
    page,
    baseUrl,
    dir,
    async snap(name: string): Promise<string> {
      const path = join(shotsDir, `${role}-${pad(++shotSeq)}-${name}.png`);
      await page.screenshot({ path });
      console.log(`[probe:${role}] shot: ${path}`);
      return path;
    },
    async aria(name = 'page'): Promise<string> {
      const yaml = await page.locator('body').ariaSnapshot();
      const path = join(logsDir, `${role}-${pad(++ariaSeq)}-${name}.aria.yaml`);
      appendFileSync(path, yaml + '\n');
      console.log(`[probe:${role}] aria: ${path}`);
      return yaml;
    },
    async close(): Promise<void> {
      await context.close();
      // A borrowed browser stays open — the caller owns its lifetime.
      if (ownsBrowser) await browser.close();
    },
  };
}

function pad(n: number): string {
  return String(n).padStart(2, '0');
}
