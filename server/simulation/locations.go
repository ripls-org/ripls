package simulation

import (
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
)

//go:embed data/venues.json
var venuesFS embed.FS

// City defines a metropolitan area for simulation.
type City struct {
	// Name of the city (e.g., "Austin").
	Name string `json:"name"`

	// State abbreviation (e.g., "TX").
	State string `json:"state"`

	// Center coordinates of the city.
	Center Coordinates `json:"center"`

	// ResidentialRadiusKm is the approximate radius for generating residential addresses.
	ResidentialRadiusKm float64 `json:"residential_radius_km"`

	// ZipCodes are valid zip codes for this city for residential address generation.
	ZipCodes []string `json:"zip_codes"`

	// StreetNames are common street names for residential address generation.
	StreetNames []string `json:"street_names"`

	// StreetSuffixes are common street suffixes (St, Ave, Blvd, etc.).
	StreetSuffixes []string `json:"street_suffixes"`
}

// Coordinates represents a geographic point.
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Venue represents a geocoded place (park, library, restaurant, etc.).
type Venue struct {
	// Name of the venue.
	Name string `json:"name"`

	// FullAddress is the complete street address.
	FullAddress string `json:"full_address"`

	// Category of venue (park, library, community_center, restaurant, etc.).
	Category string `json:"category"`

	// City this venue belongs to.
	City string `json:"city"`

	// Coordinates of the venue.
	Coordinates Coordinates `json:"coordinates"`
}

// ResidentialAddress represents a generated home address.
type ResidentialAddress struct {
	// Street is the full street line (e.g., "2847 Oak Hill Drive").
	Street string

	// City name.
	City string

	// State abbreviation.
	State string

	// ZipCode for the address.
	ZipCode string

	// Coordinates of the address.
	Coordinates Coordinates
}

// Cities defines the 6 metropolitan areas used in simulations.
var Cities = []City{
	{
		Name: "Austin", State: "TX",
		Center:              Coordinates{Latitude: 30.2672, Longitude: -97.7431},
		ResidentialRadiusKm: 15,
		ZipCodes:            []string{"78701", "78702", "78703", "78704", "78705", "78731", "78741", "78745", "78748", "78749", "78757", "78758"},
		StreetNames:         []string{"Bluebonnet", "Live Oak", "Pecan", "Cedar", "Guadalupe", "Lamar", "Burnet", "Congress", "Barton Springs", "South First", "Manor", "East Riverside", "Red River", "Oltorf", "Manchaca", "Slaughter", "Parmer", "Metric", "Duval", "Speedway"},
		StreetSuffixes:      []string{"St", "Ave", "Dr", "Blvd", "Ln", "Ct", "Way", "Pl", "Rd", "Cir"},
	},
	{
		Name: "Boulder", State: "CO",
		Center:              Coordinates{Latitude: 40.0150, Longitude: -105.2705},
		ResidentialRadiusKm: 8,
		ZipCodes:            []string{"80301", "80302", "80303", "80304", "80305", "80309", "80310"},
		StreetNames:         []string{"Pearl", "Broadway", "Spruce", "Pine", "Walnut", "Arapahoe", "Canyon", "Baseline", "Folsom", "Mapleton", "Marine", "Alpine", "Table Mesa", "Moorhead", "Iris", "Valmont", "Linden", "University", "Aurora", "Cascade"},
		StreetSuffixes:      []string{"St", "Ave", "Dr", "Blvd", "Ln", "Ct", "Way", "Pl", "Rd", "Cir"},
	},
	{
		Name: "New York", State: "NY",
		Center:              Coordinates{Latitude: 40.7128, Longitude: -74.0060},
		ResidentialRadiusKm: 12,
		ZipCodes:            []string{"10001", "10002", "10003", "10009", "10010", "10011", "10012", "10013", "10014", "10016", "10019", "10021", "10023", "10025", "10029", "10036", "11201", "11215", "11217", "11238"},
		StreetNames:         []string{"Broadway", "Amsterdam", "Columbus", "Lexington", "Madison", "Park", "Riverside", "West End", "York", "First", "Second", "Third", "Fifth", "Seventh", "Eighth", "Bleecker", "Houston", "Spring", "Prince", "Mott"},
		StreetSuffixes:      []string{"St", "Ave", "Blvd", "Pl"},
	},
	{
		Name: "Los Angeles", State: "CA",
		Center:              Coordinates{Latitude: 34.0522, Longitude: -118.2437},
		ResidentialRadiusKm: 20,
		ZipCodes:            []string{"90001", "90004", "90006", "90012", "90013", "90015", "90019", "90024", "90027", "90028", "90034", "90036", "90039", "90042", "90046", "90048", "90064", "90066", "90068", "90291"},
		StreetNames:         []string{"Sunset", "Hollywood", "Melrose", "Wilshire", "Santa Monica", "Venice", "Beverly", "Fairfax", "La Brea", "Vermont", "Western", "Normandie", "Silver Lake", "Echo Park", "Glendale", "Figueroa", "Temple", "Alvarado", "Fountain", "Franklin"},
		StreetSuffixes:      []string{"St", "Ave", "Dr", "Blvd", "Ln", "Way", "Pl", "Rd"},
	},
	{
		Name: "Minneapolis", State: "MN",
		Center:              Coordinates{Latitude: 44.9778, Longitude: -93.2650},
		ResidentialRadiusKm: 12,
		ZipCodes:            []string{"55401", "55402", "55403", "55404", "55405", "55406", "55407", "55408", "55409", "55410", "55411", "55412", "55413", "55414", "55416", "55417", "55418", "55419"},
		StreetNames:         []string{"Hennepin", "Nicollet", "Lake", "Lyndale", "Portland", "Park", "Chicago", "Bloomington", "Cedar", "Franklin", "Minnehaha", "Hiawatha", "Bryant", "Aldrich", "Dupont", "Emerson", "Fremont", "Girard", "Humboldt", "Irving"},
		StreetSuffixes:      []string{"St", "Ave", "Dr", "Blvd", "Ln", "Ct", "Way", "Pl", "Rd"},
	},
	{
		Name: "Baton Rouge", State: "LA",
		Center:              Coordinates{Latitude: 30.4515, Longitude: -91.1871},
		ResidentialRadiusKm: 12,
		ZipCodes:            []string{"70801", "70802", "70803", "70805", "70806", "70808", "70809", "70810", "70811", "70814", "70815", "70816", "70817", "70818", "70819", "70820"},
		StreetNames:         []string{"Government", "Florida", "Highland", "Perkins", "College", "Nicholson", "Airline", "Plank", "Greenwell Springs", "Jefferson", "Main", "Third", "Convention", "River", "Bluebonnet", "Siegen", "Coursey", "Jones Creek", "Old Hammond", "Sherwood Forest"},
		StreetSuffixes:      []string{"St", "Ave", "Dr", "Blvd", "Ln", "Ct", "Way", "Pl", "Rd"},
	},
}

