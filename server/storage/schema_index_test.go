package storage

import (
	"context"
	"regexp"
	"testing"
)

// indexNameRe reads the index name out of a CREATE INDEX statement. The ON
// clause can be on the next line, so it is not part of the match.
var indexNameRe = regexp.MustCompile(`CREATE (?:UNIQUE )?INDEX IF NOT EXISTS (\w+)`)

// Every statement in schemaIndexStatements runs with IF NOT EXISTS, and a
// failure is logged and skipped so a bad one cannot stop startup. That makes a
// typo invisible: idx_story_community named "story" where the table is "Story"
// and was missing from both environments for five months (#3088). Asserting the
// indexes exist after initialization turns that into a build failure.
//
// What this cannot catch: IF NOT EXISTS never alters an index that already
// exists, so editing the columns of an index deployed earlier leaves every
// existing database on the old definition while a fresh one here gets the new
// one. Changing an index's shape means giving it a new name.
func TestSchemaIndexesAreCreated(t *testing.T) {
	ctx := context.Background()
	sqlStorage, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	statements := schemaIndexStatements()
	if len(statements) == 0 {
		t.Fatal("schemaIndexStatements is empty")
	}

	// A repeated name is the quiet version of the same bug: the second
	// statement is a no-op under IF NOT EXISTS, so its columns never exist
	// while this test still passes on the first one's index.
	seen := map[string]bool{}

	for _, statement := range statements {
		match := indexNameRe.FindStringSubmatch(statement)
		if match == nil {
			t.Errorf("no index name in: %s", statement)
			continue
		}
		name := match[1]
		if seen[name] {
			t.Errorf("index name %s is used twice; the second statement is silently a no-op", name)
		}
		seen[name] = true

		var exists bool
		if err := sqlStorage.db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1)`,
			name).Scan(&exists); err != nil {
			t.Fatalf("checking for index %s: %v", name, err)
		}
		if !exists {
			t.Errorf("index %s does not exist after initialization — its statement names a table or column that does not: %s",
				name, statement)
		}
	}
}
