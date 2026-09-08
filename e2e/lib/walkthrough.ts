// e2e/lib/walkthrough.ts — the scene toolkit for @walkthrough user-journey reels (#2684).
//
// A reel is one spec file under tests/walkthroughs/ whose tests are SCENES: each
// scene films exactly ONE actor on ONE page (Playwright records one video per
// page, so one coherent clip per scene), while everything else — the rest of
// the cast, mid-scene reactions, world state — is seeded/driven off camera via
// RPC. Scenes run in order in one worker (`test.describe.serial`), sharing the
// worker's hermetic server + DB, so the world accumulates across scenes.
//
// Scene clips land in each scene-test's own output directory
// (test-results/walkthrough-<reel>-<NN>-…-walkthrough/). Titling scenes
// with a zero-padded index ("01 …", "02 …") makes the lexicographic directory
// order the reel order — scripts/export_walkthrough.sh <reel> collects,
// transcodes, and concatenates them into the deliverables. Keep scene titles
// SHORT and ASCII-only: the title becomes the output dir + scene filename,
// and Playwright middle-hashes long directory names.
//
// Pacing knobs (env): WALKTHROUGH_PAUSE_MS (per-screen dwell, default 2200),
// WALKTHROUGH_TYPE_DELAY_MS (per-keystroke, default 60).

import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import {
  test,
  type Browser,
  type BrowserContext,
  type Locator,
  type Page,
} from '@playwright/test';
import { injectAuth, type InjectAuthOptions } from './auth.js';
import { installRequestIdOverride } from './connect.js';
import { newRecordingContext } from './context.js';
import { SIMULATION_TIMESTAMP_HEADER, filmedAt } from './filmed-at.js';
import {
  walkthroughEmulation,
  captureContextOptions,
  isDesktopCapture,
} from './capture.js';

/** Per-screen dwell that makes a paced video watchable. */
export const PAUSE_MS = Number(process.env.WALKTHROUGH_PAUSE_MS ?? '2200');

/** Per-keystroke delay so typing reads as human. */
export const TYPE_DELAY_MS = Number(process.env.WALKTHROUGH_TYPE_DELAY_MS ?? '60');

/** The worker's server URL (set by lib/fixtures.ts / global-setup). */
export function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

/**
 * Strip the Firebase Auth Emulator's "Running in emulator mode" warning banner
 * (the JS SDK appends it to <body> on connectAuthEmulator). It's an e2e-only
 * artifact that must never appear on camera. Firebase re-appends it on later
 * connectAuthEmulator calls (e.g. when the phone-auth screen initializes), so a
 * one-shot remove isn't enough — observe body AND sweep on an interval.
 * addInitScript persists across SSR→Flutter navigations; call BEFORE the first
 * page.goto.
 */
export async function stripEmulatorBanner(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const BANNER = 'Running in emulator mode';
    const kill = () => {
      const body = document.body;
      if (!body) return;
      for (const el of Array.from(body.children)) {
        if (el.textContent?.includes(BANNER)) el.remove();
      }
    };
    const start = () => {
      if (!document.body) return;
      new MutationObserver(kill).observe(document.body, { childList: true });
      kill();
    };
    if (document.body) start();
    else document.addEventListener('DOMContentLoaded', start);
    setInterval(kill, 200);
  });
}

/**
 * Pre-accept the data-consent choice so the "Accept All" modal never films:
 * the app reads it via SharedPreferencesAsync, which on web stores each key
 * UNPREFIXED in localStorage with a JSON-encoded value (a quoted string).
 * Call before the first navigation.
 */
export async function preseedObservabilityConsent(page: Page): Promise<void> {
  await page.addInitScript(() => {
    for (const key of [
      'observability_crash_reporting',
      'observability_performance_monitoring',
      'observability_analytics',
    ]) {
      window.localStorage.setItem(key, '"granted"');
    }
  });
}

// Recording-start epoch per page, so the first settle() can compute how much
// boring head (blank page + bundle boot) the export step should trim away.
const cameraStartMs = new WeakMap<Page, number>();
const trimWritten = new WeakSet<Page>();

/** How much footage to keep before the first settled screen. */
const TRIM_LEAD_MS = 400;

/**
 * Dwell on the current screen so the viewer can take it in, annotating the
 * test report with the storyboard beat. The FIRST settle of a scene also
 * writes `trim.json` (seconds of head to cut) into the test's output dir —
 * the export step uses it to drop the blank-boot footage from the clip.
 */
export async function settle(page: Page, screen: string): Promise<void> {
  test.info().annotations.push({ type: 'screen', description: screen });
  const t0 = cameraStartMs.get(page);
  if (t0 !== undefined && !trimWritten.has(page)) {
    trimWritten.add(page);
    const trimSec = Math.max(0, (Date.now() - t0 - TRIM_LEAD_MS) / 1000);
    // The built-in page fixture's output dir may not exist yet mid-test
    // (its video is only moved there at test end).
    mkdirSync(test.info().outputDir, { recursive: true });
    writeFileSync(
      join(test.info().outputDir, 'trim.json'),
      `${JSON.stringify({ trimSec: Number(trimSec.toFixed(2)) })}\n`,
    );
  }
  await page.waitForTimeout(PAUSE_MS);
}

