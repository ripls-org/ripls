package community_subscriber

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

func TestDedupSet_SeenOrAdd(t *testing.T) {
	d := newDedupSet(time.Minute)

	if d.seenOrAdd("k") {
		t.Fatal("first seenOrAdd = true, want false (key not yet present)")
	}
	if !d.seenOrAdd("k") {
		t.Fatal("second seenOrAdd = false, want true (key already reserved)")
	}
	if d.seenOrAdd("other") {
		t.Fatal("distinct key reported as duplicate")
	}
}

func TestDedupSet_Expiry(t *testing.T) {
	d := newDedupSet(time.Minute)
	now := time.Unix(1_700_000_000, 0)
	d.now = func() time.Time { return now }

	if d.seenOrAdd("k") {
		t.Fatal("first add reported duplicate")
	}
	// Within the TTL: still a duplicate.
	now = now.Add(59 * time.Second)
	if !d.seenOrAdd("k") {
		t.Fatal("key within TTL not reported as duplicate")
	}
	// Past the TTL: the entry expired and the key is fresh again.
	now = now.Add(2 * time.Second)
	if d.seenOrAdd("k") {
		t.Fatal("expired key still reported as duplicate")
	}
}

func TestDedupSet_Remove(t *testing.T) {
	d := newDedupSet(time.Minute)
	if d.seenOrAdd("k") {
		t.Fatal("first add reported duplicate")
	}
	d.remove("k")
	if d.seenOrAdd("k") {
		t.Fatal("removed key still reported as duplicate")
	}
}

// TestDedupSet_Concurrent exercises the atomic check-and-reserve under the
// concurrent dispatch the pubsub workers produce: exactly one of N racing
// callers must observe the key as fresh.
func TestDedupSet_Concurrent(t *testing.T) {
	d := newDedupSet(time.Minute)

	const n = 64
	var wg sync.WaitGroup
	var firstClaims int32
	results := make([]bool, n)
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			results[i] = d.seenOrAdd("k")
		}(i)
	}
	wg.Wait()

	for _, dup := range results {
		if !dup {
			firstClaims++
		}
	}
	if firstClaims != 1 {
		t.Errorf("fresh claims = %d, want exactly 1", firstClaims)
	}
}

func TestDedupKey(t *testing.T) {
	ridCtx := logging.WithRequestID(context.Background(), "req-123")

	gearEvent := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		GearId:    "gear-1",
	}
	requestEvent := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED,
		Topic:     &models.CommunityEvent_RequestId{RequestId: "req-entity-1"},
	}

	tests := []struct {
		name  string
		ctx   context.Context
		event *models.CommunityEvent
		want  string
	}{
		{
			name:  "gear event with request id",
			ctx:   ridCtx,
			event: gearEvent,
			want:  "req-123|COMMUNITY_EVENT_TYPE_GEAR_SHARED|gear:gear-1|user-1",
		},
		{
			name:  "request entity event",
			ctx:   ridCtx,
			event: requestEvent,
			want:  "req-123|COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED|request:req-entity-1|user-1",
		},
		{
			name:  "no request id in context -> no dedup",
			ctx:   context.Background(),
			event: gearEvent,
			want:  "",
		},
		{
			name:  "no item id -> no dedup",
			ctx:   ridCtx,
			event: &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dedupKey(tt.ctx, tt.event, "user-1"); got != tt.want {
				t.Errorf("dedupKey = %q, want %q", got, tt.want)
			}
		})
	}
}
