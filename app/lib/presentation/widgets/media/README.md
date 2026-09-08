# Media Widgets

Image and video display, carousel, and picker components for user-uploaded media.

## Purpose

These widgets handle all user media presentation: full-screen carousel with swipe and download, background image display, and the picker flow that lets users select photos or videos from their camera or gallery.

## Key Files

- **`media_carousel.dart`** — full-screen swipeable carousel for viewing uploaded images and videos. Supports downloading, long-press actions, and attribution display. Uses stable cache keys based on media ID.
- **`background_media_image.dart`** — widget that fills a container with a cached media image, used as a background behind content.
- **`media_background.dart`** — scrollable background that blurs and dims a media image behind overlaid content.
- **`media_picker_button.dart`** — icon button that triggers the media picker dialog.
- **`media_picker_dialog.dart`** — dialog for choosing between camera capture and gallery selection with consistent quality settings.
- **`attribution_widget.dart`** — displays attribution text and link for stock images.

## When to add here vs. elsewhere

User media display and upload components belong here. Always use `cached_media_image.dart` (in `widgets/chat/`) or the stable-cache-key pattern when displaying user-uploaded images — never `Image.network` directly. Static asset display (icons, illustrations) does not belong here.
