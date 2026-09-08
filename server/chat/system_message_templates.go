package chat

import (
	"fmt"
	"strconv"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// LocalizedMessage bundles a system-chat-message's English fallback
// text with the structured template_key + params payload Phase 4b
// clients use to render the message in the viewer's locale. The Text
// field is what the server displays today (and what older clients
// continue to render); the TemplateKey + Params let new clients
// rerender via their ARB catalog.
//
// All system-chat-message text helpers should return this struct
// (one per ChatSystemAction emit pattern). Call sites pass the
// LocalizedMessage to [SystemMessageWriter.InsertLocalized] or
// [SystemMessageWriter.InsertLocalizedReturnID].
type LocalizedMessage struct {
	// Text is the literal English string the server emits. Stored
	// on SystemChatMessage.description and rendered verbatim by
	// pre-Phase-4b clients.
	Text string

	// TemplateKey is the stable identifier the client maps to an
	// ARB key (e.g. "chat.transfer.loan_started" →
	// `serverChatSystemTransferLoanStarted`). Empty when no
	// structured rendering is available (e.g. legacy free-form
	// helpers not yet migrated to this struct).
	TemplateKey string

	// Params provide ICU substitutions for the resolved ARB string.
	// Keys are placeholder names ("actorName", "gearName"); values
	// are already-localized (or round-trip-only) strings that the
	// server never translates.
	Params map[string]string
}

// actorOrSomeone renders an actor's name for the English [LocalizedMessage.Text]
// fallback, standing in "Someone" when the name is unknown.
//
// It is deliberately not applied to Params: an unknown actor reaches the client
// as an empty actorName, and the client renders its own localized stand-in from
// its ARB catalog. Substituting here too would splice an English word into an
// otherwise-translated sentence, which is the render-boundary violation #2844
// closed. Text is the legacy English `description` column (#2159), so an
// English fallback is correct there and only there.
func actorOrSomeone(name string) string {
	if name == "" {
		return "Someone"
	}
	return name
}

// --- Lifecycle helpers (Phase 4b stage 2 migration targets) ---.

// ApprovedMessage is the structured form of the loan/giveaway approval message.
func ApprovedMessage(recipientName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s was selected as the recipient", recipientName),
		TemplateKey: "chat.transfer.approved",
		Params:      map[string]string{"recipientName": recipientName},
	}
}

// CancelledTransferMessage is the structured form of the transfer cancellation message.
func CancelledTransferMessage() LocalizedMessage {
	return LocalizedMessage{
		Text:        "This request was cancelled",
		TemplateKey: "chat.transfer.cancelled",
	}
}

// StartedLoanMessage is the structured form of the loan-started message.
func StartedLoanMessage() LocalizedMessage {
	return LocalizedMessage{
		Text:        "The loan has started",
		TemplateKey: "chat.transfer.loan_started",
	}
}

// HandedOffMessage is the structured form of a loan's completion line.
// "Transfer" is internal jargon banned in user copy (#2724), so the line
// reads as the moment it marks: the item changed hands.
func HandedOffMessage() LocalizedMessage {
	return LocalizedMessage{
		Text:        "Handed off ✓",
		TemplateKey: "chat.transfer.handed_off",
	}
}

// GivenToMessage is the structured form of a giveaway's completion line:
// "Given to <recipient>" instead of the internal "transfer completed" jargon
// (#2724).
func GivenToMessage(recipientName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("Given to %s", recipientName),
		TemplateKey: "chat.transfer.given_to",
		Params:      map[string]string{"recipientName": recipientName},
	}
}

// ExperienceStartedMessage is the structured form of the experience-started message.
func ExperienceStartedMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s started the event", actorOrSomeone(actorName)),
		TemplateKey: "chat.experience.started",
		Params:      map[string]string{"actorName": actorName},
	}
}

// ExperienceCompletedByActorMessage is the structured form of the experience-marked-complete message.
func ExperienceCompletedByActorMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s marked the event complete", actorOrSomeone(actorName)),
		TemplateKey: "chat.experience.completed_by_actor",
		Params:      map[string]string{"actorName": actorName},
	}
}

// ExperienceCancelledMessage is the structured form of the experience cancellation message.
func ExperienceCancelledMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s cancelled the event", actorOrSomeone(actorName)),
		TemplateKey: "chat.experience.cancelled",
		Params:      map[string]string{"actorName": actorName},
	}
}

