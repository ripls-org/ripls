// Package storage is the server's data-access layer. It provides a
// proto-to-SQL mapping that persists protobuf messages in PostgreSQL with a
// binary_proto column and flattened scalar columns for indexed fields, along
// with object storage for media, geospatial queries, and semantic search via
// pgvector.
package storage
