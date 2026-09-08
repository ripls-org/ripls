# Experience Widgets

Reusable widgets specific to the experience detail view and its sub-flows.

## Purpose

These components are used inside `ExperienceContentView` and the experience-related modals. They handle attendee display, scheduling, and needs management.

## Key Files

### Attendees
- **`attendee_list_sheet.dart`** — bottom sheet listing all attendees with their RSVP status.

### Scheduling and time
- **`time_proposal_tile.dart`** — a tile for a single proposed time in the vote list.
- **`propose_time_form.dart`** — inline form for proposing new time slots.
- **`duration_picker_modal.dart`** — modal for selecting event duration.

### Needs
- **`experience_needs_sheets.dart`** — bottom sheets for adding, editing, and viewing experience needs (bring list, contributions).

## Subdirectories

| Directory | Contents |
|-----------|----------|
| `compose/` | The experience composer's sections and sheets |
| `needs/` | Experience-scoped needs sheets and their shared sheet chrome |

## When to add here vs. elsewhere

Experience-specific widgets belong here. Needs primitives shared with the request flow live in `widgets/shared/needs/`, and the needs flow itself is in `widgets/needs/`. General content-view building blocks (owner row, facts row) are in `widgets/content/`.
