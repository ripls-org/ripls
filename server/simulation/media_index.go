package simulation

import "fmt"

// gearAssetIndex maps gear name to its index in AllGear() for asset filename lookup.
var gearAssetIndex map[string]int

// Pure and dependency-free.
//
//nolint:gochecknoinits // builds a name->index map over the static AllGear() asset list.
func init() {
	gearAssetIndex = make(map[string]int)
	for i, g := range AllGear() {
		gearAssetIndex[g.Name] = i
	}
}

// GearAssetFilename returns the expected asset filename for a gear template.
// Returns empty string if no image is expected for this item.
func GearAssetFilename(g GearTemplate) string {
	if idx, ok := gearAssetIndex[g.Name]; ok {
		return fmt.Sprintf("gear_%03d.jpg", idx)
	}
	return ""
}

// ExperienceAssetFilename returns the expected asset filename for an experience
// at the given template index.
func ExperienceAssetFilename(idx int) string {
	return fmt.Sprintf("exp_%03d.jpg", idx)
}

// RequestAssetFilename returns the expected asset filename for a request
// at the given template index.
func RequestAssetFilename(idx int) string {
	return fmt.Sprintf("req_%03d.jpg", idx)
}

// ProfileAssetFilename returns the expected asset filename for a user profile
// at the given index.
func ProfileAssetFilename(idx int) string {
	return fmt.Sprintf("profile_%03d.jpg", idx)
}

// CommunityAssetFilename returns the expected asset filename for a community
// at the given index.
func CommunityAssetFilename(idx int) string {
	return fmt.Sprintf("community_%03d.jpg", idx)
}

// CommunityInfo holds a community's name and description for image lookup.
type CommunityInfo struct {
	Name        string
	Description string
}

// AllUniqueEmails returns all unique user emails across all scenarios in a
// stable iteration order (scenarios → communities → members). This order must
// match between the image fetcher and the simulation runner.
func AllUniqueEmails() []string {
	seen := make(map[string]bool)
	var emails []string
	for _, s := range AllScenarios() {
		for _, c := range s.Communities {
			for _, m := range c.Members {
				if !seen[m.Email] {
					seen[m.Email] = true
					emails = append(emails, m.Email)
				}
			}
		}
	}
	return emails
}

// AllUniqueCommunities returns all unique communities across all scenarios in
// a stable iteration order (scenarios → communities), deduped by name. This
// order must match between the image fetcher and the simulation runner.
func AllUniqueCommunities() []CommunityInfo {
	seen := make(map[string]bool)
	var communities []CommunityInfo
	for _, s := range AllScenarios() {
		for _, c := range s.Communities {
			if !seen[c.Name] {
				seen[c.Name] = true
				communities = append(communities, CommunityInfo{
					Name:        c.Name,
					Description: c.Description,
				})
			}
		}
	}
	return communities
}
