package chat_subscriber

import (
	"context"

	libchat "go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/chat_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/storage"
)

// topicResolution carries the per-event entity-name lookup results
// hoisted out of the recipient loop. resolveTopic populates one of
// the title fields plus (for TransferId topics) the resolved gear ID.
// All fields are optional — a missing entity name is benign and leaves
// the client to fall back to the sender's name.
type topicResolution struct {
	title          string
	transferGearID string
}

// resolveTopic performs the entity-name lookups needed to populate
// the chat payload's ConversationTitle (and, for transfer topics, the
// GearId deep link). It runs once per event before the recipient
// fan-out so a 200-member community pays one lookup, not 200 — see
// issue #2173. Missing entities are benign and return an empty title.
func resolveTopic(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	evt *chat_event_bus.PublishedEvent,
) topicResolution {
	var res topicResolution
	if topic := evt.Conversation.Topic; topic != nil {
		switch t := topic.TopicId.(type) {
		case *models.ConversationTopic_ExperienceId:
			res.title = lookupExperienceName(ctx, s, t.ExperienceId)
		case *models.ConversationTopic_GearId:
			res.title = lookupGearName(ctx, s, t.GearId)
		case *models.ConversationTopic_RequestId:
			res.title = lookupRequestTitle(ctx, s, t.RequestId)
		case *models.ConversationTopic_TransferId:
			// Resolve transfer → gear_id for deep-link. Use the prefetched transfer
			// if available; fall through to a live lookup on prefetch failure.
			if evt.Transfer != nil {
				res.transferGearID = evt.Transfer.GearId
			} else {
				transfer := &models.Transfer{}
				if err := s.GetByID(ctx, t.TransferId, transfer); err == nil {
					res.transferGearID = transfer.GearId
				}
			}
			if res.transferGearID != "" {
				res.title = lookupGearName(ctx, s, res.transferGearID)
			}
		}
	}

	// Community-wide conversations (no topic, or a CommunityId topic) take
	// their title from the community name so the user sees which community
	// the message belongs to.
	if res.title == "" && evt.Conversation.CommunityId != "" {
		res.title = lookupCommunityName(ctx, s, evt.Conversation.CommunityId)
	}
	return res
}

// buildNotification assembles the push notification for recipientID
// from a PublishedEvent. The event must be KindUserMessage with a
// non-nil Message. loc renders the localized frame copy (sender
// fallback name, "mentioned you" suffix) for the recipient's locale;
// pass nil to fall through to English. topic carries the pre-resolved
// entity-name and (for transfer topics) gear ID — see resolveTopic.
//
// The author's typed message body (previewText) is never translated
// — it is shown in whatever language the author wrote it in.
func buildNotification(
	ctx context.Context,
	evt *chat_event_bus.PublishedEvent,
	recipientID string,
	loc *l10n.Localizer,
	topic topicResolution,
) *models.Notification {
	userMsg := evt.Message.GetUserMessage()

	senderName := loc.T(ctx, "notif.chat.sender_fallback", nil)
	if evt.Sender != nil && evt.Sender.Name != "" {
		senderName = evt.Sender.Name
	}

	previewText := libchat.DecodeMentions(userMsg.GetText())
	if len(previewText) > 100 {
		previewText = previewText[:100]
	}

	// Customize title for mentioned users.
	title := senderName
	if isMentioned(evt.MentionedUserIDs, recipientID) {
		title = loc.T(ctx, "notif.chat.mentioned_you", map[string]any{
			"SenderName": senderName,
		})
	}

	senderID := userMsg.GetSenderId()
	chatPayload := &models.ChatMessagePayload{
		ConversationId: evt.Conversation.Id,
		MessageId:      evt.Message.Id,
		SenderUserId:   senderID,
		SenderName:     senderName,
		PreviewText:    previewText,
		CommunityId:    evt.Conversation.CommunityId,
	}

	// Populate topic entity IDs for deep linking. The title was resolved
	// once per event in resolveTopic and is shared across recipients.
	if t := evt.Conversation.Topic; t != nil {
		switch tid := t.TopicId.(type) {
		case *models.ConversationTopic_ExperienceId:
			chatPayload.ExperienceId = &tid.ExperienceId
		case *models.ConversationTopic_GearId:
			chatPayload.GearId = &tid.GearId
		case *models.ConversationTopic_RequestId:
			chatPayload.RequestId = &tid.RequestId
		case *models.ConversationTopic_TransferId:
			if topic.transferGearID != "" {
				gearID := topic.transferGearID
				chatPayload.GearId = &gearID
			}
		}
	}

	if topic.title != "" {
		convTitle := topic.title
		chatPayload.ConversationTitle = &convTitle
	}

	return &models.Notification{
		Title: title,
		Body:  previewText,
		Payload: &models.Notification_ChatMessage{
			ChatMessage: chatPayload,
		},
	}
}

// lookupGearName returns the human-readable gear name, or empty string when
// the lookup fails (gear deleted, transient DB error, etc.). Best-effort —
// a missing title is acceptable; the client falls back to the sender's name.
func lookupGearName(ctx context.Context, s *storage.ProtoSQLStorage, gearID string) string {
	gear := &models.Gear{}
	if err := s.GetByID(ctx, gearID, gear); err != nil {
		return ""
	}
	return gear.Name
}

// lookupExperienceName returns the human-readable experience name (best-effort).
func lookupExperienceName(ctx context.Context, s *storage.ProtoSQLStorage, experienceID string) string {
	experience := &models.Experience{}
	if err := s.GetByID(ctx, experienceID, experience); err != nil {
		return ""
	}
	return experience.Name
}

// lookupRequestTitle returns the human-readable request title (best-effort).
func lookupRequestTitle(ctx context.Context, s *storage.ProtoSQLStorage, requestID string) string {
	request := &models.Request{}
	if err := s.GetByID(ctx, requestID, request); err != nil {
		return ""
	}
	return request.Title
}

// lookupCommunityName returns the human-readable community name (best-effort).
func lookupCommunityName(ctx context.Context, s *storage.ProtoSQLStorage, communityID string) string {
	community := &models.Community{}
	if err := s.GetByID(ctx, communityID, community); err != nil {
		return ""
	}
	return community.Name
}

// isMentioned reports whether userID is in the mentionedUserIDs slice.
func isMentioned(mentionedUserIDs []string, userID string) bool {
	for _, id := range mentionedUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// chatsCategoryForPrefs returns the notification category used for chat
// message preference gating. Package-level helper so the import of models is
// centralized here and not duplicated in subscriber.go.
func chatsCategoryForPrefs() models.NotificationCategory {
	return models.NotificationCategory_NOTIFICATION_CATEGORY_CHATS
}
