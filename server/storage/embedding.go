package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.ripls.org/ripls/server/ai/embedding"
	"go.ripls.org/ripls/server/logging"
)

// EmbeddingColumnName returns the default (empty-variant) embedding column name
// for a model. Format: vendor_model (with hyphens replaced by underscores),
// e.g. "local_ripls_minilm_v1". This is the column semantic search uses.
// Note: When updating the model, increment the version in embedding.DefaultInfo()
// to automatically create a new column for the updated model.
func EmbeddingColumnName(info *embedding.Info) string {
	return embeddingColumnName(info, "")
}

// embeddingColumnName returns the column name for a model + variant. The empty
// variant is the default column (vendor_model) for backward compatibility; a
// named variant appends "_<variant>" so one table can carry several embeddings
// of the same model (e.g. name+description for search and name-only for
// activity clustering, #2694).
func embeddingColumnName(info *embedding.Info, variant string) string {
	base := fmt.Sprintf("%s_%s", info.Vendor, strings.ReplaceAll(info.Model, "-", "_"))
	if variant == "" {
		return base
	}
	return base + "_" + strings.ReplaceAll(variant, "-", "_")
}

// EmbeddingIndexName returns the HNSW index name for a table and the default
// embedding column. HNSW (Hierarchical Navigable Small World) is a graph-based
// algorithm for fast approximate nearest neighbor search in high-dimensional
// vector spaces.
func EmbeddingIndexName(tableName string, info *embedding.Info) string {
	return embeddingIndexName(tableName, info, "")
}

func embeddingIndexName(tableName string, info *embedding.Info, variant string) string {
	return fmt.Sprintf("%s_%s_hnsw_idx", tableName, embeddingColumnName(info, variant))
}

// formatVector converts []float32 to PostgreSQL vector literal "[1.0,2.0,3.0]".
func formatVector(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// EnablePgvector enables the pgvector extension for vector similarity search.
// This is called automatically by InitializePostgreSQLDatabase.
func (s *ProtoSQLStorage) EnablePgvector(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		return fmt.Errorf("enable pgvector: %w", err)
	}
	return nil
}

// EnsureEmbeddingColumn creates the default embedding column and its HNSW index
// on the specified table. Idempotent. For non-default variants (or index-less
// columns) use ensureEmbeddingColumn.
func (s *ProtoSQLStorage) EnsureEmbeddingColumn(ctx context.Context, tableName string, info *embedding.Info) error {
	return s.ensureEmbeddingColumn(ctx, tableName, info, "", true)
}

