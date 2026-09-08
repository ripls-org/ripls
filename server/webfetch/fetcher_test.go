package webfetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func TestHTTPFetcher_FetchPageContent(t *testing.T) {
	tests := []struct {
		name        string
		html        string
		wantTitle   string
		wantDesc    string
		wantBodySub string // substring that should appear in body
	}{
		{
			name: "basic page with title and meta",
			html: `<!DOCTYPE html>
<html>
<head>
	<title>Community Game Night</title>
	<meta name="description" content="Join us for board games and fun!">
</head>
<body>
	<main>
		<h1>Community Game Night</h1>
		<p>Every Friday at 7pm at the Community Center.</p>
		<p>Bring your favorite games!</p>
	</main>
</body>
</html>`,
			wantTitle:   "Community Game Night",
			wantDesc:    "Join us for board games and fun!",
			wantBodySub: "Every Friday at 7pm",
		},
		{
			name: "page with og tags",
			html: `<!DOCTYPE html>
<html>
<head>
	<title>Website Title</title>
	<meta property="og:title" content="Summer Music Festival 2025">
	<meta property="og:description" content="Three days of amazing music in the park">
	<meta name="description" content="Generic website description">
</head>
<body>
	<article>
		<h1>Summer Music Festival</h1>
		<p>July 15-17, 2025</p>
		<p>Central Park, New York</p>
	</article>
</body>
</html>`,
			wantTitle:   "Summer Music Festival 2025", // og:title preferred
			wantDesc:    "Three days of amazing music in the park",
			wantBodySub: "July 15-17, 2025",
		},
		{
			name: "page with script and style removal",
			html: `<!DOCTYPE html>
<html>
<head>
	<title>Yoga Class</title>
	<style>.hidden { display: none; }</style>
	<script>console.log("tracking");</script>
</head>
<body>
	<nav>Home | About | Contact</nav>
	<main>
		<p>Saturday morning yoga at 9am</p>
	</main>
	<footer>Copyright 2025</footer>
	<script>analytics.track();</script>
</body>
</html>`,
			wantTitle:   "Yoga Class",
			wantDesc:    "",
			wantBodySub: "Saturday morning yoga at 9am",
		},
		{
			name: "eventbrite-like page structure",
			html: `<!DOCTYPE html>
<html>
<head>
	<title>Tech Meetup - Eventbrite</title>
	<meta property="og:title" content="Tech Meetup: AI in 2025">
	<meta property="og:description" content="Learn about the latest AI trends">
</head>
<body>
	<header>Eventbrite Logo</header>
	<div class="event-details">
		<h1>Tech Meetup: AI in 2025</h1>
		<div class="event-info">
			<p>Date: January 30, 2025</p>
			<p>Time: 6:00 PM - 9:00 PM</p>
			<p>Location: TechHub Downtown, 123 Main St</p>
		</div>
		<div class="event-description">
			<p>Join us for an evening of talks and networking.</p>
		</div>
	</div>
	<footer>Powered by Eventbrite</footer>
</body>
</html>`,
			wantTitle:   "Tech Meetup: AI in 2025",
			wantDesc:    "Learn about the latest AI trends",
			wantBodySub: "January 30, 2025",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(tt.html))
			}))
			defer server.Close()

			// Create fetcher and fetch content
			fetcher := newTestFetcher()
			content, err := fetcher.FetchPageContent(context.Background(), server.URL)
			if err != nil {
				t.Fatalf("FetchPageContent failed: %v", err)
			}

			// Check title
			if content.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", content.Title, tt.wantTitle)
			}

			// Check description
			if content.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", content.Description, tt.wantDesc)
			}

			// Check body contains expected substring
			if !strings.Contains(content.BodyText, tt.wantBodySub) {
				t.Errorf("BodyText does not contain %q, got: %s", tt.wantBodySub, content.BodyText)
			}

			// Verify scripts are removed
			if strings.Contains(content.BodyText, "console.log") || strings.Contains(content.BodyText, "analytics") {
				t.Error("BodyText should not contain script content")
			}
		})
	}
}

