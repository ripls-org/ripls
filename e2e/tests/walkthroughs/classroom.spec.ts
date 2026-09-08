// e2e/tests/walkthroughs/classroom.spec.ts
//
// @walkthrough — the CLASSROOM SUPPLY-DRIVE reel (#2703), the second
// requests-family reel after the single-helper mower request (#2686). A teacher
// posts one request broken into pieces; parents fulfill it mostly by GIVING
// supplies (giveaways) with one LENDING a whiteboard for the year — the
// multi-helper generalization of the #2702 escalation, giveaway-dominant. It is
// the first reel to exercise the request-side Needs & Contributions layer
// (docs/planning.md) AND the complete-on-fulfill delivery of confirmed gear
// offers (#2703 server fix): marking the drive fulfilled hands the donations
// over (giveaways complete, the whiteboard loan goes active).
//
// Fabricated cast from the classroom pack (copy in
// e2e/fixtures/walkthroughs/classroom/walkthrough.md; the classroom hero and the
// item photos are Unsplash stock at the fixtures top level, attribution in
// ../manifest.json). Five paced scenes:
//
//   01 Ms. Okafor's Room 7 is bare two weeks out — a typed sentence becomes the
//      request, broken into a supply list, shared to the class parents and
//      copied to paste into the parents' WhatsApp group
//   02 Parents step up on her open view: Theo gives a box of picture books,
//      Nadia gives storage bins, Dana gives an art caddy, and Bea lends a
//      whiteboard for the year — real give/lend offers landing on the request
//   03 Rob, a parent with no app, gets the link in the WhatsApp group, joins by
//      phone OTP, and grabs the last piece
//   04 The parents drop everything off; Ms. Okafor marks the drive fulfilled and
//      confirms the crew — and the gifts actually land (giveaways complete, the
//      whiteboard loan goes active) instead of evaporating
//   05 The closing look: a stocked classroom, the parents who filled it, and
//      what a week of generosity added up to
//
// One scene = one test = one actor on one recording context = one clip
// (lib/walkthrough.ts); everything else happens off camera via RPC seed
// helpers. Scenes run serially in one worker, sharing the hermetic server + DB.
// Render:
//
//   e2e/scripts/run_walkthrough.sh classroom
//
// This is a SHOWCASE, not a correctness gate — off-camera RPC mid-scene is by
// design (see e2e/README.md's UI-only rule, which binds workflow specs). The
// correctness twins live in tests/workflows/ (phone-first loops) and
// server/integration_tests/request_to_giveaway_test.go (the delivery fix).

import { resolve } from 'node:path';
import { test, expect } from '../../lib/fixtures.js';
import { RequestService } from '../../gen/ripls/api/request_service_pb.js';
import { RequestState } from '../../gen/ripls/api/request_pb.js';
import { TransferService } from '../../gen/ripls/api/transfer_service_pb.js';
import { TransferState, TransferType } from '../../gen/ripls/api/transfer_pb.js';
import { Availability } from '../../gen/ripls/api/gear_pb.js';
import { MaterialCategory } from '../../gen/ripls/api/common_pb.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import { UserService } from '../../gen/ripls/api/user_service_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser, registerUserViaInvite, type SeededUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createLocation } from '../../lib/seed/locations.js';
import { setUserAvatar } from '../../lib/seed/attendees.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { updateRequestContent } from '../../lib/seed/requests.js';
import { claimRequestNeed } from '../../lib/seed/planning.js';
import { offerRequestTransfer } from '../../lib/seed/transfers.js';
import { createProvisionalUser } from '../../lib/seed/provisional.js';
import { emulatorVerificationCode } from '../../lib/ui/phone-register.js';
import { openScene, requireBaseUrl, settle, typeInto } from '../../lib/walkthrough.js';

const SPEC_SLUG = 'classroom';

const FIXTURES = resolve(__dirname, '..', '..', 'fixtures', 'walkthroughs');
const fixtureFile = (f: string) => resolve(FIXTURES, f);

const COMMUNITY_NAME = 'Room 7 Families';
const GUEST_PHONE = '+15551234583';
const SCHOOL_YEAR_DAYS = 180;

