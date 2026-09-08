# core/utils

A collection of focused utility files used across the app. This is intentionally a grab-bag — each file covers one narrow concern.

## Representative files

| File | Purpose |
|---|---|
| `community_resolver.dart` | Validates that a supplied community id is in a response's `sharedCommunities`, falling back to the first shared community |
| `savings_formatter.dart` | Converts `ImpactEstimate` proto to display-ready strings |
| `date_time_formatter.dart` | Locale-aware date and time formatting |
| `gear_metadata_formatter.dart` | Formats gear attributes (weight, material, provenance) for display |
| `rpc_utils.dart` | Header building and RPC execution helpers shared by services |
| `media_helpers.dart`, `media_picker_helper.dart`, `media_upload_helper.dart` | Media selection, upload, and display utilities |
| `location_formatter.dart`, `location_permission_helper.dart` | Location string formatting and permission handling |
| `gear_helper.dart`, `request_helpers.dart`, `experience_helper.dart` | Domain-object helpers (label, icon, state predicates) for each entity type |
| `map_helper.dart`, `discover_map_helper.dart` | Map tile and marker utilities |
| `safe_notifier.dart` | Riverpod `Notifier` mixin that guards `state` writes after disposal |
| `responsive.dart` | Screen-size breakpoints |
| `ripls_icons.dart` | App-specific icon constants |
| `image_cache_keys.dart` | Stable cache key generation for `CachedNetworkImage` |
| `video_cache_helper.dart`, `video_mute_helper.dart` | Video playback helpers |
| `navigation_helpers.dart`, `toast_helper.dart` | Imperative navigation and toast display |

## When to add a new utility file

Add a new file when the utility is used in more than one screen or widget and does not belong to a more specific layer (`services/`, `data/`, `core/theme/`). Keep each file focused on a single domain (formatting, navigation, a single entity type). If a helper is used in only one place, keep it in that file until it is reused.

Avoid putting stateful objects or Riverpod providers in this directory — those belong in `services/` or a viewmodel.
