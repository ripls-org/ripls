package community

import (
	"context"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	communitylib "go.ripls.org/ripls/server/community"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// shareTestSetup creates a community, a member-inviter, and an experience
// shared with that community. Returns the storage handle, service, the
// inviter's authenticated context, the community ID, and the experience ID.
func shareTestSetup(t *testing.T) (sqlStorage *storage.ProtoSQLStorage, svc *Service, ctx context.Context, communityID, experienceID string) {
	t.Helper()
	sqlStorage = setupTestStorage(t)
	svc = setupTestService(t, sqlStorage)

	inviterID := setupTestUser(t, sqlStorage, "host@example.com", "Host")
	ctx = createAuthenticatedContext(inviterID, "host@example.com", models.Role_ROLE_USER)

	createResp, err := svc.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Event Community",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID = createResp.Msg.Id

	expID, err := sqlStorage.Insert(context.Background(), &models.Experience{
		OwnerId: inviterID,
		Name:    "Block Party",
		State:   models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	})
	if err != nil {
		t.Fatalf("Insert experience failed: %v", err)
	}
	experienceID = expID

	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityExperience{
		CommunityId:  communityID,
		ExperienceId: experienceID,
	}); err != nil {
		t.Fatalf("Insert CommunityExperience failed: %v", err)
	}
	return sqlStorage, svc, ctx, communityID, experienceID
}

// eventShareLinkReq is a small helper that builds the polymorphic
// GetOrCreateShareLinkRequest scoped to an event target.
func eventShareLinkReq(communityID, experienceID string) *connect.Request[api.GetOrCreateShareLinkRequest] {
	return connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_ExperienceId{ExperienceId: experienceID},
	})
}

func TestService_GetOrCreateShareLink_Event_Creates(t *testing.T) {
	_, svc, ctx, communityID, experienceID := shareTestSetup(t)

	resp, err := svc.GetOrCreateShareLink(ctx, eventShareLinkReq(communityID, experienceID))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink failed: %v", err)
	}
	if resp.Msg.ShortCode == "" {
		t.Error("expected non-empty short code")
	}
	if !strings.HasSuffix(resp.Msg.ShareUrl, "/go/"+resp.Msg.ShortCode) {
		t.Errorf("share URL %q should end with /go/<short_code>", resp.Msg.ShareUrl)
	}
	if resp.Msg.CommunityId != communityID {
		t.Errorf("CommunityId = %q, want %q", resp.Msg.CommunityId, communityID)
	}
}

func TestService_GetOrCreateShareLink_Event_Idempotent(t *testing.T) {
	_, svc, ctx, communityID, experienceID := shareTestSetup(t)

	first, err := svc.GetOrCreateShareLink(ctx, eventShareLinkReq(communityID, experienceID))
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	second, err := svc.GetOrCreateShareLink(ctx, eventShareLinkReq(communityID, experienceID))
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if first.Msg.ShortCode != second.Msg.ShortCode {
		t.Errorf("expected idempotent short code, got %q then %q",
			first.Msg.ShortCode, second.Msg.ShortCode)
	}
}

// TestService_ShortLinkCodeForEntity_Experience_FindsOrCreates proves the
// off-app-notification seam mints an 8-char /go code for an entity and reuses
// it on a second call (find-or-create keyed on the community owner).
func TestService_ShortLinkCodeForEntity_Experience_FindsOrCreates(t *testing.T) {
	_, svc, _, communityID, experienceID := shareTestSetup(t)

	code, err := svc.ShortLinkCodeForEntity(context.Background(), communityID, experienceID, "", "")
	if err != nil {
		t.Fatalf("ShortLinkCodeForEntity: %v", err)
	}
	if len(code) != shortCodeLength {
		t.Fatalf("expected an %d-char short code, got %q (len %d)", shortCodeLength, code, len(code))
	}

	code2, err := svc.ShortLinkCodeForEntity(context.Background(), communityID, experienceID, "", "")
	if err != nil {
		t.Fatalf("ShortLinkCodeForEntity (2nd): %v", err)
	}
	if code2 != code {
		t.Errorf("expected the same code on reuse, got %q then %q", code, code2)
	}
}

