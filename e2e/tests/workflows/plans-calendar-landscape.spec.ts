// e2e/tests/workflows/plans-calendar-landscape.spec.ts
//
// #2908 regression — the Plans calendar must stay usable in a landscape browser
// window. Tagged @desktop so it also runs under the desktop-chromium project
// (1440×810); the phone projects run it too, which is the point — the same
// assertions must hold at both shapes.
//
// The bug: CalendarMonthGrid's day cells are square, so the grid sized itself
// from width alone and its height grew with the window's width. It sits in a
// non-scrolling Column above the day-detail panel, so past roughly a 1.1:1
// width-to-height ratio — every ordinary landscape window — the grid consumed
// the viewport, the day detail was starved to nothing, and the month's last
// rows clipped behind the dock with no way to scroll to either. At 1440×810 the
// day detail was not merely painted off-screen: it was absent from the
// accessibility tree entirely, so a screen-reader user lost the day's events
// too. The fix derives the cell from both axes (monthGridCellSide).
//
// Two assertions, because neither implies the other and each catches a
// different one of the two observed failure modes:
//   1. the seeded event is IN the accessibility tree  — fails at 1440×810
//   2. its box lies INSIDE the viewport                — fails at 1440×1400
// Note (1) alone is not enough on Flutter Web: flt-semantics nodes stay present
// and clickable while their CanvasKit pixels are painted past the viewport
// edge, so a passing click proves nothing about human reachability.
//
// See docs/client/calendar.md and docs/issues/2908-web-aspect-ratios.md.

import { test, expect } from '../../lib/fixtures.js';
import { installRequestIdOverride } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createExperience } from '../../lib/seed/experiences.js';

const SPEC_SLUG = 'plans-calendar-landscape';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts populates it');
  return url;
}

test('@desktop Plans keeps the day detail on screen in a landscape window', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'layout geometry validated on chromium only');
  const baseUrl = requireBaseUrl();

  const host = await registerUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'Wilma Wide',
  });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Aspect Ratio Lane',
  });
  // Today, so the Plans tab — which anchors on today — opens straight onto the
  // day whose detail panel this spec is about. Midday to stay on today's date
  // regardless of the runner's timezone offset.
  const today = new Date();
  today.setHours(12, 0, 0, 0);
  const eventName = 'Ladder Return Party';
  await createExperience({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
    name: eventName,
    description: 'Seeded so the selected day has something in its detail panel.',
    timeUnixSec: Math.floor(today.getTime() / 1000),
  });

  // Manual contexts do NOT inherit the project's `use.viewport` — that option
  // only reaches the built-in page/context fixtures — so the viewport has to be
  // forwarded explicitly or this spec would run at Playwright's default
  // 1280×720 under every project, and the desktop-chromium project's 1440×810
  // would silently have no effect. Forwarding it is what makes one spec assert
  // the same invariant at whatever shape the running project defines.
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

    await page.goto(`${baseUrl}/`);
    // A fresh user gets the Data Consent modal once CanvasKit boots; it blocks
    // every later tap, so dismiss it first.
    await page
      .getByRole('button', { name: /accept all/i })
      .click({ timeout: 30_000 })
      .catch(() => {});

    await page.getByRole('button', { name: /^plans$/i }).click({ timeout: 15_000 });

    // (1) Present in the accessibility tree. Before the fix this failed
    // outright at 1440×810 — the panel was never built.
    //
    // `exact` matters: a substring match also hits the month grid's cell for
    // that day, whose accessible name is "Friday, August 14, Ladder Return
    // Party". Matching loosely and taking .first() silently asserts against the
    // grid cell — which is on screen either way — so the day-detail regression
    // would go unnoticed. The detail entry's name is the bare event title.
    const entry = page.getByRole('button', { name: eventName, exact: true });
    await expect(entry).toBeVisible({ timeout: 20_000 });

    // (2) Painted inside the viewport. Before the fix a taller landscape window
    // kept the node in the tree but pushed its pixels past the bottom edge.
    //
    // Wrapped in toPass because Flutter rebuilds its semantics tree
    // continuously: a node that satisfied toBeVisible() a moment ago can be
    // detached by the next rebuild, and boundingBox() on the stale handle
    // returns null. Retrying re-resolves the locator against the current tree.
    const viewport = page.viewportSize();
    if (!viewport) throw new Error('no viewport size; every project sets one');
    const where = `${viewport.width}x${viewport.height}`;

    /// Asserts a locator's painted box sits wholly inside the viewport.
    const expectOnScreen = async (
      locator: ReturnType<typeof page.getByRole>,
      what: string,
    ): Promise<void> => {
      await expect(async () => {
        const box = await locator.boundingBox();
        expect(box, `${what} has no box at ${where}`).not.toBeNull();
        expect(
          box!.y + box!.height,
          `${what} is painted ${Math.round(
            box!.y + box!.height - viewport.height,
          )}px below the fold at ${where}`,
        ).toBeLessThanOrEqual(viewport.height);
        expect(box!.y, `${what} is painted above the top at ${where}`)
          .toBeGreaterThanOrEqual(0);
      }).toPass({ timeout: 20_000 });
    };

    await expectOnScreen(entry, `the day detail's "${eventName}"`);

    // #2912: at expanded widths (>= 840 available) CalendarPanes moves the day
    // detail into a trailing rail BESIDE the grid instead of a strip below it.
    // Assert the detail entry paints inside that right-hand rail region. The
    // rail width mirrors CalendarPanes.detailPaneWidth (380).
    if (viewport.width >= 840) {
      const RAIL_WIDTH = 380;
      await expect(async () => {
        const box = await entry.boundingBox();
        expect(box, `the day detail entry has no box at ${where}`).not.toBeNull();
        expect(
          box!.x,
          `the day detail is not in the trailing rail at ${where} ` +
            `(x=${Math.round(box!.x)}, rail starts at ${viewport.width - RAIL_WIDTH})`,
        ).toBeGreaterThanOrEqual(viewport.width - RAIL_WIDTH - 4);
      }).toPass({ timeout: 20_000 });
    }

    // The month's own last row is deliberately NOT asserted here. Day cells can
    // only be addressed by a substring of their composed label ("Monday, August
    // 31, Nothing planned yet"), which also matches container nodes that carry
    // no box — the assertion then measures whichever node the locator happened
    // to pick, and flakes. That claim is covered deterministically instead by
    // home_calendar_screen_test.dart's landscape group, which asserts the last
    // CalendarDayCell's rect against the surface across five viewport shapes.
    // What only an e2e run can prove is what this spec keeps: that the real
    // CanvasKit bundle actually paints the day detail inside a landscape
    // window.
  } finally {
    await ctx.close();
  }
});
