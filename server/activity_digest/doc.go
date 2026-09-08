// Package activity_digest assembles the daily activity digest email
// from raw storage queries. The Builder owns the cross-table fan-out
// (community events, chat messages joined to conversations, new users,
// returning sign-ins, waitlist signups, password resets), the foreign-
// key resolution (users, communities, gear), and the formatting of
// each per-community narrative line. The result is an
// email.ActivityDigestInput that the caller hands to
// email.Service.SendActivityDigest. See #1924 for design.
package activity_digest
