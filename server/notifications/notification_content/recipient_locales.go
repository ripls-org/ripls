package notification_content

import (
	"context"

	"golang.org/x/text/language"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// UserBatchReader is the storage surface LoadRecipientLocales needs: one
// batched read of User rows by id. *storage.ProtoSQLStorage satisfies it;
// tests pass an in-memory fake.
type UserBatchReader interface {
	GetByIDs(ctx context.Context, ids []string, msgType proto.Message, opts ...storage.QueryOptions) (map[string]proto.Message, error)
}

// LoadRecipientLocales returns a userID → locale map for recipients, read
// from User.preferred_language in a single batched query. Every recipient is
// present in the result: an empty or unsupported preference, a user row that
// is missing, or a failed batch read all resolve to l10n.DefaultTag, so a
// locale lookup failure degrades a push to English rather than dropping it.
func LoadRecipientLocales(ctx context.Context, users UserBatchReader, recipients []string) map[string]language.Tag {
	out := make(map[string]language.Tag, len(recipients))
	if len(recipients) == 0 {
		return out
	}
	for _, id := range recipients {
		out[id] = l10n.DefaultTag
	}
	if users == nil {
		return out
	}

	msgs, err := users.GetByIDs(ctx, recipients, &models.User{})
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"failed to batch-load recipient users for locale resolution; defaulting to English",
			"recipient_count", len(recipients), "error", err,
		)
		return out
	}
	for _, msg := range msgs {
		u, ok := msg.(*models.User)
		if !ok || u == nil {
			continue
		}
		if _, wanted := out[u.Id]; !wanted {
			continue
		}
		out[u.Id] = l10n.Normalize(u.GetPreferredLanguage())
	}
	return out
}
