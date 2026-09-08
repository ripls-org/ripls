import 'dart:async';

import 'package:connectrpc/connect.dart' as connect;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart'
    show StoryPayload, StoryType;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show ImpactEstimate;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/presentation/viewmodels/story_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_view_builders.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/floating_header.dart';
import 'package:ripls/presentation/widgets/media/video_background_host.dart';
import 'package:ripls/presentation/widgets/story/story_embedded_esm.dart';
import 'package:ripls/presentation/widgets/story/story_text_resolver.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/providers.dart';

/// StoryContentView displays a story feed item.
///
/// Stories are read-only, system-generated content that celebrate
/// community accomplishments and welcome new members.
///
/// Unlike other content views (gear, request, experience), stories:
/// - Have no owner, only participants
/// - Are read-only (no editing)
/// - Only appear in the feed (no standalone viewing)
class StoryContentView extends ConsumerStatefulWidget {
  final StoryPayload story;

  // Feed header parameters (optional, only used in feed context)
  final bool showFeedHeader;
  final User? feedActor;
  final int? feedOccurredAtUnixSec;
  final String? feedActionText;

  const StoryContentView({
    super.key,
    required this.story,
    // Feed header (disabled by default)
    this.showFeedHeader = false,
    this.feedActor,
    this.feedOccurredAtUnixSec,
    this.feedActionText,
  });

  @override
  ConsumerState<StoryContentView> createState() => _StoryContentViewState();
}

class _StoryContentViewState extends ConsumerState<StoryContentView> {
  @override
  Widget build(BuildContext context) {
    final storyAsync = ref.watch(storyProvider(widget.story));

    return storyAsync.when(
      loading: () => ContentViewBuilders.buildLoadingView(),
      error: (e, st) => ContentViewBuilders.buildErrorView(
        title: 'Failed to Load Story',
        errorMessage: e.toString(),
        onRetry: () => ref.invalidate(storyProvider(widget.story)),
      ),
      data: (state) => _buildContent(context, state),
    );
  }

