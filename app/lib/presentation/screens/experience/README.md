# Experience Screens

Screens and modals for viewing, creating, and managing community experiences.

## Purpose

Experiences are scheduled or ongoing community activities. This directory contains the full experience surface: the detail screen, creation flow, RSVP, scheduling, and completion modals.

## Key Files

### Detail
- **`experience_screen.dart`** — entry point; slides in from the right and hosts `ExperienceContentView`.
- **`experience_content_view.dart`** — scrollable body: media, description, attendees, needs, action bar, and chat.
- **`experience_preview_modal.dart`** — read-only preview card before an experience is joined.

### Creation
- **`experience_creation_modal.dart`** — AI-assisted creation bottom sheet (text or image input).

### Scheduling
- **`unified_time_modal.dart`** — unified entry point for all time-related actions; routes to propose, vote, or confirm sub-modals.
- **`time_poll_propose_modal.dart`** — propose candidate time slots.
- **`time_poll_vote_modal.dart`** — vote on proposed slots.
- **`time_poll_confirm_modal.dart`** — organizer confirms a winning slot.

### Lifecycle
- **`rsvp_modal.dart`** — RSVP intent selection (going, maybe, not going).
- **`event_joiner_modal.dart`** — step-by-step joining flow for new attendees.
- **`event_organizer_modal.dart`** — organizer controls (start, cancel, complete).
- **`mark_completed_modal.dart`** — mark an experience as completed with attendee confirmation.
- **`cancel_event_modal.dart`** — cancel an experience with reason selection.

## When to add here vs. elsewhere

Experience-specific screens and modals belong here. Reusable experience widgets (timeline, needs sheets, attendee list) live in `widgets/experience/`. Impact screens for experiences are in `screens/impact_metrics/`.
