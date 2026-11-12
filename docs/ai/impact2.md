# Community Metrics Screen Redesign

## Overview

Redesign the community metrics screen to support communities across their full lifecycle — from day one with zero data through mature communities with rich sharing history. The current screen assumes data exists (hero + metric selector + inline detail views). The new design shows progression, potential before proof, milestones, and research-backed context — without feeling like a game.

**Reference materials:**
- [progress.html](../../tmp/metrics3/progress.html) — React reference showing all lifecycle stages
- [metric-new.png](../../tmp/metrics3/metric-new.png) — New community (0 items, 1 member)
- [metric-listed.png](../../tmp/metrics3/metric-listed.png) — Items listed, no loans yet
- [metric-1st.png](../../tmp/metrics3/metric-1st.png) — First loan completed
- [metric-active.png](../../tmp/metrics3/metric-active.png) — Active community (14 loans)
- [metric-mature.png](../../tmp/metrics3/metric-mature.png) — Mature community (65 loans)
- [metric-deep.png](../../tmp/metrics3/metric-deep.png) — Metric detail overlay (slides from right)

## Problems with Current Design

1. **Empty state is hostile.** A new community with no loans sees a hero section with `$0` / `0 kg` / `0 hrs` / `0qt` across four metric tabs, followed by empty detail views. There is nothing to engage with, no guidance, and no sense of what sharing could accomplish.

2. **No progression narrative.** The screen looks identical whether the community has 0 or 65 loans. The only difference is the numbers change. There is no celebration of milestones, no "what's next," and no scaffolding that guides communities from creation through maturity.

3. **Potential is buried.** Item-level metrics already show scenario-based projections ("if 5 friends borrow ski gear → $3,200"), but the community screen shows nothing of the kind. A community with $18K of listed gear should surface what that library could yield if shared.

4. **Metric detail is inline, not navigable.** Tapping a metric dimension expands detail content below the hero. This works but gets cramped with all four metrics competing for space. The reference design opens a full detail screen from the right — more room for charts, breakdowns, and research citations.

5. **No milestone or celebration moments.** First loan, $1,000 saved, new member joining — these moments pass silently. The reference design shows dismissible milestone cards that celebrate community achievements.

6. **Research citations are absent.** The system tracks research references (Holt-Lunstad, Whillans, Helliwell & Huang) in the spec docs but none of this surfaces in the UI. Research context could add weight to the metrics without being preachy.

7. **Four metrics compete for attention.** The current tab bar with four metric values works at maturity but feels overwhelming when most are zero. Early stages should emphasize the one or two metrics that matter most.

8. **People are invisible.** Member count is a number in the metadata line. The Rich Minutes (Quality Time) tab should make the social graph tangible — who joined, when, invited by whom.

9. **No anchoring comparisons.** Abstract numbers like "14,028 kg CO₂" or "330 hrs" don't land. The reference design uses one anchoring metaphor per metric: trees for CO₂, long weekends for time, season passes for savings.

## Proposed Solution

### Community Lifecycle Stages

The screen adapts its layout based on the community's maturity level. This is not separate screens — it is a single screen with conditional rendering based on data thresholds.

| Stage | Condition | Hero shows | Tab bar | Content below |
|-------|-----------|------------|---------|---------------|
| **New** | 0 items, ≤1 member | Member count | Disabled (all zeros) | Welcome card + "What Your Circle Will Build" descriptions + next steps |
| **Listed** | ≥1 item, 0 completed transactions | Library value | Enabled with "Potential" labels | Stat pills + scenario projections + progress bar + next steps |
| **First Transaction** | 1 completed transaction (loan, request, or experience) | First metric value (e.g., "$150") | Enabled with values | Milestone card + stat pills + progress bar + scenario projections + breakdown by source |
| **Active** | 2+ completed transactions | Cumulative metric value | Enabled with values | Optional milestone card + stat pills + chart + trend card + breakdown by source |
| **Mature** | Significant history (20+ transactions or 3+ months) | Cumulative metric value | Enabled with values | Stat pills + chart + trend card + breakdown by source + methodology link |

Stage is computed client-side from `CommunityImpactMetrics` fields. No server changes needed for stage determination.

### Layout Structure (All Stages)

```
┌──────────────────────────────────────────────┐
│  DARK HERO SECTION                           │
│  ──────────────────                          │
│  "N MEMBERS" tag (uppercase, ember color)    │
│  Community Name (Lora serif, 20px)           │
│  Metadata line (members · location)          │
│                                              │
│  Large Hero Value (Lora serif, 48px)         │
│  Label (e.g., "saved by sharing")            │
│  Italic hook line (insight/comparison)       │
│                                              │
├──────────────────────────────────────────────┤
│  TAB BAR (4 metrics)                         │
│  $59.8K  │  14K kg  │  567 rm  │  330 hrs   │
│  SAVED   │  CO₂     │  SHARED  │  TIME      │
│  ═══════                                     │
├──────────────────────────────────────────────┤
│  CONTENT AREA (cream background)             │
│  ──────────────────────────────              │
│  [Milestone card — dismissible]              │
│  [Stat pills — tappable]                     │
│  [Chart card]                                │
│  [Trend/insight card]                        │
│  [Breakdown rows — tappable]                 │
│  [Methodology link]                          │
└──────────────────────────────────────────────┘
```

### Hero Section Changes

**Current:** `ImpactMetricsHeroSection` with community name, metadata, description, stat chips, and an embedded `ImpactMetricSelectorBar`.

**New:** The hero shows a single large value (the currently selected metric) with a label and an italic hook/insight line. The community name and metadata stay. Description moves out or gets truncated. The tab bar sits below the hero as a separate widget (not inside the hero).

The hero value, label, and hook all update when the user switches tabs. Each metric dimension has:
- A value (e.g., "$59.8K")
- A label (e.g., "saved by sharing instead of buying")
- A hook — a single anchoring comparison line customized to the community's actual numbers (e.g., *"$59.8K kept in your neighborhood instead of flowing to retailers."* or *"701 trees planted for a year."* or *"20 long weekends given back to your community."*)

**Hook generation:** The hook line is deterministic (not AI-generated). A set of templates per metric dimension fills in the community's actual values:
- **Money:** `"$X kept in your neighborhood instead of flowing to retailers."` / `"A season pass for every member of your circle."` (when saved/members ≥ avg season pass cost)
- **CO₂:** `"N trees planted for a year."` (where N = kg / 20)
- **Quality Time:** `"~N hours of quality face-to-face time across your circle."` (where N = rm / 60)
- **Time:** `"N long weekends given back to your community."` (where N = hours / 16) — or better: `"N vacation days from your circle."` (where N = hours / 8)

For early stages with no data, the hook is a motivational/educational line instead: *"Your library is ready. Now let's put it to work."*

### Tab Bar

A horizontal bar below the hero with four equal-width columns, each showing:
- The metric value (Lora serif, colored white when active, dim when inactive)
- A short label below (uppercase, tiny, dim)
- An underline bar matching the metric's color when active

