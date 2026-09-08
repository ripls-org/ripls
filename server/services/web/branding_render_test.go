package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/branding"
)

// The landing pages used to hard-code one deployment's identity: the product
// name in og:site_name, the app host the open-in-app logic compared against,
// the custom scheme, the store URLs, and the legal footer (#2953). These tests
// pin both directions — a configured instance renders its own values, and an
// unconfigured one renders nothing rather than the values that used to be
// compiled in.
//
// The invite error page is the surface under test because it needs no seeded
// data: any unresolvable short code renders it.
func renderInvitePage(t *testing.T, b branding.Config) string {
	t.Helper()
	svc, _, _ := setupTestService(t)
	svc.SetBranding(b)

	req := httptest.NewRequest(http.MethodGet, "/go/NOTREAL1", nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	body := w.Body.String()
	if body == "" {
		t.Fatal("invite page rendered empty")
	}
	return body
}

func TestLandingRendersConfiguredBranding(t *testing.T) {
	body := renderInvitePage(t, branding.Config{
		Name:             "Example App",
		AppBaseURL:       "https://app.example.com",
		LegalEntityName:  "Example Org, Inc.",
		PrivacyPolicyURL: "https://example.com/privacy",
		AppStoreURL:      "https://apps.apple.com/app/id123",
		PlayStoreURL:     "https://play.google.com/store/apps/details?id=com.example.app",
		DeepLinkScheme:   "exampleapp",
	})

	for _, want := range []string{
		`content="Example App"`,                                 // og:site_name
		"Example Org, Inc.",                                     // legal footer
		"https://example.com/privacy",                           // privacy link
		"https://apps.apple.com/app/id123",                      // iOS store link
		"play.google.com/store/apps/details?id=com.example.app", // Play link
		`const deepLinkScheme = "exampleapp"`,                   // scheme for the clipboard deep link
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}
}

func TestLandingOmitsUnconfiguredBranding(t *testing.T) {
	// Zero value: nothing configured. The page must still render, and must not
	// fall back to the identifiers that used to be literals in the template.
	body := renderInvitePage(t, branding.Config{})

	for _, unwanted := range []string{
		"testflight.apple.com",
		"play.google.com",
		"org.ripls.app",
		"Type2Fun",
		"example.com/privacy-policy",
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("unconfigured page leaked %q — it should render nothing for an unset value", unwanted)
		}
	}

	// The store-link container survives; only its contents are omitted, so the
	// page is still a coherent error card rather than a broken one.
	if !strings.Contains(body, `id="app-buttons"`) {
		t.Error("expected the page to still render its button container")
	}
}

func TestLandingStoreLinksAreIndependent(t *testing.T) {
	// A deployment published on only one platform must get that one link,
	// not both and not neither.
	body := renderInvitePage(t, branding.Config{
		Name:         "Example App",
		PlayStoreURL: "https://play.google.com/store/apps/details?id=com.example.app",
	})

	if !strings.Contains(body, "id=com.example.app") {
		t.Error("expected the configured Play listing to render")
	}
	if strings.Contains(body, "testflight.apple.com") || strings.Contains(body, "apps.apple.com") {
		t.Error("expected no iOS store link when AppStoreURL is unset")
	}
}
