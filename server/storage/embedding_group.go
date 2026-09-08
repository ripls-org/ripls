package storage

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai/embedding"
)

// GroupIDsByStoredEmbedding partitions a caller-supplied set of record ids into
// groups whose stored embeddings (for the given model + variant) are mutually
// similar — a small in-process clustering primitive. It reads the stored vectors
// for `ids` (one indexed by-id query on the variant column), then greedy-clusters
// them by cosine similarity: each id joins the first existing group whose
// representative (first member's) vector scores at least `minSimilarity`, else it
// seeds a new group.
//
// Ids whose embedding is NULL/missing (async generation lag, empty embed text)
// come back as their own singleton groups. Grouping is deterministic (ids are
// sorted first). Keeping the vector read + cosine math here (never exposing
// []float32 to services) upholds the storage-layer-encapsulation principle in
// docs/server/semantic_search.md. Returns ErrEmbeddingNotConfigured when the
// resolved table has no embedding configuration.
func (s *ProtoSQLStorage) GroupIDsByStoredEmbedding(
	ctx context.Context,
	msgType proto.Message,
	info *embedding.Info,
	variant string,
	ids []string,
	minSimilarity float64,
) ([][]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	typeName := string(msgType.ProtoReflect().Descriptor().FullName())
	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return nil, fmt.Errorf("message type %s is not registered for storage", typeName)
	}
	if len(s.embeddingConfigs[tableName]) == 0 {
		return nil, ErrEmbeddingNotConfigured
	}

	vecs, err := s.embeddingVectorsByIDs(ctx, tableName, info, variant, ids)
	if err != nil {
		return nil, fmt.Errorf("read %s embeddings (variant %q): %w", tableName, variant, err)
	}

	return clusterByCosine(ids, vecs, minSimilarity), nil
}

// embeddingVectorsByIDs reads stored embedding vectors for the given ids from the
// variant's embedding column. Ids with a NULL vector are omitted from the result
// (the caller treats a missing entry as "no embedding yet").
func (s *ProtoSQLStorage) embeddingVectorsByIDs(
	ctx context.Context,
	tableName string,
	info *embedding.Info,
	variant string,
	ids []string,
) (map[string][]float32, error) {
	colName := embeddingColumnName(info, variant)

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = s.dbSpec.Placeholder(i + 1)
		args[i] = id
	}

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}
	quotedCol, err := quoteIdent(colName)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(
		`SELECT id, %s FROM %s WHERE id IN (%s) AND %s IS NOT NULL`,
		quotedCol, quotedTable, strings.Join(placeholders, ", "), quotedCol,
	)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query embeddings: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]float32, len(ids))
	for rows.Next() {
		var id, vecStr string
		if err := rows.Scan(&id, &vecStr); err != nil {
			return nil, fmt.Errorf("scan embedding row: %w", err)
		}
		if vec := parseVector(vecStr); len(vec) > 0 {
			out[id] = vec
		}
	}
	return out, rows.Err()
}

// parseVector parses a pgvector text value ("[0.1,0.2,0.3]") into a []float32.
// Returns nil for empty or malformed input.
func parseVector(s string) []float32 {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]float32, 0, len(parts))
	for _, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return nil
		}
		out = append(out, float32(f))
	}
	return out
}

// clusterByCosine greedily groups ids by cosine similarity of their vectors.
// An id with no vector (missing from vecs) becomes its own singleton group and
// never absorbs other ids. Deterministic: ids are processed in sorted order and
// each id joins the first eligible group. Pure — unit-tested with synthetic
// vectors, no database or model needed.
func clusterByCosine(ids []string, vecs map[string][]float32, minSimilarity float64) [][]string {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)

	var groups [][]string
	var reps [][]float32 // representative (first-member) vector per group; nil for singleton no-vector groups

	for _, id := range sorted {
		vec := vecs[id]
		if len(vec) == 0 {
			groups = append(groups, []string{id})
			reps = append(reps, nil)
			continue
		}
		placed := false
		for gi, rep := range reps {
			if rep == nil {
				continue
			}
			if float64(embedding.CosineSimilarity(vec, rep)) >= minSimilarity {
				groups[gi] = append(groups[gi], id)
				placed = true
				break
			}
		}
		if !placed {
			groups = append(groups, []string{id})
			reps = append(reps, vec)
		}
	}
	return groups
}
