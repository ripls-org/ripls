// preview-activity-digest builds and sends the daily activity digest
// (#1924) for a single date, on demand. One-shot dev tool — not wired
// into the running server. Used to iterate on the email template
// without waiting for the hourly scheduler.
//
// Usage:
//
//	go run ./server/cmd/preview-activity-digest \
//	  --db=postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable \
//	  --mailgun-api-key=$MAILGUN_API_KEY \
//	  --mailgun-domain=mail.example.com \
//	  --mailgun-from='Example <noreply@example.com>' \
//	  --to=you@example.com \
//	  --date=2026-05-13 \
//	  --timezone=America/Denver
//
// When --date is empty, "yesterday in --timezone" is used. The send
// bypasses the activity_digest_state claim primitive — re-running on
// the same date will deliver another email.
//
// If --mailgun-api-key is empty the tool renders the HTML and text
// bodies to local files (./tmp/activity_digest.html and
// ./tmp/activity_digest.txt) instead of sending, so you can inspect
// the output without involving Mailgun.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"go.ripls.org/ripls/server/activity_digest"
	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/storage"
)

func main() {
	dbURL := flag.String("db", "postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable", "PostgreSQL connection string")
	mailgunAPIKey := flag.String("mailgun-api-key", "", "Mailgun API key. When empty, the digest is rendered to ./tmp instead of sent.")
	mailgunDomain := flag.String("mailgun-domain", "", "Mailgun sending domain.")
	mailgunFrom := flag.String("mailgun-from", "", "From address for the digest.")
	toEmail := flag.String("to", "", "Recipient.")
	periodStr := flag.String("period", "daily", "Digest cadence: daily | weekly.")
	dateStr := flag.String("date", "", "Window-anchor date YYYY-MM-DD. Daily: defaults to yesterday in --timezone. Weekly: the 7-day window ends at midnight of (date+1).")
	tzName := flag.String("timezone", "America/Denver", "IANA timezone the window is computed in.")
	outDir := flag.String("out-dir", "tmp", "Directory for rendered output when --mailgun-api-key is empty.")
	flag.Parse()

	loc, err := time.LoadLocation(*tzName)
	if err != nil {
		log.Fatalf("load timezone %q: %v", *tzName, err)
	}

	var period email.DigestPeriod
	switch *periodStr {
	case "daily", "":
		period = email.DigestPeriodDaily
	case "weekly":
		period = email.DigestPeriodWeekly
	default:
		log.Fatalf("--period %q: must be 'daily' or 'weekly'", *periodStr)
	}

	var windowDate time.Time
	if *dateStr == "" {
		windowDate = time.Now().In(loc).AddDate(0, 0, -1)
	} else {
		windowDate, err = time.ParseInLocation("2006-01-02", *dateStr, loc)
		if err != nil {
			log.Fatalf("parse --date %q: %v", *dateStr, err)
		}
	}

	ctx := context.Background()

	store, err := storage.InitializePostgreSQLDatabase(ctx, *dbURL, storage.DefaultStorageTypes())
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}
	defer store.Close()

	builder := activity_digest.New(store, nil)
	var digest email.ActivityDigestInput
	switch period {
	case email.DigestPeriodWeekly:
		// Weekly window: 7 local days ending at midnight of (windowDate + 1).
		anchor := time.Date(windowDate.Year(), windowDate.Month(), windowDate.Day(), 0, 0, 0, 0, loc)
		end := anchor.AddDate(0, 0, 1)
		start := end.AddDate(0, 0, -7)
		digest, err = builder.BuildRange(ctx, start, end, email.DigestPeriodWeekly)
	default:
		digest, err = builder.Build(ctx, windowDate)
	}
	if err != nil {
		log.Fatalf("build digest: %v", err)
	}

	fmt.Printf("Built digest for %s (%s window: %s → %s)\n",
		windowDate.Format("2006-01-02"),
		*tzName,
		time.Unix(digest.WindowStartUnixSec, 0).In(loc).Format(time.RFC3339),
		time.Unix(digest.WindowEndUnixSec, 0).In(loc).Format(time.RFC3339),
	)
	fmt.Printf("  member_actions=%d  user_chat_messages=%d  communities=%d\n",
		digest.TotalMemberActionCount, digest.TotalUserMessageCount, len(digest.Communities))
	fmt.Printf("  new_users=%d  interactive_sign_ins=%d  active_users=%d (covered=%t)  contributors=%d\n",
		len(digest.NewUsers), digest.InteractiveSignInCount,
		digest.ActiveUserCount, digest.ActiveUserDataCoversWindow, digest.ActiveContributorCount)

	if *mailgunAPIKey == "" {
		htmlBody, textBody, subject, err := email.RenderActivityDigest(digest)
		if err != nil {
			log.Fatalf("render digest: %v", err)
		}
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			log.Fatalf("mkdir %s: %v", *outDir, err)
		}
		htmlPath := filepath.Join(*outDir, "activity_digest.html")
		textPath := filepath.Join(*outDir, "activity_digest.txt")
		if err := os.WriteFile(htmlPath, []byte(htmlBody), 0o600); err != nil {
			log.Fatalf("write html: %v", err)
		}
		if err := os.WriteFile(textPath, []byte(textBody), 0o600); err != nil {
			log.Fatalf("write text: %v", err)
		}
		fmt.Printf("Subject: %s\n", subject)
		fmt.Printf("Rendered to %s and %s (no --mailgun-api-key supplied).\n", htmlPath, textPath)
		return
	}

	mg, err := email.NewMailgunService(*mailgunDomain, *mailgunAPIKey, *mailgunFrom)
	if err != nil {
		log.Fatalf("init mailgun: %v", err)
	}
	if err := mg.SendActivityDigest(ctx, *toEmail, digest); err != nil {
		log.Fatalf("send digest: %v", err)
	}
	fmt.Printf("Sent digest to %s via Mailgun domain %s.\n", *toEmail, *mailgunDomain)
}
