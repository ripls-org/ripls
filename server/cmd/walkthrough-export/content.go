package main

import (
	"fmt"
	"regexp"
	"strings"
)

// content.go — the pure shaping + PII-scrub layer between raw storage models
// and the committed fixture files. Everything here is deterministic and
// side-effect-free so the scrub rules are unit-testable (content_test.go):
// the committed pack must carry first names only, no contact details, and no
// street-level location data.

// Content is the screenplay file (content.json) an e2e walkthrough reel seeds its
// world from: the cast, the event, the needs list, and the message script.
type Content struct {
	Source        SourceInfo     `json:"source"`
	Event         EventInfo      `json:"event"`
	Host          Person         `json:"host"`
	Attendees     []Attendee     `json:"attendees"`
	Needs         []Need         `json:"needs"`
	Contributions []Contribution `json:"contributions"`
	Messages      []Message      `json:"messages"`
}

// SourceInfo records provenance + consent for the pack. The consent line is
// filled by the operator (see README) before the pack is committed.
type SourceInfo struct {
	ExperienceID    string `json:"experienceId"`
	ExportedAtUnix  int64  `json:"exportedAtUnixSec"`
	Consent         string `json:"consent"`
	PurgeIfPublic   bool   `json:"purgeIfRepoGoesPublic"`
	ScrubberVersion int    `json:"scrubberVersion"`
}

// EventInfo carries the event's public-facing fields. Location is reduced to
// venue name + locality — never address lines or coordinates (family homes).
type EventInfo struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	VenueName    string   `json:"venueName,omitempty"`
	Locality     string   `json:"locality,omitempty"`
	MediaFiles   []string `json:"mediaFiles"`
	StartUnixSec int64    `json:"startUnixSec,omitempty"`
}

// Person is a scrubbed participant: first name + avatar fixture file only.
type Person struct {
	Name       string `json:"name"`
	AvatarFile string `json:"avatarFile,omitempty"`
}

// Attendee is a Person plus their RSVP intention (yes|maybe|no).
type Attendee struct {
	Person
	RSVP string `json:"rsvp"`
}

// Need mirrors PlanningNeed's screenplay-relevant fields.
type Need struct {
	Proposer string `json:"proposer"`
	Name     string `json:"name"`
	Note     string `json:"note,omitempty"`
	Slots    int32  `json:"slots"`
}

// Contribution mirrors PlanningContribution's screenplay-relevant fields.
type Contribution struct {
	Contributor string `json:"contributor"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	FromNeed    string `json:"fromNeed,omitempty"`
}

// Message is one chat beat: sender first name, redacted text, and the chat
// photo fixture files it attaches.
type Message struct {
	Sender     string   `json:"sender"`
	Text       string   `json:"text,omitempty"`
	MediaFiles []string `json:"mediaFiles,omitempty"`
	OffsetSec  int64    `json:"offsetSec"`
}

// MediaEntry is one row of media.json — the pull list the runbook's gsutil
// step consumes. File is the fixture filename the binary will be saved as.
type MediaEntry struct {
	File        string `json:"file"`
	Kind        string `json:"kind"` // hero | event | avatar | chat
	MediaID     string `json:"mediaId"`
	GsURI       string `json:"gsUri"`
	ThumbGsURI  string `json:"thumbGsUri,omitempty"`
	ContentType string `json:"contentType"`
}

// firstName reduces a full name to its first given name — the scrub rule for
// every name that reaches a committed fixture.
func firstName(full string) string {
	fields := strings.Fields(strings.TrimSpace(full))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

var (
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	// Candidate phone runs: optional +, digits with common separators.
	// Candidates are confirmed in redactPhones (≥7 digits, not embedded in a
	// larger token) so UUID/hex fragments don't get eaten.
	phoneRe = regexp.MustCompile(`\+?\d[\d\s().-]{5,}\d`)
	// Chat mention tokens: @[kind:id:Display Label] (users, items, …).
	mentionRe = regexp.MustCompile(`@\[([a-z]+):[^:\]]*:([^\]]*)\]`)
)

// scrubText is the full free-text scrub for anything user-written (chat
// messages, notes, descriptions): mention tokens are rewritten to plain
// display text (dropping ids and, for people, surnames), then contact
// details are redacted. Fixture review is still the human backstop; this is
// the mechanical floor.
func scrubText(text string) string {
	return redactContacts(rewriteMentions(text))
}

// rewriteMentions replaces raw mention tokens with their display text: user
// mentions become "@<FirstName>" (surname is PII); other kinds (items etc.)
// keep their full label.
func rewriteMentions(text string) string {
	return mentionRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := mentionRe.FindStringSubmatch(m)
		kind, label := sub[1], sub[2]
		if kind == "user" {
			return "@" + firstName(label)
		}
		return "@" + label
	})
}

// redactContacts strips email addresses and phone-number-looking runs from
// free text so a pasted contact detail can't ride a fixture into the repo.
func redactContacts(text string) string {
	return redactPhones(emailRe.ReplaceAllString(text, "[redacted]"))
}

// redactPhones redacts phoneRe candidates that really look like phone
// numbers: at least 7 digits, and not glued to a surrounding word/hex token
// (which would be a UUID or identifier fragment, not a phone number).
func redactPhones(s string) string {
	matches := phoneRe.FindAllStringIndex(s, -1)
	if matches == nil {
		return s
	}
	isWord := func(b byte) bool {
		return b == '-' || b == '_' ||
			(b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		digits := 0
		for i := start; i < end; i++ {
			if s[i] >= '0' && s[i] <= '9' {
				digits++
			}
		}
		embedded := (start > 0 && isWord(s[start-1])) || (end < len(s) && isWord(s[end]))
		if digits < 7 || embedded {
			continue
		}
		b.WriteString(s[last:start])
		b.WriteString("[redacted]")
		last = end
	}
	b.WriteString(s[last:])
	return b.String()
}

var leadingHouseNumberRe = regexp.MustCompile(`^\d+\s`)

// venueNameOrEmpty drops venue names that are themselves street addresses
// (e.g. "123 Example Street" — a family home saved with the address as its
// name). Only non-address venue names ("Skyline Rooftop & Lounge") may
// reach a fixture; locality alone is the fallback.
func venueNameOrEmpty(name string) string {
	if leadingHouseNumberRe.MatchString(strings.TrimSpace(name)) {
		return ""
	}
	return name
}

// extForContentType maps a media row's content type to the fixture file
// extension. Unknown types keep a neutral .bin so nothing is silently
// mislabeled.
func extForContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	default:
		return ".bin"
	}
}

// fileNamer allocates stable, collision-free fixture filenames
// (avatar-maya.jpg, avatar-maya-2.jpg, event-01.jpg, chat-03.png, …).
type fileNamer struct {
	used map[string]bool
}

func newFileNamer() *fileNamer {
	return &fileNamer{used: map[string]bool{}}
}

var unsafeFileChars = regexp.MustCompile(`[^a-z0-9-]+`)

// name returns "<base><ext>" or, on collision, "<base>-N<ext>". base is
// lowercased and reduced to [a-z0-9-].
func (n *fileNamer) name(base, ext string) string {
	base = unsafeFileChars.ReplaceAllString(strings.ToLower(base), "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "media"
	}
	candidate := base + ext
	for i := 2; n.used[candidate]; i++ {
		candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
	n.used[candidate] = true
	return candidate
}
