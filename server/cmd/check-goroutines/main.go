// Command check-goroutines walks the server/ source tree, finds every go
// statement, and verifies that the spawned function literal begins with a
// recover defer. This prevents bare goroutines (which can crash the server
// process on panic) from being merged without protection.
//
// Files matching the allowlist or _test.go suffix are exempt.
//
// Usage:
//
//	go run ./server/cmd/check-goroutines [root]
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

// allowlist contains files that are permitted to spawn goroutines without an
// inline recover because they either define the recovery helper themselves,
// use the recovery pattern unconventionally for documented reasons, or are
// non-production tooling (test harnesses, simulation scripts) where a panic
// crashing the process is the desired failure mode.
var allowlist = map[string]bool{
	"server/logging/logger.go":   true,
	"server/streaming/sender.go": true,
	// Test infrastructure: integration test helpers are not production server
	// code; panics crashing the process is acceptable.
	"server/integration_tests/test_helpers.go": true,
	// Simulation scripts run as standalone tools, not as part of the server.
	"server/simulation/concurrent.go": true,
}

func main() {
	root := "./server"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	fset := token.NewFileSet()
	var violations []string

	// G703: `root` is os.Args[1]. Walking an operator-named directory is what
	// this lint tool is for, and it runs with that operator's privileges.
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error { //nolint:gosec // G703: root is this CLI tool's own argument, by design.
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Test files are exempt — they legitimately spawn deliberately-crashing
		// goroutines to exercise the recovery paths.
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Normalise path separators for allowlist lookup.
		normPath := filepath.ToSlash(path)
		// Strip leading "./" for allowlist comparison.
		normPath = strings.TrimPrefix(normPath, "./")
		if allowlist[normPath] {
			return nil
		}

		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			goStmt, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}
			// The go statement must spawn a function literal.
			callExpr, ok := goStmt.Call.Fun.(*ast.FuncLit)
			if !ok {
				pos := fset.Position(goStmt.Pos())
				violations = append(violations, fmt.Sprintf(
					"%s:%d: go statement spawns non-literal; wrap in a func literal with recover",
					pos.Filename, pos.Line,
				))
				return true
			}
			// Check that the LAST registered defer statement in the function
			// body is a recover defer (LIFO: last registered = first run on panic).
			if !hasRecoverDefer(callExpr.Body) {
				pos := fset.Position(goStmt.Pos())
				violations = append(violations, fmt.Sprintf(
					"%s:%d: goroutine missing recover defer — use logging.GoSafe or add defer func(){if r:=recover();r!=nil{…}}() as the last defer in the function body",
					pos.Filename, pos.Line,
				))
			}
			return true
		})
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-goroutines: walk error: %v\n", err)
		os.Exit(1)
	}

	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "check-goroutines: %d unprotected goroutine(s) found:\n", len(violations))
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "  %s\n", v)
		}
		os.Exit(1)
	}
	fmt.Printf("check-goroutines: all goroutines in %s are protected\n", root)
}

// hasRecoverDefer reports whether the function body contains at least one
// defer that calls recover(). The defer may be anywhere in the body — the
// LIFO rule guarantees a defer registered last runs first on panic, so any
// position is sufficient.
func hasRecoverDefer(body *ast.BlockStmt) bool {
	for _, stmt := range body.List {
		deferStmt, ok := stmt.(*ast.DeferStmt)
		if !ok {
			continue
		}
		if isRecoverFuncLit(deferStmt.Call) {
			return true
		}
	}
	return false
}

// isRecoverFuncLit returns true when call is an immediately-invoked function
// literal whose body contains a recover() call, i.e. the pattern:
//
//	defer func() { if r := recover(); r != nil { … } }()
func isRecoverFuncLit(call *ast.CallExpr) bool {
	lit, ok := call.Fun.(*ast.FuncLit)
	if !ok {
		return false
	}
	return bodyCallsRecover(lit.Body)
}

// bodyCallsRecover returns true when the block contains a call to recover().
func bodyCallsRecover(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := callExpr.Fun.(*ast.Ident)
		if ok && ident.Name == "recover" {
			found = true
			return false
		}
		return true
	})
	return found
}
