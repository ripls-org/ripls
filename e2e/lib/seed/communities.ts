// e2e/lib/seed/communities.ts — create a community and (optionally)
// mint a share link the test can dial via /go/{code}.
//
// All operations go through the public CommunityService RPCs; no
// dev-mode service is involved. The caller (`opts.creator`) must
// already be authenticated — pass the accessToken returned by
// seed/users.registerUser.

import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import { createTestClient } from '../connect.js';

export interface SeededCommunity {
  communityId: string;
  name: string;
}

export interface CreateCommunityOptions {
  baseUrl: string;
  specSlug: string;
  accessToken: string;
  /** Default: 'E2E Test Community {short uuid}'. */
  name?: string;
  /** Default: 'Auto-created by the e2e harness.'. */
  description?: string;
}

export async function createCommunity(
  opts: CreateCommunityOptions,
): Promise<SeededCommunity> {
  const client = createTestClient(CommunityService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const name = opts.name ?? `E2E ${shortId()}`;
  const description =
    opts.description ?? 'Auto-created by the e2e harness.';
  const resp = await client.createCommunity({
    name,
    description,
    mediaIds: [],
  });
  if (!resp.id) {
    throw new Error('CreateCommunity returned empty id');
  }
  return { communityId: resp.id, name };
}

export interface SeededShareLink {
  shortCode: string;
  shareUrl: string;
  // The community the link is actually scoped to. For item-target links this
  // is the item's ad-hoc origin community, which the server resolves and may
  // differ from the caller-supplied communityId (#2767); a joiner accepting
  // the link joins this community, so RSVP/offer state is recorded here.
  communityId: string;
}

export interface MintExperienceShareLinkOptions {
  baseUrl: string;
  specSlug: string;
  accessToken: string;
  communityId: string;
  experienceId: string;
}

/**
 * Mint a share link targeting a specific experience within the
 * given community. Returns the short_code (for the /go/{code} URL)
 * and the full share URL.
 */
export async function mintExperienceShareLink(
  opts: MintExperienceShareLinkOptions,
): Promise<SeededShareLink> {
  const client = createTestClient(CommunityService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const resp = await client.getOrCreateShareLink({
    communityId: opts.communityId,
    target: { case: 'experienceId', value: opts.experienceId },
  });
  if (!resp.shortCode || !resp.shareUrl) {
    throw new Error('GetOrCreateShareLink returned empty short_code / share_url');
  }
  return { shortCode: resp.shortCode, shareUrl: resp.shareUrl, communityId: resp.communityId };
}

export interface MintGearShareLinkOptions {
  baseUrl: string;
  specSlug: string;
  accessToken: string;
  communityId: string;
  gearId: string;
}

/** Mint a share link targeting a specific gear within the community. */
export async function mintGearShareLink(
  opts: MintGearShareLinkOptions,
): Promise<SeededShareLink> {
  const client = createTestClient(CommunityService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const resp = await client.getOrCreateShareLink({
    communityId: opts.communityId,
    target: { case: 'gearId', value: opts.gearId },
  });
  if (!resp.shortCode || !resp.shareUrl) {
    throw new Error('GetOrCreateShareLink returned empty short_code / share_url');
  }
  return { shortCode: resp.shortCode, shareUrl: resp.shareUrl, communityId: resp.communityId };
}

export interface MintRequestShareLinkOptions {
  baseUrl: string;
  specSlug: string;
  accessToken: string;
  communityId: string;
  requestId: string;
}

/** Mint a share link targeting a specific request within the community. */
export async function mintRequestShareLink(
  opts: MintRequestShareLinkOptions,
): Promise<SeededShareLink> {
  const client = createTestClient(CommunityService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const resp = await client.getOrCreateShareLink({
    communityId: opts.communityId,
    target: { case: 'requestId', value: opts.requestId },
  });
  if (!resp.shortCode || !resp.shareUrl) {
    throw new Error('GetOrCreateShareLink returned empty short_code / share_url');
  }
  return { shortCode: resp.shortCode, shareUrl: resp.shareUrl, communityId: resp.communityId };
}

/**
 * Mint a community-invite share link (no item target). Used for
 * the future invite-via-code Phase-1 follow-up scenarios.
 */
export async function mintCommunityInviteLink(
  opts: Omit<MintExperienceShareLinkOptions, 'experienceId'>,
): Promise<SeededShareLink> {
  const client = createTestClient(CommunityService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const resp = await client.getOrCreateShareLink({
    communityId: opts.communityId,
    target: { case: 'communityInvite', value: opts.communityId },
  });
  return { shortCode: resp.shortCode, shareUrl: resp.shareUrl, communityId: resp.communityId };
}

function shortId(): string {
  return Math.random().toString(36).slice(2, 8);
}
