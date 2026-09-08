// e2e/lib/seed/attendees.ts — seed a registered user who is already a member
// of a community, has a profile avatar, and has RSVP'd to an experience.
//
// Used by the walkthrough spec to populate the event with a believable
// "who's in" roster (avatars on the SSR landing + the in-app detail). Each
// attendee:
//   1. registers directly into the community (registerUserViaInvite — joins
//      at registration, avoiding the late-join race),
//   2. uploads a portrait via MediaService.AddMedia and sets it as their
//      avatar via UserService.SaveUser (media_id syncs to media_ids[0]),
//   3. RSVPs to the experience via ExperienceService.RSVPToExperience.

import { ExperienceService, RSVPIntention } from '../../gen/ripls/api/experience_service_pb.js';
import { UserService } from '../../gen/ripls/api/user_service_pb.js';
import { createTestClient } from '../connect.js';
import { registerUserViaInvite, type SeededUser } from './users.js';
import { uploadMedia } from './experiences.js';

export type RSVP = 'yes' | 'maybe' | 'no';

export interface SeedAttendeeOptions {
  baseUrl: string;
  specSlug: string;
  /** Bearer token of an existing community member who mints the invite. */
  inviterAccessToken: string;
  communityId: string;
  experienceId: string;
  name: string;
  /** Filesystem path to a portrait JPEG used as the avatar. */
  avatarPath: string;
  /** RSVP intention. Default 'yes'. */
  rsvp?: RSVP;
}

/** Register a user into the community with an avatar and an RSVP. */
export async function seedAttendee(opts: SeedAttendeeOptions): Promise<SeededUser> {
  const user = await registerUserViaInvite({
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    name: opts.name,
    inviterAccessToken: opts.inviterAccessToken,
    communityId: opts.communityId,
  });

  await setUserAvatar({
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: user.accessToken,
    userId: user.userId,
    avatarPath: opts.avatarPath,
  });

  const exp = createTestClient(ExperienceService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: user.accessToken,
  });
  const intention =
    opts.rsvp === 'maybe'
      ? RSVPIntention.RSVP_INTENTION_MAYBE
      : opts.rsvp === 'no'
        ? RSVPIntention.RSVP_INTENTION_NO
        : RSVPIntention.RSVP_INTENTION_YES;
  await exp.rSVPToExperience({
    experienceId: opts.experienceId,
    communityId: opts.communityId,
    intention,
  });

  return user;
}

export interface SetUserAvatarOptions {
  baseUrl: string;
  specSlug: string;
  accessToken: string;
  userId: string;
  avatarPath: string;
}

/** Upload a portrait and set it as the user's profile avatar. */
export async function setUserAvatar(opts: SetUserAvatarOptions): Promise<string> {
  const mediaId = await uploadMedia({
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
    path: opts.avatarPath,
    contentType: 'image/jpeg',
    filename: 'avatar.jpg',
  });
  const users = createTestClient(UserService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  await users.saveUser({ userId: opts.userId, mediaId });
  return mediaId;
}
