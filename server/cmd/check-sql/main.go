// Command check-sql walks the server/ source tree and enforces the storage
// layer's SQL-construction invariant: values are always bound, identifiers
// always go through quoteIdent, and nothing else may enter a query string.
//
// # Why this exists rather than gosec's G201/G202
//
// gosec has the right rules and cannot apply them here. Measured on adoption
// (#2795), `gosec -include=G201,G202 ./server/...` reports 0 issues across the
// whole tree, because its sqlStrFormat rule has two blind spots that together
// cover 100% of this codebase's SQL:
//
//  1. It only fires when the receiver of Query*/Exec* resolves to a concrete
//     *sql.DB / *sql.Tx / *sql.Stmt. ProtoSQLStorage holds SQLDB and
//     SQLExecutor *interfaces* — InstrumentedDB needs them for per-request
//     query counting (#1613), WithTx needs them to route through an
//     InstrumentedTx — so every query in server/storage/ is invisible to it.
//  2. It misses db.Query(fmt.Sprintf(...)) when the Sprintf is inline rather
//     than assigned to a variable first.
//
// This checker matches on the *call name*, not the receiver type, and judges
// the query argument however it is spelled. Both blind spots are covered by
// its own tests.
//
// # The rule
//
// The query argument of every Query*/Exec* call must be built only from:
//
//   - string literals and package-level constants;
//   - quoteIdent / quoteIdents / qualify / quotedTableFor results;
//   - Placeholder(n) results;
//   - fmt.Sprintf with a literal format string over the above;
//   - strings.Join / concatenation of the above.
//
// Anything else — a variable holding caller text, a struct field, a function
// result this checker does not recognise — must carry an explicit
//
//	// sql-fragment-allow: <reason>
//
// comment on its line or the line above. Unlike the repo's
// go-line-count-allow / dart-line-count-allow hatches, this marker needs no
// tracking issue: it marks permanently-legitimate raw SQL (search-config WHERE
// fragments, dialect type keywords), not deferred work. Requiring an issue
// link would manufacture issues that can never be closed.
//
// Usage:
//
//	go run ./server/cmd/check-sql [root]
//
// The optional root argument defaults to "./server".
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// sqlCallArgIndex maps a database call name to the index of its query-string
// argument. The *Context variants take ctx first.
var sqlCallArgIndex = map[string]int{
	"QueryContext":    1,
	"ExecContext":     1,
	"QueryRowContext": 1,
	"PrepareContext":  1,
	"Query":           0,
	"Exec":            0,
	"QueryRow":        0,
	"Prepare":         0,
}

// fragmentProducers are functions whose results are SQL text by construction —
// either already-quoted identifiers or fixed fragments assembled from
// constants. Adding a name here is a deliberate widening of the rule; it must
// be a function that cannot return caller-controlled text.
var fragmentProducers = map[string]bool{
	// server/storage/sqlident.go — the sanctioned identifier path.
	"quoteIdent":     true,
	"quoteIdents":    true,
	"qualify":        true,
	"quotedTableFor": true,
	// Fixed WHERE fragments assembled from package constants.
	"buildDeletedFilter": true,
	// Resolves a registered table name through quotedTableFor.
	"scheduledNotificationTableName": true,
	// DDL builders in protosql_schema.go — each quotes its own identifiers and
	// takes its type keywords from DatabaseSpecifics.
	"generateCreateTableSQL":         true,
	"generateAlterTableAddColumnSQL": true,
	"generateGeospatialIndexSQL":     true,
	// The fixed index list in protosql_schema.go: string literals, no inputs.
	"schemaIndexStatements": true,
	// Placeholder(n) renders $1, $2, … — the value-binding half of the rule.
	"Placeholder": true,
}

// allowMarker opts a single expression out of the rule. A reason is mandatory.
const allowMarker = "sql-fragment-allow:"

// bareAllowMarker is the marker without a reason, which is rejected so the
// hatch cannot be used silently.
const bareAllowMarker = "sql-fragment-allow"

type violation struct {
	pos token.Position
	msg string
}

