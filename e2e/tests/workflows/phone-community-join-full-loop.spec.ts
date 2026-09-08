// e2e/tests/workflows/phone-community-join-full-loop.spec.ts
//
// The phone-first pivot extended to plain community invites (#2875): a non-app
// guest opens a community invite link, sees the group on the web, registers via
// REAL Firebase phone OTP (Auth Emulator), and lands inside the community as a
// member — no app install anywhere in the loop.
//
// This is the acceptance test for #2875. Before it, a community invite was the
// one share-link flavor that dead-ended on an install-the-app card: an "Open
// Ripls App" button, a 2-second timeout, and store badges, with no way to
// continue in a browser at all. Events, gear, and requests already had their
// phone-first landings (phone-rsvp-full-loop, phone-gear-interest-full-loop,
// phone-request-offer-full-loop); this is the fourth.
//
// Unlike those three there is no separate action to auto-fire — on a community
// invite the join IS the action — so the postcondition is membership itself,
// asserted over RPC rather than from pixels: a screen that rendered but never
// joined would pass a screenshot check and fail the user.

import { test, expect } from '../../lib/fixtures.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity, mintCommunityInviteLink } from '../../lib/seed/communities.js';
import { completePhoneRegister } from '../../lib/ui/phone-register.js';

const SPEC_SLUG = 'phone-community-join-full-loop';
const GUEST_PHONE = '+15551234576';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('guest joins a community from an invite link via phone OTP (phone-first full loop)', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'phone-OTP UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ---- Host registers + creates a named community (RPC; off-camera). ----
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Aisha Rahman' });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Ferndale Tool Library',
    description: 'Neighbors sharing what they own.',
  });
  const invite = await mintCommunityInviteLink({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
  });

  const hostComm = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  const before = await hostComm.getCommunity({ id: community.communityId });
  expect(before.numMembers, 'host is the sole member before the guest joins').toBe(1);

  // ---- Guest opens the SSR community landing (real HTML from the Go server). ----
  await page.goto(`${baseUrl}/go/${invite.shortCode}`);
  // Dwell on the landing so the recorded video is easy for a human to follow.
  await page.waitForTimeout(1500);

  // The landing shows the group, its size, and who invited them — and NOT the
  // install-the-app card this replaced. The negative assertions are the
  // regression guard for #2875 itself: if the fork in HandleInvitePage ever
  // routes a community invite back to invite.html, these are what catch it.
  await expect(page.getByRole('heading', { name: 'Ferndale Tool Library' })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByText('Neighbors sharing what they own.')).toBeVisible();
  await expect(page.getByText('1 member')).toBeVisible();
  await expect(page.getByText(/Aisha/)).toBeVisible();
  await expect(page.getByText('Open Ripls App')).toHaveCount(0);
  await expect(page.getByText(/download the Ripls app/i)).toHaveCount(0);

  // The primary CTA is an <a> to /group/{id}?intent=join&code=… — the hand-off
  // into the Flutter web bundle that makes the loop phone-first.
  await page.getByRole('link', { name: /join ferndale tool library/i }).click();

  // ---- Phone-first register (the unit under test). ----
  // The community guest sees "Confirm your phone to join Ferndale Tool
  // Library" and finishes with the generic "Finish" button, which fires
  // PhoneRegister — joining the invite link's community server-side.
  await completePhoneRegister(page, {
    phone: GUEST_PHONE,
    name: 'Carlos Mendez',
    finishLabel: /^finish$/i,
  });

  // ---- Assert: the guest is a real member, over RPC. ----
  // Polled rather than asserted once: registration, the community join, and the
  // client's return to /group/{id} all settle asynchronously. No networkidle —
  // the authed bundle holds a long-lived stream that never lets it fire (#2867).
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
  expect(joined, 'guest "Carlos Mendez" joined the community after phone-OTP registration').toBe(
    true,
  );

  // ---- Assert: the guest lands *inside* the community, not on `/`. ----
  // The web community screen is located through the flt-semantics DOM tree
  // (docs/client/testing/semantics_identifiers.md), not by copy.
  await expect(page.locator('[flt-semantics-identifier="web-community-screen"]')).toBeVisible({
    timeout: 30_000,
  });
  await expect(page).toHaveURL(new RegExp(`/group/${community.communityId}`));

  // Let the community view settle on camera before the video ends.
  await page.waitForTimeout(2500);
});
