// eval-seed-from-urls is an interactive ingest tool that turns manufacturer
// product URLs into proposed golden cases for the image-mode gear-detection
// eval (#1777). Manufacturer pages emit Schema.org Product JSON-LD on every
// product page for SEO purposes, so a curated list of ~30–50 URLs across
// brands gives us the diverse, real-world catalog ABO couldn't (ABO is
// Amazon-house-brand-dominated — fatal for brand-detection assertions).
//
// For each URL the tool:
//
//  1. Fetches the page with a sane User-Agent.
//  2. Extracts product metadata from Schema.org Product JSON-LD if present;
//     falls back to OpenGraph + visible <meta> tags otherwise.
//  3. Displays the extracted fields to stdout and prompts the operator to
//     accept, edit any field, skip, or quit-and-save.
//  4. On accept, downloads the hero image and appends a proposed case to
//     the output JSON (rewritten after every accept so Ctrl-C is recoverable).
//
// All I/O is serial. Each URL is one HTTP request + one image fetch; the
// human-in-the-loop confirmation step dominates wall-clock, so parallelism
// would be premature.
//
// Usage:
//
//	go run ./server/cmd/eval-seed-from-urls \
//	  -urls scripts/eval_seed_urls.txt \
//	  -out-cases /tmp/proposed-cases.json \
//	  -out-images server/test_data/manual
//
// Input file format (one URL per line, optional category hint prefix):
//
//	power-tools https://www.dewalt.com/product/dcd771c2/...
//	camping     https://www.rei.com/product/187624/rei-co-op-half-dome-sl-2-tent
//	# blank lines and #-comments are ignored
//	https://www.example.com/no-category-hint
//
// The proposed JSON block is written to -out-cases; the human appends
// accepted entries to server/ai/eval/testdata/gear_image_goldens.json.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const userAgent = "ripls-eval-seed-from-urls/1.0 (+https://github.com/ripls-org/ripls)"

func main() {
	urlsPath := flag.String("urls", "", "Path to a text file with one URL per line (optional 'category URL' prefix)")
	outCases := flag.String("out-cases", "", "Where to write proposed JSON cases (required)")
	outImages := flag.String("out-images", "server/test_data/manual", "Where to download chosen images (relative to repo root)")
	httpTimeout := flag.Duration("http-timeout", 30*time.Second, "Per-request HTTP timeout")
	flag.Parse()

	// One context for the whole run so the HTTP fetches below are cancellable.
	ctx := context.Background()

	if *urlsPath == "" || *outCases == "" {
		log.Fatalf("required: -urls and -out-cases (see package doc)")
	}

	entries, err := readURLList(*urlsPath)
	if err != nil {
		log.Fatalf("read URL list: %v", err)
	}
	if len(entries) == 0 {
		log.Fatalf("no URLs found in %s", *urlsPath)
	}

	if err := os.MkdirAll(*outImages, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", *outImages, err)
	}

	// Resume support: if -out-cases already exists, load it and skip any
	// source URL that's already represented. Lets a partial run pick up
	// where it left off after Ctrl-C.
	existing, err := loadExistingCases(*outCases)
	if err != nil {
		log.Fatalf("load existing cases: %v", err)
	}
	done := map[string]bool{}
	for _, c := range existing {
		done[c.SourceURL] = true
	}
	if len(existing) > 0 {
		log.Printf("resuming: %d cases already present in %s", len(existing), *outCases)
	}

	client := &http.Client{Timeout: *httpTimeout}
	stdin := bufio.NewReader(os.Stdin)
	cases := append([]proposedCase(nil), existing...)

	for i, e := range entries {
		if done[e.URL] {
			continue
		}
		fmt.Printf("\n[%d/%d] %s\n", i+1, len(entries), e.URL)
		extracted, err := extractFromURL(ctx, client, e.URL)
		if err != nil {
			fmt.Printf("  ! fetch/parse failed: %v\n", err)
			fmt.Printf("  [s]kip, [q]uit: ")
			ans := readLine(stdin)
			if ans == "q" {
				break
			}
			continue
		}
		extracted.CategoryHint = e.Category

		action := promptAndEdit(stdin, extracted)
		if action == actionQuit {
			fmt.Println("quitting; saving accepted cases.")
			break
		}
		if action == actionSkip {
			continue
		}
		// actionAccept: finalize and persist immediately.
		c, err := finalizeCase(ctx, client, extracted, *outImages)
		if err != nil {
			fmt.Printf("  ! could not finalize case: %v — skipping\n", err)
			continue
		}
		cases = append(cases, c)
		if err := writeCases(cases, *outCases); err != nil {
			log.Fatalf("write cases: %v", err)
		}
		fmt.Printf("  ✓ wrote case (total: %d)\n", len(cases))
	}

	log.Printf("done. %d cases written to %s.", len(cases), *outCases)
	log.Printf("review the proposed cases, then append accepted ones to server/ai/eval/testdata/gear_image_goldens.json")
}

