// e2e/tests/walkthroughs/requests.spec.ts
//
// @walkthrough — the REQUESTS WALKTHROUGH (#2686, upgraded for #2702): five
// paced scenes of the request-morphs-into-a-loan journey, fabricated cast from
// the mower pack (e2e/fixtures/walkthroughs/mower/ — the lawn-mower hero is
// Unsplash stock, attribution in ../manifest.json):
//
//   01 June's mower dies mid-mow — one typed sentence becomes the request,
//      born with its claimable "Lawn mower" need (visible and editable on the
//      preview), shared to her street with the link copied for Theo next door
//   02 Theo (no app, no account) opens the link, joins via phone OTP, and
//      claims the need WITH HIS ACTUAL MOWER — a photo becomes a listing,
//      lend stays the default, and confirming sends a real loan offer
//   03 June finds a mower on the request — not just a hand raised — the row
//      wears its "Offered" pill, she taps it to accept ("Accepted ✓"), and
//      says thanks in the thread; Theo replies live
//   04 June grabs it from the side gate; Theo marks the handoff from his
//      phone — and the request closes ITSELF on June's open screen, counted
//      with the mower's real value
//   05 The closing look: the fulfilled request, the loan behind it, the
//      receipt
//
// One scene = one test = one actor on one recording context = one clip
// (lib/walkthrough.ts); everything else happens off camera via RPC seed
// helpers, including MID-SCENE actions the camera watches land live (Theo's
// chat reply in 03, his handoff in 04). Scenes run serially in one worker,
// sharing the hermetic server + DB, so the world accumulates scene over
// scene. Render:
//
//   e2e/scripts/run_walkthrough.sh requests
//
// This is a SHOWCASE, not a correctness gate — off-camera RPC mid-scene is by
// design here (see e2e/README.md's UI-only rule, which binds workflow specs).
// The correctness twins live in tests/workflows/ (phone-request-offer-full-loop,
// request-authoring-share) and server/integration_tests/request_to_loan_test.go.

import { resolve } from 'node:path';
import { test, expect } from '../../lib/fixtures.js';
import { RequestService } from '../../gen/ripls/api/request_service_pb.js';
import { RequestState } from '../../gen/ripls/api/request_pb.js';
import { TransferState } from '../../gen/ripls/api/transfer_pb.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import { UserService } from '../../gen/ripls/api/user_service_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser, registerUserViaInvite, type SeededUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createLocation } from '../../lib/seed/locations.js';
import { setUserAvatar } from '../../lib/seed/attendees.js';
import { sendRequestChatMessage } from '../../lib/seed/chat.js';
import { updateRequestContent } from '../../lib/seed/requests.js';
import { createProvisionalUser } from '../../lib/seed/provisional.js';
import { startGearLoan } from '../../lib/seed/transfers.js';
import { loginPhoneUser, type SeededPhoneUser } from '../../lib/seed/phone-user.js';
import { emulatorVerificationCode } from '../../lib/ui/phone-register.js';
import { openScene, requireBaseUrl, settle, typeInto } from '../../lib/walkthrough.js';

const SPEC_SLUG = 'requests';

const FIXTURES = resolve(__dirname, '..', '..', 'fixtures', 'walkthroughs');
const fixtureFile = (f: string) => resolve(FIXTURES, f);

const COMMUNITY_NAME = 'Cedar Court Neighbors';
const GUEST_PHONE = '+15551234581';

// Typed on camera. The e2e provider titles the request with the first
// sentence, keeps the REST of the prompt as the description, and extracts
// "Lawn mower" as the seeded need (server/ai/provider_e2e.go, #2702). The
// prompt IS the final copy — an off-camera title/description tidy used to
// visibly mutate the quote card between scenes (#2724). Keep gear cues
// ("borrow"/"lend"/"loan") OUT — the classifier checks gear before request
// and would misroute (see lib/ui/create-item.ts).
const REQUEST_TITLE = 'A lawn mower for the weekend';
const REQUEST_DESCRIPTION =
  'Ours died halfway through the front yard. One afternoon is all I need — happy to return it fueled, clean, and grateful.';
