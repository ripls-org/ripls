package portfolio

// Shared fixture builders for the portfolio tests. These lived in
// inbox_feed_test.go until #2830 deleted that RPC; the home-view tests still
// use them.

import (
	"go.ripls.org/ripls/server/gen/ripls/models"
)

const (
	selfID  = "user-self"
	otherID = "user-other"
)

// makeLoanTransfer returns a minimal loan Transfer.
func makeLoanTransfer(id, ownerID, recipientID, gearID string, state models.TransferState, latestRequestSec int64) *models.Transfer {
	return &models.Transfer{
		Id:                   id,
		OwnerId:              ownerID,
		RecipientId:          recipientID,
		GearId:               gearID,
		TransferType:         models.TransferType_TRANSFER_TYPE_LOAN,
		State:                state,
		LatestRequestUnixSec: latestRequestSec,
	}
}

// makeGiveawayTransfer returns a minimal giveaway Transfer.
func makeGiveawayTransfer(id, ownerID, recipientID, gearID string, state models.TransferState) *models.Transfer {
	return &models.Transfer{
		Id:           id,
		OwnerId:      ownerID,
		RecipientId:  recipientID,
		GearId:       gearID,
		TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		State:        state,
	}
}

// makeGear returns a minimal Gear proto message.
func makeGear(id, ownerID, name string) *models.Gear {
	return &models.Gear{
		Id:      id,
		OwnerId: ownerID,
		Name:    name,
	}
}

// makeExperience returns a minimal Experience.
func makeExperience(id, ownerID string, state models.ExperienceState) *models.Experience {
	return &models.Experience{
		Id:      id,
		OwnerId: ownerID,
		State:   state,
	}
}

// makeRequest returns a minimal Request.
func makeRequest(id, requesterID string, state models.RequestState) *models.Request {
	return &models.Request{
		Id:          id,
		RequesterId: requesterID,
		State:       state,
	}
}

// makeExperienceWithTime returns a minimal Experience with a specific scheduled time.
func makeExperienceWithTime(id, ownerID string, state models.ExperienceState, ts int64) *models.Experience {
	exp := makeExperience(id, ownerID, state)
	exp.Time = &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Specific{
			Specific: &models.SpecificTime{UnixTimestampSec: ts},
		},
	}
	return exp
}

// makeUserMessage returns a ChatMessage from a real user.
func makeUserMessage(id string, sentAt int64, senderID, text string, readBy map[string]bool) *models.ChatMessage {
	return &models.ChatMessage{
		Id:            id,
		SentAtUnixSec: sentAt,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: senderID,
				Text:     text,
			},
		},
		ParticipantIdToIsRead: readBy,
	}
}
