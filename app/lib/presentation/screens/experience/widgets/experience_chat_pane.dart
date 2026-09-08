import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/experience.pbenum.dart' as proto;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart' show RSVP;
import 'package:ripls/data/gen/ripls/api/experience_service.pbenum.dart'
    show RSVPIntention;
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/widgets/content/content_shared_widgets.dart'
    show ConversationPaneEmptyState;
import 'package:ripls/presentation/widgets/content/inline_conversation_view.dart';
import 'package:ripls/presentation/widgets/needs/needs_actions.dart';
import 'package:ripls/services/providers.dart';

/// ExperienceChatPane renders the chat tab pane for an experience, including
/// the inline conversation view with needs/contributions context.
class ExperienceChatPane extends ConsumerWidget {
  final String experienceId;
  final Color accentColor;
  final double paneHeight;
  final Future<void> Function() onAddMedia;
  final Future<void> Function(String mediaId) onDeleteMedia;
  final Future<void> Function(List<String> mediaIds)? onReorderMedia;

  /// Forces the conversation to initialize regardless of the carousel's active
  /// tab. Set true when the pane is hosted on its own surface (the morph-reveal
  /// conversation panel), where there is no active-tab signal to gate on; null
  /// (default) derives activeness from `state.activeTab == state.chatTabIndex`.
  final bool? forceActive;

  const ExperienceChatPane({
    super.key,
    required this.experienceId,
    required this.accentColor,
    required this.paneHeight,
    required this.onAddMedia,
    required this.onDeleteMedia,
    this.onReorderMedia,
    this.forceActive,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(experienceProvider(experienceId));
    final exp = state.experienceDetails?.experience;

    if (exp == null || exp.conversationId.isEmpty) {
      return const ConversationPaneEmptyState();
    }

    final needsState = ref.watch(experienceNeedsProvider(experienceId));
    final needById = {for (final n in needsState.needs) n.id: n};
    final contribById = {
      for (final c in needsState.contributions) c.id: c,
    };
    final experienceOwnerId = exp.owner.id;
    final isTerminal =
        exp.state == proto.ExperienceState.EXPERIENCE_STATE_COMPLETED ||
            exp.state == proto.ExperienceState.EXPERIENCE_STATE_CANCELLED;
    final rsvps = state.experienceDetails?.rsvps ?? [];
    final currentUserRsvp = rsvps.firstWhere(
      (r) => r.user.id == state.currentUserId,
      orElse: () => RSVP(),
    );
    final isRsvpedYes =
        currentUserRsvp.intention == RSVPIntention.RSVP_INTENTION_YES;
    final isRsvpedMaybe =
        currentUserRsvp.intention == RSVPIntention.RSVP_INTENTION_MAYBE;
    final needsActions = NeedsActions(
      scope: NeedsScope.experience(
        experienceId: experienceId,
        currentUserId: state.currentUserId,
        isTerminal: isTerminal,
        communityId: state.communityId ?? '',
        isRsvped: isRsvpedYes || isRsvpedMaybe,
        isRsvpedMaybe: isRsvpedMaybe,
        ownerId: experienceOwnerId,
        experienceName: exp.name,
      ),
    );

    return InlineConversationView(
      key: ValueKey(exp.conversationId),
      conversationId: exp.conversationId,
      accentColor: accentColor,
      isActive: forceActive ?? (state.activeTab == state.chatTabIndex),
      isExpanded: true,
      mediaItems: state.allMediaItems,
      onAddMedia: onAddMedia,
      onDeleteMedia: onDeleteMedia,
      isOwner: state.isOwner,
      onReorderMedia: onReorderMedia,
      onMessagesMarkedAsRead: (conversationId) =>
          resetUnreadForConversation(ref, conversationId),
      experienceNeedNames: {for (final n in needsState.needs) n.id: n.name},
      experienceContributionNames: {
        for (final c in needsState.contributions) c.id: c.title,
      },
      onExperienceNeedTap: (needId) {
        final need = needById[needId];
        if (need == null || !context.mounted) return;
        needsActions.onNeedTap(context, ref, need);
      },
      onExperienceContributionTap: (contribId) {
        final contrib = contribById[contribId];
        if (contrib == null || !context.mounted) return;
        needsActions.onContributionTap(context, ref, contrib);
      },
    );
  }
}
