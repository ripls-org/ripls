// e2e/tests/workflows/gear-authoring-share.spec.ts
//
// The gear authoring + sharing loop (#2492 "Shared with"): the host creates a
// gear through the REAL unified-create UI, invites two people from the share
// sheet that auto-opens after Save, lands directly in the new gear (no reload),
// inspects the "Shared with" card (→ access sheet), then adds a whole community
// through the card's Invite button. The non-RSVP analog of the event Who's-In
// authoring flow, exercising the gear SharedWithCard + AccessSheet built in
// finish_pivot.
//
// Creation + inviting + sharing + inspection run through the real Flutter Web
// UI; the audience is also confirmed via RPC (GetGear) so the card's counts are
// anchored to server truth.

import { test, expect } from '../../lib/fixtures.js';
import { installRequestIdOverride, createTestClient } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser, registerUserViaInvite } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createGearViaWebUI } from '../../lib/ui/create-item.js';
import { GearService } from '../../gen/ripls/api/gear_service_pb.js';

const SPEC_SLUG = 'gear-authoring-share';

test('host creates a gear, invites people + a community, and inspects the Shared with panel', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'create + share UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ── Seed (RPC): host, a named community to add later (Maple Street, with one
  // member), and two people the host invites individually (in a second shared
  // community so the invite sheet lists them). ──
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Priya Nair' });
  const neighbors = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Maple Street Neighbors',
  });
  await registerUserViaInvite({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'Wes Carter',
    inviterAccessToken: host.accessToken,
    communityId: neighbors.communityId,
  });
  const toolLibrary = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Tool Library',
  });
  for (const name of ['Sam Okafor', 'Elena Petrova']) {
    await registerUserViaInvite({
      baseUrl,
      specSlug: SPEC_SLUG,
      name,
      inviterAccessToken: host.accessToken,
      communityId: toolLibrary.communityId,
    });
  }

  const hostGear = createTestClient(GearService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });

  const ctx = await newRecordingContext(browser);
  const page = await ctx.newPage();
  try {
    await injectAuth(ctx, {
      accessToken: host.accessToken,
      refreshToken: host.refreshToken,
      user: { id: host.userId, name: host.name },
      serverUrl: baseUrl,
    });
    await installRequestIdOverride(page, SPEC_SLUG);

    // ── Host creates the gear via the web UI; the share sheet auto-opens. ──
    await createGearViaWebUI(page, baseUrl, {
      prompt: 'Lending out my pressure washer to neighbors this spring',
    });

    // ── Invite two individuals from the auto-opened share sheet. After the
    // invite, the app drops straight into the new gear — no reload. ──
    await page.getByRole('button', { name: 'Invite people', exact: true }).click();
    await invitePerson(page, 'Sam Okafor');
    await invitePerson(page, 'Elena Petrova');
    await page.getByRole('button', { name: /^invite 2$/i }).click();

    // ── The "Shared with" card is now on the gear read shell: Sam + Elena = 2
    // people the host shared with. The count base is the sharee set — the owner
    // is not someone you share with, and the who's-helping panel subtracts from
    // this same base (#2724). (The whole card is one button whose a11y name
    // carries the count + header.) ──
    const card = page.getByRole('button', { name: /shared with/i });
    await expect(card).toHaveAccessibleName(/shared with 2 people/i, { timeout: 30_000 });

    // Resolve the gear by the invite landing (mock AI titles aren't the prompt).
    const gearId = await pollFor(async () => {
      const mine = await hostGear.listUserGear({});
      for (const g of mine.items) {
        const detail = await hostGear.getGear({ id: g.id });
        const names = detail.invitedIndividuals.map((u) => u.name);
        if (names.includes('Sam Okafor') && names.includes('Elena Petrova')) return g.id;
      }
      return undefined;
    }, 25_000);
    expect(gearId, 'gear created + Sam/Elena invited as individuals').toBeTruthy();

    // ── Open the access sheet ("Shared with"): it lists the invited people by
    // name (the item's own ad-hoc community), not a faceless aggregate. ──
    await card.click();
    await expect(page.getByText('Shared with')).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('Sam')).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('Elena')).toBeVisible();
    await expect(page.getByText(/other communit/i)).toHaveCount(0);

    // ── Add a whole community from the open panel: Invite Someone → share sheet
    // → Share to communities → Maple Street → Confirm. ──
    await page.getByRole('button', { name: /invite someone/i }).click();
    await page.getByRole('button', { name: /share to communities/i }).click();
    await page.getByText('Maple Street Neighbors', { exact: true }).first().click();
    await page.getByRole('button', { name: /^confirm$/i }).click();

    // ── The community share lands (host + Sam + Elena + Wes = 4 distinct)… ──
    const afterShare = await pollFor(async () => {
      const d = await hostGear.getGear({ id: gearId! });
      const names = d.sharedCommunities.map((c) => c.communityName);
      return names.includes('Maple Street Neighbors') ? d : undefined;
    });
    expect(afterShare, 'gear shared to Maple Street Neighbors').toBeTruthy();

    // ── …and the STILL-OPEN panel updates in place to show it (no reopen). ──
    await expect(page.getByText('Maple Street Neighbors')).toBeVisible({ timeout: 10_000 });
    await page.waitForTimeout(800); // let the final panel settle on camera

    // ── RPC truth: invited individuals + community + the deduped count. ──
    const detail = await hostGear.getGear({ id: gearId! });
    const invited = detail.invitedIndividuals.map((u) => u.name).sort();
    expect(invited, 'Sam + Elena are invited individuals on the gear').toEqual([
      'Elena Petrova',
      'Sam Okafor',
    ]);
    expect(detail.sharedCommunities.map((c) => c.communityName)).toContain(
      'Maple Street Neighbors',
    );
    expect(detail.totalDistinctMemberCount).toBe(4);
  } finally {
    await ctx.close();
  }
});

/** Tap a person in the InviteMembersSheet quick-add grid. The grid labels each
 * face with the person's FIRST name (e.g. "Sam"), so we match that. After the
 * tap, a selected chip carrying the person's FULL name appears — assert it to
 * (a) confirm the RIGHT person was selected (the grid reflows as people are
 * added, and a stale semantic node could otherwise select a neighbour), and
 * (b) let the grid settle before the next tap. */
async function invitePerson(page: import('@playwright/test').Page, fullName: string): Promise<void> {
  const firstName = fullName.split(' ')[0];
  await page
    .getByRole('button', { name: new RegExp(`\\b${firstName}\\b`) })
    .first()
    .click();
  await expect(page.getByText(fullName, { exact: true })).toBeVisible({ timeout: 10_000 });
}

async function pollFor<T>(fn: () => Promise<T | undefined>, timeoutMs = 15_000): Promise<T | undefined> {
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
