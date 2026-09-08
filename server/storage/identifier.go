// Identifier validation for SQL column and table names used in dynamic queries.

package storage

import (
	"fmt"
	"regexp"
)

// identifierRe matches safe SQL identifiers: letters, digits, and underscores,
// starting with a letter or underscore. This covers all proto-derived column
// names and table names registered in the storage layer.
var identifierRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// validateIdentifier returns an error if name is not a safe SQL identifier.
// All field names and table names used in dynamically-built SQL queries must
// pass this check before interpolation to prevent identifier injection.
func validateIdentifier(name string) error {
	if !identifierRe.MatchString(name) {
		return fmt.Errorf("invalid SQL identifier %q: must match ^[a-zA-Z_][a-zA-Z0-9_]*$", name)
	}
	return nil
}