const CREATE_PROMPT = `${REQUEST_TITLE}. ${REQUEST_DESCRIPTION}`;
const SEEDED_NEED = 'Lawn mower';

// The deterministic listing the e2e provider writes for the mower photo
// (e2eGearDetectionForFilename keys on the uploaded filename).
const GEAR_TITLE = 'Honda Self-Propelled Mower';

// Typed on camera by June (keep it ASCII — it goes through real keystrokes).
const JUNE_THANKS = "Theo, you're a lifesaver. The grass was starting to win.";
// Seeded as Theo's live reply.
const THEO_REPLY = "It's in the garage — come grab it whenever. Side gate's open.";

// ── World state accumulated across scenes (same worker, same server+DB) ──
let baseUrl: string;
let june: SeededUser;
let theo: SeededPhoneUser;
let requestId: string;
let adhocCommunityId: string;
let shareCode: string;
let offerTransferId: string;

const asActor = (user: { accessToken: string; refreshToken: string; userId: string; name: string }) => ({
  accessToken: user.accessToken,
  refreshToken: user.refreshToken,
  user: { id: user.userId, name: user.name },
  serverUrl: baseUrl,
});

const rpc = (user: { accessToken: string }) =>
  ({ baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken }) as const;

const requestClient = (user: { accessToken: string }) =>
  createTestClient(RequestService, rpc(user));

/** Open the request's content view directly (/request/:id). One boot per
 * scene keeps the clip clean — the head is trimmed at export. "Open
 * conversation" renders in every request state, so it doubles as the
 * readiness signal. */
async function openRequest(page: import('@playwright/test').Page): Promise<void> {
  await page.goto(`${baseUrl}/request/${requestId}`);
  await page.getByRole('button', { name: /open conversation/i }).waitFor({ timeout: 30_000 });
  await page.waitForTimeout(1_500); // let the pane settle
}

async function pollFor<T>(fn: () => Promise<T | undefined>, timeoutMs = 30_000): Promise<T | undefined> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const v = await fn();
    if (v !== undefined) return v;
    await new Promise((r) => setTimeout(r, 500));
  }
  return undefined;
}

test.describe.configure({ mode: 'serial' });

