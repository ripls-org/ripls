package scheduled_notifications

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// userPrefsLoader is the function signature satisfied by
// server/services/user.LoadUserNotificationPreferences. Passed in by
// main.go to avoid importing the user-service package from this job
// package (which would invert the dependency direction).
type userPrefsLoader func(ctx context.Context, s *storage.ProtoSQLStorage, userID string) (*models.UserNotificationPreferences, error)

// StorageLoanWorld is the production LoanWorld backed by
// ProtoSQLStorage. The userPrefsLoader is injected so the package
// stays decoupled from the user service.
type StorageLoanWorld struct {
	storage   *storage.ProtoSQLStorage
	loadPrefs userPrefsLoader
}

// NewStorageLoanWorld constructs a LoanWorld for production wiring.
func NewStorageLoanWorld(s *storage.ProtoSQLStorage, loadPrefs userPrefsLoader) *StorageLoanWorld {
	return &StorageLoanWorld{storage: s, loadPrefs: loadPrefs}
}

// ListActiveLoansWithExpectedReturn returns every Transfer that is
// in TRANSFER_STATE_ACTIVE with a non-empty expected_return_unix_sec.
// Backed by the partial index on transfer.expected_return_unix_sec
// added alongside the field.
func (w *StorageLoanWorld) ListActiveLoansWithExpectedReturn(ctx context.Context) ([]*models.Transfer, error) {
	msgs, err := w.storage.QueryByField(ctx, "state",
		fmt.Sprintf("%d", int(models.TransferState_TRANSFER_STATE_ACTIVE)),
		&models.Transfer{})
	if err != nil {
		return nil, err
	}
	out := make([]*models.Transfer, 0, len(msgs))
	for _, m := range msgs {
		t := m.(*models.Transfer)
		if t.ExpectedReturnUnixSec == nil {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// GetTransfer loads one Transfer by id.
func (w *StorageLoanWorld) GetTransfer(ctx context.Context, transferID string) (*models.Transfer, error) {
	t := &models.Transfer{}
	if err := w.storage.GetByID(ctx, transferID, t); err != nil {
		return nil, err
	}
	return t, nil
}

// GetGear loads one Gear by id.
func (w *StorageLoanWorld) GetGear(ctx context.Context, gearID string) (*models.Gear, error) {
	g := &models.Gear{}
	if err := w.storage.GetByID(ctx, gearID, g); err != nil {
		return nil, err
	}
	return g, nil
}

// GetUserDisplayName returns the user's stored Name, or "" on any
// lookup error. Used only for copy-rendering; failures fall back to
// generic copy rather than blocking the push.
func (w *StorageLoanWorld) GetUserDisplayName(ctx context.Context, userID string) (string, error) {
	u := &models.User{}
	if err := w.storage.GetByID(ctx, userID, u); err != nil {
		return "", err
	}
	return u.Name, nil
}

// GetUserNotificationPreferences delegates to the injected loader.
func (w *StorageLoanWorld) GetUserNotificationPreferences(ctx context.Context, userID string) (*models.UserNotificationPreferences, error) {
	if w.loadPrefs == nil {
		return nil, nil
	}
	return w.loadPrefs(ctx, w.storage, userID)
}
