// e2e/tests/walkthroughs/gear.spec.ts
//
// @walkthrough — the ITEM-SHARING WALKTHROUGH (#2687): seven paced scenes of
// the giveaway journey, fabricated cast and content from the kitchen pack
// (e2e/fixtures/walkthroughs/kitchen/ — screenplay in content.json; food
// images are Unsplash stock, attribution in ../manifest.json):
//
//   01 Maya photographs the spaghetti — the photo becomes the listing
//      (e2e provider keys content on the fixture filename), Give, save,
//      share to two communities from one sheet
//   02 The bread and the broccoli go the same way, at speed
//   03 Priya claims the sourdough (single requester) and says hi in the
//      item's own conversation
//   04 Two want the pasta: Marcus's and Dana's claims + messages land live
//      in Maya's open chat ("sauce simmering right now, zero noodles")
//   05 Maya picks Dana from the who-wants-it panel — transparent to everyone
//   06 Everything claimed: Maya marks the items given, one by one
//   07 Dana's side: the spaghetti has a new home
//
// One scene = one test = one actor on one recording context = one clip
// (lib/walkthrough.ts); everything else happens off camera via RPC seed
// helpers, including MID-SCENE actions the camera watches land live (claims,
// chat messages). Scenes run serially in one worker, sharing the hermetic
// server + DB, so the world accumulates scene over scene. Render:
//
//   e2e/scripts/run_walkthrough.sh gear
//
// This is a SHOWCASE, not a correctness gate — off-camera RPC mid-scene is by
// design here (see e2e/README.md's UI-only rule, which binds workflow specs).
// The correctness twins live in tests/workflows/ (phone-gear-giveaway-full-loop,
// gear-authoring-share).

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { test, expect } from '../../lib/fixtures.js';
import { GearService } from '../../gen/ripls/api/gear_service_pb.js';
import { TransferService } from '../../gen/ripls/api/transfer_service_pb.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import { UserService } from '../../gen/ripls/api/user_service_pb.js';
import { Availability } from '../../gen/ripls/api/gear_pb.js';
import { TransferState } from '../../gen/ripls/api/transfer_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser, registerUserViaInvite, type SeededUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createLocation } from '../../lib/seed/locations.js';
import { setUserAvatar } from '../../lib/seed/attendees.js';
import { sendGearChatMessage } from '../../lib/seed/chat.js';
import { expressGearInterest, selectGearRecipient, completeGearTransfer } from '../../lib/seed/transfers.js';
import { openScene, requireBaseUrl, settle, typeInto } from '../../lib/walkthrough.js';

const SPEC_SLUG = 'gear';

const FIXTURES = resolve(__dirname, '..', '..', 'fixtures', 'walkthroughs');
const fixtureFile = (f: string) => resolve(FIXTURES, f);

// The fabricated screenplay (see the pack's manifest.json — no real people).
interface KitchenContent {
  offerer: { name: string; avatarFile: string };
  cast: { name: string; avatarFile: string }[];
  communities: { name: string; members: string[] }[];
  items: {
    imageFile: string;
    title: string;
    communities: string[];
    recipient: string;
  }[];
  spaghettiChat: { sender: string; text: string }[];
  breadChat: { sender: string; text: string }[];
  broccoliChat: { sender: string; text: string }[];
}
const CONTENT = JSON.parse(
  readFileSync(resolve(FIXTURES, 'kitchen', 'content.json'), 'utf-8'),
) as KitchenContent;

// ── World state accumulated across scenes (same worker, same server+DB) ──
let baseUrl: string;
let maya: SeededUser;
const cast: Record<string, SeededUser> = {};
const communityIds: Record<string, string> = {};
// Gear ids by fixture key, discovered after the on-camera creates.
let spaghettiId: string;
let breadId: string;
let broccoliId: string;
// Transfer ids by gear id, discovered after claims land.
const transferByGear: Record<string, string> = {};

const gearClient = (user: SeededUser) =>
  createTestClient(GearService, { baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken });
const transferClient = (user: SeededUser) =>
  createTestClient(TransferService, { baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken });

const asActor = (user: SeededUser) => ({
  accessToken: user.accessToken,
  refreshToken: user.refreshToken,
  user: { id: user.userId, name: user.name },
  serverUrl: baseUrl,
});

const rpc = (user: SeededUser) =>
  ({ baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken }) as const;

/** Open a gear item's content view directly (/gear/:id). One boot per scene
 * keeps the clip clean — the head is trimmed at export. */
