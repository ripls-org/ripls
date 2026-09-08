package profile

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/storage"
)

func setupTestService(t *testing.T) (*Service, *storage.ProtoSQLStorage) {
	t.Helper()
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed: %v", err)
	}
	calc := impact_metrics.NewCalculator(db, cfg)
	return New(db, calc, nil), db
}

func ctxWithAuth(userID string) context.Context {
	return authn.SetInfo(context.Background(), &auth.Info{
		UserID: userID,
		Email:  "test@example.com",
		Role:   models.Role_ROLE_USER,
	})
}

func insertUser(t *testing.T, db *storage.ProtoSQLStorage, name, description string, mediaIDs []string) string {
	t.Helper()
	user := &models.User{
		Id:          uuid.New().String(),
		Name:        name,
		Description: description,
		MediaIds:    mediaIDs,
	}
	id, err := db.Insert(context.Background(), user)
	if err != nil {
		t.Fatalf("insertUser: %v", err)
	}
	return id
}

func insertCommunity(t *testing.T, db *storage.ProtoSQLStorage, name string) string {
	t.Helper()
	community := &models.Community{
		Id:          uuid.New().String(),
		Name:        name,
		CreatorId:   "test",
		OwnerUserId: "test",
	}
	id, err := db.Insert(context.Background(), community)
	if err != nil {
		t.Fatalf("insertCommunity: %v", err)
	}
	return id
}

func insertMembership(t *testing.T, db *storage.ProtoSQLStorage, communityID, userID string, joinedAt int64) {
	t.Helper()
	m := &models.CommunityUser{
		Id:               uuid.New().String(),
		CommunityId:      communityID,
		UserId:           userID,
		InviterId:        userID,
		CreatedAtUnixSec: joinedAt,
	}
	if _, err := db.Insert(context.Background(), m); err != nil {
		t.Fatalf("insertMembership: %v", err)
	}
}

func loanImpact(costUSD, carbonGrams, timeMinutes float32) *models.ImpactEstimate {
	ie := &models.ImpactEstimate{}
	if costUSD > 0 {
		ie.MoneySaved = &models.MoneySavings{
			ValueUsd: &models.Estimate{Mean: costUSD, Stddev: costUSD * 0.3},
		}
	}
	if carbonGrams > 0 {
		ie.EmissionsPrevented = &models.PreventedEmissions{
			ManufactureAvoidedCarbon: &models.CarbonEstimate{
				Co2EGrams: &models.Estimate{Mean: carbonGrams, Stddev: carbonGrams * 0.4},
			},
		}
	}
	if timeMinutes > 0 {
		ie.TimeSaved = &models.TimeSavings{
			Minutes: &models.Estimate{Mean: timeMinutes, Stddev: timeMinutes * 0.3},
		}
	}
	return ie
}

func insertCompletedLoan(t *testing.T, db *storage.ProtoSQLStorage, communityID, ownerID, recipientID, gearID string, ie *models.ImpactEstimate) {
	t.Helper()
	now := time.Now().Unix()
	pickup := now - 3600
	transfer := &models.Transfer{
		Id:                  uuid.New().String(),
		CommunityId:         communityID,
		OwnerId:             ownerID,
		RecipientId:         recipientID,
		GearId:              gearID,
		TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
		State:               models.TransferState_TRANSFER_STATE_COMPLETED,
		ActualPickupUnixSec: &pickup,
		ActualReturnUnixSec: &now,
		ImpactEstimate:      ie,
	}
	if _, err := db.Insert(context.Background(), transfer); err != nil {
		t.Fatalf("insertCompletedLoan: %v", err)
	}
}

// insertGearWithCategory inserts a gear row tagged with the given
// category — used by tests that exercise the Known For chip
// derivation via the shared `known_for` library.
func insertGearWithCategory(t *testing.T, db *storage.ProtoSQLStorage, ownerID, name, category string) string {
	t.Helper()
	g := &models.Gear{
		Id:               uuid.New().String(),
		OwnerId:          ownerID,
		Name:             name,
		State:            models.GearState_GEAR_STATE_AVAILABLE,
		CreatedAtUnixSec: 1,
		Category:         &models.TrackedString{Value: category},
	}
	id, err := db.Insert(context.Background(), g)
	if err != nil {
		t.Fatalf("insertGearWithCategory: %v", err)
	}
	return id
}

