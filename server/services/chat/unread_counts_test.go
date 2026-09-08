package chat

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestGetUnreadCounts(t *testing.T) {
	tests := []struct {
		name                   string
		setupFn                func(t *testing.T, s *Service, userID string)
		userID                 string
		wantTotalCount         int32
		wantCommunityCounts    map[string]int32
		wantConversationCounts map[string]int32
		wantErr                bool
		wantErrCode            connect.Code
	}{
		{
			name:   "no conversations",
			userID: "user1",
			setupFn: func(t *testing.T, s *Service, userID string) {
				// No setup needed - no conversations
			},
			wantTotalCount:         0,
			wantCommunityCounts:    map[string]int32{},
			wantConversationCounts: map[string]int32{},
		},
		{
			name:   "single community with unread messages",
			userID: "user1",
			setupFn: func(t *testing.T, s *Service, userID string) {
				// Create community and users
				createTestUsers(t, s.storage, "user1", "User 1")
				createTestUsers(t, s.storage, "user2", "User 2")
				communityID := createTestCommunity(t, s.storage, "user1")
				createTestCommunityMembership(t, s.storage, communityID, "user1")
				createTestCommunityMembership(t, s.storage, communityID, "user2")

				// Create gear item
				gearID, err := s.storage.Insert(context.Background(), &models.Gear{
					OwnerId: "user1",
					Name:    "Test Gear",
				})
				if err != nil {
					t.Fatalf("failed to create gear: %v", err)
				}

				// Create conversation
				conversation := &models.ChatConversation{
					CommunityId:    communityID,
					ParticipantIds: []string{"user1", "user2"},
					Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
				}
				convID, err := s.chatConvStorage.Insert(context.Background(), conversation)
				if err != nil {
					t.Fatalf("failed to create conversation: %v", err)
				}

				// Create 3 unread messages from user2 to user1
				for i := 0; i < 3; i++ {
					msg := &models.ChatMessage{
						ConversationId: convID,
						Message: &models.ChatMessage_UserMessage{
							UserMessage: &models.UserChatMessage{
								SenderId: "user2",
								Text:     "Hello",
							},
						},
						ParticipantIdToIsRead: map[string]bool{
							"user1": false, // Unread
							"user2": true,
						},
					}
					_, err := s.storage.Insert(context.Background(), msg)
					if err != nil {
						t.Fatalf("failed to create message: %v", err)
					}
				}
			},
			wantTotalCount:      3,
			wantCommunityCounts: map[string]int32{
				// communityID will be set dynamically in test
			},
			wantConversationCounts: map[string]int32{
				// conversationID will be set dynamically in test
			},
		},
		{
			name:   "multiple communities with different unread counts",
			userID: "user1",
			setupFn: func(t *testing.T, s *Service, userID string) {
				// Create users
				createTestUsers(t, s.storage, "user1", "User 1")
				createTestUsers(t, s.storage, "user2", "User 2")

				// Create two communities
				communityA := createTestCommunity(t, s.storage, "user1")
				communityB := createTestCommunity(t, s.storage, "user1")
				createTestCommunityMembership(t, s.storage, communityA, "user1")
				createTestCommunityMembership(t, s.storage, communityA, "user2")
				createTestCommunityMembership(t, s.storage, communityB, "user1")
				createTestCommunityMembership(t, s.storage, communityB, "user2")

				// Create gear items for each community
				gearA, err := s.storage.Insert(context.Background(), &models.Gear{
					OwnerId: "user1",
					Name:    "Gear A",
				})
				if err != nil {
					t.Fatalf("failed to create gear A: %v", err)
				}

				gearB, err := s.storage.Insert(context.Background(), &models.Gear{
					OwnerId: "user1",
					Name:    "Gear B",
				})
				if err != nil {
					t.Fatalf("failed to create gear B: %v", err)
				}

				// Create conversation in community A with 5 unread messages
				convA := &models.ChatConversation{
					CommunityId:    communityA,
					ParticipantIds: []string{"user1", "user2"},
					Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearA}},
				}
				convAID, err := s.chatConvStorage.Insert(context.Background(), convA)
				if err != nil {
					t.Fatalf("failed to create conversation A: %v", err)
				}

				for i := 0; i < 5; i++ {
					msg := &models.ChatMessage{
						ConversationId: convAID,
						Message: &models.ChatMessage_UserMessage{
							UserMessage: &models.UserChatMessage{
								SenderId: "user2",
								Text:     "Message in A",
							},
						},
						ParticipantIdToIsRead: map[string]bool{
							"user1": false,
							"user2": true,
						},
					}
					_, err := s.storage.Insert(context.Background(), msg)
					if err != nil {
						t.Fatalf("failed to create message in A: %v", err)
					}
				}

				// Create conversation in community B with 3 unread messages
				convB := &models.ChatConversation{
					CommunityId:    communityB,
					ParticipantIds: []string{"user1", "user2"},
					Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearB}},
				}
				convBID, err := s.chatConvStorage.Insert(context.Background(), convB)
				if err != nil {
					t.Fatalf("failed to create conversation B: %v", err)
				}

				for i := 0; i < 3; i++ {
					msg := &models.ChatMessage{
						ConversationId: convBID,
						Message: &models.ChatMessage_UserMessage{
							UserMessage: &models.UserChatMessage{
								SenderId: "user2",
								Text:     "Message in B",
							},
						},
						ParticipantIdToIsRead: map[string]bool{
							"user1": false,
							"user2": true,
						},
					}
					_, err := s.storage.Insert(context.Background(), msg)
					if err != nil {
						t.Fatalf("failed to create message in B: %v", err)
					}
				}
			},
			wantTotalCount:      8, // 5 + 3
			wantCommunityCounts: map[string]int32{
				// Will be populated dynamically in test
			},
			wantConversationCounts: map[string]int32{
				// Will be populated dynamically in test
			},
		},
		{
			name:   "excludes archived conversations",
			userID: "user1",
			setupFn: func(t *testing.T, s *Service, userID string) {
				// Create users and community
				createTestUsers(t, s.storage, "user1", "User 1")
				createTestUsers(t, s.storage, "user2", "User 2")
				communityID := createTestCommunity(t, s.storage, "user1")
				createTestCommunityMembership(t, s.storage, communityID, "user1")
				createTestCommunityMembership(t, s.storage, communityID, "user2")

				// Create an active transfer (not archived)
				activeTransferID := createTestTransferWithState(t, s.storage, "user1", "user2", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED)

				// Create conversation for active transfer
				activeConv := &models.ChatConversation{
					CommunityId:    communityID,
					ParticipantIds: []string{"user1", "user2"},
					Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{TransferId: activeTransferID}},
				}
				activeConvID, err := s.chatConvStorage.Insert(context.Background(), activeConv)
				if err != nil {
					t.Fatalf("failed to create active conversation: %v", err)
				}

				// Add 2 unread messages to active conversation
				for i := 0; i < 2; i++ {
					msg := &models.ChatMessage{
						ConversationId: activeConvID,
						Message: &models.ChatMessage_UserMessage{
							UserMessage: &models.UserChatMessage{
								SenderId: "user2",
								Text:     "Active message",
							},
						},
						ParticipantIdToIsRead: map[string]bool{
							"user1": false,
							"user2": true,
						},
					}
					_, err := s.storage.Insert(context.Background(), msg)
					if err != nil {
						t.Fatalf("failed to create message: %v", err)
					}
				}

				// Create a completed/archived transfer
				archivedTransferID := createTestTransferWithState(t, s.storage, "user1", "user2", models.TransferState_TRANSFER_STATE_COMPLETED)

				// Create conversation for archived transfer
				archivedConv := &models.ChatConversation{
					CommunityId:    communityID,
					ParticipantIds: []string{"user1", "user2"},
					Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{TransferId: archivedTransferID}},
				}
				archivedConvID, err := s.chatConvStorage.Insert(context.Background(), archivedConv)
				if err != nil {
					t.Fatalf("failed to create archived conversation: %v", err)
				}

				// Add unread messages to archived conversation (should be excluded)
				for i := 0; i < 5; i++ {
					msg := &models.ChatMessage{
						ConversationId: archivedConvID,
						Message: &models.ChatMessage_UserMessage{
							UserMessage: &models.UserChatMessage{
								SenderId: "user2",
								Text:     "Archived message",
							},
						},
						ParticipantIdToIsRead: map[string]bool{
							"user1": false, // Unread but should be ignored
							"user2": true,
						},
					}
					_, err := s.storage.Insert(context.Background(), msg)
					if err != nil {
						t.Fatalf("failed to create message: %v", err)
					}
				}
			},
			wantTotalCount:      2, // Only count active conversation, ignore archived
			wantCommunityCounts: map[string]int32{
				// Will be populated dynamically in test
			},
			wantConversationCounts: map[string]int32{
				// Will be populated dynamically in test
			},
		},
		{
			name:   "no unread messages",
			userID: "user1",
			setupFn: func(t *testing.T, s *Service, userID string) {
				// Create community and users
				createTestUsers(t, s.storage, "user1", "User 1")
				createTestUsers(t, s.storage, "user2", "User 2")
				communityID := createTestCommunity(t, s.storage, "user1")
				createTestCommunityMembership(t, s.storage, communityID, "user1")
				createTestCommunityMembership(t, s.storage, communityID, "user2")

				// Create gear and conversation
				gearID, err := s.storage.Insert(context.Background(), &models.Gear{
					OwnerId: "user1",
					Name:    "Test Gear",
				})
				if err != nil {
					t.Fatalf("failed to create gear: %v", err)
				}

				conversation := &models.ChatConversation{
					CommunityId:    communityID,
					ParticipantIds: []string{"user1", "user2"},
					Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
				}
				convID, err := s.chatConvStorage.Insert(context.Background(), conversation)
				if err != nil {
					t.Fatalf("failed to create conversation: %v", err)
				}

				// Create messages that are all read
				msg := &models.ChatMessage{
					ConversationId: convID,
					Message: &models.ChatMessage_UserMessage{
						UserMessage: &models.UserChatMessage{
							SenderId: "user2",
							Text:     "Read message",
						},
					},
					ParticipantIdToIsRead: map[string]bool{
						"user1": true, // Read
						"user2": true,
					},
				}
				_, err = s.storage.Insert(context.Background(), msg)
				if err != nil {
					t.Fatalf("failed to create message: %v", err)
				}
			},
			wantTotalCount:         0,
			wantCommunityCounts:    map[string]int32{},
			wantConversationCounts: map[string]int32{},
		},
		{
			name:   "system messages excluded from unread counts",
			userID: "user1",
			setupFn: func(t *testing.T, s *Service, userID string) {
				createTestUsers(t, s.storage, "user1", "User 1")
				createTestUsers(t, s.storage, "user2", "User 2")
				communityID := createTestCommunity(t, s.storage, "user1")
				createTestCommunityMembership(t, s.storage, communityID, "user1")
				createTestCommunityMembership(t, s.storage, communityID, "user2")

				gearID, err := s.storage.Insert(context.Background(), &models.Gear{
					OwnerId: "user1",
					Name:    "Test Gear",
				})
				if err != nil {
					t.Fatalf("failed to create gear: %v", err)
				}

				conversation := &models.ChatConversation{
					CommunityId:    communityID,
					ParticipantIds: []string{"user1", "user2"},
					Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
				}
				convID, err := s.chatConvStorage.Insert(context.Background(), conversation)
				if err != nil {
					t.Fatalf("failed to create conversation: %v", err)
				}

				// 2 user messages (unread)
				for i := 0; i < 2; i++ {
					_, err := s.storage.Insert(context.Background(), &models.ChatMessage{
						ConversationId: convID,
						Message: &models.ChatMessage_UserMessage{
							UserMessage: &models.UserChatMessage{
								SenderId: "user2",
								Text:     "Hello",
							},
						},
						ParticipantIdToIsRead: map[string]bool{
							"user1": false,
							"user2": true,
						},
					})
					if err != nil {
						t.Fatalf("failed to create user message: %v", err)
					}
				}

				// 3 system messages (unread — but should be excluded)
				actorID := "user2"
				systemActions := []models.ChatSystemAction{
					models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
					models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED,
					models.ChatSystemAction_CHAT_SYSTEM_ACTION_LOAN_SHARED,
				}
				for _, action := range systemActions {
					_, err := s.storage.Insert(context.Background(), &models.ChatMessage{
						ConversationId: convID,
						Message: &models.ChatMessage_SystemMessage{
							SystemMessage: &models.SystemChatMessage{
								ActorId:     &actorID,
								Action:      action,
								Description: "system event",
							},
						},
						ParticipantIdToIsRead: map[string]bool{
							"user1": false,
							"user2": true,
						},
					})
					if err != nil {
						t.Fatalf("failed to create system message: %v", err)
					}
				}
			},
			wantTotalCount:         2, // Only user messages, not the 3 system messages.
			wantCommunityCounts:    map[string]int32{},
			wantConversationCounts: map[string]int32{},
		},
		{
			name:   "unauthenticated request",
			userID: "",
			setupFn: func(t *testing.T, s *Service, userID string) {
				// No setup needed
			},
			wantErr:     true,
			wantErrCode: connect.CodeUnauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := setupTestChatService(t)

			// Run setup
			tt.setupFn(t, svc, tt.userID)

			// Create context with auth if userID provided
			ctx := context.Background()
			if tt.userID != "" {
				ctx = contextWithAuth(tt.userID, tt.userID+"@test.com")
			}

			// Make request
			req := connect.NewRequest(&api.GetUnreadCountsRequest{})
			resp, err := svc.GetUnreadCounts(ctx, req)

			// Check error
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.wantErrCode != 0 {
					if code := connect.CodeOf(err); code != tt.wantErrCode {
						t.Errorf("want error code %v, got %v", tt.wantErrCode, code)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check total count
			if resp.Msg.TotalUnreadCount != tt.wantTotalCount {
				t.Errorf("want total count %d, got %d", tt.wantTotalCount, resp.Msg.TotalUnreadCount)
			}

			// For dynamic community counts tests, just check the total matches expected
			if tt.name == "single community with unread messages" {
				if len(resp.Msg.CommunityIdToUnreadCount) != 1 {
					t.Errorf("expected 1 community, got %d", len(resp.Msg.CommunityIdToUnreadCount))
				}
				// Check that some community has 3 unread
				found := false
				for _, count := range resp.Msg.CommunityIdToUnreadCount {
					if count == 3 {
						found = true
						break
					}
				}
				if !found {
					t.Error("expected to find community with 3 unread messages")
				}
			}

			if tt.name == "multiple communities with different unread counts" {
				if len(resp.Msg.CommunityIdToUnreadCount) != 2 {
					t.Errorf("expected 2 communities, got %d", len(resp.Msg.CommunityIdToUnreadCount))
				}
				// Check counts add up
				totalFromCommunities := int32(0)
				for _, count := range resp.Msg.CommunityIdToUnreadCount {
					totalFromCommunities += count
				}
				if totalFromCommunities != tt.wantTotalCount {
					t.Errorf("community counts sum %d != total count %d", totalFromCommunities, tt.wantTotalCount)
				}
			}

			if tt.name == "excludes archived conversations" {
				// Should only have 1 community and 1 conversation (active one)
				if len(resp.Msg.CommunityIdToUnreadCount) != 1 {
					t.Errorf("expected 1 community, got %d", len(resp.Msg.CommunityIdToUnreadCount))
				}
				if len(resp.Msg.ConversationIdToUnreadCount) != 1 {
					t.Errorf("expected 1 conversation, got %d", len(resp.Msg.ConversationIdToUnreadCount))
				}
			}

			if tt.name == "system messages excluded from unread counts" {
				if len(resp.Msg.CommunityIdToUnreadCount) != 1 {
					t.Errorf("expected 1 community, got %d", len(resp.Msg.CommunityIdToUnreadCount))
				}
				// Verify total is 2 (not 5 which includes system messages)
				found := false
				for _, count := range resp.Msg.ConversationIdToUnreadCount {
					if count == 2 {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected conversation with 2 unread user messages, got %v", resp.Msg.ConversationIdToUnreadCount)
				}
			}

			if tt.name == "no unread messages" {
				if len(resp.Msg.CommunityIdToUnreadCount) != 0 {
					t.Errorf("expected 0 communities with unread, got %d", len(resp.Msg.CommunityIdToUnreadCount))
				}
				if len(resp.Msg.ConversationIdToUnreadCount) != 0 {
					t.Errorf("expected 0 conversations with unread, got %d", len(resp.Msg.ConversationIdToUnreadCount))
				}
			}
		})
	}
}