// TestService_ShortLinkCodeForEntity_ReusesHostLink proves the seam returns the
// SAME link the host already minted via the RPC — for a per-item community the
// owner is the host, so a notification link and a host-shared link coincide.
func TestService_ShortLinkCodeForEntity_ReusesHostLink(t *testing.T) {
	_, svc, ctx, communityID, experienceID := shareTestSetup(t)

	rpcResp, err := svc.GetOrCreateShareLink(ctx, eventShareLinkReq(communityID, experienceID))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink: %v", err)
	}

	code, err := svc.ShortLinkCodeForEntity(context.Background(), communityID, experienceID, "", "")
	if err != nil {
		t.Fatalf("ShortLinkCodeForEntity: %v", err)
	}
	if code != rpcResp.Msg.ShortCode {
		t.Errorf("expected to reuse host-minted link %q, got %q", rpcResp.Msg.ShortCode, code)
	}
}

// TestService_ShortLinkCodeForEntity_ExperienceWinsOverGear proves the entity
// precedence: when more than one id is supplied, the minted link targets the
// experience.
func TestService_ShortLinkCodeForEntity_ExperienceWinsOverGear(t *testing.T) {
	_, svc, _, communityID, experienceID := shareTestSetup(t)

	code, err := svc.ShortLinkCodeForEntity(context.Background(), communityID, experienceID, "some-gear-id", "")
	if err != nil {
		t.Fatalf("ShortLinkCodeForEntity: %v", err)
	}
	links, err := svc.lookupShareLink(context.Background(), code)
	if err != nil || len(links) != 1 {
		t.Fatalf("lookupShareLink(%q): links=%d err=%v", code, len(links), err)
	}
	if links[0].GetExperienceId() != experienceID {
		t.Errorf("expected link targeting experience %q, got experience=%q gear=%q",
			experienceID, links[0].GetExperienceId(), links[0].GetGearId())
	}
}

// TestService_ShortLinkCodeForEntity_NoEntity proves a notification with no
// share-linkable entity falls back to a community-invite link that lands on the
// community itself (e.g. a member-joined "say hi" notification).
func TestService_ShortLinkCodeForEntity_NoEntity(t *testing.T) {
	_, svc, _, communityID, _ := shareTestSetup(t)

	code, err := svc.ShortLinkCodeForEntity(context.Background(), communityID, "", "", "")
	if err != nil {
		t.Fatalf("ShortLinkCodeForEntity: %v", err)
	}
	if len(code) != shortCodeLength {
		t.Fatalf("expected an %d-char community link code, got %q (len %d)", shortCodeLength, code, len(code))
	}
	links, err := svc.lookupShareLink(context.Background(), code)
	if err != nil || len(links) != 1 {
		t.Fatalf("lookupShareLink(%q): links=%d err=%v", code, len(links), err)
	}
	if links[0].GetCommunityInviteId() != communityID {
		t.Errorf("expected a community-invite link for %q, got target community_invite=%q",
			communityID, links[0].GetCommunityInviteId())
	}
}

