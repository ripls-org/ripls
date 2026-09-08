// e2e/tests/workflows/event-whos-in-counts.spec.ts
//
// The "Who's In" roster (#2492): the host creates an event through the web UI,
// invites a non-empty community via the share sheet, RPC-triggers a mix of
// yes / no / maybe RSVPs from that community's members, and then the roster is
// asserted to show the right per-section counts — Going / Maybe / Not going for
// the individual responders, plus a per-community "N haven't responded" count
// for the members who stayed silent (responders are subtracted from it).
//
// Per the project's UI-test principle, the *creation + invite* happen through
// the real Flutter Web UI; the RSVP fan-out is RPC (preparing state), and the
// final counts are read from the rendered roster.

import { test, expect } from '../../lib/fixtures.js';
import { installRequestIdOverride, createTestClient } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser, registerUserViaInvite } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createEventViaWebUI } from '../../lib/ui/create-event.js';
import {
  ExperienceService,
  RSVPIntention,
} from '../../gen/ripls/api/experience_service_pb.js';

const SPEC_SLUG = 'event-whos-in-counts';

type Member = Awaited<ReturnType<typeof registerUserViaInvite>>;

test("Who's In shows RSVP group counts + per-community non-responder count", async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'create + roster UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ── Seed (RPC): host + a community with four members ──
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'E2E Host' });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Trail Crew',
  });
  const members: Member[] = [];
  for (const name of ['Ada', 'Ben', 'Cy', 'Dot']) {
    members.push(
      await registerUserViaInvite({
        baseUrl,
        specSlug: SPEC_SLUG,
        name,
        inviterAccessToken: host.accessToken,
        communityId: community.communityId,
      }),
    );
  }

  const hostExp = createTestClient(ExperienceService, {
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

    // ── Host creates the event via the web UI, then adds the community ──
    await createEventViaWebUI(page, baseUrl);
    // The share sheet is open → Share to communities → pick Trail Crew → Confirm.
    await page.getByRole('button', { name: /share to communities/i }).click();
    await page.getByText(community.name, { exact: true }).first().click();
    await page.getByRole('button', { name: /^confirm$/i }).click();

    // ── Resolve the created event + wait for the community share to land ──
    let experienceId = '';
    const shareDeadline = Date.now() + 15_000;
    while (Date.now() < shareDeadline) {
      const mine = await hostExp.listMyExperiences({});
      const created = mine.experiences[0];
      if (created && created.sharedCommunityIds.includes(community.communityId)) {
        experienceId = created.id;
        break;
      }
      await new Promise((r) => setTimeout(r, 250));
    }
    expect(experienceId, 'event created + shared to the community').not.toBe('');

    // ── RPC: members RSVP yes / no / maybe; Dot stays silent ──
    const rsvp = async (member: Member, intention: RSVPIntention) => {
      const c = createTestClient(ExperienceService, {
        baseUrl,
        specSlug: SPEC_SLUG,
        accessToken: member.accessToken,
      });
      await c.rSVPToExperience({
        experienceId,
        communityId: community.communityId,
        intention,
      });
    };
    await rsvp(members[0], RSVPIntention.RSVP_INTENTION_YES); // Ada → going
    await rsvp(members[1], RSVPIntention.RSVP_INTENTION_NO); // Ben → not going
    await rsvp(members[2], RSVPIntention.RSVP_INTENTION_MAYBE); // Cy → maybe
    // members[3] (Dot) never responds → the community's lone non-responder.

    // ── Host opens the event → Who's In roster → assert the counts ──
    await page.goto(`${baseUrl}/event/${experienceId}`);
    await expect(
      page.locator('[flt-semantics-identifier="web-event-screen"]'),
    ).toBeAttached({ timeout: 20_000 });
    await page.getByRole('button', { name: 'View attendees' }).click();

    // Flat roster with per-row status pills (no section headers). The host can
    // change anyone's status, so each attendee row's pill carries a
    // "Change status, currently <X>" label. Going = host (auto-YES) + Ada = 2;
    // Maybe = Cy = 1; Not going = Ben = 1.
    await expect(
      page.getByRole('button', { name: 'Change status, currently Going' }),
    ).toHaveCount(2, { timeout: 10_000 });
    await expect(
      page.getByRole('button', { name: 'Change status, currently Maybe' }),
    ).toHaveCount(1);
    await expect(
      page.getByRole('button', { name: 'Change status, currently Not going' }),
    ).toHaveCount(1);
    // Trail Crew: 4 members − host − 3 responders (Ada/Ben/Cy) = 1 who hasn't
    // responded (Dot).
    await expect(page.getByText("1 hasn't responded")).toBeVisible();
  } finally {
    await ctx.close();
  }
});

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) {
    throw new Error('E2E_BASE_URL not set; global-setup.ts is supposed to populate it');
  }
  return url;
}
