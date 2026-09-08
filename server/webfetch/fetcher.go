// Package webfetch provides web content fetching for AI processing.
// This package handles HTTP fetching and HTML cleaning only.
// Event extraction is handled by the AI provider.
package webfetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"go.ripls.org/ripls/server/safehttp"
)

// FetchErrorKind classifies the type of fetch failure.
type FetchErrorKind int

const (
	// FetchErrorBlocked indicates the site returned 403 Forbidden.
	FetchErrorBlocked FetchErrorKind = iota
	// FetchErrorNotFound indicates the page was not found (404).
	FetchErrorNotFound
	// FetchErrorTimeout indicates the request timed out or the context deadline was exceeded.
	FetchErrorTimeout
	// FetchErrorConnection indicates a network-level connection failure.
	FetchErrorConnection
	// FetchErrorBadStatus indicates an unexpected HTTP status code.
	FetchErrorBadStatus
	// FetchErrorInvalidURL indicates the URL is malformed or has an unsupported scheme.
	FetchErrorInvalidURL
	// FetchErrorParse indicates HTML parsing failed.
	FetchErrorParse
)

// FetchError is a typed error from FetchPageContent that classifies the failure mode.
type FetchError struct {
	Kind       FetchErrorKind
	StatusCode int    // HTTP status code, if applicable.
	Message    string // Technical message for logging.
	Err        error  // Underlying error, if any.
}

// Error implements the error interface.
func (e *FetchError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error.
func (e *FetchError) Unwrap() error {
	return e.Err
}

const (
	// DefaultTimeout is the default HTTP request timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxBodySize is the maximum response body size (5MB).
	DefaultMaxBodySize = 5 * 1024 * 1024

	// DefaultMaxBodyTextLength is the maximum body text length for LLM context.
	DefaultMaxBodyTextLength = 8000

	// DefaultUserAgent mimics a standard browser to avoid bot-detection blocks.
	DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

	// schemeHTTP and schemeHTTPS are the only URL schemes this package will
	// fetch or follow a redirect to. Anything else (file:, gopher:, data:) is
	// an SSRF vector — see docs/server/conventions.md §Security: Outbound HTTP.
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// Fetcher fetches web content for AI processing.
type Fetcher interface {
	// FetchPageContent retrieves a webpage and extracts text content.
	FetchPageContent(ctx context.Context, url string) (*PageContent, error)
}

// PageContent represents extracted webpage content for AI processing.
type PageContent struct {
	URL         string               // Canonical URL (after redirects)
	Title       string               // <title> or og:title
	Description string               // meta description or og:description
	BodyText    string               // Cleaned body text (HTML stripped, truncated for LLM context)
	ImageURL    string               // Primary image URL (same as ImageURLCandidates[0] when non-empty)
	ImageURLs   []string             // Priority-ordered image candidates (primary + alternates), deduped
	Event       *StructuredEventData // Structured event data from JSON-LD (nil if not found)
}

// StructuredEventData holds machine-readable event information extracted from
// schema.org JSON-LD markup. When present, these fields are authoritative and
// should be preferred over LLM-extracted dates.
type StructuredEventData struct {
	Name         string     // schema.org Event name
	StartDate    *time.Time // Parsed startDate (nil if missing or unparseable)
	EndDate      *time.Time // Parsed endDate (nil if missing or unparseable)
	Location     string     // Combined location string for geocoding (e.g., "Boulder Airport, 3393 Airport Rd, Boulder, CO")
	LocationName string     // Place name only (e.g., "Boulder Airport"), empty if location is a plain string
	Latitude     *float64   // geo.latitude from JSON-LD Place (nil if not present)
	Longitude    *float64   // geo.longitude from JSON-LD Place (nil if not present)
}

// HTTPFetcher implements Fetcher using net/http.
type HTTPFetcher struct {
	client            *http.Client
	userAgent         string
	maxBodySize       int64
	maxBodyTextLength int
	timeout           time.Duration // stored during option processing; used to build client
	skipIPPreCheck    bool          // suppresses IP-literal pre-check; tests only
}

// Option configures an HTTPFetcher.
type Option func(*HTTPFetcher)

// WithTimeout sets the HTTP client timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(f *HTTPFetcher) {
		f.timeout = timeout
	}
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(f *HTTPFetcher) {
		f.userAgent = ua
	}
}

// WithMaxBodySize sets the maximum response body size.
func WithMaxBodySize(size int64) Option {
	return func(f *HTTPFetcher) {
		f.maxBodySize = size
	}
}

