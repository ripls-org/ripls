import 'package:flutter/widgets.dart';
import 'mention_types.dart';

/// A TextEditingController that tracks @-mention queries and insertions.
///
/// Detects when the user is typing a mention query (text after @) and provides
/// the query text. When a mention is selected, it inserts `@DisplayName` into
/// the text field and stores the mention metadata separately. The mentions are
/// displayed with blue styling, and the encoded format is generated when
/// getting the message text.
class MentionTextController extends TextEditingController {
  /// Callback when the mention query changes.
  ///
  /// Called with the query text when the user is typing after @, or null
  /// when not in mention mode. Can be set after construction.
  ValueChanged<MentionQuery?>? onMentionQueryChanged;

  MentionQuery? _currentQuery;

  /// Stores metadata for inserted mentions, keyed by their display text position.
  /// Key is the start index of `@DisplayName` in the text.
  final List<_InsertedMention> _insertedMentions = [];

  MentionTextController({
    super.text,
    this.onMentionQueryChanged,
  });

  /// The current mention query, or null if not in mention mode.
  MentionQuery? get currentQuery => _currentQuery;

  /// Returns the text with encoded mentions for sending to the server.
  ///
  /// Replaces `@DisplayName` with `@[type:id:DisplayName]` for each stored mention.
  String get encodedText {
    if (_insertedMentions.isEmpty) {
      return text;
    }

    // Sort mentions by position (descending) to replace from end to start
    // This prevents position shifting issues
    final sortedMentions = List<_InsertedMention>.from(_insertedMentions)
      ..sort((a, b) => b.startIndex.compareTo(a.startIndex));

    String result = text;
    for (final mention in sortedMentions) {
      final displayText = '@${mention.displayName}';
      final encoded = '@[${mention.type.name}:${mention.id}:${mention.displayName}]';

      // Verify the mention is still at the expected position
      if (mention.startIndex >= 0 &&
          mention.startIndex + displayText.length <= result.length &&
          result.substring(mention.startIndex, mention.startIndex + displayText.length) == displayText) {
        result = result.substring(0, mention.startIndex) +
            encoded +
            result.substring(mention.startIndex + displayText.length);
      }
    }

    return result;
  }

  @override
  TextSpan buildTextSpan({
    required BuildContext context,
    TextStyle? style,
    required bool withComposing,
  }) {
    if (_insertedMentions.isEmpty) {
      return super.buildTextSpan(
        context: context,
        style: style,
        withComposing: withComposing,
      );
    }

    // Build segments based on inserted mentions
    final segments = _buildSegments();
    if (segments.isEmpty) {
      return super.buildTextSpan(
        context: context,
        style: style,
        withComposing: withComposing,
      );
    }

    // Build styled spans
    final children = <InlineSpan>[];
    for (final segment in segments) {
      if (segment.isMention) {
        children.add(TextSpan(
          text: segment.text,
          style: style?.copyWith(
            color: const Color(0xFF2196F3), // Blue color for mentions
            fontWeight: FontWeight.w500,
          ),
        ));
      } else {
        children.add(TextSpan(
          text: segment.text,
          style: style,
        ));
      }
    }

    return TextSpan(children: children, style: style);
  }

  /// Builds text segments based on current mention positions.
  List<_TextSegment> _buildSegments() {
    if (text.isEmpty) return [];

    // Sort mentions by position
    final sortedMentions = List<_InsertedMention>.from(_insertedMentions)
      ..sort((a, b) => a.startIndex.compareTo(b.startIndex));

    final segments = <_TextSegment>[];
    int currentIndex = 0;

    for (final mention in sortedMentions) {
      final displayText = '@${mention.displayName}';
      final endIndex = mention.startIndex + displayText.length;

      // Verify mention is still valid
      if (mention.startIndex < 0 ||
          endIndex > text.length ||
          text.substring(mention.startIndex, endIndex) != displayText) {
        continue;
      }

      // Add text before mention
      if (mention.startIndex > currentIndex) {
        segments.add(_TextSegment(
          text: text.substring(currentIndex, mention.startIndex),
          isMention: false,
        ));
      }

      // Add mention
      segments.add(_TextSegment(
        text: displayText,
        isMention: true,
      ));

      currentIndex = endIndex;
    }

    // Add remaining text
    if (currentIndex < text.length) {
      segments.add(_TextSegment(
        text: text.substring(currentIndex),
        isMention: false,
      ));
    }

    return segments;
  }

