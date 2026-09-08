package gear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// TestFetchGearForModify_LogLevels verifies the three outcomes of the
// soft-delete-aware lookup helper: live gear returns the row with no log
// noise; soft-deleted returns NotFound and logs WARN (so it does not trip
// the "Server Error Logged" Cloud Monitoring policy in prod — #2711); a
// hard miss returns NotFound and logs ERROR (real referential-integrity
// bug, should keep paging — same doctrine as fetchExperienceForRead).
func TestFetchGearForModify_LogLevels(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	ctx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)

	if _, err := testStorage.Insert(ctx, &models.User{
		Id: "owner1", Email: "owner@example.com", Name: "Owner",
	}); err != nil {
		t.Fatalf("insert owner: %v", err)
	}

	liveID, err := testStorage.Insert(ctx, &models.Gear{
		OwnerId: "owner1",
		Name:    "Live Gear",
	})
	if err != nil {
		t.Fatalf("insert live gear: %v", err)
	}

	softDeletedID, err := testStorage.Insert(ctx, &models.Gear{
		OwnerId: "owner1",
		Name:    "Soft-Deleted Gear",
	})
	if err != nil {
		t.Fatalf("insert soft-delete gear: %v", err)
	}
	deleted := &models.Gear{}
	if err := testStorage.GetByID(context.Background(), softDeletedID, deleted); err != nil {
		t.Fatalf("GetByID before soft-delete: %v", err)
	}
	deleted.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  "owner1",
		DeletedAtUnixSec: time.Now().Unix(),
	}
	if err := testStorage.Update(context.Background(), deleted); err != nil {
		t.Fatalf("Update to soft-delete: %v", err)
	}

	type tc struct {
		name       string
		id         string
		wantGear   bool
		wantNotFnd bool
		wantLevel  slog.Level
		wantReason string
	}
	cases := []tc{
		{name: "live", id: liveID, wantGear: true, wantLevel: 0},
		{name: "soft_deleted", id: softDeletedID, wantNotFnd: true, wantLevel: slog.LevelWarn, wantReason: "soft_deleted"},
		{name: "missing", id: "00000000-0000-0000-0000-000000000000", wantNotFnd: true, wantLevel: slog.LevelError},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

			got, err := service.fetchGearForModify(ctx, c.id, logger, "Test")

			if c.wantGear {
				if err != nil {
					t.Fatalf("expected gear, got error: %v", err)
				}
				if got == nil || got.Id != c.id {
					t.Fatalf("expected gear id %q, got %+v", c.id, got)
				}
				if buf.Len() != 0 {
					t.Errorf("live lookup should not log; got: %s", buf.String())
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error for case %q, got nil", c.name)
			}
			var connectErr *connect.Error
			if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeNotFound {
				t.Fatalf("expected connect.CodeNotFound, got %v", err)
			}

			line := strings.TrimSpace(buf.String())
			if line == "" {
				t.Fatalf("expected one log line, got none")
			}
			var entry map[string]any
			if err := json.Unmarshal([]byte(strings.Split(line, "\n")[0]), &entry); err != nil {
				t.Fatalf("log line not valid JSON: %v\n%s", err, line)
			}
			gotLevel, _ := entry["level"].(string)
			wantLevelStr := c.wantLevel.String()
			if gotLevel != wantLevelStr {
				t.Errorf("want log level %q, got %q (line=%s)", wantLevelStr, gotLevel, line)
			}
			if c.wantReason != "" {
				if gotReason, _ := entry["reason"].(string); gotReason != c.wantReason {
					t.Errorf("want reason=%q, got %q (line=%s)", c.wantReason, gotReason, line)
				}
			}
		})
	}

	// Sanity: storage layer must still return ErrRecordNotFound for a hard
	// miss so the helper's error classification stays correct.
	if err := testStorage.GetByID(ctx, "00000000-0000-0000-0000-000000000000", &models.Gear{}); !errors.Is(err, storage.ErrRecordNotFound) {
		t.Errorf("expected ErrRecordNotFound from storage, got %v", err)
	}
}
