package chat

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestAppendMediaToParentEntity(t *testing.T) {
	svc, testStorage := setupTestChatService(t)
	ctx := context.Background()

	t.Run("transfer conversation appends to gear", func(t *testing.T) {
		// Create gear with initial media
		gear := &models.Gear{
			OwnerId:  "user123",
			Name:     "Test Gear",
			MediaIds: []string{"media1", "media2"},
		}
		gearID, err := testStorage.Insert(ctx, gear)
		if err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		// Create transfer for gear
		transfer := &models.Transfer{
			GearId:       gearID,
			OwnerId:      "user123",
			RecipientId:  "user456",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_ACTIVE,
		}
		transferID, err := testStorage.Insert(ctx, transfer)
		if err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}

		// Create conversation for transfer
		conversation := &models.ChatConversation{
			ParticipantIds: []string{"user123", "user456"},
			Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{
				TransferId: transferID,
			}},
		}

		// Append media
		newMediaIds := []string{"media3", "media4"}
		err = svc.appendMediaToParentEntity(ctx, conversation, newMediaIds)
		if err != nil {
			t.Fatalf("appendMediaToParentEntity failed: %v", err)
		}

		// Verify gear now has all media
		updatedGear := &models.Gear{}
		err = testStorage.GetByID(ctx, gearID, updatedGear)
		if err != nil {
			t.Fatalf("Failed to get updated gear: %v", err)
		}

		expectedMedia := []string{"media1", "media2", "media3", "media4"}
		if len(updatedGear.MediaIds) != len(expectedMedia) {
			t.Errorf("Expected %d media IDs, got %d", len(expectedMedia), len(updatedGear.MediaIds))
		}

		for i, expected := range expectedMedia {
			if updatedGear.MediaIds[i] != expected {
				t.Errorf("Media ID at index %d: expected %s, got %s", i, expected, updatedGear.MediaIds[i])
			}
		}
	})

	t.Run("request conversation appends to request", func(t *testing.T) {
		// Create request with initial media
		request := &models.Request{
			RequesterId: "user123",
			Title:       "Test Request",
			Description: "Looking for something",
			MediaIds:    []string{"initial-media"},
			State:       models.RequestState_REQUEST_STATE_ACTIVE,
		}
		requestID, err := testStorage.Insert(ctx, request)
		if err != nil {
			t.Fatalf("Failed to insert request: %v", err)
		}

		// Create conversation for request
		conversation := &models.ChatConversation{
			ParticipantIds: []string{"user123", "user456"},
			Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_RequestId{
				RequestId: requestID,
			}},
		}

		// Append media from chat
		newMediaIds := []string{"media2", "media3"}
		err = svc.appendMediaToParentEntity(ctx, conversation, newMediaIds)
		if err != nil {
			t.Fatalf("appendMediaToParentEntity failed: %v", err)
		}

		// Verify request now has all media
		updatedRequest := &models.Request{}
		err = testStorage.GetByID(ctx, requestID, updatedRequest)
		if err != nil {
			t.Fatalf("Failed to get updated request: %v", err)
		}

		expectedMedia := []string{"initial-media", "media2", "media3"}
		if len(updatedRequest.MediaIds) != len(expectedMedia) {
			t.Fatalf("Expected %d media items, got %d", len(expectedMedia), len(updatedRequest.MediaIds))
		}

		for i, expected := range expectedMedia {
			if updatedRequest.MediaIds[i] != expected {
				t.Errorf("Media ID at index %d: expected %s, got %s", i, expected, updatedRequest.MediaIds[i])
			}
		}
	})

	t.Run("experience conversation appends to experience", func(t *testing.T) {
		// Create experience with initial media
		experience := &models.Experience{
			OwnerId:  "user123",
			Name:     "Test Experience",
			MediaIds: []string{"media1"},
			State:    models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
		}
		experienceID, err := testStorage.Insert(ctx, experience)
		if err != nil {
			t.Fatalf("Failed to insert experience: %v", err)
		}

		// Create conversation for experience
		conversation := &models.ChatConversation{
			ParticipantIds: []string{"user123", "user456"},
			Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_ExperienceId{
				ExperienceId: experienceID,
			}},
		}

		// Append media
		newMediaIds := []string{"media2"}
		err = svc.appendMediaToParentEntity(ctx, conversation, newMediaIds)
		if err != nil {
			t.Fatalf("appendMediaToParentEntity failed: %v", err)
		}

		// Verify experience now has all media
		updatedExperience := &models.Experience{}
		err = testStorage.GetByID(ctx, experienceID, updatedExperience)
		if err != nil {
			t.Fatalf("Failed to get updated experience: %v", err)
		}

		expectedMedia := []string{"media1", "media2"}
		if len(updatedExperience.MediaIds) != len(expectedMedia) {
			t.Errorf("Expected %d media IDs, got %d", len(expectedMedia), len(updatedExperience.MediaIds))
		}

		for i, expected := range expectedMedia {
			if updatedExperience.MediaIds[i] != expected {
				t.Errorf("Media ID at index %d: expected %s, got %s", i, expected, updatedExperience.MediaIds[i])
			}
		}
	})

	t.Run("duplicate media IDs only added once", func(t *testing.T) {
		// Create gear with existing media
		gear := &models.Gear{
			OwnerId:  "user123",
			Name:     "Test Gear",
			MediaIds: []string{"media1", "media2"},
		}
		gearID, err := testStorage.Insert(ctx, gear)
		if err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		// Create transfer
		transfer := &models.Transfer{
			GearId:       gearID,
			OwnerId:      "user123",
			RecipientId:  "user456",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_ACTIVE,
		}
		transferID, err := testStorage.Insert(ctx, transfer)
		if err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}

		conversation := &models.ChatConversation{
			ParticipantIds: []string{"user123", "user456"},
			Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{
				TransferId: transferID,
			}},
		}

		// Try to append media that already exists
		duplicateMediaIds := []string{"media2", "media3"}
		err = svc.appendMediaToParentEntity(ctx, conversation, duplicateMediaIds)
		if err != nil {
			t.Fatalf("appendMediaToParentEntity failed: %v", err)
		}

		// Verify: media2 not duplicated, media3 added
		updatedGear := &models.Gear{}
		err = testStorage.GetByID(ctx, gearID, updatedGear)
		if err != nil {
			t.Fatalf("Failed to get updated gear: %v", err)
		}

		expectedMedia := []string{"media1", "media2", "media3"}
		if len(updatedGear.MediaIds) != len(expectedMedia) {
			t.Errorf("Expected %d media IDs (no duplicates), got %d", len(expectedMedia), len(updatedGear.MediaIds))
		}

		// Verify order and content
		for i, expected := range expectedMedia {
			if updatedGear.MediaIds[i] != expected {
				t.Errorf("Media ID at index %d: expected %s, got %s", i, expected, updatedGear.MediaIds[i])
			}
		}
	})

	t.Run("direct message conversation skips append", func(t *testing.T) {
		// Direct message conversation (no parent entity)
		conversation := &models.ChatConversation{
			ParticipantIds: []string{"user123", "user456"},
			// No topic - direct message
		}

		// Should not error
		err := svc.appendMediaToParentEntity(ctx, conversation, []string{"media1"})
		if err != nil {
			t.Errorf("Expected no error for direct message conversation, got: %v", err)
		}
	})

	t.Run("multiple images in single message", func(t *testing.T) {
		// Create gear
		gear := &models.Gear{
			OwnerId:  "user123",
			Name:     "Test Gear",
			MediaIds: []string{},
		}
		gearID, err := testStorage.Insert(ctx, gear)
		if err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		// Create transfer
		transfer := &models.Transfer{
			GearId:       gearID,
			OwnerId:      "user123",
			RecipientId:  "user456",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_ACTIVE,
		}
		transferID, err := testStorage.Insert(ctx, transfer)
		if err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}

		conversation := &models.ChatConversation{
			ParticipantIds: []string{"user123", "user456"},
			Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{
				TransferId: transferID,
			}},
		}

		// Append multiple media at once
		multipleMediaIds := []string{"media1", "media2", "media3", "media4"}
		err = svc.appendMediaToParentEntity(ctx, conversation, multipleMediaIds)
		if err != nil {
			t.Fatalf("appendMediaToParentEntity failed: %v", err)
		}

		// Verify all media added
		updatedGear := &models.Gear{}
		err = testStorage.GetByID(ctx, gearID, updatedGear)
		if err != nil {
			t.Fatalf("Failed to get updated gear: %v", err)
		}

		if len(updatedGear.MediaIds) != 4 {
			t.Errorf("Expected 4 media IDs, got %d", len(updatedGear.MediaIds))
		}
	})

	t.Run("transfer with non-existent gear logs error", func(t *testing.T) {
		// Create transfer with non-existent gear
		transfer := &models.Transfer{
			GearId:       "nonexistent-gear",
			OwnerId:      "user123",
			RecipientId:  "user456",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_ACTIVE,
		}
		transferID, err := testStorage.Insert(ctx, transfer)
		if err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}

		conversation := &models.ChatConversation{
			ParticipantIds: []string{"user123", "user456"},
			Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{
				TransferId: transferID,
			}},
		}

		// Should return error (gear not found)
		err = svc.appendMediaToParentEntity(ctx, conversation, []string{"media1"})
		if err == nil {
			t.Error("Expected error when gear not found")
		}
	})

	t.Run("empty media list is no-op", func(t *testing.T) {
		// Create gear
		gear := &models.Gear{
			OwnerId:  "user123",
			Name:     "Test Gear",
			MediaIds: []string{"media1"},
		}
		gearID, err := testStorage.Insert(ctx, gear)
		if err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		// Create transfer
		transfer := &models.Transfer{
			GearId:       gearID,
			OwnerId:      "user123",
			RecipientId:  "user456",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_ACTIVE,
		}
		transferID, err := testStorage.Insert(ctx, transfer)
		if err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}

		conversation := &models.ChatConversation{
			ParticipantIds: []string{"user123", "user456"},
			Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{
				TransferId: transferID,
			}},
		}

		// Append empty list
		err = svc.appendMediaToParentEntity(ctx, conversation, []string{})
		if err != nil {
			t.Fatalf("appendMediaToParentEntity failed: %v", err)
		}

		// Verify no changes
		updatedGear := &models.Gear{}
		err = testStorage.GetByID(ctx, gearID, updatedGear)
		if err != nil {
			t.Fatalf("Failed to get updated gear: %v", err)
		}

		if len(updatedGear.MediaIds) != 1 || updatedGear.MediaIds[0] != "media1" {
			t.Errorf("Expected original media unchanged, got %v", updatedGear.MediaIds)
		}
	})
}