  /// Whether the user is currently typing a mention query.
  bool get isInMentionMode => _currentQuery != null;

  @override
  set value(TextEditingValue newValue) {
    final oldText = text;
    final oldSelection = selection;

    // Check for atomic mention deletion (backspace into a mention)
    final adjustedValue = _handleMentionDeletion(oldText, oldSelection, newValue);

    super.value = adjustedValue;
    _detectMentionQuery();

    // Clean up invalid mentions when text changes
    if (oldText != adjustedValue.text) {
      _cleanupInvalidMentions();
    }
  }

  /// Handles atomic deletion of mentions.
  ///
  /// When the user backspaces into a mention, this deletes the entire mention
  /// as a unit rather than character-by-character.
  TextEditingValue _handleMentionDeletion(
    String oldText,
    TextSelection oldSelection,
    TextEditingValue newValue,
  ) {
    // Only handle single-character deletions (backspace/delete)
    if (newValue.text.length != oldText.length - 1) {
      return newValue;
    }

    // Only handle collapsed selections (no text selected)
    if (!oldSelection.isCollapsed) {
      return newValue;
    }

    final cursorPos = oldSelection.baseOffset;

    // Check if the deletion affected any mention
    for (final mention in _insertedMentions) {
      final displayText = '@${mention.displayName}';
      final mentionStart = mention.startIndex;
      final mentionEnd = mentionStart + displayText.length;

      // Check if cursor was at end of mention (backspace) or inside mention
      // Backspace: cursor was at mentionEnd, now text is shorter
      // Delete: cursor was inside mention
      if (cursorPos > mentionStart && cursorPos <= mentionEnd) {
        // Delete the entire mention
        final beforeMention = oldText.substring(0, mentionStart);
        final afterMention = oldText.substring(mentionEnd);
        final newText = beforeMention + afterMention;

        // Remove this mention from tracking
        _insertedMentions.remove(mention);

        // Adjust positions of mentions after this one
        _adjustMentionPositions(mentionStart, -displayText.length);

        return TextEditingValue(
          text: newText,
          selection: TextSelection.collapsed(offset: mentionStart),
        );
      }
    }

    return newValue;
  }

  @override
  void clear() {
    _insertedMentions.clear();
    super.clear();
  }

  /// Removes mentions that are no longer valid in the current text.
  void _cleanupInvalidMentions() {
    _insertedMentions.removeWhere((mention) {
      final displayText = '@${mention.displayName}';
      final endIndex = mention.startIndex + displayText.length;

      // Remove if position is invalid or text doesn't match
      if (mention.startIndex < 0 ||
          endIndex > text.length ||
          text.substring(mention.startIndex, endIndex) != displayText) {
        return true;
      }
      return false;
    });
  }

  /// Inserts a mention at the current query position.
  ///
  /// Replaces the @query text with `@DisplayName` and stores the mention metadata.
  /// The encoded format is generated when accessing [encodedText].
  void insertMention(MentionSuggestion suggestion) {
    final query = _currentQuery;
    if (query == null) return;

    final displayText = '@${suggestion.displayName}';
    final beforeAt = text.substring(0, query.atIndex);
    final afterQuery = text.substring(query.atIndex + query.queryText.length + 1);

    // Update mention positions for any mentions after this insertion point
    final lengthDiff = displayText.length - (query.queryText.length + 1); // +1 for @
    _adjustMentionPositions(query.atIndex, lengthDiff);

    // Store the mention metadata
    _insertedMentions.add(_InsertedMention(
      type: suggestion.type,
      id: suggestion.id,
      displayName: suggestion.displayName,
      startIndex: query.atIndex,
    ));

    // Insert display text with trailing space
    final newText = '$beforeAt$displayText $afterQuery';
    final newCursorPosition = beforeAt.length + displayText.length + 1;

    value = TextEditingValue(
      text: newText,
      selection: TextSelection.collapsed(offset: newCursorPosition),
    );
  }