// --- input parsing ---.

// urlEntry is one line of the input list: a URL plus an optional
// category hint to seed `tags[0]` and bias the reviewer's eye.
type urlEntry struct {
	Category string
	URL      string
}

// readURLList parses the file at path. Each non-blank, non-comment line is
// either "URL" or "category URL". Lines with malformed URLs are reported
// and skipped (the script tolerates a few bad lines so an operator can
// keep curating without blocking on a typo).
func readURLList(path string) ([]urlEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []urlEntry
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var category, urlStr string
		if i := strings.IndexAny(line, " \t"); i >= 0 {
			category = strings.TrimSpace(line[:i])
			urlStr = strings.TrimSpace(line[i+1:])
		} else {
			urlStr = line
		}
		if _, err := url.ParseRequestURI(urlStr); err != nil {
			log.Printf("line %d: skipping malformed URL %q: %v", lineNum, urlStr, err)
			continue
		}
		out = append(out, urlEntry{Category: category, URL: urlStr})
	}
	return out, scanner.Err()
}

// --- types ---.

// extractedFields is the in-memory representation we build from a fetched
// page. Every field is a string so the editor can round-trip user edits;
// numeric fields get parsed at finalization time. ImageURL must be an
// absolute URL — the extractor resolves relative refs against the page URL.
type extractedFields struct {
	SourceURL    string
	CategoryHint string
	Title        string
	Brand        string
	Model        string
	WeightStr    string // raw user-facing form, e.g. "1814 g" or "4 lb"
	WeightGrams  float64
	PriceStr     string // "$99" or "USD 99.00"
	PriceUSD     float64
	ImageURL     string
	Description  string
}

// proposedCase mirrors the gear_image_goldens.json shape so the reviewer
// can paste accepted blocks directly. Fields that the human elected not
// to assert are omitted via omitempty.
type proposedCase struct {
	ID        string         `json:"id"`
	Tags      []string       `json:"tags,omitempty"`
	ImageFile string         `json:"image_file"`
	MimeType  string         `json:"mime_type"`
	SourceURL string         `json:"source_url"`
	Expected  proposedExpect `json:"expected"`
}

type proposedExpect struct {
	TitleContains    []string `json:"title_contains,omitempty"`
	BrandContains    []string `json:"brand_contains,omitempty"`
	ModelContains    []string `json:"model_contains,omitempty"`
	WeightGramsApprx *float64 `json:"weight_grams_approx,omitempty"`
	ValueEstimateUSD *float64 `json:"value_estimate_usd,omitempty"`
	HasDescription   *bool    `json:"has_description,omitempty"`
	MinConfidence    *float64 `json:"min_confidence,omitempty"`
}

// --- extraction ---.

