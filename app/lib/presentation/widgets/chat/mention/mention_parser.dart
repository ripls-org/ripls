import 'mention_types.dart';

/// Utilities for parsing and encoding @-mentions in message text.
///
/// The mention format is: @[type:id:display_name]
/// Example: @[user:abc123:John Doe]
class MentionParser {
  /// Regex pattern for matching encoded mentions.
  ///
  /// Matches: @[type:id:display_name]
  /// Where type is one of: user, loan, giveaway, request, experience
  static final RegExp mentionPattern = RegExp(
    r'@\[(user|loan|giveaway|request|experience):([a-zA-Z0-9_-]+):([^\]]+)\]',
  );

  /// Parses all mentions from the given text.
  ///
  /// Returns a list of [ParsedMention] objects sorted by their position in the text.
  static List<ParsedMention> parseMentions(String text) {
    final mentions = <ParsedMention>[];

    for (final match in mentionPattern.allMatches(text)) {
      final typeStr = match.group(1);
      final id = match.group(2);
      final displayName = match.group(3);

      if (typeStr == null || id == null || displayName == null) continue;

      final type = MentionTypeDisplay.fromString(typeStr);
      if (type == null) continue;

      mentions.add(ParsedMention(
        type: type,
        id: id,
        displayName: displayName,
        startIndex: match.start,
        endIndex: match.end,
      ));
    }

    return mentions;
  }

  /// Checks if the text contains any mentions.
  static bool hasMentions(String text) {
    return mentionPattern.hasMatch(text);
  }

  /// Extracts the plain text representation of a message with mentions.
  ///
  /// Replaces encoded mentions with just the display name prefixed with @.
  /// Example: "Hello @[user:123:John]!" -> "Hello @John!"
  static String toPlainText(String text) {
    return text.replaceAllMapped(mentionPattern, (match) {
      final displayName = match.group(3) ?? '';
      return '@$displayName';
    });
  }

  /// Splits text into segments of plain text and mentions.
  ///
  /// Returns a list of [MentionSegment] objects representing the text.
  static List<MentionSegment> segmentText(String text) {
    final segments = <MentionSegment>[];
    final mentions = parseMentions(text);

    if (mentions.isEmpty) {
      if (text.isNotEmpty) {
        segments.add(MentionSegment.text(text));
      }
      return segments;
    }

    int currentIndex = 0;

    for (final mention in mentions) {
      // Add text before this mention
      if (mention.startIndex > currentIndex) {
        segments.add(MentionSegment.text(
          text.substring(currentIndex, mention.startIndex),
        ));
      }

      // Add the mention
      segments.add(MentionSegment.mention(mention));

      currentIndex = mention.endIndex;
    }

    // Add remaining text after last mention
    if (currentIndex < text.length) {
      segments.add(MentionSegment.text(text.substring(currentIndex)));
    }

    return segments;
  }
}

/// A segment of text that may be plain text or a mention.
class MentionSegment {
  /// Whether this segment is a mention.
  final bool isMention;

  /// The plain text content (only set if isMention is false).
  final String? text;

  /// The mention data (only set if isMention is true).
  final ParsedMention? mention;

  const MentionSegment._({
    required this.isMention,
    this.text,
    this.mention,
  });

  /// Creates a plain text segment.
  factory MentionSegment.text(String text) {
    return MentionSegment._(isMention: false, text: text);
  }

  /// Creates a mention segment.
  factory MentionSegment.mention(ParsedMention mention) {
    return MentionSegment._(isMention: true, mention: mention);
  }
}
