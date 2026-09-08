package smsoptout

import (
	"context"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/contact"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// IsOptedOut reports whether the phone number has opted out of all platform
// text messages. The number is normalized to E.164 before lookup. An
// unparseable number is treated as not-opted-out (there is no record to match);
// the downstream send will fail on its own if the number is truly invalid.
func IsOptedOut(ctx context.Context, st *storage.ProtoSQLStorage, phone string) (bool, error) {
	normalized, err := contact.NormalizePhoneE164(phone)
	if err != nil {
		return false, nil
	}
	row, err := lookup(ctx, st, normalized)
	if err != nil || row == nil {
		return false, err
	}
	return row.OptedOut, nil
}

// RecordOptOut marks the number opted out (STOP). Idempotent: a number already
// opted out is left unchanged. Returns an error only on a storage failure; an
// unparseable number is a no-op.
func RecordOptOut(ctx context.Context, st *storage.ProtoSQLStorage, phone string) error {
	return set(ctx, st, phone, true)
}

// RecordOptIn clears the opt-out (START), re-enabling sends. Idempotent: a
// number that was never opted out (or already opted back in) is left unchanged.
func RecordOptIn(ctx context.Context, st *storage.ProtoSQLStorage, phone string) error {
	return set(ctx, st, phone, false)
}

// set upserts the opt-out state, writing only on an actual transition so repeat
// STOP/START keywords don't churn the row.
func set(ctx context.Context, st *storage.ProtoSQLStorage, phone string, optedOut bool) error {
	normalized, err := contact.NormalizePhoneE164(phone)
	if err != nil {
		return nil
	}
	now := clock.UnixSec(ctx)

	existing, err := lookup(ctx, st, normalized)
	if err != nil {
		return err
	}
	if existing == nil {
		rec := &models.SmsOptOut{
			PhoneNumber:      normalized,
			OptedOut:         optedOut,
			UpdatedAtUnixSec: now,
		}
		if optedOut {
			rec.OptedOutAtUnixSec = now
		}
		_, err = st.Insert(ctx, rec)
		return err
	}
	if existing.OptedOut == optedOut {
		return nil // no transition; nothing to persist
	}
	existing.OptedOut = optedOut
	existing.UpdatedAtUnixSec = now
	if optedOut {
		existing.OptedOutAtUnixSec = now
	}
	return st.Update(ctx, existing)
}

// lookup returns the single preference row for a normalized number, or nil when
// none exists. A unique index on phone_number guarantees at most one row.
func lookup(ctx context.Context, st *storage.ProtoSQLStorage, normalized string) (*models.SmsOptOut, error) {
	rows, err := storage.QueryByField[*models.SmsOptOut](st, ctx, "phone_number", normalized)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}
