// Package errs classifies infrastructure errors as transient or
// persistent, and helps long-running background jobs decide when
// a streak of transient failures should escalate from WARN to
// ERROR so on-call gets paged.
package errs
