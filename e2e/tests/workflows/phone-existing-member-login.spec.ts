// e2e/tests/workflows/phone-existing-member-login.spec.ts
//
// The phone-first invitee who is ALREADY a Ripls member (#2492). A returning
// member opens an invite on a device where they're not signed in, enters their
// phone, and confirms the OTP. The phone already has an account, so PhoneRegister
// returns AlreadyExists — the flow must LOG THEM IN (PhoneLogin) and fire the
// same auto-action, NOT reject them as a duplicate. Guards the register→login
// fallback in PhoneAuthScreen._handlePhoneRegister.
//
// The member's account is registered off-camera (registerPhoneUser: emulator
// OTP → PhoneRegister), so the in-app attempt with the same number exercises the
// returning-member path rather than a fresh sign-up.

import { test, expect } from '../../lib/fixtures.js';
import { TransferService } from '../../gen/ripls/api/transfer_service_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser } from '../../lib/seed/users.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { mintGearShareLink } from '../../lib/seed/communities.js';
import { registerPhoneUser } from '../../lib/seed/phone-user.js';
import { completePhoneRegister } from '../../lib/ui/phone-register.js';
import { resolve } from 'node:path';

const SPEC_SLUG = 'phone-existing-member-login';
const MEMBER_PHONE = '+15551234574';
const MEMBER_NAME = 'Jordan Mercer';
const ITEM_NAME = 'Pressure Washer';
const HERO = resolve(__dirname, '..', '..', 'fixtures', 'walkthroughs', 'gear-pressure-washer.jpg');

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('a returning member is logged in (not rejected) when they re-enter their phone', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'phone-OTP UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ---- Host shares a gear for loan; the member already has a phone account. ----
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Owen Park' });
  const gear = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: ITEM_NAME,
    description: 'Electric pressure washer — happy to lend it out on weekends.',
    heroImagePath: HERO,
  });
  const shareLink = await mintGearShareLink({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: gear.communityId,
    gearId: gear.gearId,
  });
  // The returning member: a real AUTH_METHOD_PHONE account already exists for
  // this number (registered off-camera).
  await registerPhoneUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    phoneNumber: MEMBER_PHONE,
    name: MEMBER_NAME,
  });

  // ---- The member opens the invite (not signed in here) and goes phone-first. ----
  await page.goto(`${baseUrl}/go/${shareLink.shortCode}`);
  await page.waitForTimeout(1500);
  await page.getByRole('link', { name: /ask .* to borrow/i }).click();

  // Phone → OTP → name → Finish. The number is already registered, so
  // PhoneRegister returns AlreadyExists and the flow falls back to PhoneLogin.
  await completePhoneRegister(page, {
    phone: MEMBER_PHONE,
    name: MEMBER_NAME,
    finishLabel: /^finish$/i,
  });

  // ---- Assert: they were logged in and their ExpressInterest fired (not an
  // "account already exists" rejection). The host can't borrow their own gear,
  // so a transfer with the member as recipient can only be the returning member.
  const hostTransfer = createTestClient(TransferService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  let memberBorrowed = false;
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const resp = await hostTransfer.getGearTransfers({ gearId: gear.gearId });
    memberBorrowed = resp.transfers.some((t) => t.recipient?.name === MEMBER_NAME);
    if (memberBorrowed) break;
    await new Promise((r) => setTimeout(r, 500));
  }
  expect(
    memberBorrowed,
    `returning member "${MEMBER_NAME}" was logged in and borrowed the gear (not rejected as duplicate)`,
  ).toBe(true);

  // Let the in-app item view (hero image) settle on camera before the video ends.
  await page.waitForTimeout(2500);
});
