package experience

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/clock"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestFetchExperienceForRead_LogLevels verifies the three outcomes of the
// soft-delete-aware lookup helper: live experience returns the row with no
// log noise; soft-deleted returns NotFound and logs WARN (so it does not
// trip the "Server Error Logged" Cloud Monitoring policy in prod); a hard
// miss returns NotFound and logs ERROR (real bug, should keep paging).
func TestFetchExperienceForRead_LogLevels(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "owner1", "owner@example.com", "Owner")

	ctx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)
	community := createTestCommunity(t, testStorage, "Community", "owner1")
	createTestCommunityMembership(t, testStorage, community, "owner1")

	saveResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Live Experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	liveID := saveResp.Msg.Experience.Id

	softDeletedResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Soft-Deleted Experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience for soft-delete case failed: %v", err)
	}
	softDeletedID := softDeletedResp.Msg.Experience.Id

	deletedExp := &models.Experience{}
	if err := testStorage.GetByID(context.Background(), softDeletedID, deletedExp); err != nil {
		t.Fatalf("GetByID before soft-delete failed: %v", err)
	}
	deletedExp.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  "owner1",
		DeletedAtUnixSec: clock.UnixSec(context.Background()),
	}
	if err := testStorage.Update(context.Background(), deletedExp); err != nil {
		t.Fatalf("Update to soft-delete failed: %v", err)
	}

	type tc struct {
		name       string
		id         string
		wantExp    bool
		wantNotFnd bool
		wantLevel  slog.Level
		wantReason string
	}
	cases := []tc{
		{name: "live", id: liveID, wantExp: true, wantLevel: 0},
		{name: "soft_deleted", id: softDeletedID, wantNotFnd: true, wantLevel: slog.LevelWarn, wantReason: "soft_deleted"},
		{name: "missing", id: "00000000-0000-0000-0000-000000000000", wantNotFnd: true, wantLevel: slog.LevelError},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

			got, err := service.fetchExperienceForRead(ctx, c.id, logger, "Test")

			if c.wantExp {
				if err != nil {
					t.Fatalf("expected experience, got error: %v", err)
				}
				if got == nil || got.Id != c.id {
					t.Fatalf("expected experience id %q, got %+v", c.id, got)
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
	if err := testStorage.GetByID(ctx, "no-such-id", &models.Experience{}); !errors.Is(err, storage.ErrRecordNotFound) {
		t.Errorf("expected ErrRecordNotFound from storage, got %v", err)
	}
}
