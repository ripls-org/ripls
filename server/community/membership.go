package community

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// GetNumCommunityMembers returns the number of members in a community.
// Returns an error if the community does not exist.
func GetNumCommunityMembers(ctx context.Context, store *storage.ProtoSQLStorage, communityID string) (int, error) {
	// Verify community exists
	community := &models.Community{}
	if err := store.GetByID(ctx, communityID, community); err != nil {
		return 0, fmt.Errorf("community not found: %w", err)
	}

	memberships, err := store.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{})
	if err != nil {
		return 0, err
	}
	return len(memberships), nil
}
