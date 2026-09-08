---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Natural-language discovery of gear/requests/experiences via a locally-run fine-tuned MiniLM embedder (ONNX) with pgvector HNSW columns in Postgres — storage-layer encapsulation, similarity thresholds, and graceful fallback to text LIKE search.
  globs: [server/ai/embedding/**, server/storage/embedding.go, server/storage/protosql_search.go, server/services/search/**]
  triggers: [semantic-search, embedding, pgvector, onnx, minilm, vector, hnsw, similarity, nearest-neighbor]
  lens: [server, domain]
  domain: server
freshness:
  verified_commit: "a3587b44b"
  verified_on: "2026-08-29"
---
# Semantic Search Architecture

## Overview

Semantic search enables natural language discovery of gear, requests, and experiences by understanding meaning rather than just matching keywords. A search for "power tools" finds items like "cordless drill" even without exact word matches. The implementation uses a locally-run fine-tuned sentence transformer model with vector embeddings stored in PostgreSQL using pgvector. The storage layer transparently handles embedding generation, search execution, and fallback to text search.

## Design Principles

**Storage-Layer Encapsulation**: All embedding logic lives in the storage layer. Services call query methods like `QueryGearByCommunitySearch()` without knowledge of whether semantic or text search is used. This keeps services simple and centralizes embedding management.

**Local Model Inference**: Embeddings are generated using a fine-tuned MiniLM model running locally via ONNX Runtime. This eliminates external API dependencies, reduces latency, ensures privacy, and provides consistent behavior across environments.

**Graceful Degradation**: If semantic search fails or isn't configured, queries automatically fall back to text-based LIKE search. This ensures the system remains functional during model loading issues or before embeddings are populated.

**Similarity Threshold**: Results below a minimum similarity threshold (0.5, `SemanticSearchMinSimilarity`) are filtered out, ensuring only semantically relevant items are returned.

## Embedding Model

The system uses a fine-tuned sentence transformer model optimized for the Ripls domain:

- **Model**: `ripls-minilm-v1` (fine-tuned from `sentence-transformers/all-MiniLM-L6-v2`)
- **Dimensions**: 384
- **Format**: ONNX for cross-platform inference
- **Location**: `/app/models/ripls_embedding.onnx` (Docker) or configured via `-embedding-model-path`

The model was trained on Ripls-specific data to better understand sharing/borrowing terminology and item descriptions. Key training characteristics:

- Average similarity for matching pairs: ~0.91
- Average similarity for non-matching pairs: ~0.19
- Clear separation between relevant and irrelevant results

### Embedder

The embedding system is encapsulated in the `embedding.Embedder` struct, an
ONNX-backed embedder constructed via `embedding.New(modelPath, vocabPath)` that
loads the model and vocabulary at startup:

```go
type Embedder struct { /* ... */ }

// New loads the ONNX model + tokenizer vocab and returns an *Embedder.
func New(modelPath, vocabPath string) (*Embedder, error)

func (e *Embedder) Info() *Info
func (e *Embedder) Generate(ctx context.Context, text string) ([]float32, error)
func (e *Embedder) Close() error

