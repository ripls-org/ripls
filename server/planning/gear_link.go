package planning

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// ValidateGearLink confirms that gearID, if non-empty, exists and is owned
// by userID. Returns CodeInvalidArgument when the gear is missing or owned
// by a different user. Returns nil for an empty gearID — callers may pass
// the raw optional field through without a nil check.
func ValidateGearLink(ctx context.Context, st *storage.ProtoSQLStorage, userID, gearID string) error {
	if gearID == "" {
		return nil
	}
	gear := &models.Gear{}
	if err := st.GetByID(ctx, gearID, gear); err != nil {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("linked gear not found"))
	}
	if gear.OwnerId != userID {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("linked gear is not owned by the caller"))
	}
	if gear.Deleted != nil {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("linked gear has been removed"))
	}
	return nil
}
