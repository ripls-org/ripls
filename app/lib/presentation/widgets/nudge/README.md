# Nudge Widgets

Nudge cards that prompt users to take sharing actions.

## Purpose

Nudges are server-sent prompts that appear in the feed encouraging users to add gear, create requests, or start experiences. These widgets render the nudge as a card with background media and a call-to-action button — full-screen in the feed, or bounded by a host (the calendar's open day, the Home zero state).

## Key Files

- **`nudge_content_view.dart`** — resolves the nudge's background media and stock-photo attribution, and dispatches to the correct card variant based on the nudge type. CTA taps open the matching creation modal without dismissing the nudge card.
- **`nudge_card_variants.dart`** — builder functions for each nudge variant (add-gear, request, experience) producing the appropriate card layout with title, body, and CTA label. The variants are `Stack(fit: StackFit.expand)` compositions, so they cannot self-size — every host bounds them — and they fit their copy's line count to the height they are given.
- **`nudge_presentation.dart`** — `NudgePresentation`, the full-screen vs. embedded geometry switch.

## When to add here vs. elsewhere

Nudge-specific display widgets belong here. The creation modals opened by nudge CTAs live in `screens/<feature>/`. Feed rendering logic that decides when to show a nudge is in the feed viewmodel.
