// Package known_for derives "is known for" topic tags from items a
// user or community has shared into scope. Three sources contribute
// to a single shared counts map keyed on the normalized category
// string — gear listed in scope (with a +1 bonus per completed
// loan), experiences shared into scope (with a +1 bonus when state
// is COMPLETED), and requests shared into scope (with a +1 bonus
// when state is FULFILLED). A category surfacing from more than one
// source collapses into one chip whose weight is the sum.
//
// Both the user profile (ModePerUser) and the workshop community
// screen (ModePerCommunity) consume the same vocabulary so the
// surfaces read with the same voice. See README.md for the full
// table and conventions.
package known_for
