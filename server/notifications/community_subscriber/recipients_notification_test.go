package community_subscriber

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestBuildNotification_InvitationLinkUsed(t *testing.T) {
	testStorage := setupTestStorage(t)

	joiner := &models.User{Name: "Carol Smith", Email: "carol@example.com", Role: models.Role_ROLE_USER}
	joinerID, _ := testStorage.Insert(context.Background(), joiner)

	community := &models.Community{Name: "Backyard Tools", Description: "Test", CreatorId: joinerID, OwnerUserId: joinerID}
	communityID, _ := testStorage.Insert(context.Background(), community)

	event := &models.CommunityEvent{
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
		ActorId:     joinerID,
		CommunityId: communityID,
	}

	notif := buildNotification(context.Background(), testStorage, event, enLoc(t))
	if notif.Title != "New member" {
		t.Errorf("expected title 'New member', got %q", notif.Title)
	}
	// Body uses the joiner's first name and the community name so the
	// recipient can tell which community got the new member.
	if notif.Body != "Carol joined Backyard Tools" {
		t.Errorf("expected body 'Carol joined Backyard Tools', got %q", notif.Body)
	}
	// Payload must carry community_id so the FCM router can deep-link to
	// the community detail card.
	payload := notif.GetCommunityEvent()
	if payload == nil || payload.CommunityId != communityID {
		t.Errorf("expected payload.CommunityId=%q, got %+v", communityID, payload)
	}
}

func TestBuildNotification_ExperienceRSVPYes(t *testing.T) {
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, _ := testStorage.Insert(context.Background(), owner)

	actor := &models.User{Name: "Alice", Email: "alice@example.com", Role: models.Role_ROLE_USER}
	actorID, _ := testStorage.Insert(context.Background(), actor)

	experience := &models.Experience{Name: "Morning Hike", OwnerId: ownerID}
	experienceID, _ := testStorage.Insert(context.Background(), experience)

	event := &models.CommunityEvent{
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
		ActorId:      actorID,
		ObjectUserId: ownerID,
		Topic:        &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
	}

	notification := buildNotification(context.Background(), testStorage, event, enLoc(t))

	if notification.Title != "New RSVP" {
		t.Errorf("Expected title 'RSVP to Morning Hike', got '%s'", notification.Title)
	}
	expectedBody := "Alice is attending Morning Hike"
	if notification.Body != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, notification.Body)
	}
	payload := notification.GetCommunityEvent()
	if payload == nil {
		t.Fatal("Expected community event payload, got nil")
	}
	if payload.GetExperienceId() != experienceID {
		t.Errorf("Expected experience_id '%s', got '%s'", experienceID, payload.GetExperienceId())
	}
}

func TestBuildNotification_TransferInterestExpressed(t *testing.T) {
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, _ := testStorage.Insert(context.Background(), owner)

	actor := &models.User{Name: "Alice", Email: "alice@example.com", Role: models.Role_ROLE_USER}
	actorID, _ := testStorage.Insert(context.Background(), actor)

	gear := &models.Gear{Name: "Mountain Bike", OwnerId: ownerID}
	gearID, _ := testStorage.Insert(context.Background(), gear)

	transfer := &models.Transfer{GearId: gearID, OwnerId: ownerID, RecipientId: actorID}
	transferID, _ := testStorage.Insert(context.Background(), transfer)

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		ActorId:   actorID,
		GearId:    gearID,
		Topic:     &models.CommunityEvent_TransferId{TransferId: transferID},
	}

	notification := buildNotification(context.Background(), testStorage, event, enLoc(t))

	if notification.Title != "Interest in Mountain Bike" {
		t.Errorf("Expected title 'Interest in Mountain Bike', got '%s'", notification.Title)
	}
	expectedBody := "Alice wants to borrow your Mountain Bike"
	if notification.Body != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, notification.Body)
	}
	payload := notification.GetCommunityEvent()
	if payload == nil {
		t.Fatal("Expected community event payload, got nil")
	}
	if payload.GearId != gearID {
		t.Errorf("Expected gear_id '%s', got '%s'", gearID, payload.GearId)
	}
	if payload.GearName != "Mountain Bike" {
		t.Errorf("Expected gear_name 'Mountain Bike', got '%s'", payload.GearName)
	}
}

