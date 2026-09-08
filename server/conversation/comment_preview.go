package conversation

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/chat"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

const maxPreviewTextLength = 100

// CommentPreview holds denormalized comment preview data for an item's conversation.
type CommentPreview struct {
	UnreadCount        int32
	LastMessageText    *string
	LastMessageSender  *api.User
	LastMessageTimeAgo *string
	RecentCommenters   []*api.User
}

// EnrichWithCommentPreview computes comment preview data for a single conversation.
// Returns an empty preview (no error) if conversationID is empty or has no messages.
func EnrichWithCommentPreview(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	conversationID string,
	currentUserID string,
) (*CommentPreview, error) {
	if conversationID == "" {
		return &CommentPreview{}, nil
	}

	messages, err := store.QueryByField(ctx, "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		return nil, fmt.Errorf("query messages for comment preview: %w", err)
	}

	if len(messages) == 0 {
		return &CommentPreview{}, nil
	}

	return buildPreview(ctx, store, messages, currentUserID, time.Now())
}

// BatchEnrichWithCommentPreview computes comment previews for multiple conversations
// in a single batch to avoid N+1 queries.
// Returns a map of conversationID -> CommentPreview.
func BatchEnrichWithCommentPreview(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	conversationIDs []string,
	currentUserID string,
) (map[string]*CommentPreview, error) {
	previews, _, _, err := BatchEnrichWithCommentPreviewAndCounts(ctx, store, conversationIDs, currentUserID)
	return previews, err
}

// BatchEnrichWithCommentPreviewAndCounts computes comment previews, non-system
// message counts, and chronological poster IDs for multiple conversations in a
// single batch to avoid N+1 queries.
// Returns a map of conversationID -> CommentPreview, a map of
// conversationID -> non-system message count, and a map of
// conversationID -> deduplicated user-message sender IDs in first-post order.
func BatchEnrichWithCommentPreviewAndCounts(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	conversationIDs []string,
	currentUserID string,
) (map[string]*CommentPreview, map[string]int32, map[string][]string, error) {
	logger := logging.LoggerWithContext(ctx).With("operation", "BatchEnrichWithCommentPreviewAndCounts")

	previews := make(map[string]*CommentPreview, len(conversationIDs))
	counts := make(map[string]int32, len(conversationIDs))
	postersByConv := make(map[string][]string, len(conversationIDs))

	// Filter out empty IDs.
	nonEmpty := make([]string, 0, len(conversationIDs))
	for _, id := range conversationIDs {
		if id != "" {
			nonEmpty = append(nonEmpty, id)
		}
	}
	if len(nonEmpty) == 0 {
		return previews, counts, postersByConv, nil
	}

	// Batch-fetch all messages across all conversations.
	allMessages, err := storage.QueryByFieldIn[*models.ChatMessage](store, ctx, "conversation_id", nonEmpty)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("batch query messages for comment previews: %w", err)
	}

	// Group messages by conversation ID.
	messagesByConv := make(map[string][]proto.Message)
	for _, msg := range allMessages {
		messagesByConv[msg.ConversationId] = append(messagesByConv[msg.ConversationId], msg)
	}

	// Collect all sender user IDs across all conversations for a single batch user fetch.
	allChatMessages := make([]*models.ChatMessage, 0, len(allMessages))
	allChatMessages = append(allChatMessages, allMessages...)
	allUserIDs := collectAllSenderIDs(allChatMessages)

	userMap, err := services.FetchAPIUsersBatch(ctx, store, allUserIDs)
	if err != nil {
		logger.Warn("failed to batch fetch users for comment previews", "error", err)
		userMap = make(map[string]*api.User)
	}

	now := time.Now()
	for _, convID := range nonEmpty {
		msgs := messagesByConv[convID]
		if len(msgs) == 0 {
			previews[convID] = &CommentPreview{}
			counts[convID] = 0
			postersByConv[convID] = nil
			continue
		}
		preview, err := buildPreviewWithUserMap(msgs, currentUserID, userMap, now)
		if err != nil {
			logger.Warn("failed to build comment preview", "conversation_id", convID, "error", err)
			previews[convID] = &CommentPreview{}
			counts[convID] = 0
			postersByConv[convID] = nil
			continue
		}
		previews[convID] = preview

		// Count non-system messages and build poster list in one pass.
		// postersByConv: deduplicated user-message sender IDs, first-post order.
		var count int32
		seen := make(map[string]bool)
		var posters []string
		for _, m := range msgs {
			chatMsg := m.(*models.ChatMessage)
			if !chat.IsSystemMessage(chatMsg) {
				count++
			}
			if userMsg := chatMsg.GetUserMessage(); userMsg != nil {
				if !seen[userMsg.SenderId] {
					seen[userMsg.SenderId] = true
					posters = append(posters, userMsg.SenderId)
				}
			}
		}
		counts[convID] = count
		postersByConv[convID] = posters
	}

	return previews, counts, postersByConv, nil
}

// buildPreview computes a CommentPreview from messages, fetching user data as needed.
func buildPreview(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	messages []proto.Message,
	currentUserID string,
	now time.Time,
) (*CommentPreview, error) {
	// Collect all unique sender IDs.
	chatMessages := make([]*models.ChatMessage, 0, len(messages))
	for _, m := range messages {
		chatMessages = append(chatMessages, m.(*models.ChatMessage))
	}

	allUserIDs := collectAllSenderIDs(chatMessages)

	userMap, err := services.FetchAPIUsersBatch(ctx, store, allUserIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch users for comment preview: %w", err)
	}

	return buildPreviewWithUserMap(messages, currentUserID, userMap, now)
}

