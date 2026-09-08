// Package esm is the shared library for Experience Sampling Methodology
// (ESM). It owns the storage queries, the response aggregation, and the
// feed-item construction for ESM prompts. Both the ESM service (response
// submission, signal aggregation) and the feed service (feed assembly)
// depend on this library so that no service-to-service injection is
// required between them.
package esm