func TestService_GetOrCreateShareLink_Event_RejectsNonMember(t *testing.T) {
	sqlStorage, svc, _, communityID, experienceID := shareTestSetup(t)

	strangerID := setupTestUser(t, sqlStorage, "stranger@example.com", "Stranger")
	strangerCtx := createAuthenticatedContext(strangerID, "stranger@example.com", models.Role_ROLE_USER)

	_, err := svc.GetOrCreateShareLink(strangerCtx, eventShareLinkReq(communityID, experienceID))
	if err == nil {
		t.Fatal("expected error when non-member calls the RPC, got nil")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if got := connectErr.Code(); got != connect.CodePermissionDenied && got != connect.CodeNotFound {
		t.Errorf("expected PermissionDenied or NotFound, got %v", got)
	}
}

func TestService_GetOrCreateShareLink_Event_RejectsExperienceNotInCommunity(t *testing.T) {
	sqlStorage, svc, ctx, communityID, _ := shareTestSetup(t)

	// Insert an experience that is NOT shared with the test community.
	otherOwnerID := setupTestUser(t, sqlStorage, "other@example.com", "Other")
	otherExpID, err := sqlStorage.Insert(context.Background(), &models.Experience{
		OwnerId: otherOwnerID,
		Name:    "Other Event",
		State:   models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	})
	if err != nil {
		t.Fatalf("Insert other experience failed: %v", err)
	}

	_, err = svc.GetOrCreateShareLink(ctx, eventShareLinkReq(communityID, otherExpID))
	if err == nil {
		t.Fatal("expected error when event is not in the named community, got nil")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if got := connectErr.Code(); got != connect.CodeFailedPrecondition {
		t.Errorf("expected FailedPrecondition, got %v", got)
	}
}

// TestService_AcceptInvitationLink_Event_JoinsCommunity verifies that an
// event-flavored share link is accepted via AcceptInvitationLink — the user
// becomes a CommunityUser without any RSVP being written. The RSVP step is
// the client's responsibility per docs/issues/2050-web-rsvp-actions.md §D1.
func TestService_AcceptInvitationLink_Event_JoinsCommunity(t *testing.T) {
	sqlStorage, svc, hostCtx, communityID, experienceID := shareTestSetup(t)

	linkResp, err := svc.GetOrCreateShareLink(hostCtx, eventShareLinkReq(communityID, experienceID))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink (event) failed: %v", err)
	}

	guestID := setupTestUser(t, sqlStorage, "guest@example.com", "Guest")
	guestCtx := createAuthenticatedContext(guestID, "guest@example.com", models.Role_ROLE_USER)

	acceptResp, err := svc.AcceptInvitationLink(guestCtx, connect.NewRequest(&api.AcceptInvitationLinkRequest{
		ShortCode: linkResp.Msg.ShortCode,
	}))
	if err != nil {
		t.Fatalf("AcceptInvitationLink (event) failed: %v", err)
	}
	if acceptResp.Msg.CommunityId != communityID {
		t.Errorf("CommunityId = %q, want %q", acceptResp.Msg.CommunityId, communityID)
	}

	// Verify the guest is now a member.
	memberships, err := sqlStorage.QueryByField(guestCtx, "user_id", guestID, &models.CommunityUser{})
	if err != nil {
		t.Fatalf("QueryByField CommunityUser failed: %v", err)
	}
	foundMembership := false
	for _, msg := range memberships {
		if msg.(*models.CommunityUser).CommunityId == communityID {
			foundMembership = true
			break
		}
	}
	if !foundMembership {
		t.Error("expected guest to be a CommunityUser of the hosting community after accepting event share link")
	}

	// Verify no RSVP was created (client owns that step).
	rsvps, err := sqlStorage.QueryByField(guestCtx, "user_id", guestID, &models.ExperienceRSVP{})
	if err != nil {
		t.Fatalf("QueryByField ExperienceRSVP failed: %v", err)
	}
	for _, msg := range rsvps {
		if msg.(*models.ExperienceRSVP).ExperienceId == experienceID {
			t.Errorf("unexpected ExperienceRSVP row written by AcceptInvitationLink; client should fire RSVPToExperience separately")
		}
	}
}

func TestService_GetOrCreateShareLink_Gear_Creates(t *testing.T) {
	sqlStorage, svc, ctx, communityID, _ := shareTestSetup(t)

	// Insert a gear owned by the host and share it with the test community.
	ownerID := setupTestUser(t, sqlStorage, "owner2@example.com", "Owner2")
	gearID, err := sqlStorage.Insert(context.Background(), &models.Gear{
		OwnerId: ownerID,
		Name:    "Tent",
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	})
	if err != nil {
		t.Fatalf("Insert gear failed: %v", err)
	}
	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityGear{
		CommunityId: communityID,
		GearId:      gearID,
	}); err != nil {
		t.Fatalf("Insert CommunityGear failed: %v", err)
	}

	resp, err := svc.GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_GearId{GearId: gearID},
	}))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink(gear) failed: %v", err)
	}
	if resp.Msg.ShortCode == "" {
		t.Error("expected non-empty short code")
	}
	if !strings.HasSuffix(resp.Msg.ShareUrl, "/go/"+resp.Msg.ShortCode) {
		t.Errorf("share URL %q should end with /go/<short_code>", resp.Msg.ShareUrl)
	}

	// Verify the row was inserted with gear_id as the populated target.
	rows, err := sqlStorage.QueryByField(context.Background(), "short_code", resp.Msg.ShortCode, &models.ShareLink{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected 1 share_link row for short_code: rows=%d err=%v", len(rows), err)
	}
	if got := rows[0].(*models.ShareLink).GetGearId(); got != gearID {
		t.Errorf("GearId on inserted row = %q, want %q", got, gearID)
	}
}

