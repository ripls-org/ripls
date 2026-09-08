package community

import (
	"context"
	"math"
	"slices"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestService_ListCommunities(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	user1ID := setupTestUser(t, testStorage, "user1@example.com", "User One")
	user2ID := setupTestUser(t, testStorage, "user2@example.com", "User Two")

	ctx1 := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)
	ctx2 := createAuthenticatedContext(user2ID, "user2@example.com", models.Role_ROLE_USER)

	// Create communities for user1
	createReq1 := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "User1 Community",
	})
	resp1, err := service.CreateCommunity(ctx1, createReq1)
	if err != nil {
		t.Fatalf("Failed to create community 1: %v", err)
	}

	// Create community for user2
	createReq2 := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "User2 Community",
	})
	resp2, err := service.CreateCommunity(ctx2, createReq2)
	if err != nil {
		t.Fatalf("Failed to create community 2: %v", err)
	}

	t.Run("list member communities", func(t *testing.T) {
		req := connect.NewRequest(&api.ListCommunitiesRequest{})

		resp, err := service.ListCommunities(ctx1, req)
		if err != nil {
			t.Fatalf("ListCommunities failed: %v", err)
		}

		// User1 should only see their own community
		if len(resp.Msg.Communities) != 1 {
			t.Fatalf("Expected 1 community, got %d", len(resp.Msg.Communities))
		}

		if resp.Msg.Communities[0].Id != resp1.Msg.Id {
			t.Errorf("Expected community ID %s, got %s", resp1.Msg.Id, resp.Msg.Communities[0].Id)
		}
		if resp.Msg.Communities[0].OwnerUserId != user1ID {
			t.Errorf("Expected OwnerUserId %s, got %q", user1ID, resp.Msg.Communities[0].OwnerUserId)
		}
	})

	t.Run("member sees correct communities after joining", func(t *testing.T) {
		// Add user1 to user2's community using invitation link
		addUserToCommunity(t, service, resp2.Msg.Id, user2ID, user1ID, "user2@example.com", "user1@example.com")

		// Now user1 should see both communities
		listReq := connect.NewRequest(&api.ListCommunitiesRequest{})
		listResp, err := service.ListCommunities(ctx1, listReq)
		if err != nil {
			t.Fatalf("ListCommunities failed: %v", err)
		}

		if len(listResp.Msg.Communities) != 2 {
			t.Fatalf("Expected 2 communities, got %d", len(listResp.Msg.Communities))
		}
	})
}

