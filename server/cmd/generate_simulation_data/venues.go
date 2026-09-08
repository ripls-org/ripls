package main

// venueCategory defines an OSM tag query and how many results to fetch per city.
type venueCategory struct {
	// OSM tag key (e.g., "amenity", "leisure", "tourism").
	Key string

	// OSM tag value (e.g., "library", "park", "restaurant").
	Value string

	// Category label for the output.
	Category string

	// Limit is the max results to fetch per city.
	Limit int
}

// venueCategories returns the OSM tag queries to run per city.
func venueCategories() []venueCategory {
	return []venueCategory{
		{Key: "leisure", Value: "park", Category: "park", Limit: 15},
		{Key: "amenity", Value: "library", Category: "library", Limit: 10},
		{Key: "amenity", Value: "community_centre", Category: "community_center", Limit: 10},
		{Key: "amenity", Value: "cafe", Category: "cafe", Limit: 10},
		{Key: "amenity", Value: "restaurant", Category: "restaurant", Limit: 10},
		{Key: "amenity", Value: "pub", Category: "brewery", Limit: 5},
		{Key: "leisure", Value: "fitness_centre", Category: "gym", Limit: 5},
		{Key: "amenity", Value: "place_of_worship", Category: "church", Limit: 5},
		{Key: "amenity", Value: "school", Category: "school", Limit: 5},
		{Key: "tourism", Value: "museum", Category: "museum", Limit: 5},
		{Key: "amenity", Value: "theatre", Category: "theater", Limit: 5},
		{Key: "amenity", Value: "marketplace", Category: "market", Limit: 3},
		{Key: "shop", Value: "hardware", Category: "retail", Limit: 3},
	}
}
