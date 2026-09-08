// prompts.go contains the content-generation (Gen*) prompt templates shared
// across providers. Sibling files hold the rest: prompts_summaries.go
// (conversation/experience summaries), prompts_story.go (celebratory
// stories), prompts_unified.go (unified-create classifier),
// prompts_hero_card.go, and prompts_impact_row.go.
//
// Editing guidance — prefer the most token-efficient phrasing:
//   - Every added line increases TTFT and per-call cost on every Gen* request.
//     Slow providers (e.g. gpt-5-mini) are disproportionately affected: an
//     earlier draft of the Request value_estimate section with 4 examples per
//     branch added ~200 input tokens and slowed gpt-5-mini TTFT by 25%
//     (~3s per request). The trimmed form kept the classify-and-branch intent
//     at ~half the tokens with no measurable quality regression on the
//     benchmark-tagged eval suite (server/ai/eval/prompt_test.go).
//   - Prefer 2 examples over 4+ when examples are the steering mechanism.
//     Prose that restates what an example already shows is waste.
//   - Do NOT include examples whose inputs or outputs overlap with any
//     golden case in server/ai/eval/testdata/ — that is handing the model
//     the answers. Pick semantically-similar but disjoint items/services.
//   - When adding a rule/branch, measure net token add before merging by
//     diffing input_tokens across a sample eval log; flag >10% growth in
//     the commit message.
//   - Trim in the same diff that adds — "we'll tighten later" leaves
//     verbose prompts in production.
package ai

import (
	"fmt"
	"time"
)

// withWeekday parses an RFC 3339 timestamp and returns "<orig> (<Weekday>)" so
// the model gets the day-of-week as an explicit grounding token rather than
// having to derive it from the date itself. Smaller models drift +1 day on
// relative-date prompts ("this Saturday", "last Thursday") when they have to
// compute weekday from raw ISO; appending it removes that calendar-lookup
// step. Returns the input unchanged if it does not parse — callers shouldn't
// see a behavior change for malformed timestamps.
func withWeekday(currentTime string) string {
	if currentTime == "" {
		return currentTime
	}
	t, err := time.Parse(time.RFC3339, currentTime)
	if err != nil {
		return currentTime
	}
	return fmt.Sprintf("%s (%s)", currentTime, t.Weekday())
}

// toneAndStyle contains the standard tone & style guidance used across all content generation prompts.
const toneAndStyle = `TONE & STYLE:
- Sound like a friend talking to friends, not a marketer selling an item
- Keep it casual and conversational, like a text message
- Do not be overly formal - avoid parentheses, colons, and semi-colons
- Provide enough detail to be helpful, but not overly verbose`

// Time-confidence values a Gen* provider returns in the time_confidence
// field, mapping to api.TimeConfidence. Explicit means the user stated a time
// outright; inferred means the model derived one; unknown means neither.
const (
	TimeConfidenceExplicit = "EXPLICIT"
	TimeConfidenceInferred = "INFERRED"
	TimeConfidenceUnknown  = "UNKNOWN"
)

// LocationQueryUserPrimary is the sentinel a Gen* provider returns in
// location_query when the user referred to their own place ("my house",
// "home") rather than a geocodable location. Callers must resolve it against
// the user's primary location instead of sending it to the geocoder — see
// locationExtractionRules below, which is what instructs the model to emit it.
const LocationQueryUserPrimary = "USER_PRIMARY_LOCATION"

// locationExtractionRules contains shared location extraction logic across all content types.
const locationExtractionRules = `LOCATION EXTRACTION:
- Extract location mentions from the user's input
- Return the location query string exactly as mentioned (for server-side geocoding)
- Recognize contextual references:
  * "my place", "my house", "my apartment", "home" → return "USER_PRIMARY_LOCATION"
  * "your place", "your house" → return "your house"
  * Named locations → return as-is ("North Boulder Park", "The Cup", "Central Library")
- If no location mentioned, return empty string
- CRITICAL: Don't invent locations - only extract what's actually mentioned
- Examples:
  * "drill at my workshop" → location_query: "my workshop"
  * "camping gear at home" → location_query: "USER_PRIMARY_LOCATION"
  * "sewing kit for a school costume" → location_query: "" (no location mentioned)
  * "bike repair at Central Library" → location_query: "Central Library"`