func TestService_GetOrCreateShareLink_Gear_RejectsGearNotInCommunity(t *testing.T) {
	sqlStorage, svc, ctx, communityID, _ := shareTestSetup(t)

	// Insert a gear that is NOT shared with the test community.
	ownerID := setupTestUser(t, sqlStorage, "other-owner@example.com", "Other Owner")
	gearID, err := sqlStorage.Insert(context.Background(), &models.Gear{
		OwnerId: ownerID,
		Name:    "Tent",
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	})
	if err != nil {
		t.Fatalf("Insert gear failed: %v", err)
	}

	_, err = svc.GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_GearId{GearId: gearID},
	}))
	if err == nil {
		t.Fatal("expected error when gear is not in the named community, got nil")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if got := connectErr.Code(); got != connect.CodeFailedPrecondition {
		t.Errorf("expected FailedPrecondition, got %v", got)
	}
}

func TestService_GetOrCreateShareLink_Request_Creates(t *testing.T) {
	sqlStorage, svc, ctx, communityID, _ := shareTestSetup(t)

	requesterID := setupTestUser(t, sqlStorage, "requester@example.com", "Requester")
	requestID, err := sqlStorage.Insert(context.Background(), &models.Request{
		RequesterId: requesterID,
		Title:       "Need a saw",
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	})
	if err != nil {
		t.Fatalf("Insert request failed: %v", err)
	}
	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityRequest{
		CommunityId: communityID,
		RequestId:   requestID,
	}); err != nil {
		t.Fatalf("Insert CommunityRequest failed: %v", err)
	}

	resp, err := svc.GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_RequestId{RequestId: requestID},
	}))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink(request) failed: %v", err)
	}
	rows, _ := sqlStorage.QueryByField(context.Background(), "short_code", resp.Msg.ShortCode, &models.ShareLink{})
	if len(rows) != 1 || rows[0].(*models.ShareLink).GetRequestId() != requestID {
		t.Errorf("expected a share_link row with request_id=%q", requestID)
	}
}