test('@walkthrough 01 the request', async ({ browser }) => {
  test.setTimeout(300_000);
  baseUrl = requireBaseUrl();

  // ---- OFF CAMERA: June, her street, and two neighbors already on it.
  //      Avatars come from the committed stock set (Unsplash, ../manifest.json). ----
  june = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'June Park' });
  await setUserAvatar({
    baseUrl, specSlug: SPEC_SLUG, accessToken: june.accessToken,
    userId: june.userId, avatarPath: fixtureFile('avatar-6.jpg'),
  });

  // June's home — request generation falls back to the requester's primary
  // residence, so the request carries a real neighborhood.
  const homeId = await createLocation({
    baseUrl, specSlug: SPEC_SLUG, accessToken: june.accessToken,
    name: '2107 Cedar Court', addressLines: ['2107 Cedar Court'],
    locality: 'Austin', administrativeArea: 'Texas', regionCode: 'US',
    postalCode: '78704', latitudeDeg: 30.243, longitudeDeg: -97.748,
  });
  await createTestClient(UserService, rpc(june)).saveUser({
    userId: june.userId, primaryResidenceLocationId: homeId,
  });

  const cedar = await createCommunity({
    baseUrl, specSlug: SPEC_SLUG, accessToken: june.accessToken, name: COMMUNITY_NAME,
  });
  for (const [name, avatar] of [
    ['Rosa Delgado', 'avatar-9.jpg'],
    ['Ben Okafor', 'avatar-1.jpg'],
  ] as const) {
    const user = await registerUserViaInvite({
      baseUrl, specSlug: SPEC_SLUG, inviterAccessToken: june.accessToken,
      communityId: cedar.communityId, name,
    });
    await setUserAvatar({
      baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken,
      userId: user.userId, avatarPath: fixtureFile(avatar),
    });
  }

  // ---- ON CAMERA: one typed sentence becomes the request. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-01`, actor: asActor(june) });
  const { page } = scene;
  try {
    await page.goto(`${baseUrl}/`);
    // Wait for a rendered Home before the first settle — the first settle
    // sets the export's head-trim point. The semantics tree lands before the
    // canvas paints on skwasm, so give pixels a beat too (#2724).
    await page.getByRole('button', { name: /^create$/i }).waitFor({ timeout: 30_000 });
    await page.waitForTimeout(1_200);
    await settle(page, 'Home — the grass won');

    await page.getByRole('button', { name: /^create$/i }).click();
    await page.getByRole('button', { name: /^text$/i }).click();
    await settle(page, 'Create — text tab');
    await typeInto(page, CREATE_PROMPT);
    await settle(page, 'The request, typed like a text');
    await page.getByRole('button', { name: /^draft it$/i }).click();
    const saveBtn = page.getByRole('button', { name: /^save request$/i });
    await saveBtn.waitFor({ timeout: 30_000 });
    // The preview surfaces the extracted need on an editable "Needs" row
    // (#2702) — the requester sees exactly what helpers will be able to
    // claim before saving.
    await settle(page, 'The sentence became a request — need and all');

    // June snaps the dead mower ON CAMERA so the request is born with its
    // hero: the share sheet now opens over the request's own view (#2724
    // H1), and without a hero that view is a black placeholder — production
    // fills one from stock imagery, which the e2e stack has no provider for.
    await page.getByRole('button', { name: /add a photo/i }).click();
    const photosBtn = page.getByRole('button', { name: /^photos$/i });
    await photosBtn.waitFor({ timeout: 20_000 });
    const heroChooser = page.waitForEvent('filechooser', { timeout: 20_000 });
    await photosBtn.click();
    await (await heroChooser).setFiles(fixtureFile('request-lawn-mower.jpg'));
    await settle(page, 'The mower that quit, photographed');

    await saveBtn.click();

    // The share sheet auto-opens (two-phase create → share, #2492).
    await page.getByRole('button', { name: /copy link/i }).waitFor({ timeout: 20_000 });

    // OFF CAMERA, while the sheet covers the screen: discover the created
    // request, mint its share audience, and re-assert the on-camera content
    // with the committed mower hero (the hermetic server has no stock-imagery
    // provider, so without this the SSR landing falls back to the logo).
    const found = await pollFor(async () => {
      const mine = await requestClient(june).listMyRequests({});
      return mine.requests.find((r) => r.title === REQUEST_TITLE)?.id;
    }, 15_000);
    if (!found) throw new Error(`UI-created request "${REQUEST_TITLE}" not found via ListMyRequests`);
    requestId = found;
    const share = await createTestClient(CommunityService, rpc(june)).shareItem({
      item: { case: 'requestId', value: requestId },
    });
    if (!share.adhocCommunityId || !share.shareUrl) {
      throw new Error('ShareItem did not return adhoc community + share url');
    }
    adhocCommunityId = share.adhocCommunityId;
    shareCode = share.shareUrl.split('/').pop() ?? '';
    if (!shareCode) throw new Error(`no short code in share url ${share.shareUrl}`);
    // Pin ONLY the location — title, description, and hero all keep their
    // on-camera values (re-asserting them mutated the quote card between
    // scenes, #2724).
    await updateRequestContent({
      ...rpc(june), requestId,
      locationId: homeId, // WHERE reads as her street, not "TBD"
    });
    await settle(page, 'Share sheet');

    // Copy the link for the text thread FIRST — confirming a community share
    // dismisses the whole sheet.
    await page.getByRole('button', { name: /copy link/i }).click();
    await settle(page, 'Link copied — for Theo next door');

    await page.getByRole('button', { name: /share to communities/i }).click();
    await settle(page, 'Pick the street');
    await page.getByText(COMMUNITY_NAME, { exact: true }).first().click();
    await settle(page, 'Cedar Court picked');
    await page.getByRole('button', { name: /^confirm$/i }).click();
    await settle(page, 'Shared with the street');
  } finally {
    await scene.close();
  }

  // OFF CAMERA: June "texts Theo the link" — pre-invite him by phone (a
  // provisional member the phone-first flow promotes on verify).
  await createProvisionalUser({
    baseUrl, specSlug: SPEC_SLUG, accessToken: june.accessToken,
    communityId: adhocCommunityId, name: 'Theo Alvarez', phoneNumber: GUEST_PHONE,
  });

  // Server-side truth: the request is live with the mower hero attached —
  // and it was BORN with its claimable need (#2702), so helpers always have
  // something concrete to pick up.
  const detail = await requestClient(june).getRequest({ requestId });
  expect(detail.request?.state, 'request is live').toBe(RequestState.ACTIVE);
  expect(detail.request?.mediaIds?.length ?? 0, 'mower hero attached').toBeGreaterThan(0);
  const planning = await requestClient(june).listRequestNeedsAndContributions({ requestId });
  expect(
    planning.needs.map((n) => n.name),
    'the request was born with its seeded need',
  ).toContain(SEEDED_NEED);
});

