// Package streaming provides a generic serializing sender for Connect
// server-streaming RPCs. It serializes concurrent emits from multiple goroutines
// onto a single Send target, satisfying Connect's one-Send-at-a-time contract.
package streaming