// Typed on camera — and it IS the final copy: the provider titles the request
// with the first clause and keeps the rest as the description, so nothing
// gets renamed off camera (a title/description tidy used to visibly mutate
// the share sheet vs the landing page, #2724). "need" routes the classifier
// to Request (not gear/event). Keep gear cues ("borrow"/"lend"/"loan") OUT or
// the create flow misroutes to gear — the LENDING happens later, on a parent's
// offer, never in the request text.
//
// Names the whole supply list in the request text: create extracts every named
// thing and the request is born with one claimable need per item (#2731). Keep
// gear cues ("borrow"/"lend"/"loan") OUT so the classifier routes to Request,
// not gear — the lending happens later, on a parent's offer.
const REQUEST_TITLE = 'Back-to-school supplies for Room 7';
const REQUEST_DESCRIPTION =
  'The new year starts in two weeks and the classroom is bare. We need picture books, a whiteboard, storage bins, art supplies, and construction paper — new or gently used, anything helps. Thank you!';
const CREATE_PROMPT = `${REQUEST_TITLE}. ${REQUEST_DESCRIPTION}`;

// The supply list — every item named in the request text above, extracted at
// create (deterministic e2e provider mirrors what the real model does).
const NEED_BOOKS = 'Picture books';
const NEED_WHITEBOARD = 'Whiteboard';
const NEED_BINS = 'Storage bins';
const NEED_ART = 'Art supplies';
const NEED_PAPER = 'Construction paper';

// ── World state accumulated across scenes (same worker, same server+DB) ──
let baseUrl: string;
let maya: SeededUser;
let theo: SeededUser;
let nadia: SeededUser;
let dana: SeededUser;
let bea: SeededUser;
let requestId: string;
let roomCommunityId: string;
let adhocCommunityId: string;
let shareCode: string;
let schoolLocationId: string;
const needIds = new Map<string, string>();
// give/lend offer transfer ids, keyed by the giver's name.
const offerTransferIds = new Map<string, string>();

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
const transferClient = (user: { accessToken: string }) =>
  createTestClient(TransferService, rpc(user));

