package storage_test

import (
	"errors"
	"fmt"
	"testing"

	"go.ripls.org/ripls/server/storage"
)

func TestErrRecordNotFoundWrapping(t *testing.T) {
	wrapped := fmt.Errorf("record not found with id abc: %w", storage.ErrRecordNotFound)

	if !errors.Is(wrapped, storage.ErrRecordNotFound) {
		t.Errorf("errors.Is(wrapped, ErrRecordNotFound) = false, want true")
	}
}

func TestErrRecordNotFoundDirect(t *testing.T) {
	if !errors.Is(storage.ErrRecordNotFound, storage.ErrRecordNotFound) {
		t.Errorf("errors.Is(ErrRecordNotFound, ErrRecordNotFound) = false, want true")
	}
}

func TestErrRecordNotFoundNotMatchesOtherErrors(t *testing.T) {
	other := errors.New("some other error")
	if errors.Is(other, storage.ErrRecordNotFound) {
		t.Errorf("errors.Is(other, ErrRecordNotFound) = true, want false")
	}
}