func main() {
	root := "./server"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	fset := token.NewFileSet()
	var files []*ast.File

	// G703: `root` is os.Args[1]. Walking an operator-named directory is what
	// this lint tool is for, and it runs with that operator's privileges.
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error { //nolint:gosec // G703: root is this CLI tool's own argument, by design.
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Generated protobuf/connect code, and vendored Go inside npm trees.
			if d.Name() == "gen" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		files = append(files, f)
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-sql: walk error: %v\n", err)
		os.Exit(1)
	}

	consts := collectConstNames(files)

	var violations []violation
	for _, f := range files {
		violations = append(violations, checkFile(fset, f, consts)...)
	}

	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "check-sql: %d unsafe SQL construction(s) found:\n", len(violations))
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "  %s:%d:%d: %s\n", v.pos.Filename, v.pos.Line, v.pos.Column, v.msg)
		}
		fmt.Fprintf(os.Stderr, "\nValues must be bound with Placeholder(n); identifiers must go through\n"+
			"quoteIdent. If the expression is genuinely a fixed SQL fragment, mark it:\n"+
			"    // %s <why this cannot carry caller input>\n", allowMarker)
		os.Exit(1)
	}
	fmt.Printf("check-sql: SQL construction in %s is parameterized and identifier-quoted\n", root)
}

// collectConstNames gathers every package-level constant name in the tree. A
// constant is compile-time text and can never carry caller input, so it is
// always safe to interpolate.
func collectConstNames(files []*ast.File) map[string]bool {
	names := map[string]bool{}
	for _, f := range files {
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range vs.Names {
					names[name.Name] = true
				}
			}
		}
	}
	return names
}

// checker holds the per-file state for one pass.
type checker struct {
	fset       *token.FileSet
	consts     map[string]bool
	allowLines map[int]bool

	// safe tracks local identifiers currently holding SQL-safe text.
	safe map[string]bool
	// placeholderFuncs tracks locals aliased to a Placeholder function, e.g.
	// `p := s.dbSpec.Placeholder`, so `p(1)` is recognised.
	placeholderFuncs map[string]bool
}

func checkFile(fset *token.FileSet, f *ast.File, consts map[string]bool) []violation {
	c := &checker{
		fset:       fset,
		consts:     consts,
		allowLines: collectAllowLines(fset, f),
	}

	var violations []violation
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		// Skip pass-through wrappers: a method that implements one of the SQL
		// call names by forwarding its own `query` parameter (InstrumentedDB,
		// InstrumentedTx) has nothing to judge — the string was already judged
		// wherever it was built.
		if isPassThroughWrapper(fn) {
			continue
		}
		c.safe = map[string]bool{}
		c.placeholderFuncs = map[string]bool{}
		// Parameters carry caller input by definition. Record them explicitly so
		// a parameter that happens to share a name with a package constant is
		// not laundered by the constant lookup in isSafe.
		markParamsUnsafe(fn, c.safe)
		violations = append(violations, c.checkFunc(fn)...)
	}

	// A bare marker with no reason is a violation wherever it appears.
	violations = append(violations, bareMarkerViolations(fset, f)...)
	return violations
}

// markParamsUnsafe records every named parameter of fn as unsafe.
func markParamsUnsafe(fn *ast.FuncDecl, safe map[string]bool) {
	if fn.Type.Params == nil {
		return
	}
	for _, param := range fn.Type.Params.List {
		for _, name := range param.Names {
			if name.Name != "_" {
				safe[name.Name] = false
			}
		}
	}
}

// isPassThroughWrapper reports whether fn implements one of the SQL call names
// and takes a `query string` parameter — the InstrumentedDB / InstrumentedTx
// shape, which forwards a string built and checked elsewhere.
func isPassThroughWrapper(fn *ast.FuncDecl) bool {
	if fn.Recv == nil {
		return false
	}
	if _, ok := sqlCallArgIndex[fn.Name.Name]; !ok {
		return false
	}
	for _, param := range fn.Type.Params.List {
		for _, name := range param.Names {
			if name.Name == "query" {
				return true
			}
		}
	}
	return false
}

// collectAllowLines records the lines covered by a well-formed
// `// sql-fragment-allow: <reason>` comment — the comment's own line and the
// line after it, so the marker works both as a trailing comment and as a
// comment on the preceding line.
func collectAllowLines(fset *token.FileSet, f *ast.File) map[int]bool {
	lines := map[int]bool{}
	for _, group := range f.Comments {
		for _, comment := range group.List {
			text, ok := markerText(comment.Text)
			if !ok || !strings.HasPrefix(text, allowMarker) {
				continue
			}
			reason := strings.TrimSpace(strings.TrimPrefix(text, allowMarker))
			if reason == "" {
				continue // bare marker; reported separately
			}
			line := fset.Position(comment.Pos()).Line
			lines[line] = true
			lines[line+1] = true
		}
	}
	return lines
}

// markerText strips a comment's leader and surrounding space, so a marker is
// recognised only when it *starts* the comment. A doc comment that mentions
// the marker name in prose is not a marker.
func markerText(raw string) (string, bool) {
	switch {
	case strings.HasPrefix(raw, "//"):
		return strings.TrimSpace(raw[2:]), true
	case strings.HasPrefix(raw, "/*"):
		return strings.TrimSpace(strings.TrimSuffix(raw[2:], "*/")), true
	}
	return "", false
}