async function openGear(page: import('@playwright/test').Page, gearId: string): Promise<void> {
  await page.goto(`${baseUrl}/gear/${gearId}`);
  await page.getByRole('button', { name: /view who wants it/i }).waitFor({ timeout: 30_000 });
  await page.waitForTimeout(1_500); // let the pane settle
}

/**
 * The on-camera create beat: Create → Image tab → gallery pick → streamed
 * listing (the e2e provider keys food content on the fixture filename) →
 * Give → Save → share sheet → invite the given communities → confirm.
 * Assumes the page shows the home scaffold (Create button reachable).
 */
async function createItemFromPhoto(
  page: import('@playwright/test').Page,
  imageFile: string,
  shareTo: string[],
  opts: { pace: 'featured' | 'brisk' },
): Promise<void> {
  const featured = opts.pace === 'featured';
  await page.getByRole('button', { name: /^create$/i }).click();
  await page.getByRole('button', { name: /^image$/i }).click();
  if (featured) await settle(page, 'Create — point the camera at it');

  const chooser = page.waitForEvent('filechooser', { timeout: 20_000 });
  await page.getByRole('button', { name: /open gallery/i }).click();
  await (await chooser).setFiles(fixtureFile(imageFile));

  // The stream classifies Item and fills title/description/details; the
  // generic Save button enables once content is saveable (#2687).
  const saveBtn = page.getByRole('button', { name: /^save$/i });
  await saveBtn.waitFor({ timeout: 30_000 });
  await settle(page, 'Photo became a listing');
  if (featured) await settle(page, 'Name, description, value — written for her');

  // This is a giveaway, not a loan.
  await page.getByRole('button', { name: /^give$/i }).click();
  await settle(page, 'Give, not lend');

  await saveBtn.click();

  // The share sheet auto-opens (two-phase create → share, #2492).
  await page.getByRole('button', { name: /share to communities/i }).waitFor({ timeout: 20_000 });
  if (featured) await settle(page, 'Share sheet');

  // Additive multi-select community picker; Confirm dismisses the sheet.
  await page.getByRole('button', { name: /share to communities/i }).click();
  await settle(page, 'Pick the circles');
  for (const name of shareTo) {
    await page.getByText(name, { exact: true }).first().click();
  }
  await settle(page, 'Circles picked');
  await page.getByRole('button', { name: /^confirm$/i }).click();
  await settle(page, 'Shared');
}

/** Find the gear the UI just created by its deterministic provider title,
 * then pin FOR_GIVEAWAY availability on each named community so the claim
 * RPCs are deterministic (the toggle sets intent; this makes it law). */
async function discoverAndPin(title: string, shareTo: string[]): Promise<string> {
  const mine = await gearClient(maya).listUserGear({});
  const item = (mine.items ?? []).find((g) => g.name === title);
  if (!item?.id) {
    throw new Error(`UI-created item "${title}" not found via ListUserGear`);
  }
  const community = createTestClient(CommunityService, rpc(maya));
  // Availability is item-wide (#2492/#2687), so the give/lend choice is one
  // call rather than an argument on each community share — and it comes first,
  // because ShareItem inherits the mode and the GEAR_SHARED community event
  // takes its "New giveaway" copy from it.
  await community.setGearAvailability({
    gearId: item.id,
    availability: Availability.FOR_GIVEAWAY,
  });
  await community.shareItem({
    item: { case: 'gearId', value: item.id },
    shareToCommunityIds: shareTo.map((name) => communityIds[name]),
  });
  return item.id;
}

/** The owner's sole transfer for a gear item. Each requester gets their own
 * transfer record (they share the gear conversation), so this helper is only
 * safe for items with a single claimant — use transferState(id) otherwise. */
async function transferForGear(gearId: string): Promise<{ id: string; state: TransferState; recipientId: string }> {
  const resp = await transferClient(maya).listMyTransfers({});
  const matches = (resp.transfers ?? []).filter((tr) => tr.gearId === gearId);
  if (matches.length !== 1 || !matches[0].id) {
    throw new Error(`expected exactly 1 transfer for gear ${gearId}, found ${matches.length}`);
  }
  const t = matches[0];
  return { id: t.id, state: t.state, recipientId: t.recipient?.id ?? '' };
}

