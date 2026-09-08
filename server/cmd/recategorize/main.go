// recategorize clears and re-derives the Category field on every
// non-deleted Experience and Request row in the local PostgreSQL
// database using the keyword categorizer in `server/category`. Goes
// through the storage layer so the change writes both the flat
// `category` column AND the proto-serialized `binary_proto` blob
// (a plain SQL UPDATE only touches the flat column, leaving the
// proto stale — and the server reads from the proto).
//
// Why no recategorizeGear? Gear's AI pipeline already populates
// `Gear.Category` at save time (see
// `services/gear/gen_ai_streaming.go`) — it doesn't carry the
// `TODO(#2013): Add category detection to AI provider` stub that
// `services/experience/gen_ai.go` and `services/request/gen.go`
// have. Gear rows therefore already have their categories; only
// experience and request rows need the keyword fallback. If a
// gear-side backfill ever becomes necessary, mirror the helpers
// below.
//
// One-shot dev tool.
//
// TODO(#2013): retire this command once the AI category-detection
// pipeline lands. At that point the AI populates Category at save
// time on the experience and request paths too, and re-deriving
// from a keyword dictionary is no longer useful.
//
// Usage:
//
//	go run ./server/cmd/recategorize \
//	  --db=postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable
//
// By default the tool re-categorizes every row regardless of
// current value (so old / hand-typed categories get replaced with
// keyword-matched ones). Pass --only-empty to skip rows that
// already carry a non-empty category.
package main

import (
	"context"
	"flag"
	"log"

	"go.ripls.org/ripls/server/category"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func main() {
	dbURL := flag.String("db",
		"postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable",
		"PostgreSQL connection string")
	onlyEmpty := flag.Bool("only-empty", false,
		"Skip rows that already carry a non-empty Category")
	flag.Parse()

	ctx := context.Background()

	store, err := storage.InitializePostgreSQLDatabase(ctx, *dbURL, storage.DefaultStorageTypes())
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}
	defer store.Close()

	expStats := recategorizeExperiences(ctx, store, *onlyEmpty)
	log.Printf("experiences: scanned=%d updated=%d cleared=%d no_match=%d skipped_non_empty=%d",
		expStats.scanned, expStats.updated, expStats.cleared,
		expStats.noMatch, expStats.skippedNonEmpty)

	reqStats := recategorizeRequests(ctx, store, *onlyEmpty)
	log.Printf("requests: scanned=%d updated=%d cleared=%d no_match=%d skipped_non_empty=%d",
		reqStats.scanned, reqStats.updated, reqStats.cleared,
		reqStats.noMatch, reqStats.skippedNonEmpty)
}

// stats accumulates counters for the run.
type stats struct {
	scanned         int
	updated         int
	cleared         int
	noMatch         int
	skippedNonEmpty int
}

func recategorizeExperiences(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	onlyEmpty bool,
) stats {
	rows, err := store.ListAll(ctx, &models.Experience{})
	if err != nil {
		log.Fatalf("ListAll(Experience): %v", err)
	}
	var s stats
	for _, msg := range rows {
		e, ok := msg.(*models.Experience)
		if !ok || e == nil || e.Deleted != nil {
			continue
		}
		s.scanned++
		if onlyEmpty && e.Category != "" {
			s.skippedNonEmpty++
			continue
		}
		newCat := category.Categorize(e.Name, e.Description)
		if newCat == e.Category {
			if newCat == "" {
				s.noMatch++
			}
			continue
		}
		e.Category = newCat
		if err := store.Update(ctx, e); err != nil {
			log.Printf("update experience %s: %v", e.Id, err)
			continue
		}
		if newCat == "" {
			s.cleared++
		} else {
			s.updated++
		}
	}
	return s
}

func recategorizeRequests(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	onlyEmpty bool,
) stats {
	rows, err := store.ListAll(ctx, &models.Request{})
	if err != nil {
		log.Fatalf("ListAll(Request): %v", err)
	}
	var s stats
	for _, msg := range rows {
		r, ok := msg.(*models.Request)
		if !ok || r == nil || r.Deleted != nil {
			continue
		}
		s.scanned++
		if onlyEmpty && r.Category != "" {
			s.skippedNonEmpty++
			continue
		}
		newCat := category.Categorize(r.Title, r.Description)
		if newCat == r.Category {
			if newCat == "" {
				s.noMatch++
			}
			continue
		}
		r.Category = newCat
		if err := store.Update(ctx, r); err != nil {
			log.Printf("update request %s: %v", r.Id, err)
			continue
		}
		if newCat == "" {
			s.cleared++
		} else {
			s.updated++
		}
	}
	return s
}