// TestListCommunities_AdHocVisibility covers the picker-clutter rule (#2492):
// host-only nameless (ad-hoc/per-item) communities are suppressed, while
// nameless communities with other members surface with a member count and a
// first-name preview so the client can render them like a group chat. Named
// communities are unaffected.
func TestListCommunities_AdHocVisibility(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	bobID := setupTestUser(t, testStorage, "bob@example.com", "Bob Jones")
	carolID := setupTestUser(t, testStorage, "carol@example.com", "Carol Lee")
	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)

	named, err := service.CreateCommunity(ctxAlice,
		connect.NewRequest(&api.CreateCommunityRequest{Name: "Book Club"}))
	if err != nil {
		t.Fatalf("create named community: %v", err)
	}

	// Host-only nameless community (no audience) — pure per-item clutter.
	soloID, err := service.ProvisionAdHocCommunity(ctxAlice, aliceID,
		AdHocOrigin{ExperienceID: "exp-solo"}, AdHocAudience{})
	if err != nil {
		t.Fatalf("provision host-only ad-hoc community: %v", err)
	}

	// Nameless community with two other members — a real group.
	groupID, err := service.ProvisionAdHocCommunity(ctxAlice, aliceID,
		AdHocOrigin{ExperienceID: "exp-group"},
		AdHocAudience{MemberUserIDs: []string{bobID, carolID}})
	if err != nil {
		t.Fatalf("provision multi-member ad-hoc community: %v", err)
	}

	resp, err := service.ListCommunities(ctxAlice,
		connect.NewRequest(&api.ListCommunitiesRequest{}))
	if err != nil {
		t.Fatalf("ListCommunities failed: %v", err)
	}

	byID := make(map[string]*api.CommunityItem, len(resp.Msg.Communities))
	for _, c := range resp.Msg.Communities {
		byID[c.Id] = c
	}

	if _, ok := byID[soloID]; ok {
		t.Error("host-only nameless community should be suppressed from ListCommunities")
	}
	if len(resp.Msg.Communities) != 2 {
		t.Fatalf("expected 2 communities (named + group), got %d", len(resp.Msg.Communities))
	}

	if c := byID[named.Msg.Id]; c == nil {
		t.Error("named community missing from list")
	} else {
		if c.Name != "Book Club" {
			t.Errorf("named community name = %q, want %q", c.Name, "Book Club")
		}
		if c.MemberCount != 1 {
			t.Errorf("named community member_count = %d, want 1", c.MemberCount)
		}
		if len(c.MemberPreviewFirstNames) != 0 {
			t.Errorf("named community should carry no preview names, got %v", c.MemberPreviewFirstNames)
		}
	}

	if c := byID[groupID]; c == nil {
		t.Error("multi-member nameless community missing from list")
	} else {
		if c.Name != "" {
			t.Errorf("ad-hoc community should be nameless, got %q", c.Name)
		}
		if c.MemberCount != 3 {
			t.Errorf("group member_count = %d, want 3", c.MemberCount)
		}
		want := []string{"Bob", "Carol"} // excludes the caller (Alice), sorted
		if !slices.Equal(c.MemberPreviewFirstNames, want) {
			t.Errorf("group member_preview_first_names = %v, want %v", c.MemberPreviewFirstNames, want)
		}
	}
}

// TestListCommunities_OriginItemName covers the per-item community labeling
// (#2492): a nameless ad-hoc community carries origin_item_name = the spawning
// item's display name (experience/gear name, request title, or — for a
// transfer — the gear behind it) so the client can render "Group from {item}".
// Named communities never carry it.
func TestListCommunities_OriginItemName(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	bobID := setupTestUser(t, testStorage, "bob@example.com", "Bob Jones")
	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)

	// Insert the four kinds of spawning item.
	expID, err := testStorage.Insert(ctxAlice, &models.Experience{OwnerId: aliceID, Name: "Dinner at Este"})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	gearID, err := testStorage.Insert(ctxAlice, &models.Gear{OwnerId: aliceID, Name: "Camp Stove"})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	reqID, err := testStorage.Insert(ctxAlice, &models.Request{Title: "Need a ladder"})
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	// A transfer has no name of its own — its label comes from the gear behind it.
	loanedGearID, err := testStorage.Insert(ctxAlice, &models.Gear{OwnerId: aliceID, Name: "Loaned Tent"})
	if err != nil {
		t.Fatalf("insert loaned gear: %v", err)
	}
	transferID, err := testStorage.Insert(ctxAlice, &models.Transfer{OwnerId: aliceID, GearId: loanedGearID})
	if err != nil {
		t.Fatalf("insert transfer: %v", err)
	}

	// A nameless per-item community per origin kind, each with a second member so
	// it isn't suppressed as host-only.
	origins := []struct {
		origin AdHocOrigin
		want   string
	}{
		{AdHocOrigin{ExperienceID: expID}, "Dinner at Este"},
		{AdHocOrigin{GearID: gearID}, "Camp Stove"},
		{AdHocOrigin{RequestID: reqID}, "Need a ladder"},
		{AdHocOrigin{TransferID: transferID}, "Loaned Tent"},
	}
	wantByCommunity := make(map[string]string)
	for _, o := range origins {
		cid, provErr := service.ProvisionAdHocCommunity(ctxAlice, aliceID, o.origin,
			AdHocAudience{MemberUserIDs: []string{bobID}})
		if provErr != nil {
			t.Fatalf("provision ad-hoc community for %+v: %v", o.origin, provErr)
		}
		wantByCommunity[cid] = o.want
	}

	// A named community with an origin must NOT surface an origin label.
	namedID, err := service.ProvisionAdHocCommunity(ctxAlice, aliceID,
		AdHocOrigin{ExperienceID: expID}, AdHocAudience{MemberUserIDs: []string{bobID}})
	if err != nil {
		t.Fatalf("provision community to name: %v", err)
	}
	if _, err := service.UpdateCommunity(ctxAlice, connect.NewRequest(&api.UpdateCommunityRequest{
		Id:   namedID,
		Name: "Dinner Crew",
	})); err != nil {
		t.Fatalf("name community: %v", err)
	}
	wantByCommunity[namedID] = "" // named → no origin label

	resp, err := service.ListCommunities(ctxAlice,
		connect.NewRequest(&api.ListCommunitiesRequest{}))
	if err != nil {
		t.Fatalf("ListCommunities: %v", err)
	}

	byID := make(map[string]*api.CommunityItem, len(resp.Msg.Communities))
	for _, c := range resp.Msg.Communities {
		byID[c.Id] = c
	}
	for cid, want := range wantByCommunity {
		c := byID[cid]
		if c == nil {
			t.Errorf("community %s missing from list", cid)
			continue
		}
		if c.OriginItemName != want {
			t.Errorf("community %s origin_item_name = %q, want %q", cid, c.OriginItemName, want)
		}
	}
}

