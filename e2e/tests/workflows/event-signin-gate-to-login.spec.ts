// e2e/tests/workflows/event-signin-gate-to-login.spec.ts
//
// Event-flow unification (#2644 follow-up): web event guests now match gear/
// request. The primary path (an "I'm in"/"Maybe" tap → rsvp intent) goes
// phone-first to /verify-phone (covered by phone-rsvp-full-loop and
// phone-guest-email-register). The no-intent "Sign in to RSVP" gate — the only
// thing that used to route events to the separate RegisterScreen via
// /register?experience_id — now routes to /login, exactly like the gear/request
// web screens. This retires the last RegisterScreen.experienceId consumer.
//
// This spec proves the gate lands on /login (existing users sign in; new users
// register phone-first from there), NOT the retired /register path.

import { test, expect } from '../../lib/fixtures.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createExperience } from '../../lib/seed/experiences.js';

const SPEC_SLUG = 'event-signin-gate-to-login';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('the event "Sign in to RSVP" gate routes to /login, not /register', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'Flutter Web UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Gate Host' });
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

  let uncaughtException: Error | null = null;
  page.on('pageerror', (err) => {
    uncaughtException = err;
  });

  // Bare /event/{id} (no rsvp intent, no share code): an unauthenticated guest
  // gets the "Sign in to RSVP" auth gate, not the phone-first auto-RSVP path.
  await page.goto(`${baseUrl}/event/${experience.experienceId}`);
  await page
    .locator('[flt-semantics-identifier="web-event-screen"]')
    .waitFor({ timeout: 20_000 });

  await page.getByRole('button', { name: /sign in to rsvp/i }).click();

  // Post-unification the gate goes to /login (matching the gear/request web
  // screens), not the retired /register?experience_id path.
  await page.waitForURL(/\/login/, { timeout: 15_000 });
  expect(page.url(), 'sign-in gate routes to /login, not /register').not.toContain(
    '/register',
  );
  expect(uncaughtException, 'gate navigation produced no uncaught exception').toBeNull();
});