// ensureEmbeddingColumn creates the embedding column for a model + variant, and
// (when createIndex is true) its HNSW ANN index. If the column exists with a
// different dimension it is dropped and recreated. Idempotent and safe to call
// repeatedly. Variants that are only read by id pass createIndex=false so no
// index is maintained on writes for a column no query ranks by distance.
func (s *ProtoSQLStorage) ensureEmbeddingColumn(ctx context.Context, tableName string, info *embedding.Info, variant string, createIndex bool) error {
	if err := validateIdentifier(info.Vendor); err != nil {
		return fmt.Errorf("embedding vendor: %w", err)
	}
	sanitizedModel := strings.ReplaceAll(info.Model, "-", "_")
	if err := validateIdentifier(sanitizedModel); err != nil {
		return fmt.Errorf("embedding model: %w", err)
	}
	if variant != "" {
		if err := validateIdentifier(strings.ReplaceAll(variant, "-", "_")); err != nil {
			return fmt.Errorf("embedding variant: %w", err)
		}
	}

	colName := embeddingColumnName(info, variant)
	indexName := embeddingIndexName(tableName, info, variant)

	// createIndexSQL builds (and runs) the HNSW index create, when requested.
	// m=16: bi-directional links per node (higher = better recall, more memory).
	// ef_construction=64: candidate-list size during build (higher = better
	// quality, slower build).
	createIndexIfWanted := func() error {
		if !createIndex {
			return nil
		}
		quotedIndex, err := quoteIdent(indexName)
		if err != nil {
			return err
		}
		quotedTable, err := quoteIdent(tableName)
		if err != nil {
			return err
		}
		quotedCol, err := quoteIdent(colName)
		if err != nil {
			return err
		}
		indexSQL := fmt.Sprintf(`
			CREATE INDEX IF NOT EXISTS %s
			ON %s USING hnsw (%s vector_cosine_ops)
			WITH (m = 16, ef_construction = 64)
		`, quotedIndex, quotedTable, quotedCol)
		if _, err := s.db.ExecContext(ctx, indexSQL); err != nil {
			return fmt.Errorf("create index %s: %w", indexName, err)
		}
		return nil
	}

	// Check if column exists and get its current dimension
	var existingDim int
	err := s.db.QueryRowContext(ctx, `
		SELECT atttypmod
		FROM pg_attribute
		WHERE attrelid = $1::regclass
		  AND attname = $2
		  AND NOT attisdropped
	`, tableName, colName).Scan(&existingDim)

	if err == nil {
		// Column exists - check if dimension matches
		// pgvector stores dimension in atttypmod directly
		if existingDim != info.Dimensions {
			logger := logging.LoggerWithContext(ctx)
			logger.WarnContext(ctx, "embedding dimension mismatch, recreating column",
				"table", tableName,
				"column", colName,
				"existing_dimension", existingDim,
				"expected_dimension", info.Dimensions,
			)

			// Drop the index first (if exists)
			quotedIndex, err := quoteIdent(indexName)
			if err != nil {
				return err
			}
			dropIndexSQL := fmt.Sprintf(`DROP INDEX IF EXISTS %s`, quotedIndex)
			if _, err := s.db.ExecContext(ctx, dropIndexSQL); err != nil {
				return fmt.Errorf("drop index %s: %w", indexName, err)
			}

			// Drop the column
			quotedTable, err := quoteIdent(tableName)
			if err != nil {
				return err
			}
			quotedCol, err := quoteIdent(colName)
			if err != nil {
				return err
			}
			dropColSQL := fmt.Sprintf(`ALTER TABLE %s DROP COLUMN IF EXISTS %s`, quotedTable, quotedCol)
			if _, err := s.db.ExecContext(ctx, dropColSQL); err != nil {
				return fmt.Errorf("drop column %s from %s: %w", colName, tableName, err)
			}
		} else {
			// Column exists with correct dimension - ensure index exists and return
			return createIndexIfWanted()
		}
	}
	// If err != nil, column doesn't exist - proceed to create it

	// Add embedding column
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return err
	}
	quotedCol, err := quoteIdent(colName)
	if err != nil {
		return err
	}
	// info.Dimensions is an int from the embedder's own metadata, not caller text.
	alterSQL := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s vector(%d)`,
		quotedTable, quotedCol, info.Dimensions)
	if _, err := s.db.ExecContext(ctx, alterSQL); err != nil {
		return fmt.Errorf("add column %s to %s: %w", colName, tableName, err)
	}

	return createIndexIfWanted()
}

// ListEmbeddingColumns returns all embedding column names in the specified table.
func (s *ProtoSQLStorage) ListEmbeddingColumns(ctx context.Context, tableName string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_name = $1
		  AND data_type = 'USER-DEFINED'
		  AND udt_name = 'vector'
	`, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		columns = append(columns, col)
	}
	return columns, rows.Err()
}

// DropEmbeddingColumn removes an embedding column and its index from the specified table.
func (s *ProtoSQLStorage) DropEmbeddingColumn(ctx context.Context, tableName string, info *embedding.Info) error {
	// Drop index first
	indexName := EmbeddingIndexName(tableName, info)
	quotedIndex, err := quoteIdent(indexName)
	if err != nil {
		return err
	}
	dropIdx := fmt.Sprintf(`DROP INDEX IF EXISTS %s`, quotedIndex)
	if _, err := s.db.ExecContext(ctx, dropIdx); err != nil {
		return fmt.Errorf("drop index %s: %w", indexName, err)
	}

	// Drop column
	colName := EmbeddingColumnName(info)
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return err
	}
	quotedCol, err := quoteIdent(colName)
	if err != nil {
		return err
	}
	dropCol := fmt.Sprintf(`ALTER TABLE %s DROP COLUMN IF EXISTS %s`, quotedTable, quotedCol)
	if _, err := s.db.ExecContext(ctx, dropCol); err != nil {
		return fmt.Errorf("drop column %s from %s: %w", colName, tableName, err)
	}

	return nil
}