// bareMarkerViolations reports markers written without a reason.
func bareMarkerViolations(fset *token.FileSet, f *ast.File) []violation {
	var out []violation
	for _, group := range f.Comments {
		for _, comment := range group.List {
			text, ok := markerText(comment.Text)
			if !ok {
				continue
			}
			// Only an attempt at the marker is reported. `sql-fragment-allow`
			// or `sql-fragment-allow:` alone is a marker missing its reason;
			// `sql-fragment-allow markers are …` is prose, and prose does not
			// suppress anything, so the expression stays flagged on its own.
			trimmed := strings.TrimSpace(strings.TrimSuffix(text, ":"))
			if trimmed != bareAllowMarker {
				continue
			}
			out = append(out, violation{
				pos: fset.Position(comment.Pos()),
				msg: fmt.Sprintf("%s marker has no reason — write `// %s <why this cannot carry caller input>`",
					bareAllowMarker, allowMarker),
			})
		}
	}
	return out
}

// checkFunc walks one function body in source order, tracking which locals
// hold SQL-safe text and judging every database call it finds.
func (c *checker) checkFunc(fn *ast.FuncDecl) []violation {
	var violations []violation

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			c.trackAssign(node)
		case *ast.DeclStmt:
			c.trackVarDecl(node)
		case *ast.RangeStmt:
			c.trackRange(node)
		case *ast.CallExpr:
			if v, bad := c.checkSQLCall(node); bad {
				violations = append(violations, v)
			}
		}
		return true
	})

	return violations
}

// trackAssign updates the safe-locals map for an assignment statement.
func (c *checker) trackAssign(stmt *ast.AssignStmt) {
	// `p := s.dbSpec.Placeholder` — a function value, not a string.
	if len(stmt.Lhs) == 1 && len(stmt.Rhs) == 1 {
		if ident, ok := stmt.Lhs[0].(*ast.Ident); ok {
			if sel, ok := stmt.Rhs[0].(*ast.SelectorExpr); ok && sel.Sel.Name == "Placeholder" {
				c.placeholderFuncs[ident.Name] = true
				return
			}
		}
	}

	// `x[i] = expr` — narrows the slice's safety.
	if len(stmt.Lhs) == 1 && len(stmt.Rhs) == 1 {
		if idx, ok := stmt.Lhs[0].(*ast.IndexExpr); ok {
			if ident, ok := idx.X.(*ast.Ident); ok {
				if !c.isSafe(stmt.Rhs[0]) {
					c.safe[ident.Name] = false
				}
				return
			}
		}
	}

	// Multi-value RHS (`x, err := f()`): judge the single call for the first LHS.
	if len(stmt.Rhs) == 1 && len(stmt.Lhs) > 1 {
		safe := c.isSafe(stmt.Rhs[0])
		if ident, ok := stmt.Lhs[0].(*ast.Ident); ok && ident.Name != "_" {
			c.assignSafety(ident.Name, stmt.Tok, safe)
		}
		return
	}

	for i, lhs := range stmt.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok || ident.Name == "_" || i >= len(stmt.Rhs) {
			continue
		}
		c.assignSafety(ident.Name, stmt.Tok, c.isSafe(stmt.Rhs[i]))
	}
}

// assignSafety applies an assignment's safety to a local.
//
// Only a declaration (`:=`) sets safety outright. A plain `=` or `+=` can
// weaken it but never strengthen it, because this checker walks the AST in
// source order and cannot tell a conditional branch from an unconditional one:
//
//	func f(limit int) {
//	    if limit <= 0 { limit = defaultLimit }   // <- NOT proof limit is safe
//	    ... fmt.Sprintf("... LIMIT %d", limit)
//	}
//
// Treating that reassignment as authoritative would launder the caller's value
// on the branch that does not run. Erring toward "still unsafe" costs a
// marker at worst; erring the other way is how a checker like this quietly
// stops checking.
func (c *checker) assignSafety(name string, tok token.Token, safe bool) {
	if tok == token.DEFINE {
		c.safe[name] = safe
		return
	}
	if !safe {
		c.safe[name] = false
	}
}

// trackVarDecl marks `var x string` / `var x []string` safe (zero value) and
// judges `var x = expr`.
func (c *checker) trackVarDecl(stmt *ast.DeclStmt) {
	gen, ok := stmt.Decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR {
		return
	}
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range vs.Names {
			if len(vs.Values) == 0 {
				c.safe[name.Name] = true
				continue
			}
			if i < len(vs.Values) {
				c.safe[name.Name] = c.isSafe(vs.Values[i])
			}
		}
	}
}

