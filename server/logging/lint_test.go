package logging

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loggerMethodNames is the set of slog/logging method names whose arguments
// include alternating key-value pairs.
var loggerMethodNames = map[string]bool{
	"With":         true,
	"Info":         true,
	"InfoContext":  true,
	"Warn":         true,
	"WarnContext":  true,
	"Error":        true,
	"ErrorContext": true,
	"Debug":        true,
	"DebugContext": true,
}

// keySkipCount is the number of leading positional arguments to skip when
// counting attribute keys. For With/WithGroup the first arg is key (index 0).
// For Info/Warn/Error/Debug and their *Context variants the first arg is the
// message string (skipped for key scan).
var keySkipCount = map[string]int{
	"With":         0,
	"Info":         1,
	"InfoContext":  1,
	"Warn":         1,
	"WarnContext":  1,
	"Error":        1,
	"ErrorContext": 1,
	"Debug":        1,
	"DebugContext": 1,
}

// contextArgSkip returns the extra leading argument added by *Context methods
// (the context.Context parameter is the first argument, before the message).
var contextArgSkip = map[string]int{
	"InfoContext":  1,
	"WarnContext":  1,
	"ErrorContext": 1,
	"DebugContext": 1,
}

// TestNoEntityRequestIDLogKey walks server/services/ and server/jobs/ and
// fails if any string literal "request_id" appears as an attribute key in a
// logger call. The key request_id is reserved for the HTTP correlation ID
// injected by LoggerWithContext; entity IDs for models.Request must use
// target_request_id to avoid the collision documented in issue #1548.
func TestNoEntityRequestIDLogKey(t *testing.T) {
	// Locate the repository root relative to this file's package path.
	// The test binary cwd is the package directory (server/logging).
	repoRoot := filepath.Join("..", "..")
	dirsToScan := []string{
		filepath.Join(repoRoot, "server", "services"),
		filepath.Join(repoRoot, "server", "jobs"),
	}

	// allowlist holds file paths (relative to repo root) whose uses of
	// "request_id" as a log key are intentional HTTP-correlation usages.
	// Populated here only if middleware or similar infrastructure files
	// legitimately set "request_id". Ideally empty after Phase 1.
	allowlistFile := filepath.Join(repoRoot, "server", "logging", "request_id_log_key_allowlist.txt")
	allowlist := loadAllowlist(allowlistFile)

	fset := token.NewFileSet()
	var violations []string

	for _, dir := range dirsToScan {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			// Skip test files — test helpers that construct fixture loggers
			// may legitimately use "request_id" to test the HTTP-correlation path.
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}

			relPath, _ := filepath.Rel(repoRoot, path)
			if allowlist[relPath] {
				return nil
			}

			src, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}

			f, parseErr := parser.ParseFile(fset, path, src, 0)
			if parseErr != nil {
				// Non-Go or generated file — skip quietly.
				return nil
			}

			// Only scan files that import the logging package.
			if !importsLogging(f) {
				return nil
			}

			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				method := sel.Sel.Name
				if !loggerMethodNames[method] {
					return true
				}

				args := call.Args
				skip := keySkipCount[method]
				// *Context methods have an extra context.Context arg at the front.
				skip += contextArgSkip[method]

				// Scan even-indexed positions starting after `skip` for string key literals.
				for i := skip; i < len(args); i += 2 {
					lit, isLit := args[i].(*ast.BasicLit)
					if !isLit || lit.Kind != token.STRING {
						continue
					}
					// Strip surrounding quotes.
					val := strings.Trim(lit.Value, `"`+"`")
					if val == "request_id" {
						pos := fset.Position(lit.Pos())
						violations = append(violations,
							pos.String()+": logger attribute key \"request_id\" found — use \"target_request_id\" for models.Request entity IDs to avoid collision with the HTTP correlation key",
						)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("error walking %s: %v", dir, err)
		}
	}

	for _, v := range violations {
		t.Errorf("%s", v)
	}
}

// importsLogging returns true when the file imports the server logging package.
func importsLogging(f *ast.File) bool {
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if strings.HasSuffix(path, "/logging") || path == "go.ripls.org/ripls/server/logging" {
			return true
		}
	}
	return false
}

// loadAllowlist reads an optional allowlist file (one repo-relative path per
// line, lines starting with # are comments). Missing file is not an error.
func loadAllowlist(path string) map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]bool{}
	}
	result := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		result[line] = true
	}
	return result
}