/**
 * Type like a person into a Flutter-web textbox — the first one on the page
 * unless [field] names another. `.fill()` doesn't reach Flutter's controller
 * (it leaves the field looking untouched and its form disabled) — real
 * keystrokes via pressSequentially do.
 */
export async function typeInto(
  page: Page,
  value: string,
  opts: { field?: Locator } = {},
): Promise<void> {
  const field = opts.field ?? page.getByRole('textbox').first();
  await field.waitFor({ timeout: 20_000 });
  // Retry the whole click→type→verify: Flutter-web drops keystrokes (often
  // ALL of them, sometimes just the first) when typing starts before the
  // field's focus/IME is wired after the click. Same pattern as
  // lib/ui/email-register.ts.
  for (let attempt = 0; attempt < 3; attempt++) {
    await field.click();
    await page.waitForTimeout(300);
    await field.pressSequentially(value, { delay: TYPE_DELAY_MS });
    // Best-effort read-back: inputValue() throws on Flutter-web's custom
    // flt-semantics element (not a real <input>), in which case we can't
    // verify and trust the settle above.
    let typed: string | null = null;
    try {
      typed = await field.inputValue();
    } catch {
      return;
    }
    // Compare letters+digits only: fields may reformat what was typed (the
    // phone field renders "+15551234576" as "+1 (555) 123-4576").
    const norm = (s: string) => s.replace(/[^\p{L}\p{N}]/gu, '');
    if (norm(typed) === norm(value)) return;
    await field.selectText();
    await page.keyboard.press('Backspace');
  }
  throw new Error(`could not reliably type "${value}"`);
}

/**
 * Register recording start for a page NOT created via openScene (the built-in
 * fixture page in single-scene reels), so its first settle writes trim.json.
 * Call at the top of the test body.
 */
export function armCameraTrim(page: Page): void {
  cameraStartMs.set(page, Date.now());
}

/** One scene's camera: a recording context + the single on-camera page. */
export interface Scene {
  context: BrowserContext;
  page: Page;
  /** Close the context so Playwright flushes the clip to disk. */
  close(): Promise<void>;
}

export interface OpenSceneOptions {
  /** Request-ID slug for this scene (shows up in server logs). */
  slug: string;
  /**
   * The on-camera actor's session, injected before first navigation. Omit for
   * a logged-out scene (e.g. an invite landing → phone signup).
   */
  actor?: InjectAuthOptions;
}

/**
 * Open a scene: a fresh capture-shaped (phone by default; desktop under
 * WALKTHROUGH_VIEWPORT=desktop, #2912), dark-mode, video-recording context
 * with the emulator banner stripped and (optionally) the actor signed in.
 *
 * Manual contexts inherit NOTHING from the project's `use` options, so this
 * re-applies the full walkthrough emulation from lib/capture.ts. The browser
 * itself is launched by the walkthrough project, so the 2× device-scale
 * launch flag is already in effect.
 */
export async function openScene(browser: Browser, opts: OpenSceneOptions): Promise<Scene> {
  const context = await newRecordingContext(browser, {
    ...captureContextOptions,
    ...walkthroughEmulation,
  });
  // Fail fast on a stuck locator: without this, a click on a never-appearing
  // element waits silently until the SCENE timeout (minutes of dead footage)
  // instead of failing at the offending step. Desktop captures get double the
  // budget: the 2880×1620 screencast rasterizes ~3.5× the phone recipe's
  // pixels, which is enough per-keystroke overhead that a long typed prompt
  // overruns 20s at authentic typing pace.
  context.setDefaultTimeout(isDesktopCapture ? 40_000 : 20_000);
  // When the reel declares the moment it is filmed at, both halves of this
  // scene have to believe it: the app's own calls carry it to the server, and
  // the page's clock is what turns a start time into "3d from now". Setting
  // one without the other is worse than setting neither — see lib/filmed-at.ts.
  const when = filmedAt();
  if (when) {
    await context.setExtraHTTPHeaders({
      [SIMULATION_TIMESTAMP_HEADER]: String(Math.floor(when.getTime() / 1000)),
    });
  }
  if (opts.actor) await injectAuth(context, opts.actor);
  const page = await context.newPage();
  if (when) await page.clock.setFixedTime(when);
  cameraStartMs.set(page, Date.now());
  await stripEmulatorBanner(page);
  await preseedObservabilityConsent(page);
  await installRequestIdOverride(page, opts.slug);
  return {
    context,
    page,
    close: async () => {
      await context.close();
    },
  };
}
