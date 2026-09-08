// e2e/tests/workflows/item-routes-desktop.spec.ts
//
// #2912 — the item content routes (gear / request / experience) render as a
// full-bleed hero with a bottom-centered caption column on a desktop-wide
// window, instead of a sheet and cards stretched across the whole viewport.
// Also covers a morph-reveal panel: its body holds the same measure over the
// full-bleed scrim.
//
// Tagged @desktop for the desktop-chromium project; under the phone projects
// every band widens to the viewport and the assertions reduce to "on screen".
// Geometry only, never tap success (#2908).

import { test, expect } from '../../lib/fixtures.js';
import { installRequestIdOverride } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { seedRequest } from '../../lib/seed/requests.js';
import { createExperience } from '../../lib/seed/experiences.js';

const SPEC_SLUG = 'item-routes-desktop';

/// Mirrors Responsive.contentMaxWidth (app/lib/core/utils/responsive.dart).
const CONTENT_MAX_WIDTH = 640;

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts populates it');
  return url;
}

test('@desktop item routes hold the caption column over the hero', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'layout geometry validated on chromium only');
  const baseUrl = requireBaseUrl();

  const host = await registerUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'Rex Routes',
  });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Caption Court',
  });
  const gear = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Extension Ladder',
  });
  const request = await seedRequest({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    title: 'Need a canopy for Saturday',
  });
  const eventTime = new Date();
  eventTime.setHours(12, 0, 0, 0);
  const experience = await createExperience({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
    name: 'Ladder Return Party',
    description: 'Seeded so the experience route has a real read shell.',
    timeUnixSec: Math.floor(eventTime.getTime() / 1000),
  });

  // Forward the project viewport — manual contexts inherit nothing (#2908).
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
    const bandWidth = Math.min(viewport.width, CONTENT_MAX_WIDTH);
    const bandLeft = (viewport.width - bandWidth) / 2;
    const bandRight = bandLeft + bandWidth;
    const SLACK = 2;

    const expectInBand = async (
      locator: ReturnType<typeof page.getByText>,
      what: string,
    ): Promise<void> => {
      await expect(async () => {
        const box = await locator.boundingBox();
        expect(box, `${what} has no box at ${where}`).not.toBeNull();
        expect(
          box!.x,
          `${what} starts ${Math.round(bandLeft - box!.x)}px left of the measure band at ${where}`,
        ).toBeGreaterThanOrEqual(bandLeft - SLACK);
        expect(
          box!.x + box!.width,
          `${what} overruns the measure band at ${where}`,
        ).toBeLessThanOrEqual(bandRight + SLACK);
        expect(box!.y + box!.height, `${what} is below the fold at ${where}`)
          .toBeLessThanOrEqual(viewport.height);
      }).toPass({ timeout: 20_000 });
    };

    // First navigation: land on Home once to clear the Data Consent sheet.
    await page.goto(`${baseUrl}/`);
    await page
      .getByRole('button', { name: /accept all/i })
      .click({ timeout: 30_000 })
      .catch(() => {});

    // The read shells' headlines are painted CanvasKit text with no DOM text
    // node of their own (only the hero img carries the name as a label), so
    // every assertion below anchors on the shells' interactive cards, whose
    // merged aria labels are stable ("See the calendar", "View details",
    // "Open conversation …").

    // Gear route — the who-card's "See the calendar" pill (a window-wide pill
    // before #2912) and the details card hold the measure.
    await page.goto(`${baseUrl}/gear/${gear.gearId}`);
    const seeCalendar = page
      .getByRole('button', { name: /^see the calendar$/i })
      .first();
    await expect(seeCalendar).toBeVisible({ timeout: 20_000 });
    await expectInBand(seeCalendar, 'the "See the calendar" pill');
    await expectInBand(
      page.getByRole('button', { name: /view details/i }).first(),
      'the "View details" card',
    );

    // Morph panel — the gear discussion card grows over the hero; the panel
    // body holds the measure while the scrim stays full-bleed.
    const discussion = page
      .getByRole('button', { name: /open conversation/i })
      .first();
    await expect(discussion).toBeVisible({ timeout: 20_000 });
    await discussion.click();
    const messageBox = page.getByRole('textbox').first();
    await expect(messageBox).toBeVisible({ timeout: 20_000 });
    await expectInBand(messageBox, "the conversation panel's message field");

    // Request route — the discussion card holds the measure.
    await page.goto(`${baseUrl}/request/${request.requestId}`);
    const requestDiscussion = page
      .getByRole('button', { name: /open conversation/i })
      .first();
    await expect(requestDiscussion).toBeVisible({ timeout: 20_000 });
    await expectInBand(requestDiscussion, "the request route's discussion card");

    // Experience route — the discussion card holds the measure.
    await page.goto(`${baseUrl}/experience/${experience.experienceId}`);
    const eventDiscussion = page
      .getByRole('button', { name: /open conversation/i })
      .first();
    await expect(eventDiscussion).toBeVisible({ timeout: 20_000 });
    await expectInBand(eventDiscussion, "the experience route's discussion card");
  } finally {
    await ctx.close();
  }
});