// WithMaxBodyTextLength sets the maximum body text length for LLM context.
func WithMaxBodyTextLength(length int) Option {
	return func(f *HTTPFetcher) {
		f.maxBodyTextLength = length
	}
}

// NewHTTPFetcher creates a new HTTPFetcher with the given options.
// The underlying HTTP client is SSRF-guarded via safehttp.NewClient;
// it rejects user-supplied URLs that resolve to private, loopback, or
// link-local addresses. For tests that must reach loopback httptest
// servers, use the package-internal newTestFetcher helper.
func NewHTTPFetcher(opts ...Option) *HTTPFetcher {
	f := &HTTPFetcher{
		userAgent:         DefaultUserAgent,
		maxBodySize:       DefaultMaxBodySize,
		maxBodyTextLength: DefaultMaxBodyTextLength,
		timeout:           DefaultTimeout,
	}

	for _, opt := range opts {
		opt(f)
	}

	jar, _ := cookiejar.New(nil)
	f.client = safehttp.NewClient(
		safehttp.WithTimeout(f.timeout),
		safehttp.WithRedirectSchemes(schemeHTTP, schemeHTTPS),
		safehttp.WithCookieJar(jar),
	)

	return f
}

// FetchPageContent retrieves a webpage and extracts text content.
// Errors are always of type *FetchError with a Kind field classifying the failure.
func (f *HTTPFetcher) FetchPageContent(ctx context.Context, rawURL string) (*PageContent, error) {
	// Validate URL.
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, &FetchError{Kind: FetchErrorInvalidURL, Message: "invalid URL", Err: err}
	}

	if parsedURL.Scheme != schemeHTTP && parsedURL.Scheme != schemeHTTPS {
		return nil, &FetchError{Kind: FetchErrorInvalidURL, Message: fmt.Sprintf("invalid URL scheme: %s (must be http or https)", parsedURL.Scheme)}
	}

	if parsedURL.Host == "" {
		return nil, &FetchError{Kind: FetchErrorInvalidURL, Message: "invalid URL: missing host"}
	}

	// Fail fast for IP-literal hosts in non-public ranges. The safe-outbound
	// dialer also enforces this at connect time (catching DNS rebinding), but
	// checking here gives a precise FetchErrorInvalidURL before any dial and
	// before the context deadline is consumed.
	// skipIPPreCheck is set only by test helpers that bypass the dialer guard
	// so they can reach loopback httptest servers.
	if !f.skipIPPreCheck {
		if host := parsedURL.Hostname(); !safehttp.IsHostPubliclyRoutable(host) {
			return nil, &FetchError{
				Kind:    FetchErrorInvalidURL,
				Message: fmt.Sprintf("URL host %s is not publicly routable", host),
			}
		}
	}

	// Create request.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, &FetchError{Kind: FetchErrorConnection, Message: "creating request", Err: err}
	}

	// Set browser-like headers to avoid bot-detection blocks.
	// Note: Accept-Encoding is NOT set explicitly so Go's Transport handles
	// gzip compression and decompression automatically.
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	// Execute request.
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, classifyRequestError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyStatusCode(resp.StatusCode)
	}

	// Limit body size to prevent abuse.
	limitedReader := io.LimitReader(resp.Body, f.maxBodySize)

	// Parse HTML.
	doc, err := goquery.NewDocumentFromReader(limitedReader)
	if err != nil {
		return nil, &FetchError{Kind: FetchErrorParse, Message: "parsing HTML", Err: err}
	}

	content := &PageContent{
		URL: resp.Request.URL.String(), // Canonical URL after redirects
	}

	// Extract title
	content.Title = f.extractTitle(doc)

	// Extract description
	content.Description = f.extractDescription(doc)

	// Extract body text
	content.BodyText = f.extractBodyText(doc)

	// Extract image candidates. ImageURL is the first entry for
	// backwards compatibility; ImageURLs carries the full list for
	// callers that surface alternates to the user.
	content.ImageURLs = f.extractImageURLCandidates(doc, resp.Request.URL)
	if len(content.ImageURLs) > 0 {
		content.ImageURL = content.ImageURLs[0]
	}

	// Extract structured event data from JSON-LD markup.
	content.Event = extractJSONLDEvent(doc)

	return content, nil
}

// extractTitle extracts the page title from og:title or <title> tag.
func (f *HTTPFetcher) extractTitle(doc *goquery.Document) string {
	// Try og:title first (often cleaner for event pages)
	if ogTitle, exists := doc.Find(`meta[property="og:title"]`).Attr("content"); exists && ogTitle != "" {
		return strings.TrimSpace(ogTitle)
	}

	// Fall back to <title>
	return strings.TrimSpace(doc.Find("title").First().Text())
}

