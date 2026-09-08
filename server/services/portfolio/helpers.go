package portfolio

// Small shared helpers for the portfolio service: message inspection, unread
// counting, and per-entity field extraction. What remains here after #2830
// removed the legacy inbox RPCs — the row-assembly helpers that made up most
// of this file went with them.

import (
	"strings"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

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

// computeUnreadCount counts messages unread by the given user from other senders.
// Creation anchor messages are excluded — they exist to anchor UI cards and should
// not inflate unread badges.
func computeUnreadCount(messages []proto.Message, userID string) int32 {
	count := int32(0)
	for _, m := range messages {
		msg := m.(*models.ChatMessage)
		if chat.IsCreationAnchorMessage(msg) {
			continue
		}
		if messageSenderID(msg) != userID && !msg.ParticipantIdToIsRead[userID] {
			count++
		}
	}
	return count
}

// firstName returns the first word of a display name.
func firstName(displayName string) string {
	parts := strings.Fields(displayName)
	if len(parts) == 0 {
		return displayName
	}
	return parts[0]
}

// firstMediaID returns the first media ID from a list, or empty string if none.
func firstMediaID(mediaIDs []string) string {
	if len(mediaIDs) > 0 {
		return mediaIDs[0]
	}
	return ""
}

// experienceScheduledTime extracts the scheduled start time from an experience.
func experienceScheduledTime(e *models.Experience) int64 {
	if e.Time == nil {
		return 0
	}
	if spec := e.Time.GetSpecific(); spec != nil {
		return spec.UnixTimestampSec
	}
	if r := e.Time.GetRange(); r != nil {
		return r.StartUnixSec
	}
	return 0
}

// experienceIsAllDay reports whether the experience has a date but no
// time-of-day, so the UI shows "All day" rather than a midnight clock.
func experienceIsAllDay(e *models.Experience) bool {
	if e == nil || e.Time == nil {
		return false
	}
	if spec := e.Time.GetSpecific(); spec != nil {
		return spec.IsAllDay
	}
	if r := e.Time.GetRange(); r != nil {
		return r.IsAllDay
	}
	return false
}

// requestTitle returns the most human-readable title for a request.
func requestTitle(r *models.Request) string {
	if r.Title != "" {
		return r.Title
	}
	desc := r.Description
	runes := []rune(desc)
	if len(runes) > 60 {
		return string(runes[:60]) + "…"
	}
	return desc
}

// dedup returns a deduplicated copy of the input slice (preserving first occurrence order).
func dedup(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
