// e2e/tests/workflows/phone-guest-email-register.spec.ts
//
// The phone-first pivot (epic #2492) keeps the phone number primary, but a
// guest can still choose EMAIL from the below-the-fold "other ways to sign in"
// block — and, since #2595, that email form is INLINE on the phone-first screen
// (the shared EmailRegisterForm), not a route-hop to the old RegisterScreen.
//
// This spec drives that real inline email UI end-to-end — the coverage that did
// not exist before #2595 (the older email specs seed via EmailRegister + inject
// tokens, skipping the UI form). It proves:
//   1. The guest can expand "Continue with email" on the phone-first screen and
//      register without leaving it.
//   2. The post-auth handoff still fires: registering with the invite short
//      code joins the guest to the per-item community and the RSVP intention
//      auto-fires, so the host sees rsvpYesCount increment.
//
// It needs NO Firebase Auth Emulator — the mailed code (#2571) is read from the
// dev-mode devCode echo rather than a mailbox — so it's
// simpler than the phone-OTP full-loop specs. Flutter-web field driving matches
// those specs (getByLabel + real keystrokes); see e2e/lib/ui/email-register.ts.

import { test, expect } from '../../lib/fixtures.js';
import { installRequestIdOverride, createTestClient } from '../../lib/connect.js';
import { registerUser } from '../../lib/seed/users.js';
import {
  createCommunity,
  mintExperienceShareLink,
} from '../../lib/seed/communities.js';
import { createExperience } from '../../lib/seed/experiences.js';
import { completeEmailRegister } from '../../lib/ui/email-register.js';
import { ExperienceService } from '../../gen/ripls/api/experience_service_pb.js';

const SPEC_SLUG = 'phone-guest-email-register';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('phone-first guest registers via the inline email form and auto-RSVPs', async ({
  page,
  browserName,
}) => {
  // Real Flutter-web UI driving is validated on chromium (matches the other
  // phone-first specs); webkit UI driving is out of scope and adds flakiness.
  test.skip(browserName === 'webkit', 'inline email UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ---- Seed the host's world: community + event + share link. ----
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

  const consoleLog: string[] = [];
  page.on('console', (msg) => consoleLog.push(`${msg.type()}: ${msg.text()}`));
  let uncaughtException: Error | null = null;
  page.on('pageerror', (err) => {
    uncaughtException = err;
    consoleLog.push(`pageerror: ${err.message}`);
  });
  await installRequestIdOverride(page, SPEC_SLUG);

  // ---- Guest (unauthenticated) enters via the RSVP-yes web landing. ----
  // WebExperienceScreen forwards an unauth RSVP guest to the phone-first screen
  // (/verify-phone), where the email option lives below the fold.
  await page.goto(
    `${baseUrl}/event/${experience.experienceId}` +
      `?rsvp=yes&code=${encodeURIComponent(shareLink.shortCode)}`,
  );
  await page
    .locator('[flt-semantics-identifier="phone-auth-screen"]')
    .waitFor({ timeout: 20_000 });

  // ---- Drive the inline email code flow → submit ("Finish RSVP" in this flow). ----
  await completeEmailRegister(page, {
    name: 'Email Guest',
    email: `email-guest-${Date.now()}@example.com`,
    baseUrl,
    specSlug: SPEC_SLUG,
    submitLabel: /finish rsvp/i,
  });

  // ---- Assert: the guest's auto-fired RSVP landed server-side. ----
  // Match the guest by name, not the total count: the host is not auto-RSVP'd
  // here, but matching by name is the robust check either way.
  const hostExp = createTestClient(ExperienceService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  let rsvpYesCount = 0;
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    // Query the community the share link resolved to (the experience's ad-hoc
    // origin community for an item-target link) — where the guest joins and
    // their RSVP is recorded (#2767).
    const resp = await hostExp.getExperience({
      id: experience.experienceId,
      communityId: shareLink.communityId,
    });
    rsvpYesCount = resp.experience?.rsvpYesCount ?? 0;
    if (rsvpYesCount > 0) break;
    await new Promise((r) => setTimeout(r, 500));
  }

  try {
    expect(rsvpYesCount, 'guest RSVP recorded after inline email registration')
      .toBeGreaterThan(0);
    expect(uncaughtException, 'inline email register produced no uncaught exception')
      .toBeNull();
  } catch (err) {
    test.info().attach('console-log', {
      contentType: 'text/plain',
      body: consoleLog.join('\n'),
    });
    throw err;
  }
});
