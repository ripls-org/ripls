import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// ContentDiscussionCard is the compact entry point into the content's
/// conversation (see docs/issues/2278-experience-content-redesign.md). It pins
/// a single quote — the content description, attributed to its owner — above
/// the attribution line that carries the thread meta ("— Alfred · 3 replies ·
/// last 2h") and an unread dot. When the caller supplies a [latestLine], the
/// most recent reply renders as a separate small labeled preview beneath the
/// attribution; the quote itself never rotates (#2724 — a cross-fade
/// carousel here resurfaced superseded text with ghosting artifacts).
///
/// Role-agnostic and always-on-dark. The caller resolves all strings: the
/// pluralized [replyCountLabel] — whose count should exclude the seeded
/// description (#2724) — the [startLabel] shown when there are no replies,
/// the [lastActivityLabel] suffix, and the [latestLine] preview. The whole
/// card is tappable to open the conversation.
class ContentDiscussionCard extends StatelessWidget {
  /// Name of the content owner (author of the opening quote).
  final String authorName;

  /// The opening quote text (the content description). Empty drops the quote.
  final String firstComment;

  /// Total replies in the thread, excluding the seeded description.
  final int replyCount;

  /// Resolved "{n} replies" label (caller-pluralized). Shown when
  /// [replyCount] > 0.
  final String replyCountLabel;

  /// Resolved "Start the conversation" label, shown when there are no replies.
  final String startLabel;

  /// Resolved "last {time}" suffix (e.g. "last 2h"). Appended to the reply
  /// count when present.
  final String? lastActivityLabel;

  /// Resolved latest-reply preview ("💬 Maya: Can't wait!"). Null/empty hides
  /// the preview line. Rendered as its own labeled line under the attribution,
  /// never swapped into the quote.
  final String? latestLine;

  /// Whether to show the unread accent dot.
  final bool hasUnread;

  /// Accent for the unread dot.
  final Color accentColor;

  /// Opens the conversation, receiving the card's current on-screen rect
  /// (global coordinates) so the caller can morph/grow the conversation from
  /// the card's footprint. When null the card is not tappable and the chevron
  /// is hidden (e.g. there is no thread yet).
  final ValueChanged<Rect>? onTap;

  /// Screen-reader label for the tap target.
  final String semanticsLabel;

  const ContentDiscussionCard({
    super.key,
    required this.authorName,
    required this.firstComment,
    required this.replyCount,
    required this.replyCountLabel,
    required this.startLabel,
    required this.accentColor,
    required this.semanticsLabel,
    this.lastActivityLabel,
    this.latestLine,
    this.hasUnread = false,
    this.onTap,
  });

  static const int _quoteMaxLines = 2;

  static const TextStyle _quoteStyle = TextStyle(
    fontFamily: AppTheme.headingFont,
    fontStyle: FontStyle.italic,
    color: AppColors.onContentImage,
    fontSize: 15,
    height: 1.45,
  );

  @override
  Widget build(BuildContext context) {
    final hasQuote = firstComment.trim().isNotEmpty;
    final latest = latestLine?.trim();
    final hasLatest = latest != null && latest.isNotEmpty;

    final card = Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      decoration: BoxDecoration(
        color: AppColors.darkTextPrimary.withValues(alpha: 0.03),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: AppColors.darkBorder),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.end,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                if (hasQuote) ...[
                  _quote(firstComment.trim()),
                  const SizedBox(height: 10),
                ],
                _attribution(hasQuote ? authorName : null),
                if (hasLatest) ...[
                  const SizedBox(height: 8),
                  _latest(latest),
                ],
              ],
            ),
          ),
          const SizedBox(width: 12),
          _trailing(),
        ],
      ),
    );

    if (onTap == null) return card;
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: () {
        // Capture the card's own footprint so the caller can grow from it.
        final box = context.findRenderObject();
        final rect = box is RenderBox && box.hasSize
            ? box.localToGlobal(Offset.zero) & box.size
            : Rect.zero;
        onTap!(rect);
      },
      excludeChildSemantics: false,
      child: card,
    );
  }

  /// The quoted description body, in the editorial serif italic, capped at two
  /// lines. The decorative quote marks are dropped when the text overflows —
  /// an opening quote whose closing mate was ellipsized away reads as a typo
  /// (#2724).
  Widget _quote(String text) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final quoted = '"$text"';
        final painter = TextPainter(
          text: TextSpan(text: quoted, style: _quoteStyle),
          maxLines: _quoteMaxLines,
          textDirection: Directionality.of(context),
          textScaler: MediaQuery.textScalerOf(context),
        )..layout(maxWidth: constraints.maxWidth);
        final overflows = painter.didExceedMaxLines;
        painter.dispose();
        return Text(
          overflows ? text : quoted,
          maxLines: _quoteMaxLines,
          overflow: TextOverflow.ellipsis,
          style: _quoteStyle,
        );
      },
    );
  }

  /// The attribution line: "— {author} · {reply meta}". When there are replies
  /// the meta is "{n} replies · last {time}"; otherwise it is the start prompt.
  Widget _attribution(String? author) {
    final hasReplies = replyCount > 0;
    final meta = hasReplies
        ? (lastActivityLabel != null
              ? '$replyCountLabel · $lastActivityLabel'
              : replyCountLabel)
        : startLabel;
    final hasAuthor = author != null && author.isNotEmpty;
    return Text.rich(
      TextSpan(
        children: [
          const TextSpan(text: '— '),
          if (hasAuthor)
            TextSpan(
              text: author,
              style: const TextStyle(
                color: AppColors.onContentImageSecondary,
                fontWeight: FontWeight.w700,
              ),
            ),
          TextSpan(text: hasAuthor ? ' · $meta' : meta),
        ],
        style: const TextStyle(
          color: AppColors.onContentImageFaint,
          fontSize: 12,
          fontWeight: FontWeight.w500,
        ),
      ),
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
    );
  }

  /// The latest-reply preview line ("💬 Maya: Can't wait!") — a small labeled
  /// line of its own, kept visually subordinate to the pinned quote.
  Widget _latest(String text) {
    return Text(
      text,
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
      style: const TextStyle(
        color: AppColors.onContentImageSecondary,
        fontSize: 12,
        fontWeight: FontWeight.w500,
      ),
    );
  }

  /// The unread dot (when [hasUnread]) and the open-conversation chevron
  /// (when tappable), pinned to the bottom-right.
  Widget _trailing() {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (hasUnread) ...[
          Container(
            width: 7,
            height: 7,
            decoration: BoxDecoration(
              color: accentColor,
              shape: BoxShape.circle,
            ),
          ),
          const SizedBox(width: 8),
        ],
        if (onTap != null)
          const Icon(
            Icons.chevron_right_rounded,
            size: 20,
            color: AppColors.onContentImageFaint,
          ),
      ],
    );
  }
}
