package web

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

const testHostname = "test.example.com"

// setupTestService creates a web service with real DB storage and mock bucket.
func setupTestService(t *testing.T) (*Service, *storage.ProtoSQLStorage, *services.MockBucketStorage) {
	t.Helper()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	mockBucket := &services.MockBucketStorage{}
	svc, err := New(sqlStorage, mockBucket, testHostname, []byte("test-unsubscribe-secret"))
	if err != nil {
		t.Fatalf("Failed to create web service: %v", err)
	}

	return svc, sqlStorage, mockBucket
}

// generateTestShortCode generates a random 8-char short code for tests.
func generateTestShortCode(t *testing.T) string {
	t.Helper()
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"
	b := make([]byte, 8)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			t.Fatalf("Failed to generate test short code: %v", err)
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// createTestUser inserts a test user into the database.
func createTestUser(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, name string) *models.User {
	t.Helper()
	now := time.Now().Unix()
	user := &models.User{
		Id:         uuid.New().String(),
		Email:      name + "@test.com",
		Name:       name,
		CreatedAt:  now,
		UpdatedAt:  now,
		Role:       models.Role_ROLE_USER,
		AuthMethod: models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
	}
	_, err := sqlStorage.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	return user
}

// createTestCommunity inserts a test community into the database.
func createTestCommunity(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, creatorID string, mediaIDs []string) *models.Community {
	t.Helper()
	now := time.Now().Unix()
	community := &models.Community{
		Id:               uuid.New().String(),
		Name:             "Test Community",
		Description:      "A test community for web service tests",
		CreatorId:        creatorID,
		OwnerUserId:      creatorID,
		MediaIds:         mediaIDs,
		CreatedAtUnixSec: now,
		UpdatedAtUnixSec: now,
	}
	_, err := sqlStorage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Failed to create test community: %v", err)
	}
	return community
}

// createTestInvitation inserts a test community-invite share link into the database.
func createTestInvitation(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID, inviterID string, revoked bool) *models.ShareLink {
	t.Helper()
	invitation := &models.ShareLink{
		Id:               uuid.New().String(),
		CommunityId:      communityID,
		InviterId:        inviterID,
		ShortCode:        generateTestShortCode(t),
		IsRevoked:        revoked,
		CreatedAtUnixSec: time.Now().Unix(),
		Target: &models.ShareLink_CommunityInviteId{
			CommunityInviteId: communityID,
		},
	}
	_, err := sqlStorage.Insert(ctx, invitation)
	if err != nil {
		t.Fatalf("Failed to create test invitation: %v", err)
	}
	return invitation
}

// createTestMembership inserts a community membership into the database.
func createTestMembership(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	membership := &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           userID,
		InviterId:        userID,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	_, err := sqlStorage.Insert(ctx, membership)
	if err != nil {
		t.Fatalf("Failed to create test membership: %v", err)
	}
}

// createTestMedia inserts a test media record into the database.
func createTestMedia(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, userID string) *models.Media {
	t.Helper()
	media := &models.Media{
		Id:         uuid.New().String(),
		UserId:     userID,
		StorageUrl: "mock://bucket/" + userID + "/media",
	}
	_, err := sqlStorage.Insert(ctx, media)
	if err != nil {
		t.Fatalf("Failed to create test media: %v", err)
	}
	return media
}

func TestHandleInvitePage_ValidInvitation(t *testing.T) {
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()

	// Set up test data.
	inviter := createTestUser(t, ctx, sqlStorage, "Alice")
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id, nil)
	createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

	_ = mockBucket // no community image in this test

	req := httptest.NewRequest(http.MethodGet, "/go/"+invitation.ShortCode, nil)
	w := httptest.NewRecorder()

	svc.HandleInvitePage(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()

	// Verify OG tags are present with correct community info.
	if !strings.Contains(body, `og:title`) {
		t.Error("Expected og:title meta tag in response")
	}
	if !strings.Contains(body, community.Name) {
		t.Errorf("Expected community name %q in response body", community.Name)
	}
	if !strings.Contains(body, inviter.Name) {
		t.Errorf("Expected inviter name %q in response body", inviter.Name)
	}
	if !strings.Contains(body, invitation.ShortCode) {
		t.Errorf("Expected short code %q in response body", invitation.ShortCode)
	}
	// Default image should be used (no community media).
	expectedDefaultImage := "https://test.example.com/assets/logo.png"
	if !strings.Contains(body, expectedDefaultImage) {
		t.Errorf("Expected default OG image URL %q in response body", expectedDefaultImage)
	}
}

func TestHandleInvitePage_ValidInvitationWithCommunityImage(t *testing.T) {
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()

	inviter := createTestUser(t, ctx, sqlStorage, "Bob")
	media := createTestMedia(t, ctx, sqlStorage, inviter.Id)
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id, []string{media.Id})
	createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

	mockBucket.SignedURL = "https://storage.example.com/community-photo.jpg"

	req := httptest.NewRequest(http.MethodGet, "/go/"+invitation.ShortCode, nil)
	w := httptest.NewRecorder()

	svc.HandleInvitePage(w, req)

	body := w.Body.String()

	if !strings.Contains(body, mockBucket.SignedURL) {
		t.Errorf("Expected presigned image URL %q in response body", mockBucket.SignedURL)
	}
	// Should NOT contain default logo as the OG image.
	defaultImage := "https://test.example.com/assets/logo.png"
	if strings.Contains(body, `og:image" content="`+defaultImage) {
		t.Error("Expected community image URL, not default logo, in og:image tag")
	}
}

