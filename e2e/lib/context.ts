// e2e/lib/context.ts — a browser context that always records video.
//
// The workflow specs create their own contexts (via browser.newContext) so they
// can inject auth into localStorage *before* the first navigation. Playwright's
// config-level `use.video` option only applies to the built-in `page`/`context`
// fixtures — NOT to contexts created manually with `browser.newContext()` — so
// video has to be requested explicitly. Routing every spec through this helper
// keeps "always record video" in one place; global-teardown.ts then flattens
// each run's clips into e2e/videos/ (gitignored) for easy watching.

import {
  type Browser,
  type BrowserContext,
  type BrowserContextOptions,
  test,
} from '@playwright/test';
import { captureVideoSize } from './capture.js';

/**
 * Like `browser.newContext(options)`, but always records video to the current
 * test's output directory (so global-teardown.ts can collect it). Pass the same
 * options you'd pass to `browser.newContext`; an explicit `recordVideo` in
 * `options` wins (e.g. a custom size).
 */
export async function newRecordingContext(
  browser: Browser,
  options: BrowserContextOptions = {},
): Promise<BrowserContext> {
  return browser.newContext({
    ...options,
    recordVideo: {
      dir: test.info().outputDir,
      // Manual contexts ignore the project's `video.size`, defaulting to the
      // viewport scaled down to 800px. Capture at the active viewport's 2×
      // device pixels (captureVideoSize, lib/capture.ts — phone by default,
      // desktop under WALKTHROUGH_VIEWPORT=desktop) to match the walkthrough
      // project, so the clip is crisp/retina rather than downscaled.
      // (mobile-chromium forces device scale 2 via
      // --force-device-scale-factor; see playwright.config.ts.)
      size: captureVideoSize,
      ...options.recordVideo,
    },
  });
}
