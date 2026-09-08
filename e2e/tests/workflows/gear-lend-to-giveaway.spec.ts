// e2e/tests/workflows/gear-lend-to-giveaway.spec.ts
//
// #1152 "How change lend to giveaway?": the owner switches an item from lending
// to a giveaway from the item's own edit screen — the capability the reporter
// couldn't find. Two guards run here on every PR:
//
//   1. The happy path: seed a gear shared FOR_LOAN, open it, enter edit mode via
//      the manage menu, tap the Give-away segment, confirm the permanent-transfer
//      dialog, and assert (via GetGear) the item is now FOR_GIVEAWAY.
//   2. The safety lock: with a loan in progress the toggle is locked and shows
//      the "finish or cancel …" hint, and the item stays FOR_LOAN — the
//      server-side in-progress guard reaching the UI.
//
// The flip runs through the real Flutter Web UI; server truth is confirmed via
// the GetGear RPC so the assertion is anchored to the CommunityGear junction,
// not just the on-screen chrome.

import { test, expect } from '../../lib/fixtures.js';
import { createTestClient } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser, registerUserViaInvite } from '../../lib/seed/users.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { GearService } from '../../gen/ripls/api/gear_service_pb.js';
import { TransferService } from '../../gen/ripls/api/transfer_service_pb.js';
import { Availability } from '../../gen/ripls/api/gear_pb.js';

const SPEC_SLUG = 'gear-lend-to-giveaway';

test('owner switches an item from lending to a giveaway on the edit screen', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'the edit + toggle UI flow is validated on chromium only');
  test.setTimeout(180_000);
  const baseUrl = requireBaseUrl();

  // ── Seed (RPC): a host with one item shared for LOAN. ──
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Marcus Hall' });
  const gear = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Harry Potter 1000pc Puzzle',
    availability: Availability.FOR_LOAN,
  });

  const hostGear = createTestClient(GearService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });

  // Precondition: the item really starts as a loan.
  const before = await hostGear.getGear({ id: gear.gearId, communityId: gear.communityId });
  expect(before.availability, 'seeded as a loan').toBe(Availability.FOR_LOAN);

  const ctx = await newRecordingContext(browser);
  const page = await ctx.newPage();
  try {
    await injectAuth(ctx, {
      accessToken: host.accessToken,
      refreshToken: host.refreshToken,
      user: { id: host.userId, name: host.name },
      serverUrl: baseUrl,
    });
    await page.goto(`${baseUrl}/gear/${gear.gearId}`);

    // The hero resolving proves the item loaded for its owner.
    await expect(
      page.getByRole('img', { name: new RegExp(gear.name, 'i') }),
    ).toBeVisible({ timeout: 30_000 });

    // ── Owner opens the manage menu → Set details → edit mode. ──
    await page.getByRole('button', { name: /open manage menu/i }).click();
    await page.getByRole('button', { name: /set details/i }).click();

    // ── The Lend / Give-away toggle is now on screen. Flip to Give away. ──
    const giveAwaySegment = page.locator(
      '[flt-semantics-identifier="gear-availability-give-away"]',
    );
    await expect(giveAwaySegment).toBeVisible({ timeout: 15_000 });
    await giveAwaySegment.click();

    // ── Confirm the permanent-transfer dialog (its CTA wording is distinct from
    //    the "Give away" segment so this locator is unambiguous). ──
    await expect(page.getByText('Give this away?')).toBeVisible({ timeout: 10_000 });
    await page.getByRole('button', { name: 'Give it away', exact: true }).click();

    // ── RPC truth: the CommunityGear junction now reads FOR_GIVEAWAY. ──
    const after = await pollFor(async () => {
      const d = await hostGear.getGear({ id: gear.gearId, communityId: gear.communityId });
      return d.availability === Availability.FOR_GIVEAWAY ? d : undefined;
    }, 20_000);
    expect(after, 'the item became a giveaway after using the toggle').toBeTruthy();
  } finally {
    await ctx.close();
  }
});

test('the lend/give toggle is locked while a loan is in progress', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'one browser is enough for the guard-lock check');
  test.setTimeout(180_000);
  const baseUrl = requireBaseUrl();

  // ── Seed: a loan item plus a neighbour who has expressed interest, so a
  //    transfer is in progress for the item. ──
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Nadia Fox' });
  const gear = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Cordless Drill',
    availability: Availability.FOR_LOAN,
  });
  const borrower = await registerUserViaInvite({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'Theo Park',
    inviterAccessToken: host.accessToken,
    communityId: gear.communityId,
  });
  const borrowerTransfer = createTestClient(TransferService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: borrower.accessToken,
  });
  await borrowerTransfer.expressInterest({ gearId: gear.gearId });

  const hostGear = createTestClient(GearService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  const hostTransfer = createTestClient(TransferService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });

  // Confirm the in-progress transfer is visible to the owner server-side (the
  // exact signal the client's lock reads).
  const pending = await pollFor(async () => {
    const ctx = await hostTransfer.getGearTransferContext({
      gearId: gear.gearId,
      communityId: gear.communityId,
    });
    return (ctx.context?.pendingRequests ?? []).length > 0 ? ctx : undefined;
  }, 20_000);
  expect(pending, 'owner sees the pending interest server-side').toBeTruthy();

  const ctx = await newRecordingContext(browser);
  const page = await ctx.newPage();
  try {
    await injectAuth(ctx, {
      accessToken: host.accessToken,
      refreshToken: host.refreshToken,
      user: { id: host.userId, name: host.name },
      serverUrl: baseUrl,
    });
    await page.goto(`${baseUrl}/gear/${gear.gearId}`);
    await expect(
      page.getByRole('img', { name: new RegExp(gear.name, 'i') }),
    ).toBeVisible({ timeout: 30_000 });

    await page.getByRole('button', { name: /open manage menu/i }).click();
    await page.getByRole('button', { name: /set details/i }).click();

    const giveAway = page.getByRole('button', { name: 'Give away', exact: true });
    await expect(giveAway).toBeVisible({ timeout: 15_000 });

    // The guard has two layers: the client proactively locks the control when it
    // already knows a transfer is in progress, and the server rejects the flip
    // regardless. If the control isn't pre-locked, drive it through the server
    // path (tap → confirm → rejection).
    if (await giveAway.isEnabled()) {
      await giveAway.click();
      await expect(page.getByText('Give this away?')).toBeVisible({ timeout: 10_000 });
      await page.getByRole('button', { name: 'Give it away', exact: true }).click();
    }

    // Either way a user-visible guard message appears — the proactive lock hint
    // or the server's rejection toast — both name the in-progress loan/giveaway.
    await expect(
      page.getByText(/(in-progress loan or giveaway|loan or giveaway in progress)/i),
    ).toBeVisible({ timeout: 15_000 });

    // The invariant that matters: the item never left loan mode.
    await page.waitForTimeout(500);
    const still = await hostGear.getGear({ id: gear.gearId, communityId: gear.communityId });
    expect(still.availability, 'the in-progress guard keeps the item a loan').toBe(
      Availability.FOR_LOAN,
    );
  } finally {
    await ctx.close();
  }
});

async function pollFor<T>(
  fn: () => Promise<T | undefined>,
  timeoutMs = 15_000,
): Promise<T | undefined> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const v = await fn();
    if (v !== undefined) return v;
    await new Promise((r) => setTimeout(r, 250));
  }
  return undefined;
}

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts is supposed to populate it');
  return url;
}
