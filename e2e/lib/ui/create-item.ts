// e2e/lib/ui/create-item.ts — drive the host's web UI through unified create for
// the non-event item types (#2492). The sibling of create-event.ts: same
// two-phase create→share flow, but the AI classifier picks GEAR vs REQUEST from
// the prompt (server/ai/provider_e2e.go::classifyE2EText — "lend/loan/giveaway"
// → gear, "need/looking for/request" → request) and the preview's primary CTA
// label differs (gear uses the generic "Save"; request uses "Save Request").

import { type Page } from '@playwright/test';
import { dismissDataConsent } from './consent.js';

/**
 * Drives an authenticated host page through unified create and resolves once the
 * share sheet has auto-opened (its "Copy Link" button is visible), leaving it
 * open for the caller to invite people / communities.
 *
 * The page must already be authenticated (inject auth + installRequestIdOverride
 * first); the caller owns the browser context lifecycle. Requires the server's
 * deterministic AI provider (--mock-ai-provider).
 */
async function createViaWebUI(
  page: Page,
  baseUrl: string,
  opts: { prompt: string; saveLabel: RegExp },
): Promise<void> {
  await page.goto(`${baseUrl}/`);
  await dismissDataConsent(page);

  await page.getByRole('button', { name: /^create$/i }).click();
  // The drawer defaults to image mode — switch to the Text tab.
  await page.getByRole('button', { name: /^text$/i }).click();
  // Flutter-web text fields ignore .fill(); use real keystrokes so the
  // TextEditingController updates and Generate enables.
  const promptField = page.getByRole('textbox').first();
  await promptField.click();
  await promptField.pressSequentially(opts.prompt, { delay: 20 });

  await page.getByRole('button', { name: /^draft it$/i }).click();
  const saveBtn = page.getByRole('button', { name: opts.saveLabel });
  await saveBtn.waitFor({ timeout: 30_000 });
  await saveBtn.click();

  // The share sheet auto-opens after Save — "Copy Link" is unique to it.
  await page.getByRole('button', { name: /copy link/i }).waitFor({ timeout: 20_000 });
}

/**
 * Create a gear item via the real unified-create UI. The prompt must contain a
 * gear cue ("lend"/"loan"/"borrow"/"giveaway") so the classifier routes to gear;
 * the gear preview's primary button is the generic "Save".
 */
export async function createGearViaWebUI(
  page: Page,
  baseUrl: string,
  opts: { prompt?: string } = {},
): Promise<void> {
  await createViaWebUI(page, baseUrl, {
    prompt: opts.prompt ?? 'Lending out my pressure washer to neighbors this spring',
    saveLabel: /^save$/i,
  });
}

/**
 * Create a request via the real unified-create UI. The prompt must contain a
 * request cue ("need"/"looking for"/"request") so the classifier routes to
 * request; the request preview's primary button is "Save Request".
 */
export async function createRequestViaWebUI(
  page: Page,
  baseUrl: string,
  opts: { prompt?: string } = {},
): Promise<void> {
  await createViaWebUI(page, baseUrl, {
    // Keep gear cues ("borrow"/"lend"/"loan") OUT of the prompt — the classifier
    // checks gear before request, so they'd misroute to a gear.
    prompt: opts.prompt ?? 'Need folding tables for a weekend bake sale',
    saveLabel: /^save request$/i,
  });
}
