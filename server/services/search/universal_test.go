package search

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func setupCommunityMember(t *testing.T, s *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	membership := &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           userID,
		InviterId:        userID,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	if _, err := s.Insert(context.Background(), membership); err != nil {
		t.Fatalf("Failed to add member to community: %v", err)
	}
}

func setupTestRequest(t *testing.T, s *storage.ProtoSQLStorage, requesterID, title string) string {
	t.Helper()
	request := &models.Request{
		Title:       title,
		Description: "Test request",
		RequesterId: requesterID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	id, err := s.Insert(context.Background(), request)
	if err != nil {
		t.Fatalf("Failed to create test request: %v", err)
	}
	return id
}

func setupCommunityRequest(t *testing.T, s *storage.ProtoSQLStorage, communityID, requestID string) {
	t.Helper()
	cr := &models.CommunityRequest{
		CommunityId:     communityID,
		RequestId:       requestID,
		SharedAtUnixSec: time.Now().Unix(),
	}
	if _, err := s.Insert(context.Background(), cr); err != nil {
		t.Fatalf("Failed to create community request: %v", err)
	}
}

func setupTestExperience(t *testing.T, s *storage.ProtoSQLStorage, ownerID, name string) string {
	t.Helper()
	experience := &models.Experience{
		Name:        name,
		Description: "Test experience",
		OwnerId:     ownerID,
		State:       models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	}
	id, err := s.Insert(context.Background(), experience)
	if err != nil {
		t.Fatalf("Failed to create test experience: %v", err)
	}
	return id
}

func setupCommunityExperience(t *testing.T, s *storage.ProtoSQLStorage, communityID, experienceID string) {
	t.Helper()
	ce := &models.CommunityExperience{
		CommunityId:     communityID,
		ExperienceId:    experienceID,
		SharedAtUnixSec: time.Now().Unix(),
	}
	if _, err := s.Insert(context.Background(), ce); err != nil {
		t.Fatalf("Failed to create community experience: %v", err)
	}
}

// universalSearch runs the UniversalSearch RPC for the given user and query.
func universalSearch(
	t *testing.T,
	service *Service,
	userID, email, query string,
	maxPerGroup *int32,
) *api.UniversalSearchResponse {
	t.Helper()
	ctx := createAuthenticatedContext(userID, email)
	req := connect.NewRequest(&api.UniversalSearchRequest{
		Query:              query,
		MaxResultsPerGroup: maxPerGroup,
	})
	resp, err := service.UniversalSearch(ctx, req)
	if err != nil {
		t.Fatalf("UniversalSearch failed: %v", err)
	}
	return resp.Msg
}

// entityKeys returns the entity keys of all items in a group for membership checks.
func entityKeys(items []*api.SearchResultItem) map[string]bool {
	keys := make(map[string]bool, len(items))
	for _, item := range items {
		keys[searchResultEntityKey(item)] = true
	}
	return keys
}

func TestUniversalSearch_GroupsResults(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	callerID := setupTestUser(t, sqlStorage, "caller@test.com", "Caller")
	communityID := setupTestCommunity(t, sqlStorage, callerID, "Community A")

	gearID := setupTestGear(t, sqlStorage, callerID, "Drill Press")
	setupCommunityGear(t, sqlStorage, communityID, gearID)

	requestID := setupTestRequest(t, sqlStorage, callerID, "Need a Drill")
	setupCommunityRequest(t, sqlStorage, communityID, requestID)

	experienceID := setupTestExperience(t, sqlStorage, callerID, "Drill Workshop")
	setupCommunityExperience(t, sqlStorage, communityID, experienceID)

	memberID := setupTestUser(t, sqlStorage, "member@test.com", "Drill Sergeant")
	setupCommunityMember(t, sqlStorage, communityID, memberID)

	resp := universalSearch(t, service, callerID, "caller@test.com", "drill", nil)

	library := entityKeys(resp.LibraryResults)
	if !library["gear:"+gearID] {
		t.Errorf("expected gear %s in library_results, got %v", gearID, library)
	}
	if !library["request:"+requestID] {
		t.Errorf("expected request %s in library_results, got %v", requestID, library)
	}
	if len(resp.LibraryResults) != 2 {
		t.Errorf("expected exactly 2 library results, got %d", len(resp.LibraryResults))
	}

	plans := entityKeys(resp.PlansResults)
	if !plans["experience:"+experienceID] {
		t.Errorf("expected experience %s in plans_results, got %v", experienceID, plans)
	}
	if len(resp.PlansResults) != 1 {
		t.Errorf("expected exactly 1 plans result, got %d", len(resp.PlansResults))
	}

	people := entityKeys(resp.PeopleResults)
	if !people["user:"+memberID] {
		t.Errorf("expected user %s in people_results, got %v", memberID, people)
	}
	if len(resp.PeopleResults) != 1 {
		t.Errorf("expected exactly 1 people result, got %d", len(resp.PeopleResults))
	}
}

// TestUniversalSearch_ScopedToCallerCommunities is the trust-boundary test:
// matching items in a community the caller does not belong to must never
// appear, since the scope is derived from the caller's own memberships.
func TestUniversalSearch_ScopedToCallerCommunities(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	callerID := setupTestUser(t, sqlStorage, "caller@test.com", "Caller")
	memberCommunityID := setupTestCommunity(t, sqlStorage, callerID, "Member Community")
	ownGearID := setupTestGear(t, sqlStorage, callerID, "Drill Press")
	setupCommunityGear(t, sqlStorage, memberCommunityID, ownGearID)

	// A second community the caller is NOT a member of, containing a matching item.
	otherUserID := setupTestUser(t, sqlStorage, "other@test.com", "Other Owner")
	otherCommunityID := setupTestCommunity(t, sqlStorage, otherUserID, "Other Community")
	foreignGearID := setupTestGear(t, sqlStorage, otherUserID, "Drill Hammer")
	setupCommunityGear(t, sqlStorage, otherCommunityID, foreignGearID)

	resp := universalSearch(t, service, callerID, "caller@test.com", "drill", nil)

	library := entityKeys(resp.LibraryResults)
	if !library["gear:"+ownGearID] {
		t.Errorf("expected caller's own gear %s in library_results", ownGearID)
	}
	for _, group := range [][]*api.SearchResultItem{
		resp.LibraryResults, resp.PlansResults, resp.PeopleResults,
	} {
		if entityKeys(group)["gear:"+foreignGearID] {
			t.Fatalf("gear from non-member community leaked into results: %s", foreignGearID)
		}
	}
}

func TestUniversalSearch_PerGroupCap(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	callerID := setupTestUser(t, sqlStorage, "caller@test.com", "Caller")
	communityID := setupTestCommunity(t, sqlStorage, callerID, "Community A")

	seeded := defaultUniversalGroupSize + 2
	for i := 0; i < seeded; i++ {
		gearID := setupTestGear(t, sqlStorage, callerID, fmt.Sprintf("Drill %d", i))
		setupCommunityGear(t, sqlStorage, communityID, gearID)
	}

	// Unset cap uses the server default.
	resp := universalSearch(t, service, callerID, "caller@test.com", "drill", nil)
	if len(resp.LibraryResults) != defaultUniversalGroupSize {
		t.Errorf("expected default cap of %d library results, got %d",
			defaultUniversalGroupSize, len(resp.LibraryResults))
	}

	// Explicit cap below the default is honored.
	small := int32(3)
	resp = universalSearch(t, service, callerID, "caller@test.com", "drill", &small)
	if len(resp.LibraryResults) != int(small) {
		t.Errorf("expected %d library results with explicit cap, got %d",
			small, len(resp.LibraryResults))
	}
}

func TestUniversalSearch_EmptyQuery(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	for _, query := range []string{"", "   "} {
		ctx := createAuthenticatedContext("user-1", "user@test.com")
		req := connect.NewRequest(&api.UniversalSearchRequest{Query: query})

		_, err := service.UniversalSearch(ctx, req)
		if err == nil {
			t.Fatalf("expected error for query %q", query)
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("expected InvalidArgument for query %q, got %v", query, connect.CodeOf(err))
		}
	}
}

func TestUniversalSearch_NoCommunities(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "loner@test.com", "Loner")

	resp := universalSearch(t, service, userID, "loner@test.com", "drill", nil)
	if len(resp.LibraryResults)+len(resp.PlansResults)+len(resp.PeopleResults) != 0 {
		t.Errorf("expected empty response for user with no communities, got %+v", resp)
	}
}

