package simulation

import (
	"fmt"
	"time"
)

// SuburbanNeighborhood creates a medium-sized community of 17 members
// with heavy gear sharing, neighborhood events, and 6 months of activity.
func SuburbanNeighborhood() Scenario {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	return Scenario{
		Name:        "suburban-neighborhood",
		Description: "A suburban neighborhood sharing tools, outdoor gear, and hosting community events",
		Communities: []CommunityDef{
			{
				Name:        "Oakwood Heights Neighbors",
				Description: "Neighbors helping neighbors in the Oakwood Heights subdivision",
				Region:      "Austin, TX",
				Members: []MemberDef{
					{Name: "Marcus Thompson", Email: "marcus@example.com", Persona: PersonaAlfred},
					{Name: "David Chen", Email: "david@example.com", Persona: PersonaAlfred},
					{Name: "Robert Kim", Email: "robert@example.com", Persona: PersonaAlfred},
					{Name: "Sarah Johnson", Email: "sarah@example.com", Persona: PersonaDerek},
					{Name: "Mike Patel", Email: "mike@example.com", Persona: PersonaDerek},
					{Name: "Lisa Garcia", Email: "lisa@example.com", Persona: PersonaDerek},
					{Name: "Tom Wilson", Email: "tom@example.com", Persona: PersonaDerek},
					{Name: "Jake Martinez", Email: "jake@example.com", Persona: PersonaGary},
					{Name: "Chris Lee", Email: "chris@example.com", Persona: PersonaGary},
					{Name: "Alex Rivera", Email: "alex@example.com", Persona: PersonaGary},
					{Name: "Nancy Park", Email: "nancy@example.com", Persona: PersonaBetty},
					{Name: "Karen Wright", Email: "karen@example.com", Persona: PersonaBetty},
					{Name: "Jenny Brooks", Email: "jenny@example.com", Persona: PersonaEmma},
					{Name: "Amy Foster", Email: "amy@example.com", Persona: PersonaEmma},
					{Name: "Rachel Adams", Email: "rachel@example.com", Persona: PersonaEmma},
					{Name: "Brian Cox", Email: "brian@example.com", Persona: PersonaHenry},
					{Name: "Steve Rogers", Email: "steve@example.com", Persona: PersonaHenry},
				},
			},
		},
		StartTime: now.Add(-6 * 30 * 24 * time.Hour), // 6 months ago.
		EndTime:   now,
	}
}

// CollegeFriends creates a small group of 7 college friends with
// frequent borrowing, events, and 3 months of activity.
func CollegeFriends() Scenario {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	return Scenario{
		Name:        "college-friends",
		Description: "A tight-knit group of college friends sharing kitchen gear, party supplies, and outdoor equipment",
		Communities: []CommunityDef{
			{
				Name:        "Campus Crew",
				Description: "Friends from State U who share everything",
				Region:      "Boulder, CO",
				Members: []MemberDef{
					{Name: "Tyler Reed", Email: "tyler@example.com", Persona: PersonaAlfred},
					{Name: "Jordan Lee", Email: "jordan@example.com", Persona: PersonaDerek},
					{Name: "Sam Torres", Email: "sam@example.com", Persona: PersonaDerek},
					{Name: "Riley Chen", Email: "riley@example.com", Persona: PersonaGary},
					{Name: "Casey Kim", Email: "casey@example.com", Persona: PersonaGary},
					{Name: "Morgan Brooks", Email: "morgan@example.com", Persona: PersonaBetty},
					{Name: "Pat Sullivan", Email: "pat@example.com", Persona: PersonaEmma},
				},
			},
		},
		StartTime: now.Add(-3 * 30 * 24 * time.Hour), // 3 months ago.
		EndTime:   now,
	}
}