// extractDescription extracts the page description from meta tags.
func (f *HTTPFetcher) extractDescription(doc *goquery.Document) string {
	// Try og:description first
	if ogDesc, exists := doc.Find(`meta[property="og:description"]`).Attr("content"); exists && ogDesc != "" {
		return strings.TrimSpace(ogDesc)
	}

	// Fall back to meta description
	if desc, exists := doc.Find(`meta[name="description"]`).Attr("content"); exists && desc != "" {
		return strings.TrimSpace(desc)
	}

	return ""
}

// extractBodyText extracts cleaned text content from the page body.
func (f *HTTPFetcher) extractBodyText(doc *goquery.Document) string {
	// Clone the document to avoid modifying the original
	docCopy := doc.Clone()

	// Remove script, style, and other non-content elements
	docCopy.Find("script, style, noscript, iframe, nav, footer, header, aside, [role=navigation], [role=banner], [aria-hidden=true]").Remove()

	var text string

	// Try to get text from main content areas first (more focused content)
	mainContent := docCopy.Find("main, article, [role=main], .content, .event-details, .event-description, .event-info")
	if mainContent.Length() > 0 {
		text = cleanText(mainContent.Text())
	} else {
		// Fall back to body text
		text = cleanText(docCopy.Find("body").Text())
	}

	// Truncate to max length for LLM context
	if len(text) > f.maxBodyTextLength {
		text = text[:f.maxBodyTextLength] + "..."
	}

	return text
}

// cleanText normalizes whitespace and trims the text.
func cleanText(text string) string {
	// Replace multiple whitespace characters with single space
	var builder strings.Builder
	lastWasSpace := true // Start true to trim leading space

	for _, r := range text {
		isSpace := r == ' ' || r == '\t' || r == '\n' || r == '\r'
		if isSpace {
			if !lastWasSpace {
				builder.WriteRune(' ')
				lastWasSpace = true
			}
		} else {
			builder.WriteRune(r)
			lastWasSpace = false
		}
	}

	result := builder.String()
	return strings.TrimSpace(result)
}

// extractImageURL extracts the primary image URL from meta tags or content.
// Equivalent to the first entry returned by extractImageURLCandidates;
// returns the empty string when the page produced no usable images. See
// extractImageURLCandidates for the priority order.
func (f *HTTPFetcher) extractImageURL(doc *goquery.Document, baseURL *url.URL) string {
	candidates := f.extractImageURLCandidates(doc, baseURL)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

// extractImageURLCandidates returns a priority-ordered, deduplicated list of
// image URLs extracted from the page. The first entry is the primary image
// (same as extractImageURL); subsequent entries are alternates suitable for
// surfacing as one-tap replacements.
//
// Priority: schema.org Product.image (JSON-LD) → Amazon data-old-hires /
// data-a-dynamic-image → og:image → twitter:image → main-content <img>
// srcset/src.
//
// JSON-LD takes precedence because e-commerce pages typically reference the
// high-resolution master there. Amazon strips standard metadata from its
// SSR'd product HTML, so the Amazon-specific attribute path is the only
// way to surface its image alternates without running JS.
func (f *HTTPFetcher) extractImageURLCandidates(doc *goquery.Document, baseURL *url.URL) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(raw string) {
		if raw == "" {
			return
		}
		resolved := resolveImageURL(raw, baseURL)
		if resolved == "" {
			return
		}
		key := strings.ToLower(resolved)
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, resolved)
	}

	for _, img := range extractJSONLDProductImages(doc) {
		add(img)
	}

	for _, img := range extractAmazonProductImages(doc) {
		add(img)
	}

	if ogImage, exists := doc.Find(`meta[property="og:image"]`).Attr("content"); exists {
		add(ogImage)
	}

	if twitterImage, exists := doc.Find(`meta[name="twitter:image"]`).Attr("content"); exists {
		add(twitterImage)
	}

	mainContent := doc.Find("main, article, [role=main], .content, .event-details")
	if mainContent.Length() == 0 {
		mainContent = doc.Find("body")
	}
	mainContent.Find("img").Each(func(_ int, sel *goquery.Selection) {
		src, _ := sel.Attr("src")
		srcset, _ := sel.Attr("srcset")
		if src == "" && srcset == "" {
			return
		}
		if strings.HasPrefix(src, "data:") {
			src = ""
		}
		// Skip tiny images (likely icons/spacers).
		if width, exists := sel.Attr("width"); exists {
			if w := parseIntAttr(width); w > 0 && w < 100 {
				return
			}
		}
		if height, exists := sel.Attr("height"); exists {
			if h := parseIntAttr(height); h > 0 && h < 100 {
				return
			}
		}
		if srcset != "" {
			if best := pickBestSrcsetURL(srcset); best != "" {
				add(best)
			}
		}
		if src != "" {
			add(src)
		}
	})

	return out
}

