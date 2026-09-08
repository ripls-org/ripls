// Package config defines the server's command-line flag surface and turns it
// into a validated Config: it declares every flag, resolves paired
// -<name>-file secret flags via secretsflag, and enforces startup invariants
// (required flags, JWT secret length, CORS allowlist rules) before the rest
// of the process boots.
package config