// LoadVenues loads the cached venue data from the embedded JSON file.
func LoadVenues() (map[string][]Venue, error) {
	data, err := venuesFS.ReadFile("data/venues.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded venues: %w", err)
	}

	var venues map[string][]Venue
	if err := json.Unmarshal(data, &venues); err != nil {
		return nil, fmt.Errorf("unmarshal venues: %w", err)
	}

	return venues, nil
}

// VenuesForCity returns venues for the given city name.
func VenuesForCity(allVenues map[string][]Venue, cityName string) []Venue {
	return allVenues[cityName]
}

// GenerateResidentialAddress generates a realistic residential address for the given city
// using the provided PRNG for deterministic generation.
func GenerateResidentialAddress(rng *rand.Rand, city City) ResidentialAddress {
	// Generate house number (100-9999).
	houseNum := rng.Intn(9900) + 100

	// Pick a random street name and suffix.
	streetName := city.StreetNames[rng.Intn(len(city.StreetNames))]
	suffix := city.StreetSuffixes[rng.Intn(len(city.StreetSuffixes))]

	// Pick a random zip code.
	zipCode := city.ZipCodes[rng.Intn(len(city.ZipCodes))]

	// Generate coordinates within the residential radius of city center.
	coords := randomPointInRadius(rng, city.Center, city.ResidentialRadiusKm)

	return ResidentialAddress{
		Street:      fmt.Sprintf("%d %s %s", houseNum, streetName, suffix),
		City:        city.Name,
		State:       city.State,
		ZipCode:     zipCode,
		Coordinates: coords,
	}
}

// randomPointInRadius generates a random point within a radius (in km) of a center point.
// Uses uniform distribution within a circle.
func randomPointInRadius(rng *rand.Rand, center Coordinates, radiusKm float64) Coordinates {
	// Convert radius to degrees (approximate).
	// 1 degree latitude ≈ 111 km.
	// 1 degree longitude ≈ 111 * cos(latitude) km.
	latRadiusDeg := radiusKm / 111.0
	lonRadiusDeg := radiusKm / (111.0 * math.Cos(center.Latitude*math.Pi/180.0))

	// Uniform distribution within circle using rejection sampling equivalent.
	// Use sqrt for uniform area distribution.
	r := math.Sqrt(rng.Float64())
	theta := rng.Float64() * 2 * math.Pi

	lat := center.Latitude + r*latRadiusDeg*math.Sin(theta)
	lon := center.Longitude + r*lonRadiusDeg*math.Cos(theta)

	return Coordinates{
		Latitude:  lat,
		Longitude: lon,
	}
}

// CityByName returns the City definition for a given name, or an error if not found.
func CityByName(name string) (City, error) {
	for _, c := range Cities {
		if c.Name == name {
			return c, nil
		}
	}
	return City{}, fmt.Errorf("unknown city: %q", name)
}
