package login

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_OIDCRegister_WithAvatar(t *testing.T) {
	ctx := context.Background()

	// Create a test HTTP server that serves a real avatar PNG. It must be a
	// decodable image because the upload path now sanitizes (decodes +
	// re-encodes) it (#1953).
	fakeAvatarData := func() []byte {
		img := image.NewRGBA(image.Rect(0, 0, 32, 32))
		for y := 0; y < 32; y++ {
			for x := 0; x < 32; x++ {
				img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 200, A: 255})
			}
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatalf("encode avatar png: %v", err)
		}
		return buf.Bytes()
	}()
	avatarServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeAvatarData)
	}))
	defer avatarServer.Close()

	// Create a test service with mock OIDC
	service, _, oidcSetup := createServiceWithMockOIDC(t)
	sqlStorage := service.storage

	// Create inviter user and community
	inviter := createTestUser(t, ctx, sqlStorage, "inviter@example.com", "Inviter")
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

	// Create a valid test token with the test server's avatar URL
	avatarURL := avatarServer.URL + "/avatar.png"
	testToken := oidcSetup.CreateValidGoogleTokenWithPicture(t, "google-user-123", "oidc@example.com", "OIDC User", avatarURL)

	req := connect.NewRequest(&api.OIDCRegisterRequest{
		Provider:  api.OIDCProvider_OIDC_PROVIDER_GOOGLE,
		IdToken:   testToken,
		ShortCode: invitation.ShortCode,
	})

	// Register the user - should download and store the avatar
	resp, err := service.OIDCRegister(ctx, req)
	if err != nil {
		t.Fatalf("OIDCRegister failed: %v", err)
	}

	// Verify user object in response is populated
	if resp.Msg.User == nil {
		t.Fatal("Expected user object to be populated")
	}
	if resp.Msg.User.Id == "" {
		t.Fatal("Expected user ID to be generated")
	}
	if resp.Msg.User.Name == "" {
		t.Error("Expected user name to be populated")
	}
	// Media ID should be populated from avatar download
	if resp.Msg.User.MediaId == "" {
		t.Error("Expected media_id in response after avatar download")
	}

	// Verify user was added to the database
	user := &models.User{}
	err = sqlStorage.GetByID(ctx, resp.Msg.User.Id, user)
	if err != nil {
		t.Fatalf("Failed to retrieve user: %v", err)
	}

	// Verify user has correct auth method
	if user.AuthMethod != models.AuthMethod_AUTH_METHOD_GOOGLE {
		t.Errorf("Expected auth method GOOGLE, got %v", user.AuthMethod)
	}

	// Verify media_ids[] is populated after avatar download.
	if len(user.MediaIds) == 0 {
		t.Fatal("Expected media_ids[] to be populated after avatar download")
	}
	avatarID := user.MediaIds[0]

	// Verify the media was actually stored
	media := &models.Media{}
	err = sqlStorage.GetByID(ctx, avatarID, media)
	if err != nil {
		t.Fatalf("Failed to retrieve media: %v", err)
	}

	// Verify media fields
	if media.UserId != user.Id {
		t.Errorf("Expected media user_id %s, got %s", user.Id, media.UserId)
	}
	if media.ContentType != "image/png" {
		t.Errorf("Expected content type image/png, got %s", media.ContentType)
	}
	if media.GetFilename() != "avatar.png" {
		t.Errorf("Expected filename avatar.png, got %s", media.GetFilename())
	}
	if !strings.Contains(media.GetDescription(), "OIDC provider") {
		t.Errorf("Expected description to mention OIDC provider, got %s", media.GetDescription())
	}
	if media.SizeBytes != int64(len(fakeAvatarData)) {
		t.Errorf("Expected size %d, got %d", len(fakeAvatarData), media.SizeBytes)
	}
	if media.StorageUrl == "" {
		t.Error("Expected storage_url to be set")
	}
}

func TestService_OIDCRegister_WithoutAvatar(t *testing.T) {
	ctx := context.Background()

	// Create a test service with mock OIDC
	service, _, oidcSetup := createServiceWithMockOIDC(t)
	sqlStorage := service.storage

	// Create inviter user and community
	inviter := createTestUser(t, ctx, sqlStorage, "inviter@example.com", "Inviter")
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

	// Create a valid test token WITHOUT a picture URL (empty string)
	testToken := oidcSetup.CreateValidGoogleTokenWithPicture(t, "google-user-456", "oidc-noavatar@example.com", "OIDC User No Avatar", "")

	req := connect.NewRequest(&api.OIDCRegisterRequest{
		Provider:  api.OIDCProvider_OIDC_PROVIDER_GOOGLE,
		IdToken:   testToken,
		ShortCode: invitation.ShortCode,
	})

	// Register the user - should succeed without avatar
	resp, err := service.OIDCRegister(ctx, req)
	if err != nil {
		t.Fatalf("OIDCRegister failed: %v", err)
	}

	// Verify user was created
	if resp.Msg.User.Id == "" {
		t.Fatal("Expected user ID to be generated")
	}

	// Verify user was added to the database
	user := &models.User{}
	err = sqlStorage.GetByID(ctx, resp.Msg.User.Id, user)
	if err != nil {
		t.Fatalf("Failed to retrieve user: %v", err)
	}

	// Verify media_ids is empty (no avatar to download)
	if len(user.MediaIds) != 0 {
		t.Errorf("Expected media_ids to be empty when no avatar provided, got %v", user.MediaIds)
	}
}

func TestService_OIDCRegister_AvatarDownloadFails(t *testing.T) {
	ctx := context.Background()

	// Create a test HTTP server that returns 404 for avatar
	avatarServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	}))
	defer avatarServer.Close()

	// Create a test service with mock OIDC
	service, _, oidcSetup := createServiceWithMockOIDC(t)
	sqlStorage := service.storage

	// Create inviter user and community
	inviter := createTestUser(t, ctx, sqlStorage, "inviter@example.com", "Inviter")
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

	// Create a valid test token with a URL that will return 404
	avatarURL := avatarServer.URL + "/missing.png"
	testToken := oidcSetup.CreateValidGoogleTokenWithPicture(t, "google-user-789", "oidc-failavatar@example.com", "OIDC User Fail Avatar", avatarURL)

	req := connect.NewRequest(&api.OIDCRegisterRequest{
		Provider:  api.OIDCProvider_OIDC_PROVIDER_GOOGLE,
		IdToken:   testToken,
		ShortCode: invitation.ShortCode,
	})

	// Register should still succeed even though avatar download fails
	resp, err := service.OIDCRegister(ctx, req)
	if err != nil {
		t.Fatalf("OIDCRegister should succeed even with avatar download failure: %v", err)
	}

	// Verify user was created
	if resp.Msg.User.Id == "" {
		t.Fatal("Expected user ID to be generated")
	}

	// Verify user was added to the database
	user := &models.User{}
	err = sqlStorage.GetByID(ctx, resp.Msg.User.Id, user)
	if err != nil {
		t.Fatalf("Failed to retrieve user: %v", err)
	}

	// Verify media_ids is empty (avatar download failed, but registration succeeded)
	if len(user.MediaIds) != 0 {
		t.Errorf("Expected media_ids to be empty when avatar download fails, got %v", user.MediaIds)
	}
}