func TestHTTPFetcher_URLValidation(t *testing.T) {
	fetcher := newTestFetcher()
	ctx := context.Background()

	tests := []struct {
		name     string
		url      string
		wantKind FetchErrorKind
	}{
		{
			name:     "invalid scheme",
			url:      "ftp://example.com/event",
			wantKind: FetchErrorInvalidURL,
		},
		{
			name:     "javascript scheme",
			url:      "javascript:alert(1)",
			wantKind: FetchErrorInvalidURL,
		},
		{
			name:     "missing host",
			url:      "https:///path",
			wantKind: FetchErrorInvalidURL,
		},
		{
			name:     "invalid URL",
			url:      "://not-a-url",
			wantKind: FetchErrorInvalidURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fetcher.FetchPageContent(ctx, tt.url)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var fetchErr *FetchError
			if !errors.As(err, &fetchErr) {
				t.Fatalf("expected *FetchError, got %T", err)
			}
			if fetchErr.Kind != tt.wantKind {
				t.Errorf("FetchError.Kind = %d, want %d", fetchErr.Kind, tt.wantKind)
			}
		})
	}
}

func TestHTTPFetcher_HTTPErrors(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		wantKind       FetchErrorKind
		wantStatusCode int
	}{
		{
			name:           "not found",
			statusCode:     http.StatusNotFound,
			wantKind:       FetchErrorNotFound,
			wantStatusCode: 404,
		},
		{
			name:           "server error",
			statusCode:     http.StatusInternalServerError,
			wantKind:       FetchErrorBadStatus,
			wantStatusCode: 500,
		},
		{
			name:           "forbidden",
			statusCode:     http.StatusForbidden,
			wantKind:       FetchErrorBlocked,
			wantStatusCode: 403,
		},
		{
			name:           "rate limited",
			statusCode:     http.StatusTooManyRequests,
			wantKind:       FetchErrorBlocked,
			wantStatusCode: 429,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			fetcher := newTestFetcher()
			_, err := fetcher.FetchPageContent(context.Background(), server.URL)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var fetchErr *FetchError
			if !errors.As(err, &fetchErr) {
				t.Fatalf("expected *FetchError, got %T", err)
			}
			if fetchErr.Kind != tt.wantKind {
				t.Errorf("FetchError.Kind = %d, want %d", fetchErr.Kind, tt.wantKind)
			}
			if fetchErr.StatusCode != tt.wantStatusCode {
				t.Errorf("FetchError.StatusCode = %d, want %d", fetchErr.StatusCode, tt.wantStatusCode)
			}
		})
	}
}

func TestHTTPFetcher_Timeout(t *testing.T) {
	// Create slow server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) //nolint:forbidigo // fake HTTP server deliberately blocks to trigger a timeout
		_, _ = w.Write([]byte("<html><body>slow</body></html>"))
	}))
	defer server.Close()

	// Create fetcher with short timeout.
	fetcher := newTestFetcher(WithTimeout(50 * time.Millisecond))

	_, err := fetcher.FetchPageContent(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	var fetchErr *FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected *FetchError, got %T", err)
	}
	if fetchErr.Kind != FetchErrorTimeout {
		t.Errorf("FetchError.Kind = %d, want FetchErrorTimeout (%d)", fetchErr.Kind, FetchErrorTimeout)
	}
}

func TestHTTPFetcher_BodyTextTruncation(t *testing.T) {
	// Create page with lots of text
	longText := strings.Repeat("This is a long paragraph of text. ", 500)
	html := "<html><body><main>" + longText + "</main></body></html>"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	// Use short max body text length
	fetcher := newTestFetcher(WithMaxBodyTextLength(100))
	content, err := fetcher.FetchPageContent(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchPageContent failed: %v", err)
	}

	// Check truncation
	if len(content.BodyText) > 110 { // 100 + "..."
		t.Errorf("BodyText length = %d, want <= 103", len(content.BodyText))
	}
	if !strings.HasSuffix(content.BodyText, "...") {
		t.Error("Truncated BodyText should end with ...")
	}
}

