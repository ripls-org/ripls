// Package category derives a coarse category label from
// user-provided text (e.g. an experience's name + description, a
// request's title + description) using a keyword dictionary.
//
// The AI pipeline does not set a category for experiences or requests,
// so this keyword heuristic is what gives the `known_for` derivation
// enough signal to surface "host" and "asker" chips alongside the
// gear-based "lender" chips.
package category
