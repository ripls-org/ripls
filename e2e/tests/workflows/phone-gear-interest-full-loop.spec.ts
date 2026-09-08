// e2e/tests/workflows/phone-gear-interest-full-loop.spec.ts
//
// The phone-first pivot extended to gear (epic #2492, WEB-4): a non-app guest
// opens a shared-gear invite link, views the item on the web, registers via
// REAL Firebase phone OTP (Auth Emulator), and their ExpressInterest auto-fires
// as they join the gear's ad-hoc community and their pre-seeded provisional
// identity is promoted to the real account. Mirrors phone-rsvp-full-loop and
// docs/workflows/phone_first_gear_interest.md.
//
// Unlike the RSVP spec, the host's gear is RPC-seeded off-camera (SaveGear
// provisions the per-item community and carries FOR_LOAN into its first share)
// — the unit under test is the guest's phone-first ExpressInterest loop, not
// gear creation.

import { test, expect } from '../../lib/fixtures.js';
import { TransferService } from '../../gen/ripls/api/transfer_service_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser } from '../../lib/seed/users.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { mintGearShareLink } from '../../lib/seed/communities.js';
import { createProvisionalUser } from '../../lib/seed/provisional.js';
import { completePhoneRegister } from '../../lib/ui/phone-register.js';

const SPEC_SLUG = 'phone-gear-interest-full-loop';
const GUEST_PHONE = '+15551234571';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('guest borrows a shared gear via phone OTP (phone-first full loop)', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'phone-OTP UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ---- Host registers + shares a gear for loan (RPC; off-camera). ----
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
  // Pre-invite the guest by phone → a provisional placeholder promote-on-verify claims.
  await createProvisionalUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: gear.communityId,
    name: 'Lena Hoffmann',
    phoneNumber: GUEST_PHONE,
  });

  // ---- Guest opens the SSR gear landing (real HTML served by the Go server). ----
  await page.goto(`${baseUrl}/go/${shareLink.shortCode}`);
  // Dwell on the landing so the recorded video is easy for a human to follow.
  await page.waitForTimeout(1500);
  // The gear landing's primary CTA is an <a> to /item/{id}?intent=interest&code=…;
  // its accessible name is the aria-label ("Ask Dana to borrow Cordless Drill").
  await page.getByRole('link', { name: /ask .* to borrow/i }).click();

  // ---- Phone-first register (the unit under test). ----
  // The gear guest sees "Confirm your phone to claim {item}" and finishes with
  // the generic "Finish" button (phoneAuthFinishItem), which fires PhoneRegister
  // then auto-fires ExpressInterest after the community join.
  await completePhoneRegister(page, {
    phone: GUEST_PHONE,
    name: 'Lena Hoffmann',
    finishLabel: /^finish$/i,
  });

  // ---- Assert: the GUEST's auto-fired ExpressInterest created a transfer. ----
  // Match the guest by name (a loan auto-selects the recipient, so recipient is
  // populated). The host can't express interest on their own gear, so this can
  // only be the guest — no false-green from a pre-existing transfer.
  const hostTransfer = createTestClient(TransferService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  let guestBorrowed = false;
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const resp = await hostTransfer.getGearTransfers({ gearId: gear.gearId });
    guestBorrowed = resp.transfers.some((t) => t.recipient?.name === 'Lena Hoffmann');
    if (guestBorrowed) break;
    await new Promise((r) => setTimeout(r, 500));
  }
  expect(guestBorrowed, 'guest "Lena Hoffmann" borrowed the gear after phone-OTP registration').toBe(
    true,
  );

  // Let the in-app item view (hero image) settle on camera before the video ends.
  await page.waitForTimeout(2500);
});
