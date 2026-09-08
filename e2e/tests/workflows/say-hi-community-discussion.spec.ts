// e2e/tests/workflows/say-hi-community-discussion.spec.ts
//
// The "say hi" link lands a member in the community discussion (#2876).
//
// A member-joined notification reads "{someone} joined {community}" with a
// "Say hi at {link}" CTA. That link is a community-invite `/go/{code}` — so
// before this, the CTA promising a conversation opened an invitation page,
// addressed to someone who had been a member for weeks. The fix is a closed
// destination hint (`?to=discuss`) that the SSR landing reads to drop the
// invitation framing and thread `tab=discuss` into the /group hand-off.
//
// Both halves are asserted here because they fail independently: the landing
// can render member copy while the hand-off drops the tab, and vice versa.

import { test, expect } from '../../lib/fixtures.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity, mintCommunityInviteLink } from '../../lib/seed/communities.js';
import { injectAuth } from '../../lib/auth.js';

const SPEC_SLUG = 'say-hi-community-discussion';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('the say-hi landing addresses a member, not an invitee', async ({ page }) => {
  const baseUrl = requireBaseUrl();

  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Aisha Rahman' });
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

  // The link shape the member-joined notification builds.
  await page.goto(`${baseUrl}/go/${invite.shortCode}?to=discuss`);

  await expect(page.getByRole('heading', { name: 'Ferndale Tool Library' })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByRole('link', { name: /open the discussion/i })).toBeVisible();

  // The invitation framing is gone — this reader is already in the group.
  await expect(page.getByText(/invited you/i)).toHaveCount(0);
  await expect(page.getByRole('link', { name: /join the group/i })).toHaveCount(0);

  // And the same link WITHOUT the hint still renders the ordinary invite, so
  // the hint is what changed the page rather than the page changing for
  // everyone.
  await page.goto(`${baseUrl}/go/${invite.shortCode}`);
  await expect(page.getByRole('link', { name: /join ferndale tool library/i })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByText(/invited you/i)).toBeVisible();
});

test('a member following the say-hi link lands in the discussion', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'Flutter Web bundle flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Aisha Rahman' });
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

  // The recipient of a "say hi" notification is an existing member by
  // construction — that is the whole reason the invite framing was wrong.
  const member = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Sam Rivera' });
  const memberComm = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: member.accessToken,
  });
  await memberComm.acceptInvitationLink({ shortCode: invite.shortCode });

  await injectAuth(page, {
    accessToken: member.accessToken,
    refreshToken: member.refreshToken,
    user: { id: member.userId, name: member.name },
    serverUrl: baseUrl,
  });

  await page.goto(`${baseUrl}/go/${invite.shortCode}?to=discuss`);
  await page.getByRole('link', { name: /open the discussion/i }).click();

  // The hand-off carries the tab, not just the community.
  await expect(page).toHaveURL(/\/group\/[^?]+\?.*tab=discuss/, { timeout: 30_000 });

  // The conversation panel is open on arrival — the promise the notification
  // made. Asserting on the community screen alone would pass even if the panel
  // never opened, and a bare textbox locator would match any field on the
  // profile, so this keys on the panel's own semantics identifier. No
  // networkidle: the authed bundle holds a long-lived stream that never lets
  // it fire (#2867).
  await expect(
    page.locator('[flt-semantics-identifier="web-community-screen"]'),
  ).toBeVisible({ timeout: 30_000 });
  await expect(
    page.locator('[flt-semantics-identifier="community-conversation-panel"]'),
  ).toBeVisible({ timeout: 30_000 });
});
