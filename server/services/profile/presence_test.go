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

func insertRequest(t *testing.T, db *storage.ProtoSQLStorage, requesterID, title string, createdAt int64, neededBy *int64) string {
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
		t.Fatalf("insertRequest: %v", err)
	}
	return r.Id
}

func insertCommunityRequest(t *testing.T, db *storage.ProtoSQLStorage, communityID, requestID string) {
	t.Helper()
	cr := &models.CommunityRequest{
		Id:          uuid.New().String(),
		RequestId:   requestID,
		CommunityId: communityID,
	}
	if _, err := db.Insert(context.Background(), cr); err != nil {
		t.Fatalf("insertCommunityRequest: %v", err)
	}
}

func insertOffer(t *testing.T, db *storage.ProtoSQLStorage, requestID, userID string, withdrawn bool) {
	t.Helper()
	o := &models.RequestOffer{
		Id:               uuid.New().String(),
		RequestId:        requestID,
		UserId:           userID,
		CreatedAtUnixSec: time.Now().Unix(),
		Withdrawn:        withdrawn,
	}
	if _, err := db.Insert(context.Background(), o); err != nil {
		t.Fatalf("insertOffer: %v", err)
	}
}

func insertExperienceAt(t *testing.T, db *storage.ProtoSQLStorage, ownerID, name string, startUnixSec int64, state models.ExperienceState) string {
	t.Helper()
	e := &models.Experience{
		Id:      uuid.New().String(),
		OwnerId: ownerID,
		Name:    name,
		State:   state,
		Time: &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{UnixTimestampSec: startUnixSec},
			},
		},
	}
	if _, err := db.Insert(context.Background(), e); err != nil {
		t.Fatalf("insertExperienceAt: %v", err)
	}
	return e.Id
}

func insertYesRSVP(t *testing.T, db *storage.ProtoSQLStorage, experienceID, userID, communityID string) {
	t.Helper()
	r := &models.ExperienceRSVP{
		Id:           uuid.New().String(),
		ExperienceId: experienceID,
		UserId:       userID,
		CommunityId:  communityID,
		Intention:    models.RSVPIntention_RSVP_INTENTION_YES,
	}
	if _, err := db.Insert(context.Background(), r); err != nil {
		t.Fatalf("insertYesRSVP: %v", err)
	}
}

func presenceReq(target string) *connect.Request[api.GetProfilePresenceForViewerRequest] {
	return connect.NewRequest(&api.GetProfilePresenceForViewerRequest{TargetUserId: target})
}

func TestGetProfilePresenceForViewer_RejectsUnauthenticated(t *testing.T) {
	svc, _ := setupTestService(t)
	_, err := svc.GetProfilePresenceForViewer(context.Background(), presenceReq("anyone"))
	if err == nil {
		t.Fatal("expected error for unauthenticated call")
	}
}

func TestGetProfilePresenceForViewer_RejectsEmptyTarget(t *testing.T) {
	svc, _ := setupTestService(t)
	_, err := svc.GetProfilePresenceForViewer(ctxWithAuth("viewer"), presenceReq(""))
	if err == nil {
		t.Fatal("expected error for empty target_user_id")
	}
}

func TestGetProfilePresenceForViewer_SelfViewIsQuiet(t *testing.T) {
	svc, db := setupTestService(t)
	self := insertUser(t, db, "Self", "", nil)
	resp, err := svc.GetProfilePresenceForViewer(ctxWithAuth(self), presenceReq(self))
	if err != nil {
		t.Fatalf("self-view presence: %v", err)
	}
	if resp.Msg.SheetKind != api.ProfileSheetKind_PROFILE_SHEET_KIND_QUIET {
		t.Errorf("self-view sheet_kind = %v, want QUIET", resp.Msg.SheetKind)
	}
}

func TestGetProfilePresenceForViewer_NoSharedCommunitiesIsQuiet(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)
	c := insertCommunity(t, db, "Target Only")
	insertMembership(t, db, c, target, 100)

	resp, err := svc.GetProfilePresenceForViewer(ctxWithAuth(viewer), presenceReq(target))
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	if resp.Msg.SheetKind != api.ProfileSheetKind_PROFILE_SHEET_KIND_QUIET {
		t.Errorf("sheet_kind = %v, want QUIET", resp.Msg.SheetKind)
	}
}