// buildGearDetectionPrompt creates the prompt for gear detection with
// automatic content type detection. Emphasizes per-field separation
// (title is the human-meaningful name; brand and model designation go
// in their own fields) after a 2026-05-09 A/B run on the small-model
// tier showed 4–5 point pass-rate gains and a 12–25 point gain
// specifically on model_contains, by curing a v1 anti-pattern where
// vision was packing brand + model into the title field. ~35% shorter
// than the prior version. See docs/issues/1777-baseline-2026-05-09.md
// for context.
func buildGearDetectionPrompt() string {
	return `You analyze product images for a community sharing platform. Identify the SINGLE most prominent item and return structured info about it.

` + toneAndStyle + `

## Content type

Classify as one of:
- GEAR — tools, sports/outdoor, electronics, home/garden, recreation, books/media, vehicles
- CLOTHES — outerwear, dresses, shoes, accessories
- FOOD — packaged goods, fresh produce, prepared meals

## Output fields

Each field has a specific purpose. Don't pack the same info into the wrong field.

**title** (~20 chars, max 30): A human-meaningful name. Always include the product noun ("drill", "tent", "stand mixer"). It's OK to include brand or model line if it makes the title more recognizable ("DeWalt 20V Drill", "Barista Express Espresso"). But the brand and model fields below ALSO need to be populated separately — title isn't a substitute.

**description** (~80 chars, max 160): One crisp sentence naming the item and its primary use ("Compact cordless drill with high-performance motor and two-battery kit."). No photo narration ("has a black handle and red trigger") — that's already visible in the image. No marketing language ("perfect for weekend projects"). No paraphrasing of structured fields below — title, brand, model, weight, and category cover those. Exactly one sentence ending in a period; if you can't say it in one, stop after the first.

**category**: For GEAR, the specific category ("Power Tools", "Camping", "Electronics"). Always "Clothing" for CLOTHES, "Food" for FOOD.

**brand**: Manufacturer/brand as printed or recognized. Empty string if not visible. Examples: "DeWalt", "REI", "KitchenAid", "The North Face".

**model**: The product DESIGNATION — SKU code, marketing name, model line, or series. Vision typically sees this on the housing, topsheet, control face, side panel, or identification badge. Populate this WHENEVER you can see the designation, even if it's not a numeric SKU. Empty string only if no designation is visible at all.
- SKU style: "DCD771C2", "MS 250", "HC-2350"
- Marketing-name style: "Barista Express", "Force 3 XXL", "Builder Series", "Atmos AG 65"
- Series style: "M18 FUEL", "POWER+", "PWRCORE 12"

**material_category**: One of solid_metal, solid_plastic, mixed_plastic_metal, mixed_wood_metal, mixed_wood_plastic, wood, aluminum, fabric, cordless_power_tool, corded_power_tool, petrol_tool, electronics_small. Pick the closest match by primary material or tool type:
- Power tool with battery → cordless_power_tool
- Power tool with cord → corded_power_tool
- Gas-powered → petrol_tool
- Phones, speakers, headphones, projectors, e-readers → electronics_small
- Hand tool with wooden handle/tote → mixed_wood_metal
- Tool with plastic handle and metal head → mixed_plastic_metal
- Tents, sleeping bags, soft goods → fabric
- Mostly aluminum (ladders, cookware) → aluminum

**weight_grams**: Estimated weight in grams. Use visible labels, known model specs, or reasonable estimates. Examples: cordless drill ~1500g, camping tent ~3000g, headphones ~250g. Use 0 only if you truly can't estimate.

**confidence** (0.0–1.0): Identification certainty. Higher if brand/model clearly visible.

**value_estimate** (REQUIRED for GEAR and CLOTHES): an object with
- estimated_value_usd (number, USD, never 0): based on brand, model, condition. For used items, 40–70% of retail.
- confidence (0.0–1.0)
- reasoning (≤10 words)

**content_type**: "GEAR" | "CLOTHES" | "FOOD".
**content_type_confidence** (0.0–1.0).

## Anti-patterns
- Don't leave model empty when the designation is visible — even marketing names ("Barista Express") count.
- Don't use placeholders ("Unknown", "<UNKNOWN>", "N/A", "Generic", "Not visible"). Use empty string.
- Don't invent specs you can't observe.

Return a JSON object with a single "gear_item" field containing the primary item (or null if no shareable item visible). The item must include: title, description, category, brand, model, material_category, weight_grams, confidence, value_estimate (with estimated_value_usd, confidence, reasoning), content_type, and content_type_confidence.`
}

// buildGearGenerationPrompt creates the prompt for gear generation from text.
//
// Example inputs:
//
//	"camping tent"           → "4-Person Camping Tent", category: "Camping & Outdoors"
//	"DeWalt 20V drill"       → "DeWalt 20V Cordless Drill", brand: "DeWalt"
//	"tent at my garage"      → "Camping Tent", location_query: "my garage"
func buildGearGenerationPrompt(userPrompt, region string) string {
	regionContext := ""
	if region != "" {
		regionContext = fmt.Sprintf(" in the %s area", region)
	}

	return fmt.Sprintf(`You extract structured metadata about an item a user wants to share with their community%s.

USER INPUT: "%s"

`+toneAndStyle+`

FIELDS:

1. TITLE (target ~20 characters, max 30): clear, specific item name; include brand and model when mentioned ("DeWalt 20V Drill", "Coleman 4-Person Tent", "Trek Road Bike").

2. CATEGORY: specific but common and recognizable ("Power Tools", "Camping & Outdoors", "Bicycles & Cycling", "Lawn & Garden", "Electronics", "Sports Equipment").

3. BRAND: manufacturer name ONLY if explicitly mentioned, else "". Don't invent or assume; never use placeholders ("Unknown", "<UNKNOWN>", "N/A", "Generic").

4. MATERIAL_CATEGORY: one of solid_metal, solid_plastic, mixed_plastic_metal, mixed_wood_metal, mixed_wood_plastic, wood, aluminum, fabric, cordless_power_tool, corded_power_tool, petrol_tool, electronics_small. Pick by PRIMARY material or tool type:
   - Power tool with battery → cordless_power_tool; with cord → corded_power_tool; gas-powered → petrol_tool
   - Phones, tablets, speakers → electronics_small
   - Hand tool with wooden handle → mixed_wood_metal; plastic handle with metal head → mixed_plastic_metal
   - Tents, bags, clothing → fabric; ladders, cookware → aluminum
   - Otherwise the dominant material (solid_metal, solid_plastic, wood, ...). Empty if truly uncertain.

5. WEIGHT_GRAMS: estimated weight from known specs or item type (cordless drill ~2000, camping tent ~3000, bicycle ~12000, book ~400). 0 only if no reasonable estimate.

6. VALUE_ESTIMATE (REQUIRED):
   - estimated_value_usd: typical retail price for the named brand/model, else category average. NEVER 0 - if uncertain, estimate with low confidence instead.
   - confidence: 0.8-1.0 known brand/model pricing; 0.5-0.8 recognizable type; 0.3-0.5 generic item; 0.1-0.3 very uncertain.
   - reasoning: ≤10 words ("DeWalt 20V drills retail $80-120").

7. `+locationExtractionRules+`
   - LOCATION_QUERY: the extracted location string, or "" if none mentioned.

8. CONFIDENCE (0.0 to 1.0): high (0.8+) for specific brand/model detail, medium (0.5-0.8) for clear but generic items, low for vague prompts.`,
		regionContext, userPrompt)
}

