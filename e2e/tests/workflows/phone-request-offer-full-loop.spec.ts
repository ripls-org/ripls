// e2e/tests/workflows/phone-request-offer-full-loop.spec.ts
//
// The phone-first pivot extended to requests (epic #2492, WEB-3): a non-app
// guest opens a shared-request invite link, views the request on the web, registers
// via REAL Firebase phone OTP (Auth Emulator), and their OfferToFulfill
// auto-fires as they join the request's ad-hoc community and their pre-seeded
// provisional identity is promoted to the real account. Mirrors
// phone-rsvp-full-loop and docs/workflows/phone_first_request_offer.md.
//
// The host's request is RPC-seeded off-camera (SubmitRequest provisions the
// per-item community and shares the request in) — the unit under test is the
// guest's phone-first OfferToFulfill loop.

import { test, expect } from '../../lib/fixtures.js';
import { RequestService } from '../../gen/ripls/api/request_service_pb.js';
import { RequestState } from '../../gen/ripls/api/request_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser } from '../../lib/seed/users.js';
import { seedRequest } from '../../lib/seed/requests.js';
import { mintRequestShareLink } from '../../lib/seed/communities.js';
import { createProvisionalUser } from '../../lib/seed/provisional.js';
import { completePhoneRegister } from '../../lib/ui/phone-register.js';

const SPEC_SLUG = 'phone-request-offer-full-loop';
const GUEST_PHONE = '+15551234572';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('guest offers to help on a shared request via phone OTP (phone-first full loop)', async ({
  page,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'phone-OTP UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ---- Host registers + submits a request (RPC; off-camera). ----
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Aisha Rahman' });
  const request = await seedRequest({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    title: 'An extension ladder for the weekend',
  });
  const shareLink = await mintRequestShareLink({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: request.communityId,
    requestId: request.requestId,
  });
  await createProvisionalUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: request.communityId,
    name: 'Carlos Mendez',
    phoneNumber: GUEST_PHONE,
  });

  // ---- Guest opens the SSR request landing (real HTML served by the Go server). ----
  await page.goto(`${baseUrl}/go/${shareLink.shortCode}`);
  // Dwell on the landing so the recorded video is easy for a human to follow.
  await page.waitForTimeout(1500);
  // The request landing's primary CTA is an <a> to /need/{id}?intent=offer&code=…;
  // its accessible name is the aria-label ("Offer to help Aisha").
  await page.getByRole('link', { name: /offer to help/i }).click();

  // ---- Phone-first register (the unit under test). ----
  // The request guest sees "Confirm your phone to help with {item}" and finishes
  // with the generic "Finish" button, which fires PhoneRegister then auto-fires
  // OfferToFulfill after the community join.
  await completePhoneRegister(page, {
    phone: GUEST_PHONE,
    name: 'Carlos Mendez',
    finishLabel: /^finish$/i,
  });

  // ---- Assert: the GUEST's auto-fired OfferToFulfill landed server-side. ----
  // The host can't offer on their own request, so the ACTIVE → OFFERS_RECEIVED
  // transition proves the guest's offer closed the loop.
  const hostRequest = createTestClient(RequestService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  let guestOffered = false;
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const resp = await hostRequest.getRequest({
      requestId: request.requestId,
      communityId: request.communityId,
    });
    guestOffered = resp.request?.state === RequestState.OFFERS_RECEIVED;
    if (guestOffered) break;
    await new Promise((r) => setTimeout(r, 500));
  }
  expect(guestOffered, 'guest offered to help on the request after phone-OTP registration').toBe(
    true,
  );

  // Let the in-app item view (hero image) settle on camera before the video ends.
  await page.waitForTimeout(2500);
});
