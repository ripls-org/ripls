package experience

import (
	"context"
	"reflect"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestGetExperience_InvitedNoReply verifies the "still no reply" list:
// caller-visible community members who have not responded, excluding the host
// and anyone with an RSVP, and never leaking members of communities the caller
// is not in.
func TestGetExperience_InvitedNoReply(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	for _, u := range []struct{ id, email, name string }{
		{"owner1", "owner@example.com", "Owner"},
		{"maya", "maya@example.com", "Maya"},
		{"sam", "sam@example.com", "Sam"},
		{"wes", "wes@example.com", "Wes"},
		{"bob", "bob@example.com", "Bob"},
	} {
		createTestUser(t, testStorage, u.id, u.email, u.name)
	}

	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)
	mayaCtx := createAuthenticatedContext("maya", "maya@example.com", models.Role_ROLE_USER)

	communityA := createTestCommunity(t, testStorage, "Community A", "owner1")
	communityB := createTestCommunity(t, testStorage, "Community B", "owner1")
	for _, uid := range []string{"owner1", "maya", "sam", "wes"} {
		createTestCommunityMembership(t, testStorage, communityA, uid)
	}
	for _, uid := range []string{"owner1", "bob"} {
		createTestCommunityMembership(t, testStorage, communityB, uid)
	}

	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Hike",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id
	for _, cid := range []string{communityA, communityB} {
		shareExperienceForTest(t, service, ownerCtx, expID, cid)
	}

	// Maya responds; everyone else has not replied.
	if _, err := service.RSVPToExperience(mayaCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityA,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	})); err != nil {
		t.Fatalf("RSVP failed: %v", err)
	}

	noReplyIDs := func(t *testing.T, ctx context.Context) []string {
		t.Helper()
		resp, err := service.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		ids := make([]string, 0, len(resp.Msg.InvitedNoReply))
		for _, u := range resp.Msg.InvitedNoReply {
			ids = append(ids, u.Id)
		}
		return ids
	}

	t.Run("owner sees non-responders across their communities, minus host", func(t *testing.T) {
		// Owner is a member of A and B; host (owner1) and responder (maya) excluded.
		got := noReplyIDs(t, ownerCtx)
		want := []string{"bob", "sam", "wes"} // sorted by id
		if !reflect.DeepEqual(got, want) {
			t.Errorf("InvitedNoReply = %v, want %v", got, want)
		}
	})

	t.Run("caller only sees non-responders in their own communities", func(t *testing.T) {
		// Maya is a member of A only, so bob (community B) must not leak; maya
		// herself responded, so she is excluded too.
		got := noReplyIDs(t, mayaCtx)
		want := []string{"sam", "wes"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("InvitedNoReply = %v, want %v", got, want)
		}
	})

	t.Run("per-community no-reply counts exclude host + responders", func(t *testing.T) {
		resp, err := service.GetExperience(ownerCtx, connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		// Community A members owner1,maya,sam,wes → minus host(owner1) minus
		// responder(maya) = {sam,wes} = 2. Community B owner1,bob → minus host = {bob} = 1.
		if got := resp.Msg.CommunityNoReplyCounts[communityA]; got != 2 {
			t.Errorf("CommunityNoReplyCounts[A] = %d, want 2", got)
		}
		if got := resp.Msg.CommunityNoReplyCounts[communityB]; got != 1 {
			t.Errorf("CommunityNoReplyCounts[B] = %d, want 1", got)
		}
	})

	t.Run("no-reply counts are scoped to the caller's communities", func(t *testing.T) {
		// Maya is in A only — community B's count must not leak to her.
		resp, err := service.GetExperience(mayaCtx, connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		if got := resp.Msg.CommunityNoReplyCounts[communityA]; got != 2 {
			t.Errorf("CommunityNoReplyCounts[A] = %d, want 2", got)
		}
		if _, ok := resp.Msg.CommunityNoReplyCounts[communityB]; ok {
			t.Error("CommunityNoReplyCounts must not include community B for maya")
		}
	})
}

// TestGetExperience_OriginCommunityFlagged verifies that the event's own
// per-item (origin) community — auto-provisioned by SaveExperience — is marked
// IsOriginCommunity in SharedCommunities, while a real named community the
// event is also shared into is not. The Who's In roster uses this flag to hide
// the redundant origin row (#2492).
func TestGetExperience_OriginCommunityFlagged(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "owner1", "owner@example.com", "Owner")
	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)

	named := createTestCommunity(t, testStorage, "Lake Travis Outdoor Club", "owner1")
	createTestCommunityMembership(t, testStorage, named, "owner1")

	createResp, err := service.SaveExperience(ownerCtx,
		connect.NewRequest(&api.SaveExperienceRequest{Name: "Paddle"}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// SaveExperience auto-provisions the event's own per-item (origin) community;
	// additionally share into a real named community.
	shareExperienceForTest(t, service, ownerCtx, expID, named)

	resp, err := service.GetExperience(ownerCtx,
		connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
	if err != nil {
		t.Fatalf("GetExperience: %v", err)
	}

	var sawNamed, sawOrigin bool
	for _, sc := range resp.Msg.SharedCommunities {
		switch {
		case sc.CommunityId == named:
			sawNamed = true
			if sc.IsOriginCommunity {
				t.Error("named community must not be flagged IsOriginCommunity")
			}
		case sc.IsOriginCommunity:
			sawOrigin = true
			if sc.CommunityName != "" {
				t.Errorf("origin community should be nameless, got %q", sc.CommunityName)
			}
		}
	}
	if !sawNamed {
		t.Error("named community missing from SharedCommunities")
	}
	if !sawOrigin {
		t.Error("event's own per-item community was not flagged IsOriginCommunity")
	}
}

// TestGetExperience_InvitedIndividuals verifies that directly-invited
// individuals — members of the event's own ad-hoc origin community who haven't
// responded, excluding the host and responders — surface in InvitedIndividuals
// for the Who's In "Invited" group (#2492).
func TestGetExperience_InvitedIndividuals(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	for _, u := range []struct{ id, email, name string }{
		{"owner1", "owner@example.com", "Owner"},
		{"alice", "alice@example.com", "Alice"},
		{"bob", "bob@example.com", "Bob"},
	} {
		createTestUser(t, testStorage, u.id, u.email, u.name)
	}
	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)
	aliceCtx := createAuthenticatedContext("alice", "alice@example.com", models.Role_ROLE_USER)

	createResp, err := service.SaveExperience(ownerCtx,
		connect.NewRequest(&api.SaveExperienceRequest{Name: "Paddle"}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Resolve the auto-provisioned origin community from the response.
	resp1, err := service.GetExperience(ownerCtx,
		connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
	if err != nil {
		t.Fatalf("GetExperience: %v", err)
	}
	var originID string
	for _, sc := range resp1.Msg.SharedCommunities {
		if sc.IsOriginCommunity {
			originID = sc.CommunityId
		}
	}
	if originID == "" {
		t.Fatal("no origin community on the experience")
	}

	// Directly invite alice + bob into the origin community; alice RSVPs yes,
	// bob stays silent. Only bob should surface as a directly-invited
	// individual (alice responded, owner is the host).
	createTestCommunityMembership(t, testStorage, originID, "alice")
	createTestCommunityMembership(t, testStorage, originID, "bob")
	if _, err := service.RSVPToExperience(aliceCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  originID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	})); err != nil {
		t.Fatalf("RSVP: %v", err)
	}

	resp2, err := service.GetExperience(ownerCtx,
		connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
	if err != nil {
		t.Fatalf("GetExperience: %v", err)
	}
	got := make([]string, 0, len(resp2.Msg.InvitedIndividuals))
	for _, u := range resp2.Msg.InvitedIndividuals {
		got = append(got, u.Id)
	}
	want := []string{"bob"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("InvitedIndividuals = %v, want %v", got, want)
	}
}

// TestGetExperience_SharedCommunitiesAfterTerminalState verifies that
// GetExperience returns the full SharedCommunities list even after the
// experience reaches a terminal state (completed or cancelled). Completion and
// cancellation archive CommunityExperience rows for feed-hiding purposes, but
// that must not erase the access-sheet data visible to the owner.
func TestGetExperience_SharedCommunitiesAfterTerminalState(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "owner1", "owner@example.com", "Owner")
	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)

	communityA := createTestCommunity(t, testStorage, "Community A", "owner1")
	communityB := createTestCommunity(t, testStorage, "Community B", "owner1")
	createTestCommunityMembership(t, testStorage, communityA, "owner1")
	createTestCommunityMembership(t, testStorage, communityB, "owner1")

	setup := func(t *testing.T) string {
		t.Helper()
		createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
			Name: "Shared Experience",
		}))
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}
		expID := createResp.Msg.Experience.Id

		for _, cid := range []string{communityA, communityB} {
			shareExperienceForTest(t, service, ownerCtx, expID, cid)
		}
		return expID
	}

	assertSharedCommunities := func(t *testing.T, expID string, wantCount int) {
		t.Helper()
		resp, err := service.GetExperience(ownerCtx, connect.NewRequest(&api.GetExperienceRequest{
			Id: expID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		if got := len(resp.Msg.SharedCommunities); got != wantCount {
			t.Errorf("want %d SharedCommunities, got %d — access ring/sheet will show wrong count", wantCount, got)
		}
	}

	t.Run("shared communities visible after completion", func(t *testing.T) {
		expID := setup(t)

		_, err := service.MarkExperienceInProcess(ownerCtx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
			ExperienceId: expID,
		}))
		if err != nil {
			t.Fatalf("MarkExperienceInProcess failed: %v", err)
		}

		_, err = service.CompleteExperience(ownerCtx, connect.NewRequest(&api.CompleteExperienceRequest{
			ExperienceId: expID,
		}))
		if err != nil {
			t.Fatalf("CompleteExperience failed: %v", err)
		}

		// 2 explicitly-shared communities (A, B) + the experience's per-item
		// community it was born into at creation (#2492).
		assertSharedCommunities(t, expID, 3)
	})

	t.Run("shared communities visible after cancellation", func(t *testing.T) {
		expID := setup(t)

		_, err := service.CancelExperience(ownerCtx, connect.NewRequest(&api.CancelExperienceRequest{
			ExperienceId: expID,
		}))
		if err != nil {
			t.Fatalf("CancelExperience failed: %v", err)
		}

		// 2 explicitly-shared communities (A, B) + the experience's per-item
		// community it was born into at creation (#2492).
		assertSharedCommunities(t, expID, 3)
	})
}