type Info struct {
    Vendor     string // e.g., "local"
    Model      string // e.g., "ripls-minilm-v1"
    Dimensions int    // e.g., 384
}
```

`DefaultInfo()` returns the `*Info` for the local fine-tuned model.

## Database Schema

The pgvector extension adds vector column support to PostgreSQL. A dedicated embedding column is created on tables with embedding configuration:

```
gear.ripls_minilm_v1    vector(384)
request.ripls_minilm_v1 vector(384)
```

Each column has an HNSW (Hierarchical Navigable Small World) index for fast approximate nearest neighbor search:

```sql
CREATE INDEX gear_ripls_minilm_v1_hnsw_idx
ON gear USING hnsw (ripls_minilm_v1 vector_cosine_ops)
WITH (m = 16, ef_construction = 64)
```

Column and index creation is idempotent—`EnsureEmbeddingColumn()` safely handles repeated calls.

## Embedding Configuration

Tables that support semantic search are configured in `DefaultStorageTypes()` with one or more `EmbeddingFieldConfig`s (the `Embeddings` slice), each specifying which fields to concatenate for embedding text:

```go
{
    MessageType: &models.Gear{},
    TableName:   "gear",
    Embeddings:  []*EmbeddingFieldConfig{{Fields: []string{"name", "description"}}},
},
{
    MessageType: &models.Request{},
    TableName:   "request",
    Embeddings:  []*EmbeddingFieldConfig{{Fields: []string{"title", "description"}}},
},
{
    MessageType: &models.Experience{},
    TableName:   "experience",
    Embeddings: []*EmbeddingFieldConfig{
        {Fields: []string{"name", "description"}},                    // default: search
        {Fields: []string{"name"}, Variant: "name", SkipIndex: true}, // name-only: activity clustering
    },
},
```

### Multiple embedding variants per table

A table may declare **several** embeddings of the same model, distinguished by
`Variant`. The empty variant is the default column (`vendor_model`, used by
semantic search and backward-compatible); a named variant appends `_<variant>`
(e.g. `local_ripls_minilm_v1_name`). Set `SkipIndex: true` for a variant that is
only read by id (no ANN search) so no HNSW index is maintained on writes.

The `experience` table uses this: the default `name+description` embedding powers
search, while a `name`-only variant powers open-day **activity clustering**
(#2674) — reading the whole-experience vector clustered activities poorly, so a
name-only vector is precomputed and read by `GroupIDsByStoredEmbedding`. Column
management, per-variant generation on insert/update (one completion signal per
row), and startup backfill all iterate every configured variant.

When a configured type is inserted or updated, the storage layer extracts text from the specified fields and asynchronously generates embeddings.

## Embedding Generation

Embeddings are generated at two points:

**On Insert/Update**: After a record is saved, `generateEmbedding()` runs asynchronously in a goroutine. It extracts text from configured fields using proto reflection, generates an embedding from the local model, and stores it via `UpdateEmbedding()`. Failures are logged but don't block the operation. When the caller is inside a `WithTx` transaction (the common case for types with array-column registrations such as Gear, Request, and Experience), the goroutine is scheduled *after* the transaction commits via an after-commit hook, so the row is always visible on the connection pool before the `UPDATE` runs.

**Startup Backfill**: `BackfillEmbeddings()` runs in the background after server startup, processing records with NULL embedding columns. It queries in batches of 100, generates embeddings with rate limiting (50ms between items), and is safe to interrupt—only NULL columns are processed. Progress is logged at start and completion.

## Search Execution

Search methods like `QueryGearByCommunitySearch()` implement a two-phase approach:

1. **Semantic Search**: If a query is provided and embeddings are configured, generate an embedding for the search query, then execute a vector similarity search using cosine distance with threshold filtering:

```sql
SELECT binary_proto, 1 - (embedding <=> $query::vector) as similarity
FROM gear
WHERE community_id = $community
  AND embedding IS NOT NULL
  AND embedding <=> $query::vector <= 0.5  -- threshold: 1 - 0.5 = 0.5
