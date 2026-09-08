// e2e/tests/workflows/phone-gear-email-register.spec.ts
//
// Sibling of phone-guest-email-register.spec.ts (which covers the EVENT/RSVP
// email path). This covers the GEAR guest choosing EMAIL instead of phone OTP.
//
// The point: a gear/request web guest is routed to the phone-first screen
// (/verify-phone), where — since #2595 — the email form is INLINE (never a
// route-hop to the old RegisterScreen). The post-auth handoff runs through
// PhoneAuthScreen's own gearId (carried on /verify-phone?gear_id=…) into
// postRegistrationDestination's kIsWeb `/item/{id}` branch, so ExpressInterest
// auto-fires. This is the path that makes RegisterScreen.gearId/requestId
// removable (#2644): gear/request guests never touch RegisterScreen, for phone
// OR email/Google.
//
// Needs NO Firebase Auth Emulator. Email sign-up is a mailed one-time code
// since #2571; the helper reads it from the dev-mode devCode echo, so the real
// UI is driven and only the mailbox is skipped.

import { test, expect } from '../../lib/fixtures.js';
import { installRequestIdOverride, createTestClient } from '../../lib/connect.js';
import { TransferService } from '../../gen/ripls/api/transfer_service_pb.js';
import { registerUser } from '../../lib/seed/users.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { mintGearShareLink } from '../../lib/seed/communities.js';
import { completeEmailRegister } from '../../lib/ui/email-register.js';

const SPEC_SLUG = 'phone-gear-email-register';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('phone-first gear guest registers via the inline email form and auto-expresses interest', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'inline email UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ---- Host shares a gear for loan (RPC; off-camera). ----
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Marcus Bell' });
  const gear = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Cordless Drill',
  });
  const shareLink = await mintGearShareLink({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: gear.communityId,
    gearId: gear.gearId,
  });

  const consoleLog: string[] = [];
  page.on('console', (msg) => consoleLog.push(`${msg.type()}: ${msg.text()}`));
  let uncaughtException: Error | null = null;
  page.on('pageerror', (err) => {
    uncaughtException = err;
    consoleLog.push(`pageerror: ${err.message}`);
  });
  await installRequestIdOverride(page, SPEC_SLUG);

  // ---- Guest (unauthenticated) enters the gear action flow. ----
  // WebItemActionScreen forwards an unauth guest with an action intent to the
  // phone-first screen (/verify-phone?gear_id=…), where email lives below the
  // fold. (The SSR CTA builds exactly this /item URL; navigating directly is the
  // same entry, matching phone-guest-email-register's /event goto.)
  await page.goto(
    `${baseUrl}/item/${gear.gearId}` +
      `?intent=interest&code=${encodeURIComponent(shareLink.shortCode)}`,
  );
  await page
    .locator('[flt-semantics-identifier="phone-auth-screen"]')
    .waitFor({ timeout: 20_000 });

  // ---- Drive the inline email code flow → submit ("Finish" for the item flow). ----
  await completeEmailRegister(page, {
    name: 'Email Gear Guest',
    email: `email-gear-${Date.now()}@example.com`,
    baseUrl,
    specSlug: SPEC_SLUG,
    submitLabel: /^finish$/i,
  });

  // ---- Assert: the guest's auto-fired ExpressInterest created a transfer. ----
  // A loan auto-selects the recipient, so match the guest by name — the host
  // can't express interest on their own gear, so this can only be the guest.
  const hostTransfer = createTestClient(TransferService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  let guestBorrowed = false;
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const resp = await hostTransfer.getGearTransfers({ gearId: gear.gearId });
    guestBorrowed = resp.transfers.some((t) => t.recipient?.name === 'Email Gear Guest');
    if (guestBorrowed) break;
    await new Promise((r) => setTimeout(r, 500));
  }

  try {
    expect(
      guestBorrowed,
      'gear guest expressed interest after inline email registration',
    ).toBe(true);
    expect(uncaughtException, 'inline email register produced no uncaught exception').toBeNull();
  } catch (err) {
    test.info().attach('console-log', {
      contentType: 'text/plain',
      body: consoleLog.join('\n'),
    });
    throw err;
  }
});