// buildPreviewWithUserMap computes a CommentPreview from messages using a pre-fetched user map.
func buildPreviewWithUserMap(
	messages []proto.Message,
	currentUserID string,
	userMap map[string]*api.User,
	now time.Time,
) (*CommentPreview, error) {
	preview := &CommentPreview{}

	// Compute unread count and find the most recent non-system message.
	// All system messages (joins, approvals, creation anchors, etc.) are excluded
	// from both unread counts and last-message preview.
	preview.UnreadCount = chat.CountUnreadUserMessages(messages, currentUserID)

	var latest *models.ChatMessage
	for _, m := range messages {
		msg := m.(*models.ChatMessage)
		if chat.IsSystemMessage(msg) {
			continue
		}
		if latest == nil || msg.SentAtUnixSec > latest.SentAtUnixSec {
			latest = msg
		}
	}

	if latest != nil {
		var text string
		switch m := latest.Message.(type) {
		case *models.ChatMessage_UserMessage:
			var senderName string
			if u := userMap[m.UserMessage.SenderId]; u != nil {
				preview.LastMessageSender = u
				senderName = firstName(u.Name)
			}
			if m.UserMessage.Text != "" {
				// Decode encoded @-mentions to "@Display Name" so previews
				// render human-readable text rather than the raw on-wire form.
				text = truncateText(chat.DecodeMentions(m.UserMessage.Text), maxPreviewTextLength)
			} else if len(m.UserMessage.MediaIds) > 0 {
				// Image-only message — generate a readable fallback.
				if senderName != "" {
					text = senderName + " shared an image"
				} else {
					text = "shared an image"
				}
			}
		case *models.ChatMessage_SystemMessage:
			// Set the actor as sender so the client can show their avatar.
			// The client does not display the name for system messages since
			// the actor's name is already embedded in the description text.
			if actorID := m.SystemMessage.GetActorId(); actorID != "" {
				if u := userMap[actorID]; u != nil {
					preview.LastMessageSender = u
				}
			}
			text = truncateText(m.SystemMessage.Description, maxPreviewTextLength)
		}
		if text != "" {
			preview.LastMessageText = &text
		}

		timeAgo := formatTimeAgo(latest.SentAtUnixSec, now)
		if timeAgo != "" {
			preview.LastMessageTimeAgo = &timeAgo
		}
	}

	// Build recent commenters: up to 3 most recent unique user-message senders (excluding current user).
	preview.RecentCommenters = buildRecentCommenters(messages, userMap, 3)

	return preview, nil
}

// buildRecentCommenters returns up to maxCount unique commenters ordered by most recent message.
func buildRecentCommenters(
	messages []proto.Message,
	userMap map[string]*api.User,
	maxCount int,
) []*api.User {
	// Sort by recency: iterate newest-first to build ordered unique list.
	type senderTime struct {
		senderID string
		sentAt   int64
	}
	senders := make([]senderTime, 0, len(messages))
	for _, m := range messages {
		msg := m.(*models.ChatMessage)
		// Only count user messages (not system messages).
		if msg.GetUserMessage() == nil {
			continue
		}
		sid := messageSenderID(msg)
		if sid == "" {
			continue
		}
		senders = append(senders, senderTime{senderID: sid, sentAt: msg.SentAtUnixSec})
	}

	// Find most recent time per sender (including current user).
	latestBySender := make(map[string]int64)
	for _, s := range senders {
		if s.sentAt > latestBySender[s.senderID] {
			latestBySender[s.senderID] = s.sentAt
		}
	}

	// Sort senders by their most recent message time (descending).
	type rankedSender struct {
		senderID string
		sentAt   int64
	}
	ranked := make([]rankedSender, 0, len(latestBySender))
	for id, t := range latestBySender {
		ranked = append(ranked, rankedSender{senderID: id, sentAt: t})
	}
	// Simple insertion sort since list is small.
	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && ranked[j].sentAt > ranked[j-1].sentAt; j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}

	result := make([]*api.User, 0, maxCount)
	for _, r := range ranked {
		if len(result) >= maxCount {
			break
		}
		if u := userMap[r.senderID]; u != nil {
			result = append(result, u)
		}
	}
	return result
}

// messageSenderID extracts the sender user ID from a chat message.
func messageSenderID(msg *models.ChatMessage) string {
	switch m := msg.Message.(type) {
	case *models.ChatMessage_UserMessage:
		return m.UserMessage.SenderId
	case *models.ChatMessage_SystemMessage:
		return m.SystemMessage.GetActorId()
	default:
		return ""
	}
}

// truncateText truncates text to maxLen runes.
func truncateText(text string, maxLen int) string {
	if utf8.RuneCountInString(text) <= maxLen {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxLen])
}

// formatTimeAgo returns a human-readable relative time like "12m", "2h", "3d".
func formatTimeAgo(unixSec int64, now time.Time) string {
	if unixSec <= 0 {
		return ""
	}
	diff := now.Unix() - unixSec
	if diff < 0 {
		diff = 0
	}
	switch {
	case diff < 3600:
		return fmt.Sprintf("%dm", diff/60)
	case diff < 86400:
		return fmt.Sprintf("%dh", diff/3600)
	default:
		return fmt.Sprintf("%dd", diff/86400)
	}
}

// firstName returns the first word of a full name, or the full name if it has no spaces.
func firstName(name string) string {
	for i, r := range name {
		if r == ' ' {
			return name[:i]
		}
	}
	return name
}

// collectAllSenderIDs extracts unique sender IDs from a slice of ChatMessages.
func collectAllSenderIDs(messages []*models.ChatMessage) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, msg := range messages {
		sid := messageSenderID(msg)
		if sid != "" && !seen[sid] {
			seen[sid] = true
			ids = append(ids, sid)
		}
	}
	return ids
}
