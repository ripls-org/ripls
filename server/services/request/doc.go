// Package request implements the RequestService RPC interface: creating and
// managing borrowing or help requests, AI-generated request descriptions, offer
// matching, fulfillment lifecycle, needs and contributions, and impact
// estimation at fulfillment.
//
// Async writers: any goroutine that does a read-modify-write on a live entity
// MUST use the tx-guarded pattern to avoid clobbering concurrent mutations:
// storage.WithTx + GetByID(QueryOptions{ForUpdate: true, IncludeDeleted: true}),
// copy only the owned fields onto the fresh snapshot, then plain tx.Update.
// See storage.WithTx godoc → "Locked reload" and #2900 for rationale.
package request
