package webfetch

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// htmlDoc creates a goquery Document from an HTML string.
func htmlDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}
	return doc
}

func TestExtractJSONLDEvent_Eventbrite(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json">
		{
			"@context": "https://schema.org",
			"@type": "Event",
			"name": "Summer Jazz Festival 2026",
			"startDate": "2026-07-18T19:00:00-06:00",
			"endDate": "2026-07-18T23:00:00-06:00",
			"location": {
				"@type": "Place",
				"name": "Main Street Plaza",
				"address": {
					"@type": "PostalAddress",
					"streetAddress": "123 Main Street",
					"addressLocality": "Boulder",
					"addressRegion": "CO"
				}
			}
		}
		</script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected structured event data, got nil")
	}

	if event.Name != "Summer Jazz Festival 2026" {
		t.Errorf("Name = %q, want %q", event.Name, "Summer Jazz Festival 2026")
	}

	if event.StartDate == nil {
		t.Fatal("StartDate is nil")
	}

	expectedStart := time.Date(2026, 7, 18, 19, 0, 0, 0, time.FixedZone("", -6*60*60))
	if !event.StartDate.Equal(expectedStart) {
		t.Errorf("StartDate = %v, want %v", event.StartDate, expectedStart)
	}

	if event.EndDate == nil {
		t.Fatal("EndDate is nil")
	}

	if !strings.Contains(event.Location, "Main Street Plaza") {
		t.Errorf("Location = %q, want to contain %q", event.Location, "Main Street Plaza")
	}

	if !strings.Contains(event.Location, "Boulder") {
		t.Errorf("Location = %q, want to contain %q", event.Location, "Boulder")
	}
}

func TestExtractJSONLDEvent_Meetup(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json">
		{
			"@context": "https://schema.org",
			"@type": "SocialEvent",
			"name": "Boulder Trail Runners Monthly Run",
			"startDate": "2026-05-02T08:00:00",
			"location": {
				"@type": "Place",
				"name": "Chautauqua Park"
			}
		}
		</script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected structured event data")
	}

	if event.Name != "Boulder Trail Runners Monthly Run" {
		t.Errorf("Name = %q", event.Name)
	}

	if event.StartDate == nil {
		t.Fatal("StartDate is nil")
	}

	// No timezone in the input — parsed as UTC.
	expected := time.Date(2026, 5, 2, 8, 0, 0, 0, time.UTC)
	if !event.StartDate.Equal(expected) {
		t.Errorf("StartDate = %v, want %v", event.StartDate, expected)
	}

	if event.EndDate != nil {
		t.Errorf("EndDate should be nil, got %v", event.EndDate)
	}

	if event.Location != "Chautauqua Park" {
		t.Errorf("Location = %q, want %q", event.Location, "Chautauqua Park")
	}
}

func TestExtractJSONLDEvent_ArrayFormat(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json">
		[
			{"@type": "WebSite", "name": "My Site"},
			{
				"@type": "Event",
				"name": "Community Gathering",
				"startDate": "2026-06-15T18:00:00-04:00"
			}
		]
		</script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected structured event data from array")
	}

	if event.Name != "Community Gathering" {
		t.Errorf("Name = %q, want %q", event.Name, "Community Gathering")
	}

	if event.StartDate == nil {
		t.Fatal("StartDate is nil")
	}
}

func TestExtractJSONLDEvent_GraphWrapper(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json">
		{
			"@context": "https://schema.org",
			"@graph": [
				{"@type": "WebSite", "name": "My Site"},
				{
					"@type": "Event",
					"name": "Workshop",
					"startDate": "2026-08-10T14:00:00+00:00"
				}
			]
		}
		</script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected structured event data from @graph")
	}

	if event.Name != "Workshop" {
		t.Errorf("Name = %q, want %q", event.Name, "Workshop")
	}
}

