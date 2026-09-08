package notifications

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/storage"
)

// fakeBucket satisfies storage.BucketStorage via an embedded (nil) interface and
// overrides only Get — the one method buildEmailExtras calls. Any other call
// would panic, which is the point: the test exercises exactly Get.
type fakeBucket struct {
	storage.BucketStorage
	data        []byte
	contentType string
	err         error
}

func (f *fakeBucket) Get(context.Context, string) ([]byte, string, error) {
	return f.data, f.contentType, f.err
}

func setupSummaryService(t *testing.T, bucket storage.BucketStorage) (*service, *storage.ProtoSQLStorage) {
	t.Helper()
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	svc := NewService(nil, store, WithOffAppEmail(OffAppEmailConfig{
		Enabled:     true,
		Sender:      &fakeEmailSender{},
		AppBaseURL:  "https://test.example.com",
		UnsubSecret: []byte(testUnsubSecret),
		Bucket:      bucket,
	})).(*service)
	return svc, store
}

func enLocalizer(t *testing.T) *l10n.Localizer {
	t.Helper()
	loc, err := l10n.NewLocalizer(language.English)
	if err != nil {
		t.Fatalf("NewLocalizer(en): %v", err)
	}
	return loc
}

func TestBuildEmailExtras_Experience(t *testing.T) {
	bucket := &fakeBucket{data: []byte("\xff\xd8jpegbytes"), contentType: "image/jpeg"}
	svc, store := setupSummaryService(t, bucket)
	ctx := context.Background()

	ownerID, _ := store.Insert(ctx, &models.User{Name: "Alex Owner", Email: "alex@example.com", Role: models.Role_ROLE_USER})
	mediaID, _ := store.Insert(ctx, &models.Media{UserId: ownerID})
	expID, _ := store.Insert(ctx, &models.Experience{
		Name:        "Backyard BBQ",
		OwnerId:     ownerID,
		Description: "Burgers and lawn games.",
		MediaIds:    []string{mediaID},
	})
	// Two YES, one NO → "2 going".
	for _, intent := range []models.RSVPIntention{
		models.RSVPIntention_RSVP_INTENTION_YES,
		models.RSVPIntention_RSVP_INTENTION_YES,
		models.RSVPIntention_RSVP_INTENTION_NO,
	} {
		if _, err := store.Insert(ctx, &models.ExperienceRSVP{ExperienceId: expID, Intention: intent}); err != nil {
			t.Fatalf("insert rsvp: %v", err)
		}
	}

	sum, inline, heroURL := svc.buildEmailExtras(ctx, &models.CommunityEventPayload{ExperienceId: &expID}, enLocalizer(t))
	if sum == nil {
		t.Fatal("expected a summary, got nil")
	}
	if sum.Name != "Backyard BBQ" {
		t.Errorf("Name = %q, want Backyard BBQ", sum.Name)
	}
	if sum.Eyebrow != "Event" {
		t.Errorf("Eyebrow = %q, want Event", sum.Eyebrow)
	}
	if sum.OwnerLine != "Hosted by Alex" {
		t.Errorf("OwnerLine = %q, want 'Hosted by Alex'", sum.OwnerLine)
	}
	if sum.Going != "2 going" {
		t.Errorf("Going = %q, want '2 going'", sum.Going)
	}
	if sum.Description != "Burgers and lawn games." {
		t.Errorf("Description = %q", sum.Description)
	}
	if heroURL != "cid:hero.jpg" || len(inline) != 1 || inline[0].Filename != "hero.jpg" {
		t.Errorf("hero = %q inline = %+v, want cid:hero.jpg + 1 inline image", heroURL, inline)
	}
}