/** A specific transfer's state + recipient, by id. */
async function transferState(transferId: string): Promise<{ state: TransferState; recipientId: string }> {
  const resp = await transferClient(maya).getTransfer({ transferId });
  if (!resp.transfer) throw new Error(`transfer ${transferId} not found`);
  return {
    state: resp.transfer.state,
    recipientId: resp.transfer.recipient?.id ?? '',
  };
}

test.describe.configure({ mode: 'serial' });

test('@walkthrough 01 photo to listing', async ({ browser }) => {
  test.setTimeout(300_000);
  baseUrl = requireBaseUrl();

  // ---- OFF CAMERA: the cast and the circles. Maya belongs to three
  //      communities; the claimants are members via invite. Avatars come
  //      from the committed stock set (Unsplash, see ../manifest.json). ----
  maya = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: CONTENT.offerer.name });
  await setUserAvatar({
    baseUrl, specSlug: SPEC_SLUG, accessToken: maya.accessToken,
    userId: maya.userId, avatarPath: fixtureFile(CONTENT.offerer.avatarFile),
  });

  // Maya's home — the photo-detected item's location falls back to the
  // owner's primary residence, so listings carry a real neighborhood.
  const homeId = await createLocation({
    baseUrl, specSlug: SPEC_SLUG, accessToken: maya.accessToken,
    name: '1608 Alta Vista Ave', addressLines: ['1608 Alta Vista Ave'],
    locality: 'Austin', administrativeArea: 'Texas', regionCode: 'US',
    postalCode: '78704', latitudeDeg: 30.246, longitudeDeg: -97.742,
  });
  await createTestClient(UserService, rpc(maya)).saveUser({
    userId: maya.userId, primaryResidenceLocationId: homeId,
  });

  for (const c of CONTENT.communities) {
    const created = await createCommunity({
      baseUrl, specSlug: SPEC_SLUG, accessToken: maya.accessToken, name: c.name,
    });
    communityIds[c.name] = created.communityId;
  }
  for (const member of CONTENT.cast) {
    // Each cast member belongs to exactly one of Maya's circles (see
    // content.json); claim eligibility only needs one shared community.
    const memberships = CONTENT.communities.filter((c) => c.members.includes(member.name));
    if (memberships.length === 0) continue;
    const user = await registerUserViaInvite({
      baseUrl, specSlug: SPEC_SLUG, inviterAccessToken: maya.accessToken,
      communityId: communityIds[memberships[0].name], name: member.name,
    });
    cast[member.name] = user;
    await setUserAvatar({
      baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken,
      userId: user.userId, avatarPath: fixtureFile(member.avatarFile),
    });
  }

  // ---- ON CAMERA: the flagship beat — a photo becomes a listing. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-01`, actor: asActor(maya) });
  const { page } = scene;
  try {
    await page.goto(`${baseUrl}/`);
    // Wait for a rendered Home before the first settle — the first settle
    // sets the export's head-trim point. The semantics tree lands before the
    // canvas paints on skwasm, so give pixels a beat too (#2724).
    await page.getByRole('button', { name: /^create$/i }).waitFor({ timeout: 30_000 });
    await page.waitForTimeout(1_200);
    await settle(page, 'Home — two days before the trip');

    const spaghetti = CONTENT.items[0];
    await createItemFromPhoto(page, spaghetti.imageFile, spaghetti.communities, { pace: 'featured' });
  } finally {
    await scene.close();
  }

  // OFF CAMERA: discover the created item + pin giveaway availability.
  spaghettiId = await discoverAndPin(CONTENT.items[0].title, CONTENT.items[0].communities);

  // Server-side truth: the streamed content satisfied the Save gate with a
  // real description (the #2687 fix) — not a hand-typed rescue.
  const g = await gearClient(maya).getGear({ id: spaghettiId, communityId: '' });
  expect(g.description, 'photo-generated description landed').not.toBe('');
});

