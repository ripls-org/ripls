// e2e/tests/workflows/phone-rsvp-full-loop.spec.ts
//
// The phone-first pivot milestone (epic #2492, WEB-2): a non-app guest opens a
// per-item invite link, views the event on the web, registers via REAL Firebase
// phone OTP (Auth Emulator), and their RSVP auto-fires as they join the
// community and their pre-seeded provisional identity is promoted to the real
// account. The now-registered invitee then closes the growth loop (#2630):
// they open the Who's In roster's Invite action and get their own /go/ share
// link (member link-resharing), with the host-only audience rows absent.
// Mirrors docs/workflows/phone_first_rsvp.md.
//
// Unlike the other specs (which inject tokens to skip auth — correct for specs
// whose subject ISN'T auth), this one drives the REAL phone-register UI, because
// the phone-first auth IS the unit under test. OTP is deterministic via the
// Firebase Auth Emulator booted by the harness (global-setup.ts): after the
// client requests a code, we read it back from the emulator's debug endpoint.
//
// Flutter-web specifics this spec relies on: buttons are reached via getByRole
// (the a11y tree exposes their labels), but text fields need real keystrokes
// (click + pressSequentially) because .fill() sets the a11y-proxy value without
// reaching Flutter's TextEditingController. See docs/client/testing/semantics_identifiers.md.
// IMPORTANT: the Flutter bundle is embedded in the Go server binary, so the
// harness must rebuild tmp/server AFTER build:web:e2e (see 2492-web2-pivot-e2e.md).

import { test, expect, type Browser } from '../../lib/fixtures.js';
import { ExperienceService, RSVPIntention } from '../../gen/ripls/api/experience_service_pb.js';
import { createTestClient, installRequestIdOverride } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser, type SeededUser } from '../../lib/seed/users.js';
import { mintExperienceShareLink } from '../../lib/seed/communities.js';
import { createProvisionalUser } from '../../lib/seed/provisional.js';
import { createEventViaWebUI } from '../../lib/ui/create-event.js';

const SPEC_SLUG = 'phone-rsvp-full-loop';
const GUEST_PHONE = '+15551234567';
const EMULATOR_PROJECT = 'demo-ripls';
// The host types this prompt into the unified-create flow. The e2e server's
// deterministic AI provider (--mock-ai-provider) classifies it as an event and
// echoes it into a saveable title + description.
const EVENT_PROMPT = 'Sunset rooftop gathering this Friday at 7pm';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

/**
 * Drive the host's web UI through the two-phase create flow (#2492): open the
 * unified-create modal → Text tab → type a prompt → Generate → Save Event. The
 * server provisions the event's per-item community at save and the share sheet
 * auto-opens (we wait for "Copy Link" as proof). Runs in its own authenticated
 * browser context so it doesn't share state with the unauthenticated guest.
 */
async function hostCreatesEventViaWebUI(browser: Browser, baseUrl: string, host: SeededUser): Promise<void> {
  const ctx = await newRecordingContext(browser);
  try {
    const page = await ctx.newPage();
    await injectAuth(ctx, {
      accessToken: host.accessToken,
      refreshToken: host.refreshToken,
      user: { id: host.userId, name: host.name },
      serverUrl: baseUrl,
    });
    await installRequestIdOverride(page, SPEC_SLUG);
    await createEventViaWebUI(page, baseUrl, { prompt: EVENT_PROMPT });
  } finally {
    await ctx.close();
  }
}