func TestHTTPFetcher_RedirectFollowing(t *testing.T) {
	// Create server that redirects
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><head><title>Final Page</title></head><body>Content</body></html>"))
	}))
	defer finalServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL, http.StatusMovedPermanently)
	}))
	defer redirectServer.Close()

	fetcher := newTestFetcher()
	content, err := fetcher.FetchPageContent(context.Background(), redirectServer.URL)
	if err != nil {
		t.Fatalf("FetchPageContent failed: %v", err)
	}

	// URL should be the final URL after redirect
	if content.URL != finalServer.URL {
		t.Errorf("URL = %q, want %q (should follow redirect)", content.URL, finalServer.URL)
	}

	if content.Title != "Final Page" {
		t.Errorf("Title = %q, want %q", content.Title, "Final Page")
	}
}

func TestHTTPFetcher_UserAgent(t *testing.T) {
	var receivedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("<html><body>test</body></html>"))
	}))
	defer server.Close()

	customUA := "CustomBot/2.0"
	fetcher := newTestFetcher(WithUserAgent(customUA))
	_, err := fetcher.FetchPageContent(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchPageContent failed: %v", err)
	}

	if receivedUA != customUA {
		t.Errorf("User-Agent = %q, want %q", receivedUA, customUA)
	}
}

func TestHTTPFetcher_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second) //nolint:forbidigo // fake HTTP server deliberately blocks to exercise context cancellation
		_, _ = w.Write([]byte("<html><body>slow</body></html>"))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	fetcher := newTestFetcher()
	_, err := fetcher.FetchPageContent(ctx, server.URL)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	var fetchErr *FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected *FetchError, got %T", err)
	}
	if fetchErr.Kind != FetchErrorTimeout {
		t.Errorf("FetchError.Kind = %d, want FetchErrorTimeout (%d)", fetchErr.Kind, FetchErrorTimeout)
	}
}

func TestHTTPFetcher_BrowserHeaders(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		_, _ = w.Write([]byte("<html><body>test</body></html>"))
	}))
	defer server.Close()

	fetcher := newTestFetcher()
	_, err := fetcher.FetchPageContent(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchPageContent failed: %v", err)
	}

	// Verify browser-like headers are sent.
	wantHeaders := map[string]string{
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
		"Sec-Fetch-User":            "?1",
		"Upgrade-Insecure-Requests": "1",
	}
	for header, wantValue := range wantHeaders {
		got := receivedHeaders.Get(header)
		if got != wantValue {
			t.Errorf("Header %q = %q, want %q", header, got, wantValue)
		}
	}

	// Verify default UA is browser-like (not bot-identifying).
	ua := receivedHeaders.Get("User-Agent")
	if strings.Contains(ua, "RiplsBot") {
		t.Errorf("User-Agent should not contain 'RiplsBot', got %q", ua)
	}
	if !strings.Contains(ua, "Mozilla") {
		t.Errorf("User-Agent should contain 'Mozilla', got %q", ua)
	}
}

