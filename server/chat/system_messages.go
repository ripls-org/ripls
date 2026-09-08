package chat

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/chat_event_bus"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// coalesceWindowSec is the time window within which rapid system messages from
// the same actor with the same action family are collapsed into one card.
const coalesceWindowSec = 300 // 5 minutes

// creationAnchorActions is the authoritative set of actions that anchor info
// cards at the top of a conversation. These must never be coalesced and should
// not be counted in comment totals or shown as the most recent comment.
// IsCreationAnchorMessage delegates here so there is one list to maintain.
var creationAnchorActions = map[models.ChatSystemAction]bool{
	models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CREATED: true,
	models.ChatSystemAction_CHAT_SYSTEM_ACTION_REQUEST_CREATED:    true,
	models.ChatSystemAction_CHAT_SYSTEM_ACTION_GIVEAWAY_SHARED:    true,
	models.ChatSystemAction_CHAT_SYSTEM_ACTION_LOAN_SHARED:        true,
}

// actionFamily returns the coalescing family for action, or "" if the action
// should never be coalesced (creation anchors, one-shot lifecycle events).
// Two messages coalesce only when they share the same non-empty family,
// the same conversation, the same actor, and fall within coalesceWindowSec.
//
// RSVP variants collapse into a single "rsvp" family card.
// Experience need/contribution actions get their own families,
// further narrowed by a per-need or per-contribution coalesce key.
func actionFamily(action models.ChatSystemAction) string {
	if creationAnchorActions[action] {
		return ""
	}
	switch action {
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_MAYBE,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_NO:
		return "rsvp"
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_NEED_ADDED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_NEED_REMOVED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_ADDED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_REMOVED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_UPDATED:
		return "exp_need"
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CONTRIBUTION_ADDED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CONTRIBUTION_REMOVED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_CONTRIBUTION_ADDED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_CONTRIBUTION_REMOVED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_NEED_CLAIMED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_CLAIMED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_OFFERED:
		// One "offer/claim" family so a single help action never stacks
		// multiple lines (#2724): "X offered to help" (OFFERED, keyless)
		// is superseded in place by the richer "X is bringing <need>" claim
		// line, which is in turn superseded by the gear-backed
		// "X is bringing <need> — lending <item>" escalation line
		// (both keyed by the contribution id). See coalesceKeyCompatible for
		// the upgrade-only key matching that makes this safe.
		return "offer_claim"
	}
	return ""
}

// SystemMessageWriter inserts system messages into conversations.
// This is a shared library that can be used by any service that needs
// to add system messages to chat conversations.
// Note: System messages do NOT trigger push notifications - community events handle that.
type SystemMessageWriter struct {
	storage *storage.ProtoSQLStorage
	bus     chat_event_bus.Publisher
}

// NewSystemMessageWriter creates a new writer instance. bus is the chat event
// bus used to fan out inserted messages to active streams and push subscribers;
// pass nil in tests that only care about storage behavior.
func NewSystemMessageWriter(
	storage *storage.ProtoSQLStorage,
	bus chat_event_bus.Publisher,
) *SystemMessageWriter {
	return &SystemMessageWriter{
		storage: storage,
		bus:     bus,
	}
}

// SystemMessageInsertOptions carries optional metadata for system message insertion.
type SystemMessageInsertOptions struct {
	// CoalesceKey narrows coalescing to a specific sub-entity (e.g., experience need ID).
	// When set, two messages only coalesce if they share the same key within the family.
	// Populated by callers for experience need/contribution actions (see #1186).
	CoalesceKey *string

	// PollID ties the system message to a specific time poll so the client can
	// render historical poll messages correctly when a second poll is created.
	// When non-empty, stored on the SystemChatMessage and broadcast in the stream
	// response. Populated by ProposeTime on the first option of each poll.
	PollID string

	// TemplateKey + TemplateParams form the structured-template payload
	// the client uses to render the message in the viewer's locale.
	// When TemplateKey is empty, the client falls back to the literal
	// text passed as the InsertSystemMessage `text` argument. Phase 4b
	// of #1904.
	TemplateKey    string
	TemplateParams map[string]string

	// SentAtUnixSec overrides the message timestamp. When nil, the current
	// clock time is used. Conversation history breaks sent_at_unix_sec ties
	// on a random UUID id, so callers emitting several system messages in
	// the same second (batch need-adds) or a message that must sort after a
	// same-second sibling (a claim after its RSVP line) use this to force a
	// deterministic order — the same pattern as
	// [UserMessageOptions.SentAtUnixSec].
	SentAtUnixSec *int64
}