  /// Adjusts mention positions when text is inserted or deleted.
  void _adjustMentionPositions(int changeIndex, int delta) {
    for (int i = 0; i < _insertedMentions.length; i++) {
      final mention = _insertedMentions[i];
      if (mention.startIndex > changeIndex) {
        _insertedMentions[i] = _InsertedMention(
          type: mention.type,
          id: mention.id,
          displayName: mention.displayName,
          startIndex: mention.startIndex + delta,
        );
      }
    }
  }

  /// Cancels the current mention query.
  ///
  /// Call this when the overlay is dismissed via escape or tap outside.
  void cancelMention() {
    if (_currentQuery != null) {
      _currentQuery = null;
      onMentionQueryChanged?.call(null);
    }
  }

  /// Detects if the user is typing a mention query.
  ///
  /// A mention query starts with @ and continues until:
  /// - A space, newline, or the end of input
  /// - The cursor moves away from the query
  void _detectMentionQuery() {
    final cursorPos = selection.baseOffset;
    if (cursorPos < 0 || !selection.isCollapsed) {
      _updateQuery(null);
      return;
    }

    // Look backwards from cursor for @
    final textBeforeCursor = text.substring(0, cursorPos);
    final atIndex = _findMentionStart(textBeforeCursor);

    if (atIndex == -1) {
      _updateQuery(null);
      return;
    }

    // Extract the query text (everything from @ to cursor)
    final queryText = textBeforeCursor.substring(atIndex + 1);

    // Check if query contains invalid characters (spaces, newlines, etc.)
    if (queryText.contains(RegExp(r'[\s\n]'))) {
      _updateQuery(null);
      return;
    }

    _updateQuery(MentionQuery(
      atIndex: atIndex,
      queryText: queryText,
    ));
  }

  /// Finds the start of a mention (@) looking backwards from the text.
  ///
  /// Returns the index of @ if found, or -1 if not in a mention context.
  /// The @ must be at the start of the text or preceded by whitespace or punctuation.
  int _findMentionStart(String text) {
    if (text.isEmpty) return -1;

    // Look backwards for @
    for (int i = text.length - 1; i >= 0; i--) {
      final char = text[i];

      // Found @
      if (char == '@') {
        // @ at start of text, after whitespace, or after punctuation is valid
        if (i == 0 || _isMentionBoundary(text[i - 1])) {
          return i;
        }
        // @ after non-boundary character is not a mention trigger
        return -1;
      }

      // Hit whitespace before finding @ - no mention in progress
      if (_isWhitespace(char)) {
        return -1;
      }
    }

    return -1;
  }

  /// Returns true if the character is a valid boundary before @.
  ///
  /// Includes whitespace and common punctuation that can precede a mention.
  bool _isMentionBoundary(String char) {
    return _isWhitespace(char) || _isPunctuation(char);
  }

  bool _isWhitespace(String char) {
    return char == ' ' || char == '\n' || char == '\t';
  }

  /// Punctuation characters that can precede a mention.
  bool _isPunctuation(String char) {
    const punctuation = '([{<-:;,!?"\'/';
    return punctuation.contains(char);
  }

  void _updateQuery(MentionQuery? newQuery) {
    if (_currentQuery != newQuery) {
      _currentQuery = newQuery;
      onMentionQueryChanged?.call(newQuery);
    }
  }
}

/// Represents an active mention query being typed by the user.
class MentionQuery {
  /// The index of the @ character in the text.
  final int atIndex;

  /// The query text after @ (without the @ itself).
  final String queryText;

  const MentionQuery({
    required this.atIndex,
    required this.queryText,
  });

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is MentionQuery &&
        other.atIndex == atIndex &&
        other.queryText == queryText;
  }

  @override
  int get hashCode => Object.hash(atIndex, queryText);

  @override
  String toString() => 'MentionQuery(atIndex: $atIndex, query: "$queryText")';
}

/// Stores metadata for an inserted mention.
class _InsertedMention {
  /// The type of mention (user, loan, etc.).
  final MentionType type;

  /// The unique ID of the mentioned entity.
  final String id;

  /// The display name shown in the text field.
  final String displayName;

  /// The start index of `@DisplayName` in the text.
  final int startIndex;

  const _InsertedMention({
    required this.type,
    required this.id,
    required this.displayName,
    required this.startIndex,
  });
}

/// Internal helper for text segmentation.
class _TextSegment {
  final String text;
  final bool isMention;

  const _TextSegment({
    required this.text,
    required this.isMention,
  });
}