  Widget _buildContent(BuildContext context, StoryState state) {
    final mediaUrl = state.primaryMediaUrl;

    return Stack(
      fit: StackFit.expand,
      children: [
        // Background media
        VideoBackgroundHost(
          mediaPath: state.mediaPath ?? mediaUrl?.url,
          mediaId: state.mediaId ?? mediaUrl?.mediaId,
          isVideo: state.isVideo,
          thumbnailUrl: state.backgroundThumbnailUrl,
          isMuted: true,
        ),

        // Semi-transparent black overlay (like onboarding)
        Container(color: OverlayTokens.scrimFloor),

        // Centered content overlay — bottom reserved for the ESM block.
        SafeArea(
          child: Padding(
            padding: EdgeInsets.fromLTRB(
              32,
              100,
              32,
              state.story.hasEmbeddedEsmPrompt()
                  ? FloatingHeader.contentBottom(context) + 60.0
                  : 100.0,
            ),
            child: Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                crossAxisAlignment: CrossAxisAlignment.center,
                children: [
                  // Editorial kicker — server-formatted "DAY · MONTH D ·
                  // LOCATION" line that anchors the recap visually. Server
                  // populates only for experience-recap stories; absent
                  // means render nothing rather than collapsing the layout.
                  if (state.story.hasKicker() && state.story.kicker.isNotEmpty)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 16),
                      child: Text(
                        state.story.kicker,
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          color: GlassTokens.textMuted,
                          fontSize: 10,
                          fontWeight: FontWeight.w600,
                          letterSpacing: 2.2,
                          shadows: const [
                            Shadow(
                              offset: Offset(0, 1),
                              blurRadius: 4,
                              color: Colors.black45,
                            ),
                          ],
                        ),
                      ),
                    ),

                  // Story title and description (tappable)
                  Builder(
                    builder: (context) {
                      final resolved =
                          resolveStoryText(state.story, context.l10n);
                      return Tappable(
                    semanticsLabel: resolved.title,
                    onTap: _hasRelatedItem(state.story)
                        ? () => _navigateToRelatedItem(context, state.story)
                        : null,
                    excludeChildSemantics: false,
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        // Story title
                        Text(
                          resolved.title,
                          textAlign: TextAlign.center,
                          style: Theme.of(context).textTheme.headlineMedium
                              ?.copyWith(
                                color: GlassTokens.textPrimary,
                                fontWeight: FontWeight.w700,
                                shadows: [
                                  const Shadow(
                                    offset: Offset(0, 2),
                                    blurRadius: 8,
                                    color: Colors.black45,
                                  ),
                                ],
                              ),
                        ),
                        const SizedBox(height: 24),

                        // Story description
                        if (resolved.description.isNotEmpty)
                          Text(
                            resolved.description,
                            textAlign: TextAlign.center,
                            style: Theme.of(context).textTheme.bodyLarge
                                ?.copyWith(
                                  fontFamily: AppTheme.headingFont,
                                  color: GlassTokens.textPrimary,
                                  fontSize: 15.5,
                                  height: 1.55,
                                  shadows: [
                                    const Shadow(
                                      offset: Offset(0, 1),
                                      blurRadius: 6,
                                      color: Colors.black45,
                                    ),
                                  ],
                                ),
                          ),
                      ],
                    ),
                  );
                    },
                  ),
                  const SizedBox(height: 32),

                  // Participants (without label) — suppressed for the
                  // experience-recap layout, where names are baked into
                  // the story prose and the editorial column stays clean.
                  if (state.participants.isNotEmpty &&
                      state.story.storyType !=
                          StoryType.STORY_TYPE_EXPERIENCE_CONCLUDED)
                    _buildParticipantsList(context, state),

                  // Add spacing between impact and participants
                  if (state.story.hasImpact() &&
                      state.participants.isNotEmpty &&
                      state.story.storyType !=
                          StoryType.STORY_TYPE_EXPERIENCE_CONCLUDED)
                    const SizedBox(height: 24),

                  // Impact section (if available). Experience-recap stories
                  // get the inline editorial row; other stories keep the
                  // standard card layout.
                  if (state.story.hasImpact())
                    state.story.storyType ==
                            StoryType.STORY_TYPE_EXPERIENCE_CONCLUDED
                        ? _buildRecapMetricsRow(context, state.story.impact)
                        : _buildSavingsDisplay(state.story.impact),

                  // Actor-only Undo link — server populates
                  // undoable_action when the requesting user can still
                  // reverse the action that generated this story.
                  if (state.story.hasUndoableAction())
                    _buildUndoLink(context, state.story),
                ],
              ),
            ),
          ),
        ),

        // ESM voting block — pinned above the floating bottom nav so it
        // is never occluded, matching the reference design's bottom-anchored
        // layout (story-recap-v1-voted.jsx).
        if (state.story.hasEmbeddedEsmPrompt())
          Positioned(
            left: 24,
            right: 24,
            bottom: FloatingHeader.contentBottom(context),
            child: StoryEmbeddedEsm(story: state.story),
          ),

        // Feed header (if in feed context)
        if (widget.showFeedHeader &&
            widget.feedActor != null &&
            widget.feedOccurredAtUnixSec != null &&
            widget.feedActionText != null)
          ContentViewBuilders.buildFeedHeader(
            context: context,
            actor: widget.feedActor!,
            occurredAtUnixSec: widget.feedOccurredAtUnixSec!,
            actionText: widget.feedActionText!,
            isNavVisible: ref.watch(homeProvider).isNavVisible,
          ),
      ],
    );
  }

  bool _hasRelatedItem(StoryPayload story) {
    return story.gearId.isNotEmpty ||
        story.experienceId.isNotEmpty ||
        story.requestId.isNotEmpty;
  }

  void _navigateToRelatedItem(BuildContext context, StoryPayload story) {
    if (story.gearId.isNotEmpty) {
      NavigationHelpers.pushToItemScreen(context: context, itemId: story.gearId, itemType: 'gear');
    } else if (story.experienceId.isNotEmpty) {
      NavigationHelpers.pushToItemScreen(context: context, itemId: story.experienceId, itemType: 'experience');
    } else if (story.requestId.isNotEmpty) {
      NavigationHelpers.pushToItemScreen(context: context, itemId: story.requestId, itemType: 'request');
    }
  }

  /// Builds the Undo link surfaced on the story screen for the actor.
  /// Dispatches to the matching Undo* RPC based on story_type.
  Widget _buildUndoLink(BuildContext context, StoryPayload story) {
    return Padding(
      padding: const EdgeInsets.only(top: 32),
      child: TextButton(
        onPressed: () => _handleUndo(context, story),
        child: Text(
          story.undoableAction.label.isNotEmpty
              ? story.undoableAction.label
              : 'Undo',
          style: Theme.of(context).textTheme.labelLarge?.copyWith(
                color: GlassTokens.textPrimary,
                decoration: TextDecoration.underline,
                decorationColor: GlassTokens.textMuted,
                shadows: [
                  const Shadow(
                    offset: Offset(0, 1),
                    blurRadius: 6,
                    color: Colors.black45,
                  ),
                ],
              ),
        ),
      ),
    );
  }

  /// Dispatches the Undo RPC appropriate for this story's type, then
  /// navigates back to the now-active item screen on success. On
  /// failure, surfaces the typed UndoErrorDetail user_message and
  /// refreshes the story so the stale affordance disappears.
  Future<void> _handleUndo(BuildContext context, StoryPayload story) async {
    final communityEventId = story.undoableAction.communityEventId;
    final transferService = ref.read(transferServiceProvider);
    final requestService = ref.read(requestServiceProvider);
    final experienceService = ref.read(experienceServiceProvider);

    Future<void> call;
    switch (story.storyType) {
      case StoryType.STORY_TYPE_LOAN_COMPLETED:
        call = transferService.undoCompleteLoan(
          communityEventId: communityEventId,
        );
      case StoryType.STORY_TYPE_GIVEAWAY_COMPLETED:
        call = transferService.undoCompleteGiveaway(
          communityEventId: communityEventId,
        );
      case StoryType.STORY_TYPE_REQUEST_FULFILLED:
        call = requestService.undoMarkRequestFulfilled(
          communityEventId: communityEventId,
        );
      case StoryType.STORY_TYPE_EXPERIENCE_CONCLUDED:
        call = experienceService.undoCompleteExperience(
          communityEventId: communityEventId,
        );
      default:
        // Registry should prevent this — server only populates
        // undoable_action for server-authoritative story types.
        ToastHelper.showWarning(context, 'This action cannot be undone.');
        return;
    }

    try {
      await call;
      if (!context.mounted) return;
      ToastHelper.showSuccess(context, 'Undone.');
      // Navigate back to the now-active item screen (or pop the story
      // if we arrived here from the feed).
      unawaited(Navigator.of(context).maybePop());
    } on connect.ConnectException catch (e) {
      final userMessage = ToastHelper.extractUndoUserMessage(e) ?? e.message;
      if (!context.mounted) return;
      ToastHelper.showError(context, userMessage);
      // Refresh the story so the stale Undo link disappears if the
      // server no longer considers the action undoable.
      ref.invalidate(storyProvider(widget.story));
    } catch (e) {
      if (!context.mounted) return;
      ToastHelper.showError(context, e.toString());
    }
  }

  Widget _buildParticipantsList(BuildContext context, StoryState state) {
    // Show up to 5 participants in the list
    final visibleParticipants = state.participants.take(5).toList();
    final remainingCount =
        state.participants.length - visibleParticipants.length;

    return Wrap(
      alignment: WrapAlignment.center,
      spacing: 12,
      runSpacing: 12,
      children: [
        ...visibleParticipants.map(
          (participant) => _buildParticipantChip(context, participant),
        ),
        if (remainingCount > 0) _buildRemainingChip(context, remainingCount),
      ],
    );
  }

  Widget _buildParticipantChip(BuildContext context, User participant) {
    return Tappable(
      semanticsLabel: participant.name,
      onTap: () => ContentViewHelpers.openUserScreen(context, participant.id),
      child: Container(
        decoration: BoxDecoration(
          color: OverlayTokens.fieldFill,
          borderRadius: BorderRadius.circular(20),
          border: Border.all(
            color: GlassTokens.borderSoft,
            width: 1,
          ),
        ),
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            UserAvatar(user: participant, radius: 12),
            const SizedBox(width: 8),
            Text(
              participant.name.split(' ').first,
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: GlassTokens.textPrimary,
                fontWeight: FontWeight.w500,
                shadows: [
                  const Shadow(
                    offset: Offset(0, 1),
                    blurRadius: 2,
                    color: Colors.black45,
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildRemainingChip(BuildContext context, int count) {
    return Container(
      decoration: BoxDecoration(
        color: OverlayTokens.fieldFill,
        borderRadius: BorderRadius.circular(20),
        border: Border.all(
          color: GlassTokens.borderSoft,
          width: 1,
        ),
      ),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      child: Text(
        '+$count more',
        style: Theme.of(context).textTheme.bodyMedium?.copyWith(
          color: GlassTokens.textPrimary,
          fontWeight: FontWeight.w500,
          shadows: [
            const Shadow(
              offset: Offset(0, 1),
              blurRadius: 2,
              color: Colors.black45,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildSavingsDisplay(ImpactEstimate impact) {
    final savingsData = SavingsFormatter.formatSavings(impact);
    if (savingsData == null) return const SizedBox.shrink();

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: GlassTokens.borderSoft,
          width: 1,
        ),
      ),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceEvenly,
        children: [
          _buildSavingsMetric(
            context,
            context.l10n.impactMoneySaved,
            savingsData.costSaved,
          ),
          // Prefer the QualityTime composite when populated (experience
          // and request stories carry it). Falls back to TimeSaved for
          // gear stories. Both formatters render minutes-only via
          // SavingsFormatter so the number matches the completion modal,
          // QT detail modal, and results tab.
          _buildSavingsMetric(
            context,
            context.l10n.impactQualityTime,
            savingsData.qtSaved ?? savingsData.timeSaved,
          ),
          _buildSavingsMetric(
            context,
            context.l10n.impactCo2Avoided,
            savingsData.co2Saved,
          ),
        ],
      ),
    );
  }

  Widget _buildRecapMetricsRow(BuildContext context, ImpactEstimate impact) {
    final savingsData = SavingsFormatter.formatSavings(impact);
    if (savingsData == null) return const SizedBox.shrink();

    final timeValue = savingsData.qtSaved ?? savingsData.timeSaved;

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 12),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            height: 1,
            color: GlassTokens.borderSoft,
          ),
          const SizedBox(height: 14),
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              _buildRecapMetric(
                context,
                value: savingsData.costSaved,
                label: context.l10n.impactMoneySaved,
              ),
              _buildRecapMetric(
                context,
                value: timeValue,
                label: context.l10n.storyRecapTogether,
              ),
              _buildRecapMetric(
                context,
                value: savingsData.co2Saved,
                label: context.l10n.impactCo2Avoided,
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildRecapMetric(
    BuildContext context, {
    required String value,
    required String label,
  }) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.end,
      children: [
        Text(
          value,
          style: const TextStyle(
            color: GlassTokens.textPrimary,
            fontFamily: AppTheme.headingFont,
            fontSize: 19,
            fontWeight: FontWeight.w700,
            shadows: [
              Shadow(offset: Offset(0, 1), blurRadius: 4, color: Colors.black45),
            ],
          ),
        ),
        const SizedBox(width: 6),
        Padding(
          padding: const EdgeInsets.only(bottom: 3),
          child: Text(
            label,
            style: TextStyle(
              color: GlassTokens.textMuted,
              fontSize: 9.5,
              fontWeight: FontWeight.w600,
              letterSpacing: 1.2,
              shadows: const [
                Shadow(offset: Offset(0, 1), blurRadius: 2, color: Colors.black45),
              ],
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildSavingsMetric(BuildContext context, String label, String value) {
    return Column(
      children: [
        Text(
          value,
          style: Theme.of(context).textTheme.titleLarge?.copyWith(
            color: GlassTokens.textPrimary,
            fontWeight: FontWeight.bold,
            shadows: [
              const Shadow(
                offset: Offset(0, 1),
                blurRadius: 4,
                color: Colors.black45,
              ),
            ],
          ),
        ),
        const SizedBox(height: 4),
        Text(
          label,
          style: Theme.of(context).textTheme.labelSmall?.copyWith(
            color: GlassTokens.textSecondary,
            fontWeight: FontWeight.w600,
            letterSpacing: 0.5,
            shadows: [
              const Shadow(
                offset: Offset(0, 1),
                blurRadius: 2,
                color: Colors.black45,
              ),
            ],
          ),
        ),
      ],
    );
  }
}