// buildCommunityGenerationPrompt creates the prompt for community image-keyword
// generation. Community generation produces ONLY image-search keywords — the
// terms used to find a good background image for the community the user's text
// describes. There is no title, description, confidence, or reasoning.
func buildCommunityGenerationPrompt(userPrompt, region string) string {
	regionContext := ""
	if region != "" {
		regionContext = fmt.Sprintf(" in the %s area", region)
	}

	return fmt.Sprintf(`You pick a background image for a new community on a local sharing platform. A user is starting a community%s and described it as:
"%s"

Produce SEARCH_KEYWORDS (3-5 terms) for finding a fitting background image in a stock-photo library:
- Focus on visual concepts that represent the community's purpose
- Use specific, visual nouns that yield relevant photos
- Examples for a woodworking community: ["woodworking", "workshop", "sawdust"]
- Examples for a pottery studio: ["pottery", "ceramics", "clay"]
- Skip personal names, dates, and abstract terms stock libraries don't index

Return a JSON object matching the expected schema with a single search_keywords array.`,
		regionContext, userPrompt)
}

// buildRequestGenerationPrompt creates the prompt for request content generation.
func buildRequestGenerationPrompt(userPrompt, region string) string {
	regionContext := ""
	if region != "" {
		regionContext = fmt.Sprintf(" in the %s area", region)
	}

	return fmt.Sprintf(`You help someone ask their community%s for help. Extract structured metadata from their request.

USER INPUT: "%s"

`+toneAndStyle+`

FIELDS:

1. TITLE (target ~30 characters, max 50): specific summary of the help needed - never vague like "Need Help" ("Power Drill for Weekend Project", "Help Moving Furniture Saturday", "Advice on Deck Repair").

2. SEARCH_KEYWORDS (3-5 visual terms for a stock-image query):
   - MUST include the primary item or service the user is asking about (a chainsaw request must include "chainsaw"). Essential for discoverability.
   - Plus 2-3 context terms: activity, setting, or category. Phrases for specific concepts ("power tool"), single words for broad categories ("workshop", "cleaning").
   - Generic visual terms only - never personal names, dates/times, or venue/city/business names; stock libraries index subject matter, not proper nouns.

3. `+locationExtractionRules+`
   - LOCATION_QUERY: the extracted location string, or "" if none mentioned.

4. VALUE_ESTIMATE (REQUIRED). Classify first: SERVICE (labor/expertise) or ITEM BORROW (use someone's physical item).
   - SERVICE: hire-cost for the session at local%s rates (dog grooming $50-100/session; house cleaning visit $120-200).
   - ITEM BORROW: one-session rental-equivalent fee, NOT retail replacement value (kayak for a day $30-60; sewing machine for a weekend $15-30).
   - estimated_value_usd: NEVER 0 - if uncertain, estimate with low confidence instead.
   - confidence: 0.8-1.0 specific comparable known; 0.5-0.8 recognizable category; 0.3-0.5 complexity-based estimate; 0.1-0.3 rough guess.
   - reasoning: ≤10 words ("Tutoring ~$40-60/hr Bay Area").

5. CONFIDENCE (0.0 to 1.0): high (0.8+) for clear, actionable requests; medium for understandable but vague; low for unclear ones.`,
		regionContext, userPrompt, regionContext)
}

// buildExperienceFromTextPrompt creates the prompt for generating experiences from text.
func buildExperienceFromTextPrompt(userPrompt, region, currentTime string) string {
	currentTime = withWeekday(currentTime)
	return fmt.Sprintf(`You're helping someone document or plan a community activity. Extract structured metadata from their input.

USER INPUT: "%s"
USER REGION: "%s"
CURRENT TIME: %s

`+toneAndStyle+`

FIELDS:

1. TITLE (max 60 chars): casual and natural, not an event announcement ("Coffee catch-up", "Morning run").
   - Extracted date in the PAST relative to CURRENT TIME → retrospective past tense ("We hiked the ridge trail"). Future or unknown date → forward-looking, inviting ("Let's go hiking").
   - Include a location or time ONLY if the user explicitly mentioned it - never inferred times, never invented locations ("Hiking meetup", not "Hiking at Park Saturday").

2. DATE (YYYY-MM-DD or ""), TIME (HH:MM 24-hour or ""), TIME_CONFIDENCE:
   - Resolve relative dates against CURRENT TIME in its timezone offset, unless the user names another timezone ("3pm EST"): "tomorrow" → next day; "this weekend" → upcoming Saturday; "next week" → +7 days; "tonight" → same day evening.
   - Fuzzy times: morning/breakfast 08:00, brunch 10:30, lunch 12:00, afternoon 14:00, evening 18:00, dinner 18:30, night 20:00.
   - TIME_CONFIDENCE: EXPLICIT (specific time given: "3pm", "2:30", "noon"), INFERRED (fuzzy time: "morning", "evening"), UNKNOWN (no time mentioned).

3. `+locationExtractionRules+`
   - LOCATION_QUERY: the extracted location string, or "" if none mentioned.

4. SEARCH_KEYWORDS (3-5 visual terms for a stock-image query):
   - MUST include the primary activity (a hiking plan must include "hiking"). Essential for discoverability.
   - Plus 2-3 broader terms: setting, mood, or category. Phrases name concepts ("book club", "stand-up comedy"); single words for broad categories ("fitness", "outdoors").
   - Generic visual terms only - never personal names, dates/times, or venue/city/business names; stock libraries index subject matter, not proper nouns ("pickleball, paddle, court" - not "pickleball, 3rd Shot, Longmont").

5. MENTIONED_NAMES: real person names the user explicitly mentions as participants ("picnic with Dario and Mei" → ["Dario", "Mei"]). Generic words ("friends", "everyone", "the crew") → [].

6. VALUE_ESTIMATE (REQUIRED): PER-PERSON value in USD. If the user states a price, fee, or ticket cost, use that amount. Otherwise estimate the cost to hire someone to host a similar experience at USER REGION market rates (guided outdoor activity $50-100; cooking class $75-150; game night $20-40; fitness class $20-40).
   - estimated_value_usd: NEVER 0 - if uncertain, estimate with low confidence instead.
   - confidence: 0.8-1.0 comparable pricing known; 0.5-0.8 known type; 0.3-0.5 general estimate; 0.1-0.3 rough guess.
   - reasoning: ≤10 words ("Guided hikes ~$50-80/person locally").

7. CONFIDENCE (0.0 to 1.0): how well you understood what they want to do; lower for vague prompts.

EXAMPLES:

Input: "pottery class thursday 6pm at Clay Studio"
Current Time: 2026-02-22T10:00:00-07:00 (Sunday)
Output:
Title: "Pottery class"
Date: 2026-02-26
Time: 18:00
Time Confidence: EXPLICIT
Location Query: "Clay Studio"
Mentioned Names: []
Search Keywords: ["pottery", "ceramics", "art class"]
Confidence: 0.9

Input: "trivia sometime"
Current Time: 2026-02-22T10:00:00-07:00 (Sunday)
Output:
Title: "Trivia night"
Date: (not set)
Time: (not set)
Time Confidence: UNKNOWN
Location Query: (not set)
Mentioned Names: []
Search Keywords: ["trivia", "quiz night", "games"]
Confidence: 0.6`, userPrompt, region, currentTime)
}

