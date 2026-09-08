// e2e/lib/seed/provisional.ts — seed a phone-keyed provisional user.
//
// The phone-first pivot e2e seeds a provisional (not-yet-registered) member
// keyed to the guest's phone number, so that when the guest verifies that phone
// through the web UI, promote-on-verify (ID-2) claims the placeholder and joins
// the real account. Goes through the public CommunityService.CreateProvisionalUser
// RPC; the caller must be an authenticated member of the community.

import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import { createTestClient } from '../connect.js';

export interface SeededProvisionalUser {
  provisionalUserId: string;
  name: string;
}

export interface CreateProvisionalUserOptions {
  baseUrl: string;
  specSlug: string;
  accessToken: string;
  communityId: string;
  name: string;
  /** E.164 phone number, e.g. "+15551234567". */
  phoneNumber: string;
}

export async function createProvisionalUser(
  opts: CreateProvisionalUserOptions,
): Promise<SeededProvisionalUser> {
  const client = createTestClient(CommunityService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const resp = await client.createProvisionalUser({
    communityId: opts.communityId,
    name: opts.name,
    contact: { case: 'phoneNumber', value: opts.phoneNumber },
  });
  if (!resp.provisionalUser?.id) {
    throw new Error('CreateProvisionalUser returned empty id');
  }
  return { provisionalUserId: resp.provisionalUser.id, name: opts.name };
}
