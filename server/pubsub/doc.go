// Package pubsub provides a generic in-process publish/subscribe primitive
// over typed events. Subscribers consume serially per-subscriber; the publisher
// fans out to all subscribers concurrently. Default semantics are at-most-once,
// in-memory, with bounded-channel backpressure. See README.md for the contract.
package pubsub