func TestExtractJSONLDEvent_DateOnly(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json">
		{
			"@type": "Event",
			"name": "All Day Fair",
			"startDate": "2026-09-20"
		}
		</script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected structured event data")
	}

	if event.StartDate == nil {
		t.Fatal("StartDate is nil for date-only format")
	}

	expected := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	if !event.StartDate.Equal(expected) {
		t.Errorf("StartDate = %v, want %v", event.StartDate, expected)
	}
}

func TestExtractJSONLDEvent_LocationString(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json">
		{
			"@type": "Event",
			"name": "Concert",
			"startDate": "2026-07-01T20:00:00-05:00",
			"location": "Madison Square Garden, New York"
		}
		</script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected structured event data")
	}

	if event.Location != "Madison Square Garden, New York" {
		t.Errorf("Location = %q, want %q", event.Location, "Madison Square Garden, New York")
	}
}

func TestExtractJSONLDEvent_PostalAddress(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json">
		{
			"@type": "Event",
			"name": "Conference",
			"startDate": "2026-05-20T09:00:00-06:00",
			"location": {
				"@type": "Place",
				"name": "Convention Center",
				"address": {
					"@type": "PostalAddress",
					"streetAddress": "1850 Pearl Street",
					"addressLocality": "Boulder",
					"addressRegion": "CO"
				}
			}
		}
		</script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected structured event data")
	}

	if !strings.Contains(event.Location, "Convention Center") {
		t.Errorf("Location = %q, want to contain %q", event.Location, "Convention Center")
	}
	if !strings.Contains(event.Location, "1850 Pearl Street") {
		t.Errorf("Location = %q, want to contain %q", event.Location, "1850 Pearl Street")
	}
}

func TestExtractJSONLDEvent_NotAnEvent(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json">
		{
			"@type": "Product",
			"name": "Running Shoes",
			"price": "129.99"
		}
		</script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event != nil {
		t.Errorf("Expected nil for non-Event type, got %+v", event)
	}
}

func TestExtractJSONLDEvent_MalformedJSON(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json">
		{invalid json here}
		</script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event != nil {
		t.Errorf("Expected nil for malformed JSON, got %+v", event)
	}
}

func TestExtractJSONLDEvent_EmptyScript(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json"></script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		// Empty script should return nil, not crash.
		return
	}
	t.Logf("Empty script returned: %+v", event)
}

func TestExtractJSONLDEvent_NoScript(t *testing.T) {
	doc := htmlDoc(t, `<html><head></head><body><p>No JSON-LD here</p></body></html>`)

	event := extractJSONLDEvent(doc)
	if event != nil {
		t.Errorf("Expected nil when no JSON-LD script exists")
	}
}

func TestExtractJSONLDEvent_MultipleScripts(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json">
		{"@type": "Organization", "name": "Acme Inc"}
		</script>
		<script type="application/ld+json">
		{
			"@type": "Event",
			"name": "Team Picnic",
			"startDate": "2026-06-01T12:00:00-06:00"
		}
		</script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected structured event from second script tag")
	}

	if event.Name != "Team Picnic" {
		t.Errorf("Name = %q, want %q", event.Name, "Team Picnic")
	}
}

func TestExtractJSONLDEvent_MissingStartDate(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json">
		{
			"@type": "Event",
			"name": "Undated Event",
			"location": "Community Center"
		}
		</script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected event even without startDate")
	}

	if event.StartDate != nil {
		t.Errorf("StartDate should be nil, got %v", event.StartDate)
	}

	if event.Name != "Undated Event" {
		t.Errorf("Name = %q, want %q", event.Name, "Undated Event")
	}
}

func TestExtractJSONLDEvent_MusicEvent(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
		<script type="application/ld+json">
		{
			"@type": "MusicEvent",
			"name": "Jazz Night",
			"startDate": "2026-07-18T20:00:00-06:00"
		}
		</script>
		</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected MusicEvent to be recognized as an Event subtype")
	}

	if event.Name != "Jazz Night" {
		t.Errorf("Name = %q, want %q", event.Name, "Jazz Night")
	}
}