// TestListCommunities_NamedSortBeforeNameless covers the Workshop-carousel
// ordering rule (#2492): every named community sorts ahead of every nameless
// (ad-hoc) community, regardless of activity, so the "needs a name" entries
// collect at the end of the list where the owner can promote them.
func TestListCommunities_NamedSortBeforeNameless(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	bobID := setupTestUser(t, testStorage, "bob@example.com", "Bob Jones")
	carolID := setupTestUser(t, testStorage, "carol@example.com", "Carol Lee")
	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)

	// Two named communities.
	for _, name := range []string{"Book Club", "Trail Crew"} {
		if _, err := service.CreateCommunity(ctxAlice,
			connect.NewRequest(&api.CreateCommunityRequest{Name: name})); err != nil {
			t.Fatalf("create named community %q: %v", name, err)
		}
	}

	// Two multi-member nameless (ad-hoc) communities. They need *distinct*
	// audiences, else member_signature dedup reuses the first (#2492 COMM-1).
	adHoc := []struct {
		origin   string
		audience []string
	}{
		{"exp-a", []string{bobID}},
		{"exp-b", []string{carolID}},
	}
	for _, a := range adHoc {
		if _, err := service.ProvisionAdHocCommunity(ctxAlice, aliceID,
			AdHocOrigin{ExperienceID: a.origin},
			AdHocAudience{MemberUserIDs: a.audience}); err != nil {
			t.Fatalf("provision ad-hoc community %q: %v", a.origin, err)
		}
	}

	resp, err := service.ListCommunities(ctxAlice,
		connect.NewRequest(&api.ListCommunitiesRequest{}))
	if err != nil {
		t.Fatalf("ListCommunities failed: %v", err)
	}
	if len(resp.Msg.Communities) != 4 {
		t.Fatalf("expected 4 communities, got %d", len(resp.Msg.Communities))
	}

	// Once a nameless community appears, no named community may follow it.
	sawNameless := false
	for i, c := range resp.Msg.Communities {
		if c.Name == "" {
			sawNameless = true
			continue
		}
		if sawNameless {
			t.Errorf("named community %q at index %d follows a nameless one — named must sort first",
				c.Name, i)
		}
	}
}

