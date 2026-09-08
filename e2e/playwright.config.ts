// Playwright configuration for the Flutter Web E2E harness (#2162).
//
// Two Phase-1 projects: mobile-chromium and webkit, both using
// Playwright's bundled iPhone 14 device preset (viewport + UA + touch
// flags). WebKit is Playwright's patched WebKit — close enough to
// Safari to catch most Safari-specific bugs without a device cloud.
//
// Video capture is split by environment (#2738). LOCALLY every test records
// (`video: 'on'`) so the clips are always available to watch — global-teardown.ts
// flattens each run's per-test videos into e2e/videos/ (gitignored, rebuilt per
// run) so they're easy to find without digging through the per-test
// test-results/ folders. In CI (`process.env.CI`) video is `retain-on-failure`:
// recording every passing test's clip and uploading the whole test-results/ dir
// on failure made these artifacts ~97.5% of the org's 2 GB Actions-storage quota.
// The `videoMode` constant below drives both the global default (the `webkit`
// project) and the phone-capture recipe (the `mobile-chromium` project); the
// `walkthrough` project overrides it back to `'on'` because producing video on
// pass IS its deliverable. The `walkthrough` project keeps its own HI-DPI video settings
// + export pipeline and filters to specs tagged @walkthrough (the paced
// onboarding-baseline spec) — those are kept OUT of the standard projects via
// `grepInvert` so the deliberately-slow, video-generating specs never bloat
// the normal suite or CI gate.
//
// `webServer` is intentionally NOT configured here — the test server
// is bootstrapped per-spec by e2e/lib/server.ts (which itself shells
// out to scripts/start_e2e_server.sh to avoid duplicating the
// ~10-flag startup invocation in TS). See the plan
// (docs/issues/2162-web-e2e-harness.md) for rationale.

import { defineConfig, devices } from '@playwright/test';
import {
  PHONE_VIDEO_SIZE,
  captureContextOptions,
  captureVideoSize,
  isDesktopCapture,
  walkthroughEmulation,
  phoneContextOptions,
} from './lib/capture';

// CI keeps only failure clips to bound the org Actions-storage quota (#2738);
// locally, record every test so global-teardown.ts collects them into e2e/videos/.
const videoMode: 'retain-on-failure' | 'on' = process.env.CI ? 'retain-on-failure' : 'on';

// Phone-shaped, HI-DPI video capture — shared by the standard mobile-chromium
// project and the walkthrough project so every recorded clip is
// showcase-grade (#2492 video legibility). The capture recipe (tall 390×844
// viewport, browser-level --force-device-scale-factor=2, 780×1688 1:1 video
// size) lives in lib/capture.ts, the shared source of truth with the
// manually-created walkthrough scene contexts (lib/walkthrough.ts).
// Chromium-only (the flag is a Chromium arg). The webkit project keeps the
// iPhone-14 preset + the global use.video:'on' (1× capture).
const phoneVideoCaptureChromium = {
  browserName: 'chromium' as const,
  launchOptions: { args: ['--force-device-scale-factor=2', '--high-dpi-support=1'] },
  ...phoneContextOptions,
  // CI: retain-on-failure; local: on (see videoMode). The `walkthrough` project
  // spreads this recipe but re-pins mode to 'on' so its reel always records.
  video: { mode: videoMode, size: PHONE_VIDEO_SIZE },
};

