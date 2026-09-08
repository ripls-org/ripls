package simulation

import (
	"math"
	"math/rand"
	"testing"
)

func TestCityByName(t *testing.T) {
	for _, city := range Cities {
		got, err := CityByName(city.Name)
		if err != nil {
			t.Errorf("CityByName(%q) unexpected error: %v", city.Name, err)
		}
		if got.State != city.State {
			t.Errorf("CityByName(%q).State = %q, want %q", city.Name, got.State, city.State)
		}
	}

	_, err := CityByName("Nonexistent")
	if err == nil {
		t.Error("CityByName(Nonexistent) expected error, got nil")
	}
}

func TestCitiesHaveRequiredFields(t *testing.T) {
	if len(Cities) != 6 {
		t.Fatalf("Expected 6 cities, got %d", len(Cities))
	}

	for _, city := range Cities {
		if city.Name == "" {
			t.Error("City has empty name")
		}
		if city.State == "" {
			t.Errorf("City %q has empty state", city.Name)
		}
		if city.Center.Latitude == 0 || city.Center.Longitude == 0 {
			t.Errorf("City %q has zero coordinates", city.Name)
		}
		if city.ResidentialRadiusKm <= 0 {
			t.Errorf("City %q has non-positive residential radius", city.Name)
		}
		if len(city.ZipCodes) == 0 {
			t.Errorf("City %q has no zip codes", city.Name)
		}
		if len(city.StreetNames) == 0 {
			t.Errorf("City %q has no street names", city.Name)
		}
		if len(city.StreetSuffixes) == 0 {
			t.Errorf("City %q has no street suffixes", city.Name)
		}
	}
}

func TestGenerateResidentialAddress(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	city := Cities[0] // Austin

	addr := GenerateResidentialAddress(rng, city)

	if addr.Street == "" {
		t.Error("Generated address has empty street")
	}
	if addr.City != city.Name {
		t.Errorf("City = %q, want %q", addr.City, city.Name)
	}
	if addr.State != city.State {
		t.Errorf("State = %q, want %q", addr.State, city.State)
	}
	if addr.ZipCode == "" {
		t.Error("Generated address has empty zip code")
	}
	if addr.Coordinates.Latitude == 0 || addr.Coordinates.Longitude == 0 {
		t.Error("Generated address has zero coordinates")
	}

	// Verify coordinates are within reasonable distance of city center.
	dist := haversineKm(city.Center, addr.Coordinates)
	if dist > city.ResidentialRadiusKm*1.1 {
		t.Errorf("Generated address %.1f km from center, max %.1f km", dist, city.ResidentialRadiusKm)
	}
}

func TestGenerateResidentialAddressDeterministic(t *testing.T) {
	city := Cities[0]
	addr1 := GenerateResidentialAddress(rand.New(rand.NewSource(42)), city)
	addr2 := GenerateResidentialAddress(rand.New(rand.NewSource(42)), city)

	if addr1.Street != addr2.Street {
		t.Errorf("Same seed produced different streets: %q vs %q", addr1.Street, addr2.Street)
	}
	if addr1.Coordinates.Latitude != addr2.Coordinates.Latitude {
		t.Error("Same seed produced different coordinates")
	}
}

func TestGenerateResidentialAddressVariety(t *testing.T) {
	rng := rand.New(rand.NewSource(123))
	city := Cities[0]

	streets := make(map[string]bool)
	for range 50 {
		addr := GenerateResidentialAddress(rng, city)
		streets[addr.Street] = true
	}

	// With 20 street names, 10 suffixes, and 9900 house numbers,
	// 50 addresses should be mostly unique.
	if len(streets) < 40 {
		t.Errorf("Only %d unique streets out of 50 generated", len(streets))
	}
}

func TestLoadVenues(t *testing.T) {
	venues, err := LoadVenues()
	if err != nil {
		t.Fatalf("LoadVenues() error: %v", err)
	}

	// Should have data for all 6 cities.
	for _, city := range Cities {
		cityVenues := venues[city.Name]
		if len(cityVenues) == 0 {
			t.Errorf("No venues for %s", city.Name)
			continue
		}

		// Verify venue fields are populated.
		for i, v := range cityVenues {
			if v.Name == "" {
				t.Errorf("%s venue[%d] has empty name", city.Name, i)
			}
			if v.City != city.Name {
				t.Errorf("%s venue[%d] city = %q, want %q", city.Name, i, v.City, city.Name)
			}
			if v.Coordinates.Latitude == 0 && v.Coordinates.Longitude == 0 {
				t.Errorf("%s venue[%d] %q has zero coordinates", city.Name, i, v.Name)
			}
			if v.Category == "" {
				t.Errorf("%s venue[%d] %q has empty category", city.Name, i, v.Name)
			}
		}
	}
}

func TestVenuesForCity(t *testing.T) {
	venues, err := LoadVenues()
	if err != nil {
		t.Fatalf("LoadVenues() error: %v", err)
	}

	austin := VenuesForCity(venues, "Austin")
	if len(austin) == 0 {
		t.Error("VenuesForCity(Austin) returned no venues")
	}

	empty := VenuesForCity(venues, "Nonexistent")
	if len(empty) != 0 {
		t.Errorf("VenuesForCity(Nonexistent) returned %d venues, want 0", len(empty))
	}
}

// haversineKm computes the great-circle distance between two points in kilometers.
func haversineKm(a, b Coordinates) float64 {
	const earthRadiusKm = 6371.0

	lat1 := a.Latitude * math.Pi / 180
	lat2 := b.Latitude * math.Pi / 180
	dLat := (b.Latitude - a.Latitude) * math.Pi / 180
	dLon := (b.Longitude - a.Longitude) * math.Pi / 180

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)

	return 2 * earthRadiusKm * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
}