// InsertLocalized inserts a structured-template system message,
// using the [LocalizedMessage] returned by a `*Message` helper in
// `system_message_templates.go`. The Text field becomes the
// conversation's description; TemplateKey + Params land on
// SystemChatMessage.template_key/template_params so the client
// renders the message in the viewer's locale. Phase 4b of #1904.
//
// Equivalent to [InsertSystemMessage] when the helper provides
// only a Text field (TemplateKey empty), so older non-migrated
// emit sites can switch over incrementally.
func (w *SystemMessageWriter) InsertLocalized(
	ctx context.Context,
	conversationID string,
	actorID string,
	action models.ChatSystemAction,
	msg LocalizedMessage,
	opts ...SystemMessageInsertOptions,
) error {
	_, err := w.InsertLocalizedReturnID(ctx, conversationID, actorID, action, msg, opts...)
	return err
}

// InsertLocalizedReturnID is [InsertLocalized] returning the inserted
// message id (mirrors [InsertSystemMessageReturnID]'s contract for
// undo plumbing).
func (w *SystemMessageWriter) InsertLocalizedReturnID(
	ctx context.Context,
	conversationID string,
	actorID string,
	action models.ChatSystemAction,
	msg LocalizedMessage,
	opts ...SystemMessageInsertOptions,
) (string, error) {
	merged := SystemMessageInsertOptions{}
	if len(opts) > 0 {
		merged = opts[0]
	}
	if msg.TemplateKey != "" {
		merged.TemplateKey = msg.TemplateKey
		merged.TemplateParams = msg.Params
	}
	return w.InsertSystemMessageReturnID(ctx, conversationID, actorID, action, msg.Text, merged)
}

// InsertSystemMessage adds a system message to a conversation and broadcasts to active streams.
// Push notifications are NOT sent for system messages - community events handle notifications instead.
// actorID is the user who triggered the action (stored in sender_id).
// text is the human-readable message to display.
//
// When the action belongs to a coalescing family (see actionFamily), the writer
// first searches for a recent message from the same actor in the same family
// within coalesceWindowSec. On a hit it updates the existing card in place and
// broadcasts a SystemMessageUpdate; on a miss it falls through to a normal insert.
func (w *SystemMessageWriter) InsertSystemMessage(
	ctx context.Context,
	conversationID string,
	actorID string,
	action models.ChatSystemAction,
	text string,
	opts ...SystemMessageInsertOptions,
) error {
	_, err := w.InsertSystemMessageReturnID(ctx, conversationID, actorID, action, text, opts...)
	return err
}

