// e2e/tests/workflows/streams-desktop.spec.ts
//
// #2912 — the three stream destinations (Home, Library, People) hold a
// centered measure on a desktop-wide window instead of stretching their
// single column across it:
//   - Home: the zero-state's "Ask for something" pill — a ~1400px-wide slab
//     before #2912 — sits inside the centered reading band;
//   - Library: shelf content starts at the GALLERY measure's left edge, and
//     the header's title starts at the same place (the shelf's scroll region
//     deliberately stays window-wide; it is the CONTENT inset that aligns);
//   - People: a directory row sits inside the reading band.
//
// Library uses a different band from the other two on purpose (#2926). It
// lays out fixed-size tiles rather than prose, so line length is not what
// should bound it; held at the reading measure it left a 420px void down the
// left of a 1440 window while tiles ran off the right.
//
// Tagged @desktop for the desktop-chromium project; under the phone projects
// every band widens to the viewport and the assertions reduce to "on screen".
// Geometry only, never tap success (#2908: flt-semantics nodes stay clickable
// while painted off-screen).

import { test, expect } from '../../lib/fixtures.js';
import { createTestClient, installRequestIdOverride } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';

const SPEC_SLUG = 'streams-desktop';

/// Mirrors Responsive.contentMaxWidth (app/lib/core/utils/responsive.dart).
const CONTENT_MAX_WIDTH = 640;

/// Mirrors Responsive.galleryMaxWidth (app/lib/core/utils/responsive.dart).
const GALLERY_MAX_WIDTH = 1200;

/// Mirrors Responsive.baseInset — and DestinationHeader's own 20px horizontal
/// padding, which is why the header title and the first shelf tile land on
/// the same x.
const BASE_INSET = 20;

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts populates it');
  return url;
}

