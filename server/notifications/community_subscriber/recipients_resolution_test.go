package community_subscriber

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestGetNotificationRecipients_TransferInterestExpressed(t *testing.T) {
	// When someone expresses interest in gear, only the gear owner should be notified.
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Gear Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, err := testStorage.Insert(context.Background(), owner)
	if err != nil {
		t.Fatalf("Failed to create owner: %v", err)
	}

	gear := &models.Gear{Name: "Mountain Bike", OwnerId: ownerID}
	gearID, err := testStorage.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	interestedUser := &models.User{Name: "Interested User", Email: "interested@example.com", Role: models.Role_ROLE_USER}
	interestedUserID, err := testStorage.Insert(context.Background(), interestedUser)
	if err != nil {
		t.Fatalf("Failed to create interested user: %v", err)
	}

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		ActorId:   interestedUserID,
		GearId:    gearID,
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 1 {
		t.Fatalf("Expected 1 recipient (gear owner), got %d", len(recipients))
	}
	if recipients[0] != ownerID {
		t.Errorf("Expected gear owner %s, got %s", ownerID, recipients[0])
	}
}

func TestGetNotificationRecipients_TransferRecipientSelected(t *testing.T) {
	// When owner approves a recipient, only that recipient should be notified.
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, err := testStorage.Insert(context.Background(), owner)
	if err != nil {
		t.Fatalf("Failed to create owner: %v", err)
	}

	recipient := &models.User{Name: "Approved Recipient", Email: "recipient@example.com", Role: models.Role_ROLE_USER}
	recipientID, err := testStorage.Insert(context.Background(), recipient)
	if err != nil {
		t.Fatalf("Failed to create recipient: %v", err)
	}

	gear := &models.Gear{OwnerId: ownerID, Name: "Test Gear", Description: "A test gear item"}
	gearID, err := testStorage.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	transfer := &models.Transfer{
		GearId:       gearID,
		OwnerId:      ownerID,
		RecipientId:  recipientID,
		TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
	}
	transferID, err := testStorage.Insert(context.Background(), transfer)
	if err != nil {
		t.Fatalf("Failed to create transfer: %v", err)
	}

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED,
		ActorId:   ownerID,
		Topic:     &models.CommunityEvent_TransferId{TransferId: transferID},
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 1 {
		t.Fatalf("Expected 1 recipient (approved user), got %d", len(recipients))
	}
	if recipients[0] != recipientID {
		t.Errorf("Expected approved recipient %s, got %s", recipientID, recipients[0])
	}
}

func TestGetNotificationRecipients_RequestOfferMade(t *testing.T) {
	// When someone offers to help with a request, only the request creator should be notified.
	testStorage := setupTestStorage(t)

	requester := &models.User{Name: "Requester", Email: "requester@example.com", Role: models.Role_ROLE_USER}
	requesterID, err := testStorage.Insert(context.Background(), requester)
	if err != nil {
		t.Fatalf("Failed to create requester: %v", err)
	}

	request := &models.Request{Title: "Need a ladder", RequesterId: requesterID}
	requestID, err := testStorage.Insert(context.Background(), request)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	offerer := &models.User{Name: "Helper", Email: "helper@example.com", Role: models.Role_ROLE_USER}
	offererID, err := testStorage.Insert(context.Background(), offerer)
	if err != nil {
		t.Fatalf("Failed to create offerer: %v", err)
	}

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE,
		ActorId:   offererID,
		Topic:     &models.CommunityEvent_RequestId{RequestId: requestID},
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 1 {
		t.Fatalf("Expected 1 recipient (request creator), got %d", len(recipients))
	}
	if recipients[0] != requesterID {
		t.Errorf("Expected request creator %s, got %s", requesterID, recipients[0])
	}
}

