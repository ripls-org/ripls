package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// analyze parses one synthetic file and returns the violation messages.
func analyze(t *testing.T, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "probe.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	consts := collectConstNames([]*ast.File{f})
	var msgs []string
	for _, v := range checkFile(fset, f, consts) {
		msgs = append(msgs, v.msg)
	}
	return msgs
}

const preamble = `package probe

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const notDeletedFilter = "AND deleted = 0"

type spec struct{}

func (spec) Placeholder(i int) string { return "$1" }

type store struct {
	db     interface {
		QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
		ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	}
	raw    *sql.DB
	dbSpec spec
}

func quoteIdent(name string) (string, error) { return "\"" + name + "\"", nil }
func quoteIdents(n []string) ([]string, error) { return n, nil }

var _ = fmt.Sprintf
var _ = strings.Join
`

func TestRejectsRawValueInterpolation(t *testing.T) {
	msgs := analyze(t, preamble+`
func (s *store) f(ctx context.Context, name string) {
	q := fmt.Sprintf("SELECT * FROM users WHERE name = '%s'", name)
	s.db.QueryContext(ctx, q)
}
`)
	if len(msgs) != 1 {
		t.Fatalf("got %d violations %v, want 1", len(msgs), msgs)
	}
	if !strings.Contains(msgs[0], "variable q") {
		t.Errorf("message = %q, want it to name the query variable", msgs[0])
	}
}

// TestCatchesInterfaceReceiver is gosec blind spot #1: G201 only fires when the
// receiver resolves to a concrete *sql.DB, so an interface-typed executor —
// exactly what ProtoSQLStorage holds — hides the query entirely. This checker
// matches on the call name instead.
func TestCatchesInterfaceReceiver(t *testing.T) {
	msgs := analyze(t, preamble+`
func (s *store) viaInterface(ctx context.Context, name string) {
	q := fmt.Sprintf("SELECT * FROM users WHERE name = '%s'", name)
	s.db.QueryContext(ctx, q)
}
`)
	if len(msgs) != 1 {
		t.Fatalf("interface-typed executor not flagged: got %v", msgs)
	}
}

// TestCatchesInlineSprintf is gosec blind spot #2: db.Query(fmt.Sprintf(...))
// is missed when the Sprintf is inline rather than assigned first.
func TestCatchesInlineSprintf(t *testing.T) {
	msgs := analyze(t, preamble+`
func (s *store) inline(ctx context.Context, name string) {
	s.raw.QueryContext(ctx, fmt.Sprintf("SELECT * FROM users WHERE name = '%s'", name))
}
`)
	if len(msgs) != 1 {
		t.Fatalf("inline Sprintf not flagged: got %v", msgs)
	}
}

func TestRejectsConcatenation(t *testing.T) {
	msgs := analyze(t, preamble+`
func (s *store) concat(ctx context.Context, name string) {
	s.db.QueryContext(ctx, "SELECT * FROM users WHERE name = '"+name+"'")
}
`)
	if len(msgs) != 1 {
		t.Fatalf("concatenation not flagged: got %v", msgs)
	}
	if !strings.Contains(msgs[0], "concatenation") {
		t.Errorf("message = %q, want it to name the concatenation", msgs[0])
	}
}

func TestAcceptsBoundValuesAndQuotedIdentifiers(t *testing.T) {
	msgs := analyze(t, preamble+`
func (s *store) ok(ctx context.Context, table, field string, values []string) error {
	quotedTable, err := quoteIdent(table)
	if err != nil {
		return err
	}
	quotedField, err := quoteIdent(field)
	if err != nil {
		return err
	}
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		placeholders[i] = s.dbSpec.Placeholder(i + 1)
		args[i] = v
	}
	q := fmt.Sprintf("SELECT binary_proto FROM %s WHERE %s IN (%s) %s",
		quotedTable, quotedField, strings.Join(placeholders, ", "), notDeletedFilter)
	s.db.QueryContext(ctx, q, args...)
	return nil
}
`)
	if len(msgs) != 0 {
		t.Fatalf("conforming code flagged: %v", msgs)
	}
}

func TestAcceptsStringLiteralQuery(t *testing.T) {
	msgs := analyze(t, preamble+`
func (s *store) lit(ctx context.Context, id string) {
	s.db.QueryContext(ctx, "SELECT 1 FROM gear WHERE id = $1", id)
}
`)
	if len(msgs) != 0 {
		t.Fatalf("literal query flagged: %v", msgs)
	}
}