test('@walkthrough 02 a mower steps up', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: Theo taps the link from his texts — no app, no account —
  //      joins via phone OTP (epic #2492, WEB-3), and then does the new thing
  //      (#2702): claims the "Lawn mower" need with HIS mower. A photo
  //      becomes a listing, lend stays the default, and confirming sends a
  //      real loan offer to June. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-02` });
  const { page } = scene;
  try {
    // Navigate by short code against the LOCAL server — the display host is
    // ripls.app (E2E_INVITE_LINK_HOSTNAME) for camera realism.
    await page.goto(`${baseUrl}/go/${shareCode}`);
    await page.getByRole('link', { name: /offer to help/i }).waitFor({ timeout: 20_000 });
    await settle(page, 'The request, as a real webpage');
    await page.getByRole('link', { name: /offer to help/i }).click();

    await page.getByRole('button', { name: /send code/i }).waitFor({ timeout: 30_000 });
    await settle(page, 'Confirm your phone');
    await typeInto(page, GUEST_PHONE);
    await page.getByRole('button', { name: /send code/i }).click();

    await page.getByRole('button', { name: /^verify$/i }).waitFor({ timeout: 20_000 });
    const code = await emulatorVerificationCode(GUEST_PHONE);
    await typeInto(page, code);
    // Let the six typed digits read on camera — without this dwell the whole
    // verification screen flashed by in under a second (#2724).
    await settle(page, 'Six digits from a text');
    await page.getByRole('button', { name: /^verify$/i }).click();

    await page.getByRole('button', { name: /^finish$/i }).waitFor({ timeout: 20_000 });
    await typeInto(page, 'Theo Alvarez');
    await settle(page, 'Just a name');
    await page.getByRole('button', { name: /^finish$/i }).click();

    // Theo's promoted-account offer auto-fires server-side; he lands on the
    // in-app request view with the seeded need visible on the card.
    await page.getByRole('button', { name: /open conversation/i }).waitFor({ timeout: 30_000 });
    await settle(page, "He's in — and the request names what it needs");

    // The claim-first flow (#2702): open the Who's-helping panel, tap the
    // need.
    await page.mouse.wheel(0, 300);
    await page.waitForTimeout(800);
    await page.getByRole('button', { name: /view helpers/i }).click();
    await page.getByRole('button', { name: SEEDED_NEED }).first().waitFor({ timeout: 20_000 });
    await settle(page, 'One thing still needed');
    await page.getByRole('button', { name: SEEDED_NEED }).first().click();

    // The claim sheet — "Link an item · Add or link yours (optional)". Theo's
    // library is empty, so he adds a new item from the picker.
    await page
      .getByRole('button', { name: /link an item/i })
      .waitFor({ timeout: 20_000 });
    await settle(page, "I'll bring this — with what, exactly?");
    await page.getByRole('button', { name: /link an item/i }).click();
    // The dashed "Add a new item" row (full unified create — photo, text, or
    // link; opens on photo capture). Its merged semantics label is the
    // stable locator on Flutter Web.
    const addItemRow = page
      .getByRole('button', { name: /add a new item to your library/i })
      .first();
    await addItemRow.waitFor({ timeout: 20_000 });
    await settle(page, 'His mower is one photo away');

    await addItemRow.click();
    const galleryBtn = page.getByRole('button', { name: /open gallery/i });
    await galleryBtn.waitFor({ timeout: 20_000 });
    // Arm the chooser immediately before the click that opens it — armed
    // any earlier, the intervening waits burn its timeout.
    const chooser = page.waitForEvent('filechooser', { timeout: 20_000 });
    await galleryBtn.click();
    // Theo's OWN mower — a different photo than June's request hero (the
    // same file in both roles read as June owning the mower she asked for,
    // #2724). Any "mower" filename maps to the deterministic Honda listing.
    await (await chooser).setFiles(fixtureFile('gear-mower.jpg'));

    // The photo becomes a listing (deterministic e2e detection by filename);
    // saving auto-links it back onto the claim (capture is save-only — no
    // share sheet; the offer share happens with the claim itself).
    const saveBtn = page.getByRole('button', { name: /^save$/i });
    await saveBtn.waitFor({ timeout: 30_000 });
    await settle(page, 'Photo became a listing');
    await saveBtn.click();

    // Back on the claim sheet: gear linked, Lend/Give choice visible with
    // lend as the default — the item comes back.
    const confirmCta = page.getByRole('button', { name: /i'll bring this/i });
    await confirmCta.waitFor({ timeout: 30_000 });
    await settle(page, 'Lend it — it comes back');
    await confirmCta.click();

    // The claim lands AND escalates into a real loan offer (#2702) — the
    // undoable toast says the mower is now visible to the group.
    await settle(page, 'A real loan offer, sent');

    const offer = await pollFor(async () => {
      const resp = await requestClient(june).getRequest({ requestId });
      const o = resp.request?.gearOffers?.[0];
      return o && o.state === TransferState.RECIPIENT_SELECTED ? o : undefined;
    });
    expect(offer, 'the claim escalated into a live loan offer').toBeTruthy();
    offerTransferId = offer!.transferId;
    expect(offer!.gearName, 'the offer carries the real mower').toBe(GEAR_TITLE);
    await settle(page, 'His mower, on the request');
  } finally {
    await scene.close();
  }

  // OFF CAMERA: recover Theo's session (the UI flow never exposes tokens) so
  // later scenes can act as him — avatar now, chat reply in 03, handoff in 04.
  theo = await loginPhoneUser({ baseUrl, specSlug: SPEC_SLUG, phoneNumber: GUEST_PHONE });
  await setUserAvatar({
    baseUrl, specSlug: SPEC_SLUG, accessToken: theo.accessToken,
    userId: theo.userId, avatarPath: fixtureFile('avatar-4.jpg'),
  });
});

test('@walkthrough 03 offer accepted', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: June finds a mower on her request — not just a hand
  //      raised. The row wears its "Offered" pill; she taps it ("Accepted ✓")
  //      and says thanks where the street can see. No reload mid-scene: the
  //      panel closes back onto the same screen and the thread opens from
  //      there. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-03`, actor: asActor(june) });
  const { page } = scene;
  try {
    await openRequest(page);
    await settle(page, 'Not just a hand raised — a mower');

    // The card's need row carries the LENDING tag; the "Offered" accept
    // toggle lives on the expanded Who's-helping panel.
    await page.mouse.wheel(0, 300);
    await page.waitForTimeout(800);
    await settle(page, 'Lending — it says so on the card');
    await page.getByRole('button', { name: /view helpers/i }).click();
    // The pill is a Toggle rendered as a sibling of the row — reach it via
    // its semanticsLabel through the browser a11y tree (identifiers on
    // Toggle never reach DOM; see docs/client/testing/semantics_identifiers.md).
    const acceptChip = page.getByRole('button', { name: /accept this offer/i }).first();
    await acceptChip.waitFor({ timeout: 20_000 });
    await settle(page, 'Offered — it says so on the row');
    await acceptChip.click();

    const accepted = await pollFor(async () => {
      const resp = await requestClient(june).getRequest({ requestId });
      const o = resp.request?.gearOffers?.find((g) => g.transferId === offerTransferId);
      return o?.acceptedAtUnixSec ? true : undefined;
    });
    expect(accepted, "June accepted Theo's offer").toBe(true);
    await settle(page, 'Accepted');

    // Thanks, in the open — close the panel (same screen, no reload) and
    // open the thread. UNANCHORED name match: IconAction's accessible name
    // merges the icon label with its tooltip, so /^close$/ never matches
    // (same reason scene 05 matches /edit request/ unanchored). last() in
    // case the non-opaque route keeps an underlying close in the tree.
    await page.getByRole('button', { name: /close/i }).last().click();
    await page.getByRole('button', { name: /open conversation/i }).waitFor({ timeout: 20_000 });
    await page.getByRole('button', { name: /open conversation/i }).click();
    await settle(page, 'The thread on the request');
    await typeInto(page, JUNE_THANKS);
    await page.getByRole('button', { name: /send message/i }).click();
    await settle(page, 'Thanks, said in the open');

    // OFF CAMERA, mid-scene: Theo's reply lands while June watches.
    await sendRequestChatMessage({ ...rpc(theo), requestId, text: THEO_REPLY });
    await settle(page, 'Side gate is open');
  } finally {
    await scene.close();
  }
});

