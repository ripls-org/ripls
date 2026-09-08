package emailsuppression

import (
	"context"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// IsSuppressed reports whether the email address has opted out of platform
// email. The address is normalized before lookup so it matches regardless of
// input casing.
func IsSuppressed(ctx context.Context, st *storage.ProtoSQLStorage, email string) (bool, error) {
	normalized := auth.NormalizeEmail(email)
	if normalized == "" {
		return false, nil
	}
	rows, err := storage.QueryByField[*models.EmailSuppression](st, ctx, "email", normalized)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// Suppress records that the email address has opted out of platform email. It is
// idempotent: an already-suppressed address is left unchanged.
func Suppress(ctx context.Context, st *storage.ProtoSQLStorage, email string) error {
	normalized := auth.NormalizeEmail(email)
	if normalized == "" {
		return nil
	}
	existing, err := storage.QueryByField[*models.EmailSuppression](st, ctx, "email", normalized)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	_, err = st.Insert(ctx, &models.EmailSuppression{
		Email:             normalized,
		OptedOutAtUnixSec: clock.UnixSec(ctx),
	})
	return err
}
