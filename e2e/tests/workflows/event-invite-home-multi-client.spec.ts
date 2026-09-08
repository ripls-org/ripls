// e2e/tests/workflows/event-invite-home-multi-client.spec.ts — proves the
// multi-user "invite individuals to an event" loop for #2492.
//
// What this spec proves
// ─────────────────────
//   1. A host can invite multiple individuals to an event (ShareItem with
//      member invitees → the event's per-item community).
//   2. Each invited individual sees the event on their HOME (Up-next) — a real
//      web client, landing on the Home tab — even before they respond. This is
//      the regression that motivated the test: invitees couldn't find the
//      event at all (server/services/portfolio/fetch.go + home_view.go).
//   3. The host can set an invitee's attendance status (SetExperienceMemberRSVP)
//      and that status is reflected in the invitee's home view
//      (Invited → Going → Maybe).
//   4. An invitee can change their own status through the production RSVP UI,
//      and the host — after a reload — sees it reflected in the roster.
//
// Status text is read via the GetHomeView RPC, not the DOM: Flutter Web with
// CanvasKit paints row subtitles to the canvas (only the row title is exposed
// in the accessibility tree), so the subtitle ("Invited"/"Going"/"Maybe") is
// not Playwright-queryable. The event's PRESENCE on home is asserted through
// the web UI (the up-next row's a11y label is its title).
//
// TODO(#2531): the host-sees-the-change step uses an explicit reload because
// the host's already-open page does not yet update live via
// StreamUserEvents. Drop the reload once cross-client streaming is proven.

import { test, expect } from '../../lib/fixtures.js';
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
import {
  HomeUpNextStatus,
  PortfolioService,
} from '../../gen/ripls/api/portfolio_pb.js';

const SPEC_SLUG = 'event-invite-home-multi-client';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) {
    throw new Error('E2E_BASE_URL not set; global-setup.ts populates it');
  }
  return url;
}

