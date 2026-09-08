// Types and data structures for @-mentions in chat.
//
// This file defines the core types used throughout the @-mention feature,
// including mention types, suggestion items, and encoded mention format.

import 'package:flutter/widgets.dart';

/// The type of entity being mentioned.
enum MentionType {
  /// A user in the community.
  user,

  /// An active loan (gear being lent).
  loan,

  /// An active giveaway (gear being given away).
  giveaway,

  /// A request for gear or help.
  request,

  /// A community experience/event.
  experience,
}

/// Extension to provide display properties for mention types.
extension MentionTypeDisplay on MentionType {
  /// The icon to display for this mention type.
  String get icon {
    switch (this) {
      case MentionType.user:
        return '👤';
      case MentionType.loan:
        return '🔄';
      case MentionType.giveaway:
        return '🎁';
      case MentionType.request:
        return '❓';
      case MentionType.experience:
        return '📅';
    }
  }

  /// The label to display for this mention type (used for badges).
  String get label {
    switch (this) {
      case MentionType.user:
        return 'User';
      case MentionType.loan:
        return 'Loan';
      case MentionType.giveaway:
        return 'Giveaway';
      case MentionType.request:
        return 'Request';
      case MentionType.experience:
        return 'Event';
    }
  }

  /// The encoded type string used in the mention format.
  String get encodedType {
    switch (this) {
      case MentionType.user:
        return 'user';
      case MentionType.loan:
        return 'loan';
      case MentionType.giveaway:
        return 'giveaway';
      case MentionType.request:
        return 'request';
      case MentionType.experience:
        return 'experience';
    }
  }

  /// Parses a type string to a MentionType.
  static MentionType? fromString(String type) {
    switch (type) {
      case 'user':
        return MentionType.user;
      case 'loan':
        return MentionType.loan;
      case 'giveaway':
        return MentionType.giveaway;
      case 'request':
        return MentionType.request;
      case 'experience':
        return MentionType.experience;
      default:
        return null;
    }
  }
}

/// A suggestion item for the autocomplete menu.
class MentionSuggestion {
  /// The type of mention.
  final MentionType type;

  /// The unique identifier for the entity.
  final String id;

  /// The display name of the entity.
  final String displayName;

  /// Optional media ID for avatar/image display (primarily for users).
  final String? mediaId;

  /// Optional secondary text (e.g., status for items).
  final String? subtitle;

  /// Optional image provider for thumbnail display.
  /// In production, this would be loaded from the media repository.
  final ImageProvider? imageProvider;

  const MentionSuggestion({
    required this.type,
    required this.id,
    required this.displayName,
    this.mediaId,
    this.subtitle,
    this.imageProvider,
  });

  /// Encodes this suggestion as a mention string.
  ///
  /// Format: @[type:id:display_name]
  String toEncodedMention() {
    return '@[${type.encodedType}:$id:$displayName]';
  }

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is MentionSuggestion &&
        other.type == type &&
        other.id == id &&
        other.displayName == displayName;
  }

  @override
  int get hashCode => Object.hash(type, id, displayName);

  @override
  String toString() =>
      'MentionSuggestion(type: $type, id: $id, displayName: $displayName)';
}

/// A parsed mention from message text.
class ParsedMention {
  /// The type of mention.
  final MentionType type;

  /// The unique identifier for the entity.
  final String id;

  /// The display name of the entity.
  final String displayName;

  /// The start index of the mention in the original text.
  final int startIndex;

  /// The end index of the mention in the original text.
  final int endIndex;

  const ParsedMention({
    required this.type,
    required this.id,
    required this.displayName,
    required this.startIndex,
    required this.endIndex,
  });

  /// The length of the encoded mention in the source text.
  int get length => endIndex - startIndex;

  @override
  String toString() => 'ParsedMention(type: $type, id: $id, name: $displayName)';
}
