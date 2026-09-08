// Command backfill-feedback-urls repairs the GCS feedback-screenshot URLs
// embedded in user-feedback GitHub issues (#2549).
//
// Feedback screenshots were originally signed via the runtime SA's IAM signBlob,
// whose Google-managed key rotates every ~1-2 weeks — so those URLs return HTTP
// 403 SignatureDoesNotMatch long before their stated expiry. This one-shot tool
// re-signs each feedback-system object with the durable user-managed signer key
// and rewrites the issue body, replacing the dead URLs with fresh, durable ones.
//
// It is safe to re-run: each pass re-signs and converges every issue to a valid
// URL. Objects that no longer exist in the bucket are left untouched and logged.
//
// Dry-run by default; pass -apply to actually rewrite issue bodies. Run it only
// AFTER the durable signer is deployed (so newly-filed issues already sign
// durably) — the tool and the server use the same signer key.
//
// PROD below is your production project id; `scripts/gcp_project.sh prod`
// resolves it. The media bucket is named after that project.
//
// Usage:
//
//	# Fetch the signer key locally (needs Secret Manager read via ADC):
//	gcloud secrets versions access latest --secret=feedback-signer-key \
//	  --project="$PROD" > /tmp/fb-signer.json
//
//	# Dry-run a single issue first (prints what it would change):
//	go run ./server/cmd/backfill-feedback-urls \
//	  -gcs-bucket="$PROD" \
//	  -feedback-signer-key-file=/tmp/fb-signer.json \
//	  -github-token="$(gh auth token)" \
//	  -issue=2270
//
//	# Apply to that one issue:
//	go run ./server/cmd/backfill-feedback-urls ... -issue=2270 -apply
//
//	# Then the full batch (all user-feedback issues):
//	go run ./server/cmd/backfill-feedback-urls ... -apply
//	shred -u /tmp/fb-signer.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v90/github"

	"go.ripls.org/ripls/server/storage"
)

// keyRe extracts the object UUID from a feedback-system URL or path.
var keyRe = regexp.MustCompile(`/feedback-system/([0-9a-fA-F-]{36})`)

