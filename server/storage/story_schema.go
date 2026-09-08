package storage

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// InitializeStoryArrayColumns idempotently runs the DDL for every
// denormalized TEXT[] column on the Story type — currently just
// participant_ids, which powers StoryStorage.ListByUser. One-time-
// effective per database.
//
// DDL only — does NOT register the per-write sync. Pair every startup
// call with [ProtoSQLStorage.RegisterStoryArrayColumns] (which must
// run every process start regardless of schema state).
func (s *ProtoSQLStorage) InitializeStoryArrayColumns(ctx context.Context) error {
	if !s.isTypeRegistered(&models.Story{}) {
		return nil
	}
	extractor := TopLevelRepeatedString(&models.Story{}, "participant_ids")
	if err := s.InitializeArrayColumn(ctx, &models.Story{}, "participant_ids", extractor); err != nil {
		return fmt.Errorf("initialize story.participant_ids: %w", err)
	}
	return nil
}

// RegisterStoryArrayColumns registers the per-write auto-sync for every
// Story TEXT[] denormalization column. Required on every server start:
// without it, StoryStorage.Insert leaves participant_ids stale and
// ListByUser silently misses rows.
//
// Safe no-op when Story is not registered for storage.
func (s *ProtoSQLStorage) RegisterStoryArrayColumns() {
	if !s.isTypeRegistered(&models.Story{}) {
		return
	}
	s.RegisterArrayColumn(&models.Story{}, "participant_ids", TopLevelRepeatedString(&models.Story{}, "participant_ids"))
}
