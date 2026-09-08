package community

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestGenerateShortCode(t *testing.T) {
	t.Run("returns correct length", func(t *testing.T) {
		code, err := generateShortCode()
		if err != nil {
			t.Fatalf("generateShortCode() error: %v", err)
		}
		if len(code) != shortCodeLength {
			t.Errorf("Expected length %d, got %d", shortCodeLength, len(code))
		}
	})

	t.Run("uses only valid charset characters", func(t *testing.T) {
		for range 100 {
			code, err := generateShortCode()
			if err != nil {
				t.Fatalf("generateShortCode() error: %v", err)
			}
			for _, c := range code {
				if !strings.ContainsRune(shortCodeCharset, c) {
					t.Errorf("Short code %q contains invalid character %q", code, c)
				}
			}
		}
	})

	t.Run("excludes confusable characters", func(t *testing.T) {
		confusable := "0Oo1lIi"
		for range 100 {
			code, err := generateShortCode()
			if err != nil {
				t.Fatalf("generateShortCode() error: %v", err)
			}
			for _, c := range code {
				if strings.ContainsRune(confusable, c) {
					t.Errorf("Short code %q contains confusable character %q", code, c)
				}
			}
		}
	})

	t.Run("generates distinct codes", func(t *testing.T) {
		codes := make(map[string]bool)
		for range 100 {
			code, err := generateShortCode()
			if err != nil {
				t.Fatalf("generateShortCode() error: %v", err)
			}
			if codes[code] {
				t.Errorf("Duplicate code generated: %s", code)
			}
			codes[code] = true
		}
	})
}

func TestShortCodeCharset(t *testing.T) {
	t.Run("does not contain confusable characters", func(t *testing.T) {
		confusable := []byte{'0', 'O', 'o', '1', 'l', 'I', 'i'}
		for _, c := range confusable {
			if strings.ContainsRune(shortCodeCharset, rune(c)) {
				t.Errorf("Charset contains confusable character %q", c)
			}
		}
	})

	t.Run("contains expected character ranges", func(t *testing.T) {
		// Should have uppercase (minus O and I)
		for c := byte('A'); c <= 'Z'; c++ {
			if c == 'O' || c == 'I' {
				continue
			}
			if !strings.ContainsRune(shortCodeCharset, rune(c)) {
				t.Errorf("Charset missing expected uppercase character %q", c)
			}
		}

		// Should have lowercase (minus i, l, o which are confusable)
		for c := byte('a'); c <= 'z'; c++ {
			if c == 'i' || c == 'l' || c == 'o' {
				continue
			}
			if !strings.ContainsRune(shortCodeCharset, rune(c)) {
				t.Errorf("Charset missing expected lowercase character %q", c)
			}
		}

		// Should have digits 2-9 (minus 0 and 1)
		for c := byte('2'); c <= '9'; c++ {
			if !strings.ContainsRune(shortCodeCharset, rune(c)) {
				t.Errorf("Charset missing expected digit %q", c)
			}
		}
	})
}

func TestGenerateUniqueShortCode(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	t.Run("generates a valid short code", func(t *testing.T) {
		code, err := service.generateUniqueShortCode(context.Background())
		if err != nil {
			t.Fatalf("generateUniqueShortCode() error: %v", err)
		}
		if len(code) != shortCodeLength {
			t.Errorf("Expected length %d, got %d", shortCodeLength, len(code))
		}
	})

	t.Run("returns code not already in database", func(t *testing.T) {
		ctx := context.Background()
		code, err := service.generateUniqueShortCode(ctx)
		if err != nil {
			t.Fatalf("generateUniqueShortCode() error: %v", err)
		}

		// Verify it's not in the DB
		results, err := service.lookupShareLink(ctx, code)
		if err != nil {
			t.Fatalf("lookupShareLink() error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("Expected no existing share link with code %q", code)
		}
	})
}

func TestLookupShareLink(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)
	ctx := context.Background()

	t.Run("returns empty for non-existent short code", func(t *testing.T) {
		results, err := service.lookupShareLink(ctx, "nonexist")
		if err != nil {
			t.Fatalf("lookupShareLink() error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("Expected 0 results, got %d", len(results))
		}
	})

	t.Run("finds share link by short code", func(t *testing.T) {
		userID := setupTestUser(t, testStorage, "inviter@example.com", "Inviter")
		userCtx := createAuthenticatedContext(userID, "inviter@example.com", models.Role_ROLE_USER)

		// Create community and invite link
		createResp, err := service.CreateCommunity(userCtx, connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Lookup Test Community",
		}))
		if err != nil {
			t.Fatalf("CreateCommunity failed: %v", err)
		}

		inviteResp := mintCommunityInvite(t, userCtx, service, createResp.Msg.Id)

		shortCode := inviteResp.ShortCode
		if shortCode == "" {
			t.Fatal("Expected non-empty short code in response")
		}

		results, err := service.lookupShareLink(ctx, shortCode)
		if err != nil {
			t.Fatalf("lookupShareLink() error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].ShortCode != shortCode {
			t.Errorf("Expected short code %q, got %q", shortCode, results[0].ShortCode)
		}
		if results[0].CommunityId != createResp.Msg.Id {
			t.Errorf("Expected community ID %q, got %q", createResp.Msg.Id, results[0].CommunityId)
		}
	})
}

func TestGetOrCreateShareLink_ShortCodeFields(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	userID := setupTestUser(t, testStorage, "creator@example.com", "Creator")
	ctx := createAuthenticatedContext(userID, "creator@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Short Code Test Community",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := createResp.Msg.Id

	t.Run("response includes short_code", func(t *testing.T) {
		resp := mintCommunityInvite(t, ctx, service, communityID)

		if resp.ShortCode == "" {
			t.Error("Expected non-empty ShortCode in response")
		}
		if len(resp.ShortCode) != shortCodeLength {
			t.Errorf("Expected ShortCode length %d, got %d", shortCodeLength, len(resp.ShortCode))
		}
	})

	t.Run("URL uses /go/ format with short code", func(t *testing.T) {
		resp := mintCommunityInvite(t, ctx, service, communityID)

		expectedPrefix := "https://test.example.com/go/"
		if !strings.HasPrefix(resp.ShareUrl, expectedPrefix) {
			t.Errorf("Expected URL prefix %q, got %q", expectedPrefix, resp.ShareUrl)
		}

		// URL should contain the short code
		if !strings.Contains(resp.ShareUrl, resp.ShortCode) {
			t.Errorf("Expected URL to contain short code %q, got %q", resp.ShortCode, resp.ShareUrl)
		}
	})

	t.Run("subsequent calls return same short_code", func(t *testing.T) {
		resp1 := mintCommunityInvite(t, ctx, service, communityID)
		resp2 := mintCommunityInvite(t, ctx, service, communityID)

		if resp1.ShortCode != resp2.ShortCode {
			t.Errorf("Expected same short code, got %q and %q", resp1.ShortCode, resp2.ShortCode)
		}
	})
}
