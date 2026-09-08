// Command generate_simulation_data fetches external data for simulation assets.
//
// Subcommands:
//
//	venues  - Query OpenStreetMap for real venue data (default, no API key needed)
//	images  - Download stock photos from Pixabay (requires PIXABAY_API_KEY or -pixabay-key)
//
// Usage:
//
//	go run ./server/cmd/generate_simulation_data/
//	go run ./server/cmd/generate_simulation_data/ venues
//	go run ./server/cmd/generate_simulation_data/ images -pixabay-key=YOUR_KEY
//	PIXABAY_API_KEY=YOUR_KEY go run ./server/cmd/generate_simulation_data/ images
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.ripls.org/ripls/server/simulation"
)

const overpassEndpoint = "https://overpass-api.de/api/interpreter"

// overpassResponse is the JSON structure returned by the Overpass API.
type overpassResponse struct {
	Elements []overpassElement `json:"elements"`
}

// overpassElement is a single OSM node/way/relation returned by Overpass.
type overpassElement struct {
	Type   string            `json:"type"`
	ID     int64             `json:"id"`
	Lat    float64           `json:"lat"`
	Lon    float64           `json:"lon"`
	Center *overpassCenter   `json:"center,omitempty"`
	Tags   map[string]string `json:"tags"`
}

type overpassCenter struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func main() {
	pixabayKey := flag.String("pixabay-key", os.Getenv("PIXABAY_API_KEY"), "Pixabay API key for image downloads")
	flag.Parse()

	subcommand := "venues"
	if flag.NArg() > 0 {
		subcommand = flag.Arg(0)
	}

	switch subcommand {
	case "venues":
		fetchVenues()
	case "images":
		downloadImages(*pixabayKey)
	default:
		log.Fatalf("Unknown subcommand %q. Use 'venues' or 'images'.", subcommand)
	}
}

// fetchVenues queries OpenStreetMap for venue data.
func fetchVenues() {
	categories := venueCategories()
	allVenues := make(map[string][]simulation.Venue)

	for _, city := range simulation.Cities {
		log.Printf("Fetching venues for %s, %s...", city.Name, city.State)

		radiusMeters := int(city.ResidentialRadiusKm * 1000 * 2)

		elements, err := queryAllCategories(city, categories, radiusMeters)
		if err != nil {
			log.Printf("  Error: %v", err)
			continue
		}

		// Elements come back in category order (multiple out statements).
		// Parse and tag each element with its category.
		cityVenues := assignCategories(elements, categories, city)

		log.Printf("  Got %d venues for %s", len(cityVenues), city.Name)
		allVenues[city.Name] = cityVenues

		// Generous delay between cities to avoid rate limiting.
		log.Printf("  Waiting 10s before next city...")
		time.Sleep(10 * time.Second)
	}

	writeOutput(allVenues)
}

// queryAllCategories sends a single Overpass query per city with multiple
// search+out statement pairs, one per category.
func queryAllCategories(city simulation.City, categories []venueCategory, radiusMeters int) ([]overpassElement, error) {
	// Build a single query with multiple nwr/out pairs.
	// Each pair searches for one category and outputs up to N results.
	// Overpass processes these sequentially and returns all results concatenated.
	var stmts []string
	for _, cat := range categories {
		stmt := fmt.Sprintf(
			`nwr["%s"="%s"]["name"](around:%d,%f,%f);out center %d;`,
			cat.Key, cat.Value, radiusMeters,
			city.Center.Latitude, city.Center.Longitude,
			cat.Limit,
		)
		stmts = append(stmts, stmt)
	}

	query := fmt.Sprintf("[out:json][timeout:180];%s", strings.Join(stmts, ""))

	reqURL := fmt.Sprintf("%s?data=%s", overpassEndpoint, url.QueryEscape(query))
	log.Printf("  Query length: %d bytes", len(query))

	client := &http.Client{Timeout: 210 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var result overpassResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse JSON: %w (body: %s)", err, truncate(string(body), 300))
	}

	return result.Elements, nil
}

// assignCategories distributes the flat element list back into categories.
// Since Overpass returns results in statement order with per-statement limits,
// we can reconstruct which category each element belongs to by tracking the
// tag match.
func assignCategories(elements []overpassElement, categories []venueCategory, city simulation.City) []simulation.Venue {
	var venues []simulation.Venue
	seen := make(map[int64]bool)

	for _, e := range elements {
		if seen[e.ID] {
			continue
		}

		name := e.Tags["name"]
		if name == "" {
			continue
		}

		lat, lon := e.Lat, e.Lon
		if e.Center != nil {
			lat, lon = e.Center.Lat, e.Center.Lon
		}
		if lat == 0 && lon == 0 {
			continue
		}

		// Determine category from element tags.
		category := matchCategory(e.Tags, categories)

		seen[e.ID] = true
		venues = append(venues, simulation.Venue{
			Name:        name,
			FullAddress: buildAddress(e.Tags, city),
			Category:    category,
			City:        city.Name,
			Coordinates: simulation.Coordinates{
				Latitude:  lat,
				Longitude: lon,
			},
		})
	}

	return venues
}

// matchCategory finds which category an element belongs to by checking its tags.
func matchCategory(tags map[string]string, categories []venueCategory) string {
	for _, cat := range categories {
		if tags[cat.Key] == cat.Value {
			return cat.Category
		}
	}
	return "other"
}

// buildAddress constructs a readable address from OSM addr:* tags.
func buildAddress(tags map[string]string, city simulation.City) string {
	var parts []string

	num := tags["addr:housenumber"]
	street := tags["addr:street"]
	if street != "" {
		if num != "" {
			parts = append(parts, num+" "+street)
		} else {
			parts = append(parts, street)
		}
	}

	locality := tags["addr:city"]
	if locality == "" {
		locality = city.Name
	}
	state := tags["addr:state"]
	if state == "" {
		state = city.State
	}
	parts = append(parts, locality)
	parts = append(parts, state)

	if zip := tags["addr:postcode"]; zip != "" {
		parts = append(parts, zip)
	}

	return strings.Join(parts, ", ")
}

func writeOutput(allVenues map[string][]simulation.Venue) {
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	outPath := filepath.Join(repoRoot, "server", "simulation", "data", "venues.json")

	data, err := json.MarshalIndent(allVenues, "", "  ")
	if err != nil {
		log.Fatalf("Marshal venues: %v", err)
	}

	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		log.Fatalf("Write venues.json: %v", err)
	}

	total := 0
	for city, venues := range allVenues {
		total += len(venues)
		catCounts := make(map[string]int)
		for _, v := range venues {
			catCounts[v.Category]++
		}
		log.Printf("  %s: %d venues %v", city, len(venues), catCounts)
	}
	log.Printf("Total: %d venues across %d cities", total, len(allVenues))
	log.Printf("Wrote %d bytes to %s", len(data), outPath)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