func TestGetNotificationRecipients_RequestFulfilled(t *testing.T) {
	// When a request is fulfilled, all offerers (except the actor) should be notified.
	testStorage := setupTestStorage(t)

	requester := &models.User{Name: "Requester", Email: "requester@example.com", Role: models.Role_ROLE_USER}
	requesterID, err := testStorage.Insert(context.Background(), requester)
	if err != nil {
		t.Fatalf("Failed to create requester: %v", err)
	}

	offerer1 := &models.User{Name: "Offerer1", Email: "offerer1@example.com", Role: models.Role_ROLE_USER}
	offerer1ID, _ := testStorage.Insert(context.Background(), offerer1)

	offerer2 := &models.User{Name: "Offerer2", Email: "offerer2@example.com", Role: models.Role_ROLE_USER}
	offerer2ID, _ := testStorage.Insert(context.Background(), offerer2)

	request := &models.Request{Title: "Need a ladder", RequesterId: requesterID}
	requestID, err := testStorage.Insert(context.Background(), request)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	community := &models.Community{Name: "Test Community", Description: "Test", CreatorId: requesterID, OwnerUserId: requesterID}
	communityID, err := testStorage.Insert(context.Background(), community)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}

	_, err = testStorage.Insert(context.Background(), &models.CommunityRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	if err != nil {
		t.Fatalf("Failed to link request to community: %v", err)
	}

	for _, offererID := range []string{offerer1ID, offerer2ID} {
		offer := &models.RequestOffer{
			RequestId:        requestID,
			UserId:           offererID,
			CommunityId:      communityID,
			CreatedAtUnixSec: 1234567890,
		}
		_, err = testStorage.Insert(context.Background(), offer)
		if err != nil {
			t.Fatalf("Failed to create offer: %v", err)
		}
	}

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED,
		ActorId:   requesterID,
		Topic:     &models.CommunityEvent_RequestId{RequestId: requestID},
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 2 {
		t.Fatalf("Expected 2 recipients (both offerers), got %d", len(recipients))
	}

	recipientSet := make(map[string]bool)
	for _, r := range recipients {
		recipientSet[r] = true
	}
	if !recipientSet[offerer1ID] {
		t.Errorf("Expected offerer1 to be notified")
	}
	if !recipientSet[offerer2ID] {
		t.Errorf("Expected offerer2 to be notified")
	}
	if recipientSet[requesterID] {
		t.Errorf("Requester (actor) should not be notified")
	}
}

func TestGetNotificationRecipients_ExperienceRSVP(t *testing.T) {
	// When someone RSVPs to an experience, only the experience owner should be notified.
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Experience Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, err := testStorage.Insert(context.Background(), owner)
	if err != nil {
		t.Fatalf("Failed to create owner: %v", err)
	}

	rsvpUser := &models.User{Name: "RSVP User", Email: "rsvp@example.com", Role: models.Role_ROLE_USER}
	rsvpUserID, err := testStorage.Insert(context.Background(), rsvpUser)
	if err != nil {
		t.Fatalf("Failed to create RSVP user: %v", err)
	}

	experience := &models.Experience{Name: "Morning Hike", OwnerId: ownerID}
	experienceID, err := testStorage.Insert(context.Background(), experience)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}

	event := &models.CommunityEvent{
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
		ActorId:      rsvpUserID,
		ObjectUserId: ownerID,
		Topic:        &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 1 {
		t.Fatalf("Expected 1 recipient (experience owner), got %d", len(recipients))
	}
	if recipients[0] != ownerID {
		t.Errorf("Expected experience owner %s, got %s", ownerID, recipients[0])
	}
}

