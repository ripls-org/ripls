package community

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// countCommunityEvents returns how many persisted CommunityEvent rows for the
// community match the given type.
func countCommunityEvents(t *testing.T, st *storage.ProtoSQLStorage, communityID string, et models.CommunityEventType) int {
	t.Helper()
	rows, err := st.QueryByField(context.Background(), "community_id", communityID, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("query community events: %v", err)
	}
	n := 0
	for _, r := range rows {
		if r.(*models.CommunityEvent).EventType == et {
			n++
		}
	}
	return n
}

func TestProvisionAdHocCommunity(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	t.Run("provisions a nameless community with origin and host membership", func(t *testing.T) {
		hostID := setupTestUser(t, testStorage, "host@example.com", "Host")
		ctx := createAuthenticatedContext(hostID, "host@example.com", models.Role_ROLE_USER)

		id, err := service.ProvisionAdHocCommunity(ctx, hostID, AdHocOrigin{ExperienceID: "exp-1"}, AdHocAudience{})
		if err != nil {
			t.Fatalf("ProvisionAdHocCommunity failed: %v", err)
		}
		if id == "" {
			t.Fatal("expected a non-empty community id")
		}

		community := &models.Community{}
		if err := testStorage.GetByID(ctx, id, community); err != nil {
			t.Fatalf("get provisioned community: %v", err)
		}
		if community.Name != "" {
			t.Errorf("expected ad-hoc community to be nameless, got name %q", community.Name)
		}
		if !IsAdHoc(community) {
			t.Error("expected IsAdHoc to be true for a nameless community")
		}
		if community.GetOriginExperienceId() != "exp-1" {
			t.Errorf("expected origin_experience_id 'exp-1', got %q", community.GetOriginExperienceId())
		}
		if community.OwnerUserId != hostID || community.CreatorId != hostID {
			t.Errorf("expected host %s to be creator+owner, got creator=%s owner=%s", hostID, community.CreatorId, community.OwnerUserId)
		}

		// Host is a member.
		members, err := testStorage.QueryByField(ctx, "community_id", id, &models.CommunityUser{})
		if err != nil {
			t.Fatalf("query members: %v", err)
		}
		if len(members) != 1 {
			t.Fatalf("expected 1 member (host), got %d", len(members))
		}
		if members[0].(*models.CommunityUser).UserId != hostID {
			t.Errorf("expected host to be the member, got %s", members[0].(*models.CommunityUser).UserId)
		}

		// Creation event fired, no premature naming event.
		if got := countCommunityEvents(t, testStorage, id, models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_CREATED); got != 1 {
			t.Errorf("expected 1 COMMUNITY_CREATED event, got %d", got)
		}
		if got := countCommunityEvents(t, testStorage, id, models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED); got != 0 {
			t.Errorf("expected 0 COMMUNITY_NAMED events on a fresh ad-hoc community, got %d", got)
		}
	})

	t.Run("seeds extra members and dedupes the host", func(t *testing.T) {
		hostID := setupTestUser(t, testStorage, "host2@example.com", "Host Two")
		memberA := setupTestUser(t, testStorage, "a@example.com", "Member A")
		memberB := setupTestUser(t, testStorage, "b@example.com", "Member B")
		ctx := createAuthenticatedContext(hostID, "host2@example.com", models.Role_ROLE_USER)

		// Pass the host again (and a blank) among the extras — both should be
		// deduped/ignored so the uniqueness constraint isn't tripped.
		id, err := service.ProvisionAdHocCommunity(ctx, hostID, AdHocOrigin{GearID: "gear-1"}, AdHocAudience{MemberUserIDs: []string{memberA, memberB, hostID, ""}})
		if err != nil {
			t.Fatalf("ProvisionAdHocCommunity failed: %v", err)
		}

		members, err := testStorage.QueryByField(ctx, "community_id", id, &models.CommunityUser{})
		if err != nil {
			t.Fatalf("query members: %v", err)
		}
		got := map[string]bool{}
		for _, m := range members {
			got[m.(*models.CommunityUser).UserId] = true
		}
		if len(members) != 3 || !got[hostID] || !got[memberA] || !got[memberB] {
			t.Errorf("expected exactly {host, memberA, memberB}, got %d members %v", len(members), got)
		}

		community := &models.Community{}
		if err := testStorage.GetByID(ctx, id, community); err != nil {
			t.Fatalf("get community: %v", err)
		}
		if community.GetOriginGearId() != "gear-1" {
			t.Errorf("expected origin_gear_id 'gear-1', got %q", community.GetOriginGearId())
		}
	})

	t.Run("rejects an origin with no item id set", func(t *testing.T) {
		hostID := setupTestUser(t, testStorage, "host3@example.com", "Host Three")
		ctx := createAuthenticatedContext(hostID, "host3@example.com", models.Role_ROLE_USER)

		_, err := service.ProvisionAdHocCommunity(ctx, hostID, AdHocOrigin{}, AdHocAudience{})
		if err == nil {
			t.Fatal("expected an error when no origin id is set")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", connect.CodeOf(err))
		}
	})

	t.Run("rejects an empty host", func(t *testing.T) {
		ctx := context.Background()
		if _, err := service.ProvisionAdHocCommunity(ctx, "", AdHocOrigin{RequestID: "req-1"}, AdHocAudience{}); err == nil {
			t.Fatal("expected an error when host id is empty")
		}
	})
}

