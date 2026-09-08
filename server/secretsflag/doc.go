// Package secretsflag resolves a credential value from either a -<name>
// value flag or a paired -<name>-file=<path> file flag, so the operator can
// keep secret bytes out of the process command line (and out of
// /proc/<pid>/cmdline).
//
// Callers pair an existing flag.String with a sibling -<name>-file flag and
// call Resolve at startup; the helper trims a single trailing newline and
// rejects both-flags-set as ambiguous.
package secretsflag
