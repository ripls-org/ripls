// Package undo owns the generalized-undo feature's central registry,
// which classifies every CommunityEventType by how (or whether) the
// action it records can be reversed.
//
// The registry is the single source of truth behind the enforcement
// tests in registry_test.go. Any new CommunityEventType added to the
// proto without a corresponding entry here will fail
// TestEveryEventTypeClassified on CI — preventing a contributor from
// shipping a new state-changing action without thinking through its
// undo story.
//
// See docs/ai/undo_plan.md §10.6 for the broader design.
package undo

import (
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// Classification identifies how a CommunityEvent can (or cannot) be
// reversed, and which UX surface exposes the undo affordance.
type Classification int

const (
	// ClassificationUnspecified is the zero value and indicates an event
	// type whose undo behavior has not been classified. Any event type
	// mapping to this value fails TestEveryEventTypeClassified.
	ClassificationUnspecified Classification = iota

	// ClassificationClientOnly: an inverse RPC or UI path already exists
	// for this action and covers the need without server retraction. No
	// Undo* RPC, no UndoData, no chat retraction.
	ClassificationClientOnly

	// ClassificationServerSnackbar: reversible via a dedicated Undo* RPC
	// triggered from a transient snackbar. Retracts the system chat
	// message emitted by the forward action and emits a retraction
	// CommunityEvent.
	ClassificationServerSnackbar

	// ClassificationServerStory: reversible via a dedicated Undo* RPC
	// triggered from either a snackbar or a persistent "Undo" link on
	// the story screen the action generates. Same server mechanism as
	// ClassificationServerSnackbar, plus the story-fetch response
	// populates UndoableAction.
	ClassificationServerStory

	// ClassificationRetentionRestore: creation-type action whose inverse
	// is a soft-delete (or its restoration). Uses the entity's existing
	// DeletedMetadata lifecycle plus a "Recently deleted" UI surface —
	// not the Undo* RPC family.
	ClassificationRetentionRestore

	// ClassificationIrreversible: flagged at action time via a
	// confirmation dialog; no undo path.
	ClassificationIrreversible

	// ClassificationInternal: server-emitted event that is not itself a
	// user action (e.g., a retraction event written by an Undo* RPC).
	// Not actionable; exists in the registry only so every enum value
	// has a home.
	ClassificationInternal
)

func (c Classification) String() string {
	switch c {
	case ClassificationClientOnly:
		return "client-only"
	case ClassificationServerSnackbar:
		return "server-snackbar"
	case ClassificationServerStory:
		return "server-story"
	case ClassificationRetentionRestore:
		return "retention-restore"
	case ClassificationIrreversible:
		return "irreversible"
	case ClassificationInternal:
		return "internal"
	default:
		return "unspecified"
	}
}

// Entry records a single CommunityEventType's undo classification plus
// the metadata needed by tests, documentation codegen, and undo RPC
// handlers.
type Entry struct {
	// Classification is the undo category this event belongs to.
	Class Classification

	// UndoRPC is the name of the Undo* RPC that reverses this event,
	// for server-authoritative classifications. Informational —
	// consumed by the docs generator and by
	// TestEveryServerUndoableActionHasReversalDataValidator. Empty for
	// client-only / retention-restore / internal / irreversible.
	//
	// A single event type (e.g., TRANSFER_COMPLETED) may correspond to
	// multiple undo RPCs (UndoCompleteLoan vs. UndoCompleteGiveaway);
	// in that case list them space-separated in dispatch-order. The
	// RPC handler decides which branch to take based on transfer_type.
	UndoRPC string

	// Rationale is a short, human-readable explanation of the
	// classification — one sentence, present on every entry. For
	// client-only entries, explains the natural inverse path. For
	// retention-restore entries, references the soft-delete field and
	// restore mechanism. For server-authoritative entries, references
	// the relevant section of docs/ai/undo_plan.md.
	Rationale string

	// ValidateUndoData is populated once the corresponding Undo* RPC is
	// implemented. Until then it is nil; the registry populator must
	// set it or TestEveryServerUndoableActionHasValidator fails.
	//
	// Returns a descriptive error when the UndoData variant attached
	// to an event of this type is missing required fields for
	// reversal. Called by the undo RPC handler before applying the
	// inverse transition.
	ValidateUndoData func(*models.UndoData) error
}

// Registry maps every CommunityEventType to its undo classification.
// Keep this in enum-value order for easier diff review when new event
// types are added.
//
// Adding a new CommunityEventType without an entry here fails
// TestEveryEventTypeClassified — that's the intended guard rail. If
// the right classification is unclear, default to
// ClassificationClientOnly with a rationale explaining why no
// server-side undo is needed.
var Registry = map[models.CommunityEventType]Entry{
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED: {
		Class:     ClassificationClientOnly,
		Rationale: "Join flow; inverse is 'Leave community'. Welcome story shows no undo link.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is UnshareGear (already an RPC).",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_ITEM_SHARED_WITH_USER: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is removing the member from the item's community (already an RPC). The notification has been sent by then, so an undo could not unsay it.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_UNSHARED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is ShareGear (already an RPC).",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT: {
		Class:     ClassificationClientOnly,
		Rationale: "Within the 30-day rejoin window the natural inverse is RejoinCommunity (no invite required); after the window, rejoin requires a fresh invite. Snackbar-only per plan §3.2.5.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is CancelTransfer from INTEREST_EXPRESSED (already supported). Chat pair JOINED/LEFT is acceptable.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED: {
		Class:     ClassificationServerSnackbar,
		UndoRPC:   "UndoSelectRecipient",
		Rationale: "State-machine loosening + chat retraction; see docs/ai/undo_plan.md §3.2.1.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE: {
		Class:     ClassificationServerSnackbar,
		UndoRPC:   "UndoStartLoan",
		Rationale: "loan_start UndoData variant; reverts gear state. See plan §3.2.1.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED: {
		Class:     ClassificationServerStory,
		UndoRPC:   "UndoCompleteLoan UndoCompleteGiveaway",
		Rationale: "loan_completion or giveaway_cascade UndoData variant depending on transfer_type. See plan §3.2.1.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED: {
		Class:     ClassificationRetentionRestore,
		Rationale: "Soft-delete on Request + existing restore path. See plan §3.2.2.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is WithdrawOffer (already an RPC).",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED: {
		Class:     ClassificationClientOnly,
		Rationale: "Enum defined for client event routing; no server emission today. When the SelectOffer RPC lands, reclassify as server-snackbar.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED: {
		Class:     ClassificationServerStory,
		UndoRPC:   "UndoMarkRequestFulfilled",
		Rationale: "request_fulfillment UndoData variant; clears ImpactEstimate. See plan §3.2.2.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED: {
		Class:     ClassificationServerSnackbar,
		UndoRPC:   "UndoCancelRequest",
		Rationale: "request_cancel UndoData variant. See plan §3.2.2.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_CREATED: {
		Class:     ClassificationRetentionRestore,
		Rationale: "Soft-delete on Community + existing restore path. See plan §3.2.5.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is re-expressing interest. Chat pair JOINED/LEFT is acceptable.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is re-offering via OfferToFulfill.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED: {
		Class:     ClassificationRetentionRestore,
		Rationale: "Soft-delete on Experience + existing restore path. See plan §3.2.3.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED: {
		Class:     ClassificationServerSnackbar,
		UndoRPC:   "UndoCancelTransfer",
		Rationale: "transfer_cancel UndoData variant; may restore gear state if cancel-from-ACTIVE. See plan §3.2.1.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES: {
		Class:     ClassificationClientOnly,
		Rationale: "The RSVP selector on the experience screen is always visible — no undo affordance needed. System-message dedup handles rapid flips.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE: {
		Class:     ClassificationClientOnly,
		Rationale: "See EXPERIENCE_RSVP_YES.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO: {
		Class:     ClassificationClientOnly,
		Rationale: "See EXPERIENCE_RSVP_YES.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED: {
		Class:     ClassificationClientOnly,
		Rationale: "Re-edit the proposed time via the same flow.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is RemoveNeed in the same UI flow.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_REMOVED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is AddNeed in the same UI flow.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_UPDATED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is another UpdateNeed in the same UI flow (NK4 picker re-opens with prior values).",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is UnclaimNeed in the same UI flow.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is RemoveContribution in the same UI flow.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_REMOVED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is AddContribution in the same UI flow.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED: {
		Class:     ClassificationServerStory,
		UndoRPC:   "UndoCompleteExperience",
		Rationale: "experience_completion UndoData variant; clears ImpactEstimate. See plan §3.2.3.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is CompleteExperience or CancelExperience from IN_PROCESS. No UnmarkInProcess RPC; the owner moves the lifecycle forward instead. No server retraction needed.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is creating a new experience via SaveExperience. No UncancelExperience RPC by design — cancellation is intentionally terminal. No server retraction needed.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is editing the experience again via SaveExperience. No server retraction needed.",
	},
	// Retraction events. Emitted by Undo* RPCs; they are themselves
	// not actionable. Listed for enum-coverage completeness only.
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED_UNDONE: {
		Class:     ClassificationInternal,
		Rationale: "Retraction event emitted by UndoSelectRecipient.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE_UNDONE: {
		Class:     ClassificationInternal,
		Rationale: "Retraction event emitted by UndoStartLoan.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE: {
		Class:     ClassificationInternal,
		Rationale: "Retraction event emitted by UndoCompleteLoan / UndoCompleteGiveaway.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED_UNDONE: {
		Class:     ClassificationInternal,
		Rationale: "Retraction event emitted by UndoCancelTransfer.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED_UNDONE: {
		Class:     ClassificationInternal,
		Rationale: "Retraction event; reserved for a future UndoSelectOffer.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED_UNDONE: {
		Class:     ClassificationInternal,
		Rationale: "Retraction event emitted by UndoMarkRequestFulfilled.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED_UNDONE: {
		Class:     ClassificationInternal,
		Rationale: "Retraction event emitted by UndoCancelRequest.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED_UNDONE: {
		Class:     ClassificationInternal,
		Rationale: "Retraction event emitted by UndoCompleteExperience.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED: {
		Class:     ClassificationRetentionRestore,
		Rationale: "30-day soft-delete reversed by RestoreCommunity (not by an Undo* RPC). See docs/community_delete_and_leave.md §6.3.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED: {
		Class:     ClassificationInternal,
		Rationale: "System-emitted by RestoreCommunity; not a user-actionable event.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is the new owner initiating their own ownership transfer. See docs/community_delete_and_leave.md §6.3.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is LeaveCommunity within the same 30-day window.",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED: {
		Class:     ClassificationClientOnly,
		Rationale: "Inverse is editing the name again (or clearing it) via UpdateCommunity; the name field is always editable, so no undo affordance is needed (#2492).",
	},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_ROSTER_CHANGED: {
		Class:     ClassificationInternal,
		Rationale: "Not a user action of its own — a server-emitted, no-push live-refresh signal fanned out so open rosters update when a host resets/removes a member or an RSVP needs to reach a sibling community. The underlying actions (RSVP, reset, remove) carry their own classifications (#2492).",
	},
}

// LookupOrPanic returns the registry entry for an event type and
// panics if none exists. Useful inside undo-RPC handlers that have
// already been guarded by TestEveryEventTypeClassified.
func LookupOrPanic(eventType models.CommunityEventType) Entry {
	entry, ok := Registry[eventType]
	if !ok {
		panic(fmt.Sprintf("undo: no registry entry for CommunityEventType %s — add one in server/undo/registry.go", eventType))
	}
	return entry
}
