package storage

import "errors"

// ErrRecordNotFound is returned when a Get/Update target is missing.
var ErrRecordNotFound = errors.New("storage: record not found")

// ErrMediaDeletedDuringCopy is returned by CopyStockImageForUser when the new
// media record was deleted while its bucket copy ran, typically because its
// owner or community was deleted meanwhile. The copied object has been removed;
// there is nothing to retry, so callers treat it as an outcome, not a failure.
var ErrMediaDeletedDuringCopy = errors.New("storage: media deleted during copy")

// ErrForUpdateOutsideTransaction is returned when GetByID is called with
// QueryOptions{ForUpdate: true} outside of a WithTx callback. SELECT FOR UPDATE
// outside a transaction acquires and immediately releases a lock, which is
// not useful and almost certainly a caller bug.
var ErrForUpdateOutsideTransaction = errors.New("storage: ForUpdate requires an active transaction")
