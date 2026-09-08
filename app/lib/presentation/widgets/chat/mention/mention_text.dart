import 'package:flutter/material.dart';
import 'mention_chip.dart';
import 'mention_parser.dart';
import 'mention_types.dart';

/// Callback signature for when a mention is tapped.
typedef OnMentionTap = void Function(MentionType type, String id);

/// A text widget that renders @-mentions as styled, tappable chips.
///
/// Parses the text for encoded mentions (e.g., @[user:123:John Doe]) and
/// renders them as [MentionChip] widgets inline with the regular text.
class MentionText extends StatelessWidget {
  /// The text to display, which may contain encoded mentions.
  final String text;

  /// The text style for non-mention text.
  final TextStyle? style;

  /// Whether the text is displayed on a dark background (e.g., outgoing chat bubble).
  final bool onDarkBackground;

  /// Callback when a mention chip is tapped.
  final OnMentionTap? onMentionTap;

  /// Maximum number of lines for the text.
  final int? maxLines;

  /// How to handle text overflow.
  final TextOverflow? overflow;

  const MentionText({
    super.key,
    required this.text,
    this.style,
    this.onDarkBackground = false,
    this.onMentionTap,
    this.maxLines,
    this.overflow,
  });

  @override
  Widget build(BuildContext context) {
    // If no mentions, just render plain text
    if (!MentionParser.hasMentions(text)) {
      return Text(
        text,
        style: style,
        maxLines: maxLines,
        overflow: overflow,
      );
    }

    // Parse and render with inline mention chips
    final segments = MentionParser.segmentText(text);
    final spans = <InlineSpan>[];

    for (final segment in segments) {
      if (segment.isMention) {
        spans.add(_buildMentionSpan(segment.mention!));
      } else {
        spans.add(TextSpan(text: segment.text, style: style));
      }
    }

    return Text.rich(
      TextSpan(children: spans),
      maxLines: maxLines,
      overflow: overflow,
    );
  }

  WidgetSpan _buildMentionSpan(ParsedMention mention) {
    return WidgetSpan(
      alignment: PlaceholderAlignment.middle,
      child: MentionChip.fromParsed(
        mention: mention,
        onDarkBackground: onDarkBackground,
        onTap: onMentionTap != null
            ? () => onMentionTap!(mention.type, mention.id)
            : null,
      ),
    );
  }
}
