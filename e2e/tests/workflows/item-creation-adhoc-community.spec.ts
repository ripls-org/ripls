// e2e/tests/workflows/item-creation-adhoc-community.spec.ts
//
// Per-item ad-hoc communities at creation (epic #2492, CREATE-1 + WEB-3/WEB-4):
// every item type is born with its own nameless ad-hoc community (host the sole
// member), and the host can invite other users into it — the same guarantee the
// event flow has (see event-invite-home-multi-client.spec.ts). This asserts it
// for GEAR and REQUEST, whose per-item community is now provisioned at SaveGear /
// SubmitRequest.
//
// RPC-only (no browser): the subject is the server contract, not any UI.

import { test, expect } from '../../lib/fixtures.js';
import { GearService } from '../../gen/ripls/api/gear_service_pb.js';
import { RequestService } from '../../gen/ripls/api/request_service_pb.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser } from '../../lib/seed/users.js';

const SPEC_SLUG = 'item-creation-adhoc-community';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('gear creation provisions a nameless ad-hoc community the host can invite into', async () => {
  const baseUrl = requireBaseUrl();
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Gear Host' });
  const alice = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Gear Alice' });

  // ── Create the gear; assert it's born with its per-item community. ──
  const gearClient = createTestClient(GearService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  const saveResp = await gearClient.saveGear({
    name: 'Cordless Drill',
    description: 'A test drill',
  });
  expect(saveResp.id, 'SaveGear returns a gear id').toBeTruthy();
  expect(
    saveResp.itemCommunityId,
    'SaveGear provisions the gear\'s per-item ad-hoc community',
  ).toBeTruthy();
  const communityId = saveResp.itemCommunityId;

  const hostComm = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });

  // The provisioned community is nameless (ad-hoc) and host-only at first.
  const before = await hostComm.getCommunity({ id: communityId });
  expect(before.name, 'the per-item community is nameless (ad-hoc)').toBe('');
  expect(before.numMembers, 'host is the sole member at creation').toBe(1);

  // ── Invite Alice; ShareItem reuses the SAME per-item community. ──
  const shareResp = await hostComm.shareItem({
    item: { case: 'gearId', value: saveResp.id },
    invitees: [{ identity: { case: 'memberUserId', value: alice.userId } }],
  });
  expect(
    shareResp.adhocCommunityId,
    'ShareItem reuses the gear\'s per-item community (no duplicate)',
  ).toBe(communityId);

  // Alice is now a member, and the community surfaces in her own list.
  const after = await hostComm.getCommunity({ id: communityId });
  expect(after.numMembers, 'invitee joined the ad-hoc community').toBe(2);

  const aliceComm = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: alice.accessToken,
  });
  const aliceCommunities = await aliceComm.listCommunities({});
  expect(
    aliceCommunities.communities.map((c) => c.id),
    'invitee sees the gear\'s ad-hoc community in her communities',
  ).toContain(communityId);
});

test('request creation provisions a nameless ad-hoc community the host can invite into', async () => {
  const baseUrl = requireBaseUrl();
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Request Host' });
  const alice = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Request Alice' });

  const requestClient = createTestClient(RequestService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  const submitResp = await requestClient.submitRequest({
    // Always created in its own per-item community (#2492); SubmitRequestRequest
    // no longer carries a community_id, so the empty one here was inert.
    title: 'An extension ladder',
    description: 'A test request',
  });
  expect(submitResp.requestId, 'SubmitRequest returns a request id').toBeTruthy();
  expect(
    submitResp.itemCommunityId,
    'SubmitRequest provisions the request\'s per-item ad-hoc community',
  ).toBeTruthy();
  const communityId = submitResp.itemCommunityId;

  const hostComm = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });

  const before = await hostComm.getCommunity({ id: communityId });
  expect(before.name, 'the per-item community is nameless (ad-hoc)').toBe('');
  expect(before.numMembers, 'host is the sole member at creation').toBe(1);

  const shareResp = await hostComm.shareItem({
    item: { case: 'requestId', value: submitResp.requestId },
    invitees: [{ identity: { case: 'memberUserId', value: alice.userId } }],
  });
  expect(
    shareResp.adhocCommunityId,
    'ShareItem reuses the request\'s per-item community (no duplicate)',
  ).toBe(communityId);

  const after = await hostComm.getCommunity({ id: communityId });
  expect(after.numMembers, 'invitee joined the ad-hoc community').toBe(2);

  const aliceComm = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: alice.accessToken,
  });
  const aliceCommunities = await aliceComm.listCommunities({});
  expect(
    aliceCommunities.communities.map((c) => c.id),
    'invitee sees the request\'s ad-hoc community in her communities',
  ).toContain(communityId);
});
