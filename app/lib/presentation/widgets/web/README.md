# Web-only UI helpers

Widgets and helpers that only fire on Flutter Web. The web bundle
is served at the apex domain (`https://ripls.app`,
`https://dev.ripls.app`) as of #2157; before that, only a guest-
only event-landing surface was reachable.

## Files

- **`web_unsupported.dart`** — `WebUnsupported` static helpers that
  surface localized "open the mobile app" snackbars when a web
  visitor taps a control whose mobile plugin has no web
  implementation. Each helper is `kIsWeb`-gated and returns
  `bool` for short-circuit:

  ```dart
  if (WebUnsupported.showCreateNotice(context)) return;
  // …mobile flow that uses camera / image_picker / gal / geolocator…
  ```

  The full list of guarded entry points (and the underlying
  plugins that drove each guard) lives in
  `app/web/README.md` § "Web-unsupported features".

## When to add code here

- A new control on a screen the web bundle exposes calls a plugin
  with no web implementation — guard at the entry point with a
  `WebUnsupported.show*Notice` and add a row to the table in
  `app/web/README.md`. If the existing notice categories
  (create / calendar / photo upload) don't fit the new feature,
  add a new helper here plus matching ARB strings.

- A new visual placeholder for a feature that's silently degraded
  on web (vs. a snackbar after a user action) lives elsewhere —
  see `presentation/widgets/map/web_map_placeholder.dart` for the
  reference pattern.