// resolveImageURL resolves a potentially relative URL, validates the scheme,
// and rewrites known e-commerce CDN URLs so they reference the high-resolution
// master variant rather than the small link-preview thumbnail. Returns the
// empty string when the URL is invalid or has an unsupported scheme.
func resolveImageURL(rawURL string, baseURL *url.URL) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	// Parse the URL
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	// Resolve relative URLs against base
	resolved := baseURL.ResolveReference(parsed)

	// Only allow http/https schemes
	if resolved.Scheme != schemeHTTP && resolved.Scheme != schemeHTTPS {
		return ""
	}

	// Upgrade plain http:// to https://. Pages occasionally emit
	// http-scheme og:image / JSON-LD entries even when the host serves
	// the same path over https; downstream importers (AddMediaFromURL)
	// reject non-https for security, so leaving http here would just
	// produce 400s on tap. Only upgrade when the base page is itself
	// https (so test fixtures and genuinely http-only sites aren't
	// silently rewritten).
	if resolved.Scheme == schemeHTTP && baseURL.Scheme == schemeHTTPS {
		resolved.Scheme = schemeHTTPS
	}

	// Upgrade known e-commerce CDN URLs to their high-resolution master.
	return upgradeImageURL(resolved).String()
}

// extractJSONLDEvent scans the document for schema.org Event JSON-LD markup
// and returns structured event data if found. Returns nil if no valid Event
// markup is present.
func extractJSONLDEvent(doc *goquery.Document) *StructuredEventData {
	var event *StructuredEventData

	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		raw := strings.TrimSpace(sel.Text())
		if raw == "" {
			return true // continue
		}

		// Try parsing as a single object first.
		parsed := parseJSONLDEvent(raw)
		if parsed != nil {
			event = parsed
			return false // break — use the first valid Event found
		}

		// Try parsing as an array of objects (some sites wrap JSON-LD in an array).
		var arr []json.RawMessage
		if json.Unmarshal([]byte(raw), &arr) == nil {
			for _, item := range arr {
				parsed = parseJSONLDEvent(string(item))
				if parsed != nil {
					event = parsed
					return false // break
				}
			}
		}

		return true // continue
	})

	return event
}

// jsonLDObject represents a generic JSON-LD object for type detection.
type jsonLDObject struct {
	Type     string          `json:"@type"`
	Name     string          `json:"name"`
	Start    string          `json:"startDate"`
	End      string          `json:"endDate"`
	Location json.RawMessage `json:"location"`
	Graph    []jsonLDObject  `json:"@graph"`
}

// parseJSONLDEvent attempts to parse a JSON string as a schema.org Event.
// Returns nil if the JSON is invalid or not an Event type.
func parseJSONLDEvent(raw string) *StructuredEventData {
	var obj jsonLDObject
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil
	}

	// Check for @graph wrapper (WordPress, some CMSes).
	if len(obj.Graph) > 0 {
		for _, item := range obj.Graph {
			if isEventType(item.Type) {
				return buildEventData(&item)
			}
		}
		return nil
	}

	if !isEventType(obj.Type) {
		return nil
	}

	return buildEventData(&obj)
}

// isEventType checks if a @type value represents a schema.org Event.
func isEventType(t string) bool {
	lower := strings.ToLower(t)
	return lower == "event" ||
		lower == "socialevent" ||
		lower == "musicalevent" ||
		lower == "sportsevent" ||
		lower == "educationevent" ||
		lower == "businessevent" ||
		lower == "festival" ||
		strings.HasSuffix(lower, "event")
}

// buildEventData constructs StructuredEventData from a parsed JSON-LD object.
func buildEventData(obj *jsonLDObject) *StructuredEventData {
	event := &StructuredEventData{
		Name: obj.Name,
	}

	// Parse startDate.
	if obj.Start != "" {
		if t := parseISO8601(obj.Start); t != nil {
			event.StartDate = t
		}
	}

	// Parse endDate.
	if obj.End != "" {
		if t := parseISO8601(obj.End); t != nil {
			event.EndDate = t
		}
	}

	// Parse location.
	event.Location = extractJSONLDLocation(obj.Location)
	event.LocationName = extractJSONLDLocationName(obj.Location)

	// Extract geo coordinates from location Place object.
	event.Latitude, event.Longitude = extractJSONLDGeo(obj.Location)

	return event
}

