# Storage

The `storage` package is the server's data-access layer: a proto-to-SQL mapping that stores protobuf messages in PostgreSQL tables with a `binary_proto` column for the full message and flattened scalar columns for indexed fields. It also provides bucket storage (object storage for media), geospatial queries, semantic search via pgvector, and test infrastructure.

## Architecture

```
storage/
├── protosql.go          # Core CRUD: Insert, GetByID, ListAll, QueryByField, Update, Delete
├── protosql_batch.go    # Batch operations: GetByIDs, QueryByFieldIn, InsertBatch
├── protosql_generic.go  # Generic config-driven search helpers
├── protosql_schema.go   # Schema introspection and column-name sanitization
├── protosql_search.go   # Full-text and ILIKE search
├── protosql_spatial.go  # Geospatial queries (PostGIS)
├── protosql_types.go    # Type marshalling for scalar proto fields
├── instrumented_db.go   # InstrumentedDB: wraps *sql.DB to count queries per request
├── query_assert.go      # AssertMaxQueries: test helper to catch N+1 regressions
├── postgresql.go        # Database connection setup and pool configuration
├── migrations.go        # Schema migrations run at startup
├── bucket.go            # BucketStorage: object storage interface and GCS implementation
├── embedding.go         # pgvector embedding storage and similarity search
├── feed.go              # FeedStorage: activity feed queries
├── story.go             # StoryStorage: story queries and deduplication
├── geospatial.go        # Geospatial index helpers
├── cascade_delete.go    # Cascading soft-delete across related entities
├── media.go             # Media metadata helpers
├── watched.go           # WatchStorage: community watch/unwatch
├── semaphore.go         # DB-level advisory lock semaphore
└── testing.go           # SetupTestStorage: per-package test DB via testcontainers
```

## Key concepts

- **ProtoSQLStorage** — the primary struct; all services hold a `*ProtoSQLStorage`. Messages are registered at startup via `allowedTypes`.
- **InstrumentedDB / SQLDB** — wraps `*sql.DB` to count per-request queries; used by `AssertMaxQueries` to prevent N+1 regressions in tests.
- **Column naming** — field names longer than 63 bytes are truncated with a SHA-256 hash suffix by `sanitizeColumnName`.
- **SQL safety** — this package is the only one in the server that issues SQL, and inside it exactly two things may enter a query string: values bound with `dbSpec.Placeholder(n)`, and identifiers quoted with `quoteIdent` (`sqlident.go`). `npm run lint:go:sql` (`server/cmd/check-sql`) fails the build on anything else; a genuinely fixed fragment needs a `// sql-fragment-allow: <reason>` marker. Full rules in [`docs/server/conventions.md`](../../docs/server/conventions.md) § SQL Safety.

## When to add code here vs. elsewhere

SQL and storage plumbing belongs here. Derived calculations over stored data (impact metrics, community regions) belong in the packages that own those concepts.
