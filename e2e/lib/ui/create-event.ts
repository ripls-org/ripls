// e2e/lib/ui/create-event.ts — drive the host's web UI through the two-phase
// create flow (#2492). Shared by the phone-first loop and the Who's-In counts
// spec so both exercise the REAL unified-create UI rather than the RPC seed.

import { type Page } from '@playwright/test';
import { dismissDataConsent } from './consent.js';

/**
 * Drives an authenticated host page through unified create: open the create
 * modal → Text tab → type a prompt → Generate → Save Event. On a successful save
 * the server provisions the event's per-item community and the share sheet
 * auto-opens — this resolves once "Copy Link" (unique to that sheet) is visible,
 * leaving the sheet open for the caller (e.g. to "Add community").
 *
 * The page must already be authenticated (inject auth + installRequestIdOverride
 * before calling); the caller owns the browser context lifecycle. Requires the
 * server's deterministic AI provider (--mock-ai-provider) so generation
 * completes without a live LLM.
 */
export async function createEventViaWebUI(
  page: Page,
  baseUrl: string,
  opts: { prompt?: string } = {},
): Promise<void> {
  await page.goto(`${baseUrl}/`);

  // First-load data-consent modal blocks the home UI — accept it if shown.
  await dismissDataConsent(page);

  // Open the unified-create modal from the home "Create" affordance.
  await page.getByRole('button', { name: /^create$/i }).click();
  // The input drawer defaults to image mode — switch to the Text tab.
  await page.getByRole('button', { name: /^text$/i }).click();
  // Flutter-web text fields ignore .fill(); use real keystrokes so Flutter's
  // TextEditingController updates and the Generate button enables.
  const promptField = page.getByRole('textbox').first();
  await promptField.click();
  await promptField.pressSequentially(
    opts.prompt ?? 'Sunset rooftop gathering this Friday at 7pm',
    { delay: 20 },
  );

  // Generate → wait for the streamed preview to become saveable → Save.
  await page.getByRole('button', { name: /^draft it$/i }).click();
  const saveBtn = page.getByRole('button', { name: /^save event$/i });
  await saveBtn.waitFor({ timeout: 30_000 });
  await saveBtn.click();

  // The share sheet auto-opens right after Save — the two-phase create→share
  // handoff. "Copy Link" is unique to that sheet.
  await page.getByRole('button', { name: /copy link/i }).waitFor({ timeout: 20_000 });
}
