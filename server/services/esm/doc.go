// Package esm implements the ESMService RPC interface for submitting
// responses to Experience Sampling Methodology prompts and reading
// host-facing aggregated signal. Storage queries, aggregation, and
// feed-item construction live in the server/esm shared library so the
// feed service can consume the same logic without service-to-service
// injection.
package esm
