package transfer

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

func TestService_NoAuthentication(t *testing.T) {
	service, _, _, _ := setupTestServiceWithNotifications(t)

	ctx := context.Background()

	t.Run("ExpressInterest requires auth", func(t *testing.T) {
		req := connect.NewRequest(&api.ExpressInterestRequest{
			GearId: "some-gear-id",
		})
		_, err := service.ExpressInterest(ctx, req)
		if err == nil {
			t.Fatal("Expected error for unauthenticated request")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connectErr.Code())
		}
	})

	t.Run("ListMyTransfers requires auth", func(t *testing.T) {
		req := connect.NewRequest(&api.ListMyTransfersRequest{})
		_, err := service.ListMyTransfers(ctx, req)
		if err == nil {
			t.Fatal("Expected error for unauthenticated request")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connectErr.Code())
		}
	})
}
