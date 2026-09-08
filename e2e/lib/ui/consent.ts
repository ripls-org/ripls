// e2e/lib/ui/consent.ts — robust dismissal of the first-load Data Consent modal.
//
// Root cause of the event-rsvp-streaming flake (#2531): the spec tapped
// "Accept All" with a 4s timeout and SWALLOWED any failure (`.catch`). But the
// consent modal can only appear AFTER the Flutter bundle bootstraps — which
// playwright.config documents as 8-12s on a cold load — so the 4s tap fired
// during bootstrap, before the button existed, hit nothing, and the swallowed
// miss left the modal covering Home. The Playwright accessibility snapshot at
// the moment of failure contained ONLY the consent dialog; the event card was
// simply behind it. (It is not slow rendering — once the modal is up it renders
// fast; the tap just fired too early and its failure was hidden.)
//
// The fix: wait for the button to actually exist (the bootstrap window, the same
// budget as the config's expect.timeout), click it once, and CONFIRM it's gone —
// never swallow, so a stuck modal fails loudly here instead of as a confusing
// "card not found" 25s later.

import { expect, type Page } from '@playwright/test';

/**
 * Dismiss the first-load Data Consent modal if present.
 *
 * Waits out the bundle-bootstrap window for the modal to appear; tolerates
 * genuine absence (a navigation within an already-consented context). When the
 * modal is up it is accepted and its dismissal is verified.
 */
export async function dismissDataConsent(page: Page): Promise<void> {
  const acceptAll = page.getByRole('button', { name: /accept all/i });
  // Bundle bootstrap (~8-12s cold) must finish before the modal can render —
  // this is the bootstrap budget, NOT a render wait. Tolerate genuine absence.
  const present = await acceptAll
    .waitFor({ state: 'visible', timeout: 15_000 })
    .then(() => true)
    .catch(() => false);
  if (!present) return;

  // The modal is up; rendering is fast, so one click suffices. click() auto-waits
  // for actionability. Verify dismissal so a stuck modal fails loudly here.
  await acceptAll.click();
  await expect(acceptAll).toHaveCount(0, { timeout: 5_000 });
}