// TestListCommunities_GearCountAndArea covers the Library location sheet
// fields (#2634): gear_count counts every non-archived share whose gear still
// exists (geocoded or not), and the area anchor is the centroid of the
// geocoded gear's coordinates — absent when no shared gear is geocoded.
// Archived community-gear rows contribute to neither.
func TestListCommunities_GearCountAndArea(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)

	mkCommunity := func(name string) string {
		resp, err := service.CreateCommunity(ctxAlice,
			connect.NewRequest(&api.CreateCommunityRequest{Name: name}))
		if err != nil {
			t.Fatalf("create community %q: %v", name, err)
		}
		return resp.Msg.Id
	}
	gearCommunityID := mkCommunity("Gear Rich")
	emptyCommunityID := mkCommunity("Gear Free")

	mkLocation := func(lat, lng float64) string {
		id, err := testStorage.Insert(ctxAlice, &models.Location{
			Geolocation: &models.Geolocation{LatitudeDeg: lat, LongitudeDeg: lng},
		})
		if err != nil {
			t.Fatalf("insert location (%v, %v): %v", lat, lng, err)
		}
		return id
	}
	mkGear := func(name, locationID string) string {
		id, err := testStorage.Insert(ctxAlice, &models.Gear{
			OwnerId:    aliceID,
			Name:       name,
			LocationId: locationID,
		})
		if err != nil {
			t.Fatalf("insert gear %q: %v", name, err)
		}
		return id
	}
	shareGear := func(communityID, gearID string, archived bool) {
		if _, err := testStorage.Insert(ctxAlice, &models.CommunityGear{
			CommunityId:  communityID,
			GearId:       gearID,
			Availability: models.Availability_AVAILABILITY_FOR_LOAN,
			Archived:     archived,
		}); err != nil {
			t.Fatalf("share gear %s into %s: %v", gearID, communityID, err)
		}
	}

	listItem := func(communityID string) *api.CommunityItem {
		t.Helper()
		resp, err := service.ListCommunities(ctxAlice,
			connect.NewRequest(&api.ListCommunitiesRequest{}))
		if err != nil {
			t.Fatalf("ListCommunities: %v", err)
		}
		for _, c := range resp.Msg.Communities {
			if c.Id == communityID {
				return c
			}
		}
		t.Fatalf("community %s missing from list", communityID)
		return nil
	}
	assertArea := func(c *api.CommunityItem, wantLat, wantLng float64) {
		t.Helper()
		if c.AreaLatitudeDeg == nil || c.AreaLongitudeDeg == nil {
			t.Fatalf("area = (%v, %v), want (%v, %v)",
				c.AreaLatitudeDeg, c.AreaLongitudeDeg, wantLat, wantLng)
		}
		const eps = 1e-9
		if math.Abs(*c.AreaLatitudeDeg-wantLat) > eps || math.Abs(*c.AreaLongitudeDeg-wantLng) > eps {
			t.Errorf("area = (%v, %v), want (%v, %v)",
				*c.AreaLatitudeDeg, *c.AreaLongitudeDeg, wantLat, wantLng)
		}
	}

	// Two geocoded gear whose centroid is (40.5, -104.5).
	shareGear(gearCommunityID, mkGear("Tent", mkLocation(40.0, -105.0)), false)
	shareGear(gearCommunityID, mkGear("Stove", mkLocation(41.0, -104.0)), false)

	t.Run("two geocoded gear yield centroid and count", func(t *testing.T) {
		c := listItem(gearCommunityID)
		if c.GearCount != 2 {
			t.Errorf("gear_count = %d, want 2", c.GearCount)
		}
		assertArea(c, 40.5, -104.5)
	})

	t.Run("non-geocoded gear counts but does not move the centroid", func(t *testing.T) {
		shareGear(gearCommunityID, mkGear("Ladder", ""), false)

		c := listItem(gearCommunityID)
		if c.GearCount != 3 {
			t.Errorf("gear_count = %d, want 3", c.GearCount)
		}
		assertArea(c, 40.5, -104.5)
	})

	t.Run("archived rows excluded from count and centroid", func(t *testing.T) {
		// Far-away geocoded gear behind an archived share: were it counted,
		// both the count and the centroid would move.
		shareGear(gearCommunityID, mkGear("Given Away", mkLocation(10.0, 10.0)), true)

		c := listItem(gearCommunityID)
		if c.GearCount != 3 {
			t.Errorf("gear_count = %d, want 3", c.GearCount)
		}
		assertArea(c, 40.5, -104.5)
	})

	t.Run("community with no gear has zero count and absent area", func(t *testing.T) {
		c := listItem(emptyCommunityID)
		if c.GearCount != 0 {
			t.Errorf("gear_count = %d, want 0", c.GearCount)
		}
		if c.AreaLatitudeDeg != nil || c.AreaLongitudeDeg != nil {
			t.Errorf("area = (%v, %v), want absent", c.AreaLatitudeDeg, c.AreaLongitudeDeg)
		}
	})
}