// ActiveCommunityOrg creates a large scenario with 37 members split across
// two communities (20 + 17, each under the 32-member limit) with diverse
// activity over 12 months.
func ActiveCommunityOrg() Scenario {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	return Scenario{
		Name:        "active-community-org",
		Description: "A thriving community organization with tool libraries, regular events, and mutual aid",
		Communities: []CommunityDef{
			{
				Name:        "Eastside Tool Library",
				Description: "Community tool sharing and workshop space on the east side",
				Region:      "Minneapolis, MN",
				Members: []MemberDef{
					{Name: "Anna Schmidt", Email: "anna@example.com", Persona: PersonaAlfred},
					{Name: "Ben Kowalski", Email: "ben.k@example.com", Persona: PersonaAlfred},
					{Name: "Carl Nguyen", Email: "carl@example.com", Persona: PersonaAlfred},
					{Name: "Dan O'Brien", Email: "dan@example.com", Persona: PersonaDerek},
					{Name: "Elena Ruiz", Email: "elena@example.com", Persona: PersonaDerek},
					{Name: "Frank Tanaka", Email: "frank@example.com", Persona: PersonaDerek},
					{Name: "Grace Liu", Email: "grace@example.com", Persona: PersonaDerek},
					{Name: "Hank Miller", Email: "hank@example.com", Persona: PersonaGary},
					{Name: "Irene Santos", Email: "irene@example.com", Persona: PersonaGary},
					{Name: "Jack Turner", Email: "jack@example.com", Persona: PersonaGary},
					{Name: "Kelly Pham", Email: "kelly@example.com", Persona: PersonaBetty},
					{Name: "Leo Washington", Email: "leo@example.com", Persona: PersonaBetty},
					{Name: "Mia Costa", Email: "mia@example.com", Persona: PersonaBetty},
					{Name: "Nick Harper", Email: "nick@example.com", Persona: PersonaEmma},
					{Name: "Olivia Chen", Email: "olivia@example.com", Persona: PersonaEmma},
					{Name: "Paul Evans", Email: "paul@example.com", Persona: PersonaEmma},
					{Name: "Quinn Moore", Email: "quinn@example.com", Persona: PersonaEmma},
					{Name: "Rosa Fernandez", Email: "rosa@example.com", Persona: PersonaHenry},
					{Name: "Sean Burke", Email: "sean@example.com", Persona: PersonaHenry},
					{Name: "Tara Singh", Email: "tara@example.com", Persona: PersonaHenry},
				},
			},
			{
				Name:        "Westside Mutual Aid",
				Description: "Neighbors supporting neighbors on the west side",
				Region:      "Minneapolis, MN",
				Members: []MemberDef{
					{Name: "Uma Patel", Email: "uma@example.com", Persona: PersonaAlfred},
					{Name: "Vic Larsson", Email: "vic@example.com", Persona: PersonaAlfred},
					{Name: "Wendy Cho", Email: "wendy@example.com", Persona: PersonaDerek},
					{Name: "Xavier Morales", Email: "xavier@example.com", Persona: PersonaDerek},
					{Name: "Yuki Tanaka", Email: "yuki@example.com", Persona: PersonaDerek},
					{Name: "Zara Ahmed", Email: "zara@example.com", Persona: PersonaDerek},
					{Name: "Aaron White", Email: "aaron@example.com", Persona: PersonaGary},
					{Name: "Bella Torres", Email: "bella@example.com", Persona: PersonaGary},
					{Name: "Caleb Reeves", Email: "caleb@example.com", Persona: PersonaGary},
					{Name: "Diana Nash", Email: "diana.n@example.com", Persona: PersonaBetty},
					{Name: "Ethan Park", Email: "ethan@example.com", Persona: PersonaBetty},
					{Name: "Fiona Doyle", Email: "fiona@example.com", Persona: PersonaEmma},
					{Name: "George Kim", Email: "george@example.com", Persona: PersonaEmma},
					{Name: "Hannah Cole", Email: "hannah@example.com", Persona: PersonaEmma},
					{Name: "Isaac Lin", Email: "isaac@example.com", Persona: PersonaEmma},
					{Name: "Julia Grant", Email: "julia@example.com", Persona: PersonaHenry},
					{Name: "Kyle Foster", Email: "kyle@example.com", Persona: PersonaHenry},
				},
			},
		},
		StartTime: now.Add(-12 * 30 * 24 * time.Hour), // 12 months ago.
		EndTime:   now,
	}
}

// NewCommunity creates a tiny community of 4 members just getting started
// with minimal activity over 2 weeks.
func NewCommunity() Scenario {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	return Scenario{
		Name:        "new-community",
		Description: "A brand new community just getting started",
		Communities: []CommunityDef{
			{
				Name:        "New Friends Sharing",
				Description: "We just started sharing things!",
				Region:      "Baton Rouge, LA",
				Members: []MemberDef{
					{Name: "Nora Bell", Email: "nora@example.com", Persona: PersonaAlfred},
					{Name: "Owen Hart", Email: "owen@example.com", Persona: PersonaDerek},
					{Name: "Piper Lane", Email: "piper@example.com", Persona: PersonaGary},
					{Name: "Reed Shaw", Email: "reed@example.com", Persona: PersonaEmma},
				},
			},
		},
		StartTime: now.Add(-14 * 24 * time.Hour), // 2 weeks ago.
		EndTime:   now,
	}
}