test('@desktop Home, Library, and People hold the reading measure', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'layout geometry validated on chromium only');
  const baseUrl = requireBaseUrl();

  const host = await registerUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'Stella Streams',
  });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Measure Lane',
  });
  const gearName = 'Extension Ladder';
  const gear = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: gearName,
  });
  // Share the gear into the named community — seedSharedGear leaves it in its
  // per-item ad-hoc community only, which the Library's discover search does
  // not cover, so without this the shelf is legitimately empty.
  const communityClient = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  await communityClient.shareItem({
    item: { case: 'gearId', value: gear.gearId },
    shareToCommunityIds: [community.communityId],
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
    const SLACK = 2;

    /// The left edge of a centered band of `measure` width. Under the phone
    /// projects every measure exceeds the viewport, so this collapses to 0
    /// and the assertions reduce to "on screen".
    const bandLeftFor = (measure: number): number =>
      (viewport.width - Math.min(viewport.width, measure)) / 2;

    /// Asserts a locator's painted box: inside the viewport, starting at or
    /// right of the band's left edge, and (optionally) ending inside it.
    const expectInBand = async (
      locator: ReturnType<typeof page.getByRole>,
      what: string,
      {
        rightBounded = true,
        measure = CONTENT_MAX_WIDTH,
      }: { rightBounded?: boolean; measure?: number } = {},
    ): Promise<void> => {
      const bandLeft = bandLeftFor(measure);
      const bandRight = bandLeft + Math.min(viewport.width, measure);
      await expect(async () => {
        const box = await locator.boundingBox();
        expect(box, `${what} has no box at ${where}`).not.toBeNull();
        expect(
          box!.x,
          `${what} starts ${Math.round(bandLeft - box!.x)}px left of the measure band at ${where}`,
        ).toBeGreaterThanOrEqual(bandLeft - SLACK);
        if (rightBounded) {
          expect(
            box!.x + box!.width,
            `${what} overruns the measure band at ${where}`,
          ).toBeLessThanOrEqual(bandRight + SLACK);
        }
        expect(box!.y + box!.height, `${what} is below the fold at ${where}`)
          .toBeLessThanOrEqual(viewport.height);
      }).toPass({ timeout: 20_000 });
    };

    await page.goto(`${baseUrl}/`);
    await page
      .getByRole('button', { name: /accept all/i })
      .click({ timeout: 30_000 })
      .catch(() => {});

    // Home — the zero-state's Ask pill holds the measure. All three prompts
    // are identical since #2936, so any of them would do; this one is the
    // slab that was ~1400px wide before #2912.
    const askPill = page.getByRole('button', { name: /ask for something/i });
    await expect(askPill).toBeVisible({ timeout: 20_000 });
    await expectInBand(askPill, 'the "Ask for something" pill');

    // Library — the seeded gear's shelf tile starts EXACTLY at the gallery
    // band's left edge plus the base inset. An equality, not a lower bound:
    // #2926 was an inset that was too *large* (420px at 1440, the reading
    // measure's gutter), which a "at or right of the band" assertion waves
    // through. The right edge stays unasserted — a populated shelf
    // legitimately scrolls past it, and off the left edge too once dragged
    // (the gutter is scroll padding by design).
    await page.getByRole('button', { name: /^library$/i }).click({ timeout: 15_000 });
    const shelfTile = page.getByRole('button', { name: new RegExp(gearName, 'i') }).first();
    await expect(shelfTile).toBeVisible({ timeout: 20_000 });
    const expectedShelfLeft = bandLeftFor(GALLERY_MAX_WIDTH) + BASE_INSET;
    await expect(async () => {
      const box = await shelfTile.boundingBox();
      expect(box, `the "${gearName}" shelf tile has no box at ${where}`).not.toBeNull();
      expect(
        Math.abs(box!.x - expectedShelfLeft),
        `the "${gearName}" shelf tile starts at ${Math.round(box!.x)}px, ` +
          `expected ${expectedShelfLeft}px (gallery band + base inset) at ${where}`,
      ).toBeLessThanOrEqual(SLACK);
      expect(box!.y + box!.height, `the shelf tile is below the fold at ${where}`)
        .toBeLessThanOrEqual(viewport.height);
    }).toPass({ timeout: 20_000 });

    // ...and the header title starts in the same place. This is the
    // invariant #2926 actually reported: a header centered at one measure
    // over shelves inset to another reads as a void down the left. The
    // Library title is tappable (it opens the location sheet), so it is a
    // button with semantic siblings and keeps its own paint rect — unlike
    // the merged directory rows below. Matched on its semantics label, not
    // its visible "Near me ▾" text: Tappable's semanticsLabel replaces the
    // child's text in the accessible name.
    const libraryTitle = page
      .getByRole('button', { name: /change the distance anchor/i })
      .first();
    await expect(libraryTitle).toBeVisible({ timeout: 20_000 });
    const titleBox = await libraryTitle.boundingBox();
    const tileBox = await shelfTile.boundingBox();
    expect(titleBox, `the Library header title has no box at ${where}`).not.toBeNull();
    expect(tileBox, `the shelf tile has no box at ${where}`).not.toBeNull();
    expect(
      Math.abs(titleBox!.x - tileBox!.x),
      `the Library header title and the shelf tile disagree by ` +
        `${Math.round(titleBox!.x - tileBox!.x)}px at ${where}`,
    ).toBeLessThanOrEqual(SLACK);

    // People — a semantics-vs-paint trap in the #2908 family, discovered by
    // probing: the directory ListView is deliberately window-wide (scrollbar
    // at the edge; only its children hold the measure), and a list item whose
    // entire content merges into ONE button node gets the sliver-wide item
    // wrapper's rect (0w1440 here) while its paint sits inside the band. So
    // the row's box cannot be asserted from e2e; its paint geometry is proven
    // by directory_screen_test.dart's desktop group (text rects are real).
    // What e2e asserts honestly on this surface: the row exists on screen,
    // and the header title — a node with semantic siblings, which therefore
    // keeps its own paint-representative rect — holds the band.
    await page.getByRole('button', { name: /^people$/i }).click({ timeout: 15_000 });
    const communityRow = page.getByRole('button', { name: /measure lane/i }).first();
    await expect(communityRow).toBeVisible({ timeout: 20_000 });
    const peopleTitle = page.getByRole('button', { name: /sort & filter/i }).first();
    await expect(peopleTitle).toBeVisible({ timeout: 20_000 });
    await expectInBand(peopleTitle, "the People header's title");
  } finally {
    await ctx.close();
  }
});