func TestService_GetExperience(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Create test user
	testUser := &models.User{
		Id:    "user123",
		Email: "test@example.com",
		Name:  "Test User",
	}
	_, err := testStorage.Insert(ctx, testUser)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Create test experience
	testExp := &models.Experience{
		Name:        "Test Hike",
		Description: "A test hiking trip",
		OwnerId:     "user123",
		State:       models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
		Time: &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Tbd{
				Tbd: &models.TimeTBD{},
			},
		},
	}

	expID, err := testStorage.Insert(ctx, testExp)
	if err != nil {
		t.Fatalf("Failed to insert test experience: %v", err)
	}

	communityID := createTestCommunity(t, testStorage, "Test Community", "user123")
	createTestCommunityMembership(t, testStorage, communityID, "user123")

	t.Run("successful retrieval with authentication", func(t *testing.T) {
		req := connect.NewRequest(&api.GetExperienceRequest{
			Id:          expID,
			CommunityId: communityID,
		})

		resp, err := service.GetExperience(ctx, req)
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}

		if resp.Msg.Experience.Id != expID {
			t.Errorf("Expected ID %s, got %s", expID, resp.Msg.Experience.Id)
		}

		if resp.Msg.Experience.Name != testExp.Name {
			t.Errorf("Expected name %s, got %s", testExp.Name, resp.Msg.Experience.Name)
		}

		if resp.Msg.Experience.Owner == nil {
			t.Fatal("Expected Owner to be populated")
		}

		if resp.Msg.Experience.Owner.Id != testExp.OwnerId {
			t.Errorf("Expected Owner.Id %s, got %s", testExp.OwnerId, resp.Msg.Experience.Owner.Id)
		}
	})

	t.Run("experience not found", func(t *testing.T) {
		req := connect.NewRequest(&api.GetExperienceRequest{
			Id: "nonexistent-id",
		})

		_, err := service.GetExperience(ctx, req)
		if err == nil {
			t.Fatal("Expected error for non-existent experience")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connectErr.Code())
		}
	})
}