// RequestFulfilledMessage is the structured form of the request-fulfilled message.
func RequestFulfilledMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s marked the request as fulfilled", actorOrSomeone(actorName)),
		TemplateKey: "chat.request.fulfilled",
		Params:      map[string]string{"actorName": actorName},
	}
}

// RequestCancelledMessage is the structured form of the request-cancellation message.
func RequestCancelledMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s cancelled the request", actorOrSomeone(actorName)),
		TemplateKey: "chat.request.cancelled",
		Params:      map[string]string{"actorName": actorName},
	}
}

// GearOfferedMessage is the structured form of the gear-backed offer message
// posted to the origin (request/experience) conversation when a helper offers
// their actual item toward it ("X is lending Y" / "X is giving Y", #2702).
// Phrased without the possessive — "X will lend their Y" read awkwardly for a
// named person (#2724).
func GearOfferedMessage(actorName, gearName string, giveaway bool) LocalizedMessage {
	if giveaway {
		return LocalizedMessage{
			Text:        fmt.Sprintf("%s is giving %s", actorOrSomeone(actorName), gearName),
			TemplateKey: "chat.request.gear_offer_give",
			Params:      map[string]string{"actorName": actorName, "gearName": gearName},
		}
	}
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s is lending %s", actorOrSomeone(actorName), gearName),
		TemplateKey: "chat.request.gear_offer_lend",
		Params:      map[string]string{"actorName": actorName, "gearName": gearName},
	}
}

// GearOfferedForNeedMessage is the gear-backed offer message when the offer
// escalates a claim on a named need: one line carrying both the need and the
// item ("X is bringing Y — lending Z", #2724). It coalesces over the
// earlier claim/offer lines (same actor, CoalesceKey = contribution id) so a
// single action never stacks multiple system lines.
func GearOfferedForNeedMessage(actorName, needName, gearName string, giveaway bool) LocalizedMessage {
	if giveaway {
		return LocalizedMessage{
			Text:        fmt.Sprintf("%s is bringing %s — giving %s", actorOrSomeone(actorName), needName, gearName),
			TemplateKey: "chat.request.gear_offer_give_for_need",
			Params: map[string]string{
				"actorName": actorName, "needName": needName, "gearName": gearName,
			},
		}
	}
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s is bringing %s — lending %s", actorOrSomeone(actorName), needName, gearName),
		TemplateKey: "chat.request.gear_offer_lend_for_need",
		Params: map[string]string{
			"actorName": actorName, "needName": needName, "gearName": gearName,
		},
	}
}

// OfferAcceptedMessage is the structured form of the requester accepting a
// helper's gear-backed offer ("X accepted Y's offer", #2724) — the
// emotional beat of the request flow, previously silent in chat.
func OfferAcceptedMessage(actorName, helperName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s accepted %s's offer", actorOrSomeone(actorName), helperName),
		TemplateKey: "chat.request.offer_accepted",
		Params:      map[string]string{"actorName": actorName, "helperName": helperName},
	}
}

// RequestFulfilledByHandoffMessage is the structured form of the fulfillment
// message when a gear-backed offer's handoff closed the request (#2702):
// "Fulfilled with X's Y".
func RequestFulfilledByHandoffMessage(helperName, gearName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("Fulfilled with %s's %s", helperName, gearName),
		TemplateKey: "chat.request.fulfilled_by_handoff",
		Params:      map[string]string{"helperName": helperName, "gearName": gearName},
	}
}

// --- Transfer interest helpers ---.

// RequestedToBorrowMessage is the structured form of "X requested to borrow".
func RequestedToBorrowMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s requested to borrow", actorOrSomeone(actorName)),
		TemplateKey: "chat.transfer.requested_to_borrow",
		Params:      map[string]string{"actorName": actorName},
	}
}

// RaisedHandMessage is the structured form of a giveaway interest expression.
// The UI's register for giveaways is "raise a hand" (not the bureaucratic
// "submitted interest", #2724); this replaces SubmittedInterestMessage.
func RaisedHandMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s raised a hand", actorOrSomeone(actorName)),
		TemplateKey: "chat.transfer.raised_hand",
		Params:      map[string]string{"actorName": actorName},
	}
}

// WithdrewInterestMessage is the structured form of "X withdrew interest".
func WithdrewInterestMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s withdrew interest", actorOrSomeone(actorName)),
		TemplateKey: "chat.transfer.withdrew_interest",
		Params:      map[string]string{"actorName": actorName},
	}
}