// loanGearNTimes registers `n` distinct COMPLETED loans of the same
// gear inside `community` to push the known_for category counter over
// the threshold. Each loan goes to a fresh recipient.
func loanGearNTimes(t *testing.T, db *storage.ProtoSQLStorage, community, owner, gearID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		recipient := insertUser(t, db, "Recipient", "", nil)
		insertCompletedLoan(t, db, community, owner, recipient, gearID, nil)
	}
}

func TestGetUserProfileForViewer_RejectsUnauthenticated(t *testing.T) {
	svc, _ := setupTestService(t)
	req := connect.NewRequest(&api.GetUserProfileForViewerRequest{TargetUserId: "anyone"})
	_, err := svc.GetUserProfileForViewer(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for unauthenticated call")
	}
	if ce := new(connect.Error); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("expected Unauthenticated, got %v (%v)", connect.CodeOf(err), ce)
	}
}

func TestGetUserProfileForViewer_RejectsEmptyTarget(t *testing.T) {
	svc, _ := setupTestService(t)
	req := connect.NewRequest(&api.GetUserProfileForViewerRequest{TargetUserId: ""})
	_, err := svc.GetUserProfileForViewer(ctxWithAuth("viewer"), req)
	if err == nil {
		t.Fatal("expected error for empty target_user_id")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", connect.CodeOf(err))
	}
}

// TestGetUserProfileForViewer_AllowsSelfCall verifies that a viewer
// can request their own profile through this service. Self-call
// produces no postcards (since they would all reference the viewer's
// own gear / experiences / help) but otherwise renders normally:
// identity, every community the user is in listed as "shared", and
// the user's aggregate activity numbers.
func TestGetUserProfileForViewer_AllowsSelfCall(t *testing.T) {
	svc, db := setupTestService(t)
	self := insertUser(t, db, "Self", "", nil)
	community := insertCommunity(t, db, "My Crew")
	insertMembership(t, db, community, self, time.Now().Unix())

	req := connect.NewRequest(&api.GetUserProfileForViewerRequest{TargetUserId: self})
	resp, err := svc.GetUserProfileForViewer(ctxWithAuth(self), req)
	if err != nil {
		t.Fatalf("GetUserProfileForViewer (self): %v", err)
	}
	if resp.Msg.TargetUserId != self {
		t.Errorf("target_user_id = %q, want %q", resp.Msg.TargetUserId, self)
	}
	if len(resp.Msg.SharedCommunities) != 1 {
		t.Errorf("shared_communities len = %d, want 1 (every community lists as shared on self)", len(resp.Msg.SharedCommunities))
	}
	if resp.Msg.OtherCommunityCount != 0 {
		t.Errorf("other_community_count = %d, want 0", resp.Msg.OtherCommunityCount)
	}
	if resp.Msg.Payload == nil {
		t.Fatal("payload is nil")
	}
	if got := len(resp.Msg.Payload.CtaRows); got != 0 {
		t.Errorf("len(cta_rows) = %d, want 0 (postcards skipped on self)", got)
	}
}

