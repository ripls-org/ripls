package community

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// TestCommunity_FieldRoundTrip verifies that user-provided fields on
// CreateCommunityRequest survive the create → fetch cycle through
// GetCommunity. See docs/proto_conventions.md for the round-trip
// convention and issue #1144 for the motivation.
func TestCommunity_FieldRoundTrip(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc := setupTestService(t, testStorage)

	userID := setupTestUser(t, testStorage, "roundtrip@example.com", "Round Trip")
	ctx := createAuthenticatedContext(userID, "roundtrip@example.com", models.Role_ROLE_USER)

	const (
		wantName        = "Round Trip Community"
		wantDescription = "Community for round-trip testing"
	)
	wantMediaIDs := []string{"media-rt-1", "media-rt-2"}

	var createdID string

	save := func() error {
		resp, err := svc.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
			Name:        wantName,
			Description: wantDescription,
			MediaIds:    wantMediaIDs,
		}))
		if err != nil {
			return err
		}
		createdID = resp.Msg.Id
		return nil
	}

	fetch := func() (*api.GetCommunityResponse, error) {
		resp, err := svc.GetCommunity(ctx, connect.NewRequest(&api.GetCommunityRequest{
			Id: createdID,
		}))
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}

	services.AssertFieldRoundTrip(t, "name", wantName,
		save,
		func() (string, error) {
			resp, err := fetch()
			if err != nil {
				return "", err
			}
			return resp.Name, nil
		},
	)

	services.AssertFieldRoundTrip(t, "description", wantDescription,
		func() error { return nil },
		func() (string, error) {
			resp, err := fetch()
			if err != nil {
				return "", err
			}
			return resp.Description, nil
		},
	)

	services.AssertFieldRoundTrip(t, "media_ids", wantMediaIDs,
		func() error { return nil },
		func() ([]string, error) {
			resp, err := fetch()
			if err != nil {
				return nil, err
			}
			return resp.MediaIds, nil
		},
	)
}