func main() {
	token := flag.String("github-token", os.Getenv("GITHUB_TOKEN"), "GitHub token with issue write access (or set GITHUB_TOKEN)")
	owner := flag.String("repo-owner", "", "GitHub repository owner (required)")
	repo := flag.String("repo-name", "ripls", "GitHub repository name")
	label := flag.String("label", "user-feedback", "Only scan issues carrying this label (ignored when -issue is set)")
	issueNum := flag.Int("issue", 0, "If set, process only this one issue number (good for a test run before the full batch)")
	bucketName := flag.String("gcs-bucket", "", "GCS media bucket holding feedback screenshots, usually named after the prod project (required)")
	signerKeyFile := flag.String("feedback-signer-key-file", "", "Path to the feedback signer service-account JSON key (required)")
	expiryDays := flag.Int("expiry-days", 365, "Durable signed-URL lifetime in days")
	apply := flag.Bool("apply", false, "Rewrite issue bodies. Default is a dry run that changes nothing.")
	flag.Parse()

	switch {
	case *token == "":
		log.Fatal("--github-token (or GITHUB_TOKEN) is required")
	case *owner == "":
		log.Fatal("--repo-owner is required")
	case *bucketName == "":
		log.Fatal("--gcs-bucket is required")
	case *signerKeyFile == "":
		log.Fatal("--feedback-signer-key-file is required")
	}

	ctx := context.Background()

	email, pem, err := loadSignerKey(*signerKeyFile)
	if err != nil {
		log.Fatalf("load signer key: %v", err)
	}

	bucket, err := storage.NewGCSBucketStorage(ctx, *bucketName, storage.WithURLSigner(email, []byte(pem)))
	if err != nil {
		log.Fatalf("init GCS: %v", err)
	}

	gh, err := github.NewClient(github.WithAuthToken(*token))
	if err != nil {
		log.Fatalf("init GitHub client: %v", err)
	}

	// Match only feedback-system URLs in THIS bucket, with or without a query.
	urlRe := regexp.MustCompile(`https://storage\.googleapis\.com/` +
		regexp.QuoteMeta(*bucketName) + `/feedback-system/[0-9a-fA-F-]{36}(?:\?[^)\s]*)?`)
	expiry := time.Duration(*expiryDays) * 24 * time.Hour

	// resolve re-signs one object key. It returns the fresh URL and true, or
	// false to leave the original in place (object missing or sign failure).
	var missing, errored int
	resolve := func(issueNum int, key string) (string, bool) {
		if _, _, gerr := bucket.Get(ctx, key); gerr != nil {
			log.Printf("issue #%d: object %s missing/unreadable, leaving as-is: %v", issueNum, key, gerr)
			missing++
			return "", false
		}
		signed, serr := bucket.GetDurableSignedURL(ctx, key, expiry)
		if serr != nil {
			log.Printf("issue #%d: sign %s failed, leaving as-is: %v", issueNum, key, serr)
			errored++
			return "", false
		}
		return signed, true
	}

	var scanned, changed, resigned int

	// processIssue scans one issue and rewrites its feedback URLs (or logs the
	// dry-run intent). Shared by the single-issue and full-batch paths.
	processIssue := func(iss *github.Issue) {
		if iss.IsPullRequest() {
			return
		}
		scanned++
		body := iss.GetBody()
		if !strings.Contains(body, "/feedback-system/") {
			return
		}

		num := iss.GetNumber()
		newBody, n := rewriteBody(body, urlRe, func(key string) (string, bool) {
			return resolve(num, key)
		})
		if n == 0 || newBody == body {
			return
		}
		changed++
		resigned += n

		if !*apply {
			log.Printf("issue #%d %q: WOULD rewrite %d screenshot URL(s) (dry-run)", num, iss.GetTitle(), n)
			return
		}
		if _, _, err := gh.Issues.Update(ctx, *owner, *repo, num, github.UpdateIssueRequest{Body: &newBody}); err != nil {
			log.Printf("issue #%d: edit failed: %v", num, err)
			return
		}
		log.Printf("issue #%d %q: rewrote %d screenshot URL(s)", num, iss.GetTitle(), n)
	}

	if *issueNum != 0 {
		// Single-issue mode: fetch just the one issue (ignores -label).
		iss, _, err := gh.Issues.Get(ctx, *owner, *repo, *issueNum)
		if err != nil {
			log.Fatalf("get issue #%d: %v", *issueNum, err)
		}
		processIssue(iss)
	} else {
		opts := &github.IssueListByRepoOptions{
			Labels:      []string{*label},
			State:       "all",
			ListOptions: github.ListOptions{PerPage: 100},
		}
		for {
			issues, resp, err := gh.Issues.ListByRepo(ctx, *owner, *repo, opts)
			if err != nil {
				log.Fatalf("list issues: %v", err)
			}
			for _, iss := range issues {
				processIssue(iss)
			}
			if resp.NextPage == 0 {
				break
			}
			opts.ListOptions.Page = resp.NextPage
		}
	}

	mode := "DRY-RUN (nothing written — pass -apply to persist)"
	if *apply {
		mode = "APPLIED"
	}
	log.Printf("done [%s]: issues scanned=%d changed=%d | urls re-signed=%d missing=%d errored=%d",
		mode, scanned, changed, resigned, missing, errored)
}

// rewriteBody replaces every feedback-system URL in body with a fresh URL from
// resolve. resolve returns the replacement and true, or false to keep the
// original (e.g. the object no longer exists). It returns the rewritten body and
// the number of URLs actually replaced.
func rewriteBody(body string, urlRe *regexp.Regexp, resolve func(key string) (string, bool)) (string, int) {
	replaced := 0
	out := urlRe.ReplaceAllStringFunc(body, func(oldURL string) string {
		m := keyRe.FindStringSubmatch(oldURL)
		if m == nil {
			return oldURL
		}
		newURL, ok := resolve("feedback-system/" + m[1])
		if !ok {
			return oldURL
		}
		replaced++
		return newURL
	})
	return out, replaced
}

// loadSignerKey reads a GCP service-account JSON key file and returns its
// client_email and PEM private_key. Both are required.
func loadSignerKey(path string) (email, privateKeyPEM string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var k struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal(data, &k); err != nil {
		return "", "", fmt.Errorf("parse service-account JSON: %w", err)
	}
	if k.ClientEmail == "" || k.PrivateKey == "" {
		return "", "", fmt.Errorf("service-account key missing client_email or private_key")
	}
	return k.ClientEmail, k.PrivateKey, nil
}
