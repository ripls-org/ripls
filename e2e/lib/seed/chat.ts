// e2e/lib/seed/chat.ts — send chat messages into an experience's conversation
// via RPC, as a given user.
//
// Exists for the user journey walkthroughs (#2684): the on-camera actor watches
// photos + comments stream in live, and chat-photo AUTHORING is blocked on
// Flutter Web (inline_conversation_view.dart defers the picker via
// WebUnsupported), so off-camera cast members must post through the public
// ChatService instead. Also usable as ordinary workflow-spec precondition
// seeding.

import { ChatService } from '../../gen/ripls/api/chat_service_pb.js';
import { GearService } from '../../gen/ripls/api/gear_service_pb.js';
import { createTestClient } from '../connect.js';

export interface SendRequestChatMessageOptions {
  baseUrl: string;
  specSlug: string;
  /** The SENDER's access token — messages are attributed to this user. */
  accessToken: string;
  requestId: string;
  text?: string;
  /** Media ids previously uploaded by the same sender (uploadMedia). */
  mediaIds?: string[];
}

/**
 * Send one message into the request's single shared conversation (created when
 * the request is born into its per-item community, reused across shares — see
 * docs/workflows/request.md § Conversation Integration). The sender must be a
 * participant (the requester, or an offerer via OfferToFulfill). Returns the
 * message id.
 */
export async function sendRequestChatMessage(
  opts: SendRequestChatMessageOptions,
): Promise<string> {
  const client = createTestClient(ChatService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const conv = await client.getConversationForRequest({ requestId: opts.requestId });
  const conversationId = conv.conversation?.conversationId;
  if (!conversationId) {
    throw new Error(`no conversation for request ${opts.requestId}`);
  }
  const resp = await client.sendMessage({
    conversationId,
    text: opts.text ?? '',
    mediaIds: opts.mediaIds ?? [],
  });
  if (!resp.messageId) throw new Error('SendMessage returned empty message id');
  return resp.messageId;
}

export interface SendExperienceChatMessageOptions {
  baseUrl: string;
  specSlug: string;
  /** The SENDER's access token — messages are attributed to this user. */
  accessToken: string;
  experienceId: string;
  text?: string;
  /** Media ids previously uploaded by the same sender (uploadMedia). */
  mediaIds?: string[];
}

/**
 * Send one message (text and/or photos) into the experience's conversation.
 * Returns the message id.
 */
export async function sendExperienceChatMessage(
  opts: SendExperienceChatMessageOptions,
): Promise<string> {
  const client = createTestClient(ChatService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const conv = await client.getConversationForExperience({ experienceId: opts.experienceId });
  const conversationId = conv.conversation?.conversationId;
  if (!conversationId) {
    throw new Error(`no conversation for experience ${opts.experienceId}`);
  }
  const resp = await client.sendMessage({
    conversationId,
    text: opts.text ?? '',
    mediaIds: opts.mediaIds ?? [],
  });
  if (!resp.messageId) throw new Error('SendMessage returned empty message id');
  return resp.messageId;
}

export interface SendGearChatMessageOptions {
  baseUrl: string;
  specSlug: string;
  /** The SENDER's access token — messages are attributed to this user. */
  accessToken: string;
  gearId: string;
  /** Optional community context for resolving the conversation. */
  communityId?: string;
  text?: string;
  /** Media ids previously uploaded by the same sender (uploadMedia). */
  mediaIds?: string[];
}

/**
 * Send one message into the gear item's perpetual conversation (loans and
 * giveaways share it — see docs/workflows/giveaway.md § Conversation Model).
 * The conversation id rides on GetGear. Returns the message id.
 */
export async function sendGearChatMessage(
  opts: SendGearChatMessageOptions,
): Promise<string> {
  const clientOpts = {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  };
  const gear = await createTestClient(GearService, clientOpts).getGear({
    id: opts.gearId,
    communityId: opts.communityId ?? '',
  });
  const conversationId = gear.conversationId;
  if (!conversationId) {
    throw new Error(`no conversation for gear ${opts.gearId}`);
  }
  const resp = await createTestClient(ChatService, clientOpts).sendMessage({
    conversationId,
    text: opts.text ?? '',
    mediaIds: opts.mediaIds ?? [],
  });
  if (!resp.messageId) throw new Error('SendMessage returned empty message id');
  return resp.messageId;
}