// InsertSystemMessageReturnID is identical to InsertSystemMessage but
// additionally returns the inserted message's id. Used by
// server-authoritative undo flows that need to persist
// system_chat_message_id on the emitted CommunityEvent so the message
// can be soft-deleted on undo via direct read.
//
// When the action coalesces into an existing message, returns the
// existing message's id (not a newly-inserted one).
func (w *SystemMessageWriter) InsertSystemMessageReturnID(
	ctx context.Context,
	conversationID string,
	actorID string,
	action models.ChatSystemAction,
	text string,
	opts ...SystemMessageInsertOptions,
) (string, error) {
	var opt SystemMessageInsertOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	logger := logging.LoggerWithContext(ctx).With(
		"conversation_id", conversationID,
		"actor_id", actorID,
		"action", action.String(),
	)

	if conversationID == "" {
		return "", fmt.Errorf("conversation ID is required")
	}

	// Step 1: Fetch conversation
	conversation := &models.ChatConversation{}
	if err := w.storage.GetByID(ctx, conversationID, conversation); err != nil {
		return "", fmt.Errorf("conversation not found: %w", err)
	}

	now := clock.UnixSec(ctx)
	if opt.SentAtUnixSec != nil {
		now = *opt.SentAtUnixSec
	}

	// Step 2: Attempt coalesce if this action belongs to a family.
	if family := actionFamily(action); family != "" {
		coalescedID, coalesced, err := w.tryCoalesce(ctx, logger, conversation, actorID, action, family, text, now, opt)
		if err != nil {
			// Coalesce lookup failed — fall through to normal insert so we never
			// silently drop a system message due to a transient query error.
			logger.WarnContext(ctx, "coalesce lookup failed, falling back to insert",
				"operation", "CoalesceSystemMessage",
				"error", err,
			)
		} else if coalesced {
			return coalescedID, nil
		}
	}

	// Step 3: Initialize read status — all participants start unread.
	participantReadStatus := make(map[string]bool, len(conversation.ParticipantIds))
	for _, participantID := range conversation.ParticipantIds {
		participantReadStatus[participantID] = false
	}

	// Step 4: Create and insert message.
	sysChatMsg := &models.SystemChatMessage{
		ActorId:     &actorID,
		Action:      action,
		Description: text,
		CoalesceKey: opt.CoalesceKey,
	}
	if opt.PollID != "" {
		sysChatMsg.PollId = &opt.PollID
	}
	if opt.TemplateKey != "" {
		sysChatMsg.TemplateKey = &opt.TemplateKey
		sysChatMsg.TemplateParams = opt.TemplateParams
	}
	message := &models.ChatMessage{
		ConversationId:        conversationID,
		SentAtUnixSec:         now,
		ParticipantIdToIsRead: participantReadStatus,
		Message: &models.ChatMessage_SystemMessage{
			SystemMessage: sysChatMsg,
		},
	}

	messageID, err := w.storage.Insert(ctx, message)
	if err != nil {
		return "", fmt.Errorf("failed to insert system message: %w", err)
	}
	message.Id = messageID

	logger.InfoContext(ctx, "inserted system message", "message_id", messageID)

	// Step 5: Update conversation timestamp.
	conversation.LastMessageAtUnixSec = now
	if err := w.storage.Update(ctx, conversation); err != nil {
		logger.ErrorContext(ctx, "failed to update conversation timestamp", "error", err)
		// Non-fatal - continue with broadcast and notifications
	}

	// Step 6: Publish to the chat bus. Stream and push subscribers handle fan-out.
	// Push notifications are NOT sent for system messages — community events handle them.
	if w.bus != nil {
		if err := w.bus.Publish(ctx, chat_event_bus.KindSystemMessage, message, conversation); err != nil {
			logger.WarnContext(ctx, "chat bus publish failed for system message", "error", err)
		}
	}

	return messageID, nil
}

// coalesceKeyCompatible reports whether a candidate message's coalesce key can
// be superseded by an insert carrying optKey. Matching is upgrade-only:
//
//   - a keyless insert only matches keyless candidates (it must never clobber
//     a richer keyed line, e.g. "X offered to help" arriving after
//     "X is bringing: <need>"),
//   - a keyed insert matches candidates with the same key or with no key
//     (the richer keyed line supersedes the generic keyless one).
func coalesceKeyCompatible(candidateKey, optKey *string) bool {
	if optKey == nil {
		return candidateKey == nil
	}
	return candidateKey == nil || *candidateKey == *optKey
}