// buildExperienceFromImagePrompt creates the prompt for analyzing images and creating experiences.
func buildExperienceFromImagePrompt(region, notes, currentTime string) string {
	currentTime = withWeekday(currentTime)
	prompt := fmt.Sprintf(`You're helping someone create an invitation from an image. First decide what the image is, then extract structured metadata.

USER REGION: "%s"
CURRENT TIME: %s`, region, currentTime)

	if notes != "" {
		prompt += fmt.Sprintf(`
USER NOTES: "%s"`, notes)
	}

	prompt += `

IMAGE KINDS:
A. FLYER/POSTER - the image itself advertises an event in text. Extract the details printed on it; add nothing it doesn't show.
B. SCENE/PHOTO - no event text. Infer the activity to invite people to: food or table scene → meal together; court, field, or sports gear → pick-up game; trail, park, or camp gear → outdoor outing; party or game setup → get-together; unclear → generic hangout. Suggest the activity - NEVER invent a specific time or place for a scene.

` + toneAndStyle + `

FIELDS:

1. TITLE (max 60 chars): casual and natural. Flyers: based on what the flyer says. Scenes: the suggested activity ("Pick-up basketball", "Dinner party").

2. DESCRIPTION (max 400 chars): flyers: the key readable details - what it is, what people will do, anything notable to bring or know. Scenes: a warm, open-ended invitation ("Anyone up for some basketball? We can figure out when and where.").

3. DATE (YYYY-MM-DD or ""), TIME (HH:MM 24-hour or ""), TIME_CONFIDENCE:
   - Extract only dates and times readable on the image, in the user's timezone (from CURRENT TIME) unless the flyer names one.
   - A date printed without a year resolves to its next occurrence after CURRENT TIME; a date printed WITH a year stays as printed, even if past. Recurring phrasing ("Every Saturday 9am") → the next occurrence.
   - No real date or time → "" - NEVER placeholders like "<UNKNOWN>" or "TBD".
   - TIME_CONFIDENCE: EXPLICIT (clear on the image), INFERRED (partial, e.g. "Saturday mornings"), UNKNOWN (nothing readable - nearly always for scenes).

4. LOCATION_QUERY: venue or address readable on a flyer, or a clearly recognizable landmark or signage in a scene; otherwise "". NEVER copy USER REGION into location_query - it describes the reader, not the event.

5. SEARCH_KEYWORDS (3-5 visual terms for a stock-image query): the primary activity first, plus setting/mood/category terms. Generic visual terms only - never personal names or venue/city/business names.

6. VALUE_ESTIMATE (REQUIRED): PER-PERSON value in USD. If the image states a price or ticket cost, use that amount. Otherwise estimate the cost to hire someone to host a similar experience at local rates.
   - estimated_value_usd: NEVER 0 - if uncertain, estimate with low confidence instead.
   - confidence: 0.8-1.0 stated price or comparable known; 0.5-0.8 known type; 0.3-0.5 general estimate; 0.1-0.3 rough guess.
   - reasoning: ≤10 words.

7. CONFIDENCE (0.0 to 1.0): high for clear flyers, moderate for readable scenes, low when the image is unclear, shows no shareable activity, or is a commercial ad with no community angle - then leave the other fields empty rather than inventing them.

EXAMPLES:

Flyer reads: "Repair Cafe - Westside Tool Library - Saturday April 18 - 10 AM to 1 PM - Free"
Current Time: 2026-03-01T10:00:00-07:00 (Sunday)
Output:
Title: "Repair Cafe at the tool library"
Description: "Free repair cafe at Westside Tool Library - bring broken lamps, bikes, or clothes and fix them with volunteer helpers."
Date: 2026-04-18 | Time: 10:00 | Time Confidence: EXPLICIT
Location Query: "Westside Tool Library"
Search Keywords: ["repair", "tools", "workshop"]
Confidence: 0.9

Photo shows: a picnic blanket and basket in a park, no text
Output:
Title: "Picnic in the park"
Description: "Thinking a picnic would be fun - I'll bring the blanket, someone bring snacks. Who's in?"
Date: "" | Time: "" | Time Confidence: UNKNOWN
Location Query: ""
Search Keywords: ["picnic", "park", "outdoors"]
Confidence: 0.7`

	return prompt
}