// --- Undo helpers (varies per undone action) ---.

// UndoneMessage is the structured form of "X undid: …". One template
// key per undone-action variant — the client maps each to a paired
// ARB key like serverChatSystemUndoneApproved, rather than carrying
// an English label as a parameter.
func UndoneMessage(actorName string, undoneAction models.ChatSystemAction) LocalizedMessage {
	suffix, key := undoneActionTemplateMeta(undoneAction)
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s undid: %s", actorOrSomeone(actorName), suffix),
		TemplateKey: key,
		Params:      map[string]string{"actorName": actorName},
	}
}

// undoneActionTemplateMeta returns the English label and the
// template_key for the given undone action.
func undoneActionTemplateMeta(action models.ChatSystemAction) (label, templateKey string) {
	switch action {
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED:
		return "selecting the recipient", "chat.undone.approved"
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED:
		return "marking this complete", "chat.undone.completed"
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_FULFILLED:
		return "marking this request fulfilled", "chat.undone.fulfilled"
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_STARTED:
		return "starting the loan", "chat.undone.started"
	case models.ChatSystemAction_CHAT_SYSTEM_ACTION_CANCELLED:
		return "cancelling this", "chat.undone.cancelled"
	default:
		return "the previous action", "chat.undone.generic"
	}
}

// --- Experience time/location/share/save helpers ---.

// TimePollOpenedMessage is the structured form of the time-poll open message.
func TimePollOpenedMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s is asking the group when to meet. Tap to vote.", actorOrSomeone(actorName)),
		TemplateKey: "chat.experience.time_poll_opened",
		Params:      map[string]string{"actorName": actorName},
	}
}

// TimeParam describes an experience time for a system message. A concrete
// instant travels as UnixSec + UTCOffsetMin and the client renders the
// event-local wall time in the viewer's locale; an informal time travels as
// Description, the organizer's own words, passed through untranslated. TBD
// selects the *_tbd template variant instead.
//
// Nothing here is server-rendered English: the concrete branch used to also
// carry a "Mon, Jan 2 · 3:04 PM" string, which spliced an English timestamp
// into otherwise-translated sentences (#2844).
type TimeParam struct {
	// Description is the organizer's free-text time ("after work sometime"),
	// verbatim user content that is never translated. Empty for concrete
	// instants.
	Description string

	// TBD marks an unset time; the message uses the *_tbd template and
	// carries no time value.
	TBD bool

	// UnixSec is the instant (Unix seconds) for a concrete time; zero
	// otherwise.
	UnixSec int64

	// UTCOffsetMin is the event timezone's UTC offset, in minutes, at
	// UnixSec — enough for a client to reconstruct the event-local wall
	// time without a timezone database.
	UTCOffsetMin int
}

// englishTime renders the time for the English [LocalizedMessage.Text]
// fallback only. Clients render from the structured params instead; see
// [actorOrSomeone] for why the English stays out of Params.
func (t TimeParam) englishTime() string {
	if t.Description != "" {
		return t.Description
	}
	if t.UnixSec == 0 {
		return "TBD"
	}
	wall := time.Unix(t.UnixSec, 0).UTC().Add(time.Duration(t.UTCOffsetMin) * time.Minute)
	return wall.Format("Mon, Jan 2 · 3:04 PM")
}

// params builds the template params for a time-bearing message: the
// structured instant when one exists, or the organizer's own description
// when the time is informal.
func (t TimeParam) params(actorName string) map[string]string {
	p := map[string]string{"actorName": actorName}
	if t.UnixSec != 0 {
		p["timeUnixSec"] = strconv.FormatInt(t.UnixSec, 10)
		p["timeUtcOffsetMin"] = strconv.Itoa(t.UTCOffsetMin)
	}
	if t.Description != "" {
		p["formattedTime"] = t.Description
	}
	return p
}

// TimeConfirmedMessage is the structured form of the time-confirmed message.
func TimeConfirmedMessage(actorName string, t TimeParam) LocalizedMessage {
	if t.TBD {
		return LocalizedMessage{
			Text:        fmt.Sprintf("%s confirmed the time: TBD", actorOrSomeone(actorName)),
			TemplateKey: "chat.experience.time_confirmed_tbd",
			Params:      map[string]string{"actorName": actorName},
		}
	}
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s confirmed the time: %s", actorOrSomeone(actorName), t.englishTime()),
		TemplateKey: "chat.experience.time_confirmed",
		Params:      t.params(actorName),
	}
}