// tryCoalesce searches for an existing system message from actorID in the same
// family within coalesceWindowSec, updates it in place, and broadcasts a
// SystemMessageUpdate. Returns (messageID, true, nil) on a successful
// coalesce (the coalesced message's id), ("", false, nil) when no candidate
// was found, or ("", false, err) on lookup failure.
func (w *SystemMessageWriter) tryCoalesce(
	ctx context.Context,
	logger *logging.Logger,
	conversation *models.ChatConversation,
	actorID string,
	action models.ChatSystemAction,
	family string,
	text string,
	now int64,
	opt SystemMessageInsertOptions,
) (string, bool, error) {
	// Single query — no N+1 per enum value. Coalesce-key matching happens in
	// Go (not SQL) so a keyed insert can also supersede a keyless sibling —
	// see coalesceKeyCompatible.
	fields := map[string]any{
		"conversation_id":         conversation.Id,
		"system_message_actor_id": actorID,
	}

	candidates, err := storage.QueryByFields[*models.ChatMessage](
		w.storage, ctx, fields, storage.QueryOptions{Limit: 50},
	)
	if err != nil {
		return "", false, fmt.Errorf("coalesce query failed: %w", err)
	}

	windowStart := now - coalesceWindowSec
	var best *models.ChatMessage
	for _, c := range candidates {
		sys := c.GetSystemMessage()
		if sys == nil {
			continue
		}
		if actionFamily(sys.Action) != family {
			continue
		}
		if !coalesceKeyCompatible(sys.CoalesceKey, opt.CoalesceKey) {
			continue
		}
		if c.SentAtUnixSec < windowStart {
			continue
		}
		if best == nil || c.SentAtUnixSec > best.SentAtUnixSec {
			best = c
		}
	}

	if best == nil {
		logger.DebugContext(ctx, "no coalesce candidate found",
			"operation", "CoalesceSystemMessage",
			"family", family,
		)
		return "", false, nil
	}

	// Update the existing message in place. The structured-template
	// payload must move with Action + Description — otherwise a
	// coalesced RSVP_YES → RSVP_MAYBE keeps the old template_key
	// (and a legacy emit followed by a localized coalesce silently
	// drops the new structured payload).
	sys := best.GetSystemMessage()
	sys.Action = action
	sys.Description = text
	if opt.CoalesceKey != nil {
		// A keyed insert superseding a keyless line (e.g. a need claim
		// replacing "X offered to help") upgrades the row's key so later
		// same-key emits (the gear-backed escalation line) keep coalescing
		// onto it.
		sys.CoalesceKey = opt.CoalesceKey
	}
	if opt.TemplateKey != "" {
		sys.TemplateKey = &opt.TemplateKey
		sys.TemplateParams = opt.TemplateParams
	} else {
		// Coalescing a localized message with a legacy (text-only)
		// emit. Clear the structured payload so the client renders
		// the new literal text instead of a stale template.
		sys.TemplateKey = nil
		sys.TemplateParams = nil
	}
	best.SentAtUnixSec = now

	// Reset read status: actor stays read, everyone else is unread.
	for _, pid := range conversation.ParticipantIds {
		best.ParticipantIdToIsRead[pid] = (pid == actorID)
	}

	if err := w.storage.Update(ctx, best); err != nil {
		return "", false, fmt.Errorf("failed to update coalesced system message: %w", err)
	}

	logger.InfoContext(ctx, "coalesced system message",
		"operation", "CoalesceSystemMessage",
		"message_id", best.Id,
		"family", family,
	)

	// Re-bump conversation timestamp.
	conversation.LastMessageAtUnixSec = now
	if err := w.storage.Update(ctx, conversation); err != nil {
		logger.ErrorContext(ctx, "failed to update conversation timestamp after coalesce", "error", err)
		// Non-fatal.
	}

	// Publish as SystemMessageUpdate so live clients update the card in place.
	if w.bus != nil {
		if err := w.bus.Publish(ctx, chat_event_bus.KindSystemMessageUpdate, best, conversation); err != nil {
			logger.WarnContext(ctx, "chat bus publish failed for system message update",
				"operation", "CoalesceSystemMessage",
				"error", err,
			)
		}
	}

	return best.Id, true, nil
}

// UserMessageOptions carries optional metadata for on-behalf-of user messages.
type UserMessageOptions struct {
	// NeedID is set when the message body is the note on a need.
	NeedID *string
	// NeedName is the display name of the need — stored so the client
	// chip renders without waiting for the needs list to load.
	NeedName *string
	// ContributionID is set when the message body is the description on a contribution.
	ContributionID *string
	// ContributionTitle is the display title of the contribution.
	ContributionTitle *string
	// SentAtUnixSec overrides the message timestamp. When nil, the current
	// clock time is used. Callers that insert a message immediately after a
	// same-second anchor use this to force a deterministic sort order, since
	// conversation history breaks sent_at_unix_sec ties on a random UUID id.
	SentAtUnixSec *int64
}