func TestHandleInvitePage_InvalidCode(t *testing.T) {
	svc, _, _ := setupTestService(t)

	req := httptest.NewRequest(http.MethodGet, "/go/NONEXIST", nil)
	w := httptest.NewRecorder()

	svc.HandleInvitePage(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 (render error page), got %d", resp.StatusCode)
	}

	body := w.Body.String()

	if !strings.Contains(body, "Invalid Invitation") {
		t.Error("Expected 'Invalid Invitation' heading in response")
	}
	if !strings.Contains(body, "not valid") {
		t.Error("Expected error message about invalid link in response")
	}
}

func TestHandleInvitePage_RevokedInvitation(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	inviter := createTestUser(t, ctx, sqlStorage, "Carol")
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id, nil)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, true)

	req := httptest.NewRequest(http.MethodGet, "/go/"+invitation.ShortCode, nil)
	w := httptest.NewRecorder()

	svc.HandleInvitePage(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 (render error page), got %d", resp.StatusCode)
	}

	body := w.Body.String()

	if !strings.Contains(body, "Invalid Invitation") {
		t.Error("Expected 'Invalid Invitation' heading for revoked invitation")
	}
	if !strings.Contains(body, "revoked") {
		t.Error("Expected revocation message in response")
	}
}

func TestHandleInvitePage_EmptyShortCode(t *testing.T) {
	svc, _, _ := setupTestService(t)

	req := httptest.NewRequest(http.MethodGet, "/go/", nil)
	w := httptest.NewRecorder()

	svc.HandleInvitePage(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 (render error page), got %d", resp.StatusCode)
	}

	body := w.Body.String()

	if !strings.Contains(body, "Invalid Invitation") {
		t.Error("Expected 'Invalid Invitation' heading for empty short code")
	}
	if !strings.Contains(body, "No invitation code provided") {
		t.Error("Expected error message about missing code in response")
	}
}

func TestServeStaticAssets_CSS(t *testing.T) {
	svc, _, _ := setupTestService(t)

	handler := svc.ServeStaticAssets()

	req := httptest.NewRequest(http.MethodGet, "/css/shared.css", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 for shared.css, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Ripls Website") {
		t.Error("Expected shared.css content in response")
	}
}

// expectedAASAPaths is the deliberately narrow allowlist of URL paths
// the iOS app claims via Universal Links. It MUST stay narrow:
// widening to /* would steal every link from the browser when the
// app is installed, including the web bundle's own routes
// (/feed, /community/{id}, /gear/{id}, etc.). Anchoring this list to
// a test makes a silent expansion fail CI.
//
// Equivalent guard for Android lives in expectedAssetLinksPaths
// implicitly — `delegate_permission/common.handle_all_urls` is
// scoped by the domain only, not per-path, so there's no path
// allowlist to pin. The exact relation + package_name list is
// asserted in TestAssetLinks_PinnedShape.
var expectedAASAPaths = []string{
	"/go/*",
	"/invite",
	"/invite/*",
	"/reset-password",
	"/reset-password/*",
}

