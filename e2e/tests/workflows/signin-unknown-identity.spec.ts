// e2e/tests/workflows/signin-unknown-identity.spec.ts
//
// Signing in with an identity that has no account must say so immediately,
// before any code is issued (#2571).
//
// The behaviour these guard replaced was actively misleading: entering an
// unknown address on the login screen mailed a code, showed the code box, and
// only after the person typed the code did it report there was no account to
// sign in to. The phone equivalent was worse — the code is sent from the device
// by the identity provider, so an unregistered number cost a real text message
// and still ended at "no account found" one OTP later.
//
// Three cases, because the fix is easy to get half-right:
//   1. Email sign-in, unknown address  → refused up front, no code step.
//   2. Phone sign-in, unregistered     → refused up front, no OTP step.
//   3. Email SIGN-UP, unknown address  → still sends a code. This is the
//      regression that matters: an unknown address is the NORMAL case when
//      registering, so the sign-in guard must not leak onto that surface.
//
// Needs no Firebase Auth Emulator: case 2 asserts verification never starts.

import { test, expect, type Page } from '../../lib/fixtures.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity, mintCommunityInviteLink } from '../../lib/seed/communities.js';
import { dismissDataConsent } from '../../lib/ui/consent.js';

const SPEC_SLUG = 'signin-unknown-identity';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

/** Type into the single visible text field, with Flutter-web's keystroke quirks. */
async function typeInto(page: Page, value: string): Promise<void> {
  const field = page.getByRole('textbox').last();
  await field.waitFor({ timeout: 20_000 });
  await field.click();
  await page.waitForTimeout(150);
  await field.selectText();
  await page.keyboard.press('Backspace');
  await field.pressSequentially(value, { delay: 30 });
}

test('email sign-in refuses an unknown address without issuing a code', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'email-code UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  await page.goto(`${baseUrl}/login`);
  await dismissDataConsent(page);
  await page.getByText(/login with email/i).click();

  await typeInto(page, `definitely-not-a-member-${Date.now()}@example.com`);
  await page.getByRole('button', { name: /email me a code/i }).click();

  // .first(): the message renders inside a LiveRegion, so it appears both as
  // visible text and as the announced semantics node.
  await expect(
    page.getByText(/no account found for that address/i).first(),
    'an unknown address is reported immediately',
  ).toBeVisible({ timeout: 20_000 });

  // The substance: the flow must not advance. A code box here means someone is
  // being asked to enter a code for an account that does not exist.
  //
  // Tracks the code step's submit label, which is "Verify" for both the email
  // and phone flows (#2925). This is a count-0 assertion, so a stale label here
  // would not fail — it would pass vacuously and stop guarding anything. On
  // /login the phone option is a navigation button, so no phone "Verify" exists
  // in this tree to confuse the count.
  await expect(
    page.getByRole('button', { name: /^verify$/i }),
    'no code step is presented for an address with no account',
  ).toHaveCount(0);
  await expect(
    page.getByText(/we sent a 6-digit code/i),
    'no "we sent a code" message, because nothing was sent',
  ).toHaveCount(0);
});

test('phone sign-in refuses an unregistered number before sending a text', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'phone UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  await page.goto(`${baseUrl}/login`);
  await dismissDataConsent(page);
  await page.getByRole('button', { name: /log in with phone/i }).click();

  await typeInto(page, '5550009999');
  await page.getByRole('button', { name: /send code/i }).click();

  await expect(
    page.getByText(/no account found for that number/i).first(),
    'an unregistered number is reported immediately',
  ).toBeVisible({ timeout: 20_000 });

  // The point of checking before verification: no OTP step means the identity
  // provider was never asked to send, so no text message was paid for.
  await expect(
    page.getByRole('button', { name: /^verify$/i }),
    'verification never starts for a number with no account',
  ).toHaveCount(0);
});

// The guard is scoped to sign-in surfaces. On a registration surface an unknown
// address is the whole point, so a code must still be issued — if this breaks,
// nobody can sign up at all, which is a far worse failure than the one the
// guard fixes.
test('email sign-up still issues a code for an unknown address', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'email-code UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Invite Host' });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  const invite = await mintCommunityInviteLink({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
  });

  // /invite?token= is the in-app register surface. A community invite's
  // /go/{code} serves the SSR "You're Invited — Open Ripls App" landing page
  // instead, which never reaches the Flutter register screen.
  await page.goto(`${baseUrl}/invite?token=${invite.shortCode}`);
  await dismissDataConsent(page);
  await page.getByText(/continue with email|sign up with email/i).click();

  await typeInto(page, `brand-new-${Date.now()}@example.com`);
  await page.getByRole('button', { name: /email me a code/i }).click();

  await expect(
    page.getByRole('button', { name: /^verify$/i }),
    'a registration surface still issues a code for an address with no account',
  ).toBeVisible({ timeout: 20_000 });
  await expect(
    page.getByText(/no account found/i),
    'sign-up must not report "no account" — that is the normal case here',
  ).toHaveCount(0);
});