// extractFromURL fetches the page at u, attempts Schema.org JSON-LD
// extraction first, and falls back to OpenGraph meta tags. Returns a
// populated extractedFields with at least Title / ImageURL populated;
// missing optional fields stay zero so the operator can fill them in.
func extractFromURL(ctx context.Context, client *http.Client, u string) (*extractedFields, error) {
	body, err := fetchHTML(ctx, client, u)
	if err != nil {
		return nil, err
	}
	got := &extractedFields{SourceURL: u}

	// Try Schema.org Product JSON-LD first.
	if err := fillFromJSONLD(got, body); err != nil {
		log.Printf("  (json-ld extraction soft-failed: %v)", err)
	}

	// Fill any gaps from OpenGraph / Twitter / standard meta tags.
	fillFromMetaTags(got, body)

	// Resolve a relative image URL against the page URL.
	if got.ImageURL != "" {
		if abs, err := absURL(u, got.ImageURL); err == nil {
			got.ImageURL = abs
		}
	}

	if got.Title == "" && got.ImageURL == "" {
		return nil, fmt.Errorf("no usable product metadata found (no JSON-LD Product, no og:image)")
	}
	return got, nil
}

func fetchHTML(ctx context.Context, client *http.Client, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
}

var jsonLDPattern = regexp.MustCompile(`(?is)<script[^>]+type=["']application/ld\+json["'][^>]*>(.*?)</script>`)

// fillFromJSONLD scans the page for <script type="application/ld+json">
// blocks, decodes each, and copies fields from any block whose @type is
// Product (or whose @graph contains a Product). Missing fields are left
// at their zero value so meta-tag extraction can fill them.
func fillFromJSONLD(got *extractedFields, body []byte) error {
	matches := jsonLDPattern.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		return errors.New("no application/ld+json blocks")
	}
	for _, m := range matches {
		raw := strings.TrimSpace(string(m[1]))
		if raw == "" {
			continue
		}
		var generic any
		if err := json.Unmarshal([]byte(raw), &generic); err != nil {
			continue // skip malformed blocks
		}
		walkJSONLD(generic, got)
	}
	return nil
}

// walkJSONLD recursively walks the decoded JSON-LD structure looking for
// any object with @type "Product" and copies its fields into got. Some
// sites wrap products in @graph arrays or nest them inside CollectionPage.
func walkJSONLD(v any, got *extractedFields) {
	switch n := v.(type) {
	case map[string]any:
		if isProduct(n["@type"]) {
			fillFromProductObject(got, n)
		}
		for _, child := range n {
			walkJSONLD(child, got)
		}
	case []any:
		for _, child := range n {
			walkJSONLD(child, got)
		}
	}
}

func isProduct(t any) bool {
	switch v := t.(type) {
	case string:
		return v == "Product"
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == "Product" {
				return true
			}
		}
	}
	return false
}

// fillFromProductObject extracts the schema.org Product fields from a
// JSON-LD object. Only fields not already populated are overwritten so
// the first-seen Product on a page wins.
func fillFromProductObject(got *extractedFields, p map[string]any) {
	if got.Title == "" {
		got.Title = strings.TrimSpace(asString(p["name"]))
	}
	if got.Brand == "" {
		got.Brand = brandOf(p["brand"])
	}
	if got.Model == "" {
		got.Model = strings.TrimSpace(asString(p["model"]))
		if got.Model == "" {
			got.Model = strings.TrimSpace(asString(p["mpn"]))
		}
	}
	if got.Description == "" {
		got.Description = strings.TrimSpace(asString(p["description"]))
	}
	if got.ImageURL == "" {
		got.ImageURL = imageOf(p["image"])
	}
	if got.WeightGrams == 0 {
		got.WeightGrams, got.WeightStr = weightOf(p["weight"])
	}
	if got.PriceUSD == 0 {
		got.PriceUSD, got.PriceStr = priceOf(p["offers"])
	}
}

// --- field-shape helpers (Schema.org Product fields are wildly polymorphic) ---.

