// e2e/lib/capture.ts — the single source of truth for phone-shaped, HI-DPI
// video capture, shared by playwright.config.ts (the mobile-chromium and
// walkthrough projects) and lib/walkthrough.ts (manually created scene
// contexts, which do NOT inherit the project's context options).
//
// The capture recipe (#2492 video legibility):
//  - A tall 390×844 phone viewport (~9:19.5), NOT the squat visual viewport
//    the iPhone-14 isMobile preset records — hence isMobile:false.
//  - 2× capture: Playwright's video recorder IGNORES the context
//    deviceScaleFactor; only the browser launch flag
//    --force-device-scale-factor=2 raises the screencast resolution.
//    video.size is set to those device pixels (780×1688) so the frame is
//    captured 1:1 (not letterboxed). Flutter CanvasKit also rasterizes text
//    at the forced 2×.
//  - Chromium-only (the flag is a Chromium arg).

/** The CSS-pixel phone viewport every capture-grade context uses. */
export const PHONE_VIEWPORT = { width: 390, height: 844 } as const;

/** The 2× device-pixel capture size (PHONE_VIEWPORT × deviceScaleFactor). */
export const PHONE_VIDEO_SIZE = { width: 780, height: 1688 } as const;

export const PHONE_USER_AGENT =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1';

/**
 * Context-level phone emulation. Manual `browser.newContext()` calls get NONE
 * of the project's `use` options, so any context whose page should look like
 * the project's capture must spread these in explicitly.
 */
export const phoneContextOptions = {
  viewport: { ...PHONE_VIEWPORT },
  deviceScaleFactor: 2,
  isMobile: false,
  hasTouch: true,
  userAgent: PHONE_USER_AGENT,
} as const;

/**
 * Walkthrough-only emulation on top of the capture context options: a
 * DARK-MODE device with a pinned tz + locale so an event's evening time
 * renders identically on the SSR landing and in-app (no UTC drift). Matches
 * the walkthrough project in playwright.config.ts.
 */
export const walkthroughEmulation = {
  colorScheme: 'dark',
  timezoneId: 'America/Los_Angeles',
  locale: 'en-US',
} as const;

// --- Desktop-aspect capture (#2912) -----------------------------------------
//
// A walkthrough reel can render at a 16:9 laptop shape instead of the phone,
// as a LOCAL evaluation artifact for desktop layout work — same 2× recipe,
// same dark-mode walkthrough emulation, but a genuine desktop device (mouse,
// desktop UA) so the SSR landings behave as they would for a real laptop
// visitor. Selected per render via WALKTHROUGH_VIEWPORT=desktop (threaded
// through e2e/scripts/run_walkthrough.sh); everything that captures reads the
// capture* aliases below so phone stays the default everywhere else.

/** The CSS-pixel desktop viewport (matches the desktop-chromium project). */
export const DESKTOP_VIEWPORT = { width: 1440, height: 810 } as const;

/** The 2× device-pixel capture size for desktop renders. */
export const DESKTOP_VIDEO_SIZE = { width: 2880, height: 1620 } as const;

export const DESKTOP_USER_AGENT =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36';

/** Context-level desktop emulation — the laptop twin of phoneContextOptions. */
export const desktopContextOptions = {
  viewport: { ...DESKTOP_VIEWPORT },
  deviceScaleFactor: 2,
  isMobile: false,
  hasTouch: false,
  userAgent: DESKTOP_USER_AGENT,
} as const;

/** True when this render was asked for at desktop aspect (#2912). */
export const isDesktopCapture = process.env.WALKTHROUGH_VIEWPORT === 'desktop';

/** The active capture emulation: desktop when WALKTHROUGH_VIEWPORT=desktop, phone otherwise. */
export const captureContextOptions = isDesktopCapture
  ? desktopContextOptions
  : phoneContextOptions;

/** The active 2× video size matching captureContextOptions' viewport. */
export const captureVideoSize = isDesktopCapture ? DESKTOP_VIDEO_SIZE : PHONE_VIDEO_SIZE;
