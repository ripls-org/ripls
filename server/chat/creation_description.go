package chat

import (
	"context"
	"fmt"
	"strings"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// PostCreationDescription posts the creator's free-form description as the first
// user comment in a newly created entity's conversation (experience, gear, or
// request), immediately after the creation anchor system message. It is a no-op
// returning nil when writer or conversationID is empty, or when description is
// blank after trimming.
//
// The comment is timestamped one second after the anchor so it sorts
// deterministically after the anchor in conversation history: messages are
// ordered by sent_at_unix_sec and ties are broken on a random UUID id, so a
// same-second insert would otherwise order nondeterministically relative to the
// anchor. clock.UnixSec truncates to whole seconds and is stable within a single
// handler, so now+1 reliably lands after the anchor inserted in the same call.
//
// The raw description is posted verbatim (only the empty-check is trimmed) so
// the comment text matches the description stored on the entity exactly.
func PostCreationDescription(
	ctx context.Context,
	writer *SystemMessageWriter,
	conversationID, actorID, description string,
) error {
	if writer == nil || conversationID == "" {
		return nil
	}
	if strings.TrimSpace(description) == "" {
		return nil
	}
	sentAt := clock.UnixSec(ctx) + 1
	if err := writer.InsertUserMessageOnBehalfOf(
		ctx,
		conversationID,
		actorID,
		description,
		UserMessageOptions{SentAtUnixSec: &sentAt},
	); err != nil {
		return fmt.Errorf("failed to post creation description to chat: %w", err)
	}
	return nil
}

// PostGearCreationMessages seeds a newly created gear conversation with the
// sharing anchor (LOAN_SHARED / GIVEAWAY_SHARED) followed by the owner's
// description as the first comment — the same opening pair experiences and
// requests get.
//
// It is called from both gear conversation-creation paths so the conversation
// opens identically no matter which one births it: ShareGear (the share-time
// path) and the chat service's backfill path (the conversation born lazily at
// chat-open when share-time creation was missed, #2509). The anchor and the
// description are both attributed to the gear owner (actorID = ownerID).
//
// No-op when writer is nil, conversationID is empty, or the availability is not
// loan/giveaway (no sharing context to anchor). A blank description still emits
// the anchor but no comment (PostCreationDescription handles the empty case).
func PostGearCreationMessages(
	ctx context.Context,
	writer *SystemMessageWriter,
	conversationID, ownerID, ownerName, gearName, description string,
	availability models.Availability,
) error {
	if writer == nil || conversationID == "" {
		return nil
	}
	switch availability {
	case models.Availability_AVAILABILITY_FOR_GIVEAWAY:
		if err := writer.InsertLocalized(ctx, conversationID, ownerID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_GIVEAWAY_SHARED,
			GearSharedForGiveawayMessage(ownerName, gearName)); err != nil {
			return fmt.Errorf("failed to post giveaway anchor: %w", err)
		}
	case models.Availability_AVAILABILITY_FOR_LOAN:
		if err := writer.InsertLocalized(ctx, conversationID, ownerID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_LOAN_SHARED,
			GearSharedForLoanMessage(ownerName, gearName)); err != nil {
			return fmt.Errorf("failed to post loan anchor: %w", err)
		}
	default:
		// No sharing context (e.g. AVAILABILITY_UNSPECIFIED) — nothing to seed.
		return nil
	}
	return PostCreationDescription(ctx, writer, conversationID, ownerID, description)
}