// LoadTestSmall creates a quick profiling scenario: 1 community, 10 members,
// 1 month of activity. Balanced persona mix for broad RPC coverage.
func LoadTestSmall() Scenario {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	return Scenario{
		Name:        "load-test-small",
		Description: "Quick profiling run: 1 community, 10 members, 1 month (~2 min)",
		Communities: []CommunityDef{
			{
				Name:        "LT-Small Community",
				Description: "Load test community for quick profiling",
				Region:      "Austin, TX",
				Members: []MemberDef{
					{Name: "LT Alice Monroe", Email: "lt-alice@example.com", Persona: PersonaAlfred},
					{Name: "LT Bob Fischer", Email: "lt-bob@example.com", Persona: PersonaDerek},
					{Name: "LT Carol Santos", Email: "lt-carol@example.com", Persona: PersonaDerek},
					{Name: "LT Dave Nguyen", Email: "lt-dave@example.com", Persona: PersonaGary},
					{Name: "LT Eva Park", Email: "lt-eva@example.com", Persona: PersonaGary},
					{Name: "LT Frank Wells", Email: "lt-frank@example.com", Persona: PersonaBetty},
					{Name: "LT Grace Ito", Email: "lt-grace@example.com", Persona: PersonaEmma},
					{Name: "LT Hank Davis", Email: "lt-hank@example.com", Persona: PersonaEmma},
					{Name: "LT Iris Chen", Email: "lt-iris@example.com", Persona: PersonaHenry},
					{Name: "LT Jake Ross", Email: "lt-jake@example.com", Persona: PersonaHenry},
				},
			},
		},
		StartTime: now.Add(-30 * 24 * time.Hour), // 1 month ago.
		EndTime:   now,
	}
}

// LoadTestMedium creates a medium profiling scenario: 2 communities, 30 members
// total, 3 months of activity. Tests cross-community queries and moderate load.
func LoadTestMedium() Scenario {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	return Scenario{
		Name:        "load-test-medium",
		Description: "Medium load: 2 communities, 30 members, 3 months (~10 min)",
		Communities: []CommunityDef{
			{
				Name:        "LT-Medium Alpha",
				Description: "Load test community alpha for medium profiling",
				Region:      "Austin, TX",
				Members: []MemberDef{
					{Name: "LTM Ada Lane", Email: "ltm-ada@example.com", Persona: PersonaAlfred},
					{Name: "LTM Ben Cruz", Email: "ltm-ben@example.com", Persona: PersonaAlfred},
					{Name: "LTM Cora Blake", Email: "ltm-cora@example.com", Persona: PersonaDerek},
					{Name: "LTM Dean Hart", Email: "ltm-dean@example.com", Persona: PersonaDerek},
					{Name: "LTM Ella Voss", Email: "ltm-ella@example.com", Persona: PersonaDerek},
					{Name: "LTM Finn Cole", Email: "ltm-finn@example.com", Persona: PersonaGary},
					{Name: "LTM Gina Ruiz", Email: "ltm-gina@example.com", Persona: PersonaGary},
					{Name: "LTM Hugo Pham", Email: "ltm-hugo@example.com", Persona: PersonaGary},
					{Name: "LTM Ivy Nash", Email: "ltm-ivy@example.com", Persona: PersonaBetty},
					{Name: "LTM Joel Park", Email: "ltm-joel@example.com", Persona: PersonaBetty},
					{Name: "LTM Kate Sun", Email: "ltm-kate@example.com", Persona: PersonaEmma},
					{Name: "LTM Liam Fox", Email: "ltm-liam@example.com", Persona: PersonaEmma},
					{Name: "LTM Maya Rios", Email: "ltm-maya@example.com", Persona: PersonaEmma},
					{Name: "LTM Noah Bell", Email: "ltm-noah@example.com", Persona: PersonaHenry},
					{Name: "LTM Opal King", Email: "ltm-opal@example.com", Persona: PersonaHenry},
				},
			},
			{
				Name:        "LT-Medium Beta",
				Description: "Load test community beta for medium profiling",
				Region:      "Boulder, CO",
				Members: []MemberDef{
					{Name: "LTM Pete Shaw", Email: "ltm-pete@example.com", Persona: PersonaAlfred},
					{Name: "LTM Quinn Ray", Email: "ltm-quinn@example.com", Persona: PersonaAlfred},
					{Name: "LTM Rose Webb", Email: "ltm-rose@example.com", Persona: PersonaDerek},
					{Name: "LTM Sean Troy", Email: "ltm-sean@example.com", Persona: PersonaDerek},
					{Name: "LTM Tina Yoon", Email: "ltm-tina@example.com", Persona: PersonaDerek},
					{Name: "LTM Umar Gale", Email: "ltm-umar@example.com", Persona: PersonaGary},
					{Name: "LTM Vera Knox", Email: "ltm-vera@example.com", Persona: PersonaGary},
					{Name: "LTM Walt Judd", Email: "ltm-walt@example.com", Persona: PersonaGary},
					{Name: "LTM Xena Hill", Email: "ltm-xena@example.com", Persona: PersonaBetty},
					{Name: "LTM Yuri Long", Email: "ltm-yuri@example.com", Persona: PersonaBetty},
					{Name: "LTM Zane Cook", Email: "ltm-zane@example.com", Persona: PersonaEmma},
					{Name: "LTM Anya Gist", Email: "ltm-anya@example.com", Persona: PersonaEmma},
					{Name: "LTM Bree Holt", Email: "ltm-bree@example.com", Persona: PersonaEmma},
					{Name: "LTM Cody Fisk", Email: "ltm-cody@example.com", Persona: PersonaHenry},
					{Name: "LTM Drew Lark", Email: "ltm-drew@example.com", Persona: PersonaHenry},
				},
			},
		},
		StartTime: now.Add(-3 * 30 * 24 * time.Hour), // 3 months ago.
		EndTime:   now,
	}
}

