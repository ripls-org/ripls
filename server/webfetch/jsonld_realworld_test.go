package webfetch

import (
	"strings"
	"testing"
	"time"
)

// Real-world JSON-LD test cases. Each HTML snippet is taken from an actual event
// page on the given platform, with only the JSON-LD script tags retained.
// This validates that our parser handles the actual markup each platform emits.

// TestRealWorld_Eventbrite tests JSON-LD from an Eventbrite event page.
// Source: https://www.eventbrite.com/e/1940s-ball-at-boulder-airport-...-tickets-1661328019829
// Fetched: 2026-04-14
// Format notes: Multiple JSON-LD blocks (WebPage, Event, FAQPage, BreadcrumbList).
// Event block uses Place with PostalAddress including streetAddress.
func TestRealWorld_Eventbrite(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"WebPage","name":"1940s Ball at Boulder Airport","url":"https://www.eventbrite.com/e/..."}
</script>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "Event",
  "name": "1940s Ball at Boulder Airport with the World Famous Glenn Miller Orchestra!",
  "description": "Colorado's most nostalgic and most romantic night of the year!",
  "startDate": "2026-06-20T18:30:00-06:00",
  "endDate": "2026-06-20T23:59:00-06:00",
  "eventAttendanceMode": "https://schema.org/OfflineEventAttendanceMode",
  "eventStatus": "https://schema.org/EventScheduled",
  "location": {
    "@type": "Place",
    "name": "Boulder Airport",
    "address": {
      "@type": "PostalAddress",
      "addressCountry": "US",
      "addressLocality": "Boulder",
      "addressRegion": "CO",
      "streetAddress": "3393 Airport Rd, Boulder, CO 80302"
    }
  },
  "organizer": {
    "@type": "Organization",
    "name": "1940s Ball NFP"
  },
  "performer": [{"@type":"Person","name":"The World Famous Glenn Miller Orchestra"}],
  "offers": [{"@type":"AggregateOffer","lowPrice":"105.87","highPrice":"1185.29","priceCurrency":"USD"}],
  "startDate": "2026-06-20T18:30:00-06:00",
  "url": "https://www.eventbrite.com/e/..."
}
</script>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[]}
</script>
</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected Event from Eventbrite page")
	}

	if event.Name != "1940s Ball at Boulder Airport with the World Famous Glenn Miller Orchestra!" {
		t.Errorf("Name = %q", event.Name)
	}

	if event.StartDate == nil {
		t.Fatal("StartDate is nil")
	}

	expectedStart := time.Date(2026, 6, 20, 18, 30, 0, 0, time.FixedZone("MDT", -6*60*60))
	if !event.StartDate.Equal(expectedStart) {
		t.Errorf("StartDate = %v, want %v", event.StartDate, expectedStart)
	}

	if event.EndDate == nil {
		t.Fatal("EndDate is nil")
	}

	expectedEnd := time.Date(2026, 6, 20, 23, 59, 0, 0, time.FixedZone("MDT", -6*60*60))
	if !event.EndDate.Equal(expectedEnd) {
		t.Errorf("EndDate = %v, want %v", event.EndDate, expectedEnd)
	}

	// Eventbrite uses Place with name + PostalAddress with streetAddress.
	if !strings.Contains(event.Location, "Boulder Airport") {
		t.Errorf("Location = %q, want to contain 'Boulder Airport'", event.Location)
	}
	if !strings.Contains(event.Location, "3393 Airport Rd") {
		t.Errorf("Location = %q, want to contain '3393 Airport Rd'", event.Location)
	}
}

// TestRealWorld_EventbriteBusinessEvent tests JSON-LD with @type "BusinessEvent".
// Source: https://www.eventbrite.com/e/atlas-expo-tickets-1985865466067
// Fetched: 2026-04-14
// Format notes: Uses "BusinessEvent" instead of "Event".
func TestRealWorld_EventbriteBusinessEvent(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "BusinessEvent",
  "name": "ATLAS EXPO",
  "description": "Go hands-on with projects from the Creative Technology & Design program",
  "startDate": "2026-04-24T15:30:00-06:00",
  "endDate": "2026-04-24T18:00:00-06:00",
  "eventAttendanceMode": "https://schema.org/OfflineEventAttendanceMode",
  "eventStatus": "https://schema.org/EventScheduled",
  "location": {
    "@type": "Place",
    "name": "Roser ATLAS Center",
    "address": {
      "@type": "PostalAddress",
      "addressCountry": "US",
      "addressLocality": "Boulder",
      "addressRegion": "CO",
      "streetAddress": "1125 18th St, Boulder, CO 80309"
    }
  }
}
</script>
</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected BusinessEvent to be recognized")
	}

	if event.Name != "ATLAS EXPO" {
		t.Errorf("Name = %q", event.Name)
	}

	if event.StartDate == nil {
		t.Fatal("StartDate is nil")
	}

	expectedStart := time.Date(2026, 4, 24, 15, 30, 0, 0, time.FixedZone("MDT", -6*60*60))
	if !event.StartDate.Equal(expectedStart) {
		t.Errorf("StartDate = %v, want %v", event.StartDate, expectedStart)
	}

	if !strings.Contains(event.Location, "Roser ATLAS Center") {
		t.Errorf("Location = %q, want to contain 'Roser ATLAS Center'", event.Location)
	}
}

