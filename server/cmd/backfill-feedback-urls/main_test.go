package main

import (
	"regexp"
	"strings"
	"testing"
)

// bucketURLRe mirrors the pattern main() builds for a given bucket.
func bucketURLRe(bucket string) *regexp.Regexp {
	return regexp.MustCompile(`https://storage\.googleapis\.com/` +
		regexp.QuoteMeta(bucket) + `/feedback-system/[0-9a-fA-F-]{36}(?:\?[^)\s]*)?`)
}

const (
	uuidA = "14559cb8-3e7a-44e6-b2b9-353e28f4b35c"
	uuidB = "a1b2c3d4-e5f6-4788-9a0b-1c2d3e4f5061"
)

func TestRewriteBody_ReplacesEachFeedbackURL(t *testing.T) {
	re := bucketURLRe("example-prod")
	body := "## Screenshots\n\n" +
		"![Screenshot 1](https://storage.googleapis.com/example-prod/feedback-system/" + uuidA + "?Expires=1&Signature=OLD%2Fdead)\n\n" +
		"![Screenshot 2](https://storage.googleapis.com/example-prod/feedback-system/" + uuidB + "?Expires=2&Signature=alsodead)\n"

	var gotKeys []string
	out, n := rewriteBody(body, re, func(key string) (string, bool) {
		gotKeys = append(gotKeys, key)
		return "https://storage.googleapis.com/example-prod/" + key + "?fresh=1", true
	})

	if n != 2 {
		t.Fatalf("replaced = %d, want 2", n)
	}
	wantKeys := []string{"feedback-system/" + uuidA, "feedback-system/" + uuidB}
	if strings.Join(gotKeys, ",") != strings.Join(wantKeys, ",") {
		t.Errorf("keys = %v, want %v", gotKeys, wantKeys)
	}
	if strings.Contains(out, "dead") || strings.Contains(out, "alsodead") {
		t.Errorf("old signatures still present:\n%s", out)
	}
	if strings.Count(out, "fresh=1") != 2 {
		t.Errorf("expected 2 fresh URLs, got:\n%s", out)
	}
	// The markdown scaffolding and alt text must survive untouched.
	if !strings.Contains(out, "![Screenshot 1](") || !strings.Contains(out, "## Screenshots") {
		t.Errorf("markdown structure damaged:\n%s", out)
	}
}

func TestRewriteBody_KeepsOriginalWhenResolveFails(t *testing.T) {
	re := bucketURLRe("example-prod")
	orig := "![s](https://storage.googleapis.com/example-prod/feedback-system/" + uuidA + "?Expires=1&Signature=keepme)"

	out, n := rewriteBody(orig, re, func(string) (string, bool) {
		return "", false // e.g. object missing
	})

	if n != 0 {
		t.Errorf("replaced = %d, want 0 when resolve declines", n)
	}
	if out != orig {
		t.Errorf("body changed despite declined resolve:\ngot  %s\nwant %s", out, orig)
	}
}

func TestRewriteBody_IgnoresOtherBucketsAndPaths(t *testing.T) {
	re := bucketURLRe("example-prod")
	// A different bucket, and a normal media object (not feedback-system) —
	// neither should be touched.
	body := "![other](https://storage.googleapis.com/example-dev/feedback-system/" + uuidA + "?x=1) " +
		"![media](https://storage.googleapis.com/example-prod/user123/" + uuidB + "?y=2)"

	out, n := rewriteBody(body, re, func(string) (string, bool) {
		return "REPLACED", true
	})

	if n != 0 {
		t.Errorf("replaced = %d, want 0 (wrong bucket / non-feedback path)", n)
	}
	if out != body {
		t.Errorf("unrelated URLs were modified:\n%s", out)
	}
}

func TestRewriteBody_MatchesBarePathWithoutQuery(t *testing.T) {
	re := bucketURLRe("example-prod")
	body := "url: https://storage.googleapis.com/example-prod/feedback-system/" + uuidA + " end"

	out, n := rewriteBody(body, re, func(string) (string, bool) {
		return "NEW", true
	})
	if n != 1 {
		t.Fatalf("replaced = %d, want 1", n)
	}
	if !strings.Contains(out, "url: NEW end") {
		t.Errorf("unexpected rewrite: %s", out)
	}
}