// TestGetUserProfileForViewer_SelfView_StrictMirror covers issue #1996:
// when viewer == target, the rail, knownFor chips, and per-user impact
// narratives all populate from the target's full active community set
// — a strict mirror of what any other viewer (with full community
// overlap) would see — while postcards stay empty.
func TestGetUserProfileForViewer_SelfView_StrictMirror(t *testing.T) {
	svc, db := setupTestService(t)
	self := insertUser(t, db, "Self", "", nil)
	community := insertCommunity(t, db, "My Crew")
	insertMembership(t, db, community, self, time.Now().Unix())

	// Available, published gear → Available Now rail item.
	gearID := insertGear(t, db, self, "Electric chainsaw",
		[]string{"media-1"}, models.GearState_GEAR_STATE_AVAILABLE)
	insertCommunityGear(t, db, community, gearID)

	// Two completed loans of a categorized gear item → Known For chip.
	taggedGear := insertGearWithCategory(t, db, self, "Drill", "Power Tools")
	loanGearNTimes(t, db, community, self, taggedGear, 2)

	req := connect.NewRequest(&api.GetUserProfileForViewerRequest{TargetUserId: self})
	resp, err := svc.GetUserProfileForViewer(ctxWithAuth(self), req)
	if err != nil {
		t.Fatalf("GetUserProfileForViewer (self): %v", err)
	}

	if got := len(resp.Msg.AvailableNowItems); got != 1 {
		t.Errorf("available_now_items len = %d, want 1 (self-view rail must populate from own communities)", got)
	} else if resp.Msg.AvailableNowItems[0].ContextId != gearID {
		t.Errorf("available_now_items[0].context_id = %q, want %q",
			resp.Msg.AvailableNowItems[0].ContextId, gearID)
	}

	if got := len(resp.Msg.KnownFor); got != 1 {
		t.Errorf("known_for len = %d, want 1 (self-view chips must populate from own communities)", got)
	} else if resp.Msg.KnownFor[0] != "Power Tools" {
		t.Errorf("known_for[0] = %q, want %q", resp.Msg.KnownFor[0], "Power Tools")
	}

	if resp.Msg.Payload == nil {
		t.Fatal("payload is nil")
	}
	if got := len(resp.Msg.Payload.CtaRows); got != 0 {
		t.Errorf("len(cta_rows) = %d, want 0 (postcards stay empty on self-view)", got)
	}
}

// TestGetUserProfileForViewer_NumbersAreTargetScoped verifies that the
// hero/ticker numbers reflect the target user's full activity, across
// both shared and non-shared communities, not just the intersection.
func TestGetUserProfileForViewer_NumbersAreTargetScoped(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "Helpful neighbor", []string{"media-abc"})
	other := insertUser(t, db, "Other", "", nil)

	sharedCommunity := insertCommunity(t, db, "Shared Crew")
	nonSharedCommunity := insertCommunity(t, db, "Secret Society")

	now := time.Now().Unix()
	insertMembership(t, db, sharedCommunity, viewer, now)
	insertMembership(t, db, sharedCommunity, target, now-86400)
	// Target is in nonShared; viewer is not.
	insertMembership(t, db, nonSharedCommunity, target, now-2*86400)

	// Each loan contributes one to acts (TotalLoans), $100 savings,
	// 2200 g of carbon, 60 minutes of time-banked.
	gearA := uuid.New().String()
	gearB := uuid.New().String()
	insertCompletedLoan(t, db, sharedCommunity, target, other, gearA,
		loanImpact(100, 2200, 60))
	insertCompletedLoan(t, db, nonSharedCommunity, target, other, gearB,
		loanImpact(100, 2200, 60))

	req := connect.NewRequest(&api.GetUserProfileForViewerRequest{TargetUserId: target})
	resp, err := svc.GetUserProfileForViewer(ctxWithAuth(viewer), req)
	if err != nil {
		t.Fatalf("GetUserProfileForViewer: %v", err)
	}

	if resp.Msg.TargetName != "Target" {
		t.Errorf("target_name = %q, want %q", resp.Msg.TargetName, "Target")
	}
	if resp.Msg.TargetDescription == nil || *resp.Msg.TargetDescription != "Helpful neighbor" {
		t.Errorf("target_description = %v, want 'Helpful neighbor'", resp.Msg.TargetDescription)
	}
	if resp.Msg.TargetMediaId == nil || *resp.Msg.TargetMediaId != "media-abc" {
		t.Errorf("target_media_id = %v, want 'media-abc'", resp.Msg.TargetMediaId)
	}

	if len(resp.Msg.SharedCommunities) != 1 {
		t.Fatalf("shared_communities len = %d, want 1", len(resp.Msg.SharedCommunities))
	}
	if resp.Msg.SharedCommunities[0].Id != sharedCommunity {
		t.Errorf("shared_communities[0].Id = %q, want %q", resp.Msg.SharedCommunities[0].Id, sharedCommunity)
	}
	if resp.Msg.SharedCommunities[0].Name != "Shared Crew" {
		t.Errorf("shared_communities[0].Name = %q, want %q", resp.Msg.SharedCommunities[0].Name, "Shared Crew")
	}
	if resp.Msg.OtherCommunityCount != 1 {
		t.Errorf("other_community_count = %d, want 1", resp.Msg.OtherCommunityCount)
	}

	payload := resp.Msg.Payload
	if payload == nil {
		t.Fatal("payload is nil")
	}
	// 2 completed loans across both communities.
	if payload.ActsCount == nil || *payload.ActsCount < 2 {
		t.Errorf("acts_count = %v, want >= 2 (target scope, not intersection)", payload.ActsCount)
	}
	// $200 total cost savings across both loans.
	if payload.ReplacedCostUsd == nil || *payload.ReplacedCostUsd < 200 {
		t.Errorf("replaced_cost_usd = %v, want >= 200", payload.ReplacedCostUsd)
	}
	if payload.HoursTogether == nil {
		t.Error("hours_together should be populated")
	}
	if payload.EventsCount == nil {
		t.Error("events_count should be populated (even when zero)")
	}
	if payload.SinceUnixSec == nil || *payload.SinceUnixSec == 0 {
		t.Error("since_unix_sec should be populated from earliest membership")
	}
	// Earliest membership is the non-shared one (now-2*86400).
	expectedSince := now - 2*86400
	if payload.SinceUnixSec != nil && *payload.SinceUnixSec != expectedSince {
		t.Errorf("since_unix_sec = %d, want %d (earliest across all target communities)",
			*payload.SinceUnixSec, expectedSince)
	}
}

