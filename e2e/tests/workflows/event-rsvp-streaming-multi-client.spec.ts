// e2e/tests/workflows/event-rsvp-streaming-multi-client.spec.ts — proves that
// RSVP changes propagate to OTHER clients' already-open screens in real time
// via StreamUserEvents, with no reload (#2492 / #2531).
//
// Two directions, both asserted on a page that stays open the whole time:
//   1. Attendee → host: the host has the Who's-In roster open; an invitee
//      RSVPs through the production composer; the host's roster updates live.
//   2. Host → attendee: an invitee has the event open; the host sets that
//      invitee's RSVP (SetExperienceMemberRSVP); the invitee's open page
//      updates live (their "RSVP to this event" call-to-action disappears once
//      they have a status).
//
// Status pill text rides the accessibility tree as the host-only "Change
// status, currently <X>" label, which IS Playwright-queryable (unlike row
// subtitles painted to the CanvasKit canvas). Direction 2 keys off the
// presence/absence of the RSVP call-to-action button.

import { test, expect, type Page } from '../../lib/fixtures.js';
import { installRequestIdOverride, createTestClient } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { dismissDataConsent } from '../../lib/ui/consent.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser, type SeededUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createExperience } from '../../lib/seed/experiences.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import {
  ExperienceService,
  RSVPIntention,
} from '../../gen/ripls/api/experience_service_pb.js';

const SPEC_SLUG = 'event-rsvp-streaming-multi-client';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts populates it');
  return url;
}

test('RSVP changes stream to other clients without a reload', async ({ browser }) => {
  test.setTimeout(150_000); // three web clients + streaming waits
  const baseUrl = requireBaseUrl();

  // ── PRECONDITIONS (RPC seed) ─────────────────────────────────────
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'E2E Host' });
  const alice = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'E2E Alice' });
  const bob = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'E2E Bob' });

  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  const eventTime = Math.floor(Date.now() / 1000) + 3 * 86400;
  const experience = await createExperience({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
    name: `Stream E2E ${Math.floor(Date.now() / 1000)}`,
    timeUnixSec: eventTime,
  });

  const hostComm = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  await hostComm.shareItem({
    item: { case: 'experienceId', value: experience.experienceId },
    invitees: [
      { identity: { case: 'memberUserId', value: alice.userId } },
      { identity: { case: 'memberUserId', value: bob.userId } },
    ],
  });

  // Open the event detail by navigating through the authenticated home and
  // tapping the Up-next card. This is in-app navigation (no page reload), so the
  // event screen mounts with auth already in memory — avoiding the cold /event
  // deep-link race where the bundle's GetExperience can fire before auth loads
  // and get stuck on the guest "Sign in to RSVP" view.
  const openEvent = async (user: SeededUser, slug: string): Promise<Page> => {
    const ctx = await newRecordingContext(browser);
    await injectAuth(ctx, {
      accessToken: user.accessToken,
      refreshToken: user.refreshToken,
      user: { id: user.userId, name: user.name },
      serverUrl: baseUrl,
    });
    const page = await ctx.newPage();
    await installRequestIdOverride(page, `${SPEC_SLUG}-${slug}`);
    await page.goto(`${baseUrl}/`);
    await dismissDataConsent(page);
    const eventCard = page
      .getByRole('button', { name: experience.name })
      .first();
    await expect(
      eventCard,
      `${user.name} sees the event on their Home Up-next`,
    ).toBeVisible({ timeout: 25_000 });
    await eventCard.tap();
    await page.waitForTimeout(1_500); // let the in-app push settle
    return page;
  };

  // ── DIRECTION 1: attendee RSVP → host's open roster updates live ──
  const hostPage = await openEvent(host, 'host');
  await hostPage.getByRole('button', { name: 'View attendees' }).tap();
  // Initially only the host is "Going" (Alice + Bob are invited, no reply).
  await expect(
    hostPage.getByRole('button', { name: 'Change status, currently Going' }),
    'initially only the host is going',
  ).toHaveCount(1, { timeout: 15_000 });

  // Bob RSVPs Going through the production composer, in his own client.
  const bobPage = await openEvent(bob, 'bob');
  await bobPage.getByRole('button', { name: 'RSVP to this event' }).tap();
  await bobPage.getByRole('button', { name: 'RSVP: Going' }).tap();
  await bobPage.getByRole('button', { name: 'Send your RSVP' }).tap();

  // The host's roster — never reloaded — now shows two "Going" (host + Bob),
  // delivered over StreamUserEvents.
  await expect(
    hostPage.getByRole('button', { name: 'Change status, currently Going' }),
    "host's open roster reflects Bob's RSVP live (no reload)",
  ).toHaveCount(2, { timeout: 20_000 });

  // ── DIRECTION 2: host sets RSVP → attendee's open page updates live ─
  const alicePage = await openEvent(alice, 'alice');
  // Alice is invited with no reply → she sees the RSVP call-to-action.
  await expect(
    alicePage.getByRole('button', { name: 'RSVP to this event' }),
    'Alice (invited) sees the RSVP call-to-action',
  ).toBeVisible({ timeout: 15_000 });

  // The host sets Alice's RSVP to Going.
  const hostExp = createTestClient(ExperienceService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  await hostExp.setExperienceMemberRSVP({
    experienceId: experience.experienceId,
    memberUserId: alice.userId,
    intention: RSVPIntention.RSVP_INTENTION_YES,
  });

  // Alice's page — never reloaded — picks up her new status over the stream:
  // the call-to-action disappears once she has an RSVP.
  await expect(
    alicePage.getByRole('button', { name: 'RSVP to this event' }),
    "Alice's open page reflects the host-set RSVP live (no reload)",
  ).toHaveCount(0, { timeout: 20_000 });

  await hostPage.context().close();
  await bobPage.context().close();
  await alicePage.context().close();
});
