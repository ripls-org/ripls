package profile

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

func insertGear(t *testing.T, db *storage.ProtoSQLStorage, ownerID, name string, mediaIDs []string, state models.GearState) string {
	t.Helper()
	g := &models.Gear{
		Id:               uuid.New().String(),
		OwnerId:          ownerID,
		Name:             name,
		MediaIds:         mediaIDs,
		State:            state,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	id, err := db.Insert(context.Background(), g)
	if err != nil {
		t.Fatalf("insertGear: %v", err)
	}
	return id
}

func insertCommunityGear(t *testing.T, db *storage.ProtoSQLStorage, communityID, gearID string) {
	t.Helper()
	cg := &models.CommunityGear{
		Id:               uuid.New().String(),
		CommunityId:      communityID,
		GearId:           gearID,
		CreatedAtUnixSec: time.Now().Unix(),
		Availability:     models.Availability_AVAILABILITY_FOR_LOAN,
	}
	if _, err := db.Insert(context.Background(), cg); err != nil {
		t.Fatalf("insertCommunityGear: %v", err)
	}
}

func insertExperience(t *testing.T, db *storage.ProtoSQLStorage, ownerID, name string, startUnix int64, state models.ExperienceState) string {
	t.Helper()
	e := &models.Experience{
		Id:      uuid.New().String(),
		OwnerId: ownerID,
		Name:    name,
		State:   state,
		Time: &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{UnixTimestampSec: startUnix},
			},
		},
	}
	id, err := db.Insert(context.Background(), e)
	if err != nil {
		t.Fatalf("insertExperience: %v", err)
	}
	return id
}

func insertCommunityExperience(t *testing.T, db *storage.ProtoSQLStorage, communityID, experienceID string) {
	t.Helper()
	ce := &models.CommunityExperience{
		Id:              uuid.New().String(),
		CommunityId:     communityID,
		ExperienceId:    experienceID,
		SharedAtUnixSec: time.Now().Unix(),
	}
	if _, err := db.Insert(context.Background(), ce); err != nil {
		t.Fatalf("insertCommunityExperience: %v", err)
	}
}

func insertFulfilledRequest(t *testing.T, db *storage.ProtoSQLStorage, requesterID, helperID, title, communityID string) string {
	t.Helper()
	r := &models.Request{
		Id:                 uuid.New().String(),
		RequesterId:        requesterID,
		Title:              title,
		Description:        "",
		State:              models.RequestState_REQUEST_STATE_FULFILLED,
		ConfirmedHelperIds: []string{helperID},
	}
	id, err := db.Insert(context.Background(), r)
	if err != nil {
		t.Fatalf("insertFulfilledRequest: %v", err)
	}
	cr := &models.CommunityRequest{
		Id:              uuid.New().String(),
		RequestId:       id,
		CommunityId:     communityID,
		SharedAtUnixSec: time.Now().Unix(),
	}
	if _, err := db.Insert(context.Background(), cr); err != nil {
		t.Fatalf("insertCommunityRequest: %v", err)
	}
	return id
}

// TestPostcards_AllThreeEmitted exercises the happy path: target has
// borrowable gear, an upcoming experience, and a help history inside a
// shared community. Three postcards should be returned.
func TestPostcards_AllThreeEmitted(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Liam Pemberton", "", nil)
	other := insertUser(t, db, "Other", "", nil)

	shared := insertCommunity(t, db, "Shared Crew")
	now := time.Now().Unix()
	insertMembership(t, db, shared, viewer, now)
	insertMembership(t, db, shared, target, now)

	// Gear that has been lent once and is shared with the community.
	gearID := insertGear(t, db, target, "Pressure Washer", []string{"media-1"},
		models.GearState_GEAR_STATE_AVAILABLE)
	insertCommunityGear(t, db, shared, gearID)
	insertCompletedLoan(t, db, shared, target, other, gearID, loanImpact(50, 0, 0))

	// Upcoming experience.
	insertExperience(t, db, target, "Saturday hike", now+86400,
		models.ExperienceState_EXPERIENCE_STATE_ACTIVE)
	// Wire to community via CommunityExperience.
	expIDs, err := storage.QueryByField[*models.Experience](db, context.Background(), "owner_id", target)
	if err != nil {
		t.Fatalf("query experiences: %v", err)
	}
	insertCommunityExperience(t, db, shared, expIDs[0].Id)

	// Past help.
	insertFulfilledRequest(t, db, other, target, "Move a couch", shared)

	req := connect.NewRequest(&api.GetUserProfileForViewerRequest{TargetUserId: target})
	resp, err := svc.GetUserProfileForViewer(ctxWithAuth(viewer), req)
	if err != nil {
		t.Fatalf("GetUserProfileForViewer: %v", err)
	}
	if resp.Msg.Payload == nil {
		t.Fatal("payload is nil")
	}
	if got := len(resp.Msg.Payload.CtaRows); got != 3 {
		t.Fatalf("len(cta_rows) = %d, want 3", got)
	}

	kinds := make(map[api.ItemKind]bool, 3)
	for _, row := range resp.Msg.Payload.CtaRows {
		kind := row.Item.GetKind()
		kinds[kind] = true
		if row.Headline == "" {
			t.Errorf("row %v has empty headline", kind)
		}
		// Postcards always reference an entity so the dispatcher has
		// somewhere to land.
		if row.Item.GetContextId() == "" {
			t.Errorf("row %v has empty item.context_id", kind)
		}
		if row.CommunityId == nil || *row.CommunityId != shared {
			t.Errorf("row %v has wrong community_id: %v", kind, row.CommunityId)
		}
	}
	for _, want := range []api.ItemKind{
		api.ItemKind_ITEM_KIND_GEAR,
		api.ItemKind_ITEM_KIND_EXPERIENCE,
		api.ItemKind_ITEM_KIND_REQUEST,
	} {
		if !kinds[want] {
			t.Errorf("missing postcard kind %v", want)
		}
	}
}

