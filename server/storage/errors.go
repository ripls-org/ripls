package storage

import "errors"

// ErrRecordNotFound is returned when a Get/Update target is missing.
var ErrRecordNotFound = errors.New("storage: record not found")

// ErrForUpdateOutsideTransaction is returned when GetByID is called with
// QueryOptions{ForUpdate: true} outside of a WithTx callback. SELECT FOR UPDATE
// outside a transaction acquires and immediately releases a lock, which is
// not useful and almost certainly a caller bug.
var ErrForUpdateOutsideTransaction = errors.New("storage: ForUpdate requires an active transaction")
