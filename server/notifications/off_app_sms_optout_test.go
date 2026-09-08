package notifications

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestShouldDiscloseOptOut(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	cases := []struct {
		name string
		last int64
		want bool
	}{
		{"never disclosed", 0, true},
		{"within interval", now.Add(-10 * 24 * time.Hour).Unix(), false},
		{"exactly at interval", now.Add(-optOutDisclosureInterval).Unix(), true},
		{"after interval", now.Add(-40 * 24 * time.Hour).Unix(), true},
	}
	for _, tc := range cases {
		u := &models.User{SmsOptoutDisclosedAtUnixSec: tc.last}
		if got := shouldDiscloseOptOut(u, now); got != tc.want {
			t.Errorf("%s: shouldDiscloseOptOut = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestSendOffAppSMS_OptOutCadence proves the disclosure rides the first platform
// SMS and records a timestamp, then a second send within the interval omits the
// footer and leaves the timestamp unchanged.
func TestSendOffAppSMS_OptOutCadence(t *testing.T) {
	svc, sender, store := setupSMSService(t)
	ctx := context.Background()

	uid, err := store.Insert(ctx, &models.User{
		Name:        "Deviceless",
		Email:       "cadence@example.com",
		Role:        models.Role_ROLE_USER,
		PhoneNumber: proto.String("+14155559999"),
	})
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	fetch := func() *models.User {
		got := &models.User{}
		if err := store.GetByID(ctx, uid, got); err != nil {
			t.Fatalf("get user: %v", err)
		}
		return got
	}

	n := &models.Notification{Title: "Interest in your drill", Body: "Alex wants to borrow it."}

	// First send → footer present, disclosure timestamp recorded.
	if !svc.sendOffAppSMS(ctx, fetch(), n) {
		t.Fatal("first send: expected sent=true")
	}
	if first := sender.Messages()[0].Body; !strings.Contains(first, "Reply STOP") {
		t.Errorf("first SMS should carry the opt-out footer:\n%s", first)
	}
	recorded := fetch().GetSmsOptoutDisclosedAtUnixSec()
	if recorded == 0 {
		t.Fatal("first send should record the disclosure timestamp")
	}

	// Second send within the interval → no footer, timestamp unchanged.
	if !svc.sendOffAppSMS(ctx, fetch(), n) {
		t.Fatal("second send: expected sent=true")
	}
	if second := sender.Messages()[1].Body; strings.Contains(second, "STOP") {
		t.Errorf("second SMS within interval should omit the footer:\n%s", second)
	}
	if after := fetch().GetSmsOptoutDisclosedAtUnixSec(); after != recorded {
		t.Errorf("timestamp should be unchanged within interval: was %d, now %d", recorded, after)
	}
}
