// Package needs is the shared library that detects and surfaces
// time-pressured community Requests for the Workshop's "This week need"
// surface.
//
// Per docs/server/architecture.md (Pattern 1 — Standalone Libraries),
// this package is consumed by the Workshop service but does not depend
// on it. The Request service (or any future service) can also consume
// these helpers without service-to-service coupling.
//
// The detection rule in v1: an open Request (REQUEST_STATE_ACTIVE) in
// one of the host's circles with low claim count (under-claimed) and
// approaching deadline (≤ 7 days). Selection is conservative — at most
// one need surfaces per Workshop fetch.
package needs
