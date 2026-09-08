# Location Widgets

Location search, autocomplete, and picker components.

## Purpose

These widgets let users search for and select a location (pickup point, event venue, home base) using geocoding-backed autocomplete. They are used wherever the app needs a location from the user.

## Key Files

- **`location_picker_modal.dart`** — full-featured bottom sheet for selecting a location. Combines autocomplete search, "Use my current location" option, and a saved-locations list. Returns a `Location` or `GeocodedLocation` on confirm.
- **`location_autocomplete_field.dart`** — text field with debounced geocoding suggestions dropdown. Can be embedded independently when the full modal is not needed.
- **`location_modal_widgets.dart`** — shared sub-widgets used inside the picker modal (location list tile, section header, saved-location row).

## When to add here vs. elsewhere

Location UI belongs here. Geocoding API calls and location service logic live in `services/mapbox_location_service.dart`. Location formatting utilities are in `core/utils/location_formatter.dart`.
