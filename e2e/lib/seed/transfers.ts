// e2e/lib/seed/transfers.ts — drive the gear transfer (loan/giveaway) state
// machine via RPC as a given user: express interest, select a recipient,
// complete the transfer.
//
// Exists for the user journey walkthroughs (#2687): claims and handoffs land
// mid-scene while the camera watches the owner's screen, so off-camera cast
// members act through the public TransferService. Also usable as ordinary
// workflow-spec precondition seeding.

import { TransferService } from '../../gen/ripls/api/transfer_service_pb.js';
import { TransferType } from '../../gen/ripls/api/transfer_pb.js';
import { createTestClient } from '../connect.js';

interface TransferRPCOptions {
  baseUrl: string;
  specSlug: string;
  /** The ACTING user's access token — the RPC runs as this user. */
  accessToken: string;
}

export interface ExpressedInterest {
  transferId: string;
  /** The gear conversation the interested user just joined. */
  conversationId: string;
}

/**
 * Express interest in a shared gear item (joins the item's single group
 * transfer, or creates it). The actor must be a member of a community the
 * gear is shared with, and must not be the owner.
 */
export async function expressGearInterest(
  opts: TransferRPCOptions & { gearId: string },
): Promise<ExpressedInterest> {
  const client = createTestClient(TransferService, opts);
  const resp = await client.expressInterest({ gearId: opts.gearId });
  if (!resp.transfer?.id) {
    throw new Error(`ExpressInterest for gear ${opts.gearId} returned no transfer`);
  }
  return {
    transferId: resp.transfer.id,
    conversationId: resp.transfer.conversationId ?? '',
  };
}

/** Select which interested user receives the item (owner only). */
export async function selectGearRecipient(
  opts: TransferRPCOptions & { transferId: string; recipientId: string },
): Promise<void> {
  const client = createTestClient(TransferService, opts);
  await client.selectRecipient({
    transferId: opts.transferId,
    recipientId: opts.recipientId,
  });
}

/**
 * Complete the transfer (owner or selected recipient). For giveaways this
 * moves RECIPIENT_SELECTED → COMPLETED and marks the gear GIVEN_AWAY.
 */
export async function completeGearTransfer(
  opts: TransferRPCOptions & { transferId: string },
): Promise<void> {
  const client = createTestClient(TransferService, opts);
  await client.completeTransfer({ transferId: opts.transferId });
}

/**
 * Start a loan (owner only): RECIPIENT_SELECTED → ACTIVE, the handoff moment.
 * For an origin-linked gear-backed request offer (#2702) this is what
 * auto-fulfills a single-need request.
 */
export async function startGearLoan(
  opts: TransferRPCOptions & { transferId: string },
): Promise<void> {
  const client = createTestClient(TransferService, opts);
  await client.startLoan({ transferId: opts.transferId });
}

/**
 * Escalate a gear-linked request claim into a real give/lend offer targeting
 * the requester (#2702/#2703). The offer is born RECIPIENT_SELECTED with the
 * requester as its recipient; when the requester marks the request fulfilled
 * and confirms this helper, a giveaway completes and a loan goes active.
 * Requires the contribution id from an earlier gear-linked ClaimRequestNeed /
 * AddRequestContribution; `requesterId` must be the request's creator. Returns
 * the created transfer id.
 */
export async function offerRequestTransfer(
  opts: TransferRPCOptions & {
    gearId: string;
    transferType: TransferType;
    requesterId: string;
    communityId: string;
    requestId: string;
    contributionId: string;
    loanDurationDays?: number;
  },
): Promise<string> {
  const client = createTestClient(TransferService, opts);
  const resp = await client.offerTransfer({
    gearId: opts.gearId,
    transferType: opts.transferType,
    recipientUserId: opts.requesterId,
    communityId: opts.communityId,
    originRequestId: opts.requestId,
    contributionId: opts.contributionId,
    loanDurationDays: opts.loanDurationDays,
  });
  if (!resp.transfer?.id) {
    throw new Error(`OfferTransfer for gear ${opts.gearId} returned no transfer`);
  }
  return resp.transfer.id;
}
