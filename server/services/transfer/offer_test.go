package transfer

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// offerFixture is the world an OfferTransfer test starts from: a helper who
// owns available gear, a requester with a live request, and one community both
// belong to with the request shared into it.
type offerFixture struct {
	service        *Service
	storage        *storage.ProtoSQLStorage
	mockNotif      *notifications.MockService
	done           chan struct{}
	ownerID        string
	askerID        string
	gearID         string
	communityID    string
	requestID      string
	contributionID string
	sharedWith     []models.Availability // availabilities the fake gear sharer was called with
}

func setupOfferFixture(t *testing.T) *offerFixture {
	t.Helper()
	service, testStorage, mockNotif, done := setupTestServiceWithNotifications(t)
	ctx := context.Background()

	f := &offerFixture{service: service, storage: testStorage, mockNotif: mockNotif, done: done}
	f.ownerID = setupTestUser(t, testStorage, "Theo", "theo@example.com")
	f.askerID = setupTestUser(t, testStorage, "June", "june@example.com")
	f.gearID = setupTestGear(t, testStorage, f.ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	f.communityID = setupCommunityAndShareGear(t, testStorage, f.ownerID, f.askerID, f.gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	request := &models.Request{
		RequesterId: f.askerID,
		Title:       "Looking for a lawn mower",
		Description: "Ours gave up mid-mow",
		State:       models.RequestState_REQUEST_STATE_OFFERS_RECEIVED,
	}
	requestID, err := testStorage.Insert(ctx, request)
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	f.requestID = requestID
	if _, err := testStorage.Insert(ctx, &models.CommunityRequest{
		RequestId:   requestID,
		CommunityId: f.communityID,
		Archived:    false,
	}); err != nil {
		t.Fatalf("insert community request: %v", err)
	}

	gearID := f.gearID
	contribution := &models.PlanningContribution{
		ContributorId: f.ownerID,
		Title:         "Lawn mower",
		Scope:         &models.PlanningContribution_RequestId{RequestId: requestID},
		GearId:        &gearID,
	}
	contributionID, err := testStorage.Insert(ctx, contribution)
	if err != nil {
		t.Fatalf("insert contribution: %v", err)
	}
	f.contributionID = contributionID

	// Fake gear sharer: records the availability and upserts the junction the
	// way the community service's shareGearToCommunity would.
	service.SetGearSharer(func(ctx context.Context, gearID, communityID, actorUserID string, availability models.Availability) error {
		f.sharedWith = append(f.sharedWith, availability)
		existing, err := storage.QueryByFields[*models.CommunityGear](testStorage, ctx, map[string]any{
			"gear_id":      gearID,
			"community_id": communityID,
		})
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			existing[0].Availability = availability
			return testStorage.Update(ctx, existing[0])
		}
		_, err = testStorage.Insert(ctx, &models.CommunityGear{
			GearId:       gearID,
			CommunityId:  communityID,
			Availability: availability,
		})
		return err
	})
	return f
}

func (f *offerFixture) offerRequest(transferType api.TransferType) *api.OfferTransferRequest {
	return &api.OfferTransferRequest{
		GearId:          f.gearID,
		TransferType:    transferType,
		RecipientUserId: f.askerID,
		CommunityId:     f.communityID,
		OriginRequestId: f.requestID,
		ContributionId:  f.contributionID,
	}
}

func (f *offerFixture) ownerCtx() context.Context {
	return createAuthenticatedContext(f.ownerID, "theo@example.com", models.Role_ROLE_USER)
}

func TestOfferTransfer_Loan_HappyPath(t *testing.T) {
	f := setupOfferFixture(t)

	resp, err := f.service.OfferTransfer(f.ownerCtx(), connect.NewRequest(f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN)))
	if err != nil {
		t.Fatalf("OfferTransfer failed: %v", err)
	}

	tr := resp.Msg.Transfer
	if tr.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Errorf("expected RECIPIENT_SELECTED, got %s", tr.State)
	}
	if tr.GetOriginRequestId() != f.requestID {
		t.Errorf("expected origin_request_id %s, got %q", f.requestID, tr.GetOriginRequestId())
	}
	if tr.Recipient == nil || tr.Recipient.Id != f.askerID {
		t.Errorf("expected recipient %s, got %+v", f.askerID, tr.Recipient)
	}

	// The contribution now carries the transfer link.
	contribution := &models.PlanningContribution{}
	if err := f.storage.GetByID(context.Background(), f.contributionID, contribution); err != nil {
		t.Fatalf("reload contribution: %v", err)
	}
	if contribution.GetTransferId() != tr.Id {
		t.Errorf("expected contribution.transfer_id %s, got %q", tr.Id, contribution.GetTransferId())
	}

	// The gear was shared with the loan availability.
	if len(f.sharedWith) != 1 || f.sharedWith[0] != models.Availability_AVAILABILITY_FOR_LOAN {
		t.Errorf("expected one FOR_LOAN share, got %v", f.sharedWith)
	}

	// The owner's transfer list carries the origin.
	listResp, err := f.service.ListMyTransfers(f.ownerCtx(), connect.NewRequest(&api.ListMyTransfersRequest{}))
	if err != nil {
		t.Fatalf("ListMyTransfers failed: %v", err)
	}
	found := false
	for _, lt := range listResp.Msg.Transfers {
		if lt.Id == tr.Id && lt.GetOriginRequestId() == f.requestID {
			found = true
		}
	}
	if !found {
		t.Error("expected the offer transfer with origin in ListMyTransfers")
	}
}

