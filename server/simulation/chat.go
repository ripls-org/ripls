package simulation

import (
	"context"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// chatContext holds substitution data for expanding chat message templates.
type chatContext struct {
	ItemName        string
	OwnerName       string
	OwnerEmail      string
	BorrowerName    string
	BorrowerEmail   string
	Duration        string
	MentionUserID   string
	MentionUserName string
}

// chatMessage is a resolved message ready to send.
type chatMessage struct {
	SenderEmail string
	Text        string
}

// expandTemplate substitutes placeholders in a chat template string.
func expandTemplate(text string, cc chatContext) string {
	r := strings.NewReplacer(
		"{item_name}", cc.ItemName,
		"{owner_name}", cc.OwnerName,
		"{borrower_name}", cc.BorrowerName,
		"{duration}", cc.Duration,
		"{mention_user_id}", cc.MentionUserID,
		"{mention_user_name}", cc.MentionUserName,
	)
	return r.Replace(text)
}

// buildConversation selects messages from phases and assigns senders.
// senderRoles maps phase name → email of the sender for that phase.
// maxMessages caps the total number of messages returned.
func buildConversation(rng *rand.Rand, phases []ChatPhase, phaseNames []string, cc chatContext, senderRoles map[string]string, maxMessages int) []chatMessage {
	var msgs []chatMessage
	for _, phaseName := range phaseNames {
		if len(msgs) >= maxMessages {
			break
		}
		// Find the phase in the template set.
		var phase *ChatPhase
		for i := range phases {
			if phases[i].Phase == phaseName {
				phase = &phases[i]
				break
			}
		}
		if phase == nil || len(phase.Messages) == 0 {
			continue
		}

		sender := senderRoles[phaseName]
		if sender == "" {
			continue
		}

		// Pick exactly 1 message from this phase. Multiple messages from
		// the same sender in a row reads as duplicated; conversation variety
		// comes from having multiple phases instead.
		idx := rng.Intn(len(phase.Messages))
		text := expandTemplate(phase.Messages[idx].Text, cc)
		msgs = append(msgs, chatMessage{
			SenderEmail: sender,
			Text:        text,
		})
	}
	return msgs
}

// planChatMessageTimes returns a per-message timestamp for the given
// sequence, starting at baseTime and advancing by a realistic 5-20 minute
// interval between messages so they sort in the intended order
// (same-second messages would otherwise sort randomly by UUID in the DB).
//
// If deadline is non-zero, messages whose planned time would land at or
// after it are dropped — the returned slice may be shorter than msgs.
// Callers must check len(returned) before iterating msgs. Issue #1920:
// without this guard, chat sub-flows anchored near a lifecycle step's
// Time could walk past the next sibling step (or scenario.EndTime),
// producing chat messages with sent_at_unix_sec in the future.
func planChatMessageTimes(rng *rand.Rand, msgs []chatMessage, baseTime, deadline time.Time) []time.Time {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]time.Time, 0, len(msgs))
	msgTime := baseTime
	for range msgs {
		if !deadline.IsZero() && !msgTime.Before(deadline) {
			break
		}
		out = append(out, msgTime)
		msgTime = msgTime.Add(time.Duration(5+rng.Intn(16)) * time.Minute)
	}
	return out
}

// sendChatMessages sends a sequence of chat messages to a conversation,
// using getClient to obtain a per-sender client. Returns the number sent.
// Each message gets a simulated timestamp from planChatMessageTimes.
func sendChatMessages(ctx context.Context, getClient func(email string) *Client, rng *rand.Rand, conversationID string, msgs []chatMessage, baseTime, deadline time.Time) int {
	times := planChatMessageTimes(rng, msgs, baseTime, deadline)
	sent := 0
	for i, msg := range msgs {
		if i >= len(times) {
			break
		}
		c := getClient(msg.SenderEmail)
		if c == nil {
			slog.Warn("no client for chat sender",
				"email", msg.SenderEmail,
			)
			continue
		}

		c.SetTimestamp(times[i])
		_, err := c.Chat().SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
			ConversationId: conversationID,
			Text:           msg.Text,
		}))
		if err != nil {
			slog.Warn("failed to send chat message",
				"conversation_id", conversationID,
				"sender", msg.SenderEmail,
				"error", err,
			)
			continue
		}
		sent++
	}
	return sent
}

// firstName extracts the first name from a full "First Last" display name.
func firstName(fullName string) string {
	if i := strings.Index(fullName, " "); i > 0 {
		return fullName[:i]
	}
	return fullName
}

// loanDurationLabel returns a human-readable duration string.
func loanDurationLabel() string {
	return "a week or so"
}

