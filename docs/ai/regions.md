# Regional Community Leaderboard Filtering

**Status:** Planning
**Last Updated:** 2025-12-27
**Author:** Claude Code
**Scope:** US-only deployment

---

## Executive Summary

This document outlines the design and implementation plan for regional filtering on the community leaderboard (global_screen). Users will be able to filter communities by geographic region (neighborhood, city, county, state) to discover top communities in their area for specific skills.

### Key Decisions

1. **Member-Derived Regions:** Community regions are automatically computed from aggregated member locations using "most common" value per administrative level
2. **Majority Rule:** Communities appear in regions where ≥50% of members share the same value at each level (neighborhood, city, county, state)
3. **Hierarchical Fallback:** If no common neighborhood exists, community only appears in city-level searches. If no common city, only county/state
4. **Always Editable:** Region remains editable in community settings at all times, allowing permanent manual override with Mapbox search
5. **No Additional API Costs:** Use existing Mapbox Search Box API context data instead of separate Tilequery calls
6. **Scoped Naming:** Store "Boulder, CO" format with Mapbox place IDs for disambiguation
7. **Privacy-First:** City-level granularity only, no individual address exposure
8. **US-Only:** Initial deployment covers US administrative divisions only

### Architecture Compliance

**Client-Side:**
- **Repository Pattern:** New CommunityRepository with Stash-based caching (cache namespace: `'community'`)
- **ViewModel Data Access:** GlobalViewModel calls repository, never services directly
- **Async-First:** All data access async with FutureBuilder/AsyncValue for loading states
- **Cache Invalidation:** Pull-to-refresh clears `community:*` pattern, mutations invalidate specific keys
- **Freezed State:** GlobalState updated to track selected region (immutable with `copyWith()`)

**Server-Side:**
- **Shared Library:** Region computation logic in `server/community/regions.go` (not in service layer)
- **Service Independence:** CommunityService doesn't depend on other services, only storage/auth
- **File Organization:** New region-specific files (`regions.go`, `regions_test.go`) keep service.go focused
- **Unit Testing:** Comprehensive tests for region computation logic with table-driven tests
- **Proto Separation:** API protos (request/response) separate from models protos (storage)

---

## Table of Contents

