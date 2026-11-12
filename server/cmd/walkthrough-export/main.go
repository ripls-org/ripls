// walkthrough-export extracts one event's full content graph — event fields,
// roster with RSVPs, needs/contributions, chat script, and a media pull-list —
// from a Ripls database into scrubbed fixture inputs for the user journey walkthrough
// reels (#2684, consumed under e2e/fixtures/walkthroughs/). READ-ONLY by
// construction: it opens storage via OpenPostgreSQLDatabaseReadOnly (no DDL,
// no migrations) and issues only SELECTs, so it runs as a SELECT-only database
// role (prod: metabase_reader). PII scrub rules live in content.go; the
// extraction runbook (kept privately) covers prod access + the gsutil media
// pull that consumes media.json.
//
// Usage:
//
//	go run ./server/cmd/walkthrough-export \
//	  -db="postgres://ripls_reader:…@127.0.0.1:15432/ripls?sslmode=disable" \
//	  -name-like="picnic" -out=./picnic-export
//
// With -name-like, candidate events are listed; re-run with -experience-id to
// select one. Writes content.json + media.json into -out.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// rsvpLabel renders an RSVP intention as the lowercase word the walkthrough
// JSON uses ("yes"/"maybe"/"no"/"unspecified").
func rsvpLabel(intention models.RSVPIntention) string {
	return strings.ToLower(strings.TrimPrefix(intention.String(), "RSVP_INTENTION_"))
}

// scrubberVersion is stamped into content.json so a pack can be traced to the
// scrub rules that produced it. Bump when the rules in content.go change.
const scrubberVersion = 2

func main() {
	dbURL := flag.String("db", "", "PostgreSQL connection string (required; use a read-only role)")
	experienceID := flag.String("experience-id", "", "experience to export")
	nameLike := flag.String("name-like", "", "case-insensitive substring to locate the experience by name")
	outDir := flag.String("out", "./walkthrough-export", "output directory for content.json + media.json")
	flag.Parse()

	if *dbURL == "" {
		log.Fatal("-db is required")
	}
	if *experienceID == "" && *nameLike == "" {
		log.Fatal("one of -experience-id or -name-like is required")
	}

	ctx := context.Background()

	store, err := storage.OpenPostgreSQLDatabaseReadOnly(ctx, *dbURL, storage.DefaultStorageTypes())
	if err != nil {
		log.Fatalf("open storage (read-only): %v", err)
	}
	defer store.Close()

	exp := locateExperience(ctx, store, *experienceID, *nameLike)
	content, media := exportExperience(ctx, store, exp)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", *outDir, err)
	}
	writeJSON(filepath.Join(*outDir, "content.json"), content)
	writeJSON(filepath.Join(*outDir, "media.json"), media)
	fmt.Printf("exported experience %s → %s (content.json, media.json: %d media files)\n",
		exp.GetId(), *outDir, len(media))
	fmt.Println("next: pull binaries per media.json, review content.json, fill source.consent (see the extraction runbook)")
}

// locateExperience resolves the target event: by id when given, else by
// listing name matches. On multiple matches it prints the candidates and
// exits so the operator re-runs with -experience-id.
func locateExperience(ctx context.Context, store *storage.ProtoSQLStorage, id, nameLike string) *models.Experience {
	if id != "" {
		exp := &models.Experience{}
		if err := store.GetByID(ctx, id, exp); err != nil {
			log.Fatalf("load experience %s: %v", id, err)
		}
		return exp
	}

	msgs, err := store.ListAll(ctx, &models.Experience{})
	if err != nil {
		log.Fatalf("list experiences: %v", err)
	}
	var matches []*models.Experience
	needle := strings.ToLower(nameLike)
	for _, m := range msgs {
		exp := m.(*models.Experience)
		if strings.Contains(strings.ToLower(exp.GetName()), needle) {
			matches = append(matches, exp)
		}
	}
	switch len(matches) {
	case 0:
		log.Fatalf("no experience name matches %q", nameLike)
	case 1:
		return matches[0]
	}
	fmt.Fprintf(os.Stderr, "%d experiences match %q — re-run with -experience-id:\n", len(matches), nameLike)
	for _, exp := range matches {
		fmt.Fprintf(os.Stderr, "  %s  %q  start=%s\n",
			exp.GetId(), exp.GetName(), eventStartString(exp))
	}
	os.Exit(2)
	return nil
}

