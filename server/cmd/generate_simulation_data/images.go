package main

import (
	"encoding/json"
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

// pixabayResponse is the JSON structure returned by the Pixabay API.
type pixabayResponse struct {
	Total     int            `json:"total"`
	TotalHits int            `json:"totalHits"`
	Hits      []pixabayPhoto `json:"hits"`
}

// pixabayPhoto is a single photo from Pixabay.
type pixabayPhoto struct {
	ID            int    `json:"id"`
	PageURL       string `json:"pageURL"`
	PreviewURL    string `json:"previewURL"`
	WebformatURL  string `json:"webformatURL"`
	LargeImageURL string `json:"largeImageURL"`
	User          string `json:"user"`
	UserID        int    `json:"user_id"`
	Tags          string `json:"tags"`
}

// downloadImages fetches one stock photo per catalog item from Pixabay.
// Images are saved with filenames matching the catalog index so the simulation
// can look them up directly.
func downloadImages(apiKey string) {
	if apiKey == "" {
		log.Fatal("Pixabay API key required. Set PIXABAY_API_KEY or pass -pixabay-key flag.")
	}

	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	assetsDir := filepath.Join(repoRoot, "server", "simulation", "assets")

	client := &http.Client{Timeout: 30 * time.Second}

	var attribution []attributionEntry

	// Gear: one image per catalog item.
	// Fallback: item name + category -> item name alone -> category alone.
	gearDir := filepath.Join(assetsDir, "gear")
	if err := os.MkdirAll(gearDir, 0o755); err != nil {
		log.Fatalf("Create gear dir: %v", err)
	}
	allGear := allGearItems()
	log.Printf("Downloading images for %d gear items...", len(allGear))
	for i, g := range allGear {
		queries := []string{
			fmt.Sprintf("%s %s", g.Name, strings.ToLower(g.Category)),
			g.Name,
			strings.ToLower(g.Category),
		}
		filename := fmt.Sprintf("gear_%03d.jpg", i)
		a := downloadForItem(client, apiKey, queries, gearDir, filename, "gear")
		if a != nil {
			a.ItemName = g.Name
			attribution = append(attribution, *a)
		}
	}

	// Experiences: one image per template.
	// Fallback: experience name -> experience category.
	expDir := filepath.Join(assetsDir, "experiences")
	if err := os.MkdirAll(expDir, 0o755); err != nil {
		log.Fatalf("Create experiences dir: %v", err)
	}
	experiences, err := simulation.LoadExperienceTemplates()
	if err != nil {
		log.Fatalf("Load experience templates: %v", err)
	}
	log.Printf("Downloading images for %d experience templates...", len(experiences))
	for i, e := range experiences {
		queries := []string{e.Name}
		if e.Category != "" {
			queries = append(queries, e.Category)
		}
		filename := fmt.Sprintf("exp_%03d.jpg", i)
		a := downloadForItem(client, apiKey, queries, expDir, filename, "experiences")
		if a != nil {
			a.ItemName = e.Name
			attribution = append(attribution, *a)
		}
	}

	// Requests: one image per template.
	// Fallback: request title -> request category.
	reqDir := filepath.Join(assetsDir, "requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		log.Fatalf("Create requests dir: %v", err)
	}
	requests, err := simulation.LoadRequestTemplates()
	if err != nil {
		log.Fatalf("Load request templates: %v", err)
	}
	log.Printf("Downloading images for %d request templates...", len(requests))
	for i, r := range requests {
		queries := []string{r.Title}
		if r.Category != "" {
			queries = append(queries, r.Category)
		}
		filename := fmt.Sprintf("req_%03d.jpg", i)
		a := downloadForItem(client, apiKey, queries, reqDir, filename, "requests")
		if a != nil {
			a.ItemName = r.Title
			attribution = append(attribution, *a)
		}
	}

	// Profiles: one image per unique user across all scenarios.
	profileDir := filepath.Join(assetsDir, "profiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		log.Fatalf("Create profiles dir: %v", err)
	}
	emails := simulation.AllUniqueEmails()
	log.Printf("Downloading profile images for %d users...", len(emails))
	for i, email := range emails {
		queries := []string{"friendly person portrait outdoor"}
		// Vary page to get different results for each user.
		filename := fmt.Sprintf("profile_%03d.jpg", i)
		a := downloadForItemPage(client, apiKey, queries, profileDir, filename, "profiles", (i%10)+1)
		if a != nil {
			a.ItemName = email
			attribution = append(attribution, *a)
		}
	}

	// Communities: one image per unique community across all scenarios.
	commDir := filepath.Join(assetsDir, "communities")
	if err := os.MkdirAll(commDir, 0o755); err != nil {
		log.Fatalf("Create communities dir: %v", err)
	}
	communities := simulation.AllUniqueCommunities()
	log.Printf("Downloading images for %d communities...", len(communities))
	for i, c := range communities {
		queries := []string{c.Description, c.Name}
		filename := fmt.Sprintf("community_%03d.jpg", i)
		a := downloadForItem(client, apiKey, queries, commDir, filename, "communities")
		if a != nil {
			a.ItemName = c.Name
			attribution = append(attribution, *a)
		}
	}

	writeAttributionFile(assetsDir, attribution)
	log.Printf("Done! Images saved to %s", assetsDir)
}

// attributionEntry records the source of a downloaded image.
type attributionEntry struct {
	Category     string
	ItemName     string
	Filename     string
	Photographer string
	SourceURL    string
}

// downloadForItem searches Pixabay and downloads a single image for one catalog item.
// It tries the primary query first, then falls back to each alternative query in order.
func downloadForItem(client *http.Client, apiKey string, queries []string, dir, filename, category string) *attributionEntry {
	return downloadForItemPage(client, apiKey, queries, dir, filename, category, 1)
}

// downloadForItemPage searches Pixabay with a specific page number and fallback queries.
func downloadForItemPage(client *http.Client, apiKey string, queries []string, dir, filename, category string, page int) *attributionEntry {
	outPath := filepath.Join(dir, filename)

	// Skip if already downloaded.
	if _, err := os.Stat(outPath); err == nil {
		return nil
	}

	// Try each query in order until one returns results.
	var photo *pixabayPhoto
	var usedQuery string
	for _, query := range queries {
		photos, err := searchPixabay(client, apiKey, query, 3, page)
		if err != nil {
			log.Printf("  [%s] %s: search failed: %v", category, query, err)
			time.Sleep(2 * time.Second)
			continue
		}
		if len(photos) > 0 {
			photo = &photos[0]
			usedQuery = query
			break
		}
		log.Printf("  [%s] %s: no results, trying fallback...", category, query)
	}

	if photo == nil {
		log.Printf("  [%s] %s: no results from any query", category, queries[0])
		return nil
	}

	// Use webformat (~640px) or large (~1280px) depending on availability.
	imageURL := photo.WebformatURL
	if photo.LargeImageURL != "" {
		imageURL = photo.LargeImageURL
	}
	if err := downloadImage(client, imageURL, outPath); err != nil {
		log.Printf("  [%s] %s: download failed: %v", category, usedQuery, err)
		return nil
	}

	log.Printf("  [%s] %s -> %s (by %s)", category, usedQuery, filename, photo.User)

	// Pixabay allows 100 req/min — light rate limit to be polite.
	time.Sleep(700 * time.Millisecond)

	return &attributionEntry{
		Category:     category,
		Filename:     filename,
		Photographer: photo.User,
		SourceURL:    photo.PageURL,
	}
}

// searchPixabay queries the Pixabay API for photos.
func searchPixabay(client *http.Client, apiKey, query string, perPage, page int) ([]pixabayPhoto, error) {
	u, err := url.Parse("https://pixabay.com/api/")
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("key", apiKey)
	q.Set("q", query)
	q.Set("per_page", fmt.Sprintf("%d", perPage))
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("orientation", "horizontal")
	q.Set("image_type", "photo")
	q.Set("safesearch", "true")
	u.RawQuery = q.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		log.Printf("    Rate limited, waiting 60s...")
		time.Sleep(60 * time.Second)
		// Retry once.
		resp2, err := client.Get(u.String())
		if err != nil {
			return nil, err
		}
		defer resp2.Body.Close()
		resp = resp2
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	var result pixabayResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Hits, nil
}

// downloadImage downloads a single image to disk.
func downloadImage(client *http.Client, imageURL, outPath string) error {
	resp, err := client.Get(imageURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// allGearItems returns all gear catalog items in a stable order.
func allGearItems() []simulation.GearTemplate {
	var all []simulation.GearTemplate
	all = append(all, simulation.ToolsGear...)
	all = append(all, simulation.KitchenGear...)
	all = append(all, simulation.OutdoorGear...)
	all = append(all, simulation.GardenGear...)
	all = append(all, simulation.PartyGear...)
	all = append(all, simulation.ElectronicsGear...)
	return all
}

// writeAttributionFile creates an ATTRIBUTION.md crediting photographers.
func writeAttributionFile(assetsDir string, entries []attributionEntry) {
	outPath := filepath.Join(assetsDir, "ATTRIBUTION.md")

	var sb strings.Builder
	sb.WriteString("# Image Attribution\n\n")
	sb.WriteString("All images sourced from [Pixabay](https://pixabay.com/) under the\n")
	sb.WriteString("[Pixabay Content License](https://pixabay.com/service/license-summary/).\n\n")

	currentCategory := ""
	for _, e := range entries {
		if e.Category != currentCategory {
			currentCategory = e.Category
			fmt.Fprintf(&sb, "## %s\n\n", currentCategory)
		}
		fmt.Fprintf(&sb, "- `%s` (%s) by %s — [Pixabay](%s)\n", e.Filename, e.ItemName, e.Photographer, e.SourceURL)
	}

	if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
		log.Printf("Warning: could not write attribution file: %v", err)
	} else {
		log.Printf("Wrote attribution to %s", outPath)
	}
}