// TestFirstName covers the first-token extraction used for ad-hoc community
// name previews.
func TestFirstName(t *testing.T) {
	cases := map[string]string{
		"Alice Smith":     "Alice",
		"Bob":             "Bob",
		"  Carol   Lee  ": "Carol",
		"":                "",
		"   ":             "",
	}
	for in, want := range cases {
		if got := firstName(in); got != want {
			t.Errorf("firstName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestFirstFewMemberFirstNames verifies the preview excludes the caller, skips
// members with a missing record or blank name, sorts for stability, and caps
// at the limit.
func TestFirstFewMemberFirstNames(t *testing.T) {
	users := map[string]*models.User{
		"self":   {Id: "self", Name: "Me Myself"},
		"u1":     {Id: "u1", Name: "Alice Smith"},
		"u2":     {Id: "u2", Name: "Bob Jones"},
		"u3":     {Id: "u3", Name: "Carol Lee"},
		"u4":     {Id: "u4", Name: "Dave Park"},
		"noname": {Id: "noname", Name: ""},
	}
	memberIDs := []string{"self", "u3", "u1", "u2", "u4", "noname", "missing"}
	got := firstFewMemberFirstNames(memberIDs, "self", users, memberPreviewLimit)
	want := []string{"Alice", "Bob", "Carol"}
	if !slices.Equal(got, want) {
		t.Errorf("firstFewMemberFirstNames = %v, want %v", got, want)
	}
}

// TestListCommunitiesQueryCount verifies that ListCommunities uses exactly 2
// queries regardless of community count, preventing N+1 regressions.
func TestListCommunitiesQueryCount(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	ctx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Create 5 communities so any N+1 would be visible.
	const numCommunities = 5
	for i := range numCommunities {
		req := connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Community " + string(rune('A'+i)),
		})
		if _, err := service.CreateCommunity(ctx, req); err != nil {
			t.Fatalf("failed to create community %d: %v", i, err)
		}
	}

	statsCtx := storage.WithQueryStats(ctx)
	var listErr error
	// Expect 5 queries: memberships, batch GetByIDs for communities, the batch
	// member load (member counts + ad-hoc name previews), the batch
	// community-gear load (gear count + area anchor, #2634), and the GROUP BY
	// for the recency sort (#1898). The preview-user fetch is skipped here
	// because all these communities are named, and the gear/location batch
	// fetches are skipped because no gear is shared.
	storage.AssertMaxQueries(t, statsCtx, 5, func() {
		_, listErr = service.ListCommunities(statsCtx, connect.NewRequest(&api.ListCommunitiesRequest{}))
	})
	if listErr != nil {
		t.Fatalf("ListCommunities failed: %v", listErr)
	}
}

// TestListCommunitiesSortedByActivity asserts the Workshop carousel's
// ordering contract (#1898): communities with the most recent
// CommunityEvent surface first, ones with no events sink to the end.
func TestListCommunitiesSortedByActivity(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	ctx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	mk := func(name string) string {
		resp, err := service.CreateCommunity(ctx,
			connect.NewRequest(&api.CreateCommunityRequest{Name: name}))
		if err != nil {
			t.Fatalf("CreateCommunity %q failed: %v", name, err)
		}
		return resp.Msg.Id
	}
	oldID := mk("Old Activity")
	recentID := mk("Recent Activity")
	silentID := mk("No Activity")

	event := func(communityID string, atUnixSec int64) {
		evt := &models.CommunityEvent{
			Id:                uuid.New().String(),
			CommunityId:       communityID,
			EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			OccurredAtUnixSec: atUnixSec,
		}
		if _, err := testStorage.Insert(ctx, evt); err != nil {
			t.Fatalf("insert CommunityEvent: %v", err)
		}
	}
	// CreateCommunity emits a COMMUNITY_CREATED event of its own, so
	// seed test events with very large timestamps to guarantee they
	// dominate the per-community MAX regardless of the creation
	// event's wall-clock time.
	const farFuture = 9_000_000_000 // year 2255
	event(oldID, farFuture)
	event(recentID, farFuture+1_000)

	resp, err := service.ListCommunities(ctx,
		connect.NewRequest(&api.ListCommunitiesRequest{}))
	if err != nil {
		t.Fatalf("ListCommunities failed: %v", err)
	}
	if len(resp.Msg.Communities) != 3 {
		t.Fatalf("expected 3 communities, got %d", len(resp.Msg.Communities))
	}
	got := []string{
		resp.Msg.Communities[0].Id,
		resp.Msg.Communities[1].Id,
		resp.Msg.Communities[2].Id,
	}
	want := []string{recentID, oldID, silentID}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: got %s, want %s (full order: %v)",
				i, got[i], want[i], got)
		}
	}
}