async function openRequest(page: import('@playwright/test').Page): Promise<void> {
  await page.goto(`${baseUrl}/request/${requestId}`);
  await page.getByRole('button', { name: /open conversation/i }).waitFor({ timeout: 30_000 });
  await page.waitForTimeout(1_500);
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

async function refreshNeedIds(): Promise<void> {
  const planning = await requestClient(maya).listRequestNeedsAndContributions({ requestId });
  needIds.clear();
  for (const n of planning.needs) needIds.set(n.name, n.id);
}

/** Off camera: a parent gives (or lends) their actual item toward a need. Seeds
 * the gear, claims the need with it linked, then escalates into a real
 * give/lend offer targeting the teacher — exactly the #2702 client flow. */
async function pledgeGear(
  giver: SeededUser,
  need: string,
  gearName: string,
  gearFile: string,
  transferType: TransferType,
  impact: { valueUsd: number; weightGrams: number; material: MaterialCategory },
): Promise<void> {
  const gear = await seedSharedGear({
    ...rpc(giver),
    name: gearName,
    heroImagePath: fixtureFile(gearFile),
    availability:
      transferType === TransferType.GIVEAWAY ? Availability.FOR_GIVEAWAY : Availability.FOR_LOAN,
    impact,
  });
  const contributionId = await claimRequestNeed({
    ...rpc(giver), requestId, needId: needIds.get(need)!, communityId: roomCommunityId, gearId: gear.gearId,
  });
  const transferId = await offerRequestTransfer({
    ...rpc(giver),
    gearId: gear.gearId,
    transferType,
    requesterId: maya.userId,
    communityId: roomCommunityId,
    requestId,
    contributionId,
    loanDurationDays: transferType === TransferType.LOAN ? SCHOOL_YEAR_DAYS : undefined,
  });
  offerTransferIds.set(giver.name, transferId);
}

test.describe.configure({ mode: 'serial' });

test('@walkthrough 01 the supply list', async ({ browser }) => {
  test.setTimeout(300_000);
  baseUrl = requireBaseUrl();

  // ---- OFF CAMERA: Ms. Okafor, her class-parents group, and four parents. ----
  maya = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Maya Okafor' });
  await setUserAvatar({
    baseUrl, specSlug: SPEC_SLUG, accessToken: maya.accessToken,
    userId: maya.userId, avatarPath: fixtureFile('avatar-2.jpg'),
  });

  // Her school — request generation falls back to the requester's primary
  // residence, so pin a real place for a non-"TBD" WHERE.
  schoolLocationId = await createLocation({
    baseUrl, specSlug: SPEC_SLUG, accessToken: maya.accessToken,
    name: 'Bexley Elementary — Room 7', addressLines: ['500 Cassingham Blvd'],
    locality: 'Bexley', administrativeArea: 'Ohio', regionCode: 'US',
    postalCode: '43209', latitudeDeg: 39.964, longitudeDeg: -82.937,
  });
  await createTestClient(UserService, rpc(maya)).saveUser({
    userId: maya.userId, primaryResidenceLocationId: schoolLocationId,
  });

  const room = await createCommunity({
    baseUrl, specSlug: SPEC_SLUG, accessToken: maya.accessToken, name: COMMUNITY_NAME,
  });
  roomCommunityId = room.communityId;

  const parents: Array<[SeededUser | undefined, string, string]> = [
    [undefined, 'Theo Bell', 'avatar-1.jpg'],
    [undefined, 'Nadia Cruz', 'avatar-7.jpg'],
    [undefined, 'Dana Cho', 'avatar-9.jpg'],
    [undefined, 'Bea Ruiz', 'avatar-3.jpg'],
  ];
  for (const entry of parents) {
    const user = await registerUserViaInvite({
      baseUrl, specSlug: SPEC_SLUG, inviterAccessToken: maya.accessToken,
      communityId: roomCommunityId, name: entry[1],
    });
    await setUserAvatar({
      baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken,
      userId: user.userId, avatarPath: fixtureFile(entry[2]),
    });
    entry[0] = user;
  }
  theo = parents[0][0]!;
  nadia = parents[1][0]!;
  dana = parents[2][0]!;
  bea = parents[3][0]!;

  // ---- ON CAMERA: one typed sentence becomes the request. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-01`, actor: asActor(maya) });
  const { page } = scene;
  try {
    await page.goto(`${baseUrl}/`);
    // The semantics tree lands before the canvas paints on skwasm, so give
    // pixels a beat before the head-trim settle (#2724).
    await page.getByRole('button', { name: /^create$/i }).waitFor({ timeout: 30_000 });
    await page.waitForTimeout(1_200);
    await settle(page, 'Home — a bare classroom to fill');

    await page.getByRole('button', { name: /^create$/i }).click();
    await page.getByRole('button', { name: /^text$/i }).click();
    await settle(page, 'Create — text tab');
    await typeInto(page, CREATE_PROMPT);
    await settle(page, 'The request, typed like a note home');
    await page.getByRole('button', { name: /^draft it$/i }).click();
    const saveBtn = page.getByRole('button', { name: /^save request$/i });
    await saveBtn.waitFor({ timeout: 30_000 });
    await settle(page, 'A supply request — every piece named');

    // Ms. Okafor adds the classroom photo ON CAMERA so the request is born
    // with its hero: the share sheet now opens over the request's own view
    // (#2724), and without a hero that view is a black placeholder —
    // production fills one from stock imagery, which the e2e stack has no
    // provider for.
    await page.getByRole('button', { name: /add a photo/i }).click();
    const photosBtn = page.getByRole('button', { name: /^photos$/i });
    await photosBtn.waitFor({ timeout: 20_000 });
    const chooser = page.waitForEvent('filechooser', { timeout: 20_000 });
    await photosBtn.click();
    await (await chooser).setFiles(fixtureFile('hero-classroom.jpg'));
    await settle(page, 'Room 7, photographed');

    await saveBtn.click();

    await page.getByRole('button', { name: /copy link/i }).waitFor({ timeout: 20_000 });

    // OFF CAMERA, while the share sheet covers the screen: discover the request
    // and mint its share audience. The five needs are already on it — create
    // read them out of her sentence (#2731), so there is nothing to post here.
    const found = await pollFor(async () => {
      const mine = await requestClient(maya).listMyRequests({});
      return mine.requests.find((r) => r.title === REQUEST_TITLE)?.id;
    }, 15_000);
    if (!found) throw new Error(`UI-created request "${REQUEST_TITLE}" not found via ListMyRequests`);
    requestId = found;

    const share = await createTestClient(CommunityService, rpc(maya)).shareItem({
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
      ...rpc(maya), requestId,
      locationId: schoolLocationId,
    });

    await settle(page, 'Share sheet');
    await page.getByRole('button', { name: /copy link/i }).click();
    await settle(page, 'Link copied — to paste in the parents’ WhatsApp');

    await page.getByRole('button', { name: /share to communities/i }).click();
    await settle(page, 'Pick the class parents');
    await page.getByText(COMMUNITY_NAME, { exact: true }).first().click();
    await settle(page, 'Room 7 Families picked');
    await page.getByRole('button', { name: /^confirm$/i }).click();
    await settle(page, 'Shared with the class');

    // ---- ON CAMERA: her one sentence already became five claimable needs.
    //      Create read the supply list out of the request's own text (#2731) —
    //      no pasting, no re-typing what she already wrote. Open the request to
    //      see them, each its own thing a parent can bring. ----
    await openRequest(page);
    await settle(page, 'One sentence — five claimable needs');
  } finally {
    await scene.close();
  }
  await refreshNeedIds();

  // OFF CAMERA: Ms. Okafor "pastes the link in WhatsApp" — pre-invite Rob by
  // phone (a provisional member the phone-first flow promotes on verify).
  await createProvisionalUser({
    baseUrl, specSlug: SPEC_SLUG, accessToken: maya.accessToken,
    communityId: adhocCommunityId, name: 'Rob Feeney', phoneNumber: GUEST_PHONE,
  });

  // Server-side truth: live request with the classroom hero and the whole list.
  const detail = await requestClient(maya).getRequest({ requestId });
  expect(detail.request?.state, 'request is live').toBe(RequestState.ACTIVE);
  expect(detail.request?.mediaIds?.length ?? 0, 'classroom hero attached').toBeGreaterThan(0);
  expect(
    [...needIds.keys()].sort(),
    'the request carries the whole supply list',
  ).toEqual([NEED_ART, NEED_BINS, NEED_PAPER, NEED_BOOKS, NEED_WHITEBOARD].sort());
});

test('@walkthrough 02 parents pitch in', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- OFF CAMERA: parents give (and one lends) their actual items. Each is a
  //      real give/lend offer targeting Ms. Okafor — the escalated offers show
  //      on her request just like a loan offer does (gearOffers), so unlike a
  //      plain claim pill she can see exactly what's coming. ----
  await pledgeGear(theo, NEED_BOOKS, 'Box of picture books', 'gear-books.jpg', TransferType.GIVEAWAY, {
    valueUsd: 35, weightGrams: 3500, material: MaterialCategory.WOOD,
  });
  await pledgeGear(nadia, NEED_BINS, 'Storage bins', 'gear-bins.jpg', TransferType.GIVEAWAY, {
    valueUsd: 25, weightGrams: 2200, material: MaterialCategory.SOLID_PLASTIC,
  });
  await pledgeGear(dana, NEED_ART, 'Art supplies caddy', 'gear-art-supplies.jpg', TransferType.GIVEAWAY, {
    valueUsd: 40, weightGrams: 1600, material: MaterialCategory.SOLID_PLASTIC,
  });
  await pledgeGear(bea, NEED_WHITEBOARD, 'Dry-erase whiteboard', 'gear-whiteboard.jpg', TransferType.LOAN, {
    valueUsd: 60, weightGrams: 4000, material: MaterialCategory.MIXED_PLASTIC_METAL,
  });

  // ---- ON CAMERA: Ms. Okafor's view of the classroom filling up. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-02`, actor: asActor(maya) });
  const { page } = scene;
  try {
    await openRequest(page);
    await settle(page, 'The class comes through');

    // Hover the scrollable first — a wheel at the default (0,0) pointer
    // position reaches no Flutter scrollable and films a frozen frame (#2724).
    await page.mouse.move(195, 500);
    await page.mouse.wheel(0, 300);
    await page.waitForTimeout(800);
    await settle(page, 'Given and lent — real things, on the way');
    await page.getByRole('button', { name: /view helpers/i }).click();
    await page.getByRole('button', { name: NEED_BOOKS }).first().waitFor({ timeout: 20_000 });
    await page.waitForTimeout(1_200);
    await settle(page, 'Books, bins, art — and a whiteboard for the year');
  } finally {
    await scene.close();
  }

  // Server-side truth: four gear offers landed on the request, three giveaways
  // and one loan.
  const offers = await pollFor(async () => {
    const resp = await requestClient(maya).getRequest({ requestId });
    const gearOffers = resp.request?.gearOffers ?? [];
    return gearOffers.length >= 4 ? gearOffers : undefined;
  });
  expect(offers, 'four gear offers on the request').toBeTruthy();
  const giftCount = offers!.filter((o) => o.transferType === TransferType.GIVEAWAY).length;
  const loanCount = offers!.filter((o) => o.transferType === TransferType.LOAN).length;
  expect(giftCount, 'three giveaways').toBe(3);
  expect(loanCount, 'one loan (the whiteboard)').toBe(1);
});