ORDER BY embedding <=> $query::vector
LIMIT 100
```

2. **Text Fallback**: If semantic search fails (model error, not configured, or empty results), fall back to case-insensitive `ILIKE` matching on the configured text fields:

```sql
SELECT binary_proto FROM "gear" e
INNER JOIN "community_gear" j ON e.id = j."gear_id"
WHERE j.community_id = $1
AND (e."name" ILIKE $2 ESCAPE '\' OR e."description" ILIKE $3 ESCAPE '\')
```

The pattern is bound, and the search term is run through `escapeLikePattern`
before it is wrapped in `%…%`, so `%`, `_` and `\` typed by a member match
themselves instead of acting as wildcards (#2795). Before that, a search for
`%` matched every row in the community.

Use `ILIKE`, never `LOWER(col) LIKE LOWER($1)` — the latter prevents index use
and is forbidden by [`conventions.md`](conventions.md) § SQL Efficiency rule 7.

### Similarity Threshold

The `SemanticSearchMinSimilarity` constant (0.5) filters out results with low semantic relevance. This threshold was chosen based on production testing:

- Good matches: ~0.55-0.65+ similarity
- Moderately related: ~0.4-0.5 similarity
- Unrelated items (noise floor): ~0.3-0.4 similarity

A threshold of 0.5 filters out the noise floor while keeping relevant results.

### Search Results

Search results include similarity scores for transparency:

```go
type GearSearchResult struct {
    Gear           *models.Gear
    Location       *models.Location
    DistanceMeters float64
    Similarity     float64 // Cosine similarity (0-1), 0 for text search
}
```

## Unified Search

The `QueryCommunitySearch()` method searches across all item types (gear, requests, experiences) simultaneously and returns results ranked by a composite score combining semantic similarity and geographic distance:

```go
type UnifiedSearchResult struct {
    ItemType           string  // "gear", "request", or "experience"
    ID                 string
    SemanticSimilarity float64
    DistanceMeters     float64
    CompositeScore     float64 // Combined ranking score
    // Type-specific data...
}
```

## RPC Integration

The search service is a thin wrapper around storage methods:

```go
func (s *Service) Search(ctx context.Context, req *connect.Request[api.SearchRequest]) (*connect.Response[api.SearchResponse], error) {
    results, err := s.storage.QueryCommunitySearch(
        ctx, communityIDs, req.Msg.Query, ...)
    // Convert to API response...
}
```

The `SearchService` exposes the unified `Search` RPC, `UniversalSearch`, and
`GetSearchSuggestions` (see `proto/ripls/api/search_service.proto`). `Search`
resolves to `storage.QueryCommunitySearch()` for the semantic path and the
per-type `searchGearExact` / `searchRequestsExact` / `searchExperiencesExact` /
`searchUsers` helpers for the text fallback. It requires authentication
(`auth.RequireAuth`); results include distance calculations when user
coordinates are provided. The storage layer still exposes per-type
`QueryGearByCommunitySearch` / `QueryRequestByCommunitySearch` methods, but the
RPC path uses the unified query.

`UniversalSearch` (`server/services/search/universal.go`) reuses the same
`semanticSearch()` helper — and therefore the same `storage.QueryCommunitySearch()`
semantic/text-fallback path described above — but carries no community scope on
the request; the server derives it from the caller's own active memberships and
fans the query out across all of them. Results are deduplicated, then split into
three fixed groups (library, plans, people) by item type rather than returned as
one ranked list.

## Startup Sequence

1. Initialize PostgreSQL with pgvector extension enabled
2. Load ONNX embedding model and vocabulary
3. Create embedder instance with model paths
4. Call `SetEmbedder()` to configure storage and ensure embedding columns exist
5. Start background `BackfillEmbeddings()` goroutine
6. Register search service with storage reference

## Docker Deployment

The Docker image bundles the ONNX model and ONNX Runtime:

```dockerfile
# Runtime stage uses Debian for glibc compatibility
FROM debian:bookworm-slim

# Install ONNX Runtime (version pinned via ARG ONNXRUNTIME_VERSION, currently 1.29.0)
RUN wget "https://github.com/microsoft/onnxruntime/releases/..." && ...

# Copy model files
COPY model_tuning/ripls_embedding.onnx /app/models/
COPY model_tuning/ripls_embedding_tokenizer/vocab.txt /app/models/

# Start with embedding flags
CMD ./server -embedding-model-path=/app/models/ripls_embedding.onnx \
             -embedding-vocab-path=/app/models/vocab.txt ...
```

## Key Files

- `server/ai/embedding/`: Standalone embedding library
  - `embedding.go`: `Embedder` struct (ONNX Runtime-based), `Info`, `New()`, `Generate()`
  - `tokenizer/tokenizer.go`: WordPiece tokenizer for text preprocessing
- `server/storage/embedding.go`: Column management, embedder configuration, backfill
- `server/storage/protosql_types.go`: `EmbeddingFieldConfig`, `SemanticSearchMinSimilarity`, `GearSearchResult` / `UnifiedSearchResult` result structs
- `server/storage/protosql_search.go`: Search methods with semantic/text fallback
- `server/services/search/service.go`: `Search` RPC handler
- `server/services/search/universal.go`: `UniversalSearch` RPC handler (grouped, membership-scoped)
- `server/main.go`: Embedder initialization and wiring
- `model_tuning/`: Model training scripts and artifacts
  - `ripls_embedding.onnx`: Fine-tuned ONNX model (Git LFS)
  - `ripls_embedding_tokenizer/vocab.txt`: Tokenizer vocabulary

## Model Training

The embedding model can be retrained using the scripts in `model_tuning/`:

1. Generate training data from production database
2. Fine-tune the base MiniLM model on Ripls-specific pairs
3. Export to ONNX format
4. Update the model files and deploy

See `model_tuning/README.md` for detailed training instructions.