// TestGetUnreadCounts_NoPlusOneQueries verifies that GetUnreadCounts issues a bounded
// number of queries regardless of conversation count (no N+1 regression).
func TestGetUnreadCounts_NoPlusOneQueries(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "user1", "User One")
	createTestUsers(t, sqlStorage, "user2", "User Two")
	communityID := createTestCommunity(t, sqlStorage, "user1")
	createTestCommunityMembership(t, sqlStorage, communityID, "user1")
	createTestCommunityMembership(t, sqlStorage, communityID, "user2")

	ctxUser1 := contextWithAuth("user1", "user1@test.com")

	// Use the wrapper so that participant_ids is indexed for ListByParticipant.
	chatConvStorage := storage.NewChatConversationStorage(sqlStorage)

	// Create 3 conversations, each with an unread message from user2.
	const numConversations = 3
	for i := 0; i < numConversations; i++ {
		gearID, err := sqlStorage.Insert(context.Background(), &models.Gear{
			OwnerId: "user1",
			Name:    "Gear",
		})
		if err != nil {
			t.Fatalf("failed to create gear: %v", err)
		}

		conv := &models.ChatConversation{
			CommunityId:    communityID,
			ParticipantIds: []string{"user1", "user2"},
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
		}
		convID, err := chatConvStorage.Insert(context.Background(), conv)
		if err != nil {
			t.Fatalf("failed to create conversation: %v", err)
		}

		_, err = sqlStorage.Insert(context.Background(), &models.ChatMessage{
			ConversationId: convID,
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: "user2",
					Text:     "hello",
				},
			},
			ParticipantIdToIsRead: map[string]bool{"user1": false, "user2": true},
		})
		if err != nil {
			t.Fatalf("failed to create message: %v", err)
		}
	}

	// Query budget after the indexed ListByParticipant fix:
	// 1 (ListByParticipant) + 1 (batch messages) + 1 (batch transfers, empty) +
	// 1 (batch requests, empty) = 4. Allow max=5 for incidental overhead.
	ctxWithStats := storage.WithQueryStats(ctxUser1)
	storage.AssertMaxQueries(t, ctxWithStats, 5, func() {
		resp, err := svc.GetUnreadCounts(ctxWithStats, connect.NewRequest(&api.GetUnreadCountsRequest{}))
		if err != nil {
			t.Fatalf("GetUnreadCounts: %v", err)
		}
		if resp.Msg.TotalUnreadCount != numConversations {
			t.Errorf("want total unread %d, got %d", numConversations, resp.Msg.TotalUnreadCount)
		}
	})
}

