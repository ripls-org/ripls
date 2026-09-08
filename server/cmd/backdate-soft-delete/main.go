// backdate-soft-delete rewrites a soft-deleted community's
// deleted_at to a specified age in days, updating both the
// proto-serialized `binary_proto` blob AND the flat
// `deleted_deleted_at_unix_sec` column. SQL UPDATE alone only
// touches the flat column, leaving the proto stale — which the
// purge job's per-row re-check correctly rejects (Safeguard #5).
//
// One-shot dev tool — not for production use.
//
// Usage:
//
//	go run ./server/cmd/backdate-soft-delete \
//	  --community-id=<uuid> \
//	  --age-days=31 \
//	  --db=postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func main() {
	dbURL := flag.String("db", "postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable", "PostgreSQL connection string")
	communityID := flag.String("community-id", "", "ID of the soft-deleted community to backdate (required)")
	ageDays := flag.Int("age-days", 31, "Set deleted_at to this many days in the past")
	flag.Parse()

	if *communityID == "" {
		log.Fatal("--community-id is required")
	}

	ctx := context.Background()

	store, err := storage.InitializePostgreSQLDatabase(ctx, *dbURL, storage.DefaultStorageTypes())
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}
	defer store.Close()

	community := &models.Community{}
	if err := store.GetByID(ctx, *communityID, community, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		log.Fatalf("GetByID(%s): %v", *communityID, err)
	}
	if community.GetDeleted() == nil || community.GetDeleted().GetDeletedAtUnixSec() == 0 {
		log.Fatalf("community %s is not soft-deleted; nothing to backdate", *communityID)
	}

	newAt := time.Now().Unix() - int64(*ageDays)*86400
	community.Deleted.DeletedAtUnixSec = newAt
	if err := store.Update(ctx, community); err != nil {
		log.Fatalf("Update: %v", err)
	}

	log.Printf("backdated community %s deleted_at to %d (%d days ago)\n",
		community.Id, newAt, *ageDays)
}