func TestOfferTransfer_Giveaway_SharesForGiveaway(t *testing.T) {
	f := setupOfferFixture(t)

	resp, err := f.service.OfferTransfer(f.ownerCtx(), connect.NewRequest(f.offerRequest(api.TransferType_TRANSFER_TYPE_GIVEAWAY)))
	if err != nil {
		t.Fatalf("OfferTransfer failed: %v", err)
	}
	if resp.Msg.Transfer.TransferType != api.TransferType_TRANSFER_TYPE_GIVEAWAY {
		t.Errorf("expected GIVEAWAY, got %s", resp.Msg.Transfer.TransferType)
	}
	// Toggle wins: the share carries the giveaway availability even though the
	// gear was previously shared FOR_LOAN in this community.
	if len(f.sharedWith) != 1 || f.sharedWith[0] != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
		t.Errorf("expected one FOR_GIVEAWAY share, got %v", f.sharedWith)
	}
}

func TestOfferTransfer_RecipientSelectedPushSuppressed(t *testing.T) {
	f := setupOfferFixture(t)

	if _, err := f.service.OfferTransfer(f.ownerCtx(), connect.NewRequest(f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN))); err != nil {
		t.Fatalf("OfferTransfer failed: %v", err)
	}
	// Wait for the bus to dispatch the RECIPIENT_SELECTED event.
	<-f.done

	for _, call := range f.mockNotif.GetCalls() {
		if call.UserID == f.askerID {
			t.Errorf("expected no push to the requester for an origin-linked RECIPIENT_SELECTED, got %+v", call)
		}
	}
}

