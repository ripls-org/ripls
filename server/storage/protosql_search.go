// Community search: config-driven semantic and text search across gear, requests, and experiences.

package storage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// communitySearchConfig defines the parameters that vary between search functions.
//
// INVARIANT: every field here must be a compile-time literal, set only by the
// config constructors below. Several are spliced into the query as raw SQL
// fragments, which is why their call sites carry `sql-fragment-allow` markers.
// If a field ever needs to carry a runtime value, bind it with
// dbSpec.Placeholder instead of widening these fields — the markers assert
// that no caller input reaches them, and check-sql cannot verify that for you.
type communitySearchConfig struct {
	// itemType identifies the result type ("gear", "request", or "experience").
	itemType string
	// entityType is the proto type name (e.g., "ripls.models.Gear").
	entityType string
	// joinType is the junction table proto type name (e.g., "ripls.models.CommunityGear").
	joinType string
	// joinColumn is the column in the junction table that references the entity (e.g., "gear_id").
	joinColumn string
	// searchColumns are the entity columns to ILIKE against (e.g., ["name", "description"]).
	searchColumns []string
	// extraWhereSQL is appended to the WHERE clause (e.g., "AND cr.archived = false").
	extraWhereSQL string
	// hasAvailability selects cg.availability from the join table when true.
	hasAvailability bool
	// orderBySQL is appended after the WHERE clause for text queries (e.g., "ORDER BY cg.created_at_unix_sec DESC").
	orderBySQL string
	// limitSQL is appended after ORDER BY (e.g., "LIMIT 100").
	limitSQL string
}

// --- Config definitions ---.

// notDeletedFilter is the standard WHERE-clause fragment used to exclude soft-deleted
// rows from raw-SQL search queries. The generic readers (QueryByField, GetByID, ...)
// get the same behavior automatically via buildDeletedFilter; these search paths use
// raw joins so they must apply the filter explicitly.
const notDeletedFilter = "AND COALESCE(e.deleted_deleted_at_unix_sec, 0) = 0 AND COALESCE(j.deleted_deleted_at_unix_sec, 0) = 0"

func gearSearchConfig() communitySearchConfig {
	return communitySearchConfig{
		itemType:      "gear",
		entityType:    "ripls.models.Gear",
		joinType:      "ripls.models.CommunityGear",
		joinColumn:    "gear_id",
		searchColumns: []string{"name", "description"},
		extraWhereSQL: notDeletedFilter,
	}
}

func requestSearchConfig() communitySearchConfig {
	return communitySearchConfig{
		itemType:      "request",
		entityType:    "ripls.models.Request",
		joinType:      "ripls.models.CommunityRequest",
		joinColumn:    "request_id",
		searchColumns: []string{"title", "description"},
		extraWhereSQL: "AND j.archived = false AND e.state = '1' " + notDeletedFilter,
	}
}

func unifiedGearConfig() communitySearchConfig {
	return communitySearchConfig{
		itemType:        "gear",
		entityType:      "ripls.models.Gear",
		joinType:        "ripls.models.CommunityGear",
		joinColumn:      "gear_id",
		searchColumns:   []string{"name", "description"},
		hasAvailability: true,
		extraWhereSQL:   notDeletedFilter,
		orderBySQL:      "ORDER BY j.created_at_unix_sec DESC",
		limitSQL:        "LIMIT 100",
	}
}

func unifiedRequestConfig(includeCompleted bool) communitySearchConfig {
	extraWhere := "AND j.archived = false " + notDeletedFilter
	if !includeCompleted {
		extraWhere += " AND e.state IN ('1', '2')"
	}
	return communitySearchConfig{
		itemType:      "request",
		entityType:    "ripls.models.Request",
		joinType:      "ripls.models.CommunityRequest",
		joinColumn:    "request_id",
		searchColumns: []string{"title", "description"},
		extraWhereSQL: extraWhere,
		orderBySQL:    "ORDER BY j.shared_at_unix_sec DESC",
	}
}