// Title kept SHORT: long titles get hash-truncated in Playwright's output
// dir names, which scrambles the exporter's scene ordering
// (docs/walkthroughs.md → short ASCII titles).
test('@walkthrough 04 it closes itself', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: June's view of the moment the mower changes hands. She
  //      goes to grab it from the side gate (Theo's invitation from scene
  //      03); Theo marks the handoff from his phone (off camera) — the loan
  //      goes active, and June's OPEN SCREEN closes the request by itself
  //      (the fulfillment event streams in and the view refreshes live),
  //      counted with the mower's real value (#2702, fixes the $0 "backdoor
  //      loan", #2275). ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-04`, actor: asActor(june) });
  const { page } = scene;
  try {
    await openRequest(page);
    await settle(page, 'Saturday — June goes to grab it');

    // OFF CAMERA, mid-scene: Theo marks the handoff. The bus coupling
    // auto-fulfills the single-need request while the camera stays on
    // June's screen.
    await startGearLoan({ ...rpc(theo), transferId: offerTransferId });
    const fulfilled = await pollFor(async () => {
      const resp = await requestClient(june).getRequest({ requestId });
      return resp.request?.state === RequestState.FULFILLED ? true : undefined;
    });
    expect(fulfilled, 'the handoff fulfilled the request').toBe(true);

    // The REQUEST_FULFILLED event streams to the open view (event_router →
    // content cache invalidation → silent refresh) — give the morph a beat
    // to land on camera, then linger on it.
    await page.waitForTimeout(3_000);
    await settle(page, 'The request closed itself — live');
    await page.mouse.wheel(0, 400);
    await page.waitForTimeout(800);
    await settle(page, 'Counted — with the real mower');
  } finally {
    await scene.close();
  }

  // Server-side truth: fulfillment credits Theo and points at his loan.
  const detail = await requestClient(june).getRequest({ requestId });
  expect(detail.request?.confirmedHelperIds ?? [], 'Theo is the confirmed helper')
    .toContain(theo.userId);
  const offer = detail.request?.gearOffers?.find((g) => g.transferId === offerTransferId);
  expect(offer?.state, 'the loan is active').toBe(TransferState.ACTIVE);
});

test('@walkthrough 05 what it added up to', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: the closing look — the request as a small record of a
  //      favor that became a real loan. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-05`, actor: asActor(june) });
  const { page } = scene;
  try {
    await openRequest(page);
    await settle(page, 'The request, closed');
    // Bring the inline impact row fully into view (bottom-edge widgets'
    // semantics are culled until scrolled).
    await page.mouse.wheel(0, 400);
    await page.waitForTimeout(800);
    await settle(page, 'What it added up to');

    // The manage overflow survives fulfillment (owner + not-cancelled) and
    // gains the View Impact row.
    await page.getByRole('button', { name: /edit request/i }).click();
    await page.getByRole('button', { name: /view impact/i }).click();
    await settle(page, 'The receipt');
    await settle(page, 'One photo away from lent');
  } finally {
    await scene.close();
  }

  // Read-only scene: the assert just fails the render if the state regressed.
  const detail = await requestClient(june).getRequest({ requestId });
  expect(detail.request?.state, 'still fulfilled at close').toBe(RequestState.FULFILLED);
});