// UpdateEmbedding stores a vector in the default embedding column for a record.
func (s *ProtoSQLStorage) UpdateEmbedding(ctx context.Context, tableName, id string, info *embedding.Info, emb []float32) error {
	return s.updateEmbedding(ctx, tableName, id, info, "", emb)
}

// updateEmbedding stores a vector in the given variant's embedding column.
func (s *ProtoSQLStorage) updateEmbedding(ctx context.Context, tableName, id string, info *embedding.Info, variant string, emb []float32) error {
	colName := embeddingColumnName(info, variant)
	vectorStr := formatVector(emb)

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return err
	}
	quotedCol, err := quoteIdent(colName)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`UPDATE %s SET %s = $1::vector WHERE id = $2`, quotedTable, quotedCol)
	result, err := s.db.ExecContext(ctx, query, vectorStr, id)
	if err != nil {
		return fmt.Errorf("update embedding: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("record not found with id %s: %w", id, ErrRecordNotFound)
	}

	return nil
}

// EmbeddingSearchResult contains a record ID and its similarity score.
type EmbeddingSearchResult struct {
	ID         string
	Similarity float64
}

// QueryByEmbeddingSimilarity finds records using vector similarity search.
// Returns results ordered by similarity (highest first).
// The similarity score is cosine similarity (1.0 = identical, 0.0 = orthogonal).
func (s *ProtoSQLStorage) QueryByEmbeddingSimilarity(
	ctx context.Context,
	tableName string,
	info *embedding.Info,
	queryEmbedding []float32,
	limit int,
) ([]EmbeddingSearchResult, error) {
	colName := EmbeddingColumnName(info)
	vectorStr := formatVector(queryEmbedding)

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}
	quotedCol, err := quoteIdent(colName)
	if err != nil {
		return nil, err
	}

	// Use cosine distance operator (<=>), convert to similarity (1 - distance)
	query := fmt.Sprintf(`
		SELECT id, 1 - (%s <=> $1::vector) as similarity
		FROM %s
		WHERE %s IS NOT NULL
		ORDER BY %s <=> $1::vector
		LIMIT $2
	`, quotedCol, quotedTable, quotedCol, quotedCol)

	rows, err := s.db.QueryContext(ctx, query, vectorStr, limit)
	if err != nil {
		return nil, fmt.Errorf("query by embedding: %w", err)
	}
	defer rows.Close()

	var results []EmbeddingSearchResult
	for rows.Next() {
		var r EmbeddingSearchResult
		if err := rows.Scan(&r.ID, &r.Similarity); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// CountWithEmbedding returns the total records and count with embeddings in the
// default embedding column.
func (s *ProtoSQLStorage) CountWithEmbedding(ctx context.Context, tableName string, info *embedding.Info) (total, withEmbedding int, err error) {
	return s.countWithEmbedding(ctx, tableName, info, "")
}

func (s *ProtoSQLStorage) countWithEmbedding(ctx context.Context, tableName string, info *embedding.Info, variant string) (total, withEmbedding int, err error) {
	colName := embeddingColumnName(info, variant)
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return 0, 0, err
	}
	quotedCol, err := quoteIdent(colName)
	if err != nil {
		return 0, 0, err
	}
	query := fmt.Sprintf(`
		SELECT COUNT(*) as total, COUNT(%s) as with_embedding
		FROM %s
	`, quotedCol, quotedTable)

	err = s.db.QueryRowContext(ctx, query).Scan(&total, &withEmbedding)
	return total, withEmbedding, err
}