// TestGetUserProfileForViewer_LeakageInvariant asserts that the
// non-shared community's id and name appear nowhere in the response
// proto's serialized bytes. The aggregate numbers may reflect activity
// inside it, but the identity-level fields and the on-the-wire
// surface must not name or identify it.
func TestGetUserProfileForViewer_LeakageInvariant(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)

	shared := insertCommunity(t, db, "Visible Crew")
	hidden := insertCommunity(t, db, "Hidden Society")

	now := time.Now().Unix()
	insertMembership(t, db, shared, viewer, now)
	insertMembership(t, db, shared, target, now)
	insertMembership(t, db, hidden, target, now)

	req := connect.NewRequest(&api.GetUserProfileForViewerRequest{TargetUserId: target})
	resp, err := svc.GetUserProfileForViewer(ctxWithAuth(viewer), req)
	if err != nil {
		t.Fatalf("GetUserProfileForViewer: %v", err)
	}

	wire, err := proto.Marshal(resp.Msg)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(string(wire), hidden) {
		t.Errorf("non-shared community id %q leaked into response wire", hidden)
	}
	if strings.Contains(string(wire), "Hidden Society") {
		t.Errorf("non-shared community name leaked into response wire")
	}
	if resp.Msg.OtherCommunityCount != 1 {
		t.Errorf("other_community_count = %d, want 1", resp.Msg.OtherCommunityCount)
	}
}

// TestGetUserProfileForViewer_EmptyOverlap covers the case where the
// viewer shares no communities with the target. The aggregate numbers
// still render (they are target-scoped, not intersection-scoped) and
// the other_community_count surfaces correctly.
func TestGetUserProfileForViewer_EmptyOverlap(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)

	viewerOnly := insertCommunity(t, db, "Viewer's Crew")
	targetOnly := insertCommunity(t, db, "Target's Crew")

	now := time.Now().Unix()
	insertMembership(t, db, viewerOnly, viewer, now)
	insertMembership(t, db, targetOnly, target, now)

	req := connect.NewRequest(&api.GetUserProfileForViewerRequest{TargetUserId: target})
	resp, err := svc.GetUserProfileForViewer(ctxWithAuth(viewer), req)
	if err != nil {
		t.Fatalf("GetUserProfileForViewer: %v", err)
	}

	if len(resp.Msg.SharedCommunities) != 0 {
		t.Errorf("shared_communities len = %d, want 0", len(resp.Msg.SharedCommunities))
	}
	if resp.Msg.OtherCommunityCount != 1 {
		t.Errorf("other_community_count = %d, want 1", resp.Msg.OtherCommunityCount)
	}
	if resp.Msg.Payload == nil {
		t.Fatal("payload should still render when overlap is empty")
	}
}