func TestUniversalGroupLimit(t *testing.T) {
	tests := []struct {
		name string
		set  *int32
		want int
	}{
		{name: "unset uses default", set: nil, want: defaultUniversalGroupSize},
		{name: "zero uses default", set: ptrInt32(0), want: defaultUniversalGroupSize},
		{name: "negative uses default", set: ptrInt32(-5), want: defaultUniversalGroupSize},
		{name: "in-range value honored", set: ptrInt32(5), want: 5},
		{name: "ceiling honored", set: ptrInt32(maxUniversalGroupSize), want: maxUniversalGroupSize},
		{name: "above ceiling clamped", set: ptrInt32(maxUniversalGroupSize + 75), want: maxUniversalGroupSize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &api.UniversalSearchRequest{Query: "q", MaxResultsPerGroup: tt.set}
			if got := universalGroupLimit(req); got != tt.want {
				t.Errorf("universalGroupLimit(%v) = %d, want %d", tt.set, got, tt.want)
			}
		})
	}
}

func ptrInt32(v int32) *int32 { return &v }

// TestGroupUniversalResults_KeepsBestScored verifies that capping a group keeps
// the highest-scored items in descending score order.
func TestGroupUniversalResults_KeepsBestScored(t *testing.T) {
	gearItem := func(id string, score float64) *api.SearchResultItem {
		return &api.SearchResultItem{
			ItemType:       api.SearchItemType_SEARCH_ITEM_TYPE_GEAR,
			CompositeScore: score,
			Item:           &api.SearchResultItem_Gear{Gear: &api.Gear{Id: id}},
		}
	}
	results := []*api.SearchResultItem{
		gearItem("low", 0.2),
		gearItem("high", 0.9),
		gearItem("mid", 0.5),
		gearItem("lowest", 0.1),
	}

	library, plans, people := groupUniversalResults(results, 2)
	if len(plans) != 0 || len(people) != 0 {
		t.Errorf("expected only library results, got plans=%d people=%d", len(plans), len(people))
	}
	if len(library) != 2 {
		t.Fatalf("expected library capped at 2, got %d", len(library))
	}
	if library[0].GetGear().Id != "high" || library[1].GetGear().Id != "mid" {
		t.Errorf("expected best-scored [high, mid], got [%s, %s]",
			library[0].GetGear().Id, library[1].GetGear().Id)
	}
}