test('@walkthrough 03 the last piece', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: Rob taps the link from the WhatsApp group — no app, no
  //      account — joins via phone OTP (epic #2492, WEB-3) and grabs the last
  //      open piece: a pack of construction paper. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-03` });
  const { page } = scene;
  try {
    await page.goto(`${baseUrl}/go/${shareCode}`);
    await page.getByRole('link', { name: /offer to help/i }).waitFor({ timeout: 20_000 });
    await settle(page, 'The request, straight from WhatsApp');
    await page.getByRole('link', { name: /offer to help/i }).click();

    await page.getByRole('button', { name: /send code/i }).waitFor({ timeout: 30_000 });
    await settle(page, 'Confirm your phone');
    await typeInto(page, GUEST_PHONE);
    await page.getByRole('button', { name: /send code/i }).click();

    await page.getByRole('button', { name: /^verify$/i }).waitFor({ timeout: 20_000 });
    const code = await emulatorVerificationCode(GUEST_PHONE);
    await typeInto(page, code);
    await page.getByRole('button', { name: /^verify$/i }).click();

    await page.getByRole('button', { name: /^finish$/i }).waitFor({ timeout: 20_000 });
    await typeInto(page, 'Rob Feeney');
    await settle(page, 'Just a name');
    await page.getByRole('button', { name: /^finish$/i }).click();

    await page.getByRole('button', { name: /open conversation/i }).waitFor({ timeout: 30_000 });
    await settle(page, "He's in — one piece left");

    await page.mouse.move(195, 500);
    await page.mouse.wheel(0, 300);
    await page.waitForTimeout(800);
    await page.getByRole('button', { name: /view helpers/i }).click();
    await page.getByRole('button', { name: NEED_PAPER }).first().waitFor({ timeout: 20_000 });
    await settle(page, 'Construction paper — he can grab that');
    await page.getByRole('button', { name: NEED_PAPER }).first().click();

    const confirmCta = page.getByRole('button', { name: /i'll bring this/i });
    await confirmCta.waitFor({ timeout: 20_000 });
    await settle(page, 'Count him in');
    await confirmCta.click();
    await settle(page, "Rob's on the list");
  } finally {
    await scene.close();
  }

  const planning = await requestClient(maya).listRequestNeedsAndContributions({ requestId });
  const paperClaim = planning.contributions.find((c) => c.title === NEED_PAPER);
  expect(paperClaim, 'the last piece was claimed').toBeTruthy();
});

test('@walkthrough 04 everything delivered', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: drop-off day. Ms. Okafor marks the drive fulfilled and
  //      confirms the crew — and the gifts actually LAND: the giveaways complete
  //      and the whiteboard loan goes active (#2703 fix). The modal pre-confirms
  //      every helper, so she just confirms. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-04`, actor: asActor(maya) });
  const { page } = scene;
  try {
    await openRequest(page);
    await settle(page, 'Drop-off day');

    await page.getByRole('button', { name: /edit request/i }).click();
    await page.getByRole('button', { name: /mark fulfilled/i }).click();

    await page.getByRole('button', { name: /^mark fulfilled$/i }).last().waitFor({ timeout: 20_000 });
    await settle(page, 'Everyone who pitched in — already credited');
    await page.getByRole('button', { name: /^mark fulfilled$/i }).last().click();
    await settle(page, 'Fulfilled — and the gifts are hers to keep');
  } finally {
    await scene.close();
  }

  // Server-side truth: fulfilled, the crew credited, and the confirmed offers
  // DELIVERED — giveaways completed, the whiteboard loan active (the fix).
  const detail = await requestClient(maya).getRequest({ requestId });
  expect(detail.request?.state, 'the drive is fulfilled').toBe(RequestState.FULFILLED);
  expect(
    detail.request?.confirmedHelperIds?.length ?? 0,
    'the whole crew was credited',
  ).toBeGreaterThanOrEqual(4);

  for (const giver of [theo, nadia, dana]) {
    const delivered = await pollFor(async () => {
      const resp = await transferClient(maya).getTransfer({ transferId: offerTransferIds.get(giver.name)! });
      return resp.transfer?.state === TransferState.COMPLETED ? true : undefined;
    });
    expect(delivered, `${giver.name}'s giveaway completed on fulfillment`).toBe(true);
  }
  const whiteboard = await pollFor(async () => {
    const resp = await transferClient(maya).getTransfer({ transferId: offerTransferIds.get(bea.name)! });
    return resp.transfer?.state === TransferState.ACTIVE ? true : undefined;
  });
  expect(whiteboard, 'the whiteboard loan went active on fulfillment').toBe(true);
});