func buildRequestImageAnalysisPrompt(region string) string {
	regionContext := ""
	locationGuidance := "Keep the tone warm and neighborly"
	if region != "" {
		regionContext = fmt.Sprintf(" in the %s area", region)
		locationGuidance = fmt.Sprintf("You can naturally mention %s if it adds helpful context", region)
	}

	return fmt.Sprintf(`You are helping someone ask their community for help or to borrow something%s.

Analyze the image and write a friendly, personal request as if the person themselves is asking their neighbors for help.

`+toneAndStyle+`
- Write in first person ("I need...", "I'm looking for...")
- Sound warm, friendly, and appreciative
- Show gratitude in advance ("Would really appreciate it!", "Thanks in advance!")
- %s

Generate:
1. TITLE (target ~30 characters, max 50):
   - What you need, stated simply and directly
   - Examples: "Need a Ladder This Weekend", "Looking for Camping Gear", "Anyone Have a Power Drill?"

2. DESCRIPTION (target ~200 characters, max 400):
   - Write 2-3 sentences as the person asking
   - Explain what you see in the image and what help is needed
   - Include helpful context (when, why, how long)
   - End warmly
   - Examples:
     * "I'm building some shelves and realized I don't have a drill! Would anyone be willing to lend me a power drill for the weekend? Happy to return it Sunday evening. Thanks so much!"
     * "Going camping next month and looking to borrow a tent and sleeping bags if anyone has extras. We're a family of four. Would take great care of everything!"

3. SEARCH_KEYWORDS (3-5 keywords):
   - Keywords for finding a relevant stock photo
   - Focus on visual concepts related to the need
   - Examples: ["tools", "drill", "workshop"], ["camping", "tent", "outdoors"], ["moving", "boxes", "furniture"]
   - Use specific, visual nouns that represent the help needed

4. CONFIDENCE (0.0 to 1.0):
   - High (0.8-1.0): Image clearly shows a need or request
   - Medium (0.5-0.8): Image suggests a need but is ambiguous
   - Low (0.0-0.5): Unclear what help is needed from the image

5. REASONING:
   - 1-2 sentences explaining what you see in the image and why you interpreted the need this way

6. VALUE_ESTIMATE (REQUIRED):
   ALWAYS estimate the value of this help request.
   Consider: What would it cost to hire someone for this type of help?
   Factor in the user's location%s for local labor rates.

   a. estimated_value_usd (number, REQUIRED):
      - Value in US dollars (e.g., 75.0 for $75.00)
      - Consider local labor market rates, service type, and complexity
      - NEVER return 0 - always provide your best estimate

   b. confidence (float, 0.0 to 1.0, REQUIRED):
      - 0.8-1.0: Specific comparable service found
      - 0.5-0.8: Recognizable service category
      - 0.3-0.5: General estimate based on complexity
      - 0.1-0.3: Very uncertain, rough guess

   c. reasoning (string, REQUIRED, ≤ 10 words):
      - Very brief rationale referencing local rates or comparable services

Return a JSON object matching the expected schema.`, regionContext, locationGuidance, regionContext)
}

// buildInformalToCalendarPrompt creates the prompt for converting informal time descriptions to structured dates/times.
func buildInformalToCalendarPrompt(informalDescription, currentTime, timezone string) string {
	currentTime = withWeekday(currentTime)
	return fmt.Sprintf(`You are a time parsing assistant helping convert natural language time descriptions into structured date/time information.

USER INPUT: "%s"
CURRENT TIME: %s
TIMEZONE: %s

Your task is to extract structured date/time information from the informal description.

TIME PARSING RULES:
1. RELATIVE DATE REFERENCES:
   - "tomorrow" → next day from current time
   - "today" → current day
   - "tonight" → current day evening
   - "this weekend" → upcoming Saturday-Sunday
   - "next week" → 7 days from now
   - "next weekend" → Saturday-Sunday of next week
   - "next [weekday]" → next occurrence of that weekday
   - "this [weekday]" → upcoming occurrence this week
   - "in X days/hours" → calculate from current time

2. TIME OF DAY INFERENCE:
   - "morning" → 08:00
   - "afternoon" → 14:00
   - "evening" → 18:00
   - "night" → 20:00
   - "lunch" → 12:00
   - "breakfast" → 08:00
   - "dinner" → 18:30
   - "brunch" → 10:30

3. EXPLICIT TIME PARSING:
   - "3pm", "3:00pm", "15:00" → parse as exact time
   - "noon", "midnight" → 12:00, 00:00
   - "9am", "9:30am" → parse as morning time

4. DURATION EXTRACTION (optional):
   - "for 2 hours" → duration_minutes: 120
   - "1 hour" → duration_minutes: 60
   - "30 minutes" → duration_minutes: 30
   - If no duration mentioned → duration_minutes: 0

5. TIME RANGES:
   - "Saturday-Sunday" → create range with start and end dates
   - "this weekend" → Saturday-Sunday range
   - "next week" → Monday-Friday range
   - Single date/time → create specific time, not range

6. CONFIDENCE LEVELS:
   - EXPLICIT: User gave specific time ("3pm", "2:30", "noon", "December 5 at 6pm")
   - INFERRED: User gave fuzzy time ("morning", "afternoon", "evening", "tomorrow morning")
   - UNKNOWN: No clear time information ("sometime", "TBD", "later")

RESPONSE FORMAT:
Return JSON with the following structure:

For SPECIFIC TIME (single date/time):
{
  "time_type": "specific",
  "unix_timestamp_sec": <Unix timestamp in seconds>,
  "timezone": "%s",
  "duration_minutes": <duration in minutes, 0 if not specified>,
  "confidence": "EXPLICIT" | "INFERRED" | "UNKNOWN"
}

For TIME RANGE (multi-day or date range):
{
  "time_type": "range",
  "start_unix_sec": <start Unix timestamp>,
  "end_unix_sec": <end Unix timestamp>,
  "duration_minutes": <duration in minutes, 0 if not specified>,
  "description": "<informal description like 'this weekend'>",
  "confidence": "EXPLICIT" | "INFERRED" | "UNKNOWN"
}

For TBD/UNKNOWN:
{
  "time_type": "tbd",
  "confidence": "UNKNOWN"
}

EXAMPLES:

Input: "tomorrow afternoon"
Current: 2025-11-30T15:00:00-07:00
Output: {
  "time_type": "specific",
  "unix_timestamp_sec": 1733090400,
  "timezone": "America/Denver",
  "duration_minutes": 0,
  "confidence": "INFERRED"
}

Input: "next Friday at 6pm"
Current: 2025-11-30T15:00:00-07:00
Output: {
  "time_type": "specific",
  "unix_timestamp_sec": 1733533200,
  "timezone": "America/Denver",
  "duration_minutes": 0,
  "confidence": "EXPLICIT"
}

Input: "this weekend"
Current: 2025-11-30T15:00:00-07:00
Output: {
  "time_type": "range",
  "start_unix_sec": 1733025600,
  "end_unix_sec": 1733198400,
  "duration_minutes": 0,
  "description": "this weekend",
  "confidence": "INFERRED"
}

Input: "coffee for 1 hour tomorrow at 9am"
Current: 2025-11-30T15:00:00-07:00
Output: {
  "time_type": "specific",
  "unix_timestamp_sec": 1733065200,
  "timezone": "America/Denver",
  "duration_minutes": 60,
  "confidence": "EXPLICIT"
}

IMPORTANT:
- Always return valid Unix timestamps (seconds since epoch)
- Use the provided timezone for all calculations
- Be generous with inferred times - it's better to return a guess than TBD
- Only return TBD if absolutely no time information can be extracted

Return only the JSON object, no additional text.`, informalDescription, currentTime, timezone, timezone)
}

