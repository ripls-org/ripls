// e2e/tests/workflows/login-screen-email-code.spec.ts
//
// Email sign-in from the LOGIN SCREEN (#2571). The other email specs all enter
// through the phone-first guest screen's below-the-fold expander, which is the
// path a link recipient takes. This is the path an existing member takes when
// they open the app and sign in deliberately — a different host widget, a
// different expander, and the only place the password fallback lives.
//
// It covers both halves of the migration affordance:
//   1. The email section leads with the one-time code and signs the member in.
//   2. "Use my password instead" reveals the legacy form, which still works.
//
// (2) is the one that matters for the accounts that predate the code flow: if it
// regressed, those users would be locked out with no way in until they noticed
// the code option. Widget tests prove the toggle swaps the widget tree; only
// this proves the credential behind it still authenticates.
//
// Needs NO Firebase Auth Emulator: the code is read from the dev-mode `devCode`
// echo rather than a mailbox.

import { test, expect, type Page, type Locator } from '../../lib/fixtures.js';
import { registerUser, seedLegacyPasswordUser } from '../../lib/seed/users.js';
import { completeEmailRegister } from '../../lib/ui/email-register.js';
import { dismissDataConsent } from '../../lib/ui/consent.js';

const SPEC_SLUG = 'login-screen-email-code';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

/**
 * Assert the app left the auth gate for the signed-in app.
 *
 * The router only navigates off `/login` once `setAuthState` has persisted a
 * session, so leaving it *is* the success signal — the same fact
 * `event-signin-gate-to-login.spec.ts` asserts in the other direction.
 *
 * Two surfaces were tried first and are deliberately not used. Stored
 * credentials: the access token moved from SharedPreferences to SecureStorage,
 * which mangles and encrypts its web keys, so a localStorage probe tests an
 * implementation detail that has already moved once. The home greeting
 * ("Good afternoon, Rae"): CanvasKit prunes static text from the accessibility
 * tree, so `getByText` cannot see it — the documented trap in
 * docs/client/testing.md.
 *
 * *Whose* session it is comes from construction rather than assertion: the
 * address is unique per run and the code was minted for it, so only that
 * account can be the one signed in.
 */
async function expectSignedIn(page: Page): Promise<void> {
  await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 30_000 });
  // And the auth gate is genuinely gone, not merely re-routed behind a modal.
  await dismissDataConsent(page);
  await expect(
    page.getByText(/login with email/i),
    'the login form is gone once the session is established',
  ).toHaveCount(0, { timeout: 10_000 });
}

test('an existing member signs in from the login screen with an emailed code', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'email-code UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  const member = await registerUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'Rae Whitfield',
    email: `login-code-${Date.now()}@example.com`,
  });

  await page.goto(`${baseUrl}/login`);
  // First load shows the data-consent modal, which covers the login form.
  await dismissDataConsent(page);
  await page.getByText(/login with email/i).waitFor({ timeout: 20_000 });

  // The address already has an account, so the register attempt comes back
  // alreadyExists and the flow falls back to EmailLogin with the same proof.
  await completeEmailRegister(page, {
    email: member.email,
    name: 'Rae Whitfield',
    baseUrl,
    specSlug: SPEC_SLUG,
    submitLabel: /^log in$/i,
    // The account exists, so the flow must sign in directly. Asserting the name
    // step is absent is the substance of this test: the previous version filled
    // it in and passed while a returning member was being asked to re-introduce
    // themselves on the login screen.
    expectNameStep: false,
  });

  await expectSignedIn(page);
});

test('the legacy password form still signs in the accounts that hold one', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'email UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // A password-holding account, the shape that predates the code flow. Written
  // straight to the database: registration refuses a password in every
  // environment now (#2864), so this shape can only be manufactured out of
  // band. That is the point — the server e2e drives has no branch that would
  // create one.
  const legacyEmail = `legacy-${Date.now()}@example.com`;
  const legacyPassword = 'legacy-password-1234';
  seedLegacyPasswordUser({
    email: legacyEmail,
    name: 'Marta Olsen',
    password: legacyPassword,
  });

  await page.goto(`${baseUrl}/login`);
  await dismissDataConsent(page);
  await page.getByText(/login with email/i).click();

  // The code flow is what shows first; the password form is behind the opt-in.
  await page.getByRole('button', { name: /use my password instead/i }).click();

  const typeInto = async (locator: Locator, value: string) => {
    await locator.waitFor({ timeout: 20_000 });
    await locator.click();
    await page.waitForTimeout(150);
    await locator.pressSequentially(value, { delay: 30 });
  };

  const fields = page.getByRole('textbox');
  await typeInto(fields.nth(0), legacyEmail);
  await typeInto(page.locator('input[type="password"]').first(), legacyPassword);

  await page.getByRole('button', { name: /^log in$/i }).click();

  await expectSignedIn(page);
});