func TestService_GetOrCreateShareLink_Transfer_Creates(t *testing.T) {
	sqlStorage, svc, ctx, communityID, _ := shareTestSetup(t)

	ownerID := setupTestUser(t, sqlStorage, "tx-owner@example.com", "Tx Owner")
	gearID, err := sqlStorage.Insert(context.Background(), &models.Gear{
		OwnerId: ownerID,
		Name:    "Drill",
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	})
	if err != nil {
		t.Fatalf("Insert gear failed: %v", err)
	}
	transferID, err := sqlStorage.Insert(context.Background(), &models.Transfer{
		OwnerId:      ownerID,
		GearId:       gearID,
		CommunityId:  communityID,
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_ACTIVE,
	})
	if err != nil {
		t.Fatalf("Insert transfer failed: %v", err)
	}

	resp, err := svc.GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_TransferId{TransferId: transferID},
	}))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink(transfer) failed: %v", err)
	}
	rows, _ := sqlStorage.QueryByField(context.Background(), "short_code", resp.Msg.ShortCode, &models.ShareLink{})
	if len(rows) != 1 || rows[0].(*models.ShareLink).GetTransferId() != transferID {
		t.Errorf("expected a share_link row with transfer_id=%q", transferID)
	}
}

func TestService_GetOrCreateShareLink_MissingTarget(t *testing.T) {
	_, svc, ctx, communityID, _ := shareTestSetup(t)

	_, err := svc.GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
	}))
	if err == nil {
		t.Fatal("expected InvalidArgument when target is missing, got nil")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if got := connectErr.Code(); got != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", got)
	}
}

// =============================================================================
// RevokeShareLink tests
// =============================================================================.

func TestService_RevokeShareLink_CommunityInvite(t *testing.T) {
	sqlStorage, svc, ctx, communityID, _ := shareTestSetup(t)

	// Mint a community-invite link.
	linkResp, err := svc.GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_CommunityInvite{CommunityInvite: communityID},
	}))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink failed: %v", err)
	}
	shortCode := linkResp.Msg.ShortCode

	_, err = svc.RevokeShareLink(ctx, connect.NewRequest(&api.RevokeShareLinkRequest{
		ShortCode: shortCode,
	}))
	if err != nil {
		t.Fatalf("RevokeShareLink failed: %v", err)
	}

	// Verify is_revoked is set.
	rows, err := sqlStorage.QueryByField(ctx, "short_code", shortCode, &models.ShareLink{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected 1 row; rows=%d err=%v", len(rows), err)
	}
	if !rows[0].(*models.ShareLink).IsRevoked {
		t.Error("expected is_revoked=true after RevokeShareLink")
	}
}

func TestService_RevokeShareLink_GearVariant(t *testing.T) {
	sqlStorage, svc, ctx, communityID, _ := shareTestSetup(t)

	ownerID := setupTestUser(t, sqlStorage, "gear-owner@example.com", "Gear Owner")
	gearID, err := sqlStorage.Insert(ctx, &models.Gear{
		OwnerId: ownerID,
		Name:    "Bike",
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	})
	if err != nil {
		t.Fatalf("Insert gear: %v", err)
	}
	if _, err := sqlStorage.Insert(ctx, &models.CommunityGear{
		CommunityId: communityID,
		GearId:      gearID,
	}); err != nil {
		t.Fatalf("Insert CommunityGear: %v", err)
	}

	linkResp, err := svc.GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_GearId{GearId: gearID},
	}))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink(gear): %v", err)
	}
	shortCode := linkResp.Msg.ShortCode

	_, err = svc.RevokeShareLink(ctx, connect.NewRequest(&api.RevokeShareLinkRequest{
		ShortCode: shortCode,
	}))
	if err != nil {
		t.Fatalf("RevokeShareLink(gear): %v", err)
	}

	rows, _ := sqlStorage.QueryByField(ctx, "short_code", shortCode, &models.ShareLink{})
	if len(rows) != 1 || !rows[0].(*models.ShareLink).IsRevoked {
		t.Error("expected is_revoked=true on gear share link")
	}
}