func TestGetNotificationRecipients_ExperienceRSVPMaybe(t *testing.T) {
	// When someone RSVPs MAYBE to an experience, only the experience owner should be notified.
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, _ := testStorage.Insert(context.Background(), owner)

	rsvpUser := &models.User{Name: "Maybe User", Email: "maybe@example.com", Role: models.Role_ROLE_USER}
	rsvpUserID, _ := testStorage.Insert(context.Background(), rsvpUser)

	event := &models.CommunityEvent{
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE,
		ActorId:      rsvpUserID,
		ObjectUserId: ownerID,
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 1 {
		t.Fatalf("Expected 1 recipient (experience owner), got %d", len(recipients))
	}
	if recipients[0] != ownerID {
		t.Errorf("Expected experience owner %s, got %s", ownerID, recipients[0])
	}
}

func TestGetNotificationRecipients_TransferCancelled_OwnerCancels(t *testing.T) {
	// When the owner cancels a transfer, the recipient should be notified.
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, _ := testStorage.Insert(context.Background(), owner)

	recipient := &models.User{Name: "Borrower", Email: "borrower@example.com", Role: models.Role_ROLE_USER}
	recipientID, _ := testStorage.Insert(context.Background(), recipient)

	gear := &models.Gear{Name: "Drill", OwnerId: ownerID}
	gearID, _ := testStorage.Insert(context.Background(), gear)

	transfer := &models.Transfer{
		GearId:       gearID,
		OwnerId:      ownerID,
		RecipientId:  recipientID,
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_CANCELLED,
	}
	transferID, _ := testStorage.Insert(context.Background(), transfer)

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
		ActorId:   ownerID,
		GearId:    gearID,
		Topic:     &models.CommunityEvent_TransferId{TransferId: transferID},
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 1 {
		t.Fatalf("Expected 1 recipient (borrower), got %d", len(recipients))
	}
	if recipients[0] != recipientID {
		t.Errorf("Expected borrower %s, got %s", recipientID, recipients[0])
	}
}

func TestGetNotificationRecipients_TransferCancelled_RecipientCancels(t *testing.T) {
	// When the recipient cancels a transfer, the owner should be notified.
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, _ := testStorage.Insert(context.Background(), owner)

	recipient := &models.User{Name: "Borrower", Email: "borrower@example.com", Role: models.Role_ROLE_USER}
	recipientID, _ := testStorage.Insert(context.Background(), recipient)

	gear := &models.Gear{Name: "Drill", OwnerId: ownerID}
	gearID, _ := testStorage.Insert(context.Background(), gear)

	transfer := &models.Transfer{
		GearId:       gearID,
		OwnerId:      ownerID,
		RecipientId:  recipientID,
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_CANCELLED,
	}
	transferID, _ := testStorage.Insert(context.Background(), transfer)

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
		ActorId:   recipientID,
		GearId:    gearID,
		Topic:     &models.CommunityEvent_TransferId{TransferId: transferID},
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 1 {
		t.Fatalf("Expected 1 recipient (owner), got %d", len(recipients))
	}
	if recipients[0] != ownerID {
		t.Errorf("Expected owner %s, got %s", ownerID, recipients[0])
	}
}

func TestGetNotificationRecipients_TransferPickupProposed_OwnerProposes(t *testing.T) {
	// When the owner proposes a pickup time, the recipient should be notified.
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, _ := testStorage.Insert(context.Background(), owner)

	recipient := &models.User{Name: "Recipient", Email: "recipient@example.com", Role: models.Role_ROLE_USER}
	recipientID, _ := testStorage.Insert(context.Background(), recipient)

	gear := &models.Gear{Name: "Drill", OwnerId: ownerID}
	gearID, _ := testStorage.Insert(context.Background(), gear)

	transfer := &models.Transfer{
		GearId:       gearID,
		OwnerId:      ownerID,
		RecipientId:  recipientID,
		TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
	}
	transferID, _ := testStorage.Insert(context.Background(), transfer)

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED,
		ActorId:   ownerID,
		GearId:    gearID,
		Topic:     &models.CommunityEvent_TransferId{TransferId: transferID},
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 1 {
		t.Fatalf("Expected 1 recipient, got %d", len(recipients))
	}
	if recipients[0] != recipientID {
		t.Errorf("Expected recipient %s, got %s", recipientID, recipients[0])
	}
}

func TestGetNotificationRecipients_TransferPickupProposed_RecipientProposes(t *testing.T) {
	// When the recipient proposes a pickup time, the owner should be notified.
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, _ := testStorage.Insert(context.Background(), owner)

	recipient := &models.User{Name: "Recipient", Email: "recipient@example.com", Role: models.Role_ROLE_USER}
	recipientID, _ := testStorage.Insert(context.Background(), recipient)

	gear := &models.Gear{Name: "Drill", OwnerId: ownerID}
	gearID, _ := testStorage.Insert(context.Background(), gear)

	transfer := &models.Transfer{
		GearId:       gearID,
		OwnerId:      ownerID,
		RecipientId:  recipientID,
		TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
	}
	transferID, _ := testStorage.Insert(context.Background(), transfer)

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED,
		ActorId:   recipientID,
		GearId:    gearID,
		Topic:     &models.CommunityEvent_TransferId{TransferId: transferID},
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 1 {
		t.Fatalf("Expected 1 recipient (owner), got %d", len(recipients))
	}
	if recipients[0] != ownerID {
		t.Errorf("Expected owner %s, got %s", ownerID, recipients[0])
	}
}

func TestGetNotificationRecipients_ExperienceCreated(t *testing.T) {
	// When an experience is created, all community members should be notified.
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, _ := testStorage.Insert(context.Background(), owner)

	member1 := &models.User{Name: "Member1", Email: "member1@example.com", Role: models.Role_ROLE_USER}
	member1ID, _ := testStorage.Insert(context.Background(), member1)

	member2 := &models.User{Name: "Member2", Email: "member2@example.com", Role: models.Role_ROLE_USER}
	member2ID, _ := testStorage.Insert(context.Background(), member2)

	community := &models.Community{Name: "Test", Description: "Test", CreatorId: ownerID, OwnerUserId: ownerID}
	communityID, _ := testStorage.Insert(context.Background(), community)

	for _, userID := range []string{ownerID, member1ID, member2ID} {
		_, err := testStorage.Insert(context.Background(), &models.CommunityUser{
			CommunityId: communityID,
			UserId:      userID,
		})
		if err != nil {
			t.Fatalf("Failed to add member: %v", err)
		}
	}

	event := &models.CommunityEvent{
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		ActorId:     ownerID,
		CommunityId: communityID,
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	// Creator (actor) is excluded — only non-actor community members are returned.
	if len(recipients) != 2 {
		t.Fatalf("Expected 2 recipients (community members minus creator), got %d", len(recipients))
	}

	recipientSet := make(map[string]bool)
	for _, r := range recipients {
		recipientSet[r] = true
	}
	if recipientSet[ownerID] {
		t.Errorf("Creator should not be in recipients")
	}
	if !recipientSet[member1ID] {
		t.Errorf("Expected member1 in recipients")
	}
	if !recipientSet[member2ID] {
		t.Errorf("Expected member2 in recipients")
	}
}

func TestGetNotificationRecipients_InvitationLinkUsed(t *testing.T) {
	// When a new member joins via an invitation link, all existing members
	// (minus the new joiner) should receive a notification.
	testStorage := setupTestStorage(t)

	existing1 := &models.User{Name: "Alice", Email: "alice@example.com", Role: models.Role_ROLE_USER}
	existing1ID, _ := testStorage.Insert(context.Background(), existing1)

	existing2 := &models.User{Name: "Bob", Email: "bob@example.com", Role: models.Role_ROLE_USER}
	existing2ID, _ := testStorage.Insert(context.Background(), existing2)

	joiner := &models.User{Name: "Carol", Email: "carol@example.com", Role: models.Role_ROLE_USER}
	joinerID, _ := testStorage.Insert(context.Background(), joiner)

	community := &models.Community{Name: "Test", Description: "Test", CreatorId: existing1ID, OwnerUserId: existing1ID}
	communityID, _ := testStorage.Insert(context.Background(), community)

	for _, userID := range []string{existing1ID, existing2ID, joinerID} {
		if _, err := testStorage.Insert(context.Background(), &models.CommunityUser{
			CommunityId: communityID,
			UserId:      userID,
		}); err != nil {
			t.Fatalf("Failed to add member: %v", err)
		}
	}

	event := &models.CommunityEvent{
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
		ActorId:     joinerID,
		CommunityId: communityID,
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 2 {
		t.Fatalf("Expected 2 recipients (existing members minus joiner), got %d", len(recipients))
	}
	got := map[string]bool{}
	for _, r := range recipients {
		got[r] = true
	}
	if got[joinerID] {
		t.Errorf("joiner should not be in recipients")
	}
	if !got[existing1ID] || !got[existing2ID] {
		t.Errorf("expected both existing members in recipients, got %v", recipients)
	}
}