func TestService_ListExperiences(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	// Create test user
	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Create community
	community := &models.Community{
		Name:        "Test Community",
		CreatorId:   "user123",
		OwnerUserId: "user123",
	}
	communityID, err := testStorage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}

	// Add user as member
	membership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      "user123",
	}
	_, err = testStorage.Insert(ctx, membership)
	if err != nil {
		t.Fatalf("Failed to create membership: %v", err)
	}

	// Create and share experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Shared Experience",
		Description: "A shared experience",
	})
	createResp, err := service.SaveExperience(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ctx, expID, communityID)

	t.Run("list experiences in community", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListExperiencesRequest{
			CommunityId: communityID,
		})

		resp, err := service.ListExperiences(ctx, listReq)
		if err != nil {
			t.Fatalf("ListExperiences failed: %v", err)
		}

		if len(resp.Msg.Experiences) == 0 {
			t.Error("Expected at least 1 experience")
		}

		found := false
		for _, item := range resp.Msg.Experiences {
			if item.Id == expID {
				found = true
				break
			}
		}

		if !found {
			t.Error("Expected to find shared experience in list")
		}
	})
}

func TestService_ListMyExperiences(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	// Create test user
	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Create experiences
	for i := 0; i < 3; i++ {
		createReq := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "My Experience " + string(rune('A'+i)),
			Description: "Test experience",
		})
		_, err := service.SaveExperience(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create experience %d: %v", i, err)
		}
	}

	t.Run("list my experiences", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListMyExperiencesRequest{})

		resp, err := service.ListMyExperiences(ctx, listReq)
		if err != nil {
			t.Fatalf("ListMyExperiences failed: %v", err)
		}

		if len(resp.Msg.Experiences) < 3 {
			t.Errorf("Expected at least 3 experiences, got %d", len(resp.Msg.Experiences))
		}

		// Verify all are owned by user123
		for _, item := range resp.Msg.Experiences {
			if item.Owner.Id != "user123" {
				t.Errorf("Expected all experiences to be owned by user123, got %s", item.Owner.Id)
			}
		}
	})
}

