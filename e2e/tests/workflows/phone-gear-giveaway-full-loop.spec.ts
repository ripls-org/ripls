// e2e/tests/workflows/phone-gear-giveaway-full-loop.spec.ts
//
// The phone-first pivot for a gear GIVEAWAY (epic #2492, WEB-4) — the giveaway
// twin of phone-gear-interest-full-loop (which covers a loan). A non-app guest
// opens a shared-giveaway link, views the item on the web, registers via REAL
// Firebase phone OTP (Auth Emulator), and their ExpressInterest auto-fires as
// they join the gear's ad-hoc community and their pre-seeded provisional
// identity is promoted. Mirrors docs/workflows/phone_first_gear_interest.md.
//
// Loan vs giveaway differs in two observable ways the loan spec can't cover:
//   - the SSR landing CTA is "I want this" (aria "Tell {owner} you want {item}"),
//     not "Ask to borrow"; and
//   - a giveaway does NOT auto-select a recipient — it sits at INTEREST_EXPRESSED
//     awaiting the owner's choice, so the guest surfaces in the transfer
//     context's pending_requests (borrower), not on transfer.recipient.

import { test, expect } from '../../lib/fixtures.js';
import { TransferService } from '../../gen/ripls/api/transfer_service_pb.js';
import { Availability } from '../../gen/ripls/api/gear_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser } from '../../lib/seed/users.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { mintGearShareLink } from '../../lib/seed/communities.js';
import { createProvisionalUser } from '../../lib/seed/provisional.js';
import { completePhoneRegister } from '../../lib/ui/phone-register.js';
import { resolve } from 'node:path';

const SPEC_SLUG = 'phone-gear-giveaway-full-loop';
const GUEST_PHONE = '+15551234573';
const HOST_NAME = 'Maya Chen';
const GUEST_NAME = 'Tomás Rivera';
const ITEM_NAME = 'Vintage Record Player';
const HERO = resolve(__dirname, '..', '..', 'fixtures', 'walkthroughs', 'gear-record-player.jpg');

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('guest claims a shared giveaway via phone OTP (phone-first full loop)', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'phone-OTP UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ---- Host registers + shares a gear for GIVEAWAY (RPC; off-camera). ----
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: HOST_NAME });
  const gear = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: ITEM_NAME,
    description: 'Restored 1970s turntable — works great, just out of room. Free to a good home.',
    availability: Availability.FOR_GIVEAWAY,
    heroImagePath: HERO,
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
    name: GUEST_NAME,
    phoneNumber: GUEST_PHONE,
  });

  // ---- Guest opens the SSR giveaway landing (real HTML served by the Go server). ----
  await page.goto(`${baseUrl}/go/${shareLink.shortCode}`);
  // Dwell on the landing so the recorded video is easy for a human to follow.
  await page.waitForTimeout(1500);
  // The giveaway CTA is an <a> to /item/{id}?intent=interest&code=…; its
  // accessible name is the aria-label ("Tell Maya Chen you want Vintage Record Player").
  await page.getByRole('link', { name: /tell .* you want/i }).click();

  // ---- Phone-first register (the unit under test). ----
  // The gear guest sees "Confirm your phone to claim {item}" and finishes with
  // the generic "Finish" button, which fires PhoneRegister then auto-fires
  // ExpressInterest after the community join.
  await completePhoneRegister(page, {
    phone: GUEST_PHONE,
    name: GUEST_NAME,
    finishLabel: /^finish$/i,
  });

  // ---- Assert: the GUEST's auto-fired ExpressInterest recorded their interest. ----
  // A giveaway does not auto-select a recipient, so transfer.recipient stays
  // empty; the interested guest appears in the transfer context's
  // pending_requests as the borrower. The host can't express interest on their
  // own gear, so a pending request by the guest's name can only be the guest.
  const hostTransfer = createTestClient(TransferService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  let guestWantsIt = false;
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const ctx = await hostTransfer.getGearTransferContext({
      gearId: gear.gearId,
      communityId: gear.communityId,
    });
    guestWantsIt = (ctx.context?.pendingRequests ?? []).some(
      (r) => r.borrower?.name === GUEST_NAME,
    );
    if (guestWantsIt) break;
    await new Promise((r) => setTimeout(r, 500));
  }
  expect(
    guestWantsIt,
    `guest "${GUEST_NAME}" expressed interest in the giveaway after phone-OTP registration`,
  ).toBe(true);

  // Let the in-app item view (hero image) settle on camera before the video ends.
  await page.waitForTimeout(2500);
});