func TestGetProfilePresenceForViewer_ActiveAskSoonestDeadlineWins(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)
	shared := insertCommunity(t, db, "Shared Crew")
	insertMembership(t, db, shared, viewer, 100)
	insertMembership(t, db, shared, target, 100)

	now := time.Now().Unix()
	later := now + 5*24*3600
	sooner := now + 2*24*3600
	laterAsk := insertRequest(t, db, target, "Later ask", now-3600, &later)
	soonerAsk := insertRequest(t, db, target, "Belay partner", now-3600, &sooner)
	noDeadline := insertRequest(t, db, target, "No deadline ask", now-3600, nil)
	insertCommunityRequest(t, db, shared, laterAsk)
	insertCommunityRequest(t, db, shared, soonerAsk)
	insertCommunityRequest(t, db, shared, noDeadline)

	helper := insertUser(t, db, "Alfred", "", []string{"m-alfred"})
	insertOffer(t, db, soonerAsk, helper, false)
	insertOffer(t, db, soonerAsk, viewer, true) // withdrawn — not counted

	resp, err := svc.GetProfilePresenceForViewer(ctxWithAuth(viewer), presenceReq(target))
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	if resp.Msg.SheetKind != api.ProfileSheetKind_PROFILE_SHEET_KIND_ACTIVE_ASK {
		t.Fatalf("sheet_kind = %v, want ACTIVE_ASK", resp.Msg.SheetKind)
	}
	ask := resp.Msg.ActiveAsk
	if ask == nil {
		t.Fatal("active_ask is nil")
	}
	if ask.RequestId != soonerAsk {
		t.Errorf("active_ask.request_id = %q, want soonest-deadline ask %q", ask.RequestId, soonerAsk)
	}
	if ask.Title != "Belay partner" {
		t.Errorf("active_ask.title = %q", ask.Title)
	}
	if ask.CommunityId != shared {
		t.Errorf("active_ask.community_id = %q, want %q", ask.CommunityId, shared)
	}
	if ask.NeededByUnixSec == nil || *ask.NeededByUnixSec != sooner {
		t.Errorf("active_ask.needed_by = %v, want %d", ask.NeededByUnixSec, sooner)
	}
	if ask.CommittedCount != 1 {
		t.Errorf("committed_count = %d, want 1 (withdrawn offers excluded)", ask.CommittedCount)
	}
	if len(ask.Faces) != 1 || ask.Faces[0].DisplayName != "Alfred" {
		t.Errorf("faces = %v, want [Alfred]", ask.Faces)
	}
	if ask.ViewerCommitted {
		t.Error("viewer_committed = true, want false (viewer's offer is withdrawn)")
	}
}

func TestGetProfilePresenceForViewer_ViewerCommittedExcludedFromFaces(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)
	shared := insertCommunity(t, db, "Shared Crew")
	insertMembership(t, db, shared, viewer, 100)
	insertMembership(t, db, shared, target, 100)

	now := time.Now().Unix()
	deadline := now + 24*3600
	askID := insertRequest(t, db, target, "Belay partner", now-3600, &deadline)
	insertCommunityRequest(t, db, shared, askID)
	insertOffer(t, db, askID, viewer, false)

	resp, err := svc.GetProfilePresenceForViewer(ctxWithAuth(viewer), presenceReq(target))
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	ask := resp.Msg.ActiveAsk
	if ask == nil {
		t.Fatal("active_ask is nil")
	}
	if !ask.ViewerCommitted {
		t.Error("viewer_committed = false, want true")
	}
	if ask.CommittedCount != 1 {
		t.Errorf("committed_count = %d, want 1", ask.CommittedCount)
	}
	for _, f := range ask.Faces {
		if f.UserId == viewer {
			t.Error("faces include the viewer")
		}
	}
}

func TestGetProfilePresenceForViewer_StaleDeadlinelessAskDoesNotSurface(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)
	shared := insertCommunity(t, db, "Shared Crew")
	insertMembership(t, db, shared, viewer, 100)
	insertMembership(t, db, shared, target, 100)

	stale := time.Now().Unix() - 30*24*3600
	askID := insertRequest(t, db, target, "Old ask", stale, nil)
	insertCommunityRequest(t, db, shared, askID)

	resp, err := svc.GetProfilePresenceForViewer(ctxWithAuth(viewer), presenceReq(target))
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	if resp.Msg.SheetKind != api.ProfileSheetKind_PROFILE_SHEET_KIND_QUIET {
		t.Errorf("sheet_kind = %v, want QUIET (stale deadline-less ask filtered)", resp.Msg.SheetKind)
	}
}

func TestGetProfilePresenceForViewer_NextEventAndFaces(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)
	other := insertUser(t, db, "Betty", "", []string{"m-betty"})
	shared := insertCommunity(t, db, "Shared Crew")
	for _, u := range []string{viewer, target, other} {
		insertMembership(t, db, shared, u, 100)
	}

	now := time.Now().Unix()
	past := insertExperienceAt(t, db, target, "Past climb", now-7*24*3600,
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED)
	soon := insertExperienceAt(t, db, target, "Maple Canyon", now+3*24*3600,
		models.ExperienceState_EXPERIENCE_STATE_ACTIVE)
	later := insertExperienceAt(t, db, target, "Later trip", now+9*24*3600,
		models.ExperienceState_EXPERIENCE_STATE_ACTIVE)
	for _, e := range []string{past, soon, later} {
		insertYesRSVP(t, db, e, viewer, shared)
		insertYesRSVP(t, db, e, target, shared)
	}
	insertYesRSVP(t, db, soon, other, shared)

	resp, err := svc.GetProfilePresenceForViewer(ctxWithAuth(viewer), presenceReq(target))
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	if resp.Msg.SheetKind != api.ProfileSheetKind_PROFILE_SHEET_KIND_NEXT_EVENT {
		t.Fatalf("sheet_kind = %v, want NEXT_EVENT (past history exists)", resp.Msg.SheetKind)
	}
	if resp.Msg.SuppressHistory {
		t.Error("suppress_history = true, want false with shared history")
	}
	ev := resp.Msg.NextEvent
	if ev == nil {
		t.Fatal("next_event is nil")
	}
	if ev.ExperienceId != soon {
		t.Errorf("next_event = %q, want soonest upcoming %q", ev.ExperienceId, soon)
	}
	if ev.GoingCount != 3 {
		t.Errorf("going_count = %d, want 3", ev.GoingCount)
	}
	// Faces exclude the viewer (they know they're in).
	for _, f := range ev.Faces {
		if f.UserId == viewer {
			t.Errorf("faces include the viewer")
		}
	}
}