func asString(v any) string {
	switch n := v.(type) {
	case string:
		return n
	case []any:
		for _, item := range n {
			if s, ok := item.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// brandOf accepts a string, a Brand object {name: "..."}, or an array
// containing either.
func brandOf(v any) string {
	switch n := v.(type) {
	case string:
		return strings.TrimSpace(n)
	case map[string]any:
		return strings.TrimSpace(asString(n["name"]))
	case []any:
		for _, item := range n {
			if s := brandOf(item); s != "" {
				return s
			}
		}
	}
	return ""
}

// imageOf accepts a URL string, an ImageObject {url: "..."}, or an array
// of either; returns the first absolute or root-relative URL.
func imageOf(v any) string {
	switch n := v.(type) {
	case string:
		return strings.TrimSpace(n)
	case map[string]any:
		if u := strings.TrimSpace(asString(n["url"])); u != "" {
			return u
		}
		return strings.TrimSpace(asString(n["contentUrl"]))
	case []any:
		for _, item := range n {
			if u := imageOf(item); u != "" {
				return u
			}
		}
	}
	return ""
}

// weightOf accepts a QuantitativeValue {value, unitCode} or
// {value, unitText}. Returns grams + a human-friendly form, or 0 / "".
func weightOf(v any) (float64, string) {
	obj, ok := v.(map[string]any)
	if !ok {
		return 0, ""
	}
	val, ok := asFloat(obj["value"])
	if !ok || val <= 0 {
		return 0, ""
	}
	unit := asString(obj["unitCode"])
	if unit == "" {
		unit = asString(obj["unitText"])
	}
	grams, ok := unitToGrams(unit, val)
	if !ok {
		return 0, ""
	}
	return grams, fmt.Sprintf("%.0f g (source: %.4g %s)", grams, val, unit)
}

// priceOf accepts an Offer or array of Offers. Filters to USD-priced
// entries and returns the first one. Returns 0 / "" if no USD offer.
func priceOf(v any) (float64, string) {
	switch n := v.(type) {
	case map[string]any:
		return offerPrice(n)
	case []any:
		for _, item := range n {
			if p, s := priceOf(item); p > 0 {
				return p, s
			}
		}
	}
	return 0, ""
}

func offerPrice(o map[string]any) (float64, string) {
	currency := asString(o["priceCurrency"])
	if currency != "" && !strings.EqualFold(currency, "USD") {
		return 0, ""
	}
	val, ok := asFloat(o["price"])
	if !ok || val <= 0 {
		return 0, ""
	}
	if currency == "" {
		currency = "USD"
	}
	return val, fmt.Sprintf("$%.2f %s", val, currency)
}

// asFloat tolerates the string-encoded numerics that JSON-LD producers
// frequently emit ("99.00") alongside genuine numbers.
func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

// unitToGrams converts a UN/CEFACT unit code (e.g. LBR, KGM, GRM, ONZ)
// or a unit text ("lb", "kg", "g", "oz") to grams.
func unitToGrams(unit string, value float64) (float64, bool) {
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "GRM", "GRAM", "GRAMS", "G":
		return value, true
	case "KGM", "KILOGRAM", "KILOGRAMS", "KG":
		return value * 1000, true
	case "LBR", "POUND", "POUNDS", "LB", "LBS":
		return value * 453.592, true
	case "ONZ", "OUNCE", "OUNCES", "OZ":
		return value * 28.3495, true
	}
	return 0, false
}

// --- meta-tag fallback ---.

var metaPattern = regexp.MustCompile(`(?is)<meta\s+[^>]*?(?:property|name)=["']([^"']+)["'][^>]*?content=["']([^"']+)["'][^>]*?>`)

// fillFromMetaTags reads OpenGraph and standard <meta> tags to fill any
// fields the JSON-LD step left blank. Operates on the raw page bytes
// because we don't need a full DOM for `<meta>` extraction.
func fillFromMetaTags(got *extractedFields, body []byte) {
	tags := map[string]string{}
	for _, m := range metaPattern.FindAllSubmatch(body, -1) {
		key := strings.ToLower(string(m[1]))
		val := html(string(m[2]))
		if _, exists := tags[key]; !exists {
			tags[key] = val
		}
	}

	if got.Title == "" {
		got.Title = firstNonEmpty(tags["og:title"], tags["twitter:title"], extractTitleTag(body))
	}
	if got.Description == "" {
		got.Description = firstNonEmpty(tags["og:description"], tags["twitter:description"], tags["description"])
	}
	if got.ImageURL == "" {
		got.ImageURL = firstNonEmpty(tags["og:image"], tags["twitter:image"])
	}
	if got.Brand == "" {
		got.Brand = firstNonEmpty(tags["product:brand"], tags["og:brand"])
	}
	if got.PriceUSD == 0 {
		amount := firstNonEmpty(tags["product:price:amount"], tags["og:price:amount"])
		currency := firstNonEmpty(tags["product:price:currency"], tags["og:price:currency"])
		if amount != "" && (currency == "" || strings.EqualFold(currency, "USD")) {
			if v, err := strconv.ParseFloat(strings.TrimSpace(amount), 64); err == nil && v > 0 {
				got.PriceUSD = v
				got.PriceStr = fmt.Sprintf("$%.2f USD", v)
			}
		}
	}
}

var titleTagPattern = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func extractTitleTag(body []byte) string {
	if m := titleTagPattern.FindSubmatch(body); m != nil {
		return strings.TrimSpace(html(string(m[1])))
	}
	return ""
}

// html does a minimal HTML-entity decode for the handful of entities we
// see in title / description content. Full decoding would need
// golang.org/x/net/html which would push the dep cost up for marginal value.
func html(s string) string {
	r := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&apos;", "'",
		"&#39;", "'",
		"&nbsp;", " ",
	)
	return r.Replace(s)
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// absURL resolves ref against base. Returns ref unchanged on parse error.
func absURL(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return ref, err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref, err
	}
	return b.ResolveReference(r).String(), nil
}

// --- interactive prompt ---.

type promptAction int

const (
	actionAccept promptAction = iota
	actionSkip
	actionQuit
)

// promptAndEdit displays the extracted fields and loops on the operator's
// choice until they accept, skip, or quit. Edit operations stay in this
// loop and re-display the (now updated) fields.
func promptAndEdit(stdin *bufio.Reader, e *extractedFields) promptAction {
	for {
		printExtracted(e)
		fmt.Print("[a]ccept, [e]dit field, [s]kip, [q]uit and save: ")
		switch readLine(stdin) {
		case "a", "":
			return actionAccept
		case "s":
			return actionSkip
		case "q":
			return actionQuit
		case "e":
			editField(stdin, e)
		default:
			fmt.Println("  (unrecognized — type a, e, s, or q)")
		}
	}
}

func printExtracted(e *extractedFields) {
	hint := e.CategoryHint
	if hint == "" {
		hint = "(none — set with `e` if you want a tag)"
	}
	fmt.Printf("  category: %s\n", hint)
	fmt.Printf("  title:    %s\n", e.Title)
	fmt.Printf("  brand:    %s\n", e.Brand)
	fmt.Printf("  model:    %s\n", e.Model)
	if e.WeightGrams > 0 {
		fmt.Printf("  weight:   %.0f g\n", e.WeightGrams)
	} else {
		fmt.Printf("  weight:   (not extracted)\n")
	}
	if e.PriceUSD > 0 {
		fmt.Printf("  price:    $%.2f USD\n", e.PriceUSD)
	} else {
		fmt.Printf("  price:    (not extracted)\n")
	}
	fmt.Printf("  image:    %s\n", e.ImageURL)
	if e.Description != "" {
		fmt.Printf("  desc:     %s\n", truncate(e.Description, 120))
	}
}

// editField prompts for a field name and a new value, parsing weight and
// price specially so the operator can type "1814 g" or "$99" naturally.
func editField(stdin *bufio.Reader, e *extractedFields) {
	fmt.Print("  field (category/title/brand/model/weight/price/image/desc): ")
	field := strings.ToLower(readLine(stdin))
	switch field {
	case "category":
		fmt.Printf("  new category (current: %q): ", e.CategoryHint)
		e.CategoryHint = readLine(stdin)
	case "title":
		fmt.Printf("  new title (current: %q): ", e.Title)
		e.Title = readLine(stdin)
	case "brand":
		fmt.Printf("  new brand (current: %q): ", e.Brand)
		e.Brand = readLine(stdin)
	case "model":
		fmt.Printf("  new model (current: %q): ", e.Model)
		e.Model = readLine(stdin)
	case "weight":
		fmt.Printf("  new weight (current: %.0f g; format e.g. '1800 g' or '4 lb'): ", e.WeightGrams)
		raw := readLine(stdin)
		if raw == "" {
			return
		}
		if g, ok := parseWeightInput(raw); ok {
			e.WeightGrams = g
			e.WeightStr = fmt.Sprintf("%.0f g (operator-edited from %q)", g, raw)
		} else {
			fmt.Printf("  ! could not parse %q (try '1800 g' or '4 lb')\n", raw)
		}
	case "price":
		fmt.Printf("  new price USD (current: %.2f; format e.g. '99' or '$99.00'): ", e.PriceUSD)
		raw := strings.TrimPrefix(strings.TrimSpace(readLine(stdin)), "$")
		if raw == "" {
			return
		}
		if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 {
			e.PriceUSD = v
			e.PriceStr = fmt.Sprintf("$%.2f USD (operator-edited)", v)
		} else {
			fmt.Printf("  ! could not parse %q\n", raw)
		}
	case "image":
		fmt.Printf("  new image URL (current: %s): ", e.ImageURL)
		e.ImageURL = readLine(stdin)
	case "desc":
		fmt.Printf("  new description (current: %s): ", truncate(e.Description, 60))
		e.Description = readLine(stdin)
	default:
		fmt.Printf("  (unknown field %q)\n", field)
	}
}

func parseWeightInput(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	// Split off the unit suffix. Allow no-space form too ("1800g").
	for i, r := range s {
		if r >= '0' && r <= '9' || r == '.' {
			continue
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(s[:i]), 64)
		if err != nil {
			return 0, false
		}
		unit := strings.TrimSpace(s[i:])
		if unit == "" {
			return val, true // bare number → grams
		}
		if g, ok := unitToGrams(unit, val); ok {
			return g, true
		}
		return 0, false
	}
	// Whole string is numeric → grams.
	if val, err := strconv.ParseFloat(s, 64); err == nil {
		return val, true
	}
	return 0, false
}

func readLine(r *bufio.Reader) string {
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return strings.TrimSpace(line)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// --- finalization ---.

// finalizeCase downloads the hero image to the output dir and constructs
// the proposedCase that will be written to -out-cases.
func finalizeCase(ctx context.Context, client *http.Client, e *extractedFields, outImagesDir string) (proposedCase, error) {
	id, err := caseIDFromURL(e.SourceURL)
	if err != nil {
		return proposedCase{}, err
	}
	ext := imageExtFromURL(e.ImageURL)
	imagePath := filepath.Join(outImagesDir, id+ext)
	mime, err := downloadImage(ctx, client, e.ImageURL, imagePath)
	if err != nil {
		return proposedCase{}, err
	}

	weightPtr := nilIfZero(e.WeightGrams)
	pricePtr := nilIfZero(e.PriceUSD)
	hasDesc := e.Description != ""
	minConf := 0.5

	c := proposedCase{
		ID:        id,
		Tags:      tagsFor(e),
		ImageFile: filepath.Base(outImagesDir) + "/" + id + ext,
		MimeType:  mime,
		SourceURL: e.SourceURL,
		Expected: proposedExpect{
			TitleContains:    titleSubstrings(e),
			WeightGramsApprx: weightPtr,
			ValueEstimateUSD: pricePtr,
			HasDescription:   &hasDesc,
			MinConfidence:    &minConf,
		},
	}
	if e.Brand != "" {
		c.Expected.BrandContains = []string{strings.ToLower(strings.TrimSpace(e.Brand))}
	}
	if m := strings.TrimSpace(e.Model); m != "" && !looksLikeSKU(m) {
		c.Expected.ModelContains = []string{strings.ToLower(m)}
	}
	return c, nil
}

func nilIfZero(v float64) *float64 {
	if v <= 0 {
		return nil
	}
	return &v
}

// caseIDFromURL builds a stable, alphanumeric case ID from the URL host
// + the last path segment. e.g. "rei.com/.../half-dome-sl-2-tent" →
// "rei_com_half_dome_sl_2_tent".
func caseIDFromURL(u string) (string, error) {
	parsed, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	host := strings.TrimPrefix(parsed.Host, "www.")
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	last := segments[len(segments)-1]
	return sanitize(host + "_" + last), nil
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '.' || r == '/' || r == '_' || r == ' ':
			if b.Len() > 0 && b.String()[b.Len()-1] != '_' {
				b.WriteRune('_')
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func imageExtFromURL(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	ext := strings.ToLower(filepath.Ext(u))
	switch ext {
	case extJPG, ".jpeg", ".png", ".webp":
		return ext
	default:
		return extJPG
	}
}

// downloadImage fetches the URL and writes the body to dst. Returns the
// MIME type derived from the response Content-Type header (or from the
// extension as a fallback).
func downloadImage(ctx context.Context, client *http.Client, u, dst string) (string, error) {
	if u == "" {
		return "", errors.New("empty image URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}

	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, io.LimitReader(resp.Body, 50*1024*1024)); err != nil {
		return "", err
	}

	mime := resp.Header.Get("Content-Type")
	if i := strings.IndexAny(mime, ";"); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		mime = mimeFromExt(filepath.Ext(dst))
	}
	return mime, nil
}

func mimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case extJPG, ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

// tagsFor returns the tags slice for a case: category hint first if set,
// then a derived tag from the URL host so reviewers can spot per-vendor
// concentration.
func tagsFor(e *extractedFields) []string {
	var out []string
	if e.CategoryHint != "" {
		out = append(out, e.CategoryHint)
	}
	if u, err := url.Parse(e.SourceURL); err == nil {
		host := strings.TrimPrefix(u.Host, "www.")
		if i := strings.IndexByte(host, '.'); i > 0 {
			out = append(out, "vendor:"+host[:i])
		}
	}
	return out
}

// titleSubstrings picks a handful of useful tokens from the product title.
// We include lowercase words ≥4 chars, skipping the brand (already
// asserted separately) and a small stopword set.
func titleSubstrings(e *extractedFields) []string {
	if e.Title == "" {
		return nil
	}
	brand := strings.ToLower(strings.TrimSpace(e.Brand))
	tokens := strings.Fields(strings.ToLower(e.Title))
	var out []string
	for _, t := range tokens {
		t = strings.Trim(t, ",.()[]{}\"'")
		if len(t) < 4 || isStopword(t) {
			continue
		}
		if brand != "" && t == brand {
			continue
		}
		out = append(out, t)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func isStopword(t string) bool {
	switch t {
	case "the", "and", "with", "from", "your", "this", "that",
		"new", "set", "kit", "pack", "size", "small", "medium",
		"large", "inch", "inches", "for":
		return true
	}
	return false
}

// looksLikeSKU returns true for strings that look like internal SKU codes
// (alphanumeric soup, no spaces, a digit somewhere in the middle).
// Vision models won't reproduce these, so asserting model_contains on
// them only generates false negatives.
func looksLikeSKU(s string) bool {
	if strings.ContainsAny(s, " /") {
		return false
	}
	hasLetter, hasDigit := false, false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}

// --- output ---.

func loadExistingCases(path string) ([]proposedCase, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var out []proposedCase
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return out, nil
}

func writeCases(cases []proposedCase, path string) error {
	data, err := json.MarshalIndent(cases, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
