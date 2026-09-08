package portfolio

// Shared pre-processing helpers for the portfolio fetch layer: transfer
// deduplication, participant-ID collection, and the terminal-state predicates.
// These outlived the legacy inbox RPCs they were written for (#2830) — GetHomeView
// and the fetch sections still use them.

import (
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// deduplicateAndFilterTransfers merges and filters to non-terminal, non-deleted transfers.
func deduplicateAndFilterTransfers(ownerRaw, recipientRaw []proto.Message) []*models.Transfer {
	seen := make(map[string]bool)
	var result []*models.Transfer
	for _, msgs := range [][]proto.Message{ownerRaw, recipientRaw} {
		for _, m := range msgs {
			t := m.(*models.Transfer)
			if seen[t.Id] {
				continue
			}
			seen[t.Id] = true
			if t.State == models.TransferState_TRANSFER_STATE_COMPLETED ||
				t.State == models.TransferState_TRANSFER_STATE_CANCELLED {
				continue
			}
			if t.Deleted != nil && t.Deleted.DeletedAtUnixSec > 0 {
				continue
			}
			result = append(result, t)
		}
	}
	return result
}

// deduplicateAllTransfers merges owner+recipient transfer lists, deduplicates,
// keeping all non-deleted transfers regardless of state.
func deduplicateAllTransfers(ownerRaw, recipientRaw []proto.Message) []*models.Transfer {
	seen := make(map[string]bool)
	var result []*models.Transfer
	for _, msgs := range [][]proto.Message{ownerRaw, recipientRaw} {
		for _, m := range msgs {
			t := m.(*models.Transfer)
			if seen[t.Id] {
				continue
			}
			seen[t.Id] = true
			if t.Deleted != nil && t.Deleted.DeletedAtUnixSec > 0 {
				continue
			}
			result = append(result, t)
		}
	}
	return result
}

// collectUserIDs gathers all relevant user IDs from items for a single batch fetch.
func collectUserIDs(
	transfers []*models.Transfer,
	expMap map[string]proto.Message,
	activeExpIDs []string,
	expAttendeesMap map[string][]string,
	reqMap map[string]proto.Message,
	activeReqIDs []string,
) []string {
	seen := make(map[string]bool)
	var ids []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, t := range transfers {
		add(t.OwnerId)
		add(t.RecipientId)
	}
	for _, expID := range activeExpIDs {
		if m, ok := expMap[expID]; ok {
			add(m.(*models.Experience).OwnerId)
		}
		for _, uid := range expAttendeesMap[expID] {
			add(uid)
		}
	}
	for _, reqID := range activeReqIDs {
		if m, ok := reqMap[reqID]; ok {
			r := m.(*models.Request)
			add(r.RequesterId)
			for _, hid := range r.ConfirmedHelperIds {
				add(hid)
			}
		}
	}
	return ids
}

// collectCommunityItemOwnerIDs collects owner IDs of non-archived community items.
func collectCommunityItemOwnerIDs(
	gearMap map[string]proto.Message,
	commExpRaw []proto.Message,
	expMap map[string]proto.Message,
	commReqRaw []proto.Message,
	reqMap map[string]proto.Message,
) []string {
	seen := make(map[string]bool)
	var ids []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, m := range gearMap {
		add(m.(*models.Gear).OwnerId)
	}
	for _, m := range commExpRaw {
		ce := m.(*models.CommunityExperience)
		if ce.Archived {
			continue
		}
		if em, ok := expMap[ce.ExperienceId]; ok {
			add(em.(*models.Experience).OwnerId)
		}
	}
	for _, m := range commReqRaw {
		cr := m.(*models.CommunityRequest)
		if cr.Archived {
			continue
		}
		if rm, ok := reqMap[cr.RequestId]; ok {
			add(rm.(*models.Request).RequesterId)
		}
	}
	return ids
}

func isRequestTerminal(s models.RequestState) bool {
	return s == models.RequestState_REQUEST_STATE_FULFILLED ||
		s == models.RequestState_REQUEST_STATE_CANCELLED
}

// transferTerminalAt picks the best-available "wrapped up at" timestamp
// for a terminal transfer. Loans set actual_return_unix_sec on the
// COMPLETED transition; for giveaways and cancelled transfers we fall
// back to latest_request_unix_sec, which tracks the most recent
// state-change-driving event. Returns 0 when no timestamp is available.
func transferTerminalAt(t *models.Transfer) int64 {
	if t.ActualReturnUnixSec != nil && *t.ActualReturnUnixSec > 0 {
		return *t.ActualReturnUnixSec
	}
	if t.LatestRequestUnixSec > 0 {
		return t.LatestRequestUnixSec
	}
	return 0
}

// experienceTerminalAt returns CompletedAtUnixSec when present; otherwise 0.
// Cancelled experiences without a completion timestamp are excluded —
// they have no canonical "wrapped up at" moment.
func experienceTerminalAt(e *models.Experience) int64 {
	if e.CompletedAtUnixSec != nil && *e.CompletedAtUnixSec > 0 {
		return *e.CompletedAtUnixSec
	}
	return 0
}