func TestParseISO8601(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *time.Time
	}{
		{
			name:  "RFC3339 with offset",
			input: "2026-07-18T19:00:00-06:00",
			want:  timePtr(time.Date(2026, 7, 18, 19, 0, 0, 0, time.FixedZone("", -6*60*60))),
		},
		{
			name:  "RFC3339 UTC",
			input: "2026-07-18T19:00:00Z",
			want:  timePtr(time.Date(2026, 7, 18, 19, 0, 0, 0, time.UTC)),
		},
		{
			name:  "datetime without timezone",
			input: "2026-05-20T09:00:00",
			want:  timePtr(time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)),
		},
		{
			name:  "date only",
			input: "2026-09-20",
			want:  timePtr(time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)),
		},
		{
			name:  "datetime with space",
			input: "2026-05-20 09:00:00",
			want:  timePtr(time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)),
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "garbage",
			input: "not a date",
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseISO8601(tc.input)
			if tc.want == nil {
				if got != nil {
					t.Errorf("parseISO8601(%q) = %v, want nil", tc.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseISO8601(%q) = nil, want %v", tc.input, tc.want)
			}
			if !got.Equal(*tc.want) {
				t.Errorf("parseISO8601(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestExtractJSONLDGeo_FloatCoordinates(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
	<script type="application/ld+json">
	{
		"@context": "https://schema.org",
		"@type": "Event",
		"name": "Community Hike",
		"startDate": "2026-05-17T09:00:00-06:00",
		"location": {
			"@type": "Place",
			"name": "Galvanize Boulder",
			"geo": {
				"@type": "GeoCoordinates",
				"latitude": 40.016722,
				"longitude": -105.281713
			}
		}
	}
	</script>
	</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected Event")
	}
	if event.Latitude == nil || event.Longitude == nil {
		t.Fatal("Expected geo coordinates")
	}
	if *event.Latitude != 40.016722 {
		t.Errorf("Latitude = %f, want 40.016722", *event.Latitude)
	}
	if *event.Longitude != -105.281713 {
		t.Errorf("Longitude = %f, want -105.281713", *event.Longitude)
	}
}

func TestExtractJSONLDGeo_StringCoordinates(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
	<script type="application/ld+json">
	{
		"@context": "https://schema.org",
		"@type": "SportsEvent",
		"name": "Colorado Triathlon",
		"startDate": "2026-06-06T07:15:00-06:00",
		"location": {
			"@type": "Place",
			"name": "Boulder Reservoir",
			"geo": {
				"@type": "GeoCoordinates",
				"latitude": "40.071939559156",
				"longitude": "-105.22858031101"
			}
		}
	}
	</script>
	</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected Event")
	}
	if event.Latitude == nil || event.Longitude == nil {
		t.Fatal("Expected geo coordinates from string values")
	}
	if *event.Latitude < 40.07 || *event.Latitude > 40.08 {
		t.Errorf("Latitude = %f, want ~40.07", *event.Latitude)
	}
	if *event.Longitude < -105.23 || *event.Longitude > -105.22 {
		t.Errorf("Longitude = %f, want ~-105.23", *event.Longitude)
	}
}

func TestExtractJSONLDGeo_NoGeo(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
	<script type="application/ld+json">
	{
		"@context": "https://schema.org",
		"@type": "Event",
		"name": "Some Event",
		"startDate": "2026-04-05T12:00:00-06:00",
		"location": {
			"@type": "Place",
			"name": "Some Place"
		}
	}
	</script>
	</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected Event")
	}
	if event.Latitude != nil || event.Longitude != nil {
		t.Errorf("Expected nil geo coordinates when no geo field, got lat=%v lng=%v", event.Latitude, event.Longitude)
	}
}

func TestExtractJSONLDGeo_StringLocation(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
	<script type="application/ld+json">
	{
		"@context": "https://schema.org",
		"@type": "Event",
		"name": "Remote Meetup",
		"startDate": "2026-04-05T12:00:00-06:00",
		"location": "Online"
	}
	</script>
	</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected Event")
	}
	if event.Latitude != nil || event.Longitude != nil {
		t.Errorf("Expected nil geo for string location, got lat=%v lng=%v", event.Latitude, event.Longitude)
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
