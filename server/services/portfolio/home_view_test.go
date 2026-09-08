package portfolio

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------.

// emptyHomeExtras returns a homeExtras with empty maps.
func emptyHomeExtras() *homeExtras {
	return &homeExtras{
		needsByRequestID:     make(map[string][]*models.PlanningNeed),
		offersByRequestID:    make(map[string][]*models.RequestOffer),
		dismissedDecisionIDs: make(map[string]bool),
	}
}

// newHomeData builds a minimal fetchedData for assembleHomeView tests.
func newHomeData() *fetchedData {
	return &fetchedData{
		communityIDs:       []string{"comm-1"},
		communityNameMap:   map[string]string{"comm-1": "Boulder BC"},
		communityMediaMap:  map[string]string{},
		expMap:             map[string]proto.Message{},
		commExpMap:         map[string]*models.CommunityExperience{},
		commReqMap:         map[string]*models.CommunityRequest{},
		reqMap:             map[string]proto.Message{},
		userRSVPTimes:      map[string]int64{},
		userRSVPIntentions: map[string]models.RSVPIntention{},
		invitedExpIDs:      map[string]bool{},
		expAttendeesMap:    map[string][]string{},
		reqOffererIDs:      map[string][]string{},
		transferConvMap:    map[string]*models.ChatConversation{},
		convByID:           map[string]*models.ChatConversation{},
		messagesByConvID:   map[string][]proto.Message{},
		convUnreadCount:    map[string]int32{},
		gearProtoMap:       map[string]proto.Message{},
		locationNameMap:    map[string]string{},
		userMap: map[string]*api.User{
			selfID:  {Id: selfID, Name: "Self User", MediaId: "m-self"},
			otherID: {Id: otherID, Name: "Sarah Lee", MediaId: "m-other"},
		},
		communityMemberCountMap: map[string]int32{},
		communityMemberUserIDs:  map[string][]string{},
		gearCommunityIDs:        map[string][]string{},
		expCommunityIDs:         map[string][]string{},
		reqCommunityIDs:         map[string][]string{},
		watchedExpIDs:           map[string]bool{},
		watchedReqIDs:           map[string]bool{},
		watchedGearIDs:          map[string]bool{},
	}
}

var (
	homeNow = time.Unix(1_700_000_000, 0) // fixed "now" for deterministic tests
	homeTZ  = time.UTC
)

// TestAssembleHomeView_EmptyData verifies the zero-commitment new-user case:
// every section empty, every counter zero — the client renders the hero.
func TestAssembleHomeView_EmptyData(t *testing.T) {
	resp := assembleHomeView(newHomeData(), emptyHomeExtras(), selfID, homeTZ, homeNow)
	if resp.DecisionCount != 0 || len(resp.Decisions) != 0 ||
		len(resp.UpNext) != 0 || len(resp.YourAsks) != 0 ||
		len(resp.Gear) != 0 || len(resp.RecentActivity) != 0 {
		t.Errorf("expected fully empty response, got %+v", resp)
	}
	if resp.GearCounts == nil || resp.GearCounts.Total != 0 {
		t.Errorf("gear counts = %+v, want zero totals", resp.GearCounts)
	}
}
