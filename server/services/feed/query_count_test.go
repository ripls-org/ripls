package feed

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// Query-count guard for the feed assembly path.
//
// generateFeedItems fans out over storage: for a page of events it batch-fetches
// gear, requests, experiences, users, three community pivots, locations, offers,
// transfers, RSVPs, three kinds of conversation and their message timestamps.
// Splitting a function shaped like that is exactly how an N+1 gets introduced
// without any test noticing — the feed items come back identical and only the
// query count moves. docs/server/profiling.md prescribes AssertMaxQueries for
// this; the feed package had no coverage before this.
//
// The bound is empirical. To regenerate after an intentional change: drop the
// AssertMaxQueries wrapper, run the test, and read the count off GetQueryStats.
// Raising the bound to make a refactor pass is the regression this guards
// against — raise it only once the extra queries are confirmed intended.
func TestGetFeed_QueryCount(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Breadth matters more than depth: every branch of the assembly should have
	// at least one row to walk, so a per-item query shows up in the count.
	userID := setupTestUser(t, sqlStorage, "qc-self@test.com", "Self")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Query Count Community")

	gearID := setupTestGear(t, sqlStorage, userID, "Query Count Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)
	createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)

	requestID := setupTestRequest(t, sqlStorage, userID, communityID,
		"Query Count Request", models.RequestState_REQUEST_STATE_ACTIVE)
	createRequestCreatedEvent(t, sqlStorage, communityID, userID, requestID)

	experienceID := setupTestExperience(t, sqlStorage, userID, "Query Count Experience")
	createExperienceCreatedEvent(t, sqlStorage, communityID, userID, experienceID)

	ctx := storage.WithQueryStats(createAuthenticatedContext(userID, "qc-self@test.com", models.Role_ROLE_USER))

	// Measured at 17 for this fixture; bounded at 22 to absorb minor drift.
	// Deliberately tight — a loose bound would let an N+1 regression through and
	// the guard would be decorative.
	const maxQueries = 22

	storage.AssertMaxQueries(t, ctx, maxQueries, func() {
		if _, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
			CommunityIds: []string{communityID},
			PageSize:     20,
		})); err != nil {
			t.Fatalf("GetFeed: %v", err)
		}
	})
}