func unifiedExperienceConfig(includeCompleted bool) communitySearchConfig {
	extraWhere := notDeletedFilter
	if !includeCompleted {
		// Exclude COMPLETED (4) and CANCELLED (5); keep ACTIVE (1), JOINED (2), IN_PROCESS (3).
		extraWhere += " AND e.state IN ('1', '2', '3')"
	}
	return communitySearchConfig{
		itemType:      "experience",
		entityType:    "ripls.models.Experience",
		joinType:      "ripls.models.CommunityExperience",
		joinColumn:    "experience_id",
		searchColumns: []string{"name", "description"},
		extraWhereSQL: extraWhere,
		orderBySQL:    "ORDER BY j.shared_at_unix_sec DESC",
	}
}

// likeEscapeChar is the character escapeLikePattern inserts before a literal
// wildcard, and likeEscapeClause is the matching SQL suffix that declares it.
//
// Backslash is PostgreSQL's default for LIKE, but declaring it explicitly
// keeps the pattern correct regardless of standard_conforming_strings. Note
// that ESCAPE takes exactly ONE character: writing `ESCAPE '\\'` inside a raw
// Go string literal sends two backslashes and PostgreSQL rejects it with
// "invalid escape string" (22025). Keeping the clause in one constant is what
// stops the three ILIKE sites from disagreeing about that.
const (
	likeEscapeChar   = `\`
	likeEscapeClause = `ESCAPE '\'`
)