test('@walkthrough 02 clear the fridge', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: the other two items, at speed — repetition is cheap. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-02`, actor: asActor(maya) });
  const { page } = scene;
  try {
    await page.goto(`${baseUrl}/`);
    await page.getByRole('button', { name: /^create$/i }).waitFor({ timeout: 30_000 });
    await settle(page, 'Back at the shelf');

    const bread = CONTENT.items[1];
    await createItemFromPhoto(page, bread.imageFile, bread.communities, { pace: 'brisk' });

    // The post-save flow lands on the new item's own screen (the share sheet
    // opens over it, #2724); pop back to Home IN-APP for the next create —
    // a goto here films a full app reboot (white flash + boot splash).
    await page.getByRole('button', { name: /^back$/i }).first().click();
    await page.getByRole('button', { name: /^create$/i }).waitFor({ timeout: 30_000 });
    await settle(page, 'One down');

    const broccoli = CONTENT.items[2];
    await createItemFromPhoto(page, broccoli.imageFile, broccoli.communities, { pace: 'brisk' });
    await settle(page, 'The whole shelf, shared');
  } finally {
    await scene.close();
  }

  breadId = await discoverAndPin(CONTENT.items[1].title, CONTENT.items[1].communities);
  broccoliId = await discoverAndPin(CONTENT.items[2].title, CONTENT.items[2].communities);
});

test('@walkthrough 03 first claim', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: Priya claims the sourdough — a single requester. ----
  const priya = cast['Priya'];
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-03`, actor: asActor(priya) });
  const { page } = scene;
  try {
    await openGear(page, breadId);
    await settle(page, 'The sourdough, from her street');

    await page.getByRole('button', { name: /express interest/i }).click();
    await settle(page, 'One tap to claim');

    // Say hi where the item lives — its own conversation.
    await page.getByRole('button', { name: /open conversation/i }).click();
    await settle(page, 'The item has its own chat');
    await typeInto(page, CONTENT.breadChat[0].text);
    await page.getByRole('button', { name: /send message/i }).click();
    await settle(page, 'Spoken for');
  } finally {
    await scene.close();
  }

  // Server-side truth: the on-camera claim landed as an open group transfer.
  const t = await transferForGear(breadId);
  transferByGear[breadId] = t.id;
  expect(t.state, 'Priya joined the bread giveaway').toBe(TransferState.INTEREST_EXPRESSED);
});

test('@walkthrough 04 two want the pasta', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: Maya watches the spaghetti conversation as two claims
  //      land live. ----
  const marcus = cast['Marcus'];
  const dana = cast['Dana'];
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-04`, actor: asActor(maya) });
  const { page } = scene;
  try {
    await openGear(page, spaghettiId);
    await settle(page, 'The spaghetti');
    await page.getByRole('button', { name: /open conversation/i }).click();
    await settle(page, 'Item chat');

    // OFF CAMERA, mid-scene: Marcus wants it…
    const marcusClaim = await expressGearInterest({ ...rpc(marcus), gearId: spaghettiId });
    await sendGearChatMessage({
      ...rpc(marcus), gearId: spaghettiId, text: CONTENT.spaghettiChat[0].text,
      communityId: communityIds['Alameda Neighbors'],
    });
    await settle(page, 'Marcus could use it');

    // …then Dana, with the line that decides it.
    const danaClaim = await expressGearInterest({ ...rpc(dana), gearId: spaghettiId });
    await sendGearChatMessage({
      ...rpc(dana), gearId: spaghettiId, text: CONTENT.spaghettiChat[1].text,
      communityId: communityIds['Fourth Grade Moms'],
    });
    await settle(page, 'Dana has sauce on the stove');

    // Each requester gets their own transfer record; they share the gear
    // conversation (see server/services/transfer/interest.go). Keep Dana's —
    // scene 05 selects her, and completion auto-cancels the others.
    expect(marcusClaim.transferId, "Marcus's claim landed").not.toBe('');
    expect(danaClaim.transferId, "Dana's claim landed").not.toBe('');
    transferByGear[spaghettiId] = danaClaim.transferId;

    // Maya answers in front of everyone.
    await typeInto(page, CONTENT.spaghettiChat[2].text);
    await page.getByRole('button', { name: /send message/i }).click();
    await settle(page, 'Destiny, decided in the open');
  } finally {
    await scene.close();
  }

  const t = await transferState(transferByGear[spaghettiId]);
  expect(t.state, 'spaghetti giveaway open for selection').toBe(TransferState.INTEREST_EXPRESSED);
});

test('@walkthrough 05 it goes to dana', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: Maya picks Dana from the who-wants-it panel. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-05`, actor: asActor(maya) });
  const { page } = scene;
  try {
    await openGear(page, spaghettiId);
    await settle(page, 'Two hands raised');

    await page.getByRole('button', { name: /view who wants it/i }).click();
    await settle(page, 'Who wants it');

    await page.getByRole('button', { name: /select dana as the recipient/i }).click();
    await settle(page, 'It goes to Dana');
    await settle(page, 'Marcus can see why');
  } finally {
    await scene.close();
  }

  // Server-side truth: Dana's transfer moved to RECIPIENT_SELECTED.
  const t = await transferState(transferByGear[spaghettiId]);
  expect(t.state, 'spaghetti recipient selected').toBe(TransferState.RECIPIENT_SELECTED);
  expect(t.recipientId, 'and it is Dana').toBe(cast['Dana'].userId);
});