func TestService_RevokeShareLink_PermissionDenied_NonOwner(t *testing.T) {
	sqlStorage, svc, ctx, communityID, experienceID := shareTestSetup(t)

	// Inviter (ctx) creates a link.
	linkResp, err := svc.GetOrCreateShareLink(ctx, eventShareLinkReq(communityID, experienceID))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink: %v", err)
	}
	shortCode := linkResp.Msg.ShortCode

	// A different user (not the inviter) tries to revoke it.
	strangerID := setupTestUser(t, sqlStorage, "stranger2@example.com", "Stranger2")
	strangerCtx := createAuthenticatedContext(strangerID, "stranger2@example.com", models.Role_ROLE_USER)

	_, err = svc.RevokeShareLink(strangerCtx, connect.NewRequest(&api.RevokeShareLinkRequest{
		ShortCode: shortCode,
	}))
	if err == nil {
		t.Fatal("expected PermissionDenied when non-owner revokes, got nil")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if got := connectErr.Code(); got != connect.CodePermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", got)
	}
}

func TestService_RevokeShareLink_NotFound(t *testing.T) {
	_, svc, ctx, _, _ := shareTestSetup(t)

	_, err := svc.RevokeShareLink(ctx, connect.NewRequest(&api.RevokeShareLinkRequest{
		ShortCode: "nonexistent",
	}))
	if err == nil {
		t.Fatal("expected NotFound for unknown short code, got nil")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if got := connectErr.Code(); got != connect.CodeNotFound {
		t.Errorf("expected NotFound, got %v", got)
	}
}

func TestService_RevokeShareLink_Idempotent(t *testing.T) {
	_, svc, ctx, communityID, experienceID := shareTestSetup(t)

	linkResp, err := svc.GetOrCreateShareLink(ctx, eventShareLinkReq(communityID, experienceID))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink: %v", err)
	}
	shortCode := linkResp.Msg.ShortCode

	revokeReq := connect.NewRequest(&api.RevokeShareLinkRequest{ShortCode: shortCode})
	if _, err = svc.RevokeShareLink(ctx, revokeReq); err != nil {
		t.Fatalf("first RevokeShareLink: %v", err)
	}
	// Second revoke must also succeed (idempotent).
	if _, err = svc.RevokeShareLink(ctx, revokeReq); err != nil {
		t.Fatalf("second RevokeShareLink should be idempotent, got: %v", err)
	}
}

// =============================================================================
// Ad-hoc community scoping tests (#2767)
// =============================================================================.

// provisionAdHocExperienceCommunity provisions the per-item ad-hoc community
// for an experience and links the experience into it (so the verify closure
// passes). Returns the ad-hoc community id.
func provisionAdHocExperienceCommunity(t *testing.T, sqlStorage *storage.ProtoSQLStorage, svc *Service, hostID, experienceID string) string {
	t.Helper()
	bus := busFor(t, svc)
	adHocID, err := communitylib.ProvisionPerItemCommunity(
		context.Background(), sqlStorage, bus,
		hostID, communitylib.Origin{ExperienceID: experienceID},
	)
	if err != nil {
		t.Fatalf("ProvisionPerItemCommunity: %v", err)
	}
	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityExperience{
		CommunityId:  adHocID,
		ExperienceId: experienceID,
	}); err != nil {
		t.Fatalf("Insert CommunityExperience (ad-hoc): %v", err)
	}
	return adHocID
}

// TestService_GetOrCreateShareLink_Event_ScopesToAdHocCommunity verifies that
// when a caller supplies a named community, GetOrCreateShareLink re-scopes the
// link to the item's ad-hoc origin community. The response's community_id and
// the stored ShareLink row both reflect the ad-hoc community.
func TestService_GetOrCreateShareLink_Event_ScopesToAdHocCommunity(t *testing.T) {
	sqlStorage, svc, ctx, namedCommunityID, experienceID := shareTestSetup(t)

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		t.Fatalf("RequireAuth: %v", err)
	}
	adHocID := provisionAdHocExperienceCommunity(t, sqlStorage, svc, authInfo.UserID, experienceID)

	resp, err := svc.GetOrCreateShareLink(ctx, eventShareLinkReq(namedCommunityID, experienceID))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink: %v", err)
	}

	if resp.Msg.CommunityId != adHocID {
		t.Errorf("response CommunityId = %q, want ad-hoc %q", resp.Msg.CommunityId, adHocID)
	}
	rows, err := sqlStorage.QueryByField(context.Background(), "short_code", resp.Msg.ShortCode, &models.ShareLink{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected 1 share_link row; rows=%d err=%v", len(rows), err)
	}
	if got := rows[0].(*models.ShareLink).CommunityId; got != adHocID {
		t.Errorf("ShareLink CommunityId = %q, want ad-hoc %q", got, adHocID)
	}
}