// TestGetUnreadCounts_RowCountAssertion verifies that GetUnreadCounts only counts
// unread messages in the caller's conversations and does not include other users'
// conversations, regardless of total platform conversation count.
func TestGetUnreadCounts_RowCountAssertion(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "user1", "User One")
	createTestUsers(t, sqlStorage, "user2", "User Two")
	communityID := createTestCommunity(t, sqlStorage, "user1")
	createTestCommunityMembership(t, sqlStorage, communityID, "user1")
	createTestCommunityMembership(t, sqlStorage, communityID, "user2")

	// N extra conversations user1 is NOT in (user2 only), M user1 IS in.
	const N = 20
	const M = 3

	chatConvStorage := storage.NewChatConversationStorage(sqlStorage)

	// Create N conversations for user2 only (user1 not a participant).
	for i := 0; i < N; i++ {
		gearID, err := sqlStorage.Insert(context.Background(), &models.Gear{OwnerId: "user2", Name: "Gear"})
		if err != nil {
			t.Fatalf("failed to create gear: %v", err)
		}
		conv := &models.ChatConversation{
			CommunityId:    communityID,
			ParticipantIds: []string{"user2"},
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
		}
		convID, err := chatConvStorage.Insert(context.Background(), conv)
		if err != nil {
			t.Fatalf("failed to create other-user conversation: %v", err)
		}
		_, err = sqlStorage.Insert(context.Background(), &models.ChatMessage{
			ConversationId: convID,
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{SenderId: "user2", Text: "hi"},
			},
		})
		if err != nil {
			t.Fatalf("failed to create message: %v", err)
		}
	}

	// Create M conversations user1 IS in, each with an unread message from user2.
	for i := 0; i < M; i++ {
		gearID, err := sqlStorage.Insert(context.Background(), &models.Gear{OwnerId: "user1", Name: "Gear"})
		if err != nil {
			t.Fatalf("failed to create gear: %v", err)
		}
		conv := &models.ChatConversation{
			CommunityId:    communityID,
			ParticipantIds: []string{"user1", "user2"},
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
		}
		convID, err := chatConvStorage.Insert(context.Background(), conv)
		if err != nil {
			t.Fatalf("failed to create user1 conversation: %v", err)
		}
		_, err = sqlStorage.Insert(context.Background(), &models.ChatMessage{
			ConversationId: convID,
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{SenderId: "user2", Text: "hello"},
			},
			ParticipantIdToIsRead: map[string]bool{"user1": false, "user2": true},
		})
		if err != nil {
			t.Fatalf("failed to create message: %v", err)
		}
	}

	ctxUser1 := contextWithAuth("user1", "user1@test.com")
	ctxWithStats := storage.WithQueryStats(ctxUser1)
	storage.AssertMaxQueries(t, ctxWithStats, 5, func() {
		resp, err := svc.GetUnreadCounts(ctxWithStats, connect.NewRequest(&api.GetUnreadCountsRequest{}))
		if err != nil {
			t.Fatalf("GetUnreadCounts: %v", err)
		}
		// Only user1's M conversations should be counted — none of the N user2 conversations.
		if resp.Msg.TotalUnreadCount != M {
			t.Errorf("want total unread %d (user1's conversations only), got %d", M, resp.Msg.TotalUnreadCount)
		}
	})
}