func TestOfferTransfer_Validation(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(t *testing.T, f *offerFixture, req *api.OfferTransferRequest) context.Context
		wantCode connect.Code
	}{
		{
			name: "non-owner caller",
			mutate: func(t *testing.T, f *offerFixture, req *api.OfferTransferRequest) context.Context {
				return createAuthenticatedContext(f.askerID, "june@example.com", models.Role_ROLE_USER)
			},
			wantCode: connect.CodePermissionDenied,
		},
		{
			name: "own request",
			mutate: func(t *testing.T, f *offerFixture, req *api.OfferTransferRequest) context.Context {
				// Make the owner the requester too.
				request := &models.Request{}
				if err := f.storage.GetByID(context.Background(), f.requestID, request); err != nil {
					t.Fatalf("reload request: %v", err)
				}
				request.RequesterId = f.ownerID
				if err := f.storage.Update(context.Background(), request); err != nil {
					t.Fatalf("update request: %v", err)
				}
				req.RecipientUserId = f.ownerID
				return f.ownerCtx()
			},
			wantCode: connect.CodePermissionDenied,
		},
		{
			name: "recipient is not the requester",
			mutate: func(t *testing.T, f *offerFixture, req *api.OfferTransferRequest) context.Context {
				req.RecipientUserId = f.ownerID
				return f.ownerCtx()
			},
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "terminal request",
			mutate: func(t *testing.T, f *offerFixture, req *api.OfferTransferRequest) context.Context {
				request := &models.Request{}
				if err := f.storage.GetByID(context.Background(), f.requestID, request); err != nil {
					t.Fatalf("reload request: %v", err)
				}
				request.State = models.RequestState_REQUEST_STATE_FULFILLED
				if err := f.storage.Update(context.Background(), request); err != nil {
					t.Fatalf("update request: %v", err)
				}
				return f.ownerCtx()
			},
			wantCode: connect.CodeFailedPrecondition,
		},
		{
			name: "gear unavailable",
			mutate: func(t *testing.T, f *offerFixture, req *api.OfferTransferRequest) context.Context {
				gear := &models.Gear{}
				if err := f.storage.GetByID(context.Background(), f.gearID, gear); err != nil {
					t.Fatalf("reload gear: %v", err)
				}
				gear.State = models.GearState_GEAR_STATE_UNAVAILABLE
				if err := f.storage.Update(context.Background(), gear); err != nil {
					t.Fatalf("update gear: %v", err)
				}
				return f.ownerCtx()
			},
			wantCode: connect.CodeFailedPrecondition,
		},
		{
			name: "request not shared into community",
			mutate: func(t *testing.T, f *offerFixture, req *api.OfferTransferRequest) context.Context {
				rows, err := storage.QueryByFields[*models.CommunityRequest](f.storage, context.Background(), map[string]any{
					"request_id": f.requestID,
				})
				if err != nil || len(rows) == 0 {
					t.Fatalf("load community request: %v", err)
				}
				rows[0].Archived = true
				if err := f.storage.Update(context.Background(), rows[0]); err != nil {
					t.Fatalf("archive community request: %v", err)
				}
				return f.ownerCtx()
			},
			wantCode: connect.CodeFailedPrecondition,
		},
		{
			name: "contribution owned by someone else",
			mutate: func(t *testing.T, f *offerFixture, req *api.OfferTransferRequest) context.Context {
				contribution := &models.PlanningContribution{}
				if err := f.storage.GetByID(context.Background(), f.contributionID, contribution); err != nil {
					t.Fatalf("reload contribution: %v", err)
				}
				contribution.ContributorId = f.askerID
				if err := f.storage.Update(context.Background(), contribution); err != nil {
					t.Fatalf("update contribution: %v", err)
				}
				return f.ownerCtx()
			},
			wantCode: connect.CodeInvalidArgument,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := setupOfferFixture(t)
			req := f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN)
			ctx := tc.mutate(t, f, req)

			_, err := f.service.OfferTransfer(ctx, connect.NewRequest(req))
			if err == nil {
				t.Fatal("expected error, got success")
			}
			if connect.CodeOf(err) != tc.wantCode {
				t.Errorf("expected code %s, got %s (%v)", tc.wantCode, connect.CodeOf(err), err)
			}
		})
	}
}

func TestOfferTransfer_DuplicateOfferRejected(t *testing.T) {
	f := setupOfferFixture(t)

	if _, err := f.service.OfferTransfer(f.ownerCtx(), connect.NewRequest(f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN))); err != nil {
		t.Fatalf("first OfferTransfer failed: %v", err)
	}

	// A second contribution escalating the same gear on the same request is
	// rejected while the first offer is live.
	ctx := context.Background()
	gearID := f.gearID
	secondContributionID, err := f.storage.Insert(ctx, &models.PlanningContribution{
		ContributorId: f.ownerID,
		Title:         "Lawn mower again",
		Scope:         &models.PlanningContribution_RequestId{RequestId: f.requestID},
		GearId:        &gearID,
	})
	if err != nil {
		t.Fatalf("insert second contribution: %v", err)
	}
	req := f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN)
	req.ContributionId = secondContributionID

	_, err = f.service.OfferTransfer(f.ownerCtx(), connect.NewRequest(req))
	if connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Errorf("expected AlreadyExists, got %v", err)
	}
}