// TestService_GetOrCreateShareLink_Event_SelfHealsToAdHoc verifies that an
// existing ShareLink row scoped to a named community is re-scoped to the ad-hoc
// community in-place on the next mint, without changing the short code (so
// already-distributed URLs self-heal).
func TestService_GetOrCreateShareLink_Event_SelfHealsToAdHoc(t *testing.T) {
	sqlStorage, svc, ctx, namedCommunityID, experienceID := shareTestSetup(t)

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		t.Fatalf("RequireAuth: %v", err)
	}

	// Pre-insert a link scoped to the named community (simulates links minted
	// before this fix).
	shortCode, err := svc.generateUniqueShortCode(context.Background())
	if err != nil {
		t.Fatalf("generateUniqueShortCode: %v", err)
	}
	if _, err := sqlStorage.Insert(context.Background(), &models.ShareLink{
		CommunityId: namedCommunityID,
		InviterId:   authInfo.UserID,
		ShortCode:   shortCode,
		IsRevoked:   false,
		Target:      &models.ShareLink_ExperienceId{ExperienceId: experienceID},
	}); err != nil {
		t.Fatalf("Insert pre-existing named-community link: %v", err)
	}

	adHocID := provisionAdHocExperienceCommunity(t, sqlStorage, svc, authInfo.UserID, experienceID)

	resp, err := svc.GetOrCreateShareLink(ctx, eventShareLinkReq(namedCommunityID, experienceID))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink (self-heal): %v", err)
	}

	// Same short code (row reused), community_id updated to ad-hoc.
	if resp.Msg.ShortCode != shortCode {
		t.Errorf("expected reused short code %q, got %q", shortCode, resp.Msg.ShortCode)
	}
	if resp.Msg.CommunityId != adHocID {
		t.Errorf("response CommunityId = %q, want ad-hoc %q", resp.Msg.CommunityId, adHocID)
	}
	rows, _ := sqlStorage.QueryByField(context.Background(), "short_code", shortCode, &models.ShareLink{})
	if len(rows) != 1 || rows[0].(*models.ShareLink).CommunityId != adHocID {
		t.Errorf("ShareLink row not self-healed; want CommunityId=%q", adHocID)
	}
}

// TestService_GetOrCreateShareLink_Event_FallsBackOnLegacyItem verifies that
// when no ad-hoc community exists (legacy item predating #2492), the link falls
// back to the caller-supplied community without error.
func TestService_GetOrCreateShareLink_Event_FallsBackOnLegacyItem(t *testing.T) {
	_, svc, ctx, namedCommunityID, experienceID := shareTestSetup(t)

	// No ad-hoc community provisioned — simulates a legacy item.
	resp, err := svc.GetOrCreateShareLink(ctx, eventShareLinkReq(namedCommunityID, experienceID))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink (legacy fallback): %v", err)
	}
	if resp.Msg.CommunityId != namedCommunityID {
		t.Errorf("response CommunityId = %q, want named fallback %q", resp.Msg.CommunityId, namedCommunityID)
	}
}