func TestBuildEmailExtras_GearGiveawayVsLoan(t *testing.T) {
	svc, store := setupSummaryService(t, nil) // nil bucket → no hero, summary still built
	ctx := context.Background()
	loc := enLocalizer(t)

	ownerID, _ := store.Insert(ctx, &models.User{Name: "Sam Owner", Email: "sam@example.com", Role: models.Role_ROLE_USER})
	commID, _ := store.Insert(ctx, &models.Community{Name: "Block", CreatorId: ownerID, OwnerUserId: ownerID})
	gearID, _ := store.Insert(ctx, &models.Gear{Name: "Power Drill", OwnerId: ownerID, Description: "20V cordless."})

	// No CommunityGear row → loan eyebrow.
	sum, _, _ := svc.buildEmailExtras(ctx, &models.CommunityEventPayload{GearId: gearID, CommunityId: commID}, loc)
	if sum == nil || sum.Eyebrow != "Available to borrow" {
		t.Fatalf("loan: Eyebrow = %v, want 'Available to borrow'", sum)
	}
	if sum.OwnerLine != "Shared by Sam" || sum.Name != "Power Drill" {
		t.Errorf("loan: got Name=%q Owner=%q", sum.Name, sum.OwnerLine)
	}

	// Mark as giveaway → giveaway eyebrow.
	if _, err := store.Insert(ctx, &models.CommunityGear{GearId: gearID, CommunityId: commID, Availability: models.Availability_AVAILABILITY_FOR_GIVEAWAY}); err != nil {
		t.Fatalf("insert community gear: %v", err)
	}
	sum, _, _ = svc.buildEmailExtras(ctx, &models.CommunityEventPayload{GearId: gearID, CommunityId: commID}, loc)
	if sum == nil || sum.Eyebrow != "Free to a good home" {
		t.Fatalf("giveaway: Eyebrow = %v, want 'Free to a good home'", sum)
	}
}

func TestBuildEmailExtras_Request(t *testing.T) {
	svc, store := setupSummaryService(t, nil)
	ctx := context.Background()

	reqUserID, _ := store.Insert(ctx, &models.User{Name: "Pat Requester", Email: "pat@example.com", Role: models.Role_ROLE_USER})
	reqID, _ := store.Insert(ctx, &models.Request{Title: "Folding Table", RequesterId: reqUserID, Description: "For a party."})

	sum, _, hero := svc.buildEmailExtras(ctx, &models.CommunityEventPayload{RequestId: &reqID}, enLocalizer(t))
	if sum == nil || sum.Eyebrow != "Request" || sum.Name != "Folding Table" {
		t.Fatalf("request summary = %+v", sum)
	}
	if sum.OwnerLine != "Requested by Pat" {
		t.Errorf("OwnerLine = %q, want 'Requested by Pat'", sum.OwnerLine)
	}
	if hero != "" {
		t.Errorf("hero = %q, want empty (no bucket)", hero)
	}
}

func TestBuildEmailExtras_NoEntity(t *testing.T) {
	svc, _ := setupSummaryService(t, nil)
	sum, inline, hero := svc.buildEmailExtras(context.Background(), &models.CommunityEventPayload{CommunityName: "Block"}, enLocalizer(t))
	if sum != nil || inline != nil || hero != "" {
		t.Errorf("no-entity payload should yield no extras; got %+v %v %q", sum, inline, hero)
	}
}

func TestTruncateDescription(t *testing.T) {
	if got := truncateDescription("  hi  "); got != "hi" {
		t.Errorf("trim: %q", got)
	}
	long := strings.Repeat("a", maxSummaryDescription+50)
	got := truncateDescription(long)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) != maxSummaryDescription+1 {
		t.Errorf("truncate len = %d, want %d + ellipsis", len([]rune(got)), maxSummaryDescription)
	}
}

func TestImageExt(t *testing.T) {
	cases := map[string]string{
		"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp",
		"image/gif": "gif", "application/octet-stream": "jpg", "": "jpg",
	}
	for ct, want := range cases {
		if got := imageExt(ct); got != want {
			t.Errorf("imageExt(%q) = %q, want %q", ct, got, want)
		}
	}
}

func TestFormatSummaryDate(t *testing.T) {
	if got := formatSummaryDate(nil); got != "" {
		t.Errorf("nil time = %q, want empty", got)
	}
	// 2026-06-27 12:00 UTC.
	specific := &models.ExperienceTime{TimeType: &models.ExperienceTime_Specific{
		Specific: &models.SpecificTime{UnixTimestampSec: 1782561600},
	}}
	if got := formatSummaryDate(specific); got != "June 27" {
		t.Errorf("formatSummaryDate = %q, want 'June 27'", got)
	}
}