func TestHTTPFetcher_ImageURLExtraction(t *testing.T) {
	tests := []struct {
		name         string
		html         string
		wantImageURL string
	}{
		{
			name: "og:image takes priority",
			html: `<!DOCTYPE html>
<html>
<head>
	<title>Event Page</title>
	<meta property="og:image" content="https://example.com/og-image.jpg">
	<meta name="twitter:image" content="https://example.com/twitter-image.jpg">
</head>
<body>
	<main>
		<img src="/content-image.jpg">
	</main>
</body>
</html>`,
			wantImageURL: "https://example.com/og-image.jpg",
		},
		{
			name: "twitter:image fallback when no og:image",
			html: `<!DOCTYPE html>
<html>
<head>
	<title>Event Page</title>
	<meta name="twitter:image" content="https://example.com/twitter-image.jpg">
</head>
<body>
	<main>
		<img src="/content-image.jpg">
	</main>
</body>
</html>`,
			wantImageURL: "https://example.com/twitter-image.jpg",
		},
		{
			name: "content image fallback when no meta tags",
			html: `<!DOCTYPE html>
<html>
<head><title>Event Page</title></head>
<body>
	<main>
		<img src="/content-image.jpg" width="400" height="300">
	</main>
</body>
</html>`,
			wantImageURL: "/content-image.jpg", // Will be resolved against server URL in test
		},
		{
			name: "skip small images",
			html: `<!DOCTYPE html>
<html>
<head><title>Event Page</title></head>
<body>
	<main>
		<img src="/icon.png" width="32" height="32">
		<img src="/spacer.gif" width="1" height="1">
		<img src="/real-image.jpg" width="400" height="300">
	</main>
</body>
</html>`,
			wantImageURL: "/real-image.jpg", // Will be resolved against server URL
		},
		{
			name: "skip data URIs",
			html: `<!DOCTYPE html>
<html>
<head><title>Event Page</title></head>
<body>
	<main>
		<img src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP">
		<img src="/real-image.jpg">
	</main>
</body>
</html>`,
			wantImageURL: "/real-image.jpg",
		},
		{
			name: "resolve relative URLs",
			html: `<!DOCTYPE html>
<html>
<head>
	<meta property="og:image" content="/images/event.jpg">
</head>
<body></body>
</html>`,
			wantImageURL: "/images/event.jpg", // Will be resolved against server URL
		},
		{
			name: "no image found",
			html: `<!DOCTYPE html>
<html>
<head><title>No Images</title></head>
<body>
	<main>
		<p>Just text content</p>
	</main>
</body>
</html>`,
			wantImageURL: "",
		},
		{
			name: "eventbrite-like page with og:image",
			html: `<!DOCTYPE html>
<html>
<head>
	<title>Tech Meetup - Eventbrite</title>
	<meta property="og:image" content="https://img.evbuc.com/event-image.jpg">
</head>
<body>
	<div class="event-details">
		<img src="/small-icon.png" width="50">
		<h1>Tech Meetup</h1>
	</div>
</body>
</html>`,
			wantImageURL: "https://img.evbuc.com/event-image.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(tt.html))
			}))
			defer server.Close()

			// Create fetcher and fetch content
			fetcher := newTestFetcher()
			content, err := fetcher.FetchPageContent(context.Background(), server.URL)
			if err != nil {
				t.Fatalf("FetchPageContent failed: %v", err)
			}

			// For relative URLs, check they were resolved to absolute
			wantURL := tt.wantImageURL
			if wantURL != "" && !strings.HasPrefix(wantURL, "http") {
				// Relative URL should be resolved to server URL
				wantURL = server.URL + tt.wantImageURL
			}

			if content.ImageURL != wantURL {
				t.Errorf("ImageURL = %q, want %q", content.ImageURL, wantURL)
			}
		})
	}
}

