package storage

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// mediaIdsTopLevelTypes lists proto types that carry a top-level
// repeated `media_ids` field. Each gets a denormalized TEXT[] column
// of the same name plus a GIN index so the GetMedia access-control
// check (#1529) can answer "is this media id surfaced on any X the
// caller can reach?" with a single indexed lookup.
var mediaIdsTopLevelTypes = []proto.Message{
	&models.User{},
	&models.Gear{},
	&models.Experience{},
	&models.Request{},
	&models.Community{},
	&models.Story{},
}

// chatMessageMediaIds extracts the media_ids attached to a [ChatMessage].
// They live inside the user_message variant of the oneof; system messages
// have none.
func chatMessageMediaIds(msg proto.Message) []string {
	cm, ok := msg.(*models.ChatMessage)
	if !ok || cm == nil {
		return nil
	}
	user := cm.GetUserMessage()
	if user == nil {
		return nil
	}
	return user.GetMediaIds()
}

// RegisterMediaIdsArrayColumns registers the `media_ids` TEXT[]
// denormalization for every type listed in mediaIdsTopLevelTypes plus
// ChatMessage (whose values live in the nested UserChatMessage variant).
//
// Types that aren't registered for storage in this instance (e.g. a test
// using a narrow TypeConfig list) are skipped — registration is a no-op
// for them since no Insert / Update would ever route through.
//
// Call once at storage construction time, after [initializeDatabase] has
// registered the proto types. Subsequent Insert / Update calls on these
// types atomically sync the column from the proto.
func (s *ProtoSQLStorage) RegisterMediaIdsArrayColumns() {
	for _, msg := range mediaIdsTopLevelTypes {
		if !s.isTypeRegistered(msg) {
			continue
		}
		s.RegisterArrayColumn(msg, "media_ids", TopLevelRepeatedString(msg, "media_ids"))
	}
	if s.isTypeRegistered(&models.ChatMessage{}) {
		s.RegisterArrayColumn(&models.ChatMessage{}, "media_ids", chatMessageMediaIds)
	}
}

// InitializeMediaIdsColumns idempotently creates the `media_ids` TEXT[]
// column, backfills it from binary_proto, and creates a GIN index on
// every table whose proto type was registered for storage. Types not
// registered in this instance are skipped silently so the helper is safe
// to call regardless of TypeConfig.
func (s *ProtoSQLStorage) InitializeMediaIdsColumns(ctx context.Context) error {
	for _, msg := range mediaIdsTopLevelTypes {
		if !s.isTypeRegistered(msg) {
			continue
		}
		extract := TopLevelRepeatedString(msg, "media_ids")
		if err := s.InitializeArrayColumn(ctx, msg, "media_ids", extract); err != nil {
			return fmt.Errorf("initialize media_ids for %T: %w", msg, err)
		}
	}
	if s.isTypeRegistered(&models.ChatMessage{}) {
		if err := s.InitializeArrayColumn(ctx, &models.ChatMessage{}, "media_ids", chatMessageMediaIds); err != nil {
			return fmt.Errorf("initialize media_ids for ChatMessage: %w", err)
		}
	}
	return nil
}