// escapeLikePattern neutralises the LIKE wildcards in user-supplied search
// text: `%` (any run of characters), `_` (any single character), and the
// escape character itself.
//
// The search term has always been *bound*, so this was never an injection
// hole. It was a correctness and cost bug: a member who typed `%` matched
// every row in the community, and `_` matched any single character, so
// "a_c" silently matched "abc". Both also defeat any index the planner would
// otherwise use for a prefix match.
func escapeLikePattern(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '%', '_', '\\':
			b.WriteString(likeEscapeChar)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// likeContains wraps an escaped search term in the substring wildcards, giving
// the `%term%` pattern the ILIKE clauses expect.
func likeContains(s string) string {
	return "%" + escapeLikePattern(s) + "%"
}

// --- Generic executors ---.

// executeSemanticSearch runs a vector similarity search using the given config.
func (s *ProtoSQLStorage) executeSemanticSearch(
	ctx context.Context,
	cfg communitySearchConfig,
	colName, vectorStr, communityID string,
	userLatDeg, userLonDeg float64,
) ([]UnifiedSearchResult, error) {
	entityTable, err := s.quotedTableFor(cfg.entityType)
	if err != nil {
		return nil, err
	}
	joinTable, err := s.quotedTableFor(cfg.joinType)
	if err != nil {
		return nil, err
	}
	locTable, err := s.quotedTableFor("ripls.models.Location")
	if err != nil {
		return nil, err
	}
	// colName is derived from the embedder's vendor/model at runtime, so it is
	// validated here rather than trusted from EnsureEmbeddingColumn's earlier
	// check — the two used to be separated by the whole read path.
	embeddingCol, err := qualify("e", colName)
	if err != nil {
		return nil, err
	}
	joinCol, err := qualify("j", cfg.joinColumn)
	if err != nil {
		return nil, err
	}

	maxDistance := 1.0 - SemanticSearchMinSimilarity

	// Build SELECT columns
	selectCols := "e.binary_proto, l.binary_proto"
	if cfg.hasAvailability {
		selectCols += ", j.availability"
	}
	selectCols += fmt.Sprintf(", 1 - (%s <=> %s::vector) as similarity", embeddingCol, s.dbSpec.Placeholder(1))

	// Build WHERE clause
	where := fmt.Sprintf("j.community_id = %s AND %s IS NOT NULL AND %s <=> %s::vector <= %s",
		s.dbSpec.Placeholder(2), embeddingCol,
		embeddingCol, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(3))
	if cfg.extraWhereSQL != "" {
		// sql-fragment-allow: communitySearchConfig fields are package-level literals (see the type doc); no caller input reaches them
		where += " " + cfg.extraWhereSQL
	}

	// Build ORDER BY + LIMIT
	orderLimit := fmt.Sprintf("ORDER BY %s <=> %s::vector", embeddingCol, s.dbSpec.Placeholder(1))
	if cfg.limitSQL != "" {
		// sql-fragment-allow: communitySearchConfig fields are package-level literals (see the type doc); no caller input reaches them
		orderLimit += " " + cfg.limitSQL
	}

	query := fmt.Sprintf(`SELECT %s FROM %s e INNER JOIN %s j ON e.id = %s LEFT JOIN %s l ON e.location_id = l.id WHERE %s %s`,
		selectCols, entityTable, joinTable, joinCol, locTable, where, orderLimit)

	rows, err := s.db.QueryContext(ctx, query, vectorStr, communityID, maxDistance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []UnifiedSearchResult
	for rows.Next() {
		result, err := s.scanSearchRow(rows, cfg, userLatDeg, userLonDeg, true)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

// executeTextSearch runs a text-based ILIKE search using the given config.
func (s *ProtoSQLStorage) executeTextSearch(
	ctx context.Context,
	cfg communitySearchConfig,
	searchQuery, communityID string,
	userLatDeg, userLonDeg float64,
) ([]UnifiedSearchResult, error) {
	entityTable, err := s.quotedTableFor(cfg.entityType)
	if err != nil {
		return nil, err
	}
	joinTable, err := s.quotedTableFor(cfg.joinType)
	if err != nil {
		return nil, err
	}
	locTable, err := s.quotedTableFor("ripls.models.Location")
	if err != nil {
		return nil, err
	}
	joinCol, err := qualify("j", cfg.joinColumn)
	if err != nil {
		return nil, err
	}

	// Build SELECT columns
	selectCols := "e.binary_proto, l.binary_proto"
	if cfg.hasAvailability {
		selectCols += ", j.availability"
	}

	// Build WHERE clause
	where := fmt.Sprintf("j.community_id = %s", s.dbSpec.Placeholder(1))
	if cfg.extraWhereSQL != "" {
		// sql-fragment-allow: communitySearchConfig fields are package-level literals (see the type doc); no caller input reaches them
		where += " " + cfg.extraWhereSQL
	}

	args := []any{communityID}

	if searchQuery != "" {
		// Build LIKE conditions for each search column
		var likeClauses []string
		for _, col := range cfg.searchColumns {
			searchCol, err := qualify("e", col)
			if err != nil {
				return nil, err
			}
			args = append(args, likeContains(searchQuery))
			likeClauses = append(likeClauses, fmt.Sprintf("%s ILIKE %s %s", searchCol, s.dbSpec.Placeholder(len(args)), likeEscapeClause))
		}
		where += " AND (" + likeClauses[0]
		for _, clause := range likeClauses[1:] {
			where += " OR " + clause
		}
		where += ")"
	}

	// Build ORDER BY + LIMIT
	orderLimit := ""
	if cfg.orderBySQL != "" {
		// sql-fragment-allow: communitySearchConfig fields are package-level literals (see the type doc); no caller input reaches them
		orderLimit = cfg.orderBySQL
	}
	if cfg.limitSQL != "" {
		if orderLimit != "" {
			orderLimit += " "
		}
		// sql-fragment-allow: communitySearchConfig fields are package-level literals (see the type doc); no caller input reaches them
		orderLimit += cfg.limitSQL
	}

	query := fmt.Sprintf(`SELECT %s FROM %s e INNER JOIN %s j ON e.id = %s LEFT JOIN %s l ON e.location_id = l.id WHERE %s %s`,
		selectCols, entityTable, joinTable, joinCol, locTable, where, orderLimit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []UnifiedSearchResult
	for rows.Next() {
		result, err := s.scanSearchRow(rows, cfg, userLatDeg, userLonDeg, false)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

// scanSearchRow scans a single row from a search query into a UnifiedSearchResult.
func (s *ProtoSQLStorage) scanSearchRow(
	rows interface{ Scan(dest ...any) error },
	cfg communitySearchConfig,
	userLatDeg, userLonDeg float64,
	hasSimilarity bool,
) (UnifiedSearchResult, error) {
	var entityProto, locProto []byte
	var availability int32
	var similarity float64

	// Build scan targets based on config
	scanTargets := []any{&entityProto, &locProto}
	if cfg.hasAvailability {
		scanTargets = append(scanTargets, &availability)
	}
	if hasSimilarity {
		scanTargets = append(scanTargets, &similarity)
	} else {
		similarity = 1.0 // text search defaults to 1.0
	}

	if err := rows.Scan(scanTargets...); err != nil {
		return UnifiedSearchResult{}, err
	}

	// Unmarshal location
	var loc *models.Location
	var dist float64
	if len(locProto) > 0 {
		loc = &models.Location{}
		if err := proto.Unmarshal(locProto, loc); err != nil {
			return UnifiedSearchResult{}, err
		}
		if loc.Geolocation != nil {
			dist = HaversineDistance(userLatDeg, userLonDeg,
				loc.Geolocation.LatitudeDeg, loc.Geolocation.LongitudeDeg)
		}
	}

	result := UnifiedSearchResult{
		ItemType:           cfg.itemType,
		SemanticSimilarity: similarity,
		DistanceMeters:     dist,
		CompositeScore:     computeCompositeScore(similarity, dist),
		Location:           loc,
		Availability:       availability,
	}

	// Unmarshal entity and set the appropriate field
	switch cfg.itemType {
	case "gear":
		gear := &models.Gear{}
		if err := proto.Unmarshal(entityProto, gear); err != nil {
			return UnifiedSearchResult{}, err
		}
		result.Gear = gear
		result.ID = gear.Id
	case "request":
		req := &models.Request{}
		if err := proto.Unmarshal(entityProto, req); err != nil {
			return UnifiedSearchResult{}, err
		}
		result.Request = req
		result.ID = req.Id
	case "experience":
		exp := &models.Experience{}
		if err := proto.Unmarshal(entityProto, exp); err != nil {
			return UnifiedSearchResult{}, err
		}
		result.Experience = exp
		result.ID = exp.Id
	}

	return result, nil
}

// --- Per-type public methods ---.

// IsGearInCommunity checks if a gear item is actively shared with a specific
// community. Soft-deleted join rows do not count, mirroring the filtering the
// generic readers apply via buildDeletedFilter.
func (s *ProtoSQLStorage) IsGearInCommunity(ctx context.Context, gearID, communityID string) (bool, error) {
	query := `SELECT EXISTS(
		SELECT 1 FROM community_gear
		WHERE gear_id = $1 AND community_id = $2
		  AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec IS NULL)
	)`
	var exists bool
	err := s.db.QueryRowContext(ctx, query, gearID, communityID).Scan(&exists)
	return exists, err
}

// QueryGearByCommunitySearch searches for gear within a community.
// Tries semantic (vector similarity) search first if embedding providers are configured,
// falling back to text search (LIKE) on error or if not configured.
func (s *ProtoSQLStorage) QueryGearByCommunitySearch(
	ctx context.Context,
	communityID string,
	searchQuery string,
	userLatDeg, userLonDeg float64,
) ([]GearSearchResult, error) {
	cfg := gearSearchConfig()
	results, err := s.searchWithFallback(ctx, cfg, "gear_search", communityID, searchQuery, userLatDeg, userLonDeg)
	if err != nil {
		return nil, err
	}
	return unifiedToGearResults(results), nil
}

// QueryRequestByCommunitySearch searches for requests within a community.
// Tries semantic (vector similarity) search first if embedding providers are configured,
// falling back to text search (LIKE) on error or if not configured.
func (s *ProtoSQLStorage) QueryRequestByCommunitySearch(
	ctx context.Context,
	communityID string,
	searchQuery string,
	userLatDeg, userLonDeg float64,
) ([]RequestSearchResult, error) {
	cfg := requestSearchConfig()
	results, err := s.searchWithFallback(ctx, cfg, "request_search", communityID, searchQuery, userLatDeg, userLonDeg)
	if err != nil {
		return nil, err
	}
	return unifiedToRequestResults(results), nil
}

// searchWithFallback tries semantic search first, falling back to text search.
func (s *ProtoSQLStorage) searchWithFallback(
	ctx context.Context,
	cfg communitySearchConfig,
	operation, communityID, searchQuery string,
	userLatDeg, userLonDeg float64,
) ([]UnifiedSearchResult, error) {
	logger := logging.LoggerWithContext(ctx)

	if searchQuery != "" {
		results, err := s.semanticSearchSingle(ctx, cfg, searchQuery, communityID, userLatDeg, userLonDeg)
		if err == nil {
			logger.DebugContext(ctx, "semantic search completed",
				"operation", operation,
				"query", searchQuery,
				"community_id", communityID,
				"results_count", len(results),
			)
			return results, nil
		}
		if !errors.Is(err, ErrEmbeddingNotConfigured) {
			logger.WarnContext(ctx, "semantic search failed, falling back to text search",
				"operation", operation,
				"query", searchQuery,
				"error", err,
			)
		}
	}

	results, err := s.executeTextSearch(ctx, cfg, searchQuery, communityID, userLatDeg, userLonDeg)
	if err != nil {
		return nil, err
	}
	if searchQuery != "" {
		logger.DebugContext(ctx, "text search completed",
			"operation", operation,
			"query", searchQuery,
			"community_id", communityID,
			"results_count", len(results),
		)
	}

	// Sort text results by distance for per-type searches (unified sorts by composite score)
	sort.Slice(results, func(i, j int) bool {
		return results[i].DistanceMeters < results[j].DistanceMeters
	})
	return results, nil
}

// semanticSearchSingle generates an embedding and executes a semantic search for a single config.
func (s *ProtoSQLStorage) semanticSearchSingle(
	ctx context.Context,
	cfg communitySearchConfig,
	searchQuery, communityID string,
	userLatDeg, userLonDeg float64,
) ([]UnifiedSearchResult, error) {
	embedder := s.GetEmbedder()
	if embedder == nil {
		return nil, ErrEmbeddingNotConfigured
	}

	entityTable := s.allowedTypes[cfg.entityType]
	if len(s.embeddingConfigs[entityTable]) == 0 {
		return nil, ErrEmbeddingNotConfigured
	}

	info := embedder.Info()
	queryEmbedding, err := embedder.Generate(ctx, searchQuery)
	if err != nil {
		return nil, fmt.Errorf("generate query embedding: %w", err)
	}

	colName := EmbeddingColumnName(info)
	vectorStr := formatVector(queryEmbedding)

	return s.executeSemanticSearch(ctx, cfg, colName, vectorStr, communityID, userLatDeg, userLonDeg)
}

// unifiedToGearResults converts UnifiedSearchResults to GearSearchResults.
func unifiedToGearResults(results []UnifiedSearchResult) []GearSearchResult {
	out := make([]GearSearchResult, len(results))
	for i, r := range results {
		out[i] = GearSearchResult{
			Gear:           r.Gear,
			Location:       r.Location,
			DistanceMeters: r.DistanceMeters,
			Similarity:     r.SemanticSimilarity,
		}
	}
	return out
}

// unifiedToRequestResults converts UnifiedSearchResults to RequestSearchResults.
func unifiedToRequestResults(results []UnifiedSearchResult) []RequestSearchResult {
	out := make([]RequestSearchResult, len(results))
	for i, r := range results {
		out[i] = RequestSearchResult{
			Request:        r.Request,
			Location:       r.Location,
			DistanceMeters: r.DistanceMeters,
			Similarity:     r.SemanticSimilarity,
		}
	}
	return out
}

// --- Unified search public methods ---.

// QueryCommunitySearch performs a unified search across gear, requests, and experiences.
// When includeCompleted is true, completed/fulfilled/cancelled items are included.
func (s *ProtoSQLStorage) QueryCommunitySearch(
	ctx context.Context,
	communityID string,
	searchQuery string,
	userLatDeg, userLonDeg float64,
	includeCompleted bool,
) ([]UnifiedSearchResult, error) {
	logger := logging.LoggerWithContext(ctx)

	if searchQuery != "" {
		results, err := s.semanticUnifiedSearch(ctx, communityID, searchQuery, userLatDeg, userLonDeg, includeCompleted)
		if err == nil && len(results) > 0 {
			logger.DebugContext(ctx, "unified semantic search completed",
				"operation", "unified_search",
				"query", searchQuery,
				"community_id", communityID,
				"results_count", len(results),
			)
			return results, nil
		}
		if err != nil && !errors.Is(err, ErrEmbeddingNotConfigured) {
			logger.WarnContext(ctx, "semantic search failed, falling back to text",
				"operation", "unified_search",
				"error", err,
			)
		}
	}

	results, err := s.textUnifiedSearch(ctx, communityID, searchQuery, userLatDeg, userLonDeg, includeCompleted)
	if err == nil {
		logger.DebugContext(ctx, "unified text search completed",
			"operation", "unified_search",
			"query", searchQuery,
			"community_id", communityID,
			"results_count", len(results),
		)
	}
	return results, err
}

// semanticUnifiedSearch generates an embedding and searches across all entity types.
func (s *ProtoSQLStorage) semanticUnifiedSearch(
	ctx context.Context,
	communityID string,
	searchQuery string,
	userLatDeg, userLonDeg float64,
	includeCompleted bool,
) ([]UnifiedSearchResult, error) {
	embedder := s.GetEmbedder()
	if embedder == nil {
		return nil, ErrEmbeddingNotConfigured
	}
	info := embedder.Info()

	queryEmbedding, err := embedder.Generate(ctx, searchQuery)
	if err != nil {
		return nil, fmt.Errorf("generate query embedding: %w", err)
	}

	colName := EmbeddingColumnName(info)
	vectorStr := formatVector(queryEmbedding)
	logger := logging.LoggerWithContext(ctx)

	configs := []communitySearchConfig{
		unifiedGearConfig(),
		unifiedRequestConfig(includeCompleted),
		unifiedExperienceConfig(includeCompleted),
	}

	var allResults []UnifiedSearchResult
	for _, cfg := range configs {
		entityTable := s.allowedTypes[cfg.entityType]
		if len(s.embeddingConfigs[entityTable]) == 0 {
			continue
		}
		results, err := s.executeSemanticSearch(ctx, cfg, colName, vectorStr, communityID, userLatDeg, userLonDeg)
		if err != nil {
			logger.WarnContext(ctx, "semantic search error",
				"operation", "unified_search",
				"table", cfg.itemType,
				"error", err,
			)
		} else {
			allResults = append(allResults, results...)
		}
	}

	if len(allResults) == 0 {
		return nil, ErrEmbeddingNotConfigured
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].CompositeScore > allResults[j].CompositeScore
	})
	return allResults, nil
}

// textUnifiedSearch searches across all entity types using text matching.
func (s *ProtoSQLStorage) textUnifiedSearch(
	ctx context.Context,
	communityID string,
	searchQuery string,
	userLatDeg, userLonDeg float64,
	includeCompleted bool,
) ([]UnifiedSearchResult, error) {
	logger := logging.LoggerWithContext(ctx)

	configs := []communitySearchConfig{
		unifiedGearConfig(),
		unifiedRequestConfig(includeCompleted),
		unifiedExperienceConfig(includeCompleted),
	}

	var allResults []UnifiedSearchResult
	for _, cfg := range configs {
		results, err := s.executeTextSearch(ctx, cfg, searchQuery, communityID, userLatDeg, userLonDeg)
		if err != nil {
			logger.WarnContext(ctx, "text search error",
				"operation", "unified_search",
				"table", cfg.itemType,
				"error", err,
			)
		} else {
			allResults = append(allResults, results...)
		}
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].CompositeScore > allResults[j].CompositeScore
	})
	return allResults, nil
}

// SearchCommunityUsersByName returns community members whose display names match the given
// query string using case-insensitive ILIKE matching.
// Results are limited to limit rows (capped at 50). A zero or negative limit defaults to 10.
func (s *ProtoSQLStorage) SearchCommunityUsersByName(ctx context.Context, communityID, query string, limit int) ([]*models.User, error) {
	userTable, err := s.quotedTableFor("ripls.models.User")
	if err != nil {
		return nil, err
	}
	memberTable, err := s.quotedTableFor("ripls.models.CommunityUser")
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	sqlQuery := fmt.Sprintf(
		`SELECT u.binary_proto FROM %s u
		 INNER JOIN %s cu ON u.id = cu.user_id
		 WHERE cu.community_id = %s
		   AND u.name ILIKE %s %s
		 LIMIT %s`,
		userTable, memberTable,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
		likeEscapeClause,
		s.dbSpec.Placeholder(3),
	)

	rows, err := s.db.QueryContext(ctx, sqlQuery, communityID, likeContains(query), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search community users by name: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var protoData []byte
		if err := rows.Scan(&protoData); err != nil {
			return nil, fmt.Errorf("failed to scan user row: %w", err)
		}
		user := &models.User{}
		if err := proto.Unmarshal(protoData, user); err != nil {
			return nil, fmt.Errorf("failed to unmarshal user proto: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// SearchProvisionalUsersByName returns UNCLAIMED provisional users in a
// community whose display names match the given query string using
// case-insensitive ILIKE matching. Claimed placeholders are excluded (#2699):
// promote-on-verify keeps the claimed row for history, but its person is a
// real member now — surfacing the stale placeholder in the completion
// quick-add would double-attribute them. Claimed rows remain visible via
// ListProvisionalUsers (the manage-members surface).
// Results are limited to limit rows (capped at 50). A zero or negative limit defaults to 10.
func (s *ProtoSQLStorage) SearchProvisionalUsersByName(ctx context.Context, communityID, query string, limit int) ([]*models.ProvisionalUser, error) {
	provisionalTable, err := s.quotedTableFor("ripls.models.ProvisionalUser")
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	sqlQuery := fmt.Sprintf(
		`SELECT binary_proto FROM %s
		 WHERE community_id = %s
		   AND name ILIKE %s %s
		   AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec IS NULL)
		   AND (claimed_by_user_id IS NULL OR claimed_by_user_id = '')
		 LIMIT %s`,
		provisionalTable,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
		likeEscapeClause,
		s.dbSpec.Placeholder(3),
	)

	rows, err := s.db.QueryContext(ctx, sqlQuery, communityID, likeContains(query), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search provisional users by name: %w", err)
	}
	defer rows.Close()

	var provs []*models.ProvisionalUser
	for rows.Next() {
		var protoData []byte
		if err := rows.Scan(&protoData); err != nil {
			return nil, fmt.Errorf("failed to scan provisional user row: %w", err)
		}
		prov := &models.ProvisionalUser{}
		if err := proto.Unmarshal(protoData, prov); err != nil {
			return nil, fmt.Errorf("failed to unmarshal provisional user proto: %w", err)
		}
		provs = append(provs, prov)
	}
	return provs, rows.Err()
}