// DetailChangedTimeMessage is the structured form of the time-changed message.
func DetailChangedTimeMessage(actorName string, t TimeParam) LocalizedMessage {
	if t.TBD {
		return LocalizedMessage{
			Text:        fmt.Sprintf("%s updated time → TBD", actorOrSomeone(actorName)),
			TemplateKey: "chat.experience.detail_changed_time_tbd",
			Params:      map[string]string{"actorName": actorName},
		}
	}
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s updated time → %s", actorOrSomeone(actorName), t.englishTime()),
		TemplateKey: "chat.experience.detail_changed_time",
		Params:      t.params(actorName),
	}
}

// TimePollCancelledMessage is the structured form of the time-poll-ended message.
func TimePollCancelledMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s ended the time poll", actorOrSomeone(actorName)),
		TemplateKey: "chat.experience.time_poll_cancelled",
		Params:      map[string]string{"actorName": actorName},
	}
}

// LocationPollOpenedMessage is the structured form of the location-poll open message.
func LocationPollOpenedMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s is asking the group where to meet. Tap to vote.", actorOrSomeone(actorName)),
		TemplateKey: "chat.experience.location_poll_opened",
		Params:      map[string]string{"actorName": actorName},
	}
}

// DetailChangedLocationMessage is the structured form of the location-changed message.
func DetailChangedLocationMessage(actorName, locationName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s updated location → %s", actorOrSomeone(actorName), locationName),
		TemplateKey: "chat.experience.detail_changed_location",
		Params:      map[string]string{"actorName": actorName, "locationName": locationName},
	}
}

// DetailChangedLocationUnnamedMessage is the location-changed message for a
// location with no displayable name — the client renders a localized
// "updated the location" line instead of a server-invented placeholder.
func DetailChangedLocationUnnamedMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s updated the location", actorOrSomeone(actorName)),
		TemplateKey: "chat.experience.detail_changed_location_unnamed",
		Params:      map[string]string{"actorName": actorName},
	}
}

// LocationPollCancelledMessage is the structured form of the location-poll-ended message.
func LocationPollCancelledMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s ended the location poll", actorOrSomeone(actorName)),
		TemplateKey: "chat.experience.location_poll_cancelled",
		Params:      map[string]string{"actorName": actorName},
	}
}

// ExperienceCreatedMessage is the structured form of the experience-created anchor.
func ExperienceCreatedMessage(actorName, eventName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s created an event: %s", actorOrSomeone(actorName), eventName),
		TemplateKey: "chat.experience.created",
		Params:      map[string]string{"actorName": actorName, "eventName": eventName},
	}
}

// ExperienceCompletionSummaryMessage is the user-typed completion summary
// posted to the conversation. The summary itself is author-language content
// (round-trips), so only the frame is localized.
func ExperienceCompletionSummaryMessage(actorName, summary string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s: %s", actorOrSomeone(actorName), summary),
		TemplateKey: "chat.experience.completion_summary",
		Params:      map[string]string{"actorName": actorName, "summary": summary},
	}
}

// --- RSVP helpers ---.

// RSVPYesMessage is the structured form of "X is attending".
func RSVPYesMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s is attending", actorOrSomeone(actorName)),
		TemplateKey: "chat.experience.rsvp_yes",
		Params:      map[string]string{"actorName": actorName},
	}
}

// RSVPMaybeMessage is the structured form of "X might attend".
func RSVPMaybeMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s might attend", actorOrSomeone(actorName)),
		TemplateKey: "chat.experience.rsvp_maybe",
		Params:      map[string]string{"actorName": actorName},
	}
}

// RSVPNoMessage is the structured form of "X declined".
func RSVPNoMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s declined", actorOrSomeone(actorName)),
		TemplateKey: "chat.experience.rsvp_no",
		Params:      map[string]string{"actorName": actorName},
	}
}

// --- Request helpers ---.

// OfferedToHelpMessage is the structured form of "X offered to help".
func OfferedToHelpMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s offered to help", actorOrSomeone(actorName)),
		TemplateKey: "chat.request.offered",
		Params:      map[string]string{"actorName": actorName},
	}
}

// WithdrewOfferMessage is the structured form of "X withdrew their offer".
func WithdrewOfferMessage(actorName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s withdrew their offer", actorOrSomeone(actorName)),
		TemplateKey: "chat.request.withdrew_offer",
		Params:      map[string]string{"actorName": actorName},
	}
}