func TestService_ListDeletedCommunitiesForRestore(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice")
	bobID := setupTestUser(t, testStorage, "bob@example.com", "Bob")

	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)
	ctxBob := createAuthenticatedContext(bobID, "bob@example.com", models.Role_ROLE_USER)

	// Alice creates a community and adds Bob.
	createReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Test Community",
	})
	createResp, err := service.CreateCommunity(ctxAlice, createReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createResp.Msg.Id
	addUserToCommunity(t, service, communityID, aliceID, bobID, "alice@example.com", "bob@example.com")

	t.Run("active community is not in either user's list", func(t *testing.T) {
		for _, ctx := range []context.Context{ctxAlice, ctxBob} {
			resp, err := service.ListDeletedCommunitiesForRestore(ctx,
				connect.NewRequest(&api.ListDeletedCommunitiesForRestoreRequest{}))
			if err != nil {
				t.Fatalf("ListDeletedCommunitiesForRestore: %v", err)
			}
			if len(resp.Msg.Communities) != 0 {
				t.Errorf("expected 0 results for active community, got %d", len(resp.Msg.Communities))
			}
		}
	})

	t.Run("after delete, both members see the community in their list", func(t *testing.T) {
		if _, err := service.DeleteCommunity(ctxAlice, connect.NewRequest(&api.DeleteCommunityRequest{
			Id: communityID,
		})); err != nil {
			t.Fatalf("DeleteCommunity: %v", err)
		}

		for _, c := range []struct {
			label string
			ctx   context.Context
		}{
			{"alice (deleter)", ctxAlice},
			{"bob (snapshot member)", ctxBob},
		} {
			resp, err := service.ListDeletedCommunitiesForRestore(c.ctx,
				connect.NewRequest(&api.ListDeletedCommunitiesForRestoreRequest{}))
			if err != nil {
				t.Fatalf("%s: ListDeletedCommunitiesForRestore: %v", c.label, err)
			}
			if len(resp.Msg.Communities) != 1 {
				t.Fatalf("%s: expected 1 result, got %d", c.label, len(resp.Msg.Communities))
			}
			item := resp.Msg.Communities[0]
			if item.Id != communityID {
				t.Errorf("%s: id = %q, want %q", c.label, item.Id, communityID)
			}
			if item.DeletedAtUnixSec == 0 {
				t.Errorf("%s: DeletedAtUnixSec should be populated", c.label)
			}
			if item.DeletedByUserId != aliceID {
				t.Errorf("%s: DeletedByUserId = %q, want %q", c.label, item.DeletedByUserId, aliceID)
			}
		}
	})
}