test('@walkthrough 06 everything claimed', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- OFF CAMERA: Jules claims the broccoli from the vegan circle, and
  //      Maya settles the uncontested recipients by RPC so the on-camera
  //      beat is the handoffs, not more menus. ----
  const jules = cast['Jules'];
  const priya = cast['Priya'];
  await expressGearInterest({ ...rpc(jules), gearId: broccoliId });
  await sendGearChatMessage({
    ...rpc(jules), gearId: broccoliId, text: CONTENT.broccoliChat[0].text,
    communityId: communityIds['Vegans of Travis Heights'],
  });
  const broccoliTransfer = await transferForGear(broccoliId);
  transferByGear[broccoliId] = broccoliTransfer.id;
  await selectGearRecipient({
    ...rpc(maya), transferId: transferByGear[breadId], recipientId: priya.userId,
  });
  await selectGearRecipient({
    ...rpc(maya), transferId: transferByGear[broccoliId], recipientId: jules.userId,
  });

  // ---- ON CAMERA: Maya reviews and hands everything off. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-06`, actor: asActor(maya) });
  const { page } = scene;
  try {
    await page.goto(`${baseUrl}/`);
    await page.getByRole('button', { name: /^create$/i }).waitFor({ timeout: 30_000 });
    await settle(page, 'Home — everything claimed');

    // Spaghetti → Dana — opened from My gear (avatar → "My gear" filters
    // the Library tab to Maya's own items; the Home "Yours" list this used
    // to open from moved here in the #2634 dock redesign), never a
    // mid-scene goto: a reload films a full app reboot (#2724).
    await page.getByRole('button', { name: /open account menu/i }).click();
    await page.getByRole('button', { name: /^my gear$/i }).click();
    await page
      .getByRole('button', { name: CONTENT.items[0].title })
      .first()
      .waitFor({ timeout: 30_000 });
    await settle(page, 'My gear');
    await page.getByRole('button', { name: CONTENT.items[0].title }).first().click();
    await page.getByRole('button', { name: /view who wants it/i }).waitFor({ timeout: 30_000 });
    await page.waitForTimeout(1_500);
    await page.getByRole('button', { name: /view who wants it/i }).click();
    await settle(page, 'Dana, confirmed');
    await page.getByRole('button', { name: /mark this giveaway as given/i }).click();
    await settle(page, 'Spaghetti — handed off');

    // Bread → Priya — close the panel, back to My gear (the person filter
    // is Library-tab state, so it's still active on return), in from the
    // same filtered list.
    // Unanchored /close/: IconAction sets BOTH a tooltip and the icon's
    // semanticLabel, which merge into one accessible name — "^close$" misses it.
    await page.getByRole('button', { name: /close/i }).first().click();
    await page.getByRole('button', { name: /^back$/i }).first().click();
    await page
      .getByRole('button', { name: CONTENT.items[1].title })
      .first()
      .waitFor({ timeout: 30_000 });
    await page.getByRole('button', { name: CONTENT.items[1].title }).first().click();
    await page.getByRole('button', { name: /view who wants it/i }).waitFor({ timeout: 30_000 });
    await page.waitForTimeout(1_500);
    await page.getByRole('button', { name: /view who wants it/i }).click();
    await settle(page, 'Priya, confirmed');
    await page.getByRole('button', { name: /mark this giveaway as given/i }).click();
    await settle(page, 'Sourdough — handed off');
  } finally {
    await scene.close();
  }

  // OFF CAMERA: the broccoli handoff completes too.
  await completeGearTransfer({ ...rpc(maya), transferId: transferByGear[broccoliId] });

  // Server-side truth: all three recipients' giveaways completed. A silent
  // failure here would render the closing scene wrong instead of failing
  // the reel. (Marcus's parallel spaghetti transfer is auto-cancelled by
  // Dana's completion — by design, not asserted here.)
  for (const gearId of [spaghettiId, breadId, broccoliId]) {
    const t = await transferState(transferByGear[gearId]);
    expect(t.state, `giveaway completed for ${gearId}`).toBe(TransferState.COMPLETED);
  }
});