// LoadTestLarge creates a heavy profiling scenario: 3 communities, 75 members
// total, 6 months of activity. Stress tests connection pool and search indexes.
func LoadTestLarge() Scenario {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	return Scenario{
		Name:        "load-test-large",
		Description: "Heavy load: 3 communities, 75 members, 6 months (~30 min)",
		Communities: []CommunityDef{
			{
				Name:        "LT-Large Alpha",
				Description: "Load test large community alpha",
				Region:      "Austin, TX",
				Members:     generateMembers("ltl-a", 25),
			},
			{
				Name:        "LT-Large Beta",
				Description: "Load test large community beta",
				Region:      "Minneapolis, MN",
				Members:     generateMembers("ltl-b", 25),
			},
			{
				Name:        "LT-Large Gamma",
				Description: "Load test large community gamma",
				Region:      "Boulder, CO",
				Members:     generateMembers("ltl-c", 25),
			},
		},
		StartTime: now.Add(-6 * 30 * 24 * time.Hour), // 6 months ago.
		EndTime:   now,
	}
}

// ReadHeavy creates a read-heavy profiling scenario: 1 community, 20 members,
// 1 month. Designed to be run with --read-multiplier=5 to isolate read path
// performance with minimal write noise.
func ReadHeavy() Scenario {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	return Scenario{
		Name:        "read-heavy",
		Description: "Read-heavy: 1 community, 20 members, 1 month (use with --read-multiplier=5)",
		Communities: []CommunityDef{
			{
				Name:        "LT-Read Heavy",
				Description: "Load test community for read-heavy profiling",
				Region:      "Austin, TX",
				Members:     generateMembers("ltr", 20),
			},
		},
		StartTime: now.Add(-30 * 24 * time.Hour), // 1 month ago.
		EndTime:   now,
	}
}

// generateMembers creates n members with a balanced persona distribution.
// The prefix distinguishes emails across scenarios (e.g., "ltl-a" for large-alpha).
func generateMembers(prefix string, n int) []MemberDef {
	personas := []PersonaType{
		PersonaAlfred, PersonaDerek, PersonaGary,
		PersonaBetty, PersonaEmma, PersonaHenry,
	}
	firstNames := []string{
		"Alex", "Blake", "Casey", "Dana", "Eden", "Fern",
		"Glen", "Hope", "Ivan", "Jade", "Kent", "Luna",
		"Max", "Nina", "Owen", "Pia", "Quinn", "Rio",
		"Sky", "Tess", "Uri", "Val", "Wren", "Xia",
		"York",
	}
	lastNames := []string{
		"Stone", "Grant", "Wells", "Cross", "Blake", "Swift",
		"Chase", "Drake", "Frost", "Glenn", "Hayes", "Joyce",
		"Keane", "Locke", "Marsh", "Noble", "Price", "Quinn",
		"Reyes", "Sharp", "Trace", "Urban", "Vance", "Wolfe",
		"Young",
	}

	members := make([]MemberDef, n)
	for i := range n {
		fIdx := i % len(firstNames)
		lIdx := i % len(lastNames)
		members[i] = MemberDef{
			Name:    firstNames[fIdx] + " " + lastNames[lIdx],
			Email:   fmt.Sprintf("%s-%d@example.com", prefix, i),
			Persona: personas[i%len(personas)],
		}
	}
	return members
}

// AllScenarios returns all built-in scenarios.
func AllScenarios() []Scenario {
	return []Scenario{
		SuburbanNeighborhood(),
		CollegeFriends(),
		ActiveCommunityOrg(),
		NewCommunity(),
		LoadTestSmall(),
		LoadTestMedium(),
		LoadTestLarge(),
		ReadHeavy(),
	}
}
