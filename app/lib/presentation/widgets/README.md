# Widgets

Reusable UI components shared across two or more screens.

## Purpose

This directory and its subdirectories hold widgets that are not specific to a single screen. Top-level files here are components used broadly across the app without belonging to a single feature group.

## Top-Level Files (selected)

- **`user_avatar.dart`** — round avatar with fallback initials for any user.
- **`community_avatar.dart`** — community logo display.
- **`group_avatar.dart`** — stacked avatars representing a group of people.
- **`empty_content_state.dart`** — standard empty-state placeholder widget.
- **`server_error_screen.dart`** — full-screen error view for unrecoverable server errors.
- **`unread_badge.dart`** — red count badge overlaid on icons and avatars.
- **`plus_button_modal.dart`** — floating action button that presents the creation type picker.
- **`oidc_sign_in_buttons.dart`** — sign-in buttons for OIDC auth providers.
- **`environment_badge.dart`** — dev/staging badge shown in non-production builds.
- **`connectivity_banner.dart`** — offline notice banner.
- **`font_fallback_warmup.dart`** — offstage text that pre-downloads the web engine's lazy emoji/symbol fallback fonts during the splash screen.
- **`swipe_to_close_mixin.dart`** / **`swipe_to_close_wrapper.dart`** — gesture-based modal dismiss.
- **`magazine_thumbnail.dart`** — photo card thumbnail used in discover and feed lists.
- **`profile_menu_avatar.dart`** / **`profile_menu_modal.dart`** — the avatar that opens the account menu, and the menu itself.
- **`provisional_user_avatar.dart`** / **`provisional_user_profile_sheet.dart`** — avatar and profile sheet for a not-yet-registered person.

## Subdirectories

| Directory | Contents |
|-----------|----------|
| `accessibility/` | The accessibility primitives every interactive surface composes: `Tappable`, `Toggle`, `IconAction`, `accessibleDuration`, `showAccessibleModal`, semantic announcer |
| `adaptive/` | Platform/form-factor wrappers (auth screen wrapper) |
| `chat/` | Message list, conversation chrome, mentions, and reactions |
| `community/` | Community member preview row |
| `completion/` | Dark-themed person-search and confirmation widgets for transfer completion flows |
| `content/` | Shared content-view building blocks used by gear, request, experience, and community screens |
| `create/` | The unified-create surface's pills, toggles, sheets, and preview card |
| `creation/` | Reusable creation modal components (input toggle, camera viewport, text area) |
| `discover/` | Library shelf tiles, location anchor sheet, map style sheet, gallery overlay |
| `experience/` | Experience-specific widgets (attendee list, time proposal, needs sheets) |
| `feedback/` | User feedback sheet |
| `gear/` | Gear-specific widgets (metadata sheet, borrower/interest sheets) |
| `home/` | Home tab section widgets rendering `GetHomeViewResponse` |
| `impact/` | Impact display widgets (hero, formula and provenance breakdowns, loan history) |
| `item/` | Item-scoped metric view data |
| `location/` | Location autocomplete field and picker modal |
| `media/` | Media carousel, background image, picker button and dialog |
| `modal/` | Modal scaffolding utilities and the `glass/` frosted-glass primitives |
| `navigation/` | Bottom nav dock and its destinations |
| `needs/` | The needs flow: propose / picker / volunteer / archived sheets, claim rows and chips |
| `nudge/` | Nudge card variants and full-screen nudge view |
| `observability/` | Analytics consent dialog |
| `planning/` | Poll banner shown on planning surfaces |
| `poll/` | Poll option widgets, flexible vote row, TBD card, manage menu |
| `profile_section/` | The profile surface: hero, chips, metric rows, members panel, conversation panel |
| `request/` | Request-specific widgets (compose sheet and sections) |
| `search/` | Search scope and nearby pills, close button |
| `settings/` | Settings screen building blocks (section header, card container, pickers) |
| `shared/` | Needs primitives shared by the needs flow and the experience batch sheets |
| `sharing/` | Community picker and item share sheet |
| `story/` | Story text resolver and embedded ESM view |
| `transfer/` | Transfer modal shell, phase bar, and step checklist widgets |
| `weather/` | Weather chip |
| `web/` | Web-only unsupported-platform placeholder |
| `workshop/` | Workshop overview, switcher, morph transition, and metrics sentence |

## When to add here vs. elsewhere

A widget belongs here if it is used by more than one screen or feature directory. If a widget is only used by one screen, keep it as a private method or nested class in that screen's file. If it is specific enough to one feature area, add it to the appropriate subdirectory.
