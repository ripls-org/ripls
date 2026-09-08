package conversation

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestIsItemDoneFromMaps(t *testing.T) {
	completedTransfer := &models.Transfer{Id: "t1", State: models.TransferState_TRANSFER_STATE_COMPLETED}
	cancelledTransfer := &models.Transfer{Id: "t2", State: models.TransferState_TRANSFER_STATE_CANCELLED}
	activeTransfer := &models.Transfer{Id: "t3", State: models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED}

	fulfilledRequest := &models.Request{Id: "r1", State: models.RequestState_REQUEST_STATE_FULFILLED}
	cancelledRequest := &models.Request{Id: "r2", State: models.RequestState_REQUEST_STATE_CANCELLED}
	activeRequest := &models.Request{Id: "r3", State: models.RequestState_REQUEST_STATE_ACTIVE}

	transfers := map[string]proto.Message{
		"t1": completedTransfer,
		"t2": cancelledTransfer,
		"t3": activeTransfer,
	}
	requests := map[string]proto.Message{
		"r1": fulfilledRequest,
		"r2": cancelledRequest,
		"r3": activeRequest,
	}

	tests := []struct {
		conv *models.ChatConversation
		name string
		want bool
	}{
		{
			name: "completed transfer is done",
			conv: &models.ChatConversation{Topic: &models.ConversationTopic{
				TopicId: &models.ConversationTopic_TransferId{TransferId: "t1"},
			}},
			want: true,
		},
		{
			name: "cancelled transfer is done",
			conv: &models.ChatConversation{Topic: &models.ConversationTopic{
				TopicId: &models.ConversationTopic_TransferId{TransferId: "t2"},
			}},
			want: true,
		},
		{
			name: "active transfer is not done",
			conv: &models.ChatConversation{Topic: &models.ConversationTopic{
				TopicId: &models.ConversationTopic_TransferId{TransferId: "t3"},
			}},
			want: false,
		},
		{
			name: "missing transfer defaults to not done",
			conv: &models.ChatConversation{Topic: &models.ConversationTopic{
				TopicId: &models.ConversationTopic_TransferId{TransferId: "nonexistent"},
			}},
			want: false,
		},
		{
			name: "fulfilled request is done",
			conv: &models.ChatConversation{Topic: &models.ConversationTopic{
				TopicId: &models.ConversationTopic_RequestId{RequestId: "r1"},
			}},
			want: true,
		},
		{
			name: "cancelled request is done",
			conv: &models.ChatConversation{Topic: &models.ConversationTopic{
				TopicId: &models.ConversationTopic_RequestId{RequestId: "r2"},
			}},
			want: true,
		},
		{
			name: "active request is not done",
			conv: &models.ChatConversation{Topic: &models.ConversationTopic{
				TopicId: &models.ConversationTopic_RequestId{RequestId: "r3"},
			}},
			want: false,
		},
		{
			name: "missing request defaults to not done",
			conv: &models.ChatConversation{Topic: &models.ConversationTopic{
				TopicId: &models.ConversationTopic_RequestId{RequestId: "nonexistent"},
			}},
			want: false,
		},
		{
			name: "gear topic (no terminal state) is not done",
			conv: &models.ChatConversation{Topic: &models.ConversationTopic{
				TopicId: &models.ConversationTopic_GearId{GearId: "g1"},
			}},
			want: false,
		},
		{
			name: "nil topic is not done",
			conv: &models.ChatConversation{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsItemDoneFromMaps(tt.conv, transfers, requests)
			if got != tt.want {
				t.Errorf("IsItemDoneFromMaps() = %v, want %v", got, tt.want)
			}
		})
	}
}
