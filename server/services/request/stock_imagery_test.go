package request

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/chat"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/notifications"
	commsub "go.ripls.org/ripls/server/notifications/community_subscriber"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/storage"
)

// setupTestServiceForAsyncTesting creates a request service with notification and stock imagery signals for testing.
func setupTestServiceForAsyncTesting(t *testing.T) (*Service, *storage.ProtoSQLStorage, chan struct{}, chan struct{}) {
	testStorage := setupTestStorage(t)

	// Create local bucket storage for testing
	tmpDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tmpDir+"/bucket", "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create bucket storage: %v", err)
	}

	mockNotif := notifications.NewMockService()
	fakeProvider := media.NewFakeProvider(testStorage, bucket)
	mockAIProvider := ai.NewMockProvider()
	systemMessageWriter := chat.NewSystemMessageWriter(testStorage, nil)

	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(testStorage, topic)
	notifSub := commsub.New(testStorage, mockNotif)
	if _, err := bus.Subscribe(notifSub); err != nil {
		t.Fatalf("subscribe notification subscriber: %v", err)
	}
	service, notifDone, stockImageryDone := NewForTesting(testStorage, bucket, mockNotif, bus, fakeProvider, mockAIProvider, systemMessageWriter)
	tap := &doneTapSubscriber{ch: notifDone}
	if _, err := bus.Subscribe(tap); err != nil {
		t.Fatalf("subscribe done-tap subscriber: %v", err)
	}
	return service, testStorage, notifDone, stockImageryDone
}

func TestSubmitRequest_WithStockImageFetching(t *testing.T) {
	service, testStorage, _, stockImageryDone := setupTestServiceForAsyncTesting(t)

	// Setup test users and community
	requesterID := setupTestUser(t, testStorage, "Test Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, requesterID)

	// Create request without media_id (should trigger stock image fetch)
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	submitReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need camping gear for weekend trip",
	})

	submitResp, err := service.SubmitRequest(ctx, submitReq)
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}

	requestID := submitResp.Msg.RequestId
	t.Logf("Request created: %s", requestID)
	shareRequestInto(t, ctx, service, submitResp.Msg.RequestId, communityID)

	// Wait for stock image fetch to complete
	select {
	case <-stockImageryDone:
		t.Log("Stock image fetch completed")
	case <-context.Background().Done():
		t.Fatal("Timeout waiting for stock image fetch to complete")
	}

	// Verify the request now has a media_id
	request := &models.Request{}
	err = testStorage.GetByID(context.Background(), requestID, request)
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	if len(request.MediaIds) == 0 {
		t.Error("Expected media_ids to be populated after stock image fetch, but it was empty")
	} else {
		t.Logf("Request has media_ids: %v", request.MediaIds)
	}

	// Verify the media was actually stored
	mediaRecord := &models.Media{}
	err = testStorage.GetByID(context.Background(), request.MediaIds[0], mediaRecord)
	if err != nil {
		t.Fatalf("Failed to get media: %v", err)
	}

	if mediaRecord.ContentType != "image/jpeg" {
		t.Errorf("Expected content type 'image/jpeg', got %q", mediaRecord.ContentType)
	}

	expectedFilename := fmt.Sprintf("request-%s.jpg", requestID)
	if mediaRecord.GetFilename() != expectedFilename {
		t.Errorf("Expected filename %q, got %q", expectedFilename, mediaRecord.GetFilename())
	}

	// Verify the media copy has source_stock_image_id set for provenance tracking
	if mediaRecord.GetSourceStockImageId() == "" {
		t.Errorf("Expected request media to have source_stock_image_id set for provenance tracking")
	}

	// Verify source_stock_image_id references a valid StockImage record
	stockImage := &models.StockImage{}
	err = testStorage.GetByID(context.Background(), mediaRecord.GetSourceStockImageId(), stockImage)
	if err != nil {
		t.Fatalf("Failed to get referenced stock image: %v", err)
	}

	if stockImage.MediaId == "" {
		t.Errorf("Expected stock image to have media_id set")
	}
}

func TestSubmitRequest_WithProvidedMediaId(t *testing.T) {
	service, testStorage, _, stockImageryDone := setupTestServiceForAsyncTesting(t)

	// Setup test users and community
	requesterID := setupTestUser(t, testStorage, "Test Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, requesterID)

	// Create request WITH media_ids (should NOT trigger stock image fetch)
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	submitReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need camping gear",
		MediaIds:    []string{"existing-media-123"},
	})

	submitResp, err := service.SubmitRequest(ctx, submitReq)
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}

	requestID := submitResp.Msg.RequestId
	shareRequestInto(t, ctx, service, submitResp.Msg.RequestId, communityID)

	// Verify no stock image fetch was triggered by checking the channel is empty
	select {
	case <-stockImageryDone:
		t.Error("Stock image fetch should not have been triggered when media_id was provided")
	default:
		t.Log("Correctly skipped stock image fetch when media_id was provided")
	}

	// Verify the request has the provided media_id
	request := &models.Request{}
	err = testStorage.GetByID(context.Background(), requestID, request)
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	if len(request.MediaIds) != 1 || request.MediaIds[0] != "existing-media-123" {
		t.Errorf("Expected media_ids ['existing-media-123'], got %v", request.MediaIds)
	}
}
