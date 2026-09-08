// e2e/tests/workflows/library-shelf-gear.spec.ts
//
// Maps to: docs/client/calendar.md's sibling surface — the Library tab
// (DiscoverScreen, #2634 v2). No 1:1 workflow doc example; this is a
// Library-tab render regression.
//
// What this spec proves (UI-integration failure class RPC tests can't):
//   - The Library dock destination renders gear shared into the viewer's
//     communities as shelf tiles (SearchService empty-query → category
//     shelves → LibraryShelfTile), aggregating across MORE THAN ONE of the
//     viewer's communities onto the single tab.
//   - The borrowable-vs-giveaway distinction is painted correctly: a
//     FOR_GIVEAWAY tile announces "<name> · Giveaway" (the corner pill),
//     while a FOR_LOAN tile announces just its name — a #2634 v2 render
//     detail carried only in the tile's semantics, invisible to any RPC test.
//
// What this spec does NOT prove:
//   - Search scoring/ranking or distance math — server/services/search owns
//     that (unit-tested). Here the shelves are only asserted to CONTAIN the
//     seeded gear, not their order.
//   - Gear detail routing — covered separately; a tile tap pushes GearScreen.
//
// Every state change in the test body is a real UI gesture on the Flutter
// Web bundle; preconditions are seeded via RPC.

import { test, expect } from '../../lib/fixtures.js';
import { createTestClient, installRequestIdOverride } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { dismissDataConsent } from '../../lib/ui/consent.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { Availability } from '../../gen/ripls/api/gear_pb.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';

const SPEC_SLUG = 'library-shelf-gear';

// Distinct, stable names so each tile is locatable by its semantics label.
const LOAN_GEAR = 'Cordless Impact Drill';
const GIVEAWAY_GEAR = 'Garden Trowel Set';
// The giveaway tile's semantics label appends the pill via libraryTileGiveaway.
const GIVEAWAY_TILE_LABEL = `${GIVEAWAY_GEAR} · Giveaway`; // "… · Giveaway"

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts populates it');
  return url;
}

test('Library tab renders shared gear on shelves with the giveaway/borrowable pill', async ({
  browser,
  browserName,
}) => {
  test.skip(
    browserName === 'webkit',
    'Library shelves validated on chromium only (sibling: community-calendar-shared-event)',
  );
  const baseUrl = requireBaseUrl();

  // ── PRECONDITIONS (RPC seed) ─────────────────────────────────────────────
  // The viewer is the host, a member of TWO named communities. One gear is
  // shared into each (a gear's own per-item ad-hoc community is host-only and
  // does NOT count toward the discover scope — the viewer must belong to a
  // real community). Both tiles appearing on the single Library tab proves it
  // unions the search across the viewer's communities.
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Dana Host' });
  const toolLibrary = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Rosewood Tool Library',
  });
  const mutualAid = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Oakhurst Mutual Aid',
  });

  const loan = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: LOAN_GEAR,
    availability: Availability.FOR_LOAN,
  });
  const giveaway = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: GIVEAWAY_GEAR,
    availability: Availability.FOR_GIVEAWAY,
  });

  // Share each gear into a (different) real community the viewer belongs to.
  // Availability is item-wide and already set at creation (#2687), and ShareItem
  // inherits it, so the shelf tile still paints the right lend/give pill.
  const communityClient = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  await communityClient.shareItem({
    item: { case: 'gearId', value: loan.gearId },
    shareToCommunityIds: [toolLibrary.communityId],
  });
  await communityClient.shareItem({
    item: { case: 'gearId', value: giveaway.gearId },
    shareToCommunityIds: [mutualAid.communityId],
  });

  // ── BROWSER CONTEXT + AUTH ───────────────────────────────────────────────
  const ctx = await newRecordingContext(browser, { locale: 'en-US' });
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
    await dismissDataConsent(page);

    // ── STEP: open the Library tab (opens on the shelves/list view) ─────────
    await page.getByRole('button', { name: 'Library', exact: true }).click({ timeout: 15_000 });

    // ── ASSERT: both gear render as shelf tiles ─────────────────────────────
    // Borrowable tile: name only, no pill.
    await expect(
      page.getByRole('button', { name: LOAN_GEAR, exact: true }),
    ).toBeVisible({ timeout: 15_000 });
    // Giveaway tile: name + " · Giveaway" (the corner pill in its semantics).
    await expect(
      page.getByRole('button', { name: GIVEAWAY_TILE_LABEL, exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    // ── ASSERT: the pill is exclusive to the giveaway — the borrowable gear
    // must NOT carry a "· Giveaway" suffix, and the giveaway gear's tile is
    // not the bare name.
    await expect(
      page.getByRole('button', { name: `${LOAN_GEAR} · Giveaway`, exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole('button', { name: GIVEAWAY_GEAR, exact: true }),
    ).toHaveCount(0);
  } finally {
    await ctx.close();
  }
});

test('Library filter: turning off "Giving" removes giveaway gear from the shelves', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'Library filter sheet validated on chromium only');
  const baseUrl = requireBaseUrl();

  // Same shape as above: viewer/host in two communities, one loan gear + one
  // giveaway gear, each shared into a community the viewer belongs to.
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Reed Host' });
  const toolLibrary = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Fernwood Tool Library',
  });
  const mutualAid = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Fernwood Mutual Aid',
  });
  const loan = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: LOAN_GEAR,
    availability: Availability.FOR_LOAN,
  });
  const giveaway = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: GIVEAWAY_GEAR,
    availability: Availability.FOR_GIVEAWAY,
  });
  const communityClient = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  await communityClient.shareItem({
    item: { case: 'gearId', value: loan.gearId },
    shareToCommunityIds: [toolLibrary.communityId],
  });
  await communityClient.shareItem({
    item: { case: 'gearId', value: giveaway.gearId },
    shareToCommunityIds: [mutualAid.communityId],
  });

  const ctx = await newRecordingContext(browser, { locale: 'en-US' });
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
    await dismissDataConsent(page);
    await page.getByRole('button', { name: 'Library', exact: true }).click({ timeout: 15_000 });

    const loanTile = page.getByRole('button', { name: LOAN_GEAR, exact: true });
    const giveawayTile = page.getByRole('button', { name: GIVEAWAY_TILE_LABEL, exact: true });
    // Baseline: both shelves render.
    await expect(loanTile).toBeVisible({ timeout: 15_000 });
    await expect(giveawayTile).toBeVisible({ timeout: 15_000 });

    // Open the location/filter sheet from the header anchor and turn off the
    // "Giving" category (maps to AVAILABILITY_FOR_GIVEAWAY, filtered client-side).
    await page.getByRole('button', { name: 'Change the distance anchor' }).click({ timeout: 15_000 });
    const givingChip = page.getByRole('button', { name: 'Giving category' });
    await expect(givingChip).toBeVisible({ timeout: 15_000 });
    await givingChip.click();
    // Close the sheet (dismissible modal) so the re-rendered shelves are visible.
    await page.keyboard.press('Escape');

    // The giveaway gear is filtered out; the borrowable gear stays.
    await expect(giveawayTile).toHaveCount(0, { timeout: 15_000 });
    await expect(loanTile).toBeVisible();
  } finally {
    await ctx.close();
  }
});
