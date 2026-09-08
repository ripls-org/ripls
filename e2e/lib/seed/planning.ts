// e2e/lib/seed/planning.ts — seed Needs & Contributions (docs/planning.md)
// for BOTH planning scopes via their public RPCs: Experience-scope over
// ExperienceService, Request-scope over RequestService.
//
// Used by the user journey walkthroughs (#2684) to let off-camera cast members
// post and claim needs while one actor is on camera, and as ordinary
// precondition seeding for Plan-tab specs.
//
// - Experience scope: callers must be the owner or a YES/MAYBE RSVP of the
//   experience (claiming auto-RSVPs the claimer YES).
// - Request scope (#2703): only the requester may post needs; any community
//   member may claim a need or add a freestanding contribution. Claims and
//   contributions may optionally link a Gear item (the gear-pill on the row).

import { ExperienceService } from '../../gen/ripls/api/experience_service_pb.js';
import { RequestService } from '../../gen/ripls/api/request_service_pb.js';
import { createTestClient } from '../connect.js';

interface PlanningCallOptions {
  baseUrl: string;
  specSlug: string;
  /** The ACTOR's access token — needs/claims are attributed to this user. */
  accessToken: string;
  experienceId: string;
}

function planningClient(opts: PlanningCallOptions) {
  return createTestClient(ExperienceService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
}

/** Post a need ("Request Something"); returns the need id for claims. */
export async function addExperienceNeed(
  opts: PlanningCallOptions & { name: string; note?: string; slots?: number },
): Promise<string> {
  const resp = await planningClient(opts).addExperienceNeed({
    experienceId: opts.experienceId,
    name: opts.name,
    note: opts.note,
    slots: opts.slots ?? 1,
  });
  const id = resp.need?.id;
  if (!id) throw new Error(`AddExperienceNeed(${opts.name}) returned empty need id`);
  return id;
}

/** Claim a need ("I'll bring it"); returns the created contribution id. */
export async function claimExperienceNeed(
  opts: PlanningCallOptions & { needId: string; note?: string },
): Promise<string> {
  const resp = await planningClient(opts).claimExperienceNeed({
    needId: opts.needId,
    experienceId: opts.experienceId,
    note: opts.note,
  });
  const id = resp.contribution?.id;
  if (!id) throw new Error(`ClaimExperienceNeed(${opts.needId}) returned empty contribution id`);
  return id;
}

/** Add a freestanding contribution ("Offer Something"). */
export async function addExperienceContribution(
  opts: PlanningCallOptions & { title: string; description?: string },
): Promise<string> {
  const resp = await planningClient(opts).addExperienceContribution({
    experienceId: opts.experienceId,
    title: opts.title,
    description: opts.description,
  });
  const id = resp.contribution?.id;
  if (!id) throw new Error(`AddExperienceContribution(${opts.title}) returned empty contribution id`);
  return id;
}

// ── Request scope (docs/planning.md) — the request-need twins (#2703) ──
// Mirror the experience functions above, over RequestService. The requester
// posts needs ("Break it down"); any community member claims a need or adds a
// freestanding contribution, optionally linking a Gear item.

interface RequestPlanningCallOptions {
  baseUrl: string;
  specSlug: string;
  /** The ACTOR's access token — needs/claims are attributed to this user. */
  accessToken: string;
  requestId: string;
}

function requestPlanningClient(opts: RequestPlanningCallOptions) {
  return createTestClient(RequestService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
}

/** Post a need ("Break it down"); requester only. Returns the need id. */
export async function addRequestNeed(
  opts: RequestPlanningCallOptions & { name: string; note?: string; slots?: number },
): Promise<string> {
  const resp = await requestPlanningClient(opts).addRequestNeed({
    requestId: opts.requestId,
    name: opts.name,
    note: opts.note,
    slots: opts.slots ?? 1,
  });
  const id = resp.need?.id;
  if (!id) throw new Error(`AddRequestNeed(${opts.name}) returned empty need id`);
  return id;
}

/**
 * Claim a need ("I'll help with this"); any community member. `communityId`
 * is the community the claimer is acting from (used to auto-create a
 * RequestOffer if none exists). Pass `gearId` to link the actual item being
 * brought (the gear pill). Returns the created contribution id.
 */
export async function claimRequestNeed(
  opts: RequestPlanningCallOptions & {
    needId: string;
    communityId: string;
    note?: string;
    gearId?: string;
  },
): Promise<string> {
  const resp = await requestPlanningClient(opts).claimRequestNeed({
    needId: opts.needId,
    requestId: opts.requestId,
    communityId: opts.communityId,
    note: opts.note,
    gearId: opts.gearId,
  });
  const id = resp.contribution?.id;
  if (!id) throw new Error(`ClaimRequestNeed(${opts.needId}) returned empty contribution id`);
  return id;
}

/** Add a freestanding contribution; optionally linking a Gear item. */
export async function addRequestContribution(
  opts: RequestPlanningCallOptions & { title: string; description?: string; gearId?: string },
): Promise<string> {
  const resp = await requestPlanningClient(opts).addRequestContribution({
    requestId: opts.requestId,
    title: opts.title,
    description: opts.description,
    gearId: opts.gearId,
  });
  const id = resp.contribution?.id;
  if (!id) throw new Error(`AddRequestContribution(${opts.title}) returned empty contribution id`);
  return id;
}