// setupQueryCountScenario creates a community with numExperiences shared experiences,
// each with numRSVPs RSVPs, numProposals time proposals (each with numVotes votes),
// and numMessages chat messages. Returns the community ID and authenticated context.
func setupQueryCountScenario(
	t *testing.T,
	service *Service,
	testStorage *storage.ProtoSQLStorage,
	prefix string,
	numExperiences, numRSVPs, numProposals, numVotes, _ int,
) (communityID string, ownerCtx context.Context) {
	t.Helper()

	ownerID := prefix + "-owner"
	createTestUser(t, testStorage, ownerID, ownerID+"@example.com", "Owner")
	ownerCtx = createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)
	communityID = createTestCommunity(t, testStorage, prefix+" Community", ownerID)
	createTestCommunityMembership(t, testStorage, communityID, ownerID)

	// Pre-create RSVP, proposer, and voter users once (shared across all experiences).
	rsvpCtxs := make([]context.Context, numRSVPs)
	for j := range numRSVPs {
		id := prefix + "-rsvper-" + string(rune('A'+j))
		createTestUser(t, testStorage, id, id+"@example.com", "RSVPer")
		createTestCommunityMembership(t, testStorage, communityID, id)
		rsvpCtxs[j] = createAuthenticatedContext(id, id+"@example.com", models.Role_ROLE_USER)
	}
	proposerCtxs := make([]context.Context, numProposals)
	for j := range numProposals {
		id := prefix + "-proposer-" + string(rune('A'+j))
		createTestUser(t, testStorage, id, id+"@example.com", "Proposer")
		createTestCommunityMembership(t, testStorage, communityID, id)
		ctx := createAuthenticatedContext(id, id+"@example.com", models.Role_ROLE_USER)
		proposerCtxs[j] = ctx
		// RSVP so proposer is a participant.
		_, _ = service.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			CommunityId: communityID,
			Intention:   api.RSVPIntention_RSVP_INTENTION_YES,
		}))
	}
	voterCtxs := make([]context.Context, numVotes)
	for k := range numVotes {
		id := prefix + "-voter-" + string(rune('A'+k))
		createTestUser(t, testStorage, id, id+"@example.com", "Voter")
		createTestCommunityMembership(t, testStorage, communityID, id)
		voterCtxs[k] = createAuthenticatedContext(id, id+"@example.com", models.Role_ROLE_USER)
	}

	for i := range numExperiences {
		saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
			Name: prefix + " Exp",
		}))
		if err != nil {
			t.Fatalf("SaveExperience %d: %v", i, err)
		}
		expID := saveResp.Msg.Experience.Id

		shareExperienceForTest(t, service, ownerCtx, expID, communityID)

		for j, rsvpCtx := range rsvpCtxs {
			_, err := service.RSVPToExperience(rsvpCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
				ExperienceId: expID,
				CommunityId:  communityID,
				Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
			}))
			if err != nil {
				t.Fatalf("RSVPToExperience exp=%d rsvp=%d: %v", i, j, err)
			}
		}

		for j, proposerCtx := range proposerCtxs {
			// RSVP proposer to this experience so they are a participant.
			_, _ = service.RSVPToExperience(proposerCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
				ExperienceId: expID,
				CommunityId:  communityID,
				Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
			}))
			propResp, err := service.ProposeTime(proposerCtx, connect.NewRequest(&api.ProposeTimeRequest{
				ExperienceId: expID,
				Time: &api.ExperienceTime{
					TimeType: &api.ExperienceTime_Tbd{Tbd: &api.TimeTBD{}},
				},
			}))
			if err != nil {
				t.Fatalf("ProposeTime exp=%d proposal=%d: %v", i, j, err)
			}
			proposalID := propResp.Msg.Proposal.Id

			for k, voterCtx := range voterCtxs {
				_, _ = service.VoteOnTime(voterCtx, connect.NewRequest(&api.VoteOnTimeRequest{
					ProposalId: proposalID,
					Status:     api.TimeVoteStatus_TIME_VOTE_STATUS_YES,
				}))
				_ = k
			}
		}
	}
	return communityID, ownerCtx
}

