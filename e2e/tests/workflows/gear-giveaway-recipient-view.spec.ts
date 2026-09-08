// The recipient's view of a giveaway that has completed (#2695).
//
// This lived as the closing scene of the gear reel, which made a showcase the
// only thing standing between three shipped bugs and a re-run. Reels are not
// correctness gates (docs/walkthroughs.md); the clip was cut as redundant —
// the scene before it already shows the giveaway concluding — so the guard
// moved here, where it belongs and runs on every PR.
//
// All three faces of #2695 were about the person who RECEIVED the item, on
// the item's own page:
//   1. loading it at all — pre-fix the recipient got PermissionDenied;
//   2. the completed-giveaway card rendering for them;
//   3. the item's conversation still opening once the transfer is done.
import { resolve } from 'node:path';
import { test, expect } from '../../lib/fixtures.js';
import { TransferService } from '../../gen/ripls/api/transfer_service_pb.js';
import { Availability } from '../../gen/ripls/api/gear_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser, registerUserViaInvite } from '../../lib/seed/users.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { injectAuth } from '../../lib/auth.js';
import { newRecordingContext } from '../../lib/context.js';

const SPEC_SLUG = 'gear-giveaway-recipient-view';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set');
  return url;
}

test('a completed giveaway still opens for the person who received it', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'one browser is enough for a permission + render guard');
  test.setTimeout(180_000);
  const baseUrl = requireBaseUrl();

  // ---- Seed: Maya gives away a record player; Dana wants it. ----
  const maya = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Maya Chen' });
  const gear = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: maya.accessToken,
    name: 'Vintage Record Player',
    availability: Availability.FOR_GIVEAWAY,
    heroImagePath: resolve(__dirname, '..', '..', 'fixtures', 'walkthroughs', 'gear-record-player.jpg'),
  });
  const dana = await registerUserViaInvite({
    baseUrl,
    specSlug: SPEC_SLUG,
    inviterAccessToken: maya.accessToken,
    communityId: gear.communityId,
    name: 'Dana Rivera',
  });

  const transferAs = (token: string) =>
    createTestClient(TransferService, { baseUrl, specSlug: SPEC_SLUG, accessToken: token });

  const interest = await transferAs(dana.accessToken).expressInterest({ gearId: gear.gearId });
  const transferId = interest.transfer?.id;
  expect(transferId, 'ExpressInterest returned a transfer').toBeTruthy();

  // Maya picks Dana and hands it over.
  await transferAs(maya.accessToken).selectRecipient({
    transferId,
    recipientId: dana.userId,
  });
  await transferAs(maya.accessToken).completeTransfer({ transferId });

  // ---- Dana opens the item she received. ----
  const ctx = await newRecordingContext(browser);
  const page = await ctx.newPage();
  try {
    await injectAuth(ctx, {
      accessToken: dana.accessToken,
      refreshToken: dana.refreshToken,
      user: { id: dana.userId, name: dana.name },
      serverUrl: baseUrl,
    });
    await page.goto(`${baseUrl}/gear/${gear.gearId}`);

    // Faces 1 and 2. That the hero resolves at all proves the fetch was
    // authorized — pre-fix the recipient got PermissionDenied. Its accessible
    // name is the merged item header, which is where the recipient reads that
    // the giveaway concluded and that the item is theirs.
    const hero = page.getByRole('img', { name: new RegExp(gear.name, 'i') });
    await expect(hero).toBeVisible({ timeout: 30_000 });
    await expect(hero).toHaveAccessibleName(/giveaway completed/i);
    await expect(hero).toHaveAccessibleName(/received it/i);
    await expect(hero).toHaveAccessibleName(new RegExp(dana.name, 'i'));

    // Face 3: the item's conversation still opens once the transfer is done —
    // the story stays on the item.
    await page.getByRole('button', { name: /open conversation/i }).click();
    await expect(page.getByRole('button', { name: /send message/i })).toBeVisible({
      timeout: 20_000,
    });
  } finally {
    await ctx.close();
  }
});
