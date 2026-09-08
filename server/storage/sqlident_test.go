package storage

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lib/pq"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestQuoteIdent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"simple", "name", `"name"`, false},
		{"snake case", "created_at_unix_sec", `"created_at_unix_sec"`, false},
		{"leading underscore", "_internal", `"_internal"`, false},
		{"digits after letter", "col2", `"col2"`, false},
		{"mixed case", "FeedItemView", `"FeedItemView"`, false},

		{"empty", "", "", true},
		{"leading digit", "2col", "", true},
		{"embedded quote", `id" ; DROP TABLE "user`, "", true},
		{"embedded backslash", `id\name`, "", true},
		{"paren break-out", "id) OR 1=1 --", "", true},
		{"whitespace", "user id", "", true},
		{"semicolon", "id;", "", true},
		{"dot qualified", "e.name", "", true},
		{"unicode", "naïve", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := quoteIdent(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("quoteIdent(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("quoteIdent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestQuoteIdentDiffersFromGoQuoting pins the reason quoteIdent exists rather
// than %q: Go escapes an embedded double quote with a backslash, PostgreSQL
// doubles it. The two agree only on names that need no escaping — which is why
// %q looked correct in this codebase for as long as validateIdentifier ran
// first, and would have stopped being correct the moment it didn't.
func TestQuoteIdentDiffersFromGoQuoting(t *testing.T) {
	const hostile = `a"b`

	goQuoted := fmt.Sprintf("%q", hostile)
	pgQuoted := pq.QuoteIdentifier(hostile)

	if goQuoted == pgQuoted {
		t.Fatalf("expected %%q and pq.QuoteIdentifier to disagree on %q, both produced %s", hostile, goQuoted)
	}
	if goQuoted != `"a\"b"` {
		t.Errorf("Go quoting = %s, want %s", goQuoted, `"a\"b"`)
	}
	if pgQuoted != `"a""b"` {
		t.Errorf("PostgreSQL quoting = %s, want %s", pgQuoted, `"a""b"`)
	}

	// And quoteIdent refuses it outright, which is the actual defence.
	if _, err := quoteIdent(hostile); err == nil {
		t.Errorf("quoteIdent(%q) = nil error, want rejection", hostile)
	}
}

func TestQuoteIdents(t *testing.T) {
	got, err := quoteIdents([]string{"id", "name", "binary_proto"})
	if err != nil {
		t.Fatalf("quoteIdents: %v", err)
	}
	want := `"id", "name", "binary_proto"`
	if strings.Join(got, ", ") != want {
		t.Errorf("quoteIdents = %q, want %q", strings.Join(got, ", "), want)
	}

	if _, err := quoteIdents([]string{"id", `bad"name`}); err == nil {
		t.Error("quoteIdents accepted a hostile name in the middle of the list")
	}
}

// TestQuoteIdentAcceptsSanitizedColumnNames guards the interaction between the
// 63-byte truncation and the identifier regex: a column name long enough to be
// hashed must still be quotable.
func TestQuoteIdentAcceptsSanitizedColumnNames(t *testing.T) {
	long := "a" + strings.Repeat("_very_long_nested_message_field", 5)
	sanitized := sanitizeColumnName(long)

	if len(sanitized) > maxColumnNameLen {
		t.Fatalf("sanitizeColumnName produced %d bytes, over the %d limit", len(sanitized), maxColumnNameLen)
	}
	if _, err := quoteIdent(sanitized); err != nil {
		t.Errorf("quoteIdent rejected a sanitized column name %q: %v", sanitized, err)
	}
}

func TestQualify(t *testing.T) {
	got, err := qualify("e", "name")
	if err != nil {
		t.Fatalf("qualify: %v", err)
	}
	if got != `e."name"` {
		t.Errorf("qualify = %q, want %q", got, `e."name"`)
	}

	if _, err := qualify("e", "name; DROP TABLE user"); err == nil {
		t.Error("qualify accepted a hostile column name")
	}
}

// TestValidateSchemaIdentifiers is the startup guard: every table name and
// descriptor-derived column name in the real storage configuration must be a
// usable SQL identifier, checked once rather than per query.
func TestValidateSchemaIdentifiers(t *testing.T) {
	if err := validateSchemaIdentifiers(DefaultStorageTypes()); err != nil {
		t.Fatalf("DefaultStorageTypes() has an unusable SQL identifier: %v", err)
	}
}

func TestValidateSchemaIdentifiersRejectsBadTableName(t *testing.T) {
	configs := []TypeConfig{
		{MessageType: &models.User{}, TableName: `user"; DROP TABLE gear; --`},
	}
	err := validateSchemaIdentifiers(configs)
	if err == nil {
		t.Fatal("validateSchemaIdentifiers accepted a hostile table name")
	}
	if !strings.Contains(err.Error(), "unusable table name") {
		t.Errorf("error = %v, want it to name the table-name problem", err)
	}
}

func TestValidateSchemaIdentifiersRejectsBadEmbeddingField(t *testing.T) {
	configs := []TypeConfig{
		{
			MessageType: &models.Gear{},
			TableName:   "gear",
			Embeddings:  []*EmbeddingFieldConfig{{Fields: []string{"name", "description) --"}}},
		},
	}
	if err := validateSchemaIdentifiers(configs); err == nil {
		t.Fatal("validateSchemaIdentifiers accepted a hostile embedding field name")
	}
}

func TestValidateMessageColumns(t *testing.T) {
	if err := validateMessageColumns((&models.Gear{}).ProtoReflect().Descriptor()); err != nil {
		t.Errorf("validateMessageColumns(Gear) = %v, want nil", err)
	}
}

// TestEmbeddingTextColumnFlattensDottedPaths pins the fix for the stock_image
// backfill bug found while migrating #2795: EmbeddingFieldConfig.Fields holds
// proto field *paths*, which are dotted for nested messages, but the read path
// has to name the flattened column that flattenFields actually created.
// Quoting the dotted path directly produced SELECT "provider_image.description",
// a column that has never existed, and BackfillEmbeddings swallowed the error
// per-record — so stock_image embeddings silently never backfilled.
func TestEmbeddingTextColumnFlattensDottedPaths(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"name", "name"},
		{"description", "description"},
		{"provider_image.description", "provider_image_description"},
		{"provider_image.alt_description", "provider_image_alt_description"},
	}
	for _, tt := range tests {
		if got := embeddingTextColumn(tt.path); got != tt.want {
			t.Errorf("embeddingTextColumn(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// TestEmbeddingTextColumnsMatchFlattenedColumns is the property the bug above
// violated: for every table with an embedding config, every configured field
// path must resolve to a column the schema actually creates.
func TestEmbeddingTextColumnsMatchFlattenedColumns(t *testing.T) {
	for _, config := range DefaultStorageTypes() {
		if len(config.Embeddings) == 0 {
			continue
		}
		descriptor := config.MessageType.ProtoReflect().Descriptor()
		actual := make(map[string]bool)
		for _, f := range flattenFields(descriptor, "") {
			actual[f.columnName] = true
		}
		for _, embedding := range config.Embeddings {
			for _, path := range embedding.Fields {
				col := embeddingTextColumn(path)
				if !actual[col] {
					t.Errorf("table %q: embedding field %q maps to column %q, which the schema does not create",
						config.TableName, path, col)
				}
			}
		}
	}
}