// buildExperienceFromWebpagePrompt creates the prompt for extracting experience content from webpage text.
func buildExperienceFromWebpagePrompt(pageTitle, pageDescription, pageBody, region, currentTime string) string {
	currentTime = withWeekday(currentTime)
	return fmt.Sprintf(`You're helping someone create an invitation from an event webpage they found. Extract the event's details into structured metadata.

PAGE TITLE: "%s"
PAGE DESCRIPTION: "%s"
PAGE CONTENT:
---
%s
---

USER REGION: "%s"
CURRENT TIME: %s

`+toneAndStyle+`

FIELDS:

1. TITLE (max 60 chars): the event's actual name from the page, phrased casually - something you'd text to friends. Only what's on the page.

2. DESCRIPTION (max 400 chars): what the event is and its key details (what, when, where) from the page, in a welcoming tone. Only details actually present; if the event is online/virtual, say so here.

3. DATE (YYYY-MM-DD or ""), TIME (HH:MM 24-hour or ""), TIME_CONFIDENCE:
   - Extract dates and times from the page, in the user's timezone (from CURRENT TIME) unless the page states one.
   - A date without a year resolves to its next occurrence after CURRENT TIME; a date WITH a year stays as printed, even if past. Recurring phrasing ("first Thursday of each month") → the next occurrence. Multi-day ranges → the first day. Ignore registration or ticket deadlines - extract the event date.
   - No real date or time on the page → "" - NEVER placeholders like "<UNKNOWN>" or "TBD".
   - TIME_CONFIDENCE: EXPLICIT (clear date/time on the page), INFERRED (partial, e.g. "Saturday evenings"), UNKNOWN (none found).

4. `+locationExtractionRules+`
   - LOCATION_QUERY: the venue name or address from the page, or "" if none. Online/virtual events → "". NEVER copy USER REGION into location_query - it describes the reader, not the event.

5. SEARCH_KEYWORDS (3-5 visual terms for a stock-image query): the primary activity first, plus setting/mood/category terms. Generic visual terms only - never personal names or venue/city/business names.

6. VALUE_ESTIMATE (REQUIRED): PER-PERSON value in USD. If the page states a ticket price or fee, use that amount. Otherwise estimate the cost to hire someone to host a similar experience at local rates.
   - estimated_value_usd: NEVER 0 - if uncertain, estimate with low confidence instead.
   - confidence: 0.8-1.0 stated price or comparable known; 0.5-0.8 known type; 0.3-0.5 general estimate; 0.1-0.3 rough guess.
   - reasoning: ≤10 words.

7. CONFIDENCE (0.0 to 1.0): high for clear event pages. LOW when the page is not a specific event (venue home pages, program listings, articles) - then leave date, time, and location empty rather than inventing them.

EXAMPLES:

Page Title: "Community Repair Cafe - Fix It Together"
Page Content: "Repair Cafe at the Westside Tool Library. Saturday, April 18, 2026, 10:00 AM - 1:00 PM. Bring broken lamps, bikes, and clothes - volunteer fixers on hand. Free, drop-in."
Current Time: 2026-03-01T10:00:00-07:00 (Sunday)
Output:
Title: "Repair Cafe at the tool library"
Description: "Drop-in repair cafe at Westside Tool Library on April 18, 10am-1pm - bring broken lamps, bikes, or clothes and volunteer fixers will help. Free!"
Date: 2026-04-18 | Time: 10:00 | Time Confidence: EXPLICIT
Location Query: "Westside Tool Library"
Search Keywords: ["repair", "tools", "workshop"]
Confidence: 0.9

Page Title: "Sunrise Hardware - About Us"
Page Content: "Sunrise Hardware has served the neighborhood since 1982. Open Mon-Sat 8am-6pm. Visit us for tools, paint, and garden supplies."
Output:
Title: "Sunrise Hardware"
Description: "A neighborhood hardware store page - no specific event listed."
Date: "" | Time: "" | Time Confidence: UNKNOWN
Location Query: ""
Search Keywords: ["hardware", "tools", "store"]
Confidence: 0.2`, pageTitle, pageDescription, pageBody, region, currentTime)
}

