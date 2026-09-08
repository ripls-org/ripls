package login

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// downloadAvatar downloads an avatar image from a URL.
func (s *Service) downloadAvatar(ctx context.Context, avatarURL string) ([]byte, string, error) {
	if avatarURL == "" {
		return nil, "", fmt.Errorf("avatar URL is empty")
	}

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, avatarURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download avatar: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read response body, bounded by the codebase-wide image cap (#1953).
	imageData, err := io.ReadAll(io.LimitReader(resp.Body, storage.MaxImageUploadBytes))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read avatar data: %w", err)
	}

	// Determine content type from response header
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg" // Default to JPEG if not specified
	}

	return imageData, contentType, nil
}

// fetchAndStoreAvatar downloads an avatar from a URL and stores it as media.
// Returns the media ID on success, or an empty string and error on failure.
func (s *Service) fetchAndStoreAvatar(ctx context.Context, userID, avatarURL string) (string, error) {
	// Download avatar
	imageData, contentType, err := s.downloadAvatar(ctx, avatarURL)
	if err != nil {
		return "", fmt.Errorf("failed to download avatar: %w", err)
	}

	// Determine filename from content type
	filename := "avatar.jpg"
	if strings.Contains(contentType, "png") {
		filename = "avatar.png"
	} else if strings.Contains(contentType, "gif") {
		filename = "avatar.gif"
	} else if strings.Contains(contentType, "webp") {
		filename = "avatar.webp"
	}

	// Store avatar using shared helper
	mediaID, err := storage.StoreMedia(
		ctx,
		s.storage,
		s.bucket,
		userID,
		imageData,
		contentType,
		filename,
		"Profile picture from OIDC provider",
		"",    // Not from stock imagery
		false, // stripMetadata: retain EXIF (#1953)
	)
	if err != nil {
		return "", fmt.Errorf("failed to store avatar: %w", err)
	}

	return mediaID, nil
}

// getAuthMethodName returns a human-readable name for an auth method.
func getAuthMethodName(method models.AuthMethod) string {
	switch method {
	case models.AuthMethod_AUTH_METHOD_GOOGLE:
		return "Google"
	case models.AuthMethod_AUTH_METHOD_APPLE:
		return "Apple"
	case models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD:
		return "email and password"
	case models.AuthMethod_AUTH_METHOD_PHONE:
		return "phone number"
	default:
		return "the original method"
	}
}

// mapProviderToAuthMethod maps an OIDC provider to an auth method.
func mapProviderToAuthMethod(provider api.OIDCProvider) models.AuthMethod {
	switch provider {
	case api.OIDCProvider_OIDC_PROVIDER_GOOGLE:
		return models.AuthMethod_AUTH_METHOD_GOOGLE
	case api.OIDCProvider_OIDC_PROVIDER_APPLE:
		return models.AuthMethod_AUTH_METHOD_APPLE
	default:
		return models.AuthMethod_AUTH_METHOD_UNSPECIFIED
	}
}

// toAPIUser converts a models.User to an api.User for API responses.
func toAPIUser(user *models.User) *api.User {
	return &api.User{
		Id:      user.Id,
		Name:    user.Name,
		MediaId: services.PrimaryAvatarMediaID(user),
	}
}

// seedPreferredLanguage sets user.PreferredLanguage from the
// Accept-Language header captured into ctx by the HTTP middleware.
// Falls back to the default tag when no header was sent. The
// resolved tag is normalized against the supported-locale list so a
// caller asking for `en-US` lands as `en`.
//
// Called from every registration path (EmailRegister, OIDCRegister,
// PhoneRegister) before the user row is inserted, so new accounts
// always start with a usable language preference even before the
// client has a chance to send UpdateUserProfile.
func seedPreferredLanguage(ctx context.Context, user *models.User) {
	tag := l10n.LocaleFromContext(ctx).String()
	user.PreferredLanguage = &tag
}
