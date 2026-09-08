// e2e/tests/ported-smoke.spec.ts — three foundational bundle-
// bootstrap checks. Each spec asserts the page lands where the
// GoRouter should send it and that the bundle didn't throw an
// uncaught exception while doing so.
//
// 1. /event/{fake-id}?rsvp=yes&code=… — unauth event-landing
//    render via the hardcoded all-zeros UUID. WebExperienceScreen
//    renders for an unknown experience ID because the unauth
//    placeholder path doesn't do any DB lookup, so this exercises
//    routing + PathUrlStrategy + Firebase init with zero seed.
// 2. /feed (no auth) — apex catch-all + SPA fallback + GoRouter
//    unauth redirect to /login. Pins the redirect.
// 3. /feed (authenticated) — apex catch-all + auth-state hydration.
//    Pins that the router did NOT bounce to /login.

import { test, expect } from '../lib/fixtures.js';
import { injectAuth } from '../lib/auth.js';
import { installRequestIdOverride } from '../lib/connect.js';
import { registerUser } from '../lib/seed/users.js';

const SPEC_SLUG = 'ported-smoke';
const FAKE_EVENT_ID = '00000000-0000-0000-0000-000000000000';

function trackPageErrors(page: import('@playwright/test').Page): {
  errors: Error[];
  consoleLog: string[];
} {
  const errors: Error[] = [];
  const consoleLog: string[] = [];
  page.on('pageerror', (err) => {
    errors.push(err);
    consoleLog.push(`pageerror: ${err.message}`);
  });
  page.on('console', (msg) => consoleLog.push(`${msg.type()}: ${msg.text()}`));
  return { errors, consoleLog };
}

test('/event/{fake-id}?rsvp=yes&code=… routes an unauth guest to the phone-first screen without throwing', async ({ page }) => {
  const baseUrl = requireBaseUrl();
  const { errors, consoleLog } = trackPageErrors(page);
  await installRequestIdOverride(page, SPEC_SLUG);

  // Phone-first onboarding (#2492): an unauthenticated guest arriving with an
  // RSVP intention is routed straight from the event landing to /verify-phone
  // (the "Confirm your phone to RSVP" screen), so the phone-auth anchor is what
  // attaches — not the web-event gate.
  await page.goto(`${baseUrl}/event/${FAKE_EVENT_ID}?rsvp=yes&code=CIDUMMY`);
  await expect(
    page.locator('[flt-semantics-identifier="phone-auth-screen"]'),
  ).toBeAttached({ timeout: 20_000 });

  try {
    expect(errors, 'unauth event landing produced no pageerror').toEqual([]);
  } catch (err) {
    test.info().attach('console-log', {
      contentType: 'text/plain',
      body: consoleLog.join('\n'),
    });
    throw err;
  }
});

test('/feed (unauth) redirects to /login without throwing', async ({ page }) => {
  const baseUrl = requireBaseUrl();
  const { errors, consoleLog } = trackPageErrors(page);
  await installRequestIdOverride(page, SPEC_SLUG);

  await page.goto(`${baseUrl}/feed`);
  // Bundle bootstrap + GoRouter resolution can take a few seconds;
  // give it room.
  await page.waitForURL(/\/login(\?|$)/, { timeout: 20_000 });

  try {
    expect(errors, 'unauth /feed produced no pageerror').toEqual([]);
  } catch (err) {
    test.info().attach('console-log', {
      contentType: 'text/plain',
      body: consoleLog.join('\n'),
    });
    throw err;
  }
});

test('/feed (authed) renders without bouncing to /login', async ({ page }) => {
  const baseUrl = requireBaseUrl();
  const { errors, consoleLog } = trackPageErrors(page);
  await installRequestIdOverride(page, SPEC_SLUG);

  // Mint a fresh user; inject auth state; navigate.
  const user = await registerUser({ baseUrl, specSlug: SPEC_SLUG });
  await injectAuth(page.context(), {
    accessToken: user.accessToken,
    refreshToken: user.refreshToken,
    user: { id: user.userId, name: user.name },
    serverUrl: baseUrl,
  });

  // Wait for an authenticated RPC to succeed rather than for the network to
  // fall idle. `networkidle` cannot be reached here: the app holds a long-lived
  // StreamUserEvents connection open for the whole session (#2867), so there is
  // always a request in flight. It only ever worked because this user is minted
  // with no communities, and the per-community streams it replaced were skipped
  // on an empty list — an accident of the fixture, not a property of the app.
  //
  // A 200 on an authed RPC is also the stronger signal for what this test
  // actually asserts: the token survived hydration, so there is nothing left to
  // bounce us to /login.
  const authedRpc = page.waitForResponse(
    (r) => r.url().includes('CommunityService/ListCommunities') && r.status() === 200,
    { timeout: 20_000 },
  );
  await page.goto(`${baseUrl}/feed`);
  await authedRpc;

  // The exact rendered URL after auth-state hydration can be `/feed`
  // or a sub-route, but it must NOT have bounced to /login.
  const finalPath = new URL(page.url()).pathname;

  try {
    expect(finalPath, 'authed /feed did not bounce to /login').not.toMatch(/^\/login/);
    expect(errors, 'authed /feed produced no pageerror').toEqual([]);
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