func TestAppendUniqueMediaIds(t *testing.T) {
	t.Run("appends updated IDs to existing list", func(t *testing.T) {
		existing := []string{"a", "b", "c"}
		updated := []string{"d", "e"}
		result := appendUniqueMediaIds(existing, updated)

		expected := []string{"a", "b", "c", "d", "e"}
		if len(result) != len(expected) {
			t.Errorf("Expected %d items, got %d", len(expected), len(result))
		}

		for i, exp := range expected {
			if result[i] != exp {
				t.Errorf("At index %d: expected %s, got %s", i, exp, result[i])
			}
		}
	})

	t.Run("skips duplicate IDs", func(t *testing.T) {
		existing := []string{"a", "b", "c"}
		updated := []string{"b", "d", "c", "e"}
		result := appendUniqueMediaIds(existing, updated)

		expected := []string{"a", "b", "c", "d", "e"}
		if len(result) != len(expected) {
			t.Errorf("Expected %d items (no duplicates), got %d", len(expected), len(result))
		}

		for i, exp := range expected {
			if result[i] != exp {
				t.Errorf("At index %d: expected %s, got %s", i, exp, result[i])
			}
		}
	})

	t.Run("handles empty existing list", func(t *testing.T) {
		existing := []string{}
		updated := []string{"a", "b"}
		result := appendUniqueMediaIds(existing, updated)

		if len(result) != 2 {
			t.Errorf("Expected 2 items, got %d", len(result))
		}
	})

	t.Run("handles empty updated list", func(t *testing.T) {
		existing := []string{"a", "b"}
		updated := []string{}
		result := appendUniqueMediaIds(existing, updated)

		if len(result) != 2 {
			t.Errorf("Expected 2 items unchanged, got %d", len(result))
		}
	})

	t.Run("handles duplicates within updated list", func(t *testing.T) {
		existing := []string{"a"}
		updated := []string{"b", "b", "c", "c"}
		result := appendUniqueMediaIds(existing, updated)

		expected := []string{"a", "b", "c"}
		if len(result) != len(expected) {
			t.Errorf("Expected %d items (deduped), got %d", len(expected), len(result))
		}
	})
}
