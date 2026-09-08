// seed-purge-fixture inserts a soft-deleted community with one
// row in every cascade table, backdated to 31 days ago, so the
// daily purge job (#1620) has a candidate to act on. One-shot
// dev tool — not for production use.
//
// Usage:
//
//	go run ./server/cmd/seed-purge-fixture \
//	  --db=postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable
//
// Prints the seeded community's ID on success.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func main() {
	dbURL := flag.String("db", "postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable", "PostgreSQL connection string")
	flag.Parse()

	ctx := context.Background()

	store, err := storage.InitializePostgreSQLDatabase(ctx, *dbURL, storage.DefaultStorageTypes())
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}
	defer store.Close()

	now := time.Now().Unix()
	deletedAt := now - community.PurgeWindowSeconds - 3600 // 31 days + 1 hour ago

	community1 := &models.Community{
		Name:        "purge-test",
		Description: "Seeded by seed-purge-fixture for #1620 testing.",
		OwnerUserId: "purge-test-owner",
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  "purge-test-owner",
			DeletedAtUnixSec: deletedAt,
		},
		DeletedSnapshot: &models.CommunityDeletedSnapshot{
			MemberUserIds: []string{"purge-test-owner"},
		},
	}
	communityID, err := store.Insert(ctx, community1)
	if err != nil {
		log.Fatalf("insert community: %v", err)
	}

	type rowFn func() error
	rows := []rowFn{
		func() error {
			_, err := store.Insert(ctx, &models.CommunityUser{CommunityId: communityID, UserId: "purge-test-owner"})
			return err
		},
		func() error {
			_, err := store.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: "purge-test-gear-1"})
			return err
		},
		func() error {
			_, err := store.Insert(ctx, &models.CommunityRequest{CommunityId: communityID, RequestId: "purge-test-req-1"})
			return err
		},
		func() error {
			_, err := store.Insert(ctx, &models.CommunityExperience{CommunityId: communityID, ExperienceId: "purge-test-exp-1"})
			return err
		},
		func() error {
			_, err := store.Insert(ctx, &models.CommunityNotificationPreferences{CommunityId: communityID, UserId: "purge-test-owner"})
			return err
		},
		func() error {
			// TODO(#2056): Remove this legacy-table insert once
			// community_invitation_link is dropped. The share_link
			// insert below covers the new path.
			_, err := store.Insert(ctx, &models.CommunityInvitationLink{CommunityId: communityID, ShortCode: "purge-test-code"})
			return err
		},
		func() error {
			_, err := store.Insert(ctx, &models.ShareLink{
				CommunityId: communityID,
				ShortCode:   "purge-test-share",
				Target: &models.ShareLink_CommunityInviteId{
					CommunityInviteId: communityID,
				},
			})
			return err
		},
		func() error {
			_, err := store.Insert(ctx, &models.CommunityEvent{CommunityId: communityID, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT})
			return err
		},
		func() error {
			_, err := store.Insert(ctx, &models.CommunityRegion{CommunityId: communityID, RegionId: "purge-test-region"})
			return err
		},
		func() error {
			_, err := store.Insert(ctx, &models.Story{CommunityId: communityID})
			return err
		},
		func() error {
			_, err := store.Insert(ctx, &models.StoredNudge{CommunityId: communityID})
			return err
		},
		func() error {
			_, err := store.Insert(ctx, &models.FeedItemView{CommunityId: communityID, UserId: "purge-test-owner", FeedItemId: "fi-1"})
			return err
		},
	}
	for i, fn := range rows {
		if err := fn(); err != nil {
			log.Fatalf("seed cascade row %d: %v", i, err)
		}
	}

	convID, err := store.Insert(ctx, &models.ChatConversation{CommunityId: communityID})
	if err != nil {
		log.Fatalf("seed conversation: %v", err)
	}
	if _, err := store.Insert(ctx, &models.ChatMessage{ConversationId: convID}); err != nil {
		log.Fatalf("seed chat message: %v", err)
	}

	// Confirm via fresh read.
	got := &models.Community{}
	if err := store.GetByID(ctx, communityID, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		log.Fatalf("read back: %v", err)
	}
	if got.GetDeleted().GetDeletedAtUnixSec() != deletedAt {
		log.Fatalf("deleted_at mismatch: got %d, want %d", got.GetDeleted().GetDeletedAtUnixSec(), deletedAt)
	}

	fmt.Printf("seeded community %s, deleted_at=%d (now-%dh)\n",
		communityID, deletedAt, (now-deletedAt)/3600)
}