// trackRange marks the value variable of a range over a safe slice as safe.
func (c *checker) trackRange(stmt *ast.RangeStmt) {
	safe := c.isSafe(stmt.X)
	if v, ok := stmt.Value.(*ast.Ident); ok && v.Name != "_" {
		c.safe[v.Name] = safe
	}
}

// checkSQLCall judges a database call's query argument. It matches on the call
// name rather than the receiver type, which is the whole point: gosec's
// equivalent rule requires a concrete *sql.DB and therefore sees nothing here.
func (c *checker) checkSQLCall(call *ast.CallExpr) (violation, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return violation{}, false
	}
	argIdx, ok := sqlCallArgIndex[sel.Sel.Name]
	if !ok {
		return violation{}, false
	}
	// `u.Query()` on a URL takes no arguments; a SQL call always has a query.
	if len(call.Args) <= argIdx {
		return violation{}, false
	}

	arg := call.Args[argIdx]
	if c.isSafe(arg) {
		return violation{}, false
	}

	return violation{
		pos: c.fset.Position(arg.Pos()),
		msg: fmt.Sprintf("%s query argument is not provably parameterized (%s)",
			sel.Sel.Name, describe(arg)),
	}, true
}

// isSafe reports whether an expression can only produce SQL text built from
// literals, constants, quoted identifiers and placeholders.
func (c *checker) isSafe(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	// An explicit marker overrides the analysis.
	if c.allowLines[c.fset.Position(expr.Pos()).Line] {
		return true
	}

	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING

	case *ast.ParenExpr:
		return c.isSafe(e.X)

	case *ast.Ident:
		if safe, tracked := c.safe[e.Name]; tracked {
			return safe
		}
		return c.consts[e.Name]

	case *ast.BinaryExpr:
		return e.Op == token.ADD && c.isSafe(e.X) && c.isSafe(e.Y)

	case *ast.CallExpr:
		return c.isSafeCall(e)

	case *ast.SelectorExpr:
		// A package-level constant referenced as pkg.Name.
		return c.consts[e.Sel.Name]

	case *ast.IndexExpr:
		// An element of a safe slice is safe.
		return c.isSafe(e.X)

	case *ast.SliceExpr:
		// A sub-slice of a safe slice is safe.
		return c.isSafe(e.X)

	case *ast.CompositeLit:
		// []string{"...", "..."} — safe when every element is.
		for _, elt := range e.Elts {
			if !c.isSafe(elt) {
				return false
			}
		}
		return true
	}
	return false
}

// isSafeCall judges the recognised SQL-fragment-producing calls.
func (c *checker) isSafeCall(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		// A local aliased to a Placeholder function: `p := ...Placeholder; p(1)`.
		if c.placeholderFuncs[fun.Name] {
			return true
		}
		switch fun.Name {
		case "make":
			// A freshly made slice holds nothing yet; element assignments narrow it.
			return true
		case "append":
			// The destination's existing safety carries: appending a literal to
			// a slice that already holds caller text does not make it safe.
			if len(call.Args) == 0 || !c.isSafe(call.Args[0]) {
				return false
			}
			for _, arg := range call.Args[1:] {
				if !c.isSafe(arg) {
					return false
				}
			}
			return true
		case "string":
			return len(call.Args) == 1 && c.isSafe(call.Args[0])
		}
		return fragmentProducers[fun.Name]

	case *ast.SelectorExpr:
		name := fun.Sel.Name
		if fragmentProducers[name] {
			return true
		}
		switch name {
		case "Sprintf":
			// The format string must be a literal, and every argument safe.
			if len(call.Args) == 0 || !c.isSafe(call.Args[0]) {
				return false
			}
			for _, arg := range call.Args[1:] {
				if !c.isSafe(arg) {
					return false
				}
			}
			return true
		case "Join":
			return len(call.Args) == 2 && c.isSafe(call.Args[0])
		case "Repeat", "TrimSuffix", "TrimPrefix", "TrimSpace":
			return len(call.Args) > 0 && c.isSafe(call.Args[0])
		}
		return false
	}
	return false
}

// describe renders a short, human-readable shape for the violation message.
func describe(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return "variable " + e.Name
	case *ast.SelectorExpr:
		return "field or method " + e.Sel.Name
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			return "string concatenation"
		}
	case *ast.CallExpr:
		switch fun := e.Fun.(type) {
		case *ast.Ident:
			return "result of " + fun.Name + "()"
		case *ast.SelectorExpr:
			return "result of " + fun.Sel.Name + "()"
		}
	}
	return "unrecognized expression"
}