func TestBuildNotification_RequestOfferMade(t *testing.T) {
	testStorage := setupTestStorage(t)

	requester := &models.User{Name: "Requester", Email: "requester@example.com", Role: models.Role_ROLE_USER}
	requesterID, _ := testStorage.Insert(context.Background(), requester)

	offerer := &models.User{Name: "Helper Bob", Email: "bob@example.com", Role: models.Role_ROLE_USER}
	offererID, _ := testStorage.Insert(context.Background(), offerer)

	request := &models.Request{Title: "Need a ladder", RequesterId: requesterID}
	requestID, _ := testStorage.Insert(context.Background(), request)

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE,
		ActorId:   offererID,
		Topic:     &models.CommunityEvent_RequestId{RequestId: requestID},
	}

	notification := buildNotification(context.Background(), testStorage, event, enLoc(t))

	if notification.Title != "Help offered" {
		t.Errorf("Expected title 'Help Offered', got '%s'", notification.Title)
	}
	expectedBody := "Helper Bob offered to help with Need a ladder"
	if notification.Body != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, notification.Body)
	}
	payload := notification.GetCommunityEvent()
	if payload == nil {
		t.Fatal("Expected community event payload, got nil")
	}
	if payload.GetRequestId() != requestID {
		t.Errorf("Expected request_id '%s', got '%s'", requestID, payload.GetRequestId())
	}
	if payload.RequestTitle != "Need a ladder" {
		t.Errorf("Expected request_title 'Need a ladder', got '%s'", payload.RequestTitle)
	}
}

func TestBuildNotification_TransferCancelledLoan(t *testing.T) {
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, _ := testStorage.Insert(context.Background(), owner)

	actor := &models.User{Name: "Alice", Email: "alice@example.com", Role: models.Role_ROLE_USER}
	actorID, _ := testStorage.Insert(context.Background(), actor)

	gear := &models.Gear{Name: "Mountain Bike", OwnerId: ownerID}
	gearID, _ := testStorage.Insert(context.Background(), gear)

	event := &models.CommunityEvent{
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
		ActorId:      actorID,
		GearId:       gearID,
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
	}

	notification := buildNotification(context.Background(), testStorage, event, enLoc(t))

	if notification.Title != "Loan cancelled" {
		t.Errorf("Expected title 'Loan Cancelled', got '%s'", notification.Title)
	}
	expectedBody := "The loan of Mountain Bike was cancelled by Alice"
	if notification.Body != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, notification.Body)
	}
}

func TestBuildNotification_TransferCancelledGiveaway(t *testing.T) {
	testStorage := setupTestStorage(t)

	owner := &models.User{Name: "Owner", Email: "owner@example.com", Role: models.Role_ROLE_USER}
	ownerID, _ := testStorage.Insert(context.Background(), owner)

	actor := &models.User{Name: "Bob", Email: "bob@example.com", Role: models.Role_ROLE_USER}
	actorID, _ := testStorage.Insert(context.Background(), actor)

	gear := &models.Gear{Name: "Old Tent", OwnerId: ownerID}
	gearID, _ := testStorage.Insert(context.Background(), gear)

	event := &models.CommunityEvent{
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
		ActorId:      actorID,
		GearId:       gearID,
		TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
	}

	notification := buildNotification(context.Background(), testStorage, event, enLoc(t))

	if notification.Title != "Giveaway cancelled" {
		t.Errorf("Expected title 'Giveaway Cancelled', got '%s'", notification.Title)
	}
	expectedBody := "The giveaway of Old Tent was cancelled by Bob"
	if notification.Body != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, notification.Body)
	}
}

