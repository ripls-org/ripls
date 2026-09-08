package planning

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// PostNeedAdded posts a chat message for a newly added need.
// If the need has a note, it is posted as a user message so participants can react;
// otherwise a system message is used.
//
// sentAtUnixSec optionally overrides the message timestamp. Conversation
// history breaks same-second ties on a random UUID id, so batch callers that
// post one line per need in a single handler pass sequential timestamps to
// keep the lines in creation order (#2724); single-add callers pass nil.
func PostNeedAdded(
	ctx context.Context,
	writer *chat.SystemMessageWriter,
	conversationID, userID, displayName string,
	need *models.PlanningNeed,
	sentAtUnixSec *int64,
) error {
	if writer == nil || conversationID == "" {
		return nil
	}
	needID := need.Id
	if need.Note != nil && *need.Note != "" {
		if err := writer.InsertUserMessageOnBehalfOf(
			ctx,
			conversationID,
			userID,
			*need.Note,
			chat.UserMessageOptions{NeedID: &needID, NeedName: &need.Name, SentAtUnixSec: sentAtUnixSec},
		); err != nil {
			return fmt.Errorf("failed to post need note to chat: %w", err)
		}
		return nil
	}
	if err := writer.InsertLocalized(
		ctx,
		conversationID,
		userID,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_ADDED,
		chat.PlanningNeedAddedMessage(displayName, need.Name, ""),
		chat.SystemMessageInsertOptions{CoalesceKey: &needID, SentAtUnixSec: sentAtUnixSec},
	); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to write need-added system message", "error", err)
	}
	return nil
}

// PostNeedRemoved posts a system message for a removed need.
func PostNeedRemoved(
	ctx context.Context,
	writer *chat.SystemMessageWriter,
	conversationID, userID, displayName, needID, needName string,
) {
	if writer == nil || conversationID == "" {
		return
	}
	if err := writer.InsertLocalized(
		ctx,
		conversationID,
		userID,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_REMOVED,
		chat.PlanningNeedRemovedMessage(displayName, needName),
		chat.SystemMessageInsertOptions{CoalesceKey: &needID},
	); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to write need-removed system message", "error", err)
	}
}

// PostNeedUpdated posts a system message recording an in-place edit to a
// need's name, note, or slot count. Same coalesce family as add/remove
// so a series of edits collapses into a single chat card.
func PostNeedUpdated(
	ctx context.Context,
	writer *chat.SystemMessageWriter,
	conversationID, userID, displayName, needID, needName string,
) {
	if writer == nil || conversationID == "" {
		return
	}
	text := chat.ExperienceNeedUpdatedText(displayName, needName)
	if err := writer.InsertSystemMessage(
		ctx,
		conversationID,
		userID,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_UPDATED,
		text,
		chat.SystemMessageInsertOptions{CoalesceKey: &needID},
	); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to write need-updated system message", "error", err)
	}
}

// PostNeedClaimed posts a chat message for a claimed need slot.
//
// The line is stamped one second after the current clock so it always sorts
// after a same-second sibling from the same flow (the client RSVPs, then
// claims — history breaks same-second ties on a random UUID id, which used to
// put "X is attending" AFTER "X is bringing: …", #2724). Same pattern as
// chat.PostCreationDescription.
func PostNeedClaimed(
	ctx context.Context,
	writer *chat.SystemMessageWriter,
	conversationID, userID, displayName string,
	need *models.PlanningNeed,
	contrib *models.PlanningContribution,
) error {
	if writer == nil || conversationID == "" {
		return nil
	}
	contribID := contrib.Id
	sentAt := clock.UnixSec(ctx) + 1
	if contrib.Description != nil && *contrib.Description != "" {
		if err := writer.InsertUserMessageOnBehalfOf(
			ctx,
			conversationID,
			userID,
			*contrib.Description,
			chat.UserMessageOptions{ContributionID: &contribID, ContributionTitle: &contrib.Title, SentAtUnixSec: &sentAt},
		); err != nil {
			return fmt.Errorf("failed to post claim note to chat: %w", err)
		}
		return nil
	}
	if err := writer.InsertLocalized(
		ctx,
		conversationID,
		userID,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_CLAIMED,
		chat.PlanningNeedClaimedMessage(displayName, need.Name, ""),
		chat.SystemMessageInsertOptions{CoalesceKey: &contribID, SentAtUnixSec: &sentAt},
	); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to write need-claimed system message", "error", err)
	}
	return nil
}

// PostContributionAdded posts a chat message for a new free-form contribution.
//
// Stamped one second after the current clock for the same reason as
// PostNeedClaimed: the contribute-while-RSVPing flow emits both lines in the
// same second, and the attendance line must sort first (#2724).
func PostContributionAdded(
	ctx context.Context,
	writer *chat.SystemMessageWriter,
	conversationID, userID, displayName string,
	contrib *models.PlanningContribution,
) error {
	if writer == nil || conversationID == "" {
		return nil
	}
	contribID := contrib.Id
	sentAt := clock.UnixSec(ctx) + 1
	if contrib.Description != nil && *contrib.Description != "" {
		if err := writer.InsertUserMessageOnBehalfOf(
			ctx,
			conversationID,
			userID,
			*contrib.Description,
			chat.UserMessageOptions{ContributionID: &contribID, ContributionTitle: &contrib.Title, SentAtUnixSec: &sentAt},
		); err != nil {
			return fmt.Errorf("failed to post contribution description to chat: %w", err)
		}
		return nil
	}
	if err := writer.InsertLocalized(
		ctx,
		conversationID,
		userID,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_CONTRIBUTION_ADDED,
		chat.PlanningContributionAddedMessage(displayName, contrib.Title, ""),
		chat.SystemMessageInsertOptions{CoalesceKey: &contribID, SentAtUnixSec: &sentAt},
	); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to write contribution-added system message", "error", err)
	}
	return nil
}

// PostContributionRemoved posts a system message for a removed contribution.
func PostContributionRemoved(
	ctx context.Context,
	writer *chat.SystemMessageWriter,
	conversationID, userID, displayName, contribID, title string,
) {
	if writer == nil || conversationID == "" {
		return
	}
	if err := writer.InsertLocalized(
		ctx,
		conversationID,
		userID,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_CONTRIBUTION_REMOVED,
		chat.PlanningContributionRemovedMessage(displayName, title),
		chat.SystemMessageInsertOptions{CoalesceKey: &contribID},
	); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to write contribution-removed system message", "error", err)
	}
}