func TestGetProfilePresenceForViewer_ColdStart(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)
	shared := insertCommunity(t, db, "Shared Crew")
	insertMembership(t, db, shared, viewer, 100)
	insertMembership(t, db, shared, target, 100)

	now := time.Now().Unix()
	first := insertExperienceAt(t, db, target, "First one together", now+2*24*3600,
		models.ExperienceState_EXPERIENCE_STATE_ACTIVE)
	insertYesRSVP(t, db, first, viewer, shared)
	insertYesRSVP(t, db, first, target, shared)

	resp, err := svc.GetProfilePresenceForViewer(ctxWithAuth(viewer), presenceReq(target))
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	if resp.Msg.SheetKind != api.ProfileSheetKind_PROFILE_SHEET_KIND_COLD_START {
		t.Fatalf("sheet_kind = %v, want COLD_START", resp.Msg.SheetKind)
	}
	if !resp.Msg.SuppressHistory {
		t.Error("suppress_history = false, want true on cold start")
	}
	if resp.Msg.NextEvent == nil || resp.Msg.NextEvent.ExperienceId != first {
		t.Errorf("next_event = %v, want %q", resp.Msg.NextEvent, first)
	}
}

// TestGetProfilePresenceForViewer_LeakageInvariant seeds an ask and an
// event in a community the target belongs to but the viewer does not.
// Neither the hidden community id nor the hidden entity titles may
// appear anywhere in the marshalled response.
func TestGetProfilePresenceForViewer_LeakageInvariant(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)
	shared := insertCommunity(t, db, "Shared Crew")
	hidden := insertCommunity(t, db, "Hidden Society")
	insertMembership(t, db, shared, viewer, 100)
	insertMembership(t, db, shared, target, 100)
	insertMembership(t, db, hidden, target, 100)

	now := time.Now().Unix()
	deadline := now + 24*3600
	hiddenAsk := insertRequest(t, db, target, "Hidden ask title", now-3600, &deadline)
	insertCommunityRequest(t, db, hidden, hiddenAsk)

	hiddenEvent := insertExperienceAt(t, db, target, "Hidden event title", now+24*3600,
		models.ExperienceState_EXPERIENCE_STATE_ACTIVE)
	insertYesRSVP(t, db, hiddenEvent, viewer, hidden)
	insertYesRSVP(t, db, hiddenEvent, target, hidden)

	resp, err := svc.GetProfilePresenceForViewer(ctxWithAuth(viewer), presenceReq(target))
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	if resp.Msg.SheetKind != api.ProfileSheetKind_PROFILE_SHEET_KIND_QUIET {
		t.Errorf("sheet_kind = %v, want QUIET (hidden-community entities excluded)", resp.Msg.SheetKind)
	}
	wire, err := proto.Marshal(resp.Msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{hidden, "Hidden ask title", "Hidden event title"} {
		if strings.Contains(string(wire), forbidden) {
			t.Errorf("hidden value %q leaked into response wire", forbidden)
		}
	}
}

// TestGetProfilePresenceForViewer_QueryBound pins the handler's read
// count so ask/event detection cannot regress into per-entity N+1
// fan-out.
func TestGetProfilePresenceForViewer_QueryBound(t *testing.T) {
	svc, db := setupTestService(t)
	viewer := insertUser(t, db, "Viewer", "", nil)
	target := insertUser(t, db, "Target", "", nil)
	shared := insertCommunity(t, db, "Shared Crew")
	insertMembership(t, db, shared, viewer, 100)
	insertMembership(t, db, shared, target, 100)

	now := time.Now().Unix()
	for i := 0; i < 4; i++ {
		e := insertExperienceAt(t, db, target, "Trip", now+int64(i+1)*24*3600,
			models.ExperienceState_EXPERIENCE_STATE_ACTIVE)
		insertYesRSVP(t, db, e, viewer, shared)
		insertYesRSVP(t, db, e, target, shared)
	}

	ctx := storage.WithQueryStats(ctxWithAuth(viewer))
	storage.AssertMaxQueries(t, ctx, 12, func() {
		if _, err := svc.GetProfilePresenceForViewer(ctx, presenceReq(target)); err != nil {
			t.Fatalf("presence: %v", err)
		}
	})
}
