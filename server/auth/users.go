package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// NormalizeEmail returns email lowercased with surrounding whitespace
// trimmed. Apply at every read and write site so case and whitespace
// variants of the same address collide.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// UserManager is an abstraction for handling user persistence.
type UserManager struct {
	storage *storage.ProtoSQLStorage
}

// NewUserManager creates a new instance wrapping `storage`.
func NewUserManager(storage *storage.ProtoSQLStorage) *UserManager {
	return &UserManager{storage: storage}
}

// CreateUser creates a new user in the database.
func (um *UserManager) CreateUser(ctx context.Context, email, name string, role models.Role) (*models.User, error) {
	email = NormalizeEmail(email)
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CreateUser",
		"user_email", logging.MaskEmail(email),
		"role", role.String(),
	)

	// Check if user already exists
	existingUser, err := um.GetUserByEmail(ctx, email)
	if err == nil && existingUser != nil {
		logger.WarnContext(ctx, "user creation failed: email already exists")
		return nil, fmt.Errorf("user with email %s already exists", email)
	}

	// Create new user
	now := time.Now().Unix()
	user := &models.User{
		Id:        uuid.New().String(),
		Email:     email,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
		Role:      role,
	}

	// Insert into database
	_, err = um.storage.Insert(ctx, user)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert user into database",
			"error", err,
		)
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	logger.InfoContext(ctx, "user created successfully",
		"user_id", user.Id,
	)
	return user, nil
}

// GetUserByEmail retrieves a user by email address. Email is normalized
// (lowercased + trimmed) before lookup so case and whitespace variants
// match. Soft-deleted rows are excluded by storage.QueryByField's default
// deleted filter.
func (um *UserManager) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	email = NormalizeEmail(email)
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetUserByEmail",
		"user_email", logging.MaskEmail(email),
	)

	results, err := um.storage.QueryByField(ctx, "email", email, &models.User{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query user by email",
			"error", err,
		)
		return nil, fmt.Errorf("failed to query user by email: %w", err)
	}

	if len(results) == 0 {
		logger.DebugContext(ctx, "user not found")
		return nil, nil
	}

	if len(results) > 1 {
		logger.ErrorContext(ctx, "multiple users found with same email",
			"count", len(results),
		)
		return nil, fmt.Errorf("multiple users found with email %s", email)
	}

	user, ok := results[0].(*models.User)
	if !ok {
		logger.ErrorContext(ctx, "unexpected type returned from query")
		return nil, fmt.Errorf("unexpected type returned from query")
	}

	logger.DebugContext(ctx, "user retrieved successfully",
		"user_id", user.Id,
	)
	return user, nil
}

// GetUserByPhone retrieves a user by phone number (E.164 format).
// Returns nil, nil if no user is found.
func (um *UserManager) GetUserByPhone(ctx context.Context, phoneNumber string) (*models.User, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetUserByPhone",
	)

	results, err := um.storage.QueryByField(ctx, "phone_number", phoneNumber, &models.User{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query user by phone",
			"error", err,
		)
		return nil, fmt.Errorf("failed to query user by phone: %w", err)
	}

	if len(results) == 0 {
		logger.DebugContext(ctx, "user not found by phone")
		return nil, nil
	}

	if len(results) > 1 {
		logger.ErrorContext(ctx, "multiple users found with same phone number",
			"count", len(results),
		)
		return nil, fmt.Errorf("multiple users found with phone number")
	}

	user, ok := results[0].(*models.User)
	if !ok {
		logger.ErrorContext(ctx, "unexpected type returned from query")
		return nil, fmt.Errorf("unexpected type returned from query")
	}

	logger.DebugContext(ctx, "user retrieved successfully",
		"user_id", user.Id,
	)
	return user, nil
}

// GetUserByID retrieves a user by ID.
func (um *UserManager) GetUserByID(ctx context.Context, userID string) (*models.User, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetUserByID",
		"user_id", userID,
	)

	user := &models.User{}
	err := um.storage.GetByID(ctx, userID, user)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get user by ID",
			"error", err,
		)
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	logger.DebugContext(ctx, "user retrieved successfully by ID")
	return user, nil
}

// UpdateUser updates an existing user in the database.
func (um *UserManager) UpdateUser(ctx context.Context, user *models.User) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "UpdateUser",
		"user_id", user.Id,
		"user_email", logging.MaskEmail(user.Email),
	)

	user.UpdatedAt = time.Now().Unix()
	err := um.storage.Update(ctx, user)
	if err != nil {
		logger.ErrorContext(ctx, "failed to update user",
			"error", err,
		)
		return fmt.Errorf("failed to update user: %w", err)
	}

	logger.InfoContext(ctx, "user updated successfully")
	return nil
}

// DeleteUser soft-deletes a user by setting the deleted metadata.
func (um *UserManager) DeleteUser(ctx context.Context, userID string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DeleteUser",
		"user_id", userID,
	)

	// Get the user
	user := &models.User{}
	err := um.storage.GetByID(ctx, userID, user)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get user for deletion", "error", err)
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Set the deleted metadata
	user.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  userID,
		DeletedAtUnixSec: time.Now().Unix(),
	}
	user.UpdatedAt = time.Now().Unix()

	err = um.storage.Update(ctx, user)
	if err != nil {
		logger.ErrorContext(ctx, "failed to soft-delete user", "error", err)
		return fmt.Errorf("failed to delete user: %w", err)
	}

	logger.InfoContext(ctx, "user soft-deleted successfully")
	return nil
}

// CreateOrUpdateFromOIDC creates a new user or updates an existing user based on OIDC claims.
func (um *UserManager) CreateOrUpdateFromOIDC(ctx context.Context, userInfo *UserInfo) (*models.User, error) {
	normalizedEmail := NormalizeEmail(userInfo.Email)
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CreateOrUpdateFromOIDC",
		"auth_method", "oidc",
		"user_email", logging.MaskEmail(normalizedEmail),
	)

	// Try to get existing user by email
	existingUser, err := um.GetUserByEmail(ctx, normalizedEmail)
	if err == nil && existingUser != nil {
		// Update existing user with latest info from OIDC provider
		now := time.Now().Unix()
		existingUser.Name = userInfo.Name
		existingUser.UpdatedAt = now

		logger.InfoContext(ctx, "updating existing user from OIDC",
			"user_id", existingUser.Id,
		)

		// Update in database (this would need an Update method in storage)
		// For now, we'll just return the existing user
		return existingUser, nil
	}

	// Create new user from OIDC claims
	now := time.Now().Unix()
	user := &models.User{
		Id:        uuid.New().String(),
		Email:     normalizedEmail,
		Name:      userInfo.Name,
		CreatedAt: now,
		UpdatedAt: now,
		Role:      models.Role_ROLE_USER, // Default role for OIDC users
	}

	// Insert into database
	_, err = um.storage.Insert(ctx, user)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create user from OIDC",
			"error", err,
		)
		return nil, fmt.Errorf("failed to create user from OIDC: %w", err)
	}

	logger.InfoContext(ctx, "created new user from OIDC",
		"user_id", user.Id,
	)
	return user, nil
}