// buildGearFromWebpagePrompt creates the prompt for extracting gear content and value from a product webpage.
func buildGearFromWebpagePrompt(pageTitle, pageDescription, pageBody, region string) string {
	regionContext := ""
	if region != "" {
		regionContext = fmt.Sprintf(" in the %s area", region)
	}

	return fmt.Sprintf(`You're helping someone add an item to their community sharing library%s from a product page. Extract the product details into structured metadata.

PAGE TITLE: "%s"
PAGE DESCRIPTION: "%s"
PAGE CONTENT:
---
%s
---

`+toneAndStyle+`

FIELDS:

1. TITLE (target ~20 characters, max 30): clear item name that ALWAYS includes the product noun ("DeWalt 20V Drill", not "DeWalt DCD771C2"); include brand and model line when they make it more recognizable.

2. DESCRIPTION (target ~200 characters, max 400): 2-3 sentences on what the item is, its key features, and why a borrower would want it - a friend describing an item, not marketing copy.

3. CATEGORY: specific but common ("Power Tools", "Camping & Outdoors", "Kitchen Appliances", "Electronics", "Sports Equipment").

4. BRAND: manufacturer name from the page, else "". Never placeholders ("Unknown", "<UNKNOWN>", "N/A", "Generic").

5. MATERIAL_CATEGORY: one of solid_metal, solid_plastic, mixed_plastic_metal, mixed_wood_metal, mixed_wood_plastic, wood, aluminum, fabric, cordless_power_tool, corded_power_tool, petrol_tool, electronics_small. Pick by PRIMARY material or tool type from the page's specs (battery tool → cordless_power_tool; corded → corded_power_tool; gas → petrol_tool; tents/bags/soft goods → fabric; ladders/cookware → aluminum). Empty if truly uncertain.

6. WEIGHT_GRAMS: the item weight from the page, converted to grams (1 lb = 454 g, 1 oz = 28 g, 1 kg = 1000 g). 0 only if absent and not reasonably estimable.

7. VALUE_ESTIMATE (REQUIRED):
   - estimated_value_usd: the page's price. Prefer the regular/MSRP price over sale or member prices; for a price range or bundle, use the base configuration shown. No price visible → estimate from product type. NEVER 0 - if uncertain, estimate with low confidence instead.
   - confidence: 0.9-1.0 exact price on page; 0.7-0.9 price visible but promotional; 0.5-0.7 estimated from similar products; 0.3 and below rough guess.
   - reasoning: ≤10 words ("Listed at $99.00, MSRP").

8. SEARCH_KEYWORDS (3-5 visual terms for a stock-photo query): the item first, then category/setting terms ("drill", "power tools", "workshop"). Generic visual terms only - no brand or store names.

9. CONFIDENCE (0.0 to 1.0): high for clear product pages. LOW when the page is not a product listing (store home pages, category indexes, news articles) - then leave the other fields empty rather than inventing an item.

EXAMPLES:

Page Title: "Lodge 6-Quart Enameled Dutch Oven"
Page Content: "Lodge 6 Qt Enameled Cast Iron Dutch Oven. Sale $59.90, reg. $79.95. Chip-resistant porcelain enamel, self-basting lid. Weight: 16.66 lbs."
Output:
Title: "Lodge 6Qt Dutch Oven"
Description: "Enameled cast iron dutch oven, great for braises and bread. Self-basting lid and chip-resistant enamel. Heavy but cooks like a dream."
Category: "Kitchen Appliances" | Brand: "Lodge" | Material: solid_metal | Weight: 7560
Value: 79.95 (confidence 0.9, "Regular price $79.95, on sale $59.90")
Search Keywords: ["dutch oven", "cast iron", "kitchen"]
Confidence: 0.95

Page Title: "Garden Prep 101 - Getting Beds Ready"
Page Content: "Spring is coming! Our favorite all-around helper is the Gorilla Carts 4-cu-ft poly wheelbarrow - sturdy, easy to dump, and around $99 at most hardware stores..."
Output:
Title: "Gorilla Carts Wheelbarrow"
Description: "Sturdy 4 cu ft poly dump cart that makes yard cleanup and garden prep way easier. Easy-dump design."
Category: "Lawn & Garden" | Brand: "Gorilla Carts" | Material: mixed_plastic_metal | Weight: 0
Value: 99 (confidence 0.6, "Article cites ~$99 at hardware stores")
Search Keywords: ["wheelbarrow", "garden", "yard work"]
Confidence: 0.7`,
		regionContext, pageTitle, pageDescription, pageBody)
}

// buildSocialAttributePrompt creates the prompt for inferring Social Footprint input
// attributes (duration and vulnerability) from transaction context.
// txType is one of: gear_loan, giveaway, request, experience.
// itemValueUSD is 0 when unknown.
func buildSocialAttributePrompt(title, description, txType string, itemValueUSD float32) string {
	txDesc := map[string]string{
		"gear_loan": "a gear loan (one person lends an item to another)",
		"giveaway":  "a giveaway (one person gives an item to another)",
		"request":   "a neighbor-help request (one person asks for help, another fulfills it)",
		// experience-string-allow: names a transaction type for the model; this prompt returns numeric Social Footprint attributes, never user-visible copy.
		"experience": "a shared experience or event (host invites neighbors to participate)",
	}
	txSentence := txDesc[txType]
	if txSentence == "" {
		txSentence = "a community sharing transaction"
	}

	valueStr := ""
	if itemValueUSD > 0 {
		valueStr = fmt.Sprintf(" The item is estimated at $%.0f USD.", itemValueUSD)
	}

	return fmt.Sprintf(`You are estimating two Social Footprint attributes for a community sharing transaction.

Transaction type: %s
Title: %s
Description: %s%s

TASK 1 — DURATION
Estimate the expected face-to-face interaction time in minutes between the two people involved.
This is the time they spend together during the handoff, help session, or event — NOT the duration of
any activity before or after. Focus on in-person contact time only.

Guidelines by transaction type:
- gear_loan: handoff and brief demo (5–60 min based on item complexity)
- giveaway: simple handoff (5–20 min)
- request: the help session itself (30–180 min based on task scope)
- experience: the shared event duration (60–240 min)

TASK 2 — VULNERABILITY
Classify the interaction's vulnerability level as one of: high, medium, low.
Vulnerability reflects the degree of personal trust and exposure involved:
- high (1.3×): hosting someone at home, lending high-value/personal items, intimate help tasks
- medium (1.0×): standard gear handoffs, most neighbor help, typical events at shared spaces
- low (0.8×): brief drop-offs of simple items, fully public events with little personal exposure

Return ONLY a JSON object with these exact fields:
{
  "duration_minutes": <float, estimated interaction minutes>,
  "duration_confidence": <float 0.0-1.0, how confident you are in the duration>,
  "duration_reasoning": <string, one sentence explaining the duration estimate>,
  "vulnerability_level": <string: "high", "medium", or "low">,
  "vulnerability_reasoning": <string, one sentence explaining the vulnerability level>
}`, txSentence, title, description, valueStr)
}

