// e2e/tests/workflows/desktop-shell.spec.ts
//
// #2912 — the universal shell surfaces must read as designed on a desktop-wide
// window instead of stretching to it:
//   1. the Data Consent sheet (a fresh session's first modal) is capped at the
//      sheet measure, centered, with its action row fully on screen — the row
//      was a confirmed near-miss at 1440×810 in the #2908 investigation;
//   2. the NavDock capsule is capped at the dock measure and centered, not a
//      ~1412px-wide slab of four tiny icons;
//   3. a bottom sheet opened from the shell (the unified-create modal via the
//      dock's Create button) sits inside the same centered sheet band.
//
// Tagged @desktop so the desktop-chromium project (1440×810) runs it; the
// phone projects run it too, where every band widens to the full viewport and
// the assertions reduce to "on screen" — the same invariant at both shapes.
// All assertions are on bounding-box geometry, never tap success: flt-semantics
// nodes stay clickable while painted off-screen (#2908).

import { test, expect } from '../../lib/fixtures.js';
import { installRequestIdOverride } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';

const SPEC_SLUG = 'desktop-shell';

// Mirror the app's constants (Responsive.sheetMaxWidth / dockMaxWidth and the
// dock overlay's 14px side padding). Keep in sync with
// app/lib/core/utils/responsive.dart.
const SHEET_MAX_WIDTH = 560;
const DOCK_MAX_WIDTH = 500;
const DOCK_SIDE_PADDING = 14;

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts populates it');
  return url;
}

test('@desktop consent sheet, dock capsule, and bottom sheets hold the desktop measure', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'layout geometry validated on chromium only');
  const baseUrl = requireBaseUrl();

  const host = await registerUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'Desmond Wide',
  });
  await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Measure Lane',
  });

  // Manual contexts do NOT inherit the project's use.viewport (#2908 trap) —
  // forward it so the desktop-chromium project's 1440×810 actually applies.
  const projectViewport = test.info().project.use.viewport;
  const ctx = await newRecordingContext(browser, {
    viewport: projectViewport ?? { width: 1440, height: 810 },
  });
  const page = await ctx.newPage();
  try {
    await injectAuth(ctx, {
      accessToken: host.accessToken,
      refreshToken: host.refreshToken,
      user: { id: host.userId, name: host.name },
      serverUrl: baseUrl,
    });
    await installRequestIdOverride(page, SPEC_SLUG);

    const viewport = page.viewportSize();
    if (!viewport) throw new Error('no viewport size; every project sets one');
    const where = `${viewport.width}x${viewport.height}`;

    // The centered band a capped element must sit inside at this shape. On a
    // phone viewport the cap exceeds the window, so the band is the window and
    // the assertion reduces to "on screen".
    const band = (cap: number) => {
      const width = Math.min(viewport.width, cap);
      return {
        left: (viewport.width - width) / 2,
        right: (viewport.width + width) / 2,
      };
    };
    const SLACK = 2;

    /// Asserts a locator's painted box sits inside the given horizontal band
    /// and wholly inside the viewport. toPass because Flutter rebuilds its
    /// semantics tree continuously and a stale handle's boundingBox is null.
    const expectInBand = async (
      locator: ReturnType<typeof page.getByRole>,
      cap: number,
      what: string,
    ): Promise<void> => {
      const { left, right } = band(cap);
      await expect(async () => {
        const box = await locator.boundingBox();
        expect(box, `${what} has no box at ${where}`).not.toBeNull();
        expect(
          box!.x,
          `${what} starts ${Math.round(left - box!.x)}px left of the measure band at ${where}`,
        ).toBeGreaterThanOrEqual(left - SLACK);
        expect(
          box!.x + box!.width,
          `${what} ends ${Math.round(box!.x + box!.width - right)}px right of the measure band at ${where}`,
        ).toBeLessThanOrEqual(right + SLACK);
        expect(
          box!.y + box!.height,
          `${what} is painted below the fold at ${where}`,
        ).toBeLessThanOrEqual(viewport.height);
        expect(box!.y, `${what} is painted above the top at ${where}`)
          .toBeGreaterThanOrEqual(0);
      }).toPass({ timeout: 20_000 });
    };

    await page.goto(`${baseUrl}/`);

    // 1. The Data Consent sheet: a fresh session's first modal. Its Accept-All
    // action must sit inside the centered sheet band AND fully on screen —
    // the on-screen half is the #2908 near-miss regression.
    const acceptAll = page.getByRole('button', { name: /accept all/i });
    await expect(acceptAll).toBeVisible({ timeout: 30_000 });
    await expectInBand(acceptAll, SHEET_MAX_WIDTH, "the consent sheet's Accept All");
    await acceptAll.click();

    // 2. The dock capsule: its first tab and its Create button must both sit
    // inside the centered dock band (the capsule spans at most the dock
    // measure inside the overlay's side padding).
    const dockCap = Math.min(DOCK_MAX_WIDTH, viewport.width - 2 * DOCK_SIDE_PADDING);
    const homeTab = page.getByRole('button', { name: /^home$/i });
    await expect(homeTab).toBeVisible({ timeout: 20_000 });
    await expectInBand(homeTab, dockCap, "the dock's Home tab");
    const createButton = page.getByRole('button', { name: /^create$/i });
    await expectInBand(createButton, dockCap, "the dock's Create button");

    // 3. A bottom sheet opened from the shell: the zero-state's "Ask for
    // something" prompt opens the create sheet. Since #2936 that entry point
    // seeds the request type, which opens the drawer on the Text tab — the
    // tab click below is now a no-op there, and is kept because this spec
    // only cares that a sheet holds the measure, not which tab it landed on.
    await page
      .getByRole('button', { name: /ask for something/i })
      .click({ timeout: 20_000 });
    await page
      .getByRole('button', { name: /^text$/i })
      .click({ timeout: 20_000 })
      .catch(() => {}); // already in text mode
    const promptField = page.getByRole('textbox').first();
    await expect(promptField).toBeVisible({ timeout: 20_000 });
    await expectInBand(promptField, SHEET_MAX_WIDTH, "the create sheet's prompt field");
  } finally {
    await ctx.close();
  }
});
