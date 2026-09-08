import 'package:ripls/core/utils/date_time_formatter.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/presentation/models/discover_item.dart';

/// MagazineItem is the view model for [MagazineThumbnail].
///
/// Adapts a [DiscoverItem] (discover/search context) into a single,
/// stateless data class. No async lookups are performed here — location
/// names must be pre-resolved before construction.
class MagazineItem {
  /// Stable identifier used as a list key.
  final String id;

  /// Media ID of the background image. Empty if no image is available.
  final String mediaId;

  /// Primary display title for the card.
  final String title;

  /// Human-readable type label, e.g. "LENDING", "GIVING", "EVENT", "HELP".
  final String typeLabel;

  /// Short date string shown after the type label for scheduled events,
  /// e.g. "Mar 5". Null for untimed items.
  final String? dateLabel;

  /// First name of the sender of the most recent comment. Null if none.
  final String? commentSender;

  /// Truncated text of the most recent comment. Null if none.
  final String? commentText;

  /// Display name of the item owner/requester. Null in daily context
  /// (the panel already shows the person).
  final String? ownerName;

  /// Pre-resolved human-readable location name, e.g. "Denver, CO".
  /// Null if no location is set.
  final String? locationName;

  /// Fallback emoji displayed when no image is available.
  final String emoji;

  /// Internal type token used to pick the emoji fallback background color.
  /// One of: "lending", "giving", "event", "help".
  final String itemType;

  const MagazineItem({
    required this.id,
    required this.mediaId,
    required this.title,
    required this.typeLabel,
    this.dateLabel,
    this.commentSender,
    this.commentText,
    this.ownerName,
    this.locationName,
    required this.emoji,
    required this.itemType,
  });

  /// Creates a MagazineItem from a [DiscoverItem] with an optional
  /// pre-resolved [locationName].
  ///
  /// Comment fields are null — discover items do not carry message previews.
  /// Owner name is taken from the item's owner/requester field.
  factory MagazineItem.fromDiscoverItem(
    DiscoverItem item, {
    String? locationName,
  }) {
    return item.when(
      gear: (gear) {
        final isGiveaway =
            gear.availability == Availability.AVAILABILITY_FOR_GIVEAWAY;
        return MagazineItem(
          id: gear.id,
          mediaId: gear.mediaIds.isNotEmpty ? gear.mediaIds.first : '',
          title: gear.name,
          typeLabel: isGiveaway ? 'Giving' : 'Lending',
          dateLabel: null,
          commentSender: null,
          commentText: null,
          ownerName: gear.owner.name.isNotEmpty ? gear.owner.name : null,
          locationName: locationName,
          emoji: isGiveaway ? '🎁' : '↩',
          itemType: isGiveaway ? 'giving' : 'lending',
        );
      },
      request: (request) => MagazineItem(
        id: request.id,
        mediaId: request.mediaIds.isNotEmpty ? request.mediaIds.first : '',
        title:
            request.title.isNotEmpty ? request.title : request.description,
        typeLabel: 'Help',
        dateLabel: null,
        commentSender: null,
        commentText: null,
        ownerName:
            request.requester.name.isNotEmpty ? request.requester.name : null,
        locationName: locationName,
        emoji: '🤝',
        itemType: 'help',
      ),
      experience: (experience) {
        final timeLabel =
            DateTimeFormatter.formatExperienceTime(experience.time);
        return MagazineItem(
          id: experience.id,
          mediaId:
              experience.mediaIds.isNotEmpty ? experience.mediaIds.first : '',
          title: experience.name,
          typeLabel: 'Event',
          dateLabel: timeLabel == 'TBD' ? null : timeLabel,
          commentSender: null,
          commentText: null,
          ownerName:
              experience.owner.name.isNotEmpty ? experience.owner.name : null,
          locationName: locationName,
          emoji: '📅',
          itemType: 'event',
        );
      },
    );
  }

  /// Returns the bottom-line display text, following the priority order:
  ///
  /// 1. "Sender · comment text" if any comment data is present.
  /// 2. "OwnerName · LocationName" if both are set.
  /// 3. Owner name alone if only owner is set.
  /// 4. Location name alone if only location is set.
  /// 5. Empty string — card height is fixed regardless.
  String get bottomLine {
    if (commentSender != null || commentText != null) {
      final parts = <String>[];
      if (commentSender != null) parts.add(commentSender!);
      if (commentText != null) parts.add(commentText!);
      return parts.join(' · ');
    }
    if (ownerName != null && locationName != null) {
      return '$ownerName · $locationName';
    }
    if (ownerName != null) return ownerName!;
    if (locationName != null) return locationName!;
    return '';
  }
}
