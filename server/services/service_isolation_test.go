package services_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestServiceIsolation verifies that no file under server/services/<A>/ imports
// server/services/<B>/ for B ≠ A. Services must depend on shared libraries, not
// on each other; see docs/server/architecture.md §"Service Independence".
func TestServiceIsolation(t *testing.T) {
	// This test file lives in server/services/. "." is server/services/.
	serviceDir := "."
	entries, err := os.ReadDir(serviceDir)
	if err != nil {
		t.Fatalf("cannot read services directory: %v", err)
	}

	// Collect the set of service package sub-paths, e.g. "services/community".
	serviceNames := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() {
			serviceNames["server/services/"+e.Name()] = true
		}
	}

	fset := token.NewFileSet()

	err = filepath.WalkDir(serviceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}

		// Determine which service this file belongs to.
		rel, _ := filepath.Rel(serviceDir, path)
		parts := strings.SplitN(rel, string(os.PathSeparator), 2)
		if len(parts) < 2 {
			return nil
		}
		ownerService := "server/services/" + parts[0]

		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			// Skip files that don't parse (e.g. generated files not yet present).
			return nil
		}

		for _, imp := range f.Imports {
			// Strip surrounding quotes from the import path.
			importPath := strings.Trim(imp.Path.Value, `"`)

			// Ignore non-service imports.
			if !strings.HasPrefix(importPath, "go.ripls.org/ripls/server/services/") {
				continue
			}

			// Extract "server/services/<name>" from the full module path.
			afterModule := strings.TrimPrefix(importPath, "go.ripls.org/ripls/")
			// afterModule is now e.g. "server/services/community" or "server/services/community/subpkg"
			importedService := strings.Join(strings.SplitN(afterModule, "/", 4)[:3], "/")

			if importedService != ownerService && serviceNames[importedService] {
				t.Errorf(
					"%s (service %q) imports %q — services must not import other services; "+
						"move shared code to a library package under server/",
					path, ownerService, importPath,
				)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}
