// Package branding holds the instance's user-facing identity — product name,
// link origins, legal details, and store listings — as a value type that any
// layer can depend on.
//
// It sits below server/config on purpose: services must be able to read
// branding without importing the flag-parsing package, which only the top-level
// wiring files do.
package branding
