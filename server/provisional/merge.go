// Package provisional provides utilities for managing provisional user accounts
// and merging them into real user accounts when the
// person registers.
package provisional

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// MergeIntoUser migrates all provisional user data (RSVPs, attendance records)
// to a real user account and marks the provisional user as claimed.
//
// This function is called during registration when the new user accepted an
// invitation link that was tied to a provisional user. Operations are applied
// sequentially; if any step fails, the error is returned but previously
// completed steps are not rolled back. Callers should surface the error so the
// user can retry — re-running this function is safe because each step is
// idempotent (it re-checks the provisional_user_id before migrating).
func MergeIntoUser(ctx context.Context, store *storage.ProtoSQLStorage, provisionalUserID, realUserID string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "provisional.MergeIntoUser",
		"provisional_user_id", provisionalUserID,
		"real_user_id", realUserID,
	)

	// Load the provisional user to verify it exists and isn't already claimed.
	prov := &models.ProvisionalUser{}
	if err := store.GetByID(ctx, provisionalUserID, prov); err != nil {
		return fmt.Errorf("provisional user not found: %w", err)
	}

	if prov.ClaimedByUserId != nil {
		// Already claimed — idempotent, nothing to do.
		logger.InfoContext(ctx, "provisional user already claimed, skipping merge",
			"claimed_by", *prov.ClaimedByUserId)
		return nil
	}

	// Migrate RSVPs: update all ExperienceRSVP records where provisional_user_id matches.
	if err := migrateRSVPs(ctx, store, provisionalUserID, realUserID, logger); err != nil {
		return fmt.Errorf("failed to migrate RSVPs: %w", err)
	}

	// Migrate transfers: update all Transfer records where provisional_recipient_id matches.
	if err := migrateTransfers(ctx, store, provisionalUserID, realUserID, logger); err != nil {
		return fmt.Errorf("failed to migrate transfers: %w", err)
	}

	// Mark the provisional user as claimed.
	prov.ClaimedByUserId = &realUserID
	if err := store.Update(ctx, prov); err != nil {
		return fmt.Errorf("failed to mark provisional user as claimed: %w", err)
	}

	logger.InfoContext(ctx, "provisional user merged into real user account")
	return nil
}

// migrateTransfers migrates all Transfer records where provisional_recipient_id matches the
// provisional user to use the real user's ID as recipient_id instead.
func migrateTransfers(ctx context.Context, store *storage.ProtoSQLStorage, provisionalUserID, realUserID string, logger *logging.Logger) error {
	results, err := store.QueryByField(ctx, "provisional_recipient_id", provisionalUserID, &models.Transfer{})
	if err != nil {
		return fmt.Errorf("failed to query transfers for provisional user: %w", err)
	}

	for _, r := range results {
		transfer := r.(*models.Transfer)
		transfer.RecipientId = realUserID
		transfer.ProvisionalRecipientId = nil
		if err := store.Update(ctx, transfer); err != nil {
			return fmt.Errorf("failed to migrate transfer %s: %w", transfer.Id, err)
		}
	}

	if len(results) > 0 {
		logger.InfoContext(ctx, "migrated transfers from provisional user", "count", len(results))
	}
	return nil
}

// migrateRSVPs migrates all ExperienceRSVP records from a provisional user to a real user.
func migrateRSVPs(ctx context.Context, store *storage.ProtoSQLStorage, provisionalUserID, realUserID string, logger *logging.Logger) error {
	results, err := store.QueryByField(ctx, "provisional_user_id", provisionalUserID, &models.ExperienceRSVP{})
	if err != nil {
		return fmt.Errorf("failed to query RSVPs for provisional user: %w", err)
	}

	for _, r := range results {
		rsvp := r.(*models.ExperienceRSVP)

		// Check if the real user already has an RSVP for the same experience+community.
		// If so, keep the real user's RSVP and just delete the provisional one.
		existing, err := store.QueryByFields(ctx, map[string]any{
			"experience_id": rsvp.ExperienceId,
			"community_id":  rsvp.CommunityId,
			"user_id":       realUserID,
		}, &models.ExperienceRSVP{})
		if err != nil {
			return fmt.Errorf("failed to check existing RSVP: %w", err)
		}

		if len(existing) > 0 {
			// Real user already has an RSVP — soft-delete the provisional one.
			rsvp.ProvisionalUserId = nil
			if err := store.Delete(ctx, rsvp); err != nil {
				logger.WarnContext(ctx, "failed to delete conflicting provisional RSVP",
					"rsvp_id", rsvp.Id, "error", err)
			}
			continue
		}

		// Reassign the RSVP to the real user.
		rsvp.UserId = realUserID
		rsvp.ProvisionalUserId = nil
		if err := store.Update(ctx, rsvp); err != nil {
			return fmt.Errorf("failed to migrate RSVP %s: %w", rsvp.Id, err)
		}
	}

	if len(results) > 0 {
		logger.InfoContext(ctx, "migrated RSVPs from provisional user", "count", len(results))
	}
	return nil
}