func eventStartString(exp *models.Experience) string {
	sec := exp.GetTime().GetSpecific().GetUnixTimestampSec()
	if sec == 0 {
		return "n/a"
	}
	return time.Unix(sec, 0).UTC().Format("2006-01-02")
}

// exportExperience walks the event's object graph and shapes it into the
// scrubbed screenplay + media pull-list.
func exportExperience(ctx context.Context, store *storage.ProtoSQLStorage, exp *models.Experience) (*Content, []MediaEntry) {
	namer := newFileNamer()
	var media []MediaEntry

	// addMedia resolves a media row and allocates its fixture filename,
	// memoized by media id — the same photo often appears both on the event
	// and in a chat message, and must land in the pack exactly once (first
	// allocation wins, so event media processed first keeps event-* names).
	// Returns "" when the row is missing (logged, not fatal — a deleted
	// avatar must not sink the export).
	fileByMediaID := map[string]string{}
	addMedia := func(mediaID, kind, base string) string {
		if mediaID == "" {
			return ""
		}
		if f, ok := fileByMediaID[mediaID]; ok {
			return f
		}
		row := &models.Media{}
		if err := store.GetByID(ctx, mediaID, row); err != nil {
			log.Printf("WARN: media %s (%s): %v — skipped", mediaID, kind, err)
			return ""
		}
		file := namer.name(base, extForContentType(row.GetContentType()))
		fileByMediaID[mediaID] = file
		media = append(media, MediaEntry{
			File:        file,
			Kind:        kind,
			MediaID:     mediaID,
			GsURI:       row.GetStorageUrl(),
			ThumbGsURI:  row.GetThumbnailStorageUrl(),
			ContentType: row.GetContentType(),
		})
		return file
	}

	// People: host + everyone who RSVP'd (registered or provisional).
	people := map[string]Person{} // keyed by user id / provisional id
	loadUser := func(userID string) Person {
		if p, ok := people[userID]; ok {
			return p
		}
		u := &models.User{}
		if err := store.GetByID(ctx, userID, u); err != nil {
			log.Printf("WARN: user %s: %v — skipped", userID, err)
			return Person{}
		}
		p := Person{Name: firstName(u.GetName())}
		if ids := u.GetMediaIds(); len(ids) > 0 {
			p.AvatarFile = addMedia(ids[0], "avatar", "avatar-"+p.Name)
		}
		people[userID] = p
		return p
	}

	// nameOf resolves any participant id (registered or already-seen
	// provisional) to a scrubbed first name, loading users lazily so
	// chat-only senders still resolve. Unknown ids render as "Someone"
	// rather than leaking raw ids into fixtures.
	nameOf := func(id string) string {
		if p, ok := people[id]; ok && p.Name != "" {
			return p.Name
		}
		if p := loadUser(id); p.Name != "" {
			return p.Name
		}
		return "Someone"
	}

	host := loadUser(exp.GetOwnerId())

	// Roster.
	rsvpMsgs, err := store.QueryByField(ctx, "experience_id", exp.GetId(), &models.ExperienceRSVP{})
	if err != nil {
		log.Fatalf("load RSVPs: %v", err)
	}
	var attendees []Attendee
	for _, m := range rsvpMsgs {
		r := m.(*models.ExperienceRSVP)
		var p Person
		switch {
		case r.GetUserId() != "" && r.GetUserId() != exp.GetOwnerId():
			p = loadUser(r.GetUserId())
		case r.GetProvisionalUserId() != "":
			prov := &models.ProvisionalUser{}
			if err := store.GetByID(ctx, r.GetProvisionalUserId(), prov); err != nil {
				log.Printf("WARN: provisional user %s: %v — skipped", r.GetProvisionalUserId(), err)
				continue
			}
			p = Person{Name: firstName(prov.GetName())}
			people[r.GetProvisionalUserId()] = p
		default:
			continue // the host's own auto-RSVP
		}
		if p.Name == "" {
			continue
		}
		attendees = append(attendees, Attendee{Person: p, RSVP: rsvpLabel(r.GetIntention())})
	}
	sort.Slice(attendees, func(i, j int) bool { return attendees[i].Name < attendees[j].Name })

	// Event media.
	var eventFiles []string
	for _, id := range exp.GetMediaIds() {
		if f := addMedia(id, "event", "event"); f != "" {
			eventFiles = append(eventFiles, f)
		}
	}

	// Venue — name + locality only (never address lines / coordinates; see
	// content.go scrub rules).
	var venueName, locality string
	if exp.GetLocationId() != "" {
		loc := &models.Location{}
		if err := store.GetByID(ctx, exp.GetLocationId(), loc); err != nil {
			log.Printf("WARN: location %s: %v — skipped", exp.GetLocationId(), err)
		} else {
			venueName = venueNameOrEmpty(loc.GetName())
			locality = loc.GetAddress().GetLocality()
		}
	}

	// Needs + contributions.
	needMsgs, err := store.QueryByField(ctx, "experience_id", exp.GetId(), &models.PlanningNeed{})
	if err != nil {
		log.Fatalf("load needs: %v", err)
	}
	needNames := map[string]string{} // need id → name (for contribution back-refs)
	var needs []Need
	for _, m := range needMsgs {
		n := m.(*models.PlanningNeed)
		needNames[n.GetId()] = n.GetName()
		needs = append(needs, Need{
			Proposer: nameOf(n.GetProposerId()),
			Name:     n.GetName(),
			Note:     scrubText(n.GetNote()),
			Slots:    n.GetSlots(),
		})
	}

	contribMsgs, err := store.QueryByField(ctx, "experience_id", exp.GetId(), &models.PlanningContribution{})
	if err != nil {
		log.Fatalf("load contributions: %v", err)
	}
	var contribs []Contribution
	for _, m := range contribMsgs {
		c := m.(*models.PlanningContribution)
		fromNeed := needNames[c.GetFromNeedId()]
		if fromNeed == "" {
			fromNeed = c.GetOriginalNeedName()
		}
		contribs = append(contribs, Contribution{
			Contributor: nameOf(c.GetContributorId()),
			Title:       c.GetTitle(),
			Description: scrubText(c.GetDescription()),
			FromNeed:    fromNeed,
		})
	}

	// Chat script: user messages (text + photos), in order, with offsets from
	// the first message so the reel can replay the cadence.
	var messages []Message
	if convID := exp.GetConversationId(); convID != "" {
		msgMsgs, err := store.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
		if err != nil {
			log.Fatalf("load chat messages: %v", err)
		}
		chat := make([]*models.ChatMessage, 0, len(msgMsgs))
		for _, m := range msgMsgs {
			chat = append(chat, m.(*models.ChatMessage))
		}
		sort.Slice(chat, func(i, j int) bool {
			return chat[i].GetSentAtUnixSec() < chat[j].GetSentAtUnixSec()
		})
		var t0 int64
		for _, cm := range chat {
			um := cm.GetUserMessage()
			if um == nil {
				continue // system messages are re-created by the reel's own actions
			}
			if t0 == 0 {
				t0 = cm.GetSentAtUnixSec()
			}
			var files []string
			for _, id := range um.GetMediaIds() {
				if f := addMedia(id, "chat", "chat"); f != "" {
					files = append(files, f)
				}
			}
			messages = append(messages, Message{
				Sender:     nameOf(um.GetSenderId()),
				Text:       scrubText(um.GetText()),
				MediaFiles: files,
				OffsetSec:  cm.GetSentAtUnixSec() - t0,
			})
		}
	}

	content := &Content{
		Source: SourceInfo{
			ExperienceID:    exp.GetId(),
			ExportedAtUnix:  time.Now().Unix(),
			Consent:         "", // operator fills in (see the extraction runbook)
			PurgeIfPublic:   true,
			ScrubberVersion: scrubberVersion,
		},
		Event: EventInfo{
			Name:         exp.GetName(),
			Description:  scrubText(exp.GetDescription()),
			VenueName:    venueName,
			Locality:     locality,
			MediaFiles:   eventFiles,
			StartUnixSec: exp.GetTime().GetSpecific().GetUnixTimestampSec(),
		},
		Host:          host,
		Attendees:     attendees,
		Needs:         needs,
		Contributions: contribs,
		Messages:      messages,
	}
	return content, media
}

func writeJSON(path string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
}