// TestProvisionAdHocCommunity_Dedup covers the reviewer's "three events with the
// same three people shouldn't be three communities" requirement: a second item
// with the same audience reuses the first community (keeping its origin), while
// a different audience or a host-only item gets its own.
func TestProvisionAdHocCommunity_Dedup(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	host := setupTestUser(t, testStorage, "dedup-host@example.com", "Dedup Host")
	memberA := setupTestUser(t, testStorage, "dedup-a@example.com", "A")
	memberB := setupTestUser(t, testStorage, "dedup-b@example.com", "B")
	ctx := createAuthenticatedContext(host, "dedup-host@example.com", models.Role_ROLE_USER)

	countAdHoc := func() int {
		t.Helper()
		rows, err := storage.QueryByField[*models.Community](testStorage, ctx, "owner_user_id", host)
		if err != nil {
			t.Fatalf("query communities: %v", err)
		}
		n := 0
		for _, c := range rows {
			if IsAdHoc(c) {
				n++
			}
		}
		return n
	}

	t.Run("same real-user audience reuses the community and keeps the first origin", func(t *testing.T) {
		first, err := service.ProvisionAdHocCommunity(ctx, host, AdHocOrigin{ExperienceID: "exp-A"}, AdHocAudience{MemberUserIDs: []string{memberA, memberB}})
		if err != nil {
			t.Fatalf("first provision: %v", err)
		}
		// A second item for the same {host, A, B} audience — order shuffled to
		// prove the match is set-based — must resolve to the SAME community.
		second, err := service.ProvisionAdHocCommunity(ctx, host, AdHocOrigin{ExperienceID: "exp-B"}, AdHocAudience{MemberUserIDs: []string{memberB, memberA}})
		if err != nil {
			t.Fatalf("second provision: %v", err)
		}
		if second != first {
			t.Fatalf("expected the same audience to reuse community %s, got a new one %s", first, second)
		}
		// Origin stays the first item; the duplicate item does not overwrite it.
		community := &models.Community{}
		if err := testStorage.GetByID(ctx, first, community); err != nil {
			t.Fatalf("get community: %v", err)
		}
		if community.GetOriginExperienceId() != "exp-A" {
			t.Errorf("expected reused community to keep origin 'exp-A', got %q", community.GetOriginExperienceId())
		}
	})

	t.Run("a different audience gets its own community", func(t *testing.T) {
		before := countAdHoc()
		id, err := service.ProvisionAdHocCommunity(ctx, host, AdHocOrigin{ExperienceID: "exp-C"}, AdHocAudience{MemberUserIDs: []string{memberA}})
		if err != nil {
			t.Fatalf("provision: %v", err)
		}
		if id == "" {
			t.Fatal("expected a community id")
		}
		if got := countAdHoc(); got != before+1 {
			t.Errorf("expected a new community for a different audience (count %d→%d), got %d", before, before+1, got)
		}
	})

	t.Run("same provisional handles dedup even with no real-user members", func(t *testing.T) {
		aud := AdHocAudience{ContactHandles: []string{"+15551112222", "+15553334444"}}
		first, err := service.ProvisionAdHocCommunity(ctx, host, AdHocOrigin{ExperienceID: "exp-D"}, aud)
		if err != nil {
			t.Fatalf("first provision: %v", err)
		}
		second, err := service.ProvisionAdHocCommunity(ctx, host, AdHocOrigin{ExperienceID: "exp-E"}, AdHocAudience{ContactHandles: []string{"+15553334444", "+15551112222"}})
		if err != nil {
			t.Fatalf("second provision: %v", err)
		}
		if second != first {
			t.Errorf("expected identical phone-invite audiences to reuse community %s, got %s", first, second)
		}
	})

	t.Run("host-only (no invitees) items are never deduped together", func(t *testing.T) {
		first, err := service.ProvisionAdHocCommunity(ctx, host, AdHocOrigin{ExperienceID: "exp-F"}, AdHocAudience{})
		if err != nil {
			t.Fatalf("first provision: %v", err)
		}
		second, err := service.ProvisionAdHocCommunity(ctx, host, AdHocOrigin{ExperienceID: "exp-G"}, AdHocAudience{})
		if err != nil {
			t.Fatalf("second provision: %v", err)
		}
		if second == first {
			t.Error("expected two audience-less ad-hoc items to be distinct communities, but they were deduped")
		}
	})
}

