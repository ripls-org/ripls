package transfer

import (
	"context"
	"sort"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
	undotesting "go.ripls.org/ripls/server/undo/testing"
)

// TestUndoSelectRecipient_RoundTrip asserts that undoing a recipient
// selection restores the full composite snapshot (transfer + chat
// messages) to byte-equal pre-action state. Load-bearing correctness
// invariant per docs/ai/undo_plan.md §10.5.
func TestUndoSelectRecipient_RoundTrip(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()

	// Recipient expresses interest so the transfer reaches INTEREST_EXPRESSED,
	// which is the pre-action state SelectRecipient mutates.
	recipCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(recipCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, _ := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	transferID := transfers[0].(*models.Transfer).Id
	gearConvID, err := service.getGearConversationID(ctx, gearID, transfers[0].(*models.Transfer).CommunityId)
	if err != nil {
		t.Fatalf("get gear conversation: %v", err)
	}

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	preActionChatIDs := chatMessageIDs(t, testStorage, gearConvID)

	undotesting.RoundTrip(t, service, undotesting.Case[*Service]{
		Name: "select-recipient",
		SnapshotBefore: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotTransferAndChat(t, testStorage, transferID, gearConvID, preActionChatIDs)
		},
		Mutate: func(t *testing.T, _ *Service) string {
			resp, err := service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
				TransferId:  transferID,
				RecipientId: recipientID,
			}))
			if err != nil {
				t.Fatalf("SelectRecipient failed: %v", err)
			}
			services.WaitForNotification(t, done)
			return resp.Msg.CommunityEventId
		},
		SnapshotAfter: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotTransferAndChat(t, testStorage, transferID, gearConvID, preActionChatIDs)
		},
		Undo: func(t *testing.T, _ *Service, communityEventID string) {
			if _, err := service.UndoSelectRecipient(ownerCtx, connect.NewRequest(&api.UndoSelectRecipientRequest{
				CommunityEventId: communityEventID,
			})); err != nil {
				t.Fatalf("UndoSelectRecipient failed: %v", err)
			}
		},
	})
}

// TestUndoCompleteLoan_RoundTrip asserts that undoing a loan
// completion restores transfer + gear + chat to byte-equal pre-action
// state.
func TestUndoCompleteLoan_RoundTrip(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()

	borrowerCtx := createAuthenticatedContext(borrowerID, "borrower@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(borrowerCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, _ := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	transfer := transfers[0].(*models.Transfer)
	transferID := transfer.Id
	gearConvID, err := service.getGearConversationID(ctx, gearID, transfer.CommunityId)
	if err != nil {
		t.Fatalf("get gear conversation: %v", err)
	}

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	if _, err := service.StartLoan(ownerCtx, connect.NewRequest(&api.StartLoanRequest{TransferId: transferID})); err != nil {
		t.Fatalf("StartLoan failed: %v", err)
	}
	preActionChatIDs := chatMessageIDs(t, testStorage, gearConvID)

	// Transfer is now ACTIVE — that's the state CompleteTransfer mutates
	// from and Undo must restore.
	undotesting.RoundTrip(t, service, undotesting.Case[*Service]{
		Name: "complete-loan",
		SnapshotBefore: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotLoanComposite(t, testStorage, transferID, gearID, gearConvID, preActionChatIDs)
		},
		Mutate: func(t *testing.T, _ *Service) string {
			resp, err := service.CompleteTransfer(ownerCtx, connect.NewRequest(&api.CompleteTransferRequest{
				TransferId: transferID,
			}))
			if err != nil {
				t.Fatalf("CompleteTransfer failed: %v", err)
			}
			return resp.Msg.CommunityEventId
		},
		SnapshotAfter: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotLoanComposite(t, testStorage, transferID, gearID, gearConvID, preActionChatIDs)
		},
		Undo: func(t *testing.T, _ *Service, communityEventID string) {
			if _, err := service.UndoCompleteLoan(ownerCtx, connect.NewRequest(&api.UndoCompleteLoanRequest{
				CommunityEventId: communityEventID,
			})); err != nil {
				t.Fatalf("UndoCompleteLoan failed: %v", err)
			}
		},
	})
}