// TestPostcards_NoSharedCommunities asserts that no postcards are
// emitted when the viewer and target share no communities. The
// aggregate numbers still render (covered by the Phase 1 tests).
func TestPostcards_NoSharedCommunities(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)
	other := insertUser(t, db, "Other", "", nil)

	viewerOnly := insertCommunity(t, db, "Viewer Only")
	targetOnly := insertCommunity(t, db, "Target Only")
	now := time.Now().Unix()
	insertMembership(t, db, viewerOnly, viewer, now)
	insertMembership(t, db, targetOnly, target, now)

	// Target has gear and an experience inside targetOnly, but the viewer
	// is not in that community — nothing should surface.
	gearID := insertGear(t, db, target, "Drill", nil,
		models.GearState_GEAR_STATE_AVAILABLE)
	insertCommunityGear(t, db, targetOnly, gearID)
	insertCompletedLoan(t, db, targetOnly, target, other, gearID, loanImpact(10, 0, 0))

	req := connect.NewRequest(&api.GetUserProfileForViewerRequest{TargetUserId: target})
	resp, err := svc.GetUserProfileForViewer(ctxWithAuth(viewer), req)
	if err != nil {
		t.Fatalf("GetUserProfileForViewer: %v", err)
	}
	if got := len(resp.Msg.Payload.CtaRows); got != 0 {
		t.Errorf("len(cta_rows) = %d, want 0 (no shared communities)", got)
	}
}

// TestPostcards_NonSharedDataIsExcluded asserts that gear, experiences,
// and help in communities the viewer is not in never surface on a
// postcard, even when the target also has data in a shared community.
func TestPostcards_NonSharedDataIsExcluded(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)

	shared := insertCommunity(t, db, "Shared Crew")
	hidden := insertCommunity(t, db, "Hidden Society")
	now := time.Now().Unix()
	insertMembership(t, db, shared, viewer, now)
	insertMembership(t, db, shared, target, now)
	insertMembership(t, db, hidden, target, now)

	// Gear shared only in the hidden community.
	hiddenGear := insertGear(t, db, target, "Secret Sled", nil,
		models.GearState_GEAR_STATE_AVAILABLE)
	insertCommunityGear(t, db, hidden, hiddenGear)
	// Experience inside hidden community only.
	hiddenExp := insertExperience(t, db, target, "Secret Salon", now+3600,
		models.ExperienceState_EXPERIENCE_STATE_ACTIVE)
	insertCommunityExperience(t, db, hidden, hiddenExp)

	req := connect.NewRequest(&api.GetUserProfileForViewerRequest{TargetUserId: target})
	resp, err := svc.GetUserProfileForViewer(ctxWithAuth(viewer), req)
	if err != nil {
		t.Fatalf("GetUserProfileForViewer: %v", err)
	}
	if got := len(resp.Msg.Payload.CtaRows); got != 0 {
		t.Errorf("len(cta_rows) = %d, want 0 (no data in shared community)", got)
	}

	wire, err := proto.Marshal(resp.Msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(wire), hiddenGear) {
		t.Error("hidden gear id leaked into postcard surface")
	}
	if strings.Contains(string(wire), "Secret Sled") {
		t.Error("hidden gear name leaked")
	}
	if strings.Contains(string(wire), "Secret Salon") {
		t.Error("hidden experience name leaked")
	}
}

// TestPostcards_GearWithActiveLoanIsSkipped asserts that a gear with
// an ongoing transfer (ACTIVE / RECIPIENT_SELECTED state) is not
// surfaced — the viewer can't borrow it right now.
func TestPostcards_GearWithActiveLoanIsSkipped(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)
	other := insertUser(t, db, "Other", "", nil)

	shared := insertCommunity(t, db, "Shared")
	now := time.Now().Unix()
	insertMembership(t, db, shared, viewer, now)
	insertMembership(t, db, shared, target, now)

	gearID := insertGear(t, db, target, "Currently Out", nil,
		models.GearState_GEAR_STATE_AVAILABLE)
	insertCommunityGear(t, db, shared, gearID)

	// Active transfer ties up the gear.
	pickup := now - 600
	if _, err := db.Insert(context.Background(), &models.Transfer{
		Id:                  uuid.New().String(),
		CommunityId:         shared,
		OwnerId:             target,
		RecipientId:         other,
		GearId:              gearID,
		TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
		State:               models.TransferState_TRANSFER_STATE_ACTIVE,
		ActualPickupUnixSec: &pickup,
	}); err != nil {
		t.Fatalf("insert active transfer: %v", err)
	}

	req := connect.NewRequest(&api.GetUserProfileForViewerRequest{TargetUserId: target})
	resp, err := svc.GetUserProfileForViewer(ctxWithAuth(viewer), req)
	if err != nil {
		t.Fatalf("GetUserProfileForViewer: %v", err)
	}
	for _, row := range resp.Msg.Payload.CtaRows {
		if row.Item.GetKind() == api.ItemKind_ITEM_KIND_GEAR {
			t.Errorf("expected no gear postcard, got %v", row)
		}
	}
}

func TestFirstName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Liam Pemberton", "Liam"},
		{"Liam", "Liam"},
		{"", "them"},
		{"  Liam  Pemberton", "Liam"},
	}
	for _, tt := range tests {
		if got := firstName(tt.in); got != tt.want {
			t.Errorf("firstName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
