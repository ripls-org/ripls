package device

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// Service implements the DeviceService RPC interface.
type Service struct {
	storage             *storage.ProtoSQLStorage
	notificationService notifications.Service
}

// New creates a new device service.
func New(sqlStorage *storage.ProtoSQLStorage, notifService notifications.Service) *Service {
	return &Service{
		storage:             sqlStorage,
		notificationService: notifService,
	}
}

// RegisterDeviceToken registers a device token for push notifications.
func (s *Service) RegisterDeviceToken(
	ctx context.Context,
	req *connect.Request[api.RegisterDeviceTokenRequest],
) (*connect.Response[api.RegisterDeviceTokenResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.InfoContext(ctx, "registering device token",
		"platform", req.Msg.Platform,
		"device_token", logging.MaskToken(req.Msg.DeviceToken),
		"has_installation_id", req.Msg.InstallationId != "",
	)

	// Check if token already registered for this user
	existing, err := s.getDeviceByToken(ctx, req.Msg.DeviceToken)
	if err == nil && existing.UserId == authInfo.UserID {
		// Already registered, return existing ID (idempotent).
		//
		// Except for the installation ID: a device registered before the client
		// started reporting one keeps re-registering the same token, so this
		// early return is the only place that row is ever seen again. Without
		// the update below it would stay on the deprecated token path forever
		// (#3043).
		if req.Msg.InstallationId != "" && existing.InstallationId != req.Msg.InstallationId {
			existing.InstallationId = req.Msg.InstallationId
			if updErr := s.storage.Update(ctx, existing); updErr != nil {
				// Non-fatal: the token path still delivers.
				logger.WarnContext(ctx, "failed to backfill device installation ID",
					"device_id", existing.Id,
					"error", updErr,
				)
			}
		}
		logger.InfoContext(ctx, "device token already registered",
			"device_id", existing.Id,
		)
		return connect.NewResponse(&api.RegisterDeviceTokenResponse{
			DeviceId: existing.Id,
		}), nil
	}

	// If token exists for different user, remove it (user switched accounts)
	if err == nil && existing.UserId != authInfo.UserID {
		logger.InfoContext(ctx, "device token belongs to different user, removing old registration",
			"previous_user_id", existing.UserId,
		)
		if delErr := s.storage.Delete(ctx, existing); delErr != nil {
			logger.ErrorContext(ctx, "failed to delete old device registration",
				"error", delErr,
			)
		}
	}

	// Create new device registration
	now := time.Now().Unix()
	device := &models.UserDevice{
		UserId:           authInfo.UserID,
		DeviceToken:      req.Msg.DeviceToken,
		InstallationId:   req.Msg.InstallationId,
		Platform:         convertPlatform(req.Msg.Platform),
		CreatedAtUnixSec: now,
		LastUsedUnixSec:  now,
	}

	id, err := s.storage.Insert(ctx, device)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert device",
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "RegisterDeviceToken", err)
	}

	logger.InfoContext(ctx, "registered device",
		"device_id", id,
	)

	return connect.NewResponse(&api.RegisterDeviceTokenResponse{
		DeviceId: id,
	}), nil
}

// UnregisterDeviceToken unregisters a device token.
func (s *Service) UnregisterDeviceToken(
	ctx context.Context,
	req *connect.Request[api.UnregisterDeviceTokenRequest],
) (*connect.Response[api.UnregisterDeviceTokenResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.InfoContext(ctx, "unregistering device token",
		"device_token", logging.MaskToken(req.Msg.DeviceToken),
	)

	// Use shared unregister logic from notification service
	if err := s.notificationService.UnregisterDevice(ctx, req.Msg.DeviceToken); err != nil {
		logger.ErrorContext(ctx, "failed to unregister device",
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "UnregisterDeviceToken", err)
	}

	return connect.NewResponse(&api.UnregisterDeviceTokenResponse{}), nil
}

// getDeviceByToken finds a device by its token.
func (s *Service) getDeviceByToken(ctx context.Context, deviceToken string) (*models.UserDevice, error) {
	messages, err := s.storage.QueryByField(ctx, "device_token", deviceToken, &models.UserDevice{})
	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("device not found")
	}

	return messages[0].(*models.UserDevice), nil
}

// convertPlatform converts API platform enum to models platform enum.
func convertPlatform(apiPlatform api.DevicePlatform) models.DevicePlatform {
	switch apiPlatform {
	case api.DevicePlatform_DEVICE_PLATFORM_IOS:
		return models.DevicePlatform_DEVICE_PLATFORM_IOS
	case api.DevicePlatform_DEVICE_PLATFORM_ANDROID:
		return models.DevicePlatform_DEVICE_PLATFORM_ANDROID
	case api.DevicePlatform_DEVICE_PLATFORM_WEB:
		return models.DevicePlatform_DEVICE_PLATFORM_WEB
	default:
		return models.DevicePlatform_DEVICE_PLATFORM_UNSPECIFIED
	}
}

// Verify that Service implements the DeviceServiceHandler interface.
var _ apiconnect.DeviceServiceHandler = (*Service)(nil)