// TestUndoStartLoan_RoundTrip asserts undoing a start-loan restores
// transfer + gear + pre-action chat.
func TestUndoStartLoan_RoundTrip(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	borrowerCtx := createAuthenticatedContext(borrowerID, "borrower@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(borrowerCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, _ := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	transfer := transfers[0].(*models.Transfer)
	transferID := transfer.Id
	gearConvID, err := service.getGearConversationID(ctx, gearID, transfer.CommunityId)
	if err != nil {
		t.Fatalf("get gear conversation: %v", err)
	}

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	preActionChatIDs := chatMessageIDs(t, testStorage, gearConvID)

	undotesting.RoundTrip(t, service, undotesting.Case[*Service]{
		Name: "start-loan",
		SnapshotBefore: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotLoanComposite(t, testStorage, transferID, gearID, gearConvID, preActionChatIDs)
		},
		Mutate: func(t *testing.T, _ *Service) string {
			resp, err := service.StartLoan(ownerCtx, connect.NewRequest(&api.StartLoanRequest{TransferId: transferID}))
			if err != nil {
				t.Fatalf("StartLoan failed: %v", err)
			}
			return resp.Msg.CommunityEventId
		},
		SnapshotAfter: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotLoanComposite(t, testStorage, transferID, gearID, gearConvID, preActionChatIDs)
		},
		Undo: func(t *testing.T, _ *Service, communityEventID string) {
			if _, err := service.UndoStartLoan(ownerCtx, connect.NewRequest(&api.UndoStartLoanRequest{
				CommunityEventId: communityEventID,
			})); err != nil {
				t.Fatalf("UndoStartLoan failed: %v", err)
			}
		},
	})
}

// TestUndoCancelTransfer_RoundTrip asserts undoing a cancel restores
// transfer + gear + pre-action chat. Tests the cancel-from-ACTIVE case
// (loan in progress) since that's the cascade path with gear state
// restoration.
func TestUndoCancelTransfer_RoundTrip(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	borrowerCtx := createAuthenticatedContext(borrowerID, "borrower@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(borrowerCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, _ := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	transfer := transfers[0].(*models.Transfer)
	transferID := transfer.Id
	gearConvID, err := service.getGearConversationID(ctx, gearID, transfer.CommunityId)
	if err != nil {
		t.Fatalf("get gear conversation: %v", err)
	}

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	if _, err := service.StartLoan(ownerCtx, connect.NewRequest(&api.StartLoanRequest{TransferId: transferID})); err != nil {
		t.Fatalf("StartLoan failed: %v", err)
	}
	// Transfer is now ACTIVE with gear UNAVAILABLE — the pre-action
	// state Undo must restore.
	preActionChatIDs := chatMessageIDs(t, testStorage, gearConvID)

	undotesting.RoundTrip(t, service, undotesting.Case[*Service]{
		Name: "cancel-transfer-from-active",
		SnapshotBefore: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotLoanComposite(t, testStorage, transferID, gearID, gearConvID, preActionChatIDs)
		},
		Mutate: func(t *testing.T, _ *Service) string {
			resp, err := service.CancelTransfer(ownerCtx, connect.NewRequest(&api.CancelTransferRequest{TransferId: transferID}))
			if err != nil {
				t.Fatalf("CancelTransfer failed: %v", err)
			}
			return resp.Msg.CommunityEventId
		},
		SnapshotAfter: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotLoanComposite(t, testStorage, transferID, gearID, gearConvID, preActionChatIDs)
		},
		Undo: func(t *testing.T, _ *Service, communityEventID string) {
			if _, err := service.UndoCancelTransfer(ownerCtx, connect.NewRequest(&api.UndoCancelTransferRequest{
				CommunityEventId: communityEventID,
			})); err != nil {
				t.Fatalf("UndoCancelTransfer failed: %v", err)
			}
		},
	})
}

