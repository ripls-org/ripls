import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/design_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart'
    show experienceProvider;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/planning/poll_banner.dart';

/// Renders the right inline widget for a TIME_PROPOSED system message based
/// on the current poll state:
/// - Live poll → [PollBanner] (coral/sage with vote count and "Vote →" CTA).
/// - Poll ended (proposals exist but the poll is no longer active) → a sage
///   "Poll ended · See results →" banner that opens the read-only results
///   view of the same modal.
/// - No poll yet (e.g. data still loading) → plain system text fallback.
class PollBannerOrLabel extends ConsumerWidget {
  const PollBannerOrLabel({
    super.key,
    required this.experienceId,
    required this.currentUserId,
    required this.fallbackText,
    required this.onTap,
    this.messagePollId,
    this.isLatestTimeProposed = true,
  });

  final String experienceId;
  final String currentUserId;
  final String fallbackText;
  final VoidCallback onTap;

  /// The poll_id stamped on this chat message when it was created.
  /// Null for messages created before the poll_id field was introduced.
  final String? messagePollId;

  /// Whether this is the latest TIME_PROPOSED message in the conversation.
  /// Used to disambiguate legacy messages without a stored poll_id: only the
  /// latest such message can represent the currently-live poll; earlier ones
  /// are rendered as ended. Defaults to true so standalone uses (e.g. tests)
  /// retain backward-compatible behaviour.
  final bool isLatestTimeProposed;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final exp = ref.watch(
      experienceProvider(experienceId)
          .select((s) => s.experienceDetails?.experience),
    );

    if (exp == null) {
      return _buildFallback();
    }

    // When this message carries a poll_id (post-fix messages), use it to
    // determine whether this specific poll is still live. A stale poll_id
    // means a newer poll exists and this message should show as ended.
    if (messagePollId != null && messagePollId!.isNotEmpty) {
      final isThisMessageLive = exp.timePollActive &&
          exp.hasCurrentPollId() &&
          exp.currentPollId == messagePollId;
      if (isThisMessageLive) {
        return Padding(
          padding: const EdgeInsets.symmetric(vertical: 4),
          child: PollBanner(
            experienceId: experienceId,
            currentUserId: currentUserId,
            onTap: onTap,
          ),
        );
      }
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 4),
        child: _PollEndedChatBanner(onTap: onTap),
      );
    }

    // Legacy path for messages without a stored poll_id: only render the
    // live banner when this is the latest TIME_PROPOSED message. Earlier
    // legacy messages cannot represent the currently-live poll — showing
    // them as live causes the bug where a stale pre-fix chat card points to
    // a newly-created second poll.
    if (exp.timePollActive && isLatestTimeProposed) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 4),
        child: PollBanner(
          experienceId: experienceId,
          currentUserId: currentUserId,
          onTap: onTap,
        ),
      );
    }

    // Any non-latest legacy TIME_PROPOSED is historical by definition, even
    // if the experience has no surviving proposals (e.g. everyone deleted).
    // For latest legacy messages, fall back to proposal / completed state.
    final pollEnded = !isLatestTimeProposed ||
        (exp.hasTimePollCompleted() && exp.timePollCompleted) ||
        exp.timeProposals.isNotEmpty;
    if (pollEnded) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 4),
        child: _PollEndedChatBanner(onTap: onTap),
      );
    }

    return _buildFallback();
  }

  Widget _buildFallback() {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Center(
        child: Text(
          fallbackText,
          textAlign: TextAlign.center,
          style: const TextStyle(
            color: OverlayTokens.textPrimary,
            fontSize: 13,
          ),
        ),
      ),
    );
  }
}

/// Sage-tinted "Poll ended · See results →" banner shown inline in chat for
/// a historical TIME_PROPOSED system message after the poll has ended.
class _PollEndedChatBanner extends StatelessWidget {
  const _PollEndedChatBanner({required this.onTap});

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final sage = AppColors.experienceSageGreen;
    return Tappable(
      semanticsLabel: context.l10n.a11yChatViewPollResults,
      onTap: onTap,
      child: Container(
        decoration: BoxDecoration(
          color: sage.withValues(alpha: 0.15),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: sage.withValues(alpha: 0.40)),
        ),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        child: Row(
          children: [
            Icon(Icons.poll_outlined, size: 18, color: sage),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                context.l10n.chatPollEndedHeadline,
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                  color: OverlayTokens.textPrimary,
                ),
              ),
            ),
            Container(
              padding:
                  const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
              decoration: BoxDecoration(
                color: sage,
                borderRadius: BorderRadius.circular(999),
              ),
              child: Text(
                context.l10n.chatPollEndedCta,
                style: const TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w700,
                  // Dark on the sage fill, matching every other filled CTA.
                  // White measured 3.05:1 here — 12pt bold is not WCAG "large
                  // text" (that needs 14pt bold), so it owed 4.5:1. The dark
                  // ink gives 5.32:1.
                  color: DesignTokens.darkOnPrimary,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