func TestAssetLinks_PinnedShape(t *testing.T) {
	svc, _, _ := setupTestService(t)

	handler := svc.ServeWellKnown()

	req := httptest.NewRequest(http.MethodGet, "/.well-known/assetlinks.json", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 for assetlinks.json, got %d", resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	// Parse and assert the exact shape, not just substring containment.
	// assetlinks.json is an array of statements, each declaring a
	// relation + target. Widening either field accidentally is the
	// failure mode this test pins down.
	type target struct {
		Namespace              string   `json:"namespace"`
		PackageName            string   `json:"package_name"`
		SHA256CertFingerprints []string `json:"sha256_cert_fingerprints"`
	}
	type statement struct {
		Relation []string `json:"relation"`
		Target   target   `json:"target"`
	}

	var statements []statement
	if err := json.Unmarshal(w.Body.Bytes(), &statements); err != nil {
		t.Fatalf("failed to parse assetlinks.json: %v", err)
	}

	if len(statements) == 0 {
		t.Fatal("assetlinks.json must declare at least one statement")
	}

	// Each statement must declare exactly the
	// handle_all_urls relation and an android_app target. The
	// list of package names is the *only* part allowed to vary
	// (dev vs prod variants).
	wantRelation := []string{"delegate_permission/common.handle_all_urls"}
	allowedPackages := map[string]bool{
		"org.ripls.app":     true,
		"org.ripls.app.dev": true,
	}
	seenPackages := map[string]bool{}
	for i, stmt := range statements {
		if !reflect.DeepEqual(stmt.Relation, wantRelation) {
			t.Errorf("statement[%d].relation = %v, want %v "+
				"(adding new relations changes the Universal Link "+
				"contract — pin the test before changing this)",
				i, stmt.Relation, wantRelation)
		}
		if stmt.Target.Namespace != "android_app" {
			t.Errorf("statement[%d].target.namespace = %q, want android_app",
				i, stmt.Target.Namespace)
		}
		if !allowedPackages[stmt.Target.PackageName] {
			t.Errorf("statement[%d].target.package_name = %q, "+
				"not in allowlist %v — file an issue before adding "+
				"a new app variant to the Universal Link claim list",
				i, stmt.Target.PackageName, allowedPackages)
		}
		if len(stmt.Target.SHA256CertFingerprints) == 0 {
			t.Errorf("statement[%d].target.sha256_cert_fingerprints is empty; "+
				"the cert fingerprint pins the app identity for App Links",
				i)
		}
		seenPackages[stmt.Target.PackageName] = true
	}
	if !seenPackages["org.ripls.app"] {
		t.Error("assetlinks.json must claim org.ripls.app (prod variant)")
	}
}

func TestAASA_PinnedShape(t *testing.T) {
	svc, _, _ := setupTestService(t)

	handler := svc.ServeWellKnown()

	req := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 for apple-app-site-association, got %d", resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type application/json for apple-app-site-association, got %s", contentType)
	}

	// AASA structure:
	// {
	//   "applinks": {
	//     "apps": [],
	//     "details": [
	//       { "appID": "...", "paths": ["/go/*", ...] }
	//     ]
	//   }
	// }
	type detail struct {
		AppID string   `json:"appID"`
		Paths []string `json:"paths"`
	}
	type applinks struct {
		Apps    []string `json:"apps"`
		Details []detail `json:"details"`
	}
	type aasa struct {
		Applinks applinks `json:"applinks"`
	}

	var parsed aasa
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse apple-app-site-association: %v", err)
	}

	if len(parsed.Applinks.Details) == 0 {
		t.Fatal("AASA must declare at least one applinks.details entry")
	}

	for i, d := range parsed.Applinks.Details {
		if d.AppID == "" {
			t.Errorf("details[%d].appID is empty", i)
		}
		// The paths list is the gate that decides whether a tap
		// from outside the browser opens the native app vs the
		// browser. Adding paths here silently steals links from the
		// web bundle — pin the exact list.
		if !reflect.DeepEqual(d.Paths, expectedAASAPaths) {
			t.Errorf("details[%d].paths = %v, want %v\n"+
				"\n"+
				"Widening this list is dangerous. The web bundle is\n"+
				"served at the apex domain and owns /feed, /community/{id},\n"+
				"/gear/{id}, etc. If you add any of those paths to\n"+
				"applinks.details.paths, iOS will intercept the browser\n"+
				"tap and open the native app instead — stranding users\n"+
				"in app surfaces they didn't ask for and breaking the\n"+
				"web bundle. If you genuinely need to add a path, update\n"+
				"expectedAASAPaths in this file along with the change,\n"+
				"and document why the new path should leave the browser.",
				i, d.Paths, expectedAASAPaths)
		}
	}
}

func TestServeStaticAssets_Logo(t *testing.T) {
	svc, _, _ := setupTestService(t)

	handler := svc.ServeStaticAssets()

	req := httptest.NewRequest(http.MethodGet, "/assets/logo.png", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 for logo.png, got %d", resp.StatusCode)
	}

	if resp.ContentLength == 0 {
		t.Error("Expected non-empty content for logo.png")
	}
}

// TestServeStaticAssets_FlutterBundleFallthrough verifies that
// requests under /assets/* that aren't in the marketing-content embed
// fall through to the Flutter Web bundle. Without the fallthrough,
// the mux's longest-prefix dispatch sends every /assets/* request to
// the marketing-content handler (which 404s on bundle assets like
// FontManifest.json and fonts/MaterialIcons-Regular.otf), and the
// Flutter engine renders every icon as a tofu missing-glyph box.
//
// Skipped when no Flutter Web bundle has been embedded yet (fresh
// checkout state). Run `npm run build:web` first to enable it.
func TestServeStaticAssets_FlutterBundleFallthrough(t *testing.T) {
	if !bundleAvailable() {
		t.Skip("no Flutter Web bundle present in app_assets/; run `npm run build:web` to enable this test")
	}

	svc, _, _ := setupTestService(t)
	handler := svc.ServeStaticAssets()

	// The Flutter Web bundle emits these at /assets/*. They live in
	// `server/services/web/app_assets/assets/`, not in the marketing
	// embed at `website/content/assets/`. The fallthrough is what
	// makes them resolve.
	for _, target := range []string{
		"/assets/FontManifest.json",
		"/assets/AssetManifest.bin.json",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("%s returned %d, want 200 (Flutter bundle asset must resolve via fallthrough)",
				target, w.Code)
		}
	}

	// And the marketing-content path is still served, not shadowed.
	req := httptest.NewRequest(http.MethodGet, "/assets/logo.png", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("/assets/logo.png returned %d, want 200 "+
			"(marketing asset must still resolve)", w.Code)
	}
}
