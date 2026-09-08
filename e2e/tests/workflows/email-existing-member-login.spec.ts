// e2e/tests/workflows/email-existing-member-login.spec.ts
//
// The email analog of phone-existing-member-login (#2571). A returning member
// opens an invite on a device where they're not signed in, enters their EMAIL,
// and confirms the mailed one-time code. The address already has an account, so
// EmailRegister returns AlreadyExists — the flow must LOG THEM IN (EmailLogin
// with the same proof token) and fire the same auto-action, NOT reject them as a
// duplicate. Guards the register→login fallback in
// EmailAuthScreen._handleSubmitName, which is deliberately the same shape as
// PhoneAuthScreen._handlePhoneRegister.
//
// This is the sign-IN counterpart to the two email-register specs, which only
// ever exercise a fresh address. The two paths diverge entirely on the server
// (EmailRegister vs EmailLogin, and the credential-presence gate that lets a
// non-email/password account sign in by code), so register coverage does not
// imply login coverage.
//
// Needs NO Firebase Auth Emulator: the code is read from the dev-mode `devCode`
// echo rather than a mailbox, so the real UI is driven and only the mailbox is
// skipped.

import { test, expect } from '../../lib/fixtures.js';
import { TransferService } from '../../gen/ripls/api/transfer_service_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser } from '../../lib/seed/users.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { mintGearShareLink } from '../../lib/seed/communities.js';
import { completeEmailRegister } from '../../lib/ui/email-register.js';
import { resolve } from 'node:path';

const SPEC_SLUG = 'email-existing-member-login';
const MEMBER_EMAIL = 'jordan.mercer@example.com';
const MEMBER_NAME = 'Jordan Mercer';
const ITEM_NAME = 'Pressure Washer';
const HERO = resolve(__dirname, '..', '..', 'fixtures', 'walkthroughs', 'gear-pressure-washer.jpg');

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('a returning member is logged in (not rejected) when they re-enter their email', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'email-code UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ---- Host shares a gear for loan; the member already has an account. ----
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
  // The returning member: a real account already exists for this address,
  // registered off-camera through the same code flow.
  await registerUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    email: MEMBER_EMAIL,
    name: MEMBER_NAME,
  });

  // ---- The member opens the invite (not signed in here) and picks email. ----
  await page.goto(`${baseUrl}/go/${shareLink.shortCode}`);
  await page.waitForTimeout(1500);
  await page.getByRole('link', { name: /ask .* to borrow/i }).click();

  // Address → code → name → Finish. The address is already registered, so
  // EmailRegister returns AlreadyExists and the flow falls back to EmailLogin
  // with the proof token it already holds.
  await completeEmailRegister(page, {
    email: MEMBER_EMAIL,
    name: MEMBER_NAME,
    baseUrl,
    specSlug: SPEC_SLUG,
    submitLabel: /^finish$/i,
    // Returning member: signed in directly, never asked for a name.
    expectNameStep: false,
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
