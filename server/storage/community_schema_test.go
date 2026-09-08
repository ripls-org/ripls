package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/lib/pq"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestInitializeCommunityOwnerUserID_BackfillsCreatorID seeds a Community
// with an empty OwnerUserId, re-runs the initializer, and verifies the
// flat column and the round-tripped proto both reflect creator_id.
func TestInitializeCommunityOwnerUserID_BackfillsCreatorID(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Direct INSERT into the flat column with empty owner_user_id, bypassing
	// storage.Insert (which would now write owner_user_id from the proto and
	// pre-emptively block the empty case via the CHECK constraint added by
	// the same initializer that already ran in SetupTestStorage). Fabricate a
	// row that mimics the pre-#1644 prod state.
	community := &models.Community{
		Id:               "comm-backfill-1",
		Name:             "Backfill Test",
		Description:      "seeded with empty owner_user_id",
		CreatorId:        "user-creator-1",
		OwnerUserId:      "", // explicit empty — what existing rows look like
		CreatedAtUnixSec: 1735689600,
		UpdatedAtUnixSec: 1735689600,
	}
	binaryProto, err := proto.Marshal(community)
	if err != nil {
		t.Fatalf("marshal community: %v", err)
	}
	// Drop the constraint, insert the bad row, then re-run the initializer
	// which should backfill it.
	if _, err := storage.db.ExecContext(ctx, `ALTER TABLE "community" DROP CONSTRAINT IF EXISTS community_owner_required`); err != nil {
		t.Fatalf("drop constraint: %v", err)
	}
	if _, err := storage.db.ExecContext(ctx, `
		INSERT INTO "community" (id, name, description, creator_id, owner_user_id, binary_proto)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, community.Id, community.Name, community.Description, community.CreatorId, community.OwnerUserId, binaryProto); err != nil {
		t.Fatalf("seed community: %v", err)
	}

	if err := storage.InitializeCommunityOwnerUserID(ctx); err != nil {
		t.Fatalf("InitializeCommunityOwnerUserID: %v", err)
	}

	// Flat column should be backfilled.
	var flatOwner string
	if err := storage.db.QueryRowContext(ctx, `SELECT owner_user_id FROM "community" WHERE id = $1`, community.Id).Scan(&flatOwner); err != nil {
		t.Fatalf("read flat owner_user_id: %v", err)
	}
	if flatOwner != community.CreatorId {
		t.Errorf("flat owner_user_id = %q, want %q", flatOwner, community.CreatorId)
	}

	// Binary proto should round-trip with the same value.
	got := &models.Community{}
	if err := storage.GetByID(ctx, community.Id, got); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.OwnerUserId != community.CreatorId {
		t.Errorf("got.OwnerUserId = %q, want %q", got.OwnerUserId, community.CreatorId)
	}
}

// TestInitializeCommunityOwnerUserID_LeavesOwnedRowsAlone confirms the
// backfill is a no-op for rows where owner_user_id is already set.
func TestInitializeCommunityOwnerUserID_LeavesOwnedRowsAlone(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	community := &models.Community{
		Id:               "comm-noop-1",
		Name:             "Already Owned",
		CreatorId:        "user-creator-2",
		OwnerUserId:      "user-different-owner",
		CreatedAtUnixSec: 1735689600,
		UpdatedAtUnixSec: 1735689600,
	}
	if _, err := storage.Insert(ctx, community); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := storage.InitializeCommunityOwnerUserID(ctx); err != nil {
		t.Fatalf("InitializeCommunityOwnerUserID: %v", err)
	}

	got := &models.Community{}
	if err := storage.GetByID(ctx, community.Id, got); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.OwnerUserId != "user-different-owner" {
		t.Errorf("OwnerUserId clobbered: got %q, want %q", got.OwnerUserId, "user-different-owner")
	}
}

// TestCommunityOwnerRequiredConstraintRejectsEmptyOwner asserts the
// CHECK constraint rejects an active row with an empty owner_user_id.
// SetupTestStorage already installed the constraint via the same
// initializer the production startup uses.
func TestCommunityOwnerRequiredConstraintRejectsEmptyOwner(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	bad := &models.Community{
		Id:               "comm-bad-1",
		Name:             "No Owner",
		CreatorId:        "user-creator-3",
		OwnerUserId:      "",
		CreatedAtUnixSec: 1735689600,
		UpdatedAtUnixSec: 1735689600,
	}
	_, err := storage.Insert(ctx, bad)
	if err == nil {
		t.Fatalf("Insert with empty OwnerUserId unexpectedly succeeded")
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		t.Fatalf("expected *pq.Error, got %T: %v", err, err)
	}
	if string(pqErr.Code) != "23514" {
		t.Errorf("expected SQLSTATE 23514 (check_violation), got %q", pqErr.Code)
	}

	// Same shape with a non-empty owner should succeed.
	good := &models.Community{
		Id:               "comm-good-1",
		Name:             bad.Name,
		CreatorId:        bad.CreatorId,
		OwnerUserId:      bad.CreatorId,
		CreatedAtUnixSec: bad.CreatedAtUnixSec,
		UpdatedAtUnixSec: bad.UpdatedAtUnixSec,
	}
	if _, err := storage.Insert(ctx, good); err != nil {
		t.Fatalf("Insert with valid OwnerUserId: %v", err)
	}
}

// TestCommunityOwnerRequiredConstraintAllowsSoftDeletedRow confirms the
// CHECK lets soft-deleted rows have an empty owner_user_id (the
// post-delete state in #1619 Phase 2).
func TestCommunityOwnerRequiredConstraintAllowsSoftDeletedRow(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	community := &models.Community{
		Id:          "comm-deleted-1",
		Name:        "Soft Deleted",
		CreatorId:   "user-creator-4",
		OwnerUserId: "",
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  "user-creator-4",
			DeletedAtUnixSec: 1735689999,
		},
		CreatedAtUnixSec: 1735689600,
		UpdatedAtUnixSec: 1735689600,
	}
	if _, err := storage.Insert(ctx, community); err != nil {
		t.Fatalf("Insert soft-deleted community: %v", err)
	}
}

// TestInitializeCommunityUserUniqueness_RejectsDuplicate asserts the
// UNIQUE(community_id, user_id) constraint is in place after setup.
func TestInitializeCommunityUserUniqueness_RejectsDuplicate(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	first := &models.CommunityUser{
		Id:               "cu-1",
		CommunityId:      "comm-unique-1",
		UserId:           "user-1",
		InviterId:        "user-inviter",
		CreatedAtUnixSec: 1735689600,
	}
	if _, err := storage.Insert(ctx, first); err != nil {
		t.Fatalf("seed first membership: %v", err)
	}

	dup := &models.CommunityUser{
		Id:               "cu-2", // different primary key
		CommunityId:      first.CommunityId,
		UserId:           first.UserId,
		InviterId:        "user-inviter",
		CreatedAtUnixSec: 1735689700,
	}
	_, err := storage.Insert(ctx, dup)
	if err == nil {
		t.Fatalf("duplicate (community_id, user_id) Insert unexpectedly succeeded")
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		t.Fatalf("expected *pq.Error, got %T: %v", err, err)
	}
	if string(pqErr.Code) != "23505" {
		t.Errorf("expected SQLSTATE 23505 (unique_violation), got %q", pqErr.Code)
	}
}

// TestInitializeCommunityOwnerUserID_Idempotent re-runs the helper
// twice on the same DB and confirms no error is returned (i.e. the
// constraint-existence and column-existence guards work).
func TestInitializeCommunityOwnerUserID_Idempotent(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	if err := storage.InitializeCommunityOwnerUserID(ctx); err != nil {
		t.Fatalf("first re-run: %v", err)
	}
	if err := storage.InitializeCommunityOwnerUserID(ctx); err != nil {
		t.Fatalf("second re-run: %v", err)
	}
	if err := storage.InitializeCommunityUserUniqueness(ctx); err != nil {
		t.Fatalf("uniqueness re-run: %v", err)
	}
}

// seedCommunityGearWithConvID inserts a community_gear row with the given
// conversation_id value in the flat SQL column only (not in binary_proto),
// simulating a pre-migration row that still carries the deprecated field
// as a raw column value. Adds the legacy column if it does not exist (fresh
// test DB schema does not include it after the Phase 3 proto deletion).
func seedCommunityGearWithConvID(t *testing.T, s *ProtoSQLStorage, id, communityID, gearID, convID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE "community_gear" ADD COLUMN IF NOT EXISTS conversation_id TEXT`); err != nil {
		t.Fatalf("add legacy conversation_id column to community_gear: %v", err)
	}
	cg := &models.CommunityGear{Id: id, CommunityId: communityID, GearId: gearID}
	cgBytes, err := proto.Marshal(cg)
	if err != nil {
		t.Fatalf("marshal community_gear: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO "community_gear" (id, community_id, gear_id, conversation_id, binary_proto)
		VALUES ($1, $2, $3, $4, $5)
	`, id, communityID, gearID, convID, cgBytes); err != nil {
		t.Fatalf("seed community_gear: %v", err)
	}
}

// TestInitializeCommunityConversationIDMigration_BackfillsGear seeds a
// community_gear row with a conversation_id flat column value and a Gear with
// an empty conversation_id, then verifies the backfill copies the value.
func TestInitializeCommunityConversationIDMigration_BackfillsGear(t *testing.T) {
	s, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	gearID := "gear-conv-backfill-1"
	convID := "conv-gear-1"

	gear := &models.Gear{Id: gearID, Name: "Drill", OwnerId: "user-owner-1"}
	if _, err := s.Insert(ctx, gear); err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	seedCommunityGearWithConvID(t, s, "cg-conv-bf-1", "comm-1", gearID, convID)

	if err := s.InitializeCommunityConversationIDMigration(ctx); err != nil {
		t.Fatalf("InitializeCommunityConversationIDMigration: %v", err)
	}

	got := &models.Gear{}
	if err := s.GetByID(ctx, gearID, got); err != nil {
		t.Fatalf("GetByID gear: %v", err)
	}
	if got.ConversationId != convID {
		t.Errorf("gear.ConversationId = %q, want %q", got.ConversationId, convID)
	}
}

// TestInitializeCommunityConversationIDMigration_LeavesPopulatedGearAlone
// verifies the backfill does not overwrite a Gear.conversation_id already set.
func TestInitializeCommunityConversationIDMigration_LeavesPopulatedGearAlone(t *testing.T) {
	s, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	gearID := "gear-conv-noop-1"
	existingConvID := "conv-gear-existing"

	gear := &models.Gear{Id: gearID, Name: "Hammer", OwnerId: "user-owner-2", ConversationId: existingConvID}
	if _, err := s.Insert(ctx, gear); err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	seedCommunityGearWithConvID(t, s, "cg-conv-noop-1", "comm-2", gearID, "conv-gear-deprecated")

	if err := s.InitializeCommunityConversationIDMigration(ctx); err != nil {
		t.Fatalf("InitializeCommunityConversationIDMigration: %v", err)
	}

	got := &models.Gear{}
	if err := s.GetByID(ctx, gearID, got); err != nil {
		t.Fatalf("GetByID gear: %v", err)
	}
	if got.ConversationId != existingConvID {
		t.Errorf("gear.ConversationId clobbered: got %q, want %q", got.ConversationId, existingConvID)
	}
}

// TestInitializeCommunityConversationIDMigration_BackfillsRequest mirrors the
// gear test for community_request → Request.
func TestInitializeCommunityConversationIDMigration_BackfillsRequest(t *testing.T) {
	s, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	requestID := "req-conv-backfill-1"
	convID := "conv-req-1"

	request := &models.Request{Id: requestID, RequesterId: "user-requester-1", Title: "Need a ladder"}
	if _, err := s.Insert(ctx, request); err != nil {
		t.Fatalf("insert request: %v", err)
	}

	cr := &models.CommunityRequest{Id: "cr-conv-bf-1", CommunityId: "comm-3", RequestId: requestID}
	crBytes, _ := proto.Marshal(cr)
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE "community_request" ADD COLUMN IF NOT EXISTS conversation_id TEXT`); err != nil {
		t.Fatalf("add legacy conversation_id column to community_request: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO "community_request" (id, community_id, request_id, conversation_id, binary_proto)
		VALUES ($1, $2, $3, $4, $5)
	`, cr.Id, cr.CommunityId, cr.RequestId, convID, crBytes); err != nil {
		t.Fatalf("seed community_request: %v", err)
	}

	if err := s.InitializeCommunityConversationIDMigration(ctx); err != nil {
		t.Fatalf("InitializeCommunityConversationIDMigration: %v", err)
	}

	got := &models.Request{}
	if err := s.GetByID(ctx, requestID, got); err != nil {
		t.Fatalf("GetByID request: %v", err)
	}
	if got.ConversationId != convID {
		t.Errorf("request.ConversationId = %q, want %q", got.ConversationId, convID)
	}
}

// TestInitializeCommunityConversationIDMigration_BackfillsExperience mirrors
// the gear test for community_experience → Experience.
func TestInitializeCommunityConversationIDMigration_BackfillsExperience(t *testing.T) {
	s, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	experienceID := "exp-conv-backfill-1"
	convID := "conv-exp-1"

	experience := &models.Experience{Id: experienceID, OwnerId: "user-owner-3", Name: "Community Potluck"}
	if _, err := s.Insert(ctx, experience); err != nil {
		t.Fatalf("insert experience: %v", err)
	}

	ce := &models.CommunityExperience{Id: "ce-conv-bf-1", CommunityId: "comm-4", ExperienceId: experienceID}
	ceBytes, _ := proto.Marshal(ce)
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE "community_experience" ADD COLUMN IF NOT EXISTS conversation_id TEXT`); err != nil {
		t.Fatalf("add legacy conversation_id column to community_experience: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO "community_experience" (id, community_id, experience_id, conversation_id, binary_proto)
		VALUES ($1, $2, $3, $4, $5)
	`, ce.Id, ce.CommunityId, ce.ExperienceId, convID, ceBytes); err != nil {
		t.Fatalf("seed community_experience: %v", err)
	}

	if err := s.InitializeCommunityConversationIDMigration(ctx); err != nil {
		t.Fatalf("InitializeCommunityConversationIDMigration: %v", err)
	}

	got := &models.Experience{}
	if err := s.GetByID(ctx, experienceID, got); err != nil {
		t.Fatalf("GetByID experience: %v", err)
	}
	if got.ConversationId != convID {
		t.Errorf("experience.ConversationId = %q, want %q", got.ConversationId, convID)
	}
}

// TestInitializeCommunityConversationIDMigration_SkipsMissingParent verifies
// that the backfill skips a join row whose parent entity does not exist,
// instead of failing.
func TestInitializeCommunityConversationIDMigration_SkipsMissingParent(t *testing.T) {
	s, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	// Seed a community_gear row whose gear_id does not exist.
	seedCommunityGearWithConvID(t, s, "cg-orphan-1", "comm-5", "gear-missing", "conv-orphan")

	if err := s.InitializeCommunityConversationIDMigration(ctx); err != nil {
		t.Fatalf("InitializeCommunityConversationIDMigration should not fail on missing parent: %v", err)
	}
}

// TestInitializeCommunityConversationIDMigration_Idempotent verifies two
// consecutive runs produce no error.
func TestInitializeCommunityConversationIDMigration_Idempotent(t *testing.T) {
	s, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	if err := s.InitializeCommunityConversationIDMigration(ctx); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := s.InitializeCommunityConversationIDMigration(ctx); err != nil {
		t.Fatalf("second run: %v", err)
	}
}
