// Package activitystamp records per-user daily activity for the ops
// activity digest (#2665). It wraps authenticated HTTP routes and, at
// most once per user per interval, asynchronously upserts a
// (user, UTC day) row into the user_active_day telemetry table — giving
// the digest a true "active users" count instead of inferring activity
// from refresh-token churn. Stamping is strictly best-effort: it never
// adds latency to or fails a request.
package activitystamp