/** Read the verification code the Auth Emulator generated for a phone number. */
async function emulatorVerificationCode(phone: string): Promise<string> {
  const host = process.env.FIREBASE_AUTH_EMULATOR_HOST ?? '127.0.0.1:9099';
  const url = `http://${host}/emulator/v1/projects/${EMULATOR_PROJECT}/verificationCodes`;
  // The code is generated when the client calls sendVerificationCode; poll briefly.
  for (let i = 0; i < 20; i++) {
    const resp = await fetch(url);
    if (resp.ok) {
      const body = (await resp.json()) as {
        verificationCodes?: { phoneNumber: string; code: string }[];
      };
      const match = (body.verificationCodes ?? []).filter((c) => c.phoneNumber === phone).pop();
      if (match?.code) return match.code;
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`no emulator verification code for ${phone}`);
}

test('host creates an event via web UI; guest joins via phone OTP and auto-RSVPs (phone-first full loop)', async ({ page, browser, browserName }) => {
  // The UI-driving (real keystrokes into Flutter-web fields) is validated on
  // chromium; webkit UI driving is out of scope for the WEB-2 milestone and adds
  // flakiness, so skip it there.
  test.skip(browserName === 'webkit', 'phone-OTP + create UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ---- Host registers (RPC; auth only — no login UI), then creates the event
  //      through the real web UI (the two-phase Save-only create flow). ----
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG });
  await hostCreatesEventViaWebUI(browser, baseUrl, host);

  // Read the event the host just created. A freshly-registered host has exactly
  // one experience, born in its per-item community (#2492) — the single entry in
  // shared_community_ids.
  const hostExp = createTestClient(ExperienceService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  const mine = await hostExp.listMyExperiences({});
  const created = mine.experiences[0];
  if (!created?.id || created.sharedCommunityIds.length === 0) {
    throw new Error('UI-created event not found, or it has no per-item community');
  }
  const experienceId = created.id;
  const communityId = created.sharedCommunityIds[0]; // the per-item ad-hoc community

  const shareLink = await mintExperienceShareLink({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId,
    experienceId,
  });
  // Pre-invite the guest by phone → a provisional placeholder promote-on-verify claims.
  await createProvisionalUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId,
    name: 'Phone Guest',
    phoneNumber: GUEST_PHONE,
  });

  // ---- Guest opens the SSR invite landing (real HTML served by the Go server) ----
  await page.goto(`${baseUrl}/go/${shareLink.shortCode}`);
  // The SSR event page's "I'm in" CTA is an <a> to /event/{id}?rsvp=yes&code=…;
  // its accessible name is the aria-label ("RSVP yes to this event"), not the
  // visible "I'm in" text (aria-label overrides text content).
  await page.getByRole('link', { name: /rsvp yes/i }).click();

  // ---- Guest reaches the Flutter web app (unauthenticated) ----
  // Phone-first onboarding (#2492): an RSVP-flavored guest is routed straight
  // to the "Confirm your phone to RSVP" screen — the "Sign in to RSVP" gate and
  // the "You're Invited" register screen are skipped. The phone field's
  // "Send Code" button is the first thing they see.

  // ---- Phone-auth flow (Flutter web) ----
  // Flutter-web text fields ignore Playwright's .fill() (it sets the a11y proxy
  // input's value, which doesn't reach Flutter's TextEditingController). Real
  // keystrokes via click + pressSequentially do reach it (so the controller
  // updates and the disabled-until-input buttons enable).
  const typeInto = async (value: string) => {
    const field = page.getByRole('textbox').first();
    await field.click();
    await field.pressSequentially(value, { delay: 20 });
  };

  // Step 1: phone number → Send Code.
  await page.getByRole('button', { name: /send code/i }).waitFor({ timeout: 20_000 });
  await typeInto(GUEST_PHONE);
  await page.getByRole('button', { name: /send code/i }).click();

  // Step 2: read the emulator-generated OTP, enter it → Verify.
  await page.getByRole('button', { name: /^verify$/i }).waitFor({ timeout: 20_000 });
  const code = await emulatorVerificationCode(GUEST_PHONE);
  await typeInto(code);
  await page.getByRole('button', { name: /^verify$/i }).click();

  // Step 3: "Phone Verified" → name → Finish RSVP (fires PhoneRegister).
  await page.getByRole('button', { name: /finish rsvp/i }).waitFor({ timeout: 20_000 });
  await typeInto('Phone Guest');
  await page.getByRole('button', { name: /finish rsvp/i }).click();

  // ---- Assert: the GUEST's auto-fired RSVP landed server-side ----
  // Match the guest by name, not the total yes-count: the host auto-RSVPs yes
  // on event creation, so a `count > 0` check passes even if the guest's RSVP
  // never landed (the exact false-green the phone-first cache bug hid).
  let guestRsvped = false;
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const resp = await hostExp.getExperience({ id: experienceId, communityId });
    guestRsvped = resp.rsvps.some(
      (r) => r.user?.name === 'Phone Guest' && r.intention === RSVPIntention.RSVP_INTENTION_YES,
    );
    if (guestRsvped) break;
    await new Promise((r) => setTimeout(r, 500));
  }
  expect(guestRsvped, 'guest "Phone Guest" RSVP\'d YES after phone-OTP registration').toBe(true);

  // ---- The invitee reshares (#2630): Who's In → Invite → share sheet ----
  // A yes-RSVP'er is a member of the event's per-item community (#2548), so
  // the share sheet's link fetch (ShareItem, link-only) must succeed for them
  // and render their own /go/ open link. The audience-management rows (Invite
  // people / Share to communities) stay host-only and must be absent.
  await page.goto(`${baseUrl}/event/${experienceId}`);
  await expect(
    page.locator('[flt-semantics-identifier="web-event-screen"]'),
  ).toBeAttached({ timeout: 20_000 });
  await page.getByRole('button', { name: 'View attendees' }).click();
  await page.getByRole('button', { name: 'Invite people to the event' }).click();

  // The sheet loads the guest's own open link (QR + URL + Copy/Share)…
  await expect(page.getByText('Anyone with the link can join')).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText(/\/go\//).first()).toBeVisible();
  await expect(page.getByText('Copy Link')).toBeVisible();
  // …with no host-only audience rows for a non-host member.
  await expect(page.getByRole('button', { name: 'Invite people', exact: true })).toHaveCount(0);
  await expect(
    page.getByRole('button', { name: 'Share to communities', exact: true }),
  ).toHaveCount(0);
});
