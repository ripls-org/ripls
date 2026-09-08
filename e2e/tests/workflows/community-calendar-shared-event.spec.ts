// e2e/tests/workflows/community-calendar-shared-event.spec.ts
//
// #2675 / #2677 regression — an event shared into a real, named community must
// appear on THAT community's calendar (the Workshop "On the calendar" → library
// calendar destination), even though every event is also born into its own
// nameless ad-hoc per-item community (#2492) and is therefore shared into two
// communities at once.
//
// The bug: the home-view calendar entry was attributed to a single community_id
// chosen from an unordered set, so it frequently landed on the event's ad-hoc
// backing community. The community-scoped calendar filters by community, so the
// event vanished from the real community's calendar ("Nothing planned yet") even
// though it showed on the unfiltered inbox calendar. The fix orders the set
// named-first and carries the full community_ids on the entry, which the
// community calendar intersects against. It also opens the calendar on the
// soonest planned day rather than an empty "today".
//
// See docs/client/calendar.md. Preconditions are seeded via RPC; the assertion
// runs against the real Flutter Web UI (People tab → the community's profile
// → its "Plans" action, the #2634 dock-era replacement for the old Workshop
// tab's "Open the library calendar" destination).

import { test, expect } from '../../lib/fixtures.js';
import { installRequestIdOverride } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createExperience } from '../../lib/seed/experiences.js';

const SPEC_SLUG = 'community-calendar-shared-event';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts populates it');
  return url;
}

test('event shared into a named community shows on that community calendar', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'Workshop calendar UI validated on chromium only');
  const baseUrl = requireBaseUrl();

  // Host owns a real, named community and hosts a dated event shared into it.
  // createExperience (SaveExperience) also provisions the event's own nameless
  // ad-hoc per-item community and shares the event there too — so the event is
  // in two communities, the exact #2675 condition.
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Leslie Host' });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Travis Heights Early Adopters',
  });
  // ~44 days out, a specific time — mirrors the report and stays comfortably
  // in the future regardless of when the suite runs.
  const eventUnixSec = Math.floor(Date.now() / 1000) + 44 * 24 * 60 * 60;
  await createExperience({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
    name: 'Neighborhood Craft Circle',
    description: 'Come join our friendly craft circle! All ages and skill levels.',
    timeUnixSec: eventUnixSec,
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

    await page.goto(`${baseUrl}/`);
    // A fresh user gets the Data Consent modal once CanvasKit boots; it blocks
    // every later tap, so wait for it and dismiss it (auto-waiting click).
    await page
      .getByRole('button', { name: /accept all/i })
      .click({ timeout: 30_000 })
      .catch(() => {});

    // People tab → the community's row → its profile's "Plans" action
    // (calendar_month icon) — opens the shared calendar in place via the
    // morph-reveal panel.
    await page.getByRole('button', { name: /^people$/i }).click();
    await page
      .getByRole('button', { name: 'Travis Heights Early Adopters' })
      .click({ timeout: 15_000 });
    await page.getByRole('button', { name: /^plans$/i }).first().click({ timeout: 15_000 });

    // The event must be present on the community's calendar. Before the fix this
    // day showed "Nothing planned yet". The calendar also auto-opens on the
    // event's day (not an empty "today"), so the event surfaces without paging.
    await expect(
      page.getByRole('button', { name: /Neighborhood Craft Circle/ }).first(),
    ).toBeVisible({ timeout: 15_000 });
  } finally {
    await ctx.close();
  }
});
