//go:build integration

package webfetch

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// retailerTestCase defines a test for fetching a real product page.
type retailerTestCase struct {
	name string
	url  string
}

// retailerTests contains product URLs from major retailers and manufacturers.
// Prefer stable URLs (category pages, evergreen products) over specific SKUs
// that may be discontinued. Update URLs when they go stale.
//
// Last baseline: 2026-04-03.
var retailerTests = []retailerTestCase{
	// Outdoor / Sporting
	{name: "REI", url: "https://www.rei.com/product/171554/patagonia-nano-puff-jacket-mens"},
	{name: "Backcountry", url: "https://www.backcountry.com/the-north-face-thermoball-eco-jacket-mens"},
	{name: "Moosejaw", url: "https://www.moosejaw.com/content/about-moosejaw"},
	{name: "Evo", url: "https://www.evo.com/shop/ski/jackets"},
	{name: "Sierra Trading Post", url: "https://www.sierra.com/outdoor-recreation~d~142/"},
	{name: "Steep and Cheap", url: "https://www.steepandcheap.com/mens-jackets"},

	// General retail
	{name: "Amazon", url: "https://www.amazon.com/dp/B0FY6X2XXY"},
	{name: "Amazon search", url: "https://www.amazon.com/s?k=camping+tent"},
	{name: "Walmart", url: "https://www.walmart.com/browse/sports-outdoors/camping/4125_546956"},
	{name: "Target", url: "https://www.target.com/p/4-person-dome-camping-tent-rust-embark-8482/-/A-88946142"},
	{name: "Best Buy", url: "https://www.bestbuy.com/site/searchpage.jsp?st=dyson"},
	{name: "Costco", url: "https://www.costco.com/outdoor-furniture.html"},

	// Home improvement
	{name: "Home Depot", url: "https://www.homedepot.com/b/Tools/N-5yc1v"},
	{name: "Lowe's", url: "https://www.lowes.com/l/shop/tools"},

	// Manufacturers
	{name: "Patagonia", url: "https://www.patagonia.com/product/mens-nano-puff-jacket/84212.html"},
	{name: "North Face", url: "https://www.thenorthface.com/en-us/p/mens/mens-jackets-and-vests/mens-rainwear-299284/mens-antora-jacket-NF0A7QEY"},
	{name: "Osprey", url: "https://www.osprey.com/atmos-ag-65"},
	{name: "Thule", url: "https://www.thule.com/us/en/bike-rack/trunk-bike-racks/thule-gateway-pro-2-_-900600"},
	{name: "DeWalt", url: "https://www.dewalt.com/en-us/product/dcd801t1/dewalt-20v-max-xr-12-drilldriver-kit-6ah-flexvolt-battery"},
	{name: "Makita", url: "https://www.makitatools.com/products/details/XFD131"},
	{name: "Milwaukee", url: "https://www.milwaukeetool.com/Products/Power-Tools/Drilling"},
	{name: "Weber", url: "https://www.weber.com/US/en/gas-grills/"},
	{name: "Dyson", url: "https://www.dyson.com/vacuum-cleaners/cordless"},
	{name: "Coleman", url: "https://www.coleman.com/tents/"},
	{name: "Black Diamond", url: "https://blackdiamondequipment.com/products/half-dome-helmet"},
	{name: "Yeti", url: "https://www.yeti.com/coolers/hard-coolers"},

	// Marketplace / Used
	{name: "eBay", url: "https://www.ebay.com/b/Camping-Hiking/36114/bn_1865597"},
	{name: "Craigslist", url: "https://sfbay.craigslist.org/"},
}

// TestFetcher_RetailerAccess tests whether the fetcher can successfully
// retrieve content from major retailer and manufacturer websites.
//
// This test suite measures the current success rate and documents failure
// modes. It does NOT fail on individual site failures — instead it reports
// results and fails only if the overall success rate drops below 25%.
func TestFetcher_RetailerAccess(t *testing.T) {
	fetcher := NewHTTPFetcher(WithTimeout(15 * time.Second))
	ctx := context.Background()

	var passed, failed int
	var failures []string

	for _, tc := range retailerTests {
		t.Run(tc.name, func(t *testing.T) {
			content, err := fetcher.FetchPageContent(ctx, tc.url)
			if err != nil {
				var fetchErr *FetchError
				if errors.As(err, &fetchErr) {
					t.Logf("FAILED: %s — %s (kind=%d, status=%d)", tc.name, fetchErr.Message, fetchErr.Kind, fetchErr.StatusCode)
				} else {
					t.Logf("FAILED: %s — %v", tc.name, err)
				}
				failed++
				failures = append(failures, fmt.Sprintf("%s: %v", tc.name, err))
				return
			}

			if content.Title == "" && content.BodyText == "" {
				t.Logf("FAILED: %s — got 200 but no content (empty title and body)", tc.name)
				failed++
				failures = append(failures, fmt.Sprintf("%s: empty content", tc.name))
				return
			}

			t.Logf("OK: %s — title=%q, body_len=%d, image=%v",
				tc.name, truncate(content.Title, 60), len(content.BodyText), content.ImageURL != "")
			passed++
		})
	}

	// Summary.
	total := passed + failed
	if total == 0 {
		t.Fatal("no retailer tests ran")
	}
	successRate := float64(passed) / float64(total) * 100
	t.Logf("\n=== RETAILER FETCH SUMMARY ===")
	t.Logf("Total: %d | Passed: %d | Failed: %d | Success rate: %.0f%%", total, passed, failed, successRate)
	if len(failures) > 0 {
		t.Logf("Failures:")
		for _, f := range failures {
			t.Logf("  - %s", f)
		}
	}

	// Fail if success rate is below 25% — something is fundamentally broken.
	if successRate < 25 {
		t.Errorf("success rate %.0f%% is below 25%% threshold", successRate)
	}
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