// RequestCreatedMessage is the structured form of the request-created anchor.
func RequestCreatedMessage(actorName, requestTitle string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s created a request: '%s'", actorOrSomeone(actorName), requestTitle),
		TemplateKey: "chat.request.created",
		Params:      map[string]string{"actorName": actorName, "requestTitle": requestTitle},
	}
}

// --- Community gear-sharing helpers ---.

// GearSharedForGiveawayMessage is the structured form of "X shared {gear} for giveaway".
func GearSharedForGiveawayMessage(actorName, gearName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s shared %s for giveaway", actorOrSomeone(actorName), gearName),
		TemplateKey: "chat.community.gear_shared_giveaway",
		Params:      map[string]string{"actorName": actorName, "gearName": gearName},
	}
}

// GearSharedForLoanMessage is the structured form of "X shared {gear} for loan".
func GearSharedForLoanMessage(actorName, gearName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s shared %s for loan", actorOrSomeone(actorName), gearName),
		TemplateKey: "chat.community.gear_shared_loan",
		Params:      map[string]string{"actorName": actorName, "gearName": gearName},
	}
}

// --- Planning (experience needs/contributions) helpers ---
//
// Needs and contributions can carry an optional note/description that
// the user typed in their own language. The note round-trips like
// gear and request titles — never server-translated. The optional
// snippet shape mirrors the existing chat.ExperienceNeed*Text helpers.

// PlanningNeedAddedMessage is the structured form of "X added a need: …".
// noteSnippet is the truncated user-typed note (may be empty).
func PlanningNeedAddedMessage(actorName, needName, noteSnippet string) LocalizedMessage {
	text := fmt.Sprintf("%s added a need: %s", actorOrSomeone(actorName), needName)
	templateKey := "chat.planning.need_added"
	params := map[string]string{"actorName": actorName, "needName": needName}
	if noteSnippet != "" {
		text += " — " + noteSnippet
		templateKey = "chat.planning.need_added_with_note"
		params["noteSnippet"] = noteSnippet
	}
	return LocalizedMessage{Text: text, TemplateKey: templateKey, Params: params}
}

// PlanningNeedRemovedMessage is the structured form of "X removed the need: …".
func PlanningNeedRemovedMessage(actorName, needName string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s removed the need: %s", actorOrSomeone(actorName), needName),
		TemplateKey: "chat.planning.need_removed",
		Params:      map[string]string{"actorName": actorName, "needName": needName},
	}
}

// PlanningNeedClaimedMessage is the structured form of "X is bringing: …" (claim variant).
func PlanningNeedClaimedMessage(actorName, needName, noteSnippet string) LocalizedMessage {
	text := fmt.Sprintf("%s is bringing: %s", actorOrSomeone(actorName), needName)
	templateKey := "chat.planning.need_claimed"
	params := map[string]string{"actorName": actorName, "needName": needName}
	if noteSnippet != "" {
		text += " — " + noteSnippet
		templateKey = "chat.planning.need_claimed_with_note"
		params["noteSnippet"] = noteSnippet
	}
	return LocalizedMessage{Text: text, TemplateKey: templateKey, Params: params}
}

// PlanningContributionAddedMessage is the structured form of "X is bringing: …" (contribution variant).
func PlanningContributionAddedMessage(actorName, title, descriptionSnippet string) LocalizedMessage {
	text := fmt.Sprintf("%s is bringing: %s", actorOrSomeone(actorName), title)
	templateKey := "chat.planning.contribution_added"
	params := map[string]string{"actorName": actorName, "title": title}
	if descriptionSnippet != "" {
		text += " — " + descriptionSnippet
		templateKey = "chat.planning.contribution_added_with_description"
		params["descriptionSnippet"] = descriptionSnippet
	}
	return LocalizedMessage{Text: text, TemplateKey: templateKey, Params: params}
}

// PlanningContributionRemovedMessage is the structured form of "X removed their contribution: …".
func PlanningContributionRemovedMessage(actorName, title string) LocalizedMessage {
	return LocalizedMessage{
		Text:        fmt.Sprintf("%s removed their contribution: %s", actorOrSomeone(actorName), title),
		TemplateKey: "chat.planning.contribution_removed",
		Params:      map[string]string{"actorName": actorName, "title": title},
	}
}