// TestListExperiences_QueryCountBounded verifies that ListExperiences issues a bounded
// number of queries regardless of how many experiences are in the community.
func TestListExperiences_QueryCountBounded(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	communityID, ownerCtx := setupQueryCountScenario(t, service, testStorage,
		"list-exp-qc",
		5, // 5 experiences
		2, // 2 RSVPs each
		1, // 1 time proposal each
		2, // 2 votes per proposal
		0, // no extra messages needed
	)

	statsCtx := storage.WithQueryStats(ownerCtx)
	storage.AssertMaxQueries(t, statsCtx, 12, func() {
		resp, err := service.ListExperiences(statsCtx, connect.NewRequest(&api.ListExperiencesRequest{
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("ListExperiences: %v", err)
		}
		if len(resp.Msg.Experiences) != 5 {
			t.Errorf("expected 5 experiences, got %d", len(resp.Msg.Experiences))
		}
	})
}

// TestListMyExperiences_QueryCountBounded verifies that ListMyExperiences issues a bounded
// number of queries regardless of how many experiences the user owns.
func TestListMyExperiences_QueryCountBounded(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	_, ownerCtx := setupQueryCountScenario(t, service, testStorage,
		"list-my-qc",
		5, // 5 experiences
		2, // 2 RSVPs each
		1, // 1 time proposal each
		2, // 2 votes per proposal
		0, // no extra messages needed
	)

	statsCtx := storage.WithQueryStats(ownerCtx)
	storage.AssertMaxQueries(t, statsCtx, 12, func() {
		resp, err := service.ListMyExperiences(statsCtx, connect.NewRequest(&api.ListMyExperiencesRequest{}))
		if err != nil {
			t.Fatalf("ListMyExperiences: %v", err)
		}
		if len(resp.Msg.Experiences) != 5 {
			t.Errorf("expected 5 experiences, got %d", len(resp.Msg.Experiences))
		}
	})
}

// TestGetExperience_AccessControl verifies that non-members cannot access experiences
// they are not authorized to view and that ex-members are also denied.
func TestGetExperience_AccessControl(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerID := uuid.New().String()
	memberID := uuid.New().String()
	nonMemberID := uuid.New().String()

	createTestUser(t, testStorage, ownerID, ownerID+"@test.com", "Owner")
	createTestUser(t, testStorage, memberID, memberID+"@test.com", "Member")
	createTestUser(t, testStorage, nonMemberID, nonMemberID+"@test.com", "Non-member")

	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@test.com", models.Role_ROLE_USER)
	memberCtx := createAuthenticatedContext(memberID, memberID+"@test.com", models.Role_ROLE_USER)
	nonMemberCtx := createAuthenticatedContext(nonMemberID, nonMemberID+"@test.com", models.Role_ROLE_USER)

	communityID := createTestCommunity(t, testStorage, "Test Community", ownerID)
	createTestCommunityMembership(t, testStorage, communityID, ownerID)
	createTestCommunityMembership(t, testStorage, communityID, memberID)

	// Create and share an experience.
	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Shared Experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	t.Run("member can read shared experience", func(t *testing.T) {
		_, err := service.GetExperience(memberCtx, connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
		if err != nil {
			t.Fatalf("expected success for member, got %v", err)
		}
	})

	t.Run("non-member is denied access to shared experience", func(t *testing.T) {
		_, err := service.GetExperience(nonMemberCtx, connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
		if err == nil {
			t.Fatal("expected PermissionDenied for non-member, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %s", connect.CodeOf(err))
		}
	})

	t.Run("owner can read own unshared experience", func(t *testing.T) {
		saveResp2, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
			Name: "Unshared Experience",
		}))
		if err != nil {
			t.Fatalf("SaveExperience: %v", err)
		}
		_, err = service.GetExperience(ownerCtx, connect.NewRequest(&api.GetExperienceRequest{Id: saveResp2.Msg.Experience.Id}))
		if err != nil {
			t.Fatalf("owner should read own unshared experience, got %v", err)
		}
	})

	t.Run("member sees viewer-scoped SharedCommunities with full total count", func(t *testing.T) {
		communityBID := createTestCommunity(t, testStorage, "Community B", ownerID)
		createTestCommunityMembership(t, testStorage, communityBID, ownerID)
		// memberID is in communityID but NOT in communityBID.

		saveResp3, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
			Name: "Multi-community Experience",
		}))
		if err != nil {
			t.Fatalf("SaveExperience: %v", err)
		}
		expID3 := saveResp3.Msg.Experience.Id
		for _, cid := range []string{communityID, communityBID} {
			shareExperienceForTest(t, service, ownerCtx, expID3, cid)
		}

		// Member of communityID (not communityBID): SharedCommunities contains
		// only their community; TotalSharedCommunityCount reflects the full
		// reach so the client can display "shared in 2 communities" without
		// receiving community B's name or avatar.
		resp, err := service.GetExperience(memberCtx, connect.NewRequest(&api.GetExperienceRequest{Id: expID3}))
		if err != nil {
			t.Fatalf("GetExperience for member: %v", err)
		}
		if got := len(resp.Msg.SharedCommunities); got != 1 {
			t.Errorf("member should see 1 (viewer-scoped) community in SharedCommunities, got %d", got)
		}
		// Full reach: communityID + communityBID + the experience's per-item
		// community it was born into at creation (#2492) = 3.
		if got := resp.Msg.TotalSharedCommunityCount; got != 3 {
			t.Errorf("TotalSharedCommunityCount should be 3 (full reach), got %d", got)
		}

		// Owner sees all communities in SharedCommunities (owner bypass gives full set):
		// communityID + communityBID + the per-item community (#2492) = 3.
		ownerResp, err := service.GetExperience(ownerCtx, connect.NewRequest(&api.GetExperienceRequest{Id: expID3}))
		if err != nil {
			t.Fatalf("GetExperience for owner: %v", err)
		}
		if got := len(ownerResp.Msg.SharedCommunities); got != 3 {
			t.Errorf("owner should see all 3 communities in SharedCommunities, got %d", got)
		}
	})
}

