// e2e/tests/guest-rsvp-confirm.spec.ts — Phase-1 second scenario for
// #2162. Exercises the share-link RSVP handoff: a brand-new user
// arrives via `/event/{id}?rsvp=yes&code={shortCode}`, the bundle's
// WebEventRsvpHandoff auto-joins the community via the code AND
// auto-fires RSVPToExperience after authentication.
//
// What the spec proves:
//   1. The handoff chain is wired (register → AcceptInvitationLink
//      via short_code → RSVPToExperience) and runs to completion.
//   2. The host (a separate user) sees rsvpYesCount increment on the
//      experience.
//   3. The browser-side path produces no uncaught exceptions during
//      the auto-RSVP write.
//
// Why we skip the UI register form: the form lives on RegisterScreen
// and would need additional Semantics identifiers to be locator-able
// from Playwright. The handoff itself is the regression risk we care
// about for Phase 1; the UI form is exercised by the existing widget
// tests under app/test/. We programmatically register the visitor
// via EmailRegister, inject tokens, then navigate with the rsvp+code
// query params so the handoff fires identically to the
// real-flow-after-register branch.

import { test, expect } from '../lib/fixtures.js';
import { installRequestIdOverride } from '../lib/connect.js';
import { injectAuth } from '../lib/auth.js';
import { registerUser } from '../lib/seed/users.js';
import {
  createCommunity,
  mintExperienceShareLink,
} from '../lib/seed/communities.js';
import { createExperience } from '../lib/seed/experiences.js';
import { createTestClient } from '../lib/connect.js';
import { ExperienceService } from '../gen/ripls/api/experience_service_pb.js';

const SPEC_SLUG = 'guest-rsvp-confirm';

test('share-link visitor auto-joins + RSVPs after auth', async ({ page }) => {
  const baseUrl = requireBaseUrl();

  // Phase 1: seed the host's world.
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  const experience = await createExperience({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
  });
  const shareLink = await mintExperienceShareLink({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
    experienceId: experience.experienceId,
  });

  // Phase 2: mint a fresh non-member visitor. Programmatically via
  // EmailRegister — skipping the UI form. The visitor is NOT yet a
  // member of `community`; the handoff will join them via the
  // short_code.
  const guest = await registerUser({ baseUrl, specSlug: SPEC_SLUG });

  // Phase 3: inject the guest's auth-state so the bundle hydrates
  // authenticated and the handoff sees `rsvpIntention` from the URL
  // params.
  await injectAuth(page.context(), {
    accessToken: guest.accessToken,
    refreshToken: guest.refreshToken,
    user: { id: guest.userId, name: guest.name },
    serverUrl: baseUrl,
  });
  await installRequestIdOverride(page, SPEC_SLUG);

  // Phase 4: capture browser console + uncaught exceptions for the
  // failure-report attachment.
  const consoleLog: string[] = [];
  page.on('console', (msg) => consoleLog.push(`${msg.type()}: ${msg.text()}`));
  let uncaughtException: Error | null = null;
  page.on('pageerror', (err) => {
    uncaughtException = err;
    consoleLog.push(`pageerror: ${err.message}`);
  });

  // Phase 5: navigate with the rsvp+code params the share landing
  // would forward. The handoff fires on the first authenticated
  // render.
  const targetUrl =
    `${baseUrl}/event/${experience.experienceId}` +
    `?rsvp=yes&code=${encodeURIComponent(shareLink.shortCode)}`;
  await page.goto(targetUrl);
  await expect(
    page.locator('[flt-semantics-identifier="web-event-screen"]'),
  ).toBeAttached({ timeout: 20_000 });

  // Phase 6: poll the host's view of the experience until rsvpYesCount
  // increments past zero, which proves the join + RSVP succeeded
  // server-side. Bounded poll: 15 s total, 250 ms intervals — the
  // handoff is two RPCs, so a few seconds is the realistic ceiling.
  const hostExpClient = createTestClient(ExperienceService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  let rsvpYesCount = 0;
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    // Query the community the share link actually resolved to — for an
    // item-target link that's the experience's ad-hoc origin community, which
    // the guest joins via the handoff and where their RSVP is recorded (#2767).
    const resp = await hostExpClient.getExperience({
      id: experience.experienceId,
      communityId: shareLink.communityId,
    });
    rsvpYesCount = resp.experience?.rsvpYesCount ?? 0;
    if (rsvpYesCount > 0) break;
    await new Promise((r) => setTimeout(r, 250));
  }

  try {
    expect(rsvpYesCount, 'guest RSVP recorded server-side').toBeGreaterThan(0);
    expect(uncaughtException, 'handoff produced no uncaught exception').toBeNull();
  } catch (err) {
    test.info().attach('console-log', {
      contentType: 'text/plain',
      body: consoleLog.join('\n'),
    });
    throw err;
  }
});

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) {
    throw new Error(
      'E2E_BASE_URL not set; global-setup.ts is supposed to populate it',
    );
  }
  return url;
}