func TestHTTPFetcher_ImageURLCandidates(t *testing.T) {
	tests := []struct {
		name string
		html string
		// want is matched after relative URLs are resolved against the
		// test server URL. Entries that already start with "http" pass
		// through unchanged.
		want []string
	}{
		{
			name: "multiple sources dedupe and rank by priority",
			html: `<!DOCTYPE html>
<html>
<head>
	<meta property="og:image" content="https://example.com/og.jpg">
	<meta name="twitter:image" content="https://example.com/og.jpg">
	<meta name="twitter:image" content="https://example.com/twitter.jpg">
	<script type="application/ld+json">
	{"@type":"Product","image":["https://example.com/jsonld-small.jpg","https://example.com/jsonld-large.jpg"]}
	</script>
</head>
<body>
	<main>
		<img src="/content.jpg" width="400" height="300">
	</main>
</body>
</html>`,
			want: []string{
				// JSON-LD walks largest-last → reversed to largest-first.
				"https://example.com/jsonld-large.jpg",
				"https://example.com/jsonld-small.jpg",
				"https://example.com/og.jpg",
				"/content.jpg",
			},
		},
		{
			name: "amazon data-a-dynamic-image yields multiple candidates",
			html: `<!DOCTYPE html>
<html>
<head><title>Amazon Product</title></head>
<body>
	<img id="landingImage" data-old-hires="https://m.media-amazon.com/images/I/master._AC_SL1500_.jpg"
	     data-a-dynamic-image='{"https://m.media-amazon.com/images/I/v1._AC_SX522_.jpg":[522,522],"https://m.media-amazon.com/images/I/v2._AC_SX679_.jpg":[679,679],"https://m.media-amazon.com/images/I/v3._AC_SX300_.jpg":[300,300]}'>
</body>
</html>`,
			// resolveImageURL strips the Amazon `._XX_.` variant token to
			// upgrade each URL to the master version, so all dynamic-image
			// entries collapse to the same shape — only data-old-hires
			// and the three distinct dynamic-image bases survive after
			// dedup.
			want: []string{
				"https://m.media-amazon.com/images/I/master.jpg",
				"https://m.media-amazon.com/images/I/v2.jpg",
				"https://m.media-amazon.com/images/I/v1.jpg",
				"https://m.media-amazon.com/images/I/v3.jpg",
			},
		},
		{
			name: "no images yields empty list",
			html: `<!DOCTYPE html><html><head><title>Nothing</title></head><body><p>Hi</p></body></html>`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(tt.html))
			}))
			defer server.Close()

			resp, err := http.Get(server.URL)
			if err != nil {
				t.Fatalf("http.Get: %v", err)
			}
			defer resp.Body.Close()
			doc, err := goquery.NewDocumentFromReader(resp.Body)
			if err != nil {
				t.Fatalf("parse doc: %v", err)
			}
			base, _ := url.Parse(server.URL)
			got := NewHTTPFetcher().extractImageURLCandidates(doc, base)

			// Resolve relative URLs in `want` against the test server.
			want := make([]string, 0, len(tt.want))
			for _, u := range tt.want {
				if strings.HasPrefix(u, "http") {
					want = append(want, u)
				} else {
					want = append(want, server.URL+u)
				}
			}

			if len(got) != len(want) {
				t.Fatalf("got %d candidates %v, want %d %v", len(got), got, len(want), want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("candidate[%d] = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}

func TestResolveImageURL(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/events/page.html")

	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{
			name:   "absolute https URL",
			rawURL: "https://cdn.example.com/image.jpg",
			want:   "https://cdn.example.com/image.jpg",
		},
		{
			// resolveImageURL upgrades plain http to https so downstream
			// importers (AddMediaFromURL requires https) don't reject
			// candidate URLs that came from pages with inconsistent
			// scheme metadata.
			name:   "absolute http URL upgraded to https",
			rawURL: "http://cdn.example.com/image.jpg",
			want:   "https://cdn.example.com/image.jpg",
		},
		{
			name:   "relative URL with leading slash",
			rawURL: "/images/event.jpg",
			want:   "https://example.com/images/event.jpg",
		},
		{
			name:   "relative URL without leading slash",
			rawURL: "images/event.jpg",
			want:   "https://example.com/events/images/event.jpg",
		},
		{
			name:   "protocol-relative URL",
			rawURL: "//cdn.example.com/image.jpg",
			want:   "https://cdn.example.com/image.jpg",
		},
		{
			name:   "data URI rejected",
			rawURL: "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP",
			want:   "",
		},
		{
			name:   "javascript URI rejected",
			rawURL: "javascript:void(0)",
			want:   "",
		},
		{
			name:   "empty string",
			rawURL: "",
			want:   "",
		},
		{
			name:   "whitespace only",
			rawURL: "   ",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveImageURL(tt.rawURL, baseURL)
			if got != tt.want {
				t.Errorf("resolveImageURL(%q) = %q, want %q", tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestParseIntAttr(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "simple number",
			input: "400",
			want:  400,
		},
		{
			name:  "number with px suffix",
			input: "400px",
			want:  400,
		},
		{
			name:  "number with spaces",
			input: "  400  ",
			want:  400,
		},
		{
			name:  "zero",
			input: "0",
			want:  0,
		},
		{
			name:  "invalid string",
			input: "auto",
			want:  0,
		},
		{
			name:  "empty string",
			input: "",
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseIntAttr(tt.input)
			if got != tt.want {
				t.Errorf("parseIntAttr(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestCleanText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "multiple spaces",
			input: "hello    world",
			want:  "hello world",
		},
		{
			name:  "newlines and tabs",
			input: "hello\n\n\tworld",
			want:  "hello world",
		},
		{
			name:  "leading and trailing whitespace",
			input: "  \n  hello world  \t  ",
			want:  "hello world",
		},
		{
			name:  "mixed whitespace",
			input: "  Event:\n\n  Saturday  \t  at   9am  ",
			want:  "Event: Saturday at 9am",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanText(tt.input)
			if got != tt.want {
				t.Errorf("cleanText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestHTTPFetcher_SSRFGuard verifies that the safe-outbound client rejects
// user-supplied URLs that resolve to non-public IP addresses.
func TestHTTPFetcher_SSRFGuard(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{
			name: "loopback IP literal rejected by pre-check",
			url:  "http://127.0.0.1:12345/internal",
		},
		{
			name: "RFC1918 10/8 IP literal rejected by pre-check",
			url:  "http://10.0.0.1/secret",
		},
		{
			name: "RFC1918 192.168/16 IP literal rejected by pre-check",
			url:  "http://192.168.1.1/admin",
		},
		{
			name: "link-local (GCP metadata) IP literal rejected by pre-check",
			url:  "http://169.254.169.254/computeMetadata/v1/",
		},
		{
			name: "CGNAT IP literal rejected by pre-check",
			url:  "http://100.64.0.1/internal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the real NewHTTPFetcher (SSRF guard active).
			fetcher := NewHTTPFetcher()
			_, err := fetcher.FetchPageContent(context.Background(), tt.url)
			if err == nil {
				t.Fatalf("expected SSRF rejection for %q, got nil error", tt.url)
			}
			var fetchErr *FetchError
			if !errors.As(err, &fetchErr) {
				t.Fatalf("expected *FetchError, got %T: %v", err, err)
			}
			if fetchErr.Kind != FetchErrorInvalidURL {
				t.Errorf("FetchError.Kind = %v, want FetchErrorInvalidURL for %q", fetchErr.Kind, tt.url)
			}
		})
	}
}

// TestHTTPFetcher_SSRFGuard_DialerBlocksNonLiteralPrivateIP verifies that
// the dialer Control hook blocks connections to private IPs reached via
// hostname (e.g. "localhost"), catching cases the IP-literal pre-check
// cannot see.
func TestHTTPFetcher_SSRFGuard_DialerBlocksNonLiteralPrivateIP(t *testing.T) {
	// "localhost" is not a literal IP, so the pre-check passes. The
	// dialer resolves it to 127.0.0.1 and the Control hook rejects it.
	fetcher := NewHTTPFetcher()
	_, err := fetcher.FetchPageContent(context.Background(), "http://localhost:12345/")
	if err == nil {
		t.Fatal("expected SSRF rejection for localhost, got nil")
	}
	var fetchErr *FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected *FetchError, got %T: %v", err, err)
	}
	if fetchErr.Kind != FetchErrorInvalidURL {
		t.Errorf("FetchError.Kind = %v, want FetchErrorInvalidURL", fetchErr.Kind)
	}
}
