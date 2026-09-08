// e2e/tests/workflows/people-directory.spec.ts
//
// Maps to: the People tab (DirectoryScreen, #2634). No 1:1 workflow doc; this
// is a directory-list regression.
//
// What this spec proves (UI-integration failure class RPC tests can't):
//   - The People tab MERGES two independent RPC reads into one rendered list:
//     the viewer's communities (CommunityService.ListCommunities) AND the
//     connected people (PortfolioService.GetDirectoryPeople). It
//     asserts BOTH of the viewer's communities render as rows (aggregation)
//     and a co-member who owns shared activity renders as a PERSON row — the
//     directoryPeopleProvider path no community-only spec touches.
//
// What this spec does NOT prove:
//   - Community-row → community-profile routing (covered by
//     community-calendar-shared-event.spec.ts) or the nameless-group "Name"
//     chip / promote flow (covered by community-promote-nudge.spec.ts).
//   - Sort/filter, counts, or the empty state (counts + empty string are
//     canvas Text, not reliably reachable).
//
// A person row requires the co-member to own visible shared activity — a bare
// invite raises member_count but emits no DailyPerson (server buildCommunityPeople).
// So the co-member shares gear into the community.
//
// Preconditions seeded via RPC; navigation is a real UI gesture.

import { test, expect } from '../../lib/fixtures.js';
import { createTestClient, installRequestIdOverride } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { dismissDataConsent } from '../../lib/ui/consent.js';
import { registerUser, registerUserViaInvite } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { Availability } from '../../gen/ripls/api/gear_pb.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';

const SPEC_SLUG = 'people-directory';

const COMMUNITY_A = 'Maple Grove Circle';
const COMMUNITY_B = 'Cedar Hollow Collective';
const CO_MEMBER = 'Ada Rivera';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts populates it');
  return url;
}

test('People tab merges the viewer\'s communities and a connected person into one list', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'People directory validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ── PRECONDITIONS (RPC seed) ─────────────────────────────────────────────
  const viewer = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Priya Viewer' });
  const communityA = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: viewer.accessToken,
    name: COMMUNITY_A,
  });
  const communityB = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: viewer.accessToken,
    name: COMMUNITY_B,
  });

  // A co-member of community A. The invite alone makes them a member but emits
  // no person row — so they also share gear into A, which surfaces them as the
  // owner of visible shared activity (a DailyPerson in the viewer's inbox view).
  const ada = await registerUserViaInvite({
    baseUrl,
    specSlug: SPEC_SLUG,
    inviterAccessToken: viewer.accessToken,
    communityId: communityA.communityId,
    name: CO_MEMBER,
  });
  const adaGear = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: ada.accessToken,
    name: 'Aluminum Extension Ladder',
    availability: Availability.FOR_LOAN,
  });
  const adaCommunityClient = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: ada.accessToken,
  });
  await adaCommunityClient.shareItem({
    item: { case: 'gearId', value: adaGear.gearId },
    shareToCommunityIds: [communityA.communityId],
  });

  // ── BROWSER CONTEXT + AUTH ───────────────────────────────────────────────
  const ctx = await newRecordingContext(browser, { locale: 'en-US' });
  const page = await ctx.newPage();
  try {
    await injectAuth(ctx, {
      accessToken: viewer.accessToken,
      refreshToken: viewer.refreshToken,
      user: { id: viewer.userId, name: viewer.name },
      serverUrl: baseUrl,
    });
    await installRequestIdOverride(page, SPEC_SLUG);
    await page.goto(`${baseUrl}/`);
    await dismissDataConsent(page);

    // ── STEP: open the People tab ───────────────────────────────────────────
    await page.getByRole('button', { name: /^people$/i }).click({ timeout: 15_000 });

    // ── ASSERT: both communities aggregate onto the tab as rows ─────────────
    await expect(
      page.getByRole('button', { name: COMMUNITY_A, exact: true }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(
      page.getByRole('button', { name: COMMUNITY_B, exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    // ── ASSERT: the connected co-member renders as a PERSON row ─────────────
    // (the ListCommunities + GetDirectoryPeople merge).
    await expect(
      page.getByRole('button', { name: CO_MEMBER, exact: true }),
    ).toBeVisible({ timeout: 15_000 });
  } finally {
    await ctx.close();
  }
});