// QueryWithoutEmbedding returns record IDs missing the default embedding.
// Used by backfill scripts.
func (s *ProtoSQLStorage) QueryWithoutEmbedding(ctx context.Context, tableName string, info *embedding.Info, limit int) ([]string, error) {
	return s.queryWithoutEmbedding(ctx, tableName, info, "", limit)
}

func (s *ProtoSQLStorage) queryWithoutEmbedding(ctx context.Context, tableName string, info *embedding.Info, variant string, limit int) ([]string, error) {
	colName := embeddingColumnName(info, variant)
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}
	quotedCol, err := quoteIdent(colName)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT id FROM %s WHERE %s IS NULL LIMIT $1`, quotedTable, quotedCol)

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ErrEmbeddingNotConfigured is returned when semantic search is attempted on a table without embedding config.
var ErrEmbeddingNotConfigured = errors.New("embedding not configured for this table")

// SetEmbeddingDoneChannel sets a channel that receives the outcome of async embedding.
// nil means all variants stored successfully; non-nil means at least one write failed.
// This is intended for testing only.
func (s *ProtoSQLStorage) SetEmbeddingDoneChannel(done chan error) {
	s.embeddingDone = done
}

// SetEmbedder configures the embedding generator and ensures columns exist.
// Should be called after initialization but before any Insert/Update operations.
func (s *ProtoSQLStorage) SetEmbedder(ctx context.Context, embedder *embedding.Embedder) error {
	s.embedder = embedder

	if embedder == nil {
		return nil
	}

	info := embedder.Info()

	// Validate vendor and model so embedding column names are safe SQL identifiers.
	if err := validateIdentifier(info.Vendor); err != nil {
		return fmt.Errorf("embedding vendor: %w", err)
	}
	// Model may contain hyphens; EmbeddingColumnName replaces them with underscores.
	// Validate after sanitization.
	sanitizedModel := strings.ReplaceAll(info.Model, "-", "_")
	if err := validateIdentifier(sanitizedModel); err != nil {
		return fmt.Errorf("embedding model: %w", err)
	}

	// Ensure an embedding column exists for every (table, variant) config.
	for tableName, configs := range s.embeddingConfigs {
		for _, cfg := range configs {
			if err := s.ensureEmbeddingColumn(ctx, tableName, info, cfg.Variant, !cfg.SkipIndex); err != nil {
				return fmt.Errorf("ensure embedding column for %s/%s (variant %q): %w", tableName, info.Model, cfg.Variant, err)
			}
		}
	}

	return nil
}

// GetEmbedder returns the configured embedding generator.
// Returns nil if no embedder is configured.
func (s *ProtoSQLStorage) GetEmbedder() *embedding.Embedder {
	return s.embedder
}

// extractEmbeddingText concatenates the configured fields into embedding text.
// Field names can be simple (e.g., "name") or nested with dots (e.g., "provider_image.description").
func (s *ProtoSQLStorage) extractEmbeddingText(msg proto.Message, config *EmbeddingFieldConfig) string {
	var parts []string
	for _, fieldPath := range config.Fields {
		value := extractNestedFieldValue(msg.ProtoReflect(), fieldPath)
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " ")
}

// extractNestedFieldValue extracts a string value from a proto message following a dot-separated path.
// For example, "provider_image.description" extracts the description from a nested provider_image message.
func extractNestedFieldValue(msgReflect protoreflect.Message, fieldPath string) string {
	pathParts := strings.Split(fieldPath, ".")
	current := msgReflect

	for i, part := range pathParts {
		descriptor := current.Descriptor()
		field := descriptor.Fields().ByName(protoreflect.Name(part))
		if field == nil {
			return ""
		}

		// For the last part, get the string value
		if i == len(pathParts)-1 {
			return current.Get(field).String()
		}

		// For intermediate parts, navigate into the nested message
		if field.Kind() != protoreflect.MessageKind {
			return ""
		}
		if !current.Has(field) {
			return ""
		}
		current = current.Get(field).Message()
	}
	return ""
}

// generateEmbedding creates one variant's embedding for a record and stores it.
// Returns an error on failure; callers log it and collect via errors.Join.
// Fallback on any failure: the nightly BackfillEmbeddings run will fill the NULL column.
// The per-row completion signal is emitted by queueEmbeddingGeneration, not here,
// so one insert produces exactly one signal regardless of how many variants it has.
func (s *ProtoSQLStorage) generateEmbedding(ctx context.Context, tableName, id, variant, text string) error {
	if text == "" || s.embedder == nil {
		return nil
	}

	logger := logging.LoggerWithContext(ctx)
	info := s.embedder.Info()

	emb, err := s.embedder.Generate(ctx, text)
	if err != nil {
		// Fallback: nightly BackfillEmbeddings will fill the NULL column.
		logger.WarnContext(ctx, "embedding generation failed",
			"model", info.Model,
			"table", tableName,
			"variant", variant,
			"id", id,
			"error", err,
		)
		return err
	}

	if err := s.updateEmbedding(ctx, tableName, id, info, variant, emb); err != nil {
		// Fallback: nightly BackfillEmbeddings will fill the NULL column.
		logger.WarnContext(ctx, "embedding storage failed",
			"model", info.Model,
			"table", tableName,
			"variant", variant,
			"id", id,
			"error", err,
		)
		return err
	}
	return nil
}

// queueEmbeddingGeneration schedules async generation of every configured variant's
// embedding for a just-inserted/updated row. No-op when no embedder is set or the
// table has no embedding config. Emits exactly one embeddingDone signal per
// configured row (after all variants), so tests that count signals per insert
// stay correct as variants are added. Variant text is extracted synchronously
// (before scheduling) so it reflects this row's state, not a later mutation.
//
// When called inside a WithTx callback, the goroutine is deferred until after the
// transaction commits (via deferAfterCommit), so the row is always visible on the
// connection pool before the UPDATE runs. Outside a transaction, the goroutine
// starts immediately as before.
func (s *ProtoSQLStorage) queueEmbeddingGeneration(ctx context.Context, tableName string, msg proto.Message, id string) {
	configs := s.embeddingConfigs[tableName]
	if s.embedder == nil || len(configs) == 0 {
		return
	}

	type variantText struct{ variant, text string }
	jobs := make([]variantText, 0, len(configs))
	for _, cfg := range configs {
		jobs = append(jobs, variantText{cfg.Variant, s.extractEmbeddingText(msg, cfg)})
	}

	s.deferAfterCommit(func() {
		logging.GoSafe(ctx, "storage-generate-embedding", func() {
			bg := context.Background()
			var errs []error
			for _, j := range jobs {
				if err := s.generateEmbedding(bg, tableName, id, j.variant, j.text); err != nil {
					errs = append(errs, err)
				}
			}
			if s.embeddingDone != nil {
				s.embeddingDone <- errors.Join(errs...)
			}
		})
	})
}

// EmbeddingStatus contains statistics about embedding coverage for one table's
// embedding variant.
type EmbeddingStatus struct {
	TableName     string
	Variant       string
	Total         int
	WithEmbedding int
	Missing       int
}

// GetEmbeddingStatus returns embedding coverage statistics for every configured
// (table, variant).
func (s *ProtoSQLStorage) GetEmbeddingStatus(ctx context.Context) ([]EmbeddingStatus, error) {
	if s.embedder == nil {
		return nil, nil
	}

	info := s.embedder.Info()
	var statuses []EmbeddingStatus

	for tableName, configs := range s.embeddingConfigs {
		for _, cfg := range configs {
			total, withEmb, err := s.countWithEmbedding(ctx, tableName, info, cfg.Variant)
			if err != nil {
				return nil, fmt.Errorf("count embeddings for %s (variant %q): %w", tableName, cfg.Variant, err)
			}
			statuses = append(statuses, EmbeddingStatus{
				TableName:     tableName,
				Variant:       cfg.Variant,
				Total:         total,
				WithEmbedding: withEmb,
				Missing:       total - withEmb,
			})
		}
	}

	return statuses, nil
}

// BackfillEmbeddings generates embeddings for records missing them.
// Runs in background - safe to restart anytime, only processes NULL embeddings.
// Logs progress at start and completion. Honors ctx cancellation: returns
// promptly when ctx is done so callers (e.g. main.go on SIGTERM) can shut
// down without leaking the goroutine or in-flight embedder work.
func (s *ProtoSQLStorage) BackfillEmbeddings(ctx context.Context) {
	if s.embedder == nil {
		return
	}

	logger := logging.LoggerWithContext(ctx)
	info := s.embedder.Info()

	if err := ctx.Err(); err != nil {
		logger.InfoContext(ctx, "embedding backfill cancelled before start", "model", info.Model)
		return
	}

	// Log initial status
	statuses, err := s.GetEmbeddingStatus(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			logger.InfoContext(ctx, "embedding backfill cancelled", "model", info.Model)
			return
		}
		logger.WarnContext(ctx, "failed to get embedding status", "error", err)
	} else {
		totalMissing := 0
		for _, status := range statuses {
			totalMissing += status.Missing
			if status.Missing > 0 {
				logger.InfoContext(ctx, "embedding backfill needed",
					"table", status.TableName,
					"variant", status.Variant,
					"total", status.Total,
					"with_embedding", status.WithEmbedding,
					"missing", status.Missing,
					"model", info.Model,
				)
			}
		}
		if totalMissing == 0 {
			logger.InfoContext(ctx, "all embeddings up to date", "model", info.Model)
			return
		}
	}

	// Backfill each (table, variant).
	for tableName, configs := range s.embeddingConfigs {
		for _, cfg := range configs {
			if err := ctx.Err(); err != nil {
				logger.InfoContext(ctx, "embedding backfill cancelled", "model", info.Model)
				return
			}
			if err := s.backfillTable(ctx, tableName, cfg, info); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					logger.InfoContext(ctx, "embedding backfill cancelled",
						"table", tableName,
						"variant", cfg.Variant,
						"model", info.Model,
					)
					return
				}
				logger.WarnContext(ctx, "backfill failed",
					"table", tableName,
					"variant", cfg.Variant,
					"model", info.Model,
					"error", err,
				)
			}
		}
	}

	// Log completion status
	statuses, err = s.GetEmbeddingStatus(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		logger.WarnContext(ctx, "failed to get final embedding status", "error", err)
	} else {
		for _, status := range statuses {
			logger.InfoContext(ctx, "embedding backfill complete",
				"table", status.TableName,
				"variant", status.Variant,
				"total", status.Total,
				"with_embedding", status.WithEmbedding,
				"model", info.Model,
			)
		}
	}
}

func (s *ProtoSQLStorage) backfillTable(
	ctx context.Context,
	tableName string,
	config *EmbeddingFieldConfig,
	info *embedding.Info,
) error {
	batchSize := 100
	logger := logging.LoggerWithContext(ctx)
	failedIDs := make(map[string]int) // Track failure counts per ID
	const maxFailures = 3             // Max consecutive failures before skipping an ID

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ids, err := s.queryWithoutEmbedding(ctx, tableName, info, config.Variant, batchSize)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			break
		}

		logger.InfoContext(ctx, "backfilling embeddings",
			"count", len(ids),
			"table", tableName,
			"variant", config.Variant,
			"model", info.Model,
		)

		successCount := 0
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return err
			}
			// Skip IDs that have failed too many times
			if failedIDs[id] >= maxFailures {
				logger.WarnContext(ctx, "skipping record after repeated failures",
					"table", tableName,
					"id", id,
					"failure_count", failedIDs[id],
				)
				continue
			}

			text, err := s.getEmbeddingTextByID(ctx, tableName, config, id)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				logger.WarnContext(ctx, "failed to get record for embedding",
					"table", tableName,
					"id", id,
					"error", err,
				)
				failedIDs[id]++
				continue
			}

			if text == "" {
				failedIDs[id]++
				continue
			}

			emb, err := s.embedder.Generate(ctx, text)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				logger.WarnContext(ctx, "embedding generation failed",
					"table", tableName,
					"id", id,
					"error", err,
				)
				failedIDs[id]++
				continue
			}

			if err := s.updateEmbedding(ctx, tableName, id, info, config.Variant, emb); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				logger.WarnContext(ctx, "embedding storage failed",
					"table", tableName,
					"id", id,
					"error", err,
				)
				failedIDs[id]++
			} else {
				// Success - clear failure count
				delete(failedIDs, id)
				successCount++
			}

			// Rate limiting; bail immediately on cancellation rather than
			// blocking for the full sleep window.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}

		// If we made no progress in this batch, break to avoid infinite loop
		if successCount == 0 && len(ids) > 0 {
			logger.WarnContext(ctx, "no successful embeddings in batch, stopping backfill",
				"table", tableName,
				"batch_size", len(ids),
				"failed_ids", len(failedIDs),
			)
			break
		}
	}
	return nil
}

// embeddingTextColumn maps an EmbeddingFieldConfig.Fields entry to the SQL
// column that holds it.
//
// Those entries are proto field *paths*, which may be dotted for nested
// messages ("provider_image.description"). extractEmbeddingText — the write
// path — walks them with protoreflect. The read path below has to name a
// column instead, and flattenFields stores a nested field under
// prefix + "_" + name. Translating here is what keeps the two agreeing.
//
// This was a live bug before #2795: the read path quoted the dotted path
// straight into the SELECT, so stock_image's backfill asked for a column named
// "provider_image.description" that has never existed. The error was swallowed
// by BackfillEmbeddings' per-record warn-and-continue, so stock_image
// embeddings simply never backfilled.
func embeddingTextColumn(fieldPath string) string {
	return sanitizeColumnName(strings.ReplaceAll(fieldPath, ".", "_"))
}

// getEmbeddingTextByID fetches a record and extracts embedding text from configured fields.
func (s *ProtoSQLStorage) getEmbeddingTextByID(ctx context.Context, tableName string, config *EmbeddingFieldConfig, id string) (string, error) {
	// Build query to select the configured fields, quoting each column name.
	columns := make([]string, len(config.Fields))
	for i, f := range config.Fields {
		columns[i] = embeddingTextColumn(f)
	}
	quotedFields, err := quoteIdents(columns)
	if err != nil {
		return "", err
	}
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return "", err
	}
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = %s`,
		strings.Join(quotedFields, ", "), quotedTable, s.dbSpec.Placeholder(1))

	row := s.db.QueryRowContext(ctx, query, id)

	// Scan into string slice
	values := make([]sql.NullString, len(config.Fields))
	scanDest := make([]any, len(config.Fields))
	for i := range values {
		scanDest[i] = &values[i]
	}

	if err := row.Scan(scanDest...); err != nil {
		return "", err
	}

	// Concatenate non-empty values
	var parts []string
	for _, v := range values {
		if v.Valid && v.String != "" {
			parts = append(parts, v.String)
		}
	}
	return strings.Join(parts, " "), nil
}

// QueryBySemanticSearch performs semantic similarity search on any table with embedding config.
// Returns ErrEmbeddingNotConfigured if the table doesn't support embeddings.
func (s *ProtoSQLStorage) QueryBySemanticSearch(
	ctx context.Context,
	msgType proto.Message,
	info *embedding.Info,
	queryEmbedding []float32,
	limit int,
) ([]EmbeddingSearchResult, error) {
	descriptor := msgType.ProtoReflect().Descriptor()
	typeName := string(descriptor.FullName())

	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return nil, fmt.Errorf("message type %s is not registered for storage", typeName)
	}

	// Verify this table has embedding configuration
	if len(s.embeddingConfigs[tableName]) == 0 {
		return nil, ErrEmbeddingNotConfigured
	}

	return s.QueryByEmbeddingSimilarity(ctx, tableName, info, queryEmbedding, limit)
}