// parseISO8601 parses an ISO 8601 datetime string. Handles formats commonly
// found in JSON-LD: full RFC3339, date-only, and datetime without timezone.
func parseISO8601(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// Try RFC3339 (full datetime with timezone offset).
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}

	// Try datetime without timezone (assume UTC).
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return &t
	}

	// Try datetime with space separator.
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return &t
	}

	// Try date only.
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t
	}

	return nil
}

// extractJSONLDLocation extracts a location string from a JSON-LD location field.
// The location can be a string, a Place object with a name, or a Place with an address.
func extractJSONLDLocation(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try as a plain string.
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return s
	}

	// Try as a Place object.
	var place struct {
		Name    string `json:"name"`
		Address any    `json:"address"`
	}
	if json.Unmarshal(raw, &place) != nil {
		return ""
	}

	// Use name if available.
	if place.Name != "" {
		// Try to append address string.
		if addrStr, ok := place.Address.(string); ok && addrStr != "" {
			return place.Name + ", " + addrStr
		}

		// Try address as PostalAddress object.
		if addrMap, ok := place.Address.(map[string]any); ok {
			parts := []string{}
			for _, key := range []string{"streetAddress", "addressLocality", "addressRegion"} {
				if v, exists := addrMap[key]; exists {
					if str, ok := v.(string); ok && str != "" {
						parts = append(parts, str)
					}
				}
			}
			if len(parts) > 0 {
				return place.Name + ", " + strings.Join(parts, ", ")
			}
		}

		return place.Name
	}

	return ""
}

// extractJSONLDLocationName extracts just the Place name from a JSON-LD location field.
// Returns empty string if the location is a plain string or has no Place name.
func extractJSONLDLocationName(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var place struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &place) != nil {
		return ""
	}

	return place.Name
}

// extractJSONLDGeo extracts geo coordinates from a JSON-LD location field.
// Returns (lat, lng) pointers; both nil if no geo data is present.
func extractJSONLDGeo(raw json.RawMessage) (*float64, *float64) {
	if len(raw) == 0 {
		return nil, nil
	}

	var place struct {
		Geo struct {
			Latitude  any `json:"latitude"`
			Longitude any `json:"longitude"`
		} `json:"geo"`
	}
	if json.Unmarshal(raw, &place) != nil {
		return nil, nil
	}

	lat := parseGeoCoord(place.Geo.Latitude)
	lng := parseGeoCoord(place.Geo.Longitude)
	if lat == nil || lng == nil {
		return nil, nil
	}

	return lat, lng
}

// parseGeoCoord parses a geo coordinate from a JSON value.
// JSON-LD coordinates can be numbers or strings (e.g., "40.071939559156").
func parseGeoCoord(v any) *float64 {
	switch val := v.(type) {
	case float64:
		return &val
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return &f
		}
	}
	return nil
}

// parseIntAttr parses an integer attribute value, handling common formats.
// Returns 0 if parsing fails.
func parseIntAttr(val string) int {
	// Remove common suffixes like "px"
	val = strings.TrimSuffix(strings.TrimSpace(val), "px")
	var result int
	_, _ = fmt.Sscanf(val, "%d", &result)
	return result
}

// classifyRequestError maps a net/http request error to a typed FetchError.
func classifyRequestError(err error) *FetchError {
	// SSRF guard rejection takes precedence; map to InvalidURL so callers
	// can return CodeInvalidArgument rather than the generic CodeUnavailable.
	if safehttp.IsSSRFBlockError(err) {
		return &FetchError{Kind: FetchErrorInvalidURL, Message: "URL resolves to a non-publicly-routable address", Err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &FetchError{Kind: FetchErrorTimeout, Message: "request timed out", Err: err}
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return &FetchError{Kind: FetchErrorTimeout, Message: "request timed out", Err: err}
	}
	return &FetchError{Kind: FetchErrorConnection, Message: "connection failed", Err: err}
}

// classifyStatusCode maps an HTTP status code to a typed FetchError.
func classifyStatusCode(code int) *FetchError {
	switch {
	case code == http.StatusForbidden:
		return &FetchError{Kind: FetchErrorBlocked, StatusCode: code, Message: "site returned 403 Forbidden"}
	case code == http.StatusNotFound:
		return &FetchError{Kind: FetchErrorNotFound, StatusCode: code, Message: "page not found (404)"}
	case code == http.StatusTooManyRequests:
		return &FetchError{Kind: FetchErrorBlocked, StatusCode: code, Message: "rate limited (429)"}
	default:
		return &FetchError{Kind: FetchErrorBadStatus, StatusCode: code, Message: fmt.Sprintf("unexpected status code: %d", code)}
	}
}