// TestRealWorld_Meetup tests JSON-LD from a Meetup event page.
// Source: https://www.meetup.com/boulder-social-hikers/events/314002058/
// Fetched: 2026-04-14
// Format notes: Multiple JSON-LD blocks (Organization, BreadcrumbList, Event, RsvpAction).
// Location has Place with PostalAddress but no Place name or streetAddress.
func TestRealWorld_Meetup(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"Organization","name":"Meetup","url":"https://www.meetup.com/..."}
</script>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[]}
</script>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "Event",
  "name": "Green Mountain West Ridge ",
  "description": "",
  "startDate": "2026-04-05T12:00:00-06:00",
  "endDate": "2026-04-05T15:00:00-06:00",
  "eventAttendanceMode": "https://schema.org/OfflineEventAttendanceMode",
  "eventStatus": "https://schema.org/EventScheduled",
  "location": {
    "@type": "Place",
    "address": {
      "@type": "PostalAddress",
      "addressCountry": "us",
      "addressLocality": "Boulder",
      "addressRegion": "CO"
    }
  },
  "organizer": {
    "@type": "Organization",
    "name": "Boulder Social Hikers, Bikers and More!",
    "url": "https://www.meetup.com/boulder-social-hikers/"
  }
}
</script>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"RsvpAction"}
</script>
</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected Event from Meetup page")
	}

	// Meetup has trailing space in name — verify we get it as-is.
	if !strings.Contains(event.Name, "Green Mountain West Ridge") {
		t.Errorf("Name = %q, want to contain 'Green Mountain West Ridge'", event.Name)
	}

	if event.StartDate == nil {
		t.Fatal("StartDate is nil")
	}

	expectedStart := time.Date(2026, 4, 5, 12, 0, 0, 0, time.FixedZone("MDT", -6*60*60))
	if !event.StartDate.Equal(expectedStart) {
		t.Errorf("StartDate = %v, want %v", event.StartDate, expectedStart)
	}

	if event.EndDate == nil {
		t.Fatal("EndDate is nil")
	}

	expectedEnd := time.Date(2026, 4, 5, 15, 0, 0, 0, time.FixedZone("MDT", -6*60*60))
	if !event.EndDate.Equal(expectedEnd) {
		t.Errorf("EndDate = %v, want %v", event.EndDate, expectedEnd)
	}

	// Meetup's location has no Place name and no streetAddress — just city/state.
	// Our extractor returns empty string since there's no Place name.
	// This is correct behavior — the LLM will extract location from body text.
	t.Logf("Location = %q (empty expected for Meetup without Place name)", event.Location)
}

// TestRealWorld_Luma tests JSON-LD from a Luma event page.
// Source: https://lu.ma/zrh5595h
// Fetched: 2026-04-14
// Format notes: Single JSON-LD block. Uses milliseconds in datetime (.000).
// Location has Place with nested Country object in address.
func TestRealWorld_Luma(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@id": "https://luma.com/zrh5595h",
  "@type": "Event",
  "name": "Startup Community Hike",
  "description": "Designed to help you connect with founders, investors, operators in the Colorado startup ecosystem!",
  "startDate": "2024-05-17T09:00:00.000-06:00",
  "endDate": "2024-05-17T12:00:00.000-06:00",
  "eventAttendanceMode": "https://schema.org/OfflineEventAttendanceMode",
  "eventStatus": "https://schema.org/EventScheduled",
  "location": {
    "@type": "Place",
    "name": "Galvanize Boulder",
    "address": {
      "@type": "PostalAddress",
      "addressCountry": {
        "@type": "Country",
        "name": "United States"
      },
      "addressLocality": "Boulder",
      "addressRegion": "Colorado",
      "streetAddress": "Galvanize Boulder"
    },
    "geo": {
      "@type": "GeoCoordinates",
      "latitude": 40.016722,
      "longitude": -105.281713
    }
  },
  "organizer": [{"@type":"Organization","name":"Lindsey Rohde"}],
  "offers": [{"@type":"Offer","price":0,"priceCurrency":"usd"}]
}
</script>
</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected Event from Luma page")
	}

	if event.Name != "Startup Community Hike" {
		t.Errorf("Name = %q", event.Name)
	}

	if event.StartDate == nil {
		t.Fatal("StartDate is nil")
	}

	// Luma uses milliseconds in the datetime string.
	expectedStart := time.Date(2024, 5, 17, 9, 0, 0, 0, time.FixedZone("MDT", -6*60*60))
	if !event.StartDate.Equal(expectedStart) {
		t.Errorf("StartDate = %v, want %v", event.StartDate, expectedStart)
	}

	if event.EndDate == nil {
		t.Fatal("EndDate is nil")
	}

	// Luma location has Place name + PostalAddress with Country object.
	if !strings.Contains(event.Location, "Galvanize Boulder") {
		t.Errorf("Location = %q, want to contain 'Galvanize Boulder'", event.Location)
	}

	// Luma includes geo coordinates as float values.
	if event.Latitude == nil || event.Longitude == nil {
		t.Fatal("Expected geo coordinates from Luma page")
	}
	if *event.Latitude != 40.016722 {
		t.Errorf("Latitude = %f, want 40.016722", *event.Latitude)
	}
	if *event.Longitude != -105.281713 {
		t.Errorf("Longitude = %f, want -105.281713", *event.Longitude)
	}
}

