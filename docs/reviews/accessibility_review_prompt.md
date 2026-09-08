---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Accessibility review checklist — semantic labels, screen-reader support, touch targets, focus/navigation, forms, media, motion. Follow its "Areas to Review" section.
  triggers: [accessibility, a11y, semantics, screen-reader, touch-target, focus, contrast]
  lens: [accessibility]
  globs: [app/lib/presentation/**]
  alwaysApply: true
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Accessibility Review Prompt

Run this review with Claude Code CLI:
```bash
claude -p "$(cat docs/reviews/accessibility_review_prompt.md)"
```

Or use the slash command: `/accessibility-review`

---

## Instructions

Perform a comprehensive accessibility (a11y) review of the Flutter application. Write your findings to `docs/reviews/reports/accessibility_review.md`.

## Step 0. Review Previous Report (If Exists)

Before starting the review, check if a previous report exists:

```bash
cat docs/reviews/reports/accessibility_review.md 2>/dev/null || echo "No previous report"
```

**If a previous report exists:**
1. Note the date and all findings from the previous report
2. Track which issues were identified previously
3. In your new report, include a "Changes Since Last Review" section that highlights:
   - Issues that have been remediated
   - Issues that remain unaddressed
   - New issues discovered
   - Accessibility improvements made
   - Overall trend (improving/stable/declining)

## Areas to Review

### 1. Semantic Labels (app/lib/)

- **Semantics Widgets**: Check for proper use of `Semantics` widget
- **Label Properties**: Verify `semanticsLabel` on Images, Icons, Buttons
- **Meaningful Labels**: Check labels are descriptive (not "button 1")
- **Dynamic Content**: Verify dynamically loaded content has labels

### 2. Screen Reader Support

- **Reading Order**: Check logical reading order matches visual order
- **Headings**: Verify proper heading hierarchy for navigation
- **Live Regions**: Check for `liveRegion` on dynamic content updates
- **Focus Management**: Verify focus moves logically after interactions

### 3. Touch Targets

- **Minimum Size**: Check touch targets are at least 48x48 dp
- **Spacing**: Verify adequate spacing between interactive elements
- **Hit Areas**: Check if small icons have expanded hit areas

### 4. Visual Accessibility

- **Color Contrast**: Check text contrast ratios (4.5:1 for normal, 3:1 for large)
- **Color Independence**: Verify info isn't conveyed by color alone
- **Text Scaling**: Check UI handles large text sizes (up to 200%)
- **Dark Mode**: Verify dark mode maintains accessibility

### 5. Navigation & Focus

- **Keyboard Navigation**: Check all interactive elements are reachable
- **Focus Indicators**: Verify visible focus indicators exist
- **Skip Links**: Check for skip navigation options on complex screens
- **Back Navigation**: Verify back button behavior is predictable

### 6. Forms & Input

- **Label Association**: Check form fields have associated labels
- **Error Identification**: Verify errors are announced to screen readers
- **Input Assistance**: Check for helpful hints and auto-complete
- **Required Fields**: Verify required fields are marked accessibly

### 7. Media & Images

- **Alt Text**: Check all meaningful images have alt text
- **Decorative Images**: Verify decorative images are hidden from a11y tree
- **Video Captions**: Check for caption support on videos
- **Audio Descriptions**: Verify complex visuals have descriptions

### 8. Motion & Animation

- **Reduced Motion**: Check for `reduceMotion` media query support
- **Auto-Playing**: Verify animations can be paused
- **Flashing Content**: Check for content that flashes (seizure risk)

## Output Format

Write `docs/reviews/reports/accessibility_review.md` with:

```markdown
# Accessibility Review - [DATE]

## Executive Summary
[1-2 paragraph overview of accessibility posture]

## Changes Since Last Review

> *If no previous report exists, write "This is the first accessibility review."*

**Previous Review Date**: [DATE or N/A]

**Remediated Issues:**
- ✅ [Issue from previous report that has been fixed]
  - Location: [file:line]
  - Fixed in: [commit/PR reference if known]

**Unaddressed Issues:**
- ⚠️ [Issue still present from previous report]
  - Original Priority: [Critical/High/Medium/Low]
  - Current Priority: [May escalate if aging]
  - Days Open: [X days]

**New Issues:**
- 🆕 [Issue not present in previous report]

**Improvements:**
- [Accessibility improvements since last review]

**Trend**: [Improving / Stable / Declining] - [Brief explanation]

---

## WCAG Compliance Summary

| Guideline | Level A | Level AA | Notes |
|-----------|---------|----------|-------|
| Perceivable | ✅/❌ | ✅/❌ | ... |
| Operable | ✅/❌ | ✅/❌ | ... |
| Understandable | ✅/❌ | ✅/❌ | ... |
| Robust | ✅/❌ | ✅/❌ | ... |

## Critical Issues
[Barriers preventing access for users with disabilities]

## High Priority
[Significant accessibility barriers]

## Medium Priority
[Issues affecting usability for some users]

## Low Priority
[Enhancements and best practices]

## Positive Findings
[Accessible patterns worth highlighting]

## Recommendations
[Prioritized action items]
```

For each finding include:
- **Location**: File path and line number
- **Description**: What the issue is
- **WCAG Criteria**: Which WCAG guideline it violates
- **Impact**: Who is affected and how
- **Recommendation**: How to fix it with code examples