1. [Problem Statement](#problem-statement)
2. [Current State](#current-state)
3. [Requirements](#requirements)
4. [Challenges](#challenges)
5. [Potential Solutions](#potential-solutions)
6. [Recommended Approach](#recommended-approach)
7. [Open Questions](#open-questions)
8. [Phased Implementation Plan](#phased-implementation-plan)
9. [Data Model Changes](#data-model-changes)
10. [Alternative Approaches Considered](#alternative-approaches-considered)

---

## Problem Statement

Users want to filter the global community leaderboard by geographic region to see top communities in their area for specific skills. Regions should support multiple administrative levels (neighborhood, city, county, state) to enable both broad and hyperlocal discovery.

**Scope:** Initial implementation is US-only. All communities are within the United States, and all region filtering uses US administrative divisions.

**Use Cases:**
- "Show me the top gardening communities in Boulder" (city)
- "What are the best cycling communities in Colorado?" (state)
- "Which communities are strongest in woodworking in my neighborhood?" (neighborhood)
- "Top reading communities nationwide" (no filter, all US communities)

**Current Gap:** Communities have no geographic association, making regional filtering impossible. The global leaderboard shows all communities ranked by skill quality score without location context.

---

## Current State

### Existing Data Model

**Community Model** ([community.proto](../../proto/ripls/models/community.proto)):
- `id`, `name`, `description`, `creator_id`
- `media_ids`, timestamps
- **No location fields**

**CommunitySkill Model** ([skill.proto](../../proto/ripls/models/skill.proto)):
- Links communities to skills with aggregate quality scores
- `quality_score`: Sum of equity-weighted skill scores across all members
- `user_count`: Number of distinct users demonstrating the skill
- Used for leaderboard ranking

**Location Model** ([location.proto](../../proto/ripls/models/location.proto)):
- `Geolocation`: lat/lon coordinates
- `Address`: region_code (CLDR country), postal_code, locality (city), address_lines
- **Limitation:** Locality field is a simple string, not structured administrative levels

**User Model**:
- `primary_residence_location_id`: User's home location
- `other_location_ids`: Additional saved places
- Users have locations, but communities do not

### Global Screen Implementation

**File:** [global_screen.dart](../../app/lib/presentation/screens/global/global_screen.dart)

**Current Features:**
- Loads all communities via `ListCommunities` RPC
- Displays leaderboard sorted by skill quality score (fake equity score for now)
- Filter UI exists but not functional (Phase 1 dummy data)
- Location search typeahead active but no backend integration
- Category filter active but uses hardcoded dummy categories

**ViewModel:** [global_view_model.dart](../../app/lib/presentation/viewmodels/global_view_model.dart)
- `selectedLocationName`: String display name (e.g., "Boulder")
- `selectedCategory`: String skill name (e.g., "Gardening")
- `isSearchActive`: Boolean for location search mode
- **No actual filtering logic** - displays all communities regardless of selection

---

## Requirements

### Functional Requirements

1. **Multi-Level Geographic Filtering**
   - Support neighborhood, city, county, state/province, country levels
   - User selects region from location search typeahead
   - Leaderboard displays only communities in that region
   - Filter applies independently per skill category

2. **Location Search & Geocoding**
   - Leverage existing Mapbox Search Box API integration
   - Search returns structured location results with administrative hierarchy
   - Client sends selected region to server for filtering

3. **Community-Region Association**
   - Communities must be associated with one or more regions
   - Association method must be automatic (no manual admin data entry)
   - Should reflect actual member distribution

4. **Leaderboard Ranking**
   - Within a region, rank communities by skill quality score
   - Preserve existing equity-weighted skill scoring system
   - Support filtering by both region AND skill simultaneously

5. **Performance**
   - Sub-second query response for filtered leaderboard
   - Efficient spatial indexing for large-scale deployment
   - Client-side caching of region metadata

### Non-Functional Requirements

1. **Data Quality:** Region associations should be accurate and representative
2. **Scalability:** Support thousands of communities across US regions
3. **Maintainability:** Minimize manual data management overhead
4. **Privacy:** No exposure of individual member addresses (city-level granularity only)
5. **Consistency:** Region boundaries align with US administrative divisions
6. **Geographic Scope:** US-only for initial deployment

---

## Challenges

### 1. Community Location Ambiguity

**Problem:** Communities are not inherently tied to physical locations. They are social groups that can span geographies.

**Examples:**
- "Boulder Outdoors" - members live in Boulder, CO
- "Colorado Trail Runners" - members scattered across Colorado
- "Global Book Club" - members worldwide, virtual community
- "Capitol Hill Gardeners" - neighborhood-specific in Denver

**Implication:** No single "community location" exists. Must aggregate member locations.

### 2. Administrative Level Granularity

**Problem:** Location model's `Address.locality` is a freeform string (e.g., "Boulder"), not a structured hierarchy.

**Missing Data:**
- Neighborhood names not captured
- County boundaries not defined
- State/province separate from locality
- No canonical region IDs for matching

**Current Workaround:** Mapbox Search Box API returns structured location data with administrative levels, but we don't persist this structure.

### 3. Member Location Accuracy

**Problem:** Not all community members have locations set.

**User Location Coverage:**
- `primary_residence_location_id`: Required during onboarding (high coverage)
- `other_location_ids`: Optional saved places (low coverage)
- Some users may set locations far from actual residence (privacy)

**Implication:** Community-region mapping may be incomplete or inaccurate if based solely on member locations.

### 4. Region Boundary Representation

**Problem:** Administrative boundaries are complex polygons, not points.

**Current System:** Locations stored as point coordinates (lat/lon), not polygons.

**Options:**
1. Point-in-polygon queries (complex, requires PostGIS or external service)
2. Bounding box approximations (inaccurate for irregular shapes)
3. Lookup tables mapping coordinates to regions (maintenance burden)
4. External geocoding service (Mapbox Tilequery API) - network latency

### 5. Multi-Region Communities

**Problem:** Community members may live in different regions (e.g., "Denver Metro Hikers" spans multiple suburbs).

**Solutions:**
- **Primary Region:** Assign community to most-represented region
- **Multi-Region:** Allow communities to appear in multiple region leaderboards
- **Weighted Presence:** Rank communities by % of members in region

**Tradeoff:** Simplicity vs. accuracy

### 6. Data Freshness

**Problem:** Member locations change over time (users move, join/leave communities).

**Options:**
- Real-time recalculation on every query (slow)
- Materialized view updated periodically (stale data)
- Event-driven updates on member changes (complex)

---

## Potential Solutions

### Solution A: Member-Derived Community Regions (Recommended)

**Concept:** Derive community region(s) from aggregated member primary residence locations.

**How It Works:**
1. On community creation or member change, aggregate all member `primary_residence_location_id` locations
2. Extract administrative hierarchy from each location's enhanced `Address` (neighborhood, locality, county, administrative_area)
3. For each administrative level, find the most common value:
   - **Neighborhood:** Most common `Address.neighborhood` (if ≥50% of members share it)
   - **City:** Most common `Address.locality` (if ≥50% of members share it)
   - **County:** Most common `Address.county` (if ≥50% of members share it)
   - **State:** Most common `Address.administrative_area` (if ≥50% of members share it)
4. Create `CommunityRegion` entries only for levels where ≥50% threshold is met
5. If no common neighborhood, community only appears in city-level searches
6. If no common city, only county or state-level searches
7. Store associations in `CommunityRegion` table
8. Leaderboard queries filter by matching `region_type` and `region_name`

**Example:**
- Community with 10 members: 6 in "Boulder, CO", 3 in "Denver, CO", 1 in "Fort Collins, CO"
- Result: No common city (Boulder only 60%, not majority). But all in Colorado → state-level only
- CommunityRegion: `{type: "state", name: "Colorado", member_percentage: 1.0}`

**Storage:**
```proto
// New message in ripls/models/community.proto
message CommunityRegion {
  string id = 1;
  string community_id = 2;
  string region_type = 3;  // "neighborhood", "city", "county", "state", "country"
  string region_name = 4;  // "Boulder", "Boulder County", "Colorado", "US"
  string region_code = 5;  // Optional: structured code (e.g., "US-CO" for Colorado)
  double member_percentage = 6;  // % of community members in this region (0.0 - 1.0)
  int64 created_at_unix_sec = 7;
  int64 updated_at_unix_sec = 8;
}
```

**Pros:**
- ✅ Automatic - no manual region assignment
- ✅ Reflects actual member distribution
- ✅ Updates dynamically as membership changes
- ✅ Supports multi-region communities (store multiple rows)
- ✅ Uses existing location data infrastructure

**Cons:**
- ⚠️ Requires enhanced Address model (county, state fields)
- ⚠️ Depends on member location accuracy
- ⚠️ Computation overhead on member changes
- ⚠️ Communities with no members have no regions (edge case)

---

### Solution B: Creator-Specified Community Location

**Concept:** Community creator manually sets community's primary region during creation.

**How It Works:**
1. Add `location_id` field to Community model
2. During community creation, prompt creator to select location via Mapbox search
3. Persist location reference in Community table
4. Leaderboard queries join Community → Location → extract region

**Storage:**
```proto
// Modified ripls/models/community.proto
message Community {
  string id = 1;
  string name = 2;
  string description = 3;
  string creator_id = 4;
  repeated string media_ids = 5;
  int64 created_at_unix_sec = 6;
  int64 updated_at_unix_sec = 7;
  string location_id = 8;  // NEW: Primary location for this community
}
```

**Pros:**
- ✅ Simple to implement
- ✅ Explicit intent from creator
- ✅ Single source of truth

**Cons:**
- ❌ Manual data entry (user friction during creation)
- ❌ Static - doesn't reflect membership changes
- ❌ Creator may misrepresent scope (e.g., sets "Boulder" but members are in Denver)
- ❌ Requires UI changes to community creation flow

---

### Solution C: Geo-Fencing with PostGIS

**Concept:** Store administrative boundary polygons in PostGIS, perform point-in-polygon queries.

**How It Works:**
1. Import administrative boundary shapefiles (e.g., US Census TIGER/Line, OpenStreetMap)
2. Store polygons in PostGIS-enabled PostgreSQL
3. For each community, aggregate member locations
4. Query PostGIS to determine which regions contain member cluster
5. Leaderboard queries use spatial joins

**Pros:**
- ✅ Highly accurate boundary matching
- ✅ Supports complex irregular regions (neighborhoods, counties)
- ✅ Handles overlapping boundaries (e.g., city within county)

**Cons:**
- ❌ Significant complexity (PostGIS setup, shapefile imports)
- ❌ Performance overhead for spatial queries at scale
- ❌ Boundary data maintenance (updates, corrections)
- ❌ Global coverage requires multiple data sources
- ❌ Overkill for current use case

---

### Solution D: Mapbox Tilequery API (External Service)

**Concept:** Use Mapbox Tilequery API to resolve coordinates to administrative regions on-the-fly.

**How It Works:**
1. For each community, compute centroid of member locations
2. Send centroid to Mapbox Tilequery API
3. API returns administrative hierarchy (neighborhood, city, county, state, country)
4. Cache region data in `CommunityRegion` table
5. Periodically refresh (weekly) to handle boundary changes

**Pros:**
- ✅ No boundary data storage required
- ✅ Always up-to-date with Mapbox data
- ✅ Global coverage out-of-the-box
- ✅ Handles administrative level queries (neighborhood → country)

**Cons:**
- ⚠️ External API dependency (network latency, costs)
- ⚠️ Rate limits (need caching strategy)
- ⚠️ Requires Mapbox API token
- ⚠️ Service availability risk

---

## Recommended Approach

**Solution A: Member-Derived Regions with Existing API Integration**

### Architecture

1. **Enhanced Address Model:**
   - Add structured fields: `neighborhood`, `county`, `administrative_area` (state/province)
   - Populate during location save by parsing Mapbox Search Box API `context` array
   - No additional API calls needed - data already available in search results
   - Store full administrative hierarchy for later filtering

2. **CommunityRegion Table:**
   - Automatically computed from member locations using "most common" algorithm
   - For each admin level, create entry if ≥50% of members share same value
   - Multiple entries per community possible (e.g., has city + state, but no neighborhood)
   - Track `member_percentage` for actual percentage (0.5-1.0)
   - Track `is_override` flag for manual vs. auto-computed regions
   - Refresh via event-driven updates (member join/leave)

3. **Region Computation Algorithm:**
   - Aggregate all member primary residence locations
   - For each level (neighborhood, city, county, state), count occurrences
   - If most common value ≥50%, create CommunityRegion entry
   - Example: 8/10 members in Boulder → city entry. All 10 in Colorado → state entry
   - No triangulation or complex geometry - simple count-based thresholds

4. **Leaderboard Query:**
   - Client sends `region_name` + `region_type` (e.g., "Boulder", "city")
   - Server queries `CommunityRegion` table for exact matches
   - Joins with `CommunitySkill` for skill-filtered ranking
   - Returns ranked communities with quality scores

### Why This Approach

- **Automatic:** No manual region assignment required (with override option)
- **Accurate:** Based on actual member distribution with simple majority logic
- **Scalable:** Materialized region data enables fast queries
- **Flexible:** Hierarchical fallback (neighborhood → city → county → state)
- **Maintainable:** Event-driven updates keep data fresh
- **Cost-Effective:** Zero additional API calls (reuses existing Mapbox search data)
- **Simple:** No complex geometry or triangulation - just counting and thresholds

### Tradeoffs Accepted

- **Majority-Only Representation:** Communities with distributed membership (no ≥50% region) appear only at higher admin levels (acceptable - reflects actual geographic spread)
- **Data Duplication:** Administrative data stored in both Location and CommunityRegion (acceptable for query performance)
- **Eventual Consistency:** Region updates lag member changes by ~1 minute (acceptable for leaderboard use case)
- **Override Persistence:** Manual overrides stay in effect even after membership changes (acceptable - user choice to maintain control)

---

## Open Questions

### 1. Regional Representation Threshold

**Question:** What % of members must be in a region for the community to appear in that region's leaderboard?

**Decision:** Start with majority rule (≥50% of members) for single primary region. Expand to multi-region in Phase 2 if users request it.

---

### 2. New Community Cold Start

**Question:** How do we handle new communities with few members where region detection from member distribution is not yet meaningful?

**Decision:** Use creator's primary residence as fallback for initial region assignment. The region remains editable in community settings at all times, allowing the creator (or admin) to manually override even after automatic computation begins. Users can choose to let automatic computation manage regions or maintain manual control.

**Rationale:** Every community needs a region from day one for discoverability. The creator's location is a reasonable proxy since they typically start communities in their own area. Keeping it editable indefinitely provides flexibility for communities where automatic detection doesn't match the community's actual scope (e.g., a distributed community focused on a specific region, virtual communities that want a geographic anchor, etc.).

---

### 3. Location Privacy

**Question:** Should users be able to opt out of contributing their location to community region calculations?

**Decision:** Primary residence always contributes to community region calculations (no opt-out). This is consistent with current behavior where primary residence is used for proximity queries and gear discovery.

**Rationale:** We only use city-level granularity for region associations (not precise coordinates), which is already public information. The community region calculation exposes no new private data beyond what's already shown in the app.

---

### 4. Administrative Data Source

**Question:** Which data source should we use for administrative boundaries?

**Decision:** Mapbox Tilequery API for administrative hierarchy enrichment.

**Rationale:** We already use Mapbox for search/maps, data quality is high, and cost is acceptable (~$50/month for 100k locations). However, since this is a US-only application initially, we can optimize for US-specific data quality and potentially use TIGER/Line as a future enhancement for offline enrichment.

**Note:** Administrative boundaries themselves (polygons) are NOT needed for this implementation. We only need the administrative hierarchy metadata (neighborhood, city, county, state names). Since we already retrieve this data during location lookups (SaveLocation via Mapbox Search Box API), we can enhance the Address model to store these fields instead of making separate Tilequery calls. This avoids additional API costs and leverages existing geocoding infrastructure.

---

### 5. Region Name Conflicts

**Question:** How do we handle cities with duplicate names (e.g., "Springfield" exists in 30+ US states)?

**Decision:** Store scoped names ("Boulder, CO") in `region_name` field. Use `region_code` for structured matching with Mapbox place IDs (e.g., "place.1234567"). Display scoped name in UI.

**Rationale:** Scoped names are human-readable and unambiguous. The Mapbox place ID in `region_code` provides a canonical identifier for exact matching, preventing false positives from similarly-named places. Users see "Boulder, CO" while the system uses the place ID for precision.

---

### 6. Neighborhood Granularity

**Question:** Neighborhoods are not standardized (e.g., "Capitol Hill" in Denver has fuzzy boundaries). How do we handle this?

**Decision:** Use Mapbox neighborhoods where available, fall back to city-level filtering when neighborhood data is unavailable. Do NOT use postal codes.

**Rationale:** Postal codes (ZIP codes) are too granular and not intuitive for users searching by region. Mapbox provides neighborhood data for major cities, which is sufficient for the hyperlocal use case. For areas without defined neighborhoods, city-level filtering is an acceptable alternative.

---

### 7. Virtual Communities

**Question:** Should fully virtual/global communities (e.g., "Global Book Club") appear in regional leaderboards?

**Decision:** Not applicable for initial US-only deployment. All communities must have a US-based region.

**Rationale:** Since this is a US-only application initially, there are no truly "global" communities spanning multiple countries. All communities will have US-based regions (city, state, or nationwide). If/when the app expands internationally, we can revisit this question and potentially add a "Global" region type for cross-border communities.

**Future Consideration:** For international expansion, communities with no dominant region (no region ≥50% of members) could appear under a special "Global" category or "Distributed" region type.

---

## Phased Implementation Plan

### Phase 1: Enhanced Location Model (2 weeks)

**Goal:** Capture administrative hierarchy in location data.

**Tasks:**
1. **Update Location Proto:**
   - Add `neighborhood`, `county`, `administrative_area` fields to `Address` message
   - Update API proto (`location_service.proto`) with new fields
   - Run `npm run generate` to regenerate code

2. **Enhance Mapbox Search Box API Integration:**
   - Modify existing `SaveLocation` RPC to extract full administrative context
   - Mapbox Search Box API already returns `context` array with administrative hierarchy
   - Parse context to extract: `neighborhood`, `place` (city), `district` (county), `region` (state)
   - No additional API calls needed - data already available in search results

3. **Update SaveLocation RPC:**
   - Parse Mapbox `context` array from existing search/geocoding response
   - Populate enhanced Address fields (neighborhood, county, administrative_area)
   - Store enriched data in Location record
   - Return enriched location to client

4. **Client Updates:**
   - Update Dart location models (regenerated from proto)
   - No UI changes required (fields optional, backward compatible)

4. **Backfill Existing Locations:**
   - Write migration script to enrich existing Location records
   - For locations with only lat/lon, call Mapbox Geocoding API (reverse geocode) to get context
   - Parse context array and populate administrative fields
   - Rate limit to 10 req/sec (Mapbox limits)
   - Note: This is a one-time cost for existing data; new locations get enriched automatically

**Deliverables:**
- Enhanced Location model with administrative hierarchy
- All locations enriched with neighborhood, county, state data
- No additional Mapbox API costs (uses existing Search Box / Geocoding API responses)

**Testing:**
- Unit tests for context array parsing logic
- Integration tests for SaveLocation with enrichment
- Verify enriched data in Location records
- Test backfill migration script on sample data

---

### Phase 2: CommunityRegion Computation (3 weeks)

**Goal:** Automatically compute and store community-region associations.

**Tasks:**
1. **Create CommunityRegion Proto:**
   - Add `CommunityRegion` message to `ripls/models/community.proto`
   - Add storage layer in `server/storage/community.go`
   - Add unit tests for storage operations (`server/storage/community_test.go`)
   - Generate code with `npm run generate`

2. **Create Region Computation Library:**
   - New file: `server/community/regions.go` (shared library, not service)
   - Function: `ComputeRegions(storage *storage.ProtoSQLStorage, communityID string) ([]CommunityRegion, error)`
   - Logic:
     - Query all members' primary residence locations
     - For each administrative level (neighborhood, city, county, state):
       - Count occurrences of each value
       - If ≥50% of members share the same value, create CommunityRegion entry
       - Set `member_percentage` to actual percentage (e.g., 0.6 for 60%)
     - Return all qualifying regions
   - Unit tests: `server/community/regions_test.go` with table-driven tests
   - Example: Community might have city + state entries, but no neighborhood entry

3. **Event-Driven Updates:**
   - Trigger region recomputation on:
     - New member joins community
     - Member leaves community
     - Member updates primary residence
   - Use async job queue (prevent blocking RPC)
   - Debounce: batch updates every 5 minutes

4. **Backfill Existing Communities:**
   - Run `ComputeCommunityRegions` for all existing communities
   - Log communities with insufficient data (no members, no locations)

5. **Add Region Query RPC:**
   - New RPC: `ListCommunityRegions(communityID)`
   - Returns list of regions with member percentages
   - Used for admin visibility and debugging

5. **Add RPCs to CommunityService:**
   - In `server/services/community/regions.go` (new file):
     - `ListCommunityRegions(communityID)` - Returns current regions
     - `SetCommunityRegionOverride(communityID, regionType, regionName, regionCode)` - Manual override
   - Unit tests: `server/services/community/regions_test.go`
   - Integration tests: Add to `server/main_integration_test.go`

6. **Community Settings Integration (Client):**
   - Add "Region" field to community settings screen
   - Call `ListCommunityRegions` RPC via CommunityRepository to display current regions
   - Show all region levels (neighborhood, city, county, state) if present
   - Add "Edit Region" button that opens Mapbox location search
   - When user selects location from Mapbox search:
     - Extract full administrative context from search result
     - Call `SetCommunityRegionOverride` RPC
     - Invalidate `community:*` cache to force refresh
   - Display override badge to indicate manual vs. auto-computed regions

**Deliverables:**
- `CommunityRegion` table populated for all communities
- Event-driven updates maintain data freshness
- RPC to query community regions
- Manual region override capability for new communities

**Testing:**
- Unit tests for region aggregation logic
- Integration tests for event-driven updates
- Verify region percentages sum to 100%
- Test manual override for new communities

---

### Phase 3: Regional Leaderboard Filtering (2 weeks)

**Goal:** Enable region-filtered leaderboard queries in global screen.

**Tasks:**

1. **Create CommunityRepository (Client):**
   - New repository at `app/lib/data/repositories/community_repository.dart`
   - Wraps CommunityService with Stash caching
   - Cache namespace: `'community'`
   - Cache keys:
     - `'community:$communityId'` for individual communities
     - `'community:list'` for all communities list
     - `'community:region:$regionType:$regionName'` for filtered lists
   - Methods:
     - `Future<CommunityItem> get(String communityId)`
     - `Future<List<CommunityItem>> listAll()`
     - `Future<List<CommunityItem>> listByRegion(String regionType, String regionName)`
     - `Future<void> invalidateRegionLists()` (clear all region-filtered caches)

2. **Update ListCommunities RPC:**
   - Add optional `region_filter` to `ListCommunitiesRequest`:
     ```proto
     message RegionFilter {
       string region_type = 1;  // "city", "state", "country"
       string region_name = 2;  // "Boulder, CO", "Colorado"
     }
     ```
   - When filter provided, join with `CommunityRegion` table
   - Return only communities in specified region
   - Maintain skill-based ranking (quality_score DESC)

2. **Server-Side Query Logic:**
   - Implement efficient SQL join: `Community → CommunityRegion → CommunitySkill`
   - Add index on `CommunityRegion(region_type, region_name)`
   - Filter by `member_percentage ≥ 0.5` (majority rule)

3. **Update GlobalViewModel:**
   - Replace direct CommunityService calls with CommunityRepository calls
   - Methods:
     - `loadCommunities()` → calls `communityRepository.listAll()`
     - `loadCommunitiesByRegion(regionType, regionName)` → calls `communityRepository.listByRegion()`
   - Store selected region in GlobalState:
     ```dart
     @freezed
     class GlobalState with _$GlobalState {
       const factory GlobalState({
         @Default([]) List<CommunityItem> communities,
         @Default(true) bool isLoading,
         String? errorMessage,
         @Default('Gardening') String selectedCategory,
         String? selectedRegionType,  // NEW: "city", "state", null for all
         String? selectedRegionName,   // NEW: "Boulder, CO", "Colorado"
         @Default(false) bool isSearchActive,
       }) = _GlobalState;
     }
     ```
   - Automatic caching: Repository handles cache-or-fetch pattern
   - Pull-to-refresh: Clear `community:*` cache pattern

4. **Update GlobalScreen UI:**
   - Wire up location search to call `loadCommunitiesByRegion()`
   - Display region name in LeaderboardHeader
   - "Clear Filter" button to reset to all communities

**Deliverables:**
- CommunityRepository with Stash-based caching
- Regional filtering in GlobalViewModel
- Fast queries (<500ms) with proper indexing
- UI displays selected region

**Testing:**
- **Client:**
  - Unit tests for CommunityRepository cache behavior
  - ViewModel tests with mocked repository
  - Widget tests for region filter UI
- **Server:**
  - Unit tests for region filter SQL query logic
  - Integration tests for ListCommunities with region_filter
  - Performance tests with 10,000 communities

---

### Phase 4: Location Search Enhancements (1 week)

**Goal:** Improve location search UX in global screen with community-based region suggestions.

**Tasks:**
1. **Enhance Location Search:**
   - **Do NOT use Mapbox Search Box API** for region search
   - Instead, fetch available regions directly from communities the user is a member of
   - Create new RPC: `ListAvailableRegions()` that returns distinct regions from `CommunityRegion` table
   - Update `LeaderboardHeader` typeahead to show only regions with actual communities
   - Display hierarchical labels: "Boulder, Colorado" with region type badges
   - Filter typeahead results client-side based on user input

2. **Dynamic Skills Filter:**
   - Skills filter chips should show top skills for the selected region
   - When region changes, fetch top skills for that region from community-skill associations
   - Create new RPC: `ListTopSkillsForRegion(regionType, regionName)` that:
     - Queries communities in the specified region
     - Aggregates their CommunitySkill records
     - Returns top N skills by total quality score
   - Update GlobalViewModel to track and update skills dynamically
   - Skills chips update automatically when region selection changes

3. **Region Scope Icons:**
   - Visual indicators in typeahead: 🏘️ (neighborhood), 🏙️ (city), 📍 (county), 🗺️ (state)
   - Badge region type next to each search result

4. **All Regions Option:**
   - Add "All Regions" option to clear regional filter
   - Show all communities when no region filter applied
   - Display nationwide top skills when no region selected

**Deliverables:**
- Community-based region search (no external API)
- Dynamic skills filter that updates per region
- Region type visual indicators
- Efficient region and skill queries

**Testing:**
- Unit tests for ListAvailableRegions RPC
- Unit tests for ListTopSkillsForRegion RPC
- Integration tests for dynamic skills updates
- Verify region list matches actual community coverage
- Test skills update when region changes

---

### Phase 4.1: Skill-Based Filtering & Quality Scores (1 week)

**Goal:** Enable skill-based filtering in the global leaderboard and display accurate quality scores for each community.

**Problem Statement:**
Currently, the global leaderboard shows fake equity scores and cannot filter communities by skill. Users should be able to:
1. Select a skill from the dynamic skills list
2. See only communities that have that skill
3. View each community's quality score for the selected skill
4. Rank communities by their quality score for that skill

**Current Limitations:**
- `ListCommunities` RPC returns `CommunityItem` which doesn't include skill quality scores
- No skill-based filtering capability in the communities list
- Frontend displays fake equity scores generated client-side
- Inefficient to fetch skills for each community individually (N+1 queries)

**Proposed Solution:**

#### 1. Backend Changes

**Update `ListCommunitiesRequest` in `proto/ripls/api/community_service.proto`:**
```proto
message ListCommunitiesRequest {
  // Optional: Filter communities by region.
  RegionFilter region_filter = 1;

  // Optional: Filter communities by skill and include quality scores.
  // When set, only returns communities that have this skill.
  string skill_id = 2;
}
```

**Update `CommunityItem` to include skill quality score:**
```proto
message CommunityItem {
  string id = 1;
  string name = 2;
  string description = 3;
  repeated string media_ids = 4;

  // Optional: Quality score for the requested skill (when skill_id filter is used).
  // Range: 0.0 - 100.0
  // Only populated when skill_id is specified in the request.
  double skill_quality_score = 5;
}
```

**Update `ListCommunities` RPC handler in `server/services/community/membership.go`:**
```go
func (s *Service) ListCommunities(
	ctx context.Context,
	req *connect.Request[api.ListCommunitiesRequest],
) (*connect.Response[api.ListCommunitiesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	// Add filters to logs
	if req.Msg.RegionFilter != nil {
		logger = logger.With(
			"region_type", req.Msg.RegionFilter.RegionType,
			"region_name", req.Msg.RegionFilter.RegionName,
		)
	}
	if req.Msg.SkillId != "" {
		logger = logger.With("skill_id", req.Msg.SkillId)
	}

	logger.DebugContext(ctx, "listing communities")

	// Get communities where user is a member
	memberships, err := s.storage.QueryByField(ctx, "user_id", authInfo.UserID, &models.CommunityUser{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query user memberships", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Build skill scores map if skill filter is specified
	skillScores := make(map[string]float64)
	if req.Msg.SkillId != "" {
		// Query all CommunitySkill records for this skill
		skillRecords, err := s.storage.QueryByField(ctx, "skill_id", req.Msg.SkillId, &models.CommunitySkill{})
		if err != nil {
			logger.ErrorContext(ctx, "failed to query community skills", "error", err)
			return nil, connect.NewError(connect.CodeInternal, err)
		}

		// Build map of community_id -> quality_score
		for _, msg := range skillRecords {
			cs := msg.(*models.CommunitySkill)
			skillScores[cs.CommunityId] = cs.QualityScore
		}
	}

	// Get each community and apply filters
	communities := []proto.Message{}
	for _, msg := range memberships {
		membership := msg.(*models.CommunityUser)
		community := &models.Community{}
		err = s.storage.GetByID(ctx, membership.CommunityId, community)
		if err != nil {
			logger.ErrorContext(ctx, "failed to get community", "community_id", membership.CommunityId, "error", err)
			continue
		}

		// If skill filter is specified, skip communities without this skill
		if req.Msg.SkillId != "" {
			if _, hasSkill := skillScores[community.Id]; !hasSkill {
				continue
			}
		}

		// If region filter is specified, check if community matches
		if req.Msg.RegionFilter != nil {
			matchesRegion, err := s.communityMatchesRegion(ctx, community.Id, req.Msg.RegionFilter)
			if err != nil {
				logger.ErrorContext(ctx, "failed to check region filter", "community_id", community.Id, "error", err)
				continue
			}
			if !matchesRegion {
				continue
			}
		}

		communities = append(communities, community)
	}

	// Convert to API response
	items := make([]*api.CommunityItem, 0, len(communities))
	for _, msg := range communities {
		community := msg.(*models.Community)
		item := &api.CommunityItem{
			Id:          community.Id,
			Name:        community.Name,
			Description: community.Description,
			MediaIds:    community.MediaIds,
		}

		// Add skill quality score if skill filter was used
		if req.Msg.SkillId != "" {
			if score, ok := skillScores[community.Id]; ok {
				item.SkillQualityScore = score
			}
		}

		items = append(items, item)
	}

	logger.InfoContext(ctx, "listed communities", "count", len(items))
	return connect.NewResponse(&api.ListCommunitiesResponse{
		Communities: items,
	}), nil
}
```

#### 2. Frontend Changes

**Update `CommunityService` in `app/lib/services/community_service.dart`:**
```dart
Future<List<CommunityItem>> listCommunities({
  RegionFilter? regionFilter,
  String? skillId,
}) async {
  return RpcUtils.executeRpc(
    () async {
      final request = ListCommunitiesRequest(
        regionFilter: regionFilter,
        skillId: skillId ?? '',
      );
      final response = await _client.listCommunities(
        request,
        headers: _buildHeaders(),
      );
      return response.communities;
    },
    _errorHandler,
    onUnauthenticated: _onUnauthenticated,
  );
}
```

**Update `CommunityRepository` in `app/lib/data/repositories/community_repository.dart`:**
```dart
Future<List<CommunityItem>> listUserCommunities({
  RegionFilter? regionFilter,
  String? skillId,
}) async {
  // Build cache key with filters
  String cacheKey = 'user:list';
  if (regionFilter != null) {
    cacheKey = '$cacheKey:${regionFilter.regionType}:${regionFilter.regionName}';
  }
  if (skillId != null && skillId.isNotEmpty) {
    cacheKey = '$cacheKey:skill:$skillId';
  }

  return _cache.getList(
    listKey: cacheKey,
    fetch: () => _service.listCommunities(
      regionFilter: regionFilter,
      skillId: skillId,
    ),
  );
}
```

**Update `GlobalState` in `app/lib/presentation/viewmodels/global_view_model.dart`:**
```dart
@freezed
sealed class GlobalState with _$GlobalState {
  const factory GlobalState({
    @Default([]) List<CommunityItem> communities,
    @Default(true) bool isLoading,
    String? errorMessage,
    @Default('Gardening') String selectedCategory,
    @Default('Boulder') String selectedLocationName,
    @Default(false) bool isSearchActive,
    String? selectedRegionType,
    String? selectedRegionName,
    String? selectedSkillId,  // NEW: Track selected skill ID
    @Default([]) List<RegionSkillItem> availableSkills,
    @Default([]) List<AvailableRegionItem> availableRegions,
    @Default(false) bool isLoadingSkills,
  }) = _GlobalState;
}
```

**Update `GlobalNotifier.loadCommunities()`:**
```dart
Future<void> loadCommunities() async {
  final hasRegionFilter = state.selectedRegionType != null && state.selectedRegionName != null;
  final hasSkillFilter = state.selectedSkillId != null;

  _log.info('📥 Loading communities', {
    'region': hasRegionFilter ? state.selectedRegionName : 'all',
    'skill': hasSkillFilter ? state.selectedSkillId : 'all',
  });

  state = state.copyWith(isLoading: true, errorMessage: null);

  try {
    RegionFilter? regionFilter;
    if (hasRegionFilter) {
      regionFilter = RegionFilter(
        regionType: state.selectedRegionType!,
        regionName: state.selectedRegionName!,
      );
    }

    final communities = await _communityRepository.listUserCommunities(
      regionFilter: regionFilter,
      skillId: state.selectedSkillId,
    );

    state = state.copyWith(
      communities: communities,
      isLoading: false,
      errorMessage: null,
    );

    _log.info('✅ Loaded ${communities.length} communities');
  } catch (e, stackTrace) {
    _log.severe('❌ Error loading communities: $e', e, stackTrace);
    state = state.copyWith(isLoading: false, errorMessage: e.toString());
  }
}
```

**Add method to select skill by name:**
```dart
Future<void> setSelectedSkill(String skillName) async {
  // Find the skill ID from availableSkills
  final skill = state.availableSkills.firstWhere(
    (s) => s.skillName == skillName,
    orElse: () => RegionSkillItem(),
  );

  if (skill.skillId.isEmpty) {
    _log.warning('⚠️ Skill not found: $skillName');
    return;
  }

  _log.info('🎯 Setting skill filter: $skillName (${skill.skillId})');

  state = state.copyWith(
    selectedCategory: skillName,
    selectedSkillId: skill.skillId,
  );

  // Reload communities with the skill filter
  await loadCommunities();
}
```

**Update `global_screen.dart` to use skill quality scores:**
```dart
Widget _buildContent(GlobalState state, GlobalNotifier notifier) {
  if (state.isLoading) {
    return const Center(child: CircularProgressIndicator());
  }

  if (state.hasError) {
    return _buildErrorState(state.errorMessage!, notifier);
  }

  if (state.isEmpty) {
    return _buildEmptyState();
  }

  // Sort communities by skill quality score (when skill filter is active)
  final sortedCommunities = List<CommunityItem>.from(state.communities);
  sortedCommunities.sort((a, b) {
    // If skill filter is active, sort by skill quality score
    if (state.selectedSkillId != null) {
      return b.skillQualityScore.compareTo(a.skillQualityScore);
    }
    // Otherwise sort alphabetically by name
    return a.name.compareTo(b.name);
  });

  return RefreshIndicator(
    onRefresh: () => notifier.refresh(),
    child: ListView.builder(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      itemCount: sortedCommunities.length,
      itemBuilder: (context, index) {
        final community = sortedCommunities[index];
        final rank = index + 1;

        return Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: EnhancedCommunityCard(
            communityItem: community,
            rank: rank,
            onTap: () => _openCommunityScreen(community.id),
            // Use real skill quality score if available, otherwise 0
            equityScore: community.skillQualityScore,
            // Only show score when skill filter is active
            showScore: state.selectedSkillId != null,
          ),
        );
      },
    ),
  );
}
```

**Update `EnhancedCommunityCard` to conditionally show scores:**
```dart
class EnhancedCommunityCard extends StatelessWidget {
  final CommunityItem communityItem;
  final int rank;
  final VoidCallback onTap;
  final double equityScore;
  final bool showScore;  // NEW: Only show score when skill filter is active

  const EnhancedCommunityCard({
    super.key,
    required this.communityItem,
    required this.rank,
    required this.onTap,
    required this.equityScore,
    this.showScore = true,
  });

  // ... rest of implementation
}
```

#### 3. Cache Invalidation Strategy

**When to invalidate caches:**
- After a community's skills change (via events, manual skill edits)
- When skill quality scores are recomputed
- Pattern: `community:user:list:*:skill:{skill_id}`

**Repository methods:**
```dart
Future<void> invalidateSkillFilteredLists(String skillId) async {
  // Invalidate all cached lists for this skill
  await _cache.invalidatePattern('user:list:*:skill:$skillId');
}
```

**Deliverables:**
- Communities can be filtered by skill ID
- Quality scores are fetched efficiently in a single query
- Leaderboard displays real quality scores (0-100 range)
- Communities are ranked by their quality score for the selected skill
- Cache keys include skill filter to prevent stale data
- Score display is only shown when a skill filter is active

**Testing:**
- Unit tests for skill filtering in `ListCommunities`
- Integration tests for quality score retrieval
- Test cache invalidation when skills change
- Verify N+1 query prevention (single skill query per request)
- Test empty results when no communities have the selected skill

**Performance Considerations:**
- Single additional query per request: `QueryByField("skill_id", skillId, &models.CommunitySkill{})`
- Map lookup is O(1) for each community
- Total complexity: O(n + m) where n = communities, m = skill records

---

### Phase 4.3: Regional Rankings & Community Metrics Display

**Status:** Not yet implemented

**Goal:** Display regional context and rankings for community skills in the Community Metrics View, similar to how user regions are shown in the User Screen.

**User Story:** As a community member, I want to see my community's regional context and how we rank regionally for each skill, so I understand our standing within our geographic area.

#### Requirements

**1. Regional Information Display**

Display community's region in the Community Metrics View header, mirroring the user region display pattern from User Screen:
- Show primary region with icon (e.g., "📍 Boulder, CO" or "🗺️ Colorado")
- Use same visual styling as User Screen region display
- Position below community name/description in the header section

**2. Skill Thumbnail Badges**

Add regional ranking badges to skill thumbnails:
- **Badge Position:** Upper-left corner of each skill thumbnail
- **Badge Content:** Regional rank (e.g., "#3" or "1st")
- **Badge Style:**
  - Small circular or pill-shaped badge
  - Prominent background color (e.g., gold for #1, silver for #2, bronze for #3, accent color for others)
  - White text, bold weight
  - Drop shadow for visibility over thumbnail images
- **Ranking Scope:** Rank is relative to all communities in the same region with this skill
- **Conditional Display:** Only show badge when community is ranked (has the skill with a quality score)

**3. Community Skill Detail Card**

Add a new information card in Community Skill View between description and tabs:

**Card Layout:**
```
┌─────────────────────────────────────────────────┐
│                                                  │
│  [Score Icon]  87                               │
│                                                  │
│  [Trophy Icon] #3 in Boulder, CO                │
│                Top 15% of 47 communities         │
│                                                  │
└─────────────────────────────────────────────────┘
```

**Card Components:**
- **Quality Score Section:**
  - Icon: Star or checkmark icon
  - Value: Large text showing score (e.g., "87") - no "out of 100" or label text
  - Score has no upper limit, display raw value

- **Regional Ranking Section:**
  - Icon: Trophy or medal icon
  - Rank: "#3 in Boulder, CO" (use community's primary region) - no "Regional Ranking" label
  - Percentile: "Top 15% of 47 communities" (calculated from rank and total)
  - Show region name from community's primary `CommunityRegion`

**Visual Design:**
- Card background: Slightly elevated surface (subtle shadow)
- Use icons for visual hierarchy
- Adequate spacing between sections
- Responsive layout (stack vertically on narrow screens)
- Clean, minimal design without redundant labels

#### Backend Requirements

**1. Extend Existing Proto Messages**

Add regional ranking fields to existing skill data structures:

**Update `CommunitySkillData` (used in skill detail view):**
```proto
message CommunitySkillData {
  string skill_id = 1;
  string skill_name = 2;
  double quality_score = 3;

  // NEW: Regional ranking fields
  int32 regional_rank = 4;           // Rank within primary region (1 = best)
  int32 total_in_region = 5;         // Total communities with this skill in region
  string region_name = 6;            // Primary region name (e.g., "Boulder, CO")
  string region_type = 7;            // Region type (e.g., "city", "state")

  // ... existing fields
}
```

**Update `SkillItemData` (used in skill thumbnails/list):**
```proto
message SkillItemData {
  string skill_id = 1;
  string skill_name = 2;
  double quality_score = 3;

  // NEW: Regional ranking for badge display
  int32 regional_rank = 4;           // Rank within primary region (1 = best)
  int32 total_in_region = 5;         // Total communities with this skill in region

  // ... existing fields
}
```

**2. Implementation in Existing RPCs**

Update existing skill-fetching RPCs to compute and include rankings:

**In `GetCommunitySkill` RPC:**
1. Get community's `CommunitySkill` record for the skill
2. Get community's primary region (highest `member_percentage` from `CommunityRegion`)
3. Query all `CommunitySkill` records for this skill in the same region:
   - Join `CommunitySkill` with `CommunityRegion` on `community_id`
   - Filter by `skill_id` and matching `region_name` + `region_type`
4. Sort by `quality_score` descending
5. Find community's rank in sorted list
6. Populate `regional_rank`, `total_in_region`, `region_name`, `region_type` in response

**In `ListCommunitySkills` RPC:**
- Same logic as above, but compute rankings for all skills in batch
- Single query to get primary region
- Single query per skill to get regional competitors
- Efficient batch ranking computation in memory

**Performance Optimization:**
- Cache regional rankings per skill (TTL: 1 hour, key: `skill:{skill_id}:region:{region_name}:rankings`)
- Batch compute rankings for all community skills in one pass
- Only compute rankings when skill data is requested (lazy evaluation)

#### Frontend Requirements

**1. Community Metrics View Header**

Update `CommunityMetricsView` or equivalent:
- Add region display component below community description
- Fetch primary region from `ListCommunityRegions` RPC
- Display with location icon matching region type
- Use same styling as User Screen region display

**2. Skill Thumbnail Component**

Update skill thumbnail widget (in metrics view):
- Read `regionalRank` from `SkillItemData`
- Render badge in upper-left corner with `Stack` widget
- Badge styling:
  ```dart
  Container(
    padding: EdgeInsets.symmetric(horizontal: 6, vertical: 2),
    decoration: BoxDecoration(
      color: _getRankBadgeColor(rank),
      borderRadius: BorderRadius.circular(12),
      boxShadow: [BoxShadow(blurRadius: 4, color: Colors.black26)],
    ),
    child: Text(
      _formatRank(rank), // "#1", "#2", etc.
      style: TextStyle(
        color: Colors.white,
        fontSize: 10,
        fontWeight: FontWeight.bold,
      ),
    ),
  )
  ```
- Rank formatting: "#1", "#2", "1st", "2nd", "3rd", etc.
- Color scheme: Gold (#FFD700) for #1, Silver (#C0C0C0) for #2, Bronze (#CD7F32) for #3, Accent for others

**3. Community Skill View Card**

Add new widget `CommunitySkillRankingCard`:
```dart
class CommunitySkillRankingCard extends StatelessWidget {
  final CommunitySkillData skillData;

  // Layout:
  // - Quality score with icon (no label text)
  // - Regional rank with icon (no "Regional Ranking" label)
  // - Clean, minimal design
  // - Percentile calculation: "Top X% of Y communities"
  //
  // Data comes directly from CommunitySkillData:
  // - skillData.qualityScore
  // - skillData.regionalRank
  // - skillData.totalInRegion
  // - skillData.regionName
}
```

**4. Data Loading**

- Rankings are included automatically in existing skill RPCs
- No additional RPC calls needed
- Cache behavior follows existing skill caching strategy
- Show loading skeleton while fetching skills (same as current behavior)

#### UI/UX Considerations

**Badge Visibility:**
- Ensure badge text is readable over any thumbnail background
- Use drop shadow or stroke for contrast
- Consider badge size on mobile vs. desktop

**Ranking Context:**
- Show both absolute rank (#3) and relative percentile (Top 15%)
- Use "Top X%" for ranks in top half, "Bottom X%" for lower half, or just show percentile
- Handle edge cases: only community with skill, tied ranks

**Regional Scope:**
- Clearly communicate that ranking is regional, not global
- Region name should match what's shown in community header
- If community has multiple regions, use primary (highest member_percentage)

**Empty States:**
- If community has no region assigned: Show "Not ranked - no region assigned"
- If community is only one with skill in region: Show "Only community with this skill in [region]"
- If skill has no quality score: Don't show ranking

#### Testing Requirements

**Backend:**
- Test ranking calculation with various quality scores
- Test tied scores (same rank for multiple communities)
- Test communities without region assignments
- Test skills with only one community in region
- Test batch RPC performance with 20+ skills
- Verify cache invalidation when skills/regions change

**Frontend:**
- Test badge rendering on various thumbnail sizes
- Test badge contrast over light/dark thumbnail images
- Test ranking card layout on mobile and desktop
- Test loading states and error handling
- Test with missing data (no rank, no region)

#### Deliverables

1. Community region display in metrics view header
2. Regional rank badges on skill thumbnails
3. Minimal quality score & regional ranking card in skill detail view
4. Extended proto messages: `CommunitySkillData` and `SkillItemData` with ranking fields
5. Updated RPCs: `GetCommunitySkill` and `ListCommunitySkills` to compute rankings
6. Caching strategy for regional rankings (integrated with existing skill caching)
7. Tests for ranking calculation and UI components

#### Future Enhancements (Not in Phase 4.3)

- **Trend Indicators:** Show if rank improved/declined over time (↑↓ arrows)
- **Multiple Region Rankings:** If community spans multiple regions, show rank in each
- **Skill Comparison:** Compare community's ranking across all its skills
- **Leaderboard Link:** Link from ranking card to global leaderboard filtered by this skill
- **Historical Charts:** Graph showing rank changes over time

---

### Phase 4.6: Fix Manual Region Override with Mapbox Search (1 week)

**Status:** Planning
**Priority:** High - Current implementation is blocking community creators from setting regions

**Problem Statement:**

The current `SetCommunityRegionOverride` RPC requires a `region_id` parameter referencing an existing region in the database. This creates a chicken-and-egg problem:
1. Community creators want to manually set their community's region
2. The governance screen uses `GeographicAreaAutocompleteField` which only shows regions that already have communities
3. If a community wants to set a region that no other communities use yet (e.g., a new city), they cannot do so
4. The client cannot create new regions - only the server can

**Current Flow (Broken):**
```
User selects region from GeographicAreaAutocompleteField
  → Returns RegionItem with existing region_id
  → Calls SetCommunityRegionOverride(community_id, region_id)
  → Server verifies region exists (line 98-102 in regions.go)
  → ERROR if region doesn't exist
```

**Issue:** The governance screen should allow creators to search for ANY geographic location (city, county, state), not just regions that already have communities.

#### Proposed Solution

**Client does Mapbox lookup once, then server creates region from provided details:**

1. **Client Changes:**
   - Replace `GeographicAreaAutocompleteField` with Mapbox location search in governance screen
   - User types location query (e.g., "Boulder, CO", "Denver County", "Colorado")
   - Client calls Mapbox Search Box API directly (already integrated via `MapboxLocationService`)
   - User selects location from Mapbox results
   - Client extracts all necessary details from the Mapbox response:
     - Coordinates (latitude, longitude)
     - Full address components (neighborhood, locality, county, administrative_area)
     - Region type (derived from Mapbox feature type)
   - Client sends all extracted details to server (NOT a location query string)

2. **Proto Changes:**
   - Create new `CreateRegionFromAddressRequest` RPC that accepts full address details
   - Update `SetCommunityRegionOverride` to accept `region_id` (keep existing interface)
   - Two-step process: create region first, then set override

3. **Server Changes:**
   - New RPC: `CreateRegionFromAddress` that:
     - Accepts full address details from client (no Mapbox calls needed)
     - Uses existing `regionlib.GetOrCreateRegion()` to find or create the region
     - Returns the region_id
   - Existing `SetCommunityRegionOverride` remains unchanged
   - No duplicate Mapbox API calls - client has already done the geocoding

#### Architecture Alignment

**Reuse Existing Infrastructure:**
- **Client-side Mapbox:** Use existing `MapboxLocationService.searchAddresses()` in Flutter app
  - Already implements Search Box API integration
  - Returns full address details including coordinates and administrative hierarchy
  - No server-side Mapbox calls needed
- **Region Creation:** Use existing `regionlib.GetOrCreateRegion()` in `server/location/regions.go` (lines 21-80)
  - Handles deduplication by type + name + parent
  - Creates hierarchical paths
  - Normalizes region names

**Efficiency Benefits:**
- Only ONE Mapbox API call (client-side search) instead of TWO (client search + server reverse geocode)
- Server receives all needed data from client, no external API calls
- Follows established pattern from Location creation flow

#### Implementation Details

**1. Create New Proto (`proto/ripls/api/location_service.proto`):**

```proto
// Request to create a region from address details (for manual community region override)
message CreateRegionFromAddressRequest {
  // Full address with administrative hierarchy from Mapbox
  ripls.models.Address address = 1;

  // Coordinates
  ripls.models.Geolocation geolocation = 2;

  // Preferred region type (client's best guess based on Mapbox feature type)
  // One of: "neighborhood", "city", "county", "state"
  string preferred_region_type = 3;
}

message CreateRegionFromAddressResponse {
  // The created or found region ID
  string region_id = 1;

  // The actual region type that was used (may differ from preferred)
  string region_type = 2;

  // The region name
  string region_name = 3;
}
```

**2. Add New Server RPC (`server/services/location/regions.go` - new file):**

```go
// CreateRegionFromAddress creates or finds a region based on address details.
// This is used by the community governance flow to create regions for manual override.
func (s *Service) CreateRegionFromAddress(
	ctx context.Context,
	req *connect.Request[api.CreateRegionFromAddressRequest],
) (*connect.Response[api.CreateRegionFromAddressResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CreateRegionFromAddress",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"preferred_type", req.Msg.PreferredRegionType,
	)

	address := req.Msg.Address
	if address == nil {
		logger.WarnContext(ctx, "address is required")
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("address is required"))
	}

	// Determine region type and name from address components
	// Priority: neighborhood > city > county > state
	var regionType, regionName, regionCode, parentRegionID string

	if address.Neighborhood != "" && req.Msg.PreferredRegionType == "neighborhood" {
		regionType = "neighborhood"
		regionName = address.Neighborhood
		// Parent is city
		if address.Locality != "" {
			cityRegionID, _ := regionlib.GetOrCreateRegion(ctx, s.storage, "city", address.Locality, "", "")
			parentRegionID = cityRegionID
		}
	} else if address.Locality != "" {
		// City level (most common)
		regionType = "city"
		regionName = address.Locality
		// Scope city name with state (e.g., "Boulder, CO")
		if address.AdministrativeArea != "" {
			stateCode := extractStateCode(address.AdministrativeArea)
			if stateCode != "" {
				regionName = fmt.Sprintf("%s, %s", address.Locality, stateCode)
			}
		}
		// Parent is state
		if address.AdministrativeArea != "" {
			stateRegionID, _ := regionlib.GetOrCreateRegion(ctx, s.storage, "state", address.AdministrativeArea, extractStateCode(address.AdministrativeArea), "")
			parentRegionID = stateRegionID
		}
	} else if address.County != "" {
		// County level
		regionType = "county"
		regionName = address.County
		// Parent is state
		if address.AdministrativeArea != "" {
			stateRegionID, _ := regionlib.GetOrCreateRegion(ctx, s.storage, "state", address.AdministrativeArea, extractStateCode(address.AdministrativeArea), "")
			parentRegionID = stateRegionID
		}
	} else if address.AdministrativeArea != "" {
		// State level
		regionType = "state"
		regionName = address.AdministrativeArea
		regionCode = extractStateCode(address.AdministrativeArea)
	} else {
		logger.WarnContext(ctx, "unable to determine region from address")
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unable to determine region type"))
	}

	logger = logger.With(
		"region_type", regionType,
		"region_name", regionName,
	)

	// Create or find the region using existing library
	regionID, err := regionlib.GetOrCreateRegion(ctx, s.storage, regionType, regionName, regionCode, parentRegionID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create or find region", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create region"))
	}

	logger.InfoContext(ctx, "created or found region", "region_id", regionID)

	return connect.NewResponse(&api.CreateRegionFromAddressResponse{
		RegionId:   regionID,
		RegionType: regionType,
		RegionName: regionName,
	}), nil
}

// extractStateCode extracts state code from state name (e.g., "Colorado" -> "CO").
func extractStateCode(stateName string) string {
	// TODO: Implement state name -> code lookup table
	// For now, return empty string - will store full state name
	return ""
}
```

**3. Update Client (`app/lib/presentation/screens/governance/governance_screen.dart`):**

Replace `GeographicAreaAutocompleteField` with `LocationAutocompleteField`:

```dart
LocationAutocompleteField(
  controller: _locationController,
  onPlaceSelected: (place) async {
    // Extract address details from Mapbox result
    await _setOverrideFromPlace(place);
  },
  hintText: 'Search for city, county, or state...',
)
```

Add new method to handle region creation and override:

```dart
Future<void> _setOverrideFromPlace(LocationResult place) async {
  try {
    final locationService = ref.read(locationServiceProvider);
    final communityService = ref.read(communityServiceProvider);

    // Step 1: Create region from Mapbox place details
    final regionResponse = await locationService.createRegionFromAddress(
      address: place.address,  // Full address from Mapbox
      geolocation: place.geolocation,
      preferredRegionType: _determineRegionType(place.type),
    );

    // Step 2: Set community region override with the new region ID
    await communityService.setCommunityRegionOverride(
      communityId: widget.communityId,
      regionId: regionResponse.regionId,
    );

    if (mounted) {
      ToastHelper.showSuccess(
        context,
        'Region updated to ${regionResponse.regionName}'
      );
      await _loadCurrentRegions();
    }
  } catch (e) {
    if (mounted) {
      ToastHelper.showError(context, 'Failed to update region: $e');
    }
  }
}

String _determineRegionType(String mapboxType) {
  // Map Mapbox feature types to our region types
  switch (mapboxType) {
    case 'neighborhood':
      return 'neighborhood';
    case 'locality':
    case 'place':
      return 'city';
    case 'district':
      return 'county';
    case 'region':
      return 'state';
    default:
      return 'city'; // Default to city
  }
}
```

**4. Add Service Method (`app/lib/services/location_service.dart`):**

```dart
Future<CreateRegionFromAddressResponse> createRegionFromAddress({
  required Address address,
  required Geolocation geolocation,
  required String preferredRegionType,
}) async {
  return RpcUtils.executeRpc(
    () async {
      final request = CreateRegionFromAddressRequest(
        address: address,
        geolocation: geolocation,
        preferredRegionType: preferredRegionType,
      );
      final response = await _client.createRegionFromAddress(
        request,
        headers: _buildHeaders(),
      );
      return response;
    },
    _errorHandler,
    onUnauthenticated: _onUnauthenticated,
  );
}
```

**5. Keep Existing `SetCommunityRegionOverride` Unchanged:**

The existing RPC in `community_service.proto` and implementation stays the same - it already accepts `region_id`.

#### Why This Approach

**✅ Advantages:**
1. **Unblocks Creators:** Community creators can now set ANY region, not just existing ones
2. **Efficient:** Only ONE Mapbox API call (client-side) instead of TWO (client + server)
3. **No Server-Side API Calls:** Server receives all data from client, no external dependencies
4. **Reuses Infrastructure:** Uses existing `MapboxLocationService` client-side integration
5. **Automatic Region Creation:** Regions are created automatically via `GetOrCreateRegion`
6. **Deduplication:** Existing region library handles normalization and deduplication
7. **Hierarchical:** Creates proper parent-child region relationships (city → state)
8. **Consistent UX:** Uses same location search pattern as gear/user locations
9. **Follows Existing Pattern:** Same flow as SaveLocation (client geocodes, server stores)

**⚠️ Tradeoffs:**
- **Two-Step Process:** Client must call CreateRegionFromAddress, then SetCommunityRegionOverride
  - Acceptable: Keeps region creation logic in LocationService where it belongs
  - Clear separation of concerns (location vs. community services)
- **State Code Lookup:** Need to implement state name → code mapping
  - Can defer: Store full state name initially, add lookup table later
- **Client-Side Region Type Mapping:** Client must map Mapbox feature types to region types
  - Simple mapping logic, well-documented in governance screen

#### Architecture Compliance

**✅ Follows Best Practices:**
1. **Service Independence:** LocationService doesn't depend on CommunityService (clean separation)
2. **Shared Library Pattern:** Uses existing `regionlib.GetOrCreateRegion()` for region creation logic
3. **Structured Logging:** Uses `logging.LoggerWithContext(ctx)` with proper field masking
4. **Repository Pattern (Client):** Client uses LocationService through LocationRepository (not direct)
5. **Async-First (Client):** All data access is async with FutureBuilder/AsyncValue
6. **No Manual Caching (Client):** Repository handles all caching, no manual cache maps in ViewModels

**Server-Side Patterns:**
- ✅ Uses `auth.RequireAuth(ctx)` for authentication
- ✅ Proper error handling with `connect.NewError()`
- ✅ Structured logging with operation, user_id, masked email
- ✅ Input validation with descriptive error messages
- ✅ Leverages shared `regionlib` library (no service-to-service dependencies)

**Client-Side Patterns:**
- ✅ ViewModel calls Repository (not Service directly)
- ✅ Repository wraps Service with caching
- ✅ All methods are async (`Future<T>`)
- ✅ Uses existing `MapboxLocationService` for geocoding
- ✅ Follows established Location creation pattern

#### Testing Plan

**Unit Tests (Required):**

**Server-Side (`server/services/location/regions_test.go`):**
1. Test region creation from address components (table-driven tests)
   - City with state → creates "Boulder, CO" region
   - County with state → creates "Boulder County" region
   - State only → creates "Colorado" region
   - Neighborhood with city → creates neighborhood with city parent
2. Test `extractStateCode()` function (when implemented)
3. Test error cases:
   - Missing address in request
   - Empty address (no locality, county, or state)
   - GetOrCreateRegion failures
4. Test region deduplication (same address twice returns same region_id)

**Client-Side (`app/test/services/location_service_test.dart`):**
1. Test createRegionFromAddress success path
2. Test error handling (invalid address, network errors)
3. Mock service responses

**Client-Side (`app/test/data/repositories/location_repository_test.dart`):**
1. Test that createRegionFromAddress delegates to service (no caching)
2. Verify correct parameters passed through

**Client-Side (`app/test/presentation/screens/governance/governance_screen_test.dart`):**
1. Test region selection flow with mocked services
2. Test error display on region creation failure
3. Test success message after override

**Integration Tests:**
1. End-to-end: governance screen → Mapbox → region creation → override set
2. Test with regions that already exist (should reuse)
3. Test with new regions (should create)
4. Test permission denied for non-creators

**Manual Testing:**
1. Set override for city that exists: "Boulder, CO"
2. Set override for city that doesn't exist: "Aspen, CO"
3. Set override for state: "Colorado"
4. Set override for county: "Boulder County"
5. Verify region appears in ListCommunityRegions
6. Verify override flag is set correctly

#### Deliverables

1. Updated proto with `location_query` field
2. Updated server RPC with Mapbox geocoding integration
3. Updated client with location search (remove GeographicAreaAutocompleteField)
4. Unit and integration tests
5. Manual testing confirmation
6. Documentation update for manual override flow

#### Timeline

- **Day 1-2:** Proto changes + code generation + server implementation
- **Day 3-4:** Client updates (governance screen + service layer)
- **Day 5:** Unit tests + integration tests
- **Day 6:** Manual testing + bug fixes
- **Day 7:** Documentation + code review

---

### Phase 4.7: Global Screen Redesign - CommunitySkillView-Inspired Layout (1 week)

**Status:** Planning
**Priority:** Medium - UX improvement for better content discovery

**Goal:** Redesign the global_screen to adopt a more visual, content-focused layout similar to CommunitySkillView, with a prominent skill description card and thumbnail grid of communities.

#### Current State

The current global_screen shows:
- Header with region name and search icon
- Filter row with selected skill chip
- Vertical list of `EnhancedCommunityCard` components showing communities ranked by skill quality score
- Each card displays: rank, community name, description, score, member count

#### Proposed Design

Redesign to mirror CommunitySkillView layout:

**Top Header:**
- Region name (e.g., "Boulder, CO" or "All Regions")
- Search icon that toggles a search input area for changing the region
- Uses same region selection as current implementation

**Filter Row:**
- Horizontal scrolling skill chips (same as current)
- Selected skill highlighted with accent color
- Shows top skills for the selected region

**Skill Description Card:**
- Large card with skill image/illustration
- Skill name as title
- Rich description text explaining what this skill represents
- Visual styling similar to experience cards or gear detail cards
- Prominent placement to give context before showing communities

**Community Thumbnail Grid:**
- Replace vertical list with 2-column grid (mobile) or 3-column grid (tablet/desktop)
- Each thumbnail shows:
  - Community media image (if available) or placeholder
  - Community name overlay
  - Skill quality score badge in upper-right corner (circular badge with score)
  - Tap to navigate to community detail screen
- Grid uses `GridView.builder` with responsive column count
- Maintains rank-based ordering (highest score first)

#### Layout Structure

```
┌─────────────────────────────────────────────────┐
│  Boulder, CO                           [search] │  ← Header
├─────────────────────────────────────────────────┤
│  [Gardening] [Woodworking] [Cycling] ...       │  ← Filter chips
├─────────────────────────────────────────────────┤
│  ┌───────────────────────────────────────────┐ │
│  │  [Gardening Image]                        │ │
│  │                                           │ │  ← Skill card
│  │  Gardening                                │ │
│  │  Growing plants, vegetables, and flowers  │ │
│  │  for food, beauty, and sustainability.    │ │
│  └───────────────────────────────────────────┘ │
├─────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐           │
│  │ [Community   │  │ [Community   │           │
│  │  Image]      │  │  Image]      │           │
│  │              │  │              │           │  ← Grid
│  │ Community A  │  │ Community B  │           │
│  │        [87] ←│  │        [85] ←│           │  ← Badges
│  └──────────────┘  └──────────────┘           │
│  ┌──────────────┐  ┌──────────────┐           │
│  │ [Community   │  │ [Community   │           │
│  │  Image]      │  │  Image]      │           │
│  │              │  │              │           │
│  │ Community C  │  │ Community D  │           │
│  │        [82] ←│  │        [79] ←│           │
│  └──────────────┘  └──────────────┘           │
└─────────────────────────────────────────────────┘
```

#### Implementation Details

**1. Remove Tabs (No Member/Experience Sections)**

Unlike CommunitySkillView which has "Members" and "Experiences" tabs, the global_screen will only show the community grid. This keeps the focus on discovering and comparing communities.

**2. Skill Description Card Component**

Extract reusable widget: `SkillDescriptionCard` from existing implementations

**Current State:**
- `UserSkillView._buildDescriptionCard()` (lines 206-253)
- `CommunitySkillsView._buildDescriptionCard()` (lines 208-266)
- Both have nearly identical implementations with skill media + expandable description

**Refactor to Reusable Widget:**

Create `/app/lib/presentation/widgets/skill/skill_description_card.dart`:

```dart
class SkillDescriptionCard extends ConsumerStatefulWidget {
  final String skillDescription;
  final List<String> skillMediaIds;
  final Widget? footer; // Optional: for ranking info in CommunitySkillsView

  const SkillDescriptionCard({
    super.key,
    required this.skillDescription,
    this.skillMediaIds = const [],
    this.footer,
  });
}
```

**Implementation Details:**
- 80x80px skill media on left (or gradient placeholder with lightbulb icon)
- Expandable description text on right (tap to expand/collapse, 3 lines max)
- Optional footer widget for community ranking sentence
- Card styling: rounded corners (16px), border, subtle shadow
- Reuses existing `_buildSkillMedia()` and `_buildSkillMediaPlaceholder()` patterns

**Usage in Global Screen:**
```dart
SkillDescriptionCard(
  skillDescription: selectedSkill.description,
  skillMediaIds: selectedSkill.mediaIds,
  // No footer needed for global screen
)
```

**Data Source:**
- ✅ Skill proto already includes `description` field (line 23 in skill.proto)
- ✅ Skill proto already includes `media_ids` field (line 26 in skill.proto)
- ✅ Descriptions generated by LLM during skill extraction
- ✅ Media automatically fetched from Unsplash when skill is created

**3. Community Thumbnail Component**

Reuse existing widget: `CommunityThumbnail` (from `/app/lib/presentation/widgets/community/community_thumbnail.dart`)

This widget is already used in UserScreen and provides:
- Square aspect ratio (1:1)
- Background: community image or gradient placeholder
- Bottom overlay: community name (white text, dark gradient background)
- Upper-right badge via `badgeText` parameter
- Tap navigation to CommunityPublicScreen

**Usage:**
```dart
CommunityThumbnail(
  community: communityItem,
  badgeText: communityItem.skillQualityScore.round().toString(),
)
```

**Badge Styling (from existing widget):**
- Small rectangular badge with rounded corners (10px radius)
- Green background (Colors.green.shade600)
- White text, bold weight (fontSize: 10, fontWeight: w700)
- Position: 8px from top-right corner
- Padding: horizontal 6px, vertical 2px

**4. Grid Configuration**

```dart
GridView.builder(
  padding: const EdgeInsets.all(16),
  gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
    crossAxisCount: _getColumnCount(context),  // 2 mobile, 3 tablet/desktop
    childAspectRatio: 1.0,  // Square thumbnails
    crossAxisSpacing: 12,
    mainAxisSpacing: 12,
  ),
  itemCount: sortedCommunities.length,
  itemBuilder: (context, index) {
    final community = sortedCommunities[index];
    return CommunityThumbnail(
      community: community,
      badgeText: community.skillQualityScore.round().toString(),
    );
  },
)
```

**5. Header with Region Search**

Keep existing header implementation:
- Display region name (or "All Regions")
- Search icon toggles location search input
- Location search uses `GeographicAreaAutocompleteField` (from Phase 4)
- Selecting region filters communities and updates skill chips

**6. Responsive Layout**

- **Mobile (< 600px):** 2-column grid
- **Tablet (600-900px):** 3-column grid
- **Desktop (> 900px):** 3 or 4-column grid
- Skill description card: full width on all breakpoints
- Skill chips: horizontal scrolling on mobile

**7. Empty States**

- **No communities for skill:** "No communities found for [Skill] in [Region]"
- **No skill selected:** Show all communities (sorted alphabetically)
- **No region selected:** "Select a region to see top communities"

#### Data Flow Changes

**Current:**
```
1. Load communities with region + skill filter
2. Sort by skill quality score
3. Display in vertical list (EnhancedCommunityCard)
```

**New:**
```
1. Load communities with region + skill filter
2. Load skill details (name, description, image)
3. Sort communities by skill quality score
4. Display skill card
5. Display communities in grid (CommunityThumbnailCard)
```

**Additional Data Requirements:**
- ✅ Skill descriptions already exist in `Skill.description` field
- ✅ Skill images already exist in `Skill.media_ids` field
- ✅ Data automatically populated during skill creation (LLM + Unsplash)
- ⚠️ Need to expose skill data in `ListCommunities` response or create new `GetSkill` RPC

#### Backend Changes

**No proto changes needed** - Skill model already has required fields:

```proto
// proto/ripls/models/skill.proto (EXISTING)
message Skill {
  string id = 1;
  string name = 2;
  string description = 3;          // ✅ Already exists
  repeated string media_ids = 4;   // ✅ Already exists
}
```

**New RPC Required:**

Add `GetSkill` RPC to expose skill details for global screen:

```proto
// proto/ripls/api/skill_service.proto
message GetSkillRequest {
  string skill_id = 1;
}

message GetSkillResponse {
  string id = 1;
  string name = 2;
  string description = 3;
  repeated string media_ids = 4;
}
```

**Alternative:** Extend `RegionSkillItem` (from Phase 4) to include description and media:

```proto
// proto/ripls/api/community_service.proto (UPDATE EXISTING)
message RegionSkillItem {
  string skill_id = 1;
  string skill_name = 2;
  int32 community_count = 3;
  double total_quality_score = 4;

  // NEW: Add skill details for global screen
  string description = 5;
  repeated string media_ids = 6;
}
```

**Recommendation:** Extend `RegionSkillItem` to avoid extra RPC calls. Skills are already fetched via `ListTopSkillsForRegion`.

#### UI/UX Improvements Over Current Design

**✅ Advantages:**
1. **Visual Hierarchy:** Skill card gives context before showing communities
2. **Scannable Grid:** Easier to compare multiple communities at a glance
3. **Consistent Scores:** Badge in same position across all thumbnails
4. **Less Scrolling:** Grid shows more communities per viewport
5. **Better Media Usage:** Community images are prominent (not just small thumbnails)
6. **Cleaner Design:** Removes redundant text (descriptions) from list view
7. **Skill Context:** Users understand what the skill means before seeing rankings

**⚠️ Tradeoffs:**
- **Less Detail Per Community:** No description text in grid view (must tap to see)
- **More Visual Noise:** Grid with images can be busier than clean list
- **Requires Good Images:** Communities without media need attractive placeholders

#### Testing Requirements

**Frontend:**
- Test responsive grid layouts (2, 3, 4 columns)
- Test score badge visibility on light/dark community images
- Test empty states (no communities, no skill selected)
- Test skill card with long descriptions (ellipsis/truncation)
- Test navigation from thumbnail to community detail screen

**Visual Regression:**
- Compare old list layout vs. new grid layout
- Verify score values match between designs
- Test on various screen sizes (mobile, tablet, desktop)

**Performance:**
- Test grid scrolling with 50+ communities
- Verify image loading doesn't block UI (use cached_network_image)
- Test skill card image loading

#### Implementation Steps

**Day 1: Backend - Extend RegionSkillItem**
- Add `description` and `media_ids` fields to `RegionSkillItem` proto
- Update `ListTopSkillsForRegion` RPC to populate new fields from Skill table
- Run `npm run generate` to regenerate code
- Test RPC returns skill details correctly

**Day 2: Component Refactoring**
- Extract `SkillDescriptionCard` widget from duplicate code in UserSkillView and CommunitySkillsView
- Create `/app/lib/presentation/widgets/skill/skill_description_card.dart`
- Refactor UserSkillView to use new SkillDescriptionCard widget
- Refactor CommunitySkillsView to use new SkillDescriptionCard widget (with footer for ranking)
- Test widget in isolation with mock data
- Verify existing screens still work correctly after refactoring

**Day 3-4: Layout Integration**
- Replace vertical list with GridView in global_screen
- Add responsive column count logic (_getColumnCount helper)
- Position skill card above grid
- Wire up skill data from GlobalState
- Keep existing header with search functionality

**Day 5: Refinement**
- Test responsive layouts on mobile/tablet/desktop
- Implement empty states (no communities, no skill selected)
- Add loading skeletons for skill card + grid
- Handle missing skill media (fallback to gradient placeholder)

**Day 6: Testing & Bug Fixes**
- Manual testing on devices
- Fix visual issues (badge visibility, text overflow)
- Test with various data scenarios (few/many communities, missing skill data)
- Verify CommunityThumbnail navigation works correctly

**Day 7: Polish & Documentation**
- Final UX polish
- Update CLAUDE.md with new screen structure
- Add widget tests for SkillDescriptionCard
- Screenshot documentation

#### Deliverables

1. Extended `RegionSkillItem` proto with skill details
2. Updated `ListTopSkillsForRegion` RPC implementation
3. **Extracted** `SkillDescriptionCard` widget (refactored from duplicate code)
4. Refactored `UserSkillView` to use `SkillDescriptionCard`
5. Refactored `CommunitySkillsView` to use `SkillDescriptionCard`
6. Updated `global_screen.dart` with grid layout
7. Reused `CommunityThumbnail` widget (no changes needed)
8. Responsive grid configuration
9. Empty state handling
10. Widget tests for SkillDescriptionCard
11. Documentation updates

#### Future Enhancements (Not in Phase 4.7)

- **Skill Images:** Upload custom skill images to media library
- **Community Filters:** Toggle filters (public/private, size, activity level)
- **Sort Options:** Switch between score, alphabetical, member count
- **List/Grid Toggle:** Allow users to switch between list and grid views
- **Pagination:** Load more communities as user scrolls (infinite scroll)

---

### Phase 5: Multi-Region Support (Future)

**Goal:** Allow communities to appear in multiple region leaderboards.

**Tasks:**
1. Lower threshold from 50% → 20% for multi-region inclusion
2. Update leaderboard to show community presence (e.g., "60% Boulder, 25% Denver")
3. De-duplicate communities in search results (show once with multiple regions)

**Deferred Rationale:** Complexity not justified until user demand validated.

---

### Phase 6: Neighborhood & County Filters (Future)

**Goal:** Support finer-grained neighborhood filtering.

**Tasks:**
1. Populate `neighborhood` field from Mapbox Tilequery
2. Add neighborhood option to region type selector
3. Handle areas with no defined neighborhoods (fallback to city)

**Deferred Rationale:** Neighborhood data coverage varies by city. Wait for user feedback on desired granularity.

---

## Data Model Changes

### New Proto Messages

**File:** `proto/ripls/models/community.proto`

```proto
// Association between a community and a geographic region.
// Communities can have multiple region associations (city, state, country).
message CommunityRegion {
  // Unique identifier for this association.
  string id = 1;

  // The community this region association belongs to.
  string community_id = 2;

  // Type of region (e.g., "neighborhood", "city", "county", "state", "country").
  string region_type = 3;

  // Human-readable region name (e.g., "Boulder, CO", "Colorado", "United States").
  // Scoped to avoid duplicates (includes parent region for cities).
  string region_name = 4;

  // Optional: Structured region code for exact matching (e.g., Mapbox place ID, FIPS code).
  string region_code = 5;

  // Percentage of community members whose primary residence is in this region (0.0 - 1.0).
  // Used for determining primary region and filtering thresholds.
  // For manually overridden regions, this may be 1.0 (100%) even with few members.
  double member_percentage = 6;

  // True if this region was manually set by creator/admin, false if auto-computed from member locations.
  // Manual overrides take precedence over auto-computation until community reaches threshold size.
  bool is_override = 7;

  // When this association was created (Unix timestamp in seconds).
  int64 created_at_unix_sec = 8;

  // When this association was last updated (Unix timestamp in seconds).
  int64 updated_at_unix_sec = 9;
}
```

**File:** `proto/ripls/models/location.proto`

```proto
// Enhanced Address with administrative hierarchy
message Address {
    // CLDR region code (e.g., "US", "GB", "JP") - existing field
    string region_code = 1;

    // Postal code (e.g., "94043", "SW1A 1AA") - existing field
    string postal_code = 2;

    // City or locality name (e.g., "Austin", "London") - existing field
    string locality = 3;

    // Street address lines (e.g., ["506 Monroe St", "Building 41"]) - existing field
    repeated string address_lines = 4;

    // NEW: Neighborhood name (e.g., "Capitol Hill", "SoHo")
    // Optional - may be empty if neighborhood not defined
    string neighborhood = 5;

    // NEW: County or equivalent administrative division (e.g., "Boulder County", "King County")
    // Optional - may be empty for countries without counties
    string county = 6;

    // NEW: State/province/administrative area (e.g., "Colorado", "California", "Ontario")
    // Maps to Mapbox "region" field
    string administrative_area = 7;
}
```

### New RPC API

**File:** `proto/ripls/api/community_service.proto`

```proto
// Filter for regional leaderboard queries
message RegionFilter {
  // Type of region (e.g., "city", "state", "country", "global")
  string region_type = 1;

  // Human-readable region name (e.g., "Boulder, CO", "Colorado", "United States")
  // Must match CommunityRegion.region_name for exact filtering
  string region_name = 2;
}

// Modified ListCommunitiesRequest to support regional filtering
message ListCommunitiesRequest {
  // Optional: Filter communities by region
  RegionFilter region_filter = 1;

  // Optional: Filter communities by skill
  string skill_id = 2;
}

// Request to manually override a community's region (for new communities or special cases)
message SetCommunityRegionOverrideRequest {
  // Community ID to update
  string community_id = 1;

  // Region type (e.g., "city", "state")
  string region_type = 2;

  // Region name (e.g., "Boulder, CO")
  string region_name = 3;

  // Optional: Mapbox place ID for exact matching
  string region_code = 4;
}

// Response after setting region override
message SetCommunityRegionOverrideResponse {}

// Request to list regions associated with a community
message ListCommunityRegionsRequest {
  string community_id = 1;
}

// Response with list of regions
message ListCommunityRegionsResponse {
  repeated CommunityRegionItem regions = 1;
}

// A single region association for a community
message CommunityRegionItem {
  string region_type = 1;
  string region_name = 2;
  string region_code = 3;
  double member_percentage = 4;
  bool is_override = 5;  // True if manually set, false if auto-computed
}
```

### Database Schema

**Table:** `community_regions`

```sql
CREATE TABLE community_regions (
    id TEXT PRIMARY KEY,
    community_id TEXT NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    region_type TEXT NOT NULL CHECK (region_type IN ('neighborhood', 'city', 'county', 'state', 'country')),
    region_name TEXT NOT NULL,
    region_code TEXT,
    member_percentage REAL NOT NULL CHECK (member_percentage >= 0.0 AND member_percentage <= 1.0),
    is_override BOOLEAN NOT NULL DEFAULT FALSE,  -- True if manually set by creator/admin
    created_at_unix_sec BIGINT NOT NULL,
    updated_at_unix_sec BIGINT NOT NULL,
    UNIQUE (community_id, region_type, region_name)
);

CREATE INDEX idx_community_regions_region ON community_regions(region_type, region_name);
CREATE INDEX idx_community_regions_community ON community_regions(community_id);
```

**Table:** `locations` (modified)

```sql
ALTER TABLE locations
ADD COLUMN neighborhood TEXT,
ADD COLUMN county TEXT,
ADD COLUMN administrative_area TEXT;

CREATE INDEX idx_locations_locality ON locations(locality);
CREATE INDEX idx_locations_administrative_area ON locations(administrative_area);
```

---

## Alternative Approaches Considered

### IP Geolocation

**Concept:** Geolocate users by IP address instead of requiring location input.

**Rejected Because:**
- Inaccurate (city-level at best, often wrong)
- VPN/proxy interference
- Privacy concerns (tracking without consent)
- Already have user-provided locations (better data)

### Community Tags

**Concept:** Allow creators to add freeform location tags (e.g., #Boulder #Colorado).

**Rejected Because:**
- Unstructured data (hard to query)
- Tag spam / misrepresentation
- No validation of accuracy
- Doesn't solve administrative hierarchy problem

### Single Community Location Field

**Concept:** Add `location_id` to Community model (Solution B variant).

**Rejected Because:**
- Static (doesn't update with membership changes)
- Single-region limitation (can't represent distributed communities)
- Manual creator input (user friction)
- Doesn't reflect actual member distribution

---

## Success Metrics

**Phase 1 (Location Enrichment):**
- ✅ 95%+ of locations have `administrative_area` populated
- ✅ Mapbox Tilequery API calls <100ms p95 latency
- ✅ Zero data corruption during backfill migration

**Phase 2 (CommunityRegion Computation):**
- ✅ 90%+ of communities have at least one region association
- ✅ Region recomputation completes <10 seconds for largest community
- ✅ Event-driven updates lag <5 minutes from member change

**Phase 3 (Regional Leaderboard):**
- ✅ Filtered leaderboard queries <500ms p95 latency
- ✅ Zero user-reported incorrect region associations
- ✅ 30%+ of global screen users apply region filter

**Phase 4 (UX Enhancements):**
- ✅ "Near Me" filter adoption >20% of sessions
- ✅ Location search typeahead <200ms p95 response time
- ✅ User satisfaction survey: >4.0/5.0 for regional filtering feature

---

## References

### Internal Documentation
- [Client Geospatial System](../client/geospatial.md)
- [Community Proto](../../proto/ripls/models/community.proto)
- [Location Proto](../../proto/ripls/models/location.proto)
- [Skill Proto](../../proto/ripls/models/skill.proto)
- [Global Screen Implementation](../../app/lib/presentation/screens/global/global_screen.dart)

### External Resources
- [Mapbox Search Box API](https://docs.mapbox.com/api/search/search-box/)
- [Mapbox Tilequery API](https://docs.mapbox.com/api/maps/tilequery/)
- [CLDR Territory Codes](https://cldr.unicode.org/)
- [PostgreSQL PostGIS](https://postgis.net/documentation/)
- [US Census TIGER/Line Shapefiles](https://www.census.gov/geographies/mapping-files/time-series/geo/tiger-line-file.html)

---

**Last Updated:** 2025-12-27
**Status:** Ready for Review
