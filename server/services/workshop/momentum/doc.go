// Package momentum is the Workshop service's cascade-priority detection
// and lever-copy validation subpackage. It owns the pure-function
// detectors that decide which `StoredNudge` rows to generate on Workshop
// surfaces (Quest hero, Bring Something Back, seed-the-catalyst) and the
// validator that enforces the brief's lever-copy rules (≤9 words,
// contains a concrete object).
//
// The package is consumed by the parent workshop service (brief
// assembly, generation pipeline, lever-copy tests) and by
// server/jobs/WorkshopGenerationJob, which pre-warms nudges without
// pulling in the full Connect handler stack.
//
// Dependencies are limited to `server/storage` and `server/esm` (for the
// ESM repeat-signal detector, which calls `esm.AggregateAttendeesOnly`
// directly).
package momentum