// buildExperienceSuggestionsPrompt creates the prompt for generating need/contribution chip labels.
func buildExperienceSuggestionsPrompt(name, description, category string) string {
	categoryLine := ""
	if category != "" {
		categoryLine = fmt.Sprintf("\nCategory: %s", category)
	}

	descLine := ""
	if description != "" {
		descLine = fmt.Sprintf("\nDescription: %s", description)
	}

	return fmt.Sprintf(`You are helping organizers of a community experience crowdsource what people can bring or do.

Experience name: %s%s%s

Generate up to 8 short labels (1–4 words each) for things a participant could claim.
Mix two kinds of suggestions, in roughly equal proportion:
  1. ITEMS to bring — noun phrases. Examples: "Water", "First aid kit", "Folding chairs", "Photos"
  2. TASKS to do — short imperative phrases. Examples: "Drive carpool", "Clean up", "Teach beginners", "Greet guests", "Set up tables"

Both forms are valid. Pick the form that reads most naturally for each suggestion.
Avoid filler verbs like "bring" or "pack" on item suggestions — say "Water", not "Bring water".
For task suggestions, lead with the verb in imperative form.

Also provide a short category_hint (2–4 words, lowercase) that describes the experience type for display
(e.g., "outdoor hike", "cooking class", "yoga session").

%s

CRITICAL RULES:
- Labels MUST be 1–4 words — never full sentences
- Mix items (noun phrases) and tasks (imperative actions) — both are valuable
- Labels must be specific to this experience type, not generic
- Capitalize the first letter of each label
- Do NOT repeat the same concept twice (don't list both "Music" and "DJ booth")
- Order suggestions by relevance — most likely / most useful first
- Return ONLY the JSON object, no explanation

Return JSON:
{
  "suggestions": ["label 1", "label 2", ...],
  "category_hint": "short category phrase"
}`,
		name,
		descLine,
		categoryLine,
		toneAndStyle)
}

// buildRequestSuggestionsPrompt creates the prompt for generating Plan-tab chip labels for a help request.
func buildRequestSuggestionsPrompt(title, description, location string) string {
	descLine := ""
	if description != "" {
		descLine = fmt.Sprintf("\nDescription: %s", description)
	}
	locationLine := ""
	if location != "" {
		locationLine = fmt.Sprintf("\nLocation: %s", location)
	}

	return fmt.Sprintf(`You are helping someone coordinate a community help request.

Request: %s%s%s

Generate three sets of short chip labels for a planning UI.
Labels must be 1–4 words each — never full sentences.
Mix two kinds of labels in each list, in roughly equal proportion:
  1. ITEMS — noun phrases for things to bring/lend/provide. Examples: "Cargo straps", "Moving blankets", "Pickup truck"
  2. TASKS — short imperative phrases for things to do. Examples: "Drive truck", "Load boxes", "Watch kids", "Cook dinner", "Translate paperwork"

Both forms are valid. Pick the form that reads most naturally for each suggestion.
Avoid filler verbs like "bring" or "help with" on item suggestions — say "Moving blankets", not "Bring moving blankets".
For task suggestions, lead with the verb in imperative form.

1. "additional_asks": up to 4 things the requester might also need that round out their ask.
2. "breakdown_pieces": up to 4 pieces the task can be split into for separate helpers to claim.
3. "offer_ideas": up to 8 ways a helper could contribute.
4. "seed_needs": the claimable things the request text PLAINLY NAMES, most important first.
   These become the request's starting needs, so extract only what the requester actually wrote:
   - When the text names ONE thing, return a single label ("Lawn mower", "Moving help", "Ride to airport").
   - When the text ENUMERATES several distinct things ("we need picture books, a whiteboard, storage
     bins, art supplies, and construction paper"), return one label per named thing, in the order named.
   - When the text names NOTHING concrete to bring or do (only a situation or feeling), return [].
   Do NOT invent needs the requester did not name — inferred extras belong in additional_asks or
   breakdown_pieces, never here. At most 8. Precision matters more than recall: a wrongly-added need
   makes the request read as more demanding than it is.

CRITICAL RULES:
- Labels MUST be 1–4 words — never full sentences
- Mix items (noun phrases) and tasks (imperative actions) — both are valuable
- Specific to this request, not generic
- Capitalize first letter of each label
- No duplicates across the four lists
- Order each list by relevance — most likely / most useful first
- Return ONLY the JSON object

Return JSON:
{
  "additional_asks": ["label 1", ...],
  "breakdown_pieces": ["label 1", ...],
  "offer_ideas": ["label 1", ...],
  "seed_needs": ["label 1", ...]
}`,
		title,
		descLine,
		locationLine)
}