// TestUndoCompleteGiveaway_RoundTrip asserts that undoing a giveaway
// completion restores the full cascade — primary transfer, gear,
// every CommunityGear relationship for the gear, and every sibling
// transfer auto-cancelled by the cascade — to byte-equal pre-action
// state.
func TestUndoCompleteGiveaway_RoundTrip(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")
	sibling1ID := setupTestUser(t, testStorage, "Sibling1", "sibling1@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()

	// Add sibling as community member and have them express interest too,
	// so completion auto-cancels their transfer (covering the cascade).
	siblingMembership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      sibling1ID,
		InviterId:   ownerID,
	}
	if _, err := testStorage.Insert(ctx, siblingMembership); err != nil {
		t.Fatalf("add sibling membership: %v", err)
	}

	// Recipient expresses interest — creates the transfer we'll complete.
	recipCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(recipCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest (recipient) failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Sibling also expresses interest — creates the sibling transfer
	// that completion will auto-cancel.
	siblingCtx := createAuthenticatedContext(sibling1ID, "sibling1@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(siblingCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest (sibling) failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Owner selects recipient — transfers to RECIPIENT_SELECTED state.
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	transfers, _ := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"recipient_id": recipientID,
	}, &models.Transfer{})
	if len(transfers) == 0 {
		t.Fatal("recipient transfer not found")
	}
	transferID := transfers[0].(*models.Transfer).Id

	if _, err := service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transferID,
		RecipientId: recipientID,
	})); err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done)

	gearConvID, err := service.getGearConversationID(ctx, gearID, communityID)
	if err != nil {
		t.Fatalf("get gear conversation: %v", err)
	}
	preActionChatIDs := chatMessageIDs(t, testStorage, gearConvID)

	// State right before CompleteTransfer: primary in RECIPIENT_SELECTED,
	// sibling in INTEREST_EXPRESSED, gear AVAILABLE, community_gear
	// un-archived. That's what undo must restore.
	undotesting.RoundTrip(t, service, undotesting.Case[*Service]{
		Name: "complete-giveaway",
		SnapshotBefore: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotGiveawayComposite(t, testStorage, gearID, gearConvID, preActionChatIDs)
		},
		Mutate: func(t *testing.T, _ *Service) string {
			resp, err := service.CompleteTransfer(ownerCtx, connect.NewRequest(&api.CompleteTransferRequest{
				TransferId: transferID,
			}))
			if err != nil {
				t.Fatalf("CompleteTransfer failed: %v", err)
			}
			return resp.Msg.CommunityEventId
		},
		SnapshotAfter: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotGiveawayComposite(t, testStorage, gearID, gearConvID, preActionChatIDs)
		},
		Undo: func(t *testing.T, _ *Service, communityEventID string) {
			if _, err := service.UndoCompleteGiveaway(ownerCtx, connect.NewRequest(&api.UndoCompleteGiveawayRequest{
				CommunityEventId: communityEventID,
			})); err != nil {
				t.Fatalf("UndoCompleteGiveaway failed: %v", err)
			}
		},
	})
}

// snapshotGiveawayComposite captures everything CompleteTransfer on a
// giveaway mutates: gear, every transfer for the gear (primary +
// siblings), every community_gear row for the gear, plus pre-action
// chat messages.
func snapshotGiveawayComposite(t *testing.T, s *storage.ProtoSQLStorage, gearID, conversationID string, preActionChatIDs []string) undotesting.Snapshot {
	t.Helper()
	ctx := context.Background()

	gear := &models.Gear{}
	if err := s.GetByID(ctx, gearID, gear); err != nil {
		t.Fatalf("snapshot: load gear: %v", err)
	}

	entries := []undotesting.SnapshotEntry{
		{Label: "gear", Message: gear},
	}

	transfers, err := s.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("snapshot: query transfers: %v", err)
	}
	transferModels := make([]*models.Transfer, 0, len(transfers))
	for _, tr := range transfers {
		transferModels = append(transferModels, tr.(*models.Transfer))
	}
	sort.Slice(transferModels, func(i, j int) bool {
		return transferModels[i].Id < transferModels[j].Id
	})
	for _, m := range transferModels {
		entries = append(entries, undotesting.SnapshotEntry{
			Label:   "transfer:" + m.Id,
			Message: m,
		})
	}

	communityGears, err := s.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
	if err != nil {
		t.Fatalf("snapshot: query community_gears: %v", err)
	}
	cgModels := make([]*models.CommunityGear, 0, len(communityGears))
	for _, cg := range communityGears {
		cgModels = append(cgModels, cg.(*models.CommunityGear))
	}
	sort.Slice(cgModels, func(i, j int) bool {
		return cgModels[i].Id < cgModels[j].Id
	})
	for _, m := range cgModels {
		entries = append(entries, undotesting.SnapshotEntry{
			Label:   "community_gear:" + m.Id,
			Message: m,
		})
	}

	for _, msgID := range preActionChatIDs {
		msg := &models.ChatMessage{}
		if err := s.GetByID(ctx, msgID, msg); err != nil {
			entries = append(entries, undotesting.SnapshotEntry{
				Label:   "chat:" + msgID + " (missing)",
				Message: &models.ChatMessage{Id: msgID},
			})
			continue
		}
		entries = append(entries, undotesting.SnapshotEntry{
			Label:   "chat:" + msg.Id,
			Message: msg,
		})
	}
	return undotesting.Snapshot{Entries: entries}
}