export default defineConfig({
  testDir: './tests',
  globalSetup: require.resolve('./global-setup.ts'),
  globalTeardown: require.resolve('./global-teardown.ts'),
  // Per-test timeout. The Flutter Web bundle takes 8-12s to download +
  // initialize + render first frame on the self-hosted runner, so the
  // floor is well above Playwright's default 30s. Multi-client workflow
  // specs open several authenticated browser contexts in sequence, each
  // paying that cold-bundle cost, so they need substantial headroom on the
  // (slower-than-local) CI runner.
  timeout: 120_000,
  expect: {
    // Locator auto-wait timeout. Big enough to absorb a bundle-bootstrap
    // hiccup, small enough to fail fast when a Semantics identifier
    // genuinely doesn't appear.
    timeout: 15_000,
  },
  // Fail the build in CI on test.only left behind.
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  // Run specs in parallel in CI. Each worker gets its OWN isolated server +
  // ephemeral database (lib/fixtures.ts: worker binds E2E_BASE_PORT +
  // parallelIndex, with a fresh DB in the shared Postgres container), so
  // concurrent specs never collide. Default 4; override with E2E_WORKERS.
  // Locally Playwright's default (≈ half the cores) is fine.
  workers: process.env.CI ? Number(process.env.E2E_WORKERS ?? '4') : undefined,
  reporter: [
    ['html', { open: 'never' }],
    process.env.CI ? ['github'] : ['list'],
  ],
  use: {
    // CI uploads trace.zip on failure so triagers can step through.
    trace: 'retain-on-failure',
    // Screenshot only on failure to keep artefact size manageable.
    screenshot: 'only-on-failure',
    // Record video for every test locally (global-teardown.ts collects them into
    // e2e/videos/ for easy watching); in CI, only failure clips (#2738). Drives
    // the `webkit` project, which inherits this global. See videoMode above.
    video: videoMode,
  },
  projects: [
    {
      name: 'mobile-chromium',
      // @walkthrough specs (paced video baselines) are excluded — they only run
      // under the walkthrough project below.
      grepInvert: /@walkthrough/,
      // Walkthrough-grade phone capture for every run (tall viewport, 2× pixels) —
      // see phoneVideoCaptureChromium. WebKit fidelity lives in the 'webkit'
      // project below (which keeps the iPhone-14 preset).
      use: { ...phoneVideoCaptureChromium },
    },
    {
      name: 'webkit',
      grepInvert: /@walkthrough/,
      use: { ...devices['iPhone 14'], browserName: 'webkit' },
    },
    {
      // Landscape desktop coverage (#2908). Every other project renders a tall
      // phone, which is why an app that was unusable in ordinary browser
      // windows shipped green: past roughly a 1.1:1 width-to-height ratio the
      // Plans month grid grew taller than the viewport and swallowed the
      // day-detail panel below it, and nothing in CI had ever instantiated a
      // landscape viewport to notice.
      //
      // Deliberately scoped to @desktop-tagged specs rather than the whole
      // suite: running everything twice would roughly double CI wall-clock and
      // artefact volume for coverage that only layout-sensitive specs need
      // (the #2738 storage lesson). Tag a spec @desktop when its failure mode
      // is geometric.
      name: 'desktop-chromium',
      grep: /@desktop/,
      use: {
        browserName: 'chromium',
        // A 16:9 laptop — the shape the reporter hit and the most common
        // desktop window there is.
        viewport: { width: 1440, height: 810 },
        video: videoMode,
      },
    },
    {
      // Generates watchable, paced user-journey walkthrough videos (#2684)
      // for design iteration and sharing (epic #2492). Only runs
      // @walkthrough-tagged specs (tests/walkthroughs/*), always records video
      // ('on' keeps it on pass too), chromium-only. Render a reel end-to-end
      // (hermetic DB + export) with:
      //   e2e/scripts/run_walkthrough.sh <reel-slug>
      // or run the specs alone with:
      //   npx playwright test --project=walkthrough
      //
      // Capture recipe: phoneVideoCaptureChromium (lib/capture.ts) plus
      // walkthrough-only emulation (walkthroughEmulation): a DARK-MODE device —
      // the SSR landing is forced dark in CSS regardless
      // (website/content/css/tokens.css .theme-dark); the Flutter app
      // respects the device theme, so it renders dark too, a faithful
      // dark-device clip — with a pinned tz + locale so an event's evening
      // time renders identically on the SSR landing and in-app.
      name: 'walkthrough',
      grep: /@walkthrough/,
      use: {
        ...phoneVideoCaptureChromium,
        // Phone by default; WALKTHROUGH_VIEWPORT=desktop swaps in the 16:9
        // laptop capture for desktop-evaluation renders (#2912). Manually
        // created scene contexts pick up the same switch via openScene
        // (lib/walkthrough.ts); this spread covers single-scene reels that
        // use the built-in page fixture.
        ...captureContextOptions,
        ...walkthroughEmulation,
        // Reels ARE the video deliverable (#2684 / epic #2492), so always record
        // on pass — re-pin mode to 'on', overriding the CI-gated videoMode that
        // phoneVideoCaptureChromium carries. See docs/walkthroughs.md.
        video: { mode: 'on' as const, size: captureVideoSize },
        // Fail fast on a stuck locator: paced scenes run with generous test
        // timeouts, and an unbounded action wait turns a wrong label into
        // minutes of dead footage before the failure. (Manually created scene
        // contexts get the same guard via openScene in lib/walkthrough.ts.)
        // Desktop captures get double the budget — the 2880×1620 screencast's
        // per-action rasterization overhead overruns 20s on long typed prompts.
        actionTimeout: isDesktopCapture ? 40_000 : 20_000,
      },
    },
  ],
});