// InsertUserMessageOnBehalfOf posts a user-authored message into a conversation on behalf
// of the given actor. Unlike SendMessage (which handles the full RPC path including
// mention processing and push notifications), this helper is intended for server-side
// actions where a user's note or description should appear in chat as their own message.
// Push notifications are NOT sent — the experience's existing notification path handles that.
func (w *SystemMessageWriter) InsertUserMessageOnBehalfOf(
	ctx context.Context,
	conversationID string,
	actorID string,
	text string,
	opts UserMessageOptions,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "InsertUserMessageOnBehalfOf",
		"conversation_id", conversationID,
		"actor_id", actorID,
	)

	if conversationID == "" {
		return fmt.Errorf("conversation ID is required")
	}
	if actorID == "" {
		return fmt.Errorf("actor ID is required")
	}
	if text == "" {
		return fmt.Errorf("message text is required")
	}

	// Fetch conversation to get participant list for read-status initialization.
	conversation := &models.ChatConversation{}
	if err := w.storage.GetByID(ctx, conversationID, conversation); err != nil {
		return fmt.Errorf("conversation not found: %w", err)
	}

	now := clock.UnixSec(ctx)
	if opts.SentAtUnixSec != nil {
		now = *opts.SentAtUnixSec
	}
	participantReadStatus := make(map[string]bool, len(conversation.ParticipantIds))
	for _, pid := range conversation.ParticipantIds {
		// Sender's own message is pre-marked read; others are unread.
		participantReadStatus[pid] = (pid == actorID)
	}

	userMsg := &models.UserChatMessage{
		SenderId:          actorID,
		Text:              text,
		NeedId:            opts.NeedID,
		NeedName:          opts.NeedName,
		ContributionId:    opts.ContributionID,
		ContributionTitle: opts.ContributionTitle,
	}

	message := &models.ChatMessage{
		ConversationId:        conversationID,
		SentAtUnixSec:         now,
		ParticipantIdToIsRead: participantReadStatus,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: userMsg,
		},
	}

	messageID, err := w.storage.Insert(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to insert user message on behalf of %s: %w", actorID, err)
	}
	message.Id = messageID

	logger.InfoContext(ctx, "inserted on-behalf-of user message", "message_id", messageID)

	// Update conversation timestamp.
	conversation.LastMessageAtUnixSec = now
	if err := w.storage.Update(ctx, conversation); err != nil {
		logger.WarnContext(ctx, "failed to update conversation timestamp", "error", err)
		// Non-fatal — continue with broadcast.
	}

	// Publish to the chat bus. WithOnBehalfOf suppresses push notifications
	// since the caller's own notification path handles them.
	if w.bus != nil {
		if err := w.bus.Publish(ctx, chat_event_bus.KindUserMessage, message, conversation,
			chat_event_bus.WithOnBehalfOf(true)); err != nil {
			logger.WarnContext(ctx, "chat bus publish failed for on-behalf-of message", "error", err)
		}
	}

	return nil
}

// IsCreationAnchorMessage reports whether a message is a creation anchor —
// a system message posted automatically when an item is first shared with a
// community (e.g., experience created, request submitted, giveaway shared).
// These messages exist to anchor an info card at the top of the conversation
// and should not be counted in comment totals or shown as the most recent comment.
func IsCreationAnchorMessage(msg *models.ChatMessage) bool {
	sys := msg.GetSystemMessage()
	if sys == nil {
		return false
	}
	return creationAnchorActions[sys.Action]
}

// Helper functions to generate display text for each action type.

// JoinedText returns the display text for a join action.
func JoinedText(actorName string) string {
	return fmt.Sprintf("%s joined the conversation", actorName)
}

// RequestedToBorrowText returns the display text for a loan interest expression.
func RequestedToBorrowText(actorName string) string {
	return fmt.Sprintf("%s requested to borrow", actorName)
}

// WithdrewInterestText returns the display text for a giveaway interest withdrawal.
func WithdrewInterestText(actorName string) string {
	return fmt.Sprintf("%s withdrew interest", actorName)
}

// LeftText returns the display text for a leave action.
func LeftText(actorName string) string {
	return fmt.Sprintf("%s left the conversation", actorName)
}

// ApprovedText returns the display text for an approval action.
func ApprovedText(recipientName string) string {
	return fmt.Sprintf("%s was selected as the recipient", recipientName)
}

// DeniedText returns the display text for a denial action.
func DeniedText(userName string) string {
	return fmt.Sprintf("%s was removed from consideration", userName)
}

// CancelledText returns the display text for a cancellation action.
func CancelledText() string {
	return "This request was cancelled"
}

// StartedText returns the display text for a loan start action.
func StartedText() string {
	return "The loan has started"
}

// OfferedText returns the display text for a help offer action.
func OfferedText(actorName string) string {
	return fmt.Sprintf("%s offered to help", actorName)
}

// FulfilledText returns the display text for a request fulfillment action.
func FulfilledText() string {
	return "This request has been fulfilled"
}

// TimeProposedText returns the display text for a time proposal action.
func TimeProposedText(actorName, formattedTime string) string {
	return fmt.Sprintf("%s proposed a new time: %s", actorName, formattedTime)
}

// TimePollOpenedText returns the display text when an organizer opens a time poll.
func TimePollOpenedText(actorName string) string {
	return fmt.Sprintf("%s is asking the group when to meet. Tap to vote.", actorName)
}