func TestBuildNotification_ExperienceCreated(t *testing.T) {
	testStorage := setupTestStorage(t)

	actor := &models.User{Name: "Alice", Email: "alice@example.com", Role: models.Role_ROLE_USER}
	actorID, _ := testStorage.Insert(context.Background(), actor)

	experience := &models.Experience{Name: "Morning Hike", OwnerId: actorID}
	experienceID, _ := testStorage.Insert(context.Background(), experience)

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		ActorId:   actorID,
		Topic:     &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
	}

	notification := buildNotification(context.Background(), testStorage, event, enLoc(t))

	if notification.Title != "New event" {
		t.Errorf("Expected title 'New Event', got '%s'", notification.Title)
	}
	expectedBody := "Alice created Morning Hike"
	if notification.Body != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, notification.Body)
	}
}

func TestBuildNotification_NewCopyTemplates(t *testing.T) {
	testStorage := setupTestStorage(t)

	actorID := insertTestUser(t, testStorage, "actor@example.com", "Alice")

	gear := &models.Gear{Name: "Tent", OwnerId: actorID}
	gearID, _ := testStorage.Insert(context.Background(), gear)
	request := &models.Request{Title: "Borrow a kayak", RequesterId: actorID}
	requestID, _ := testStorage.Insert(context.Background(), request)
	experience := &models.Experience{Name: "Beach Cleanup", OwnerId: actorID}
	experienceID, _ := testStorage.Insert(context.Background(), experience)

	cases := []struct {
		eventType models.CommunityEventType
		gearID    string
		topic     any
		wantTitle string
		wantBody  string
		transfer  models.TransferType
	}{
		{
			// No CommunityGear availability → unspecified → loan branch; the
			// article-free name "Tent" gets its article from the template.
			eventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			gearID:    gearID,
			wantTitle: "New gear to borrow",
			wantBody:  "Alice is lending out a Tent",
		},
		{
			eventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
			topic:     &models.CommunityEvent_RequestId{RequestId: requestID},
			wantTitle: "New request",
			wantBody:  "Alice made a new request: Borrow a kayak",
		},
		{
			eventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED,
			topic:     &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
			wantTitle: "Event completed",
			wantBody:  "Beach Cleanup has wrapped up",
		},
		{
			eventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN,
			gearID:    gearID,
			wantTitle: "Interest withdrawn",
			wantBody:  "Alice no longer wants to borrow your Tent",
		},
		{
			eventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE,
			gearID:    gearID,
			transfer:  models.TransferType_TRANSFER_TYPE_LOAN,
			wantTitle: "Loan started",
			wantBody:  "Your loan of Tent is now active",
		},
		{
			eventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED,
			topic:     &models.CommunityEvent_RequestId{RequestId: requestID},
			wantTitle: "Request cancelled",
			wantBody:  "Alice cancelled their request: Borrow a kayak",
		},
		{
			eventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED,
			topic:     &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
			// The title is a category label now; the body carries the event name
			// so it stands alone off-app (#2896).
			wantTitle: "Event starting now",
			wantBody:  "Beach Cleanup is starting now",
		},
		{
			eventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED,
			topic:     &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
			wantTitle: "Event cancelled",
			wantBody:  "Beach Cleanup was cancelled",
		},
	}
	for _, tc := range cases {
		t.Run(tc.eventType.String(), func(t *testing.T) {
			event := &models.CommunityEvent{
				EventType:    tc.eventType,
				ActorId:      actorID,
				GearId:       tc.gearID,
				TransferType: tc.transfer,
			}
			switch v := tc.topic.(type) {
			case *models.CommunityEvent_RequestId:
				event.Topic = v
			case *models.CommunityEvent_ExperienceId:
				event.Topic = v
			}
			notif := buildNotification(context.Background(), testStorage, event, enLoc(t))
			if notif.Title != tc.wantTitle {
				t.Errorf("title = %q, want %q", notif.Title, tc.wantTitle)
			}
			if notif.Body != tc.wantBody {
				t.Errorf("body = %q, want %q", notif.Body, tc.wantBody)
			}
		})
	}
}
