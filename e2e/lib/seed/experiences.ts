// e2e/lib/seed/experiences.ts — seed an experience (event), optionally
// with a hero video, and share it with a community.
//
// Uses the public ExperienceService.SaveExperience, CommunityService.ShareItem
// (the unified add-path for every item type), and MediaService.AddMedia. No
// dev-mode plumbing.

import { readFileSync } from 'node:fs';
import { ExperienceService } from '../../gen/ripls/api/experience_service_pb.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import { MediaService } from '../../gen/ripls/api/media_service_pb.js';
import { createTestClient } from '../connect.js';

export interface SeededExperience {
  experienceId: string;
  name: string;
  description: string;
  /** Media IDs in display order. First is the hero. Empty when no media seeded. */
  mediaIds: string[];
}

export interface CreateExperienceOptions {
  baseUrl: string;
  specSlug: string;
  accessToken: string;
  communityId: string;
  /** Default: 'E2E Test Event {short id}'. */
  name?: string;
  /** Default: 'Auto-created by the e2e harness.'. */
  description?: string;
  /**
   * Optional scheduled start (Unix seconds). When set, the event carries a
   * specific time so it surfaces on the home Up-next agenda and calendar
   * (undated events are excluded there). Omit for an undated event.
   */
  timeUnixSec?: number;
  /**
   * Optional location id (from seed/locations.ts createLocation). Gives the
   * event coordinates — needed when the viewer's home-view weather/suggestions
   * must resolve a place (server resolveViewerPlace reads event locations).
   * Default: '' (no location).
   */
  locationId?: string;
  /**
   * Optional hero media to upload before SaveExperience. Pass a
   * filesystem path; the bytes are read and posted via
   * MediaService.AddMedia. The returned media_id rides on
   * SaveExperience as the first media_id (the hero slot).
   */
  heroMedia?: {
    path: string;
    contentType: string;
    filename: string;
  };
}

export async function createExperience(
  opts: CreateExperienceOptions,
): Promise<SeededExperience> {
  const mediaIds: string[] = [];
  if (opts.heroMedia) {
    const mediaId = await uploadMedia({
      baseUrl: opts.baseUrl,
      specSlug: opts.specSlug,
      accessToken: opts.accessToken,
      ...opts.heroMedia,
    });
    mediaIds.push(mediaId);
  }

  const expClient = createTestClient(ExperienceService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const name = opts.name ?? `E2E Event ${shortId()}`;
  const description = opts.description ?? 'Auto-created by the e2e harness.';
  const saveResp = await expClient.saveExperience({
    name,
    description,
    mediaIds,
    locationId: opts.locationId ?? '',
    maxParticipants: 0,
    ...(opts.timeUnixSec != null
      ? {
          time: {
            timeType: {
              case: 'specific' as const,
              value: { unixTimestampSec: BigInt(opts.timeUnixSec) },
            },
          },
        }
      : {}),
  });
  if (!saveResp.experience?.id) {
    throw new Error('SaveExperience returned no experience id');
  }
  const experienceId = saveResp.experience.id;

  const communityClient = createTestClient(CommunityService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  await communityClient.shareItem({
    item: { case: 'experienceId', value: experienceId },
    shareToCommunityIds: [opts.communityId],
  });

  return { experienceId, name, description, mediaIds };
}

export interface UploadMediaOptions {
  baseUrl: string;
  specSlug: string;
  accessToken: string;
  path: string;
  contentType: string;
  filename: string;
}

export async function uploadMedia(opts: UploadMediaOptions): Promise<string> {
  const bytes = readFileSync(opts.path);
  const client = createTestClient(MediaService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const resp = await client.addMedia({
    encodedBytes: new Uint8Array(bytes),
    contentType: opts.contentType,
    filename: opts.filename,
    description: '',
  });
  if (!resp.id) {
    throw new Error(`AddMedia returned empty id for ${opts.filename}`);
  }
  return resp.id;
}

function shortId(): string {
  return Math.random().toString(36).slice(2, 8);
}