// TestConditionalReassignmentDoesNotLaunder pins the soundness rule: a plain
// `=` inside a branch cannot prove a parameter is safe, because the other
// branch still carries the caller's value.
func TestConditionalReassignmentDoesNotLaunder(t *testing.T) {
	msgs := analyze(t, preamble+`
const defaultLimit = "200"

func (s *store) limits(ctx context.Context, limit string) {
	if limit == "" {
		limit = defaultLimit
	}
	q := fmt.Sprintf("SELECT 1 FROM gear LIMIT %s", limit)
	s.db.QueryContext(ctx, q)
}
`)
	if len(msgs) != 1 {
		t.Fatalf("conditionally-reassigned parameter was laundered: got %v", msgs)
	}
}

func TestAllowMarkerSuppresses(t *testing.T) {
	msgs := analyze(t, preamble+`
type cfg struct{ extraWhere string }

func (s *store) fragment(ctx context.Context, c cfg) {
	where := "community_id = $1"
	// sql-fragment-allow: cfg fields are package-level literals
	where += " " + c.extraWhere
	q := fmt.Sprintf("SELECT 1 FROM gear WHERE %s", where)
	s.db.QueryContext(ctx, q)
}
`)
	if len(msgs) != 0 {
		t.Fatalf("marker did not suppress: %v", msgs)
	}
}

func TestAllowMarkerRequiresReason(t *testing.T) {
	msgs := analyze(t, preamble+`
type cfg struct{ extraWhere string }

func (s *store) fragment(ctx context.Context, c cfg) {
	where := "community_id = $1"
	// sql-fragment-allow
	where += " " + c.extraWhere
	q := fmt.Sprintf("SELECT 1 FROM gear WHERE %s", where)
	s.db.QueryContext(ctx, q)
}
`)
	var sawBare bool
	for _, m := range msgs {
		if strings.Contains(m, "has no reason") {
			sawBare = true
		}
	}
	if !sawBare {
		t.Fatalf("bare marker accepted: %v", msgs)
	}
}

// TestProseMentionIsNotAMarker guards the distinction between a marker and a
// doc comment that talks about markers — the type docs and this command's own
// package doc both do.
func TestProseMentionIsNotAMarker(t *testing.T) {
	msgs := analyze(t, preamble+`
// cfg carries raw fragments, which is why its call sites use
// sql-fragment-allow markers.
type cfg struct{ extraWhere string }

func (s *store) fragment(ctx context.Context, c cfg) {
	q := fmt.Sprintf("SELECT 1 FROM gear WHERE %s", c.extraWhere)
	s.db.QueryContext(ctx, q)
}
`)
	if len(msgs) != 1 {
		t.Fatalf("prose mention changed the outcome: got %v", msgs)
	}
	for _, m := range msgs {
		if strings.Contains(m, "has no reason") {
			t.Errorf("prose mention reported as a bare marker: %v", msgs)
		}
	}
}

// TestPassThroughWrapperExempt covers InstrumentedDB / InstrumentedTx, which
// forward a query string that was already judged where it was built.
func TestPassThroughWrapperExempt(t *testing.T) {
	msgs := analyze(t, preamble+`
type wrapper struct{ inner *sql.DB }

func (w *wrapper) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return w.inner.QueryContext(ctx, query, args...)
}
`)
	if len(msgs) != 0 {
		t.Fatalf("pass-through wrapper flagged: %v", msgs)
	}
}

// TestURLQueryNotFlagged guards against matching url.Values.Query(), which
// shares a name with the SQL call but takes no arguments.
func TestURLQueryNotFlagged(t *testing.T) {
	msgs := analyze(t, preamble+`
type urlish struct{}

func (urlish) Query() map[string][]string { return nil }

func readParam(u urlish) []string {
	q := u.Query()
	return q["token"]
}
`)
	if len(msgs) != 0 {
		t.Fatalf("url.Query() flagged: %v", msgs)
	}
}

func TestAppendDoesNotLaunderUnsafeSlice(t *testing.T) {
	msgs := analyze(t, preamble+`
func taint(name string) []string { return []string{name} }

func (s *store) appended(ctx context.Context, name string) {
	clauses := taint(name)
	clauses = append(clauses, "AND deleted = 0")
	q := fmt.Sprintf("SELECT 1 FROM gear WHERE %s", strings.Join(clauses, " "))
	s.db.QueryContext(ctx, q)
}
`)
	if len(msgs) != 1 {
		t.Fatalf("append laundered an unsafe slice: got %v", msgs)
	}
}
