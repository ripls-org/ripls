// Unit tests for StreamGenCommunity.

package community

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

type captureCommunityTarget struct {
	mu     sync.Mutex
	events []*api.StreamGenCommunityResponse
}

func (c *captureCommunityTarget) Send(ev *api.StreamGenCommunityResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
	return nil
}

func (c *captureCommunityTarget) snapshot() []*api.StreamGenCommunityResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*api.StreamGenCommunityResponse, len(c.events))
	copy(out, c.events)
	return out
}

func scriptedCommunityStream(events []ai.FieldEvent, final ai.CommunityStreamFinal) func(context.Context, string, string) (<-chan ai.FieldEvent, <-chan ai.CommunityStreamFinal, error) {
	return func(_ context.Context, _, _ string) (<-chan ai.FieldEvent, <-chan ai.CommunityStreamFinal, error) {
		fields := make(chan ai.FieldEvent, len(events))
		finalCh := make(chan ai.CommunityStreamFinal, 1)
		go func() {
			for _, ev := range events {
				fields <- ev
			}
			close(fields)
			finalCh <- final
			close(finalCh)
		}()
		return fields, finalCh, nil
	}
}

func communityRawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestStreamGenCommunity_NoStockProvider verifies that with no stock imagery
// provider configured the stream emits only a terminal `final` event with no
// media — and never a (removed) title/description or a media_ready event.
func TestStreamGenCommunity_NoStockProvider(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(t, sqlStorage)

	mock := ai.NewMockProvider()
	mock.GenerateCommunityContentStreamingFunc = scriptedCommunityStream(
		[]ai.FieldEvent{
			{Key: ai.StreamFieldSearchKeywords, Value: communityRawJSON(t, []string{"cycling", "boulder"})},
		},
		ai.CommunityStreamFinal{Result: &ai.CommunityGeneration{
			SearchKeywords: []string{"cycling", "boulder"},
		}},
	)
	service.aiProvider = mock

	target := &captureCommunityTarget{}
	sender := newCommunityEventSender(target)

	ctx := context.Background()
	logger := logging.LoggerWithContext(ctx)
	msg := &api.StreamGenCommunityRequest{Prompt: "boulder cycling group"}
	if err := service.streamGenCommunityCore(ctx, logger, time.Now(), msg, sender); err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 (final) event with no stock provider; got %d", len(events))
	}
	final, ok := events[0].Event.(*api.StreamGenCommunityResponse_Final)
	if !ok {
		t.Fatalf("event = %T, want Final", events[0].Event)
	}
	if len(final.Final.MediaIds) != 0 {
		t.Errorf("final.MediaIds = %v, want empty", final.Final.MediaIds)
	}
}

// TestStreamGenCommunity_WithStockProvider verifies that the search_keywords
// fan-out fires a media_ready event and the terminal final carries the media.
func TestStreamGenCommunity_WithStockProvider(t *testing.T) {
	sqlStorage := setupTestStorage(t)

	tmpDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tmpDir+"/bucket", "http://localhost:8080")
	require.NoError(t, err)

	notifService := notifications.NewMockService()
	bus := newTestBus(t, sqlStorage, notifService)
	service := New(sqlStorage, bucket, notifService, bus, "test.example.com")

	mock := ai.NewMockProvider()
	mock.GenerateCommunityContentStreamingFunc = scriptedCommunityStream(
		[]ai.FieldEvent{
			{Key: ai.StreamFieldSearchKeywords, Value: communityRawJSON(t, []string{"photography"})},
		},
		ai.CommunityStreamFinal{Result: &ai.CommunityGeneration{
			SearchKeywords: []string{"photography"},
		}},
	)
	service.SetAIProvider(mock)
	service.SetStockImageryProvider(media.NewFakeProvider(sqlStorage, bucket))

	target := &captureCommunityTarget{}
	sender := newCommunityEventSender(target)

	ctx := context.Background()
	logger := logging.LoggerWithContext(ctx)
	msg := &api.StreamGenCommunityRequest{Prompt: "a photography community"}
	if err := service.streamGenCommunityCore(ctx, logger, time.Now(), msg, sender); err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	var sawMedia, sawFinal bool
	for _, ev := range events {
		switch e := ev.Event.(type) {
		case *api.StreamGenCommunityResponse_MediaReady:
			sawMedia = true
			if len(e.MediaReady.MediaIds) != 1 {
				t.Errorf("media_ready.MediaIds = %v, want 1 id", e.MediaReady.MediaIds)
			}
		case *api.StreamGenCommunityResponse_Final:
			sawFinal = true
			if len(e.Final.MediaIds) != 1 {
				t.Errorf("final.MediaIds = %v, want 1 id", e.Final.MediaIds)
			}
		case *api.StreamGenCommunityResponse_Error:
			t.Errorf("unexpected error event: %v", e.Error)
		}
	}
	if !sawMedia || !sawFinal {
		t.Errorf("event coverage: media=%v final=%v", sawMedia, sawFinal)
	}
}

// TestStreamGenCommunity_EmptyPromptError surfaces the invalid-argument error
// path through the streaming response when the prompt is empty.
func TestStreamGenCommunity_EmptyPromptError(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(t, sqlStorage)
	target := &captureCommunityTarget{}
	sender := newCommunityEventSender(target)

	ctx := context.Background()
	logger := logging.LoggerWithContext(ctx)
	msg := &api.StreamGenCommunityRequest{Prompt: ""}
	if err := service.streamGenCommunityCore(ctx, logger, time.Now(), msg, sender); err != nil {
		t.Fatalf("stream returned go error; expected error event instead: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 error event; got %d", len(events))
	}
	errEv, ok := events[0].Event.(*api.StreamGenCommunityResponse_Error)
	if !ok {
		t.Fatalf("event = %T, want Error", events[0].Event)
	}
	if errEv.Error.Code != api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT {
		t.Errorf("error code = %v, want INVALID_ARGUMENT", errEv.Error.Code)
	}
}

// TestStreamGenCommunity_AIProviderError surfaces an AI-provider failure as an
// `error` event with AI_PROVIDER_FAILED code, not a Go error.
func TestStreamGenCommunity_AIProviderError(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(t, sqlStorage)

	mock := ai.NewMockProvider()
	mock.GenerateCommunityContentStreamingFunc = func(_ context.Context, _, _ string) (<-chan ai.FieldEvent, <-chan ai.CommunityStreamFinal, error) {
		fields := make(chan ai.FieldEvent)
		close(fields)
		finalCh := make(chan ai.CommunityStreamFinal, 1)
		finalCh <- ai.CommunityStreamFinal{Err: errors.New("upstream model down")}
		close(finalCh)
		return fields, finalCh, nil
	}
	service.aiProvider = mock

	target := &captureCommunityTarget{}
	sender := newCommunityEventSender(target)

	ctx := context.Background()
	logger := logging.LoggerWithContext(ctx)
	msg := &api.StreamGenCommunityRequest{Prompt: "anything"}
	if err := service.streamGenCommunityCore(ctx, logger, time.Now(), msg, sender); err != nil {
		t.Fatalf("stream returned go error; expected error event instead: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 error event; got %d", len(events))
	}
	errEv, ok := events[0].Event.(*api.StreamGenCommunityResponse_Error)
	if !ok {
		t.Fatalf("event = %T, want Error", events[0].Event)
	}
	if errEv.Error.Code != api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED {
		t.Errorf("error code = %v, want AI_PROVIDER_FAILED", errEv.Error.Code)
	}
}
