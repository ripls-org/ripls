// e2e/tests/workflows/invite-link-deep-link-paths.spec.ts
//
// Deep-link entry-point coverage for the invite-link query-param cutover
// (#2562 / #2644). The typed gear/request/event `/go/{code}` landings are
// covered by the phone-first full-loops; this spec fills the gaps:
//
//   1. Bare community `/go/{code}` renders the community SSR landing (#2875 —
//      the phone-first one, not the install-only card it replaced; the full
//      join loop is phone-community-join-full-loop.spec.ts).
//   2. A LEGACY decorated `/go/{code}?gear_id=…` on a community invite still
//      renders the plain community landing — the query param is ignored, no
//      item preview, no error (graceful degradation of the cutover).
//   3. A LEGACY decorated `/go/{code}?gear_id=…` on a TYPED gear share link
//      renders the gear landing for the ROW's target — the row wins, the
//      query param is inert.
//   4. An authenticated non-member who opens `/invite?token={code}` for a bare
//      community invite joins the community (the client /invite path resolves
//      the target from the row via CheckInvitation and no longer reads item
//      ids from the URL).
//
// Tests 1-3 assert against the server-rendered HTML (real DOM, not CanvasKit);
// test 4 drives the Flutter Web bundle and asserts membership via RPC.

import { test, expect } from '../../lib/fixtures.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser } from '../../lib/seed/users.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import {
  createCommunity,
  mintCommunityInviteLink,
  mintGearShareLink,
} from '../../lib/seed/communities.js';
import { injectAuth } from '../../lib/auth.js';

const SPEC_SLUG = 'invite-link-deep-link-paths';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('bare community /go/{code} renders the community-invite SSR landing', async ({
  page,
}) => {
  const baseUrl = requireBaseUrl();

  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Ferndale Host' });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Ferndale Tool Library',
  });
  const invite = await mintCommunityInviteLink({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
  });

  // Assertions are against the server-rendered HTML. (Before #2875 this ran in
  // a hand-rolled JS-disabled context, because the install-only landing
  // auto-navigated to ripls:// on a mobile UA and stalled the load. The
  // community landing has no auto-open, so the standard `page` fixture works —
  // which also means this test gets recorded like every other one.)
  await page.goto(`${baseUrl}/go/${invite.shortCode}`);

  // The community landing: the group name as the hero heading, the inviter
  // row, the member count, and the join CTA. (No item preview — this is a
  // plain community invite.)
  await expect(page.getByRole('heading', { name: 'Ferndale Tool Library' })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByText(/invited you/i)).toBeVisible();
  await expect(page.getByRole('link', { name: /join ferndale tool library/i })).toBeVisible();
  // The community landing never carries an item "Ask to borrow" CTA.
  await expect(page.getByRole('link', { name: /ask .* to borrow/i })).toHaveCount(0);
});

test('legacy decorated /go/{code}?gear_id= on a community invite still renders the community landing', async ({
  page,
}) => {
  const baseUrl = requireBaseUrl();

  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Decorated Host' });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Maple Street Collective',
  });
  const invite = await mintCommunityInviteLink({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
  });
  // A real gear id to decorate the URL with — the way a legacy
  // GetOrCreateInviteLink URL would have carried it.
  const gear = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Impact Wrench',
  });

  await page.goto(`${baseUrl}/go/${invite.shortCode}?gear_id=${gear.gearId}`);

  // The legacy query param is ignored: still the plain community landing,
  // and crucially NOT a gear item preview.
  await expect(page.getByRole('heading', { name: 'Maple Street Collective' })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByRole('link', { name: /join maple street collective/i })).toBeVisible();
  await expect(page.getByText(/Impact Wrench/)).toHaveCount(0);
  await expect(page.getByRole('link', { name: /ask .* to borrow/i })).toHaveCount(0);
});

test('legacy decorated /go/{code}?gear_id= on a typed gear link renders the row target, ignoring the param', async ({
  page,
}) => {
  const baseUrl = requireBaseUrl();

  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Dana Lin' });
  const gear = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Cordless Drill',
  });
  const shareLink = await mintGearShareLink({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: gear.communityId,
    gearId: gear.gearId,
  });

  // Decorate the typed gear link with a bogus gear_id query param. The
  // landing must come from the ShareLink row's target (Cordless Drill),
  // not the query param.
  await page.goto(`${baseUrl}/go/${shareLink.shortCode}?gear_id=bogus-gear-id-9999`);

  // The gear landing renders for the row's gear, with its borrow CTA.
  await expect(page.getByText(/Cordless Drill/)).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole('link', { name: /ask .* to borrow/i })).toBeVisible();
});

test('authenticated non-member joins a community via /invite?token={code}', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'Flutter Web bundle flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Riverside Host' });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Riverside Menders',
  });
  const invite = await mintCommunityInviteLink({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
  });

  // A fresh authenticated user who is NOT yet a member.
  const invitee = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Sam Rivera' });

  const hostComm = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  const before = await hostComm.getCommunity({ id: community.communityId });
  expect(before.numMembers, 'host is the sole member before the invitee joins').toBe(1);

  // Open the bare invite in the authed bundle. The /invite route resolves the
  // target from the row (CheckInvitation), joins via AcceptInvitationLink, and
  // lands home — no item-id query params involved.
  await injectAuth(page, {
    accessToken: invitee.accessToken,
    refreshToken: invitee.refreshToken,
    user: { id: invitee.userId, name: invitee.name },
    serverUrl: baseUrl,
  });
  await page.goto(`${baseUrl}/invite?token=${invite.shortCode}`);

  // The join is fired by the bundle; poll membership from the host's view.
  let joined = false;
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const now = await hostComm.getCommunity({ id: community.communityId });
    if (now.numMembers === 2) {
      joined = true;
      break;
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  expect(joined, 'invitee "Sam Rivera" joined via the bare /invite deep link').toBe(true);

  // And the community surfaces in the invitee's own list.
  const inviteeComm = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: invitee.accessToken,
  });
  const mine = await inviteeComm.listCommunities({});
  expect(
    mine.communities.map((c) => c.id),
    'invitee sees the joined community in her list',
  ).toContain(community.communityId);
});