// TestService_ShortLinkCodeForEntity_ScopesToAdHocCommunity verifies that the
// off-app notification seam re-scopes item-target links to the ad-hoc
// community, so notification links never carry a spurious named community in
// their preview.
func TestService_ShortLinkCodeForEntity_ScopesToAdHocCommunity(t *testing.T) {
	sqlStorage, svc, ctx, namedCommunityID, experienceID := shareTestSetup(t)

	authInfo, _ := auth.RequireAuth(ctx)
	bus := busFor(t, svc)
	adHocID, err := communitylib.ProvisionPerItemCommunity(
		context.Background(), sqlStorage, bus,
		authInfo.UserID, communitylib.Origin{ExperienceID: experienceID},
	)
	if err != nil {
		t.Fatalf("ProvisionPerItemCommunity: %v", err)
	}

	// ShortLinkCodeForEntity is called with the named community (as a notification would).
	code, err := svc.ShortLinkCodeForEntity(context.Background(), namedCommunityID, experienceID, "", "")
	if err != nil {
		t.Fatalf("ShortLinkCodeForEntity: %v", err)
	}

	links, err := svc.lookupShareLink(context.Background(), code)
	if err != nil || len(links) != 1 {
		t.Fatalf("lookupShareLink(%q): links=%d err=%v", code, len(links), err)
	}
	if links[0].CommunityId != adHocID {
		t.Errorf("ShortLinkCodeForEntity link CommunityId = %q, want ad-hoc %q",
			links[0].CommunityId, adHocID)
	}
}

// TestService_AcceptInvitationLink_Event_JoinsAdHocNotNamed verifies that a
// joiner accepting an item-target share link joins the item's ad-hoc community,
// not the caller's named community.
func TestService_AcceptInvitationLink_Event_JoinsAdHocNotNamed(t *testing.T) {
	sqlStorage, svc, hostCtx, namedCommunityID, experienceID := shareTestSetup(t)

	authInfo, _ := auth.RequireAuth(hostCtx)
	adHocID := provisionAdHocExperienceCommunity(t, sqlStorage, svc, authInfo.UserID, experienceID)

	// Mint the link with the named community — it is re-scoped to ad-hoc.
	linkResp, err := svc.GetOrCreateShareLink(hostCtx, eventShareLinkReq(namedCommunityID, experienceID))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink: %v", err)
	}
	if linkResp.Msg.CommunityId != adHocID {
		t.Fatalf("link CommunityId pre-condition failed: got %q, want %q",
			linkResp.Msg.CommunityId, adHocID)
	}

	guestID := setupTestUser(t, sqlStorage, "guest-adhoc@example.com", "Guest AdHoc")
	guestCtx := createAuthenticatedContext(guestID, "guest-adhoc@example.com", models.Role_ROLE_USER)

	acceptResp, err := svc.AcceptInvitationLink(guestCtx, connect.NewRequest(&api.AcceptInvitationLinkRequest{
		ShortCode: linkResp.Msg.ShortCode,
	}))
	if err != nil {
		t.Fatalf("AcceptInvitationLink: %v", err)
	}
	if acceptResp.Msg.CommunityId != adHocID {
		t.Errorf("AcceptInvitationLink CommunityId = %q, want ad-hoc %q",
			acceptResp.Msg.CommunityId, adHocID)
	}

	// Guest is a member of the ad-hoc community, not the named one.
	memberships, err := sqlStorage.QueryByField(context.Background(), "user_id", guestID, &models.CommunityUser{})
	if err != nil {
		t.Fatalf("QueryByField CommunityUser: %v", err)
	}
	joinedAdHoc := false
	for _, m := range memberships {
		cu := m.(*models.CommunityUser)
		if cu.CommunityId == namedCommunityID {
			t.Errorf("guest joined named community %q — expected ad-hoc only", namedCommunityID)
		}
		if cu.CommunityId == adHocID {
			joinedAdHoc = true
		}
	}
	if !joinedAdHoc {
		t.Error("guest did not join the ad-hoc community")
	}
}