**Tab colors:**
- Money (Saved): `--ember` (#D07030) — maps to `AppColors.lightPrimary` / coral
- CO₂: `--forest` (#4C8A4A)
- Quality Time (Shared): `--ocean` (#268080) — maps to teal
- Time: `--stone` (#786A50)

**Early-stage behavior:**
- **New:** All tabs show `$0` / `0 kg` / `0 rm` / `0 hrs`, all dimmed/disabled
- **Listed:** Tabs show potential values with "Potential" labels (e.g., "~$9.1K POTENTIAL")
- **First Transaction onward:** Tabs show actual cumulative values

### Content Area: Stage-Specific Components

#### New Community Content

1. **Welcome celebration card** — Warm white card with gradient top border. "Your community is born!" title. Description text. CTA button "Add your first item."
2. **"What Your Circle Will Build" section** — Four description cards (one per metric), each with a left color bar matching the metric color. Text-only — no numbers. These explain what each metric means and why it matters. Include one research-backed statistic per card (e.g., "A circle of 5 friends sharing gear typically saves $12K/year — that's a season pass for every member.")
3. **Next step cards** — Actionable prompts: "List 5 items to get started," "Invite your first friend."

#### Listed Community Content (per-tab)

1. **Stat pills** — Row of 3 tappable rounded cards (Library value, Utilized %, Item count). Library pill is tappable → opens detail overlay.
2. **Progress bar** — Shows progress toward a milestone (e.g., "Library Value $18.2K" with progress toward $100K community benchmark).
3. **Scenario projection card** — "If your circle starts sharing" with 3-4 scenario rows, each showing a what-if and a projected value. Final row shows cumulative potential. Based on the community's actual inventory.
4. **Community trend card** — Research-backed nudge (e.g., "Groups that list 25+ items save 3× more in year one.").
5. **Next step card** — "Start your first loan" / "Turn this potential into real savings."

#### First Transaction Content (per-tab)

1. **Milestone card** — Dismissible. "First loan completed" with description. Gradient top border.
2. **Stat pills** — Library, Utilized %, Loan count.
3. **Progress bar** — "Toward first $500" with fraction display.
4. **Scenario projection card** — "What's ahead for your circle" with 2-3 projections.
5. **Breakdown by source** — Tappable rows: Loans, Giveaways, Requests (each with count and value, chevron to open detail).

#### Active / Mature Content (per-tab)

1. **Milestone card** — Dismissible. Stage-appropriate (e.g., "$1,000 saved together", "5th member joined"). Not shown at mature stage.
2. **Stat pills** — Three context-appropriate pills per metric (e.g., Money: Library / Utilized / Avg Value; CO₂: kg Total / Trees / Cars Equiv; QT: Total RM / Per Person / Members; Time: Total Hrs / Per Person / Avg/Loan).
3. **Chart card** — Cumulative line chart (Money, QT, Time) or monthly bar chart (CO₂). Uses `cumulative_trend` from `GetCommunityMetricDetailResponse`.
4. **Trend card** — Contextual insight with arrow icon (e.g., "65 loans completed across 5 members. Your circle is 16% through its shared library.").
5. **Breakdown by source** — Loans / Giveaways / Requests rows with counts and values, tappable with chevron.
6. **Methodology link** — "How we calculate savings →" at bottom, opens methodology doc.

#### Quality Time Tab Extras (All Stages)

- **Sufficiency ring** — Ring visualization showing weekly QT minutes vs. 360-minute target. Label shows percentage toward sufficiency.
- **Members pill** — Tappable, opens Members detail overlay showing who joined, when, and who invited them.

### Metric Detail Overlay

Tapping a stat pill or breakdown row opens a detail screen that slides in from the right (per [modals.md](../client/modals.md) patterns). Uses `SwipeToCloseMixin` + `NavigationHelpers.pushScreen()`.

**Layout:**
```
┌──────────────────────────────────────────────┐
│  ← Community Library                         │
├──────────────────────────────────────────────┤
│  $385.1K (large, colored)                    │
│  total value across all listed items         │
│  "A shared library worth more than most      │
│   people's homes."                           │
├──────────────────────────────────────────────┤
│  ↑ Growing steadily as members add more      │
│    gear. Every item listed is a future save. │
├──────────────────────────────────────────────┤
│  LIBRARY VALUE OVER TIME                     │
│  [chart]                                     │
├──────────────────────────────────────────────┤
│  MOST VALUABLE ITEMS                         │
│  • Winnebago Revel Campervan   $145K         │
│    Alfred · 2 loans                          │
│  • Backcountry Ski Kit         $18K          │
│    Sarah · 4 loans                           │
├──────────────────────────────────────────────┤
│  BY CATEGORY                                 │
│  • Ski & Snow (34 items)       $238K         │
│  • Vehicles (3 items)          $108K         │
│  • Camping (42 items)          $31K          │
├──────────────────────────────────────────────┤
│  How we estimate item values →               │
└──────────────────────────────────────────────┘
```

**Detail overlay types:**
- **Community Library** — Library value, top items by value, items by category. Data from gear listing + value estimates.
- **Savings from Loans** — Loan savings, top loans by savings, monthly chart. Data from `money_detail` in `GetCommunityMetricDetailResponse`.
- **Emissions Breakdown** — Total kg, trees equiv, manufacture vs. waste split. Data from `emissions_prevented_detail`.
- **Quality Time Detail** — Total RM, per-person, weekly average, sufficiency ring, by interaction type. Data from `quality_time_detail`.
- **Time Breakdown** — Total hours, per-person, avg/loan, by source. Data from `time_detail`.
- **Community Members** — Member count, list with join date and inviter. New data needed (see Open Questions).

### Milestone System

Milestones are dismissible cards that celebrate community moments. They appear at the top of the content area.

**Proposed milestone set:**

| Milestone | Trigger | Message |
|-----------|---------|---------|
| Community created | 0 transactions, 1 member | "Your community is born!" (welcome card, not dismissible) |
| First item listed | gear_count goes from 0 to 1 | "First item listed — your library is started." |
| First loan completed | completed_loans goes from 0 to 1 | "First loan completed. [Borrower] borrowed [item]." |
| First request fulfilled | fulfilled_requests goes from 0 to 1 | "First request fulfilled! [Helper] helped out." |
| First experience completed | past_events goes from 0 to 1 | "First community gathering completed!" |
| $500 saved | cost_savings ≥ $500 | "Your circle saved its first $500." |
| $1,000 saved | cost_savings ≥ $1,000 | "$1,000 saved together!" |
| $5,000 saved | cost_savings ≥ $5,000 | "$5K saved — that's a ski trip for everyone." |
| $10,000 saved | cost_savings ≥ $10,000 | "$10K kept in your neighborhood." |
| 5th member | member_count ≥ 5 | "5th member joined! Communities this size see the biggest jump in savings." |
| 10th member | member_count ≥ 10 | "10 people strong — your circle is becoming a movement." |
| 10 items listed | gear_count ≥ 10 | "10 items in your shared library." |
| 50 items listed | gear_count ≥ 50 | "50 items! Your library is worth more than most tool collections." |

**Dismissal storage:** Client-side. Store dismissed milestone IDs in `SharedPreferences` keyed by `community_id`. No server changes.

**Feed stories:** Milestones should also be celebrated in the community feed. This is a separate feature (feed story creation) and out of scope for this doc. Flagged for future work.

### Research Citation System

Citations appear in two places:
1. **Insight cards** — Italic quote + citation line below a divider. Used in early-stage description cards and QT tab content.
2. **Detail overlay** — Research-backed context in the detail overlay body.

**Citation format:**
```
"Strong social ties increase lifespan as much as quitting smoking."
— Holt-Lunstad et al. (2010): Social integration comparable to
smoking cessation for mortality risk.
```

**Citation data source:** Hardcoded in the client initially. Each metric dimension has 2-3 curated citations from existing research references in the spec docs:

- **Money:** Consumer spending research, sharing economy savings studies
- **CO₂:** IPCC lifecycle assessment references, material emission factors
- **Quality Time:** Holt-Lunstad (2010) social integration, Helliwell & Huang (2011) social trust and well-being
- **Time:** Whillans et al. (2017) time affluence and life satisfaction

**Future curation:** Move citations to a structured data file (JSON/YAML) that can be updated without code changes. Consider serving via a new RPC alongside methodology docs.

### Anchoring Comparisons

One comparison per metric, used in the hero hook line and detail overlay hero:

| Metric | Comparison | Formula |
|--------|-----------|---------|
| Money Saved | Season passes | `saved / avg_season_pass_cost` (≈ $1,200) |
| CO₂ Avoided | Trees planted for a year | `kg / 20` (one tree absorbs ~20 kg CO₂/year) |
| Quality Time | Hours of face-to-face time | `rm / 60` |
| Time Recovered | Vacation days (8 hrs each) | `hours / 8` |

The comparison value appears in the hero hook and in the detail overlay hero subtitle.

## Open Questions

1. **Members data for QT tab overlay.** The current `CommunityImpactMetrics` only has `member_count`. To show "who joined, when, invited by whom" we need either a new RPC or to extend `GetCommunityMetricDetailResponse` for the QT dimension. The community member list already exists server-side (`CommunityUser` records with `created_at` and possibly `invited_by`). **Recommendation:** Add a `members` repeated field to `QualityTimeDetail` containing a lightweight `CommunityMemberSummary` (user_id, display_name, joined_at, invited_by_user_id, invited_by_display_name). This keeps it in the existing RPC.

2. **Scenario projections data source.** The "Listed" stage shows "If 5 friends borrow ski gear → $3,200" projections. Item-level `potentialImpact` already exists. For community-level projections, options:
   - **Option A (recommended):** Compute on the client from `total_value_usd` and item category distribution. Simple formulas: potential savings ≈ `total_value * prevented_purchase_rate * utilization_rate`.
   - **Option B:** Add a new `potential_scenarios` field to `CommunityImpactMetrics` computed server-side from actual inventory.
   - **Recommendation:** Start with Option A for MVP. The scenario cards are motivational, not precision instruments. Move to Option B if we want item-specific scenarios ("5 friends borrow the ski gear").

3. **Library detail overlay data.** The "Community Library" detail shows top items by value and items by category. This requires either a new RPC or extending the existing metrics response with item-level data. **Recommendation:** New RPC `GetCommunityLibrarySummary` returning top items and category breakdown. Keep it separate from impact metrics since library data is structural, not impact.

4. **Milestone feed stories.** Milestones should also be celebrated in the community feed as stories. This requires the feed system to support milestone story types. **Recommendation:** Out of scope for this redesign. Track as a separate issue.

5. **Chart rendering.** The reference design shows SVG line/bar charts. Flutter options:
   - **fl_chart** — Popular, customizable, supports line/bar charts. Already used elsewhere in the app? Check dependencies.
   - **Custom `CustomPainter`** — Full control, no dependency, but more work.
   - **Recommendation:** Use `fl_chart` if already a dependency, otherwise `CustomPainter` for simple cumulative line and monthly bar charts. The charts are decorative — they don't need interactivity.

6. **Tab bar vs. hero metric value.** In the reference design, switching tabs updates both the hero value and the content below. The current `ImpactMetricSelectorBar` is inside the hero. The new design has the tab bar below the hero as a separate widget. This means the hero section needs to receive the currently-selected metric's value/label/hook as parameters (not render the selector itself). **Decision needed:** Should the hero + tab bar be a single combined widget, or separate widgets composed in the screen? **Recommendation:** Separate widgets. The screen manages the selected metric state and passes props down to both the hero and the content area.

## Architecture Compliance Notes

### Server Changes Required

1. **Community member list in QT detail** — Extend `QualityTimeDetail` with member summaries (see Open Question 1).
2. **Community library summary** — New RPC for library detail overlay (see Open Question 3). Could be deferred if the library detail overlay is deferred.
3. **Scenario projections** — Optional. Can be computed client-side for MVP.

### Client Changes

- **No new repositories or services** needed for the basic redesign. All data comes from existing `communityImpactProvider` and `communityImpactMetricDetailProvider`.
- **New widgets** needed for: lifecycle-stage-aware content, milestone cards, scenario cards, stat pills, progress bars, chart cards, trend cards, next-step cards, tab bar, detail overlays.
- **Milestone dismissal** uses `SharedPreferences` — no repository needed.
- **Detail overlay navigation** uses `SwipeToCloseMixin` + `NavigationHelpers.pushScreen()` per [modals.md](../client/modals.md).
- **MVVM compliance:** All new widgets are presentation-only. State management stays in existing view models. New view model may be needed for detail overlay state.

### Compliant

- Repository pattern preserved — all data through existing providers.
- No manual cache maps in view models.
- Widget extraction keeps files under 1000 lines.
- `CachedMediaImage` for user-uploaded media.
- Structured logging for any new server code.

### Issues to Watch

- **File count:** This redesign creates many new widget files. Group by feature: `widgets/impact/community/` subdirectory for community-specific metric widgets.
- **Conditional rendering complexity:** Stage-based rendering with per-tab content can get complex. Keep stage determination in the view model, widget selection in the screen.
- **Chart performance:** Ensure chart widgets don't rebuild unnecessarily. Use `const` constructors and cache chart data computation.

## Phased Implementation Plan

### Phase 1: Hero + Tab Bar Refactor

**Goal:** Replace the current hero + inline metric selector with the new hero (large value + label + hook) and a separate tab bar below it.

**Files changed:**
- [community_metrics_screen.dart](../../app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart) — New layout structure
- **New:** `app/lib/presentation/widgets/impact/community/impact_tab_bar.dart` — Four-column tab bar with values, labels, underline
- **New:** `app/lib/presentation/widgets/impact/community/impact_hero_value.dart` — Large value + label + hook line for the hero

**Changes:**
1. Create `ImpactTabBar` widget: four equal columns, each with value text (Lora serif) and label, active tab has colored underline and white text, inactive tabs dimmed.
2. Create `ImpactHeroValue` widget: large value (Lora, 48px), label below, italic hook line. Accepts value, label, hook, and color.
3. Refactor `CommunityMetricsScreen` to compose: hero (with `ImpactHeroValue` inside) → `ImpactTabBar` → content area.
4. Remove `ImpactMetricSelectorBar` usage from community screen (it stays for item/user screens).
5. Hero value/label/hook update when tab selection changes.

**Hook templates:** Implement deterministic hook generation as a utility function. Input: metric dimension + community data. Output: hook string.

**Test:** Tab switching updates hero value/label/hook. All four tabs render correct values. Early stages show dimmed/disabled tabs.

### Phase 2: Stage Detection + New/Listed Stages

**Goal:** Implement lifecycle stage detection and the "New" and "Listed" stage content.

**Files changed:**
- **New:** `app/lib/presentation/widgets/impact/community/community_stage.dart` — Stage enum + detection logic
- **New:** `app/lib/presentation/widgets/impact/community/welcome_card.dart` — "Your community is born!" celebration card
- **New:** `app/lib/presentation/widgets/impact/community/metric_description_card.dart` — "What Your Circle Will Build" text card
- **New:** `app/lib/presentation/widgets/impact/community/next_step_card.dart` — Actionable next-step prompt
- **New:** `app/lib/presentation/widgets/impact/community/scenario_card.dart` — "If your circle starts sharing" projection card
- **New:** `app/lib/presentation/widgets/impact/community/stat_pill_row.dart` — Row of 3 tappable stat pills
- **New:** `app/lib/presentation/widgets/impact/community/progress_bar_card.dart` — Progress toward milestone
- [community_metrics_screen.dart](../../app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart) — Stage-based content routing

**Changes:**
1. Implement `CommunityStage` enum (new, listed, firstTransaction, active, mature) with a `detect()` factory that takes `CommunityImpactMetrics`.
2. Create all early-stage widgets (welcome, descriptions, scenarios, next steps, stat pills, progress bar).
3. Route content rendering based on stage in `CommunityMetricsScreen`.
4. New stage: welcome card + four description cards + two next-step cards.
5. Listed stage: stat pills + scenario card (per tab) + progress bar + trend card + next step.

**Scenario computation:** Client-side. For money: `total_value * 0.5 * 0.15` (50% prevented purchase rate × 15% utilization). For CO₂: derive from `total_value` using avg emission factor. For time: `item_count * 2 hrs` (avg time saved per loan). Display as 3-4 "what if" rows.

**Test:** Stage detection returns correct stage for various data combinations. New stage renders welcome card. Listed stage renders scenario projections. Tab switching renders tab-specific scenarios.

### Phase 3: Milestone Cards

**Goal:** Implement dismissible milestone cards that celebrate community achievements.

**Files changed:**
- **New:** `app/lib/presentation/widgets/impact/community/milestone_card.dart` — Dismissible milestone card widget
- **New:** `app/lib/presentation/widgets/impact/community/milestone_tracker.dart` — Tracks milestone state (which milestones are earned, which are dismissed)
- [community_metrics_screen.dart](../../app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart) — Render milestones at top of content area

**Changes:**
1. Create `MilestoneCard` widget: gradient top border, title, description, dismiss button (×). Animate out on dismiss (opacity + height collapse).
2. Create `MilestoneTracker` — utility class that computes earned milestones from `CommunityImpactMetrics`, reads dismissed milestones from `SharedPreferences`, returns list of milestones to show.
3. Render active (earned but not dismissed) milestones at top of content area for First Transaction and Active stages.

**Test:** Milestones appear when thresholds are crossed. Dismiss persists across rebuilds. Multiple milestones stack correctly.

### Phase 4: Active/Mature Stage Content

**Goal:** Implement the full content layout for communities with real data — charts, trend cards, breakdown rows.

**Files changed:**
- **New:** `app/lib/presentation/widgets/impact/community/impact_chart_card.dart` — Cumulative line chart or monthly bar chart
- **New:** `app/lib/presentation/widgets/impact/community/trend_card.dart` — Contextual insight with arrow icon
- **New:** `app/lib/presentation/widgets/impact/community/breakdown_row.dart` — Tappable source breakdown row (dot + label + count + value + chevron)
- **New:** `app/lib/presentation/widgets/impact/community/methodology_link.dart` — "How we calculate X →" link
- **New:** `app/lib/presentation/widgets/impact/community/sufficiency_ring.dart` — QT weekly sufficiency ring visualization
- [community_metrics_screen.dart](../../app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart) — Active/mature content rendering

**Changes:**
1. Chart card renders cumulative line (Money, QT, Time) or monthly bar (CO₂) from `cumulative_trend` data.
2. Trend card shows contextual insight text with up-arrow icon.
3. Breakdown rows for Loans/Giveaways/Requests with count and value. Tappable → opens detail overlay (Phase 6).
4. Sufficiency ring for QT tab — ring showing weekly minutes vs. 360 target.
5. Methodology link at bottom opens `MethodologyDocScreen`.

**Chart implementation:** Use `CustomPainter` for simple line/bar charts. No external dependency needed for the basic shapes shown in the reference.

**Test:** Charts render with mock data. Breakdown rows show correct counts/values. Sufficiency ring renders at correct percentage. Tab switching shows tab-appropriate content.

### Phase 5: Research Citations + Insight Cards

**Goal:** Surface research-backed context in insight cards and early-stage description cards.

**Files changed:**
- **New:** `app/lib/presentation/widgets/impact/community/insight_card.dart` — Italic quote + citation divider card with left color bar
- **New:** `app/lib/core/utils/impact_citations.dart` — Curated citation data (hardcoded initially)
- Update early-stage description cards and QT tab content to include insight cards.

**Changes:**
1. Create `InsightCard` widget: left color bar, italic Lora serif quote, divider, citation text below.
2. Create citations utility with per-dimension curated citations.
3. Add insight cards to: QT tab content (sufficiency research), Time tab content (time affluence), early-stage description cards.

**Citations included:**
- Holt-Lunstad et al. (2010) — social integration and mortality
- Helliwell & Huang (2011) — social trust and life satisfaction
- Whillans et al. (2017) — time affluence and happiness
- Consumer spending statistics for money context

**Test:** Insight cards render with correct quote and citation. Color bars match metric dimension.

### Phase 6: Metric Detail Overlays

**Goal:** Replace inline metric detail expansion with full-screen detail overlays that slide in from the right.

**Files changed:**
- **New:** `app/lib/presentation/screens/impact_metrics/community_metric_detail_screen.dart` — Full detail overlay screen with `SwipeToCloseMixin`
- Update breakdown rows and stat pills to navigate to detail screen on tap.
- Reuse existing detail view data from `communityImpactMetricDetailProvider`.

**Changes:**
1. Create `CommunityMetricDetailScreen` — full-screen overlay with:
   - Back button + title
   - Large hero value + subtitle + hook
   - Optional trend card
   - Chart (larger than in the main screen)
   - Stat pills
   - Breakdown sections (top items, by category, by source)
   - Methodology link
2. Use `SwipeToCloseMixin` for swipe-right-to-close.
3. Navigate via `NavigationHelpers.pushScreen()`.
4. Data from existing `GetCommunityMetricDetailResponse`.

**Detail variants:**
- Money: "Savings from Loans" — top loans, monthly chart, by source
- CO₂: "Emissions Breakdown" — by type (manufacture/waste), monthly chart
- QT: "Quality Time Detail" — sufficiency ring, by interaction type, members list
- Time: "Time Breakdown" — by source, comparison card (borrow vs. shop)

**Test:** Tapping stat pill opens detail overlay. Swipe-right closes. Back button closes. Data renders correctly from provider.

### Phase 7: Members Detail Overlay (QT Tab)

**Goal:** Make the social graph tangible — show who joined, when, and who invited them.

**Files changed:**
- **Server:** Extend `QualityTimeDetail` in proto with `CommunityMemberSummary` repeated field
- **Server:** Populate member summaries in metric detail calculator
- **New:** `app/lib/presentation/screens/impact_metrics/community_members_screen.dart` — Members detail overlay
- QT stat pills "Members" pill navigates to members screen.

**Changes:**
1. Add `CommunityMemberSummary` message to proto (user_id, display_name, profile_media_id, joined_at_unix_sec, invited_by_display_name).
2. Add `members` field to `QualityTimeDetail`.
3. Server populates from `CommunityUser` records + user profiles.
4. Members overlay shows list with dot timeline, each row: name, "Joined [date] · Invited by [name]" or "Founded the group · [date]".

**Test:** Members overlay shows correct member list. Tapping Members pill opens overlay. First member shows "Founded" instead of "Invited by."

### Phase 8: Polish + Cleanup

**Goal:** Final visual polish, remove old inline detail views, ensure all stages render correctly.

**Files changed:**
- Remove inline detail view rendering from community screen (the `_buildMetricDetailSection` method and `PeriodToggleBar`).
- Remove `PeriodToggleBar` dependency if no longer used elsewhere.
- Visual polish: spacing, fonts, colors matching reference exactly.
- Verify all stage transitions work smoothly.
- Test empty states, error states, loading states.

**Test:** Full manual test across all lifecycle stages. Verify no regressions in item/user metric screens. Performance check on chart rendering.

### Phase 9: Hero Area Polish — Match Reference Designs

**Goal:** Make the upper hero area match the reference designs. Remove visual clutter (member count, description, horizontal divider). Make the large metric value use the theme's highlight color (orange/coral). Fix the hero to a stable height so tab-switching does not cause layout jumps.

**Files changed:**
- [impact_hero_value.dart](../../app/lib/presentation/widgets/impact/community/impact_hero_value.dart) — Large value color changed from white to `AppColors.lightPrimary` (coral). Hook line truncated to 2 lines with tap-to-expand (prevents height changes).
- [impact_hero_section.dart](../../app/lib/presentation/widgets/impact/impact_hero_section.dart) — Added `showBottomDivider` parameter (default `true`). When `false`, the horizontal rule above `bottomContent` is omitted.
- [community_metrics_screen.dart](../../app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart) — Removed `subtitle` (member count badge), `metadata` (location line), and `description` from the hero. Set `showBottomDivider: false`. Removed unused `_buildMetaLine` helper.

**Changes:**
1. Hero no longer shows "N MEMBERS" tag, metadata line, or community description.
2. Large metric value renders in `AppColors.lightPrimary` (#E07A5F) instead of white, matching the reference designs' orange hero numbers.
3. Hook line limited to 2 lines with ellipsis; tappable to expand. This keeps the hero a stable height across all four metric tabs.
4. Horizontal divider between hero content and the large value removed (via `showBottomDivider: false`). Item/user metrics screens unaffected — they retain the default `showBottomDivider: true`.

**Test:** Switching tabs does not cause hero height to change. Large value is orange. No member count, description, or divider visible. Item/user metric screens still show their divider.

### Phase 10: Metric Detail Screens & Content Area Polish

**Goal:** Make all metric values in content areas use the highlight color (orange). Remove visual clutter (chevrons, bullet dots). Make every metric row tappable, opening a detail screen that slides in from the right. Detail screen design matches [metric-deep.png](../../tmp/metrics3/metric-deep.png).

**Content area changes (community_metrics_screen.dart):**
1. All metric values (numbers, currency, percentages) in the content/body area render in `AppColors.lightPrimary` (#E07A5F) instead of the current color.
2. Remove all arrow chevron icons (`>` / `Icons.chevron_right`) from metric rows — the rows are tappable but don't show an explicit affordance.
3. Remove the colored dot bullet points next to breakdown rows (Loans, Giveaways, Requests, etc.). Rows should have no leading icon/dot.
4. Every metric row is tappable via `GestureDetector` / `InkWell`, opening a detail screen for that metric dimension.

**New file: metric_detail_screen.dart**
A full-screen detail view for a single metric dimension. Slides in from the right using `NavigationHelpers.pushScreen()` (see [modals.md](../client/modals.md)) to avoid black flickering during the transition. The screen widget should use `SwipeToCloseMixin` for swipe-back-to-dismiss support.

**Detail screen layout (matching reference):**
1. **App bar area** — Back arrow (`←`) + community name ("Community Library") as title text. Dark background matching the hero gradient.
2. **Hero value** — Large orange value (48px serif, `AppColors.lightPrimary`) with label below and italic hook/quote. Reuses `ImpactHeroValue` widget.
3. **Trend card** — Rounded card with an upward/downward trend icon, bold trend summary ("Growing steadily"), and a one-line explanation. Light background (`AppColors.cardBackground` or similar warm beige).
4. **Chart** — Section header in small caps ("LIBRARY VALUE OVER TIME" or metric-appropriate title). Line chart showing the metric's historical data. Chart style: warm orange line, minimal gridlines, clean axis labels.
5. **Top items section** — Section header in small caps ("MOST VALUABLE ITEMS" or metric-appropriate). List of item rows, each showing:
   - Small dot indicator (dark, not colored)
   - Item name (bold) + owner name (secondary text) + loan/transaction count
   - Metric value on the right in `AppColors.lightPrimary`
6. **By category section** — Section header in small caps ("BY CATEGORY"). List of category rows, each showing:
   - Small colored bar/indicator
   - Category name + item count
   - Aggregated metric value on the right in `AppColors.lightPrimary`

**Content varies by metric dimension:**
- **Shared Value ($):** Top items by dollar value, categories by dollar value, chart = "Library Value Over Time"
- **Rich Minutes:** Top items by minutes shared, categories by time, chart = "Minutes Shared Over Time"
- **CO₂ Saved:** Top items by CO₂ impact, categories by CO₂, chart = "CO₂ Avoided Over Time"
- **Active Members:** Top contributors by activity count, no categories section, chart = "Active Members Over Time"

**Navigation pattern:**
- Use `NavigationHelpers.pushScreen()` which creates a `CupertinoPageRoute`-style slide-from-right transition without the black flash that `Navigator.push` with `MaterialPageRoute` can cause.
- The detail screen should mix in `SwipeToCloseMixin` for iOS-style swipe-back gesture support.
- Back arrow in the app bar calls `Navigator.pop(context)`.

**Files to create:**
- `app/lib/presentation/screens/impact_metrics/metric_detail_screen.dart` — Full detail screen widget with `SwipeToCloseMixin`

**Files to modify:**
- `app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart` — Change metric value colors to orange, remove chevrons and bullet dots, add tap handlers that push `MetricDetailScreen`
- Possibly extract shared metric row widgets if needed for reuse between the main screen and detail screen

**Data requirements:**
- The detail screen needs historical time-series data for the chart, top items, and category breakdowns. This may require a new RPC endpoint or extending `GetCommunityMetrics` to include drill-down data. Define the data contract but don't implement the server side in this phase if it requires new proto changes — use placeholder/mock data initially.

**Test:** Tapping any metric row opens the detail screen sliding from the right with no black flash. Detail screen shows the correct metric dimension's data. Swipe-back gesture works. No chevrons or colored dots visible on the main screen. All metric values are orange.

### Phase 11: Category Drill-Down Screens (Loans, Giveaways, Requests)

**Goal:** Every breakdown row on the community metrics screen (Loans, Giveaways, Requests, etc.) is tappable and opens a category-specific detail screen. The detail screen shows that category's contribution to the currently selected metric dimension, with a growth chart and a table of individual items.

**Interaction:**
- On the **Shared Value** tab, tapping "Loans · 85 completed · $48.0K" opens a detail screen scoped to loan-based savings.
- On the **Rich Minutes** tab, tapping the same "Loans" row opens a detail screen scoped to loan-based minutes shared.
- Same pattern for Giveaways, Requests, and any other breakdown categories, across all four metric tabs (Shared Value, Rich Minutes, CO₂ Saved, Active Members).

**Detail screen layout (reuses Phase 10 MetricDetailScreen pattern):**
1. **App bar** — Back arrow + category title (e.g., "Loans" or "Giveaways"). Dark hero gradient background.
2. **Hero value** — Large orange value (serif 48px, `AppColors.lightPrimary`) showing the category's total for the selected metric dimension (e.g., "$48.0K" for loan savings). Label below (e.g., "saved through 85 completed loans"). Italic hook/quote line via `ImpactHook` contextual to the category + metric.
3. **Trend card** — Category-specific trend (e.g., "Loan savings up 12% this month"). Same rounded card style as Phase 10.
4. **Chart** — Line chart showing the category's metric over time (e.g., "Loan Savings Over Time"). Same chart style as Phase 10.
5. **Individual items table** — Section header in small caps (e.g., "INDIVIDUAL LOANS"). Each row shows:
   - Item name (bold) + parties involved (lender → borrower or similar)
   - Date or duration info (secondary text)
   - Metric value on the right in `AppColors.lightPrimary` (e.g., "$1,200" saved)
   - Rows sorted by metric value descending (highest impact first)

**Content varies by category × metric dimension:**

| Category | Shared Value | Rich Minutes | CO₂ Saved | Active Members |
|----------|-------------|-------------|-----------|----------------|
| **Loans** | $ saved per loan | Minutes per loan | CO₂ per loan | Unique lenders/borrowers |
| **Giveaways** | $ value of items given | Minutes of interaction | CO₂ per giveaway | Unique givers/receivers |
| **Requests** | $ value fulfilled | Minutes of interaction | CO₂ per fulfillment | Unique requesters/fulfillers |

**Navigation pattern:**
- Same as Phase 10: `NavigationHelpers.pushScreen()` + `SwipeToCloseMixin`.
- Can be opened from the main community metrics screen breakdown rows, or potentially from a Phase 10 detail screen's "top items" rows (nested drill-down).

**Files to create:**
- `app/lib/presentation/screens/impact_metrics/category_detail_screen.dart` — Category-scoped detail screen. Accepts the category type (loan/giveaway/request) and the currently selected metric dimension. Reuses `ImpactHeroValue` and chart components from Phase 10.

**Files to modify:**
- `app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart` — Add tap handlers to each breakdown row (Loans, Giveaways, Requests) that push `CategoryDetailScreen` with the row's category and the current metric tab.
- Phase 10's `metric_detail_screen.dart` — Optionally make "top items" rows tappable to drill into category detail (if the item belongs to a clear category).

**Data requirements:**
- Needs per-category, per-metric time-series data and individual item breakdowns. Likely requires extending the community metrics RPC or adding a new `GetCategoryMetricDetail` endpoint. Use placeholder/mock data initially if proto changes are deferred.

**Test:** Tapping "Loans" on the Shared Value tab opens a detail screen showing loan-specific savings, chart, and individual loan table. Tapping "Loans" on the Rich Minutes tab shows loan-specific minutes instead. Same for Giveaways and Requests across all four tabs. Back navigation and swipe-to-dismiss work correctly. No regressions on the main metrics screen or Phase 10 detail screens.

### Phase 12: Fix Detail Screen Titles, Values, and Table Sections

**Goal:** The deep-dive metric detail screens currently show incorrect titles, values, and table data. Fix them so every detail screen accurately reflects the specific category × metric dimension the user tapped. Replace the generic "Top Sources" section with "Most Valuable Items" (showing the metric-appropriate unit), and replace "By Category" / "Per Member" / "Per Loan" with "Top Contributors" (listing members who contributed the most value).

**Problem 1 — Title and hero value are wrong:**
The detail screen title and large hero number must match what the user tapped. Example: On the Shared Value tab, tapping "Loans · 85 completed · $48.0K" should show:
- **Title:** "Loans Value Saved" (not a generic title)
- **Hero value:** "$48.0K" (the exact value from the row they tapped)
- **Label:** Context-specific (e.g., "saved through 85 completed loans")

Title format per category × metric:

| Category | Shared Value | Rich Minutes | CO₂ Saved | Active Members |
|----------|-------------|-------------|-----------|----------------|
| **Loans** | Loans Value Saved | Loans Time Shared | Loans CO₂ Avoided | Loans Active Members |
| **Giveaways** | Giveaways Value Shared | Giveaways Time Shared | Giveaways CO₂ Avoided | Giveaways Active Members |
| **Requests** | Requests Value Fulfilled | Requests Time Shared | Requests CO₂ Avoided | Requests Active Members |

**Problem 2 — "Top Sources" should be "Most Valuable Items":**
Replace the "TOP SOURCES" section with **"MOST VALUABLE ITEMS"**. Each row lists an individual item (gear, request, experience) with its metric-specific value:
- **Shared Value:** Dollar amount saved/shared (e.g., "$1,200")
- **CO₂ Saved:** CO₂ weight avoided (e.g., "45 kg")
- **Rich Minutes:** Time shared (e.g., "320 min")
- **Active Members:** Activity count (e.g., "12 actions")

Rows sorted by metric value descending. Each row shows: item name (bold), owner name (secondary), and the metric value on the right in `AppColors.lightPrimary`.

**Problem 3 — "By Category" / "Per Member" should be "Top Contributors":**
Replace the "BY CATEGORY" / "PER MEMBER" / "PER LOAN" section with **"TOP CONTRIBUTORS"**. This lists the community members who have contributed the most value for this specific category × metric combination:
- Each row shows: member name (bold), number of contributions (secondary, e.g., "14 loans"), and the member's total metric value on the right in `AppColors.lightPrimary`.
- Rows sorted by total contribution descending.
- Example on Loans × Shared Value: "Sarah · 14 loans · $12.3K" means Sarah's loans have saved $12.3K total.

**Files changed:**
- [metric_detail_screen.dart](../../app/lib/presentation/screens/impact_metrics/metric_detail_screen.dart) — Added `category` parameter. Title resolves to category × dimension (e.g., "Loans Value Saved") when category is set, otherwise falls back to community name or generic title. Replaced hardcoded "TOP SOURCES" section (which used fake proportional splits of aggregate data) with "MOST VALUABLE ITEMS" / "MOST CO₂ AVOIDED" / "MOST TIME SHARED" / "MOST TIME RECOVERED" using server-provided `topItems` from `GetCommunityMetricDetailResponse`. Replaced hardcoded "BY CATEGORY" section (which showed "Per Member" / "Per Loan" averages) with "TOP CONTRIBUTORS" using server-provided `topContributors`. Removed unused `_formatMoney`, `_formatHours`, `_formatRm` helpers.
- [community_metric_detail_screen.dart](../../app/lib/presentation/screens/impact_metrics/community_metric_detail_screen.dart) — Forwarded `category` parameter.

**Changes:**
1. Title uses `_categoryDimensionTitle(category, dimension)` when a category is provided, producing titles like "Loans Value Saved", "Giveaways CO₂ Avoided", etc.
2. Hero value and label are loaded from the server via `communityImpactProvider` — the detail screen is self-sufficient and does not receive data from the caller.
3. "TOP SOURCES" → dimension-specific header ("MOST VALUABLE ITEMS", "MOST CO₂ AVOIDED", etc.) populated from `detail.topItems` (server data: name, sharerName, loanCount, cumulativeSaved).
4. "BY CATEGORY" → "TOP CONTRIBUTORS" populated from `detail.topContributors` (server data: name, detail, value).

**Test:** On Shared Value tab, tap "Loans" → title says "Loans Value Saved", hero value loaded from server, first table is "MOST VALUABLE ITEMS" with server-provided items, second table is "TOP CONTRIBUTORS" with server-provided members. Repeat for other categories and metric tabs — each shows correct title and server data. No regressions when opening the detail screen without a category (generic dimension view).

### Phase 13: Dollar Value Tab Redesign — Charts, Top Row, and Detail Screens

**Goal:** Fix the Shared Value (dollar) metrics tab: improve chart readability, replace the top-row `$ Saved` metric with `# Actions`, add an Events breakdown row, and make every tappable element open a purpose-built detail screen with correct title, hero value, and tabular data.

**Problem 1 — Chart has no value labels and skips interim months:**
The cumulative chart on the Shared Value tab shows a line but no Y-axis values, making it impossible to gauge amounts. It also jumps from the first data point to the last, omitting intermediate months.

**Fix:**
- Add Y-axis value labels to the chart (e.g., "$0", "$5K", "$10K") so the user can read amounts at a glance.
- Show all interim months between the first and last data points on the X-axis, even months with no new activity (carry forward the cumulative value).

**Problem 2 — Top row shows wrong third metric:**
The top row currently shows Library Value, Utilization Rate, and $ Saved. The `$ Saved` metric is redundant (it's the hero value). Replace it with **# of Actions** — the total count of completed loans + giveaways + requests.

**Fix:**
- Top row becomes: **Library Value** | **Utilization** | **Actions**
- Actions = completed loans + completed giveaways + completed requests.

**Problem 3 — Detail screens show generic data:**
Tapping any top-row metric or breakdown row opens a detail screen with incorrect or generic content. Each detail screen must show a specific title, hero value, and tailored tabular data.

**Detail screen specifications:**

| Tapped Element | Title | Hero Value | Table Data | Sort |
|---|---|---|---|---|
| **Library Value** (top row) | Library Value | Total value of all items in the library | Most valuable items (name, value, times loaned) | By value descending |
| **Actions** (top row) | Community Actions | Total # of actions (loans + giveaways + requests completed) | All actions with value saved per action | Chronological, most recent first |
| **Loans** (breakdown row) | Loans Value Saved | Total value saved through loans (not total $ saved) | Most valuable loans (name, value saved) | By value saved descending |
| **Giveaways** (breakdown row) | Giveaways Value Saved | Total value saved through giveaways (not total $ saved) | Most valuable giveaways (name, value saved) | By value saved descending |
| **Requests** (breakdown row) | Requests Value Saved | Total value saved through requests (not total $ saved) | Most valuable requests (name, value saved) | By value saved descending |
| **Events** (new breakdown row) | Events Value Saved | Total value saved through Experiences | Most valuable Experiences (name, value saved) | By value saved descending |

**Problem 4 — No Events breakdown row:**
The Shared Value tab shows breakdown rows for Loans, Giveaways, and Requests but omits Experiences (Events). Add an **Events** row beneath Requests showing the total value saved via Experiences.

**Files to modify:**
- `app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart` — Replace `$ Saved` top-row metric with `Actions`; add Events breakdown row; update tap handlers for all elements to pass correct detail screen parameters.
- `app/lib/presentation/screens/impact_metrics/metric_detail_screen.dart` — Support new detail types (Library Value with times-loaned column, Community Actions with chronological sort, Events category). Update title resolution and table rendering logic.
- `app/lib/presentation/widgets/impact/community/impact_chart_card.dart` — Add Y-axis value labels; render all interim months on X-axis.

**Data requirements:**
- Library value (sum of all item values) and per-item value + loan count — may need new fields on `GetCommunityMetricDetailResponse` or a new RPC.
- Actions count (loans + giveaways + requests completed) — can be computed client-side from existing breakdown data.
- Events/Experiences breakdown — may need extending `CommunityImpactMetrics` to include an experiences category alongside loans/giveaways/requests.
- Chart interim months — client-side interpolation of `cumulative_trend` data to fill gaps.

**Test:** On Shared Value tab, top row shows "Library Value", "Utilization", and "Actions" (not $ Saved). Chart displays Y-axis value labels and all interim months. Tapping Library Value opens "Library Value" detail with item table sorted by value showing times loaned. Tapping Actions opens "Community Actions" with chronological action list. Tapping Loans/Giveaways/Requests opens category-specific "X Value Saved" detail. Events row appears beneath Requests; tapping it opens "Events Value Saved" detail. No regressions on other metric tabs.

---

**Last Updated:** 2026-03-10
**Status:** Design document — Phases 9, 12, 13, and 13b implemented, Phases 10–11 and 14 documented (not implemented)

### Phase 13 Implementation Notes

**Files changed:**
- [metric_detail_screen.dart](../../app/lib/presentation/screens/impact_metrics/metric_detail_screen.dart) — Replaced monolithic `_buildTopItemsRows`/`_buildTopContributorRows` with `_buildCategoryTableContent` dispatcher that routes to category-specific table builders: `_buildLibraryValueTable` (items sorted by value with loan count), `_buildActionsTable` (source breakdown as rows), `_buildCategoryFilteredTable` (category-specific top items or summary). Hero values now use real server `sourceBreakdown` data instead of fake percentages. Trend text is category-aware via `_trendTextForCategory`. Chart title varies by category via `_chartTitleForCategory`. Chart hidden for Actions category.
- [community_metrics_screen.dart](../../app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart) — `_buildBreakdownSection` now accepts `detailAsync` parameter and uses real server `sourceBreakdown.formatted` values instead of hardcoded percentage multipliers (0.8, 0.15, 0.05).
- [impact_service.dart](../../app/lib/services/impact_service.dart) — Added `SourceBreakdown` to exports.
- [impact_repository.dart](../../app/lib/data/repositories/impact_repository.dart) — Added `SourceBreakdown` to re-exports.

### Phase 13b: Per-Category Item Data and Tap Navigation (implemented)

**Problem:** The detail screens for Loans, Giveaways, Requests, and Events all showed the exact same two items because the server's `computeTopItems` only processed transfers (loans+giveaways combined), limited output to 2 items, and had no source type tagging. The client had no way to filter items by category.

**Root cause:** Server-side, not client-side. `computeTopItems` grouped ALL transfers by gear, applied `LIMIT 2`, and returned the same list to every category detail screen.

**Fix — Proto:**
- Added `source_type` field (string, field 9) to `TopItem` message in `impact_service.proto`. Values: `"Loans"`, `"Giveaways"`, `"Requests"`, `"Events"`.

**Fix — Server (`metric_detail_rankings.go`):**
- Rewrote `computeTopItems` to accept transfers, requests, AND experiences (was transfers only).
- Tags every item with `SourceType` based on transfer type, request, or experience.
- Keys transfer stats by `gearId:sourceType` so the same gear can appear under both Loans and Giveaways.
- Removed the `LIMIT 2` — returns ALL items sorted by cumulative value descending.
- Requests are included as TopItems (using request title as name, helper as sharer).
- Events/experiences are included as TopItems (using experience name, organizer as sharer).
- Added `parseDollarValue` helper to sort formatted value strings numerically.

**Fix — Server (`metric_detail_calculator.go`):**
- Updated `computeTopItems` call to pass `requests` and `experiences` in addition to `transfers`.

**Fix — Client (`metric_detail_screen.dart`):**
- `_buildCategoryFilteredTable` now filters `detail.topItems` by `item.sourceType == categoryLabel` — each category shows only its own items.
- Removed all `_buildGenericTopContributorRows` calls and deleted the method entirely — no "Top Contributors" sections on any category detail screen.
- Added `gearId` field to `_DetailRow` data class and a chevron indicator + `GestureDetector` to `_DetailRowWidget`. Tapping a row with a `gearId` navigates to the gear detail screen via `NavigationHelpers.pushToItemScreen`.
- Library Value, category-filtered, and generic rows all pass `gearId` for tap navigation.

**Fix — Anchoring metaphor:**
- Changed money hook from "season passes" ($1,200 each) to "cups of coffee" ($5.50 each, BLS 2024 average).
- Updated `impact_hook.dart` `_moneyHook` and `community_metrics_screen.dart` description card.

**Files changed:**
- [impact_service.proto](../../proto/ripls/api/impact_service.proto) — Added `source_type = 9` to `TopItem`.
- [metric_detail_rankings.go](../../server/impact_metrics/metric_detail_rankings.go) — Rewrote `computeTopItems` (new signature: transfers, requests, experiences, dimension, cache). Added `parseDollarValue`. Added `strings`/`strconv` imports.
- [metric_detail_calculator.go](../../server/impact_metrics/metric_detail_calculator.go) — Updated `computeTopItems` call site.
- [metric_detail_rankings_test.go](../../server/impact_metrics/metric_detail_rankings_test.go) — Updated test calls to match new signature; adjusted `TestTopItemsLimit` to expect all items (no limit) and verify `SourceType` tagging.
- [metric_detail_screen.dart](../../app/lib/presentation/screens/impact_metrics/metric_detail_screen.dart) — Category filtering by `sourceType`, tap-to-navigate via `gearId`, removed all top contributors.
- [impact_hook.dart](../../app/lib/presentation/widgets/impact/community/impact_hook.dart) — Coffee metaphor.
- [community_metrics_screen.dart](../../app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart) — Updated description card stat text.

**Test:** On Shared Value tab, tap "Loans" → detail shows only loan items sorted by value saved. Tap "Giveaways" → shows only giveaway items. Tap "Requests" → shows only request items. Tap "Events" → shows only event items. Each row is tappable (chevron indicator) and navigates to the gear detail screen. Item values within each category add up to the hero total. No "Top Contributors" sections anywhere. Coffee metaphor displays on the money hook.

### Phase 14: Apply Per-Category Detail Screens to All Metric Tabs

**Goal:** The Phase 13/13b fixes (per-category item data, tap navigation, no top contributors) currently work on the Shared Value (money) tab. Apply the same pattern to the other three metric tabs: Rich Minutes (quality time), CO₂ Avoided (emissions), and Time Recovered (time). Each tab's category detail screens should show category-filtered items with the correct metric-specific values.

**Current state:** The server already returns `topItems` with `sourceType` tags for ALL dimensions (money, time, emissions, quality_time) — the `computeTopItems` function processes all transfers/requests/experiences regardless of dimension and formats values using `formatValue(value, dimension)`. The client already filters by `sourceType` in `_buildCategoryFilteredTable`. So the plumbing is in place — the remaining work is ensuring each tab's breakdown rows are tappable and that the detail screens render correctly for non-money dimensions.

**What needs to happen:**

1. **Verify breakdown rows are tappable on all tabs.** Currently `_buildBreakdownSection` on `community_metrics_screen.dart` navigates to `MetricDetailScreen.push()` with the current `dimension` and `category`. Confirm this works for all four tabs, not just money.

2. **Verify hero values use `sourceBreakdown` for non-money dimensions.** The `_heroValueFor` method in `metric_detail_screen.dart` already looks up `sourceBreakdown` by category label. Confirm the server's `sourceBreakdown` labels match across dimensions. Note: the Time dimension uses different labels ("Loan time", "Help requests", "Event coord.") — these won't match the category strings "Loans", "Requests", "Events" used by the client. Fix the label mismatch.

3. **Fix Time dimension `sourceBreakdown` labels.** The server's `buildBreakdown` in `metric_detail_calculator.go` uses special labels for the Time dimension: `"Loan time"`, `"Help requests"`, `"Event coord."`. These don't match the `sourceType` tags on `topItems` (`"Loans"`, `"Requests"`, `"Events"`). Either:
   - (a) Standardize server labels to always use "Loans", "Giveaways", "Requests", "Events" and handle display formatting on the client, OR
   - (b) Map the Time dimension labels to standard category strings on the client side.
   - Recommendation: Option (a) — use consistent labels everywhere and let the client format for display.

4. **Verify `_categoryTableTitle` works for non-money dimensions.** Currently returns "MOST VALUABLE LOANS" etc. For CO₂ it should say "MOST CO₂ AVOIDED BY LOANS", for Time "MOST TIME RECOVERED BY LOANS", for Quality Time "MOST TIME SHARED THROUGH LOANS". Update `_categoryTableTitle` to accept the dimension.

5. **Verify item values display correctly for non-money dimensions.** The server already formats `cumulativeSaved` per dimension (e.g., "2.5 hrs", "45 kg", "12.3 min"). The client displays `item.cumulativeSaved` as-is. This should already work — verify with real data.

6. **Verify chart titles per dimension × category.** `_chartTitleForCategory` already handles all category × dimension combinations. Verify it produces sensible titles like "LOAN CO₂ AVOIDED OVER TIME".

**Files to modify:**
- `server/impact_metrics/metric_detail_calculator.go` — Standardize `buildBreakdown` labels for Time dimension to use "Loans", "Requests", "Events" (matching `sourceType` tags).
- `app/lib/presentation/screens/impact_metrics/metric_detail_screen.dart` — Update `_categoryTableTitle` to produce dimension-aware section headers. Verify hero values and item display for non-money dimensions.
- `app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart` — Verify breakdown row tap handlers pass correct dimension and category for all four tabs.

**Test:** On each of the four metric tabs (Shared Value, Rich Minutes, CO₂ Avoided, Time Recovered):
- Tapping a breakdown row (Loans, Giveaways, Requests, Events) opens a detail screen with the correct title, hero value, and filtered item list.
- Item values are displayed in the correct unit (dollars, minutes, kg CO₂, hours).
- Item values within each category add up to the hero total.
- Items are tappable and navigate to the gear detail screen.
- No regressions on the Shared Value tab (already working from Phase 13b).

### Phase 15: Fix Quality Time (Rich Minutes) Metric Detail Screen

**Goal:** The Rich Minutes tab's detail screens should match the layout and conventions of the other metric tabs (Shared Value, CO₂ Avoided, Time Recovered). Currently the Quality Time category labels, members screen layout, hook text, and value formatting are inconsistent.

**What needs to happen:**

1. **Standardize category labels.** The Rich Minutes breakdown currently shows labels like "From Loans", "From Experiences", etc. Change these to match the standard labels used by the other tabs: "Loans", "Giveaways", "Requests", "Events".

2. **Fix members screen layout.** The members detail screen for Rich Minutes should match the layout used by the other deep metrics screens (same table structure, same row formatting, same navigation behavior).

3. **Update the hook text.** Change the Rich Minutes anchoring hook to: "That's ~XX hours you were fully present with people and not with a screen."

4. **Add space between value and units in hero metric.** The top-level hero value currently renders as e.g. "58mins" — add a space so it reads "58 mins". Check all metric tabs for this formatting issue and fix consistently.

**Files to modify:**
- `app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart` — Fix category labels for Rich Minutes breakdown rows.
- `app/lib/presentation/screens/impact_metrics/metric_detail_screen.dart` — Fix members screen layout to match other detail screens.
- `server/impact_metrics/metric_detail_calculator.go` — Fix breakdown labels and/or value formatting for quality_time dimension.
- `app/lib/presentation/widgets/impact/community/impact_hook.dart` — Update Rich Minutes hook text.

**Test:** On the Rich Minutes tab:
- Breakdown rows show "Loans", "Giveaways", "Requests", "Events" (not "From Loans", etc.).
- Tapping a breakdown row opens a detail screen matching the layout of other metric detail screens.
- Members screen matches the layout of members screens on other tabs.
- Hook text reads "That's ~XX hours you were fully present with people and not with a screen."
- Hero metric displays "58 mins" (with space), not "58mins".
- No regressions on other metric tabs.

### Phase 16: Fix Time Recovered Metric Detail Screen

**Goal:** The Time Recovered tab's detail screens should match the layout and conventions of the other metric tabs (Shared Value, CO₂ Avoided, Rich Minutes). Currently the Time Recovered category labels, members screen layout, and hook text are inconsistent.

**What needs to happen:**

1. **Standardize category labels.** The Time Recovered breakdown currently shows labels like "From Loans", "From Experiences", etc. Change these to match the standard labels used by the other tabs: "Loans", "Giveaways", "Requests", "Events".

2. **Fix members screen layout.** The members detail screen for Time Recovered should match the layout used by the other deep metrics screens (same table structure, same row formatting, same navigation behavior).

3. **Update the hook text.** Change the Time Recovered anchoring hook to: "That's like getting ~XX additional vacation days from work or school."

**Files to modify:**
- `app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart` — Fix category labels for Time Recovered breakdown rows.
- `app/lib/presentation/screens/impact_metrics/metric_detail_screen.dart` — Fix members screen layout to match other detail screens.
- `server/impact_metrics/metric_detail_calculator.go` — Fix breakdown labels and/or value formatting for time dimension.
- `app/lib/presentation/widgets/impact/community/impact_hook.dart` — Update Time Recovered hook text.

**Test:** On the Time Recovered tab:
- Breakdown rows show "Loans", "Giveaways", "Requests", "Events" (not "From Loans", etc.).
- Tapping a breakdown row opens a detail screen matching the layout of other metric detail screens.
- Members screen matches the layout of members screens on other tabs.
- Hook text reads "That's like getting ~XX additional vacation days from work or school."
- No regressions on other metric tabs.

### Phase 17: Fix Tab-Switch Flicker on First Visit

**Goal:** Eliminate the visual flicker where tapping a non-money metric tab (Rich Minutes, CO₂ Avoided, Time Recovered) momentarily snaps the UI back to the Shared Value ($) tab before settling on the correct tab.

**Problem:** When a user logs in and first navigates to the community metrics screen, tapping a metric tab triggers `selectMetric()` in `CommunityImpactMetricDetailNotifier`. If the tapped dimension's detail data is not yet cached, `selectMetric()` sets `state = const AsyncLoading()` (line 65 of `community_metric_detail_view_model.dart`). This replaces the entire `AsyncData(...)` with a bare `AsyncLoading` that carries no `selectedMetric` field. Meanwhile, `community_metrics_screen.dart` resolves `selectedMetric` via `detailAsync.when(loading: () => IMPACT_METRIC_DIMENSION_MONEY)` (line 143) — so during the loading interval the screen reads the default money dimension, causing the hero value, tab bar highlight, and content area to all flash back to Shared Value before the fetch completes and the correct tab renders.

**Root cause:** Two independent issues compound:
1. **Notifier loses selected tab during fetch.** `selectMetric()` overwrites the entire state with `const AsyncLoading()`, discarding the `selectedMetric` the user just chose.
2. **Screen falls back to money on loading.** The `when(loading: ...)` callback hardcodes `IMPACT_METRIC_DIMENSION_MONEY` instead of preserving the previously known tab.

**Possible solutions:**

1. **Option A — Use `AsyncValue.guard` with optimistic `selectedMetric` update.** Before fetching, immediately emit `AsyncData(current.copyWith(selectedMetric: metric))` so the UI reflects the new tab instantly. Then fetch data in the background and emit again with the loaded detail. Downside: two rapid state emissions; content area shows stale/empty data for the new dimension during the fetch.

2. **Option B — Replace `const AsyncLoading()` with Riverpod's `AsyncLoading` that preserves previous value.** Riverpod's `AsyncValue` supports `isLoading` with a previous value via `AsyncData(...).copyWith(isLoading: true)` or by using `state = AsyncLoading<T>().copyWithPrevious(state)`. This keeps the old state accessible via `detailAsync.valueOrNull` during loading, and the screen can read `selectedMetric` from it. Downside: requires the screen to use `valueOrNull` or `when(skipLoadingOnReload: true)` to avoid the loading fallback.

3. **Option C — Track `selectedMetric` as separate local state in the screen widget.** Store the selected tab index as a `useState` / local `StateProvider` independent of the async notifier. The tab bar updates immediately on tap; the content area shows a loading indicator while data fetches. Downside: splits the "selected tab" concept across two locations (screen state + notifier state), creating a sync risk.

4. **Option D — Combine A and B.** Optimistically update `selectedMetric` first (Option A), then use `copyWithPrevious` (Option B) for the loading phase so the screen never sees a bare `AsyncLoading`. This gives instant tab feedback AND preserves the previous content during fetch.

**Recommendation: Option D** — it is the most robust and idiomatic Riverpod approach. The `selectMetric` method should:
1. Immediately emit `AsyncData(current.copyWith(selectedMetric: metric))` so the tab bar and hero update instantly.
2. Then emit a loading state that preserves the previous value (e.g., `state = const AsyncLoading<CommunityMetricDetailState>().copyWithPrevious(state)`) so the screen can still read `selectedMetric` via `valueOrNull`.
3. On the screen side, replace `loading: () => MONEY` with `loading: () => detailAsync.valueOrNull?.selectedMetric ?? MONEY` so even if the previous value leaks through, it's the correct tab.

**Files to modify:**
- `app/lib/presentation/viewmodels/community_metric_detail_view_model.dart` — In `selectMetric()`, replace `state = const AsyncLoading()` with optimistic `selectedMetric` update + `copyWithPrevious` loading state.
- `app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart` — Replace `loading: () => IMPACT_METRIC_DIMENSION_MONEY` with `loading: () => detailAsync.valueOrNull?.selectedMetric ?? IMPACT_METRIC_DIMENSION_MONEY`.

**Test:** On the community metrics screen after a fresh login (cold cache):
- Tap Rich Minutes tab — tab bar highlights immediately with no flicker back to Shared Value.
- Tap CO₂ Avoided tab — same: no momentary snap to money.
- Tap Time Recovered tab — same.
- Content area may show a brief loading indicator while data fetches, but the hero value, tab highlight, and hook text never revert to the money dimension.
- Rapid tab switching (tap CO₂ then immediately tap Time) settles on the last-tapped tab with no intermediate flicker.
- No regressions when tabs are already cached (second tap should be instant as before).