func TestUpdateCommunity_PromoteByNaming(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	t.Run("naming an ad-hoc community promotes it and emits COMMUNITY_NAMED once", func(t *testing.T) {
		hostID := setupTestUser(t, testStorage, "promote-host@example.com", "Promote Host")
		ctx := createAuthenticatedContext(hostID, "promote-host@example.com", models.Role_ROLE_USER)

		id, err := service.ProvisionAdHocCommunity(ctx, hostID, AdHocOrigin{ExperienceID: "exp-promote"}, AdHocAudience{})
		if err != nil {
			t.Fatalf("provision: %v", err)
		}

		// Name it → promotion.
		if _, err := service.UpdateCommunity(ctx, connect.NewRequest(&api.UpdateCommunityRequest{
			Id:   id,
			Name: "Trail Crew",
		})); err != nil {
			t.Fatalf("UpdateCommunity (naming): %v", err)
		}

		community := &models.Community{}
		if err := testStorage.GetByID(ctx, id, community); err != nil {
			t.Fatalf("get community: %v", err)
		}
		if community.Name != "Trail Crew" {
			t.Errorf("expected name 'Trail Crew', got %q", community.Name)
		}
		if IsAdHoc(community) {
			t.Error("expected the community to no longer be ad-hoc after naming")
		}
		// Origin provenance survives promotion.
		if community.GetOriginExperienceId() != "exp-promote" {
			t.Errorf("expected origin to survive promotion, got %q", community.GetOriginExperienceId())
		}
		if got := countCommunityEvents(t, testStorage, id, models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED); got != 1 {
			t.Fatalf("expected exactly 1 COMMUNITY_NAMED event, got %d", got)
		}

		// A second update (renaming an already-named community) must not re-emit.
		if _, err := service.UpdateCommunity(ctx, connect.NewRequest(&api.UpdateCommunityRequest{
			Id:   id,
			Name: "Trail Crew 2",
		})); err != nil {
			t.Fatalf("UpdateCommunity (rename): %v", err)
		}
		if got := countCommunityEvents(t, testStorage, id, models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED); got != 1 {
			t.Errorf("expected COMMUNITY_NAMED to stay at 1 after a rename, got %d", got)
		}
	})

	t.Run("updating an ad-hoc community without a name does not promote it", func(t *testing.T) {
		hostID := setupTestUser(t, testStorage, "desc-host@example.com", "Desc Host")
		ctx := createAuthenticatedContext(hostID, "desc-host@example.com", models.Role_ROLE_USER)

		id, err := service.ProvisionAdHocCommunity(ctx, hostID, AdHocOrigin{RequestID: "req-desc"}, AdHocAudience{})
		if err != nil {
			t.Fatalf("provision: %v", err)
		}

		if _, err := service.UpdateCommunity(ctx, connect.NewRequest(&api.UpdateCommunityRequest{
			Id:          id,
			Description: "still figuring out a name",
		})); err != nil {
			t.Fatalf("UpdateCommunity (description only): %v", err)
		}

		community := &models.Community{}
		if err := testStorage.GetByID(ctx, id, community); err != nil {
			t.Fatalf("get community: %v", err)
		}
		if !IsAdHoc(community) {
			t.Error("expected community to stay ad-hoc when only description changed")
		}
		if got := countCommunityEvents(t, testStorage, id, models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED); got != 0 {
			t.Errorf("expected 0 COMMUNITY_NAMED events, got %d", got)
		}
	})
}