// TestListExperiences_AccessControl verifies that non-members cannot list
// experiences in a community they don't belong to.
func TestListExperiences_AccessControl(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerID := uuid.New().String()
	nonMemberID := uuid.New().String()

	createTestUser(t, testStorage, ownerID, ownerID+"@test.com", "Owner")
	createTestUser(t, testStorage, nonMemberID, nonMemberID+"@test.com", "Non-member")

	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@test.com", models.Role_ROLE_USER)
	nonMemberCtx := createAuthenticatedContext(nonMemberID, nonMemberID+"@test.com", models.Role_ROLE_USER)

	communityID := createTestCommunity(t, testStorage, "Test Community", ownerID)
	createTestCommunityMembership(t, testStorage, communityID, ownerID)

	t.Run("non-member cannot list community experiences", func(t *testing.T) {
		_, err := service.ListExperiences(nonMemberCtx, connect.NewRequest(&api.ListExperiencesRequest{
			CommunityId: communityID,
		}))
		if err == nil {
			t.Fatal("expected PermissionDenied for non-member, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %s", connect.CodeOf(err))
		}
	})

	t.Run("member can list community experiences", func(t *testing.T) {
		_, err := service.ListExperiences(ownerCtx, connect.NewRequest(&api.ListExperiencesRequest{
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("expected success for owner/member, got %v", err)
		}
	})
}
