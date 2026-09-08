# Request Widgets

Reusable widgets specific to the request compose flow.

## Purpose

Everything under here belongs to composing a request. The request detail
view itself is assembled from the shared content building blocks; only
the compose surface needs request-specific widgets.

## Subdirectories

| Directory | Contents |
|-----------|----------|
| `compose/` | `request_compose_sheet.dart` — the compose bottom sheet — and `request_compose_section.dart`, the section it embeds in a host screen |

## When to add here vs. elsewhere

Request-specific widgets belong here. Needs primitives shared with the
experience flow live in `widgets/shared/needs/`, and the needs flow
itself is in `widgets/needs/`. General content building blocks (owner
row, facts row) are in `widgets/content/`.
