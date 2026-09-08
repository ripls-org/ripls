// e2e/lib/ui/email-register.ts — drive the inline email sign-up flow
// (EmailAuthScreen) that both RegisterScreen and the phone-first guest screen
// host (#2595). Since #2571 this is a mailed one-time code rather than a
// password, so it is three steps: address → code → name.
//
// It still needs NO Firebase Auth Emulator. The code is read from the
// dev-mode-only `devCode` field on RequestEmailCode, the same seam the Go
// integration helpers use — so this drives the real UI while skipping only the
// mailbox.
//
// Flutter-web specifics (see docs/client/testing/semantics_identifiers.md and
// e2e/README.md): text fields ignore Playwright's .fill() (it sets the
// a11y-proxy value without reaching Flutter's TextEditingController), so we use
// real keystrokes via click + pressSequentially. Only one text field is on
// screen per step, so `getByRole('textbox')` is unambiguous — unlike the old
// four-field password form, which needed accessible-name disambiguation.

import { expect, type Page } from '@playwright/test';
import { LoginService } from '../../gen/ripls/api/login_service_pb.js';
import { createTestClient } from '../connect.js';

export interface CompleteEmailRegisterOptions {
  name: string;
  email: string;
  /** Base URL of the server under test, used to read the dev-mode code. */
  baseUrl: string;
  /** Spec slug for per-spec client tagging, as elsewhere in the harness. */
  specSlug: string;
  /**
   * The final button's label. On the phone-first guest RSVP flow this is
   * `/finish rsvp/i`; on RegisterScreen it's `/create account/i`.
   */
  submitLabel: RegExp;
  /**
   * Whether the name step is expected. Defaults to true (a new address).
   *
   * Pass false for an address that already has an account: the helper then
   * ASSERTS the step never appears rather than filling it in. That assertion is
   * the point — an earlier version of this helper dutifully typed a name on the
   * login screen and passed, hiding the bug where returning members were asked
   * "What should we call you?".
   */
  expectNameStep?: boolean;
}

/**
 * Read the code the server just issued for an address.
 *
 * The code is fetched over RPC rather than from a mailbox: RequestEmailCode
 * echoes it back on `devCode` when the server runs in dev mode. Note this
 * *re-requests* a code, which retires the one the UI's own request produced —
 * harmless because the newest code is the live one, and the UI submits whatever
 * we type next.
 */
async function readDevCode(
  opts: Pick<CompleteEmailRegisterOptions, 'baseUrl' | 'specSlug' | 'email'>,
): Promise<string> {
  const client = createTestClient(LoginService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
  });
  const resp = await client.requestEmailCode({ email: opts.email });
  if (!resp.devCode) {
    throw new Error(
      `RequestEmailCode returned no devCode for ${opts.email}; check server dev-mode flag`,
    );
  }
  return resp.devCode;
}

/** Type into the single text field on the current step. */
async function typeIntoStepField(page: Page, value: string): Promise<void> {
  const field = page.getByRole('textbox').last();
  await field.waitFor({ timeout: 20_000 });
  // Retry the whole click→type→verify: Flutter-web drops the first keystroke if
  // typing starts before the field's focus/IME is wired after the click, so a
  // single attempt intermittently loses a leading character.
  for (let attempt = 0; attempt < 3; attempt++) {
    await field.click();
    // Settle so the focus/IME is ready before the first keystroke lands.
    await page.waitForTimeout(150);
    // Clear first. The code field arrives PRE-FILLED against a dev server (the
    // app autofills from the same devCode this helper reads), so typing
    // straight in would append and submit a 12-digit code.
    await field.selectText();
    await page.keyboard.press('Backspace');
    await field.pressSequentially(value, { delay: 30 });
    // Best-effort read-back: inputValue() throws on Flutter-web's custom
    // flt-semantics element (not a real <input>), in which case we can't verify
    // and trust the settle above.
    let typed: string | null = null;
    try {
      typed = await field.inputValue();
    } catch {
      return;
    }
    if (typed === value) return;
    await field.selectText();
    await page.keyboard.press('Backspace');
  }
  throw new Error(`could not reliably type "${value}" into the current field`);
}

/**
 * Expand the email section and drive address → code → name to submission.
 * Assumes the page is already on a screen that hosts EmailAuthScreen (e.g. the
 * phone-first guest screen). Returns once the final button is clicked.
 *
 * The expander label differs by host screen — the phone-first guest screen says
 * "Continue with email" (it shares the Google button's verb, #2724),
 * RegisterScreen says "Sign up with Email", and LoginScreen says "Login with
 * Email" — so the locator accepts any of them.
 */
export async function completeEmailRegister(
  page: Page,
  opts: CompleteEmailRegisterOptions,
): Promise<void> {
  // Expand the collapsed email section so the fields render.
  await page
    .getByText(/continue with email|sign up with email|login with email/i)
    .click();

  // Step 1 — address.
  await typeIntoStepField(page, opts.email);
  // Distinct from the phone step's own "Send Code" button, which is on the
  // same screen above this one.
  await page.getByRole('button', { name: /email me a code/i }).click();

  // Step 2 — code. Wait for the code step to render before reading the code, so
  // the UI's own request has already been issued.
  //
  // "Verify" is shared with the phone code step (#2925) — the two perform the
  // same action, so they carry the same label. That is safe to locate page-wide
  // here: PhoneAuthScreen renders this email form only on its phone-number
  // step, so a phone "Verify" is never in the tree at the same time.
  await page
    .getByRole('button', { name: /^verify$/i })
    .waitFor({ timeout: 20_000 });
  await typeIntoStepField(page, await readDevCode(opts));
  await page.getByRole('button', { name: /^verify$/i }).click();

  // Step 3 — name. Only a NEW address reaches this: the server reports whether
  // the account already exists once ownership is proven, and a returning member
  // is signed straight in. Callers that expect a returning member should pass
  // expectNameStep: false, which asserts the step is skipped.
  const nameStep = page.getByRole('button', { name: opts.submitLabel });
  if (opts.expectNameStep === false) {
    await expect(
      nameStep,
      'a returning member must not be asked for a display name they already chose',
    ).toHaveCount(0, { timeout: 10_000 });
    return;
  }

  await nameStep.waitFor({ timeout: 20_000 });
  await typeIntoStepField(page, opts.name);
  await nameStep.click();
}
