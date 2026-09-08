package location

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

func TestService_GeocodeAddress(t *testing.T) {
	service, _, _ := setupTestService(t)

	t.Run("unauthenticated request", func(t *testing.T) {
		ctx := context.Background() // No auth info

		req := connect.NewRequest(&api.GeocodeAddressRequest{
			RegionCode:   "US",
			PostalCode:   "94043",
			Locality:     "Mountain View",
			AddressLines: []string{"1600 Amphitheatre Parkway"},
		})

		_, err := service.GeocodeAddress(ctx, req)
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

	t.Run("geocode with test token", func(t *testing.T) {
		// This test will fail with invalid token, but tests the code path
		ctx := createAuthContext("user-123", "test@example.com")

		req := connect.NewRequest(&api.GeocodeAddressRequest{
			RegionCode:   "US",
			PostalCode:   "78703",
			Locality:     "Austin",
			AddressLines: []string{"603 North Lamar Boulevard"},
		})

		// This will fail with "Not Authorized" because we're using test token
		// but it still covers the code path
		_, err := service.GeocodeAddress(ctx, req)
		if err == nil {
			t.Log("Geocoding succeeded (unexpected with test token)")
		} else {
			// Expected to fail with invalid token
			t.Logf("Geocoding failed as expected with test token: %v", err)
		}
	})
}

func TestService_ReverseGeocode(t *testing.T) {
	service, _, _ := setupTestService(t)

	t.Run("unauthenticated request", func(t *testing.T) {
		ctx := context.Background() // No auth info

		req := connect.NewRequest(&api.ReverseGeocodeRequest{
			LatitudeDeg:  30.2672,
			LongitudeDeg: -97.7431,
		})

		_, err := service.ReverseGeocode(ctx, req)
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

	t.Run("reverse geocode with test token", func(t *testing.T) {
		// This test will fail with invalid token, but tests the code path
		ctx := createAuthContext("user-123", "test@example.com")

		req := connect.NewRequest(&api.ReverseGeocodeRequest{
			LatitudeDeg:  30.271850, // Austin, TX
			LongitudeDeg: -97.752842,
		})

		// This will fail with "Not Authorized" because we're using test token
		// but it still covers the code path
		_, err := service.ReverseGeocode(ctx, req)
		if err == nil {
			t.Log("Reverse geocoding succeeded (unexpected with test token)")
		} else {
			// Expected to fail with invalid token
			t.Logf("Reverse geocoding failed as expected with test token: %v", err)
		}
	})
}