// TimeConfirmedText returns the display text when an organizer confirms a time from a poll.
func TimeConfirmedText(actorName, formattedTime string) string {
	return fmt.Sprintf("%s confirmed the time: %s", actorName, formattedTime)
}

// DetailChangedTimeText returns the display text when an organizer updates the event time.
func DetailChangedTimeText(actorName, formattedTime string) string {
	return fmt.Sprintf("%s updated time → %s", actorName, formattedTime)
}

// UndoneText returns the display text for a CHAT_SYSTEM_ACTION_UNDONE
// replacement message. undoneAction is the action being retracted
// (e.g., CHAT_SYSTEM_ACTION_APPROVED); the returned string summarizes
// the retraction legibly for chat viewers.
func UndoneText(actorName string, undoneAction models.ChatSystemAction) string {
	return fmt.Sprintf("%s undid: %s", actorName, undoneActionLabel(undoneAction))
}

// undoneActionLabel returns a short human-readable label for a
// retracted action, suitable for embedding in UndoneText. New
// CHAT_SYSTEM_ACTION_* entries added by future undo RPCs should
// extend this switch.
func undoneActionLabel(action models.ChatSystemAction) string {
	switch action {
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED:
		return "selecting the recipient"
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED:
		return "marking this complete"
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_FULFILLED:
		return "marking this request fulfilled"
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_STARTED:
		return "starting the loan"
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_CANCELLED:
		return "cancelling this"
	default:
		return "the previous action"
	}
}

// DetailChangedLocationText returns the display text when an organizer updates the event location.
func DetailChangedLocationText(actorName, locationName string) string {
	return fmt.Sprintf("%s updated location → %s", actorName, locationName)
}

// TimePollCancelledText returns the display text for a time poll end action.
func TimePollCancelledText(actorName string) string {
	return fmt.Sprintf("%s ended the time poll", actorName)
}

// LocationPollOpenedText returns the display text when an organizer opens a
// location poll.
func LocationPollOpenedText(actorName string) string {
	return fmt.Sprintf("%s is asking the group where to meet. Tap to vote.", actorName)
}

// LocationPollCancelledText returns the display text for a location poll end
// action.
func LocationPollCancelledText(actorName string) string {
	return fmt.Sprintf("%s ended the location poll", actorName)
}

// ExperienceNeedAddedText returns the display text when a participant adds a need.
// If note is non-empty, a truncated snippet is appended after an em dash.
func ExperienceNeedAddedText(actorName, needName string, note *string) string {
	base := fmt.Sprintf("%s added a need: %s", actorName, needName)
	if note != nil && *note != "" {
		return base + " — " + truncateSnippet(*note, 60)
	}
	return base
}

// ExperienceNeedRemovedText returns the display text when a proposer removes a need.
func ExperienceNeedRemovedText(actorName, needName string) string {
	return fmt.Sprintf("%s removed the need: %s", actorName, needName)
}

// ExperienceNeedUpdatedText returns the display text when a proposer edits a
// need's name, note, or slot count in place. Mirrors the added/removed
// helpers' shape so all three sit in the same coalesce family.
func ExperienceNeedUpdatedText(actorName, needName string) string {
	return fmt.Sprintf("%s updated the need: %s", actorName, needName)
}

// ExperienceNeedClaimedText returns the display text when a participant claims a need slot.
// If note is non-empty, a truncated snippet is appended after an em dash.
func ExperienceNeedClaimedText(actorName, needName string, note *string) string {
	base := fmt.Sprintf("%s is bringing: %s", actorName, needName)
	if note != nil && *note != "" {
		return base + " — " + truncateSnippet(*note, 60)
	}
	return base
}

// ExperienceContributionAddedText returns the display text when a participant adds a free-form contribution.
// If description is non-empty, a truncated snippet is appended after an em dash.
func ExperienceContributionAddedText(actorName, title string, description *string) string {
	base := fmt.Sprintf("%s is bringing: %s", actorName, title)
	if description != nil && *description != "" {
		return base + " — " + truncateSnippet(*description, 60)
	}
	return base
}

// ExperienceContributionRemovedText returns the display text when a contributor removes their contribution.
func ExperienceContributionRemovedText(actorName, title string) string {
	return fmt.Sprintf("%s removed their contribution: %s", actorName, title)
}

// truncateSnippet returns up to maxLen characters, appending "…" if truncated.
func truncateSnippet(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}
