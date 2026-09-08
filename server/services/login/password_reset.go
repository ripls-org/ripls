package login

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// RequestPasswordReset initiates a password reset by sending an email with a reset link.
func (s *Service) RequestPasswordReset(
	ctx context.Context,
	req *connect.Request[api.RequestPasswordResetRequest],
) (*connect.Response[api.RequestPasswordResetResponse], error) {
	if req.Msg.Email == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("email is required"))
	}

	// Look up user by email
	user, err := s.userManager.GetUserByEmail(ctx, req.Msg.Email)
	if err != nil {
		// For security, always return success even if user lookup fails
		return connect.NewResponse(&api.RequestPasswordResetResponse{}), nil
	}

	if user == nil {
		// For security, don't reveal that the user doesn't exist
		return connect.NewResponse(&api.RequestPasswordResetResponse{}), nil
	}

	// Only allow password reset for email/password auth users
	if user.AuthMethod != models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD {
		// For security, don't reveal auth method - just silently succeed
		return connect.NewResponse(&api.RequestPasswordResetResponse{}), nil
	}

	// Generate secure random token
	token := uuid.New().String()

	// Create reset request with 1-hour expiration
	now := clock.UnixSec(ctx)
	resetRequest := &models.PendingPasswordReset{
		Id:               uuid.New().String(),
		UserId:           user.Id,
		Token:            token,
		CreatedAtUnixSec: now,
		ExpiresAtUnixSec: now + 3600, // 1 hour from now
	}

	// Store the reset request
	_, err = s.storage.Insert(ctx, resetRequest)
	if err != nil {
		return nil, connecterr.Internal(ctx, "RequestPasswordReset", err, "detail",

			// Build reset URL
			"failed to create password reset request")
	}

	// Built from the same origin every other outbound link uses. This was
	// hardcoded to the production host until #2953, so a reset mailed by any
	// non-production server sent the recipient to production, where the token
	// did not exist.
	resetURL := s.branding.AppURL("/reset-password?token=" + token)

	// Send email
	logger := logging.LoggerWithContext(ctx)
	err = s.emailService.SendPasswordReset(ctx, user.Email, resetURL, user.GetPreferredLanguage())
	if err != nil {
		// Log error but don't fail the request - user shouldn't know if email failed
		logger.WarnContext(ctx, "failed to send password reset email",
			"user_id", user.Id,
			"user_email", logging.MaskEmail(user.Email),
			"error", err,
		)
	} else {
		logger.InfoContext(ctx, "password reset email sent",
			"user_id", user.Id,
			"user_email", logging.MaskEmail(user.Email),
		)
	}

	return connect.NewResponse(&api.RequestPasswordResetResponse{}), nil
}

// CheckResetPasswordToken validates a password reset token and returns associated email.
func (s *Service) CheckResetPasswordToken(
	ctx context.Context,
	req *connect.Request[api.CheckResetPasswordTokenRequest],
) (*connect.Response[api.CheckResetPasswordTokenResponse], error) {
	if req.Msg.Token == "" {
		return connect.NewResponse(&api.CheckResetPasswordTokenResponse{
			IsValid:      false,
			ErrorMessage: "token is required",
		}), nil
	}

	// Look up reset request by token
	resetRequests, err := s.storage.QueryByField(ctx, "token", req.Msg.Token, &models.PendingPasswordReset{})
	if err != nil {
		return connect.NewResponse(&api.CheckResetPasswordTokenResponse{
			IsValid:      false,
			ErrorMessage: "failed to validate token",
		}), nil
	}

	if len(resetRequests) == 0 {
		return connect.NewResponse(&api.CheckResetPasswordTokenResponse{
			IsValid:      false,
			ErrorMessage: "invalid token",
		}), nil
	}

	resetRequest := resetRequests[0].(*models.PendingPasswordReset)

	// Check if token has expired
	now := clock.UnixSec(ctx)
	if now > resetRequest.ExpiresAtUnixSec {
		return connect.NewResponse(&api.CheckResetPasswordTokenResponse{
			IsValid:      false,
			ErrorMessage: "token has expired",
		}), nil
	}

	// Get user to return email
	user := &models.User{}
	if err := s.storage.GetByID(ctx, resetRequest.UserId, user); err != nil {
		return connect.NewResponse(&api.CheckResetPasswordTokenResponse{
			IsValid:      false,
			ErrorMessage: "failed to retrieve user information",
		}), nil
	}

	return connect.NewResponse(&api.CheckResetPasswordTokenResponse{
		IsValid: true,
		Email:   user.Email,
	}), nil
}

// ResetPassword completes a password reset with a new password.
func (s *Service) ResetPassword(
	ctx context.Context,
	req *connect.Request[api.ResetPasswordRequest],
) (*connect.Response[api.ResetPasswordResponse], error) {
	if req.Msg.Token == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("token is required"))
	}

	if req.Msg.NewPassword == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("new password is required"))
	}

	// Look up reset request by token
	resetRequests, err := s.storage.QueryByField(ctx, "token", req.Msg.Token, &models.PendingPasswordReset{})
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid token"))
	}

	if len(resetRequests) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid token"))
	}

	resetRequest := resetRequests[0].(*models.PendingPasswordReset)

	// Check if token has expired
	now := clock.UnixSec(ctx)
	if now > resetRequest.ExpiresAtUnixSec {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("token has expired"))
	}

	// Get user
	user := &models.User{}
	if err := s.storage.GetByID(ctx, resetRequest.UserId, user); err != nil {
		return nil, connecterr.Internal(ctx, "ResetPassword", fmt.Errorf("failed to retrieve user"))
	}

	// Hash new password
	passwordHash, err := auth.HashPassword(req.Msg.NewPassword)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid password: %w", err))
	}

	// Update user's password
	user.PasswordHash = passwordHash
	user.UpdatedAt = now

	if err := s.storage.Update(ctx, user); err != nil {
		return nil, connecterr.Internal(ctx, "ResetPassword", err, "detail", "failed to update password")
	}

	logger := logging.LoggerWithContext(ctx)
	logger.InfoContext(ctx, "password reset successful",
		"user_id", user.Id,
		"user_email", logging.MaskEmail(user.Email),
	)

	// Delete the reset request (invalidate token)
	if err := s.storage.Delete(ctx, resetRequest); err != nil {
		// Log but don't fail - password was updated successfully
		logger.WarnContext(ctx, "failed to delete password reset request",
			"user_id", user.Id,
			"error", err,
		)
	}

	return connect.NewResponse(&api.ResetPasswordResponse{}), nil
}