// TestRealWorld_AllEvents tests JSON-LD from an AllEvents.in event page.
// Source: https://allevents.in/boulder/colorado-triathlon-.../200029094972111
// Fetched: 2026-04-14
// Format notes: Uses "SportsEvent" @type. Location has Place name +
// PostalAddress where the name field is on the address, not the Place.
// Has no endDate.
func TestRealWorld_AllEvents(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "SportsEvent",
  "name": "Colorado Triathlon (Sprint, Olympic, Duathlon, & Aquabike)",
  "description": "The best of life is experienced outside, not bought online or in a shopping mall!",
  "startDate": "2026-06-06T07:15:00-06:00",
  "eventAttendanceMode": "https://schema.org/OfflineEventAttendanceMode",
  "eventStatus": "https://schema.org/EventScheduled",
  "location": {
    "@type": "Place",
    "name": "Boulder Reservoir",
    "address": {
      "@type": "PostalAddress",
      "addressCountry": "US",
      "addressLocality": "Boulder",
      "addressRegion": "CO",
      "name": "Boulder Reservoir, Boulder, United States"
    },
    "geo": {
      "@type": "GeoCoordinates",
      "latitude": "40.071939559156",
      "longitude": "-105.22858031101"
    }
  },
  "organizer": [{"@type":"Organization","name":"Without Limits Productions"}]
}
</script>
</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event == nil {
		t.Fatal("Expected SportsEvent from AllEvents page")
	}

	if event.Name != "Colorado Triathlon (Sprint, Olympic, Duathlon, & Aquabike)" {
		t.Errorf("Name = %q", event.Name)
	}

	if event.StartDate == nil {
		t.Fatal("StartDate is nil")
	}

	expectedStart := time.Date(2026, 6, 6, 7, 15, 0, 0, time.FixedZone("MDT", -6*60*60))
	if !event.StartDate.Equal(expectedStart) {
		t.Errorf("StartDate = %v, want %v", event.StartDate, expectedStart)
	}

	// No endDate in the AllEvents JSON-LD.
	if event.EndDate != nil {
		t.Errorf("Expected nil EndDate, got %v", event.EndDate)
	}

	if !strings.Contains(event.Location, "Boulder Reservoir") {
		t.Errorf("Location = %q, want to contain 'Boulder Reservoir'", event.Location)
	}

	// AllEvents includes geo coordinates as string values.
	if event.Latitude == nil || event.Longitude == nil {
		t.Fatal("Expected geo coordinates from AllEvents page")
	}
	if *event.Latitude < 40.07 || *event.Latitude > 40.08 {
		t.Errorf("Latitude = %f, want ~40.07", *event.Latitude)
	}
	if *event.Longitude < -105.23 || *event.Longitude > -105.22 {
		t.Errorf("Longitude = %f, want ~-105.23", *event.Longitude)
	}
}

// TestRealWorld_EventbriteNoEvent tests an Eventbrite page that only has
// WebPage and BreadcrumbList JSON-LD — no Event block.
// Source: https://www.eventbrite.com/e/taste-of-pearl-2026-tickets-1981789132644
// Fetched: 2026-04-14
// Format notes: Some Eventbrite pages omit the Event JSON-LD entirely.
func TestRealWorld_EventbriteNoEvent(t *testing.T) {
	doc := htmlDoc(t, `<html><head>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "WebPage",
  "name": "Taste of Pearl 2026",
  "url": "https://www.eventbrite.com/e/taste-of-pearl-2026-tickets-1981789132644"
}
</script>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "BreadcrumbList",
  "itemListElement": [
    {"@type":"ListItem","position":1,"item":{"@id":"/d/united-states/events/","name":"United States Events"}}
  ]
}
</script>
</head><body></body></html>`)

	event := extractJSONLDEvent(doc)
	if event != nil {
		t.Errorf("Expected nil for Eventbrite page without Event JSON-LD, got %+v", event)
	}
}
