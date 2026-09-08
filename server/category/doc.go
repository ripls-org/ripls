// Package category derives a coarse category label from
// user-provided text (e.g. an experience's name + description, a
// request's title + description) using a keyword dictionary.
//
// The categorizer is a stopgap for the experience- and request-side
// TODO(#2013) markers in `server/services/experience/gen_ai.go` and
// `server/services/request/gen.go`. Until those are resolved by a
// proper LLM pass, the keyword heuristic in this package gives the
// `known_for` derivation enough signal to surface "host" and "asker"
// chips alongside the existing gear-based "lender" chips.
package category