// sendLoanChat sends chat messages for a loan flow. deadline bounds the
// last message's planned timestamp (use the next sibling lifecycle step's
// Time to keep chat tightly packed under it).
func sendLoanChat(ctx context.Context, getClient func(email string) *Client, state *State, rng *rand.Rand, templates *ChatTemplateSet, ref string, phaseNames []string, baseTime, deadline time.Time) {
	convID, ok := state.ConversationIDs[ref]
	if !ok || convID == "" {
		return
	}

	gearRef := extractGearRef(ref)
	tmpl, ok := state.GearTemplates[gearRef]
	if !ok {
		return
	}

	ownerEmail := state.GearOwners[gearRef]
	// Borrower is the first interested user.
	interested := state.InterestedUsers[ref]
	if len(interested) == 0 {
		return
	}
	borrowerEmail := interested[0]

	cc := chatContext{
		ItemName:      tmpl.Name,
		OwnerName:     firstName(state.UserNames[ownerEmail]),
		OwnerEmail:    ownerEmail,
		BorrowerName:  firstName(state.UserNames[borrowerEmail]),
		BorrowerEmail: borrowerEmail,
		Duration:      loanDurationLabel(),
	}

	senderRoles := map[string]string{
		"interest":          borrowerEmail,
		"owner_reply":       ownerEmail,
		"logistics":         borrowerEmail,
		"logistics_confirm": ownerEmail,
		"thanks":            borrowerEmail,
	}

	msgs := buildConversation(rng, templates.Loan, phaseNames, cc, senderRoles, 6)
	sent := sendChatMessages(ctx, getClient, rng, convID, msgs, baseTime, deadline)
	state.ChatMessagesSent += sent
}

// sendGiveawayChat sends chat messages for a giveaway flow. deadline bounds
// the last message's planned timestamp.
func sendGiveawayChat(ctx context.Context, getClient func(email string) *Client, state *State, rng *rand.Rand, templates *ChatTemplateSet, ref string, phaseNames []string, baseTime, deadline time.Time) {
	convID, ok := state.ConversationIDs[ref]
	if !ok || convID == "" {
		return
	}

	gearRef := extractGearRef(ref)
	tmpl, ok := state.GearTemplates[gearRef]
	if !ok {
		return
	}

	ownerEmail := state.GearOwners[gearRef]
	interested := state.InterestedUsers[ref]
	if len(interested) == 0 {
		return
	}
	recipientEmail := interested[0]

	cc := chatContext{
		ItemName:      tmpl.Name,
		OwnerName:     firstName(state.UserNames[ownerEmail]),
		OwnerEmail:    ownerEmail,
		BorrowerName:  firstName(state.UserNames[recipientEmail]),
		BorrowerEmail: recipientEmail,
	}

	senderRoles := map[string]string{
		"interest":    recipientEmail,
		"owner_reply": ownerEmail,
		"logistics":   recipientEmail,
		"thanks":      recipientEmail,
	}

	msgs := buildConversation(rng, templates.Giveaway, phaseNames, cc, senderRoles, 6)
	sent := sendChatMessages(ctx, getClient, rng, convID, msgs, baseTime, deadline)
	state.ChatMessagesSent += sent
}

// sendRequestChat sends chat messages for a request flow. deadline bounds the
// last message's planned timestamp.
func sendRequestChat(ctx context.Context, getClient func(email string) *Client, state *State, rng *rand.Rand, templates *ChatTemplateSet, ref, requesterEmail string, phaseNames []string, baseTime, deadline time.Time) {
	convID, ok := state.ConversationIDs[ref]
	if !ok || convID == "" {
		return
	}

	// Helper is the first user who offered.
	helpers := state.HelperUsers[ref]
	var helperEmail string
	if len(helpers) > 0 {
		helperEmail = helpers[0]
	}

	cc := chatContext{
		OwnerName:    firstName(state.UserNames[requesterEmail]),
		OwnerEmail:   requesterEmail,
		BorrowerName: firstName(state.UserNames[helperEmail]),
	}

	senderRoles := map[string]string{
		"clarification":    helperEmail,
		"requester_reply":  requesterEmail,
		"offer":            helperEmail,
		"planning":         requesterEmail,
		"requester_thanks": requesterEmail,
	}

	msgs := buildConversation(rng, templates.Request, phaseNames, cc, senderRoles, 6)
	sent := sendChatMessages(ctx, getClient, rng, convID, msgs, baseTime, deadline)
	state.ChatMessagesSent += sent
}

// sendExperienceChat sends chat messages for an experience flow. deadline
// bounds the last message's planned timestamp.
func sendExperienceChat(ctx context.Context, getClient func(email string) *Client, state *State, rng *rand.Rand, templates *ChatTemplateSet, ref, hostEmail, attendeeEmail, communityName string, phaseNames []string, baseTime, deadline time.Time) {
	convID, ok := state.ConversationIDs[ref]
	if !ok || convID == "" {
		return
	}

	cc := chatContext{
		OwnerName:  firstName(state.UserNames[hostEmail]),
		OwnerEmail: hostEmail,
	}

	// For mention_suggestion, pick a random community member who isn't the
	// attendee or host.
	if members, ok := state.CommunityMembers[communityName]; ok && len(members) > 2 {
		for _, tries := range rng.Perm(len(members)) {
			email := members[tries]
			if email != hostEmail && email != attendeeEmail {
				cc.MentionUserID = state.UserIDs[email]
				cc.MentionUserName = firstName(state.UserNames[email])
				break
			}
		}
	}

	senderRoles := map[string]string{
		"rsvp_excitement":    attendeeEmail,
		"question":           attendeeEmail,
		"host_reply":         hostEmail,
		"mention_suggestion": attendeeEmail,
	}

	msgs := buildConversation(rng, templates.Experience, phaseNames, cc, senderRoles, 4)
	sent := sendChatMessages(ctx, getClient, rng, convID, msgs, baseTime, deadline)
	state.ChatMessagesSent += sent
}
