package main

import "testing"

func TestFirstName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Maya Lopez", "Maya"},
		{"  Priya  ", "Priya"},
		{"Jean-Luc Picard", "Jean-Luc"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := firstName(c.in); got != c.want {
			t.Errorf("firstName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRedactContacts(t *testing.T) {
	cases := []struct{ in, want string }{
		{"email me at maya@example.com ok?", "email me at [redacted] ok?"},
		{"call +1 (555) 123-4567 tonight", "call [redacted] tonight"},
		{"my number is 555-123-4567", "my number is [redacted]"},
		{"see you at 6:30, bring 2 chairs", "see you at 6:30, bring 2 chairs"},
		// UUID/identifier fragments must survive: embedded in a larger token
		// (hex letters on the boundaries) or too few digits.
		{"id a4ac610f-667e-4bff-a1d2-e08dfb73feb6 ok", "id a4ac610f-667e-4bff-a1d2-e08dfb73feb6 ok"},
		{"order 12-34 arrived", "order 12-34 arrived"},
		{"", ""},
	}
	for _, c := range cases {
		if got := redactContacts(c.in); got != c.want {
			t.Errorf("redactContacts(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestScrubTextRewritesMentions(t *testing.T) {
	cases := []struct{ in, want string }{
		// User mentions: id dropped, surname dropped.
		{
			"@[user:0ff9a6be-d3a8-482d-9f4f-412e47797588:Maya Lopez] so excited!!",
			"@Maya so excited!!",
		},
		// Non-user mentions keep their full label (item names aren't PII).
		{"bringing @[gear:abc-123:Folding Tables] tomorrow", "bringing @Folding Tables tomorrow"},
		// Mention rewrite runs before phone redaction, so the mention's UUID
		// digits never look like a phone number.
		{
			"@[user:02db24b9-2869-4fca-a080-9ad22a27b09e:Sam Rivera] call me at 555-123-4567",
			"@Sam call me at [redacted]",
		},
		{"no mentions here", "no mentions here"},
	}
	for _, c := range cases {
		if got := scrubText(c.in); got != c.want {
			t.Errorf("scrubText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestVenueNameOrEmpty(t *testing.T) {
	cases := []struct{ in, want string }{
		{"123 Example Street", ""}, // street address saved as venue name
		{"Skyline Rooftop & Lounge", "Skyline Rooftop & Lounge"},
		{"  12 Oak Ave", ""},
		{"Studio 54", "Studio 54"}, // number not leading → kept
		{"", ""},
	}
	for _, c := range cases {
		if got := venueNameOrEmpty(c.in); got != c.want {
			t.Errorf("venueNameOrEmpty(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtForContentType(t *testing.T) {
	cases := []struct{ in, want string }{
		{"image/jpeg", ".jpg"},
		{"IMAGE/PNG", ".png"},
		{"video/mp4", ".mp4"},
		{"application/octet-stream", ".bin"},
		{"", ".bin"},
	}
	for _, c := range cases {
		if got := extForContentType(c.in); got != c.want {
			t.Errorf("extForContentType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFileNamerDedupes(t *testing.T) {
	n := newFileNamer()
	if got := n.name("avatar-Maya", ".jpg"); got != "avatar-maya.jpg" {
		t.Errorf("first name = %q, want avatar-maya.jpg", got)
	}
	if got := n.name("avatar-maya", ".jpg"); got != "avatar-maya-2.jpg" {
		t.Errorf("collision name = %q, want avatar-maya-2.jpg", got)
	}
	if got := n.name("avatar-maya", ".jpg"); got != "avatar-maya-3.jpg" {
		t.Errorf("second collision = %q, want avatar-maya-3.jpg", got)
	}
	if got := n.name("", ".png"); got != "media.png" {
		t.Errorf("empty base = %q, want media.png", got)
	}
	if got := n.name("Événement d'été!", ".jpg"); got != "v-nement-d-t.jpg" {
		t.Errorf("unsafe chars = %q, want v-nement-d-t.jpg", got)
	}
}