test('host invites individuals to an event; they see it on home; status syncs both ways', async ({
  browser,
}) => {
  const baseUrl = requireBaseUrl();

  // ── PRECONDITIONS (RPC seed; not part of the test body) ──────────
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'E2E Host' });
  const alice = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'E2E Alice' });
  const bob = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'E2E Bob' });

  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });

  // Future-dated so the event lands on the Up-next agenda (undated events
  // are excluded there).
  const eventTime = Math.floor(Date.now() / 1000) + 3 * 86400;
  const experience = await createExperience({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
    name: `Invite E2E ${Math.floor(Date.now() / 1000)}`,
    timeUnixSec: eventTime,
  });

  // Host invites Alice + Bob as individuals — ShareItem adds them to the
  // event's per-item community (no RSVP row yet → "invited, no reply").
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

  // Helper: the invitee's home Up-next entry for this event (server truth).
  const homeEntry = async (accessToken: string) => {
    const client = createTestClient(PortfolioService, {
      baseUrl,
      specSlug: SPEC_SLUG,
      accessToken,
    });
    const resp = await client.getHomeView({ timezone: 'UTC' });
    return resp.upNext.find((e) => e.contentId === experience.experienceId) ?? null;
  };


  // Helper: open a fresh web client on the Home tab for a seeded user, and
  // assert the invited event is on their Up-next agenda.
  const expectEventOnHome = async (user: SeededUser, slug: string) => {
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
    await expect(
      page.getByRole('button', { name: experience.name }),
      `${user.name} sees the invited event on their Home Up-next`,
    ).toBeVisible({ timeout: 25_000 });
    await ctx.close();
  };

  const hostExp = createTestClient(ExperienceService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });

  // ── 1. Both invitees SEE the event on their home (the regression) ─
  await expectEventOnHome(alice, 'alice');
  await expectEventOnHome(bob, 'bob');

  // Server truth: both are invited (no reply yet). The relationship is a
  // typed status; the rendered label lives in the client's ARB (#2835).
  expect((await homeEntry(alice.accessToken))?.status).toBe(
    HomeUpNextStatus.INVITED,
  );
  expect((await homeEntry(bob.accessToken))?.status).toBe(
    HomeUpNextStatus.INVITED,
  );

  // ── 2. Host sets Alice's status; her home view reflects it ────────
  await hostExp.setExperienceMemberRSVP({
    experienceId: experience.experienceId,
    memberUserId: alice.userId,
    intention: RSVPIntention.RSVP_INTENTION_YES,
  });
  await expect
    .poll(async () => (await homeEntry(alice.accessToken))?.status, {
      timeout: 10_000,
    })
    .toBe(HomeUpNextStatus.GOING);
  // The event stays on Alice's home after the host set her status.
  await expectEventOnHome(alice, 'alice-going');

  await hostExp.setExperienceMemberRSVP({
    experienceId: experience.experienceId,
    memberUserId: alice.userId,
    intention: RSVPIntention.RSVP_INTENTION_MAYBE,
  });
  await expect
    .poll(async () => (await homeEntry(alice.accessToken))?.status, {
      timeout: 10_000,
    })
    .toBe(HomeUpNextStatus.MAYBE);

  // ── 3. Bob changes his own status through the production RSVP UI ──
  // Bob is still "Invited", so the read shell shows the RSVP call-to-action.
  {
    const ctx = await newRecordingContext(browser);
    await injectAuth(ctx, {
      accessToken: bob.accessToken,
      refreshToken: bob.refreshToken,
      user: { id: bob.userId, name: bob.name },
      serverUrl: baseUrl,
    });
    const page = await ctx.newPage();
    await installRequestIdOverride(page, `${SPEC_SLUG}-bob-rsvp`);
    await page.goto(`${baseUrl}/event/${experience.experienceId}`);
    await dismissDataConsent(page);
    await expect(
      page.locator('[flt-semantics-identifier="web-event-screen"]'),
    ).toBeAttached({ timeout: 25_000 });
    await page.getByRole('button', { name: 'RSVP to this event' }).tap();
    await page.getByRole('button', { name: 'RSVP: Going' }).tap();
    await page.getByRole('button', { name: 'Send your RSVP' }).tap();
    // Server records Bob as going.
    await expect
      .poll(
        async () => {
          const resp = await hostExp.getExperience({
            id: experience.experienceId,
            communityId: community.communityId,
          });
          return resp.rsvps.find((r) => r.user?.id === bob.userId)?.intention;
        },
        { timeout: 10_000 },
      )
      .toBe(RSVPIntention.RSVP_INTENTION_YES);
    await ctx.close();
  }

  // Bob's own home now shows the event as going.
  await expect
    .poll(async () => (await homeEntry(bob.accessToken))?.status, {
      timeout: 10_000,
    })
    .toBe(HomeUpNextStatus.GOING);

  // ── 4. The host, after a reload, sees Bob in the roster ──────────
  // TODO(#2531): reload stands in for live StreamUserEvents updates.
  {
    const ctx = await newRecordingContext(browser);
    await injectAuth(ctx, {
      accessToken: host.accessToken,
      refreshToken: host.refreshToken,
      user: { id: host.userId, name: host.name },
      serverUrl: baseUrl,
    });
    const page = await ctx.newPage();
    await installRequestIdOverride(page, `${SPEC_SLUG}-host-roster`);
    await page.goto(`${baseUrl}/event/${experience.experienceId}`);
    await dismissDataConsent(page);
    await expect(
      page.locator('[flt-semantics-identifier="web-event-screen"]'),
    ).toBeAttached({ timeout: 25_000 });
    await page.getByRole('button', { name: 'View attendees' }).tap();
    await expect(
      page.getByRole('button', {
        name: "View E2E Bob's profile E2E Bob",
      }),
      'host roster shows Bob (going) after reload',
    ).toBeVisible({ timeout: 10_000 });
    await ctx.close();
  }
});