// addRequestConversation gives the fixture's request a conversation so the
// offer's system line has somewhere to land, returning its id.
func (f *offerFixture) addRequestConversation(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	convID, err := f.storage.Insert(ctx, &models.ChatConversation{
		CommunityId:    f.communityID,
		ParticipantIds: []string{f.ownerID, f.askerID},
	})
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	request := &models.Request{}
	if err := f.storage.GetByID(ctx, f.requestID, request); err != nil {
		t.Fatalf("reload request: %v", err)
	}
	request.ConversationId = convID
	if err := f.storage.Update(ctx, request); err != nil {
		t.Fatalf("update request: %v", err)
	}
	return convID
}

// offerSystemLine returns the single OFFERED system message in convID.
func (f *offerFixture) offerSystemLine(t *testing.T, convID string) *models.SystemChatMessage {
	t.Helper()
	msgs, err := f.storage.QueryByField(context.Background(), "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	var line *models.SystemChatMessage
	for _, m := range msgs {
		sys := m.(*models.ChatMessage).GetSystemMessage()
		if sys != nil && sys.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_OFFERED {
			if line != nil {
				t.Fatal("expected exactly one OFFERED system line, found more")
			}
			line = sys
		}
	}
	if line == nil {
		t.Fatal("no OFFERED system line written")
	}
	return line
}

// TestOfferTransfer_WritesCombinedNeedLine verifies a claim-escalating offer
// announces itself as ONE line carrying both the need and the item
// ("X is bringing <need> — lending <item>", #2724), keyed by the
// contribution so it supersedes the earlier claim line in place.
func TestOfferTransfer_WritesCombinedNeedLine(t *testing.T) {
	f := setupOfferFixture(t)
	convID := f.addRequestConversation(t)

	// Make the contribution a claim on a named need.
	ctx := context.Background()
	contribution := &models.PlanningContribution{}
	if err := f.storage.GetByID(ctx, f.contributionID, contribution); err != nil {
		t.Fatalf("reload contribution: %v", err)
	}
	needID, needName := "need-1", "Lawn mower"
	contribution.FromNeedId = &needID
	contribution.OriginalNeedName = &needName
	if err := f.storage.Update(ctx, contribution); err != nil {
		t.Fatalf("update contribution: %v", err)
	}

	if _, err := f.service.OfferTransfer(f.ownerCtx(), connect.NewRequest(f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN))); err != nil {
		t.Fatalf("OfferTransfer failed: %v", err)
	}

	line := f.offerSystemLine(t, convID)
	if line.GetTemplateKey() != "chat.request.gear_offer_lend_for_need" {
		t.Errorf("TemplateKey = %q, want combined need+lend key", line.GetTemplateKey())
	}
	if line.TemplateParams["needName"] != needName {
		t.Errorf("needName param = %q, want %q", line.TemplateParams["needName"], needName)
	}
	if line.GetCoalesceKey() != f.contributionID {
		t.Errorf("CoalesceKey = %q, want contribution id %q (supersedes the claim line)", line.GetCoalesceKey(), f.contributionID)
	}
}

// TestOfferTransfer_WritesPossessiveFreeOfferLine verifies a free-form
// (no-need) offer reads without the possessive ("X is lending <item>",
// #2724).
func TestOfferTransfer_WritesPossessiveFreeOfferLine(t *testing.T) {
	f := setupOfferFixture(t)
	convID := f.addRequestConversation(t)

	if _, err := f.service.OfferTransfer(f.ownerCtx(), connect.NewRequest(f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN))); err != nil {
		t.Fatalf("OfferTransfer failed: %v", err)
	}

	line := f.offerSystemLine(t, convID)
	if line.GetTemplateKey() != "chat.request.gear_offer_lend" {
		t.Errorf("TemplateKey = %q, want gear_offer_lend", line.GetTemplateKey())
	}
	if got := line.Description; got == "" || strings.Contains(got, "their") {
		t.Errorf("Description = %q; want possessive-free offer line", got)
	}
}
