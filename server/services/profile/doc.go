// Package profile implements the ProfileService RPC interface for the
// viewer-facing user profile surface.
//
// GetUserProfileForViewer returns the target user's identity, the list
// of communities the viewer and target share, a count of communities
// the viewer cannot see, and the target's aggregate hero/ticker numbers
// plus a short list of viewer-actionable postcards. Aggregate numbers
// span the target's full activity; per-community identifiers are
// emitted only when the viewer is a member.
package profile
