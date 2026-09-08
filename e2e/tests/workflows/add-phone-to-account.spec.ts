// e2e/tests/workflows/add-phone-to-account.spec.ts
//
// Adding a verified phone number to an existing email/password account (#2596).
// An email-registered user opens their profile, taps the (tucked-away) "Add
// phone number" row, and completes the phone-OTP flow in *attach* mode. The
// number must land on their EXISTING account — additive, not a new sign-up.
//
// Drives the attach branch of PhoneAuthScreen + UserService.AddPhoneNumber
// end-to-end through the real Flutter Web bundle. OTP is read back from the
// Firebase Auth Emulator the harness boots (global-setup.ts). Asserts via a
// self GetUser that the phone is on the same account.

import { test, expect } from '../../lib/fixtures.js';
import { UserService } from '../../gen/ripls/api/user_service_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser } from '../../lib/seed/users.js';
import { injectAuth } from '../../lib/auth.js';
import { dismissDataConsent } from '../../lib/ui/consent.js';
import { completePhoneAttach } from '../../lib/ui/phone-register.js';

const SPEC_SLUG = 'add-phone-to-account';
const USER_NAME = 'Avery Phoneless';
const NEW_PHONE = '+15551234588';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('an email/password user adds a verified phone from profile settings', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'phone-OTP UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ---- An existing email/password account that has no phone yet. ----
  const user = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: USER_NAME });

  await injectAuth(page, {
    accessToken: user.accessToken,
    refreshToken: user.refreshToken,
    user: { id: user.userId, name: user.name },
    serverUrl: baseUrl,
  });

  // ---- Navigate home → account menu → Settings → Profile → Edit profile. ----
  await page.goto(baseUrl);
  // First load shows the data-consent modal, which blocks the home UI.
  await dismissDataConsent(page);
  // Account menu lives on the home app bar; wait for the bundle to render it.
  const accountMenu = page.getByRole('button', { name: /open account menu/i });
  await accountMenu.waitFor({ timeout: 30_000 });
  await accountMenu.click();
  await page.getByRole('button', { name: /^settings$/i }).click();
  // Settings hub → the profile card is titled with the user's name.
  await page.getByRole('button', { name: USER_NAME }).click();
  // Profile settings → Edit profile.
  await page.getByRole('button', { name: /update profile/i }).click();

  // ---- The tucked-away add-phone row → attach-mode phone-OTP flow. ----
  const addPhoneRow = page.locator('[flt-semantics-identifier="profile-add-phone"]');
  await addPhoneRow.waitFor({ timeout: 20_000 });
  await addPhoneRow.click();

  await completePhoneAttach(page, { phone: NEW_PHONE });

  // ---- Assert: the phone is now on the SAME account (additive), server-side.
  // GetUser returns phone_number only on a self-fetch, so the user's own token
  // is what surfaces it.
  const userClient = createTestClient(UserService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: user.accessToken,
  });
  let attached = false;
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const resp = await userClient.getUser({ userId: user.userId });
    if (resp.phoneNumber === NEW_PHONE) {
      attached = true;
      break;
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  expect(
    attached,
    `phone ${NEW_PHONE} was attached to the existing email account ${user.email}`,
  ).toBe(true);

  // Let the profile-edit row settle on camera (it now shows the number).
  await page.waitForTimeout(1500);
});
