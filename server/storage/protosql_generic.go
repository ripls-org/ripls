// Generic type-safe wrappers for proto-SQL storage operations.
// These free functions eliminate the need for callers to type-assert
// proto.Message results from the underlying storage API.

package storage

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// ProtoMessage constrains T to a proto.Message pointer type.
// Usage: GetByIDs[*models.Gear](...) where *models.Gear implements proto.Message.
type ProtoMessage[E any] interface {
	*E
	proto.Message
}

// GetByIDs retrieves multiple proto messages by their IDs, returning a typed map.
// Missing IDs are silently omitted. Wraps ProtoSQLStorage.GetByIDs.
func GetByIDs[T ProtoMessage[E], E any](s *ProtoSQLStorage, ctx context.Context, ids []string, opts ...QueryOptions) (map[string]T, error) {
	var zero E
	raw, err := s.GetByIDs(ctx, ids, T(&zero), opts...)
	if err != nil {
		return nil, err
	}
	result := make(map[string]T, len(raw))
	for id, msg := range raw {
		result[id] = msg.(T)
	}
	return result, nil
}

// QueryByField retrieves proto messages by a field value, returning a typed slice.
// Wraps ProtoSQLStorage.QueryByField.
func QueryByField[T ProtoMessage[E], E any](s *ProtoSQLStorage, ctx context.Context, fieldName string, value any, opts ...QueryOptions) ([]T, error) {
	var zero E
	raw, err := s.QueryByField(ctx, fieldName, value, T(&zero), opts...)
	if err != nil {
		return nil, err
	}
	result := make([]T, len(raw))
	for i, msg := range raw {
		result[i] = msg.(T)
	}
	return result, nil
}

// QueryByFields retrieves proto messages matching multiple field values, returning a typed slice.
// Wraps ProtoSQLStorage.QueryByFields.
func QueryByFields[T ProtoMessage[E], E any](s *ProtoSQLStorage, ctx context.Context, fieldValues map[string]any, opts ...QueryOptions) ([]T, error) {
	var zero E
	raw, err := s.QueryByFields(ctx, fieldValues, T(&zero), opts...)
	if err != nil {
		return nil, err
	}
	result := make([]T, len(raw))
	for i, msg := range raw {
		result[i] = msg.(T)
	}
	return result, nil
}

// QueryByFieldIn retrieves proto messages where a field matches any of the provided values.
// Wraps ProtoSQLStorage.QueryByFieldIn.
func QueryByFieldIn[T ProtoMessage[E], E any](s *ProtoSQLStorage, ctx context.Context, fieldName string, values []string, opts ...QueryOptions) ([]T, error) {
	var zero E
	raw, err := s.QueryByFieldIn(ctx, fieldName, values, T(&zero), opts...)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	result := make([]T, len(raw))
	for i, msg := range raw {
		result[i] = msg.(T)
	}
	return result, nil
}

// CollectField extracts a string field from each element in a slice.
func CollectField[T any](items []T, extract func(T) string) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = extract(item)
	}
	return result
}

// ToMap converts a slice to a map keyed by a field extracted from each element.
// If multiple elements share the same key, the last one wins.
func ToMap[T any](items []T, key func(T) string) map[string]T {
	result := make(map[string]T, len(items))
	for _, item := range items {
		result[key(item)] = item
	}
	return result
}
