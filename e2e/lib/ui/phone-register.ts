// e2e/lib/ui/phone-register.ts — drive the Flutter-web phone-first register UI
// (the unit under test in the phone-first full-loop specs, #2492). OTP is read
// back from the Firebase Auth Emulator the harness boots (global-setup.ts).
//
// Flutter-web text fields ignore Playwright's .fill() (it sets the a11y-proxy
// value without reaching Flutter's TextEditingController); real keystrokes via
// click + pressSequentially do. Buttons are reached via getByRole (their labels
// are in the a11y tree). See docs/client/testing/semantics_identifiers.md.

import { type Locator, type Page } from '@playwright/test';

/**
 * Click → settle → type → read back, retrying up to 3 times. Flutter-web
 * drops keystrokes (sometimes all of them) when typing starts before the
 * field's focus/IME is wired after the click — for the PHONE field that's
 * fatal in a non-obvious way: the emulator stores the OTP under the mangled
 * number, and the poll for the real one times out ("no emulator verification
 * code"). Same pattern as email-register.ts / lib/walkthrough.ts; comparison is
 * letters+digits only because the phone field reformats what was typed.
 */
async function typeIntoField(page: Page, field: Locator, value: string): Promise<void> {
  for (let attempt = 0; attempt < 3; attempt++) {
    await field.click();
    await page.waitForTimeout(300);
    await field.pressSequentially(value, { delay: 20 });
    let typed: string | null = null;
    try {
      typed = await field.inputValue();
    } catch {
      return; // not a readable input — trust the settle above
    }
    const norm = (s: string) => s.replace(/[^\p{L}\p{N}]/gu, '');
    if (norm(typed) === norm(value)) return;
    await field.selectText();
    await page.keyboard.press('Backspace');
  }
  throw new Error(`could not reliably type "${value}"`);
}

const EMULATOR_PROJECT = 'demo-ripls';

/** Read the verification code the Auth Emulator generated for a phone number. */
export async function emulatorVerificationCode(phone: string): Promise<string> {
  const host = process.env.FIREBASE_AUTH_EMULATOR_HOST ?? '127.0.0.1:9099';
  const url = `http://${host}/emulator/v1/projects/${EMULATOR_PROJECT}/verificationCodes`;
  // The code is generated when the client calls sendVerificationCode; poll briefly.
  for (let i = 0; i < 20; i++) {
    const resp = await fetch(url);
    if (resp.ok) {
      const body = (await resp.json()) as {
        verificationCodes?: { phoneNumber: string; code: string }[];
      };
      const match = (body.verificationCodes ?? [])
        .filter((c) => c.phoneNumber === phone)
        .pop();
      if (match?.code) return match.code;
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`no emulator verification code for ${phone}`);
}

export interface CompletePhoneRegisterOptions {
  phone: string;
  name: string;
  /**
   * The name-step finish button label. `/finish rsvp/i` for events,
   * `/^finish$/i` for the gear/request guest action flows.
   */
  finishLabel: RegExp;
}

/**
 * Drive the three-step phone-first register flow on the open page: phone →
 * Send Code → OTP → Verify → name → finish (which fires PhoneRegister). Assumes
 * the page is already on the phone-entry screen (the CTA on the SSR landing was
 * tapped). Returns once the finish button is clicked.
 */
export async function completePhoneRegister(
  page: Page,
  opts: CompletePhoneRegisterOptions,
): Promise<void> {
  const typeInto = (value: string) =>
    typeIntoField(page, page.getByRole('textbox').first(), value);

  // Step 1: phone number → Send Code.
  await page.getByRole('button', { name: /send code/i }).waitFor({ timeout: 20_000 });
  await typeInto(opts.phone);
  await page.getByRole('button', { name: /send code/i }).click();

  // Step 2: read the emulator-generated OTP, enter it → Verify.
  await page.getByRole('button', { name: /^verify$/i }).waitFor({ timeout: 20_000 });
  const code = await emulatorVerificationCode(opts.phone);
  await typeInto(code);
  await page.getByRole('button', { name: /^verify$/i }).click();

  // Step 3: "Phone Verified" → name → finish (fires PhoneRegister + the action).
  await page.getByRole('button', { name: opts.finishLabel }).waitFor({ timeout: 20_000 });
  await typeInto(opts.name);
  await page.getByRole('button', { name: opts.finishLabel }).click();
}

/**
 * Drive the phone-OTP flow in *attach* mode (adding a phone to the signed-in
 * account, #2596): phone → Send Code → OTP → Verify. There is no name step — on
 * Verify the screen fires `AddPhoneNumber` and pops back to the caller. Assumes
 * the page is already on the attach `PhoneAuthScreen`.
 *
 * The text-field locator is scoped to the `phone-auth-screen` semantics
 * container because this screen is pushed *over* another screen that has its own
 * text fields (e.g. the profile-edit name/description) — an unscoped
 * `getByRole('textbox')` could otherwise match the wrong field underneath.
 */
export async function completePhoneAttach(
  page: Page,
  opts: { phone: string },
): Promise<void> {
  const screen = page.locator('[flt-semantics-identifier="phone-auth-screen"]');
  const typeInto = (value: string) =>
    typeIntoField(page, screen.getByRole('textbox').first(), value);

  // Step 1: phone number → Send Code.
  await page.getByRole('button', { name: /send code/i }).waitFor({ timeout: 20_000 });
  await typeInto(opts.phone);
  await page.getByRole('button', { name: /send code/i }).click();

  // Step 2: read the emulator-generated OTP, enter it → Verify. Verify fires
  // AddPhoneNumber and the screen pops on success.
  await page.getByRole('button', { name: /^verify$/i }).waitFor({ timeout: 20_000 });
  const code = await emulatorVerificationCode(opts.phone);
  await typeInto(code);
  await page.getByRole('button', { name: /^verify$/i }).click();
}
