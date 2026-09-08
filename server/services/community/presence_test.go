package community

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func insertPresenceRequest(t *testing.T, db *storage.ProtoSQLStorage, communityID, requesterID, title string, createdAt int64, neededBy *int64) string {
	t.Helper()
	r := &models.Request{
		Id:               uuid.New().String(),
		RequesterId:      requesterID,
		Title:            title,
		State:            models.RequestState_REQUEST_STATE_ACTIVE,
		CreatedAtUnixSec: createdAt,
		NeededByUnixSec:  neededBy,
	}
	if _, err := db.Insert(context.Background(), r); err != nil {
		t.Fatalf("insert request: %v", err)
	}
	cr := &models.CommunityRequest{
		Id:          uuid.New().String(),
		RequestId:   r.Id,
		CommunityId: communityID,
	}
	if _, err := db.Insert(context.Background(), cr); err != nil {
		t.Fatalf("insert community_request: %v", err)
	}
	return r.Id
}

func insertPresenceExperience(t *testing.T, db *storage.ProtoSQLStorage, communityID, ownerID, name string, startUnixSec int64) string {
	t.Helper()
	e := &models.Experience{
		Id:      uuid.New().String(),
		OwnerId: ownerID,
		Name:    name,
		State:   models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
		Time: &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{UnixTimestampSec: startUnixSec},
			},
		},
	}
	if _, err := db.Insert(context.Background(), e); err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	ce := &models.CommunityExperience{
		Id:           uuid.New().String(),
		CommunityId:  communityID,
		ExperienceId: e.Id,
	}
	if _, err := db.Insert(context.Background(), ce); err != nil {
		t.Fatalf("insert community_experience: %v", err)
	}
	return e.Id
}

func communityPresenceReq(communityID string) *connect.Request[api.GetCommunityPresenceForViewerRequest] {
	return connect.NewRequest(&api.GetCommunityPresenceForViewerRequest{
		CommunityId: communityID,
	})
}

// setupPresenceCommunity creates a community owned by a fresh user and
// returns (service, storage, ownerID, ownerCtx, communityID).
func setupPresenceCommunity(t *testing.T) (*Service, *storage.ProtoSQLStorage, string, context.Context, string) {
	t.Helper()
	db := setupTestStorage(t)
	svc := setupTestService(t, db)
	owner := setupTestUser(t, db, "owner@example.com", "Owner")
	ctx := createAuthenticatedContext(owner, "owner@example.com", models.Role_ROLE_USER)
	resp, err := svc.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Boulder Backcountry Crew",
	}))
	if err != nil {
		t.Fatalf("create community: %v", err)
	}
	return svc, db, owner, ctx, resp.Msg.Id
}

func TestGetCommunityPresenceForViewer_RejectsNonMember(t *testing.T) {
	svc, db, _, _, communityID := setupPresenceCommunity(t)
	outsider := setupTestUser(t, db, "outsider@example.com", "Outsider")
	ctx := createAuthenticatedContext(outsider, "outsider@example.com", models.Role_ROLE_USER)

	_, err := svc.GetCommunityPresenceForViewer(ctx, communityPresenceReq(communityID))
	if err == nil {
		t.Fatal("expected error for non-member caller")
	}
}

func TestGetCommunityPresenceForViewer_QuietWhenNothingOpen(t *testing.T) {
	svc, _, _, ctx, communityID := setupPresenceCommunity(t)

	resp, err := svc.GetCommunityPresenceForViewer(ctx, communityPresenceReq(communityID))
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	if resp.Msg.SheetKind != api.ProfileSheetKind_PROFILE_SHEET_KIND_QUIET {
		t.Errorf("sheet_kind = %v, want QUIET", resp.Msg.SheetKind)
	}
}