// snapshotTransferAndChat loads the transfer plus every chat message
// that existed in the gear conversation BEFORE the action. Used for
// UndoSelectRecipient round-trip.
//
// Chat is snapshotted as a closed set keyed by id — we assert only
// that every message present pre-action is still present and
// byte-equal post-undo. We do not snapshot the conversation as an
// open list because the CHAT_SYSTEM_ACTION_UNDONE retraction is a net
// append and therefore breaks strict list equality by design.
func snapshotTransferAndChat(t *testing.T, s *storage.ProtoSQLStorage, transferID, conversationID string, preActionChatIDs []string) undotesting.Snapshot {
	t.Helper()
	ctx := context.Background()

	transfer := &models.Transfer{}
	if err := s.GetByID(ctx, transferID, transfer); err != nil {
		t.Fatalf("snapshot: load transfer: %v", err)
	}
	entries := []undotesting.SnapshotEntry{
		{Label: "transfer", Message: transfer},
	}
	for _, msgID := range preActionChatIDs {
		msg := &models.ChatMessage{}
		if err := s.GetByID(ctx, msgID, msg); err != nil {
			// Soft-deleted messages are filtered by GetByID (storage
			// layer's deleted filter). A missing message here means
			// undo failed to restore a message that was present
			// pre-action — register as an entry with a nil message so
			// the diff shows the label clearly.
			entries = append(entries, undotesting.SnapshotEntry{
				Label:   "chat:" + msgID + " (missing)",
				Message: &models.ChatMessage{Id: msgID},
			})
			continue
		}
		entries = append(entries, undotesting.SnapshotEntry{
			Label:   "chat:" + msg.Id,
			Message: msg,
		})
	}
	return undotesting.Snapshot{Entries: entries}
}

// snapshotLoanComposite loads transfer + gear + pre-action chat
// messages for a loan. Used for UndoCompleteLoan round-trip.
func snapshotLoanComposite(t *testing.T, s *storage.ProtoSQLStorage, transferID, gearID, conversationID string, preActionChatIDs []string) undotesting.Snapshot {
	t.Helper()
	ctx := context.Background()

	transfer := &models.Transfer{}
	if err := s.GetByID(ctx, transferID, transfer); err != nil {
		t.Fatalf("snapshot: load transfer: %v", err)
	}
	gear := &models.Gear{}
	if err := s.GetByID(ctx, gearID, gear); err != nil {
		t.Fatalf("snapshot: load gear: %v", err)
	}

	entries := []undotesting.SnapshotEntry{
		{Label: "transfer", Message: transfer},
		{Label: "gear", Message: gear},
	}
	for _, msgID := range preActionChatIDs {
		msg := &models.ChatMessage{}
		if err := s.GetByID(ctx, msgID, msg); err != nil {
			entries = append(entries, undotesting.SnapshotEntry{
				Label:   "chat:" + msgID + " (missing)",
				Message: &models.ChatMessage{Id: msgID},
			})
			continue
		}
		entries = append(entries, undotesting.SnapshotEntry{
			Label:   "chat:" + msg.Id,
			Message: msg,
		})
	}
	return undotesting.Snapshot{Entries: entries}
}

// chatMessageIDs returns the ids of every non-deleted chat message in
// a conversation, in no particular order — used to record the
// "pre-action set" that a round-trip's post-undo snapshot must
// still contain.
func chatMessageIDs(t *testing.T, s *storage.ProtoSQLStorage, conversationID string) []string {
	t.Helper()
	msgs, err := s.QueryByField(context.Background(), "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("chat message ids: query: %v", err)
	}
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.(*models.ChatMessage).Id)
	}
	return ids
}