func TestGetCommunityPresenceForViewer_PriorityRuleAndQueue(t *testing.T) {
	svc, db, owner, ctx, communityID := setupPresenceCommunity(t)

	now := time.Now().Unix()
	soon := now + 24*3600
	later := now + 3*24*3600
	// Deadline ties resolve to longest-unclaimed (oldest created).
	tieOld := insertPresenceRequest(t, db, communityID, owner, "Truck for the haul", now-4*24*3600, &soon)
	insertPresenceRequest(t, db, communityID, owner, "Crash pad", now-3600, &soon)
	insertPresenceRequest(t, db, communityID, owner, "Kid-wrangler", now-3600, &later)
	insertPresenceRequest(t, db, communityID, owner, "No deadline", now-3600, nil)

	resp, err := svc.GetCommunityPresenceForViewer(ctx, communityPresenceReq(communityID))
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	if resp.Msg.SheetKind != api.ProfileSheetKind_PROFILE_SHEET_KIND_ACTIVE_ASK {
		t.Fatalf("sheet_kind = %v, want ACTIVE_ASK", resp.Msg.SheetKind)
	}
	ask := resp.Msg.ActiveAsk
	if ask == nil || ask.RequestId != tieOld {
		t.Fatalf("active_ask = %v, want tie broken to longest-unclaimed %q", ask, tieOld)
	}
	if len(resp.Msg.QueuedAsks) != 3 {
		t.Fatalf("queued_asks = %d, want 3", len(resp.Msg.QueuedAsks))
	}
	// Queue keeps priority order: same-deadline newer ask, later
	// deadline, then deadline-less.
	if resp.Msg.QueuedAsks[0].Title != "Crash pad" ||
		resp.Msg.QueuedAsks[1].Title != "Kid-wrangler" ||
		resp.Msg.QueuedAsks[2].Title != "No deadline" {
		t.Errorf("queue order = %v", resp.Msg.QueuedAsks)
	}
	// The winning ask never repeats in the queue.
	for _, q := range resp.Msg.QueuedAsks {
		if q.RequestId == tieOld {
			t.Error("winning ask also appears in queued_asks")
		}
	}
}

func TestGetCommunityPresenceForViewer_NextEventFallback(t *testing.T) {
	svc, db, owner, ctx, communityID := setupPresenceCommunity(t)

	now := time.Now().Unix()
	insertPresenceExperience(t, db, communityID, owner, "Later trip", now+9*24*3600)
	soonest := insertPresenceExperience(t, db, communityID, owner, "Maple Canyon", now+2*24*3600)

	resp, err := svc.GetCommunityPresenceForViewer(ctx, communityPresenceReq(communityID))
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	if resp.Msg.SheetKind != api.ProfileSheetKind_PROFILE_SHEET_KIND_NEXT_EVENT {
		t.Fatalf("sheet_kind = %v, want NEXT_EVENT", resp.Msg.SheetKind)
	}
	if resp.Msg.NextEvent == nil || resp.Msg.NextEvent.ExperienceId != soonest {
		t.Errorf("next_event = %v, want soonest %q", resp.Msg.NextEvent, soonest)
	}
}

// TestGetCommunityPresenceForViewer_LeakageInvariant seeds an ask in a
// different community; it must not influence or appear in this
// community's presence.
func TestGetCommunityPresenceForViewer_LeakageInvariant(t *testing.T) {
	svc, db, owner, ctx, communityID := setupPresenceCommunity(t)

	otherResp, err := svc.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Other Crew",
	}))
	if err != nil {
		t.Fatalf("create other community: %v", err)
	}
	now := time.Now().Unix()
	deadline := now + 24*3600
	insertPresenceRequest(t, db, otherResp.Msg.Id, owner, "Other-community ask", now-3600, &deadline)

	resp, err := svc.GetCommunityPresenceForViewer(ctx, communityPresenceReq(communityID))
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	if resp.Msg.SheetKind != api.ProfileSheetKind_PROFILE_SHEET_KIND_QUIET {
		t.Errorf("sheet_kind = %v, want QUIET", resp.Msg.SheetKind)
	}
	wire, err := proto.Marshal(resp.Msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(wire), "Other-community ask") {
		t.Error("other-community ask leaked into response wire")
	}
}

// TestGetCommunityPresenceForViewer_QueryBound pins the read count so
// ask/event detection cannot regress into per-entity fan-out.
func TestGetCommunityPresenceForViewer_QueryBound(t *testing.T) {
	svc, db, owner, ctx, communityID := setupPresenceCommunity(t)

	now := time.Now().Unix()
	for i := 0; i < 4; i++ {
		deadline := now + int64(i+1)*24*3600
		insertPresenceRequest(t, db, communityID, owner, "Ask", now-3600, &deadline)
	}

	statsCtx := storage.WithQueryStats(ctx)
	storage.AssertMaxQueries(t, statsCtx, 10, func() {
		if _, err := svc.GetCommunityPresenceForViewer(statsCtx, communityPresenceReq(communityID)); err != nil {
			t.Fatalf("presence: %v", err)
		}
	})
}
